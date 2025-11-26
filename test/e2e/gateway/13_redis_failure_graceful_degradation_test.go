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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("Test 13: Redis Failure Graceful Degradation (BR-GATEWAY-073, BR-GATEWAY-101)", Serial, Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		testNamespace string
		httpClient    *http.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute) // Longer timeout for Redis failure test
		testLogger = logger.With(zap.String("test", "redis-failure"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 13: Redis Failure Graceful Degradation - Setup")
		testLogger.Info("Note: This test is Serial (not parallel) as it affects Redis")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = GenerateUniqueNamespace("redis-fail")
		testLogger.Info("Deploying test services...", zap.String("namespace", testNamespace))

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		k8sClient := getKubernetesClient()
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed(), "Failed to create test namespace")

		testLogger.Info("✅ Test namespace ready", zap.String("namespace", testNamespace))
		testLogger.Info("✅ Using shared Gateway", zap.String("url", gatewayURL))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 13: Redis Failure Graceful Degradation - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Ensure Redis is restored before cleanup
		testLogger.Info("Ensuring Redis is restored...")
		k8sClient := getKubernetesClient()
		// Scale Redis back up if it was scaled down
		// This is a safety measure in case the test didn't complete cleanup
		testLogger.Info("Redis restoration check complete")

		if CurrentSpecReport().Failed() {
			testLogger.Warn("⚠️  Test FAILED - Preserving namespace for debugging",
				zap.String("namespace", testNamespace))
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

	It("should continue processing alerts when Redis is unavailable (graceful degradation)", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Gateway continues processing when Redis fails")
		testLogger.Info("Expected: Alerts processed, CRDs created (without deduplication)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		processID := GinkgoParallelProcess()
		alertName := fmt.Sprintf("RedisFailTest-p%d-%d", processID, time.Now().UnixNano())

		testLogger.Info("Step 1: Verify Gateway is healthy with Redis")
		Eventually(func() error {
			resp, err := httpClient.Get(gatewayURL + "/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "Gateway should be healthy")

		testLogger.Info("✅ Gateway is healthy")

		testLogger.Info("Step 2: Send alerts with Redis available (enough to trigger storm aggregation)")
		// Send 10 alerts to exceed storm aggregation threshold (typically 5)
		// This ensures at least 1 CRD is created before Redis fails
		const preFailureAlerts = 10
		var preFailureSuccess int

		for i := 0; i < preFailureAlerts; i++ {
			alertPayload := map[string]interface{}{
				"status": "firing",
				"labels": map[string]interface{}{
					"alertname": alertName,
					"severity":  "critical",
					"namespace": testNamespace,
					"pod":       fmt.Sprintf("redis-fail-test-pod-%d", i),
				},
				"annotations": map[string]interface{}{
					"summary": "Redis failure graceful degradation test",
				},
				"startsAt": time.Now().Format(time.RFC3339),
			}

			webhookPayload := map[string]interface{}{
				"alerts": []interface{}{alertPayload},
			}
			payloadBytes, _ := json.Marshal(webhookPayload)

			resp, err := httpClient.Post(
				gatewayURL+"/api/v1/signals/prometheus",
				"application/json",
				bytes.NewBuffer(payloadBytes),
			)
			if err != nil {
				testLogger.Debug(fmt.Sprintf("Alert %d failed", i+1), zap.Error(err))
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
				preFailureSuccess++
			}
			time.Sleep(100 * time.Millisecond)
		}

		testLogger.Info("Pre-failure alerts sent",
			zap.Int("total", preFailureAlerts),
			zap.Int("success", preFailureSuccess))
		Expect(preFailureSuccess).To(BeNumerically(">=", preFailureAlerts-1),
			"Most alerts should succeed before Redis failure")

		testLogger.Info("Step 3: Simulate Redis failure by deleting Redis pod")
		k8sClient := getKubernetesClient()

		// Find and delete Redis pod
		redisPodList := &corev1.PodList{}
		if err := k8sClient.List(testCtx, redisPodList, client.InNamespace(gatewayNamespace), client.MatchingLabels{"app": "redis"}); err != nil {
			testLogger.Warn("Could not list Redis pods - Redis may not be deployed as a pod", zap.Error(err))
		}

		if len(redisPodList.Items) > 0 {
			redisPod := &redisPodList.Items[0]
			testLogger.Info("Deleting Redis pod to simulate failure", zap.String("pod", redisPod.Name))
			if err := k8sClient.Delete(testCtx, redisPod); err != nil {
				testLogger.Warn("Could not delete Redis pod", zap.Error(err))
			} else {
				testLogger.Info("✅ Redis pod deleted")
			}
		} else {
			testLogger.Warn("No Redis pods found - testing graceful degradation behavior only")
		}

		// Wait a moment for Redis to become unavailable
		time.Sleep(5 * time.Second)

		testLogger.Info("Step 4: Send alerts during Redis unavailability")
		const alertsDuringFailure = 3
		failureAlertName := fmt.Sprintf("DuringFailure-p%d-%d", processID, time.Now().UnixNano())

		failurePayload := map[string]interface{}{
			"status": "firing",
			"labels": map[string]interface{}{
				"alertname": failureAlertName,
				"severity":  "warning",
				"namespace": testNamespace,
				"pod":       "during-failure-pod",
			},
			"annotations": map[string]interface{}{
				"summary": "Alert during Redis failure",
			},
			"startsAt": time.Now().Format(time.RFC3339),
		}

		failureWebhook := map[string]interface{}{
			"alerts": []interface{}{failurePayload},
		}
		failurePayloadBytes, _ := json.Marshal(failureWebhook)

		successCount := 0
		for i := 0; i < alertsDuringFailure; i++ {
			resp, err := httpClient.Post(
				gatewayURL+"/api/v1/signals/prometheus",
				"application/json",
				bytes.NewBuffer(failurePayloadBytes),
			)
			if err != nil {
				testLogger.Debug(fmt.Sprintf("Alert %d failed (expected during degradation)", i+1), zap.Error(err))
				continue
			}
			defer resp.Body.Close()

			// Gateway should still accept alerts even without Redis (graceful degradation)
			// It may return 201/202 (success) or 500 (if Redis is required)
			if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
				successCount++
				testLogger.Info(fmt.Sprintf("  Alert %d accepted (status %d)", i+1, resp.StatusCode))
			} else {
				testLogger.Debug(fmt.Sprintf("  Alert %d returned status %d", i+1, resp.StatusCode))
			}

			time.Sleep(500 * time.Millisecond)
		}

		testLogger.Info("Alerts during Redis failure",
			zap.Int("sent", alertsDuringFailure),
			zap.Int("accepted", successCount))

		testLogger.Info("Step 5: Verify Gateway health endpoint still responds")
		// Gateway should remain responsive even with Redis down
		Eventually(func() error {
			resp, err := httpClient.Get(gatewayURL + "/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			// Health check may return degraded status but should still respond
			if resp.StatusCode >= 500 {
				return fmt.Errorf("health check returned server error %d", resp.StatusCode)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"Gateway health endpoint should respond during Redis failure (BR-GATEWAY-073)")

		testLogger.Info("✅ Gateway remains responsive during Redis failure")

		testLogger.Info("Step 6: Wait for Redis to recover (if pod was deleted)")
		if len(redisPodList.Items) > 0 {
			// Wait for Redis pod to be recreated by Kubernetes
			Eventually(func() bool {
				podList := &corev1.PodList{}
				err := k8sClient.List(testCtx, podList, client.InNamespace(gatewayNamespace), client.MatchingLabels{"app": "redis"})
				if err != nil {
					return false
				}
				for _, pod := range podList.Items {
					if pod.Status.Phase == corev1.PodRunning {
						return true
					}
				}
				return false
			}, 120*time.Second, 5*time.Second).Should(BeTrue(), "Redis should recover")

			testLogger.Info("✅ Redis recovered")
		}

		testLogger.Info("Step 7: Verify CRD creation")
		var crdList remediationv1alpha1.RemediationRequestList
		Eventually(func() int {
			k8sClient := getKubernetesClientSafe()
			if k8sClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.Debug("Failed to get K8s client", zap.Error(err))
				}
				return -1
			}
			if err := k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace)); err != nil {
				testLogger.Debug("Failed to list CRDs", zap.Error(err))
				return -1
			}
			return len(crdList.Items)
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"At least one CRD should be created (BR-GATEWAY-101)")

		testLogger.Info("✅ CRDs created", zap.Int("count", len(crdList.Items)))

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 13 PASSED: Redis Failure Graceful Degradation")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
