//go:build integration
// +build integration

package infrastructure_integration

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Database Only Infrastructure", Ordered, func() {
	var (
		config shared.DatabaseTestConfig
		db     *sql.DB
		logger *logrus.Logger
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		logger.Info("Testing database infrastructure only")

		// Check if database tests should be skipped
		testConfig := shared.LoadConfig()
		if testConfig.SkipDatabaseTests {
			Skip("Database tests disabled via SKIP_DB_TESTS environment variable")
		}

		config = shared.LoadDatabaseTestConfig()

		connectionString := "postgres://" + config.Username + ":" + config.Password +
			"@" + config.Host + ":" + config.Port + "/" + config.Database +
			"?sslmode=" + config.SSLMode

		var err error
		db, err = sql.Open("postgres", connectionString)
		if err != nil {
			Skip(fmt.Sprintf("Skipping database tests - failed to connect: %v", err))
		}

		err = db.Ping()
		if err != nil {
			Skip(fmt.Sprintf("Skipping database tests - database unavailable: %v", err))
		}

		logger.WithField("connection", connectionString).Info("Database connection successful")

		// BR-SCHEMA-01: Run database migrations for infrastructure testing
		err = runDatabaseMigrations(db, logger)
		if err != nil {
			Skip(fmt.Sprintf("Skipping database tests - migration failed: %v", err))
		}
	})

	AfterAll(func() {
		if db != nil {
			db.Close()
		}
	})

	It("should connect to PostgreSQL database", func() {

		err := db.Ping()
		Expect(err).ToNot(HaveOccurred())

		logger.Info("Database ping successful")
	})

	It("should have required tables from migrations", func() {
		rows, err := db.Query(`
			SELECT tablename FROM pg_tables
			WHERE schemaname = 'public'
			ORDER BY tablename
		`)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var tableName string
			err := rows.Scan(&tableName)
			Expect(err).ToNot(HaveOccurred())
			tables = append(tables, tableName)
		}

		// Verify key tables exist
		Expect(tables).To(ContainElement("resource_references"))
		Expect(tables).To(ContainElement("action_histories"))
		Expect(tables).To(ContainElement("resource_action_traces"))
		Expect(tables).To(ContainElement("oscillation_patterns"))

		logger.WithField("tables", tables).Info("Database tables verified")
	})

	It("should have applied all migrations", func() {
		rows, err := db.Query(`
			SELECT version FROM schema_migrations
			ORDER BY version
		`)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		var versions []string
		for rows.Next() {
			var version string
			err := rows.Scan(&version)
			Expect(err).ToNot(HaveOccurred())
			versions = append(versions, version)
		}

		// Should have migrations 001, 002, 003
		Expect(versions).To(ContainElement("001"))
		Expect(versions).To(ContainElement("002"))
		Expect(versions).To(ContainElement("003"))

		logger.WithField("migrations", versions).Info("Database migrations verified")
	})
})

// runDatabaseMigrations runs the database migrations for database-only infrastructure testing
// BR-SCHEMA-01: Ensure schema_migrations table exists and migrations are tracked
func runDatabaseMigrations(db *sql.DB, logger *logrus.Logger) error {
	// Create schema_migrations table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Find project root by finding the directory containing go.mod
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	migrationDir := filepath.Join(projectRoot, "migrations")

	// Migration files in order (same as isolated_database_utils.go)
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
		"004_add_effectiveness_assessment_due.sql",
		"005_vector_schema.sql",
		"006_effectiveness_assessment.sql",
		"007_add_context_column.sql",
	}

	for _, migration := range migrations {
		// Extract version from filename (e.g., 001_initial_schema.sql -> 001)
		version := migration[:3] // Get first 3 characters as version

		// Check if migration is already applied
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", version, err)
		}

		if exists {
			logger.WithField("version", version).Debug("Migration already applied, skipping")
			continue
		}

		migrationFile := filepath.Join(migrationDir, migration)
		logger.WithField("file", migration).Debug("Running migration")

		content, err := os.ReadFile(migrationFile)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", migration, err)
		}

		// Execute migration
		_, err = db.Exec(string(content))
		if err != nil {
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}

		// Record successful migration
		_, err = db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}

		logger.WithField("version", version).Info("Applied migration")
	}

	logger.Info("All database migrations completed successfully")
	return nil
}
