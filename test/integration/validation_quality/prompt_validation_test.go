//go:build integration
// +build integration

package validation_quality

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Prompt Validation and Edge Case Testing", Ordered, func() {
	var (
		logger       *logrus.Logger
		stateManager *shared.ComprehensiveStateManager
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Use comprehensive state manager with database isolation
		patterns := &shared.TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Prompt Validation and Edge Case Testing")

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}
	})

	AfterAll(func() {
		// Comprehensive cleanup
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		logger.Debug("Starting prompt validation test with isolated state")
	})

	AfterEach(func() {
		logger.Debug("Prompt validation test completed - state automatically isolated")
	})

	getRepository := func() actionhistory.Repository {
		// Get isolated database repository from state manager
		dbHelper := stateManager.GetDatabaseHelper()
		return dbHelper.GetRepository()
	}

	createSLMClient := func() llm.Client {
		// Use fake client to eliminate external dependencies
		return shared.NewTestSLMClient()
	}

	Context("Critical Decision Boundary Testing", func() {
		It("should correctly escalate when action patterns suggest system instability", func() {
			client := createSLMClient()

			// Create a pattern of failed actions across different types
			failurePatterns := []struct {
				actionType    string
				failures      int
				effectiveness float64
			}{
				{"restart_pod", 3, 0.15},
				{"scale_deployment", 2, 0.25},
				{"increase_resources", 2, 0.20},
			}

			actionCounter := 0
			for _, pattern := range failurePatterns {
				for i := 0; i < pattern.failures; i++ {
					actionCounter++
					actionRecord := &actionhistory.ActionRecord{
						ResourceReference: actionhistory.ResourceReference{
							ResourceUID: fmt.Sprintf("test-uid-%d", actionCounter),
							Namespace:   "production",
							Kind:        "Deployment",
							Name:        fmt.Sprintf("unstable-service-%s-%d", pattern.actionType, i),
						},
						ActionID:  fmt.Sprintf("failure-%s-%d", pattern.actionType, i),
						Timestamp: time.Now().Add(-time.Duration(i+1) * time.Hour),
						Alert: actionhistory.AlertContext{
							Name:        "SystemInstability",
							Severity:    "critical",
							Labels:      map[string]string{"pattern": "failure"},
							Annotations: map[string]string{"pattern": "failure"},
							FiringTime:  time.Now().Add(-time.Duration(i+1) * time.Hour),
						},
						ModelUsed:           "fake-test-model",
						Confidence:          0.8,
						Reasoning:           shared.StringPtr("Test failure pattern"),
						ActionType:          pattern.actionType,
						Parameters:          map[string]interface{}{"test": true},
						ResourceStateBefore: map[string]interface{}{"status": "before"},
						ResourceStateAfter:  map[string]interface{}{"status": "after"},
					}

					trace, err := getRepository().StoreAction(context.Background(), actionRecord)
					Expect(err).ToNot(HaveOccurred())

					trace.EffectivenessScore = &pattern.effectiveness
					trace.ExecutionStatus = "failed"
					err = getRepository().UpdateActionTrace(context.Background(), trace)
					Expect(err).ToNot(HaveOccurred())
				}
			}

			// Test with an alert that should trigger escalation
			alert := types.Alert{
				Name:        "SystemInstability",
				Status:      "firing",
				Severity:    "critical",
				Description: "Multiple action failures detected across service",
				Labels: map[string]string{
					"alertname": "SystemInstability",
					"service":   "unstable-service",
					"namespace": "production",
					"severity":  "critical",
					"pattern":   "failure",
				},
				Annotations: map[string]string{
					"summary":     "System instability pattern detected",
					"description": "Multiple automated actions have failed",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil(), "BR-SF-001-RISK-SCORE: Prompt validation must return valid safety validation for risk assessment requirements")

			// Should recommend escalation due to failure pattern
			Expect(recommendation.Action).To(ContainSubstring("escalate"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.6))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning.Summary,
			}).Info("System instability escalation validation")
		})

		It("should handle malformed alert inputs gracefully", func() {
			client := createSLMClient()

			// Test various malformed inputs
			malformedAlerts := []types.Alert{
				// Empty alert
				{},
				// Missing critical fields
				{Name: "TestAlert"},
				// Extremely long description
				{
					Name:        "LongDescriptionAlert",
					Description: strings.Repeat("A", 10000),
					Severity:    "warning",
				},
				// Special characters in names
				{
					Name:        "Special@#$%^Alert",
					Description: "Alert with special characters",
					Severity:    "info",
				},
				// Very long label values
				{
					Name:        "LongLabelsAlert",
					Description: "Normal description",
					Severity:    "warning",
					Labels: map[string]string{
						"long_label": strings.Repeat("X", 5000),
					},
				},
			}

			for i, alert := range malformedAlerts {
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)

				// Should handle gracefully - either return error or valid recommendation
				if err != nil {
					// Acceptable to error on malformed input
					logger.WithFields(logrus.Fields{
						"test_case": i,
						"error":     err.Error(),
					}).Info("Malformed input gracefully handled with error")
				} else {
					// Or return a valid recommendation
					Expect(recommendation).ToNot(BeNil(), "BR-SF-001-RISK-SCORE: Prompt validation must return valid safety validation for risk assessment requirements")
					Expect(recommendation.Action).To(BeNumerically(">=", 1), "BR-SF-001-RISK-SCORE: Prompt validation must provide data for risk assessment requirements")
					logger.WithFields(logrus.Fields{
						"test_case": i,
						"action":    recommendation.Action,
					}).Info("Malformed input gracefully handled with recommendation")
				}
			}
		})

		It("should provide consistent recommendations for identical alerts", func() {
			client := createSLMClient()

			alert := types.Alert{
				Name:        "ConsistencyTest",
				Description: "Test alert for consistency validation",
				Severity:    "warning",
				Labels: map[string]string{
					"alertname": "ConsistencyTest",
					"service":   "test-service",
				},
			}

			var recommendations []*types.ActionRecommendation
			const numTests = 5

			// Get multiple recommendations for the same alert
			for i := 0; i < numTests; i++ {
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-SF-001-RISK-SCORE: Prompt validation must return valid safety validation for risk assessment requirements")
				recommendations = append(recommendations, shared.ConvertAnalyzeAlertResponse(recommendation))
			}

			// Verify consistency (fake client should return same results)
			baseAction := recommendations[0].Action
			baseConfidence := recommendations[0].Confidence

			for i, rec := range recommendations {
				Expect(rec.Action).To(Equal(baseAction),
					fmt.Sprintf("Recommendation %d action should be consistent", i))
				Expect(rec.Confidence).To(Equal(baseConfidence),
					fmt.Sprintf("Recommendation %d confidence should be consistent", i))
			}

			logger.WithFields(logrus.Fields{
				"action":     baseAction,
				"confidence": baseConfidence,
				"iterations": numTests,
			}).Info("Consistency validation completed")
		})
	})

	Context("Edge Case Scenario Testing", func() {
		It("should handle resource exhaustion scenarios appropriately", func() {
			client := createSLMClient()

			exhaustionScenarios := []types.Alert{
				{
					Name:        "MemoryExhaustion",
					Description: "Memory usage at 95% - OOM imminent",
					Severity:    "critical",
					Labels: map[string]string{
						"alertname":    "MemoryExhaustion",
						"instance":     "production-server-01",
						"memory_usage": "95%",
					},
				},
				{
					Name:        "DiskSpaceExhaustion",
					Description: "Disk space at 98% capacity",
					Severity:    "critical",
					Labels: map[string]string{
						"alertname":   "DiskSpaceExhaustion",
						"mount_point": "/var/log",
						"usage":       "98%",
					},
				},
				{
					Name:        "FileDescriptorExhaustion",
					Description: "File descriptors at 99% limit",
					Severity:    "warning",
					Labels: map[string]string{
						"alertname": "FileDescriptorExhaustion",
						"process":   "application-server",
					},
				},
			}

			for _, alert := range exhaustionScenarios {
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-SF-001-RISK-SCORE: Prompt validation must return valid safety validation for risk assessment requirements")
				Expect(recommendation.Action).To(BeNumerically(">=", 1), "BR-SF-001-RISK-SCORE: Prompt validation must provide data for risk assessment requirements")

				// Should be high confidence for resource exhaustion
				Expect(recommendation.Confidence).To(BeNumerically(">=", 0.7))

				logger.WithFields(logrus.Fields{
					"alert":      alert.Name,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Info("Resource exhaustion scenario validated")
			}
		})

		It("should handle cascading failure scenarios", func() {
			client := createSLMClient()

			// Simulate cascade: Database → API → Frontend
			cascadeAlerts := []types.Alert{
				{
					Name:        "DatabaseConnectionFailure",
					Description: "Database connection pool exhausted",
					Severity:    "critical",
					Labels: map[string]string{
						"component": "database",
						"layer":     "data",
					},
				},
				{
					Name:        "APIServiceFailure",
					Description: "API service responding with 50x errors",
					Severity:    "critical",
					Labels: map[string]string{
						"component":  "api",
						"layer":      "service",
						"depends_on": "database",
					},
				},
				{
					Name:        "FrontendServiceFailure",
					Description: "Frontend unable to reach backend APIs",
					Severity:    "warning",
					Labels: map[string]string{
						"component":  "frontend",
						"layer":      "presentation",
						"depends_on": "api",
					},
				},
			}

			var cascadeRecommendations []*types.ActionRecommendation

			for _, alert := range cascadeAlerts {
				// Guideline #1: Reuse existing type conversion helper
				analyzeResponse, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(analyzeResponse).ToNot(BeNil(), "BR-SF-001-RISK-SCORE: Prompt validation must return valid safety validation for risk assessment requirements")
				recommendation := shared.ConvertAnalyzeAlertResponse(analyzeResponse)
				cascadeRecommendations = append(cascadeRecommendations, recommendation)

				logger.WithFields(logrus.Fields{
					"component":  alert.Labels["component"],
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Info("Cascade failure component analyzed")
			}

			// Verify that recommendations are sensible
			Expect(len(cascadeRecommendations)).To(Equal(3))
			for i, rec := range cascadeRecommendations {
				Expect(rec.Action).To(BeNumerically(">=", 1), "BR-SF-001-RISK-SCORE: Prompt validation must provide data for risk assessment requirements")
				Expect(rec.Confidence).To(BeNumerically(">", 0.0))
				Expect(rec.Confidence).To(BeNumerically("<=", 1.0))
				logger.WithFields(logrus.Fields{
					"cascade_step": i,
					"action":       rec.Action,
				}).Info("Cascade step validated")
			}
		})
	})
})
