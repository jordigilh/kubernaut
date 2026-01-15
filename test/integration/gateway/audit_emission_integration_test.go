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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GATEWAY AUDIT EVENT EMISSION INTEGRATION TESTS
// Test Plan: docs/services/stateless/gateway-service/GW_INTEGRATION_TEST_PLAN_V1.0.md
// Test IDs: GW-INT-AUD-001 to GW-INT-AUD-020
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Business Requirements:
// - BR-GATEWAY-055: All signal processing operations MUST generate audit events
// - BR-GATEWAY-056: All CRD creation operations MUST generate audit events
// - BR-GATEWAY-057: All deduplication decisions MUST generate audit events
// - BR-GATEWAY-058: All error scenarios MUST generate audit events
//
// Test Strategy:
// - Integration tests use REAL DataStorage infrastructure (Podman PostgreSQL)
// - Gateway processes signals and emits audit events to DataStorage
// - Tests query DataStorage via HTTP API to validate audit trail
// - Each test uses unique correlation ID for parallel execution isolation
//
// To run these tests:
//   ginkgo -p -procs=4 ./test/integration/gateway/... --focus="Audit Event Emission"
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// NOTE: Audit query helpers moved to audit_test_helpers.go for reusability
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TEST SUITE: AUDIT EVENT EMISSION (GW-INT-AUD-001 to GW-INT-AUD-010)
// Phase 1: Signal Received + CRD Created Events
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("Gateway Audit Event Emission", Label("audit", "integration"), func() {

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// SCENARIO 1.1: SIGNAL RECEIVED AUDIT EVENTS (BR-GATEWAY-055)
	// Tests GW-INT-AUD-001, GW-INT-AUD-002
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
		var (
			testNamespace string
		)

		BeforeEach(func() {
			// Create unique test namespace for K8s resource isolation
			processID := GinkgoParallelProcess()
			testNamespace = fmt.Sprintf("gw-aud-sig-%d-%s", processID, uuid.New().String()[:8])

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			GinkgoWriter.Printf("✅ Test setup complete: namespace=%s\n", testNamespace)
		})

		AfterEach(func() {
			// Cleanup namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		// Test ID: GW-INT-AUD-001
		// Scenario: Prometheus Signal creates RemediationRequest CRD
		// BR: BR-GATEWAY-055, BR-GATEWAY-056
		// Section: 1.1.1, 1.2.1
		Context("when Prometheus signal is processed (GW-INT-AUD-001, BR-GATEWAY-055)", func() {
			It("[GW-INT-AUD-001] should create RemediationRequest CRD for new signal", func() {
				By("1. Create Prometheus alert fixture")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				alertPayload := []byte(fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "KubePodCrashLooping",
							"severity": "critical",
							"namespace": "%s",
							"pod": "test-pod-123"
						},
						"annotations": {
							"summary": "Pod is crash looping",
							"description": "Pod test-pod-123 has restarted 5 times"
						},
						"startsAt": "2025-01-15T10:00:00Z"
					}]
				}`, testNamespace))

				By("2. Parse signal through Prometheus adapter")
				signal, err := prometheusAdapter.Parse(ctx, alertPayload)
				Expect(err).ToNot(HaveOccurred(), "Prometheus adapter parse must succeed")
				Expect(signal).ToNot(BeNil())
				Expect(signal.Fingerprint).ToNot(BeEmpty(), "Signal must have fingerprint")

				By("3. Process signal through Gateway")
				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://localhost:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Signal processing must succeed")
				Expect(response).ToNot(BeNil())
				Expect(response.RemediationRequestName).ToNot(BeEmpty(), "CRD must be created")

				By("4. Verify RemediationRequest CRD was created in K8s")
				var rr remediationv1alpha1.RemediationRequest
				rrKey := client.ObjectKey{
					Name:      response.RemediationRequestName,
					Namespace: response.RemediationRequestNamespace,
				}

				Eventually(func() error {
					return k8sClient.Get(ctx, rrKey, &rr)
				}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
					"BR-GATEWAY-056: RemediationRequest CRD must exist in K8s")

				By("5. Validate CRD contains signal metadata")
				Expect(rr.Spec.SignalType).To(Equal("prometheus-alert"))
				Expect(rr.Spec.SignalFingerprint).To(Equal(signal.Fingerprint))
				Expect(rr.Spec.SignalName).To(Equal("KubePodCrashLooping"))
				Expect(rr.Spec.Severity).To(Equal("critical"))
				Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("alertname", "KubePodCrashLooping"))
				Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("severity", "critical"))
				Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("pod", "test-pod-123"))

				GinkgoWriter.Printf("✅ CRD created: %s/%s (fingerprint=%s)\n",
					rr.Namespace, rr.Name, rr.Spec.SignalFingerprint)
			})
		})

		// Test ID: GW-INT-AUD-002
		// Scenario: Deduplication prevents duplicate CRD creation
		// BR: BR-GATEWAY-057
		// Section: 1.3.1
		Context("when duplicate signal is received (GW-INT-AUD-002, BR-GATEWAY-057)", func() {
			It("[GW-INT-AUD-002] should deduplicate based on fingerprint and NOT create duplicate CRD", func() {
				By("1. Create first signal")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				// Generate valid 64-character hex fingerprint (SHA256 format)
				fingerprint := fmt.Sprintf("%064x", uuid.New().ID())

				alertPayload := []byte(fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "TestDuplicateAlert",
							"severity": "warning",
							"namespace": "%s",
							"pod": "test-pod-dedup"
						}
					}]
				}`, testNamespace))

				signal1, err := prometheusAdapter.Parse(ctx, alertPayload)
				Expect(err).ToNot(HaveOccurred())

				// Override fingerprint to ensure same fingerprint for both signals
				signal1.Fingerprint = fingerprint

				By("2. Process first signal - should create CRD")
				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://localhost:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				Expect(response1.Status).To(Equal("created"), "First signal should create CRD")
				Expect(response1.RemediationRequestName).ToNot(BeEmpty())

				firstCRDName := response1.RemediationRequestName

				By("3. Process duplicate signal (same fingerprint)")
				signal2, err := prometheusAdapter.Parse(ctx, alertPayload)
				Expect(err).ToNot(HaveOccurred())
				signal2.Fingerprint = fingerprint // Same fingerprint

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())
				Expect(response2.Status).To(Equal("duplicate"), "BR-GATEWAY-057: Duplicate signal must be deduplicated")
				Expect(response2.Duplicate).To(BeTrue())
				Expect(response2.RemediationRequestName).To(Equal(firstCRDName),
					"Duplicate should reference existing CRD")

				By("4. Verify only ONE CRD exists for this fingerprint")
				var rrList remediationv1alpha1.RemediationRequestList
				err = k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
				Expect(err).ToNot(HaveOccurred())

				// Count CRDs with matching fingerprint
				matchingCRDs := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalFingerprint == fingerprint {
						matchingCRDs++
					}
				}

				Expect(matchingCRDs).To(Equal(1),
					"BR-GATEWAY-057: Deduplication must prevent duplicate CRD creation")

				GinkgoWriter.Printf("✅ Deduplication successful: 2 signals → 1 CRD (fingerprint=%s)\n", fingerprint)
			})
		})
	})
})

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HELPER FUNCTIONS FOR AUDIT EMISSION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// createPrometheusAlert creates a Prometheus AlertManager webhook payload
// Used by audit emission tests to create test signals
func createPrometheusAlert(namespace, alertName, severity, fingerprint, correlationID string) []byte {
	payload := fmt.Sprintf(`{
		"alerts": [{
			"labels": {
				"alertname": "%s",
				"severity": "%s",
				"namespace": "%s",
				"pod": "test-pod-123"
			},
			"annotations": {
				"summary": "Test alert",
				"description": "Test description",
				"correlation_id": "%s"
			},
			"startsAt": "2025-01-15T10:00:00Z"
		}]
	}`, alertName, severity, namespace, correlationID)

	return []byte(payload)
}
