package panels

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Prometheus metric names for streaming panels.
const (
	MetricStreamingDuration      = "anthropic_streaming_duration_seconds"
	MetricStreamingChunks        = "anthropic_streaming_chunks"
	MetricStreamingEvents        = "anthropic_streaming_events_total"
	MetricTimeToFirstToken       = "gen_ai_server_time_to_first_token_seconds"
	MetricThroughputOutputTokens = "anthropic_throughput_output_tokens_per_second"
)

// AvgStreamingDuration returns a timeseries showing average streaming duration over time.
func AvgStreamingDuration() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Streaming Duration").
		Description("Average streaming duration per request over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		WithTarget(promQuery(
			fmt.Sprintf(`sum(rate(%s_sum{%s}[$__rate_interval])) / sum(rate(%s_count{%s}[$__rate_interval]))`, MetricStreamingDuration, filter, MetricStreamingDuration, filter),
			"Avg Duration",
		)).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// AvgChunksPerRequest returns a timeseries showing average chunks per request over time.
func AvgChunksPerRequest() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("Avg Chunks per Request").
		Description("Average number of streaming chunks per request over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		WithTarget(promQuery(
			fmt.Sprintf(`sum(rate(%s_sum{%s}[$__rate_interval])) / sum(rate(%s_count{%s}[$__rate_interval]))`, MetricStreamingChunks, filter, MetricStreamingChunks, filter),
			"Avg Chunks",
		)).
		Unit("short").
		Legend(defaultLegend()).
		Tooltip(singleTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// SSEEventTypesOverTime returns a stacked timeseries showing SSE event type rates.
func SSEEventTypesOverTime() cog.Builder[dashboard.Panel] {
	return timeseries.NewPanelBuilder().
		Title("SSE Event Types Over Time").
		Description("Rate of server-sent events by type over time").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		WithTarget(promQuery(
			fmt.Sprintf(`sum by (event_type) (rate(%s{%s}[$__rate_interval]))`, MetricStreamingEvents, filter),
			"{{event_type}}",
		)).
		FillOpacity(30).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(rightTableLegend()).
		Tooltip(multiTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor())
}

// AvgTTFTvsThroughput returns a dual-axis timeseries comparing time to first token
// with output token throughput.
func AvgTTFTvsThroughput() cog.Builder[dashboard.Panel] {
	ttftQuery := promQuery(
		fmt.Sprintf(`sum(rate(%s_sum{%s}[$__rate_interval])) / sum(rate(%s_count{%s}[$__rate_interval]))`, MetricTimeToFirstToken, filter, MetricTimeToFirstToken, filter),
		"TTFT",
	)
	throughputQuery := promQuery(
		fmt.Sprintf(`avg(%s{%s})`, MetricThroughputOutputTokens, filter),
		"Throughput",
	)

	return timeseries.NewPanelBuilder().
		Title("Avg TTFT vs Throughput").
		Description("Average time to first token compared with output token throughput").
		Datasource(datasourceRef()).
		Height(8).
		Span(24).
		WithTarget(ttftQuery).
		WithTarget(throughputQuery).
		Unit("s").
		Legend(defaultLegend()).
		Tooltip(multiTooltip()).
		Thresholds(greenThresholds()).
		ColorScheme(paletteColor()).
		OverrideByName("Throughput", []dashboard.DynamicConfigValue{
			{Id: "custom.axisPlacement", Value: "right"},
			{Id: "unit", Value: "short"},
		})
}

// StreamingVsNonStreaming returns a donut piechart showing the ratio of streaming
// to non-streaming requests.
func StreamingVsNonStreaming() cog.Builder[dashboard.Panel] {
	return piechart.NewPanelBuilder().
		Title("Streaming vs Non-Streaming").
		Description("Distribution of streaming and non-streaming requests over the selected range").
		Datasource(datasourceRef()).
		Height(8).
		Span(12).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(increase(%s{%s, anthropic_request_streaming="true"}[$__range]))`, MetricRequests, filter),
			"Streaming",
		)).
		WithTarget(instantQuery(
			fmt.Sprintf(`sum(increase(%s{%s, anthropic_request_streaming="false"}[$__range]))`, MetricRequests, filter),
			"Non-Streaming",
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
