package anthropicreceiver

import "strings"

// CostContext provides additional context that affects cost calculation.
type CostContext struct {
	Speed           string // "fast" or "standard"
	TotalInputTokens int   // Total input tokens for long context detection
}

// CostResult holds the computed cost breakdown for a request.
type CostResult struct {
	InputCost         float64
	OutputCost        float64
	CacheReadCost     float64
	CacheCreationCost float64
	TotalCost         float64
	CacheSavings      float64
	Multiplier        string // "standard", "fast", "long_context", "fast+long_context"
}

// ComputeCost calculates the cost for a request given model, usage, pricing table, and cost context.
func ComputeCost(model string, usage Usage, pricing map[string]ModelPricing, ctx CostContext) CostResult {
	p := lookupPricing(model, pricing)
	if p == nil {
		return CostResult{}
	}

	result := CostResult{
		InputCost:         float64(usage.InputTokens) * p.InputPerMToken / 1_000_000,
		OutputCost:        float64(usage.OutputTokens) * p.OutputPerMToken / 1_000_000,
		CacheReadCost:     float64(usage.CacheReadInputTokens) * p.CacheReadPerMToken / 1_000_000,
		CacheCreationCost: float64(usage.CacheCreationInputTokens) * p.CacheCreationPerMToken / 1_000_000,
		Multiplier:        "standard",
	}

	isFast := ctx.Speed == "fast"
	isLongContext := ctx.TotalInputTokens > 200_000

	if isFast && isLongContext {
		result.Multiplier = "fast+long_context"
	} else if isFast {
		result.Multiplier = "fast"
	} else if isLongContext {
		result.Multiplier = "long_context"
	}

	// Apply fast mode: 6x all costs
	if isFast {
		result.InputCost *= 6
		result.OutputCost *= 6
		result.CacheReadCost *= 6
		result.CacheCreationCost *= 6
	}

	// Apply long context: 2x input costs, 1.5x output costs
	if isLongContext {
		result.InputCost *= 2
		result.CacheReadCost *= 2
		result.CacheCreationCost *= 2
		result.OutputCost *= 1.5
	}

	result.TotalCost = result.InputCost + result.OutputCost + result.CacheReadCost + result.CacheCreationCost

	// Cache savings: difference between what cache reads would have cost at input price vs cache read price
	baseSavings := float64(usage.CacheReadInputTokens) * (p.InputPerMToken - p.CacheReadPerMToken) / 1_000_000
	if isFast {
		baseSavings *= 6
	}
	if isLongContext {
		baseSavings *= 2
	}
	result.CacheSavings = baseSavings

	return result
}

// lookupPricing finds the pricing for a model, trying exact match first,
// then prefix match to handle versioned model names.
func lookupPricing(model string, pricing map[string]ModelPricing) *ModelPricing {
	if p, ok := pricing[model]; ok {
		return &p
	}
	// Try prefix matching for versioned model names (e.g., "claude-sonnet-4-6-20250514")
	for name, p := range pricing {
		if strings.HasPrefix(model, name) || strings.HasPrefix(name, model) {
			return &p
		}
	}
	return nil
}
