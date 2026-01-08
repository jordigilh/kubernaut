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

// ========================================
// AUDIT EXPORT INTEGRATION TESTS - SOC2 COMPLIANCE
// ========================================
//
// Purpose: Test SOC2 audit export and hash chain verification against
// REAL PostgreSQL database to validate tamper-evidence compliance.
//
// Business Requirements:
// - BR-SOC2-001: Hash chain verification (CC8.1 - Tamper-evident logs)
// - BR-SOC2-002: Audit export API (AU-9 - Protection of Audit Information)
// - BR-SOC2-003: Digital signatures for exports
//
// Test Strategy:
// - Uses REAL PostgreSQL database (not mocks)
// - Tests hash chain insertion AND verification
// - Tests export metadata generation
// - Validates hash chain integrity detection
//
// Coverage Gap Addressed:
// This file addresses the critical gap where SOC2 hash chain verification
// was ONLY tested at E2E tier, violating defense-in-depth principles.
//
// Testing Tier Compliance (per 03-testing-strategy.mdc):
// - Unit tests: Hash calculation algorithms (isolated)
// - Integration tests (THIS FILE): Hash chain with real PostgreSQL
// - E2E tests: Complete SOC2 workflow with cert-manager
//
// ========================================

var _ = Describe("Audit Export Integration Tests - SOC2", func() {
	var (
		auditRepo *repository.AuditEventsRepository
		testID    string
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
		testID = generateTestID()

		// Clean up test data
		_, err := db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("soc2-export-%s%%", testID))
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id LIKE $1",
				fmt.Sprintf("soc2-export-%s%%", testID))
		}
	})

	// ========================================
	// HASH CHAIN INSERTION TESTS
	// ========================================
	Describe("Hash Chain Insertion", func() {
		Context("when creating audit events with hash chain", func() {
			It("should calculate and store correct hash chain linkage", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-chain-insert", testID)

				// Create 3 events in sequence
				var eventIDs []uuid.UUID
				for i := 0; i < 3; i++ {
					event := &repository.AuditEvent{
						EventID:         uuid.New(),
						EventDate:       repository.DateOnly(time.Now()),
						EventType:       "workflow.created",
						EventAction:     fmt.Sprintf("create-workflow-%d", i),
						EventCategory:   "workflow",
						EventOutcome:    "success",
						CorrelationID:   correlationID,
						ResourceType:    "workflow",
						ResourceID:      fmt.Sprintf("workflow-%d", i),
						ActorID:         "system",
						ActorType:       "service_account",
						EventData:       map[string]interface{}{"index": i, "test": "hash-chain"},
						Version:         "1",
					}

					createdEvent, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
					Expect(createdEvent).ToNot(BeNil())
					Expect(createdEvent.EventHash).ToNot(BeEmpty(), "Event hash should be calculated")

					eventIDs = append(eventIDs, createdEvent.EventID)

					if i > 0 {
						// Verify previous_event_hash links to previous event
						var prevHash string
						err = db.QueryRowContext(ctx,
							"SELECT event_hash FROM audit_events WHERE event_id = $1",
							eventIDs[i-1]).Scan(&prevHash)
						Expect(err).ToNot(HaveOccurred())
						Expect(createdEvent.PreviousEventHash).To(Equal(prevHash),
							"Event %d should link to previous event's hash", i)
					}
				}

				// Verify complete chain in database
				rows, err := db.QueryContext(ctx,
					`SELECT event_id, event_hash, previous_event_hash
					 FROM audit_events
					 WHERE correlation_id = $1
					 ORDER BY event_date ASC`,
					correlationID)
				Expect(err).ToNot(HaveOccurred())
				defer rows.Close()

				var chainLinks []struct {
					EventID          uuid.UUID
					EventHash        string
					PreviousEventHash string
				}

				for rows.Next() {
					var link struct {
						EventID          uuid.UUID
						EventHash        string
						PreviousEventHash string
					}
					err = rows.Scan(&link.EventID, &link.EventHash, &link.PreviousEventHash)
					Expect(err).ToNot(HaveOccurred())
					chainLinks = append(chainLinks, link)
				}

				Expect(chainLinks).To(HaveLen(3), "Should have 3 events in chain")
				Expect(chainLinks[0].PreviousEventHash).To(BeEmpty(), "First event should have no previous hash")
				Expect(chainLinks[1].PreviousEventHash).To(Equal(chainLinks[0].EventHash))
				Expect(chainLinks[2].PreviousEventHash).To(Equal(chainLinks[1].EventHash))
			})
		})
	})

	// ========================================
	// HASH CHAIN VERIFICATION TESTS
	// ========================================
	Describe("Hash Chain Verification", func() {
		Context("when exporting audit events with valid hash chain", func() {
			It("should verify hash chain integrity correctly", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-verify-valid", testID)

				// Create 5 events with valid hash chain
				for i := 0; i < 5; i++ {
					event := &repository.AuditEvent{
						EventID:         uuid.New(),
						EventDate:       repository.DateOnly(time.Now()),
						EventType:       "workflow.executed",
						EventAction:     fmt.Sprintf("execute-step-%d", i),
						EventCategory:   "workflow",
						EventOutcome:    "success",
						CorrelationID:   correlationID,
						ResourceType:    "workflow",
						ResourceID:      "test-workflow",
						ActorID:         "user@example.com",
						ActorType:       "user",
						EventData:       map[string]interface{}{"step": i, "status": "completed"},
						Version:         "1",
					}

					_, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
				}

				// Export and verify
				filters := repository.ExportFilters{
					CorrelationID: correlationID,
				}

				result, err := auditRepo.Export(ctx, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Events).To(HaveLen(5), "Should export all 5 events")

				// Verify hash chain validation results
				Expect(result.TotalEventsQueried).To(Equal(5))
				Expect(result.ValidChainEvents).To(Equal(5),
					"All events should have valid hash chain")
				Expect(result.BrokenChainEvents).To(Equal(0))
				Expect(result.ChainIntegrityPercent).To(BeNumerically("==", 100.0))

				// Verify each event's hash_chain_valid flag
				for i, event := range result.Events {
					Expect(event.HashChainValid).To(BeTrue(),
						"Event %d should have valid hash chain", i)
				}
			})
		})

		Context("when audit event is tampered (hash mismatch)", func() {
			It("should detect hash chain tampering", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-verify-tampered", testID)

				// Create 3 events
				var eventIDs []uuid.UUID
				for i := 0; i < 3; i++ {
					event := &repository.AuditEvent{
						EventID:         uuid.New(),
						EventDate:       repository.DateOnly(time.Now()),
						EventType:       "data.modified",
						EventAction:     fmt.Sprintf("modify-%d", i),
						EventCategory:   "data",
						EventOutcome:    "success",
						CorrelationID:   correlationID,
						ResourceType:    "database",
						ResourceID:      "test-db",
						ActorID:         "admin",
						ActorType:       "user",
						EventData:       map[string]interface{}{"operation": "update", "index": i},
						Version:         "1",
					}

					created, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
					eventIDs = append(eventIDs, created.EventID)
				}

				// TAMPER with middle event's event_data (simulating malicious modification)
				_, err := db.ExecContext(ctx,
					`UPDATE audit_events
					 SET event_data = '{"operation": "TAMPERED", "malicious": true}'::jsonb
					 WHERE event_id = $1`,
					eventIDs[1])
				Expect(err).ToNot(HaveOccurred())

				// Export and verify
				filters := repository.ExportFilters{
					CorrelationID: correlationID,
				}

				result, err := auditRepo.Export(ctx, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())

				// Verify tampering is detected
				Expect(result.ValidChainEvents).To(BeNumerically("<", 3),
					"Tampering should be detected")
				Expect(result.BrokenChainEvents).To(BeNumerically(">", 0),
					"At least one event should have invalid hash")
				Expect(result.ChainIntegrityPercent).To(BeNumerically("<", 100.0),
					"Chain integrity should be < 100%")

				// Verify tampered event is flagged
				var tamperedEventDetected bool
				for _, event := range result.Events {
					if event.EventID == eventIDs[1] {
						Expect(event.HashChainValid).To(BeFalse(),
							"Tampered event should have invalid hash chain")
						tamperedEventDetected = true
					}
				}
				Expect(tamperedEventDetected).To(BeTrue(), "Tampered event should be in export")
			})
		})

		Context("when previous_event_hash linkage is broken", func() {
			It("should detect broken chain linkage", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-verify-broken", testID)

				// Create 3 events
				var eventIDs []uuid.UUID
				for i := 0; i < 3; i++ {
					event := &repository.AuditEvent{
						EventID:         uuid.New(),
						EventDate:       repository.DateOnly(time.Now()),
						EventType:       "system.operation",
						EventAction:     fmt.Sprintf("operation-%d", i),
						EventCategory:   "system",
						EventOutcome:    "success",
						CorrelationID:   correlationID,
						ResourceType:    "cluster",
						ResourceID:      "prod-cluster",
						ActorID:         "system",
						ActorType:       "service_account",
						EventData:       map[string]interface{}{"op": i},
						Version:         "1",
					}

					created, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
					eventIDs = append(eventIDs, created.EventID)
				}

				// BREAK the chain by modifying previous_event_hash of event 2
				_, err := db.ExecContext(ctx,
					`UPDATE audit_events
					 SET previous_event_hash = 'INVALID_HASH_BREAKS_CHAIN'
					 WHERE event_id = $1`,
					eventIDs[2])
				Expect(err).ToNot(HaveOccurred())

				// Export and verify
				filters := repository.ExportFilters{
					CorrelationID: correlationID,
				}

				result, err := auditRepo.Export(ctx, filters)
				Expect(err).ToNot(HaveOccurred())

				// Verify broken chain is detected
				Expect(result.BrokenChainEvents).To(BeNumerically(">", 0),
					"Broken chain linkage should be detected")

				// Event 2 should be flagged as invalid
				for _, event := range result.Events {
					if event.EventID == eventIDs[2] {
						Expect(event.HashChainValid).To(BeFalse(),
							"Event with broken previous_event_hash should be invalid")
					}
				}
			})
		})
	})

	// ========================================
	// EXPORT METADATA TESTS
	// ========================================
	Describe("Export Metadata", func() {
		Context("when exporting with filters", func() {
			It("should include correct export metadata", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-metadata", testID)

				// Create test events
				for i := 0; i < 3; i++ {
					event := &repository.AuditEvent{
						EventID:         uuid.New(),
						EventDate:       repository.DateOnly(time.Now()),
						EventType:       "test.event",
						EventAction:     "test",
						EventCategory:   "test",
						EventOutcome:    "success",
						CorrelationID:   correlationID,
						ResourceType:    "test",
						ResourceID:      "test-resource",
						ActorID:         "test-user",
						ActorType:       "user",
						EventData:       map[string]interface{}{"index": i},
						Version:         "1",
					}

					_, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
				}

				// Export
				filters := repository.ExportFilters{
					CorrelationID: correlationID,
					RedactPII:     false,
				}

				result, err := auditRepo.Export(ctx, filters)
				Expect(err).ToNot(HaveOccurred())

				// Verify export result metadata
				Expect(result.TotalEventsQueried).To(Equal(3))
				Expect(result.VerificationTimestamp).ToNot(BeZero())
				Expect(result.ChainIntegrityPercent).To(BeNumerically(">=", 0.0))
			})
		})
	})

	// ========================================
	// HASH CALCULATION CORRECTNESS TESTS
	// ========================================
	Describe("Hash Calculation Algorithm", func() {
		Context("when calculating event hash", func() {
			It("should produce consistent hash for same event data", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-hash-calc", testID)

				// Create first event
				event1 := &repository.AuditEvent{
					EventID:         uuid.New(),
					EventDate:       repository.DateOnly(time.Now()),
					EventType:       "test.hash",
					EventAction:     "test-hash-consistency",
					EventCategory:   "test",
					EventOutcome:    "success",
					CorrelationID:   correlationID,
					ResourceType:    "test",
					ResourceID:      "hash-test",
					ActorID:         "tester",
					ActorType:       "user",
					EventData:       map[string]interface{}{"key": "value", "number": 42},
					Version:         "1",
				}

				created, err := auditRepo.Create(ctx, event1)
				Expect(err).ToNot(HaveOccurred())

				// Manually calculate what the hash should be
				// Hash = SHA256(previous_hash + event_json)
				// For first event, previous_hash is empty string
				eventForHashing := *created
				eventForHashing.EventHash = ""
				eventForHashing.PreviousEventHash = ""
				eventForHashing.EventDate = repository.DateOnly{} // Clear generated field

				eventJSON, err := json.Marshal(eventForHashing)
				Expect(err).ToNot(HaveOccurred())

				hasher := sha256.New()
				hasher.Write([]byte("")) // previous_hash is empty for first event
				hasher.Write(eventJSON)
				expectedHash := hex.EncodeToString(hasher.Sum(nil))

				// Verify stored hash matches calculation
				Expect(created.EventHash).To(Equal(expectedHash),
					"Stored hash should match SHA256(previous_hash + event_json)")
			})

			It("should include previous hash in chain calculation", func() {
				correlationID := fmt.Sprintf("soc2-export-%s-hash-chain-calc", testID)

				// Create first event
				event1 := &repository.AuditEvent{
					EventID:         uuid.New(),
					EventDate:       repository.DateOnly(time.Now()),
					EventType:       "test.chain",
					EventAction:     "first",
					EventCategory:   "test",
					EventOutcome:    "success",
					CorrelationID:   correlationID,
					ResourceType:    "test",
					ResourceID:      "chain-test",
					ActorID:         "tester",
					ActorType:       "user",
					EventData:       map[string]interface{}{"order": 1},
					Version:         "1",
				}

				first, err := auditRepo.Create(ctx, event1)
				Expect(err).ToNot(HaveOccurred())

				// Create second event
				event2 := &repository.AuditEvent{
					EventID:         uuid.New(),
					EventDate:       repository.DateOnly(time.Now()),
					EventType:       "test.chain",
					EventAction:     "second",
					EventCategory:   "test",
					EventOutcome:    "success",
					CorrelationID:   correlationID,
					ResourceType:    "test",
					ResourceID:      "chain-test",
					ActorID:         "tester",
					ActorType:       "user",
					EventData:       map[string]interface{}{"order": 2},
					Version:         "1",
				}

				second, err := auditRepo.Create(ctx, event2)
				Expect(err).ToNot(HaveOccurred())

				// Manually calculate second event's hash
				eventForHashing := *second
				eventForHashing.EventHash = ""
				eventForHashing.PreviousEventHash = ""
				eventForHashing.EventDate = repository.DateOnly{}

				eventJSON, err := json.Marshal(eventForHashing)
				Expect(err).ToNot(HaveOccurred())

				hasher := sha256.New()
				hasher.Write([]byte(first.EventHash)) // Include previous hash
				hasher.Write(eventJSON)
				expectedHash := hex.EncodeToString(hasher.Sum(nil))

				// Verify second event's hash includes first event's hash
				Expect(second.EventHash).To(Equal(expectedHash),
					"Second event hash should include first event's hash in calculation")
			})
		})
	})
})

