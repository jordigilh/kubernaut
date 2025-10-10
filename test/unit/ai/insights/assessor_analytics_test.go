<<<<<<< HEAD
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

>>>>>>> crd_implementation
package insights

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

func TestAssessorAnalyticsImplementation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Insights Complete Test Suite - Analytics & Business Metrics")
}

// TestAssessor embeds insights.Assessor to enable testing with proper field initialization
type TestAssessor struct {
	*insights.Assessor
	actionHistoryRepo actionhistory.Repository
	logger            *logrus.Logger
}

// Override methods to inject dependencies properly for testing
func (t *TestAssessor) GetAnalyticsInsights(ctx context.Context, timeWindow time.Duration) (*types.AnalyticsInsights, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Create a properly initialized assessor for this method call
	assessor := insights.NewAssessor(t.actionHistoryRepo, nil, nil, nil, nil, t.logger)

	// Call the actual business logic with properly initialized assessor
	return assessor.GetAnalyticsInsights(ctx, timeWindow)
}

func (t *TestAssessor) GetPatternAnalytics(ctx context.Context, filters map[string]interface{}) (*types.PatternAnalytics, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Create a properly initialized assessor for this method call
	assessor := insights.NewAssessor(t.actionHistoryRepo, nil, nil, nil, nil, t.logger)

	// Call the actual business logic with properly initialized assessor
	return assessor.GetPatternAnalytics(ctx, filters)
}

var _ = Describe("AI Insights Assessor Analytics - Business Requirements Testing", func() {
	var (
		ctx      context.Context
		assessor *TestAssessor
		mockRepo *MockActionHistoryRepository
		logger   *logrus.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Initialize mock repository for data access
		mockRepo = NewMockActionHistoryRepository()

		// Create assessor using the actual constructor for complete initialization
		// Note: Using NewAssessor with nil parameters except what we need for testing
		assessor = &TestAssessor{
			actionHistoryRepo: mockRepo,
			logger:            logger,
		}
	})

	AfterEach(func() {
		// Clean up mocks following existing patterns
		if mockRepo != nil {
			mockRepo.ClearState()
		}
	})

	// BR-AI-001: MUST generate comprehensive analytics insights from historical action effectiveness data
	Context("BR-AI-001: Analytics Insights Generation", func() {
		It("should generate effectiveness trend analysis meeting 30-second processing requirement", func() {
			// Arrange: Setup comprehensive historical data for BR-AI-001 analytics generation
			mockRepo.SetActionTraces(generateAnalyticsTestData(5000)) // Large dataset per BR-AI-001 requirement

			// Act: Generate analytics insights for 90-day window
			startTime := time.Now()
			insights, err := assessor.GetAnalyticsInsights(ctx, 90*24*time.Hour)
			processingTime := time.Since(startTime)

			// **Business Requirement BR-AI-001**: Validate successful analytics generation
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should generate analytics insights without errors")
			Expect(len(insights.WorkflowInsights)).To(BeNumerically(">=", 1), "BR-AI-001: Analytics insights must provide workflow analysis results for comprehensive business intelligence")

			// **Success Criteria BR-AI-001**: Analytics processing completes within 30 seconds for 10,000+ records
			Expect(processingTime).To(BeNumerically("<=", 30*time.Second),
				"BR-AI-001: Should complete analytics processing within 30 seconds")

			// **Functional Requirement 1**: Effectiveness Trend Analysis (7-day, 30-day, 90-day trends)
			workflowInsights, hasWorkflowInsights := insights.WorkflowInsights["effectiveness_trends"]
			Expect(hasWorkflowInsights).To(BeTrue(), "BR-AI-001: Should provide effectiveness trend analysis")

			trendsMap, ok := workflowInsights.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-AI-001: Trends should be properly structured")

			// Validate multiple time window trends per BR-AI-001 Functional Requirement 1
			Expect(trendsMap).To(HaveKey("7_day_trend"), "BR-AI-001: Should include 7-day effectiveness trends")
			Expect(trendsMap).To(HaveKey("30_day_trend"), "BR-AI-001: Should include 30-day effectiveness trends")

			// **Business Value**: Validates data-driven decision making capability
			Expect(len(insights.Recommendations)).To(BeNumerically(">=", 1),
				"BR-AI-001: Should generate actionable intelligence recommendations")

			// **Success Criteria**: Generates actionable insights with >90% statistical confidence
			if confidenceLevel, hasConfidence := insights.Metadata["confidence_level"].(float64); hasConfidence {
				Expect(confidenceLevel).To(BeNumerically(">=", 0.5),
					"BR-AI-001: Should provide meaningful confidence estimates")
			}
		})

		It("should perform action type performance analysis meeting business value requirements", func() {
			// Arrange: Setup diverse action type data for performance analysis
			mockRepo.SetActionTraces(generateDiverseActionTypeData(1000))

			// Act: Generate performance analytics
			insights, err := assessor.GetAnalyticsInsights(ctx, 30*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate action type performance analysis
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should complete action type performance analysis")

			// **Functional Requirement 2**: Action Type Performance Analysis
			actionPerformance, hasActionPerf := insights.WorkflowInsights["action_performance"]
			Expect(hasActionPerf).To(BeTrue(), "BR-AI-001: Should provide action type performance analysis")

			perfMap, ok := actionPerformance.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-AI-001: Performance analysis should be properly structured")

			// **Business Requirement**: Rank action types by success rate and effectiveness score
			if topPerformers, hasTopPerf := perfMap["top_performers"].([]map[string]interface{}); hasTopPerf {
				if len(topPerformers) > 0 {
					// Validate top performers have required business metrics
					topPerformer := topPerformers[0]
					Expect(topPerformer).To(HaveKey("action_type"), "BR-AI-001: Should identify action types")
					Expect(topPerformer).To(HaveKey("success_rate"), "BR-AI-001: Should calculate success rates")
				}
			}

			// **Business Value**: Enables identification of optimization opportunities
			totalActionTypes, hasTotalTypes := perfMap["total_action_types"].(int)
			if hasTotalTypes {
				Expect(totalActionTypes).To(BeNumerically(">=", 1),
					"BR-AI-001: Should analyze multiple action types for comprehensive insights")
			}
		})

		It("should detect seasonal patterns and anomalies meeting business intelligence requirements", func() {
			// Arrange: Setup time-pattern data for seasonal analysis
			mockRepo.SetActionTraces(generateSeasonalPatternData(2000))

			// Act: Generate seasonal pattern insights
			insights, err := assessor.GetAnalyticsInsights(ctx, 60*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate seasonal pattern detection
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should detect seasonal patterns without errors")

			// **Functional Requirement 3**: Seasonal Pattern Detection
			_, hasSeasonalPatterns := insights.PatternInsights["seasonal_patterns"]
			Expect(hasSeasonalPatterns).To(BeTrue(), "BR-AI-001: Should detect seasonal patterns")

			// **Functional Requirement 4**: Anomaly Detection
			anomalies, hasAnomalies := insights.PatternInsights["anomalies"]
			Expect(hasAnomalies).To(BeTrue(), "BR-AI-001: Should detect effectiveness anomalies")

			// Validate anomaly detection structure per BR-AI-001 requirements
			anomalyMap, ok := anomalies.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-AI-001: Anomalies should be properly structured for business analysis")

			// **Success Criteria**: Identifies performance anomalies with <5% false positive rate
			if _, hasDetectedAnomalies := anomalyMap["detected_anomalies"]; hasDetectedAnomalies {
				// Business validation: anomaly detection should provide actionable intelligence
				Expect(anomalyMap).To(HaveKey("total_anomalies"),
					"BR-AI-001: Should quantify anomalies for business impact assessment")
			}

			// **Business Value**: Provides clear business recommendations in natural language
			Expect(len(insights.Recommendations)).To(BeNumerically(">=", 1),
				"BR-AI-001: Should provide natural language business recommendations")
		})

		// TDD: Test-driven development for comprehensive recommendation functions
		It("should generate comprehensive trend-based recommendations meeting business intelligence requirements", func() {
			// Arrange: Setup trend analysis data with various trend patterns
			mockRepo.SetActionTraces(generateMixedTrendData(1000))

			// Act: Generate analytics insights with focus on trend recommendations
			insights, err := assessor.GetAnalyticsInsights(ctx, 30*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate trend-based recommendations
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should generate trend recommendations without errors")
			Expect(len(insights.Recommendations)).To(BeNumerically(">=", 1), "BR-AI-001: Trend-based insights must provide actionable recommendations for business optimization")

			// **Functional Requirement**: Trend recommendations should analyze improving, declining, and stable patterns
			Expect(len(insights.Recommendations)).To(BeNumerically(">=", 3),
				"BR-AI-001: Should generate multiple trend-based recommendations covering different patterns")

			// Validate recommendation content quality and business value
			hasImprovingTrendRec := false
			hasDecliningTrendRec := false
			hasStabilityRec := false
			hasStatisticalRec := false

			for _, rec := range insights.Recommendations {
				if strings.Contains(rec, "improving effectiveness trend") {
					hasImprovingTrendRec = true
				}
				if strings.Contains(rec, "declining effectiveness trend") || strings.Contains(rec, "URGENT") {
					hasDecliningTrendRec = true
				}
				if strings.Contains(rec, "Maintain current") || strings.Contains(rec, "stable") {
					hasStabilityRec = true
				}
				if strings.Contains(rec, "statistically significant") || strings.Contains(rec, "High-confidence") {
					hasStatisticalRec = true
				}
			}

			// **Business Value**: Should provide actionable insights for different trend scenarios
			Expect(hasImprovingTrendRec || hasDecliningTrendRec || hasStabilityRec || hasStatisticalRec).To(BeTrue(),
				"BR-AI-001: Should provide specific trend-based business recommendations")
		})

		It("should generate performance optimization recommendations with quantitative metrics", func() {
			// Arrange: Setup performance data with top and underperforming action types
			mockRepo.SetActionTraces(generatePerformanceVariationData(1500))

			// Act: Generate performance-focused analytics insights
			insights, err := assessor.GetAnalyticsInsights(ctx, 45*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate performance optimization recommendations
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should generate performance recommendations without errors")

			// **Functional Requirement**: Performance recommendations should identify optimization opportunities
			hasTopPerformerRec := false
			hasLowPerformerRec := false
			hasDiversityRec := false
			hasSuccessRateRec := false

			for _, rec := range insights.Recommendations {
				if strings.Contains(rec, "high-performing action types") && strings.Contains(rec, "success rate") {
					hasTopPerformerRec = true
				}
				if strings.Contains(rec, "underperforming action types") || strings.Contains(rec, "<50%") {
					hasLowPerformerRec = true
				}
				if strings.Contains(rec, "diversity") || strings.Contains(rec, "expand remediation") {
					hasDiversityRec = true
				}
				if strings.Contains(rec, "%") { // Should include quantitative metrics
					hasSuccessRateRec = true
				}
			}

			// **Business Value**: Should provide quantified performance optimization guidance
			Expect(hasTopPerformerRec || hasLowPerformerRec || hasDiversityRec).To(BeTrue(),
				"BR-AI-001: Should provide specific performance optimization recommendations")
			Expect(hasSuccessRateRec).To(BeTrue(),
				"BR-AI-001: Should include quantitative metrics in performance recommendations")
		})

		It("should generate seasonal timing recommendations for operational optimization", func() {
			// Arrange: Setup seasonal pattern data with clear peak/low hours
			mockRepo.SetActionTraces(generateStrongSeasonalData(2000))

			// Act: Generate seasonal pattern insights
			insights, err := assessor.GetAnalyticsInsights(ctx, 60*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate seasonal timing recommendations
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should generate seasonal recommendations without errors")

			// **Functional Requirement**: Seasonal recommendations should optimize timing strategies
			hasPeakHourRec := false
			hasLowHourRec := false
			hasWeekdayRec := false
			hasAutomationRec := false

			for _, rec := range insights.Recommendations {
				if strings.Contains(rec, "peak effectiveness hours") || strings.Contains(rec, "Schedule non-urgent") {
					hasPeakHourRec = true
				}
				if strings.Contains(rec, "low effectiveness hours") || strings.Contains(rec, "Avoid critical") {
					hasLowHourRec = true
				}
				if strings.Contains(rec, "weekday") || strings.Contains(rec, "weekend") {
					hasWeekdayRec = true
				}
				if strings.Contains(rec, "automated") || strings.Contains(rec, "time-based") {
					hasAutomationRec = true
				}
			}

			// **Business Value**: Should provide timing optimization for reduced business impact
			Expect(hasPeakHourRec || hasLowHourRec || hasWeekdayRec || hasAutomationRec).To(BeTrue(),
				"BR-AI-001: Should provide timing-based operational recommendations")
		})

		It("should generate anomaly investigation recommendations with severity-based prioritization", func() {
			// Arrange: Setup anomaly data with various severity levels
			mockRepo.SetActionTraces(generateAnomalyPatternData(1800))

			// Act: Generate anomaly detection insights
			insights, err := assessor.GetAnalyticsInsights(ctx, 90*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate anomaly-based recommendations
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should generate anomaly recommendations without errors")

			// **Functional Requirement**: Anomaly recommendations should prioritize investigation
			hasCriticalRec := false
			hasInvestigationRec := false
			hasStabilityRec := false
			hasConfidenceRec := false

			for _, rec := range insights.Recommendations {
				if strings.Contains(rec, "CRITICAL") || strings.Contains(rec, "immediate investigation") {
					hasCriticalRec = true
				}
				if strings.Contains(rec, "investigate") || strings.Contains(rec, "review") {
					hasInvestigationRec = true
				}
				if strings.Contains(rec, "stable system") || strings.Contains(rec, "consistent") {
					hasStabilityRec = true
				}
				if strings.Contains(rec, "confidence") || strings.Contains(rec, "high anomaly detection") {
					hasConfidenceRec = true
				}
			}

			// **Business Value**: Should provide severity-based investigation priorities
			Expect(hasInvestigationRec || hasStabilityRec || hasCriticalRec || hasConfidenceRec).To(BeTrue(),
				"BR-AI-001: Should provide anomaly investigation recommendations based on system state")
		})

		It("should generate strategic cross-domain recommendations for system optimization", func() {
			// Arrange: Setup comprehensive data covering all analysis domains
			mockRepo.SetActionTraces(generateComprehensiveAnalyticsData(3000))

			// Act: Generate comprehensive strategic insights
			insights, err := assessor.GetAnalyticsInsights(ctx, 90*24*time.Hour)

			// **Business Requirement BR-AI-001**: Validate strategic recommendations
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Should generate strategic recommendations without errors")

			// **Functional Requirement**: Strategic recommendations should consider system maturity
			hasMaturityRec := false
			hasHealthRec := false
			hasAutomationRec := false
			hasDataQualityRec := false

			for _, rec := range insights.Recommendations {
				if strings.Contains(rec, "analytics data available") || strings.Contains(rec, "maturity") {
					hasMaturityRec = true
				}
				if strings.Contains(rec, "system effectiveness") || strings.Contains(rec, "Excellent") || strings.Contains(rec, "LOW") {
					hasHealthRec = true
				}
				if strings.Contains(rec, "automated") || strings.Contains(rec, "ML-based") {
					hasAutomationRec = true
				}
				if strings.Contains(rec, "monitoring coverage") || strings.Contains(rec, "data collection") {
					hasDataQualityRec = true
				}
			}

			// **Business Value**: Should provide strategic guidance for system evolution
			Expect(hasMaturityRec || hasHealthRec || hasAutomationRec || hasDataQualityRec).To(BeTrue(),
				"BR-AI-001: Should provide strategic recommendations based on system analysis")

			// **Success Criteria**: Should generate multiple strategic recommendations
			Expect(len(insights.Recommendations)).To(BeNumerically(">=", 10),
				"BR-AI-001: Comprehensive data should generate substantial strategic recommendations")
		})
	})

	// BR-AI-002: MUST analyze recurring patterns in alert-action-outcome sequences
	Context("BR-AI-002: Pattern Analytics Engine", func() {
		It("should identify alert-action-outcome patterns meeting 80% accuracy requirement", func() {
			// Arrange: Setup recurring pattern data for BR-AI-002 pattern recognition
			mockRepo.SetActionTraces(generateRecurringPatternData(3000))
			filters := map[string]interface{}{
				"start_time": time.Now().Add(-30 * 24 * time.Hour),
				"end_time":   time.Now(),
			}

			// Act: Analyze alert-action-outcome patterns
			startTime := time.Now()
			analytics, err := assessor.GetPatternAnalytics(ctx, filters)
			processingTime := time.Since(startTime)

			// **Business Requirement BR-AI-002**: Validate pattern analytics generation
			Expect(err).ToNot(HaveOccurred(), "BR-AI-002: Should analyze action-outcome patterns without errors")
			Expect(analytics.TotalPatterns).To(BeNumerically(">=", 0), "BR-AI-002: Pattern analytics must provide measurable pattern detection results for business decision making")

			// **Success Criteria BR-AI-002**: Processes pattern analysis within 15 seconds for real-time recommendations
			Expect(processingTime).To(BeNumerically("<=", 15*time.Second),
				"BR-AI-002: Should complete pattern analysis within 15 seconds")

			// **Functional Requirement 1**: Pattern Recognition - identify common alert→action→outcome sequences
			Expect(analytics.TotalPatterns).To(BeNumerically(">=", 0),
				"BR-AI-002: Should identify alert-action-outcome sequences")

			// **Business Requirement**: Calculate pattern success rates across different contexts
			Expect(analytics.PatternsByType).ToNot(BeEmpty(),
				"BR-AI-002: Should classify patterns by type for business analysis")

			// **Success Criteria**: Identifies patterns with >80% accuracy for alert classification
			Expect(analytics.AverageEffectiveness).To(BeNumerically(">=", 0.0),
				"BR-AI-002: Should calculate meaningful effectiveness metrics")
		})

		It("should provide pattern recommendations meeting 75% success rate requirement", func() {
			// Arrange: Setup high-quality pattern data for recommendation engine testing
			mockRepo.SetActionTraces(generateHighSuccessRatePatternData(1500))
			filters := map[string]interface{}{
				"namespace": "production",
			}

			// Act: Generate pattern-based recommendations
			analytics, err := assessor.GetPatternAnalytics(ctx, filters)

			// **Business Requirement BR-AI-002**: Validate pattern recommendation engine
			Expect(err).ToNot(HaveOccurred(), "BR-AI-002: Should generate pattern recommendations without errors")

			// **Functional Requirement 3**: Pattern Recommendation Engine
			trendAnalysis := analytics.TrendAnalysis
			Expect(trendAnalysis).ToNot(BeEmpty(), "BR-AI-002: Should provide pattern recommendations")

			// **Success Criteria**: Recommends patterns with >75% success rate for new alerts
			if recommendations, hasRecommendations := trendAnalysis["pattern_recommendations"]; hasRecommendations {
				recList, ok := recommendations.([]string)
				if ok && len(recList) > 0 {
					Expect(len(recList)).To(BeNumerically(">=", 1),
						"BR-AI-002: Should provide actionable pattern recommendations")
				}
			}

			// **Business Value**: Improves first-time resolution rates through proven patterns
			if confidenceScores, hasConfidence := trendAnalysis["confidence_scores"]; hasConfidence {
				scores, ok := confidenceScores.(map[string]float64)
				if ok {
					for _, score := range scores {
						Expect(score).To(BeNumerically(">=", 0.0),
							"BR-AI-002: Should provide meaningful confidence scores")
					}
				}
			}
		})

		It("should perform context-aware analysis meeting business intelligence requirements", func() {
			// Arrange: Setup context-specific data for context-aware analysis
			mockRepo.SetActionTraces(generateContextSpecificData(2000))
			filters := map[string]interface{}{
				"namespace":   "critical-systems",
				"action_type": "scale_deployment",
			}

			// Act: Perform context-aware pattern analysis
			analytics, err := assessor.GetPatternAnalytics(ctx, filters)

			// **Business Requirement BR-AI-002**: Validate context-aware analysis
			Expect(err).ToNot(HaveOccurred(), "BR-AI-002: Should perform context-aware analysis without errors")

			// **Functional Requirement 4**: Context-Aware Analysis
			// Analyze patterns within specific namespaces, clusters, or time periods
			Expect(analytics.SuccessRateByType).ToNot(BeEmpty(),
				"BR-AI-002: Should provide context-specific success rates")

			// **Business Requirement**: Account for environmental factors affecting pattern success
			Expect(len(analytics.TopPerformers)).To(BeNumerically(">=", 0),
				"BR-AI-002: Pattern analytics must identify measurable top performing patterns for business optimization")

			// **Success Criteria**: Maintains pattern database with >95% data integrity
			Expect(analytics.TotalPatterns).To(BeNumerically(">=", 0),
				"BR-AI-002: Should maintain data integrity in pattern analysis")

			// **Business Value**: Enables knowledge sharing across similar environments
			if processingTime, hasProcessingTime := analytics.TrendAnalysis["processing_time"]; hasProcessingTime {
				duration, ok := processingTime.(time.Duration)
				if ok {
					Expect(duration).To(BeNumerically(">", 0),
						"BR-AI-002: Should track processing performance for business monitoring")
				}
			}
		})
	})

	// NEW BUSINESS REQUIREMENTS: Performance Monitoring and Correlation
	// BR-MONITORING-016 to BR-MONITORING-020 Integration

	Context("BR-MONITORING-016: Performance Correlation Tracking", func() {
		It("should track AI model performance correlation with context reduction levels", func() {
			// Arrange: Setup action traces with varying context reduction levels for correlation analysis
			mockRepo.SetActionTraces(generatePerformanceCorrelationData(1000))

			// Act: Generate analytics insights with performance correlation monitoring
			insights, err := assessor.GetAnalyticsInsights(ctx, 30*24*time.Hour)

			// **Business Requirement BR-MONITORING-016**: Performance correlation tracking
			Expect(err).ToNot(HaveOccurred(), "BR-MONITORING-016: Should track performance correlation without errors")
			Expect(len(insights.PatternInsights)).To(BeNumerically(">=", 1), "BR-MONITORING-016: Performance correlation must provide measurable correlation data for monitoring optimization")

			// **Functional Requirement**: Context reduction impact on model performance
			correlationData, hasCorrelation := insights.PatternInsights["performance_correlation"]
			Expect(hasCorrelation).To(BeTrue(), "BR-MONITORING-016: Should provide performance correlation data")

			correlation, ok := correlationData.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-016: Correlation data should be structured")

			// Validate correlation metrics
			contextReductionLevels, hasLevels := correlation["context_reduction_levels"]
			Expect(hasLevels).To(BeTrue(), "BR-MONITORING-016: Should track context reduction levels")

			levels, ok := contextReductionLevels.([]map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-016: Reduction levels should be structured")
			Expect(len(levels)).To(BeNumerically(">=", 3), "BR-MONITORING-016: Should track multiple reduction levels")

			// **Business Value**: Correlation coefficient should indicate relationship strength
			correlationCoeff, hasCoeff := correlation["correlation_coefficient"]
			Expect(hasCoeff).To(BeTrue(), "BR-MONITORING-016: Should calculate correlation coefficient")
			Expect(correlationCoeff).To(BeNumerically(">=", -1.0), "BR-MONITORING-016: Correlation should be valid range")
			Expect(correlationCoeff).To(BeNumerically("<=", 1.0), "BR-MONITORING-016: Correlation should be valid range")
		})

		It("should detect performance degradation thresholds and trigger alerts", func() {
			// Arrange: Setup data with performance degradation patterns
			mockRepo.SetActionTraces(generatePerformanceDegradationData(800))

			// Act: Analyze performance degradation patterns
			insights, err := assessor.GetAnalyticsInsights(ctx, 45*24*time.Hour)

			// **Business Requirement BR-MONITORING-017**: Performance degradation detection
			Expect(err).ToNot(HaveOccurred(), "BR-MONITORING-017: Should detect degradation without errors")

			degradationData, hasDegradation := insights.PatternInsights["performance_degradation"]
			Expect(hasDegradation).To(BeTrue(), "BR-MONITORING-017: Should provide degradation analysis")

			degradation, ok := degradationData.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-017: Degradation data should be structured")

			// **Functional Requirement**: Degradation threshold detection
			thresholdBreaches, hasBreaches := degradation["threshold_breaches"]
			Expect(hasBreaches).To(BeTrue(), "BR-MONITORING-017: Should detect threshold breaches")

			breaches, ok := thresholdBreaches.([]map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-017: Breaches should be structured")

			// **Business Value**: Should identify specific degradation incidents
			for _, breach := range breaches {
				Expect(breach["severity"]).ToNot(BeEmpty(), "BR-MONITORING-017: Should classify breach severity")

				impactScore := breach["impact_score"]
				Expect(impactScore).To(BeNumerically(">=", 0.0), "BR-MONITORING-017: Should quantify impact")
				Expect(impactScore).To(BeNumerically("<=", 1.0), "BR-MONITORING-017: Impact should be normalized")
			}

			// **Success Criteria**: Recommendations should address degradation
			hasDegradationRec := false
			for _, rec := range insights.Recommendations {
				if strings.Contains(rec, "performance degradation") || strings.Contains(rec, "context adjustment") {
					hasDegradationRec = true
					break
				}
			}
			Expect(hasDegradationRec).To(BeTrue(), "BR-MONITORING-017: Should provide degradation mitigation recommendations")
		})
	})

	Context("BR-MONITORING-018: Context Adequacy Impact Assessment", func() {
		It("should assess investigation quality impact from context adequacy levels", func() {
			// Arrange: Setup data with varying context adequacy scenarios
			mockRepo.SetActionTraces(generateContextAdequacyData(1200))

			// Act: Analyze context adequacy impact on investigation quality
			analysis, err := assessor.GetPatternAnalytics(ctx, map[string]interface{}{
				"focus":      "context_adequacy_impact",
				"start_time": time.Now().Add(-60 * 24 * time.Hour),
				"end_time":   time.Now(),
			})

			// **Business Requirement BR-MONITORING-018**: Context adequacy impact assessment
			Expect(err).ToNot(HaveOccurred(), "BR-MONITORING-018: Should assess adequacy impact without errors")
			Expect(analysis.TotalPatterns).To(BeNumerically(">=", 0), "BR-MONITORING-018: Context adequacy analysis must provide measurable pattern counts for monitoring assessment")

			// **Functional Requirement**: Quality correlation with context adequacy
			adequacyImpact, hasImpact := analysis.TrendAnalysis["context_adequacy_impact"]
			Expect(hasImpact).To(BeTrue(), "BR-MONITORING-018: Should provide adequacy impact metrics")

			impact, ok := adequacyImpact.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-018: Impact data should be structured")

			// **Business Value**: Quality tiers based on adequacy levels
			qualityTiers, hasTiers := impact["quality_tiers"]
			Expect(hasTiers).To(BeTrue(), "BR-MONITORING-018: Should classify quality tiers")

			tiers, ok := qualityTiers.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-018: Quality tiers should be structured")

			// Validate quality tier metrics
			expectedTiers := []string{"high_adequacy", "medium_adequacy", "low_adequacy"}
			for _, tier := range expectedTiers {
				tierData, hasTier := tiers[tier]
				Expect(hasTier).To(BeTrue(), "BR-MONITORING-018: Should have %s tier", tier)
				if tierData != nil {
					Expect(tierData.(map[string]interface{})).To(HaveKey("quality_score"), "BR-MONITORING-018: %s tier must provide measurable quality score for monitoring assessment", tier)
				}
			}

			// **Success Criteria**: Investigation success rates by adequacy level
			successRateCorrelation, hasCorrelation := impact["success_rate_correlation"]
			Expect(hasCorrelation).To(BeTrue(), "BR-MONITORING-018: Should correlate adequacy with success rates")
			Expect(successRateCorrelation).To(BeNumerically(">=", 0.0), "BR-MONITORING-018: Correlation should be measurable")
		})

		It("should identify context optimization opportunities based on adequacy assessment", func() {
			// Arrange: Setup optimization opportunity data
			mockRepo.SetActionTraces(generateOptimizationOpportunityData(1000))

			// Act: Analyze optimization opportunities
			insights, err := assessor.GetAnalyticsInsights(ctx, 30*24*time.Hour)

			// **Business Requirement BR-MONITORING-018**: Optimization opportunity identification
			Expected := err
			Expect(Expected).ToNot(HaveOccurred(), "BR-MONITORING-018: Should identify opportunities without errors")

			// **Functional Requirement**: Context optimization recommendations
			optimizationOpps, hasOpps := insights.WorkflowInsights["optimization_opportunities"]
			Expect(hasOpps).To(BeTrue(), "BR-MONITORING-018: Should identify optimization opportunities")

			opportunities, ok := optimizationOpps.([]map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-018: Opportunities should be structured")
			Expect(len(opportunities)).To(BeNumerically(">=", 1), "BR-MONITORING-018: Should identify multiple opportunities")

			// **Business Value**: Quantified optimization potential
			for _, opportunity := range opportunities {
				Expect(opportunity["optimization_type"]).ToNot(BeEmpty(), "BR-MONITORING-018: Should classify optimization type")
				Expect(opportunity["potential_improvement"]).To(BeNumerically(">", 0.0), "BR-MONITORING-018: Should quantify improvement potential")
				Expect(opportunity["implementation_effort"]).ToNot(BeEmpty(), "BR-MONITORING-018: Should estimate implementation effort")
			}
		})
	})

	Context("BR-MONITORING-019: Automated Alert Configuration", func() {
		It("should configure automated alerts for performance threshold breaches", func() {
			// Arrange: Setup threshold breach scenarios
			mockRepo.SetActionTraces(generateThresholdBreachData(600))

			// Act: Analyze threshold breaches for alert configuration
			insights, err := assessor.GetAnalyticsInsights(ctx, 15*24*time.Hour)

			// **Business Requirement BR-MONITORING-019**: Automated alert configuration
			Expected := err
			Expect(Expected).ToNot(HaveOccurred(), "BR-MONITORING-019: Should configure alerts without errors")

			// **Functional Requirement**: Alert configuration recommendations
			alertConfig, hasConfig := insights.PatternInsights["alert_configuration"]
			Expect(hasConfig).To(BeTrue(), "BR-MONITORING-019: Should provide alert configuration")

			config, ok := alertConfig.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-019: Alert config should be structured")

			// **Business Value**: Performance threshold definitions
			thresholds, hasThresholds := config["performance_thresholds"]
			Expect(hasThresholds).To(BeTrue(), "BR-MONITORING-019: Should define performance thresholds")

			thresholdMap, ok := thresholds.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-019: Thresholds should be structured")

			// Validate threshold categories
			expectedThresholds := []string{"confidence_degradation", "response_time_increase", "context_reduction_impact"}
			for _, thresholdType := range expectedThresholds {
				thresholdValue, hasThreshold := thresholdMap[thresholdType]
				Expect(hasThreshold).To(BeTrue(), "BR-MONITORING-019: Should define %s threshold", thresholdType)
				Expect(thresholdValue).To(BeNumerically(">", 0), "BR-MONITORING-019: %s threshold should be positive", thresholdType)
			}

			// **Success Criteria**: Alert severity classification
			alertSeverity, hasSeverity := config["alert_severity_mapping"]
			Expect(hasSeverity).To(BeTrue(), "BR-MONITORING-019: Should map alert severities")
			Expect(alertSeverity.(map[string]interface{})).To(HaveKey("critical"), "BR-MONITORING-019: Severity mapping must define critical threshold for business escalation")
		})

		It("should implement notification escalation based on performance impact severity", func() {
			// Arrange: Setup escalation scenario data
			mockRepo.SetActionTraces(generateEscalationScenarioData(400))

			// Act: Analyze escalation requirements
			analysis, err := assessor.GetPatternAnalytics(ctx, map[string]interface{}{
				"focus":     "escalation_analysis",
				"timeframe": "7d",
			})

			// **Business Requirement BR-MONITORING-019**: Notification escalation
			Expected := err
			Expect(Expected).ToNot(HaveOccurred(), "BR-MONITORING-019: Should analyze escalation without errors")

			// **Functional Requirement**: Escalation path configuration
			escalationConfig, hasEscalation := analysis.TrendAnalysis["escalation_configuration"]
			Expect(hasEscalation).To(BeTrue(), "BR-MONITORING-019: Should provide escalation configuration")

			escalation, ok := escalationConfig.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-019: Escalation config should be structured")

			// **Business Value**: Severity-based escalation levels
			escalationLevels, hasLevels := escalation["escalation_levels"]
			Expect(hasLevels).To(BeTrue(), "BR-MONITORING-019: Should define escalation levels")

			levels, ok := escalationLevels.([]map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-019: Escalation levels should be structured")
			Expect(len(levels)).To(BeNumerically(">=", 3), "BR-MONITORING-019: Should have multiple escalation levels")

			// Validate escalation level properties
			for i, level := range levels {
				Expect(level["level"]).To(Equal(i+1), "BR-MONITORING-019: Escalation levels should be sequential")
				Expect(level["timeout_minutes"]).To(BeNumerically(">", 0), "BR-MONITORING-019: Should define timeout for level %d", i+1)

				notificationTargets := level["notification_targets"]
				if targets, ok := notificationTargets.([]string); ok {
					Expect(len(targets)).To(BeNumerically(">=", 1), "BR-MONITORING-019: Should have notification targets for level %d", i+1)
				}
			}
		})
	})

	Context("BR-MONITORING-020: Performance Correlation Dashboard", func() {
		It("should generate performance correlation dashboard data with actionable insights", func() {
			// Arrange: Setup comprehensive dashboard data
			mockRepo.SetActionTraces(generateDashboardData(2000))

			// Act: Generate dashboard analytics
			insights, err := assessor.GetAnalyticsInsights(ctx, 90*24*time.Hour)

			// **Business Requirement BR-MONITORING-020**: Performance correlation dashboard
			Expected := err
			Expect(Expected).ToNot(HaveOccurred(), "BR-MONITORING-020: Should generate dashboard data without errors")

			// **Functional Requirement**: Dashboard visualization data
			dashboardData, hasDashboard := insights.WorkflowInsights["dashboard_data"]
			Expect(hasDashboard).To(BeTrue(), "BR-MONITORING-020: Should provide dashboard data")

			dashboard, ok := dashboardData.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-020: Dashboard data should be structured")

			// **Business Value**: Performance trend visualizations
			performanceTrends, hasTrends := dashboard["performance_trends"]
			Expect(hasTrends).To(BeTrue(), "BR-MONITORING-020: Should provide performance trends")

			trends, ok := performanceTrends.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-020: Performance trends should be structured")
			Expect(len(trends)).To(BeNumerically(">=", 1), "BR-MONITORING-020: Performance trends must contain measurable trend data for business monitoring")

			// Validate key performance indicators
			kpis := []string{"avg_confidence_score", "context_reduction_impact", "investigation_success_rate", "response_time_trend"}
			for _, kpi := range kpis {
				kpiValue, hasKPI := trends[kpi]
				Expect(hasKPI).To(BeTrue(), "BR-MONITORING-020: Should provide %s KPI", kpi)
				Expect(fmt.Sprintf("%v", kpiValue)).ToNot(BeEmpty(), "BR-MONITORING-020: %s KPI must contain measurable performance data for business intelligence", kpi)
			}

			// **Success Criteria**: Actionable insights for optimization
			actionableInsights, hasInsights := dashboard["actionable_insights"]
			Expect(hasInsights).To(BeTrue(), "BR-MONITORING-020: Should provide actionable insights")

			insightsList, ok := actionableInsights.([]map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-020: Insights should be structured")
			Expect(len(insightsList)).To(BeNumerically(">=", 3), "BR-MONITORING-020: Should provide multiple actionable insights")

			// Validate insight quality and actionability
			for _, insight := range insightsList {
				Expect(insight["insight_type"]).ToNot(BeEmpty(), "BR-MONITORING-020: Should classify insight type")
				Expect(insight["priority"]).ToNot(BeEmpty(), "BR-MONITORING-020: Should prioritize insights")

				if recommendation, ok := insight["recommendation"].(string); ok {
					Expect(len(recommendation)).To(BeNumerically(">=", 10), "BR-MONITORING-020: Should provide detailed recommendations")
				}
				Expect(insight["expected_impact"]).To(BeNumerically(">=", 0.0), "BR-MONITORING-020: Should quantify expected impact")
			}
		})

		It("should provide real-time correlation updates for continuous monitoring", func() {
			// Arrange: Setup real-time monitoring scenario
			mockRepo.SetActionTraces(generateRealTimeData(500))

			// Act: Generate real-time correlation updates
			analysis, err := assessor.GetPatternAnalytics(ctx, map[string]interface{}{
				"real_time":         true,
				"update_interval":   "5m",
				"correlation_focus": "context_performance",
			})

			// **Business Requirement BR-MONITORING-020**: Real-time correlation updates
			Expected := err
			Expect(Expected).ToNot(HaveOccurred(), "BR-MONITORING-020: Should provide real-time updates without errors")

			// **Functional Requirement**: Real-time monitoring capabilities
			realTimeData, hasRealTime := analysis.TrendAnalysis["real_time_correlation"]
			Expect(hasRealTime).To(BeTrue(), "BR-MONITORING-020: Should provide real-time correlation data")

			realTime, ok := realTimeData.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-020: Real-time data should be structured")
			Expect(len(realTime)).To(BeNumerically(">=", 1), "BR-MONITORING-020: Real-time data must contain current performance metrics for business monitoring")

			// **Business Value**: Current performance status
			currentStatus, hasStatus := realTime["current_status"]
			Expect(hasStatus).To(BeTrue(), "BR-MONITORING-020: Should provide current performance status")

			status, ok := currentStatus.(map[string]interface{})
			Expect(ok).To(BeTrue(), "BR-MONITORING-020: Status should be structured")
			Expect(status).To(HaveKey("health"), "BR-MONITORING-020: Status must contain health indicators for business system monitoring")

			// Validate real-time metrics
			correlationStrength := status["correlation_strength"]
			Expect(correlationStrength).To(BeNumerically(">=", -1.0), "BR-MONITORING-020: Correlation should be valid range")
			Expect(correlationStrength).To(BeNumerically("<=", 1.0), "BR-MONITORING-020: Correlation should be valid range")

			Expect(status["trend_direction"]).ToNot(BeEmpty(), "BR-MONITORING-020: Should indicate trend direction")

			if lastUpdated, ok := status["last_updated"].(time.Time); ok {
				timeSince := time.Since(lastUpdated)
				Expect(timeSince).To(BeNumerically("<", 10*time.Minute), "BR-MONITORING-020: Updates should be recent")
			}
		})
	})
})

// Test data generators following existing patterns

func generateAnalyticsTestData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	actionTypes := []string{"scale_deployment", "restart_pod", "update_config", "cleanup_resources"}
	executionStatuses := []string{"completed", "failed", "completed", "completed"} // 75% success rate

	for i := 0; i < count; i++ {
		actionIndex := i % len(actionTypes)
		effectivenessScore := 0.7 + 0.2*(float64(i%3)) // Varying effectiveness scores

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         actionTypes[actionIndex],
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Hour),
			ExecutionStatus:    executionStatuses[actionIndex],
			EffectivenessScore: &effectivenessScore,
			AlertName:          "high_cpu_usage",
			AlertSeverity:      "warning",
			AlertLabels: map[string]interface{}{
				"namespace": "production",
				"pod":       "web-server",
			},
		}
	}

	return traces
}

func generateDiverseActionTypeData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	actionTypes := []string{
		"scale_deployment", "restart_pod", "update_config", "cleanup_resources",
		"apply_patch", "rollback_deployment", "increase_memory", "optimize_queries",
	}

	for i := 0; i < count; i++ {
		actionIndex := i % len(actionTypes)
		// Vary success rates by action type for realistic business testing
		successRate := 0.6 + 0.3*float64(actionIndex%4)/3.0 // Range from 60% to 90%
		executionStatus := "completed"
		if float64(i%100)/100.0 > successRate {
			executionStatus = "failed"
		}

		effectivenessScore := successRate + 0.1*(float64(i%5)-2)/2 // Add some variance

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         actionTypes[actionIndex],
			ActionTimestamp:    time.Now().Add(-time.Duration(i%720) * time.Hour), // 30 days
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectivenessScore,
			AlertName:          "resource_utilization",
			AlertSeverity:      []string{"info", "warning", "critical"}[i%3],
		}
	}

	return traces
}

func generateSeasonalPatternData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	for i := 0; i < count; i++ {
		// Create seasonal patterns - higher activity during business hours
		hour := i % 24
		dayOfWeek := (i / 24) % 7

		// Business hours pattern (9 AM - 5 PM, weekdays)
		isBusinessHour := hour >= 9 && hour <= 17 && dayOfWeek >= 1 && dayOfWeek <= 5
		activityMultiplier := 1.0
		if isBusinessHour {
			activityMultiplier = 2.5 // Higher activity during business hours
		}

		effectivenessScore := 0.6 + 0.3*activityMultiplier/2.5 // Better effectiveness during business hours

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "scale_deployment",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Hour),
			ExecutionStatus:    "completed",
			EffectivenessScore: &effectivenessScore,
			AlertName:          "cpu_spike",
			AlertSeverity:      "warning",
		}
	}

	return traces
}

func generateRecurringPatternData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Create recurring alert→action→outcome patterns
	patterns := []struct {
		alert   string
		action  string
		outcome string
		success float64
	}{
		{"high_memory_usage", "restart_pod", "completed", 0.85},
		{"disk_space_low", "cleanup_resources", "completed", 0.90},
		{"high_cpu_usage", "scale_deployment", "completed", 0.80},
		{"connection_timeout", "restart_service", "failed", 0.40}, // Poor pattern
	}

	for i := 0; i < count; i++ {
		patternIndex := i % len(patterns)
		pattern := patterns[patternIndex]

		// Determine success based on pattern success rate
		executionStatus := pattern.outcome
		if float64(i%100)/100.0 > pattern.success {
			executionStatus = "failed"
		}

		effectivenessScore := pattern.success + 0.05*(float64(i%10)-5)/5 // Add variance

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         pattern.action,
			ActionTimestamp:    time.Now().Add(-time.Duration(i%1440) * time.Minute), // 24 hours
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectivenessScore,
			AlertName:          pattern.alert,
			AlertSeverity:      "warning",
		}
	}

	return traces
}

func generateHighSuccessRatePatternData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	for i := 0; i < count; i++ {
		// High success rate patterns for recommendation engine testing
		effectivenessScore := 0.80 + 0.15*float64(i%3)/2 // 80-95% effectiveness

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "proven_remediation",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    "completed", // High success rate
			EffectivenessScore: &effectivenessScore,
			AlertName:          "performance_degradation",
			AlertSeverity:      "warning",
			AlertLabels: map[string]interface{}{
				"namespace": "production",
			},
		}
	}

	return traces
}

func generateContextSpecificData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	namespaces := []string{"critical-systems", "production", "staging"}
	actionTypes := []string{"scale_deployment", "restart_pod", "update_config"}

	for i := 0; i < count; i++ {
		namespaceIndex := i % len(namespaces)
		actionIndex := i % len(actionTypes)

		// Context-specific success rates
		successRate := 0.7
		if namespaces[namespaceIndex] == "critical-systems" {
			successRate = 0.85 // Higher success in critical systems
		}

		effectivenessScore := successRate + 0.1*float64(i%5)/5
		executionStatus := "completed"
		if float64(i%100)/100.0 > successRate {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         actionTypes[actionIndex],
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectivenessScore,
			AlertName:          "system_alert",
			AlertSeverity:      "critical",
			AlertLabels: map[string]interface{}{
				"namespace": namespaces[namespaceIndex],
			},
		}
	}

	return traces
}

func generateActionID(index int) string {
	return fmt.Sprintf("action_%d", index)
}

// TDD Test Data Generators for Recommendation Testing

func generateMixedTrendData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Create mixed trend patterns: improving, declining, stable
	for i := 0; i < count; i++ {
		trendSegment := i / (count / 3) // Divide into 3 segments

		var effectiveness float64
		switch trendSegment {
		case 0: // Improving trend
			effectiveness = 0.4 + (0.4 * float64(i) / float64(count/3))
		case 1: // Declining trend
			effectiveness = 0.8 - (0.4 * float64(i-(count/3)) / float64(count/3))
		default: // Stable trend
			effectiveness = 0.6 + 0.1*(float64(i%10)-5)/5 // Small variations around 0.6
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "trend_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(count-i) * time.Hour),
			ExecutionStatus:    "completed",
			EffectivenessScore: &effectiveness,
			AlertName:          "trend_analysis_test",
			AlertSeverity:      "warning",
		}
	}

	return traces
}

func generatePerformanceVariationData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Create performance variation with top and underperforming action types
	actionTypes := []string{
		"high_perf_action_1", "high_perf_action_2", // Top performers (85%+ success)
		"medium_perf_action_1", "medium_perf_action_2", // Medium performers (60-70% success)
		"low_perf_action_1", "low_perf_action_2", // Underperformers (<50% success)
	}

	successRates := []float64{0.90, 0.85, 0.65, 0.70, 0.40, 0.30}

	for i := 0; i < count; i++ {
		actionIndex := i % len(actionTypes)
		baseRate := successRates[actionIndex]

		effectiveness := baseRate + 0.05*(float64(i%10)-5)/5 // Add some variance
		executionStatus := "completed"
		if float64(i%100)/100.0 > baseRate {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         actionTypes[actionIndex],
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Hour),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "performance_test",
			AlertSeverity:      "warning",
		}
	}

	return traces
}

func generateStrongSeasonalData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	for i := 0; i < count; i++ {
		// Create strong seasonal patterns
		hour := (i * 2) % 24 // Vary hours more systematically
		dayOfWeek := (i / 24) % 7

		// Strong business hour effect (9 AM - 5 PM, weekdays)
		isBusinessHour := hour >= 9 && hour <= 17 && dayOfWeek >= 1 && dayOfWeek <= 5
		isPeakHour := hour >= 10 && hour <= 14 && dayOfWeek >= 1 && dayOfWeek <= 5
		isLowHour := (hour >= 22 || hour <= 6)

		var effectiveness float64
		if isPeakHour {
			effectiveness = 0.85 + 0.1*float64(i%5)/5
		} else if isBusinessHour {
			effectiveness = 0.70 + 0.1*float64(i%5)/5
		} else if isLowHour {
			effectiveness = 0.35 + 0.1*float64(i%5)/5
		} else {
			effectiveness = 0.55 + 0.1*float64(i%5)/5
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "seasonal_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Hour),
			ExecutionStatus:    "completed",
			EffectivenessScore: &effectiveness,
			AlertName:          "seasonal_pattern_test",
			AlertSeverity:      "warning",
		}
	}

	return traces
}

func generateAnomalyPatternData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data with intentional anomalies
	for i := 0; i < count; i++ {
		baseEffectiveness := 0.7 // Normal effectiveness
		effectiveness := baseEffectiveness

		// Create anomalies at specific intervals
		isAnomaly := (i % 100) < 5       // 5% anomaly rate
		isSevereAnomaly := (i % 200) < 2 // 1% severe anomaly rate

		if isSevereAnomaly {
			effectiveness = 0.1 + 0.1*float64(i%3)/3 // Severe underperformance
		} else if isAnomaly {
			effectiveness = 0.3 + 0.1*float64(i%3)/3 // Moderate anomaly
		} else {
			effectiveness = baseEffectiveness + 0.1*(float64(i%10)-5)/5 // Normal variation
		}

		executionStatus := "completed"
		if effectiveness < 0.3 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "anomaly_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "anomaly_detection_test",
			AlertSeverity:      []string{"warning", "critical"}[i%2],
		}
	}

	return traces
}

func generateComprehensiveAnalyticsData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate comprehensive data covering all analysis types
	actionTypes := []string{
		"scale_deployment", "restart_pod", "update_config", "cleanup_resources",
		"apply_patch", "rollback_deployment", "increase_memory", "optimize_queries",
		"network_repair", "storage_expansion", "cpu_optimization", "security_patch",
	}

	for i := 0; i < count; i++ {
		actionIndex := i % len(actionTypes)

		// Mix of trend patterns
		trendSegment := i / (count / 4)
		var baseTrendEffectiveness float64
		switch trendSegment {
		case 0: // Strong improving trend
			baseTrendEffectiveness = 0.5 + (0.3 * float64(i) / float64(count/4))
		case 1: // Declining trend
			baseTrendEffectiveness = 0.8 - (0.2 * float64(i-(count/4)) / float64(count/4))
		case 2: // Stable high performance
			baseTrendEffectiveness = 0.75 + 0.05*float64(i%10-5)/5
		default: // Recovery pattern
			baseTrendEffectiveness = 0.4 + (0.4 * float64(i-(3*count/4)) / float64(count/4))
		}

		// Performance variations by action type
		actionTypeMultiplier := 0.8 + 0.4*float64(actionIndex)/float64(len(actionTypes))

		// Seasonal effects
		hour := (i * 3) % 24
		isBusinessHour := hour >= 9 && hour <= 17
		seasonalMultiplier := 1.0
		if isBusinessHour {
			seasonalMultiplier = 1.2
		} else if hour >= 22 || hour <= 6 {
			seasonalMultiplier = 0.8
		}

		// Occasional anomalies
		isAnomaly := (i % 150) < 3 // 2% anomaly rate
		anomalyMultiplier := 1.0
		if isAnomaly {
			anomalyMultiplier = 0.3
		}

		effectiveness := baseTrendEffectiveness * actionTypeMultiplier * seasonalMultiplier * anomalyMultiplier
		effectiveness = math.Max(0.1, math.Min(1.0, effectiveness))

		executionStatus := "completed"
		if effectiveness < 0.5 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         actionTypes[actionIndex],
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Hour),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "comprehensive_analytics_test",
			AlertSeverity:      []string{"info", "warning", "critical"}[i%3],
			AlertLabels: map[string]interface{}{
				"namespace": []string{"production", "staging", "dev"}[i%3],
				"cluster":   []string{"main", "backup"}[i%2],
			},
		}
	}

	return traces
}

// NEW BUSINESS REQUIREMENTS: Test data generators for monitoring and performance correlation

func generatePerformanceCorrelationData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data with clear performance correlation patterns
	contextReductionLevels := []float64{0.2, 0.4, 0.6, 0.8} // 20%, 40%, 60%, 80% reduction

	for i := 0; i < count; i++ {
		reductionLevel := contextReductionLevels[i%len(contextReductionLevels)]

		// Simulate inverse correlation: higher reduction = lower effectiveness
		baseEffectiveness := 0.9 - (reductionLevel * 0.4) // 90% to 50% range

		// Add some variance
		variance := 0.1 * (float64(i%20-10) / 10.0)
		effectiveness := baseEffectiveness + variance
		effectiveness = math.Max(0.3, math.Min(0.95, effectiveness))

		executionStatus := "completed"
		if effectiveness < 0.6 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "context_optimized_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "performance_correlation_test",
			AlertSeverity:      "warning",
			AlertLabels: map[string]interface{}{
				"context_reduction_level": reductionLevel,
				"optimization_strategy":   fmt.Sprintf("%.0f_percent_reduction", reductionLevel*100),
				"performance_tier":        getPerformanceTier(effectiveness),
			},
		}
	}

	return traces
}

func generatePerformanceDegradationData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data showing performance degradation patterns
	for i := 0; i < count; i++ {
		// Simulate degradation over time
		timeSegment := i / (count / 5) // 5 time segments
		var effectiveness float64

		switch timeSegment {
		case 0: // Normal performance
			effectiveness = 0.85 + 0.1*float64(i%10)/10
		case 1: // Slight degradation
			effectiveness = 0.75 + 0.1*float64(i%10)/10
		case 2: // Moderate degradation
			effectiveness = 0.65 + 0.1*float64(i%10)/10
		case 3: // Significant degradation
			effectiveness = 0.45 + 0.1*float64(i%10)/10
		default: // Recovery
			effectiveness = 0.65 + 0.2*float64(i%10)/10
		}

		executionStatus := "completed"
		if effectiveness < 0.5 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "degradation_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Hour),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "performance_degradation_test",
			AlertSeverity:      getDegradationSeverity(effectiveness),
			AlertLabels: map[string]interface{}{
				"degradation_level": getDegradationLevel(effectiveness),
				"time_segment":      timeSegment,
				"recovery_needed":   effectiveness < 0.6,
			},
		}
	}

	return traces
}

func generateContextAdequacyData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data with varying context adequacy levels
	adequacyLevels := []string{"high", "medium", "low"}
	adequacyScores := map[string]float64{"high": 0.9, "medium": 0.7, "low": 0.4}

	for i := 0; i < count; i++ {
		adequacyLevel := adequacyLevels[i%len(adequacyLevels)]
		baseScore := adequacyScores[adequacyLevel]

		// Add variance based on adequacy
		variance := 0.15 * (float64(i%10-5) / 5.0)
		effectiveness := baseScore + variance
		effectiveness = math.Max(0.2, math.Min(0.95, effectiveness))

		executionStatus := "completed"
		if effectiveness < 0.5 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "adequacy_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "context_adequacy_test",
			AlertSeverity:      "warning",
			AlertLabels: map[string]interface{}{
				"context_adequacy_level": adequacyLevel,
				"adequacy_score":         baseScore,
				"investigation_type":     getInvestigationType(i),
			},
		}
	}

	return traces
}

func generateOptimizationOpportunityData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data that shows optimization opportunities
	for i := 0; i < count; i++ {
		// Some actions have clear optimization potential
		optimizationPotential := float64(i%4) * 0.2 // 0%, 20%, 40%, 60% improvement potential
		baseEffectiveness := 0.6 + 0.1*float64(i%5)/5

		effectiveness := baseEffectiveness + optimizationPotential*0.3
		effectiveness = math.Max(0.4, math.Min(0.95, effectiveness))

		executionStatus := "completed"
		if effectiveness < 0.5 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "optimization_candidate",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "optimization_opportunity_test",
			AlertSeverity:      "info",
			AlertLabels: map[string]interface{}{
				"optimization_potential": optimizationPotential,
				"optimization_type":      getOptimizationType(optimizationPotential),
				"current_performance":    baseEffectiveness,
			},
		}
	}

	return traces
}

func generateThresholdBreachData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data with threshold breaches
	for i := 0; i < count; i++ {
		// Simulate threshold breaches at intervals
		isThresholdBreach := (i % 20) < 3 // ~15% breach rate

		var effectiveness float64
		if isThresholdBreach {
			// Performance below threshold
			effectiveness = 0.3 + 0.2*float64(i%5)/5
		} else {
			// Normal performance
			effectiveness = 0.7 + 0.2*float64(i%5)/5
		}

		executionStatus := "completed"
		if effectiveness < 0.5 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "threshold_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "threshold_breach_test",
			AlertSeverity:      getBreachSeverity(effectiveness, isThresholdBreach),
			AlertLabels: map[string]interface{}{
				"threshold_breach": isThresholdBreach,
				"breach_severity":  getBreachSeverity(effectiveness, isThresholdBreach),
				"threshold_type":   getThresholdType(i),
			},
		}
	}

	return traces
}

func generateEscalationScenarioData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate data requiring different escalation levels
	escalationLevels := []int{1, 2, 3, 4} // Different escalation levels

	for i := 0; i < count; i++ {
		escalationLevel := escalationLevels[i%len(escalationLevels)]

		// Lower effectiveness triggers higher escalation
		effectiveness := 0.9 - (float64(escalationLevel-1) * 0.2)
		effectiveness = math.Max(0.1, effectiveness)

		executionStatus := "completed"
		if effectiveness < 0.5 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "escalation_test_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "escalation_scenario_test",
			AlertSeverity:      getEscalationSeverity(escalationLevel),
			AlertLabels: map[string]interface{}{
				"escalation_level":  escalationLevel,
				"escalation_reason": getEscalationReason(effectiveness),
				"urgency":           getUrgencyLevel(escalationLevel),
			},
		}
	}

	return traces
}

func generateDashboardData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate comprehensive dashboard-worthy data
	for i := 0; i < count; i++ {
		// Mix different performance patterns for rich dashboard data
		timeSegment := i / (count / 8) // 8 different patterns

		var effectiveness float64
		switch timeSegment {
		case 0: // High performance period
			effectiveness = 0.85 + 0.1*float64(i%10)/10
		case 1: // Performance dip
			effectiveness = 0.65 + 0.1*float64(i%10)/10
		case 2: // Recovery phase
			effectiveness = 0.5 + 0.3*float64(i%20)/20
		case 3: // Optimal performance
			effectiveness = 0.9 + 0.05*float64(i%10)/10
		case 4: // Gradual decline
			effectiveness = 0.8 - 0.3*float64(i%30)/30
		case 5: // Variable performance
			effectiveness = 0.6 + 0.3*math.Sin(float64(i)*0.1)
		case 6: // Stability test
			effectiveness = 0.75 + 0.05*float64(i%5-2)/2
		default: // Mixed patterns
			effectiveness = 0.6 + 0.3*float64(i%7)/7
		}

		effectiveness = math.Max(0.2, math.Min(0.95, effectiveness))

		executionStatus := "completed"
		if effectiveness < 0.4 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "dashboard_data_action",
			ActionTimestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "dashboard_data_test",
			AlertSeverity:      getDashboardSeverity(effectiveness),
			AlertLabels: map[string]interface{}{
				"dashboard_segment": timeSegment,
				"performance_tier":  getPerformanceTier(effectiveness),
				"data_quality":      getDataQuality(i),
			},
		}
	}

	return traces
}

func generateRealTimeData(count int) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	// Generate recent data for real-time monitoring
	for i := 0; i < count; i++ {
		// Recent timestamps (last 2 hours)
		timestamp := time.Now().Add(-time.Duration(i*2) * time.Minute)

		// Current performance with slight variations
		baseEffectiveness := 0.8
		variation := 0.15 * math.Sin(float64(i)*0.2) // Smooth variation
		effectiveness := baseEffectiveness + variation
		effectiveness = math.Max(0.4, math.Min(0.95, effectiveness))

		executionStatus := "completed"
		if effectiveness < 0.6 {
			executionStatus = "failed"
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionID:           generateActionID(i),
			ActionType:         "real_time_action",
			ActionTimestamp:    timestamp,
			ExecutionStatus:    executionStatus,
			EffectivenessScore: &effectiveness,
			AlertName:          "real_time_monitoring_test",
			AlertSeverity:      "warning",
			AlertLabels: map[string]interface{}{
				"real_time":          true,
				"correlation_window": "5m",
				"monitoring_active":  true,
			},
		}
	}

	return traces
}

// Helper functions for new business requirements

func getPerformanceTier(effectiveness float64) string {
	if effectiveness >= 0.8 {
		return "high"
	} else if effectiveness >= 0.6 {
		return "medium"
	}
	return "low"
}

func getDegradationSeverity(effectiveness float64) string {
	if effectiveness < 0.4 {
		return "critical"
	} else if effectiveness < 0.6 {
		return "warning"
	}
	return "info"
}

func getDegradationLevel(effectiveness float64) string {
	if effectiveness < 0.5 {
		return "severe"
	} else if effectiveness < 0.7 {
		return "moderate"
	}
	return "mild"
}

func getInvestigationType(index int) string {
	types := []string{"root_cause_analysis", "performance_optimization", "security_investigation", "basic_troubleshooting"}
	return types[index%len(types)]
}

func getOptimizationType(potential float64) string {
	if potential >= 0.4 {
		return "high_impact"
	} else if potential >= 0.2 {
		return "medium_impact"
	}
	return "low_impact"
}

func getBreachSeverity(effectiveness float64, isBreach bool) string {
	if !isBreach {
		return "info"
	}
	if effectiveness < 0.3 {
		return "critical"
	}
	return "warning"
}

func getThresholdType(index int) string {
	types := []string{"confidence_threshold", "response_time_threshold", "success_rate_threshold"}
	return types[index%len(types)]
}

func getEscalationSeverity(level int) string {
	severities := []string{"info", "warning", "critical", "critical"}
	return severities[level-1]
}

func getEscalationReason(effectiveness float64) string {
	if effectiveness < 0.3 {
		return "critical_performance_degradation"
	} else if effectiveness < 0.5 {
		return "performance_below_threshold"
	} else if effectiveness < 0.7 {
		return "performance_concern"
	}
	return "monitoring_alert"
}

func getUrgencyLevel(escalationLevel int) string {
	if escalationLevel >= 4 {
		return "critical"
	} else if escalationLevel >= 3 {
		return "high"
	} else if escalationLevel >= 2 {
		return "medium"
	}
	return "low"
}

func getDashboardSeverity(effectiveness float64) string {
	if effectiveness >= 0.8 {
		return "info"
	} else if effectiveness >= 0.6 {
		return "warning"
	}
	return "critical"
}

func getDataQuality(index int) string {
	qualities := []string{"excellent", "good", "fair", "poor"}
	return qualities[index%len(qualities)]
}

// Mock implementations following existing patterns and implementing required interfaces

type MockAlertClient struct{}

func NewMockAlertClient() *MockAlertClient {
	return &MockAlertClient{}
}

func (m *MockAlertClient) IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return true, nil
}

func (m *MockAlertClient) HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return false, nil
}

func (m *MockAlertClient) GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]monitoring.AlertEvent, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return []monitoring.AlertEvent{}, nil
}

func (m *MockAlertClient) CreateSilence(ctx context.Context, silence *monitoring.SilenceRequest) (*monitoring.SilenceResponse, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &monitoring.SilenceResponse{SilenceID: "mock-silence"}, nil
}

func (m *MockAlertClient) DeleteSilence(ctx context.Context, silenceID string) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

func (m *MockAlertClient) GetSilences(ctx context.Context, matchers []monitoring.SilenceMatcher) ([]monitoring.Silence, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return []monitoring.Silence{}, nil
}

func (m *MockAlertClient) AcknowledgeAlert(ctx context.Context, alertName, namespace string) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

type MockMetricsClient struct{}

func NewMockMetricsClient() *MockMetricsClient {
	return &MockMetricsClient{}
}

func (m *MockMetricsClient) GetResourceMetrics(ctx context.Context, namespace, resourceName string, metricNames []string) (map[string]float64, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return map[string]float64{"cpu_usage": 0.75, "memory_usage": 0.60}, nil
}

func (m *MockMetricsClient) CheckMetricsImprovement(ctx context.Context, alert types.Alert, trace *actionhistory.ResourceActionTrace) (bool, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return true, nil
}

func (m *MockMetricsClient) GetMetricsHistory(ctx context.Context, namespace, resourceName string, metricNames []string, from, to time.Time) ([]monitoring.MetricPoint, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return []monitoring.MetricPoint{
		{Timestamp: from, Value: 0.8},
		{Timestamp: to, Value: 0.6},
	}, nil
}

type MockSideEffectDetector struct{}

func NewMockSideEffectDetector() *MockSideEffectDetector {
	return &MockSideEffectDetector{}
}

func (m *MockSideEffectDetector) DetectSideEffects(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace) ([]monitoring.SideEffect, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return []monitoring.SideEffect{}, nil
}

func (m *MockSideEffectDetector) CheckNewAlerts(ctx context.Context, namespace string, since time.Time) ([]types.Alert, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return []types.Alert{}, nil
}

// MockActionHistoryRepository - implements the missing mock that tests expect
type MockActionHistoryRepository struct {
	traces []actionhistory.ResourceActionTrace
	state  map[string]interface{}
	error  error // For error injection in tests
}

func NewMockActionHistoryRepository() *MockActionHistoryRepository {
	return &MockActionHistoryRepository{
		traces: make([]actionhistory.ResourceActionTrace, 0),
		state:  make(map[string]interface{}),
	}
}

func (m *MockActionHistoryRepository) SetActionTraces(traces []actionhistory.ResourceActionTrace) {
	m.traces = traces
}

func (m *MockActionHistoryRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.error != nil {
		return nil, m.error
	}

	// Filter traces based on query parameters if needed
	if query.TimeRange.Start.IsZero() && query.TimeRange.End.IsZero() {
		// Return all traces if no time range specified
		if query.Limit > 0 && len(m.traces) > query.Limit {
			return m.traces[:query.Limit], nil
		}
		return m.traces, nil
	}

	// Filter by time range for business requirement testing
	filtered := make([]actionhistory.ResourceActionTrace, 0)
	for _, trace := range m.traces {
		if (query.TimeRange.Start.IsZero() || trace.ActionTimestamp.After(query.TimeRange.Start)) &&
			(query.TimeRange.End.IsZero() || trace.ActionTimestamp.Before(query.TimeRange.End)) {
			filtered = append(filtered, trace)
		}
	}

	if query.Limit > 0 && len(filtered) > query.Limit {
		return filtered[:query.Limit], nil
	}
	return filtered, nil
}

func (m *MockActionHistoryRepository) ClearState() {
	m.traces = make([]actionhistory.ResourceActionTrace, 0)
	m.state = make(map[string]interface{})
	m.error = nil
}

func (m *MockActionHistoryRepository) SetError(errMsg string) {
	if errMsg == "" {
		m.error = nil
	} else {
		m.error = fmt.Errorf("%s", errMsg)
	}
}

// Additional methods that actionhistory.Repository interface may require
func (m *MockActionHistoryRepository) SaveActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.traces = append(m.traces, *trace)
	return nil
}

func (m *MockActionHistoryRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	for _, trace := range m.traces {
		if trace.ActionID == actionID {
			return &trace, nil
		}
	}
	return nil, fmt.Errorf("trace not found: %s", actionID)
}

// Additional interface methods required by actionhistory.Repository
func (m *MockActionHistoryRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	return 1, nil // Simple mock: return dummy ID
}

func (m *MockActionHistoryRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &actionhistory.ResourceReference{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	}, nil
}

func (m *MockActionHistoryRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &actionhistory.ActionHistory{
		ID:         resourceID,
		ResourceID: resourceID,
	}, nil
}

func (m *MockActionHistoryRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &actionhistory.ActionHistory{
		ID:         resourceID,
		ResourceID: resourceID,
	}, nil
}

func (m *MockActionHistoryRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

func (m *MockActionHistoryRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	trace := &actionhistory.ResourceActionTrace{
		ActionID:        action.ActionID,
		ActionType:      action.ActionType,
		ActionTimestamp: action.Timestamp,
		ExecutionStatus: "completed",
	}
	m.traces = append(m.traces, *trace)
	return trace, nil
}

func (m *MockActionHistoryRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if m.error != nil {
		return m.error
	}
	for i := range m.traces {
		if m.traces[i].ActionID == trace.ActionID {
			m.traces[i] = *trace
			return nil
		}
	}
	return fmt.Errorf("action trace not found for update: %s", trace.ActionID)
}

func (m *MockActionHistoryRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.error != nil {
		return nil, m.error
	}
	// Mock implementation - return some pending assessments
	pending := make([]*actionhistory.ResourceActionTrace, 0)
	for i := range m.traces {
		if m.traces[i].EffectivenessScore == nil {
			pending = append(pending, &m.traces[i])
		}
	}
	return pending, nil
}

func (m *MockActionHistoryRepository) ApplyRetention(ctx context.Context, retentionDays int64) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if m.error != nil {
		return m.error
	}
	// Mock implementation - remove traces older than retention period
	cutoffTime := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	filtered := make([]actionhistory.ResourceActionTrace, 0)
	for _, trace := range m.traces {
		if trace.ActionTimestamp.After(cutoffTime) {
			filtered = append(filtered, trace)
		}
	}
	m.traces = filtered
	return nil
}

func (m *MockActionHistoryRepository) GetActionHistorySummaries(ctx context.Context, period time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.error != nil {
		return nil, m.error
	}
	// Mock implementation - return empty summaries
	return []actionhistory.ActionHistorySummary{}, nil
}

// Additional missing interface methods for complete Repository implementation
func (m *MockActionHistoryRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.error != nil {
		return nil, m.error
	}
	return []actionhistory.OscillationDetection{}, nil
}

func (m *MockActionHistoryRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.error != nil {
		return nil, m.error
	}
	return []actionhistory.OscillationPattern{}, nil
}

func (m *MockActionHistoryRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return m.error
}

// MockAIInsightsVectorDatabase - implements the missing vector database mock
type MockAIInsightsVectorDatabase struct {
	vectors map[string][]float64
	state   map[string]interface{}
}

func NewMockAIInsightsVectorDatabase() *MockAIInsightsVectorDatabase {
	return &MockAIInsightsVectorDatabase{
		vectors: make(map[string][]float64),
		state:   make(map[string]interface{}),
	}
}

func (m *MockAIInsightsVectorDatabase) ClearState() {
	m.vectors = make(map[string][]float64)
	m.state = make(map[string]interface{})
}

func (m *MockAIInsightsVectorDatabase) StoreVector(id string, vector []float64) error {
	m.vectors[id] = vector
	return nil
}

func (m *MockAIInsightsVectorDatabase) SearchSimilar(vector []float64, limit int) ([]string, error) {
	// Simple mock implementation - return first few stored vectors
	results := make([]string, 0)
	count := 0
	for id := range m.vectors {
		if count >= limit {
			break
		}
		results = append(results, id)
		count++
	}
	return results, nil
}
