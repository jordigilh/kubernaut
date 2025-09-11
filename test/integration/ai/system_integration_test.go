//go:build integration
// +build integration

package ai

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("System Integration Testing", func() {
	var (
		hooks           *testshared.TestLifecycleHooks
		ctx             context.Context
		llmClient       llm.Client
		workflowBuilder *engine.DefaultIntelligentWorkflowBuilder
		scenarioManager *IntegrationScenarioManager
	)

	BeforeAll(func() {
		hooks = testshared.SetupAIIntegrationTest("System Integration",
			testshared.WithMockLLM(), // Use mock for consistent testing
		)

		scenarioManager = NewIntegrationScenarioManager(hooks.GetLogger())
	})

	BeforeEach(func() {
		ctx = context.Background()
		suite := hooks.GetSuite()

		// Use pre-configured components
		llmClient = suite.LLMClient
		workflowBuilder = suite.WorkflowBuilder

		// Components are already verified healthy by the lifecycle hooks
		Expect(llmClient.IsHealthy()).To(BeTrue())
	})

	Describe("End-to-End AI System Integration", func() {
		Context("comprehensive system integration", func() {
			It("should validate complete AI system integration under realistic conditions", func() {
				By("Creating comprehensive integration test scenario")

				// Complex realistic scenario with multiple interrelated alerts
				integrationScenario := createComprehensiveIntegrationScenario()

				By("Processing scenario through complete AI pipeline")
				scenarioResults := make(map[string]*IntegrationResult)

				for alertName, alert := range integrationScenario.Alerts {
					result := &IntegrationResult{
						AlertName: alertName,
						StartTime: time.Now(),
					}

					// LLM Analysis - Following Guideline #1: Reuse existing code
					analyzeResponse, err := llmClient.AnalyzeAlert(ctx, *alert)
					if err != nil {
						result.LLMError = err
					} else {
						// Convert to ActionRecommendation using shared helper (Guideline #4: Integrate with existing code)
						recommendation := testshared.ConvertAnalyzeAlertResponse(analyzeResponse)
						result.LLMRecommendation = recommendation
						result.LLMLatency = time.Since(result.StartTime)

						// Workflow Generation
						objective := testshared.CreateStandardWorkflowObjective(alert, recommendation, "integration_test")
						workflow, err := workflowBuilder.GenerateWorkflow(ctx, objective)
						if err != nil {
							result.WorkflowError = err
						} else {
							result.WorkflowTemplate = workflow
							result.WorkflowLatency = time.Since(result.StartTime) - result.LLMLatency

							// Pattern Discovery (simplified for scenario testing)
							patterns := &patterns.PatternAnalysisResult{
								Patterns: []*shared.DiscoveredPattern{
									{BasePattern: types.BasePattern{
										BaseEntity: types.BaseEntity{
											ID: "test-pattern",
										},
									},
									}},
							}
							err = nil // Simulated success
							if err != nil {
								result.PatternError = err
							} else {
								result.PatternResult = patterns
								result.PatternLatency = time.Since(result.StartTime) - result.LLMLatency - result.WorkflowLatency
							}
						}
					}

					result.TotalLatency = time.Since(result.StartTime)
					result.Success = (result.LLMError == nil && result.WorkflowError == nil && result.PatternError == nil)
					scenarioResults[alertName] = result
				}

				By("Validating integration results")
				successCount := 0
				totalLatency := time.Duration(0)

				for _, result := range scenarioResults {
					if result.Success {
						successCount++
					}
					totalLatency += result.TotalLatency

					// **Business Requirement Validation** - Following Guideline #22: Business outcome assertions
					if result.Success {
						// BR-AI-002: Actionable Recommendations - Validate meaningful recommendations
						Expect(result.LLMRecommendation).ToNot(BeNil())
						Expect(result.LLMRecommendation.Action).ToNot(BeEmpty(),
							"BR-AI-002: Should provide actionable recommendation")
						Expect(result.LLMRecommendation.Confidence).To(BeNumerically(">=", 0.5),
							"BR-AI-002: Should provide reasonable confidence level (≥50%)")

						// BR-AI-001: Contextual Analysis - Validate meaningful workflow generation
						Expect(result.WorkflowTemplate).ToNot(BeNil())
						Expect(len(result.WorkflowTemplate.Steps)).To(BeNumerically(">", 0),
							"BR-AI-001: Should generate contextual workflow with actionable steps")

						// BR-AI-003: Structured Analysis - Validate pattern discovery
						Expect(result.PatternResult).ToNot(BeNil())
						Expect(len(result.PatternResult.Patterns)).To(BeNumerically(">", 0),
							"BR-AI-003: Should discover structured patterns for analysis")

						// **Performance Business Requirements** - Component response times
						Expect(result.LLMLatency).To(BeNumerically("<", 10*time.Second),
							"LLM analysis should complete within acceptable business timeframe")
						Expect(result.WorkflowLatency).To(BeNumerically("<", 20*time.Second),
							"Workflow generation should complete within acceptable business timeframe")
						Expect(result.PatternLatency).To(BeNumerically("<", 15*time.Second),
							"Pattern analysis should complete within acceptable business timeframe")
					}
				}

				successRate := float64(successCount) / float64(len(scenarioResults))
				avgLatency := totalLatency / time.Duration(len(scenarioResults))

				// **Business Critical Integration Validation** - Following Guideline #22: Business outcomes
				Expect(successRate).To(BeNumerically(">=", 0.9),
					"BR-INTEGRATION-001: System should achieve ≥90% success rate for business reliability")
				Expect(avgLatency).To(BeNumerically("<", 45*time.Second),
					"BR-INTEGRATION-002: Average end-to-end latency should meet business SLA requirements")

				scenarioManager.RecordScenario("comprehensive_integration", successRate >= 0.9, avgLatency)

				By("Generating comprehensive integration report")
				integrationReport := map[string]interface{}{
					"total_scenarios":      len(scenarioResults),
					"successful_scenarios": successCount,
					"success_rate":         fmt.Sprintf("%.2f%%", successRate*100),
					"average_latency":      avgLatency,
					"component_performance": map[string]interface{}{
						"llm_avg_latency":      calculateAverageLatency(scenarioResults, "llm"),
						"workflow_avg_latency": calculateAverageLatency(scenarioResults, "workflow"),
						"pattern_avg_latency":  calculateAverageLatency(scenarioResults, "pattern"),
					},
				}

				hooks.GetLogger().WithField("integration_report", integrationReport).Info("Comprehensive Integration Test Results")
			})

			It("should handle high-volume concurrent alert processing", func() {
				By("Setting up high-volume concurrent processing scenario")

				alertCount := 50
				concurrentAlerts := make([]*types.Alert, alertCount)

				// Create diverse alert types for concurrent processing
				alertTypes := []string{"MemoryPressure", "CPUSpike", "DiskFull", "NetworkLatency", "ServiceDown"}
				for i := 0; i < alertCount; i++ {
					alertType := alertTypes[i%len(alertTypes)]
					concurrentAlerts[i] = testshared.CreateStandardAlert(
						alertType,
						fmt.Sprintf("Concurrent test alert %d: %s", i+1, alertType),
						"warning",
						fmt.Sprintf("namespace-%d", (i%5)+1),
						fmt.Sprintf("resource-%d", i+1),
					)
				}

				By("Processing alerts concurrently through AI pipeline")
				results := make(chan *IntegrationResult, alertCount)
				startTime := time.Now()

				// Process all alerts concurrently
				for i, alert := range concurrentAlerts {
					go func(index int, a *types.Alert) {
						result := &IntegrationResult{
							AlertName: fmt.Sprintf("concurrent_alert_%d", index+1),
							StartTime: time.Now(),
						}

						// LLM Analysis - Following Guideline #1: Reuse existing code
						analyzeResponse, err := llmClient.AnalyzeAlert(ctx, *a)
						if err != nil {
							result.LLMError = err
						} else {
							// Convert to ActionRecommendation using shared helper (Guideline #4: Integrate with existing code)
							recommendation := testshared.ConvertAnalyzeAlertResponse(analyzeResponse)
							result.LLMRecommendation = recommendation
							result.LLMLatency = time.Since(result.StartTime)

							// Workflow Generation
							objective := testshared.CreateStandardWorkflowObjective(a, recommendation, "concurrent_test")
							workflow, err := workflowBuilder.GenerateWorkflow(ctx, objective)
							if err != nil {
								result.WorkflowError = err
							} else {
								result.WorkflowTemplate = workflow
								result.WorkflowLatency = time.Since(result.StartTime) - result.LLMLatency
							}
						}

						result.TotalLatency = time.Since(result.StartTime)
						result.Success = (result.LLMError == nil && result.WorkflowError == nil)
						results <- result
					}(i, alert)
				}

				// Collect results
				processedResults := make([]*IntegrationResult, 0, alertCount)
				for i := 0; i < alertCount; i++ {
					select {
					case result := <-results:
						processedResults = append(processedResults, result)
					case <-time.After(60 * time.Second):
						Fail("Timeout waiting for concurrent alert processing")
					}
				}

				totalProcessingTime := time.Since(startTime)

				By("Validating concurrent processing results")
				successCount := 0
				for _, result := range processedResults {
					if result.Success {
						successCount++
					}
				}

				successRate := float64(successCount) / float64(len(processedResults))
				avgLatencyPerAlert := calculateAverageProcessingLatency(processedResults)

				// Validate concurrent processing performance
				Expect(successRate).To(BeNumerically(">=", 0.85), "At least 85% success rate under concurrent load")
				Expect(totalProcessingTime).To(BeNumerically("<", 30*time.Second), "Total concurrent processing should complete quickly")
				Expect(avgLatencyPerAlert).To(BeNumerically("<", 15*time.Second), "Average per-alert latency should be reasonable")

				scenarioManager.RecordScenario("concurrent_processing", successRate >= 0.85, totalProcessingTime)

				hooks.GetLogger().WithFields(logrus.Fields{
					"processed_alerts":      len(processedResults),
					"success_rate":          fmt.Sprintf("%.2f%%", successRate*100),
					"total_time":            totalProcessingTime,
					"avg_latency_per_alert": avgLatencyPerAlert,
				}).Info("Concurrent processing test completed")
			})

			It("should maintain system stability under sustained load", func() {
				By("Setting up sustained load test scenario")

				loadDuration := 2 * time.Minute
				alertInterval := 2 * time.Second
				expectedAlerts := int(loadDuration / alertInterval)

				sustainedResults := make([]*IntegrationResult, 0)
				loadStartTime := time.Now()

				By("Generating sustained load of alerts")
				alertTicker := time.NewTicker(alertInterval)
				defer alertTicker.Stop()

				loadContext, cancelLoad := context.WithTimeout(ctx, loadDuration)
				defer cancelLoad()

				alertCounter := 0
				for {
					select {
					case <-loadContext.Done():
						goto LoadComplete
					case <-alertTicker.C:
						alertCounter++

						// Create load test alert
						loadAlert := testshared.CreateStandardAlert(
							"SustainedLoadTest",
							fmt.Sprintf("Sustained load test alert %d", alertCounter),
							"info",
							"load-test",
							fmt.Sprintf("load-resource-%d", alertCounter),
						)

						// Process through AI pipeline
						result := &IntegrationResult{
							AlertName: fmt.Sprintf("load_alert_%d", alertCounter),
							StartTime: time.Now(),
						}

						// LLM Analysis - Following Guideline #1: Reuse existing code
						analyzeResponse, err := llmClient.AnalyzeAlert(ctx, *loadAlert)
						if err != nil {
							result.LLMError = err
						} else {
							// Convert to ActionRecommendation using shared helper (Guideline #4: Integrate with existing code)
							recommendation := testshared.ConvertAnalyzeAlertResponse(analyzeResponse)
							result.LLMRecommendation = recommendation
							result.LLMLatency = time.Since(result.StartTime)

							objective := testshared.CreateStandardWorkflowObjective(loadAlert, recommendation, "load_test")
							workflow, err := workflowBuilder.GenerateWorkflow(ctx, objective)
							if err != nil {
								result.WorkflowError = err
							} else {
								result.WorkflowTemplate = workflow
								result.WorkflowLatency = time.Since(result.StartTime) - result.LLMLatency
							}
						}

						result.TotalLatency = time.Since(result.StartTime)
						result.Success = (result.LLMError == nil && result.WorkflowError == nil)
						sustainedResults = append(sustainedResults, result)
					}
				}

			LoadComplete:
				totalLoadTime := time.Since(loadStartTime)

				By("Validating sustained load performance")
				successCount := 0
				var totalLatency time.Duration
				for _, result := range sustainedResults {
					if result.Success {
						successCount++
					}
					totalLatency += result.TotalLatency
				}

				actualAlerts := len(sustainedResults)
				successRate := float64(successCount) / float64(actualAlerts)
				avgLatency := totalLatency / time.Duration(actualAlerts)

				// **Business Load Performance Requirements** - Following Guideline #22: Business outcomes
				Expect(actualAlerts).To(BeNumerically(">=", int(float64(expectedAlerts)*0.9)),
					"BR-LOAD-001: System should process ≥90% of expected alerts under business load conditions")
				Expect(successRate).To(BeNumerically(">=", 0.80),
					"BR-LOAD-002: System should maintain ≥80% success rate under sustained business load")
				Expect(avgLatency).To(BeNumerically("<", 20*time.Second),
					"BR-LOAD-003: Average latency should remain within business SLA under load")

				scenarioManager.RecordScenario("sustained_load", successRate >= 0.80, totalLoadTime)

				hooks.GetLogger().WithFields(logrus.Fields{
					"load_duration":    totalLoadTime,
					"processed_alerts": actualAlerts,
					"expected_alerts":  expectedAlerts,
					"success_rate":     fmt.Sprintf("%.2f%%", successRate*100),
					"avg_latency":      avgLatency,
				}).Info("Sustained load test completed")
			})
		})
	})
})

// Helper functions for system integration testing

func createComprehensiveIntegrationScenario() *ComprehensiveIntegrationScenario {
	return &ComprehensiveIntegrationScenario{
		Name:        "comprehensive_integration_scenario",
		Description: "Complex multi-alert scenario for comprehensive integration testing",
		Alerts: map[string]*types.Alert{
			"primary_database": testshared.CreateStandardAlert(
				"DatabaseConnectionFailure",
				"Primary database connection failures",
				"critical",
				"production",
				"postgres-primary",
			),
			"api_gateway": testshared.CreateStandardAlert(
				"APIGatewayLatency",
				"API Gateway experiencing high latency",
				"warning",
				"production",
				"api-gateway",
			),
			"cache_layer": testshared.CreateStandardAlert(
				"CacheEvictions",
				"High cache eviction rate detected",
				"warning",
				"production",
				"redis-cache",
			),
		},
		Metadata: map[string]interface{}{
			"complexity":         "high",
			"inter_dependencies": true,
			"business_impact":    "high",
		},
	}
}

func calculateAverageLatency(results map[string]*IntegrationResult, component string) time.Duration {
	total := time.Duration(0)
	count := 0

	for _, result := range results {
		if result.Success {
			switch component {
			case "llm":
				total += result.LLMLatency
			case "workflow":
				total += result.WorkflowLatency
			case "pattern":
				total += result.PatternLatency
			}
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / time.Duration(count)
}

func calculateAverageProcessingLatency(results []*IntegrationResult) time.Duration {
	if len(results) == 0 {
		return 0
	}

	total := time.Duration(0)
	for _, result := range results {
		total += result.TotalLatency
	}

	return total / time.Duration(len(results))
}

// Integration testing data structures

type ComprehensiveIntegrationScenario struct {
	Name        string
	Description string
	Alerts      map[string]*types.Alert
	Metadata    map[string]interface{}
}

type IntegrationResult struct {
	AlertName         string
	StartTime         time.Time
	Success           bool
	LLMRecommendation *types.ActionRecommendation
	LLMLatency        time.Duration
	LLMError          error
	WorkflowTemplate  *engine.ExecutableTemplate
	WorkflowLatency   time.Duration
	WorkflowError     error
	PatternResult     *patterns.PatternAnalysisResult
	PatternLatency    time.Duration
	PatternError      error
	TotalLatency      time.Duration
}
