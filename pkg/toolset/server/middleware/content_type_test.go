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
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContentTypeMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Content-Type Middleware Suite")
}

var _ = Describe("Content-Type Validation Middleware", func() {
	var (
		mux *http.ServeMux
	)

	BeforeEach(func() {
		mux = http.NewServeMux()
		
		// Test endpoint that accepts POST requests (with Content-Type validation)
		mux.HandleFunc("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{"message": "success"})
			} else if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{"message": "success"})
			}
		})
	})

	Context("Valid Content-Type", func() {
		It("should accept application/json for POST requests", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with valid Content-Type should succeed
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"test": "data"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "test-req-001")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be processed successfully
			Expect(w.Code).To(Equal(http.StatusOK), "Valid Content-Type should result in 200 OK")
			
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(response["message"]).To(Equal("success"), "Response should contain success message")
		})

		It("should accept application/json with charset parameter for POST requests", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with Content-Type including charset should succeed
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"test": "data"}`))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			req.Header.Set("X-Request-ID", "test-req-002")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be processed successfully
			Expect(w.Code).To(Equal(http.StatusOK), "Content-Type with charset should result in 200 OK")
			
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(response["message"]).To(Equal("success"), "Response should contain success message")
		})
	})

	Context("Invalid Content-Type", func() {
		It("should reject text/plain Content-Type with 415 error", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with text/plain should return 415 Unsupported Media Type
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader("plain text data"))
			req.Header.Set("Content-Type", "text/plain")
			req.Header.Set("X-Request-ID", "test-req-003")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be rejected with 415
			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType), "text/plain should result in 415 Unsupported Media Type")
			
			// Validate RFC 7807 error response structure
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")
			
			var errorResponse map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			
			// Validate RFC 7807 required fields
			Expect(errorResponse["type"]).To(ContainSubstring("unsupported-media-type"), "Error type should indicate unsupported media type")
			Expect(errorResponse["title"]).To(Equal("Unsupported Media Type"), "Error title should be 'Unsupported Media Type'")
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("text/plain"), "Error detail should mention the invalid Content-Type")
			Expect(errorResponse["instance"]).To(Equal("/api/v1/test"), "Error instance should be the request path")
			Expect(errorResponse["request_id"]).To(Equal("test-req-003"), "Error should include request ID")
		})

		It("should reject text/html Content-Type with 415 error", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with text/html should return 415 Unsupported Media Type
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader("<html><body>test</body></html>"))
			req.Header.Set("Content-Type", "text/html")
			req.Header.Set("X-Request-ID", "test-req-004")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be rejected with 415
			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType), "text/html should result in 415 Unsupported Media Type")
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")
			
			var errorResponse map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("text/html"), "Error detail should mention the invalid Content-Type")
		})

		It("should reject application/xml Content-Type with 415 error", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with application/xml should return 415 Unsupported Media Type
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader("<xml><test>data</test></xml>"))
			req.Header.Set("Content-Type", "application/xml")
			req.Header.Set("X-Request-ID", "test-req-005")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be rejected with 415
			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType), "application/xml should result in 415 Unsupported Media Type")
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")
			
			var errorResponse map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("application/xml"), "Error detail should mention the invalid Content-Type")
		})

		It("should reject multipart/form-data Content-Type with 415 error", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with multipart/form-data should return 415 Unsupported Media Type
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader("form data"))
			req.Header.Set("Content-Type", "multipart/form-data")
			req.Header.Set("X-Request-ID", "test-req-006")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be rejected with 415
			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType), "multipart/form-data should result in 415 Unsupported Media Type")
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")
			
			var errorResponse map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("multipart/form-data"), "Error detail should mention the invalid Content-Type")
		})

		It("should reject requests with missing Content-Type header with 415 error", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST without Content-Type should return 415 Unsupported Media Type
			
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"test": "data"}`))
			// Intentionally NOT setting Content-Type header
			req.Header.Set("X-Request-ID", "test-req-007")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: Request should be rejected with 415
			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType), "Missing Content-Type should result in 415 Unsupported Media Type")
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")
			
			var errorResponse map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("missing"), "Error detail should indicate missing Content-Type")
		})
	})

	Context("GET Requests", func() {
		It("should not validate Content-Type for GET requests", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: GET requests should not be validated for Content-Type
			
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			// Intentionally NOT setting Content-Type header
			req.Header.Set("X-Request-ID", "test-req-008")
			
			w := httptest.NewRecorder()
			
			// Wrap handler with Content-Type validation middleware
			handler := ValidateContentType(mux)
			handler.ServeHTTP(w, req)
			
			// Validate Behavior: GET request should succeed without Content-Type
			Expect(w.Code).To(Equal(http.StatusOK), "GET request should succeed without Content-Type validation")
			
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(response["message"]).To(Equal("success"), "Response should contain success message")
		})
	})
})

