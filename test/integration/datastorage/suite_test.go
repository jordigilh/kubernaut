package datastorage

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

	"github.com/jordigilh/kubernaut/pkg/datastorage/dualwrite"
	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

func TestDataStorageIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Integration Suite (Kind)")
}

var (
	suite  *kind.IntegrationSuite
	db     *sql.DB
	sqlxDB *sqlx.DB
	logger *zap.Logger
	ctx    context.Context
	cancel context.CancelFunc
)

var _ = BeforeSuite(func() {
	// Setup test context
	ctx, cancel = context.WithCancel(context.Background())

	// Use Kind cluster test template for standardized setup
	// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
	suite = kind.Setup("datastorage-test")

	// Setup logger
	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	var err error
	logger, err = logConfig.Build()
	Expect(err).ToNot(HaveOccurred())

	// Connect to PostgreSQL (running via make bootstrap-dev)
	// Database: postgres (master database for test isolation)
	// User: postgres
	// Password: postgres
	// Host: localhost:5432
	connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	Expect(err).ToNot(HaveOccurred())

	// Wait for PostgreSQL to be ready
	Eventually(func() error {
		return db.PingContext(ctx)
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	// Create sqlx wrapper for query operations
	sqlxDB = sqlx.NewDb(db, "postgres")

	// CRITICAL: Create pgvector extension at database level BEFORE any tests run
	// Extensions are database-scoped, not schema-scoped
	// This ensures all test schemas can use vector types
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	Expect(err).ToNot(HaveOccurred(), "Failed to create pgvector extension")

	GinkgoWriter.Println("✅ Data Storage integration test environment ready!")
	GinkgoWriter.Println("   - PostgreSQL: localhost:5432")
	GinkgoWriter.Println("   - pgvector extension: enabled")
})

var _ = AfterSuite(func() {
	// Close database connection
	if db != nil {
		_ = db.Close()
	}

	// Cancel context
	if cancel != nil {
		cancel()
	}

	// Automatic cleanup of namespaces and registered resources
	if suite != nil {
		suite.Cleanup()
	}

	GinkgoWriter.Println("✅ Data Storage integration test environment cleaned up!")
})

// dbWrapper wraps *sql.DB to implement dualwrite.DB interface
type dbWrapper struct {
	db *sql.DB
}

func (w *dbWrapper) Begin() (dualwrite.Tx, error) {
	tx, err := w.db.Begin()
	if err != nil {
		return nil, err
	}
	return &txWrapper{tx: tx}, nil
}

// BeginTx implements context-aware transaction start (BR-STORAGE-016)
func (w *dbWrapper) BeginTx(ctx context.Context, opts *sql.TxOptions) (dualwrite.Tx, error) {
	tx, err := w.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &txWrapper{tx: tx}, nil
}

// txWrapper wraps *sql.Tx to implement dualwrite.Tx interface
type txWrapper struct {
	tx *sql.Tx
}

func (w *txWrapper) Commit() error {
	return w.tx.Commit()
}

func (w *txWrapper) Rollback() error {
	return w.tx.Rollback()
}

func (w *txWrapper) Exec(query string, args ...interface{}) (sql.Result, error) {
	return w.tx.Exec(query, args...)
}

func (w *txWrapper) QueryRow(query string, args ...interface{}) dualwrite.Row {
	return w.tx.QueryRow(query, args...)
}

// generateTestEmbedding creates a 384-dimensional test embedding
// Per Day 8: Fixed from 3-4 dimensions to proper 384 dimensions
func generateTestEmbedding(seed float32) []float32 {
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = (float32(i) / 384.0) + seed
	}
	return embedding
}
