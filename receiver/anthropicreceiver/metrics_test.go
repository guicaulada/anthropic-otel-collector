package anthropicreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

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

	allMetrics := sm.At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

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

func TestEmitMetrics_HistogramBuckets(t *testing.T) {
	// Verify that histogram metrics have explicit bucket boundaries
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	// Find gen_ai.client.operation.duration histogram and check it has buckets
	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "gen_ai.client.operation.duration" {
			require.Equal(t, pmetric.MetricTypeHistogram, m.Type())
			dp := m.Histogram().DataPoints().At(0)
			assert.Greater(t, dp.ExplicitBounds().Len(), 0, "should have explicit bucket boundaries")
			assert.Equal(t, dp.ExplicitBounds().Len()+1, dp.BucketCounts().Len(), "bucket counts should be bounds+1")
			return
		}
	}
	t.Fatal("gen_ai.client.operation.duration metric not found")
}

func TestEmitMetrics_ActiveRequests(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.activeRequests = 5

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}
	assert.True(t, metricNames["anthropic.requests.active"])
}

func TestEmitMetrics_SessionMetrics(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.session = &SessionContext{
		SessionID:     "ses_test123",
		ProjectPath:   "/home/user/my-project",
		ProjectName:   "my-project",
		RequestNumber: 1,
		IsNewSession:  true,
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

	assert.True(t, metricNames["claude_code.session.requests"], "should have session requests metric")
	assert.True(t, metricNames["claude_code.session.active_duration"], "should have session active duration metric")
	assert.True(t, metricNames["claude_code.session.cost"], "should have session cost metric")
	assert.True(t, metricNames["claude_code.session.tokens.input"], "should have session input tokens metric")
	assert.True(t, metricNames["claude_code.session.tokens.output"], "should have session output tokens metric")
	assert.True(t, metricNames["claude_code.project.requests"], "should have project requests metric")
	assert.True(t, metricNames["claude_code.project.cost"], "should have project cost metric")
}

func TestEmitMetrics_NoSessionMetrics(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	// No session set

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.False(t, metricNames["claude_code.session.requests"], "should not have session metrics without session")
	assert.False(t, metricNames["claude_code.project.requests"], "should not have project metrics without session")
}

func TestEmitMetrics_ProjectNameInCommonAttrs(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.session = &SessionContext{
		SessionID:   "ses_test",
		ProjectName: "my-project",
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	require.Len(t, metrics, 1)

	// Check that the first metric (gen_ai.client.operation.duration) has project name
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "gen_ai.client.operation.duration" {
			dp := m.Histogram().DataPoints().At(0)
			val, ok := dp.Attributes().Get("claude_code.project.name")
			require.True(t, ok, "should have project name in common attrs")
			assert.Equal(t, "my-project", val.Str())
			return
		}
	}
	t.Fatal("gen_ai.client.operation.duration metric not found")
}

func TestEmitMetrics_DuplicateToolCallsRemoved(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.response.Content = append(data.response.Content, ContentBlock{
		Type: "tool_use",
		Name: "Edit",
		ID:   "tc_1",
	})

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	for i := 0; i < allMetrics.Len(); i++ {
		assert.NotEqual(t, "anthropic.tool_calls", allMetrics.At(i).Name(), "anthropic.tool_calls should be removed (duplicate of anthropic.tool_use.calls)")
	}
}

func TestEmitMetrics_OutputTokensRateLimitGuarded(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.rateLimit.OutputTokensLimit = 0 // API never returns these

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	for i := 0; i < allMetrics.Len(); i++ {
		assert.NotEqual(t, "anthropic.ratelimit.output_tokens.limit", allMetrics.At(i).Name(), "should not emit output_tokens.limit when limit is 0")
	}
}

func TestEmitMetrics_ConversationTurns(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.request.Messages = []Message{
		{Role: "user", Content: []byte(`"Hello"`)},
		{Role: "assistant", Content: []byte(`"Hi"`)},
		{Role: "user", Content: []byte(`"How are you?"`)},
		{Role: "assistant", Content: []byte(`"Good"`)},
		{Role: "user", Content: []byte(`"Do something"`)},
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}
	assert.True(t, metricNames["anthropic.request.conversation_turns"])
}

func TestEmitMetrics_CacheSavings(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.cost.CacheSavings = 0.05

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	var found bool
	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "anthropic.cost.cache_savings" {
			found = true
			require.Equal(t, pmetric.MetricTypeSum, m.Type())
			dp := m.Sum().DataPoints().At(0)
			assert.InDelta(t, 0.05, dp.DoubleValue(), 0.0000001)
		}
	}
	assert.True(t, found, "should emit anthropic.cost.cache_savings metric")
}

func TestEmitMetrics_CacheSavings_NotEmittedWhenZero(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.cost.CacheSavings = 0

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	for i := 0; i < allMetrics.Len(); i++ {
		assert.NotEqual(t, "anthropic.cost.cache_savings", allMetrics.At(i).Name(), "should not emit cache savings when zero")
	}
}

func TestEmitMetrics_OutputUtilization(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	// MaxTokens=1024, OutputTokens=20 → utilization = 20/1024 ≈ 0.01953125
	data.request.MaxTokens = 1024
	data.response.Usage.OutputTokens = 20

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	var found bool
	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "anthropic.tokens.output_utilization" {
			found = true
			require.Equal(t, pmetric.MetricTypeGauge, m.Type())
			dp := m.Gauge().DataPoints().At(0)
			expected := 20.0 / 1024.0
			assert.InDelta(t, expected, dp.DoubleValue(), 0.0000001)
		}
	}
	assert.True(t, found, "should emit anthropic.tokens.output_utilization gauge")
}

func TestEmitMetrics_SessionEnrichmentMetrics(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.session = &SessionContext{
		SessionID:   "ses_enrich",
		ProjectName: "my-project",
	}
	data.response.Usage.CacheReadInputTokens = 500
	data.request.Messages = []Message{
		{Role: "user", Content: []byte(`"Hello"`)},
		{Role: "assistant", Content: []byte(`"Hi"`)},
		{Role: "user", Content: []byte(`"Do something"`)},
	}
	data.toolCalls = []ToolCallInfo{
		{ToolName: "Edit", LinesAdded: 10, LinesRemoved: 3},
		{ToolName: "Read"},
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["claude_code.session.tokens.cache_read"], "should have session cache read tokens")
	assert.True(t, metricNames["claude_code.session.conversation_turns"], "should have session conversation turns")
	assert.True(t, metricNames["claude_code.session.tool_calls"], "should have session tool calls")
	assert.True(t, metricNames["claude_code.session.lines_changed"], "should have session lines changed")

	// Verify cache read value
	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "claude_code.session.tokens.cache_read" {
			dp := m.Sum().DataPoints().At(0)
			assert.Equal(t, int64(500), dp.IntValue())
		}
		if m.Name() == "claude_code.session.tool_calls" {
			dp := m.Sum().DataPoints().At(0)
			assert.Equal(t, int64(2), dp.IntValue())
		}
		if m.Name() == "claude_code.session.lines_changed" {
			dp := m.Sum().DataPoints().At(0)
			assert.Equal(t, int64(13), dp.IntValue()) // 10+3
		}
	}
}

func TestEmitMetrics_SessionErrors(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.session = &SessionContext{
		SessionID:   "ses_err",
		ProjectName: "my-project",
	}
	data.statusCode = 500
	data.response = nil
	data.cost = CostResult{}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["claude_code.session.errors"], "should have session errors metric")
}

func TestEmitMetrics_ProjectEnrichmentMetrics(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.session = &SessionContext{
		SessionID:   "ses_proj",
		ProjectName: "my-project",
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["claude_code.project.tokens.input"], "should have project input tokens")
	assert.True(t, metricNames["claude_code.project.tokens.output"], "should have project output tokens")
}

func TestEmitMetrics_ProjectErrors(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.session = &SessionContext{
		SessionID:   "ses_proj_err",
		ProjectName: "my-project",
	}
	data.statusCode = 429
	data.response = nil
	data.cost = CostResult{}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	metricNames := make(map[string]bool)
	for i := 0; i < allMetrics.Len(); i++ {
		metricNames[allMetrics.At(i).Name()] = true
	}

	assert.True(t, metricNames["claude_code.project.errors"], "should have project errors metric")
}

func TestEmitMetrics_ErrorsByType(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 429
	data.response = nil
	data.cost = CostResult{}
	data.errorResponse = &AnthropicError{
		Error: ErrorDetail{
			Type:    "rate_limit_error",
			Message: "Rate limited",
		},
	}

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	var found bool
	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "anthropic.errors.by_type" {
			found = true
			dp := m.Sum().DataPoints().At(0)
			errType, ok := dp.Attributes().Get("error.type")
			require.True(t, ok, "should have error.type attribute")
			assert.Equal(t, "rate_limit_error", errType.Str())
		}
	}
	assert.True(t, found, "should emit anthropic.errors.by_type metric")
}

func TestEmitMetrics_ErrorsByType_NoErrorResponse(t *testing.T) {
	tb, _, metricsSink, _ := newTestTelemetryBuilder(t)
	data := newTestRequestData()
	data.statusCode = 500
	data.response = nil
	data.cost = CostResult{}
	data.errorResponse = nil

	err := tb.emitMetrics(context.Background(), data)
	require.NoError(t, err)

	metrics := metricsSink.AllMetrics()
	allMetrics := metrics[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()

	for i := 0; i < allMetrics.Len(); i++ {
		m := allMetrics.At(i)
		if m.Name() == "anthropic.errors.by_type" {
			dp := m.Sum().DataPoints().At(0)
			errType, ok := dp.Attributes().Get("error.type")
			require.True(t, ok, "should have error.type attribute")
			assert.Equal(t, "http_500", errType.Str())
			return
		}
	}
	t.Fatal("anthropic.errors.by_type metric not found")
}
