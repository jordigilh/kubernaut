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
	"errors"
	"fmt"
)

// Sentinel errors for dual-write operations
// These enable type-safe error detection using errors.Is()
//
// Why Typed Errors?
// - Type-safe error detection (works with error wrapping)
// - No fragile string matching ("vector DB" vs "VectorStore")
// - Compiler-checked (typos caught at compile time)
// - Standard Go practice (since Go 1.13)
//
// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-CODE-TRIAGE.md
// Finding #3: Fragile error detection
var (
	// ErrVectorDB indicates an error from Vector DB operations
	ErrVectorDB = errors.New("vector DB error")

	// ErrPostgreSQL indicates an error from PostgreSQL operations
	ErrPostgreSQL = errors.New("postgresql error")

	// ErrTransaction indicates a transaction failure
	ErrTransaction = errors.New("transaction error")

	// ErrValidation indicates an input validation failure
	ErrValidation = errors.New("validation error")

	// ErrContextCanceled indicates context cancellation or timeout
	ErrContextCanceled = errors.New("context canceled")
)

// WrapVectorDBError wraps an error with ErrVectorDB sentinel
// Use this when Vector DB operations fail
func WrapVectorDBError(err error, op string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %v", ErrVectorDB, op, err)
}

// WrapPostgreSQLError wraps an error with ErrPostgreSQL sentinel
// Use this when PostgreSQL operations fail
func WrapPostgreSQLError(err error, op string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %v", ErrPostgreSQL, op, err)
}

// WrapTransactionError wraps an error with ErrTransaction sentinel
// Use this when transaction operations fail
func WrapTransactionError(err error, op string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %v", ErrTransaction, op, err)
}

// WrapValidationError wraps an error with ErrValidation sentinel
// Use this when validation fails
func WrapValidationError(err error, field string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %v", ErrValidation, field, err)
}

// IsVectorDBError checks if an error is related to Vector DB operations
// This is a type-safe replacement for string-based error detection
func IsVectorDBError(err error) bool {
	return errors.Is(err, ErrVectorDB)
}

// IsPostgreSQLError checks if an error is related to PostgreSQL operations
func IsPostgreSQLError(err error) bool {
	return errors.Is(err, ErrPostgreSQL)
}

// IsTransactionError checks if an error is related to transaction failures
func IsTransactionError(err error) bool {
	return errors.Is(err, ErrTransaction)
}

// IsValidationError checks if an error is related to validation failures
func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidation)
}

// IsContextCanceled checks if an error is due to context cancellation
func IsContextCanceled(err error) bool {
	return errors.Is(err, ErrContextCanceled) || errors.Is(err, errors.New("context canceled"))
}
