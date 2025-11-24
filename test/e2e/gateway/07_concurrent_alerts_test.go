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

var _ = Describe("Test 7: Concurrent Alert Aggregation (P1)", Label("e2e", "storm", "concurrent", "p1"), Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		gatewayURL    string
		alertName     string
		k8sClient     client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)
		testLogger = logger.With(zap.String("test", "concurrent-alerts"))
		httpClient = &http.Client{Timeout: 30 * time.Second}

		// ✅ Generate UNIQUE namespace and alert name for test isolation
		testNamespace = GenerateUniqueNamespace("e2e-concurrent")
		alertName = GenerateUniqueAlertName("HighMemory")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 7: Concurrent Alert Aggregation - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Creating test namespace...", zap.String("namespace", testNamespace))

		// ✅ Create ONLY namespace (use shared Gateway)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		k8sClient = getKubernetesClient()
		gatewayURL = "http://localhost:8080"
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		testLogger.Info("✅ Test namespace ready", zap.String("namespace", testNamespace))
		testLogger.Info("✅ Using shared Gateway", zap.String("url", gatewayURL))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 7: Concurrent Alert Aggregation - Cleanup")
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

		// ✅ Flush Redis for test isolation
		testLogger.Info("Flushing Redis for test isolation...")
		err := CleanupRedisForTest(testNamespace)
		if err != nil {
			testLogger.Warn("Failed to flush Redis", zap.Error(err))
		}

		// ✅ Cleanup test namespace (CRDs only)
		testLogger.Info("Cleaning up test namespace...", zap.String("namespace", testNamespace))
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		_ = k8sClient.Delete(testCtx, ns)

		if testCancel != nil {
			testCancel()
		}

		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should aggregate 15 concurrent Prometheus alerts into 1 storm CRD", func() {
		// BR-GATEWAY-016: Storm aggregation
		// BUSINESS OUTCOME: 15 rapid-fire alerts → 1 aggregated CRD (97% cost reduction)
		// This validates HTTP status codes: 201 Created (before storm) → 202 Accepted (during storm)
		// This validates the complete flow: Webhook → Storm Detection → Aggregation → CRD

		// Step 1: Send 15 concurrent alerts (simulating alert storm)
		// All alerts have same namespace + alertname → should trigger storm detection
		testLogger.Info("Sending 15 concurrent alerts...")

		type alertResponse struct {
			statusCode int
			err        error
		}

		results := make(chan alertResponse, 15)
		var wg sync.WaitGroup

		for i := 1; i <= 15; i++ {
			wg.Add(1)
			go func(podNum int) {
				defer wg.Done()

				payload := fmt.Sprintf(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "payment-api-%d",
							"severity": "critical"
						},
						"annotations": {
							"summary": "High memory usage on pod payment-api-%d"
						}
					}]
				}`, alertName, testNamespace, podNum, podNum)

				req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload)))
				if err != nil {
					results <- alertResponse{statusCode: 0, err: err}
					return
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				if err != nil {
					results <- alertResponse{statusCode: 0, err: err}
					return
				}
				defer resp.Body.Close()

				results <- alertResponse{statusCode: resp.StatusCode, err: nil}
			}(i)
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(results)

		// Step 2: Validate HTTP status codes
		// First alert should return 201 (Created)
		// Subsequent alerts during storm should return 202 (Accepted)
		statusCodes := make(map[int]int)
		for result := range results {
			Expect(result.err).ToNot(HaveOccurred(), "HTTP request should succeed")
			statusCodes[result.statusCode]++
		}

		testLogger.Info("Alert responses received",
			zap.Int("201_created", statusCodes[201]),
			zap.Int("202_accepted", statusCodes[202]))

		// We expect:
		// - At least 1 alert with 201 (Created) - the first alert before storm detection
		// - Most alerts with 202 (Accepted) - during storm aggregation
		Expect(statusCodes[201]).To(BeNumerically(">=", 1), "Should have at least 1 CRD created (201)")
		Expect(statusCodes[202]).To(BeNumerically(">=", 1), "Should have at least 1 alert during storm (202)")

		// Step 3: Wait for storm aggregation to complete
		testLogger.Info("Waiting for storm aggregation to complete...")
		time.Sleep(5 * time.Second)

		// Step 4: Verify only 1 aggregated CRD was created
		// This validates that storm aggregation prevented duplicate CRDs
		Eventually(func() int {
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
			if err != nil {
				testLogger.Debug("Failed to list CRDs", zap.Error(err))
				return -1
			}
			return len(crdList.Items)
		}, 30*time.Second, 2*time.Second).Should(Equal(1), "Should have exactly 1 aggregated CRD")

		testLogger.Info("Concurrent alert aggregation validated")

		// Step 5: Verify the aggregated CRD has correct metadata
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(len(crdList.Items)).To(Equal(1))

		crd := crdList.Items[0]
		testLogger.Info("Aggregated CRD details",
			zap.String("name", crd.Name),
			zap.String("namespace", crd.Namespace),
			zap.Int("occurrenceCount", crd.Spec.Deduplication.OccurrenceCount))

		// The CRD should have aggregated multiple alerts
		Expect(crd.Spec.Deduplication.OccurrenceCount).To(BeNumerically(">=", 1),
			"Aggregated CRD should have occurrence count >= 1")

		testLogger.Info("✅ Concurrent alert aggregation test completed successfully")
	})
})

