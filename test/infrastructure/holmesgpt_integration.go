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
	"os"
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
//   PostgreSQL:   15439  (HAPI-specific, shared with Notification/WE)
//   Redis:        16387  (HAPI-specific, shared with Notification/WE)
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
	HAPIIntegrationPostgresPort    = 15439 // HAPI-specific port (shared with Notification/WE)
	HAPIIntegrationRedisPort       = 16387 // HAPI-specific port (shared with Notification/WE)
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
// - Ports 15439, 16387, 18098, 18120 must be available (per DD-TEST-001 v1.8)
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
	// STEP 7: Build and start HAPI service
	// ============================================================================
	fmt.Fprintf(writer, "🏗️  Building HAPI (HolmesGPT-API) image...\n")

	// Use composite image tag per DD-INTEGRATION-001 v2.0 (collision avoidance)
	hapiImageTag := GenerateInfraImageName("holmesgpt-api", "holmesgptapi")
	fmt.Fprintf(writer, "   Image tag: %s (composite per DD-INTEGRATION-001 v2.0)\n", hapiImageTag)

	buildHapiCmd := exec.Command("podman", "build",
		"-t", hapiImageTag,
		"-f", filepath.Join(projectRoot, "holmesgpt-api/Dockerfile"),
		projectRoot,
	)
	buildHapiCmd.Stdout = writer
	buildHapiCmd.Stderr = writer
	if err := buildHapiCmd.Run(); err != nil {
		return fmt.Errorf("failed to build HAPI image: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ HAPI image built: %s\n\n", hapiImageTag)

	// ADR-030: Create minimal HAPI config file for integration tests
	hapiConfigDir := filepath.Join(projectRoot, "test", "integration", "holmesgptapi", "hapi-config")
	os.MkdirAll(hapiConfigDir, 0755)

	hapiConfig := `# HolmesGPT-API Integration Test Configuration
# Per ADR-030: Configuration Management Standard
# Only business-critical settings exposed

logging:
  level: "DEBUG"

llm:
  provider: "mock"
  model: "mock/test-model"
  endpoint: "http://localhost:11434"
  secrets_file: "/etc/holmesgpt/secrets/llm-credentials.yaml"

data_storage:
  url: "http://` + HAPIIntegrationDataStorageContainer + `:8080"
`
	os.WriteFile(filepath.Join(hapiConfigDir, "config.yaml"), []byte(hapiConfig), 0644)

	// Create secrets directory and llm-credentials.yaml (ADR-030: secrets in separate file)
	hapiSecretsDir := filepath.Join(hapiConfigDir, "secrets")
	os.MkdirAll(hapiSecretsDir, 0755)
	hapiSecrets := `# LLM Provider Credentials (Integration Tests)
api_key: "mock-api-key"
`
	os.WriteFile(filepath.Join(hapiSecretsDir, "llm-credentials.yaml"), []byte(hapiSecrets), 0644)

	fmt.Fprintf(writer, "🚀 Starting HAPI (HolmesGPT-API) container...\n")
	hapiCmd := exec.Command("podman", "run", "-d",
		"--name", HAPIIntegrationHAPIContainer,
		"--network", HAPIIntegrationNetwork,
		"-p", fmt.Sprintf("%d:8080", HAPIIntegrationServicePort),
		"-v", fmt.Sprintf("%s:/etc/holmesgpt:ro", hapiConfigDir),
		"-e", "MOCK_LLM_MODE=true", // Use mock LLM for integration tests
		hapiImageTag,
		"-config", "/etc/holmesgpt/config.yaml", // ADR-030: Use -config flag (like Go services)
	)
	hapiCmd.Stdout = writer
	hapiCmd.Stderr = writer
	if err := hapiCmd.Run(); err != nil {
		return fmt.Errorf("failed to start HAPI: %w", err)
	}

	// CRITICAL: Wait for HAPI HTTP health endpoint
	hapiURL := fmt.Sprintf("http://localhost:%d/health", HAPIIntegrationServicePort)
	if err := WaitForHTTPHealth(hapiURL, 60*time.Second, writer); err != nil {
		return fmt.Errorf("HAPI failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ HAPI ready at %s\n\n", hapiURL)

	// ============================================================================
	// Success Summary
	// ============================================================================
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ HolmesGPT API Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d (ready)\n", HAPIIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d (ready)\n", HAPIIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d (healthy)\n", HAPIIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  HAPI:           http://localhost:%d (healthy)\n", HAPIIntegrationServicePort)
	fmt.Fprintf(writer, "  Duration:       ~3-4 minutes\n")
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

	// Stop containers (reverse order: HAPI first, then dependencies)
	containers := []string{
		HAPIIntegrationHAPIContainer,
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
