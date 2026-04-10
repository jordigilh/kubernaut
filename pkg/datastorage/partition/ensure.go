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

package partition

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DefaultLookaheadMonths is the number of future months to ensure beyond the current month.
// With a value of 3, partitions are created for M0..M+3 (4 total).
const DefaultLookaheadMonths = 3

// Clock abstracts time.Now for testability (retention workers, startup hooks).
type Clock interface {
	Now() time.Time
}

// UTCClock returns the real wall clock in UTC.
type UTCClock struct{}

// Now returns the current time in UTC.
func (UTCClock) Now() time.Time { return time.Now().UTC() }

// ParentTable defines a partitioned table that needs monthly child partitions.
type ParentTable struct {
	Name       string // Parent table name (e.g., "audit_events")
	ColumnName string // Partition key column (e.g., "event_date"); used by retention (#485)
}

// AuditEventsTable is the partition definition for audit_events.
var AuditEventsTable = ParentTable{
	Name:       "audit_events",
	ColumnName: "event_date",
}

// ResourceActionTracesTable is the partition definition for resource_action_traces.
var ResourceActionTracesTable = ParentTable{
	Name:       "resource_action_traces",
	ColumnName: "action_timestamp",
}

// AllTables returns both partitioned tables that require monthly partitions.
func AllTables() []ParentTable {
	return []ParentTable{AuditEventsTable, ResourceActionTracesTable}
}

// PartitionSpec describes a single monthly partition to be created.
type PartitionSpec struct {
	ParentTable string    // Parent table name
	Name        string    // Child partition name (e.g., "audit_events_2026_04")
	RangeStart  time.Time // Inclusive lower bound (first of month, UTC)
	RangeEnd    time.Time // Exclusive upper bound (first of next month, UTC)
}

// ComputePartitionSpecs returns the list of PartitionSpecs for the given tables,
// covering the current month through now+lookahead months (inclusive).
// All boundary computation uses UTC.
func ComputePartitionSpecs(now time.Time, lookahead int, tables []ParentTable) []PartitionSpec {
	utcNow := now.UTC()
	specs := make([]PartitionSpec, 0, len(tables)*(lookahead+1))

	for _, table := range tables {
		for offset := 0; offset <= lookahead; offset++ {
			m := utcNow.AddDate(0, offset, 0)
			year := m.Year()
			month := m.Month()

			rangeStart := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
			rangeEnd := rangeStart.AddDate(0, 1, 0)

			specs = append(specs, PartitionSpec{
				ParentTable: table.Name,
				Name:        FormatPartitionName(table.Name, year, month),
				RangeStart:  rangeStart,
				RangeEnd:    rangeEnd,
			})
		}
	}

	return specs
}

// FormatPartitionName returns the child partition name for a given parent table and month.
// Format: "{parent}_{YYYY}_{MM}" (e.g., "audit_events_2026_04").
func FormatPartitionName(parentTable string, year int, month time.Month) string {
	return fmt.Sprintf("%s_%04d_%02d", parentTable, year, month)
}

// GenerateDDL returns the CREATE TABLE IF NOT EXISTS DDL for a single partition.
func GenerateDDL(spec PartitionSpec) string {
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		spec.Name,
		spec.ParentTable,
		spec.RangeStart.Format("2006-01-02"),
		spec.RangeEnd.Format("2006-01-02"),
	)
}

// DBExecutor abstracts *sql.DB for partition DDL execution.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// EnsureMonthlyPartitions creates missing monthly partitions for the given tables.
// It computes the partition window [now's month .. now's month + lookahead] in UTC,
// generates idempotent DDL (CREATE TABLE IF NOT EXISTS ... PARTITION OF ...),
// and executes each statement against the database.
//
// Returns an error if any DDL execution fails. On failure, the caller should
// treat the database as unsafe for writes and refuse to start (fail-fast).
func EnsureMonthlyPartitions(ctx context.Context, db DBExecutor, now time.Time, lookahead int, tables []ParentTable) error {
	specs := ComputePartitionSpecs(now, lookahead, tables)
	for _, spec := range specs {
		ddl := GenerateDDL(spec)
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("failed to create partition %s: %w", spec.Name, err)
		}
	}
	return nil
}
