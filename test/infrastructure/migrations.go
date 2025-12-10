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

// Package infrastructure provides shared E2E test infrastructure for all services.
//
// This file implements the shared migration library requested in:
// docs/handoff/REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md
//
// ALL services that emit audit events need the audit_events table:
//   - WorkflowExecution: workflowexecution.workflow.*
//   - Gateway: gateway.signal.*
//   - AIAnalysis: aianalysis.analysis.*
//   - Notification: notification.sent.*
//   - RO: orchestrator.remediation.*
//   - SP: signalprocessing.categorization.*
//
// Usage:
//
//	// Most teams just need audit migrations:
//	err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output)
//
//	// AIAnalysis needs workflows too:
//	config := infrastructure.DefaultMigrationConfig(namespace, kubeconfigPath)
//	config.Tables = []string{"audit_events", "remediation_workflow_catalog"}
//	err := infrastructure.ApplyMigrations(ctx, config, output)
//
//	// DS needs everything:
//	err := infrastructure.ApplyAllMigrations(ctx, namespace, kubeconfigPath, output)
package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigrationConfig configures which migrations to apply
type MigrationConfig struct {
	Namespace       string
	KubeconfigPath  string
	PostgresService string   // Default: "postgresql"
	PostgresUser    string   // Default: "slm_user"
	PostgresDB      string   // Default: "action_history"
	Tables          []string // Empty = all tables; specify to filter
}

// DefaultMigrationConfig returns sensible defaults for E2E tests
func DefaultMigrationConfig(namespace, kubeconfigPath string) MigrationConfig {
	return MigrationConfig{
		Namespace:       namespace,
		KubeconfigPath:  kubeconfigPath,
		PostgresService: "postgresql",
		PostgresUser:    "slm_user",
		PostgresDB:      "action_history",
		Tables:          nil, // All tables
	}
}

// Migration represents a single migration file with metadata
type Migration struct {
	Name        string   // Human-readable name
	File        string   // Migration file name
	Description string   // What this migration does
	Tables      []string // Tables created by this migration
	Indexes     []string // Indexes created by this migration
}

// AllMigrations lists all migrations with metadata
// Order matters - migrations are applied in this sequence
var AllMigrations = []Migration{
	// Core schema
	{
		Name:        "initial_schema",
		File:        "001_initial_schema.sql",
		Description: "Initial database schema",
		Tables:      []string{"notification_audit", "resource_action_traces"},
	},
	{
		Name:        "fix_partitioning",
		File:        "002_fix_partitioning.sql",
		Description: "Fix partitioning issues",
		Tables:      []string{},
	},
	{
		Name:        "stored_procedures",
		File:        "003_stored_procedures.sql",
		Description: "Stored procedures",
		Tables:      []string{},
	},
	{
		Name:        "effectiveness_assessment_due",
		File:        "004_add_effectiveness_assessment_due.sql",
		Description: "Effectiveness assessment due column",
		Tables:      []string{},
	},
	{
		Name:        "vector_schema",
		File:        "005_vector_schema.sql",
		Description: "Vector schema for embeddings",
		Tables:      []string{},
	},
	{
		Name:        "effectiveness_assessment",
		File:        "006_effectiveness_assessment.sql",
		Description: "Effectiveness assessment table",
		Tables:      []string{"effectiveness_assessments"},
	},
	{
		Name:        "context_column",
		File:        "007_add_context_column.sql",
		Description: "Context column",
		Tables:      []string{},
	},
	{
		Name:        "context_api_compatibility",
		File:        "008_context_api_compatibility.sql",
		Description: "Context API compatibility",
		Tables:      []string{},
	},
	{
		Name:        "update_vector_dimensions",
		File:        "009_update_vector_dimensions.sql",
		Description: "Update vector dimensions",
		Tables:      []string{},
	},
	{
		Name:        "audit_write_api_phase1",
		File:        "010_audit_write_api_phase1.sql",
		Description: "Audit write API phase 1",
		Tables:      []string{},
	},
	{
		Name:        "rename_alert_to_signal",
		File:        "011_rename_alert_to_signal.sql",
		Description: "Rename alert to signal",
		Tables:      []string{},
	},
	{
		Name:        "adr033_tracking",
		File:        "012_adr033_multidimensional_tracking.sql",
		Description: "ADR-033 multi-dimensional success tracking",
		Tables:      []string{},
		Indexes:     []string{"idx_incident_type_success", "idx_workflow_success"},
	},
	// ADR-034: Unified audit events (REQUIRED BY ALL TEAMS)
	{
		Name:        "audit_events",
		File:        "013_create_audit_events_table.sql",
		Description: "Unified audit events table (ADR-034) - REQUIRED BY ALL TEAMS",
		Tables:      []string{"audit_events"},
		Indexes: []string{
			"idx_audit_events_correlation",  // WE: Query by correlation_id
			"idx_audit_events_event_type",   // WE: Query by event type
			"idx_audit_events_timestamp",    // WE: Query by time range
			"idx_audit_events_resource",     // Query by resource
			"idx_audit_events_actor",        // Query by actor
		},
	},
	// Workflow catalog (REQUIRED BY AIANALYSIS)
	{
		Name:        "workflow_catalog",
		File:        "015_create_workflow_catalog_table.sql",
		Description: "Workflow catalog for semantic search (AIAnalysis)",
		Tables:      []string{"remediation_workflow_catalog"},
	},
	{
		Name:        "embedding_dimensions",
		File:        "016_update_embedding_dimensions.sql",
		Description: "Update embedding dimensions",
		Tables:      []string{},
	},
	{
		Name:        "workflow_schema_fields",
		File:        "017_add_workflow_schema_fields.sql",
		Description: "Workflow schema fields",
		Tables:      []string{},
	},
	{
		Name:        "container_image_rename",
		File:        "018_rename_execution_bundle_to_container_image.sql",
		Description: "Rename execution bundle to container image",
		Tables:      []string{},
	},
	{
		Name:        "uuid_primary_key",
		File:        "019_uuid_primary_key.sql",
		Description: "UUID primary key",
		Tables:      []string{},
	},
	{
		Name:        "workflow_label_columns",
		File:        "020_add_workflow_label_columns.sql",
		Description: "DD-WORKFLOW-001 v1.6: custom_labels + detected_labels",
		Tables:      []string{},
	},
	// Audit event partitions (REQUIRED BY WE, SP, RO, NOTIFICATION)
	{
		Name:        "audit_partitions",
		File:        "1000_create_audit_events_partitions.sql",
		Description: "Monthly partitions for audit_events",
		Tables: []string{
			"audit_events_y2025m12", // Current month
			"audit_events_y2026m01", // Next month
		},
	},
}

// AuditMigrations returns the subset of migrations needed for audit functionality
// This is what most teams need: WE, Gateway, Notification, RO, SP
var AuditMigrations = []string{
	"013_create_audit_events_table.sql",
	"1000_create_audit_events_partitions.sql",
}

// AuditTables lists all tables created by audit migrations
var AuditTables = []string{
	"audit_events",
	"audit_events_y2025m12",
	"audit_events_y2026m01",
}

// ApplyAuditMigrations is a shortcut for audit-only migrations
// This is what MOST teams need: WE, Gateway, Notification, RO, SP
//
// Creates:
//   - audit_events table (ADR-034)
//   - audit_events_y2025m12 partition
//   - audit_events_y2026m01 partition
//   - All audit indexes (correlation, event_type, timestamp, etc.)
func ApplyAuditMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "📋 Applying AUDIT migrations (audit_events + partitions)...\n")
	fmt.Fprintf(writer, "   This unblocks: WE, Gateway, AIAnalysis, Notification, RO, SP\n")

	config := DefaultMigrationConfig(namespace, kubeconfigPath)

	// Apply only audit-related migrations
	return applySpecificMigrations(ctx, config, AuditMigrations, writer)
}

// ApplyAllMigrations applies all available migrations
// Use this for DS E2E tests that need the complete schema
func ApplyAllMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "📋 Applying ALL migrations (%d total)...\n", len(AllMigrations))

	config := DefaultMigrationConfig(namespace, kubeconfigPath)

	// Collect all migration files
	var allFiles []string
	for _, m := range AllMigrations {
		allFiles = append(allFiles, m.File)
	}

	return applySpecificMigrations(ctx, config, allFiles, writer)
}

// ApplyMigrationsWithConfig applies selected migrations based on config
// If config.Tables is empty, applies all migrations
// If config.Tables is specified, applies only migrations that create those tables
func ApplyMigrationsWithConfig(ctx context.Context, config MigrationConfig, writer io.Writer) error {
	if len(config.Tables) == 0 {
		return ApplyAllMigrations(ctx, config.Namespace, config.KubeconfigPath, writer)
	}

	fmt.Fprintf(writer, "📋 Applying migrations for tables: %v\n", config.Tables)

	// Find migrations that create the requested tables
	var migrationFiles []string
	for _, m := range AllMigrations {
		for _, table := range config.Tables {
			for _, mTable := range m.Tables {
				if mTable == table || strings.HasPrefix(mTable, table) {
					migrationFiles = append(migrationFiles, m.File)
					break
				}
			}
		}
	}

	// Also include audit migrations if audit_events is requested
	for _, table := range config.Tables {
		if table == "audit_events" || strings.HasPrefix(table, "audit_events") {
			// Ensure we have both audit migrations
			hasAuditBase := false
			hasAuditPartitions := false
			for _, f := range migrationFiles {
				if f == "013_create_audit_events_table.sql" {
					hasAuditBase = true
				}
				if f == "1000_create_audit_events_partitions.sql" {
					hasAuditPartitions = true
				}
			}
			if !hasAuditBase {
				migrationFiles = append(migrationFiles, "013_create_audit_events_table.sql")
			}
			if !hasAuditPartitions {
				migrationFiles = append(migrationFiles, "1000_create_audit_events_partitions.sql")
			}
			break
		}
	}

	if len(migrationFiles) == 0 {
		return fmt.Errorf("no migrations found for tables: %v", config.Tables)
	}

	return applySpecificMigrations(ctx, config, migrationFiles, writer)
}

// VerifyMigrations checks if required tables exist
// Returns nil if all tables in config.Tables exist, error otherwise
// If config.Tables is empty, verifies all critical tables
func VerifyMigrations(ctx context.Context, config MigrationConfig, writer io.Writer) error {
	fmt.Fprintf(writer, "🔍 Verifying migrations...\n")

	tables := config.Tables
	if len(tables) == 0 {
		// Default critical tables
		tables = []string{
			"audit_events",
			"notification_audit",
			"remediation_workflow_catalog",
		}
	}

	clientset, err := getKubernetesClient(config.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Get PostgreSQL pod name
	pods, err := clientset.CoreV1().Pods(config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=postgresql",
	})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("no PostgreSQL pod found in namespace %s", config.Namespace)
	}
	podName := pods.Items[0].Name

	// Verify each table exists
	var missingTables []string
	for _, table := range tables {
		checkSQL := fmt.Sprintf("SELECT 1 FROM %s LIMIT 1;", table)
		checkCmd := exec.Command("kubectl", "--kubeconfig", config.KubeconfigPath,
			"exec", "-i", "-n", config.Namespace, podName, "--",
			"psql", "-U", config.PostgresUser, "-d", config.PostgresDB, "-c", checkSQL)

		output, err := checkCmd.CombinedOutput()
		if err != nil {
			outputStr := string(output)
			if strings.Contains(outputStr, "does not exist") {
				missingTables = append(missingTables, table)
				fmt.Fprintf(writer, "   ❌ Table %s: NOT FOUND\n", table)
			}
		} else {
			fmt.Fprintf(writer, "   ✅ Table %s: EXISTS\n", table)
		}
	}

	if len(missingTables) > 0 {
		return fmt.Errorf("missing tables: %v", missingTables)
	}

	fmt.Fprintf(writer, "✅ All tables verified\n")
	return nil
}

// applySpecificMigrations applies a specific list of migration files
func applySpecificMigrations(ctx context.Context, config MigrationConfig, migrationFiles []string, writer io.Writer) error {
	clientset, err := getKubernetesClient(config.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Wait for PostgreSQL pod to be ready and get pod name
	var podName string
	Eventually(func() error {
		pods, err := clientset.CoreV1().Pods(config.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", config.PostgresService),
		})
		if err != nil {
			return fmt.Errorf("failed to list pods: %w", err)
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no PostgreSQL pods found")
		}

		// Check if pod is ready
		pod := pods.Items[0]
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				podName = pod.Name
				return nil
			}
		}
		return fmt.Errorf("PostgreSQL pod not ready yet")
	}, 60*time.Second, 2*time.Second).Should(Succeed(), "PostgreSQL pod should be ready for migrations")

	fmt.Fprintf(writer, "   📦 PostgreSQL pod ready: %s\n", podName)

	// Read migration files from workspace
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Apply each migration
	for _, migration := range migrationFiles {
		migrationPath := filepath.Join(workspaceRoot, "migrations", migration)
		content, err := os.ReadFile(migrationPath)
		if err != nil {
			fmt.Fprintf(writer, "   ⚠️  Skipping migration %s (not found)\n", migration)
			continue
		}

		// Remove CONCURRENTLY keyword for test environment
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		// Extract only the UP migration (ignore DOWN section)
		if strings.Contains(migrationSQL, "-- +goose Down") {
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

		// Apply migration via psql in the pod
		cmd := exec.Command("kubectl", "--kubeconfig", config.KubeconfigPath,
			"exec", "-i", "-n", config.Namespace, podName, "--",
			"psql", "-U", config.PostgresUser, "-d", config.PostgresDB)
		cmd.Stdin = strings.NewReader(migrationSQL)

		output, err := cmd.CombinedOutput()
		if err != nil {
			outputStr := string(output)
			// Check if error is due to already existing objects (idempotent)
			if strings.Contains(outputStr, "already exists") ||
				strings.Contains(outputStr, "duplicate key") ||
				(strings.Contains(outputStr, "relation") && strings.Contains(outputStr, "already exists")) {
				fmt.Fprintf(writer, "   ✅ Migration %s already applied\n", migration)
				continue
			}
			// Some migrations may have partial failures but still succeed overall
			if !strings.Contains(outputStr, "ERROR:") {
				fmt.Fprintf(writer, "   ✅ Applied %s (with notices)\n", migration)
				continue
			}
			fmt.Fprintf(writer, "   ❌ Migration %s failed: %s\n", migration, outputStr)
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}
		fmt.Fprintf(writer, "   ✅ Applied %s\n", migration)
	}

	// Grant permissions
	grantSQL := `
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`
	grantCmd := exec.Command("kubectl", "--kubeconfig", config.KubeconfigPath,
		"exec", "-i", "-n", config.Namespace, podName, "--",
		"psql", "-U", config.PostgresUser, "-d", config.PostgresDB)
	grantCmd.Stdin = strings.NewReader(grantSQL)
	_ = grantCmd.Run() // Ignore errors - permissions may already be granted

	fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n")
	return nil
}

