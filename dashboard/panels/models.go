package panels

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
)

// Prometheus metric names for model and request analysis.
const (
	MetricRequests              = "anthropic_requests_total"
	MetricStopReason            = "anthropic_stop_reason_total"
	MetricContentBlocks         = "anthropic_content_blocks_total"
	MetricRequestsBySpeed       = "anthropic_requests_by_speed_total"
	MetricMessagesCount         = "anthropic_request_messages_count"
	MetricSystemPromptSize      = "anthropic_request_system_prompt_size"
	MetricThinkingEnabled       = "anthropic_thinking_enabled_total"
	MetricServerToolWebSearch   = "anthropic_server_tool_use_web_search_requests_total"
	MetricServerToolWebFetch    = "anthropic_server_tool_use_web_fetch_requests_total"
	MetricServerToolCodeExecution = "anthropic_server_tool_use_code_execution_requests_total"
)

// RequestsByModel returns a donut piechart showing requests broken down by model.
func RequestsByModel() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Requests by Model").
		Description("Distribution of requests by model over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum by (gen_ai_request_model) (increase(%s{%s}[$__range]))`, MetricRequests, filter),
			"{{gen_ai_request_model}}",
		)).
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true),
		).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// StopReasons returns a donut piechart showing the distribution of stop reasons.
func StopReasons() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Stop Reasons").
		Description("Distribution of stop reasons over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum by (stop_reason) (increase(%s{%s}[$__range]))`, MetricStopReason, filter),
			"{{stop_reason}}",
		)).
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true),
		).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// ContentBlockTypes returns a horizontal bar gauge showing content block types.
func ContentBlockTypes() cog.Builder[dashboard.Panel] {
	return bargauge.NewPanelBuilder().
		Title("Content Block Types").
		Description("Distribution of content block types over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum by (type) (increase(%s{%s}[$__range]))`, MetricContentBlocks, filter),
			"{{type}}",
		)).
		Orientation(common.VizOrientationHorizontal).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// RequestsBySpeed returns a donut piechart showing requests broken down by speed tier.
func RequestsBySpeed() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Requests by Speed").
		Description("Distribution of requests by speed tier (standard/fast). Shows data only when speed metadata is present in requests.").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum by (speed) (increase(%s{%s}[$__range]))`, MetricRequestsBySpeed, filter),
			"{{speed}}",
		)).
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true),
		).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// AvgMessagesPerRequest returns a stat panel showing average messages per request.
func AvgMessagesPerRequest() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Avg Messages per Request").
		Description("Average number of messages per request").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(rate(%s_sum{%s}[$__rate_interval])) / sum(rate(%s_count{%s}[$__rate_interval]))`, MetricMessagesCount, filter, MetricMessagesCount, filter),
			"",
		)).
		Unit("short").
		Thresholds(greenThresholds()).
		ColorScheme(fixedColor("blue"))
}

// AvgSystemPromptSize returns a stat panel showing average system prompt size.
func AvgSystemPromptSize() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Avg System Prompt Size").
		Description("Average system prompt size in bytes").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(rate(%s_sum{%s}[$__rate_interval])) / sum(rate(%s_count{%s}[$__rate_interval]))`, MetricSystemPromptSize, filter, MetricSystemPromptSize, filter),
			"",
		)).
		Unit("decbytes").
		Thresholds(greenThresholds()).
		ColorScheme(fixedColor("blue"))
}

// ThinkingEnabledRequests returns a stat panel showing thinking-enabled request count.
func ThinkingEnabledRequests() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Thinking Enabled Requests").
		Description("Total requests with thinking enabled over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, MetricThinkingEnabled, filter),
			"",
		)).
		Unit("short").
		Thresholds(greenThresholds()).
		ColorScheme(fixedColor("blue"))
}

// ServerToolUse returns a horizontal bar gauge showing server-side tool usage counts.
func ServerToolUse() cog.Builder[dashboard.Panel] {
	return bargauge.NewPanelBuilder().
		Title("Server Tool Use").
		Description("Server-side tool usage over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, MetricServerToolWebSearch, filter),
			"Web Search",
		)).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, MetricServerToolWebFetch, filter),
			"Web Fetch",
		)).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, MetricServerToolCodeExecution, filter),
			"Code Execution",
		)).
		Orientation(common.VizOrientationHorizontal).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}
