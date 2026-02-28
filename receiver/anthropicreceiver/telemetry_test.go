package anthropicreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// --- emitTraces tests ---

func TestEmitTraces(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	rs := traces[0].ResourceSpans()
	require.Equal(t, 1, rs.Len())

	ss := rs.At(0).ScopeSpans()
	require.Equal(t, 1, ss.Len())

	spans := ss.At(0).Spans()
	require.GreaterOrEqual(t, spans.Len(), 2) // root span + upstream span

	// Root span
	rootSpan := spans.At(0)
	assert.Equal(t, "chat claude-sonnet-4-6", rootSpan.Name())

	// Verify attributes
	attrs := rootSpan.Attributes()
	val, ok := attrs.Get("gen_ai.provider.name")
	require.True(t, ok)
	assert.Equal(t, "anthropic", val.Str())

	val, ok = attrs.Get("gen_ai.request.model")
	require.True(t, ok)
	assert.Equal(t, "claude-sonnet-4-6", val.Str())

	val, ok = attrs.Get("gen_ai.usage.input_tokens")
	require.True(t, ok)
	assert.Equal(t, int64(100), val.Int())

	val, ok = attrs.Get("gen_ai.usage.output_tokens")
	require.True(t, ok)
	assert.Equal(t, int64(20), val.Int())

	val, ok = attrs.Get("anthropic.api_key_hash")
	require.True(t, ok)
	assert.Equal(t, "abcd1234", val.Str())

	// Verify events exist
	events := rootSpan.Events()
	assert.GreaterOrEqual(t, events.Len(), 2) // at least request + response events

	// Find the gen_ai.request event
	var foundReqEvent, foundRespEvent bool
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		switch ev.Name() {
		case "gen_ai.request":
			foundReqEvent = true
		case "gen_ai.response":
			foundRespEvent = true
		}
	}
	assert.True(t, foundReqEvent, "should have gen_ai.request event")
	assert.True(t, foundRespEvent, "should have gen_ai.response event")

	// Upstream span
	upstreamSpan := spans.At(1)
	assert.Equal(t, "POST /v1/messages", upstreamSpan.Name())
}

func TestEmitTraces_ErrorStatus(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 401
	data.response = nil
	data.errorResponse = &AnthropicError{
		Type: "error",
		Error: ErrorDetail{
			Type:    "authentication_error",
			Message: "Invalid API key",
		},
	}

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	spans := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	rootSpan := spans.At(0)

	assert.Equal(t, "chat claude-sonnet-4-6", rootSpan.Name())

	// Verify error event exists
	events := rootSpan.Events()
	var foundErrorEvent bool
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		if ev.Name() == "gen_ai.error" {
			foundErrorEvent = true
			errType, _ := ev.Attributes().Get("error.type")
			assert.Equal(t, "authentication_error", errType.Str())
			errMsg, _ := ev.Attributes().Get("error.message")
			assert.Equal(t, "Invalid API key", errMsg.Str())
		}
	}
	assert.True(t, foundErrorEvent)
}

// --- emitMetrics tests ---

func TestEmitMetrics(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	rm := metrics[0].ResourceMetrics()
	require.Equal(t, 1, rm.Len())

	sm := rm.At(0).ScopeMetrics()
	require.Equal(t, 1, sm.Len())

	// Collect all metric names
	allMetrics := sm.At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	// Verify expected metric names are present
	expectedMetrics := []string{
		"gen_ai.client.operation.duration",
		"gen_ai.client.token.usage",
		"anthropic.requests",
		"anthropic.request.body.size",
		"anthropic.response.body.size",
		"anthropic.upstream.latency",
		"anthropic.tokens.input",
		"anthropic.tokens.output",
		"anthropic.cache.hit_ratio",
		"anthropic.stop_reason",
		"anthropic.content_blocks",
		"anthropic.response.text_length",
		"anthropic.ratelimit.requests.limit",
		"anthropic.request.max_tokens",
		"anthropic.request.messages_count",
		"anthropic.cost.request",
		"anthropic.cost.total",
	}

	for _, name := range expectedMetrics {
		assert.True(t, metricNames[name], "expected metric %q to be present", name)
	}
}

func TestEmitMetrics_ErrorMetric(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 500
	data.response = nil
	data.cost = CostResult{}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	sm := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0)
	allMetrics := sm.Metrics()

	var foundErrorMetric bool
	for i := 0; i < allMetrics.Len(); i++ {
		if allMetrics.At(i).Name() == "anthropic.errors" {
			foundErrorMetric = true
		}
	}
	assert.True(t, foundErrorMetric, "should emit anthropic.errors metric for error status")
}

// --- emitLogs tests ---

func TestEmitLogs(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	rl := logs[0].ResourceLogs()
	require.Equal(t, 1, rl.Len())

	sl := rl.At(0).ScopeLogs()
	require.Equal(t, 1, sl.Len())

	logRecords := sl.At(0).LogRecords()
	// Should have at least the operation log and cost log
	assert.GreaterOrEqual(t, logRecords.Len(), 2)

	// Check the first log record (operation log)
	firstLog := logRecords.At(0)
	bodyMap := firstLog.Body().Map()

	val, ok := bodyMap.Get("gen_ai.operation.name")
	require.True(t, ok)
	assert.Equal(t, "chat", val.Str())

	val, ok = bodyMap.Get("gen_ai.provider.name")
	require.True(t, ok)
	assert.Equal(t, "anthropic", val.Str())
}

func TestEmitLogs_WithError(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 401
	data.errorResponse = &AnthropicError{
		Type: "error",
		Error: ErrorDetail{
			Type:    "authentication_error",
			Message: "Invalid API key",
		},
	}

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()

	// Find error log
	var foundErrorLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "gen_ai.error" {
			foundErrorLog = true
			assert.Contains(t, lr.Body().Str(), "authentication_error")
		}
	}
	assert.True(t, foundErrorLog)
}

func TestEmitLogs_WithStreaming(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.isStreaming = true
	data.streaming = &StreamingMetrics{
		TotalEvents:      10,
		TotalChunks:      5,
		HasFirstToken:    true,
		TimeToFirstToken: 100 * time.Millisecond,
		Duration:         1 * time.Second,
	}

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()

	var foundStreamingLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "anthropic.streaming.summary" {
			foundStreamingLog = true
		}
	}
	assert.True(t, foundStreamingLog, "should have streaming summary log")
}

func TestEmitLogs_WithToolCalls(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.Content = append(data.response.Content, ContentBlock{
		Type: "tool_use",
		Name: "Edit",
		ID:   "tc_1",
	})
	data.toolCalls = []ToolCallInfo{
		{
			ToolName:     "Edit",
			ToolCallID:   "tc_1",
			FilePath:     "/src/main.go",
			FileExt:      ".go",
			LinesAdded:   5,
			LinesRemoved: 3,
		},
	}

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()

	var foundToolCallLog, foundDetailedToolCallLog, foundFileChangeLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if !ok {
			continue
		}
		switch eventName.Str() {
		case "gen_ai.tool_call":
			foundToolCallLog = true
		case "anthropic.tool_call":
			foundDetailedToolCallLog = true
		case "anthropic.file_change":
			foundFileChangeLog = true
		}
	}
	assert.True(t, foundToolCallLog, "should have tool call log")
	assert.True(t, foundDetailedToolCallLog, "should have detailed tool call log")
	assert.True(t, foundFileChangeLog, "should have file change log")
}

func TestEmitLogs_WithBodyCapture(t *testing.T) {
	cfg := defaultConfig()
	cfg.CaptureRequestBody = true
	cfg.CaptureResponseBody = true

	logsSink := &consumertest.LogsSink{}
	tb := newTelemetryBuilder(cfg, zap.NewNop(), nil, nil, logsSink)

	data := newTestRequestData()
	data.requestBody = []byte(`{"model":"claude-sonnet-4-6"}`)
	data.responseBody = []byte(`{"id":"msg_test"}`)

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()

	var foundReqBody, foundRespBody bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if !ok {
			continue
		}
		switch eventName.Str() {
		case "gen_ai.request.body":
			foundReqBody = true
		case "gen_ai.response.body":
			foundRespBody = true
		}
	}
	assert.True(t, foundReqBody, "should have request body log")
	assert.True(t, foundRespBody, "should have response body log")
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

// --- New span attributes tests ---

func TestEmitTraces_NewSpanAttributes(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.request.StopSequences = []string{"END", "STOP"}
	data.request.Messages = []Message{
		{Role: "user", Content: []byte(`"Hello"`)},
		{Role: "assistant", Content: []byte(`"Hi"`)},
		{Role: "user", Content: []byte(`"How are you?"`)},
	}
	stopSeq := "END"
	data.response.StopSequence = &stopSeq
	data.response.Content = []ContentBlock{
		{Type: "thinking", Thinking: "Let me think about this carefully"},
		{Type: "text", Text: "Hello, how can I help?"},
	}

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	rootSpan := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	attrs := rootSpan.Attributes()

	// stop_sequences_count
	val, ok := attrs.Get("anthropic.request.stop_sequences_count")
	require.True(t, ok, "should have stop_sequences_count")
	assert.Equal(t, int64(2), val.Int())

	// per-role message counts
	val, ok = attrs.Get("anthropic.request.user_messages_count")
	require.True(t, ok, "should have user_messages_count")
	assert.Equal(t, int64(2), val.Int())

	val, ok = attrs.Get("anthropic.request.assistant_messages_count")
	require.True(t, ok, "should have assistant_messages_count")
	assert.Equal(t, int64(1), val.Int())

	// stop_sequence
	val, ok = attrs.Get("gen_ai.response.stop_sequence")
	require.True(t, ok, "should have stop_sequence")
	assert.Equal(t, "END", val.Str())

	// thinking_length
	val, ok = attrs.Get("anthropic.response.thinking_length")
	require.True(t, ok, "should have thinking_length")
	assert.Equal(t, int64(len("Let me think about this carefully")), val.Int())
}

// --- Glob/Grep span events tests ---

func TestEmitTraces_GlobGrepEvents(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.toolCalls = []ToolCallInfo{
		{
			ToolName: "Glob",
			FilePath: "/src",
			Pattern:  "**/*.go",
		},
		{
			ToolName: "Grep",
			FilePath: "/src",
			Pattern:  "func main",
		},
	}

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	rootSpan := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	events := rootSpan.Events()

	var foundGlob, foundGrep bool
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		switch ev.Name() {
		case "anthropic.tool_use.glob":
			foundGlob = true
			pattern, _ := ev.Attributes().Get("pattern")
			assert.Equal(t, "**/*.go", pattern.Str())
			path, ok := ev.Attributes().Get("file.path")
			assert.True(t, ok)
			assert.Equal(t, "/src", path.Str())
		case "anthropic.tool_use.grep":
			foundGrep = true
			pattern, _ := ev.Attributes().Get("pattern")
			assert.Equal(t, "func main", pattern.Str())
		}
	}
	assert.True(t, foundGlob, "should have glob event")
	assert.True(t, foundGrep, "should have grep event")
}

// --- New metrics tests ---

func TestEmitMetrics_ThinkingOutputLength(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.Content = []ContentBlock{
		{Type: "thinking", Thinking: "Some deep thought here"},
		{Type: "text", Text: "The answer"},
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.thinking.output_length"], "should emit thinking output length metric")
}

func TestEmitMetrics_GlobGrepSearches(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.toolCalls = []ToolCallInfo{
		{ToolName: "Glob", Pattern: "**/*.go"},
		{ToolName: "Grep", Pattern: "func main"},
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.tool_use.glob_searches"], "should emit glob searches metric")
	assert.True(t, metricNames["anthropic.tool_use.grep_searches"], "should emit grep searches metric")
}

func TestEmitMetrics_FileTypeMetric(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.toolCalls = []ToolCallInfo{
		{ToolName: "Edit", FilePath: "/src/main.go", FileExt: ".go", LinesAdded: 5, LinesRemoved: 3},
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.tool_use.file_type"], "should emit file_type metric")
}

// --- New log fields tests ---

func TestEmitLogs_OperationLogNewFields(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.request.Messages = []Message{
		{Role: "user", Content: []byte(`"Hello"`)},
		{Role: "assistant", Content: []byte(`"Hi"`)},
		{Role: "user", Content: []byte(`"How?"`)},
	}
	stopSeq := "STOP"
	data.response.StopSequence = &stopSeq
	data.response.Content = []ContentBlock{
		{Type: "thinking", Thinking: "Deep thought"},
		{Type: "text", Text: "Answer"},
	}

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	firstLog := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	bodyMap := firstLog.Body().Map()

	val, ok := bodyMap.Get("anthropic.request.user_messages_count")
	require.True(t, ok, "should have user_messages_count in operation log")
	assert.Equal(t, int64(2), val.Int())

	val, ok = bodyMap.Get("anthropic.request.assistant_messages_count")
	require.True(t, ok, "should have assistant_messages_count in operation log")
	assert.Equal(t, int64(1), val.Int())

	val, ok = bodyMap.Get("gen_ai.response.stop_sequence")
	require.True(t, ok, "should have stop_sequence in operation log")
	assert.Equal(t, "STOP", val.Str())

	val, ok = bodyMap.Get("anthropic.response.thinking_length")
	require.True(t, ok, "should have thinking_length in operation log")
	assert.Equal(t, int64(len("Deep thought")), val.Int())
}

func TestEmitLogs_DetailedToolCallLogPattern(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.toolCalls = []ToolCallInfo{
		{
			ToolName:   "Glob",
			ToolCallID: "tc_glob",
			Pattern:    "**/*.go",
			FilePath:   "/src",
		},
	}

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	var foundPattern bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "anthropic.tool_call" {
			pattern, ok := lr.Attributes().Get("pattern")
			if ok {
				foundPattern = true
				assert.Equal(t, "**/*.go", pattern.Str())
			}
		}
	}
	assert.True(t, foundPattern, "should have pattern attribute in detailed tool call log")
}

// --- Phase 4: New span attribute tests ---

func TestEmitTraces_SpeedAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.speed = "fast"

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	rootSpan := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	val, ok := rootSpan.Attributes().Get("anthropic.usage.speed")
	require.True(t, ok)
	assert.Equal(t, "fast", val.Str())
}

func TestEmitTraces_ServerToolUseAttributes(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.Usage.ServerToolUse = &ServerToolUse{
		WebSearchRequests:     3,
		WebFetchRequests:      1,
		CodeExecutionRequests: 2,
	}

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	val, ok := attrs.Get("anthropic.usage.server_tool_use.web_search_requests")
	require.True(t, ok)
	assert.Equal(t, int64(3), val.Int())

	val, ok = attrs.Get("anthropic.usage.server_tool_use.web_fetch_requests")
	require.True(t, ok)
	assert.Equal(t, int64(1), val.Int())

	val, ok = attrs.Get("anthropic.usage.server_tool_use.code_execution_requests")
	require.True(t, ok)
	assert.Equal(t, int64(2), val.Int())
}

func TestEmitTraces_BetaFeaturesAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.betaFeatures = "output-128k-2025-02-19,token-counting-2025-02-10"

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("anthropic.request.beta_features")
	require.True(t, ok)
	assert.Equal(t, "output-128k-2025-02-19,token-counting-2025-02-10", val.Str())
}

func TestEmitTraces_OrganizationIDAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.organizationID = "org_abc123"

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("anthropic.organization_id")
	require.True(t, ok)
	assert.Equal(t, "org_abc123", val.Str())
}

func TestEmitTraces_RateLimitResetAttributes(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.rateLimit.RequestsReset = "2025-01-15T12:00:00Z"
	data.rateLimit.InputTokensReset = "2025-01-15T12:01:00Z"
	data.rateLimit.OutputTokensReset = "2025-01-15T12:02:00Z"
	data.rateLimit.TokensLimit = 200000
	data.rateLimit.TokensRemaining = 150000
	data.rateLimit.RetryAfter = "30"
	data.rateLimit.UnifiedStatus = "allowed"

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	val, ok := attrs.Get("anthropic.ratelimit.requests.reset")
	require.True(t, ok)
	assert.Equal(t, "2025-01-15T12:00:00Z", val.Str())

	val, ok = attrs.Get("anthropic.ratelimit.tokens.limit")
	require.True(t, ok)
	assert.Equal(t, int64(200000), val.Int())

	val, ok = attrs.Get("anthropic.ratelimit.retry_after")
	require.True(t, ok)
	assert.Equal(t, "30", val.Str())

	val, ok = attrs.Get("anthropic.ratelimit.unified_status")
	require.True(t, ok)
	assert.Equal(t, "allowed", val.Str())
}

func TestEmitTraces_CostMultiplierAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.cost.Multiplier = "fast"

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("anthropic.cost.multiplier")
	require.True(t, ok)
	assert.Equal(t, "fast", val.Str())
}

// --- Phase 4: New metric tests ---

func TestEmitMetrics_ServerToolUse(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.Usage.ServerToolUse = &ServerToolUse{
		WebSearchRequests:     5,
		WebFetchRequests:      2,
		CodeExecutionRequests: 1,
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.server_tool_use.web_search_requests"])
	assert.True(t, metricNames["anthropic.server_tool_use.web_fetch_requests"])
	assert.True(t, metricNames["anthropic.server_tool_use.code_execution_requests"])
	assert.True(t, metricNames["anthropic.cost.server_tool_use.web_search"])
}

func TestEmitMetrics_SpeedBreakdown(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.speed = "fast"

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.requests.by_speed"])
}

func TestEmitMetrics_Throughput(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.throughput.output_tokens_per_second"])
}

func TestEmitMetrics_CostMultipliedRequests(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.cost.Multiplier = "fast"

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["anthropic.cost.multiplied_requests"])
}

// --- Phase 4: New log tests ---

func TestEmitLogs_NotableStopReason_Refusal(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.StopReason = "refusal"

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	var foundNotableLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "anthropic.notable_stop_reason" {
			foundNotableLog = true
			assert.Contains(t, lr.Body().Str(), "Safety refusal")
			stopReason, _ := lr.Attributes().Get("stop_reason")
			assert.Equal(t, "refusal", stopReason.Str())
		}
	}
	assert.True(t, foundNotableLog, "should have notable stop reason log for refusal")
}

func TestEmitLogs_NotableStopReason_PauseTurn(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.StopReason = "pause_turn"

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	var foundNotableLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "anthropic.notable_stop_reason" {
			foundNotableLog = true
			assert.Contains(t, lr.Body().Str(), "Turn paused")
		}
	}
	assert.True(t, foundNotableLog, "should have notable stop reason log for pause_turn")
}

func TestEmitLogs_NotableStopReason_ContextExceeded(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.StopReason = "model_context_window_exceeded"

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	var foundNotableLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "anthropic.notable_stop_reason" {
			foundNotableLog = true
			assert.Contains(t, lr.Body().Str(), "Context window exceeded")
		}
	}
	assert.True(t, foundNotableLog, "should have notable stop reason log for context_window_exceeded")
}

func TestEmitLogs_NotableStopReason_NormalEndTurn(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.StopReason = "end_turn"

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	logRecords := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok {
			assert.NotEqual(t, "anthropic.notable_stop_reason", eventName.Str(), "end_turn should not produce a notable stop reason log")
		}
	}
}

func TestEmitLogs_OperationLog_NewFields(t *testing.T) {
	tb, _, _, logsSink := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.speed = "fast"
	data.betaFeatures = "output-128k-2025-02-19"
	data.organizationID = "org_abc123"
	data.cost.Multiplier = "fast"
	data.response.Usage.ServerToolUse = &ServerToolUse{
		WebSearchRequests: 3,
	}

	err := tb.emitLogs(context.Background(), data)
	require.NoError(t, err)

	logs := logsSink.AllLogs()
	require.Len(t, logs, 1)

	firstLog := logs[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	bodyMap := firstLog.Body().Map()

	val, ok := bodyMap.Get("anthropic.usage.speed")
	require.True(t, ok, "should have speed in operation log")
	assert.Equal(t, "fast", val.Str())

	val, ok = bodyMap.Get("anthropic.request.beta_features")
	require.True(t, ok, "should have beta_features in operation log")
	assert.Equal(t, "output-128k-2025-02-19", val.Str())

	val, ok = bodyMap.Get("anthropic.organization_id")
	require.True(t, ok, "should have organization_id in operation log")
	assert.Equal(t, "org_abc123", val.Str())

	val, ok = bodyMap.Get("anthropic.cost.multiplier")
	require.True(t, ok, "should have cost multiplier in operation log")
	assert.Equal(t, "fast", val.Str())

	stuMap, ok := bodyMap.Get("anthropic.usage.server_tool_use")
	require.True(t, ok, "should have server_tool_use in operation log")
	webSearch, ok := stuMap.Map().Get("web_search_requests")
	require.True(t, ok)
	assert.Equal(t, int64(3), webSearch.Int())
}
