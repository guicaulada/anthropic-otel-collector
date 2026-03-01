package main

// MetricRequests is the Prometheus metric name for request counts,
// used in template variable queries.
const MetricRequests = "anthropic_requests_total"

func strPtr(s string) *string {
	return &s
}
