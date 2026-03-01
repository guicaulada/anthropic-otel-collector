package anthropicreceiver

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func (tb *telemetryBuilder) emitTraces(ctx context.Context, data *requestData) error {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	setResourceAttributes(rs.Resource().Attributes())
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
		rootSpan.Status().SetCode(ptrace.StatusCodeUnset)
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
	upAttrs.PutInt("server.port", int64(tb.serverPort))
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
	attrs.PutInt("server.port", int64(tb.serverPort))
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
		if tc := data.request.ToolChoiceType(); tc != "" {
			attrs.PutStr("anthropic.request.tool_choice", tc)
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
	if data.statusCode >= 400 {
		if data.errorResponse != nil {
			attrs.PutStr("error.type", data.errorResponse.Error.Type)
		} else {
			attrs.PutStr("error.type", fmt.Sprintf("http_%d", data.statusCode))
		}
	}
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

	// API version
	if data.apiVersion != "" {
		attrs.PutStr("anthropic.request.api_version", data.apiVersion)
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
