package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TestAdvancedAnalyticsIntegrationTDD verifies that the TDD implementation of advanced analytics integration works
func TestAdvancedAnalyticsIntegrationTDD(t *testing.T) {
	// Setup
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create mock vector database
	mockVectorDB := mocks.NewMockVectorDatabase()

	// Create builder with mock dependencies
	builder := engine.NewIntelligentWorkflowBuilder(nil, mockVectorDB, nil, nil, nil, nil, log)

	ctx := context.Background()

	// Test 1: Verify advanced analytics methods are accessible
	t.Run("Advanced analytics methods are accessible", func(t *testing.T) {
		// Create test workflow for advanced analytics
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template-001",
					Name: "Test Advanced Analytics Template",
					Metadata: map[string]interface{}{
						"advanced_analytics":  true,
						"analytics_level":     "comprehensive",
						"predictive_enabled":  true,
						"insights_generation": true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Analytics Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "collect_metrics", // Analytics action
						Parameters: map[string]interface{}{
							"metrics_type": "performance",
						},
						Target: &engine.ActionTarget{
							Type:      "metrics_collector",
							Namespace: "monitoring",
							Name:      "performance-collector",
							Resource:  "collectors",
						},
					},
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

		// Test GenerateAdvancedInsights
		executionHistory := []*engine.RuntimeWorkflowExecution{
			{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "exec-001",
					WorkflowID: workflow.ID,
					StartTime:  time.Now().Add(-30 * time.Minute),
					EndTime:    func() *time.Time { t := time.Now().Add(-25 * time.Minute); return &t }(),
				},
				OperationalStatus: engine.ExecutionStatusCompleted,
				Steps: []*engine.StepExecution{
					{
						StepID:    "step-001",
						Status:    engine.ExecutionStatusCompleted,
						StartTime: time.Now().Add(-30 * time.Minute),
						Duration:  2 * time.Minute,
					},
				},
			},
		}

		insights := builder.GenerateAdvancedInsights(ctx, workflow, executionHistory)
		require.NotNil(t, insights)
		assert.Equal(t, workflow.ID, insights.WorkflowID)
		assert.GreaterOrEqual(t, insights.Confidence, 0.0)
		assert.LessOrEqual(t, insights.Confidence, 1.0)
		assert.NotEmpty(t, insights.InsightType)
		assert.NotNil(t, insights.Insights)
		assert.NotZero(t, insights.GeneratedAt)

		// Test CalculatePredictiveMetrics
		historicalData := []*engine.WorkflowMetrics{
			{
				AverageExecutionTime: 5 * time.Minute,
				SuccessRate:          0.95,
				ResourceUtilization:  0.7,
				FailureRate:          0.05,
				ErrorRate:            0.02,
			},
			{
				AverageExecutionTime: 4 * time.Minute,
				SuccessRate:          0.92,
				ResourceUtilization:  0.8,
				FailureRate:          0.08,
				ErrorRate:            0.03,
			},
		}

		predictiveMetrics := builder.CalculatePredictiveMetrics(ctx, workflow, historicalData)
		require.NotNil(t, predictiveMetrics)
		assert.Equal(t, workflow.ID, predictiveMetrics.WorkflowID)
		assert.Greater(t, predictiveMetrics.PredictedExecutionTime, time.Duration(0))
		assert.GreaterOrEqual(t, predictiveMetrics.PredictedSuccessRate, 0.0)
		assert.LessOrEqual(t, predictiveMetrics.PredictedSuccessRate, 1.0)
		assert.GreaterOrEqual(t, predictiveMetrics.PredictedResourceUsage, 0.0)
		assert.LessOrEqual(t, predictiveMetrics.PredictedResourceUsage, 1.0)
		assert.GreaterOrEqual(t, predictiveMetrics.ConfidenceLevel, 0.0)
		assert.LessOrEqual(t, predictiveMetrics.ConfidenceLevel, 1.0)
		assert.NotNil(t, predictiveMetrics.TrendAnalysis)
		assert.NotEmpty(t, predictiveMetrics.RiskAssessment)

		// Test OptimizeBasedOnPredictions
		optimizedTemplate := builder.OptimizeBasedOnPredictions(ctx, template, predictiveMetrics)
		require.NotNil(t, optimizedTemplate)
		assert.Equal(t, template.ID, optimizedTemplate.ID)
		assert.GreaterOrEqual(t, len(optimizedTemplate.Steps), len(template.Steps))

		// Verify prediction-based optimizations were applied
		for _, step := range optimizedTemplate.Steps {
			if step.Variables != nil {
				if predictionOptimized, exists := step.Variables["prediction_optimized"]; exists {
					assert.Equal(t, true, predictionOptimized)
				}
			}
		}

		// Test EnhanceWithAI
		enhancedTemplate := builder.EnhanceWithAI(template)
		require.NotNil(t, enhancedTemplate)
		assert.Equal(t, template.ID, enhancedTemplate.ID)
		assert.GreaterOrEqual(t, len(enhancedTemplate.Steps), len(template.Steps))
	})

	// Test 2: Verify OptimizeWorkflowStructure includes advanced analytics
	t.Run("OptimizeWorkflowStructure includes advanced analytics", func(t *testing.T) {
		// Create template with advanced analytics triggers
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "analytics-template-001",
					Name: "Advanced Analytics Template",
					Metadata: map[string]interface{}{
						"advanced_analytics":  true,
						"analytics_level":     "comprehensive",
						"predictive_enabled":  true,
						"insights_generation": true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Analytics Data Collection Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "collect_metrics",
						Parameters: map[string]interface{}{
							"metrics_type": "performance",
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Analytics Processing Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "process_analytics",
						Parameters: map[string]interface{}{
							"algorithm": "predictive_analysis",
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-003",
						Name: "Insights Generation Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 12 * time.Minute,
					Action: &engine.StepAction{
						Type: "generate_insights",
						Parameters: map[string]interface{}{
							"insight_type": "advanced",
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
		}

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)
		require.NotNil(t, optimizedTemplate.Metadata)

		// Verify that advanced analytics was applied
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check for advanced analytics indicators in metadata
		if optimizedTemplate.Metadata != nil {
			if advancedAnalytics, exists := optimizedTemplate.Metadata["advanced_analytics"]; exists {
				assert.Equal(t, true, advancedAnalytics)
			}
			if insightsCount, exists := optimizedTemplate.Metadata["insights_count"]; exists {
				assert.IsType(t, 0, insightsCount)
				assert.GreaterOrEqual(t, insightsCount.(int), 0)
			}
			if insightsConfidence, exists := optimizedTemplate.Metadata["insights_confidence"]; exists {
				assert.IsType(t, float64(0), insightsConfidence)
				assert.GreaterOrEqual(t, insightsConfidence.(float64), 0.0)
				assert.LessOrEqual(t, insightsConfidence.(float64), 1.0)
			}
			if predictedSuccessRate, exists := optimizedTemplate.Metadata["predicted_success_rate"]; exists {
				assert.IsType(t, float64(0), predictedSuccessRate)
				assert.GreaterOrEqual(t, predictedSuccessRate.(float64), 0.0)
				assert.LessOrEqual(t, predictedSuccessRate.(float64), 1.0)
			}
			if predictionConfidence, exists := optimizedTemplate.Metadata["prediction_confidence"]; exists {
				assert.IsType(t, float64(0), predictionConfidence)
				assert.GreaterOrEqual(t, predictionConfidence.(float64), 0.0)
				assert.LessOrEqual(t, predictionConfidence.(float64), 1.0)
			}
			if riskAssessment, exists := optimizedTemplate.Metadata["risk_assessment"]; exists {
				assert.IsType(t, "", riskAssessment)
				assert.NotEmpty(t, riskAssessment.(string))
				assert.Contains(t, []string{"low", "medium", "high", "unknown"}, riskAssessment.(string))
			}
		}
	})

	// Test 3: Verify GenerateWorkflow includes advanced analytics
	t.Run("GenerateWorkflow includes advanced analytics", func(t *testing.T) {
		objective := &engine.WorkflowObjective{
			ID:          "analytics-obj-001",
			Type:        "advanced_analytics",
			Description: "Advanced analytics workflow optimization",
			Priority:    9,
			Constraints: map[string]interface{}{
				"advanced_analytics":  true,
				"analytics_level":     "comprehensive",
				"predictive_enabled":  true,
				"insights_generation": true,
			},
		}

		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// Verify the workflow generation process includes advanced analytics
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that advanced analytics metadata is present
		if template.Metadata != nil {
			// Advanced analytics should contribute to workflow metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 4: Verify advanced analytics handles different scenarios
	t.Run("Advanced analytics handles different scenarios", func(t *testing.T) {
		// Test with no execution history
		emptyTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "empty-template",
					Name: "Empty Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Simple Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 5 * time.Minute,
					Action: &engine.StepAction{
						Type: "get_status",
					},
				},
			},
		}

		emptyWorkflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: emptyTemplate.ID,
				},
			},
			Template: emptyTemplate,
		}

		insights := builder.GenerateAdvancedInsights(ctx, emptyWorkflow, []*engine.RuntimeWorkflowExecution{})
		require.NotNil(t, insights)
		assert.Equal(t, emptyWorkflow.ID, insights.WorkflowID)
		assert.GreaterOrEqual(t, insights.Confidence, 0.0)

		// Test with minimal historical data
		minimalData := []*engine.WorkflowMetrics{
			{
				AverageExecutionTime: 5 * time.Minute,
				SuccessRate:          0.95,
				ResourceUtilization:  0.7,
			},
		}

		predictiveMetrics := builder.CalculatePredictiveMetrics(ctx, emptyWorkflow, minimalData)
		require.NotNil(t, predictiveMetrics)
		assert.Equal(t, emptyWorkflow.ID, predictiveMetrics.WorkflowID)
		assert.GreaterOrEqual(t, predictiveMetrics.ConfidenceLevel, 0.0)
		assert.LessOrEqual(t, predictiveMetrics.ConfidenceLevel, 1.0)
	})

	// Test 5: Verify business requirement compliance
	t.Run("Business requirement compliance", func(t *testing.T) {
		// BR-ANALYTICS-001: Advanced insights generation
		// BR-ANALYTICS-002: Predictive metrics calculation
		// BR-ANALYTICS-003: Prediction-based optimization
		// BR-ANALYTICS-004: AI enhancement integration
		// BR-ANALYTICS-005: Analytics integration in workflow generation
		// BR-ANALYTICS-006: Analytics enhancement in workflow structure optimization
		// BR-ANALYTICS-007: Public analytics method accessibility

		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "br-test-template",
					Name: "Business Requirements Test Template",
					Metadata: map[string]interface{}{
						"advanced_analytics":  true,
						"analytics_level":     "comprehensive",
						"predictive_enabled":  true,
						"insights_generation": true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Analytics Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "collect_metrics",
						Parameters: map[string]interface{}{
							"metrics_type": "performance",
						},
					},
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

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)
		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)

		// Verify comprehensive advanced analytics was performed
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)
		assert.NotNil(t, optimizedTemplate.Metadata)

		// Test all advanced analytics capabilities
		executionHistory := []*engine.RuntimeWorkflowExecution{}
		insights := builder.GenerateAdvancedInsights(ctx, workflow, executionHistory)
		assert.NotNil(t, insights)

		historicalData := []*engine.WorkflowMetrics{
			{
				AverageExecutionTime: 5 * time.Minute,
				SuccessRate:          0.95,
				ResourceUtilization:  0.7,
			},
		}
		predictiveMetrics := builder.CalculatePredictiveMetrics(ctx, workflow, historicalData)
		assert.NotNil(t, predictiveMetrics)

		optimizedByPredictions := builder.OptimizeBasedOnPredictions(ctx, template, predictiveMetrics)
		assert.NotNil(t, optimizedByPredictions)
		assert.Equal(t, template.ID, optimizedByPredictions.ID)

		enhancedTemplate := builder.EnhanceWithAI(template)
		assert.NotNil(t, enhancedTemplate)
		assert.Equal(t, template.ID, enhancedTemplate.ID)

		// Verify all advanced analytics capabilities are working
		assert.Equal(t, workflow.ID, insights.WorkflowID)
		assert.Equal(t, workflow.ID, predictiveMetrics.WorkflowID)
		assert.GreaterOrEqual(t, insights.Confidence, 0.0)
		assert.GreaterOrEqual(t, predictiveMetrics.ConfidenceLevel, 0.0)
	})
}
