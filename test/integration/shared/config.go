package shared

import (
	"os"
	"strconv"
	"time"
)

// IntegrationConfig holds configuration for integration tests
type IntegrationConfig struct {
	OllamaEndpoint  string
	OllamaModel     string
	TestTimeout     time.Duration
	MaxRetries      int
	SkipSlowTests   bool
	LogLevel        string
	SkipIntegration bool
	// External MCP server configuration
	ExternalMCPServerEndpoint string
}

// LoadConfig loads integration test configuration from environment variables
func LoadConfig() IntegrationConfig {
	return IntegrationConfig{
		OllamaEndpoint:  GetEnvOrDefault("OLLAMA_ENDPOINT", "http://localhost:11434"),
		OllamaModel:     GetEnvOrDefault("OLLAMA_MODEL", "granite3.1-dense:8b"),
		TestTimeout:     getDurationEnvOrDefault("TEST_TIMEOUT", 120*time.Second),
		MaxRetries:      getIntEnvOrDefault("MAX_RETRIES", 3),
		SkipSlowTests:   getBoolEnvOrDefault("SKIP_SLOW_TESTS", false),
		LogLevel:        GetEnvOrDefault("LOG_LEVEL", "debug"),
		SkipIntegration: getBoolEnvOrDefault("SKIP_INTEGRATION", false),
		// External MCP server configuration
		ExternalMCPServerEndpoint: GetEnvOrDefault("EXTERNAL_MCP_ENDPOINT", "http://localhost:8080"),
	}
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
