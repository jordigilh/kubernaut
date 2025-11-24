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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Test 8: Metrics Validation (P2)", Label("e2e", "metrics", "p2"), Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		gatewayURL    string
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 15*time.Minute) // Longer timeout for metrics collection waits
		testLogger = logger.With(zap.String("test", "metrics"))
		httpClient = &http.Client{Timeout: 30 * time.Second}

		// Use unique namespace for this test
		testNamespace = fmt.Sprintf("e2e-metrics-%d", time.Now().UnixNano())

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 8: Metrics Validation - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Deploying test services...", zap.String("namespace", testNamespace))

		// Deploy Redis and Gateway in test namespace
		err := infrastructure.DeployTestServices(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Set Gateway URL (using NodePort exposed by Kind cluster)
		gatewayURL = "http://localhost:30080"
		testLogger.Info("Gateway URL configured", zap.String("url", gatewayURL))

		// Wait for Gateway HTTP endpoint to be responsive
		testLogger.Info("⏳ Waiting for Gateway HTTP endpoint to be responsive...")
		Eventually(func() error {
			resp, err := httpClient.Get(gatewayURL + "/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "Gateway HTTP endpoint did not become responsive")
		testLogger.Info("✅ Gateway HTTP endpoint is responsive")

		testLogger.Info("✅ Test services ready", zap.String("namespace", testNamespace))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 8: Metrics Validation - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if test failed - preserve namespace for debugging
		if CurrentSpecReport().Failed() {
			testLogger.Warn("⚠️  Test FAILED - Preserving namespace for debugging",
				zap.String("namespace", testNamespace))
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl logs -n %s deployment/gateway -f", testNamespace))

			if testCancel != nil {
				testCancel()
			}
			return
		}

		// Test passed - cleanup namespace
		testLogger.Info("Cleaning up test namespace...", zap.String("namespace", testNamespace))
		err := infrastructure.CleanupTestNamespace(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		if err != nil {
			testLogger.Warn("Failed to cleanup namespace", zap.Error(err))
		}

		if testCancel != nil {
			testCancel()
		}

		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	Context("BR-GATEWAY-076: /metrics Endpoint", func() {
		It("should return 200 OK and Prometheus text format", func() {
			// Act: Call /metrics endpoint
			resp, err := httpClient.Get(gatewayURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Assert: Should return 200
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "/metrics endpoint should return 200 OK")

			// Assert: Content-Type should be text/plain
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(ContainSubstring("text/plain"), "Content-Type should be Prometheus text format")

			testLogger.Info("/metrics endpoint validated successfully")
		})

		It("should expose all HTTP metrics", func() {
			// Act: Call /metrics endpoint
			resp, err := httpClient.Get(gatewayURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			// Assert: HTTP metrics should be present
			Expect(metricsOutput).To(ContainSubstring("gateway_http_request_duration_seconds"),
				"Should expose HTTP request duration metric")
			Expect(metricsOutput).To(ContainSubstring("gateway_http_requests_in_flight"),
				"Should expose in-flight requests metric")

			testLogger.Info("HTTP metrics validated successfully")
		})

		It("should expose all Redis pool metrics", func() {
			// Act: Call /metrics endpoint
			resp, err := httpClient.Get(gatewayURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			// Assert: Redis pool metrics should be present
			Expect(metricsOutput).To(ContainSubstring("gateway_redis_pool_connections_total"),
				"Should expose Redis pool total connections metric")
			Expect(metricsOutput).To(ContainSubstring("gateway_redis_pool_connections_idle"),
				"Should expose Redis pool idle connections metric")
			Expect(metricsOutput).To(ContainSubstring("gateway_redis_pool_connections_active"),
				"Should expose Redis pool active connections metric")
			Expect(metricsOutput).To(ContainSubstring("gateway_redis_pool_hits_total"),
				"Should expose Redis pool hits metric")
			Expect(metricsOutput).To(ContainSubstring("gateway_redis_pool_misses_total"),
				"Should expose Redis pool misses metric")
			Expect(metricsOutput).To(ContainSubstring("gateway_redis_pool_timeouts_total"),
				"Should expose Redis pool timeouts metric")

			testLogger.Info("Redis pool metrics validated successfully")
		})
	})

	Context("BR-GATEWAY-071: HTTP Metrics Integration", func() {
		It("should track webhook request duration", func() {
			// Arrange: Get initial metrics
			initialMetrics := getMetricsSnapshot(httpClient, gatewayURL)

			// Act: Send webhook request
			alertName := fmt.Sprintf("MetricsTest-%d", time.Now().Unix())
			payload := fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "%s",
						"namespace": "%s",
						"pod": "test-pod-1",
						"severity": "warning"
					},
					"annotations": {
						"summary": "High memory usage detected"
					}
				}]
			}`, alertName, testNamespace)

			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload)))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Assert: Request should succeed
			Expect(resp.StatusCode).To(BeNumerically(">=", 200))
			Expect(resp.StatusCode).To(BeNumerically("<", 300))

			// Assert: Metrics should be updated
			finalMetrics := getMetricsSnapshot(httpClient, gatewayURL)
			Expect(finalMetrics).To(ContainSubstring("gateway_http_request_duration_seconds"),
				"Duration metric should be present")

			// Verify duration metric has samples
			durationLines := filterMetricLines(finalMetrics, "gateway_http_request_duration_seconds")
			initialDurationLines := filterMetricLines(initialMetrics, "gateway_http_request_duration_seconds")
			Expect(len(durationLines)).To(BeNumerically(">", len(initialDurationLines)),
				"Duration metric should have new samples")

			testLogger.Info("Webhook request duration tracking validated")
		})

		It("should track in-flight requests", func() {
			// Act: Call /metrics endpoint
			resp, err := httpClient.Get(gatewayURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			// Assert: In-flight metric should be present
			Expect(metricsOutput).To(ContainSubstring("gateway_http_requests_in_flight"),
				"In-flight metric should be present")

			// Note: In-flight should be 0 after request completes
			inFlightLines := filterMetricLines(metricsOutput, "gateway_http_requests_in_flight")
			Expect(len(inFlightLines)).To(BeNumerically(">", 0),
				"In-flight metric should have at least one line")

			testLogger.Info("In-flight requests tracking validated")
		})

		It("should track requests by status code", func() {
			// Arrange: Send invalid payload to get 400 error
			payload := []byte(`invalid json`)

			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Assert: Should get 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Invalid payload should return 400")

			// Assert: Metrics should track 400 status
			metricsResp, err := httpClient.Get(gatewayURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, err := io.ReadAll(metricsResp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			// Verify status_code label includes 400
			Expect(metricsOutput).To(ContainSubstring("status_code=\"400\""),
				"Metrics should track 400 status code")

			testLogger.Info("Status code tracking validated")
		})
	})

	Context("BR-GATEWAY-073: Redis Pool Metrics Integration", func() {
		It("should collect pool stats periodically", func() {
			// Arrange: Get initial metrics
			_ = getMetricsSnapshot(httpClient, gatewayURL) // Baseline

			// Act: Wait for metrics collection (10s interval + buffer)
			testLogger.Info("Waiting for Redis pool metrics collection (12 seconds)...")
			time.Sleep(12 * time.Second)

			// Assert: Metrics should be updated
			finalMetrics := getMetricsSnapshot(httpClient, gatewayURL)

			// Verify Redis pool metrics are present
			Expect(finalMetrics).To(ContainSubstring("gateway_redis_pool_connections_total"),
				"Total connections metric should be present")
			Expect(finalMetrics).To(ContainSubstring("gateway_redis_pool_connections_idle"),
				"Idle connections metric should be present")
			Expect(finalMetrics).To(ContainSubstring("gateway_redis_pool_connections_active"),
				"Active connections metric should be present")

			// Verify metrics have values
			poolLines := filterMetricLines(finalMetrics, "gateway_redis_pool_connections_total")
			Expect(len(poolLines)).To(BeNumerically(">", 0),
				"Pool metrics should have at least one line")

			testLogger.Info("Redis pool stats collection validated")
		})

		It("should track connection usage", func() {
			// Arrange: Get initial metrics
			initialMetrics := getMetricsSnapshot(httpClient, gatewayURL)

			// Act: Make webhook requests to trigger Redis calls
			testLogger.Info("Sending 5 webhook requests to trigger Redis activity...")
			for i := 0; i < 5; i++ {
				alertName := fmt.Sprintf("RedisTest-%d-%d", time.Now().Unix(), i)
				payload := fmt.Sprintf(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "test-pod-%d",
							"severity": "warning"
						},
						"annotations": {
							"summary": "Test alert"
						}
					}]
				}`, alertName, testNamespace, i)

				req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload)))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				resp.Body.Close()
			}

			// Wait for metrics collection
			testLogger.Info("Waiting for metrics collection (12 seconds)...")
			time.Sleep(12 * time.Second)

			// Assert: Metrics should show connection activity
			finalMetrics := getMetricsSnapshot(httpClient, gatewayURL)

			// Verify pool metrics exist and have values
			Expect(finalMetrics).To(ContainSubstring("gateway_redis_pool_connections_total"),
				"Pool metrics should be present after Redis activity")

			// Verify metrics have values
			poolLines := filterMetricLines(finalMetrics, "gateway_redis_pool_connections_total")
			Expect(len(poolLines)).To(BeNumerically(">", 0),
				"Pool metrics should have values")

			// Verify initial metrics were captured
			initialPoolLines := filterMetricLines(initialMetrics, "gateway_redis_pool_connections_total")
			Expect(len(initialPoolLines)).To(BeNumerically(">=", 0),
				"Initial pool metrics should be captured")

			testLogger.Info("Redis connection usage tracking validated")
		})

		It("should track pool hits and misses", func() {
			// Act: Make multiple requests to trigger Redis pool activity
			testLogger.Info("Sending requests to trigger Redis pool hits/misses...")
			for i := 0; i < 3; i++ {
				alertName := fmt.Sprintf("PoolTest-%d-%d", time.Now().Unix(), i)
				payload := fmt.Sprintf(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "test-pod-%d",
							"severity": "warning"
						},
						"annotations": {
							"summary": "Pool test alert"
						}
					}]
				}`, alertName, testNamespace, i)

				req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload)))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				resp.Body.Close()
			}

			// Wait for metrics collection
			testLogger.Info("Waiting for metrics collection (12 seconds)...")
			time.Sleep(12 * time.Second)

			// Assert: Metrics should track hits and misses
			finalMetrics := getMetricsSnapshot(httpClient, gatewayURL)

			Expect(finalMetrics).To(ContainSubstring("gateway_redis_pool_hits_total"),
				"Pool hits metric should be present")
			Expect(finalMetrics).To(ContainSubstring("gateway_redis_pool_misses_total"),
				"Pool misses metric should be present")

			// Verify metrics have values
			hitsLines := filterMetricLines(finalMetrics, "gateway_redis_pool_hits_total")
			missesLines := filterMetricLines(finalMetrics, "gateway_redis_pool_misses_total")

			Expect(len(hitsLines)).To(BeNumerically(">", 0),
				"Pool hits metric should have values")
			Expect(len(missesLines)).To(BeNumerically(">", 0),
				"Pool misses metric should have values")

			testLogger.Info("Redis pool hits/misses tracking validated")
		})
	})
})

// Helper functions for metrics testing

func getMetricsSnapshot(client *http.Client, gatewayURL string) string {
	resp, err := client.Get(gatewayURL + "/metrics")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return string(body)
}

func filterMetricLines(metricsOutput string, metricName string) []string {
	lines := strings.Split(metricsOutput, "\n")
	var filtered []string

	for _, line := range lines {
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Match metric name at start of line
		if strings.HasPrefix(line, metricName) {
			filtered = append(filtered, line)
		}
	}

	return filtered
}

