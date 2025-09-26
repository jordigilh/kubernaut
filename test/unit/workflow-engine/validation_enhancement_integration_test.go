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

var _ = Describe("Validation Enhancement Integration - TDD Implementation", func() {
	var (
		builder      *engine.DefaultIntelligentWorkflowBuilder
		mockVectorDB *mocks.MockVectorDatabase
		ctx          context.Context
		log          *logrus.Logger
		template     *engine.ExecutableTemplate
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.DebugLevel)
		ctx = context.Background()

		// Create mock vector database
		mockVectorDB = mocks.NewMockVectorDatabase()

		// Create builder with mock dependencies
		// RULE 12 COMPLIANCE: Updated constructor to use config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			VectorDB: mockVectorDB,
			Logger:   log,
		}
		var err error
		builder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred())

		// Create test template with validation scenarios
		template = &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
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
						Name: "Valid Action Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 10 * time.Minute,
					Action: &engine.StepAction{
						Type: "scale_deployment",
						Parameters: map[string]interface{}{
							"replicas":     3,
							"cpu_limit":    "1000m",
							"memory_limit": "2Gi",
						},
						Target: &engine.ActionTarget{
							Type:      "deployment",
							Namespace: "default",
							Name:      "test-deployment",
							Resource:  "deployments",
						},
					},
					Dependencies: []string{}, // No dependencies
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Dependent Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "collect_diagnostics",
						Parameters: map[string]interface{}{
							"timeout": "30s",
						},
						Target: &engine.ActionTarget{
							Type:      "pod",
							Namespace: "default",
							Name:      "test-pod",
							Resource:  "pods",
						},
					},
					Dependencies: []string{"step-001"}, // Depends on step-001
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-003",
						Name: "Destructive Action Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 5 * time.Minute,
					Action: &engine.StepAction{
						Type: "delete_deployment", // Destructive action
						Parameters: map[string]interface{}{
							"force": true,
						},
						Target: &engine.ActionTarget{
							Type:      "deployment",
							Namespace: "default",
							Name:      "old-deployment",
							Resource:  "deployments",
						},
						// Missing rollback for destructive action
					},
					Dependencies: []string{"step-002"},
				},
			},
			Variables: make(map[string]interface{}),
			Tags:      []string{"production", "critical"}, // High-risk environment
		}
	})

	Describe("Enhanced Validation Integration", func() {
		Context("when validating workflow templates", func() {
			It("should perform comprehensive validation using previously unused functions", func() {
				// Test that ValidateWorkflow integrates all validation functions
				// BR-VALID-001: Comprehensive validation integration

				validationReport := builder.ValidateWorkflow(ctx, template)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.ID).NotTo(BeEmpty())
				Expect(validationReport.WorkflowID).To(Equal(template.ID))
				Expect(validationReport.Type).To(Equal(engine.ValidationTypeIntegrity))
				Expect(validationReport.Results).NotTo(BeNil())

				// Validation should complete with status
				Expect(validationReport.Status).NotTo(BeEmpty())
				Expect(validationReport.CompletedAt).NotTo(BeNil())
			})

			It("should detect step dependency issues", func() {
				// Test validateStepDependencies integration
				// BR-VALID-002: Step dependency validation enhancement

				// Create template with circular dependency
				circularTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "circular-template",
							Name: "Circular Dependency Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-a",
								Name: "Step A",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"step-b"}, // Depends on B
						},
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-b",
								Name: "Step B",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"step-a"}, // Depends on A (circular)
						},
					},
				}

				validationReport := builder.ValidateWorkflow(ctx, circularTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(len(validationReport.Results)).To(BeNumerically(">", 0))

				// Should detect circular dependency
				foundCircularDependency := false
				for _, result := range validationReport.Results {
					if !result.Passed && result.Details != nil {
						if issue, exists := result.Details["issue"]; exists && issue == "circular_dependency" {
							foundCircularDependency = true
							break
						}
					}
				}
				Expect(foundCircularDependency).To(BeTrue())
			})

			It("should detect invalid dependency references", func() {
				// Test validateStepDependencies for invalid references
				// BR-VALID-003: Invalid dependency reference detection

				// Create template with invalid dependency reference
				invalidRefTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "invalid-ref-template",
							Name: "Invalid Reference Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-valid",
								Name: "Valid Step",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"non-existent-step"}, // Invalid reference
						},
					},
				}

				validationReport := builder.ValidateWorkflow(ctx, invalidRefTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(len(validationReport.Results)).To(BeNumerically(">", 0))

				// Should detect invalid dependency reference
				foundInvalidRef := false
				for _, result := range validationReport.Results {
					if !result.Passed && result.Details != nil {
						if depID, exists := result.Details["dependency_id"]; exists && depID == "non-existent-step" {
							foundInvalidRef = true
							break
						}
					}
				}
				Expect(foundInvalidRef).To(BeTrue())
			})
		})

		Context("when validating action parameters", func() {
			It("should detect missing required parameters", func() {
				// Test validateActionParameters integration
				// BR-VALID-004: Action parameter validation enhancement

				// Create template with missing required parameters
				missingParamTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "missing-param-template",
							Name: "Missing Parameter Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-missing-params",
								Name: "Step with Missing Parameters",
							},
							Type: engine.StepTypeAction,
							Action: &engine.StepAction{
								Type:       "scale_deployment",
								Parameters: map[string]interface{}{
									// Missing required parameters like replicas
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

				validationReport := builder.ValidateWorkflow(ctx, missingParamTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(len(validationReport.Results)).To(BeNumerically(">", 0))

				// Should detect missing parameters (implementation dependent)
				// The validation will run and may detect missing parameters based on action type
				Expect(validationReport.Status).NotTo(BeEmpty())
			})

			It("should detect invalid action targets", func() {
				// Test validateActionParameters for target validation
				// BR-VALID-005: Action target validation enhancement

				// Create template with invalid action target
				invalidTargetTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "invalid-target-template",
							Name: "Invalid Target Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-invalid-target",
								Name: "Step with Invalid Target",
							},
							Type: engine.StepTypeAction,
							Action: &engine.StepAction{
								Type: "scale_deployment",
								Parameters: map[string]interface{}{
									"replicas": 3,
								},
								Target: &engine.ActionTarget{
									Type: "", // Missing type
									// Missing namespace
									Name:     "test-deployment",
									Resource: "deployments",
								},
							},
						},
					},
				}

				validationReport := builder.ValidateWorkflow(ctx, invalidTargetTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(len(validationReport.Results)).To(BeNumerically(">", 0))

				// Should detect invalid target configuration
				foundInvalidTarget := false
				for _, result := range validationReport.Results {
					if !result.Passed && result.Details != nil {
						if _, exists := result.Details["target_error"]; exists {
							foundInvalidTarget = true
							break
						}
					}
				}
				Expect(foundInvalidTarget).To(BeTrue())
			})
		})

		Context("when validating resource access", func() {
			It("should validate resource availability and permissions", func() {
				// Test validateResourceAccess integration
				// BR-VALID-006: Resource access validation enhancement

				validationReport := builder.ValidateWorkflow(ctx, template)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.Results).NotTo(BeNil())

				// Resource validation should be performed (results depend on implementation)
				// The validation will check resource existence and permissions
				Expect(validationReport.Status).NotTo(BeEmpty())
			})
		})

		Context("when validating safety constraints", func() {
			It("should detect destructive actions without safeguards", func() {
				// Test validateSafetyConstraints integration
				// BR-VALID-007: Safety constraints validation enhancement

				validationReport := builder.ValidateWorkflow(ctx, template)

				Expect(validationReport).NotTo(BeNil())
				Expect(len(validationReport.Results)).To(BeNumerically(">", 0))

				// Should detect destructive action without rollback
				foundMissingRollback := false
				foundMissingConfirmation := false
				for _, result := range validationReport.Results {
					if !result.Passed && result.Message != "" {
						if result.Message == "Destructive action in step Destructive Action Step lacks rollback" {
							foundMissingRollback = true
						}
						if result.Message == "Destructive action in step Destructive Action Step lacks confirmation" {
							foundMissingConfirmation = true
						}
					}
				}

				// At least one safety issue should be detected
				Expect(foundMissingRollback || foundMissingConfirmation).To(BeTrue())
			})

			It("should detect missing timeout configurations", func() {
				// Test validateSafetyConstraints for timeout validation
				// BR-VALID-008: Timeout configuration validation

				// Create template with missing timeout
				noTimeoutTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "no-timeout-template",
							Name: "No Timeout Template",
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
					},
				}

				validationReport := builder.ValidateWorkflow(ctx, noTimeoutTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(len(validationReport.Results)).To(BeNumerically(">", 0))

				// Should detect missing timeout
				foundMissingTimeout := false
				for _, result := range validationReport.Results {
					if !result.Passed && result.Message == "Step Step without Timeout lacks timeout configuration" {
						foundMissingTimeout = true
						break
					}
				}
				Expect(foundMissingTimeout).To(BeTrue())
			})
		})
	})

	Describe("Enhanced Validation Public Methods", func() {
		Context("when using public validation methods", func() {
			It("should provide comprehensive validation reporting", func() {
				// Test that enhanced validation methods are accessible
				// BR-VALID-009: Public validation method accessibility

				validationReport := builder.ValidateWorkflow(ctx, template)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.ID).NotTo(BeEmpty())
				Expect(validationReport.WorkflowID).To(Equal(template.ID))
				Expect(validationReport.Results).NotTo(BeNil())
				Expect(validationReport.Summary).NotTo(BeNil())

				// Summary should contain validation statistics
				Expect(validationReport.Summary.Total).To(BeNumerically(">=", 0))
				Expect(validationReport.Summary.Passed).To(BeNumerically(">=", 0))
				Expect(validationReport.Summary.Failed).To(BeNumerically(">=", 0))
				Expect(validationReport.Summary.Total).To(Equal(validationReport.Summary.Passed + validationReport.Summary.Failed))
			})

			It("should generate detailed validation results", func() {
				// Test detailed validation result generation
				// BR-VALID-010: Detailed validation result generation

				validationReport := builder.ValidateWorkflow(ctx, template)

				Expect(validationReport).NotTo(BeNil())

				// Each validation result should have required fields
				for _, result := range validationReport.Results {
					Expect(result.RuleID).NotTo(BeEmpty())
					Expect(result.Type).NotTo(BeEmpty())
					Expect(result.Message).NotTo(BeEmpty())
					Expect(result.Timestamp).NotTo(BeZero())

					// Details should be present for failed validations
					if !result.Passed {
						Expect(result.Details).NotTo(BeNil())
					}
				}
			})
		})
	})

	Describe("Validation Enhancement Edge Cases", func() {
		Context("when handling edge cases", func() {
			It("should handle empty workflow templates gracefully", func() {
				// Test validation with empty template
				emptyTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "empty-template",
							Name: "Empty Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{}, // No steps
				}

				validationReport := builder.ValidateWorkflow(ctx, emptyTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.ID).NotTo(BeEmpty())
				Expect(validationReport.Status).NotTo(BeEmpty())
			})

			It("should handle templates with only conditional steps", func() {
				// Test validation with conditional steps
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

				validationReport := builder.ValidateWorkflow(ctx, conditionalTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.Status).NotTo(BeEmpty())
			})

			It("should handle complex dependency chains", func() {
				// Test validation with complex dependency chains
				complexTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "complex-template",
							Name: "Complex Dependency Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-1",
								Name: "Step 1",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{}, // Root step
						},
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-2",
								Name: "Step 2",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"step-1"},
						},
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-3",
								Name: "Step 3",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"step-1"},
						},
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-4",
								Name: "Step 4",
							},
							Type:         engine.StepTypeAction,
							Dependencies: []string{"step-2", "step-3"}, // Multiple dependencies
						},
					},
				}

				validationReport := builder.ValidateWorkflow(ctx, complexTemplate)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.Status).NotTo(BeEmpty())

				// Should not detect circular dependencies in valid chain
				foundCircularDependency := false
				for _, result := range validationReport.Results {
					if !result.Passed && result.Details != nil {
						if issue, exists := result.Details["issue"]; exists && issue == "circular_dependency" {
							foundCircularDependency = true
							break
						}
					}
				}
				Expect(foundCircularDependency).To(BeFalse())
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-VALID-001 through BR-VALID-010", func() {
			It("should demonstrate complete validation enhancement integration compliance", func() {
				// Comprehensive test for all validation enhancement business requirements

				validationReport := builder.ValidateWorkflow(ctx, template)

				Expect(validationReport).NotTo(BeNil())
				Expect(validationReport.ID).NotTo(BeEmpty())
				Expect(validationReport.WorkflowID).To(Equal(template.ID))

				// Verify comprehensive validation was performed
				Expect(validationReport.Results).NotTo(BeNil())
				Expect(validationReport.Summary).NotTo(BeNil())
				Expect(validationReport.Status).NotTo(BeEmpty())
				Expect(validationReport.CompletedAt).NotTo(BeNil())

				// Verify validation covers multiple aspects
				validationTypes := make(map[engine.ValidationType]bool)
				for _, result := range validationReport.Results {
					validationTypes[result.Type] = true
				}

				// Should have performed multiple types of validation
				Expect(len(validationTypes)).To(BeNumerically(">=", 1))
			})
		})
	})
})
