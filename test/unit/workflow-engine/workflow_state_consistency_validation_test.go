package workflowengine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestWorkflowStateConsistencyValidation function removed to avoid Ginkgo suite conflict
// Tests are now handled by the main TestWorkflowEngine suite function

var _ = Describe("Workflow State Consistency Validation", func() {

	// Business Requirement: BR-REL-011 - Maintain workflow state consistency across all operations
	// Business Requirement: BR-DATA-014 - Provide state validation and consistency checks
	Context("BR-REL-011 & BR-DATA-014: Workflow State Consistency Validation", func() {
		var (
			validator *engine.WorkflowStateValidator
			execution *engine.RuntimeWorkflowExecution
			logger    *logrus.Logger
			ctx       context.Context
			cancel    context.CancelFunc
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

			ctx, cancel = context.WithCancel(context.Background())

			// Create validator with test configuration
			config := &engine.StateValidationConfig{
				EnableRealTimeValidation:  true,
				EnableDeepStateValidation: true,
				MaxValidationWorkers:      5,
				ValidationTimeoutPerStep:  time.Second * 10,
				StrictConsistencyMode:     true,
				EnablePerformanceMetrics:  true,
				CheckpointValidationDepth: 3,
			}

			validator = engine.NewWorkflowStateValidator(config, logger)

			// Create test workflow execution
			execution = &engine.RuntimeWorkflowExecution{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "test-execution-001",
					WorkflowID: "test-workflow-001",
					Status:     "running",
					StartTime:  time.Now(),
					Metadata:   make(map[string]interface{}),
				},
				OperationalStatus: engine.ExecutionStatusRunning,
				Input: &engine.WorkflowInput{
					Parameters:  make(map[string]interface{}),
					Environment: "test",
					Priority:    engine.PriorityMedium,
					Requester:   "test-user",
					Context:     make(map[string]interface{}),
				},
				Context: &engine.ExecutionContext{
					User:          "test-user",
					RequestID:     "test-request-001",
					TraceID:       "test-trace-001",
					CorrelationID: "test-correlation-001",
					Variables:     make(map[string]interface{}),
					Configuration: make(map[string]interface{}),
				},
				Steps:       make([]*engine.StepExecution, 0),
				CurrentStep: 0,
				Duration:    0,
			}
		})

		AfterEach(func() {
			if validator != nil {
				validator.Stop()
			}
			cancel()
		})

		It("should initialize with proper state validation configuration", func() {
			// Business Validation: Validator should be properly configured
			Expect(validator).ToNot(BeNil())

			metrics := validator.GetValidationMetrics()
			Expect(metrics.TotalValidations).To(Equal(int64(0)))
			Expect(metrics.ValidationErrors).To(Equal(int64(0)))
			Expect(metrics.IsHealthy).To(BeTrue())
		})

		It("should validate basic workflow execution state consistency", func() {
			// Act: Validate basic execution state
			result := validator.ValidateExecutionState(ctx, execution)

			// Business Validation: Basic state should be consistent
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Errors).To(BeEmpty())
			Expect(result.ConsistencyScore).To(BeNumerically(">=", 0.95))
			Expect(result.ValidationChecks).To(HaveKey("basic_state"))
			Expect(result.ValidationChecks).To(HaveKey("execution_metadata"))
			Expect(result.ValidationChecks).To(HaveKey("operational_status"))
		})

		It("should detect state inconsistencies in workflow execution", func() {
			// Arrange: Create inconsistent state
			execution.OperationalStatus = engine.ExecutionStatusCompleted
			execution.EndTime = nil // Missing end time for completed execution
			execution.CurrentStep = 5
			execution.Steps = make([]*engine.StepExecution, 3) // Current step beyond available steps

			// Act: Validate inconsistent state
			result := validator.ValidateExecutionState(ctx, execution)

			// Business Validation: Should detect inconsistencies
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).ToNot(BeEmpty())
			Expect(result.ConsistencyScore).To(BeNumerically("<", 0.5))

			// Should detect specific inconsistencies
			errorMessages := result.GetErrorSummary()
			Expect(errorMessages).To(ContainSubstring("completed status without end time"))
			Expect(errorMessages).To(ContainSubstring("current step index out of bounds"))
		})

		It("should validate step execution state consistency", func() {
			// Arrange: Add test steps
			step1 := &engine.StepExecution{
				StepID:     "step-001",
				Status:     engine.ExecutionStatusCompleted,
				StartTime:  time.Now().Add(-5 * time.Minute),
				EndTime:    &[]time.Time{time.Now().Add(-3 * time.Minute)}[0],
				Duration:   2 * time.Minute,
				RetryCount: 0,
				Variables:  make(map[string]interface{}),
				Metadata:   make(map[string]interface{}),
			}

			step2 := &engine.StepExecution{
				StepID:     "step-002",
				Status:     engine.ExecutionStatusRunning,
				StartTime:  time.Now().Add(-2 * time.Minute),
				EndTime:    nil,
				Duration:   0,
				RetryCount: 1,
				Variables:  make(map[string]interface{}),
				Metadata:   make(map[string]interface{}),
			}

			execution.Steps = []*engine.StepExecution{step1, step2}
			execution.CurrentStep = 1

			// Act: Validate step state consistency
			result := validator.ValidateStepStates(ctx, execution)

			// Business Validation: Steps should be consistent
			Expect(result.IsValid).To(BeTrue())
			Expect(result.StepValidations).To(HaveLen(2))
			Expect(result.StepValidations[0].IsValid).To(BeTrue())
			Expect(result.StepValidations[1].IsValid).To(BeTrue())
			Expect(result.OverallConsistencyScore).To(BeNumerically(">=", 0.9))
		})

		It("should detect step timeline inconsistencies", func() {
			// Arrange: Create timeline inconsistencies
			step1 := &engine.StepExecution{
				StepID:    "step-001",
				Status:    engine.ExecutionStatusCompleted,
				StartTime: time.Now().Add(-1 * time.Minute), // Started after step2
				EndTime:   &[]time.Time{time.Now()}[0],
				Duration:  1 * time.Minute,
			}

			step2 := &engine.StepExecution{
				StepID:    "step-002",
				Status:    engine.ExecutionStatusRunning,
				StartTime: time.Now().Add(-5 * time.Minute), // Started before step1 but step1 completed first
				EndTime:   nil,
				Duration:  0,
			}

			execution.Steps = []*engine.StepExecution{step1, step2}

			// Act: Validate timeline consistency
			result := validator.ValidateExecutionTimeline(ctx, execution)

			// Business Validation: Should detect timeline issues
			Expect(result.IsValid).To(BeFalse())
			Expect(result.TimelineViolations).ToNot(BeEmpty())
			Expect(result.TimelineViolations[0].Type).To(Equal("step_order_violation"))
			Expect(result.OverallTimelineScore).To(BeNumerically("<", 0.7))
		})

		It("should validate state transitions are valid", func() {
			// Arrange: Simulate state transitions
			originalStatus := execution.OperationalStatus

			// Act: Validate valid state transition
			transitionResult := validator.ValidateStateTransition(ctx, execution, engine.ExecutionStatusRunning, engine.ExecutionStatusCompleted)

			// Business Validation: Valid transition should be allowed
			Expect(transitionResult.IsValidTransition).To(BeTrue())
			Expect(transitionResult.TransitionReason).To(ContainSubstring("valid transition"))
			Expect(transitionResult.RequiredPreconditions).ToNot(BeEmpty())

			// Test invalid transition
			invalidTransitionResult := validator.ValidateStateTransition(ctx, execution, engine.ExecutionStatusCompleted, engine.ExecutionStatusRunning)

			// Business Validation: Invalid transition should be rejected
			Expect(invalidTransitionResult.IsValidTransition).To(BeFalse())
			Expect(invalidTransitionResult.TransitionReason).To(ContainSubstring("invalid transition"))
			Expect(originalStatus).To(Equal(execution.OperationalStatus)) // State should remain unchanged
		})

		It("should handle concurrent state validation operations safely", func() {
			// Arrange: Setup for concurrent operations
			const numWorkers = 25
			const operationsPerWorker = 40

			var wg sync.WaitGroup
			var validationCount int64
			var successfulValidations int64
			var consistencyViolations int64

			// Create multiple workflow executions for concurrent validation
			executions := make([]*engine.RuntimeWorkflowExecution, numWorkers)
			for i := 0; i < numWorkers; i++ {
				executions[i] = &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         fmt.Sprintf("test-execution-%03d", i),
						WorkflowID: "test-workflow-concurrent",
						Status:     "running",
						StartTime:  time.Now(),
						Metadata:   make(map[string]interface{}),
					},
					OperationalStatus: engine.ExecutionStatusRunning,
					Input: &engine.WorkflowInput{
						Parameters:  make(map[string]interface{}),
						Environment: "test-concurrent",
						Priority:    engine.PriorityMedium,
					},
					Context: &engine.ExecutionContext{
						Variables:     make(map[string]interface{}),
						Configuration: make(map[string]interface{}),
					},
					Steps: make([]*engine.StepExecution, 0),
				}
			}

			// Act: Concurrent validation operations
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerWorker; j++ {
						atomic.AddInt64(&validationCount, 1)
						targetExecution := executions[workerID]

						// Validate execution state
						result := validator.ValidateExecutionState(ctx, targetExecution)
						if result.IsValid {
							atomic.AddInt64(&successfulValidations, 1)
						} else {
							atomic.AddInt64(&consistencyViolations, 1)
						}

						// Test state transitions
						_ = validator.ValidateStateTransition(ctx, targetExecution,
							engine.ExecutionStatusRunning, engine.ExecutionStatusPaused)

						// Small delay to increase concurrency pressure
						time.Sleep(time.Microsecond * 10)
					}
				}(i)
			}

			// Wait for completion with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(30 * time.Second):
				Fail("Test timed out - possible deadlock in state validation")
			}

			// Business Validation: All operations should complete without corruption
			finalValidations := atomic.LoadInt64(&validationCount)
			finalSuccessful := atomic.LoadInt64(&successfulValidations)
			finalViolations := atomic.LoadInt64(&consistencyViolations)

			expectedTotal := int64(numWorkers * operationsPerWorker)
			Expect(finalValidations).To(Equal(expectedTotal))
			Expect(finalSuccessful + finalViolations).To(Equal(expectedTotal))

			// Validator should remain healthy after concurrent operations
			metrics := validator.GetValidationMetrics()
			Expect(metrics.IsHealthy).To(BeTrue())
			Expect(metrics.TotalValidations).To(BeNumerically(">=", finalValidations))
		})

		It("should provide comprehensive validation metrics and monitoring", func() {
			// Arrange: Perform various validation operations
			validator.ValidateExecutionState(ctx, execution)
			validator.ValidateStepStates(ctx, execution)
			validator.ValidateExecutionTimeline(ctx, execution)

			// Act: Get validation metrics
			metrics := validator.GetValidationMetrics()

			// Business Validation: Metrics should be accurate and comprehensive
			Expect(metrics.TotalValidations).To(BeNumerically(">=", 3))
			Expect(metrics.AverageValidationTime).To(BeNumerically(">", 0))
			Expect(metrics.ValidationSuccessRate).To(BeNumerically(">=", 0.8))
			Expect(metrics.IsHealthy).To(BeTrue())
			Expect(metrics.ValidationWorkerCount).To(Equal(5)) // From config
			Expect(metrics.LastValidationTime).To(BeTemporally("~", time.Now(), time.Second))

			// Performance metrics should be tracked
			Expect(metrics.PerformanceMetrics).ToNot(BeNil())
			Expect(metrics.PerformanceMetrics.AverageExecutionStateValidationTime).To(BeNumerically(">", 0))
		})

		It("should support state validation checkpoints for long-running workflows", func() {
			// Arrange: Create long-running workflow with multiple checkpoints
			execution.Duration = 2 * time.Hour
			execution.CurrentStep = 10

			// Add multiple completed steps
			for i := 0; i < 10; i++ {
				step := &engine.StepExecution{
					StepID:    fmt.Sprintf("step-%03d", i),
					Status:    engine.ExecutionStatusCompleted,
					StartTime: time.Now().Add(-2*time.Hour + time.Duration(i)*10*time.Minute),
					EndTime:   &[]time.Time{time.Now().Add(-2*time.Hour + time.Duration(i+1)*10*time.Minute)}[0],
					Duration:  10 * time.Minute,
				}
				execution.Steps = append(execution.Steps, step)
			}

			// Act: Validate with checkpoint support
			checkpointResult := validator.ValidateWithCheckpoints(ctx, execution)

			// Business Validation: Checkpoint validation should be comprehensive
			Expect(checkpointResult.IsValid).To(BeTrue())
			Expect(checkpointResult.CheckpointValidations).To(HaveLen(3)) // Based on config depth
			Expect(checkpointResult.CheckpointConsistencyScore).To(BeNumerically(">=", 0.9))

			// Each checkpoint should have detailed validation
			for _, checkpoint := range checkpointResult.CheckpointValidations {
				Expect(checkpoint.IsValid).To(BeTrue())
				Expect(checkpoint.ValidatedSteps).To(BeNumerically(">", 0))
				Expect(checkpoint.ValidationTime).To(BeNumerically(">", 0))
			}
		})

		It("should validate resource state consistency across workflow execution", func() {
			// Arrange: Add resource tracking to execution
			execution.Context.Variables["allocated_cpu"] = 2.0
			execution.Context.Variables["allocated_memory"] = "4Gi"
			execution.Context.Variables["resource_state"] = "allocated"

			step1 := &engine.StepExecution{
				StepID: "resource-step-001",
				Status: engine.ExecutionStatusCompleted,
				Variables: map[string]interface{}{
					"resource_usage_cpu":    1.5,
					"resource_usage_memory": "2Gi",
					"resource_state":        "consumed",
				},
			}

			execution.Steps = []*engine.StepExecution{step1}

			// Act: Validate resource state consistency
			resourceResult := validator.ValidateResourceStateConsistency(ctx, execution)

			// Business Validation: Resource state should be consistent
			Expect(resourceResult.IsValid).To(BeTrue())
			Expect(resourceResult.ResourceConsistencyScore).To(BeNumerically(">=", 0.8))
			Expect(resourceResult.ResourceValidations).To(HaveKey("cpu_allocation"))
			Expect(resourceResult.ResourceValidations).To(HaveKey("memory_allocation"))
			Expect(resourceResult.ResourceValidations).To(HaveKey("resource_state_progression"))
		})

		It("should generate detailed validation reports for compliance", func() {
			// Arrange: Create execution with various states
			execution.Steps = []*engine.StepExecution{
				{
					StepID:     "compliance-step-001",
					Status:     engine.ExecutionStatusCompleted,
					StartTime:  time.Now().Add(-10 * time.Minute),
					EndTime:    &[]time.Time{time.Now().Add(-8 * time.Minute)}[0],
					Duration:   2 * time.Minute,
					RetryCount: 1,
				},
				{
					StepID:     "compliance-step-002",
					Status:     engine.ExecutionStatusRunning,
					StartTime:  time.Now().Add(-8 * time.Minute),
					RetryCount: 0,
				},
			}

			// Act: Generate compliance validation report
			report := validator.GenerateComplianceValidationReport(ctx, execution)

			// Business Validation: Report should be comprehensive and compliant
			Expect(report.ExecutionID).To(Equal(execution.ID))
			Expect(report.ValidationTimestamp).To(BeTemporally("~", time.Now(), time.Second))
			Expect(report.OverallComplianceScore).To(BeNumerically(">=", 0.85))
			Expect(report.BusinessRequirementsCoverage).To(HaveKey("BR-REL-011"))
			Expect(report.BusinessRequirementsCoverage).To(HaveKey("BR-DATA-014"))

			// Report should include detailed validation sections
			Expect(report.StateConsistencyValidation).ToNot(BeNil())
			Expect(report.TimelineValidation).ToNot(BeNil())
			Expect(report.ResourceValidation).ToNot(BeNil())
			Expect(report.ComplianceRecommendations).ToNot(BeEmpty())
		})
	})
})
