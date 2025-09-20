//go:build integration
// +build integration

package workflow_optimization

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-ORCH-002: Adaptive Resource Allocation Integration", Ordered, func() {
	var (
		hooks               *testshared.TestLifecycleHooks
		ctx                 context.Context
		suite               *testshared.StandardTestSuite
		realWorkflowBuilder engine.IntelligentWorkflowBuilder
		selfOptimizer       engine.SelfOptimizer
		resourceAllocator   engine.AdaptiveResourceAllocator // Business Contract: Need this interface
		logger              *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure with real components
		hooks = testshared.SetupAIIntegrationTest("Adaptive Resource Allocation Integration",
			testshared.WithRealVectorDB(), // Real pgvector integration for resource data storage
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
		Expect(suite.VectorDB).ToNot(BeNil(), "Vector database should be available for resource allocation")

		// Create real workflow builder with all dependencies (no mocks)
		realWorkflowBuilder = engine.NewIntelligentWorkflowBuilder(
			suite.LLMClient,        // Real LLM client for AI-driven workflow generation
			suite.VectorDB,         // Real vector database for pattern storage and retrieval
			suite.AnalyticsEngine,  // Real analytics engine from test suite
			suite.MetricsCollector, // Real AI metrics collector from test suite
			testshared.CreatePatternStoreForTesting(suite.Logger), // Real pattern store
			suite.ExecutionRepo, // Real execution repository from test suite
			suite.Logger,        // Real logger for operational visibility
		)
		Expect(realWorkflowBuilder).ToNot(BeNil())

		// Create self optimizer with real workflow builder integration
		selfOptimizer = engine.NewDefaultSelfOptimizer(
			realWorkflowBuilder, // Real component integration - no mocks
			engine.DefaultSelfOptimizerConfig(),
			logger,
		)
		Expect(selfOptimizer).ToNot(BeNil())

		// Create adaptive resource allocator - Business Contract: Real component needed
		resourceAllocator = createAdaptiveResourceAllocator(suite.VectorDB, suite.AnalyticsEngine, logger)
		Expect(resourceAllocator).ToNot(BeNil())
	})

	Context("when performing adaptive resource allocation with real monitoring", func() {
		It("should optimize resource allocation based on real workflow execution patterns", func() {
			// Business Requirement: BR-ORCH-002 - Adaptive Resource Allocation
			// Success Criteria: >20% resource efficiency improvement (BR-ORK-359)
			// Following guideline: Test business requirements, not implementation

			// Generate real workflow with resource-intensive steps
			resourceIntensiveWorkflow := generateResourceIntensiveWorkflow(ctx, realWorkflowBuilder)
			Expect(resourceIntensiveWorkflow).ToNot(BeNil())
			Expect(len(resourceIntensiveWorkflow.Template.Steps)).To(BeNumerically(">=", 3), "Resource-intensive workflow should have >= 3 steps")

			// Generate execution history with resource usage patterns
			executionHistory := generateResourcePatternHistory(ctx, realWorkflowBuilder, 50)
			Expect(executionHistory).To(HaveLen(50), "Should generate 50 execution history entries with resource patterns")

			// Measure baseline resource allocation before optimization
			// Business Contract: measureResourceAllocation method needed
			baselineResourceUsage := measureResourceAllocation(ctx, resourceIntensiveWorkflow, resourceAllocator)
			Expect(baselineResourceUsage.CPUUtilization).To(BeNumerically(">", 0), "Baseline CPU utilization should be measurable")
			Expect(baselineResourceUsage.MemoryUtilization).To(BeNumerically(">", 0), "Baseline memory utilization should be measurable")
			logger.WithFields(logrus.Fields{
				"baseline_cpu":    baselineResourceUsage.CPUUtilization,
				"baseline_memory": baselineResourceUsage.MemoryUtilization,
				"baseline_cost":   baselineResourceUsage.EstimatedCost,
			}).Info("Measured baseline resource allocation")

			// Perform adaptive resource allocation optimization
			// Business Contract: AdaptiveResourceAllocator.OptimizeResourceAllocation method
			optimizedAllocation, err := resourceAllocator.OptimizeResourceAllocation(ctx, resourceIntensiveWorkflow, executionHistory)
			Expect(err).ToNot(HaveOccurred(), "Resource allocation optimization should complete successfully")
			Expect(optimizedAllocation).ToNot(BeNil())

			// Business validation: Resource allocation should be improved
			Expect(optimizedAllocation.OptimizationApplied).To(BeTrue(), "Resource optimization should be applied")
			Expect(optimizedAllocation.EstimatedEfficiencyGain).To(BeNumerically(">", 0), "Should provide measurable efficiency gain")

			// Measure optimized resource allocation
			optimizedResourceUsage := measureResourceAllocation(ctx, resourceIntensiveWorkflow, resourceAllocator)
			logger.WithFields(logrus.Fields{
				"optimized_cpu":    optimizedResourceUsage.CPUUtilization,
				"optimized_memory": optimizedResourceUsage.MemoryUtilization,
				"optimized_cost":   optimizedResourceUsage.EstimatedCost,
			}).Info("Measured optimized resource allocation")

			// Business requirement validation: >20% resource efficiency improvement (BR-ORK-359)
			// Following guideline: Strong business assertions backed on business outcomes
			efficiencyImprovement := (baselineResourceUsage.EstimatedCost - optimizedResourceUsage.EstimatedCost) / baselineResourceUsage.EstimatedCost
			Expect(efficiencyImprovement).To(BeNumerically(">=", 0.20),
				"Adaptive resource allocation must achieve >20% efficiency improvement (BR-ORK-359)")

			// Additional business validation: Resource constraints respected
			Expect(optimizedResourceUsage.CPUUtilization).To(BeNumerically("<=", baselineResourceUsage.CPUUtilization),
				"Optimized CPU usage should not exceed baseline")
			Expect(optimizedResourceUsage.MemoryUtilization).To(BeNumerically("<=", baselineResourceUsage.MemoryUtilization*1.1),
				"Optimized memory usage should not significantly exceed baseline")
		})

		It("should adapt resource allocation to cluster capacity and constraints", func() {
			// Business Requirement: BR-ORCH-002 - Cluster-aware resource allocation
			// Following guideline: Test actual business requirement expectations

			// Create workflow that needs cluster capacity consideration
			clusterWorkflow := generateClusterSensitiveWorkflow(ctx, realWorkflowBuilder)
			Expect(clusterWorkflow).ToNot(BeNil())

			// Simulate different cluster capacity scenarios
			// Business Contract: ClusterCapacity type and methods needed
			highCapacityCluster := createClusterCapacityProfile(engine.ClusterCapacityHigh, 16, 64*1024) // 16 CPUs, 64GB RAM
			lowCapacityCluster := createClusterCapacityProfile(engine.ClusterCapacityLow, 4, 8*1024)     // 4 CPUs, 8GB RAM

			// Test resource allocation adaptation to high capacity cluster
			highCapacityAllocation, err := resourceAllocator.OptimizeForClusterCapacity(ctx, clusterWorkflow, highCapacityCluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(highCapacityAllocation).ToNot(BeNil())

			// Test resource allocation adaptation to low capacity cluster
			lowCapacityAllocation, err := resourceAllocator.OptimizeForClusterCapacity(ctx, clusterWorkflow, lowCapacityCluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(lowCapacityAllocation).ToNot(BeNil())

			// Business validation: Resource allocation should adapt to cluster capacity
			Expect(highCapacityAllocation.AllocatedCPU).To(BeNumerically(">", lowCapacityAllocation.AllocatedCPU),
				"High capacity cluster should allow higher CPU allocation")
			Expect(highCapacityAllocation.AllocatedMemory).To(BeNumerically(">", lowCapacityAllocation.AllocatedMemory),
				"High capacity cluster should allow higher memory allocation")

			// Validate resource constraints are respected
			Expect(lowCapacityAllocation.AllocatedCPU).To(BeNumerically("<=", lowCapacityCluster.AvailableCPU),
				"Low capacity allocation should respect cluster CPU limits")
			Expect(lowCapacityAllocation.AllocatedMemory).To(BeNumerically("<=", lowCapacityCluster.AvailableMemory),
				"Low capacity allocation should respect cluster memory limits")
		})

		It("should provide predictive resource allocation based on historical patterns", func() {
			// Business Requirement: BR-ORCH-002 - Predictive resource allocation
			// Following guideline: Strong business assertions

			testWorkflow := generateResourceVariableWorkflow(ctx, realWorkflowBuilder)
			Expect(testWorkflow).ToNot(BeNil())

			// Generate historical execution patterns with resource variation
			historicalPatterns := generateHistoricalResourcePatterns(ctx, realWorkflowBuilder, 100)
			Expect(historicalPatterns).To(HaveLen(100))

			// Perform predictive resource allocation
			// Business Contract: AdaptiveResourceAllocator.PredictResourceRequirements method
			prediction, err := resourceAllocator.PredictResourceRequirements(ctx, testWorkflow, historicalPatterns)
			Expect(err).ToNot(HaveOccurred())
			Expect(prediction).ToNot(BeNil())

			// Business validation: Predictions should be meaningful and accurate
			Expect(prediction.PredictedCPU).To(BeNumerically(">", 0),
				"CPU prediction should provide meaningful values")
			Expect(prediction.PredictedMemory).To(BeNumerically(">", 0),
				"Memory prediction should provide meaningful values")
			Expect(prediction.ConfidenceLevel).To(BeNumerically(">=", 0.7),
				"Resource predictions should have at least 70% confidence")

			// Validate prediction accuracy boundaries
			Expect(prediction.PredictionAccuracy).To(BeNumerically(">=", 0.8),
				"Predictive resource allocation should achieve >=80% accuracy")
		})
	})

	Context("when handling resource allocation edge cases with real monitoring", func() {
		It("should handle resource contention scenarios gracefully", func() {
			// Business Requirement: BR-ORCH-002 - Resource contention handling
			// Following guideline: Test business requirements, not implementation details

			contentionWorkflow := generateResourceContentionWorkflow(ctx, realWorkflowBuilder)

			// Simulate resource contention scenario
			contentionCluster := createClusterCapacityProfile(engine.ClusterCapacityConstrained, 2, 4*1024) // Very limited

			// Resource allocation should handle contention gracefully
			allocation, err := resourceAllocator.OptimizeForClusterCapacity(ctx, contentionWorkflow, contentionCluster)

			// Business expectation: Either succeed with warnings or fail with clear error
			if err != nil {
				// If error, it should be informative about resource constraints
				Expect(err.Error()).To(ContainSubstring("resource"),
					"Error should explain resource constraints")
			} else {
				// If success, should provide some allocation within limits
				Expect(allocation).ToNot(BeNil())
				Expect(allocation.AllocatedCPU).To(BeNumerically("<=", contentionCluster.AvailableCPU),
					"Allocation should respect cluster limits during contention")
			}
		})

		It("should maintain resource allocation efficiency under varying workloads", func() {
			// Business Requirement: BR-ORCH-002 - Workload variation adaptation
			// Following guideline: Strong business assertions

			varyingWorkflows := []engine.Workflow{
				*generateLightweightWorkflow(ctx, realWorkflowBuilder),
				*generateResourceIntensiveWorkflow(ctx, realWorkflowBuilder),
				*generateMemoryIntensiveWorkflow(ctx, realWorkflowBuilder),
			}

			allocationResults := make([]*engine.ResourceAllocationResult, len(varyingWorkflows))

			// Test allocation across different workload types
			for i, workflow := range varyingWorkflows {
				allocation, err := resourceAllocator.OptimizeResourceAllocation(ctx, &workflow, generateResourcePatternHistory(ctx, realWorkflowBuilder, 20))
				Expect(err).ToNot(HaveOccurred())
				allocationResults[i] = allocation
			}

			// Business validation: All workloads should receive appropriate allocations
			for i, result := range allocationResults {
				Expect(result.OptimizationApplied).To(BeTrue(),
					"Workload %d should receive optimized resource allocation", i)
				Expect(result.EstimatedEfficiencyGain).To(BeNumerically(">", 0),
					"Workload %d should show efficiency improvement", i)
			}
		})
	})
})

// Business Contract Helper Functions - These define the business contracts needed for compilation
// Following guideline: Define business contracts to enable tests to compile

func createAdaptiveResourceAllocator(vectorDB vector.VectorDatabase, analytics types.AnalyticsEngine, logger *logrus.Logger) engine.AdaptiveResourceAllocator {
	// Business Contract: Create AdaptiveResourceAllocator for real component integration
	panic("IMPLEMENTATION NEEDED: createAdaptiveResourceAllocator - Business Contract for adaptive resource allocator creation")
}

func generateResourceIntensiveWorkflow(ctx context.Context, builder engine.IntelligentWorkflowBuilder) *engine.Workflow {
	// Business Contract: Generate workflow with high resource requirements
	workflow := &engine.Workflow{
		Template: &engine.ExecutableTemplate{
			Steps: []*engine.ExecutableWorkflowStep{
				{Type: engine.StepTypeAction}, // CPU-intensive step
				{Type: engine.StepTypeAction}, // Memory-intensive step
				{Type: engine.StepTypeAction}, // IO-intensive step
			},
		},
		Status: engine.StatusPending,
	}
	workflow.ID = "test-resource-intensive-workflow-001"
	workflow.Name = "Resource Intensive Test Workflow"
	return workflow
}

func generateClusterSensitiveWorkflow(ctx context.Context, builder engine.IntelligentWorkflowBuilder) *engine.Workflow {
	// Business Contract: Generate workflow that varies based on cluster capacity
	workflow := &engine.Workflow{
		Template: &engine.ExecutableTemplate{
			Steps: []*engine.ExecutableWorkflowStep{
				{Type: engine.StepTypeParallel}, // Parallel execution sensitive to cluster capacity
				{Type: engine.StepTypeAction},   // Sequential processing
			},
		},
		Status: engine.StatusPending,
	}
	workflow.ID = "test-cluster-sensitive-workflow-001"
	workflow.Name = "Cluster Sensitive Test Workflow"
	return workflow
}

func generateResourceVariableWorkflow(ctx context.Context, builder engine.IntelligentWorkflowBuilder) *engine.Workflow {
	// Business Contract: Generate workflow with variable resource patterns
	workflow := &engine.Workflow{
		Template: &engine.ExecutableTemplate{
			Steps: []*engine.ExecutableWorkflowStep{
				{Type: engine.StepTypeAction}, // Variable resource usage
				{Type: engine.StepTypeAction}, // Predictable resource usage
			},
		},
		Status: engine.StatusPending,
	}
	workflow.ID = "test-resource-variable-workflow-001"
	workflow.Name = "Resource Variable Test Workflow"
	return workflow
}

func generateResourceContentionWorkflow(ctx context.Context, builder engine.IntelligentWorkflowBuilder) *engine.Workflow {
	// Business Contract: Generate workflow that creates resource contention
	workflow := &engine.Workflow{
		Template: &engine.ExecutableTemplate{
			Steps: []*engine.ExecutableWorkflowStep{
				{Type: engine.StepTypeAction}, // High contention step
			},
		},
		Status: engine.StatusPending,
	}
	workflow.ID = "test-resource-contention-workflow-001"
	workflow.Name = "Resource Contention Test Workflow"
	return workflow
}

func generateLightweightWorkflow(ctx context.Context, builder engine.IntelligentWorkflowBuilder) *engine.Workflow {
	// Business Contract: Generate workflow with minimal resource requirements
	workflow := &engine.Workflow{
		Template: &engine.ExecutableTemplate{
			Steps: []*engine.ExecutableWorkflowStep{
				{Type: engine.StepTypeAction}, // Lightweight step
			},
		},
		Status: engine.StatusPending,
	}
	workflow.ID = "test-lightweight-workflow-001"
	workflow.Name = "Lightweight Test Workflow"
	return workflow
}

func generateMemoryIntensiveWorkflow(ctx context.Context, builder engine.IntelligentWorkflowBuilder) *engine.Workflow {
	// Business Contract: Generate workflow with high memory requirements
	workflow := &engine.Workflow{
		Template: &engine.ExecutableTemplate{
			Steps: []*engine.ExecutableWorkflowStep{
				{Type: engine.StepTypeAction}, // Memory-intensive step
				{Type: engine.StepTypeAction}, // Memory-intensive step
			},
		},
		Status: engine.StatusPending,
	}
	workflow.ID = "test-memory-intensive-workflow-001"
	workflow.Name = "Memory Intensive Test Workflow"
	return workflow
}

func generateResourcePatternHistory(ctx context.Context, builder engine.IntelligentWorkflowBuilder, count int) []*engine.RuntimeWorkflowExecution {
	// Business Contract: Generate execution history with resource usage patterns
	history := make([]*engine.RuntimeWorkflowExecution, count)
	for i := 0; i < count; i++ {
		execution := &engine.RuntimeWorkflowExecution{
			OperationalStatus: engine.ExecutionStatusCompleted,
			Duration:          time.Duration(150+i*20) * time.Millisecond, // Varying execution times
			Steps: []*engine.StepExecution{
				{Status: engine.ExecutionStatusCompleted, Duration: time.Duration(75+i*10) * time.Millisecond},
				{Status: engine.ExecutionStatusCompleted, Duration: time.Duration(75+i*10) * time.Millisecond},
			},
		}
		execution.ID = fmt.Sprintf("test-resource-execution-%03d", i)
		history[i] = execution
	}
	return history
}

func generateHistoricalResourcePatterns(ctx context.Context, builder engine.IntelligentWorkflowBuilder, count int) []*engine.RuntimeWorkflowExecution {
	// Business Contract: Generate historical patterns for predictive analysis
	return generateResourcePatternHistory(ctx, builder, count) // Reuse with larger dataset
}

func measureResourceAllocation(ctx context.Context, workflow *engine.Workflow, allocator engine.AdaptiveResourceAllocator) *engine.AdaptiveResourceMetrics {
	// Business Contract: Measure current resource allocation for comparison
	panic("IMPLEMENTATION NEEDED: measureResourceAllocation - Business Contract for resource allocation measurement")
}

func createClusterCapacityProfile(capacity engine.ClusterCapacityLevel, cpu int, memory int) *engine.ClusterCapacity {
	// Business Contract: Create cluster capacity profile for testing
	panic("IMPLEMENTATION NEEDED: createClusterCapacityProfile - Business Contract for cluster capacity profile creation")
}
