package panels

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
)

// TotalCost returns a stat panel showing the total cost over the selected time range.
func TotalCost() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Total Cost").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
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

// TotalRequests returns a stat panel showing the total number of requests.
func TotalRequests() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Total Requests").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
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

// ErrorRate returns a stat panel showing the current error rate as a percentage.
func ErrorRate() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Error Rate").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
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

// AvgLatency returns a stat panel showing the average operation duration.
func AvgLatency() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Avg Latency").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
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

// TotalTokens returns a stat panel showing the combined input and output token count.
func TotalTokens() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Total Tokens").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
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

// CacheHitRatio returns a stat panel showing the average cache hit ratio.
func CacheHitRatio() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Cache Hit Ratio").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
		Unit("percentunit").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`avg(anthropic_cache_hit_ratio{%s})`),
				"Cache Hit Ratio",
			),
		)
}

// AvgCostPerRequest returns a stat panel showing the average cost per request.
func AvgCostPerRequest() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Avg Cost / Request").
		Datasource(datasourceRef()).
		Height(8).
		Span(5).
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
				f(`sum(rate(anthropic_cost_request_sum{%s}[$__rate_interval])) / sum(rate(anthropic_cost_request_count{%s}[$__rate_interval]))`),
				"Avg Cost / Request",
			),
		)
}

// OutputThroughput returns a stat panel showing average output token throughput.
func OutputThroughput() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Output Throughput").
		Datasource(datasourceRef()).
		Height(8).
		Span(5).
		Unit("suffix: tok/s").
		Thresholds(greenThresholds()).
		GraphMode(common.BigValueGraphModeArea).
		ColorMode(common.BigValueColorModeValue).
		ReduceOptions(
			common.NewReduceDataOptionsBuilder().
				Calcs([]string{"lastNotNull"}),
		).
		WithTarget(
			promInstantQuery(
				f(`avg(anthropic_throughput_output_tokens_per_second_per_second{%s})`),
				"Output Throughput",
			),
		)
}

// RequestsPerMinute returns a stat panel showing the current request rate per minute.
func RequestsPerMinute() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Requests / Min").
		Datasource(datasourceRef()).
		Height(8).
		Span(5).
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
				f(`sum(rate(anthropic_requests_total{%s}[$__rate_interval])) * 60`),
				"Requests / Min",
			),
		)
}

// FastModeRequests returns a stat panel showing the total fast-mode requests.
func FastModeRequests() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Fast Mode Requests").
		Datasource(datasourceRef()).
		Height(8).
		Span(5).
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
				f(`sum(increase(anthropic_requests_by_speed_total{%s, speed="fast"}[$__range])) or vector(0)`),
				"Fast Mode Requests",
			),
		)
}

// CacheSavingsStat returns a stat panel showing total cache savings over the selected range.
func CacheSavingsStat() cog.Builder[dashboard.Panel] {
	return stat.NewPanelBuilder().
		Title("Cache Savings").
		Datasource(datasourceRef()).
		Height(8).
		Span(4).
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
