package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

type dashboardDef struct {
	name     string
	filename string
	builder  func() (dashboard.Dashboard, error)
}

func main() {
	outputDir := flag.String("output-dir", "", "Directory to write dashboard JSON files (stdout if empty)")
	flag.Parse()

	dashboards := []dashboardDef{
		{"main", "anthropic-claude-code-usage.json", buildDashboard},
		{"activity", "claude-code-activity.json", buildPublicDashboard},
		{"lean", "claude-code-lean.json", buildLeanDashboard},
	}

	for _, d := range dashboards {
		dash, err := d.builder()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build %s dashboard: %v\n", d.name, err)
			os.Exit(1)
		}

		data, err := json.MarshalIndent(dash, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal %s dashboard: %v\n", d.name, err)
			os.Exit(1)
		}

		if *outputDir == "" {
			fmt.Println(string(data))
			continue
		}

		path := filepath.Join(*outputDir, d.filename)
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s dashboard: %v\n", d.name, err)
			os.Exit(1)
		}
	}
}
