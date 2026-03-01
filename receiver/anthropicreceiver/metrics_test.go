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
