package shared

import (
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/testutil/hybrid"
)

// CreateIntegrationTestLLMClient creates a standardized LLM client for integration testing
// Following project guidelines: REUSE existing mock infrastructure, AVOID duplication
// Replaces the duplicate MockSLMClient with centralized hybrid approach
func CreateIntegrationTestLLMClient() llm.Client {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce test noise
	return hybrid.CreateLLMClient(logger)
}

// NewMockSLMClient creates a mock SLM client using centralized infrastructure
// DEPRECATED: Use CreateIntegrationTestLLMClient() instead for new code
func NewMockSLMClient() llm.Client {
	return CreateIntegrationTestLLMClient()
}

// MockK8sTestEnvironment provides a mock K8s test environment
type MockK8sTestEnvironment struct {
	Client interface{} // Placeholder client
}

// NOTE: IntegrationTestUtils and NewIntegrationTestUtils are defined in database_test_utils.go
// They should be accessible as shared.IntegrationTestUtils and shared.NewIntegrationTestUtils
// from external packages. This file contains only helper types and mock implementations.
