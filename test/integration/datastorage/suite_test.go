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

package datastorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // DD-010: Migrated from lib/pq
	"github.com/jmoiron/sqlx"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
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
	db                *sqlx.DB // Changed from *sql.DB to support workflow repository
	redisClient       *redis.Client
	repo              *repository.NotificationAuditRepository
	dlqClient         *dlq.Client
	logger            logr.Logger
	ctx               context.Context
	cancel            context.CancelFunc
	postgresContainer = "datastorage-postgres-test"
	redisContainer    = "datastorage-redis-test"
	serviceContainer  = "datastorage-service-test"
	datastorageURL    string
	configDir         string // ADR-030: Directory for config and secret files
	schemaName        string // Schema name for this parallel process (for isolation)

	// BR-STORAGE-014: Embedding service integration
	embeddingServer *httptest.Server // Mock embedding service
	embeddingClient embedding.Client
)

// generateTestID creates a unique test identifier for data isolation
// Format: test-{process}-{timestamp}
// This enables parallel test execution by ensuring each test has unique data
func generateTestID() string {
	return fmt.Sprintf("test-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
}

// generateTestUUID creates a unique UUID for test data isolation
// Used for audit events and other UUID-based records
func generateTestUUID() uuid.UUID {
	return uuid.New()
}

// createProcessSchema creates a process-specific PostgreSQL schema for test isolation
// This enables parallel test execution without data interference between processes
// Each parallel process gets its own schema (e.g., test_process_1, test_process_2)
//
// Strategy:
// 1. Create empty schema
// 2. Copy table structure from public schema (without data)
// 3. Set search_path to use this schema
//
// Note: Extensions (pgvector, uuid-ossp) are database-wide and already created in public schema
func createProcessSchema(db *sqlx.DB, processNum int) (string, error) {
	schemaName := fmt.Sprintf("test_process_%d", processNum)

	GinkgoWriter.Printf("🏗️  [Process %d] Creating schema: %s\n", processNum, schemaName)

	// Drop schema if it exists (cleanup from previous failed runs)
	_, err := db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	if err != nil {
		return "", fmt.Errorf("failed to drop existing schema: %w", err)
	}

	// Create new schema
	_, err = db.Exec(fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	if err != nil {
		return "", fmt.Errorf("failed to create schema: %w", err)
	}

	// Copy all table structures from public schema to this schema (without data)
	// This includes: audit_events, remediation_workflow_catalog, action_trace, etc.
	GinkgoWriter.Printf("📋 [Process %d] Copying table structures from public schema...\n", processNum)

	// Get list of all tables in public schema
	rows, err := db.Query(`
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename NOT LIKE 'pg_%'
		AND tablename NOT LIKE 'sql_%'
		ORDER BY tablename
	`)
	if err != nil {
		return "", fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return "", fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	// Copy each table structure (CREATE TABLE LIKE)
	for _, tableName := range tables {
		// Use CREATE TABLE ... (LIKE ...) to copy structure including indexes
		query := fmt.Sprintf(
			"CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)",
			schemaName, tableName, tableName,
		)
		_, err = db.Exec(query)
		if err != nil {
			return "", fmt.Errorf("failed to copy table %s: %w", tableName, err)
		}
		GinkgoWriter.Printf("  ✅ [Process %d] Copied table: %s\n", processNum, tableName)
	}

	// Set search_path to use this schema
	_, err = db.Exec(fmt.Sprintf("SET search_path TO %s", schemaName))
	if err != nil {
		return "", fmt.Errorf("failed to set search_path: %w", err)
	}

	GinkgoWriter.Printf("✅ [Process %d] Schema created with %d tables, search_path set: %s\n", processNum, len(tables), schemaName)
	return schemaName, nil
}

// dropProcessSchema drops the process-specific schema (cleanup)
func dropProcessSchema(db *sqlx.DB, schemaName string) error {
	if schemaName == "" || schemaName == "public" {
		// Safety check: never drop public schema
		return nil
	}

	GinkgoWriter.Printf("🗑️  Dropping schema: %s\n", schemaName)
	_, err := db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	if err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	GinkgoWriter.Printf("✅ Schema dropped: %s\n", schemaName)
	return nil
}

// usePublicSchema sets the search_path to public schema
// This is used by Serial tests that need to access data in the public schema
// (e.g., schema validation tests, tests that check partitions, etc.)
func usePublicSchema() {
	if db != nil {
		// Close all idle connections to force them to reconnect with new search_path
		db.SetMaxIdleConns(0)

		// Set search_path for all future connections
		_, err := db.Exec("SET search_path TO public")
		if err != nil {
			GinkgoWriter.Printf("⚠️  Failed to set search_path to public: %v\n", err)
		}

		// Restore idle connection pool
		db.SetMaxIdleConns(10)

		GinkgoWriter.Printf("🔄 Set search_path to public schema\n")
	}
}

// preflightCheck validates the test environment before running tests
// This ensures we have a clean slate and prevents test failures due to corrupted data
func preflightCheck() error {
	GinkgoWriter.Println("🔍 Running preflight checks...")

	// 1. Check if podman is available
	if err := exec.Command("podman", "version").Run(); err != nil {
		return fmt.Errorf("❌ Podman not available: %w", err)
	}
	GinkgoWriter.Println("  ✅ Podman is available")

	// 2. Check for stale containers from previous runs
	cmd := exec.Command("podman", "ps", "-a", "--filter", "name=datastorage-", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		GinkgoWriter.Printf("  ⚠️  Found stale containers from previous runs:\n%s", string(output))
		GinkgoWriter.Println("  🧹 Will clean up stale containers...")
	}

	// 3. Check for stale networks
	cmd = exec.Command("podman", "network", "ls", "--filter", "name=datastorage-test", "--format", "{{.Name}}")
	output, err = cmd.Output()
	if err == nil && len(output) > 0 {
		GinkgoWriter.Printf("  ⚠️  Found stale network: %s", string(output))
		GinkgoWriter.Println("  🧹 Will clean up stale network...")
	}

	// 4. Check for port conflicts (15433 for PostgreSQL, 16379 for Redis) - DD-TEST-001
	cmd = exec.Command("sh", "-c", "lsof -i :15433 -i :16379 || true")
	output, _ = cmd.Output()
	if len(output) > 0 {
		GinkgoWriter.Printf("  ⚠️  Ports 15433 or 16379 may be in use:\n%s", string(output))
		GinkgoWriter.Println("  ⚠️  This may cause test failures if not cleaned up")
	}

	// 5. Verify we're not in a dirty state (check for running containers)
	cmd = exec.Command("podman", "ps", "--filter", "name=datastorage-", "--format", "{{.Names}}")
	output, err = cmd.Output()
	if err == nil && len(output) > 0 {
		GinkgoWriter.Printf("  ⚠️  Found running datastorage containers:\n%s", string(output))
		return fmt.Errorf("❌ Running containers detected - cleanup required")
	}

	GinkgoWriter.Println("  ✅ No running datastorage containers")
	GinkgoWriter.Println("✅ Preflight checks passed")
	return nil
}

// cleanupContainers removes any existing test containers and networks
// This is called both in preflight and after tests to ensure clean state
func cleanupContainers() {
	GinkgoWriter.Println("🧹 Cleaning up test infrastructure...")

	// Stop and remove integration test containers
	containers := []string{serviceContainer, postgresContainer, redisContainer}
	for _, container := range containers {
		// Stop container
		cmd := exec.Command("podman", "stop", container)
		if err := cmd.Run(); err == nil {
			GinkgoWriter.Printf("  🛑 Stopped container: %s\n", container)
		}

		// Remove container
		cmd = exec.Command("podman", "rm", "-f", container)
		if err := cmd.Run(); err == nil {
			GinkgoWriter.Printf("  🗑️  Removed container: %s\n", container)
		}
	}

	// Clean up ANY containers with "datastorage-" prefix (including E2E containers)
	GinkgoWriter.Println("  🔍 Checking for other datastorage containers...")
	cmd := exec.Command("podman", "ps", "-a", "--filter", "name=datastorage-", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		staleContainers := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, container := range staleContainers {
			if container != "" {
				GinkgoWriter.Printf("  🧹 Cleaning up stale container: %s\n", container)
				exec.Command("podman", "stop", container).Run()
				exec.Command("podman", "rm", "-f", container).Run()
			}
		}
	}

	// Remove network (ignore error if it doesn't exist)
	cmd = exec.Command("podman", "network", "rm", "datastorage-test")
	if err := cmd.Run(); err == nil {
		GinkgoWriter.Println("  🗑️  Removed network: datastorage-test")
	}

	// Clean up Kind clusters from E2E tests (if any)
	GinkgoWriter.Println("  🔍 Checking for Kind clusters...")
	cmd = exec.Command("kind", "get", "clusters")
	output, err = cmd.Output()
	if err == nil && len(output) > 0 {
		clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, cluster := range clusters {
			if strings.HasPrefix(cluster, "datastorage-e2e-") {
				GinkgoWriter.Printf("  🧹 Deleting Kind cluster: %s\n", cluster)
				exec.Command("kind", "delete", "cluster", "--name", cluster).Run()
			}
		}
	}

	// Wait a moment for cleanup to complete
	time.Sleep(2 * time.Second)

	GinkgoWriter.Println("✅ Cleanup complete")
}

// SynchronizedBeforeSuite runs infrastructure setup once (process 1) and shares connection info with all processes
// This enables parallel test execution while sharing the same PostgreSQL/Redis/Service infrastructure
var _ = SynchronizedBeforeSuite(
	// Process 1: Setup shared infrastructure
	func() []byte {
		GinkgoWriter.Printf("🔧 [Process %d] Setting up shared Podman infrastructure (ADR-016)\n", GinkgoParallelProcess())

		// 0. Preflight check: Validate environment and detect stale resources
		if os.Getenv("POSTGRES_HOST") == "" {
			GinkgoWriter.Println("🔍 Running preflight checks...")
			if err := preflightCheck(); err != nil {
				// If preflight fails, attempt cleanup and retry
				GinkgoWriter.Printf("⚠️  Preflight check failed: %v\n", err)
				GinkgoWriter.Println("🧹 Attempting cleanup and retry...")
				cleanupContainers()

				// Retry preflight after cleanup
				if err := preflightCheck(); err != nil {
					Fail(fmt.Sprintf("❌ Preflight check failed after cleanup: %v", err))
				}
			}
		} else {
			GinkgoWriter.Println("🔍 Skipping preflight checks (using external infrastructure)")
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

		// 5. Apply schema with propagation handling to PUBLIC schema
		// This creates extensions (database-wide) and base schema
		GinkgoWriter.Println("📋 Applying schema migrations to public schema...")
		applyMigrationsWithPropagationTo(tempDB.DB) // Use tempDB.DB to get *sql.DB from sqlx

		// Note: We keep the connection open for parallel processes to use
		// Each parallel process will create its own schema and copy the table structure
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

			// Determine service URL based on environment (DD-TEST-001)
			port := "18090"
			if p := os.Getenv("DATASTORAGE_PORT"); p != "" {
				port = p
			}
			serviceURL = fmt.Sprintf("http://localhost:%s", port)
		}

		GinkgoWriter.Println("✅ Infrastructure ready!")

		// Export environment variables for tests that create their own connections
		// This ensures all tests use the correct ports (e.g., graceful shutdown tests)
		if os.Getenv("POSTGRES_HOST") == "" {
			os.Setenv("POSTGRES_HOST", "localhost")
			os.Setenv("POSTGRES_PORT", "15433") // Mapped port from container (DD-TEST-001)
			os.Setenv("REDIS_HOST", "localhost")
			os.Setenv("REDIS_PORT", "16379") // DD-TEST-001
			GinkgoWriter.Println("📌 Exported environment variables for test infrastructure")
		}

		// Return connection info to all processes
		return []byte(serviceURL)
	},
	// All processes: Connect to shared infrastructure
	func(data []byte) {
		processNum := GinkgoParallelProcess()
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to shared infrastructure\n", processNum)

		ctx, cancel = context.WithCancel(context.Background())

		// Setup logger (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		// Declare err for use in subsequent operations
		var err error
		_ = err // Suppress unused warning until used

		// Parse service URL from process 1
		datastorageURL = string(data)

		// Connect to PostgreSQL
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to PostgreSQL...\n", processNum)
		connectPostgreSQL()

		// ========================================
		// SCHEMA-LEVEL ISOLATION FOR PARALLEL EXECUTION
		// ========================================
		// Create process-specific schema for complete test isolation
		// This prevents search query collisions between parallel processes
		// Each process gets its own schema: test_process_1, test_process_2, etc.
		// The schema is a copy of the public schema structure (tables, indexes, etc.)
		GinkgoWriter.Printf("🏗️  [Process %d] Setting up schema-level isolation...\n", processNum)
		schemaName, err = createProcessSchema(db, processNum)
		Expect(err).ToNot(HaveOccurred(), "Schema creation should succeed")

		// Connect to Redis
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to Redis...\n", processNum)
		connectRedis()

		// Create repository and DLQ client instances
		GinkgoWriter.Printf("🏗️  [Process %d] Creating repository and DLQ client...\n", processNum)
		repo = repository.NewNotificationAuditRepository(db.DB, logger) // Use db.DB to get *sql.DB from sqlx
		dlqClient, err = dlq.NewClient(redisClient, logger)
		Expect(err).ToNot(HaveOccurred(), "DLQ client creation should succeed")

		// BR-STORAGE-014: Create mock embedding service for integration tests
		GinkgoWriter.Printf("🏗️  [Process %d] Creating mock embedding service...\n", processNum)
		embeddingServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Mock 768-dimensional embedding response
			mockEmbedding := make([]float32, 768)
			for i := range mockEmbedding {
				mockEmbedding[i] = float32(i) * 0.001 // Generate deterministic values
			}

			resp := map[string]interface{}{
				"embedding":  mockEmbedding,
				"dimensions": 768,
				"model":      "all-mpnet-base-v2",
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}))

		// Create Redis cache for embeddings
		redisOpts := &redis.Options{
			Addr: fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")),
		}
		redisSharedClient := rediscache.NewClient(redisOpts, logger)
		embeddingCache := rediscache.NewCache[[]float32](redisSharedClient, "embeddings", 24*time.Hour)

		// Create embedding client
		embeddingClient = embedding.NewClient(embeddingServer.URL, embeddingCache, logger)
		GinkgoWriter.Printf("✅ [Process %d] Mock embedding service ready at %s\n", processNum, embeddingServer.URL)

		GinkgoWriter.Printf("✅ [Process %d] Ready to run tests in schema: %s\n", processNum, schemaName)
	},
)

var _ = AfterSuite(func() {
	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("🧹 [Process %d] Cleaning up...\n", processNum)

	// Clean up process-specific schema
	if db != nil && schemaName != "" {
		GinkgoWriter.Printf("🗑️  [Process %d] Dropping schema: %s\n", processNum, schemaName)
		if err := dropProcessSchema(db, schemaName); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to drop schema: %v\n", processNum, err)
		}
	}

	// Capture service logs before cleanup for debugging (only process 1)
	if processNum == 1 && serviceContainer != "" {
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

	// Clean up mock embedding server
	if embeddingServer != nil {
		embeddingServer.Close()
		GinkgoWriter.Printf("🧹 [Process %d] Closed mock embedding server\n", processNum)
	}

	// Use centralized cleanup function (only process 1)
	if processNum == 1 {
		cleanupContainers()
	}

	// Post-cleanup verification
	if os.Getenv("POSTGRES_HOST") == "" {
		GinkgoWriter.Println("🔍 Verifying cleanup...")
		cmd := exec.Command("podman", "ps", "-a", "--filter", "name=datastorage-", "--format", "{{.Names}}")
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			GinkgoWriter.Printf("⚠️  Warning: Some containers still exist after cleanup:\n%s", string(output))
		} else {
			GinkgoWriter.Println("✅ All datastorage containers cleaned up successfully")
		}
	}

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
		"-p", "15433:5432", // DD-TEST-001
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
			port = "16379" // DD-TEST-001
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
		"-p", "16379:6379", // DD-TEST-001
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
func mustConnectPostgreSQL() *sqlx.DB {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "15433" // DD-TEST-001
	}

	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", host, port)
	tempDB, err := sqlx.Connect("pgx", connStr)
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
		port = "15433" // DD-TEST-001
	}

	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", host, port)
	var err error
	db, err = sqlx.Connect("pgx", connStr) // DD-010: Using pgx driver with sqlx
	Expect(err).ToNot(HaveOccurred())

	// Configure connection pool for parallel execution
	// Default is 2 max open connections, which is insufficient for parallel tests
	db.SetMaxOpenConns(50)                 // Allow up to 50 concurrent connections (4 procs * 10 tests)
	db.SetMaxIdleConns(10)                 // Keep 10 idle connections ready
	db.SetConnMaxLifetime(5 * time.Minute) // Recycle connections every 5 minutes

	// Verify connection
	err = db.Ping()
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Printf("✅ PostgreSQL connection established with sqlx (pool: max_open=50, max_idle=10)\n")
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
		port = "16379" // DD-TEST-001
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
		"012_adr033_multidimensional_tracking.sql",           // ADR-033: Multi-dimensional success tracking
		"013_create_audit_events_table.sql",                  // ADR-034: Unified audit events table
		"015_create_workflow_catalog_table.sql",              // BR-STORAGE-012/013/014: Workflow catalog with semantic search
		"016_update_embedding_dimensions.sql",                // BR-STORAGE-014: Update to 768 dimensions (all-mpnet-base-v2)
		"017_add_workflow_schema_fields.sql",                 // ADR-043: Add parameters, execution_engine, execution_bundle
		"018_rename_execution_bundle_to_container_image.sql", // DD-WORKFLOW-002 v2.4: Rename to container_image, add container_digest
		"019_uuid_primary_key.sql",                           // DD-WORKFLOW-002 v3.0: UUID primary key, workflow_name field
		"020_add_workflow_label_columns.sql",                 // DD-WORKFLOW-001 v1.6: Add custom_labels, detected_labels columns
		"1000_create_audit_events_partitions.sql",            // ADR-034: audit_events partitions (Nov 2025 - Feb 2026)
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

	// 4. Create dynamic partitions for current month (prevents time-based test failures)
	GinkgoWriter.Println("  📅 Creating dynamic partitions for current month...")
	createDynamicPartitions(ctx, targetDB)

	// 5. Wait for schema propagation (Context API lesson)
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
		"012_adr033_multidimensional_tracking.sql",           // ADR-033: Multi-dimensional success tracking
		"013_create_audit_events_table.sql",                  // ADR-034: Unified audit events table
		"015_create_workflow_catalog_table.sql",              // BR-STORAGE-012/013/014: Workflow catalog with semantic search
		"016_update_embedding_dimensions.sql",                // BR-STORAGE-014: Update to 768 dimensions (all-mpnet-base-v2)
		"017_add_workflow_schema_fields.sql",                 // ADR-043: Add parameters, execution_engine, execution_bundle
		"018_rename_execution_bundle_to_container_image.sql", // DD-WORKFLOW-002 v2.4: Rename to container_image, add container_digest
		"019_uuid_primary_key.sql",                           // DD-WORKFLOW-002 v3.0: UUID primary key, workflow_name field
		"020_add_workflow_label_columns.sql",                 // DD-WORKFLOW-001 v1.6: Add custom_labels, detected_labels columns
		"1000_create_audit_events_partitions.sql",            // ADR-034: audit_events partitions (Nov 2025 - Feb 2026)
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

	// 4. Create dynamic partitions for current month (prevents time-based test failures)
	GinkgoWriter.Println("  📅 Creating dynamic partitions for current month...")
	createDynamicPartitions(ctx, db)

	// 5. Grant permissions to test user
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
		"-p", "18090:8080", // DD-TEST-001
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
	datastorageURL = "http://localhost:18090" // DD-TEST-001

	// Wait up to 60 seconds for service to be ready (increased for parallel execution)
	Eventually(func() int {
		resp, err := http.Get(datastorageURL + "/health")
		if err != nil || resp == nil {
			GinkgoWriter.Printf("  ⏳ Waiting for service... (error: %v)\n", err)
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}, "60s", "2s").Should(Equal(200), "Data Storage Service should be healthy")

	// Print container logs for debugging (first 100 lines)
	logs, logErr := exec.Command("podman", "logs", "--tail", "100", serviceContainer).CombinedOutput()
	if logErr == nil {
		GinkgoWriter.Printf("\n📋 Data Storage Service logs:\n%s\n", string(logs))
	}

	GinkgoWriter.Printf("  ✅ Data Storage Service ready at %s\n", datastorageURL)
}

// DBExecutor is an interface that both *sql.DB and *sqlx.DB satisfy
// Used for dynamic partition creation in tests
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// createDynamicPartitions creates partitions for the current month and next month
// This ensures tests don't fail due to time-based partition issues
// DD-TEST-001: Dynamic partition creation for time-independent tests
func createDynamicPartitions(ctx context.Context, targetDB DBExecutor) {
	now := time.Now()

	// Create partitions for current month and next 2 months
	for i := 0; i < 3; i++ {
		month := now.AddDate(0, i, 0)
		year := month.Year()
		monthNum := int(month.Month())

		// Calculate partition boundaries
		startDate := time.Date(year, time.Month(monthNum), 1, 0, 0, 0, 0, time.UTC)
		endDate := startDate.AddDate(0, 1, 0)

		partitionName := fmt.Sprintf("resource_action_traces_y%dm%02d", year, monthNum)
		startStr := startDate.Format("2006-01-02")
		endStr := endDate.Format("2006-01-02")

		// Check if partition already exists
		var exists bool
		checkQuery := `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`
		err := targetDB.QueryRowContext(ctx, checkQuery, partitionName).Scan(&exists)
		if err != nil {
			GinkgoWriter.Printf("  ⚠️  Failed to check partition %s: %v\n", partitionName, err)
			continue
		}

		if exists {
			GinkgoWriter.Printf("  ✅ Partition %s already exists\n", partitionName)
			continue
		}

		// Create partition
		createQuery := fmt.Sprintf(`
			CREATE TABLE %s
			PARTITION OF resource_action_traces
			FOR VALUES FROM ('%s') TO ('%s')
		`, partitionName, startStr, endStr)

		_, err = targetDB.ExecContext(ctx, createQuery)
		if err != nil {
			GinkgoWriter.Printf("  ⚠️  Failed to create partition %s: %v\n", partitionName, err)
		} else {
			GinkgoWriter.Printf("  ✅ Created partition %s (%s to %s)\n", partitionName, startStr, endStr)
		}
	}
}
