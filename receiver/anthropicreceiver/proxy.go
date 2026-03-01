package anthropicreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// proxyRequestContext holds all context for a proxied request, avoiding long parameter lists.
type proxyRequestContext struct {
	ctx             context.Context
	startTime       time.Time
	upstreamLatency time.Duration
	anthropicReq    *AnthropicRequest
	requestBody     []byte
	apiKeyHash      string
	rateLimit       RateLimitInfo
	requestID       string
	betaFeatures    string
	apiVersion      string
}

func (r *anthropicReceiver) handleProxy(w http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	atomic.AddInt64(&r.activeRequests, 1)
	defer atomic.AddInt64(&r.activeRequests, -1)

	ctx := req.Context()

	// Read request body with size limit
	var bodyReader io.Reader = req.Body
	if r.cfg.MaxRequestBodySize > 0 {
		bodyReader = io.LimitReader(req.Body, r.cfg.MaxRequestBodySize+1)
	}
	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	req.Body.Close()
	if r.cfg.MaxRequestBodySize > 0 && int64(len(bodyBytes)) > r.cfg.MaxRequestBodySize {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Parse request
	var anthropicReq AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		r.logger.Warn("Failed to parse request body", zap.Error(err))
		// Forward the request anyway — let the upstream validate
	}

	// Extract API key hash
	apiKey := req.Header.Get("x-api-key")
	apiKeyHash := hashAPIKey(apiKey)

	// Build upstream request
	upstreamURL := r.upstreamURL.JoinPath(req.URL.Path)
	if req.URL.RawQuery != "" {
		upstreamURL.RawQuery = req.URL.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, req.Method, upstreamURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "Failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers, but strip Accept-Encoding so we get uncompressed responses
	// that we can parse for telemetry extraction.
	for key, values := range req.Header {
		if strings.EqualFold(key, "Accept-Encoding") {
			continue
		}
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
		}
	}

	// Execute upstream request
	upstreamStart := time.Now()
	resp, err := r.httpClient.Do(upstreamReq)
	if err != nil {
		r.logger.Error("Upstream request failed", zap.Error(err))
		http.Error(w, "Upstream request failed", http.StatusBadGateway)
		return
	}
	upstreamLatency := time.Since(upstreamStart)
	defer resp.Body.Close()

	// Copy response headers to client
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Extract rate limit info
	rateLimit := ExtractRateLimitInfo(resp.Header)
	requestID := resp.Header.Get("request-id")

	// Capture request headers for telemetry
	betaFeatures := req.Header.Get("anthropic-beta")
	apiVersion := req.Header.Get("anthropic-version")

	prc := &proxyRequestContext{
		ctx:             ctx,
		startTime:       startTime,
		upstreamLatency: upstreamLatency,
		anthropicReq:    &anthropicReq,
		requestBody:     bodyBytes,
		apiKeyHash:      apiKeyHash,
		rateLimit:       rateLimit,
		requestID:       requestID,
		betaFeatures:    betaFeatures,
		apiVersion:      apiVersion,
	}

	if anthropicReq.Stream && resp.StatusCode == http.StatusOK {
		r.handleStreamingResponse(w, resp, prc)
	} else {
		r.handleNonStreamingResponse(w, resp, prc)
	}
}

func (r *anthropicReceiver) handleNonStreamingResponse(
	w http.ResponseWriter,
	resp *http.Response,
	prc *proxyRequestContext,
) {
	// Read full response
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Error("Failed to read response body", zap.Error(err))
		http.Error(w, "Failed to read response", http.StatusBadGateway)
		return
	}

	// Write response to client
	w.WriteHeader(resp.StatusCode)
	w.Write(respBytes)

	endTime := time.Now()

	// Parse response
	var anthropicResp AnthropicResponse
	var anthropicErr *AnthropicError
	var toolCalls []ToolCallInfo

	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(respBytes, &anthropicResp); err != nil {
			r.logger.Warn("Failed to parse response body", zap.Error(err))
		} else if r.cfg.ParseToolCalls {
			toolCalls = ParseToolCalls(anthropicResp.Content)
		}
	} else {
		var errResp AnthropicError
		if err := json.Unmarshal(respBytes, &errResp); err == nil {
			anthropicErr = &errResp
		}
	}

	// Compute cost
	var cost CostResult
	if resp.StatusCode == http.StatusOK {
		responseModel := anthropicResp.Model
		if responseModel == "" {
			responseModel = prc.anthropicReq.Model
		}
		costCtx := CostContext{
			Speed:            anthropicResp.Usage.Speed,
			TotalInputTokens: anthropicResp.Usage.TotalInputTokens(),
		}
		cost = ComputeCost(responseModel, anthropicResp.Usage, r.cfg.Pricing, costCtx)
	}

	// Extract additional metadata
	speed := ""
	orgID := prc.rateLimit.OrganizationID
	if resp.StatusCode == http.StatusOK {
		speed = anthropicResp.Usage.Speed
	}

	// Build request data
	data := &requestData{
		startTime:       prc.startTime,
		endTime:         endTime,
		upstreamLatency: prc.upstreamLatency,
		request:         prc.anthropicReq,
		requestBody:     prc.requestBody,
		requestSize:     len(prc.requestBody),
		apiKeyHash:      prc.apiKeyHash,
		responseBody:    respBytes,
		responseSize:    len(respBytes),
		statusCode:      resp.StatusCode,
		requestID:       prc.requestID,
		rateLimit:       prc.rateLimit,
		isStreaming:     false,
		toolCalls:       toolCalls,
		cost:            cost,
		errorResponse:   anthropicErr,
		activeRequests:  atomic.LoadInt64(&r.activeRequests),
		betaFeatures:    prc.betaFeatures,
		organizationID:  orgID,
		speed:           speed,
		apiVersion:      prc.apiVersion,
	}

	if resp.StatusCode == http.StatusOK {
		data.response = &anthropicResp
	}

	// Emit telemetry
	r.telemetry.emit(prc.ctx, data)
}

func (r *anthropicReceiver) handleStreamingResponse(
	w http.ResponseWriter,
	resp *http.Response,
	prc *proxyRequestContext,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		r.logger.Error("ResponseWriter does not support flushing")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)

	accumulator := newStreamAccumulator()
	var responseSize int

	// Read SSE stream line-by-line, forward to client, and accumulate
	reader := resp.Body
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	// Track for SSE parsing
	var currentEvent string
	var dataLines []string

	for {
		n, readErr := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)

			// Process complete lines
			for {
				idx := bytes.IndexByte(buf, '\n')
				if idx < 0 {
					break
				}
				line := string(buf[:idx])
				buf = buf[idx+1:]

				// Write line to client
				lineBytes := []byte(line + "\n")
				responseSize += len(lineBytes)
				w.Write(lineBytes)
				flusher.Flush()

				// Parse SSE
				line = strings.TrimRight(line, "\r")
				if line == "" {
					// End of event
					if currentEvent != "" && len(dataLines) > 0 {
						data := strings.Join(dataLines, "\n")
						event := SSEEvent{
							Event: currentEvent,
							Data:  json.RawMessage(data),
						}
						if err := accumulator.ProcessEvent(event); err != nil {
							r.logger.Warn("Failed to process SSE event",
								zap.String("event", currentEvent),
								zap.Error(err))
						}
					}
					currentEvent = ""
					dataLines = nil
				} else if strings.HasPrefix(line, "event: ") {
					currentEvent = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "data: ") {
					dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
				}
			}
		}

		if readErr != nil {
			if readErr != io.EOF {
				r.logger.Error("Error reading stream", zap.Error(readErr))
			}
			// Flush remaining buffer
			if len(buf) > 0 {
				w.Write(buf)
				flusher.Flush()
				responseSize += len(buf)
			}
			break
		}
	}

	endTime := time.Now()
	streamingMetrics := accumulator.StreamingMetrics()
	anthropicResp := accumulator.Response()

	// Parse tool calls
	var toolCalls []ToolCallInfo
	if r.cfg.ParseToolCalls {
		toolCalls = ParseToolCalls(anthropicResp.Content)
	}

	// Compute cost
	responseModel := anthropicResp.Model
	if responseModel == "" {
		responseModel = prc.anthropicReq.Model
	}
	costCtx := CostContext{
		Speed:            anthropicResp.Usage.Speed,
		TotalInputTokens: anthropicResp.Usage.TotalInputTokens(),
	}
	cost := ComputeCost(responseModel, anthropicResp.Usage, r.cfg.Pricing, costCtx)

	// Reconstruct response body for logging
	var responseBody []byte
	if r.cfg.CaptureResponseBody {
		responseBody, _ = json.Marshal(anthropicResp)
	}

	// Build request data
	data := &requestData{
		startTime:       prc.startTime,
		endTime:         endTime,
		upstreamLatency: prc.upstreamLatency,
		request:         prc.anthropicReq,
		requestBody:     prc.requestBody,
		requestSize:     len(prc.requestBody),
		apiKeyHash:      prc.apiKeyHash,
		response:        anthropicResp,
		responseBody:    responseBody,
		responseSize:    responseSize,
		statusCode:      resp.StatusCode,
		requestID:       prc.requestID,
		rateLimit:       prc.rateLimit,
		isStreaming:     true,
		streaming:       &streamingMetrics,
		toolCalls:       toolCalls,
		cost:            cost,
		activeRequests:  atomic.LoadInt64(&r.activeRequests),
		betaFeatures:    prc.betaFeatures,
		organizationID:  prc.rateLimit.OrganizationID,
		speed:           anthropicResp.Usage.Speed,
		apiVersion:      prc.apiVersion,
	}

	// Emit telemetry
	r.telemetry.emit(prc.ctx, data)
}

// activeRequestsGauge returns the current number of active requests.
func (r *anthropicReceiver) activeRequestsGauge() int64 {
	return atomic.LoadInt64(&r.activeRequests)
}

// truncateBody truncates a body to maxSize bytes, returning the truncated body.
func truncateBody(body []byte, maxSize int) []byte {
	if maxSize <= 0 || len(body) <= maxSize {
		return body
	}
	truncated := make([]byte, maxSize)
	copy(truncated, body)
	return append(truncated, []byte(fmt.Sprintf("... [truncated, %d bytes total]", len(body)))...)
}
