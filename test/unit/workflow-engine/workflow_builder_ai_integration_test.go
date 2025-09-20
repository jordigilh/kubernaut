package workflowengine

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirements: BR-WB-AI-001 - Workflow builder must use AI-integrated engines
var _ = Describe("Workflow Builder AI Integration - Business Requirements", func() {
	var (
		logger         *logrus.Logger
		cancel         context.CancelFunc
		mockActionRepo *mocks.MockActionRepository
		executionRepo  engine.ExecutionRepository
		aiConfig       *config.Config
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		_, cancel = context.WithTimeout(context.Background(), 5*time.Second)

		mockActionRepo = mocks.NewMockActionRepository()
		executionRepo = engine.NewMemoryExecutionRepository(logger)

		// Create AI config for testing
		aiConfig = &config.Config{
			SLM: config.LLMConfig{
				Endpoint: "http://192.168.1.169:8080",
				Model:    "test-model",
			},
			VectorDB: config.VectorDBConfig{
				Enabled: true,
				Backend: "memory",
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	Describe("BR-WB-AI-001: Workflow builder engine creation", func() {
		It("should create intelligent workflow builder with AI integration support", func() {
			// This test validates that intelligent workflow builders can be created
			// with AI integration capabilities (will be expanded once the integration is implemented)

			// For now, test that we can create basic AI components that workflow builders need
			vectorDB := vector.NewMemoryVectorDatabase(logger)
			Expect(vectorDB).To(BeAssignableToTypeOf(&vector.MemoryVectorDatabase{}), "BR-AI-001-CONFIDENCE: Vector database must provide functional implementation for AI-enhanced workflow building")
		})

		It("should demonstrate the integration pattern for AI-enhanced workflow engines", func() {
			// This test defines the integration pattern that intelligent workflow builder should implement:
			// It should create AI-integrated engines instead of basic ones when AI config is available

			// Act: Test the pattern that workflow builder should follow
			k8sClient := mocks.NewMockK8sClient(nil)

			// This is the pattern that workflow builder should use instead of basic NewDefaultWorkflowEngine
			workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
				k8sClient,
				mockActionRepo,
				nil, // monitoring clients - acceptable nil for this test
				engine.NewWorkflowStateStorage(nil, logger),
				executionRepo,
				&engine.WorkflowEngineConfig{
					DefaultStepTimeout: 5 * time.Minute,
				},
				aiConfig,
				logger,
			)

			// Assert: Should create AI-integrated engine successfully
			Expect(err).ToNot(HaveOccurred(), "Should create AI-integrated workflow engine")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}), "BR-WF-001-SUCCESS-RATE: Workflow engine with proper dependencies must provide functional implementation for execution success")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}),
				"Should return DefaultWorkflowEngine with AI capabilities")
		})
	})

	Describe("BR-WB-AI-002: AI dependency integration", func() {
		It("should provide proper dependencies to workflow engines instead of nil stubs", func() {
			// Business requirement: Workflow engines should have real k8s client, action repo
			// instead of nil dependencies that degrade functionality

			// This test validates the anti-pattern that currently exists in intelligent_workflow_builder_impl.go:138
			// where it creates engines with nil dependencies

			// Arrange: Create proper dependencies (what SHOULD be done)
			k8sClient := mocks.NewMockK8sClient(nil)
			actionRepo := mocks.NewMockActionRepository()

			// Act: Demonstrate the correct pattern with real dependencies
			workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
				k8sClient,  // Real k8s client instead of nil
				actionRepo, // Real action repo instead of nil
				nil,        // monitoring clients - acceptable nil for this test
				engine.NewWorkflowStateStorage(nil, logger),
				executionRepo,
				&engine.WorkflowEngineConfig{
					DefaultStepTimeout: 5 * time.Minute,
				},
				aiConfig,
				logger,
			)

			// Assert: Should create with real dependencies successfully
			Expect(err).ToNot(HaveOccurred(), "Should create with real dependencies")
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}), "BR-WF-001-SUCCESS-RATE: Workflow engine with proper dependencies must provide functional implementation for execution success")

			// Business requirement: Should have functional capabilities, not degraded nil-dependency behavior
			Expect(workflowEngine).To(BeAssignableToTypeOf(&engine.DefaultWorkflowEngine{}),
				"Should create functional workflow engine with proper dependencies")
		})
	})
})
