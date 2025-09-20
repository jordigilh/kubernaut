package main

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	orchestration "github.com/jordigilh/kubernaut/pkg/orchestration/adaptive"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirements: BR-ORCH-MAIN-001 - Main application must have adaptive orchestrator
var _ = Describe("Adaptive Orchestrator Integration - Business Requirements", func() {
	var (
		logger   *logrus.Logger
		ctx      context.Context
		cancel   context.CancelFunc
		aiConfig *config.Config
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)

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

	Describe("BR-ORCH-MAIN-001: Adaptive orchestrator creation and integration", func() {
		It("should create adaptive orchestrator with proper dependencies", func() {
			// This test validates the integration pattern that main application should implement

			// Act: Create adaptive orchestrator with proper dependencies (simulating main app)
			orchestrator, err := createAdaptiveOrchestrator(ctx, aiConfig, logger)

			// Assert: Should create successfully with real dependencies
			Expect(err).ToNot(HaveOccurred(), "Should create adaptive orchestrator")
			Expect(orchestrator).ToNot(BeNil(), "Orchestrator should be created")
			Expect(orchestrator).To(BeAssignableToTypeOf(&orchestration.DefaultAdaptiveOrchestrator{}),
				"Should return DefaultAdaptiveOrchestrator implementation")
		})

		It("should start and stop adaptive orchestrator gracefully", func() {
			// Business requirement: Orchestrator should integrate with application lifecycle

			// Arrange: Create orchestrator
			orchestrator, err := createAdaptiveOrchestrator(ctx, aiConfig, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(orchestrator).ToNot(BeNil())

			// Act: Start orchestrator
			err = orchestrator.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "Should start successfully")

			// Act: Stop orchestrator
			err = orchestrator.Stop()
			Expect(err).ToNot(HaveOccurred(), "Should stop gracefully")
		})

		It("should integrate orchestrator with workflow execution capabilities", func() {
			// Business requirement: Orchestrator should be capable of executing workflows

			// Arrange: Create orchestrator
			orchestrator, err := createAdaptiveOrchestrator(ctx, aiConfig, logger)
			Expect(err).ToNot(HaveOccurred())

			// Start orchestrator
			err = orchestrator.Start(ctx)
			Expect(err).ToNot(HaveOccurred())
			defer orchestrator.Stop()

			// Act: Create a simple workflow template for testing using constructor
			template := engine.NewWorkflowTemplate("test-workflow-001", "Test Integration Workflow")
			template.BaseVersionedEntity.Description = "Tests orchestrator integration"

			// Create workflow from template
			workflow, err := orchestrator.CreateWorkflow(ctx, template)

			// Assert: Should create workflow successfully
			Expect(err).ToNot(HaveOccurred(), "Should create workflow")
			Expect(workflow).ToNot(BeNil(), "Workflow should be created")
			Expect(workflow.ID).To(Equal("test-workflow-001"), "Should preserve workflow ID")
		})
	})

	Describe("BR-ORCH-MAIN-002: Production integration patterns", func() {
		It("should handle missing AI configuration gracefully", func() {
			// Act: Create orchestrator with nil config (graceful degradation)
			orchestrator, err := createAdaptiveOrchestrator(ctx, nil, logger)

			// Assert: Should still create orchestrator with fallback behavior
			Expect(err).ToNot(HaveOccurred(), "Should handle missing config gracefully")
			Expect(orchestrator).ToNot(BeNil(), "Should create fallback orchestrator")
		})
	})
})

// Note: createAdaptiveOrchestrator is now implemented in main.go
// This test file now uses the production implementation
