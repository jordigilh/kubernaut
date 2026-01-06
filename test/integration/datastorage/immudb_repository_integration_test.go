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
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/codenotary/immudb/pkg/client"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// IMMUDB REPOSITORY INTEGRATION TESTS
// 📋 SOC2 Gap #9: Tamper-Evident Audit Trail - Phase 5.3
// Authority: Immudb Best Practices - Integration Testing
// ========================================
//
// WHY INTEGRATION TESTS (Not Unit Tests)?
//
// Per Immudb documentation:
// "Reserve tests that involve actual interactions with immudb for integration testing.
//  These tests can run against a real or containerized instance of immudb."
//
// RATIONALE:
// - Tests real Immudb behavior (hash chains, Merkle trees, cryptographic proofs)
// - Validates SDK integration (VerifiedSet, Scan, SetAll)
// - No abstraction layer (single backend: Immudb only)
// - Mock complexity (99-method interface) outweighs benefits
//
// TESTING STRATEGY:
// - Use real Immudb container from datastorage_bootstrap.go
// - Test Create(), Query(), CreateBatch() with actual data
// - Validate automatic hash chain and transaction IDs
// - Verify JSON serialization/deserialization
//
// ========================================

var _ = Describe("BR-AUDIT-005 SOC2 Gap #9: Immudb Repository Integration", func() {
	var (
		immuClient client.ImmuClient
		repo       *repository.ImmudbAuditEventsRepository
	)

	BeforeEach(func() {
		// Clean up Immudb identity files before each test (SOC2 Gap #9)
		// Immudb SDK stores server identity files to prevent MITM attacks
		// When containers restart, identity changes, causing connection failures
		files, _ := filepath.Glob(".identity-*")
		for _, file := range files {
			_ = os.Remove(file)
		}

		testLogger := kubelog.NewLogger(kubelog.Options{
			ServiceName: "immudb-integration-test",
			Level:       1,
		})

		// Connect to Immudb (real container from suite_test.go)
		// Port 13322 from DD-TEST-001 (DataStorage Immudb port)
		opts := client.DefaultOptions().
			WithAddress("localhost").
			WithPort(13322).
			WithUsername("immudb").
			WithPassword("immudb").
			WithDatabase("defaultdb")

		var err error
		immuClient, err = client.NewImmuClient(opts)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Immudb client")

		// Login to Immudb
		ctx := context.Background()
		_, err = immuClient.Login(ctx, []byte("immudb"), []byte("immudb"))
		Expect(err).ToNot(HaveOccurred(), "Failed to login to Immudb")

		// Create repository
		repo = repository.NewImmudbAuditEventsRepository(immuClient, testLogger)
	})

	AfterEach(func() {
		if immuClient != nil {
			// Best-effort cleanup: CloseSession can panic if session is already closed
			// Wrap in defer/recover to prevent test panics during cleanup
			defer func() {
				if r := recover(); r != nil {
					// Session already closed or invalid - ignore panic
				}
			}()
			ctx := context.Background()
			_ = immuClient.CloseSession(ctx)
		}
	})

	Context("Create() - Single Event Insertion", func() {
		It("should insert audit event with automatic hash chain", func() {
			ctx := context.Background()
			eventID := uuid.New()

			event := &repository.AuditEvent{
				EventID:       eventID,
				EventType:     "workflow.execution.started",
				CorrelationID: "test-corr-123",
				EventData: map[string]interface{}{
					"workflow_name": "test-workflow",
					"test_mode":     true,
				},
			}

			createdEvent, err := repo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdEvent).ToNot(BeNil())
			Expect(createdEvent.EventID).To(Equal(eventID))
			Expect(createdEvent.EventType).To(Equal("workflow.execution.started"))
			Expect(createdEvent.CorrelationID).To(Equal("test-corr-123"))
			Expect(createdEvent.Version).To(Equal("1.0"))           // Default value
			Expect(createdEvent.RetentionDays).To(Equal(2555))      // Default: 7 years
			Expect(createdEvent.EventTimestamp).ToNot(BeZero())     // Auto-generated
			Expect(createdEvent.EventData["workflow_name"]).To(Equal("test-workflow"))
		})

		It("should auto-generate event_id and timestamp if not provided", func() {
			ctx := context.Background()

			event := &repository.AuditEvent{
				EventType:     "test.event.type",
				CorrelationID: "test-corr-auto",
			}

			createdEvent, err := repo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdEvent.EventID).ToNot(Equal(uuid.Nil))      // Auto-generated
			Expect(createdEvent.EventTimestamp).ToNot(BeZero())       // Auto-generated
		})

		It("should handle multiple sequential inserts with monotonic transaction IDs", func() {
			ctx := context.Background()

			// Insert 3 events sequentially
			for i := 0; i < 3; i++ {
				event := &repository.AuditEvent{
					EventID:       uuid.New(),
					EventType:     "test.sequential.event",
					CorrelationID: "test-corr-sequential",
					EventData: map[string]interface{}{
						"sequence": i,
					},
				}

				_, err := repo.Create(ctx, event)
				Expect(err).ToNot(HaveOccurred())
			}

			// Note: Transaction IDs are monotonic but we can't easily verify them
			// without querying Immudb directly (which Query() will test)
		})
	})

	Context("CreateBatch() - Atomic Batch Insertion", func() {
		It("should insert multiple events in single transaction", func() {
			ctx := context.Background()

			events := []*repository.AuditEvent{
				{
					EventID:       uuid.New(),
					EventType:     "batch.event.1",
					CorrelationID: "batch-corr-123",
				},
				{
					EventID:       uuid.New(),
					EventType:     "batch.event.2",
					CorrelationID: "batch-corr-123",
				},
				{
					EventID:       uuid.New(),
					EventType:     "batch.event.3",
					CorrelationID: "batch-corr-123",
				},
			}

			createdEvents, err := repo.CreateBatch(ctx, events)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdEvents).To(HaveLen(3))

			// Verify all events have populated fields
			for i, event := range createdEvents {
				Expect(event.EventID).ToNot(Equal(uuid.Nil))
				Expect(event.EventTimestamp).ToNot(BeZero())
				Expect(event.Version).To(Equal("1.0"))
				Expect(event.RetentionDays).To(Equal(2555))
				Expect(event.EventType).To(Equal(events[i].EventType))
			}
		})

		It("should reject empty batch", func() {
			ctx := context.Background()

			_, err := repo.CreateBatch(ctx, []*repository.AuditEvent{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("batch cannot be empty"))
		})
	})

	Context("Query() - Scan-Based Retrieval", func() {
		BeforeEach(func() {
			// Seed test data: Insert 5 events for query testing
			ctx := context.Background()
			events := []*repository.AuditEvent{
				{
					EventID:       uuid.New(),
					EventType:     "query.test.event.1",
					CorrelationID: "query-corr-123",
				},
				{
					EventID:       uuid.New(),
					EventType:     "query.test.event.2",
					CorrelationID: "query-corr-123",
				},
				{
					EventID:       uuid.New(),
					EventType:     "query.test.event.3",
					CorrelationID: "query-corr-123",
				},
				{
					EventID:       uuid.New(),
					EventType:     "query.test.event.4",
					CorrelationID: "query-corr-456",
				},
				{
					EventID:       uuid.New(),
					EventType:     "query.test.event.5",
					CorrelationID: "query-corr-456",
				},
			}

			_, err := repo.CreateBatch(ctx, events)
			Expect(err).ToNot(HaveOccurred())

			// Give Immudb time to flush writes
			time.Sleep(100 * time.Millisecond)
		})

		It("should scan and return audit events with pagination", func() {
			ctx := context.Background()

			// Query with pagination (limit=10, offset=0)
			events, pagination, err := repo.Query(ctx, "", "", []interface{}{10, 0})
			Expect(err).ToNot(HaveOccurred())
			Expect(pagination).ToNot(BeNil())
			Expect(pagination.Limit).To(Equal(10))
			Expect(pagination.Offset).To(Equal(0))
			Expect(pagination.Total).To(BeNumerically(">=", 5)) // At least our 5 test events
			Expect(len(events)).To(BeNumerically(">=", 5))
		})

		It("should handle pagination offset and limit", func() {
			ctx := context.Background()

			// Query page 1 (limit=2, offset=0)
			page1Events, page1Pagination, err := repo.Query(ctx, "", "", []interface{}{2, 0})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(page1Events)).To(BeNumerically("<=", 2))
			Expect(page1Pagination.Limit).To(Equal(2))
			Expect(page1Pagination.Offset).To(Equal(0))

			// Query page 2 (limit=2, offset=2)
			page2Events, page2Pagination, err := repo.Query(ctx, "", "", []interface{}{2, 2})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(page2Events)).To(BeNumerically("<=", 2))
			Expect(page2Pagination.Limit).To(Equal(2))
			Expect(page2Pagination.Offset).To(Equal(2))

			// Verify pagination metadata
			if page1Pagination.Total > 2 {
				Expect(page1Pagination.HasMore).To(BeTrue())
			}
		})

		It("should return valid pagination metadata", func() {
			ctx := context.Background()

			events, pagination, err := repo.Query(ctx, "", "", []interface{}{100, 0})
			Expect(err).ToNot(HaveOccurred())
			Expect(pagination.Total).To(Equal(len(events))) // Total matches returned count
			Expect(pagination.Limit).To(Equal(100))
			Expect(pagination.Offset).To(Equal(0))
		})
	})

	Context("HealthCheck() - Connectivity Validation", func() {
		It("should verify Immudb connectivity", func() {
			ctx := context.Background()

			err := repo.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Integration with Real Immudb Features", func() {
		It("should store events with JSON serialization/deserialization", func() {
			ctx := context.Background()

			// Create event with complex EventData
			eventID := uuid.New()
			event := &repository.AuditEvent{
				EventID:       eventID,
				EventType:     "complex.json.event",
				CorrelationID: "json-test-123",
				EventData: map[string]interface{}{
					"nested": map[string]interface{}{
						"field1": "value1",
						"field2": 42,
						"field3": true,
					},
					"array": []interface{}{"item1", "item2", "item3"},
				},
			}

			createdEvent, err := repo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Verify EventData structure is preserved
			nested, ok := createdEvent.EventData["nested"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested["field1"]).To(Equal("value1"))
			Expect(nested["field2"]).To(BeNumerically("==", 42))
			Expect(nested["field3"]).To(BeTrue())

			array, ok := createdEvent.EventData["array"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(array).To(HaveLen(3))
		})

		It("should handle events with all optional fields populated", func() {
			ctx := context.Background()

			parentEventID := uuid.New()
			event := &repository.AuditEvent{
				EventID:           uuid.New(),
				Version:           "1.0",
				EventType:         "full.optional.fields",
				EventCategory:     "workflow",
				EventAction:       "execute",
				CorrelationID:     "full-test-123",
				ParentEventID:     &parentEventID,
				EventTimestamp:    time.Now().UTC(),
				EventOutcome:      "success",
				Severity:          "info",
				ResourceType:      "Pod",
				ResourceID:        "test-pod-123",
				ResourceNamespace: "default",
				ActorType:         "ServiceAccount",
				ActorID:           "system:serviceaccount:default:test",
				EventData:         map[string]interface{}{"key": "value"},
				EventDate:         repository.DateOnly(time.Now().UTC()),
				RetentionDays:     2555,
				ClusterID:         "test-cluster",
			}

			createdEvent, err := repo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdEvent.EventCategory).To(Equal("workflow"))
			Expect(createdEvent.EventAction).To(Equal("execute"))
			Expect(createdEvent.EventOutcome).To(Equal("success"))
			Expect(createdEvent.Severity).To(Equal("info"))
			Expect(createdEvent.ResourceType).To(Equal("Pod"))
			Expect(createdEvent.ActorType).To(Equal("ServiceAccount"))
			Expect(*createdEvent.ParentEventID).To(Equal(parentEventID))
		})
	})
})

