package anthropicreceiver

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

const (
	typeStr   = "anthropic"
	stability = component.StabilityLevelAlpha
)

// componentType is the component type identifier.
var componentType = component.MustNewType(typeStr)

// NewFactory creates a new factory for the Anthropic receiver.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		componentType,
		createDefaultConfig,
		receiver.WithTraces(createTracesReceiver, stability),
		receiver.WithMetrics(createMetricsReceiver, stability),
		receiver.WithLogs(createLogsReceiver, stability),
	)
}

func createDefaultConfig() component.Config {
	return defaultConfig()
}

// sharedReceivers stores shared receiver instances keyed by component.ID.
// This ensures traces, metrics, and logs signals share a single HTTP server.
var (
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu        sync.Mutex
)

func getOrCreateReceiver(
	id component.ID,
	cfg *Config,
	settings receiver.Settings,
) *anthropicReceiver {
	sharedMu.Lock()
	defer sharedMu.Unlock()

	if r, ok := sharedReceivers[id]; ok {
		return r
	}

	r := newAnthropicReceiver(cfg, settings)
	sharedReceivers[id] = r
	return r
}

func removeReceiver(id component.ID) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	delete(sharedReceivers, id)
}

func createTracesReceiver(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (receiver.Traces, error) {
	rCfg := cfg.(*Config)
	r := getOrCreateReceiver(settings.ID, rCfg, settings)
	r.tracesConsumer = nextConsumer
	return r, nil
}

func createMetricsReceiver(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (receiver.Metrics, error) {
	rCfg := cfg.(*Config)
	r := getOrCreateReceiver(settings.ID, rCfg, settings)
	r.metricsConsumer = nextConsumer
	return r, nil
}

func createLogsReceiver(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	rCfg := cfg.(*Config)
	r := getOrCreateReceiver(settings.ID, rCfg, settings)
	r.logsConsumer = nextConsumer
	return r, nil
}
