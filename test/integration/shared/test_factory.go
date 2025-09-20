//go:build integration
// +build integration

package shared

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
	"github.com/sirupsen/logrus"
)

// MockAIMetricsCollector provides a test implementation of engine.AIMetricsCollector
type MockAIMetricsCollector struct{}

func (m *MockAIMetricsCollector) CollectMetrics(ctx context.Context, execution *engine.RuntimeWorkflowExecution) (map[string]float64, error) {
	return map[string]float64{
		"execution_time": execution.Duration.Seconds(),
		"step_count":     float64(len(execution.Steps)),
		"success_rate":   execution.GetSuccessRate(),
	}, nil
}

func (m *MockAIMetricsCollector) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange engine.WorkflowTimeRange) (map[string]float64, error) {
	return map[string]float64{
		"average_execution_time": 120.0,
		"total_executions":       50.0,
		"overall_success_rate":   0.85,
	}, nil
}

func (m *MockAIMetricsCollector) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	return nil
}

func (m *MockAIMetricsCollector) EvaluateResponseQuality(ctx context.Context, response string, context map[string]interface{}) (*engine.AIResponseQuality, error) {
	return &engine.AIResponseQuality{
		Score:      0.85,
		Confidence: 0.9,
		Relevance:  0.8,
		Clarity:    0.85,
	}, nil
}

// AnalyticsEngineAdapter adapts insights.AnalyticsEngineImpl to types.AnalyticsEngine
type AnalyticsEngineAdapter struct {
	engine *insights.AnalyticsEngineImpl
}

// AnalyzeData implements types.AnalyticsEngine interface
func (a *AnalyticsEngineAdapter) AnalyzeData() error {
	// Delegate to the underlying engine
	return a.engine.AnalyzeData()
}

// AnalyzeWorkflowEffectiveness implements types.AnalyticsEngine interface
func (a *AnalyticsEngineAdapter) AnalyzeWorkflowEffectiveness(ctx context.Context, execution *types.RuntimeWorkflowExecution) (*types.EffectivenessReport, error) {
	// Create a basic effectiveness report from execution data
	report := &types.EffectivenessReport{
		ID:          fmt.Sprintf("effectiveness-%s", execution.ID),
		ExecutionID: execution.ID,
		Score:       0.85, // Mock success score
		Metadata: map[string]interface{}{
			"summary":      fmt.Sprintf("Effectiveness analysis for execution %s", execution.ID),
			"generated_at": time.Now(),
		},
	}

	return report, nil
}

// GetAnalyticsInsights implements types.AnalyticsEngine interface
func (a *AnalyticsEngineAdapter) GetAnalyticsInsights(ctx context.Context, timeWindow time.Duration) (*types.AnalyticsInsights, error) {
	// Delegate to the underlying engine
	return a.engine.GetAnalyticsInsights(ctx, timeWindow)
}

// GetPatternAnalytics implements types.AnalyticsEngine interface
func (a *AnalyticsEngineAdapter) GetPatternAnalytics(ctx context.Context, filters map[string]interface{}) (*types.PatternAnalytics, error) {
	// Delegate to the underlying engine
	return a.engine.GetPatternAnalytics(ctx, filters)
}

// GetPatternInsights implements types.AnalyticsEngine interface
func (a *AnalyticsEngineAdapter) GetPatternInsights(ctx context.Context, patternID string) (*types.PatternInsights, error) {
	insights := &types.PatternInsights{
		PatternID:     patternID,
		Effectiveness: 0.75,
		UsageCount:    10,
		Insights:      []string{"Pattern shows consistent performance", "Recommended for similar scenarios"},
		Metrics: map[string]interface{}{
			"success_rate":     0.85,
			"average_duration": "2m30s",
			"resource_usage":   "moderate",
		},
	}

	return insights, nil
}

// Helper function to convert llm.AnalyzeAlertResponse to types.ActionRecommendation
func ConvertAnalyzeAlertResponse(response *llm.AnalyzeAlertResponse) *types.ActionRecommendation {
	if response == nil {
		return nil
	}

	return &types.ActionRecommendation{
		Action:     response.Action,
		Parameters: response.Parameters,
		Confidence: response.Confidence,
		Reasoning:  response.Reasoning,
	}
}

// Helper method for generating insights
func (a *AnalyticsEngineAdapter) generateInsights(execution *engine.RuntimeWorkflowExecution) []string {
	insights := make([]string, 0)

	if execution.IsSuccessful() {
		insights = append(insights, "Workflow completed successfully")
	} else if execution.IsFailed() {
		insights = append(insights, "Workflow failed - review error details")
	}

	if execution.GetSuccessRate() > 0.8 {
		insights = append(insights, "High step success rate indicates good workflow design")
	}

	return insights
}

// PatternStoreAdapter adapts StandardPatternStore to engine.PatternStore
type PatternStoreAdapter struct {
	store *StandardPatternStore
}

// StorePattern implements engine.PatternStore interface
func (a *PatternStoreAdapter) StorePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	return a.store.StoreEnginePattern(ctx, pattern)
}

// GetPattern implements engine.PatternStore interface
func (a *PatternStoreAdapter) GetPattern(ctx context.Context, patternID string) (*types.DiscoveredPattern, error) {
	return a.store.GetPattern(ctx, patternID)
}

// ListPatterns implements engine.PatternStore interface
func (a *PatternStoreAdapter) ListPatterns(ctx context.Context, patternType string) ([]*types.DiscoveredPattern, error) {
	return a.store.ListPatterns(ctx, patternType)
}

// DeletePattern implements engine.PatternStore interface
func (a *PatternStoreAdapter) DeletePattern(ctx context.Context, patternID string) error {
	return a.store.DeletePattern(ctx, patternID)
}

// CreatePatternStoreForTesting creates a standardized pattern store for testing
// This consolidates the duplicate createPatternStore functions across test files
func CreatePatternStoreForTesting(logger *logrus.Logger) engine.PatternStore {
	// Business Contract: Create pattern store for workflow builder testing
	// Following guideline: REUSE existing code and AVOID duplication (Principle #13)
	standardStore := NewStandardPatternStore(logger)
	return &PatternStoreAdapter{
		store: standardStore,
	}
}

// StandardTestSuite provides common test setup for all integration tests
type StandardTestSuite struct {
	SuiteName        string
	Logger           *logrus.Logger
	Ctx              context.Context
	StateManager     *ComprehensiveStateManager
	TestEnv          *testenv.TestEnvironment
	DB               *sql.DB
	LLMClient        llm.Client
	VectorDB         vector.VectorDatabase
	AnalyticsEngine  types.AnalyticsEngine
	MetricsCollector engine.AIMetricsCollector
	ExecutionRepo    engine.ExecutionRepository
	WorkflowBuilder  *engine.DefaultIntelligentWorkflowBuilder

	// Configuration options
	config *TestSuiteConfig
}

// TestSuiteConfig holds configuration for test suite setup
type TestSuiteConfig struct {
	UseRealDatabase    bool
	UseRealLLM         bool
	UseRealVectorDB    bool
	DatabaseIsolation  IsolationStrategy
	EnablePerformance  bool
	EnableDebugLogging bool
	SkipCleanup        bool
	CustomCleanup      func() error
}

// TestOption allows for flexible configuration of test suites
type TestOption func(*TestSuiteConfig)

// NewStandardTestSuite creates a complete test environment with sensible defaults
func NewStandardTestSuite(suiteName string, opts ...TestOption) *StandardTestSuite {
	// Default configuration
	config := &TestSuiteConfig{
		UseRealDatabase:    true,
		UseRealLLM:         false, // Default to mock for speed
		UseRealVectorDB:    true,
		DatabaseIsolation:  TransactionIsolation,
		EnablePerformance:  false,
		EnableDebugLogging: false,
		SkipCleanup:        false,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Create logger
	logger := logrus.New()
	if config.EnableDebugLogging {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	return &StandardTestSuite{
		SuiteName: suiteName,
		Logger:    logger,
		Ctx:       context.Background(),
		config:    config,
	}
}

// Setup initializes all test components
func (s *StandardTestSuite) Setup() error {
	s.Logger.WithField("suite", s.SuiteName).Info("Setting up standard test suite")

	// Setup state manager
	builder := NewTestSuite(s.SuiteName).
		WithLogger(s.Logger).
		WithStandardLLMEnvironment()

	if s.config.UseRealDatabase {
		builder = builder.WithDatabaseIsolation(s.config.DatabaseIsolation)
	}

	if s.config.CustomCleanup != nil {
		builder = builder.WithCustomCleanup(s.config.CustomCleanup)
	}

	s.StateManager = builder.Build()

	// Setup test environment
	var err error
	s.TestEnv, err = testenv.SetupEnvironment()
	if err != nil {
		return fmt.Errorf("failed to setup test environment: %w", err)
	}

	// Setup database if needed
	if s.config.UseRealDatabase {
		if err := s.setupDatabase(); err != nil {
			return fmt.Errorf("failed to setup database: %w", err)
		}
	}

	// Setup LLM client
	if err := s.setupLLMClient(); err != nil {
		return fmt.Errorf("failed to setup LLM client: %w", err)
	}

	// Setup vector database if needed
	if s.config.UseRealVectorDB && s.DB != nil {
		if err := s.setupVectorDB(); err != nil {
			return fmt.Errorf("failed to setup vector database: %w", err)
		}
	}

	// Setup analytics engine and workflow components
	if err := s.setupWorkflowComponents(); err != nil {
		return fmt.Errorf("failed to setup workflow components: %w", err)
	}

	s.Logger.WithField("suite", s.SuiteName).Info("Standard test suite setup completed")
	return nil
}

// Cleanup handles all cleanup tasks
func (s *StandardTestSuite) Cleanup() error {
	if s.config.SkipCleanup {
		return nil
	}

	s.Logger.WithField("suite", s.SuiteName).Info("Cleaning up standard test suite")

	var errors []error

	// Cleanup test environment
	if s.TestEnv != nil {
		if err := s.TestEnv.Cleanup(); err != nil {
			errors = append(errors, fmt.Errorf("test environment cleanup failed: %w", err))
		}
	}

	// State manager cleanup is handled automatically
	if s.StateManager != nil {
		if err := s.StateManager.CleanupAllState(); err != nil {
			errors = append(errors, fmt.Errorf("state manager cleanup failed: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %v", errors)
	}

	s.Logger.WithField("suite", s.SuiteName).Info("Standard test suite cleanup completed")
	return nil
}

// setupDatabase initializes database connection
func (s *StandardTestSuite) setupDatabase() error {
	dbHelper := s.StateManager.GetDatabaseHelper()
	if dbHelper == nil {
		s.Logger.Warn("Database helper unavailable, tests will run without database functionality")
		return nil // Graceful degradation
	}

	dbInterface := dbHelper.GetDatabase()
	if dbInterface == nil {
		s.Logger.Warn("Database connection unavailable, tests will run without database functionality")
		return nil // Graceful degradation
	}

	var ok bool
	s.DB, ok = dbInterface.(*sql.DB)
	if !ok {
		return fmt.Errorf("database is not PostgreSQL")
	}

	return nil
}

// setupLLMClient initializes LLM client
func (s *StandardTestSuite) setupLLMClient() error {
	if s.config.UseRealLLM {
		// For real LLM, we'd need additional configuration
		s.LLMClient = NewSLMClient()
	} else {
		// Use mock client for faster tests
		s.LLMClient = NewFakeSLMClient()
	}

	if !s.LLMClient.IsHealthy() {
		return fmt.Errorf("LLM client is not healthy")
	}

	return nil
}

// setupVectorDB initializes vector database
func (s *StandardTestSuite) setupVectorDB() error {
	vectorConfig := StandardVectorDBConfig()
	factory := vector.NewVectorDatabaseFactory(vectorConfig, s.DB, s.Logger)

	var err error
	s.VectorDB, err = factory.CreateVectorDatabase()
	if err != nil {
		return fmt.Errorf("failed to create vector database: %w", err)
	}

	return nil
}

// setupWorkflowComponents initializes analytics engine and workflow builder
func (s *StandardTestSuite) setupWorkflowComponents() error {
	// Create pattern extractor and store
	patternStore := NewStandardPatternStore(s.Logger)

	// Setup analytics engine with adapter
	if s.VectorDB != nil {
		insightsEngine := insights.NewAnalyticsEngine()
		s.AnalyticsEngine = &AnalyticsEngineAdapter{engine: insightsEngine}
	}

	// Setup metrics collector
	s.MetricsCollector = &MockAIMetricsCollector{}

	// Setup execution repository
	s.ExecutionRepo = engine.NewInMemoryExecutionRepository(s.Logger)

	// Setup workflow builder using interface types
	patternStoreAdapter := &PatternStoreAdapter{store: patternStore}
	s.WorkflowBuilder = engine.NewIntelligentWorkflowBuilder(
		s.LLMClient,
		s.VectorDB,
		s.AnalyticsEngine,
		s.MetricsCollector,
		patternStoreAdapter,
		s.ExecutionRepo,
		s.Logger,
	)

	return nil
}

// Configuration option functions
func WithRealDatabase() TestOption {
	return func(c *TestSuiteConfig) { c.UseRealDatabase = true }
}

func WithMockDatabase() TestOption {
	return func(c *TestSuiteConfig) { c.UseRealDatabase = false }
}

func WithRealLLM() TestOption {
	return func(c *TestSuiteConfig) { c.UseRealLLM = true }
}

func WithMockLLM() TestOption {
	return func(c *TestSuiteConfig) { c.UseRealLLM = false }
}

func WithRealVectorDB() TestOption {
	return func(c *TestSuiteConfig) { c.UseRealVectorDB = true }
}

func WithMockVectorDB() TestOption {
	return func(c *TestSuiteConfig) { c.UseRealVectorDB = false }
}

func WithDatabaseIsolation(strategy IsolationStrategy) TestOption {
	return func(c *TestSuiteConfig) { c.DatabaseIsolation = strategy }
}

func WithPerformanceMonitoring() TestOption {
	return func(c *TestSuiteConfig) { c.EnablePerformance = true }
}

func WithDebugLogging() TestOption {
	return func(c *TestSuiteConfig) { c.EnableDebugLogging = true }
}

func WithCustomCleanup(cleanupFunc func() error) TestOption {
	return func(c *TestSuiteConfig) { c.CustomCleanup = cleanupFunc }
}

func WithoutCleanup() TestOption {
	return func(c *TestSuiteConfig) { c.SkipCleanup = true }
}
