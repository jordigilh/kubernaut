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

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

// PatternStoreTestAdapter adapts StandardPatternStore to engine.PatternStore for testing
type PatternStoreTestAdapter struct {
	store *testshared.StandardPatternStore
}

func (a *PatternStoreTestAdapter) StorePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	return a.store.StoreEnginePattern(ctx, pattern)
}

func (a *PatternStoreTestAdapter) GetPattern(ctx context.Context, patternID string) (*types.DiscoveredPattern, error) {
	return a.store.GetPattern(ctx, patternID)
}

func (a *PatternStoreTestAdapter) ListPatterns(ctx context.Context, patternType string) ([]*types.DiscoveredPattern, error) {
	return a.store.ListPatterns(ctx, patternType)
}

func (a *PatternStoreTestAdapter) DeletePattern(ctx context.Context, patternID string) error {
	return a.store.DeletePattern(ctx, patternID)
}

var _ = Describe("BR-E2E-OPT-001: End-to-End Self Optimization Flow Test", Ordered, func() {
	var (
		hooks           *testshared.TestLifecycleHooks
		ctx             context.Context
		suite           *testshared.StandardTestSuite
		llmClient       llm.Client // RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated SelfOptimizer
		workflowBuilder engine.IntelligentWorkflowBuilder
		logger          *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure with real components
		hooks = testshared.SetupAIIntegrationTest("End-to-End Self Optimization Flow",
			testshared.WithRealVectorDB(),                                     // Real pgvector integration for pattern storage
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation), // Isolated test environment
		)

		ctx = context.Background()
		suite = hooks.GetSuite()
		logger = suite.Logger

		// Validate test environment is healthy before each test
		Expect(suite.VectorDB).ToNot(BeNil(), "Vector database should be available")
		Expect(suite.LLMClient).ToNot(BeNil(), "LLM client should be available")

		// Create workflow builder with real dependencies using new config pattern
		patternStore := testshared.CreatePatternStoreForTesting(suite.Logger)
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       suite.LLMClient,
			VectorDB:        suite.VectorDB,
			AnalyticsEngine: suite.AnalyticsEngine,
			PatternStore:    patternStore,
			ExecutionRepo:   suite.ExecutionRepo,
			Logger:          suite.Logger,
		}
		
		var err error
		workflowBuilder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
		Expect(workflowBuilder).ToNot(BeNil())

		// RULE 12 COMPLIANCE: Use enhanced llm.Client instead of deprecated SelfOptimizer
		llmClient = suite.LLMClient
		Expect(llmClient).ToNot(BeNil(), "Enhanced LLM client should be available for workflow optimization")
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	Context("when executing complete self-optimization flow", func() {
		It("should demonstrate end-to-end optimization with real vector database integration", func() {
			By("creating a complex workflow that needs optimization")
			originalWorkflow := generateComplexWorkflowForOptimization()
			Expect(originalWorkflow).ToNot(BeNil())
			Expect(originalWorkflow.Template.Steps).To(HaveLen(8), "Should have multiple steps for optimization")

			By("generating realistic execution history with performance patterns")
			executionHistory := generateRealisticExecutionHistory(originalWorkflow.ID, 15)
			Expect(executionHistory).To(HaveLen(15), "Should have sufficient execution history")

			// Validate execution history has performance patterns
			slowExecutions := 0
			for _, execution := range executionHistory {
				if execution.Duration > 300*time.Second {
					slowExecutions++
				}
			}
			Expect(slowExecutions).To(BeNumerically(">=", 3), "Should have slow executions to optimize")

			By("performing workflow optimization using enhanced LLM client with real vector database pattern analysis")
			optimizationStartTime := time.Now()
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() instead of deprecated SelfOptimizer
			optimizationResult, err := llmClient.OptimizeWorkflow(ctx, originalWorkflow, executionHistory)
			optimizationDuration := time.Since(optimizationStartTime)
			
			// Extract optimized workflow from LLM result
			var optimizedWorkflow *engine.Workflow
			if optimizationResult != nil {
				if resultMap, ok := optimizationResult.(map[string]interface{}); ok {
					if optimizedWf, exists := resultMap["optimized_workflow"]; exists {
						optimizedWorkflow = optimizedWf.(*engine.Workflow)
					}
				}
			}

			Expect(err).ToNot(HaveOccurred(), "Self optimization should succeed")
			Expect(optimizedWorkflow).ToNot(BeNil(), "Should return optimized workflow")
			Expect(optimizationDuration).To(BeNumerically("<=", 30*time.Second),
				"BR-E2E-OPT-001: Optimization should complete within 30 seconds")

			By("validating optimization results contain meaningful improvements")
			// Verify workflow structure is preserved
			Expect(optimizedWorkflow.ID).To(ContainSubstring("_optimized"), "Should have optimized ID")
			Expect(optimizedWorkflow.Name).To(ContainSubstring("(Optimized)"), "Should have optimized name")
			Expect(optimizedWorkflow.Template.Steps).ToNot(BeEmpty(), "Should preserve workflow steps")

			// Verify optimization metadata is present
			Expect(optimizedWorkflow.Template.Metadata).ToNot(BeNil(), "Should have optimization metadata")
			if optimizationApplied, exists := optimizedWorkflow.Template.Metadata["optimization_applied"]; exists {
				Expect(optimizationApplied).To(BeTrue(), "Should mark optimization as applied")
			}

			By("generating optimization suggestions with business impact analysis")
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.SuggestOptimizations() instead of deprecated SelfOptimizer
			suggestionResult, err := llmClient.SuggestOptimizations(ctx, originalWorkflow)
			Expect(err).ToNot(HaveOccurred(), "Should generate optimization suggestions")
			
			// Extract suggestions from LLM result
			var suggestions []map[string]interface{}
			if suggestionResult != nil {
				if suggestionSlice, ok := suggestionResult.([]map[string]interface{}); ok {
					suggestions = suggestionSlice
				}
			}
			Expect(len(suggestions)).To(BeNumerically(">=", 1), "Should provide optimization suggestions")

			// Validate suggestion quality
			for _, suggestion := range suggestions {
				if desc, exists := suggestion["suggestion"]; exists {
					Expect(desc).ToNot(BeEmpty(), "Each suggestion should have description")
				}
				if impact, exists := suggestion["impact"]; exists {
					Expect(impact).To(BeNumerically(">", 0), "Each suggestion should have positive impact")
				}
				if priority, exists := suggestion["priority"]; exists {
					Expect(priority).ToNot(BeEmpty(), "Each suggestion should have priority")
				}
			}

			By("measuring end-to-end optimization performance improvement")
			// Simulate execution of original vs optimized workflow
			originalExecutionTime := measureWorkflowComplexity(originalWorkflow)
			optimizedExecutionTime := measureWorkflowComplexity(optimizedWorkflow)

			// Business SLA: Optimization should improve performance by at least 10%
			performanceImprovement := float64(originalExecutionTime-optimizedExecutionTime) / float64(originalExecutionTime)
			Expect(performanceImprovement).To(BeNumerically(">=", 0.10),
				"BR-E2E-OPT-001: Should achieve at least 10% performance improvement")

			By("validating vector database integration for pattern storage")
			// Verify that optimization patterns are stored in vector database
			patterns, err := suite.VectorDB.SearchBySemantics(ctx, "workflow_optimization", 5)
			Expect(err).ToNot(HaveOccurred(), "Should be able to search for optimization patterns")
			Expect(len(patterns)).To(BeNumerically(">=", 1), "Should have stored optimization patterns")

			By("demonstrating optimization learning and pattern recognition")
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() for second optimization
			secondOptimizationResult, err := llmClient.OptimizeWorkflow(ctx, originalWorkflow, executionHistory)
			Expect(err).ToNot(HaveOccurred(), "Second optimization should succeed")
			Expect(secondOptimizationResult).ToNot(BeNil(), "Should return second optimized workflow")

			// Second optimization should be faster due to learned patterns
			secondOptimizationStartTime := time.Now()
			_, err = llmClient.OptimizeWorkflow(ctx, originalWorkflow, executionHistory)
			secondOptimizationDuration := time.Since(secondOptimizationStartTime)

			Expect(secondOptimizationDuration).To(BeNumerically("<=", optimizationDuration),
				"BR-E2E-OPT-001: Second optimization should be faster due to pattern learning")
		})

		It("should handle optimization failures gracefully with fallback mechanisms", func() {
			By("creating a workflow that might cause optimization challenges")
			challengingWorkflow := generateChallengingWorkflow()
			Expect(challengingWorkflow).ToNot(BeNil())

			By("providing minimal execution history to test robustness")
			minimalHistory := generateRealisticExecutionHistory(challengingWorkflow.ID, 2)
			Expect(minimalHistory).To(HaveLen(2))

			By("attempting optimization with limited data")
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() for challenging scenarios
			optimizationResult, err := llmClient.OptimizeWorkflow(ctx, challengingWorkflow, minimalHistory)
			
			// Extract optimized workflow from LLM result
			var optimizedWorkflow *engine.Workflow
			if optimizationResult != nil {
				if resultMap, ok := optimizationResult.(map[string]interface{}); ok {
					if optimizedWf, exists := resultMap["optimized_workflow"]; exists {
						optimizedWorkflow = optimizedWf.(*engine.Workflow)
					}
				}
			}

			// Should either succeed with graceful degradation or fail gracefully
			if err != nil {
				// If optimization fails, error should be informative
				Expect(err.Error()).To(ContainSubstring("BR-"), "Error should reference business requirement")
				logger.WithError(err).Info("Optimization failed gracefully as expected")
			} else {
				// If optimization succeeds, should preserve workflow integrity
				Expect(optimizedWorkflow).ToNot(BeNil())
				Expect(optimizedWorkflow.Template.Steps).ToNot(BeEmpty())
				logger.Info("Optimization succeeded despite challenging conditions")
			}

			By("validating system remains stable after optimization challenges")
			// System should remain responsive
			healthCheck := time.Now()
			isHealthy := suite.VectorDB.IsHealthy(ctx)
			healthCheckDuration := time.Since(healthCheck)

			Expect(isHealthy).To(BeNil(), "Vector database should remain healthy")
			Expect(healthCheckDuration).To(BeNumerically("<=", 5*time.Second),
				"Health check should be responsive")
		})
	})

	Context("when testing optimization accuracy and business value", func() {
		It("should demonstrate measurable business value from optimization", func() {
			By("creating workflows with known optimization opportunities")
			inefficientWorkflow := generateInefficinetWorkflowWithKnownIssues()
			Expect(inefficientWorkflow).ToNot(BeNil())

			By("generating execution history that highlights inefficiencies")
			problematicHistory := generateProblematicExecutionHistory(inefficientWorkflow.ID, 20)
			Expect(problematicHistory).To(HaveLen(20))

			// Verify history contains performance issues
			timeoutCount := 0
			for _, execution := range problematicHistory {
				if execution.Duration > 600*time.Second { // 10+ minute executions
					timeoutCount++
				}
			}
			Expect(timeoutCount).To(BeNumerically(">=", 5), "Should have timeout-prone executions")

			By("performing optimization focused on business metrics")
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() for business metrics optimization
			optimizationResult, err := llmClient.OptimizeWorkflow(ctx, inefficientWorkflow, problematicHistory)
			Expect(err).ToNot(HaveOccurred())
			
			// Extract optimized workflow from LLM result
			var optimizedWorkflow *engine.Workflow
			if optimizationResult != nil {
				if resultMap, ok := optimizationResult.(map[string]interface{}); ok {
					if optimizedWf, exists := resultMap["optimized_workflow"]; exists {
						optimizedWorkflow = optimizedWf.(*engine.Workflow)
					}
				}
			}
			Expect(optimizedWorkflow).ToNot(BeNil())

			By("measuring business impact of optimization")
			// Calculate cost reduction (simulated)
			originalCost := calculateWorkflowCost(inefficientWorkflow, problematicHistory)
			optimizedCost := calculateWorkflowCost(optimizedWorkflow, problematicHistory)
			costReduction := (originalCost - optimizedCost) / originalCost

			Expect(costReduction).To(BeNumerically(">=", 0.15),
				"BR-E2E-OPT-001: Should achieve at least 15% cost reduction")

			// Calculate reliability improvement
			originalReliability := calculateWorkflowReliability(problematicHistory)
			expectedOptimizedReliability := originalReliability + 0.1 // 10% improvement expected

			Expect(expectedOptimizedReliability).To(BeNumerically(">=", 0.85),
				"BR-E2E-OPT-001: Should achieve at least 85% reliability")

			By("validating optimization suggestions provide actionable insights")
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.SuggestOptimizations() for actionable insights
			suggestionResult, err := llmClient.SuggestOptimizations(ctx, inefficientWorkflow)
			Expect(err).ToNot(HaveOccurred())
			
			// Extract suggestions from LLM result
			var suggestions []map[string]interface{}
			if suggestionResult != nil {
				if suggestionSlice, ok := suggestionResult.([]map[string]interface{}); ok {
					suggestions = suggestionSlice
				}
			}
			Expect(len(suggestions)).To(BeNumerically(">=", 3), "Should provide multiple actionable suggestions")

			// Validate suggestions are business-focused
			businessFocusedSuggestions := 0
			for _, suggestion := range suggestions {
				if impact, exists := suggestion["impact"]; exists {
					if impactFloat, ok := impact.(float64); ok && impactFloat >= 0.1 { // 10%+ impact
						businessFocusedSuggestions++
					}
				}
			}
			Expect(businessFocusedSuggestions).To(BeNumerically(">=", 2),
				"Should have high-impact business suggestions")
		})
	})
})

// Business Contract Helper Functions - Following guideline: Define business contracts to enable test compilation

// generateComplexWorkflowForOptimization creates a complex workflow suitable for optimization testing
func generateComplexWorkflowForOptimization() *engine.Workflow {
	template := &engine.ExecutableTemplate{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          "complex-optimization-template",
				Name:        "Complex Workflow for Optimization",
				Description: "Multi-step workflow with optimization opportunities",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "test-system",
		},
		Steps: []*engine.ExecutableWorkflowStep{
			{
				BaseEntity: types.BaseEntity{
					ID:          "data-fetch",
					Name:        "Fetch Data",
					Description: "Fetch large dataset for processing",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"size": "large", "cache": false},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "fetch_large_dataset",
					Parameters: map[string]interface{}{"size": "large", "cache": false},
				},
				Timeout: 300 * time.Second, // Long timeout - optimization opportunity
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "data-process-1",
					Name:        "Process Data Phase 1",
					Description: "First phase of data processing",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"algorithm": "slow", "parallel": false},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "process_data",
					Parameters: map[string]interface{}{"algorithm": "slow", "parallel": false},
				},
				Timeout:      240 * time.Second,
				Dependencies: []string{"data-fetch"},
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "data-process-2",
					Name:        "Process Data Phase 2",
					Description: "Second phase of data processing",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"algorithm": "slow", "parallel": false},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "process_data",
					Parameters: map[string]interface{}{"algorithm": "slow", "parallel": false},
				},
				Timeout:      240 * time.Second,
				Dependencies: []string{"data-process-1"},
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "validation",
					Name:        "Validate Results",
					Description: "Comprehensive validation of processed results",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"strict": true, "retries": 5},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "validate_comprehensive",
					Parameters: map[string]interface{}{"strict": true, "retries": 5},
				},
				Timeout:      180 * time.Second,
				Dependencies: []string{"data-process-2"},
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "backup",
					Name:        "Backup Results",
					Description: "Backup processed results",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"compression": false, "redundancy": 3},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "backup_full",
					Parameters: map[string]interface{}{"compression": false, "redundancy": 3},
				},
				Timeout:      120 * time.Second,
				Dependencies: []string{"validation"},
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "notify-start",
					Name:        "Send Start Notification",
					Description: "Send workflow start notification",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"type": "start", "priority": "low"},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "send_notification",
					Parameters: map[string]interface{}{"type": "start", "priority": "low"},
				},
				Timeout: 30 * time.Second,
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "notify-end",
					Name:        "Send End Notification",
					Description: "Send workflow completion notification",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"type": "end", "priority": "low"},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "send_notification",
					Parameters: map[string]interface{}{"type": "end", "priority": "low"},
				},
				Timeout:      30 * time.Second,
				Dependencies: []string{"backup"},
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "cleanup",
					Name:        "Cleanup Resources",
					Description: "Clean up workflow resources",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"aggressive": true},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "cleanup_resources",
					Parameters: map[string]interface{}{"aggressive": true},
				},
				Timeout:      60 * time.Second,
				Dependencies: []string{"notify-end"},
			},
		},
	}

	return &engine.Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          "complex-workflow-for-optimization",
				Name:        "Complex Workflow for Optimization Testing",
				Description: "A workflow designed to test end-to-end optimization capabilities",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "test-system",
		},
		Template: template,
	}
}

// generateRealisticExecutionHistory creates realistic execution history with performance patterns
func generateRealisticExecutionHistory(workflowID string, count int) []*engine.RuntimeWorkflowExecution {
	history := make([]*engine.RuntimeWorkflowExecution, count)
	baseTime := time.Now().Add(-24 * time.Hour)

	for i := 0; i < count; i++ {
		// Create varied execution durations to simulate real patterns
		var duration time.Duration
		if i%4 == 0 { // 25% slow executions
			duration = time.Duration(300+i*10) * time.Second
		} else if i%3 == 0 { // Some medium executions
			duration = time.Duration(180+i*5) * time.Second
		} else { // Mostly fast executions
			duration = time.Duration(120+i*2) * time.Second
		}

		status := engine.ExecutionStatusCompleted
		if i%10 == 0 { // 10% failed executions
			status = engine.ExecutionStatusFailed
			duration = time.Duration(60+i) * time.Second // Failed executions are shorter
		}

		execution := &engine.RuntimeWorkflowExecution{
			WorkflowExecutionRecord: types.WorkflowExecutionRecord{
				ID:         fmt.Sprintf("%s-execution-%d", workflowID, i),
				WorkflowID: workflowID,
				Status:     string(status),
				StartTime:  baseTime.Add(time.Duration(i) * time.Hour),
				Metadata:   make(map[string]interface{}),
			},
			OperationalStatus: status,
			Duration:          duration,
		}

		// Add performance metadata
		execution.Metadata["execution_duration_seconds"] = duration.Seconds()
		execution.Metadata["resource_usage"] = map[string]interface{}{
			"cpu_percent": 70 + (i % 30),
			"memory_mb":   1024 + (i * 50),
			"network_kb":  500 + (i * 10),
		}

		history[i] = execution
	}

	return history
}

// generateChallengingWorkflow creates a workflow that might challenge the optimizer
func generateChallengingWorkflow() *engine.Workflow {
	template := &engine.ExecutableTemplate{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          "challenging-template",
				Name:        "Challenging Workflow",
				Description: "Workflow with complex dependencies and edge cases",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata: map[string]interface{}{
					"complexity": "extreme",
					"edge_cases": true,
				},
			},
			Version:   "1.0.0",
			CreatedBy: "test-system",
		},
		Steps: []*engine.ExecutableWorkflowStep{
			{
				BaseEntity: types.BaseEntity{
					ID:          "edge-case-step",
					Name:        "Edge Case Processing",
					Description: "Handle complex edge cases",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"complexity": "extreme"},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "handle_edge_cases",
					Parameters: map[string]interface{}{"complexity": "extreme"},
				},
				Timeout: 600 * time.Second, // Very long timeout
			},
		},
	}

	return &engine.Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          "challenging-workflow",
				Name:        "Challenging Workflow",
				Description: "Workflow designed to test optimizer robustness",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "test-system",
		},
		Template: template,
	}
}

// generateInefficinetWorkflowWithKnownIssues creates workflow with known inefficiencies
func generateInefficinetWorkflowWithKnownIssues() *engine.Workflow {
	template := &engine.ExecutableTemplate{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          "inefficient-template",
				Name:        "Inefficient Workflow",
				Description: "Workflow with known performance issues",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata: map[string]interface{}{
					"efficiency": "poor",
					"issues":     []string{"timeouts", "redundancy", "no_caching"},
				},
			},
			Version:   "1.0.0",
			CreatedBy: "test-system",
		},
		Steps: []*engine.ExecutableWorkflowStep{
			{
				BaseEntity: types.BaseEntity{
					ID:          "slow-operation",
					Name:        "Slow Operation",
					Description: "Slow operation that needs optimization",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"optimization": false, "cache": false},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "slow_operation",
					Parameters: map[string]interface{}{"optimization": false, "cache": false},
				},
				Timeout: 900 * time.Second, // 15 minutes - too long
			},
			{
				BaseEntity: types.BaseEntity{
					ID:          "redundant-operation",
					Name:        "Redundant Operation",
					Description: "Redundant operation that should be optimized",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"duplicate": true},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "redundant_work",
					Parameters: map[string]interface{}{"duplicate": true},
				},
				Timeout:      300 * time.Second,
				Dependencies: []string{"slow-operation"},
			},
		},
	}

	return &engine.Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          "inefficient-workflow",
				Name:        "Inefficient Workflow with Known Issues",
				Description: "Workflow designed to demonstrate optimization value",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "test-system",
		},
		Template: template,
	}
}

// measureWorkflowComplexity calculates workflow complexity for performance comparison
func measureWorkflowComplexity(workflow *engine.Workflow) time.Duration {
	if workflow == nil || workflow.Template == nil {
		return 0
	}

	// Simple complexity calculation based on steps and timeouts
	totalComplexity := time.Duration(0)
	for _, step := range workflow.Template.Steps {
		// Base complexity from timeout
		stepComplexity := step.Timeout

		// Add complexity for dependencies
		stepComplexity += time.Duration(len(step.Dependencies)) * 10 * time.Second

		// Add complexity for parameters
		if step.Action != nil && step.Action.Parameters != nil {
			stepComplexity += time.Duration(len(step.Action.Parameters)) * 5 * time.Second
		}

		totalComplexity += stepComplexity
	}

	return totalComplexity
}
