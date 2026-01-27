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
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// AUDIT EVENTS BATCH WRITE API INTEGRATION TESTS (TDD RED Phase)
// 📋 Design Decision: DD-AUDIT-002 | BR-AUDIT-001
// Authority: DD-AUDIT-002 "DataStorageClient.StoreBatch"
// ========================================
//
// This file defines the integration test contract for the batch audit write API.
// DD-AUDIT-002 mandates StoreBatch(ctx, events []*AuditEvent) accepts ARRAYS.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (this file)
// - Implementation code written SECOND (audit_events_batch_handler.go)
// - Contract: POST /api/v1/audit/events/batch with JSON ARRAY body
//
// Business Requirements:
// - BR-AUDIT-001: Complete audit trail with no data loss
// - DD-AUDIT-002: StoreBatch interface must accept arrays
//
// Defense-in-Depth:
// - Integration tests (this file) - Tier 1
// - Unit tests (audit_events_batch_handler_test.go) - Tier 2
//
// ========================================

var _ = Describe("Audit Events Batch Write API Integration Tests", func() {
	var testCorrelationID string

	BeforeEach(func() {
		// CRITICAL: Use public schema for audit_events table queries
		// HTTP API writes to public schema, test queries must target same schema

		// Ensure service is ready before each test
		Eventually(func() int {
			resp, err := HTTPClient.Get(dataStorageURL + "/health")
			if err != nil || resp == nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}, "10s", "500ms").Should(Equal(200), "Data Storage Service should be ready")

		// Generate unique correlation ID for test isolation
		testCorrelationID = generateTestID()

		// Clean up test data before each test
		_, err := testDB.Exec("DELETE FROM audit_events WHERE correlation_id LIKE $1", testCorrelationID+"%")
		if err != nil {
			GinkgoWriter.Printf("Note: audit_events cleanup note: %v\n", err)
		}
	})

	Context("DD-AUDIT-002: Batch Audit Write API", func() {
		// ========================================
		// BEHAVIOR: Handler accepts JSON array of audit events
		// CORRECTNESS: All events in batch are persisted atomically
		// DD-AUDIT-002: StoreBatch interface must accept arrays
		// ========================================
		When("HTTPDataStorageClient sends batch of audit events", func() {
			It("should accept batch of 3 events and return 201 with all event_ids", FlakeAttempts(3), func() {
				// This test simulates what HTTPDataStorageClient.StoreBatch() does:
				// - Marshals []*AuditEvent as JSON array
				// - Sends to POST /api/v1/audit/events/batch
				// - Expects 201 Created with array of event_ids

				By("Building 3 Gateway events using ogen types")
				ctx := context.Background()
				client, err := createOpenAPIClient(dataStorageURL)
				Expect(err).ToNot(HaveOccurred())

				timestamp := time.Now().Add(-5 * time.Second).UTC()
				events := []ogenclient.AuditEventRequest{
					{
						Version:        "1.0",
						EventType:      "gateway.signal.received",
						EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
						EventAction:    "signal_received",
						EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventTimestamp: timestamp,
						CorrelationID:  testCorrelationID + "-1",
						EventData: ogenclient.AuditEventRequestEventData{
							Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
							GatewayAuditPayload: ogenclient.GatewayAuditPayload{
								EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
								SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
								AlertName:   "BatchTest1",
								Namespace:   "default",
								Fingerprint: "test-fingerprint-1",
							},
						},
					},
					{
						Version:        "1.0",
						EventType:      "gateway.crd.created",
						EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
						EventAction:    "crd_created",
						EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventTimestamp: timestamp,
						CorrelationID:  testCorrelationID + "-2",
						EventData: ogenclient.AuditEventRequestEventData{
							Type: ogenclient.AuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData,
							GatewayAuditPayload: ogenclient.GatewayAuditPayload{
								EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
								SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
								AlertName:   "BatchTest2",
								Namespace:   "default",
								Fingerprint: "test-fingerprint-2",
							},
						},
					},
					{
						Version:        "1.0",
						EventType:      "gateway.signal.received",
						EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
						EventAction:    "signal_received",
						EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventTimestamp: timestamp,
						CorrelationID:  testCorrelationID + "-3",
						EventData: ogenclient.AuditEventRequestEventData{
							Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
							GatewayAuditPayload: ogenclient.GatewayAuditPayload{
								EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
								SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
								AlertName:   "BatchTest3",
								Namespace:   "default",
								Fingerprint: "test-fingerprint-3",
							},
						},
					},
				}

				By("Sending batch using ogen client")
				eventIDs, err := postAuditEventBatch(ctx, client, events)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventIDs).To(HaveLen(3), "Should return 3 event_ids")

				By("Verifying all event_ids are valid UUIDs")
				for _, idStr := range eventIDs {
					Expect(idStr).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`))
				}

				// ✅ CORRECTNESS: All events persisted in database
				By("Verifying all 3 events persisted in database (CORRECTNESS)")
				for i, idStr := range eventIDs {
					// Handle async HTTP API processing - data may not be committed immediately
					Eventually(func() int {
						var count int
						err := testDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE event_id = $1", idStr).Scan(&count)
						if err != nil {
							return -1
						}
						return count
					}, 5*time.Second, 100*time.Millisecond).Should(Equal(1), "Event %d should exist in database", i+1)
				}

				// ✅ CORRECTNESS: Verify event content matches sent payload
				By("Verifying event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbCorrelationID string
				err = testDB.QueryRow(`
					SELECT event_type, correlation_id
					FROM audit_events
					WHERE event_id = $1
				`, eventIDs[0]).Scan(&dbEventType, &dbCorrelationID)
				Expect(err).ToNot(HaveOccurred())
				Expect(dbEventType).To(Equal("gateway.signal.received"))
				Expect(dbCorrelationID).To(Equal(testCorrelationID + "-1"))
			})
		})

		// ========================================
		// BEHAVIOR: Atomic batch - all succeed or all fail
		// CORRECTNESS: No partial writes on validation failure
		// ========================================
		When("batch contains one invalid event", func() {
			It("should reject entire batch with 400 Bad Request (atomic)", func() {
				By("Creating batch with one invalid event (missing event_type)")
				events := []map[string]interface{}{
					{
						"version":         "1.0",
						"event_type":      "gateway.signal.received",
						"event_category":  "gateway",
						"event_action":    "signal_received",
						"event_outcome":   "success",
						"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
						"correlation_id":  testCorrelationID + "-atomic-1",
						"event_data":      map[string]interface{}{"valid": true},
					},
					{
						// INVALID: Missing event_type
						"version":         "1.0",
						"event_category":  "gateway",
						"event_action":    "signal_received",
						"event_outcome":   "success",
						"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
						"correlation_id":  testCorrelationID + "-atomic-2",
						"event_data":      map[string]interface{}{"valid": false},
					},
				}

				body, _ := json.Marshal(events)
				req, _ := http.NewRequest("POST", dataStorageURL+"/api/v1/audit/events/batch", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := HTTPClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				By("Verifying 400 Bad Request response")
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				By("Verifying RFC 7807 error format")
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var problem map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&problem)
				Expect(err).ToNot(HaveOccurred())
				Expect(problem).To(HaveKey("type"))
				Expect(problem).To(HaveKey("title"))
				Expect(problem).To(HaveKey("status"))
				Expect(problem).To(HaveKey("detail"))

				// ✅ CORRECTNESS: No events should be persisted (atomic rollback)
				By("Verifying no events were persisted (atomic rollback)")
				var count int
				err = testDB.QueryRow(`
					SELECT COUNT(*) FROM audit_events
					WHERE correlation_id LIKE $1
				`, testCorrelationID+"-atomic%").Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(0), "No events should be persisted on validation failure")
			})
		})

		// ========================================
		// BEHAVIOR: Handler rejects non-array payloads
		// CORRECTNESS: Clear error message per RFC 7807
		// ========================================
		When("payload is not an array", func() {
			It("should return 400 Bad Request with clear error", func() {
				By("Sending single object instead of array")
				event := map[string]interface{}{
					"version":         "1.0",
					"event_type":      "gateway.signal.received",
					"event_category":  "gateway",
					"event_action":    "signal_received",
					"event_outcome":   "success",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  testCorrelationID + "-single",
					"event_data":      map[string]interface{}{},
				}

				body, _ := json.Marshal(event)
				req, _ := http.NewRequest("POST", dataStorageURL+"/api/v1/audit/events/batch", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := HTTPClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				var problem map[string]interface{}
				_ = json.NewDecoder(resp.Body).Decode(&problem)
				detail, _ := problem["detail"].(string)
				Expect(detail).To(ContainSubstring("array"), "Error should mention array requirement")
			})
		})

		// ========================================
		// BEHAVIOR: Handler rejects empty arrays
		// CORRECTNESS: Prevents wasted database transactions
		// ========================================
		When("batch is empty array", func() {
			It("should return 400 Bad Request", func() {
				events := []map[string]interface{}{}

				body, _ := json.Marshal(events)
				req, _ := http.NewRequest("POST", dataStorageURL+"/api/v1/audit/events/batch", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := HTTPClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				var problem map[string]interface{}
				_ = json.NewDecoder(resp.Body).Decode(&problem)
				detail, _ := problem["detail"].(string)
				Expect(detail).To(ContainSubstring("empty"), "Error should mention empty batch")
			})
		})

		// ========================================
		// BEHAVIOR: Handler handles large batches efficiently
		// CORRECTNESS: All 100 events persisted in single transaction
		// ========================================
		When("batch contains 100 events", func() {
			It("should persist all events efficiently", FlakeAttempts(3), func() {
				By("Creating batch of 100 events using ogen types")
				ctx := context.Background()
				client, err := createOpenAPIClient(dataStorageURL)
				Expect(err).ToNot(HaveOccurred())

				timestamp := time.Now().Add(-5 * time.Second).UTC()
				events := make([]ogenclient.AuditEventRequest, 100)
				for i := 0; i < 100; i++ {
					events[i] = ogenclient.AuditEventRequest{
						Version:        "1.0",
						EventType:      "gateway.signal.received",
						EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
						EventAction:    "signal_received",
						EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventTimestamp: timestamp,
						CorrelationID:  testCorrelationID + "-large",
						EventData: ogenclient.AuditEventRequestEventData{
							Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
							GatewayAuditPayload: ogenclient.GatewayAuditPayload{
								EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
								SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
								AlertName:   "LargeBatchTest",
								Namespace:   "default",
								Fingerprint: "test-fingerprint-large",
							},
						},
					}
				}

				By("Sending batch using ogen client")
				start := time.Now()
				eventIDs, err := postAuditEventBatch(ctx, client, events)
				duration := time.Since(start)
				Expect(err).ToNot(HaveOccurred())

				By("Verifying response contains 100 event_ids")
				Expect(eventIDs).To(HaveLen(100))

				// ✅ CORRECTNESS: All 100 events in database
				By("Verifying all 100 events persisted in database")
				// Handle async HTTP API processing - large batch may take longer to commit
				Eventually(func() int {
					var count int
					err := testDB.QueryRow(`
						SELECT COUNT(*) FROM audit_events
						WHERE correlation_id = $1
					`, testCorrelationID+"-large").Scan(&count)
					if err != nil {
						return -1
					}
					return count
				}, 10*time.Second, 100*time.Millisecond).Should(Equal(100),
					"All 100 events should be persisted in database after async processing")

				// Performance check (should be <5s for 100 events)
				Expect(duration).To(BeNumerically("<", 5*time.Second),
					"100 events should be processed in <5 seconds")
			})
		})
	})
})
