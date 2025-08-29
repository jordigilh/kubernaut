//go:build integration
// +build integration

package integration

import (
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// Shared variables for all new actions integration tests
var (
	client     slm.Client
	slmConfig  config.SLMConfig
	testConfig shared.IntegrationConfig
	monitor    *shared.ResourceMonitor
	report     *shared.IntegrationTestReport
	logger     *logrus.Logger
	testEnv    *shared.TestEnvironment
)

// Helper function moved to database_test_utils.go to avoid duplication

var _ = BeforeSuite(func() {
	testConfig = shared.LoadConfig()

	if testConfig.SkipIntegration {
		Skip("Integration tests skipped via SKIP_INTEGRATION")
	}

	// Create logger
	logger = logrus.New()
	if level, err := logrus.ParseLevel(testConfig.LogLevel); err == nil {
		logger.SetLevel(level)
	}
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Load SLM configuration
	slmConfig = config.SLMConfig{
		Provider:    "localai",
		Endpoint:    testConfig.OllamaEndpoint,
		Model:       testConfig.OllamaModel,
		Temperature: 0.3,
		MaxTokens:   500,
		Timeout:     30 * time.Second,
		RetryCount:  testConfig.MaxRetries,
	}

	logger.WithFields(logrus.Fields{
		"endpoint": slmConfig.Endpoint,
		"model":    slmConfig.Model,
		"suite":    "NewActionsIntegration",
	}).Info("Setting up new actions integration test suite")

	// Create SLM client
	var err error
	client, err = slm.NewClient(slmConfig, logger)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SLM client")

	// Check health
	if !client.IsHealthy() {
		Skip("SLM client is not healthy, skipping new actions integration tests")
	}

	// Initialize monitoring and reporting
	monitor = shared.NewResourceMonitor()
	report = shared.NewIntegrationTestReport()

	// Setup test environment
	testEnv, err = shared.SetupFakeEnvironment()
	Expect(err).ToNot(HaveOccurred(), "Failed to setup test environment")
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		err := testEnv.Cleanup()
		if err != nil {
			logger.WithError(err).Error("Failed to cleanup test environment")
		}
	}

	// Generate final resource report
	resourceReport := monitor.GenerateReport()
	report.CalculateStats(resourceReport)

	logger.WithFields(logrus.Fields{
		"total_tests":  report.TotalTests,
		"passed_tests": report.PassedTests,
		"failed_tests": len(report.FailedTests),
		"avg_response": report.AverageResponse,
	}).Info("New actions integration test suite completed")
})

var _ = BeforeEach(func() {
	report.TotalTests++
})
