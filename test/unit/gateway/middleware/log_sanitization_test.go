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
	"bytes"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("Log Sanitization (VULN-GATEWAY-004)", func() {
	var (
		recorder    *httptest.ResponseRecorder
		testHandler http.Handler
		logBuffer   *bytes.Buffer
	)

	BeforeEach(func() {
		recorder = httptest.NewRecorder()
		logBuffer = &bytes.Buffer{}

		// Create test handler that always returns 200 OK
		testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		})
	})

	// TDD RED Phase - Test 1-3: Sensitive field redaction
	Context("Sensitive Field Redaction", func() {
		It("should redact password fields from logs", func() {
			// Arrange: Create sanitizing logger middleware
			sanitizer := middleware.NewSanitizingLogger(logBuffer)
			handler := sanitizer(testHandler)

			// Act: Send request with password in body
			body := `{"username":"admin","password":"secret123","email":"admin@example.com"}`
			req := httptest.NewRequest("POST", "/webhook/prometheus", bytes.NewBufferString(body))
			handler.ServeHTTP(recorder, req)

			// Assert: Password should be redacted in logs
			logOutput := logBuffer.String()
			Expect(logOutput).ToNot(ContainSubstring("secret123"))
			Expect(logOutput).To(ContainSubstring("[REDACTED]"))
		})

		It("should redact token fields from logs", func() {
			// Arrange
			sanitizer := middleware.NewSanitizingLogger(logBuffer)
			handler := sanitizer(testHandler)

			// Act: Send request with token in header
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req.Header.Set("Authorization", "Bearer secret-token-12345")
			handler.ServeHTTP(recorder, req)

			// Assert: Token should be redacted in logs
			logOutput := logBuffer.String()
			Expect(logOutput).ToNot(ContainSubstring("secret-token-12345"))
			Expect(logOutput).To(ContainSubstring("[REDACTED]"))
		})

		It("should redact API keys from logs", func() {
			// Arrange
			sanitizer := middleware.NewSanitizingLogger(logBuffer)
			handler := sanitizer(testHandler)

			// Act: Send request with API key
			body := `{"api_key":"sk-1234567890abcdef","data":"test"}`
			req := httptest.NewRequest("POST", "/webhook/prometheus", bytes.NewBufferString(body))
			handler.ServeHTTP(recorder, req)

			// Assert: API key should be redacted
			logOutput := logBuffer.String()
			Expect(logOutput).ToNot(ContainSubstring("sk-1234567890abcdef"))
			Expect(logOutput).To(ContainSubstring("[REDACTED]"))
		})
	})

	// TDD RED Phase - Test 4-6: Webhook data sanitization
	Context("Webhook Data Sanitization", func() {
		It("should redact annotations from Prometheus webhooks", func() {
			// Arrange
			sanitizer := middleware.NewSanitizingLogger(logBuffer)
			handler := sanitizer(testHandler)

			// Act: Send Prometheus webhook with sensitive annotations
			body := `{"alerts":[{"annotations":{"secret":"sensitive-data","description":"alert"}}]}`
			req := httptest.NewRequest("POST", "/webhook/prometheus", bytes.NewBufferString(body))
			handler.ServeHTTP(recorder, req)

			// Assert: Annotations should be redacted
			logOutput := logBuffer.String()
			Expect(logOutput).ToNot(ContainSubstring("sensitive-data"))
		})

		It("should redact generatorURL from webhooks", func() {
			// Arrange
			sanitizer := middleware.NewSanitizingLogger(logBuffer)
			handler := sanitizer(testHandler)

			// Act: Send webhook with generatorURL
			body := `{"generatorURL":"https://internal.company.com/alerts/123?token=secret"}`
			req := httptest.NewRequest("POST", "/webhook/prometheus", bytes.NewBufferString(body))
			handler.ServeHTTP(recorder, req)

			// Assert: GeneratorURL should be redacted
			logOutput := logBuffer.String()
			Expect(logOutput).ToNot(ContainSubstring("internal.company.com"))
			Expect(logOutput).ToNot(ContainSubstring("token=secret"))
		})

		It("should preserve non-sensitive fields in logs", func() {
			// Arrange
			sanitizer := middleware.NewSanitizingLogger(logBuffer)
			handler := sanitizer(testHandler)

			// Act: Send request with mix of sensitive and non-sensitive data
			body := `{"alertname":"HighMemoryUsage","severity":"critical","password":"secret"}`
			req := httptest.NewRequest("POST", "/webhook/prometheus", bytes.NewBufferString(body))
			handler.ServeHTTP(recorder, req)

			// Assert: Non-sensitive fields preserved, sensitive redacted
			logOutput := logBuffer.String()
			Expect(logOutput).To(ContainSubstring("HighMemoryUsage"))
			Expect(logOutput).To(ContainSubstring("critical"))
			Expect(logOutput).ToNot(ContainSubstring("secret"))
		})
	})
})
