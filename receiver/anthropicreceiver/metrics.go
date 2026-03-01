package anthropicreceiver

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func (tb *telemetryBuilder) emitMetrics(ctx context.Context, data *requestData) error {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	setResourceAttributes(rm.Resource().Attributes())
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
		m.PutInt("server.port", int64(tb.serverPort))
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
		tb.addHistogramDP(sm, "gen_ai.client.token.usage", "{token}", start, now, float64(data.response.Usage.InputTokens), inputAttrs)

		outputAttrs := commonAttrs()
		outputAttrs.PutStr("gen_ai.token.type", "output")
		tb.addHistogramDP(sm, "gen_ai.client.token.usage", "{token}", start, now, float64(data.response.Usage.OutputTokens), outputAttrs)
	}

	// 3. gen_ai.server.time_to_first_token (streaming)
	if data.isStreaming && data.streaming != nil && data.streaming.HasFirstToken {
		tb.addHistogramDP(sm, "gen_ai.server.time_to_first_token", "s", start, now, data.streaming.TimeToFirstToken.Seconds(), commonAttrs())
	}

	// 4. gen_ai.server.time_per_output_token (streaming)
	if data.isStreaming && data.streaming != nil && data.streaming.AvgTimePerToken > 0 {
		tb.addHistogramDP(sm, "gen_ai.server.time_per_output_token", "s", start, now, data.streaming.AvgTimePerToken.Seconds(), commonAttrs())
	}

	// 5. anthropic.requests
	tb.addSumDP(sm, "anthropic.requests", "{request}", start, now, 1, commonAttrs())

	// 7. anthropic.errors
	if data.statusCode >= 400 {
		tb.addSumDP(sm, "anthropic.errors", "{error}", start, now, 1, commonAttrs())
	}

	// 8. anthropic.request.body.size
	tb.addHistogramDP(sm, "anthropic.request.body.size", "By", start, now, float64(data.requestSize), commonAttrs())

	// 9. anthropic.response.body.size
	tb.addHistogramDP(sm, "anthropic.response.body.size", "By", start, now, float64(data.responseSize), commonAttrs())

	// 10. anthropic.upstream.latency
	tb.addHistogramDP(sm, "anthropic.upstream.latency", "s", start, now, data.upstreamLatency.Seconds(), commonAttrs())

	if data.response != nil {
		usage := data.response.Usage

		// 11-17. Token counters
		tb.addSumDP(sm, "anthropic.tokens.input", "{token}", start, now, int64(usage.InputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.output", "{token}", start, now, int64(usage.OutputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.cache_read", "{token}", start, now, int64(usage.CacheReadInputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.cache_creation", "{token}", start, now, int64(usage.CacheCreationInputTokens), commonAttrs())
		tb.addSumDP(sm, "anthropic.tokens.total_input", "{token}", start, now, int64(usage.TotalInputTokens()), commonAttrs())

		// 18. anthropic.cache.hit_ratio
		tb.addGaugeDP(sm, "anthropic.cache.hit_ratio", "1", start, now, CacheHitRatio(usage), commonAttrs())

		// 33. anthropic.stop_reason
		stopAttrs := commonAttrs()
		stopAttrs.PutStr("stop_reason", data.response.StopReason)
		tb.addSumDP(sm, "anthropic.stop_reason", "{request}", start, now, 1, stopAttrs)

		// 34. anthropic.tool_calls
		for _, block := range data.response.ToolCalls() {
			tcAttrs := commonAttrs()
			tcAttrs.PutStr("tool.name", block.Name)
			tb.addSumDP(sm, "anthropic.tool_calls", "{call}", start, now, 1, tcAttrs)
		}

		// 35. anthropic.content_blocks
		for blockType, count := range data.response.ContentBlockCounts() {
			cbAttrs := commonAttrs()
			cbAttrs.PutStr("type", blockType)
			tb.addSumDP(sm, "anthropic.content_blocks", "{block}", start, now, int64(count), cbAttrs)
		}

		// 36. anthropic.response.text_length
		tb.addHistogramDP(sm, "anthropic.response.text_length", "{char}", start, now, float64(len(data.response.TextContent())), commonAttrs())

		// anthropic.thinking.output_length
		if thinkingLen := data.response.ThinkingLength(); thinkingLen > 0 {
			tb.addHistogramDP(sm, "anthropic.thinking.output_length", "{char}", start, now, float64(thinkingLen), commonAttrs())
		}
	}

	// Rate limit metrics (19-27)
	if data.rateLimit.RequestsLimit > 0 {
		tb.addGaugeDP(sm, "anthropic.ratelimit.requests.limit", "{request}", start, now, float64(data.rateLimit.RequestsLimit), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.requests.remaining", "{request}", start, now, float64(data.rateLimit.RequestsRemaining), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.requests.utilization", "1", start, now, data.rateLimit.RequestsUtilization(), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.input_tokens.limit", "{token}", start, now, float64(data.rateLimit.InputTokensLimit), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.input_tokens.remaining", "{token}", start, now, float64(data.rateLimit.InputTokensRemaining), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.input_tokens.utilization", "1", start, now, data.rateLimit.InputTokensUtilization(), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.limit", "{token}", start, now, float64(data.rateLimit.OutputTokensLimit), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.remaining", "{token}", start, now, float64(data.rateLimit.OutputTokensRemaining), commonAttrs())
		tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.utilization", "1", start, now, data.rateLimit.OutputTokensUtilization(), commonAttrs())
	}

	// Request parameter metrics (28-32)
	if data.request != nil {
		tb.addHistogramDP(sm, "anthropic.request.max_tokens", "{token}", start, now, float64(data.request.MaxTokens), commonAttrs())
		if data.request.Temperature != nil {
			tb.addHistogramDP(sm, "anthropic.request.temperature", "1", start, now, *data.request.Temperature, commonAttrs())
		}
		tb.addHistogramDP(sm, "anthropic.request.messages_count", "{message}", start, now, float64(len(data.request.Messages)), commonAttrs())
		tb.addHistogramDP(sm, "anthropic.request.system_prompt.size", "{char}", start, now, float64(data.request.SystemPromptSize()), commonAttrs())
		tb.addHistogramDP(sm, "anthropic.request.tools_count", "{tool}", start, now, float64(len(data.request.Tools)), commonAttrs())

		// 37-38. Thinking metrics
		if data.request.Thinking != nil {
			tb.addSumDP(sm, "anthropic.thinking.enabled", "{request}", start, now, 1, commonAttrs())
			tb.addHistogramDP(sm, "anthropic.thinking.budget_tokens", "{token}", start, now, float64(data.request.Thinking.BudgetTokens), commonAttrs())
		}
	}

	// Streaming metrics (39-42)
	if data.isStreaming && data.streaming != nil {
		for eventType, count := range data.streaming.EventCounts {
			evAttrs := commonAttrs()
			evAttrs.PutStr("event_type", eventType)
			tb.addSumDP(sm, "anthropic.streaming.events", "{event}", start, now, int64(count), evAttrs)
		}
		tb.addHistogramDP(sm, "anthropic.streaming.duration", "s", start, now, data.streaming.Duration.Seconds(), commonAttrs())
		tb.addHistogramDP(sm, "anthropic.streaming.chunks", "{chunk}", start, now, float64(data.streaming.TotalChunks), commonAttrs())

		for _, bd := range data.streaming.BlockDurations {
			tb.addHistogramDP(sm, "anthropic.streaming.content_block.duration", "s", start, now, bd.Seconds(), commonAttrs())
		}
	}

	// Cost metrics (43-48)
	if data.cost.TotalCost > 0 {
		tb.addHistogramDP(sm, "anthropic.cost.request", "{USD}", start, now, data.cost.TotalCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.input_tokens", "{USD}", start, now, data.cost.InputCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.output_tokens", "{USD}", start, now, data.cost.OutputCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.cache_read", "{USD}", start, now, data.cost.CacheReadCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.cache_creation", "{USD}", start, now, data.cost.CacheCreationCost, commonAttrs())
		tb.addSumDPf(sm, "anthropic.cost.total", "{USD}", start, now, data.cost.TotalCost, commonAttrs())
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
		tb.addSumDP(sm, "anthropic.tool_use.calls", "{call}", start, now, 1, tcAttrs)

		switch tc.ToolName {
		case "Edit":
			tb.addSumDP(sm, "anthropic.tool_use.file_edits", "{edit}", start, now, 1, tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_added", "{line}", start, now, int64(tc.LinesAdded), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_removed", "{line}", start, now, int64(tc.LinesRemoved), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_changed", "{line}", start, now, int64(tc.LinesAdded+tc.LinesRemoved), tcAttrs)
			tb.addHistogramDP(sm, "anthropic.tool_use.edit_size", "{char}", start, now, float64(tc.EditSize), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.files_touched", "{file}", start, now, 1, tcAttrs)
			if tc.FileExt != "" {
				ftAttrs := commonAttrs()
				ftAttrs.PutStr("file.extension", tc.FileExt)
				ftAttrs.PutStr("operation", "edit")
				tb.addSumDP(sm, "anthropic.tool_use.file_type", "{operation}", start, now, 1, ftAttrs)
			}
		case "Write":
			tb.addSumDP(sm, "anthropic.tool_use.file_creates", "{file}", start, now, 1, tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_added", "{line}", start, now, int64(tc.LinesAdded), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.lines_changed", "{line}", start, now, int64(tc.LinesAdded), tcAttrs)
			tb.addHistogramDP(sm, "anthropic.tool_use.write_size", "{char}", start, now, float64(tc.WriteSize), tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.files_touched", "{file}", start, now, 1, tcAttrs)
			if tc.FileExt != "" {
				ftAttrs := commonAttrs()
				ftAttrs.PutStr("file.extension", tc.FileExt)
				ftAttrs.PutStr("operation", "write")
				tb.addSumDP(sm, "anthropic.tool_use.file_type", "{operation}", start, now, 1, ftAttrs)
			}
		case "Read":
			tb.addSumDP(sm, "anthropic.tool_use.file_reads", "{read}", start, now, 1, tcAttrs)
			tb.addSumDP(sm, "anthropic.tool_use.files_touched", "{file}", start, now, 1, tcAttrs)
			if tc.FileExt != "" {
				ftAttrs := commonAttrs()
				ftAttrs.PutStr("file.extension", tc.FileExt)
				ftAttrs.PutStr("operation", "read")
				tb.addSumDP(sm, "anthropic.tool_use.file_type", "{operation}", start, now, 1, ftAttrs)
			}
		case "Bash":
			tb.addSumDP(sm, "anthropic.tool_use.bash_commands", "{command}", start, now, 1, tcAttrs)
		case "Glob":
			tb.addSumDP(sm, "anthropic.tool_use.glob_searches", "{search}", start, now, 1, tcAttrs)
		case "Grep":
			tb.addSumDP(sm, "anthropic.tool_use.grep_searches", "{search}", start, now, 1, tcAttrs)
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
