package anthropicreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	val, ok := attrs.Get("anthropic.request.stop_sequences_count")
	require.True(t, ok, "should have stop_sequences_count")
	assert.Equal(t, int64(2), val.Int())

	val, ok = attrs.Get("anthropic.request.user_messages_count")
	require.True(t, ok, "should have user_messages_count")
	assert.Equal(t, int64(2), val.Int())

	val, ok = attrs.Get("anthropic.request.assistant_messages_count")
	require.True(t, ok, "should have assistant_messages_count")
	assert.Equal(t, int64(1), val.Int())

	val, ok = attrs.Get("gen_ai.response.stop_sequence")
	require.True(t, ok, "should have stop_sequence")
	assert.Equal(t, "END", val.Str())

	val, ok = attrs.Get("anthropic.response.thinking_length")
	require.True(t, ok, "should have thinking_length")
	assert.Equal(t, int64(len("Let me think about this carefully")), val.Int())
}

func TestEmitTraces_GlobGrepEvents(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.toolCalls = []ToolCallInfo{
		{ToolName: "Glob", FilePath: "/src", Pattern: "**/*.go"},
		{ToolName: "Grep", FilePath: "/src", Pattern: "func main"},
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

func TestEmitTraces_StatusCodeUnsetForSuccess(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	rootSpan := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, int32(0), int32(rootSpan.Status().Code()), "successful span should have StatusCodeUnset (0)")
}

func TestEmitTraces_ErrorTypeAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 429
	data.response = nil
	data.errorResponse = &AnthropicError{
		Type: "error",
		Error: ErrorDetail{
			Type:    "rate_limit_error",
			Message: "Rate limited",
		},
	}

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("error.type")
	require.True(t, ok, "should have error.type attribute")
	assert.Equal(t, "rate_limit_error", val.Str())
}

func TestEmitTraces_ErrorTypeAttributeFallback(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 500
	data.response = nil

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("error.type")
	require.True(t, ok, "should have error.type attribute")
	assert.Equal(t, "http_500", val.Str())
}

func TestEmitTraces_ToolChoiceAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.request.ToolChoice = []byte(`{"type":"auto"}`)

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("anthropic.request.tool_choice")
	require.True(t, ok, "should have tool_choice attribute")
	assert.Equal(t, "auto", val.Str())
}

func TestEmitTraces_ApiVersionAttribute(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.apiVersion = "2024-01-01"

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("anthropic.request.api_version")
	require.True(t, ok, "should have api_version attribute")
	assert.Equal(t, "2024-01-01", val.Str())
}

func TestEmitTraces_ServerPortIsInt(t *testing.T) {
	tb, tracesSink, _, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitTraces(context.Background(), data)
	require.NoError(t, err)

	traces := tracesSink.AllTraces()
	require.Len(t, traces, 1)

	attrs := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	val, ok := attrs.Get("server.port")
	require.True(t, ok, "should have server.port attribute")
	assert.Equal(t, int64(443), val.Int(), "server.port should be int, not string")
}
