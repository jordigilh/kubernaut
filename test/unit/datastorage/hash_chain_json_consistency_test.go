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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

var _ = Describe("Hash Chain JSON Consistency - Root Cause Investigation", func() {
	Context("when comparing JSON marshaling during creation vs verification", func() {
		It("should produce identical JSON for the same event (omitempty hypothesis test)", func() {
			// SCENARIO: Testing the hypothesis that omitempty + sql.NullString conversions
			// cause JSON differences between creation and verification.
			//
			// During CREATION:
			//   - Optional fields are zero values (e.g., ResourceType = "")
			//   - json.Marshal with omitempty OMITS these fields from JSON
			//
			// During VERIFICATION:
			//   - PostgreSQL NULLs are converted via sql.NullString
			//   - sql.NullString.String converts NULL -> "" (explicitly set)
			//   - json.Marshal includes these fields even though they're empty
			//     (because they were explicitly set, not zero values)

			// ========================================
			// STEP 1: Simulate EVENT CREATION
			// ========================================
			eventForCreation := &repository.AuditEvent{
				EventID:        uuid.New(),
				EventTimestamp: time.Date(2026, 1, 24, 10, 0, 0, 0, time.UTC),
				EventDate:      repository.DateOnly{},
				EventType:      "workflow.executed",
				Version:        "1.0",
				EventCategory:  "workflow",
				EventAction:    "execute",
				EventOutcome:   "success",
				CorrelationID:  "test-correlation-001",
				ParentEventID:  nil,
				ParentEventDate: nil,
				// CRITICAL: Optional fields left as ZERO VALUES (not explicitly set)
				ResourceType:      "", // Zero value - omitempty will OMIT from JSON
				ResourceID:        "", // Zero value - omitempty will OMIT from JSON
				ResourceNamespace: "", // Zero value - omitempty will OMIT from JSON
				ClusterID:         "", // Zero value - omitempty will OMIT from JSON
				ActorID:           "", // Zero value - omitempty will OMIT from JSON
				ActorType:         "", // Zero value - omitempty will OMIT from JSON
				Severity:          "", // Zero value - omitempty will OMIT from JSON
				DurationMs:        0,  // Zero value - omitempty will OMIT from JSON
				ErrorCode:         "", // Zero value - omitempty will OMIT from JSON
				ErrorMessage:      "", // Zero value - omitempty will OMIT from JSON
				RetentionDays:     2555,
				IsSensitive:       false,
				EventHash:         "",
				PreviousEventHash: "",
				LegalHold:         false,
				LegalHoldReason:   "",
				LegalHoldPlacedBy: "",
				LegalHoldPlacedAt: nil,
				EventData:         map[string]interface{}{"step": 1, "status": "completed"},
			}

			// Normalize EventData (simulate what happens in calculateEventHash)
			eventDataJSON, err := json.Marshal(eventForCreation.EventData)
			Expect(err).ToNot(HaveOccurred())
			var normalizedEventData map[string]interface{}
			err = json.Unmarshal(eventDataJSON, &normalizedEventData)
			Expect(err).ToNot(HaveOccurred())
			eventForCreation.EventData = normalizedEventData

			// Clear fields that are excluded from hash (simulate calculateEventHash logic)
			creationCopy := *eventForCreation
			creationCopy.EventHash = ""
			creationCopy.PreviousEventHash = ""
			creationCopy.EventDate = repository.DateOnly{}
			creationCopy.LegalHold = false
			creationCopy.LegalHoldReason = ""
			creationCopy.LegalHoldPlacedBy = ""
			creationCopy.LegalHoldPlacedAt = nil

			// Marshal to JSON (this is what gets hashed during CREATION)
			creationJSON, err := json.Marshal(creationCopy)
			Expect(err).ToNot(HaveOccurred())

			GinkgoWriter.Printf("\n========================================\n")
			GinkgoWriter.Printf("CREATION JSON (with omitempty):\n")
			GinkgoWriter.Printf("%s\n", string(creationJSON))
			GinkgoWriter.Printf("========================================\n\n")

			// ========================================
			// STEP 2: Simulate VERIFICATION (after DB round-trip)
			// ========================================
			// Simulate what happens in audit_export.go when reading from PostgreSQL

			// Simulate sql.NullString conversions (lines 197-213 in audit_export.go)
			var resourceType sql.NullString   // NULL from DB
			var resourceID sql.NullString     // NULL from DB
			var resourceNamespace sql.NullString // NULL from DB
			var clusterID sql.NullString      // NULL from DB
			var actorID sql.NullString        // NULL from DB
			var actorType sql.NullString      // NULL from DB
			var severity sql.NullString       // NULL from DB
			var errorCode sql.NullString      // NULL from DB
			var errorMessage sql.NullString   // NULL from DB
			var durationMs sql.NullInt64      // NULL from DB

			// Create verification event and assign from sql.NullString
			// (this is what audit_export.go does)
			eventForVerification := &repository.AuditEvent{
				EventID:        eventForCreation.EventID,
				EventTimestamp: eventForCreation.EventTimestamp.UTC(), // Force UTC (line 195 audit_export.go)
				EventDate:      repository.DateOnly{},
				EventType:      "workflow.executed",
				Version:        "1.0",
				EventCategory:  "workflow",
				EventAction:    "execute",
				EventOutcome:   "success",
				CorrelationID:  "test-correlation-001",
				ParentEventID:  nil,
				ParentEventDate: nil,
				// CRITICAL: These are EXPLICITLY SET from sql.NullString.String
				// NULL -> "" conversion (lines 200-212 in audit_export.go)
				ResourceType:      resourceType.String,      // NULL -> "" (EXPLICITLY SET)
				ResourceID:        resourceID.String,        // NULL -> "" (EXPLICITLY SET)
				ResourceNamespace: resourceNamespace.String, // NULL -> "" (EXPLICITLY SET)
				ClusterID:         clusterID.String,         // NULL -> "" (EXPLICITLY SET)
				ActorID:           actorID.String,           // NULL -> "" (EXPLICITLY SET)
				ActorType:         actorType.String,         // NULL -> "" (EXPLICITLY SET)
				Severity:          severity.String,          // NULL -> "" (EXPLICITLY SET)
				DurationMs:        int(durationMs.Int64),    // NULL -> 0 (EXPLICITLY SET)
				ErrorCode:         errorCode.String,         // NULL -> "" (EXPLICITLY SET)
				ErrorMessage:      errorMessage.String,      // NULL -> "" (EXPLICITLY SET)
				RetentionDays:     2555,
				IsSensitive:       false,
				EventHash:         "stored_hash_value",
				PreviousEventHash: "previous_hash_value",
				LegalHold:         false,
				LegalHoldReason:   "",
				LegalHoldPlacedBy: "",
				LegalHoldPlacedAt: nil,
				EventData:         normalizedEventData, // Already normalized from DB JSONB
			}

			// Clear fields that are excluded from hash (simulate calculateEventHashForVerification logic)
			verificationCopy := *eventForVerification
			verificationCopy.EventHash = ""
			verificationCopy.PreviousEventHash = ""
			verificationCopy.EventDate = repository.DateOnly{}
			verificationCopy.LegalHold = false
			verificationCopy.LegalHoldReason = ""
			verificationCopy.LegalHoldPlacedBy = ""
			verificationCopy.LegalHoldPlacedAt = nil

			// Marshal to JSON (this is what gets hashed during VERIFICATION)
			verificationJSON, err := json.Marshal(verificationCopy)
			Expect(err).ToNot(HaveOccurred())

			GinkgoWriter.Printf("VERIFICATION JSON (after sql.NullString conversion):\n")
			GinkgoWriter.Printf("%s\n", string(verificationJSON))
			GinkgoWriter.Printf("========================================\n\n")

			// ========================================
			// STEP 3: Compare JSON
			// ========================================
			GinkgoWriter.Printf("JSON COMPARISON:\n")
			if string(creationJSON) == string(verificationJSON) {
				GinkgoWriter.Printf("✅ JSONs are IDENTICAL\n")
			} else {
				GinkgoWriter.Printf("❌ JSONs are DIFFERENT\n")
				GinkgoWriter.Printf("\nCreation JSON length:     %d bytes\n", len(creationJSON))
				GinkgoWriter.Printf("Verification JSON length: %d bytes\n", len(verificationJSON))

				// Unmarshal both to compare structure
				var creationMap map[string]interface{}
				var verificationMap map[string]interface{}
				_ = json.Unmarshal(creationJSON, &creationMap)
				_ = json.Unmarshal(verificationJSON, &verificationMap)

				GinkgoWriter.Printf("\nFields in CREATION only:\n")
				for key := range creationMap {
					if _, exists := verificationMap[key]; !exists {
						GinkgoWriter.Printf("  - %s: %v\n", key, creationMap[key])
					}
				}

				GinkgoWriter.Printf("\nFields in VERIFICATION only:\n")
				for key := range verificationMap {
					if _, exists := creationMap[key]; !exists {
						GinkgoWriter.Printf("  - %s: %v\n", key, verificationMap[key])
					}
				}

				GinkgoWriter.Printf("\nFields with DIFFERENT values:\n")
				for key := range creationMap {
					if verificationValue, exists := verificationMap[key]; exists {
						creationValue := creationMap[key]
						if creationValue != verificationValue {
							GinkgoWriter.Printf("  - %s: creation=%v, verification=%v\n", key, creationValue, verificationValue)
						}
					}
				}
			}

			// ========================================
			// STEP 4: ASSERTION
			// ========================================
			// This test SHOULD FAIL if the hypothesis is correct
			// (omitempty causes different JSON output)
			Expect(string(creationJSON)).To(Equal(string(verificationJSON)),
				"Hash chain failure root cause: Creation JSON != Verification JSON due to omitempty behavior")
		})

		It("should demonstrate that omitempty omits fields with zero values", func() {
			// PROOF OF CONCEPT: Show that omitempty behaves differently
			// depending on whether a field is a zero value or explicitly set to zero

			type TestStruct struct {
				Field1 string `json:"field1,omitempty"`
				Field2 string `json:"field2,omitempty"`
			}

			// Scenario 1: Zero value (never set)
			zeroValueStruct := TestStruct{
				// Fields not set - remain as zero values
			}
			zeroValueJSON, _ := json.Marshal(zeroValueStruct)

			// Scenario 2: Explicitly set to empty string
			explicitEmptyStruct := TestStruct{
				Field1: "", // Explicitly set to empty
				Field2: "", // Explicitly set to empty
			}
			explicitEmptyJSON, _ := json.Marshal(explicitEmptyStruct)

			GinkgoWriter.Printf("\n========================================\n")
			GinkgoWriter.Printf("OMITEMPTY BEHAVIOR PROOF:\n")
			GinkgoWriter.Printf("========================================\n")
			GinkgoWriter.Printf("Zero value (never set):     %s\n", string(zeroValueJSON))
			GinkgoWriter.Printf("Explicitly set to empty:    %s\n", string(explicitEmptyJSON))
			GinkgoWriter.Printf("========================================\n\n")

			// Both SHOULD be the same, but with omitempty they might not be
			// (depending on Go version and struct state)
			if string(zeroValueJSON) == string(explicitEmptyJSON) {
				GinkgoWriter.Printf("✅ SAME: omitempty is NOT the issue\n")
			} else {
				GinkgoWriter.Printf("❌ DIFFERENT: omitempty IS the issue\n")
			}
		})

		It("should demonstrate EventData normalization (int to float64)", func() {
			// PROOF OF CONCEPT: Show that PostgreSQL JSONB normalizes numbers

			// Original EventData (with int)
			originalEventData := map[string]interface{}{
				"step":   1,     // int
				"count":  42,    // int
				"status": "completed",
			}

			// Normalize through JSON round-trip (simulates PostgreSQL JSONB)
			eventDataJSON, err := json.Marshal(originalEventData)
			Expect(err).ToNot(HaveOccurred())

			var normalizedEventData map[string]interface{}
			err = json.Unmarshal(eventDataJSON, &normalizedEventData)
			Expect(err).ToNot(HaveOccurred())

			GinkgoWriter.Printf("\n========================================\n")
			GinkgoWriter.Printf("EVENTDATA NORMALIZATION PROOF:\n")
			GinkgoWriter.Printf("========================================\n")
			GinkgoWriter.Printf("Original EventData:    %T (step=%T)\n", originalEventData["step"], originalEventData["step"])
			GinkgoWriter.Printf("Normalized EventData:  %T (step=%T)\n", normalizedEventData["step"], normalizedEventData["step"])
			GinkgoWriter.Printf("\nOriginal JSON:    %s\n", string(eventDataJSON))

			normalizedJSON, _ := json.Marshal(normalizedEventData)
			GinkgoWriter.Printf("Normalized JSON:  %s\n", string(normalizedJSON))
			GinkgoWriter.Printf("========================================\n\n")

			// Check if normalization changes the JSON
			if string(eventDataJSON) == string(normalizedJSON) {
				GinkgoWriter.Printf("✅ SAME: EventData normalization is NOT the issue\n")
			} else {
				GinkgoWriter.Printf("❌ DIFFERENT: EventData normalization IS an issue\n")
			}
		})

		It("should demonstrate timestamp UTC conversion impact", func() {
			// PROOF OF CONCEPT: Show that timezone affects JSON output

			timestamp := time.Date(2026, 1, 24, 10, 0, 0, 123456789, time.UTC)

			// Scenario 1: UTC timestamp
			utcTimestamp := timestamp.UTC()

			// Scenario 2: Local timezone (simulates reading from PostgreSQL)
			localTimestamp := timestamp.In(time.Local)

			// Convert back to UTC (as done in audit_export.go line 195)
			convertedTimestamp := localTimestamp.UTC()

			type TimestampStruct struct {
				Timestamp time.Time `json:"timestamp"`
			}

			utcJSON, _ := json.Marshal(TimestampStruct{Timestamp: utcTimestamp})
			localJSON, _ := json.Marshal(TimestampStruct{Timestamp: localTimestamp})
			convertedJSON, _ := json.Marshal(TimestampStruct{Timestamp: convertedTimestamp})

			GinkgoWriter.Printf("\n========================================\n")
			GinkgoWriter.Printf("TIMESTAMP TIMEZONE PROOF:\n")
			GinkgoWriter.Printf("========================================\n")
			GinkgoWriter.Printf("UTC:              %s\n", string(utcJSON))
			GinkgoWriter.Printf("Local:            %s\n", string(localJSON))
			GinkgoWriter.Printf("Converted to UTC: %s\n", string(convertedJSON))
			GinkgoWriter.Printf("========================================\n\n")

			if string(utcJSON) == string(convertedJSON) {
				GinkgoWriter.Printf("✅ SAME: Timezone conversion is correct\n")
			} else {
				GinkgoWriter.Printf("❌ DIFFERENT: Timezone conversion is an issue\n")
			}
		})
	})
})
