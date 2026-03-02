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
	"fmt"
	"os"
	"os/exec"
	"strings" // Used by cleanupContainers and migration processing
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/test/infrastructure"
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
	// Shared infrastructure (set once by process 1, read by all processes)
	postgresContainer = "datastorage-postgres-test"
	redisContainer    = "datastorage-redis-test"
	configDir         string // ADR-030: Directory for config and secret files

	// Per-process resources (each parallel process has its own instance)
	// These are created in SynchronizedBeforeSuite Phase 2 (runs on ALL processes)
	// and closed in SynchronizedAfterSuite Phase 1 (runs on ALL processes)
	db          *sqlx.DB      // Per-process DB connection (shared public schema)
	redisClient *redis.Client // Per-process Redis client
	repo        *repository.NotificationAuditRepository
	dlqClient   *dlq.Client
	logger      logr.Logger
	ctx         context.Context
	cancel      context.CancelFunc

	// DD-WE-006: envtest K8s client for dependency validation integration tests.
	// Each parallel process starts its own envtest instance (core API only, no CRDs).
	dsTestEnv   *envtest.Environment
	k8sClient   client.Client
)

// This enables parallel test execution by ensuring each test has unique data
// Uses UUID for guaranteed uniqueness across parallel processes and fast CI environments.
// UnixNano() has ~100ns resolution which can cause collisions in parallel tests.
func generateTestID() string { //nolint:unused
	return fmt.Sprintf("test-%d-%s", GinkgoParallelProcess(), uuid.New().String())
}

// generateTestUUID creates a unique UUID for test data isolation
// Used for audit events and other UUID-based records
func generateTestUUID() uuid.UUID { //nolint:unused
	return uuid.New()
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
	// Allow skipping cleanup for debugging (DS team investigation)
	if os.Getenv("KEEP_CONTAINERS_ON_FAILURE") != "" {
		GinkgoWriter.Println("⚠️  Skipping cleanup (KEEP_CONTAINERS_ON_FAILURE=1 set for debugging)")
		GinkgoWriter.Printf("   To inspect: podman ps -a | grep datastorage\n")
		GinkgoWriter.Printf("   Logs: podman logs datastorage-service-test\n")
		return
	}

	GinkgoWriter.Println("🧹 Cleaning up test infrastructure...")

	// Stop and remove integration test containers (PostgreSQL, Redis - service runs in-process)
	containers := []string{postgresContainer, redisContainer}
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
				_ = exec.Command("podman", "stop", container).Run()
				_ = exec.Command("podman", "rm", "-f", container).Run()
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
				_ = exec.Command("kind", "delete", "cluster", "--name", cluster).Run()
			}
		}
	}

	// Per TESTING_GUIDELINES.md: Use Eventually() instead of time.Sleep()
	Eventually(func() error {
		// Verify cleanup by checking container is gone
		cmd := exec.Command("podman", "ps", "-a", "--format", "{{.Names}}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}
		if strings.Contains(string(output), "datastorage-postgres-test") {
			return fmt.Errorf("Container still exists")
		}
		return nil
	}, 10*time.Second, 1*time.Second).Should(Succeed(), "Cleanup should complete")

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

		// 2. Start PostgreSQL
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
		_ = tempDB.Close()

		// 6. Return infrastructure status
		// Integration tests no longer use HTTP - they use direct repository calls
		// Return a simple "ready" signal to Phase 2
		GinkgoWriter.Println("✅ Infrastructure ready for integration tests")
		return []byte("ready")
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

		// ========================================
		// CRITICAL: Set environment variables in ALL parallel processes
		// ========================================
		// Env vars set in process 1 don't propagate to processes 2, 3, 4
		// Each parallel process needs these vars for tests that create their own
		// connections (e.g., graceful shutdown tests)
		if os.Getenv("POSTGRES_HOST") == "" {
			_ = os.Setenv("POSTGRES_HOST", "localhost")
			_ = os.Setenv("POSTGRES_PORT", "15433") // Mapped port from container (DD-TEST-001)
			_ = os.Setenv("REDIS_HOST", "localhost")
			_ = os.Setenv("REDIS_PORT", "16379") // DD-TEST-001
			GinkgoWriter.Printf("📌 [Process %d] Exported environment variables for test infrastructure\n", processNum)
		}

		// Connect to PostgreSQL (all processes share the public schema)
		// Test isolation is achieved through unique testIDs in fixture data,
		// not per-process schemas. This eliminates search_path race conditions
		// that caused IT-DS-016-005/006 failures (partitioned audit_events
		// cannot be correctly copied with CREATE TABLE LIKE).
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to PostgreSQL...\n", processNum)
		connectPostgreSQL()

		// Connect to Redis
		GinkgoWriter.Printf("🔌 [Process %d] Connecting to Redis...\n", processNum)
		connectRedis()

		// Create repository and DLQ client instances
		GinkgoWriter.Printf("🏗️  [Process %d] Creating repository and DLQ client...\n", processNum)
		repo = repository.NewNotificationAuditRepository(db.DB, logger) // Use db.DB to get *sql.DB from sqlx
		dlqClient, err = dlq.NewClient(redisClient, logger, 10000)      // Gap 3.3: Pass max length for capacity monitoring
		Expect(err).ToNot(HaveOccurred(), "DLQ client creation should succeed")

		// DD-WE-006: Start envtest for dependency validation tests (K8s Secrets/ConfigMaps).
		// Per TESTING_GUIDELINES.md: Integration tests use envtest for K8s, not fake clients.
		GinkgoWriter.Printf("🔧 [Process %d] Starting envtest for K8s dependency validation...\n", processNum)
		_ = os.Setenv("KUBEBUILDER_CONTROLPLANE_START_TIMEOUT", "60s")
		dsTestEnv = &envtest.Environment{
			ErrorIfCRDPathMissing: false,
			ControlPlane: envtest.ControlPlane{
				APIServer: &envtest.APIServer{
					SecureServing: envtest.SecureServing{
						ListenAddr: envtest.ListenAddr{
							Address: "127.0.0.1",
						},
					},
				},
			},
		}
		envCfg, envErr := dsTestEnv.Start()
		Expect(envErr).ToNot(HaveOccurred(), "envtest should start successfully")
		Expect(envCfg.Host).ToNot(BeEmpty(), "envtest should provide a valid API server host")

		Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
		k8sClient, envErr = client.New(envCfg, client.Options{Scheme: scheme.Scheme})
		Expect(envErr).ToNot(HaveOccurred())

		depNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-workflows"}}
		Expect(k8sClient.Create(ctx, depNs)).To(Succeed())
		GinkgoWriter.Printf("✅ [Process %d] envtest ready (kubernaut-workflows namespace created)\n", processNum)

		GinkgoWriter.Printf("✅ [Process %d] Ready to run tests (shared public schema)\n", processNum)
	},
)

var _ = SynchronizedAfterSuite(func() {
	// Phase 1: Runs on ALL parallel processes (per-process cleanup)
	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("🧹 [Process %d] Per-process cleanup...\n", processNum)

	// NOTE: Do NOT close db/redisClient here!
	// Ginkgo may interrupt tests (for timeouts/failures) and run cleanup while
	// Eventually() blocks are still running in goroutines. Closing db here causes
	// "sql: database is closed" errors in those goroutines.
	//
	// These resources are closed in Phase 2 after ALL processes truly complete.

	// DD-WE-006: Stop envtest
	if dsTestEnv != nil {
		if err := dsTestEnv.Stop(); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to stop envtest: %v\n", processNum, err)
		} else {
			GinkgoWriter.Printf("✅ [Process %d] envtest stopped\n", processNum)
		}
	}

	if cancel != nil {
		cancel()
	}

	GinkgoWriter.Printf("✅ [Process %d] Per-process cleanup complete (db/redis still open)\n", processNum)
}, func() {
	// Phase 2: Runs ONCE on parallel process #1 (shared infrastructure cleanup)
	// This ensures PostgreSQL/Redis are only stopped AFTER all processes finish
	GinkgoWriter.Println("🛑 [Process 1] Stopping shared infrastructure...")

	// Close per-process resources (safe now - all processes finished)
	if db != nil {
		_ = db.Close()
		GinkgoWriter.Println("✅ Closed database connection")
	}

	if redisClient != nil {
		// DD-008: Flush Redis DLQ streams before closing to prevent duplicate key errors
		// Problem: DLQ streams persist across test runs. When tests run consecutively:
		// 1. Test inserts events → graceful shutdown drains to DB → DLQ cleared
		// 2. NEXT run: Fresh PostgreSQL but OLD Redis DLQ streams still present
		// 3. Graceful shutdown drains OLD messages → duplicate key errors (events already in DB)
		// Solution: FLUSHALL ensures each test run starts with clean Redis state
		GinkgoWriter.Println("🧹 Flushing Redis DLQ streams (DD-008: Idempotent drain support)...")
		if err := redisClient.FlushAll(context.Background()).Err(); err != nil {
			GinkgoWriter.Printf("⚠️  Failed to flush Redis: %v\n", err)
		} else {
			GinkgoWriter.Println("✅ Redis DLQ streams flushed")
		}

		_ = redisClient.Close()
		GinkgoWriter.Println("✅ Closed Redis connection")
	}

	// Note: Per-process servers already closed in Phase 1 cleanup

	// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
	// ALWAYS collect logs - failures may have occurred on other parallel processes
	// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
	GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
	infrastructure.MustGatherContainerLogs("datastorage", []string{
		postgresContainer, // datastorage-postgres-test
		redisContainer,    // datastorage-redis-test
	}, GinkgoWriter)

	// Clean up shared containers (PostgreSQL, Redis)
	cleanupContainers()

	// DD-TEST-001 v1.1: Clean up infrastructure images to prevent disk space issues
	GinkgoWriter.Println("🧹 DD-TEST-001 v1.1: Cleaning up infrastructure images...")
	pruneCmd := exec.Command("podman", "image", "prune", "-f",
		"--filter", "label=datastorage-test=true")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune infrastructure images: %v\n%s\n", pruneErr, pruneOutput)
	} else {
		GinkgoWriter.Println("✅ Infrastructure images pruned (saves ~500MB-1GB)")
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
		_ = exec.Command("podman", "network", "rm", "datastorage-test").Run()
	}

	// Remove config directory
	if configDir != "" {
		_ = os.RemoveAll(configDir)
	}

	GinkgoWriter.Println("✅ Shared infrastructure cleanup complete")
})

// createNetwork creates a shared Podman network for container-to-container communication
// This allows the Data Storage Service container to connect to PostgreSQL and Redis by container name
// Works on both Linux and macOS (Podman Machine)
func createNetwork() {
	// Remove existing network if it exists
	_ = exec.Command("podman", "network", "rm", "datastorage-test").Run()
	// Create new network
	cmd := exec.Command("podman", "network", "create", "datastorage-test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to create network: %s\n", output)
		Fail(fmt.Sprintf("Network creation failed: %v", err))
	}

	GinkgoWriter.Println("✅ Podman network created")
}

// startPostgreSQL starts PostgreSQL container
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
			// Fix: Set search_path at connection level for consistency
			connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'", host, port)
			testDB, err := sql.Open("pgx", connStr)
			if err != nil {
				return err
			}
			defer func() { _ = testDB.Close() }()
			return testDB.Ping()
		}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

		GinkgoWriter.Println("✅ PostgreSQL is ready")
		return
	}

	// Running locally - start our own container
	GinkgoWriter.Println("🏠 Starting local PostgreSQL container...")

	// Cleanup existing container
	_ = exec.Command("podman", "stop", postgresContainer).Run()
	_ = exec.Command("podman", "rm", postgresContainer).Run()
	// Force remove any existing container to ensure fresh state
	// This prevents data contamination from previous test runs
	GinkgoWriter.Println("🧹 Removing any existing PostgreSQL container...")
	_ = exec.Command("podman", "rm", "-f", postgresContainer).Run()
	// Per TESTING_GUIDELINES.md: Use Eventually() instead of time.Sleep()
	Eventually(func() bool {
		cmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", postgresContainer), "--format", "{{.Names}}")
		output, _ := cmd.CombinedOutput()
		return !strings.Contains(string(output), postgresContainer)
	}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "Container should be removed")

	// Start PostgreSQL
	// Use --network=datastorage-test for container-to-container communication
	// Increase max_connections for parallel test execution (default is 100)
	GinkgoWriter.Println("🔧 Starting fresh PostgreSQL container...")
	cmd := exec.Command("podman", "run", "-d",
		"--name", postgresContainer,
		"--network", "datastorage-test",
		"-p", "15433:5432", // DD-TEST-001
		"-e", "POSTGRES_DB=action_history",
		"-e", "POSTGRES_USER=slm_user",
		"-e", "POSTGRES_PASSWORD=test_password",
		"docker.io/library/postgres:16-alpine",
		"-c", "max_connections=200")

	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to start PostgreSQL: %s\n", output)
		Fail(fmt.Sprintf("PostgreSQL container failed to start: %v", err))
	}

	// Wait for PostgreSQL ready
	// Per TESTING_GUIDELINES.md: Eventually() handles waiting, no time.Sleep() needed
	GinkgoWriter.Println("⏳ Waiting for PostgreSQL to be ready...")

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
			defer func() { _ = testClient.Close() }()
			return testClient.Ping(ctx).Err()
		}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

		GinkgoWriter.Println("✅ Redis is ready")
		return
	}

	// Running locally - start our own container
	GinkgoWriter.Println("🏠 Starting local Redis container...")

	// Cleanup existing container
	_ = exec.Command("podman", "stop", redisContainer).Run()
	_ = exec.Command("podman", "rm", redisContainer).Run()
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
	// Per TESTING_GUIDELINES.md: Eventually() handles waiting, no time.Sleep() needed
	GinkgoWriter.Println("⏳ Waiting for Redis to be ready...")

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

	// Fix: Set search_path at connection level so ALL connections use public schema
	// This prevents connection pool issues where new connections don't inherit SET search_path
	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'", host, port)
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

	// Fix: Set search_path at connection level so ALL connections use public schema
	// This prevents connection pool issues where new connections don't inherit SET search_path
	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'", host, port)
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

	// 2. Auto-discover ALL migrations from filesystem (no manual sync required!)
	// This prevents test failures when DataStorage team adds new migrations
	// Reference: docs/handoff/MIGRATION_SYNC_PREVENTION_STRATEGY.md
	GinkgoWriter.Println("  📜 Auto-discovering migrations from filesystem...")
	migrationsDir := "../../../migrations"
	migrations, err := infrastructure.DiscoverMigrations(migrationsDir)
	Expect(err).ToNot(HaveOccurred(), "Migration discovery should succeed")

	GinkgoWriter.Printf("  📋 Found %d migrations to apply (auto-discovered)\n", len(migrations))

	// 3. Apply each migration in order
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

	// 4. Verify and create critical constraints in public schema
	// Migration 019 should create uq_workflow_name_version, but verify it exists
	// This prevents test failures when workflow repository tests use public schema
	GinkgoWriter.Println("  🔍 Verifying critical constraints in public schema...")
	verifyAndCreatePublicSchemaConstraints(ctx, targetDB)

	// 5. Create dynamic partitions for current month (prevents time-based test failures)
	GinkgoWriter.Println("  📅 Creating dynamic partitions for current month...")
	createDynamicPartitions(ctx, targetDB)

	// 5. Wait for schema propagation (Context API lesson)
	// PostgreSQL needs time to propagate schema changes to new connections
	// Per TESTING_GUIDELINES.md: Use Eventually() to verify schema propagation
	GinkgoWriter.Println("  ⏳ Waiting for schema propagation...")
	Eventually(func() error {
		// Verify schema exists by attempting a simple query
		_, err := targetDB.ExecContext(ctx, "SELECT 1")
		return err
	}, 5*time.Second, 100*time.Millisecond).Should(Succeed(), "Schema should propagate")
	GinkgoWriter.Println("  ✅ Schema propagation complete")
}

// verifyAndCreatePublicSchemaConstraints ensures critical constraints exist in public schema
// Migration 019 should create these, but this provides a safety net for test reliability
// Authority: Migration 019 (uq_workflow_name_version constraint)
// Business Requirement: BR-STORAGE-012 (Workflow Catalog Immutability)
func verifyAndCreatePublicSchemaConstraints(ctx context.Context, targetDB *sql.DB) {
	// Check if uq_workflow_name_version constraint exists
	var constraintExists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint con
			JOIN pg_class rel ON rel.oid = con.conrelid
			JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
			WHERE nsp.nspname = 'public'
			  AND rel.relname = 'remediation_workflow_catalog'
			  AND con.conname = 'uq_workflow_name_version'
		)
	`
	err := targetDB.QueryRowContext(ctx, checkQuery).Scan(&constraintExists)
	if err != nil {
		GinkgoWriter.Printf("  ⚠️  Failed to check constraint existence: %v\n", err)
		Fail(fmt.Sprintf("Failed to verify constraints: %v", err))
	}

	if constraintExists {
		GinkgoWriter.Println("  ✅ Constraint uq_workflow_name_version exists in public schema")
		return
	}

	// Constraint missing - create it (defensive programming for test reliability)
	GinkgoWriter.Println("  ⚠️  Constraint uq_workflow_name_version missing - creating...")
	createConstraintSQL := `
		ALTER TABLE public.remediation_workflow_catalog
		ADD CONSTRAINT uq_workflow_name_version UNIQUE (workflow_name, version)
	`
	_, err = targetDB.ExecContext(ctx, createConstraintSQL)
	if err != nil {
		GinkgoWriter.Printf("  ❌ Failed to create constraint: %v\n", err)
		Fail(fmt.Sprintf("Failed to create uq_workflow_name_version constraint: %v", err))
	}
	GinkgoWriter.Println("  ✅ Created constraint uq_workflow_name_version in public schema")
}

// NOTE: Container-based service functions (buildDataStorageService, startDataStorageService, waitForServiceReady)
// removed as part of refactoring to in-process testing pattern (consistent with other services).
// DataStorage now runs via httptest.Server with server.NewServer() for integration tests.

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
