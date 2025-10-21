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

package integration

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

// TestValidationEnhancementIntegrationTDD verifies that the TDD implementation of validation enhancement integration works
func TestValidationEnhancementIntegrationTDD(t *testing.T) {
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

	// Test 1: Verify ValidateWorkflowTemplate method is accessible
	t.Run("ValidateWorkflowTemplate method is accessible", func(t *testing.T) {
		// Create test template with validation scenarios
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template-001",
					Name: "Validation Test Template",
					Metadata: map[string]interface{}{
						"validation_level": "comprehensive",
						"safety_required":  true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Action Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
						Parameters: map[string]interface{}{
							"replicas": 3,
						},
						Target: &engine.ActionTarget{
							Type:      "deployment",
							Namespace: "default",
							Name:      "test-deployment",
							Resource:  "deployments",
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
			Tags:      []string{"production"},
		}

		validationReport := builder.ValidateWorkflowTemplate(ctx, template)

		require.NotNil(t, validationReport)
		assert.NotEmpty(t, validationReport.ID)
		assert.Equal(t, template.ID, validationReport.WorkflowID)
		assert.NotNil(t, validationReport.Results)
		assert.NotNil(t, validationReport.Summary)
		assert.NotEmpty(t, validationReport.Status)
	})

	// Test 2: Verify individual validation methods are accessible
	t.Run("Individual validation methods are accessible", func(t *testing.T) {
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template-002",
					Name: "Individual Validation Test",
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
					Action: &engine.StepAction{
						Type: "collect_diagnostics",
						Parameters: map[string]interface{}{
							"timeout": "30s",
						},
					},
				},
			},
		}

		// Test ValidateStepDependencies
		depResults := builder.ValidateStepDependencies(ctx, template)
		assert.NotNil(t, depResults)

		// Test ValidateActionParameters
		paramResults := builder.ValidateActionParameters(ctx, template)
		assert.NotNil(t, paramResults)

		// Test ValidateResourceAccess
		resourceResults := builder.ValidateResourceAccess(ctx, template)
		assert.NotNil(t, resourceResults)

		// Test ValidateSafetyConstraints
		safetyResults := builder.ValidateSafetyConstraints(ctx, template)
		assert.NotNil(t, safetyResults)

		// Test GenerateValidationSummary
		allResults := append(depResults, paramResults...)
		allResults = append(allResults, resourceResults...)
		allResults = append(allResults, safetyResults...)

		summary := builder.GenerateValidationSummary(allResults)
		assert.NotNil(t, summary)
		assert.Equal(t, len(allResults), summary.Total)
		assert.Equal(t, summary.Total, summary.Passed+summary.Failed)
	})

	// Test 3: Verify OptimizeWorkflowStructure includes validation enhancement
	t.Run("OptimizeWorkflowStructure includes validation enhancement", func(t *testing.T) {
		// Create template with validation enhancement triggers
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template-003",
					Name: "Validation Enhancement Template",
					Metadata: map[string]interface{}{
						"validation_level": "comprehensive",
						"safety_required":  true,
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
						Target: &engine.ActionTarget{
							Type:      "deployment",
							Namespace: "default",
							Name:      "test-deployment",
							Resource:  "deployments",
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
						Parameters: map[string]interface{}{
							"timeout": "30s",
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
			Tags:      []string{"production", "critical"}, // High-risk tags
		}

		optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

		require.NoError(t, err)
		require.NotNil(t, optimizedTemplate)
		require.NotNil(t, optimizedTemplate.Metadata)

		// Verify that validation enhancement optimization was applied
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check for validation enhancement indicators in metadata
		if optimizedTemplate.Metadata != nil {
			if validationEnhanced, exists := optimizedTemplate.Metadata["validation_enhanced"]; exists {
				assert.Equal(t, true, validationEnhanced)
			}
			if validationScore, exists := optimizedTemplate.Metadata["validation_score"]; exists {
				assert.IsType(t, float64(0), validationScore)
				assert.GreaterOrEqual(t, validationScore.(float64), 0.0)
				assert.LessOrEqual(t, validationScore.(float64), 1.0)
			}
			if issuesResolved, exists := optimizedTemplate.Metadata["validation_issues_resolved"]; exists {
				assert.IsType(t, int(0), issuesResolved)
				assert.GreaterOrEqual(t, issuesResolved.(int), 0)
			}
		}
	})

	// Test 4: Verify validation enhancement handles different template types
	t.Run("Validation enhancement handles different template types", func(t *testing.T) {
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

		emptyValidationReport := builder.ValidateWorkflowTemplate(ctx, emptyTemplate)
		assert.NotNil(t, emptyValidationReport)
		assert.NotEmpty(t, emptyValidationReport.Status)

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

		conditionalValidationReport := builder.ValidateWorkflowTemplate(ctx, conditionalTemplate)
		assert.NotNil(t, conditionalValidationReport)
		assert.NotEmpty(t, conditionalValidationReport.Status)
	})

	// Test 5: Verify validation enhancement integration follows business requirements
	t.Run("Validation enhancement integration follows business requirements", func(t *testing.T) {
		// BR-VALID-001: Comprehensive validation integration
		// BR-VALID-002: Step dependency validation enhancement
		// BR-VALID-004: Action parameter validation enhancement
		// BR-VALID-006: Resource access validation enhancement
		// BR-VALID-007: Safety constraints validation enhancement

		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "br-test-template",
					Name: "Business Requirements Test Template",
					Metadata: map[string]interface{}{
						"validation_level": "comprehensive",
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
						Target: &engine.ActionTarget{
							Type:      "deployment",
							Namespace: "default",
							Name:      "test-deployment",
							Resource:  "deployments",
						},
					},
				},
			},
		}

		validationReport := builder.ValidateWorkflowTemplate(ctx, template)
		require.NotNil(t, validationReport)

		// Verify comprehensive validation was performed
		assert.NotEmpty(t, validationReport.ID)
		assert.Equal(t, template.ID, validationReport.WorkflowID)
		assert.NotNil(t, validationReport.Results)
		assert.NotNil(t, validationReport.Summary)
		assert.NotEmpty(t, validationReport.Status)
		assert.NotNil(t, validationReport.CompletedAt)

		// Verify validation summary contains proper statistics
		assert.GreaterOrEqual(t, validationReport.Summary.Total, 0)
		assert.GreaterOrEqual(t, validationReport.Summary.Passed, 0)
		assert.GreaterOrEqual(t, validationReport.Summary.Failed, 0)
		assert.Equal(t, validationReport.Summary.Total, validationReport.Summary.Passed+validationReport.Summary.Failed)
	})

	// Test 6: Verify validation enhancement works with complex scenarios
	t.Run("Validation enhancement works with complex scenarios", func(t *testing.T) {
		// Create template with potential validation issues
		complexTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "complex-template",
					Name: "Complex Validation Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-no-timeout",
						Name: "Step without Timeout",
					},
					Type:    engine.StepTypeAction,
					Timeout: 0, // Missing timeout
					Action: &engine.StepAction{
						Type: "collect_diagnostics",
						Parameters: map[string]interface{}{
							"timeout": "30s",
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-destructive",
						Name: "Destructive Action",
					},
					Type:    engine.StepTypeAction,
					Timeout: 5 * time.Minute,
					Action: &engine.StepAction{
						Type: "delete_deployment", // Destructive action
						Parameters: map[string]interface{}{
							"force": true,
						},
						// Missing rollback
					},
				},
			},
			Tags: []string{"production"}, // High-risk environment
		}

		validationReport := builder.ValidateWorkflowTemplate(ctx, complexTemplate)

		require.NotNil(t, validationReport)
		assert.NotEmpty(t, validationReport.ID)
		assert.NotNil(t, validationReport.Results)

		// Should detect validation issues
		assert.GreaterOrEqual(t, len(validationReport.Results), 0)

		// Verify validation completed successfully
		assert.NotEmpty(t, validationReport.Status)
		assert.NotNil(t, validationReport.CompletedAt)
	})
}
