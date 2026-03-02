package anthropic

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/DevvGwardo/anthropic-otel-collector/receiver/anthropicreceiver"
	"github.com/DevvGwardo/anthropic-otel-collector/receiver/anthropicreceiver/adapter"
)

type Adapter struct {
	baseURL string
	pricing map[string]adapter.ModelPricing
}

func NewAdapter(baseURL string) *Adapter {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Adapter{baseURL: baseURL}
}

func (a *Adapter) Name() string { return "anthropic" }

func (a *Adapter) Detect(req *http.Request, body []byte) bool {
	if req.Header.Get("x-api-key") != "" {
		return true
	}
	if req.Header.Get("anthropic-version") != "" {
		return true
	}
	return false
}

func (a *Adapter) ParseRequest(body []byte) (*adapter.LLMRequest, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &adapter.LLMRequest{RawBody: body, Headers: map[string]string{}}, nil
	}

	var req anthropicreceiver.AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	toolNames := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t.Name != "" {
			toolNames = append(toolNames, t.Name)
		}
	}

	return &adapter.LLMRequest{
		Model:        req.Model,
		Messages:     len(req.Messages),
		SystemPrompt: parseSystemPrompt(req.System),
		Tools:        toolNames,
		Headers:      map[string]string{},
		Stream:       req.Stream,
		RawBody:      body,
	}, nil
}

func (a *Adapter) ParseResponse(body []byte, isStreaming bool) (*adapter.LLMResponse, error) {
	if isStreaming {
		return parseStreamingResponse(body), nil
	}

	var resp anthropicreceiver.AnthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	usage := adapter.Usage{
		InputTokens:      int64(resp.Usage.InputTokens),
		OutputTokens:     int64(resp.Usage.OutputTokens),
		CacheReadTokens:  int64(resp.Usage.CacheReadInputTokens),
		CacheWriteTokens: int64(resp.Usage.CacheCreationInputTokens),
	}
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens + usage.CacheWriteTokens

	return &adapter.LLMResponse{
		Model:        resp.Model,
		Content:      extractContentText(resp.Content),
		ToolCalls:    extractToolCalls(resp.Content),
		Usage:        usage,
		FinishReason: resp.StopReason,
		RawBody:      body,
	}, nil
}

func (a *Adapter) ExtractUsage(resp *adapter.LLMResponse) adapter.Usage {
	if resp == nil {
		return adapter.Usage{}
	}
	return resp.Usage
}

func (a *Adapter) CalculateCost(usage adapter.Usage, model string, pricing map[string]adapter.ModelPricing) float64 {
	activePricing := pricing
	if len(activePricing) == 0 {
		activePricing = a.pricing
	}
	p, ok := lookupPricing(model, activePricing)
	if !ok {
		return 0
	}

	inputCost := float64(usage.InputTokens) * p.InputPerMToken / 1_000_000
	outputCost := float64(usage.OutputTokens) * p.OutputPerMToken / 1_000_000
	cacheReadCost := float64(usage.CacheReadTokens) * p.CacheReadPerMToken / 1_000_000
	cacheWriteCost := float64(usage.CacheWriteTokens) * p.CacheCreationPerMToken / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheWriteCost
}

func (a *Adapter) GetUpstreamURL() string { return a.baseURL }

func (a *Adapter) ExtractContext(req *adapter.LLMRequest) map[string]string {
	ctx := map[string]string{
		"provider": "anthropic",
	}
	if req == nil {
		return ctx
	}

	ctx["model"] = req.Model
	ctx["messages"] = strconv.Itoa(req.Messages)
	ctx["stream"] = strconv.FormatBool(req.Stream)
	ctx["tools_count"] = strconv.Itoa(len(req.Tools))
	if req.SystemPrompt != "" {
		ctx["has_system_prompt"] = "true"
	}
	return ctx
}

func parseSystemPrompt(system json.RawMessage) string {
	if len(system) == 0 || string(system) == "null" {
		return ""
	}

	var asString string
	if err := json.Unmarshal(system, &asString); err == nil {
		return asString
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(system, &blocks); err != nil {
		return ""
	}

	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func extractContentText(blocks []anthropicreceiver.ContentBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

func extractToolCalls(blocks []anthropicreceiver.ContentBlock) []adapter.ToolCall {
	toolCalls := make([]adapter.ToolCall, 0)
	for _, block := range blocks {
		if block.Type != "tool_use" || block.Name == "" {
			continue
		}

		var args map[string]interface{}
		if len(block.Input) > 0 && string(block.Input) != "null" {
			_ = json.Unmarshal(block.Input, &args)
		}
		if args == nil {
			args = map[string]interface{}{}
		}

		toolCalls = append(toolCalls, adapter.ToolCall{
			Name:      block.Name,
			Arguments: args,
		})
	}
	return toolCalls
}

func parseStreamingResponse(body []byte) *adapter.LLMResponse {
	resp := &adapter.LLMResponse{
		RawBody: body,
	}

	var content strings.Builder
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var event struct {
			Type string `json:"type"`
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Delta struct {
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		switch event.Type {
		case anthropicreceiver.SSEEventMessageStart:
			resp.Model = event.Message.Model
			mu := event.Message.Usage
			resp.Usage.InputTokens = int64(mu.InputTokens)
			cacheRead := mu.CacheReadInputTokens
			cacheWrite := mu.CacheCreationInputTokens
			resp.Usage.CacheReadTokens = int64(cacheRead)
			resp.Usage.CacheWriteTokens = int64(cacheWrite)
		case anthropicreceiver.SSEEventContentBlockDelta:
			if event.Delta.Text != "" {
				content.WriteString(event.Delta.Text)
			}
		case anthropicreceiver.SSEEventMessageDelta:
			if event.Delta.StopReason != "" {
				resp.FinishReason = event.Delta.StopReason
			}
			if event.Usage.OutputTokens > 0 {
				resp.Usage.OutputTokens = int64(event.Usage.OutputTokens)
			}
			if event.Usage.InputTokens > 0 {
				resp.Usage.InputTokens = int64(event.Usage.InputTokens)
			}
			du := event.Usage
			cr := du.CacheReadInputTokens
			cw := du.CacheCreationInputTokens
			if cr > 0 {
				resp.Usage.CacheReadTokens = int64(cr)
			}
			if cw > 0 {
				resp.Usage.CacheWriteTokens = int64(cw)
			}
		}
	}

	resp.Content = content.String()
	resp.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens + resp.Usage.CacheReadTokens + resp.Usage.CacheWriteTokens
	return resp
}

func lookupPricing(model string, pricing map[string]adapter.ModelPricing) (adapter.ModelPricing, bool) {
	if len(pricing) == 0 || model == "" {
		return adapter.ModelPricing{}, false
	}
	if p, ok := pricing[model]; ok {
		return p, true
	}
	for name, p := range pricing {
		if strings.HasPrefix(model, name) || strings.HasPrefix(name, model) {
			return p, true
		}
	}
	return adapter.ModelPricing{}, false
}
