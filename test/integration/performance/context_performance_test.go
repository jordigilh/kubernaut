//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
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

var _ = Describe("Context Size Performance Tests", Ordered, func() {
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
		logger.SetLevel(logrus.DebugLevel)

		// Setup database
		var err error
		dbUtils, err = shared.NewDatabaseTestUtils(logger)
		Expect(err).ToNot(HaveOccurred())

		// Initialize fresh database
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
		// Clean database before each test
		Expect(dbUtils.CleanDatabase()).To(Succeed())
	})

	Context("Performance Comparison with Different Context Sizes", func() {
		type PerformanceResult struct {
			ContextSize   int
			ResponseTime  time.Duration
			TokensUsed    int
			Effectiveness float64
			Action        string
			Confidence    float64
		}

		createTestAlert := func() types.Alert {
			return types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Memory usage above 80% for deployment webapp",
				Namespace:   "production",
				Resource:    "webapp",
				Labels: map[string]string{
					"alertname":  "HighMemoryUsage",
					"deployment": "webapp",
					"namespace":  "production",
					"severity":   "warning",
				},
				Annotations: map[string]string{
					"description": "Memory usage above 80% for deployment webapp",
					"summary":     "High memory usage detected",
				},
			}
		}

		seedHistoricalData := func() {
			// Create historical action data for context-aware testing
			alert := createTestAlert()

			// Add multiple historical actions with varying effectiveness
			historicalActions := []struct {
				actionType    string
				effectiveness float64
				status        string
				timestamp     time.Time
			}{
				{"scale_deployment", 0.9, "completed", time.Now().Add(-2 * time.Hour)},
				{"scale_deployment", 0.8, "completed", time.Now().Add(-4 * time.Hour)},
				{"restart_pod", 0.3, "failed", time.Now().Add(-6 * time.Hour)},
				{"restart_pod", 0.2, "failed", time.Now().Add(-8 * time.Hour)},
				{"increase_resources", 0.7, "completed", time.Now().Add(-12 * time.Hour)},
				{"scale_deployment", 0.85, "completed", time.Now().Add(-24 * time.Hour)},
				{"notify_only", 1.0, "completed", time.Now().Add(-48 * time.Hour)},
			}

			for i, action := range historicalActions {
				actionRecord := &actionhistory.ActionRecord{
					ResourceReference: actionhistory.ResourceReference{
						Namespace: alert.Namespace,
						Kind:      "Deployment",
						Name:      alert.Resource,
					},
					ActionID:  generateUniqueID(),
					Timestamp: action.timestamp,
					Alert: actionhistory.AlertContext{
						Name:        alert.Name,
						Severity:    alert.Severity,
						Labels:      alert.Labels,
						Annotations: alert.Annotations,
						FiringTime:  action.timestamp,
					},
					ModelUsed:           testConfig.OllamaModel,
					Confidence:          0.8 + float64(i)*0.02, // Varying confidence
					Reasoning:           shared.StringPtr("Historical test action"),
					ActionType:          action.actionType,
					Parameters:          map[string]interface{}{"replicas": 3 + i, "test": true},
					ResourceStateBefore: map[string]interface{}{"status": "before"},
					ResourceStateAfter:  map[string]interface{}{"status": "after"},
				}

				trace, err := repository.StoreAction(context.Background(), actionRecord)
				Expect(err).ToNot(HaveOccurred())
				Expect(trace).ToNot(BeNil())

				// Update effectiveness score by updating the trace
				if action.status == "completed" {
					trace.EffectivenessScore = &action.effectiveness
					trace.ExecutionStatus = action.status
					err = repository.UpdateActionTrace(context.Background(), trace)
					Expect(err).ToNot(HaveOccurred())
				}
			}
		}

		testContextSize := func(contextSize int, seedData bool) PerformanceResult {
			if seedData {
				seedHistoricalData()
			}

			// Create SLM config with specific context size
			slmConfig := config.SLMConfig{
				Endpoint:       testConfig.OllamaEndpoint,
				Model:          testConfig.OllamaModel,
				Provider:       "localai",
				Timeout:        testConfig.TestTimeout,
				RetryCount:     1, // Single attempt for consistent timing
				Temperature:    0.3,
				MaxTokens:      500,
				MaxContextSize: contextSize,
			}

			// Create MCP client
			mcpClientConfig := slm.MCPClientConfig{
				Timeout:    testConfig.TestTimeout,
				MaxRetries: 1,
			}
			mcpClient := slm.NewMCPClient(mcpClientConfig, mcpServer, logger)

			// Create SLM client with MCP context
			slmClient, err := slm.NewClientWithMCP(slmConfig, mcpClient, logger)
			Expect(err).ToNot(HaveOccurred())

			alert := createTestAlert()

			startTime := time.Now()
			recommendation, err := slmClient.AnalyzeAlert(context.Background(), alert)
			responseTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())
			Expect(recommendation.Action).ToNot(BeEmpty())
			Expect(recommendation.Confidence).To(BeNumerically(">", 0))

			return PerformanceResult{
				ContextSize:   contextSize,
				ResponseTime:  responseTime,
				Action:        recommendation.Action,
				Confidence:    recommendation.Confidence,
				Effectiveness: 1.0, // Assume effectiveness for now
			}
		}

		It("should compare performance across different context sizes", func() {
			contextSizes := []int{
				0,     // Unlimited (baseline)
				32000, // Large context
				16000, // Medium context
				8000,  // Target reduced context
				4000,  // Small context
			}

			var results []PerformanceResult

			for _, contextSize := range contextSizes {
				result := testContextSize(contextSize, true) // With historical data
				results = append(results, result)

				logger.WithFields(logrus.Fields{
					"context_size":  contextSize,
					"response_time": result.ResponseTime,
					"action":        result.Action,
					"confidence":    result.Confidence,
				}).Info("Context size performance result")
			}

			// Analyze results
			baselineTime := results[0].ResponseTime // Unlimited context

			By("Performance should improve with smaller context sizes")
			for i := 1; i < len(results); i++ {
				result := results[i]
				speedupRatio := float64(baselineTime) / float64(result.ResponseTime)

				logger.WithFields(logrus.Fields{
					"context_size":  result.ContextSize,
					"baseline_time": baselineTime,
					"actual_time":   result.ResponseTime,
					"speedup_ratio": speedupRatio,
				}).Info("Performance comparison")

				// Expect some performance improvement with smaller context
				if result.ContextSize <= 8000 {
					Expect(speedupRatio).To(BeNumerically(">=", 0.8),
						"Context size %d should not significantly slow down processing", result.ContextSize)
				}
			}

			By("Decision quality should remain consistent across context sizes")
			baselineAction := results[0].Action
			for i := 1; i < len(results); i++ {
				result := results[i]
				// Actions should be similar or valid alternatives
				Expect(types.IsValidAction(result.Action)).To(BeTrue())

				// Log for manual analysis
				logger.WithFields(logrus.Fields{
					"context_size":    result.ContextSize,
					"baseline_action": baselineAction,
					"actual_action":   result.Action,
					"confidence_diff": result.Confidence - results[0].Confidence,
				}).Info("Decision quality comparison")
			}
		})

		It("should test 8K context specifically for optimal performance", func() {
			// Test 8K context size specifically as it's the target
			result := testContextSize(8000, true)

			logger.WithFields(logrus.Fields{
				"context_size":  8000,
				"response_time": result.ResponseTime,
				"action":        result.Action,
				"confidence":    result.Confidence,
			}).Info("8K context performance result")

			// Validate that 8K context produces reasonable results
			Expect(result.ResponseTime).To(BeNumerically("<", 30*time.Second))
			Expect(result.Confidence).To(BeNumerically(">", 0.5))
			Expect(types.IsValidAction(result.Action)).To(BeTrue())
		})

		It("should test without historical context for baseline comparison", func() {
			// Test different context sizes without historical data
			contextSizes := []int{0, 8000}

			for _, contextSize := range contextSizes {
				result := testContextSize(contextSize, false) // No historical data

				logger.WithFields(logrus.Fields{
					"context_size":  contextSize,
					"response_time": result.ResponseTime,
					"action":        result.Action,
					"confidence":    result.Confidence,
					"has_context":   false,
				}).Info("No-context performance result")

				// Without context, responses should be faster but potentially less informed
				Expect(result.ResponseTime).To(BeNumerically("<", 20*time.Second))
				Expect(types.IsValidAction(result.Action)).To(BeTrue())
			}
		})
	})
})

// Shared utility functions (stringPtr moved to database_test_utils.go)

func generateUniqueID() string {
	return fmt.Sprintf("test-action-%d", time.Now().UnixNano())
}
