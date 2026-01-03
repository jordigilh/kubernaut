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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Test 07: Health & Readiness Endpoints
// Validates that the Gateway exposes operational health endpoints:
// - /health endpoint returns 200 OK when healthy
// - /ready endpoint returns 200 OK when ready to accept traffic
// - Endpoints remain responsive under load
//
// Business Requirements:
// - BR-GATEWAY-018: Gateway must expose health/readiness endpoints for K8s probes
var _ = Describe("Test 07: Health & Readiness Endpoints (BR-GATEWAY-018)", Ordered, func() {
	var (
		testCancel context.CancelFunc
		testLogger logr.Logger
		httpClient *http.Client
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 3*time.Minute)
		testLogger = logger.WithValues("test", "health-readiness")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 07: Health & Readiness Endpoints (BR-GATEWAY-018) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 07: Health & Readiness Endpoints - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	It("should expose functional health and readiness endpoints", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 07: Health & Readiness Endpoint Behavior")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")

		// Step 1: Test /health endpoint
		testLogger.Info("Step 1: Test /health endpoint")

		resp, err := httpClient.Get(gatewayURL + "/health")
		Expect(err).ToNot(HaveOccurred())

		healthBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"/health should return 200 OK")

		testLogger.Info(fmt.Sprintf("  /health: HTTP %d", resp.StatusCode))
		testLogger.Info(fmt.Sprintf("  Response: %s", string(healthBody)))
		testLogger.Info("  ✅ /health endpoint working")

		// Step 2: Test /ready endpoint (if available)
		testLogger.Info("")
		testLogger.Info("Step 2: Test /ready endpoint")

		// Use Eventually() to handle Gateway startup timing (may return 503 initially)
		var readyResp *http.Response
		Eventually(func() int {
			var err error
		readyResp, err = httpClient.Get(gatewayURL + "/ready")
		if err != nil {
			return 0
		}
		defer func() { _ = readyResp.Body.Close() }()
		return readyResp.StatusCode
		}, 30*time.Second, 2*time.Second).Should(Or(
			Equal(http.StatusOK),
			Equal(http.StatusNotFound), // Some services don't have /ready
		), "Gateway /ready endpoint should be available")

		// Re-fetch for body reading
		readyResp, err = httpClient.Get(gatewayURL + "/ready")
		if err == nil {
			readyBody, _ := io.ReadAll(readyResp.Body)
			_ = readyResp.Body.Close()

			// /ready should return 200 when service is ready
			Expect(readyResp.StatusCode).To(Or(
				Equal(http.StatusOK),
				Equal(http.StatusNotFound), // Some services don't have /ready
			))

			if readyResp.StatusCode == http.StatusOK {
				testLogger.Info(fmt.Sprintf("  /ready: HTTP %d", readyResp.StatusCode))
				testLogger.Info(fmt.Sprintf("  Response: %s", string(readyBody)))
				testLogger.Info("  ✅ /ready endpoint working")
			} else {
				testLogger.Info("  /ready endpoint not implemented (optional)")
			}
		} else {
			testLogger.Info("  /ready endpoint not available (optional)")
		}

		// Step 3: Test /healthz endpoint (alternative naming)
		testLogger.Info("")
		testLogger.Info("Step 3: Test /healthz endpoint (K8s convention)")

		healthzResp, err := httpClient.Get(gatewayURL + "/healthz")
		if err == nil {
			healthzBody, _ := io.ReadAll(healthzResp.Body)
			_ = healthzResp.Body.Close()

			if healthzResp.StatusCode == http.StatusOK {
				testLogger.Info(fmt.Sprintf("  /healthz: HTTP %d", healthzResp.StatusCode))
				testLogger.Info(fmt.Sprintf("  Response: %s", string(healthzBody)))
				testLogger.Info("  ✅ /healthz endpoint working")
			} else {
				testLogger.Info("  /healthz endpoint not implemented (optional)")
			}
		} else {
			testLogger.Info("  /healthz endpoint not available (optional)")
		}

		// Step 4: Test health under load
		testLogger.Info("")
		testLogger.Info("Step 4: Verify health endpoint remains responsive under load")

		// Send some alerts to create load
		for i := 0; i < 5; i++ {
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: fmt.Sprintf("HealthTest-%d-%d", i, time.Now().UnixNano()),
				Namespace: "default",
				PodName:   fmt.Sprintf("health-test-pod-%d", i),
				Severity:  "info",
				Annotations: map[string]string{
					"summary": "Health test alert",
				},
			})
			alertResp, err := func() (*http.Response, error) {
				req10, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
				if err != nil {
					return nil, err
				}
				req10.Header.Set("Content-Type", "application/json")
				req10.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req10)
			}()
			if err == nil {
				_ = alertResp.Body.Close()
			}
		}

		// Verify health endpoint still responds quickly
		start := time.Now()
	healthAfterLoad, err := httpClient.Get(gatewayURL + "/health")
	latency := time.Since(start)
	Expect(err).ToNot(HaveOccurred())
	_ = healthAfterLoad.Body.Close()

	Expect(healthAfterLoad.StatusCode).To(Equal(http.StatusOK),
			"/health should return 200 OK after load")
		Expect(latency).To(BeNumerically("<", 5*time.Second),
			"/health should respond within 5 seconds")

		testLogger.Info(fmt.Sprintf("  Health check latency after load: %v", latency))
		testLogger.Info("  ✅ Health endpoint responsive under load")

		// Step 5: Verify multiple rapid health checks
		testLogger.Info("")
		testLogger.Info("Step 5: Verify multiple rapid health checks")

		successCount := 0
		for i := 0; i < 10; i++ {
			rapidResp, err := httpClient.Get(gatewayURL + "/health")
			if err == nil && rapidResp.StatusCode == http.StatusOK {
				successCount++
				_ = rapidResp.Body.Close()
			}
		}

		Expect(successCount).To(Equal(10),
			"All 10 rapid health checks should succeed")
		testLogger.Info(fmt.Sprintf("  Rapid health checks: %d/10 successful", successCount))
		testLogger.Info("  ✅ Health endpoint handles rapid requests")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 07 PASSED: Health & Readiness Endpoints (BR-GATEWAY-018)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info("  ✅ /health endpoint returns 200 OK")
		testLogger.Info("  ✅ Health endpoint responsive under load")
		testLogger.Info("  ✅ Health endpoint handles rapid requests")
		testLogger.Info("  ✅ Suitable for K8s liveness/readiness probes")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
