package contextapi

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/client"
)

func TestContextAPIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context API Integration Suite (Reuses Data Storage Infrastructure)")
}

var (
	db         *sql.DB
	sqlxDB     *sqlx.DB
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	testSchema string
	dbClient   *client.PostgresClient
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Setup logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

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
	db, err = sql.Open("postgres", connStr)
	Expect(err).ToNot(HaveOccurred())

	// Wait for PostgreSQL to be ready
	Eventually(func() error {
		return db.PingContext(ctx)
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready (start with: make bootstrap-dev)")

	// Create sqlx wrapper for query operations
	sqlxDB = sqlx.NewDb(db, "postgres")

	// CRITICAL: Verify pgvector extension exists
	// Extension is created by Data Storage Service migrations
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	Expect(err).ToNot(HaveOccurred(), "Failed to create pgvector extension")

	// BR-CONTEXT-011: Schema Alignment - Use Data Storage Service schema (public)
	// Schema Authority: Data Storage Service (DD-SCHEMA-001)
	// Tables: resource_action_traces, action_histories, resource_references
	// Test isolation: Use test-uid-* and test-rr-* prefixes, cleaned up in AfterEach
	testSchema = "public" // Use existing Data Storage schema
	GinkgoWriter.Println("Using Data Storage Service schema:", testSchema)

	// Verify Data Storage schema tables exist
	var tableCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('resource_references', 'action_histories', 'resource_action_traces')
	`).Scan(&tableCount)
	Expect(err).ToNot(HaveOccurred())
	Expect(tableCount).To(Equal(3), "Data Storage schema tables must exist (run migrations first)")
	GinkgoWriter.Println("✅ Verified Data Storage schema tables exist")

	// BR-CONTEXT-001: Historical Context Query - Create Context API database client
	// Uses Data Storage Service schema directly (no test schema needed)
	dbClient, err = client.NewPostgresClient(connStr, logger)
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ Context API integration test environment ready (reusing Data Storage infrastructure)!")
	GinkgoWriter.Println("   - PostgreSQL: localhost:5432 (port-forward from kubernaut-system)")
	GinkgoWriter.Println("   - Database: action_history (Data Storage Service)")
	GinkgoWriter.Println("   - Schema: public (Data Storage schema - DD-SCHEMA-001)")
	GinkgoWriter.Println("   - pgvector extension: enabled")
	GinkgoWriter.Println("   - Test data isolation: test-uid-* and test-rr-* prefixes")
})

var _ = AfterSuite(func() {
	defer cancel()

	// Close Context API client
	if dbClient != nil {
		err := dbClient.Close()
		Expect(err).ToNot(HaveOccurred())
	}

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
