package engine

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// Following testing framework directives for BDD-style unit tests
var _ = Describe("IntelligentWorkflowBuilder Unit Tests", func() {
	var (
		logger  *logrus.Logger
		builder *DefaultIntelligentWorkflowBuilder
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create a basic builder instance for testing helper methods
		builder = &DefaultIntelligentWorkflowBuilder{
			log: logger,
			config: &WorkflowBuilderConfig{
				MaxWorkflowSteps:      20,
				DefaultStepTimeout:    5 * time.Minute,
				MaxRetries:            3,
				MinPatternSimilarity:  0.8,
				MinExecutionCount:     5,
				MinSuccessRate:        0.7,
				PatternLookbackDays:   30,
				EnableSafetyChecks:    true,
				EnableSimulation:      true,
				ValidationTimeout:     2 * time.Minute,
				EnableLearning:        true,
				LearningBatchSize:     100,
				PatternUpdateInterval: time.Hour,
			},
		}
	})

	Describe("Configuration and Initialization", func() {
		Context("when creating a new workflow builder", func() {
			It("should initialize with default configuration", func() {
				Expect(builder.config).ToNot(BeNil())
				Expect(builder.config.MaxWorkflowSteps).To(Equal(20))
				Expect(builder.config.DefaultStepTimeout).To(Equal(5 * time.Minute))
				Expect(builder.config.EnableSafetyChecks).To(BeTrue())
			})

			It("should have proper logging configured", func() {
				Expect(builder.log).ToNot(BeNil())
				Expect(builder.log.Level).To(Equal(logrus.ErrorLevel))
			})
		})
	})

	Describe("Helper Method Functionality", func() {
		Context("when applying safety constraints", func() {
			var template *WorkflowTemplate

			BeforeEach(func() {
				template = &WorkflowTemplate{
					ID:   "test-template",
					Name: "Test Template",
					Steps: []*WorkflowStep{
						{
							ID:   "step-1",
							Name: "Test Step",
							Type: StepTypeAction,
							Action: &StepAction{
								Type: "scale_deployment",
							},
							Timeout: 3 * time.Minute,
						},
					},
				}
			})

			It("should apply high safety constraints", func() {
				builder.applySafetyConstraints(template, "high")

				// High safety should add additional safety measures
				// This is testing the helper method works without errors
				Expect(template).ToNot(BeNil())
				Expect(len(template.Steps)).To(BeNumerically(">=", 1))
			})

			It("should apply medium safety constraints", func() {
				builder.applySafetyConstraints(template, "medium")

				Expect(template).ToNot(BeNil())
				Expect(len(template.Steps)).To(BeNumerically(">=", 1))
			})

			It("should apply low safety constraints", func() {
				builder.applySafetyConstraints(template, "low")

				Expect(template).ToNot(BeNil())
				Expect(len(template.Steps)).To(BeNumerically(">=", 1))
			})
		})

		Context("when optimizing step ordering", func() {
			var template *WorkflowTemplate

			BeforeEach(func() {
				template = &WorkflowTemplate{
					ID:   "test-template",
					Name: "Test Template",
					Steps: []*WorkflowStep{
						{
							ID:   "step-1",
							Name: "Action Step",
							Type: StepTypeAction,
							Action: &StepAction{
								Type: "cleanup",
							},
						},
						{
							ID:   "step-2",
							Name: "Validation Step",
							Type: StepTypeAction,
							Action: &StepAction{
								Type: "validate",
							},
						},
						{
							ID:   "step-3",
							Name: "Execution Step",
							Type: StepTypeAction,
							Action: &StepAction{
								Type: "execute",
							},
						},
					},
				}
			})

			It("should reorder steps for optimal execution", func() {
				originalStepCount := len(template.Steps)

				builder.optimizeStepOrdering(template)

				Expect(len(template.Steps)).To(Equal(originalStepCount))
				// After optimization, validation steps should come first
				firstStep := template.Steps[0]
				Expect(firstStep.Action.Type).To(Equal("validate"))
			})

			It("should handle templates with single steps", func() {
				singleStepTemplate := &WorkflowTemplate{
					ID:   "single-step",
					Name: "Single Step Template",
					Steps: []*WorkflowStep{
						{
							ID:   "only-step",
							Name: "Only Step",
							Type: StepTypeAction,
							Action: &StepAction{
								Type: "execute",
							},
						},
					},
				}

				builder.optimizeStepOrdering(singleStepTemplate)

				Expect(len(singleStepTemplate.Steps)).To(Equal(1))
				Expect(singleStepTemplate.Steps[0].ID).To(Equal("only-step"))
			})

			It("should handle empty templates gracefully", func() {
				emptyTemplate := &WorkflowTemplate{
					ID:    "empty",
					Name:  "Empty Template",
					Steps: []*WorkflowStep{},
				}

				builder.optimizeStepOrdering(emptyTemplate)

				Expect(len(emptyTemplate.Steps)).To(Equal(0))
			})
		})

		Context("when adjusting timeouts for max duration", func() {
			var template *WorkflowTemplate

			BeforeEach(func() {
				template = &WorkflowTemplate{
					ID:   "timeout-test",
					Name: "Timeout Test Template",
					Steps: []*WorkflowStep{
						{
							ID:      "step-1",
							Name:    "Long Step",
							Type:    StepTypeAction,
							Timeout: 10 * time.Minute,
						},
						{
							ID:      "step-2",
							Name:    "Another Long Step",
							Type:    StepTypeAction,
							Timeout: 15 * time.Minute,
						},
					},
				}
			})

			It("should adjust step timeouts to fit within max duration", func() {
				maxDuration := 20 * time.Minute

				builder.adjustTimeoutsForMaxDuration(template, maxDuration)

				// Calculate total timeout after adjustment
				totalTimeout := time.Duration(0)
				for _, step := range template.Steps {
					totalTimeout += step.Timeout
				}

				// Should be within the max duration (including buffer)
				Expect(totalTimeout).To(BeNumerically("<=", maxDuration))
			})

			It("should set overall execution timeout", func() {
				maxDuration := 20 * time.Minute

				builder.adjustTimeoutsForMaxDuration(template, maxDuration)

				Expect(template.Timeouts).ToNot(BeNil())
				Expect(template.Timeouts.Execution).To(Equal(maxDuration))
			})
		})

		Context("when applying resource constraints", func() {
			var template *WorkflowTemplate

			BeforeEach(func() {
				template = &WorkflowTemplate{
					ID:   "resource-test",
					Name: "Resource Test Template",
					Steps: []*WorkflowStep{
						{
							ID:   "step-1",
							Name: "Resource Step",
							Type: StepTypeAction,
							Action: &StepAction{
								Type:       "scale_deployment",
								Parameters: make(map[string]interface{}),
							},
						},
					},
				}
			})

			It("should apply CPU limits", func() {
				limits := map[string]interface{}{
					"cpu": "2000m",
				}

				builder.applyResourceConstraints(template, limits)

				step := template.Steps[0]
				Expect(step.Action.Parameters["cpu_limit"]).To(Equal("2000m"))
			})

			It("should apply memory limits", func() {
				limits := map[string]interface{}{
					"memory": "4Gi",
				}

				builder.applyResourceConstraints(template, limits)

				step := template.Steps[0]
				Expect(step.Action.Parameters["memory_limit"]).To(Equal("4Gi"))
			})

			It("should apply multiple resource limits", func() {
				limits := map[string]interface{}{
					"cpu":     "1000m",
					"memory":  "2Gi",
					"storage": "10Gi",
				}

				builder.applyResourceConstraints(template, limits)

				step := template.Steps[0]
				Expect(step.Action.Parameters["cpu_limit"]).To(Equal("1000m"))
				Expect(step.Action.Parameters["memory_limit"]).To(Equal("2Gi"))
				Expect(step.Action.Parameters["storage_limit"]).To(Equal("10Gi"))
			})
		})
	})

	Describe("Pattern Management", func() {
		Context("when calculating learning success rate", func() {
			It("should calculate success rate correctly", func() {
				learnings := []*WorkflowLearning{
					{
						ID: "learning-1",
						Data: map[string]interface{}{
							"success": true,
						},
					},
					{
						ID: "learning-2",
						Data: map[string]interface{}{
							"success": false,
						},
					},
					{
						ID: "learning-3",
						Data: map[string]interface{}{
							"success": true,
						},
					},
				}

				successRate := builder.calculateLearningSuccessRate(learnings)

				Expect(successRate).To(BeNumerically("~", 0.666, 0.01))
			})

			It("should handle empty learnings", func() {
				learnings := []*WorkflowLearning{}

				successRate := builder.calculateLearningSuccessRate(learnings)

				Expect(successRate).To(Equal(0.0))
			})

			It("should handle learnings without success field", func() {
				learnings := []*WorkflowLearning{
					{
						ID: "learning-1",
						Data: map[string]interface{}{
							"other_field": "value",
						},
					},
				}

				successRate := builder.calculateLearningSuccessRate(learnings)

				Expect(successRate).To(Equal(0.0))
			})
		})

		Context("when calculating cost efficiency", func() {
			It("should calculate efficiency correctly", func() {
				usage := &ResourceUsageMetrics{
					CPUUsage:    100.0,
					MemoryUsage: 50.0,
					DiskUsage:   25.0,
					NetworkIO:   10.0,
				}

				efficiency := builder.calculateCostEfficiency(usage, 0.8)

				Expect(efficiency).To(BeNumerically(">", 0.0))
				Expect(efficiency).To(BeNumerically("<=", 1.0))
			})

			It("should handle zero effectiveness", func() {
				usage := &ResourceUsageMetrics{
					CPUUsage: 100.0,
				}

				efficiency := builder.calculateCostEfficiency(usage, 0.0)

				Expect(efficiency).To(Equal(0.0))
			})

			It("should handle nil usage", func() {
				efficiency := builder.calculateCostEfficiency(nil, 0.8)

				Expect(efficiency).To(Equal(0.0))
			})
		})
	})

	Describe("Error Handling and Edge Cases", func() {
		Context("when dealing with nil values", func() {
			It("should handle nil template in safety constraints", func() {
				Expect(func() {
					builder.applySafetyConstraints(nil, "high")
				}).ToNot(Panic())
			})

			It("should handle nil template in timeout adjustment", func() {
				Expect(func() {
					builder.adjustTimeoutsForMaxDuration(nil, 10*time.Minute)
				}).ToNot(Panic())
			})

			It("should handle nil template in resource constraints", func() {
				limits := map[string]interface{}{"cpu": "1000m"}

				Expect(func() {
					builder.applyResourceConstraints(nil, limits)
				}).ToNot(Panic())
			})
		})

		Context("when dealing with invalid input", func() {
			It("should handle empty safety level", func() {
				template := &WorkflowTemplate{
					ID:    "test",
					Steps: []*WorkflowStep{},
				}

				Expect(func() {
					builder.applySafetyConstraints(template, "")
				}).ToNot(Panic())
			})

			It("should handle zero max duration", func() {
				template := &WorkflowTemplate{
					ID:    "test",
					Steps: []*WorkflowStep{},
				}

				Expect(func() {
					builder.adjustTimeoutsForMaxDuration(template, 0)
				}).ToNot(Panic())
			})

			It("should handle nil resource limits", func() {
				template := &WorkflowTemplate{
					ID:    "test",
					Steps: []*WorkflowStep{},
				}

				Expect(func() {
					builder.applyResourceConstraints(template, nil)
				}).ToNot(Panic())
			})
		})
	})

	Describe("Performance Optimization Features", func() {
		Context("when aggregating resource usage", func() {
			It("should aggregate usage metrics correctly", func() {
				total := &ResourceUsageMetrics{
					CPUUsage:    100.0,
					MemoryUsage: 200.0,
					DiskUsage:   50.0,
					NetworkIO:   25.0,
				}

				usage := &ResourceUsageMetrics{
					CPUUsage:    50.0,
					MemoryUsage: 100.0,
					DiskUsage:   25.0,
					NetworkIO:   15.0,
				}

				builder.aggregateResourceUsage(total, usage)

				Expect(total.CPUUsage).To(Equal(150.0))
				Expect(total.MemoryUsage).To(Equal(300.0))
				Expect(total.DiskUsage).To(Equal(75.0))
				Expect(total.NetworkIO).To(Equal(40.0))
			})

			It("should handle nil total metrics", func() {
				usage := &ResourceUsageMetrics{CPUUsage: 100.0}

				Expect(func() {
					builder.aggregateResourceUsage(nil, usage)
				}).ToNot(Panic())
			})

			It("should handle nil usage metrics", func() {
				total := &ResourceUsageMetrics{CPUUsage: 100.0}

				Expect(func() {
					builder.aggregateResourceUsage(total, nil)
				}).ToNot(Panic())

				// Total should remain unchanged
				Expect(total.CPUUsage).To(Equal(100.0))
			})
		})
	})
})

// Test suite setup following testing framework directives
func TestIntelligentWorkflowBuilderUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "IntelligentWorkflowBuilder Unit Test Suite")
}
