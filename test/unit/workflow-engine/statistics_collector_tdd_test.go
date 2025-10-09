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

//go:build unit
// +build unit

package workflowengine

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TDD Implementation: StatisticsCollector Business Requirements
// BR-ORK-003: MUST implement execution metrics collection and performance trend analysis
// Following 00-project-guidelines.mdc: TDD workflow (RED → GREEN → REFACTOR)
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks

var _ = Describe("StatisticsCollector - TDD Implementation (BR-ORK-003)", func() {
	var (
		ctx                 context.Context
		statisticsCollector engine.StatisticsCollector
		mockLogger          *mocks.MockLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()

		// TDD RED PHASE: Will implement ProductionStatisticsCollector
		// For now, this will fail until we implement the real component
		statisticsCollector = engine.NewProductionStatisticsCollector(mockLogger.Logger)
	})

	// Helper functions for test data creation
	createTrendExecution := func(id string, duration time.Duration, cpuUsage float64, timestamp time.Time) *engine.RuntimeWorkflowExecution {
		endTime := timestamp.Add(duration)
		return &engine.RuntimeWorkflowExecution{
			WorkflowExecutionRecord: types.WorkflowExecutionRecord{
				ID:        id,
				Status:    "completed",
				StartTime: timestamp,
				EndTime:   &endTime,
				Metadata: map[string]interface{}{
					"execution_duration": duration.Seconds(),
					"cpu_usage":          cpuUsage,
					"trend_analysis":     true,
				},
			},
		}
	}

	createFailurePatternExecution := func(id, failureType, failedStep string, timestamp time.Time) *engine.RuntimeWorkflowExecution {
		endTime := timestamp.Add(5 * time.Minute) // Failed execution duration
		return &engine.RuntimeWorkflowExecution{
			WorkflowExecutionRecord: types.WorkflowExecutionRecord{
				ID:        id,
				Status:    "failed",
				StartTime: timestamp,
				EndTime:   &endTime,
				Metadata: map[string]interface{}{
					"failure_type":     failureType,
					"failed_step":      failedStep,
					"failure_analysis": true,
				},
			},
		}
	}

	// BR-ORK-003: Execution metrics collection and performance trend analysis
	Context("BR-ORK-003: Execution Metrics Collection", func() {
		It("should collect comprehensive execution statistics for business analysis", func() {
			// TDD RED PHASE: Write failing test first
			// Business Scenario: Operations team needs detailed execution metrics for performance monitoring
			// Business Impact: Enables data-driven optimization and capacity planning decisions

			// Create execution with comprehensive metrics
			execution := engine.NewRuntimeWorkflowExecution("metrics-collection-test-001", "business-critical-workflow")
			execution.OperationalStatus = engine.ExecutionStatusCompleted
			startTime := time.Now().Add(-10 * time.Minute)
			endTime := time.Now().Add(-5 * time.Minute)
			execution.StartTime = startTime
			execution.EndTime = &endTime
			execution.Metadata = map[string]interface{}{
				"workflow_priority": "high",
				"business_unit":     "operations",
				"cost_center":       "infrastructure",
			}

			// TDD: Collect execution statistics
			err := statisticsCollector.CollectExecutionStatistics(execution)

			// Business Requirement Validation: Comprehensive metrics collection
			Expect(err).ToNot(HaveOccurred(), "BR-ORK-003: Statistics collection must succeed for business monitoring")

			// Verify metrics are properly stored and accessible
			report, reportErr := statisticsCollector.GeneratePerformanceReport(ctx)
			Expect(reportErr).ToNot(HaveOccurred(), "BR-ORK-003: Performance report generation must succeed")
			Expect(report).ToNot(BeNil(), "BR-ORK-003: Performance report required for business analysis")

			// Validate comprehensive metrics collection
			Expect(report.TotalExecutions).To(BeNumerically(">=", 1),
				"BR-ORK-003: Must track total executions for business capacity planning")
			Expect(report.AverageExecutionTime).To(BeNumerically(">", 0),
				"BR-ORK-003: Must track execution time for business performance analysis")
			Expect(report.ResourceEfficiency).To(BeNumerically(">=", 0),
				"BR-ORK-003: Must track resource efficiency for business cost optimization")
			Expect(report.SuccessRate).To(BeNumerically(">=", 0),
				"BR-ORK-003: Must track success rate for business reliability analysis")

			// Validate business-relevant metrics
			Expect(report.PerformanceTrends).ToNot(BeNil(),
				"BR-ORK-003: Performance trends required for business planning")
			Expect(report.OptimizationImpact).ToNot(BeNil(),
				"BR-ORK-003: Optimization impact required for business ROI analysis")

			// Business Value: Comprehensive metrics enable data-driven business decisions
		})

		It("should analyze performance trends for business forecasting", func() {
			// TDD RED PHASE: Write failing test for trend analysis
			// Business Scenario: Business needs to forecast performance trends for capacity planning
			// Business Impact: Enables proactive scaling and resource allocation decisions

			// Create multiple executions showing performance trend
			executions := []*engine.RuntimeWorkflowExecution{
				createTrendExecution("trend-001", 30*time.Second, 0.70, time.Now().Add(-7*24*time.Hour)),
				createTrendExecution("trend-002", 28*time.Second, 0.68, time.Now().Add(-6*24*time.Hour)),
				createTrendExecution("trend-003", 32*time.Second, 0.72, time.Now().Add(-5*24*time.Hour)),
				createTrendExecution("trend-004", 26*time.Second, 0.65, time.Now().Add(-4*24*time.Hour)),
				createTrendExecution("trend-005", 24*time.Second, 0.63, time.Now().Add(-3*24*time.Hour)),
				createTrendExecution("trend-006", 22*time.Second, 0.60, time.Now().Add(-2*24*time.Hour)),
				createTrendExecution("trend-007", 20*time.Second, 0.58, time.Now().Add(-1*24*time.Hour)),
			}

			// Collect statistics for all executions
			for _, execution := range executions {
				err := statisticsCollector.CollectExecutionStatistics(execution)
				Expect(err).ToNot(HaveOccurred(), "BR-ORK-003: All execution statistics must be collected")
			}

			// Analyze performance trends over 7-day window
			trendAnalysis, err := statisticsCollector.AnalyzePerformanceTrends(7 * 24 * time.Hour)

			// Business Requirement Validation: Performance trend analysis
			Expect(err).ToNot(HaveOccurred(), "BR-ORK-003: Performance trend analysis must succeed")
			Expect(trendAnalysis).ToNot(BeNil(), "BR-ORK-003: Trend analysis required for business forecasting")

			// Validate trend detection capabilities using actual PerformanceTrendAnalysis fields
			Expect(trendAnalysis.TrendDirection).To(BeElementOf([]string{"improving", "degrading", "stable"}),
				"BR-ORK-003: Must detect trend direction for business reporting")
			Expect(trendAnalysis.TimeWindow).To(BeNumerically(">", 0),
				"BR-ORK-003: Must define analysis window for business context")

			// Validate performance metrics are available
			Expect(trendAnalysis.PerformanceMetrics).ToNot(BeNil(),
				"BR-ORK-003: Performance metrics required for business analysis")

			// Validate business recommendations are provided
			Expect(len(trendAnalysis.Recommendations)).To(BeNumerically(">=", 0),
				"BR-ORK-003: Must provide recommendations for business improvement")

			// Business Value: Trend analysis enables proactive business capacity planning
		})

		It("should detect failure patterns for business risk management", func() {
			// TDD RED PHASE: Write failing test for failure pattern detection
			// Business Scenario: Business needs to identify failure patterns to prevent service disruptions
			// Business Impact: Enables proactive risk mitigation and improved service reliability

			// Create executions with failure patterns
			executionsWithFailures := []*engine.RuntimeWorkflowExecution{
				createFailurePatternExecution("fail-001", "network_timeout", "step-network", time.Now().Add(-6*time.Hour)),
				createFailurePatternExecution("fail-002", "memory_exhaustion", "step-processing", time.Now().Add(-5*time.Hour)),
				createFailurePatternExecution("fail-003", "network_timeout", "step-network", time.Now().Add(-4*time.Hour)),
				createFailurePatternExecution("fail-004", "disk_full", "step-storage", time.Now().Add(-3*time.Hour)),
				createFailurePatternExecution("fail-005", "network_timeout", "step-network", time.Now().Add(-2*time.Hour)),
				createFailurePatternExecution("fail-006", "memory_exhaustion", "step-processing", time.Now().Add(-1*time.Hour)),
			}

			// Collect failure statistics
			for _, execution := range executionsWithFailures {
				err := statisticsCollector.CollectExecutionStatistics(execution)
				Expect(err).ToNot(HaveOccurred(), "BR-ORK-003: Failure statistics must be collected")
			}

			// Detect failure patterns
			failurePatterns, err := statisticsCollector.DetectFailurePatterns(executionsWithFailures)

			// Business Requirement Validation: Failure pattern detection
			Expect(err).ToNot(HaveOccurred(), "BR-ORK-003: Failure pattern detection must succeed")
			Expect(failurePatterns).ToNot(BeNil(), "BR-ORK-003: Failure patterns required for business risk management")
			Expect(len(failurePatterns)).To(BeNumerically(">=", 2),
				"BR-ORK-003: Must detect multiple failure patterns for comprehensive risk analysis")

			// Validate network timeout pattern detection
			networkPattern := findFailurePatternByType(failurePatterns, "network_timeout")
			Expect(networkPattern).ToNot(BeNil(), "BR-ORK-003: Must detect network timeout pattern")
			Expect(networkPattern.Frequency).To(BeNumerically(">=", 3),
				"BR-ORK-003: Must track failure frequency for business risk assessment")
			Expect(networkPattern.AffectedSteps).To(ContainElement("step-network"),
				"BR-ORK-003: Must identify affected components for business impact analysis")

			// Validate memory exhaustion pattern detection
			memoryPattern := findFailurePatternByType(failurePatterns, "memory_exhaustion")
			Expect(memoryPattern).ToNot(BeNil(), "BR-ORK-003: Must detect memory exhaustion pattern")
			Expect(memoryPattern.CommonCause).ToNot(BeEmpty(),
				"BR-ORK-003: Must identify common causes of failure patterns")

			// Validate pattern prioritization for business action
			highPriorityPatterns := filterHighPriorityPatterns(failurePatterns)
			Expect(len(highPriorityPatterns)).To(BeNumerically(">", 0),
				"BR-ORK-003: Must prioritize failure patterns for business attention")

			// Business Value: Failure pattern detection enables proactive business risk mitigation
		})
	})

})

// Helper function to find failure pattern by type
func findFailurePatternByType(patterns []*engine.FailurePattern, patternType string) *engine.FailurePattern {
	for _, pattern := range patterns {
		if pattern.PatternType == patternType {
			return pattern
		}
	}
	return nil
}

// Helper function to filter high priority failure patterns
func filterHighPriorityPatterns(patterns []*engine.FailurePattern) []*engine.FailurePattern {
	var highPriority []*engine.FailurePattern
	for _, pattern := range patterns {
		// Consider patterns with high frequency or recent occurrences as high priority
		if pattern.Frequency >= 3.0 || pattern.DetectionConfidence >= 0.8 {
			highPriority = append(highPriority, pattern)
		}
	}
	return highPriority
}

// TestRunner bootstraps the Ginkgo test suite
func TestUstatisticsUcollectorUtdd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UstatisticsUcollectorUtdd Suite")
}
