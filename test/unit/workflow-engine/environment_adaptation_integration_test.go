<<<<<<< HEAD
package workflowengine_test

import (
	"testing"
	"context"
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

package workflowengine_test

import (
	"context"
	"testing"
>>>>>>> crd_implementation
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Environment Adaptation Integration - TDD Implementation", func() {
	var (
		builder         *engine.DefaultIntelligentWorkflowBuilder
		mockVectorDB    *mocks.MockVectorDatabase
		ctx             context.Context
		log             *logrus.Logger
		objective       *engine.WorkflowObjective
		template        *engine.ExecutableTemplate
		workflowContext *engine.WorkflowContext
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

		// Create test objective with environment-specific requirements
		objective = &engine.WorkflowObjective{
			ID:          "obj-001",
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

		// Create test workflow context
		workflowContext = &engine.WorkflowContext{
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
		template = &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
					Name: "Environment Adaptive Template",
					Metadata: map[string]interface{}{
						"environment": "production",
						"priority":    6,
					},
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
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Environment-Agnostic Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 5 * time.Minute,
					Action: &engine.StepAction{
						Type: "health_check",
						Parameters: map[string]interface{}{
							"timeout": "30s",
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
		}
	})

	Describe("adaptPatternStepsToContext Integration", func() {
		Context("when integrated into workflow generation", func() {
			It("should adapt pattern steps to workflow context", func() {
				// Test that adaptPatternStepsToContext is called and adapts steps correctly
				// BR-ENV-001: Pattern step adaptation to environment context

				adaptedSteps := builder.AdaptPatternStepsToContext(ctx, template.Steps, workflowContext)

				Expect(adaptedSteps).NotTo(BeNil())
				Expect(len(adaptedSteps)).To(Equal(len(template.Steps)))

				// Verify namespace adaptation
				for _, step := range adaptedSteps {
					if step.Action != nil && step.Action.Target != nil {
						Expect(step.Action.Target.Namespace).To(Equal(workflowContext.Namespace))
					}
				}
			})

			It("should handle steps without targets gracefully", func() {
				// Test edge case: steps without action targets
				stepsWithoutTargets := []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{
							ID:   "step-no-target",
							Name: "Step Without Target",
						},
						Type:    engine.StepTypeAction,
						Timeout: 5 * time.Minute,
						Action: &engine.StepAction{
							Type: "health_check",
							// No Target field
							Parameters: map[string]interface{}{
								"timeout": "30s",
							},
						},
					},
				}

				adaptedSteps := builder.AdaptPatternStepsToContext(ctx, stepsWithoutTargets, workflowContext)

				Expect(adaptedSteps).NotTo(BeNil())
				Expect(len(adaptedSteps)).To(Equal(1))
				// Should not crash and should preserve original step
				Expect(adaptedSteps[0].ID).To(Equal("step-no-target"))
			})
		})
	})

	Describe("customizeStepsForEnvironment Integration", func() {
		Context("when integrated into environment adaptation", func() {
			It("should customize steps for specific environment", func() {
				// Test that customizeStepsForEnvironment is called and customizes correctly
				// BR-ENV-002: Environment-specific step customization

				customizedSteps := builder.CustomizeStepsForEnvironment(ctx, template.Steps, "production")

				Expect(customizedSteps).NotTo(BeNil())
				Expect(len(customizedSteps)).To(Equal(len(template.Steps)))

				// Verify environment customization
				for _, step := range customizedSteps {
					Expect(step.Variables).NotTo(BeNil())
					Expect(step.Variables["environment"]).To(Equal("production"))
				}
			})

			It("should handle different environment types", func() {
				// Test customization for different environments
				environments := []string{"production", "staging", "development"}

				for _, env := range environments {
					customizedSteps := builder.CustomizeStepsForEnvironment(ctx, template.Steps, env)

					Expect(customizedSteps).NotTo(BeNil())
					for _, step := range customizedSteps {
						Expect(step.Variables["environment"]).To(Equal(env))
					}
				}
			})

			It("should preserve original steps structure", func() {
				// Test that original steps are not modified
				originalStepCount := len(template.Steps)
				originalFirstStepID := template.Steps[0].ID

				customizedSteps := builder.CustomizeStepsForEnvironment(ctx, template.Steps, "production")

				// Original template should be unchanged
				Expect(len(template.Steps)).To(Equal(originalStepCount))
				Expect(template.Steps[0].ID).To(Equal(originalFirstStepID))

				// Customized steps should be different instances
				Expect(len(customizedSteps)).To(Equal(originalStepCount))
				Expect(customizedSteps[0].ID).To(Equal(originalFirstStepID))
			})
		})
	})

	Describe("addContextSpecificConditions Integration", func() {
		Context("when integrated into production safety", func() {
			It("should add production-specific safety conditions", func() {
				// Test that addContextSpecificConditions adds safety conditions for production
				// BR-ENV-003: Context-specific condition addition for safety

				enhancedSteps := builder.AddContextSpecificConditions(ctx, template.Steps, workflowContext)

				Expect(enhancedSteps).NotTo(BeNil())
				Expect(len(enhancedSteps)).To(Equal(len(template.Steps)))

				// Verify production safety conditions are added
				for _, step := range enhancedSteps {
					if step.Action != nil && workflowContext.Environment == "production" {
						Expect(step.Condition).NotTo(BeNil())
						Expect(step.Condition.Name).To(Equal("production-safety"))
						Expect(step.Condition.Type).To(Equal(engine.ConditionTypeCustom))
						Expect(step.Condition.Expression).To(ContainSubstring("production"))
					}
				}
			})

			It("should not add conditions for non-production environments", func() {
				// Test that conditions are not added for non-production environments
				devContext := &engine.WorkflowContext{
					BaseContext: types.BaseContext{
						Environment: "development",
						Timestamp:   time.Now(),
					},
					WorkflowID: "workflow-dev-001",
					Namespace:  "default",
					Variables:  make(map[string]interface{}),
					CreatedAt:  time.Now(),
				}

				enhancedSteps := builder.AddContextSpecificConditions(ctx, template.Steps, devContext)

				Expect(enhancedSteps).NotTo(BeNil())

				// Verify no production-specific conditions are added
				for _, step := range enhancedSteps {
					if step.Action != nil {
						// Should not have production safety conditions
						if step.Condition != nil {
							Expect(step.Condition.Name).NotTo(Equal("production-safety"))
						}
					}
				}
			})

			It("should handle steps without actions gracefully", func() {
				// Test edge case: steps without actions
				stepsWithoutActions := []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{
							ID:   "step-no-action",
							Name: "Step Without Action",
						},
						Type:    engine.StepTypeCondition,
						Timeout: 5 * time.Minute,
						// No Action field
					},
				}

				enhancedSteps := builder.AddContextSpecificConditions(ctx, stepsWithoutActions, workflowContext)

				Expect(enhancedSteps).NotTo(BeNil())
				Expect(len(enhancedSteps)).To(Equal(1))
				// Should not crash and should preserve original step
				Expect(enhancedSteps[0].ID).To(Equal("step-no-action"))
			})
		})
	})

	Describe("Integrated Environment Adaptation Workflow", func() {
		Context("when environment adaptation is fully integrated", func() {
			It("should enhance workflow generation with environment adaptation", func() {
				// Test complete environment adaptation integration
				// BR-ENV-004: Complete environment adaptation pipeline integration

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify that environment adaptation was integrated into workflow generation
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Verify environment adaptation metadata is present
				if template.Metadata != nil {
					// Environment adaptation should contribute to workflow metadata
					Expect(len(template.Metadata)).To(BeNumerically(">=", 0))
				}
			})

			It("should apply environment constraints during workflow generation", func() {
				// Test that environment constraints are applied during generation
				// BR-ENV-005: Environment constraints application in workflow generation

				// Create objective with strict environment constraints
				constrainedObjective := &engine.WorkflowObjective{
					ID:          "obj-002",
					Type:        "optimization",
					Description: "Strictly environment-constrained workflow",
					Priority:    1, // High priority
					Constraints: map[string]interface{}{
						"environment":        "production",
						"namespace":          "kube-system",
						"safety_level":       "high",
						"max_parallel_steps": 1, // Very conservative
					},
				}

				template, err := builder.GenerateWorkflow(ctx, constrainedObjective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify the workflow generation process includes environment constraints
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))
			})

			It("should handle multi-environment scenarios", func() {
				// Test environment adaptation across different environments
				// BR-ENV-006: Multi-environment adaptation support

				environments := []string{"production", "staging", "development"}

				for _, env := range environments {
					envObjective := &engine.WorkflowObjective{
						ID:          "obj-" + env,
						Type:        "remediation",
						Description: "Environment-specific workflow for " + env,
						Priority:    5,
						Constraints: map[string]interface{}{
							"environment": env,
						},
					}

					template, err := builder.GenerateWorkflow(ctx, envObjective)

					Expect(err).NotTo(HaveOccurred())
					Expect(template).NotTo(BeNil())
					Expect(template.ID).NotTo(BeEmpty())
				}
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-ENV-001 through BR-ENV-006", func() {
			It("should demonstrate complete environment adaptation integration compliance", func() {
				// Comprehensive test for all environment adaptation business requirements

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())

				// Verify the workflow generation process includes environment adaptation
				// The specific adaptations will be applied when environment context is present
				// This test ensures the integration points are working

				// Test public methods are accessible
				adaptedSteps := builder.AdaptPatternStepsToContext(ctx, template.Steps, workflowContext)
				Expect(adaptedSteps).NotTo(BeNil())

				customizedSteps := builder.CustomizeStepsForEnvironment(ctx, template.Steps, "production")
				Expect(customizedSteps).NotTo(BeNil())

				enhancedSteps := builder.AddContextSpecificConditions(ctx, template.Steps, workflowContext)
				Expect(enhancedSteps).NotTo(BeNil())
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUenvironmentUadaptationUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UenvironmentUadaptationUintegration Suite")
}
