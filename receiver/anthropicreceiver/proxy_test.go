package anthropicreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func setupTestReceiver(t *testing.T, upstreamURL string) (*anthropicReceiver, int) {
	t.Helper()

	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	port := getFreePort(t)

	cfg := defaultConfig()
	cfg.ServerConfig.NetAddr.Endpoint = fmt.Sprintf("127.0.0.1:%d", port)
	cfg.AnthropicAPI = upstreamURL
	cfg.ParseToolCalls = true

	settings := receivertest.NewNopSettings(componentType)
	r := newAnthropicReceiver(cfg, settings)
	r.tracesConsumer = consumertest.NewNop()
	r.metricsConsumer = consumertest.NewNop()
	r.logsConsumer = consumertest.NewNop()

	err := r.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	// Wait for server to be ready
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}, 2*time.Second, 50*time.Millisecond)

	t.Cleanup(func() {
		r.Shutdown(context.Background())
	})

	return r, port
}

func TestProxy_NonStreamingRequest(t *testing.T) {
	// Mock upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request was forwarded
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "claude-sonnet-4-6")

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "req_test_123")
		w.Header().Set("anthropic-ratelimit-requests-limit", "1000")
		w.Header().Set("anthropic-ratelimit-requests-remaining", "999")

		resp := AnthropicResponse{
			ID:         "msg_test",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-sonnet-4-6",
			StopReason: "end_turn",
			Content: []ContentBlock{
				{Type: "text", Text: "Hello from the test!"},
			},
			Usage: Usage{
				InputTokens:  100,
				OutputTokens: 20,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	_, port := setupTestReceiver(t, upstream.URL)

	// Send request to receiver proxy
	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
		"application/json",
		strings.NewReader(reqBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response AnthropicResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "msg_test", response.ID)
	assert.Equal(t, "Hello from the test!", response.TextContent())
	assert.Equal(t, 100, response.Usage.InputTokens)
	assert.Equal(t, 20, response.Usage.OutputTokens)
}

func TestProxy_StreamingRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("request-id", "req_stream_123")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-6\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":5}}\n\n",
			"event: message_stop\ndata: {}\n\n",
		}

		for _, ev := range events {
			fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	_, port := setupTestReceiver(t, upstream.URL)

	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`
	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
		"application/json",
		strings.NewReader(reqBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read and verify SSE stream was forwarded
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	bodyStr := string(body)
	assert.Contains(t, bodyStr, "event: message_start")
	assert.Contains(t, bodyStr, "event: content_block_delta")
	assert.Contains(t, bodyStr, "event: message_stop")
}

func TestProxy_ErrorResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		errResp := AnthropicError{
			Type: "error",
			Error: ErrorDetail{
				Type:    "authentication_error",
				Message: "Invalid API key",
			},
		}
		json.NewEncoder(w).Encode(errResp)
	}))
	defer upstream.Close()

	_, port := setupTestReceiver(t, upstream.URL)

	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
		"application/json",
		strings.NewReader(reqBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var errResp AnthropicError
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "authentication_error", errResp.Error.Type)
}

func TestProxy_RateLimitErrorResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		errResp := AnthropicError{
			Type: "error",
			Error: ErrorDetail{
				Type:    "rate_limit_error",
				Message: "Rate limit exceeded",
			},
		}
		json.NewEncoder(w).Encode(errResp)
	}))
	defer upstream.Close()

	_, port := setupTestReceiver(t, upstream.URL)

	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
		"application/json",
		strings.NewReader(reqBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestHashAPIKey(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		assert.Equal(t, "", hashAPIKey(""))
	})

	t.Run("returns 8 char hex", func(t *testing.T) {
		hash := hashAPIKey("sk-ant-test-key-123")
		assert.Len(t, hash, 8)
		// Verify it's hex characters
		for _, c := range hash {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'), "expected hex character, got %c", c)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		hash1 := hashAPIKey("sk-ant-test-key-456")
		hash2 := hashAPIKey("sk-ant-test-key-456")
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different keys produce different hashes", func(t *testing.T) {
		hash1 := hashAPIKey("key-a")
		hash2 := hashAPIKey("key-b")
		assert.NotEqual(t, hash1, hash2)
	})
}

func TestActiveRequestsGauge(t *testing.T) {
	r := &anthropicReceiver{}

	assert.Equal(t, int64(0), r.activeRequestsGauge())

	atomic.AddInt64(&r.activeRequests, 1)
	assert.Equal(t, int64(1), r.activeRequestsGauge())

	atomic.AddInt64(&r.activeRequests, 1)
	assert.Equal(t, int64(2), r.activeRequestsGauge())

	atomic.AddInt64(&r.activeRequests, -1)
	assert.Equal(t, int64(1), r.activeRequestsGauge())
}

func TestTruncateBody(t *testing.T) {
	t.Run("nil body", func(t *testing.T) {
		result := truncateBody(nil, 100)
		assert.Nil(t, result)
	})

	t.Run("body within limit", func(t *testing.T) {
		body := []byte("short body")
		result := truncateBody(body, 100)
		assert.Equal(t, body, result)
	})

	t.Run("body exceeds limit", func(t *testing.T) {
		body := []byte("this is a longer body that exceeds the limit")
		result := truncateBody(body, 10)
		assert.Contains(t, string(result), "this is a ")
		assert.Contains(t, string(result), "truncated")
	})

	t.Run("zero max size", func(t *testing.T) {
		body := []byte("test")
		result := truncateBody(body, 0)
		assert.Equal(t, body, result)
	})
}

func TestProxy_ConcurrentRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // small delay to ensure concurrency
		w.Header().Set("Content-Type", "application/json")
		resp := AnthropicResponse{
			ID:         "msg_concurrent",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-sonnet-4-6",
			StopReason: "end_turn",
			Content:    []ContentBlock{{Type: "text", Text: "ok"}},
			Usage:      Usage{InputTokens: 10, OutputTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	recv, port := setupTestReceiver(t, upstream.URL)

	const n = 10
	type result struct {
		status int
		err    error
	}
	results := make([]result, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
			resp, err := http.Post(
				fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
				"application/json",
				strings.NewReader(reqBody),
			)
			if err != nil {
				results[idx] = result{err: err}
				return
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body)
			results[idx] = result{status: resp.StatusCode}
		}(i)
	}

	wg.Wait()

	for i, r := range results {
		assert.NoError(t, r.err, "request %d should not error", i)
		if r.err == nil {
			assert.Equal(t, http.StatusOK, r.status, "request %d should return 200", i)
		}
	}

	// After all requests complete, activeRequests should be 0
	assert.Equal(t, int64(0), recv.activeRequestsGauge())
}

func TestProxy_UpstreamTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond — simulate a hung upstream
		select {
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	}))
	defer upstream.Close()

	_, port := setupTestReceiver(t, upstream.URL)

	// Override the receiver's HTTP client timeout to something short for testing
	// We can't easily access the receiver's client, so we use a client with a short timeout
	client := &http.Client{Timeout: 500 * time.Millisecond}
	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`

	resp, err := client.Post(
		fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
		"application/json",
		strings.NewReader(reqBody),
	)

	if err != nil {
		// Client timeout — expected since upstream never responds
		assert.Contains(t, err.Error(), "context deadline exceeded")
		return
	}
	defer resp.Body.Close()
	// If we got a response, it should be 502 (bad gateway) from the proxy's own timeout
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}

func TestProxy_RequestBodyTooLarge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	port := getFreePort(t)

	cfg := defaultConfig()
	cfg.ServerConfig.NetAddr.Endpoint = fmt.Sprintf("127.0.0.1:%d", port)
	cfg.AnthropicAPI = upstream.URL
	cfg.MaxRequestBodySize = 100 // 100 bytes limit

	settings := receivertest.NewNopSettings(componentType)
	r := newAnthropicReceiver(cfg, settings)
	r.tracesConsumer = consumertest.NewNop()
	r.metricsConsumer = consumertest.NewNop()
	r.logsConsumer = consumertest.NewNop()

	err := r.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}, 2*time.Second, 50*time.Millisecond)

	t.Cleanup(func() {
		r.Shutdown(context.Background())
	})

	// Send a request body that exceeds the limit
	largeBody := strings.Repeat("x", 200)
	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port),
		"application/json",
		strings.NewReader(largeBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestProxy_StreamingWithNewFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify beta header is forwarded
		assert.Equal(t, "output-128k-2025-02-19", r.Header.Get("anthropic-beta"))

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("request-id", "req_integration_001")
		w.Header().Set("anthropic-ratelimit-requests-limit", "1000")
		w.Header().Set("anthropic-ratelimit-requests-remaining", "900")
		w.Header().Set("anthropic-ratelimit-requests-reset", "2025-01-15T12:00:00Z")
		w.Header().Set("anthropic-ratelimit-input-tokens-limit", "100000")
		w.Header().Set("anthropic-ratelimit-input-tokens-remaining", "80000")
		w.Header().Set("anthropic-ratelimit-tokens-limit", "200000")
		w.Header().Set("anthropic-ratelimit-tokens-remaining", "150000")
		w.Header().Set("x-anthropic-organization-id", "org_test123")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_int\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-6\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":50,\"output_tokens\":0,\"speed\":\"fast\"}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello!\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig123\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"server_tool_use\",\"id\":\"stu_1\",\"name\":\"web_search\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"test\\\"}\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":10,\"input_tokens\":50,\"cache_read_input_tokens\":20}}\n\n",
			"event: message_stop\ndata: {}\n\n",
		}

		for _, ev := range events {
			fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	_, port := setupTestReceiver(t, upstream.URL)

	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`
	req, err := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/v1/messages", port), strings.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "output-128k-2025-02-19")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	bodyStr := string(body)
	assert.Contains(t, bodyStr, "event: message_start")
	assert.Contains(t, bodyStr, "signature_delta")
	assert.Contains(t, bodyStr, "server_tool_use")
	assert.Contains(t, bodyStr, "event: message_stop")
}
