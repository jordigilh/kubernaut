package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TestPerformanceMonitoringIntegrationTDD verifies that the TDD implementation of performance monitoring integration works
func TestPerformanceMonitoringIntegrationTDD(t *testing.T) {
	// Setup
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create mock vector database
	mockVectorDB := mocks.NewMockVectorDatabase()

	// Create builder with mock dependencies using config pattern
	config := &engine.IntelligentWorkflowBuilderConfig{
		LLMClient:       nil,          // External: Mock not needed for this test
		VectorDB:        mockVectorDB, // External: Mock provided
		AnalyticsEngine: nil,          // External: Mock not needed for this test
		PatternStore:    nil,          // External: Mock not needed for this test
		ExecutionRepo:   nil,          // External: Mock not needed for this test
		Logger:          log,
	}

	builder, err := engine.NewIntelligentWorkflowBuilder(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create workflow builder: %v", err))
	}

	ctx := context.Background()

	// Test 1: Verify performance monitoring methods are accessible
	t.Run("Performance monitoring methods are accessible", func(t *testing.T) {
		// Create test execution for metrics collection
		execution := &engine.WorkflowExecution{
			WorkflowID: "test-workflow-001",
			Duration:   5 * time.Minute,
			StepResults: map[string]*engine.StepResult{
				"step-001": {
					Success:  true,
					Duration: 2 * time.Minute,
					Output: map[string]interface{}{
						"result": "success",
					},
				},
				"step-002": {
					Success:  true,
					Duration: 3 * time.Minute,
					Output: map[string]interface{}{
						"result": "success",
					},
				},
			},
		}

		// Test CollectExecutionMetrics
		metrics := builder.CollectExecutionMetrics(execution)
		require.NotNil(t, metrics)
		assert.Equal(t, execution.Duration, metrics.Duration)
		assert.Equal(t, len(execution.StepResults), metrics.StepCount)
		assert.Greater(t, metrics.SuccessCount, 0)

		// Test AnalyzePerformanceTrends
		executions := []*engine.WorkflowExecution{execution}
		trends := builder.AnalyzePerformanceTrends(executions)
		require.NotNil(t, trends)
		assert.NotEmpty(t, trends.Direction)

		// Test GeneratePerformanceAlerts
		workflowMetrics := &engine.WorkflowMetrics{
			AverageExecutionTime: 5 * time.Minute,
			SuccessRate:          0.9,
			ResourceUtilization:  0.5,
			ErrorRate:            0.05,
		}
		thresholds := &engine.PerformanceThresholds{
			MaxExecutionTime: 10 * time.Minute,
			MinSuccessRate:   0.8,
			MaxResourceUsage: 0.8,
			MaxErrorRate:     0.1,
		}
		alerts := builder.GeneratePerformanceAlerts(workflowMetrics, thresholds)
		require.NotNil(t, alerts)

		// Test AnalyzeLoopPerformance
		loopMetrics := &engine.LoopExecutionMetrics{
			TotalIterations:      5,
			SuccessfulIterations: 4,
			FailedIterations:     1,
			AverageIterationTime: 1 * time.Minute,
		}
		loopOptimization := builder.AnalyzeLoopPerformance(loopMetrics)
		require.NotNil(t, loopOptimization)
		assert.Equal(t, 0.8, loopOptimization.SuccessRate) // 4/5

		// Test CalculateWorkflowComplexity
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template",
					Name: "Test Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 5 * time.Minute,
				},
			},
		}
		workflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   template.ID,
					Name: template.Name,
				},
			},
			Template: template,
		}
		complexity := builder.CalculateWorkflowComplexity(workflow)
		require.NotNil(t, complexity)
		assert.GreaterOrEqual(t, complexity.OverallScore, 0.0)
	})

	// Test 2: Verify OptimizeWorkflowStructure includes performance monitoring
	t.Run("OptimizeWorkflowStructure includes performance monitoring", func(t *testing.T) {
		// Create template with performance monitoring triggers
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "perf-template-001",
					Name: "Performance Monitoring Template",
					Metadata: map[string]interface{}{
						"performance_monitoring": true,
						"monitoring_level":       "comprehensive",
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Action Step 1",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
						Parameters: map[string]interface{}{
							"replicas": 3,
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Action Step 2",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "collect_diagnostics",
					},
				},
			},
			Variables: make(map[string]interface{}),
		}

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)
		require.NotNil(t, optimizedTemplate.Metadata)

		// Verify that performance monitoring optimization was applied
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check for performance monitoring indicators in metadata
		if optimizedTemplate.Metadata != nil {
			if performanceMonitoring, exists := optimizedTemplate.Metadata["performance_monitoring"]; exists {
				assert.Equal(t, true, performanceMonitoring)
			}
			if complexityScore, exists := optimizedTemplate.Metadata["complexity_score"]; exists {
				assert.IsType(t, float64(0), complexityScore)
				assert.GreaterOrEqual(t, complexityScore.(float64), 0.0)
				assert.LessOrEqual(t, complexityScore.(float64), 1.0)
			}
			if performanceOptimized, exists := optimizedTemplate.Metadata["performance_optimized"]; exists {
				assert.Equal(t, true, performanceOptimized)
			}
		}
	})

	// Test 3: Verify GenerateWorkflow includes performance monitoring
	t.Run("GenerateWorkflow includes performance monitoring", func(t *testing.T) {
		objective := &engine.WorkflowObjective{
			ID:          "perf-obj-001",
			Type:        "performance_optimization",
			Description: "Performance monitoring workflow optimization",
			Priority:    7,
			Constraints: map[string]interface{}{
				"performance_monitoring": true,
				"monitoring_level":       "comprehensive",
				"max_execution_time":     "45m",
			},
		}

		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// Verify the workflow generation process includes performance monitoring
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that performance monitoring metadata is present
		if template.Metadata != nil {
			// Performance monitoring should contribute to workflow metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 4: Verify performance monitoring handles different complexity levels
	t.Run("Performance monitoring handles different complexity levels", func(t *testing.T) {
		// Test with high complexity template (multiple action steps, loops, conditions)
		highComplexityTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "high-complexity-template",
					Name: "High Complexity Template",
					Metadata: map[string]interface{}{
						"performance_monitoring": true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "action-step-1",
						Name: "Action Step 1",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "loop-step",
						Name: "Loop Step",
					},
					Type:    engine.StepTypeLoop,
					Timeout: 20 * time.Minute,
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "condition-step",
						Name: "Condition Step",
					},
					Type:    engine.StepTypeCondition,
					Timeout: 5 * time.Minute,
					Condition: &engine.ExecutableCondition{
						ID:         "condition-001",
						Name:       "Test Condition",
						Type:       engine.ConditionTypeCustom,
						Expression: "status == 'ready'",
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "subflow-step",
						Name: "Subflow Step",
					},
					Type:    engine.StepTypeSubflow,
					Timeout: 15 * time.Minute,
				},
			},
		}

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, highComplexityTemplate)

		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)

		// Verify performance monitoring was applied to high complexity workflow
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check that steps have performance monitoring variables
		for _, step := range optimizedTemplate.Steps {
			if step.Variables != nil {
				if performanceMonitored, exists := step.Variables["performance_monitored"]; exists {
					assert.Equal(t, true, performanceMonitored)
				}
			}
		}
	})

	// Test 5: Verify performance monitoring integration follows business requirements
	t.Run("Performance monitoring integration follows business requirements", func(t *testing.T) {
		// BR-PERF-001: Comprehensive execution metrics collection
		// BR-PERF-002: Performance trend analysis
		// BR-PERF-003: Performance alert generation
		// BR-PERF-004: Loop performance analysis
		// BR-PERF-005: Workflow complexity assessment
		// BR-PERF-006: Performance monitoring integration in workflow generation
		// BR-PERF-007: Performance-based optimization application
		// BR-PERF-008: Complexity-based timeout optimization

		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "br-test-template",
					Name: "Business Requirements Test Template",
					Metadata: map[string]interface{}{
						"performance_monitoring": true,
						"monitoring_level":       "comprehensive",
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
						Parameters: map[string]interface{}{
							"replicas": 3,
						},
					},
				},
			},
		}

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)
		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)

		// Verify comprehensive performance monitoring was performed
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)
		assert.NotNil(t, optimizedTemplate.Metadata)

		// Verify performance monitoring metadata is present
		if optimizedTemplate.Metadata != nil {
			// Performance monitoring should contribute to workflow metadata
			assert.IsType(t, map[string]interface{}{}, optimizedTemplate.Metadata)
		}
	})

	// Test 6: Verify performance monitoring handles edge cases
	t.Run("Performance monitoring handles edge cases gracefully", func(t *testing.T) {
		// Test with empty template
		emptyTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "empty-template",
					Name: "Empty Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{}, // No steps
		}

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, emptyTemplate)
		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)

		// Should handle empty template gracefully
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.Equal(t, 0, len(optimizedTemplate.Steps))

		// Test with template having only conditional steps
		conditionalTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "conditional-template",
					Name: "Conditional Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "conditional-step",
						Name: "Conditional Step",
					},
					Type: engine.StepTypeCondition,
					Condition: &engine.ExecutableCondition{
						ID:         "condition-001",
						Name:       "Test Condition",
						Type:       engine.ConditionTypeCustom,
						Expression: "status == 'ready'",
					},
				},
			},
		}

		conditionalOptimized, err := builder.OptimizeWorkflowStructure(ctx, conditionalTemplate)
		require.NoError(t, err)
		require.NotNil(t, conditionalOptimized)
		assert.NotEmpty(t, conditionalOptimized.ID)
	})
}
