// Package anthropicreceiver implements an OpenTelemetry Collector receiver
// that acts as an HTTP reverse proxy for the Anthropic API.
//
// It captures traces, metrics, and logs from every API call passing through it,
// including token usage, costs, rate limits, streaming metrics, and tool call analysis.
// Clients simply point their Anthropic SDK's base_url at the collector.
package anthropicreceiver // import "github.com/guicaulada/anthropic-otel-collector/receiver/anthropicreceiver"
