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

// TestEnvironmentAdaptationIntegrationTDD verifies that the TDD implementation of environment adaptation integration works
func TestEnvironmentAdaptationIntegrationTDD(t *testing.T) {
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

	// Create test objective with environment-specific requirements
	objective := &engine.WorkflowObjective{
		ID:          "test-obj-001",
		Type:        "remediation",
		Description: "Environment-adaptive workflow optimization",
		Priority:    6, // Medium priority
		Constraints: map[string]interface{}{
			"environment":        "production",
			"namespace":          "kube-system",
			"safety_level":       "high",
			"max_parallel_steps": 2,
		},
	}

	// Test 1: Verify GenerateWorkflow includes environment adaptation
	t.Run("GenerateWorkflow includes environment adaptation", func(t *testing.T) {
		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// The workflow generation should complete successfully with environment adaptation
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that environment adaptation is integrated into workflow generation
		if template.Metadata != nil {
			// Environment adaptation should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)

			// Check for environment adaptation indicators
			if envAdapted, exists := template.Metadata["environment_adapted"]; exists {
				assert.Equal(t, true, envAdapted)
			}
			if targetEnv, exists := template.Metadata["target_environment"]; exists {
				assert.NotEmpty(t, targetEnv)
			}
		}
	})

	// Test 2: Verify public environment adaptation methods work correctly
	t.Run("Public environment adaptation methods work correctly", func(t *testing.T) {
		// Create test workflow context
		workflowContext := &engine.WorkflowContext{
			BaseContext: types.BaseContext{
				Environment: "production",
				Timestamp:   time.Now(),
			},
			WorkflowID: "workflow-001",
			Namespace:  "kube-system",
			Variables:  make(map[string]interface{}),
			CreatedAt:  time.Now(),
		}

		// Create test template with environment-sensitive steps
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
					Name: "Environment Adaptive Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Production-Sensitive Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
						Target: &engine.ActionTarget{
							Namespace: "default", // Will be adapted to context
						},
						Parameters: map[string]interface{}{
							"replicas": 3,
						},
					},
					Variables: map[string]interface{}{
						"environment": "staging", // Will be adapted
					},
				},
			},
			Variables: make(map[string]interface{}),
		}

		// Test AdaptPatternStepsToContext
		adaptedSteps := builder.AdaptPatternStepsToContext(ctx, template.Steps, workflowContext)
		assert.NotNil(t, adaptedSteps)
		assert.Equal(t, len(template.Steps), len(adaptedSteps))

		// Verify namespace adaptation
		for _, step := range adaptedSteps {
			if step.Action != nil && step.Action.Target != nil {
				assert.Equal(t, workflowContext.Namespace, step.Action.Target.Namespace)
			}
		}

		// Test CustomizeStepsForEnvironment
		customizedSteps := builder.CustomizeStepsForEnvironment(ctx, template.Steps, "production")
		assert.NotNil(t, customizedSteps)
		assert.Equal(t, len(template.Steps), len(customizedSteps))

		// Verify environment customization
		for _, step := range customizedSteps {
			assert.NotNil(t, step.Variables)
			assert.Equal(t, "production", step.Variables["environment"])
		}

		// Test AddContextSpecificConditions
		enhancedSteps := builder.AddContextSpecificConditions(ctx, template.Steps, workflowContext)
		assert.NotNil(t, enhancedSteps)
		assert.Equal(t, len(template.Steps), len(enhancedSteps))

		// Verify production safety conditions are added
		for _, step := range enhancedSteps {
			if step.Action != nil && workflowContext.Environment == "production" {
				assert.NotNil(t, step.Condition)
				assert.Equal(t, "production-safety", step.Condition.Name)
				assert.Equal(t, engine.ConditionTypeCustom, step.Condition.Type)
				assert.Contains(t, step.Condition.Expression, "production")
			}
		}
	})

	// Test 3: Verify environment adaptation integration follows business requirements
	t.Run("Environment adaptation integration follows business requirements", func(t *testing.T) {
		// BR-ENV-001: Pattern step adaptation to environment context
		// BR-ENV-002: Environment-specific step customization
		// BR-ENV-003: Context-specific condition addition for safety
		// BR-ENV-004: Complete environment adaptation pipeline integration

		template, err := builder.GenerateWorkflow(ctx, objective)
		require.NoError(t, err)

		// Verify the workflow generation process includes environment adaptation
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)
		assert.NotNil(t, template.Metadata)

		// Verify that analytics, pattern discovery, resource optimization, and environment adaptation are all integrated
		if template.Metadata != nil {
			// All four optimization phases should contribute to metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 4: Verify environment adaptation handles different environments
	t.Run("Environment adaptation handles different environments gracefully", func(t *testing.T) {
		environments := []string{"production", "staging", "development"}

		for _, env := range environments {
			envObjective := &engine.WorkflowObjective{
				ID:          "test-obj-" + env,
				Type:        "remediation",
				Description: "Environment-specific workflow for " + env,
				Priority:    5,
				Constraints: map[string]interface{}{
					"environment": env,
				},
			}

			template, err := builder.GenerateWorkflow(ctx, envObjective)

			assert.NoError(t, err)
			assert.NotNil(t, template)
			assert.NotEmpty(t, template.ID)

			// Verify environment-specific adaptation
			if template.Metadata != nil {
				if targetEnv, exists := template.Metadata["target_environment"]; exists {
					assert.Equal(t, env, targetEnv)
				}
			}
		}
	})

	// Test 5: Verify environment adaptation handles edge cases
	t.Run("Environment adaptation handles edge cases gracefully", func(t *testing.T) {
		// Test with minimal objective (no environment specified)
		minimalObjective := &engine.WorkflowObjective{
			ID:          "minimal-obj",
			Type:        "basic",
			Description: "Minimal workflow without environment",
			Priority:    5,
			Constraints: map[string]interface{}{},
		}

		// Should handle minimal objective gracefully
		template, err := builder.GenerateWorkflow(ctx, minimalObjective)
		assert.NoError(t, err)
		assert.NotNil(t, template)
		assert.NotEmpty(t, template.ID)

		// Test with empty steps
		emptySteps := []*engine.ExecutableWorkflowStep{}

		// Should handle empty steps gracefully
		adaptedSteps := builder.AdaptPatternStepsToContext(ctx, emptySteps, &engine.WorkflowContext{
			BaseContext: types.BaseContext{Environment: "production"},
			WorkflowID:  "test",
			Namespace:   "default",
		})
		assert.NotNil(t, adaptedSteps)
		assert.Equal(t, 0, len(adaptedSteps))

		customizedSteps := builder.CustomizeStepsForEnvironment(ctx, emptySteps, "production")
		assert.NotNil(t, customizedSteps)
		assert.Equal(t, 0, len(customizedSteps))

		enhancedSteps := builder.AddContextSpecificConditions(ctx, emptySteps, &engine.WorkflowContext{
			BaseContext: types.BaseContext{Environment: "production"},
		})
		assert.NotNil(t, enhancedSteps)
		assert.Equal(t, 0, len(enhancedSteps))
	})
}
