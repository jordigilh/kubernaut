package holmesgpt

import (
	"testing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// TDD Phase 1 Implementation: Success Rate Indicators Function Activation
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-INS-007, BR-AI-008, BR-AI-002

var _ = Describe("Success Rate Indicators Function Activation - TDD Phase 1", func() {
	var (
		client holmesgpt.Client
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise

		var err error
		client, err = holmesgpt.NewClient("http://test-endpoint", "test-key", logger)
		Expect(err).ToNot(HaveOccurred(), "Should create test client")
	})

	Describe("BR-INS-007: getSuccessRateIndicators activation", func() {
		It("should provide success rate indicators meeting >80% requirement", func() {
			// Arrange: Create alert context for success rate analysis
			alertContext := types.AlertContext{
				Name:        "DeploymentFailure",
				Severity:    "critical",
				Description: "Deployment rollout failed, need remediation strategy",
				Labels: map[string]string{
					"namespace":       "production",
					"resource_type":   "deployment",
					"failure_type":    "rollout",
					"strategy_needed": "true",
				},
			}

			// Act: Get success rate indicators (THIS WILL FAIL until we activate it)
			successRates := client.GetSuccessRateIndicators(alertContext)

			// Assert: Should return success rate indicators
			Expect(successRates).ToNot(BeNil(), "BR-INS-007: Should return success rate indicators")
			Expect(len(successRates)).To(BeNumerically(">=", 3), "Should provide multiple strategy options")

			// Validate BR-INS-007: Success rate >80% requirement
			Expect(successRates).To(HaveKey("rolling_deployment"), "Should include rolling deployment strategy")
			Expect(successRates).To(HaveKey("horizontal_scaling"), "Should include scaling strategy")

			// Business requirement validation: Success rates must be >80% for recommended strategies
			rollingRate := successRates["rolling_deployment"]
			Expect(rollingRate).To(BeNumerically(">", 0.8), "BR-INS-007: Rolling deployment success rate must be >80%")

			scalingRate := successRates["horizontal_scaling"]
			Expect(scalingRate).To(BeNumerically(">", 0.8), "BR-INS-007: Horizontal scaling success rate must be >80%")
		})

		It("should adapt success rates based on alert context and severity", func() {
			// Arrange: Create different alert contexts
			criticalContext := types.AlertContext{
				Name:        "ServiceOutage",
				Severity:    "critical",
				Description: "Complete service outage requiring immediate action",
				Labels: map[string]string{
					"namespace":    "production",
					"impact_level": "critical",
					"urgency":      "immediate",
				},
			}

			routineContext := types.AlertContext{
				Name:        "MinorPerformanceDegradation",
				Severity:    "warning",
				Description: "Slight performance degradation observed",
				Labels: map[string]string{
					"namespace":    "staging",
					"impact_level": "low",
					"urgency":      "routine",
				},
			}

			// Act: Get success rates for different contexts
			criticalRates := client.GetSuccessRateIndicators(criticalContext)
			routineRates := client.GetSuccessRateIndicators(routineContext)

			// Assert: Should provide context-appropriate success rates
			Expect(criticalRates).ToNot(BeNil(), "Should provide rates for critical alerts")
			Expect(routineRates).ToNot(BeNil(), "Should provide rates for routine alerts")

			// Business logic: Critical contexts should have more conservative (higher success rate) strategies
			if criticalRolling, exists := criticalRates["rolling_deployment"]; exists {
				if routineRolling, exists := routineRates["rolling_deployment"]; exists {
					Expect(criticalRolling).To(BeNumerically(">=", routineRolling),
						"Critical contexts should prefer higher success rate strategies")
				}
			}
		})

		It("should integrate with existing effectiveness assessment framework", func() {
			// Arrange: Create alert context that matches effectiveness patterns
			alertContext := types.AlertContext{
				Name:        "RecurringIssue",
				Severity:    "warning",
				Description: "Issue that has occurred multiple times with known patterns",
				Labels: map[string]string{
					"namespace":             "production",
					"pattern_type":          "recurring",
					"historical_data":       "available",
					"effectiveness_tracked": "true",
				},
			}

			// Act: Get success rate indicators with historical context
			successRates := client.GetSuccessRateIndicators(alertContext)

			// Assert: Should provide rates that align with effectiveness framework
			Expect(successRates).ToNot(BeNil(), "Should provide success rates")

			// Validate integration with effectiveness assessment
			for strategy, rate := range successRates {
				Expect(rate).To(BeAssignableToTypeOf(0.0), "Success rate should be numeric for strategy: %s", strategy)
				Expect(rate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative for strategy: %s", strategy)
				Expect(rate).To(BeNumerically("<=", 1.0), "Success rate should be <= 100% for strategy: %s", strategy)
			}

			// Should include strategies that align with effectiveness tracking
			Expect(successRates).To(HaveKey("resource_adjustment"), "Should include resource adjustment strategy")
		})

		It("should provide success rates for all major remediation strategy types", func() {
			// Arrange: Create comprehensive alert context
			alertContext := types.AlertContext{
				Name:        "ComprehensiveResourceIssue",
				Severity:    "warning",
				Description: "Multi-faceted resource issue requiring strategy selection",
				Labels: map[string]string{
					"namespace":        "production",
					"issue_type":       "resource",
					"complexity":       "multi_faceted",
					"strategy_options": "multiple",
				},
			}

			// Act: Get comprehensive success rate indicators
			successRates := client.GetSuccessRateIndicators(alertContext)

			// Assert: Should provide rates for major strategy categories
			Expect(successRates).ToNot(BeNil(), "Should provide success rate indicators")

			// Validate coverage of major strategy types
			expectedStrategies := []string{
				"rolling_deployment",
				"horizontal_scaling",
				"resource_adjustment",
			}

			for _, strategy := range expectedStrategies {
				Expect(successRates).To(HaveKey(strategy), "Should include success rate for %s", strategy)
				rate := successRates[strategy]
				Expect(rate).To(BeNumerically(">=", 0.5), "Strategy %s should have reasonable success rate", strategy)
			}
		})

		It("should handle edge cases and provide fallback rates", func() {
			// Arrange: Create edge case contexts
			unknownContext := types.AlertContext{
				Name:        "UnknownAlert",
				Severity:    "unknown",
				Description: "Alert with unknown characteristics",
			}

			minimalContext := types.AlertContext{
				Name:     "MinimalAlert",
				Severity: "info",
			}

			// Act: Get success rates for edge cases
			unknownRates := client.GetSuccessRateIndicators(unknownContext)
			minimalRates := client.GetSuccessRateIndicators(minimalContext)

			// Assert: Should provide fallback rates for edge cases
			Expect(unknownRates).ToNot(BeNil(), "Should handle unknown alert context")
			Expect(minimalRates).ToNot(BeNil(), "Should handle minimal alert context")

			// Should provide at least basic strategy options
			Expect(len(unknownRates)).To(BeNumerically(">=", 1), "Should provide at least one fallback strategy")
			Expect(len(minimalRates)).To(BeNumerically(">=", 1), "Should provide at least one fallback strategy")

			// Fallback rates should be conservative but reasonable
			for _, rate := range unknownRates {
				Expect(rate).To(BeNumerically(">=", 0.3), "Fallback rates should be reasonable")
				Expect(rate).To(BeNumerically("<=", 0.95), "Fallback rates should be conservative")
			}
		})
	})

	Describe("Integration with pattern analytics and historical data", func() {
		It("should provide success rates that align with historical pattern analysis", func() {
			// Arrange: Create alert context with historical significance
			alertContext := types.AlertContext{
				Name:        "HistoricallyTrackedIssue",
				Severity:    "warning",
				Description: "Issue type with extensive historical tracking data",
				Labels: map[string]string{
					"namespace":           "production",
					"historical_tracking": "extensive",
					"pattern_confidence":  "high",
					"analytics_available": "true",
				},
			}

			// Act: Get success rates with historical context
			successRates := client.GetSuccessRateIndicators(alertContext)

			// Assert: Should provide rates that reflect historical analysis
			Expect(successRates).ToNot(BeNil(), "Should provide historically-informed success rates")

			// Validate alignment with historical effectiveness patterns
			// High-confidence historical data should result in more precise success rates
			rateVariance := calculateSuccessRateVariance(successRates)
			Expect(rateVariance).To(BeNumerically("<", 0.3),
				"Historical data should reduce success rate variance (more precise predictions)")
		})
	})
})

// Helper function for test validation
func calculateSuccessRateVariance(rates map[string]float64) float64 {
	if len(rates) == 0 {
		return 0.0
	}

	// Calculate mean
	sum := 0.0
	for _, rate := range rates {
		sum += rate
	}
	mean := sum / float64(len(rates))

	// Calculate variance
	varianceSum := 0.0
	for _, rate := range rates {
		diff := rate - mean
		varianceSum += diff * diff
	}

	return varianceSum / float64(len(rates))
}

// TestRunner bootstraps the Ginkgo test suite
func TestUsuccessUrateUactivation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UsuccessUrateUactivation Suite")
}
