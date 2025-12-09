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

package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// HTTPDataStorageClient UNIT TESTS (TDD Tier 1)
// 📋 Design Decision: DD-AUDIT-002 | BR-AUDIT-001
// Authority: DD-AUDIT-002 "DataStorageClient.StoreBatch"
// ========================================
//
// These unit tests verify the HTTPDataStorageClient behavior:
// - StoreBatch sends JSON array to correct batch endpoint
// - Proper error handling for HTTP failures
// - Correct Content-Type headers
//
// Defense-in-Depth Testing:
// - Tier 1: Unit tests (this file)
// - Tier 2: Integration tests (test/integration/audit/client_server_test.go)
//
// ========================================

var _ = Describe("HTTPDataStorageClient Unit Tests (DD-AUDIT-002)", func() {
	var (
		testServer   *httptest.Server
		client       audit.DataStorageClient
		ctx          context.Context
		receivedBody []byte
	)

	BeforeEach(func() {
		ctx = context.Background()
		receivedBody = nil
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	// Helper to create test audit events
	createTestEvents := func(count int) []*audit.AuditEvent {
		events := make([]*audit.AuditEvent, count)
		for i := 0; i < count; i++ {
			events[i] = &audit.AuditEvent{
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "signal_received",
				EventOutcome:   "success",
				EventTimestamp: time.Now().UTC(),
				CorrelationID:  "test-correlation-" + string(rune(i+'0')),
				ActorType:      "service",
				ActorID:        "gateway-service",
				ResourceType:   "pod",
				ResourceID:     "test-pod-" + string(rune(i+'0')),
				EventVersion:   "1.0",
				EventData:      []byte(`{"signal_type":"prometheus"}`),
			}
		}
		return events
	}

	Context("StoreBatch - Endpoint Behavior (DD-AUDIT-002)", func() {
		// BEHAVIOR: Client sends batch to /api/v1/audit/events/batch
		// CORRECTNESS: Request body is JSON array
		It("should send batch to correct endpoint", func() {
			var requestPath string

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestPath = r.URL.Path
				body, _ := io.ReadAll(r.Body)
				receivedBody = body

				// Respond with success
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"event_ids": []string{"uuid-1", "uuid-2"},
					"message":   "success",
				})
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})
			events := createTestEvents(2)

			err := client.StoreBatch(ctx, events)

			// CORRECTNESS: Request sent to batch endpoint
			Expect(err).ToNot(HaveOccurred())
			Expect(requestPath).To(Equal("/api/v1/audit/events/batch"),
				"DD-AUDIT-002: StoreBatch must call /batch endpoint")

			// CORRECTNESS: Request body is JSON array
			var receivedEvents []map[string]interface{}
			err = json.Unmarshal(receivedBody, &receivedEvents)
			Expect(err).ToNot(HaveOccurred(), "Request body should be valid JSON array")
			Expect(receivedEvents).To(HaveLen(2), "Should send all events in batch")
		})

		// BEHAVIOR: Client sets Content-Type: application/json
		// CORRECTNESS: Header matches API contract
		It("should set Content-Type header to application/json", func() {
			var contentType string

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contentType = r.Header.Get("Content-Type")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{"event_ids": []string{"uuid-1"}})
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})
			events := createTestEvents(1)

			err := client.StoreBatch(ctx, events)

			Expect(err).ToNot(HaveOccurred())
			Expect(contentType).To(Equal("application/json"))
		})

		// BEHAVIOR: Empty batch returns nil without making HTTP call
		// CORRECTNESS: Optimization to avoid unnecessary network calls
		It("should return nil for empty batch without HTTP call", func() {
			httpCallMade := false

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpCallMade = true
				w.WriteHeader(http.StatusCreated)
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})

			err := client.StoreBatch(ctx, []*audit.AuditEvent{})

			Expect(err).ToNot(HaveOccurred())
			Expect(httpCallMade).To(BeFalse(), "Should not make HTTP call for empty batch")
		})
	})

	Context("StoreBatch - Error Handling (GAP-11)", func() {
		// BEHAVIOR: Client returns error on HTTP 4xx
		// CORRECTNESS: Error indicates validation failure
		It("should return error on HTTP 400 (Bad Request)", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"type":   "validation-error",
					"title":  "Validation Error",
					"status": 400,
					"detail": "event_type is required",
				})
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})
			events := createTestEvents(1)

			err := client.StoreBatch(ctx, events)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("400"))
		})

		// BEHAVIOR: Client returns error on HTTP 5xx
		// CORRECTNESS: Error indicates server failure (retry candidate)
		It("should return error on HTTP 500 (Server Error)", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})
			events := createTestEvents(1)

			err := client.StoreBatch(ctx, events)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})

		// BEHAVIOR: Client returns error on connection failure
		// CORRECTNESS: Error indicates network failure (retry candidate)
		It("should return error on connection failure", func() {
			// Use invalid URL to cause connection failure
			client = audit.NewHTTPDataStorageClient("http://invalid-host:99999", &http.Client{
				Timeout: 100 * time.Millisecond,
			})
			events := createTestEvents(1)

			err := client.StoreBatch(ctx, events)

			Expect(err).To(HaveOccurred())
		})
	})

	Context("StoreBatch - Payload Structure (DD-AUDIT-002)", func() {
		// BEHAVIOR: Each event in batch contains required fields
		// CORRECTNESS: Payload matches API contract
		It("should include all required fields in each event", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				receivedBody = body
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{"event_ids": []string{"uuid-1"}})
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})

			event := &audit.AuditEvent{
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "signal_received",
				EventOutcome:   "success",
				EventTimestamp: time.Now().UTC(),
				CorrelationID:  "corr-123",
				ActorType:      "service",
				ActorID:        "gateway-service",
				ResourceType:   "pod",
				ResourceID:     "test-pod",
				EventVersion:   "1.0",
				EventData:      []byte(`{"signal_type":"prometheus"}`),
			}

			err := client.StoreBatch(ctx, []*audit.AuditEvent{event})
			Expect(err).ToNot(HaveOccurred())

			var receivedEvents []map[string]interface{}
			err = json.Unmarshal(receivedBody, &receivedEvents)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedEvents).To(HaveLen(1))

			receivedEvent := receivedEvents[0]
			// Required fields per DD-AUDIT-002 (with backward-compatible field names)
			Expect(receivedEvent).To(HaveKey("version"))
			Expect(receivedEvent).To(HaveKey("event_type"))
			Expect(receivedEvent).To(HaveKey("event_timestamp"))
			Expect(receivedEvent).To(HaveKey("correlation_id"))
			Expect(receivedEvent).To(HaveKey("event_data"))
			Expect(receivedEvent).To(HaveKey("event_category"))
			// Client uses legacy field names for backward compatibility
			Expect(receivedEvent).To(HaveKey("operation")) // event_action (legacy)
			Expect(receivedEvent).To(HaveKey("outcome"))   // event_outcome (legacy)
		})

		// BEHAVIOR: Large batch (100 events) is sent as single request
		// CORRECTNESS: Efficient batch handling without chunking
		It("should send large batch as single request", func() {
			requestCount := 0

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				body, _ := io.ReadAll(r.Body)
				receivedBody = body
				w.WriteHeader(http.StatusCreated)

				// Return 100 event_ids
				eventIDs := make([]string, 100)
				for i := 0; i < 100; i++ {
					eventIDs[i] = "uuid-" + string(rune(i))
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"event_ids": eventIDs})
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})
			events := createTestEvents(100)

			err := client.StoreBatch(ctx, events)
			Expect(err).ToNot(HaveOccurred())

			// CORRECTNESS: Single request for entire batch (not 100 individual calls)
			Expect(requestCount).To(Equal(1), "Should send 100 events in 1 request, not 100 requests")

			var receivedEvents []map[string]interface{}
			json.Unmarshal(receivedBody, &receivedEvents)
			Expect(receivedEvents).To(HaveLen(100))
		})
	})

	Context("StoreBatch - Context Handling", func() {
		// BEHAVIOR: Client respects context cancellation
		// CORRECTNESS: Request aborts when context is cancelled
		It("should respect context cancellation", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(500 * time.Millisecond) // Simulate slow server
				w.WriteHeader(http.StatusCreated)
			}))

			client = audit.NewHTTPDataStorageClient(testServer.URL, &http.Client{})
			events := createTestEvents(1)

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			err := client.StoreBatch(ctx, events)

			// Context cancelled should cause error
			Expect(err).To(HaveOccurred())
		})
	})
})

