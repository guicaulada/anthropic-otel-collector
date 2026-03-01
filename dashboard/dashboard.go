package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/guicaulada/anthropic-otel-collector/dashboard/panels"
)

func buildDashboard() (dashboard.Dashboard, error) {
	builder := dashboard.NewDashboardBuilder("Anthropic Claude Code Usage").
		Uid("anthropic-claude-code-usage").
		Description("Comprehensive monitoring for Anthropic Claude Code API usage via the anthropic-otel-collector").
		Tags([]string{"anthropic", "claude", "opentelemetry"}).
		Refresh("30s").
		Time("now-6h", "now").
		Timezone("browser").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Variables(buildVariables()).
		// Row 1: Overview
		WithRow(dashboard.NewRowBuilder("Overview")).
		WithPanel(panels.TotalCost()).
		WithPanel(panels.TotalRequests()).
		WithPanel(panels.ErrorRate()).
		WithPanel(panels.AvgLatency()).
		WithPanel(panels.TotalTokens()).
		WithPanel(panels.OutputThroughput()).
		WithPanel(panels.CacheHitRatio()).
		WithPanel(panels.CacheSavingsStat()).
		// Row 2: Cost Analysis
		WithRow(dashboard.NewRowBuilder("Cost Analysis")).
		WithPanel(panels.CostOverTime()).
		WithPanel(panels.CostByModel()).
		WithPanel(panels.CostByCategory()).
		WithPanel(panels.AvgCostPerRequestTimeseries()).
		WithPanel(panels.CostPerOutputToken()).
		WithPanel(panels.CacheSavingsOverTime()).
		WithPanel(panels.WebSearchCost()).
		// Row 3: Token Usage
		WithRow(dashboard.NewRowBuilder("Token Usage")).
		WithPanel(panels.TokenUsageOverTime()).
		WithPanel(panels.CacheHitRatioOverTime()).
		WithPanel(panels.InputVsOutputBreakdown()).
		WithPanel(panels.CacheTokensDetail()).
		WithPanel(panels.OutputUtilization()).
		// Row 4: Performance
		WithRow(dashboard.NewRowBuilder("Performance")).
		WithPanel(panels.AvgOperationDuration()).
		WithPanel(panels.AvgTimeToFirstToken()).
		WithPanel(panels.AvgTimePerOutputToken()).
		WithPanel(panels.OutputThroughputTimeseries()).
		WithPanel(panels.AvgUpstreamLatency()).
		WithPanel(panels.RequestResponseBodySize()).
		WithPanel(panels.RequestRate()).
		WithPanel(panels.ErrorRateOverTime()).
		// Row 5: Tool Usage
		WithRow(dashboard.NewRowBuilder("Tool Usage")).
		WithPanel(panels.ToolCallDistribution()).
		WithPanel(panels.ToolCallsOverTime()).
		WithPanel(panels.FileOperations()).
		WithPanel(panels.LinesChangedOverTime()).
		WithPanel(panels.FileTypesWorkedOn()).
		WithPanel(panels.BashCommands()).
		WithPanel(panels.SearchOperations()).
		WithPanel(panels.FilesTouched()).
		// Row 6: Rate Limits
		WithRow(dashboard.NewRowBuilder("Rate Limits")).
		WithPanel(panels.RequestRateLimitUtilization()).
		WithPanel(panels.InputTokenRateLimitUtilization()).
		WithPanel(panels.RateLimitUtilizationOverTime()).
		WithPanel(panels.RateLimitRemaining()).
		// Row 7: Model & Request Analysis
		WithRow(dashboard.NewRowBuilder("Model & Request Analysis")).
		WithPanel(panels.RequestsByModel()).
		WithPanel(panels.StopReasons()).
		WithPanel(panels.ContentBlockTypes()).
		WithPanel(panels.ServerToolUse()).
		WithPanel(panels.AvgMessagesPerRequest()).
		WithPanel(panels.AvgSystemPromptSize()).
		WithPanel(panels.ThinkingEnabledRequests()).
		// Row 8: Streaming
		WithRow(dashboard.NewRowBuilder("Streaming")).
		WithPanel(panels.AvgStreamingDuration()).
		WithPanel(panels.AvgChunksPerRequest()).
		WithPanel(panels.SSEEventTypesOverTime()).
		WithPanel(panels.StreamingVsNonStreaming()).
		WithPanel(panels.AvgTTFTvsThroughput()).
		// Row 9: Projects
		WithRow(dashboard.NewRowBuilder("Projects")).
		WithPanel(panels.ProjectCostBreakdown()).
		WithPanel(panels.ProjectRequestsOverTime()).
		WithPanel(panels.CumulativeCostByProject())

	return builder.Build()
}

func buildVariables() []cog.Builder[dashboard.VariableModel] {
	dsVar := dashboard.NewDatasourceVariableBuilder("datasource").
		Label("Data Source").
		Type("prometheus").
		Current(dashboard.VariableOption{
			Text:     dashboard.StringOrArrayOfString{String: strPtr("default")},
			Value:    dashboard.StringOrArrayOfString{String: strPtr("default")},
			Selected: boolPtr(true),
		})

	modelQuery := "label_values(" + MetricRequests + ", gen_ai_request_model)"
	modelVar := dashboard.NewQueryVariableBuilder("model").
		Label("Model").
		Datasource(common.DataSourceRef{
			Uid:  strPtr("$datasource"),
			Type: strPtr("prometheus"),
		}).
		Query(dashboard.StringOrMap{String: &modelQuery}).
		Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
		IncludeAll(true).
		AllValue(".*").
		Multi(true).
		Sort(dashboard.VariableSortAlphabeticalAsc)

	apiKeyQuery := "label_values(" + MetricRequests + ", anthropic_api_key_hash)"
	apiKeyVar := dashboard.NewQueryVariableBuilder("api_key_hash").
		Label("API Key").
		Datasource(common.DataSourceRef{
			Uid:  strPtr("$datasource"),
			Type: strPtr("prometheus"),
		}).
		Query(dashboard.StringOrMap{String: &apiKeyQuery}).
		Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
		IncludeAll(true).
		AllValue(".*").
		Multi(true).
		Sort(dashboard.VariableSortAlphabeticalAsc)

	projectQuery := "label_values(" + MetricProjectRequests + ", claude_code_project_name)"
	projectVar := dashboard.NewQueryVariableBuilder("project").
		Label("Project").
		Datasource(common.DataSourceRef{
			Uid:  strPtr("$datasource"),
			Type: strPtr("prometheus"),
		}).
		Query(dashboard.StringOrMap{String: &projectQuery}).
		Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
		IncludeAll(true).
		AllValue(".*").
		Multi(true).
		Sort(dashboard.VariableSortAlphabeticalAsc)

	return []cog.Builder[dashboard.VariableModel]{
		dsVar,
		modelVar,
		apiKeyVar,
		projectVar,
	}
}

func boolPtr(b bool) *bool {
	return &b
}
