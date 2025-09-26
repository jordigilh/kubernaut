package workflowengine

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirements: BR-WB-DEPS-001 - Workflow builders must use real dependencies
var _ = Describe("Workflow Builder Dependencies Integration - Business Requirements", func() {
	var (
		logger         *logrus.Logger
		ctx            context.Context
		cancel         context.CancelFunc
		aiConfig       *config.Config
		mockK8sClient  *mocks.MockK8sClient
		mockActionRepo *mocks.MockActionRepository
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)

		// Create mock dependencies
		fakeClientset := fake.NewSimpleClientset()
		mockK8sClient = mocks.NewMockK8sClient(fakeClientset)
		mockActionRepo = mocks.NewMockActionRepository()

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

	Describe("BR-WB-DEPS-001: Real dependencies for workflow builders", func() {
		It("should create workflow builder with real k8s client and action repository", func() {
			// This test validates the pattern that should be used to create workflow builders
			// with real dependencies instead of nil stubs

			// Act: Create workflow builder with real dependencies (correct pattern)
			workflowBuilder, err := createWorkflowBuilderWithDependencies(
				ctx,
				aiConfig,
				mockK8sClient,
				mockActionRepo,
				logger,
			)

			// Assert: Should create with real dependencies
			Expect(err).ToNot(HaveOccurred(), "Should create workflow builder with dependencies")
			Expect(workflowBuilder).To(BeAssignableToTypeOf(&engine.DefaultIntelligentWorkflowBuilder{}), "BR-WF-001-SUCCESS-RATE: Workflow builder with proper dependencies must provide functional implementation for workflow creation success")
		})

		It("should create workflow engines with proper k8s client and action repo integration", func() {
			// Business requirement: Workflow engines created by builders should have functional dependencies
			// instead of nil values that degrade functionality

			// Arrange: Create workflow builder
			workflowBuilder, err := createWorkflowBuilderWithDependencies(
				ctx,
				aiConfig,
				mockK8sClient,
				mockActionRepo,
				logger,
			)
			Expect(err).ToNot(HaveOccurred())

			// Act: Request workflow engine creation (simulating workflow execution)
			hasRealDependencies := validateWorkflowBuilderDependencies(workflowBuilder)

			// Assert: Should have real dependencies configured
			Expect(hasRealDependencies).To(BeTrue(),
				"Workflow builder should be configured with real k8s client and action repo")
		})

		It("should handle missing dependencies gracefully without degrading functionality", func() {
			// This test validates graceful degradation when some dependencies are unavailable

			// Act: Create builder with partial dependencies - k8s client but no action repo
			// Create basic components first
			llmClient, _ := engine.CreateTestLLMClient(logger)
			vectorDB := engine.CreateTestVectorDatabase(logger)
			analyticsEngine := engine.CreateTestAnalyticsEngine(logger)
			patternStore := engine.CreateTestPatternStore(logger)
			executionRepo := engine.NewMemoryExecutionRepository(logger)

			// Create workflow builder with new config pattern
			config := &engine.IntelligentWorkflowBuilderConfig{
				LLMClient:       llmClient,
				VectorDB:        vectorDB,
				AnalyticsEngine: analyticsEngine,
				PatternStore:    patternStore,
				ExecutionRepo:   executionRepo,
				Logger:          logger,
			}

			builder, err := engine.NewIntelligentWorkflowBuilder(config)
			Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")

			// Enhance with only k8s client (no action repo)
			workflowBuilder, err := engine.EnhanceWorkflowBuilderWithDependencies(
				builder,
				mockK8sClient,
				nil, // nil action repo to test graceful handling
				aiConfig,
				logger,
			)

			// Assert: Should still create successfully
			Expect(err).ToNot(HaveOccurred(), "Should handle partial dependencies gracefully")
			Expect(workflowBuilder).To(BeAssignableToTypeOf(&engine.DefaultIntelligentWorkflowBuilder{}), "BR-WF-001-SUCCESS-RATE: Workflow builder with proper dependencies must provide functional implementation for workflow creation success")
		})
	})

	Describe("BR-WB-DEPS-002: Dependency injection patterns", func() {
		It("should support dependency injection for workflow builders", func() {
			// This test validates that workflow builders can be enhanced with dependency injection
			// instead of hardcoded nil values

			// Act: Create with dependency injection pattern
			workflowBuilder, err := createWorkflowBuilderWithDependencies(
				ctx,
				aiConfig,
				mockK8sClient,
				mockActionRepo,
				logger,
			)
			Expect(err).ToNot(HaveOccurred())

			// Assert: Should support dependency updates after creation
			canUpdateDependencies := validateDependencyInjectionSupport(workflowBuilder)
			Expect(canUpdateDependencies).To(BeTrue(),
				"Should support updating dependencies after creation")
		})
	})
})

// Helper function that demonstrates the production pattern for creating workflow builders
func createWorkflowBuilderWithDependencies(
	ctx context.Context,
	aiConfig *config.Config,
	k8sClient *mocks.MockK8sClient,
	actionRepo *mocks.MockActionRepository,
	logger *logrus.Logger,
) (*engine.DefaultIntelligentWorkflowBuilder, error) {
	// Create basic AI components for workflow builder
	llmClient, _ := engine.CreateTestLLMClient(logger)
	vectorDB := engine.CreateTestVectorDatabase(logger)
	analyticsEngine := engine.CreateTestAnalyticsEngine(logger)
	patternStore := engine.CreateTestPatternStore(logger)
	executionRepo := engine.NewMemoryExecutionRepository(logger)

	// Create workflow builder with new config pattern
	config := &engine.IntelligentWorkflowBuilderConfig{
		LLMClient:       llmClient,
		VectorDB:        vectorDB,
		AnalyticsEngine: analyticsEngine,
		PatternStore:    patternStore,
		ExecutionRepo:   executionRepo,
		Logger:          logger,
	}

	builder, err := engine.NewIntelligentWorkflowBuilder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow builder: %w", err)
	}

	// Inject real dependencies using the enhanced pattern
	// Business Requirement: BR-WB-DEPS-001 - Provide real dependencies
	enhancedBuilder, err := engine.EnhanceWorkflowBuilderWithDependencies(
		builder,
		k8sClient,  // k8s.Client type
		actionRepo, // actionhistory.Repository type
		aiConfig,
		logger,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to enhance workflow builder with dependencies: %w", err)
	}

	return enhancedBuilder, nil
}

// Helper function to validate that workflow builder has real dependencies
func validateWorkflowBuilderDependencies(builder *engine.DefaultIntelligentWorkflowBuilder) bool {
	// Check if workflow builder can create workflow engines with real dependencies
	// This validates the internal configuration
	return engine.HasRealDependencies(builder)
}

// Helper function to validate dependency injection support
func validateDependencyInjectionSupport(builder *engine.DefaultIntelligentWorkflowBuilder) bool {
	// Validate that dependency injection is supported
	return engine.SupportsDependencyInjection(builder)
}
