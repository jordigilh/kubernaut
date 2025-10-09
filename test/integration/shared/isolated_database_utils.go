//go:build integration
// +build integration

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

package shared

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/sirupsen/logrus"
)

// DatabaseExecutor interface for both *sql.DB and *sql.Tx
type DatabaseExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// IsolatedDatabaseTestUtils provides database isolation for tests
type IsolatedDatabaseTestUtils struct {
	logger     *logrus.Logger
	masterDB   *sql.DB
	testDB     *sql.DB
	testTx     *sql.Tx
	currentDB  DatabaseExecutor // Current database executor (DB or Tx)
	schemaName string
	repository actionhistory.Repository
	config     DatabaseTestConfig
	cleanup    []func() error
	mutex      sync.Mutex
}

// IsolationStrategy defines how database isolation should work
type IsolationStrategy int

const (
	// TransactionIsolation - Each test runs in a transaction that gets rolled back
	TransactionIsolation IsolationStrategy = iota
	// SchemaIsolation - Each test gets its own schema that gets dropped
	SchemaIsolation
	// TableTruncation - Tables are truncated between tests (fastest but least isolated)
	TableTruncation
)

// NewIsolatedDatabaseTestUtils creates new isolated database test utilities
func NewIsolatedDatabaseTestUtils(logger *logrus.Logger, strategy IsolationStrategy) (*IsolatedDatabaseTestUtils, error) {
	// Check if database tests should be skipped
	testConfig := LoadConfig()
	if testConfig.SkipDatabaseTests {
		logger.Info("Database tests skipped via SKIP_DB_TESTS environment variable")
		return nil, fmt.Errorf("database tests are disabled")
	}

	config := LoadDatabaseTestConfig()

	// Log connection details for debugging
	logger.WithFields(logrus.Fields{
		"host":          config.Host,
		"port":          config.Port,
		"database":      config.Database,
		"user":          config.Username,
		"use_container": testConfig.UseContainerDB,
	}).Info("Initializing database test utilities")

	// Generate unique schema name for this test instance
	schemaName := fmt.Sprintf("test_%d_%d", time.Now().Unix(), rand.Intn(10000))

	utils := &IsolatedDatabaseTestUtils{
		logger:     logger,
		schemaName: schemaName,
		config:     config,
		cleanup:    make([]func() error, 0),
	}

	// Connect to master database
	if err := utils.connectToMasterDatabase(); err != nil {
		// Integration tests MUST fail if database is unavailable
		logger.WithError(err).Error("Failed to connect to database - integration tests require real database")
		if testConfig.UseContainerDB {
			return nil, fmt.Errorf("containerized database connection failed: %w (try running: make bootstrap-dev)", err)
		}
		return nil, fmt.Errorf("database connection failed - integration tests require real database: %w", err)
	}

	// Initialize isolation based on strategy
	switch strategy {
	case TransactionIsolation:
		if err := utils.initializeTransactionIsolation(); err != nil {
			utils.Close()
			return nil, fmt.Errorf("failed to initialize transaction isolation: %w", err)
		}
	case SchemaIsolation:
		if err := utils.initializeSchemaIsolation(); err != nil {
			utils.Close()
			return nil, fmt.Errorf("failed to initialize schema isolation: %w", err)
		}
	case TableTruncation:
		if err := utils.initializeTableTruncation(); err != nil {
			utils.Close()
			return nil, fmt.Errorf("failed to initialize table truncation: %w", err)
		}
	default:
		utils.Close()
		return nil, fmt.Errorf("unsupported isolation strategy: %v", strategy)
	}

	return utils, nil
}

// connectToMasterDatabase establishes connection to the main test database
func (d *IsolatedDatabaseTestUtils) connectToMasterDatabase() error {
	connectionString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		d.config.Username, d.config.Password, d.config.Host, d.config.Port,
		d.config.Database, d.config.SSLMode)

	var err error
	d.masterDB, err = sql.Open("postgres", connectionString)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.masterDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	d.logger.WithField("connection", connectionString).Debug("Master database connection established")
	return nil
}

// initializeTransactionIsolation sets up transaction-based isolation
func (d *IsolatedDatabaseTestUtils) initializeTransactionIsolation() error {
	d.logger.Debug("Initializing transaction-based database isolation")

	// Ensure schema exists and is up-to-date
	if err := d.ensureSchemaExists(); err != nil {
		return fmt.Errorf("failed to ensure schema exists: %w", err)
	}

	// Don't start transaction here - it will be started per test via StartTest()
	// Set current DB and create repository using the master database for now
	d.currentDB = d.masterDB
	d.repository = actionhistory.NewPostgreSQLRepository(d.masterDB, d.logger)

	d.logger.Debug("Transaction-based isolation initialized")
	return nil
}

// StartTest begins a new transaction for the current test
func (d *IsolatedDatabaseTestUtils) StartTest() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.testTx != nil {
		// Rollback any existing transaction
		d.testTx.Rollback()
	}

	var err error
	d.testTx, err = d.masterDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start test transaction: %w", err)
	}

	// Use transaction-scoped repository for proper isolation
	d.currentDB = d.testTx
	d.repository = actionhistory.NewPostgreSQLRepositoryWithTx(d.testTx, d.logger)

	d.logger.Debug("Started new test transaction")
	return nil
}

// EndTest rolls back the current test transaction
func (d *IsolatedDatabaseTestUtils) EndTest() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.testTx != nil {
		err := d.testTx.Rollback()
		d.testTx = nil

		// Reset repository to use master DB
		d.repository = actionhistory.NewPostgreSQLRepository(d.masterDB, d.logger)

		if err != nil {
			return fmt.Errorf("failed to rollback test transaction: %w", err)
		}

		d.logger.Debug("Rolled back test transaction")
	}

	return nil
}

// initializeSchemaIsolation sets up schema-based isolation
func (d *IsolatedDatabaseTestUtils) initializeSchemaIsolation() error {
	d.logger.WithField("schema", d.schemaName).Debug("Initializing schema-based database isolation")

	// Create unique schema for this test
	_, err := d.masterDB.Exec(fmt.Sprintf("CREATE SCHEMA %s", d.schemaName))
	if err != nil {
		return fmt.Errorf("failed to create test schema %s: %w", d.schemaName, err)
	}

	// Add cleanup to drop schema
	d.addCleanup(func() error {
		d.logger.WithField("schema", d.schemaName).Debug("Dropping test schema")
		_, err := d.masterDB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", d.schemaName))
		return err
	})

	// Set search path to use test schema
	_, err = d.masterDB.Exec(fmt.Sprintf("SET search_path = %s, public", d.schemaName))
	if err != nil {
		return fmt.Errorf("failed to set search path to test schema: %w", err)
	}

	// Run migrations in test schema
	if err := d.runMigrationsInSchema(); err != nil {
		return fmt.Errorf("failed to run migrations in test schema: %w", err)
	}

	// Create repository using schema-isolated database
	d.repository = actionhistory.NewPostgreSQLRepository(d.masterDB, d.logger)

	d.logger.WithField("schema", d.schemaName).Debug("Schema-based isolation initialized")
	return nil
}

// initializeTableTruncation sets up table truncation isolation
func (d *IsolatedDatabaseTestUtils) initializeTableTruncation() error {
	d.logger.Debug("Initializing table truncation isolation")

	// Ensure schema exists
	if err := d.ensureSchemaExists(); err != nil {
		return fmt.Errorf("failed to ensure schema exists: %w", err)
	}

	// Create repository
	d.repository = actionhistory.NewPostgreSQLRepository(d.masterDB, d.logger)

	// Add cleanup to truncate all tables
	d.addCleanup(func() error {
		return d.truncateAllTables()
	})

	d.logger.Debug("Table truncation isolation initialized")
	return nil
}

// ensureSchemaExists ensures the main schema exists and is migrated
func (d *IsolatedDatabaseTestUtils) ensureSchemaExists() error {
	// Check if tables exist
	var count int
	err := d.masterDB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('resource_references', 'action_history')
	`).Scan(&count)

	if err != nil {
		return fmt.Errorf("failed to check existing tables: %w", err)
	}

	// If no tables exist, run migrations
	if count == 0 {
		d.logger.Debug("Running initial migrations")
		if err := d.runMigrations(); err != nil {
			return fmt.Errorf("failed to run initial migrations: %w", err)
		}
	}

	return nil
}

// runMigrations runs database migrations
func (d *IsolatedDatabaseTestUtils) runMigrations() error {
	return d.runMigrationsInPath("migrations")
}

// runMigrationsInSchema runs migrations in the test schema
func (d *IsolatedDatabaseTestUtils) runMigrationsInSchema() error {
	return d.runMigrationsInPath("migrations")
}

// runMigrationsInPath executes migration files from the given path
func (d *IsolatedDatabaseTestUtils) runMigrationsInPath(migrationsPath string) error {
	// Find project root
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// BR-SCHEMA-01: Create schema_migrations table for proper tracking
	_, err = d.masterDB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	migrationDir := filepath.Join(projectRoot, migrationsPath)

	// Migration files in order
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

		// Check if migration is already applied - BR-SCHEMA-01: Prevent duplicate migrations
		var exists bool
		err := d.masterDB.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", version, err)
		}

		if exists {
			d.logger.WithField("version", version).Debug("Migration already applied, skipping")
			continue
		}

		migrationFile := filepath.Join(migrationDir, migration)
		d.logger.WithField("file", migration).Debug("Running migration")

		content, err := readFile(migrationFile)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", migration, err)
		}

		// Execute migration
		_, err = d.masterDB.Exec(string(content))
		if err != nil {
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}

		// BR-SCHEMA-01: Record successful migration application
		_, err = d.masterDB.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}

		d.logger.WithField("version", version).Debug("Migration applied and recorded")
	}

	d.logger.Debug("All migrations completed successfully")
	return nil
}

// truncateAllTables truncates all tables for cleanup
func (d *IsolatedDatabaseTestUtils) truncateAllTables() error {
	d.logger.Debug("Truncating all tables")

	// Get all table names
	rows, err := d.masterDB.Query(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
		AND table_name NOT LIKE 'pg_%'
	`)
	if err != nil {
		return fmt.Errorf("failed to get table names: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	// Truncate all tables
	if len(tables) > 0 {
		truncateSQL := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE",
			strings.Join(tables, ", "))
		_, err = d.masterDB.Exec(truncateSQL)
		if err != nil {
			return fmt.Errorf("failed to truncate tables: %w", err)
		}

		d.logger.WithField("tables", len(tables)).Debug("Tables truncated")
	}

	return nil
}

// GetRepository returns the isolated repository
func (d *IsolatedDatabaseTestUtils) GetRepository() actionhistory.Repository {
	return d.repository
}

// GetDatabase returns the database connection (for direct queries if needed)
func (d *IsolatedDatabaseTestUtils) GetDatabase() interface{} {
	if d.testTx != nil {
		return d.testTx
	}
	return d.masterDB
}

// addCleanup adds a cleanup function to be called on Close
func (d *IsolatedDatabaseTestUtils) addCleanup(cleanup func() error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.cleanup = append(d.cleanup, cleanup)
}

// Close performs cleanup and closes all connections
func (d *IsolatedDatabaseTestUtils) Close() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.logger.Debug("Closing isolated database test utils")

	// Run cleanup functions in reverse order
	for i := len(d.cleanup) - 1; i >= 0; i-- {
		if err := d.cleanup[i](); err != nil {
			d.logger.WithError(err).Warn("Cleanup function failed")
		}
	}

	// Close connections
	if d.testTx != nil {
		d.testTx.Rollback() // Ensure rollback if not already done
		d.testTx = nil
	}

	if d.masterDB != nil {
		d.masterDB.Close()
		d.masterDB = nil
	}

	d.logger.Debug("Database isolation cleanup completed")
}

// Helper function to read file content
func readFile(filepath string) ([]byte, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// DatabaseErrorType represents different types of database errors that can be simulated
type DatabaseErrorType string

const (
	DBConnectionFailure   DatabaseErrorType = "connection_failure"
	DBTransactionTimeout  DatabaseErrorType = "transaction_timeout"
	DBConstraintViolation DatabaseErrorType = "constraint_violation"
	DBDeadlock            DatabaseErrorType = "deadlock"
	DBTableLocked         DatabaseErrorType = "table_locked"
	DBQueryTimeout        DatabaseErrorType = "query_timeout"
	DBConnectionPoolFull  DatabaseErrorType = "connection_pool_full"
	DBInvalidSyntax       DatabaseErrorType = "invalid_syntax"
	DBPermissionDenied    DatabaseErrorType = "permission_denied"
	DBDiskFull            DatabaseErrorType = "disk_full"
)

// Enhanced Database Error Injection Methods as specified in TODO requirements

// SimulateDatabaseError simulates specific database error conditions
func (d *IsolatedDatabaseTestUtils) SimulateDatabaseError(errorType DatabaseErrorType) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.logger.WithFields(logrus.Fields{
		"error_type": errorType,
		"test_db":    d.schemaName,
	}).Info("Simulating database error")

	switch errorType {
	case DBConnectionFailure:
		// Close current connections to simulate connection failure
		if d.testTx != nil {
			d.testTx.Rollback()
			d.testTx = nil
		}
		if d.testDB != nil {
			d.testDB.Close()
			d.testDB = nil
		}
		d.currentDB = nil

	case DBTransactionTimeout:
		// Start a long-running transaction that will timeout
		if d.testTx != nil {
			// Execute a query that will take time and potentially timeout
			go func() {
				_, err := d.testTx.Exec("SELECT pg_sleep(10)") // Long sleep
				if err != nil {
					d.logger.WithError(err).Debug("Transaction timeout simulation completed")
				}
			}()
		}

	case DBDeadlock:
		// Simulate deadlock by creating conflicting transactions
		// This is a simplified simulation - real deadlocks are more complex
		if d.testTx != nil {
			// Create a scenario that could lead to deadlock
			_, err := d.testTx.Exec(`
				CREATE TABLE IF NOT EXISTS deadlock_test_1 (id int);
				CREATE TABLE IF NOT EXISTS deadlock_test_2 (id int);
			`)
			if err != nil {
				return fmt.Errorf("failed to setup deadlock simulation: %w", err)
			}
		}

	case DBTableLocked:
		// Simulate table lock by starting a long-running exclusive lock
		if d.testTx != nil {
			_, err := d.testTx.Exec("LOCK TABLE action_history IN ACCESS EXCLUSIVE MODE")
			if err != nil {
				d.logger.WithError(err).Debug("Table lock simulation failed, table may not exist")
			}
		}

	case DBConstraintViolation:
		// This will be triggered when actual operations violate constraints
		d.logger.Debug("Constraint violation simulation prepared")

	case DBQueryTimeout:
		// Simulate query timeout by setting a very low statement timeout
		if d.testTx != nil {
			_, err := d.testTx.Exec("SET statement_timeout = '1ms'")
			if err != nil {
				return fmt.Errorf("failed to set query timeout: %w", err)
			}
		}

	case DBConnectionPoolFull:
		// Simulate by exhausting available connections
		d.logger.Debug("Connection pool exhaustion simulation - would require multiple connections")

	default:
		return fmt.Errorf("unsupported database error type: %s", errorType)
	}

	return nil
}

// ConfigureErrorScenario configures database client for specific error scenario
func (d *IsolatedDatabaseTestUtils) ConfigureErrorScenario(scenario ErrorScenario) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.logger.WithFields(logrus.Fields{
		"scenario": scenario.Name,
		"category": scenario.Category,
	}).Info("Configuring database error scenario")

	switch scenario.Category {
	case "database":
		switch scenario.Name {
		case "database_connection_loss":
			return d.SimulateDatabaseError(DBConnectionFailure)
		case "transaction_deadlock":
			return d.SimulateDatabaseError(DBDeadlock)
		case "table_lock_timeout":
			return d.SimulateDatabaseError(DBTableLocked)
		case "connection_pool_exhausted":
			return d.SimulateDatabaseError(DBConnectionPoolFull)
		case "query_timeout":
			return d.SimulateDatabaseError(DBQueryTimeout)
		default:
			// Generic database error
			return d.SimulateDatabaseError(DBConnectionFailure)
		}

	case "timeout":
		return d.SimulateDatabaseError(DBQueryTimeout)

	case "network":
		// Network issues affecting database connectivity
		return d.SimulateDatabaseError(DBConnectionFailure)

	default:
		return fmt.Errorf("unsupported scenario category for database: %s", scenario.Category)
	}
}

// InjectConnectionInstability simulates unstable database connections
func (d *IsolatedDatabaseTestUtils) InjectConnectionInstability() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.logger.Info("Injecting database connection instability")

	// Simulate intermittent connection issues
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

			// Randomly close and reconnect
			if rand.Float64() < 0.3 { // 30% chance
				if d.testDB != nil {
					d.logger.Debug("Simulating connection drop")
					d.testDB.Close()

					// Try to reconnect after a short delay
					time.Sleep(100 * time.Millisecond)
					// Note: In a real scenario, we'd need to recreate the connection
					// This is a simplified simulation
				}
			}
		}
	}()

	return nil
}

// ResetDatabaseErrorState resets all error injection state
func (d *IsolatedDatabaseTestUtils) ResetDatabaseErrorState() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.logger.Info("Resetting database error injection state")

	// Reset statement timeout if it was set
	if d.testTx != nil {
		_, err := d.testTx.Exec("SET statement_timeout = DEFAULT")
		if err != nil {
			d.logger.WithError(err).Debug("Failed to reset statement timeout")
		}

		// Clean up any test tables created for error simulation
		_, err = d.testTx.Exec("DROP TABLE IF EXISTS deadlock_test_1, deadlock_test_2")
		if err != nil {
			d.logger.WithError(err).Debug("Failed to clean up deadlock test tables")
		}
	}

	return nil
}

// GetDatabaseState returns current database connection state
func (d *IsolatedDatabaseTestUtils) GetDatabaseState() map[string]interface{} {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	state := make(map[string]interface{})
	state["schema_name"] = d.schemaName
	state["has_test_db"] = d.testDB != nil
	state["has_test_tx"] = d.testTx != nil
	state["has_master_db"] = d.masterDB != nil
	state["current_executor"] = d.currentDB != nil

	// Try to ping the database to check connectivity
	if d.testDB != nil {
		err := d.testDB.Ping()
		state["connectivity"] = err == nil
		if err != nil {
			state["connectivity_error"] = err.Error()
		}
	} else {
		state["connectivity"] = false
	}

	return state
}

// CreateErrorProneDatabaseOperation creates a database operation that's likely to fail
func (d *IsolatedDatabaseTestUtils) CreateErrorProneDatabaseOperation() func() error {
	return func() error {
		d.mutex.Lock()
		defer d.mutex.Unlock()

		if d.currentDB == nil {
			return fmt.Errorf("no database connection available")
		}

		// This operation is designed to potentially fail in various ways
		queries := []string{
			"INSERT INTO action_history (id) VALUES (NULL)",                               // May violate NOT NULL constraint
			"SELECT * FROM non_existent_table",                                            // Table doesn't exist
			"INSERT INTO action_history (alert_fingerprint) VALUES ('test') RETURNING id", // May work or fail
		}

		for _, query := range queries {
			_, err := d.currentDB.Exec(query)
			if err != nil {
				return fmt.Errorf("error-prone operation failed as expected: %w", err)
			}
		}

		return nil
	}
}
