package shared

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockSLMClient provides a mock SLM client for testing
type MockSLMClient struct{}

func (m *MockSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	return &types.ActionRecommendation{
		Action:     "notify_only",
		Confidence: 0.5,
		Reasoning:  &types.ReasoningDetails{Summary: "Mock integration test response"},
	}, nil
}

func (m *MockSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	return `{"action": "notify_only", "confidence": 0.5, "reasoning": "Mock chat completion"}`, nil
}

func (m *MockSLMClient) IsHealthy() bool {
	return true
}

// MockK8sTestEnvironment provides a mock K8s test environment
type MockK8sTestEnvironment struct {
	Client interface{} // Placeholder client
}

// NOTE: IntegrationTestUtils and NewIntegrationTestUtils are defined in database_test_utils.go
// They should be accessible as shared.IntegrationTestUtils and shared.NewIntegrationTestUtils
// from external packages. This file contains only helper types and mock implementations.
