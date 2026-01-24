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

var _ = Describe("Hash Chain DB Round-Trip Investigation", func() {
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
			fmt.Sprintf("%%-hash-roundtrip-%s%%", testID))
		Expect(err).ToNot(HaveOccurred())
	})

	It("should produce identical hash before and after PostgreSQL round-trip", func() {
		correlationID := fmt.Sprintf("test-hash-roundtrip-%s", testID)

		// ========================================
		// STEP 1: Create event WITH specific timestamp
		// ========================================
		originalTimestamp := time.Date(2026, 1, 24, 10, 30, 45, 123456789, time.UTC)

		eventBeforeDB := &repository.AuditEvent{
			EventID:        uuid.New(),
			EventTimestamp: originalTimestamp,
			EventType:      "test.hash.check",
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
			IsSensitive:    false,
			EventData:      map[string]interface{}{"step": 1, "status": "test"},
		}

		GinkgoWriter.Printf("\n========================================\n")
		GinkgoWriter.Printf("BEFORE DB INSERT:\n")
		GinkgoWriter.Printf("========================================\n")
		GinkgoWriter.Printf("EventID:        %s\n", eventBeforeDB.EventID)
		GinkgoWriter.Printf("EventTimestamp: %s (precision: nanosecond)\n", eventBeforeDB.EventTimestamp.Format(time.RFC3339Nano))
		GinkgoWriter.Printf("EventData:      %+v\n", eventBeforeDB.EventData)
		GinkgoWriter.Printf("ParentEventID:  %v\n", eventBeforeDB.ParentEventID)

		// ========================================
		// STEP 2: Insert into PostgreSQL
		// ========================================
		created, err := auditRepo.Create(ctx, eventBeforeDB)
		Expect(err).ToNot(HaveOccurred())
		Expect(created).ToNot(BeNil())

		GinkgoWriter.Printf("\nEvent created with hash: %s\n", created.EventHash)

		// Wait for transaction to commit
		time.Sleep(100 * time.Millisecond)

		// ========================================
		// STEP 3: Read back from PostgreSQL
		// ========================================
		filters := repository.ExportFilters{
			CorrelationID: correlationID,
		}
		result, err := auditRepo.Export(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))

		eventAfterDB := result.Events[0].AuditEvent

		GinkgoWriter.Printf("\n========================================\n")
		GinkgoWriter.Printf("AFTER DB ROUND-TRIP:\n")
		GinkgoWriter.Printf("========================================\n")
		GinkgoWriter.Printf("EventID:        %s\n", eventAfterDB.EventID)
		GinkgoWriter.Printf("EventTimestamp: %s (precision: %s)\n",
			eventAfterDB.EventTimestamp.Format(time.RFC3339Nano),
			"microsecond (PostgreSQL truncates)")
		GinkgoWriter.Printf("EventData:      %+v (types: %T)\n", eventAfterDB.EventData, eventAfterDB.EventData)
		GinkgoWriter.Printf("ParentEventID:  %v\n", eventAfterDB.ParentEventID)
		GinkgoWriter.Printf("Stored hash:    %s\n", eventAfterDB.EventHash)

		// ========================================
		// STEP 4: Prepare both events for hashing
		// ========================================
		// Simulate calculateEventHash() logic
		prepareForHashing := func(event *repository.AuditEvent, label string) (string, []byte) {
			eventCopy := *event
			eventCopy.EventHash = ""
			eventCopy.PreviousEventHash = ""
			eventCopy.EventDate = repository.DateOnly{}
			eventCopy.LegalHold = false
			eventCopy.LegalHoldReason = ""
			eventCopy.LegalHoldPlacedBy = ""
			eventCopy.LegalHoldPlacedAt = nil

			eventJSON, err := json.Marshal(eventCopy)
			Expect(err).ToNot(HaveOccurred())

			hasher := sha256.New()
			hasher.Write([]byte("")) // empty previous hash
			hasher.Write(eventJSON)
			hashBytes := hasher.Sum(nil)
			hash := hex.EncodeToString(hashBytes)

			GinkgoWriter.Printf("\n%s JSON:\n%s\n", label, string(eventJSON))
			GinkgoWriter.Printf("%s HASH: %s\n", label, hash)

			return hash, eventJSON
		}

		hashBefore, jsonBefore := prepareForHashing(eventBeforeDB, "BEFORE DB")
		hashAfter, jsonAfter := prepareForHashing(eventAfterDB, "AFTER DB")

		// ========================================
		// STEP 5: Compare JSONs byte-by-byte
		// ========================================
		GinkgoWriter.Printf("\n========================================\n")
		GinkgoWriter.Printf("JSON COMPARISON:\n")
		GinkgoWriter.Printf("========================================\n")

		if string(jsonBefore) == string(jsonAfter) {
			GinkgoWriter.Printf("✅ JSONs are IDENTICAL\n")
		} else {
			GinkgoWriter.Printf("❌ JSONs are DIFFERENT\n")
			GinkgoWriter.Printf("\nBefore length: %d bytes\n", len(jsonBefore))
			GinkgoWriter.Printf("After length:  %d bytes\n", len(jsonAfter))

			// Unmarshal to compare structure
			var beforeMap, afterMap map[string]interface{}
			_ = json.Unmarshal(jsonBefore, &beforeMap)
			_ = json.Unmarshal(jsonAfter, &afterMap)

			GinkgoWriter.Printf("\nDIFFERENCES:\n")
			for key := range beforeMap {
				beforeVal := fmt.Sprintf("%v", beforeMap[key])
				afterVal := fmt.Sprintf("%v", afterMap[key])
				if beforeVal != afterVal {
					GinkgoWriter.Printf("  %s:\n", key)
					GinkgoWriter.Printf("    BEFORE: %s (type: %T)\n", beforeVal, beforeMap[key])
					GinkgoWriter.Printf("    AFTER:  %s (type: %T)\n", afterVal, afterMap[key])
				}
			}
		}

		// ========================================
		// STEP 6: Compare Hashes
		// ========================================
		GinkgoWriter.Printf("\n========================================\n")
		GinkgoWriter.Printf("HASH COMPARISON:\n")
		GinkgoWriter.Printf("========================================\n")
		GinkgoWriter.Printf("Hash BEFORE DB: %s\n", hashBefore)
		GinkgoWriter.Printf("Hash AFTER DB:  %s\n", hashAfter)
		GinkgoWriter.Printf("Stored hash:    %s\n", eventAfterDB.EventHash)

		if hashBefore == hashAfter {
			GinkgoWriter.Printf("✅ Hashes are IDENTICAL\n")
		} else {
			GinkgoWriter.Printf("❌ Hashes are DIFFERENT (THIS IS THE BUG!)\n")
		}

		// ========================================
		// STEP 7: Verify hash chain validation
		// ========================================
		if !result.Events[0].HashChainValid {
			GinkgoWriter.Printf("\n❌ Hash chain validation FAILED in Export\n")
			GinkgoWriter.Printf("Expected hash: %s\n", hashAfter)
			GinkgoWriter.Printf("Actual hash:   %s\n", eventAfterDB.EventHash)
		} else {
			GinkgoWriter.Printf("\n✅ Hash chain validation PASSED\n")
		}

		// ========================================
		// ASSERTIONS
		// ========================================
		Expect(string(jsonBefore)).To(Equal(string(jsonAfter)),
			"JSON must be identical before and after DB round-trip for hash chain to work")
		Expect(hashBefore).To(Equal(hashAfter),
			"Hash must be identical before and after DB round-trip")
		Expect(result.Events[0].HashChainValid).To(BeTrue(),
			"Export should validate hash chain as valid")
	})
})
