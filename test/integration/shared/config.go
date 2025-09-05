package shared

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// IntegrationConfig holds configuration for integration tests
type IntegrationConfig struct {
	LLMEndpoint     string
	LLMModel        string
	LLMProvider     string // "ollama", "ramalama", "localai"
	TestTimeout     time.Duration
	MaxRetries      int
	SkipSlowTests   bool
	LogLevel        string
	SkipIntegration bool
}

// LoadConfig loads integration test configuration from environment variables
func LoadConfig() IntegrationConfig {
	endpoint := GetEnvOrDefault("LLM_ENDPOINT", "http://localhost:11434")
	model := GetEnvOrDefault("LLM_MODEL", "granite3.1-dense:8b")
	provider := GetEnvOrDefault("LLM_PROVIDER", detectProviderFromEndpoint(endpoint))

	return IntegrationConfig{
		LLMEndpoint:     endpoint,
		LLMModel:        model,
		LLMProvider:     provider,
		TestTimeout:     getDurationEnvOrDefault("TEST_TIMEOUT", 120*time.Second),
		MaxRetries:      getIntEnvOrDefault("MAX_RETRIES", 3),
		SkipSlowTests:   getBoolEnvOrDefault("SKIP_SLOW_TESTS", false),
		LogLevel:        GetEnvOrDefault("LOG_LEVEL", "debug"),
		SkipIntegration: getBoolEnvOrDefault("SKIP_INTEGRATION", false),
	}
}

// detectProviderFromEndpoint attempts to detect provider type from endpoint
func detectProviderFromEndpoint(endpoint string) string {
	// Parse the endpoint to detect provider based on port
	if strings.Contains(endpoint, ":11434") {
		return "ollama"
	}
	if strings.Contains(endpoint, ":8080") {
		return "localai"
	}
	// Default to localai for other cases
	return "localai"
}

func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDurationEnvOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getIntEnvOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getBoolEnvOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
