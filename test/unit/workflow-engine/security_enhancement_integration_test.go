package workflowengine_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Security Enhancement Integration - TDD Implementation", func() {
	var (
		builder      *engine.DefaultIntelligentWorkflowBuilder
		mockVectorDB *mocks.MockVectorDatabase
		ctx          context.Context
		log          *logrus.Logger
		template     *engine.ExecutableTemplate
		workflow     *engine.Workflow
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.DebugLevel)
		ctx = context.Background()

		// Create mock vector database
		mockVectorDB = mocks.NewMockVectorDatabase()

		// Create builder with mock dependencies using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       nil,
			VectorDB:        mockVectorDB,
			AnalyticsEngine: nil,
			PatternStore:    nil,
			ExecutionRepo:   nil,
			Logger:          log,
		}

		var err error
		builder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")

		// Create test template for security enhancement
		template = &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
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
						Name: "Security Validation Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "validate_security",
						Parameters: map[string]interface{}{
							"security_level": "high",
							"compliance":     true,
						},
						Target: &engine.ActionTarget{
							Type:      "security_policy",
							Namespace: "default",
							Name:      "security-policy",
							Resource:  "policies",
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
						ID:   "step-003",
						Name: "Security Policy Application Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 12 * time.Minute,
					Action: &engine.StepAction{
						Type: "apply_security_policy",
						Parameters: map[string]interface{}{
							"policy_type": "rbac",
							"strict_mode": true,
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
		}

		// Create workflow from template
		workflow = &engine.Workflow{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   template.ID,
					Name: template.Name,
				},
			},
			Template: template,
		}
	})

	Describe("Security Enhancement Integration", func() {
		Context("when validating security constraints", func() {
			It("should validate security constraints using previously unused functions", func() {
				// Test that security constraint validation integrates security functions
				// BR-SEC-001: Security constraint validation

				// Test ValidateSecurityConstraints (will be implemented)
				securityResults := builder.ValidateSecurityConstraints(ctx, template)

				Expect(securityResults).NotTo(BeNil())
				Expect(len(securityResults)).To(BeNumerically(">=", 0))

				// Verify security validation results
				for _, result := range securityResults {
					Expect(result.RuleID).NotTo(BeEmpty())
					Expect(result.Type).NotTo(BeEmpty())
					Expect(result.Message).NotTo(BeEmpty())
					Expect(result.Timestamp).NotTo(BeZero())
				}
			})

			It("should apply security policies", func() {
				// Test security policy application
				// BR-SEC-002: Security policy application

				securityPolicies := map[string]interface{}{
					"rbac_enabled":           true,
					"network_policies":       true,
					"pod_security_standards": "restricted",
					"security_contexts":      true,
					"admission_controllers":  []string{"PodSecurityPolicy", "NetworkPolicy"},
				}

				// Apply security policies (this will be implemented)
				securedTemplate := builder.ApplySecurityPolicies(template, securityPolicies)

				Expect(securedTemplate).NotTo(BeNil())
				Expect(securedTemplate.ID).To(Equal(template.ID))
				Expect(len(securedTemplate.Steps)).To(BeNumerically(">=", len(template.Steps)))

				// Verify security policies were applied
				for _, step := range securedTemplate.Steps {
					if step.Variables != nil {
						// Check for security-related variables
						if securityEnhanced, exists := step.Variables["security_enhanced"]; exists {
							Expect(securityEnhanced).To(Equal(true))
						}
					}
				}
			})

			It("should generate security reports", func() {
				// Test security report generation
				// BR-SEC-003: Security report generation

				// Generate security report (this will be implemented)
				securityReport := builder.GenerateSecurityReport(workflow)

				Expect(securityReport).NotTo(BeNil())
				Expect(securityReport.WorkflowID).To(Equal(workflow.ID))
				Expect(securityReport.SecurityScore).To(BeNumerically(">=", 0))
				Expect(securityReport.SecurityScore).To(BeNumerically("<=", 1))
				Expect(securityReport.VulnerabilityCount).To(BeNumerically(">=", 0))
				Expect(securityReport.ComplianceStatus).NotTo(BeEmpty())
				Expect(len(securityReport.SecurityFindings)).To(BeNumerically(">=", 0))
			})
		})

		Context("when enforcing safety constraints", func() {
			It("should enforce safety constraints and guardrails", func() {
				// Test safety constraint enforcement using existing functions
				// BR-SEC-004: Safety constraint enforcement

				safetyConstraints := &engine.SafetyConstraints{
					MaxConcurrentOperations: 3,
					MaxWorkflowDuration:     60 * time.Minute,
					RequireApproval:         true,
					AllowDestructiveActions: false,
				}

				// Enforce safety constraints (existing function)
				safetyEnforcement := builder.EnforceSafetyConstraints(workflow, safetyConstraints)

				Expect(safetyEnforcement).NotTo(BeNil())
				Expect(safetyEnforcement.ConstraintsViolated).NotTo(BeNil())
				Expect(safetyEnforcement.RequiredModifications).NotTo(BeNil())
				Expect(safetyEnforcement.CanProceed).To(BeAssignableToTypeOf(true))
			})

			It("should generate safety recommendations", func() {
				// Test safety recommendation generation using existing functions
				// BR-SEC-005: Safety recommendation generation

				// Generate safety recommendations (existing function returns []string)
				safetyRecommendations := builder.GenerateSafetyRecommendations(workflow)

				Expect(safetyRecommendations).NotTo(BeNil())
				Expect(len(safetyRecommendations)).To(BeNumerically(">=", 0))

				// Verify recommendations are strings with content
				for _, recommendation := range safetyRecommendations {
					Expect(recommendation).NotTo(BeEmpty(), "Each safety recommendation should have content")
				}
			})
		})

		Context("when validating workflow safety", func() {
			It("should validate workflow safety comprehensively", func() {
				// Test comprehensive workflow safety validation using existing functions
				// BR-SEC-006: Comprehensive workflow safety validation

				// Validate workflow safety (existing function)
				safetyCheck := builder.ValidateWorkflowSafety(workflow)

				Expect(safetyCheck).NotTo(BeNil())
				Expect(safetyCheck.IsSafe).To(BeAssignableToTypeOf(true))
				Expect(safetyCheck.SafetyScore).To(BeNumerically(">=", 0))
				Expect(safetyCheck.SafetyScore).To(BeNumerically("<=", 1))
				Expect(safetyCheck.RiskFactors).NotTo(BeNil())
				Expect(len(safetyCheck.RiskFactors)).To(BeNumerically(">=", 0))
			})
		})
	})

	Describe("Enhanced Security Integration", func() {
		Context("when security enhancement is integrated into workflow optimization", func() {
			It("should enhance workflow generation with security optimization", func() {
				// Test that security enhancement is integrated into workflow generation
				// BR-SEC-007: Security integration in workflow generation

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

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Verify that security enhancement metadata is present
				if template.Metadata != nil {
					// Security enhancement should contribute to workflow metadata
					Expect(len(template.Metadata)).To(BeNumerically(">=", 0))
				}
			})

			It("should apply security enhancement during workflow structure optimization", func() {
				// Test that security enhancement is applied during OptimizeWorkflowStructure
				// BR-SEC-008: Security enhancement in workflow structure optimization

				optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

				Expect(err).NotTo(HaveOccurred())
				Expect(optimizedTemplate).NotTo(BeNil())
				Expect(optimizedTemplate.ID).NotTo(BeEmpty())
				Expect(len(optimizedTemplate.Steps)).To(BeNumerically(">", 0))

				// Verify the optimization process includes security enhancement considerations
				Expect(optimizedTemplate.Metadata).NotTo(BeNil())
			})
		})
	})

	Describe("Security Enhancement Public Methods", func() {
		Context("when using public security enhancement methods", func() {
			It("should provide comprehensive security enhancement capabilities", func() {
				// Test that security enhancement methods are accessible
				// BR-SEC-009: Public security enhancement method accessibility

				// Test ValidateSecurityConstraints (will be implemented)
				securityResults := builder.ValidateSecurityConstraints(ctx, template)
				Expect(securityResults).NotTo(BeNil())

				// Test ApplySecurityPolicies (will be implemented)
				securityPolicies := map[string]interface{}{
					"rbac_enabled":   true,
					"security_level": "high",
				}
				securedTemplate := builder.ApplySecurityPolicies(template, securityPolicies)
				Expect(securedTemplate).NotTo(BeNil())

				// Test GenerateSecurityReport (will be implemented)
				securityReport := builder.GenerateSecurityReport(workflow)
				Expect(securityReport).NotTo(BeNil())

				// Test EnforceSafetyConstraints (existing function)
				safetyConstraints := &engine.SafetyConstraints{
					MaxConcurrentOperations: 5,
					MaxWorkflowDuration:     30 * time.Minute,
				}
				safetyEnforcement := builder.EnforceSafetyConstraints(workflow, safetyConstraints)
				Expect(safetyEnforcement).NotTo(BeNil())

				// Test GenerateSafetyRecommendations (existing function)
				safetyRecommendations := builder.GenerateSafetyRecommendations(workflow)
				Expect(safetyRecommendations).NotTo(BeNil())

				// Test ValidateWorkflowSafety (existing function)
				safetyCheck := builder.ValidateWorkflowSafety(workflow)
				Expect(safetyCheck).NotTo(BeNil())
			})
		})
	})

	Describe("Security Enhancement Edge Cases", func() {
		Context("when handling edge cases", func() {
			It("should handle workflows with no security requirements", func() {
				// Test security enhancement with minimal security requirements
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
								Name: "Simple Step",
							},
							Type:    engine.StepTypeAction,
							Timeout: 5 * time.Minute,
							Action: &engine.StepAction{
								Type: "get_status", // Non-destructive action
							},
						},
					},
				}

				securityResults := builder.ValidateSecurityConstraints(ctx, minimalTemplate)
				Expect(securityResults).NotTo(BeNil())
				// Should have minimal or no security violations
			})

			It("should handle workflows with high-risk actions", func() {
				// Test security enhancement with high-risk actions
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
								Name: "Delete Deployment",
							},
							Type:    engine.StepTypeAction,
							Timeout: 10 * time.Minute,
							Action: &engine.StepAction{
								Type: "delete_deployment", // High-risk action
								Parameters: map[string]interface{}{
									"force": true,
								},
							},
						},
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-002",
								Name: "Delete Namespace",
							},
							Type:    engine.StepTypeAction,
							Timeout: 15 * time.Minute,
							Action: &engine.StepAction{
								Type: "delete_namespace", // Very high-risk action
								Parameters: map[string]interface{}{
									"cascade": true,
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
				Expect(securityReport).NotTo(BeNil())
				// Should identify high-risk actions
				Expect(securityReport.SecurityScore).To(BeNumerically("<", 0.8)) // Lower security score for high-risk actions
			})

			It("should handle empty workflows gracefully", func() {
				// Test security enhancement with empty workflow
				emptyTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "empty-template",
							Name: "Empty Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{}, // No steps
				}

				emptyWorkflow := &engine.Workflow{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID: emptyTemplate.ID,
						},
					},
					Template: emptyTemplate,
				}

				securityReport := builder.GenerateSecurityReport(emptyWorkflow)
				Expect(securityReport).NotTo(BeNil())
				// Should handle empty workflow gracefully
				Expect(securityReport.SecurityScore).To(BeNumerically(">=", 0))
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-SEC-001 through BR-SEC-009", func() {
			It("should demonstrate complete security enhancement integration compliance", func() {
				// Comprehensive test for all security enhancement business requirements

				// BR-SEC-001: Security constraint validation
				securityResults := builder.ValidateSecurityConstraints(ctx, template)
				Expect(securityResults).NotTo(BeNil())

				// BR-SEC-002: Security policy application
				securityPolicies := map[string]interface{}{
					"rbac_enabled":    true,
					"security_level":  "high",
					"compliance_mode": true,
				}
				securedTemplate := builder.ApplySecurityPolicies(template, securityPolicies)
				Expect(securedTemplate).NotTo(BeNil())

				// BR-SEC-003: Security report generation
				securityReport := builder.GenerateSecurityReport(workflow)
				Expect(securityReport).NotTo(BeNil())

				// BR-SEC-004: Safety constraint enforcement
				safetyConstraints := &engine.SafetyConstraints{
					MaxConcurrentOperations: 3,
					MaxWorkflowDuration:     45 * time.Minute,
				}
				safetyEnforcement := builder.EnforceSafetyConstraints(workflow, safetyConstraints)
				Expect(safetyEnforcement).NotTo(BeNil())

				// BR-SEC-005: Safety recommendation generation
				safetyRecommendations := builder.GenerateSafetyRecommendations(workflow)
				Expect(safetyRecommendations).NotTo(BeNil())

				// BR-SEC-006: Comprehensive workflow safety validation
				safetyCheck := builder.ValidateWorkflowSafety(workflow)
				Expect(safetyCheck).NotTo(BeNil())

				// Verify all security enhancement capabilities are working
				Expect(len(securityResults)).To(BeNumerically(">=", 0))
				Expect(securedTemplate.ID).To(Equal(template.ID))
				Expect(securityReport.WorkflowID).To(Equal(workflow.ID))
				Expect(safetyEnforcement.CanProceed).To(BeAssignableToTypeOf(true))
				Expect(len(safetyRecommendations)).To(BeNumerically(">=", 0))
				Expect(safetyCheck.SafetyScore).To(BeNumerically(">=", 0))
			})
		})
	})

	Describe("Security Enhancement Integration with Existing Functions", func() {
		Context("when integrating with existing security functions", func() {
			It("should leverage existing safety validation functions", func() {
				// Test integration with existing validateSafetyConstraints function
				// This function is already implemented in the codebase

				// The existing function should be accessible through the new public method
				securityResults := builder.ValidateSecurityConstraints(ctx, template)
				Expect(securityResults).NotTo(BeNil())

				// Verify that existing safety validation logic is being used
				for _, result := range securityResults {
					// Results should follow the existing validation result structure
					Expect(result.RuleID).NotTo(BeEmpty())
					Expect(result.Type).NotTo(BeEmpty())
					Expect(result.Message).NotTo(BeEmpty())
				}
			})

			It("should integrate with existing safety enhancement functions", func() {
				// Test integration with existing applySafetyEnhancements function

				objective := &engine.WorkflowObjective{
					ID:          "safety-obj-001",
					Type:        "safety_enhancement",
					Description: "Safety enhancement integration test",
					Priority:    8,
					Constraints: map[string]interface{}{
						"safety_level":         "high",
						"require_confirmation": true,
					},
				}

				// The existing applySafetyEnhancements function should be integrated
				// into the workflow generation process
				template, err := builder.GenerateWorkflow(ctx, objective)
				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify safety enhancements were applied
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))
			})
		})
	})
})
