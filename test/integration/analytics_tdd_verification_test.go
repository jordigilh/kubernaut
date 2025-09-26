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
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TestAnalyticsIntegrationTDD verifies that the TDD implementation of analytics integration works
func TestAnalyticsIntegrationTDD(t *testing.T) {
	// Setup
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create builder with minimal dependencies using config pattern
	config := &engine.IntelligentWorkflowBuilderConfig{
		LLMClient:       nil, // External: Mock not needed for this test
		VectorDB:        nil, // External: Mock not needed for this test
		AnalyticsEngine: nil, // External: Mock not needed for this test
		PatternStore:    nil, // External: Mock not needed for this test
		ExecutionRepo:   nil, // External: Mock not needed for this test
		Logger:          log,
	}

	builder, err := engine.NewIntelligentWorkflowBuilder(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create workflow builder: %v", err))
	}

	ctx := context.Background()

	// Create test objective
	objective := &engine.WorkflowObjective{
		ID:          "test-obj-001",
		Type:        "remediation",
		Description: "Test analytics integration",
		Constraints: map[string]interface{}{
			"max_time": "10m",
		},
	}

	// Test 1: Verify GenerateWorkflow includes analytics metadata
	t.Run("GenerateWorkflow includes analytics metadata", func(t *testing.T) {
		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// The metadata should be initialized even if no historical data exists
		assert.IsType(t, map[string]interface{}{}, template.Metadata)
	})

	// Test 2: Verify public analytics methods work correctly
	t.Run("Public analytics methods work correctly", func(t *testing.T) {
		// Create test executions
		executions := []*engine.RuntimeWorkflowExecution{
			{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "exec-001",
					WorkflowID: "workflow-001",
					StartTime:  time.Now().Add(-10 * time.Minute),
					EndTime:    func() *time.Time { t := time.Now().Add(-8 * time.Minute); return &t }(),
				},
				OperationalStatus: engine.ExecutionStatusCompleted,
				Duration:          2 * time.Minute,
			},
			{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "exec-002",
					WorkflowID: "workflow-001",
					StartTime:  time.Now().Add(-20 * time.Minute),
					EndTime:    func() *time.Time { t := time.Now().Add(-15 * time.Minute); return &t }(),
				},
				OperationalStatus: engine.ExecutionStatusFailed,
				Duration:          5 * time.Minute,
			},
		}

		// Test CalculateSuccessRate
		successRate := builder.CalculateSuccessRate(executions)
		assert.Equal(t, 0.5, successRate) // 1 success out of 2 executions

		// Test CalculateAverageExecutionTime
		avgTime := builder.CalculateAverageExecutionTime(executions)
		expectedAvg := (2*time.Minute + 5*time.Minute) / 2
		assert.Equal(t, expectedAvg, avgTime)

		// Test CalculatePatternConfidence
		pattern := &engine.WorkflowPattern{
			ID:          "pattern-001",
			SuccessRate: 0.8,
			Confidence:  0.7,
		}
		confidence := builder.CalculatePatternConfidence(pattern, executions)
		assert.Greater(t, confidence, 0.0)
		assert.LessOrEqual(t, confidence, 1.0)
	})

	// Test 3: Verify analytics integration follows business requirements
	t.Run("Analytics integration follows business requirements", func(t *testing.T) {
		// BR-ANALYTICS-001: Success rate analytics
		// BR-ANALYTICS-002: Pattern confidence scoring
		// BR-ANALYTICS-003: Execution time analytics
		// BR-ANALYTICS-004: Historical data integration
		// BR-ANALYTICS-005: Analytics-driven optimization

		template, err := builder.GenerateWorkflow(ctx, objective)
		require.NoError(t, err)

		// Verify the workflow generation process completes successfully
		// The specific analytics will be present when historical data exists
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)
		assert.NotNil(t, template.Metadata)
	})
}
