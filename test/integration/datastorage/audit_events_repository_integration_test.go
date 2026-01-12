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

package datastorage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// AUDIT EVENTS REPOSITORY INTEGRATION TESTS
// ========================================
//
// Purpose: Test AuditEventsRepository against REAL PostgreSQL database
// to catch schema mismatches and field mapping bugs.
//
// Business Requirements:
// - BR-STORAGE-033: Unified audit trail persistence
// - BR-STORAGE-032: ADR-034 compliance
// - BR-AUDIT-001: All service operations generate audit events
//
// Test Strategy:
// - Uses REAL PostgreSQL database (not mocks)
// - Validates ALL ADR-034 fields are persisted and retrieved
// - Tests NULL handling for optional fields
// - Validates pagination and filtering
//
// Coverage Gap Addressed:
// This file addresses the gap identified in TEST_COVERAGE_GAP_ANALYSIS_AUDIT_FIELDS.md
// where missing unit tests allowed the bug (missing version, namespace, cluster_name)
// to reach Gateway E2E tests.
//
// Defense-in-Depth Strategy:
// - Integration tests (this file): Catch schema/field mapping bugs with real DB
// - E2E tests (Gateway): Validate complete business flows
//
// ========================================

var _ = Describe("AuditEventsRepository Integration Tests", func() {
	var (
		auditRepo *repository.AuditEventsRepository
		testID    string
	)

	BeforeEach(func() {

		// Create repository with real database
		// Note: db is *sqlx.DB, but repository expects *sql.DB
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)

		// Generate unique test ID for isolation
		testID = generateTestID()

		// Clean up test data
		_, err := db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("test-repo-%s%%", testID))
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id LIKE $1",
				fmt.Sprintf("test-repo-%s%%", testID))
		}
	})

	// ========================================
	// CREATE METHOD TESTS
	// ========================================
	Describe("Create", func() {
		Context("with all ADR-034 required fields", func() {
			It("should persist audit event with version, namespace, cluster_name", func() {
				// ARRANGE: Create test event with ALL ADR-034 fields
				testEvent := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0", // ADR-034 required field
					EventTimestamp:    time.Now().Add(-5 * time.Second).UTC(),
					EventType:         "gateway.signal.received",
					EventCategory:     "gateway",
					EventAction:       "received",
					EventOutcome:      "success",
					CorrelationID:     fmt.Sprintf("test-repo-%s-001", testID),
					ResourceType:      "Signal",
					ResourceID:        "fp-123",
					ResourceNamespace: "default",      // ADR-034 field (was missing in bug)
					ClusterID:         "prod-cluster", // ADR-034 field (was missing in bug)
					ActorType:         "service",
					ActorID:           "gateway-service",
					Severity:          "info",
					EventData: map[string]interface{}{
						"signal_type": "prometheus",
						"alert_name":  "HighCPU",
					},
				}

				// ACT: Create audit event
				result, err := auditRepo.Create(ctx, testEvent)

				// ASSERT: Create succeeds
				Expect(err).ToNot(HaveOccurred(), "Create should succeed")
				Expect(result).ToNot(BeNil())
				Expect(result.EventID).To(Equal(testEvent.EventID))

				// ASSERT: Verify ALL ADR-034 fields persisted to database
				// This is the critical test that would have caught the bug
				var (
					dbVersion, dbNamespace, dbClusterName                       sql.NullString
					dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
					dbCorrelationID, dbResourceType, dbResourceID               string
					dbActorType, dbActorID, dbSeverity                          sql.NullString
				)

				row := db.QueryRowContext(ctx, `
					SELECT event_version, event_type, event_category, event_action, event_outcome,
					       correlation_id, resource_type, resource_id, namespace, cluster_name,
					       actor_type, actor_id, severity
					FROM audit_events
					WHERE event_id = $1
				`, testEvent.EventID)

				err = row.Scan(
					&dbVersion, // event_version (was missing in bug)
					&dbEventType,
					&dbEventCategory,
					&dbEventAction,
					&dbEventOutcome,
					&dbCorrelationID,
					&dbResourceType,
					&dbResourceID,
					&dbNamespace,   // namespace (was missing in bug)
					&dbClusterName, // cluster_name (was missing in bug)
					&dbActorType,
					&dbActorID,
					&dbSeverity,
				)

				Expect(err).ToNot(HaveOccurred(), "Should retrieve event from database")

				// CRITICAL ASSERTIONS: These would have caught the bug
				Expect(dbVersion.Valid).To(BeTrue(), "event_version should not be NULL")
				Expect(dbVersion.String).To(Equal("1.0"), "event_version should be '1.0'")

				Expect(dbNamespace.Valid).To(BeTrue(), "namespace should not be NULL")
				Expect(dbNamespace.String).To(Equal("default"), "namespace should match input")

				Expect(dbClusterName.Valid).To(BeTrue(), "cluster_name should not be NULL")
				Expect(dbClusterName.String).To(Equal("prod-cluster"), "cluster_name should match input")

				// Verify other ADR-034 fields
				Expect(dbEventType).To(Equal("gateway.signal.received"))
				Expect(dbEventCategory).To(Equal("gateway"))
				Expect(dbEventAction).To(Equal("received"))
				Expect(dbEventOutcome).To(Equal("success"))
				Expect(dbCorrelationID).To(Equal(testEvent.CorrelationID))
				Expect(dbResourceType).To(Equal("Signal"))
				Expect(dbResourceID).To(Equal("fp-123"))
				Expect(dbActorType.String).To(Equal("service"))
				Expect(dbActorID.String).To(Equal("gateway-service"))
				Expect(dbSeverity.String).To(Equal("info"))
			})

			It("should default version to '1.0' if not provided", func() {
				// ARRANGE: Event without version
				testEvent := &repository.AuditEvent{
					EventID:        uuid.New(),
					Version:        "", // Empty - should default to "1.0"
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					EventType:      "gateway.signal.received",
					EventCategory:  "gateway",
					EventAction:    "received",
					EventOutcome:   "success",
					CorrelationID:  fmt.Sprintf("test-repo-%s-002", testID),
					EventData:      map[string]interface{}{},
				}

				// ACT: Create event
				result, err := auditRepo.Create(ctx, testEvent)

				// ASSERT: Version defaults to "1.0" per ADR-034
				Expect(err).ToNot(HaveOccurred())

				var dbVersion string
				err = db.QueryRowContext(ctx,
					"SELECT event_version FROM audit_events WHERE event_id = $1",
					result.EventID).Scan(&dbVersion)

				Expect(err).ToNot(HaveOccurred())
				Expect(dbVersion).To(Equal("1.0"), "Should default to '1.0' per ADR-034")
			})

			It("should handle NULL optional fields (namespace, cluster_name)", func() {
				// ARRANGE: Event without optional fields
				testEvent := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventTimestamp:    time.Now().Add(-5 * time.Second).UTC(),
					EventType:         "gateway.signal.received",
					EventCategory:     "gateway",
					EventAction:       "received",
					EventOutcome:      "success",
					CorrelationID:     fmt.Sprintf("test-repo-%s-003", testID),
					ResourceNamespace: "", // Empty - should be NULL in DB
					ClusterID:         "", // Empty - should be NULL in DB
					EventData:         map[string]interface{}{},
				}

				// ACT: Create event
				result, err := auditRepo.Create(ctx, testEvent)

				// ASSERT: NULL fields handled correctly
				Expect(err).ToNot(HaveOccurred())

				var dbNamespace, dbClusterName sql.NullString
				err = db.QueryRowContext(ctx,
					"SELECT namespace, cluster_name FROM audit_events WHERE event_id = $1",
					result.EventID).Scan(&dbNamespace, &dbClusterName)

				Expect(err).ToNot(HaveOccurred())
				Expect(dbNamespace.Valid).To(BeFalse(), "namespace should be NULL when empty")
				Expect(dbClusterName.Valid).To(BeFalse(), "cluster_name should be NULL when empty")
			})
		})
	})

	// ========================================
	// QUERY METHOD TESTS - CRITICAL FOR BUG FIX
	// ========================================
	//
	// These tests would have caught the missing field bug where
	// version, namespace, cluster_name weren't being selected/scanned.
	//
	// ========================================
	Describe("Query", func() {
		var (
			builder       *query.AuditEventsQueryBuilder
			correlationID string
		)

		BeforeEach(func() {
			correlationID = fmt.Sprintf("test-repo-%s-query", testID)

			// Insert test events into database
			for i := 0; i < 3; i++ {
				testEvent := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventTimestamp:    time.Now().Add(-5 * time.Second).UTC().Add(time.Duration(i) * time.Second),
					EventType:         fmt.Sprintf("gateway.signal.received-%d", i),
					EventCategory:     "gateway",
					EventAction:       "received",
					EventOutcome:      "success",
					CorrelationID:     correlationID,
					ResourceType:      "Signal",
					ResourceID:        fmt.Sprintf("fp-%d", i),
					ResourceNamespace: "default",
					ClusterID:         "prod-cluster",
					ActorType:         "service",
					ActorID:           "gateway-service",
					Severity:          "info",
					EventData:         map[string]interface{}{"test": i},
				}

				_, err := auditRepo.Create(ctx, testEvent)
				Expect(err).ToNot(HaveOccurred())
			}

			// Create query builder
			builder = query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger))
			builder = builder.WithCorrelationID(correlationID)
			builder = builder.WithLimit(50).WithOffset(0)
		})

		Context("with correlation_id filter", func() {
			It("should retrieve events with ALL ADR-034 fields including version, namespace, cluster_name", func() {
				// ACT: Build query and execute
				querySQL, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				countSQL, countArgs, err := builder.BuildCount()
				Expect(err).ToNot(HaveOccurred())

				events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, append(countArgs, args[len(countArgs):]...))

				// ASSERT: Query succeeds
				Expect(err).ToNot(HaveOccurred(), "Query should succeed")
				Expect(events).To(HaveLen(3), "Should return 3 events")
				Expect(pagination).ToNot(BeNil())
				Expect(pagination.Total).To(Equal(3))

				// ASSERT: ALL ADR-034 fields are populated
				// This is the critical test that would have caught the bug
				for _, event := range events {
					// CRITICAL FIELDS (were missing in bug)
					Expect(event.Version).To(Equal("1.0"),
						"Version field MUST be populated from event_version column (ADR-034)")

					Expect(event.ResourceNamespace).To(Equal("default"),
						"Namespace field MUST be populated from namespace column (ADR-034)")

					Expect(event.ClusterID).To(Equal("prod-cluster"),
						"ClusterID field MUST be populated from cluster_name column (ADR-034)")

					// Standard ADR-034 fields
					Expect(event.EventType).To(ContainSubstring("gateway.signal.received"))
					Expect(event.EventCategory).To(Equal("gateway"))
					Expect(event.EventAction).To(Equal("received"))
					Expect(event.EventOutcome).To(Equal("success"))
					Expect(event.CorrelationID).To(Equal(correlationID))
					Expect(event.ResourceType).To(Equal("Signal"))
					Expect(event.ActorType).To(Equal("service"))
					Expect(event.ActorID).To(Equal("gateway-service"))
					Expect(event.Severity).To(Equal("info"))
				}
			})

			It("should handle NULL namespace and cluster_name", func() {
				// ARRANGE: Insert event with NULL optional fields
				testEvent := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventTimestamp:    time.Now().Add(-5 * time.Second).UTC(),
					EventType:         "test.event.null_fields",
					EventCategory:     "test",
					EventAction:       "test",
					EventOutcome:      "success",
					CorrelationID:     fmt.Sprintf("test-repo-%s-null", testID),
					ResourceNamespace: "", // Empty - should be NULL
					ClusterID:         "", // Empty - should be NULL
					EventData:         map[string]interface{}{},
				}

				_, err := auditRepo.Create(ctx, testEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT: Query event
				builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger))
				builder = builder.WithCorrelationID(testEvent.CorrelationID)
				builder = builder.WithLimit(10).WithOffset(0)

				querySQL, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				countSQL, countArgs, err := builder.BuildCount()
				Expect(err).ToNot(HaveOccurred())

				events, _, err := auditRepo.Query(ctx, querySQL, countSQL, append(countArgs, args[len(countArgs):]...))

				// ASSERT: NULL fields handled correctly
				Expect(err).ToNot(HaveOccurred())
				Expect(events).To(HaveLen(1))

				event := events[0]
				Expect(event.ResourceNamespace).To(BeEmpty(), "NULL namespace should be empty string")
				Expect(event.ClusterID).To(BeEmpty(), "NULL cluster_name should be empty string")
			})
		})

		Context("with pagination", func() {
			It("should apply limit and offset correctly", func() {
				// ACT: Query with pagination (limit=2, offset=1)
				builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger))
				builder = builder.WithCorrelationID(correlationID)
				builder = builder.WithLimit(2).WithOffset(1)

				querySQL, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				countSQL, countArgs, err := builder.BuildCount()
				Expect(err).ToNot(HaveOccurred())

				events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, append(countArgs, args[len(countArgs):]...))

				// ASSERT: Pagination applied correctly
				Expect(err).ToNot(HaveOccurred())
				Expect(events).To(HaveLen(2), "Should return 2 events (limit=2)")
				Expect(pagination.Limit).To(Equal(2))
				Expect(pagination.Offset).To(Equal(1))
				Expect(pagination.Total).To(Equal(3))
				Expect(pagination.HasMore).To(BeFalse(), "offset(1) + len(2) = 3, no more pages")
			})

			It("should return correct total count", func() {
				// ACT: Query with small limit
				builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger))
				builder = builder.WithCorrelationID(correlationID)
				builder = builder.WithLimit(1).WithOffset(0)

				querySQL, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				countSQL, countArgs, err := builder.BuildCount()
				Expect(err).ToNot(HaveOccurred())

				events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, append(countArgs, args[len(countArgs):]...))

				// ASSERT: Total count is accurate (not page size)
				Expect(err).ToNot(HaveOccurred())
				Expect(events).To(HaveLen(1), "Should return 1 event (limit=1)")
				Expect(pagination.Total).To(Equal(3), "Total should be 3 (all events)")
				Expect(pagination.HasMore).To(BeTrue(), "Should have more pages")
			})
		})

		Context("with event_type filter", func() {
			It("should filter by event_type correctly", func() {
				// ACT: Query with event_type filter
				builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger))
				builder = builder.WithCorrelationID(correlationID)
				builder = builder.WithEventType("gateway.signal.received-0")
				builder = builder.WithLimit(50).WithOffset(0)

				querySQL, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				countSQL, countArgs, err := builder.BuildCount()
				Expect(err).ToNot(HaveOccurred())

				events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, append(countArgs, args[len(countArgs):]...))

				// ASSERT: Only matching event returned
				Expect(err).ToNot(HaveOccurred())
				Expect(events).To(HaveLen(1), "Should return 1 event matching filter")
				Expect(pagination.Total).To(Equal(1))
				Expect(events[0].EventType).To(Equal("gateway.signal.received-0"))
			})
		})
	})

	// ========================================
	// BATCH CREATE TESTS
	// ========================================
	Describe("CreateBatch", func() {
		It("should persist multiple events with all ADR-034 fields", func() {
			// ARRANGE: Create batch of events
			events := []*repository.AuditEvent{
				{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventTimestamp:    time.Now().Add(-5 * time.Second).UTC(),
					EventType:         "batch.event.1",
					EventCategory:     "test",
					EventAction:       "test",
					EventOutcome:      "success",
					CorrelationID:     fmt.Sprintf("test-repo-%s-batch", testID),
					ResourceNamespace: "ns-1",
					ClusterID:         "cluster-1",
					EventData:         map[string]interface{}{"batch": 1},
				},
				{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventTimestamp:    time.Now().Add(-5 * time.Second).UTC(),
					EventType:         "batch.event.2",
					EventCategory:     "test",
					EventAction:       "test",
					EventOutcome:      "success",
					CorrelationID:     fmt.Sprintf("test-repo-%s-batch", testID),
					ResourceNamespace: "ns-2",
					ClusterID:         "cluster-2",
					EventData:         map[string]interface{}{"batch": 2},
				},
			}

			// ACT: Create batch
			results, err := auditRepo.CreateBatch(ctx, events)

			// ASSERT: Batch create succeeds
			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(HaveLen(2))

			// ASSERT: Verify all events persisted with correct fields
			for i, event := range events {
				var dbVersion, dbNamespace, dbClusterName string
				err := db.QueryRowContext(ctx, `
					SELECT event_version, namespace, cluster_name
					FROM audit_events
					WHERE event_id = $1
				`, event.EventID).Scan(&dbVersion, &dbNamespace, &dbClusterName)

				Expect(err).ToNot(HaveOccurred())
				Expect(dbVersion).To(Equal("1.0"))
				Expect(dbNamespace).To(Equal(fmt.Sprintf("ns-%d", i+1)))
				Expect(dbClusterName).To(Equal(fmt.Sprintf("cluster-%d", i+1)))
			}
		})
	})

	// ========================================
	// HEALTH CHECK TESTS
	// ========================================
	Describe("HealthCheck", func() {
		It("should succeed when database is reachable", func() {
			// ACT: Health check
			err := auditRepo.HealthCheck(ctx)

			// ASSERT: Health check succeeds
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
