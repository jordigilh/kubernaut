package shared

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// IntegrationConfig holds configuration for integration tests
type IntegrationConfig struct {
	LLMEndpoint       string
	LLMModel          string
	LLMProvider       string // "ollama", "ramalama", "localai", "mock"
	TestTimeout       time.Duration
	MaxRetries        int
	SkipSlowTests     bool
	SkipDatabaseTests bool
	UseContainerDB    bool
	LogLevel          string
	SkipIntegration   bool
	UseMockLLM        bool // Use mock LLM for CI/CD
	UseRealK8s        bool // Use real Kind cluster instead of fake K8s client
	CIMode            bool // Running in CI/CD environment
}

// LoadConfig loads integration test configuration from environment variables
func LoadConfig() IntegrationConfig {
	// Detect CI/CD mode
	ciMode := getBoolEnvOrDefault("CI", false) || getBoolEnvOrDefault("GITHUB_ACTIONS", false)

	// Configure LLM settings based on environment
	var endpoint, model, provider string
	var useMockLLM bool

	if ciMode || getBoolEnvOrDefault("USE_MOCK_LLM", false) {
		// CI/CD mode: use mock LLM
		endpoint = "mock://localhost:8080"
		model = "mock-model"
		provider = "mock"
		useMockLLM = true
	} else {
		// Local development: use real LLM at ramalama endpoint
		endpoint = GetEnvOrDefault("LLM_ENDPOINT", "http://192.168.1.169:8080")
		model = GetEnvOrDefault("LLM_MODEL", "ggml-org/gpt-oss-20b-GGUF")
		provider = GetEnvOrDefault("LLM_PROVIDER", detectProviderFromEndpoint(endpoint))
		useMockLLM = false
	}

	// Default to real K8s (Kind) for integration tests
	useRealK8s := !getBoolEnvOrDefault("USE_FAKE_K8S_CLIENT", false)

	return IntegrationConfig{
		LLMEndpoint:       endpoint,
		LLMModel:          model,
		LLMProvider:       provider,
		TestTimeout:       getDurationEnvOrDefault("TEST_TIMEOUT", 120*time.Second),
		MaxRetries:        getIntEnvOrDefault("MAX_RETRIES", 3),
		SkipSlowTests:     getBoolEnvOrDefault("SKIP_SLOW_TESTS", false),
		SkipDatabaseTests: getBoolEnvOrDefault("SKIP_DB_TESTS", false),
		UseContainerDB:    getBoolEnvOrDefault("USE_CONTAINER_DB", true),
		LogLevel:          GetEnvOrDefault("LOG_LEVEL", "debug"),
		SkipIntegration:   getBoolEnvOrDefault("SKIP_INTEGRATION", false),
		UseMockLLM:        useMockLLM,
		UseRealK8s:        useRealK8s,
		CIMode:            ciMode,
	}
}

// detectProviderFromEndpoint attempts to detect provider type from endpoint
func detectProviderFromEndpoint(endpoint string) string {
	// Handle mock provider
	if strings.HasPrefix(endpoint, "mock://") {
		return "mock"
	}

	// Parse the endpoint to detect provider based on port
	if strings.Contains(endpoint, ":11434") {
		return "ollama"
	}
	if strings.Contains(endpoint, ":8080") {
		return "ramalama" // Default for port 8080 with ramalama deployment
	}
	// Default to ramalama for other cases
	return "ramalama"
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
