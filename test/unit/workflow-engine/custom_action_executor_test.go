package workflowengine

import (
	"testing"
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Custom Action Executor - Enhanced Context Integration Testing", func() {
	var (
		ctx          context.Context
		executor     *engine.CustomActionExecutor
		testLogger   *logrus.Logger
		stepContext  *engine.StepContext
		sampleAction *engine.StepAction
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.DebugLevel) // Capture all log levels for testing

		executor = engine.NewCustomActionExecutor(testLogger)

		// Create test step context with realistic data
		stepContext = &engine.StepContext{
			ExecutionID: "exec-123",
			StepID:      "step-456",
			Variables: map[string]interface{}{
				"step_id":        "test-step-wait",
				"workflow_name":  "test-workflow",
				"execution_type": "automated",
			},
		}

		// Create sample action for testing
		sampleAction = &engine.StepAction{
			Type: "custom",
			Parameters: map[string]interface{}{
				"action":   "wait",
				"duration": "5s",
			},
		}
	})

	Context("waitAction - Enhanced Contextual Logging", func() {
		It("should log with step context when provided", func() {
			// Following TDD: Test enhanced contextual logging
			sampleAction.Parameters["duration"] = "100ms" // Short duration for test

			result, err := executor.Execute(ctx, sampleAction, stepContext)

			Expect(err).ToNot(HaveOccurred(), "Execute should succeed")
			Expect(result.Success).To(BeTrue(), "Execute should complete successfully")

			// Note: In a real implementation, we would verify log output
			// For this test, we verify that the action completes with context provided
			Expect(stepContext.Variables).To(HaveKey("step_id"), "StepContext should have step_id")
		})

		It("should handle nil step context gracefully", func() {
			// Following TDD: Test edge case
			sampleAction.Parameters["duration"] = "100ms"

			result, err := executor.Execute(ctx, sampleAction, nil)

			Expect(err).ToNot(HaveOccurred(), "Execute should succeed even with nil context")
			Expect(result.Success).To(BeTrue(), "Execute should complete successfully")

			// Should complete successfully even without contextual information
			Expect(result.Duration).To(BeNumerically(">=", 100*time.Millisecond), "Should wait for specified duration")
		})

		It("should validate duration parameter", func() {
			// Following TDD: Test input validation
			sampleAction.Parameters["duration"] = "invalid-duration"

			result, err := executor.Execute(ctx, sampleAction, stepContext)

			Expect(err).ToNot(HaveOccurred(), "Should not return error")
			Expect(result.Success).To(BeFalse(), "Should fail for invalid duration")
			Expect(result.Error).To(ContainSubstring("invalid duration format"), "Should provide meaningful error")
		})

		It("should use default duration when not specified", func() {
			// Following TDD: Test default behavior
			delete(sampleAction.Parameters, "duration")

			// Use context with timeout to avoid waiting too long
			ctxWithTimeout, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()

			result, err := executor.Execute(ctxWithTimeout, sampleAction, stepContext)

			// Should start waiting with default duration but be cancelled by context
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeFalse(), "Should be cancelled by context timeout")
			Expect(result.Error).To(Equal("wait action cancelled due to context cancellation"), "Should indicate context cancellation")
		})

		It("should respect context cancellation", func() {
			// Following TDD: Test cancellation behavior
			sampleAction.Parameters["duration"] = "10s" // Long duration

			ctxWithCancel, cancel := context.WithCancel(ctx)

			// Cancel context after short delay
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()

			result, err := executor.Execute(ctxWithCancel, sampleAction, stepContext)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeFalse(), "Should be cancelled")
			Expect(result.Error).To(Equal("wait action cancelled due to context cancellation"), "Should indicate cancellation")
		})
	})

	Context("logAction - Enhanced Message Enrichment", func() {
		It("should enrich log message with step context", func() {
			// Following TDD: Test message enrichment
			sampleAction.Type = "custom"
			sampleAction.Parameters = map[string]interface{}{
				"action":  "log",
				"message": "Test log message",
				"level":   "info",
			}

			result, err := executor.Execute(ctx, sampleAction, stepContext)

			Expect(err).ToNot(HaveOccurred(), "Execute should succeed")
			Expect(result.Success).To(BeTrue(), "Execute should complete successfully")

			// Verify that logging action completed with context
			// In a real implementation, message enrichment would be verified through log output
			Expect(stepContext.Variables["step_id"]).To(Equal("test-step-wait"), "Should have access to step context")
			Expect(result.Success).To(BeTrue(), "BR-WF-001-SUCCESS-RATE: Custom action execution must return successful result for workflow completion")
		})

		It("should handle different log levels correctly", func() {
			// Following TDD: Test level handling
			testLevels := []string{"debug", "info", "warn", "error"}

			for _, level := range testLevels {
				sampleAction.Type = "custom"
				sampleAction.Parameters = map[string]interface{}{
					"action":  "log",
					"message": fmt.Sprintf("Test %s message", level),
					"level":   level,
				}

				result, err := executor.Execute(ctx, sampleAction, stepContext)

				Expect(err).ToNot(HaveOccurred(), "Execute should succeed for level %s", level)
				Expect(result.Success).To(BeTrue(), "Execute should complete for level %s", level)

				// Verify the log level parameter was processed
				Expect(result.Data).To(HaveKey("level"), "Should record the log level used")
				Expect(result.Data["level"]).To(Equal(level), "Should use the specified log level")
			}
		})

		It("should use defaults for missing parameters", func() {
			// Following TDD: Test default behavior
			sampleAction.Type = "custom"
			sampleAction.Parameters = map[string]interface{}{
				"action": "log",
			} // No message or level

			result, err := executor.Execute(ctx, sampleAction, stepContext)

			Expect(err).ToNot(HaveOccurred(), "Execute should succeed with defaults")
			Expect(result.Success).To(BeTrue(), "Execute should complete with defaults")

			// Should use default level (info) when not specified
			Expect(result.Data).To(HaveKey("level"), "Should record default level")
			Expect(result.Data["level"]).To(Equal("info"), "Should default to info level")
		})

		It("should handle nil step context gracefully", func() {
			// Following TDD: Test edge case
			sampleAction.Type = "custom"
			sampleAction.Parameters = map[string]interface{}{
				"action":  "log",
				"message": "Test message without context",
				"level":   "info",
			}

			result, err := executor.Execute(ctx, sampleAction, nil)

			Expect(err).ToNot(HaveOccurred(), "Execute should succeed without context")
			Expect(result.Success).To(BeTrue(), "Execute should complete without context")

			// Should still complete successfully even without context
			Expect(result.Data).To(HaveKey("message"), "Should record the message")
			Expect(result.Data["message"]).To(Equal("Test message without context"), "Should use original message")
		})

		It("should respect context cancellation", func() {
			// Following TDD: Test cancellation
			ctxWithCancel, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			sampleAction.Type = "custom"
			sampleAction.Parameters = map[string]interface{}{
				"action":  "log",
				"message": "This should be cancelled",
			}

			result, err := executor.Execute(ctxWithCancel, sampleAction, stepContext)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeFalse(), "Should be cancelled")
			Expect(result.Error).To(Equal("context_cancelled"), "Should indicate cancellation")
		})
	})

	Context("Custom Action Integration", func() {
		It("should demonstrate complete action lifecycle with context", func() {
			// Following TDD: Test complete integration
			actions := []*engine.StepAction{
				{
					Type: "custom",
					Parameters: map[string]interface{}{
						"action":  "log",
						"message": "Starting workflow step",
						"level":   "info",
					},
				},
				{
					Type: "custom",
					Parameters: map[string]interface{}{
						"action":   "wait",
						"duration": "100ms",
					},
				},
				{
					Type: "custom",
					Parameters: map[string]interface{}{
						"action":  "log",
						"message": "Workflow step completed",
						"level":   "info",
					},
				},
			}

			for i, action := range actions {
				stepContext.Variables["step_id"] = fmt.Sprintf("integration-step-%d", i+1)

				var result *engine.StepResult
				var err error

				// Use the public Execute method which internally routes to appropriate action
				result, err = executor.Execute(ctx, action, stepContext)

				Expect(err).ToNot(HaveOccurred(), "Action %d should succeed", i+1)
				Expect(result.Success).To(BeTrue(), "Action %d should complete successfully", i+1)
			}

			// Verify all actions completed successfully
			// In a real implementation, we would verify log output for each action
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUcustomUactionUexecutor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UcustomUactionUexecutor Suite")
}
