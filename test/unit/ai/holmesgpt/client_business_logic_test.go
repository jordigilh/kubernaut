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
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("HolmesGPT Client - Business Requirements Validation", func() {
	var (
		client holmesgpt.Client
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		client, err = holmesgpt.NewClient("http://test-holmesgpt:8090", "", nil)
		Expect(err).ToNot(HaveOccurred())
	})

	// BR-INS-007: Optimal Remediation Strategy Insights - Context-aware Processing
	Describe("BR-INS-007: Dynamic Alert Context Processing", func() {
		Context("when processing different alert severities", func() {
			It("should adapt strategy recommendations based on critical alerts", func() {
				// BR-TYPE-001: Use standardized alert data structures
				criticalAlert := &holmesgpt.InvestigateRequest{
					AlertName:       "MemoryLeakDetected",
					Namespace:       "production",
					Labels:          map[string]string{"team": "payments", "env": "prod", "service": "payment-service"},
					Annotations:     map[string]string{"runbook_url": "https://wiki.company.com/payment-runbook"},
					Priority:        "critical",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				response, err := client.Investigate(ctx, criticalAlert)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StrategyInsights.ConfidenceLevel).To(BeNumerically(">=", 0.8), "BR-AI-001-CONFIDENCE: HolmesGPT investigation must return high confidence scores for reliable AI decision making")

				// BR-INS-007: Strategy recommendations must be contextually relevant
				Expect(len(response.StrategyInsights.RecommendedStrategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Strategy insights must provide measurable strategy recommendations for business decision making")
				Expect(len(response.StrategyInsights.RecommendedStrategies)).To(BeNumerically(">=", 1))

				// Business requirement: Critical alerts should prioritize fast, reliable strategies
				criticalStrategies := response.StrategyInsights.RecommendedStrategies
				for _, strategy := range criticalStrategies {
					By(fmt.Sprintf("Validating strategy: %s for critical alert", strategy.StrategyName))
					// BR-INS-007: >80% success rate requirement for critical production services
					Expect(strategy.ExpectedSuccessRate).To(BeNumerically(">=", 0.80),
						"Critical alerts must have strategies with >80% success rate")
					// Business requirement: Critical alerts should resolve quickly
					Expect(strategy.TimeToResolve).To(BeNumerically("<=", 30*time.Minute),
						"Critical alert strategies should resolve within 30 minutes")
				}
			})

			It("should provide different strategies for warning vs critical alerts", func() {
				// BR-TYPE-001: Use standardized alert data structures
				warningAlert := &holmesgpt.InvestigateRequest{
					AlertName:       "HighCPUUsage",
					Namespace:       "development",
					Labels:          map[string]string{"service": "test-service", "env": "dev"},
					Annotations:     map[string]string{},
					Priority:        "warning",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				criticalAlert := &holmesgpt.InvestigateRequest{
					AlertName:       "ServiceDown",
					Namespace:       "production",
					Labels:          map[string]string{"service": "payment-service", "env": "prod"},
					Annotations:     map[string]string{},
					Priority:        "critical",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				warningResponse, err := client.Investigate(ctx, warningAlert)
				Expect(err).ToNot(HaveOccurred())

				criticalResponse, err := client.Investigate(ctx, criticalAlert)
				Expect(err).ToNot(HaveOccurred())

				// BR-INS-007: Strategy selection should differ based on alert context
				warningStrategies := warningResponse.StrategyInsights.RecommendedStrategies
				criticalStrategies := criticalResponse.StrategyInsights.RecommendedStrategies

				// Business validation: Strategies should be contextually different
				Expect(len(warningStrategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Warning alert strategies must be provided for recommendation confidence")
				Expect(len(criticalStrategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Critical alert strategies must be provided for recommendation confidence")

				// Business requirement: Critical alerts should have more aggressive strategies
				if len(criticalStrategies) > 0 && len(warningStrategies) > 0 {
					criticalROI := criticalStrategies[0].ROI
					warningROI := warningStrategies[0].ROI

					By("Critical alerts should justify higher-cost, higher-ROI strategies")
					// This validates dynamic processing vs static returns
					Expect(criticalROI).ToNot(Equal(warningROI),
						"ROI calculations must be context-specific, not static")
				}
			})
		})

		Context("when processing service-specific alerts", func() {
			It("should incorporate service context into strategy selection", func() {
				// BR-TYPE-001: Use standardized alert data structures
				paymentServiceAlert := &holmesgpt.InvestigateRequest{
					AlertName:       "PaymentProcessingDown",
					Namespace:       "production",
					Labels:          map[string]string{"business_critical": "true", "revenue_impact": "high", "service": "payment-service"},
					Annotations:     map[string]string{},
					Priority:        "critical",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				response, err := client.Investigate(ctx, paymentServiceAlert)
				Expect(err).ToNot(HaveOccurred())

				// BR-INS-007: Business impact assessment must reflect actual service criticality
				Expect(response.StrategyInsights.BusinessImpact).To(ContainSubstring("payment"),
					"Business impact assessment must reference the specific service context")

				// Business requirement: High-revenue services should have premium strategies
				strategies := response.StrategyInsights.RecommendedStrategies
				Expect(len(strategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: High-revenue services must receive strategy recommendations")

				hasHighSuccessRateStrategy := false
				for _, strategy := range strategies {
					if strategy.ExpectedSuccessRate > 0.90 {
						hasHighSuccessRateStrategy = true
						break
					}
				}
				Expect(hasHighSuccessRateStrategy).To(BeTrue(),
					"Payment service alerts should include >90% success rate strategies")
			})
		})
	})

	// BR-HAPI-002: Accept Alert Context (name, namespace, labels, annotations)
	Describe("BR-HAPI-002: Alert Context Extraction and Processing", func() {
		Context("when processing alerts with comprehensive metadata", func() {
			It("should extract and utilize all alert context components", func() {
				// BR-TYPE-001: Use standardized alert data structures
				comprehensiveAlert := &holmesgpt.InvestigateRequest{
					AlertName: "DatabaseConnectionPool",
					Namespace: "backend",
					Labels: map[string]string{
						"cluster":     "production-east",
						"team":        "backend",
						"environment": "production",
						"component":   "database",
						"service":     "user-service",
					},
					Annotations: map[string]string{
						"description": "Database connection pool exhausted",
						"runbook_url": "https://wiki.company.com/db-runbook",
						"dashboard":   "https://grafana.company.com/db-dashboard",
						"summary":     "Connection pool at 95% capacity",
					},
					Priority:        "warning",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				response, err := client.Investigate(ctx, comprehensiveAlert)
				Expect(err).ToNot(HaveOccurred())

				// BR-HAPI-002: Alert context must be processed, not ignored
				investigation := response.Summary
				Expect(investigation).To(ContainSubstring("DatabaseConnectionPool"),
					"Investigation must reference the specific alert name")
				Expect(investigation).To(ContainSubstring("backend"),
					"Investigation must reference the namespace context")
				Expect(investigation).To(ContainSubstring("user-service"),
					"Investigation must reference the service context")

				// Business requirement: Context should influence strategy selection
				strategies := response.StrategyInsights.RecommendedStrategies
				Expect(len(strategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Context-aware strategies must be provided for AI recommendation confidence")

				// Database-related alerts should have database-appropriate strategies
				hasDbRelatedStrategy := false
				for _, strategy := range strategies {
					if strategy.BusinessJustification != "" {
						justification := strategy.BusinessJustification
						if strings.Contains(justification, "database") ||
							strings.Contains(justification, "connection") ||
							strings.Contains(justification, "pool") {
							hasDbRelatedStrategy = true
							break
						}
					}
				}
				Expect(hasDbRelatedStrategy).To(BeTrue(),
					"Database alerts should generate database-specific strategies")
			})

			It("should handle missing alert context gracefully with validation", func() {
				// BR-TYPE-001: Use standardized alert data structures
				minimalAlert := &holmesgpt.InvestigateRequest{
					AlertName:       "GenericAlert",
					Namespace:       "",
					Labels:          map[string]string{},
					Annotations:     map[string]string{},
					Priority:        "",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				response, err := client.Investigate(ctx, minimalAlert)
				Expect(err).ToNot(HaveOccurred())

				// BR-HAPI-013: Context validation should identify missing information
				Expect(response.Summary).To(ContainSubstring("GenericAlert"))

				// Business requirement: Limited context should result in conservative strategies
				strategies := response.StrategyInsights.RecommendedStrategies
				if len(strategies) > 0 {
					// Conservative strategies should have moderate success rates and costs
					for _, strategy := range strategies {
						Expect(strategy.ExpectedSuccessRate).To(BeNumerically(">=", 0.75),
							"Conservative strategies should still meet minimum success rate")
						Expect(strategy.EstimatedCost).To(BeNumerically("<=", 500),
							"Limited context should result in lower-cost strategies")
					}
				}
			})
		})
	})

	// BR-INS-007: ROI Analysis and Cost-Effectiveness
	Describe("BR-INS-007: Dynamic ROI Calculation", func() {
		Context("when analyzing strategies for different business contexts", func() {
			It("should calculate ROI based on actual business impact factors", func() {
				// BR-TYPE-001: Use standardized alert data structures
				highImpactAlert := &holmesgpt.InvestigateRequest{
					AlertName: "EcommerceCheckoutDown",
					Namespace: "ecommerce",
					Labels: map[string]string{
						"revenue_impact": "high",
						"user_facing":    "true",
						"business_tier":  "critical",
						"service":        "checkout-service",
					},
					Annotations:     map[string]string{},
					Priority:        "critical",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				lowImpactAlert := &holmesgpt.InvestigateRequest{
					AlertName: "DevToolsSlowResponse",
					Namespace: "development",
					Labels: map[string]string{
						"revenue_impact": "none",
						"user_facing":    "false",
						"business_tier":  "support",
						"service":        "dev-tools",
					},
					Annotations:     map[string]string{},
					Priority:        "warning",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				highImpactResponse, err := client.Investigate(ctx, highImpactAlert)
				Expect(err).ToNot(HaveOccurred())

				lowImpactResponse, err := client.Investigate(ctx, lowImpactAlert)
				Expect(err).ToNot(HaveOccurred())

				// BR-INS-007: ROI calculations must reflect business context
				highImpactStrategies := highImpactResponse.StrategyInsights.RecommendedStrategies
				lowImpactStrategies := lowImpactResponse.StrategyInsights.RecommendedStrategies

				Expect(len(highImpactStrategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: High-impact scenarios must provide strategy recommendations")
				Expect(len(lowImpactStrategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Low-impact scenarios must provide strategy recommendations")

				// Business requirement: High-impact alerts should justify higher ROI
				if len(highImpactStrategies) > 0 && len(lowImpactStrategies) > 0 {
					highROI := highImpactStrategies[0].ROI
					lowROI := lowImpactStrategies[0].ROI

					By("High business impact should result in higher ROI strategies")
					Expect(highROI).To(BeNumerically(">", lowROI),
						"ROI must be calculated based on business impact, not static values")

					// Business validation: ROI should be within reasonable business ranges
					Expect(highROI).To(BeNumerically(">=", 0.1),
						"High-impact strategies should show at least 10% ROI")
					Expect(highROI).To(BeNumerically("<=", 5.0),
						"ROI should be within realistic business bounds (<=500%)")
				}
			})

			It("should provide detailed cost-benefit analysis for strategy selection", func() {
				// BR-TYPE-001: Use standardized alert data structures
				businessCriticalAlert := &holmesgpt.InvestigateRequest{
					AlertName: "PaymentGatewayTimeout",
					Namespace: "payments",
					Labels: map[string]string{
						"sla_tier":        "premium",
						"downtime_cost":   "1000", // $1000/minute
						"customer_impact": "high",
						"service":         "gateway-service",
					},
					Annotations:     map[string]string{},
					Priority:        "critical",
					AsyncProcessing: false,
					IncludeContext:  true,
				}

				response, err := client.Investigate(ctx, businessCriticalAlert)
				Expect(err).ToNot(HaveOccurred())

				strategies := response.StrategyInsights.RecommendedStrategies
				Expect(len(strategies)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Context-aware strategies must be provided for AI recommendation confidence")

				// BR-INS-007: Cost-benefit analysis should be specific to business context
				for _, strategy := range strategies {
					By(fmt.Sprintf("Validating cost-benefit for strategy: %s", strategy.StrategyName))

					// Business requirement: Strategies should consider downtime costs
					Expect(strategy.EstimatedCost).To(BeNumerically(">", 0),
						"Strategy costs must be calculated, not defaulted to zero")

					// High-SLA services should prioritize time-to-resolution
					Expect(strategy.TimeToResolve).To(BeNumerically("<=", 15*time.Minute),
						"Premium SLA services require fast resolution strategies")

					// Business justification should reference cost factors
					if strategy.BusinessJustification != "" {
						justification := strategy.BusinessJustification
						hasBusinessContext := strings.Contains(justification, "cost") ||
							strings.Contains(justification, "downtime") ||
							strings.Contains(justification, "premium") ||
							strings.Contains(justification, "revenue")
						Expect(hasBusinessContext).To(BeTrue(),
							"Business justification must reference actual cost factors")
					}
				}
			})
		})
	})

	// BR-HAPI-011: Context-Aware Analysis
	Describe("BR-HAPI-011: Historical Pattern Integration", func() {
		Context("when requesting historical patterns", func() {
			It("should correlate patterns with current alert context", func() {
				alertContext := types.AlertContext{
					ID:          "test-alert-001",
					Name:        "MemoryLeakAlert",
					Severity:    "critical",
					Description: "Memory usage increasing continuously",
					Labels:      map[string]string{"service": "api-server", "team": "backend"},
					Annotations: map[string]string{"pattern": "memory_leak"},
				}

				patternRequest := &holmesgpt.PatternRequest{
					PatternType:  "remediation_success",
					TimeWindow:   24 * time.Hour,
					AlertContext: alertContext,
				}

				response, err := client.GetHistoricalPatterns(ctx, patternRequest)
				Expect(err).ToNot(HaveOccurred())

				// BR-HAPI-011: Historical patterns must be contextually relevant
				Expect(len(response.Patterns)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Historical pattern analysis must detect patterns for AI confidence requirements")

				// Business requirement: Patterns should relate to alert characteristics
				relevantPatternFound := false
				for _, pattern := range response.Patterns {
					By(fmt.Sprintf("Evaluating pattern relevance: %s", pattern.PatternID))

					// Pattern should be relevant to the alert context
					contextMatch := strings.Contains(pattern.BusinessContext, "memory") ||
						strings.Contains(pattern.BusinessContext, "leak") ||
						strings.Contains(pattern.BusinessContext, "api") ||
						strings.Contains(pattern.BusinessContext, alertContext.Labels["service"])

					if contextMatch {
						relevantPatternFound = true
						// BR-INS-007: Historical success rate must meet >80% requirement
						Expect(pattern.HistoricalSuccessRate).To(BeNumerically(">=", 0.80),
							"Historical patterns must show >80% success rate")
					}
				}

				Expect(relevantPatternFound).To(BeTrue(),
					"Historical patterns must include context-relevant entries")

				// Business requirement: Statistical significance validation
				Expect(response.StatisticalPValue).To(BeNumerically("<=", 0.05),
					"Pattern analysis must be statistically significant (p-value ≤ 0.05)")
			})
		})
	})

	// Strategy Analysis Tests - Using correct method name
	Describe("BR-INS-007: Strategy Analysis", func() {
		Context("when analyzing remediation strategies", func() {
			It("should select optimal strategies based on business context and ROI analysis", func() {
				alertContext := types.AlertContext{
					ID:       "test-alert-strategy",
					Name:     "DatabaseConnectionFailure",
					Severity: "critical",
					Labels: map[string]string{
						"service":   "payment-service",
						"namespace": "production",
					},
				}

				strategies := []holmesgpt.RemediationStrategy{
					{Name: "immediate_restart", Cost: 50, TimeToResolve: 2 * time.Minute},
					{Name: "connection_pool_scaling", Cost: 200, TimeToResolve: 5 * time.Minute},
					{Name: "database_failover", Cost: 800, TimeToResolve: 1 * time.Minute},
				}

				response, err := client.AnalyzeRemediationStrategies(ctx, &holmesgpt.StrategyAnalysisRequest{
					AvailableStrategies: strategies,
					AlertContext:        alertContext,
				})
				Expect(err).ToNot(HaveOccurred())

				optimal := response.OptimalStrategy

				// Business requirement: Should not just pick first strategy
				Expect(optimal.Name).ToNot(Equal("immediate_restart"),
					"Should not default to first strategy for critical payment service issues")

				// Business requirement: Should consider business context in selection
				if optimal.Name == "database_failover" {
					Expect(optimal.ExpectedROI).To(BeNumerically(">=", 0.20),
						"High-cost strategies should show high ROI for payment services")
				}

				// Justification should be business-specific, not generic
				Expect(optimal.Justification).To(ContainSubstring("payment"),
					"Strategy justification should reference specific service context")

				// Success rate should meet business requirements
				Expect(optimal.SuccessRate).To(BeNumerically(">=", 0.80),
					"Selected strategy must meet >80% success rate requirement")
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUclientUbusinessUlogic(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UclientUbusinessUlogic Suite")
}
