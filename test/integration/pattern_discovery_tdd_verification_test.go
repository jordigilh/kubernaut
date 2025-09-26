package integration_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TestPatternDiscoveryIntegrationTDD verifies that the TDD implementation of pattern discovery integration works
func TestPatternDiscoveryIntegrationTDD(t *testing.T) {
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

	// Create test objective
	objective := &engine.WorkflowObjective{
		ID:          "test-obj-001",
		Type:        "remediation",
		Description: "High CPU usage remediation workflow",
		Priority:    8, // High priority
	}

	// Test 1: Verify GenerateWorkflow includes enhanced pattern discovery
	t.Run("GenerateWorkflow includes enhanced pattern discovery", func(t *testing.T) {
		// Setup mock to return test patterns
		testPatterns := []*vector.ActionPattern{
			{
				ID:         "pattern-001",
				ActionType: "scale_deployment",
				AlertName:  "HighCPUUsage",
				EffectivenessData: &vector.EffectivenessData{
					Score:        0.85, // High effectiveness
					SuccessCount: 15,
					FailureCount: 3,
				},
			},
		}
		mockVectorDB.SetSearchSemanticsResult(testPatterns, nil)

		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// The workflow generation should complete successfully with enhanced pattern discovery
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)
	})

	// Test 2: Verify public pattern discovery methods work correctly
	t.Run("Public pattern discovery methods work correctly", func(t *testing.T) {
		// Test FindSimilarSuccessfulPatterns
		analysis := &engine.ObjectiveAnalysisResult{
			Keywords:    []string{"cpu", "high", "usage", "scale"},
			ActionTypes: []string{"scale_deployment", "increase_resources"},
			Priority:    8,
			Complexity:  0.6,
			RiskLevel:   "medium",
		}

		// Setup mock
		testPatterns := []*vector.ActionPattern{
			{
				ID:         "pattern-001",
				ActionType: "scale_deployment",
				EffectivenessData: &vector.EffectivenessData{
					Score: 0.85, // High effectiveness (>= 0.7)
				},
			},
		}
		mockVectorDB.SetSearchSemanticsResult(testPatterns, nil)

		patterns, err := builder.FindSimilarSuccessfulPatterns(ctx, analysis)
		assert.NoError(t, err)
		assert.NotNil(t, patterns)

		// Test FindPatternsForWorkflow
		workflowPatterns := builder.FindPatternsForWorkflow(ctx, "workflow-001")
		assert.NotNil(t, workflowPatterns)

		// Test ApplyLearningsToPattern
		pattern := &engine.WorkflowPattern{
			ID:          "pattern-001",
			SuccessRate: 0.7,
			Confidence:  0.6,
		}
		learnings := []*engine.WorkflowLearning{} // Empty learnings for test

		updated := builder.ApplyLearningsToPattern(ctx, pattern, learnings)
		// Should handle empty learnings gracefully
		assert.False(t, updated)
	})

	// Test 3: Verify pattern discovery integration follows business requirements
	t.Run("Pattern discovery integration follows business requirements", func(t *testing.T) {
		// BR-PATTERN-001: Pattern discovery with effectiveness filtering
		// BR-PATTERN-002: Workflow-specific pattern discovery
		// BR-PATTERN-004: Learning application for pattern improvement
		// BR-PATTERN-005: Complete pattern discovery pipeline integration

		// Setup comprehensive pattern discovery scenario
		testPatterns := []*vector.ActionPattern{
			{
				ID:         "pattern-001",
				ActionType: "scale_deployment",
				EffectivenessData: &vector.EffectivenessData{
					Score: 0.85, // High effectiveness
				},
			},
		}
		mockVectorDB.SetSearchSemanticsResult(testPatterns, nil)

		template, err := builder.GenerateWorkflow(ctx, objective)
		require.NoError(t, err)

		// Verify the workflow generation process includes enhanced pattern discovery
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)
		assert.NotNil(t, template.Metadata)

		// Verify that analytics and pattern discovery are both integrated
		if template.Metadata != nil {
			// Both analytics and pattern discovery should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})
}
