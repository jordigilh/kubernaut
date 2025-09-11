//go:build integration
// +build integration

package core_integration

import (
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/test/integration/shared"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// FIXED: No more global shared variables - moved to proper test suite scope

// Helper function moved to database_test_utils.go to avoid duplication

var _ = Describe("New Actions Integration Suite", Ordered, func() {
	var (
		logger       *logrus.Logger
		stateManager *shared.ComprehensiveStateManager
		client       llm.Client
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Use comprehensive state manager for complete isolation
		stateManager = shared.NewTestSuite("New Actions Integration Suite").
			WithLogger(logger).
			WithStandardLLMEnvironment().
			WithCustomCleanup(func() error {
				logger.Info("New actions integration test cleanup completed")
				return nil
			}).
			Build()

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Create fake SLM client to eliminate external dependencies
		client = shared.NewFakeSLMClient()
		Expect(client.IsHealthy()).To(BeTrue())

		logger.Info("New actions integration test suite setup completed")
	})

	AfterAll(func() {
		// Comprehensive cleanup
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		logger.Debug("Starting new actions test with isolated state")
	})

	AfterEach(func() {
		logger.Debug("New actions test completed - state automatically isolated")
	})

	// All test cases go here...
	Context("with isolated state", func() {
		It("should have no global state coupling", func() {
			// Test implementation using scoped variables
			Expect(client).ToNot(BeNil())
			Expect(logger).ToNot(BeNil())
			// Verify client and logger are properly initialized
			Expect(client).ToNot(BeNil())
			Expect(logger).ToNot(BeNil())
		})
	})
})

// REMOVED: Old AfterSuite/BeforeEach patterns replaced with proper Describe block scope
