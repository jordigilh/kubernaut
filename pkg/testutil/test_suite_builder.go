package testutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

// TestSuiteBuilder provides a fluent interface for setting up unit tests
type TestSuiteBuilder struct {
	suiteName     string
	withK8sClient bool
	withLogger    bool
	withTestEnv   bool
	logLevel      logrus.Level
	customSetup   []func() error
	customCleanup []func() error
}

// TestSuiteComponents contains all common test components
type TestSuiteComponents struct {
	Context   context.Context
	Logger    *logrus.Logger
	TestEnv   *testenv.TestEnvironment
	K8sClient k8s.Client
}

// NewTestSuiteBuilder creates a new test suite builder
func NewTestSuiteBuilder(suiteName string) *TestSuiteBuilder {
	return &TestSuiteBuilder{
		suiteName:     suiteName,
		withLogger:    true,              // Default to having logger
		withTestEnv:   false,             // Only add when needed
		withK8sClient: false,             // Only add when needed
		logLevel:      logrus.FatalLevel, // Suppress logs by default
		customSetup:   make([]func() error, 0),
		customCleanup: make([]func() error, 0),
	}
}

// WithK8sClient enables K8s client setup
func (b *TestSuiteBuilder) WithK8sClient() *TestSuiteBuilder {
	b.withK8sClient = true
	b.withTestEnv = true // K8s client requires test env
	return b
}

// WithTestEnvironment enables test environment setup
func (b *TestSuiteBuilder) WithTestEnvironment() *TestSuiteBuilder {
	b.withTestEnv = true
	return b
}

// WithLogLevel sets the log level
func (b *TestSuiteBuilder) WithLogLevel(level logrus.Level) *TestSuiteBuilder {
	b.logLevel = level
	return b
}

// WithCustomSetup adds custom setup function
func (b *TestSuiteBuilder) WithCustomSetup(setupFunc func() error) *TestSuiteBuilder {
	b.customSetup = append(b.customSetup, setupFunc)
	return b
}

// WithCustomCleanup adds custom cleanup function
func (b *TestSuiteBuilder) WithCustomCleanup(cleanupFunc func() error) *TestSuiteBuilder {
	b.customCleanup = append(b.customCleanup, cleanupFunc)
	return b
}

// Build sets up the test suite and returns components
func (b *TestSuiteBuilder) Build() *TestSuiteComponents {
	components := &TestSuiteComponents{}

	BeforeEach(func() {
		var err error

		// Setup context
		components.Context = context.Background()

		// Setup logger
		if b.withLogger {
			components.Logger = logrus.New()
			components.Logger.SetLevel(b.logLevel)
		}

		// Setup test environment
		if b.withTestEnv {
			components.TestEnv, err = testenv.SetupEnvironment()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to setup test environment")
			gomega.Expect(components.TestEnv).NotTo(gomega.BeNil())

			// Create default namespace
			err = components.TestEnv.CreateDefaultNamespace()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create default namespace")
		}

		// Setup K8s client
		if b.withK8sClient && components.TestEnv != nil {
			components.K8sClient = components.TestEnv.CreateK8sClient(components.Logger)
			gomega.Expect(components.K8sClient).NotTo(gomega.BeNil())
		}

		// Run custom setup functions
		for _, setupFunc := range b.customSetup {
			err := setupFunc()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Custom setup function failed")
		}
	})

	AfterEach(func() {
		// Run custom cleanup functions
		for _, cleanupFunc := range b.customCleanup {
			err := cleanupFunc()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Custom cleanup function failed")
		}

		// Cleanup test environment
		if components.TestEnv != nil {
			err := components.TestEnv.Cleanup()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to cleanup test environment")
		}
	})

	return components
}

// Common test setup patterns

// StandardUnitTestSuite creates a standard unit test setup (logger only)
func StandardUnitTestSuite(suiteName string) *TestSuiteComponents {
	return NewTestSuiteBuilder(suiteName).Build()
}

// K8sUnitTestSuite creates a unit test setup with K8s client
func K8sUnitTestSuite(suiteName string) *TestSuiteComponents {
	return NewTestSuiteBuilder(suiteName).
		WithK8sClient().
		Build()
}

// AITestSuite creates a test setup optimized for AI component testing
func AITestSuite(suiteName string) *TestSuiteComponents {
	return NewTestSuiteBuilder(suiteName).
		WithK8sClient().
		WithLogLevel(logrus.FatalLevel).
		Build()
}
