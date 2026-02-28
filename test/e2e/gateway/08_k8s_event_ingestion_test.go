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
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
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
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "k8s-event-ingestion")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 08: Kubernetes Event Ingestion - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Create unique test namespace (Pattern: RO E2E)
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "k8s-event")
		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace", "namespace", testNamespace)
		} else {
			// Clean up test namespace (Pattern: RO E2E)
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
			testLogger.Info("✅ Test cleanup complete")
		}
		if testCancel != nil {
			testCancel()
		}
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
		var firstRRName string

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

			if i == 0 {
				// First event: use retry to handle scope informer cache propagation delay
				resp := sendWebhookExpectCreated(gatewayURL, "/api/v1/signals/kubernetes-event", payloadBytes)
				acceptedCount++
				// Capture the RR name from the first successfully created event
				var gwResp GatewayResponse
				if err := json.Unmarshal(resp.Body, &gwResp); err == nil && gwResp.RemediationRequestName != "" {
					firstRRName = gwResp.RemediationRequestName
				}
				testLogger.V(1).Info(fmt.Sprintf("  Event %d for pod %s: HTTP %d (retried for scope cache)",
					i+1, podName, resp.StatusCode))
			} else {
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

				if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
					acceptedCount++
				}
				testLogger.V(1).Info(fmt.Sprintf("  Event %d for pod %s: HTTP %d", i+1, podName, resp.StatusCode))
			}
		}

		// BEHAVIOR: At least some events should be accepted
		Expect(acceptedCount).To(BeNumerically(">=", 1),
			"Gateway should accept at least 1 K8s Event (BR-GATEWAY-002)")
		testLogger.Info(fmt.Sprintf("  Gateway accepted %d/%d K8s Events", acceptedCount, len(podNames)))

		// CORRECTNESS TEST: CRD contains correct resource information
		// Use the specific RR name from the first event instead of listing all RRs
		// (avoids test isolation issues from stale RRs in shared gatewayNamespace)
		testLogger.Info("")
		testLogger.Info("Step 2: Verify CRD contains correct resource information")

		Expect(firstRRName).ToNot(BeEmpty(), "First event should return an RR name in the response")

		var crd remediationv1alpha1.RemediationRequest
		Eventually(func() error {
			return k8sClient.Get(testCtx, client.ObjectKey{
				Namespace: gatewayNamespace,
				Name:      firstRRName,
			}, &crd)
		}, 30*time.Second, 1*time.Second).Should(Succeed(),
			"CRD should be queryable by name from first event response")

		// CORRECTNESS: CRD in controller namespace (ADR-057); target resource matches event
		Expect(crd.Namespace).To(Equal(gatewayNamespace),
			"ADR-057: RRs created in controller namespace")
		testLogger.Info(fmt.Sprintf("  CRD namespace correct: %s", crd.Namespace))

		// CORRECTNESS: CRD has target resource populated (from involvedObject)
		Expect(crd.Spec.TargetResource.Name).ToNot(BeEmpty(),
			"CRD should have target resource from K8s Event")
		testLogger.Info(fmt.Sprintf("  CRD target resource: %s/%s/%s",
			crd.Spec.TargetResource.Namespace,
			crd.Spec.TargetResource.Kind,
			crd.Spec.TargetResource.Name))

		// CORRECTNESS: Target resource matches event involvedObject (Pod)
		Expect(crd.Spec.TargetResource.Kind).To(Equal("Pod"),
			"Target resource should be Pod from K8s Event involvedObject")
		testLogger.Info(fmt.Sprintf("  Target resource kind: %s", crd.Spec.TargetResource.Kind))

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 08 PASSED: Kubernetes Event Ingestion")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Behavior Validated:")
		testLogger.Info(fmt.Sprintf("  Gateway accepts K8s Event payloads: %d accepted", acceptedCount))
		testLogger.Info("Correctness Validated:")
		testLogger.Info(fmt.Sprintf("  CRD in controller namespace; target: %s/%s",
			crd.Spec.TargetResource.Kind, crd.Spec.TargetResource.Name))
		testLogger.Info("  CRD contains Pod resource from involvedObject")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
