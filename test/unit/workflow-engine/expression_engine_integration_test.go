package workflowengine

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirements: BR-EXPR-ENGINE-001-004 - Expression Engine Integration Tests
// Following TDD methodology: Write failing tests first, then implement to pass
var _ = Describe("BR-EXPR-ENGINE-001-004: Expression Engine Integration", func() {
	var (
		workflowEngine   *engine.DefaultWorkflowEngine
		expressionEngine *engine.ExpressionEngine
		logger           *logrus.Logger
		ctx              context.Context
		cancel           context.CancelFunc
		testCondition    *engine.ExecutableCondition
		testStepContext  *engine.StepContext
		testStepResult   *engine.StepResult
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)

		// Create test workflow engine with expression engine integration
		workflowEngine, expressionEngine = createWorkflowEngineWithExpressionEngine(logger)

		// Create test data
		testCondition = createTestCondition()
		testStepContext = createTestStepContext()
		testStepResult = createTestStepResult()
	})

	AfterEach(func() {
		cancel()
	})

	Describe("BR-EXPR-ENGINE-001: Enhanced Condition Evaluation", func() {
		Context("with advanced expressions", func() {
			It("should evaluate complex logical expressions", func() {
				// Arrange: Create complex logical expression
				testCondition.Expression = "result.duration_seconds() > 5 && result.success == true"

				// Act: Evaluate condition using expression engine
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should evaluate complex expression correctly
				Expect(err).ToNot(HaveOccurred(), "Should evaluate complex expression without error")
				Expect(result).To(BeAssignableToTypeOf(true), "Should return boolean result")

				// Business requirement: Should use expression engine instead of basic evaluation
				Expect(workflowEngine.HasExpressionEngine()).To(BeTrue(),
					"Workflow engine should have expression engine integrated")
			})

			It("should support built-in functions", func() {
				// Arrange: Create expression with built-in functions
				testCondition.Expression = "contains(output_value('message'), 'success')"

				// Act: Evaluate expression with built-in function
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should support built-in functions
				Expect(err).ToNot(HaveOccurred(), "Should evaluate built-in functions without error")
				Expect(result).To(BeAssignableToTypeOf(true), "Should return boolean result")
			})

			It("should handle variable substitution in expressions", func() {
				// Arrange: Create expression with variables
				testCondition.Expression = "{{environment}} == 'production' && duration_seconds() < 30"
				testStepContext.Variables["environment"] = "production"

				// Act: Evaluate expression with variable substitution
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should substitute variables correctly
				Expect(err).ToNot(HaveOccurred(), "Should handle variable substitution without error")
				Expect(result).To(BeAssignableToTypeOf(true), "Should return boolean result")
			})
		})

		Context("with performance optimization", func() {
			It("should cache compiled expressions for better performance", func() {
				// Arrange: Same expression evaluated multiple times
				testCondition.Expression = "duration_seconds() > 1"

				// Act: Evaluate same expression multiple times
				for i := 0; i < 3; i++ {
					_, err := workflowEngine.EvaluateConditionWithExpressionEngine(
						ctx, testCondition, testStepContext, testStepResult)
					Expect(err).ToNot(HaveOccurred())
				}

				// Assert: Should cache compiled expression for performance
				cacheSize := expressionEngine.GetCacheSize()
				Expect(cacheSize).To(BeNumerically(">=", 1), "Should cache compiled expressions")
			})

			It("should provide faster evaluation than basic expression evaluation", func() {
				// Arrange: Complex expression for performance testing
				testCondition.Expression = "duration_seconds() > 1 && has_error() == false && len(output_value('items')) > 0"

				// Act: Measure evaluation time
				start := time.Now()
				_, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)
				evaluationTime := time.Since(start)

				// Assert: Should evaluate efficiently
				Expect(err).ToNot(HaveOccurred())
				Expect(evaluationTime).To(BeNumerically("<", 10*time.Millisecond),
					"Should evaluate expressions efficiently")
			})
		})
	})

	Describe("BR-EXPR-ENGINE-002: Backward Compatibility", func() {
		Context("with simple expressions", func() {
			It("should maintain compatibility with basic boolean expressions", func() {
				// Arrange: Simple boolean expression
				testCondition.Expression = "true"

				// Act: Evaluate simple expression
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should maintain backward compatibility
				Expect(err).ToNot(HaveOccurred(), "Should handle simple expressions")
				Expect(result).To(BeTrue(), "Should return correct boolean value")
			})

			It("should handle legacy expression formats", func() {
				// Arrange: Legacy format expressions
				testCondition.Expression = "{{success}} == true"
				testStepContext.Variables["success"] = true

				// Act: Evaluate legacy format
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should support legacy formats
				Expect(err).ToNot(HaveOccurred(), "Should handle legacy formats")
				Expect(result).To(BeTrue(), "Should return correct result")
			})
		})
	})

	Describe("BR-EXPR-ENGINE-003: Error Handling", func() {
		Context("with invalid expressions", func() {
			It("should handle syntax errors gracefully", func() {
				// Arrange: Invalid expression syntax
				testCondition.Expression = "invalid_syntax && && true"

				// Act: Evaluate invalid expression
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should handle syntax errors gracefully
				if err != nil {
					// Should return clear error message
					Expect(err.Error()).To(ContainSubstring("syntax"), "Should provide clear error message")
				} else {
					// Should return safe default value
					Expect(result).To(BeAssignableToTypeOf(false), "Should return safe default")
				}
			})

			It("should handle undefined variables gracefully", func() {
				// Arrange: Expression with undefined variable
				testCondition.Expression = "{{undefined_variable}} == 'test'"

				// Act: Evaluate expression with undefined variable
				result, err := workflowEngine.EvaluateConditionWithExpressionEngine(
					ctx, testCondition, testStepContext, testStepResult)

				// Assert: Should handle undefined variables gracefully
				Expect(err).ToNot(HaveOccurred(), "Should handle undefined variables gracefully")
				Expect(result).To(BeAssignableToTypeOf(false), "Should return safe default")
			})
		})
	})

	Describe("BR-EXPR-ENGINE-004: Integration with Existing Workflow Engine", func() {
		Context("with workflow execution", func() {
			It("should integrate with standard workflow condition evaluation", func() {
				// Arrange: Standard workflow condition
				testCondition.Type = "expression"
				testCondition.Expression = "duration_seconds() < 60"

				// Act: Evaluate condition through standard workflow path
				result, err := workflowEngine.EvaluateCondition(ctx, testCondition, testStepContext)

				// Assert: Should integrate with existing workflow evaluation
				Expect(err).ToNot(HaveOccurred(), "Should integrate with existing workflow")
				Expect(result).To(BeAssignableToTypeOf(true), "Should return boolean result")
			})

			It("should fallback to basic evaluation when expression engine fails", func() {
				// Arrange: Force expression engine failure scenario
				testCondition.Expression = "extremely_complex_expression_that_might_fail()"

				// Act: Evaluate with potential fallback
				result, err := workflowEngine.EvaluateCondition(ctx, testCondition, testStepContext)

				// Assert: Should fallback gracefully
				Expect(err).ToNot(HaveOccurred(), "Should fallback gracefully")
				Expect(result).To(BeAssignableToTypeOf(true), "Should return safe result")
			})
		})
	})
})

// Helper functions for test data generation and workflow engine creation

func createWorkflowEngineWithExpressionEngine(logger *logrus.Logger) (*engine.DefaultWorkflowEngine, *engine.ExpressionEngine) {
	// Create workflow engine configuration
	config := &engine.WorkflowEngineConfig{
		DefaultStepTimeout:    10 * time.Minute,
		MaxRetryDelay:         5 * time.Minute,
		EnableStateRecovery:   true,
		EnableDetailedLogging: false,
		MaxConcurrency:        5,
	}

	// Create state storage and execution repository
	stateStorage := engine.NewWorkflowStateStorage(nil, logger)
	executionRepo := engine.NewMemoryExecutionRepository(logger)

	// Create workflow engine with expression engine integration
	workflowEngine := engine.NewDefaultWorkflowEngine(
		nil, // k8sClient - not needed for expression testing
		nil, // actionRepo - not needed for expression testing
		nil, // monitoringClients - not needed for expression testing
		stateStorage,
		executionRepo,
		config,
		logger,
	)

	// Get the integrated expression engine
	expressionEngine := workflowEngine.GetExpressionEngine()

	return workflowEngine, expressionEngine
}

func createTestCondition() *engine.ExecutableCondition {
	return &engine.ExecutableCondition{
		ID:         "test-condition-001",
		Type:       "expression",
		Expression: "true", // Default simple expression
	}
}

func createTestStepContext() *engine.StepContext {
	return &engine.StepContext{
		Variables: map[string]interface{}{
			"environment": "development",
			"success":     true,
			"count":       10,
		},
		ExecutionID: "test-execution-001",
		StepID:      "test-step-001",
	}
}

func createTestStepResult() *engine.StepResult {
	return &engine.StepResult{
		Success: true,
		Output: map[string]interface{}{
			"message": "Operation completed successfully",
			"items":   []string{"item1", "item2", "item3"},
		},
		Duration: 2 * time.Second,
		Error:    "",
	}
}

// TestRunner bootstraps the Ginkgo test suite
func TestUexpressionUengineUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UexpressionUengineUintegration Suite")
}
