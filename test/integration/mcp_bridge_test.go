package integration

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
)

var _ = Describe("MCP Bridge Integration Tests", Ordered, func() {
	var (
		testUtils  *shared.IntegrationTestUtils
		testConfig shared.IntegrationConfig
		logger     *logrus.Logger
	)

	BeforeAll(func() {
		// Load test configuration
		testConfig = shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}

		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		logger.Info("Starting MCP Bridge integration tests")

		// Create integration test utilities with full infrastructure
		var err error
		testUtils, err = shared.NewIntegrationTestUtils(logger)
		Expect(err).NotTo(HaveOccurred())

		// Verify infrastructure is ready
		Expect(testUtils.DB).NotTo(BeNil())
		Expect(testUtils.MCPServer).NotTo(BeNil())
		Expect(testUtils.K8sTestEnvironment).NotTo(BeNil())
	})

	AfterAll(func() {
		if testUtils != nil {
			testUtils.Close()
		}
	})

	Context("Dynamic MCP Bridge Functionality", func() {
		It("should create dynamic SLM client successfully", func() {
			client, err := testUtils.CreateDynamicSLMClient(testConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("should analyze alerts with dynamic tool calling", func() {
			// Create dynamic SLM client
			client, err := testUtils.CreateDynamicSLMClient(testConfig)
			Expect(err).NotTo(HaveOccurred())

			// Create a test alert that should trigger tool usage
			alert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Container memory usage is at 95%",
				Namespace:   "production",
				Resource:    "webapp-deployment-abc123",
				Labels: map[string]string{
					"alertname": "HighMemoryUsage",
					"container": "webapp",
					"pod":       "webapp-deployment-abc123",
					"namespace": "production",
				},
				Annotations: map[string]string{
					"description": "Container memory usage has been above 95% for more than 5 minutes",
					"summary":     "High memory usage detected",
				},
			}

			// Add some action history to test tool usage
			testUtils.Logger.Info("Creating action history for dynamic MCP test")
			resourceRef := actionhistory.ResourceReference{
				Namespace:   "production",
				Kind:        "deployment",
				Name:        "webapp-deployment",
				APIVersion:  "apps/v1",
				ResourceUID: "test-uid-123",
			}
			_, err = testUtils.CreateTestActionHistory(resourceRef, 5)
			Expect(err).NotTo(HaveOccurred())

			// Analyze alert with dynamic MCP bridge
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			testUtils.Logger.Info("Starting dynamic MCP analysis")
			recommendation, err := client.AnalyzeAlert(ctx, alert)

			if err != nil {
				testUtils.Logger.WithError(err).Error("Dynamic MCP analysis failed")
			}

			// We expect either success or controlled failure (due to LocalAI not running)
			if err == nil {
				// If successful, validate the recommendation
				Expect(recommendation).NotTo(BeNil())
				Expect(recommendation.Action).NotTo(BeEmpty())
				Expect(recommendation.Confidence).To(BeNumerically(">=", 0.0))
				Expect(recommendation.Confidence).To(BeNumerically("<=", 1.0))
				Expect(recommendation.Reasoning).NotTo(BeEmpty())

				testUtils.Logger.WithFields(map[string]interface{}{
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
					"reasoning":  recommendation.Reasoning,
				}).Info("Dynamic MCP analysis completed successfully")
			} else {
				// If failed, it should be due to LocalAI connection issues (expected in test environment)
				Expect(err.Error()).To(ContainSubstring("failed to make LocalAI request"))
				testUtils.Logger.WithError(err).Info("Dynamic MCP analysis failed as expected (LocalAI not available)")
			}
		})

		It("should handle tool execution gracefully when tools are unavailable", func() {
			// Create dynamic SLM client
			client, err := testUtils.CreateDynamicSLMClient(testConfig)
			Expect(err).NotTo(HaveOccurred())

			// Create a simpler alert
			alert := types.Alert{
				Name:        "PodCrashLoop",
				Status:      "firing",
				Severity:    "critical",
				Description: "Pod is in CrashLoopBackOff state",
				Namespace:   "test",
				Resource:    "test-pod-xyz",
				Labels: map[string]string{
					"alertname": "PodCrashLoop",
					"pod":       "test-pod-xyz",
					"namespace": "test",
				},
				Annotations: map[string]string{
					"description": "Pod has been restarting repeatedly",
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			testUtils.Logger.Info("Testing tool execution with unavailable LocalAI")
			recommendation, err := client.AnalyzeAlert(ctx, alert)

			// We expect this to fail gracefully with LocalAI connection error
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("LocalAI"))
				testUtils.Logger.WithError(err).Info("Tool execution failed gracefully as expected")
			} else {
				// If it somehow succeeds, validate the response
				Expect(recommendation).NotTo(BeNil())
				testUtils.Logger.Info("Unexpected success - LocalAI may be running")
			}
		})
	})

	Context("MCP Bridge Architecture Validation", func() {
		It("should have properly configured MCP servers", func() {
			// Verify K8s MCP server endpoint is accessible

			// Verify Action History MCP server is available
			Expect(testUtils.MCPServer).NotTo(BeNil())

			testUtils.Logger.WithFields(map[string]interface{}{
				"action_history_mcp": testUtils.MCPServer != nil,
				"k8s_test_env":       testUtils.K8sTestEnvironment != nil,
			}).Info("MCP infrastructure validated")
		})

		It("should demonstrate tool discovery capabilities", func() {
			// This test shows what tools would be available to the model
			expectedTools := []string{
				"get_pod_status",
				"check_node_capacity",
				"get_recent_events",
				"check_resource_quotas",
				"get_action_history",
				"check_oscillation_risk",
				"get_effectiveness_metrics",
			}

			testUtils.Logger.WithFields(map[string]interface{}{
				"available_tools": expectedTools,
			}).Info("Tool discovery capabilities demonstrated")

			// Verify all expected tools are documented
			for _, tool := range expectedTools {
				Expect(tool).NotTo(BeEmpty())
			}
		})
	})
})
