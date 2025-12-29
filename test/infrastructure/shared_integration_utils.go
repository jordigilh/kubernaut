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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Shared Integration Test Infrastructure Utilities
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// These utilities are shared across all Podman-based integration test infrastructure:
// - Gateway, RemediationOrchestrator, WorkflowExecution, SignalProcessing, Notification, DataStorage, AIAnalysis
//
// Per DD-TEST-002: Sequential Startup Pattern
// - Sequential container startup (eliminates race conditions)
// - Explicit health checks after each service
// - No podman-compose dependency (only podman needed)
// - Parallel-safe with unique ports (DD-TEST-001)
//
// Design Philosophy:
// - Parameterized for reuse (container names, ports, credentials)
// - Consistent behavior across all services
// - Easy to test and maintain
// - Follows DRY principle
//
// Mirrors E2E Pattern:
// - E2E tests have DeployDataStorageTestServices() (Kubernetes)
// - Integration tests now have these shared utilities (Podman)
//
// Created: December 26, 2025
// Addresses: ~720 lines of duplicated code across 6 services
// Savings: ~320 lines (-44% duplication)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Image Naming Utilities (DD-INTEGRATION-001 v2.0)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateInfraImageName generates a composite image tag for shared infrastructure images
// per DD-INTEGRATION-001 v2.0 Section "Image Naming Convention".
//
// Format: localhost/{infrastructure}:{consumer}-{8-char-hex-uuid}
//
// Examples:
//   GenerateInfraImageName("datastorage", "aianalysis")
//   → "localhost/datastorage:aianalysis-a3b5c7d9"
//
//   GenerateInfraImageName("datastorage", "workflowexecution")
//   → "localhost/datastorage:workflowexecution-1884d074"
//
// Benefits:
// - Prevents image tag collisions during parallel test runs
// - Clear traceability (which consumer built which image)
// - Consistent with DD-INTEGRATION-001 v2.0 standard
//
// Parameters:
//   infrastructure: The infrastructure service name (e.g., "datastorage", "redis")
//   consumer: The consuming service name (e.g., "aianalysis", "gateway", "workflowexecution")
//
// Returns: Composite image tag in the format required by DD-INTEGRATION-001
func GenerateInfraImageName(infrastructure, consumer string) string {
	// Generate 8-character hex UUID for uniqueness
	uuid := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(uuid); err != nil {
		// Fallback to timestamp if crypto/rand fails (should never happen)
		return fmt.Sprintf("localhost/%s:%s-%d", infrastructure, consumer, time.Now().Unix())
	}

	hexUUID := hex.EncodeToString(uuid)
	return fmt.Sprintf("localhost/%s:%s-%s", infrastructure, consumer, hexUUID)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// PostgreSQL Infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// PostgreSQLConfig holds configuration for starting a PostgreSQL container
type PostgreSQLConfig struct {
	ContainerName  string // e.g., "gateway_postgres_1"
	Port           int    // e.g., 15437
	DBName         string // e.g., "kubernaut"
	DBUser         string // e.g., "kubernaut"
	DBPassword     string // e.g., "kubernaut-test-password"
	Network        string // Optional: custom network name (e.g., "gateway_test-network")
	MaxConnections int    // Optional: e.g., 200 (default: PostgreSQL default)
}

// StartPostgreSQL starts a PostgreSQL container for integration tests
// Uses postgres:16-alpine image for consistency across all services
//
// Pattern: DD-TEST-002 Sequential Startup
// - Cleanup existing container first
// - Start with explicit configuration
// - Return immediately (caller handles health check)
//
// Usage:
//   cfg := PostgreSQLConfig{
//       ContainerName: "myservice_postgres_1",
//       Port: 15437,
//       DBName: "action_history",
//       DBUser: "slm_user",
//       DBPassword: "test_password",
//   }
//   if err := StartPostgreSQL(cfg, writer); err != nil {
//       return err
//   }
func StartPostgreSQL(cfg PostgreSQLConfig, writer io.Writer) error {
	// Build podman run command
	args := []string{"run", "-d",
		"--name", cfg.ContainerName,
		"-p", fmt.Sprintf("%d:5432", cfg.Port),
		"-e", "POSTGRES_DB=" + cfg.DBName,
		"-e", "POSTGRES_USER=" + cfg.DBUser,
		"-e", "POSTGRES_PASSWORD=" + cfg.DBPassword,
	}

	// Add network if specified
	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
	}

	args = append(args, "postgres:16-alpine")

	// Add max_connections if specified
	if cfg.MaxConnections > 0 {
		args = append(args, "-c", fmt.Sprintf("max_connections=%d", cfg.MaxConnections))
	}

	cmd := exec.Command("podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// WaitForPostgreSQLReady waits for PostgreSQL to be ready to accept connections
// Uses pg_isready command to check PostgreSQL availability
//
// Pattern: DD-TEST-002 Health Check
// - 30 attempts with 1 second intervals
// - Logs progress for debugging
// - Returns error if not ready after timeout
//
// Usage:
//   if err := WaitForPostgreSQLReady("myservice_postgres_1", "slm_user", "action_history", writer); err != nil {
//       return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
//   }
func WaitForPostgreSQLReady(containerName, dbUser, dbName string, writer io.Writer) error {
	maxAttempts := 30
	for i := 1; i <= maxAttempts; i++ {
		cmd := exec.Command("podman", "exec", containerName,
			"pg_isready", "-U", dbUser, "-d", dbName)
		if cmd.Run() == nil {
			fmt.Fprintf(writer, "   ✅ PostgreSQL ready (attempt %d/%d)\n", i, maxAttempts)
			return nil
		}
		if i < maxAttempts {
			time.Sleep(1 * time.Second)
		}
	}
	return fmt.Errorf("PostgreSQL failed to become ready after %d attempts", maxAttempts)
}

// RedisConfig holds configuration for starting a Redis container
type RedisConfig struct {
	ContainerName string // e.g., "gateway_redis_1"
	Port          int    // e.g., 16383
	Network       string // Optional: custom network name (e.g., "gateway_test-network")
}

// StartRedis starts a Redis container for integration tests
// Uses redis:7-alpine image for consistency across all services
//
// Pattern: DD-TEST-002 Sequential Startup
// - Cleanup existing container first
// - Start with minimal configuration
// - Return immediately (caller handles health check)
//
// Usage:
//   cfg := RedisConfig{
//       ContainerName: "myservice_redis_1",
//       Port: 16383,
//   }
//   if err := StartRedis(cfg, writer); err != nil {
//       return err
//   }
func StartRedis(cfg RedisConfig, writer io.Writer) error {
	args := []string{"run", "-d",
		"--name", cfg.ContainerName,
		"-p", fmt.Sprintf("%d:6379", cfg.Port),
	}

	// Add network if specified
	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
	}

	args = append(args, "redis:7-alpine")

	cmd := exec.Command("podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// WaitForRedisReady waits for Redis to be ready to accept connections
// Uses redis-cli ping to check Redis availability
//
// Pattern: DD-TEST-002 Health Check
// - 30 attempts with 1 second intervals
// - Expects "PONG" response from redis-cli ping
// - Logs progress for debugging
//
// Usage:
//   if err := WaitForRedisReady("myservice_redis_1", writer); err != nil {
//       return fmt.Errorf("Redis failed to become ready: %w", err)
//   }
func WaitForRedisReady(containerName string, writer io.Writer) error {
	maxAttempts := 30
	for i := 1; i <= maxAttempts; i++ {
		cmd := exec.Command("podman", "exec", containerName, "redis-cli", "ping")
		output, err := cmd.CombinedOutput()
		if err == nil && string(output) == "PONG\n" {
			fmt.Fprintf(writer, "   ✅ Redis ready (attempt %d/%d)\n", i, maxAttempts)
			return nil
		}
		if i < maxAttempts {
			time.Sleep(1 * time.Second)
		}
	}
	return fmt.Errorf("Redis failed to become ready after %d attempts", maxAttempts)
}

// WaitForHTTPHealth waits for an HTTP health endpoint to return 200 OK
// Generic health check suitable for any HTTP service (DataStorage, HolmesGPT-API, etc.)
//
// Pattern: DD-TEST-002 Health Check
// - Configurable timeout (typically 30-60 seconds)
// - 5-second HTTP client timeout per request
// - Logs every 5th attempt for debugging
// - Returns detailed error with attempt count
//
// Usage:
//   if err := WaitForHTTPHealth("http://localhost:18096/health", 30*time.Second, writer); err != nil {
//       return fmt.Errorf("DataStorage failed to become healthy: %w", err)
//   }
func WaitForHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintf(writer, "   ✅ Health check passed (attempt %d)\n", attempt)
				return nil
			}
			// Log every 5th non-OK status for debugging
			if attempt%5 == 0 {
				fmt.Fprintf(writer, "   ⏳ Attempt %d: Status %d (waiting for 200 OK)...\n", attempt, resp.StatusCode)
			}
		} else {
			// Log every 5th connection error for debugging
			if attempt%5 == 0 {
				fmt.Fprintf(writer, "   ⏳ Attempt %d: Connection failed (%v), retrying...\n", attempt, err)
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("health check failed for %s after %v (attempts: %d)", healthURL, timeout, attempt)
}

// CleanupContainers stops and removes containers
// Safe to call even if containers don't exist (errors are ignored)
//
// Pattern: DD-TEST-002 Cleanup
// - Stop with 5-second timeout (graceful shutdown)
// - Force remove if stop fails
// - Ignore all errors (idempotent)
//
// Usage:
//   CleanupContainers([]string{
//       "myservice_postgres_1",
//       "myservice_redis_1",
//       "myservice_datastorage_1",
//   }, writer)
func CleanupContainers(containerNames []string, writer io.Writer) {
	for _, container := range containerNames {
		// Stop container (5-second timeout for graceful shutdown)
		stopCmd := exec.Command("podman", "stop", "-t", "5", container)
		stopCmd.Stdout = writer
		stopCmd.Stderr = writer
		_ = stopCmd.Run() // Ignore errors (container might not exist)

		// Remove container (force)
		rmCmd := exec.Command("podman", "rm", "-f", container)
		rmCmd.Stdout = writer
		rmCmd.Stderr = writer
		_ = rmCmd.Run() // Ignore errors (container might not exist)
	}
}

// MigrationsConfig holds configuration for running database migrations
type MigrationsConfig struct {
	ContainerName   string // e.g., "gateway_migrations"
	Network         string // Optional: Podman network name (if not using host network)
	PostgresHost    string // e.g., "localhost" or "gateway_postgres_1" (if using network)
	PostgresPort    int    // e.g., 15437
	DBName          string // e.g., "kubernaut"
	DBUser          string // e.g., "kubernaut"
	DBPassword      string // e.g., "kubernaut-test-password"
	MigrationsImage string // e.g., "quay.io/jordigilh/datastorage-migrations:latest"
}

// RunMigrations runs database migrations in a temporary container
// Container is automatically removed after migrations complete (--rm flag)
//
// Pattern: DD-TEST-002 Sequential Startup
// - Runs after PostgreSQL is ready
// - Blocks until migrations complete
// - Container is ephemeral (removed after completion)
//
// Usage:
//   cfg := MigrationsConfig{
//       ContainerName: "gateway_migrations",
//       PostgresHost: "localhost",
//       PostgresPort: 15437,
//       DBName: "kubernaut",
//       DBUser: "kubernaut",
//       DBPassword: "kubernaut-test-password",
//       MigrationsImage: "quay.io/jordigilh/datastorage-migrations:latest",
//   }
//   if err := RunMigrations(cfg, writer); err != nil {
//       return fmt.Errorf("failed to run migrations: %w", err)
//   }
func RunMigrations(cfg MigrationsConfig, writer io.Writer) error {
	// Build DATABASE_URL
	databaseURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.DBName,
	)

	// Build podman run command
	args := []string{"run", "--rm",
		"--name", cfg.ContainerName,
		"-e", "DATABASE_URL=" + databaseURL,
	}

	// Add network if specified
	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
	}

	// Add image
	args = append(args, cfg.MigrationsImage)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// IntegrationDataStorageConfig holds configuration for starting a DataStorage container in integration tests
// Note: This is separate from E2E DataStorageConfig (different use case and structure)
type IntegrationDataStorageConfig struct {
	ContainerName  string            // e.g., "aianalysis_datastorage_1"
	Port           int               // e.g., 18091 (HTTP API port)
	MetricsPort    int               // Optional: e.g., 18092 (Prometheus metrics)
	Network        string            // Optional: custom network name
	PostgresHost   string            // e.g., "localhost" or "aianalysis_postgres_1" (if using network)
	PostgresPort   int               // e.g., 15434
	DBName         string            // e.g., "action_history"
	DBUser         string            // e.g., "slm_user"
	DBPassword     string            // e.g., "test_password"
	RedisHost      string            // e.g., "localhost" or "aianalysis_redis_1" (if using network)
	RedisPort      int               // e.g., 16380
	LogLevel       string            // Optional: "info", "debug", "error" (default: "info")
	ImageTag       string            // REQUIRED: Composite tag per DD-INTEGRATION-001 v2.0 (use GenerateInfraImageName("datastorage", "yourservice"))
	ExtraEnvVars   map[string]string // Optional: additional environment variables
}

// StartDataStorage starts a DataStorage container for integration tests
// Uses docker/data-storage.Dockerfile (authoritative location per DD-INTEGRATION-001)
//
// Pattern: DD-TEST-002 Sequential Startup
// - Build DataStorage image with --no-cache
// - Start container with explicit configuration
// - Return immediately (caller handles health check using WaitForHTTPHealth)
//
// Usage:
//   cfg := IntegrationDataStorageConfig{
//       ContainerName: "aianalysis_datastorage_1",
//       Port: 18091,
//       Network: "aianalysis_test-network",
//       PostgresHost: "aianalysis_postgres_1",
//       PostgresPort: 5432,
//       DBName: "action_history",
//       DBUser: "slm_user",
//       DBPassword: "test_password",
//       RedisHost: "aianalysis_redis_1",
//       RedisPort: 6379,
//   }
//   if err := StartDataStorage(cfg, writer); err != nil {
//       return err
//   }
//
//   // Wait for health check
//   if err := WaitForHTTPHealth("http://localhost:18091/health", 60*time.Second, writer); err != nil {
//       return err
//   }
func StartDataStorage(cfg IntegrationDataStorageConfig, writer io.Writer) error {
	projectRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Set defaults
	if cfg.ImageTag == "" {
		// ❌ DEPRECATED (DD-INTEGRATION-001 v1.0): "kubernaut-datastorage:latest"
		// ✅ REQUIRED (DD-INTEGRATION-001 v2.0): Caller MUST provide explicit composite tag
		// Format: localhost/{infrastructure}:{consumer}-{uuid}
		// Use GenerateInfraImageName("datastorage", "yourservice") in caller
		return fmt.Errorf("ImageTag is required (DD-INTEGRATION-001 v2.0): use GenerateInfraImageName(\"datastorage\", \"yourservice\")")
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	// STEP 1: Generate config files from template (ADR-030 requirement)
	fmt.Fprintf(writer, "   Generating DataStorage config and secrets (ADR-030)...\n")
	configDir, err := generateDataStorageConfig(cfg, projectRoot)
	if err != nil {
		return fmt.Errorf("failed to generate config files: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Config and secrets generated: %s\n", configDir)

	// STEP 2: Build DataStorage image
	// Per DD-INTEGRATION-001: Use docker/data-storage.Dockerfile (authoritative location)
	fmt.Fprintf(writer, "   Building DataStorage image (tag: %s)...\n", cfg.ImageTag)
	buildArgs := []string{
		"build",
		"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes
		"-t", cfg.ImageTag,
		"-f", filepath.Join(projectRoot, "docker", "data-storage.Dockerfile"),
		projectRoot, // Build context is the project root
	}

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ DataStorage image built: %s\n", cfg.ImageTag)

	// STEP 3: Start DataStorage container
	runArgs := []string{"run", "-d",
		"--name", cfg.ContainerName,
		"-p", fmt.Sprintf("%d:8080", cfg.Port), // Map host port to container's 8080
	}

	// Add metrics port if specified
	if cfg.MetricsPort > 0 {
		runArgs = append(runArgs, "-p", fmt.Sprintf("%d:9090", cfg.MetricsPort))
	}

	// Add network if specified
	if cfg.Network != "" {
		runArgs = append(runArgs, "--network", cfg.Network)
	}

	// ADR-030: Mount config directory (includes config.yaml and secrets files)
	// The generated config directory is mounted into the container at /etc/datastorage
	runArgs = append(runArgs,
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-v", fmt.Sprintf("%s:/etc/datastorage-secrets:ro", configDir), // Same dir mounted for secrets
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
	)

	// Add extra environment variables if provided
	for key, value := range cfg.ExtraEnvVars {
		runArgs = append(runArgs, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add image
	runArgs = append(runArgs, cfg.ImageTag)

	cmd := exec.Command("podman", runArgs...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start DataStorage container: %w", err)
	}

	return nil
}

// generateDataStorageConfig generates a config file from template for DataStorage service
// Returns the path to the generated config file (in /tmp)
//
// ADR-030 Compliance: DataStorage requires CONFIG_PATH pointing to a YAML file
// This function reads the template, replaces placeholders, and writes to a temp file
func generateDataStorageConfig(cfg IntegrationDataStorageConfig, projectRoot string) (string, error) {
	// Read template file
	templatePath := filepath.Join(projectRoot, "test", "infrastructure", "datastorage-integration-template.yaml")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	// Replace placeholders with actual values
	config := string(templateBytes)
	config = strings.ReplaceAll(config, "{{LOG_LEVEL}}", cfg.LogLevel)
	config = strings.ReplaceAll(config, "{{POSTGRES_HOST}}", cfg.PostgresHost)
	config = strings.ReplaceAll(config, "{{POSTGRES_PORT}}", fmt.Sprintf("%d", cfg.PostgresPort))
	config = strings.ReplaceAll(config, "{{DB_NAME}}", cfg.DBName)
	config = strings.ReplaceAll(config, "{{DB_USER}}", cfg.DBUser)
	config = strings.ReplaceAll(config, "{{DB_PASSWORD}}", cfg.DBPassword)
	config = strings.ReplaceAll(config, "{{REDIS_HOST}}", cfg.RedisHost)
	config = strings.ReplaceAll(config, "{{REDIS_PORT}}", fmt.Sprintf("%d", cfg.RedisPort))
	config = strings.ReplaceAll(config, "{{REDIS_DB}}", "0") // Integration tests always use Redis DB 0

	// Write to project directory (not /tmp - Podman on macOS can't mount from there)
	// Use container name to avoid collisions between parallel test runs
	configBaseDir := filepath.Join(projectRoot, "test", "infrastructure", ".generated-configs")
	configDir := filepath.Join(configBaseDir, cfg.ContainerName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write main config file
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	// ADR-030 Section 6: Create secrets files
	// Database credentials
	dbSecrets := fmt.Sprintf("password: %s\n", cfg.DBPassword)
	dbSecretsFile := filepath.Join(configDir, "database-credentials.yaml")
	if err := os.WriteFile(dbSecretsFile, []byte(dbSecrets), 0644); err != nil {
		return "", fmt.Errorf("failed to write database secrets file: %w", err)
	}

	// Redis credentials (empty password for integration tests)
	redisSecrets := "password: \"\"\n"
	redisSecretsFile := filepath.Join(configDir, "redis-credentials.yaml")
	if err := os.WriteFile(redisSecretsFile, []byte(redisSecrets), 0644); err != nil {
		return "", fmt.Errorf("failed to write redis secrets file: %w", err)
	}

	return configDir, nil // Return directory, not just config file
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Utility Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// findProjectRoot walks up the directory tree to find the project root (where go.mod is located).
// This is used by infrastructure setup functions to locate Dockerfiles and other project resources.
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// Walk up to find project root (contains go.mod)
	projectRoot := cwd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			return projectRoot, nil
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			// Reached filesystem root, return cwd as fallback
			return cwd, nil
		}
		projectRoot = parent
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Usage Examples
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Example 1: Basic Integration Infrastructure Setup
//
//   func StartMyServiceIntegrationInfrastructure(writer io.Writer) error {
//       // Step 1: Cleanup existing containers
//       CleanupContainers([]string{
//           "myservice_postgres_1",
//           "myservice_redis_1",
//           "myservice_datastorage_1",
//       }, writer)
//
//       // Step 2: Start PostgreSQL
//       if err := StartPostgreSQL(PostgreSQLConfig{
//           ContainerName: "myservice_postgres_1",
//           Port: 15437,
//           DBName: "action_history",
//           DBUser: "slm_user",
//           DBPassword: "test_password",
//       }, writer); err != nil {
//           return err
//       }
//
//       // Step 3: Wait for PostgreSQL ready
//       if err := WaitForPostgreSQLReady("myservice_postgres_1", "slm_user", "action_history", writer); err != nil {
//           return err
//       }
//
//       // Step 4: Run migrations
//       if err := RunMigrations(MigrationsConfig{
//           ContainerName: "myservice_migrations",
//           PostgresHost: "localhost",
//           PostgresPort: 15437,
//           DBName: "action_history",
//           DBUser: "slm_user",
//           DBPassword: "test_password",
//           MigrationsImage: "quay.io/jordigilh/datastorage-migrations:latest",
//       }, writer); err != nil {
//           return err
//       }
//
//       // Step 5: Start Redis
//       if err := StartRedis(RedisConfig{
//           ContainerName: "myservice_redis_1",
//           Port: 16383,
//       }, writer); err != nil {
//           return err
//       }
//
//       // Step 6: Wait for Redis ready
//       if err := WaitForRedisReady("myservice_redis_1", writer); err != nil {
//           return err
//       }
//
//       // Step 7: Start DataStorage (using shared utility)
//       if err := StartDataStorage(IntegrationDataStorageConfig{
//           ContainerName: "myservice_datastorage_1",
//           Port: 18096,
//           Network: "myservice_test-network",
//           PostgresHost: "myservice_postgres_1",
//           PostgresPort: 5432,
//           DBName: "action_history",
//           DBUser: "slm_user",
//           DBPassword: "test_password",
//           RedisHost: "myservice_redis_1",
//           RedisPort: 6379,
//       }, writer); err != nil {
//           return err
//       }
//
//       // Step 8: Wait for DataStorage HTTP health
//       if err := WaitForHTTPHealth("http://localhost:18096/health", 60*time.Second, writer); err != nil {
//           return err
//       }
//
//       return nil
//   }
//
// Example 2: Cleanup Infrastructure
//
//   func StopMyServiceIntegrationInfrastructure(writer io.Writer) error {
//       CleanupContainers([]string{
//           "myservice_datastorage_1",
//           "myservice_redis_1",
//           "myservice_postgres_1",
//           "myservice_migrations", // Usually already removed (--rm flag)
//       }, writer)
//       return nil
//   }
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

