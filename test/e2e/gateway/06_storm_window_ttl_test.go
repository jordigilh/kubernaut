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
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Test 6: Storm Window TTL Expiration (P1)", Label("e2e", "storm", "ttl", "p1"), Ordered, func() {
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
		testCtx, testCancel = context.WithTimeout(ctx, 15*time.Minute) // Longer timeout for 90s wait
		testLogger = logger.With(zap.String("test", "storm-window-ttl"))
		httpClient = &http.Client{Timeout: 30 * time.Second}

		// Use unique namespace and alert name for this test
		testNamespace = fmt.Sprintf("e2e-storm-ttl-%d", time.Now().UnixNano())
		alertName = fmt.Sprintf("HighCPU-%d", time.Now().UnixNano())

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 6: Storm Window TTL Expiration - Setup")
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

		// Get K8s client for CRD verification
		k8sClient = getKubernetesClient()

		testLogger.Info("✅ Test services ready", zap.String("namespace", testNamespace))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 6: Storm Window TTL Expiration - Cleanup")
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

			// First alert creates CRD (201), subsequent alerts during storm return 202
			if i == 1 {
				Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
					"First alert should create CRD or be accepted")
			} else {
				Expect(resp.StatusCode).To(Equal(http.StatusAccepted),
					"Subsequent alerts during storm should return 202")
			}

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

		// Step 4: Verify CRD was created
		Eventually(func() int {
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
			if err != nil {
				testLogger.Debug("Failed to list CRDs", zap.Error(err))
				return -1
			}
			return len(crdList.Items)
		}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1), "Should have at least 1 CRD")

		testLogger.Info("✅ Storm window TTL expiration test completed successfully")
	})
})

