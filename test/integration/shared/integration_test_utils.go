package shared

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockSLMClient provides a mock SLM client for testing
type MockSLMClient struct{}

func (m *MockSLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	// Convert interface{} to types.Alert for processing
	var typedAlert types.Alert
	if a, ok := alert.(types.Alert); ok {
		typedAlert = a
	} else {
		return nil, fmt.Errorf("invalid alert type provided to mock SLM client")
	}

	_ = typedAlert // Use the typed alert (currently unused in mock)

	return &llm.AnalyzeAlertResponse{
		Action:     "notify_only",
		Confidence: 0.5,
		Parameters: make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"reasoning": "Mock integration test response",
		},
	}, nil
}

func (m *MockSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	return `{"action": "notify_only", "confidence": 0.5, "reasoning": "Mock chat completion"}`, nil
}

func (m *MockSLMClient) IsHealthy() bool {
	return true
}

// Health monitoring methods required for llm.Client interface

// LivenessCheck performs a liveness check simulation
func (m *MockSLMClient) LivenessCheck(ctx context.Context) error {
	return nil // Always healthy for mock
}

// ReadinessCheck performs a readiness check simulation
func (m *MockSLMClient) ReadinessCheck(ctx context.Context) error {
	return nil // Always ready for mock
}

// GetEndpoint returns the simulated endpoint for the mock client
func (m *MockSLMClient) GetEndpoint() string {
	return "http://mock-slm-client:8080"
}

// GetModel returns the simulated model name for the mock client
func (m *MockSLMClient) GetModel() string {
	return "mock-model"
}

// GetMinParameterCount returns the minimum parameter count for mock testing
func (m *MockSLMClient) GetMinParameterCount() int64 {
	return 1000000000 // 1B parameters for mock model
}

// GenerateResponse provides a simple response generation for compatibility
func (m *MockSLMClient) GenerateResponse(prompt string) (string, error) {
	return "Mock response for: " + prompt, nil
}

// GenerateWorkflow implements the LLM interface for workflow generation
func (m *MockSLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	return &llm.WorkflowGenerationResult{
		WorkflowID:  "mock-workflow",
		Success:     true,
		GeneratedAt: "2025-09-15T13:30:00Z",
		StepCount:   1,
		Name:        "Mock Workflow",
		Description: "Generated mock workflow for testing",
	}, nil
}

// MockK8sTestEnvironment provides a mock K8s test environment
type MockK8sTestEnvironment struct {
	Client interface{} // Placeholder client
}

// NOTE: IntegrationTestUtils and NewIntegrationTestUtils are defined in database_test_utils.go
// They should be accessible as shared.IntegrationTestUtils and shared.NewIntegrationTestUtils
// from external packages. This file contains only helper types and mock implementations.
