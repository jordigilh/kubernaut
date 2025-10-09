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
