package model_comparison_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// TestResult captures the results of running a test case against a model
type TestResult struct {
	TestCase       AlertTestCase               `json:"test_case"`
	Recommendation *types.ActionRecommendation `json:"recommendation"`
	ResponseTime   time.Duration               `json:"response_time"`
	Error          error                       `json:"error,omitempty"`
	Timestamp      time.Time                   `json:"timestamp"`
}

// ModelResults aggregates results for a single model across all test cases
type ModelResults struct {
	ModelName   string             `json:"model_name"`
	TestResults []TestResult       `json:"test_results"`
	Performance PerformanceMetrics `json:"performance"`
	Reasoning   ReasoningMetrics   `json:"reasoning"`
	Summary     ModelSummary       `json:"summary"`
}

type AlertTestCase struct {
	Name               string      `json:"name"`
	Alert              types.Alert `json:"alert"`
	ExpectedAction     string      `json:"expected_action"`
	ExpectedConfidence float64     `json:"expected_confidence"`
	Reasoning          string      `json:"reasoning"`
	AlternativeActions []string    `json:"alternative_actions,omitempty"`
	Category           string      `json:"category"`
}

type ModelTestConfig struct {
	ModelName        string        `json:"model_name"`
	ServerType       string        `json:"server_type"` // "ramallama" or "vllm"
	Endpoint         string        `json:"endpoint"`
	MaxTokens        int           `json:"max_tokens"`
	Temperature      float64       `json:"temperature"`
	Timeout          time.Duration `json:"timeout"`
	Description      string        `json:"description"`
	ExpectedStrength string        `json:"expected_strength"`
}

type PerformanceMetrics struct {
	ResponseTime   PerformanceData `json:"response_time"`
	Throughput     float64         `json:"throughput"` // requests per minute
	ErrorRate      float64         `json:"error_rate"`
	TotalRequests  int             `json:"total_requests"`
	SuccessfulRuns int             `json:"successful_runs"`
}

type PerformanceData struct {
	Mean   time.Duration `json:"mean"`
	P50    time.Duration `json:"p50"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
	Min    time.Duration `json:"min"`
	Max    time.Duration `json:"max"`
	StdDev time.Duration `json:"stddev"`
}

type ReasoningMetrics struct {
	ActionAccuracy         float64         `json:"action_accuracy"`        // % correct actions
	ConfidenceCalibration  float64         `json:"confidence_calibration"` // How well confidence matches accuracy
	ReasoningQuality       ReasoningScores `json:"reasoning_quality"`
	ConsistencyScore       float64         `json:"consistency_score"`        // Same input → same output
	AlternativeActionMatch float64         `json:"alternative_action_match"` // % acceptable alternatives
}

type ReasoningScores struct {
	Relevance      float64 `json:"relevance"`       // Reasoning mentions relevant factors
	Completeness   float64 `json:"completeness"`    // Covers key decision factors
	Clarity        float64 `json:"clarity"`         // Clear, understandable explanation
	TechnicalDepth float64 `json:"technical_depth"` // Demonstrates K8s understanding
	Overall        float64 `json:"overall"`
}

type ModelSummary struct {
	TotalTestCases    int     `json:"total_test_cases"`
	SuccessfulCases   int     `json:"successful_cases"`
	FailedCases       int     `json:"failed_cases"`
	AverageConfidence float64 `json:"average_confidence"`
	OverallRating     string  `json:"overall_rating"` // "Excellent", "Good", "Fair", "Poor"
	Recommendation    string  `json:"recommendation"`
}

// Define test models configuration (Demo Mode - Ollama)
var TestModels = []ModelTestConfig{
	{
		ModelName:        "granite3.1-dense:8b",
		ServerType:       "ollama",
		Endpoint:         "http://localhost:11434",
		MaxTokens:        500,
		Temperature:      0.3,
		Timeout:          30 * time.Second,
		Description:      "Current production baseline model (8B parameters)",
		ExpectedStrength: "general_kubernetes_reasoning",
	},
	{
		ModelName:        "deepseek-coder:6.7b",
		ServerType:       "ollama",
		Endpoint:         "http://localhost:11434",
		MaxTokens:        500,
		Temperature:      0.3,
		Timeout:          30 * time.Second,
		Description:      "DeepSeek code reasoning specialist (6.7B parameters)",
		ExpectedStrength: "kubernetes_troubleshooting",
	},
	{
		ModelName:        "granite3.3:2b",
		ServerType:       "ollama",
		Endpoint:         "http://localhost:11434",
		MaxTokens:        500,
		Temperature:      0.3,
		Timeout:          30 * time.Second,
		Description:      "Newer granite model (2B parameters)",
		ExpectedStrength: "efficiency_and_speed",
	},
}

// Define comprehensive test scenarios based on existing integration tests
var TestScenarios = []AlertTestCase{
	// Memory Alert Scenarios
	{
		Name: "HighMemoryUsage_WebApp_Production",
		Alert: types.Alert{
			Name:        "HighMemoryUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod is using 95% of memory limit",
			Namespace:   "production",
			Resource:    "web-app-pod-123",
			Labels: map[string]string{
				"alertname": "HighMemoryUsage",
				"app":       "webapp",
				"severity":  "warning",
				"namespace": "production",
			},
			Annotations: map[string]string{
				"description": "Pod is using 95% of memory limit",
				"summary":     "High memory usage detected",
			},
		},
		ExpectedAction:     "increase_resources",
		ExpectedConfidence: 0.85,
		Reasoning:          "Memory pressure should trigger resource increase",
		AlternativeActions: []string{"restart_pod", "scale_deployment"},
		Category:           "memory",
	},
	{
		Name: "OOMKilled_CrashLoop_Production",
		Alert: types.Alert{
			Name:        "PodCrashLoopBackOff",
			Status:      "firing",
			Severity:    "critical",
			Description: "Pod in CrashLoopBackOff state",
			Namespace:   "production",
			Resource:    "api-service-pod-456",
			Labels: map[string]string{
				"alertname": "PodCrashLoopBackOff",
				"app":       "api-service",
				"reason":    "OOMKilled",
			},
			Annotations: map[string]string{
				"termination_reason": "OOMKilled",
				"description":        "Container killed due to OutOfMemory",
			},
		},
		ExpectedAction:     "increase_resources",
		ExpectedConfidence: 0.9,
		Reasoning:          "OOMKilled should NEVER use scale_deployment, always increase_resources",
		AlternativeActions: []string{"restart_pod"},
		Category:           "memory_critical",
	},
	// CPU Alert Scenarios
	{
		Name: "HighCPUUsage_Scaling_Needed",
		Alert: types.Alert{
			Name:        "HighCPUUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod CPU utilization at 90%",
			Namespace:   "production",
			Resource:    "worker-deployment",
			Labels: map[string]string{
				"alertname":       "HighCPUUsage",
				"app":             "worker",
				"cpu_utilization": "90%",
			},
			Annotations: map[string]string{
				"description": "High CPU usage indicates scaling needed",
			},
		},
		ExpectedAction:     "scale_deployment",
		ExpectedConfidence: 0.8,
		Reasoning:          "High CPU suggests need for horizontal scaling",
		AlternativeActions: []string{"increase_resources", "optimize_resources"},
		Category:           "cpu",
	},
	// Storage Alert Scenarios
	{
		Name: "DiskSpaceLow_CleanupNeeded",
		Alert: types.Alert{
			Name:        "DiskSpaceLow",
			Status:      "firing",
			Severity:    "warning",
			Description: "Disk usage at 85% on /var/logs",
			Namespace:   "production",
			Resource:    "logging-pod",
			Labels: map[string]string{
				"alertname": "DiskSpaceLow",
				"volume":    "/var/logs",
				"usage":     "85%",
			},
			Annotations: map[string]string{
				"description": "Clear disk space before critical threshold",
			},
		},
		ExpectedAction:     "cleanup_storage",
		ExpectedConfidence: 0.9,
		Reasoning:          "Clear disk space before critical threshold",
		AlternativeActions: []string{"expand_pvc", "backup_data"},
		Category:           "storage",
	},
	// Network Alert Scenarios
	{
		Name: "ServiceUnavailable_NetworkIssue",
		Alert: types.Alert{
			Name:        "ServiceUnavailable",
			Status:      "firing",
			Severity:    "critical",
			Description: "Service cannot be reached",
			Namespace:   "production",
			Resource:    "payment-api",
			Labels: map[string]string{
				"alertname": "ServiceUnavailable",
				"service":   "payment-api",
				"error":     "connection_refused",
			},
			Annotations: map[string]string{
				"description": "Network connectivity issues detected",
			},
		},
		ExpectedAction:     "restart_pod",
		ExpectedConfidence: 0.7,
		Reasoning:          "Network connectivity issues often resolve with restart",
		AlternativeActions: []string{"update_network_policy", "restart_network"},
		Category:           "network",
	},
	// Security Alert Scenarios
	{
		Name: "SecurityBreach_Quarantine",
		Alert: types.Alert{
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
			Annotations: map[string]string{
				"description": "Immediate containment required",
			},
		},
		ExpectedAction:     "quarantine_pod",
		ExpectedConfidence: 0.95,
		Reasoning:          "Security threats require immediate isolation",
		AlternativeActions: []string{"notify_only", "audit_logs"},
		Category:           "security",
	},
	// Node Alert Scenarios
	{
		Name: "NodeUnreachable_Drain_Needed",
		Alert: types.Alert{
			Name:        "NodeUnreachable",
			Status:      "firing",
			Severity:    "critical",
			Description: "Node is not responding",
			Namespace:   "kube-system",
			Resource:    "worker-node-1",
			Labels: map[string]string{
				"alertname": "NodeUnreachable",
				"node":      "worker-node-1",
				"kubelet":   "not_responding",
			},
			Annotations: map[string]string{
				"node_level_action_required": "true",
			},
		},
		ExpectedAction:     "drain_node",
		ExpectedConfidence: 0.8,
		Reasoning:          "Node issues require drain for maintenance",
		AlternativeActions: []string{"collect_diagnostics", "notify_only"},
		Category:           "node",
	},
	// Database Alert Scenarios
	{
		Name: "DatabaseConnectionFailure",
		Alert: types.Alert{
			Name:        "DatabaseConnectionPool",
			Status:      "firing",
			Severity:    "critical",
			Description: "Database connection pool exhausted",
			Namespace:   "production",
			Resource:    "postgres-primary",
			Labels: map[string]string{
				"alertname": "DatabaseConnectionPool",
				"database":  "postgres",
				"pool":      "exhausted",
			},
			Annotations: map[string]string{
				"description": "All database connections in use",
			},
		},
		ExpectedAction:     "restart_pod",
		ExpectedConfidence: 0.75,
		Reasoning:          "Connection pool exhaustion often resolves with restart",
		AlternativeActions: []string{"failover_database", "scale_deployment"},
		Category:           "database",
	},
	// Complex Scenarios
	{
		Name: "ImagePullBackOff_Investigation",
		Alert: types.Alert{
			Name:        "PodImagePullBackOff",
			Status:      "firing",
			Severity:    "warning",
			Description: "Cannot pull container image",
			Namespace:   "production",
			Resource:    "app-deployment",
			Labels: map[string]string{
				"alertname": "PodImagePullBackOff",
				"reason":    "ImagePullBackOff",
			},
			Annotations: map[string]string{
				"termination_reason": "ImagePullBackOff",
			},
		},
		ExpectedAction:     "collect_diagnostics",
		ExpectedConfidence: 0.8,
		Reasoning:          "Image pull failures require investigation",
		AlternativeActions: []string{"rollback_deployment", "notify_only"},
		Category:           "deployment",
	},
}

// Global variables for AfterSuite callback
var (
	modelComparisonResults map[string]*ModelResults
	modelComparisonLogger  *logrus.Logger
)

var _ = Describe("Model Comparison Tests (Ollama Demo)", func() {
	var (
		results map[string]*ModelResults
		logger  *logrus.Logger
	)

	BeforeEach(func() {
		results = make(map[string]*ModelResults)
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Assign to global variables for AfterSuite
		modelComparisonResults = results
		modelComparisonLogger = logger

		// Skip if model comparison is disabled
		if os.Getenv("SKIP_MODEL_COMPARISON") != "" {
			Skip("Model comparison tests disabled")
		}
	})

	// Test each model against all test scenarios
	for _, model := range TestModels {
		modelName := model.ModelName

		Context(fmt.Sprintf("Model: %s", modelName), func() {
			var client slm.Client

			BeforeEach(func() {
				// Create SLM client for this model
				cfg := config.SLMConfig{
					Provider:    "localai", // Using OpenAI-compatible API
					Endpoint:    model.Endpoint,
					Model:       model.ModelName,
					Temperature: float32(model.Temperature),
					MaxTokens:   model.MaxTokens,
					Timeout:     model.Timeout,
					RetryCount:  1,
				}

				var err error
				client, err = slm.NewClient(cfg, logger)
				Expect(err).NotTo(HaveOccurred())

				// Test health check with timeout
				healthy := false
				for i := 0; i < 5; i++ {
					if client.IsHealthy() {
						healthy = true
						break
					}
					time.Sleep(2 * time.Second)
				}

				if !healthy {
					Skip(fmt.Sprintf("Model %s is not healthy, skipping tests", modelName))
				}
			})

			// Initialize results for this model
			BeforeEach(func() {
				if results[modelName] == nil {
					results[modelName] = &ModelResults{
						ModelName:   modelName,
						TestResults: make([]TestResult, 0),
					}
				}
			})

			// Run each test scenario (demo subset)
			demoScenarios := TestScenarios[:5] // First 5 scenarios for demo
			for _, testCase := range demoScenarios {
				testCaseName := testCase.Name

				It(fmt.Sprintf("should handle %s correctly", testCaseName), func() {
					// Run test case multiple times for consistency (demo: single run)
					var testResults []TestResult
					const numRuns = 1 // Demo mode: single run for speed

					for run := 0; run < numRuns; run++ {
						startTime := time.Now()

						ctx, cancel := context.WithTimeout(context.Background(), model.Timeout)
						recommendation, err := client.AnalyzeAlert(ctx, testCase.Alert)
						cancel()

						responseTime := time.Since(startTime)

						result := TestResult{
							TestCase:       testCase,
							Recommendation: recommendation,
							ResponseTime:   responseTime,
							Error:          err,
							Timestamp:      startTime,
						}

						testResults = append(testResults, result)

						// Basic validation for this run
						if err == nil {
							Expect(result.Recommendation).NotTo(BeNil())
							Expect(result.Recommendation.Action).NotTo(BeEmpty())
							Expect(result.Recommendation.Confidence).To(BeNumerically(">=", 0.0))
							Expect(result.Recommendation.Confidence).To(BeNumerically("<=", 1.0))
							Expect(result.ResponseTime).To(BeNumerically("<", model.Timeout))
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

// AfterSuite runs after all tests complete
var _ = AfterSuite(func() {
	// Calculate final metrics for each model
	for modelName, modelResult := range modelComparisonResults {
		CalculateModelMetrics(modelResult)
		modelComparisonLogger.WithFields(logrus.Fields{
			"model":            modelName,
			"total_tests":      modelResult.Summary.TotalTestCases,
			"successful_tests": modelResult.Summary.SuccessfulCases,
			"action_accuracy":  fmt.Sprintf("%.2f%%", modelResult.Reasoning.ActionAccuracy*100),
			"avg_response":     modelResult.Performance.ResponseTime.Mean,
		}).Info("Model evaluation completed")
	}

	// Generate comparison report
	GenerateComparisonReport(modelComparisonResults, modelComparisonLogger)

	// Export results to JSON
	ExportResultsToJSON(modelComparisonResults, modelComparisonLogger)

	// Generate recommendations
	GenerateModelRecommendations(modelComparisonResults, modelComparisonLogger)
})

// Helper functions for metrics calculation and reporting
func CalculateModelMetrics(modelResult *ModelResults) {
	if len(modelResult.TestResults) == 0 {
		return
	}

	// Calculate performance metrics
	var responseTimes []time.Duration
	successCount := 0
	var totalConfidence float64
	correctActions := 0

	for _, result := range modelResult.TestResults {
		responseTimes = append(responseTimes, result.ResponseTime)

		if result.Error == nil {
			successCount++
			totalConfidence += result.Recommendation.Confidence

			// Check if action matches expected or alternatives
			if result.Recommendation.Action == result.TestCase.ExpectedAction {
				correctActions++
			} else {
				// Check alternatives
				for _, alt := range result.TestCase.AlternativeActions {
					if result.Recommendation.Action == alt {
						correctActions++
						break
					}
				}
			}
		}
	}

	totalTests := len(modelResult.TestResults)

	// Performance metrics
	modelResult.Performance.TotalRequests = totalTests
	modelResult.Performance.SuccessfulRuns = successCount
	modelResult.Performance.ErrorRate = float64(totalTests-successCount) / float64(totalTests)

	if len(responseTimes) > 0 {
		modelResult.Performance.ResponseTime = CalculatePerformanceData(responseTimes)
		modelResult.Performance.Throughput = 60.0 / modelResult.Performance.ResponseTime.Mean.Seconds()
	}

	// Reasoning metrics
	if successCount > 0 {
		modelResult.Reasoning.ActionAccuracy = float64(correctActions) / float64(successCount)
		modelResult.Reasoning.ConfidenceCalibration = totalConfidence / float64(successCount)
	}

	// Summary
	modelResult.Summary.TotalTestCases = totalTests
	modelResult.Summary.SuccessfulCases = successCount
	modelResult.Summary.FailedCases = totalTests - successCount
	if successCount > 0 {
		modelResult.Summary.AverageConfidence = totalConfidence / float64(successCount)
	}

	// Overall rating
	if modelResult.Reasoning.ActionAccuracy >= 0.9 && modelResult.Performance.ErrorRate <= 0.1 {
		modelResult.Summary.OverallRating = "Excellent"
	} else if modelResult.Reasoning.ActionAccuracy >= 0.8 && modelResult.Performance.ErrorRate <= 0.2 {
		modelResult.Summary.OverallRating = "Good"
	} else if modelResult.Reasoning.ActionAccuracy >= 0.7 && modelResult.Performance.ErrorRate <= 0.3 {
		modelResult.Summary.OverallRating = "Fair"
	} else {
		modelResult.Summary.OverallRating = "Poor"
	}
}

func CalculatePerformanceData(times []time.Duration) PerformanceData {
	if len(times) == 0 {
		return PerformanceData{}
	}

	// Sort times for percentile calculation
	for i := 0; i < len(times)-1; i++ {
		for j := 0; j < len(times)-i-1; j++ {
			if times[j] > times[j+1] {
				times[j], times[j+1] = times[j+1], times[j]
			}
		}
	}

	var sum time.Duration
	min := times[0]
	max := times[len(times)-1]

	for _, t := range times {
		sum += t
	}
	mean := sum / time.Duration(len(times))

	// Calculate percentiles
	p50 := times[len(times)*50/100]
	p95 := times[len(times)*95/100]
	p99 := times[len(times)*99/100]

	return PerformanceData{
		Mean: mean,
		P50:  p50,
		P95:  p95,
		P99:  p99,
		Min:  min,
		Max:  max,
	}
}

func GenerateComparisonReport(results map[string]*ModelResults, logger *logrus.Logger) {
	report := "# Model Comparison Report (Ollama Demo)\n\n"
	report += "## Summary\n\n"

	for modelName, result := range results {
		report += fmt.Sprintf("### %s\n", modelName)
		report += fmt.Sprintf("- **Overall Rating**: %s\n", result.Summary.OverallRating)
		report += fmt.Sprintf("- **Action Accuracy**: %.2f%%\n", result.Reasoning.ActionAccuracy*100)
		report += fmt.Sprintf("- **Success Rate**: %.2f%%\n", (1.0-result.Performance.ErrorRate)*100)
		report += fmt.Sprintf("- **Avg Response Time**: %v\n", result.Performance.ResponseTime.Mean)
		report += fmt.Sprintf("- **P95 Response Time**: %v\n", result.Performance.ResponseTime.P95)
		report += "\n"
	}

	report += "## Demo Mode Notes\n\n"
	report += "This was run in **demo mode** using ollama with model switching.\n\n"
	report += "**Limitations:**\n"
	report += "- Single ollama instance (model switching overhead)\n"
	report += "- Limited test scenarios (5 vs full 15)\n"
	report += "- Single run per scenario (vs 3 runs for consistency)\n\n"
	report += "**For Production Comparison:**\n"
	report += "- Install ramallama: `cargo install ramallama`\n"
	report += "- Run: `make model-comparison-full`\n\n"

	// Write report to file
	err := os.WriteFile("model_comparison_report.md", []byte(report), 0644)
	if err != nil {
		logger.WithError(err).Error("Failed to write comparison report")
	} else {
		logger.Info("Model comparison report generated: model_comparison_report.md")
	}
}

func ExportResultsToJSON(results map[string]*ModelResults, logger *logrus.Logger) {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		logger.WithError(err).Error("Failed to marshal results to JSON")
		return
	}

	err = os.WriteFile("model_comparison_results.json", data, 0644)
	if err != nil {
		logger.WithError(err).Error("Failed to write results to JSON file")
	} else {
		logger.Info("Model comparison results exported: model_comparison_results.json")
	}
}

func GenerateModelRecommendations(results map[string]*ModelResults, logger *logrus.Logger) {
	// Find best performing model
	var bestModel string
	var bestScore float64

	for modelName, result := range results {
		// Composite score: 40% accuracy, 30% success rate, 20% speed, 10% confidence
		score := result.Reasoning.ActionAccuracy*0.4 +
			(1.0-result.Performance.ErrorRate)*0.3 +
			(30.0/result.Performance.ResponseTime.Mean.Seconds())*0.2 + // Normalized speed score
			result.Summary.AverageConfidence*0.1

		if score > bestScore {
			bestScore = score
			bestModel = modelName
		}
	}

	recommendation := "## Recommendation\n\n"
	recommendation += fmt.Sprintf("**Best performing model**: %s (score: %.3f)\n\n", bestModel, bestScore)

	if bestModel != "granite3.1-dense:8b" {
		recommendation += "**Migration recommended** to improve performance.\n"
	} else {
		recommendation += "**Current model remains optimal** for production use.\n"
	}

	err := os.WriteFile("model_recommendation.md", []byte(recommendation), 0644)
	if err != nil {
		logger.WithError(err).Error("Failed to write recommendation")
	} else {
		logger.Info("Model recommendation generated: model_recommendation.md")
	}
}
