package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/guicaulada/anthropic-otel-collector/dashboard/panels"
)

func buildCodingStatsDashboard() (dashboard.Dashboard, error) {
	builder := dashboard.NewDashboardBuilder("Claude Code Coding Stats").
		Uid("claude-code-coding-stats").
		Description("Compact coding stats dashboard showing lines changed, file types, projects, and tool usage").
		Tags([]string{"anthropic", "claude", "coding", "stats"}).
		Time("now-30d", "now").
		Timezone("browser").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Variables(buildCodingStatsVariables()).
		// Row 1: Stats (6+6+6+6=24)
		WithPanel(panels.TotalLinesChanged()).
		WithPanel(panels.TotalFilesTouched()).
		WithPanel(panels.TotalRequests()).
		WithPanel(panels.TotalTokens()).
		// Row 2: Activity (16+8=24)
		WithPanel(panels.LinesChangedOverTime()).
		WithPanel(panels.FileOperations()).
		// Row 3: Projects (6+18=24)
		WithPanel(panels.ProjectRequestsBreakdown()).
		WithPanel(panels.ProjectRequestsOverTime()).
		// Row 4: Breakdown (12+12=24)
		WithPanel(panels.FileTypesWorkedOn()).
		WithPanel(panels.ToolCallDistribution())

	return builder.Build()
}

func buildCodingStatsVariables() []cog.Builder[dashboard.VariableModel] {
	dsVar := dashboard.NewDatasourceVariableBuilder("datasource").
		Label("Data Source").
		Type("prometheus").
		Current(dashboard.VariableOption{
			Text:     dashboard.StringOrArrayOfString{String: strPtr("default")},
			Value:    dashboard.StringOrArrayOfString{String: strPtr("default")},
			Selected: boolPtr(true),
		}).
		Hide(dashboard.VariableHideHideVariable)

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
		Sort(dashboard.VariableSortAlphabeticalAsc).
		Hide(dashboard.VariableHideHideVariable)

	return []cog.Builder[dashboard.VariableModel]{
		dsVar,
		modelVar,
		apiKeyVar,
		projectVar,
	}
}
