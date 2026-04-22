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

	"github.com/google/uuid"
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

		// Issue #753: Health/readiness probes on dedicated port 8081 (/healthz, /readyz)

		// Step 1: Test /healthz endpoint (liveness)
		testLogger.Info("Step 1: Test /healthz endpoint (liveness)")

		resp, err := httpClient.Get(gatewayHealthURL + "/healthz")
		Expect(err).ToNot(HaveOccurred())

		healthBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"/healthz should return 200 OK")

		testLogger.Info(fmt.Sprintf("  /healthz: HTTP %d", resp.StatusCode))
		testLogger.Info(fmt.Sprintf("  Response: %s", string(healthBody)))
		testLogger.Info("  ✅ /healthz endpoint working")

		// Step 2: Test /readyz endpoint (readiness)
		testLogger.Info("")
		testLogger.Info("Step 2: Test /readyz endpoint (readiness)")

		var readyResp *http.Response
		Eventually(func() int {
			var err error
			readyResp, err = httpClient.Get(gatewayHealthURL + "/readyz")
			if err != nil {
				return 0
			}
			defer func() { _ = readyResp.Body.Close() }()
			return readyResp.StatusCode
		}, 30*time.Second, 2*time.Second).Should(
			Equal(http.StatusOK),
			"Gateway /readyz endpoint should be available")

		readyResp, err = httpClient.Get(gatewayHealthURL + "/readyz")
		Expect(err).ToNot(HaveOccurred())
		readyBody, _ := io.ReadAll(readyResp.Body)
		_ = readyResp.Body.Close()

		Expect(readyResp.StatusCode).To(Equal(http.StatusOK))
		testLogger.Info(fmt.Sprintf("  /readyz: HTTP %d", readyResp.StatusCode))
		testLogger.Info(fmt.Sprintf("  Response: %s", string(readyBody)))
		testLogger.Info("  ✅ /readyz endpoint working")

		// Step 4: Test health under load
		testLogger.Info("")
		testLogger.Info("Step 4: Verify health endpoint remains responsive under load")

		// Send some alerts to create load
		for i := 0; i < 5; i++ {
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: fmt.Sprintf("HealthTest-%d-%s", i, uuid.New().String()[:8]),
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
				setE2EAuthHeader(req10)
				return httpClient.Do(req10)
			}()
			if err == nil {
				_ = alertResp.Body.Close()
			}
		}

		// Verify health endpoint still responds quickly
		start := time.Now()
		healthAfterLoad, err := httpClient.Get(gatewayHealthURL + "/healthz")
		latency := time.Since(start)
		Expect(err).ToNot(HaveOccurred())
		_ = healthAfterLoad.Body.Close()

		Expect(healthAfterLoad.StatusCode).To(Equal(http.StatusOK),
			"/healthz should return 200 OK after load")
		Expect(latency).To(BeNumerically("<", 5*time.Second),
			"/healthz should respond within 5 seconds")

		testLogger.Info(fmt.Sprintf("  Health check latency after load: %v", latency))
		testLogger.Info("  ✅ Health endpoint responsive under load")

		// Step 5: Verify multiple rapid health checks
		testLogger.Info("")
		testLogger.Info("Step 5: Verify multiple rapid health checks")

		successCount := 0
		for i := 0; i < 10; i++ {
			rapidResp, err := httpClient.Get(gatewayHealthURL + "/healthz")
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
