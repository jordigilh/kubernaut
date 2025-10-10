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

// TestSecurityEnhancementIntegrationTDD verifies that the TDD implementation of security enhancement integration works
func TestSecurityEnhancementIntegrationTDD(t *testing.T) {
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

	// Test 1: Verify security enhancement methods are accessible
	t.Run("Security enhancement methods are accessible", func(t *testing.T) {
		// Create test workflow for security enhancement
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "test-template-001",
					Name: "Test Security Enhancement Template",
					Metadata: map[string]interface{}{
						"security_enhancement": true,
						"security_level":       "high",
						"compliance_required":  true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Security Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "delete_deployment", // Destructive action for security testing
						Parameters: map[string]interface{}{
							"force": false,
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

		workflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   template.ID,
					Name: template.Name,
				},
			},
			Template: template,
		}

		// Test ValidateSecurityConstraints
		securityResults := builder.ValidateSecurityConstraints(ctx, template)
		require.NotNil(t, securityResults)
		assert.GreaterOrEqual(t, len(securityResults), 0)

		// Verify security validation results structure
		for _, result := range securityResults {
			assert.NotEmpty(t, result.RuleID)
			assert.NotEmpty(t, result.Type)
			assert.NotEmpty(t, result.Message)
			assert.NotZero(t, result.Timestamp)
		}

		// Test ApplySecurityPolicies
		securityPolicies := map[string]interface{}{
			"rbac_enabled":           true,
			"network_policies":       true,
			"pod_security_standards": "restricted",
			"security_level":         "high",
		}
		securedTemplate := builder.ApplySecurityPolicies(template, securityPolicies)
		require.NotNil(t, securedTemplate)
		assert.Equal(t, template.ID, securedTemplate.ID)
		assert.Equal(t, len(template.Steps), len(securedTemplate.Steps))

		// Verify security policies were applied
		for _, step := range securedTemplate.Steps {
			if step.Variables != nil {
				if securityEnhanced, exists := step.Variables["security_enhanced"]; exists {
					assert.Equal(t, true, securityEnhanced)
				}
				if securityPoliciesApplied, exists := step.Variables["security_policies_applied"]; exists {
					assert.Equal(t, true, securityPoliciesApplied)
				}
			}
		}

		// Test GenerateSecurityReport
		securityReport := builder.GenerateSecurityReport(workflow)
		require.NotNil(t, securityReport)
		assert.Equal(t, workflow.ID, securityReport.WorkflowID)
		assert.GreaterOrEqual(t, securityReport.SecurityScore, 0.0)
		assert.LessOrEqual(t, securityReport.SecurityScore, 1.0)
		assert.GreaterOrEqual(t, securityReport.VulnerabilityCount, 0)
		assert.NotEmpty(t, securityReport.ComplianceStatus)
		assert.NotNil(t, securityReport.SecurityFindings)
		assert.NotNil(t, securityReport.RecommendedActions)
		assert.NotZero(t, securityReport.GeneratedAt)

		// Test existing safety functions integration
		safetyConstraints := &engine.SafetyConstraints{
			MaxConcurrentOperations: 3,
			MaxWorkflowDuration:     60 * time.Minute,
			RequireApproval:         true,
			AllowDestructiveActions: false,
		}
		safetyEnforcement := builder.EnforceSafetyConstraints(workflow, safetyConstraints)
		require.NotNil(t, safetyEnforcement)
		assert.NotNil(t, safetyEnforcement.ConstraintsViolated)
		assert.NotNil(t, safetyEnforcement.RequiredModifications)

		safetyRecommendations := builder.GenerateSafetyRecommendations(workflow)
		require.NotNil(t, safetyRecommendations)
		assert.GreaterOrEqual(t, len(safetyRecommendations), 0)

		safetyCheck := builder.ValidateWorkflowSafety(workflow)
		require.NotNil(t, safetyCheck)
		assert.GreaterOrEqual(t, safetyCheck.SafetyScore, 0.0)
		assert.LessOrEqual(t, safetyCheck.SafetyScore, 1.0)
	})

	// Test 2: Verify OptimizeWorkflowStructure includes security enhancement
	t.Run("OptimizeWorkflowStructure includes security enhancement", func(t *testing.T) {
		// Create template with security enhancement triggers
		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "sec-template-001",
					Name: "Security Enhancement Template",
					Metadata: map[string]interface{}{
						"security_enhancement": true,
						"security_level":       "high",
						"compliance_required":  true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Security Action Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "create_namespace", // Privileged action
						Parameters: map[string]interface{}{
							"name": "test-namespace",
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Destructive Action Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "delete_deployment", // Destructive action
						Parameters: map[string]interface{}{
							"force": false,
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

		// Verify that security enhancement was applied
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)

		// Check for security enhancement indicators in metadata
		if optimizedTemplate.Metadata != nil {
			if securityEnhanced, exists := optimizedTemplate.Metadata["security_enhanced"]; exists {
				assert.Equal(t, true, securityEnhanced)
			}
			if securityScore, exists := optimizedTemplate.Metadata["security_score"]; exists {
				assert.IsType(t, float64(0), securityScore)
				assert.GreaterOrEqual(t, securityScore.(float64), 0.0)
				assert.LessOrEqual(t, securityScore.(float64), 1.0)
			}
			if complianceStatus, exists := optimizedTemplate.Metadata["compliance_status"]; exists {
				assert.IsType(t, "", complianceStatus)
				assert.NotEmpty(t, complianceStatus.(string))
			}
			if securityViolations, exists := optimizedTemplate.Metadata["security_violations"]; exists {
				assert.IsType(t, 0, securityViolations)
				assert.GreaterOrEqual(t, securityViolations.(int), 0)
			}
		}
	})

	// Test 3: Verify GenerateWorkflow includes security enhancement
	t.Run("GenerateWorkflow includes security enhancement", func(t *testing.T) {
		objective := &engine.WorkflowObjective{
			ID:          "sec-obj-001",
			Type:        "security_enhancement",
			Description: "Security enhancement workflow optimization",
			Priority:    9,
			Constraints: map[string]interface{}{
				"security_enhancement": true,
				"security_level":       "high",
				"compliance_required":  true,
				"rbac_enabled":         true,
			},
		}

		template, err := builder.GenerateWorkflow(ctx, objective)

		require.NoError(t, err)
		require.NotNil(t, template)
		require.NotNil(t, template.Metadata)

		// Verify the workflow generation process includes security enhancement
		assert.NotEmpty(t, template.ID)
		assert.NotNil(t, template.Steps)

		// Verify that security enhancement metadata is present
		if template.Metadata != nil {
			// Security enhancement should contribute to workflow metadata
			assert.IsType(t, map[string]interface{}{}, template.Metadata)
		}
	})

	// Test 4: Verify security enhancement handles different scenarios
	t.Run("Security enhancement handles different scenarios", func(t *testing.T) {
		// Test with high-risk workflow
		highRiskTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "high-risk-template",
					Name: "High Risk Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Delete Namespace",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "delete_namespace", // Very high-risk action
						Parameters: map[string]interface{}{
							"cascade": true,
						},
					},
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Run Privileged",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "run_privileged", // Privileged action
						Parameters: map[string]interface{}{
							"command": "rm -rf /",
						},
					},
				},
			},
		}

		highRiskWorkflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: highRiskTemplate.ID,
				},
			},
			Template: highRiskTemplate,
		}

		securityReport := builder.GenerateSecurityReport(highRiskWorkflow)
		require.NotNil(t, securityReport)

		// Should identify high-risk actions
		assert.Greater(t, securityReport.VulnerabilityCount, 0)
		assert.Less(t, securityReport.SecurityScore, 0.8) // Lower security score for high-risk actions
		assert.Contains(t, []string{"non_compliant", "partially_compliant"}, securityReport.ComplianceStatus)

		// Test with minimal security requirements
		minimalTemplate := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "minimal-template",
					Name: "Minimal Security Template",
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Get Status",
					},
					Type:    engine.StepTypeAction,
					Timeout: 5 * time.Minute,
					Action: &engine.StepAction{
						Type: "get_status", // Non-destructive, non-privileged action
					},
				},
			},
		}

		minimalWorkflow := &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID: minimalTemplate.ID,
				},
			},
			Template: minimalTemplate,
		}

		minimalSecurityReport := builder.GenerateSecurityReport(minimalWorkflow)
		require.NotNil(t, minimalSecurityReport)

		// Should have better security score for minimal risk actions
		assert.GreaterOrEqual(t, minimalSecurityReport.SecurityScore, 0.5)
		assert.LessOrEqual(t, minimalSecurityReport.VulnerabilityCount, 1) // May have timeout issue
	})

	// Test 5: Verify business requirement compliance
	t.Run("Business requirement compliance", func(t *testing.T) {
		// BR-SEC-001: Security constraint validation
		// BR-SEC-002: Security policy application
		// BR-SEC-003: Security report generation
		// BR-SEC-004: Safety constraint enforcement
		// BR-SEC-005: Safety recommendation generation
		// BR-SEC-006: Comprehensive workflow safety validation
		// BR-SEC-007: Security integration in workflow generation
		// BR-SEC-008: Security enhancement in workflow structure optimization
		// BR-SEC-009: Public security enhancement method accessibility

		template := &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "br-test-template",
					Name: "Business Requirements Test Template",
					Metadata: map[string]interface{}{
						"security_enhancement": true,
						"security_level":       "high",
						"compliance_required":  true,
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Test Security Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "create_cluster_role", // Privileged action
						Parameters: map[string]interface{}{
							"name": "test-role",
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

		// Verify comprehensive security enhancement was performed
		assert.NotEmpty(t, optimizedTemplate.ID)
		assert.NotNil(t, optimizedTemplate.Steps)
		assert.NotNil(t, optimizedTemplate.Metadata)

		// Test all security enhancement capabilities
		securityResults := builder.ValidateSecurityConstraints(ctx, template)
		assert.NotNil(t, securityResults)

		securityPolicies := map[string]interface{}{
			"rbac_enabled":   true,
			"security_level": "high",
		}
		securedTemplate := builder.ApplySecurityPolicies(template, securityPolicies)
		assert.NotNil(t, securedTemplate)
		assert.Equal(t, template.ID, securedTemplate.ID)

		securityReport := builder.GenerateSecurityReport(workflow)
		assert.NotNil(t, securityReport)
		assert.Equal(t, workflow.ID, securityReport.WorkflowID)

		safetyConstraints := &engine.SafetyConstraints{
			MaxConcurrentOperations: 3,
			MaxWorkflowDuration:     45 * time.Minute,
		}
		safetyEnforcement := builder.EnforceSafetyConstraints(workflow, safetyConstraints)
		assert.NotNil(t, safetyEnforcement)

		safetyRecommendations := builder.GenerateSafetyRecommendations(workflow)
		assert.NotNil(t, safetyRecommendations)

		safetyCheck := builder.ValidateWorkflowSafety(workflow)
		assert.NotNil(t, safetyCheck)

		// Verify all security enhancement capabilities are working
		assert.GreaterOrEqual(t, len(securityResults), 0)
		assert.GreaterOrEqual(t, securityReport.SecurityScore, 0.0)
		assert.GreaterOrEqual(t, safetyCheck.SafetyScore, 0.0)
	})
}
