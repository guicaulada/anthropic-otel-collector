package anthropicreceiver

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

type anthropicReceiver struct {
	cfg    *Config
	logger *zap.Logger
	settings receiver.Settings

	tracesConsumer  consumer.Traces
	metricsConsumer consumer.Metrics
	logsConsumer    consumer.Logs

	server         *http.Server
	upstreamURL    *url.URL
	httpClient     *http.Client
	telemetry      *telemetryBuilder
	sessionTracker *sessionTracker

	activeRequests int64

	startOnce sync.Once
	stopOnce  sync.Once
	shutdownWG sync.WaitGroup
}

func newAnthropicReceiver(cfg *Config, settings receiver.Settings) *anthropicReceiver {
	return &anthropicReceiver{
		cfg:      cfg,
		logger:   settings.Logger,
		settings: settings,
	}
}

// Start implements component.Component.
func (r *anthropicReceiver) Start(ctx context.Context, host component.Host) error {
	var err error
	r.startOnce.Do(func() {
		err = r.start(ctx, host)
	})
	return err
}

func (r *anthropicReceiver) start(ctx context.Context, host component.Host) error {
	u, err := url.Parse(r.cfg.AnthropicAPI)
	if err != nil {
		return err
	}
	r.upstreamURL = u

	r.httpClient = &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			TLSClientConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	r.telemetry = newTelemetryBuilder(
		r.cfg,
		r.logger,
		r.tracesConsumer,
		r.metricsConsumer,
		r.logsConsumer,
	)

	r.sessionTracker = newSessionTracker(r.cfg.SessionTimeout)

	mux := http.NewServeMux()
	mux.HandleFunc("/", r.handleProxy)

	var httpServer *http.Server
	httpServer, err = r.cfg.ServerConfig.ToServer(ctx, host.GetExtensions(), r.settings.TelemetrySettings, mux)
	if err != nil {
		return err
	}
	r.server = httpServer

	listener, lErr := r.cfg.ServerConfig.ToListener(ctx)
	if lErr != nil {
		return lErr
	}

	r.logger.Info("Starting Anthropic receiver",
		zap.String("endpoint", r.cfg.ServerConfig.NetAddr.Endpoint),
		zap.String("upstream", r.cfg.AnthropicAPI),
	)

	r.shutdownWG.Add(1)
	go func() {
		defer r.shutdownWG.Done()
		if sErr := r.server.Serve(listener); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			r.logger.Error("HTTP server error", zap.Error(sErr))
		}
	}()

	return nil
}

// Shutdown implements component.Component.
func (r *anthropicReceiver) Shutdown(ctx context.Context) error {
	var err error
	r.stopOnce.Do(func() {
		err = r.shutdown(ctx)
	})
	return err
}

func (r *anthropicReceiver) shutdown(ctx context.Context) error {
	removeReceiver(r.settings.ID)

	if r.sessionTracker != nil {
		r.sessionTracker.Stop()
	}

	if r.server != nil {
		if err := r.server.Shutdown(ctx); err != nil {
			return err
		}
	}

	r.shutdownWG.Wait()

	if r.httpClient != nil {
		r.httpClient.CloseIdleConnections()
	}

	r.logger.Info("Anthropic receiver stopped")
	return nil
}
