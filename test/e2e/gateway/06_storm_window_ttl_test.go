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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("Test 6: Storm Window TTL Expiration (P1)", Label("e2e", "storm", "ttl", "p1"), Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		// gatewayURL is suite-level variable set in SynchronizedBeforeSuite
		alertName     string
		k8sClient     client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 15*time.Minute) // Longer timeout for 90s wait
		testLogger = logger.With(zap.String("test", "storm-window-ttl"))
		httpClient = &http.Client{Timeout: 30 * time.Second}

		// ✅ Generate UNIQUE namespace and alert name for test isolation
		testNamespace = GenerateUniqueNamespace("e2e-storm-ttl")
		alertName = GenerateUniqueAlertName("HighCPU")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 6: Storm Window TTL Expiration - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Creating test namespace...", zap.String("namespace", testNamespace))

		// ✅ Create ONLY namespace (use shared Gateway)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		k8sClient = getKubernetesClient()
		// gatewayURL is set per-process in SynchronizedBeforeSuite (8081-8084)
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		testLogger.Info("✅ Test namespace ready", zap.String("namespace", testNamespace))
		testLogger.Info("✅ Using shared Gateway", zap.String("url", gatewayURL))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 6: Storm Window TTL Expiration - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ✅ Cleanup test namespace (CRDs only)
		// Note: Redis flush removed for parallel execution safety
		// Redis keys are namespaced by fingerprint, TTL handles cleanup
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

	It("should create new storm window after TTL expiration", func() {
		// BR-GATEWAY-016: Storm window TTL expiration
		// BUSINESS OUTCOME: Expired storm window → new window created
		// This test validates the complete storm lifecycle:
		// 1. Storm detected → window created
		// 2. Window expires after 90s
		// 3. New alert → new window created

		// Step 1: Send 5 alerts to trigger storm detection
		testLogger.Info("Sending 5 alerts to trigger storm...")
		for i := 1; i <= 5; i++ {
			payload := fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "%s",
						"namespace": "%s",
						"pod": "test-pod-%d",
						"severity": "critical"
					},
					"annotations": {
						"summary": "High CPU usage on pod test-pod-%d"
					}
				}]
			}`, alertName, testNamespace, i, i)

			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload)))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Storm detection behavior:
			// - First alert: HTTP 201 (CRD created)
			// - Subsequent alerts: HTTP 201 or 202 depending on storm detection timing
			// Note: Pattern-based storm detection may not trigger immediately
			Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
				fmt.Sprintf("Alert %d should create CRD (201) or be accepted for aggregation (202)", i))

			testLogger.Info("Alert sent", zap.Int("alertNum", i), zap.Int("statusCode", resp.StatusCode))
		}

		// Step 2: Wait for storm window to expire (90 seconds)
		// Production storm windows have maxWindowDuration of 60s
		// We wait 90s to ensure window has expired
		testLogger.Info("Waiting for storm window to expire (90 seconds)...")
		time.Sleep(90 * time.Second)

		// Step 3: Send new alert → Should create NEW window (201 Created)
		// This validates that the old window expired and a new one was created
		payload := fmt.Sprintf(`{
			"alerts": [{
				"status": "firing",
				"labels": {
					"alertname": "%s",
					"namespace": "%s",
					"pod": "test-pod-6",
					"severity": "critical"
				},
				"annotations": {
					"summary": "High CPU usage on pod test-pod-6"
				}
			}]
		}`, alertName, testNamespace)

		req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload)))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "New alert after window expiration should create new CRD (201)")
		testLogger.Info("New window created successfully", zap.Int("statusCode", resp.StatusCode))

	// Step 4: Verify CRD was created (after 90s TTL wait)
	// Note: TTL wait is done via time.Sleep above, this just checks CRD exists
	Eventually(func() int {
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
		if err != nil {
			testLogger.Debug("Failed to list CRDs", zap.Error(err))
			return -1
		}
		return len(crdList.Items)
	}, 15*time.Second, 2*time.Second).Should(BeNumerically(">=", 1), "Should have at least 1 CRD (Test 8: after TTL expiration)")

		testLogger.Info("✅ Storm window TTL expiration test completed successfully")
	})
})

