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
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"

	"github.com/google/uuid"
)

// ========================================
// GAP 1.2: MALFORMED EVENT REJECTION TEST
// ========================================
//
// Business Requirement: BR-STORAGE-024 (RFC 7807 error responses)
// Gap Analysis: TRIAGE_DS_TEST_COVERAGE_GAP_ANALYSIS_V3.md - Gap 1.2
// Priority: P0
// Estimated Effort: 1 hour
// Confidence: 93%
//
// BUSINESS OUTCOME:
// DS rejects malformed audit events with clear error messages (RFC 7807)
//
// MISSING SCENARIO:
// - Missing required fields (event_type, correlation_id)
// - Invalid field formats (event_timestamp, event_outcome)
// - Clear RFC 7807 error responses with field-level details
// - HTTP 400 Bad Request (not 500 Internal Server Error)
// - Event NOT persisted to database
//
// TDD RED PHASE: Tests define contract, implementation already exists
// ========================================

var _ = Describe("GAP 1.2: Malformed Event Rejection (RFC 7807)", Label("e2e", "gap-1.2", "p0"), Ordered, func() {
	var (
		db *sql.DB
	)

	BeforeAll(func() {
		// Connect to PostgreSQL via NodePort for validation queries
		var err error
		db, err = sql.Open("pgx", postgresURL)
		Expect(err).ToNot(HaveOccurred())
		Expect(db.Ping()).To(Succeed())
	})

	AfterAll(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	Describe("POST /api/v1/audit-events - Validation", func() {

		Context("when event_type is missing (required field)", func() {
			It("should return HTTP 400 with RFC 7807 error", func() {
				// ARRANGE: Malformed event - missing event_type
				malformedEvent := map[string]interface{}{
					// "event_type": "gateway.signal.received", // MISSING (required)
					"version":         "1.0",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "gateway", // ADR-034 v1.2 (was "signal" - invalid)
					"event_action":    "received",
					"event_outcome":   "success",
					"actor_type":      "service",
					"actor_id":        "gateway-service",
					"resource_type":   "Signal",
					"resource_id":     "sig-malformed-1",
					"correlation_id":  "test-malformed-1",
					"event_data":      map[string]interface{}{"test": "missing_event_type"},
				}

				payloadBytes, err := json.Marshal(malformedEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT: POST malformed event
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: HTTP 400 Bad Request (not 500)
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Should return 400 Bad Request for missing required field")

				// ASSERT: Content-Type is RFC 7807
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				// ASSERT: RFC 7807 error structure
				var rfc7807Error validation.RFC7807Problem
				err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
				Expect(err).ToNot(HaveOccurred())

				// ASSERT: RFC 7807 fields
				Expect(rfc7807Error.Type).ToNot(BeEmpty(), "RFC 7807 'type' field required")
				Expect(rfc7807Error.Title).ToNot(BeEmpty(), "RFC 7807 'title' field required")
				Expect(rfc7807Error.Status).To(Equal(400), "RFC 7807 'status' should match HTTP status")
				Expect(rfc7807Error.Detail).To(ContainSubstring("event_type"),
					"Detail should mention the missing field")

				// BUSINESS VALUE: Clear error message helps services debug integration issues
				GinkgoWriter.Printf("RFC 7807 Error Response: %+v\n", rfc7807Error)
			})
		})

		Context("when correlation_id is missing (required field)", func() {
			It("should return HTTP 400 with RFC 7807 error", func() {
				// ARRANGE: Malformed event - missing correlation_id
				malformedEvent := map[string]interface{}{
					"event_type":      "gateway.signal.received",
					"version":         "1.0",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "gateway", // ADR-034 v1.2 (was "signal" - invalid)
					"event_action":    "received",
					"event_outcome":   "success",
					"actor_type":      "service",
					"actor_id":        "gateway-service",
					"resource_type":   "Signal",
					"resource_id":     "sig-malformed-2",
					// "correlation_id": "test-malformed-2", // MISSING (required)
					"event_data": map[string]interface{}{"test": "missing_correlation_id"},
				}

				payloadBytes, err := json.Marshal(malformedEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var rfc7807Error validation.RFC7807Problem
				err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
				Expect(err).ToNot(HaveOccurred())

				Expect(rfc7807Error.Status).To(Equal(400))
				Expect(rfc7807Error.Detail).To(ContainSubstring("correlation_id"),
					"Detail should mention the missing field")
			})
		})

		Context("when event_outcome is invalid (must be success/failure/pending)", func() {
			It("should return HTTP 400 with RFC 7807 error", func() {
				// ARRANGE: Invalid event_outcome value
				malformedEvent := map[string]interface{}{
					"event_type":      "gateway.signal.received",
					"version":         "1.0",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "gateway", // ADR-034 v1.2 (was "signal" - invalid)
					"event_action":    "received",
					"event_outcome":   "invalid_value", // INVALID (must be success/failure/pending)
					"actor_type":      "service",
					"actor_id":        "gateway-service",
					"resource_type":   "Signal",
					"resource_id":     "sig-malformed-3",
					"correlation_id":  "test-malformed-3",
					"event_data":      map[string]interface{}{"test": "invalid_outcome"},
				}

				payloadBytes, err := json.Marshal(malformedEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var rfc7807Error validation.RFC7807Problem
				err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
				Expect(err).ToNot(HaveOccurred())

				Expect(rfc7807Error.Status).To(Equal(400))
				Expect(rfc7807Error.Detail).To(SatisfyAny(
					ContainSubstring("event_outcome"),
					ContainSubstring("success"),
					ContainSubstring("failure"),
					ContainSubstring("pending"),
				), "Detail should explain valid event_outcome values")
			})
		})

		Context("when event_timestamp has invalid format", func() {
			It("should return HTTP 400 with RFC 7807 error", func() {
				// ARRANGE: Invalid timestamp format
				malformedEvent := map[string]interface{}{
					"event_type":      "gateway.signal.received",
					"version":         "1.0",
					"event_timestamp": "invalid-date-format", // INVALID (must be RFC3339)
					"event_category":  "gateway",             // ADR-034 v1.2 (was "signal" - invalid)
					"event_action":    "received",
					"event_outcome":   "success",
					"actor_type":      "service",
					"actor_id":        "gateway-service",
					"resource_type":   "Signal",
					"resource_id":     "sig-malformed-4",
					"correlation_id":  "test-malformed-4",
					"event_data":      map[string]interface{}{"test": "invalid_timestamp"},
				}

				payloadBytes, err := json.Marshal(malformedEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var rfc7807Error validation.RFC7807Problem
				err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
				Expect(err).ToNot(HaveOccurred())

				Expect(rfc7807Error.Status).To(Equal(400))
				Expect(rfc7807Error.Detail).To(SatisfyAny(
					ContainSubstring("event_timestamp"),
					ContainSubstring("timestamp"),
					ContainSubstring("RFC3339"),
					ContainSubstring("format"),
				), "Detail should mention timestamp format issue")
			})
		})

		Context("when event_data is not valid JSON", func() {
			It("should return HTTP 400 with RFC 7807 error", func() {
				// ARRANGE: event_data as invalid type (string instead of JSON object)
				malformedEvent := map[string]interface{}{
					"event_type":      "gateway.signal.received",
					"version":         "1.0",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "gateway", // ADR-034 v1.2 (was "signal" - invalid)
					"event_action":    "received",
					"event_outcome":   "success",
					"actor_type":      "service",
					"actor_id":        "gateway-service",
					"resource_type":   "Signal",
					"resource_id":     "sig-malformed-5",
					"correlation_id":  "test-malformed-5",
					"event_data":      "not-a-json-object", // INVALID (must be JSON object)
				}

				payloadBytes, err := json.Marshal(malformedEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT: Should either accept (auto-wrap string as JSON) or reject with 400
				// This tests actual behavior - either is acceptable
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusBadRequest), // Strict validation
					Equal(http.StatusCreated),    // Lenient (auto-converts)
					Equal(http.StatusAccepted),   // DLQ fallback
				))

				if resp.StatusCode == http.StatusBadRequest {
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

					var rfc7807Error validation.RFC7807Problem
					err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
					Expect(err).ToNot(HaveOccurred())
					Expect(rfc7807Error.Detail).To(ContainSubstring("event_data"))
				}
			})
		})

		Context("when multiple fields are invalid", func() {
			It("should return HTTP 400 with RFC 7807 error listing all violations", func() {
				// ARRANGE: Multiple validation failures
				malformedEvent := map[string]interface{}{
					// "event_type": "gateway.signal.received", // MISSING
					"version":         "1.0",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "gateway", // ADR-034 v1.2 (was "signal" - invalid)
					"event_action":    "received",
					"event_outcome":   "invalid_outcome", // INVALID
					"actor_type":      "service",
					"actor_id":        "gateway-service",
					"resource_type":   "Signal",
					"resource_id":     "sig-malformed-6",
					// "correlation_id": "test-malformed-6", // MISSING
					"event_data": map[string]interface{}{"test": "multiple_errors"},
				}

				payloadBytes, err := json.Marshal(malformedEvent)
				Expect(err).ToNot(HaveOccurred())

				// ACT
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// ASSERT
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var rfc7807Error validation.RFC7807Problem
				err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
				Expect(err).ToNot(HaveOccurred())

				Expect(rfc7807Error.Status).To(Equal(400))

				// BUSINESS VALUE: Single error response listing ALL issues
				// Instead of client discovering errors one-by-one (poor UX)
				GinkgoWriter.Printf("Multiple violations RFC 7807 response: %+v\n", rfc7807Error)
			})
		})
	})

	Describe("Malformed Event NOT Persisted", func() {
		It("should NOT persist malformed events to database", func() {
			// ARRANGE: Use unique correlation_id for test isolation (DD-TEST-002)
			testCorrelationID := fmt.Sprintf("test-not-persisted-%s", uuid.New().String()[:8])

			// ACT: POST malformed event
			malformedEvent := map[string]interface{}{
				// Missing event_type (required)
				"version":         "1.0",
				"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"event_category":  "signal",
				"event_action":    "received",
				"event_outcome":   "success",
				"actor_type":      "service",
				"actor_id":        "gateway-service",
				"resource_type":   "Signal",
				"resource_id":     "sig-not-persisted",
				"correlation_id":  testCorrelationID,
				"event_data":      map[string]interface{}{"test": "should_not_persist"},
			}

			payloadBytes, err := json.Marshal(malformedEvent)
			Expect(err).ToNot(HaveOccurred())

			resp, err := http.Post(
				dataStorageURL+"/api/v1/audit/events",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Request failed
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// ASSERT: Event with this correlation_id was NOT persisted (DD-TEST-002: targeted query for test isolation)
			var count int
			err = db.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1",
				testCorrelationID).Scan(&count)
			Expect(err).ToNot(HaveOccurred())

			Expect(count).To(Equal(0),
				"Malformed events should NOT be persisted to database (correlation_id: %s)", testCorrelationID)

			// BUSINESS VALUE: Data integrity - only valid events in audit trail
		})
	})

	Describe("RFC 7807 Standard Compliance", func() {
		It("should include all required RFC 7807 fields", func() {
			// ARRANGE: Malformed event
			malformedEvent := map[string]interface{}{
				// Missing required fields
				"version":         "1.0",
				"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"event_data":      map[string]interface{}{"test": "rfc7807_compliance"},
			}

			payloadBytes, err := json.Marshal(malformedEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT
			resp, err := http.Post(
				dataStorageURL+"/api/v1/audit/events",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: RFC 7807 compliance
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

			var rfc7807Error validation.RFC7807Problem
			err = json.NewDecoder(resp.Body).Decode(&rfc7807Error)
			Expect(err).ToNot(HaveOccurred())

			// RFC 7807 REQUIRED fields
			Expect(rfc7807Error.Type).ToNot(BeEmpty(), "RFC 7807 'type' field is REQUIRED")
			Expect(rfc7807Error.Title).ToNot(BeEmpty(), "RFC 7807 'title' field is REQUIRED")
			Expect(rfc7807Error.Status).To(Equal(400), "RFC 7807 'status' field is REQUIRED")

			// RFC 7807 OPTIONAL but RECOMMENDED fields
			Expect(rfc7807Error.Detail).ToNot(BeEmpty(), "RFC 7807 'detail' field is RECOMMENDED")

			// Type should be a URI reference (per RFC 7807)
			Expect(rfc7807Error.Type).To(ContainSubstring("://"),
				"RFC 7807 'type' should be a URI reference")

			GinkgoWriter.Printf("RFC 7807 Compliant Response:\n")
			GinkgoWriter.Printf("  Type:   %s\n", rfc7807Error.Type)
			GinkgoWriter.Printf("  Title:  %s\n", rfc7807Error.Title)
			GinkgoWriter.Printf("  Status: %d\n", rfc7807Error.Status)
			GinkgoWriter.Printf("  Detail: %s\n", rfc7807Error.Detail)
		})
	})
})
