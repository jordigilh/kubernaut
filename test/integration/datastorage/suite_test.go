package datastorage

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // DD-010: Migrated from lib/pq
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// DATA STORAGE INTEGRATION TESTS (TDD RED Phase)
// 📋 Tests Define Contract: Infrastructure setup required
// Authority: IMPLEMENTATION_PLAN_V4.8.md Day 7
// ========================================
//
// This file defines the integration test infrastructure contract.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (this file)
// - Infrastructure implemented SECOND (BeforeSuite/AfterSuite)
// - Contract: Real PostgreSQL + Redis required
//
// Business Requirements:
// - BR-STORAGE-001 to BR-STORAGE-020: Validate audit write API
// - DD-009: DLQ fallback validation
//
// ADR-016 Compliance: Podman for stateless services (not Kind)
//
// ========================================

func TestDataStorageIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Integration Suite (ADR-016: Podman PostgreSQL + Redis)")
}

var (
	db                *sql.DB
	redisClient       *redis.Client
	repo              *repository.NotificationAuditRepository
	dlqClient         *dlq.Client
	logger            *zap.Logger
	ctx               context.Context
	cancel            context.CancelFunc
	postgresContainer = "datastorage-postgres-test"
	redisContainer    = "datastorage-redis-test"
	serviceContainer  = "datastorage-service-test"
	datastorageURL    string
	configDir         string // ADR-030: Directory for config and secret files
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Setup logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("🔧 Setting up Podman infrastructure (ADR-016: stateless service)")

	// 1. Start PostgreSQL with pgvector
	GinkgoWriter.Println("📦 Starting PostgreSQL container...")
	startPostgreSQL()

	// 2. Start Redis for DLQ
	GinkgoWriter.Println("📦 Starting Redis container...")
	startRedis()

	// 3. Connect to PostgreSQL
	GinkgoWriter.Println("🔌 Connecting to PostgreSQL...")
	connectPostgreSQL()

	// 4. Apply schema with propagation handling
	GinkgoWriter.Println("📋 Applying schema migrations...")
	applyMigrationsWithPropagation()

	// 5. Connect to Redis
	GinkgoWriter.Println("🔌 Connecting to Redis...")
	connectRedis()

	// 6. Create repository and DLQ client instances
	GinkgoWriter.Println("🏗️  Creating repository and DLQ client...")
	repo = repository.NewNotificationAuditRepository(db, logger)
	dlqClient = dlq.NewClient(redisClient, logger)

	// 7. Create ADR-030 config files
	GinkgoWriter.Println("📝 Creating ADR-030 config and secret files...")
	createConfigFiles()

	// 8. Build and start Data Storage Service container (HTTP API)
	GinkgoWriter.Println("🏗️  Building Data Storage Service image (ADR-027)...")
	buildDataStorageService()

	GinkgoWriter.Println("🚀 Starting Data Storage Service container...")
	startDataStorageService()

	GinkgoWriter.Println("⏳ Waiting for Data Storage Service to be ready...")
	waitForServiceReady()

	GinkgoWriter.Println("✅ Infrastructure ready!")
})

var _ = AfterSuite(func() {
	GinkgoWriter.Println("🧹 Cleaning up Podman containers...")

	if cancel != nil {
		cancel()
	}

	if db != nil {
		db.Close()
	}

	if redisClient != nil {
		redisClient.Close()
	}

	// Stop and remove containers
	exec.Command("podman", "stop", serviceContainer).Run()
	exec.Command("podman", "rm", serviceContainer).Run()
	exec.Command("podman", "stop", postgresContainer).Run()
	exec.Command("podman", "rm", postgresContainer).Run()
	exec.Command("podman", "stop", redisContainer).Run()
	exec.Command("podman", "rm", redisContainer).Run()

	// Remove config directory
	if configDir != "" {
		os.RemoveAll(configDir)
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})

// startPostgreSQL starts PostgreSQL container with pgvector
func startPostgreSQL() {
	// Cleanup existing container
	exec.Command("podman", "stop", postgresContainer).Run()
	exec.Command("podman", "rm", postgresContainer).Run()

	// Start PostgreSQL with pgvector
	cmd := exec.Command("podman", "run", "-d",
		"--name", postgresContainer,
		"-p", "5433:5432",
		"-e", "POSTGRES_DB=action_history",
		"-e", "POSTGRES_USER=slm_user",
		"-e", "POSTGRES_PASSWORD=test_password",
		"pgvector/pgvector:pg16")

	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to start PostgreSQL: %s\n", output)
		Fail(fmt.Sprintf("PostgreSQL container failed to start: %v", err))
	}

	// Wait for PostgreSQL ready
	GinkgoWriter.Println("⏳ Waiting for PostgreSQL to be ready...")
	time.Sleep(3 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", postgresContainer, "pg_isready", "-U", "slm_user")
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	GinkgoWriter.Println("✅ PostgreSQL started successfully")
}

// startRedis starts Redis container for DLQ
func startRedis() {
	// Cleanup existing container
	exec.Command("podman", "stop", redisContainer).Run()
	exec.Command("podman", "rm", redisContainer).Run()

	// Start Redis
	cmd := exec.Command("podman", "run", "-d",
		"--name", redisContainer,
		"-p", "6379:6379",
		"redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to start Redis: %s\n", output)
		Fail(fmt.Sprintf("Redis container failed to start: %v", err))
	}

	// Wait for Redis ready
	GinkgoWriter.Println("⏳ Waiting for Redis to be ready...")
	time.Sleep(2 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", redisContainer, "redis-cli", "ping")
		output, err := testCmd.CombinedOutput()
		if err != nil || string(output) != "PONG\n" {
			return fmt.Errorf("Redis not ready: %v", err)
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	GinkgoWriter.Println("✅ Redis started successfully")
}

// connectPostgreSQL establishes PostgreSQL connection
func connectPostgreSQL() {
	connStr := "host=localhost port=5433 user=slm_user password=test_password dbname=action_history sslmode=disable"
	var err error
	db, err = sql.Open("pgx", connStr) // DD-010: Using pgx driver
	Expect(err).ToNot(HaveOccurred())

	// Verify connection
	err = db.Ping()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ PostgreSQL connection established")
}

// connectRedis establishes Redis connection
func connectRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Verify connection
	err := redisClient.Ping(ctx).Err()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ Redis connection established")
}

// applyMigrationsWithPropagation handles PostgreSQL schema propagation timing
// Context API Lesson: Schema changes not immediately visible to new connections
func applyMigrationsWithPropagation() {
	// 1. Drop and recreate schema for clean state
	GinkgoWriter.Println("  🗑️  Dropping existing schema...")
	_, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	Expect(err).ToNot(HaveOccurred())

	// 2. Enable pgvector extension BEFORE migrations
	GinkgoWriter.Println("  🔌 Enabling pgvector extension...")
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector;")
	Expect(err).ToNot(HaveOccurred())

	// 3. Apply ALL migrations in order (mirrors production)
	GinkgoWriter.Println("  📜 Applying all migrations in order...")
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
		"004_add_effectiveness_assessment_due.sql",
		"005_vector_schema.sql",
		"006_effectiveness_assessment.sql",
		"006_update_vector_dimensions.sql",
		"007_add_context_column.sql",
		"008_context_api_compatibility.sql",
		"010_audit_write_api_phase1.sql",
		"011_rename_alert_to_signal.sql",
		"999_add_nov_2025_partition.sql",
	}

	for _, migration := range migrations {
		GinkgoWriter.Printf("  📜 Applying %s...\n", migration)
		migrationPath := "../../../migrations/" + migration
		content, err := os.ReadFile(migrationPath)
		if err != nil {
			GinkgoWriter.Printf("  ❌ Migration file not found: %v\n", err)
			Fail(fmt.Sprintf("Migration file %s not found: %v", migration, err))
		}

		// Remove CONCURRENTLY keyword for test environment
		// CONCURRENTLY cannot run inside a transaction block
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		_, err = db.ExecContext(ctx, migrationSQL)
		if err != nil {
			GinkgoWriter.Printf("  ❌ Migration %s failed: %v\n", migration, err)
			Fail(fmt.Sprintf("Migration %s failed: %v", migration, err))
		}
	}
	GinkgoWriter.Println("  ✅ All migrations applied successfully")

	// 4. Grant permissions to test user
	GinkgoWriter.Println("  🔐 Granting permissions...")
	_, err = db.ExecContext(ctx, `
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`)
	Expect(err).ToNot(HaveOccurred())

	// 5. ⚠️ CRITICAL: Wait for schema propagation
	// Context API Lesson: 7+ hours debugging without this
	GinkgoWriter.Println("  ⏳ Waiting for PostgreSQL schema propagation (2s)...")
	time.Sleep(2 * time.Second)

	// 6. Verify schema using pg_class (handles partitioned tables)
	// Context API Lesson: information_schema.tables doesn't show partitioned tables
	GinkgoWriter.Println("  ✅ Verifying schema...")

	// Verify resource_action_traces exists
	verifySQL := `
		SELECT COUNT(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind IN ('r', 'p')  -- 'r' = regular, 'p' = partitioned
		  AND c.relname = 'resource_action_traces';
	`
	var count int
	err = db.QueryRowContext(ctx, verifySQL).Scan(&count)
	Expect(err).ToNot(HaveOccurred())
	Expect(count).To(Equal(1), "Expected resource_action_traces table to exist")

	// Verify notification_audit exists
	verifySQL2 := `
		SELECT COUNT(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind IN ('r', 'p')  -- 'r' = regular, 'p' = partitioned
		  AND c.relname = 'notification_audit';
	`
	err = db.QueryRowContext(ctx, verifySQL2).Scan(&count)
	Expect(err).ToNot(HaveOccurred())
	Expect(count).To(Equal(1), "Expected notification_audit table to exist")

	GinkgoWriter.Println("  ✅ Schema verification complete!")
}

// createConfigFiles creates ADR-030 compliant config and secret files
func createConfigFiles() {
	var err error
	configDir, err = os.MkdirTemp("", "datastorage-config-*")
	Expect(err).ToNot(HaveOccurred())

	// Get container IPs (will be used in config)
	postgresIP := getContainerIP(postgresContainer)
	redisIP := getContainerIP(redisContainer)

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
  level: info
  format: json
`, postgresIP, redisIP)

	configPath := filepath.Join(configDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configYAML), 0644)
	Expect(err).ToNot(HaveOccurred())

	// Create database secrets file
	dbSecretsYAML := `
username: slm_user
password: test_password
`
	dbSecretsPath := filepath.Join(configDir, "db-secrets.yaml")
	err = os.WriteFile(dbSecretsPath, []byte(dbSecretsYAML), 0644)
	Expect(err).ToNot(HaveOccurred())

	// Create Redis secrets file
	redisSecretsYAML := `password: ""` // Redis without auth in test
	redisSecretsPath := filepath.Join(configDir, "redis-secrets.yaml")
	err = os.WriteFile(redisSecretsPath, []byte(redisSecretsYAML), 0644)
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Printf("  ✅ Config files created in %s\n", configDir)
}

// buildDataStorageService builds the Data Storage Service container image
// Note: Builds for ARM64 (Apple Silicon) for local integration tests
// Production multi-arch builds handled separately
func buildDataStorageService() {
	// Cleanup any existing image
	exec.Command("podman", "rmi", "-f", "data-storage:test").Run()

	// Build image for ARM64 (local testing on Apple Silicon)
	buildCmd := exec.Command("podman", "build",
		"--build-arg", "GOARCH=arm64",
		"-t", "data-storage:test",
		"-f", "docker/data-storage.Dockerfile",
		".")
	buildCmd.Dir = "../../../" // Run from workspace root

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Build output:\n%s\n", string(output))
		Fail(fmt.Sprintf("Failed to build Data Storage Service image: %v", err))
	}

	GinkgoWriter.Println("  ✅ Data Storage Service image built successfully")
}

// startDataStorageService starts the Data Storage Service container
func startDataStorageService() {
	// Cleanup existing container
	exec.Command("podman", "stop", serviceContainer).Run()
	exec.Command("podman", "rm", serviceContainer).Run()

	// Get PostgreSQL container IP (they're on the same Podman network)
	postgresIP := getContainerIP(postgresContainer)
	redisIP := getContainerIP(redisContainer)

	GinkgoWriter.Printf("  📍 PostgreSQL IP: %s\n", postgresIP)
	GinkgoWriter.Printf("  📍 Redis IP: %s\n", redisIP)

	// Mount config files (ADR-030)
	configMount := fmt.Sprintf("%s/config.yaml:/etc/datastorage/config.yaml:ro", configDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", configDir)

	// Start service container with ADR-030 config
	startCmd := exec.Command("podman", "run", "-d",
		"--name", serviceContainer,
		"-p", "8080:8080",
		"-v", configMount,
		"-v", secretsMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"data-storage:test")

	output, err := startCmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Start output:\n%s\n", string(output))
		Fail(fmt.Sprintf("Failed to start Data Storage Service container: %v", err))
	}

	GinkgoWriter.Println("  ✅ Data Storage Service container started")
}

// getContainerIP retrieves the IP address of a Podman container
func getContainerIP(containerName string) string {
	cmd := exec.Command("podman", "inspect", "-f", "{{.NetworkSettings.IPAddress}}", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		Fail(fmt.Sprintf("Failed to get IP for container %s: %v", containerName, err))
	}
	ip := strings.TrimSpace(string(output))
	if ip == "" {
		Fail(fmt.Sprintf("Container %s has no IP address", containerName))
	}
	return ip
}

// waitForServiceReady waits for the Data Storage Service health endpoint to respond
func waitForServiceReady() {
	datastorageURL = "http://localhost:8080"

	// Wait up to 30 seconds for service to be ready
	Eventually(func() int {
		resp, err := http.Get(datastorageURL + "/health")
		if err != nil || resp == nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}, "30s", "1s").Should(Equal(200), "Data Storage Service should be healthy")

	// Print container logs for debugging (first 100 lines)
	logs, logErr := exec.Command("podman", "logs", "--tail", "100", serviceContainer).CombinedOutput()
	if logErr == nil {
		GinkgoWriter.Printf("\n📋 Data Storage Service logs:\n%s\n", string(logs))
	}

	GinkgoWriter.Printf("  ✅ Data Storage Service ready at %s\n", datastorageURL)
}
