package panels

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// CostOverTime returns a timeseries panel showing total cost rate in $/hr.
func CostOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cost Over Time").
		Description("Total cost rate in $/hr").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
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

// CostByModel returns a stacked timeseries panel showing cost rate per model.
func CostByModel() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cost by Model").
		Description("Cost rate in $/hr broken down by model").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
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
				f(`sum by (gen_ai_request_model) (rate(anthropic_cost_total{%s}[$__rate_interval])) * 3600`),
				"{{gen_ai_request_model}}",
			),
		)
}

// CostByCategory returns a donut piechart panel showing cost distribution by category.
func CostByCategory() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Cost by Category").
		Description("Cost distribution across input, output, cache read, and cache creation").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("currencyUSD").
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
				f(`sum(increase(anthropic_cost_input_tokens_total{%s}[$__range]))`),
				"Input Tokens",
			),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_cost_output_tokens_total{%s}[$__range]))`),
				"Output Tokens",
			),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_cost_cache_read_total{%s}[$__range]))`),
				"Cache Read",
			),
		).
		WithTarget(
			promInstantQuery(
				f(`sum(increase(anthropic_cost_cache_creation_total{%s}[$__range]))`),
				"Cache Creation",
			),
		)
}

// CumulativeCost returns a timeseries panel showing the cumulative total cost.
func CumulativeCost() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Cumulative Cost").
		Description("Running total of all costs").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("currencyUSD").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(anthropic_cost_total{%s})`),
				"Cumulative Cost",
			),
		)
}

// AvgCostPerRequestTimeseries returns a timeseries panel showing average cost per request over time.
func AvgCostPerRequestTimeseries() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Cost / Request").
		Description("Average cost per request over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("currencyUSD").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_cost_request_sum{%s}[$__rate_interval])) / sum(rate(anthropic_cost_request_count{%s}[$__rate_interval]))`),
				"Avg Cost / Request",
			),
		)
}

// CostMultipliedRequests returns a stacked timeseries panel showing request rates by cost multiplier.
func CostMultipliedRequests() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Requests by Cost Multiplier").
		Description("Request rate broken down by cost multiplier").
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
				f(`sum by (multiplier) (rate(anthropic_cost_multiplied_requests_total{%s}[$__rate_interval]))`),
				"{{multiplier}}x",
			),
		)
}

// WebSearchCost returns a timeseries panel showing web search tool cost rate.
func WebSearchCost() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Web Search Cost").
		Description("Web search server tool cost rate in $/hr").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("currencyUSD").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_cost_server_tool_use_web_search_total{%s}[$__rate_interval])) * 3600`),
				"Web Search $/hr",
			),
		)
}
