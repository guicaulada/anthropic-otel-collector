package anthropicreceiver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := defaultConfig()
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("empty anthropic_api", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.AnthropicAPI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "anthropic_api must not be empty")
	})

	t.Run("negative max_body_capture_size", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.MaxBodyCaptureSize = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_body_capture_size must be non-negative")
	})

	t.Run("zero max_body_capture_size is valid", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.MaxBodyCaptureSize = 0
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("rate_limit_warning_threshold below zero", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.RateLimitWarningThreshold = -0.1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate_limit_warning_threshold must be between 0 and 1")
	})

	t.Run("rate_limit_warning_threshold above one", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.RateLimitWarningThreshold = 1.1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate_limit_warning_threshold must be between 0 and 1")
	})

	t.Run("rate_limit_warning_threshold zero is valid", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.RateLimitWarningThreshold = 0
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("rate_limit_warning_threshold one is valid", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.RateLimitWarningThreshold = 1
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("negative pricing input", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.Pricing["test-model"] = ModelPricing{
			InputPerMToken: -1.0,
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pricing values for model")
		assert.Contains(t, err.Error(), "must be non-negative")
	})

	t.Run("negative pricing output", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.Pricing["test-model"] = ModelPricing{
			OutputPerMToken: -1.0,
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be non-negative")
	})

	t.Run("negative pricing cache read", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.Pricing["test-model"] = ModelPricing{
			CacheReadPerMToken: -1.0,
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be non-negative")
	})

	t.Run("negative pricing cache creation", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.Pricing["test-model"] = ModelPricing{
			CacheCreationPerMToken: -1.0,
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be non-negative")
	})

	t.Run("zero pricing is valid", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.Pricing["test-model"] = ModelPricing{}
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("negative max_request_body_size", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.MaxRequestBodySize = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_request_body_size must be non-negative")
	})

	t.Run("zero max_request_body_size is valid", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.MaxRequestBodySize = 0
		err := cfg.Validate()
		require.NoError(t, err)
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	assert.Equal(t, "0.0.0.0:4319", cfg.ServerConfig.NetAddr.Endpoint)
	assert.Equal(t, "https://api.anthropic.com", cfg.AnthropicAPI)
	assert.Equal(t, int64(10*1024*1024), cfg.MaxRequestBodySize)
	assert.False(t, cfg.CaptureRequestBody)
	assert.False(t, cfg.CaptureResponseBody)
	assert.Equal(t, 65536, cfg.MaxBodyCaptureSize)
	assert.True(t, cfg.RedactAPIKey)
	assert.Equal(t, 0.8, cfg.RateLimitWarningThreshold)
	assert.True(t, cfg.ParseToolCalls)
	assert.False(t, cfg.IncludeFilePathLabel)
	assert.NotEmpty(t, cfg.Pricing)

	// Verify at least one known model is in pricing
	_, ok := cfg.Pricing["claude-sonnet-4-6"]
	assert.True(t, ok, "expected claude-sonnet-4-6 in default pricing")
}
