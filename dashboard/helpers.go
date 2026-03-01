package main

// MetricRequests is the Prometheus metric name for request counts,
// used in template variable queries.
const MetricRequests = "anthropic_requests_total"

// MetricProjectRequests is the Prometheus metric name for project-level request counts.
const MetricProjectRequests = "claude_code_project_requests_total"

func strPtr(s string) *string {
	return &s
}
