package panels

import (
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
)

// filter is the common PromQL label filter applied to all queries.
const filter = `gen_ai_request_model=~"$model", anthropic_api_key_hash=~"$api_key_hash"`

// f replaces all %s placeholders in a PromQL expression with the standard filter.
func f(expr string) string {
	return strings.ReplaceAll(expr, "%s", filter)
}

// datasourceRef returns the variable datasource reference used by all panels.
func datasourceRef() dashboard.DataSourceRef {
	return dashboard.DataSourceRef{
		Uid:  strPtr("$datasource"),
		Type: strPtr("prometheus"),
	}
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(v float64) *float64 {
	return &v
}

// promQuery creates a Prometheus query builder with legend format.
func promQuery(expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().
		Expr(expr).
		LegendFormat(legend)
}

// promRangeQuery creates a Prometheus range query builder.
func promRangeQuery(expr, legend string) *prometheus.DataqueryBuilder {
	return promQuery(expr, legend).Range()
}

// instantQuery creates a Prometheus instant query builder.
func instantQuery(expr, legend string) *prometheus.DataqueryBuilder {
	return promQuery(expr, legend).Instant()
}

// promInstantQuery is an alias for instantQuery.
func promInstantQuery(expr, legend string) *prometheus.DataqueryBuilder {
	return instantQuery(expr, legend)
}

// defaultLegend returns a standard bottom legend for timeseries panels.
func defaultLegend() *common.VizLegendOptionsBuilder {
	return common.NewVizLegendOptionsBuilder().
		DisplayMode(common.LegendDisplayModeList).
		Placement(common.LegendPlacementBottom).
		ShowLegend(true)
}

// tableLegend returns a legend displayed as a table.
func tableLegend() *common.VizLegendOptionsBuilder {
	return common.NewVizLegendOptionsBuilder().
		DisplayMode(common.LegendDisplayModeTable).
		Placement(common.LegendPlacementBottom).
		ShowLegend(true)
}

// rightTableLegend returns a table-format legend placed on the right side.
// This is the standard Grafana pattern for panels with many series.
func rightTableLegend() *common.VizLegendOptionsBuilder {
	return common.NewVizLegendOptionsBuilder().
		DisplayMode(common.LegendDisplayModeTable).
		Placement(common.LegendPlacementRight).
		ShowLegend(true)
}

// hiddenLegend returns a hidden legend.
func hiddenLegend() *common.VizLegendOptionsBuilder {
	return common.NewVizLegendOptionsBuilder().
		ShowLegend(false)
}

// singleTooltip returns a single-series tooltip.
func singleTooltip() *common.VizTooltipOptionsBuilder {
	return common.NewVizTooltipOptionsBuilder().
		Mode(common.TooltipDisplayModeSingle).
		Sort(common.SortOrderNone)
}

// multiTooltip returns a multi-series tooltip sorted descending.
func multiTooltip() *common.VizTooltipOptionsBuilder {
	return common.NewVizTooltipOptionsBuilder().
		Mode(common.TooltipDisplayModeMulti).
		Sort(common.SortOrderDescending)
}

// greenThresholds returns thresholds with a single green step (base).
func greenThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green"},
		})
}

// redGreenThresholds returns thresholds that go from green to red at the given value.
func redGreenThresholds(redValue float64) *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green"},
			{Value: float64Ptr(redValue), Color: "red"},
		})
}

// utilizationThresholds returns green/yellow/red thresholds for utilization gauges.
func utilizationThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green"},
			{Value: float64Ptr(0.6), Color: "yellow"},
			{Value: float64Ptr(0.8), Color: "red"},
		})
}

// paletteColor returns a field color in palette-classic mode.
func paletteColor() *dashboard.FieldColorBuilder {
	return dashboard.NewFieldColorBuilder().
		Mode(dashboard.FieldColorModeIdPaletteClassic)
}

// fixedColor returns a field color with a fixed color.
func fixedColor(color string) *dashboard.FieldColorBuilder {
	return dashboard.NewFieldColorBuilder().
		Mode(dashboard.FieldColorModeIdFixed).
		FixedColor(color)
}
