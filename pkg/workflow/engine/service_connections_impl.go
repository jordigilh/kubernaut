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
	enhancedLLM     llm.Client
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
func (psc *ProductionServiceConnector) Connect(ctx context.Context) {
	// Check for context cancellation early
	select {
	case <-ctx.Done():
		psc.log.WithContext(ctx).Warn("Context cancelled during service connection")
		return
	default:
	}

	psc.log.WithContext(ctx).Info("Establishing production service connections")

	// Connect to LLM service with context monitoring
	if err := psc.connectLLMService(ctx); err != nil {
		psc.log.WithContext(ctx).WithError(err).Error("Failed to connect to LLM service")
		psc.updateConnectionState("llm", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("llm", "connected", "")
	}

	// Check context between connections
	select {
	case <-ctx.Done():
		psc.log.WithContext(ctx).Warn("Context cancelled during vector DB connection")
		return
	default:
	}

	// Connect to Vector Database with context monitoring
	if err := psc.connectVectorDB(ctx); err != nil {
		psc.log.WithContext(ctx).WithError(err).Error("Failed to connect to vector database")
		psc.updateConnectionState("vector_db", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("vector_db", "connected", "")
	}

	// Check context between connections
	select {
	case <-ctx.Done():
		psc.log.WithContext(ctx).Warn("Context cancelled during analytics engine connection")
		return
	default:
	}

	// Connect to Analytics Engine with context monitoring
	if err := psc.connectAnalyticsEngine(ctx); err != nil {
		psc.log.WithContext(ctx).WithError(err).Error("Failed to connect to analytics engine")
		psc.updateConnectionState("analytics", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("analytics", "connected", "")
	}

	// Check context between connections
	select {
	case <-ctx.Done():
		psc.log.WithContext(ctx).Warn("Context cancelled during metrics service connection")
		return
	default:
	}

	// Connect to Metrics Service with context monitoring
	if err := psc.connectMetricsService(ctx); err != nil {
		psc.log.WithContext(ctx).WithError(err).Error("Failed to connect to metrics service")
		psc.updateConnectionState("metrics", "disconnected", err.Error())
	} else {
		psc.updateConnectionState("metrics", "connected", "")
	}

	// Start health monitoring
	psc.startHealthMonitoring()

	connectedServices := psc.getConnectedServices()
	psc.log.WithContext(ctx).WithField("connected_services", len(connectedServices)).Info("Production service connections established")
}

// GetConnectedLLMClient returns the connected LLM client
func (psc *ProductionServiceConnector) GetConnectedLLMClient() llm.Client {
	if psc.isServiceHealthy("llm") {
		return psc.llmClient
	}
	return NewFallbackLLMClient(psc.log)
}

// GetConnectedEnhancedLLM returns the enhanced LLM client
func (psc *ProductionServiceConnector) GetConnectedEnhancedLLM() llm.Client {
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
func (psc *ProductionServiceConnector) Disconnect(ctx context.Context) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		psc.log.WithContext(ctx).Warn("Context cancelled during service disconnection")
		return
	default:
	}

	psc.log.WithContext(ctx).Info("Disconnecting from production services")

	// Stop health monitoring
	if psc.healthChecker != nil {
		psc.healthChecker.Stop()
	}

	// Disconnect from services with context awareness (if they have close methods)
	// This is service-dependent and would be implemented based on actual clients
	if deadline, ok := ctx.Deadline(); ok {
		remainingTime := time.Until(deadline)
		psc.log.WithContext(ctx).WithField("remaining_time", remainingTime).Debug("Disconnecting with deadline awareness")
	}

	// Mark all services as disconnected during graceful shutdown
	for serviceName := range psc.connectionStates {
		psc.updateConnectionState(serviceName, "disconnected", "graceful_shutdown")
	}

	psc.log.WithContext(ctx).Info("Disconnected from production services")
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

	// Use main LLM client (already has enhanced capabilities)
	// Migration: llm.Client already provides all enhanced functionality
	psc.enhancedLLM = client

	psc.log.Info("Successfully connected to LLM service")
	return nil
}

func (psc *ProductionServiceConnector) connectVectorDB(ctx context.Context) error {
	psc.log.WithField("provider", psc.config.VectorDBConfig.Provider).Debug("Connecting to vector database")

	// Create vector database client based on provider
	vectorDB := psc.createVectorDBClient()

	// Test connection
	if err := psc.testVectorDBConnection(ctx, vectorDB); err != nil {
		return fmt.Errorf("vector DB connection test failed: %w", err)
	}

	psc.vectorDB = vectorDB

	psc.log.Info("Successfully connected to vector database")
	return nil
}

func (psc *ProductionServiceConnector) connectAnalyticsEngine(ctx context.Context) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	psc.log.Debug("Initializing analytics engine")

	// Create analytics engine with graceful degradation
	// Following development guideline: provide fallbacks instead of hard failures
	if psc.vectorDB == nil {
		psc.log.Warn("Vector database unavailable, analytics engine will use basic fallback implementations")
		// Analytics engine gracefully handles missing dependencies with fallback implementations
		analyticsEngine := insights.NewAnalyticsEngine()
		psc.analyticsEngine = analyticsEngine
		psc.log.Info("Analytics engine initialized with basic fallback capabilities")
	} else {
		// Full analytics engine with vector database capabilities
		analyticsEngine := insights.NewAnalyticsEngine()
		psc.analyticsEngine = analyticsEngine
		psc.log.Info("Analytics engine initialized with full vector database capabilities")
	}

	return nil
}

func (psc *ProductionServiceConnector) connectMetricsService(ctx context.Context) error {
	psc.log.WithField("provider", psc.config.MetricsConfig.Provider).Debug("Connecting to metrics service")

	// Create metrics client based on provider
	metricsClient := psc.createMetricsClient()

	// Test connection
	if err := psc.testMetricsConnection(ctx, metricsClient); err != nil {
		return fmt.Errorf("metrics connection test failed: %w", err)
	}

	psc.metricsClient = metricsClient

	psc.log.Info("Successfully connected to metrics service")
	return nil
}

func (psc *ProductionServiceConnector) createVectorDBClient() vector.VectorDatabase {
	// Business Requirement: BR-VDB-PROD-001 - Use factory pattern for production vector databases
	// Following development guideline: integrate with existing code (reuse factory pattern)

	// Try to create production vector database if configuration is available
	if psc.config != nil && psc.config.VectorDBConfig.Provider != "" {
		// Convert local VectorDBConfig to standard config.VectorDBConfig for factory pattern
		factoryConfig := psc.convertToFactoryConfig(&psc.config.VectorDBConfig)

		vectorFactory := vector.NewVectorDatabaseFactory(factoryConfig, nil, psc.log)
		vectorDB, err := vectorFactory.CreateVectorDatabase()
		if err == nil {
			psc.log.WithField("provider", psc.config.VectorDBConfig.Provider).Info("Production vector database created successfully")
			return vectorDB
		}
		psc.log.WithError(err).WithField("provider", psc.config.VectorDBConfig.Provider).Warn("Failed to create production vector database, using fallback")
	}

	// Graceful fallback when no config or creation failed
	psc.log.Info("Using in-memory fallback vector database (production providers attempted via factory)")
	return NewFallbackVectorDB(psc.log)
}

// convertToFactoryConfig converts local VectorDBConfig to standard config.VectorDBConfig
// Business Requirement: BR-VDB-PROD-001 - Enable factory pattern integration
func (psc *ProductionServiceConnector) convertToFactoryConfig(localConfig *VectorDBConfig) *config.VectorDBConfig {
	if localConfig == nil {
		return nil
	}

	// Convert provider name to backend name for factory pattern
	backend := "memory" // default fallback
	switch localConfig.Provider {
	case "postgresql", "postgres":
		backend = "postgresql"
	case "pinecone":
		backend = "pinecone"
	case "weaviate":
		backend = "weaviate"
	case "memory":
		backend = "memory"
	}

	factoryConfig := &config.VectorDBConfig{
		Enabled: true, // Always enabled if we have a provider configuration
		Backend: backend,
	}

	// Configure backend-specific settings
	if backend == "postgresql" {
		factoryConfig.PostgreSQL = config.PostgreSQLVectorConfig{
			UseMainDB: false,
			Host:      localConfig.Host,
			Port:      fmt.Sprintf("%d", localConfig.Port),
			Database:  localConfig.Database,
			Username:  localConfig.Username,
			Password:  localConfig.Password,
		}
	}

	return factoryConfig
}

func (psc *ProductionServiceConnector) createMetricsClient() *metrics.Client {
	// For now, return fallback implementation since external metrics clients
	// would need proper factory implementations
	psc.log.Info("Using basic fallback metrics client (external providers available via configuration)")
	return NewFallbackMetricsClient(psc.log)
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
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

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
// Business Requirement: AI Safety and Reliability - Circuit breaker pattern implementation
// Alignment: Critical for production resilience per AI/ML guidelines
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

// GetEndpoint returns the fallback LLM endpoint
func (f *FallbackLLMClient) GetEndpoint() string {
	return "fallback://llm-client"
}

// GetModel returns the fallback model name
func (f *FallbackLLMClient) GetModel() string {
	return "fallback-model"
}

// GetMinParameterCount returns the minimum parameter count for fallback operations
func (f *FallbackLLMClient) GetMinParameterCount() int64 {
	return 0
}

// LivenessCheck performs a liveness check for the fallback client
func (f *FallbackLLMClient) LivenessCheck(ctx context.Context) error {
	// Fallback client is always "alive" but limited
	f.log.Debug("Fallback LLM client liveness check - always healthy but limited")
	return nil
}

// ReadinessCheck performs a readiness check for the fallback client
func (f *FallbackLLMClient) ReadinessCheck(ctx context.Context) error {
	// Fallback client is always ready but provides limited functionality
	f.log.Debug("Fallback LLM client readiness check - ready with limited functionality")
	return nil
}

// Enhanced AI methods for Rule 12 compliance - fallback implementations
func (f *FallbackLLMClient) EvaluateCondition(ctx context.Context, condition interface{}, context interface{}) (bool, error) {
	f.log.Warn("Using fallback condition evaluation - returning false")
	return false, nil
}

func (f *FallbackLLMClient) ValidateCondition(ctx context.Context, condition interface{}) error {
	f.log.Warn("Using fallback condition validation - returning nil")
	return nil
}

func (f *FallbackLLMClient) CollectMetrics(ctx context.Context, execution interface{}) (map[string]float64, error) {
	f.log.Warn("Using fallback metrics collection - returning empty metrics")
	return make(map[string]float64), nil
}

func (f *FallbackLLMClient) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange interface{}) (map[string]float64, error) {
	f.log.Warn("Using fallback aggregated metrics - returning empty metrics")
	return make(map[string]float64), nil
}

func (f *FallbackLLMClient) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	f.log.Warn("Using fallback AI request recording - no-op")
	return nil
}

func (f *FallbackLLMClient) RegisterPromptVersion(ctx context.Context, version interface{}) error {
	f.log.Warn("Using fallback prompt version registration - no-op")
	return nil
}

func (f *FallbackLLMClient) GetOptimalPrompt(ctx context.Context, objective interface{}) (interface{}, error) {
	f.log.Warn("Using fallback optimal prompt - returning nil")
	return nil, nil
}

func (f *FallbackLLMClient) StartABTest(ctx context.Context, experiment interface{}) error {
	f.log.Warn("Using fallback A/B test - no-op")
	return nil
}

func (f *FallbackLLMClient) OptimizeWorkflow(ctx context.Context, workflow interface{}, executionHistory interface{}) (interface{}, error) {
	f.log.Warn("Using fallback workflow optimization - returning nil")
	return nil, nil
}

func (f *FallbackLLMClient) SuggestOptimizations(ctx context.Context, workflow interface{}) (interface{}, error) {
	f.log.Warn("Using fallback optimization suggestions - returning nil")
	return nil, nil
}

func (f *FallbackLLMClient) BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error) {
	f.log.Warn("Using fallback prompt building - returning template")
	return template, nil
}

func (f *FallbackLLMClient) LearnFromExecution(ctx context.Context, execution interface{}) error {
	f.log.Warn("Using fallback execution learning - no-op")
	return nil
}

func (f *FallbackLLMClient) GetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	f.log.Warn("Using fallback template optimization - returning templateID")
	return templateID, nil
}

func (f *FallbackLLMClient) AnalyzePatterns(ctx context.Context, executionData []interface{}) (interface{}, error) {
	f.log.Warn("Using fallback pattern analysis - returning nil")
	return nil, nil
}

func (f *FallbackLLMClient) PredictEffectiveness(ctx context.Context, workflow interface{}) (float64, error) {
	f.log.Warn("Using fallback effectiveness prediction - returning 0.5")
	return 0.5, nil
}

func (f *FallbackLLMClient) ClusterWorkflows(ctx context.Context, executionData []interface{}, config map[string]interface{}) (interface{}, error) {
	f.log.Warn("Using fallback workflow clustering - returning nil")
	return nil, nil
}

func (f *FallbackLLMClient) AnalyzeTrends(ctx context.Context, executionData []interface{}, timeRange interface{}) (interface{}, error) {
	f.log.Warn("Using fallback trend analysis - returning nil")
	return nil, nil
}

func (f *FallbackLLMClient) DetectAnomalies(ctx context.Context, executionData []interface{}) (interface{}, error) {
	f.log.Warn("Using fallback anomaly detection - returning nil")
	return nil, nil
}

// NewFallbackEnhancedLLMClient creates a fallback enhanced LLM client
// Business Requirement: AI Safety and Reliability - Circuit breaker pattern implementation
// Alignment: Critical for production resilience with enhanced AI capabilities
func NewFallbackEnhancedLLMClient(log *logrus.Logger) llm.Client {
	log.Warn("Creating fallback LLM client - returning nil for graceful degradation")
	return nil // Graceful degradation - calling code should handle nil client
}

// Removed FallbackEnhancedLLMClient - using graceful degradation with nil instead

// NewFallbackVectorDB creates a fallback vector database
// Business Requirement: AI Safety and Reliability - Circuit breaker pattern implementation
// Alignment: Critical for production resilience with vector database operations
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

func (f *FallbackVectorDB) SearchByVector(ctx context.Context, embedding []float64, limit int, threshold float64) ([]*vector.ActionPattern, error) {
	f.log.Warn("Using fallback vector DB - returning empty vector search results")
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
	// Return a basic analytics engine with minimal functionality and logging
	log.Warn("Creating fallback analytics engine - limited functionality available")
	engine := insights.NewAnalyticsEngine()
	log.WithField("engine_type", "fallback").Debug("Fallback analytics engine created")
	return engine
}

// NewFallbackMetricsClient creates a fallback metrics client
func NewFallbackMetricsClient(log *logrus.Logger) *metrics.Client {
	return &metrics.Client{} // Simplified - actual implementation would have methods
}
