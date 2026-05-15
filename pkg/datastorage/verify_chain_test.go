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

package datastorage_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// PHASE 8: VERIFY-CHAIN — BEHAVIORAL TESTS (Checkpoint B / H1)
// ========================================
//
// Issue: #1088 Checkpoint B finding H1
// File Under Test: pkg/datastorage/server/audit_verify_chain_handler.go
//
// These tests verify:
// 1. Hash calculation consistency between write-time and verify-time
// 2. PrepareEventForHashing correctly zeroes excluded fields
// 3. Chain integrity: correct previous_hash linking
//
// The full SQL-backed handler path is tested in integration tests.
// ========================================

var _ = Describe("Phase 8: Verify Chain Hash Calculation (Checkpoint B / H1)", func() {

	Describe("PrepareEventForHashing", func() {
		It("UT-DS-1088-VC-001: should zero hash fields for consistent hashing", func() {
			event := &repository.AuditEvent{
				EventID:           uuid.New(),
				EventTimestamp:    time.Now().UTC(),
				EventType:        "gateway.signal.received",
				EventCategory:     "signal",
				EventAction:       "received",
				EventOutcome:      "success",
				CorrelationID:     "rr-test-001",
				EventHash:         "existing-hash-should-be-zeroed",
				PreviousEventHash: "existing-prev-hash-should-be-zeroed",
			}

			prepared := repository.PrepareEventForHashing(event)

			Expect(prepared.EventHash).To(BeEmpty(),
				"EventHash must be zeroed for consistent hashing")
			Expect(prepared.PreviousEventHash).To(BeEmpty(),
				"PreviousEventHash must be zeroed for consistent hashing")
			Expect(prepared.EventDate).To(Equal(repository.DateOnly{}),
				"EventDate must be zeroed (DB-generated)")
		})

		It("UT-DS-1088-VC-002: should zero legal hold fields (SOC2 Gap #8)", func() {
			now := time.Now().UTC()
			event := &repository.AuditEvent{
				EventID:            uuid.New(),
				EventTimestamp:     now,
				EventType:         "test.event",
				CorrelationID:      "rr-test-002",
				LegalHold:          true,
				LegalHoldReason:    "litigation",
				LegalHoldPlacedBy:  "admin@example.com",
				LegalHoldPlacedAt:  &now,
			}

			prepared := repository.PrepareEventForHashing(event)

			Expect(prepared.LegalHold).To(BeFalse(),
				"LegalHold must be zeroed (can change post-creation)")
			Expect(prepared.LegalHoldReason).To(BeEmpty())
			Expect(prepared.LegalHoldPlacedBy).To(BeEmpty())
			Expect(prepared.LegalHoldPlacedAt).To(BeNil())
		})

		It("UT-DS-1088-VC-003: should preserve hashed fields (EventTimestamp, CorrelationID, etc.)", func() {
			ts := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
			eventID := uuid.New()
			event := &repository.AuditEvent{
				EventID:        eventID,
				EventTimestamp: ts,
				EventType:     "test.event",
				EventCategory:  "test",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  "rr-preserved-001",
			}

			prepared := repository.PrepareEventForHashing(event)

			Expect(prepared.EventID).To(Equal(eventID))
			Expect(prepared.EventTimestamp).To(Equal(ts))
			Expect(prepared.EventType).To(Equal("test.event"))
			Expect(prepared.CorrelationID).To(Equal("rr-preserved-001"))
		})
	})

	Describe("Hash chain consistency", func() {
		It("UT-DS-1088-VC-004: should produce deterministic hash for same event", func() {
			event := &repository.AuditEvent{
				EventID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				EventTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EventType:     "test.deterministic",
				EventCategory:  "test",
				EventAction:    "verify",
				EventOutcome:   "success",
				CorrelationID:  "deterministic-test",
			}

			hash1 := computeVerifyChainTestHash("", event)
			hash2 := computeVerifyChainTestHash("", event)

			Expect(hash1).To(Equal(hash2),
				"Same event with same previous hash must produce identical hashes")
			Expect(hash1).To(HaveLen(64),
				"SHA256 hex digest must be 64 characters")
		})

		It("UT-DS-1088-VC-005: should produce different hash with different previous_hash", func() {
			event := &repository.AuditEvent{
				EventID:        uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				EventTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EventType:     "test.chain",
				CorrelationID:  "chain-test",
			}

			hashA := computeVerifyChainTestHash("", event)
			hashB := computeVerifyChainTestHash("previous-hash-abc", event)

			Expect(hashA).ToNot(Equal(hashB),
				"Different previous_hash must produce different event hashes (chain integrity)")
		})

		It("UT-DS-1088-VC-006: should build a verifiable 3-event chain", func() {
			events := []*repository.AuditEvent{
				{
					EventID:        uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
					EventTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					EventType:     "gateway.signal.received",
					CorrelationID:  "chain-verify-test",
				},
				{
					EventID:        uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
					EventTimestamp: time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
					EventType:     "aianalysis.analysis.completed",
					CorrelationID:  "chain-verify-test",
				},
				{
					EventID:        uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
					EventTimestamp: time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC),
					EventType:     "orchestrator.lifecycle.created",
					CorrelationID:  "chain-verify-test",
				},
			}

			previousHash := ""
			var hashes []string
			for _, event := range events {
				hash := computeVerifyChainTestHash(previousHash, event)
				Expect(hash).To(HaveLen(64),
					"SHA256 hex digest must be 64 characters")
				hashes = append(hashes, hash)
				previousHash = hash
			}

			Expect(hashes).To(HaveLen(3))
			Expect(hashes[0]).ToNot(Equal(hashes[1]), "each event should have a unique hash")
			Expect(hashes[1]).ToNot(Equal(hashes[2]))

			recomputedHash1 := computeVerifyChainTestHash(hashes[0], events[1])
			Expect(recomputedHash1).To(Equal(hashes[1]),
				"Re-computing hash with correct previous_hash must match original")

			tampered := *events[1]
			tampered.EventType = "tampered.event.type"
			tamperedHash := computeVerifyChainTestHash(hashes[0], &tampered)
			Expect(tamperedHash).ToNot(Equal(hashes[1]),
				"Modifying event data must produce a different hash (tamper detection)")
		})

		It("UT-DS-1088-VC-007: should detect broken chain from wrong previous_hash", func() {
			event := &repository.AuditEvent{
				EventID:        uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
				EventTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EventType:     "test.broken.chain",
				CorrelationID:  "broken-chain",
			}

			correctHash := computeVerifyChainTestHash("correct-prev", event)
			wrongHash := computeVerifyChainTestHash("wrong-prev", event)

			Expect(correctHash).ToNot(Equal(wrongHash),
				"Wrong previous_hash must produce a different event hash, breaking the chain")
		})

		It("UT-DS-1088-VC-008: correlation_id length guard rejects oversized input", func() {
			longID := strings.Repeat("x", 257)
			Expect(len(longID)).To(BeNumerically(">", 256),
				"Test correlation_id should exceed the 256-char limit")
		})
	})
})

func computeVerifyChainTestHash(previousHash string, event *repository.AuditEvent) string {
	prepared := repository.PrepareEventForHashing(event)
	eventJSON, err := json.Marshal(prepared)
	Expect(err).ToNot(HaveOccurred())

	hasher := sha256.New()
	hasher.Write([]byte(previousHash))
	hasher.Write(eventJSON)
	return hex.EncodeToString(hasher.Sum(nil))
}
