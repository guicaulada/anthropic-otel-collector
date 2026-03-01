package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

func main() {
	outputDir := flag.String("output-dir", "", "Directory to write dashboard JSON files (stdout if empty)")
	which := flag.String("dashboard", "all", "Which dashboard to build: main, user, or all")
	flag.Parse()

	type dashEntry struct {
		name    string
		builder func() (dashboard.Dashboard, error)
	}

	all := []dashEntry{
		{"main", buildDashboard},
		{"user", buildUserDashboard},
	}

	var selected []dashEntry
	switch *which {
	case "main":
		selected = []dashEntry{all[0]}
	case "user":
		selected = []dashEntry{all[1]}
	case "all":
		selected = all
	default:
		fmt.Fprintf(os.Stderr, "unknown dashboard: %s\n", *which)
		os.Exit(1)
	}

	for _, entry := range selected {
		dash, err := entry.builder()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build %s dashboard: %v\n", entry.name, err)
			os.Exit(1)
		}
		if err := outputDashboard(dash, entry.name, *outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "failed to output %s dashboard: %v\n", entry.name, err)
			os.Exit(1)
		}
	}
}

func outputDashboard(dash dashboard.Dashboard, name, outputDir string) error {
	data, err := json.MarshalIndent(dash, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if outputDir == "" {
		fmt.Println(string(data))
		return nil
	}
	path := filepath.Join(outputDir, dashboardFilename(name))
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func dashboardFilename(name string) string {
	switch name {
	case "main":
		return "anthropic-claude-code-usage.json"
	case "user":
		return "claude-code-user-dashboard.json"
	default:
		return name + ".json"
	}
}
