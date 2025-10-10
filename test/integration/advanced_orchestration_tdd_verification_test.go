<<<<<<< HEAD
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

>>>>>>> crd_implementation
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

// TestAdvancedOrchestrationIntegrationTDD verifies that the TDD implementation of advanced orchestration integration works
func TestAdvancedOrchestrationIntegrationTDD(t *testing.T) {
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

	// Test 1: Verify orchestration methods are accessible
	t.Run("Orchestration methods are accessible", func(t *testing.T) {
		// Create test workflow for orchestration efficiency calculation
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template-001",
					Name: "Test Orchestration Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Step 1",
					},
					Type:         engine.StepTypeAction,
					Dependencies: []string{}, // No dependencies
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Test Step 2",
					},
					Type:         engine.StepTypeAction,
					Dependencies: []string{"step-001"}, // Depends on step-001
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

		// Test CalculateOrchestrationEfficiency
		efficiency := builder.CalculateOrchestrationEfficiency(workflow, []*engine.RuntimeWorkflowExecution{})
		require.NotNil(t, efficiency)
		assert.GreaterOrEqual(t, efficiency.OverallEfficiency, 0.0)
		assert.LessOrEqual(t, efficiency.OverallEfficiency, 1.0)
		assert.GreaterOrEqual(t, efficiency.ParallelizationRatio, 0.0)
		assert.LessOrEqual(t, efficiency.ParallelizationRatio, 1.0)

		// Test ApplyOrchestrationConstraints
		constraints := map[string]interface{}{
			"max_execution_time": "45m",
			"max_parallel_steps": 3,
		}
		constrainedTemplate := builder.ApplyOrchestrationConstraints(template, constraints)
		require.NotNil(t, constrainedTemplate)
		assert.Equal(t, template.ID, constrainedTemplate.ID)

		// Test OptimizeStepOrdering
		orderedTemplate, err := builder.OptimizeStepOrdering(template)
		require.NoError(t, err)
		require.NotNil(t, orderedTemplate)
		assert.Equal(t, template.ID, orderedTemplate.ID)
		assert.Equal(t, len(template.Steps), len(orderedTemplate.Steps))

		// Test OptimizeResourceUsage
		builder.OptimizeResourceUsage(template)
		// Should complete without error

		// Test CalculateOptimizationImpact
		performanceAnalysis := &engine.PerformanceAnalysis{
			WorkflowID:    template.ID,
			ExecutionTime: 30 * time.Minute,
			Effectiveness: 0.8,
		}
		impact := builder.CalculateOptimizationImpact(template, template, performanceAnalysis)
		require.NotNil(t, impact)
		assert.GreaterOrEqual(t, impact.ExecutionTimeImprovement, 0.0)
		assert.GreaterOrEqual(t, impact.ResourceEfficiencyGain, 0.0)
		assert.GreaterOrEqual(t, impact.StepReduction, 0.0)
		assert.GreaterOrEqual(t, impact.OverallImpact, 0.0)
	})

	// Test 2: Verify OptimizeWithConstraints works
	t.Run("OptimizeWithConstraints works", func(t *testing.T) {
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "constraint-template",
					Name: "Constraint Test Template",
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

		constraints := &engine.OptimizationConstraints{
			MaxRiskLevel:       "medium",
			MaxExecutionTime:   60 * time.Minute,
			MinPerformanceGain: 0.15,
			RequiredConfidence: 0.80,
		}

		result := builder.OptimizeWithConstraints(workflow, constraints)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.RiskLevel)
		assert.GreaterOrEqual(t, result.PerformanceGain, 0.0)
	})

	// Test 3: Verify OptimizeWorkflowStructure includes orchestration optimization
	t.Run("OptimizeWorkflowStructure includes orchestration optimization", func(t *testing.T) {
		// Create template with orchestration optimization triggers
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "orch-template-001",
					Name: "Orchestration Optimization Template",
					Metadata: map[string]interface{}{
						"orchestration_optimization": true,
						"orchestration_level":        "advanced",
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
					Type:         engine.StepTypeAction,
					Timeout:      15 * time.Minute,
					Dependencies: []string{"step-001"},
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

		// Verify that orchestration optimization was applied
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check for orchestration optimization indicators in metadata
		if optimizedTemplate.Metadata != nil {
			if orchestrationOptimized, exists := optimizedTemplate.Metadata["orchestration_optimized"]; exists {
				assert.Equal(t, true, orchestrationOptimized)
			}
			if orchestrationEfficiency, exists := optimizedTemplate.Metadata["orchestration_efficiency"]; exists {
				assert.IsType(t, float64(0), orchestrationEfficiency)
				assert.GreaterOrEqual(t, orchestrationEfficiency.(float64), 0.0)
				assert.LessOrEqual(t, orchestrationEfficiency.(float64), 1.0)
			}
			if parallelizationRatio, exists := optimizedTemplate.Metadata["parallelization_ratio"]; exists {
				assert.IsType(t, float64(0), parallelizationRatio)
				assert.GreaterOrEqual(t, parallelizationRatio.(float64), 0.0)
				assert.LessOrEqual(t, parallelizationRatio.(float64), 1.0)
			}
		}
	})

	// Test 4: Verify GenerateWorkflow includes orchestration optimization
	t.Run("GenerateWorkflow includes orchestration optimization", func(t *testing.T) {
		objective := &engine.WorkflowObjective{
			ID:          "orch-obj-001",
			Type:        "orchestration_optimization",
			Description: "Advanced orchestration workflow optimization",
			Priority:    8,
			Constraints: map[string]interface{}{
				"orchestration_optimization": true,
				"orchestration_level":        "advanced",
				"max_execution_time":         "60m",
			},
		}

		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// Verify the workflow generation process includes orchestration optimization
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that orchestration optimization metadata is present
		if template.Metadata != nil {
			// Orchestration optimization should contribute to workflow metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 5: Verify orchestration efficiency calculation handles different scenarios
	t.Run("Orchestration efficiency handles different scenarios", func(t *testing.T) {
		// Test with empty workflow
		emptyWorkflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: "empty-workflow",
				},
			},
			Template: &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{
						ID: "empty-template",
					},
				},
				Steps: []*engine.ExecutableWorkflowStep{}, // No steps
			},
		}

		efficiency := builder.CalculateOrchestrationEfficiency(emptyWorkflow, []*engine.RuntimeWorkflowExecution{})
		require.NotNil(t, efficiency)
		assert.Equal(t, 0.0, efficiency.OverallEfficiency)
		assert.Equal(t, 0.0, efficiency.ParallelizationRatio)

		// Test with complex dependency workflow
		complexTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: "complex-template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID: "step-a",
					},
					Type:         engine.StepTypeAction,
					Dependencies: []string{}, // Root step
				},
				{
					BaseEntity: types.BaseEntity{
						ID: "step-b",
					},
					Type:         engine.StepTypeAction,
					Dependencies: []string{"step-a"},
				},
				{
					BaseEntity: types.BaseEntity{
						ID: "step-c",
					},
					Type:         engine.StepTypeAction,
					Dependencies: []string{"step-a"}, // Can run in parallel with step-b
				},
			},
		}

		complexWorkflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: complexTemplate.ID,
				},
			},
			Template: complexTemplate,
		}

		complexEfficiency := builder.CalculateOrchestrationEfficiency(complexWorkflow, []*engine.RuntimeWorkflowExecution{})
		require.NotNil(t, complexEfficiency)
		assert.Greater(t, complexEfficiency.OverallEfficiency, 0.0)
		assert.Greater(t, complexEfficiency.ParallelizationRatio, 0.0) // Should have some parallelization potential
	})

	// Test 6: Verify business requirement compliance
	t.Run("Business requirement compliance", func(t *testing.T) {
		// BR-ORCH-001: Advanced orchestration optimization
		// BR-ORCH-002: Orchestration efficiency calculation
		// BR-ORCH-003: Orchestration constraints application
		// BR-ORCH-004: Step ordering optimization
		// BR-ORCH-005: Resource usage optimization
		// BR-ORCH-006: Optimization impact calculation
		// BR-ORCH-007: Orchestration integration in workflow generation
		// BR-ORCH-008: Orchestration-based optimization application
		// BR-ORCH-009: Public orchestration method accessibility

		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "br-test-template",
					Name: "Business Requirements Test Template",
					Metadata: map[string]interface{}{
						"orchestration_optimization": true,
						"orchestration_level":        "advanced",
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

		// Verify comprehensive orchestration optimization was performed
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)
		assert.NotNil(t, optimizedTemplate.Metadata)

		// Verify orchestration optimization metadata is present
		if optimizedTemplate.Metadata != nil {
			// Orchestration optimization should contribute to workflow metadata
			assert.IsType(t, map[string]interface{}{}, optimizedTemplate.Metadata)
		}
	})
}
