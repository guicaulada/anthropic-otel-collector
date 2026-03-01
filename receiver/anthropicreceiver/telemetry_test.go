package anthropicreceiver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
)

func newTestTelemetryBuilder(t *testing.T) (*telemetryBuilder, *consumertest.TracesSink, *consumertest.MetricsSink, *consumertest.LogsSink) {
	t.Helper()
	cfg := defaultConfig()
	tracesSink := &consumertest.TracesSink{}
	metricsSink := &consumertest.MetricsSink{}
	logsSink := &consumertest.LogsSink{}
	tb := newTelemetryBuilder(cfg, zap.NewNop(), tracesSink, metricsSink, logsSink)
	return tb, tracesSink, metricsSink, logsSink
}

func newTestRequestData() *requestData {
	now := time.Now()
	return &requestData{
		startTime:       now.Add(-100 * time.Millisecond),
		endTime:         now,
		upstreamLatency: 80 * time.Millisecond,
		request: &AnthropicRequest{
			Model:     "claude-sonnet-4-6",
			MaxTokens: 1024,
			Messages: []Message{
				{Role: "user", Content: []byte(`"Hello"`)},
			},
		},
		requestSize: 100,
		apiKeyHash:  "abcd1234",
		response: &AnthropicResponse{
			ID:         "msg_test",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-sonnet-4-6",
			StopReason: "end_turn",
			Content: []ContentBlock{
				{Type: "text", Text: "Hello, how can I help?"},
			},
			Usage: Usage{
				InputTokens:  100,
				OutputTokens: 20,
			},
		},
		responseSize: 200,
		statusCode:   200,
		requestID:    "req_test_001",
		rateLimit: RateLimitInfo{
			RequestsLimit:         1000,
			RequestsRemaining:     900,
			InputTokensLimit:      100000,
			InputTokensRemaining:  80000,
			OutputTokensLimit:     50000,
			OutputTokensRemaining: 40000,
		},
		cost: CostResult{
			InputCost:  0.0003,
			OutputCost: 0.0003,
			TotalCost:  0.0006,
		},
	}
}

// --- requestData.requestModel tests ---

func TestRequestData_RequestModel(t *testing.T) {
	t.Run("from request", func(t *testing.T) {
		data := &requestData{
			request:  &AnthropicRequest{Model: "claude-sonnet-4-6"},
			response: &AnthropicResponse{Model: "claude-sonnet-4-6-20250514"},
		}
		assert.Equal(t, "claude-sonnet-4-6", data.requestModel())
	})

	t.Run("fallback to response model", func(t *testing.T) {
		data := &requestData{
			response: &AnthropicResponse{Model: "claude-sonnet-4-6"},
		}
		assert.Equal(t, "claude-sonnet-4-6", data.requestModel())
	})

	t.Run("fallback to unknown", func(t *testing.T) {
		data := &requestData{}
		assert.Equal(t, "unknown", data.requestModel())
	})
}

// --- generateTraceID / generateSpanID tests ---

func TestGenerateTraceID_Deterministic(t *testing.T) {
	data := &requestData{
		requestID: "req_123",
		startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	tid1 := generateTraceID(data)
	tid2 := generateTraceID(data)

	assert.Equal(t, tid1, tid2, "same inputs should produce same trace ID")
	assert.NotEqual(t, pcommon.TraceID{}, tid1, "trace ID should not be zero")
}

func TestGenerateSpanID_Deterministic(t *testing.T) {
	data := &requestData{
		requestID: "req_456",
		startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	sid1 := generateSpanID(data, 0)
	sid2 := generateSpanID(data, 0)
	assert.Equal(t, sid1, sid2, "same inputs should produce same span ID")
	assert.NotEqual(t, pcommon.SpanID{}, sid1, "span ID should not be zero")
}

func TestGenerateSpanID_DifferentIndexes(t *testing.T) {
	data := &requestData{
		requestID: "req_789",
		startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	sid0 := generateSpanID(data, 0)
	sid1 := generateSpanID(data, 1)

	assert.NotEqual(t, sid0, sid1, "different indexes should produce different span IDs")
}

func TestGenerateTraceID_DifferentData(t *testing.T) {
	data1 := &requestData{
		requestID: "req_aaa",
		startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data2 := &requestData{
		requestID: "req_bbb",
		startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	tid1 := generateTraceID(data1)
	tid2 := generateTraceID(data2)

	assert.NotEqual(t, tid1, tid2, "different data should produce different trace IDs")
}
