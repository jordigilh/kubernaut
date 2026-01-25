package middleware

// BR-GATEWAY-074, BR-GATEWAY-075, BR-042: Validation Middleware Unit Tests
// Authority: pkg/gateway/middleware/timestamp.go, content_type.go
//
// **SCOPE**: Validation middleware behavior in isolation
// **PATTERN**: Unit test with httptest (no infrastructure)
//
// Tests:
// - Timestamp validation (expired, future, valid)
// - Content-Type validation (invalid, missing, valid)
// - RFC7807 error response format

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("Validation Middleware", func() {
	Context("BR-GATEWAY-074: Timestamp Validation", func() {
		It("should reject requests with expired timestamps", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// 5-minute tolerance window
			wrappedHandler := middleware.TimestampValidator(5 * time.Minute)(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			// Timestamp 10 minutes in the past (outside tolerance)
			// Note: Middleware expects Unix epoch (seconds), not RFC3339
			expiredTimestamp := time.Now().Add(-10 * time.Minute)
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", expiredTimestamp.Unix()))

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-074: Expired timestamps must be rejected with 400")

			body, _ := io.ReadAll(rr.Body)
			Expect(string(body)).To(ContainSubstring("timestamp"),
				"BR-GATEWAY-074: Error response must explain timestamp validation failure")
		})

		It("should reject requests with future timestamps", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.TimestampValidator(5 * time.Minute)(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			// Timestamp 10 minutes in the future (outside tolerance)
			futureTimestamp := time.Now().Add(10 * time.Minute)
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", futureTimestamp.Unix()))

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-075: Future timestamps must be rejected with 400")
		})

		It("should accept requests with valid timestamps", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.TimestampValidator(5 * time.Minute)(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			// Current timestamp (within tolerance)
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"BR-GATEWAY-074: Valid timestamps must be accepted")
		})

		It("should accept requests with timestamps at tolerance boundary", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.TimestampValidator(5 * time.Minute)(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			// Timestamp 4 minutes 59 seconds in the past (just inside boundary)
			// Note: Using 4:59 instead of 5:00 to avoid timing race where validation
			// happens a few milliseconds later and crosses the boundary
			boundaryTimestamp := time.Now().Add(-4*time.Minute - 59*time.Second)
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", boundaryTimestamp.Unix()))

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"BR-GATEWAY-074: Timestamps just inside tolerance boundary must be accepted")
		})

		It("should handle missing X-Timestamp header gracefully", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.TimestampValidator(5 * time.Minute)(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			// No X-Timestamp header

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			// Behavior depends on implementation:
			// - May accept (no timestamp = no validation)
			// - May reject (timestamp required)
			// We just verify it doesn't panic
			Expect(rr.Code).To(Or(Equal(http.StatusOK), Equal(http.StatusBadRequest)),
				"BR-GATEWAY-074: Missing timestamp must be handled gracefully")
		})

		It("should handle malformed X-Timestamp header", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.TimestampValidator(5 * time.Minute)(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			req.Header.Set("X-Timestamp", "not-a-valid-timestamp")

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-074: Malformed timestamps must be rejected")
		})
	})

	Context("BR-042: Content-Type Validation", func() {
		It("should reject requests with invalid Content-Type", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.ValidateContentType(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			req.Header.Set("Content-Type", "text/plain") // Invalid for JSON API

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnsupportedMediaType),
				"BR-042: Invalid Content-Type must be rejected with 415")

			body, _ := io.ReadAll(rr.Body)
			Expect(string(body)).To(ContainSubstring("application/json"),
				"BR-042: Error response must explain expected Content-Type")
		})

		It("should accept requests with application/json Content-Type", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.ValidateContentType(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"BR-042: application/json Content-Type must be accepted")
		})

		It("should accept requests with application/json; charset=utf-8", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.ValidateContentType(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			req.Header.Set("Content-Type", "application/json; charset=utf-8")

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"BR-042: application/json with charset must be accepted")
		})

		It("should return RFC7807 error response for invalid Content-Type", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.ValidateContentType(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/signals/test", nil)
			req.Header.Set("Content-Type", "application/xml")

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnsupportedMediaType))
			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"),
				"BR-042: Error response must use RFC7807 format")

			// Parse RFC7807 response
			var problemDetails map[string]interface{}
			body, _ := io.ReadAll(rr.Body)
			err := json.Unmarshal(body, &problemDetails)
			Expect(err).ToNot(HaveOccurred(),
				"BR-042: Error response must be valid JSON")

			Expect(problemDetails).To(HaveKey("type"),
				"BR-042: RFC7807 response must have 'type' field")
			Expect(problemDetails).To(HaveKey("title"),
				"BR-042: RFC7807 response must have 'title' field")
			Expect(problemDetails).To(HaveKey("detail"),
				"BR-042: RFC7807 response must have 'detail' field")
		})
	})
})
