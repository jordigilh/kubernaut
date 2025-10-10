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
//go:build integration
// +build integration

package validation_quality

import (
	"context"
	"fmt"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Confidence and Consistency Validation Suite", Ordered, func() {
	var (
		logger       *logrus.Logger
		stateManager *shared.ComprehensiveStateManager
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Use comprehensive state manager with database transaction isolation
		patterns := &shared.TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Confidence and Consistency Validation")

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}
	})

	AfterAll(func() {
		// Comprehensive cleanup of all managed state
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		logger.Debug("Starting isolated test with comprehensive state management")
	})

	AfterEach(func() {
		logger.Debug("Test completed - comprehensive state isolation maintained")
	})

	createSLMClient := func() llm.Client {
		// Use fake client to eliminate external dependencies
		return shared.NewTestSLMClient()
	}

	Context("Confidence Calibration Validation", func() {
		It("should exhibit high confidence for clear-cut scenarios", func() {
			client := createSLMClient()

			clearCutScenarios := []struct {
				name            string
				alert           types.Alert
				minConfidence   float64
				expectedActions []string
			}{
				{
					name: "obvious_storage_expansion",
					alert: types.Alert{
						Name:        "PVCNearFull",
						Status:      "firing",
						Severity:    "warning",
						Description: "PVC is 95% full and growing",
						Namespace:   "production",
						Resource:    "database-storage",
						Labels: map[string]string{
							"alertname": "PVCNearFull",
							"usage":     "95%",
							"trend":     "increasing",
						},
						Annotations: map[string]string{
							"description": "Storage usage is 95% and increasing rapidly",
							"solution":    "expand_storage",
						},
					},
					minConfidence:   0.85,
					expectedActions: []string{"expand_pvc"},
				},
				{
					name: "obvious_security_threat",
					alert: types.Alert{
						Name:        "ActiveSecurityThreat",
						Status:      "firing",
						Severity:    "critical",
						Description: "Active malware detected in pod",
						Namespace:   "production",
						Resource:    "compromised-pod",
						Labels: map[string]string{
							"alertname":   "ActiveSecurityThreat",
							"threat_type": "malware",
							"severity":    "critical",
						},
						Annotations: map[string]string{
							"description": "Confirmed malware presence detected",
							"action":      "immediate_isolation",
						},
					},
					minConfidence:   0.90,
					expectedActions: []string{"quarantine_pod"},
				},
				{
					name: "obvious_deployment_failure",
					alert: types.Alert{
						Name:        "DeploymentFailed",
						Status:      "firing",
						Severity:    "critical",
						Description: "Deployment completely failed - 0 ready replicas",
						Namespace:   "production",
						Resource:    "web-app",
						Labels: map[string]string{
							"alertname":        "DeploymentFailed",
							"ready_replicas":   "0",
							"desired_replicas": "3",
						},
						Annotations: map[string]string{
							"description":     "All replicas failed to start",
							"last_known_good": "revision-5",
						},
					},
					minConfidence:   0.80,
					expectedActions: []string{"rollback_deployment"},
				},
			}

			for _, scenario := range clearCutScenarios {
				recommendation, err := client.AnalyzeAlert(context.Background(), scenario.alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-AI-001-CONFIDENCE: Confidence consistency validation must return valid confidence metrics for AI requirements")

				// Validate high confidence for clear scenarios
				Expect(recommendation.Confidence).To(BeNumerically(">=", scenario.minConfidence),
					"Scenario %s should have confidence >= %f, got %f",
					scenario.name, scenario.minConfidence, recommendation.Confidence)

				// Validate appropriate action
				Expect(recommendation.Action).To(BeElementOf(scenario.expectedActions),
					"Scenario %s produced unexpected action: %s", scenario.name, recommendation.Action)

				logger.WithFields(logrus.Fields{
					"scenario":     scenario.name,
					"action":       recommendation.Action,
					"confidence":   recommendation.Confidence,
					"expected_min": scenario.minConfidence,
				}).Info("High confidence scenario validation")
			}
		})

		It("should exhibit moderate confidence for ambiguous scenarios", func() {
			client := createSLMClient()

			ambiguousScenarios := []struct {
				name          string
				alert         types.Alert
				maxConfidence float64
				minConfidence float64
			}{
				{
					name: "vague_performance_issue",
					alert: types.Alert{
						Name:        "PerformanceDegradation",
						Status:      "firing",
						Severity:    "warning",
						Description: "Application response time degraded",
						Namespace:   "production",
						Resource:    "microservice",
						Labels: map[string]string{
							"alertname": "PerformanceDegradation",
							"metric":    "response_time",
						},
						Annotations: map[string]string{
							"description": "Response times are higher than normal",
						},
					},
					minConfidence: 0.3,
					maxConfidence: 0.75,
				},
				{
					name: "intermittent_connectivity",
					alert: types.Alert{
						Name:        "IntermittentConnectivity",
						Status:      "firing",
						Severity:    "warning",
						Description: "Occasional connection timeouts",
						Namespace:   "production",
						Resource:    "api-gateway",
						Labels: map[string]string{
							"alertname": "IntermittentConnectivity",
							"pattern":   "occasional",
						},
						Annotations: map[string]string{
							"description": "Some requests are timing out",
						},
					},
					minConfidence: 0.3,
					maxConfidence: 0.7,
				},
			}

			for _, scenario := range ambiguousScenarios {
				recommendation, err := client.AnalyzeAlert(context.Background(), scenario.alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-AI-001-CONFIDENCE: Confidence consistency validation must return valid confidence metrics for AI requirements")

				// Should have moderate confidence for ambiguous cases
				Expect(recommendation.Confidence).To(BeNumerically(">=", scenario.minConfidence),
					"Scenario %s confidence too low: got %f, expected >= %f",
					scenario.name, recommendation.Confidence, scenario.minConfidence)

				Expect(recommendation.Confidence).To(BeNumerically("<=", scenario.maxConfidence),
					"Scenario %s confidence too high: got %f, expected <= %f",
					scenario.name, recommendation.Confidence, scenario.maxConfidence)

				logger.WithFields(logrus.Fields{
					"scenario":       scenario.name,
					"action":         recommendation.Action,
					"confidence":     recommendation.Confidence,
					"expected_range": fmt.Sprintf("%.2f-%.2f", scenario.minConfidence, scenario.maxConfidence),
				}).Info("Moderate confidence scenario validation")
			}
		})
	})

	Context("Decision Consistency Under Variations", func() {
		It("should maintain consistency despite minor alert variations", func() {
			client := createSLMClient()

			baseAlert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Memory usage above 80%",
				Namespace:   "production",
				Resource:    "web-service",
				Labels: map[string]string{
					"alertname": "HighMemoryUsage",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"description": "Memory usage is above normal threshold",
				},
			}

			// Create variations of the same essential alert
			variations := []types.Alert{
				baseAlert, // Original
				{
					// Slightly different description
					Name:        "HighMemoryUsage",
					Status:      "firing",
					Severity:    "warning",
					Description: "Memory consumption above 80% threshold",
					Namespace:   "production",
					Resource:    "web-service",
					Labels:      baseAlert.Labels,
					Annotations: map[string]string{
						"description": "Memory utilization exceeds normal levels",
					},
				},
				{
					// Additional label
					Name:        "HighMemoryUsage",
					Status:      "firing",
					Severity:    "warning",
					Description: "Memory usage above 80%",
					Namespace:   "production",
					Resource:    "web-service",
					Labels: map[string]string{
						"alertname": "HighMemoryUsage",
						"severity":  "warning",
						"component": "backend",
					},
					Annotations: baseAlert.Annotations,
				},
			}

			var recommendations []*types.ActionRecommendation
			var actions []string
			var confidences []float64

			for i, alert := range variations {
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-AI-001-CONFIDENCE: Confidence consistency validation must return valid confidence metrics for AI requirements")

				recommendations = append(recommendations, shared.ConvertAnalyzeAlertResponse(recommendation))
				actions = append(actions, recommendation.Action)
				confidences = append(confidences, recommendation.Confidence)

				logger.WithFields(logrus.Fields{
					"variation":  i + 1,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Info("Consistency variation test")
			}

			// All variations should produce the same action
			firstAction := actions[0]
			for i, action := range actions {
				Expect(action).To(Equal(firstAction),
					"Variation %d produced different action: expected %s, got %s", i+1, firstAction, action)
			}

			// Confidence should be within reasonable range (±0.15)
			firstConfidence := confidences[0]
			for i, confidence := range confidences {
				Expect(math.Abs(confidence-firstConfidence)).To(BeNumerically("<=", 0.15),
					"Variation %d confidence too different: expected ~%f, got %f", i+1, firstConfidence, confidence)
			}

			logger.WithField("consistency_test", "passed").Info("Minor variation consistency verified")
		})

		It("should show appropriate sensitivity to significant alert changes", func() {
			client := createSLMClient()

			baseAlert := types.Alert{
				Name:        "ResourceIssue",
				Status:      "firing",
				Severity:    "warning",
				Description: "Resource usage elevated",
				Namespace:   "production",
				Resource:    "service",
				Labels: map[string]string{
					"alertname": "ResourceIssue",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"description": "Resource metrics above normal",
				},
			}

			// Create significantly different variations
			significantVariations := []struct {
				name  string
				alert types.Alert
			}{
				{
					name:  "original_warning",
					alert: baseAlert,
				},
				{
					name: "escalated_critical",
					alert: types.Alert{
						Name:        "ResourceIssue",
						Status:      "firing",
						Severity:    "critical", // Escalated severity
						Description: "Resource usage critically high",
						Namespace:   "production",
						Resource:    "service",
						Labels: map[string]string{
							"alertname": "ResourceIssue",
							"severity":  "critical",
						},
						Annotations: map[string]string{
							"description": "Critical resource exhaustion detected",
						},
					},
				},
				{
					name: "security_context",
					alert: types.Alert{
						Name:        "SecurityIncident",
						Status:      "firing",
						Severity:    "critical",
						Description: "Security breach detected",
						Namespace:   "production",
						Resource:    "service",
						Labels: map[string]string{
							"alertname":   "SecurityIncident",
							"severity":    "critical",
							"threat_type": "intrusion",
						},
						Annotations: map[string]string{
							"description": "Unauthorized access detected",
						},
					},
				},
			}

			var results []struct {
				name       string
				action     string
				confidence float64
			}

			for _, variation := range significantVariations {
				recommendation, err := client.AnalyzeAlert(context.Background(), variation.alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-AI-001-CONFIDENCE: Confidence consistency validation must return valid confidence metrics for AI requirements")

				results = append(results, struct {
					name       string
					action     string
					confidence float64
				}{
					name:       variation.name,
					action:     recommendation.Action,
					confidence: recommendation.Confidence,
				})

				logger.WithFields(logrus.Fields{
					"variation":  variation.name,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Info("Significant variation test")
			}

			// Should produce different actions for significantly different contexts
			warningAction := results[0].action
			criticalAction := results[1].action
			securityAction := results[2].action

			// Critical alert should potentially have different response than warning
			if criticalAction == warningAction {
				// If same action, confidence should be notably different
				warningConfidence := results[0].confidence
				criticalConfidence := results[1].confidence
				Expect(math.Abs(criticalConfidence-warningConfidence)).To(BeNumerically(">", 0.1),
					"Critical alert should show different confidence than warning")
			}

			// Security alert should definitely have different approach
			Expect(securityAction).To(BeElementOf([]string{
				"quarantine_pod",
				"notify_only",
				"collect_diagnostics",
			}), "Security alert should trigger security-focused actions")

			logger.WithField("sensitivity_test", "passed").Info("Significant variation sensitivity verified")
		})
	})

	Context("Temperature and Randomness Control", func() {
		It("should demonstrate controlled randomness within acceptable bounds", func() {
			// Test with same alert multiple times to verify consistency bounds
			// Configuration no longer needed for fake client

			client := shared.NewTestSLMClient()

			alert := types.Alert{
				Name:        "ConsistencyTest",
				Status:      "firing",
				Severity:    "warning",
				Description: "Test alert for consistency validation",
				Namespace:   "production",
				Resource:    "test-service",
				Labels: map[string]string{
					"alertname": "ConsistencyTest",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"description": "Test alert for model consistency",
				},
			}

			const iterations = 10
			var actions []string
			var confidences []float64

			for i := 0; i < iterations; i++ {
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil(), "BR-AI-001-CONFIDENCE: Confidence consistency validation must return valid confidence metrics for AI requirements")

				actions = append(actions, recommendation.Action)
				confidences = append(confidences, recommendation.Confidence)

				logger.WithFields(logrus.Fields{
					"iteration":  i + 1,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Debug("Randomness control test iteration")
			}

			// Calculate action consistency
			actionCounts := make(map[string]int)
			for _, action := range actions {
				actionCounts[action]++
			}

			// Dominant action should appear in majority of cases (>60%)
			maxCount := 0
			var dominantAction string
			for action, count := range actionCounts {
				if count > maxCount {
					maxCount = count
					dominantAction = action
				}
			}

			consistencyRatio := float64(maxCount) / float64(iterations)
			Expect(consistencyRatio).To(BeNumerically(">=", 0.6),
				"Dominant action %s should appear in >60%% of cases, got %.1f%%",
				dominantAction, consistencyRatio*100)

			// Calculate confidence variance
			var confidenceSum float64
			for _, conf := range confidences {
				confidenceSum += conf
			}
			avgConfidence := confidenceSum / float64(iterations)

			var varianceSum float64
			for _, conf := range confidences {
				diff := conf - avgConfidence
				varianceSum += diff * diff
			}
			confidenceVariance := varianceSum / float64(iterations)
			confidenceStdDev := math.Sqrt(confidenceVariance)

			// Standard deviation should be reasonable (< 0.2)
			Expect(confidenceStdDev).To(BeNumerically("<=", 0.2),
				"Confidence standard deviation should be <= 0.2, got %.3f", confidenceStdDev)

			logger.WithFields(logrus.Fields{
				"dominant_action":    dominantAction,
				"consistency_ratio":  consistencyRatio,
				"avg_confidence":     avgConfidence,
				"confidence_std_dev": confidenceStdDev,
				"unique_actions":     len(actionCounts),
			}).Info("Randomness control validation completed")
		})
	})
})
