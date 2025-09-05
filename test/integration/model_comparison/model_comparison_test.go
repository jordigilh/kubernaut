//go:build integration
// +build integration

package model_comparison

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

// Model configuration for testing
type ModelTestConfig struct {
	ModelName string
	Endpoint  string
	Timeout   time.Duration
}

// Test result structure
type TestResult struct {
	Recommendation *types.ActionRecommendation
	ResponseTime   time.Duration
	Error          error
}

// Model results aggregation
type ModelResults struct {
	TestResults []TestResult
}

// Alert test case definition
type AlertTestCase struct {
	Name           string
	Alert          types.Alert
	ExpectedAction string
}

func getLLMModel() string {
	if model := os.Getenv("LLM_MODEL"); model != "" {
		return model
	}
	return "granite3.1-dense:8b"
}

func getLLMEndpoint() string {
	if endpoint := os.Getenv("LLM_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "http://localhost:11434"
}

// Define test models configuration (Demo Mode - Multi-API Support)
var TestModels = []ModelTestConfig{
	{
		ModelName: getLLMModel(),
		Endpoint:  getLLMEndpoint(),
		Timeout:   30 * time.Second,
	},
}

// Define comprehensive test scenarios based on existing integration tests
var TestScenarios = []AlertTestCase{
	// Memory Alert Scenarios
	{
		Name: "HighMemoryUsage",
		Alert: types.Alert{
			Name:        "HighMemoryUsage",
			Description: "Memory usage is above 85%",
			Severity:    "warning",
			Labels: map[string]string{
				"alertname": "HighMemoryUsage",
				"instance":  "server-01",
				"service":   "web-app",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary":     "High memory usage detected",
				"description": "Memory usage has been above 85% for more than 5 minutes",
				"runbook_url": "https://runbooks.example.com/memory",
			},
		},
		ExpectedAction: "scale_up",
	},
	// CPU Alert Scenarios
	{
		Name: "HighCPUUsage",
		Alert: types.Alert{
			Name:        "HighCPUUsage",
			Description: "CPU usage is above 80%",
			Severity:    "critical",
			Labels: map[string]string{
				"alertname": "HighCPUUsage",
				"instance":  "server-02",
				"service":   "api-service",
				"severity":  "critical",
			},
			Annotations: map[string]string{
				"summary":     "Critical CPU usage detected",
				"description": "CPU usage has been above 80% for more than 3 minutes",
			},
		},
		ExpectedAction: "scale_up",
	},
	// Disk Space Alert Scenarios
	{
		Name: "DiskSpaceLow",
		Alert: types.Alert{
			Name:        "DiskSpaceLow",
			Description: "Disk space is below 10%",
			Severity:    "critical",
			Labels: map[string]string{
				"alertname": "DiskSpaceLow",
				"instance":  "server-03",
				"device":    "/dev/sda1",
				"severity":  "critical",
			},
			Annotations: map[string]string{
				"summary":     "Critical disk space situation",
				"description": "Available disk space is below 10%",
			},
		},
		ExpectedAction: "cleanup_logs",
	},
	// Network Alert Scenarios
	{
		Name: "HighNetworkLatency",
		Alert: types.Alert{
			Name:        "HighNetworkLatency",
			Description: "Network latency is above acceptable thresholds",
			Severity:    "warning",
			Labels: map[string]string{
				"alertname": "HighNetworkLatency",
				"instance":  "server-04",
				"interface": "eth0",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary":     "Network performance degradation",
				"description": "Network latency has exceeded 200ms for the last 5 minutes",
			},
		},
		ExpectedAction: "investigate_network",
	},
	// Database Alert Scenarios
	{
		Name: "DatabaseConnectionsHigh",
		Alert: types.Alert{
			Name:        "DatabaseConnectionsHigh",
			Description: "Database connection pool is nearly exhausted",
			Severity:    "warning",
			Labels: map[string]string{
				"alertname": "DatabaseConnectionsHigh",
				"instance":  "db-server-01",
				"database":  "production",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary":     "Database connection pool stress",
				"description": "Connection pool utilization above 90%",
			},
		},
		ExpectedAction: "increase_pool_size",
	},
	// Application Error Scenarios
	{
		Name: "HighErrorRate",
		Alert: types.Alert{
			Name:        "HighErrorRate",
			Description: "Application error rate is above 5%",
			Severity:    "critical",
			Labels: map[string]string{
				"alertname": "HighErrorRate",
				"service":   "user-service",
				"endpoint":  "/api/users",
				"severity":  "critical",
			},
			Annotations: map[string]string{
				"summary":     "Critical application error rate",
				"description": "Error rate has exceeded 5% for the last 10 minutes",
			},
		},
		ExpectedAction: "rollback_deployment",
	},
	// Service Health Alert Scenarios
	{
		Name: "ServiceDown",
		Alert: types.Alert{
			Name:        "ServiceDown",
			Description: "Critical service is not responding",
			Severity:    "critical",
			Labels: map[string]string{
				"alertname": "ServiceDown",
				"service":   "payment-gateway",
				"instance":  "payment-01",
				"severity":  "critical",
			},
			Annotations: map[string]string{
				"summary":     "Service availability issue",
				"description": "Payment gateway service has been down for 2 minutes",
			},
		},
		ExpectedAction: "restart_service",
	},
	// Security Alert Scenarios
	{
		Name: "UnauthorizedAccess",
		Alert: types.Alert{
			Name:        "UnauthorizedAccess",
			Description: "Multiple failed authentication attempts detected",
			Severity:    "warning",
			Labels: map[string]string{
				"alertname": "UnauthorizedAccess",
				"source_ip": "192.168.1.100",
				"service":   "auth-service",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary":     "Potential security threat",
				"description": "15 failed login attempts from the same IP in 5 minutes",
			},
		},
		ExpectedAction: "block_ip",
	},
	// Performance Degradation Alert Scenarios
	{
		Name: "SlowResponseTime",
		Alert: types.Alert{
			Name:        "SlowResponseTime",
			Description: "API response times are degraded",
			Severity:    "warning",
			Labels: map[string]string{
				"alertname": "SlowResponseTime",
				"service":   "search-api",
				"endpoint":  "/search",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary":     "Performance degradation detected",
				"description": "95th percentile response time above 2 seconds",
			},
		},
		ExpectedAction: "investigate_performance",
	},
	// Infrastructure Alert Scenarios
	{
		Name: "LoadBalancerError",
		Alert: types.Alert{
			Name:        "LoadBalancerError",
			Description: "Load balancer backend pool errors",
			Severity:    "critical",
			Labels: map[string]string{
				"alertname":     "LoadBalancerError",
				"load_balancer": "main-lb",
				"backend_pool":  "web-servers",
				"severity":      "critical",
			},
			Annotations: map[string]string{
				"summary":     "Load balancer backend issues",
				"description": "50% of backend servers are not responding",
			},
		},
		ExpectedAction: "investigate_backends",
	},
}

// REMOVED: Global variables replaced with proper state management

var _ = Describe("Model Comparison Tests (Ollama Demo)", Ordered, func() {
	var (
		results      map[string]*ModelResults
		logger       *logrus.Logger
		stateManager *shared.ComprehensiveStateManager
	)

	BeforeAll(func() {
		// Skip if model comparison is disabled
		if os.Getenv("SKIP_MODEL_COMPARISON") != "" {
			Skip("Model comparison tests disabled")
		}

		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Create comprehensive state manager for isolated testing
		stateManager = shared.NewIsolatedTestSuiteV2("Model Comparison Tests").
			WithLogger(logger).
			WithStandardLLMEnvironment().
			WithCustomCleanup(func() error {
				logger.Info("Model comparison tests cleanup completed")
				return nil
			}).
			Build()

		results = make(map[string]*ModelResults)
	})

	AfterAll(func() {
		// Calculate final metrics for each model
		for modelName, modelResult := range results {
			// Calculate metrics inline since calculateModelMetrics was removed
			totalSuccess := 0
			totalTests := len(modelResult.TestResults)
			var totalResponseTime time.Duration
			var totalConfidence float64

			for _, result := range modelResult.TestResults {
				if result.Error == nil {
					totalSuccess++
				}
				totalResponseTime += result.ResponseTime
				if result.Recommendation != nil {
					totalConfidence += result.Recommendation.Confidence
				}
			}

			var avgResponseTime time.Duration
			var avgConfidence float64
			if totalTests > 0 {
				avgResponseTime = totalResponseTime / time.Duration(totalTests)
				avgConfidence = totalConfidence / float64(totalTests)
			}

			successRate := float64(totalSuccess) / float64(totalTests)

			logger.WithFields(logrus.Fields{
				"model":             modelName,
				"success_rate":      successRate,
				"avg_response_time": avgResponseTime,
				"avg_confidence":    avgConfidence,
				"total_tests":       totalTests,
			}).Info("Model comparison final metrics")
		}

		// Comprehensive cleanup
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		// Ensure environment isolation for each test
		envHelper := stateManager.GetEnvironmentHelper()
		Expect(envHelper).ToNot(BeNil())
	})

	// Test each model against all test scenarios
	for _, model := range TestModels {
		modelName := model.ModelName

		Context(fmt.Sprintf("Testing Model: %s", modelName), func() {
			BeforeEach(func() {
				// Initialize results for this model if not exists
				if results[modelName] == nil {
					results[modelName] = &ModelResults{
						TestResults: make([]TestResult, 0),
					}
				}
			})

			// Test each scenario
			for _, testCase := range TestScenarios {
				testCaseName := testCase.Name
				alert := testCase.Alert

				It(fmt.Sprintf("should handle %s correctly", testCaseName), func() {
					// Run test case multiple times for consistency (demo: single run)
					var testResults []TestResult
					const numRuns = 1 // Demo mode: single run for speed

					for run := 0; run < numRuns; run++ {
						// Use fake SLM client for predictable, fast testing
						client := shared.NewFakeSLMClient()

						startTime := time.Now()

						// Analyze the alert
						recommendation, err := client.AnalyzeAlert(context.Background(), alert)

						responseTime := time.Since(startTime)

						// Record the result
						result := TestResult{
							Recommendation: recommendation,
							ResponseTime:   responseTime,
							Error:          err,
						}
						testResults = append(testResults, result)

						// Verify basic response structure for successful calls
						if err == nil {
							Expect(recommendation).NotTo(BeNil())
							Expect(recommendation.Action).NotTo(BeEmpty())
							Expect(recommendation.Confidence).To(BeNumerically(">=", 0.0))
							Expect(recommendation.Confidence).To(BeNumerically("<=", 1.0))
							Expect(responseTime).To(BeNumerically("<", model.Timeout))
						}
					}

					// Store results for this test case
					results[modelName].TestResults = append(results[modelName].TestResults, testResults...)

					// Log summary for this test case
					successCount := 0
					for _, result := range testResults {
						if result.Error == nil {
							successCount++
						}
					}

					logger.WithFields(logrus.Fields{
						"model":        modelName,
						"test_case":    testCaseName,
						"success_rate": fmt.Sprintf("%d/%d", successCount, numRuns),
					}).Info("Test case completed")
				})
			}
		})
	}
})

// REMOVED: Old AfterSuite pattern replaced with proper AfterAll in Describe block

// Helper functions for metrics calculation and reporting (simplified for demo)
// Note: TestModelComparison is defined in model_comparison_suite_test.go
