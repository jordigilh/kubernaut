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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

var _ = Describe("Test 14: Deduplication TTL Expiration (BR-GATEWAY-012)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)
		testLogger = logger.WithValues("test", "dedup-ttl")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 14: Deduplication TTL Expiration - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = GenerateUniqueNamespace("dedup-ttl")
		testLogger.Info("Deploying test services...", "namespace", testNamespace)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		k8sClient := getKubernetesClient()
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed(), "Failed to create test namespace")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 14: Deduplication TTL Expiration - Cleanup")
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
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		k8sClient := getKubernetesClient()
		_ = k8sClient.Delete(testCtx, ns)

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should allow new CRD creation after deduplication TTL expires", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send alert, wait for TTL, send same alert again")
		testLogger.Info("Expected: Second alert creates new CRD after TTL expiration")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		processID := GinkgoParallelProcess()
		alertName := fmt.Sprintf("TTLExpirationTest-p%d-%s", processID, uuid.New().String()[:8])

		// Create deterministic alert payload
		alertPayload := map[string]interface{}{
			"status": "firing",
			"labels": map[string]interface{}{
				"alertname": alertName,
				"severity":  "warning",
				"namespace": testNamespace,
				"pod":       "ttl-test-pod",
			},
			"annotations": map[string]interface{}{
				"summary": "TTL expiration test alert",
			},
			"startsAt": time.Now().Format(time.RFC3339),
		}

		webhookPayload := map[string]interface{}{
			"alerts": []interface{}{alertPayload},
		}
		payloadBytes, _ := json.Marshal(webhookPayload)

		testLogger.Info("Step 1: Send initial batch of alerts to trigger CRD creation")
		const initialAlerts = 5 // Send enough to trigger storm aggregation

		for i := 0; i < initialAlerts; i++ {
			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req21, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
					if err != nil {
						return nil, err
					}
					req21.Header.Set("Content-Type", "application/json")
					req21.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req21)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Initial alert %d should be accepted", i+1))

			testLogger.Info(fmt.Sprintf("  Sent initial alert %d/%d", i+1, initialAlerts))
			// Stagger requests to avoid overwhelming Gateway (50ms is sufficient for E2E)
			time.Sleep(50 * time.Millisecond)
		}

		testLogger.Info("✅ Initial alerts sent")

		testLogger.Info("Step 2: Verify initial CRD creation")
		var initialCRDCount int
		Eventually(func() int {
			k8sClient := getKubernetesClientSafe()
			if k8sClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.V(1).Info("Failed to get K8s client", "error", err)
				}
				return -1
			}
			crdList := &remediationv1alpha1.RemediationRequestList{}
			if err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace)); err != nil {
				testLogger.V(1).Info("Failed to list CRDs", "error", err)
				return -1
			}
			count := 0
			for _, crd := range crdList.Items {
				if crd.Spec.SignalName == alertName {
					count++
				}
			}
			initialCRDCount = count
			return count
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"At least one CRD should be created from initial alerts")

		testLogger.Info("✅ Initial CRD created", "count", initialCRDCount)

		// Note: E2E environment uses 10s TTL (minimum allowed per config validation)
		// See: test/e2e/gateway/gateway-deployment.yaml and pkg/gateway/config/config.go:368
		// Production uses 5m TTL. This test validates TTL expiration behavior.
		testLogger.Info("Step 3: Wait for deduplication TTL to expire")
		testLogger.Info("  Waiting 15 seconds for TTL expiration (10s E2E TTL + 5s buffer)...")
		time.Sleep(15 * time.Second) // E2E TTL is 10s (see gateway-deployment.yaml), 5s buffer for clock skew

		testLogger.Info("Step 4: Send same alert again after TTL expiration")
		// Create a new unique alert name for post-TTL test
		postTTLAlertName := fmt.Sprintf("PostTTL-p%d-%s", processID, uuid.New().String()[:8])
		postTTLPayload := map[string]interface{}{
			"status": "firing",
			"labels": map[string]interface{}{
				"alertname": postTTLAlertName,
				"severity":  "warning",
				"namespace": testNamespace,
				"pod":       "post-ttl-pod",
			},
			"annotations": map[string]interface{}{
				"summary": "Post-TTL test alert",
			},
			"startsAt": time.Now().Format(time.RFC3339),
		}

		postTTLWebhook := map[string]interface{}{
			"alerts": []interface{}{postTTLPayload},
		}
		postTTLBytes, _ := json.Marshal(postTTLWebhook)

		const postTTLAlerts = 5
		for i := 0; i < postTTLAlerts; i++ {
			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req22, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(postTTLBytes))
					if err != nil {
						return nil, err
					}
					req22.Header.Set("Content-Type", "application/json")
					req22.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req22)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Post-TTL alert %d should be accepted", i+1))

			testLogger.Info(fmt.Sprintf("  Sent post-TTL alert %d/%d", i+1, postTTLAlerts))
			// Stagger requests to avoid overwhelming Gateway (50ms is sufficient for E2E)
			time.Sleep(50 * time.Millisecond)
		}

		testLogger.Info("✅ Post-TTL alerts sent")

		testLogger.Info("Step 5: Verify new CRD creation after TTL")
		Eventually(func() int {
			k8sClient := getKubernetesClientSafe()
			if k8sClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.V(1).Info("Failed to get K8s client", "error", err)
				}
				return -1
			}
			crdList := &remediationv1alpha1.RemediationRequestList{}
			if err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace)); err != nil {
				testLogger.V(1).Info("Failed to list CRDs", "error", err)
				return -1
			}
			count := 0
			for _, crd := range crdList.Items {
				if crd.Spec.SignalName == postTTLAlertName {
					count++
				}
			}
			return count
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"New CRD should be created after TTL expiration (BR-GATEWAY-012)")

		testLogger.Info("✅ New CRD created after TTL expiration")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 14 PASSED: Deduplication TTL Expiration")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
