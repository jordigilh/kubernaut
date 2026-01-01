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
// This file implements the RemediationOrchestrator E2E infrastructure.
// Uses the shared migration library per DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md
//
// RO Audit Events Emitted:
//   - orchestrator.lifecycle.started
//   - orchestrator.phase.transitioned
//   - orchestrator.remediation.completed
//   - orchestrator.remediation.failed
//
// All audit events require the audit_events table (DD-AUDIT-003).
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
)

// RemediationOrchestrator E2E container names
const (
	ROIntegrationPostgresContainer    = "ro-e2e-postgres"
	ROIntegrationRedisContainer       = "ro-e2e-redis"
	ROIntegrationDataStorageContainer = "ro-e2e-datastorage"
	ROIntegrationNetwork              = "ro-e2e-network"
)

// ROClusterConfig holds configuration for RO E2E cluster setup
type ROClusterConfig struct {
	ClusterName    string
	KubeconfigPath string
	Namespace      string
	Writer         io.Writer
}

// DefaultROClusterConfig returns default configuration for RO E2E tests
func DefaultROClusterConfig() ROClusterConfig {
	homeDir, _ := os.UserHomeDir()
	return ROClusterConfig{
		ClusterName:    "ro-e2e",
		KubeconfigPath: filepath.Join(homeDir, ".kube", "ro-e2e-config"),
		Namespace:      "kubernaut-system",
	}
}

// CreateROCluster creates a Kind cluster for RemediationOrchestrator E2E testing.
// This is called ONCE in SynchronizedBeforeSuite (first parallel process only).
//
// Steps:
// 1. Create Kind cluster with production-like configuration
// 2. Export kubeconfig to ~/.kube/ro-e2e-config
// 3. Install ALL CRDs required for RO orchestration
// 4. Deploy PostgreSQL for audit storage (DD-AUDIT-003)
// 5. Apply audit migrations using shared library
// 6. Deploy Data Storage service
// 7. Deploy RO controller and dependent controllers
//
// Time: ~2-3 minutes (full stack deployment)
func CreateROCluster(ctx context.Context, config ROClusterConfig) error {
	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "RemediationOrchestrator E2E Cluster Setup")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Check if cluster already exists
	if roClusterExists(config.ClusterName) {
		fmt.Fprintln(writer, "♻️  Reusing existing cluster")
		if err := roExportKubeconfig(config.ClusterName, config.KubeconfigPath, writer); err != nil {
			return fmt.Errorf("failed to export kubeconfig: %w", err)
		}
	} else {
		// Create Kind cluster
		fmt.Fprintln(writer, "📦 Creating Kind cluster...")
		if err := createROKindCluster(config.ClusterName, config.KubeconfigPath, writer); err != nil {
			return fmt.Errorf("failed to create Kind cluster: %w", err)
		}
	}

	// 2. Set KUBECONFIG environment variable
	if err := os.Setenv("KUBECONFIG", config.KubeconfigPath); err != nil {
		return fmt.Errorf("failed to set KUBECONFIG: %w", err)
	}
	fmt.Fprintf(writer, "📂 KUBECONFIG=%s\n", config.KubeconfigPath)

	// 3. Install ALL CRDs required for RO orchestration
	fmt.Fprintln(writer, "📋 Installing CRDs...")
	if err := installROCRDs(config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}

	// 4. Create namespace
	fmt.Fprintf(writer, "📁 Creating namespace: %s\n", config.Namespace)
	if err := roCreateNamespace(config.KubeconfigPath, config.Namespace, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 5. Deploy PostgreSQL for audit storage
	fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
	if err := deployROPostgreSQL(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 6. Apply audit migrations using shared library (DD-AUDIT-003)
	fmt.Fprintln(writer, "🔄 Applying audit migrations (shared library)...")
	if err := ApplyAuditMigrations(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// 7. Verify migrations applied correctly
	fmt.Fprintln(writer, "✅ Verifying migrations...")
	migConfig := DefaultMigrationConfig(config.Namespace, config.KubeconfigPath)
	migConfig.Tables = []string{"audit_events"}
	if err := VerifyMigrations(ctx, migConfig, writer); err != nil {
		return fmt.Errorf("migration verification failed: %w", err)
	}

	// 8. Deploy Data Storage service
	fmt.Fprintln(writer, "📦 Deploying Data Storage service...")
	if err := deployDataStorageForRO(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "✅ RemediationOrchestrator E2E cluster ready!")
	fmt.Fprintf(writer, "   Cluster: %s\n", config.ClusterName)
	fmt.Fprintf(writer, "   Kubeconfig: %s\n", config.KubeconfigPath)
	fmt.Fprintf(writer, "   Namespace: %s\n", config.Namespace)
	fmt.Fprintln(writer, "   Audit: audit_events table + partitions + indexes")
	fmt.Fprintln(writer, "")

	return nil
}

// DeleteROCluster deletes the RO E2E Kind cluster
func DeleteROCluster(config ROClusterConfig, preserveOnFailure bool) error {
	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	if preserveOnFailure {
		fmt.Fprintln(writer, "⚠️  PRESERVE_E2E_CLUSTER=true, keeping cluster for debugging")
		fmt.Fprintf(writer, "   To access: export KUBECONFIG=%s\n", config.KubeconfigPath)
		fmt.Fprintf(writer, "   To delete: kind delete cluster --name %s\n", config.ClusterName)
		return nil
	}

	fmt.Fprintln(writer, "🗑️  Deleting Kind cluster...")
	cmd := exec.Command("kind", "delete", "cluster", "--name", config.ClusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Failed to delete cluster (may not exist): %v\n", err)
	}

	// Remove kubeconfig file
	if config.KubeconfigPath != "" {
		defaultConfig := os.ExpandEnv("$HOME/.kube/config")
		if config.KubeconfigPath != defaultConfig {
			_ = os.Remove(config.KubeconfigPath)
			fmt.Fprintf(writer, "🗑️  Removed kubeconfig: %s\n", config.KubeconfigPath)
		}
	}

	return nil
}

// ============================================================================
// Internal Helper Functions (prefixed with ro to avoid conflicts)
// ============================================================================

func roClusterExists(name string) bool {
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range roSplitLines(string(output)) {
		if line == name {
			return true
		}
	}
	return false
}

func roSplitLines(s string) []string {
	var lines []string
	var current string
	for _, c := range s {
		if c == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func createROKindCluster(name, kubeconfig string, writer io.Writer) error {
	cmd := exec.Command("kind", "create", "cluster",
		"--name", name,
		"--kubeconfig", kubeconfig,
		"--wait", "120s",
	)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}
	return nil
}

func roExportKubeconfig(name, kubeconfig string, writer io.Writer) error {
	cmd := exec.Command("kind", "export", "kubeconfig",
		"--name", name,
		"--kubeconfig", kubeconfig,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func installROCRDs(kubeconfig string, writer io.Writer) error {
	// CRDs required for RO orchestration
	// Updated to kubernaut.ai API group (Dec 25, 2025)
	crdPaths := []string{
		"config/crd/bases/kubernaut.ai_remediationrequests.yaml",
		"config/crd/bases/kubernaut.ai_remediationapprovalrequests.yaml",
		"config/crd/bases/kubernaut.ai_signalprocessings.yaml",
		"config/crd/bases/kubernaut.ai_aianalyses.yaml",
		"config/crd/bases/kubernaut.ai_workflowexecutions.yaml",
		"config/crd/bases/kubernaut.ai_notificationrequests.yaml",
	}

	for _, crdPath := range crdPaths {
		fullPath := roFindProjectFile(crdPath)
		if fullPath == "" {
			fmt.Fprintf(writer, "⚠️  CRD not found: %s\n", crdPath)
			continue
		}

		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig,
			"apply", "-f", fullPath)
		cmd.Stdout = writer
		cmd.Stderr = writer

		if err := cmd.Run(); err != nil {
			fmt.Fprintf(writer, "⚠️  Failed to install CRD %s: %v\n", crdPath, err)
		}
	}
	return nil
}

func roFindProjectFile(relativePath string) string {
	paths := []string{
		relativePath,
		"../../../" + relativePath,
		"../../" + relativePath,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func roCreateNamespace(kubeconfig, namespace string, writer io.Writer) error {
	// Check if namespace already exists (idempotent operation)
	checkCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig,
		"get", "namespace", namespace)
	if err := checkCmd.Run(); err == nil {
		// Namespace already exists, nothing to do
		fmt.Fprintf(writer, "   ♻️  Namespace %s already exists (reusing)\n", namespace)
		return nil
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig,
		"create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	yaml, err := cmd.Output()
	if err != nil {
		return err
	}

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	applyCmd.Stdin = roBytesReader(yaml)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer
	if err := applyCmd.Run(); err != nil {
		// Check if error is "AlreadyExists" - if so, ignore (race condition from parallel processes)
		if errOutput := err.Error(); len(errOutput) > 0 {
			if strings.Contains(errOutput, "already exists") {
				fmt.Fprintf(writer, "   ♻️  Namespace %s already exists (created by parallel process)\n", namespace)
				return nil
			}
		}
		return err
	}
	return nil
}

func roBytesReader(b []byte) *roBytesReaderImpl {
	return &roBytesReaderImpl{data: b}
}

type roBytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *roBytesReaderImpl) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func deployROPostgreSQL(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
	fmt.Fprintln(writer, "   Checking for existing PostgreSQL...")

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
		"get", "pod", "-l", "app=postgresql", "-o", "name")
	output, _ := cmd.Output()
	if len(output) > 0 {
		fmt.Fprintln(writer, "   ♻️  Reusing existing PostgreSQL")
		return waitForROPostgreSQL(ctx, namespace, kubeconfig, writer)
	}

	fmt.Fprintln(writer, "   Deploying PostgreSQL...")
	return createMinimalROPostgreSQL(namespace, kubeconfig, writer)
}

func createMinimalROPostgreSQL(namespace, kubeconfig string, writer io.Writer) error {
	manifest := `
apiVersion: v1
kind: Service
metadata:
  name: postgresql
spec:
  ports:
    - port: 5432
  selector:
    app: postgresql
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgresql
spec:
  serviceName: postgresql
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
        - name: postgresql
          image: postgres:16-alpine
          ports:
            - containerPort: 5432
          env:
            - name: POSTGRES_USER
              value: slm_user
            - name: POSTGRES_PASSWORD
              value: slm_password
            - name: POSTGRES_DB
              value: action_history
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "slm_user", "-d", "action_history"]
            initialDelaySeconds: 5
            periodSeconds: 5
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace, "apply", "-f", "-")
	cmd.Stdin = roBytesReader([]byte(manifest))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL: %w", err)
	}

	return waitForROPostgreSQL(context.Background(), namespace, kubeconfig, writer)
}

func waitForROPostgreSQL(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
	fmt.Fprintln(writer, "   Waiting for PostgreSQL to be ready...")

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
			"wait", "--for=condition=ready", "pod", "-l", "app=postgresql", "--timeout=10s")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "   ✅ PostgreSQL ready")
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("PostgreSQL not ready within timeout")
}

func deployDataStorageForRO(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
	manifestPath := roFindProjectFile("deploy/datastorage/deployment.yaml")
	if manifestPath == "" {
		fmt.Fprintln(writer, "   ⚠️  Data Storage manifest not found, skipping (audit will fail)")
		return nil
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
		"apply", "-f", manifestPath)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "   ⚠️  Failed to deploy Data Storage: %v\n", err)
		return nil
	}

	fmt.Fprintln(writer, "   Waiting for Data Storage to be ready...")
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
			"wait", "--for=condition=ready", "pod", "-l", "app=datastorage", "--timeout=10s")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "   ✅ Data Storage ready")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	fmt.Fprintln(writer, "   ⚠️  Data Storage not ready within timeout (audit may fail)")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// RemediationOrchestrator Integration Test Infrastructure (Podman Compose)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Pattern: AIAnalysis Pattern (Programmatic podman-compose management)
// Authority: docs/handoff/TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md
//
// Port Allocation (per DD-TEST-001):
//   PostgreSQL:       15435 → 5432 (RO-specific, from range 15433-15442)
//   Redis:            16381 → 6379 (RO-specific, from range 16379-16388)
//   Data Storage API: 18140 → 8080 (RO-specific, after stateless services)
//   DS Metrics:       18141 → 9090
//
// Dependencies:
//   RO → Data Storage (audit events, workflow catalog)
//   Data Storage → PostgreSQL (persistence)
//   Data Storage → Redis (caching/DLQ)
//
// Parallel Execution:
//   - Uses SynchronizedBeforeSuite for parallel-safe setup
//   - Process 1 starts infrastructure ONCE
//   - ALL processes share the same infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// RO Integration Test Ports (per DD-TEST-001)
	ROIntegrationPostgresPort           = 15435
	ROIntegrationRedisPort              = 16381
	ROIntegrationDataStoragePort        = 18140
	ROIntegrationDataStorageMetricsPort = 18141

	// Compose project name
	ROIntegrationComposeProject = "remediationorchestrator-integration"

	// Compose file path relative to project root
	ROIntegrationComposeFile = "test/integration/remediationorchestrator/podman-compose.remediationorchestrator.test.yml"
)

// StartROIntegrationInfrastructure starts the full podman-compose stack for RO integration tests
// This includes: PostgreSQL, Redis, and Data Storage API
//
// Pattern: AIAnalysis Pattern (per TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md)
// - Programmatic podman-compose management
// - Health checks via HTTP endpoints
// - Parallel-safe (called from SynchronizedBeforeSuite)
//
// Prerequisites:
// - podman-compose must be installed
// - Ports 15435, 16381, 18140, 18141 must be available
//
// Returns:
// - error: Any errors during infrastructure startup
func StartROIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting RO Integration Test Infrastructure (DataStorage Team Pattern)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", ROIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", ROIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", ROIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  DS Metrics:     http://localhost:%d\n", ROIntegrationDataStorageMetricsPort)
	fmt.Fprintf(writer, "  Pattern:        DD-TEST-002 Sequential Startup (DataStorage Team Implementation)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	projectRoot := getProjectRoot()

	// Step 1: Cleanup existing containers (using shared utility)
	fmt.Fprintf(writer, "🧹 Cleaning up any existing containers...\n")
	CleanupContainers([]string{
		ROIntegrationPostgresContainer,
		ROIntegrationRedisContainer,
		ROIntegrationDataStorageContainer,
	}, writer)

	// Step 2: Network strategy
	// Note: Using port mapping (-p) instead of custom podman network to avoid DNS resolution issues
	// All services connect via host.containers.internal:PORT (same pattern as Gateway/Notification/WE)
	fmt.Fprintf(writer, "🌐 Network: Using port mapping for localhost connectivity\n\n")

	// Step 3: Start PostgreSQL FIRST (using shared utility)
	fmt.Fprintf(writer, "🔵 Starting PostgreSQL...\n")
	if err := StartPostgreSQL(PostgreSQLConfig{
		ContainerName:  ROIntegrationPostgresContainer,
		Port:           ROIntegrationPostgresPort,
		DBName:         "action_history",
		DBUser:         "slm_user",
		DBPassword:     "test_password",
		MaxConnections: 200,
	}, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// Step 4: WAIT for PostgreSQL to be ready (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
	if err := WaitForPostgreSQLReady(ROIntegrationPostgresContainer, "slm_user", "action_history", writer); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}

	// Step 4b: VERIFY database is actually queryable (not just accepting connections)
	// Per DD-TEST-002: pg_isready only checks connections, not full DB initialization
	fmt.Fprintf(writer, "⏳ Verifying database is queryable...\n")
	maxAttempts := 10
	for i := 1; i <= maxAttempts; i++ {
		testQueryCmd := exec.Command("podman", "exec", ROIntegrationPostgresContainer,
			"psql", "-U", "slm_user", "-d", "action_history", "-c", "SELECT 1;")
		testQueryCmd.Stdout = writer
		testQueryCmd.Stderr = writer
		if testQueryCmd.Run() == nil {
			fmt.Fprintf(writer, "   ✅ Database queryable (attempt %d/%d)\n", i, maxAttempts)
			break
		}
		if i < maxAttempts {
			fmt.Fprintf(writer, "   ⏳ Database not yet queryable, retrying... (attempt %d/%d)\n", i, maxAttempts)
			time.Sleep(1 * time.Second)
		} else {
			return fmt.Errorf("database failed to become queryable after %d attempts", maxAttempts)
		}
	}

	// Step 5: Run migrations (using host.containers.internal for port mapping)
	fmt.Fprintf(writer, "🔄 Running migrations...\n")
	migrationsCmd := exec.Command("podman", "run", "--rm",
		"-e", "PGHOST=host.containers.internal", // Use host.containers.internal for port-mapped PostgreSQL
		"-e", fmt.Sprintf("PGPORT=%d", ROIntegrationPostgresPort),
		"-e", "PGUSER=slm_user",
		"-e", "PGPASSWORD=test_password",
		"-e", "PGDATABASE=action_history",
		"-v", filepath.Join(projectRoot, "migrations")+":/migrations:ro",
		"postgres:16-alpine",
		"sh", "-c",
		`set -e
echo "Applying migrations (Up sections only)..."
find /migrations -maxdepth 1 -name '*.sql' -type f | sort | while read f; do
  echo "Applying $f..."
  sed -n '1,/^-- +goose Down/p' "$f" | grep -v '^-- +goose Down' | psql 2>&1
done
echo "Migrations complete!"`)
	migrationsCmd.Stdout = writer
	migrationsCmd.Stderr = writer
	if err := migrationsCmd.Run(); err != nil {
		// Include a hint about checking the test output above for actual migration errors
		fmt.Fprintf(writer, "\n❌ Migration command failed - check output above for specific SQL errors\n")
		return fmt.Errorf("migrations failed (check test output for details): %w", err)
	}
	fmt.Fprintf(writer, "✅ Migrations complete\n")

	// Step 6: Start Redis SECOND (using shared utility)
	fmt.Fprintf(writer, "🔵 Starting Redis...\n")
	if err := StartRedis(RedisConfig{
		ContainerName: ROIntegrationRedisContainer,
		Port:          ROIntegrationRedisPort,
	}, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// Step 7: WAIT for Redis to be ready (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := WaitForRedisReady(ROIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("Redis failed to become ready: %w", err)
	}

	// Step 7b: Get Redis IP (workaround for Podman DNS issues on macOS)
	fmt.Fprintf(writer, "🔍 Getting Redis IP for DataStorage config...\n")
	redisIPCmd := exec.Command("podman", "inspect", ROIntegrationRedisContainer,
		"--format", `{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}`)
	redisIPOutput, err := redisIPCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get Redis container IP: %w", err)
	}
	redisHost := strings.TrimSpace(string(redisIPOutput))
	if redisHost == "" {
		return fmt.Errorf("failed to get Redis container IP: empty result")
	}
	fmt.Fprintf(writer, "   Redis IP: %s\n", redisHost)

	// Step 8: Create DataStorage config files (DataStorage team pattern)
	fmt.Fprintf(writer, "📝 Creating DataStorage config files...\n")
	configDir := filepath.Join(projectRoot, "test", "integration", "remediationorchestrator", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config.yaml (matching DataStorage team's pattern exactly)
	configYAML := fmt.Sprintf(`service:
  name: data-storage
  metricsPort: 9090
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: %s
  port: 5432
  name: action_history
  user: slm_user
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: %s:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
`, pgHost, redisHost)

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0666); err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	// Create db-secrets.yaml
	dbSecretsYAML := `username: slm_user
password: test_password
`
	dbSecretsPath := filepath.Join(configDir, "db-secrets.yaml")
	if err := os.WriteFile(dbSecretsPath, []byte(dbSecretsYAML), 0666); err != nil {
		return fmt.Errorf("failed to write db-secrets.yaml: %w", err)
	}

	// Create redis-secrets.yaml
	redisSecretsYAML := `password: ""
`
	redisSecretsPath := filepath.Join(configDir, "redis-secrets.yaml")
	if err := os.WriteFile(redisSecretsPath, []byte(redisSecretsYAML), 0666); err != nil {
		return fmt.Errorf("failed to write redis-secrets.yaml: %w", err)
	}

	fmt.Fprintf(writer, "✅ Config files created\n")

	// Step 9: Build DataStorage image (DataStorage team pattern)
	fmt.Fprintf(writer, "🏗️  Building DataStorage image...\n")
	exec.Command("podman", "rmi", "-f", "data-storage:ro-integration-test").Run()
	buildCmd := exec.Command("podman", "build",
		"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes

		"--build-arg", "GOARCH=arm64", // Match DataStorage team's build
		"-t", "data-storage:ro-integration-test",
		"-f", filepath.Join(projectRoot, "docker", "data-storage.Dockerfile"),
		projectRoot)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}

	// Step 10: Start DataStorage LAST (DD-TEST-002 Sequential Pattern)
	fmt.Fprintf(writer, "🔵 Starting DataStorage...\n")
	// Mount config directory (DataStorage team pattern - ro, no :Z flag)
	configMount := fmt.Sprintf("%s:/etc/datastorage:ro", configDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", configDir)

	datastorageCmd := exec.Command("podman", "run", "-d",
		"--name", ROIntegrationDataStorageContainer,
		"-p", fmt.Sprintf("%d:8080", ROIntegrationDataStoragePort),
		"-p", fmt.Sprintf("%d:9090", ROIntegrationDataStorageMetricsPort),
		"-v", configMount,
		"-v", secretsMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"data-storage:ro-integration-test")
	datastorageCmd.Stdout = writer
	datastorageCmd.Stderr = writer
	if err := datastorageCmd.Run(); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// Step 11: WAIT for DataStorage health check (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for DataStorage to be healthy (may take up to 60s for startup)...\n")
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://127.0.0.1:%d/health", ROIntegrationDataStoragePort),
		60*time.Second,
		writer,
	); err != nil {
		// Print container logs for debugging (DataStorage team pattern)
		fmt.Fprintf(writer, "\n📋 DataStorage container logs (last 100 lines):\n")
		logs, _ := exec.Command("podman", "logs", "--tail", "100", ROIntegrationDataStorageContainer).CombinedOutput()
		fmt.Fprintf(writer, "%s\n", logs)
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "✅ DataStorage is healthy\n")

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ RO Integration Infrastructure Ready (DataStorage Team Pattern)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopROIntegrationInfrastructure stops and cleans up the RO integration test infrastructure
//
// Pattern: AIAnalysis Pattern (per TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md)
// - Programmatic podman-compose cleanup
// - Removes volumes (-v flag)
// - Parallel-safe (called from SynchronizedAfterSuite)
//
// Returns:
// - error: Any errors during infrastructure cleanup
func StopROIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "🛑 Stopping RO Integration Infrastructure (DD-TEST-002)...\n")

	// Stop and remove containers (using shared utility)
	CleanupContainers([]string{
		ROIntegrationDataStorageContainer,
		ROIntegrationRedisContainer,
		ROIntegrationPostgresContainer,
	}, writer)

	// Remove network (ignore errors)
	networkCmd := exec.Command("podman", "network", "rm", ROIntegrationNetwork)
	_ = networkCmd.Run()

	fmt.Fprintf(writer, "✅ RO Integration Infrastructure stopped and cleaned up\n")
	return nil
}
