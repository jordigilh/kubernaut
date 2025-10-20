package contextapi

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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

	// BR-CONTEXT-011: Schema Alignment - Connect to the existing PostgreSQL instance
	// Uses Data Storage Service infrastructure (make bootstrap-dev)
	// Database: postgres (master database for test isolation)
	// User: postgres
	// Password: postgres
	// Host: localhost:5432
	//
	// INFRASTRUCTURE SHARING NOTE:
	// This PostgreSQL instance is SHARED with Data Storage Service integration tests
	// - Context API uses same PostgreSQL instance with separate schemas (contextapi_test_<timestamp>)
	// - Schema-based isolation ensures no conflicts between test suites
	// - Both services share remediation_audit schema from internal/database/schema/
	// - Zero schema drift guaranteed through shared infrastructure
	connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	Expect(err).ToNot(HaveOccurred())

	// Wait for PostgreSQL to be ready
	Eventually(func() error {
		return db.PingContext(ctx)
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready (start with: make bootstrap-dev)")

	// Create sqlx wrapper for query operations
	sqlxDB = sqlx.NewDb(db, "postgres")

	// CRITICAL: Create pgvector extension at database level BEFORE any tests run
	// Extensions are database-scoped, not schema-scoped
	// This ensures all test schemas can use vector types
	// Note: Data Storage Service integration tests also rely on this extension
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	Expect(err).ToNot(HaveOccurred(), "Failed to create pgvector extension")

	// BR-CONTEXT-011: Schema Alignment - Create a unique schema for this test run for isolation
	testSchema = fmt.Sprintf("contextapi_test_%d", time.Now().UnixNano())
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", testSchema))
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Printf("Created test schema: %s\n", testSchema)

	// Set search path to the new schema
	_, err = db.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s, public", testSchema))
	Expect(err).ToNot(HaveOccurred())

	// BR-CONTEXT-011: Zero Schema Drift - Load the authoritative remediation_audit schema
	// AUTHORITATIVE SOURCE: internal/database/schema/remediation_audit.sql
	// This is the SAME schema used by Data Storage Service
	// Any changes to schema MUST be made in this file only
	schemaFile := filepath.Join("..", "..", "..", "internal", "database", "schema", "remediation_audit.sql")
	schemaSQL, err := os.ReadFile(schemaFile)
	Expect(err).ToNot(HaveOccurred(), "Failed to read authoritative schema file")

	_, err = db.ExecContext(ctx, string(schemaSQL))
	Expect(err).ToNot(HaveOccurred(), "Failed to apply authoritative schema to test schema")
	GinkgoWriter.Println("✅ Authoritative remediation_audit schema applied to test schema")

	// BR-CONTEXT-001: Historical Context Query - Create Context API database client
	connStrWithSchema := fmt.Sprintf("host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable search_path=%s,public", testSchema)
	dbClient, err = client.NewPostgresClient(connStrWithSchema, logger)
	Expect(err).ToNot(HaveOccurred())

	GinkgoWriter.Println("✅ Context API integration test environment ready (reusing Data Storage infrastructure)!")
	GinkgoWriter.Println("   - PostgreSQL: localhost:5432 (SHARED with Data Storage tests)")
	GinkgoWriter.Println("   - pgvector extension: enabled")
	GinkgoWriter.Println("   - Test schema:", testSchema)
	GinkgoWriter.Println("   - Infrastructure sharing: Schema-based isolation for parallel testing")
})

var _ = AfterSuite(func() {
	defer cancel()

	// Close Context API client
	if dbClient != nil {
		err := dbClient.Close()
		Expect(err).ToNot(HaveOccurred())
	}

	// Clean up the test schema
	if db != nil {
		GinkgoWriter.Println("Dropping test schema:", testSchema)
		_, err := db.ExecContext(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", testSchema))
		Expect(err).ToNot(HaveOccurred(), "Failed to drop test schema")
		db.Close()
	}

	if logger != nil {
		logger.Sync()
	}

	GinkgoWriter.Println("✅ Context API integration test cleanup complete")
})
