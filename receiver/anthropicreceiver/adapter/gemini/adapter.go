package gemini

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
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &Adapter{baseURL: baseURL}
}

func (a *Adapter) Name() string { return "gemini" }

func (a *Adapter) Detect(req *http.Request, body []byte) bool {
	if req.URL.Query().Get("key") != "" {
		return true
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	_, ok := probe["contents"]
	return ok
}

func (a *Adapter) ParseRequest(body []byte) (*adapter.LLMRequest, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &adapter.LLMRequest{RawBody: body, Headers: map[string]string{}}, nil
	}

	var req geminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream,omitempty"`
	}
	_ = json.Unmarshal(body, &meta)

	tools := make([]string, 0)
	for _, t := range req.Tools {
		for _, decl := range t.FunctionDeclarations {
			if decl.Name != "" {
				tools = append(tools, decl.Name)
			}
		}
	}

	return &adapter.LLMRequest{
		Model:    meta.Model,
		Messages: len(req.Contents),
		Tools:    tools,
		Headers:  map[string]string{},
		Stream:   meta.Stream,
		RawBody:  body,
	}, nil
}

func (a *Adapter) ParseResponse(body []byte, isStreaming bool) (*adapter.LLMResponse, error) {
	if isStreaming {
		return parseStreamingResponse(body), nil
	}

	var resp geminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	usage := adapter.Usage{
		InputTokens:  int64(resp.UsageMetadata.PromptTokenCount),
		OutputTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
		TotalTokens:  int64(resp.UsageMetadata.TotalTokenCount),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	out := &adapter.LLMResponse{
		Usage:   usage,
		RawBody: body,
	}

	if len(resp.Candidates) == 0 {
		return out, nil
	}

	first := resp.Candidates[0]
	out.Content = extractPartsText(first.Content.Parts)
	out.FinishReason = first.FinishReason
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
		"provider": "gemini",
	}
	if req == nil {
		return ctx
	}

	ctx["model"] = req.Model
	ctx["messages"] = strconv.Itoa(req.Messages)
	ctx["stream"] = strconv.FormatBool(req.Stream)
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
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var chunk geminiResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if len(chunk.Candidates) > 0 {
			first := chunk.Candidates[0]
			if first.FinishReason != "" {
				resp.FinishReason = first.FinishReason
			}
			for _, part := range first.Content.Parts {
				if part.Text != "" {
					content.WriteString(part.Text)
				}
			}
		}

		um := chunk.UsageMetadata
		if um.PromptTokenCount > 0 {
			resp.Usage.InputTokens = int64(um.PromptTokenCount)
		}
		candidates := um.CandidatesTokenCount
		if candidates > 0 {
			resp.Usage.OutputTokens = int64(candidates)
		}
		if um.TotalTokenCount > 0 {
			resp.Usage.TotalTokens = int64(um.TotalTokenCount)
		}
	}

	resp.Content = content.String()
	if resp.Usage.TotalTokens == 0 {
		resp.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens
	}
	return resp
}

func extractPartsText(parts []geminiPart) string {
	var out []string
	for _, p := range parts {
		if p.Text != "" {
			out = append(out, p.Text)
		}
	}
	return strings.Join(out, "\n")
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

type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	Tools            []geminiTool    `json:"tools,omitempty"`
	GenerationConfig json.RawMessage `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []struct {
		Name string `json:"name"`
	} `json:"functionDeclarations,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}
