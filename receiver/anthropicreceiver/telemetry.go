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
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
	serverPort      string
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
// Returns the hostname and port as separate strings, using the scheme's
// default port when no explicit port is specified.
func parseServerAddr(rawURL string) (host, port string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, ""
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
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

	// Additional metadata
	betaFeatures   string
	organizationID string
	speed          string
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

// ---- Traces ----

func (tb *telemetryBuilder) emitTraces(ctx context.Context, data *requestData) error {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("github.com/guicaulada/anthropic-otel-collector/receiver/anthropicreceiver")

	model := data.requestModel()

	// Root span
	rootSpan := ss.Spans().AppendEmpty()
	rootSpan.SetName("chat " + model)
	rootSpan.SetKind(ptrace.SpanKindClient)
	rootSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(data.startTime))
	rootSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	rootSpan.SetTraceID(generateTraceID(data))
	rootSpan.SetSpanID(generateSpanID(data, 0))

	if data.statusCode >= 400 {
		rootSpan.Status().SetCode(ptrace.StatusCodeError)
		if data.errorResponse != nil {
			rootSpan.Status().SetMessage(data.errorResponse.Error.Message)
		}
	} else {
		rootSpan.Status().SetCode(ptrace.StatusCodeOk)
	}

	// Set all attributes
	attrs := rootSpan.Attributes()
	tb.setSpanAttributes(attrs, data)

	// Add events
	tb.addSpanEvents(rootSpan, data)

	// Child span for upstream call
	upstreamSpan := ss.Spans().AppendEmpty()
	upstreamSpan.SetName("POST /v1/messages")
	upstreamSpan.SetKind(ptrace.SpanKindClient)
	upstreamSpan.SetTraceID(rootSpan.TraceID())
	upstreamSpan.SetParentSpanID(rootSpan.SpanID())
	upstreamSpan.SetSpanID(generateSpanID(data, 1))

	upstreamStart := data.startTime
	upstreamEnd := data.startTime.Add(data.upstreamLatency)
	if data.isStreaming && data.streaming != nil {
		upstreamEnd = data.endTime
	}
	upstreamSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(upstreamStart))
	upstreamSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(upstreamEnd))

	upAttrs := upstreamSpan.Attributes()
	upAttrs.PutStr("http.request.method", "POST")
	upAttrs.PutStr("server.address", tb.serverHost)
	upAttrs.PutStr("server.port", tb.serverPort)
	upAttrs.PutInt("http.response.status_code", int64(data.statusCode))
	upAttrs.PutStr("url.full", tb.cfg.AnthropicAPI+"/v1/messages")

	if data.statusCode >= 400 {
		upstreamSpan.Status().SetCode(ptrace.StatusCodeError)
	}

	return tb.tracesConsumer.ConsumeTraces(ctx, td)
}

func (tb *telemetryBuilder) setSpanAttributes(attrs pcommon.Map, data *requestData) {
	attrs.PutStr("gen_ai.operation.name", "chat")
	attrs.PutStr("gen_ai.provider.name", "anthropic")
	attrs.PutStr("gen_ai.request.model", data.requestModel())
	attrs.PutStr("http.request.method", "POST")
	attrs.PutStr("server.address", tb.serverHost)
	attrs.PutStr("server.port", tb.serverPort)
	attrs.PutStr("url.path", "/v1/messages")
	attrs.PutBool("anthropic.request.streaming", data.isStreaming)

	if data.apiKeyHash != "" {
		attrs.PutStr("anthropic.api_key_hash", data.apiKeyHash)
	}

	if data.request != nil {
		attrs.PutInt("gen_ai.request.max_tokens", int64(data.request.MaxTokens))
		if data.request.Temperature != nil {
			attrs.PutDouble("gen_ai.request.temperature", *data.request.Temperature)
		}
		if data.request.TopP != nil {
			attrs.PutDouble("gen_ai.request.top_p", *data.request.TopP)
		}
		if data.request.TopK != nil {
			attrs.PutInt("gen_ai.request.top_k", int64(*data.request.TopK))
		}
		attrs.PutInt("anthropic.request.messages_count", int64(len(data.request.Messages)))
		for role, count := range data.request.MessageRoleCounts() {
			attrs.PutInt("anthropic.request."+role+"_messages_count", int64(count))
		}
		attrs.PutBool("anthropic.request.has_system_prompt", data.request.HasSystemPrompt())
		attrs.PutInt("anthropic.request.system_prompt.size", int64(data.request.SystemPromptSize()))
		attrs.PutInt("anthropic.request.tools_count", int64(len(data.request.Tools)))
		attrs.PutInt("anthropic.request.stop_sequences_count", int64(len(data.request.StopSequences)))

		if data.request.Thinking != nil {
			attrs.PutBool("anthropic.request.thinking.enabled", true)
			attrs.PutInt("anthropic.request.thinking.budget_tokens", int64(data.request.Thinking.BudgetTokens))
		}
	}

	if data.response != nil {
		attrs.PutStr("gen_ai.response.model", data.response.Model)
		attrs.PutStr("gen_ai.response.id", data.response.ID)

		finishReasons := attrs.PutEmptySlice("gen_ai.response.finish_reasons")
		finishReasons.AppendEmpty().SetStr(data.response.StopReason)

		usage := data.response.Usage
		attrs.PutInt("gen_ai.usage.input_tokens", int64(usage.InputTokens))
		attrs.PutInt("gen_ai.usage.output_tokens", int64(usage.OutputTokens))
		attrs.PutInt("gen_ai.usage.cache_read.input_tokens", int64(usage.CacheReadInputTokens))
		attrs.PutInt("gen_ai.usage.cache_creation.input_tokens", int64(usage.CacheCreationInputTokens))
		attrs.PutInt("anthropic.usage.total_input_tokens", int64(usage.TotalInputTokens()))
		attrs.PutDouble("anthropic.cache.hit_ratio", CacheHitRatio(usage))

		if data.response.StopSequence != nil {
			attrs.PutStr("gen_ai.response.stop_sequence", *data.response.StopSequence)
		}

		attrs.PutInt("anthropic.response.content_blocks_count", int64(len(data.response.Content)))
		attrs.PutInt("anthropic.response.text_length", int64(len(data.response.TextContent())))
		attrs.PutInt("anthropic.response.tool_calls_count", int64(len(data.response.ToolCalls())))
		attrs.PutInt("anthropic.response.thinking_length", int64(data.response.ThinkingLength()))
	}

	attrs.PutInt("http.response.status_code", int64(data.statusCode))
	attrs.PutInt("http.request.body.size", int64(data.requestSize))
	attrs.PutInt("http.response.body.size", int64(data.responseSize))

	if data.requestID != "" {
		attrs.PutStr("anthropic.request_id", data.requestID)
	}

	attrs.PutDouble("anthropic.upstream.latency_ms", float64(data.upstreamLatency.Milliseconds()))

	// Rate limit attributes
	if data.rateLimit.RequestsLimit > 0 {
		attrs.PutInt("anthropic.ratelimit.requests.limit", int64(data.rateLimit.RequestsLimit))
		attrs.PutInt("anthropic.ratelimit.requests.remaining", int64(data.rateLimit.RequestsRemaining))
		attrs.PutInt("anthropic.ratelimit.input_tokens.limit", int64(data.rateLimit.InputTokensLimit))
		attrs.PutInt("anthropic.ratelimit.input_tokens.remaining", int64(data.rateLimit.InputTokensRemaining))
		attrs.PutInt("anthropic.ratelimit.output_tokens.limit", int64(data.rateLimit.OutputTokensLimit))
		attrs.PutInt("anthropic.ratelimit.output_tokens.remaining", int64(data.rateLimit.OutputTokensRemaining))
	}

	// Rate limit reset times
	if data.rateLimit.RequestsReset != "" {
		attrs.PutStr("anthropic.ratelimit.requests.reset", data.rateLimit.RequestsReset)
	}
	if data.rateLimit.InputTokensReset != "" {
		attrs.PutStr("anthropic.ratelimit.input_tokens.reset", data.rateLimit.InputTokensReset)
	}
	if data.rateLimit.OutputTokensReset != "" {
		attrs.PutStr("anthropic.ratelimit.output_tokens.reset", data.rateLimit.OutputTokensReset)
	}

	// Unified token limits
	if data.rateLimit.TokensLimit > 0 {
		attrs.PutInt("anthropic.ratelimit.tokens.limit", int64(data.rateLimit.TokensLimit))
		attrs.PutInt("anthropic.ratelimit.tokens.remaining", int64(data.rateLimit.TokensRemaining))
	}

	// Retry after
	if data.rateLimit.RetryAfter != "" {
		attrs.PutStr("anthropic.ratelimit.retry_after", data.rateLimit.RetryAfter)
	}

	// Unified status
	if data.rateLimit.UnifiedStatus != "" {
		attrs.PutStr("anthropic.ratelimit.unified_status", data.rateLimit.UnifiedStatus)
	}

	// Speed
	if data.speed != "" {
		attrs.PutStr("anthropic.usage.speed", data.speed)
	}

	// Server tool use
	if data.response != nil && data.response.Usage.ServerToolUse != nil {
		stu := data.response.Usage.ServerToolUse
		attrs.PutInt("anthropic.usage.server_tool_use.web_search_requests", int64(stu.WebSearchRequests))
		attrs.PutInt("anthropic.usage.server_tool_use.web_fetch_requests", int64(stu.WebFetchRequests))
		attrs.PutInt("anthropic.usage.server_tool_use.code_execution_requests", int64(stu.CodeExecutionRequests))
	}

	// Beta features
	if data.betaFeatures != "" {
		attrs.PutStr("anthropic.request.beta_features", data.betaFeatures)
	}

	// Organization ID
	if data.organizationID != "" {
		attrs.PutStr("anthropic.organization_id", data.organizationID)
	}

	// Cost multiplier
	if data.cost.Multiplier != "" && data.cost.Multiplier != "standard" {
		attrs.PutStr("anthropic.cost.multiplier", data.cost.Multiplier)
	}

	// Streaming attributes
	if data.isStreaming && data.streaming != nil {
		if data.streaming.HasFirstToken {
			attrs.PutDouble("anthropic.streaming.time_to_first_token_ms", float64(data.streaming.TimeToFirstToken.Milliseconds()))
		}
		attrs.PutInt("anthropic.streaming.total_chunks", int64(data.streaming.TotalChunks))
		attrs.PutInt("anthropic.streaming.total_events", int64(data.streaming.TotalEvents))
		if data.streaming.AvgTimePerToken > 0 {
			attrs.PutDouble("anthropic.streaming.avg_time_per_token_ms", float64(data.streaming.AvgTimePerToken.Milliseconds()))
		}
	}
}

func (tb *telemetryBuilder) addSpanEvents(span ptrace.Span, data *requestData) {
	// Request event
	reqEvent := span.Events().AppendEmpty()
	reqEvent.SetName("gen_ai.request")
	reqEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.startTime))
	reqEvent.Attributes().PutStr("gen_ai.request.model", data.requestModel())
	if data.request != nil {
		reqEvent.Attributes().PutInt("messages_count", int64(len(data.request.Messages)))
	}

	// Response event
	if data.response != nil {
		respEvent := span.Events().AppendEmpty()
		respEvent.SetName("gen_ai.response")
		respEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
		respEvent.Attributes().PutStr("gen_ai.response.id", data.response.ID)
		finishReasons := respEvent.Attributes().PutEmptySlice("gen_ai.response.finish_reasons")
		finishReasons.AppendEmpty().SetStr(data.response.StopReason)

		// Content block events
		for i, block := range data.response.Content {
			blockEvent := span.Events().AppendEmpty()
			blockEvent.SetName("gen_ai.content_block")
			blockEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			blockEvent.Attributes().PutStr("type", block.Type)
			blockEvent.Attributes().PutInt("index", int64(i))
			switch block.Type {
			case "text":
				blockEvent.Attributes().PutInt("text_length", int64(len(block.Text)))
			case "tool_use":
				blockEvent.Attributes().PutStr("tool_name", block.Name)
			case "thinking":
				blockEvent.Attributes().PutInt("thinking_length", int64(len(block.Thinking)))
			case "server_tool_use":
				blockEvent.Attributes().PutStr("tool_name", block.Name)
			case "web_search_tool_result", "code_execution_tool_result":
				blockEvent.Attributes().PutInt("text_length", int64(len(block.Text)))
			}
		}

		// Tool call events
		for _, block := range data.response.ToolCalls() {
			tcEvent := span.Events().AppendEmpty()
			tcEvent.SetName("gen_ai.tool_call")
			tcEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			tcEvent.Attributes().PutStr("tool_name", block.Name)
			tcEvent.Attributes().PutStr("tool_call_id", block.ID)
		}

		// Thinking event
		for _, block := range data.response.ThinkingBlocks() {
			thinkEvent := span.Events().AppendEmpty()
			thinkEvent.SetName("gen_ai.thinking")
			thinkEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			thinkEvent.Attributes().PutInt("thinking_length", int64(len(block.Thinking)))
		}
	}

	// Error event
	if data.errorResponse != nil {
		errEvent := span.Events().AppendEmpty()
		errEvent.SetName("gen_ai.error")
		errEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
		errEvent.Attributes().PutStr("error.type", data.errorResponse.Error.Type)
		errEvent.Attributes().PutStr("error.message", data.errorResponse.Error.Message)
	}

	// Rate limit warning event
	if data.rateLimit.RequestsLimit > 0 {
		threshold := tb.cfg.RateLimitWarningThreshold
		if u := data.rateLimit.RequestsUtilization(); u > threshold {
			tb.addRateLimitEvent(span, "requests", u, data)
		}
		if u := data.rateLimit.InputTokensUtilization(); u > threshold {
			tb.addRateLimitEvent(span, "input_tokens", u, data)
		}
		if u := data.rateLimit.OutputTokensUtilization(); u > threshold {
			tb.addRateLimitEvent(span, "output_tokens", u, data)
		}
	}

	// Streaming events
	if data.isStreaming && data.streaming != nil {
		if data.streaming.HasFirstToken {
			ftEvent := span.Events().AppendEmpty()
			ftEvent.SetName("anthropic.stream.first_token")
			ftEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.startTime.Add(data.streaming.TimeToFirstToken)))
			ftEvent.Attributes().PutDouble("time_since_request_ms", float64(data.streaming.TimeToFirstToken.Milliseconds()))
		}

		completeEvent := span.Events().AppendEmpty()
		completeEvent.SetName("anthropic.stream.complete")
		completeEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
		completeEvent.Attributes().PutInt("total_events", int64(data.streaming.TotalEvents))
		completeEvent.Attributes().PutInt("total_chunks", int64(data.streaming.TotalChunks))
	}

	// Tool use file events
	for _, tc := range data.toolCalls {
		switch tc.ToolName {
		case "Edit":
			ev := span.Events().AppendEmpty()
			ev.SetName("anthropic.tool_use.file_edit")
			ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			ev.Attributes().PutStr("file.path", tc.FilePath)
			ev.Attributes().PutStr("file.extension", tc.FileExt)
			ev.Attributes().PutInt("lines_added", int64(tc.LinesAdded))
			ev.Attributes().PutInt("lines_removed", int64(tc.LinesRemoved))
		case "Write":
			ev := span.Events().AppendEmpty()
			ev.SetName("anthropic.tool_use.file_create")
			ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			ev.Attributes().PutStr("file.path", tc.FilePath)
			ev.Attributes().PutStr("file.extension", tc.FileExt)
			ev.Attributes().PutInt("content_size", int64(tc.WriteSize))
		case "Read":
			ev := span.Events().AppendEmpty()
			ev.SetName("anthropic.tool_use.file_read")
			ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			ev.Attributes().PutStr("file.path", tc.FilePath)
			ev.Attributes().PutStr("file.extension", tc.FileExt)
		case "Bash":
			ev := span.Events().AppendEmpty()
			ev.SetName("anthropic.tool_use.bash")
			ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			ev.Attributes().PutStr("command_preview", CommandPreview(tc.Command, 100))
		case "Glob":
			ev := span.Events().AppendEmpty()
			ev.SetName("anthropic.tool_use.glob")
			ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			ev.Attributes().PutStr("pattern", tc.Pattern)
			if tc.FilePath != "" {
				ev.Attributes().PutStr("file.path", tc.FilePath)
			}
		case "Grep":
			ev := span.Events().AppendEmpty()
			ev.SetName("anthropic.tool_use.grep")
			ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
			ev.Attributes().PutStr("pattern", tc.Pattern)
			if tc.FilePath != "" {
				ev.Attributes().PutStr("file.path", tc.FilePath)
			}
		}
	}

	// Cost event
	if data.cost.TotalCost > 0 {
		costEvent := span.Events().AppendEmpty()
		costEvent.SetName("anthropic.cost")
		costEvent.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
		costEvent.Attributes().PutDouble("cost.total", data.cost.TotalCost)
		costEvent.Attributes().PutDouble("cost.input", data.cost.InputCost)
		costEvent.Attributes().PutDouble("cost.output", data.cost.OutputCost)
		costEvent.Attributes().PutDouble("cost.cache", data.cost.CacheReadCost+data.cost.CacheCreationCost)
	}
}

func (tb *telemetryBuilder) addRateLimitEvent(span ptrace.Span, resource string, utilization float64, data *requestData) {
	ev := span.Events().AppendEmpty()
	ev.SetName("anthropic.rate_limit_warning")
	ev.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	ev.Attributes().PutStr("resource", resource)
	ev.Attributes().PutDouble("utilization", utilization)
}

// ---- Metrics ----

func (tb *telemetryBuilder) emitMetrics(ctx context.Context, data *requestData) error {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("github.com/guicaulada/anthropic-otel-collector/receiver/anthropicreceiver")

	model := data.requestModel()
	responseModel := ""
	if data.response != nil {
		responseModel = data.response.Model
	}

	commonAttrs := func() pcommon.Map {
		m := pcommon.NewMap()
		m.PutStr("gen_ai.operation.name", "chat")
		m.PutStr("gen_ai.provider.name", "anthropic")
		m.PutStr("gen_ai.request.model", model)
		if responseModel != "" {
			m.PutStr("gen_ai.response.model", responseModel)
		}
		m.PutInt("http.response.status_code", int64(data.statusCode))
		if data.statusCode >= 400 {
			if data.errorResponse != nil {
				m.PutStr("error.type", data.errorResponse.Error.Type)
			} else {
				m.PutStr("error.type", fmt.Sprintf("http_%d", data.statusCode))
			}
		}
		m.PutBool("anthropic.request.streaming", data.isStreaming)
		if data.apiKeyHash != "" {
			m.PutStr("anthropic.api_key_hash", data.apiKeyHash)
		}
		m.PutStr("server.address", tb.serverHost)
		m.PutStr("server.port", tb.serverPort)
		m.PutStr("http.request.method", "POST")
		return m
	}

	start := pcommon.NewTimestampFromTime(data.startTime)
	now := pcommon.NewTimestampFromTime(data.endTime)
	duration := data.endTime.Sub(data.startTime).Seconds()

	// 1. gen_ai.client.operation.duration
	tb.addHistogramDP(sm, "gen_ai.client.operation.duration", "s", start, now, duration, commonAttrs())

	// 2. gen_ai.client.token.usage (input)
	if data.response != nil {
		inputAttrs := commonAttrs()
		inputAttrs.PutStr("gen_ai.token.type", "input")
		tb.addHistogramDP(sm,"gen_ai.client.token.usage", "{token}", start, now,float64(data.response.Usage.InputTokens), inputAttrs)

		outputAttrs := commonAttrs()
		outputAttrs.PutStr("gen_ai.token.type", "output")
		tb.addHistogramDP(sm,"gen_ai.client.token.usage", "{token}", start, now,float64(data.response.Usage.OutputTokens), outputAttrs)
	}

	// 3. gen_ai.server.time_to_first_token (streaming)
	if data.isStreaming && data.streaming != nil && data.streaming.HasFirstToken {
		tb.addHistogramDP(sm,"gen_ai.server.time_to_first_token", "s", start, now,data.streaming.TimeToFirstToken.Seconds(), commonAttrs())
	}

	// 4. gen_ai.server.time_per_output_token (streaming)
	if data.isStreaming && data.streaming != nil && data.streaming.AvgTimePerToken > 0 {
		tb.addHistogramDP(sm,"gen_ai.server.time_per_output_token", "s", start, now,data.streaming.AvgTimePerToken.Seconds(), commonAttrs())
	}

	// 5. anthropic.requests
	tb.addSumDP(sm, "anthropic.requests", "{request}", start, now,1, commonAttrs())

	// 7. anthropic.errors
	if data.statusCode >= 400 {
		tb.addSumDP(sm, "anthropic.errors", "{error}", start, now,1, commonAttrs())
	}

	// 8. anthropic.request.body.size
	tb.addHistogramDP(sm,"anthropic.request.body.size", "By", start, now,float64(data.requestSize), commonAttrs())

	// 9. anthropic.response.body.size
	tb.addHistogramDP(sm,"anthropic.response.body.size", "By", start, now,float64(data.responseSize), commonAttrs())

	// 10. anthropic.upstream.latency
	tb.addHistogramDP(sm,"anthropic.upstream.latency", "s", start, now,data.upstreamLatency.Seconds(), commonAttrs())

	if data.response != nil {
		usage := data.response.Usage

		// 11-17. Token counters
		tb.addSumDP(sm, "anthropic.tokens.input", "{token}", start, now,int64(usage.InputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.output", "{token}", start, now,int64(usage.OutputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.cache_read", "{token}", start, now,int64(usage.CacheReadInputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.cache_creation", "{token}", start, now,int64(usage.CacheCreationInputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.total_input", "{token}", start, now,int64(usage.TotalInputTokens()), commonAttrs())

		// 18. anthropic.cache.hit_ratio
		tb.addGaugeDP(sm, "anthropic.cache.hit_ratio", "1", start, now,CacheHitRatio(usage), commonAttrs())

		// 33. anthropic.stop_reason
		stopAttrs := commonAttrs()
		stopAttrs.PutStr("stop_reason", data.response.StopReason)
		tb.addSumDP(sm, "anthropic.stop_reason", "{request}", start, now,1, stopAttrs)

		// 34. anthropic.tool_calls
		for _, block := range data.response.ToolCalls() {
			tcAttrs := commonAttrs()
			tcAttrs.PutStr("tool.name", block.Name)
			tb.addSumDP(sm, "anthropic.tool_calls", "{call}", start, now,1, tcAttrs)
		}

		// 35. anthropic.content_blocks
		for blockType, count := range data.response.ContentBlockCounts() {
			cbAttrs := commonAttrs()
			cbAttrs.PutStr("type", blockType)
			tb.addSumDP(sm, "anthropic.content_blocks", "{block}", start, now,int64(count), cbAttrs)
		}

		// 36. anthropic.response.text_length
		tb.addHistogramDP(sm,"anthropic.response.text_length", "{char}", start, now,float64(len(data.response.TextContent())), commonAttrs())

		// anthropic.thinking.output_length
		if thinkingLen := data.response.ThinkingLength(); thinkingLen > 0 {
			tb.addHistogramDP(sm,"anthropic.thinking.output_length", "{char}", start, now,float64(thinkingLen), commonAttrs())
		}
	}

	// Rate limit metrics (19-27)
	if data.rateLimit.RequestsLimit > 0 {
		tb.addGaugeDP(sm, "anthropic.ratelimit.requests.limit", "{request}", start, now,float64(data.rateLimit.RequestsLimit), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.requests.remaining", "{request}", start, now,float64(data.rateLimit.RequestsRemaining), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.requests.utilization", "1", start, now,data.rateLimit.RequestsUtilization(), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.input_tokens.limit", "{token}", start, now,float64(data.rateLimit.InputTokensLimit), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.input_tokens.remaining", "{token}", start, now,float64(data.rateLimit.InputTokensRemaining), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.input_tokens.utilization", "1", start, now,data.rateLimit.InputTokensUtilization(), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.limit", "{token}", start, now,float64(data.rateLimit.OutputTokensLimit), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.remaining", "{token}", start, now,float64(data.rateLimit.OutputTokensRemaining), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.utilization", "1", start, now,data.rateLimit.OutputTokensUtilization(), commonAttrs())
	}

	// Request parameter metrics (28-32)
	if data.request != nil {
		tb.addHistogramDP(sm,"anthropic.request.max_tokens", "{token}", start, now,float64(data.request.MaxTokens), commonAttrs())
		if data.request.Temperature != nil {
			tb.addHistogramDP(sm,"anthropic.request.temperature", "1", start, now,*data.request.Temperature, commonAttrs())
		}
		tb.addHistogramDP(sm,"anthropic.request.messages_count", "{message}", start, now,float64(len(data.request.Messages)), commonAttrs())
		tb.addHistogramDP(sm,"anthropic.request.system_prompt.size", "{char}", start, now,float64(data.request.SystemPromptSize()), commonAttrs())
		tb.addHistogramDP(sm,"anthropic.request.tools_count", "{tool}", start, now,float64(len(data.request.Tools)), commonAttrs())

		// 37-38. Thinking metrics
		if data.request.Thinking != nil {
			tb.addSumDP(sm, "anthropic.thinking.enabled", "{request}", start, now,1, commonAttrs())
			tb.addHistogramDP(sm,"anthropic.thinking.budget_tokens", "{token}", start, now,float64(data.request.Thinking.BudgetTokens), commonAttrs())
		}
	}

	// Streaming metrics (39-42)
	if data.isStreaming && data.streaming != nil {
		for eventType, count := range data.streaming.EventCounts {
			evAttrs := commonAttrs()
			evAttrs.PutStr("event_type", eventType)
			tb.addSumDP(sm, "anthropic.streaming.events", "{event}", start, now,int64(count), evAttrs)
		}
		tb.addHistogramDP(sm,"anthropic.streaming.duration", "s", start, now,data.streaming.Duration.Seconds(), commonAttrs())
		tb.addHistogramDP(sm,"anthropic.streaming.chunks", "{chunk}", start, now,float64(data.streaming.TotalChunks), commonAttrs())

		for _, bd := range data.streaming.BlockDurations {
			tb.addHistogramDP(sm,"anthropic.streaming.content_block.duration", "s", start, now,bd.Seconds(), commonAttrs())
		}
	}

	// Cost metrics (43-48)
	if data.cost.TotalCost > 0 {
		tb.addHistogramDP(sm,"anthropic.cost.request", "{USD}", start, now,data.cost.TotalCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.input_tokens", "{USD}", start, now,data.cost.InputCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.output_tokens", "{USD}", start, now,data.cost.OutputCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.cache_read", "{USD}", start, now,data.cost.CacheReadCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.cache_creation", "{USD}", start, now,data.cost.CacheCreationCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.total", "{USD}", start, now,data.cost.TotalCost, commonAttrs())
	}

	// Tool use metrics (49-60)
	for _, tc := range data.toolCalls {
		tcAttrs := commonAttrs()
		tcAttrs.PutStr("tool.name", tc.ToolName)
		if tc.FileExt != "" {
			tcAttrs.PutStr("file.extension", tc.FileExt)
		}
		if tb.cfg.IncludeFilePathLabel && tc.FilePath != "" {
			tcAttrs.PutStr("file.path", tc.FilePath)
		}

		// anthropic.tool_use.calls
		tb.addSumDP(sm, "anthropic.tool_use.calls", "{call}", start, now,1, tcAttrs)

		switch tc.ToolName {
		case "Edit":
			tb.addSumDP(sm, "anthropic.tool_use.file_edits", "{edit}", start, now,1, tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_added", "{line}", start, now,int64(tc.LinesAdded), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_removed", "{line}", start, now,int64(tc.LinesRemoved), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_changed", "{line}", start, now,int64(tc.LinesAdded+tc.LinesRemoved), tcAttrs)
			tb.addHistogramDP(sm,"anthropic.tool_use.edit_size", "{char}", start, now,float64(tc.EditSize), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.files_touched", "{file}", start, now,1, tcAttrs)
			if tc.FileExt != "" {
				ftAttrs := commonAttrs()
				ftAttrs.PutStr("file.extension", tc.FileExt)
				ftAttrs.PutStr("operation", "edit")
				tb.addSumDP(sm, "anthropic.tool_use.file_type", "{operation}", start, now,1, ftAttrs)
			}
		case "Write":
			tb.addSumDP(sm, "anthropic.tool_use.file_creates", "{file}", start, now,1, tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_added", "{line}", start, now,int64(tc.LinesAdded), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_changed", "{line}", start, now,int64(tc.LinesAdded), tcAttrs)
			tb.addHistogramDP(sm,"anthropic.tool_use.write_size", "{char}", start, now,float64(tc.WriteSize), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.files_touched", "{file}", start, now,1, tcAttrs)
			if tc.FileExt != "" {
				ftAttrs := commonAttrs()
				ftAttrs.PutStr("file.extension", tc.FileExt)
				ftAttrs.PutStr("operation", "write")
				tb.addSumDP(sm, "anthropic.tool_use.file_type", "{operation}", start, now,1, ftAttrs)
			}
		case "Read":
			tb.addSumDP(sm, "anthropic.tool_use.file_reads", "{read}", start, now,1, tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.files_touched", "{file}", start, now,1, tcAttrs)
			if tc.FileExt != "" {
				ftAttrs := commonAttrs()
				ftAttrs.PutStr("file.extension", tc.FileExt)
				ftAttrs.PutStr("operation", "read")
				tb.addSumDP(sm, "anthropic.tool_use.file_type", "{operation}", start, now,1, ftAttrs)
			}
		case "Bash":
			tb.addSumDP(sm, "anthropic.tool_use.bash_commands", "{command}", start, now,1, tcAttrs)
		case "Glob":
			tb.addSumDP(sm, "anthropic.tool_use.glob_searches", "{search}", start, now,1, tcAttrs)
		case "Grep":
			tb.addSumDP(sm, "anthropic.tool_use.grep_searches", "{search}", start, now,1, tcAttrs)
		}
	}

	// Server tool use metrics
	if data.response != nil && data.response.Usage.ServerToolUse != nil {
		stu := data.response.Usage.ServerToolUse
		if stu.WebSearchRequests > 0 {
			tb.addSumDP(sm, "anthropic.server_tool_use.web_search_requests", "{request}", start, now, int64(stu.WebSearchRequests), commonAttrs())
			// Web search cost: $10 per 1000 searches
			searchCost := float64(stu.WebSearchRequests) * 10.0 / 1000.0
			tb.addSumDPf(sm, "anthropic.cost.server_tool_use.web_search", "{USD}", start, now, searchCost, commonAttrs())
		}
		if stu.WebFetchRequests > 0 {
			tb.addSumDP(sm, "anthropic.server_tool_use.web_fetch_requests", "{request}", start, now, int64(stu.WebFetchRequests), commonAttrs())
		}
		if stu.CodeExecutionRequests > 0 {
			tb.addSumDP(sm, "anthropic.server_tool_use.code_execution_requests", "{request}", start, now, int64(stu.CodeExecutionRequests), commonAttrs())
		}
	}

	// Speed breakdown metric
	if data.speed != "" {
		speedAttrs := commonAttrs()
		speedAttrs.PutStr("speed", data.speed)
		tb.addSumDP(sm, "anthropic.requests.by_speed", "{request}", start, now, 1, speedAttrs)
	}

	// Output tokens per second (throughput gauge)
	if data.response != nil && data.response.Usage.OutputTokens > 0 {
		var outputDuration time.Duration
		if data.isStreaming && data.streaming != nil && data.streaming.HasFirstToken {
			outputDuration = data.streaming.Duration - data.streaming.TimeToFirstToken
		} else {
			outputDuration = data.endTime.Sub(data.startTime)
		}
		if outputDuration > 0 {
			tokensPerSec := float64(data.response.Usage.OutputTokens) / outputDuration.Seconds()
			tb.addGaugeDP(sm, "anthropic.throughput.output_tokens_per_second", "{token}/s", start, now, tokensPerSec, commonAttrs())
		}
	}

	// Cost multiplied requests counter
	if data.cost.Multiplier != "" && data.cost.Multiplier != "standard" {
		multAttrs := commonAttrs()
		multAttrs.PutStr("multiplier", data.cost.Multiplier)
		tb.addSumDP(sm, "anthropic.cost.multiplied_requests", "{request}", start, now, 1, multAttrs)
	}

	return tb.metricsConsumer.ConsumeMetrics(ctx, md)
}

func (tb *telemetryBuilder) addHistogramDP(sm pmetric.ScopeMetrics, name, unit string, startTs, ts pcommon.Timestamp, value float64, attrs pcommon.Map) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp := h.DataPoints().AppendEmpty()
	dp.SetStartTimestamp(startTs)
	dp.SetTimestamp(ts)
	dp.SetCount(1)
	dp.SetSum(value)
	dp.SetMin(value)
	dp.SetMax(value)
	attrs.CopyTo(dp.Attributes())
}

func (tb *telemetryBuilder) addSumDP(sm pmetric.ScopeMetrics, name, unit string, startTs, ts pcommon.Timestamp, value int64, attrs pcommon.Map) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	s := m.SetEmptySum()
	s.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	s.SetIsMonotonic(true)
	dp := s.DataPoints().AppendEmpty()
	dp.SetStartTimestamp(startTs)
	dp.SetTimestamp(ts)
	dp.SetIntValue(value)
	attrs.CopyTo(dp.Attributes())
}

func (tb *telemetryBuilder) addSumDPf(sm pmetric.ScopeMetrics, name, unit string, startTs, ts pcommon.Timestamp, value float64, attrs pcommon.Map) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	s := m.SetEmptySum()
	s.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	s.SetIsMonotonic(true)
	dp := s.DataPoints().AppendEmpty()
	dp.SetStartTimestamp(startTs)
	dp.SetTimestamp(ts)
	dp.SetDoubleValue(value)
	attrs.CopyTo(dp.Attributes())
}

func (tb *telemetryBuilder) addGaugeDP(sm pmetric.ScopeMetrics, name, unit string, startTs, ts pcommon.Timestamp, value float64, attrs pcommon.Map) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	g := m.SetEmptyGauge()
	dp := g.DataPoints().AppendEmpty()
	dp.SetStartTimestamp(startTs)
	dp.SetTimestamp(ts)
	dp.SetDoubleValue(value)
	attrs.CopyTo(dp.Attributes())
}

// ---- Logs ----

func (tb *telemetryBuilder) emitLogs(ctx context.Context, data *requestData) error {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("github.com/guicaulada/anthropic-otel-collector/receiver/anthropicreceiver")

	// Log 1: Request/Response detail
	tb.addOperationLog(sl, data)

	// Log 2: Request body (opt-in)
	if tb.cfg.CaptureRequestBody && len(data.requestBody) > 0 {
		tb.addBodyLog(sl, "gen_ai.request.body", plog.SeverityNumberDebug, data.requestBody, data)
	}

	// Log 3: Response body (opt-in)
	if tb.cfg.CaptureResponseBody && len(data.responseBody) > 0 {
		tb.addBodyLog(sl, "gen_ai.response.body", plog.SeverityNumberDebug, data.responseBody, data)
	}

	// Log 4: Error detail
	if data.statusCode >= 400 && data.errorResponse != nil {
		tb.addErrorLog(sl, data)
	}

	// Log 5: Rate limit warnings
	if data.rateLimit.RequestsLimit > 0 {
		threshold := tb.cfg.RateLimitWarningThreshold
		if u := data.rateLimit.RequestsUtilization(); u > threshold {
			tb.addRateLimitLog(sl, "requests", u, data)
		}
		if u := data.rateLimit.InputTokensUtilization(); u > threshold {
			tb.addRateLimitLog(sl, "input_tokens", u, data)
		}
		if u := data.rateLimit.OutputTokensUtilization(); u > threshold {
			tb.addRateLimitLog(sl, "output_tokens", u, data)
		}
	}

	// Log 6: Tool call logs (per tool_use block)
	if data.response != nil {
		for _, block := range data.response.ToolCalls() {
			tb.addToolCallLog(sl, block, data)
		}
	}

	// Log 7: Detailed tool call logs (parsed)
	for _, tc := range data.toolCalls {
		tb.addDetailedToolCallLog(sl, tc, data)
	}

	// Log 8: File change logs
	for _, tc := range data.toolCalls {
		if tc.ToolName == "Edit" || tc.ToolName == "Write" {
			tb.addFileChangeLog(sl, tc, data)
		}
	}

	// Log 9: Cost summary
	if data.cost.TotalCost > 0 {
		tb.addCostLog(sl, data)
	}

	// Log 10: Streaming summary
	if data.isStreaming && data.streaming != nil {
		tb.addStreamingSummaryLog(sl, data)
	}

	// Log 11: Notable stop reasons
	if data.response != nil {
		tb.addNotableStopReasonLog(sl, data)
	}

	return tb.logsConsumer.ConsumeLogs(ctx, ld)
}

func (tb *telemetryBuilder) addOperationLog(sl plog.ScopeLogs, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))

	if data.statusCode >= 400 {
		lr.SetSeverityNumber(plog.SeverityNumberError)
		lr.SetSeverityText("ERROR")
	} else {
		lr.SetSeverityNumber(plog.SeverityNumberInfo)
		lr.SetSeverityText("INFO")
	}

	body := lr.Body().SetEmptyMap()
	body.PutStr("gen_ai.operation.name", "chat")
	body.PutStr("gen_ai.provider.name", "anthropic")
	body.PutStr("gen_ai.request.model", data.requestModel())
	body.PutBool("anthropic.request.streaming", data.isStreaming)
	body.PutDouble("gen_ai.client.operation.duration_s", data.endTime.Sub(data.startTime).Seconds())
	body.PutInt("http.response.status_code", int64(data.statusCode))
	body.PutInt("http.request.body.size", int64(data.requestSize))
	body.PutInt("http.response.body.size", int64(data.responseSize))

	if data.requestID != "" {
		body.PutStr("anthropic.request_id", data.requestID)
	}

	if data.request != nil {
		body.PutInt("anthropic.request.messages_count", int64(len(data.request.Messages)))
		for role, count := range data.request.MessageRoleCounts() {
			body.PutInt("anthropic.request."+role+"_messages_count", int64(count))
		}
		body.PutInt("anthropic.request.max_tokens", int64(data.request.MaxTokens))
		body.PutInt("anthropic.request.tools_count", int64(len(data.request.Tools)))
	}

	if data.response != nil {
		body.PutStr("gen_ai.response.model", data.response.Model)
		body.PutStr("gen_ai.response.id", data.response.ID)
		body.PutStr("gen_ai.response.finish_reason", data.response.StopReason)
		if data.response.StopSequence != nil {
			body.PutStr("gen_ai.response.stop_sequence", *data.response.StopSequence)
		}

		usage := data.response.Usage
		body.PutInt("gen_ai.usage.input_tokens", int64(usage.InputTokens))
		body.PutInt("gen_ai.usage.output_tokens", int64(usage.OutputTokens))
		body.PutInt("gen_ai.usage.cache_read.input_tokens", int64(usage.CacheReadInputTokens))
		body.PutInt("gen_ai.usage.cache_creation.input_tokens", int64(usage.CacheCreationInputTokens))
		body.PutInt("anthropic.usage.total_input_tokens", int64(usage.TotalInputTokens()))
		body.PutDouble("anthropic.cache.hit_ratio", CacheHitRatio(usage))

		body.PutInt("anthropic.response.content_blocks_count", int64(len(data.response.Content)))
		body.PutInt("anthropic.response.tool_calls_count", int64(len(data.response.ToolCalls())))
		body.PutInt("anthropic.response.text_length", int64(len(data.response.TextContent())))
		body.PutInt("anthropic.response.thinking_length", int64(data.response.ThinkingLength()))

		if usage.ServerToolUse != nil {
			stuMap := body.PutEmptyMap("anthropic.usage.server_tool_use")
			stuMap.PutInt("web_search_requests", int64(usage.ServerToolUse.WebSearchRequests))
			stuMap.PutInt("web_fetch_requests", int64(usage.ServerToolUse.WebFetchRequests))
			stuMap.PutInt("code_execution_requests", int64(usage.ServerToolUse.CodeExecutionRequests))
		}
	}

	if data.speed != "" {
		body.PutStr("anthropic.usage.speed", data.speed)
	}
	if data.betaFeatures != "" {
		body.PutStr("anthropic.request.beta_features", data.betaFeatures)
	}
	if data.organizationID != "" {
		body.PutStr("anthropic.organization_id", data.organizationID)
	}
	if data.cost.Multiplier != "" && data.cost.Multiplier != "standard" {
		body.PutStr("anthropic.cost.multiplier", data.cost.Multiplier)
	}

	// Attributes: rate limits + api key hash
	attrs := lr.Attributes()
	if data.apiKeyHash != "" {
		attrs.PutStr("anthropic.api_key_hash", data.apiKeyHash)
	}
	if data.rateLimit.RequestsLimit > 0 {
		attrs.PutInt("anthropic.ratelimit.requests.limit", int64(data.rateLimit.RequestsLimit))
		attrs.PutInt("anthropic.ratelimit.requests.remaining", int64(data.rateLimit.RequestsRemaining))
		attrs.PutInt("anthropic.ratelimit.input_tokens.limit", int64(data.rateLimit.InputTokensLimit))
		attrs.PutInt("anthropic.ratelimit.input_tokens.remaining", int64(data.rateLimit.InputTokensRemaining))
		attrs.PutInt("anthropic.ratelimit.output_tokens.limit", int64(data.rateLimit.OutputTokensLimit))
		attrs.PutInt("anthropic.ratelimit.output_tokens.remaining", int64(data.rateLimit.OutputTokensRemaining))
	}
}

func (tb *telemetryBuilder) addBodyLog(sl plog.ScopeLogs, eventName string, severity plog.SeverityNumber, body []byte, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(severity)
	lr.SetSeverityText(severity.String())

	captured := body
	if len(captured) > tb.cfg.MaxBodyCaptureSize {
		captured = captured[:tb.cfg.MaxBodyCaptureSize]
	}
	lr.Body().SetStr(string(captured))
	lr.Attributes().PutStr("event.name", eventName)
	lr.Attributes().PutStr("gen_ai.request.model", data.requestModel())
}

func (tb *telemetryBuilder) addErrorLog(sl plog.ScopeLogs, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberError)
	lr.SetSeverityText("ERROR")

	lr.Body().SetStr(fmt.Sprintf("%s: %s", data.errorResponse.Error.Type, data.errorResponse.Error.Message))

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "gen_ai.error")
	attrs.PutStr("error.type", data.errorResponse.Error.Type)
	attrs.PutStr("error.message", data.errorResponse.Error.Message)
	attrs.PutInt("http.response.status_code", int64(data.statusCode))
	if data.requestID != "" {
		attrs.PutStr("anthropic.request_id", data.requestID)
	}
	attrs.PutStr("gen_ai.request.model", data.requestModel())
}

func (tb *telemetryBuilder) addRateLimitLog(sl plog.ScopeLogs, resource string, utilization float64, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberWarn)
	lr.SetSeverityText("WARN")

	lr.Body().SetStr(fmt.Sprintf("Rate limit warning: %s at %.0f%% utilization", resource, utilization*100))

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.rate_limit_warning")
	attrs.PutStr("resource", resource)
	attrs.PutDouble("utilization", utilization)

	var limit, remaining int
	switch resource {
	case "requests":
		limit = data.rateLimit.RequestsLimit
		remaining = data.rateLimit.RequestsRemaining
	case "input_tokens":
		limit = data.rateLimit.InputTokensLimit
		remaining = data.rateLimit.InputTokensRemaining
	case "output_tokens":
		limit = data.rateLimit.OutputTokensLimit
		remaining = data.rateLimit.OutputTokensRemaining
	}
	attrs.PutInt("limit", int64(limit))
	attrs.PutInt("remaining", int64(remaining))
}

func (tb *telemetryBuilder) addToolCallLog(sl plog.ScopeLogs, block ContentBlock, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")

	lr.Body().SetStr(fmt.Sprintf("Tool call: %s", block.Name))

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "gen_ai.tool_call")
	attrs.PutStr("tool.name", block.Name)
	attrs.PutStr("tool.call_id", block.ID)
	attrs.PutInt("tool.input_size", int64(len(block.Input)))
	attrs.PutStr("gen_ai.request.model", data.requestModel())
}

func (tb *telemetryBuilder) addDetailedToolCallLog(sl plog.ScopeLogs, tc ToolCallInfo, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")

	bodyStr := fmt.Sprintf("Tool call: %s", tc.ToolName)
	if tc.FilePath != "" {
		bodyStr += fmt.Sprintf(" on %s", tc.FilePath)
	}
	lr.Body().SetStr(bodyStr)

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.tool_call")
	attrs.PutStr("tool.name", tc.ToolName)
	attrs.PutStr("tool.call_id", tc.ToolCallID)
	if tc.FilePath != "" {
		attrs.PutStr("file.path", tc.FilePath)
	}
	if tc.FileExt != "" {
		attrs.PutStr("file.extension", tc.FileExt)
	}
	if tc.Pattern != "" {
		attrs.PutStr("pattern", tc.Pattern)
	}
	if tc.LinesAdded > 0 {
		attrs.PutInt("lines_added", int64(tc.LinesAdded))
	}
	if tc.LinesRemoved > 0 {
		attrs.PutInt("lines_removed", int64(tc.LinesRemoved))
	}
	if tc.EditSize > 0 {
		attrs.PutInt("edit_size", int64(tc.EditSize))
	}
	attrs.PutStr("gen_ai.request.model", data.requestModel())
}

func (tb *telemetryBuilder) addFileChangeLog(sl plog.ScopeLogs, tc ToolCallInfo, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")

	lr.Body().SetStr(fmt.Sprintf("File change: %s +%d -%d", tc.FilePath, tc.LinesAdded, tc.LinesRemoved))

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.file_change")
	attrs.PutStr("file.path", tc.FilePath)
	if tc.FileExt != "" {
		attrs.PutStr("file.extension", tc.FileExt)
	}
	attrs.PutInt("lines_added", int64(tc.LinesAdded))
	attrs.PutInt("lines_removed", int64(tc.LinesRemoved))
	if tc.ToolName == "Write" {
		attrs.PutStr("operation", "create")
	} else {
		attrs.PutStr("operation", "edit")
	}
}

func (tb *telemetryBuilder) addCostLog(sl plog.ScopeLogs, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")

	cacheCost := data.cost.CacheReadCost + data.cost.CacheCreationCost
	lr.Body().SetStr(fmt.Sprintf("Cost: $%.6f (input: $%.6f, output: $%.6f, cache: $%.6f)",
		data.cost.TotalCost, data.cost.InputCost, data.cost.OutputCost, cacheCost))

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.cost")
	attrs.PutDouble("cost.total", data.cost.TotalCost)
	attrs.PutDouble("cost.input_tokens", data.cost.InputCost)
	attrs.PutDouble("cost.output_tokens", data.cost.OutputCost)
	attrs.PutDouble("cost.cache_read", data.cost.CacheReadCost)
	attrs.PutDouble("cost.cache_creation", data.cost.CacheCreationCost)
	attrs.PutStr("gen_ai.request.model", data.requestModel())
}

func (tb *telemetryBuilder) addStreamingSummaryLog(sl plog.ScopeLogs, data *requestData) {
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")

	ttftMs := int64(0)
	if data.streaming.HasFirstToken {
		ttftMs = data.streaming.TimeToFirstToken.Milliseconds()
	}

	lr.Body().SetStr(fmt.Sprintf("Streaming complete: %d events, %d chunks, TTFT=%dms",
		data.streaming.TotalEvents, data.streaming.TotalChunks, ttftMs))

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.streaming.summary")
	attrs.PutInt("anthropic.streaming.total_events", int64(data.streaming.TotalEvents))
	attrs.PutInt("anthropic.streaming.total_chunks", int64(data.streaming.TotalChunks))
	attrs.PutDouble("anthropic.streaming.time_to_first_token_ms", float64(ttftMs))
	attrs.PutDouble("anthropic.streaming.duration_s", data.streaming.Duration.Seconds())
	if data.streaming.AvgTimePerToken > 0 {
		attrs.PutDouble("anthropic.streaming.avg_time_per_token_ms", float64(data.streaming.AvgTimePerToken.Milliseconds()))
	}
}

func (tb *telemetryBuilder) addNotableStopReasonLog(sl plog.ScopeLogs, data *requestData) {
	var message string
	switch data.response.StopReason {
	case "refusal":
		message = "Safety refusal: model refused to generate content"
	case "pause_turn":
		message = "Turn paused: server tool iteration limit reached"
	case "model_context_window_exceeded":
		message = "Context window exceeded"
	default:
		return
	}

	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(data.endTime))
	lr.SetSeverityNumber(plog.SeverityNumberWarn)
	lr.SetSeverityText("WARN")
	lr.Body().SetStr(message)

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.notable_stop_reason")
	attrs.PutStr("stop_reason", data.response.StopReason)
	attrs.PutStr("gen_ai.request.model", data.requestModel())
	if data.requestID != "" {
		attrs.PutStr("anthropic.request_id", data.requestID)
	}
}

// ---- Helpers ----

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
