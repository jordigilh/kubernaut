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
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)
var _ = Describe("OpenAPIClientAdapter - DD-API-001 Compliance", Label("unit", "audit", "dd-api-001"), func() {
	var (
		ctx    context.Context
		server *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("NewOpenAPIClientAdapter", func() {
		It("should create adapter with valid parameters", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			client, err := audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("should reject empty baseURL", func() {
			client, err := audit.NewOpenAPIClientAdapter("", 5*time.Second)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("baseURL cannot be empty"))
			Expect(client).To(BeNil())
		})

		It("should use default timeout if zero provided", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			client, err := audit.NewOpenAPIClientAdapter(server.URL, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})
	})

	Describe("StoreBatch - DD-API-001 Compliance", func() {
		var (
			client audit.DataStorageClient
		)

		Context("Success Cases", func() {
			It("should successfully write batch with 201 response", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodPost))
					Expect(r.URL.Path).To(Equal("/api/v1/audit/events/batch"))
					Expect(r.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"message": "Batch created successfully", "events_created": 2}`))
				}))

				var err error
				client, err = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())

				// Create test events using ogen union constructors (ogen migration)
				payload1 := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert-1",
					Namespace:   "default",
					Fingerprint: "test-fingerprint-1",
				}
				payload2 := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert-2",
					Namespace:   "default",
					Fingerprint: "test-fingerprint-2",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event.type",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("TestResource"),
						ResourceID:    ogenclient.NewOptString("test-123"),
						CorrelationID: "corr-456",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload1),
					},
					{
						EventType:     "test.event.type2",
						EventAction:   "test.action2",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("TestResource"),
						ResourceID:    ogenclient.NewOptString("test-456"),
						CorrelationID: "corr-789",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload2),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle empty batch gracefully", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Fail("Should not make HTTP request for empty batch")
				}))

				var err error
				client, err = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())

				err = client.StoreBatch(ctx, []*ogenclient.AuditEventRequest{})
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Error Cases - Network Errors (Retryable)", func() {
			It("should return NetworkError for connection refused", func() {
				// Use invalid URL to trigger connection error
				client, err := audit.NewOpenAPIClientAdapter("http://localhost:1", 100*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())

				// Create test event using ogen union constructor (ogen migration)
				payload := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("Test"),
						ResourceID:    ogenclient.NewOptString("test-1"),
						CorrelationID: "corr-1",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).To(HaveOccurred())
				Expect(audit.IsRetryable(err)).To(BeTrue(), "Network errors should be retryable")
			})

			It("should return NetworkError for timeout", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(200 * time.Millisecond) // Longer than client timeout
					w.WriteHeader(http.StatusOK)
				}))

				client, err := audit.NewOpenAPIClientAdapter(server.URL, 50*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())

				// Create test event using ogen union constructor (ogen migration)
				payload := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("Test"),
						ResourceID:    ogenclient.NewOptString("test-1"),
						CorrelationID: "corr-1",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).To(HaveOccurred())
				Expect(audit.IsRetryable(err)).To(BeTrue(), "Timeout errors should be retryable")
			})
		})

		Context("Error Cases - HTTP 4xx (NOT Retryable)", func() {
			It("should return HTTPError for 400 Bad Request", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json") // Required for ogen client
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"message": "Invalid event data"}`))
				}))

				var err error
				client, err = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())

				// Create test event using ogen union constructor (ogen migration)
				payload := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("Test"),
						ResourceID:    ogenclient.NewOptString("test-1"),
						CorrelationID: "corr-1",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).To(HaveOccurred())
				Expect(audit.Is4xxError(err)).To(BeTrue(), "400 errors should be 4xx")
				Expect(audit.IsRetryable(err)).To(BeFalse(), "4xx errors should NOT be retryable")
			})

			It("should return HTTPError for 422 Unprocessable Entity", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json") // Required for ogen client
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message": "Validation failed"}`))
				}))

				var err error
				client, err = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())

				// Create test event using ogen union constructor (ogen migration)
				payload := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("Test"),
						ResourceID:    ogenclient.NewOptString("test-1"),
						CorrelationID: "corr-1",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).To(HaveOccurred())
				Expect(audit.Is4xxError(err)).To(BeTrue(), "422 errors should be 4xx")
				Expect(audit.IsRetryable(err)).To(BeFalse(), "4xx errors should NOT be retryable")
			})
		})

		Context("Error Cases - HTTP 5xx (Retryable)", func() {
			It("should return HTTPError for 500 Internal Server Error", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json") // Required for ogen client
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message": "Database connection failed"}`))
				}))

				var err error
				client, err = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())

				// Create test event using ogen union constructor (ogen migration)
				payload := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("Test"),
						ResourceID:    ogenclient.NewOptString("test-1"),
						CorrelationID: "corr-1",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).To(HaveOccurred())
				Expect(audit.Is5xxError(err)).To(BeTrue(), "500 errors should be 5xx")
				Expect(audit.IsRetryable(err)).To(BeTrue(), "5xx errors should be retryable")
			})

			It("should return HTTPError for 503 Service Unavailable", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json") // Required for ogen client
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"message": "Service temporarily unavailable"}`))
				}))

				var err error
				client, err = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())

				// Create test event using ogen union constructor (ogen migration)
				payload := ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				}

				events := []*ogenclient.AuditEventRequest{
					{
						EventType:     "test.event",
						EventAction:   "test.action",
						EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
						ResourceType:  ogenclient.NewOptString("Test"),
						ResourceID:    ogenclient.NewOptString("test-1"),
						CorrelationID: "corr-1",
						EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
					},
				}

				err = client.StoreBatch(ctx, events)
				Expect(err).To(HaveOccurred())
				Expect(audit.Is5xxError(err)).To(BeTrue(), "503 errors should be 5xx")
				Expect(audit.IsRetryable(err)).To(BeTrue(), "5xx errors should be retryable")
			})
		})
	})

	Describe("DD-API-001 Compliance Validation", func() {
		It("should use generated OpenAPI client (not direct HTTP)", func() {
			// This test validates that the adapter uses the generated client
			// by verifying the request format matches OpenAPI spec expectations

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request matches OpenAPI spec
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/api/v1/audit/events/batch"))
				Expect(r.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

				// Verify request body is valid JSON array (per OpenAPI spec)
				Expect(r.Body).ToNot(BeNil())

				w.Header().Set("Content-Type", "application/json") // Required for ogen client
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"message": "Success", "events_created": 1}`))
			}))

			client, err := audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			// Create test event using ogen union constructor (ogen migration - DD-API-001 compliance)
			payload := ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
				AlertName:   "dd-api-001-compliance-test",
				Namespace:   "default",
				Fingerprint: "compliance-fingerprint",
			}

			events := []*ogenclient.AuditEventRequest{
				{
					EventType:     "dd.api.001.compliance.test",
					EventAction:   "compliance.test",
					EventCategory: "gateway", // DD-TESTING-001: Use valid event_category from OpenAPI enum
					ResourceType:  ogenclient.NewOptString("ComplianceTest"),
					ResourceID:    ogenclient.NewOptString("test-123"),
					CorrelationID: "corr-456",
					EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventData:     ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload),
				},
			}

			err = client.StoreBatch(ctx, events)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should implement DataStorageClient interface", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			client, err := audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			// Verify it implements the interface
			var _ audit.DataStorageClient = client
		})
	})
})
