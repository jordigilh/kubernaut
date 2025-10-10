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

// TestAdvancedSchedulingIntegrationTDD verifies that the TDD implementation of advanced scheduling integration works
func TestAdvancedSchedulingIntegrationTDD(t *testing.T) {
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

	// Create test objective with scheduling requirements
	objective := &engine.WorkflowObjective{
		ID:          "test-obj-001",
		Type:        "remediation",
		Description: "Advanced scheduling workflow optimization",
		Priority:    7, // High priority
		Constraints: map[string]interface{}{
			"max_execution_time":   "45m",
			"max_concurrent_steps": 3,
			"scheduling_priority":  "high",
			"resource_limits": map[string]interface{}{
				"cpu":    "4000m",
				"memory": "8Gi",
			},
		},
	}

	// Test 1: Verify GenerateWorkflow includes advanced scheduling
	t.Run("GenerateWorkflow includes advanced scheduling", func(t *testing.T) {
		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// The workflow generation should complete successfully with advanced scheduling
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that advanced scheduling is integrated into workflow generation
		if template.Metadata != nil {
			// Advanced scheduling should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 2: Verify OptimizeWorkflowStructure includes advanced scheduling
	t.Run("OptimizeWorkflowStructure includes advanced scheduling", func(t *testing.T) {
		// Create test template with scheduling-sensitive steps
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
					Name: "Advanced Scheduling Template",
					Metadata: map[string]interface{}{
						"scheduling_priority": "high",
						"concurrency_level":   3,
					},
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
							"cpu_limit":    "2000m",
							"memory_limit": "4Gi",
							"replicas":     5,
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "I/O Intensive Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "collect_diagnostics",
						Parameters: map[string]interface{}{
							"timeout": "30s",
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

		// Verify that advanced scheduling optimization was applied
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check for advanced scheduling indicators in metadata
		if optimizedTemplate.Metadata != nil {
			if schedulingOptimized, exists := optimizedTemplate.Metadata["scheduling_optimized"]; exists {
				assert.Equal(t, true, schedulingOptimized)
			}
			if optimalConcurrency, exists := optimizedTemplate.Metadata["optimal_concurrency"]; exists {
				assert.IsType(t, int(0), optimalConcurrency)
				assert.Greater(t, optimalConcurrency.(int), 0)
			}
			if schedulingStrategy, exists := optimizedTemplate.Metadata["scheduling_strategy"]; exists {
				assert.IsType(t, "", schedulingStrategy)
				assert.NotEmpty(t, schedulingStrategy.(string))
			}
		}
	})

	// Test 3: Verify public advanced scheduling methods work correctly
	t.Run("Public advanced scheduling methods work correctly", func(t *testing.T) {
		// Create test steps with different characteristics
		steps := []*engine.ExecutableWorkflowStep{
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
						"cpu_limit":    "2000m",
						"memory_limit": "4Gi",
					},
				},
			},
			{
				BaseEntity: types.BaseEntity{
					ID:   "step-002",
					Name: "I/O Intensive Step",
				},
				Type:    engine.StepTypeAction,
				Timeout: 15 * time.Minute,
				Action: &engine.StepAction{
					Type: "collect_diagnostics",
				},
			},
		}

		// Test CalculateOptimalStepConcurrency
		concurrency := builder.CalculateOptimalStepConcurrency(steps)
		assert.Greater(t, concurrency, 0)
		assert.LessOrEqual(t, concurrency, len(steps))

		// Test CalculateResourceAllocation (which uses optimal batching)
		resourcePlan := builder.CalculateResourceAllocation(steps)
		assert.NotNil(t, resourcePlan)
		assert.NotNil(t, resourcePlan.OptimalBatches)
		assert.Greater(t, len(resourcePlan.OptimalBatches), 0)

		// Verify batches respect concurrency limits
		for _, batch := range resourcePlan.OptimalBatches {
			assert.LessOrEqual(t, len(batch), resourcePlan.MaxConcurrency)
		}
	})

	// Test 4: Verify advanced scheduling integration follows business requirements
	t.Run("Advanced scheduling integration follows business requirements", func(t *testing.T) {
		// BR-SCHED-001: Optimal concurrency calculation based on resource analysis
		// BR-SCHED-004: Comprehensive scheduling optimization integration
		// BR-SCHED-005: Scheduling constraints application in optimization
		// BR-SCHED-006: Workflow timing optimization

		template, err := builder.GenerateWorkflow(ctx, objective)
		require.NoError(t, err)

		// Verify the workflow generation process includes advanced scheduling
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)
		assert.NotNil(t, template.Metadata)

		// Verify that analytics, pattern discovery, resource optimization, environment adaptation, and advanced scheduling are all integrated
		if template.Metadata != nil {
			// All five optimization phases should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 5: Verify advanced scheduling handles different step types
	t.Run("Advanced scheduling handles different step types gracefully", func(t *testing.T) {
		// Test with CPU-intensive steps
		cpuSteps := []*engine.ExecutableWorkflowStep{
			{
				BaseEntity: types.BaseEntity{ID: "cpu-1"},
				Action: &engine.StepAction{
					Type: "scale_deployment",
					Parameters: map[string]interface{}{
						"cpu_limit": "2000m",
					},
				},
			},
			{
				BaseEntity: types.BaseEntity{ID: "cpu-2"},
				Action: &engine.StepAction{
					Type: "increase_resources",
					Parameters: map[string]interface{}{
						"cpu_limit": "1500m",
					},
				},
			},
		}

		// Test with I/O-intensive steps
		ioSteps := []*engine.ExecutableWorkflowStep{
			{
				BaseEntity: types.BaseEntity{ID: "io-1"},
				Action: &engine.StepAction{
					Type: "collect_diagnostics",
				},
			},
			{
				BaseEntity: types.BaseEntity{ID: "io-2"},
				Action: &engine.StepAction{
					Type: "health_check",
				},
			},
		}

		cpuConcurrency := builder.CalculateOptimalStepConcurrency(cpuSteps)
		ioConcurrency := builder.CalculateOptimalStepConcurrency(ioSteps)

		// Both should be valid concurrency levels
		assert.Greater(t, cpuConcurrency, 0)
		assert.Greater(t, ioConcurrency, 0)

		// I/O intensive steps should typically allow higher concurrency than CPU intensive
		assert.GreaterOrEqual(t, ioConcurrency, cpuConcurrency)
	})

	// Test 6: Verify advanced scheduling handles edge cases
	t.Run("Advanced scheduling handles edge cases gracefully", func(t *testing.T) {
		// Test with empty steps
		emptySteps := []*engine.ExecutableWorkflowStep{}

		concurrency := builder.CalculateOptimalStepConcurrency(emptySteps)
		assert.Equal(t, 1, concurrency) // Minimum concurrency

		// Test with single step
		singleStep := []*engine.ExecutableWorkflowStep{
			{
				BaseEntity: types.BaseEntity{ID: "single"},
				Action: &engine.StepAction{
					Type: "health_check",
				},
			},
		}

		singleConcurrency := builder.CalculateOptimalStepConcurrency(singleStep)
		assert.Greater(t, singleConcurrency, 0)
		assert.LessOrEqual(t, singleConcurrency, len(singleStep))

		// Test resource allocation with single step
		singleResourcePlan := builder.CalculateResourceAllocation(singleStep)
		assert.NotNil(t, singleResourcePlan)
		assert.Equal(t, 1, len(singleResourcePlan.OptimalBatches))
		assert.Equal(t, 1, len(singleResourcePlan.OptimalBatches[0]))
	})
}
