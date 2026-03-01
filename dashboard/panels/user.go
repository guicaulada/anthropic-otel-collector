package panels

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// ---------------------------------------------------------------------------
// Row 1: Usage Overview
// ---------------------------------------------------------------------------

// UserTotalCost returns a stat panel showing total cost over the selected range.
func UserTotalCost() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Total Cost").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("currencyUSD").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_cost_total{%s}[$__range]))`),
				"Total Cost",
			),
		)
}

// UserActiveSessions returns a stat panel showing the number of active sessions.
func UserActiveSessions() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Active Sessions").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("short").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				fs(`count(count by (claude_code_session_id) (increase(`+MetricSessionRequests+`{%s}[$__range])))`),
				"Active Sessions",
			),
		)
}

// UserTotalRequests returns a stat panel showing total requests.
func UserTotalRequests() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Total Requests").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("short").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_requests_total{%s}[$__range]))`),
				"Total Requests",
			),
		)
}

// UserAvgCostPerSession returns a stat panel showing average cost per session.
func UserAvgCostPerSession() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Avg Cost / Session").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("currencyUSD").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				fs(`sum(increase(anthropic_cost_total{%s}[$__range])) / count(count by (claude_code_session_id) (increase(`+MetricSessionRequests+`{%s}[$__range])))`),
				"Avg Cost / Session",
			),
		)
}

// UserCacheSavings returns a stat panel showing total cache savings.
func UserCacheSavings() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Cache Savings").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("currencyUSD").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_cost_cache_savings_total{%s}[$__range]))`),
				"Cache Savings",
			),
		)
}

// UserTotalTokens returns a stat panel showing combined input and output tokens.
func UserTotalTokens() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Total Tokens").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("short").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tokens_input_total{%s}[$__range])) + sum(increase(anthropic_tokens_output_total{%s}[$__range]))`),
				"Total Tokens",
			),
		)
}

// UserErrorRate returns a stat panel showing the current error rate percentage.
func UserErrorRate() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Error Rate").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("percent").
		Thresholds(redGreenThresholds(5)).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`(sum(rate(anthropic_errors_total{%s}[$__rate_interval])) or vector(0)) / (sum(rate(anthropic_requests_total{%s}[$__rate_interval])) > 0) * 100`),
				"Error Rate",
			),
		)
}

// UserAvgLatency returns a stat panel showing average operation latency.
func UserAvgLatency() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Avg Latency").
		Datasource(datasourceRef()).
		Height(8).
		Span(6).
		Unit("s").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(rate(gen_ai_client_operation_duration_seconds_sum{%s}[$__rate_interval])) / sum(rate(gen_ai_client_operation_duration_seconds_count{%s}[$__rate_interval]))`),
				"Avg Latency",
			),
		)
}

// ---------------------------------------------------------------------------
// Row 2: Cost Intelligence
// ---------------------------------------------------------------------------

// UserCostOverTime returns a timeseries showing cost rate in $/hr.
func UserCostOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cost Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		Unit("currencyUSD").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_cost_total{%s}[$__rate_interval])) * 3600`),
				"Cost $/hr",
			),
		)
}

// UserCostByProject returns a piechart showing cost distribution by project.
func UserCostByProject() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Cost by Project").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("currencyUSD").
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true).
				Values([]piechart.PieChartLegendValues{piechart.PieChartLegendValuesValue, piechart.PieChartLegendValuesPercent}),
		).
		Tooltip(singleTooltip()).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				fmt.Sprintf(`sum by (claude_code_project_name) (increase(%s{%s}[$__range]))`, MetricProjectCost, projectFilter),
				"{{claude_code_project_name}}",
			),
		).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// UserCostByModel returns a piechart showing cost distribution by model.
func UserCostByModel() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Cost by Model").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("currencyUSD").
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true).
				Values([]piechart.PieChartLegendValues{piechart.PieChartLegendValuesValue, piechart.PieChartLegendValuesPercent}),
		).
		Tooltip(singleTooltip()).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum by (gen_ai_request_model) (increase(anthropic_cost_total{%s}[$__range]))`),
				"{{gen_ai_request_model}}",
			),
		).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// UserCacheSavingsOverTime returns a timeseries showing cache savings rate in $/hr.
func UserCacheSavingsOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cache Savings Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		Unit("currencyUSD").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_cost_cache_savings_total{%s}[$__rate_interval])) * 3600`),
				"Cache Savings $/hr",
			),
		)
}

// UserCumulativeCostByProject returns a stacked timeseries showing cost rate by project per 5-minute window.
func UserCumulativeCostByProject() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cost Rate by Project (5m)").
		Description("Cost per 5-minute window by project, resilient to counter resets").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		Unit("currencyUSD").
		FillOpacity(30).
		Stacking(
			common.NewStackingConfigBuilder().
				Mode(common.StackingModeNormal),
		).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fmt.Sprintf(`sum by (claude_code_project_name) (increase(%s{%s}[5m]))`, MetricProjectCost, projectFilter),
				"{{claude_code_project_name}}",
			),
		)
}

// ---------------------------------------------------------------------------
// Row 3: Session Activity
// ---------------------------------------------------------------------------

// UserSessionsOverTime returns a timeseries showing active session count over time.
func UserSessionsOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Sessions Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fs(`count(count by (claude_code_session_id) (rate(`+MetricSessionRequests+`{%s}[$__rate_interval])))`),
				"Active Sessions",
			),
		)
}

// UserConversationDepthOverTime returns a timeseries showing conversation turns rate.
func UserConversationDepthOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Conversation Depth Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fs(`sum(rate(claude_code_session_conversation_turns_total{%s}[$__rate_interval]))`),
				"Conversation Turns/s",
			),
		)
}

// UserSessionDuration returns a timeseries showing active duration per session.
func UserSessionDuration() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Session Duration").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fs(`sum by (claude_code_session_id) (`+MetricSessionActiveDuration+`{%s})`),
				"{{claude_code_session_id}}",
			),
		)
}

// UserSessionCostDistribution returns a piechart showing cost distribution across sessions.
func UserSessionCostDistribution() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Session Cost Distribution").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("currencyUSD").
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true).
				Values([]piechart.PieChartLegendValues{piechart.PieChartLegendValuesValue, piechart.PieChartLegendValuesPercent}),
		).
		Tooltip(singleTooltip()).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				fs(`sum by (claude_code_session_id) (increase(`+MetricSessionCost+`{%s}[$__range]))`),
				"{{claude_code_session_id}}",
			),
		).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// UserAvgRequestsPerSession returns a timeseries showing average requests per session.
func UserAvgRequestsPerSession() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Requests / Session").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fs(`sum(rate(`+MetricSessionRequests+`{%s}[$__rate_interval])) / count(count by (claude_code_session_id) (rate(`+MetricSessionRequests+`{%s}[$__rate_interval])))`),
				"Avg Requests/Session",
			),
		)
}

// ---------------------------------------------------------------------------
// Row 4: Productivity
// ---------------------------------------------------------------------------

// UserToolCallDistribution returns a piechart showing distribution of tool calls by tool name.
func UserToolCallDistribution() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Tool Call Distribution").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true).
				Values([]piechart.PieChartLegendValues{piechart.PieChartLegendValuesPercent, piechart.PieChartLegendValuesValue}),
		).
		Tooltip(singleTooltip()).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				`sum by (tool_name) (increase(anthropic_tool_use_calls_total{`+filter+`}[$__range]))`,
				"{{tool_name}}",
			),
		).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// UserLinesChangedOverTime returns a timeseries showing lines added and removed over time.
func UserLinesChangedOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Lines Changed Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tool_use_lines_added_total{%s}[$__rate_interval]))`),
				"Lines Added",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tool_use_lines_removed_total{%s}[$__rate_interval]))`),
				"Lines Removed",
			),
		).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		Overrides([]cog.Builder[dashboard.DashboardFieldConfigSourceOverrides]{
			dashboard.NewDashboardFieldConfigSourceOverridesBuilder().
				Matcher(dashboard.MatcherConfig{
					Id:      "byName",
					Options: "Lines Added",
				}).
				Properties([]dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}),
			dashboard.NewDashboardFieldConfigSourceOverridesBuilder().
				Matcher(dashboard.MatcherConfig{
					Id:      "byName",
					Options: "Lines Removed",
				}).
				Properties([]dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		})
}

// UserFileOperations returns a bar gauge showing file edit, create, and read counts.
func UserFileOperations() cog.Builder[dashboard.Panel] {
	return bargauge.NewPanelBuilder().
		Title("File Operations").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promInstantQuery(
				`sum(increase(anthropic_tool_use_file_edits_total{`+filter+`}[$__range]))`,
				"Edits",
			),
		).
		WithTarget(
			promInstantQuery(
				`sum(increase(anthropic_tool_use_file_creates_total{`+filter+`}[$__range]))`,
				"Creates",
			),
		).
		WithTarget(
			promInstantQuery(
				`sum(increase(anthropic_tool_use_file_reads_total{`+filter+`}[$__range]))`,
				"Reads",
			),
		).
		Orientation(common.VizOrientationHorizontal).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// UserFilesTouched returns a stat panel showing total unique files touched.
func UserFilesTouched() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Files Touched").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("short").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tool_use_files_touched_total{%s}[$__range]))`),
				"Files Touched",
			),
		)
}

// UserToolCallsBySession returns a stacked timeseries showing tool calls per session.
func UserToolCallsBySession() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Tool Calls by Session").
		Datasource(datasourceRef()).
		Height(8).
		Span(16).
		FillOpacity(30).
		Stacking(
			common.NewStackingConfigBuilder().
				Mode(common.StackingModeNormal),
		).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fs(`sum by (claude_code_session_id) (rate(claude_code_session_tool_calls_total{%s}[$__rate_interval]))`),
				"{{claude_code_session_id}}",
			),
		)
}

// ---------------------------------------------------------------------------
// Row 5: Efficiency & Performance
// ---------------------------------------------------------------------------

// UserCacheHitRatio returns a timeseries showing cache hit ratio over time.
func UserCacheHitRatio() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cache Hit Ratio").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("percentunit").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_cache_hit_ratio{%s})`),
				"Cache Hit Ratio",
			),
		)
}

// UserOutputUtilization returns a timeseries showing output token utilization over time.
func UserOutputUtilization() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Output Utilization").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("percentunit").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_tokens_output_utilization_ratio{%s})`),
				"Output Utilization",
			),
		)
}

// UserAvgTTFT returns a timeseries showing average time to first token.
func UserAvgTTFT() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Time to First Token").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(gen_ai_server_time_to_first_token_seconds_sum{%s}[$__rate_interval])) / sum(rate(gen_ai_server_time_to_first_token_seconds_count{%s}[$__rate_interval]))`),
				"Avg TTFT",
			),
		)
}

// UserOutputThroughput returns a timeseries showing output token throughput.
func UserOutputThroughput() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Output Throughput").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("suffix: tok/s").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_throughput_output_tokens_per_second_per_second{%s})`),
				"Output tok/s",
			),
		)
}

// UserInputVsOutput returns a timeseries showing input and output token rates.
func UserInputVsOutput() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Input vs Output Tokens").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_input_total{%s}[$__rate_interval]))`),
				"Input Tokens/s",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_output_total{%s}[$__rate_interval]))`),
				"Output Tokens/s",
			),
		)
}

// UserCostPerOutputToken returns a timeseries showing cost per output token over time.
func UserCostPerOutputToken() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cost / Output Token").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("currencyUSD").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_cost_total{%s}[$__rate_interval])) / sum(rate(anthropic_tokens_output_total{%s}[$__rate_interval]))`),
				"Cost / Output Token",
			),
		)
}

// ---------------------------------------------------------------------------
// Row 6: Errors & Rate Limits
// ---------------------------------------------------------------------------

// UserErrorRateOverTime returns a timeseries showing error rate percentage over time.
func UserErrorRateOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Error Rate Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("percent").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`(sum(rate(anthropic_errors_total{%s}[$__rate_interval])) or vector(0)) / (sum(rate(anthropic_requests_total{%s}[$__rate_interval])) > 0) * 100`),
				"Error Rate %",
			),
		)
}

// UserErrorsByType returns a stacked timeseries showing errors broken down by type.
func UserErrorsByType() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Errors by Type").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		FillOpacity(30).
		Stacking(
			common.NewStackingConfigBuilder().
				Mode(common.StackingModeNormal),
		).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum by (error_type) (rate(anthropic_errors_by_type_total{%s}[$__rate_interval])) or vector(0)`),
				"{{error_type}}",
			),
		)
}

// UserRateLimitUtilization returns a timeseries showing rate limit utilization for all dimensions.
func UserRateLimitUtilization() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Rate Limit Utilization").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("percentunit").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_ratelimit_requests_utilization_ratio{%s})`),
				"Requests",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_ratelimit_input_tokens_utilization_ratio{%s})`),
				"Input Tokens",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_ratelimit_output_tokens_utilization_ratio{%s})`),
				"Output Tokens",
			),
		)
}

// UserRateLimitRemaining returns a timeseries showing remaining rate limits.
func UserRateLimitRemaining() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Rate Limit Remaining").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_ratelimit_requests_remaining{%s})`),
				"Requests Remaining",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_ratelimit_input_tokens_remaining{%s})`),
				"Input Tokens Remaining",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_ratelimit_output_tokens_remaining{%s})`),
				"Output Tokens Remaining",
			),
		)
}
