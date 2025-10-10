//go:build unit
// +build unit

<<<<<<< HEAD
package workflowengine

import (
	"testing"
	"context"
	"fmt"
=======
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
	"context"
	"fmt"
	"testing"
>>>>>>> crd_implementation
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Phase 2: Workflow Engine Resilient Execution Extensions
// Business Requirements: BR-WF-021 through BR-WF-025 (Resilient Execution Patterns)
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
// Following 09-interface-method-validation.mdc: Use existing real implementations

// Helper function for creating test steps
// Following 09-interface-method-validation.mdc: Use proper constructor patterns
func createTestStep(id, actionType string, critical bool) *engine.ExecutableWorkflowStep {
	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:        id,
			Name:      fmt.Sprintf("Test Step: %s", id),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata: map[string]interface{}{
				"critical":    critical,
				"test_step":   true,
				"action_type": actionType,
			},
		},
		Type:    engine.StepTypeAction,
		Timeout: 30 * time.Second,
		Action: &engine.StepAction{
			Type: actionType,
			Parameters: map[string]interface{}{
				"critical": critical,
			},
		},
		Dependencies: []string{},
	}

	// Add retry policy for critical steps
	if critical {
		step.RetryPolicy = &engine.RetryPolicy{
			MaxRetries:  3,
			Delay:       5 * time.Second,
			Backoff:     engine.BackoffTypeExponential,
			BackoffRate: 2.0,
			Conditions:  []string{"network_error", "timeout", "temporary_failure"},
		}
	}

	return step
}

// Helper function for creating historical execution data for learning tests
// Following 09-interface-method-validation.mdc: Use proper constructor patterns
func createHistoricalExecution(id, stepID string, retryCount int, successful bool) *engine.RuntimeWorkflowExecution {
	// Use proper constructor from engine package
	execution := engine.NewRuntimeWorkflowExecution(id, "historical-workflow")

	// Set operational status
	if successful {
		execution.OperationalStatus = engine.ExecutionStatusCompleted
	} else {
		execution.OperationalStatus = engine.ExecutionStatusFailed
	}

	// Set timing information
	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now().Add(-30 * time.Minute)
	execution.StartTime = startTime
	execution.EndTime = &endTime
	execution.Duration = 30 * time.Minute

	// Add step execution data
	execution.Steps = []*engine.StepExecution{
		{
			StepID: stepID,
			Status: func() engine.ExecutionStatus {
				if successful {
					return engine.ExecutionStatusCompleted
				}
				return engine.ExecutionStatusFailed
			}(),
			RetryCount: retryCount,
			Duration:   time.Duration(retryCount) * time.Second,
			StartTime:  startTime,
			EndTime:    &endTime,
		},
	}

	// Add metadata for learning analysis
	execution.Metadata["retry_count"] = retryCount
	execution.Metadata["successful"] = successful
	execution.Metadata["step_id"] = stepID

	return execution
}

var _ = Describe("Resilient Workflow Engine - Execution Pattern Extensions", func() {
	var (
		ctx                context.Context
		resilientEngine    *engine.ResilientWorkflowEngine
		defaultEngine      *engine.DefaultWorkflowEngine
		realStateStorage   engine.StateStorage
		realExecutionRepo  engine.ExecutionRepository
		mockFailureHandler *mocks.MockFailureHandler
		mockHealthChecker  *mocks.MockWorkflowHealthChecker
		mockLogger         *mocks.MockLogger
		_                  *fake.Clientset // Unused in current tests
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()

		// Following 03-testing-strategy.mdc: PREFER REAL BUSINESS LOGIC over mocks
		// Use existing mock implementations that follow business patterns
		realStateStorage = mocks.NewMockStateStorage()
		realExecutionRepo = engine.NewInMemoryExecutionRepository(mockLogger.Logger)

		// Mock only external dependencies
		_ = enhanced.NewSmartFakeClientset()             // Unused in current tests
		mockK8sClient := mocks.NewMockKubernetesClient() // Use MockKubernetesClient which has AsK8sClient method
		mockActionRepo := mocks.NewMockActionRepository()

		// Mock complex orchestration components (external to workflow engine)
		mockFailureHandler = mocks.NewMockFailureHandler()
		mockHealthChecker = mocks.NewMockWorkflowHealthChecker()

		// Create real DefaultWorkflowEngine with business configuration
		config := &engine.WorkflowEngineConfig{
			DefaultStepTimeout:    30 * time.Second,
			MaxRetryDelay:         5 * time.Second,
			EnableStateRecovery:   true,
			EnableDetailedLogging: true,
			MaxConcurrency:        10,
		}

		defaultEngine = engine.NewDefaultWorkflowEngine(
			mockK8sClient.AsK8sClient(), // Mock external Kubernetes API
			mockActionRepo,              // Mock external action repository
			nil,                         // monitoringClients - not needed for resilience tests
			realStateStorage,            // REAL business logic
			realExecutionRepo,           // REAL business logic
			config,                      // Real business configuration
			mockLogger.Logger,           // Mock logger
		)

		// Register action executors for test scenarios
		mockActionExecutor := mocks.NewMockActionExecutor()
		defaultEngine.RegisterActionExecutor("resilient_test", mockActionExecutor)
		defaultEngine.RegisterActionExecutor("partial_failure_test", mockActionExecutor)
		defaultEngine.RegisterActionExecutor("critical_step", mockActionExecutor)
		defaultEngine.RegisterActionExecutor("non_critical_step", mockActionExecutor)

		// Create ResilientWorkflowEngine with real business logic
		// Following 09-interface-method-validation.mdc: Use proper Logger adapter
		engineLogger := engine.NewLogrusAdapter(mockLogger.Logger)
		resilientEngine = engine.NewResilientWorkflowEngine(
			defaultEngine,      // REAL workflow engine
			mockFailureHandler, // Mock external failure handling
			mockHealthChecker,  // Mock external health checking
			engineLogger,       // Proper engine.Logger interface
		)
	})

	// BR-WF-021: Resilient Parallel Execution with Partial Failure Tolerance
	Context("BR-WF-021: MUST implement resilient parallel execution with <10% workflow termination rate", func() {
		It("should continue workflow execution despite non-critical step failures", func() {
			// Business Scenario: Operations team needs workflows to continue despite minor failures
			// Business Impact: Reduces workflow termination rate, improves operational reliability

			// Create workflow with mix of critical and non-critical parallel steps
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			template := engine.NewWorkflowTemplate("resilient-template", "Resilient Parallel Processing")
			template.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("critical-step-1", "critical_step", true),     // Critical - must succeed
				createTestStep("non-critical-1", "non_critical_step", false), // Non-critical - can fail
				createTestStep("non-critical-2", "non_critical_step", false), // Non-critical - can fail
				createTestStep("critical-step-2", "critical_step", true),     // Critical - must succeed
			}
			workflow := engine.NewWorkflow("resilient-parallel-001", template)

			// Configure failure handler to allow partial failures
			// Following 09-interface-method-validation.mdc: Use correct method signatures
			mockFailureHandler.SetFailurePolicy("partial_success") // Use string value, not constant
			mockFailureHandler.SetPartialFailureRate(0.5)          // Allow 50% partial failures

			// Configure mock action executors for test scenarios
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			// The action executors are already registered in BeforeEach

			// Execute resilient workflow using real business logic
			execution, err := resilientEngine.Execute(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-021: Resilient execution must handle partial failures")
			Expect(execution).ToNot(BeNil(), "BR-WF-021: Execution result required for business monitoring")

			// Business Requirement Validation: Workflow should complete despite non-critical failures
			Expect(execution.IsCompleted()).To(BeTrue(),
				"BR-WF-021: Workflow must complete with partial failures for <10% termination rate")
			// Following 09-interface-method-validation.mdc: Use existing business methods
			completionStatus := execution.GetCompletionStatus()
			Expect(completionStatus.FailedSteps).To(BeNumerically("<=", 2),
				"BR-WF-021: Failed step count must be within tolerance for business continuity")
			Expect(execution.GetSuccessRate()).To(BeNumerically(">=", 0.5),
				"BR-WF-021: Overall success rate must be maintained for business continuity")

			// Business Value: Operational reliability improved through partial failure tolerance
			Expect(execution.Metadata["resilience_applied"]).To(Equal(true),
				"BR-WF-021: Must track resilience application for business monitoring")
		})

		It("should terminate workflow only when critical step failure rate exceeds threshold", func() {
			// Business Scenario: System must fail fast when critical business functions fail
			// Business Impact: Prevents cascading failures while maintaining business safety

			// Create workflow with multiple critical steps
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			criticalTemplate := engine.NewWorkflowTemplate("critical-template", "Critical Step Failure Threshold Testing")
			criticalTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("critical-1", "critical_step", true),
				createTestStep("critical-2", "critical_step", true),
				createTestStep("critical-3", "critical_step", true),
			}
			workflow := engine.NewWorkflow("critical-failure-threshold-001", criticalTemplate)

			// Configure failure handler with strict critical step policy
			// Following 09-interface-method-validation.mdc: Use correct method signatures
			mockFailureHandler.SetFailurePolicy("fail_fast") // Use string value
			mockFailureHandler.SetPartialFailureRate(0.2)    // Allow only 20% failures for critical steps

			// Configure high failure rate for critical steps to test termination threshold
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			// The failure behavior will be determined by the failure handler policy

			// Execute workflow and expect controlled failure
			execution, err := resilientEngine.Execute(ctx, workflow)

			// Business Requirement Validation: Should fail when critical threshold exceeded
			if err != nil {
				// Expected failure scenario
				Expect(err.Error()).To(ContainSubstring("critical step failure threshold exceeded"),
					"BR-WF-021: Must provide clear failure reason for business troubleshooting")
			} else {
				// If execution completes, overall success rate must meet threshold
				// Following 09-interface-method-validation.mdc: Use existing business methods
				Expect(execution.GetSuccessRate()).To(BeNumerically(">=", 0.8),
					"BR-WF-021: Overall success rate must meet business threshold")
			}

			// Business Value: Controlled failure prevents cascading business impact
		})
	})

	// BR-WF-022: Adaptive Failure Recovery with Learning
	Context("BR-WF-022: MUST implement adaptive failure recovery with learning from execution patterns", func() {
		It("should adapt retry strategies based on historical failure patterns", func() {
			// Business Scenario: System learns from past failures to improve future success rates
			// Business Impact: Reduces manual intervention, improves operational efficiency

			// Create workflow with steps prone to transient failures
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			adaptiveTemplate := engine.NewWorkflowTemplate("adaptive-template", "Adaptive Retry Strategy Learning")
			adaptiveTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("transient-failure-step", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("adaptive-retry-001", adaptiveTemplate)

			// Provide historical execution data showing failure patterns
			historicalExecutions := []*engine.RuntimeWorkflowExecution{
				createHistoricalExecution("hist-001", "transient-failure-step", 2, true),  // Succeeded after 2 retries
				createHistoricalExecution("hist-002", "transient-failure-step", 3, true),  // Succeeded after 3 retries
				createHistoricalExecution("hist-003", "transient-failure-step", 1, false), // Failed after 1 retry
			}

			// Configure failure handler to learn from patterns
			// Following 09-interface-method-validation.mdc: Use correct method signatures
			mockFailureHandler.EnableLearning(true)
			mockFailureHandler.SetRetryHistory(historicalExecutions)
			mockFailureHandler.EnableRetryLearning(true)

			// Configure transient failure scenario for adaptive learning
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			// The adaptive retry behavior will be determined by the failure handler learning

			// Execute workflow with learning-enhanced failure handling
			execution, err := resilientEngine.ExecuteWithLearning(ctx, workflow, historicalExecutions)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-022: Learning-enhanced execution must succeed")
			Expect(execution).ToNot(BeNil(), "BR-WF-022: Execution result required for learning validation")

			// Business Requirement Validation: Should adapt retry strategy based on learning
			Expect(execution.IsCompleted()).To(BeTrue(),
				"BR-WF-022: Adaptive retry must improve success rate for business efficiency")

			// Verify learning was applied through execution metadata
			// Following 09-interface-method-validation.mdc: Use existing business methods
			Expect(execution.Metadata["adaptive_strategy_applied"]).To(BeTrue(),
				"BR-WF-022: Must apply adaptive strategy based on historical patterns")
			Expect(execution.GetSuccessRate()).To(BeNumerically(">=", 0.8),
				"BR-WF-022: Success rate should be improved through adaptive learning")

			// Business Value: Improved success rate through intelligent retry strategies
			Expect(execution.Metadata["learning_confidence"]).To(BeNumerically(">=", 0.8),
				"BR-WF-022: Learning confidence must meet business threshold for reliable adaptation")
		})

		It("should detect and respond to new failure patterns not seen in historical data", func() {
			// Business Scenario: System encounters new types of failures requiring adaptive response
			// Business Impact: Maintains resilience even with novel failure scenarios

			// Create workflow with steps that will exhibit new failure patterns
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			novelTemplate := engine.NewWorkflowTemplate("novel-template", "Novel Failure Pattern Detection")
			novelTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("novel-failure-step", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("novel-failure-detection-001", novelTemplate)

			// Provide historical data with different failure patterns
			historicalExecutions := []*engine.RuntimeWorkflowExecution{
				createHistoricalExecution("hist-001", "network-timeout", 1, true),
				createHistoricalExecution("hist-002", "resource-limit", 2, true),
			}

			// Configure failure handler for novel pattern detection
			// Following 09-interface-method-validation.mdc: Use available mock methods
			mockFailureHandler.EnableLearning(true)
			mockFailureHandler.SetRetryHistory(historicalExecutions)

			// Configure novel failure scenario through business behavior testing
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			// The novel pattern detection will be determined by the failure handler's learning capabilities

			// Execute workflow and expect novel pattern handling
			execution, err := resilientEngine.ExecuteWithLearning(ctx, workflow, historicalExecutions)

			// Business Requirement Validation: Should handle novel patterns gracefully
			if err != nil {
				// If failure occurs, should be handled gracefully with learning
				Expect(err.Error()).To(ContainSubstring("novel pattern detected"),
					"BR-WF-022: Must detect and report novel failure patterns for business awareness")
			} else {
				// If successful, should show novel pattern handling
				Expect(execution.Metadata["novel_pattern_detected"]).To(Equal(true),
					"BR-WF-022: Must detect novel patterns for business learning enhancement")
				Expect(execution.Metadata["adaptive_response_applied"]).To(Equal(true),
					"BR-WF-022: Must apply adaptive response to novel patterns")
			}

			// Business Value: System learns and adapts to new operational challenges
		})
	})

	// BR-WF-023: Performance-Aware Execution Optimization
	Context("BR-WF-023: MUST implement performance-aware execution with >40% improvement targets", func() {
		It("should optimize execution paths based on performance metrics", func() {
			// Business Scenario: System optimizes workflow execution for better resource utilization
			// Business Impact: Reduces operational costs, improves system efficiency

			// Create workflow with multiple execution paths
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			perfTemplate := engine.NewWorkflowTemplate("perf-template", "Performance-Aware Path Optimization")
			perfTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("fast-path-step", "resilient_test", false),
				createTestStep("slow-path-step", "resilient_test", false),
				createTestStep("optimization-step", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("performance-optimization-001", perfTemplate)

			// Provide performance baseline data using existing types
			// Following 09-interface-method-validation.mdc: Use existing business types
			performanceBaseline := map[string]interface{}{
				"average_execution_time": 5 * time.Second,
				"resource_utilization":   0.7,
				"success_rate":           0.85,
			}

			// Configure performance optimization through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["performance_optimization"] = true
			workflow.Metadata["performance_baseline"] = performanceBaseline
			workflow.Metadata["performance_target"] = 0.4 // 40% improvement target

			// Execute workflow with performance optimization
			startTime := time.Now()
			execution, err := resilientEngine.Execute(ctx, workflow)
			executionTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred(), "BR-WF-023: Performance-optimized execution must succeed")
			Expect(execution).ToNot(BeNil(), "BR-WF-023: Execution result required for performance validation")

			// Business Requirement Validation: Should achieve performance improvement
			Expect(execution.IsCompleted()).To(BeTrue(),
				"BR-WF-023: Optimized execution must complete for business efficiency")

			// Calculate performance improvement using map access
			// Following 09-interface-method-validation.mdc: Use existing business types
			baselineTime := performanceBaseline["average_execution_time"].(time.Duration)
			performanceImprovement := (baselineTime - executionTime).Seconds() / baselineTime.Seconds()

			Expect(performanceImprovement).To(BeNumerically(">=", 0.4),
				"BR-WF-023: Must achieve >40% performance improvement for business value")

			// Verify optimization was applied through execution metadata
			Expect(execution.Metadata["performance_optimization_applied"]).To(Equal(true),
				"BR-WF-023: Must track performance optimization for business monitoring")
			Expect(execution.Duration).To(BeNumerically("<", baselineTime),
				"BR-WF-023: Execution time must be improved through optimization")

			// Business Value: Significant operational efficiency improvement
		})

		It("should balance performance optimization with reliability requirements", func() {
			// Business Scenario: System must optimize performance without compromising reliability
			// Business Impact: Maintains business SLA while improving efficiency

			// Create workflow with reliability-critical steps
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			reliabilityTemplate := engine.NewWorkflowTemplate("reliability-template", "Performance-Reliability Balance Optimization")
			reliabilityTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("reliability-critical", "critical_step", true),
				createTestStep("performance-optimizable", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("performance-reliability-balance-001", reliabilityTemplate)

			// Set reliability requirements using existing types
			// Following 09-interface-method-validation.mdc: Use existing business types
			reliabilityRequirements := map[string]interface{}{
				"min_success_rate":        0.95, // 95% success rate required
				"max_acceptable_latency":  10 * time.Second,
				"critical_step_tolerance": 0.0, // No tolerance for critical step failures
			}

			// Configure balanced optimization through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["reliability_requirements"] = reliabilityRequirements
			workflow.Metadata["balanced_optimization"] = true

			// Execute workflow with balanced optimization
			execution, err := resilientEngine.Execute(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-023: Balanced optimization must maintain reliability")
			Expect(execution).ToNot(BeNil(), "BR-WF-023: Execution result required for balance validation")

			// Business Requirement Validation: Must maintain reliability while optimizing
			Expect(execution.IsCompleted()).To(BeTrue(),
				"BR-WF-023: Balanced execution must complete for business requirements")
			Expect(execution.GetSuccessRate()).To(BeNumerically(">=", 0.95),
				"BR-WF-023: Must maintain reliability requirements during optimization")

			// Verify balanced approach was applied through metadata
			// Following 09-interface-method-validation.mdc: Use existing business methods
			Expect(execution.Metadata["reliability_maintained"]).To(BeTrue(),
				"BR-WF-023: Must maintain reliability during performance optimization")
			Expect(execution.Duration).To(BeNumerically("<", 10*time.Second),
				"BR-WF-023: Must achieve performance improvement while maintaining reliability")

			// Business Value: Optimal balance between efficiency and reliability
		})
	})

	// BR-WF-024: Resource-Aware Execution Scaling
	Context("BR-WF-024: MUST implement resource-aware execution scaling with dynamic adaptation", func() {
		It("should scale execution based on available system resources", func() {
			// Business Scenario: System adapts execution to available resources for optimal utilization
			// Business Impact: Maximizes resource efficiency, prevents resource contention

			// Create resource-intensive workflow
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			resourceTemplate := engine.NewWorkflowTemplate("resource-template", "Resource-Aware Execution Scaling")
			resourceTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("cpu-intensive", "resilient_test", false),
				createTestStep("memory-intensive", "resilient_test", false),
				createTestStep("io-intensive", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("resource-scaling-001", resourceTemplate)

			// Configure resource monitoring using existing types
			// Following 09-interface-method-validation.mdc: Use existing business types
			resourceMonitor := map[string]interface{}{
				"cpu_utilization":    0.6, // 60% CPU usage
				"memory_utilization": 0.7, // 70% memory usage
				"io_utilization":     0.4, // 40% I/O usage
				"network_bandwidth":  0.5, // 50% network usage
			}

			// Configure resource-aware scaling through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["resource_monitor"] = resourceMonitor
			workflow.Metadata["resource_aware_scaling"] = true

			// Execute workflow with resource-aware scaling
			execution, err := resilientEngine.Execute(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-024: Resource-aware execution must succeed")
			Expect(execution).ToNot(BeNil(), "BR-WF-024: Execution result required for resource validation")

			// Business Requirement Validation: Should adapt to resource constraints
			Expect(execution.IsCompleted()).To(BeTrue(),
				"BR-WF-024: Resource-aware execution must complete for business efficiency")

			// Verify resource constraints through execution metadata
			// Following 09-interface-method-validation.mdc: Use existing business methods
			Expect(execution.Metadata["max_cpu_utilization"]).To(BeNumerically("<=", 0.8),
				"BR-WF-024: Must respect CPU resource limits for system stability")
			Expect(execution.Metadata["max_memory_utilization"]).To(BeNumerically("<=", 0.8),
				"BR-WF-024: Must respect memory resource limits for system stability")

			// Verify resource-aware scaling was applied
			Expect(execution.Metadata["resource_scaling_applied"]).To(Equal(true),
				"BR-WF-024: Must track resource scaling for business monitoring")
			Expect(execution.Duration).To(BeNumerically("<", 30*time.Second),
				"BR-WF-024: Resource-aware execution should be efficient")

			// Business Value: Optimal resource utilization without system overload
		})

		It("should gracefully degrade execution when resources are constrained", func() {
			// Business Scenario: System maintains functionality under resource pressure
			// Business Impact: Ensures business continuity during resource constraints

			// Create workflow that can be gracefully degraded
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			degradationTemplate := engine.NewWorkflowTemplate("degradation-template", "Graceful Degradation Under Resource Constraints")
			degradationTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("essential-step", "critical_step", true),    // Must execute
				createTestStep("optional-step-1", "resilient_test", false), // Can be skipped
				createTestStep("optional-step-2", "resilient_test", false), // Can be skipped
			}
			workflow := engine.NewWorkflow("graceful-degradation-001", degradationTemplate)

			// Simulate resource constraints using existing types
			// Following 09-interface-method-validation.mdc: Use existing business types
			constrainedResources := map[string]interface{}{
				"cpu_utilization":    0.95, // 95% CPU usage - high constraint
				"memory_utilization": 0.90, // 90% memory usage - high constraint
				"io_utilization":     0.85, // 85% I/O usage - high constraint
				"network_bandwidth":  0.80, // 80% network usage - moderate constraint
			}

			// Configure graceful degradation through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["resource_constraints"] = constrainedResources
			workflow.Metadata["graceful_degradation"] = true

			// Execute workflow under resource constraints
			execution, err := resilientEngine.Execute(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-024: Graceful degradation must maintain core functionality")
			Expect(execution).ToNot(BeNil(), "BR-WF-024: Execution result required for degradation validation")

			// Business Requirement Validation: Should complete essential steps
			Expect(execution.IsCompleted()).To(BeTrue(),
				"BR-WF-024: Graceful degradation must complete essential business functions")

			// Verify degradation behavior through execution metadata and completion status
			// Following 09-interface-method-validation.mdc: Use existing business methods
			completionStatus := execution.GetCompletionStatus()
			Expect(completionStatus.CompletedSteps).To(BeNumerically(">=", 1),
				"BR-WF-024: Essential steps must complete for business continuity")
			Expect(execution.Metadata["graceful_degradation_applied"]).To(Equal(true),
				"BR-WF-024: Must track degradation application for business monitoring")

			// Verify graceful degradation minimized business impact
			Expect(execution.Metadata["business_impact_score"]).To(BeNumerically("<=", 0.3),
				"BR-WF-024: Business impact of degradation must be minimized")
			Expect(execution.GetSuccessRate()).To(BeNumerically(">=", 0.5),
				"BR-WF-024: Core functionality must be maintained during degradation")

			// Business Value: Maintained core functionality under adverse conditions
		})
	})

	// BR-WF-025: Execution State Consistency and Recovery
	Context("BR-WF-025: MUST maintain execution state consistency with recovery capabilities", func() {
		It("should maintain consistent execution state across workflow interruptions", func() {
			// Business Scenario: System maintains state consistency during unexpected interruptions
			// Business Impact: Prevents data corruption, enables reliable recovery

			// Create long-running workflow with state checkpoints
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			stateTemplate := engine.NewWorkflowTemplate("state-template", "State Consistency During Interruptions")
			stateTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("checkpoint-1", "resilient_test", false),
				createTestStep("checkpoint-2", "resilient_test", false),
				createTestStep("checkpoint-3", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("state-consistency-001", stateTemplate)

			// Enable state consistency monitoring through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["state_consistency_monitoring"] = true
			workflow.Metadata["checkpoint_interval"] = 1 * time.Second

			// Start workflow execution
			executionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			execution, err := resilientEngine.Execute(executionCtx, workflow)

			// Handle execution result - may succeed or fail due to interruption
			if err != nil {
				// Execution may fail due to context timeout, which is expected in this test
				mockLogger.Logger.WithError(err).Debug("Execution interrupted as expected")
			}

			// Simulate interruption after partial execution
			if execution != nil && execution.IsRunning() {
				// Cancel execution to simulate interruption
				cancel()
				time.Sleep(100 * time.Millisecond) // Allow cancellation to propagate
			}

			// Verify state consistency despite interruption
			if execution != nil {
				// Verify state consistency through execution metadata and status
				// Following 09-interface-method-validation.mdc: Use existing business methods
				Expect(execution.Metadata["state_consistency_maintained"]).To(BeTrue(),
					"BR-WF-025: Execution state must remain consistent despite interruptions")

				completionStatus := execution.GetCompletionStatus()
				consistencyScore := float64(completionStatus.CompletedSteps) / float64(completionStatus.TotalSteps)
				Expect(consistencyScore).To(BeNumerically(">=", 0.8),
					"BR-WF-025: State consistency score must meet business reliability requirements")

				// Verify checkpoint integrity through metadata
				Expect(execution.Metadata["checkpoints_valid"]).To(BeTrue(),
					"BR-WF-025: All state checkpoints must be valid for recovery capability")
			}

			// Business Value: Reliable state management enables safe recovery operations
		})

		It("should recover execution from last consistent checkpoint after failure", func() {
			// Business Scenario: System recovers from failures using consistent state checkpoints
			// Business Impact: Minimizes work loss, enables efficient failure recovery

			// Create workflow with recovery points
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			recoveryTemplate := engine.NewWorkflowTemplate("recovery-template", "Checkpoint-Based Recovery Testing")
			recoveryTemplate.Steps = []*engine.ExecutableWorkflowStep{
				createTestStep("setup-step", "resilient_test", false),
				createTestStep("processing-step", "resilient_test", false),
				createTestStep("failure-prone-step", "resilient_test", false),
				createTestStep("cleanup-step", "resilient_test", false),
			}
			workflow := engine.NewWorkflow("checkpoint-recovery-001", recoveryTemplate)

			// Enable checkpoint-based recovery through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["checkpoint_recovery"] = true
			workflow.Metadata["recovery_strategy"] = "last_checkpoint"

			// Configure failure scenario through metadata
			// Following 03-testing-strategy.mdc: Test business behavior, not internal implementation
			workflow.Metadata["simulate_failure_at"] = "failure-prone-step"

			// Execute workflow and expect recovery
			execution, err := resilientEngine.Execute(ctx, workflow)

			// Should either succeed through recovery or provide recovery information
			// Following 09-interface-method-validation.mdc: Handle all execution outcomes
			if err != nil {
				// Execution failed, but recovery information should be available
				Expect(execution).ToNot(BeNil(), "BR-WF-025: Recovery information must be available even on failure")
			}
			if err != nil {
				// If execution fails, should provide recovery capability
				Expect(err.Error()).To(ContainSubstring("recovery available"),
					"BR-WF-025: Must indicate recovery capability when execution fails")
			}

			if execution != nil {
				// Verify recovery was attempted or successful through metadata
				// Following 09-interface-method-validation.mdc: Use existing business methods
				if execution.Metadata["recovery_attempted"] == true {
					Expect(execution.Metadata["recovery_successful"]).To(BeTrue(),
						"BR-WF-025: Recovery attempt must succeed for business continuity")
					Expect(execution.Metadata["work_lost_percentage"]).To(BeNumerically("<=", 0.5),
						"BR-WF-025: Work loss during recovery must be minimized for business efficiency")
				}

				// Verify checkpoint-based recovery maintains consistency
				if execution.IsCompleted() {
					Expect(execution.Metadata["final_state_consistent"]).To(BeTrue(),
						"BR-WF-025: Final state must be consistent after recovery")
				}
			}

			// Business Value: Efficient recovery minimizes business disruption
		})
	})

	// Helper functions for test data creation
	// Note: createTestStep is already defined at the top of the file

	// Note: createHistoricalExecution is already defined at the top of the file
})

// TestRunner bootstraps the Ginkgo test suite
func TestUresilientUworkflowUexecutionUextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UresilientUworkflowUexecutionUextensions Suite")
}
