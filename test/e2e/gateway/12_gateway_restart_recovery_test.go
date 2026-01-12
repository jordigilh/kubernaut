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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

var _ = Describe("Test 12: Gateway Restart Recovery (BR-GATEWAY-010, BR-GATEWAY-092)", Serial, Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
		k8sClient     client.Client
	)

	BeforeAll(func() {
		Skip("TODO: Move to test/integration/gateway/ - Tests component resilience, not end-to-end workflow (DD-TEST-002)")
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute) // Longer timeout for restart test
		testLogger = logger.WithValues("test", "gateway-restart")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 12: Gateway Restart Recovery - Setup")
		testLogger.Info("Note: This test is Serial (not parallel) as it restarts the Gateway")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = GenerateUniqueNamespace("restart")
		testLogger.Info("Deploying test services...", "namespace", testNamespace)

		k8sClient = getKubernetesClient()
		Expect(CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)).To(Succeed(), "Failed to create and wait for namespace")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 12: Gateway Restart Recovery - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl logs -n %s deployment/gateway -f", testNamespace))
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			if testCancel != nil {
				testCancel()
			}
			return
		}

		testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should recover and continue processing alerts after Gateway pod restart", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send alerts, restart Gateway, verify recovery")
		testLogger.Info("Expected: Gateway recovers and continues processing")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		processID := GinkgoParallelProcess()
		alertName := fmt.Sprintf("RestartTest-p%d-%s", processID, uuid.New().String()[:8])

		testLogger.Info("Step 1: Send alerts before restart")
		const preRestartAlerts = 3

		alertPayload := map[string]interface{}{
			"status": "firing",
			"labels": map[string]interface{}{
				"alertname": alertName,
				"severity":  "critical",
				"namespace": testNamespace,
				"pod":       "restart-test-pod",
			},
			"annotations": map[string]interface{}{
				"summary": "Gateway restart recovery test",
			},
			"startsAt": time.Now().Format(time.RFC3339),
		}

		webhookPayload := map[string]interface{}{
			"alerts": []interface{}{alertPayload},
		}
		payloadBytes, _ := json.Marshal(webhookPayload)

		for i := 0; i < preRestartAlerts; i++ {
			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req17, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
					if err != nil {
						return nil, err
					}
					req17.Header.Set("Content-Type", "application/json")
					req17.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req17)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Pre-restart alert %d should be accepted", i+1))

			testLogger.Info(fmt.Sprintf("  Sent pre-restart alert %d/%d", i+1, preRestartAlerts))
			// Stagger requests to avoid overwhelming Gateway (50ms is sufficient for E2E)
			time.Sleep(50 * time.Millisecond)
		}

		testLogger.Info("✅ Pre-restart alerts sent successfully")

		testLogger.Info("Step 2: Restart Gateway pod")

		// Delete the Gateway pod to trigger restart
		podList := &corev1.PodList{}
		err := k8sClient.List(testCtx, podList, client.InNamespace(gatewayNamespace), client.MatchingLabels{"app": "gateway"})
		Expect(err).ToNot(HaveOccurred(), "Should list Gateway pods")

		if len(podList.Items) > 0 {
			pod := &podList.Items[0]
			testLogger.Info("Deleting Gateway pod to trigger restart", "pod", pod.Name)
			err = k8sClient.Delete(testCtx, pod)
			Expect(err).ToNot(HaveOccurred(), "Should delete Gateway pod")
		} else {
			testLogger.Info("No Gateway pods found - skipping restart")
		}

		testLogger.Info("Step 3: Wait for Gateway to recover")
		Eventually(func() error {
			resp, err := httpClient.Get(gatewayURL + "/health")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "Gateway should recover and pass health check")

		testLogger.Info("✅ Gateway recovered and healthy")

		testLogger.Info("Step 4: Send alerts after restart")
		const postRestartAlerts = 3
		postRestartAlertName := fmt.Sprintf("PostRestart-p%d-%s", processID, uuid.New().String()[:8])

		postRestartPayload := map[string]interface{}{
			"status": "firing",
			"labels": map[string]interface{}{
				"alertname": postRestartAlertName,
				"severity":  "warning",
				"namespace": testNamespace,
				"pod":       "post-restart-pod",
			},
			"annotations": map[string]interface{}{
				"summary": "Post-restart test alert",
			},
			"startsAt": time.Now().Format(time.RFC3339),
		}

		postWebhook := map[string]interface{}{
			"alerts": []interface{}{postRestartPayload},
		}
		postPayloadBytes, _ := json.Marshal(postWebhook)

		for i := 0; i < postRestartAlerts; i++ {
			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req18, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(postPayloadBytes))
					if err != nil {
						return nil, err
					}
					req18.Header.Set("Content-Type", "application/json")
					req18.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req18)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Post-restart alert %d should be accepted", i+1))

			testLogger.Info(fmt.Sprintf("  Sent post-restart alert %d/%d", i+1, postRestartAlerts))
			// Stagger requests to avoid overwhelming Gateway (50ms is sufficient for E2E)
			time.Sleep(50 * time.Millisecond)
		}

		testLogger.Info("✅ Post-restart alerts sent successfully")

		testLogger.Info("Step 5: Verify CRD creation after restart")
		var crdList remediationv1alpha1.RemediationRequestList
		Eventually(func() int {
			k8sClient := getKubernetesClientSafe()
			if k8sClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.V(1).Info("Failed to get K8s client", "error", err)
				} else {
					testLogger.V(1).Info("Failed to get K8s client (unknown error)")
				}
				return -1
			}
			if err := k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace)); err != nil {
				testLogger.V(1).Info("Failed to list CRDs", "error", err)
				return -1
			}
			return len(crdList.Items)
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"At least one CRD should be created after Gateway restart (BR-GATEWAY-010)")

		testLogger.Info("✅ CRDs created after restart",
			"count", len(crdList.Items))

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 12 PASSED: Gateway Restart Recovery")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
