package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	dash, err := buildDashboard()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build dashboard: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(dash, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal dashboard: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(data))
}
