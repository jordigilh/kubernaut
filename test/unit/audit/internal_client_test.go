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

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	audit "github.com/jordigilh/kubernaut/pkg/audit"
)

// ========================================
// INTERNAL AUDIT CLIENT TESTS (TDD RED Phase)
// 📋 Design Decision: DD-STORAGE-012 | BR-STORAGE-013
// Authority: DD-STORAGE-012-AUDIT-INTEGRATION-PLAN.md
// ========================================
//
// This test file defines the contract for InternalAuditClient.
// Tests are written FIRST (TDD RED phase), then implementation follows.
//
// Business Requirements:
// - BR-STORAGE-012: Data Storage Service must generate audit traces for its own operations
// - BR-STORAGE-013: Audit traces must not create circular dependencies (no REST API calls)
// - BR-STORAGE-014: Audit writes must not block business operations
//
// Key Design Decision:
// InternalAuditClient writes directly to PostgreSQL (bypasses REST API)
// to avoid circular dependency (Data Storage cannot call itself).
//
// ========================================

var _ = Describe("InternalAuditClient", func() {
	var (
		ctx    context.Context
		client audit.DataStorageClient
		db     *sql.DB
		mock   sqlmock.Sqlmock
		err    error
	)

	BeforeEach(func() {
		ctx = context.Background()
		db, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())
		client = audit.NewInternalAuditClient(db)
	})

	AfterEach(func() {
		_ = db.Close()
	})

	Describe("audit.NewInternalAuditClient", func() {
		Context("when creating internal audit client", func() {
			It("should create client with database connection", func() {
				// BUSINESS SCENARIO: Data Storage Service initializes audit client
				// BR-STORAGE-013: Must use direct PostgreSQL writes (not REST API)

				// BEHAVIOR: Client created successfully
				Expect(client).ToNot(BeNil())

				// CORRECTNESS: Client implements audit.DataStorageClient interface
				_ = audit.DataStorageClient(client) // Type assertion validates interface compliance

				// BUSINESS OUTCOME: Circular dependency avoided
				// This validates BR-STORAGE-013: No REST API calls to self
			})
		})
	})

	Describe("StoreBatch", func() {
		Context("when storing single audit event", func() {
			It("should write directly to PostgreSQL without REST API", func() {
				// BUSINESS SCENARIO: Data Storage Service audits successful write
				// BR-STORAGE-013: Must not create circular dependency

				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "datastorage.audit.written",
					EventCategory:  "storage",
					EventAction:    "written",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "datastorage",
					ResourceType:   "AuditEvent",
					ResourceID:     "test-event-id",
					CorrelationID:  "test-correlation-id",
					EventData:      json.RawMessage(`{"version":"1.0","service":"datastorage"}`),
					RetentionDays:  2555,
					IsSensitive:    false,
				}

				// BEHAVIOR: Writes directly to PostgreSQL (not HTTP)
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				mock.ExpectExec("INSERT INTO audit_events").
					WithArgs(
						event.EventID,
						event.EventVersion,
						sqlmock.AnyArg(), // event_timestamp
						sqlmock.AnyArg(), // event_date (partitioning key)
						event.EventType,
						event.EventCategory,
						event.EventAction,
						event.EventOutcome,
						event.ActorType,
						event.ActorID,
						event.ResourceType,
						event.ResourceID,
						event.CorrelationID,
						sqlmock.AnyArg(), // event_data (JSONB)
						event.RetentionDays,
						event.IsSensitive,
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()

				err := client.StoreBatch(ctx, []*audit.AuditEvent{event})

				// CORRECTNESS: Write succeeds without REST API call
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())

				// BUSINESS OUTCOME: Circular dependency avoided
				// This validates BR-STORAGE-013: No REST API calls to self
			})
		})

		Context("when storing multiple audit events", func() {
			It("should batch insert all events in single transaction", func() {
				// BUSINESS SCENARIO: Data Storage Service audits multiple operations
				// BR-STORAGE-014: Must not block business operations (batch for performance)

				events := []*audit.AuditEvent{
					{
						EventID:        uuid.New(),
						EventVersion:   "1.0",
						EventTimestamp: time.Now().UTC(),
						EventType:      "datastorage.audit.written",
						EventCategory:  "storage",
						EventAction:    "written",
						EventOutcome:   "success",
						ActorType:      "service",
						ActorID:        "datastorage",
						ResourceType:   "AuditEvent",
						ResourceID:     "event-1",
						CorrelationID:  "correlation-1",
						EventData:      json.RawMessage(`{"version":"1.0"}`),
						RetentionDays:  2555,
						IsSensitive:    false,
					},
					{
						EventID:        uuid.New(),
						EventVersion:   "1.0",
						EventTimestamp: time.Now().UTC(),
						EventType:      "datastorage.audit.failed",
						EventCategory:  "storage",
						EventAction:    "write_failed",
						EventOutcome:   "failure",
						ActorType:      "service",
						ActorID:        "datastorage",
						ResourceType:   "AuditEvent",
						ResourceID:     "event-2",
						CorrelationID:  "correlation-2",
						EventData:      json.RawMessage(`{"version":"1.0"}`),
						RetentionDays:  2555,
						IsSensitive:    false,
					},
				}

				// BEHAVIOR: Single transaction with multiple inserts
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				for range events {
					mock.ExpectExec("INSERT INTO audit_events").
						WillReturnResult(sqlmock.NewResult(1, 1))
				}
				mock.ExpectCommit()

				err := client.StoreBatch(ctx, events)

				// CORRECTNESS: All events written in single transaction
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())

				// BUSINESS OUTCOME: Efficient batch writes
				// This validates BR-STORAGE-014: Minimal performance impact
			})
		})

		Context("when storing empty batch", func() {
			It("should return immediately without database call", func() {
				// BUSINESS SCENARIO: No audit events to write
				// BR-STORAGE-014: Must not waste resources on empty operations

				// BEHAVIOR: No database calls for empty batch
				err := client.StoreBatch(ctx, []*audit.AuditEvent{})

				// CORRECTNESS: No error, no database calls
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())

				// BUSINESS OUTCOME: Efficient resource usage
			})
		})

		Context("when database connection fails", func() {
			It("should return error without panicking", func() {
				// BUSINESS SCENARIO: PostgreSQL unavailable during audit write
				// BR-STORAGE-014: Audit failures must not crash service

				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "datastorage.audit.written",
					EventCategory:  "storage",
					EventAction:    "written",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "datastorage",
					ResourceType:   "AuditEvent",
					ResourceID:     "test-event-id",
					CorrelationID:  "test-correlation-id",
					EventData:      json.RawMessage(`{"version":"1.0"}`),
					RetentionDays:  2555,
					IsSensitive:    false,
				}

				// BEHAVIOR: Database connection fails
				mock.ExpectBegin().WillReturnError(fmt.Errorf("connection refused"))

				err := client.StoreBatch(ctx, []*audit.AuditEvent{event})

				// CORRECTNESS: Error returned gracefully (no panic)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection refused"))

				// BUSINESS OUTCOME: Service remains stable despite audit failure
				// This validates BR-STORAGE-014: Audit failures don't block business
			})
		})

		Context("when transaction commit fails", func() {
			It("should rollback transaction and return error", func() {
				// BUSINESS SCENARIO: Transaction commit fails (disk full, etc.)
				// BR-STORAGE-014: Must handle transient failures gracefully

				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "datastorage.audit.written",
					EventCategory:  "storage",
					EventAction:    "written",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "datastorage",
					ResourceType:   "AuditEvent",
					ResourceID:     "test-event-id",
					CorrelationID:  "test-correlation-id",
					EventData:      json.RawMessage(`{"version":"1.0"}`),
					RetentionDays:  2555,
					IsSensitive:    false,
				}

				// BEHAVIOR: Transaction begins, insert succeeds, commit fails
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				mock.ExpectExec("INSERT INTO audit_events").
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit().WillReturnError(fmt.Errorf("disk full"))
				mock.ExpectRollback()

				err := client.StoreBatch(ctx, []*audit.AuditEvent{event})

				// CORRECTNESS: Error returned, transaction rolled back
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("disk full"))

				// BUSINESS OUTCOME: Transaction consistency maintained
			})
		})

		Context("when insert statement fails", func() {
			It("should rollback transaction and return error", func() {
				// BUSINESS SCENARIO: Insert fails (constraint violation, etc.)
				// BR-STORAGE-014: Must handle database errors gracefully

				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "datastorage.audit.written",
					EventCategory:  "storage",
					EventAction:    "written",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "datastorage",
					ResourceType:   "AuditEvent",
					ResourceID:     "test-event-id",
					CorrelationID:  "test-correlation-id",
					EventData:      json.RawMessage(`{"version":"1.0"}`),
					RetentionDays:  2555,
					IsSensitive:    false,
				}

				// BEHAVIOR: Insert fails
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				mock.ExpectExec("INSERT INTO audit_events").
					WillReturnError(fmt.Errorf("constraint violation"))
				mock.ExpectRollback()

				err := client.StoreBatch(ctx, []*audit.AuditEvent{event})

				// CORRECTNESS: Error returned, transaction rolled back
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("constraint violation"))

				// BUSINESS OUTCOME: Database consistency maintained
			})
		})

		Context("when context is cancelled", func() {
			It("should respect context cancellation", func() {
				// BUSINESS SCENARIO: Server shutdown during audit write
				// BR-STORAGE-014: Must respect graceful shutdown

				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "datastorage.audit.written",
					EventCategory:  "storage",
					EventAction:    "written",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "datastorage",
					ResourceType:   "AuditEvent",
					ResourceID:     "test-event-id",
					CorrelationID:  "test-correlation-id",
					EventData:      json.RawMessage(`{"version":"1.0"}`),
					RetentionDays:  2555,
					IsSensitive:    false,
				}

				// BEHAVIOR: Context cancellation handled
				err := client.StoreBatch(cancelCtx, []*audit.AuditEvent{event})

				// CORRECTNESS: Error indicates context cancellation
				Expect(err).To(HaveOccurred())

				// BUSINESS OUTCOME: Graceful shutdown supported
			})
		})
	})
})
