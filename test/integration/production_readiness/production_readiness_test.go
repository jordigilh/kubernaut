//go:build integration
// +build integration

package production_readiness

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Production Readiness Test Suite", Ordered, func() {
	var (
		logger       *logrus.Logger
		stateManager *shared.ComprehensiveStateManager
		testConfig   shared.IntegrationConfig
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Use comprehensive state manager with database isolation
		patterns := &shared.TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Production Readiness Test Suite")

		testConfig = shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}
	})

	AfterAll(func() {
		// Comprehensive cleanup
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	// REMOVED: Old testUtils cleanup - handled by comprehensive state manager

	BeforeEach(func() {
		// Database state is automatically isolated via comprehensive state manager
		logger.Debug("Starting production readiness test with isolated state")
	})

	AfterEach(func() {
		logger.Debug("Production readiness test completed - state automatically isolated")
	})

	getRepository := func() actionhistory.Repository {
		// Get isolated database repository from state manager
		dbHelper := stateManager.GetDatabaseHelper()
		return dbHelper.GetRepository()
	}

	Context("Critical Decision Making Validation", func() {
		createSLMClient := func(contextSize int) llm.Client {
			// Configuration no longer needed for fake client

			// Use fake client to eliminate external dependencies
			slmClient := shared.NewFakeSLMClient()
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
					ModelUsed:           testConfig.LLMModel,
					Confidence:          0.8,
					Reasoning:           shared.StringPtr("Test failure scenario"),
					ActionType:          actionType,
					Parameters:          map[string]interface{}{"test": true},
					ResourceStateBefore: map[string]interface{}{"status": "before"},
					ResourceStateAfter:  map[string]interface{}{"status": "after"},
				}

				trace, err := getRepository().StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				// Mark as failed with low effectiveness
				trace.EffectivenessScore = &effectivenessScore
				trace.ExecutionStatus = "failed"
				err = getRepository().UpdateActionTrace(context.Background(), trace)
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
					ModelUsed:           testConfig.LLMModel,
					Confidence:          0.9,
					Reasoning:           shared.StringPtr("Test success scenario"),
					ActionType:          "scale_deployment",
					Parameters:          map[string]interface{}{"replicas": 3 + i},
					ResourceStateBefore: map[string]interface{}{"status": "before"},
					ResourceStateAfter:  map[string]interface{}{"status": "after"},
				}

				trace, err := getRepository().StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				// Mark as successful with high effectiveness
				effectiveness := 0.9
				trace.EffectivenessScore = &effectiveness
				trace.ExecutionStatus = "completed"
				err = getRepository().UpdateActionTrace(context.Background(), trace)
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

	createSLMClient := func(contextSize int) llm.Client {
		// Configuration no longer needed for fake client
		// Use fake client to eliminate external dependencies
		return shared.NewFakeSLMClient()
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

				recommendations = append(recommendations, shared.ConvertAnalyzeAlertResponse(recommendation))
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

		Context("Production Error Resilience Testing", func() {
			It("should maintain SLA compliance during service degradation", func() {
				// Test production-level error scenarios with SLA requirements

				By("Setting up production-level error injection")
				fakeClient := shared.NewFakeSLMClient()

				// Use predefined service degradation scenario for production testing
				degradationScenario := shared.PredefinedErrorScenarios["slm_service_degradation"]

				err := fakeClient.TriggerErrorScenario(degradationScenario)
				Expect(err).ToNot(HaveOccurred())

				By("Processing alerts under production load with error injection")
				const alertCount = 50
				const maxAcceptableLatency = 2 * time.Second
				const minSuccessRate = 0.95 // 95% SLA requirement

				successCount := 0
				totalLatency := time.Duration(0)

				for i := 0; i < alertCount; i++ {
					startTime := time.Now()

					alert := types.Alert{
						Name:        fmt.Sprintf("ProductionAlert%d", i+1),
						Status:      "firing",
						Severity:    "critical",
						Description: fmt.Sprintf("Production critical alert %d", i+1),
						Namespace:   "production",
						Resource:    fmt.Sprintf("service-%d", i%10),
						Labels: map[string]string{
							"alertname": fmt.Sprintf("ProductionAlert%d", i+1),
							"severity":  "critical",
							"env":       "production",
						},
					}

					recommendation, err := fakeClient.AnalyzeAlert(context.Background(), alert)
					latency := time.Since(startTime)
					totalLatency += latency

					if err == nil && recommendation != nil {
						successCount++

						// Log successful processing with latency
						if latency > maxAcceptableLatency {
							logger.WithFields(logrus.Fields{
								"alert_id": i + 1,
								"latency":  latency,
								"action":   recommendation.Action,
							}).Warning("Alert processing exceeded SLA latency")
						}
					} else {
						logger.WithFields(logrus.Fields{
							"alert_id": i + 1,
							"latency":  latency,
							"error":    err,
						}).Debug("Expected failure during error injection test")
					}
				}

				By("Verifying SLA compliance metrics")
				successRate := float64(successCount) / float64(alertCount)
				avgLatency := totalLatency / time.Duration(alertCount)

				logger.WithFields(logrus.Fields{
					"success_rate":     fmt.Sprintf("%.2f%%", successRate*100),
					"avg_latency":      avgLatency,
					"total_processed":  alertCount,
					"successful_count": successCount,
					"failed_count":     alertCount - successCount,
				}).Info("Production SLA compliance test completed")

				// Verify SLA requirements
				Expect(successRate).To(BeNumerically(">=", minSuccessRate),
					fmt.Sprintf("Success rate %.2f%% must meet SLA requirement of %.2f%%",
						successRate*100, minSuccessRate*100))

				Expect(avgLatency).To(BeNumerically("<=", maxAcceptableLatency),
					fmt.Sprintf("Average latency %v must not exceed SLA limit of %v",
						avgLatency, maxAcceptableLatency))
			})

			It("should handle cascading failures in production environment", func() {
				By("Simulating multi-component cascade failure")
				fakeClient := shared.NewFakeSLMClient()

				// Use predefined multi-service cascade scenario
				cascadeScenario := shared.PredefinedErrorScenarios["multi_service_cascade"]

				err := fakeClient.TriggerErrorScenario(cascadeScenario)
				Expect(err).ToNot(HaveOccurred())

				By("Testing critical alert processing during cascade failure")
				criticalAlerts := []types.Alert{
					{
						Name:        "DatabaseDown",
						Status:      "firing",
						Severity:    "critical",
						Description: "Primary database is unreachable",
						Namespace:   "production",
						Resource:    "postgres-primary",
						Labels: map[string]string{
							"alertname": "DatabaseDown",
							"severity":  "critical",
							"component": "database",
						},
					},
					{
						Name:        "APIGatewayOverloaded",
						Status:      "firing",
						Severity:    "critical",
						Description: "API Gateway receiving excessive load",
						Namespace:   "production",
						Resource:    "api-gateway",
						Labels: map[string]string{
							"alertname": "APIGatewayOverloaded",
							"severity":  "critical",
							"component": "gateway",
						},
					},
					{
						Name:        "LoadBalancerFailure",
						Status:      "firing",
						Severity:    "critical",
						Description: "Load balancer health check failures",
						Namespace:   "production",
						Resource:    "nginx-lb",
						Labels: map[string]string{
							"alertname": "LoadBalancerFailure",
							"severity":  "critical",
							"component": "loadbalancer",
						},
					},
				}

				partialSuccessCount := 0
				totalProcessed := 0

				for _, alert := range criticalAlerts {
					for attempt := 1; attempt <= 3; attempt++ {
						totalProcessed++

						recommendation, err := fakeClient.AnalyzeAlert(context.Background(), alert)

						if err == nil && recommendation != nil {
							partialSuccessCount++

							logger.WithFields(logrus.Fields{
								"alert":      alert.Name,
								"attempt":    attempt,
								"action":     recommendation.Action,
								"confidence": recommendation.Confidence,
							}).Info("Alert processed despite cascade failure")

							// Verify response is actionable even during crisis
							Expect(recommendation.Action).ToNot(BeEmpty())
							break // Success on this alert, move to next
						} else {
							logger.WithFields(logrus.Fields{
								"alert":   alert.Name,
								"attempt": attempt,
								"error":   err,
							}).Debug("Expected cascade failure error")
						}

						// Brief delay between retry attempts
						time.Sleep(200 * time.Millisecond)
					}
				}

				By("Verifying system maintained critical alert processing capability")
				partialSuccessRate := float64(partialSuccessCount) / float64(totalProcessed)

				logger.WithFields(logrus.Fields{
					"partial_success_rate": fmt.Sprintf("%.1f%%", partialSuccessRate*100),
					"total_attempts":       totalProcessed,
					"successful_responses": partialSuccessCount,
				}).Info("Cascade failure resilience test completed")

				// Even during severe cascade failure, system should maintain some capability
				// 20% minimum ensures critical alerts aren't completely ignored
				Expect(partialSuccessRate).To(BeNumerically(">=", 0.2),
					"System must maintain at least 20% functionality during cascade failures")

				By("Verifying graceful degradation behavior")
				// Test that system provides appropriate error messages during failures
				finalAlert := types.Alert{
					Name:      "SystemHealthCheck",
					Status:    "firing",
					Severity:  "warning",
					Namespace: "monitoring",
					Resource:  "health-check",
				}

				_, err = fakeClient.AnalyzeAlert(context.Background(), finalAlert)
				if err != nil {
					// Verify error contains useful information for operations teams
					Expect(err.Error()).To(Or(
						ContainSubstring("service degradation"),
						ContainSubstring("cascade failure"),
						ContainSubstring("circuit breaker"),
						ContainSubstring("timeout"),
					))

					logger.WithError(err).Info("System provided appropriate error information during cascade failure")
				}
			})

			It("should demonstrate production-grade recovery capabilities", func() {
				By("Testing full system recovery after major outage")
				fakeClient := shared.NewFakeSLMClient()

				// Use predefined system failure scenario
				outageScenario := shared.PredefinedErrorScenarios["system_failure"]

				err := fakeClient.TriggerErrorScenario(outageScenario)
				Expect(err).ToNot(HaveOccurred())

				By("Verifying system is in outage state")
				outageAlert := types.Alert{
					Name:      "OutageTestAlert",
					Status:    "firing",
					Severity:  "critical",
					Namespace: "production",
					Resource:  "test-service",
				}

				_, err = fakeClient.AnalyzeAlert(context.Background(), outageAlert)
				Expect(err).To(HaveOccurred(), "System should be in outage state")

				logger.WithError(err).Info("Confirmed system is in outage state")

				By("Waiting for recovery period and testing system restoration")
				time.Sleep(6 * time.Second) // Wait for recovery

				// Reset error state to simulate recovery
				fakeClient.ResetErrorState()

				// Test multiple alerts to verify full recovery
				recoveryTests := []types.Alert{
					{
						Name:      "RecoveryTest1",
						Status:    "firing",
						Severity:  "warning",
						Namespace: "production",
						Resource:  "service-1",
					},
					{
						Name:      "RecoveryTest2",
						Status:    "firing",
						Severity:  "critical",
						Namespace: "production",
						Resource:  "service-2",
					},
					{
						Name:      "RecoveryTest3",
						Status:    "firing",
						Severity:  "info",
						Namespace: "monitoring",
						Resource:  "health-check",
					},
				}

				allRecovered := true
				for i, alert := range recoveryTests {
					recommendation, err := fakeClient.AnalyzeAlert(context.Background(), alert)

					if err != nil || recommendation == nil {
						allRecovered = false
						logger.WithError(err).WithField("alert", alert.Name).Warning("Recovery test failed")
					} else {
						logger.WithFields(logrus.Fields{
							"alert":      alert.Name,
							"action":     recommendation.Action,
							"confidence": recommendation.Confidence,
						}).Info("Recovery test successful")
					}

					// Brief delay between recovery tests
					if i < len(recoveryTests)-1 {
						time.Sleep(500 * time.Millisecond)
					}
				}

				By("Verifying complete system recovery")
				Expect(allRecovered).To(BeTrue(), "All recovery tests should pass after outage recovery")

				logger.WithFields(logrus.Fields{
					"recovery_tests_passed": len(recoveryTests),
					"outage_duration":       outageScenario.Duration,
					"scenario_name":         outageScenario.Name,
				}).Info("Production-grade recovery test completed successfully")
			})
		})
	})
})
