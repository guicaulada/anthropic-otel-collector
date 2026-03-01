package panels

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// ToolCallDistribution returns a donut piechart showing the distribution of tool calls.
func ToolCallDistribution() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Tool Call Distribution").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promRangeQuery(
				`sum by (tool_name) (increase(anthropic_tool_use_calls_total{`+filter+`}[$__range]))`,
				"{{tool_name}}",
			),
		).
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true).
				Values([]piechart.PieChartLegendValues{piechart.PieChartLegendValuesPercent, piechart.PieChartLegendValuesValue}),
		).
		Tooltip(singleTooltip()).
		ColorScheme(paletteColor())
}

// ToolCallsOverTime returns a stacked timeseries of tool calls over time.
func ToolCallsOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Tool Calls Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(16).
		WithTarget(
			promRangeQuery(
				`sum by (tool_name) (rate(anthropic_tool_use_calls_total{`+filter+`}[$__rate_interval]))`,
				"{{tool_name}}",
			),
		).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		FillOpacity(30).
		Stacking(
			common.NewStackingConfigBuilder().
				Mode(common.StackingModeNormal),
		).
		ColorScheme(paletteColor())
}

// FileOperations returns a horizontal bar gauge showing file edit, create, and read counts.
func FileOperations() cog.Builder[dashboard.Panel] {
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

// LinesChangedOverTime returns a timeseries of lines added and removed over time.
func LinesChangedOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Lines Changed Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(16).
		WithTarget(
			promRangeQuery(
				`sum(rate(anthropic_tool_use_lines_added_total{`+filter+`}[$__rate_interval]))`,
				"Lines Added",
			),
		).
		WithTarget(
			promRangeQuery(
				`sum(rate(anthropic_tool_use_lines_removed_total{`+filter+`}[$__rate_interval]))`,
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

// FileTypesWorkedOn returns a bar gauge showing top file extensions by operation count.
func FileTypesWorkedOn() cog.Builder[dashboard.Panel] {
	return bargauge.NewPanelBuilder().
		Title("File Types Worked On").
		Description("Top 10 file extensions by number of operations over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		WithTarget(
			promInstantQuery(
				`topk(10, sum by (file_extension) (increase(anthropic_tool_use_file_type_total{`+filter+`}[$__range])))`,
				"{{file_extension}}",
			),
		).
		Orientation(common.VizOrientationHorizontal).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// BashCommands returns a stat panel showing total bash command executions.
func BashCommands() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Bash Commands").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
		WithTarget(
			promInstantQuery(
				`sum(increase(anthropic_tool_use_bash_commands_total{`+filter+`}[$__range]))`,
				"",
			),
		).
		Unit("short").
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// SearchOperations returns a stat panel showing total grep and glob search operations.
func SearchOperations() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Search Operations").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
		WithTarget(
			promInstantQuery(
				`sum(increase(anthropic_tool_use_grep_searches_total{`+filter+`}[$__range])) + sum(increase(anthropic_tool_use_glob_searches_total{`+filter+`}[$__range]))`,
				"",
			),
		).
		Unit("short").
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// FilesTouched returns a stat panel showing total unique files touched.
func FilesTouched() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Files Touched").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
		WithTarget(
			promInstantQuery(
				`sum(increase(anthropic_tool_use_files_touched_total{`+filter+`}[$__range]))`,
				"",
			),
		).
		Unit("short").
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}
