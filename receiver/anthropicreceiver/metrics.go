package anthropicreceiver

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// histogramBuckets maps metric names to explicit histogram bucket boundaries.
// These enable histogram_quantile() in Prometheus by providing the bucket
// structure needed for quantile estimation.
var histogramBuckets = map[string][]float64{
	// Duration metrics (seconds)
	"gen_ai.client.operation.duration":             {0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	"anthropic.upstream.latency":                   {0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	"anthropic.streaming.duration":                 {0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	"anthropic.streaming.content_block.duration":   {0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},

	// Time to first token (seconds)
	"gen_ai.server.time_to_first_token": {0.1, 0.25, 0.5, 1, 2, 5, 10},

	// Time per output token (seconds)
	"gen_ai.server.time_per_output_token": {0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},

	// Body size (bytes)
	"anthropic.request.body.size":  {256, 1024, 4096, 16384, 65536, 262144, 1048576},
	"anthropic.response.body.size": {256, 1024, 4096, 16384, 65536, 262144, 1048576},

	// Token counts
	"anthropic.request.max_tokens":     {10, 50, 100, 500, 1000, 5000, 10000, 50000, 100000},
	"anthropic.thinking.budget_tokens": {10, 50, 100, 500, 1000, 5000, 10000, 50000, 100000},

	// Cost per request (USD)
	"anthropic.cost.request": {0.0001, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},

	// Character lengths
	"anthropic.response.text_length":       {10, 50, 100, 500, 1000, 5000, 10000, 50000},
	"anthropic.thinking.output_length":     {10, 50, 100, 500, 1000, 5000, 10000, 50000},
	"anthropic.request.system_prompt.size": {10, 50, 100, 500, 1000, 5000, 10000, 50000},
	"anthropic.tool_use.edit_size":         {10, 50, 100, 500, 1000, 5000, 10000, 50000},
	"anthropic.tool_use.write_size":        {10, 50, 100, 500, 1000, 5000, 10000, 50000},

	// Message counts
	"anthropic.request.messages_count": {1, 2, 5, 10, 20, 50, 100},

	// Tool counts
	"anthropic.request.tools_count": {0, 1, 2, 5, 10, 20},

	// Temperature
	"anthropic.request.temperature": {0, 0.1, 0.25, 0.5, 0.75, 1.0},

	// Streaming chunks
	"anthropic.streaming.chunks": {1, 5, 10, 50, 100, 500, 1000},

	// Conversation turns
	"anthropic.request.conversation_turns": {1, 2, 5, 10, 20, 50, 100},
}

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
		if data.session != nil && data.session.ProjectName != "" {
			m.PutStr("claude_code.project.name", data.session.ProjectName)
		}
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

		// anthropic.errors.by_type — error classification with error.type attribute
		errTypeAttrs := commonAttrs()
		if data.errorResponse != nil {
			errTypeAttrs.PutStr("error.type", data.errorResponse.Error.Type)
		} else {
			errTypeAttrs.PutStr("error.type", fmt.Sprintf("http_%d", data.statusCode))
		}
		tb.addSumDP(sm, "anthropic.errors.by_type", "{error}", start, now, 1, errTypeAttrs)
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
		if data.rateLimit.OutputTokensLimit > 0 {
			tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.limit", "{token}", start, now, float64(data.rateLimit.OutputTokensLimit), commonAttrs())
			tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.remaining", "{token}", start, now, float64(data.rateLimit.OutputTokensRemaining), commonAttrs())
			tb.addGaugeDP(sm, "anthropic.ratelimit.output_tokens.utilization", "1", start, now, data.rateLimit.OutputTokensUtilization(), commonAttrs())
		}
	}

	// Token output utilization gauge
	if data.request != nil && data.response != nil && data.request.MaxTokens > 0 {
		utilization := float64(data.response.Usage.OutputTokens) / float64(data.request.MaxTokens)
		tb.addGaugeDP(sm, "anthropic.tokens.output_utilization", "1", start, now, utilization, commonAttrs())
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

		// Conversation depth: count assistant messages as turns
		assistantTurns := 0
		for _, msg := range data.request.Messages {
			if msg.Role == "assistant" {
				assistantTurns++
			}
		}
		if assistantTurns > 0 {
			tb.addHistogramDP(sm, "anthropic.request.conversation_turns", "{turn}", start, now, float64(assistantTurns), commonAttrs())
		}

		// 37-38. Thinking metrics
		if data.request.Thinking != nil {
			tb.addSumDP(sm, "anthropic.thinking.enabled", "{request}", start, now, 1, commonAttrs())
			if data.request.Thinking.BudgetTokens > 0 {
				tb.addHistogramDP(sm, "anthropic.thinking.budget_tokens", "{token}", start, now, float64(data.request.Thinking.BudgetTokens), commonAttrs())
			}
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

	// Cache savings metric
	if data.cost.CacheSavings > 0 {
		tb.addSumDPf(sm, "anthropic.cost.cache_savings", "{USD}", start, now, data.cost.CacheSavings, commonAttrs())
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

	// Active requests gauge
	tb.addGaugeDP(sm, "anthropic.requests.active", "{request}", start, now, float64(data.activeRequests), commonAttrs())

	// Cost multiplied requests counter
	if data.cost.Multiplier != "" && data.cost.Multiplier != "standard" {
		multAttrs := commonAttrs()
		multAttrs.PutStr("multiplier", data.cost.Multiplier)
		tb.addSumDP(sm, "anthropic.cost.multiplied_requests", "{request}", start, now, 1, multAttrs)
	}

	// Claude Code session and project metrics
	if data.session != nil {
		sessionAttrs := func() pcommon.Map {
			m := pcommon.NewMap()
			m.PutStr("claude_code.session.id", data.session.SessionID)
			if data.session.ProjectName != "" {
				m.PutStr("claude_code.project.name", data.session.ProjectName)
			}
			return m
		}

		tb.addSumDP(sm, "claude_code.session.requests", "{request}", start, now, 1, sessionAttrs())
		tb.addSumDPf(sm, "claude_code.session.active_duration", "s", start, now, duration, sessionAttrs())
		tb.addSumDPf(sm, "claude_code.session.cost", "{USD}", start, now, data.cost.TotalCost, sessionAttrs())

		if data.response != nil {
			tb.addSumDP(sm, "claude_code.session.tokens.input", "{token}", start, now, int64(data.response.Usage.TotalInputTokens()), sessionAttrs())
			tb.addSumDP(sm, "claude_code.session.tokens.output", "{token}", start, now, int64(data.response.Usage.OutputTokens), sessionAttrs())
			tb.addSumDP(sm, "claude_code.session.tokens.cache_read", "{token}", start, now, int64(data.response.Usage.CacheReadInputTokens), sessionAttrs())
		}

		// Session conversation turns: count assistant messages in the request
		if data.request != nil {
			convTurns := 0
			for _, msg := range data.request.Messages {
				if msg.Role == "assistant" {
					convTurns++
				}
			}
			if convTurns > 0 {
				tb.addSumDP(sm, "claude_code.session.conversation_turns", "{turn}", start, now, int64(convTurns), sessionAttrs())
			}
		}

		// Session tool calls
		if len(data.toolCalls) > 0 {
			tb.addSumDP(sm, "claude_code.session.tool_calls", "{call}", start, now, int64(len(data.toolCalls)), sessionAttrs())
		}

		// Session lines changed: sum of LinesAdded + LinesRemoved across tool calls
		var totalLinesChanged int64
		for _, tc := range data.toolCalls {
			totalLinesChanged += int64(tc.LinesAdded + tc.LinesRemoved)
		}
		if totalLinesChanged > 0 {
			tb.addSumDP(sm, "claude_code.session.lines_changed", "{line}", start, now, totalLinesChanged, sessionAttrs())
		}

		// Session errors
		if data.statusCode >= 400 {
			tb.addSumDP(sm, "claude_code.session.errors", "{error}", start, now, 1, sessionAttrs())
		}

		// Project-level metrics (without session.id to avoid cardinality explosion)
		if data.session.ProjectName != "" {
			projectAttrs := pcommon.NewMap()
			projectAttrs.PutStr("claude_code.project.name", data.session.ProjectName)

			tb.addSumDP(sm, "claude_code.project.requests", "{request}", start, now, 1, projectAttrs)
			tb.addSumDPf(sm, "claude_code.project.cost", "{USD}", start, now, data.cost.TotalCost, projectAttrs)

			// Project token metrics
			if data.response != nil {
				tb.addSumDP(sm, "claude_code.project.tokens.input", "{token}", start, now, int64(data.response.Usage.TotalInputTokens()), projectAttrs)
				tb.addSumDP(sm, "claude_code.project.tokens.output", "{token}", start, now, int64(data.response.Usage.OutputTokens), projectAttrs)
			}

			// Project errors
			if data.statusCode >= 400 {
				tb.addSumDP(sm, "claude_code.project.errors", "{error}", start, now, 1, projectAttrs)
			}
		}
	}

	return tb.metricsConsumer.ConsumeMetrics(ctx, md)
}

func (tb *telemetryBuilder) addHistogramDP(sm pmetric.ScopeMetrics, name, unit string, startTs, ts pcommon.Timestamp, value float64, attrs pcommon.Map) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := h.DataPoints().AppendEmpty()
	dp.SetStartTimestamp(startTs)
	dp.SetTimestamp(ts)
	dp.SetCount(1)
	dp.SetSum(value)
	dp.SetMin(value)
	dp.SetMax(value)
	attrs.CopyTo(dp.Attributes())

	// Populate explicit bucket boundaries when defined for this metric.
	// Delta semantics: bucket[i] = 1 if value <= bounds[i], else 0.
	// The +Inf bucket (index len(bounds)) always equals count (1).
	if bounds, ok := histogramBuckets[name]; ok {
		dp.ExplicitBounds().FromRaw(bounds)
		counts := make([]uint64, len(bounds)+1)
		for i, b := range bounds {
			if value <= b {
				counts[i] = 1
			}
		}
		counts[len(bounds)] = 1 // +Inf bucket always equals count
		dp.BucketCounts().FromRaw(counts)
	}
}

func (tb *telemetryBuilder) addSumDP(sm pmetric.ScopeMetrics, name, unit string, startTs, ts pcommon.Timestamp, value int64, attrs pcommon.Map) {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	s := m.SetEmptySum()
	s.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
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
	s.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
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
