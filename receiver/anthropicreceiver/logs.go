package anthropicreceiver

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func (tb *telemetryBuilder) emitLogs(ctx context.Context, data *requestData) error {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	setResourceAttributes(rl.Resource().Attributes())
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
		if tc := data.request.ToolChoiceType(); tc != "" {
			body.PutStr("anthropic.request.tool_choice", tc)
		}
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
	if data.apiVersion != "" {
		body.PutStr("anthropic.request.api_version", data.apiVersion)
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

	lr.Body().SetStr("API error")

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

	lr.Body().SetStr("Rate limit warning")

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

	lr.Body().SetStr("Tool call")

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

	lr.Body().SetStr("Tool call detail")

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

	lr.Body().SetStr("File change")

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

	lr.Body().SetStr("Cost summary")

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

	lr.Body().SetStr("Streaming summary")

	ttftMs := int64(0)
	if data.streaming.HasFirstToken {
		ttftMs = data.streaming.TimeToFirstToken.Milliseconds()
	}

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
	lr.Body().SetStr("Notable stop reason")

	attrs := lr.Attributes()
	attrs.PutStr("event.name", "anthropic.notable_stop_reason")
	attrs.PutStr("message", message)
	attrs.PutStr("stop_reason", data.response.StopReason)
	attrs.PutStr("gen_ai.request.model", data.requestModel())
	if data.requestID != "" {
		attrs.PutStr("anthropic.request_id", data.requestID)
	}
}
