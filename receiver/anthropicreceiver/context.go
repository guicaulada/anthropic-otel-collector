package anthropicreceiver

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// ClaudeCodeContext holds extracted Claude Code context from a request.
type ClaudeCodeContext struct {
	// IsClaudeCode indicates whether the request originated from Claude Code.
	IsClaudeCode bool

	// WorkingDir is the primary working directory extracted from the system prompt.
	WorkingDir string

	// ProjectName is the base name of the working directory (e.g., "my-project").
	ProjectName string

	// UserID is the user identifier extracted from request metadata, if present.
	UserID string

	// ConversationDepth is the number of assistant messages in the conversation,
	// indicating how deep into a conversation this request is.
	ConversationDepth int
}

// workingDirPattern matches "Primary working directory: /path/to/project" in system prompts.
var workingDirPattern = regexp.MustCompile(`Primary working directory:\s*(\S+)`)

// ExtractClaudeCodeContext detects Claude Code requests and extracts request-relevant context.
func ExtractClaudeCodeContext(req *AnthropicRequest, betaFeatures string) ClaudeCodeContext {
	ctx := ClaudeCodeContext{}

	// Detect Claude Code from beta features header (claude-code-* prefix)
	ctx.IsClaudeCode = isClaudeCodeRequest(betaFeatures)
	if !ctx.IsClaudeCode {
		return ctx
	}

	// Extract working directory from system prompt
	if req != nil {
		ctx.WorkingDir = extractWorkingDir(req.System)
		if ctx.WorkingDir != "" {
			ctx.ProjectName = filepath.Base(ctx.WorkingDir)
		}

		// Extract user ID from metadata
		ctx.UserID = extractUserID(req.Metadata)

		// Count assistant messages as conversation depth
		ctx.ConversationDepth = countAssistantMessages(req.Messages)
	}

	return ctx
}

// isClaudeCodeRequest checks if the beta features header contains a claude-code-* prefix.
func isClaudeCodeRequest(betaFeatures string) bool {
	if betaFeatures == "" {
		return false
	}
	for _, feature := range strings.Split(betaFeatures, ",") {
		if strings.HasPrefix(strings.TrimSpace(feature), "claude-code-") {
			return true
		}
	}
	return false
}

// extractWorkingDir parses the system prompt to find "Primary working directory: /path".
func extractWorkingDir(system json.RawMessage) string {
	if system == nil || len(system) == 0 || string(system) == "null" {
		return ""
	}

	// System can be a plain string
	var s string
	if err := json.Unmarshal(system, &s); err == nil {
		return matchWorkingDir(s)
	}

	// Or an array of content blocks with text fields
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(system, &blocks); err == nil {
		for _, b := range blocks {
			if dir := matchWorkingDir(b.Text); dir != "" {
				return dir
			}
		}
	}

	return ""
}

// matchWorkingDir applies the regex to find the working directory in text.
func matchWorkingDir(text string) string {
	matches := workingDirPattern.FindStringSubmatch(text)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractUserID parses the metadata JSON to find a user_id field.
func extractUserID(metadata json.RawMessage) string {
	if metadata == nil || len(metadata) == 0 || string(metadata) == "null" {
		return ""
	}
	var m struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(metadata, &m); err != nil {
		return ""
	}
	return m.UserID
}

// countAssistantMessages returns the number of assistant messages in the conversation.
func countAssistantMessages(messages []Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == "assistant" {
			count++
		}
	}
	return count
}
