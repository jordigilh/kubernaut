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

package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-chi/chi/v5"
	kubecors "github.com/jordigilh/kubernaut/pkg/http/cors"
)

// BR-HTTP-015: CORS Integration Tests
//
// Integration tests validate CORS behavior in a Gateway-like HTTP server context.
// These tests verify that CORS middleware integrates correctly with chi router
// and handles real HTTP request/response cycles.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): CORS library behavior in isolation
// - Integration tests (>50%): CORS middleware integration with HTTP server
// - E2E tests (10-15%): CORS wiring in production Gateway deployment

var _ = Describe("BR-HTTP-015: Gateway CORS Integration", Label("integration", "gateway", "cors"), func() {

	var (
		router     chi.Router
		testServer *httptest.Server
	)

	AfterEach(func() {
		// Cleanup
		if testServer != nil {
			testServer.Close()
		}
		_ = os.Unsetenv("CORS_ALLOWED_ORIGINS")
		_ = os.Unsetenv("CORS_ALLOWED_METHODS")
		_ = os.Unsetenv("CORS_ALLOW_CREDENTIALS")
	})

	// ==============================================
	// CATEGORY 1: CORS on Gateway Endpoints (BEHAVIOR)
	// BR-HTTP-015: All Gateway endpoints must support CORS
	// ==============================================

	Context("CORS on Gateway Endpoints - BEHAVIOR", func() {

		BeforeEach(func() {
			// Setup chi router with CORS middleware (mimics Gateway setup)
			router = chi.NewRouter()

			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
			corsOpts := kubecors.FromEnvironment()
			router.Use(kubecors.Handler(corsOpts))

			// Add test endpoints mimicking Gateway
			router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"healthy"}`))
			})
			router.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ready"}`))
			})
			router.Post("/api/v1/signals/prometheus", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"status":"created"}`))
			})

			testServer = httptest.NewServer(router)
		})

		DescribeTable("should include CORS headers on all Gateway endpoints",
			func(method, endpoint string, expectedStatus int) {
				// BEHAVIOR: All Gateway endpoints respond with CORS headers
				client := &http.Client{}

				req, err := http.NewRequest(method, testServer.URL+endpoint, nil)
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Origin", "https://dashboard.kubernaut.io")

				resp, err := client.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// BEHAVIOR VALIDATION: CORS headers are present
				allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
				Expect(allowOrigin).ToNot(BeEmpty(),
					"All endpoints should include CORS headers for cross-origin access")
			},
			Entry("health endpoint supports CORS", "GET", "/health", http.StatusOK),
			Entry("readiness endpoint supports CORS", "GET", "/ready", http.StatusOK),
		)

		It("should handle preflight for webhook endpoint", func() {
			// BEHAVIOR: Browser can check if POST to webhook is allowed
			client := &http.Client{}

			req, err := http.NewRequest("OPTIONS", testServer.URL+"/api/v1/signals/prometheus", nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Origin", "https://dashboard.kubernaut.io")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Content-Type")

			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// BEHAVIOR VALIDATION: Preflight succeeds with CORS headers
			Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusNoContent)),
				"Preflight should succeed")
			Expect(resp.Header.Get("Access-Control-Allow-Origin")).ToNot(BeEmpty(),
				"Preflight should include Allow-Origin header")
			Expect(resp.Header.Get("Access-Control-Allow-Methods")).ToNot(BeEmpty(),
				"Preflight should include Allow-Methods header")
		})
	})

	// ==============================================
	// CATEGORY 2: CORS on Error Responses (BEHAVIOR)
	// BR-HTTP-015: Error responses must include CORS headers
	// ==============================================

	Context("CORS on Error Responses - BEHAVIOR", func() {

		BeforeEach(func() {
			router = chi.NewRouter()

			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
			corsOpts := kubecors.FromEnvironment()
			router.Use(kubecors.Handler(corsOpts))

			// Endpoint that returns error
			router.Post("/api/v1/signals/prometheus", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"validation-error","title":"Bad Request"}`))
			})

			testServer = httptest.NewServer(router)
		})

		It("should include CORS headers even on error responses", func() {
			// BEHAVIOR: Browser can read error details for better UX
			client := &http.Client{}

			req, err := http.NewRequest("POST", testServer.URL+"/api/v1/signals/prometheus", nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Origin", "https://dashboard.kubernaut.io")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// BEHAVIOR VALIDATION: CORS headers present even on error
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Access-Control-Allow-Origin")).ToNot(BeEmpty(),
				"Error responses must include CORS headers so browser can read error details")
		})
	})

	// ==============================================
	// CATEGORY 3: Production Mode Security (BEHAVIOR)
	// BR-HTTP-015: Production must restrict origins
	// ==============================================

	Context("Production Mode Security - BEHAVIOR", func() {

		BeforeEach(func() {
			router = chi.NewRouter()

			// Production configuration: specific origin only
			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://app.kubernaut.io")
			corsOpts := kubecors.FromEnvironment()
			router.Use(kubecors.Handler(corsOpts))

			router.Get("/api/v1/data", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":"sensitive"}`))
			})

			testServer = httptest.NewServer(router)
		})

		It("should authorize requests from whitelisted origin in production", func() {
			// BEHAVIOR: Authorized frontend CAN access API
			client := &http.Client{}

			req, err := http.NewRequest("GET", testServer.URL+"/api/v1/data", nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Origin", "https://app.kubernaut.io")

			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// BEHAVIOR VALIDATION: Whitelisted origin is authorized
			allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
			Expect(allowOrigin).To(Equal("https://app.kubernaut.io"),
				"Whitelisted origin should receive CORS authorization")
		})

		It("should NOT authorize requests from unknown origin in production", func() {
			// BEHAVIOR: Malicious site CANNOT access API
			client := &http.Client{}

			req, err := http.NewRequest("GET", testServer.URL+"/api/v1/data", nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Origin", "https://malicious-site.com")

			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// BEHAVIOR VALIDATION: Unknown origin is NOT authorized
			allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
			Expect(allowOrigin).ToNot(Equal("https://malicious-site.com"),
				"Unknown origin should NOT receive CORS authorization")
		})
	})
})
