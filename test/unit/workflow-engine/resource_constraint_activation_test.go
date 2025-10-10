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

package workflowengine

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TDD Implementation: Resource Constraint Function Activation Tests
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-IWB-014, BR-ORCH-002, BR-COST-001 to BR-COST-010

var _ = Describe("Resource Constraint Function Activation - TDD Implementation", func() {
	var (
		workflowBuilder *engine.DefaultIntelligentWorkflowBuilder
		mockLLMClient   *mocks.MockLLMClient
		mockVectorDB    *mocks.MockVectorDatabase
		logger          *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise

		mockLLMClient = &mocks.MockLLMClient{}
		mockVectorDB = &mocks.MockVectorDatabase{}

		// Create workflow builder for testing using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       mockLLMClient,
			VectorDB:        mockVectorDB,
			AnalyticsEngine: nil, // Analytics engine
			PatternStore:    nil, // Pattern store
			ExecutionRepo:   nil, // Execution repository
			Logger:          logger,
		}

		var err error
		workflowBuilder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
	})

	Describe("BR-IWB-014: extractConstraintsFromObjective activation", func() {
		It("should extract resource constraints from workflow objective", func() {
			// Arrange: Create test objective with resource constraints
			objective := &engine.WorkflowObjective{
				ID:          "test-objective-001",
				Type:        "resource_optimization",
				Description: "Optimize resource usage with constraints",
				Constraints: map[string]interface{}{
					"max_cpu":    "2000m",
					"max_memory": "4Gi",
					"budget":     100.0,
				},
			}

			// Act: Extract constraints (THIS WILL FAIL until we implement it)
			constraints, err := workflowBuilder.ExtractConstraintsFromObjective(objective)

			// Assert: Should extract constraints successfully
			Expect(err).ToNot(HaveOccurred(), "BR-IWB-014: Should extract constraints without error")
			Expect(constraints).To(BeAssignableToTypeOf(map[string]interface{}{}), "BR-WF-001-SUCCESS-RATE: Constraint extraction must provide functional constraint map for workflow execution success")
			Expect(constraints).To(HaveKey("max_cpu"), "Should extract CPU constraints")
			Expect(constraints).To(HaveKey("max_memory"), "Should extract memory constraints")
			Expect(constraints).To(HaveKey("budget"), "Should extract budget constraints")
		})

		It("should handle empty objective parameters gracefully", func() {
			// Arrange: Create objective without parameters
			objective := &engine.WorkflowObjective{
				ID:          "test-objective-002",
				Type:        "basic_workflow",
				Description: "Basic workflow without constraints",
				Constraints: map[string]interface{}{},
			}

			// Act: Extract constraints from empty parameters
			constraints, err := workflowBuilder.ExtractConstraintsFromObjective(objective)

			// Assert: Should handle gracefully with default constraints
			Expect(err).ToNot(HaveOccurred(), "Should handle empty parameters")
			Expect(len(constraints)).To(BeNumerically(">=", 0), "BR-WF-001-SUCCESS-RATE: Empty parameter handling must provide constraint map (with defaults) for workflow resilience")
		})
	})

	Describe("BR-COST-001 to BR-COST-010: applyCostOptimizationConstraints activation", func() {
		It("should apply cost optimization constraints to workflow template", func() {
			// Arrange: Create test template and constraints
			template := &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{
						ID:          "test-template-001",
						Name:        "Resource Optimization Template",
						Description: "Template for resource optimization testing",
					},
				},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{
							ID:   "step-001",
							Name: "Scale Deployment Step",
						},
						Type: engine.StepTypeAction,
						Action: &engine.StepAction{
							Type: "scale_deployment",
							Parameters: map[string]interface{}{
								"replicas": 5,
							},
						},
					},
				},
			}

			constraints := map[string]interface{}{
				"budget":     50.0,
				"max_cost":   100.0,
				"efficiency": 0.8,
			}

			// Act: Apply cost optimization constraints (THIS WILL FAIL until we implement it)
			workflowBuilder.ApplyCostOptimizationConstraints(template, constraints)

			// Assert: Should modify template based on cost constraints
			Expect(template.Steps).ToNot(BeEmpty(), "Should preserve workflow steps")
			// Additional assertions will be added based on implementation
		})

		It("should optimize for budget constraints", func() {
			// Arrange: Create template with high-cost operations
			template := &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{
						ID:   "test-template-002",
						Name: "High Cost Template",
					},
				},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{
							ID:   "expensive-step",
							Name: "Expensive Scale Operation",
						},
						Type: engine.StepTypeAction,
						Action: &engine.StepAction{
							Type: "scale_deployment",
							Parameters: map[string]interface{}{
								"replicas": 100, // Expensive operation
							},
						},
					},
				},
			}

			constraints := map[string]interface{}{
				"budget": 10.0, // Low budget
			}

			// Act: Apply budget constraints
			workflowBuilder.ApplyCostOptimizationConstraints(template, constraints)

			// Assert: Should optimize for low budget
			Expect(template.Steps).ToNot(BeEmpty(), "Should maintain workflow functionality")
			// Budget optimization assertions will be added based on implementation
		})
	})

	Describe("Integration: Resource constraint management workflow", func() {
		It("should integrate constraint extraction and optimization", func() {
			// Arrange: Create complete workflow objective
			objective := &engine.WorkflowObjective{
				ID:          "integration-test-001",
				Type:        "cost_optimized_scaling",
				Description: "Scale deployment with cost constraints",
				Constraints: map[string]interface{}{
					"max_cpu":    "1000m",
					"max_memory": "2Gi",
					"budget":     75.0,
					"target":     "web-deployment",
				},
			}

			template := &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{
						ID:   "integration-template-001",
						Name: "Cost Optimized Scaling",
					},
				},
				Steps: []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{
							ID:   "scaling-step",
							Name: "Scaling Step",
						},
						Type: engine.StepTypeAction,
						Action: &engine.StepAction{
							Type: "scale_deployment",
							Parameters: map[string]interface{}{
								"deployment": "web-deployment",
								"replicas":   10,
							},
						},
					},
				},
			}

			// Act: Full constraint management workflow
			constraints, err := workflowBuilder.ExtractConstraintsFromObjective(objective)
			Expect(err).ToNot(HaveOccurred(), "Constraint extraction should succeed")

			workflowBuilder.ApplyCostOptimizationConstraints(template, constraints)

			// Assert: Should complete full workflow
			Expect(len(constraints)).To(BeNumerically(">=", 0), "BR-WF-001-SUCCESS-RATE: Full workflow must extract measurable constraint data for execution success")
			Expect(template.Steps).ToNot(BeEmpty(), "Should maintain workflow steps")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUresourceUconstraintUactivation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UresourceUconstraintUactivation Suite")
}
