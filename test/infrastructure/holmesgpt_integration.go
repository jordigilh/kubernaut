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
	"strings"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HolmesGPT API Integration Test Infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic Podman Setup using Go
// Replaces: Python pytest fixtures calling docker-compose via subprocess
//
// Port Allocation (per DD-TEST-001 v1.8):
//   PostgreSQL:   15439  (HAPI-specific, unique - Notification now uses 15440)
//   Redis:        16387  (HAPI-specific, unique - all services have separate Redis ports)
//   DataStorage:  18098  (HAPI allocation)
//   HAPI:         18120  (HAPI service port)
//
// Dependencies:
//   HolmesGPT-API Tests → HAPI Service (HTTP API)
//   HAPI Service → Data Storage (workflow catalog, audit)
//   Data Storage → PostgreSQL (persistence)
//   Data Storage → Redis (caching/DLQ)
//
// Migration: December 27, 2025
//   From: Python pytest fixtures → docker-compose via subprocess.run()
//   To:   Go programmatic setup → shared utilities from shared_integration_utils.go
//   Benefits: Consistency with other services, no subprocess calls, reuses 720 lines of shared code
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Port allocation per DD-TEST-001 v1.8
const (
	HAPIIntegrationPostgresPort    = 15439 // HAPI-specific port (unique - Notification moved to 15440)
	HAPIIntegrationRedisPort       = 16387 // HAPI-specific port (unique - all services have separate Redis ports)
	HAPIIntegrationDataStoragePort = 18098 // HAPI allocation per DD-TEST-001 v1.8
	HAPIIntegrationServicePort     = 18120 // HAPI service port (per DD-TEST-001 v1.8)
)

// Container names (unique to HAPI integration tests)
const (
	HAPIIntegrationPostgresContainer    = "holmesgptapi_postgres_1"
	HAPIIntegrationRedisContainer       = "holmesgptapi_redis_1"
	HAPIIntegrationDataStorageContainer = "holmesgptapi_datastorage_1"
	HAPIIntegrationHAPIContainer        = "holmesgptapi_hapi_1"
	HAPIIntegrationMigrationsContainer  = "holmesgptapi_migrations"
	HAPIIntegrationNetwork              = "holmesgptapi_test-network"
)

// Database configuration
const (
	HAPIIntegrationDBName     = "action_history"
	HAPIIntegrationDBUser     = "slm_user"
	HAPIIntegrationDBPassword = "test_password"
)

// StartHolmesGPTAPIIntegrationInfrastructure starts the full Podman stack for HAPI integration tests
// This includes: PostgreSQL, Redis, DataStorage API, and HolmesGPT-API (HAPI) service
//
// Pattern: DD-TEST-002 Sequential Startup Pattern (using shared utilities from shared_integration_utils.go)
// - Programmatic `podman run` commands (NOT docker-compose)
// - Explicit health checks after each service
// - Parallel-safe (called from SynchronizedBeforeSuite)
//
// Prerequisites:
// - podman must be installed
// - Ports 15439, 16387, 18098, 18120 must be available (per DD-TEST-001 v2.0 - all unique)
//
// Returns:
// - error: Any errors during infrastructure startup
//
// Migration Note:
//
//	Replaces holmesgpt-api/tests/integration/conftest.py start_infrastructure()
//	which used subprocess.run() to call docker-compose.
func StartHolmesGPTAPIIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting HolmesGPT API Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", HAPIIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", HAPIIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", HAPIIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  HAPI:           http://localhost:%d\n", HAPIIntegrationServicePort)
	fmt.Fprintf(writer, "  Pattern:        DD-INTEGRATION-001 v2.0 (Programmatic Go)\n")
	fmt.Fprintf(writer, "  Migration:      From Python subprocess → Go shared utilities\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	projectRoot := getProjectRoot()

	// ============================================================================
	// STEP 1: Cleanup existing containers and network
	// ============================================================================
	CleanupContainers([]string{
		HAPIIntegrationHAPIContainer,
		HAPIIntegrationDataStorageContainer,
		HAPIIntegrationRedisContainer,
		HAPIIntegrationPostgresContainer,
		HAPIIntegrationMigrationsContainer,
	}, writer)
	_ = exec.Command("podman", "network", "rm", HAPIIntegrationNetwork).Run() // Ignore errors
	fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// ============================================================================
	// STEP 2: Create custom network for internal communication
	// ============================================================================
	fmt.Fprintf(writer, "🌐 Creating custom Podman network '%s'...\n", HAPIIntegrationNetwork)
	createNetworkCmd := exec.Command("podman", "network", "create", HAPIIntegrationNetwork)
	createNetworkCmd.Stdout = writer
	createNetworkCmd.Stderr = writer
	if err := createNetworkCmd.Run(); err != nil {
		// Ignore if network already exists
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create network '%s': %w", HAPIIntegrationNetwork, err)
		}
		fmt.Fprintf(writer, "  (Network '%s' already exists, continuing...)\n", HAPIIntegrationNetwork)
	}
	fmt.Fprintf(writer, "   ✅ Network '%s' created/ensured\n\n", HAPIIntegrationNetwork)

	// ============================================================================
	// STEP 3: Start PostgreSQL FIRST (DD-TEST-002 Sequential Pattern)
	// ============================================================================
	pgConfig := PostgreSQLConfig{
		ContainerName:  HAPIIntegrationPostgresContainer,
		Port:           HAPIIntegrationPostgresPort,
		DBName:         HAPIIntegrationDBName,
		DBUser:         HAPIIntegrationDBUser,
		DBPassword:     HAPIIntegrationDBPassword,
		Network:        HAPIIntegrationNetwork,
		MaxConnections: 200,
	}
	if err := StartPostgreSQL(pgConfig, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// CRITICAL: Wait for PostgreSQL to be ready before proceeding
	if err := WaitForPostgreSQLReady(HAPIIntegrationPostgresContainer, HAPIIntegrationDBUser, HAPIIntegrationDBName, writer); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n\n")

	// ============================================================================
	// STEP 4: Run migrations (inline approach - same as AIAnalysis/RO)
	// ============================================================================
	fmt.Fprintf(writer, "🔄 Running migrations...\n")
	migrationsCmd := exec.Command("podman", "run", "--rm",
		"--network", HAPIIntegrationNetwork,
		"-e", "PGHOST="+HAPIIntegrationPostgresContainer,
		"-e", "PGPORT=5432",
		"-e", "PGUSER="+HAPIIntegrationDBUser,
		"-e", "PGPASSWORD="+HAPIIntegrationDBPassword,
		"-e", "PGDATABASE="+HAPIIntegrationDBName,
		"-v", filepath.Join(projectRoot, "migrations")+":/migrations:ro",
		"postgres:16-alpine",
		"sh", "-c",
		`set -e
echo "Applying migrations (Up sections only)..."
find /migrations -maxdepth 1 -name '*.sql' -type f | sort | while read f; do
  echo "Applying $f..."
  sed -n '1,/^-- +goose Down/p' "$f" | grep -v '^-- +goose Down' | psql
done
echo "Migrations complete!"`)
	migrationsCmd.Stdout = writer
	migrationsCmd.Stderr = writer
	if err := migrationsCmd.Run(); err != nil {
		return fmt.Errorf("migrations failed: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// ============================================================================
	// STEP 5: Start Redis
	// ============================================================================
	redisConfig := RedisConfig{
		ContainerName: HAPIIntegrationRedisContainer,
		Port:          HAPIIntegrationRedisPort,
		Network:       HAPIIntegrationNetwork,
	}
	if err := StartRedis(redisConfig, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// CRITICAL: Wait for Redis to be ready before proceeding
	if err := WaitForRedisReady(HAPIIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("Redis failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Redis ready\n\n")

	// ============================================================================
	// STEP 6: Build and start DataStorage
	// ============================================================================
	fmt.Fprintf(writer, "🏗️  Building DataStorage image...\n")

	// Use composite image tag per DD-INTEGRATION-001 v2.0 (collision avoidance)
	dsImageTag := GenerateInfraImageName("datastorage", "holmesgptapi")
	fmt.Fprintf(writer, "   Image tag: %s (composite per DD-INTEGRATION-001 v2.0)\n", dsImageTag)

	buildCmd := exec.Command("podman", "build",
		"-t", dsImageTag,
		"-f", filepath.Join(projectRoot, "docker/data-storage.Dockerfile"),
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ DataStorage image built: %s\n\n", dsImageTag)

	fmt.Fprintf(writer, "🚀 Starting DataStorage container...\n")

	// ADR-030: Mount config directory and set CONFIG_PATH
	// The same directory contains both config.yaml and secrets files
	configDir := filepath.Join(projectRoot, "test", "integration", "holmesgptapi", "config")

	dsCmd := exec.Command("podman", "run", "-d",
		"--name", HAPIIntegrationDataStorageContainer,
		"--network", HAPIIntegrationNetwork,
		"-p", fmt.Sprintf("%d:8080", HAPIIntegrationDataStoragePort),
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-v", fmt.Sprintf("%s:/etc/datastorage-secrets:ro", configDir), // Mount same dir for secrets
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"-e", "LOG_LEVEL=DEBUG", // Debug level for integration tests
		dsImageTag,
	)
	dsCmd.Stdout = writer
	dsCmd.Stderr = writer
	if err := dsCmd.Run(); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// CRITICAL: Wait for DataStorage HTTP health endpoint
	dataStorageURL := fmt.Sprintf("http://localhost:%d/health", HAPIIntegrationDataStoragePort)
	if err := WaitForHTTPHealth(dataStorageURL, 60*time.Second, writer); err != nil {
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ DataStorage ready at %s\n\n", dataStorageURL)

	// ============================================================================
	// INTEGRATION TEST PATTERN: HAPI uses FastAPI TestClient (in-process)
	// ============================================================================
	// Architecture Decision (Dec 29, 2025):
	// - Integration tests: HAPI runs via FastAPI TestClient (in-process)
	// - E2E tests: HAPI runs in Kind cluster (external container)
	//
	// Why TestClient for Integration Tests?
	// ✅ Faster (~3 min vs ~7 min, no Docker builds)
	// ✅ Reliable audit persistence (in-process config)
	// ✅ Consistent with Python/FastAPI best practices
	// ✅ Easier debugging (in-process, direct access to app state)
	//
	// External dependencies for integration tests:
	// - PostgreSQL (for Data Storage persistence)
	// - Redis (for Data Storage caching)
	// - Data Storage (for audit event validation)
	//
	// See: docs/shared/HAPI_INTEGRATION_TEST_ARCHITECTURE_FIX_DEC_29_2025.md
	// ============================================================================
	fmt.Fprintf(writer, "ℹ️  HAPI Integration Test Pattern:\n")
	fmt.Fprintf(writer, "   • HAPI runs via FastAPI TestClient (in-process, no container)\n")
	fmt.Fprintf(writer, "   • Python tests import src.main:app directly\n")
	fmt.Fprintf(writer, "   • External dependencies: PostgreSQL, Redis, Data Storage only\n")
	fmt.Fprintf(writer, "   • See: holmesgpt-api/tests/integration/conftest.py\n\n")

	// ============================================================================
	// Success Summary
	// ============================================================================
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ HolmesGPT API Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d (ready)\n", HAPIIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d (ready)\n", HAPIIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d (healthy)\n", HAPIIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  HAPI:           FastAPI TestClient (in-process, no container)\n")
	fmt.Fprintf(writer, "  Duration:       ~2-3 minutes (no HAPI Docker build)\n")
	fmt.Fprintf(writer, "  Pattern:        DD-INTEGRATION-001 v2.0 (Go programmatic)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	return nil
}

// StopHolmesGPTAPIIntegrationInfrastructure stops all HAPI integration test infrastructure
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic cleanup
// - Stops containers gracefully
// - Removes network
// - Prunes infrastructure images (composite tags)
//
// Migration Note:
//
//	Replaces holmesgpt-api/tests/integration/conftest.py cleanup_infrastructure_after_tests()
//	which used subprocess.run() to call docker-compose down.
func StopHolmesGPTAPIIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "\n🛑 Stopping HolmesGPT API Integration Infrastructure...\n")

	// Stop containers (no HAPI container for integration tests - uses TestClient)
	// Note: HAPI container only exists for E2E tests in Kind
	containers := []string{
		HAPIIntegrationDataStorageContainer,
		HAPIIntegrationRedisContainer,
		HAPIIntegrationPostgresContainer,
	}
	CleanupContainers(containers, writer)

	// Remove network
	fmt.Fprintf(writer, "   Removing network %s...\n", HAPIIntegrationNetwork)
	_ = exec.Command("podman", "network", "rm", HAPIIntegrationNetwork).Run() // Ignore errors

	// Prune dangling images (DD-INTEGRATION-001 v2.0: composite tags cleanup)
	fmt.Fprintf(writer, "   Pruning dangling images (composite tags)...\n")
	pruneCmd := exec.Command("podman", "image", "prune", "-f")
	pruneCmd.Stdout = writer
	pruneCmd.Stderr = writer
	_ = pruneCmd.Run() // Ignore errors

	fmt.Fprintf(writer, "✅ Infrastructure cleaned up\n\n")
	return nil
}
