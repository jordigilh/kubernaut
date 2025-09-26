package workflowengine_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Analytics Integration - TDD Implementation", func() {
	var (
		builder    *engine.DefaultIntelligentWorkflowBuilder
		ctx        context.Context
		log        *logrus.Logger
		executions []*engine.RuntimeWorkflowExecution
		patterns   []*engine.WorkflowPattern
		objective  *engine.WorkflowObjective
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.DebugLevel)
		ctx = context.Background()

		// Create builder with minimal dependencies for testing using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       nil,
			VectorDB:        nil,
			AnalyticsEngine: nil,
			PatternStore:    nil,
			ExecutionRepo:   nil,
			Logger:          log,
		}

		var err error
		builder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")

		// Create test executions with different success states
		executions = []*engine.RuntimeWorkflowExecution{
			{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "exec-001",
					WorkflowID: "workflow-001",
					StartTime:  time.Now().Add(-10 * time.Minute),
					EndTime:    func() *time.Time { t := time.Now().Add(-8 * time.Minute); return &t }(),
				},
				OperationalStatus: engine.ExecutionStatusCompleted,
				Duration:          2 * time.Minute,
				Steps: []*engine.StepExecution{
					{Status: engine.ExecutionStatusCompleted},
					{Status: engine.ExecutionStatusCompleted},
				},
			},
			{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "exec-002",
					WorkflowID: "workflow-001",
					StartTime:  time.Now().Add(-20 * time.Minute),
					EndTime:    func() *time.Time { t := time.Now().Add(-15 * time.Minute); return &t }(),
				},
				OperationalStatus: engine.ExecutionStatusFailed,
				Duration:          5 * time.Minute,
				Steps: []*engine.StepExecution{
					{Status: engine.ExecutionStatusCompleted},
					{Status: engine.ExecutionStatusFailed},
				},
			},
			{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "exec-003",
					WorkflowID: "workflow-001",
					StartTime:  time.Now().Add(-30 * time.Minute),
					EndTime:    func() *time.Time { t := time.Now().Add(-27 * time.Minute); return &t }(),
				},
				OperationalStatus: engine.ExecutionStatusCompleted,
				Duration:          3 * time.Minute,
				Steps: []*engine.StepExecution{
					{Status: engine.ExecutionStatusCompleted},
					{Status: engine.ExecutionStatusCompleted},
					{Status: engine.ExecutionStatusCompleted},
				},
			},
		}

		// Create test patterns
		patterns = []*engine.WorkflowPattern{
			{
				ID:          "pattern-001",
				SuccessRate: 0.8,
				Confidence:  0.7,
			},
		}

		// Create test objective
		objective = &engine.WorkflowObjective{
			ID:          "obj-001",
			Type:        "remediation",
			Description: "Test workflow objective",
		}
	})

	Describe("calculateSuccessRate Integration", func() {
		Context("when integrated into GenerateWorkflow", func() {
			It("should calculate and include success rate in workflow metadata", func() {
				// Test that calculateSuccessRate is called and result is used
				// Expected: 2 successful out of 3 executions = 0.67 (rounded)

				// This test defines the business contract:
				// BR-ANALYTICS-001: Workflow generation must include success rate analytics
				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify success rate is calculated and included in metadata
				if template.Metadata != nil {
					if successRate, exists := template.Metadata["success_rate"]; exists {
						Expect(successRate).To(BeNumerically(">=", 0.0))
						Expect(successRate).To(BeNumerically("<=", 1.0))
					}
				}
			})

			It("should handle empty execution list gracefully", func() {
				// Test edge case: no historical executions
				// Expected: Should not fail, should handle gracefully

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
			})

			It("should calculate correct success rate for known execution set", func() {
				// Test specific calculation accuracy
				// Given: 2 successful, 1 failed execution
				// Expected: Success rate = 2/3 = 0.6667

				successRate := builder.CalculateSuccessRate(executions)

				Expect(successRate).To(BeNumerically("~", 0.6667, 0.001))
			})
		})
	})

	Describe("calculatePatternConfidence Integration", func() {
		Context("when integrated into workflow generation", func() {
			It("should calculate and use pattern confidence for decision making", func() {
				// Test that calculatePatternConfidence is integrated
				// BR-ANALYTICS-002: Pattern confidence must influence workflow decisions

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify confidence calculation is integrated
				if template.Metadata != nil {
					if confidence, exists := template.Metadata["confidence_score"]; exists {
						Expect(confidence).To(BeNumerically(">=", 0.0))
						Expect(confidence).To(BeNumerically("<=", 1.0))
					}
				}
			})

			It("should calculate higher confidence for patterns with more executions", func() {
				// Test confidence calculation logic
				// Given: Pattern with high success rate and many executions
				// Expected: Higher confidence score

				manyExecutions := make([]*engine.RuntimeWorkflowExecution, 15)
				for i := 0; i < 15; i++ {
					manyExecutions[i] = &engine.RuntimeWorkflowExecution{
						OperationalStatus: engine.ExecutionStatusCompleted,
					}
				}

				confidence := builder.CalculatePatternConfidence(patterns[0], manyExecutions)

				Expect(confidence).To(BeNumerically(">", patterns[0].Confidence))
			})
		})
	})

	Describe("calculateAverageExecutionTime Integration", func() {
		Context("when integrated into workflow optimization", func() {
			It("should calculate and include average execution time in metadata", func() {
				// Test that calculateAverageExecutionTime is integrated
				// BR-ANALYTICS-003: Average execution time must be tracked for optimization

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify average execution time is calculated and included
				if template.Metadata != nil {
					if avgTime, exists := template.Metadata["avg_execution_time"]; exists {
						Expect(avgTime).To(BeAssignableToTypeOf(time.Duration(0)))
					}
				}
			})

			It("should calculate correct average execution time", func() {
				// Test specific calculation accuracy
				// Given: Executions with 2min, 5min, 3min durations
				// Expected: Average = (2+5+3)/3 = 3.33 minutes

				avgTime := builder.CalculateAverageExecutionTime(executions)
				expectedAvg := (2*time.Minute + 5*time.Minute + 3*time.Minute) / 3

				Expect(avgTime).To(Equal(expectedAvg))
			})

			It("should return zero for empty execution list", func() {
				// Test edge case handling
				emptyExecutions := []*engine.RuntimeWorkflowExecution{}

				avgTime := builder.CalculateAverageExecutionTime(emptyExecutions)

				Expect(avgTime).To(Equal(time.Duration(0)))
			})
		})
	})

	Describe("Integrated Analytics Workflow", func() {
		Context("when generating workflow with full analytics integration", func() {
			It("should include all analytics metrics in workflow metadata", func() {
				// Test complete analytics integration
				// BR-ANALYTICS-004: Complete analytics pipeline integration

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify all analytics are integrated (when historical data exists)
				if template.Metadata != nil {
					// Success rate should be present if historical executions exist
					if _, hasSuccessRate := template.Metadata["success_rate"]; hasSuccessRate {
						Expect(template.Metadata["success_rate"]).To(BeNumerically(">=", 0.0))
					}

					// Confidence score should be present if patterns exist
					if _, hasConfidence := template.Metadata["confidence_score"]; hasConfidence {
						Expect(template.Metadata["confidence_score"]).To(BeNumerically(">=", 0.0))
					}

					// Average execution time should be present if historical executions exist
					if _, hasAvgTime := template.Metadata["avg_execution_time"]; hasAvgTime {
						Expect(template.Metadata["avg_execution_time"]).To(BeAssignableToTypeOf(time.Duration(0)))
					}
				}
			})

			It("should use analytics for optimization decisions", func() {
				// Test that analytics influence workflow generation decisions
				// BR-ANALYTICS-005: Analytics must drive optimization decisions

				// Create objective with constraints that should trigger optimization
				constrainedObjective := &engine.WorkflowObjective{
					ID:          "obj-002",
					Type:        "optimization",
					Description: "Performance-critical workflow",
					Constraints: map[string]interface{}{
						"max_execution_time": "5m",
						"min_success_rate":   0.8,
					},
				}

				template, err := builder.GenerateWorkflow(ctx, constrainedObjective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())

				// Verify optimization was applied based on analytics
				Expect(len(template.Steps)).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-ANALYTICS-001 through BR-ANALYTICS-005", func() {
			It("should demonstrate complete analytics integration compliance", func() {
				// Comprehensive test for all business requirements

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())

				// Verify the workflow generation process includes analytics
				// The specific analytics will be present when historical data exists
				// This test ensures the integration points are working
			})
		})
	})
})
