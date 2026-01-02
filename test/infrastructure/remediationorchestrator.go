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

// Package infrastructure provides shared test infrastructure for all services.
//
// This file implements the RemediationOrchestrator integration test infrastructure.
// Uses envtest for Kubernetes API + Podman for dependencies (PostgreSQL, Redis, DataStorage).
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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RemediationOrchestrator integration test container names (DD-TEST-001)
const (
	ROIntegrationPostgresContainer    = "ro-integration-postgres"
	ROIntegrationRedisContainer       = "ro-integration-redis"
	ROIntegrationDataStorageContainer = "ro-integration-datastorage"
	ROIntegrationNetwork              = "ro-integration-network"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// RemediationOrchestrator Integration Test Infrastructure (Podman + envtest)
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
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Integration Test Infrastructure (envtest + Podman)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// NOTE: The following functions are for integration tests (test/integration/), NOT E2E tests.
// Integration tests use envtest for K8s API + Podman for dependencies (PostgreSQL, Redis, DataStorage).
// E2E tests use Kind clusters (see E2E Test Infrastructure section above).
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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

	// Step 8: Create DataStorage config files (DataStorage team pattern)
	fmt.Fprintf(writer, "📝 Creating DataStorage config files...\n")
	configDir := filepath.Join(projectRoot, "test", "integration", "remediationorchestrator", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config.yaml (using host.containers.internal for port-mapped containers)
	// Note: DataStorage container connects to PostgreSQL/Redis via host.containers.internal
	// because they are port-mapped to the host, not on a custom network
	configYAML := `service:
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
  host: host.containers.internal
  port: ` + fmt.Sprintf("%d", ROIntegrationPostgresPort) + `
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
  addr: host.containers.internal:` + fmt.Sprintf("%d", ROIntegrationRedisPort) + `
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
`

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

	// Step 9: Build DataStorage image (using shared utility with GenerateInfraImageName)
	dsImageTag := GenerateInfraImageName("datastorage", "remediationorchestrator")
	fmt.Fprintf(writer, "🏗️  Building DataStorage image (%s)...\n", dsImageTag)
	if err := buildDataStorageImageWithTag(dsImageTag, writer); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ DataStorage image built\n")

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
		dsImageTag)
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
