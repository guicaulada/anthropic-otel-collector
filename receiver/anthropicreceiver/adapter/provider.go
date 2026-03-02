package adapter

import "net/http"

// ProviderAdapter defines the interface for LLM provider adapters.
type ProviderAdapter interface {
	Name() string
	Detect(req *http.Request, body []byte) bool
	ParseRequest(body []byte) (*LLMRequest, error)
	ParseResponse(body []byte, isStreaming bool) (*LLMResponse, error)
	ExtractUsage(resp *LLMResponse) Usage
	CalculateCost(usage Usage, model string, pricing map[string]ModelPricing) float64
	GetUpstreamURL() string
	ExtractContext(req *LLMRequest) map[string]string
}

type LLMRequest struct {
	Model        string
	Messages     int
	SystemPrompt string
	Tools        []string
	Headers      map[string]string
	Stream       bool
	RawBody      []byte
}

type LLMResponse struct {
	Model        string
	Content      string
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason string
	RawBody      []byte
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
}

type ToolCall struct {
	Name      string
	Arguments map[string]interface{}
}

type ModelPricing struct {
	InputPerMToken         float64
	OutputPerMToken        float64
	CacheReadPerMToken     float64
	CacheCreationPerMToken float64
}
