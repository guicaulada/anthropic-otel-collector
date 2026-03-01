package anthropicreceiver

import (
	"encoding/json"
	"strings"
	"time"
)

// streamAccumulator accumulates SSE events to reconstruct the complete response
// and capture streaming-specific metrics.
type streamAccumulator struct {
	// Response fields
	id           string
	model        string
	role         string
	stopReason   string
	stopSequence *string
	usage        Usage
	container    *Container

	// Content blocks
	contentBlocks []ContentBlock
	currentBlock  *accumulatorBlock

	// Streaming metrics
	startTime       time.Time
	firstTokenTime  time.Time
	hasFirstToken   bool
	totalEvents     int
	totalChunks     int
	eventCounts     map[string]int
	blockDurations  []time.Duration
}

// accumulatorBlock tracks a content block being streamed.
type accumulatorBlock struct {
	index     int
	blockType string
	startTime time.Time
	text      strings.Builder
	thinking  strings.Builder
	name      string
	id        string
	inputJSON strings.Builder
	data      string
}

// newStreamAccumulator creates a new stream accumulator.
func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		startTime:   time.Now(),
		eventCounts: make(map[string]int),
	}
}

// ProcessEvent processes a single SSE event and updates the accumulator state.
func (sa *streamAccumulator) ProcessEvent(event SSEEvent) error {
	sa.totalEvents++
	sa.eventCounts[event.Event]++

	switch event.Event {
	case SSEEventMessageStart:
		return sa.handleMessageStart(event.Data)
	case SSEEventContentBlockStart:
		return sa.handleContentBlockStart(event.Data)
	case SSEEventContentBlockDelta:
		return sa.handleContentBlockDelta(event.Data)
	case SSEEventContentBlockStop:
		return sa.handleContentBlockStop(event.Data)
	case SSEEventMessageDelta:
		return sa.handleMessageDelta(event.Data)
	case SSEEventMessageStop:
		// No data processing needed
		return nil
	case SSEEventPing:
		return nil
	case SSEEventError:
		return sa.handleError(event.Data)
	}
	return nil
}

func (sa *streamAccumulator) handleMessageStart(data json.RawMessage) error {
	var d MessageStartData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	sa.id = d.Message.ID
	sa.model = d.Message.Model
	sa.role = d.Message.Role
	sa.usage = d.Message.Usage
	sa.container = d.Message.Container
	return nil
}

func (sa *streamAccumulator) handleContentBlockStart(data json.RawMessage) error {
	var d ContentBlockStartData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	sa.currentBlock = &accumulatorBlock{
		index:     d.Index,
		blockType: d.ContentBlock.Type,
		startTime: time.Now(),
		name:      d.ContentBlock.Name,
		id:        d.ContentBlock.ID,
	}
	// redacted_thinking blocks arrive complete in content_block_start (no deltas)
	if d.ContentBlock.Type == "redacted_thinking" {
		sa.currentBlock.data = d.ContentBlock.Data
	}
	return nil
}

func (sa *streamAccumulator) handleContentBlockDelta(data json.RawMessage) error {
	var d ContentBlockDeltaData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	if sa.currentBlock == nil {
		return nil
	}

	switch d.Delta.Type {
	case "text_delta":
		if !sa.hasFirstToken {
			sa.firstTokenTime = time.Now()
			sa.hasFirstToken = true
		}
		sa.totalChunks++
		sa.currentBlock.text.WriteString(d.Delta.Text)
	case "thinking_delta":
		sa.currentBlock.thinking.WriteString(d.Delta.Thinking)
	case "input_json_delta":
		sa.currentBlock.inputJSON.WriteString(d.Delta.PartialJSON)
	case "signature_delta":
		// Signature deltas are for content integrity verification; no accumulation needed
	}

	return nil
}

func (sa *streamAccumulator) handleContentBlockStop(data json.RawMessage) error {
	if sa.currentBlock == nil {
		return nil
	}

	duration := time.Since(sa.currentBlock.startTime)
	sa.blockDurations = append(sa.blockDurations, duration)

	block := ContentBlock{
		Type: sa.currentBlock.blockType,
		Name: sa.currentBlock.name,
		ID:   sa.currentBlock.id,
	}

	switch sa.currentBlock.blockType {
	case "text":
		block.Text = sa.currentBlock.text.String()
	case "thinking":
		block.Thinking = sa.currentBlock.thinking.String()
	case "redacted_thinking":
		block.Data = sa.currentBlock.data
	case "tool_use":
		block.Input = json.RawMessage(sa.currentBlock.inputJSON.String())
	case "server_tool_use":
		block.Input = json.RawMessage(sa.currentBlock.inputJSON.String())
	case "web_search_tool_result", "code_execution_tool_result":
		block.Text = sa.currentBlock.text.String()
	}

	sa.contentBlocks = append(sa.contentBlocks, block)
	sa.currentBlock = nil
	return nil
}

func (sa *streamAccumulator) handleMessageDelta(data json.RawMessage) error {
	var d MessageDeltaData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	sa.stopReason = d.Delta.StopReason
	sa.stopSequence = d.Delta.StopSequence
	sa.usage.OutputTokens = d.Usage.OutputTokens
	if d.Usage.InputTokens != 0 {
		sa.usage.InputTokens = d.Usage.InputTokens
	}
	if d.Usage.CacheReadInputTokens != 0 {
		sa.usage.CacheReadInputTokens = d.Usage.CacheReadInputTokens
	}
	if d.Usage.CacheCreationInputTokens != 0 {
		sa.usage.CacheCreationInputTokens = d.Usage.CacheCreationInputTokens
	}
	return nil
}

func (sa *streamAccumulator) handleError(data json.RawMessage) error {
	// Error events are logged but don't stop processing
	return nil
}

// Response reconstructs the full AnthropicResponse from accumulated data.
func (sa *streamAccumulator) Response() *AnthropicResponse {
	return &AnthropicResponse{
		ID:           sa.id,
		Type:         "message",
		Role:         sa.role,
		Content:      sa.contentBlocks,
		Model:        sa.model,
		StopReason:   sa.stopReason,
		StopSequence: sa.stopSequence,
		Usage:        sa.usage,
		Container:    sa.container,
	}
}

// StreamingMetrics returns streaming-specific metrics.
func (sa *streamAccumulator) StreamingMetrics() StreamingMetrics {
	now := time.Now()
	m := StreamingMetrics{
		TotalEvents: sa.totalEvents,
		TotalChunks: sa.totalChunks,
		Duration:    now.Sub(sa.startTime),
		EventCounts: sa.eventCounts,
	}

	if sa.hasFirstToken {
		ttft := sa.firstTokenTime.Sub(sa.startTime)
		m.TimeToFirstToken = ttft
		m.HasFirstToken = true
	}

	if sa.usage.OutputTokens > 0 && sa.hasFirstToken {
		streamDuration := now.Sub(sa.firstTokenTime)
		m.AvgTimePerToken = streamDuration / time.Duration(sa.usage.OutputTokens)
	}

	m.BlockDurations = sa.blockDurations
	return m
}

// StreamingMetrics holds streaming-specific metric values.
type StreamingMetrics struct {
	TimeToFirstToken time.Duration
	HasFirstToken    bool
	TotalEvents      int
	TotalChunks      int
	Duration         time.Duration
	AvgTimePerToken  time.Duration
	EventCounts      map[string]int
	BlockDurations   []time.Duration
}

