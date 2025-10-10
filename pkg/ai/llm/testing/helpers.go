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

package testing

import (
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/sirupsen/logrus"
)

// CreateMockLLMClient creates an LLM client with mock configuration for testing
func CreateMockLLMClient(logger *logrus.Logger) (llm.Client, error) {
	config := config.LLMConfig{
		Provider:    "mock",
		Model:       "mock-model",
		Temperature: 0.1,
	}

	return llm.NewClient(config, logger)
}

// CreateTestRegistry creates a provider registry that includes the mock provider for testing
// @deprecated: Registry pattern no longer used - use CreateMockLLMClient instead
func CreateTestRegistry() interface{} {
	// Placeholder for backwards compatibility
	return nil
}

// CreateMockLLMClientWithRegistry creates an LLM client using mock configuration
func CreateMockLLMClientWithRegistry(logger *logrus.Logger) (llm.Client, error) {
	return CreateMockLLMClient(logger)
}
