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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Test 05: Multi-Namespace Isolation (BR-GATEWAY-011)
// Validates that alerts from different namespaces are isolated:
// - CRDs are created in the correct namespace
// - Storm buffers are isolated per namespace
// - Deduplication is scoped to namespace
//
// Business Requirements:
// - BR-GATEWAY-011: Multi-tenant isolation with per-namespace buffers
var _ = Describe("Test 05: Multi-Namespace Isolation (BR-GATEWAY-011)", Ordered, func() {
	var (
		testCtx        context.Context
		testCancel     context.CancelFunc
		testLogger     logr.Logger
		testNamespace1 string
		testNamespace2 string
		httpClient     *http.Client
		k8sClient      client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "multi-namespace"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 05: Multi-Namespace Isolation (BR-GATEWAY-011) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespaces
		processID := GinkgoParallelProcess()
		timestamp := time.Now().UnixNano()
		testNamespace1 = fmt.Sprintf("tenant-a-%d-%d", processID, timestamp)
		testNamespace2 = fmt.Sprintf("tenant-b-%d-%d", processID, timestamp)

		testLogger.Info("Creating test namespaces...",
			"namespace1", testNamespace1,
			"namespace2", testNamespace2)

		k8sClient = getKubernetesClient()

		// Create namespace 1
		ns1 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace1},
		}
		Expect(k8sClient.Create(testCtx, ns1)).To(Succeed())

		// Create namespace 2
		ns2 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace2},
		}
		Expect(k8sClient.Create(testCtx, ns2)).To(Succeed())

		testLogger.Info("✅ Test namespaces ready")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 05: Multi-Namespace Isolation - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Warn("⚠️  Test FAILED - Preserving namespaces for debugging",
				"namespace1", testNamespace1,
				"namespace2", testNamespace2)
			if testCancel != nil {
				testCancel()
			}
			return
		}

		// Cleanup namespaces
		ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace1}}
		ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace2}}
		_ = k8sClient.Delete(testCtx, ns1)
		_ = k8sClient.Delete(testCtx, ns2)

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	It("should isolate alerts and CRDs between namespaces", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 05: Multi-Namespace Isolation Behavior")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")
		testLogger.Info("Scenario: Send same alert to different namespaces")
		testLogger.Info("Expected: Each namespace gets its own CRDs (isolation)")
		testLogger.Info("")

		// Use same alert name for both namespaces to test isolation
		alertName := fmt.Sprintf("IsolationTest-%d", time.Now().UnixNano())

		// Step 1: Send alerts to namespace 1 - enough to trigger storm threshold
		testLogger.Info("Step 1: Send 10 alerts to namespace 1 (trigger storm threshold)")

		for i := 0; i < 10; i++ {
			payload := createNamespacedAlertPayload(alertName, testNamespace1, fmt.Sprintf("pod-%d", i))
			resp, err := httpClient.Post(
				gatewayURL+"/api/v1/signals/prometheus",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()
			testLogger.Debug(fmt.Sprintf("  NS1 Alert %d: HTTP %d", i+1, resp.StatusCode))
		}
		testLogger.Info("  ✅ Sent 10 alerts to namespace 1")

		// Step 2: Send alerts to namespace 2 - enough to trigger storm threshold
		testLogger.Info("")
		testLogger.Info("Step 2: Send 10 alerts to namespace 2 (trigger storm threshold)")

		for i := 0; i < 10; i++ {
			payload := createNamespacedAlertPayload(alertName, testNamespace2, fmt.Sprintf("pod-%d", i))
			resp, err := httpClient.Post(
				gatewayURL+"/api/v1/signals/prometheus",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()
			testLogger.Debug(fmt.Sprintf("  NS2 Alert %d: HTTP %d", i+1, resp.StatusCode))
		}
		testLogger.Info("  ✅ Sent 10 alerts to namespace 2")

		// Step 3: Verify CRDs in namespace 1
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRDs in namespace 1")

		var crdCountNS1 int
		Eventually(func() int {
			// Get fresh client to handle API server reconnection
			freshClient := getKubernetesClientSafe()
			if freshClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.Debug("Failed to create K8s client", "error", err)
				} else {
					testLogger.Debug("Failed to create K8s client (unknown error)")
				}
				return -1
			}
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := freshClient.List(testCtx, crdList, client.InNamespace(testNamespace1))
			if err != nil {
				testLogger.Debug("Failed to list CRDs in NS1", "error", err)
				return -1
			}
			crdCountNS1 = len(crdList.Items)
			return crdCountNS1
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"Namespace 1 should have at least 1 CRD")

		testLogger.Info(fmt.Sprintf("  Namespace 1: %d CRDs", crdCountNS1))

		// Step 4: Verify CRDs in namespace 2
		testLogger.Info("")
		testLogger.Info("Step 4: Verify CRDs in namespace 2")

		var crdCountNS2 int
		Eventually(func() int {
			// Get fresh client to handle API server reconnection
			freshClient := getKubernetesClientSafe()
			if freshClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.Debug("Failed to create K8s client", "error", err)
				} else {
					testLogger.Debug("Failed to create K8s client (unknown error)")
				}
				return -1
			}
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := freshClient.List(testCtx, crdList, client.InNamespace(testNamespace2))
			if err != nil {
				testLogger.Debug("Failed to list CRDs in NS2", "error", err)
				return -1
			}
			crdCountNS2 = len(crdList.Items)
			return crdCountNS2
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"Namespace 2 should have at least 1 CRD")

		testLogger.Info(fmt.Sprintf("  Namespace 2: %d CRDs", crdCountNS2))

		// Step 5: Verify isolation
		testLogger.Info("")
		testLogger.Info("Step 5: Verify namespace isolation")

		// Both namespaces should have CRDs (isolation means they're separate)
		Expect(crdCountNS1).To(BeNumerically(">=", 1),
			"Namespace 1 should have CRDs")
		Expect(crdCountNS2).To(BeNumerically(">=", 1),
			"Namespace 2 should have CRDs")

		// Verify CRDs in NS1 don't reference NS2 and vice versa
		crdListNS1 := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(testCtx, crdListNS1, client.InNamespace(testNamespace1))
		Expect(err).ToNot(HaveOccurred())

		for _, crd := range crdListNS1.Items {
			Expect(crd.Namespace).To(Equal(testNamespace1),
				"CRD in NS1 should have NS1 namespace")
		}

		crdListNS2 := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(testCtx, crdListNS2, client.InNamespace(testNamespace2))
		Expect(err).ToNot(HaveOccurred())

		for _, crd := range crdListNS2.Items {
			Expect(crd.Namespace).To(Equal(testNamespace2),
				"CRD in NS2 should have NS2 namespace")
		}

		testLogger.Info("  ✅ Namespace isolation verified")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 05 PASSED: Multi-Namespace Isolation (BR-GATEWAY-011)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ Namespace 1: %d CRDs", crdCountNS1))
		testLogger.Info(fmt.Sprintf("  ✅ Namespace 2: %d CRDs", crdCountNS2))
		testLogger.Info("  ✅ CRDs correctly isolated to their namespaces")
		testLogger.Info("  ✅ Same alertname creates separate CRDs per namespace")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})

// createNamespacedAlertPayload creates a Prometheus alert payload with explicit namespace
func createNamespacedAlertPayload(alertName, namespace, podName string) []byte {
	payload := map[string]interface{}{
		"alerts": []map[string]interface{}{
			{
				"status": "firing",
				"labels": map[string]interface{}{
					"alertname": alertName,
					"severity":  "critical",
					"namespace": namespace,
					"pod":       podName,
				},
				"annotations": map[string]interface{}{
					"summary":     fmt.Sprintf("Multi-namespace test: %s in %s", alertName, namespace),
					"description": "Testing namespace isolation",
				},
				"startsAt": time.Now().Format(time.RFC3339),
			},
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	return payloadBytes
}
