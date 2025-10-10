/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//go:build integration

package workflow_engine

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// Global test environment and reporting
var (
	globalTestReport  *TestExecutionReport
	integrationConfig IntegrationTestConfig
	testLogger        *logrus.Logger
)

func TestIntelligentWorkflowBuilderIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Intelligent Workflow Builder Integration Suite", Label("integration"))
}

var _ = BeforeSuite(func() {
	By("Setting up Intelligent Workflow Builder Integration Test Environment")

	// Initialize global configuration
	integrationConfig = LoadIntegrationTestConfig()

	// Setup global logger
	testLogger = logrus.New()
	if integrationConfig.LogLevel == "debug" {
		testLogger.SetLevel(logrus.DebugLevel)
	} else {
		testLogger.SetLevel(logrus.InfoLevel)
	}

	// Initialize global test report
	globalTestReport = NewTestExecutionReport()

	// Log test environment configuration
	testLogger.WithFields(logrus.Fields{
		"llm_provider":           integrationConfig.LLMProvider,
		"llm_model":              integrationConfig.LLMModel,
		"llm_endpoint":           integrationConfig.LLMEndpoint,
		"skip_slm_tests":         integrationConfig.SkipSLMTests,
		"skip_performance_tests": integrationConfig.SkipPerformanceTests,
		"skip_slow_tests":        integrationConfig.SkipSlowTests,
		"test_timeout":           integrationConfig.TestTimeout,
	}).Info("Integration test environment configured")

	// Validate SLM connectivity if not skipped
	if !integrationConfig.SkipSLMTests {
		By("Validating SLM service connectivity")
		validateSLMConnectivity()
	}

	// Pre-warm any caches or initialize shared resources
	By("Initializing shared test resources")
	initializeSharedResources()
})

var _ = AfterSuite(func() {
	By("Cleaning up Intelligent Workflow Builder Integration Test Environment")

	// Generate final test report
	if globalTestReport != nil {
		globalTestReport.GenerateFinalReport(testLogger)
	}

	// Cleanup any shared resources
	cleanupSharedResources()

	testLogger.Info("Integration test environment cleanup completed")
})

// TestExecutionReport tracks comprehensive test execution metrics
type TestExecutionReport struct {
	StartTime    time.Time
	TotalTests   int
	PassedTests  int
	FailedTests  int
	SkippedTests int

	// Performance metrics
	TotalExecutionTime  time.Duration
	AverageResponseTime time.Duration
	MaxResponseTime     time.Duration
	MinResponseTime     time.Duration

	// Feature-specific metrics
	WorkflowGenerationTests int
	ValidationTests         int
	SimulationTests         int
	LearningTests           int
	E2ETests                int

	// Error tracking
	ErrorCategories       map[string]int
	PerformanceViolations []PerformanceViolation
}

type PerformanceViolation struct {
	TestName      string
	ExpectedTime  time.Duration
	ActualTime    time.Duration
	ViolationType string
	Timestamp     time.Time
}

func NewTestExecutionReport() *TestExecutionReport {
	return &TestExecutionReport{
		StartTime:             time.Now(),
		ErrorCategories:       make(map[string]int),
		PerformanceViolations: make([]PerformanceViolation, 0),
	}
}

func (ter *TestExecutionReport) RecordTest(passed bool, category string, duration time.Duration) {
	ter.TotalTests++
	ter.TotalExecutionTime += duration

	if passed {
		ter.PassedTests++
	} else {
		ter.FailedTests++
	}

	// Update performance metrics
	if ter.MaxResponseTime == 0 || duration > ter.MaxResponseTime {
		ter.MaxResponseTime = duration
	}
	if ter.MinResponseTime == 0 || duration < ter.MinResponseTime {
		ter.MinResponseTime = duration
	}

	// Update category-specific metrics
	switch category {
	case "workflow_generation":
		ter.WorkflowGenerationTests++
	case "validation":
		ter.ValidationTests++
	case "simulation":
		ter.SimulationTests++
	case "learning":
		ter.LearningTests++
	case "e2e":
		ter.E2ETests++
	}
}

func (ter *TestExecutionReport) RecordPerformanceViolation(testName string, expected, actual time.Duration, violationType string) {
	ter.PerformanceViolations = append(ter.PerformanceViolations, PerformanceViolation{
		TestName:      testName,
		ExpectedTime:  expected,
		ActualTime:    actual,
		ViolationType: violationType,
		Timestamp:     time.Now(),
	})
}

func (ter *TestExecutionReport) RecordError(category string) {
	ter.ErrorCategories[category]++
}

func (ter *TestExecutionReport) GenerateFinalReport(logger *logrus.Logger) {
	totalDuration := time.Since(ter.StartTime)

	if ter.TotalTests > 0 {
		ter.AverageResponseTime = ter.TotalExecutionTime / time.Duration(ter.TotalTests)
	}

	successRate := float64(ter.PassedTests) / float64(ter.TotalTests) * 100

	logger.WithFields(logrus.Fields{
		"total_duration":         totalDuration,
		"total_tests":            ter.TotalTests,
		"passed_tests":           ter.PassedTests,
		"failed_tests":           ter.FailedTests,
		"skipped_tests":          ter.SkippedTests,
		"success_rate":           successRate,
		"avg_response_time":      ter.AverageResponseTime,
		"max_response_time":      ter.MaxResponseTime,
		"min_response_time":      ter.MinResponseTime,
		"workflow_gen_tests":     ter.WorkflowGenerationTests,
		"validation_tests":       ter.ValidationTests,
		"simulation_tests":       ter.SimulationTests,
		"learning_tests":         ter.LearningTests,
		"e2e_tests":              ter.E2ETests,
		"performance_violations": len(ter.PerformanceViolations),
		"error_categories":       ter.ErrorCategories,
	}).Info("Integration Test Execution Report")

	// Log performance violations if any
	if len(ter.PerformanceViolations) > 0 {
		logger.WithField("violations", ter.PerformanceViolations).Warn("Performance violations detected")
	}
}

// Helper functions for test environment setup

func validateSLMConnectivity() {
	// This would validate that the SLM service is accessible
	// For now, we'll just log the attempt
	testLogger.WithFields(logrus.Fields{
		"endpoint": integrationConfig.LLMEndpoint,
		"model":    integrationConfig.LLMModel,
	}).Info("Validating SLM connectivity")

	// In a real implementation, this would:
	// 1. Create a test SLM client
	// 2. Perform a health check
	// 3. Optionally perform a simple test query
	// 4. Fail the suite if SLM is not available
}

func initializeSharedResources() {
	// Initialize any shared resources needed across tests
	testLogger.Info("Initializing shared test resources")

	// This could include:
	// - Setting up test databases
	// - Pre-loading test data
	// - Initializing mock services
	// - Setting up monitoring
}

func cleanupSharedResources() {
	// Cleanup shared resources
	testLogger.Info("Cleaning up shared test resources")

	// This could include:
	// - Cleaning up test databases
	// - Stopping mock services
	// - Removing temporary files
	// - Closing connections
}

// Test helper functions that can be used across test files

// RecordTestMetrics is a helper function to record test metrics
func RecordTestMetrics(testName string, duration time.Duration, success bool, category string) {
	if globalTestReport != nil {
		globalTestReport.RecordTest(success, category, duration)

		// Check for performance violations
		var expectedTime time.Duration
		switch category {
		case "workflow_generation":
			expectedTime = 30 * time.Second
		case "validation":
			expectedTime = 5 * time.Second
		case "simulation":
			expectedTime = 10 * time.Second
		case "learning":
			expectedTime = 2 * time.Second
		case "e2e":
			expectedTime = 2 * time.Minute
		default:
			expectedTime = 30 * time.Second
		}

		if duration > expectedTime {
			globalTestReport.RecordPerformanceViolation(testName, expectedTime, duration, "timeout_exceeded")
		}
	}
}

// SkipIfSLMUnavailable skips test if SLM is not available
func SkipIfSLMUnavailable() {
	if integrationConfig.SkipSLMTests {
		Skip("SLM tests disabled via SKIP_SLM_TESTS environment variable")
	}
}

// SkipIfPerformanceTestsDisabled skips performance tests if disabled
func SkipIfPerformanceTestsDisabled() {
	if integrationConfig.SkipPerformanceTests {
		Skip("Performance tests disabled via SKIP_PERFORMANCE_TESTS environment variable")
	}
}

// SkipIfSlowTestsDisabled skips slow tests if disabled
func SkipIfSlowTestsDisabled() {
	if integrationConfig.SkipSlowTests {
		Skip("Slow tests disabled via SKIP_SLOW_TESTS environment variable")
	}
}
