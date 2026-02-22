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
// Notification Integration Test Infrastructure Constants
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Per DD-TEST-001 v1.7 (December 2025): Port Allocation Strategy
// Per DD-TEST-002: Integration Test Container Orchestration Pattern
//
// Notification integration tests use sequential `podman run` commands
// with shared infrastructure utilities (shared_integration_utils.go).
//
// Migration History:
// - Before Dec 21, 2025: Used podman-compose (race conditions)
// - Dec 21, 2025: Migrated to DD-TEST-002 sequential startup
// - Dec 22, 2025: Aligned ports to DD-TEST-001 sequential pattern
// - Dec 26, 2025: Migrated to shared integration utilities (shared_integration_utils.go)
//
// Related:
// - notification.go: E2E infrastructure (Kind cluster)
// - shared_integration_utils.go: Shared utilities for all integration tests
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Notification Integration Test Ports (per DD-TEST-001 v1.7 - December 2025)
// Sequential allocation after AIAnalysis (15438/16384/18095/19095)
const (
	// PostgreSQL port for Notification integration tests
	// Changed from 15439 to 15440 to resolve HAPI conflict (DD-TEST-001 v2.0) on Jan 1, 2026
	NTIntegrationPostgresPort = 15440

	// Redis port for Notification integration tests
	// Changed from 16399 (ad-hoc "+20") to 16385 (DD-TEST-001 sequential) on Dec 22, 2025
	NTIntegrationRedisPort = 16385

	// DataStorage HTTP API port for Notification integration tests
	// Changed from 18110 (ad-hoc "+20") to 18096 (DD-TEST-001 sequential) on Dec 22, 2025
	NTIntegrationDataStoragePort = 18096

	// DataStorage Metrics port for Notification integration tests
	// Changed from 19110 (ad-hoc "+20") to 19096 (DD-TEST-001 metrics pattern) on Dec 22, 2025
	NTIntegrationMetricsPort = 19096
)

// Notification Integration Test Container Names
// These match the container names in setup-infrastructure.sh for sequential startup
const (
	// PostgreSQL container name (matches setup-infrastructure.sh)
	NTIntegrationPostgresContainer = "notification_postgres_1"

	// Redis container name (matches setup-infrastructure.sh)
	NTIntegrationRedisContainer = "notification_redis_1"

	// DataStorage container name (matches setup-infrastructure.sh)
	NTIntegrationDataStorageContainer = "notification_datastorage_1"

	// Migrations container name (matches setup-infrastructure.sh)
	NTIntegrationMigrationsContainer = "notification_migrations"

	// Network name for container communication (matches setup-infrastructure.sh)
	NTIntegrationNetwork = "notification_test-network"
)

// Notification Integration Test Database Configuration
// These match the database settings in setup-infrastructure.sh
const (
	// Database name for audit events
	NTIntegrationDBName = "action_history"

	// Database user
	NTIntegrationDBUser = "slm_user"

	// Database password
	NTIntegrationDBPassword = "test_password"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// USAGE NOTES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Infrastructure is managed programmatically (no manual scripts needed).
//
// Running Tests:
//   make test-integration-notification  # Infrastructure starts/stops automatically
//
// Manual Infrastructure Control:
//   import "github.com/jordigilh/kubernaut/test/infrastructure"
//   infrastructure.StartNotificationIntegrationInfrastructure(os.Stdout)
//   defer infrastructure.StopNotificationIntegrationInfrastructure(os.Stdout)
//
// Health Check:
//   curl http://127.0.0.1:18096/health  # Should return 200 OK
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Infrastructure Management Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// StartNotificationIntegrationInfrastructure starts the Notification integration test infrastructure
// using sequential podman run commands per DD-TEST-002.
//
// Pattern: DD-TEST-002 Sequential Startup Pattern (using shared utilities)
// - Sequential container startup (eliminates race conditions)
// - Explicit health checks after each service
// - Uses shared_integration_utils.go for PostgreSQL, Redis, migrations
// - No podman-compose needed (only podman)
// - Parallel-safe with unique ports (DD-TEST-001)
//
// Infrastructure Components:
// - PostgreSQL (port 15440): DataStorage backend (unique, no longer shared with HAPI)
// - Redis (port 16385): DataStorage DLQ
// - DataStorage API (port 18096): Audit events
//
// Returns:
// - error: Any errors during infrastructure startup
func StartNotificationIntegrationInfrastructure(writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Notification Integration Test Infrastructure Setup\n")
	_, _ = fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", NTIntegrationPostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:          localhost:%d\n", NTIntegrationRedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", NTIntegrationDataStoragePort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// ============================================================================
	// STEP 1: Cleanup existing containers
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
	CleanupContainers([]string{
		NTIntegrationPostgresContainer,
		NTIntegrationRedisContainer,
		NTIntegrationDataStorageContainer,
		NTIntegrationMigrationsContainer,
	}, writer)
	_, _ = fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// ============================================================================
	// STEP 2: Start PostgreSQL FIRST (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
	if err := StartPostgreSQL(PostgreSQLConfig{
		ContainerName: NTIntegrationPostgresContainer,
		Port:          NTIntegrationPostgresPort,
		DBName:        NTIntegrationDBName,
		DBUser:        NTIntegrationDBUser,
		DBPassword:    NTIntegrationDBPassword,
	}, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// ============================================================================
	// STEP 3: Wait for PostgreSQL to be ready (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
	if err := WaitForPostgreSQLReady(
		NTIntegrationPostgresContainer,
		NTIntegrationDBUser,
		NTIntegrationDBName,
		writer,
	); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "\n")

	// ============================================================================
	// STEP 4: Run migrations (using local migrations directory)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🔄 Running database migrations...\n")
	projectRoot := getProjectRoot()
	migrationsCmd := exec.Command("podman", "run", "--rm",
		"-e", "PGHOST=host.containers.internal", // Use host.containers.internal for port-mapped PostgreSQL
		"-e", fmt.Sprintf("PGPORT=%d", NTIntegrationPostgresPort),
		"-e", fmt.Sprintf("PGUSER=%s", NTIntegrationDBUser),
		"-e", fmt.Sprintf("PGPASSWORD=%s", NTIntegrationDBPassword),
		"-e", fmt.Sprintf("PGDATABASE=%s", NTIntegrationDBName),
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
		_, _ = fmt.Fprintf(writer, "\n❌ Migration command failed - check output above for specific SQL errors\n")
		return fmt.Errorf("migrations failed (check test output for details): %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// ============================================================================
	// STEP 5: Start Redis (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "🔴 Starting Redis...\n")
	if err := StartRedis(RedisConfig{
		ContainerName: NTIntegrationRedisContainer,
		Port:          NTIntegrationRedisPort,
	}, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// ============================================================================
	// STEP 6: Wait for Redis to be ready (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := WaitForRedisReady(NTIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("redis failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "\n")

	// ============================================================================
	// STEP 7: Build DataStorage image (using GenerateInfraImageName for consistency)
	// ============================================================================
	dsImageTag := GenerateInfraImageName("datastorage", "notification")
	_, _ = fmt.Fprintf(writer, "🏗️  Resolving DataStorage image (%s)...\n", dsImageTag)
	actualDSImage, err := buildDataStorageImageWithTag(dsImageTag, writer)
	if err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage image ready: %s\n\n", actualDSImage)

	// ============================================================================
	// STEP 8: Start DataStorage LAST (service-specific)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
	if err := startNotificationDataStorage(actualDSImage, writer); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// ============================================================================
	// STEP 9: Wait for DataStorage HTTP endpoint (using shared utility)
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://127.0.0.1:%d/health", NTIntegrationDataStoragePort),
		30*time.Second,
		writer,
	); err != nil {
		// Print container logs for debugging
		_, _ = fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", NTIntegrationDataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "\n")

	// ============================================================================
	// SUCCESS
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "✅ Notification Integration Infrastructure Ready\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", NTIntegrationPostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:             localhost:%d\n", NTIntegrationRedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage HTTP:  http://localhost:%d\n", NTIntegrationDataStoragePort)
	_, _ = fmt.Fprintf(writer, "  DataStorage Metrics: http://localhost:%d\n", NTIntegrationMetricsPort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopNotificationIntegrationInfrastructure stops and cleans up the Notification integration test infrastructure
//
// Pattern: DD-TEST-002 Sequential Cleanup (using shared utilities)
// - Stop containers in reverse order
// - Remove containers and network
// - Parallel-safe (called from SynchronizedAfterSuite)
//
// Returns:
// - error: Any errors during infrastructure cleanup
func StopNotificationIntegrationInfrastructure(writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🛑 Stopping Notification Integration Infrastructure...\n")

	// Stop and remove containers (uses shared utility)
	CleanupContainers([]string{
		NTIntegrationDataStorageContainer,
		NTIntegrationRedisContainer,
		NTIntegrationPostgresContainer,
		NTIntegrationMigrationsContainer, // Usually already removed (--rm flag)
	}, writer)

	// Remove network (ignore errors)
	networkCmd := exec.Command("podman", "network", "rm", NTIntegrationNetwork)
	_ = networkCmd.Run()

	_, _ = fmt.Fprintf(writer, "✅ Notification Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Service-Specific Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// startNotificationDataStorage starts the DataStorage container for Notification integration tests
// This is service-specific because it needs to connect to Notification-specific PostgreSQL/Redis instances
func startNotificationDataStorage(imageTag string, writer io.Writer) error {
	projectRoot := getProjectRoot()

	// Use existing config file from test/integration/notification/config/
	configDir := filepath.Join(projectRoot, "test", "integration", "notification", "config")
	configMount := fmt.Sprintf("%s:/etc/datastorage:ro", configDir)

	cmd := exec.Command("podman", "run", "-d",
		"--name", NTIntegrationDataStorageContainer,
		"-p", fmt.Sprintf("%d:8080", NTIntegrationDataStoragePort),
		"-p", fmt.Sprintf("%d:9090", NTIntegrationMetricsPort),
		"-v", configMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		imageTag,
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
