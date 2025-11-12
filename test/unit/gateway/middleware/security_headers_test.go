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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("Security Headers", func() {
	var (
		recorder    *httptest.ResponseRecorder
		testHandler http.Handler
	)

	BeforeEach(func() {
		recorder = httptest.NewRecorder()

		// Create test handler that always returns 200 OK
		testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		})
	})

	// TDD RED Phase - Test 1-6: Security headers should be set
	Context("Security Headers", func() {
		It("should set X-Content-Type-Options header to nosniff", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(recorder.Header().Get("X-Content-Type-Options")).To(Equal("nosniff"))
		})

		It("should set X-Frame-Options header to DENY", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Header().Get("X-Frame-Options")).To(Equal("DENY"))
		})

		It("should set X-XSS-Protection header to 1; mode=block", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Header().Get("X-XSS-Protection")).To(Equal("1; mode=block"))
		})

		It("should set Strict-Transport-Security header for HTTPS", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Header().Get("Strict-Transport-Security")).To(Equal("max-age=31536000; includeSubDomains"))
		})

		It("should set Content-Security-Policy header", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Header().Get("Content-Security-Policy")).To(Equal("default-src 'none'"))
		})

		It("should set Referrer-Policy header to no-referrer", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Header().Get("Referrer-Policy")).To(Equal("no-referrer"))
		})

		It("should set all security headers in a single request", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert: All headers should be present
			headers := recorder.Header()
			Expect(headers.Get("X-Content-Type-Options")).To(Equal("nosniff"))
			Expect(headers.Get("X-Frame-Options")).To(Equal("DENY"))
			Expect(headers.Get("X-XSS-Protection")).To(Equal("1; mode=block"))
			Expect(headers.Get("Strict-Transport-Security")).To(Equal("max-age=31536000; includeSubDomains"))
			Expect(headers.Get("Content-Security-Policy")).To(Equal("default-src 'none'"))
			Expect(headers.Get("Referrer-Policy")).To(Equal("no-referrer"))
		})

		It("should not interfere with response body", func() {
			// Arrange
			securityHeaders := middleware.SecurityHeaders()
			handler := securityHeaders(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert: Response body should be unchanged
			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(recorder.Body.String()).To(Equal("OK"))
		})
	})
})
