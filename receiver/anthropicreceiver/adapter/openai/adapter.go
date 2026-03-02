package openai

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/DevvGwardo/anthropic-otel-collector/receiver/anthropicreceiver/adapter"
)

type Adapter struct {
	baseURL string
	pricing map[string]adapter.ModelPricing
}

func NewAdapter(baseURL string) *Adapter {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &Adapter{baseURL: baseURL}
}

func (a *Adapter) Name() string { return "openai" }

func (a *Adapter) Detect(req *http.Request, body []byte) bool {
	if req.Header.Get("x-api-key") != "" {
		return false
	}

	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "Bearer ") {
		token := strings.TrimSpace(auth[7:])
		if strings.HasPrefix(token, "sk-") {
			return true
		}
	}

	var probe struct {
		Messages            []json.RawMessage `json:"messages"`
		MaxCompletionTokens *int              `json:"max_completion_tokens"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}

	return len(probe.Messages) > 0 && probe.MaxCompletionTokens != nil
}

func (a *Adapter) ParseRequest(body []byte) (*adapter.LLMRequest, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &adapter.LLMRequest{RawBody: body, Headers: map[string]string{}}, nil
	}

	var req openAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	tools := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t.Function.Name != "" {
			tools = append(tools, t.Function.Name)
		}
	}

	var systemParts []string
	for _, msg := range req.Messages {
		if msg.Role == "system" && msg.Content != "" {
			systemParts = append(systemParts, msg.Content)
		}
	}

	return &adapter.LLMRequest{
		Model:        req.Model,
		Messages:     len(req.Messages),
		SystemPrompt: strings.Join(systemParts, "\n"),
		Tools:        tools,
		Headers:      map[string]string{},
		Stream:       req.Stream,
		RawBody:      body,
	}, nil
}

func (a *Adapter) ParseResponse(body []byte, isStreaming bool) (*adapter.LLMResponse, error) {
	if isStreaming {
		return parseStreamingResponse(body), nil
	}

	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	usage := adapter.Usage{
		InputTokens:     int64(resp.Usage.PromptTokens),
		OutputTokens:    int64(resp.Usage.CompletionTokens),
		CacheReadTokens: int64(resp.Usage.PromptTokensDetails.CachedTokens),
		TotalTokens:     int64(resp.Usage.TotalTokens),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens
	}

	out := &adapter.LLMResponse{
		Model:   resp.Model,
		Usage:   usage,
		RawBody: body,
	}

	if len(resp.Choices) == 0 {
		return out, nil
	}

	firstChoice := resp.Choices[0]
	out.Content = firstChoice.Message.Content
	out.FinishReason = firstChoice.FinishReason

	if len(firstChoice.Message.ToolCalls) > 0 {
		out.ToolCalls = make([]adapter.ToolCall, 0, len(firstChoice.Message.ToolCalls))
		for _, tc := range firstChoice.Message.ToolCalls {
			if tc.Function.Name == "" {
				continue
			}

			var args map[string]interface{}
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			if args == nil {
				args = map[string]interface{}{}
			}

			out.ToolCalls = append(out.ToolCalls, adapter.ToolCall{
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	return out, nil
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
		"provider": "openai",
	}
	if req == nil {
		return ctx
	}

	ctx["model"] = req.Model
	ctx["messages"] = strconv.Itoa(req.Messages)
	ctx["tools_count"] = strconv.Itoa(len(req.Tools))
	return ctx
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
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
				PromptTokensDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if chunk.Model != "" {
			resp.Model = chunk.Model
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
			}
			if choice.FinishReason != "" {
				resp.FinishReason = choice.FinishReason
			}
		}

		if chunk.Usage.PromptTokens > 0 {
			resp.Usage.InputTokens = int64(chunk.Usage.PromptTokens)
		}
		if chunk.Usage.CompletionTokens > 0 {
			resp.Usage.OutputTokens = int64(chunk.Usage.CompletionTokens)
		}
		if chunk.Usage.TotalTokens > 0 {
			resp.Usage.TotalTokens = int64(chunk.Usage.TotalTokens)
		}
		if chunk.Usage.PromptTokensDetails.CachedTokens > 0 {
			resp.Usage.CacheReadTokens = int64(chunk.Usage.PromptTokensDetails.CachedTokens)
		}
	}

	resp.Content = content.String()
	if resp.Usage.TotalTokens == 0 {
		resp.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens + resp.Usage.CacheReadTokens
	}
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

type openAIRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	Tools               []openAITool    `json:"tools,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type openAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}
