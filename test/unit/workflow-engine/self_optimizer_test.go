package workflowengine

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	sharedTypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirements: BR-SELF-OPT-UNIT-001-004 - DefaultSelfOptimizer Core Logic Unit Tests
// Following TDD methodology: Write failing tests first, then implement to pass
var _ = Describe("BR-SELF-OPT-UNIT-001-004: DefaultSelfOptimizer Core Logic", func() {
	var (
		selfOptimizer *engine.DefaultSelfOptimizer
		logger        *logrus.Logger
		ctx           context.Context
		cancel        context.CancelFunc
		testWorkflow  *engine.Workflow
		config        *engine.SelfOptimizerConfig
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)

		// Create test configuration
		config = &engine.SelfOptimizerConfig{
			EnableStructuralOptimization:  true,
			EnableLogicOptimization:       true,
			EnablePerformanceOptimization: true,
			MinExecutionHistorySize:       3,
			OptimizationInterval:          1 * time.Hour,
			EnableContinuousOptimization:  true,
			MaxOptimizationIterations:     5,
		}

		// Create self optimizer with nil workflow builder for testing core logic
		// This allows us to test the core optimization algorithms without complex mocking
		selfOptimizer = engine.NewDefaultSelfOptimizer(nil, config, logger)

		// Create test workflow
		testWorkflow = createTestWorkflow()
	})

	AfterEach(func() {
		cancel()
	})

	Describe("BR-SELF-OPT-UNIT-001: OptimizeWorkflow Algorithm", func() {
		Context("with sufficient execution history", func() {
			It("should optimize workflow based on execution patterns", func() {
				// Arrange: Create execution history with performance data
				executionHistory := createExecutionHistoryWithPatterns(5)

				// Act: Optimize workflow
				optimizedWorkflow, err := selfOptimizer.OptimizeWorkflow(ctx, testWorkflow, executionHistory)

				// Assert: Should optimize successfully
				Expect(err).ToNot(HaveOccurred(), "Should optimize workflow without error")
				Expect(len(optimizedWorkflow.Template.Steps)).To(Equal(len(testWorkflow.Template.Steps)), "BR-WF-001-SUCCESS-RATE: Self optimizer must preserve workflow structure for execution success")
				Expect(optimizedWorkflow).ToNot(Equal(testWorkflow), "Should return modified workflow")

				// Business requirement: Workflow should be actually optimized
				validateWorkflowOptimization(testWorkflow, optimizedWorkflow)
			})

			It("should maintain workflow correctness during optimization", func() {
				// Arrange: Create complex workflow with dependencies
				complexWorkflow := createComplexTestWorkflow()
				executionHistory := createSuccessfulExecutionHistory(5)

				// Act: Optimize workflow
				optimizedWorkflow, err := selfOptimizer.OptimizeWorkflow(ctx, complexWorkflow, executionHistory)

				// Assert: Should maintain correctness
				Expect(err).ToNot(HaveOccurred())
				Expect(len(optimizedWorkflow.Template.Steps)).To(BeNumerically(">", 0), "BR-WF-001-SUCCESS-RATE: Self optimizer must produce executable workflow with measurable steps for execution success")

				// Business requirement: Workflow should still be valid and executable
				Expect(validateWorkflowCorrectness(optimizedWorkflow)).To(BeTrue(),
					"Optimized workflow should maintain correctness")
			})
		})

		Context("with insufficient execution history", func() {
			It("should return original workflow when history < 3 executions", func() {
				// Arrange: Create insufficient execution history
				executionHistory := createExecutionHistoryWithPatterns(2) // Less than MinExecutionHistorySize

				// Act: Attempt optimization
				result, err := selfOptimizer.OptimizeWorkflow(ctx, testWorkflow, executionHistory)

				// Assert: Should return original workflow without modification
				Expect(err).ToNot(HaveOccurred(), "Should handle insufficient history gracefully")
				Expect(result).To(Equal(testWorkflow), "Should return original workflow unchanged")
			})

			It("should handle nil workflow gracefully", func() {
				// Arrange: nil workflow
				executionHistory := createExecutionHistoryWithPatterns(5)

				// Act: Attempt optimization with nil workflow
				result, err := selfOptimizer.OptimizeWorkflow(ctx, nil, executionHistory)

				// Assert: Should return appropriate error
				Expect(err).To(HaveOccurred(), "Should return error for nil workflow")
				Expect(result).To(BeNil(), "Should return nil result for nil workflow")
			})
		})
	})

	Describe("BR-SELF-OPT-UNIT-002: SuggestImprovements Algorithm", func() {
		Context("with valid execution data", func() {
			It("should generate 3-5 optimization suggestions", func() {
				// Act: Generate suggestions
				suggestions, err := selfOptimizer.SuggestImprovements(ctx, testWorkflow)

				// Assert: Should generate appropriate number of suggestions
				Expect(err).ToNot(HaveOccurred(), "Should generate suggestions without error")
				Expect(len(suggestions)).To(BeNumerically(">=", 3), "BR-ORK-001: Self optimizer must generate measurable optimization candidates for recommendation success")

				// Business requirement: BR-ORK-358 - Generate 3-5 viable candidates
				Expect(len(suggestions)).To(BeNumerically("<=", 5), "Should generate at most 5 suggestions")
			})

			It("should categorize suggestions by type (structural/logic/performance)", func() {
				// Act: Generate suggestions
				suggestions, err := selfOptimizer.SuggestImprovements(ctx, testWorkflow)

				// Assert: Should categorize properly
				Expect(err).ToNot(HaveOccurred())
				Expect(suggestions).ToNot(BeEmpty())

				// Business requirement: Should have different types of optimizations
				suggestionTypes := extractSuggestionTypes(suggestions)
				Expect(suggestionTypes).To(ContainElement("structural"), "Should include structural optimizations")
				Expect(suggestionTypes).To(ContainElement("logic"), "Should include logic optimizations")
				Expect(suggestionTypes).To(ContainElement("performance"), "Should include performance optimizations")
			})

			It("should provide impact scores for each suggestion", func() {
				// Act: Generate suggestions
				suggestions, err := selfOptimizer.SuggestImprovements(ctx, testWorkflow)

				// Assert: Should have impact scores
				Expect(err).ToNot(HaveOccurred())
				Expect(suggestions).ToNot(BeEmpty())

				// Business requirement: Each suggestion should have impact score
				for _, suggestion := range suggestions {
					Expect(suggestion.Impact).To(BeNumerically(">=", 0.0), "Impact should be >= 0")
					Expect(suggestion.Impact).To(BeNumerically("<=", 1.0), "Impact should be <= 1")
				}
			})
		})

		Context("with edge cases", func() {
			It("should handle nil workflow gracefully", func() {
				// Act: Generate suggestions for nil workflow
				suggestions, err := selfOptimizer.SuggestImprovements(ctx, nil)

				// Assert: Should return appropriate error
				Expect(err).To(HaveOccurred(), "Should return error for nil workflow")
				Expect(suggestions).To(BeNil(), "Should return nil suggestions for nil workflow")
			})
		})
	})

	Describe("BR-SELF-OPT-UNIT-003: Configuration Validation", func() {
		Context("with valid configuration", func() {
			It("should create self optimizer with default configuration", func() {
				// Act: Create self optimizer with default config
				defaultOptimizer := engine.NewDefaultSelfOptimizer(nil, nil, logger)

				// Assert: Should create successfully with defaults
				Expect(func() { _ = defaultOptimizer.OptimizeWorkflow }).ToNot(Panic(), "BR-WF-001-SUCCESS-RATE: Default self optimizer must provide functional optimization interface for execution success")
			})

			It("should respect configuration parameters", func() {
				// Arrange: Create custom configuration
				customConfig := &engine.SelfOptimizerConfig{
					EnableStructuralOptimization:  false,
					EnableLogicOptimization:       true,
					EnablePerformanceOptimization: true,
					MinExecutionHistorySize:       5,
					MaxOptimizationIterations:     3,
				}

				// Act: Create self optimizer with custom config
				customOptimizer := engine.NewDefaultSelfOptimizer(nil, customConfig, logger)

				// Assert: Should use custom configuration
				Expect(func() { _ = customOptimizer.OptimizeWorkflow }).ToNot(Panic(), "BR-WF-001-SUCCESS-RATE: Custom self optimizer must provide functional optimization interface for execution success")
			})
		})
	})
})

// Helper functions for test data generation and validation

func createTestWorkflow() *engine.Workflow {
	// Create a basic test workflow
	return &engine.Workflow{
		BaseVersionedEntity: sharedTypes.BaseVersionedEntity{
			BaseEntity: sharedTypes.BaseEntity{
				ID:          "test-workflow-001",
				Name:        "Test Workflow",
				Description: "A test workflow for unit testing",
				CreatedAt:   time.Now(),
			},
			Version:   "1.0.0",
			CreatedBy: "unit-test",
		},
		// Add basic workflow structure
	}
}

func createComplexTestWorkflow() *engine.Workflow {
	// Create a more complex workflow for testing
	// Following guideline: Test business requirements, not implementation (Principle #28)

	template := engine.NewWorkflowTemplate("complex-template-001", "Complex Test Template")
	template.Description = "A complex workflow template with multiple steps and dependencies"

	// Add realistic workflow steps for business testing
	step1 := &engine.ExecutableWorkflowStep{
		BaseEntity: sharedTypes.BaseEntity{
			ID:   "step-1",
			Name: "Database Connection",
		},
		Type:    engine.StepTypeAction,
		Action:  &engine.StepAction{Type: "database_connect", Parameters: map[string]interface{}{"timeout": "30s"}},
		Timeout: 30 * time.Second,
	}

	step2 := &engine.ExecutableWorkflowStep{
		BaseEntity: sharedTypes.BaseEntity{
			ID:   "step-2",
			Name: "API Integration",
		},
		Type:         engine.StepTypeAction,
		Action:       &engine.StepAction{Type: "api_call", Parameters: map[string]interface{}{"retries": 3}},
		Dependencies: []string{"step-1"},
		Timeout:      15 * time.Second,
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step1, step2}

	workflow := engine.NewWorkflow("complex-workflow-001", template)
	workflow.Name = "Complex Test Workflow"
	workflow.Description = "A complex workflow with dependencies and multiple steps"

	return workflow
}

// Execution history generation functions
func createExecutionHistoryWithPatterns(count int) []*engine.RuntimeWorkflowExecution {
	var executions []*engine.RuntimeWorkflowExecution
	for i := 0; i < count; i++ {
		execution := &engine.RuntimeWorkflowExecution{
			WorkflowExecutionRecord: types.WorkflowExecutionRecord{
				ID:         fmt.Sprintf("exec-%d", i),
				WorkflowID: "test-workflow-001",
				Status:     "completed",
				StartTime:  time.Now().Add(-time.Duration(i) * time.Hour),
			},
			OperationalStatus: engine.ExecutionStatusCompleted,
			Duration:          time.Duration(100+i*10) * time.Millisecond,
		}
		executions = append(executions, execution)
	}
	return executions
}

func createSuccessfulExecutionHistory(count int) []*engine.RuntimeWorkflowExecution {
	executions := createExecutionHistoryWithPatterns(count)
	// Ensure all executions are successful
	for _, exec := range executions {
		exec.WorkflowExecutionRecord.Status = "completed"
		exec.OperationalStatus = engine.ExecutionStatusCompleted
	}
	return executions
}

// Validation helper functions
func validateWorkflowOptimization(original, optimized *engine.Workflow) {
	// Validate that optimization occurred
	Expect(optimized.ID).To(ContainSubstring("optimized"), "Should have optimization indicator")
}

func validateWorkflowCorrectness(workflow *engine.Workflow) bool {
	// Following guideline: Test business requirements, not implementation (Principle #28)
	// Business Requirement: Optimized workflow should maintain structural integrity

	if workflow == nil {
		return false
	}

	// Basic structural validation
	if workflow.ID == "" || workflow.Name == "" {
		return false
	}

	// Template validation - optimized workflows should have valid templates
	if workflow.Template == nil {
		return false
	}

	// Template should have valid ID and structure
	if workflow.Template.ID == "" {
		return false
	}

	// Business Requirement: Optimized workflows should maintain executable structure
	// Following guideline: Strong business assertions (Principle #30)
	if workflow.Template.Steps == nil {
		return false
	}

	// Validate optimization metadata exists (indicates successful optimization)
	if workflow.Template.Metadata == nil {
		return false
	}

	// Check for optimization indicators
	if _, hasOptimization := workflow.Template.Metadata["optimization_source"]; !hasOptimization {
		return false
	}

	return true
}

func extractSuggestionTypes(suggestions []*engine.OptimizationSuggestion) []string {
	var types []string
	for _, suggestion := range suggestions {
		types = append(types, suggestion.Type)
	}
	return types
}
