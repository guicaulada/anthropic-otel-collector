package anthropicreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	require.NotNil(t, factory)
	assert.Equal(t, componentType, factory.Type())
}

func TestFactory_CreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	rCfg, ok := cfg.(*Config)
	require.True(t, ok)
	assert.Equal(t, "https://api.anthropic.com", rCfg.AnthropicAPI)
	assert.Equal(t, "0.0.0.0:4319", rCfg.ServerConfig.NetAddr.Endpoint)
}

func TestFactory_CreateTracesReceiver(t *testing.T) {
	// Clean up shared receivers to avoid cross-test pollution
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	settings := receivertest.NewNopSettings(componentType)

	receiver, err := factory.CreateTraces(context.Background(), settings, cfg, consumertest.NewNop())
	require.NoError(t, err)
	require.NotNil(t, receiver)
}

func TestFactory_CreateMetricsReceiver(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	settings := receivertest.NewNopSettings(componentType)

	receiver, err := factory.CreateMetrics(context.Background(), settings, cfg, consumertest.NewNop())
	require.NoError(t, err)
	require.NotNil(t, receiver)
}

func TestFactory_CreateLogsReceiver(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	settings := receivertest.NewNopSettings(componentType)

	receiver, err := factory.CreateLogs(context.Background(), settings, cfg, consumertest.NewNop())
	require.NoError(t, err)
	require.NotNil(t, receiver)
}

func TestFactory_SharedReceiver(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	settings := receivertest.NewNopSettings(componentType)

	tracesReceiver, err := factory.CreateTraces(context.Background(), settings, cfg, consumertest.NewNop())
	require.NoError(t, err)

	metricsReceiver, err := factory.CreateMetrics(context.Background(), settings, cfg, consumertest.NewNop())
	require.NoError(t, err)

	// Both should be the same underlying receiver instance
	assert.Same(t, tracesReceiver, metricsReceiver)

	logsReceiver, err := factory.CreateLogs(context.Background(), settings, cfg, consumertest.NewNop())
	require.NoError(t, err)

	assert.Same(t, tracesReceiver, logsReceiver)
}
