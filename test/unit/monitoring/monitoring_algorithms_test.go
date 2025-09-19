package monitoring_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
)

// BR-MON-001-015: Monitoring Algorithm Logic Tests (Phase 2 Implementation)
// Following UNIT_TEST_COVERAGE_EXTENSION_PLAN.md - Focus on pure algorithmic logic
var _ = Describe("BR-MON-001-015: Monitoring Algorithm Logic Tests", func() {

	Describe("BR-MON-001: Metric Aggregation Algorithms", func() {
		It("should calculate statistical aggregations correctly", func() {
			values := []float64{10.0, 20.0, 30.0, 40.0, 50.0}

			result := monitoring.AggregateMetrics(values)

			Expect(result.Mean).To(BeNumerically("~", 30.0, 0.01), "Should calculate correct mean")
			Expect(result.Median).To(BeNumerically("~", 30.0, 0.01), "Should calculate correct median")
			Expect(result.Min).To(Equal(10.0), "Should identify minimum value")
			Expect(result.Max).To(Equal(50.0), "Should identify maximum value")
			Expect(result.Count).To(Equal(5), "Should count all values")
			Expect(result.Sum).To(BeNumerically("~", 150.0, 0.01), "Should calculate correct sum")
		})

		It("should handle edge cases in metric aggregation", func() {
			// Test empty dataset
			emptyResult := monitoring.AggregateMetrics([]float64{})
			Expect(emptyResult.Count).To(Equal(0), "Empty dataset should have zero count")
			Expect(emptyResult.Mean).To(Equal(0.0), "Empty dataset should have zero mean")

			// Test single value
			singleValue := []float64{42.0}
			singleResult := monitoring.AggregateMetrics(singleValue)
			Expect(singleResult.Mean).To(Equal(42.0), "Single value mean should equal the value")
			Expect(singleResult.Median).To(Equal(42.0), "Single value median should equal the value")
			Expect(singleResult.StdDev).To(Equal(0.0), "Single value standard deviation should be zero")
		})

		It("should calculate percentiles accurately", func() {
			values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

			result := monitoring.AggregateMetrics(values)

			// P95 of [1,2,3,4,5,6,7,8,9,10] should be around 9.5-10
			Expect(result.P95).To(BeNumerically(">=", 9.0), "P95 should be near 95th percentile")
			Expect(result.P95).To(BeNumerically("<=", 10.0), "P95 should not exceed maximum")

			// P99 should be very close to maximum
			Expect(result.P99).To(BeNumerically(">=", 9.5), "P99 should be near maximum")
			Expect(result.P99).To(BeNumerically("<=", 10.0), "P99 should not exceed maximum")
		})

		It("should calculate standard deviation correctly", func() {
			// Test with known standard deviation
			values := []float64{2, 4, 4, 4, 5, 5, 7, 9} // StdDev ≈ 2.0

			result := monitoring.AggregateMetrics(values)

			Expect(result.StdDev).To(BeNumerically("~", 2.0, 0.1), "Should calculate correct standard deviation")
			Expect(result.Mean).To(BeNumerically("~", 5.0, 0.1), "Should calculate correct mean for StdDev validation")
		})
	})

	Describe("BR-MON-002: Threshold Evaluation Logic", func() {
		It("should evaluate upper threshold violations correctly", func() {
			// Test critical threshold violation
			result := monitoring.EvaluateThreshold(100.0, 70.0, 90.0, "upper")

			Expect(result.IsViolated).To(BeTrue(), "Should detect critical threshold violation")
			Expect(result.ViolationSeverity).To(Equal("critical"), "Should classify as critical violation")
			Expect(result.RecommendedAction).To(Equal("immediate_intervention"), "Should recommend immediate intervention")
			Expect(result.ConfidenceScore).To(BeNumerically(">=", 0.9), "Should have high confidence for critical violations")
		})

		It("should evaluate warning threshold violations correctly", func() {
			// Test warning threshold violation
			result := monitoring.EvaluateThreshold(80.0, 70.0, 90.0, "upper")

			Expect(result.IsViolated).To(BeTrue(), "Should detect warning threshold violation")
			Expect(result.ViolationSeverity).To(Equal("warning"), "Should classify as warning violation")
			Expect(result.RecommendedAction).To(Equal("monitor_closely"), "Should recommend close monitoring")
			Expect(result.ConfidenceScore).To(BeNumerically(">=", 0.8), "Should have good confidence for warning violations")
		})

		It("should handle normal threshold conditions", func() {
			// Test normal conditions
			result := monitoring.EvaluateThreshold(50.0, 70.0, 90.0, "upper")

			Expect(result.IsViolated).To(BeFalse(), "Should not detect violation for normal values")
			Expect(result.RecommendedAction).To(Equal("continue_monitoring"), "Should recommend continued monitoring")
			Expect(result.ViolationLevel).To(BeNumerically("<", 1.0), "Violation level should be less than 1.0 for normal values")
		})

		It("should evaluate lower threshold violations correctly", func() {
			// Test lower threshold critical violation
			result := monitoring.EvaluateThreshold(10.0, 30.0, 20.0, "lower")

			Expect(result.IsViolated).To(BeTrue(), "Should detect critical lower threshold violation")
			Expect(result.ViolationSeverity).To(Equal("critical"), "Should classify as critical violation")
			Expect(result.ThresholdType).To(Equal("lower"), "Should preserve threshold type")
		})

		It("should calculate violation levels accurately", func() {
			// Test violation level calculation
			result := monitoring.EvaluateThreshold(120.0, 70.0, 100.0, "upper")

			Expect(result.ViolationLevel).To(BeNumerically(">", 0), "Should calculate positive violation level")
			Expect(result.ViolationLevel).To(BeNumerically("~", 0.2, 0.01), "Should calculate correct violation percentage")
		})
	})

	Describe("BR-MON-003: Performance Calculation Algorithms", func() {
		It("should calculate performance scores with equal weights", func() {
			metrics := map[string]float64{
				"cpu_usage":    80.0,
				"memory_usage": 60.0,
				"disk_usage":   70.0,
			}
			weights := map[string]float64{
				"cpu_usage":    1.0,
				"memory_usage": 1.0,
				"disk_usage":   1.0,
			}
			benchmarks := map[string]float64{
				"cpu_usage":    100.0,
				"memory_usage": 100.0,
				"disk_usage":   100.0,
			}

			result := monitoring.CalculatePerformanceScore(metrics, weights, benchmarks)

			Expect(result.OverallScore).To(BeNumerically("~", 70.0, 1.0), "Should calculate correct overall score")
			Expect(result.PerformanceGrade).To(Equal("satisfactory"), "Should assign satisfactory grade for 70% score")
			Expect(result.ComponentScores).To(HaveLen(3), "Should calculate scores for all components")
			Expect(result.ComponentScores["cpu_usage"]).To(BeNumerically("~", 80.0, 0.1), "Should calculate correct CPU score")
		})

		It("should apply weights correctly in score calculation", func() {
			metrics := map[string]float64{
				"critical_metric":    50.0,
				"secondary_metric":   90.0,
			}
			weights := map[string]float64{
				"critical_metric":    3.0, // High weight
				"secondary_metric":   1.0, // Low weight
			}
			benchmarks := map[string]float64{
				"critical_metric":    100.0,
				"secondary_metric":   100.0,
			}

			result := monitoring.CalculatePerformanceScore(metrics, weights, benchmarks)

			// Weighted average: (50*3 + 90*1) / (3+1) = 240/4 = 60
			Expect(result.OverallScore).To(BeNumerically("~", 60.0, 1.0), "Should apply weights correctly")
		})

		It("should identify improvement areas correctly", func() {
			metrics := map[string]float64{
				"good_metric":    85.0,
				"poor_metric1":   60.0, // Below 70
				"poor_metric2":   50.0, // Below 70
			}
			benchmarks := map[string]float64{
				"good_metric":    100.0,
				"poor_metric1":   100.0,
				"poor_metric2":   100.0,
			}

			result := monitoring.CalculatePerformanceScore(metrics, nil, benchmarks)

			Expect(result.ImprovementAreas).To(ContainElement("poor_metric1"), "Should identify poor_metric1 as improvement area")
			Expect(result.ImprovementAreas).To(ContainElement("poor_metric2"), "Should identify poor_metric2 as improvement area")
			Expect(result.ImprovementAreas).ToNot(ContainElement("good_metric"), "Should not include good metrics in improvement areas")
		})

		It("should assign performance grades correctly", func() {
			testCases := []struct {
				score    float64
				expected string
			}{
				{95.0, "excellent"},
				{85.0, "good"},
				{75.0, "satisfactory"},
				{65.0, "needs_improvement"},
				{45.0, "poor"},
			}

			for _, tc := range testCases {
				metrics := map[string]float64{"test_metric": tc.score}
				benchmarks := map[string]float64{"test_metric": 100.0}

				result := monitoring.CalculatePerformanceScore(metrics, nil, benchmarks)

				Expect(result.PerformanceGrade).To(Equal(tc.expected),
					"Score %.1f should result in grade %s", tc.score, tc.expected)
			}
		})

		It("should handle empty metrics gracefully", func() {
			result := monitoring.CalculatePerformanceScore(map[string]float64{}, nil, nil)

			Expect(result.OverallScore).To(Equal(0.0), "Empty metrics should result in zero score")
		})

		It("should calculate benchmark comparison correctly", func() {
			metrics := map[string]float64{
				"metric1": 120.0, // 20% above benchmark
				"metric2": 80.0,  // 20% below benchmark
			}
			benchmarks := map[string]float64{
				"metric1": 100.0,
				"metric2": 100.0,
			}

			result := monitoring.CalculatePerformanceScore(metrics, nil, benchmarks)

			// Average deviation: (20 + (-20)) / 2 = 0
			Expect(result.BenchmarkComparison).To(BeNumerically("~", 0.0, 1.0), "Should calculate correct benchmark comparison")
		})
	})

	Describe("BR-MON-004: Trend Analysis Algorithms", func() {
		It("should detect increasing trends correctly", func() {
			values := []float64{10, 20, 30, 40, 50}
			timestamps := []time.Time{
				time.Now().Add(-4 * time.Hour),
				time.Now().Add(-3 * time.Hour),
				time.Now().Add(-2 * time.Hour),
				time.Now().Add(-1 * time.Hour),
				time.Now(),
			}

			result := monitoring.AnalyzeTrend(values, timestamps)

			Expect(result.Direction).To(Equal("increasing"), "Should detect increasing trend")
			Expect(result.Slope).To(BeNumerically(">", 0), "Slope should be positive for increasing trend")
			Expect(result.Strength).To(BeNumerically(">", 0.9), "Should have high trend strength")
		})

		It("should detect decreasing trends correctly", func() {
			values := []float64{50, 40, 30, 20, 10}
			timestamps := []time.Time{
				time.Now().Add(-4 * time.Hour),
				time.Now().Add(-3 * time.Hour),
				time.Now().Add(-2 * time.Hour),
				time.Now().Add(-1 * time.Hour),
				time.Now(),
			}

			result := monitoring.AnalyzeTrend(values, timestamps)

			Expect(result.Direction).To(Equal("decreasing"), "Should detect decreasing trend")
			Expect(result.Slope).To(BeNumerically("<", 0), "Slope should be negative for decreasing trend")
			Expect(result.Strength).To(BeNumerically(">", 0.9), "Should have high trend strength")
		})

		It("should detect stable trends correctly", func() {
			values := []float64{50, 50, 50, 50, 50}
			timestamps := []time.Time{
				time.Now().Add(-4 * time.Hour),
				time.Now().Add(-3 * time.Hour),
				time.Now().Add(-2 * time.Hour),
				time.Now().Add(-1 * time.Hour),
				time.Now(),
			}

			result := monitoring.AnalyzeTrend(values, timestamps)

			Expect(result.Direction).To(Equal("stable"), "Should detect stable trend")
			Expect(result.Slope).To(BeNumerically("~", 0, 1e-6), "Slope should be near zero for stable trend")
		})

		It("should calculate volatility correctly", func() {
			// High volatility data
			highVolatilityValues := []float64{10, 50, 20, 60, 15}
			timestamps := []time.Time{
				time.Now().Add(-4 * time.Hour),
				time.Now().Add(-3 * time.Hour),
				time.Now().Add(-2 * time.Hour),
				time.Now().Add(-1 * time.Hour),
				time.Now(),
			}

			highVolResult := monitoring.AnalyzeTrend(highVolatilityValues, timestamps)

			// Low volatility data
			lowVolatilityValues := []float64{50, 51, 49, 52, 48}
			lowVolResult := monitoring.AnalyzeTrend(lowVolatilityValues, timestamps)

			Expect(highVolResult.Volatility).To(BeNumerically(">", lowVolResult.Volatility),
				"High volatility data should have higher volatility score")
			Expect(highVolResult.Volatility).To(BeNumerically(">", 10), "High volatility should be significantly positive")
		})

		It("should handle edge cases in trend analysis", func() {
			// Empty data
			emptyResult := monitoring.AnalyzeTrend([]float64{}, []time.Time{})
			Expect(emptyResult.Direction).To(Equal("unknown"), "Empty data should result in unknown direction")

			// Mismatched arrays
			mismatchedResult := monitoring.AnalyzeTrend([]float64{1, 2}, []time.Time{time.Now()})
			Expect(mismatchedResult.Direction).To(Equal("unknown"), "Mismatched arrays should result in unknown direction")
		})
	})

	Describe("BR-MON-005: Anomaly Detection Algorithms", func() {
		It("should detect anomalies using Z-score method", func() {
			// Normal data with outliers
			values := []float64{10, 12, 11, 13, 10, 11, 12, 50, 9, 11} // 50 is an outlier
			sensitivityThreshold := 2.0 // 2 standard deviations

			result := monitoring.DetectAnomalies(values, sensitivityThreshold)

			Expect(result.AnomalousIndices).To(ContainElement(7), "Should detect index 7 (value 50) as anomalous")
			Expect(len(result.AnomalyScores)).To(Equal(len(values)), "Should calculate anomaly scores for all values")
			Expect(result.AnomalyScores[7]).To(BeNumerically(">", 0.5), "Outlier should have high anomaly score")
		})

		It("should handle normal data without anomalies", func() {
			// Uniform normal data
			values := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
			sensitivityThreshold := 2.0

			result := monitoring.DetectAnomalies(values, sensitivityThreshold)

			Expect(result.AnomalousIndices).To(BeEmpty(), "Normal data should not have anomalies")
			Expect(result.Mean).To(BeNumerically("~", 14.5, 0.1), "Should calculate correct mean")
			Expect(result.StdDev).To(BeNumerically(">", 0), "Should calculate positive standard deviation")
		})

		It("should adjust sensitivity correctly", func() {
			values := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 25} // 25 is moderate outlier

			// High sensitivity (low threshold)
			sensitiveResult := monitoring.DetectAnomalies(values, 1.5)

			// Low sensitivity (high threshold)
			tolerantResult := monitoring.DetectAnomalies(values, 3.0)

			Expect(len(sensitiveResult.AnomalousIndices)).To(BeNumerically(">=", len(tolerantResult.AnomalousIndices)),
				"High sensitivity should detect more or equal anomalies")
		})

		It("should handle edge cases in anomaly detection", func() {
			// Insufficient data
			shortResult := monitoring.DetectAnomalies([]float64{1, 2}, 2.0)
			Expect(shortResult.AnomalousIndices).To(BeEmpty(), "Insufficient data should result in no anomalies")

			// Identical values (zero standard deviation)
			identicalValues := []float64{5, 5, 5, 5, 5}
			identicalResult := monitoring.DetectAnomalies(identicalValues, 2.0)
			Expect(identicalResult.AnomalousIndices).To(BeEmpty(), "Identical values should not generate anomalies")
		})

		It("should calculate anomaly scores in valid range", func() {
			values := []float64{1, 2, 3, 4, 100, 6, 7, 8, 9, 10} // 100 is clear outlier

			result := monitoring.DetectAnomalies(values, 2.0)

			for _, score := range result.AnomalyScores {
				Expect(score).To(BeNumerically(">=", 0), "Anomaly scores should be non-negative")
				// Note: Scores can exceed 1.0 for extreme outliers, which is acceptable
			}
		})
	})
})