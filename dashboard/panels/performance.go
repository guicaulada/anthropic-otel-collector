package panels

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// AvgOperationDuration returns a timeseries showing average operation duration.
func AvgOperationDuration() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Operation Duration").
		Description("Average operation duration over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(gen_ai_client_operation_duration_seconds_sum{%s}[$__rate_interval])) / sum(rate(gen_ai_client_operation_duration_seconds_count{%s}[$__rate_interval]))`),
				"Average",
			),
		)
}

// AvgTimeToFirstToken returns a timeseries showing average time to first token.
func AvgTimeToFirstToken() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Time to First Token").
		Description("Average time to first token (TTFT) over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(gen_ai_server_time_to_first_token_seconds_sum{%s}[$__rate_interval])) / sum(rate(gen_ai_server_time_to_first_token_seconds_count{%s}[$__rate_interval]))`),
				"TTFT",
			),
		)
}

// AvgTimePerOutputToken returns a timeseries showing average time per output token.
func AvgTimePerOutputToken() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Time per Output Token").
		Description("Average time per output token over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(gen_ai_server_time_per_output_token_seconds_sum{%s}[$__rate_interval])) / sum(rate(gen_ai_server_time_per_output_token_seconds_count{%s}[$__rate_interval]))`),
				"Time/Token",
			),
		)
}

// OutputThroughputTimeseries returns a timeseries showing output token throughput.
func OutputThroughputTimeseries() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Output Throughput").
		Description("Average output tokens per second over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`avg(anthropic_throughput_output_tokens_per_second_per_second{%s})`),
				"Output tok/s",
			),
		)
}

// AvgUpstreamLatency returns a timeseries showing average upstream latency.
func AvgUpstreamLatency() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Upstream Latency").
		Description("Average upstream latency over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_upstream_latency_seconds_sum{%s}[$__rate_interval])) / sum(rate(anthropic_upstream_latency_seconds_count{%s}[$__rate_interval]))`),
				"Upstream Latency",
			),
		)
}

// RequestResponseBodySize returns a timeseries showing average request and response body sizes.
func RequestResponseBodySize() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Request / Response Body Size").
		Description("Average request and response body sizes over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		Unit("decbytes").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_request_body_size_bytes_sum{%s}[$__rate_interval])) / sum(rate(anthropic_request_body_size_bytes_count{%s}[$__rate_interval]))`),
				"Request",
			),
		).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_response_body_size_bytes_sum{%s}[$__rate_interval])) / sum(rate(anthropic_response_body_size_bytes_count{%s}[$__rate_interval]))`),
				"Response",
			),
		)
}

// RequestRate returns a timeseries showing the request rate over time.
func RequestRate() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Request Rate").
		Description("Request rate over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("reqps").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum(rate(anthropic_requests_total{%s}[$__rate_interval]))`),
				"Requests/s",
			),
		)
}

// ErrorRateOverTime returns a timeseries showing error rates broken down by error type.
func ErrorRateOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Error Rate Over Time").
		Description("Error rate broken down by error type").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		WithTarget(
			promRangeQuery(
				f(`sum by (error_type) (rate(anthropic_errors_total{%s}[$__rate_interval]))`),
				"{{error_type}}",
			),
		)
}
