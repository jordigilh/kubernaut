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
// This file implements the shared migration library.
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
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/gomega"
	"github.com/pressly/goose/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigrationConfig configures which migrations to apply
type MigrationConfig struct {
	Namespace        string
	KubeconfigPath   string
	PostgresService  string // Default: "postgresql"
	PostgresUser     string // Default: "slm_user"
	PostgresPassword string // Default: "test_password"
	PostgresDB       string // Default: "action_history"
	Tables           []string
}

// DefaultMigrationConfig returns sensible defaults for E2E tests
func DefaultMigrationConfig(namespace, kubeconfigPath string) MigrationConfig {
	return MigrationConfig{
		Namespace:        namespace,
		KubeconfigPath:   kubeconfigPath,
		PostgresService:  "postgresql",
		PostgresUser:     "slm_user",
		PostgresPassword: "test_password",
		PostgresDB:       "action_history",
		Tables:           nil,
	}
}

// ApplyAuditMigrations applies all migrations via goose (DD-012).
// With goose version tracking, all migrations are applied for consistency.
// Non-audit tables are created empty and harmless.
func ApplyAuditMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	return ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
}

// ApplyAllMigrations applies all available migrations using the goose Go library (DD-012).
// It connects to PostgreSQL via kubectl port-forward and runs goose.Up().
func ApplyAllMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}
	migrationsDir := filepath.Join(workspaceRoot, "migrations")

	config := DefaultMigrationConfig(namespace, kubeconfigPath)

	_, _ = fmt.Fprintf(writer, "📋 Applying ALL migrations with goose (DD-012)...\n")
	return applyGooseMigrationsE2E(ctx, config, migrationsDir, writer)
}

// ApplyMigrationsWithConfig applies all migrations via goose (DD-012).
// The config.Tables field is ignored; goose applies all migrations with version tracking.
func ApplyMigrationsWithConfig(ctx context.Context, config MigrationConfig, writer io.Writer) error {
	return ApplyAllMigrations(ctx, config.Namespace, config.KubeconfigPath, writer)
}

// VerifyMigrations checks if required tables exist by querying the goose_db_version table
// and verifying critical tables via a port-forwarded connection.
func VerifyMigrations(ctx context.Context, config MigrationConfig, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔍 Verifying migrations...\n")

	tables := config.Tables
	if len(tables) == 0 {
		tables = []string{
			"audit_events",
			"notification_audit",
			"remediation_workflow_catalog",
		}
	}

	podName, err := waitForPostgresPod(ctx, config, writer)
	if err != nil {
		return err
	}

	pf, err := startPortForward(ctx, config.KubeconfigPath, config.Namespace, podName, writer)
	if err != nil {
		return fmt.Errorf("failed to start port-forward: %w", err)
	}
	defer pf.Close()

	db, err := openPostgresConnection(pf.localPort, config)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer func() { _ = db.Close() }()

	var missingTables []string
	for _, table := range tables {
		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)", table).Scan(&exists)
		if err != nil || !exists {
			missingTables = append(missingTables, table)
			_, _ = fmt.Fprintf(writer, "   ❌ Table %s: NOT FOUND\n", table)
		} else {
			_, _ = fmt.Fprintf(writer, "   ✅ Table %s: EXISTS\n", table)
		}
	}

	if len(missingTables) > 0 {
		return fmt.Errorf("missing tables: %v", missingTables)
	}

	_, _ = fmt.Fprintf(writer, "✅ All tables verified\n")
	return nil
}

// applyGooseMigrationsE2E applies migrations to a PostgreSQL instance inside a Kind cluster
// using kubectl port-forward and the goose Go library.
func applyGooseMigrationsE2E(ctx context.Context, config MigrationConfig, migrationsDir string, writer io.Writer) error {
	podName, err := waitForPostgresPod(ctx, config, writer)
	if err != nil {
		return err
	}

	pf, err := startPortForward(ctx, config.KubeconfigPath, config.Namespace, podName, writer)
	if err != nil {
		return fmt.Errorf("failed to start port-forward: %w", err)
	}
	defer pf.Close()

	db, err := openPostgresConnection(pf.localPort, config)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := RunGooseMigrations(ctx, db, migrationsDir, writer); err != nil {
		return err
	}

	if err := SeedActionTypeTaxonomy(ctx, db, writer); err != nil {
		return err
	}

	grantSQL := `
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`
	if _, err := db.ExecContext(ctx, grantSQL); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to grant permissions (may already exist): %v\n", err)
	}

	return nil
}

// RunGooseMigrations applies all pending migrations in migrationsDir using the goose Go library.
// This is the core migration function shared by E2E, integration, and unit test infrastructure.
// It requires a valid *sql.DB connection and an absolute path to the migrations directory.
func RunGooseMigrations(ctx context.Context, db *sql.DB, migrationsDir string, writer io.Writer) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(migrationsDir))
	if err != nil {
		return fmt.Errorf("failed to create goose provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("goose migration failed: %w", err)
	}

	for _, r := range results {
		_, _ = fmt.Fprintf(writer, "   ✅ Applied %s (%s)\n", r.Source.Path, r.Duration)
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintf(writer, "   ✅ All migrations already applied (no pending)\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ Goose migrations complete (%d applied)\n", len(results))
	}

	return nil
}

// SeedActionTypeTaxonomy populates the action_type_taxonomy table with the
// standard action types required by workflow FK constraints (DD-WORKFLOW-016).
// Uses ON CONFLICT DO NOTHING for idempotency. Call after RunGooseMigrations.
func SeedActionTypeTaxonomy(ctx context.Context, db *sql.DB, writer io.Writer) error {
	const seedSQL = `
		INSERT INTO action_type_taxonomy (action_type, description) VALUES
			('DeletePod', '{"what": "Delete one or more specific pods", "whenToUse": "Pods are stuck in a terminal state"}'),
			('DrainNode', '{"what": "Drain and cordon a Kubernetes node", "whenToUse": "Node-level issue affecting multiple workloads"}'),
			('FixCertificate', '{"what": "Recreate a missing or corrupted CA Secret", "whenToUse": "cert-manager Certificate stuck in NotReady"}'),
			('IncreaseCPULimits', '{"what": "Increase CPU resource limits on containers", "whenToUse": "CPU throttling from low limits"}'),
			('IncreaseMemoryLimits', '{"what": "Increase memory resource limits on containers", "whenToUse": "OOM kills from low limits"}'),
			('RestartDeployment', '{"what": "Rolling restart of all pods in a workload", "whenToUse": "Workload-wide state issue"}'),
			('RestartPod', '{"what": "Kill and recreate one or more pods", "whenToUse": "Transient runtime state issue"}'),
			('RollbackDeployment', '{"what": "Revert a deployment to its previous stable revision", "whenToUse": "Recent deployment regression"}'),
			('ScaleReplicas', '{"what": "Horizontally scale a workload", "whenToUse": "Insufficient capacity to handle current load"}')
		ON CONFLICT (action_type) DO NOTHING
	`
	if _, err := db.ExecContext(ctx, seedSQL); err != nil {
		return fmt.Errorf("failed to seed action_type_taxonomy: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Action type taxonomy seeded (9 types)\n")
	return nil
}

// waitForPostgresPod waits for the PostgreSQL pod to be ready and returns its name.
func waitForPostgresPod(ctx context.Context, config MigrationConfig, writer io.Writer) (string, error) {
	clientset, err := getKubernetesClient(config.KubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to create kubernetes client: %w", err)
	}

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

		pod := pods.Items[0]
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				podName = pod.Name
				return nil
			}
		}
		return fmt.Errorf("PostgreSQL pod not ready yet")
	}, 5*time.Minute, 5*time.Second).Should(Succeed(), "PostgreSQL pod should be ready for migrations")

	_, _ = fmt.Fprintf(writer, "   📦 PostgreSQL pod ready: %s\n", podName)
	return podName, nil
}

// portForward manages a kubectl port-forward subprocess for database connectivity.
type portForward struct {
	cmd       *exec.Cmd
	localPort int
}

// startPortForward creates a kubectl port-forward tunnel to a PostgreSQL pod.
// It allocates a random available local port and waits until the tunnel is ready.
func startPortForward(ctx context.Context, kubeconfigPath, namespace, podName string, writer io.Writer) (*portForward, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	_, _ = fmt.Fprintf(writer, "   🔌 Starting port-forward to %s (localhost:%d → 5432)...\n", podName, port)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"port-forward", "-n", namespace, podName,
		fmt.Sprintf("%d:5432", port))

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start port-forward: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), time.Second)
		if dialErr == nil {
			_ = conn.Close()
			_, _ = fmt.Fprintf(writer, "   ✅ Port-forward ready (localhost:%d)\n", port)
			return &portForward{cmd: cmd, localPort: port}, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil, fmt.Errorf("port-forward to %s not ready after 30 seconds", podName)
}

// Close terminates the port-forward subprocess.
func (pf *portForward) Close() {
	if pf.cmd != nil && pf.cmd.Process != nil {
		_ = pf.cmd.Process.Kill()
		_ = pf.cmd.Wait()
	}
}

// openPostgresConnection opens a database/sql connection to PostgreSQL via a forwarded local port.
func openPostgresConnection(localPort int, config MigrationConfig) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=localhost port=%d user=%s password=%s dbname=%s sslmode=disable",
		localPort, config.PostgresUser, config.PostgresPassword, config.PostgresDB)
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}
	return db, nil
}

// DiscoverMigrations reads all migration files from a directory and returns them sorted by version
// This eliminates the need for manual synchronization between migrations/ directory and test code
//
// Migration files must follow goose naming convention: {version}_{description}.sql
// Examples: 001_initial_schema.sql, 022_add_status_reason_column.sql, 1000_create_audit_events_partitions.sql
//
// Returns:
//   - Sorted list of migration filenames (by version number, not lexicographic)
//   - Error if directory cannot be read
//
// Usage:
//
//	migrations, err := DiscoverMigrations("../../../migrations")
//	if err != nil {
//	    return fmt.Errorf("failed to discover migrations: %w", err)
//	}
//	for _, migration := range migrations {
//	    // Apply migration...
//	}
func DiscoverMigrations(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory %s: %w", migrationsDir, err)
	}

	var migrations []string
	// Goose migration pattern: {version}_{description}.sql
	// Version is 1+ digits, description is lowercase alphanumeric with underscores
	// Examples: 001_initial_schema.sql, 022_add_status_reason_column.sql
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories (e.g., testdata/)
		}

		name := entry.Name()
		// Check if file matches goose migration pattern
		if isValidMigrationFile(name) {
			migrations = append(migrations, name)
		}
	}

	if len(migrations) == 0 {
		return nil, fmt.Errorf("no migration files found in %s", migrationsDir)
	}

	// Sort by version number (numeric sort, not lexicographic)
	// This ensures 001, 002, ..., 022, 1000 are in correct order
	sort.Slice(migrations, func(i, j int) bool {
		versionI := extractMigrationVersion(migrations[i])
		versionJ := extractMigrationVersion(migrations[j])
		return versionI < versionJ
	})

	return migrations, nil
}

// isValidMigrationFile checks if a filename matches the goose migration pattern
// Valid: 001_initial_schema.sql, 022_add_status_reason_column.sql
// Invalid: seed_test_data.sql, README.md, testdata/
func isValidMigrationFile(filename string) bool {
	// Must end with .sql
	if !strings.HasSuffix(filename, ".sql") {
		return false
	}

	// Must start with digit(s) followed by underscore
	// Regex: ^\d+_[a-z0-9_]+\.sql$
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return false // No underscore separator
	}

	// First part must be all digits (version)
	version := parts[0]
	if len(version) == 0 {
		return false
	}
	for _, ch := range version {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	return true
}

// extractMigrationVersion extracts the numeric version from a migration filename
// Examples:
//   - "001_initial_schema.sql" → 1
//   - "022_add_status_reason_column.sql" → 22
//   - "1000_create_audit_events_partitions.sql" → 1000
func extractMigrationVersion(filename string) int {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0
	}

	version := 0
	_, _ = fmt.Sscanf(parts[0], "%d", &version) // Ignore errors - invalid versions return 0
	return version
}
