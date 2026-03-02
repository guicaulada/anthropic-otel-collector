package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/guicaulada/anthropic-otel-collector/dashboard/panels"
)

func buildActivityDashboard() (dashboard.Dashboard, error) {
	builder := dashboard.NewDashboardBuilder("Claude Code Developer Activity").
		Uid("claude-code-developer-activity").
		Description("Comprehensive developer activity dashboard covering projects, code changes, tools, and AI insights").
		Tags([]string{"anthropic", "claude", "developer", "activity"}).
		Refresh("30s").
		Time("now-7d", "now").
		Timezone("browser").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Variables(buildActivityVariables()).
		// Row 1: Activity Overview
		WithRow(dashboard.NewRowBuilder("Activity Overview")).
		WithPanel(panels.TotalRequests()).
		WithPanel(panels.TotalTokens()).
		WithPanel(panels.TotalLinesChanged()).
		WithPanel(panels.TotalFilesTouched()).
		// Row 2: Projects
		WithRow(dashboard.NewRowBuilder("Projects")).
		WithPanel(panels.ProjectRequestsBreakdown()).
		WithPanel(panels.ProjectRequestsOverTime()).
		// Row 3: Code Activity
		WithRow(dashboard.NewRowBuilder("Code Activity")).
		WithPanel(panels.LinesChangedOverTime()).
		WithPanel(panels.FileOperations()).
		WithPanel(panels.FileTypesWorkedOn()).
		WithPanel(panels.BashCommands()).
		WithPanel(panels.SearchOperations()).
		WithPanel(panels.FilesTouched()).
		// Row 4: Tool Usage (collapsed)
		WithRow(
			dashboard.NewRowBuilder("Tool Usage").
				WithPanel(panels.ToolCallDistribution()).
				WithPanel(panels.ToolCallsOverTime()),
		).
		// Row 5: AI Insights (collapsed)
		WithRow(
			dashboard.NewRowBuilder("AI Insights").
				WithPanel(panels.RequestsByModel()).
				WithPanel(panels.StopReasons()).
				WithPanel(panels.ContentBlockTypes()).
				WithPanel(panels.ServerToolUse()).
				WithPanel(panels.AvgMessagesPerRequest()).
				WithPanel(panels.StreamingVsNonStreaming()).
				WithPanel(panels.ThinkingEnabledRequests()),
		)

	return builder.Build()
}

func buildActivityVariables() []cog.Builder[dashboard.VariableModel] {
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
		Sort(dashboard.VariableSortAlphabeticalAsc).
		Hide(dashboard.VariableHideHideVariable)

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
		Sort(dashboard.VariableSortAlphabeticalAsc).
		Hide(dashboard.VariableHideHideVariable)

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
