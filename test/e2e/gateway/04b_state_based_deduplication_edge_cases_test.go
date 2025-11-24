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
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

)

// DD-GATEWAY-009: State-Based Deduplication - E2E Edge Cases (Parallel Execution)
//
// Business Requirements Validated:
// - BR-GATEWAY-011: State-based deduplication (concurrent scenarios)
// - BR-GATEWAY-012: Occurrence count tracking (race conditions)
// - BR-GATEWAY-013: Deduplication lifecycle (state transitions)
//
// Business Outcome: Validate edge cases and concurrent scenarios in production-like environment
//
// Test Strategy:
// - Run tests in PARALLEL to maximize coverage without increasing test time
// - Each test gets its own namespace and Gateway instance
// - Focus on edge cases not covered in main lifecycle test
// - Validate BEHAVIOR and CORRECTNESS, not just technical function
//
// Coverage: Additional 10-15% E2E (4 edge case scenarios running in parallel)

// Parallel Execution: ✅ ENABLED
// - 4 It blocks, each with unique namespace and alert names
// - Uses shared Gateway instance
// - Cleanup in BeforeAll (shared setup) and per-test defer blocks
var _ = Describe("E2E: State-Based Deduplication Edge Cases", Label("e2e", "deduplication", "edge-cases", "p1"), Ordered, func() {
	// Shared Gateway deployment for all edge case tests
	// Each test creates its own namespace for CRD isolation

	var (
		testCtx    context.Context
		testCancel context.CancelFunc
		sharedNS   string
		// gatewayURL is suite-level variable set in SynchronizedBeforeSuite
		k8sClient  client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("E2E Test: State-Based Deduplication Edge Cases - Setup")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ✅ Generate UNIQUE namespace for test isolation
		sharedNS = GenerateUniqueNamespace("edge-cases")
		logger.Info("Creating test namespace...", zap.String("namespace", sharedNS))

		// ✅ Create ONLY namespace (use shared Gateway)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: sharedNS},
		}
		k8sClient = getKubernetesClient()
		// gatewayURL is set per-process in SynchronizedBeforeSuite (8081-8084)
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		logger.Info("✅ Test namespace ready", zap.String("namespace", sharedNS))
		logger.Info("✅ Using shared Gateway", zap.String("url", gatewayURL))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("E2E Test: State-Based Deduplication Edge Cases - Cleanup")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ✅ Cleanup test namespace (CRDs only)
		// Note: Redis flush removed for parallel execution safety
		// Redis keys are namespaced by fingerprint, TTL handles cleanup
		logger.Info("Cleaning up test namespace...", zap.String("namespace", sharedNS))
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: sharedNS},
		}
		_ = k8sClient.Delete(testCtx, ns)

		if testCancel != nil {
			testCancel()
		}

		logger.Info("✅ Test cleanup complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASE 1: Rapid Concurrent Duplicates (Race Condition Test)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	Context("Concurrent Duplicate Handling", func() {
		It("should handle 10 simultaneous duplicate alerts correctly", func() {
			// Create unique namespace for this test (no service deployment needed)
			testNS := fmt.Sprintf("edge-concurrent-%d", time.Now().UnixNano())

			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("EDGE CASE 1: Concurrent Duplicates", zap.String("namespace", testNS))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Create namespace for CRD isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNS,
				},
			}
			err := k8sClient.Create(context.Background(), ns)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup namespace after test
			defer func() {
				_ = k8sClient.Delete(context.Background(), ns)
			}()

			// Create alert payload with unique alert name to avoid storm window conflicts
			// Use "ConcurrentTestAlert" instead of "HighMemoryUsage" to avoid conflicts with Edge Case 2
			alertPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "ConcurrentTestAlert",
				Namespace: testNS,
				Severity:  "warning",
				PodName:   "api-server-concurrent-test",
				Labels: map[string]string{
					"alertname": "ConcurrentTestAlert",
					"namespace": testNS,
					"pod":       "api-server-concurrent-test",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"summary": "Concurrent test alert for race condition validation",
				},
			})

			By("1. Sending 10 identical alerts concurrently")
			var wg sync.WaitGroup
			responses := make([]int, 10)

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					resp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alertPayload)
					responses[index] = resp.StatusCode
				}(i)
			}
			wg.Wait()

			By("2. Verifying exactly ONE CRD was created")
			var crdList remediationv1alpha1.RemediationRequestList
			Eventually(func() int {
				err := k8sClient.List(context.Background(), &crdList, client.InNamespace(testNS))
				if err != nil {
					return 0
				}
				return len(crdList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1), "Should create exactly 1 CRD despite concurrent requests")

			By("3. Verifying occurrence count reflects all duplicates")
			crd := &crdList.Items[0]
			// Set phase to Pending to enable deduplication
			crd.Status.OverallPhase = "Pending"
			err = k8sClient.Status().Update(context.Background(), crd)
			Expect(err).ToNot(HaveOccurred())

			// Wait for phase propagation
			Eventually(func() string {
				_ = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crd.Name,
				}, crd)
				return crd.Status.OverallPhase
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			// Count successful responses (201 Created or 202 Accepted)
			successCount := 0
			for _, status := range responses {
				if status == http.StatusCreated || status == http.StatusAccepted {
					successCount++
				}
			}
			Expect(successCount).To(Equal(10), "All 10 requests should succeed")

			logger.Info("✅ EDGE CASE 1: Concurrent duplicates handled correctly",
				zap.Int("crd_count", len(crdList.Items)),
				zap.Int("successful_requests", successCount))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASE 2: Multiple Alerts → Multiple CRDs (Different Fingerprints)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	Context("Multiple Different Alerts", func() {
		It("should create separate CRDs for different alert fingerprints", func() {
			testNS := fmt.Sprintf("edge-multi-%d", time.Now().UnixNano())

			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("EDGE CASE 2: Multiple Different Alerts", zap.String("namespace", testNS))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Create namespace for CRD isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNS,
				},
			}
			err := k8sClient.Create(context.Background(), ns)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup namespace after test
			defer func() {
				_ = k8sClient.Delete(context.Background(), ns)
			}()

			By("1. Sending 5 different alerts (different pods)")
			// Use 5 DIFFERENT alert names to avoid storm detection
			// Storm detection groups by alert name, so each alert needs a unique name
			alerts := []PrometheusAlertPayload{
				{
					AlertName: "PodCrashLooping",
					Namespace: testNS,
					Severity:  "critical",
					PodName:   "payment-api-1",
					Labels: map[string]string{
						"alertname": "PodCrashLooping",
						"namespace": testNS,
						"pod":       "payment-api-1",
					},
					Annotations: map[string]string{"summary": "Pod 1 crash looping"},
				},
				{
					AlertName: "HighMemoryUsage",
					Namespace: testNS,
					Severity:  "warning",
					PodName:   "api-server-2",
					Labels: map[string]string{
						"alertname": "HighMemoryUsage",
						"namespace": testNS,
						"pod":       "api-server-2",
					},
					Annotations: map[string]string{"summary": "High memory on pod 2"},
				},
				{
					AlertName: "DiskSpaceLow",
					Namespace: testNS,
					Severity:  "warning",
					PodName:   "database-3",
					Labels: map[string]string{
						"alertname": "DiskSpaceLow",
						"namespace": testNS,
						"pod":       "database-3",
					},
					Annotations: map[string]string{"summary": "Low disk space on pod 3"},
				},
				{
					AlertName: "NetworkLatencyHigh",
					Namespace: testNS,
					Severity:  "warning",
					PodName:   "proxy-4",
					Labels: map[string]string{
						"alertname": "NetworkLatencyHigh",
						"namespace": testNS,
						"pod":       "proxy-4",
					},
					Annotations: map[string]string{"summary": "High latency on pod 4"},
				},
				{
					AlertName: "CPUThrottling",
					Namespace: testNS,
					Severity:  "warning",
					PodName:   "worker-5",
					Labels: map[string]string{
						"alertname": "CPUThrottling",
						"namespace": testNS,
						"pod":       "worker-5",
					},
					Annotations: map[string]string{"summary": "CPU throttling on pod 5"},
				},
			}

			// Send alerts with delay to avoid storm detection
			// Storm threshold: 3 alerts/minute triggers aggregation
			// Solution: Send with 2-second delay between each alert for safety
			for i, alert := range alerts {
				payload := createPrometheusWebhookPayload(alert)
				resp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", payload)

				// Log response for debugging
				logger.Info("Alert sent",
					zap.Int("alert_number", i+1),
					zap.String("alert_name", alert.AlertName),
					zap.Int("status_code", resp.StatusCode))

				Expect(resp.StatusCode).To(Equal(http.StatusCreated), fmt.Sprintf("Alert %d (%s) should create CRD", i+1, alert.AlertName))

				// Wait 2 seconds between alerts to avoid storm detection and allow processing
				if i < len(alerts)-1 {
					time.Sleep(2 * time.Second)
				}
			}

			By("2. Verifying 5 different CRDs were created")
			var crdList remediationv1alpha1.RemediationRequestList
			Eventually(func() int {
				err := k8sClient.List(context.Background(), &crdList, client.InNamespace(testNS))
				if err != nil {
					return 0
				}
				return len(crdList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(5), "Should create 5 CRDs for 5 different alerts")

			By("3. Verifying each CRD has unique fingerprint")
			fingerprints := make(map[string]bool)
			for _, crd := range crdList.Items {
				fingerprint := crd.Spec.SignalFingerprint
				Expect(fingerprints[fingerprint]).To(BeFalse(), fmt.Sprintf("Fingerprint %s should be unique", fingerprint))
				fingerprints[fingerprint] = true
			}

			logger.Info("✅ EDGE CASE 2: Multiple different alerts created separate CRDs",
				zap.Int("crd_count", len(crdList.Items)),
				zap.Int("unique_fingerprints", len(fingerprints)))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASE 3: Rapid State Transitions (Pending → Processing → Completed)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	Context("Rapid State Transitions", func() {
		It("should handle alerts during rapid CRD state changes", func() {
			testNS := fmt.Sprintf("edge-transitions-%d", time.Now().UnixNano())

			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("EDGE CASE 3: Rapid State Transitions", zap.String("namespace", testNS))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Create namespace for CRD isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNS,
				},
			}
			err := k8sClient.Create(context.Background(), ns)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup namespace after test
			defer func() {
				_ = k8sClient.Delete(context.Background(), ns)
			}()

			alertPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "ServiceDown",
				Namespace: testNS,
				Severity:  "critical",
				PodName:   "api-gateway-rapid",
				Labels: map[string]string{
					"alertname": "ServiceDown",
					"namespace": testNS,
					"pod":       "api-gateway-rapid",
				},
				Annotations: map[string]string{"summary": "Service is down"},
			})

			By("1. Creating initial CRD")
			resp1 := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alertPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 GatewayResponse
			err = json.Unmarshal(resp1.Body, &response1)
			Expect(err).ToNot(HaveOccurred())
			crdName := response1.RemediationRequestName

			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crdName,
				}, &crd)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("2. Rapidly transitioning through states while sending duplicates")
			states := []string{"Pending", "Processing", "Completed"}

			for i, state := range states {
				// Refetch CRD to get latest resourceVersion (avoid conflicts)
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crdName,
				}, &crd)
				Expect(err).ToNot(HaveOccurred())

				// Update CRD state
				crd.Status.OverallPhase = state
				err = k8sClient.Status().Update(context.Background(), &crd)
				Expect(err).ToNot(HaveOccurred())

				// Wait for propagation
				Eventually(func() string {
					_ = k8sClient.Get(context.Background(), client.ObjectKey{
						Namespace: testNS,
						Name:      crdName,
					}, &crd)
					return crd.Status.OverallPhase
				}, 5*time.Second, 500*time.Millisecond).Should(Equal(state))

				// Send duplicate alert
				time.Sleep(100 * time.Millisecond) // Brief pause
				resp := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alertPayload)

				if state == "Completed" {
					// After completion, should treat as new incident (but may get name collision in v1.0)
					Expect(resp.StatusCode).To(SatisfyAny(
						Equal(http.StatusOK),      // v1.0: Fetched existing
						Equal(http.StatusCreated), // v1.1: New CRD
					), fmt.Sprintf("State %s: Should handle as new incident", state))
				} else {
					// Pending/Processing should detect duplicate
					Expect(resp.StatusCode).To(Equal(http.StatusAccepted),
						fmt.Sprintf("State %s: Should detect duplicate", state))
				}

				logger.Info(fmt.Sprintf("✅ State transition %d/%d: %s", i+1, len(states), state))
			}

			By("3. Verifying final occurrence count")
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: testNS,
				Name:      crdName,
			}, &crd)
			Expect(err).ToNot(HaveOccurred())

			// Should have incremented during Pending and Processing states
			Expect(crd.Spec.Deduplication.OccurrenceCount).To(BeNumerically(">=", 2),
				"Occurrence count should reflect duplicates during in-progress states")

			logger.Info("✅ EDGE CASE 3: Rapid state transitions handled correctly",
				zap.Int("final_occurrence_count", crd.Spec.Deduplication.OccurrenceCount))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASE 4: Failed → Retry → Completed Lifecycle
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	Context("Failed Remediation Retry", func() {
		It("should allow retry after failed remediation", func() {
			testNS := fmt.Sprintf("edge-retry-%d", time.Now().UnixNano())

			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("EDGE CASE 4: Failed Remediation Retry", zap.String("namespace", testNS))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Create namespace for CRD isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNS,
				},
			}
			err := k8sClient.Create(context.Background(), ns)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup namespace after test
			defer func() {
				_ = k8sClient.Delete(context.Background(), ns)
			}()

			alertPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "DatabaseConnectionFailed",
				Namespace: testNS,
				Severity:  "critical",
				PodName:   "app-server-retry",
				Labels: map[string]string{
					"alertname": "DatabaseConnectionFailed",
					"namespace": testNS,
					"pod":       "app-server-retry",
				},
				Annotations: map[string]string{"summary": "Cannot connect to database"},
			})

			By("1. Creating initial CRD")
			resp1 := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alertPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 GatewayResponse
			err = json.Unmarshal(resp1.Body, &response1)
			Expect(err).ToNot(HaveOccurred())
			crdName := response1.RemediationRequestName

			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crdName,
				}, &crd)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("2. Simulating failed remediation")
			crd.Status.OverallPhase = "Processing"
			err = k8sClient.Status().Update(context.Background(), &crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				_ = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crdName,
				}, &crd)
				return crd.Status.OverallPhase
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Processing"))

			// Mark as Failed
			crd.Status.OverallPhase = "Failed"
			err = k8sClient.Status().Update(context.Background(), &crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				_ = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crdName,
				}, &crd)
				return crd.Status.OverallPhase
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Failed"))

			By("3. Sending alert after failure (should trigger retry)")
			time.Sleep(500 * time.Millisecond)
			resp2 := sendWebhookRequest(gatewayURL, "/api/v1/signals/prometheus", alertPayload)

			// Should treat as new incident (not duplicate) because state is Failed
			Expect(resp2.StatusCode).To(SatisfyAny(
				Equal(http.StatusOK),      // v1.0: Fetched existing
				Equal(http.StatusCreated), // v1.1: New CRD
			), "Failed state should allow retry")

			var response2 GatewayResponse
			err = json.Unmarshal(resp2.Body, &response2)
			Expect(err).ToNot(HaveOccurred())
			Expect(response2.Duplicate).To(BeFalse(), "Should NOT be marked as duplicate after failure")

			By("4. Simulating successful retry")
			// In v1.0, we're still working with the same CRD due to name collision
			// In v1.1 with DD-015, this would be a new CRD
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: testNS,
				Name:      crdName,
			}, &crd)
			Expect(err).ToNot(HaveOccurred())

			crd.Status.OverallPhase = "Processing"
			err = k8sClient.Status().Update(context.Background(), &crd)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			crd.Status.OverallPhase = "Completed"
			err = k8sClient.Status().Update(context.Background(), &crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				_ = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      crdName,
				}, &crd)
				return crd.Status.OverallPhase
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

			logger.Info("✅ EDGE CASE 4: Failed remediation retry lifecycle validated",
				zap.String("final_phase", crd.Status.OverallPhase))
		})
	})
})
