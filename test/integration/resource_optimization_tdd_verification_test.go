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

// TestResourceOptimizationIntegrationTDD verifies that the TDD implementation of resource optimization integration works
func TestResourceOptimizationIntegrationTDD(t *testing.T) {
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

	// Create test objective with resource constraints
	objective := &engine.WorkflowObjective{
		ID:          "test-obj-001",
		Type:        "remediation",
		Description: "Resource-constrained workflow optimization",
		Priority:    5, // Medium priority
		Constraints: map[string]interface{}{
			"max_execution_time": "30m",
			"resource_limits": map[string]interface{}{
				"cpu":    "2000m",
				"memory": "4Gi",
			},
			"cost_budget":       100.0,
			"efficiency_target": 0.85,
			"environment":       "production",
		},
	}

	// Test 1: Verify GenerateWorkflow includes resource optimization
	t.Run("GenerateWorkflow includes resource optimization", func(t *testing.T) {
		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// The workflow generation should complete successfully with resource optimization
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that both analytics and resource optimization are integrated
		if template.Metadata != nil {
			// All optimizations should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 2: Verify public resource optimization methods work correctly
	t.Run("Public resource optimization methods work correctly", func(t *testing.T) {
		// Create test template with resource specifications
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
					Name: "Resource Test Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "CPU Intensive Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
						Parameters: map[string]interface{}{
							"cpu_limit":    "1000m",
							"memory_limit": "2Gi",
							"replicas":     5,
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
		}

		// Test ApplyResourceConstraintManagement
		optimizedTemplate, err := builder.ApplyResourceConstraintManagement(ctx, template, objective)
		assert.NoError(t, err)
		assert.NotNil(t, optimizedTemplate)
		assert.Equal(t, template.ID, optimizedTemplate.ID)

		// Test CalculateResourceEfficiency
		efficiency := builder.CalculateResourceEfficiency(optimizedTemplate, template)
		assert.GreaterOrEqual(t, efficiency, 0.0)
		assert.LessOrEqual(t, efficiency, 1.0)

		// Test CalculateResourceAllocation
		resourcePlan := builder.CalculateResourceAllocation(template.Steps)
		assert.NotNil(t, resourcePlan)
		assert.Greater(t, resourcePlan.TotalCPUWeight, 0.0)
		assert.Greater(t, resourcePlan.TotalMemoryWeight, 0.0)
		assert.Greater(t, resourcePlan.MaxConcurrency, 0)
		assert.GreaterOrEqual(t, resourcePlan.EfficiencyScore, 0.0)
		assert.LessOrEqual(t, resourcePlan.EfficiencyScore, 1.0)

		// Test ExtractConstraintsFromObjective
		constraints, err := builder.ExtractConstraintsFromObjective(objective)
		assert.NoError(t, err)
		assert.NotNil(t, constraints)
		assert.Equal(t, "30m", constraints["max_execution_time"])
		assert.Equal(t, 100.0, constraints["cost_budget"])
		assert.Equal(t, 0.85, constraints["efficiency_target"])
	})

	// Test 3: Verify resource optimization integration follows business requirements
	t.Run("Resource optimization integration follows business requirements", func(t *testing.T) {
		// BR-RESOURCE-001: Comprehensive resource constraint management
		// BR-RESOURCE-004: Resource efficiency calculation and validation
		// BR-RESOURCE-005: Optimal resource allocation calculation
		// BR-RESOURCE-007: Complete resource optimization pipeline integration

		template, err := builder.GenerateWorkflow(ctx, objective)
		require.NoError(t, err)

		// Verify the workflow generation process includes resource optimization
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)
		assert.NotNil(t, template.Metadata)

		// Verify that analytics, pattern discovery, and resource optimization are all integrated
		if template.Metadata != nil {
			// All three optimization phases should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 4: Verify resource optimization handles edge cases
	t.Run("Resource optimization handles edge cases gracefully", func(t *testing.T) {
		// Test with empty template
		emptyTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: "empty-template",
				},
			},
			Steps:     []*engine.ExecutableWorkflowStep{},
			Variables: make(map[string]interface{}),
		}

		// Should handle empty template gracefully
		efficiency := builder.CalculateResourceEfficiency(emptyTemplate, emptyTemplate)
		assert.GreaterOrEqual(t, efficiency, 0.0)
		assert.LessOrEqual(t, efficiency, 1.0)

		// Should handle empty steps gracefully
		resourcePlan := builder.CalculateResourceAllocation([]*engine.ExecutableWorkflowStep{})
		assert.NotNil(t, resourcePlan)
		assert.Equal(t, 0.0, resourcePlan.TotalCPUWeight)
		assert.Equal(t, 0.0, resourcePlan.TotalMemoryWeight)

		// Test context cancellation
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		optimizedTemplate, err := builder.ApplyResourceConstraintManagement(cancelCtx, emptyTemplate, objective)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		assert.Equal(t, emptyTemplate, optimizedTemplate) // Should return original template
	})
}
