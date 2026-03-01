package anthropicreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.uber.org/zap"
)

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
	assert.GreaterOrEqual(t, logRecords.Len(), 2)

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

	var foundErrorLog bool
	for i := 0; i < logRecords.Len(); i++ {
		lr := logRecords.At(i)
		eventName, ok := lr.Attributes().Get("event.name")
		if ok && eventName.Str() == "gen_ai.error" {
			foundErrorLog = true
			assert.Equal(t, "API error", lr.Body().Str())
			errType, _ := lr.Attributes().Get("error.type")
			assert.Equal(t, "authentication_error", errType.Str())
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
		{ToolName: "Glob", ToolCallID: "tc_glob", Pattern: "**/*.go", FilePath: "/src"},
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
			assert.Equal(t, "Notable stop reason", lr.Body().Str())
			msg, _ := lr.Attributes().Get("message")
			assert.Contains(t, msg.Str(), "Safety refusal")
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
			msg, _ := lr.Attributes().Get("message")
			assert.Contains(t, msg.Str(), "Turn paused")
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
			msg, _ := lr.Attributes().Get("message")
			assert.Contains(t, msg.Str(), "Context window exceeded")
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
