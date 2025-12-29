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

// Package sqlutil provides utility functions for working with database operations.
//
// This package was extracted from repeated patterns across DataStorage repositories
// to reduce code duplication and improve maintainability.
//
// Authority: docs/handoff/DS_REFACTORING_OPPORTUNITIES.md (Opportunity 2.1)
package sqlutil

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// ========================================
// NULL TYPE CONVERTERS (V1.0 REFACTOR)
// 📋 Authority: docs/handoff/DS_REFACTORING_OPPORTUNITIES.md
// ========================================
//
// These converters reduce 38 instances of sql.Null* conversion patterns
// across notification_audit_repository.go and audit_events_repository.go.
//
// V1.0 REFACTOR Goals:
// - Consistent null handling across all repositories
// - Reduced code duplication (38 instances → ~12 function calls)
// - Easier to test (unit test converters once)
// - Clearer intent in repository code
//
// Business Value:
// - Easier maintenance (change null handling logic once)
// - Reduced cognitive load when reading repositories
// - Fewer bugs from inconsistent null handling
//
// ========================================

// ToNullString converts a string pointer to sql.NullString.
// Returns Valid=false if pointer is nil or string is empty.
//
// Usage:
//
//	var errorMessage *string
//	nullStr := sqlutil.ToNullString(errorMessage) // Valid=false
//
//	errorMessage = &"database connection failed"
//	nullStr = sqlutil.ToNullString(errorMessage) // Valid=true, String="database connection failed"
func ToNullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: *s, Valid: true}
}

// ToNullStringValue converts a string value to sql.NullString.
// Returns Valid=false if string is empty.
//
// Usage:
//
//	nullStr := sqlutil.ToNullStringValue(audit.DeliveryStatus) // Valid based on empty check
func ToNullStringValue(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// ToNullUUID converts a UUID pointer to sql.NullString.
// Returns Valid=false if pointer is nil.
//
// Usage:
//
//	var parentEventID *uuid.UUID
//	nullUUID := sqlutil.ToNullUUID(parentEventID) // Valid=false
//
//	id := uuid.New()
//	parentEventID = &id
//	nullUUID = sqlutil.ToNullUUID(parentEventID) // Valid=true, String=id.String()
func ToNullUUID(id *uuid.UUID) sql.NullString {
	if id == nil {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: id.String(), Valid: true}
}

// ToNullTime converts a time pointer to sql.NullTime.
// Returns Valid=false if pointer is nil.
//
// Usage:
//
//	var disabledAt *time.Time
//	nullTime := sqlutil.ToNullTime(disabledAt) // Valid=false
//
//	now := time.Now()
//	disabledAt = &now
//	nullTime = sqlutil.ToNullTime(disabledAt) // Valid=true, Time=now
func ToNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// ToNullInt64 converts an int64 pointer to sql.NullInt64.
// Returns Valid=false if pointer is nil.
//
// Usage:
//
//	var durationMs *int64
//	nullInt := sqlutil.ToNullInt64(durationMs) // Valid=false
//
//	duration := int64(1500)
//	durationMs = &duration
//	nullInt = sqlutil.ToNullInt64(durationMs) // Valid=true, Int64=1500
func ToNullInt64(i *int64) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: *i, Valid: true}
}

// FromNullString extracts a string pointer from sql.NullString.
// Returns nil if Valid=false.
//
// Usage:
//
//	var nullStr sql.NullString = sql.NullString{String: "test", Valid: true}
//	str := sqlutil.FromNullString(nullStr) // str = &"test"
//
//	nullStr = sql.NullString{Valid: false}
//	str = sqlutil.FromNullString(nullStr) // str = nil
func FromNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// FromNullTime extracts a time pointer from sql.NullTime.
// Returns nil if Valid=false.
//
// Usage:
//
//	var nullTime sql.NullTime = sql.NullTime{Time: time.Now(), Valid: true}
//	t := sqlutil.FromNullTime(nullTime) // t = &time.Now()
//
//	nullTime = sql.NullTime{Valid: false}
//	t = sqlutil.FromNullTime(nullTime) // t = nil
func FromNullTime(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}

// FromNullInt64 extracts an int64 pointer from sql.NullInt64.
// Returns nil if Valid=false.
//
// Usage:
//
//	var nullInt sql.NullInt64 = sql.NullInt64{Int64: 1500, Valid: true}
//	i := sqlutil.FromNullInt64(nullInt) // i = &1500
//
//	nullInt = sql.NullInt64{Valid: false}
//	i = sqlutil.FromNullInt64(nullInt) // i = nil
func FromNullInt64(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	return &ni.Int64
}
