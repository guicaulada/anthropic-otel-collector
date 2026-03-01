package anthropicreceiver

import (
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
)

// Config defines the configuration for the Anthropic receiver.
type Config struct {
	// ServerConfig configures the HTTP server that listens for client requests.
	confighttp.ServerConfig `mapstructure:",squash"`

	// AnthropicAPI is the upstream Anthropic API URL.
	AnthropicAPI string `mapstructure:"anthropic_api"`

	// CaptureRequestBody enables logging of full request bodies.
	CaptureRequestBody bool `mapstructure:"capture_request_body"`

	// CaptureResponseBody enables logging of full response bodies.
	CaptureResponseBody bool `mapstructure:"capture_response_body"`

	// MaxBodyCaptureSize is the max size in bytes for captured bodies.
	MaxBodyCaptureSize int `mapstructure:"max_body_capture_size"`

	// RedactAPIKey controls whether API keys are redacted in logs.
	RedactAPIKey bool `mapstructure:"redact_api_key"`

	// RateLimitWarningThreshold is the utilization ratio above which a warning is emitted.
	RateLimitWarningThreshold float64 `mapstructure:"rate_limit_warning_threshold"`

	// MaxRequestBodySize is the max size in bytes for incoming request bodies (default: 10MB).
	MaxRequestBodySize int64 `mapstructure:"max_request_body_size"`

	// ParseToolCalls enables parsing of tool_use content blocks for code metrics.
	ParseToolCalls bool `mapstructure:"parse_tool_calls"`

	// IncludeFilePathLabel includes full file paths as metric labels (high cardinality).
	IncludeFilePathLabel bool `mapstructure:"include_file_path_label"`

	// Pricing is the per-model pricing table.
	Pricing map[string]ModelPricing `mapstructure:"pricing"`

	// SessionTimeout is the duration after which an inactive session is considered expired.
	SessionTimeout time.Duration `mapstructure:"session_timeout"`
}

// ModelPricing defines per-token pricing for a model.
type ModelPricing struct {
	InputPerMToken        float64 `mapstructure:"input_per_m_token"`
	OutputPerMToken       float64 `mapstructure:"output_per_m_token"`
	CacheReadPerMToken    float64 `mapstructure:"cache_read_per_m_token"`
	CacheCreationPerMToken float64 `mapstructure:"cache_creation_per_m_token"`
}

// Validate checks the receiver configuration.
func (cfg *Config) Validate() error {
	if cfg.AnthropicAPI == "" {
		return errors.New("anthropic_api must not be empty")
	}
	if cfg.MaxBodyCaptureSize < 0 {
		return fmt.Errorf("max_body_capture_size must be non-negative, got %d", cfg.MaxBodyCaptureSize)
	}
	if cfg.MaxRequestBodySize < 0 {
		return fmt.Errorf("max_request_body_size must be non-negative, got %d", cfg.MaxRequestBodySize)
	}
	if cfg.RateLimitWarningThreshold < 0 || cfg.RateLimitWarningThreshold > 1 {
		return fmt.Errorf("rate_limit_warning_threshold must be between 0 and 1, got %f", cfg.RateLimitWarningThreshold)
	}
	if cfg.SessionTimeout < 0 {
		return fmt.Errorf("session_timeout must be non-negative, got %s", cfg.SessionTimeout)
	}
	for model, pricing := range cfg.Pricing {
		if pricing.InputPerMToken < 0 || pricing.OutputPerMToken < 0 ||
			pricing.CacheReadPerMToken < 0 || pricing.CacheCreationPerMToken < 0 {
			return fmt.Errorf("pricing values for model %q must be non-negative", model)
		}
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		ServerConfig: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "0.0.0.0:4319",
				Transport: confignet.TransportTypeTCP,
			},
		},
		AnthropicAPI:              "https://api.anthropic.com",
		MaxRequestBodySize:        10 * 1024 * 1024, // 10MB
		CaptureRequestBody:        false,
		CaptureResponseBody:       false,
		MaxBodyCaptureSize:        65536,
		RedactAPIKey:              true,
		RateLimitWarningThreshold: 0.8,
		ParseToolCalls:            true,
		IncludeFilePathLabel:      false,
		Pricing:                   defaultPricing(),
		SessionTimeout:            30 * time.Minute,
	}
}

func defaultPricing() map[string]ModelPricing {
	return map[string]ModelPricing{
		"claude-opus-4-6": {
			InputPerMToken:         5.0,
			OutputPerMToken:        25.0,
			CacheReadPerMToken:     0.50,
			CacheCreationPerMToken: 6.25,
		},
		"claude-opus-4-0-20250514": {
			InputPerMToken:         5.0,
			OutputPerMToken:        25.0,
			CacheReadPerMToken:     0.50,
			CacheCreationPerMToken: 6.25,
		},
		"claude-sonnet-4-6": {
			InputPerMToken:         3.0,
			OutputPerMToken:        15.0,
			CacheReadPerMToken:     0.3,
			CacheCreationPerMToken: 3.75,
		},
		"claude-sonnet-4-0-20250514": {
			InputPerMToken:         3.0,
			OutputPerMToken:        15.0,
			CacheReadPerMToken:     0.3,
			CacheCreationPerMToken: 3.75,
		},
		"claude-sonnet-4-5-20250514": {
			InputPerMToken:         3.0,
			OutputPerMToken:        15.0,
			CacheReadPerMToken:     0.3,
			CacheCreationPerMToken: 3.75,
		},
		"claude-haiku-4-5-20251001": {
			InputPerMToken:         1.0,
			OutputPerMToken:        5.0,
			CacheReadPerMToken:     0.10,
			CacheCreationPerMToken: 1.25,
		},
		"claude-3-5-sonnet-20241022": {
			InputPerMToken:         3.0,
			OutputPerMToken:        15.0,
			CacheReadPerMToken:     0.3,
			CacheCreationPerMToken: 3.75,
		},
		"claude-3-5-haiku-20241022": {
			InputPerMToken:         0.80,
			OutputPerMToken:        4.0,
			CacheReadPerMToken:     0.08,
			CacheCreationPerMToken: 1.0,
		},
	}
}
