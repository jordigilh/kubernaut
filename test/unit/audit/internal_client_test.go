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
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	audit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Helper to create test event for internal client tests
// DD-AUDIT-002 V2.0: Uses OpenAPI types with ogen union constructors
func createInternalTestEvent(resourceID string) *ogenclient.AuditEventRequest {
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, "datastorage.audit.written")
	audit.SetEventCategory(event, "storage")
	audit.SetEventAction(event, "written")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "datastorage")
	audit.SetResource(event, "AuditEvent", resourceID)
	audit.SetCorrelationID(event, "test-correlation-id")

	// Use GatewayAuditPayload as generic test payload (ogen migration - discriminated union)
	payload := ogenclient.GatewayAuditPayload{
		EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
		SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
		AlertName:   "test-alert",
		Namespace:   "default",
		Fingerprint: "test-fingerprint",
	}
	audit.SetEventData(event, ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload))
	return event
}

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

				event := createInternalTestEvent("test-event-id")

				// BEHAVIOR: Writes directly to PostgreSQL (not HTTP)
				// DD-AUDIT-002 V2.0: OpenAPI types don't have EventID, RetentionDays, IsSensitive
				// (those are database-specific and generated server-side)
				// Extract OptString values for mock expectations (InternalAuditClient unwraps these)
				actorType := ""
				if event.ActorType.IsSet() {
					actorType = event.ActorType.Value
				}
				actorID := ""
				if event.ActorID.IsSet() {
					actorID = event.ActorID.Value
				}
				resourceType := ""
				if event.ResourceType.IsSet() {
					resourceType = event.ResourceType.Value
				}
				resourceID := ""
				if event.ResourceID.IsSet() {
					resourceID = event.ResourceID.Value
				}

				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				mock.ExpectExec("INSERT INTO audit_events").
					WithArgs(
						sqlmock.AnyArg(), // event_id (UUID generated server-side)
						event.Version,
						sqlmock.AnyArg(), // event_timestamp
						sqlmock.AnyArg(), // event_date (partitioning key)
						event.EventType,
						event.EventCategory,
						event.EventAction,
						string(event.EventOutcome), // event_outcome (enum converted to string)
						actorType,                  // actor_type (OptString unwrapped)
						actorID,                    // actor_id (OptString unwrapped)
						resourceType,               // resource_type (OptString unwrapped)
						resourceID,                 // resource_id (OptString unwrapped)
						event.CorrelationID,        // correlation_id (plain string)
						sqlmock.AnyArg(),           // event_data (JSONB)
						sqlmock.AnyArg(),           // retention_days (database default)
						sqlmock.AnyArg(),           // is_sensitive (database default)
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()

				err := client.StoreBatch(ctx, []*ogenclient.AuditEventRequest{event})

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

				// Create test events using OpenAPI types
				event1 := createInternalTestEvent("event-1")
				event2 := createInternalTestEvent("event-2")
				audit.SetEventType(event2, "datastorage.audit.failed")
				audit.SetEventAction(event2, "write_failed")
				audit.SetEventOutcome(event2, audit.OutcomeFailure)
				audit.SetCorrelationID(event2, "correlation-2")

				events := []*ogenclient.AuditEventRequest{event1, event2}

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
				err := client.StoreBatch(ctx, []*ogenclient.AuditEventRequest{})

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

				event := createInternalTestEvent("test-event-id")

				// BEHAVIOR: Database connection fails
				mock.ExpectBegin().WillReturnError(fmt.Errorf("connection refused"))

				err := client.StoreBatch(ctx, []*ogenclient.AuditEventRequest{event})

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

				event := createInternalTestEvent("test-event-id")

				// BEHAVIOR: Transaction begins, insert succeeds, commit fails
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				mock.ExpectExec("INSERT INTO audit_events").
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit().WillReturnError(fmt.Errorf("disk full"))
				mock.ExpectRollback()

				err := client.StoreBatch(ctx, []*ogenclient.AuditEventRequest{event})

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

				event := createInternalTestEvent("test-event-id")

				// BEHAVIOR: Insert fails
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO audit_events")
				mock.ExpectExec("INSERT INTO audit_events").
					WillReturnError(fmt.Errorf("constraint violation"))
				mock.ExpectRollback()

				err := client.StoreBatch(ctx, []*ogenclient.AuditEventRequest{event})

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

				event := createInternalTestEvent("test-event-id")

				// BEHAVIOR: Context cancellation handled
				err := client.StoreBatch(cancelCtx, []*ogenclient.AuditEventRequest{event})

				// CORRECTNESS: Error indicates context cancellation
				Expect(err).To(HaveOccurred())

				// BUSINESS OUTCOME: Graceful shutdown supported
			})
		})
	})
})
