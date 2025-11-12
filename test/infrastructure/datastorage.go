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
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
)

// DataStorageInfrastructure manages the Data Storage Service test infrastructure
// This includes PostgreSQL, Redis, and the Data Storage Service itself
type DataStorageInfrastructure struct {
	PostgresContainer string
	RedisContainer    string
	ServiceContainer  string
	ConfigDir         string
	ServiceURL        string
	DB                *sql.DB
	RedisClient       *redis.Client
}

// DataStorageConfig contains configuration for the Data Storage Service
type DataStorageConfig struct {
	PostgresPort string // Default: "5433"
	RedisPort    string // Default: "6380"
	ServicePort  string // Default: "8085"
	DBName       string // Default: "action_history"
	DBUser       string // Default: "slm_user"
	DBPassword   string // Default: "test_password"
}

// DefaultDataStorageConfig returns default configuration
func DefaultDataStorageConfig() *DataStorageConfig {
	return &DataStorageConfig{
		PostgresPort: "5433",
		RedisPort:    "6380",
		ServicePort:  "8085",
		DBName:       "action_history",
		DBUser:       "slm_user",
		DBPassword:   "test_password",
	}
}

// StartDataStorageInfrastructure starts all Data Storage Service infrastructure
// Returns an infrastructure handle that can be used to stop the services
func StartDataStorageInfrastructure(cfg *DataStorageConfig, writer io.Writer) (*DataStorageInfrastructure, error) {
	if cfg == nil {
		cfg = DefaultDataStorageConfig()
	}

	infra := &DataStorageInfrastructure{
		PostgresContainer: "datastorage-postgres-test",
		RedisContainer:    "datastorage-redis-test",
		ServiceContainer:  "datastorage-service-test",
		ServiceURL:        fmt.Sprintf("http://localhost:%s", cfg.ServicePort),
	}

	fmt.Fprintln(writer, "🔧 Setting up Data Storage Service infrastructure (ADR-016: Podman)")

	// 1. Start PostgreSQL
	fmt.Fprintln(writer, "📦 Starting PostgreSQL container...")
	if err := startPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// 2. Start Redis
	fmt.Fprintln(writer, "📦 Starting Redis container...")
	if err := startRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start Redis: %w", err)
	}

	// 3. Connect to PostgreSQL
	fmt.Fprintln(writer, "🔌 Connecting to PostgreSQL...")
	if err := connectPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// 4. Apply migrations
	fmt.Fprintln(writer, "📋 Applying schema migrations...")
	if err := applyMigrations(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Connect to Redis
	fmt.Fprintln(writer, "🔌 Connecting to Redis...")
	if err := connectRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// 6. Create config files
	fmt.Fprintln(writer, "📝 Creating ADR-030 config files...")
	if err := createConfigFiles(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to create config files: %w", err)
	}

	// 7. Build Data Storage Service image
	fmt.Fprintln(writer, "🏗️  Building Data Storage Service image...")
	if err := buildDataStorageService(writer); err != nil {
		return nil, fmt.Errorf("failed to build service: %w", err)
	}

	// 8. Start Data Storage Service
	fmt.Fprintln(writer, "🚀 Starting Data Storage Service container...")
	if err := startDataStorageService(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start service: %w", err)
	}

	// 9. Wait for service to be ready
	fmt.Fprintln(writer, "⏳ Waiting for Data Storage Service to be ready...")
	if err := waitForServiceReady(infra, writer); err != nil {
		return nil, fmt.Errorf("service not ready: %w", err)
	}

	fmt.Fprintln(writer, "✅ Data Storage Service infrastructure ready!")
	return infra, nil
}

// StopDataStorageInfrastructure stops all Data Storage Service infrastructure
func (infra *DataStorageInfrastructure) Stop(writer io.Writer) {
	fmt.Fprintln(writer, "🧹 Cleaning up Data Storage Service infrastructure...")

	// Close connections
	if infra.DB != nil {
		infra.DB.Close()
	}
	if infra.RedisClient != nil {
		infra.RedisClient.Close()
	}

	// Stop and remove containers
	exec.Command("podman", "stop", infra.ServiceContainer).Run()
	exec.Command("podman", "rm", infra.ServiceContainer).Run()
	exec.Command("podman", "stop", infra.PostgresContainer).Run()
	exec.Command("podman", "rm", infra.PostgresContainer).Run()
	exec.Command("podman", "stop", infra.RedisContainer).Run()
	exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Remove config directory
	if infra.ConfigDir != "" {
		os.RemoveAll(infra.ConfigDir)
	}

	fmt.Fprintln(writer, "✅ Data Storage Service infrastructure cleanup complete")
}

// Helper functions

func startPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	exec.Command("podman", "stop", infra.PostgresContainer).Run()
	exec.Command("podman", "rm", infra.PostgresContainer).Run()

	// Start PostgreSQL with pgvector
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.PostgresContainer,
		"-p", fmt.Sprintf("%s:5432", cfg.PostgresPort),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", cfg.DBName),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", cfg.DBUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", cfg.DBPassword),
		"pgvector/pgvector:pg16")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Failed to start PostgreSQL: %s\n", output)
		return fmt.Errorf("PostgreSQL container failed to start: %w", err)
	}

	// Wait for PostgreSQL ready
	fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL to be ready...")
	time.Sleep(3 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.PostgresContainer, "pg_isready", "-U", cfg.DBUser)
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	fmt.Fprintln(writer, "  ✅ PostgreSQL started successfully")
	return nil
}

func startRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	exec.Command("podman", "stop", infra.RedisContainer).Run()
	exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Start Redis
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.RedisContainer,
		"-p", fmt.Sprintf("%s:6379", cfg.RedisPort),
		"redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Failed to start Redis: %s\n", output)
		return fmt.Errorf("Redis container failed to start: %w", err)
	}

	// Wait for Redis ready
	time.Sleep(2 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.RedisContainer, "redis-cli", "ping")
		testOutput, err := testCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Redis not ready: %v, output: %s", err, string(testOutput))
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	fmt.Fprintln(writer, "  ✅ Redis started successfully")
	return nil
}

func connectPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	connStr := fmt.Sprintf("host=localhost port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.PostgresPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	var err error
	infra.DB, err = sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Wait for connection
	Eventually(func() error {
		return infra.DB.Ping()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be connectable")

	fmt.Fprintln(writer, "  ✅ PostgreSQL connection established")
	return nil
}

func applyMigrations(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Drop and recreate schema
	fmt.Fprintln(writer, "  🗑️  Dropping existing schema...")
	_, err := infra.DB.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	// Enable pgvector extension
	fmt.Fprintln(writer, "  🔌 Enabling pgvector extension...")
	_, err = infra.DB.Exec("CREATE EXTENSION IF NOT EXISTS vector;")
	if err != nil {
		return fmt.Errorf("failed to enable pgvector: %w", err)
	}

	// Apply migrations
	fmt.Fprintln(writer, "  📜 Applying all migrations in order...")
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
		"004_add_effectiveness_assessment_due.sql",
		"005_vector_schema.sql",
		"006_effectiveness_assessment.sql",
		"009_update_vector_dimensions.sql",
		"007_add_context_column.sql",
		"008_context_api_compatibility.sql",
		"010_audit_write_api_phase1.sql",
		"011_rename_alert_to_signal.sql",
		"012_adr033_multidimensional_tracking.sql",
		"999_add_nov_2025_partition.sql",
	}

	for _, migration := range migrations {
		// Try multiple paths to find migrations (supports running from different directories)
		migrationPaths := []string{
			filepath.Join("migrations", migration),                   // From workspace root
			filepath.Join("..", "..", "..", "migrations", migration), // From test/integration/contextapi/
			filepath.Join("..", "..", "migrations", migration),       // From test/integration/
		}

		var content []byte
		var err error
		found := false
		for _, migrationPath := range migrationPaths {
			content, err = os.ReadFile(migrationPath)
			if err == nil {
				found = true
				break
			}
		}

		if !found {
			fmt.Fprintf(writer, "  ❌ Migration file not found: %v\n", err)
			return fmt.Errorf("migration file %s not found: %w", migration, err)
		}

		// Remove CONCURRENTLY keyword for test environment
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		// Extract only the UP migration (ignore DOWN section)
		if strings.Contains(migrationSQL, "-- +goose Down") {
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

		_, err = infra.DB.Exec(migrationSQL)
		if err != nil {
			fmt.Fprintf(writer, "  ❌ Migration %s failed: %v\n", migration, err)
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}
		fmt.Fprintf(writer, "  ✅ Applied %s\n", migration)
	}

	// Grant permissions
	fmt.Fprintln(writer, "  🔐 Granting permissions...")
	_, err = infra.DB.Exec(`
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`)
	if err != nil {
		return fmt.Errorf("failed to grant permissions: %w", err)
	}

	// Wait for schema propagation
	fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL schema propagation (2s)...")
	time.Sleep(2 * time.Second)

	fmt.Fprintln(writer, "  ✅ All migrations applied successfully")
	return nil
}

func connectRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	infra.RedisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("localhost:%s", cfg.RedisPort),
		DB:   0,
	})

	// Verify connection
	err := infra.RedisClient.Ping(context.Background()).Err()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Redis connection established")
	return nil
}

func createConfigFiles(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	var err error
	infra.ConfigDir, err = os.MkdirTemp("", "datastorage-config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Get container IPs
	postgresIP := getContainerIP(infra.PostgresContainer)
	redisIP := getContainerIP(infra.RedisContainer)

	// Create config.yaml (ADR-030)
	configYAML := fmt.Sprintf(`
service:
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
  name: %s
  user: %s
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
`, postgresIP, cfg.DBName, cfg.DBUser, redisIP)

	configPath := filepath.Join(infra.ConfigDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	// Create database secrets file
	dbSecretsYAML := fmt.Sprintf(`
username: %s
password: %s
`, cfg.DBUser, cfg.DBPassword)
	dbSecretsPath := filepath.Join(infra.ConfigDir, "db-secrets.yaml")
	err = os.WriteFile(dbSecretsPath, []byte(dbSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write db-secrets.yaml: %w", err)
	}

	// Create Redis secrets file
	redisSecretsYAML := `password: ""` // Redis without auth in test
	redisSecretsPath := filepath.Join(infra.ConfigDir, "redis-secrets.yaml")
	err = os.WriteFile(redisSecretsPath, []byte(redisSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write redis-secrets.yaml: %w", err)
	}

	fmt.Fprintf(writer, "  ✅ Config files created in %s\n", infra.ConfigDir)
	return nil
}

func buildDataStorageService(writer io.Writer) error {
	// Find workspace root (go.mod location)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Cleanup any existing image
	exec.Command("podman", "rmi", "-f", "data-storage:test").Run()

	// Build image for ARM64 (local testing on Apple Silicon)
	buildCmd := exec.Command("podman", "build",
		"--build-arg", "GOARCH=arm64",
		"-t", "data-storage:test",
		"-f", "docker/data-storage.Dockerfile",
		".")
	buildCmd.Dir = workspaceRoot // Run from workspace root

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Build output:\n%s\n", string(output))
		return fmt.Errorf("failed to build Data Storage Service image: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Data Storage Service image built successfully")
	return nil
}

// findWorkspaceRoot finds the workspace root by looking for go.mod
func findWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the directory tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func startDataStorageService(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	exec.Command("podman", "stop", infra.ServiceContainer).Run()
	exec.Command("podman", "rm", infra.ServiceContainer).Run()

	// Mount config files (ADR-030)
	configMount := fmt.Sprintf("%s/config.yaml:/etc/datastorage/config.yaml:ro", infra.ConfigDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", infra.ConfigDir)

	// Start service container with ADR-030 config
	startCmd := exec.Command("podman", "run", "-d",
		"--name", infra.ServiceContainer,
		"-p", fmt.Sprintf("%s:8080", cfg.ServicePort),
		"-v", configMount,
		"-v", secretsMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"data-storage:test")

	output, err := startCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Start output:\n%s\n", string(output))
		return fmt.Errorf("failed to start Data Storage Service container: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Data Storage Service container started")
	return nil
}

func waitForServiceReady(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Wait up to 30 seconds for service to be ready
	var lastStatusCode int
	var lastError error

	Eventually(func() int {
		resp, err := http.Get(infra.ServiceURL + "/health")
		if err != nil {
			lastError = err
			lastStatusCode = 0
			fmt.Fprintf(writer, "    Health check attempt failed: %v\n", err)
			return 0
		}
		if resp == nil {
			lastStatusCode = 0
			return 0
		}
		defer resp.Body.Close()
		lastStatusCode = resp.StatusCode
		if lastStatusCode != 200 {
			fmt.Fprintf(writer, "    Health check returned status %d (expected 200)\n", lastStatusCode)
		}
		return lastStatusCode
	}, "30s", "1s").Should(Equal(200), "Data Storage Service should be healthy")

	// If we got here and status is not 200, print diagnostics
	if lastStatusCode != 200 {
		fmt.Fprintf(writer, "\n❌ Data Storage Service health check failed\n")
		fmt.Fprintf(writer, "  Last status code: %d\n", lastStatusCode)
		if lastError != nil {
			fmt.Fprintf(writer, "  Last error: %v\n", lastError)
		}

		// Print container logs for debugging
		logs, logErr := exec.Command("podman", "logs", "--tail", "200", infra.ServiceContainer).CombinedOutput()
		if logErr == nil {
			fmt.Fprintf(writer, "\n📋 Data Storage Service logs (last 200 lines):\n%s\n", string(logs))
		}

		// Check if container is running
		statusCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", infra.ServiceContainer), "--format", "{{.Status}}")
		statusOutput, _ := statusCmd.CombinedOutput()
		fmt.Fprintf(writer, "  Container status: %s\n", strings.TrimSpace(string(statusOutput)))
	}

	fmt.Fprintf(writer, "  ✅ Data Storage Service ready at %s\n", infra.ServiceURL)
	return nil
}

func getContainerIP(containerName string) string {
	cmd := exec.Command("podman", "inspect", "-f", "{{.NetworkSettings.IPAddress}}", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to get IP for container %s: %v", containerName, err))
	}
	ip := strings.TrimSpace(string(output))
	if ip == "" {
		panic(fmt.Sprintf("Container %s has no IP address", containerName))
	}
	return ip
}
