package panels

import (
	"fmt"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Prometheus metric names for session and project tracking.
const (
	MetricSessionRequests       = "claude_code_session_requests_total"
	MetricSessionActiveDuration = "claude_code_session_active_duration_seconds_total"
	MetricSessionCost           = "claude_code_session_cost_total"
	MetricSessionTokensInput    = "claude_code_session_tokens_input_total"
	MetricSessionTokensOutput   = "claude_code_session_tokens_output_total"
	MetricProjectRequests       = "claude_code_project_requests_total"
	MetricProjectCost           = "claude_code_project_cost_total"
)

// sessionFilter includes both the common filter and the project variable.
const sessionFilter = filter + `, claude_code_project_name=~"$project"`

// projectFilter filters by project variable only (for project-level metrics).
const projectFilter = `claude_code_project_name=~"$project"`

// fs replaces all %s placeholders with the session filter (common + project).
func fs(expr string) string {
	return strings.ReplaceAll(expr, "%s", sessionFilter)
}

// ActiveSessions returns a stat panel showing the number of active sessions.
func ActiveSessions() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Active Sessions").
		Description("Number of distinct sessions seen in the selected time range").
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

// SessionCostBreakdown returns a piechart showing cost breakdown by session.
func SessionCostBreakdown() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Session Cost Breakdown").
		Description("Cost distribution across sessions over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(9).
		Unit("currencyUSD").
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true),
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

// ProjectCostBreakdown returns a piechart showing cost breakdown by project.
func ProjectCostBreakdown() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Project Cost Breakdown").
		Description("Cost distribution across projects over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(9).
		Unit("currencyUSD").
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true),
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

// SessionRequestsOverTime returns a stacked timeseries showing requests per session.
func SessionRequestsOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Session Requests Over Time").
		Description("Request rate broken down by session").
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
				fs(`sum by (claude_code_session_id) (rate(`+MetricSessionRequests+`{%s}[$__rate_interval]))`),
				"{{claude_code_session_id}}",
			),
		)
}

// SessionActiveDuration returns a timeseries showing active duration per session.
func SessionActiveDuration() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Session Active Duration").
		Description("Cumulative active working time per session").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
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

// ProjectRequestsOverTime returns a stacked timeseries showing requests per project.
func ProjectRequestsOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Project Requests Over Time").
		Description("Request rate broken down by project").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		FillOpacity(30).
		Stacking(
			common.NewStackingConfigBuilder().
				Mode(common.StackingModeNormal),
		).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				fmt.Sprintf(`sum by (claude_code_project_name) (rate(%s{%s}[$__rate_interval]))`, MetricProjectRequests, projectFilter),
				"{{claude_code_project_name}}",
			),
		)
}

// SessionToolCalls returns a stacked timeseries showing tool calls per session.
func SessionToolCalls() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Session Tool Calls").
		Description("Tool call rate broken down by session").
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
				fs(`sum by (claude_code_session_id) (rate(claude_code_session_tool_calls_total{%s}[$__rate_interval]))`),
				"{{claude_code_session_id}}",
			),
		)
}

// SessionLinesChanged returns a stacked timeseries showing lines changed per session.
func SessionLinesChanged() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Session Lines Changed").
		Description("Lines changed rate broken down by session").
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
				fs(`sum by (claude_code_session_id) (rate(claude_code_session_lines_changed_total{%s}[$__rate_interval]))`),
				"{{claude_code_session_id}}",
			),
		)
}
