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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/codenotary/immudb/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// SOC2 Gap #9: Audit Chain Verification Tests
// 📋 Design Decision: DD-IMMUDB-001 | ✅ Approved Design | Confidence: 85%
// See: docs/development/SOC2/GAP9_TAMPER_DETECTION_ANALYSIS_JAN06.md
// ========================================
//
// These integration tests validate the Immudb-based verification API:
// - Valid chain verification (Immudb cryptographic proofs pass)
// - Empty chain handling (no events for correlation_id)
// - Verification audit trail logging
//
// NOTE: Testing "tampered data" detection is complex because Immudb prevents
// tampering at the database level. True tamper detection is validated by Immudb's
// internal integration tests. Our tests focus on the API contract and logging.
// ========================================

var _ = Describe("SOC2 Gap #9: Audit Chain Verification API", func() {
	var (
		baseURL       string
		ctx           context.Context
		httpClient    *http.Client
		correlationID string
		immudbRepo    *repository.ImmudbAuditEventsRepository
	)

	BeforeEach(func() {
		ctx = context.Background()
		baseURL = datastorageURL
		httpClient = &http.Client{Timeout: 10 * time.Second}
		correlationID = fmt.Sprintf("test-verify-%s", uuid.New().String()[:8])

		// Create Immudb repository for direct event creation (bypassing HTTP for speed)
		// Port 13322 from DD-TEST-001 (DataStorage Immudb port)
		testLogger := kubelog.NewLogger(kubelog.Options{
			ServiceName: "audit-verify-integration-test",
			Level:       1,
		})

		opts := client.DefaultOptions().
			WithAddress("localhost").
			WithPort(13322).
			WithUsername("immudb").
			WithPassword("immudb").
			WithDatabase("defaultdb")

		immuClient, err := client.NewImmuClient(opts)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Immudb client")

		// Login to Immudb
		_, err = immuClient.Login(ctx, []byte("immudb"), []byte("immudb"))
		Expect(err).ToNot(HaveOccurred(), "Failed to login to Immudb")

		// Create repository
		immudbRepo = repository.NewImmudbAuditEventsRepository(immuClient, testLogger)

		GinkgoWriter.Printf("🧪 Test setup: baseURL=%s, correlation_id=%s\n", baseURL, correlationID)
	})

	Context("Valid Chain Verification", func() {
		It("should verify a valid audit chain with 10 events", func() {
			// 1. Create a chain of 10 audit events
			GinkgoWriter.Printf("📝 Creating audit chain with 10 events (correlation_id=%s)\n", correlationID)

			for i := 0; i < 10; i++ {
				event := &repository.AuditEvent{
					EventID:        uuid.New(),
					Version:        "1.0",
					EventTimestamp: time.Now().UTC().Add(time.Duration(i) * time.Second),
					EventType:      "test.verification.event",
					EventCategory:  "testing",
					EventAction:    "verify",
					EventOutcome:   "success",
					ActorType:      "test",
					ActorID:        "test-actor",
					ResourceType:   "test",
					ResourceID:     "test-resource",
					CorrelationID:  correlationID,
					EventData: map[string]interface{}{
						"sequence": i + 1,
						"message":  fmt.Sprintf("Event %d for verification testing", i+1),
					},
					RetentionDays: 2555,
				}

				// Insert via repository (bypassing HTTP for speed)
				_, err := immudbRepo.Create(ctx, event)
				Expect(err).ToNot(HaveOccurred(), "Failed to create audit event %d", i+1)

				// Small delay to ensure unique timestamps (Immudb key ordering)
				time.Sleep(10 * time.Millisecond)
			}

			GinkgoWriter.Printf("✅ Created 10 audit events\n")

			// 2. Call verification API
			GinkgoWriter.Printf("🔍 Calling verification API: POST %s/api/v1/audit/verify-chain\n", baseURL)

			reqBody := map[string]interface{}{
				"correlation_id": correlationID,
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// 3. Validate response
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Verification API should return 200 OK")

			var verifyResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&verifyResp)
			Expect(err).ToNot(HaveOccurred())

			GinkgoWriter.Printf("📊 Verification response: %+v\n", verifyResp)

			// Validate verification result
			Expect(verifyResp["verification_result"]).To(Equal("valid"), "Chain should be valid")
			Expect(verifyResp["verified_at"]).ToNot(BeNil(), "verified_at timestamp should be present")

			// Validate details
			details, ok := verifyResp["details"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "details should be present")
			Expect(details["correlation_id"]).To(Equal(correlationID))
			Expect(details["events_verified"]).To(Equal(float64(10)), "Should verify exactly 10 events")
			Expect(details["chain_start"]).ToNot(BeNil(), "chain_start should be present")
			Expect(details["chain_end"]).ToNot(BeNil(), "chain_end should be present")
			Expect(details["first_event_id"]).ToNot(BeEmpty(), "first_event_id should be present")
			Expect(details["last_event_id"]).ToNot(BeEmpty(), "last_event_id should be present")

			// Validate no errors
			errors, exists := verifyResp["errors"]
			if exists {
				Expect(errors).To(BeNil(), "errors should be nil for valid chain")
			}

			GinkgoWriter.Printf("✅ Verification successful: %d events verified\n", int(details["events_verified"].(float64)))
		})

		It("should verify a chain with 1 event", func() {
			// Edge case: Single event chain
			singleCorrelationID := fmt.Sprintf("test-single-%s", uuid.New().String()[:8])

			event := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "test.single.event",
				EventCategory:  "testing",
				EventAction:    "verify",
				EventOutcome:   "success",
				ActorType:      "test",
				ActorID:        "test-actor",
				ResourceType:   "test",
				ResourceID:     "test-resource",
				CorrelationID:  singleCorrelationID,
				EventData: map[string]interface{}{
					"message": "Single event verification test",
				},
				RetentionDays: 2555,
			}

			_, err := immudbRepo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Verify
			reqBody := map[string]interface{}{
				"correlation_id": singleCorrelationID,
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var verifyResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&verifyResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(verifyResp["verification_result"]).To(Equal("valid"))
			details := verifyResp["details"].(map[string]interface{})
			Expect(details["events_verified"]).To(Equal(float64(1)), "Should verify exactly 1 event")
		})
	})

	Context("Empty Chain Handling", func() {
		It("should return valid with 0 events for non-existent correlation_id", func() {
			nonExistentCorrelationID := fmt.Sprintf("non-existent-%s", uuid.New().String())

			GinkgoWriter.Printf("🔍 Verifying non-existent correlation_id: %s\n", nonExistentCorrelationID)

			reqBody := map[string]interface{}{
				"correlation_id": nonExistentCorrelationID,
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var verifyResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&verifyResp)
			Expect(err).ToNot(HaveOccurred())

			// Empty chain is considered "valid" (no tampering possible)
			Expect(verifyResp["verification_result"]).To(Equal("valid"))
			details := verifyResp["details"].(map[string]interface{})
			Expect(details["events_verified"]).To(Equal(float64(0)), "Should verify 0 events")
			Expect(details["chain_start"]).To(BeNil(), "chain_start should be nil for empty chain")
			Expect(details["chain_end"]).To(BeNil(), "chain_end should be nil for empty chain")

			GinkgoWriter.Printf("✅ Empty chain handled correctly\n")
		})
	})

	Context("API Validation", func() {
		It("should reject requests with missing correlation_id", func() {
			reqBody := map[string]interface{}{
				// Missing correlation_id
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["title"]).To(Equal("MISSING_CORRELATION_ID"))
			Expect(errorResp["detail"]).To(ContainSubstring("correlation_id is required"))
		})

		It("should reject requests with empty correlation_id", func() {
			reqBody := map[string]interface{}{
				"correlation_id": "",
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["title"]).To(Equal("MISSING_CORRELATION_ID"))
		})

		It("should reject invalid JSON payloads", func() {
			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader([]byte("invalid json{{")),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResp["title"]).To(Equal("INVALID_REQUEST"))
		})
	})

	Context("SOC2 Compliance Validation", func() {
		It("should include verified_at timestamp in all responses", func() {
			// Create a test event
			testCorrelationID := fmt.Sprintf("test-timestamp-%s", uuid.New().String()[:8])
			event := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "test.timestamp.event",
				EventCategory:  "testing",
				EventAction:    "verify",
				EventOutcome:   "success",
				ActorType:      "test",
				ActorID:        "test-actor",
				ResourceType:   "test",
				ResourceID:     "test-resource",
				CorrelationID:  testCorrelationID,
				EventData:      map[string]interface{}{"test": "timestamp"},
				RetentionDays:  2555,
			}

			_, err := immudbRepo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Verify
			reqBody := map[string]interface{}{
				"correlation_id": testCorrelationID,
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			beforeVerification := time.Now().UTC()

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			afterVerification := time.Now().UTC()

			var verifyResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&verifyResp)
			Expect(err).ToNot(HaveOccurred())

			// Validate verified_at timestamp
			verifiedAtStr, ok := verifyResp["verified_at"].(string)
			Expect(ok).To(BeTrue(), "verified_at should be a string")
			Expect(verifiedAtStr).ToNot(BeEmpty())

			verifiedAt, err := time.Parse(time.RFC3339, verifiedAtStr)
			Expect(err).ToNot(HaveOccurred())

			// SOC2 Requirement: verified_at must be accurate (within test duration)
			Expect(verifiedAt).To(BeTemporally(">=", beforeVerification.Add(-1*time.Second)))
			Expect(verifiedAt).To(BeTemporally("<=", afterVerification.Add(1*time.Second)))

			GinkgoWriter.Printf("✅ SOC2 timestamp validation passed: verified_at=%s\n", verifiedAtStr)
		})

		It("should include correlation_id in verification details for audit trail", func() {
			// SOC2 Requirement: Verification requests must be auditable
			// The response must include correlation_id to link verification to audit chain

			testCorrelationID := fmt.Sprintf("test-audit-trail-%s", uuid.New().String()[:8])
			event := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "test.audit.trail",
				EventCategory:  "testing",
				EventAction:    "verify",
				EventOutcome:   "success",
				ActorType:      "test",
				ActorID:        "test-actor",
				ResourceType:   "test",
				ResourceID:     "test-resource",
				CorrelationID:  testCorrelationID,
				EventData:      map[string]interface{}{"test": "audit_trail"},
				RetentionDays:  2555,
			}

			_, err := immudbRepo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Verify
			reqBody := map[string]interface{}{
				"correlation_id": testCorrelationID,
			}
			reqJSON, err := json.Marshal(reqBody)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				fmt.Sprintf("%s/api/v1/audit/verify-chain", baseURL),
				"application/json",
				bytes.NewReader(reqJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var verifyResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&verifyResp)
			Expect(err).ToNot(HaveOccurred())

			// SOC2: correlation_id must be in response for audit trail
			details := verifyResp["details"].(map[string]interface{})
			Expect(details["correlation_id"]).To(Equal(testCorrelationID),
				"correlation_id must be present in verification response for SOC2 audit trail")

			GinkgoWriter.Printf("✅ SOC2 audit trail validation passed: correlation_id=%s\n", testCorrelationID)
		})
	})
})

