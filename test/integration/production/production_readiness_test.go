//go:build integration
// +build integration

package production

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/config"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
)

var _ = Describe("Production Readiness Test Suite", Ordered, func() {
	var (
		logger     *logrus.Logger
		testUtils  *shared.IntegrationTestUtils
		repository actionhistory.Repository
		testConfig shared.IntegrationConfig
	)

	BeforeAll(func() {
		testConfig = shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}

		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Setup database
		var err error
		testUtils, err = shared.NewIntegrationTestUtils(logger)
		Expect(err).ToNot(HaveOccurred())

		// Initialize fresh database
		Expect(testUtils.InitializeFreshDatabase()).To(Succeed())

		repository = testUtils.Repository
	})

	AfterAll(func() {
		if testUtils != nil {
			testUtils.Close()
		}
	})

	BeforeEach(func() {
		// Clean database before each test
		Expect(testUtils.CleanDatabase()).To(Succeed())
	})

	Context("Critical Decision Making Validation", func() {
		createSLMClient := func(contextSize int) slm.Client {
			slmConfig := config.SLMConfig{
				Endpoint:       testConfig.OllamaEndpoint,
				Model:          testConfig.OllamaModel,
				Provider:       "localai",
				Timeout:        testConfig.TestTimeout,
				RetryCount:     1,
				Temperature:    0.3,
				MaxTokens:      500,
				MaxContextSize: contextSize,
			}

			// Use simplified MCP client creation with real K8s MCP server
			mcpClient := testUtils.CreateMCPClient(testConfig)

			slmClient, err := slm.NewClientWithMCP(slmConfig, mcpClient, logger)
			Expect(err).ToNot(HaveOccurred())
			return slmClient
		}

		seedFailureHistory := func(resourceName, actionType string, failures int, effectivenessScore float64) {
			for i := 0; i < failures; i++ {
				actionRecord := &actionhistory.ActionRecord{
					ResourceReference: actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      resourceName,
					},
					ActionID:  fmt.Sprintf("failure-action-%d-%d", failures, i),
					Timestamp: time.Now().Add(-time.Duration(i+1) * time.Hour),
					Alert: actionhistory.AlertContext{
						Name:        "TestAlert",
						Severity:    "warning",
						Labels:      map[string]string{"test": "failure"},
						Annotations: map[string]string{"test": "failure"},
						FiringTime:  time.Now().Add(-time.Duration(i+1) * time.Hour),
					},
					ModelUsed:           testConfig.OllamaModel,
					Confidence:          0.8,
					Reasoning:           shared.StringPtr("Test failure scenario"),
					ActionType:          actionType,
					Parameters:          map[string]interface{}{"test": true},
					ResourceStateBefore: map[string]interface{}{"status": "before"},
					ResourceStateAfter:  map[string]interface{}{"status": "after"},
				}

				trace, err := repository.StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				// Mark as failed with low effectiveness
				trace.EffectivenessScore = &effectivenessScore
				trace.ExecutionStatus = "failed"
				err = repository.UpdateActionTrace(context.Background(), trace)
				Expect(err).ToNot(HaveOccurred())
			}
		}

		It("should prioritize safety over action when multiple failures detected", func() {
			client := createSLMClient(16000) // 16K context

			// Seed multiple restart_pod failures
			seedFailureHistory("critical-app", "restart_pod", 3, 0.1)

			alert := types.Alert{
				Name:        "PodCrashLooping",
				Status:      "firing",
				Severity:    "critical",
				Description: "Pod is crash looping in critical application",
				Namespace:   "production",
				Resource:    "critical-app",
				Labels: map[string]string{
					"alertname":  "PodCrashLooping",
					"deployment": "critical-app",
					"severity":   "critical",
				},
				Annotations: map[string]string{
					"description": "Pod is crash looping in critical application",
					"runbook":     "Check application logs and configuration",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// With multiple restart failures, should avoid restart_pod
			Expect(recommendation.Action).ToNot(Equal("restart_pod"))
			// Should prefer safer alternatives
			Expect(recommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"rollback_deployment",
				"increase_resources",
			}))

			logger.WithFields(logrus.Fields{
				"scenario":   "multiple_restart_failures",
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Safety-first decision validation")
		})

		It("should maintain high confidence for well-established patterns", func() {
			client := createSLMClient(16000)

			// Seed successful scale_deployment history
			for i := 0; i < 5; i++ {
				actionRecord := &actionhistory.ActionRecord{
					ResourceReference: actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      "webapp",
					},
					ActionID:  fmt.Sprintf("success-action-%d", i),
					Timestamp: time.Now().Add(-time.Duration(i+1) * time.Hour),
					Alert: actionhistory.AlertContext{
						Name:        "HighMemoryUsage",
						Severity:    "warning",
						Labels:      map[string]string{"test": "success"},
						Annotations: map[string]string{"test": "success"},
						FiringTime:  time.Now().Add(-time.Duration(i+1) * time.Hour),
					},
					ModelUsed:           testConfig.OllamaModel,
					Confidence:          0.9,
					Reasoning:           shared.StringPtr("Test success scenario"),
					ActionType:          "scale_deployment",
					Parameters:          map[string]interface{}{"replicas": 3 + i},
					ResourceStateBefore: map[string]interface{}{"status": "before"},
					ResourceStateAfter:  map[string]interface{}{"status": "after"},
				}

				trace, err := repository.StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				// Mark as successful with high effectiveness
				effectiveness := 0.9
				trace.EffectivenessScore = &effectiveness
				trace.ExecutionStatus = "completed"
				err = repository.UpdateActionTrace(context.Background(), trace)
				Expect(err).ToNot(HaveOccurred())
			}

			alert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Memory usage above 80% for deployment webapp",
				Namespace:   "production",
				Resource:    "webapp",
				Labels: map[string]string{
					"alertname":  "HighMemoryUsage",
					"deployment": "webapp",
					"severity":   "warning",
				},
				Annotations: map[string]string{
					"description": "Memory usage above 80% for deployment webapp",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should choose scale_deployment with high confidence
			Expect(recommendation.Action).To(Equal("scale_deployment"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))

			logger.WithFields(logrus.Fields{
				"scenario":   "established_success_pattern",
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
			}).Info("High confidence pattern validation")
		})

		It("should exhibit appropriate caution for security alerts", func() {
			client := createSLMClient(16000)

			alert := types.Alert{
				Name:        "SecurityThreatDetected",
				Status:      "firing",
				Severity:    "critical",
				Description: "Potential security threat detected in pod",
				Namespace:   "production",
				Resource:    "web-app-456",
				Labels: map[string]string{
					"alertname":   "SecurityThreatDetected",
					"pod":         "web-app-456",
					"severity":    "critical",
					"threat_type": "privilege_escalation",
				},
				Annotations: map[string]string{
					"description":  "Suspicious privilege escalation attempt detected",
					"threat_level": "high",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Security threats should trigger quarantine or notification
			Expect(recommendation.Action).To(BeElementOf([]string{
				"quarantine_pod",
				"notify_only",
				"collect_diagnostics",
			}))

			// Should have high confidence for security decisions
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.7))

			logger.WithFields(logrus.Fields{
				"scenario":   "security_threat",
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
			}).Info("Security alert validation")
		})
	})

	createSLMClient := func(contextSize int) slm.Client {
		slmConfig := config.SLMConfig{
			Endpoint:       testConfig.OllamaEndpoint,
			Model:          testConfig.OllamaModel,
			Provider:       "localai",
			Timeout:        testConfig.TestTimeout,
			RetryCount:     1,
			Temperature:    0.3,
			MaxTokens:      500,
			MaxContextSize: contextSize,
		}

		// Use simplified MCP client creation with real K8s MCP server
		mcpClient := testUtils.CreateMCPClient(testConfig)

		slmClient, err := slm.NewClientWithMCP(slmConfig, mcpClient, logger)
		Expect(err).ToNot(HaveOccurred())
		return slmClient
	}

	Context("Decision Consistency Validation", func() {
		It("should provide consistent decisions for identical alerts", func() {
			client := createSLMClient(16000) // Use 16K context

			alert := types.Alert{
				Name:        "HighCPUUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "CPU usage above 85% for deployment",
				Namespace:   "production",
				Resource:    "api-service",
				Labels: map[string]string{
					"alertname":  "HighCPUUsage",
					"deployment": "api-service",
					"severity":   "warning",
				},
				Annotations: map[string]string{
					"description": "CPU usage above 85% for deployment",
				},
			}

			var recommendations []*types.ActionRecommendation
			var actions []string
			var confidences []float64

			// Test multiple iterations for consistency
			for i := 0; i < 5; i++ {
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())

				recommendations = append(recommendations, recommendation)
				actions = append(actions, recommendation.Action)
				confidences = append(confidences, recommendation.Confidence)

				logger.WithFields(logrus.Fields{
					"iteration":  i + 1,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Info("Consistency test iteration")
			}

			// Verify consistency
			firstAction := actions[0]
			for i, action := range actions {
				Expect(action).To(Equal(firstAction),
					"Action inconsistency at iteration %d: expected %s, got %s", i+1, firstAction, action)
			}

			// Confidence should be relatively stable (within 0.2 range)
			firstConfidence := confidences[0]
			for i, confidence := range confidences {
				Expect(confidence).To(BeNumerically("~", firstConfidence, 0.2),
					"Confidence inconsistency at iteration %d: expected ~%f, got %f", i+1, firstConfidence, confidence)
			}

			logger.WithField("consistency_validation", "passed").Info("Decision consistency verified")
		})
	})

	Context("Context Size Impact Validation", func() {
		testContextSizeDecisionQuality := func(contextSize int, expectedAction string) {
			client := createSLMClient(contextSize)

			alert := types.Alert{
				Name:        "StorageSpaceExhaustion",
				Status:      "firing",
				Severity:    "warning",
				Description: "Persistent volume is 95% full",
				Namespace:   "database",
				Resource:    "postgres-storage",
				Labels: map[string]string{
					"alertname": "PVCNearFull",
					"pvc":       "postgres-storage",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"description": "Persistent volume is 95% full",
					"usage":       "95%",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			logger.WithFields(logrus.Fields{
				"context_size": contextSize,
				"action":       recommendation.Action,
				"confidence":   recommendation.Confidence,
				"expected":     expectedAction,
			}).Info("Context size decision quality test")

			// Decision should be appropriate regardless of context size
			Expect(recommendation.Action).To(BeElementOf([]string{
				"expand_pvc",
				"notify_only",
				"collect_diagnostics",
			}))

			// Confidence should remain reasonable
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.5))
		}

		It("should maintain decision quality across different context sizes", func() {
			contextSizes := []int{16000, 8000, 4000}

			for _, contextSize := range contextSizes {
				testContextSizeDecisionQuality(contextSize, "expand_pvc")
			}
		})
	})

	Context("Confidence Threshold Validation", func() {
		It("should exhibit appropriate confidence levels for different scenarios", func() {
			client := createSLMClient(16000)

			scenarios := []struct {
				name            string
				alert           types.Alert
				minConfidence   float64
				maxConfidence   float64
				expectedActions []string
				description     string
			}{
				{
					name: "clear_storage_issue",
					alert: types.Alert{
						Name:        "PVCNearFull",
						Status:      "firing",
						Severity:    "warning",
						Description: "Storage 95% full - clear action needed",
						Namespace:   "production",
						Resource:    "data-storage",
						Labels: map[string]string{
							"alertname": "PVCNearFull",
							"usage":     "95%",
						},
					},
					minConfidence:   0.8,
					maxConfidence:   1.0,
					expectedActions: []string{"expand_pvc"},
					description:     "Clear storage issues should have high confidence",
				},
				{
					name: "ambiguous_performance_issue",
					alert: types.Alert{
						Name:        "HighLatency",
						Status:      "firing",
						Severity:    "warning",
						Description: "Application showing high latency",
						Namespace:   "production",
						Resource:    "complex-service",
						Labels: map[string]string{
							"alertname": "HighLatency",
							"service":   "complex-service",
						},
					},
					minConfidence:   0.4,
					maxConfidence:   0.8,
					expectedActions: []string{"notify_only", "collect_diagnostics", "scale_deployment", "restart_pod"},
					description:     "Ambiguous issues should have moderate confidence",
				},
				{
					name: "critical_security_threat",
					alert: types.Alert{
						Name:        "SecurityBreach",
						Status:      "firing",
						Severity:    "critical",
						Description: "Active security breach detected",
						Namespace:   "production",
						Resource:    "compromised-pod",
						Labels: map[string]string{
							"alertname":   "SecurityBreach",
							"threat_type": "active_attack",
						},
					},
					minConfidence:   0.7,
					maxConfidence:   1.0,
					expectedActions: []string{"quarantine_pod", "notify_only"},
					description:     "Security threats should have high confidence",
				},
			}

			for _, scenario := range scenarios {
				recommendation, err := client.AnalyzeAlert(context.Background(), scenario.alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())

				// Validate confidence range
				Expect(recommendation.Confidence).To(BeNumerically(">=", scenario.minConfidence),
					"Confidence too low for %s: expected >= %f, got %f",
					scenario.name, scenario.minConfidence, recommendation.Confidence)

				Expect(recommendation.Confidence).To(BeNumerically("<=", scenario.maxConfidence),
					"Confidence too high for %s: expected <= %f, got %f",
					scenario.name, scenario.maxConfidence, recommendation.Confidence)

				// Validate action appropriateness
				Expect(recommendation.Action).To(BeElementOf(scenario.expectedActions),
					"Inappropriate action for %s: got %s, expected one of %v",
					scenario.name, recommendation.Action, scenario.expectedActions)

				logger.WithFields(logrus.Fields{
					"scenario":    scenario.name,
					"action":      recommendation.Action,
					"confidence":  recommendation.Confidence,
					"description": scenario.description,
				}).Info("Confidence threshold validation")
			}
		})
	})

	Context("Stress Testing and Reliability", func() {
		It("should handle rapid consecutive alerts reliably", func() {
			client := createSLMClient(16000) // Use 16K context

			alerts := []types.Alert{
				{
					Name: "HighMemoryUsage", Status: "firing", Severity: "warning",
					Namespace: "production", Resource: "service-a",
					Description: "Memory usage high", Labels: map[string]string{"service": "a"},
				},
				{
					Name: "HighCPUUsage", Status: "firing", Severity: "warning",
					Namespace: "production", Resource: "service-b",
					Description: "CPU usage high", Labels: map[string]string{"service": "b"},
				},
				{
					Name: "DiskSpaceLow", Status: "firing", Severity: "critical",
					Namespace: "database", Resource: "storage-c",
					Description: "Disk space low", Labels: map[string]string{"storage": "c"},
				},
			}

			var totalTime time.Duration
			successCount := 0

			for i, alert := range alerts {
				startTime := time.Now()
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				responseTime := time.Since(startTime)
				totalTime += responseTime

				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())
				Expect(types.IsValidAction(recommendation.Action)).To(BeTrue())
				Expect(recommendation.Confidence).To(BeNumerically(">", 0))

				successCount++

				logger.WithFields(logrus.Fields{
					"alert_index":   i + 1,
					"alert_name":    alert.Name,
					"action":        recommendation.Action,
					"confidence":    recommendation.Confidence,
					"response_time": responseTime,
				}).Info("Stress test alert processed")
			}

			avgResponseTime := totalTime / time.Duration(len(alerts))

			// All alerts should be processed successfully
			Expect(successCount).To(Equal(len(alerts)))

			// Average response time should be reasonable
			Expect(avgResponseTime).To(BeNumerically("<", 10*time.Second))

			logger.WithFields(logrus.Fields{
				"total_alerts":      len(alerts),
				"success_count":     successCount,
				"total_time":        totalTime,
				"avg_response_time": avgResponseTime,
			}).Info("Stress test completed")
		})
	})

	Context("Production Scenario Simulation", func() {
		It("should handle real-world cascading failure scenario", func() {
			client := createSLMClient(16000) // 16K context for complex scenario

			// Simulate a cascading failure: storage full -> pod restarts -> deployment issues

			// Step 1: Storage alert
			storageAlert := types.Alert{
				Name:        "PVCNearFull",
				Status:      "firing",
				Severity:    "warning",
				Description: "Database storage 90% full",
				Namespace:   "production",
				Resource:    "db-storage",
				Labels:      map[string]string{"alertname": "PVCNearFull", "usage": "90%"},
			}

			rec1, err := client.AnalyzeAlert(context.Background(), storageAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(rec1.Action).To(BeElementOf([]string{"expand_pvc", "notify_only"}))

			// Step 2: Pod crashing due to storage issues
			podAlert := types.Alert{
				Name:        "PodCrashLooping",
				Status:      "firing",
				Severity:    "critical",
				Description: "Database pod crashing due to storage issues",
				Namespace:   "production",
				Resource:    "database-pod",
				Labels:      map[string]string{"alertname": "PodCrashLooping", "reason": "storage_full"},
			}

			rec2, err := client.AnalyzeAlert(context.Background(), podAlert)
			Expect(err).ToNot(HaveOccurred())
			// Should avoid restart_pod since it won't fix storage issue
			Expect(rec2.Action).To(BeElementOf([]string{"notify_only", "collect_diagnostics", "expand_pvc"}))

			// Step 3: Service degradation
			serviceAlert := types.Alert{
				Name:        "ServiceDegraded",
				Status:      "firing",
				Severity:    "critical",
				Description: "Database service experiencing high error rate",
				Namespace:   "production",
				Resource:    "database-service",
				Labels:      map[string]string{"alertname": "ServiceDegraded", "error_rate": "high"},
			}

			rec3, err := client.AnalyzeAlert(context.Background(), serviceAlert)
			Expect(err).ToNot(HaveOccurred())
			// Should focus on root cause (storage) rather than symptoms
			Expect(rec3.Action).To(BeElementOf([]string{"notify_only", "collect_diagnostics", "expand_pvc"}))

			logger.WithFields(logrus.Fields{
				"scenario":       "cascading_failure",
				"storage_action": rec1.Action,
				"pod_action":     rec2.Action,
				"service_action": rec3.Action,
			}).Info("Cascading failure scenario validation")
		})
	})
})
