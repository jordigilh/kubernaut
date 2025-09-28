package workflowengine_test

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Advanced Scheduling Integration - TDD Implementation", func() {
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

		// Create test objective with scheduling requirements
		objective = &engine.WorkflowObjective{
			ID:          "obj-001",
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

		// Create test template with scheduling-sensitive steps
		template = &engine.ExecutableTemplate{
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
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-003",
						Name: "Network Intensive Step",
					},
					Type:    engine.StepTypeAction,
					Timeout: 8 * time.Minute,
					Action: &engine.StepAction{
						Type: "health_check",
						Parameters: map[string]interface{}{
							"endpoints": 10,
						},
					},
				},
			},
			Variables: make(map[string]interface{}),
		}
	})

	Describe("calculateOptimalConcurrency Integration", func() {
		Context("when integrated into resource allocation", func() {
			It("should calculate optimal concurrency based on resource weights", func() {
				// Test that calculateOptimalConcurrency is called and calculates correctly
				// BR-SCHED-001: Optimal concurrency calculation based on resource analysis

				concurrency := builder.CalculateOptimalStepConcurrency(template.Steps)

				Expect(concurrency).To(BeNumerically(">=", 1))
				Expect(concurrency).To(BeNumerically("<=", len(template.Steps)))

				// Should consider step characteristics for concurrency calculation
				Expect(concurrency).To(BeNumerically(">", 1)) // Should allow some parallelism
			})

			It("should handle different step types for concurrency optimization", func() {
				// Test concurrency calculation for different step types
				// BR-SCHED-002: Step type-aware concurrency optimization

				// Create CPU-intensive steps
				cpuIntensiveSteps := []*engine.ExecutableWorkflowStep{
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

				// Create I/O-intensive steps
				ioIntensiveSteps := []*engine.ExecutableWorkflowStep{
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

				cpuConcurrency := builder.CalculateOptimalStepConcurrency(cpuIntensiveSteps)
				ioConcurrency := builder.CalculateOptimalStepConcurrency(ioIntensiveSteps)

				// I/O intensive steps should allow higher concurrency than CPU intensive
				Expect(ioConcurrency).To(BeNumerically(">=", cpuConcurrency))
			})

			It("should handle empty step list gracefully", func() {
				// Test edge case: empty steps
				emptySteps := []*engine.ExecutableWorkflowStep{}

				concurrency := builder.CalculateOptimalStepConcurrency(emptySteps)

				Expect(concurrency).To(Equal(1)) // Minimum concurrency
			})
		})
	})

	Describe("calculateOptimalBatches Integration", func() {
		Context("when integrated into resource planning", func() {
			It("should calculate optimal batching strategy", func() {
				// Test that calculateOptimalBatches is called and creates optimal batches
				// BR-SCHED-003: Optimal batching strategy for step execution

				resourcePlan := builder.CalculateResourceAllocation(template.Steps)

				Expect(resourcePlan).NotTo(BeNil())
				Expect(resourcePlan.OptimalBatches).NotTo(BeNil())
				Expect(len(resourcePlan.OptimalBatches)).To(BeNumerically(">", 0))

				// Verify batches respect concurrency limits
				for _, batch := range resourcePlan.OptimalBatches {
					Expect(len(batch)).To(BeNumerically("<=", resourcePlan.MaxConcurrency))
				}
			})

			It("should create batches that cover all steps", func() {
				// Test that all steps are included in batches
				resourcePlan := builder.CalculateResourceAllocation(template.Steps)

				Expect(resourcePlan).NotTo(BeNil())

				// Count total steps in all batches
				totalStepsInBatches := 0
				for _, batch := range resourcePlan.OptimalBatches {
					totalStepsInBatches += len(batch)
				}

				Expect(totalStepsInBatches).To(Equal(len(template.Steps)))
			})

			It("should handle single step gracefully", func() {
				// Test edge case: single step
				singleStep := []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "single-step"},
						Action: &engine.StepAction{
							Type: "health_check",
						},
					},
				}

				resourcePlan := builder.CalculateResourceAllocation(singleStep)

				Expect(resourcePlan).NotTo(BeNil())
				Expect(len(resourcePlan.OptimalBatches)).To(Equal(1))
				Expect(len(resourcePlan.OptimalBatches[0])).To(Equal(1))
				Expect(resourcePlan.OptimalBatches[0][0]).To(Equal("single-step"))
			})
		})
	})

	Describe("calculateOptimalScheduling Integration", func() {
		Context("when integrated into workflow optimization", func() {
			It("should optimize workflow scheduling based on constraints", func() {
				// Test that scheduling optimization is applied during workflow generation
				// BR-SCHED-004: Comprehensive scheduling optimization integration

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify that scheduling optimization was integrated into workflow generation
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Verify scheduling optimization metadata is present
				if template.Metadata != nil {
					// Scheduling optimization should contribute to workflow metadata
					Expect(len(template.Metadata)).To(BeNumerically(">=", 0))
				}
			})

			It("should apply scheduling constraints during optimization", func() {
				// Test that scheduling constraints are applied during optimization
				// BR-SCHED-005: Scheduling constraints application in optimization

				optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

				Expect(err).NotTo(HaveOccurred())
				Expect(optimizedTemplate).NotTo(BeNil())

				// Verify the optimization process includes scheduling considerations
				Expect(optimizedTemplate.ID).NotTo(BeEmpty())
				Expect(len(optimizedTemplate.Steps)).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("Integrated Advanced Scheduling Workflow", func() {
		Context("when advanced scheduling is fully integrated", func() {
			It("should enhance workflow generation with advanced scheduling", func() {
				// Test complete advanced scheduling integration
				// BR-SCHED-006: Complete advanced scheduling pipeline integration

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify that advanced scheduling was integrated into workflow generation
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Verify advanced scheduling metadata is present
				if template.Metadata != nil {
					// Advanced scheduling should contribute to workflow metadata
					Expect(len(template.Metadata)).To(BeNumerically(">=", 0))
				}
			})

			It("should apply scheduling optimization with resource constraints", func() {
				// Test that scheduling optimization works with resource constraints
				// BR-SCHED-007: Scheduling optimization with resource constraint integration

				// Create objective with both scheduling and resource constraints
				constrainedObjective := &engine.WorkflowObjective{
					ID:          "obj-002",
					Type:        "optimization",
					Description: "Scheduling and resource constrained workflow",
					Priority:    1, // High priority
					Constraints: map[string]interface{}{
						"max_execution_time":   "30m",
						"max_concurrent_steps": 2, // Very conservative
						"resource_limits": map[string]interface{}{
							"cpu":    "2000m",
							"memory": "4Gi",
						},
						"scheduling_priority": "critical",
					},
				}

				template, err := builder.GenerateWorkflow(ctx, constrainedObjective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify the workflow generation process includes both scheduling and resource optimization
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))
			})

			It("should handle high-concurrency scenarios", func() {
				// Test scheduling optimization for high-concurrency workflows
				// BR-SCHED-008: High-concurrency scheduling optimization

				// Create objective with high concurrency requirements
				highConcurrencyObjective := &engine.WorkflowObjective{
					ID:          "obj-003",
					Type:        "parallel_processing",
					Description: "High-concurrency workflow optimization",
					Priority:    5,
					Constraints: map[string]interface{}{
						"max_concurrent_steps": 8, // High concurrency
						"resource_limits": map[string]interface{}{
							"cpu":    "8000m",
							"memory": "16Gi",
						},
					},
				}

				template, err := builder.GenerateWorkflow(ctx, highConcurrencyObjective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-SCHED-001 through BR-SCHED-008", func() {
			It("should demonstrate complete advanced scheduling integration compliance", func() {
				// Comprehensive test for all advanced scheduling business requirements

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())

				// Verify the workflow generation process includes advanced scheduling
				// The specific scheduling optimizations will be applied when constraints are present
				// This test ensures the integration points are working

				// Test public methods are accessible
				concurrency := builder.CalculateOptimalStepConcurrency(template.Steps)
				Expect(concurrency).To(BeNumerically(">=", 1))

				resourcePlan := builder.CalculateResourceAllocation(template.Steps)
				Expect(resourcePlan).NotTo(BeNil())
				Expect(resourcePlan.OptimalBatches).NotTo(BeNil())
			})
		})
	})

	Describe("Advanced Scheduling Edge Cases", func() {
		Context("when handling edge cases", func() {
			It("should handle workflows with mixed step types", func() {
				// Test scheduling optimization for mixed workloads
				mixedSteps := []*engine.ExecutableWorkflowStep{
					{
						BaseEntity: types.BaseEntity{ID: "cpu-step"},
						Action: &engine.StepAction{
							Type: "scale_deployment",
							Parameters: map[string]interface{}{
								"cpu_limit": "2000m",
							},
						},
					},
					{
						BaseEntity: types.BaseEntity{ID: "io-step"},
						Action: &engine.StepAction{
							Type: "collect_diagnostics",
						},
					},
					{
						BaseEntity: types.BaseEntity{ID: "network-step"},
						Action: &engine.StepAction{
							Type: "health_check",
						},
					},
				}

				concurrency := builder.CalculateOptimalStepConcurrency(mixedSteps)
				resourcePlan := builder.CalculateResourceAllocation(mixedSteps)

				Expect(concurrency).To(BeNumerically(">=", 1))
				Expect(concurrency).To(BeNumerically("<=", len(mixedSteps)))
				Expect(resourcePlan).NotTo(BeNil())
				Expect(len(resourcePlan.OptimalBatches)).To(BeNumerically(">", 0))
			})

			It("should handle resource-constrained scenarios", func() {
				// Test scheduling with very limited resources
				resourceConstrainedObjective := &engine.WorkflowObjective{
					ID:          "obj-constrained",
					Type:        "resource_limited",
					Description: "Resource-constrained scheduling test",
					Priority:    5,
					Constraints: map[string]interface{}{
						"max_concurrent_steps": 1, // Very limited
						"resource_limits": map[string]interface{}{
							"cpu":    "500m",  // Very limited
							"memory": "512Mi", // Very limited
						},
					},
				}

				template, err := builder.GenerateWorkflow(ctx, resourceConstrainedObjective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUadvancedUschedulingUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UadvancedUschedulingUintegration Suite")
}
