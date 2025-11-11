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

package toolset

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/toolset/server"
)

// Content-Type Validation Integration Tests
// Business Requirement: BR-TOOLSET-043 - Content-Type Validation Middleware
//
// These tests validate that the Dynamic Toolset HTTP server correctly validates
// Content-Type headers for POST endpoints and returns RFC 7807 compliant errors
// for invalid Content-Type values.

var _ = Describe("BR-TOOLSET-043: Content-Type Validation Integration", func() {
	var (
		testServer   *httptest.Server
		serverConfig *server.Config
		cancel       context.CancelFunc
	)

	BeforeEach(func() {
		_, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Create test server configuration
		serverConfig = &server.Config{
			Port:              8080,
			MetricsPort:       9090,
			ShutdownTimeout:   30 * time.Second,
			DiscoveryInterval: 5 * time.Minute,
		}

		// Create fake Kubernetes client for testing
		fakeClientset := fake.NewSimpleClientset()

		// Create server instance
		srv, err := server.NewServer(serverConfig, fakeClientset)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test server")

		// Create test HTTP server
		// Note: Auth/authz handled by sidecars and network policies (per ADR-036)
		testServer = httptest.NewServer(srv.Handler())
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
		if cancel != nil {
			cancel()
		}
	})
	Context("POST /api/v1/toolsets/validate endpoint", func() {
		It("should accept POST request with valid application/json Content-Type", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with valid Content-Type should be processed

			requestBody := map[string]interface{}{
				"toolset": map[string]interface{}{
					"name":     "test-toolset",
					"priority": 80,
					"tools": []map[string]interface{}{
						{
							"name":        "kubectl",
							"description": "Kubernetes CLI",
						},
					},
				},
			}

			body, err := json.Marshal(requestBody)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/toolsets/validate", bytes.NewReader(body))
			Expect(err).ToNot(HaveOccurred())

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "test-content-type-001")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Validate Behavior: Request should be processed (200 OK or 400 Bad Request for validation errors)
			// The key is that it's NOT 415 Unsupported Media Type
			Expect(resp.StatusCode).ToNot(Equal(http.StatusUnsupportedMediaType), "Valid Content-Type should not result in 415")
			Expect(resp.StatusCode).To(BeNumerically(">=", 200), "Valid Content-Type should be processed")
			Expect(resp.StatusCode).To(BeNumerically("<", 500), "Valid Content-Type should not cause server error")
		})

		It("should reject POST request with invalid text/plain Content-Type", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST with invalid Content-Type should return 415

			requestBody := "plain text data"

			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/toolsets/validate", bytes.NewReader([]byte(requestBody)))
			Expect(err).ToNot(HaveOccurred())

			req.Header.Set("Content-Type", "text/plain")
			req.Header.Set("X-Request-ID", "test-content-type-002")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Validate Behavior: Request should be rejected with 415 Unsupported Media Type
			Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType), "text/plain should result in 415 Unsupported Media Type")

			// Validate RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

			// Validate RFC 7807 required fields
			Expect(errorResponse["type"]).To(ContainSubstring("unsupported-media-type"), "Error type should indicate unsupported media type")
			Expect(errorResponse["title"]).To(Equal("Unsupported Media Type"), "Error title should be 'Unsupported Media Type'")
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("text/plain"), "Error detail should mention the invalid Content-Type")
			Expect(errorResponse["instance"]).To(Equal("/api/v1/toolsets/validate"), "Error instance should be the request path")
			Expect(errorResponse["request_id"]).To(Equal("test-content-type-002"), "Error should include request ID")
		})

		It("should reject POST request with missing Content-Type header", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: POST without Content-Type should return 415

			requestBody := map[string]interface{}{
				"toolset": map[string]interface{}{
					"name": "test-toolset",
				},
			}

			body, err := json.Marshal(requestBody)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/toolsets/validate", bytes.NewReader(body))
			Expect(err).ToNot(HaveOccurred())

			// Intentionally NOT setting Content-Type header
			req.Header.Set("X-Request-ID", "test-content-type-003")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Validate Behavior: Request should be rejected with 415 Unsupported Media Type
			Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType), "Missing Content-Type should result in 415 Unsupported Media Type")

			// Validate RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"), "Error response should use RFC 7807 format")

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

			// Validate RFC 7807 required fields
			Expect(errorResponse["type"]).To(ContainSubstring("unsupported-media-type"), "Error type should indicate unsupported media type")
			Expect(errorResponse["title"]).To(Equal("Unsupported Media Type"), "Error title should be 'Unsupported Media Type'")
			Expect(errorResponse["status"]).To(BeNumerically("==", 415), "Error status should be 415")
			Expect(errorResponse["detail"]).To(ContainSubstring("missing"), "Error detail should indicate missing Content-Type")
			Expect(errorResponse["instance"]).To(Equal("/api/v1/toolsets/validate"), "Error instance should be the request path")
			Expect(errorResponse["request_id"]).To(Equal("test-content-type-003"), "Error should include request ID")
		})
	})

	Context("GET /api/v1/health endpoint", func() {
		It("should not validate Content-Type for GET requests", func() {
			// BR-TOOLSET-043: Content-Type Validation Middleware
			// Test Behavior: GET requests should not be validated for Content-Type

			req, err := http.NewRequest(http.MethodGet, testServer.URL+"/health", nil)
			Expect(err).ToNot(HaveOccurred())

			// Intentionally NOT setting Content-Type header
			req.Header.Set("X-Request-ID", "test-content-type-004")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Validate Behavior: GET request should succeed without Content-Type validation
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "GET request should succeed without Content-Type validation")

			// Validate response is JSON (health endpoint returns JSON)
			var healthResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&healthResponse)
			Expect(err).ToNot(HaveOccurred(), "Health response should be valid JSON")
			Expect(healthResponse["status"]).To(Equal("ok"), "Health endpoint should return ok status")
		})
	})
})

