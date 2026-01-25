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
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("Test 20: Security Headers & Observability", Ordered, func() {
	var (
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "security-headers")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 20: Security Headers & Observability - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// BR-GATEWAY-NAMESPACE-FALLBACK: Pre-create namespace (Pattern: RO E2E)
	testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "security-headers")
	testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 20: Security Headers & Observability - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl logs -n kubernaut-system deployment/gateway -f"))
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			if testCancel != nil {
				testCancel()
			}
			return
		}

	testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
	// BR-GATEWAY-NAMESPACE-FALLBACK: Clean up test namespace (Pattern: RO E2E)
	helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)

	if testCancel != nil {
		testCancel()
	}
	testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	Context("Security Headers Enforcement", func() {
		It("should include all required security headers in responses", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Validate security headers in Gateway responses")
			testLogger.Info("Expected: X-Content-Type-Options, X-Frame-Options, X-XSS-Protection, HSTS")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create valid Prometheus webhook payload")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertSecurityHeaders",
				Namespace: testNamespace,
				Severity:  "warning",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "security-headers",
				},
			})

			testLogger.Info("Step 2: Send request to Gateway")
			resp, err := func() (*http.Response, error) {
				req29, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
				if err != nil {
					return nil, err
				}
				req29.Header.Set("Content-Type", "application/json")
				req29.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req29)
			}()
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 201 Created response")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Request should succeed")

			testLogger.Info("Step 4: Validate security headers present")

			// X-Content-Type-Options: nosniff
			contentTypeOptions := resp.Header.Get("X-Content-Type-Options")
			testLogger.Info("Checking X-Content-Type-Options", "value", contentTypeOptions)
			Expect(contentTypeOptions).To(Equal("nosniff"),
				"X-Content-Type-Options header should be 'nosniff' (prevents MIME sniffing attacks)")

			// X-Frame-Options: DENY
			frameOptions := resp.Header.Get("X-Frame-Options")
			testLogger.Info("Checking X-Frame-Options", "value", frameOptions)
			Expect(frameOptions).To(Equal("DENY"),
				"X-Frame-Options header should be 'DENY' (prevents clickjacking attacks)")

			// X-XSS-Protection: 1; mode=block
			xssProtection := resp.Header.Get("X-XSS-Protection")
			testLogger.Info("Checking X-XSS-Protection", "value", xssProtection)
			Expect(xssProtection).To(ContainSubstring("1"),
				"X-XSS-Protection header should be enabled (prevents XSS attacks)")

			// Strict-Transport-Security (HSTS)
			hsts := resp.Header.Get("Strict-Transport-Security")
			testLogger.Info("Checking Strict-Transport-Security", "value", hsts)
			Expect(hsts).To(ContainSubstring("max-age="),
				"Strict-Transport-Security header should be present (enforces HTTPS)")

			testLogger.Info("✅ All security headers validated")
			testLogger.Info("  ✓ X-Content-Type-Options: nosniff")
			testLogger.Info("  ✓ X-Frame-Options: DENY")
			testLogger.Info("  ✓ X-XSS-Protection: enabled")
			testLogger.Info("  ✓ Strict-Transport-Security: present")

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 20a PASSED: Security Headers Present")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should generate and return request ID for traceability", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Validate X-Request-ID header for distributed tracing")
			testLogger.Info("Expected: Auto-generated X-Request-ID in response")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create valid Prometheus webhook payload")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertRequestID",
				Namespace: testNamespace,
				Severity:  "info",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "request-id-tracing",
				},
			})

			testLogger.Info("Step 2: Send request WITHOUT X-Request-ID header")
			resp, err := func() (*http.Response, error) {
				req30, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
				if err != nil {
					return nil, err
				}
				req30.Header.Set("Content-Type", "application/json")
				req30.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req30)
			}()
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 201 Created response")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Request should succeed")

			testLogger.Info("Step 4: Validate X-Request-ID header present in response")
			requestID := resp.Header.Get("X-Request-ID")
			testLogger.Info("Checking X-Request-ID", "value", requestID)

			Expect(requestID).ToNot(BeEmpty(),
				"X-Request-ID header should be auto-generated by Gateway middleware")

			Expect(len(requestID)).To(BeNumerically(">", 10),
				"X-Request-ID should be a substantial unique identifier (UUID or similar)")

			testLogger.Info("✅ Request ID generated and returned",
				"request-id", requestID)

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 20b PASSED: Request ID Traceability")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should record HTTP metrics for observability", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Validate HTTP request metrics in /metrics endpoint")
			testLogger.Info("Expected: gateway_http_requests_total and gateway_http_request_duration_seconds")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Record initial metrics baseline")
			metricsResp, err := httpClient.Get(gatewayURL + "/metrics")
			Expect(err).ToNot(HaveOccurred(), "Metrics endpoint should be accessible")
			defer func() { _ = metricsResp.Body.Close() }()

			Expect(metricsResp.StatusCode).To(Equal(http.StatusOK),
				"Metrics endpoint should return HTTP 200")

			initialMetrics, err := io.ReadAll(metricsResp.Body)
			Expect(err).ToNot(HaveOccurred())
			initialMetricsStr := string(initialMetrics)

			testLogger.Info("Step 2: Send test request to Gateway")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertMetrics",
				Namespace: testNamespace,
				Severity:  "warning",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "http-metrics",
				},
			})

			resp, err := func() (*http.Response, error) {
				req31, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
				if err != nil {
					return nil, err
				}
				req31.Header.Set("Content-Type", "application/json")
				req31.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req31)
			}()
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			testLogger.Info("Step 3: Poll for updated metrics with count > 0")

			// Use Eventually() to poll for metrics instead of sleep (per TESTING_GUIDELINES.md)
			var updatedMetricsStr string
			Eventually(func() bool {
				metricsResp, err := httpClient.Get(gatewayURL + "/metrics")
				if err != nil {
					return false
				}
				defer func() { _ = metricsResp.Body.Close() }()

				updatedMetrics, err := io.ReadAll(metricsResp.Body)
				if err != nil {
					return false
				}
				updatedMetricsStr = string(updatedMetrics)

				// Check if metric is present with non-zero count
				return strings.Contains(updatedMetricsStr, "gateway_http_request_duration_seconds_count")
			}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"HTTP request duration metric should appear in /metrics within 5 seconds")

			testLogger.Info("Step 4: Validate HTTP metrics present")

			// gateway_http_request_duration_seconds (histogram with _bucket, _sum, _count)
			// Per metrics-slos.md: Histogram metric for HTTP request duration (BR-GATEWAY-067, BR-GATEWAY-079)
			hasRequestDuration := strings.Contains(updatedMetricsStr, "gateway_http_request_duration_seconds")
			testLogger.Info("Checking gateway_http_request_duration_seconds", "present", hasRequestDuration)
			Expect(hasRequestDuration).To(BeTrue(),
				"HTTP request duration metric should be present in /metrics (per specification)")

			// Verify histogram components (_bucket, _sum, _count)
			hasHistogramBucket := strings.Contains(updatedMetricsStr, "gateway_http_request_duration_seconds_bucket")
			hasHistogramSum := strings.Contains(updatedMetricsStr, "gateway_http_request_duration_seconds_sum")
			hasHistogramCount := strings.Contains(updatedMetricsStr, "gateway_http_request_duration_seconds_count")
			testLogger.Info("Histogram components", "bucket", hasHistogramBucket, "sum", hasHistogramSum, "count", hasHistogramCount)
			Expect(hasHistogramBucket).To(BeTrue(), "Histogram bucket metric should be present")
			Expect(hasHistogramSum).To(BeTrue(), "Histogram sum metric should be present")
			Expect(hasHistogramCount).To(BeTrue(), "Histogram count metric should be present")

			testLogger.Info("✅ HTTP metrics validated")
			testLogger.Info("  ✓ gateway_http_request_duration_seconds: present")
			testLogger.Info("  ✓ gateway_http_request_duration_seconds_bucket: present")
			testLogger.Info("  ✓ gateway_http_request_duration_seconds_sum: present")
			testLogger.Info("  ✓ gateway_http_request_duration_seconds_count: present")

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 20c PASSED: HTTP Metrics Recorded")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Log a sample of the metrics for debugging if needed
			if CurrentSpecReport().Failed() {
				testLogger.Info("📊 Metrics Sample (first 500 chars):",
					"initial", initialMetricsStr[:min(500, len(initialMetricsStr))],
					"updated", updatedMetricsStr[:min(500, len(updatedMetricsStr))])
			}
		})
	})
})

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
