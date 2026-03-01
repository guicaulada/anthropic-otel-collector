package anthropicreceiver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeCost_KnownModel(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})

	// claude-sonnet-4-6: input=$3/MTok, output=$15/MTok
	expectedInput := 1000.0 * 3.0 / 1_000_000  // 0.003
	expectedOutput := 500.0 * 15.0 / 1_000_000  // 0.0075
	expectedTotal := expectedInput + expectedOutput

	assert.InDelta(t, expectedInput, result.InputCost, 0.0000001)
	assert.InDelta(t, expectedOutput, result.OutputCost, 0.0000001)
	assert.InDelta(t, expectedTotal, result.TotalCost, 0.0000001)
	assert.InDelta(t, 0.0, result.CacheReadCost, 0.0000001)
	assert.InDelta(t, 0.0, result.CacheCreationCost, 0.0000001)
	assert.Equal(t, "standard", result.Multiplier)
}

func TestComputeCost_UnknownModel(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	result := ComputeCost("unknown-model-xyz", usage, pricing, CostContext{})

	assert.Equal(t, 0.0, result.InputCost)
	assert.Equal(t, 0.0, result.OutputCost)
	assert.Equal(t, 0.0, result.TotalCost)
}

func TestComputeCost_WithCacheTokens(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:              1000,
		OutputTokens:             500,
		CacheReadInputTokens:     200,
		CacheCreationInputTokens: 100,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})

	// claude-sonnet-4-6: cache_read=$0.3/MTok, cache_creation=$3.75/MTok
	expectedCacheRead := 200.0 * 0.3 / 1_000_000
	expectedCacheCreation := 100.0 * 3.75 / 1_000_000
	expectedInput := 1000.0 * 3.0 / 1_000_000
	expectedOutput := 500.0 * 15.0 / 1_000_000

	assert.InDelta(t, expectedCacheRead, result.CacheReadCost, 0.0000001)
	assert.InDelta(t, expectedCacheCreation, result.CacheCreationCost, 0.0000001)
	assert.InDelta(t, expectedInput+expectedOutput+expectedCacheRead+expectedCacheCreation, result.TotalCost, 0.0000001)
}

func TestComputeCost_SpecificNumbers(t *testing.T) {
	pricing := defaultPricing()

	// claude-sonnet-4-6: input=$3/MTok, output=$15/MTok
	usage := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})

	assert.InDelta(t, 3.0, result.InputCost, 0.0001)
	assert.InDelta(t, 15.0, result.OutputCost, 0.0001)
	assert.InDelta(t, 18.0, result.TotalCost, 0.0001)
}

func TestComputeCost_PrefixMatch(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	// Model name that is a prefix of a known model or vice-versa
	result := ComputeCost("claude-sonnet-4-6-20250514", usage, pricing, CostContext{})

	// Should match claude-sonnet-4-6 or claude-sonnet-4-0-20250514 via prefix
	assert.Greater(t, result.TotalCost, 0.0, "prefix matching should find pricing")
}

func TestComputeCost_ZeroUsage(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})

	assert.Equal(t, 0.0, result.InputCost)
	assert.Equal(t, 0.0, result.OutputCost)
	assert.Equal(t, 0.0, result.TotalCost)
}

func TestComputeCost_FastMode(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	standard := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})
	fast := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{Speed: "fast"})

	assert.InDelta(t, standard.InputCost*6, fast.InputCost, 0.0000001)
	assert.InDelta(t, standard.OutputCost*6, fast.OutputCost, 0.0000001)
	assert.InDelta(t, standard.TotalCost*6, fast.TotalCost, 0.0000001)
	assert.Equal(t, "fast", fast.Multiplier)
}

func TestComputeCost_LongContext(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	standard := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})
	longCtx := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{TotalInputTokens: 300_000})

	assert.InDelta(t, standard.InputCost*2, longCtx.InputCost, 0.0000001)
	assert.InDelta(t, standard.OutputCost*1.5, longCtx.OutputCost, 0.0000001)
	assert.Equal(t, "long_context", longCtx.Multiplier)
}

func TestComputeCost_CacheSavings(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		CacheReadInputTokens: 1000,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})

	// claude-sonnet-4-6: input=$3/MTok, cache_read=$0.3/MTok
	// savings = 1000 * (3.0 - 0.3) / 1_000_000 = 0.0027
	assert.InDelta(t, 0.0027, result.CacheSavings, 0.0000001)
}

func TestComputeCost_CacheSavings_FastMode(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		CacheReadInputTokens: 1000,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{Speed: "fast"})

	// 0.0027 * 6 = 0.0162
	assert.InDelta(t, 0.0162, result.CacheSavings, 0.0000001)
}

func TestComputeCost_CacheSavings_LongContext(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		CacheReadInputTokens: 1000,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{TotalInputTokens: 300_000})

	// 0.0027 * 2 = 0.0054
	assert.InDelta(t, 0.0054, result.CacheSavings, 0.0000001)
}

func TestComputeCost_CacheSavings_FastPlusLongContext(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		CacheReadInputTokens: 1000,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{Speed: "fast", TotalInputTokens: 300_000})

	// 0.0027 * 12 = 0.0324
	assert.InDelta(t, 0.0324, result.CacheSavings, 0.0000001)
}

func TestComputeCost_CacheSavings_NoCacheTokens(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	result := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})

	assert.InDelta(t, 0.0, result.CacheSavings, 0.0000001)
}

func TestComputeCost_FastPlusLongContext(t *testing.T) {
	pricing := defaultPricing()
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	standard := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{})
	combined := ComputeCost("claude-sonnet-4-6", usage, pricing, CostContext{Speed: "fast", TotalInputTokens: 300_000})

	// Fast 6x * long context 2x = 12x for input
	assert.InDelta(t, standard.InputCost*12, combined.InputCost, 0.0000001)
	// Fast 6x * long context 1.5x = 9x for output
	assert.InDelta(t, standard.OutputCost*9, combined.OutputCost, 0.0000001)
	assert.Equal(t, "fast+long_context", combined.Multiplier)
}
