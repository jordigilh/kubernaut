<<<<<<< HEAD
package holmesgpt

import (
	"testing"
	"context"
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

package holmesgpt

import (
	"context"
	"testing"
>>>>>>> crd_implementation

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// TDD Phase 2 Implementation: Strategy-Oriented Investigation Function Activation
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-INS-007 Strategy optimization

var _ = Describe("Strategy-Oriented Investigation Function Activation - TDD Phase 2", func() {
	var (
		client holmesgpt.Client
		logger *logrus.Logger
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise

		// Use mock client for unit testing
		client = mocks.NewMockClient()
	})

	Describe("BR-INS-007: generateStrategyOrientedInvestigation activation", func() {
		It("should generate HolmesGPT investigation prompt focused on strategy optimization", func() {
			// Arrange: Create alert context for strategy investigation
			alertContext := types.AlertContext{
				Name:        "PodCrashLooping",
				Severity:    "critical",
				Description: "Pod has been crash looping due to OOM errors for 15 minutes",
				Labels: map[string]string{
					"namespace":           "production",
					"deployment":          "user-service",
					"crash_reason":        "OOMKilled",
					"resource_constraint": "memory",
					"restart_count":       "12",
				},
			}

			// Act: Generate strategy-oriented investigation using available Investigate method
			investigateReq := &holmesgpt.InvestigateRequest{
				AlertName:       alertContext.Name,
				Labels:          alertContext.Labels,
				Annotations:     alertContext.Annotations,
				Priority:        alertContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			investigation, err := client.Investigate(ctx, investigateReq)

			// Assert: Should return comprehensive investigation response
			Expect(err).ToNot(HaveOccurred(), "Should successfully generate investigation")
			Expect(investigation).ToNot(BeNil(), "Should generate investigation response")
			Expect(investigation.AlertName).To(Equal("PodCrashLooping"), "Should include alert name")
			Expect(investigation.Status).To(Equal("completed"), "Should have completed status")
			Expect(investigation.Summary).ToNot(BeEmpty(), "Should include investigation summary")
			Expect(investigation.ContextUsed).To(HaveKey("br_ins_007_compliance"), "Should include strategy optimization context")

			// Should focus on strategy optimization
			Expect(investigation.StrategyInsights).ToNot(BeNil(), "Should include strategy insights")
			Expect(investigation.Recommendations).ToNot(BeEmpty(), "Should provide recommendations")
			Expect(investigation.StrategyInsights.HistoricalSuccessRate).To(BeNumerically(">", 0), "Should provide success rate")

			// Should include specific context for OOM issues
			Expect(investigation.Summary).To(ContainSubstring("memory"), "Should mention memory context")
			Expect(investigation.RootCause).ToNot(BeEmpty(), "Should provide root cause analysis")
		})

		It("should adapt investigation based on alert severity and context", func() {
			// Arrange: Create different severity contexts
			criticalContext := types.AlertContext{
				Name:        "ServiceDown",
				Severity:    "critical",
				Description: "Production service completely unavailable",
				Labels: map[string]string{
					"namespace":    "production",
					"service":      "payment-api",
					"impact_level": "high",
					"urgency":      "immediate",
				},
			}

			warningContext := types.AlertContext{
				Name:        "HighLatency",
				Severity:    "warning",
				Description: "API response times elevated above threshold",
				Labels: map[string]string{
					"namespace":    "staging",
					"service":      "user-api",
					"impact_level": "medium",
					"urgency":      "routine",
				},
			}

			// Act: Generate investigations for different severities
			criticalReq := &holmesgpt.InvestigateRequest{
				AlertName:       criticalContext.Name,
				Labels:          criticalContext.Labels,
				Annotations:     criticalContext.Annotations,
				Priority:        criticalContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			criticalInvestigation, _ := client.Investigate(ctx, criticalReq)

			warningReq := &holmesgpt.InvestigateRequest{
				AlertName:       warningContext.Name,
				Labels:          warningContext.Labels,
				Annotations:     warningContext.Annotations,
				Priority:        warningContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			warningInvestigation, _ := client.Investigate(ctx, warningReq)

			// Assert: Should adapt content based on severity
			Expect(criticalInvestigation).ToNot(BeNil(), "Critical investigation should be generated")
			Expect(criticalInvestigation.AlertName).To(Equal("ServiceDown"), "Should include alert name")
			Expect(criticalInvestigation.StrategyInsights).ToNot(BeNil(), "Should include strategy insights")

			Expect(warningInvestigation).ToNot(BeNil(), "Warning investigation should be generated")
			Expect(warningInvestigation.AlertName).To(Equal("HighLatency"), "Should include alert name")
			Expect(warningInvestigation.StrategyInsights).ToNot(BeNil(), "Should include strategy insights")

			// Critical investigations should be more detailed than warnings
			Expect(len(criticalInvestigation.Recommendations)).To(BeNumerically(">=", len(warningInvestigation.Recommendations)),
				"Critical investigations should have at least as many recommendations")
		})

		It("should include historical pattern context in investigation", func() {
			// Arrange: Create alert context with historical significance
			alertContext := types.AlertContext{
				Name:        "RecurringDeploymentFailure",
				Severity:    "warning",
				Description: "Deployment failure pattern observed multiple times",
				Labels: map[string]string{
					"namespace":           "production",
					"deployment":          "web-app",
					"pattern_type":        "recurring",
					"historical_tracking": "extensive",
					"failure_count":       "5",
					"pattern_confidence":  "high",
				},
			}

			// Act: Generate investigation with historical context
			investigateReq := &holmesgpt.InvestigateRequest{
				AlertName:       alertContext.Name,
				Labels:          alertContext.Labels,
				Annotations:     alertContext.Annotations,
				Priority:        alertContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			investigation, err := client.Investigate(ctx, investigateReq)

			// Assert: Should include historical pattern analysis
			Expect(err).ToNot(HaveOccurred(), "Should successfully generate investigation")
			Expect(investigation).ToNot(BeNil(), "Should generate investigation response")
			Expect(investigation.AlertName).To(Equal("RecurringDeploymentFailure"), "Should include alert name")
			Expect(investigation.StrategyInsights).ToNot(BeNil(), "Should include strategy insights")
			Expect(investigation.StrategyInsights.HistoricalSuccessRate).To(BeNumerically(">", 0), "Should provide historical success rate")
			Expect(investigation.Summary).ToNot(BeEmpty(), "Should provide investigation summary")

			// Should suggest pattern-based strategies
			Expect(investigation.Recommendations).ToNot(BeEmpty(), "Should provide recommendations")
			Expect(investigation.RootCause).ToNot(BeEmpty(), "Should provide root cause analysis")
		})

		It("should integrate cost and success rate context from Phase 1 functions", func() {
			// Arrange: Create alert context for integrated investigation
			alertContext := types.AlertContext{
				Name:        "ResourceConstraintViolation",
				Severity:    "warning",
				Description: "Resource limits exceeded, cost optimization needed",
				Labels: map[string]string{
					"namespace":         "production",
					"resource_type":     "memory",
					"cost_category":     "compute",
					"optimization_type": "cost_budget",
				},
			}

			// Get context from Phase 1 functions
			costFactors := client.AnalyzeCostImpactFactors(alertContext)
			successRates := client.GetSuccessRateIndicators(alertContext)

			// Act: Generate investigation that should integrate this context
			investigateReq := &holmesgpt.InvestigateRequest{
				AlertName:       alertContext.Name,
				Labels:          alertContext.Labels,
				Annotations:     alertContext.Annotations,
				Priority:        alertContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			investigation, err := client.Investigate(ctx, investigateReq)

			// Assert: Should reference cost and success rate context
			Expect(err).ToNot(HaveOccurred(), "Should successfully generate investigation")
			Expect(investigation).ToNot(BeNil(), "Should generate investigation response")
			Expect(investigation.AlertName).To(Equal("ResourceConstraintViolation"), "Should include alert name")
			Expect(investigation.StrategyInsights).ToNot(BeNil(), "Should include strategy insights")
			Expect(investigation.StrategyInsights.EstimatedROI).To(BeNumerically(">", 0), "Should provide ROI estimates")

			// Should suggest strategies that align with high success rates
			if rollingRate, exists := successRates["rolling_deployment"]; exists && rollingRate > 0.8 {
				Expect(investigation.Summary).To(ContainSubstring("deployment"), "Should suggest deployment strategies")
			}

			// Should consider cost optimization potential
			if optimizationPotential := costFactors["optimization_potential"]; optimizationPotential != nil {
				if potential := optimizationPotential.(float64); potential > 0.5 {
					Expect(investigation.StrategyInsights.BusinessImpact).To(ContainSubstring("optimization"), "Should highlight optimization potential")
				}
			}
		})

		It("should provide actionable investigation prompts for HolmesGPT", func() {
			// Arrange: Create alert context requiring actionable investigation
			alertContext := types.AlertContext{
				Name:        "DatabaseConnectionPoolExhausted",
				Severity:    "critical",
				Description: "Database connection pool has reached maximum capacity",
				Labels: map[string]string{
					"namespace":          "production",
					"database":           "postgresql",
					"pool_size":          "100",
					"active_connections": "98",
					"queue_length":       "25",
				},
			}

			// Act: Generate actionable investigation
			investigateReq := &holmesgpt.InvestigateRequest{
				AlertName:       alertContext.Name,
				Labels:          alertContext.Labels,
				Annotations:     alertContext.Annotations,
				Priority:        alertContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			investigation, err := client.Investigate(ctx, investigateReq)

			// Assert: Should provide specific, actionable investigation points
			Expect(err).ToNot(HaveOccurred(), "Should successfully generate investigation")
			Expect(investigation).ToNot(BeNil(), "Should generate investigation response")
			Expect(investigation.AlertName).To(Equal("DatabaseConnectionPoolExhausted"), "Should include alert name")
			Expect(investigation.Recommendations).ToNot(BeEmpty(), "Should provide recommendations")

			// Should include specific technical context
			Expect(investigation.Summary).To(ContainSubstring("database"), "Should include database context")
			Expect(investigation.RootCause).ToNot(BeEmpty(), "Should provide root cause analysis")
			Expect(investigation.StrategyInsights).ToNot(BeNil(), "Should include strategy insights")

			// Should request specific remediation strategies
			Expect(investigation.StrategyInsights.RecommendedStrategies).ToNot(BeEmpty(), "Should provide strategy recommendations")
			Expect(investigation.StrategyInsights.BusinessImpact).ToNot(BeEmpty(), "Should include business impact")
			Expect(investigation.StrategyInsights.TimeToResolution).To(BeNumerically(">", 0), "Should estimate resolution time")
		})

		It("should handle edge cases and provide fallback investigations", func() {
			// Arrange: Create edge case contexts
			minimalContext := types.AlertContext{
				Name:     "MinimalAlert",
				Severity: "info",
			}

			unknownContext := types.AlertContext{
				Name:        "UnknownIssue",
				Severity:    "unknown",
				Description: "Alert with unknown characteristics",
			}

			// Act: Generate investigations for edge cases
			minimalReq := &holmesgpt.InvestigateRequest{
				AlertName:       minimalContext.Name,
				Labels:          minimalContext.Labels,
				Annotations:     minimalContext.Annotations,
				Priority:        minimalContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			minimalInvestigation, err1 := client.Investigate(ctx, minimalReq)

			unknownReq := &holmesgpt.InvestigateRequest{
				AlertName:       unknownContext.Name,
				Labels:          unknownContext.Labels,
				Annotations:     unknownContext.Annotations,
				Priority:        unknownContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			unknownInvestigation, err2 := client.Investigate(ctx, unknownReq)

			// Assert: Should provide meaningful fallback investigations
			Expect(err1).ToNot(HaveOccurred(), "Should successfully generate minimal investigation")
			Expect(minimalInvestigation).ToNot(BeNil(), "Should provide investigation for minimal context")
			Expect(minimalInvestigation.AlertName).To(Equal("MinimalAlert"), "Should include alert name")
			Expect(minimalInvestigation.Status).To(Equal("completed"), "Should have completed status")

			Expect(err2).ToNot(HaveOccurred(), "Should successfully generate unknown investigation")
			Expect(unknownInvestigation).ToNot(BeNil(), "Should provide investigation for unknown context")
			Expect(unknownInvestigation.AlertName).To(Equal("UnknownIssue"), "Should include alert name")
			Expect(unknownInvestigation.StrategyInsights).ToNot(BeNil(), "Should provide strategy insights even for unknown issues")
		})
	})

	Describe("Integration with HolmesGPT service patterns", func() {
		It("should generate investigation prompts compatible with HolmesGPT API format", func() {
			// Arrange: Create alert context for API compatibility testing
			alertContext := types.AlertContext{
				Name:        "APICompatibilityTest",
				Severity:    "warning",
				Description: "Test alert for HolmesGPT API compatibility",
				Labels: map[string]string{
					"namespace": "test",
					"service":   "api-test",
				},
			}

			// Act: Generate investigation
			investigateReq := &holmesgpt.InvestigateRequest{
				AlertName:       alertContext.Name,
				Labels:          alertContext.Labels,
				Annotations:     alertContext.Annotations,
				Priority:        alertContext.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}
			investigation, err := client.Investigate(ctx, investigateReq)

			// Assert: Should be compatible with HolmesGPT investigation format
			Expect(err).ToNot(HaveOccurred(), "Should successfully generate investigation")
			Expect(investigation).ToNot(BeNil(), "Should generate investigation response")
			Expect(investigation.AlertName).To(Equal("APICompatibilityTest"), "Should include alert name")
			Expect(investigation.InvestigationID).ToNot(BeEmpty(), "Should provide investigation ID")

			// Should be structured for AI consumption
			Expect(investigation.Summary).ToNot(BeEmpty(), "Should include analysis summary")
			Expect(investigation.StrategyInsights).ToNot(BeNil(), "Should focus on strategy")
			Expect(investigation.Status).To(Equal("completed"), "Should have completed status")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUstrategyUinvestigationUactivation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UstrategyUinvestigationUactivation Suite")
}
