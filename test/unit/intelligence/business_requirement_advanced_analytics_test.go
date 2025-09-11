//go:build unit
// +build unit

package intelligence

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/intelligence/analytics"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

/*
 * Business Requirement Validation: Advanced Analytics (Phase 2)
 *
 * This test suite validates Phase 2 business requirements for advanced analytics capabilities
 * following development guidelines:
 * - Reuses existing intelligence test framework (Ginkgo/Gomega)
 * - Extends existing mocks from pattern_discovery_mocks.go
 * - Focuses on business outcomes: strategic planning, capacity optimization, operational efficiency
 * - Uses meaningful assertions with business planning and forecasting thresholds
 * - Integrates with existing intelligence and analytics components
 * - Logs all errors and analytics performance metrics
 */

var _ = Describe("Business Requirement Validation: Advanced Analytics (Phase 2)", func() {
	var (
		ctx                     context.Context
		cancel                  context.CancelFunc
		logger                  *logrus.Logger
		timeSeriesAnalyzer      *analytics.TimeSeriesAnalyzer
		workloadPatternDetector *analytics.WorkloadPatternDetector
		mockExecutionRepo       *MockExecutionRepository
		mockMetricsCollector    *MockMetricsCollector
		mockPatternStore        *MockPatternStore
		commonAssertions        *testutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for analytics metrics
		commonAssertions = testutil.NewCommonAssertions()

		// Reuse existing mocks from intelligence module following development guidelines
		mockExecutionRepo = &MockExecutionRepository{}
		mockMetricsCollector = &MockMetricsCollector{}
		mockPatternStore = &MockPatternStore{}

		// Initialize advanced analytics components for Phase 2 intelligence
		timeSeriesAnalyzer = analytics.NewTimeSeriesAnalyzer(mockMetricsCollector, logger)
		workloadPatternDetector = analytics.NewWorkloadPatternDetector(mockExecutionRepo, mockPatternStore, logger)

		setupPhase2BusinessAnalyticsData(mockMetricsCollector, mockPatternStore)
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-STAT-006
	 * Business Logic: MUST implement time series analysis for business planning and forecasting
	 *
	 * Business Success Criteria:
	 *   - Trend detection accuracy >85% for business planning with strategic decision support
	 *   - Forecasting accuracy within 15% for operational planning and resource allocation
	 *   - Seasonal decomposition with business cycle recognition for budget planning
	 *   - Capacity planning support with growth trend analysis for infrastructure investment
	 *
	 * Test Focus: Time series analytics that enable strategic business planning and operational forecasting
	 * Expected Business Value: Data-driven business planning with accurate trend forecasting and capacity optimization
	 */
	Context("BR-STAT-006: Time Series Analysis for Business Planning", func() {
		It("should provide accurate trend detection for strategic business planning and decision support", func() {
			By("Setting up business time series data with realistic operational patterns")

			// Business Context: Historical operational metrics for strategic planning
			businessTimeSeriesData := []BusinessTimeSeriesDataset{
				{
					MetricName:     "request_volume",
					BusinessDomain: "customer_traffic",
					TimeRange:      90 * 24 * time.Hour, // 90 days of data
					DataPoints: generateRealisticTimeSeries(BusinessTrendPattern{
						BaseValue:         1000.0,         // 1K requests/hour baseline
						TrendSlope:        0.02,           // 2% monthly growth
						SeasonalPeriod:    24 * time.Hour, // Daily seasonality
						SeasonalAmplitude: 0.3,            // 30% daily variation
						NoiseLevel:        0.1,            // 10% random noise
						BusinessCycles: []BusinessCycle{
							{Period: 7 * 24 * time.Hour, Amplitude: 0.15},  // Weekly cycle
							{Period: 30 * 24 * time.Hour, Amplitude: 0.08}, // Monthly cycle
						},
					}),
					ExpectedTrend:      "increasing",
					ExpectedGrowthRate: 0.02, // 2% monthly growth
					BusinessImportance: "critical",
				},
				{
					MetricName:     "infrastructure_costs",
					BusinessDomain: "operational_expenses",
					TimeRange:      180 * 24 * time.Hour, // 6 months of data
					DataPoints: generateRealisticTimeSeries(BusinessTrendPattern{
						BaseValue:         50000.0,             // $50K monthly baseline
						TrendSlope:        0.005,               // 0.5% monthly increase
						SeasonalPeriod:    30 * 24 * time.Hour, // Monthly seasonality
						SeasonalAmplitude: 0.12,                // 12% monthly variation
						NoiseLevel:        0.05,                // 5% random noise
						BusinessCycles: []BusinessCycle{
							{Period: 90 * 24 * time.Hour, Amplitude: 0.08}, // Quarterly variations
						},
					}),
					ExpectedTrend:      "stable_growth",
					ExpectedGrowthRate: 0.005, // 0.5% monthly growth
					BusinessImportance: "high",
				},
				{
					MetricName:     "error_rates",
					BusinessDomain: "service_quality",
					TimeRange:      30 * 24 * time.Hour, // 30 days of data
					DataPoints: generateRealisticTimeSeries(BusinessTrendPattern{
						BaseValue:         0.8,            // 0.8% baseline error rate
						TrendSlope:        -0.01,          // Improving (decreasing) trend
						SeasonalPeriod:    24 * time.Hour, // Daily seasonality
						SeasonalAmplitude: 0.2,            // 20% daily variation
						NoiseLevel:        0.15,           // 15% noise (errors are more variable)
					}),
					ExpectedTrend:      "decreasing",
					ExpectedGrowthRate: -0.01, // Improving by 1% monthly
					BusinessImportance: "critical",
				},
			}

			totalTrendAccuracy := 0.0
			correctTrendDetections := 0

			for _, dataset := range businessTimeSeriesData {
				By(fmt.Sprintf("Analyzing %s trends for %s business domain", dataset.MetricName, dataset.BusinessDomain))

				analysisResult, err := timeSeriesAnalyzer.AnalyzeTrends(ctx, dataset.MetricName, dataset.DataPoints)
				Expect(err).ToNot(HaveOccurred(), "Time series analysis must succeed for business planning")
				Expect(analysisResult).ToNot(BeNil(), "Must provide trend analysis results")

				// Business Requirement: Trend detection accuracy >85%
				trendDetectionAccuracy := calculateTrendAccuracy(analysisResult.DetectedTrend, dataset.ExpectedTrend)
				Expect(trendDetectionAccuracy).To(BeNumerically(">=", 0.85),
					"Trend detection accuracy must be >=85% for reliable business planning for %s", dataset.MetricName)

				totalTrendAccuracy += trendDetectionAccuracy
				correctTrendDetections++

				// Business Validation: Growth rate estimation for business planning
				growthRateError := math.Abs(analysisResult.EstimatedGrowthRate - dataset.ExpectedGrowthRate)
				maxAcceptableError := 0.005 // 0.5% maximum error for business planning
				Expect(growthRateError).To(BeNumerically("<=", maxAcceptableError),
					"Growth rate estimation error must be <=0.5%% for accurate business planning")

				// Business Requirement: Strategic insights for business decision making
				Expect(analysisResult.BusinessInsights).ToNot(BeEmpty(),
					"Must provide business insights for strategic decision making")
				Expect(analysisResult.TrendConfidence).To(BeNumerically(">=", 0.80),
					"Trend confidence must be >=80% for business planning reliability")

				// Business Validation: Statistical significance for business confidence
				Expect(analysisResult.StatisticalSignificance).To(BeTrue(),
					"Trend analysis must be statistically significant for business decision confidence")

				// Log trend analysis for business audit trail
				logger.WithFields(logrus.Fields{
					"metric_name":             dataset.MetricName,
					"business_domain":         dataset.BusinessDomain,
					"detected_trend":          analysisResult.DetectedTrend,
					"expected_trend":          dataset.ExpectedTrend,
					"trend_accuracy":          trendDetectionAccuracy,
					"growth_rate_estimated":   analysisResult.EstimatedGrowthRate,
					"growth_rate_expected":    dataset.ExpectedGrowthRate,
					"trend_confidence":        analysisResult.TrendConfidence,
					"business_importance":     dataset.BusinessImportance,
					"statistical_significant": analysisResult.StatisticalSignificance,
				}).Info("Time series trend analysis business scenario evaluated")
			}

			By("Validating overall trend detection performance for business planning reliability")

			averageTrendAccuracy := totalTrendAccuracy / float64(correctTrendDetections)

			// Business Requirement: Overall high accuracy for business planning confidence
			Expect(averageTrendAccuracy).To(BeNumerically(">=", 0.85),
				"Overall trend detection accuracy must be >=85% for business planning reliability")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":    "BR-STAT-006",
				"datasets_analyzed":       len(businessTimeSeriesData),
				"average_trend_accuracy":  averageTrendAccuracy,
				"business_planning_ready": averageTrendAccuracy >= 0.85,
				"business_impact":         "Time series analysis enables accurate business trend forecasting and strategic planning",
			}).Info("BR-STAT-006: Time series trend analysis business validation completed")
		})

		It("should provide accurate forecasting within business planning tolerances for operational resource allocation", func() {
			By("Testing forecasting accuracy for business operational planning and budget allocation")

			// Business Context: Resource planning scenarios requiring accurate forecasting
			businessForecastingScenarios := []BusinessForecastingScenario{
				{
					MetricName:           "cpu_utilization",
					BusinessPurpose:      "capacity_planning",
					HistoricalPeriod:     60 * 24 * time.Hour, // 60 days historical data
					ForecastHorizon:      30 * 24 * time.Hour, // 30 days forecast
					AccuracyTolerance:    0.15,                // 15% tolerance for capacity planning
					BusinessCriticality:  "high",
					ExpectedBudgetImpact: 25000.0, // $25K infrastructure budget impact
				},
				{
					MetricName:           "storage_consumption",
					BusinessPurpose:      "storage_expansion_planning",
					HistoricalPeriod:     90 * 24 * time.Hour, // 90 days historical data
					ForecastHorizon:      90 * 24 * time.Hour, // 90 days forecast
					AccuracyTolerance:    0.12,                // 12% tolerance for storage planning
					BusinessCriticality:  "critical",
					ExpectedBudgetImpact: 15000.0, // $15K storage budget impact
				},
				{
					MetricName:           "network_bandwidth",
					BusinessPurpose:      "network_capacity_planning",
					HistoricalPeriod:     45 * 24 * time.Hour, // 45 days historical data
					ForecastHorizon:      60 * 24 * time.Hour, // 60 days forecast
					AccuracyTolerance:    0.18,                // 18% tolerance for network planning
					BusinessCriticality:  "medium",
					ExpectedBudgetImpact: 8000.0, // $8K network budget impact
				},
			}

			totalForecastAccuracy := 0.0
			totalBudgetImpact := 0.0
			accurateForecasts := 0

			for _, scenario := range businessForecastingScenarios {
				By(fmt.Sprintf("Testing %s forecasting for %s", scenario.MetricName, scenario.BusinessPurpose))

				// Generate historical data for forecasting
				historicalData := generateBusinessMetricHistory(scenario.MetricName, scenario.HistoricalPeriod)

				// Perform forecasting analysis
				forecastResult, err := timeSeriesAnalyzer.GenerateForecast(ctx, scenario.MetricName, historicalData, scenario.ForecastHorizon)
				Expect(err).ToNot(HaveOccurred(), "Forecasting must succeed for business planning")
				Expect(forecastResult).ToNot(BeNil(), "Must provide forecast results")

				// Business Validation: Forecast completeness
				expectedDataPoints := int(scenario.ForecastHorizon.Hours() / 24) // Daily forecasts
				Expect(len(forecastResult.ForecastData)).To(BeNumerically(">=", expectedDataPoints-5),
					"Forecast must provide complete data for business planning period")

				// Business Requirement: Forecasting accuracy within tolerance for business planning
				// Simulate validation against actual data (in real scenario: compare with actual future data)
				actualData := generateBusinessMetricHistory(scenario.MetricName, scenario.ForecastHorizon)
				forecastAccuracy := calculateForecastAccuracy(forecastResult.ForecastData, actualData)

				if forecastAccuracy >= (1.0 - scenario.AccuracyTolerance) {
					accurateForecasts++
					totalBudgetImpact += scenario.ExpectedBudgetImpact
				}

				Expect(forecastAccuracy).To(BeNumerically(">=", 1.0-scenario.AccuracyTolerance),
					"Forecast accuracy must be within %.1f%% tolerance for %s business planning",
					scenario.AccuracyTolerance*100, scenario.BusinessPurpose)

				totalForecastAccuracy += forecastAccuracy

				// Business Requirement: Confidence intervals for business risk assessment
				Expect(forecastResult.ConfidenceInterval).ToNot(BeNil(),
					"Must provide confidence intervals for business risk assessment")
				Expect(forecastResult.ConfidenceLevel).To(BeNumerically(">=", 0.80),
					"Confidence level must be >=80% for business planning decisions")

				// Business Validation: Resource planning recommendations
				Expect(forecastResult.ResourcePlanningRecommendations).ToNot(BeEmpty(),
					"Must provide actionable resource planning recommendations for business operations")

				// Log forecasting results for business planning audit
				logger.WithFields(logrus.Fields{
					"metric_name":           scenario.MetricName,
					"business_purpose":      scenario.BusinessPurpose,
					"forecast_accuracy":     forecastAccuracy,
					"accuracy_tolerance":    scenario.AccuracyTolerance,
					"forecast_horizon_days": scenario.ForecastHorizon.Hours() / 24,
					"confidence_level":      forecastResult.ConfidenceLevel,
					"business_criticality":  scenario.BusinessCriticality,
					"budget_impact_usd":     scenario.ExpectedBudgetImpact,
					"meets_tolerance":       forecastAccuracy >= (1.0 - scenario.AccuracyTolerance),
				}).Info("Business forecasting scenario evaluated")
			}

			By("Calculating overall forecasting business value and planning reliability")

			averageForecastAccuracy := totalForecastAccuracy / float64(len(businessForecastingScenarios))
			forecastReliabilityRate := float64(accurateForecasts) / float64(len(businessForecastingScenarios))

			// Business Requirement: High overall forecasting accuracy for business planning confidence
			Expect(averageForecastAccuracy).To(BeNumerically(">=", 0.85),
				"Overall forecasting accuracy must be >=85% for reliable business planning")

			// Business Requirement: High reliability rate for business operational planning
			Expect(forecastReliabilityRate).To(BeNumerically(">=", 0.80),
				"Forecast reliability rate must be >=80% for business operational planning confidence")

			// Business Value: Budget planning accuracy
			Expect(totalBudgetImpact).To(BeNumerically(">=", 30000.0),
				"Must demonstrate significant budget planning impact (>=30K USD) for business value")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":      "BR-STAT-006",
				"scenario":                  "forecasting",
				"scenarios_tested":          len(businessForecastingScenarios),
				"average_forecast_accuracy": averageForecastAccuracy,
				"forecast_reliability_rate": forecastReliabilityRate,
				"total_budget_impact_usd":   totalBudgetImpact,
				"business_planning_ready":   averageForecastAccuracy >= 0.85,
				"business_impact":           "Time series forecasting enables accurate business resource planning and budget allocation",
			}).Info("BR-STAT-006: Business forecasting validation completed")
		})

		It("should detect seasonal patterns and business cycles for strategic budget and capacity planning", func() {
			By("Testing seasonal decomposition for business cycle recognition and budget planning")

			// Business Context: Seasonal business patterns affecting resource needs and budgets
			businessSeasonalScenarios := []BusinessSeasonalScenario{
				{
					MetricName:    "customer_traffic",
					BusinessCycle: "quarterly",
					SeasonalPeriods: []time.Duration{
						3 * 30 * 24 * time.Hour, // Quarterly cycle (90 days)
						7 * 24 * time.Hour,      // Weekly cycle
						24 * time.Hour,          // Daily cycle
					},
					ExpectedPeakBusinessPeriods: []string{"Q4", "end_of_week", "business_hours"},
					BusinessImportance:          "critical",
					BudgetPlanningImpact:        "high",
				},
				{
					MetricName:    "processing_workload",
					BusinessCycle: "monthly",
					SeasonalPeriods: []time.Duration{
						30 * 24 * time.Hour, // Monthly cycle
						24 * time.Hour,      // Daily cycle
					},
					ExpectedPeakBusinessPeriods: []string{"month_end", "business_hours"},
					BusinessImportance:          "high",
					BudgetPlanningImpact:        "medium",
				},
			}

			for _, scenario := range businessSeasonalScenarios {
				By(fmt.Sprintf("Analyzing seasonal patterns for %s with %s business cycle", scenario.MetricName, scenario.BusinessCycle))

				// Generate seasonal data with business patterns
				seasonalData := generateBusinessSeasonalData(scenario.MetricName, scenario.SeasonalPeriods, 180*24*time.Hour)

				// Perform seasonal decomposition
				decompositionResult, err := timeSeriesAnalyzer.SeasonalDecomposition(ctx, scenario.MetricName, seasonalData)
				Expect(err).ToNot(HaveOccurred(), "Seasonal decomposition must succeed for business planning")

				// Business Requirement: Detection of major seasonal components
				Expect(len(decompositionResult.SeasonalComponents)).To(BeNumerically(">=", len(scenario.SeasonalPeriods)-1),
					"Must detect major seasonal components for business cycle planning")

				// Business Validation: Peak period identification for business resource planning
				detectedPeakPeriods := decompositionResult.PeakPeriods
				Expect(len(detectedPeakPeriods)).To(BeNumerically(">=", 1),
					"Must identify peak periods for business capacity planning")

				// Business Requirement: Statistical significance of seasonal patterns
				for _, component := range decompositionResult.SeasonalComponents {
					Expect(component.StatisticalSignificance).To(BeTrue(),
						"Seasonal components must be statistically significant for business planning confidence")
					Expect(component.ContributionPercent).To(BeNumerically(">=", 0.05),
						"Seasonal components must contribute >=5%% variance for business relevance")
				}

				// Business Validation: Business cycle alignment
				businessCycleDetected := false
				for _, component := range decompositionResult.SeasonalComponents {
					if isBusinessCyclePeriod(component.Period, scenario.BusinessCycle) {
						businessCycleDetected = true
						break
					}
				}
				Expect(businessCycleDetected).To(BeTrue(),
					"Must detect relevant business cycle periods for strategic planning")

				// Business Requirement: Capacity planning recommendations
				Expect(decompositionResult.CapacityPlanningRecommendations).ToNot(BeEmpty(),
					"Must provide capacity planning recommendations based on seasonal analysis")

				// Log seasonal analysis for business strategic planning
				logger.WithFields(logrus.Fields{
					"metric_name":             scenario.MetricName,
					"business_cycle":          scenario.BusinessCycle,
					"seasonal_components":     len(decompositionResult.SeasonalComponents),
					"peak_periods_detected":   len(detectedPeakPeriods),
					"business_cycle_detected": businessCycleDetected,
					"business_importance":     scenario.BusinessImportance,
					"budget_planning_impact":  scenario.BudgetPlanningImpact,
				}).Info("Business seasonal analysis scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-STAT-006",
				"scenario":             "seasonal_analysis",
				"scenarios_analyzed":   len(businessSeasonalScenarios),
				"business_impact":      "Seasonal analysis enables strategic business cycle planning and budget optimization",
			}).Info("BR-STAT-006: Business seasonal analysis validation completed")
		})
	})

	/*
	 * Business Requirement: BR-CL-009
	 * Business Logic: MUST implement workload pattern detection for capacity optimization and resource planning
	 *
	 * Business Success Criteria:
	 *   - Pattern recognition accuracy with business relevance for operational efficiency
	 *   - Capacity planning with resource optimization insights reducing costs by >20%
	 *   - Performance prediction based on workload similarity for SLA management
	 *   - Workload classification supporting automated resource allocation decisions
	 *
	 * Test Focus: Workload pattern analytics that enable intelligent resource optimization and capacity planning
	 * Expected Business Value: Optimized resource allocation with cost reduction and improved performance prediction
	 */
	Context("BR-CL-009: Workload Pattern Detection for Resource Optimization", func() {
		It("should accurately detect and classify workload patterns for business resource optimization", func() {
			By("Setting up diverse business workload patterns for pattern detection analysis")

			// Business Context: Different workload patterns requiring optimized resource allocation
			businessWorkloadPatterns := []BusinessWorkloadPattern{
				{
					PatternName:    "peak_traffic_pattern",
					BusinessDomain: "web_services",
					WorkloadProfile: WorkloadProfile{
						CPUUtilization: []float64{45, 55, 65, 85, 95, 90, 75, 65, 55, 45}, // Peak hours pattern
						MemoryUsage:    []float64{60, 65, 70, 80, 85, 82, 75, 70, 65, 60},
						RequestRate:    []float64{800, 1000, 1200, 1800, 2200, 2000, 1500, 1200, 1000, 800},
						ResponseTime:   []float64{120, 150, 180, 250, 320, 280, 220, 180, 150, 120},
					},
					ExpectedClassification: "high_intensity",
					OptimalResourceAllocation: ResourceAllocation{
						CPUCores:         8,
						MemoryGB:         16,
						NetworkBandwidth: 1000,
					},
					ExpectedCostReduction: 0.25, // 25% cost reduction through optimization
					BusinessImportance:    "critical",
				},
				{
					PatternName:    "batch_processing_pattern",
					BusinessDomain: "data_processing",
					WorkloadProfile: WorkloadProfile{
						CPUUtilization: []float64{20, 25, 30, 40, 60, 85, 95, 90, 70, 30}, // Batch processing spikes
						MemoryUsage:    []float64{40, 45, 50, 60, 75, 90, 95, 92, 80, 50},
						RequestRate:    []float64{50, 60, 80, 120, 200, 350, 400, 380, 250, 100},
						ResponseTime:   []float64{800, 900, 1000, 1200, 1500, 2000, 2200, 2100, 1600, 1000},
					},
					ExpectedClassification: "batch_intensive",
					OptimalResourceAllocation: ResourceAllocation{
						CPUCores:         12,
						MemoryGB:         32,
						NetworkBandwidth: 500,
					},
					ExpectedCostReduction: 0.30, // 30% cost reduction through batch optimization
					BusinessImportance:    "high",
				},
				{
					PatternName:    "steady_state_pattern",
					BusinessDomain: "background_services",
					WorkloadProfile: WorkloadProfile{
						CPUUtilization: []float64{25, 30, 28, 32, 35, 33, 30, 28, 26, 25}, // Steady low usage
						MemoryUsage:    []float64{45, 48, 46, 50, 52, 50, 48, 46, 45, 44},
						RequestRate:    []float64{100, 120, 110, 130, 140, 135, 125, 115, 105, 100},
						ResponseTime:   []float64{200, 220, 210, 230, 240, 235, 225, 215, 205, 200},
					},
					ExpectedClassification: "steady_state",
					OptimalResourceAllocation: ResourceAllocation{
						CPUCores:         2,
						MemoryGB:         4,
						NetworkBandwidth: 100,
					},
					ExpectedCostReduction: 0.40, // 40% cost reduction through right-sizing
					BusinessImportance:    "medium",
				},
			}

			totalPatternAccuracy := 0.0
			totalCostReduction := 0.0
			correctClassifications := 0

			for _, pattern := range businessWorkloadPatterns {
				By(fmt.Sprintf("Analyzing %s workload pattern for %s domain", pattern.PatternName, pattern.BusinessDomain))

				// Perform workload pattern detection
				patternResult, err := workloadPatternDetector.DetectWorkloadPattern(ctx, pattern.PatternName, pattern.WorkloadProfile)
				Expect(err).ToNot(HaveOccurred(), "Workload pattern detection must succeed for business optimization")
				Expect(patternResult).ToNot(BeNil(), "Must provide pattern detection results")

				// Business Requirement: Accurate pattern classification for business decision making
				classificationAccuracy := calculateClassificationAccuracy(patternResult.DetectedClassification, pattern.ExpectedClassification)
				Expect(classificationAccuracy).To(BeNumerically(">=", 0.80),
					"Pattern classification accuracy must be >=80% for reliable business resource allocation")

				if classificationAccuracy >= 0.80 {
					correctClassifications++
				}
				totalPatternAccuracy += classificationAccuracy

				// Business Requirement: Resource optimization recommendations
				optimizationResult, err := workloadPatternDetector.GenerateOptimizationRecommendations(ctx, patternResult)
				Expect(err).ToNot(HaveOccurred(), "Resource optimization must succeed")
				Expect(optimizationResult).ToNot(BeNil(), "Must provide optimization recommendations")

				// Business Validation: Cost reduction through optimization
				actualCostReduction := calculateCostReduction(pattern.OptimalResourceAllocation, optimizationResult.RecommendedAllocation)
				Expect(actualCostReduction).To(BeNumerically(">=", 0.20),
					"Resource optimization must achieve >=20%% cost reduction for business value")

				// Business Requirement: Cost reduction alignment with expectations
				reductionError := math.Abs(actualCostReduction - pattern.ExpectedCostReduction)
				Expect(reductionError).To(BeNumerically("<=", 0.10),
					"Cost reduction estimation must be within 10%% of expected for business planning accuracy")

				totalCostReduction += actualCostReduction

				// Business Validation: Performance prediction capability
				Expect(patternResult.PerformancePrediction).ToNot(BeNil(),
					"Must provide performance prediction for SLA management")
				Expect(patternResult.PerformancePrediction.Confidence).To(BeNumerically(">=", 0.75),
					"Performance prediction confidence must be >=75% for business SLA planning")

				// Business Requirement: Resource allocation recommendations with business justification
				Expect(optimizationResult.BusinessJustification).ToNot(BeEmpty(),
					"Must provide business justification for resource allocation recommendations")
				Expect(optimizationResult.ROIEstimate).To(BeNumerically(">", 1.0),
					"Resource optimization ROI must be >100% for business investment justification")

				// Log workload pattern analysis for business audit
				logger.WithFields(logrus.Fields{
					"pattern_name":            pattern.PatternName,
					"business_domain":         pattern.BusinessDomain,
					"detected_classification": patternResult.DetectedClassification,
					"expected_classification": pattern.ExpectedClassification,
					"classification_accuracy": classificationAccuracy,
					"cost_reduction_actual":   actualCostReduction,
					"cost_reduction_expected": pattern.ExpectedCostReduction,
					"performance_confidence":  patternResult.PerformancePrediction.Confidence,
					"roi_estimate":            optimizationResult.ROIEstimate,
					"business_importance":     pattern.BusinessImportance,
				}).Info("Workload pattern detection business scenario evaluated")
			}

			By("Calculating overall workload pattern detection business performance")

			averagePatternAccuracy := totalPatternAccuracy / float64(len(businessWorkloadPatterns))
			averageCostReduction := totalCostReduction / float64(len(businessWorkloadPatterns))
			classificationSuccessRate := float64(correctClassifications) / float64(len(businessWorkloadPatterns))

			// Business Requirement: High overall pattern detection accuracy
			Expect(averagePatternAccuracy).To(BeNumerically(">=", 0.80),
				"Overall pattern detection accuracy must be >=80% for business resource optimization reliability")

			// Business Requirement: Significant cost reduction through pattern-based optimization
			Expect(averageCostReduction).To(BeNumerically(">=", 0.20),
				"Average cost reduction must be >=20% for meaningful business value")

			// Business Requirement: High classification success rate for automated decisions
			Expect(classificationSuccessRate).To(BeNumerically(">=", 0.75),
				"Classification success rate must be >=75% for automated business resource allocation")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":        "BR-CL-009",
				"patterns_analyzed":           len(businessWorkloadPatterns),
				"average_pattern_accuracy":    averagePatternAccuracy,
				"average_cost_reduction":      averageCostReduction,
				"classification_success_rate": classificationSuccessRate,
				"business_optimization_ready": averagePatternAccuracy >= 0.80 && averageCostReduction >= 0.20,
				"business_impact":             "Workload pattern detection enables intelligent resource optimization with significant cost reduction",
			}).Info("BR-CL-009: Workload pattern detection business validation completed")
		})

		It("should enable automated resource allocation decisions based on workload pattern classification", func() {
			By("Testing automated resource allocation decision making based on pattern classification")

			// Business Context: Automated resource allocation scenarios for different workload patterns
			automatedAllocationScenarios := []AutomatedAllocationScenario{
				{
					WorkloadPattern: "microservices_api",
					CurrentAllocation: ResourceAllocation{
						CPUCores:         4,
						MemoryGB:         8,
						NetworkBandwidth: 500,
					},
					WorkloadCharacteristics: map[string]float64{
						"request_variability": 0.6, // 60% variability in request patterns
						"memory_efficiency":   0.8, // 80% memory efficiency
						"cpu_burst_frequency": 0.3, // 30% of time in burst mode
						"network_intensity":   0.7, // 70% network intensive
					},
					ExpectedOptimization: ResourceAllocationDecision{
						Action: "horizontal_scale",
						RecommendedAllocation: ResourceAllocation{
							CPUCores:         6,
							MemoryGB:         12,
							NetworkBandwidth: 800,
						},
						ConfidenceLevel:                0.85,
						ExpectedCostChange:             0.20, // 20% cost increase for performance improvement
						ExpectedPerformanceImprovement: 0.35, // 35% performance improvement
					},
					BusinessPriority: "high",
				},
				{
					WorkloadPattern: "data_analytics_batch",
					CurrentAllocation: ResourceAllocation{
						CPUCores:         16,
						MemoryGB:         64,
						NetworkBandwidth: 1000,
					},
					WorkloadCharacteristics: map[string]float64{
						"request_variability": 0.9, // 90% variability (batch spikes)
						"memory_efficiency":   0.6, // 60% memory efficiency (can optimize)
						"cpu_burst_frequency": 0.8, // 80% of time in burst mode
						"network_intensity":   0.4, // 40% network intensive
					},
					ExpectedOptimization: ResourceAllocationDecision{
						Action: "rightsizing",
						RecommendedAllocation: ResourceAllocation{
							CPUCores:         12,
							MemoryGB:         48,
							NetworkBandwidth: 600,
						},
						ConfidenceLevel:                0.90,
						ExpectedCostChange:             -0.25, // 25% cost reduction
						ExpectedPerformanceImprovement: 0.10,  // 10% performance improvement through optimization
					},
					BusinessPriority: "medium",
				},
			}

			totalDecisionAccuracy := 0.0
			businessValueDelivered := 0.0
			correctAutomatedDecisions := 0

			for _, scenario := range automatedAllocationScenarios {
				By(fmt.Sprintf("Testing automated allocation for %s workload pattern", scenario.WorkloadPattern))

				// Perform automated resource allocation decision
				allocationDecision, err := workloadPatternDetector.AutomateResourceAllocation(
					ctx,
					scenario.WorkloadPattern,
					scenario.CurrentAllocation,
					scenario.WorkloadCharacteristics,
				)
				Expect(err).ToNot(HaveOccurred(), "Automated resource allocation must succeed for business efficiency")
				Expect(allocationDecision).ToNot(BeNil(), "Must provide allocation decision")

				// Business Requirement: High confidence automated decisions for business reliability
				Expect(allocationDecision.ConfidenceLevel).To(BeNumerically(">=", 0.80),
					"Automated decision confidence must be >=80% for business automation reliability")

				// Business Validation: Decision accuracy against expected optimization
				decisionAccuracy := calculateDecisionAccuracy(allocationDecision, scenario.ExpectedOptimization)
				Expect(decisionAccuracy).To(BeNumerically(">=", 0.75),
					"Automated decision accuracy must be >=75% for business automation trust")

				if decisionAccuracy >= 0.75 {
					correctAutomatedDecisions++
				}
				totalDecisionAccuracy += decisionAccuracy

				// Business Requirement: Cost-performance optimization balance
				costImpactError := math.Abs(allocationDecision.ExpectedCostChange - scenario.ExpectedOptimization.ExpectedCostChange)
				Expect(costImpactError).To(BeNumerically("<=", 0.10),
					"Cost impact estimation must be within 10%% for business budget planning accuracy")

				// Business Validation: Performance improvement justification
				if allocationDecision.ExpectedCostChange > 0 {
					// If cost increases, performance improvement must justify the cost
					Expect(allocationDecision.ExpectedPerformanceImprovement).To(BeNumerically(">=", 0.20),
						"Performance improvement must be >=20%% to justify cost increase for business value")
				}

				// Business Requirement: Automated decision auditability
				Expect(allocationDecision.DecisionReasoning).ToNot(BeEmpty(),
					"Must provide decision reasoning for business audit and compliance")
				Expect(allocationDecision.BusinessImpactAssessment).ToNot(BeEmpty(),
					"Must assess business impact for automated decision accountability")

				// Calculate business value from automation
				automationValue := calculateAutomationBusinessValue(scenario.CurrentAllocation, allocationDecision)
				businessValueDelivered += automationValue

				// Log automated allocation decision for business audit
				logger.WithFields(logrus.Fields{
					"workload_pattern":                 scenario.WorkloadPattern,
					"recommended_action":               allocationDecision.Action,
					"confidence_level":                 allocationDecision.ConfidenceLevel,
					"decision_accuracy":                decisionAccuracy,
					"expected_cost_change":             allocationDecision.ExpectedCostChange,
					"expected_performance_improvement": allocationDecision.ExpectedPerformanceImprovement,
					"automation_value_usd":             automationValue,
					"business_priority":                scenario.BusinessPriority,
					"decision_automated":               true,
				}).Info("Automated resource allocation business scenario evaluated")
			}

			By("Validating overall automated decision making business performance")

			averageDecisionAccuracy := totalDecisionAccuracy / float64(len(automatedAllocationScenarios))
			automatedDecisionSuccessRate := float64(correctAutomatedDecisions) / float64(len(automatedAllocationScenarios))

			// Business Requirement: High automated decision accuracy for business operational efficiency
			Expect(averageDecisionAccuracy).To(BeNumerically(">=", 0.75),
				"Average automated decision accuracy must be >=75% for business automation reliability")

			// Business Requirement: High success rate for business automation confidence
			Expect(automatedDecisionSuccessRate).To(BeNumerically(">=", 0.70),
				"Automated decision success rate must be >=70% for business automation adoption")

			// Business Value: Significant automation value for business efficiency
			Expect(businessValueDelivered).To(BeNumerically(">=", 5000.0),
				"Automated decisions must deliver >=5K USD business value for automation justification")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":       "BR-CL-009",
				"scenario":                   "automated_allocation",
				"scenarios_tested":           len(automatedAllocationScenarios),
				"average_decision_accuracy":  averageDecisionAccuracy,
				"automated_success_rate":     automatedDecisionSuccessRate,
				"total_automation_value_usd": businessValueDelivered,
				"business_automation_ready":  averageDecisionAccuracy >= 0.75 && automatedDecisionSuccessRate >= 0.70,
				"business_impact":            "Automated resource allocation delivers business efficiency with reliable decision making",
			}).Info("BR-CL-009: Automated resource allocation business validation completed")
		})
	})
})

// Business type definitions for Phase 2 Advanced Analytics

type BusinessTimeSeriesDataset struct {
	MetricName         string
	BusinessDomain     string
	TimeRange          time.Duration
	DataPoints         []TimeSeriesPoint
	ExpectedTrend      string
	ExpectedGrowthRate float64
	BusinessImportance string
}

type BusinessTrendPattern struct {
	BaseValue         float64
	TrendSlope        float64
	SeasonalPeriod    time.Duration
	SeasonalAmplitude float64
	NoiseLevel        float64
	BusinessCycles    []BusinessCycle
}

type BusinessCycle struct {
	Period    time.Duration
	Amplitude float64
}

type BusinessForecastingScenario struct {
	MetricName           string
	BusinessPurpose      string
	HistoricalPeriod     time.Duration
	ForecastHorizon      time.Duration
	AccuracyTolerance    float64
	BusinessCriticality  string
	ExpectedBudgetImpact float64
}

type BusinessSeasonalScenario struct {
	MetricName                  string
	BusinessCycle               string
	SeasonalPeriods             []time.Duration
	ExpectedPeakBusinessPeriods []string
	BusinessImportance          string
	BudgetPlanningImpact        string
}

type BusinessWorkloadPattern struct {
	PatternName               string
	BusinessDomain            string
	WorkloadProfile           WorkloadProfile
	ExpectedClassification    string
	OptimalResourceAllocation ResourceAllocation
	ExpectedCostReduction     float64
	BusinessImportance        string
}

type WorkloadProfile struct {
	CPUUtilization []float64
	MemoryUsage    []float64
	RequestRate    []float64
	ResponseTime   []float64
}

type ResourceAllocation struct {
	CPUCores         int
	MemoryGB         int
	NetworkBandwidth int
}

type AutomatedAllocationScenario struct {
	WorkloadPattern         string
	CurrentAllocation       ResourceAllocation
	WorkloadCharacteristics map[string]float64
	ExpectedOptimization    ResourceAllocationDecision
	BusinessPriority        string
}

type ResourceAllocationDecision struct {
	Action                         string
	RecommendedAllocation          ResourceAllocation
	ConfidenceLevel                float64
	ExpectedCostChange             float64
	ExpectedPerformanceImprovement float64
	DecisionReasoning              string
	BusinessImpactAssessment       string
}

type TimeSeriesPoint struct {
	Timestamp time.Time
	Value     float64
}

// Business helper functions for Phase 2 Advanced Analytics testing

func setupPhase2BusinessAnalyticsData(mockMetricsCollector *MockMetricsCollector, mockPatternStore *MockPatternStore) {
	// Setup realistic business analytics data following existing mock patterns
	businessAnalyticsPatterns := []AnalyticsPattern{
		{
			PatternType:    "time_series_trend",
			Accuracy:       0.88,
			BusinessDomain: "capacity_planning",
			DataSize:       1000,
		},
		{
			PatternType:    "workload_classification",
			Accuracy:       0.85,
			BusinessDomain: "resource_optimization",
			DataSize:       5000,
		},
	}

	for _, pattern := range businessAnalyticsPatterns {
		mockPatternStore.StorePattern(pattern.PatternType, pattern)
	}
}

func generateRealisticTimeSeries(pattern BusinessTrendPattern) []TimeSeriesPoint {
	// Generate realistic time series data with business patterns
	dataPoints := make([]TimeSeriesPoint, 0)
	currentTime := time.Now().Add(-pattern.SeasonalPeriod * 10) // 10 periods of historical data

	for i := 0; i < 1000; i++ { // Generate 1000 data points
		// Calculate trend component
		trendValue := pattern.BaseValue * (1 + pattern.TrendSlope*float64(i)/100)

		// Add seasonal component
		seasonalOffset := pattern.SeasonalAmplitude * math.Sin(2*math.Pi*float64(i)/float64(pattern.SeasonalPeriod.Hours()))

		// Add business cycles
		cycleOffset := 0.0
		for _, cycle := range pattern.BusinessCycles {
			cycleOffset += cycle.Amplitude * math.Cos(2*math.Pi*float64(i)/float64(cycle.Period.Hours()))
		}

		// Add noise
		noise := pattern.NoiseLevel * (math.Round(math.Sin(float64(i)*0.1)*100) / 100) // Deterministic "noise"

		finalValue := trendValue * (1 + seasonalOffset + cycleOffset + noise)

		dataPoints = append(dataPoints, TimeSeriesPoint{
			Timestamp: currentTime.Add(time.Duration(i) * time.Hour),
			Value:     math.Max(0, finalValue),
		})
	}

	return dataPoints
}

func generateBusinessMetricHistory(metricName string, duration time.Duration) []TimeSeriesPoint {
	// Generate business-realistic metric history for forecasting validation
	dataPoints := make([]TimeSeriesPoint, 0)
	hours := int(duration.Hours())
	baseValue := getBaseValueForMetric(metricName)

	currentTime := time.Now().Add(-duration)

	for i := 0; i < hours; i++ {
		// Add realistic business patterns based on metric type
		value := baseValue

		if metricName == "cpu_utilization" {
			// CPU usage with daily patterns and gradual growth
			dailyPattern := 0.3 * math.Sin(2*math.Pi*float64(i%24)/24) // Daily cycle
			growth := 0.001 * float64(i)                               // Gradual growth
			value = baseValue * (1 + dailyPattern + growth)
		} else if metricName == "storage_consumption" {
			// Storage with steady growth
			growth := 0.002 * float64(i) // Steady growth
			value = baseValue * (1 + growth)
		} else if metricName == "network_bandwidth" {
			// Network with business hours patterns
			businessHoursPattern := 0.4 * math.Sin(2*math.Pi*float64(i%24)/24)
			value = baseValue * (1 + businessHoursPattern)
		}

		dataPoints = append(dataPoints, TimeSeriesPoint{
			Timestamp: currentTime.Add(time.Duration(i) * time.Hour),
			Value:     math.Max(0, value),
		})
	}

	return dataPoints
}

func generateBusinessSeasonalData(metricName string, periods []time.Duration, totalDuration time.Duration) []TimeSeriesPoint {
	// Generate seasonal data with multiple business cycles
	dataPoints := make([]TimeSeriesPoint, 0)
	hours := int(totalDuration.Hours())
	baseValue := getBaseValueForMetric(metricName)

	currentTime := time.Now().Add(-totalDuration)

	for i := 0; i < hours; i++ {
		value := baseValue

		// Add seasonal components for each business period
		for _, period := range periods {
			amplitude := 0.2 // 20% amplitude for each seasonal component
			seasonalComponent := amplitude * math.Sin(2*math.Pi*float64(i)/period.Hours())
			value *= (1 + seasonalComponent)
		}

		dataPoints = append(dataPoints, TimeSeriesPoint{
			Timestamp: currentTime.Add(time.Duration(i) * time.Hour),
			Value:     math.Max(0, value),
		})
	}

	return dataPoints
}

func calculateTrendAccuracy(detected, expected string) float64 {
	// Calculate trend detection accuracy for business validation
	if detected == expected {
		return 1.0
	}

	// Partial credit for related trends
	relatedTrends := map[string][]string{
		"increasing":    {"stable_growth"},
		"decreasing":    {"stable_decline"},
		"stable_growth": {"increasing"},
	}

	if related, exists := relatedTrends[expected]; exists {
		for _, trend := range related {
			if detected == trend {
				return 0.8 // 80% accuracy for related trends
			}
		}
	}

	return 0.0
}

func calculateForecastAccuracy(forecast, actual []TimeSeriesPoint) float64 {
	// Calculate forecasting accuracy for business planning validation
	if len(forecast) == 0 || len(actual) == 0 {
		return 0.0
	}

	totalError := 0.0
	validPoints := 0

	minLength := len(forecast)
	if len(actual) < minLength {
		minLength = len(actual)
	}

	for i := 0; i < minLength; i++ {
		if actual[i].Value > 0 {
			error := math.Abs(forecast[i].Value-actual[i].Value) / actual[i].Value
			totalError += error
			validPoints++
		}
	}

	if validPoints == 0 {
		return 0.0
	}

	averageError := totalError / float64(validPoints)
	accuracy := math.Max(0, 1.0-averageError)

	return accuracy
}

func calculateClassificationAccuracy(detected, expected string) float64 {
	// Calculate workload classification accuracy
	if detected == expected {
		return 1.0
	}

	// Partial credit for similar classifications
	similarClassifications := map[string][]string{
		"high_intensity":  {"peak_intensive"},
		"batch_intensive": {"compute_intensive"},
		"steady_state":    {"low_intensity"},
	}

	if similar, exists := similarClassifications[expected]; exists {
		for _, classification := range similar {
			if detected == classification {
				return 0.7 // 70% accuracy for similar classifications
			}
		}
	}

	return 0.0
}

func calculateCostReduction(optimal, recommended ResourceAllocation) float64 {
	// Calculate cost reduction from resource optimization
	optimalCost := float64(optimal.CPUCores*100 + optimal.MemoryGB*50 + optimal.NetworkBandwidth)
	recommendedCost := float64(recommended.CPUCores*100 + recommended.MemoryGB*50 + recommended.NetworkBandwidth)

	if optimalCost <= 0 {
		return 0.0
	}

	return (optimalCost - recommendedCost) / optimalCost
}

func calculateDecisionAccuracy(decision, expected ResourceAllocationDecision) float64 {
	// Calculate automated decision accuracy
	accuracy := 0.0

	// Action accuracy (40% weight)
	if decision.Action == expected.Action {
		accuracy += 0.4
	}

	// Cost change accuracy (30% weight)
	costError := math.Abs(decision.ExpectedCostChange - expected.ExpectedCostChange)
	if costError <= 0.1 { // Within 10%
		accuracy += 0.3
	}

	// Performance improvement accuracy (30% weight)
	perfError := math.Abs(decision.ExpectedPerformanceImprovement - expected.ExpectedPerformanceImprovement)
	if perfError <= 0.15 { // Within 15%
		accuracy += 0.3
	}

	return accuracy
}

func calculateAutomationBusinessValue(current ResourceAllocation, decision ResourceAllocationDecision) float64 {
	// Calculate business value from automated resource allocation
	currentCost := float64(current.CPUCores*100 + current.MemoryGB*50 + current.NetworkBandwidth)
	costSavingsPerMonth := currentCost * math.Abs(decision.ExpectedCostChange)

	// Add performance improvement value
	performanceValue := decision.ExpectedPerformanceImprovement * 2000 // $2K per 100% performance improvement

	// Add automation efficiency value (reduced manual effort)
	automationValue := 1000.0 // $1K value from automation

	return costSavingsPerMonth + performanceValue + automationValue
}

func getBaseValueForMetric(metricName string) float64 {
	// Get base values for different business metrics
	baseValues := map[string]float64{
		"cpu_utilization":     65.0,   // 65% baseline
		"storage_consumption": 500.0,  // 500 GB baseline
		"network_bandwidth":   800.0,  // 800 Mbps baseline
		"customer_traffic":    1200.0, // 1200 requests/hour baseline
		"processing_workload": 350.0,  // 350 jobs/hour baseline
	}

	if baseValue, exists := baseValues[metricName]; exists {
		return baseValue
	}

	return 100.0 // Default baseline
}

func isBusinessCyclePeriod(period time.Duration, businessCycle string) bool {
	// Check if detected period matches expected business cycle
	switch businessCycle {
	case "quarterly":
		return period >= 80*24*time.Hour && period <= 100*24*time.Hour // ~90 days
	case "monthly":
		return period >= 25*24*time.Hour && period <= 35*24*time.Hour // ~30 days
	case "weekly":
		return period >= 6*24*time.Hour && period <= 8*24*time.Hour // ~7 days
	case "daily":
		return period >= 20*time.Hour && period <= 28*time.Hour // ~24 hours
	}

	return false
}

// Helper types for advanced analytics testing

type AnalyticsPattern struct {
	PatternType    string
	Accuracy       float64
	BusinessDomain string
	DataSize       int
}
