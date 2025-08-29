//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/internal/mcp"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
)

var _ = Describe("Prompt Validation and Edge Case Testing", Ordered, func() {
	var (
		logger     *logrus.Logger
		dbUtils    *shared.DatabaseTestUtils
		mcpServer  *mcp.ActionHistoryMCPServer
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

		var err error
		dbUtils, err = shared.NewDatabaseTestUtils(logger)
		Expect(err).ToNot(HaveOccurred())

		Expect(dbUtils.InitializeFreshDatabase()).To(Succeed())
		repository = dbUtils.Repository
		mcpServer = dbUtils.MCPServer
	})

	AfterAll(func() {
		if dbUtils != nil {
			dbUtils.Close()
		}
	})

	BeforeEach(func() {
		Expect(dbUtils.CleanDatabase()).To(Succeed())
	})

	createSLMClient := func() slm.Client {
		slmConfig := config.SLMConfig{
			Endpoint:       testConfig.OllamaEndpoint,
			Model:          testConfig.OllamaModel,
			Provider:       "localai",
			Timeout:        testConfig.TestTimeout,
			RetryCount:     1,
			Temperature:    0.3,
			MaxTokens:      500,
			MaxContextSize: 0, // Use adaptive context sizing based on complexity
		}

		mcpClientConfig := slm.MCPClientConfig{
			Timeout:    testConfig.TestTimeout,
			MaxRetries: 1,
		}
		mcpClient := slm.NewMCPClient(mcpClientConfig, mcpServer, logger)

		slmClient, err := slm.NewClientWithMCP(slmConfig, mcpClient, logger)
		Expect(err).ToNot(HaveOccurred())
		return slmClient
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

			for _, pattern := range failurePatterns {
				for i := 0; i < pattern.failures; i++ {
					actionRecord := &actionhistory.ActionRecord{
						ResourceReference: actionhistory.ResourceReference{
							Namespace: "production",
							Kind:      "Deployment",
							Name:      "unstable-service",
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
						ModelUsed:           testConfig.OllamaModel,
						Confidence:          0.8,
						Reasoning:           stringPtr("Test failure pattern"),
						ActionType:          pattern.actionType,
						Parameters:          map[string]interface{}{"test": true},
						ResourceStateBefore: map[string]interface{}{"status": "degraded"},
						ResourceStateAfter:  map[string]interface{}{"status": "failed"},
					}

					trace, err := repository.StoreAction(context.Background(), actionRecord)
					Expect(err).ToNot(HaveOccurred())

					trace.EffectivenessScore = &pattern.effectiveness
					trace.ExecutionStatus = "failed"
					err = repository.UpdateActionTrace(context.Background(), trace)
					Expect(err).ToNot(HaveOccurred())
				}
			}

			alert := types.Alert{
				Name:        "SystemInstability",
				Status:      "firing",
				Severity:    "critical",
				Description: "System showing signs of instability across multiple components",
				Namespace:   "production",
				Resource:    "unstable-service",
				Labels: map[string]string{
					"alertname": "SystemInstability",
					"severity":  "critical",
					"pattern":   "multi_failure",
				},
				Annotations: map[string]string{
					"description": "Multiple failed remediation attempts detected",
					"escalation":  "required",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should strongly prefer escalation actions
			Expect(recommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
			}))

			// Should recognize the pattern and avoid failed action types
			Expect(recommendation.Action).ToNot(BeElementOf([]string{
				"restart_pod",
				"scale_deployment",
				"increase_resources",
			}))

			// Should show awareness of the problematic pattern in reasoning
			Expect(strings.ToLower(getReasoningSummary(recommendation.Reasoning))).To(SatisfyAny(
				ContainSubstring("multiple"),
				ContainSubstring("failed"),
				ContainSubstring("pattern"),
				ContainSubstring("escalat"),
				ContainSubstring("conservative"),
			))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("System instability escalation test")
		})

		It("should maintain action bias towards proven effective patterns", func() {
			client := createSLMClient()

			// Create strong success pattern for specific action type
			for i := 0; i < 7; i++ {
				actionRecord := &actionhistory.ActionRecord{
					ResourceReference: actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      "reliable-service",
					},
					ActionID:  fmt.Sprintf("success-%d", i),
					Timestamp: time.Now().Add(-time.Duration(i+1) * time.Hour),
					Alert: actionhistory.AlertContext{
						Name:        "HighMemoryUsage",
						Severity:    "warning",
						Labels:      map[string]string{"pattern": "success"},
						Annotations: map[string]string{"pattern": "success"},
						FiringTime:  time.Now().Add(-time.Duration(i+1) * time.Hour),
					},
					ModelUsed:           testConfig.OllamaModel,
					Confidence:          0.85 + float64(i)*0.01,
					Reasoning:           stringPtr("Test success pattern"),
					ActionType:          "scale_deployment",
					Parameters:          map[string]interface{}{"replicas": 3 + i},
					ResourceStateBefore: map[string]interface{}{"replicas": 2},
					ResourceStateAfter:  map[string]interface{}{"replicas": 3 + i},
				}

				trace, err := repository.StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				effectiveness := 0.88 + float64(i)*0.02 // Increasing effectiveness
				trace.EffectivenessScore = &effectiveness
				trace.ExecutionStatus = "completed"
				err = repository.UpdateActionTrace(context.Background(), trace)
				Expect(err).ToNot(HaveOccurred())
			}

			alert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Memory usage increasing for reliable service",
				Namespace:   "production",
				Resource:    "reliable-service",
				Labels: map[string]string{
					"alertname": "HighMemoryUsage",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"description": "Memory usage above threshold",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should strongly prefer the proven successful action
			Expect(recommendation.Action).To(Equal("scale_deployment"))

			// Should have high confidence due to proven track record
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))

			// Reasoning should reference historical success
			Expect(strings.ToLower(getReasoningSummary(recommendation.Reasoning))).To(SatisfyAny(
				ContainSubstring("historical"),
				ContainSubstring("effective"),
				ContainSubstring("successful"),
				ContainSubstring("proven"),
			))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Proven pattern bias test")
		})
	})

	Context("Edge Case Handling", func() {
		It("should handle conflicting severity and action urgency appropriately", func() {
			client := createSLMClient()

			// Low severity alert but critical resource
			alert := types.Alert{
				Name:        "MinorCPUIncrease",
				Status:      "firing",
				Severity:    "info", // Low severity
				Description: "Slight CPU increase in payment processing system",
				Namespace:   "production",
				Resource:    "payment-processor", // Critical resource
				Labels: map[string]string{
					"alertname":   "MinorCPUIncrease",
					"severity":    "info",
					"criticality": "high", // Conflicting signals
					"component":   "payment",
				},
				Annotations: map[string]string{
					"description":       "CPU usage increased from 45% to 55%",
					"impact":            "payment_processing",
					"business_critical": "true",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should be conservative due to critical nature despite low severity
			Expect(recommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"scale_deployment", // Acceptable if conservative scaling
			}))

			// Should NOT take aggressive actions for low severity
			Expect(recommendation.Action).ToNot(BeElementOf([]string{
				"restart_pod",
				"rollback_deployment",
			}))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Conflicting severity test")
		})

		It("should handle missing or malformed alert data gracefully", func() {
			client := createSLMClient()

			// Alert with minimal information
			alert := types.Alert{
				Name:        "", // Missing name
				Status:      "firing",
				Severity:    "unknown", // Invalid severity
				Description: "",        // Missing description
				Namespace:   "production",
				Resource:    "mystery-service",
				Labels:      map[string]string{}, // Empty labels
				Annotations: map[string]string{
					"source": "automated_system",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should be very conservative with incomplete information
			Expect(recommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
			}))

			// Confidence should be lower due to uncertainty
			Expect(recommendation.Confidence).To(BeNumerically("<=", 0.7))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Malformed alert test")
		})

		It("should recognize and handle oscillation risk appropriately", func() {
			client := createSLMClient()

			// Create oscillation pattern: scale up, scale down, scale up, scale down
			oscillationActions := []struct {
				actionType string
				timestamp  time.Time
				params     map[string]interface{}
			}{
				{"scale_deployment", time.Now().Add(-4 * time.Hour), map[string]interface{}{"replicas": 5}},
				{"scale_deployment", time.Now().Add(-3 * time.Hour), map[string]interface{}{"replicas": 2}},
				{"scale_deployment", time.Now().Add(-2 * time.Hour), map[string]interface{}{"replicas": 6}},
				{"scale_deployment", time.Now().Add(-1 * time.Hour), map[string]interface{}{"replicas": 3}},
			}

			for i, action := range oscillationActions {
				actionRecord := &actionhistory.ActionRecord{
					ResourceReference: actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      "oscillating-service",
					},
					ActionID:  fmt.Sprintf("oscillation-%d", i),
					Timestamp: action.timestamp,
					Alert: actionhistory.AlertContext{
						Name:        "ResourceOscillation",
						Severity:    "warning",
						Labels:      map[string]string{"pattern": "oscillation"},
						Annotations: map[string]string{"pattern": "oscillation"},
						FiringTime:  action.timestamp,
					},
					ModelUsed:           testConfig.OllamaModel,
					Confidence:          0.7,
					Reasoning:           stringPtr("Test oscillation pattern"),
					ActionType:          action.actionType,
					Parameters:          action.params,
					ResourceStateBefore: map[string]interface{}{"status": "fluctuating"},
					ResourceStateAfter:  map[string]interface{}{"status": "changed"},
				}

				trace, err := repository.StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				// Moderate effectiveness but creates instability
				effectiveness := 0.6
				trace.EffectivenessScore = &effectiveness
				trace.ExecutionStatus = "completed"
				err = repository.UpdateActionTrace(context.Background(), trace)
				Expect(err).ToNot(HaveOccurred())
			}

			alert := types.Alert{
				Name:        "ResourceOscillation",
				Status:      "firing",
				Severity:    "warning",
				Description: "Service showing oscillating behavior",
				Namespace:   "production",
				Resource:    "oscillating-service",
				Labels: map[string]string{
					"alertname": "ResourceOscillation",
					"severity":  "warning",
					"pattern":   "unstable",
				},
				Annotations: map[string]string{
					"description": "Repeated scaling actions detected",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should recognize oscillation and avoid making it worse
			Expect(recommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"increase_resources", // Alternative to scaling
			}))

			// Should NOT continue the oscillation pattern
			Expect(recommendation.Action).ToNot(Equal("scale_deployment"))

			// Should show awareness of oscillation risk
			Expect(strings.ToLower(getReasoningSummary(recommendation.Reasoning))).To(SatisfyAny(
				ContainSubstring("oscillat"),
				ContainSubstring("pattern"),
				ContainSubstring("instab"),
				ContainSubstring("conservat"),
			))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Oscillation risk test")
		})
	})

	Context("Security Alert Prioritization", func() {
		It("should prioritize containment over performance for security alerts", func() {
			client := createSLMClient()

			alert := types.Alert{
				Name:        "SecurityBreach",
				Status:      "firing",
				Severity:    "critical",
				Description: "Active security breach detected - unauthorized access attempt",
				Namespace:   "production",
				Resource:    "web-frontend",
				Labels: map[string]string{
					"alertname":     "SecurityBreach",
					"severity":      "critical",
					"threat_type":   "intrusion",
					"attack_vector": "web_exploit",
				},
				Annotations: map[string]string{
					"description":    "Suspicious activity patterns indicate active intrusion",
					"affected_users": "multiple",
					"data_at_risk":   "customer_data",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Security should prioritize containment/isolation
			Expect(recommendation.Action).To(BeElementOf([]string{
				"quarantine_pod",
				"notify_only", // For immediate human intervention
			}))

			// Should NOT try performance optimizations during security incident
			Expect(recommendation.Action).ToNot(BeElementOf([]string{
				"scale_deployment",
				"increase_resources",
			}))

			// Should have high confidence for security containment
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.7))

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Security prioritization test")
		})
	})

	Context("Resource Constraint Awareness", func() {
		It("should adapt recommendations based on implied resource constraints", func() {
			client := createSLMClient()

			// High-utilization scenario where scaling might not be viable
			alert := types.Alert{
				Name:        "HighResourceUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "High resource usage detected",
				Namespace:   "production",
				Resource:    "memory-intensive-app",
				Labels: map[string]string{
					"alertname":        "HighResourceUsage",
					"severity":         "warning",
					"cluster_capacity": "high", // Implicit constraint
					"memory_pressure":  "true",
				},
				Annotations: map[string]string{
					"description":     "Memory usage approaching cluster limits",
					"cluster_status":  "resource_constrained",
					"available_nodes": "limited",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should prefer resource optimization over scaling in constrained environment
			Expect(recommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"increase_resources", // Better than scaling when constrained
				"restart_pod",        // Might help with memory leaks
			}))

			// Might still suggest scaling but should be cautious
			if recommendation.Action == "scale_deployment" {
				Expect(recommendation.Confidence).To(BeNumerically("<=", 0.7))
			}

			logger.WithFields(logrus.Fields{
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"reasoning":  recommendation.Reasoning,
			}).Info("Resource constraint awareness test")
		})
	})
})
