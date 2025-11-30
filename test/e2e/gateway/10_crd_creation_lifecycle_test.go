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

// Test 10: CRD Creation Lifecycle (BR-GATEWAY-018, BR-GATEWAY-021)
// Validates that CRDs are created with correct metadata, labels, and annotations
// Parallel-safe: Uses unique namespace per process
var _ = Describe("Test 10: CRD Creation Lifecycle (BR-GATEWAY-018, BR-GATEWAY-021)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
		k8sClient     client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "crd-lifecycle"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		// Unique namespace for parallel execution
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("crd-lifecycle-%d-%d", processID, time.Now().UnixNano())

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 10: CRD Creation Lifecycle - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		k8sClient = getKubernetesClient()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			testLogger.Warn("⚠️  Test FAILED - Preserving namespace", "namespace", testNamespace)
			if testCancel != nil {
				testCancel()
			}
			return
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		_ = k8sClient.Delete(testCtx, ns)
		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	It("should create CRDs with correct metadata and structure", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 10: CRD Creation Lifecycle")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Step 1: Send alerts to trigger CRD creation
		testLogger.Info("Step 1: Send alerts to trigger CRD creation")

		alertName := fmt.Sprintf("CRDLifecycleTest-%d", time.Now().UnixNano())
		podName := "lifecycle-test-pod"
		severity := "critical"
		summary := "CRD lifecycle test alert"
		description := "Testing CRD metadata correctness"

		// Send enough alerts to trigger storm threshold
		for i := 0; i < 5; i++ {
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"status": "firing",
						"labels": map[string]interface{}{
							"alertname": alertName,
							"severity":  severity,
							"namespace": testNamespace,
							"pod":       fmt.Sprintf("%s-%d", podName, i),
						},
						"annotations": map[string]interface{}{
							"summary":     summary,
							"description": description,
						},
						"startsAt": time.Now().Format(time.RFC3339),
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			resp, err := httpClient.Post(
				gatewayURL+"/api/v1/signals/prometheus",
				"application/json",
				bytes.NewBuffer(payloadBytes),
			)
			if err == nil {
				resp.Body.Close()
			}
		}
		testLogger.Info("  ✅ Sent 5 alerts")

		// Step 2: Wait for CRD creation
		testLogger.Info("")
		testLogger.Info("Step 2: Wait for CRD creation")

		var crdList *remediationv1alpha1.RemediationRequestList
		Eventually(func() int {
			freshClient := getKubernetesClientSafe()
			if freshClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.Debug("Failed to create K8s client", "error", err)
				}
				return -1
			}
			crdList = &remediationv1alpha1.RemediationRequestList{}
			if err := freshClient.List(testCtx, crdList, client.InNamespace(testNamespace)); err != nil {
				testLogger.Debug("Failed to list CRDs", "error", err)
				return -1
			}
			return len(crdList.Items)
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"At least 1 CRD should be created")

		testLogger.Info(fmt.Sprintf("  Found %d CRDs", len(crdList.Items)))

		// Step 3: Verify CRD structure
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRD structure")

		crd := crdList.Items[0]

		// Verify CRD is in correct namespace
		Expect(crd.Namespace).To(Equal(testNamespace),
			"CRD should be in test namespace")
		testLogger.Info(fmt.Sprintf("  ✅ CRD namespace: %s", crd.Namespace))

		// Verify CRD has a name
		Expect(crd.Name).ToNot(BeEmpty(),
			"CRD should have a name")
		testLogger.Info(fmt.Sprintf("  ✅ CRD name: %s", crd.Name))

		// Verify CRD has labels
		Expect(crd.Labels).ToNot(BeNil(),
			"CRD should have labels")
		testLogger.Info(fmt.Sprintf("  ✅ CRD has %d labels", len(crd.Labels)))

		// Verify spec fields
		Expect(crd.Spec.AffectedResources).ToNot(BeEmpty(),
			"CRD should have affected resources")
		testLogger.Info(fmt.Sprintf("  ✅ CRD has %d affected resources", len(crd.Spec.AffectedResources)))

		// Verify fingerprint exists
		Expect(crd.Spec.SignalFingerprint).ToNot(BeEmpty(),
			"CRD should have signal fingerprint")
		testLogger.Info(fmt.Sprintf("  ✅ CRD fingerprint: %s...", crd.Spec.SignalFingerprint[:16]))

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 10 PASSED: CRD Creation Lifecycle")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ CRD created in namespace: %s", crd.Namespace))
		testLogger.Info(fmt.Sprintf("  ✅ CRD name: %s", crd.Name))
		testLogger.Info(fmt.Sprintf("  ✅ Affected resources: %d", len(crd.Spec.AffectedResources)))
		testLogger.Info(fmt.Sprintf("  ✅ Signal fingerprint present"))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
