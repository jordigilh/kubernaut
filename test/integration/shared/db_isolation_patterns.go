//go:build integration
// +build integration

package shared

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
)

// DatabaseIsolationHelper provides common patterns for database-isolated tests
type DatabaseIsolationHelper struct {
	logger     *logrus.Logger
	dbUtils    *IsolatedDatabaseTestUtils
	testConfig IntegrationConfig
	strategy   IsolationStrategy
}

// SetDatabaseUtils sets the database utils for manual initialization
func (h *DatabaseIsolationHelper) SetDatabaseUtils(dbUtils *IsolatedDatabaseTestUtils) {
	h.dbUtils = dbUtils
}

// NewDatabaseIsolationHelper creates a new database isolation helper
func NewDatabaseIsolationHelper(logger *logrus.Logger, strategy IsolationStrategy) *DatabaseIsolationHelper {
	return &DatabaseIsolationHelper{
		logger:   logger,
		strategy: strategy,
	}
}

// SetupIsolatedDatabase sets up database isolation for a test suite
func (h *DatabaseIsolationHelper) SetupIsolatedDatabase(strategy IsolationStrategy) {
	BeforeAll(func() {
		h.testConfig = LoadConfig()
		if h.testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}
		if h.testConfig.SkipDatabaseTests {
			Skip("Database tests disabled via SKIP_DB_TESTS")
		}

		var err error
		h.dbUtils, err = NewIsolatedDatabaseTestUtils(h.logger, strategy)
		if err != nil {
			// Gracefully skip database tests if database is not available
			Skip(fmt.Sprintf("Skipping database tests - database unavailable: %v", err))
		}
	})

	AfterAll(func() {
		if h.dbUtils != nil {
			h.dbUtils.Close()
		}
	})

	BeforeEach(func() {
		h.logger.Debug("Starting isolated test with clean database state")
		if h.strategy == TransactionIsolation && h.dbUtils != nil {
			Expect(h.dbUtils.StartTest()).To(Succeed())
		}
	})

	AfterEach(func() {
		if h.strategy == TransactionIsolation && h.dbUtils != nil {
			Expect(h.dbUtils.EndTest()).To(Succeed())
		}
		h.logger.Debug("Test completed - database state isolated")
	})
}

// GetRepository returns the isolated database repository
func (h *DatabaseIsolationHelper) GetRepository() actionhistory.Repository {
	if h.dbUtils == nil {
		return nil
	}
	return h.dbUtils.GetRepository()
}

// GetDatabase returns the isolated database connection
func (h *DatabaseIsolationHelper) GetDatabase() interface{} {
	if h.dbUtils == nil {
		return nil
	}
	return h.dbUtils.GetDatabase()
}

// CreateFakeSLMClient creates a fake SLM client (no external dependencies)
func (h *DatabaseIsolationHelper) CreateFakeSLMClient() llm.Client {
	return NewFakeSLMClient()
}

// CreateBasicSLMClient creates a basic SLM client using test configuration
func (h *DatabaseIsolationHelper) CreateBasicSLMClient() (llm.Client, error) {
	// Configuration no longer needed for fake client

	// Use fake client to eliminate external dependencies - this method should not be used
	// Use CreateFakeSLMClient() instead for isolated testing
	return NewFakeSLMClient(), nil
}

// GetTestConfig returns the test configuration
func (h *DatabaseIsolationHelper) GetTestConfig() IntegrationConfig {
	return h.testConfig
}

// GetLogger returns the logger instance
func (h *DatabaseIsolationHelper) GetLogger() *logrus.Logger {
	return h.logger
}

// CreateTestActionHistory creates test action history data
func (h *DatabaseIsolationHelper) CreateTestActionHistory(resourceRef actionhistory.ResourceReference, numActions int) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()
	repo := h.GetRepository()

	// Ensure resource reference exists
	resourceID, err := repo.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	// Create action history
	_, err = repo.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	var traces []actionhistory.ResourceActionTrace
	baseTime := time.Now().Add(-time.Duration(numActions) * time.Minute)

	for i := 0; i < numActions; i++ {
		reasoning := fmt.Sprintf("Test reasoning for action %d", i)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("test-action-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        fmt.Sprintf("TestAlert%d", i%3),
				Severity:    h.getTestAlertSeverity(i),
				Labels:      map[string]string{"alertname": fmt.Sprintf("TestAlert%d", i%3)},
				Annotations: map[string]string{"description": fmt.Sprintf("Test alert %d", i)},
				FiringTime:  baseTime.Add(time.Duration(i) * time.Minute),
			},
			ActionType: h.getTestActionType(i),
			Parameters: map[string]interface{}{"replicas": i%3 + 1},
			Reasoning:  &reasoning,
			Confidence: 0.8,
		}

		trace, err := repo.StoreAction(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("failed to store test action %d: %w", i, err)
		}
		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateOscillationPattern creates an oscillation pattern for testing
func (h *DatabaseIsolationHelper) CreateOscillationPattern(resourceRef actionhistory.ResourceReference) error {
	ctx := context.Background()
	repo := h.GetRepository()

	baseTime := time.Now().Add(-2 * time.Hour)

	// Create alternating scale up/down actions to simulate oscillation
	actions := []struct {
		action string
		time   time.Duration
	}{
		{"scale_up", 0},
		{"scale_down", 15 * time.Minute},
		{"scale_up", 30 * time.Minute},
		{"scale_down", 45 * time.Minute},
		{"scale_up", 60 * time.Minute},
	}

	for i, actionInfo := range actions {
		reasoning := fmt.Sprintf("Oscillation test action %d", i)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("oscillation-action-%d", i),
			Timestamp:         baseTime.Add(actionInfo.time),
			Alert: actionhistory.AlertContext{
				Name:        "HighMemoryUsage",
				Severity:    "warning",
				Labels:      map[string]string{"alertname": "HighMemoryUsage"},
				Annotations: map[string]string{"description": "Memory usage oscillation test"},
				FiringTime:  baseTime.Add(actionInfo.time),
			},
			ActionType: actionInfo.action,
			Parameters: map[string]interface{}{"replicas": 3},
			Reasoning:  &reasoning,
			Confidence: 0.7,
		}

		_, err := repo.StoreAction(ctx, action)
		if err != nil {
			return fmt.Errorf("failed to store oscillation action %d: %w", i, err)
		}
	}

	return nil
}

// CreateIneffectiveSecurityHistory creates ineffective security action history
func (h *DatabaseIsolationHelper) CreateIneffectiveSecurityHistory(resourceRef actionhistory.ResourceReference) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()
	repo := h.GetRepository()

	baseTime := time.Now().Add(-3 * time.Hour)

	// Create history of failed security containment attempts
	reasoning := "Attempted containment of security threat"
	action := &actionhistory.ActionRecord{
		ResourceReference: resourceRef,
		ActionID:          "ineffective-security-1",
		Timestamp:         baseTime,
		Alert: actionhistory.AlertContext{
			Name:        "SecurityThreat",
			Severity:    "critical",
			Labels:      map[string]string{"alertname": "SecurityThreat"},
			Annotations: map[string]string{"description": "Security threat detected"},
			FiringTime:  baseTime,
		},
		ActionType: "quarantine_pod",
		Parameters: map[string]interface{}{"isolation": "network"},
		Reasoning:  &reasoning,
		Confidence: 0.9,
	}

	trace, err := repo.StoreAction(ctx, action)
	if err != nil {
		return nil, fmt.Errorf("failed to store ineffective security action: %w", err)
	}

	return []actionhistory.ResourceActionTrace{*trace}, nil
}

// CreateLowEffectivenessHistory creates low effectiveness history for an action
func (h *DatabaseIsolationHelper) CreateLowEffectivenessHistory(resourceRef actionhistory.ResourceReference, actionType string, effectiveness float64) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()
	repo := h.GetRepository()

	baseTime := time.Now().Add(-4 * time.Hour)
	var traces []actionhistory.ResourceActionTrace

	// Create multiple failed attempts of the same action type
	for i := 0; i < 3; i++ {
		reasoning := fmt.Sprintf("Low effectiveness test attempt %d", i+1)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("low-effectiveness-%s-%d", actionType, i),
			Timestamp:         baseTime.Add(time.Duration(i) * 30 * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "DiskSpaceLow",
				Severity:    "warning",
				Labels:      map[string]string{"alertname": "DiskSpaceLow"},
				Annotations: map[string]string{"description": "Disk space running low"},
				FiringTime:  baseTime.Add(time.Duration(i) * 30 * time.Minute),
			},
			ActionType: actionType,
			Parameters: map[string]interface{}{"size": "10Gi"},
			Reasoning:  &reasoning,
			Confidence: 0.4, // Low confidence due to effectiveness issues
		}

		trace, err := repo.StoreAction(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("failed to store low effectiveness action %d: %w", i, err)
		}
		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateCascadingFailureHistory creates cascading failure history
func (h *DatabaseIsolationHelper) CreateCascadingFailureHistory(resourceRef actionhistory.ResourceReference) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()
	repo := h.GetRepository()

	baseTime := time.Now().Add(-2 * time.Hour)
	var traces []actionhistory.ResourceActionTrace

	// Create a sequence of failed actions that led to cascading failures
	failureSequence := []struct {
		actionType string
		delay      time.Duration
	}{
		{"restart_pod", 0},
		{"scale_up", 15 * time.Minute},
		{"increase_resources", 30 * time.Minute},
		{"rollback_deployment", 45 * time.Minute},
	}

	for i, failure := range failureSequence {
		reasoning := fmt.Sprintf("Cascading failure response %d", i+1)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("cascading-failure-%d", i),
			Timestamp:         baseTime.Add(failure.delay),
			Alert: actionhistory.AlertContext{
				Name:        "ApplicationError",
				Severity:    "critical",
				Labels:      map[string]string{"alertname": "ApplicationError"},
				Annotations: map[string]string{"description": "Cascading failure scenario"},
				FiringTime:  baseTime.Add(failure.delay),
			},
			ActionType: failure.actionType,
			Parameters: map[string]interface{}{"attempt": i + 1},
			Reasoning:  &reasoning,
			Confidence: 0.3, // Low confidence due to cascading failures
		}

		trace, err := repo.StoreAction(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("failed to store cascading failure action %d: %w", i, err)
		}
		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateFailedRestartHistory creates a history of failed restart attempts
func (h *DatabaseIsolationHelper) CreateFailedRestartHistory(resourceRef actionhistory.ResourceReference, numAttempts int) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()
	repo := h.GetRepository()

	baseTime := time.Now().Add(-1 * time.Hour)
	var traces []actionhistory.ResourceActionTrace

	for i := 0; i < numAttempts; i++ {
		reasoning := fmt.Sprintf("Restart attempt %d for crash loop", i+1)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("failed-restart-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * 10 * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "PodCrashLooping",
				Severity:    "warning",
				Labels:      map[string]string{"alertname": "PodCrashLooping"},
				Annotations: map[string]string{"description": "Pod keeps crashing"},
				FiringTime:  baseTime.Add(time.Duration(i) * 10 * time.Minute),
			},
			ActionType: "restart_pod",
			Parameters: map[string]interface{}{"force": true},
			Reasoning:  &reasoning,
			Confidence: 0.6 - float64(i)*0.1, // Decreasing confidence with failed attempts
		}

		trace, err := repo.StoreAction(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("failed to store failed restart action %d: %w", i, err)
		}
		traces = append(traces, *trace)
	}

	return traces, nil
}

// Helper methods for test data generation
func (h *DatabaseIsolationHelper) getTestAlertSeverity(index int) string {
	severities := []string{"info", "warning", "critical"}
	return severities[index%len(severities)]
}

func (h *DatabaseIsolationHelper) getTestActionType(index int) string {
	actionTypes := []string{"restart_pod", "scale_up", "increase_resources", "collect_diagnostics"}
	return actionTypes[index%len(actionTypes)]
}

// IsolatedTestSuiteBuilder provides a fluent interface for building isolated test suites
type IsolatedTestSuiteBuilder struct {
	suiteName string
	logger    *logrus.Logger
	strategy  IsolationStrategy
	helper    *DatabaseIsolationHelper
}

// NewIsolatedTestSuite creates a new isolated test suite builder
func NewIsolatedTestSuite(suiteName string) *IsolatedTestSuiteBuilder {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &IsolatedTestSuiteBuilder{
		suiteName: suiteName,
		logger:    logger,
		strategy:  TransactionIsolation, // Default to fastest isolation
	}
}

// WithIsolationStrategy sets the database isolation strategy
func (b *IsolatedTestSuiteBuilder) WithIsolationStrategy(strategy IsolationStrategy) *IsolatedTestSuiteBuilder {
	b.strategy = strategy
	return b
}

// WithLogger sets a custom logger
func (b *IsolatedTestSuiteBuilder) WithLogger(logger *logrus.Logger) *IsolatedTestSuiteBuilder {
	b.logger = logger
	return b
}

// Build creates the test suite with database isolation
func (b *IsolatedTestSuiteBuilder) Build() *DatabaseIsolationHelper {
	b.helper = NewDatabaseIsolationHelper(b.logger, b.strategy)
	// Note: SetupIsolatedDatabase must be called manually to avoid Ginkgo nested BeforeAll issues
	return b.helper
}

// Example usage patterns for converting existing tests:

// ExampleTransactionIsolatedTest shows how to convert a test to use transaction isolation
func ExampleTransactionIsolatedTest() {
	var _ = Describe("Example Isolated Test Suite", Ordered, func() {
		var helper *DatabaseIsolationHelper

		BeforeAll(func() {
			helper = NewIsolatedTestSuite("Example Test").
				WithIsolationStrategy(TransactionIsolation).
				Build()
		})

		Context("when testing database operations", func() {
			It("should have clean database state", func() {
				// Use helper.GetRepository() instead of shared database
				// Use helper.CreateFakeSLMClient() instead of real clients

				repo := helper.GetRepository()
				Expect(repo).ToNot(BeNil())

				// Your test logic here - database will be automatically isolated
			})

			It("should not see data from previous test", func() {
				// Each test gets fresh database state automatically
				repo := helper.GetRepository()
				Expect(repo).ToNot(BeNil())

				// Test that verifies isolation
			})
		})
	})
}

// Migration helpers for updating existing test files
type TestFileMigrator struct {
	logger *logrus.Logger
}

func NewTestFileMigrator(logger *logrus.Logger) *TestFileMigrator {
	return &TestFileMigrator{logger: logger}
}

// Provides examples and patterns for migrating existing test files
func (m *TestFileMigrator) MigrationExamples() {
	// Example 1: Replace InitializeFreshDatabase patterns
	fmt.Println("BEFORE (coupled):")
	fmt.Println(`
	BeforeAll(func() {
		testUtils, err = shared.NewIntegrationTestUtils(logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(testUtils.InitializeFreshDatabase()).To(Succeed())
	})
	`)

	fmt.Println("AFTER (isolated):")
	fmt.Println(`
	BeforeAll(func() {
		helper = NewIsolatedTestSuite("My Test").
			WithIsolationStrategy(TransactionIsolation).
			Build()
	})
	`)

	// Example 2: Replace real client usage
	fmt.Println("BEFORE (real clients):")
	fmt.Println(`
	slmClient := NewFakeSLMClient()
	`)

	fmt.Println("AFTER (fake clients):")
	fmt.Println(`
	slmClient := helper.CreateFakeSLMClient()
	// Configure fake responses as needed
	`)

	// Example 3: Replace repository usage
	fmt.Println("BEFORE (shared repository):")
	fmt.Println(`
	repository := testUtils.GetRepository()
	`)

	fmt.Println("AFTER (isolated repository):")
	fmt.Println(`
	repository := helper.GetRepository()
	`)
}
