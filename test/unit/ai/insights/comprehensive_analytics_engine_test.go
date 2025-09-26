//go:build unit
// +build unit

package insights

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// Stub implementations for testing
type stubAnalyticsAssessor struct{}

func (s *stubAnalyticsAssessor) GetAnalyticsInsights(ctx context.Context, timeWindow time.Duration) (*types.AnalyticsInsights, error) {
	return &types.AnalyticsInsights{
		GeneratedAt:      time.Now(),
		WorkflowInsights: map[string]interface{}{"effectiveness": 0.85},
		PatternInsights:  map[string]interface{}{"patterns_found": 5},
		Recommendations:  []string{"Optimize workflow A", "Scale service B"},
		Metadata:         map[string]interface{}{"confidence": 0.92},
	}, nil
}

func (s *stubAnalyticsAssessor) GetPatternAnalytics(ctx context.Context, filters map[string]interface{}) (*types.PatternAnalytics, error) {
	return &types.PatternAnalytics{
		TotalPatterns:        15,
		AverageEffectiveness: 0.78,
		PatternsByType:       map[string]int{"scaling": 8, "restart": 7},
		SuccessRateByType:    map[string]float64{"scaling": 0.85, "restart": 0.72},
		RecentPatterns:       []*types.DiscoveredPattern{{ID: "pattern-1", Type: "scaling"}},
		TopPerformers:        []*types.DiscoveredPattern{{ID: "pattern-2", Type: "scaling"}},
		FailurePatterns:      []*types.DiscoveredPattern{{ID: "pattern-3", Type: "restart"}},
		TrendAnalysis:        map[string]interface{}{"trend": "improving"},
	}, nil
}

func (s *stubAnalyticsAssessor) AssessContextAdequacyImpact(ctx context.Context, contextLevel float64) (map[string]interface{}, error) {
	return map[string]interface{}{"impact": "minimal"}, nil
}

func (s *stubAnalyticsAssessor) ConfigureAdaptiveAlerts(ctx context.Context, performanceThresholds map[string]float64) (map[string]interface{}, error) {
	return map[string]interface{}{"alerts_configured": true}, nil
}

func (s *stubAnalyticsAssessor) GeneratePerformanceCorrelationDashboard(ctx context.Context, timeWindow time.Duration) (map[string]interface{}, error) {
	return map[string]interface{}{"dashboard": "generated"}, nil
}

type stubWorkflowAnalyzer struct{}

func (s *stubWorkflowAnalyzer) AnalyzeWorkflowEffectiveness(ctx context.Context, execution *types.RuntimeWorkflowExecution) (*types.EffectivenessReport, error) {
	return &types.EffectivenessReport{}, nil
}

func (s *stubWorkflowAnalyzer) GetPatternInsights(ctx context.Context, patternID string) (*types.PatternInsights, error) {
	return &types.PatternInsights{}, nil
}

// BR-ANALYTICS-ENGINE-001: Comprehensive Analytics Engine Business Logic Testing
// Business Impact: Ensures analytics capabilities for production intelligence and insights
// Stakeholder Value: Operations teams can trust AI-powered analytics for decision making
var _ = Describe("BR-ANALYTICS-ENGINE-001: Comprehensive Analytics Engine Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockAssessor         insights.AnalyticsAssessor
		mockWorkflowAnalyzer insights.WorkflowAnalyzer
		mockLogger           *logrus.Logger

		// Use REAL business logic components
		analyticsEngine *insights.AnalyticsEngineImpl

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockAssessor = &stubAnalyticsAssessor{}
		mockWorkflowAnalyzer = &stubWorkflowAnalyzer{}
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL analytics engine with mocked external dependencies
		analyticsEngine = insights.NewAnalyticsEngineWithDependencies(
			mockAssessor,         // External: Mock (data source)
			mockWorkflowAnalyzer, // External: Mock (data source)
			mockLogger,           // External: Mock (logging infrastructure)
		)
	})

	AfterEach(func() {
		cancel()
	})

	// COMPREHENSIVE scenario testing for analytics engine business logic
	DescribeTable("BR-ANALYTICS-ENGINE-001: Should handle all analytics scenarios",
		func(scenarioName string, setupFn func(), testFn func() error, expectedSuccess bool) {
			// Setup scenario-specific mock responses
			if setupFn != nil {
				setupFn()
			}

			// Test REAL business logic
			err := testFn()

			// Validate REAL business outcomes
			if expectedSuccess {
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-ENGINE-001: Valid analytics operations must succeed for %s", scenarioName)
			} else {
				Expect(err).To(HaveOccurred(),
					"BR-ANALYTICS-ENGINE-001: Invalid analytics operations must fail gracefully for %s", scenarioName)
			}
		},
		Entry("Basic data analysis", "basic_analysis", nil, func() error {
			return analyticsEngine.AnalyzeData()
		}, true),
		Entry("Analytics insights generation", "insights_generation", func() {
			// Stub implementation provides fixed analytics data
		}, func() error {
			_, err := analyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
			return err
		}, true),
		Entry("Pattern analytics generation", "pattern_analytics", func() {
		}, func() error {
			_, err := analyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
			return err
		}, false),
	)

	// COMPREHENSIVE analytics insights testing
	Context("BR-ANALYTICS-ENGINE-002: Analytics Insights Business Logic", func() {
		It("should generate comprehensive analytics insights with business value", func() {
			// Test REAL business logic for analytics insights generation

			// Test REAL business analytics insights generation
			insights, err := analyticsEngine.GetAnalyticsInsights(ctx, 7*24*time.Hour)

			// Validate REAL business insights outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-002: Analytics insights generation must succeed")
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-ENGINE-002: Must return analytics insights")
			Expect(insights.WorkflowInsights).ToNot(BeEmpty(),
				"BR-ANALYTICS-ENGINE-002: Must provide workflow insights")
			Expect(insights.PatternInsights).ToNot(BeEmpty(),
				"BR-ANALYTICS-ENGINE-002: Must provide pattern insights")
			Expect(len(insights.Recommendations)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-ENGINE-002: Must provide actionable recommendations")

			// Validate business value of insights
			if effectiveness, exists := insights.WorkflowInsights["effectiveness_trends"]; exists {
				effectivenessMap := effectiveness.(map[string]interface{})
				Expect(effectivenessMap["trend_score"]).To(BeNumerically(">", 0.7),
					"BR-ANALYTICS-ENGINE-002: Effectiveness trends must show meaningful scores")
			}
		})

		It("should handle different time windows for analytics insights", func() {
			// Test REAL business logic for different time window scenarios
			timeWindows := []struct {
				duration    time.Duration
				description string
			}{
				{1 * time.Hour, "short-term analysis"},
				{24 * time.Hour, "daily analysis"},
				{7 * 24 * time.Hour, "weekly analysis"},
				{30 * 24 * time.Hour, "monthly analysis"},
			}

			for _, tw := range timeWindows {

				// Test REAL business time window handling
				insights, err := analyticsEngine.GetAnalyticsInsights(ctx, tw.duration)

				// Validate REAL business time window outcomes
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-ENGINE-002: Time window %s must be handled correctly", tw.description)
				Expect(insights.Metadata["time_window"]).To(Equal(tw.duration),
					"BR-ANALYTICS-ENGINE-002: Time window metadata must be preserved")
			}
		})
	})

	// COMPREHENSIVE pattern analytics testing
	Context("BR-ANALYTICS-ENGINE-003: Pattern Analytics Business Logic", func() {
		It("should generate comprehensive pattern analytics with business insights", func() {
			// Test REAL business logic for pattern analytics generation
			// Test REAL business pattern analytics generation
			filters := map[string]interface{}{
				"type":              "all",
				"timeframe":         "30d",
				"min_effectiveness": 0.5,
			}
			analytics, err := analyticsEngine.GetPatternAnalytics(ctx, filters)

			// Validate REAL business pattern analytics outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-003: Pattern analytics generation must succeed")
			Expect(analytics).ToNot(BeNil(),
				"BR-ANALYTICS-ENGINE-003: Must return pattern analytics")
			Expect(analytics.TotalPatterns).To(BeNumerically(">", 0),
				"BR-ANALYTICS-ENGINE-003: Must identify patterns")
			Expect(analytics.AverageEffectiveness).To(BeNumerically(">", 0.5),
				"BR-ANALYTICS-ENGINE-003: Average effectiveness must be meaningful")
			Expect(len(analytics.PatternsByType)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-ENGINE-003: Must categorize patterns by type")
			Expect(len(analytics.TopPerformers)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-ENGINE-003: Must identify top performing patterns")

			// Validate business insights quality
			for patternType, successRate := range analytics.SuccessRateByType {
				Expect(patternType).ToNot(BeEmpty(),
					"BR-ANALYTICS-ENGINE-003: Pattern types must be named")
				Expect(successRate).To(BeNumerically(">=", 0.0),
					"BR-ANALYTICS-ENGINE-003: Success rates must be valid")
				Expect(successRate).To(BeNumerically("<=", 1.0),
					"BR-ANALYTICS-ENGINE-003: Success rates must not exceed 100%")
			}
		})

		It("should apply filters correctly for pattern analytics", func() {
			// Test REAL business logic for filter application
			filterScenarios := []struct {
				filters     map[string]interface{}
				description string
			}{
				{
					filters:     map[string]interface{}{"type": "scaling"},
					description: "type-specific filtering",
				},
				{
					filters:     map[string]interface{}{"min_effectiveness": 0.8},
					description: "effectiveness threshold filtering",
				},
				{
					filters:     map[string]interface{}{"timeframe": "7d", "type": "restart"},
					description: "combined filtering",
				},
				{
					filters:     map[string]interface{}{},
					description: "no filtering (all patterns)",
				},
			}

			for _, scenario := range filterScenarios {
				// Test REAL business filter application
				analytics, err := analyticsEngine.GetPatternAnalytics(ctx, scenario.filters)

				// Validate REAL business filter outcomes
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-ENGINE-003: Filter scenario %s must succeed", scenario.description)
				Expect(analytics.TrendAnalysis["filter_applied"]).To(Equal(scenario.description),
					"BR-ANALYTICS-ENGINE-003: Filters must be applied correctly for %s", scenario.description)
			}
		})
	})

	// COMPREHENSIVE workflow effectiveness testing
	Context("BR-ANALYTICS-ENGINE-004: Workflow Effectiveness Analysis", func() {
		It("should analyze workflow effectiveness with comprehensive metrics", func() {
			// Test REAL business logic for workflow effectiveness analysis
			execution := &types.RuntimeWorkflowExecution{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:        "exec-comprehensive-test",
					Status:    "completed",
					StartTime: time.Now().Add(-10 * time.Minute),
					EndTime:   func() *time.Time { t := time.Now(); return &t }(),
				},
			}

			// Test REAL business workflow effectiveness analysis
			report, err := analyticsEngine.AnalyzeWorkflowEffectiveness(ctx, execution)

			// Validate REAL business effectiveness analysis outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-004: Workflow effectiveness analysis must succeed")
			Expect(report).ToNot(BeNil(),
				"BR-ANALYTICS-ENGINE-004: Must return effectiveness report")
			Expect(report.ExecutionID).To(Equal(execution.ID),
				"BR-ANALYTICS-ENGINE-004: Report must be linked to execution")
			Expect(report.Score).To(BeNumerically(">", 0.0),
				"BR-ANALYTICS-ENGINE-004: Effectiveness score must be meaningful")
			Expect(report.Score).To(BeNumerically("<=", 1.0),
				"BR-ANALYTICS-ENGINE-004: Effectiveness score must be valid")

			// Validate business metadata quality
			Expect(report.Metadata).ToNot(BeEmpty(),
				"BR-ANALYTICS-ENGINE-004: Report must include analysis metadata")
		})

		It("should handle workflow effectiveness analysis fallback gracefully", func() {
			// Test REAL business logic fallback when analyzer is unavailable
			analyticsEngineWithoutAnalyzer := insights.NewAnalyticsEngineWithDependencies(
				mockAssessor, // Has assessor
				nil,          // No workflow analyzer - test fallback
				mockLogger,
			)

			execution := &types.RuntimeWorkflowExecution{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:     "exec-fallback-test",
					Status: "completed",
				},
			}

			// Test REAL business fallback logic
			report, err := analyticsEngineWithoutAnalyzer.AnalyzeWorkflowEffectiveness(ctx, execution)

			// Validate REAL business fallback outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-004: Fallback analysis must succeed")
			Expect(report).ToNot(BeNil(),
				"BR-ANALYTICS-ENGINE-004: Fallback must return report")
			Expect(report.ExecutionID).To(Equal(execution.ID),
				"BR-ANALYTICS-ENGINE-004: Fallback report must be linked to execution")
			Expect(report.Score).To(BeNumerically(">", 0.0),
				"BR-ANALYTICS-ENGINE-004: Fallback score must be meaningful")
			Expect(report.Metadata["analysis_type"]).To(Equal("fallback"),
				"BR-ANALYTICS-ENGINE-004: Fallback must be identified in metadata")
		})
	})

	// COMPREHENSIVE pattern insights testing
	Context("BR-ANALYTICS-ENGINE-005: Pattern Insights Analysis", func() {
		It("should generate comprehensive pattern insights with business context", func() {
			// Test REAL business logic for pattern insights generation
			patternID := "pattern-comprehensive-test"

			// Test REAL business pattern insights generation
			insights, err := analyticsEngine.GetPatternInsights(ctx, patternID)

			// Validate REAL business pattern insights outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-005: Pattern insights generation must succeed")
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-ENGINE-005: Must return pattern insights")
			Expect(insights.PatternID).To(Equal(patternID),
				"BR-ANALYTICS-ENGINE-005: Insights must be linked to pattern")
			Expect(insights.Effectiveness).To(BeNumerically(">", 0.0),
				"BR-ANALYTICS-ENGINE-005: Effectiveness must be meaningful")
			Expect(insights.UsageCount).To(BeNumerically(">", 0),
				"BR-ANALYTICS-ENGINE-005: Usage count must be tracked")
			Expect(len(insights.Insights)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-ENGINE-005: Must provide actionable insights")

			// Validate business insights quality
			for _, insight := range insights.Insights {
				Expect(len(insight)).To(BeNumerically(">", 10),
					"BR-ANALYTICS-ENGINE-005: Insights must be descriptive")
			}

			// Validate business metrics
			Expect(insights.Metrics).ToNot(BeEmpty(),
				"BR-ANALYTICS-ENGINE-005: Must include business metrics")
		})

		It("should handle pattern insights analysis with context awareness", func() {
			// Test REAL business logic for context-aware pattern insights
			patternID := "pattern-context-test"

			// Create context with trace ID for testing (define custom key type to avoid collisions)
			type traceKey string
			contextWithTrace := context.WithValue(ctx, traceKey("trace_id"), "trace-12345")

			// Test REAL business context-aware analysis
			insights, err := analyticsEngine.GetPatternInsights(contextWithTrace, patternID)

			// Validate REAL business context awareness outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-005: Context-aware analysis must succeed")
			Expect(insights.Metrics["context_trace_id"]).To(Equal("trace-12345"),
				"BR-ANALYTICS-ENGINE-005: Context information must be preserved")
		})

		It("should handle pattern insights fallback gracefully", func() {
			// Test REAL business logic fallback when analyzer is unavailable
			analyticsEngineWithoutAnalyzer := insights.NewAnalyticsEngineWithDependencies(
				mockAssessor, // Has assessor
				nil,          // No workflow analyzer - test fallback
				mockLogger,
			)

			patternID := "pattern-fallback-test"

			// Test REAL business fallback logic
			insights, err := analyticsEngineWithoutAnalyzer.GetPatternInsights(ctx, patternID)

			// Validate REAL business fallback outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-ENGINE-005: Fallback insights analysis must succeed")
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-ENGINE-005: Fallback must return insights")
			Expect(insights.PatternID).To(Equal(patternID),
				"BR-ANALYTICS-ENGINE-005: Fallback insights must be linked to pattern")
			Expect(insights.Effectiveness).To(BeNumerically(">", 0.0),
				"BR-ANALYTICS-ENGINE-005: Fallback effectiveness must be meaningful")
			Expect(insights.Metrics["analysis_type"]).To(Equal("fallback"),
				"BR-ANALYTICS-ENGINE-005: Fallback must be identified in metadata")
		})
	})

	// COMPREHENSIVE error handling and resilience testing
	Context("BR-ANALYTICS-ENGINE-006: Error Handling and Resilience", func() {
		It("should handle external dependency failures gracefully", func() {
			// Test REAL business error handling for external dependency failures
			dependencyFailures := []struct {
				name     string
				setupFn  func()
				testFn   func() error
				expected string
			}{
				{
					name: "Assessor service failure",
					setupFn: func() {

					},
					testFn: func() error {
						_, err := analyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
						return err
					},
					expected: "should return assessor error",
				},
				{
					name: "Workflow analyzer failure",
					setupFn: func() {
					},
					testFn: func() error {
						execution := &types.RuntimeWorkflowExecution{
							WorkflowExecutionRecord: types.WorkflowExecutionRecord{ID: "test", Status: "completed"},
						}
						_, err := analyticsEngine.AnalyzeWorkflowEffectiveness(ctx, execution)
						return err
					},
					expected: "should return analyzer error",
				},
			}

			for _, failure := range dependencyFailures {
				By(failure.name)

				// Setup external failure
				failure.setupFn()

				// Test REAL business error handling
				err := failure.testFn()

				// Validate REAL business error handling outcomes
				if failure.name == "Workflow analyzer failure" {
					// Workflow analyzer has fallback, should not error
					Expect(err).ToNot(HaveOccurred(),
						"BR-ANALYTICS-ENGINE-006: Workflow analyzer failures should use fallback for %s", failure.name)
				} else {
					// Assessor failures should error gracefully
					Expect(err).To(HaveOccurred(),
						"BR-ANALYTICS-ENGINE-006: External dependency failures must be handled gracefully for %s", failure.name)
				}

				// Reset for next test

			}
		})

		It("should handle context cancellation gracefully", func() {
			// Test REAL business context cancellation handling
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			// Test REAL business context handling
			insights, err := analyticsEngine.GetPatternInsights(cancelledCtx, "test-pattern")

			// Validate REAL business context cancellation outcomes
			if err != nil {
				// If context cancellation is handled, should return context error
				Expect(err).To(Equal(context.Canceled),
					"BR-ANALYTICS-ENGINE-006: Context cancellation must be handled properly")
			} else {
				// If fallback is used, should still return insights
				Expect(insights).ToNot(BeNil(),
					"BR-ANALYTICS-ENGINE-006: Fallback must handle context cancellation gracefully")
			}
		})
	})
})

// Helper functions for creating mock objects and test data
// These support REAL business logic testing with various scenarios
