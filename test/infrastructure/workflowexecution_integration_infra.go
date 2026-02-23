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

package infrastructure

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// WorkflowExecution Integration Infrastructure - DD-TEST-002 Go-Based Pattern
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Migration: Dec 25, 2025
// - FROM: Shell script (test/integration/workflowexecution/setup-infrastructure.sh)
// - TO: Go-based DD-TEST-002 pattern (aligns with Gateway, AIAnalysis, SignalProcessing, RO)
//
// Pattern: DD-TEST-002 Sequential Startup Pattern
// - Sequential container startup (eliminates race conditions)
// - Explicit health checks after each service
// - No podman-compose (avoids parallel startup issues)
// - Parallel-safe with unique ports (DD-TEST-001)
//
// Infrastructure Components:
// - PostgreSQL (port 15441): DataStorage backend
// - Redis (port 16388): DataStorage cache
// - DataStorage (port 18097): Audit event persistence
// - Metrics (port 19097): DataStorage metrics endpoint
//
// Related:
// - gateway.go: Reference implementation of DD-TEST-002
// - datastorage_bootstrap.go: Shared DataStorage bootstrap utilities
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// WorkflowExecution Integration Test Ports (per DD-TEST-001 v1.7 - December 2025)
// Sequential allocation after AIAnalysis (15438/16384/18095/19095)
const (
	// PostgreSQL port for WorkflowExecution integration tests
	// Changed from 15443 (ad-hoc "+10") to 15441 (DD-TEST-001 sequential) on Dec 22, 2025
	WEIntegrationPostgresPort = 15441

	// Redis port for WorkflowExecution integration tests
	// Changed from 16389 (ad-hoc "+10") to 16387 (DD-TEST-001 sequential) on Dec 22, 2025
	// Changed from 16387 (conflicted with HAPI) to 16388 (unique) on Dec 25, 2025 per DD-TEST-001 v1.9
	WEIntegrationRedisPort = 16388

	// DataStorage HTTP API port for WorkflowExecution integration tests
	// Changed from 18100 (conflicted with EffectivenessMonitor) to 18097 (DD-TEST-001 sequential) on Dec 22, 2025
	WEIntegrationDataStoragePort = 18097

	// DataStorage Metrics port for WorkflowExecution integration tests
	// Changed from 19100 (ad-hoc "+10") to 19097 (DD-TEST-001 metrics pattern) on Dec 22, 2025
	WEIntegrationMetricsPort = 19097
)

// WorkflowExecution Integration Test Container Names
const (
	// PostgreSQL container name
	WEIntegrationPostgresContainer = "workflowexecution_postgres_1"

	// Redis container name
	WEIntegrationRedisContainer = "workflowexecution_redis_1"

	// DataStorage container name
	WEIntegrationDataStorageContainer = "workflowexecution_datastorage_1"

	// Migrations container name
	WEIntegrationMigrationsContainer = "workflowexecution_migrations"
)

// WorkflowExecution Integration Test Database Configuration
const (
	// Database name for audit events
	WEIntegrationDBName = "action_history"

	// Database user
	WEIntegrationDBUser = "slm_user"

	// Database password
	WEIntegrationDBPassword = "test_password"
)

// StartWEIntegrationInfrastructure starts the WorkflowExecution integration test infrastructure
// using sequential podman run commands per DD-TEST-002.
//
// Pattern: DD-TEST-002 Sequential Startup Pattern
// - Sequential container startup (eliminates race conditions)
// - Explicit health checks after each service
// - No podman-compose (avoids parallel startup issues)
// - Parallel-safe with unique ports (DD-TEST-001)
//
// Infrastructure Components:
// - PostgreSQL (port 15441): DataStorage backend
// - Redis (port 16387): DataStorage cache
// - DataStorage (port 18097): Audit event persistence
// - Metrics (port 19097): DataStorage metrics endpoint
//
// Returns error if any infrastructure component fails to start.
func StartWEIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()

	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "WorkflowExecution Integration Test Infrastructure Setup\n")
	_, _ = fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", WEIntegrationPostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:          localhost:%d (DD-TEST-001 v1.9: unique port)\n", WEIntegrationRedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", WEIntegrationDataStoragePort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// ============================================================================
	// STEP 1: Cleanup existing containers (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
	CleanupContainers([]string{
		WEIntegrationDataStorageContainer,
		WEIntegrationRedisContainer,
		WEIntegrationPostgresContainer,
	}, writer)
	_, _ = fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// ============================================================================
	// STEP 2: Network setup (SKIPPED - using port mapping for localhost connectivity)
	// ============================================================================
	// Note: Using port mapping (-p) instead of custom podman network to avoid DNS resolution issues
	// All services connect via localhost:PORT (same pattern as Gateway)
	_, _ = fmt.Fprintf(writer, "🌐 Network: Using port mapping for localhost connectivity\n\n")

	// ============================================================================
	// STEP 3: Start PostgreSQL FIRST (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
	if err := StartPostgreSQL(PostgreSQLConfig{
		ContainerName: WEIntegrationPostgresContainer,
		Port:          WEIntegrationPostgresPort,
		DBName:        WEIntegrationDBName,
		DBUser:        WEIntegrationDBUser,
		DBPassword:    WEIntegrationDBPassword,
	}, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// CRITICAL: Wait for PostgreSQL to be ready before proceeding (using shared utility)
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
	if err := WaitForPostgreSQLReady(WEIntegrationPostgresContainer, WEIntegrationDBUser, WEIntegrationDBName, writer); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n\n")

	// ============================================================================
	// STEP 4: Run migrations
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🔄 Running database migrations...\n")
	if err := runWEMigrations(projectRoot, writer); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// ============================================================================
	// STEP 5: Start Redis SECOND (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🔴 Starting Redis...\n")
	if err := StartRedis(RedisConfig{
		ContainerName: WEIntegrationRedisContainer,
		Port:          WEIntegrationRedisPort,
	}, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// Wait for Redis to be ready (using shared utility)
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := WaitForRedisReady(WEIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("redis failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Redis ready\n\n")

	// ============================================================================
	// STEP 6: Start DataStorage LAST
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
	if err := startWEDataStorage(projectRoot, writer); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// CRITICAL: Wait for DataStorage HTTP endpoint to be ready (using shared utility)
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://127.0.0.1:%d/health", WEIntegrationDataStoragePort),
		30*time.Second,
		writer,
	); err != nil {
		// Print container logs for debugging
		_, _ = fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", WEIntegrationDataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage ready\n\n")

	// ============================================================================
	// SUCCESS
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "✅ WorkflowExecution Integration Infrastructure Ready\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", WEIntegrationPostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:             localhost:%d (DD-TEST-001 v1.9: unique port)\n", WEIntegrationRedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage HTTP:  http://localhost:%d\n", WEIntegrationDataStoragePort)
	_, _ = fmt.Fprintf(writer, "  DataStorage Metrics: http://localhost:%d\n", WEIntegrationMetricsPort)
	_, _ = fmt.Fprintf(writer, "  Database:          %s\n", WEIntegrationDBName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopWEIntegrationInfrastructure stops all WorkflowExecution integration infrastructure containers.
func StopWEIntegrationInfrastructure(writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🛑 Stopping WorkflowExecution integration infrastructure...\n")

	// Stop and remove containers (using shared utility)
	CleanupContainers([]string{
		WEIntegrationDataStorageContainer,
		WEIntegrationRedisContainer,
		WEIntegrationPostgresContainer,
	}, writer)

	_, _ = fmt.Fprintf(writer, "   ✅ All containers stopped\n")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Internal Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Service-specific helper functions (common functions moved to shared_integration_utils.go)

func runWEMigrations(projectRoot string, writer io.Writer) error {
	// Apply migrations using custom script (same pattern as Gateway)
	// Per DD-SCHEMA-001: Data Storage team owns migrations, but test infrastructure must apply them
	migrationsDir := filepath.Join(projectRoot, "migrations")

	// Apply migrations: extract only "Up" sections (stop at "-- +goose Down")
	migrationScript := `
		set -e
		echo "Applying migrations (Up sections only)..."
		find /migrations -maxdepth 1 -name "*.sql" -type f | sort | while read f; do
			echo "Applying $f..."
			sed -n "1,/^-- +goose Down/p" "$f" | grep -v "^-- +goose Down" | psql
		done
		echo "Migrations complete!"
	`

	// Use host.containers.internal for macOS compatibility (Podman VM can reach host)
	cmd := exec.Command("podman", "run", "--rm",
		"--name", WEIntegrationMigrationsContainer,
		"-v", fmt.Sprintf("%s:/migrations:ro", migrationsDir),
		"-e", "PGHOST=host.containers.internal",
		"-e", fmt.Sprintf("PGPORT=%d", WEIntegrationPostgresPort),
		"-e", "PGUSER="+WEIntegrationDBUser,
		"-e", "PGPASSWORD="+WEIntegrationDBPassword,
		"-e", "PGDATABASE="+WEIntegrationDBName,
		"postgres:16-alpine",
		"bash", "-c", migrationScript,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func startWEDataStorage(projectRoot string, writer io.Writer) error {
	// DD-TEST-001 v1.3: Use infrastructure image format for parallel test isolation
	// Format: localhost/{infrastructure}:{consumer}-{uuid}
	// Example: localhost/datastorage:workflowexecution-1884d074
	dsImage := GenerateInfraImageName("datastorage", "workflowexecution")

	_, _ = fmt.Fprintf(writer, "   Resolving DataStorage image (%s)...\n", dsImage)
	// Use shared build function (includes --no-cache, coverage support, and registry optimization)
	actualImage, err := buildDataStorageImageWithTag(dsImage, writer)
	if err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage image ready: %s\n", actualImage)

	// Mount config directory and set CONFIG_PATH (per ADR-030)
	configDir := filepath.Join(projectRoot, "test", "integration", "workflowexecution", "config")

	// DataStorage connects to PostgreSQL and Redis via host.containers.internal
	// This allows containers to reach services on the host via port mapping
	cmd := exec.Command("podman", "run",
		"-d",
		"--name", WEIntegrationDataStorageContainer,
		"-p", fmt.Sprintf("%d:8080", WEIntegrationDataStoragePort),
		"-p", fmt.Sprintf("%d:9090", WEIntegrationMetricsPort),
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"-e", "LOG_LEVEL=debug",
		"-e", "PORT=8080",
		"-e", "METRICS_PORT=9090",
		"-e", "DB_HOST=host.containers.internal",
		"-e", "DB_PORT="+fmt.Sprintf("%d", WEIntegrationPostgresPort),
		"-e", "DB_NAME="+WEIntegrationDBName,
		"-e", "DB_USER="+WEIntegrationDBUser,
		"-e", "DB_PASSWORD="+WEIntegrationDBPassword,
		"-e", "DB_SSLMODE=disable",
		"-e", "REDIS_HOST=host.containers.internal",
		"-e", "REDIS_PORT="+fmt.Sprintf("%d", WEIntegrationRedisPort),
		"-e", "REDIS_DB=0",
		actualImage, // Use resolved image (registry or local build)
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
