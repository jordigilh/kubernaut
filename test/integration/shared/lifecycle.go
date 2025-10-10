//go:build integration
// +build integration

<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package shared

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// TestLifecycleHooks provides standardized test lifecycle management
type TestLifecycleHooks struct {
	suite       *StandardTestSuite
	logger      *logrus.Logger
	setupFunc   func()
	cleanupFunc func()
}

// SetupStandardIntegrationTest creates standard BeforeAll setup for integration tests
func SetupStandardIntegrationTest(suiteName string, opts ...TestOption) *TestLifecycleHooks {
	hooks := &TestLifecycleHooks{
		logger: logrus.New(),
	}

	BeforeAll(func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting standard integration test setup")

		// Check if integration tests should be skipped
		testConfig := LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Create test suite with options
		hooks.suite = NewStandardTestSuite(suiteName, opts...)

		// Setup the test suite
		err := hooks.suite.Setup()
		Expect(err).ToNot(HaveOccurred(), "Failed to setup standard test suite")

		hooks.logger.WithField("suite", suiteName).Info("Standard integration test setup completed")
	})

	AfterAll(func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting standard integration test cleanup")

		if hooks.suite != nil {
			err := hooks.suite.Cleanup()
			Expect(err).ToNot(HaveOccurred(), "Failed to cleanup standard test suite")
		}

		hooks.logger.WithField("suite", suiteName).Info("Standard integration test cleanup completed")
	})

	return hooks
}

// SetupDatabaseIntegrationTest creates standard setup for database-focused integration tests
func SetupDatabaseIntegrationTest(suiteName string, opts ...TestOption) *TestLifecycleHooks {
	// Add database-specific options
	dbOpts := []TestOption{
		WithRealDatabase(),
		WithDatabaseIsolation(TransactionIsolation),
	}
	dbOpts = append(dbOpts, opts...)

	hooks := &TestLifecycleHooks{
		logger: logrus.New(),
	}

	BeforeAll(func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting database integration test setup")

		// Check configuration
		testConfig := LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}
		if testConfig.SkipDatabaseTests {
			Skip("Database tests disabled via SKIP_DB_TESTS")
		}

		// Create test suite with database options
		hooks.suite = NewStandardTestSuite(suiteName, dbOpts...)

		// Setup the test suite
		err := hooks.suite.Setup()
		if err != nil {
			// Integration tests MUST fail if setup fails
			Fail(fmt.Sprintf("Integration test setup failed - real environment required: %v", err))
		}

		// Verify database is available
		if hooks.suite.DB == nil {
			Fail("Database unavailable - integration tests require real database connection")
		}

		hooks.logger.WithField("suite", suiteName).Info("Database integration test setup completed")
	})

	AfterAll(func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting database integration test cleanup")

		if hooks.suite != nil {
			err := hooks.suite.Cleanup()
			Expect(err).ToNot(HaveOccurred(), "Failed to cleanup database test suite")
		}

		hooks.logger.WithField("suite", suiteName).Info("Database integration test cleanup completed")
	})

	// Add database-specific lifecycle hooks
	BeforeEach(func() {
		hooks.logger.Debug("Starting database test with clean state")
		// Database isolation is handled by the state manager automatically
	})

	AfterEach(func() {
		hooks.logger.Debug("Database test completed - state automatically isolated")
		// Cleanup is handled by the state manager automatically
	})

	return hooks
}

// SetupPerformanceIntegrationTest creates standard setup for performance-focused integration tests
func SetupPerformanceIntegrationTest(suiteName string, opts ...TestOption) *TestLifecycleHooks {
	// Add performance-specific options
	perfOpts := []TestOption{
		WithPerformanceMonitoring(),
		WithRealDatabase(),
		WithRealVectorDB(),
	}
	perfOpts = append(perfOpts, opts...)

	hooks := &TestLifecycleHooks{
		logger: logrus.New(),
	}

	BeforeAll(func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting performance integration test setup")

		// Check configuration
		testConfig := LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}
		// Note: SKIP_PERFORMANCE_TESTS not currently in config structure
		// Add this field to IntegrationConfig if needed

		// Create test suite with performance options
		hooks.suite = NewStandardTestSuite(suiteName, perfOpts...)

		// Setup the test suite
		err := hooks.suite.Setup()
		Expect(err).ToNot(HaveOccurred(), "Failed to setup performance test suite")

		hooks.logger.WithField("suite", suiteName).Info("Performance integration test setup completed")
	})

	AfterAll(func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting performance integration test cleanup")

		// Generate performance report before cleanup
		if hooks.suite != nil {
			// TODO: Add performance reporting here if available
			hooks.logger.Info("Performance test results logged")
		}

		if hooks.suite != nil {
			err := hooks.suite.Cleanup()
			Expect(err).ToNot(HaveOccurred(), "Failed to cleanup performance test suite")
		}

		hooks.logger.WithField("suite", suiteName).Info("Performance integration test cleanup completed")
	})

	return hooks
}

// SetupAIIntegrationTest creates standard setup for AI-focused integration tests
func SetupAIIntegrationTest(suiteName string, opts ...TestOption) *TestLifecycleHooks {
	// Add AI-specific options
	aiOpts := []TestOption{
		WithMockLLM(), // Use mock by default for speed
		WithRealVectorDB(),
		WithRealDatabase(),
	}
	aiOpts = append(aiOpts, opts...)

	hooks := &TestLifecycleHooks{
		logger: logrus.New(),
	}

	// Setup function to be called in test's BeforeAll
	hooks.setupFunc = func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting AI integration test setup")

		// Check configuration
		testConfig := LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Create test suite with AI options
		hooks.suite = NewStandardTestSuite(suiteName, aiOpts...)

		// Setup the test suite
		err := hooks.suite.Setup()
		Expect(err).ToNot(HaveOccurred(), "Failed to setup AI test suite")

		// Verify AI components are available
		Expect(hooks.suite.LLMClient).ToNot(BeNil(), "LLM client should be available")
		Expect(hooks.suite.LLMClient.IsHealthy()).To(BeTrue(), "LLM client should be healthy")

		if hooks.suite.WorkflowBuilder != nil {
			hooks.logger.Debug("AI workflow builder is available")
		}

		hooks.logger.WithField("suite", suiteName).Info("AI integration test setup completed")
	}

	// Cleanup function to be called in test's AfterAll
	hooks.cleanupFunc = func() {
		hooks.logger.WithField("suite", suiteName).Info("Starting AI integration test cleanup")

		if hooks.suite != nil {
			err := hooks.suite.Cleanup()
			Expect(err).ToNot(HaveOccurred(), "Failed to cleanup AI test suite")
		}

		hooks.logger.WithField("suite", suiteName).Info("AI integration test cleanup completed")
	}

	return hooks
}

// Setup calls the setup function (to be used in BeforeAll)
func (h *TestLifecycleHooks) Setup() {
	if h.setupFunc != nil {
		h.setupFunc()
	}
}

// Cleanup calls the cleanup function (to be used in AfterAll)
func (h *TestLifecycleHooks) Cleanup() {
	if h.cleanupFunc != nil {
		h.cleanupFunc()
	}
}

// GetSuite returns the underlying test suite for direct access
func (h *TestLifecycleHooks) GetSuite() *StandardTestSuite {
	return h.suite
}

// GetLogger returns the logger for the test lifecycle
func (h *TestLifecycleHooks) GetLogger() *logrus.Logger {
	return h.logger
}

// WithTestSpecificCleanup adds test-specific cleanup to the lifecycle
func (h *TestLifecycleHooks) WithTestSpecificCleanup(cleanupFunc func() error) {
	AfterEach(func() {
		err := cleanupFunc()
		Expect(err).ToNot(HaveOccurred(), "Test-specific cleanup should succeed")
	})
}

// WithTestSpecificSetup adds test-specific setup to the lifecycle
// NOTE: This function is deprecated due to Ginkgo timing issues.
// Call setupFunc directly in your test's BeforeEach instead.
func (h *TestLifecycleHooks) WithTestSpecificSetup(setupFunc func() error) {
	// Deprecated: Do not call this function as it causes Ginkgo timing issues
	h.logger.Warn("WithTestSpecificSetup is deprecated - call your setup function directly in BeforeEach")
}
