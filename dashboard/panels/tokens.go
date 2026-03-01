package panels

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// TokenUsageOverTime returns a stacked area timeseries showing token usage rates.
func TokenUsageOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Token Usage Over Time").
		Description("Rate of token usage by type over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("short").
		FillOpacity(30).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_input_total{%s}[$__rate_interval]))`),
				"Input",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_output_total{%s}[$__rate_interval]))`),
				"Output",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_cache_read_total{%s}[$__rate_interval]))`),
				"Cache Read",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_cache_creation_total{%s}[$__rate_interval]))`),
				"Cache Creation",
			),
		)
}

// CacheHitRatioOverTime returns a timeseries showing the cache hit ratio over time.
func CacheHitRatioOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cache Hit Ratio Over Time").
		Description("Average cache hit ratio over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("percentunit").
		Min(0).
		Max(1).
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_cache_hit_ratio{%s})`),
				"Cache Hit Ratio",
			),
		)
}

// InputVsOutputBreakdown returns a donut piechart showing input vs output token distribution.
func InputVsOutputBreakdown() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Input vs Output Breakdown").
		Description("Distribution of input and output tokens over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		PieType(piechart.PieChartTypeDonut).
		Legend(
			piechart.NewPieChartLegendOptionsBuilder().
				DisplayMode(common.LegendDisplayModeList).
				Placement(common.LegendPlacementBottom).
				ShowLegend(true).
				Values([]piechart.PieChartLegendValues{
					piechart.PieChartLegendValuesValue,
					piechart.PieChartLegendValuesPercent,
				}),
		).
		Tooltip(multiTooltip()).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tokens_input_total{%s}[$__range]))`),
				"Input",
			),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tokens_output_total{%s}[$__range]))`),
				"Output",
			),
		)
}

// TokensByModel returns a stacked timeseries showing token rates broken down by model.
func TokensByModel() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Tokens by Model").
		Description("Token usage rate broken down by model").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("short").
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum by (gen_ai_request_model) (rate(anthropic_tokens_input_total{%s}[$__rate_interval]))`),
				"{{gen_ai_request_model}} Input",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum by (gen_ai_request_model) (rate(anthropic_tokens_output_total{%s}[$__rate_interval]))`),
				"{{gen_ai_request_model}} Output",
			),
		)
}

// CacheTokensDetail returns a timeseries showing cache read and creation token rates.
func CacheTokensDetail() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cache Tokens Detail").
		Description("Rate of cache read and cache creation tokens").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_cache_read_total{%s}[$__rate_interval]))`),
				"Cache Read",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_tokens_cache_creation_total{%s}[$__rate_interval]))`),
				"Cache Creation",
			),
		)
}

// TotalInputTokensBreakdown returns a horizontal bar gauge showing the breakdown of input token types.
func TotalInputTokensBreakdown() cog.Builder[dashboard.Panel] {
	return bargauge.NewPanelBuilder().
		Title("Total Input Tokens Breakdown").
		Description("Breakdown of input, cache read, and cache creation tokens over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		Orientation(common.VizOrientationHorizontal).
		Thresholds(greenThresholds()).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tokens_input_total{%s}[$__range]))`),
				"Input",
			),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tokens_cache_read_total{%s}[$__range]))`),
				"Cache Read",
			),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_tokens_cache_creation_total{%s}[$__range]))`),
				"Cache Creation",
			),
		)
}
