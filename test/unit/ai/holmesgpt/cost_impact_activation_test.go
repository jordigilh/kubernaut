package holmesgpt

import (
	"testing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// TDD Phase 1 Implementation: Cost Impact Analysis Function Activation
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-INS-007, BR-LLM-010, BR-COST-001 to BR-COST-010

var _ = Describe("Cost Impact Analysis Function Activation - TDD Phase 1", func() {
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

	Describe("BR-INS-007, BR-LLM-010: analyzeCostImpactFactors activation", func() {
		It("should analyze cost impact factors for resource-intensive alerts", func() {
			// Arrange: Create high-resource alert context
			alertContext := types.AlertContext{
				Name:        "HighMemoryUsage",
				Severity:    "critical",
				Description: "Memory usage exceeding 90% on production pods",
				Labels: map[string]string{
					"namespace":     "production",
					"resource_type": "memory",
					"impact_level":  "high",
					"cost_category": "compute",
				},
			}

			// Act: Analyze cost impact factors (THIS WILL FAIL until we activate it)
			costFactors := client.AnalyzeCostImpactFactors(alertContext)

			// Assert: Should return comprehensive cost analysis
			Expect(costFactors).ToNot(BeNil(), "BR-INS-007: Should return cost impact analysis")
			Expect(costFactors).To(HaveKey("resource_cost_per_minute"), "Should include resource cost rate")
			Expect(costFactors).To(HaveKey("business_impact_cost"), "Should include business impact estimation")
			Expect(costFactors).To(HaveKey("resolution_effort_cost"), "Should include resolution effort cost")

			// Validate cost factor values are realistic
			if resourceCost, exists := costFactors["resource_cost_per_minute"]; exists {
				Expect(resourceCost).To(BeNumerically(">", 0), "Resource cost should be positive")
				Expect(resourceCost).To(BeNumerically("<", 100), "Resource cost should be reasonable")
			}

			if businessCost, exists := costFactors["business_impact_cost"]; exists {
				Expect(businessCost).To(BeNumerically(">=", 0), "Business impact cost should be non-negative")
			}
		})

		It("should adapt cost analysis based on alert severity", func() {
			// Arrange: Create different severity contexts
			criticalContext := types.AlertContext{
				Name:        "ServiceDown",
				Severity:    "critical",
				Description: "Production service completely unavailable",
				Labels: map[string]string{
					"namespace":     "production",
					"impact_level":  "critical",
					"cost_category": "availability",
				},
			}

			warningContext := types.AlertContext{
				Name:        "SlowResponse",
				Severity:    "warning",
				Description: "API response time slightly elevated",
				Labels: map[string]string{
					"namespace":     "staging",
					"impact_level":  "low",
					"cost_category": "performance",
				},
			}

			// Act: Analyze cost factors for different severities
			criticalCosts := client.AnalyzeCostImpactFactors(criticalContext)
			warningCosts := client.AnalyzeCostImpactFactors(warningContext)

			// Assert: Critical alerts should have higher cost impact
			Expect(criticalCosts).ToNot(BeNil(), "Should analyze critical alert costs")
			Expect(warningCosts).ToNot(BeNil(), "Should analyze warning alert costs")

			// Business logic validation: Critical alerts should have higher business impact
			criticalBusinessCost := criticalCosts["business_impact_cost"].(float64)
			warningBusinessCost := warningCosts["business_impact_cost"].(float64)

			Expect(criticalBusinessCost).To(BeNumerically(">", warningBusinessCost),
				"BR-COST-001: Critical alerts should have higher business impact cost than warnings")
		})

		It("should integrate with existing cost optimization framework", func() {
			// Arrange: Create alert context that matches cost optimization patterns
			alertContext := types.AlertContext{
				Name:        "ResourceConstraintViolation",
				Severity:    "warning",
				Description: "Pod resource limits exceeded, triggering cost optimization",
				Labels: map[string]string{
					"namespace":         "production",
					"optimization_type": "cost_budget",
					"budget_category":   "compute",
				},
			}

			// Act: Analyze cost impact with optimization context
			costFactors := client.AnalyzeCostImpactFactors(alertContext)

			// Assert: Should provide cost factors compatible with optimization framework
			Expect(costFactors).To(HaveKey("resource_cost_per_minute"), "Should provide rate for cost calculator")
			Expect(costFactors).To(HaveKey("optimization_potential"), "Should identify optimization opportunities")

			// Validate integration compatibility
			if optimizationPotential, exists := costFactors["optimization_potential"]; exists {
				Expect(optimizationPotential).To(BeAssignableToTypeOf(0.0), "Optimization potential should be numeric")
				potential := optimizationPotential.(float64)
				Expect(potential).To(BeNumerically(">=", 0.0), "Optimization potential should be non-negative")
				Expect(potential).To(BeNumerically("<=", 1.0), "Optimization potential should be <= 100%")
			}
		})

		It("should handle edge cases gracefully", func() {
			// Arrange: Create edge case contexts
			emptyContext := types.AlertContext{
				Name:     "MinimalAlert",
				Severity: "info",
			}

			unknownContext := types.AlertContext{
				Name:        "UnknownIssue",
				Severity:    "unknown",
				Description: "Alert with unknown characteristics",
				Labels: map[string]string{
					"category": "unknown",
				},
			}

			// Act: Analyze cost factors for edge cases
			emptyCosts := client.AnalyzeCostImpactFactors(emptyContext)
			unknownCosts := client.AnalyzeCostImpactFactors(unknownContext)

			// Assert: Should handle edge cases without errors
			Expect(emptyCosts).ToNot(BeNil(), "Should handle minimal alert context")
			Expect(unknownCosts).ToNot(BeNil(), "Should handle unknown alert context")

			// Should provide default cost estimates
			Expect(emptyCosts).To(HaveKey("resource_cost_per_minute"), "Should provide default resource cost")
			Expect(unknownCosts).To(HaveKey("business_impact_cost"), "Should provide default business impact")
		})
	})

	Describe("Integration with existing cost optimization systems", func() {
		It("should provide cost factors compatible with AIDynamicCostCalculator", func() {
			// Arrange: Create alert context for cost calculator integration
			alertContext := types.AlertContext{
				Name:        "LLMProcessingCostAlert",
				Severity:    "warning",
				Description: "LLM processing costs exceeding budget thresholds",
				Labels: map[string]string{
					"cost_type":    "llm_processing",
					"provider":     "localai",
					"model_size":   "384",
					"budget_alert": "true",
				},
			}

			// Act: Get cost factors for integration
			costFactors := client.AnalyzeCostImpactFactors(alertContext)

			// Assert: Should provide factors that integrate with existing cost framework
			Expect(costFactors).To(HaveKey("resource_cost_per_minute"), "Should provide rate for cost calculations")

			// Validate compatibility with existing cost optimization
			resourceCost := costFactors["resource_cost_per_minute"].(float64)
			Expect(resourceCost).To(BeNumerically(">", 0), "Should provide positive cost rate")
			Expect(resourceCost).To(BeNumerically("<", 10), "Should be within reasonable LLM cost range")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUcostUimpactUactivation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UcostUimpactUactivation Suite")
}
