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

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
)

func TestOpenAPIMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OpenAPI Middleware Suite")
}

var _ = Describe("OpenAPI Validator Middleware", func() {
	var (
		validator *middleware.OpenAPIValidator
		logger    logr.Logger
	)

	BeforeEach(func() {
		logger = logr.Discard()
		var err error
		// DD-API-002: OpenAPI spec embedded in binary (no path parameter needed)
		validator, err = middleware.NewOpenAPIValidator(
			logger,
			nil, // No metrics in unit tests
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(validator).ToNot(BeNil())
	})

	Describe("Validator Initialization", func() {
		It("should load embedded OpenAPI spec successfully", func() {
			// DD-API-002: Spec is embedded at compile time
			Expect(validator).ToNot(BeNil())
		})

		// NOTE: Cannot test "invalid spec path" with embedded spec
		// Build will fail at compile time if spec is missing (desired behavior)
	})

	Describe("Valid Requests", func() {
		It("should pass validation for valid audit event", func() {
			// Valid audit event with all required fields
			// Use GatewayAuditPayload structure for gateway.signal.received event type
			// NOTE: event_data.event_type is the discriminator field for the oneOf union
			// signal_type must be "alert" per schema (normalized from prometheus-alert/kubernetes-event)
			body := `{
			"version": "1.0",
			"event_type": "gateway.signal.received",
			"event_category": "gateway",
			"event_action": "received",
			"event_outcome": "success",
			"correlation_id": "test-correlation-123",
			"event_timestamp": "2025-12-13T12:00:00Z",
			"event_data": {
				"event_type": "gateway.signal.received",
				"signal_type": "alert",
				"alert_name": "HighMemoryUsage",
				"namespace": "default",
				"fingerprint": "fp-abc123"
			}
		}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			// Mock handler that should be called after validation
			handlerCalled := false
			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				GinkgoWriter.Printf("❌ Validation failed. Response: %d\nBody: %s\n", rr.Code, rr.Body.String())
			}
			Expect(rr.Code).To(Equal(http.StatusCreated))
			Expect(handlerCalled).To(BeTrue(), "Handler should be called after successful validation")
		})

		It("should pass validation with optional fields", func() {
			// Audit event with optional fields included
			// Use GatewayAuditPayload structure with all optional fields
			// NOTE: signal_type must be "alert" per schema (normalized from prometheus-alert/kubernetes-event)
			body := `{
			"version": "1.0",
			"event_type": "gateway.signal.received",
			"event_category": "gateway",
			"event_action": "received",
			"event_outcome": "success",
			"correlation_id": "test-correlation-123",
			"event_timestamp": "2025-12-13T12:00:00Z",
			"event_data": {
				"event_type": "gateway.signal.received",
				"signal_type": "alert",
				"alert_name": "HighMemoryUsage",
				"namespace": "default",
				"fingerprint": "fp-abc123",
				"severity": "critical"
			},
			"actor_type": "service",
			"actor_id": "gateway-service",
			"severity": "info"
		}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				GinkgoWriter.Printf("❌ Validation failed (optional fields test). Response: %d\nBody: %s\n", rr.Code, rr.Body.String())
			}
			Expect(rr.Code).To(Equal(http.StatusCreated))
		})
	})

	Describe("Invalid Requests - Missing Required Fields", func() {
		It("should reject request with missing event_type", func() {
			// Missing required field: event_type
			body := `{
				"version": "1.0",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "success",
				"correlation_id": "test-correlation-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			// Handler should NOT be called
			handlerCalled := false
			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(handlerCalled).To(BeFalse(), "Handler should NOT be called after validation failure")
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))
			Expect(rr.Body.String()).To(ContainSubstring("event_type"))
		})

		It("should reject request with missing version", func() {
			body := `{
				"event_type": "gateway.signal.received",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "success",
				"correlation_id": "test-correlation-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("version"))
		})

		It("should reject request with missing event_category", func() {
			body := `{
				"version": "1.0",
				"event_type": "gateway.signal.received",
				"event_action": "received",
				"event_outcome": "success",
				"correlation_id": "test-correlation-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("event_category"))
		})
	})

	Describe("Invalid Requests - Enum Validation", func() {
		It("should reject request with invalid event_outcome", func() {
			body := `{
				"version": "1.0",
				"event_type": "gateway.signal.received",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "invalid_outcome_value",
				"correlation_id": "test-correlation-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("event_outcome"))
		})
	})

	Describe("Invalid Requests - Malformed JSON", func() {
		It("should reject request with invalid JSON", func() {
			body := `{invalid json`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("Routes Not in OpenAPI Spec", func() {
		It("should pass through /health endpoint without validation", func() {
			req := httptest.NewRequest("GET", "/health", nil)
			rr := httptest.NewRecorder()

			handlerCalled := false
			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(handlerCalled).To(BeTrue(), "Health endpoint should pass through without validation")
		})

		It("should pass through /metrics endpoint without validation", func() {
			req := httptest.NewRequest("GET", "/metrics", nil)
			rr := httptest.NewRecorder()

			handlerCalled := false
			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(handlerCalled).To(BeTrue(), "Metrics endpoint should pass through without validation")
		})
	})

	Describe("RFC 7807 Error Response", func() {
		It("should return RFC 7807 problem details on validation error", func() {
			// Missing required field
			body := `{
				"version": "1.0",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "success",
				"correlation_id": "test-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "test-request-id")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))
			Expect(rr.Header().Get("X-Request-ID")).To(Equal("test-request-id"))

			// Parse response as RFC 7807 problem
			Expect(rr.Body.String()).To(ContainSubstring("type"))
			Expect(rr.Body.String()).To(ContainSubstring("title"))
			Expect(rr.Body.String()).To(ContainSubstring("status"))
			Expect(rr.Body.String()).To(ContainSubstring("detail"))
			Expect(rr.Body.String()).To(ContainSubstring("validation-error")) // RFC 7807 type contains "validation-error" (hyphenated)
		})
	})
})
