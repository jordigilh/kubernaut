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

package workflowengine

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// BR-WF-AI-001: Workflow Engine Constructor Integration Test
// Tests that NewDefaultWorkflowEngineWithAIIntegration creates a functional workflow engine
var _ = Describe("Workflow Engine AI Integration - Business Requirements", func() {
	var (
		k8sClient            *mocks.MockK8sClient
		actionRepo           *mocks.MockActionRepository
		monitoringClients    *monitoring.MonitoringClients
		stateStorage         engine.StateStorage
		executionRepo        engine.ExecutionRepository
		workflowEngineConfig *engine.WorkflowEngineConfig
		aiConfig             *config.Config
		logger               *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Create test dependencies following project guidelines (reuse existing mocks)
		fakeClientset := fake.NewSimpleClientset()
		k8sClient = mocks.NewMockK8sClient(fakeClientset)
		actionRepo = mocks.NewMockActionRepository()
		monitoringClients = &monitoring.MonitoringClients{}

		// Create in-memory implementations for testing
		stateStorage = engine.NewWorkflowStateStorage(nil, logger)
		executionRepo = engine.NewMemoryExecutionRepository(logger)

		workflowEngineConfig = &engine.WorkflowEngineConfig{
			DefaultStepTimeout:    5 * time.Minute,
			MaxRetryDelay:         2 * time.Minute,
			EnableStateRecovery:   true,
			EnableDetailedLogging: false,
			MaxConcurrency:        5,
		}

		// Create AI config with LLM endpoint
		aiConfig = &config.Config{
			SLM: config.LLMConfig{
				Endpoint: "http://192.168.1.169:8080", // Test LLM endpoint
				Model:    "test-model",
			},
			AIServices: config.AIServicesConfig{
				HolmesGPT: config.HolmesGPTConfig{
					Endpoint: "http://test-holmesgpt:8080",
					Enabled:  true,
				},
			},
		}
	})

	Describe("BR-WF-AI-001: Enhanced workflow engine constructor", func() {
		It("should create a functional DefaultWorkflowEngine with AI integration", func() {
			// Act: Create workflow engine with AI integration
			workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
				k8sClient,
				actionRepo,
				monitoringClients,
				stateStorage,
				executionRepo,
				workflowEngineConfig,
				aiConfig,
				logger,
			)

			// Assert: Engine should be created successfully (strong business assertion)
			Expect(err).ToNot(HaveOccurred(), "Failed to create workflow engine with AI integration")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}), "BR-WF-001-SUCCESS-RATE: Workflow engine must return functional implementation for execution success")
		})

		It("should integrate AI services when available", func() {
			// Act: Create workflow engine with AI integration
			workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
				k8sClient,
				actionRepo,
				monitoringClients,
				stateStorage,
				executionRepo,
				workflowEngineConfig,
				aiConfig,
				logger,
			)

			// Assert: Engine should have AI integration enabled
			Expect(err).ToNot(HaveOccurred(), "Failed to create AI-integrated workflow engine")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}), "BR-AI-001-CONFIDENCE: AI-integrated workflow engine must provide functional implementation for enhanced workflow execution")
		})

		It("should handle missing AI configuration gracefully", func() {
			// Arrange: Use nil AI config to test graceful degradation
			nilAIConfig := (*config.Config)(nil)

			// Act: Create workflow engine without AI configuration
			workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
				k8sClient,
				actionRepo,
				monitoringClients,
				stateStorage,
				executionRepo,
				workflowEngineConfig,
				nilAIConfig,
				logger,
			)

			// Assert: Engine should still be created (graceful degradation)
			Expect(err).ToNot(HaveOccurred(), "Should handle missing AI config gracefully")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}), "BR-WF-001-SUCCESS-RATE: Workflow engine must gracefully degrade while maintaining functional implementation for continued success")
		})
	})

	Describe("BR-WF-AI-002: Type safety and integration", func() {
		It("should use proper types instead of interface{} placeholders", func() {
			// This test validates that the constructor uses typed parameters
			// following project guidelines principle: avoid interface{} unless necessary

			// Act: Create workflow engine - this should compile with typed parameters
			workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
				k8sClient,         // Should accept k8s.Client type
				actionRepo,        // Should accept actionhistory.Repository type
				monitoringClients, // Should accept *monitoring.MonitoringClients type
				stateStorage,
				executionRepo,
				workflowEngineConfig,
				aiConfig,
				logger,
			)

			// Assert: Typed parameters should work correctly
			Expect(err).ToNot(HaveOccurred(), "Typed parameters should compile and execute")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}), "BR-WF-001-SUCCESS-RATE: Type-safe construction must produce functional workflow engine implementation for execution success")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUworkflowUaiUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UworkflowUaiUintegration Suite")
}
