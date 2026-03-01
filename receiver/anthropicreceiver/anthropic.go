package anthropicreceiver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// AnthropicRequest represents an Anthropic Messages API request.
type AnthropicRequest struct {
	Model         string           `json:"model"`
	MaxTokens     int              `json:"max_tokens"`
	Messages      []Message        `json:"messages"`
	System        json.RawMessage  `json:"system,omitempty"`
	Stream        bool             `json:"stream,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
	TopP          *float64         `json:"top_p,omitempty"`
	TopK          *int             `json:"top_k,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
	Tools         []Tool           `json:"tools,omitempty"`
	ToolChoice    json.RawMessage  `json:"tool_choice,omitempty"`
	Metadata      json.RawMessage  `json:"metadata,omitempty"`
	Thinking      *ThinkingConfig  `json:"thinking,omitempty"`
}

// ThinkingConfig represents extended thinking configuration.
type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// Message represents a message in the conversation.
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Tool represents a tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// AnthropicResponse represents an Anthropic Messages API response.
type AnthropicResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// ContentBlock represents a content block in a response.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Citations json.RawMessage `json:"citations,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens              int            `json:"input_tokens"`
	OutputTokens             int            `json:"output_tokens"`
	CacheReadInputTokens     int            `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int            `json:"cache_creation_input_tokens,omitempty"`
	Speed                    string         `json:"speed,omitempty"`
	ServerToolUse            *ServerToolUse `json:"server_tool_use,omitempty"`
}

// ServerToolUse represents server-side tool usage counts.
type ServerToolUse struct {
	WebSearchRequests     int `json:"web_search_requests,omitempty"`
	WebFetchRequests      int `json:"web_fetch_requests,omitempty"`
	CodeExecutionRequests int `json:"code_execution_requests,omitempty"`
}

// TotalInputTokens returns the total input tokens including cache tokens.
func (u Usage) TotalInputTokens() int {
	return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

// AnthropicError represents an Anthropic API error response.
type AnthropicError struct {
	Type  string     `json:"type"`
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error type and message.
type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SSE event types from Anthropic streaming API.
const (
	SSEEventMessageStart     = "message_start"
	SSEEventContentBlockStart = "content_block_start"
	SSEEventPing             = "ping"
	SSEEventContentBlockDelta = "content_block_delta"
	SSEEventContentBlockStop  = "content_block_stop"
	SSEEventMessageDelta     = "message_delta"
	SSEEventMessageStop      = "message_stop"
	SSEEventError            = "error"
)

// SSEEvent represents a parsed SSE event.
type SSEEvent struct {
	Event string
	Data  json.RawMessage
}

// MessageStartData represents the data field of a message_start SSE event.
type MessageStartData struct {
	Type    string           `json:"type"`
	Message MessageStartInfo `json:"message"`
}

// MessageStartInfo contains initial message metadata.
type MessageStartInfo struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   *string        `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

// ContentBlockStartData represents the data field of a content_block_start SSE event.
type ContentBlockStartData struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

// ContentBlockDeltaData represents the data field of a content_block_delta SSE event.
type ContentBlockDeltaData struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta Delta  `json:"delta"`
}

// Delta represents a streaming delta.
type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

// ContentBlockStopData represents the data field of a content_block_stop SSE event.
type ContentBlockStopData struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// MessageDeltaData represents the data field of a message_delta SSE event.
type MessageDeltaData struct {
	Type  string         `json:"type"`
	Delta MessageDelta   `json:"delta"`
	Usage MessageDeltaUsage `json:"usage"`
}

// MessageDelta contains final message fields.
type MessageDelta struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence,omitempty"`
}

// MessageDeltaUsage contains cumulative token usage from message_delta.
type MessageDeltaUsage struct {
	OutputTokens             int `json:"output_tokens"`
	InputTokens              int `json:"input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// RateLimitInfo holds rate limit information extracted from response headers.
type RateLimitInfo struct {
	RequestsLimit          int
	RequestsRemaining      int
	InputTokensLimit       int
	InputTokensRemaining   int
	OutputTokensLimit      int
	OutputTokensRemaining  int
	RequestsReset          string
	InputTokensReset       string
	OutputTokensReset      string
	TokensLimit            int
	TokensRemaining        int
	OrganizationID         string
	RetryAfter             string
	UnifiedStatus          string
}

// RequestsUtilization returns the utilization ratio for requests.
func (r RateLimitInfo) RequestsUtilization() float64 {
	if r.RequestsLimit == 0 {
		return 0
	}
	return 1 - float64(r.RequestsRemaining)/float64(r.RequestsLimit)
}

// InputTokensUtilization returns the utilization ratio for input tokens.
func (r RateLimitInfo) InputTokensUtilization() float64 {
	if r.InputTokensLimit == 0 {
		return 0
	}
	return 1 - float64(r.InputTokensRemaining)/float64(r.InputTokensLimit)
}

// OutputTokensUtilization returns the utilization ratio for output tokens.
func (r RateLimitInfo) OutputTokensUtilization() float64 {
	if r.OutputTokensLimit == 0 {
		return 0
	}
	return 1 - float64(r.OutputTokensRemaining)/float64(r.OutputTokensLimit)
}

// ExtractRateLimitInfo extracts rate limit information from HTTP response headers.
func ExtractRateLimitInfo(headers http.Header) RateLimitInfo {
	return RateLimitInfo{
		RequestsLimit:         headerInt(headers, "anthropic-ratelimit-requests-limit"),
		RequestsRemaining:     headerInt(headers, "anthropic-ratelimit-requests-remaining"),
		InputTokensLimit:      headerInt(headers, "anthropic-ratelimit-input-tokens-limit"),
		InputTokensRemaining:  headerInt(headers, "anthropic-ratelimit-input-tokens-remaining"),
		OutputTokensLimit:     headerInt(headers, "anthropic-ratelimit-output-tokens-limit"),
		OutputTokensRemaining: headerInt(headers, "anthropic-ratelimit-output-tokens-remaining"),
		RequestsReset:         headers.Get("anthropic-ratelimit-requests-reset"),
		InputTokensReset:      headers.Get("anthropic-ratelimit-input-tokens-reset"),
		OutputTokensReset:     headers.Get("anthropic-ratelimit-output-tokens-reset"),
		TokensLimit:           headerInt(headers, "anthropic-ratelimit-tokens-limit"),
		TokensRemaining:       headerInt(headers, "anthropic-ratelimit-tokens-remaining"),
		OrganizationID:        headers.Get("x-anthropic-organization-id"),
		RetryAfter:            headers.Get("retry-after"),
		UnifiedStatus:         headers.Get("x-ratelimit-status"),
	}
}

func headerInt(headers http.Header, key string) int {
	v := headers.Get(key)
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

// SystemPromptSize returns the size of the system prompt in characters.
func (r *AnthropicRequest) SystemPromptSize() int {
	if r.System == nil {
		return 0
	}
	// System can be a string or array of content blocks
	var s string
	if err := json.Unmarshal(r.System, &s); err == nil {
		return len(s)
	}
	// If it's an array, count all text content
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.System, &blocks); err == nil {
		total := 0
		for _, b := range blocks {
			total += len(b.Text)
		}
		return total
	}
	return len(r.System)
}

// HasSystemPrompt returns whether a system prompt was provided.
func (r *AnthropicRequest) HasSystemPrompt() bool {
	return r.System != nil && len(r.System) > 0 && string(r.System) != "null"
}

// MessageRoleCounts returns a map of message role to count.
func (r *AnthropicRequest) MessageRoleCounts() map[string]int {
	counts := make(map[string]int)
	for _, msg := range r.Messages {
		counts[msg.Role]++
	}
	return counts
}

// ToolChoiceType extracts the type of tool_choice from the request.
// Returns "auto", "any", "tool", "none", or "" if not set.
func (r *AnthropicRequest) ToolChoiceType() string {
	if r.ToolChoice == nil || len(r.ToolChoice) == 0 || string(r.ToolChoice) == "null" {
		return ""
	}
	var tc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(r.ToolChoice, &tc); err != nil {
		return ""
	}
	return tc.Type
}

// CacheHitRatio returns the ratio of cache read tokens to total input tokens.
func CacheHitRatio(usage Usage) float64 {
	total := usage.TotalInputTokens()
	if total == 0 {
		return 0
	}
	return float64(usage.CacheReadInputTokens) / float64(total)
}

// TextContent returns the concatenated text from all text content blocks.
func (r *AnthropicResponse) TextContent() string {
	var sb strings.Builder
	for _, block := range r.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

// ToolCalls returns all tool_use content blocks.
func (r *AnthropicResponse) ToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, block := range r.Content {
		if block.Type == "tool_use" {
			calls = append(calls, block)
		}
	}
	return calls
}

// ThinkingBlocks returns all thinking content blocks.
func (r *AnthropicResponse) ThinkingBlocks() []ContentBlock {
	var blocks []ContentBlock
	for _, block := range r.Content {
		if block.Type == "thinking" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// ThinkingLength returns the total character count across all thinking blocks.
func (r *AnthropicResponse) ThinkingLength() int {
	total := 0
	for _, block := range r.Content {
		if block.Type == "thinking" {
			total += len(block.Thinking)
		}
	}
	return total
}

// ContentBlockCounts returns a map of content block type to count.
func (r *AnthropicResponse) ContentBlockCounts() map[string]int {
	counts := make(map[string]int)
	for _, block := range r.Content {
		counts[block.Type]++
	}
	return counts
}
