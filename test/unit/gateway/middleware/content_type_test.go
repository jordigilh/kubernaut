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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Content-Type Validation
// ============================================================================
//
// BR-042: Content-Type validation
//
// BUSINESS VALUE:
// - Invalid webhook payloads rejected early (save processing resources)
// - Clear error messages for integrators
// - RFC 7807 compliant error responses for tooling compatibility
// ============================================================================

var _ = Describe("BR-042: Content-Type Validation", func() {
	var (
		nextHandler  http.Handler
		validRequest *http.Request
	)

	BeforeEach(func() {
		// Next handler that should only be called for valid requests
		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("success"))
		})

		validRequest = httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)
	})

	Context("when Content-Type is valid JSON", func() {
		It("allows request to proceed for application/json", func() {
			// BUSINESS OUTCOME: Valid JSON webhooks are processed
			validRequest.Header.Set("Content-Type", "application/json")

			handler := middleware.ValidateContentType(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validRequest)

			Expect(recorder.Code).To(Equal(http.StatusOK),
				"Valid JSON request should reach next handler")
			Expect(recorder.Body.String()).To(Equal("success"),
				"Response should be from next handler")
		})

		It("allows JSON with charset parameter", func() {
			// BUSINESS OUTCOME: UTF-8 charset specification is valid
			validRequest.Header.Set("Content-Type", "application/json; charset=utf-8")

			handler := middleware.ValidateContentType(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validRequest)

			Expect(recorder.Code).To(Equal(http.StatusOK),
				"JSON with charset should be valid")
		})
	})

	Context("when Content-Type is missing", func() {
		It("allows request during grace period", func() {
			// BUSINESS OUTCOME: Backward compatibility during migration
			// No Content-Type header set

			handler := middleware.ValidateContentType(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validRequest)

			Expect(recorder.Code).To(Equal(http.StatusOK),
				"Missing Content-Type allowed during grace period")
		})
	})

	Context("when Content-Type is invalid", func() {
		It("rejects text/plain with RFC 7807 error", func() {
			// BUSINESS OUTCOME: Non-JSON payloads rejected with clear error
			validRequest.Header.Set("Content-Type", "text/plain")

			handler := middleware.ValidateContentType(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validRequest)

			Expect(recorder.Code).To(Equal(http.StatusUnsupportedMediaType),
				"Non-JSON Content-Type should return 415")
			Expect(recorder.Header().Get("Content-Type")).To(Equal("application/problem+json"),
				"Error response should be RFC 7807 compliant")
			Expect(recorder.Header().Get("Accept")).To(Equal("application/json"),
				"Accept header tells integrator what we expect")

			// Verify RFC 7807 error structure
			var errorResponse gwerrors.RFC7807Error
			err := json.Unmarshal(recorder.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResponse.Status).To(Equal(http.StatusUnsupportedMediaType))
			Expect(errorResponse.Type).To(Equal(gwerrors.ErrorTypeUnsupportedMediaType))
		})

		It("rejects malformed Content-Type header", func() {
			// BUSINESS OUTCOME: Malformed headers rejected early
			validRequest.Header.Set("Content-Type", "invalid/type/extra/slashes")

			handler := middleware.ValidateContentType(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validRequest)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest),
				"Malformed Content-Type should return 400")
			Expect(recorder.Header().Get("Content-Type")).To(Equal("application/problem+json"),
				"Error response should be RFC 7807 compliant")
		})

		It("rejects XML with proper error", func() {
			// BUSINESS OUTCOME: XML not supported (JSON only)
			validRequest.Header.Set("Content-Type", "application/xml")

			handler := middleware.ValidateContentType(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validRequest)

			Expect(recorder.Code).To(Equal(http.StatusUnsupportedMediaType),
				"XML Content-Type should return 415")
		})
	})
})
