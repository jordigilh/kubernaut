//go:build integration
// +build integration

package shared

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// ComprehensiveStateManager manages all forms of test state isolation
type ComprehensiveStateManager struct {
	logger        *logrus.Logger
	envHelper     *EnvironmentIsolationHelper
	dbHelper      *DatabaseIsolationHelper
	resourcesLock sync.Mutex
	cleanupFuncs  []func() error
}

// NewComprehensiveStateManager creates a new comprehensive state manager
func NewComprehensiveStateManager(logger *logrus.Logger) *ComprehensiveStateManager {
	return &ComprehensiveStateManager{
		logger:       logger,
		envHelper:    NewEnvironmentIsolationHelper(logger),
		cleanupFuncs: make([]func() error, 0),
	}
}

// WithDatabaseIsolation adds database isolation to the state manager
func (c *ComprehensiveStateManager) WithDatabaseIsolation(strategy IsolationStrategy) *ComprehensiveStateManager {
	c.dbHelper = NewDatabaseIsolationHelper(c.logger, strategy)
	return c
}

// WithEnvironmentIsolation configures environment variable isolation
func (c *ComprehensiveStateManager) WithEnvironmentIsolation(envVars ...string) *ComprehensiveStateManager {
	c.envHelper.CaptureEnvironment(envVars...)
	return c
}

// AddCleanupFunction adds a custom cleanup function
func (c *ComprehensiveStateManager) AddCleanupFunction(cleanupFunc func() error) {
	c.resourcesLock.Lock()
	defer c.resourcesLock.Unlock()

	c.cleanupFuncs = append(c.cleanupFuncs, cleanupFunc)
}

// GetEnvironmentHelper returns the environment isolation helper
func (c *ComprehensiveStateManager) GetEnvironmentHelper() *EnvironmentIsolationHelper {
	return c.envHelper
}

// GetDatabaseHelper returns the database isolation helper
func (c *ComprehensiveStateManager) GetDatabaseHelper() *DatabaseIsolationHelper {
	return c.dbHelper
}

// SetupTestIsolation performs comprehensive test isolation setup
func (c *ComprehensiveStateManager) SetupTestIsolation() {
	c.logger.Debug("Setting up comprehensive test isolation")

	// Environment isolation is already set up via WithEnvironmentIsolation

	// Database isolation setup
	if c.dbHelper != nil {
		c.logger.Debug("Initializing database isolation")
		// Initialize database utilities immediately
		var err error
		c.dbHelper.dbUtils, err = NewIsolatedDatabaseTestUtils(c.logger, c.dbHelper.strategy)
		if err != nil {
			c.logger.WithError(err).Error("Failed to initialize database isolation")
		}
	}
}

// CleanupAllState performs comprehensive cleanup of all managed state
func (c *ComprehensiveStateManager) CleanupAllState() error {
	c.resourcesLock.Lock()
	defer c.resourcesLock.Unlock()

	var errors []error

	// Run custom cleanup functions
	for i := len(c.cleanupFuncs) - 1; i >= 0; i-- {
		if err := c.cleanupFuncs[i](); err != nil {
			c.logger.WithError(err).Warn("Cleanup function failed")
			errors = append(errors, err)
		}
	}

	// Restore environment variables
	if c.envHelper != nil {
		c.envHelper.RestoreEnvironment()
	}

	// Database cleanup is handled by the database helper itself

	if len(errors) > 0 {
		c.logger.WithField("errors", len(errors)).Warn("Some cleanup operations failed")
		return errors[0] // Return first error
	}

	c.logger.Debug("Comprehensive state cleanup completed successfully")
	return nil
}

// StateIsolationBuilderPattern provides a fluent API for test state setup
type StateIsolationBuilder struct {
	suiteName string
	logger    *logrus.Logger
	manager   *ComprehensiveStateManager
}

// NewIsolatedTestSuiteV2 creates a new state isolation builder
func NewIsolatedTestSuiteV2(suiteName string) *StateIsolationBuilder {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &StateIsolationBuilder{
		suiteName: suiteName,
		logger:    logger,
		manager:   NewComprehensiveStateManager(logger),
	}
}

// WithLogger sets a custom logger
func (b *StateIsolationBuilder) WithLogger(logger *logrus.Logger) *StateIsolationBuilder {
	b.logger = logger
	b.manager.logger = logger
	b.manager.envHelper.logger = logger
	return b
}

// WithDatabaseIsolation adds database isolation
func (b *StateIsolationBuilder) WithDatabaseIsolation(strategy IsolationStrategy) *StateIsolationBuilder {
	b.manager.WithDatabaseIsolation(strategy)
	return b
}

// WithEnvironmentIsolation adds environment variable isolation
func (b *StateIsolationBuilder) WithEnvironmentIsolation(envVars ...string) *StateIsolationBuilder {
	b.manager.WithEnvironmentIsolation(envVars...)
	return b
}

// WithStandardLLMEnvironment adds standard LLM environment variable isolation
func (b *StateIsolationBuilder) WithStandardLLMEnvironment() *StateIsolationBuilder {
	b.manager.WithEnvironmentIsolation(StandardLLMEnvironmentVariables()...)
	return b
}

// WithCustomCleanup adds a custom cleanup function
func (b *StateIsolationBuilder) WithCustomCleanup(cleanupFunc func() error) *StateIsolationBuilder {
	b.manager.AddCleanupFunction(cleanupFunc)
	return b
}

// Build creates the comprehensive state manager
func (b *StateIsolationBuilder) Build() *ComprehensiveStateManager {
	b.logger.WithField("suite", b.suiteName).Info("Building comprehensive state isolation")
	b.manager.SetupTestIsolation()
	return b.manager
}

// TestIsolationPatterns provides common test isolation patterns
type TestIsolationPatterns struct{}

// DatabaseTransactionIsolatedSuite creates a database transaction isolated suite
func (t *TestIsolationPatterns) DatabaseTransactionIsolatedSuite(suiteName string) *ComprehensiveStateManager {
	return NewIsolatedTestSuiteV2(suiteName).
		WithDatabaseIsolation(TransactionIsolation).
		WithStandardLLMEnvironment().
		Build()
}

// FullyIsolatedSuite creates a fully isolated test suite
func (t *TestIsolationPatterns) FullyIsolatedSuite(suiteName string) *ComprehensiveStateManager {
	return NewIsolatedTestSuiteV2(suiteName).
		WithDatabaseIsolation(SchemaIsolation).
		WithEnvironmentIsolation(StandardIntegrationTestEnvironmentVariables()...).
		Build()
}

// LightweightIsolatedSuite creates a lightweight isolated suite
func (t *TestIsolationPatterns) LightweightIsolatedSuite(suiteName string) *ComprehensiveStateManager {
	return NewIsolatedTestSuiteV2(suiteName).
		WithDatabaseIsolation(TableTruncation).
		WithStandardLLMEnvironment().
		Build()
}
