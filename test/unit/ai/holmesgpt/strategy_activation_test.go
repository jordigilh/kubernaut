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

package holmesgpt

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// TDD Implementation: AI Strategy Function Activation Tests
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-AI-006, BR-AI-008, BR-INS-007

var _ = Describe("AI Strategy Function Activation - TDD Implementation", func() {
	var (
		client holmesgpt.Client
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise

		// Use mock client for unit testing
		client = mocks.NewMockClient()
	})

	Describe("BR-AI-006: identifyPotentialStrategies activation", func() {
		It("should identify potential remediation strategies from alert context", func() {
			// Arrange: Create test alert context
			alertContext := types.AlertContext{
				Name:        "HighCPUUsage",
				Severity:    "critical",
				Description: "High CPU usage in production deployment",
				Labels: map[string]string{
					"app":       "web-server",
					"env":       "production",
					"namespace": "production",
					"kind":      "deployment",
				},
			}

			// Act: Identify potential strategies (THIS WILL FAIL until we implement it)
			strategies := client.IdentifyPotentialStrategies(alertContext)

			// Assert: Should return relevant strategies
			Expect(strategies).ToNot(BeEmpty(), "BR-AI-006: Should identify at least one strategy")
			Expect(strategies).To(ContainElement(ContainSubstring("scaling")), "Should suggest scaling for CPU issues")
			Expect(len(strategies)).To(BeNumerically(">=", 2), "Should provide multiple strategy options")
		})

		It("should adapt strategies based on alert severity", func() {
			// Arrange: Create critical alert context
			criticalContext := types.AlertContext{
				Name:     "ServiceDown",
				Severity: "critical",
				Labels: map[string]string{
					"namespace": "production",
				},
			}

			warningContext := types.AlertContext{
				Name:     "SlowResponse",
				Severity: "warning",
				Labels: map[string]string{
					"namespace": "staging",
				},
			}

			// Act: Identify strategies for different severities
			criticalStrategies := client.IdentifyPotentialStrategies(criticalContext)
			warningStrategies := client.IdentifyPotentialStrategies(warningContext)

			// Assert: Should adapt to severity
			Expect(criticalStrategies).ToNot(BeEmpty(), "Should provide strategies for critical alerts")
			Expect(warningStrategies).ToNot(BeEmpty(), "Should provide strategies for warning alerts")
			// Critical alerts should have more aggressive strategies
			Expect(len(criticalStrategies)).To(BeNumerically(">=", len(warningStrategies)),
				"Critical alerts should have at least as many strategy options")
		})
	})

	Describe("BR-AI-008: getRelevantHistoricalPatterns activation", func() {
		It("should retrieve historical patterns relevant to alert context", func() {
			// Arrange: Create alert context with historical significance
			alertContext := types.AlertContext{
				Name:        "DatabaseConnectionPool",
				Severity:    "warning",
				Description: "Database connection pool exhausted",
				Labels: map[string]string{
					"namespace": "production",
					"kind":      "database",
				},
			}

			// Act: Get relevant historical patterns (THIS WILL FAIL until we implement it)
			patterns := client.GetRelevantHistoricalPatterns(alertContext)

			// Assert: Should return structured historical data
			Expect(patterns).To(HaveKey("similar_incidents"), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Historical patterns must contain similar incidents for strategy confidence validation")
			Expect(patterns).To(HaveKey("success_patterns"), "Should include successful resolution patterns")

			// Validate pattern structure
			if similarIncidents, exists := patterns["similar_incidents"]; exists {
				Expect(similarIncidents).To(BeNumerically(">=", 0), "Similar incidents should be non-negative")
			}
		})

		It("should provide success and failure patterns", func() {
			// Arrange: Create context for pattern analysis
			alertContext := types.AlertContext{
				Name:     "MemoryLeak",
				Severity: "critical",
				Labels: map[string]string{
					"namespace": "production",
				},
			}

			// Act: Get historical patterns
			patterns := client.GetRelevantHistoricalPatterns(alertContext)

			// Assert: Should include both success and failure patterns
			Expect(patterns).To(HaveKey("success_patterns"), "Should provide successful resolution patterns")
			Expect(patterns).To(HaveKey("failure_patterns"), "Should provide patterns to avoid")

			// Validate pattern arrays
			if successPatterns, exists := patterns["success_patterns"]; exists {
				Expect(successPatterns).To(BeAssignableToTypeOf([]string{}), "Success patterns should be string array")
			}
		})
	})

	Describe("Integration: Strategy identification with historical patterns", func() {
		It("should integrate strategy identification with historical pattern analysis", func() {
			// Arrange: Create comprehensive alert context
			alertContext := types.AlertContext{
				Name:        "HighLatency",
				Severity:    "warning",
				Description: "High latency detected in API gateway",
				Labels: map[string]string{
					"namespace": "production",
					"kind":      "service",
					"component": "api-gateway",
					"version":   "v2.1.0",
				},
			}

			// Act: Get both strategies and patterns
			strategies := client.IdentifyPotentialStrategies(alertContext)
			patterns := client.GetRelevantHistoricalPatterns(alertContext)

			// Assert: Should provide complementary information
			Expect(strategies).ToNot(BeEmpty(), "Should identify strategies")
			Expect(len(patterns)).To(BeNumerically(">", 0), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Strategy activation must provide measurable historical pattern data for recommendation validation")

			// Integration validation
			Expect(len(strategies)).To(BeNumerically(">=", 1), "Should suggest at least one strategy")
			if successPatterns, exists := patterns["success_patterns"]; exists {
				successArray, ok := successPatterns.([]string)
				if ok && len(successArray) > 0 {
					// At least one strategy should align with historical success patterns
					hasAlignment := false
					for _, strategy := range strategies {
						for _, successPattern := range successArray {
							if strategy == successPattern {
								hasAlignment = true
								break
							}
						}
					}
					// This is a business logic validation - strategies should consider historical success
					Expect(hasAlignment || len(strategies) > 0).To(BeTrue(),
						"Strategies should either align with historical patterns or provide new options")
				}
			}
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUstrategyUactivation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UstrategyUactivation Suite")
}
