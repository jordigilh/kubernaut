package ratelimit

import "time"

// Config holds all rate limiting configuration.
type Config struct {
	PerIP       PerIPConfig
	PerUser     PerUserConfig
	PerProvider PerProviderConfig
	Global      GlobalConfig
}

// PerIPConfig configures pre-authentication per-IP rate limiting.
type PerIPConfig struct {
	RequestsPerSecond float64
	Burst             int
	CleanupInterval   time.Duration
	MaxAge            time.Duration
}

// PerUserConfig configures post-authentication per-user rate limiting.
type PerUserConfig struct {
	RequestsPerMinute    int
	MaxConcurrentSessions int
	ToolCallsPerMinute   int
}

// PerProviderConfig configures per-OIDC-provider JWKS fetch rate limiting.
type PerProviderConfig struct {
	FetchIntervalSeconds int
}

// GlobalConfig configures global LLM concurrency limits.
type GlobalConfig struct {
	MaxLLMConcurrency int
	TokenBudgetEnabled bool
}

// DefaultConfig returns sensible defaults matching ARCHITECTURE.md.
func DefaultConfig() Config {
	return Config{
		PerIP: PerIPConfig{
			RequestsPerSecond: 10,
			Burst:             20,
			CleanupInterval:   5 * time.Minute,
			MaxAge:            10 * time.Minute,
		},
		PerUser: PerUserConfig{
			RequestsPerMinute:    30,
			MaxConcurrentSessions: 3,
			ToolCallsPerMinute:   60,
		},
		PerProvider: PerProviderConfig{
			FetchIntervalSeconds: 300,
		},
		Global: GlobalConfig{
			MaxLLMConcurrency:  10,
			TokenBudgetEnabled: false,
		},
	}
}
