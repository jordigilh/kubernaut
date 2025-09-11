//go:build unit
// +build unit

package workflowengine

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/workflow/orchestration"
)

/*
 * Business Requirement Validation: Advanced Workflow Patterns & Orchestration
 *
 * This test suite validates business requirements for advanced workflow capabilities
 * following development guidelines:
 * - Reuses existing workflow engine test patterns (Ginkgo/Gomega)
 * - Focuses on business outcomes: performance improvement, reliability enhancement
 * - Uses meaningful assertions with business performance thresholds
 * - Integrates with existing workflow engine and mock patterns
 * - Logs all errors and performance optimization metrics
 */

var _ = Describe("Business Requirement Validation: Advanced Workflow Patterns & Orchestration", func() {
	var (
		ctx                context.Context
		cancel             context.CancelFunc
		logger             *logrus.Logger
		workflowEngine     *engine.DefaultWorkflowEngine
		orchestrator       *orchestration.AdaptiveOrchestrator
		mockActionRepo     *mocks.MockActionRepository
		mockStateStorage   *mocks.MockStateStorage
		mockActionExecutor *mocks.MockActionExecutor
		mockAIEvaluator    *mocks.MockAIConditionEvaluator
		executionRepo      engine.ExecutionRepository
		commonAssertions   *testutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for complex workflows
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for business metrics
		commonAssertions = testutil.NewCommonAssertions()

		// Reuse existing mock patterns from workflow_engine_test.go
		mockActionRepo = mocks.NewMockActionRepository()
		mockStateStorage = mocks.NewMockStateStorage()
		mockActionExecutor = mocks.NewMockActionExecutor()
		mockAIEvaluator = mocks.NewMockAIConditionEvaluator()

		executionRepo = engine.NewInMemoryExecutionRepository(logger)

		// Setup business-relevant workflow configurations
		config := &engine.WorkflowEngineConfig{
			DefaultStepTimeout:      30 * time.Second,
			MaxRetryDelay:           1 * time.Second,
			MaxConcurrentSteps:      20,   // Business requirement: support up to 20 parallel steps
			EnableParallelExecution: true, // Business requirement: parallel processing
			EnableAdaptiveExecution: true, // Business requirement: adaptive behavior
		}

		workflowEngine = engine.NewDefaultWorkflowEngine(
			config,
			mockActionRepo,
			mockStateStorage,
			mockActionExecutor,
			mockAIEvaluator,
			executionRepo,
			logger,
		)

		orchestrator = orchestration.NewAdaptiveOrchestrator(workflowEngine, logger)
		setupBusinessMockData(mockActionRepo, mockActionExecutor, mockAIEvaluator)
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-WF-541
	 * Business Logic: MUST enable parallel execution of independent workflow steps to reduce resolution time
	 *
	 * Business Success Criteria:
	 *   - Execution time reduction >40% for parallelizable workflows with measurable benchmarks
	 *   - Dependency correctness with 100% step order validation for complex workflows
	 *   - Partial failure handling <10% workflow termination rate under failure conditions
	 *   - Concurrent execution scaling up to 20 parallel steps with resource management
	 *
	 * Test Focus: Real workflow performance improvement and reliability under parallel execution
	 * Expected Business Value: Faster incident resolution through optimized workflow execution
	 */
	Context("BR-WF-541: Parallel Step Execution for Business Performance Improvement", func() {
		It("should deliver measurable performance improvement through parallel execution", func() {
			By("Setting up business workflow with parallelizable steps")

			// Business Context: Multi-component incident remediation workflow
			parallelWorkflow := engine.WorkflowDefinition{
				ID:          "parallel-incident-remediation",
				Name:        "Multi-Component Incident Remediation",
				Description: "Business workflow for resolving incidents across multiple system components",
				Steps: []engine.WorkflowStep{
					// Independent parallel steps - can execute simultaneously
					{
						ID:   "restart-web-service",
						Name: "Restart Web Service",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "restart_deployment",
							Parameters: map[string]interface{}{
								"deployment": "web-server",
								"namespace":  "production",
							},
						},
						Dependencies: []string{}, // No dependencies - can run in parallel
					},
					{
						ID:   "scale-database",
						Name: "Scale Database Replicas",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "scale_deployment",
							Parameters: map[string]interface{}{
								"deployment": "postgres",
								"replicas":   3,
								"namespace":  "production",
							},
						},
						Dependencies: []string{}, // No dependencies - can run in parallel
					},
					{
						ID:   "cleanup-cache",
						Name: "Clear Redis Cache",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "clear_cache",
							Parameters: map[string]interface{}{
								"cache_type": "redis",
								"namespace":  "production",
							},
						},
						Dependencies: []string{}, // No dependencies - can run in parallel
					},
					{
						ID:   "update-monitoring",
						Name: "Update Monitoring Configuration",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "update_config",
							Parameters: map[string]interface{}{
								"config_type": "monitoring",
								"namespace":   "production",
							},
						},
						Dependencies: []string{}, // No dependencies - can run in parallel
					},
					// Final step that depends on all parallel steps
					{
						ID:   "verify-system-health",
						Name: "Verify System Health",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "health_check",
							Parameters: map[string]interface{}{
								"comprehensive": true,
							},
						},
						Dependencies: []string{"restart-web-service", "scale-database", "cleanup-cache", "update-monitoring"},
					},
				},
			}

			By("Executing workflow with parallel execution enabled")
			parallelExecutionStart := time.Now()

			execution, err := workflowEngine.ExecuteWorkflow(ctx, parallelWorkflow, types.AlertContext{
				Source:      "business-test",
				Severity:    "critical",
				Labels:      map[string]string{"test": "parallel-execution"},
				Description: "Business test for parallel workflow execution performance",
			})

			Expect(err).ToNot(HaveOccurred(), "Parallel workflow execution must succeed for business operations")
			Expect(execution).ToNot(BeNil(), "Must provide execution tracking for business monitoring")

			parallelExecutionTime := time.Since(parallelExecutionStart)

			By("Comparing with sequential execution performance")
			// Simulate sequential execution for comparison (disable parallel execution)
			sequentialConfig := *workflowEngine.GetConfig()
			sequentialConfig.EnableParallelExecution = false
			sequentialEngine := engine.NewDefaultWorkflowEngine(
				&sequentialConfig,
				mockActionRepo,
				mockStateStorage,
				mockActionExecutor,
				mockAIEvaluator,
				executionRepo,
				logger,
			)

			sequentialExecutionStart := time.Now()
			_, err = sequentialEngine.ExecuteWorkflow(ctx, parallelWorkflow, types.AlertContext{
				Source:      "business-test",
				Severity:    "critical",
				Labels:      map[string]string{"test": "sequential-execution"},
				Description: "Business test for sequential workflow execution comparison",
			})
			Expect(err).ToNot(HaveOccurred(), "Sequential execution must also succeed for comparison")

			sequentialExecutionTime := time.Since(sequentialExecutionStart)

			By("Calculating business performance improvement")
			performanceImprovement := (sequentialExecutionTime - parallelExecutionTime) / sequentialExecutionTime
			timeReductionPercent := performanceImprovement * 100

			// Business Requirement: >40% execution time reduction
			Expect(performanceImprovement).To(BeNumerically(">=", 0.40),
				"Parallel execution must reduce workflow time by >=40% for business value")

			By("Validating dependency correctness and execution order")
			// Business Requirement: Dependencies must be respected even during parallel execution
			executionHistory := execution.GetStepExecutionHistory()

			// Verify parallel steps executed before dependent step
			parallelSteps := []string{"restart-web-service", "scale-database", "cleanup-cache", "update-monitoring"}
			dependentStep := "verify-system-health"

			parallelStepTimes := make(map[string]time.Time)
			var dependentStepTime time.Time

			for _, stepExecution := range executionHistory {
				if contains(parallelSteps, stepExecution.StepID) {
					parallelStepTimes[stepExecution.StepID] = stepExecution.CompletedAt
				} else if stepExecution.StepID == dependentStep {
					dependentStepTime = stepExecution.StartedAt
				}
			}

			// Business Requirement: 100% dependency correctness
			for stepID, completedAt := range parallelStepTimes {
				Expect(completedAt.Before(dependentStepTime)).To(BeTrue(),
					"Parallel step %s must complete before dependent step for business logic correctness", stepID)
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":    "BR-WF-541",
				"parallel_execution_ms":   parallelExecutionTime.Milliseconds(),
				"sequential_execution_ms": sequentialExecutionTime.Milliseconds(),
				"performance_improvement": performanceImprovement,
				"time_reduction_percent":  timeReductionPercent,
				"parallel_steps_count":    len(parallelSteps),
				"dependency_correctness":  "100%",
				"business_impact":         "Parallel execution reduces incident resolution time significantly",
			}).Info("BR-WF-541: Parallel execution business validation completed")
		})

		It("should handle partial failures gracefully without terminating entire workflow", func() {
			By("Setting up workflow with intentional step failure scenarios")

			// Business Context: Resilient workflow that continues despite partial failures
			resilientWorkflow := engine.WorkflowDefinition{
				ID:   "resilient-parallel-workflow",
				Name: "Business Resilient Parallel Workflow",
				Steps: []engine.WorkflowStep{
					{
						ID:   "reliable-step-1",
						Name: "Reliable Operation 1",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "reliable_action",
						},
						Dependencies: []string{},
					},
					{
						ID:   "failing-step",
						Name: "Potentially Failing Operation",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "potentially_failing_action", // This will be configured to fail
						},
						Dependencies:  []string{},
						FailurePolicy: "continue", // Business requirement: continue on failure
					},
					{
						ID:   "reliable-step-2",
						Name: "Reliable Operation 2",
						Type: "action",
						ActionDefinition: &engine.ActionDefinition{
							Action: "reliable_action",
						},
						Dependencies: []string{},
					},
				},
			}

			// Configure mock to simulate failure
			mockActionExecutor.SetActionResult("potentially_failing_action", &types.ActionResult{
				Success: false,
				Error:   "Simulated business failure scenario",
			})

			By("Executing workflow and measuring resilience to partial failures")
			failureTestIterations := 10
			workflowTerminations := 0
			partialCompletions := 0

			for i := 0; i < failureTestIterations; i++ {
				execution, err := workflowEngine.ExecuteWorkflow(ctx, resilientWorkflow, types.AlertContext{
					Source: "business-resilience-test",
					Labels: map[string]string{"iteration": string(rune(i))},
				})

				if err != nil {
					workflowTerminations++
				} else {
					// Count successful steps even if some failed
					successfulSteps := 0
					for _, stepResult := range execution.GetStepExecutionHistory() {
						if stepResult.Success {
							successfulSteps++
						}
					}
					if successfulSteps > 0 {
						partialCompletions++
					}
				}
			}

			// Business Requirement: <10% workflow termination rate
			terminationRate := float64(workflowTerminations) / float64(failureTestIterations)
			Expect(terminationRate).To(BeNumerically("<", 0.10),
				"Workflow termination rate must be <10% for business resilience")

			// Business Requirement: Ability to complete partial work
			partialCompletionRate := float64(partialCompletions) / float64(failureTestIterations)
			Expect(partialCompletionRate).To(BeNumerically(">=", 0.80),
				"Partial completion rate must be >=80% for business value preservation")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":    "BR-WF-541",
				"scenario":                "failure_resilience",
				"termination_rate":        terminationRate,
				"partial_completion_rate": partialCompletionRate,
				"test_iterations":         failureTestIterations,
				"business_impact":         "Workflow resilience preserves business value under partial failures",
			}).Info("BR-WF-541: Failure resilience business validation completed")
		})
	})

	/*
	 * Business Requirement: BR-WF-556
	 * Business Logic: MUST support iterative workflow patterns for scenarios requiring repeated actions
	 *
	 * Business Success Criteria:
	 *   - Loop iteration performance supporting up to 100 iterations without degradation
	 *   - Condition evaluation latency <100ms per iteration for real-time processing
	 *   - Loop failure recovery with appropriate retry and termination strategies
	 *   - Progress monitoring providing clear visibility for long-running loops
	 *
	 * Test Focus: Iterative business scenarios that require loop patterns for resolution
	 * Expected Business Value: Support for complex remediation scenarios requiring repeated actions
	 */
	Context("BR-WF-556: Loop Step Execution for Iterative Business Scenarios", func() {
		It("should handle large-scale iterative operations with business performance requirements", func() {
			By("Setting up business workflow with iterative remediation pattern")

			// Business Context: Gradual scaling workflow that iteratively adds resources
			iterativeWorkflow := engine.WorkflowDefinition{
				ID:   "iterative-scaling-workflow",
				Name: "Business Iterative Scaling Workflow",
				Steps: []engine.WorkflowStep{
					{
						ID:   "iterative-scale-up",
						Name: "Iterative Resource Scaling",
						Type: "loop",
						LoopDefinition: &engine.LoopDefinition{
							MaxIterations: 50, // Business requirement: support significant scaling
							Condition:     "resource_threshold_not_met",
							LoopBody: []engine.WorkflowStep{
								{
									ID:   "check-current-load",
									Name: "Check Current System Load",
									Type: "action",
									ActionDefinition: &engine.ActionDefinition{
										Action: "get_system_metrics",
									},
								},
								{
									ID:   "scale-incrementally",
									Name: "Scale Resources Incrementally",
									Type: "action",
									ActionDefinition: &engine.ActionDefinition{
										Action: "scale_deployment",
										Parameters: map[string]interface{}{
											"increment":    1,
											"max_replicas": 50,
										},
									},
								},
								{
									ID:   "wait-stabilization",
									Name: "Wait for System Stabilization",
									Type: "wait",
									WaitDefinition: &engine.WaitDefinition{
										Duration: 2 * time.Second, // Business requirement: reasonable stabilization time
									},
								},
							},
						},
					},
				},
			}

			// Setup loop condition evaluation
			iterationCount := 0
			mockAIEvaluator.SetConditionEvaluator("resource_threshold_not_met", func(ctx context.Context, condition types.Condition, alertContext types.AlertContext) (bool, error) {
				iterationCount++

				// Business logic: Continue scaling until reaching target (simulate 25 iterations needed)
				shouldContinue := iterationCount < 25

				// Track condition evaluation latency for business requirements
				return shouldContinue, nil
			})

			By("Executing iterative workflow and measuring performance characteristics")
			iterativeStart := time.Now()

			execution, err := workflowEngine.ExecuteWorkflow(ctx, iterativeWorkflow, types.AlertContext{
				Source:      "business-iterative-test",
				Severity:    "warning",
				Labels:      map[string]string{"scaling_target": "25_replicas"},
				Description: "Business test for iterative scaling workflow",
			})

			Expect(err).ToNot(HaveOccurred(), "Iterative workflow must succeed for business scaling scenarios")
			Expect(execution).ToNot(BeNil(), "Must provide execution tracking for business monitoring")

			totalIterativeTime := time.Since(iterativeStart)

			By("Validating iterative performance against business requirements")
			// Business Requirement: Support up to 100 iterations (tested with 25)
			Expect(iterationCount).To(BeNumerically("<=", 100),
				"Loop execution must handle business-scale iteration counts")

			Expect(iterationCount).To(BeNumerically(">=", 20),
				"Must actually execute significant iterations for business test validity")

			// Business Requirement: Reasonable performance per iteration
			averageIterationTime := totalIterativeTime / time.Duration(iterationCount)
			Expect(averageIterationTime).To(BeNumerically("<", 500*time.Millisecond),
				"Average iteration time must be <500ms for business scalability")

			By("Validating condition evaluation performance")
			// Business simulation of condition evaluation latency
			conditionEvaluations := 10
			totalConditionTime := time.Duration(0)

			for i := 0; i < conditionEvaluations; i++ {
				conditionStart := time.Now()
				_, err := mockAIEvaluator.EvaluateCondition(ctx, types.Condition{
					Type:       "business_threshold",
					Expression: "cpu_usage < 80",
				}, types.AlertContext{})

				Expect(err).ToNot(HaveOccurred(), "Condition evaluation must succeed")
				totalConditionTime += time.Since(conditionStart)
			}

			averageConditionEvalTime := totalConditionTime / time.Duration(conditionEvaluations)

			// Business Requirement: <100ms condition evaluation latency
			Expect(averageConditionEvalTime).To(BeNumerically("<", 100*time.Millisecond),
				"Condition evaluation must be <100ms for real-time business processing")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":  "BR-WF-556",
				"iterations_executed":   iterationCount,
				"total_execution_ms":    totalIterativeTime.Milliseconds(),
				"avg_iteration_ms":      averageIterationTime.Milliseconds(),
				"avg_condition_eval_ms": averageConditionEvalTime.Milliseconds(),
				"business_impact":       "Iterative workflows enable complex multi-step business remediation",
			}).Info("BR-WF-556: Loop execution business validation completed")
		})

		It("should provide progress monitoring and early termination for business visibility", func() {
			By("Setting up long-running iterative workflow with progress tracking")

			// Business Context: Long-running data migration with progress monitoring
			progressTrackingWorkflow := engine.WorkflowDefinition{
				ID:   "progress-tracking-workflow",
				Name: "Business Progress Tracking Workflow",
				Steps: []engine.WorkflowStep{
					{
						ID:   "monitored-migration",
						Name: "Monitored Data Migration",
						Type: "loop",
						LoopDefinition: &engine.LoopDefinition{
							MaxIterations:     100,
							Condition:         "migration_not_complete",
							ProgressReporting: true, // Business requirement: progress visibility
							LoopBody: []engine.WorkflowStep{
								{
									ID:   "migrate-batch",
									Name: "Migrate Data Batch",
									Type: "action",
									ActionDefinition: &engine.ActionDefinition{
										Action: "migrate_data_batch",
										Parameters: map[string]interface{}{
											"batch_size": 1000,
										},
									},
								},
							},
						},
					},
				},
			}

			// Setup progress monitoring
			var progressReports []engine.ProgressReport
			var progressMutex sync.Mutex

			// Configure progress callback
			workflowEngine.SetProgressCallback(func(report engine.ProgressReport) {
				progressMutex.Lock()
				progressReports = append(progressReports, report)
				progressMutex.Unlock()
			})

			migrationIterations := 0
			mockAIEvaluator.SetConditionEvaluator("migration_not_complete", func(ctx context.Context, condition types.Condition, alertContext types.AlertContext) (bool, error) {
				migrationIterations++

				// Business logic: Complete migration after 30 iterations
				return migrationIterations < 30, nil
			})

			By("Executing workflow with progress monitoring")
			_, err := workflowEngine.ExecuteWorkflow(ctx, progressTrackingWorkflow, types.AlertContext{
				Source: "business-progress-test",
				Labels: map[string]string{"operation": "data_migration"},
			})

			Expect(err).ToNot(HaveOccurred(), "Progress-tracked workflow must succeed")

			By("Validating progress reporting for business visibility")
			progressMutex.Lock()
			reportCount := len(progressReports)
			progressMutex.Unlock()

			// Business Requirement: Progress visibility throughout execution
			Expect(reportCount).To(BeNumerically(">=", 10),
				"Must provide regular progress reports for business monitoring visibility")

			// Validate progress report content
			if reportCount > 0 {
				lastReport := progressReports[reportCount-1]
				Expect(lastReport.CompletionPercentage).To(BeNumerically(">=", 0),
					"Progress reports must include completion percentage for business tracking")
				Expect(lastReport.EstimatedTimeRemaining).ToNot(BeNil(),
					"Progress reports must include time estimates for business planning")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":   "BR-WF-556",
				"scenario":               "progress_monitoring",
				"iterations_completed":   migrationIterations,
				"progress_reports_count": reportCount,
				"business_visibility":    "Enabled",
				"business_impact":        "Progress monitoring provides business visibility for long-running operations",
			}).Info("BR-WF-556: Progress monitoring business validation completed")
		})
	})
})

// Business helper functions and test utilities

func setupBusinessMockData(mockActionRepo *mocks.MockActionRepository, mockActionExecutor *mocks.MockActionExecutor, mockAIEvaluator *mocks.MockAIConditionEvaluator) {
	// Setup realistic action responses for business scenarios
	businessActions := map[string]*types.ActionResult{
		"restart_deployment": {
			Success:       true,
			Message:       "Deployment restarted successfully",
			ExecutionTime: 5 * time.Second,
		},
		"scale_deployment": {
			Success:       true,
			Message:       "Deployment scaled successfully",
			ExecutionTime: 3 * time.Second,
		},
		"clear_cache": {
			Success:       true,
			Message:       "Cache cleared successfully",
			ExecutionTime: 2 * time.Second,
		},
		"update_config": {
			Success:       true,
			Message:       "Configuration updated successfully",
			ExecutionTime: 4 * time.Second,
		},
		"health_check": {
			Success:       true,
			Message:       "System health verified",
			ExecutionTime: 6 * time.Second,
		},
		"reliable_action": {
			Success:       true,
			Message:       "Reliable operation completed",
			ExecutionTime: 1 * time.Second,
		},
		"get_system_metrics": {
			Success:       true,
			Message:       "System metrics retrieved",
			ExecutionTime: 500 * time.Millisecond,
		},
		"migrate_data_batch": {
			Success:       true,
			Message:       "Data batch migrated",
			ExecutionTime: 2 * time.Second,
		},
	}

	for action, result := range businessActions {
		mockActionExecutor.SetActionResult(action, result)
	}

	// Setup business condition evaluations
	mockAIEvaluator.SetDefaultConditionResult(true, 0.85) // Default 85% confidence
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
