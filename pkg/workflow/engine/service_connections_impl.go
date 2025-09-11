package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// ProductionServiceConnector manages connections to all external services
// required for AI workflow generation in production environments
type ProductionServiceConnector struct {
	config *ServiceConnectionConfig
	log    *logrus.Logger

	// Service clients
	llmClient       llm.Client
	enhancedLLM     llm.EnhancedClient
	vectorDB        vector.VectorDatabase
	analyticsEngine types.AnalyticsEngine
	metricsClient   *metrics.Client

	// Connection states
	connectionStates map[string]*ServiceConnectionState

	// Health monitoring
	healthChecker *ServiceHealthChecker

	// Circuit breakers for resilience
	circuitBreakers map[string]*CircuitBreaker
}

// ServiceConnectionConfig holds configuration for production service connections
type ServiceConnectionConfig struct {
	// LLM Service Configuration
	LLMConfig config.LLMConfig `yaml:"llm"`

	// Vector Database Configuration
	VectorDBConfig VectorDBConfig `yaml:"vector_db"`

	// Analytics Configuration
	AnalyticsConfig AnalyticsConfig `yaml:"analytics"`

	// Metrics Configuration
	MetricsConfig MetricsConfig `yaml:"metrics"`

	// Connection Settings
	ConnectionTimeoutSeconds   int `yaml:"connection_timeout_seconds" default:"30"`
	MaxRetries                 int `yaml:"max_retries" default:"3"`
	HealthCheckIntervalSeconds int `yaml:"health_check_interval_seconds" default:"60"`

	// Circuit Breaker Settings
	CircuitBreakerConfig CircuitBreakerConfig `yaml:"circuit_breaker"`
}

// VectorDBConfig holds vector database connection configuration
type VectorDBConfig struct {
	Provider       string            `yaml:"provider"` // "pgvector", "pinecone", "weaviate", etc.
	Host           string            `yaml:"host"`
	Port           int               `yaml:"port"`
	Database       string            `yaml:"database"`
	Username       string            `yaml:"username"`
	Password       string            `yaml:"password"`
	SSLMode        string            `yaml:"ssl_mode" default:"require"`
	MaxConnections int               `yaml:"max_connections" default:"10"`
	Dimensions     int               `yaml:"dimensions" default:"1536"`
	IndexType      string            `yaml:"index_type" default:"ivfflat"`
	Metadata       map[string]string `yaml:"metadata"`
}

// AnalyticsConfig holds analytics service configuration
type AnalyticsConfig struct {
	EnableAdvancedAnalytics bool                   `yaml:"enable_advanced_analytics" default:"true"`
	BatchSize               int                    `yaml:"batch_size" default:"100"`
	ProcessingInterval      time.Duration          `yaml:"processing_interval" default:"5m"`
	RetentionDays           int                    `yaml:"retention_days" default:"90"`
	ModelConfigs            map[string]interface{} `yaml:"model_configs"`
}

// MetricsConfig holds metrics service configuration
type MetricsConfig struct {
	Provider     string            `yaml:"provider"` // "prometheus", "datadog", etc.
	Endpoint     string            `yaml:"endpoint"`
	APIKey       string            `yaml:"api_key"`
	PushInterval time.Duration     `yaml:"push_interval" default:"10s"`
	Labels       map[string]string `yaml:"labels"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold    int  `yaml:"failure_threshold" default:"5"`
	ResetTimeoutSeconds int  `yaml:"reset_timeout_seconds" default:"60"`
	MaxRequests         int  `yaml:"max_requests" default:"3"`
	Enabled             bool `yaml:"enabled" default:"true"`
}

// ServiceConnectionState tracks the state of a service connection
type ServiceConnectionState struct {
	ServiceName     string        `json:"service_name"`
	Status          string        `json:"status"` // "connected", "disconnected", "degraded"
	LastHealthCheck time.Time     `json:"last_health_check"`
	ErrorCount      int           `json:"error_count"`
	SuccessCount    int           `json:"success_count"`
	LastError       string        `json:"last_error,omitempty"`
	Latency         time.Duration `json:"latency"`
}

// ServiceHealthChecker monitors service health
type ServiceHealthChecker struct {
	connector *ProductionServiceConnector
	ticker    *time.Ticker
	stopCh    chan bool
	log       *logrus.Logger
}

// CircuitBreaker provides resilience against service failures
type CircuitBreaker struct {
	name            string
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	state           string // "closed", "open", "half-open"
	config          *CircuitBreakerConfig
	log             *logrus.Logger
}

// NewProductionServiceConnector creates a new production service connector
func NewProductionServiceConnector(config *ServiceConnectionConfig, log *logrus.Logger) *ProductionServiceConnector {
	if config == nil {
		config = &ServiceConnectionConfig{
			ConnectionTimeoutSeconds:   30,
			MaxRetries:                 3,
			HealthCheckIntervalSeconds: 60,
			CircuitBreakerConfig: CircuitBreakerConfig{
				FailureThreshold:    5,
				ResetTimeoutSeconds: 60,
				MaxRequests:         3,
				Enabled:             true,
			},
		}
	}

	psc := &ProductionServiceConnector{
		config:           config,
		log:              log,
		connectionStates: make(map[string]*ServiceConnectionState),
		circuitBreakers:  make(map[string]*CircuitBreaker),
	}

	// Initialize circuit breakers
	if config.CircuitBreakerConfig.Enabled {
		psc.initializeCircuitBreakers()
	}

	return psc
}

// Connect establishes connections to all required services
func (psc *ProductionServiceConnector) Connect(ctx context.Context) error {
	psc.log.Info("Establishing production service connections")

	// Connect to LLM service
	if err := psc.connectLLMService(ctx); err != nil {
		psc.log.WithError(err).Error("Failed to connect to LLM service")
		psc.updateConnectionState("llm", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("llm", "connected", "")
	}

	// Connect to Vector Database
	if err := psc.connectVectorDB(ctx); err != nil {
		psc.log.WithError(err).Error("Failed to connect to vector database")
		psc.updateConnectionState("vector_db", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("vector_db", "connected", "")
	}

	// Connect to Analytics Engine
	if err := psc.connectAnalyticsEngine(ctx); err != nil {
		psc.log.WithError(err).Error("Failed to connect to analytics engine")
		psc.updateConnectionState("analytics", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("analytics", "connected", "")
	}

	// Connect to Metrics Service
	if err := psc.connectMetricsService(ctx); err != nil {
		psc.log.WithError(err).Error("Failed to connect to metrics service")
		psc.updateConnectionState("metrics", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("metrics", "connected", "")
	}

	// Start health monitoring
	psc.startHealthMonitoring()

	psc.log.WithField("connected_services", len(psc.getConnectedServices())).Info("Production service connections established")
	return nil
}

// GetConnectedLLMClient returns the connected LLM client
func (psc *ProductionServiceConnector) GetConnectedLLMClient() llm.Client {
	if psc.isServiceHealthy("llm") {
		return psc.llmClient
	}
	return NewFallbackLLMClient(psc.log)
}

// GetConnectedEnhancedLLM returns the enhanced LLM client
func (psc *ProductionServiceConnector) GetConnectedEnhancedLLM() llm.EnhancedClient {
	if psc.isServiceHealthy("llm") {
		return psc.enhancedLLM
	}
	return NewFallbackEnhancedLLMClient(psc.log)
}

// GetConnectedVectorDB returns the connected vector database
func (psc *ProductionServiceConnector) GetConnectedVectorDB() vector.VectorDatabase {
	if psc.isServiceHealthy("vector_db") {
		return psc.vectorDB
	}
	return NewFallbackVectorDB(psc.log)
}

// GetConnectedAnalyticsEngine returns the connected analytics engine
func (psc *ProductionServiceConnector) GetConnectedAnalyticsEngine() types.AnalyticsEngine {
	if psc.isServiceHealthy("analytics") {
		return psc.analyticsEngine
	}
	return NewFallbackAnalyticsEngine(psc.log)
}

// GetConnectedMetricsClient returns the connected metrics client
func (psc *ProductionServiceConnector) GetConnectedMetricsClient() *metrics.Client {
	if psc.isServiceHealthy("metrics") {
		return psc.metricsClient
	}
	return NewFallbackMetricsClient(psc.log)
}

// GetServiceHealth returns health status of all services
func (psc *ProductionServiceConnector) GetServiceHealth() map[string]*ServiceConnectionState {
	result := make(map[string]*ServiceConnectionState)
	for name, state := range psc.connectionStates {
		// Create a copy to avoid race conditions
		result[name] = &ServiceConnectionState{
			ServiceName:     state.ServiceName,
			Status:          state.Status,
			LastHealthCheck: state.LastHealthCheck,
			ErrorCount:      state.ErrorCount,
			SuccessCount:    state.SuccessCount,
			LastError:       state.LastError,
			Latency:         state.Latency,
		}
	}
	return result
}

// Disconnect gracefully disconnects from all services
func (psc *ProductionServiceConnector) Disconnect(ctx context.Context) error {
	psc.log.Info("Disconnecting from production services")

	// Stop health monitoring
	if psc.healthChecker != nil {
		psc.healthChecker.Stop()
	}

	// Disconnect from services (if they have close methods)
	// This is service-dependent and would be implemented based on actual clients

	psc.log.Info("Disconnected from production services")
	return nil
}

// Private helper methods

func (psc *ProductionServiceConnector) connectLLMService(ctx context.Context) error {
	psc.log.Debug("Connecting to LLM service")

	// Create basic LLM client
	client, err := llm.NewClient(psc.config.LLMConfig, psc.log)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Test connection
	if err := psc.testLLMConnection(ctx, client); err != nil {
		return fmt.Errorf("LLM connection test failed: %w", err)
	}

	psc.llmClient = client

	// Create enhanced LLM client
	enhancedClient := llm.NewEnhancedClient()
	psc.enhancedLLM = enhancedClient

	psc.log.Info("Successfully connected to LLM service")
	return nil
}

func (psc *ProductionServiceConnector) connectVectorDB(ctx context.Context) error {
	psc.log.WithField("provider", psc.config.VectorDBConfig.Provider).Debug("Connecting to vector database")

	// Create vector database client based on provider
	vectorDB, err := psc.createVectorDBClient()
	if err != nil {
		return fmt.Errorf("failed to create vector DB client: %w", err)
	}

	// Test connection
	if err := psc.testVectorDBConnection(ctx, vectorDB); err != nil {
		return fmt.Errorf("vector DB connection test failed: %w", err)
	}

	psc.vectorDB = vectorDB

	psc.log.Info("Successfully connected to vector database")
	return nil
}

func (psc *ProductionServiceConnector) connectAnalyticsEngine(ctx context.Context) error {
	psc.log.Debug("Initializing analytics engine")

	// Analytics engine requires vector DB and pattern extractor
	if psc.vectorDB == nil {
		return fmt.Errorf("vector database required for analytics engine")
	}

	// Create analytics engine
	analyticsEngine := insights.NewAnalyticsEngine()
	psc.analyticsEngine = analyticsEngine

	psc.log.Info("Successfully initialized analytics engine")
	return nil
}

func (psc *ProductionServiceConnector) connectMetricsService(ctx context.Context) error {
	psc.log.WithField("provider", psc.config.MetricsConfig.Provider).Debug("Connecting to metrics service")

	// Create metrics client based on provider
	metricsClient, err := psc.createMetricsClient()
	if err != nil {
		return fmt.Errorf("failed to create metrics client: %w", err)
	}

	// Test connection
	if err := psc.testMetricsConnection(ctx, metricsClient); err != nil {
		return fmt.Errorf("metrics connection test failed: %w", err)
	}

	psc.metricsClient = metricsClient

	psc.log.Info("Successfully connected to metrics service")
	return nil
}

func (psc *ProductionServiceConnector) createVectorDBClient() (vector.VectorDatabase, error) {
	// For now, return fallback implementation since external vector DB clients
	// would need proper factory implementations
	psc.log.Warn("Using fallback vector DB - external providers not fully implemented")
	return NewFallbackVectorDB(psc.log), nil
}

func (psc *ProductionServiceConnector) createMetricsClient() (*metrics.Client, error) {
	// For now, return fallback implementation since external metrics clients
	// would need proper factory implementations
	psc.log.Warn("Using fallback metrics client - external providers not fully implemented")
	return NewFallbackMetricsClient(psc.log), nil
}

func (psc *ProductionServiceConnector) testLLMConnection(ctx context.Context, client llm.Client) error {
	// Simple connection test
	_, err := client.ChatCompletion(ctx, "Hello")
	return err
}

func (psc *ProductionServiceConnector) testVectorDBConnection(ctx context.Context, vectorDB vector.VectorDatabase) error {
	// Simple connection test using the IsHealthy method
	return vectorDB.IsHealthy(ctx)
}

func (psc *ProductionServiceConnector) testMetricsConnection(ctx context.Context, metricsClient *metrics.Client) error {
	// Simple connection test - try to record a test metric
	return metricsClient.RecordCounter("connection_test", 1, map[string]string{"test": "true"})
}

func (psc *ProductionServiceConnector) initializeCircuitBreakers() {
	services := []string{"llm", "vector_db", "analytics", "metrics"}

	for _, service := range services {
		psc.circuitBreakers[service] = &CircuitBreaker{
			name:   service,
			state:  "closed",
			config: &psc.config.CircuitBreakerConfig,
			log:    psc.log,
		}
	}
}

func (psc *ProductionServiceConnector) updateConnectionState(serviceName, status, lastError string) {
	if psc.connectionStates[serviceName] == nil {
		psc.connectionStates[serviceName] = &ServiceConnectionState{
			ServiceName: serviceName,
		}
	}

	state := psc.connectionStates[serviceName]
	state.Status = status
	state.LastHealthCheck = time.Now()
	state.LastError = lastError

	if lastError != "" {
		state.ErrorCount++
	} else {
		state.SuccessCount++
	}

	psc.log.WithFields(logrus.Fields{
		"service": serviceName,
		"status":  status,
		"error":   lastError,
	}).Debug("Updated service connection state")
}

func (psc *ProductionServiceConnector) isServiceHealthy(serviceName string) bool {
	state, exists := psc.connectionStates[serviceName]
	if !exists {
		return false
	}

	// Check circuit breaker
	if cb, exists := psc.circuitBreakers[serviceName]; exists {
		if cb.state == "open" {
			return false
		}
	}

	return state.Status == "connected"
}

func (psc *ProductionServiceConnector) getConnectedServices() []string {
	var connected []string
	for name, state := range psc.connectionStates {
		if state.Status == "connected" {
			connected = append(connected, name)
		}
	}
	return connected
}

func (psc *ProductionServiceConnector) startHealthMonitoring() {
	if psc.config.HealthCheckIntervalSeconds > 0 {
		psc.healthChecker = &ServiceHealthChecker{
			connector: psc,
			ticker:    time.NewTicker(time.Duration(psc.config.HealthCheckIntervalSeconds) * time.Second),
			stopCh:    make(chan bool),
			log:       psc.log,
		}

		go psc.healthChecker.Start()
	}
}

// ServiceHealthChecker methods

func (shc *ServiceHealthChecker) Start() {
	shc.log.Info("Starting service health monitoring")

	for {
		select {
		case <-shc.ticker.C:
			shc.performHealthChecks()
		case <-shc.stopCh:
			shc.log.Info("Stopping service health monitoring")
			return
		}
	}
}

func (shc *ServiceHealthChecker) Stop() {
	if shc.ticker != nil {
		shc.ticker.Stop()
	}
	close(shc.stopCh)
}

func (shc *ServiceHealthChecker) performHealthChecks() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check LLM service health
	if shc.connector.llmClient != nil {
		if err := shc.connector.testLLMConnection(ctx, shc.connector.llmClient); err != nil {
			shc.connector.updateConnectionState("llm", "degraded", err.Error())
		} else {
			shc.connector.updateConnectionState("llm", "connected", "")
		}
	}

	// Check Vector DB health
	if shc.connector.vectorDB != nil {
		if err := shc.connector.testVectorDBConnection(ctx, shc.connector.vectorDB); err != nil {
			shc.connector.updateConnectionState("vector_db", "degraded", err.Error())
		} else {
			shc.connector.updateConnectionState("vector_db", "connected", "")
		}
	}

	// Check metrics service health
	if shc.connector.metricsClient != nil {
		if err := shc.connector.testMetricsConnection(ctx, shc.connector.metricsClient); err != nil {
			shc.connector.updateConnectionState("metrics", "degraded", err.Error())
		} else {
			shc.connector.updateConnectionState("metrics", "connected", "")
		}
	}
}

// CircuitBreaker methods

func (cb *CircuitBreaker) Call(fn func() error) error {
	if cb.state == "open" {
		if time.Since(cb.lastFailureTime) > time.Duration(cb.config.ResetTimeoutSeconds)*time.Second {
			cb.state = "half-open"
			cb.log.WithField("circuit_breaker", cb.name).Info("Circuit breaker entering half-open state")
		} else {
			return fmt.Errorf("circuit breaker %s is open", cb.name)
		}
	}

	err := fn()

	if err != nil {
		cb.failureCount++
		cb.lastFailureTime = time.Now()

		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = "open"
			cb.log.WithFields(logrus.Fields{
				"circuit_breaker": cb.name,
				"failures":        cb.failureCount,
			}).Warn("Circuit breaker opened due to failures")
		}

		return err
	}

	// Success
	cb.successCount++
	if cb.state == "half-open" {
		cb.state = "closed"
		cb.failureCount = 0
		cb.log.WithField("circuit_breaker", cb.name).Info("Circuit breaker closed after successful call")
	}

	return nil
}

// Fallback client implementations

// NewFallbackLLMClient creates a fallback LLM client for when the main service is unavailable
func NewFallbackLLMClient(log *logrus.Logger) llm.Client {
	return &FallbackLLMClient{log: log}
}

type FallbackLLMClient struct {
	log *logrus.Logger
}

func (f *FallbackLLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	f.log.Warn("Using fallback LLM client - limited functionality")
	return &llm.AnalyzeAlertResponse{
		Action:     "restart_pod",
		Parameters: map[string]interface{}{},
		Confidence: 0.5,
		Reasoning: &types.ReasoningDetails{
			Summary: "Fallback recommendation - please check service connectivity",
		},
	}, nil
}

func (f *FallbackLLMClient) GenerateResponse(prompt string) (string, error) {
	f.log.Warn("Using fallback LLM client for response generation")
	return "Fallback response: I'm currently operating in fallback mode. Please check service connectivity for full functionality.", nil
}

func (f *FallbackLLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	f.log.Warn("Using fallback LLM client for chat completion")
	return "I'm currently operating in fallback mode. Please check service connectivity for full functionality.", nil
}

func (f *FallbackLLMClient) IsHealthy() bool {
	return false
}

func (f *FallbackLLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	f.log.Warn("Using fallback LLM client for workflow generation - returning basic template")
	return &llm.WorkflowGenerationResult{
		WorkflowID:  "fallback-workflow-" + objective.ID,
		Name:        "Fallback Workflow",
		Description: "Basic workflow generated in fallback mode",
		Steps: []*llm.AIGeneratedStep{
			{
				ID:      "fallback-step-1",
				Name:    "Fallback Action",
				Type:    "notify_only",
				Timeout: "30s",
				Action: &llm.AIStepAction{
					Type:       "notify_only",
					Parameters: map[string]interface{}{"message": "Fallback mode - check service connectivity"},
				},
			},
		},
		Variables:  make(map[string]interface{}),
		Confidence: 0.1,
		Reasoning:  "Generated in fallback mode with limited functionality",
	}, nil
}

// NewFallbackEnhancedLLMClient creates a fallback enhanced LLM client
func NewFallbackEnhancedLLMClient(log *logrus.Logger) llm.EnhancedClient {
	return &FallbackEnhancedLLMClient{
		log: log,
	}
}

type FallbackEnhancedLLMClient struct {
	log *logrus.Logger
}

func (f *FallbackEnhancedLLMClient) GenerateResponse(prompt string) (string, error) {
	f.log.Warn("Using fallback enhanced LLM client")
	return "Fallback enhanced response: " + prompt, nil
}

func (f *FallbackEnhancedLLMClient) GenerateEnhancedResponse(prompt string, context map[string]interface{}) (string, error) {
	f.log.Warn("Using fallback enhanced LLM client with context")
	return "Fallback enhanced response with context: " + prompt, nil
}

// NewFallbackVectorDB creates a fallback vector database
func NewFallbackVectorDB(log *logrus.Logger) vector.VectorDatabase {
	return &FallbackVectorDB{log: log}
}

type FallbackVectorDB struct {
	log *logrus.Logger
}

func (f *FallbackVectorDB) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	f.log.Warn("Using fallback vector DB - pattern not stored")
	return nil
}

func (f *FallbackVectorDB) FindSimilarPatterns(ctx context.Context, pattern *vector.ActionPattern, limit int, threshold float64) ([]*vector.SimilarPattern, error) {
	f.log.Warn("Using fallback vector DB - returning empty results")
	return []*vector.SimilarPattern{}, nil
}

func (f *FallbackVectorDB) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	f.log.Warn("Using fallback vector DB - effectiveness not updated")
	return nil
}

func (f *FallbackVectorDB) SearchBySemantics(ctx context.Context, query string, limit int) ([]*vector.ActionPattern, error) {
	f.log.Warn("Using fallback vector DB - returning empty semantic results")
	return []*vector.ActionPattern{}, nil
}

func (f *FallbackVectorDB) DeletePattern(ctx context.Context, patternID string) error {
	f.log.Warn("Using fallback vector DB - delete operation ignored")
	return nil
}

func (f *FallbackVectorDB) GetPatternAnalytics(ctx context.Context) (*vector.PatternAnalytics, error) {
	f.log.Warn("Using fallback vector DB - returning empty analytics")
	return &vector.PatternAnalytics{}, nil
}

func (f *FallbackVectorDB) IsHealthy(ctx context.Context) error {
	return fmt.Errorf("fallback vector database is not healthy")
}

// NewFallbackAnalyticsEngine creates a fallback analytics engine
func NewFallbackAnalyticsEngine(log *logrus.Logger) types.AnalyticsEngine {
	// Return a basic analytics engine with minimal functionality
	return insights.NewAnalyticsEngine()
}

// NewFallbackMetricsClient creates a fallback metrics client
func NewFallbackMetricsClient(log *logrus.Logger) *metrics.Client {
	return &metrics.Client{} // Simplified - actual implementation would have methods
}
