package main

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TDD RED PHASE: Business Logic Integration Tests
// Business Requirements:
// - BR-HEALTH-001: LLM health monitoring integration
// - BR-AI-CONFIDENCE-001: AI confidence validation integration
// - BR-AI-SERVICE-001: Enhanced AI service capabilities

func TestAIServiceBusinessLogicIntegration(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Set environment variable to avoid metrics registration conflicts in tests
	t.Setenv("HEALTH_MONITORING_ENABLED", "false")

	t.Run("BR-HEALTH-001: LLMHealthMonitor integration", func(t *testing.T) {
		// TDD RED: This test MUST fail initially
		// Enable health monitoring for this specific test
		t.Setenv("HEALTH_MONITORING_ENABLED", "true")

		aiService := NewAIService(log)
		ctx := context.Background()

		err := aiService.Initialize(ctx)
		require.NoError(t, err)

		// Verify LLMHealthMonitor is integrated
		assert.NotNil(t, aiService.healthMonitor, "LLMHealthMonitor should be integrated")

		// Test enhanced health status
		healthStatus, err := aiService.GetEnhancedHealthStatus(ctx)
		require.NoError(t, err)

		// Validate health status structure
		assert.NotNil(t, healthStatus)
		assert.NotEmpty(t, healthStatus.ComponentType)
		assert.NotEmpty(t, healthStatus.ServiceEndpoint)
		assert.GreaterOrEqual(t, healthStatus.ResponseTime, float64(0))
		assert.GreaterOrEqual(t, healthStatus.UptimePercentage, float64(0))

		// Validate health status values
		assert.IsType(t, true, healthStatus.IsHealthy)
		assert.IsType(t, "", healthStatus.ComponentType)
		assert.IsType(t, float64(0), healthStatus.UptimePercentage)
	})

	t.Run("BR-AI-CONFIDENCE-001: ConfidenceValidator integration", func(t *testing.T) {
		// TDD RED: This test MUST fail initially
		aiService := NewAIService(log)
		ctx := context.Background()

		err := aiService.Initialize(ctx)
		require.NoError(t, err)

		// Verify ConfidenceValidator is integrated
		assert.NotNil(t, aiService.confidenceValidator, "ConfidenceValidator should be integrated")

		// Test confidence validation
		mockResponse := &llm.AnalyzeAlertResponse{
			Action:     "scale_deployment",
			Confidence: 0.85,
			Reasoning:  &types.ReasoningDetails{Summary: "Test reasoning"},
			Parameters: map[string]interface{}{"replicas": 3},
		}

		result, err := aiService.ValidateResponseConfidence(mockResponse, "high")
		require.NoError(t, err)

		// Validate confidence validation result
		assert.NotNil(t, result)
		assert.Equal(t, "ai_confidence_validation", result.Name)
		assert.Equal(t, engine.PostConditionConfidence, result.Type)
		assert.True(t, result.Satisfied, "High confidence should satisfy validation")
		assert.Equal(t, 0.85, result.Value)
		assert.Contains(t, result.Message, "meets threshold")
	})

	t.Run("BR-AI-SERVICE-001: Enhanced AI service capabilities", func(t *testing.T) {
		// TDD RED: This test MUST fail initially
		// Override environment for this specific test (disable health monitoring to avoid metrics conflicts)
		t.Setenv("HEALTH_MONITORING_ENABLED", "false")
		t.Setenv("CONFIDENCE_VALIDATION_ENABLED", "true")

		aiService := NewAIService(log)
		ctx := context.Background()

		err := aiService.Initialize(ctx)
		require.NoError(t, err)

		// Test business logic configuration loading
		config := loadBusinessLogicConfig()
		assert.NotNil(t, config)
		assert.False(t, config.HealthMonitoring.Enabled) // Disabled to avoid metrics conflicts in tests
		assert.True(t, config.ConfidenceValidation.Enabled)
		assert.Equal(t, 30*time.Second, config.HealthMonitoring.CheckInterval)
		assert.Equal(t, 0.7, config.ConfidenceValidation.MinConfidence)

		// Test configuration validation
		assert.Contains(t, config.ConfidenceValidation.Thresholds, "critical")
		assert.Contains(t, config.ConfidenceValidation.Thresholds, "high")
		assert.Contains(t, config.ConfidenceValidation.Thresholds, "medium")
		assert.Contains(t, config.ConfidenceValidation.Thresholds, "low")
	})

	t.Run("BR-AI-CONFIDENCE-001: Confidence validation with different thresholds", func(t *testing.T) {
		// TDD RED: This test MUST fail initially
		aiService := NewAIService(log)
		ctx := context.Background()

		err := aiService.Initialize(ctx)
		require.NoError(t, err)

		testCases := []struct {
			name       string
			confidence float64
			severity   string
			shouldPass bool
		}{
			{"Critical high confidence", 0.95, "critical", true},
			{"Critical low confidence", 0.6, "critical", false},
			{"High medium confidence", 0.8, "high", true},
			{"Medium low confidence", 0.65, "medium", false},
			{"Low confidence passes", 0.6, "low", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				mockResponse := &llm.AnalyzeAlertResponse{
					Action:     "scale_deployment",
					Confidence: tc.confidence,
					Reasoning:  &types.ReasoningDetails{Summary: "Test reasoning"},
					Parameters: map[string]interface{}{"replicas": 3},
				}

				result, err := aiService.ValidateResponseConfidence(mockResponse, tc.severity)
				require.NoError(t, err)

				assert.Equal(t, tc.shouldPass, result.Satisfied,
					"Confidence %.2f for severity %s should pass: %v",
					tc.confidence, tc.severity, tc.shouldPass)
			})
		}
	})

	t.Run("BR-HEALTH-001: Health monitoring with real LLM client", func(t *testing.T) {
		// TDD RED: This test MUST fail initially
		aiService := NewAIService(log)
		ctx := context.Background()

		err := aiService.Initialize(ctx)
		require.NoError(t, err)

		// Test health monitoring with different client states
		if aiService.llmClient != nil {
			// Real LLM client available
			healthStatus, err := aiService.GetEnhancedHealthStatus(ctx)
			require.NoError(t, err)

			assert.NotEqual(t, "fallback", healthStatus.ComponentType)
			assert.NotEqual(t, "internal", healthStatus.ServiceEndpoint)
		} else {
			// Fallback client only
			healthStatus, err := aiService.GetEnhancedHealthStatus(ctx)
			require.NoError(t, err)

			assert.Equal(t, "fallback", healthStatus.ComponentType)
			assert.Equal(t, "internal", healthStatus.ServiceEndpoint)
		}
	})
}

// Test helper functions for TDD RED phase
func TestBusinessLogicConfigurationHelpers(t *testing.T) {
	t.Run("Environment variable parsing helpers", func(t *testing.T) {
		// TDD RED: These functions MUST fail initially

		// Test boolean parsing
		result := getEnvOrDefaultBool("NONEXISTENT_BOOL", true)
		assert.True(t, result)

		// Test integer parsing
		intResult := getEnvOrDefaultInt("NONEXISTENT_INT", 42)
		assert.Equal(t, 42, intResult)

		// Test float parsing
		floatResult := getEnvOrDefaultFloat("NONEXISTENT_FLOAT", 3.14)
		assert.Equal(t, 3.14, floatResult)

		// Test duration parsing
		durationResult := getEnvOrDefaultDuration("NONEXISTENT_DURATION", 30*time.Second)
		assert.Equal(t, 30*time.Second, durationResult)
	})
}

// Test type definitions that MUST exist for tests to compile
func TestRequiredTypeDefinitions(t *testing.T) {
	t.Run("BusinessLogicConfig type exists", func(t *testing.T) {
		// TDD RED: This type MUST be defined for test to compile
		var config *BusinessLogicConfig
		assert.Nil(t, config) // Will fail if type doesn't exist
	})

	t.Run("AIService has required fields", func(t *testing.T) {
		// TDD RED: These fields MUST exist for test to compile
		aiService := &AIService{}

		// These will cause compilation errors if fields don't exist
		_ = aiService.healthMonitor
		_ = aiService.confidenceValidator
		_ = aiService.llmClient
		_ = aiService.fallbackClient
		_ = aiService.log
		_ = aiService.startTime
	})
}
