//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/mcp"
	"github.com/jordigilh/prometheus-alerts-slm/internal/oscillation"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// DatabaseTestUtils provides utilities for database integration testing
type DatabaseTestUtils struct {
	ConnectionString string
	DB               *sql.DB
	Repository       actionhistory.Repository
	DetectionEngine  *oscillation.OscillationDetectionEngine
	MCPServer        *mcp.ActionHistoryMCPServer
	Logger           *logrus.Logger
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
		Host:     shared.GetEnvOrDefault("DB_HOST", "localhost"),
		Port:     shared.GetEnvOrDefault("DB_PORT", "5432"),
		Database: shared.GetEnvOrDefault("DB_NAME", "action_history"),
		Username: shared.GetEnvOrDefault("DB_USER", "slm_user"),
		Password: shared.GetEnvOrDefault("DB_PASSWORD", "slm_password_dev"),
		SSLMode:  shared.GetEnvOrDefault("DB_SSL_MODE", "disable"),
	}
}

// NewDatabaseTestUtils creates new database test utilities
func NewDatabaseTestUtils(logger *logrus.Logger) (*DatabaseTestUtils, error) {
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

	// Create MCP server
	mcpServer := mcp.NewActionHistoryMCPServer(repository, detectionEngine, logger)

	return &DatabaseTestUtils{
		ConnectionString: connectionString,
		DB:               db,
		Repository:       repository,
		DetectionEngine:  detectionEngine,
		MCPServer:        mcpServer,
		Logger:           logger,
	}, nil
}

// Close closes database connections
func (d *DatabaseTestUtils) Close() error {
	if d.DB != nil {
		return d.DB.Close()
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

// getReasoningSummary safely extracts reasoning summary from ReasoningDetails
func getReasoningSummary(reasoning *types.ReasoningDetails) string {
	if reasoning != nil && reasoning.Summary != "" {
		return reasoning.Summary
	}
	return ""
}

// stringPtr returns a pointer to a string (helper for tests)
func stringPtr(s string) *string {
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
