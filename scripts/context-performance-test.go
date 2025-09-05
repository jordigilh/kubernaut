package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

func main() {
	// Setup logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Get configuration from environment
	llmEndpoint := getEnvOrDefault("LLM_ENDPOINT", "http://localhost:11434")
	llmModel := getEnvOrDefault("LLM_MODEL", "granite3.1-dense:8b")

	logger.WithFields(logrus.Fields{
		"endpoint": llmEndpoint,
		"model":    llmModel,
	}).Info("Starting context size performance comparison")

	// Test different context sizes
	contextSizes := []int{
		0,     // Unlimited (baseline)
		16000, // Large context
		8000,  // Target reduced context
		4000,  // Small context
	}

	results := make(map[int]PerformanceResult)

	for _, contextSize := range contextSizes {
		result := testContextSize(contextSize, llmEndpoint, llmModel, logger)
		results[contextSize] = result

		sizeLabel := fmt.Sprintf("%d", contextSize)
		if contextSize == 0 {
			sizeLabel = "unlimited"
		}

		logger.WithFields(logrus.Fields{
			"context_size":  sizeLabel,
			"response_time": result.ResponseTime,
			"action":        result.Action,
			"confidence":    result.Confidence,
			"tokens":        result.TokensUsed,
		}).Info("Context size test completed")
	}

	// Analyze results
	baselineTime := results[0].ResponseTime

	fmt.Printf("\n=== Context Size Performance Analysis ===\n")
	fmt.Printf("Baseline (unlimited): %v\n", baselineTime)

	for _, contextSize := range contextSizes[1:] {
		result := results[contextSize]
		speedupRatio := float64(baselineTime) / float64(result.ResponseTime)

		fmt.Printf("Context %dk: %v (speedup: %.2fx)\n",
			contextSize/1000, result.ResponseTime, speedupRatio)
	}

	// Specific analysis for 8K context
	result8K := results[8000]
	speedup8K := float64(baselineTime) / float64(result8K.ResponseTime)

	fmt.Printf("\n=== 8K Context Analysis ===\n")
	fmt.Printf("8K Response Time: %v\n", result8K.ResponseTime)
	fmt.Printf("Speedup vs Unlimited: %.2fx\n", speedup8K)
	fmt.Printf("Action Quality: %s (confidence: %.2f)\n", result8K.Action, result8K.Confidence)

	if speedup8K > 1.0 {
		fmt.Printf("✅ 8K context shows performance improvement\n")
	} else if speedup8K > 0.8 {
		fmt.Printf("⚠️  8K context shows minimal performance impact\n")
	} else {
		fmt.Printf("❌ 8K context shows performance degradation\n")
	}
}

type PerformanceResult struct {
	ContextSize  int
	ResponseTime time.Duration
	TokensUsed   int
	Action       string
	Confidence   float64
}

func testContextSize(contextSize int, endpoint, model string, logger *logrus.Logger) PerformanceResult {
	// Create SLM config with specific context size
	llmConfig := config.LLMConfig{
		Endpoint:       endpoint,
		Model:          model,
		Provider:       "localai",
		Timeout:        30 * time.Second,
		RetryCount:     1,
		Temperature:    0.3,
		MaxTokens:      500,
		MaxContextSize: contextSize,
	}

	// Create SLM client
	llmClient, err := llm.NewClient(llmConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create LLM client: %v", err)
	}

	// Create test alert
	alert := types.Alert{
		Name:        "HighMemoryUsage",
		Status:      "firing",
		Severity:    "warning",
		Description: "Memory usage above 80% for deployment webapp",
		Namespace:   "production",
		Resource:    "webapp",
		Labels: map[string]string{
			"alertname":  "HighMemoryUsage",
			"deployment": "webapp",
			"namespace":  "production",
			"severity":   "warning",
		},
		Annotations: map[string]string{
			"description": "Memory usage above 80% for deployment webapp",
			"summary":     "High memory usage detected",
		},
	}

	// Measure performance
	startTime := time.Now()
	recommendation, err := llmClient.AnalyzeAlert(context.Background(), alert)
	responseTime := time.Since(startTime)

	if err != nil {
		log.Fatalf("Failed to analyze alert: %v", err)
	}

	if recommendation == nil {
		log.Fatalf("Received nil recommendation")
	}

	return PerformanceResult{
		ContextSize:  contextSize,
		ResponseTime: responseTime,
		TokensUsed:   0, // Would need to extract from response if available
		Action:       recommendation.Action,
		Confidence:   recommendation.Confidence,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
