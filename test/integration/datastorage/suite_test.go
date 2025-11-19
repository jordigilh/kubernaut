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

// generateTestID creates a unique test identifier for data isolation
// Format: test-{process}-{timestamp}
// This enables parallel test execution by ensuring each test has unique data
func generateTestID() string {
	return fmt.Sprintf("test-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
}

// cleanupContainers removes any existing test containers and networks
func cleanupContainers() {
	// Stop and remove containers (ignore errors if they don't exist)
	exec.Command("podman", "stop", serviceContainer).Run()
	exec.Command("podman", "rm", "-f", serviceContainer).Run()
	exec.Command("podman", "stop", postgresContainer).Run()
	exec.Command("podman", "rm", "-f", postgresContainer).Run()
	exec.Command("podman", "stop", redisContainer).Run()
	exec.Command("podman", "rm", "-f", redisContainer).Run()

	// Remove network (ignore error if it doesn't exist)
	exec.Command("podman", "network", "rm", "datastorage-test").Run()

	GinkgoWriter.Println("✅ Cleanup complete")
}

// SynchronizedBeforeSuite runs infrastructure setup once (process 1) and shares connection info with all processes
// This enables parallel test execution while sharing the same PostgreSQL/Redis/Service infrastructure
var _ = SynchronizedBeforeSuite(
	// Process 1: Setup shared infrastructure
	func() []byte {
		GinkgoWriter.Printf("🔧 [Process %d] Setting up shared Podman infrastructure (ADR-016)\n", GinkgoParallelProcess())

		// 0. Force cleanup of any existing containers/networks from previous runs
		if os.Getenv("POSTGRES_HOST") == "" {
			GinkgoWriter.Println("🧹 Cleaning up any existing test infrastructure...")
			cleanupContainers()
		}

		// 1. Create shared network for local execution (skip for Docker Compose)
		if os.Getenv("POSTGRES_HOST") == "" {
			GinkgoWriter.Println("🌐 Creating shared Podman network...")
			createNetwork()
		}

		// 2. Start PostgreSQL with pgvector
		GinkgoWriter.Println("📦 Starting PostgreSQL container...")
		startPostgreSQL()

		// 3. Start Redis for DLQ
		GinkgoWriter.Println("📦 Starting Redis container...")
		startRedis()

		// 4. Connect to PostgreSQL to apply migrations
		GinkgoWriter.Println("🔌 Connecting to PostgreSQL...")
		tempDB := mustConnectPostgreSQL()

		// 5. Apply schema with propagation handling
		GinkgoWriter.Println("📋 Applying schema migrations...")
		applyMigrationsWithPropagationTo(tempDB)
		tempDB.Close()

		// 6. Setup Data Storage Service
		var serviceURL string
		if os.Getenv("DATASTORAGE_URL") != "" {
			// Docker Compose environment - use external service
			serviceURL = os.Getenv("DATASTORAGE_URL")
			GinkgoWriter.Printf("🐳 Using external Data Storage Service at %s\n", serviceURL)
		} else {
			// Local execution - build and start our own container
			GinkgoWriter.Println("📝 Creating ADR-030 config and secret files...")
			createConfigFiles()

			GinkgoWriter.Println("🏗️  Building Data Storage Service image (ADR-027)...")
			buildDataStorageService()

			GinkgoWriter.Println("🚀 Starting Data Storage Service container...")
			startDataStorageService()

			GinkgoWriter.Println("⏳ Waiting for Data Storage Service to be ready...")
			waitForServiceReady()

			// Determine service URL based on environment
			port := "8080"
			if p := os.Getenv("DATASTORAGE_PORT"); p != "" {
				port = p
			}
			serviceURL = fmt.Sprintf("http://localhost:%s", port)
		}

		GinkgoWriter.Println("✅ Infrastructure ready!")

		// Return connection info to all processes
		return []byte(serviceURL)
	},
	// All processes: Connect to shared infrastructure
	func(data []byte) {
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to shared infrastructure\n", GinkgoParallelProcess())

		ctx, cancel = context.WithCancel(context.Background())

		// Setup logger
		var err error
		logger, err = zap.NewDevelopment()
		Expect(err).ToNot(HaveOccurred())

		// Parse service URL from process 1
		datastorageURL = string(data)

		// Connect to PostgreSQL
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to PostgreSQL...\n", GinkgoParallelProcess())
		connectPostgreSQL()

		// Connect to Redis
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to Redis...\n", GinkgoParallelProcess())
		connectRedis()

		// Create repository and DLQ client instances
		GinkgoWriter.Printf("🏗️  [Process %d] Creating repository and DLQ client...\n", GinkgoParallelProcess())
		repo = repository.NewNotificationAuditRepository(db, logger)
		dlqClient = dlq.NewClient(redisClient, logger)

		GinkgoWriter.Printf("✅ [Process %d] Ready to run tests!\n", GinkgoParallelProcess())
	},
)

var _ = AfterSuite(func() {
	GinkgoWriter.Println("🧹 Cleaning up Podman containers...")

	// Capture service logs before cleanup for debugging
	if serviceContainer != "" {
		logs, err := exec.Command("podman", "logs", "--tail", "200", serviceContainer).CombinedOutput()
		if err == nil && len(logs) > 0 {
			GinkgoWriter.Printf("\n📋 Final Data Storage Service logs (last 200 lines):\n%s\n", string(logs))
		} else {
			GinkgoWriter.Printf("\n⚠️  Failed to get final service logs: %v\n", err)
		}

		// Check container status
		status, err := exec.Command("podman", "inspect", "-f", "{{.State.Status}}", serviceContainer).CombinedOutput()
		if err == nil {
			GinkgoWriter.Printf("📊 Container status: %s\n", strings.TrimSpace(string(status)))
		}
	}

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

	// Remove network (only for local execution)
	if os.Getenv("POSTGRES_HOST") == "" {
		exec.Command("podman", "network", "rm", "datastorage-test").Run()
	}

	// Remove config directory
	if configDir != "" {
		os.RemoveAll(configDir)
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})

// createNetwork creates a shared Podman network for container-to-container communication
// This allows the Data Storage Service container to connect to PostgreSQL and Redis by container name
// Works on both Linux and macOS (Podman Machine)
func createNetwork() {
	// Remove existing network if it exists
	exec.Command("podman", "network", "rm", "datastorage-test").Run()

	// Create new network
	cmd := exec.Command("podman", "network", "create", "datastorage-test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to create network: %s\n", output)
		Fail(fmt.Sprintf("Network creation failed: %v", err))
	}

	GinkgoWriter.Println("✅ Podman network created")
}

// startPostgreSQL starts PostgreSQL container with pgvector
// When POSTGRES_HOST is set (e.g., in Docker Compose), skip container creation
func startPostgreSQL() {
	// Check if running in Docker Compose environment
	if os.Getenv("POSTGRES_HOST") != "" {
		GinkgoWriter.Println("🐳 Using external PostgreSQL (Docker Compose)")
		// Wait for PostgreSQL to be ready via TCP connection
		host := os.Getenv("POSTGRES_HOST")
		port := os.Getenv("POSTGRES_PORT")
		if port == "" {
			port = "5432"
		}

		GinkgoWriter.Printf("⏳ Waiting for PostgreSQL at %s:%s to be ready...\n", host, port)
		Eventually(func() error {
			connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", host, port)
			testDB, err := sql.Open("pgx", connStr)
			if err != nil {
				return err
			}
			defer testDB.Close()
			return testDB.Ping()
		}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

		GinkgoWriter.Println("✅ PostgreSQL is ready")
		return
	}

	// Running locally - start our own container
	GinkgoWriter.Println("🏠 Starting local PostgreSQL container...")

	// Cleanup existing container
	exec.Command("podman", "stop", postgresContainer).Run()
	exec.Command("podman", "rm", postgresContainer).Run()

	// Start PostgreSQL with pgvector
	// Use --network=datastorage-test for container-to-container communication
	// Increase max_connections for parallel test execution (default is 100)
	cmd := exec.Command("podman", "run", "-d",
		"--name", postgresContainer,
		"--network", "datastorage-test",
		"-p", "5433:5432",
		"-e", "POSTGRES_DB=action_history",
		"-e", "POSTGRES_USER=slm_user",
		"-e", "POSTGRES_PASSWORD=test_password",
		"quay.io/jordigilh/pgvector:pg16",
		"-c", "max_connections=200")

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
// When REDIS_HOST is set (e.g., in Docker Compose), skip container creation
func startRedis() {
	// Check if running in Docker Compose environment
	if os.Getenv("REDIS_HOST") != "" {
		GinkgoWriter.Println("🐳 Using external Redis (Docker Compose)")
		// Wait for Redis to be ready via TCP connection
		host := os.Getenv("REDIS_HOST")
		port := os.Getenv("REDIS_PORT")
		if port == "" {
			port = "6379"
		}

		GinkgoWriter.Printf("⏳ Waiting for Redis at %s:%s to be ready...\n", host, port)
		Eventually(func() error {
			addr := fmt.Sprintf("%s:%s", host, port)
			testClient := redis.NewClient(&redis.Options{
				Addr: addr,
			})
			defer testClient.Close()
			return testClient.Ping(ctx).Err()
		}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

		GinkgoWriter.Println("✅ Redis is ready")
		return
	}

	// Running locally - start our own container
	GinkgoWriter.Println("🏠 Starting local Redis container...")

	// Cleanup existing container
	exec.Command("podman", "stop", redisContainer).Run()
	exec.Command("podman", "rm", redisContainer).Run()

	// Start Redis
	// Use --network=datastorage-test for container-to-container communication
	cmd := exec.Command("podman", "run", "-d",
		"--name", redisContainer,
		"--network", "datastorage-test",
		"-p", "6379:6379",
		"quay.io/jordigilh/redis:7-alpine")

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
// mustConnectPostgreSQL creates a new database connection (for process 1 setup)
// Returns the connection so it can be closed after migrations
func mustConnectPostgreSQL() *sql.DB {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5433"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", host, port)
	tempDB, err := sql.Open("pgx", connStr)
	Expect(err).ToNot(HaveOccurred())

	// Configure connection pool for parallel execution
	tempDB.SetMaxOpenConns(50)
	tempDB.SetMaxIdleConns(10)
	tempDB.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	err = tempDB.Ping()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ PostgreSQL connection established (pool: max_open=50)")
	return tempDB
}

func connectPostgreSQL() {
	// Use environment variables for Docker Compose compatibility
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5433"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", host, port)
	var err error
	db, err = sql.Open("pgx", connStr) // DD-010: Using pgx driver
	Expect(err).ToNot(HaveOccurred())

	// Configure connection pool for parallel execution
	// Default is 2 max open connections, which is insufficient for parallel tests
	db.SetMaxOpenConns(50)                 // Allow up to 50 concurrent connections (4 procs * 10 tests)
	db.SetMaxIdleConns(10)                 // Keep 10 idle connections ready
	db.SetConnMaxLifetime(5 * time.Minute) // Recycle connections every 5 minutes

	// Verify connection
	err = db.Ping()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Printf("✅ PostgreSQL connection established (pool: max_open=50, max_idle=10)\n")
}

// connectRedis establishes Redis connection
func connectRedis() {
	// Use environment variables for Docker Compose compatibility
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", host, port),
	})

	// Verify connection
	err := redisClient.Ping(ctx).Err()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ Redis connection established")
}

// applyMigrationsWithPropagationTo applies migrations to a specific database connection
// Used by SynchronizedBeforeSuite process 1 to setup schema once
func applyMigrationsWithPropagationTo(targetDB *sql.DB) {
	ctx := context.Background()

	// 1. Drop and recreate schema for clean state
	GinkgoWriter.Println("  🗑️  Dropping existing schema...")
	_, err := targetDB.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	Expect(err).ToNot(HaveOccurred())

	// 2. Enable pgvector extension BEFORE migrations
	GinkgoWriter.Println("  🔌 Enabling pgvector extension...")
	_, err = targetDB.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector;")
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
		"009_update_vector_dimensions.sql",
		"007_add_context_column.sql",
		"008_context_api_compatibility.sql",
		"010_audit_write_api_phase1.sql",
		"011_rename_alert_to_signal.sql",
		"012_adr033_multidimensional_tracking.sql", // ADR-033: Multi-dimensional success tracking
		"013_create_audit_events_table.sql",        // ADR-034: Unified audit events table
		"999_add_nov_2025_partition.sql",           // Legacy partition for resource_action_traces
		"1000_create_audit_events_partitions.sql",  // ADR-034: audit_events partitions (Nov 2025 - Feb 2026)
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

		// Extract only the UP migration (ignore DOWN section)
		// Goose migrations have "-- +goose Up" and "-- +goose Down" markers
		if strings.Contains(migrationSQL, "-- +goose Down") {
			// Split at the DOWN marker and only use the UP part
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

		_, err = targetDB.ExecContext(ctx, migrationSQL)
		if err != nil {
			GinkgoWriter.Printf("  ❌ Migration %s failed: %v\n", migration, err)
			Fail(fmt.Sprintf("Migration %s failed: %v", migration, err))
		}
	}

	GinkgoWriter.Println("  ✅ All migrations applied successfully")

	// 4. Wait for schema propagation (Context API lesson)
	// PostgreSQL needs time to propagate schema changes to new connections
	GinkgoWriter.Println("  ⏳ Waiting for schema propagation...")
	time.Sleep(500 * time.Millisecond)
	GinkgoWriter.Println("  ✅ Schema propagation complete")
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
		"009_update_vector_dimensions.sql",
		"007_add_context_column.sql",
		"008_context_api_compatibility.sql",
		"010_audit_write_api_phase1.sql",
		"011_rename_alert_to_signal.sql",
		"012_adr033_multidimensional_tracking.sql", // ADR-033: Multi-dimensional success tracking
		"013_create_audit_events_table.sql",        // ADR-034: Unified audit events table
		"999_add_nov_2025_partition.sql",           // Legacy partition for resource_action_traces
		"1000_create_audit_events_partitions.sql",  // ADR-034: audit_events partitions (Nov 2025 - Feb 2026)
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

		// Extract only the UP migration (ignore DOWN section)
		// Goose migrations have "-- +goose Up" and "-- +goose Down" markers
		if strings.Contains(migrationSQL, "-- +goose Down") {
			// Split at the DOWN marker and only use the UP part
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

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

	// Determine database and redis hosts based on environment
	// Docker Compose: Use service names (postgres, redis)
	// Direct execution: Use container names on shared network (datastorage-postgres-test, datastorage-redis-test)
	dbHost := os.Getenv("POSTGRES_HOST")
	if dbHost == "" {
		dbHost = postgresContainer // Use container name for container-to-container communication
	}
	dbPort := os.Getenv("POSTGRES_PORT")
	if dbPort == "" {
		dbPort = "5432" // Use internal port (not mapped port)
	}

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = redisContainer // Use container name for container-to-container communication
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

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
  port: %s
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
  addr: %s:%s
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
`, dbHost, dbPort, redisHost, redisPort)

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

	// Mount config files (ADR-030)
	configMount := fmt.Sprintf("%s/config.yaml:/etc/datastorage/config.yaml:ro", configDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", configDir)

	// Start service container with ADR-030 config
	// Use --network=datastorage-test for container-to-container communication
	// Port mapping allows host to access service on localhost:8080
	startCmd := exec.Command("podman", "run", "-d",
		"--name", serviceContainer,
		"--network", "datastorage-test",
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
