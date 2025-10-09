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
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/jordigilh/kubernaut/pkg/testutil/config"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// REFACTORED: Replace local MockSLMClient with generated mock from factory
// Following project guidelines: REUSE existing code and AVOID duplication
// Business Requirements: Support BR-AI-001, BR-AI-002, BR-PA-006 through BR-PA-010

// CreateStandardLLMClientMock creates a standardized LLM client mock using the factory
// Business Requirements: BR-AI-001, BR-AI-002 - Provides consistent AI client behavior
func CreateStandardLLMClientMock() *mocks.LLMClient {
	factory := mocks.NewMockFactory(&mocks.FactoryConfig{
		EnableDetailedLogging: false,
		ErrorSimulation:       false,
	})

	// Standard responses for integration testing
	standardResponses := []string{
		`{"action": "restart_pod", "confidence": 0.8, "reasoning": "High memory usage detected"}`,
		`{"action": "scale_up", "confidence": 0.75, "reasoning": "CPU utilization above threshold"}`,
		`{"action": "notify_only", "confidence": 0.5, "reasoning": "Monitoring alert, no action needed"}`,
	}

	return factory.CreateLLMClient(standardResponses)
}

// CreateHighConfidenceLLMClientMock creates an LLM client mock that meets high confidence thresholds
// Business Requirements: BR-AI-001-CONFIDENCE, BR-AI-002-RECOMMENDATION-CONFIDENCE
func CreateHighConfidenceLLMClientMock(env string) (*mocks.LLMClient, error) {
	thresholds, err := config.LoadThresholds(env)
	if err != nil {
		return nil, err
	}

	factory := mocks.NewMockFactory(&mocks.FactoryConfig{
		EnableDetailedLogging: false,
		BusinessThresholds: &mocks.BusinessThresholds{
			AI: mocks.AIThresholds{
				BRAI001: mocks.AIAnalysisRequirements{
					MinConfidenceScore: thresholds.AI.BRAI001.MinConfidenceScore,
				},
				BRAI002: mocks.AIRecommendationRequirements{
					RecommendationConfidence: thresholds.AI.BRAI002.RecommendationConfidence,
				},
			},
		},
	})

	// High confidence responses that meet business requirements
	highConfidenceResponses := []string{
		`{"action": "restart_pod", "confidence": 0.9, "reasoning": "Critical memory leak detected with high certainty"}`,
		`{"action": "scale_up", "confidence": 0.85, "reasoning": "Consistent CPU spike pattern identified"}`,
	}

	return factory.CreateLLMClient(highConfidenceResponses), nil
}

// CreateFailureSimulationLLMClientMock creates an LLM client mock for testing error scenarios
// Business Requirements: Testing error handling paths for BR-AI-001, BR-AI-002
func CreateFailureSimulationLLMClientMock() *mocks.LLMClient {
	factory := mocks.NewMockFactory(&mocks.FactoryConfig{
		EnableDetailedLogging: false,
		ErrorSimulation:       true,
	})

	// Use factory but override with failure behavior
	mockClient := factory.CreateLLMClient([]string{})

	// Clear default successful behavior and add failure scenarios
	// Following project guidelines: use structured error handling
	mockClient.ExpectedCalls = nil
	mockClient.On("IsHealthy").Return(false)
	mockClient.On("LivenessCheck", mock.Anything).Return(context.DeadlineExceeded)
	mockClient.On("ReadinessCheck", mock.Anything).Return(context.DeadlineExceeded)
	mockClient.On("ChatCompletion", mock.Anything, mock.Anything).Return("", context.DeadlineExceeded)

	return mockClient
}

// GetIntegrationTestThresholds provides environment-specific thresholds for integration tests
// Following project guidelines: configuration-driven business requirement validation
func GetIntegrationTestThresholds(env string) (*config.BusinessThresholds, error) {
	return config.LoadThresholds(env)
}

// ValidateIntegrationTestBusinessRequirement validates business requirements in integration tests
// Following project guidelines: test business requirements, not implementation details
func ValidateIntegrationTestBusinessRequirement(actual interface{}, requirement string, env string, description string) {
	config.ExpectBusinessRequirement(actual, requirement, env, description)
}

// CreateIntegrationTestContext provides standardized context for integration tests
// Business Requirements: Support consistent testing across all BR-XXX-### requirements
func CreateIntegrationTestContext(env string) (*IntegrationTestContext, error) {
	thresholds, err := config.LoadThresholds(env)
	if err != nil {
		return nil, err
	}

	factory := mocks.NewMockFactory(&mocks.FactoryConfig{
		EnableDetailedLogging: false,
		BusinessThresholds: &mocks.BusinessThresholds{
			Database: mocks.DatabaseThresholds{
				BRDatabase001A: mocks.DatabaseUtilizationThresholds{
					UtilizationThreshold: thresholds.Database.BRDatabase001A.UtilizationThreshold,
					MaxOpenConnections:   thresholds.Database.BRDatabase001A.MaxOpenConnections,
					MaxIdleConnections:   thresholds.Database.BRDatabase001A.MaxIdleConnections,
				},
				BRDatabase001B: mocks.DatabasePerformanceThresholds{
					HealthScoreThreshold: thresholds.Database.BRDatabase001B.HealthScoreThreshold,
					HealthyScore:         thresholds.Database.BRDatabase001B.HealthyScore,
					FailureRateThreshold: thresholds.Database.BRDatabase001B.FailureRateThreshold,
					WaitTimeThreshold:    thresholds.Database.BRDatabase001B.WaitTimeThreshold,
				},
			},
			AI: mocks.AIThresholds{
				BRAI001: mocks.AIAnalysisRequirements{
					MinConfidenceScore: thresholds.AI.BRAI001.MinConfidenceScore,
					MaxAnalysisTime:    thresholds.AI.BRAI001.MaxAnalysisTime,
				},
				BRAI002: mocks.AIRecommendationRequirements{
					RecommendationConfidence: thresholds.AI.BRAI002.RecommendationConfidence,
					ActionValidationTime:     thresholds.AI.BRAI002.ActionValidationTime,
				},
			},
		},
	})

	return &IntegrationTestContext{
		Environment:     env,
		Thresholds:      thresholds,
		MockFactory:     factory,
		LLMClient:       factory.CreateLLMClient([]string{"Integration test response"}),
		DatabaseMonitor: factory.CreateDatabaseMonitor(),
		SafetyValidator: factory.CreateSafetyValidator(),
	}, nil
}

// IntegrationTestContext provides comprehensive test context for integration testing
type IntegrationTestContext struct {
	Environment     string
	Thresholds      *config.BusinessThresholds
	MockFactory     *mocks.MockFactory
	LLMClient       *mocks.LLMClient
	DatabaseMonitor *mocks.DatabaseMonitor
	SafetyValidator *mocks.SafetyValidator
}

// ValidateBusinessRequirements validates that the test context meets all business requirements
func (ctx *IntegrationTestContext) ValidateBusinessRequirements() error {
	return config.ValidateThresholdConfiguration(ctx.Environment)
}

// CreateBusinessScenario creates a mock scenario that meets specific business requirements
func (ctx *IntegrationTestContext) CreateBusinessScenario(scenarioType string) error {
	switch scenarioType {
	case "high_confidence_ai":
		// Configure LLM client for high confidence scenarios
		ctx.LLMClient.ExpectedCalls = nil
		ctx.LLMClient.On("ChatCompletion", mock.Anything, mock.Anything).
			Return(`{"action": "restart_pod", "confidence": 0.95, "reasoning": "High confidence scenario"}`, nil)

	case "database_performance_stress":
		// Configure database monitor for performance stress scenarios
		ctx.DatabaseMonitor.ExpectedCalls = nil
		ctx.DatabaseMonitor.On("GetMetrics").Return(ctx.MockFactory.CreateDatabaseMonitor().GetMetrics())

	case "safety_validation_required":
		// Configure safety validator for scenarios requiring validation
		ctx.SafetyValidator.ExpectedCalls = nil
		ctx.SafetyValidator.On("AssessActionRisk", mock.Anything, mock.Anything).
			Return(ctx.MockFactory.CreateSafetyValidator().AssessActionRisk(context.Background(), "test_action"))
	}

	return nil
}
