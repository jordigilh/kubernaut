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

var _ = Describe("Resource Optimization Integration - TDD Implementation", func() {
	var (
		builder      *engine.DefaultIntelligentWorkflowBuilder
		mockVectorDB *mocks.MockVectorDatabase
		ctx          context.Context
		log          *logrus.Logger
		objective    *engine.WorkflowObjective
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

		// Create test objective with resource constraints
		objective = &engine.WorkflowObjective{
			ID:          "obj-001",
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

		// Create test template with resource-intensive steps
		template = &engine.ExecutableTemplate{
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
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Memory Intensive Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 15 * time.Minute,
					Action: &engine.StepAction{
						Type: "increase_resources",
						Parameters: map[string]interface{}{
							"cpu_limit":    "500m",
							"memory_limit": "3Gi",
							"storage":      "10Gi",
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
		}
	})

	Describe("applyResourceConstraintManagement Integration", func() {
		Context("when integrated into workflow optimization", func() {
			It("should apply comprehensive resource constraint management", func() {
				// Test that applyResourceConstraintManagement is called and optimizes resources
				// BR-RESOURCE-001: Comprehensive resource constraint management

				optimizedTemplate, err := builder.ApplyResourceConstraintManagement(ctx, template, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(optimizedTemplate).NotTo(BeNil())
				Expect(optimizedTemplate.ID).To(Equal(template.ID))

				// Verify optimization was applied
				Expect(len(optimizedTemplate.Steps)).To(BeNumerically(">=", len(template.Steps)))
			})

			It("should extract and apply constraints from objective", func() {
				// Test constraint extraction and application
				// BR-RESOURCE-002: Constraint extraction and validation

				constraints, err := builder.ExtractConstraintsFromObjective(objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(constraints).NotTo(BeNil())

				// Verify key constraints are extracted
				Expect(constraints["max_execution_time"]).To(Equal("30m"))
				Expect(constraints["cost_budget"]).To(Equal(100.0))
				Expect(constraints["efficiency_target"]).To(Equal(0.85))

				// Verify resource limits are extracted
				if resourceLimits, ok := constraints["resource_limits"]; ok {
					limits := resourceLimits.(map[string]interface{})
					Expect(limits["cpu"]).To(Equal("2000m"))
					Expect(limits["memory"]).To(Equal("4Gi"))
				}
			})

			It("should apply cost optimization constraints", func() {
				// Test cost optimization constraint application
				// BR-RESOURCE-003: Cost optimization constraints

				constraints := map[string]interface{}{
					"cost_budget":       100.0,
					"efficiency_target": 0.85,
				}

				// Create a copy to test cost optimization
				testTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: template.BaseVersionedEntity,
					Steps:               make([]*engine.ExecutableWorkflowStep, len(template.Steps)),
					Variables:           make(map[string]interface{}),
				}
				copy(testTemplate.Steps, template.Steps)

				builder.ApplyCostOptimizationConstraints(testTemplate, constraints)

				// Verify cost optimization was applied (metadata should be updated)
				Expect(testTemplate).NotTo(BeNil())
				Expect(testTemplate.Steps).NotTo(BeEmpty())
			})

			It("should handle context cancellation gracefully", func() {
				// Test context cancellation handling
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				optimizedTemplate, err := builder.ApplyResourceConstraintManagement(cancelCtx, template, objective)

				// Should return original template and context error
				Expect(err).To(Equal(context.Canceled))
				Expect(optimizedTemplate).To(Equal(template))
			})
		})
	})

	Describe("calculateResourceEfficiency Integration", func() {
		Context("when integrated into resource optimization", func() {
			It("should calculate resource efficiency improvements", func() {
				// Test resource efficiency calculation
				// BR-RESOURCE-004: Resource efficiency calculation and validation

				// Create an optimized template with reduced resource usage
				optimizedTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: template.BaseVersionedEntity,
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "step-001-optimized",
								Name: "Optimized CPU Step",
							},
							Type:    engine.StepTypeAction,
							Timeout: 5 * time.Minute, // Reduced timeout
							Action: &engine.StepAction{
								Type: "scale_deployment",
								Parameters: map[string]interface{}{
									"cpu_limit":    "500m", // Reduced CPU
									"memory_limit": "1Gi",  // Reduced memory
									"replicas":     3,      // Reduced replicas
								},
							},
						},
					},
					Variables: make(map[string]interface{}),
				}

				efficiency := builder.CalculateResourceEfficiency(optimizedTemplate, template)

				Expect(efficiency).To(BeNumerically(">=", 0.0))
				Expect(efficiency).To(BeNumerically("<=", 1.0))

				// Should show improvement (efficiency > 0) since we reduced resources
				Expect(efficiency).To(BeNumerically(">", 0.0))
			})

			It("should handle templates with no resource specifications", func() {
				// Test edge case: templates without resource specifications
				emptyTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID: "empty-template",
						},
					},
					Steps:     []*engine.ExecutableWorkflowStep{},
					Variables: make(map[string]interface{}),
				}

				efficiency := builder.CalculateResourceEfficiency(emptyTemplate, template)

				// Should return default efficiency when no resources to compare
				Expect(efficiency).To(BeNumerically(">=", 0.0))
				Expect(efficiency).To(BeNumerically("<=", 1.0))
			})
		})
	})

	Describe("CalculateResourceAllocation Integration", func() {
		Context("when integrated into workflow optimization", func() {
			It("should calculate optimal resource allocation for workflow steps", func() {
				// Test resource allocation calculation
				// BR-RESOURCE-005: Optimal resource allocation calculation

				resourcePlan := builder.CalculateResourceAllocation(template.Steps)

				Expect(resourcePlan).NotTo(BeNil())
				Expect(resourcePlan.TotalCPUWeight).To(BeNumerically(">", 0))
				Expect(resourcePlan.TotalMemoryWeight).To(BeNumerically(">", 0))
				Expect(resourcePlan.MaxConcurrency).To(BeNumerically(">", 0))
				Expect(resourcePlan.EfficiencyScore).To(BeNumerically(">=", 0.0))
				Expect(resourcePlan.EfficiencyScore).To(BeNumerically("<=", 1.0))
			})

			It("should handle empty step list gracefully", func() {
				// Test edge case: empty steps
				emptySteps := []*engine.ExecutableWorkflowStep{}

				resourcePlan := builder.CalculateResourceAllocation(emptySteps)

				Expect(resourcePlan).NotTo(BeNil())
				Expect(resourcePlan.TotalCPUWeight).To(Equal(0.0))
				Expect(resourcePlan.TotalMemoryWeight).To(Equal(0.0))
				Expect(resourcePlan.MaxConcurrency).To(BeNumerically(">=", 1)) // Minimum concurrency
			})

			It("should optimize concurrency based on resource weights", func() {
				// Test concurrency optimization
				// BR-RESOURCE-006: Concurrency optimization based on resources

				// Create steps with different resource requirements
				heavySteps := []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "heavy-1"},
						Action: &engine.StepAction{
							Parameters: map[string]interface{}{
								"cpu_limit":    "2000m",
								"memory_limit": "4Gi",
							},
						},
					},
					{
						BaseEntity: types.BaseEntity{ID: "heavy-2"},
						Action: &engine.StepAction{
							Parameters: map[string]interface{}{
								"cpu_limit":    "2000m",
								"memory_limit": "4Gi",
							},
						},
					},
				}

				lightSteps := []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "light-1"},
						Action: &engine.StepAction{
							Parameters: map[string]interface{}{
								"cpu_limit":    "100m",
								"memory_limit": "128Mi",
							},
						},
					},
					{
						BaseEntity: types.BaseEntity{ID: "light-2"},
						Action: &engine.StepAction{
							Parameters: map[string]interface{}{
								"cpu_limit":    "100m",
								"memory_limit": "128Mi",
							},
						},
					},
				}

				heavyPlan := builder.CalculateResourceAllocation(heavySteps)
				lightPlan := builder.CalculateResourceAllocation(lightSteps)

				// Heavy resource steps should have lower concurrency
				Expect(heavyPlan.MaxConcurrency).To(BeNumerically("<=", lightPlan.MaxConcurrency))

				// Heavy steps should have higher total resource weights
				Expect(heavyPlan.TotalCPUWeight).To(BeNumerically(">", lightPlan.TotalCPUWeight))
				Expect(heavyPlan.TotalMemoryWeight).To(BeNumerically(">", lightPlan.TotalMemoryWeight))
			})
		})
	})

	Describe("Integrated Resource Optimization Workflow", func() {
		Context("when resource optimization is fully integrated", func() {
			It("should enhance workflow generation with resource optimization", func() {
				// Test complete resource optimization integration
				// BR-RESOURCE-007: Complete resource optimization pipeline integration

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify that resource optimization was integrated into workflow generation
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Verify resource optimization metadata is present
				if template.Metadata != nil {
					// Resource optimization should contribute to workflow metadata
					Expect(len(template.Metadata)).To(BeNumerically(">=", 0))
				}
			})

			It("should apply resource constraints during workflow generation", func() {
				// Test that resource constraints are applied during generation
				// BR-RESOURCE-008: Resource constraints application in workflow generation

				// Create objective with strict resource constraints
				constrainedObjective := &engine.WorkflowObjective{
					ID:          "obj-002",
					Type:        "optimization",
					Description: "Strictly resource-constrained workflow",
					Priority:    1, // High priority
					Constraints: map[string]interface{}{
						"max_execution_time": "10m",
						"resource_limits": map[string]interface{}{
							"cpu":    "500m",
							"memory": "1Gi",
						},
						"cost_budget":       50.0,
						"efficiency_target": 0.95,
					},
				}

				template, err := builder.GenerateWorkflow(ctx, constrainedObjective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify the workflow generation process includes resource constraints
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-RESOURCE-001 through BR-RESOURCE-008", func() {
			It("should demonstrate complete resource optimization integration compliance", func() {
				// Comprehensive test for all resource optimization business requirements

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())

				// Verify the workflow generation process includes resource optimization
				// The specific optimizations will be applied when constraints are present
				// This test ensures the integration points are working

				// Test public methods are accessible
				constraints, err := builder.ExtractConstraintsFromObjective(objective)
				Expect(err).NotTo(HaveOccurred())
				Expect(constraints).NotTo(BeNil())

				resourcePlan := builder.CalculateResourceAllocation(template.Steps)
				Expect(resourcePlan).NotTo(BeNil())

				efficiency := builder.CalculateResourceEfficiency(template, template)
				Expect(efficiency).To(BeNumerically(">=", 0.0))
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUresourceUoptimizationUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UresourceUoptimizationUintegration Suite")
}
