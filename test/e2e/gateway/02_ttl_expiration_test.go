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

var _ = Describe("Test 2: TTL-Based Deduplication (P0)", Label("e2e", "deduplication", "ttl", "p0"), Ordered, func() {
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
		testLogger = logger.With(zap.String("test", "ttl-expiration"))
		httpClient = &http.Client{Timeout: 30 * time.Second}

		// Use unique namespace and alert name for this test
		testNamespace = fmt.Sprintf("e2e-ttl-test-%d", time.Now().UnixNano())
		alertName = fmt.Sprintf("HighCPU-%d", time.Now().UnixNano())

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 2: TTL-Based Deduplication - Setup")
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
		testLogger.Info("Test 2: TTL-Based Deduplication - Cleanup")
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

	It("should deduplicate based on CRD state after Redis TTL expires", func() {
		// BR-GATEWAY-008: TTL-based expiration
		// DD-GATEWAY-009: State-based deduplication
		// BUSINESS OUTCOME: Even after Redis TTL expires, Gateway checks CRD state
		// If CRD still exists and is Pending/InProgress → Increment occurrence count (202)

		// Step 1: Send first alert → Should create CRD (201 Created)
		payload1 := fmt.Sprintf(`{
			"alerts": [{
				"status": "firing",
				"labels": {
					"alertname": "%s",
					"namespace": "%s",
					"pod": "test-pod-1",
					"severity": "critical"
				},
				"annotations": {
					"summary": "High CPU usage on pod test-pod-1"
				}
			}]
		}`, alertName, testNamespace)

		req1, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload1)))
		Expect(err).ToNot(HaveOccurred())
		req1.Header.Set("Content-Type", "application/json")

		resp1, err := httpClient.Do(req1)
		Expect(err).ToNot(HaveOccurred())
		defer resp1.Body.Close()

		Expect(resp1.StatusCode).To(Equal(http.StatusCreated), "First alert should create CRD (201)")
		testLogger.Info("First alert sent successfully", zap.Int("statusCode", resp1.StatusCode))

		// Step 2: Wait for Redis TTL to expire (5s production TTL + 5s buffer)
		// Production uses 5 minutes, but E2E tests use 5 seconds for faster execution
		testLogger.Info("Waiting for Redis TTL to expire (10 seconds)...")
		time.Sleep(10 * time.Second)

		// Step 3: Send duplicate alert → Should be deduplicated (202 Accepted)
		// Even though Redis TTL expired, the CRD still exists in Kubernetes
		// State-based deduplication (DD-GATEWAY-009) checks CRD state, not just Redis
		payload2 := fmt.Sprintf(`{
			"alerts": [{
				"status": "firing",
				"labels": {
					"alertname": "%s",
					"namespace": "%s",
					"pod": "test-pod-1",
					"severity": "critical"
				},
				"annotations": {
					"summary": "High CPU usage on pod test-pod-1"
				}
			}]
		}`, alertName, testNamespace)

		req2, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer([]byte(payload2)))
		Expect(err).ToNot(HaveOccurred())
		req2.Header.Set("Content-Type", "application/json")

		resp2, err := httpClient.Do(req2)
		Expect(err).ToNot(HaveOccurred())
		defer resp2.Body.Close()

		Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate alert should be deduplicated (202)")
		testLogger.Info("Duplicate alert deduplicated successfully", zap.Int("statusCode", resp2.StatusCode))

		// Step 4: Verify only 1 CRD exists in Kubernetes
		// This confirms state-based deduplication worked correctly
		Eventually(func() int {
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
			if err != nil {
				testLogger.Debug("Failed to list CRDs", zap.Error(err))
				return -1
			}
			return len(crdList.Items)
		}, 30*time.Second, 2*time.Second).Should(Equal(1), "Should have exactly 1 CRD")

		testLogger.Info("✅ TTL expiration test completed successfully")
	})
})

