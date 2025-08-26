//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

// OllamaIntegrationSuite is the main test suite for Ollama integration
type OllamaIntegrationSuite struct {
	suite.Suite
	client       slm.Client
	config       config.SLMConfig
	testConfig   IntegrationConfig
	monitor      *ResourceMonitor
	report       *IntegrationTestReport
	logger       *logrus.Logger
}

// SetupSuite initializes the test suite
func (s *OllamaIntegrationSuite) SetupSuite() {
	s.testConfig = LoadConfig()
	
	// Skip if integration tests are disabled
	if s.testConfig.SkipIntegration {
		s.T().Skip("Integration tests skipped via SKIP_INTEGRATION")
	}

	// Create logger
	s.logger = logrus.New()
	if level, err := logrus.ParseLevel(s.testConfig.LogLevel); err == nil {
		s.logger.SetLevel(level)
	}
	s.logger.SetFormatter(&logrus.JSONFormatter{})

	// Load SLM configuration
	s.config = config.SLMConfig{
		Provider:    "localai",
		Endpoint:    s.testConfig.OllamaEndpoint,
		Model:       s.testConfig.OllamaModel,
		Temperature: 0.3,
		MaxTokens:   500,
		Timeout:     30 * time.Second,
		RetryCount:  s.testConfig.MaxRetries,
	}

	s.logger.WithFields(logrus.Fields{
		"endpoint": s.config.Endpoint,
		"model":    s.config.Model,
		"timeout":  s.config.Timeout,
	}).Info("Setting up Ollama integration test suite")

	// Create SLM client
	client, err := slm.NewClient(s.config, s.logger)
	s.Require().NoError(err, "Failed to create SLM client")
	s.client = client

	// Initialize monitoring and reporting
	s.monitor = NewResourceMonitor()
	s.report = NewIntegrationTestReport()
}

// SetupTest runs before each test
func (s *OllamaIntegrationSuite) SetupTest() {
	s.logger.WithField("test", s.T().Name()).Debug("Starting test")
}

// TearDownTest runs after each test
func (s *OllamaIntegrationSuite) TearDownTest() {
	s.logger.WithField("test", s.T().Name()).Debug("Completed test")
}

// TearDownSuite runs after all tests
func (s *OllamaIntegrationSuite) TearDownSuite() {
	// Generate final report
	resourceReport := s.monitor.GenerateReport()
	s.report.CalculateStats(resourceReport)
	
	s.logger.WithFields(logrus.Fields{
		"total_tests":        s.report.TotalTests,
		"passed_tests":       s.report.PassedTests,
		"failed_tests":       len(s.report.FailedTests),
		"skipped_tests":      len(s.report.SkippedTests),
		"average_response":   s.report.AverageResponse,
		"max_response":       s.report.MaxResponse,
		"memory_growth":      s.report.ResourceUsage.MemoryGrowth,
		"goroutine_growth":   s.report.ResourceUsage.GoroutineGrowth,
	}).Info("Integration test suite completed")

	// Print summary for human readers
	fmt.Printf("\n=== Integration Test Report ===\n")
	fmt.Printf("Total Tests: %d\n", s.report.TotalTests)
	fmt.Printf("Passed: %d\n", s.report.PassedTests)
	fmt.Printf("Failed: %d\n", len(s.report.FailedTests))
	fmt.Printf("Skipped: %d\n", len(s.report.SkippedTests))
	fmt.Printf("Average Response Time: %v\n", s.report.AverageResponse)
	fmt.Printf("Max Response Time: %v\n", s.report.MaxResponse)
	
	if len(s.report.FailedTests) > 0 {
		fmt.Printf("Failed Tests: %v\n", s.report.FailedTests)
	}
	
	fmt.Printf("Action Distribution:\n")
	for action, count := range s.report.ActionDistribution {
		fmt.Printf("  %s: %d\n", action, count)
	}
	fmt.Printf("===============================\n\n")
}

// TestOllamaConnectivity tests basic connectivity to Ollama
func (s *OllamaIntegrationSuite) TestOllamaConnectivity() {
	s.report.TotalTests++
	
	s.T().Log("Testing Ollama connectivity...")
	
	start := time.Now()
	isHealthy := s.client.IsHealthy()
	duration := time.Since(start)
	
	s.monitor.RecordMeasurement(duration)
	
	if s.Assert().True(isHealthy, "Ollama should be healthy and accessible") {
		s.report.PassedTests++
		s.report.AddModelResponse(ModelResponse{
			TestName:     "TestOllamaConnectivity",
			Success:      true,
			ResponseTime: duration,
		})
	} else {
		s.report.AddFailedTest("TestOllamaConnectivity")
	}
}

// TestModelAvailability tests that the configured model is available
func (s *OllamaIntegrationSuite) TestModelAvailability() {
	s.report.TotalTests++
	
	s.T().Log("Testing model availability...")
	
	// Use connectivity test alert
	testAlert := IntegrationTestAlerts[len(IntegrationTestAlerts)-1].Alert // Last one is connectivity test
	
	ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
	defer cancel()
	
	start := time.Now()
	recommendation, err := s.client.AnalyzeAlert(ctx, testAlert)
	duration := time.Since(start)
	
	s.monitor.RecordMeasurement(duration)
	
	if s.Assert().NoError(err, "Model should be available and responsive") &&
		s.Assert().NotNil(recommendation, "Should receive a recommendation") {
		
		s.report.PassedTests++
		s.report.AddModelResponse(ModelResponse{
			TestName:     "TestModelAvailability",
			Action:       recommendation.Action,
			Confidence:   recommendation.Confidence,
			Reasoning:    recommendation.Reasoning,
			ResponseTime: duration,
			Success:      true,
		})
		
		s.T().Logf("Model responded in %v with action: %s (confidence: %.2f)", 
			duration, recommendation.Action, recommendation.Confidence)
	} else {
		s.report.AddFailedTest("TestModelAvailability")
		if err != nil {
			s.report.AddModelResponse(ModelResponse{
				TestName:     "TestModelAvailability",
				Success:      false,
				Error:        err.Error(),
				ResponseTime: duration,
			})
		}
	}
}

// TestAlertAnalysisScenarios tests various alert scenarios
func (s *OllamaIntegrationSuite) TestAlertAnalysisScenarios() {
	for _, testCase := range IntegrationTestAlerts {
		s.Run(testCase.Name, func() {
			s.report.TotalTests++
			
			s.T().Logf("Testing alert scenario: %s", testCase.Description)
			
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()
			
			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, testCase.Alert)
			duration := time.Since(start)
			
			s.monitor.RecordMeasurement(duration)
			
			if s.Assert().NoError(err, "Alert analysis should succeed") &&
				s.Assert().NotNil(recommendation, "Should receive recommendation") {
				
				// Validate recommendation structure
				s.Assert().NotEmpty(recommendation.Action, "Action should not be empty")
				s.Assert().Contains([]string{"scale_deployment", "restart_pod", "increase_resources", "notify_only"}, 
					recommendation.Action, "Action should be valid")
				s.Assert().GreaterOrEqual(recommendation.Confidence, 0.0, "Confidence should be >= 0")
				s.Assert().LessOrEqual(recommendation.Confidence, 1.0, "Confidence should be <= 1")
				
				// Check if action is in expected list (if specified)
				if len(testCase.ExpectedActions) > 0 {
					s.Assert().Contains(testCase.ExpectedActions, recommendation.Action, 
						"Action should be one of the expected actions for this scenario")
				}
				
				// Check minimum confidence (if specified)
				if testCase.MinConfidence > 0 {
					s.Assert().GreaterOrEqual(recommendation.Confidence, testCase.MinConfidence, 
						"Confidence should meet minimum threshold")
				}
				
				s.report.PassedTests++
				s.report.AddModelResponse(ModelResponse{
					TestName:     testCase.Name,
					Action:       recommendation.Action,
					Confidence:   recommendation.Confidence,
					Reasoning:    recommendation.Reasoning,
					ResponseTime: duration,
					Success:      true,
				})
				
				s.T().Logf("Recommendation: action=%s, confidence=%.2f, time=%v", 
					recommendation.Action, recommendation.Confidence, duration)
				s.T().Logf("Reasoning: %s", recommendation.Reasoning)
				
			} else {
				s.report.AddFailedTest(testCase.Name)
				if err != nil {
					s.report.AddModelResponse(ModelResponse{
						TestName:     testCase.Name,
						Success:      false,
						Error:        err.Error(),
						ResponseTime: duration,
					})
				}
			}
		})
	}
}

// TestResponseTimePerformance tests response time performance
func (s *OllamaIntegrationSuite) TestResponseTimePerformance() {
	if s.testConfig.SkipSlowTests {
		s.T().Skip("Skipping slow performance tests")
	}
	
	s.report.TotalTests++
	s.T().Log("Testing response time performance...")
	
	const numRequests = 5
	var totalDuration time.Duration
	var maxDuration time.Duration
	
	for i := 0; i < numRequests; i++ {
		alert := PerformanceTestAlert(i)
		
		ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
		
		start := time.Now()
		recommendation, err := s.client.AnalyzeAlert(ctx, alert)
		duration := time.Since(start)
		cancel()
		
		s.monitor.RecordMeasurement(duration)
		totalDuration += duration
		if duration > maxDuration {
			maxDuration = duration
		}
		
		s.Assert().NoError(err, "Performance test request should succeed")
		s.Assert().NotNil(recommendation, "Should receive recommendation")
		
		if err == nil && recommendation != nil {
			s.report.AddModelResponse(ModelResponse{
				TestName:     fmt.Sprintf("TestResponseTimePerformance_%d", i),
				Action:       recommendation.Action,
				Confidence:   recommendation.Confidence,
				ResponseTime: duration,
				Success:      true,
			})
		}
	}
	
	avgDuration := totalDuration / numRequests
	
	// Performance expectations
	s.Assert().Less(avgDuration, 15*time.Second, "Average response should be under 15 seconds")
	s.Assert().Less(maxDuration, 30*time.Second, "Max response should be under 30 seconds")
	
	s.report.PassedTests++
	s.T().Logf("Performance results: avg=%v, max=%v", avgDuration, maxDuration)
}

// TestConcurrentRequests tests concurrent request handling
func (s *OllamaIntegrationSuite) TestConcurrentRequests() {
	if s.testConfig.SkipSlowTests {
		s.T().Skip("Skipping slow concurrent tests")
	}
	
	s.report.TotalTests++
	s.T().Log("Testing concurrent request handling...")
	
	const numRequests = 3 // Keep it reasonable for CI
	results := make(chan ModelResponse, numRequests)
	var wg sync.WaitGroup
	
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			alert := ConcurrentTestAlert(id)
			
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()
			
			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, alert)
			duration := time.Since(start)
			
			s.monitor.RecordMeasurement(duration)
			
			result := ModelResponse{
				TestName:     fmt.Sprintf("TestConcurrentRequests_%d", id),
				ResponseTime: duration,
				Success:      err == nil && recommendation != nil,
			}
			
			if err != nil {
				result.Error = err.Error()
			} else if recommendation != nil {
				result.Action = recommendation.Action
				result.Confidence = recommendation.Confidence
				result.Reasoning = recommendation.Reasoning
			}
			
			results <- result
		}(i)
	}
	
	// Wait for all goroutines to complete
	wg.Wait()
	close(results)
	
	// Collect results
	successCount := 0
	for result := range results {
		s.report.AddModelResponse(result)
		if result.Success {
			successCount++
		}
	}
	
	s.Assert().Equal(numRequests, successCount, "All concurrent requests should succeed")
	
	if successCount == numRequests {
		s.report.PassedTests++
		s.T().Logf("All %d concurrent requests succeeded", numRequests)
	} else {
		s.report.AddFailedTest("TestConcurrentRequests")
		s.T().Logf("Only %d out of %d concurrent requests succeeded", successCount, numRequests)
	}
}

// TestErrorHandling tests error handling scenarios
func (s *OllamaIntegrationSuite) TestErrorHandling() {
	s.report.TotalTests++
	s.T().Log("Testing error handling scenarios...")
	
	// Test with malformed alert
	malformedAlert := MalformedAlert()
	
	ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
	defer cancel()
	
	start := time.Now()
	recommendation, err := s.client.AnalyzeAlert(ctx, malformedAlert)
	duration := time.Since(start)
	
	s.monitor.RecordMeasurement(duration)
	
	// Should either handle gracefully or return appropriate error
	if err != nil {
		s.T().Logf("Expected error for malformed alert: %v", err)
		s.report.AddModelResponse(ModelResponse{
			TestName:     "TestErrorHandling",
			Success:      false,
			Error:        err.Error(),
			ResponseTime: duration,
		})
	} else {
		s.Assert().NotNil(recommendation, "If no error, should return recommendation")
		s.T().Logf("Graceful handling: action=%s, confidence=%.2f", 
			recommendation.Action, recommendation.Confidence)
		s.report.AddModelResponse(ModelResponse{
			TestName:     "TestErrorHandling",
			Action:       recommendation.Action,
			Confidence:   recommendation.Confidence,
			Reasoning:    recommendation.Reasoning,
			ResponseTime: duration,
			Success:      true,
		})
	}
	
	s.report.PassedTests++
}

// TestContextCancellation tests context cancellation handling
func (s *OllamaIntegrationSuite) TestContextCancellation() {
	s.report.TotalTests++
	s.T().Log("Testing context cancellation...")
	
	alert := PerformanceTestAlert(999)
	
	// Create a context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	start := time.Now()
	recommendation, err := s.client.AnalyzeAlert(ctx, alert)
	duration := time.Since(start)
	
	s.monitor.RecordMeasurement(duration)
	
	// Should handle context cancellation gracefully
	if err != nil {
		s.T().Logf("Expected context cancellation error: %v", err)
		s.Assert().Contains(err.Error(), "context", "Error should mention context")
	} else {
		s.T().Logf("Request completed before cancellation: %+v", recommendation)
	}
	
	s.report.PassedTests++
	s.report.AddModelResponse(ModelResponse{
		TestName:     "TestContextCancellation",
		Success:      err == nil,
		Error:        fmt.Sprintf("%v", err),
		ResponseTime: duration,
	})
}

// TestProductionEdgeCases runs comprehensive production scenario tests
func (s *OllamaIntegrationSuite) TestProductionEdgeCases() {
	if s.testConfig.SkipSlowTests {
		s.T().Skip("Skipping production edge case tests")
	}
	
	s.report.TotalTests++
	s.T().Log("Testing production edge case scenarios...")

	// Get all edge case scenarios
	edgeCases := GetAllEdgeCaseAlerts()
	s.T().Logf("Testing %d production edge case scenarios", len(edgeCases))

	passedEdgeCases := 0
	for _, testCase := range edgeCases {
		s.Run(testCase.Name, func() {
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()

			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, testCase.Alert)
			duration := time.Since(start)

			s.monitor.RecordMeasurement(duration)

			if s.Assert().NoError(err, "Edge case alert analysis should succeed") &&
				s.Assert().NotNil(recommendation, "Should receive recommendation for edge case") {

				// Validate recommendation structure for edge cases
				s.Assert().NotEmpty(recommendation.Action, "Action should not be empty")
				s.Assert().Contains([]string{"scale_deployment", "restart_pod", "increase_resources", "notify_only"},
					recommendation.Action, "Action should be valid")
				s.Assert().GreaterOrEqual(recommendation.Confidence, 0.0, "Confidence should be >= 0")
				s.Assert().LessOrEqual(recommendation.Confidence, 1.0, "Confidence should be <= 1")

				// Check expected actions for edge cases (if specified)
				if len(testCase.ExpectedActions) > 0 {
					s.Assert().Contains(testCase.ExpectedActions, recommendation.Action,
						"Edge case action should be one of the expected actions")
				}

				// Check minimum confidence for edge cases
				if testCase.MinConfidence > 0 {
					s.Assert().GreaterOrEqual(recommendation.Confidence, testCase.MinConfidence,
						"Edge case confidence should meet minimum threshold")
				}

				s.report.AddModelResponse(ModelResponse{
					TestName:     testCase.Name,
					Action:       recommendation.Action,
					Confidence:   recommendation.Confidence,
					Reasoning:    recommendation.Reasoning,
					ResponseTime: duration,
					Success:      true,
				})

				s.T().Logf("Edge case '%s': action=%s, confidence=%.2f, time=%v",
					testCase.Name, recommendation.Action, recommendation.Confidence, duration)

				passedEdgeCases++
			} else {
				s.report.AddFailedTest(testCase.Name)
				if err != nil {
					s.report.AddModelResponse(ModelResponse{
						TestName:     testCase.Name,
						Success:      false,
						Error:        err.Error(),
						ResponseTime: duration,
					})
				}
			}
		})
	}

	if passedEdgeCases == len(edgeCases) {
		s.report.PassedTests++
		s.T().Logf("All %d production edge cases passed successfully", passedEdgeCases)
	} else {
		s.report.AddFailedTest("TestProductionEdgeCases")
		s.T().Logf("Production edge cases: %d/%d passed", passedEdgeCases, len(edgeCases))
	}
}

// TestChaosEngineeringScenarios tests chaos engineering specific scenarios
func (s *OllamaIntegrationSuite) TestChaosEngineeringScenarios() {
	if s.testConfig.SkipSlowTests {
		s.T().Skip("Skipping chaos engineering tests")
	}

	s.report.TotalTests++
	s.T().Log("Testing chaos engineering scenarios...")

	scenarios := []string{"cpu_stress", "memory_leak"}
	
	for _, scenario := range scenarios {
		s.Run(fmt.Sprintf("ChaosEngineering_%s", scenario), func() {
			alert := ChaosEngineeringTestAlert(scenario)
			
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()

			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, alert)
			duration := time.Since(start)

			s.monitor.RecordMeasurement(duration)

			if s.Assert().NoError(err, "Chaos engineering scenario should be analyzed") &&
				s.Assert().NotNil(recommendation, "Should receive recommendation for chaos scenario") {

				// Chaos scenarios should typically recommend monitoring or minimal intervention
				expectedActions := []string{"notify_only", "restart_pod", "scale_deployment"}
				s.Assert().Contains(expectedActions, recommendation.Action,
					"Chaos scenarios should recommend appropriate conservative actions")

				s.report.AddModelResponse(ModelResponse{
					TestName:     fmt.Sprintf("ChaosEngineering_%s", scenario),
					Action:       recommendation.Action,
					Confidence:   recommendation.Confidence,
					Reasoning:    recommendation.Reasoning,
					ResponseTime: duration,
					Success:      true,
				})

				s.T().Logf("Chaos scenario '%s': action=%s, confidence=%.2f, reasoning: %s",
					scenario, recommendation.Action, recommendation.Confidence, recommendation.Reasoning)
			}
		})
	}

	s.report.PassedTests++
}

// TestSecurityIncidentHandling tests security-focused scenarios
func (s *OllamaIntegrationSuite) TestSecurityIncidentHandling() {
	s.report.TotalTests++
	s.T().Log("Testing security incident handling...")

	incidents := []string{"privilege_escalation", "data_exfiltration"}
	
	for _, incident := range incidents {
		s.Run(fmt.Sprintf("SecurityIncident_%s", incident), func() {
			alert := SecurityIncidentAlert(incident)
			
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()

			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, alert)
			duration := time.Since(start)

			s.monitor.RecordMeasurement(duration)

			if s.Assert().NoError(err, "Security incident should be analyzed") &&
				s.Assert().NotNil(recommendation, "Should receive recommendation for security incident") {

				// Security incidents should have high confidence and appropriate actions
				s.Assert().GreaterOrEqual(recommendation.Confidence, 0.8, 
					"Security incidents should have high confidence")
				
				// Security actions should be conservative (notify or restart for quarantine)
				expectedActions := []string{"notify_only", "restart_pod"}
				s.Assert().Contains(expectedActions, recommendation.Action,
					"Security incidents should recommend immediate attention or quarantine")

				s.report.AddModelResponse(ModelResponse{
					TestName:     fmt.Sprintf("SecurityIncident_%s", incident),
					Action:       recommendation.Action,
					Confidence:   recommendation.Confidence,
					Reasoning:    recommendation.Reasoning,
					ResponseTime: duration,
					Success:      true,
				})

				s.T().Logf("Security incident '%s': action=%s, confidence=%.2f",
					incident, recommendation.Action, recommendation.Confidence)
				s.T().Logf("Security reasoning: %s", recommendation.Reasoning)
			}
		})
	}

	s.report.PassedTests++
}

// TestResourceExhaustionScenarios tests resource exhaustion edge cases
func (s *OllamaIntegrationSuite) TestResourceExhaustionScenarios() {
	s.report.TotalTests++
	s.T().Log("Testing resource exhaustion scenarios...")

	resources := []string{"inode_exhaustion", "network_bandwidth"}
	
	for _, resource := range resources {
		s.Run(fmt.Sprintf("ResourceExhaustion_%s", resource), func() {
			alert := ResourceExhaustionAlert(resource)
			
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()

			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, alert)
			duration := time.Since(start)

			s.monitor.RecordMeasurement(duration)

			if s.Assert().NoError(err, "Resource exhaustion should be analyzed") &&
				s.Assert().NotNil(recommendation, "Should receive recommendation for resource exhaustion") {

				// Resource exhaustion should trigger appropriate scaling or restart actions
				expectedActions := []string{"restart_pod", "scale_deployment", "increase_resources", "notify_only"}
				s.Assert().Contains(expectedActions, recommendation.Action,
					"Resource exhaustion should recommend scaling, restart, or resources")

				s.report.AddModelResponse(ModelResponse{
					TestName:     fmt.Sprintf("ResourceExhaustion_%s", resource),
					Action:       recommendation.Action,
					Confidence:   recommendation.Confidence,
					Reasoning:    recommendation.Reasoning,
					ResponseTime: duration,
					Success:      true,
				})

				s.T().Logf("Resource exhaustion '%s': action=%s, confidence=%.2f",
					resource, recommendation.Action, recommendation.Confidence)
			}
		})
	}

	s.report.PassedTests++
}

// TestCascadingFailureResponse tests complex cascading failure scenarios
func (s *OllamaIntegrationSuite) TestCascadingFailureResponse() {
	if s.testConfig.SkipSlowTests {
		s.T().Skip("Skipping cascading failure tests")
	}

	s.report.TotalTests++
	s.T().Log("Testing cascading failure response...")

	scenarios := []string{"monitoring_cascade", "storage_cascade"}
	
	for _, scenario := range scenarios {
		s.Run(fmt.Sprintf("CascadingFailure_%s", scenario), func() {
			alert := CascadingFailureAlert(scenario)
			
			ctx, cancel := context.WithTimeout(context.Background(), s.testConfig.TestTimeout)
			defer cancel()

			start := time.Now()
			recommendation, err := s.client.AnalyzeAlert(ctx, alert)
			duration := time.Since(start)

			s.monitor.RecordMeasurement(duration)

			if s.Assert().NoError(err, "Cascading failure should be analyzed") &&
				s.Assert().NotNil(recommendation, "Should receive recommendation for cascading failure") {

				// Cascading failures should typically require human intervention
				s.Assert().GreaterOrEqual(recommendation.Confidence, 0.85, 
					"Cascading failures should have high confidence")

				// Most cascading failures should recommend notification for human intervention
				s.Assert().Contains([]string{"notify_only", "restart_pod"}, recommendation.Action,
					"Cascading failures typically require human intervention or careful restart")

				s.report.AddModelResponse(ModelResponse{
					TestName:     fmt.Sprintf("CascadingFailure_%s", scenario),
					Action:       recommendation.Action,
					Confidence:   recommendation.Confidence,
					Reasoning:    recommendation.Reasoning,
					ResponseTime: duration,
					Success:      true,
				})

				s.T().Logf("Cascading failure '%s': action=%s, confidence=%.2f",
					scenario, recommendation.Action, recommendation.Confidence)
				s.T().Logf("Cascading reasoning: %s", recommendation.Reasoning)
			}
		})
	}

	s.report.PassedTests++
}

// TestOllamaIntegration is the main test function
func TestOllamaIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	
	config := LoadConfig()
	if config.SkipIntegration {
		t.Skip("Integration tests skipped via SKIP_INTEGRATION environment variable")
	}
	
	suite.Run(t, new(OllamaIntegrationSuite))
}