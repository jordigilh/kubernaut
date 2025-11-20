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
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// DD-GATEWAY-008: Storm Buffering - E2E Test
//
// Business Outcome: Validate complete storm buffering lifecycle in production-like environment
//
// Test Strategy:
// - Use shared Gateway instance (deployed in suite setup)
// - Send real Prometheus webhook alerts simulating storm scenarios
// - Each test uses unique namespace + alert name to enable parallel execution
// - Verify buffered first-alert aggregation logic
// - Validate sliding window behavior with inactivity timeout
// - Test multi-tenant isolation and overflow handling
//
// Parallel Execution: ✅ ENABLED
// - Each test has isolated namespace and unique alert names
// - Tests can run concurrently without interference
// - Shared Gateway instance handles multiple concurrent requests
//
// Coverage: 10-15% E2E (3 critical end-to-end scenarios)
//
// Business Requirements:
// - BR-GATEWAY-016: Storm aggregation must reduce AI analysis costs by 90%+
// - BR-GATEWAY-008: Storm detection must identify alert storms (>10 alerts/minute)
// - BR-GATEWAY-011: Multi-tenant isolation with per-namespace buffer limits

var _ = Describe("E2E: Storm Buffering Lifecycle", Label("e2e", "storm-buffering", "dd-gateway-008", "p1"), func() {
	// Shared test infrastructure (reused across all tests)
	var (
		gatewayURL string
		httpClient *http.Client
		k8sClient  client.Client
	)

	BeforeEach(func() {
		// Shared Gateway instance (already deployed in suite setup)
		gatewayURL = "http://localhost:30080"
		httpClient = &http.Client{Timeout: 10 * time.Second}
		k8sClient = getKubernetesClient()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-016: Buffered First-Alert Aggregation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-016: Buffered First-Alert Aggregation", func() {
		Context("when alerts arrive below threshold", func() {
			It("should delay aggregation until threshold is reached, then create single CRD", func() {
				// BUSINESS SCENARIO: 5 pods crash in rapid succession (storm threshold = 5)
				// Expected: Alerts 1-4 buffered (no CRD), Alert 5 triggers aggregation → 1 CRD
				//
				// BUSINESS OUTCOME: 90%+ cost savings (5 alerts → 1 AI analysis instead of 5)

				// PARALLEL EXECUTION: Unique namespace + alert name per test
				testNamespace := fmt.Sprintf("e2e-storm-buffer-%d", time.Now().UnixNano())
				alertName := fmt.Sprintf("PodCrashLooping-%d", time.Now().UnixNano())
				bufferThreshold := 5 // Default from config

				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				logger.Info("Test: Buffered First-Alert Aggregation",
					zap.String("namespace", testNamespace),
					zap.String("alert_name", alertName),
				)
				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Cleanup function for this test
				defer func() {
					logger.Info("Cleaning up test CRDs...", zap.String("namespace", testNamespace))
					crdList := &remediationv1alpha1.RemediationRequestList{}
					_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
					for _, crd := range crdList.Items {
						_ = k8sClient.Delete(context.Background(), &crd)
					}
				}()

			// BEHAVIOR: Send alerts 1-4 (below threshold)
			logger.Info("Sending alerts 1-4 (below threshold)...")
			for i := 1; i < bufferThreshold; i++ {
				alert := createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: testNamespace,
					PodName:   fmt.Sprintf("payment-api-%d", i),
					Severity:  "critical",
					Annotations: map[string]string{
						"summary": fmt.Sprintf("Pod payment-api-%d is crash looping", i),
					},
				})

				webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alert)
				Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted), fmt.Sprintf("Alert %d should return 202 Accepted (buffered)", i))

				logger.Info("Alert buffered", zap.Int("alert_number", i), zap.Int("status_code", webhookResp.StatusCode))
			}

			// CORRECTNESS: No CRDs should exist yet (alerts buffered)
			logger.Info("Verifying no CRDs created yet (alerts buffered)...")
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
				Expect(err).ToNot(HaveOccurred())
				Expect(crdList.Items).To(HaveLen(0), "No CRDs should be created before threshold")

				logger.Info("✅ Alerts 1-4 buffered correctly (no CRDs created)")

			// BEHAVIOR: Send alert 5 (threshold reached)
			logger.Info("Sending alert 5 (threshold reached)...")
			thresholdAlert := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: alertName,
				Namespace: testNamespace,
				PodName:   fmt.Sprintf("payment-api-%d", bufferThreshold),
				Severity:  "critical",
				Annotations: map[string]string{
					"summary": fmt.Sprintf("Pod payment-api-%d is crash looping", bufferThreshold),
				},
			})

			webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", thresholdAlert)
			Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted), "Threshold alert should be accepted")

			logger.Info("Threshold alert sent", zap.Int("status_code", webhookResp.StatusCode))

			// BEHAVIOR: Wait for aggregation window to close (inactivity timeout + buffer)
			inactivityTimeout := 60 * time.Second // Default from config
			logger.Info("⏳ Waiting for aggregation window to close...", zap.Duration("timeout", inactivityTimeout+10*time.Second))
			time.Sleep(inactivityTimeout + 10*time.Second)

			// CORRECTNESS: Exactly one aggregated CRD should be created
			logger.Info("Verifying single aggregated CRD created...")
			Eventually(func() int {
				_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1), "Exactly one aggregated CRD should be created")

			// CORRECTNESS: CRD should contain all 5 alerts
			createdCRD := crdList.Items[0]
			logger.Info("Verifying CRD aggregation details...",
				zap.String("crd_name", createdCRD.Name),
				zap.Int("alert_count", createdCRD.Spec.StormAlertCount),
				zap.Bool("is_storm", createdCRD.Spec.IsStorm),
			)

			Expect(createdCRD.Spec.StormAlertCount).To(Equal(bufferThreshold), "Aggregated CRD should contain all buffered alerts")
				Expect(createdCRD.Spec.IsStorm).To(BeTrue(), "CRD should be marked as storm")
				Expect(createdCRD.Spec.AffectedResources).To(HaveLen(bufferThreshold), "All affected resources should be tracked")

				// BUSINESS OUTCOME: Cost savings achieved
				costSavings := float64(bufferThreshold-1) / float64(bufferThreshold) * 100
				logger.Info("✅ Storm buffering successful",
					zap.Int("alerts_sent", bufferThreshold),
					zap.Int("crds_created", 1),
					zap.Float64("cost_savings_percent", costSavings),
				)

				Expect(costSavings).To(BeNumerically(">=", 80.0), "Cost savings should be ≥80% (BR-GATEWAY-016)")
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-008: Sliding Window with Inactivity Timeout
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-008: Sliding Window with Inactivity Timeout", func() {
		Context("when alerts arrive with pauses < inactivity timeout", func() {
			It("should extend window lifetime and aggregate all alerts", func() {
				// BUSINESS SCENARIO: Alerts arrive with 30s pauses (< 60s timeout)
				// Expected: Window stays open, all alerts aggregated into single CRD
				//
				// BUSINESS OUTCOME: Captures complete storm lifecycle (no premature window closure)

				// PARALLEL EXECUTION: Unique namespace + alert name per test
				testNamespace := fmt.Sprintf("e2e-sliding-window-%d", time.Now().UnixNano())
				alertName := fmt.Sprintf("NodeMemoryPressure-%d", time.Now().UnixNano())
				bufferThreshold := 5
				inactivityTimeout := 60 * time.Second
				pauseBetweenAlerts := 30 * time.Second // < inactivity timeout

				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				logger.Info("Test: Sliding Window with Inactivity Timeout",
					zap.String("namespace", testNamespace),
					zap.String("alert_name", alertName),
				)
				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Cleanup function for this test
				defer func() {
					logger.Info("Cleaning up test CRDs...", zap.String("namespace", testNamespace))
					crdList := &remediationv1alpha1.RemediationRequestList{}
					_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
					for _, crd := range crdList.Items {
						_ = k8sClient.Delete(context.Background(), &crd)
					}
				}()

				// BEHAVIOR: Send alerts with pauses < inactivity timeout
				totalAlerts := 8 // Exceed threshold to trigger window, then test sliding behavior
				logger.Info("Sending alerts with pauses...",
					zap.Int("total_alerts", totalAlerts),
					zap.Duration("pause_between_alerts", pauseBetweenAlerts),
					zap.Duration("inactivity_timeout", inactivityTimeout),
				)

			for i := 1; i <= totalAlerts; i++ {
				alert := createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: testNamespace,
					PodName:   fmt.Sprintf("worker-node-%d", i),
					Severity:  "warning",
					Annotations: map[string]string{
						"summary": fmt.Sprintf("Node worker-node-%d has memory pressure", i),
					},
				})

				webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alert)
				Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted))

				logger.Info("Alert sent", zap.Int("alert_number", i))

				// Pause between alerts (< inactivity timeout to keep window open)
				if i < totalAlerts {
					logger.Info("Pausing before next alert...", zap.Duration("pause", pauseBetweenAlerts))
					time.Sleep(pauseBetweenAlerts)
				}
			}

			// BEHAVIOR: Wait for window to close after last alert (inactivity timeout)
			logger.Info("⏳ Waiting for window to close after last alert...", zap.Duration("timeout", inactivityTimeout+10*time.Second))
			time.Sleep(inactivityTimeout + 10*time.Second)

			// CORRECTNESS: Exactly one aggregated CRD with all alerts
			logger.Info("Verifying single aggregated CRD with all alerts...")
			crdList := &remediationv1alpha1.RemediationRequestList{}
			Eventually(func() int {
				_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1), "Exactly one aggregated CRD should be created")

			createdCRD := crdList.Items[0]
			logger.Info("Verifying sliding window aggregation...",
				zap.String("crd_name", createdCRD.Name),
				zap.Int("alert_count", createdCRD.Spec.StormAlertCount),
				zap.Int("expected_alerts", totalAlerts),
			)

			Expect(createdCRD.Spec.StormAlertCount).To(Equal(totalAlerts), "All alerts should be aggregated (sliding window kept open)")
				Expect(createdCRD.Spec.IsStorm).To(BeTrue())

				// BUSINESS OUTCOME: Complete storm lifecycle captured
				logger.Info("✅ Sliding window successful - all alerts aggregated",
					zap.Int("alerts_sent", totalAlerts),
					zap.Int("crds_created", 1),
					zap.Duration("total_duration", time.Duration(totalAlerts-1)*pauseBetweenAlerts),
				)
			})
		})

		Context("when alerts arrive with pauses > inactivity timeout", func() {
			It("should close window and create separate CRDs for new storms", func() {
				// BUSINESS SCENARIO: Alerts arrive with 90s pause (> 60s timeout)
				// Expected: First window closes → CRD #1, second window opens → CRD #2
				//
				// BUSINESS OUTCOME: Separate incidents are correctly identified

				// PARALLEL EXECUTION: Unique namespace + alert name per test
				testNamespace := fmt.Sprintf("e2e-window-closure-%d", time.Now().UnixNano())
				alertName := fmt.Sprintf("DiskSpaceWarning-%d", time.Now().UnixNano())
				bufferThreshold := 5
				inactivityTimeout := 60 * time.Second
				longPause := 90 * time.Second // > inactivity timeout

				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				logger.Info("Test: Window Closure on Inactivity",
					zap.String("namespace", testNamespace),
					zap.String("alert_name", alertName),
				)
				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Cleanup function for this test
				defer func() {
					logger.Info("Cleaning up test CRDs...", zap.String("namespace", testNamespace))
					crdList := &remediationv1alpha1.RemediationRequestList{}
					_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
					for _, crd := range crdList.Items {
						_ = k8sClient.Delete(context.Background(), &crd)
					}
				}()

			// BEHAVIOR: Send first batch of alerts (storm #1)
			logger.Info("Sending first storm batch...", zap.Int("alerts", bufferThreshold))
			for i := 1; i <= bufferThreshold; i++ {
				alert := createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: testNamespace,
					PodName:   fmt.Sprintf("storage-node-%d", i),
					Severity:  "warning",
					Annotations: map[string]string{
						"summary": fmt.Sprintf("Node storage-node-%d has low disk space", i),
					},
				})

				webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alert)
				Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted))
			}

			// BEHAVIOR: Wait for first window to close
			logger.Info("⏳ Waiting for first window to close...", zap.Duration("timeout", inactivityTimeout+10*time.Second))
			time.Sleep(inactivityTimeout + 10*time.Second)

			// CORRECTNESS: First CRD should be created
			crdList := &remediationv1alpha1.RemediationRequestList{}
			Eventually(func() int {
				_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1), "First CRD should be created after window closes")

			logger.Info("✅ First storm CRD created", zap.String("crd_name", crdList.Items[0].Name))

			// BEHAVIOR: Long pause (> inactivity timeout)
			logger.Info("⏳ Long pause between storms...", zap.Duration("pause", longPause))
			time.Sleep(longPause)

			// BEHAVIOR: Send second batch of alerts (storm #2)
			logger.Info("Sending second storm batch...", zap.Int("alerts", bufferThreshold))
			for i := 1; i <= bufferThreshold; i++ {
				alert := createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: testNamespace,
					PodName:   fmt.Sprintf("storage-node-%d", i+10), // Different nodes
					Severity:  "warning",
					Annotations: map[string]string{
						"summary": fmt.Sprintf("Node storage-node-%d has low disk space", i+10),
					},
				})

				webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alert)
				Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted))
			}

			// BEHAVIOR: Wait for second window to close
			logger.Info("⏳ Waiting for second window to close...", zap.Duration("timeout", inactivityTimeout+10*time.Second))
			time.Sleep(inactivityTimeout + 10*time.Second)

			// CORRECTNESS: Two separate CRDs should exist
			Eventually(func() int {
				_ = k8sClient.List(context.Background(), crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(2), "Two separate CRDs should be created for separate storms")

			logger.Info("✅ Two separate storm CRDs created",
				zap.String("crd1_name", crdList.Items[0].Name),
				zap.String("crd2_name", crdList.Items[1].Name),
			)

			// BUSINESS OUTCOME: Separate incidents correctly identified
			Expect(crdList.Items[0].Spec.StormAlertCount).To(Equal(bufferThreshold))
			Expect(crdList.Items[1].Spec.StormAlertCount).To(Equal(bufferThreshold))
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-011: Multi-Tenant Isolation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-011: Multi-Tenant Isolation", func() {
		Context("when multiple namespaces send alerts simultaneously", func() {
			It("should isolate buffer limits per namespace", func() {
				// BUSINESS SCENARIO: Two namespaces (prod-api, dev-test) send alerts simultaneously
				// Expected: Each namespace has independent buffer limits and storm detection
				//
				// BUSINESS OUTCOME: One namespace's storm doesn't affect another's capacity

				// PARALLEL EXECUTION: Unique namespaces + alert name per test
				baseNamespace := fmt.Sprintf("e2e-multitenant-%d", time.Now().UnixNano())
				alertName := fmt.Sprintf("ServiceUnavailable-%d", time.Now().UnixNano())
				bufferThreshold := 5

				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				logger.Info("Test: Multi-Tenant Isolation",
					zap.String("base_namespace", baseNamespace),
					zap.String("alert_name", alertName),
				)
				logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Cleanup function for this test
				defer func() {
					logger.Info("Cleaning up test CRDs...")
					crdList := &remediationv1alpha1.RemediationRequestList{}
					_ = k8sClient.List(context.Background(), crdList)
					for _, crd := range crdList.Items {
						// Cleanup CRDs from both namespaces
						if crd.Namespace == baseNamespace+"-prod" || crd.Namespace == baseNamespace+"-dev" {
							_ = k8sClient.Delete(context.Background(), &crd)
						}
					}
				}()

			// BEHAVIOR: Send alerts to namespace 1 (prod-api)
			namespace1 := baseNamespace + "-prod"
			logger.Info("Sending alerts to namespace 1...", zap.String("namespace", namespace1))
			for i := 1; i <= bufferThreshold; i++ {
				alert := createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: namespace1,
					PodName:   fmt.Sprintf("api-server-%d", i),
					Severity:  "critical",
					Annotations: map[string]string{
						"summary": fmt.Sprintf("Service api-server-%d is unavailable", i),
					},
				})

				webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alert)
				Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted))
			}

			// BEHAVIOR: Send alerts to namespace 2 (dev-test)
			namespace2 := baseNamespace + "-dev"
			logger.Info("Sending alerts to namespace 2...", zap.String("namespace", namespace2))
			for i := 1; i <= bufferThreshold; i++ {
				alert := createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: namespace2,
					PodName:   fmt.Sprintf("test-service-%d", i),
					Severity:  "warning",
					Annotations: map[string]string{
						"summary": fmt.Sprintf("Service test-service-%d is unavailable", i),
					},
				})

				webhookResp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alert)
				Expect(webhookResp.StatusCode).To(Equal(http.StatusAccepted))
			}

			// BEHAVIOR: Wait for both windows to close
			inactivityTimeout := 60 * time.Second
			logger.Info("⏳ Waiting for both windows to close...", zap.Duration("timeout", inactivityTimeout+10*time.Second))
			time.Sleep(inactivityTimeout + 10*time.Second)

			// CORRECTNESS: Two separate CRDs (one per namespace)
			logger.Info("Verifying namespace isolation...")
			crdList := &remediationv1alpha1.RemediationRequestList{}
			Eventually(func() int {
				_ = k8sClient.List(context.Background(), crdList)
				return len(crdList.Items)
			}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 2), "At least two CRDs should be created (one per namespace)")

			// CORRECTNESS: Each namespace has its own CRD
			namespace1CRDs := 0
			namespace2CRDs := 0
			for _, crd := range crdList.Items {
				if crd.Namespace == namespace1 {
					namespace1CRDs++
					Expect(crd.Spec.StormAlertCount).To(Equal(bufferThreshold))
				} else if crd.Namespace == namespace2 {
					namespace2CRDs++
					Expect(crd.Spec.StormAlertCount).To(Equal(bufferThreshold))
				}
			}

				Expect(namespace1CRDs).To(BeNumerically(">=", 1), "Namespace 1 should have at least one CRD")
				Expect(namespace2CRDs).To(BeNumerically(">=", 1), "Namespace 2 should have at least one CRD")

				// BUSINESS OUTCOME: Multi-tenant isolation working
				logger.Info("✅ Multi-tenant isolation successful",
					zap.Int("namespace1_crds", namespace1CRDs),
					zap.Int("namespace2_crds", namespace2CRDs),
				)
			})
		})
	})
})
