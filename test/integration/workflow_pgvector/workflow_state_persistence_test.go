//go:build integration
// +build integration

package workflow_pgvector

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/persistence"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-WORKFLOW-PGVECTOR-001: Workflow State pgvector Persistence", Ordered, func() {
	var (
		hooks                    *testshared.TestLifecycleHooks
		ctx                      context.Context
		suite                    *testshared.StandardTestSuite
		workflowStatePersistence *persistence.WorkflowStatePgVectorPersistence
		logger                   *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure
		hooks = testshared.SetupAIIntegrationTest("Workflow State pgvector Persistence",
			testshared.WithRealVectorDB(), // Current milestone: pgvector only
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
		)
		hooks.Setup()

		suite = hooks.GetSuite()
		logger = suite.Logger
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		ctx = context.Background()

		// Validate test environment is healthy before each test
		Expect(suite.VectorDB).ToNot(BeNil(), "Vector database should be available")
		Expect(suite.WorkflowBuilder).ToNot(BeNil(), "Workflow builder should be available")

		// Create workflow state persistence manager for integration testing
		persistenceInterface := persistence.NewWorkflowStatePgVectorPersistence(suite.VectorDB, suite.WorkflowBuilder, logger)
		Expect(persistenceInterface).ToNot(BeNil(), "Workflow state persistence should be created successfully")

		// Type assert to concrete type for testing specific methods
		var ok bool
		workflowStatePersistence, ok = persistenceInterface.(*persistence.WorkflowStatePgVectorPersistence)
		Expect(ok).To(BeTrue(), "Should be able to type assert to concrete WorkflowStatePgVectorPersistence type")
	})

	Context("when persisting workflow state to pgvector", func() {
		It("should store workflow state as vectors and enable recovery", func() {
			By("creating a test workflow with complex state")
			testWorkflow := &persistence.RuntimeWorkflow{
				ID:          "workflow-state-persistence-001",
				Name:        "Critical CPU Alert Resolution Workflow",
				Description: "Multi-step workflow for resolving critical CPU alerts with rollback capability",
				Status:      persistence.WorkflowStatusRunning,
				Steps: []*persistence.RuntimeWorkflowStep{
					{
						ID:     "step-001-analyze",
						Name:   "Analyze CPU Usage",
						Status: persistence.StepStatusCompleted,
						Action: "analyze_metrics",
						Parameters: map[string]interface{}{
							"metric":    "cpu_usage",
							"threshold": 90.0,
							"timerange": "5m",
						},
						StartTime: time.Now().Add(-10 * time.Minute),
						EndTime:   &[]time.Time{time.Now().Add(-8 * time.Minute)}[0],
						Result:    map[string]interface{}{"cpu_average": 94.2, "recommendation": "scale_up"},
					},
					{
						ID:     "step-002-remediate",
						Name:   "Scale Up Deployment",
						Status: persistence.StepStatusRunning,
						Action: "scale_deployment",
						Parameters: map[string]interface{}{
							"deployment": "critical-app",
							"namespace":  "production",
							"replicas":   5,
						},
						StartTime: time.Now().Add(-5 * time.Minute),
					},
					{
						ID:     "step-003-validate",
						Name:   "Validate Resolution",
						Status: persistence.StepStatusPending,
						Action: "validate_metrics",
						Parameters: map[string]interface{}{
							"validation_criteria": "cpu_usage < 80",
							"timeout":             "10m",
						},
					},
				},
				Metadata: map[string]interface{}{
					"alert_id":           "alert-cpu-critical-001",
					"priority":           "high",
					"auto_rollback":      true,
					"checkpoint_enabled": true,
				},
			}

			By("persisting workflow state to pgvector")
			persistStartTime := time.Now()
			checkpoint, err := workflowStatePersistence.PersistWorkflowState(ctx, testWorkflow)
			persistTime := time.Since(persistStartTime)

			Expect(err).ToNot(HaveOccurred(), "Workflow state persistence should succeed")
			Expect(checkpoint).ToNot(BeNil(), "Should create a valid checkpoint")

			// BR-WORKFLOW-PGVECTOR-001: Persistence performance validation
			Expect(persistTime).To(BeNumerically("<", 3*time.Second), "BR-WORKFLOW-PGVECTOR-001: Persistence should be efficient")

			// Validate checkpoint contains essential information
			Expect(checkpoint.WorkflowID).To(Equal(testWorkflow.ID), "Checkpoint should preserve workflow ID")
			Expect(checkpoint.StateHash).ToNot(BeEmpty(), "Should generate state hash for integrity")
			Expect(checkpoint.CompressionRatio).To(BeNumerically("<=", 1.0), "BR-WORKFLOW-PGVECTOR-001: Workflow state compression ratio should be efficient")

			By("simulating workflow interruption and state loss")
			// Simulate system restart/failure by clearing in-memory state
			err = workflowStatePersistence.SimulateWorkflowInterruption(ctx, testWorkflow.ID)
			Expect(err).ToNot(HaveOccurred(), "Workflow interruption simulation should succeed")

			By("recovering workflow state from pgvector")
			recoveryStartTime := time.Now()
			recoveredWorkflow, err := workflowStatePersistence.RecoverWorkflowFromState(ctx, testWorkflow.ID, checkpoint.CheckpointID)
			recoveryTime := time.Since(recoveryStartTime)

			Expect(err).ToNot(HaveOccurred(), "Workflow state recovery should succeed")
			Expect(recoveredWorkflow).ToNot(BeNil(), "Should recover valid workflow")

			// BR-WORKFLOW-PGVECTOR-002: Recovery performance validation
			Expect(recoveryTime).To(BeNumerically("<", 5*time.Second), "BR-WORKFLOW-PGVECTOR-002: Recovery should be timely")

			By("validating recovered workflow state integrity")
			// Validate workflow structure
			Expect(recoveredWorkflow.ID).To(Equal(testWorkflow.ID), "Workflow ID should be preserved")
			Expect(recoveredWorkflow.Name).To(Equal(testWorkflow.Name), "Workflow name should be preserved")
			Expect(len(recoveredWorkflow.Steps)).To(Equal(len(testWorkflow.Steps)), "Should preserve all workflow steps")

			// Validate step states are preserved
			for i, step := range recoveredWorkflow.Steps {
				originalStep := testWorkflow.Steps[i]
				Expect(step.ID).To(Equal(originalStep.ID), fmt.Sprintf("Step %d ID should be preserved", i))
				Expect(step.Status).To(Equal(originalStep.Status), fmt.Sprintf("Step %d status should be preserved", i))
				Expect(step.Action).To(Equal(originalStep.Action), fmt.Sprintf("Step %d action should be preserved", i))
			}

			// BR-WORKFLOW-PGVECTOR-003: State integrity validation
			stateIntegrityScore := workflowStatePersistence.CalculateStateIntegrity(testWorkflow, recoveredWorkflow)
			Expect(stateIntegrityScore).To(BeNumerically(">=", 0.95), "BR-WORKFLOW-PGVECTOR-003: Should maintain high state integrity")
		})

		It("should handle workflow continuation from vector checkpoints", func() {
			By("creating workflow with multiple checkpoints")
			continuationWorkflow := &persistence.RuntimeWorkflow{
				ID:     "workflow-continuation-001",
				Name:   "Database Recovery with Multiple Checkpoints",
				Status: persistence.WorkflowStatusRunning,
				Steps: []*persistence.RuntimeWorkflowStep{
					{
						ID:     "checkpoint-step-001",
						Name:   "Initial Database Backup",
						Status: persistence.StepStatusCompleted,
						Action: "backup_database",
					},
					{
						ID:     "checkpoint-step-002",
						Name:   "Stop Database Services",
						Status: persistence.StepStatusCompleted,
						Action: "stop_services",
					},
					{
						ID:     "checkpoint-step-003",
						Name:   "Repair Database Files",
						Status: persistence.StepStatusRunning,
						Action: "repair_files",
					},
				},
			}

			By("creating multiple checkpoints during workflow execution")
			var checkpoints []*persistence.WorkflowCheckpoint

			// Create checkpoint after each completed step
			for i, step := range continuationWorkflow.Steps {
				if step.Status == persistence.StepStatusCompleted {
					checkpointName := fmt.Sprintf("checkpoint-after-step-%d", i+1)
					checkpoint, err := workflowStatePersistence.CreateNamedCheckpoint(ctx, continuationWorkflow, checkpointName)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Should create checkpoint %s", checkpointName))
					checkpoints = append(checkpoints, checkpoint)
				}
			}

			Expect(len(checkpoints)).To(BeNumerically(">=", 2), "Should have multiple checkpoints")

			By("continuing workflow from specific checkpoint")
			targetCheckpoint := checkpoints[1] // Continue from after step 2

			continuationStartTime := time.Now()
			continuedWorkflow, err := workflowStatePersistence.ContinueWorkflowFromCheckpoint(ctx, targetCheckpoint.CheckpointID)
			continuationTime := time.Since(continuationStartTime)

			Expect(err).ToNot(HaveOccurred(), "Workflow continuation should succeed")
			Expect(continuedWorkflow).ToNot(BeNil(), "Should get valid continued workflow")

			// BR-WORKFLOW-PGVECTOR-004: Continuation performance validation
			Expect(continuationTime).To(BeNumerically("<", 4*time.Second), "BR-WORKFLOW-PGVECTOR-004: Continuation should be efficient")

			By("validating workflow continues from correct state")
			// Should have steps 1-2 completed, step 3 ready to continue
			Expect(continuedWorkflow.Steps[0].Status).To(Equal(persistence.StepStatusCompleted), "Step 1 should remain completed")
			Expect(continuedWorkflow.Steps[1].Status).To(Equal(persistence.StepStatusCompleted), "Step 2 should remain completed")

			// Step 3 should be ready to continue (reset to pending if it was running)
			Expect(continuedWorkflow.Steps[2].Status).To(BeElementOf([]persistence.StepStatus{persistence.StepStatusPending, persistence.StepStatusRunning}), "Step 3 should be ready to continue")

			// BR-WORKFLOW-PGVECTOR-005: Continuation accuracy validation
			continuationAccuracy := workflowStatePersistence.ValidateContinuationAccuracy(ctx, targetCheckpoint, continuedWorkflow)
			Expect(continuationAccuracy).To(BeNumerically(">=", 0.9), "BR-WORKFLOW-PGVECTOR-005: Should have high continuation accuracy")
		})
	})

	Context("when handling workflow vector-based decision making", func() {
		It("should use vector similarity for dynamic workflow path selection", func() {
			By("creating workflow with dynamic decision points")
			adaptiveWorkflow := &persistence.RuntimeWorkflow{
				ID:   "adaptive-workflow-001",
				Name: "Adaptive Alert Resolution with Vector Decisions",
				Steps: []*persistence.RuntimeWorkflowStep{
					{
						ID:     "decision-step-001",
						Name:   "Analyze Alert Pattern",
						Status: persistence.StepStatusCompleted,
						Action: "analyze_pattern",
						Result: map[string]interface{}{
							"pattern_vector": generateTestVector(384),
							"alert_category": "memory_pressure",
							"confidence":     0.87,
						},
					},
				},
			}

			By("storing historical successful workflow patterns")
			historicalPatterns := []*persistence.WorkflowPattern{
				{
					ID:      "pattern-memory-scale-up",
					Name:    "Memory Pressure Scale Up Pattern",
					Vector:  generateTestVector(384),
					Success: true,
					Metadata: map[string]interface{}{
						"solution_type":       "scale_up",
						"success_rate":        0.92,
						"avg_resolution_time": "3m",
					},
				},
				{
					ID:      "pattern-memory-restart",
					Name:    "Memory Pressure Restart Pattern",
					Vector:  generateDifferentTestVector(384),
					Success: true,
					Metadata: map[string]interface{}{
						"solution_type":       "restart",
						"success_rate":        0.78,
						"avg_resolution_time": "5m",
					},
				},
			}

			for _, pattern := range historicalPatterns {
				err := workflowStatePersistence.StoreWorkflowPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(), "Should store historical patterns")
			}

			By("executing vector-based decision making")
			decisionStartTime := time.Now()
			decisionResult, err := workflowStatePersistence.MakeVectorBasedDecision(ctx, adaptiveWorkflow)
			decisionTime := time.Since(decisionStartTime)

			Expect(err).ToNot(HaveOccurred(), "Vector-based decision should succeed")
			Expect(decisionResult).ToNot(BeNil(), "Should provide decision result")

			// BR-WORKFLOW-PGVECTOR-006: Decision performance validation
			Expect(decisionTime).To(BeNumerically("<", 2*time.Second), "BR-WORKFLOW-PGVECTOR-006: Decision should be fast")

			// BR-WORKFLOW-PGVECTOR-007: Decision quality validation
			Expect(decisionResult.SelectedPattern).ToNot(BeEmpty(), "Should select a pattern")
			Expect(decisionResult.ConfidenceScore).To(BeNumerically(">=", 0.7), "BR-WORKFLOW-PGVECTOR-007: Should have reasonable confidence")
			Expect(decisionResult.SimilarityScore).To(BeNumerically(">=", 0.6), "BR-WORKFLOW-PGVECTOR-007: Should have meaningful similarity")

			By("validating decision leads to workflow optimization")
			optimizedWorkflow, err := workflowStatePersistence.ApplyVectorDecision(ctx, adaptiveWorkflow, decisionResult)
			Expect(err).ToNot(HaveOccurred(), "Should apply vector decision successfully")
			Expect(optimizedWorkflow).ToNot(BeNil(), "Should get optimized workflow")

			// Workflow should be enhanced with the selected pattern
			Expect(len(optimizedWorkflow.Steps)).To(BeNumerically(">", len(adaptiveWorkflow.Steps)), "Should add steps based on pattern")

			// BR-WORKFLOW-PGVECTOR-008: Optimization effectiveness validation
			optimizationMetrics := workflowStatePersistence.CalculateOptimizationMetrics(adaptiveWorkflow, optimizedWorkflow)
			Expect(optimizationMetrics.EfficiencyImprovement).To(BeNumerically(">=", 0.1), "BR-WORKFLOW-PGVECTOR-008: Should show efficiency improvement")
			Expect(optimizationMetrics.CostReduction).To(BeNumerically(">=", 0.05), "BR-WORKFLOW-PGVECTOR-008: Workflow optimization should achieve cost reduction")
		})

		It("should optimize workflow resource allocation using vector insights", func() {
			By("analyzing resource usage patterns with vector correlation")
			resourceOptimizationRequest := &persistence.WorkflowResourceOptimizationRequest{
				WorkflowID: "resource-optimization-workflow-001",
				CurrentResources: persistence.WorkflowResourceUsage{
					CPU:     "2000m",
					Memory:  "4Gi",
					Storage: "50Gi",
					Network: "1Gbps",
				},
				Constraints: persistence.ResourceConstraints{
					MaxCost:        10.0, // dollars per hour
					MaxLatency:     time.Second * 30,
					OptimizeFor:    "cost_efficiency", // Current milestone focus
					AccuracyTarget: 0.85,
				},
			}

			By("executing resource optimization with vector analysis")
			optimizationStartTime := time.Now()
			optimizationResult, err := workflowStatePersistence.OptimizeWorkflowResources(ctx, resourceOptimizationRequest)
			optimizationTime := time.Since(optimizationStartTime)

			Expect(err).ToNot(HaveOccurred(), "Resource optimization should succeed")
			Expect(optimizationResult).ToNot(BeNil(), "Should provide optimization result")

			// BR-WORKFLOW-PGVECTOR-009: Resource optimization performance validation
			Expect(optimizationTime).To(BeNumerically("<", 10*time.Second), "BR-WORKFLOW-PGVECTOR-009: Optimization should complete efficiently")

			By("validating cost-optimized resource allocation")
			// Current milestone focuses on cost over speed
			Expect(optimizationResult.OptimizedResources.EstimatedCostPerHour).To(BeNumerically("<=", resourceOptimizationRequest.Constraints.MaxCost), "Should respect cost constraints")
			Expect(optimizationResult.CostReduction).To(BeNumerically(">=", 0.05), "Should achieve meaningful cost reduction")

			// BR-WORKFLOW-PGVECTOR-010: Resource optimization accuracy validation
			Expect(optimizationResult.AccuracyMaintained).To(BeNumerically(">=", resourceOptimizationRequest.Constraints.AccuracyTarget), "BR-WORKFLOW-PGVECTOR-010: Should maintain accuracy target")
			Expect(optimizationResult.VectorInsightsUsed).To(BeTrue(), "BR-WORKFLOW-PGVECTOR-010: Should use vector insights for optimization")

			// Validate optimization insights are actionable
			Expect(len(optimizationResult.OptimizationInsights)).To(BeNumerically(">=", 1), "Should provide optimization insights")
			for _, insight := range optimizationResult.OptimizationInsights {
				Expect(insight.ActionableRecommendation).ToNot(BeEmpty(), "Each insight should have actionable recommendation")
				Expect(insight.ExpectedImpact).To(BeNumerically(">", 0), "Each insight should show expected impact")
			}
		})
	})
})

// Helper functions for test scenarios

// testshared.Ptr is a helper function to create a pointer to a time value
func Ptr[T any](v T) *T {
	return &v
}

// generateTestVector creates a test vector of the specified dimension
func generateTestVector(dimension int) []float32 {
	vector := make([]float32, dimension)
	for i := range vector {
		vector[i] = float32(i) * 0.01 // Simple test pattern
	}
	return vector
}

// generateDifferentTestVector creates a different test vector for comparison
func generateDifferentTestVector(dimension int) []float32 {
	vector := make([]float32, dimension)
	for i := range vector {
		vector[i] = float32(dimension-i) * 0.01 // Reverse pattern
	}
	return vector
}
