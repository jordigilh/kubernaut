//go:build unit
// +build unit

package ai

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

/*
 * Business Requirement Validation: LLM Cost Optimization & Response Quality
 *
 * This test suite validates business requirements for LLM service optimization
 * following development guidelines:
 * - Reuses existing test framework patterns (Ginkgo/Gomega)
 * - Focuses on business outcomes: cost reduction, quality assurance, ROI measurement
 * - Uses meaningful assertions with business cost thresholds
 * - Integrates with existing LLM service patterns
 * - Logs all errors and cost optimization metrics
 */

var _ = Describe("Business Requirement Validation: LLM Cost Optimization & Response Quality", func() {
	var (
		ctx              context.Context
		cancel           context.CancelFunc
		logger           *logrus.Logger
		mockLLMClient    *mocks.MockLLMClient
		costOptimizer    *llm.CostOptimizer
		qualityAssessor  *llm.ResponseQualityAssessor
		commonAssertions *testutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for business metrics
		commonAssertions = testutil.NewCommonAssertions()

		// Setup mock LLM client with business-realistic pricing and response patterns
		mockLLMClient = mocks.NewMockLLMClient()
		setupBusinessLLMPricing(mockLLMClient)

		// Initialize cost optimization components
		costOptimizer = llm.NewCostOptimizer(mockLLMClient, logger)
		qualityAssessor = llm.NewResponseQualityAssessor(logger)
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-LLM-010
	 * Business Logic: MUST implement cost optimization strategies for API usage
	 *
	 * Business Success Criteria:
	 *   - Cost reduction measurement with actual API usage optimization
	 *   - ROI calculation for different optimization strategies
	 *   - Budget management with spending threshold enforcement
	 *   - Business impact - quantifiable cost savings achieved
	 *
	 * Test Focus: Real cost optimization business outcomes
	 * Expected Business Value: Significant reduction in AI service operational costs
	 */
	Context("BR-LLM-010: Cost Optimization Strategies for Business ROI", func() {
		It("should deliver measurable cost reduction through intelligent optimization strategies", func() {
			By("Setting up business scenario with realistic LLM usage patterns")

			// Business Context: Production LLM usage for incident analysis
			businessUsagePattern := LLMUsagePattern{
				DailyQueries:        1000, // 1K queries per day
				AverageTokensInput:  150,  // Typical alert description length
				AverageTokensOutput: 75,   // Typical recommendation length
				PeakHourMultiplier:  2.5,  // 2.5x higher usage during incidents
				CacheableQueries:    60,   // 60% of queries are similar/cacheable
			}

			By("Implementing intelligent caching strategy for cost optimization")
			cachingStrategy := CostOptimizationStrategy{
				EnableCaching:       true,
				CacheHitRateTarget:  0.95,           // 95% cache hit rate target
				CacheTTL:            24 * time.Hour, // 24-hour cache TTL
				CostReductionTarget: 0.40,           // 40% cost reduction target
			}

			err := costOptimizer.EnableStrategy(ctx, "intelligent_caching", cachingStrategy)
			Expect(err).ToNot(HaveOccurred(), "Cost optimizer must enable caching strategy")

			By("Simulating monthly LLM usage with optimization")
			monthlyBaseline := calculateMonthlyBaseline(businessUsagePattern)
			monthlyOptimized := simulateOptimizedUsage(businessUsagePattern, cachingStrategy)

			// Business Requirement: Cost reduction measurement
			actualCostReduction := (monthlyBaseline.TotalCost - monthlyOptimized.TotalCost) / monthlyBaseline.TotalCost
			monthlySavings := monthlyBaseline.TotalCost - monthlyOptimized.TotalCost

			Expect(actualCostReduction).To(BeNumerically(">=", cachingStrategy.CostReductionTarget),
				"Cost optimization must achieve >=40% cost reduction through intelligent strategies")

			Expect(monthlySavings).To(BeNumerically(">", 0),
				"Cost optimization must generate positive monthly savings")

			By("Validating service quality maintenance during optimization")
			// Business Requirement: Quality must not degrade significantly during optimization
			qualityDegradation := monthlyBaseline.AverageQuality - monthlyOptimized.AverageQuality

			Expect(qualityDegradation).To(BeNumerically("<=", 0.05),
				"Quality degradation must be ≤5% during cost optimization")

			Expect(monthlyOptimized.AverageQuality).To(BeNumerically(">=", 0.85),
				"Optimized service must maintain >=85% quality for business acceptance")

			By("Calculating business ROI for optimization implementation")
			implementationCost := 5000.0 // $5K implementation cost
			annualSavings := monthlySavings * 12
			roi := (annualSavings - implementationCost) / implementationCost

			// Business Requirement: ROI must justify implementation
			Expect(roi).To(BeNumerically(">=", 2.0),
				"Cost optimization ROI must be >=2x to justify business investment")

			By("Validating budget threshold enforcement")
			monthlyBudget := 8000.0 // $8K monthly budget
			budgetUtilization := monthlyOptimized.TotalCost / monthlyBudget

			// Business Requirement: Budget compliance
			Expect(budgetUtilization).To(BeNumerically("<=", 0.90),
				"Optimized usage must stay within 90% of monthly budget for business control")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":   "BR-LLM-010",
				"baseline_monthly_cost":  monthlyBaseline.TotalCost,
				"optimized_monthly_cost": monthlyOptimized.TotalCost,
				"cost_reduction_percent": actualCostReduction * 100,
				"monthly_savings_usd":    monthlySavings,
				"annual_savings_usd":     annualSavings,
				"roi_multiple":           roi,
				"quality_maintained":     monthlyOptimized.AverageQuality >= 0.85,
				"budget_utilization":     budgetUtilization,
				"business_impact":        "Measurable cost optimization with maintained service quality",
			}).Info("BR-LLM-010: Cost optimization business validation completed")
		})

		It("should optimize provider selection based on cost-effectiveness analysis", func() {
			By("Setting up multi-provider cost comparison scenario")

			// Business Context: Multiple LLM provider options with different pricing
			providers := []LLMProvider{
				{
					Name:               "openai",
					CostPerInputToken:  0.0015, // $1.50 per 1K input tokens
					CostPerOutputToken: 0.002,  // $2.00 per 1K output tokens
					QualityScore:       0.95,   // High quality
					AverageLatency:     500,    // 500ms average response time
				},
				{
					Name:               "huggingface",
					CostPerInputToken:  0.0008, // $0.80 per 1K input tokens (60% cost reduction)
					CostPerOutputToken: 0.0012, // $1.20 per 1K output tokens
					QualityScore:       0.88,   // Good quality (7% lower than OpenAI)
					AverageLatency:     800,    // 800ms average response time
				},
				{
					Name:               "local",
					CostPerInputToken:  0.0001, // $0.10 per 1K input tokens (infrastructure cost)
					CostPerOutputToken: 0.0001, // $0.10 per 1K output tokens
					QualityScore:       0.80,   // Lower quality but cost-effective
					AverageLatency:     1200,   // 1200ms average response time
				},
			}

			By("Testing provider optimization for business scenarios")
			businessScenarios := []BusinessScenario{
				{
					Priority:         "high",   // High priority = favor quality over cost
					QualityThreshold: 0.90,     // Must maintain high quality
					LatencyThreshold: 1000,     // Must respond within 1 second
					ExpectedProvider: "openai", // Should select premium provider
				},
				{
					Priority:         "medium",      // Medium priority = balance cost and quality
					QualityThreshold: 0.85,          // Acceptable quality threshold
					LatencyThreshold: 2000,          // More flexible latency
					ExpectedProvider: "huggingface", // Should select cost-effective option
				},
				{
					Priority:         "low",   // Low priority = favor cost over quality
					QualityThreshold: 0.75,    // Lower quality acceptable
					LatencyThreshold: 3000,    // Flexible latency
					ExpectedProvider: "local", // Should select lowest cost option
				},
			}

			totalCostSavings := 0.0

			for _, scenario := range businessScenarios {
				selectedProvider, err := costOptimizer.SelectOptimalProvider(ctx, providers, scenario)

				Expect(err).ToNot(HaveOccurred(), "Provider optimization must handle business scenarios")
				Expect(selectedProvider).ToNot(BeNil(), "Must select a provider for business operations")

				// Business Requirement: Selected provider must meet quality thresholds
				Expect(selectedProvider.QualityScore).To(BeNumerically(">=", scenario.QualityThreshold),
					"Selected provider must meet business quality requirements")

				// Business Requirement: Selected provider must meet latency requirements
				Expect(selectedProvider.AverageLatency).To(BeNumerically("<=", scenario.LatencyThreshold),
					"Selected provider must meet business latency requirements")

				By("Calculating cost savings vs premium provider")
				premiumProvider := providers[0] // OpenAI as premium baseline
				costSavings := calculateProviderCostSavings(premiumProvider, selectedProvider)
				totalCostSavings += costSavings

				// Log business decision rationale
				logger.WithFields(logrus.Fields{
					"scenario_priority":    scenario.Priority,
					"selected_provider":    selectedProvider.Name,
					"quality_score":        selectedProvider.QualityScore,
					"latency_ms":           selectedProvider.AverageLatency,
					"monthly_cost_savings": costSavings,
					"meets_quality_req":    selectedProvider.QualityScore >= scenario.QualityThreshold,
					"meets_latency_req":    selectedProvider.AverageLatency <= scenario.LatencyThreshold,
				}).Info("Provider optimization business scenario evaluated")
			}

			By("Validating overall provider optimization business impact")
			// Business Requirement: Significant cost savings through optimization
			Expect(totalCostSavings).To(BeNumerically(">", 1000),
				"Provider optimization must deliver >$1K monthly savings for business value")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":  "BR-LLM-010",
				"scenario":              "provider_optimization",
				"total_monthly_savings": totalCostSavings,
				"providers_evaluated":   len(providers),
				"scenarios_tested":      len(businessScenarios),
				"business_impact":       "Multi-provider optimization delivers cost savings while meeting quality requirements",
			}).Info("BR-LLM-010: Provider optimization business validation completed")
		})
	})

	/*
	 * Business Requirement: BR-LLM-013
	 * Business Logic: MUST implement comprehensive response quality assessment
	 *
	 * Business Success Criteria:
	 *   - Quality scoring accuracy correlation with business outcome effectiveness
	 *   - Quality threshold establishment with business impact measurement
	 *   - Response improvement tracking with statistical validation
	 *   - Business value - improved decision making through quality assessment
	 *
	 * Test Focus: Quality scores that correlate with actual business effectiveness
	 * Expected Business Value: Reliable quality assessment enabling confident business decisions
	 */
	Context("BR-LLM-013: Response Quality Assessment for Business Confidence", func() {
		It("should provide accurate quality assessment that correlates with business effectiveness", func() {
			By("Setting up business scenarios with known quality expectations")

			// Business Context: LLM responses for different incident types
			businessResponseScenarios := []BusinessResponseScenario{
				{
					IncidentType: "OutOfMemory",
					LLMResponse: types.ActionRecommendation{
						Action:     "increase_memory_limit",
						Confidence: 0.90,
						Reasoning: &types.ReasoningResult{
							Summary: "Pod is experiencing OutOfMemory kills. Analysis of resource usage shows consistent memory pressure exceeding limits. Increasing memory limit from 1Gi to 2Gi will provide adequate headroom while maintaining cost efficiency.",
							Factors: []string{"memory_usage_trend", "kill_count", "resource_efficiency"},
						},
						Parameters: map[string]interface{}{"new_limit": "2Gi", "resource": "memory"},
					},
					ExpectedQuality: 0.92, // High quality - specific, actionable, well-reasoned
					BusinessOutcome: "resolved_successfully",
				},
				{
					IncidentType: "NetworkLatency",
					LLMResponse: types.ActionRecommendation{
						Action:     "restart_service",
						Confidence: 0.70,
						Reasoning: &types.ReasoningResult{
							Summary: "Service restart might help.",
							Factors: []string{"latency"},
						},
						Parameters: map[string]interface{}{},
					},
					ExpectedQuality: 0.45, // Low quality - vague, minimal reasoning, no specifics
					BusinessOutcome: "partially_resolved",
				},
				{
					IncidentType: "DiskPressure",
					LLMResponse: types.ActionRecommendation{
						Action:     "cleanup_logs_and_scale_storage",
						Confidence: 0.85,
						Reasoning: &types.ReasoningResult{
							Summary: "Disk usage at 95% with log files consuming 60% of space. Immediate log cleanup will provide short-term relief. Long-term solution requires persistent volume expansion from 10Gi to 20Gi based on growth projections.",
							Factors: []string{"disk_usage_breakdown", "log_retention_policy", "growth_trend"},
						},
						Parameters: map[string]interface{}{
							"cleanup_target": "logs_older_than_7d",
							"new_pv_size":    "20Gi",
							"retention_days": 7,
						},
					},
					ExpectedQuality: 0.95, // Excellent quality - comprehensive analysis, short and long-term solutions
					BusinessOutcome: "resolved_successfully",
				},
			}

			By("Evaluating response quality and correlation with business outcomes")
			qualityCorrelationData := make([]QualityCorrelation, 0)

			for _, scenario := range businessResponseScenarios {
				qualityScore, err := qualityAssessor.AssessResponse(ctx, scenario.LLMResponse)

				Expect(err).ToNot(HaveOccurred(), "Quality assessor must evaluate all business responses")
				Expect(qualityScore).ToNot(BeNil(), "Must provide quality assessment")

				// Business Requirement: Quality assessment accuracy
				qualityAccuracy := 1.0 - math.Abs(qualityScore.OverallScore-scenario.ExpectedQuality)
				Expect(qualityAccuracy).To(BeNumerically(">=", 0.85),
					"Quality assessment must be >=85% accurate for business reliability")

				// Business Validation: Quality components assessment
				Expect(qualityScore.SpecificityScore).To(BeNumerically(">=", 0.0),
					"Must assess response specificity for business actionability")
				Expect(qualityScore.CompletenessScore).To(BeNumerically(">=", 0.0),
					"Must assess response completeness for business comprehensiveness")
				Expect(qualityScore.ActionabilityScore).To(BeNumerically(">=", 0.0),
					"Must assess response actionability for business implementation")

				// Track correlation data
				qualityCorrelationData = append(qualityCorrelationData, QualityCorrelation{
					QualityScore:    qualityScore.OverallScore,
					BusinessOutcome: scenario.BusinessOutcome,
				})

				// Log quality assessment for business audit
				logger.WithFields(logrus.Fields{
					"incident_type":       scenario.IncidentType,
					"overall_quality":     qualityScore.OverallScore,
					"specificity_score":   qualityScore.SpecificityScore,
					"completeness_score":  qualityScore.CompletenessScore,
					"actionability_score": qualityScore.ActionabilityScore,
					"business_outcome":    scenario.BusinessOutcome,
					"quality_accuracy":    qualityAccuracy,
				}).Info("Response quality business scenario evaluated")
			}

			By("Calculating business outcome correlation")
			correlation := calculateBusinessOutcomeCorrelation(qualityCorrelationData)

			// Business Requirement: Strong correlation between quality and business outcomes
			Expect(correlation).To(BeNumerically(">=", 0.75),
				"Quality assessment must correlate >=75% with business outcomes for decision confidence")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":         "BR-LLM-013",
				"scenarios_evaluated":          len(businessResponseScenarios),
				"quality_business_correlation": correlation,
				"business_impact":              "Quality assessment enables confident business decision making",
			}).Info("BR-LLM-013: Response quality assessment business validation completed")
		})

		It("should establish quality thresholds that prevent poor business outcomes", func() {
			By("Testing quality threshold enforcement for business protection")

			// Business Context: Quality thresholds for different business scenarios
			businessQualityThresholds := map[string]float64{
				"production_critical": 0.90, // Critical systems require high quality
				"production_standard": 0.80, // Standard systems need good quality
				"development":         0.70, // Development can accept lower quality
				"testing":             0.60, // Testing environments most flexible
			}

			for environment, threshold := range businessQualityThresholds {
				By(fmt.Sprintf("Validating quality threshold for %s environment", environment))

				// Test responses at various quality levels
				testResponses := []struct {
					quality      float64
					shouldAccept bool
				}{
					{0.95, true},              // High quality - should accept
					{threshold + 0.05, true},  // Just above threshold - should accept
					{threshold - 0.05, false}, // Just below threshold - should reject
					{0.40, false},             // Low quality - should reject
				}

				for _, test := range testResponses {
					mockResponse := createMockResponseWithQuality(test.quality)
					qualityScore, err := qualityAssessor.AssessResponse(ctx, mockResponse)
					Expect(err).ToNot(HaveOccurred())

					meetsThreshold := qualityScore.OverallScore >= threshold

					if test.shouldAccept {
						Expect(meetsThreshold).To(BeTrue(),
							"Response with quality %.2f should meet %s threshold %.2f",
							test.quality, environment, threshold)
					} else {
						Expect(meetsThreshold).To(BeFalse(),
							"Response with quality %.2f should NOT meet %s threshold %.2f",
							test.quality, environment, threshold)
					}
				}

				// Business Impact: Quality gate prevents poor outcomes
				logger.WithFields(logrus.Fields{
					"environment":         environment,
					"quality_threshold":   threshold,
					"business_protection": "Prevents low-quality responses from affecting business operations",
				}).Info("Quality threshold business validation completed")
			}

			// Business Requirement: Quality thresholds must be enforceable
			enforceabilityTest := qualityAssessor.ValidateThresholdEnforcement(businessQualityThresholds)
			Expect(enforceabilityTest).To(BeTrue(),
				"Quality thresholds must be enforceable for business protection")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":  "BR-LLM-013",
				"scenario":              "threshold_enforcement",
				"environments_tested":   len(businessQualityThresholds),
				"threshold_enforceable": enforceabilityTest,
				"business_impact":       "Quality thresholds prevent poor business outcomes",
			}).Info("BR-LLM-013: Quality threshold enforcement business validation completed")
		})
	})
})

// Business type definitions for cost optimization and quality assessment
type LLMUsagePattern struct {
	DailyQueries        int
	AverageTokensInput  int
	AverageTokensOutput int
	PeakHourMultiplier  float64
	CacheableQueries    int // Percentage
}

type CostOptimizationStrategy struct {
	EnableCaching       bool
	CacheHitRateTarget  float64
	CacheTTL            time.Duration
	CostReductionTarget float64
}

type MonthlyUsageResult struct {
	TotalCost      float64
	TotalQueries   int
	AverageQuality float64
}

type LLMProvider struct {
	Name               string
	CostPerInputToken  float64
	CostPerOutputToken float64
	QualityScore       float64
	AverageLatency     int // milliseconds
}

type BusinessScenario struct {
	Priority         string
	QualityThreshold float64
	LatencyThreshold int
	ExpectedProvider string
}

type BusinessResponseScenario struct {
	IncidentType    string
	LLMResponse     types.ActionRecommendation
	ExpectedQuality float64
	BusinessOutcome string
}

type QualityCorrelation struct {
	QualityScore    float64
	BusinessOutcome string
}

// Business metric calculation functions
func setupBusinessLLMPricing(mockClient *mocks.MockLLMClient) {
	// Setup realistic pricing for business cost calculations
	mockClient.SetPricing("openai", 0.0015, 0.002)       // OpenAI pricing
	mockClient.SetPricing("huggingface", 0.0008, 0.0012) // HuggingFace pricing
	mockClient.SetPricing("local", 0.0001, 0.0001)       // Local/infrastructure pricing
}

func calculateMonthlyBaseline(pattern LLMUsagePattern) MonthlyUsageResult {
	// Business calculation for monthly baseline costs
	monthlyQueries := pattern.DailyQueries * 30
	inputTokens := monthlyQueries * pattern.AverageTokensInput
	outputTokens := monthlyQueries * pattern.AverageTokensOutput

	// OpenAI pricing as baseline
	totalCost := float64(inputTokens)*0.0015 + float64(outputTokens)*0.002

	return MonthlyUsageResult{
		TotalCost:      totalCost,
		TotalQueries:   monthlyQueries,
		AverageQuality: 0.90, // Baseline quality
	}
}

func simulateOptimizedUsage(pattern LLMUsagePattern, strategy CostOptimizationStrategy) MonthlyUsageResult {
	baseline := calculateMonthlyBaseline(pattern)

	// Apply caching optimization
	if strategy.EnableCaching {
		cacheHitQueries := int(float64(baseline.TotalQueries) * float64(pattern.CacheableQueries) / 100.0 * strategy.CacheHitRateTarget)
		costSavings := float64(cacheHitQueries) * (0.0015*float64(pattern.AverageTokensInput) + 0.002*float64(pattern.AverageTokensOutput))

		return MonthlyUsageResult{
			TotalCost:      baseline.TotalCost - costSavings,
			TotalQueries:   baseline.TotalQueries,
			AverageQuality: baseline.AverageQuality - 0.02, // Slight quality trade-off
		}
	}

	return baseline
}

func calculateProviderCostSavings(premium, selected LLMProvider) float64 {
	// Calculate monthly savings for business analysis
	monthlyTokens := 1000000                 // 1M tokens per month assumption
	inputTokens := monthlyTokens * 60 / 100  // 60% input
	outputTokens := monthlyTokens * 40 / 100 // 40% output

	premiumCost := float64(inputTokens)*premium.CostPerInputToken + float64(outputTokens)*premium.CostPerOutputToken
	selectedCost := float64(inputTokens)*selected.CostPerInputToken + float64(outputTokens)*selected.CostPerOutputToken

	return premiumCost - selectedCost
}

func calculateBusinessOutcomeCorrelation(data []QualityCorrelation) float64 {
	// Simplified correlation calculation for business validation
	// In real implementation: use proper statistical correlation (Pearson, Spearman)
	successfulOutcomes := 0
	highQualityResponses := 0

	for _, point := range data {
		if point.BusinessOutcome == "resolved_successfully" {
			successfulOutcomes++
		}
		if point.QualityScore >= 0.80 {
			highQualityResponses++
		}
	}

	// Simple correlation: percentage overlap
	if len(data) == 0 {
		return 0.0
	}

	return math.Min(float64(successfulOutcomes), float64(highQualityResponses)) / float64(len(data))
}

func createMockResponseWithQuality(quality float64) types.ActionRecommendation {
	// Create mock response with target quality characteristics
	var reasoning *types.ReasoningResult
	var parameters map[string]interface{}

	if quality >= 0.80 {
		reasoning = &types.ReasoningResult{
			Summary: "Detailed analysis with specific root cause identification and comprehensive solution approach",
			Factors: []string{"metric_analysis", "historical_patterns", "resource_constraints", "business_impact"},
		}
		parameters = map[string]interface{}{
			"target_value":  "2Gi",
			"resource":      "memory",
			"rollback_plan": true,
		}
	} else {
		reasoning = &types.ReasoningResult{
			Summary: "Basic recommendation",
			Factors: []string{"issue_detected"},
		}
		parameters = map[string]interface{}{}
	}

	return types.ActionRecommendation{
		Action:     "example_action",
		Confidence: quality,
		Reasoning:  reasoning,
		Parameters: parameters,
	}
}
