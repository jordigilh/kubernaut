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

package dualwrite

import (
	"context"
	"database/sql"
)

// DB defines the database interface for dual-write operations.
// Business Requirement: BR-STORAGE-014 (Atomic dual-write)
// Business Requirement: BR-STORAGE-016 (Context propagation)
type DB interface {
	// Begin starts a new transaction (legacy - deprecated).
	// Deprecated: Use BeginTx for context propagation.
	Begin() (Tx, error)

	// BeginTx starts a new transaction with context support (preferred).
	// The context is used for cancellation and timeout.
	// opts can be nil for default isolation level.
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
}

// Tx defines the transaction interface.
type Tx interface {
	// Commit commits the transaction.
	Commit() error

	// Rollback rolls back the transaction.
	Rollback() error

	// Exec executes a query without returning rows.
	Exec(query string, args ...interface{}) (sql.Result, error)

	// QueryRow executes a query that returns at most one row (for RETURNING clause).
	QueryRow(query string, args ...interface{}) Row
}

// Row represents a single row result for scanning.
type Row interface {
	// Scan copies columns from the row into the values pointed at by dest.
	Scan(dest ...interface{}) error
}

// VectorDBClient defines the interface for vector database operations.
// Business Requirement: BR-STORAGE-012 (Vector embeddings)
type VectorDBClient interface {
	// Insert inserts a vector embedding with metadata into the vector database.
	Insert(ctx context.Context, id int64, embedding []float32, metadata map[string]interface{}) error
}

// WriteResult represents the result of a dual-write operation.
type WriteResult struct {
	// PostgreSQLID is the ID assigned by PostgreSQL
	PostgreSQLID int64

	// PostgreSQLSuccess indicates if PostgreSQL write succeeded
	PostgreSQLSuccess bool

	// VectorDBSuccess indicates if Vector DB write succeeded
	VectorDBSuccess bool

	// VectorDBError contains the Vector DB error message if it failed
	VectorDBError string

	// FallbackMode indicates if the operation fell back to PostgreSQL-only
	FallbackMode bool
}
