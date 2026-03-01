package anthropicreceiver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsClaudeCodeRequest(t *testing.T) {
	tests := []struct {
		name         string
		betaFeatures string
		expected     bool
	}{
		{"empty", "", false},
		{"unrelated feature", "output-128k-2025-02-19", false},
		{"claude-code feature", "claude-code-20250601", true},
		{"claude-code among others", "output-128k-2025-02-19,claude-code-20250601,token-counting-2025-02-10", true},
		{"claude-code with spaces", "output-128k, claude-code-20250601", true},
		{"partial match not claude-code", "not-claude-code-prefix", false},
		{"prefix only", "claude-code-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isClaudeCodeRequest(tt.betaFeatures))
		})
	}
}

func TestExtractWorkingDir(t *testing.T) {
	tests := []struct {
		name     string
		system   json.RawMessage
		expected string
	}{
		{"nil system", nil, ""},
		{"empty system", json.RawMessage(""), ""},
		{"null system", json.RawMessage("null"), ""},
		{
			"string system with working dir",
			json.RawMessage(`"You are Claude Code.\n\n# Environment\n - Primary working directory: /Users/user/projects/my-app\n - Platform: darwin"`),
			"/Users/user/projects/my-app",
		},
		{
			"string system without working dir",
			json.RawMessage(`"You are a helpful assistant."`),
			"",
		},
		{
			"array system with working dir",
			json.RawMessage(`[{"type":"text","text":"You are Claude Code.\n\n# Environment\n - Primary working directory: /home/user/code/project\n - Platform: linux"}]`),
			"/home/user/code/project",
		},
		{
			"array system in second block",
			json.RawMessage(`[{"type":"text","text":"First block"},{"type":"text","text":"Primary working directory: /opt/app"}]`),
			"/opt/app",
		},
		{
			"array system without working dir",
			json.RawMessage(`[{"type":"text","text":"Just a regular prompt"}]`),
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractWorkingDir(tt.system))
		})
	}
}

func TestExtractUserID(t *testing.T) {
	tests := []struct {
		name     string
		metadata json.RawMessage
		expected string
	}{
		{"nil metadata", nil, ""},
		{"empty metadata", json.RawMessage(""), ""},
		{"null metadata", json.RawMessage("null"), ""},
		{"empty object", json.RawMessage(`{}`), ""},
		{"with user_id", json.RawMessage(`{"user_id":"user-abc-123"}`), "user-abc-123"},
		{"with other fields", json.RawMessage(`{"user_id":"u1","other":"value"}`), "u1"},
		{"invalid json", json.RawMessage(`{invalid`), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractUserID(tt.metadata))
		})
	}
}

func TestCountAssistantMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		expected int
	}{
		{"nil messages", nil, 0},
		{"empty messages", []Message{}, 0},
		{"only user messages", []Message{
			{Role: "user"},
			{Role: "user"},
		}, 0},
		{"mixed messages", []Message{
			{Role: "user"},
			{Role: "assistant"},
			{Role: "user"},
			{Role: "assistant"},
			{Role: "user"},
		}, 2},
		{"only assistant messages", []Message{
			{Role: "assistant"},
			{Role: "assistant"},
			{Role: "assistant"},
		}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, countAssistantMessages(tt.messages))
		})
	}
}

func TestExtractClaudeCodeContext(t *testing.T) {
	t.Run("non-claude-code request", func(t *testing.T) {
		req := &AnthropicRequest{
			Model: "claude-sonnet-4-6",
			Messages: []Message{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
			},
		}
		ctx := ExtractClaudeCodeContext(req, "output-128k-2025-02-19")
		assert.False(t, ctx.IsClaudeCode)
		assert.Empty(t, ctx.WorkingDir)
		assert.Empty(t, ctx.ProjectName)
		assert.Empty(t, ctx.UserID)
		assert.Equal(t, 0, ctx.ConversationDepth)
	})

	t.Run("claude-code request with full context", func(t *testing.T) {
		req := &AnthropicRequest{
			Model: "claude-sonnet-4-6",
			System: json.RawMessage(`"You are Claude Code.\n\n# Environment\n - Primary working directory: /Users/dev/projects/my-app\n - Platform: darwin"`),
			Messages: []Message{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
				{Role: "assistant", Content: json.RawMessage(`"Hi"`)},
				{Role: "user", Content: json.RawMessage(`"Do something"`)},
				{Role: "assistant", Content: json.RawMessage(`"Done"`)},
				{Role: "user", Content: json.RawMessage(`"More"`)},
			},
			Metadata: json.RawMessage(`{"user_id":"user-xyz"}`),
		}
		ctx := ExtractClaudeCodeContext(req, "claude-code-20250601")
		assert.True(t, ctx.IsClaudeCode)
		assert.Equal(t, "/Users/dev/projects/my-app", ctx.WorkingDir)
		assert.Equal(t, "my-app", ctx.ProjectName)
		assert.Equal(t, "user-xyz", ctx.UserID)
		assert.Equal(t, 2, ctx.ConversationDepth)
	})

	t.Run("claude-code request without system prompt", func(t *testing.T) {
		req := &AnthropicRequest{
			Model: "claude-sonnet-4-6",
			Messages: []Message{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
			},
		}
		ctx := ExtractClaudeCodeContext(req, "claude-code-20250601")
		assert.True(t, ctx.IsClaudeCode)
		assert.Empty(t, ctx.WorkingDir)
		assert.Empty(t, ctx.ProjectName)
	})

	t.Run("nil request", func(t *testing.T) {
		ctx := ExtractClaudeCodeContext(nil, "claude-code-20250601")
		assert.True(t, ctx.IsClaudeCode)
		assert.Empty(t, ctx.WorkingDir)
		assert.Equal(t, 0, ctx.ConversationDepth)
	})
}
