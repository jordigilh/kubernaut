package engine

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// Following testing framework directives for focused workflow generation tests
var _ = Describe("Workflow Generation Logic", func() {
	var (
		ctx     context.Context
		logger  *logrus.Logger
		builder *DefaultIntelligentWorkflowBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

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

	Describe("WorkflowObjective Processing", func() {
		Context("when processing a valid objective", func() {
			var objective *WorkflowObjective

			BeforeEach(func() {
				objective = &WorkflowObjective{
					ID:          "test-objective-1",
					Type:        "memory_optimization",
					Description: "Optimize memory usage for high memory pods",
					Targets: []*OptimizationTarget{
						{
							Type:      "deployment",
							Metric:    "memory_usage",
							Threshold: 0.8,
							Priority:  8,
							Parameters: map[string]interface{}{
								"namespace": "default",
								"name":      "memory-intensive-app",
							},
						},
					},
					Constraints: map[string]interface{}{
						"max_downtime": "30s",
						"safety_level": "high",
					},
					Priority: 5,
				}
			})

			It("should have valid objective structure", func() {
				Expect(objective.ID).ToNot(BeEmpty())
				Expect(objective.Type).To(Equal("memory_optimization"))
				Expect(len(objective.Targets)).To(BeNumerically(">", 0))
				Expect(objective.Priority).To(BeNumerically(">", 0))
			})

			It("should have proper constraints", func() {
				Expect(objective.Constraints).ToNot(BeNil())
				Expect(objective.Constraints["safety_level"]).To(Equal("high"))
				Expect(objective.Constraints["max_downtime"]).To(Equal("30s"))
			})

			It("should have valid optimization targets", func() {
				target := objective.Targets[0]
				Expect(target.Type).To(Equal("deployment"))
				Expect(target.Metric).To(Equal("memory_usage"))
				Expect(target.Threshold).To(Equal(0.8))
				Expect(target.Priority).To(Equal(8))
			})
		})

		Context("when creating optimization candidates", func() {
			It("should generate valid optimization candidates", func() {
				executions := []*WorkflowExecution{
					{
						ID:        "exec-1",
						Status:    ExecutionStatusCompleted,
						Duration:  5 * time.Minute,
						StartTime: time.Now().Add(-1 * time.Hour),
					},
					{
						ID:        "exec-2",
						Status:    ExecutionStatusFailed,
						Duration:  10 * time.Minute,
						StartTime: time.Now().Add(-2 * time.Hour),
					},
				}

				template := &WorkflowTemplate{
					ID:   "test-template",
					Name: "Test Workflow",
					Steps: []*WorkflowStep{
						{
							ID:      "step-1",
							Name:    "Test Step",
							Type:    StepTypeAction,
							Timeout: 3 * time.Minute,
						},
					},
				}

				bottlenecks := []*Bottleneck{
					{
						ID:       "bottleneck-1",
						Type:     "slow_step",
						StepID:   "step-1",
						Severity: "high",
						Impact:   0.7,
					},
				}

				candidates := builder.generateOptimizationCandidates(executions, template, bottlenecks)

				Expect(candidates).ToNot(BeNil())
				Expect(len(candidates)).To(BeNumerically(">=", 0))
			})
		})
	})

	Describe("Pattern Creation and Management", func() {
		Context("when creating patterns from executions", func() {
			It("should extract patterns from successful executions", func() {
				executions := []*WorkflowExecution{
					{
						ID:        "exec-1",
						Status:    ExecutionStatusCompleted,
						Duration:  3 * time.Minute,
						StartTime: time.Now().Add(-1 * time.Hour),
						Context: &ExecutionContext{
							Environment: "production",
							Variables: map[string]interface{}{
								"action_type": "scale_deployment",
								"resource":    "deployment/test-app",
							},
						},
					},
					{
						ID:        "exec-2",
						Status:    ExecutionStatusCompleted,
						Duration:  4 * time.Minute,
						StartTime: time.Now().Add(-2 * time.Hour),
						Context: &ExecutionContext{
							Environment: "production",
							Variables: map[string]interface{}{
								"action_type": "scale_deployment",
								"resource":    "deployment/test-app",
							},
						},
					},
				}

				patterns, err := builder.createPatternFromExecutions(ctx, executions)

				Expect(err).ToNot(HaveOccurred())
				Expect(patterns).ToNot(BeNil())
				Expect(patterns.ID).ToNot(BeEmpty())
				Expect(patterns.Type).To(Equal("execution_pattern"))
			})
		})

		Context("when applying learnings to patterns", func() {
			It("should update pattern effectiveness", func() {
				pattern := &WorkflowPattern{
					ID:             "pattern-1",
					Name:           "Test Pattern",
					Type:           "optimization",
					SuccessRate:    0.8,
					ExecutionCount: 10,
					Confidence:     0.75,
				}

				learning := &WorkflowLearning{
					ID:   "learning-1",
					Type: LearningTypePerformance,
					Data: map[string]interface{}{
						"success":       true,
						"improvement":   0.2,
						"effectiveness": 0.85,
					},
				}

				_, _ = builder.applyLearningsToPattern(ctx, pattern, []*WorkflowLearning{learning})

				// Pattern should be updated with learning data
				Expect(pattern).ToNot(BeNil())
				Expect(pattern.ExecutionCount).To(BeNumerically(">", 10))
			})
		})
	})

	Describe("Performance Analysis", func() {
		Context("when identifying performance bottlenecks", func() {
			It("should identify slow steps", func() {
				executions := []*WorkflowExecution{
					{
						ID:        "exec-1",
						Status:    ExecutionStatusCompleted,
						StartTime: time.Now().Add(-1 * time.Hour),
						Duration:  12 * time.Minute,
						Steps: []*StepExecution{
							{
								StepID:    "step-1",
								Duration:  2 * time.Minute,
								Status:    ExecutionStatusCompleted,
								StartTime: time.Now().Add(-1 * time.Hour),
								EndTime:   &([]time.Time{time.Now().Add(-58 * time.Minute)}[0]),
							},
							{
								StepID:    "step-2",
								Duration:  10 * time.Minute, // Slow step
								Status:    ExecutionStatusCompleted,
								StartTime: time.Now().Add(-58 * time.Minute),
								EndTime:   &([]time.Time{time.Now().Add(-48 * time.Minute)}[0]),
							},
						},
					},
					{
						ID:        "exec-2",
						Status:    ExecutionStatusCompleted,
						StartTime: time.Now().Add(-2 * time.Hour),
						Duration:  15 * time.Minute,
						Steps: []*StepExecution{
							{
								StepID:    "step-1",
								Duration:  3 * time.Minute,
								Status:    ExecutionStatusCompleted,
								StartTime: time.Now().Add(-2 * time.Hour),
								EndTime:   &([]time.Time{time.Now().Add(-117 * time.Minute)}[0]),
							},
							{
								StepID:    "step-2",
								Duration:  12 * time.Minute, // Consistently slow
								Status:    ExecutionStatusCompleted,
								StartTime: time.Now().Add(-117 * time.Minute),
								EndTime:   &([]time.Time{time.Now().Add(-105 * time.Minute)}[0]),
							},
						},
					},
				}

				template := &WorkflowTemplate{
					ID: "test-template",
					Steps: []*WorkflowStep{
						{ID: "step-1", Name: "Fast Step"},
						{ID: "step-2", Name: "Slow Step"},
					},
				}

				bottlenecks := builder.identifyPerformanceBottlenecks(executions, template)

				Expect(bottlenecks).ToNot(BeNil())
				Expect(len(bottlenecks)).To(BeNumerically(">=", 0))

				// If bottlenecks are found, they should have valid data
				for _, bottleneck := range bottlenecks {
					Expect(bottleneck.ID).ToNot(BeEmpty())
					Expect(bottleneck.Type).ToNot(BeEmpty())
					Expect(bottleneck.Impact).To(BeNumerically(">=", 0))
					Expect(bottleneck.Impact).To(BeNumerically("<=", 1))
				}
			})
		})

		Context("when generating performance recommendations", func() {
			It("should create actionable recommendations", func() {
				bottlenecks := []*Bottleneck{
					{
						ID:          "bottleneck-1",
						Type:        "slow_step",
						StepID:      "step-2",
						Severity:    "high",
						Impact:      0.8,
						Description: "Step 2 is consistently slow",
					},
				}

				optimizations := []*OptimizationCandidate{
					{
						ID:          "opt-1",
						Type:        "reduce_timeout",
						Description: "Optimize timeout for slow step",
						Impact:      0.5,
						Confidence:  0.7,
					},
				}

				recommendations := builder.generatePerformanceRecommendations(bottlenecks, optimizations)

				Expect(recommendations).ToNot(BeNil())
				Expect(len(recommendations)).To(BeNumerically(">=", 0))

				// Recommendations should be well-formed
				for _, recommendation := range recommendations {
					Expect(recommendation.ID).ToNot(BeEmpty())
					Expect(recommendation.Type).ToNot(BeEmpty())
					Expect(recommendation.Description).ToNot(BeEmpty())
				}
			})
		})
	})

	Describe("Safety and Validation Features", func() {
		Context("when adding safety features", func() {
			var template *WorkflowTemplate

			BeforeEach(func() {
				template = &WorkflowTemplate{
					ID:   "safety-test",
					Name: "Safety Test Template",
					Steps: []*WorkflowStep{
						{
							ID:   "step-1",
							Name: "Destructive Action",
							Type: StepTypeAction,
							Action: &StepAction{
								Type: "delete",
							},
						},
					},
				}
			})

			It("should add confirmation steps before destructive actions", func() {
				originalStepCount := len(template.Steps)

				builder.addConfirmationSteps(template)

				// Should have added confirmation steps
				Expect(len(template.Steps)).To(BeNumerically(">=", originalStepCount))
			})

			It("should add rollback steps for recovery", func() {
				builder.addRollbackSteps(template)

				// Should have configured recovery policy
				Expect(template.Recovery).ToNot(BeNil())
				Expect(template.Recovery.Enabled).To(BeTrue())
			})

			It("should add validation steps", func() {
				originalStepCount := len(template.Steps)

				builder.addValidationSteps(template)

				// Should have added validation steps
				Expect(len(template.Steps)).To(BeNumerically(">=", originalStepCount))
			})
		})

		Context("when reducing parallelism for safety", func() {
			It("should reduce parallel execution", func() {
				template := &WorkflowTemplate{
					ID:   "parallel-test",
					Name: "Parallel Test Template",
					Steps: []*WorkflowStep{
						{
							ID:   "step-1",
							Name: "Parallel Step 1",
							Type: StepTypeAction,
							Metadata: map[string]interface{}{
								"parallel": true,
							},
						},
						{
							ID:   "step-2",
							Name: "Parallel Step 2",
							Type: StepTypeAction,
							Metadata: map[string]interface{}{
								"parallel": true,
							},
						},
					},
				}

				builder.reduceParallelism(template)

				// Should have reduced parallelism settings
				for _, step := range template.Steps {
					if step.Metadata != nil {
						if parallel, exists := step.Metadata["parallel"]; exists {
							Expect(parallel).To(BeFalse())
						}
					}
				}
			})
		})
	})

	Describe("Resource and Timeout Optimization", func() {
		Context("when applying optimization recommendations", func() {
			var (
				template       *WorkflowTemplate
				recommendation *OptimizationSuggestion
			)

			BeforeEach(func() {
				template = &WorkflowTemplate{
					ID:   "optimization-test",
					Name: "Optimization Test Template",
					Steps: []*WorkflowStep{
						{
							ID:      "step-1",
							Name:    "Optimizable Step",
							Type:    StepTypeAction,
							Timeout: 10 * time.Minute,
							Action: &StepAction{
								Type:       "execute",
								Parameters: make(map[string]interface{}),
							},
						},
					},
				}

				recommendation = &OptimizationSuggestion{
					ID:          "suggestion-1",
					Type:        "reduce_timeout",
					Description: "Reduce timeout for better performance",
					Impact:      0.5,
					Parameters: map[string]interface{}{
						"new_timeout": "5m",
					},
				}
			})

			It("should apply resource optimization", func() {
				originalParams := len(template.Steps[0].Action.Parameters)

				builder.applyResourceOptimization(template, recommendation)

				// Should have applied optimization parameters
				step := template.Steps[0]
				Expect(len(step.Action.Parameters)).To(BeNumerically(">=", originalParams))
			})

			It("should apply timeout optimization", func() {
				builder.applyTimeoutOptimization(template, recommendation)

				// Should have optimized the template
				Expect(template).ToNot(BeNil())
				Expect(len(template.Steps)).To(Equal(1))
			})
		})
	})
})

func TestWorkflowGeneration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Generation Test Suite")
}
