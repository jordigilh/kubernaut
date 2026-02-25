/*
Copyright 2026 Jordi Gil.

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

package server

import (
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// NewPgxConnConfig parses a PostgreSQL connection string and returns a
// configured pgx.ConnConfig suitable for use with database/sql via stdlib.
//
// Bug fix #200: Uses QueryExecModeDescribeExec to prevent stale prepared
// statement caches after schema migrations during Helm upgrades. This mode
// describes each query (getting parameter OIDs for correct type encoding)
// but does not cache the result, eliminating "cached plan must not change
// result type" errors while still supporting complex types like JSONB structs.
func NewPgxConnConfig(connStr string) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PostgreSQL connection string: %w", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
	return config, nil
}

// OpenPostgresDB creates a *sql.DB connection using pgx with the configuration
// returned by NewPgxConnConfig.
func OpenPostgresDB(connStr string) (*sql.DB, error) {
	connConfig, err := NewPgxConnConfig(connStr)
	if err != nil {
		return nil, err
	}
	registeredConnStr := stdlib.RegisterConnConfig(connConfig)
	db, err := sql.Open("pgx", registeredConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}
	return db, nil
}
