package anthropicreceiver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseToolCalls_EditTool(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type: "tool_use",
			Name: "Edit",
			ID:   "tc_edit_1",
			Input: json.RawMessage(`{
				"file_path": "/src/main.go",
				"old_string": "line1\nline2\nline3",
				"new_string": "newline1\nnewline2"
			}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 1)

	info := results[0]
	assert.Equal(t, "Edit", info.ToolName)
	assert.Equal(t, "tc_edit_1", info.ToolCallID)
	assert.Equal(t, "/src/main.go", info.FilePath)
	assert.Equal(t, ".go", info.FileExt)
	assert.Equal(t, 2, info.LinesAdded)   // "newline1\nnewline2" = 2 lines
	assert.Equal(t, 3, info.LinesRemoved) // "line1\nline2\nline3" = 3 lines
	assert.Greater(t, info.EditSize, 0)
}

func TestParseToolCalls_WriteTool(t *testing.T) {
	content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	blocks := []ContentBlock{
		{
			Type: "tool_use",
			Name: "Write",
			ID:   "tc_write_1",
			Input: json.RawMessage(`{
				"file_path": "/src/app.py",
				"content": "` + jsonEscapeString(content) + `"
			}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 1)

	info := results[0]
	assert.Equal(t, "Write", info.ToolName)
	assert.Equal(t, "/src/app.py", info.FilePath)
	assert.Equal(t, ".py", info.FileExt)
	assert.Equal(t, 5, info.LinesAdded) // 5 lines (trailing newline)
	assert.Equal(t, len(content), info.WriteSize)
}

func TestParseToolCalls_ReadTool(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type:  "tool_use",
			Name:  "Read",
			ID:    "tc_read_1",
			Input: json.RawMessage(`{"file_path": "/config/settings.yaml"}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 1)

	info := results[0]
	assert.Equal(t, "Read", info.ToolName)
	assert.Equal(t, "/config/settings.yaml", info.FilePath)
	assert.Equal(t, ".yaml", info.FileExt)
}

func TestParseToolCalls_BashTool(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type:  "tool_use",
			Name:  "Bash",
			ID:    "tc_bash_1",
			Input: json.RawMessage(`{"command": "go test ./..."}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 1)

	info := results[0]
	assert.Equal(t, "Bash", info.ToolName)
	assert.Equal(t, "go test ./...", info.Command)
}

func TestParseToolCalls_MixedToolTypes(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type:  "tool_use",
			Name:  "Read",
			ID:    "tc_1",
			Input: json.RawMessage(`{"file_path": "/file.ts"}`),
		},
		{
			Type:  "tool_use",
			Name:  "Edit",
			ID:    "tc_2",
			Input: json.RawMessage(`{"file_path": "/file.ts", "old_string": "old", "new_string": "new"}`),
		},
		{
			Type:  "tool_use",
			Name:  "Bash",
			ID:    "tc_3",
			Input: json.RawMessage(`{"command": "npm test"}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 3)

	assert.Equal(t, "Read", results[0].ToolName)
	assert.Equal(t, "Edit", results[1].ToolName)
	assert.Equal(t, "Bash", results[2].ToolName)
}

func TestParseToolCalls_EmptyAndNonToolBlocks(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "text", Text: "Hello world"},
		{Type: "thinking", Thinking: "Let me think"},
	}

	results := ParseToolCalls(blocks)
	assert.Empty(t, results)
}

func TestParseToolCalls_EmptySlice(t *testing.T) {
	results := ParseToolCalls(nil)
	assert.Empty(t, results)
}

func TestParseToolCalls_GlobTool(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type:  "tool_use",
			Name:  "Glob",
			ID:    "tc_glob_1",
			Input: json.RawMessage(`{"pattern": "**/*.go", "path": "/src"}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 1)
	assert.Equal(t, "Glob", results[0].ToolName)
	assert.Equal(t, "/src", results[0].FilePath)
	assert.Equal(t, "**/*.go", results[0].Pattern)
}

func TestParseToolCalls_GrepTool(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type:  "tool_use",
			Name:  "Grep",
			ID:    "tc_grep_1",
			Input: json.RawMessage(`{"pattern": "func main", "path": "/src"}`),
		},
	}

	results := ParseToolCalls(blocks)
	require.Len(t, results, 1)
	assert.Equal(t, "Grep", results[0].ToolName)
	assert.Equal(t, "/src", results[0].FilePath)
	assert.Equal(t, "func main", results[0].Pattern)
}

// --- countLines tests ---

func TestCountLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"single line no newline", "hello", 1},
		{"single line with newline", "hello\n", 1},
		{"two lines", "hello\nworld", 2},
		{"two lines with trailing newline", "hello\nworld\n", 2},
		{"three lines", "a\nb\nc", 3},
		{"only newlines", "\n\n\n", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, countLines(tt.input))
		})
	}
}

// --- CommandPreview tests ---

func TestCommandPreview(t *testing.T) {
	t.Run("short command not truncated", func(t *testing.T) {
		assert.Equal(t, "ls -la", CommandPreview("ls -la", 100))
	})

	t.Run("long command truncated", func(t *testing.T) {
		cmd := "very long command that exceeds the limit"
		result := CommandPreview(cmd, 10)
		assert.Equal(t, "very long ...", result)
		assert.Len(t, result, 13) // 10 + "..."
	})

	t.Run("exact length not truncated", func(t *testing.T) {
		cmd := "exact"
		assert.Equal(t, "exact", CommandPreview(cmd, 5))
	})
}

// --- fileExtension tests ---

func TestFileExtension(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/src/main.go", ".go"},
		{"/src/main.GO", ".go"},
		{"/src/file.test.ts", ".ts"},
		{"", ""},
		{"no-extension", ""},
		{"/path/to/.hidden", ".hidden"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, fileExtension(tt.path))
		})
	}
}

// jsonEscapeString escapes a string for embedding in a JSON string literal.
func jsonEscapeString(s string) string {
	b, _ := json.Marshal(s)
	// Remove the surrounding quotes
	return string(b[1 : len(b)-1])
}
