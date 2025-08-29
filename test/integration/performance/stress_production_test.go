//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"sync"
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

var _ = Describe("Stress Testing and Production Scenario Simulation", Ordered, func() {
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
			MaxContextSize: 2000,
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

	Context("High-Volume Stress Testing", func() {
		It("should handle burst of concurrent alerts reliably", func() {
			if testConfig.SkipSlowTests {
				Skip("Skipping slow stress test")
			}

			client := createSLMClient()

			// Create diverse alert scenarios for concurrent processing
			alertTemplates := []struct {
				name        string
				severity    string
				namespace   string
				resource    string
				description string
				labels      map[string]string
			}{
				{
					name: "HighMemoryUsage", severity: "warning", namespace: "production",
					resource: "web-service-1", description: "Memory usage above threshold",
					labels: map[string]string{"alertname": "HighMemoryUsage", "service": "web"},
				},
				{
					name: "HighCPUUsage", severity: "warning", namespace: "production",
					resource: "api-service-2", description: "CPU usage above threshold",
					labels: map[string]string{"alertname": "HighCPUUsage", "service": "api"},
				},
				{
					name: "DiskSpaceLow", severity: "critical", namespace: "database",
					resource: "postgres-storage", description: "Disk space critically low",
					labels: map[string]string{"alertname": "DiskSpaceLow", "storage": "primary"},
				},
				{
					name: "SecurityThreat", severity: "critical", namespace: "production",
					resource: "frontend-pod", description: "Security threat detected",
					labels: map[string]string{"alertname": "SecurityThreat", "threat": "intrusion"},
				},
				{
					name: "NetworkLatency", severity: "warning", namespace: "networking",
					resource: "ingress-controller", description: "High network latency",
					labels: map[string]string{"alertname": "NetworkLatency", "component": "ingress"},
				},
			}

			const concurrency = 10
			const alertsPerWorker = 3
			totalAlerts := concurrency * alertsPerWorker

			var wg sync.WaitGroup
			var mu sync.Mutex
			results := make([]struct {
				alertIndex   int
				action       string
				confidence   float64
				responseTime time.Duration
				error        error
			}, 0, totalAlerts)

			startTime := time.Now()

			// Launch concurrent workers
			for worker := 0; worker < concurrency; worker++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for alertIdx := 0; alertIdx < alertsPerWorker; alertIdx++ {
						templateIdx := (workerID*alertsPerWorker + alertIdx) % len(alertTemplates)
						template := alertTemplates[templateIdx]

						alert := types.Alert{
							Name:        template.name,
							Status:      "firing",
							Severity:    template.severity,
							Description: fmt.Sprintf("%s (worker:%d, alert:%d)", template.description, workerID, alertIdx),
							Namespace:   template.namespace,
							Resource:    fmt.Sprintf("%s-%d-%d", template.resource, workerID, alertIdx),
							Labels:      template.labels,
							Annotations: map[string]string{
								"worker": fmt.Sprintf("%d", workerID),
								"index":  fmt.Sprintf("%d", alertIdx),
							},
						}

						alertStart := time.Now()
						recommendation, err := client.AnalyzeAlert(context.Background(), alert)
						responseTime := time.Since(alertStart)

						mu.Lock()
						result := struct {
							alertIndex   int
							action       string
							confidence   float64
							responseTime time.Duration
							error        error
						}{
							alertIndex:   workerID*alertsPerWorker + alertIdx,
							responseTime: responseTime,
							error:        err,
						}

						if err == nil && recommendation != nil {
							result.action = recommendation.Action
							result.confidence = recommendation.Confidence
						}

						results = append(results, result)
						mu.Unlock()
					}
				}(worker)
			}

			wg.Wait()
			totalTime := time.Since(startTime)

			// Analyze results
			successCount := 0
			var totalResponseTime time.Duration
			var maxResponseTime time.Duration
			actionCounts := make(map[string]int)

			for _, result := range results {
				if result.error == nil {
					successCount++
					totalResponseTime += result.responseTime
					if result.responseTime > maxResponseTime {
						maxResponseTime = result.responseTime
					}
					actionCounts[result.action]++
				} else {
					logger.WithFields(logrus.Fields{
						"alert_index": result.alertIndex,
						"error":       result.error,
					}).Error("Alert processing failed in stress test")
				}
			}

			avgResponseTime := time.Duration(0)
			if successCount > 0 {
				avgResponseTime = totalResponseTime / time.Duration(successCount)
			}

			// Validate stress test results
			successRate := float64(successCount) / float64(totalAlerts)
			Expect(successRate).To(BeNumerically(">=", 0.95),
				"Success rate should be >= 95%%, got %.1f%%", successRate*100)

			Expect(avgResponseTime).To(BeNumerically("<", 15*time.Second),
				"Average response time should be < 15s, got %v", avgResponseTime)

			Expect(maxResponseTime).To(BeNumerically("<", 30*time.Second),
				"Max response time should be < 30s, got %v", maxResponseTime)

			// Validate action diversity (should not all be the same action)
			Expect(len(actionCounts)).To(BeNumerically(">=", 2),
				"Should produce diverse actions, got only: %v", actionCounts)

			logger.WithFields(logrus.Fields{
				"total_alerts":        totalAlerts,
				"success_count":       successCount,
				"success_rate":        fmt.Sprintf("%.1f%%", successRate*100),
				"total_time":          totalTime,
				"avg_response_time":   avgResponseTime,
				"max_response_time":   maxResponseTime,
				"action_distribution": actionCounts,
				"concurrency":         concurrency,
			}).Info("Stress test completed successfully")
		})

		It("should maintain performance with large historical context", func() {
			client := createSLMClient()

			// Seed large amount of historical data
			const historySize = 50
			for i := 0; i < historySize; i++ {
				actionRecord := &actionhistory.ActionRecord{
					ResourceReference: actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      "heavy-history-service",
					},
					ActionID:  fmt.Sprintf("history-action-%d", i),
					Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
					Alert: actionhistory.AlertContext{
						Name:        fmt.Sprintf("HistoryAlert-%d", i%5),
						Severity:    []string{"info", "warning", "critical"}[i%3],
						Labels:      map[string]string{"history": "true"},
						Annotations: map[string]string{"index": fmt.Sprintf("%d", i)},
						FiringTime:  time.Now().Add(-time.Duration(i) * time.Hour),
					},
					ModelUsed:           testConfig.OllamaModel,
					Confidence:          0.7 + float64(i%3)*0.1,
					Reasoning:           shared.StringPtr(fmt.Sprintf("Historical action %d", i)),
					ActionType:          []string{"scale_deployment", "restart_pod", "increase_resources", "notify_only"}[i%4],
					Parameters:          map[string]interface{}{"index": i},
					ResourceStateBefore: map[string]interface{}{"state": "before"},
					ResourceStateAfter:  map[string]interface{}{"state": "after"},
				}

				trace, err := repository.StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())

				// Vary effectiveness
				effectiveness := 0.5 + float64(i%5)*0.1
				trace.EffectivenessScore = &effectiveness
				trace.ExecutionStatus = []string{"completed", "failed"}[i%2]
				err = repository.UpdateActionTrace(context.Background(), trace)
				Expect(err).ToNot(HaveOccurred())
			}

			alert := types.Alert{
				Name:        "PerformanceWithHistory",
				Status:      "firing",
				Severity:    "warning",
				Description: "Test alert with extensive history",
				Namespace:   "production",
				Resource:    "heavy-history-service",
				Labels: map[string]string{
					"alertname": "PerformanceWithHistory",
					"test":      "heavy_context",
				},
				Annotations: map[string]string{
					"description": "Alert for testing performance with large context",
				},
			}

			// Measure performance with heavy context
			const iterations = 3
			var responseTimes []time.Duration

			for i := 0; i < iterations; i++ {
				startTime := time.Now()
				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				responseTime := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())
				Expect(types.IsValidAction(recommendation.Action)).To(BeTrue())

				responseTimes = append(responseTimes, responseTime)

				logger.WithFields(logrus.Fields{
					"iteration":     i + 1,
					"response_time": responseTime,
					"action":        recommendation.Action,
					"confidence":    recommendation.Confidence,
					"history_size":  historySize,
				}).Info("Heavy context performance test")
			}

			// Calculate average performance
			var totalTime time.Duration
			for _, rt := range responseTimes {
				totalTime += rt
			}
			avgResponseTime := totalTime / time.Duration(iterations)

			// Should still perform reasonably with large context (16K limit helps)
			Expect(avgResponseTime).To(BeNumerically("<", 20*time.Second),
				"Average response time with %d history records should be < 20s, got %v",
				historySize, avgResponseTime)

			logger.WithFields(logrus.Fields{
				"history_size":      historySize,
				"avg_response_time": avgResponseTime,
				"context_limit":     "16K",
			}).Info("Heavy context performance validation completed")
		})
	})

	Context("Real-World Production Scenarios", func() {
		It("should handle Black Friday traffic spike scenario", func() {
			client := createSLMClient()

			// Simulate progressive load increase during traffic spike
			trafficScenarios := []struct {
				stage       string
				severity    string
				description string
				resource    string
				metrics     map[string]string
			}{
				{
					stage: "early_increase", severity: "info",
					description: "Traffic starting to increase",
					resource:    "frontend-service",
					metrics:     map[string]string{"rps": "1000", "cpu": "60%", "memory": "65%"},
				},
				{
					stage: "moderate_load", severity: "warning",
					description: "Traffic significantly elevated",
					resource:    "frontend-service",
					metrics:     map[string]string{"rps": "2500", "cpu": "75%", "memory": "80%"},
				},
				{
					stage: "high_load", severity: "warning",
					description: "High traffic load detected",
					resource:    "frontend-service",
					metrics:     map[string]string{"rps": "4000", "cpu": "85%", "memory": "90%"},
				},
				{
					stage: "peak_load", severity: "critical",
					description: "Peak traffic - system under stress",
					resource:    "frontend-service",
					metrics:     map[string]string{"rps": "6000", "cpu": "95%", "memory": "95%"},
				},
			}

			var recommendations []*types.ActionRecommendation
			var escalationPattern []string

			for _, scenario := range trafficScenarios {
				alert := types.Alert{
					Name:        "TrafficSpike",
					Status:      "firing",
					Severity:    scenario.severity,
					Description: scenario.description,
					Namespace:   "production",
					Resource:    scenario.resource,
					Labels: map[string]string{
						"alertname": "TrafficSpike",
						"stage":     scenario.stage,
						"event":     "black_friday",
						"severity":  scenario.severity,
					},
					Annotations: map[string]string{
						"description":  scenario.description,
						"rps":          scenario.metrics["rps"],
						"cpu_usage":    scenario.metrics["cpu"],
						"memory_usage": scenario.metrics["memory"],
					},
				}

				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())

				recommendations = append(recommendations, recommendation)
				escalationPattern = append(escalationPattern, recommendation.Action)

				logger.WithFields(logrus.Fields{
					"stage":      scenario.stage,
					"severity":   scenario.severity,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
					"metrics":    scenario.metrics,
				}).Info("Traffic spike scenario progression")
			}

			// Validate escalation pattern
			// Early stages should prefer scaling
			earlyActions := escalationPattern[:2]
			for i, action := range earlyActions {
				Expect(action).To(BeElementOf([]string{
					"scale_deployment",
					"increase_resources",
					"notify_only",
				}), "Early stage %d should use scaling/resource actions, got %s", i+1, action)
			}

			// Peak load should be more conservative or escalate
			peakAction := escalationPattern[len(escalationPattern)-1]
			Expect(peakAction).To(BeElementOf([]string{
				"scale_deployment", // Still valid if confident
				"notify_only",      // Escalate to humans
				"collect_diagnostics",
			}), "Peak load should be conservative or escalate, got %s", peakAction)

			// Should show increasing urgency/awareness
			lastConfidence := recommendations[len(recommendations)-1].Confidence

			// Confidence patterns can vary, but should be reasonable
			Expect(lastConfidence).To(BeNumerically(">=", 0.5),
				"Even at peak load, should maintain reasonable confidence")

			logger.WithFields(logrus.Fields{
				"escalation_pattern": escalationPattern,
				"scenario":           "black_friday_traffic_spike",
			}).Info("Traffic spike scenario validation completed")
		})

		It("should handle database failure recovery scenario", func() {
			client := createSLMClient()

			// Simulate database failure and recovery decision chain
			dbFailureChain := []struct {
				stage       string
				alertName   string
				severity    string
				description string
				resource    string
				labels      map[string]string
			}{
				{
					stage: "connection_issues", alertName: "DatabaseSlow",
					severity: "warning", resource: "postgres-primary",
					description: "Database queries taking longer than usual",
					labels:      map[string]string{"db_type": "postgres", "role": "primary"},
				},
				{
					stage: "connection_failures", alertName: "DatabaseConnectionFailure",
					severity: "critical", resource: "postgres-primary",
					description: "Applications unable to connect to database",
					labels:      map[string]string{"db_type": "postgres", "error": "connection_refused"},
				},
				{
					stage: "service_impact", alertName: "ServiceDegraded",
					severity: "critical", resource: "user-service",
					description: "User service experiencing errors due to database",
					labels:      map[string]string{"service": "user", "cause": "database_failure"},
				},
				{
					stage: "cascade_prevention", alertName: "CascadingFailure",
					severity: "critical", resource: "application-cluster",
					description: "Multiple services failing due to database unavailability",
					labels:      map[string]string{"scope": "cluster", "cause": "database_cascade"},
				},
			}

			var scenarioResults []struct {
				stage     string
				action    string
				reasoning string
			}

			for _, scenario := range dbFailureChain {
				alert := types.Alert{
					Name:        scenario.alertName,
					Status:      "firing",
					Severity:    scenario.severity,
					Description: scenario.description,
					Namespace:   "production",
					Resource:    scenario.resource,
					Labels:      scenario.labels,
					Annotations: map[string]string{
						"stage":    scenario.stage,
						"incident": "database_failure",
					},
				}

				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())

				reasoningStr := ""
				if recommendation.Reasoning != nil {
					reasoningStr = recommendation.Reasoning.Summary
				}

				scenarioResults = append(scenarioResults, struct {
					stage     string
					action    string
					reasoning string
				}{
					stage:     scenario.stage,
					action:    recommendation.Action,
					reasoning: reasoningStr,
				})

				logger.WithFields(logrus.Fields{
					"stage":      scenario.stage,
					"alert":      scenario.alertName,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
					"reasoning":  reasoningStr,
				}).Info("Database failure scenario progression")
			}

			// Validate response progression
			// Early stage should focus on database itself
			firstStageAction := scenarioResults[0].action
			Expect(firstStageAction).To(BeElementOf([]string{
				"restart_pod",
				"collect_diagnostics",
				"notify_only",
				"rollback_deployment",
			}), "First stage should focus on database recovery")

			// Later stages should escalate or focus on containment
			laterStages := scenarioResults[2:]
			for _, result := range laterStages {
				Expect(result.action).To(BeElementOf([]string{
					"notify_only",
					"collect_diagnostics",
					"rollback_deployment",
					"quarantine_pod", // For containment
				}), "Later stages should escalate or contain, got %s in stage %s",
					result.action, result.stage)
			}

			logger.WithFields(logrus.Fields{
				"scenario_progression": scenarioResults,
				"scenario":             "database_failure_recovery",
			}).Info("Database failure scenario validation completed")
		})

		It("should handle security incident escalation correctly", func() {
			client := createSLMClient()

			// Simulate security incident escalation
			securityIncident := []struct {
				stage       string
				alertName   string
				severity    string
				threatLevel string
				description string
			}{
				{
					stage: "suspicious_activity", alertName: "AnomalousLogin",
					severity: "warning", threatLevel: "low",
					description: "Unusual login patterns detected",
				},
				{
					stage: "confirmed_intrusion", alertName: "UnauthorizedAccess",
					severity: "critical", threatLevel: "high",
					description: "Confirmed unauthorized access to system",
				},
				{
					stage: "data_access", alertName: "DataExfiltration",
					severity: "critical", threatLevel: "critical",
					description: "Suspected data exfiltration in progress",
				},
				{
					stage: "active_attack", alertName: "ActiveMalware",
					severity: "critical", threatLevel: "critical",
					description: "Active malware detected spreading through network",
				},
			}

			var securityActions []string
			var escalationLevels []string

			for _, scenario := range securityIncident {
				alert := types.Alert{
					Name:        scenario.alertName,
					Status:      "firing",
					Severity:    scenario.severity,
					Description: scenario.description,
					Namespace:   "production",
					Resource:    "security-target",
					Labels: map[string]string{
						"alertname":    scenario.alertName,
						"threat_level": scenario.threatLevel,
						"security":     "true",
						"incident":     "security_breach",
					},
					Annotations: map[string]string{
						"stage":       scenario.stage,
						"description": scenario.description,
						"threat_type": "intrusion",
					},
				}

				recommendation, err := client.AnalyzeAlert(context.Background(), alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation).ToNot(BeNil())

				securityActions = append(securityActions, recommendation.Action)
				escalationLevels = append(escalationLevels, scenario.threatLevel)

				logger.WithFields(logrus.Fields{
					"stage":        scenario.stage,
					"threat_level": scenario.threatLevel,
					"action":       recommendation.Action,
					"confidence":   recommendation.Confidence,
				}).Info("Security incident escalation")
			}

			// Validate security response pattern
			// All actions should be security-appropriate
			for i, action := range securityActions {
				Expect(action).To(BeElementOf([]string{
					"quarantine_pod",
					"notify_only",
					"collect_diagnostics",
				}), "Security incident stage %d should use security actions, got %s", i+1, action)

				// Should NOT use performance/scaling actions during security incident
				Expect(action).ToNot(BeElementOf([]string{
					"scale_deployment",
					"increase_resources",
					"restart_pod", // Unless for containment
				}), "Security incident should not use performance actions")
			}

			// Higher threat levels should prefer quarantine
			criticalIndices := []int{}
			for i, level := range escalationLevels {
				if level == "critical" {
					criticalIndices = append(criticalIndices, i)
				}
			}

			if len(criticalIndices) > 0 {
				criticalActions := []string{}
				for _, idx := range criticalIndices {
					criticalActions = append(criticalActions, securityActions[idx])
				}

				// At least some critical threats should trigger quarantine
				quarantineCount := 0
				for _, action := range criticalActions {
					if action == "quarantine_pod" {
						quarantineCount++
					}
				}

				Expect(quarantineCount).To(BeNumerically(">=", 1),
					"Critical security threats should trigger quarantine actions")
			}

			logger.WithFields(logrus.Fields{
				"security_actions":  securityActions,
				"escalation_levels": escalationLevels,
				"scenario":          "security_incident_escalation",
			}).Info("Security incident validation completed")
		})
	})
})
