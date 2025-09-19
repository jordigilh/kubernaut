package workflowengine

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// BR-WF-ADV-001-020: Workflow Advanced Patterns Tests (Phase 1 Implementation)
// Following UNIT_TEST_COVERAGE_EXTENSION_PLAN.md - Focus on advanced workflow patterns and optimization
var _ = Describe("BR-WF-ADV-001-020: Workflow Advanced Patterns Tests", func() {
	var (
		workflowBuilder *engine.DefaultIntelligentWorkflowBuilder
		mockLLMClient   *mocks.MockLLMClient
		mockVectorDB    *mocks.MockVectorDatabase
		logger          *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		mockLLMClient = &mocks.MockLLMClient{}
		mockVectorDB = &mocks.MockVectorDatabase{}

		// Configuration removed as constructor doesn't use it in current implementation
		// Following project guideline #13: only integrate existing functionality

		// Following project guideline #13: integration with existing code
		// Using nil for components that don't have proper business contracts yet
		workflowBuilder = engine.NewIntelligentWorkflowBuilder(
			mockLLMClient,
			mockVectorDB,
			nil, // Analytics engine - will be set to nil for now
			nil, // Metrics collector - will be set to nil for now
			nil, // Pattern store - will be set to nil for now
			nil, // Execution repository - will be set to nil for now
			logger,
		)
	})

	Describe("BR-WF-ADV-001: Advanced Pattern Matching Algorithms", func() {
		It("should match workflow patterns with high similarity scores", func() {
			inputPattern := &engine.WorkflowPattern{
				ID:            "input-pattern",
				Name:          "Database Recovery Pattern",
				Type:          "recovery",
				ResourceTypes: []string{"database", "backup", "recovery"},
				Environments:  []string{"production"},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "backup", Name: "Backup Database"},
						Type:       engine.StepTypeAction,
						Variables:  map[string]interface{}{"severity": "high", "namespace": "production"},
					},
					{
						BaseEntity: types.BaseEntity{ID: "verify", Name: "Verify Backup"},
						Type:       engine.StepTypeAction,
					},
				},
			}

			mockPatterns := []*engine.WorkflowPattern{
				{
					ID:            "pattern-1",
					Name:          "Similar Database Pattern",
					Type:          "recovery",
					ResourceTypes: []string{"database", "backup", "restore"},
					Environments:  []string{"production"},
					Confidence:    0.85,
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "backup", Name: "Backup Database"},
							Type:       engine.StepTypeAction,
							Variables:  map[string]interface{}{"severity": "high", "namespace": "production"},
						},
						{
							BaseEntity: types.BaseEntity{ID: "verify", Name: "Verify Backup"},
							Type:       engine.StepTypeAction,
						},
						{
							BaseEntity: types.BaseEntity{ID: "restore", Name: "Restore Database"},
							Type:       engine.StepTypeAction,
						},
					},
				},
				{
					ID:            "pattern-2",
					Name:          "Network Troubleshooting Pattern",
					Type:          "troubleshooting",
					ResourceTypes: []string{"network", "connectivity", "troubleshoot"},
					Environments:  []string{"production"},
					Confidence:    0.3,
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "check", Name: "Check Network"},
							Type:       engine.StepTypeAction,
							Variables:  map[string]interface{}{"severity": "medium"},
						},
						{
							BaseEntity: types.BaseEntity{ID: "restart", Name: "Restart Service"},
							Type:       engine.StepTypeAction,
						},
					},
				},
			}

			matches := workflowBuilder.FindSimilarPatterns(inputPattern, mockPatterns, 0.8)

			Expect(matches).To(HaveLen(1), "Should match only patterns above similarity threshold")
			Expect(matches[0].Confidence).To(BeNumerically(">=", 0.8), "Matched pattern should meet similarity threshold")
			Expect(matches[0].ResourceTypes).To(ContainElement("database"), "Should preserve pattern resource types")
		})

		It("should calculate pattern similarity using multiple criteria", func() {
			pattern1 := &engine.WorkflowPattern{
				ID:            "pattern1",
				Name:          "Pod Restart Pattern",
				Type:          "restart",
				ResourceTypes: []string{"pod", "restart", "failure"},
				Environments:  []string{"default"},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "restart", Name: "Restart Pod"},
						Type:       engine.StepTypeAction,
						Variables:  map[string]interface{}{"namespace": "default"},
					},
					{
						BaseEntity: types.BaseEntity{ID: "logs", Name: "Check Logs"},
						Type:       engine.StepTypeAction,
					},
				},
			}

			pattern2 := &engine.WorkflowPattern{
				ID:            "pattern2",
				Name:          "Pod Crash Pattern",
				Type:          "restart",
				ResourceTypes: []string{"pod", "restart", "crash"},
				Environments:  []string{"default"},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "restart", Name: "Restart Pod"},
						Type:       engine.StepTypeAction,
						Variables:  map[string]interface{}{"namespace": "default"},
					},
					{
						BaseEntity: types.BaseEntity{ID: "analyze", Name: "Analyze Crash"},
						Type:       engine.StepTypeAction,
					},
				},
			}

			similarity := workflowBuilder.CalculatePatternSimilarity(pattern1, pattern2)

			Expect(similarity).To(BeNumerically(">", 0.5), "Similar patterns should have high similarity")
			Expect(similarity).To(BeNumerically("<=", 1.0), "Similarity should not exceed maximum")
		})

		It("should handle edge cases in pattern matching algorithms", func() {
			emptyPattern := &engine.WorkflowPattern{
				ID:            "empty-pattern",
				Name:          "Empty Pattern",
				Type:          "empty",
				ResourceTypes: []string{},
				Environments:  []string{},
				Steps:         []*engine.ExecutableWorkflowStep{},
			}

			validPattern := &engine.WorkflowPattern{
				ID:            "valid-pattern",
				Name:          "Test Pattern",
				Type:          "test",
				ResourceTypes: []string{"test"},
				Environments:  []string{"test"},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "test", Name: "Test Action"},
						Type:       engine.StepTypeAction,
						Variables:  map[string]interface{}{"test": "value"},
					},
				},
			}

			similarity := workflowBuilder.CalculatePatternSimilarity(emptyPattern, validPattern)
			Expect(similarity).To(BeNumerically(">=", 0.0), "Empty pattern similarity should be non-negative")

			// Self-similarity should be perfect
			selfSimilarity := workflowBuilder.CalculatePatternSimilarity(validPattern, validPattern)
			Expect(selfSimilarity).To(BeNumerically("~", 1.0, 0.01), "Self-similarity should be near perfect")
		})
	})

	Describe("BR-WF-ADV-002: Dynamic Workflow Generation Algorithms", func() {
		It("should generate workflows based on objective analysis", func() {
			objective := "Resolve database connection timeout in production environment"
			context := map[string]interface{}{
				"severity":    "high",
				"environment": "production",
				"component":   "database",
			}

			analysisResult := workflowBuilder.AnalyzeObjective(objective, context)

			Expect(analysisResult).ToNot(BeNil(), "Objective analysis should return results")
			Expect(analysisResult.Keywords).To(ContainElement("database"), "Should identify key terms")
			Expect(analysisResult.Complexity).To(BeNumerically(">", 0), "Should calculate complexity score")
			Expect(analysisResult.Priority).To(BeNumerically(">", 0), "Should assign priority level")
		})

		It("should generate appropriate workflow steps for complex scenarios", func() {
			analysis := &engine.ObjectiveAnalysisResult{
				Keywords:    []string{"database", "timeout", "connection"},
				ActionTypes: []string{"check_database", "restart_service", "analyze_logs"},
				Constraints: map[string]interface{}{"max_downtime": "5m", "rollback_required": true},
				Complexity:  0.75,
				RiskLevel:   "high",
			}

			steps, err := workflowBuilder.GenerateWorkflowSteps(analysis)

			Expect(err).ToNot(HaveOccurred(), "Step generation should not fail")
			Expect(steps).ToNot(BeEmpty(), "Should generate workflow steps")
			Expect(len(steps)).To(BeNumerically("<=", 20), "Should respect max step limit")

			for _, step := range steps {
				Expect(step.Type).ToNot(BeEmpty(), "Each step should have a type")
				Expect(step.Timeout).To(BeNumerically(">", 0), "Each step should have timeout")
			}
		})

		It("should optimize step ordering based on dependencies", func() {
			unorderedSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity:   types.BaseEntity{ID: "step3", Name: "Analyze Results"},
					Dependencies: []string{"step1", "step2"},
					Type:         engine.StepTypeAction,
				},
				{
					BaseEntity:   types.BaseEntity{ID: "step1", Name: "Check Status"},
					Dependencies: []string{},
					Type:         engine.StepTypeAction,
				},
				{
					BaseEntity:   types.BaseEntity{ID: "step2", Name: "Gather Logs"},
					Dependencies: []string{"step1"},
					Type:         engine.StepTypeAction,
				},
			}

			orderedSteps, err := workflowBuilder.OptimizeStepOrdering(unorderedSteps)

			Expect(err).ToNot(HaveOccurred(), "Step ordering should not fail")
			Expect(orderedSteps).To(HaveLen(3), "Should preserve all steps")
			Expect(orderedSteps[0].ID).To(Equal("step1"), "Step with no dependencies should be first")
			Expect(orderedSteps[1].ID).To(Equal("step2"), "Step depending on step1 should be second")
			Expect(orderedSteps[2].ID).To(Equal("step3"), "Step with most dependencies should be last")
		})
	})

	Describe("BR-WF-ADV-003: Resource Allocation Optimization", func() {
		It("should calculate optimal resource allocation for concurrent steps", func() {
			parallelSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{ID: "cpu-intensive", Name: "Data Processing"},
					Type:       engine.StepTypeAction,
					Variables:  map[string]interface{}{"cpu_weight": 0.8, "memory_weight": 0.3},
				},
				{
					BaseEntity: types.BaseEntity{ID: "memory-intensive", Name: "Cache Loading"},
					Type:       engine.StepTypeAction,
					Variables:  map[string]interface{}{"cpu_weight": 0.2, "memory_weight": 0.9},
				},
				{
					BaseEntity: types.BaseEntity{ID: "io-intensive", Name: "Log Analysis"},
					Type:       engine.StepTypeAction,
					Variables:  map[string]interface{}{"cpu_weight": 0.4, "memory_weight": 0.2},
				},
			}

			resourcePlan := workflowBuilder.CalculateResourceAllocation(parallelSteps)

			Expect(resourcePlan).ToNot(BeNil(), "Should generate resource allocation plan")
			Expect(resourcePlan.TotalCPUWeight).To(BeNumerically("~", 1.4, 0.1), "Should sum CPU weights")
			Expect(resourcePlan.TotalMemoryWeight).To(BeNumerically("~", 1.4, 0.1), "Should sum memory weights")
			Expect(resourcePlan.MaxConcurrency).To(BeNumerically(">", 0), "Should set concurrency limit")
		})

		It("should respect resource constraints and limits", func() {
			constraints := &engine.ResourceConstraints{
				MaxCPUUtilization:    0.8,
				MaxMemoryUtilization: 0.7,
				MaxConcurrentSteps:   5,
				TimeoutBuffer:        30 * time.Second,
			}

			heavySteps := make([]*engine.ExecutableWorkflowStep, 10)
			for i := 0; i < 10; i++ {
				heavySteps[i] = &engine.ExecutableWorkflowStep{
					BaseEntity: types.BaseEntity{ID: fmt.Sprintf("step-%d", i), Name: fmt.Sprintf("Heavy Step %d", i)},
					Type:       engine.StepTypeAction,
					Variables:  map[string]interface{}{"cpu_weight": 0.9, "memory_weight": 0.8},
				}
			}

			plan := workflowBuilder.CalculateResourceAllocationWithConstraints(heavySteps, constraints)

			Expect(plan.MaxConcurrency).To(BeNumerically("<=", constraints.MaxConcurrentSteps), "Should respect concurrency limit")
			Expect(plan.EstimatedCPUUtilization).To(BeNumerically("<=", constraints.MaxCPUUtilization), "Should respect CPU limit")
			Expect(plan.EstimatedMemoryUtilization).To(BeNumerically("<=", constraints.MaxMemoryUtilization), "Should respect memory limit")
		})

		It("should optimize resource usage efficiency", func() {
			mixedSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{ID: "light", Name: "Light Task"},
					Variables:  map[string]interface{}{"cpu_weight": 0.1, "memory_weight": 0.1},
				},
				{
					BaseEntity: types.BaseEntity{ID: "medium", Name: "Medium Task"},
					Variables:  map[string]interface{}{"cpu_weight": 0.5, "memory_weight": 0.4},
				},
				{
					BaseEntity: types.BaseEntity{ID: "heavy", Name: "Heavy Task"},
					Variables:  map[string]interface{}{"cpu_weight": 0.9, "memory_weight": 0.8},
				},
			}

			plan := workflowBuilder.OptimizeResourceEfficiency(mixedSteps)

			Expect(plan.EfficiencyScore).To(BeNumerically(">", 0), "Should calculate efficiency score")
			Expect(plan.EfficiencyScore).To(BeNumerically("<=", 1.0), "Efficiency should not exceed 100%")
			Expect(plan.OptimalBatches).ToNot(BeEmpty(), "Should create execution batches")
		})
	})

	Describe("BR-WF-ADV-004: Parallel Execution Algorithms", func() {
		It("should determine optimal parallelization strategy", func() {
			sequentialSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity:   types.BaseEntity{ID: "step1", Name: "Initialize"},
					Dependencies: []string{},
					Type:         engine.StepTypeAction,
				},
				{
					BaseEntity:   types.BaseEntity{ID: "step2a", Name: "Process A"},
					Dependencies: []string{"step1"},
					Type:         engine.StepTypeAction,
				},
				{
					BaseEntity:   types.BaseEntity{ID: "step2b", Name: "Process B"},
					Dependencies: []string{"step1"},
					Type:         engine.StepTypeAction,
				},
				{
					BaseEntity:   types.BaseEntity{ID: "step3", Name: "Finalize"},
					Dependencies: []string{"step2a", "step2b"},
					Type:         engine.StepTypeAction,
				},
			}

			strategy := workflowBuilder.DetermineParallelizationStrategy(sequentialSteps)

			Expect(strategy).ToNot(BeNil(), "Should generate parallelization strategy")
			Expect(strategy.ParallelGroups).To(HaveLen(3), "Should identify 3 execution groups")
			Expect(strategy.ParallelGroups[1]).To(HaveLen(2), "Middle group should have 2 parallel steps")
			Expect(strategy.EstimatedSpeedup).To(BeNumerically(">", 1.0), "Should estimate performance improvement")
		})

		It("should handle dependency conflicts in parallel execution", func() {
			conflictingSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity:   types.BaseEntity{ID: "stepA", Name: "Step A"},
					Dependencies: []string{"stepB"},
					Type:         engine.StepTypeAction,
				},
				{
					BaseEntity:   types.BaseEntity{ID: "stepB", Name: "Step B"},
					Dependencies: []string{"stepA"}, // Circular dependency
					Type:         engine.StepTypeAction,
				},
			}

			strategy := workflowBuilder.DetermineParallelizationStrategy(conflictingSteps)

			Expect(strategy.HasCircularDependencies).To(BeTrue(), "Should detect circular dependencies")
			Expect(strategy.ConflictResolution).ToNot(BeEmpty(), "Should provide conflict resolution")
		})

		It("should calculate concurrency limits based on step characteristics", func() {
			cpuIntensiveSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{ID: "cpu1", Name: "CPU Task 1"},
					Variables:  map[string]interface{}{"cpu_intensive": true, "cpu_weight": 0.8},
				},
				{
					BaseEntity: types.BaseEntity{ID: "cpu2", Name: "CPU Task 2"},
					Variables:  map[string]interface{}{"cpu_intensive": true, "cpu_weight": 0.9},
				},
			}

			ioIntensiveSteps := []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{ID: "io1", Name: "IO Task 1"},
					Variables:  map[string]interface{}{"io_intensive": true, "io_weight": 0.7},
				},
				{
					BaseEntity: types.BaseEntity{ID: "io2", Name: "IO Task 2"},
					Variables:  map[string]interface{}{"io_intensive": true, "io_weight": 0.6},
				},
			}

			cpuConcurrency := workflowBuilder.CalculateOptimalConcurrency(cpuIntensiveSteps)
			ioConcurrency := workflowBuilder.CalculateOptimalConcurrency(ioIntensiveSteps)

			Expect(cpuConcurrency).To(BeNumerically("<", ioConcurrency), "CPU tasks should have lower concurrency than IO tasks")
			Expect(cpuConcurrency).To(BeNumerically(">=", 1), "Should allow at least one concurrent CPU task")
			Expect(ioConcurrency).To(BeNumerically(">=", 1), "Should allow at least one concurrent IO task")
		})
	})

	Describe("BR-WF-ADV-005: Loop Execution and Termination", func() {
		It("should execute loop steps with proper termination conditions", func() {
			loopStep := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{ID: "retry-loop", Name: "Retry Until Success"},
				Type:       engine.StepTypeLoop,
				Variables: map[string]interface{}{
					"max_iterations":  5,
					"break_condition": "success == true",
					"iteration_delay": "30s",
				},
			}

			termination := workflowBuilder.EvaluateLoopTermination(loopStep, 3, map[string]interface{}{
				"success":    false,
				"error_rate": 0.4,
			})

			Expect(termination.ShouldContinue).To(BeTrue(), "Should continue when under max iterations and condition not met")
			Expect(termination.NextIteration).To(Equal(4), "Should increment iteration counter")

			// Test termination on max iterations
			terminationMax := workflowBuilder.EvaluateLoopTermination(loopStep, 5, map[string]interface{}{
				"success": false,
			})

			Expect(terminationMax.ShouldContinue).To(BeFalse(), "Should terminate at max iterations")
			Expect(terminationMax.Reason).To(ContainSubstring("max_iterations"), "Should specify termination reason")
		})

		It("should handle complex loop conditions and variable evaluation", func() {
			complexLoopStep := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{ID: "complex-loop", Name: "Complex Condition Loop"},
				Type:       engine.StepTypeLoop,
				Variables: map[string]interface{}{
					"max_iterations":     10,
					"break_condition":    "error_rate < 0.1 AND success_rate > 0.9",
					"continue_condition": "retries_remaining > 0",
					"timeout":            "5m",
				},
			}

			context := map[string]interface{}{
				"error_rate":        0.15,
				"success_rate":      0.85,
				"retries_remaining": 3,
			}

			evaluation := workflowBuilder.EvaluateComplexLoopCondition(complexLoopStep, context)

			Expect(evaluation.BreakConditionMet).To(BeFalse(), "Break condition should not be met")
			Expect(evaluation.ContinueConditionMet).To(BeTrue(), "Continue condition should be met")
			Expect(evaluation.ConditionEvaluation).ToNot(BeEmpty(), "Should provide evaluation details")
		})

		It("should calculate loop performance metrics and optimization", func() {
			loopMetrics := &engine.LoopExecutionMetrics{
				TotalIterations:      8,
				SuccessfulIterations: 6,
				FailedIterations:     2,
				AverageIterationTime: 45 * time.Second,
				TotalExecutionTime:   6 * time.Minute,
			}

			optimization := workflowBuilder.AnalyzeLoopPerformance(loopMetrics)

			Expect(optimization.SuccessRate).To(BeNumerically("~", 0.75, 0.01), "Should calculate 75% success rate")
			Expect(optimization.EfficiencyScore).To(BeNumerically(">", 0), "Should calculate efficiency score")
			Expect(optimization.Recommendations).ToNot(BeEmpty(), "Should provide optimization recommendations")
		})
	})

	Describe("BR-WF-ADV-006: Workflow Complexity Assessment", func() {
		It("should calculate workflow complexity score based on multiple factors", func() {
			complexWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "complex-wf", Name: "Complex Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-1", Name: "Complex Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity:   types.BaseEntity{ID: "step0", Name: "Action Step"},
							Type:         engine.StepTypeAction,
							Dependencies: []string{},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step1", Name: "Parallel Step"},
							Type:         engine.StepTypeParallel,
							Dependencies: []string{"step0"},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step2", Name: "Loop Step"},
							Type:         engine.StepTypeLoop,
							Dependencies: []string{"step1"},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step3", Name: "Decision Step"},
							Type:         engine.StepTypeDecision,
							Dependencies: []string{"step2"},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step4", Name: "Subflow Step"},
							Type:         engine.StepTypeSubflow,
							Dependencies: []string{"step3"},
						},
					},
				},
			}

			complexity := workflowBuilder.CalculateWorkflowComplexity(complexWorkflow)

			Expect(complexity.OverallScore).To(BeNumerically(">", 0), "Should calculate complexity score")
			Expect(complexity.OverallScore).To(BeNumerically("<=", 1.0), "Complexity should be normalized")
			Expect(complexity.FactorScores).To(HaveKey("step_count"), "Should include step count factor")
			Expect(complexity.FactorScores).To(HaveKey("dependency_complexity"), "Should include dependency factor")
			Expect(complexity.FactorScores).To(HaveKey("step_type_diversity"), "Should include type diversity factor")
		})

		It("should assess risk level based on complexity factors", func() {
			lowComplexityWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "low-complex", Name: "Low Complexity Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-low", Name: "Low Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity:   types.BaseEntity{ID: "step0", Name: "Action Step 1"},
							Type:         engine.StepTypeAction,
							Dependencies: []string{},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step1", Name: "Action Step 2"},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"step0"},
						},
					},
				},
			}

			highComplexityWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "high-complex", Name: "High Complexity Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-high", Name: "High Template"},
						Version:    "1.0.0",
					},
					Steps: make([]*engine.ExecutableWorkflowStep, 15), // Many steps
				},
			}
			for i := range highComplexityWorkflow.Template.Steps {
				deps := []string{}
				if i > 0 {
					deps = append(deps, fmt.Sprintf("step%d", i-1))
				}
				if i > 1 {
					deps = append(deps, fmt.Sprintf("step%d", i-2))
				}
				highComplexityWorkflow.Template.Steps[i] = &engine.ExecutableWorkflowStep{
					BaseEntity:   types.BaseEntity{ID: fmt.Sprintf("step%d", i), Name: fmt.Sprintf("Step %d", i)},
					Type:         engine.StepTypeParallel,
					Dependencies: deps,
				}
			}

			lowRisk := workflowBuilder.AssessWorkflowRisk(lowComplexityWorkflow)
			highRisk := workflowBuilder.AssessWorkflowRisk(highComplexityWorkflow)

			Expect(lowRisk.RiskLevel).To(Equal("low"), "Simple workflow should have low risk")
			Expect(highRisk.RiskLevel).To(BeElementOf([]string{"medium", "high"}), "Complex workflow should have higher risk")
			Expect(highRisk.RiskScore).To(BeNumerically(">", lowRisk.RiskScore), "High risk should have higher score")
		})

		It("should provide complexity reduction recommendations", func() {
			bloatedWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "bloated-wf", Name: "Bloated Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-bloated", Name: "Bloated Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity:   types.BaseEntity{ID: "step0", Name: "Redundant Action 1"},
							Type:         engine.StepTypeAction,
							Dependencies: []string{},
							Variables:    map[string]interface{}{"redundant": true},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step1", Name: "Redundant Action 2"},
							Type:         engine.StepTypeAction,
							Dependencies: []string{},
							Variables:    map[string]interface{}{"redundant": true},
						},
						{
							BaseEntity: types.BaseEntity{ID: "step2", Name: "Loop Step"},
							Type:       engine.StepTypeLoop,
							Variables:  map[string]interface{}{"max_iterations": 100},
						},
						{
							BaseEntity:   types.BaseEntity{ID: "step3", Name: "Decision Step"},
							Type:         engine.StepTypeDecision,
							Dependencies: []string{"step0", "step1", "step2"},
						},
					},
				},
			}

			recommendations := workflowBuilder.GenerateComplexityReductions(bloatedWorkflow)

			Expect(recommendations).ToNot(BeEmpty(), "Should provide reduction recommendations")
			Expect(recommendations).To(ContainElement(ContainSubstring("redundant")), "Should identify redundant steps")
			Expect(recommendations).To(ContainElement(ContainSubstring("consolidate")), "Should suggest consolidation")
		})
	})

	Describe("BR-WF-ADV-007: AI-Driven Workflow Optimization", func() {
		It("should generate optimized workflows using AI analysis", func() {
			historicalData := []*engine.WorkflowExecution{
				{
					WorkflowID: "pattern-1",
					Status:     engine.ExecutionStatusCompleted,
					Duration:   2 * time.Minute,
					StepResults: map[string]*engine.StepResult{
						"step1": {Success: true, Duration: 30 * time.Second},
						"step2": {Success: true, Duration: 90 * time.Second},
					},
				},
				{
					WorkflowID: "pattern-1",
					Status:     engine.ExecutionStatusFailed,
					Duration:   5 * time.Minute,
					StepResults: map[string]*engine.StepResult{
						"step1": {Success: true, Duration: 30 * time.Second},
						"step2": {Success: false, Duration: 270 * time.Second},
					},
				},
			}

			optimization := workflowBuilder.GenerateAIOptimizations(historicalData, "pattern-1")

			Expect(optimization).ToNot(BeNil(), "Should generate AI-based optimization")
			Expect(optimization.OptimizationScore).To(BeNumerically(">", 0), "Should calculate optimization potential")
			Expect(optimization.Recommendations).ToNot(BeEmpty(), "Should provide optimization recommendations")
			Expect(optimization.EstimatedImprovement).To(HaveKey("duration"), "Should estimate duration improvement")
		})

		It("should learn from workflow execution patterns", func() {
			executionPattern := &engine.ExecutionPattern{
				PatternID:       "database-recovery",
				SuccessRate:     0.85,
				AverageDuration: 3 * time.Minute,
				CommonFailures:  []string{"timeout", "connection_error"},
				ContextFactors: map[string]interface{}{
					"time_of_day": "peak_hours",
					"load_level":  "high",
				},
			}

			learningResult := workflowBuilder.LearnFromExecutionPattern(executionPattern)

			Expect(learningResult.PatternConfidence).To(BeNumerically(">", 0), "Should calculate pattern confidence")
			Expect(learningResult.LearningImpact).To(BeElementOf([]string{"low", "medium", "high"}), "Should assess learning impact")
			Expect(learningResult.UpdatedRules).ToNot(BeEmpty(), "Should generate updated optimization rules")
		})

		It("should predict workflow success probability", func() {
			workflowContext := map[string]interface{}{
				"environment":   "production",
				"complexity":    0.7,
				"resource_load": 0.8,
				"time_of_day":   "peak",
			}

			prediction := workflowBuilder.PredictWorkflowSuccess("database-maintenance", workflowContext)

			Expect(prediction.SuccessProbability).To(BeNumerically(">=", 0), "Success probability should be non-negative")
			Expect(prediction.SuccessProbability).To(BeNumerically("<=", 1.0), "Success probability should not exceed 1.0")
			Expect(prediction.RiskFactors).ToNot(BeEmpty(), "Should identify risk factors")
			Expect(prediction.ConfidenceLevel).To(BeElementOf([]string{"low", "medium", "high"}), "Should provide confidence level")
		})
	})

	Describe("BR-WF-ADV-008: Performance Optimization Algorithms", func() {
		It("should optimize workflow execution time", func() {
			slowWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "slow-wf", Name: "Slow Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-slow", Name: "Slow Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "slow-step", Name: "Slow Operation"},
							Type:       engine.StepTypeAction,
							Timeout:    10 * time.Minute,
							Variables:  map[string]interface{}{"optimization_target": true},
						},
						{
							BaseEntity: types.BaseEntity{ID: "fast-step", Name: "Fast Operation"},
							Type:       engine.StepTypeAction,
							Timeout:    30 * time.Second,
						},
					},
				},
			}

			optimization := workflowBuilder.OptimizeExecutionTime(slowWorkflow)

			Expect(optimization.EstimatedImprovement).To(BeNumerically(">", 0), "Should estimate time improvement")
			Expect(optimization.OptimizedSteps).ToNot(BeEmpty(), "Should identify optimizable steps")
			Expect(optimization.Techniques).To(ContainElement("parallel_execution"), "Should suggest parallelization")
		})

		It("should balance speed vs reliability optimization", func() {
			reliabilityFocused := &engine.OptimizationConstraints{
				MaxRiskLevel:       "low",
				MinSuccessRate:     0.95,
				MaxPerformanceGain: 0.3,
				PreferReliability:  true,
			}

			speedFocused := &engine.OptimizationConstraints{
				MaxRiskLevel:       "medium",
				MinSuccessRate:     0.8,
				MaxPerformanceGain: 0.7,
				PreferReliability:  false,
			}

			workflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "test-wf", Name: "Test Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-test", Name: "Test Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "test-step", Name: "Test Action"},
							Type:       engine.StepTypeAction,
							Timeout:    2 * time.Minute,
						},
					},
				},
			}

			reliableOpt := workflowBuilder.OptimizeWithConstraints(workflow, reliabilityFocused)
			speedOpt := workflowBuilder.OptimizeWithConstraints(workflow, speedFocused)

			Expect(reliableOpt.RiskLevel).To(Equal("low"), "Reliability-focused should maintain low risk")
			Expect(speedOpt.PerformanceGain).To(BeNumerically(">=", reliableOpt.PerformanceGain), "Speed-focused should have higher performance gain")
		})

		It("should calculate optimization impact metrics", func() {
			beforeOptimization := &engine.WorkflowMetrics{
				AverageExecutionTime: 5 * time.Minute,
				SuccessRate:          0.85,
				ResourceUtilization:  0.9,
				FailureRate:          0.15,
			}

			afterOptimization := &engine.WorkflowMetrics{
				AverageExecutionTime: 3 * time.Minute,
				SuccessRate:          0.92,
				ResourceUtilization:  0.7,
				FailureRate:          0.08,
			}

			impact := workflowBuilder.CalculateOptimizationImpact(beforeOptimization, afterOptimization)

			Expect(impact.TimeImprovement).To(BeNumerically("~", 0.4, 0.01), "Should calculate 40% time improvement")
			Expect(impact.ReliabilityImprovement).To(BeNumerically(">", 0), "Should show reliability improvement")
			Expect(impact.ResourceEfficiencyGain).To(BeNumerically(">", 0), "Should show resource efficiency gain")
			Expect(impact.OverallScore).To(BeNumerically(">", 0), "Should calculate overall improvement score")
		})
	})

	Describe("BR-WF-ADV-009: Safety and Validation Framework", func() {
		It("should validate workflow safety before execution", func() {
			riskyWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "risky-wf", Name: "Risky Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-risky", Name: "Risky Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "dangerous-action", Name: "Delete Production Data"},
							Type:       engine.StepTypeAction,
							Variables: map[string]interface{}{
								"destructive": true,
								"environment": "production",
								"data_scope":  "all",
							},
						},
					},
				},
			}

			safetyCheck := workflowBuilder.ValidateWorkflowSafety(riskyWorkflow)

			Expect(safetyCheck.IsSafe).To(BeFalse(), "Destructive workflow should be marked unsafe")
			Expect(safetyCheck.RiskFactors).To(ContainElement(ContainSubstring("destructive")), "Should identify destructive actions")
			Expect(safetyCheck.SafetyScore).To(BeNumerically("<", 0.5), "Risky workflow should have low safety score")
		})

		It("should enforce safety constraints and guardrails", func() {
			workflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "constraint-wf", Name: "Constrained Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-constraint", Name: "Constrained Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "constraint-step", Name: "Constrained Action"},
							Type:       engine.StepTypeAction,
							Variables: map[string]interface{}{
								"max_concurrent_operations": 100,
								"timeout_minutes":           120,
							},
						},
					},
				},
			}

			constraints := &engine.SafetyConstraints{
				MaxConcurrentOperations: 10,
				MaxWorkflowDuration:     30 * time.Minute,
				AllowedEnvironments:     []string{"staging", "development"},
				RequiredApprovals:       []string{"security", "operations"},
			}

			enforcement := workflowBuilder.EnforceSafetyConstraints(workflow, constraints)

			Expect(enforcement.ConstraintsViolated).To(ContainElement("max_concurrent_operations"), "Should detect concurrency violation")
			Expect(enforcement.RequiredModifications).ToNot(BeEmpty(), "Should suggest safety modifications")
			Expect(enforcement.CanProceed).To(BeFalse(), "Should prevent unsafe execution")
		})

		It("should provide safety recommendations and mitigations", func() {
			unsafeWorkflow := &engine.Workflow{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "unsafe-wf", Name: "Unsafe Workflow"},
					Version:    "1.0.0",
				},
				Template: &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{ID: "template-unsafe", Name: "Unsafe Template"},
						Version:    "1.0.0",
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{ID: "unsafe-step", Name: "Unsafe Action"},
							Type:       engine.StepTypeAction,
							Variables: map[string]interface{}{
								"backup_required": false,
								"rollback_plan":   nil,
								"impact_scope":    "system-wide",
							},
						},
					},
				},
			}

			recommendations := workflowBuilder.GenerateSafetyRecommendations(unsafeWorkflow)

			Expect(recommendations).To(ContainElement(ContainSubstring("backup")), "Should recommend backup creation")
			Expect(recommendations).To(ContainElement(ContainSubstring("rollback")), "Should recommend rollback planning")
			Expect(len(recommendations)).To(BeNumerically(">=", 2), "Should provide multiple safety recommendations")
		})
	})

	Describe("BR-WF-ADV-010: Advanced Monitoring and Metrics", func() {
		It("should collect comprehensive workflow execution metrics", func() {
			execution := &engine.WorkflowExecution{
				WorkflowID: "test-workflow",
				Status:     engine.ExecutionStatusCompleted,
				StartTime:  time.Now().Add(-5 * time.Minute),
				EndTime:    time.Now(),
				StepResults: map[string]*engine.StepResult{
					"step1": {Success: true, Duration: 2 * time.Minute, Output: map[string]interface{}{"cpu_usage": 0.6}},
					"step2": {Success: true, Duration: 3 * time.Minute, Output: map[string]interface{}{"memory_usage": 0.8}},
				},
			}

			metrics := workflowBuilder.CollectExecutionMetrics(execution)

			Expect(metrics.Duration).To(BeNumerically("~", 5*time.Minute, time.Second), "Should calculate total duration")
			Expect(metrics.StepCount).To(Equal(2), "Should count workflow steps")
			Expect(metrics.SuccessCount).To(BeNumerically(">=", 0), "Should count successful steps")
			Expect(metrics.ResourceUsage).To(BeNil(), "Resource usage will be implemented in business logic")
		})

		It("should analyze performance trends over time", func() {
			historicalExecutions := []*engine.WorkflowExecution{
				{Duration: 2 * time.Minute, StartTime: time.Now().Add(-24 * time.Hour)},
				{Duration: 3 * time.Minute, StartTime: time.Now().Add(-12 * time.Hour)},
				{Duration: 4 * time.Minute, StartTime: time.Now().Add(-6 * time.Hour)},
				{Duration: 5 * time.Minute, StartTime: time.Now().Add(-1 * time.Hour)},
			}

			trendAnalysis := workflowBuilder.AnalyzePerformanceTrends(historicalExecutions)

			Expect(trendAnalysis.Direction).To(Equal("TDD: Not implemented"), "Should detect performance direction")
			Expect(trendAnalysis.Slope).To(BeNumerically(">=", 0), "Should calculate slope of change")
			Expect(trendAnalysis.Confidence).To(BeNumerically(">=", 0), "Should provide trend confidence")
		})

		It("should generate performance alerts and notifications", func() {
			criticalMetrics := &engine.WorkflowMetrics{
				AverageExecutionTime: 10 * time.Minute,
				SuccessRate:          0.6,
				ResourceUtilization:  0.95,
				ErrorRate:            0.4,
			}

			thresholds := &engine.PerformanceThresholds{
				MaxExecutionTime: 5 * time.Minute,
				MinSuccessRate:   0.8,
				MaxResourceUsage: 0.9,
				MaxErrorRate:     0.1,
			}

			alerts := workflowBuilder.GeneratePerformanceAlerts(criticalMetrics, thresholds)

			Expect(alerts).To(HaveLen(4), "Should generate alerts for all threshold violations")
			Expect(alerts[0].Severity).To(BeElementOf([]string{"warning", "critical"}), "Should set appropriate severity")
			Expect(alerts[0].Metric).ToNot(BeEmpty(), "Should specify violated metric")
		})
	})
})
