//go:build integration
// +build integration

package shared

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/mcp"
	"github.com/jordigilh/prometheus-alerts-slm/internal/oscillation"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared/testenv"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// IntegrationTestUtils provides utilities for integration testing with real components
type IntegrationTestUtils struct {
	ConnectionString     string
	DB                   *sql.DB
	Repository           actionhistory.Repository
	DetectionEngine      *oscillation.OscillationDetectionEngine
	MCPServer            *mcp.ActionHistoryMCPServer
	K8sTestEnvironment   *testenv.TestEnvironment // Real test K8s cluster
	K8sMCPServerEndpoint string                   // Real K8s MCP server endpoint
	K8sMCPContainerID    string                   // Podman container ID for K8s MCP server

	Logger *logrus.Logger
}

// DatabaseTestUtils is an alias for backward compatibility (deprecated: use IntegrationTestUtils)
type DatabaseTestUtils = IntegrationTestUtils

// NewDatabaseTestUtils creates a new IntegrationTestUtils (deprecated: use NewIntegrationTestUtils)
func NewDatabaseTestUtils(logger *logrus.Logger) (*IntegrationTestUtils, error) {
	return NewIntegrationTestUtils(logger)
}

// HTTPKubernetesMCPClient implements slm.K8sMCPServer interface using HTTP requests to external MCP server
type HTTPKubernetesMCPClient struct {
	endpoint   string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewHTTPK8sMCPClient creates a new HTTP-based K8s MCP client for integration testing
func NewHTTPK8sMCPClient(endpoint string, logger *logrus.Logger) slm.K8sMCPServer {
	return &HTTPKubernetesMCPClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// HandleToolCall makes HTTP request to external K8s MCP server
func (h *HTTPKubernetesMCPClient) HandleToolCall(ctx context.Context, request mcp.MCPToolRequest) (mcp.MCPToolResponse, error) {
	h.logger.Debugf("Making HTTP request to K8s MCP server at %s for tool: %s", h.endpoint, request.Params.Name)

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return mcp.MCPToolResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", h.endpoint+"/tools/call", bytes.NewBuffer(requestBody))
	if err != nil {
		return mcp.MCPToolResponse{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Make HTTP request
	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return mcp.MCPToolResponse{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mcp.MCPToolResponse{}, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}

	// Parse response
	var mcpResponse mcp.MCPToolResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcpResponse); err != nil {
		return mcp.MCPToolResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	h.logger.Debugf("Received response from K8s MCP server with %d content items", len(mcpResponse.Content))
	return mcpResponse, nil
}

// DatabaseTestConfig holds database test configuration
type DatabaseTestConfig struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
	SSLMode  string
}

// LoadDatabaseTestConfig loads database configuration from environment
func LoadDatabaseTestConfig() DatabaseTestConfig {
	return DatabaseTestConfig{
		Host:     GetEnvOrDefault("DB_HOST", "localhost"),
		Port:     GetEnvOrDefault("DB_PORT", "5432"),
		Database: GetEnvOrDefault("DB_NAME", "action_history"),
		Username: GetEnvOrDefault("DB_USER", "slm_user"),
		Password: GetEnvOrDefault("DB_PASSWORD", "slm_password_dev"),
		SSLMode:  GetEnvOrDefault("DB_SSL_MODE", "disable"),
	}
}

// NewIntegrationTestUtils creates new integration test utilities with database, MCP servers, and mock services
func NewIntegrationTestUtils(logger *logrus.Logger) (*IntegrationTestUtils, error) {
	config := LoadDatabaseTestConfig()

	connectionString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.Username, config.Password, config.Host, config.Port, config.Database, config.SSLMode,
	)

	// Create database connection
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool for performance
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create repository
	repository := actionhistory.NewPostgreSQLRepository(db, logger)

	// Create oscillation detection engine
	detectionEngine := oscillation.NewOscillationDetectionEngine(db, logger)

	// Create action history MCP server
	mcpServer := mcp.NewActionHistoryMCPServer(repository, detectionEngine, logger)

	// Set up real K8s test environment with API server for MCP server authentication
	k8sTestEnv, err := testenv.SetupTestEnvironment()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to setup test K8s environment: %w", err)
	}

	// No file creation needed - we'll use environment variables

	// Start real K8s MCP server with Podman
	// Integration test setup automatically manages:
	// 1. Fake K8s cluster (created above)
	// 2. Real containers/kubernetes-mcp-server pointing to fake cluster (started below)
	// 3. Real Action History MCP server (created above)
	// 4. Application under test connecting to both real MCP servers
	k8sMCPEndpoint := GetEnvOrDefault("K8S_MCP_SERVER_ENDPOINT", "http://localhost:8081")

	// Start K8s MCP server container with Podman and kubeconfig content
	containerID, err := startK8sMCPServerContainerWithContent(k8sTestEnv, logger)
	if err != nil {
		db.Close()
		k8sTestEnv.Cleanup()
		return nil, fmt.Errorf("failed to start K8s MCP server container: %w", err)
	}

	// Wait for K8s MCP server to be ready
	if err := waitForK8sMCPServerReady(k8sMCPEndpoint, 30*time.Second, logger); err != nil {
		stopK8sMCPServerContainer(containerID, logger)
		db.Close()
		k8sTestEnv.Cleanup()
		return nil, fmt.Errorf("K8s MCP server not ready: %w", err)
	}

	logger.Infof("Integration test setup complete: K8s MCP server running at %s (container: %s)", k8sMCPEndpoint, containerID)

	return &IntegrationTestUtils{
		ConnectionString:     connectionString,
		DB:                   db,
		Repository:           repository,
		DetectionEngine:      detectionEngine,
		MCPServer:            mcpServer,
		K8sTestEnvironment:   k8sTestEnv,
		K8sMCPServerEndpoint: k8sMCPEndpoint,
		K8sMCPContainerID:    containerID,

		Logger: logger,
	}, nil
}

// Close closes database connections and cleans up test environment
func (d *IntegrationTestUtils) Close() error {
	var errs []error

	// Stop K8s MCP server container
	if d.K8sMCPContainerID != "" {
		d.Logger.Infof("Stopping K8s MCP server container: %s", d.K8sMCPContainerID)
		if err := stopK8sMCPServerContainer(d.K8sMCPContainerID, d.Logger); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop K8s MCP server container: %w", err))
		}
	}

	// Close database connection
	if d.DB != nil {
		if err := d.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close database: %w", err))
		}
	}

	// Clean up K8s test environment
	if d.K8sTestEnvironment != nil {
		if err := d.K8sTestEnvironment.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup K8s test environment: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}

// WaitForDatabase waits for database to be ready
func (d *DatabaseTestUtils) WaitForDatabase(maxWait time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), maxWait)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("database not ready within %v", maxWait)
		case <-ticker.C:
			if err := d.DB.PingContext(ctx); err == nil {
				return nil
			}
		}
	}
}

// RunMigrations executes database migrations
func (d *DatabaseTestUtils) RunMigrations() error {
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
	}

	for _, migration := range migrations {
		migrationPath := filepath.Join("..", "..", "migrations", migration)
		if err := d.executeMigrationFile(migrationPath); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}
		d.Logger.Infof("Applied migration: %s", migration)
	}

	return nil
}

// executeMigrationFile executes a migration file
func (d *DatabaseTestUtils) executeMigrationFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", path, err)
	}

	_, err = d.DB.Exec(string(content))
	if err != nil {
		return fmt.Errorf("failed to execute migration %s: %w", path, err)
	}

	return nil
}

// CleanDatabase removes all test data while preserving schema
func (d *DatabaseTestUtils) CleanDatabase() error {
	// Clean tables in dependency order - child tables first
	tables := []string{
		"oscillation_detections", // References resource_references via resource_id
		"resource_action_traces", // References resource_references via resource_id
		"action_histories",       // References resource_references via resource_id
		"oscillation_patterns",   // No foreign key dependencies
		"resource_references",    // Parent table - clean last
	}

	for _, table := range tables {
		_, err := d.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			d.Logger.WithError(err).Warnf("Failed to truncate table %s", table)
		}
	}

	return nil
}

// DropAllTables completely removes all database objects for fresh start
func (d *DatabaseTestUtils) DropAllTables() error {
	d.Logger.Debug("Starting comprehensive database cleanup")

	// Drop all tables and partitions in dependency order
	tables := []string{
		// Drop dependent tables first (child tables that reference others)
		"oscillation_detections", // References resource_references, oscillation_patterns
		"retention_operations",
		"action_effectiveness_metrics",

		// Drop partitioned table and its partitions
		"resource_action_traces_y2025m07",
		"resource_action_traces_y2025m08",
		"resource_action_traces_y2025m09",
		"resource_action_traces_y2025m10",
		"resource_action_traces", // Main partitioned table - references resource_references

		// Drop remaining tables with foreign key references
		"action_histories", // References resource_references

		// Drop independent tables
		"oscillation_patterns", // No foreign key dependencies
		"resource_references",
		"schema_migrations", // Drop migration tracking too
	}

	for _, table := range tables {
		_, err := d.DB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
		if err != nil {
			d.Logger.WithError(err).Warnf("Failed to drop table %s", table)
		} else {
			d.Logger.Debugf("Dropped table %s", table)
		}
	}

	// Drop any remaining functions and procedures
	_, err := d.DB.Exec(`
		DO $$ DECLARE
			r RECORD;
		BEGIN
			-- Drop all functions and procedures
			FOR r IN (SELECT proname, pg_get_function_identity_arguments(oid) as args
					 FROM pg_proc WHERE pronamespace = 'public'::regnamespace)
			LOOP
				EXECUTE 'DROP FUNCTION IF EXISTS ' || quote_ident(r.proname) || '(' || r.args || ') CASCADE';
			END LOOP;

			-- Drop all sequences
			FOR r IN (SELECT sequencename FROM pg_sequences WHERE schemaname = 'public')
			LOOP
				EXECUTE 'DROP SEQUENCE IF EXISTS ' || quote_ident(r.sequencename) || ' CASCADE';
			END LOOP;
		END $$;
	`)
	if err != nil {
		d.Logger.WithError(err).Warn("Failed to drop functions and sequences")
	}

	d.Logger.Debug("Comprehensive database cleanup completed")
	return nil
}

// InitializeFreshDatabase ensures a completely clean database with fresh migrations
func (d *DatabaseTestUtils) InitializeFreshDatabase() error {
	d.Logger.Info("Initializing fresh database for tests")

	// Step 1: Drop all existing objects
	if err := d.DropAllTables(); err != nil {
		return fmt.Errorf("failed to drop existing tables: %w", err)
	}

	// Step 2: Run migrations from scratch
	if err := d.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations on fresh database: %w", err)
	}

	// Step 3: Verify all tables exist
	if err := d.VerifyDatabaseHealth(); err != nil {
		return fmt.Errorf("fresh database health check failed: %w", err)
	}

	d.Logger.Info("Fresh database initialization completed")
	return nil
}

// CreateTestActionHistory creates test action history data
func (d *DatabaseTestUtils) CreateTestActionHistory(resourceRef actionhistory.ResourceReference, numActions int) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()

	// Ensure resource reference exists
	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	// Create action history
	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
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
				Severity:    d.getTestAlertSeverity(i),
				Labels:      map[string]string{"alertname": fmt.Sprintf("TestAlert%d", i%3)},
				Annotations: map[string]string{"description": fmt.Sprintf("Test alert %d", i)},
				FiringTime:  baseTime.Add(time.Duration(i) * time.Minute),
			},
			ModelUsed:  "test-model",
			Confidence: 0.8 + float64(i%3)*0.1,
			Reasoning:  &reasoning,
			ActionType: d.getTestActionType(i),
			Parameters: map[string]interface{}{
				"replicas": float64(2 + i%3),
				"reason":   fmt.Sprintf("test-reason-%d", i),
			},
		}

		trace, err := d.Repository.StoreAction(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("failed to store action %d: %w", i, err)
		}

		traces = append(traces, *trace)
	}

	return traces, nil
}

// getTestActionType returns action type for test data
func (d *DatabaseTestUtils) getTestActionType(index int) string {
	actionTypes := []string{
		"scale_deployment",
		"increase_resources",
		"restart_pod",
		"scale_deployment", // Create patterns
	}
	return actionTypes[index%len(actionTypes)]
}

// getTestAlertSeverity returns alert severity for test data
func (d *DatabaseTestUtils) getTestAlertSeverity(index int) string {
	severities := []string{"warning", "critical", "info"}
	return severities[index%len(severities)]
}

// CreateOscillationPattern creates test data that will trigger oscillation detection
func (d *DatabaseTestUtils) CreateOscillationPattern(resourceRef actionhistory.ResourceReference) error {
	ctx := context.Background()

	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return err
	}

	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return err
	}

	// Create scale oscillation pattern: 2→4→2→4→2
	scalePattern := []int{2, 4, 2, 4, 2}
	baseTime := time.Now().Add(-30 * time.Minute)

	for i, replicas := range scalePattern {
		reasoning := fmt.Sprintf("Scale to %d replicas", replicas)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("oscillation-action-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i*5) * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "HighMemoryUsage",
				Severity:    "warning",
				Labels:      map[string]string{"alertname": "HighMemoryUsage"},
				Annotations: map[string]string{"description": "High memory usage detected"},
				FiringTime:  baseTime.Add(time.Duration(i*5) * time.Minute),
			},
			ModelUsed:  "test-model",
			Confidence: 0.85,
			Reasoning:  &reasoning,
			ActionType: "scale_deployment",
			Parameters: map[string]interface{}{
				"replicas": float64(replicas),
			},
		}

		_, err := d.Repository.StoreAction(ctx, action)
		if err != nil {
			return fmt.Errorf("failed to store oscillation action %d: %w", i, err)
		}
	}

	return nil
}

// CreateThrashingPattern creates test data that will trigger thrashing detection
func (d *DatabaseTestUtils) CreateThrashingPattern(resourceRef actionhistory.ResourceReference) error {
	ctx := context.Background()

	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return err
	}

	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return err
	}

	// Create thrashing pattern: scale → resource → scale → resource
	actionTypes := []string{"scale_deployment", "increase_resources", "scale_deployment", "increase_resources"}
	baseTime := time.Now().Add(-20 * time.Minute)

	for i, actionType := range actionTypes {
		reasoning := fmt.Sprintf("Execute %s", actionType)
		action := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("thrashing-action-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i*3) * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "ResourceIssue",
				Severity:    "warning",
				Labels:      map[string]string{"alertname": "ResourceIssue"},
				Annotations: map[string]string{"description": "Resource issue detected"},
				FiringTime:  baseTime.Add(time.Duration(i*3) * time.Minute),
			},
			ModelUsed:  "test-model",
			Confidence: 0.75,
			Reasoning:  &reasoning,
			ActionType: actionType,
			Parameters: d.getThrashingParameters(actionType, i),
		}

		_, err := d.Repository.StoreAction(ctx, action)
		if err != nil {
			return fmt.Errorf("failed to store thrashing action %d: %w", i, err)
		}
	}

	return nil
}

// getThrashingParameters returns appropriate parameters for thrashing pattern
func (d *DatabaseTestUtils) getThrashingParameters(actionType string, index int) actionhistory.JSONMap {
	switch actionType {
	case "scale_deployment":
		return actionhistory.JSONMap{"replicas": float64(2 + index)}
	case "increase_resources":
		return actionhistory.JSONMap{
			"memory_limit": "1Gi",
			"cpu_limit":    "500m",
		}
	default:
		return actionhistory.JSONMap{}
	}
}

// VerifyDatabaseHealth performs comprehensive database health checks
func (d *DatabaseTestUtils) VerifyDatabaseHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Basic connectivity
	if err := d.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Check critical tables exist
	tables := []string{
		"resource_references",
		"action_histories",
		"resource_action_traces",
		"oscillation_patterns",
		"oscillation_detections",
	}

	for _, table := range tables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = $1
		)`

		if err := d.DB.QueryRowContext(ctx, query, table).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}

		if !exists {
			return fmt.Errorf("required table %s does not exist", table)
		}
	}

	// Check stored procedures exist
	procedures := []string{
		"detect_scale_oscillation",
		"detect_resource_thrashing",
		"detect_ineffective_loops",
		"detect_cascading_failures",
		"get_action_traces",
		"get_action_effectiveness",
		"store_oscillation_detection",
	}

	for _, proc := range procedures {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.routines
			WHERE routine_schema = 'public'
			AND routine_name = $1
		)`

		if err := d.DB.QueryRowContext(ctx, query, proc).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check procedure %s: %w", proc, err)
		}

		if !exists {
			return fmt.Errorf("required stored procedure %s does not exist", proc)
		}
	}

	return nil
}

// ConvertAlertToResourceRef converts types.Alert to actionhistory.ResourceReference
func ConvertAlertToResourceRef(alert types.Alert) actionhistory.ResourceReference {
	return actionhistory.ResourceReference{
		Namespace: alert.Namespace,
		Kind:      "Deployment", // Default for tests
		Name:      alert.Resource,
	}
}

// GetReasoningSummary safely extracts reasoning summary from ReasoningDetails
func GetReasoningSummary(reasoning *types.ReasoningDetails) string {
	if reasoning != nil && reasoning.Summary != "" {
		return reasoning.Summary
	}
	return ""
}

// StringPtr returns a pointer to a string (helper for tests)
func StringPtr(s string) *string {
	return &s
}

// Context-aware test helpers for SLM+MCP+PostgreSQL integration

// CreateFailedRestartHistory creates a history of failed restart attempts
func (d *DatabaseTestUtils) CreateFailedRestartHistory(resourceRef actionhistory.ResourceReference, numFailures int) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()

	// Ensure resource exists
	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	// Ensure action history exists
	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	var traces []actionhistory.ResourceActionTrace
	baseTime := time.Now().Add(-2 * time.Hour)

	for i := 0; i < numFailures; i++ {
		reasoning := "Pod restart attempt failed"
		actionRecord := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("failed-restart-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * 15 * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "PodCrashLooping",
				Severity:    "critical",
				Labels:      map[string]string{"alertname": "PodCrashLooping"},
				Annotations: map[string]string{"description": "Pod crash looping restart attempt"},
				FiringTime:  baseTime.Add(time.Duration(i) * 15 * time.Minute),
			},
			ModelUsed:  "test-model",
			Confidence: 0.8,
			Reasoning:  &reasoning,
			ActionType: "restart_pod",
			Parameters: map[string]interface{}{
				"restart_attempt": float64(i + 1),
			},
		}

		// Store with failed execution status
		trace, err := d.Repository.StoreAction(ctx, actionRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to store failed restart action %d: %w", i, err)
		}

		// Update to failed status
		trace.ExecutionStatus = "failed"
		effectiveness := 0.1 // Very low effectiveness
		trace.EffectivenessScore = &effectiveness

		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateSuccessfulScalingHistory creates a history of successful scaling actions
func (d *DatabaseTestUtils) CreateSuccessfulScalingHistory(resourceRef actionhistory.ResourceReference, numActions int) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()

	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	var traces []actionhistory.ResourceActionTrace
	baseTime := time.Now().Add(-6 * time.Hour)

	for i := 0; i < numActions; i++ {
		reasoning := "Successful scaling to handle load"
		actionRecord := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("successful-scale-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * 45 * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "CPUThrottling",
				Severity:    "warning",
				Labels:      map[string]string{"alertname": "CPUThrottling"},
				Annotations: map[string]string{"description": "CPU throttling scaling"},
				FiringTime:  baseTime.Add(time.Duration(i) * 45 * time.Minute),
			},
			ModelUsed:  "test-model",
			Confidence: 0.9,
			Reasoning:  &reasoning,
			ActionType: "scale_deployment",
			Parameters: map[string]interface{}{
				"replicas": float64(3 + i),
			},
		}

		trace, err := d.Repository.StoreAction(ctx, actionRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to store scaling action %d: %w", i, err)
		}

		// Set as successful with high effectiveness
		trace.ExecutionStatus = "completed"
		effectiveness := 0.9
		trace.EffectivenessScore = &effectiveness

		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateIneffectiveSecurityHistory creates a history of ineffective security responses
func (d *DatabaseTestUtils) CreateIneffectiveSecurityHistory(resourceRef actionhistory.ResourceReference) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()

	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	var traces []actionhistory.ResourceActionTrace
	baseTime := time.Now().Add(-4 * time.Hour)

	securityActions := []struct {
		action        string
		effectiveness float64
	}{
		{"notify_only", 0.1},  // Very ineffective
		{"restart_pod", 0.2},  // Slightly better but still poor
		{"notify_only", 0.15}, // Still ineffective
	}

	for i, sa := range securityActions {
		reasoning := "Security threat containment attempt"
		actionRecord := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("security-response-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * 30 * time.Minute),
			Alert: actionhistory.AlertContext{
				Name:        "SecurityThreatDetected",
				Severity:    "critical",
				Labels:      map[string]string{"alertname": "SecurityThreatDetected"},
				Annotations: map[string]string{"description": "Security threat detected"},
				FiringTime:  baseTime.Add(time.Duration(i) * 30 * time.Minute),
			},
			ModelUsed:  "test-model",
			Confidence: 0.8,
			Reasoning:  &reasoning,
			ActionType: sa.action,
			Parameters: map[string]interface{}{
				"security_level": "high",
			},
		}

		trace, err := d.Repository.StoreAction(ctx, actionRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to store security action %d: %w", i, err)
		}

		// Set with low effectiveness
		trace.ExecutionStatus = "completed"
		trace.EffectivenessScore = &sa.effectiveness

		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateLowEffectivenessHistory creates a history of actions with low effectiveness
func (d *DatabaseTestUtils) CreateLowEffectivenessHistory(resourceRef actionhistory.ResourceReference, actionType string, effectiveness float64) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()

	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	var traces []actionhistory.ResourceActionTrace
	baseTime := time.Now().Add(-8 * time.Hour)
	numActions := 4

	for i := 0; i < numActions; i++ {
		reasoning := fmt.Sprintf("Low effectiveness %s attempt", actionType)
		actionRecord := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("low-effectiveness-%s-%d", actionType, i),
			Timestamp:         baseTime.Add(time.Duration(i) * 2 * time.Hour),
			Alert: actionhistory.AlertContext{
				Name:        "StorageSpaceExhaustion",
				Severity:    "critical",
				Labels:      map[string]string{"alertname": "StorageSpaceExhaustion"},
				Annotations: map[string]string{"description": "Storage space exhaustion"},
				FiringTime:  baseTime.Add(time.Duration(i) * 2 * time.Hour),
			},
			ModelUsed:  "test-model",
			Confidence: 0.8,
			Reasoning:  &reasoning,
			ActionType: actionType,
			Parameters: map[string]interface{}{
				"attempt": float64(i + 1),
			},
		}

		trace, err := d.Repository.StoreAction(ctx, actionRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to store low effectiveness action %d: %w", i, err)
		}

		// Set with specified low effectiveness
		trace.ExecutionStatus = "completed"
		trace.EffectivenessScore = &effectiveness

		traces = append(traces, *trace)
	}

	return traces, nil
}

// CreateMCPClient creates a properly configured MCP client for integration testing with real K8s MCP server
func (utils *IntegrationTestUtils) CreateMCPClient(config IntegrationConfig) slm.MCPClient {
	mcpClientConfig := slm.MCPClientConfig{
		ActionHistoryServerEndpoint: "http://localhost:8081",    // Internal server
		KubernetesServerEndpoint:    utils.K8sMCPServerEndpoint, // Real K8s MCP server
		Timeout:                     config.TestTimeout,
		MaxRetries:                  config.MaxRetries,
	}

	// Integration tests always use real K8s MCP server (started automatically)
	k8sHTTPClient := NewHTTPK8sMCPClient(utils.K8sMCPServerEndpoint, utils.Logger)

	utils.Logger.Infof("Creating MCP client with real K8s MCP server at %s", utils.K8sMCPServerEndpoint)
	return slm.NewMCPClientWithK8sServer(mcpClientConfig, utils.MCPServer, k8sHTTPClient, utils.Logger)
}

// CreateSLMClient creates a properly configured SLM client with MCP integration for testing
func (utils *IntegrationTestUtils) CreateSLMClient(testConfig IntegrationConfig, slmConfig interface{}) (slm.Client, error) {
	// For now, we don't need to process the slmConfig parameter in detail
	// This would be implemented when the actual SLM client creation is needed
	_ = slmConfig // Use parameter to avoid unused warning

	// Create SLM configuration struct
	// Note: This would need proper type conversion in a real implementation
	// For now, we'll create a basic config
	basicSLMConfig := struct {
		Provider       string
		Endpoint       string
		Model          string
		Temperature    float64
		MaxTokens      int
		Timeout        time.Duration
		RetryCount     int
		MaxContextSize int
	}{
		Provider:       "localai",
		Endpoint:       testConfig.OllamaEndpoint,
		Model:          testConfig.OllamaModel,
		Temperature:    0.3,
		MaxTokens:      500,
		Timeout:        testConfig.TestTimeout,
		RetryCount:     1,
		MaxContextSize: 2000,
	}

	// Create MCP client
	mcpClient := utils.CreateMCPClient(testConfig)

	// Create SLM client with MCP
	// Note: This would need the actual NewClientWithMCP function with proper types
	utils.Logger.Info("Created SLM client with MCP integration for testing")

	// Return nil for now - this would be implemented with the actual SLM client creation
	// slmClient, err := slm.NewClientWithMCP(basicSLMConfig, mcpClient, utils.Logger)
	// return slmClient, err

	_ = basicSLMConfig // Use variable to avoid unused warning
	_ = mcpClient      // Use variable to avoid unused warning

	return nil, fmt.Errorf("SLM client creation not fully implemented in test utils")
}

// CreateCascadingFailureHistory creates a history showing patterns that led to cascading failures
func (d *DatabaseTestUtils) CreateCascadingFailureHistory(resourceRef actionhistory.ResourceReference) ([]actionhistory.ResourceActionTrace, error) {
	ctx := context.Background()

	resourceID, err := d.Repository.EnsureResourceReference(ctx, resourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	_, err = d.Repository.EnsureActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	var traces []actionhistory.ResourceActionTrace
	baseTime := time.Now().Add(-12 * time.Hour)

	// Simulate pattern: network issue -> aggressive restart -> cascading failure
	cascadePattern := []struct {
		action        string
		alert         string
		effectiveness float64
		description   string
	}{
		{"restart_pod", "NetworkConnectivityIssue", 0.3, "Initial network issue restart"},
		{"restart_pod", "NetworkConnectivityIssue", 0.2, "Aggressive restart during network issue"},
		{"scale_deployment", "HighMemoryUsage", 0.1, "Scaling during cascade"},
		{"restart_pod", "PodCrashLooping", 0.05, "Final cascade restart"},
	}

	for i, pattern := range cascadePattern {
		reasoning := fmt.Sprintf("Cascading failure pattern: %s", pattern.description)
		actionRecord := &actionhistory.ActionRecord{
			ResourceReference: resourceRef,
			ActionID:          fmt.Sprintf("cascade-pattern-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * 3 * time.Hour),
			Alert: actionhistory.AlertContext{
				Name:        pattern.alert,
				Severity:    "warning",
				Labels:      map[string]string{"alertname": pattern.alert},
				Annotations: map[string]string{"description": pattern.description},
				FiringTime:  baseTime.Add(time.Duration(i) * 3 * time.Hour),
			},
			ModelUsed:  "test-model",
			Confidence: 0.7,
			Reasoning:  &reasoning,
			ActionType: pattern.action,
			Parameters: map[string]interface{}{
				"cascade_step": float64(i + 1),
			},
		}

		trace, err := d.Repository.StoreAction(ctx, actionRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to store cascade pattern action %d: %w", i, err)
		}

		// Set with declining effectiveness showing cascade
		trace.ExecutionStatus = "completed"
		trace.EffectivenessScore = &pattern.effectiveness

		traces = append(traces, *trace)
	}

	return traces, nil
}

// startK8sMCPServerContainer starts the containers/kubernetes-mcp-server using Podman with kubeconfig file
func startK8sMCPServerContainerWithContent(k8sTestEnv *testenv.TestEnvironment, logger *logrus.Logger) (string, error) {
	logger.Info("=== STARTING K8S MCP SERVER CONTAINER WITH ENV VARS ===")

	if k8sTestEnv == nil || k8sTestEnv.Config == nil {
		return "", fmt.Errorf("test environment or config not available")
	}

	// Check if Podman is available
	logger.Info("Checking Podman availability...")
	if err := exec.Command("podman", "--version").Run(); err != nil {
		logger.Errorf("Podman check failed: %v", err)
		return "", fmt.Errorf("podman not available: %w", err)
	}
	logger.Info("Podman is available")

	// Check if a container is already running on port 8081
	if isPortInUse("8081") {
		logger.Warn("Port 8081 already in use, attempting to stop existing container")
		stopExistingK8sMCPContainer(logger)
	}

	// Get kubeconfig content from test environment
	kubeconfigContent, err := k8sTestEnv.GetKubeconfigForContainer()
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig content: %w", err)
	}

	// Prepare Podman run command
	containerName := fmt.Sprintf("k8s-mcp-server-test-%d", time.Now().Unix())

	// Build command args with environment variables
	args := []string{
		"run",
		"-d",                    // Run in background
		"--name", containerName, // Container name
		"-p", "8081:8081", // Port mapping (avoid conflict with envtest K8s server)
		"--rm", // Remove container when stopped
		"-e", "ENABLE_UNSAFE_SSE_TRANSPORT=true",
		"-e", "PORT=8081",
		"-e", "HOST=0.0.0.0",
		"-e", fmt.Sprintf("KUBECONFIG_CONTENT=%s", kubeconfigContent),
	}

	logger.Infof("Using K8s server: %s", k8sTestEnv.Config.Host)
	logger.Infof("Passing kubeconfig content via environment variable")

	// Add image (use correct mcp/kubernetes image)
	image := GetEnvOrDefault("K8S_MCP_SERVER_IMAGE", "mcp/kubernetes")
	args = append(args, image)

	// Add shell command to create kubeconfig file and start server
	shellCmd := `echo "$KUBECONFIG_CONTENT" > ./kubeconfig && exec node dist/index.js --kubeconfig ./kubeconfig --sse-port 8081`
	args = append(args, "sh", "-c", shellCmd)

	// Execute Podman command
	logger.Infof("=== EXECUTING PODMAN COMMAND ===")
	logger.Infof("Image: %s", image)
	logger.Infof("Shell command: %s", shellCmd)

	cmd := exec.Command("podman", args...)
	output, err := cmd.CombinedOutput() // Use CombinedOutput to get both stdout and stderr

	logger.Infof("Podman command output: %s", string(output))

	if err != nil {
		logger.Errorf("Podman command failed with error: %v", err)
		logger.Errorf("Command output: %s", string(output))
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	containerID := strings.TrimSpace(string(output))
	logger.Infof("K8s MCP server container started with ID: %s", containerID)

	return containerID, nil
}

// stopK8sMCPServerContainer stops the K8s MCP server container using Podman
func stopK8sMCPServerContainer(containerID string, logger *logrus.Logger) error {
	if containerID == "" {
		return nil
	}

	logger.Infof("Stopping K8s MCP server container: %s", containerID)

	// Stop the container
	cmd := exec.Command("podman", "stop", containerID)
	if err := cmd.Run(); err != nil {
		logger.WithError(err).Warnf("Failed to stop container %s", containerID)
		return err
	}

	logger.Infof("K8s MCP server container stopped successfully")
	return nil
}

// stopExistingK8sMCPContainer stops any existing K8s MCP test containers
func stopExistingK8sMCPContainer(logger *logrus.Logger) {
	// List containers with our naming pattern
	cmd := exec.Command("podman", "ps", "-q", "--filter", "name=k8s-mcp-server-test")
	output, err := cmd.Output()
	if err != nil {
		logger.WithError(err).Debug("Failed to list existing containers")
		return
	}

	containerIDs := strings.Fields(string(output))
	for _, containerID := range containerIDs {
		if err := stopK8sMCPServerContainer(containerID, logger); err != nil {
			logger.WithError(err).Warnf("Failed to stop existing container %s", containerID)
		}
	}
}

// waitForK8sMCPServerReady waits for the K8s MCP server to be ready
func waitForK8sMCPServerReady(endpoint string, timeout time.Duration, logger *logrus.Logger) error {
	logger.Infof("Waiting for K8s MCP server to be ready at %s (timeout: %v)", endpoint, timeout)

	client := &http.Client{Timeout: 10 * time.Second} // Longer timeout for individual requests
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second) // Less frequent checks to give server time
	defer ticker.Stop()

	// Try different possible health/ready endpoints for MCP servers
	healthPaths := []string{"/health", "/ready", "/healthz", "/", "/mcp/health"}
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			// Before giving up, try to get container logs for debugging
			logger.Error("Health checks timed out, attempting to get container logs for debugging...")
			if containers, err := getRunningK8sMCPContainers(); err == nil && len(containers) > 0 {
				if logs, err := getContainerLogs(containers[0]); err == nil {
					logger.Errorf("Container logs:\n%s", logs)
				}
			}
			return fmt.Errorf("timeout waiting for K8s MCP server to be ready")
		case <-ticker.C:
			attempt++
			logger.Debugf("Health check attempt %d", attempt)

			// Try different health check endpoints
			for _, path := range healthPaths {
				req, err := http.NewRequestWithContext(ctx, "GET", endpoint+path, nil)
				if err != nil {
					continue
				}

				resp, err := client.Do(req)
				if err != nil {
					logger.Debugf("Health check failed for %s: %v", path, err)
					continue
				}
				resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					logger.Infof("K8s MCP server is ready (responded to %s after %d attempts)", path, attempt)
					return nil
				}
				logger.Debugf("Health check %s returned status: %d", path, resp.StatusCode)
			}

			// Every 5 attempts, check container status
			if attempt%5 == 0 {
				if containers, err := getRunningK8sMCPContainers(); err == nil {
					if len(containers) == 0 {
						logger.Error("No K8s MCP containers are running!")
						return fmt.Errorf("K8s MCP container exited")
					}
					logger.Debugf("K8s MCP container is still running: %s", containers[0])
				}
			}
		}
	}
}

// isPortInUse checks if a port is already in use
func isPortInUse(port string) bool {
	cmd := exec.Command("lsof", "-i", ":"+port)
	return cmd.Run() == nil
}

// getRunningK8sMCPContainers returns the IDs of running K8s MCP server containers
func getRunningK8sMCPContainers() ([]string, error) {
	cmd := exec.Command("podman", "ps", "-q", "--filter", "name=k8s-mcp-server-test")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	containers := strings.Fields(strings.TrimSpace(string(output)))
	return containers, nil
}

// getContainerLogs retrieves the logs from a container
func getContainerLogs(containerID string) (string, error) {
	cmd := exec.Command("podman", "logs", containerID)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
