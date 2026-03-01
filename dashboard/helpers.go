package main

// MetricRequests is the Prometheus metric name for request counts,
// used in template variable queries.
const MetricRequests = "anthropic_requests_total"

// MetricProjectRequests is the Prometheus metric name for project-level request counts.
const MetricProjectRequests = "claude_code_project_requests_total"

// MetricCacheSavings is the Prometheus metric for cache savings.
const MetricCacheSavings = "anthropic_cost_cache_savings_total"

// MetricOutputUtilization is the Prometheus metric for output token utilization.
const MetricOutputUtilization = "anthropic_tokens_output_utilization"

// MetricSessionCacheRead is the Prometheus metric for session-level cache read tokens.
const MetricSessionCacheRead = "claude_code_session_tokens_cache_read_total"

// MetricSessionConvTurns is the Prometheus metric for session conversation turns.
const MetricSessionConvTurns = "claude_code_session_conversation_turns_total"

// MetricSessionToolCalls is the Prometheus metric for session tool calls.
const MetricSessionToolCalls = "claude_code_session_tool_calls_total"

// MetricSessionLinesChanged is the Prometheus metric for session lines changed.
const MetricSessionLinesChanged = "claude_code_session_lines_changed_total"

// MetricSessionErrors is the Prometheus metric for session errors.
const MetricSessionErrors = "claude_code_session_errors_total"

// MetricProjectTokensInput is the Prometheus metric for project input tokens.
const MetricProjectTokensInput = "claude_code_project_tokens_input_total"

// MetricProjectTokensOutput is the Prometheus metric for project output tokens.
const MetricProjectTokensOutput = "claude_code_project_tokens_output_total"

// MetricProjectErrors is the Prometheus metric for project errors.
const MetricProjectErrors = "claude_code_project_errors_total"

// MetricErrorsByType is the Prometheus metric for errors broken down by type.
const MetricErrorsByType = "anthropic_errors_by_type_total"

func strPtr(s string) *string {
	return &s
}
