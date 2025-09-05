//go:build integration
// +build integration

package shared

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// ExampleIsolatedTest demonstrates proper state isolation patterns
var _ = Describe("Example: Proper State Isolation Patterns", Ordered, func() {
	var (
		stateManager *ComprehensiveStateManager
		logger       *logrus.Logger
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Create comprehensive state manager with all isolation features
		stateManager = NewIsolatedTestSuiteV2("Example Isolated Test Suite").
			WithLogger(logger).
			WithDatabaseIsolation(TransactionIsolation).
			WithStandardLLMEnvironment().
			WithCustomCleanup(func() error {
				logger.Info("Custom cleanup executed")
				return nil
			}).
			Build()
	})

	AfterAll(func() {
		// Comprehensive cleanup of all managed state
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Environment Variable Isolation", func() {
		AfterEach(func() {
			// Ensure environment variables are restored after each test
			if envHelper := stateManager.GetEnvironmentHelper(); envHelper != nil {
				envHelper.RestoreEnvironment()
			}
		})
		It("should isolate environment variable changes", func() {
			envHelper := stateManager.GetEnvironmentHelper()

			// Set test-specific environment variables
			envHelper.SetEnvironment(map[string]string{
				"LLM_ENDPOINT": "http://test-isolation:8080",
				"LLM_MODEL":    "test-isolation-model",
			})

			// Verify changes are isolated to this test
			cfg := LoadConfig()
			Expect(cfg.LLMEndpoint).To(Equal("http://test-isolation:8080"))
			Expect(cfg.LLMModel).To(Equal("test-isolation-model"))
		})

		It("should not see environment changes from previous test", func() {
			// This test should not see the environment changes from the previous test
			// due to automatic restoration in AfterEach
			cfg := LoadConfig()

			// Should see original values, not the test-specific ones
			Expect(cfg.LLMEndpoint).ToNot(Equal("http://test-isolation:8080"))
			Expect(cfg.LLMModel).ToNot(Equal("test-isolation-model"))
		})
	})

	Context("Database Isolation", func() {
		It("should have isolated database state", func() {
			dbHelper := stateManager.GetDatabaseHelper()

			if dbHelper != nil {
				repository := dbHelper.GetRepository()
				Expect(repository).ToNot(BeNil())

				// Database operations are isolated per transaction
				// Changes in this test won't affect other tests
			} else {
				Skip("Database isolation not configured for this test")
			}
		})
	})

	Context("Resource Cleanup", func() {
		It("should properly cleanup all managed resources", func() {
			// Custom cleanup functions are automatically executed
			// Environment variables are automatically restored
			// Database transactions are automatically rolled back

			// Verify that cleanup mechanisms are in place
			Expect(stateManager).ToNot(BeNil())
			Expect(stateManager.GetEnvironmentHelper()).ToNot(BeNil())
		})
	})
})

// ExampleConversionPattern demonstrates how to convert legacy tests
var _ = Describe("Example: Converting Legacy Test Patterns", Ordered, func() {

	// BEFORE (Legacy Pattern with Global State):
	// var globalClient llm.Client
	// var globalConfig config.Config
	//
	// var _ = BeforeSuite(func() {
	//     globalClient = llm.NewClient(...)
	//     globalConfig = loadConfig()
	// })

	// AFTER (Isolated Pattern):
	var stateManager *ComprehensiveStateManager

	BeforeAll(func() {
		// Use pattern helper for common configurations
		patterns := &TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Converted Legacy Test")
	})

	AfterAll(func() {
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should demonstrate isolated state patterns", func() {
		// All state is properly isolated
		// No global variables
		// Automatic cleanup

		envHelper := stateManager.GetEnvironmentHelper()
		Expect(envHelper).ToNot(BeNil())

		// Test-specific configuration without global pollution
		envHelper.SetEnvironment(map[string]string{
			"TEST_SPECIFIC_VAR": "isolated-value",
		})

		// Use fake clients instead of real clients for full isolation
		fakeClient := NewFakeSLMClient()
		Expect(fakeClient.IsHealthy()).To(BeTrue())
	})
})
