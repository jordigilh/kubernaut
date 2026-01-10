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
	"context"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Test 18: CORS Enforcement
//
// WHY E2E: This test validates the "glue" - that CORS middleware is actually
// wired into the production Gateway server startup. Integration tests with
// httptest.Server can pass even if CORS isn't wired into the real service.
//
// BUSINESS OUTCOME: When the Gateway service starts in a real Kubernetes
// environment, cross-origin requests receive proper CORS headers.
//
// Business Requirements:
// - BR-HTTP-015: Gateway must provide CORS and security policy enforcement
var _ = Describe("Test 18: CORS Enforcement (BR-HTTP-015)", Ordered, Label("e2e", "gateway", "cors"), func() {
	var (
		testCancel context.CancelFunc
		testLogger logr.Logger // DD-005: Use logr.Logger
		httpClient *http.Client
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 3*time.Minute)
		testLogger = logger.WithValues("test", "cors-enforcement") // DD-005: Use WithValues
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 18: CORS Enforcement (BR-HTTP-015) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("")
		testLogger.Info("PURPOSE: Validate CORS middleware is actually wired into")
		testLogger.Info("         production Gateway (not just passing in httptest)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 18: CORS Enforcement - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	// ==============================================
	// CRITICAL: Validates the "Glue"
	// ==============================================

	It("should return CORS headers on cross-origin requests to the running Gateway", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 18.1: Verify CORS middleware is wired into production Gateway")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")

		// Step 1: Make cross-origin request to health endpoint
		testLogger.Info("Step 1: Making cross-origin request to /health endpoint")

		req, err := http.NewRequest("GET", gatewayURL+"/health", nil)
		Expect(err).ToNot(HaveOccurred())

		// Simulate browser cross-origin request
		req.Header.Set("Origin", "https://test-dashboard.kubernaut.io")

		testLogger.Info("Request details",
			"url", gatewayURL+"/health",
			"origin", "https://test-dashboard.kubernaut.io")

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Step 2: Validate CORS headers are present
		testLogger.Info("Step 2: Validating CORS headers in response")

		allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")

		testLogger.Info("CORS Response Headers",
			"Access-Control-Allow-Origin", allowOrigin,
			"status", resp.StatusCode)

		// CRITICAL ASSERTION: CORS headers are present
		// If this fails, CORS middleware is NOT wired into Gateway startup
		Expect(allowOrigin).ToNot(BeEmpty(),
			"CRITICAL: CORS middleware is NOT wired into Gateway server startup! "+
				"Access-Control-Allow-Origin header is missing. "+
				"Check that kubecors.Handler() is added to router in setupRoutes()")

		testLogger.Info("✅ CORS middleware is correctly wired into production Gateway")
		testLogger.Info("")
	})

	It("should handle preflight OPTIONS requests in production Gateway", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 18.2: Verify preflight handling in production Gateway")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")

		// Step 1: Send preflight OPTIONS request
		testLogger.Info("Step 1: Sending preflight OPTIONS request")

		req, err := http.NewRequest("OPTIONS", gatewayURL+"/api/v1/signals/prometheus", nil)
		Expect(err).ToNot(HaveOccurred())

		req.Header.Set("Origin", "https://test-dashboard.kubernaut.io")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type,Authorization")

		testLogger.Info("Preflight request details",
			"url", gatewayURL+"/api/v1/signals/prometheus",
			"origin", "https://test-dashboard.kubernaut.io",
			"requested_method", "POST")

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Step 2: Validate preflight response
		testLogger.Info("Step 2: Validating preflight response")

		allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
		allowMethods := resp.Header.Get("Access-Control-Allow-Methods")

		testLogger.Info("Preflight Response",
			"status", resp.StatusCode,
			"Access-Control-Allow-Origin", allowOrigin,
			"Access-Control-Allow-Methods", allowMethods)

		// Preflight should succeed (200 or 204)
		Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusNoContent)),
			"Preflight request should succeed with 200 or 204")

		// CORS headers should be present
		Expect(allowOrigin).ToNot(BeEmpty(),
			"Preflight must include Access-Control-Allow-Origin")
		Expect(allowMethods).ToNot(BeEmpty(),
			"Preflight must include Access-Control-Allow-Methods")

		testLogger.Info("✅ Preflight handling is correctly implemented in production Gateway")
		testLogger.Info("")
	})

	It("should include CORS headers on readiness endpoint", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 18.3: Verify CORS on readiness endpoint")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")

		req, err := http.NewRequest("GET", gatewayURL+"/ready", nil)
		Expect(err).ToNot(HaveOccurred())

		req.Header.Set("Origin", "https://monitoring.kubernaut.io")

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Use Eventually() to handle Gateway startup timing (may return 503 initially)
		var allowOrigin string
		Eventually(func() int {
			var err error
			resp, err = httpClient.Do(req)
			if err != nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()
			allowOrigin = resp.Header.Get("Access-Control-Allow-Origin")
			return resp.StatusCode
		}, 30*time.Second, 2*time.Second).Should(Equal(http.StatusOK), "Gateway /ready endpoint should be available")

		testLogger.Info("Readiness endpoint CORS",
			"status", resp.StatusCode,
			"Access-Control-Allow-Origin", allowOrigin)

		// Readiness should return OK with CORS headers
		Expect(allowOrigin).ToNot(BeEmpty(),
			"Readiness endpoint must include CORS headers for monitoring dashboards")

		testLogger.Info("✅ Readiness endpoint includes CORS headers")
		testLogger.Info("")
	})
})
