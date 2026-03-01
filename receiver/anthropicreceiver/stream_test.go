package anthropicreceiver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamAccumulator_MessageStart(t *testing.T) {
	sa := newStreamAccumulator()

	data := `{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":0}}}`

	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventMessageStart,
		Data:  json.RawMessage(data),
	})
	require.NoError(t, err)

	assert.Equal(t, "msg_123", sa.id)
	assert.Equal(t, "claude-sonnet-4-6", sa.model)
	assert.Equal(t, "assistant", sa.role)
	assert.Equal(t, 100, sa.usage.InputTokens)
	assert.Equal(t, 1, sa.totalEvents)
	assert.Equal(t, 1, sa.eventCounts[SSEEventMessageStart])
}

func TestStreamAccumulator_ContentBlockStartAndDeltaAndStop(t *testing.T) {
	sa := newStreamAccumulator()

	// Content block start
	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)
	require.NotNil(t, sa.currentBlock)
	assert.Equal(t, "text", sa.currentBlock.blockType)
	assert.Equal(t, 0, sa.currentBlock.index)

	// Text delta
	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)
	assert.True(t, sa.hasFirstToken)
	assert.Equal(t, 1, sa.totalChunks)

	// Another delta
	deltaData2 := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData2),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, sa.totalChunks)

	// Content block stop
	stopData := `{"type":"content_block_stop","index":0}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStop,
		Data:  json.RawMessage(stopData),
	})
	require.NoError(t, err)
	assert.Nil(t, sa.currentBlock)
	require.Len(t, sa.contentBlocks, 1)
	assert.Equal(t, "text", sa.contentBlocks[0].Type)
	assert.Equal(t, "Hello world", sa.contentBlocks[0].Text)
	assert.Len(t, sa.blockDurations, 1)
}

func TestStreamAccumulator_TracksFirstTokenTime(t *testing.T) {
	sa := newStreamAccumulator()

	// Start content block
	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)

	assert.False(t, sa.hasFirstToken)

	// First text delta sets first token time
	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)
	assert.True(t, sa.hasFirstToken)
	assert.False(t, sa.firstTokenTime.IsZero())

	firstTokenTime := sa.firstTokenTime

	// Second delta should not change first token time
	deltaData2 := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData2),
	})
	require.NoError(t, err)
	assert.Equal(t, firstTokenTime, sa.firstTokenTime)
}

func TestStreamAccumulator_ThinkingDelta(t *testing.T) {
	sa := newStreamAccumulator()

	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","text":""}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)

	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)

	stopData := `{"type":"content_block_stop","index":0}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStop,
		Data:  json.RawMessage(stopData),
	})
	require.NoError(t, err)

	require.Len(t, sa.contentBlocks, 1)
	assert.Equal(t, "thinking", sa.contentBlocks[0].Type)
	assert.Equal(t, "Let me think...", sa.contentBlocks[0].Thinking)
}

func TestStreamAccumulator_MessageDelta(t *testing.T) {
	sa := newStreamAccumulator()

	data := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventMessageDelta,
		Data:  json.RawMessage(data),
	})
	require.NoError(t, err)

	assert.Equal(t, "end_turn", sa.stopReason)
	assert.Equal(t, 42, sa.usage.OutputTokens)
}

func TestStreamAccumulator_FullStreamSimulation(t *testing.T) {
	sa := newStreamAccumulator()

	events := []SSEEvent{
		{
			Event: SSEEventMessageStart,
			Data:  json.RawMessage(`{"type":"message_start","message":{"id":"msg_full","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":50,"output_tokens":0}}}`),
		},
		{
			Event: SSEEventContentBlockStart,
			Data:  json.RawMessage(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		},
		{
			Event: SSEEventContentBlockDelta,
			Data:  json.RawMessage(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
		},
		{
			Event: SSEEventContentBlockDelta,
			Data:  json.RawMessage(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world!"}}`),
		},
		{
			Event: SSEEventContentBlockStop,
			Data:  json.RawMessage(`{"type":"content_block_stop","index":0}`),
		},
		{
			Event: SSEEventMessageDelta,
			Data:  json.RawMessage(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":10}}`),
		},
		{
			Event: SSEEventMessageStop,
			Data:  json.RawMessage(`{}`),
		},
	}

	for _, ev := range events {
		err := sa.ProcessEvent(ev)
		require.NoError(t, err)
	}

	assert.Equal(t, 7, sa.totalEvents)
	assert.Equal(t, 2, sa.totalChunks)

	// Verify response reconstruction
	resp := sa.Response()
	assert.Equal(t, "msg_full", resp.ID)
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, "claude-sonnet-4-6", resp.Model)
	assert.Equal(t, "end_turn", resp.StopReason)
	assert.Equal(t, 50, resp.Usage.InputTokens)
	assert.Equal(t, 10, resp.Usage.OutputTokens)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, "text", resp.Content[0].Type)
	assert.Equal(t, "Hello, world!", resp.Content[0].Text)
}

func TestStreamAccumulator_Response(t *testing.T) {
	sa := newStreamAccumulator()
	sa.id = "msg_test"
	sa.model = "claude-sonnet-4-6"
	sa.role = "assistant"
	sa.stopReason = "end_turn"
	sa.usage = Usage{InputTokens: 100, OutputTokens: 50}
	sa.contentBlocks = []ContentBlock{
		{Type: "text", Text: "Hello"},
	}

	resp := sa.Response()

	assert.Equal(t, "msg_test", resp.ID)
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, "claude-sonnet-4-6", resp.Model)
	assert.Equal(t, "end_turn", resp.StopReason)
	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, "Hello", resp.Content[0].Text)
}

func TestStreamAccumulator_StreamingMetrics(t *testing.T) {
	sa := newStreamAccumulator()
	sa.totalEvents = 10
	sa.totalChunks = 5
	sa.hasFirstToken = true
	sa.firstTokenTime = sa.startTime.Add(100_000_000) // 100ms

	metrics := sa.StreamingMetrics()

	assert.Equal(t, 10, metrics.TotalEvents)
	assert.Equal(t, 5, metrics.TotalChunks)
	assert.True(t, metrics.HasFirstToken)
	assert.Greater(t, metrics.Duration.Nanoseconds(), int64(0))
	assert.NotNil(t, metrics.EventCounts)
}

func TestStreamAccumulator_StreamingMetrics_NoFirstToken(t *testing.T) {
	sa := newStreamAccumulator()
	sa.totalEvents = 2

	metrics := sa.StreamingMetrics()

	assert.Equal(t, 2, metrics.TotalEvents)
	assert.False(t, metrics.HasFirstToken)
	assert.Equal(t, int64(0), metrics.TimeToFirstToken.Nanoseconds())
}

func TestStreamAccumulator_PingEvent(t *testing.T) {
	sa := newStreamAccumulator()

	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventPing,
		Data:  json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, sa.totalEvents)
	assert.Equal(t, 1, sa.eventCounts[SSEEventPing])
}

func TestStreamAccumulator_ErrorEvent(t *testing.T) {
	sa := newStreamAccumulator()

	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventError,
		Data:  json.RawMessage(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, sa.totalEvents)
}

func TestStreamAccumulator_ContentBlockDeltaWithNoCurrentBlock(t *testing.T) {
	sa := newStreamAccumulator()

	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"orphan"}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)
	// Should not panic; just no-op
}

func TestStreamAccumulator_ContentBlockStopWithNoCurrentBlock(t *testing.T) {
	sa := newStreamAccumulator()

	stopData := `{"type":"content_block_stop","index":0}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStop,
		Data:  json.RawMessage(stopData),
	})
	require.NoError(t, err)
	// Should not panic
	assert.Empty(t, sa.contentBlocks)
}

func TestStreamAccumulator_ToolUseBlock(t *testing.T) {
	sa := newStreamAccumulator()

	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc_1","name":"Edit"}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)

	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\""}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)

	deltaData2 := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json": ":\"/test.go\"}"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData2),
	})
	require.NoError(t, err)

	stopData := `{"type":"content_block_stop","index":0}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStop,
		Data:  json.RawMessage(stopData),
	})
	require.NoError(t, err)

	require.Len(t, sa.contentBlocks, 1)
	assert.Equal(t, "tool_use", sa.contentBlocks[0].Type)
	assert.Equal(t, "Edit", sa.contentBlocks[0].Name)
	assert.Equal(t, "tc_1", sa.contentBlocks[0].ID)
	assert.NotEmpty(t, sa.contentBlocks[0].Input)
}

func TestStreamAccumulator_MessageDelta_FullUsage(t *testing.T) {
	sa := newStreamAccumulator()

	data := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42,"input_tokens":100,"cache_read_input_tokens":50,"cache_creation_input_tokens":25}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventMessageDelta,
		Data:  json.RawMessage(data),
	})
	require.NoError(t, err)

	assert.Equal(t, "end_turn", sa.stopReason)
	assert.Equal(t, 42, sa.usage.OutputTokens)
	assert.Equal(t, 100, sa.usage.InputTokens)
	assert.Equal(t, 50, sa.usage.CacheReadInputTokens)
	assert.Equal(t, 25, sa.usage.CacheCreationInputTokens)
}

func TestStreamAccumulator_SignatureDelta(t *testing.T) {
	sa := newStreamAccumulator()

	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)

	// Signature delta should be handled without error
	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"abc123sig"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)
	// No text should have been accumulated from signature delta
	assert.Equal(t, "", sa.currentBlock.text.String())
}

func TestStreamAccumulator_ServerToolUseBlock(t *testing.T) {
	sa := newStreamAccumulator()

	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"web_search"}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)
	assert.Equal(t, "server_tool_use", sa.currentBlock.blockType)

	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"test\"}"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)

	stopData := `{"type":"content_block_stop","index":0}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStop,
		Data:  json.RawMessage(stopData),
	})
	require.NoError(t, err)

	require.Len(t, sa.contentBlocks, 1)
	assert.Equal(t, "server_tool_use", sa.contentBlocks[0].Type)
	assert.Equal(t, "web_search", sa.contentBlocks[0].Name)
	assert.Equal(t, "stu_1", sa.contentBlocks[0].ID)
	assert.NotEmpty(t, sa.contentBlocks[0].Input)
}

func TestStreamAccumulator_WebSearchToolResultBlock(t *testing.T) {
	sa := newStreamAccumulator()

	startData := `{"type":"content_block_start","index":0,"content_block":{"type":"web_search_tool_result","text":""}}`
	err := sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStart,
		Data:  json.RawMessage(startData),
	})
	require.NoError(t, err)

	deltaData := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Search results here"}}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockDelta,
		Data:  json.RawMessage(deltaData),
	})
	require.NoError(t, err)

	stopData := `{"type":"content_block_stop","index":0}`
	err = sa.ProcessEvent(SSEEvent{
		Event: SSEEventContentBlockStop,
		Data:  json.RawMessage(stopData),
	})
	require.NoError(t, err)

	require.Len(t, sa.contentBlocks, 1)
	assert.Equal(t, "web_search_tool_result", sa.contentBlocks[0].Type)
	assert.Equal(t, "Search results here", sa.contentBlocks[0].Text)
}

