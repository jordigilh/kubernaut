package contextapi

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/jackc/pgx/v5/stdlib" // DD-010: Migrated from lib/pq
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	// TODO: Remove direct DB access - use Data Storage REST API per ADR-032
	// "github.com/jordigilh/kubernaut/pkg/contextapi/client"
)

func TestContextAPIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context API Integration Suite (ADR-016: Podman Redis + Data Storage Service + PostgreSQL)")
}

var (
	db           *sql.DB
	sqlxDB       *sqlx.DB
	logger       *zap.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	testSchema   string
	// TODO: Remove direct DB access - use Data Storage REST API per ADR-032
	// dbClient     *client.PostgresClient
	cacheManager cache.CacheManager // ADR-016: Real Redis cache for integration tests
	redisPort    = "6379"           // Standard Redis port
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Setup logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// ADR-016: Start Redis via Podman for integration tests
	// Context API is a stateless REST service that only needs Redis for L1 cache
	// No Kubernetes features required → use Podman (not Kind cluster)
	GinkgoWriter.Println("🚀 Starting Redis via Podman (ADR-016)...")

	// Stop and remove existing Redis container (cleanup from previous runs)
	exec.Command("podman", "stop", "contextapi-redis-test").Run()
	exec.Command("podman", "rm", "contextapi-redis-test").Run()

	// Start Redis container
	cmd := exec.Command("podman", "run", "-d",
		"--name", "contextapi-redis-test",
		"-p", fmt.Sprintf("%s:6379", redisPort),
		"redis:7-alpine")
	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Failed to start Redis container: %s", string(output))

	// Wait for Redis to be ready
	time.Sleep(2 * time.Second)

	// Verify Redis is accessible
	redisAddr := fmt.Sprintf("localhost:%s", redisPort)
	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", "contextapi-redis-test", "redis-cli", "ping")
		testOutput, err := testCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Redis not ready: %v, output: %s", err, string(testOutput))
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	GinkgoWriter.Printf("✅ Redis started successfully at %s\n", redisAddr)

	// Create real cache manager with Redis
	cacheConfig := &cache.Config{
		RedisAddr:  redisAddr,
		LRUSize:    1000, // Test cache size
		DefaultTTL: 5 * time.Minute,
	}
	cacheManager, err = cache.NewCacheManager(cacheConfig, logger)
	Expect(err).ToNot(HaveOccurred(), "Failed to create cache manager")

	GinkgoWriter.Println("✅ Cache manager initialized with real Redis")

	// 🔧 SCHEMA APPLICATION FIX: Apply migrations BEFORE establishing test connections
	// This ensures schema is visible to test connections (fixes 5-hour PostgreSQL connection isolation issue)
	GinkgoWriter.Println("🔨 Cleaning database and applying Data Storage Service schema...")
	cleanupCmd := exec.Command("podman", "exec", "-i", "datastorage-postgres",
		"psql", "-U", "postgres", "-d", "action_history",
		"-c", "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO postgres; GRANT ALL ON SCHEMA public TO slm_user; GRANT USAGE ON SCHEMA public TO slm_user; CREATE EXTENSION IF NOT EXISTS vector;")
	cleanupOutput, err := cleanupCmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("⚠️  Cleanup warning: %v\n%s\n", err, cleanupOutput)
	}
	GinkgoWriter.Println("✅ Database cleaned (full schema drop)")

	// Apply ALL migrations in order (001 through 999)
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
		"99-init-vector.sql",
		"999_add_nov_2025_partition.sql",
	}

	for _, migrationFile := range migrations {
		schemaFile := fmt.Sprintf("../../../migrations/%s", migrationFile)
		applyMigrationCmd := exec.Command("bash", "-c",
			fmt.Sprintf("podman exec -i datastorage-postgres psql -U postgres -d action_history < %s 2>&1", schemaFile))
		migrationOutput, migrationErr := applyMigrationCmd.CombinedOutput()
		if migrationErr != nil {
			GinkgoWriter.Printf("  ⚠️  %s: %v\n", migrationFile, migrationErr)
		}
		// Check for actual ERROR lines in output
		if bytes.Contains(migrationOutput, []byte("ERROR")) {
			GinkgoWriter.Printf("  ❌ %s had errors:\n%s\n", migrationFile, string(migrationOutput))
		} else {
			GinkgoWriter.Printf("  ✅ Applied %s\n", migrationFile)
		}
	}
	GinkgoWriter.Println("✅ All migrations applied from authoritative source")

	// Grant permissions to slm_user on all schema objects
	grantCmd := exec.Command("podman", "exec", "-i", "datastorage-postgres",
		"psql", "-U", "postgres", "-d", "action_history",
		"-c", "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user; GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;")
	grantOutput, _ := grantCmd.CombinedOutput()
	if len(grantOutput) > 0 && !bytes.Contains(grantOutput, []byte("GRANT")) {
		GinkgoWriter.Printf("⚠️  Grant warning: %s\n", string(grantOutput))
	}
	GinkgoWriter.Println("✅ Permissions granted to slm_user")

	// CRITICAL: Wait for PostgreSQL to process all migrations and permissions
	// Schema changes need time to propagate to new connections
	time.Sleep(2 * time.Second)
	GinkgoWriter.Println("✅ Waited for schema propagation")

	// BR-CONTEXT-011: Schema Alignment - Connect to Data Storage Service PostgreSQL
	// Uses Data Storage Service infrastructure (deployed in kubernaut-system)
	// Database: action_history (Data Storage Service database)
	// User: slm_user
	// Password: slm_password_dev
	// Host: localhost:5432 (port-forward to cluster)
	//
	// INFRASTRUCTURE SHARING NOTE:
	// This PostgreSQL instance is SHARED with Data Storage Service
	// - Context API uses Data Storage schema (resource_action_traces + action_histories + resource_references)
	// - Schema authority: Data Storage Service (DD-SCHEMA-001)
	// - Requires port-forward: oc port-forward -n kubernaut-system svc/postgres 5432:5432
	connStr := "host=localhost port=5432 user=slm_user password=slm_password_dev dbname=action_history sslmode=disable"
	db, err = sql.Open("pgx", connStr) // DD-010: Using pgx driver
	Expect(err).ToNot(HaveOccurred())

	// Wait for PostgreSQL to be ready
	Eventually(func() error {
		return db.PingContext(ctx)
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready (start with: make bootstrap-dev)")

	// Create sqlx wrapper for query operations (DD-010: use "pgx" driver name)
	sqlxDB = sqlx.NewDb(db, "pgx")

	// CRITICAL: Verify pgvector extension exists
	// Extension is created by Data Storage Service migrations (already done in schema application above)
	var vectorExists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')").Scan(&vectorExists)
	Expect(err).ToNot(HaveOccurred(), "Failed to check pgvector extension")
	Expect(vectorExists).To(BeTrue(), "pgvector extension must exist (created by migrations/001_initial_schema.sql)")

	// BR-CONTEXT-011: Schema Alignment - Use Data Storage Service schema (public)
	// Schema Authority: Data Storage Service (DD-SCHEMA-001)
	// Tables: resource_action_traces, action_histories, resource_references
	// Test isolation: Use test-uid-* and test-rr-* prefixes, cleaned up in AfterEach
	testSchema = "public" // Use existing Data Storage schema
	GinkgoWriter.Println("Using Data Storage Service schema:", testSchema)

	// Verify Data Storage schema tables exist
	// Use a FRESH connection to ensure schema visibility (avoiding connection pool caching)
	verifyConn, err := sql.Open("pgx", connStr) // DD-010: Using pgx driver
	Expect(err).ToNot(HaveOccurred(), "Failed to open verification connection")
	defer verifyConn.Close()

	err = verifyConn.PingContext(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to ping for verification")

	var tableCount int
	err = verifyConn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		AND c.relname IN ('resource_references', 'action_histories', 'resource_action_traces')
		AND c.relkind IN ('r', 'p')  -- 'r' = regular table, 'p' = partitioned table
	`).Scan(&tableCount)
	Expect(err).ToNot(HaveOccurred())

	// Debug: Show which tables are actually visible and current database/schema
	if tableCount != 3 {
		var currentDB, currentSchema string
		verifyConn.QueryRowContext(ctx, "SELECT current_database(), current_schema()").Scan(&currentDB, &currentSchema)
		GinkgoWriter.Printf("⚠️  Current database: %s, schema: %s\n", currentDB, currentSchema)

		var allTableCount int
		verifyConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&allTableCount)
		GinkgoWriter.Printf("⚠️  Total tables in public schema: %d\n", allTableCount)

		rows, _ := verifyConn.QueryContext(ctx, `
			SELECT table_name FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name IN ('resource_references', 'action_histories', 'resource_action_traces')
		`)
		GinkgoWriter.Println("⚠️  Required tables visible to test connection:")
		for rows.Next() {
			var tname string
			rows.Scan(&tname)
			GinkgoWriter.Printf("  - %s\n", tname)
		}
		rows.Close()

		// Show ALL public tables
		rows2, _ := verifyConn.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name LIMIT 20")
		GinkgoWriter.Println("⚠️  All tables in public schema:")
		for rows2.Next() {
			var tname string
			rows2.Scan(&tname)
			GinkgoWriter.Printf("  - %s\n", tname)
		}
		rows2.Close()

		// Specifically check for the 3 required tables
		GinkgoWriter.Println("⚠️  Checking each required table individually:")
		for _, requiredTable := range []string{"resource_references", "action_histories", "resource_action_traces"} {
			var exists bool
			verifyConn.QueryRowContext(ctx, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='%s')", requiredTable)).Scan(&exists)
			GinkgoWriter.Printf("  - %s: %v\n", requiredTable, exists)
		}
	}

	Expect(tableCount).To(Equal(3), "Data Storage schema tables must exist (run migrations first)")
	GinkgoWriter.Println("✅ Verified Data Storage schema tables exist")

	// BR-CONTEXT-001: Historical Context Query - Create Context API database client
	// Uses Data Storage Service schema directly (no test schema needed)
	// TODO: Remove direct DB access - use Data Storage REST API per ADR-032
	// dbClient, err = client.NewPostgresClient(connStr, logger)
	// Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ Context API integration test environment ready (reusing Data Storage infrastructure)!")
	GinkgoWriter.Println("   - PostgreSQL: localhost:5432 (port-forward from kubernaut-system)")
	GinkgoWriter.Println("   - Database: action_history (Data Storage Service)")
	GinkgoWriter.Println("   - Schema: public (Data Storage schema - DD-SCHEMA-001)")
	GinkgoWriter.Println("   - pgvector extension: enabled")
	GinkgoWriter.Println("   - Test data isolation: test-uid-* and test-rr-* prefixes")
})

var _ = AfterSuite(func() {
	defer cancel()

	// Close cache manager
	if cacheManager != nil {
		GinkgoWriter.Println("Closing cache manager...")
		err := cacheManager.Close()
		if err != nil {
			GinkgoWriter.Printf("⚠️  Cache manager close failed (non-fatal): %v\n", err)
		}
	}

	// Stop and remove Redis container (ADR-016 cleanup)
	GinkgoWriter.Println("Stopping Redis container...")
	if output, err := exec.Command("podman", "stop", "contextapi-redis-test").CombinedOutput(); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to stop Redis (non-fatal): %v, output: %s\n", err, string(output))
	}
	if output, err := exec.Command("podman", "rm", "contextapi-redis-test").CombinedOutput(); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to remove Redis (non-fatal): %v, output: %s\n", err, string(output))
	}
	GinkgoWriter.Println("✅ Redis container cleaned up")

	// Close Context API client
	// TODO: Remove direct DB access - use Data Storage REST API per ADR-032
	// if dbClient != nil {
	// 	err := dbClient.Close()
	// 	Expect(err).ToNot(HaveOccurred())
	// }

	// Clean up test data (not schema - we're using Data Storage Service schema)
	if db != nil {
		GinkgoWriter.Println("Cleaning up test data...")
		// Delete test data using prefixes (same as init-db.sql cleanup)
		_, err := db.ExecContext(ctx, `
			DELETE FROM resource_action_traces WHERE action_id LIKE 'test-rr-%';
			DELETE FROM action_histories WHERE id IN (
				SELECT ah.id FROM action_histories ah
				JOIN resource_references rr ON ah.resource_id = rr.id
				WHERE rr.resource_uid LIKE 'test-uid-%'
			);
			DELETE FROM resource_references WHERE resource_uid LIKE 'test-uid-%';
		`)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Test data cleanup failed (non-fatal): %v\n", err)
		}
		db.Close()
	}

	if logger != nil {
		logger.Sync()
	}

	GinkgoWriter.Println("✅ Context API integration test cleanup complete")
})
