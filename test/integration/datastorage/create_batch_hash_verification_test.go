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

package datastorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// SOC2 Round 2: H-1 + M-1 + M-2 — CreateBatch Hash Chain Verification Tests
// These tests verify that CreateBatch() produces correct hash chains after the
// EventData normalization, event_version, and EventTimestamp fixes.

var _ = Describe("CreateBatch Hash Chain Verification", func() {
	var (
		auditRepo *repository.AuditEventsRepository
		ctx       context.Context
		testID    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
		testID = generateTestID()

		// Clean up test data
		_, err := db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%batch-hash-%s%%", testID))
		Expect(err).ToNot(HaveOccurred())
	})

	It("should produce identical hash for batch-inserted events with integer EventData", func() {
		correlationID := fmt.Sprintf("batch-hash-%s-int", testID)

		event := &repository.AuditEvent{
			EventID:        uuid.New(),
			EventTimestamp: time.Now().UTC(),
			EventType:      "test.batch.hash",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  correlationID,
			ResourceType:   "test-resource",
			ResourceID:     "test-id",
			ActorID:        "test-actor",
			ActorType:      "user",
			RetentionDays:  2555,
			EventData:      map[string]interface{}{"step": 1, "count": 42},
		}

		// Insert via CreateBatch
		created, err := auditRepo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(1))

		// Read back from DB
		filters := repository.ExportFilters{CorrelationID: correlationID}
		result, err := auditRepo.Export(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))

		eventAfterDB := result.Events[0].AuditEvent

		// Recompute hash from read-back event
		eventCopy := repository.PrepareEventForHashing(eventAfterDB)
		eventJSON, err := json.Marshal(eventCopy)
		Expect(err).ToNot(HaveOccurred())

		hasher := sha256.New()
		hasher.Write([]byte("")) // empty previous hash (first in chain)
		hasher.Write(eventJSON)
		recomputedHash := hex.EncodeToString(hasher.Sum(nil))

		Expect(eventAfterDB.EventHash).To(Equal(recomputedHash),
			"Stored hash must match recomputed hash after DB round-trip (H-1: EventData normalization)")
		Expect(result.Events[0].HashChainValid).To(BeTrue(),
			"Export should validate hash chain as valid")
	})

	It("should preserve event_version in batch-inserted events", func() {
		correlationID := fmt.Sprintf("batch-hash-%s-ver", testID)

		event := &repository.AuditEvent{
			EventID:       uuid.New(),
			EventType:     "test.batch.version",
			Version:       "1.0",
			EventCategory: "test",
			EventAction:   "verify",
			EventOutcome:  "success",
			CorrelationID: correlationID,
			ResourceType:  "test-resource",
			ResourceID:    "test-id",
			ActorID:       "test-actor",
			ActorType:     "user",
		}

		created, err := auditRepo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(1))

		// Query DB directly for event_version
		var version string
		err = db.QueryRowContext(ctx,
			"SELECT event_version FROM audit_events WHERE event_id = $1",
			created[0].EventID,
		).Scan(&version)
		Expect(err).ToNot(HaveOccurred())
		Expect(version).To(Equal("1.0"),
			"M-1: event_version must be persisted in batch insert")
	})

	It("should NOT overwrite EventTimestamp after batch insert", func() {
		correlationID := fmt.Sprintf("batch-hash-%s-ts", testID)

		// Use a specific timestamp truncated to microseconds (matching PostgreSQL precision)
		originalTimestamp := time.Date(2026, 2, 5, 14, 30, 45, 123456000, time.UTC)

		event := &repository.AuditEvent{
			EventID:        uuid.New(),
			EventTimestamp: originalTimestamp,
			EventType:      "test.batch.timestamp",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  correlationID,
			ResourceType:   "test-resource",
			ResourceID:     "test-id",
			ActorID:        "test-actor",
			ActorType:      "user",
		}

		created, err := auditRepo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(1))

		// M-2: EventTimestamp must not be overwritten by DB-returned value
		Expect(created[0].EventTimestamp).To(Equal(originalTimestamp),
			"M-2: EventTimestamp must match original after batch insert")
	})

	It("should produce valid hash when caller provides non-UTC timestamp", func() {
		correlationID := fmt.Sprintf("batch-hash-%s-utc", testID)

		// Simulate a caller providing a timestamp in a non-UTC timezone (e.g., EST = UTC-5).
		// Before the H-1 fix, Create()/CreateBatch() did NOT call .UTC() on caller-provided
		// timestamps, but Export/Verify DID — causing hash mismatch on verification.
		est := time.FixedZone("EST", -5*60*60)
		nonUTCTimestamp := time.Date(2026, 2, 5, 10, 30, 45, 123456000, est)

		event := &repository.AuditEvent{
			EventID:        uuid.New(),
			EventTimestamp: nonUTCTimestamp, // Non-UTC: 2026-02-05T10:30:45.123456-05:00
			EventType:      "test.batch.utc",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  correlationID,
			ResourceType:   "test-resource",
			ResourceID:     "test-id",
			ActorID:        "test-actor",
			ActorType:      "user",
			RetentionDays:  2555,
			EventData:      map[string]interface{}{"timezone_test": true},
		}

		// Insert via CreateBatch with non-UTC timestamp
		created, err := auditRepo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(1))

		// The stored timestamp should have been normalized to UTC
		Expect(created[0].EventTimestamp.Location()).To(Equal(time.UTC),
			"H-1: EventTimestamp must be normalized to UTC before storage")

		// Export and verify hash chain — this is the critical check.
		// Export calls .UTC() on the timestamp. If Create/CreateBatch didn't also
		// call .UTC(), the JSON representation would differ, producing a different hash.
		filters := repository.ExportFilters{CorrelationID: correlationID}
		result, err := auditRepo.Export(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))

		Expect(result.Events[0].HashChainValid).To(BeTrue(),
			"H-1: Hash chain must be valid after non-UTC timestamp round-trip")
	})

	It("should produce valid hash when Create() receives non-UTC timestamp", func() {
		correlationID := fmt.Sprintf("batch-hash-%s-create-utc", testID)

		// Same test but via Create() (single event path)
		cet := time.FixedZone("CET", 1*60*60)
		nonUTCTimestamp := time.Date(2026, 2, 5, 16, 30, 45, 123456000, cet)

		event := &repository.AuditEvent{
			EventID:        uuid.New(),
			EventTimestamp: nonUTCTimestamp, // Non-UTC: 2026-02-05T16:30:45.123456+01:00
			EventType:      "test.create.utc",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  correlationID,
			ResourceType:   "test-resource",
			ResourceID:     "test-id",
			ActorID:        "test-actor",
			ActorType:      "user",
			RetentionDays:  2555,
			EventData:      map[string]interface{}{"timezone_test": true},
		}

		created, err := auditRepo.Create(ctx, event)
		Expect(err).ToNot(HaveOccurred())

		Expect(created.EventTimestamp.Location()).To(Equal(time.UTC),
			"H-1: Create() must normalize EventTimestamp to UTC")

		filters := repository.ExportFilters{CorrelationID: correlationID}
		result, err := auditRepo.Export(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))

		Expect(result.Events[0].HashChainValid).To(BeTrue(),
			"H-1: Hash chain must be valid after non-UTC timestamp round-trip via Create()")
	})

	It("should maintain hash chain across batch with same correlation_id", func() {
		correlationID := fmt.Sprintf("batch-hash-%s-chain", testID)

		events := make([]*repository.AuditEvent, 3)
		for i := 0; i < 3; i++ {
			events[i] = &repository.AuditEvent{
				EventID:       uuid.New(),
				EventType:     "test.batch.chain",
				Version:       "1.0",
				EventCategory: "test",
				EventAction:   "verify",
				EventOutcome:  "success",
				CorrelationID: correlationID,
				ResourceType:  "test-resource",
				ResourceID:    fmt.Sprintf("test-id-%d", i),
				ActorID:       "test-actor",
				ActorType:     "user",
				EventData:     map[string]interface{}{"step": i},
			}
		}

		created, err := auditRepo.CreateBatch(ctx, events)
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(3))

		// Export and verify hash chain
		filters := repository.ExportFilters{CorrelationID: correlationID}
		result, err := auditRepo.Export(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(3))

		// Verify all events have valid hash chains
		for i, exportedEvent := range result.Events {
			Expect(exportedEvent.HashChainValid).To(BeTrue(),
				fmt.Sprintf("Event %d hash chain should be valid", i))
		}

		// Verify chain linkage: each event's previous_event_hash matches the prior event's event_hash
		Expect(result.Events[0].PreviousEventHash).To(Equal(""),
			"First event should have empty previous_event_hash")
		Expect(result.Events[1].PreviousEventHash).To(Equal(result.Events[0].EventHash),
			"Second event's previous_event_hash should equal first event's event_hash")
		Expect(result.Events[2].PreviousEventHash).To(Equal(result.Events[1].EventHash),
			"Third event's previous_event_hash should equal second event's event_hash")
	})
})
