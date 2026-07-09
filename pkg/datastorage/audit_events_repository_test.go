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

package datastorage_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// AUDIT EVENTS REPOSITORY UNIT TESTS
// 📋 Business Requirements:
//   - BR-STORAGE-021: REST API Read Endpoints
//   - BR-STORAGE-022: Query Filtering
//   - BR-STORAGE-023: Pagination Support
//
// 📋 Testing Principle: Behavior + Correctness
// 📋 Bug Coverage: Array slice panic prevention (fixed 2026-01-06)
// ========================================
var _ = Describe("AuditEventsRepository - Query with Minimal Args", func() {
	var (
		mockDB   *sql.DB
		mock     sqlmock.Sqlmock
		repo     *repository.AuditEventsRepository
		ctx      context.Context
		logger   logr.Logger
		querySQL string
		countSQL string
	)

	BeforeEach(func() {
		var err error
		mockDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		logger = kubelog.NewLogger(kubelog.DefaultOptions())
		repo = repository.NewAuditEventsRepository(mockDB, logger)

		// Simple query and count SQL for testing
		querySQL = "SELECT * FROM audit_events WHERE event_category = $1 LIMIT $2 OFFSET $3"
		countSQL = "SELECT COUNT(*) FROM audit_events WHERE event_category = $1"
	})

	AfterEach(func() {
		_ = mockDB.Close()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BUG COVERAGE: Array Slice Panic Prevention
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// This test covers a critical bug found in Gateway E2E Test 15 (2026-01-06):
	//
	// PROBLEM: Lines 667, 762-763 in audit_events_repository.go had:
	//   args[:len(args)-2]
	//   args[len(args)-2]
	//   args[len(args)-1]
	// These PANIC if len(args) < 2, causing HTTP 500 errors.
	//
	// FIX: Added bounds check in two locations:
	//   countArgs := args
	//   if len(args) >= 2 {
	//       countArgs = args[:len(args)-2]
	//   }
	//
	// These tests ensure the fix works for all edge cases.
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// Helper to create standard audit event columns for mock rows
	auditEventColumns := []string{
		"event_id", "event_version", "event_type", "event_category",
		"event_action", "correlation_id", "event_timestamp", "event_outcome",
		"signal_severity", "resource_type", "resource_id", "actor_type",
		"actor_id", "actor_ip", "parent_event_id", "event_data", "event_date",
		"namespace", "cluster_id",
		"duration_ms", "error_code", "error_message",
		"event_hash", "previous_event_hash", "retention_days", "is_sensitive", "parent_event_date",
		"legal_hold", "legal_hold_reason", "legal_hold_placed_by", "legal_hold_placed_at",
	}

	Context("Query with minimal filter arguments (Bug Fix: Array Slice Panic)", func() {
		// BR-STORAGE-021: REST API Read Endpoints
		// BR-STORAGE-023: Pagination Support
		// Testing BEHAVIOR: Query handles all arg count combinations without panicking
		// Testing CORRECTNESS: Pagination metadata correctly extracted from args

		// Setup mock helper
		setupMocks := func(args []interface{}, queryStr string, countStr string, expectedTotal int, countArgsCount int) {
			// Mock count query
			if countArgsCount == 0 {
				mock.ExpectQuery("SELECT COUNT").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedTotal))
			} else {
				// Manually build count args to avoid type issues
				countArgs := args[:countArgsCount]
				switch countArgsCount {
				case 1:
					mock.ExpectQuery("SELECT COUNT").
						WithArgs(countArgs[0]).
						WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedTotal))
				case 2:
					mock.ExpectQuery("SELECT COUNT").
						WithArgs(countArgs[0], countArgs[1]).
						WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedTotal))
				}
			}

			// Mock main query
			switch len(args) {
			case 0:
				mock.ExpectQuery("SELECT \\* FROM audit_events").
					WillReturnRows(sqlmock.NewRows(auditEventColumns))
			case 1:
				mock.ExpectQuery("SELECT \\* FROM audit_events").
					WithArgs(args[0]).
					WillReturnRows(sqlmock.NewRows(auditEventColumns))
			case 2:
				mock.ExpectQuery("SELECT \\* FROM audit_events").
					WithArgs(args[0], args[1]).
					WillReturnRows(sqlmock.NewRows(auditEventColumns))
			case 3:
				mock.ExpectQuery("SELECT \\* FROM audit_events").
					WithArgs(args[0], args[1], args[2]).
					WillReturnRows(sqlmock.NewRows(auditEventColumns))
			case 4:
				mock.ExpectQuery("SELECT \\* FROM audit_events").
					WithArgs(args[0], args[1], args[2], args[3]).
					WillReturnRows(sqlmock.NewRows(auditEventColumns))
			}
		}

		DescribeTable("should handle various arg count combinations",
			func(description string, args []interface{}, queryStr string, countStr string, expectedTotal int, countArgsCount int) {
				setupMocks(args, queryStr, countStr, expectedTotal, countArgsCount)

				// Execute query
				events, pagination, err := repo.Query(ctx, queryStr, countStr, args)

				// Assertions: mock returns empty row set, so events is empty;
				// pagination.Total reflects the COUNT query result.
				Expect(err).ToNot(HaveOccurred(), "Should not panic or error: "+description)
				Expect(events).To(BeEmpty())
				Expect(pagination.Total).To(Equal(expectedTotal))
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			},
			Entry("0 args (empty) - Most extreme edge case",
				"Query with NO args at all (Bug would panic: args[:-2])",
				[]interface{}{},
				"SELECT * FROM audit_events",
				"SELECT COUNT(*) FROM audit_events",
				100,
				0,
			),
			Entry("1 arg (limit only) - Edge case that caused panic",
				"Query with ONLY limit (Bug would panic: args[:-1])",
				[]interface{}{10},
				"SELECT * FROM audit_events LIMIT $1",
				"SELECT COUNT(*) FROM audit_events",
				7,
				1,
			),
			Entry("2 args (limit + offset) - Boundary case",
				"Query with NO filters, only pagination (Bug would try: args[:0])",
				[]interface{}{10, 0},
				"SELECT * FROM audit_events LIMIT $1 OFFSET $2",
				"SELECT COUNT(*) FROM audit_events",
				5,
				0,
			),
			Entry("3 args (1 filter + pagination) - Normal case",
				"Query with 1 filter + pagination (Bug would try: args[:1])",
				[]interface{}{"gateway", 10, 0},
				"SELECT * FROM audit_events WHERE event_category = $1 LIMIT $2 OFFSET $3",
				"SELECT COUNT(*) FROM audit_events WHERE event_category = $1",
				3,
				1,
			),
			Entry("4 args (2 filters + pagination) - Complex case",
				"Query with 2 filters + pagination (Bug would try: args[:2])",
				[]interface{}{"gateway", "rr-123", 10, 0},
				"SELECT * FROM audit_events WHERE event_category = $1 AND correlation_id = $2 LIMIT $3 OFFSET $4",
				"SELECT COUNT(*) FROM audit_events WHERE event_category = $1 AND correlation_id = $2",
				2,
				2,
			),
		)
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// REGRESSION TEST: Gateway E2E Test 15 Scenario
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// This test reproduces the exact scenario from Gateway E2E Test 15
	// that caused 15 consecutive HTTP 500 errors (2026-01-06).
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Regression: Gateway E2E Test 15 Scenario", func() {
		It("should handle Gateway audit query (event_category + correlation_id)", func() {
			// This is the EXACT query that caused Gateway E2E Test 15 to fail:
			// - eventCategory: "gateway"
			// - correlationID: "rr-bb9514796a20-1767754293"
			// - Default pagination: limit=10, offset=0
			//
			// Args: ["gateway", "rr-bb9514796a20-1767754293", 10, 0]
			// Bug would try: args[:2] = ["gateway", "rr-bb9514796a20-1767754293"] ✅ OK
			// But test ensures no regressions

			args := []interface{}{"gateway", "rr-bb9514796a20-1767754293", 10, 0}
			querySQL = "SELECT * FROM audit_events WHERE event_category = $1 AND correlation_id = $2 LIMIT $3 OFFSET $4"
			countSQL = "SELECT COUNT(*) FROM audit_events WHERE event_category = $1 AND correlation_id = $2"

			// Mock count query
			mock.ExpectQuery("SELECT COUNT").
				WithArgs("gateway", "rr-bb9514796a20-1767754293").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

			// Mock main query - return 2 events (signal.received + crd.created)
			rows := sqlmock.NewRows([]string{
				"event_id", "event_version", "event_type", "event_category",
				"event_action", "correlation_id", "event_timestamp", "event_outcome",
				"signal_severity", "resource_type", "resource_id", "actor_type",
				"actor_id", "actor_ip", "parent_event_id", "event_data", "event_date",
				"namespace", "cluster_id",
				"duration_ms", "error_code", "error_message",
				"event_hash", "previous_event_hash", "retention_days", "is_sensitive", "parent_event_date",
				"legal_hold", "legal_hold_reason", "legal_hold_placed_by", "legal_hold_placed_at",
			}).
				AddRow(
					"550e8400-e29b-41d4-a716-446655440001", "1.0", "gateway.signal.received", "gateway",
					"received", "rr-bb9514796a20-1767754293", time.Now(), "success",
					nil, sql.NullString{String: "Signal", Valid: true}, sql.NullString{String: "bb9514796a20", Valid: true},
					sql.NullString{String: "gateway", Valid: true}, sql.NullString{String: "gateway-pod", Valid: true},
					sql.NullString{}, sql.NullString{}, []byte(`{}`), time.Now(),
					sql.NullString{String: "audit-11-1767754293143527000", Valid: true}, sql.NullString{},
					sql.NullInt64{}, sql.NullString{}, sql.NullString{},
					sql.NullString{}, sql.NullString{}, sql.NullInt64{Int64: 2555, Valid: true}, sql.NullBool{}, nil,
					false, sql.NullString{}, sql.NullString{}, nil,
				).
				AddRow(
					"550e8400-e29b-41d4-a716-446655440002", "1.0", "gateway.crd.created", "gateway",
					"created", "rr-bb9514796a20-1767754293", time.Now(), "success",
					nil, sql.NullString{String: "RemediationRequest", Valid: true}, sql.NullString{String: "rr-bb9514796a20", Valid: true},
					sql.NullString{String: "gateway", Valid: true}, sql.NullString{String: "gateway-pod", Valid: true},
					sql.NullString{}, sql.NullString{}, []byte(`{}`), time.Now(),
					sql.NullString{String: "audit-11-1767754293143527000", Valid: true}, sql.NullString{},
					sql.NullInt64{}, sql.NullString{}, sql.NullString{},
					sql.NullString{}, sql.NullString{}, sql.NullInt64{Int64: 2555, Valid: true}, sql.NullBool{}, nil,
					false, sql.NullString{}, sql.NullString{}, nil,
				)

			mock.ExpectQuery("SELECT \\* FROM audit_events").
				WithArgs("gateway", "rr-bb9514796a20-1767754293", 10, 0).
				WillReturnRows(rows)

			events, pagination, err := repo.Query(ctx, querySQL, countSQL, args)

			Expect(err).ToNot(HaveOccurred(), "Should not panic or error - Gateway E2E Test 15 should pass!")
			Expect(events).To(HaveLen(2), "Should return 2 events (signal.received + crd.created)")
			Expect(pagination.Total).To(Equal(2))

			// Verify event types match Gateway E2E Test 15 expectations
			Expect(events[0].EventType).To(Equal("gateway.signal.received"))
			Expect(events[1].EventType).To(Equal("gateway.crd.created"))
			Expect(events[0].CorrelationID).To(Equal("rr-bb9514796a20-1767754293"))
			Expect(events[1].CorrelationID).To(Equal("rr-bb9514796a20-1767754293"))

			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})
	})
})
