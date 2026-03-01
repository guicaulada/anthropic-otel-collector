package panels

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/gauge"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)


// RequestRateLimitUtilization returns a gauge showing request rate limit utilization.
func RequestRateLimitUtilization() cog.Builder[dashboard.Panel] {
	return gauge.NewPanelBuilder().
		Title("Request Rate Limit Utilization").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promInstantQuery(
				`avg(anthropic_ratelimit_requests_utilization_ratio{`+filter+`})`,
				"",
			),
		).
		Unit("percentunit").
		Min(0).
		Max(1).
		Thresholds(utilizationThresholds())
}

// InputTokenRateLimitUtilization returns a gauge showing input token rate limit utilization.
func InputTokenRateLimitUtilization() cog.Builder[dashboard.Panel] {
	return gauge.NewPanelBuilder().
		Title("Input Token Rate Limit Utilization").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promInstantQuery(
				`avg(anthropic_ratelimit_input_tokens_utilization_ratio{`+filter+`})`,
				"",
			),
		).
		Unit("percentunit").
		Min(0).
		Max(1).
		Thresholds(utilizationThresholds())
}

// OutputTokenRateLimitUtilization returns a gauge showing output token rate limit utilization.
func OutputTokenRateLimitUtilization() cog.Builder[dashboard.Panel] {
	return gauge.NewPanelBuilder().
		Title("Output Token Rate Limit Utilization").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promInstantQuery(
				`avg(anthropic_ratelimit_output_tokens_utilization_ratio{`+filter+`})`,
				"",
			),
		).
		Unit("percentunit").
		Min(0).
		Max(1).
		Thresholds(utilizationThresholds())
}

// RequestsRemaining returns a timeseries showing remaining request rate limit capacity.
func RequestsRemaining() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Requests Remaining").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promRangeQuery(
				`avg(anthropic_ratelimit_requests_remaining{`+filter+`})`,
				"Remaining",
			),
		).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds())
}

// InputTokensRemaining returns a timeseries showing remaining input token rate limit capacity.
func InputTokensRemaining() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Input Tokens Remaining").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promRangeQuery(
				`avg(anthropic_ratelimit_input_tokens_remaining{`+filter+`})`,
				"Remaining",
			),
		).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds())
}

// OutputTokensRemaining returns a timeseries showing remaining output token rate limit capacity.
func OutputTokensRemaining() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Output Tokens Remaining").
		Datasource(datasourceRef()).
		Height(8).
		Span(8).
		WithTarget(
			promRangeQuery(
				`avg(anthropic_ratelimit_output_tokens_remaining{`+filter+`})`,
				"Remaining",
			),
		).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds())
}

// RateLimitUtilizationOverTime returns a timeseries showing all rate limit utilization over time.
func RateLimitUtilizationOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Rate Limit Utilization Over Time").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		WithTarget(
			promRangeQuery(
				`avg(anthropic_ratelimit_requests_utilization_ratio{`+filter+`})`,
				"Requests",
			),
		).
		WithTarget(
			promRangeQuery(
				`avg(anthropic_ratelimit_input_tokens_utilization_ratio{`+filter+`})`,
				"Input Tokens",
			),
		).
		WithTarget(
			promRangeQuery(
				`avg(anthropic_ratelimit_output_tokens_utilization_ratio{`+filter+`})`,
				"Output Tokens",
			),
		).
		Unit("percentunit").
		Min(0).
		Max(1).
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		Thresholds(utilizationThresholds()).
		ColorScheme(paletteColor())
}
