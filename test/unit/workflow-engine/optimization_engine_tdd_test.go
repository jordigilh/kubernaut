//go:build unit
// +build unit

package workflowengine

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TDD Implementation: OptimizationEngine Business Requirements
// BR-ORCH-001: MUST continuously optimize with ≥80% confidence, ≥15% performance gains
// Following 00-project-guidelines.mdc: TDD workflow (RED → GREEN → REFACTOR)
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks

// Helper functions for test data creation
// Following 09-interface-method-validation.mdc: Use proper constructor patterns
func createOptimizationHistoryExecution(id string, duration time.Duration, successful bool) *engine.RuntimeWorkflowExecution {
	// Use proper constructor from constructors.go
	execution := engine.NewRuntimeWorkflowExecution(id, "optimization-test-workflow")

	// Set execution status based on success
	if successful {
		execution.OperationalStatus = engine.ExecutionStatusCompleted
		execution.Status = "completed"
	} else {
		execution.OperationalStatus = engine.ExecutionStatusFailed
		execution.Status = "failed"
	}

	// Set timing information
	startTime := time.Now().Add(-time.Hour)
	endTime := startTime.Add(duration)
	execution.StartTime = startTime
	execution.EndTime = &endTime
	execution.Duration = duration

	// Add test metadata
	execution.Metadata = map[string]interface{}{
		"execution_duration":   duration.Seconds(),
		"optimization_applied": false,
		"baseline_execution":   true,
	}

	return execution
}

func createPerformanceHistoryExecution(id string, duration time.Duration, optimizationGain float64) *engine.RuntimeWorkflowExecution {
	// Use proper constructor pattern
	execution := engine.NewRuntimeWorkflowExecution(id, "performance-test-workflow")
	execution.OperationalStatus = engine.ExecutionStatusCompleted
	execution.Status = "completed"

	// Set timing information
	startTime := time.Now().Add(-time.Hour)
	endTime := startTime.Add(duration)
	execution.StartTime = startTime
	execution.EndTime = &endTime
	execution.Duration = duration

	// Add performance metadata
	execution.Metadata = map[string]interface{}{
		"execution_duration":   duration.Seconds(),
		"optimization_gain":    optimizationGain,
		"performance_baseline": optimizationGain == 0.0,
	}

	return execution
}

func createLearningHistoryExecution(id, strategy string, gain, confidence float64) *engine.RuntimeWorkflowExecution {
	// Use proper constructor pattern
	execution := engine.NewRuntimeWorkflowExecution(id, "learning-test-workflow")
	execution.OperationalStatus = engine.ExecutionStatusCompleted
	execution.Status = "completed"

	// Set timing information
	startTime := time.Now().Add(-time.Hour)
	endTime := startTime.Add(20 * time.Second)
	execution.StartTime = startTime
	execution.EndTime = &endTime
	execution.Duration = 20 * time.Second

	// Add learning metadata
	execution.Metadata = map[string]interface{}{
		"optimization_strategy": strategy,
		"performance_gain":      gain,
		"confidence_score":      confidence,
		"learning_iteration":    true,
	}

	return execution
}

var _ = Describe("OptimizationEngine - TDD Implementation (BR-ORCH-001)", func() {
	var (
		ctx                context.Context
		optimizationEngine engine.OptimizationEngine
		mockLogger         *mocks.MockLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()

		// TDD RED PHASE: Use real ProductionOptimizationEngine with enhanced llm.Client
		// RULE 12 COMPLIANCE: Updated constructor to include llm.Client
		mockLLMClient := mocks.NewMockLLMClient()
		optimizationEngine = engine.NewProductionOptimizationEngine(mockLLMClient, mockLogger.Logger)
	})

	// BR-ORCH-001: Self-optimization with ≥80% confidence, ≥15% performance gains
	Context("BR-ORCH-001: Self-optimization Requirements", func() {
		It("should achieve ≥80% confidence in optimization recommendations", func() {
			// TDD RED PHASE: Write failing test first
			// Business Scenario: System must provide high-confidence optimization recommendations
			// Business Impact: Ensures reliable optimization decisions for production workloads

			// Create workflow with optimization opportunities
			// Following 09-interface-method-validation.mdc: Use proper constructor patterns
			template := engine.NewWorkflowTemplate("opt-template", "Optimization Test Template")

			// Create steps using proper ExecutableWorkflowStep structure
			step1 := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{
					ID:        "slow-step-1",
					Name:      "CPU Intensive Step",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Metadata: map[string]interface{}{
						"cpu_weight":    0.8,
						"memory_weight": 0.3,
						"avg_duration":  25 * time.Second,
					},
				},
				Type:    engine.StepTypeAction,
				Timeout: 30 * time.Second,
				Action: &engine.StepAction{
					Type: "cpu_intensive",
					Parameters: map[string]interface{}{
						"parallelizable": true,
					},
				},
				Dependencies: []string{}, // Empty dependencies for parallelization
			}

			step2 := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{
					ID:        "slow-step-2",
					Name:      "IO Intensive Step",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Metadata: map[string]interface{}{
						"io_weight":     0.9,
						"memory_weight": 0.6,
						"avg_duration":  40 * time.Second,
					},
				},
				Type:    engine.StepTypeAction,
				Timeout: 45 * time.Second,
				Action: &engine.StepAction{
					Type: "io_intensive",
					Parameters: map[string]interface{}{
						"parallelizable": true,
					},
				},
				Dependencies: []string{}, // Empty dependencies for parallelization
			}

			template.Steps = []*engine.ExecutableWorkflowStep{step1, step2}
			workflow := engine.NewWorkflow("optimization-confidence-test", template)

			// Provide historical execution data for confidence calculation
			executionHistory := []*engine.RuntimeWorkflowExecution{
				createOptimizationHistoryExecution("hist-001", 35*time.Second, true),
				createOptimizationHistoryExecution("hist-002", 42*time.Second, true),
				createOptimizationHistoryExecution("hist-003", 38*time.Second, true),
				createOptimizationHistoryExecution("hist-004", 33*time.Second, true),
				createOptimizationHistoryExecution("hist-005", 41*time.Second, true),
			}

			// TDD: Execute optimization analysis
			optimizationResult, err := optimizationEngine.OptimizeOrchestrationStrategies(ctx, workflow, executionHistory)

			// Business Requirement Validation: ≥80% confidence requirement
			Expect(err).ToNot(HaveOccurred(), "BR-ORCH-001: Optimization analysis must succeed for business reliability")
			Expect(optimizationResult).ToNot(BeNil(), "BR-ORCH-001: Optimization result required for business decision making")
			Expect(optimizationResult.Confidence).To(BeNumerically(">=", 0.80),
				"BR-ORCH-001: Optimization confidence must meet ≥80% business requirement")

			// Validate optimization recommendations quality
			// Note: OptimizationCandidates is interface{}, need to check if candidates exist
			Expect(optimizationResult.OptimizationCandidates).ToNot(BeNil(),
				"BR-ORCH-001: Must provide optimization candidates for business value")

			// Get candidates through the engine's AnalyzeOptimizationOpportunities method
			candidates, candidatesErr := optimizationEngine.AnalyzeOptimizationOpportunities(workflow)
			Expect(candidatesErr).ToNot(HaveOccurred(), "BR-ORCH-001: Must be able to analyze optimization opportunities")
			Expect(len(candidates)).To(BeNumerically(">", 0), "BR-ORCH-001: Must provide optimization candidates")

			for _, candidate := range candidates {
				Expect(candidate.Confidence).To(BeNumerically(">=", 0.80),
					"BR-ORCH-001: Each optimization candidate must meet confidence threshold")
			}

			// Business Value: High-confidence optimization enables safe production deployment
		})

		It("should achieve ≥15% performance improvement targets", func() {
			// TDD RED PHASE: Write failing test for performance gains
			// Business Scenario: System must deliver measurable performance improvements
			// Business Impact: Reduces operational costs and improves system efficiency

			// Create workflow with clear performance improvement opportunities
			perfTemplate := engine.NewWorkflowTemplate("perf-template", "Performance Test Template")
			perfStep1 := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{
					ID:        "parallelizable-step-1",
					Name:      "CPU Bound Step 1",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Metadata: map[string]interface{}{
						"parallelizable":    true,
						"current_duration":  18 * time.Second,
						"optimization_type": "parallelization",
					},
				},
				Type:    engine.StepTypeAction,
				Timeout: 20 * time.Second,
				Action: &engine.StepAction{
					Type: "cpu_bound",
					Parameters: map[string]interface{}{
						"parallelizable": true,
					},
				},
				Dependencies: []string{},
			}

			perfStep2 := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{
					ID:        "parallelizable-step-2",
					Name:      "CPU Bound Step 2",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Metadata: map[string]interface{}{
						"parallelizable":    true,
						"current_duration":  22 * time.Second,
						"optimization_type": "parallelization",
					},
				},
				Type:    engine.StepTypeAction,
				Timeout: 25 * time.Second,
				Action: &engine.StepAction{
					Type: "cpu_bound",
					Parameters: map[string]interface{}{
						"parallelizable": true,
					},
				},
				Dependencies: []string{},
			}

			perfTemplate.Steps = []*engine.ExecutableWorkflowStep{perfStep1, perfStep2}
			workflow := engine.NewWorkflow("performance-improvement-test", perfTemplate)

			// Historical data showing current performance baseline
			baselineHistory := []*engine.RuntimeWorkflowExecution{
				createPerformanceHistoryExecution("baseline-001", 40*time.Second, 0.0), // No optimization
				createPerformanceHistoryExecution("baseline-002", 43*time.Second, 0.0),
				createPerformanceHistoryExecution("baseline-003", 38*time.Second, 0.0),
			}

			// Execute optimization with performance improvement focus
			optimizationResult, err := optimizationEngine.OptimizeOrchestrationStrategies(ctx, workflow, baselineHistory)

			// Business Requirement Validation: ≥15% performance improvement
			Expect(err).ToNot(HaveOccurred(), "BR-ORCH-001: Performance optimization must succeed")
			Expect(optimizationResult).ToNot(BeNil(), "BR-ORCH-001: Performance result required for business metrics")
			Expect(optimizationResult.EstimatedPerformanceGain).To(BeNumerically(">=", 0.15),
				"BR-ORCH-001: Performance improvement must meet ≥15% business requirement")

			// Validate specific optimization strategies deliver gains
			// Get candidates through the engine's method since OptimizationCandidates is interface{}
			perfCandidates, perfCandidatesErr := optimizationEngine.AnalyzeOptimizationOpportunities(workflow)
			Expect(perfCandidatesErr).ToNot(HaveOccurred(), "BR-ORCH-001: Must be able to analyze opportunities")

			for _, candidate := range perfCandidates {
				if candidate.Type == "parallelization" {
					Expect(candidate.PredictedTimeReduction).To(BeNumerically(">=", 0.15),
						"BR-ORCH-001: Parallelization optimization must deliver ≥15% gain")
				}
			}

			// Verify optimization maintains reliability while improving performance
			Expect(optimizationResult.ReliabilityImpact).To(BeNumerically("<=", 0.05),
				"BR-ORCH-001: Performance optimization must not significantly impact reliability")

			// Business Value: Measurable performance improvements reduce operational costs
		})

		It("should continuously optimize based on execution feedback", func() {
			// TDD RED PHASE: Write failing test for continuous optimization
			// Business Scenario: System learns from execution results to improve future optimizations
			// Business Impact: Enables adaptive optimization that improves over time

			// Create workflow for continuous optimization testing
			contTemplate := engine.NewWorkflowTemplate("cont-template", "Continuous Optimization Template")
			contStep := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{
					ID:        "adaptive-step",
					Name:      "Adaptive Operation Step",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Metadata: map[string]interface{}{
						"optimization_history": []map[string]interface{}{
							{"strategy": "parallel", "gain": 0.12, "confidence": 0.75},
							{"strategy": "sequential", "gain": 0.08, "confidence": 0.85},
							{"strategy": "batch", "gain": 0.18, "confidence": 0.82},
						},
					},
				},
				Type:    engine.StepTypeAction,
				Timeout: 30 * time.Second,
				Action: &engine.StepAction{
					Type: "adaptive_operation",
				},
			}
			contTemplate.Steps = []*engine.ExecutableWorkflowStep{contStep}
			workflow := engine.NewWorkflow("continuous-optimization-test", contTemplate)

			// Execution history showing optimization learning progression
			learningHistory := []*engine.RuntimeWorkflowExecution{
				createLearningHistoryExecution("learn-001", "parallel", 0.12, 0.75),
				createLearningHistoryExecution("learn-002", "sequential", 0.08, 0.85),
				createLearningHistoryExecution("learn-003", "batch", 0.18, 0.82),
				createLearningHistoryExecution("learn-004", "batch", 0.20, 0.88), // Learning improvement
			}

			// Execute continuous optimization
			optimizationResult, err := optimizationEngine.OptimizeOrchestrationStrategies(ctx, workflow, learningHistory)

			// Business Requirement Validation: Continuous improvement capability
			Expect(err).ToNot(HaveOccurred(), "BR-ORCH-001: Continuous optimization must succeed")
			Expect(optimizationResult).ToNot(BeNil(), "BR-ORCH-001: Continuous optimization result required")

			// Verify system learns from execution feedback
			Expect(optimizationResult.LearningMetrics).ToNot(BeNil(),
				"BR-ORCH-001: Must track learning metrics for continuous improvement")
			Expect(optimizationResult.LearningMetrics.ImprovementTrend).To(BeNumerically(">", 0),
				"BR-ORCH-001: Learning must show positive improvement trend")

			// Verify learning metrics are available
			Expect(optimizationResult.LearningMetrics).ToNot(BeNil(),
				"BR-ORCH-001: Must track learning metrics for continuous improvement")

			// Verify recommended strategy is available
			Expect(optimizationResult.RecommendedStrategy).ToNot(BeNil(),
				"BR-ORCH-001: Must recommend best strategy based on learning")

			// Business Value: Adaptive optimization improves system performance over time
		})
	})

})
