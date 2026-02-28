package anthropicreceiver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestReceiver_StartShutdown(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	port := getFreePort(t)
	cfg := defaultConfig()
	cfg.ServerConfig.NetAddr.Endpoint = fmt.Sprintf("127.0.0.1:%d", port)

	settings := receivertest.NewNopSettings(componentType)
	r := newAnthropicReceiver(cfg, settings)
	r.tracesConsumer = consumertest.NewNop()
	r.metricsConsumer = consumertest.NewNop()
	r.logsConsumer = consumertest.NewNop()

	ctx := context.Background()
	host := componenttest.NewNopHost()

	err := r.Start(ctx, host)
	require.NoError(t, err)

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)

	// Verify HTTP server is listening
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err == nil {
		resp.Body.Close()
	}
	// We don't check the response code since /health isn't a defined endpoint,
	// but the server should accept the connection.

	err = r.Shutdown(ctx)
	require.NoError(t, err)
}

func TestReceiver_StartHTTPServer(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	port := getFreePort(t)
	cfg := defaultConfig()
	cfg.ServerConfig.NetAddr.Endpoint = fmt.Sprintf("127.0.0.1:%d", port)

	settings := receivertest.NewNopSettings(componentType)
	r := newAnthropicReceiver(cfg, settings)
	r.tracesConsumer = consumertest.NewNop()
	r.metricsConsumer = consumertest.NewNop()
	r.logsConsumer = consumertest.NewNop()

	ctx := context.Background()
	host := componenttest.NewNopHost()

	err := r.Start(ctx, host)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	// Server should accept connections on the configured port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	require.NoError(t, err, "should be able to connect to the HTTP server")
	conn.Close()

	err = r.Shutdown(ctx)
	require.NoError(t, err)
}

func TestReceiver_ShutdownIdempotent(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	port := getFreePort(t)
	cfg := defaultConfig()
	cfg.ServerConfig.NetAddr.Endpoint = fmt.Sprintf("127.0.0.1:%d", port)

	settings := receivertest.NewNopSettings(componentType)
	r := newAnthropicReceiver(cfg, settings)
	r.tracesConsumer = consumertest.NewNop()
	r.metricsConsumer = consumertest.NewNop()
	r.logsConsumer = consumertest.NewNop()

	ctx := context.Background()
	host := componenttest.NewNopHost()

	err := r.Start(ctx, host)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Shutdown twice should not error
	err = r.Shutdown(ctx)
	require.NoError(t, err)

	err = r.Shutdown(ctx)
	require.NoError(t, err)
}

func TestReceiver_StartIdempotent(t *testing.T) {
	sharedMu.Lock()
	sharedReceivers = make(map[component.ID]*anthropicReceiver)
	sharedMu.Unlock()

	port := getFreePort(t)
	cfg := defaultConfig()
	cfg.ServerConfig.NetAddr.Endpoint = fmt.Sprintf("127.0.0.1:%d", port)

	settings := receivertest.NewNopSettings(componentType)
	r := newAnthropicReceiver(cfg, settings)
	r.tracesConsumer = consumertest.NewNop()
	r.metricsConsumer = consumertest.NewNop()
	r.logsConsumer = consumertest.NewNop()

	ctx := context.Background()
	host := componenttest.NewNopHost()

	// Start twice should not error (startOnce)
	err := r.Start(ctx, host)
	require.NoError(t, err)

	err = r.Start(ctx, host)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = r.Shutdown(ctx)
	require.NoError(t, err)
}
