package anthropicreceiver

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
)

// telemetryBuilder constructs and emits traces, metrics, and logs from captured request data.
type telemetryBuilder struct {
	cfg             *Config
	logger          *zap.Logger
	tracesConsumer  consumer.Traces
	metricsConsumer consumer.Metrics
	logsConsumer    consumer.Logs
	serverHost      string
	serverPort      int
}

func newTelemetryBuilder(
	cfg *Config,
	logger *zap.Logger,
	traces consumer.Traces,
	metrics consumer.Metrics,
	logs consumer.Logs,
) *telemetryBuilder {
	host, port := parseServerAddr(cfg.AnthropicAPI)
	return &telemetryBuilder{
		cfg:             cfg,
		logger:          logger,
		tracesConsumer:  traces,
		metricsConsumer: metrics,
		logsConsumer:    logs,
		serverHost:      host,
		serverPort:      port,
	}
}

// parseServerAddr extracts the hostname and port from a URL string.
// Returns the hostname and port (as int), using the scheme's
// default port when no explicit port is specified.
func parseServerAddr(rawURL string) (string, int) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, 0
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		switch u.Scheme {
		case "https":
			return host, 443
		case "http":
			return host, 80
		}
		return host, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

// requestData holds all captured data for a single API call.
type requestData struct {
	// Timing
	startTime       time.Time
	endTime         time.Time
	upstreamLatency time.Duration

	// Request
	request     *AnthropicRequest
	requestBody []byte
	requestSize int
	apiKeyHash  string

	// Response
	response     *AnthropicResponse
	responseBody []byte
	responseSize int
	statusCode   int
	requestID    string

	// Rate limits
	rateLimit RateLimitInfo

	// Streaming
	isStreaming bool
	streaming  *StreamingMetrics

	// Parsed tool calls
	toolCalls []ToolCallInfo

	// Cost
	cost CostResult

	// Error
	errorResponse *AnthropicError

	// Active requests
	activeRequests int64

	// Additional metadata
	betaFeatures   string
	organizationID string
	speed          string
	apiVersion     string
}

func (tb *telemetryBuilder) emit(ctx context.Context, data *requestData) {
	if tb.tracesConsumer != nil {
		if err := tb.emitTraces(ctx, data); err != nil {
			tb.logger.Error("Failed to emit traces", zap.Error(err))
		}
	}
	if tb.metricsConsumer != nil {
		if err := tb.emitMetrics(ctx, data); err != nil {
			tb.logger.Error("Failed to emit metrics", zap.Error(err))
		}
	}
	if tb.logsConsumer != nil {
		if err := tb.emitLogs(ctx, data); err != nil {
			tb.logger.Error("Failed to emit logs", zap.Error(err))
		}
	}
}

func setResourceAttributes(attrs pcommon.Map) {
	attrs.PutStr("service.name", "anthropic-otel-collector")
	attrs.PutStr("service.namespace", "anthropic")
}

func (data *requestData) requestModel() string {
	if data.request != nil {
		return data.request.Model
	}
	if data.response != nil {
		return data.response.Model
	}
	return "unknown"
}

// hashAPIKey returns a truncated SHA256 hash of the API key for identification.
func hashAPIKey(key string) string {
	if key == "" {
		return ""
	}
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:4])
}

func generateTraceID(data *requestData) pcommon.TraceID {
	h := sha256.Sum256([]byte(data.requestID + data.startTime.String()))
	var tid pcommon.TraceID
	copy(tid[:], h[:16])
	return tid
}

func generateSpanID(data *requestData, index int) pcommon.SpanID {
	h := sha256.Sum256([]byte(data.requestID + data.startTime.String() + strconv.Itoa(index)))
	var sid pcommon.SpanID
	copy(sid[:], h[:8])
	return sid
}
