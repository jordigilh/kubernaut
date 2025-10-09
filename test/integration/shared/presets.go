//go:build integration
// +build integration

/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package shared

import (
	"github.com/jordigilh/kubernaut/internal/config"
)

// StandardVectorDBConfig returns the standard vector database configuration for tests
func StandardVectorDBConfig() *config.VectorDBConfig {
	return &config.VectorDBConfig{
		Enabled: true,
		Backend: "postgresql",
		EmbeddingService: config.EmbeddingConfig{
			Service:   "local",
			Dimension: 384,
		},
		PostgreSQL: config.PostgreSQLVectorConfig{
			UseMainDB:  true,
			IndexLists: 50,
		},
	}
}

// TestVectorDBConfig returns a lightweight vector database configuration for testing
func TestVectorDBConfig() *config.VectorDBConfig {
	return &config.VectorDBConfig{
		Enabled: true,
		Backend: "postgresql",
		EmbeddingService: config.EmbeddingConfig{
			Service:   "local",
			Dimension: 128, // Smaller dimension for faster tests
		},
		PostgreSQL: config.PostgreSQLVectorConfig{
			UseMainDB:  true,
			IndexLists: 10, // Fewer index lists for faster setup
		},
	}
}

// PerformanceVectorDBConfig returns a vector database configuration optimized for performance testing
func PerformanceVectorDBConfig() *config.VectorDBConfig {
	return &config.VectorDBConfig{
		Enabled: true,
		Backend: "postgresql",
		EmbeddingService: config.EmbeddingConfig{
			Service:   "local",
			Dimension: 512, // Larger dimension for performance testing
		},
		PostgreSQL: config.PostgreSQLVectorConfig{
			UseMainDB:  true,
			IndexLists: 100, // More index lists for better performance
		},
	}
}

// StandardDatabaseConfig returns the standard database configuration for tests
func StandardDatabaseConfig() *config.DatabaseConfig {
	return &config.DatabaseConfig{
		Host:     "localhost",
		Port:     "5433", // Standard test port
		Username: "postgres",
		Password: "password",
		Database: "action_history",
		SSLMode:  "disable",
		Enabled:  true,
	}
}

// TestDatabaseConfig returns a lightweight database configuration for testing
func TestDatabaseConfig() *config.DatabaseConfig {
	return &config.DatabaseConfig{
		Host:     "localhost",
		Port:     "5433",
		Username: "postgres",
		Password: "password",
		Database: "test_action_history",
		SSLMode:  "disable",
		Enabled:  true,
	}
}

// StandardLLMConfig returns the standard LLM configuration for tests
func StandardLLMConfig() *config.LLMConfig {
	return &config.LLMConfig{
		Provider:    "test",
		Model:       "granite3.1-dense:8b",
		MaxTokens:   1000,
		Temperature: 0.1,
		Timeout:     30,
		RetryCount:  3,
		Endpoint:    "http://192.168.1.169:8080",
	}
}

// FastLLMConfig returns an LLM configuration optimized for fast test execution
func FastLLMConfig() *config.LLMConfig {
	return &config.LLMConfig{
		Provider:    "test",
		Model:       "fake-test-model",
		MaxTokens:   500,
		Temperature: 0.0, // Deterministic for testing
		Timeout:     10,
		RetryCount:  1,
		Endpoint:    "http://192.168.1.169:8080",
	}
}

// PerformanceLLMConfig returns an LLM configuration for performance testing
func PerformanceLLMConfig() *config.LLMConfig {
	return &config.LLMConfig{
		Provider:    "test",
		Model:       "granite3.1-dense:8b",
		MaxTokens:   2000,
		Temperature: 0.1,
		Timeout:     60,
		RetryCount:  5,
		Endpoint:    "http://192.168.1.169:8080",
	}
}

// NOTE: Redis configuration not available in current config structure

// NOTE: Redis configuration not available in current config structure

// StandardEnvironmentVariables returns the standard set of environment variables for tests
func StandardEnvironmentVariables() []string {
	return []string{
		"DB_HOST",
		"DB_PORT",
		"DB_NAME",
		"DB_USER",
		"DB_PASSWORD",
		"REDIS_HOST",
		"REDIS_PORT",
		"VECTOR_DB_HOST",
		"VECTOR_DB_PORT",
	}
}

// TestLLMEnvironmentVariables returns the test LLM-related environment variables
func TestLLMEnvironmentVariables() []string {
	return []string{
		"SLM_ENDPOINT",
		"SLM_MODEL",
		"SLM_API_KEY",
		"SLM_PROVIDER",
		"SLM_MAX_TOKENS",
		"SLM_TEMPERATURE",
		"SLM_TIMEOUT",
	}
}

// PerformanceTestEnvironmentVariables returns environment variables for performance testing
func PerformanceTestEnvironmentVariables() []string {
	base := StandardEnvironmentVariables()
	llm := TestLLMEnvironmentVariables()
	performance := []string{
		"PERFORMANCE_TEST_DURATION",
		"PERFORMANCE_MAX_CONCURRENCY",
		"PERFORMANCE_TARGET_RPS",
		"PERFORMANCE_MEMORY_LIMIT",
	}

	result := make([]string, 0, len(base)+len(llm)+len(performance))
	result = append(result, base...)
	result = append(result, llm...)
	result = append(result, performance...)
	return result
}

// DefaultTestEnvironmentValues returns default values for test environment variables
func DefaultTestEnvironmentValues() map[string]string {
	return map[string]string{
		"DB_HOST":        "localhost",
		"DB_PORT":        "5433",
		"DB_NAME":        "action_history",
		"DB_USER":        "postgres",
		"DB_PASSWORD":    "password",
		"VECTOR_DB_HOST": "localhost",
		"VECTOR_DB_PORT": "5433",
		"SLM_PROVIDER":   "test",
		"SLM_MODEL":      "granite3.1-dense:8b",
		"SLM_TIMEOUT":    "30s",
	}
}
