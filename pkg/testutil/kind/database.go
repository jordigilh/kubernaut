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

package kind

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	. "github.com/onsi/gomega"
)

// PostgreSQLConfig defines PostgreSQL connection parameters.
type PostgreSQLConfig struct {
	// Host is the PostgreSQL service name in Kind cluster
	// Default: "postgresql.integration.svc.cluster.local"
	Host string

	// Port is the PostgreSQL port
	// Default: 5432
	Port int

	// Database is the database name
	// Default: "kubernaut_test"
	Database string

	// User is the PostgreSQL user
	// Default: "postgres"
	User string

	// Password is the PostgreSQL password
	// Default: "postgres"
	Password string

	// SSLMode is the SSL mode
	// Default: "disable" (safe for Kind cluster internal traffic)
	SSLMode string
}

// GetPostgreSQLConnection creates a connection to PostgreSQL running in Kind cluster.
// This assumes PostgreSQL was deployed via `make bootstrap-dev`.
//
// Example:
//
//	db, err := suite.GetPostgreSQLConnection(kind.PostgreSQLConfig{
//	    Database: "my_test_db",
//	})
//	Expect(err).ToNot(HaveOccurred())
//	defer db.Close()
func (s *IntegrationSuite) GetPostgreSQLConnection(config PostgreSQLConfig) (*sql.DB, error) {
	// Set defaults
	if config.Host == "" {
		config.Host = "postgresql.integration.svc.cluster.local"
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.Database == "" {
		config.Database = "kubernaut_test"
	}
	if config.User == "" {
		config.User = "postgres"
	}
	if config.Password == "" {
		config.Password = "postgres"
	}
	if config.SSLMode == "" {
		config.SSLMode = "disable"
	}

	// Build connection string
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.Database, config.SSLMode,
	)

	// Open connection
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Register cleanup
	s.RegisterCleanup(func() {
		db.Close()
	})

	return db, nil
}

// GetDefaultPostgreSQLConnection creates a connection to PostgreSQL with default config.
// This is a convenience method for the most common case.
//
// Example:
//
//	db, err := suite.GetDefaultPostgreSQLConnection()
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) GetDefaultPostgreSQLConnection() (*sql.DB, error) {
	return s.GetPostgreSQLConnection(PostgreSQLConfig{})
}

// WaitForPostgreSQLReady waits for PostgreSQL to be ready to accept connections.
// This is useful to ensure PostgreSQL is fully initialized before running tests.
//
// Example:
//
//	suite.WaitForPostgreSQLReady(60 * time.Second)
//	db, err := suite.GetDefaultPostgreSQLConnection()
func (s *IntegrationSuite) WaitForPostgreSQLReady(timeout time.Duration) {
	Eventually(func() error {
		db, err := s.GetPostgreSQLConnection(PostgreSQLConfig{})
		if err != nil {
			return err
		}
		defer db.Close()

		// Try a simple query
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		return err
	}, timeout, 2*time.Second).Should(Succeed(),
		"PostgreSQL should be ready to accept connections")
}

// ExecuteSQL executes a SQL statement on the default PostgreSQL database.
// This is useful for setup/teardown operations.
//
// Example:
//
//	err := suite.ExecuteSQL("CREATE TABLE test (id serial PRIMARY KEY)")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) ExecuteSQL(query string) error {
	db, err := s.GetDefaultPostgreSQLConnection()
	if err != nil {
		return err
	}

	_, err = db.Exec(query)
	return err
}

// TruncateTable truncates a table in the default PostgreSQL database.
// This is useful for cleaning up test data between tests.
//
// Example:
//
//	err := suite.TruncateTable("remediation_audit")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) TruncateTable(tableName string) error {
	return s.ExecuteSQL(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", tableName))
}
