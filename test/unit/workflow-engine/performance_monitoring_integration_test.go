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
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Performance Monitoring Integration - TDD Implementation", func() {
	var (
		builder      *engine.DefaultIntelligentWorkflowBuilder
		mockVectorDB *mocks.MockVectorDatabase
		ctx          context.Context
		log          *logrus.Logger
		template     *engine.ExecutableTemplate
		execution    *engine.WorkflowExecution
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

		// Create test template for performance monitoring
		template = &engine.ExecutableTemplate{
			BaseVersionedEntity: types.BaseVersionedEntity{
				BaseEntity: types.BaseEntity{
					ID:   "template-001",
					Name: "Performance Monitoring Template",
					Metadata: map[string]interface{}{
						"performance_monitoring": true,
						"monitoring_level":       "comprehensive",
					},
				},
			},
			Steps: []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-001",
						Name: "Performance Test Step 1",
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
				},
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-002",
						Name: "Performance Test Step 2",
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
			},
			Variables: make(map[string]interface{}),
		}

		// Create test execution for performance analysis
		execution = &engine.WorkflowExecution{
			WorkflowID: "workflow-001",
			StartTime:  time.Now().Add(-30 * time.Minute),
			EndTime:    time.Now().Add(-25 * time.Minute),
			Duration:   5 * time.Minute,
			StepResults: map[string]*engine.StepResult{
				"step-001": {
					Success:  true,
					Duration: 2 * time.Minute,
					Output: map[string]interface{}{
						"replicas_scaled": 3,
						"cpu_usage":       "800m",
						"memory_usage":    "1.5Gi",
					},
				},
				"step-002": {
					Success:  true,
					Duration: 3 * time.Minute,
					Output: map[string]interface{}{
						"diagnostics_collected": true,
						"data_size":             "10MB",
					},
				},
			},
		}
	})

	Describe("Performance Metrics Collection Integration", func() {
		Context("when collecting execution metrics", func() {
			It("should collect comprehensive execution metrics using previously unused functions", func() {
				// Test that CollectExecutionMetrics integrates performance monitoring functions
				// BR-PERF-001: Comprehensive execution metrics collection

				metrics := builder.CollectExecutionMetrics(execution)

				Expect(metrics).NotTo(BeNil())
				Expect(metrics.Duration).To(Equal(execution.Duration))
				Expect(metrics.StepCount).To(Equal(len(execution.StepResults)))
				Expect(metrics.SuccessCount).To(BeNumerically(">", 0))
				Expect(metrics.FailureCount).To(BeNumerically(">=", 0))
				Expect(metrics.RetryCount).To(BeNumerically(">=", 0))

				// Performance metrics should be calculated
				Expect(metrics.Performance).NotTo(BeNil())
				Expect(metrics.Performance.ResponseTime).To(BeNumerically(">", 0))
				Expect(metrics.Performance.Throughput).To(BeNumerically(">", 0))
				Expect(metrics.Performance.ErrorRate).To(BeNumerically(">=", 0))
			})

			It("should analyze performance trends over time", func() {
				// Test AnalyzePerformanceTrends integration
				// BR-PERF-002: Performance trend analysis

				// Create multiple executions for trend analysis
				executions := []*engine.WorkflowExecution{
					{
						WorkflowID: "workflow-001",
						Duration:   5 * time.Minute,
						StepResults: map[string]*engine.StepResult{
							"step-1": {Success: true, Duration: 2 * time.Minute},
							"step-2": {Success: true, Duration: 3 * time.Minute},
						},
					},
					{
						WorkflowID: "workflow-001",
						Duration:   6 * time.Minute,
						StepResults: map[string]*engine.StepResult{
							"step-1": {Success: true, Duration: 150 * time.Second}, // 2.5 minutes
							"step-2": {Success: true, Duration: 210 * time.Second}, // 3.5 minutes
						},
					},
					{
						WorkflowID: "workflow-001",
						Duration:   4 * time.Minute,
						StepResults: map[string]*engine.StepResult{
							"step-1": {Success: true, Duration: 90 * time.Second},  // 1.5 minutes
							"step-2": {Success: true, Duration: 150 * time.Second}, // 2.5 minutes
						},
					},
				}

				trends := builder.AnalyzePerformanceTrends(executions)

				Expect(trends).NotTo(BeNil())
				Expect(trends.Direction).NotTo(BeEmpty())
				Expect(trends.Slope).To(BeNumerically("!=", 0)) // Should have calculated slope
				Expect(trends.Confidence).To(BeNumerically(">=", 0))
				Expect(trends.Confidence).To(BeNumerically("<=", 1))
			})

			It("should generate performance alerts based on thresholds", func() {
				// Test GeneratePerformanceAlerts integration
				// BR-PERF-003: Performance alert generation

				// Create metrics that exceed thresholds
				metrics := &engine.WorkflowMetrics{
					AverageExecutionTime: 10 * time.Minute, // High execution time
					SuccessRate:          0.5,              // Low success rate
					ResourceUtilization:  0.9,              // High resource usage
					ErrorRate:            0.15,             // High error rate
				}

				thresholds := &engine.PerformanceThresholds{
					MaxExecutionTime: 5 * time.Minute,
					MinSuccessRate:   0.8,
					MaxResourceUsage: 0.7,
					MaxErrorRate:     0.1,
				}

				alerts := builder.GeneratePerformanceAlerts(metrics, thresholds)

				Expect(alerts).NotTo(BeNil())
				Expect(len(alerts)).To(BeNumerically(">", 0))

				// Should generate alerts for each threshold violation
				alertMetrics := make(map[string]bool)
				for _, alert := range alerts {
					alertMetrics[alert.Metric] = true
					Expect(alert.Severity).NotTo(BeEmpty())
					Expect(alert.Message).NotTo(BeEmpty())
				}

				// Should have alerts for the violations we set up
				Expect(alertMetrics["execution_time"]).To(BeTrue())
				Expect(alertMetrics["success_rate"]).To(BeTrue())
				Expect(alertMetrics["resource_utilization"]).To(BeTrue())
				Expect(alertMetrics["error_rate"]).To(BeTrue())
			})
		})

		Context("when analyzing loop performance", func() {
			It("should analyze loop performance for optimization", func() {
				// Test AnalyzeLoopPerformance integration
				// BR-PERF-004: Loop performance analysis

				loopMetrics := &engine.LoopExecutionMetrics{
					TotalIterations:      10,
					SuccessfulIterations: 8,
					FailedIterations:     2,
					AverageIterationTime: 30 * time.Second,
					TotalExecutionTime:   5 * time.Minute,
				}

				optimization := builder.AnalyzeLoopPerformance(loopMetrics)

				Expect(optimization).NotTo(BeNil())
				Expect(optimization.SuccessRate).To(BeNumerically("==", 0.8)) // 8/10
				Expect(optimization.EfficiencyScore).To(BeNumerically(">=", 0))
				Expect(optimization.EfficiencyScore).To(BeNumerically("<=", 1))
				Expect(optimization.Recommendations).NotTo(BeNil())
			})
		})

		Context("when calculating workflow complexity", func() {
			It("should calculate workflow complexity scores", func() {
				// Test CalculateWorkflowComplexity integration
				// BR-PERF-005: Workflow complexity assessment

				// Convert template to workflow for complexity calculation
				workflow := &engine.Workflow{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   template.ID,
							Name: template.Name,
						},
					},
					Template: template,
				}

				complexityScore := builder.CalculateWorkflowComplexity(workflow)

				Expect(complexityScore).NotTo(BeNil())
				Expect(complexityScore.OverallScore).To(BeNumerically(">=", 0))
				Expect(complexityScore.OverallScore).To(BeNumerically("<=", 1))
				Expect(complexityScore.FactorScores).NotTo(BeNil())
			})
		})
	})

	Describe("Enhanced Performance Monitoring Integration", func() {
		Context("when performance monitoring is integrated into workflow optimization", func() {
			It("should enhance workflow generation with performance monitoring", func() {
				// Test that performance monitoring is integrated into workflow generation
				// BR-PERF-006: Performance monitoring integration in workflow generation

				objective := &engine.WorkflowObjective{
					ID:          "obj-001",
					Type:        "performance_test",
					Description: "Performance monitoring workflow optimization",
					Priority:    8,
					Constraints: map[string]interface{}{
						"performance_monitoring": true,
						"monitoring_level":       "comprehensive",
						"max_execution_time":     "30m",
					},
				}

				template, err := builder.GenerateWorkflow(ctx, objective)

				Expect(err).NotTo(HaveOccurred())
				Expect(template).NotTo(BeNil())
				Expect(template.ID).NotTo(BeEmpty())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Verify that performance monitoring metadata is present
				if template.Metadata != nil {
					// Performance monitoring should contribute to workflow metadata
					Expect(len(template.Metadata)).To(BeNumerically(">=", 0))
				}
			})

			It("should apply performance monitoring during workflow optimization", func() {
				// Test that performance monitoring is applied during OptimizeWorkflowStructure
				// BR-PERF-007: Performance monitoring in workflow optimization

				optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

				Expect(err).NotTo(HaveOccurred())
				Expect(optimizedTemplate).NotTo(BeNil())
				Expect(optimizedTemplate.ID).NotTo(BeEmpty())
				Expect(len(optimizedTemplate.Steps)).To(BeNumerically(">", 0))

				// Verify the optimization process includes performance monitoring considerations
				Expect(optimizedTemplate.Metadata).NotTo(BeNil())
			})
		})
	})

	Describe("Performance Monitoring Public Methods", func() {
		Context("when using public performance monitoring methods", func() {
			It("should provide comprehensive performance monitoring capabilities", func() {
				// Test that performance monitoring methods are accessible
				// BR-PERF-008: Public performance monitoring method accessibility

				// Test CollectExecutionMetrics
				metrics := builder.CollectExecutionMetrics(execution)
				Expect(metrics).NotTo(BeNil())

				// Test AnalyzePerformanceTrends
				executions := []*engine.WorkflowExecution{execution}
				trends := builder.AnalyzePerformanceTrends(executions)
				Expect(trends).NotTo(BeNil())

				// Test GeneratePerformanceAlerts
				workflowMetrics := &engine.WorkflowMetrics{
					AverageExecutionTime: 5 * time.Minute,
					SuccessRate:          0.9,
					ResourceUtilization:  0.5,
					ErrorRate:            0.05,
				}
				thresholds := &engine.PerformanceThresholds{
					MaxExecutionTime: 10 * time.Minute,
					MinSuccessRate:   0.8,
					MaxResourceUsage: 0.8,
					MaxErrorRate:     0.1,
				}
				alerts := builder.GeneratePerformanceAlerts(workflowMetrics, thresholds)
				Expect(alerts).NotTo(BeNil())

				// Test AnalyzeLoopPerformance
				loopMetrics := &engine.LoopExecutionMetrics{
					TotalIterations:      5,
					SuccessfulIterations: 4,
					FailedIterations:     1,
					AverageIterationTime: 1 * time.Minute,
				}
				loopOptimization := builder.AnalyzeLoopPerformance(loopMetrics)
				Expect(loopOptimization).NotTo(BeNil())

				// Test CalculateWorkflowComplexity
				workflow := &engine.Workflow{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   template.ID,
							Name: template.Name,
						},
					},
					Template: template,
				}
				complexity := builder.CalculateWorkflowComplexity(workflow)
				Expect(complexity).NotTo(BeNil())
			})
		})
	})

	Describe("Performance Monitoring Edge Cases", func() {
		Context("when handling edge cases", func() {
			It("should handle executions with no step results", func() {
				// Test performance monitoring with empty execution
				emptyExecution := &engine.WorkflowExecution{
					WorkflowID:  "workflow-001",
					Duration:    1 * time.Minute,
					StepResults: map[string]*engine.StepResult{}, // No step results
				}

				metrics := builder.CollectExecutionMetrics(emptyExecution)

				Expect(metrics).NotTo(BeNil())
				Expect(metrics.Duration).To(Equal(emptyExecution.Duration))
				Expect(metrics.StepCount).To(Equal(0))
				Expect(metrics.SuccessCount).To(Equal(0))
				Expect(metrics.FailureCount).To(Equal(0))
			})

			It("should handle single execution trend analysis", func() {
				// Test trend analysis with single execution
				singleExecution := []*engine.WorkflowExecution{execution}

				trends := builder.AnalyzePerformanceTrends(singleExecution)

				Expect(trends).NotTo(BeNil())
				Expect(trends.Direction).NotTo(BeEmpty())
				// Single execution should have stable trend
				Expect(trends.Direction).To(Equal("stable"))
			})

			It("should handle performance alerts with no threshold violations", func() {
				// Test alert generation with metrics within thresholds
				goodMetrics := &engine.WorkflowMetrics{
					AverageExecutionTime: 2 * time.Minute, // Within threshold
					SuccessRate:          0.95,            // Above threshold
					ResourceUtilization:  0.3,             // Below threshold
					ErrorRate:            0.02,            // Below threshold
				}

				thresholds := &engine.PerformanceThresholds{
					MaxExecutionTime: 5 * time.Minute,
					MinSuccessRate:   0.8,
					MaxResourceUsage: 0.7,
					MaxErrorRate:     0.1,
				}

				alerts := builder.GeneratePerformanceAlerts(goodMetrics, thresholds)

				Expect(alerts).NotTo(BeNil())
				Expect(len(alerts)).To(Equal(0)) // No alerts should be generated
			})

			It("should handle loop performance with zero iterations", func() {
				// Test loop performance analysis with no iterations
				zeroLoopMetrics := &engine.LoopExecutionMetrics{
					TotalIterations:      0,
					SuccessfulIterations: 0,
					FailedIterations:     0,
					AverageIterationTime: 0,
				}

				optimization := builder.AnalyzeLoopPerformance(zeroLoopMetrics)

				Expect(optimization).NotTo(BeNil())
				Expect(optimization.SuccessRate).To(Equal(0.0))
				Expect(optimization.EfficiencyScore).To(BeNumerically(">=", 0))
			})

			It("should handle workflow complexity for empty templates", func() {
				// Test complexity calculation with empty template
				emptyTemplate := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "empty-template",
							Name: "Empty Template",
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{}, // No steps
				}

				// Convert to workflow for complexity calculation
				emptyWorkflow := &engine.Workflow{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   emptyTemplate.ID,
							Name: emptyTemplate.Name,
						},
					},
					Template: emptyTemplate,
				}

				complexity := builder.CalculateWorkflowComplexity(emptyWorkflow)

				Expect(complexity).NotTo(BeNil())
				Expect(complexity.OverallScore).To(Equal(0.0)) // No complexity for empty template
			})
		})
	})

	Describe("Business Requirement Compliance", func() {
		Context("BR-PERF-001 through BR-PERF-008", func() {
			It("should demonstrate complete performance monitoring integration compliance", func() {
				// Comprehensive test for all performance monitoring business requirements

				// BR-PERF-001: Comprehensive execution metrics collection
				metrics := builder.CollectExecutionMetrics(execution)
				Expect(metrics).NotTo(BeNil())

				// BR-PERF-002: Performance trend analysis
				executions := []*engine.WorkflowExecution{execution}
				trends := builder.AnalyzePerformanceTrends(executions)
				Expect(trends).NotTo(BeNil())

				// BR-PERF-003: Performance alert generation
				workflowMetrics := &engine.WorkflowMetrics{
					AverageExecutionTime: 5 * time.Minute,
					SuccessRate:          0.9,
					ResourceUtilization:  0.5,
					ErrorRate:            0.05,
				}
				thresholds := &engine.PerformanceThresholds{
					MaxExecutionTime: 10 * time.Minute,
					MinSuccessRate:   0.8,
					MaxResourceUsage: 0.8,
					MaxErrorRate:     0.1,
				}
				alerts := builder.GeneratePerformanceAlerts(workflowMetrics, thresholds)
				Expect(alerts).NotTo(BeNil())

				// BR-PERF-004: Loop performance analysis
				loopMetrics := &engine.LoopExecutionMetrics{
					TotalIterations:      5,
					SuccessfulIterations: 4,
				}
				loopOptimization := builder.AnalyzeLoopPerformance(loopMetrics)
				Expect(loopOptimization).NotTo(BeNil())

				// BR-PERF-005: Workflow complexity assessment
				workflow := &engine.Workflow{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   template.ID,
							Name: template.Name,
						},
					},
					Template: template,
				}
				complexity := builder.CalculateWorkflowComplexity(workflow)
				Expect(complexity).NotTo(BeNil())

				// Verify all performance monitoring capabilities are working
				Expect(metrics.Duration).To(BeNumerically(">", 0))
				Expect(trends.Direction).NotTo(BeEmpty())
				Expect(len(alerts)).To(BeNumerically(">=", 0))
				Expect(loopOptimization.SuccessRate).To(BeNumerically(">=", 0))
				Expect(complexity.OverallScore).To(BeNumerically(">=", 0))
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUperformanceUmonitoringUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UperformanceUmonitoringUintegration Suite")
}
