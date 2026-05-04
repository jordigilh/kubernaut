package ratelimit

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all rate limiting configuration.
type Config struct {
	PerIP       PerIPConfig       `yaml:"perIP"`
	PerUser     PerUserConfig     `yaml:"perUser"`
	PerProvider PerProviderConfig `yaml:"perProvider"`
	Global      GlobalConfig      `yaml:"global"`
}

// PerIPConfig configures pre-authentication per-IP rate limiting.
type PerIPConfig struct {
	RequestsPerSecond float64       `yaml:"requestsPerSecond"`
	Burst             int           `yaml:"burst"`
	CleanupInterval   time.Duration `yaml:"cleanupInterval"`
	MaxAge            time.Duration `yaml:"maxAge"`
}

// PerUserConfig configures post-authentication per-user rate limiting.
type PerUserConfig struct {
	RequestsPerMinute     int           `yaml:"requestsPerMinute"`
	MaxConcurrentSessions int           `yaml:"maxConcurrentSessions"`
	ToolCallsPerMinute    int           `yaml:"toolCallsPerMinute"`
	CleanupInterval       time.Duration `yaml:"cleanupInterval"`
	MaxAge                time.Duration `yaml:"maxAge"`
}

// PerProviderConfig configures per-OIDC-provider JWKS fetch rate limiting.
type PerProviderConfig struct {
	FetchIntervalSeconds int `yaml:"fetchIntervalSeconds"`
}

// GlobalConfig configures global LLM concurrency limits.
type GlobalConfig struct {
	MaxLLMConcurrency  int  `yaml:"maxLLMConcurrency"`
	TokenBudgetEnabled bool `yaml:"tokenBudgetEnabled"`
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
			RequestsPerMinute:     30,
			MaxConcurrentSessions: 3,
			ToolCallsPerMinute:    60,
			CleanupInterval:       5 * time.Minute,
			MaxAge:                10 * time.Minute,
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

// LoadRateLimitConfigFromFile reads and parses a rate limit Config from a YAML file.
func LoadRateLimitConfigFromFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read ratelimit config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse ratelimit config: %w", err)
	}
	return cfg, nil
}
