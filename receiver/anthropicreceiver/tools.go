package anthropicreceiver

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// ToolCallInfo represents parsed information from a tool_use content block.
type ToolCallInfo struct {
	ToolName     string
	ToolCallID   string
	FilePath     string
	FileExt      string
	Pattern      string
	LinesAdded   int
	LinesRemoved int
	EditSize     int
	WriteSize    int
	Command      string
}

// EditToolInput represents the input for an Edit tool call.
type EditToolInput struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// WriteToolInput represents the input for a Write tool call.
type WriteToolInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// ReadToolInput represents the input for a Read tool call.
type ReadToolInput struct {
	FilePath string `json:"file_path"`
}

// BashToolInput represents the input for a Bash tool call.
type BashToolInput struct {
	Command string `json:"command"`
}

// GlobToolInput represents the input for a Glob tool call.
type GlobToolInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// GrepToolInput represents the input for a Grep tool call.
type GrepToolInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// ParseToolCalls extracts structured information from tool_use content blocks.
func ParseToolCalls(blocks []ContentBlock) []ToolCallInfo {
	var results []ToolCallInfo
	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		info := parseToolCall(block)
		results = append(results, info)
	}
	return results
}

func parseToolCall(block ContentBlock) ToolCallInfo {
	info := ToolCallInfo{
		ToolName:   block.Name,
		ToolCallID: block.ID,
	}

	switch block.Name {
	case "Edit":
		var input EditToolInput
		if err := json.Unmarshal(block.Input, &input); err == nil {
			info.FilePath = input.FilePath
			info.FileExt = fileExtension(input.FilePath)
			info.LinesAdded = countLines(input.NewString)
			info.LinesRemoved = countLines(input.OldString)
			info.EditSize = len(input.OldString) + len(input.NewString)
		}
	case "Write":
		var input WriteToolInput
		if err := json.Unmarshal(block.Input, &input); err == nil {
			info.FilePath = input.FilePath
			info.FileExt = fileExtension(input.FilePath)
			info.LinesAdded = countLines(input.Content)
			info.WriteSize = len(input.Content)
		}
	case "Read":
		var input ReadToolInput
		if err := json.Unmarshal(block.Input, &input); err == nil {
			info.FilePath = input.FilePath
			info.FileExt = fileExtension(input.FilePath)
		}
	case "Bash":
		var input BashToolInput
		if err := json.Unmarshal(block.Input, &input); err == nil {
			info.Command = input.Command
		}
	case "Glob":
		var input GlobToolInput
		if err := json.Unmarshal(block.Input, &input); err == nil {
			info.FilePath = input.Path
			info.Pattern = input.Pattern
		}
	case "Grep":
		var input GrepToolInput
		if err := json.Unmarshal(block.Input, &input); err == nil {
			info.FilePath = input.Path
			info.Pattern = input.Pattern
		}
	}

	return info
}

func fileExtension(path string) string {
	if path == "" {
		return ""
	}
	return strings.ToLower(filepath.Ext(path))
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	// Count the last line if it doesn't end with newline
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// CommandPreview returns the first maxLen characters of a command for logging.
func CommandPreview(command string, maxLen int) string {
	if len(command) <= maxLen {
		return command
	}
	return command[:maxLen] + "..."
}
