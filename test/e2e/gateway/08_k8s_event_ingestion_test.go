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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

// Test 08: Kubernetes Event Ingestion (BR-GATEWAY-002)
// BEHAVIOR: Gateway accepts K8s Event payloads and creates RemediationRequest CRDs
// CORRECTNESS: CRDs contain the correct resource information from K8s Events
// Parallel-safe: Uses unique namespace per process
var _ = Describe("Test 08: Kubernetes Event Ingestion (BR-GATEWAY-002)", Ordered, func() {
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
		testLogger = logger.WithValues("test", "k8s-event-ingestion")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		// Unique namespace for parallel execution
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("k8s-event-%d-%s", processID, uuid.New().String()[:8])

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 08: Kubernetes Event Ingestion - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		k8sClient = getKubernetesClient()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace", "namespace", testNamespace)
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

	It("should process K8s Events and create CRDs with correct resource information", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 08: Kubernetes Event Ingestion - Behavior Validation")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// BEHAVIOR TEST: Gateway accepts K8s Event format
		// CORRECTNESS: HTTP response indicates acceptance
		testLogger.Info("Step 1: Verify Gateway accepts K8s Event payload format")

		podNames := []string{"api-server-0", "api-server-1", "api-server-2", "api-server-3", "api-server-4"}
		eventReason := "BackOff"
		eventMessage := "Back-off restarting failed container"
		acceptedCount := 0

		for i, podName := range podNames {
			payload := map[string]interface{}{
				"type":    "Warning",
				"reason":  eventReason,
				"message": eventMessage,
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      podName,
					"namespace": testNamespace,
				},
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("event-%d-%s", i, uuid.New().String()[:8]),
					"namespace": testNamespace,
				},
				"firstTimestamp": time.Now().Format(time.RFC3339),
				"lastTimestamp":  time.Now().Format(time.RFC3339),
				"count":          1,
			}
			payloadBytes, _ := json.Marshal(payload)

			var resp *http.Response
			Eventually(func() error {
				req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/kubernetes-event", bytes.NewBuffer(payloadBytes))
				if err != nil {
					return err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				resp, err = httpClient.Do(req)
				return err
			}, 10*time.Second, 1*time.Second).Should(Succeed())
			_ = resp.Body.Close()

			// CORRECTNESS: Gateway should accept (201/202) or handle gracefully
			if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
				acceptedCount++
			}
			testLogger.V(1).Info(fmt.Sprintf("  Event %d for pod %s: HTTP %d", i+1, podName, resp.StatusCode))
		}

		// BEHAVIOR: At least some events should be accepted
		Expect(acceptedCount).To(BeNumerically(">=", 1),
			"Gateway should accept at least 1 K8s Event (BR-GATEWAY-002)")
		testLogger.Info(fmt.Sprintf("  ✅ Gateway accepted %d/%d K8s Events", acceptedCount, len(podNames)))

		// BEHAVIOR TEST: K8s Events trigger CRD creation
		// CORRECTNESS: CRDs are created in the correct namespace
		testLogger.Info("")
		testLogger.Info("Step 2: Verify CRDs are created from K8s Events")

		var crdList *remediationv1alpha1.RemediationRequestList
		Eventually(func() int {
			freshClient := getKubernetesClientSafe()
			if freshClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.V(1).Info("Failed to create K8s client", "error", err)
				}
				return -1
			}
			crdList = &remediationv1alpha1.RemediationRequestList{}
			if err := freshClient.List(testCtx, crdList, client.InNamespace(testNamespace)); err != nil {
				testLogger.V(1).Info("Failed to list CRDs", "error", err)
				return -1
			}
			return len(crdList.Items)
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"K8s Events should result in CRD creation (BR-GATEWAY-002)")

		testLogger.Info(fmt.Sprintf("  ✅ Created %d CRDs from K8s Events", len(crdList.Items)))

		// CORRECTNESS TEST: CRD contains correct resource information
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRD contains correct resource information")

		crd := crdList.Items[0]

		// CORRECTNESS: CRD namespace matches event namespace
		Expect(crd.Namespace).To(Equal(testNamespace),
			"CRD namespace should match K8s Event namespace")
		testLogger.Info(fmt.Sprintf("  ✅ CRD namespace correct: %s", crd.Namespace))

		// CORRECTNESS: CRD has target resource populated (from involvedObject)
		Expect(crd.Spec.TargetResource.Name).ToNot(BeEmpty(),
			"CRD should have target resource from K8s Event")
		testLogger.Info(fmt.Sprintf("  ✅ CRD target resource: %s/%s/%s",
			crd.Spec.TargetResource.Namespace,
			crd.Spec.TargetResource.Kind,
			crd.Spec.TargetResource.Name))

		// CORRECTNESS: Target resource matches event involvedObject (Pod)
		Expect(crd.Spec.TargetResource.Kind).To(Equal("Pod"),
			"Target resource should be Pod from K8s Event involvedObject")
		testLogger.Info(fmt.Sprintf("  ✅ Target resource kind: %s", crd.Spec.TargetResource.Kind))

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 08 PASSED: Kubernetes Event Ingestion")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Behavior Validated:")
		testLogger.Info(fmt.Sprintf("  ✅ Gateway accepts K8s Event payloads: %d accepted", acceptedCount))
		testLogger.Info(fmt.Sprintf("  ✅ K8s Events trigger CRD creation: %d CRDs", len(crdList.Items)))
		testLogger.Info("Correctness Validated:")
		testLogger.Info(fmt.Sprintf("  ✅ CRD namespace matches event: %s", testNamespace))
		testLogger.Info("  ✅ CRD contains Pod resource from involvedObject")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
