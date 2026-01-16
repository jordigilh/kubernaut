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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	sharedhelpers "github.com/jordigilh/kubernaut/test/shared/helpers"
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
				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
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
				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
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
				Expect(response2.Status).To(Equal("deduplicated"), "BR-GATEWAY-057: Duplicate signal must be deduplicated")
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

		// Test ID: GW-INT-AUD-003
		// Scenario: Correlation ID format validation
		// BR: BR-GATEWAY-055
		// Section: 1.1.3
		Context("when validating correlation ID format (GW-INT-AUD-003, BR-GATEWAY-055)", func() {
			It("[GW-INT-AUD-003] should generate correlation IDs with correct format for audit traceability", func() {
				By("1. Process multiple Prometheus signals")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				signal1Payload := createPrometheusAlert(testNamespace, "test-alert-1", "critical", "", "")
				signal2Payload := createPrometheusAlert(testNamespace, "test-alert-2", "warning", "", "")

				// Parse signals
				signal1, err := prometheusAdapter.Parse(ctx, signal1Payload)
				Expect(err).ToNot(HaveOccurred())
				signal2, err := prometheusAdapter.Parse(ctx, signal2Payload)
				Expect(err).ToNot(HaveOccurred())

				// Create Gateway server
				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				// Process signals
				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				correlationID1 := response1.RemediationRequestName // RR name = correlation ID

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())
				correlationID2 := response2.RemediationRequestName

				By("2. Validate correlation ID format")
				// BR-GATEWAY-055: Format: rr-{12-char-hex-fingerprint}-{10-digit-timestamp}
				// This enables fingerprint extraction for deduplication
				correlationIDPattern := `^rr-[a-f0-9]{12}-\d{10}$`
				Expect(correlationID1).To(MatchRegexp(correlationIDPattern),
					"BR-GATEWAY-055: Correlation ID must follow rr-{fingerprint}-{timestamp} format")
				Expect(correlationID2).To(MatchRegexp(correlationIDPattern),
					"BR-GATEWAY-055: Correlation ID must follow standard format")

				By("3. Validate correlation IDs are unique")
				Expect(correlationID1).ToNot(Equal(correlationID2),
					"BR-GATEWAY-055: Each signal must have unique correlation ID for audit tracing")

				GinkgoWriter.Printf("✅ Correlation IDs valid: %s, %s\n", correlationID1, correlationID2)
			})
		})

		// Test ID: GW-INT-AUD-004
		// Scenario: Signal labels and annotations preservation
		// BR: BR-GATEWAY-055
		// Section: 1.1.4
		Context("when processing signals with custom labels (GW-INT-AUD-004, BR-GATEWAY-055)", func() {
			It("[GW-INT-AUD-004] should preserve all signal labels and annotations in audit events", func() {
				By("1. Create Prometheus alert with custom labels")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				customPayload := []byte(fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "CustomLabelsTest",
							"severity": "critical",
							"namespace": "%s",
							"team": "platform",
							"environment": "production",
							"component": "api-server"
						},
						"annotations": {
							"summary": "Custom alert with metadata",
							"description": "Testing label preservation",
							"runbook_url": "https://wiki.example.com/runbook"
						},
						"startsAt": "2025-01-15T10:00:00Z"
					}]
				}`, testNamespace))

				By("2. Parse and process signal through Gateway")
				signal, err := prometheusAdapter.Parse(ctx, customPayload)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				correlationID := response.RemediationRequestName

				By("3. Query audit event from DataStorage")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.signal.received"
				var receivedEvents []ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					receivedEvents = events
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
					"gateway.signal.received audit event should exist")

				By("4. Validate all custom labels are preserved")
				event := receivedEvents[0]
				payload, ok := extractGatewayPayload(&event)
				Expect(ok).To(BeTrue())

				signalLabels, hasLabels := payload.SignalLabels.Get()
				Expect(hasLabels).To(BeTrue(), "BR-GATEWAY-055: SignalLabels must be preserved")
				Expect(signalLabels).To(HaveKeyWithValue("team", "platform"))
				Expect(signalLabels).To(HaveKeyWithValue("environment", "production"))
				Expect(signalLabels).To(HaveKeyWithValue("component", "api-server"))
				Expect(signalLabels).To(HaveKeyWithValue("severity", "critical"))

				By("5. Validate all annotations are preserved")
				signalAnnotations, hasAnnotations := payload.SignalAnnotations.Get()
				Expect(hasAnnotations).To(BeTrue(), "BR-GATEWAY-055: SignalAnnotations must be preserved")
				Expect(signalAnnotations).To(HaveKeyWithValue("summary", "Custom alert with metadata"))
				Expect(signalAnnotations).To(HaveKeyWithValue("runbook_url", "https://wiki.example.com/runbook"))

				GinkgoWriter.Printf("✅ All custom labels and annotations preserved in audit event\n")
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// SCENARIO 1.2: CRD CREATED AUDIT EVENTS (BR-GATEWAY-056)
	// Tests GW-INT-AUD-006, GW-INT-AUD-007
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-056: CRD Created Audit Events", func() {
		var (
			testNamespace string
		)

		BeforeEach(func() {
			processID := GinkgoParallelProcess()
			testNamespace = fmt.Sprintf("gw-aud-crd-%d-%s", processID, uuid.New().String()[:8])

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			GinkgoWriter.Printf("✅ Test setup complete: namespace=%s\n", testNamespace)
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		// Test ID: GW-INT-AUD-006
		// Scenario: CRD created audit event emission
		// BR: BR-GATEWAY-056
		// Section: 1.2.1
		Context("when CRD is created (GW-INT-AUD-006, BR-GATEWAY-056)", func() {
			It("[GW-INT-AUD-006] should emit gateway.crd.created audit event after RemediationRequest creation", func() {
				By("1. Process Prometheus signal to create CRD")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				signalPayload := createPrometheusAlert(testNamespace, "test-crd-audit", "critical", "", "")

				signal, err := prometheusAdapter.Parse(ctx, signalPayload)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				correlationID := response.RemediationRequestName

				By("2. Query gateway.crd.created audit event from DataStorage")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.created"
				var crdCreatedEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					crdCreatedEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
					"gateway.crd.created audit event should exist in DataStorage")

				By("3. Validate audit event metadata")
				Expect(crdCreatedEvent.EventType).To(Equal("gateway.crd.created"))
				Expect(crdCreatedEvent.EventAction).To(Equal("created"))
				Expect(crdCreatedEvent.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryGateway))
				Expect(crdCreatedEvent.CorrelationID).To(Equal(correlationID))

				By("4. Validate Gateway payload contains CRD reference")
				payload, ok := extractGatewayPayload(crdCreatedEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-056: RemediationRequest field must contain namespace/name format
				rrRef, hasRR := payload.RemediationRequest.Get()
				Expect(hasRR).To(BeTrue(), "BR-GATEWAY-056: RemediationRequest field must be present")
				Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`),
					"BR-GATEWAY-056: RemediationRequest must be in namespace/name format")

				// Validate namespace matches
				Expect(payload.Namespace).To(Equal(testNamespace))

				GinkgoWriter.Printf("✅ CRD created audit event validated: rr=%s\n", rrRef)
			})
		})

		// Test ID: GW-INT-AUD-007
		// Scenario: CRD target resource in audit event
		// BR: BR-GATEWAY-056
		// Section: 1.2.2
		Context("when CRD has target resource (GW-INT-AUD-007, BR-GATEWAY-056)", func() {
			It("[GW-INT-AUD-007] should include target resource metadata in CRD created audit event", func() {
				By("1. Create Prometheus alert with resource information")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				// Prometheus alerts include pod/namespace in labels
				payloadWithResource := []byte(fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "PodCrashLoop",
							"severity": "critical",
							"namespace": "%s",
							"pod": "failing-pod-xyz",
							"container": "app"
						},
						"annotations": {
							"summary": "Pod is crash looping",
							"description": "Pod failing-pod-xyz in namespace %s is restarting"
						},
						"startsAt": "2025-01-15T10:00:00Z"
					}]
				}`, testNamespace, testNamespace))

				By("2. Parse and process signal through Gateway")
				signal, err := prometheusAdapter.Parse(ctx, payloadWithResource)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				correlationID := response.RemediationRequestName

				By("3. Query gateway.crd.created audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.created"
				var crdCreatedEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					crdCreatedEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("4. Validate target resource metadata is preserved")
				payload, ok := extractGatewayPayload(crdCreatedEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-056: CRD created event includes ResourceKind and ResourceName
				// Note: SignalLabels are in gateway.signal.received, not gateway.crd.created
				// The CRD created event focuses on the created resource metadata

				// Validate namespace is preserved
				Expect(payload.Namespace).To(Equal(testNamespace),
					"BR-GATEWAY-056: Namespace must match signal namespace")

				// Validate alert name is preserved
				Expect(payload.AlertName).To(Equal("PodCrashLoop"),
					"BR-GATEWAY-056: Alert name must be preserved")

				// Validate RemediationRequest reference
				rrRef, hasRR := payload.RemediationRequest.Get()
				Expect(hasRR).To(BeTrue())
				Expect(rrRef).To(ContainSubstring(testNamespace),
					"BR-GATEWAY-056: RR reference must contain namespace")
				Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`),
					"BR-GATEWAY-056: RR reference must be in namespace/name format")

				GinkgoWriter.Printf("✅ Target resource metadata preserved: alert=%s, namespace=%s\n",
					payload.AlertName, payload.Namespace)
			})
		})

		// Test ID: GW-INT-AUD-008
		// Scenario: CRD fingerprint in audit event
		// BR: BR-GATEWAY-056
		// Section: 1.2.3
		Context("when validating fingerprint in CRD audit (GW-INT-AUD-008, BR-GATEWAY-056)", func() {
			It("[GW-INT-AUD-008] should include fingerprint in gateway.crd.created audit event for dedup tracking", func() {
				By("1. Process Prometheus signal to create CRD")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				signalPayload := createPrometheusAlert(testNamespace, "high-cpu-usage", "warning", "", "")

				signal, err := prometheusAdapter.Parse(ctx, signalPayload)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				correlationID := response.RemediationRequestName

				By("2. Query gateway.crd.created audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.created"
				var crdCreatedEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					crdCreatedEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("3. Validate fingerprint format")
				payload, ok := extractGatewayPayload(crdCreatedEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-056: Fingerprint must be SHA-256 format (64 hex chars)
				Expect(payload.Fingerprint).To(MatchRegexp("^[a-f0-9]{64}$"),
					"BR-GATEWAY-056: Fingerprint must be 64-character hex (SHA-256)")
				Expect(payload.Fingerprint).To(Equal(signal.Fingerprint),
					"BR-GATEWAY-056: Audit fingerprint must match signal fingerprint")

				GinkgoWriter.Printf("✅ Fingerprint validated: %s\n", payload.Fingerprint)
			})
		})

		// Test ID: GW-INT-AUD-010
		// Scenario: Unique correlation IDs for multiple CRDs
		// BR: BR-GATEWAY-056
		// Section: 1.2.5
		Context("when creating multiple CRDs (GW-INT-AUD-010, BR-GATEWAY-056)", func() {
			It("[GW-INT-AUD-010] should emit unique correlation IDs for each CRD creation", func() {
				By("1. Process multiple Prometheus signals")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				signal1Payload := createPrometheusAlert(testNamespace, "alert-multi-1", "critical", "", "")
				signal2Payload := createPrometheusAlert(testNamespace, "alert-multi-2", "warning", "", "")

				signal1, err := prometheusAdapter.Parse(ctx, signal1Payload)
				Expect(err).ToNot(HaveOccurred())
				signal2, err := prometheusAdapter.Parse(ctx, signal2Payload)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				correlationID1 := response1.RemediationRequestName

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())
				correlationID2 := response2.RemediationRequestName

				By("2. Validate correlation IDs are unique and properly formatted")
				Expect(correlationID1).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`),
					"BR-GATEWAY-056: Correlation ID must follow standard format")
				Expect(correlationID2).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`))
				Expect(correlationID1).ToNot(Equal(correlationID2),
					"BR-GATEWAY-056: Each CRD must have unique correlation ID")

				By("3. Validate correlation IDs match CRD names")
				var rr1 remediationv1alpha1.RemediationRequest
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{
						Name:      correlationID1,
						Namespace: testNamespace,
					}, &rr1)
				}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
					"BR-GATEWAY-056: Correlation ID must match CRD name for audit-to-CRD mapping")

				GinkgoWriter.Printf("✅ Unique correlation IDs validated: %s, %s\n", correlationID1, correlationID2)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// SCENARIO 1.3: SIGNAL DEDUPLICATED AUDIT EVENTS (BR-GATEWAY-057)
	// Tests GW-INT-AUD-011, GW-INT-AUD-012
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-057: Signal Deduplicated Audit Events", func() {
		var (
			testNamespace string
		)

		BeforeEach(func() {
			processID := GinkgoParallelProcess()
			testNamespace = fmt.Sprintf("gw-aud-dedup-%d-%s", processID, uuid.New().String()[:8])

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			GinkgoWriter.Printf("✅ Test setup complete: namespace=%s\n", testNamespace)
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		// Test ID: GW-INT-AUD-011
		// Scenario: Deduplication audit event emission
		// BR: BR-GATEWAY-057
		// Section: 1.3.1
		Context("when duplicate signal arrives (GW-INT-AUD-011, BR-GATEWAY-057)", func() {
			It("[GW-INT-AUD-011] should emit gateway.signal.deduplicated audit event for duplicate signal", func() {
				By("1. Create first RemediationRequest CRD")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				// Use identical fingerprint to trigger deduplication
				firstSignalPayload := createPrometheusAlert(testNamespace, "repeated-error", "error", "", "")

				signal1, err := prometheusAdapter.Parse(ctx, firstSignalPayload)
				Expect(err).ToNot(HaveOccurred())
				firstFingerprint := signal1.Fingerprint

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				firstCRDName := response1.RemediationRequestName

				// Wait for CRD to be created
				time.Sleep(1 * time.Second)

				By("2. Send duplicate signal with same fingerprint")
				secondSignalPayload := createPrometheusAlert(testNamespace, "repeated-error", "error", firstFingerprint, "")
				signal2, err := prometheusAdapter.Parse(ctx, secondSignalPayload)
				Expect(err).ToNot(HaveOccurred())

				// Override fingerprint to match first signal (to trigger deduplication)
				signal2.Fingerprint = firstFingerprint

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())

				// BR-GATEWAY-057: Deduplication should return existing CRD name
				Expect(response2.RemediationRequestName).To(Equal(firstCRDName),
					"BR-GATEWAY-057: Dedup should return existing CRD, not create new one")

				By("3. Query gateway.signal.deduplicated audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				// Note: The deduplicated event uses the FIRST CRD's correlation ID (existing RR)
				eventType := "gateway.signal.deduplicated"
				var dedupEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &firstCRDName, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					dedupEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
					"gateway.signal.deduplicated audit event should exist")

				By("4. Validate deduplication audit metadata")
				Expect(dedupEvent.EventType).To(Equal("gateway.signal.deduplicated"))
				Expect(dedupEvent.EventAction).To(Equal("deduplicated"))
				Expect(dedupEvent.CorrelationID).To(Equal(firstCRDName))

				GinkgoWriter.Printf("✅ Dedup audit event validated for CRD: %s\n", firstCRDName)
			})
		})

		// Test ID: GW-INT-AUD-012
		// Scenario: Existing RR reference in dedup audit
		// BR: BR-GATEWAY-057
		// Section: 1.3.2
		Context("when tracking existing RR (GW-INT-AUD-012, BR-GATEWAY-057)", func() {
			It("[GW-INT-AUD-012] should include existing RR reference in deduplicated audit event", func() {
				By("1. Create first RemediationRequest CRD")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				firstSignalPayload := createPrometheusAlert(testNamespace, "existing-rr-test", "critical", "", "")

				signal1, err := prometheusAdapter.Parse(ctx, firstSignalPayload)
				Expect(err).ToNot(HaveOccurred())
				firstFingerprint := signal1.Fingerprint

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				existingRRName := response1.RemediationRequestName

				time.Sleep(1 * time.Second)

				By("2. Send duplicate signal")
				secondSignalPayload := createPrometheusAlert(testNamespace, "existing-rr-test", "critical", firstFingerprint, "")
				signal2, err := prometheusAdapter.Parse(ctx, secondSignalPayload)
				Expect(err).ToNot(HaveOccurred())
				signal2.Fingerprint = firstFingerprint

				_, err = gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())

				By("3. Query gateway.signal.deduplicated audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.signal.deduplicated"
				var dedupEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &existingRRName, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					dedupEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("4. Validate existing RR reference in audit payload")
				payload, ok := extractGatewayPayload(dedupEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-057: RemediationRequest field contains existing RR reference
				rrRef, hasRR := payload.RemediationRequest.Get()
				Expect(hasRR).To(BeTrue(), "BR-GATEWAY-057: RemediationRequest field must be present")
				Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`),
					"BR-GATEWAY-057: RR reference must be in namespace/name format")
				Expect(rrRef).To(ContainSubstring(existingRRName),
					"BR-GATEWAY-057: RR reference must contain existing RR name")

				// Validate namespace is included
				Expect(rrRef).To(ContainSubstring(testNamespace))

				GinkgoWriter.Printf("✅ Existing RR reference validated: %s\n", rrRef)
			})
		})

		// Test ID: GW-INT-AUD-009
		// Scenario: CRD Occurrence Count
		// BR: BR-GATEWAY-056
		// Section: 1.2.3
		Context("when creating CRD (GW-INT-AUD-009, BR-GATEWAY-056)", func() {
			It("[GW-INT-AUD-009] should include occurrence_count=1 in CRD created audit event for new signal", func() {
				By("1. Create unique Prometheus alert")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				fingerprint := fmt.Sprintf("aaa%s000000000000000000000000000000000000000000000000000000000", uuid.New().String()[:8])
				correlationID := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alertPayload := createPrometheusAlert(testNamespace, "TestAlert", "critical", fingerprint, correlationID)

				By("2. Process signal through Gateway")
				signal, err := prometheusAdapter.Parse(ctx, alertPayload)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.Status).To(Equal("created"))

				By("3. Query gateway.crd.created audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.created"
				var crdCreatedEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					crdCreatedEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("4. Validate occurrence_count field in audit payload")
				payload, ok := extractGatewayPayload(crdCreatedEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-056: OccurrenceCount must be 1 for new CRD
				occurrenceCount, hasCount := payload.OccurrenceCount.Get()
				Expect(hasCount).To(BeTrue(), "BR-GATEWAY-056: OccurrenceCount field must be present")
				Expect(occurrenceCount).To(Equal(int32(1)),
					"BR-GATEWAY-056: OccurrenceCount must be 1 for new signal")

				GinkgoWriter.Printf("✅ OccurrenceCount validated: %d\n", occurrenceCount)
			})
		})

		// Test ID: GW-INT-AUD-013
		// Scenario: Deduplication Occurrence Count
		// BR: BR-GATEWAY-057
		// Section: 1.3.2
		Context("when processing duplicate signals (GW-INT-AUD-013, BR-GATEWAY-057)", func() {
			It("[GW-INT-AUD-013] should include incremented occurrence_count in deduplication audit events", func() {
				By("1. Create initial signal and process")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				fingerprint := fmt.Sprintf("bbb%s000000000000000000000000000000000000000000000000000000000", uuid.New().String()[:8])
				correlationID1 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert1 := createPrometheusAlert(testNamespace, "HighCPU", "critical", fingerprint, correlationID1)

				signal1, err := prometheusAdapter.Parse(ctx, alert1)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				existingRRName := response1.RemediationRequestName

				By("2. Process duplicate signal (same fingerprint)")
				time.Sleep(1 * time.Second) // Ensure distinct timestamp
				correlationID2 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert2 := createPrometheusAlert(testNamespace, "HighCPU", "critical", fingerprint, correlationID2)

				signal2, err := prometheusAdapter.Parse(ctx, alert2)
				Expect(err).ToNot(HaveOccurred())

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())
				Expect(response2.Status).To(Equal("deduplicated"))

				By("3. Query gateway.signal.deduplicated audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.signal.deduplicated"
				var dedupEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &existingRRName, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					dedupEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("4. Validate occurrence_count is incremented")
				payload, ok := extractGatewayPayload(dedupEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-057: OccurrenceCount should be incremented (>1)
				occurrenceCount, hasCount := payload.OccurrenceCount.Get()
				Expect(hasCount).To(BeTrue(), "BR-GATEWAY-057: OccurrenceCount field must be present")
				Expect(occurrenceCount).To(BeNumerically(">", 1),
					"BR-GATEWAY-057: OccurrenceCount must be incremented for duplicate")

				GinkgoWriter.Printf("✅ OccurrenceCount incremented: %d\n", occurrenceCount)
			})
		})

		// Test ID: GW-INT-AUD-014
		// Scenario: Multiple Fingerprints Independence
		// BR: BR-GATEWAY-057
		// Section: 1.3.2
		Context("when processing multiple distinct fingerprints (GW-INT-AUD-014, BR-GATEWAY-057)", func() {
			It("[GW-INT-AUD-014] should handle deduplication independently for different fingerprints", func() {
				By("1. Create first signal with fingerprint A")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				fingerprintA := fmt.Sprintf("ccc%s000000000000000000000000000000000000000000000000000000000", uuid.New().String()[:8])
				correlationID1 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert1 := createPrometheusAlert(testNamespace, "HighMemory", "warning", fingerprintA, correlationID1)

				signal1, err := prometheusAdapter.Parse(ctx, alert1)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				Expect(response1.Status).To(Equal("created"))
				rrNameA := response1.RemediationRequestName

				By("2. Create second signal with fingerprint B")
				time.Sleep(1 * time.Second)
				fingerprintB := fmt.Sprintf("ddd%s000000000000000000000000000000000000000000000000000000000", uuid.New().String()[:8])
				correlationID2 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert2 := createPrometheusAlert(testNamespace, "DiskFull", "critical", fingerprintB, correlationID2)

				signal2, err := prometheusAdapter.Parse(ctx, alert2)
				Expect(err).ToNot(HaveOccurred())

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())
				Expect(response2.Status).To(Equal("created"))
				rrNameB := response2.RemediationRequestName

				By("3. Process duplicate of fingerprint A")
				time.Sleep(1 * time.Second)
				correlationID3 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert3 := createPrometheusAlert(testNamespace, "HighMemory", "warning", fingerprintA, correlationID3)

				signal3, err := prometheusAdapter.Parse(ctx, alert3)
				Expect(err).ToNot(HaveOccurred())

				response3, err := gwServer.ProcessSignal(ctx, signal3)
				Expect(err).ToNot(HaveOccurred())
				Expect(response3.Status).To(Equal("deduplicated"))

				By("4. Verify deduplication audit references correct RR (fingerprint A)")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.signal.deduplicated"
				var dedupEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &rrNameA, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					dedupEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				payload, ok := extractGatewayPayload(dedupEvent)
				Expect(ok).To(BeTrue())

				// BR-GATEWAY-057: RemediationRequest must reference fingerprint A's RR
				rrRef, hasRR := payload.RemediationRequest.Get()
				Expect(hasRR).To(BeTrue())
				Expect(rrRef).To(ContainSubstring(rrNameA),
					"BR-GATEWAY-057: Dedup audit must reference correct RR for fingerprint A")
				Expect(rrRef).ToNot(ContainSubstring(rrNameB),
					"BR-GATEWAY-057: Dedup audit must NOT reference RR for fingerprint B")

				GinkgoWriter.Printf("✅ Independent deduplication validated: RR A=%s, RR B=%s\n", rrNameA, rrNameB)
			})
		})

		// Test ID: GW-INT-AUD-015
		// Scenario: Deduplication Phase-Based Rejection
		// BR: BR-GATEWAY-057
		// Section: 1.3.3
		Context("when signal arrives for completed RR (GW-INT-AUD-015, BR-GATEWAY-057)", func() {
			It("[GW-INT-AUD-015] should NOT deduplicate signals for RRs in terminal phases", func() {
				By("1. Create initial signal and process")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				fingerprint := fmt.Sprintf("eee%s000000000000000000000000000000000000000000000000000000000", uuid.New().String()[:8])
				correlationID1 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert1 := createPrometheusAlert(testNamespace, "ServiceDown", "critical", fingerprint, correlationID1)

				signal1, err := prometheusAdapter.Parse(ctx, alert1)
				Expect(err).ToNot(HaveOccurred())

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				response1, err := gwServer.ProcessSignal(ctx, signal1)
				Expect(err).ToNot(HaveOccurred())
				existingRRName := response1.RemediationRequestName

				By("2. Update existing RR to Completed phase (simulate terminal state)")
				var rr remediationv1alpha1.RemediationRequest
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{
						Name:      existingRRName,
						Namespace: testNamespace,
					}, &rr)
				}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

				// Update status to Completed (terminal phase)
				rr.Status.OverallPhase = remediationv1alpha1.PhaseCompleted
				Expect(k8sClient.Status().Update(ctx, &rr)).To(Succeed())

				By("3. Process duplicate signal (same fingerprint)")
				time.Sleep(1 * time.Second)
				correlationID2 := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert2 := createPrometheusAlert(testNamespace, "ServiceDown", "critical", fingerprint, correlationID2)

				signal2, err := prometheusAdapter.Parse(ctx, alert2)
				Expect(err).ToNot(HaveOccurred())

				response2, err := gwServer.ProcessSignal(ctx, signal2)
				Expect(err).ToNot(HaveOccurred())

				// BR-GATEWAY-057: Should create NEW RR (not deduplicate to Completed RR)
				Expect(response2.Status).To(Equal("created"),
					"BR-GATEWAY-057: Must create new RR for signals when existing RR is in terminal phase")
				Expect(response2.RemediationRequestName).ToNot(Equal(existingRRName),
					"BR-GATEWAY-057: New RR must have different name than terminal RR")

				By("4. Verify NEW gateway.crd.created audit (not deduplication)")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.created"
				var crdCreatedEvent *ogenclient.AuditEvent
				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID2, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					crdCreatedEvent = &events[0]
					return true
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Validate it's a creation event, not deduplication
				Expect(crdCreatedEvent.EventType).To(Equal("gateway.crd.created"))
				payload, ok := extractGatewayPayload(crdCreatedEvent)
				Expect(ok).To(BeTrue())
				Expect(payload.Fingerprint).To(Equal(fingerprint))

				GinkgoWriter.Printf("✅ Phase-based dedup rejection validated: Old RR=%s (Completed), New RR=%s\n",
					existingRRName, response2.RemediationRequestName)
			})
		})
	})

	// Test ID: GW-INT-AUD-020
	// Scenario: Audit ID Uniqueness
	// BR: BR-GATEWAY-055
	// Section: 1.4.5
	Context("when processing multiple signals (GW-INT-AUD-020, BR-GATEWAY-055)", func() {
		var testNamespace string

		BeforeEach(func() {
			// Create unique test namespace for K8s resource isolation
			processID := GinkgoParallelProcess()
			testNamespace = fmt.Sprintf("gw-aud-unique-%d-%s", processID, uuid.New().String()[:8])

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

		It("[GW-INT-AUD-020] should assign globally unique audit IDs to all events", func() {
			By("1. Create and process 3 unique signals")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			var correlationIDs []string
			for i := 0; i < 3; i++ {
				fingerprint := fmt.Sprintf("%064x", uuid.New().ID()+uint32(i))
				correlationID := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix()+int64(i))
				correlationIDs = append(correlationIDs, correlationID)

				alert := createPrometheusAlert(testNamespace, fmt.Sprintf("TestAlert%d", i), "critical", fingerprint, correlationID)
				signal, err := prometheusAdapter.Parse(ctx, alert)
				Expect(err).ToNot(HaveOccurred())

				_, err = gwServer.ProcessSignal(ctx, signal)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(100 * time.Millisecond) // Small delay between signals
			}

			By("2. Query all audit events for the 3 correlation IDs")
			client, err := createOgenClient()
			Expect(err).ToNot(HaveOccurred())

			var allAuditIDs []string
			eventCount := 0

			// Wait for all events to be flushed (3 signals * 2 events = 6 total)
			Eventually(func() int {
				allAuditIDs = []string{} // Reset
				eventCount = 0
				for _, corrID := range correlationIDs {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &corrID, nil, nil)
					if err != nil {
						continue
					}
					for _, event := range events {
						// EventID is OptUUID - extract if present
						if eventID, ok := event.EventID.Get(); ok {
							allAuditIDs = append(allAuditIDs, eventID.String())
							eventCount++
						}
					}
				}
				return eventCount
			}, 15*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 6),
				"Should have at least 6 audit events (3 signals * 2 events each)")

			By("3. Validate all audit IDs are unique")
			uniqueIDs := make(map[string]bool)
			duplicates := []string{}

			for _, id := range allAuditIDs {
				if uniqueIDs[id] {
					duplicates = append(duplicates, id)
				}
				uniqueIDs[id] = true
			}

			Expect(duplicates).To(BeEmpty(),
				"BR-GATEWAY-055: All audit event IDs must be globally unique (found duplicates: %v)", duplicates)
			Expect(len(uniqueIDs)).To(Equal(len(allAuditIDs)),
				"BR-GATEWAY-055: Number of unique IDs must match total audit events")

			By("4. Validate audit ID format (ULID or UUID)")
			for _, id := range allAuditIDs {
				Expect(id).ToNot(BeEmpty(), "BR-GATEWAY-055: Audit ID must not be empty")
				Expect(len(id)).To(BeNumerically(">=", 20),
					"BR-GATEWAY-055: Audit ID must be at least 20 characters (ULID or UUID format)")
			}

			GinkgoWriter.Printf("✅ Audit ID uniqueness validated: %d events with %d unique IDs\n",
				eventCount, len(uniqueIDs))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Tests GW-INT-AUD-016, GW-INT-AUD-017, GW-INT-AUD-018, GW-INT-AUD-019
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-058: CRD Creation Failed Audit Events", func() {
		var (
			testNamespace string
		)

		BeforeEach(func() {
			// Create unique test namespace for K8s resource isolation
			processID := GinkgoParallelProcess()
			testNamespace = fmt.Sprintf("gw-aud-fail-%d-%s", processID, uuid.New().String()[:8])

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

		// Test ID: GW-INT-AUD-016
		// Scenario: K8s API Error Audit Event
		// BR: BR-GATEWAY-058
		// Section: 1.4.1
		Context("when K8s API fails (GW-INT-AUD-016, BR-GATEWAY-058)", func() {
			It("[GW-INT-AUD-016] should emit gateway.crd.failed audit event when K8s API fails", func() {
				By("1. Create signal and process with failing K8s client")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				fingerprint := fmt.Sprintf("%064x", uuid.New().ID())
				correlationID := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert := createPrometheusAlert(testNamespace, "TestK8sFailure", "critical", fingerprint, correlationID)

				signal, err := prometheusAdapter.Parse(ctx, alert)
				Expect(err).ToNot(HaveOccurred())

				// Create Gateway with ErrorInjectableK8sClient
				failingK8sClient := &ErrorInjectableK8sClient{
					Client:     k8sClient,
					failCreate: true,
					errorMsg:   "API server unavailable",
				}

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, failingK8sClient)
				Expect(err).ToNot(HaveOccurred())

				_, err = gwServer.ProcessSignal(ctx, signal)
				Expect(err).To(HaveOccurred(), "BR-GATEWAY-058: ProcessSignal should return error when K8s fails")

				By("2. Query gateway.crd.failed audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.failed"
				var failedEvent *ogenclient.AuditEvent

				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					failedEvent = &events[0]
					return true
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
					"BR-GATEWAY-058: Should emit gateway.crd.failed audit event")

				By("3. Validate gateway.crd.failed audit event fields")
				Expect(failedEvent.EventType).To(Equal("gateway.crd.failed"))
				Expect(failedEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeFailure))
				Expect(failedEvent.CorrelationID).To(Equal(fingerprint),
					"BR-GATEWAY-058: Correlation ID should be fingerprint when no RR created")

				By("4. Validate ErrorDetails in audit payload")
				payload, ok := extractGatewayPayload(failedEvent)
				Expect(ok).To(BeTrue(), "BR-GATEWAY-058: Must have GatewayAuditPayload")

				errorDetails, hasError := payload.ErrorDetails.Get()
				Expect(hasError).To(BeTrue(), "BR-GATEWAY-058: gateway.crd.failed must include ErrorDetails")
				Expect(errorDetails.Message).To(ContainSubstring("API server unavailable"),
					"BR-GATEWAY-058: Error message provides troubleshooting context")
				Expect(errorDetails.Component).To(Equal(ogenclient.ErrorDetailsComponentGateway),
					"BR-GATEWAY-058: Error component identifies source")

				GinkgoWriter.Printf("✅ K8s API failure audit validated: event_id=%v\n", failedEvent.EventID)
			})
		})

		// Test ID: GW-INT-AUD-017
		// Scenario: Error Type Classification in Audit
		// BR: BR-GATEWAY-058
		// Section: 1.4.2
		Context("when classifying error types (GW-INT-AUD-017, BR-GATEWAY-058)", func() {
			It("[GW-INT-AUD-017] should include error_type (transient vs permanent) in audit event", func() {
				By("1. Create signal and process with transient K8s error")
				prometheusAdapter := adapters.NewPrometheusAdapter()
				fingerprint := fmt.Sprintf("%064x", uuid.New().ID())
				correlationID := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				alert := createPrometheusAlert(testNamespace, "TestErrorType", "critical", fingerprint, correlationID)

				signal, err := prometheusAdapter.Parse(ctx, alert)
				Expect(err).ToNot(HaveOccurred())

				// Simulate transient error (503 Service Unavailable)
				failingK8sClient := &ErrorInjectableK8sClient{
					Client:     k8sClient,
					failCreate: true,
					errorMsg:   "503 Service Unavailable: API server temporarily unavailable",
				}

				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, failingK8sClient)
				Expect(err).ToNot(HaveOccurred())

				_, err = gwServer.ProcessSignal(ctx, signal)
				Expect(err).To(HaveOccurred())

				By("2. Query gateway.crd.failed audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.failed"
				var failedEvent *ogenclient.AuditEvent

				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					failedEvent = &events[0]
					return true
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("3. Validate ErrorDetails indicates transient error")
				payload, ok := extractGatewayPayload(failedEvent)
				Expect(ok).To(BeTrue())

				errorDetails, hasError := payload.ErrorDetails.Get()
				Expect(hasError).To(BeTrue(), "BR-GATEWAY-058: gateway.crd.failed must include ErrorDetails")

				// BR-GATEWAY-058: RetryPossible indicates transient vs permanent error
				Expect(errorDetails.RetryPossible).To(BeTrue(),
					"BR-GATEWAY-058: 503 errors are transient and retryable")
				Expect(errorDetails.Message).To(ContainSubstring("Service Unavailable"),
					"BR-GATEWAY-058: Error message provides context")

				GinkgoWriter.Printf("✅ Error type classification validated: retry_possible=%v\n", errorDetails.RetryPossible)
			})
		})

		// Test ID: GW-INT-AUD-018
		// Scenario: Retry Attempt Audit Events
		// BR: BR-GATEWAY-058
		// Section: 1.4.3
		Context("when retrying failed CRD creation (GW-INT-AUD-018, BR-GATEWAY-058)", func() {
			It("[GW-INT-AUD-018] should emit separate audit events for each retry attempt", func() {
				Skip("Deferred: Gateway ProcessSignal() does not implement retry logic yet - BR-GATEWAY-188 required")
				// Implementation Note:
				// - Gateway's ProcessSignal() currently fails immediately on K8s errors
				// - No retry loop exists in the current implementation
				// - Requires BR-GATEWAY-188 (Exponential Backoff & Retry) to be implemented first
				// - Once retry logic is added, this test will validate:
				//   1. Each retry attempt emits a separate gateway.crd.failed event
				//   2. Each event has unique EventID but same CorrelationID
				//   3. ErrorDetails includes retry count or attempt number
			})
		})

		// Test ID: GW-INT-AUD-019
		// Scenario: Circuit Breaker State in Audit
		// BR: BR-GATEWAY-058, BR-GATEWAY-093
		// Section: 1.4.4
		Context("when circuit breaker is open (GW-INT-AUD-019, BR-GATEWAY-058, BR-GATEWAY-093)", func() {
			It("[GW-INT-AUD-019] should emit gateway.crd.failed audit event with circuit breaker error details", func() {
				By("1. Create circuit breaker that starts in OPEN state (immediate fail-fast)")
				// Simulate circuit breaker being open by making it trip immediately
				prometheusAdapter := adapters.NewPrometheusAdapter()
				failingK8sClient := &ErrorInjectableK8sClient{
					Client:     k8sClient,
					failCreate: true,
					errorMsg:   "simulated K8s API unavailable for circuit breaker test",
				}

				By("2. Trip circuit breaker by making 10+ failed requests")
				gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
				gwServer, err := createGatewayServer(gatewayConfig, logger, failingK8sClient)
				Expect(err).ToNot(HaveOccurred())

				// Make 10 failing requests to trip circuit breaker (50% failure rate threshold)
				for i := 0; i < 10; i++ {
					fingerprint := fmt.Sprintf("cb-trip-%d-%064x", i, time.Now().UnixNano())
					correlationID := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
					alertPayload := createPrometheusAlert(testNamespace, "CircuitBreakerTripTest", "critical", fingerprint, correlationID)

					signal, err := prometheusAdapter.Parse(ctx, alertPayload)
					Expect(err).ToNot(HaveOccurred())

					// Expect failure (circuit breaker should trip after ~5 failures)
					_, _ = gwServer.ProcessSignal(ctx, signal)
				}

				By("3. Process signal that should hit open circuit breaker")
				// Circuit breaker should now be OPEN, causing immediate failure
				testFingerprint := fmt.Sprintf("%064x", uuid.New().ID())
				testCorrelationID := fmt.Sprintf("rr-%s-%d", uuid.New().String()[:12], time.Now().Unix())
				testAlertPayload := createPrometheusAlert(testNamespace, "CircuitBreakerOpenTest", "critical", testFingerprint, testCorrelationID)

				testSignal, err := prometheusAdapter.Parse(ctx, testAlertPayload)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, testSignal)
				Expect(err).To(HaveOccurred(), "ProcessSignal should fail when circuit breaker is open")
				Expect(response).To(BeNil())

				By("4. Query gateway.crd.failed audit event")
				client, err := createOgenClient()
				Expect(err).ToNot(HaveOccurred())

				eventType := "gateway.crd.failed"
				var failedEvent *ogenclient.AuditEvent

				// BR-GATEWAY-058: For failed CRD creation, correlation ID is the fingerprint (no RR name yet)
				fingerprintCorrelationID := testSignal.Fingerprint

				Eventually(func() bool {
					events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &fingerprintCorrelationID, &eventType, nil)
					if err != nil || len(events) == 0 {
						return false
					}
					failedEvent = &events[0]
					return true
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
					"BR-GATEWAY-058: Should emit gateway.crd.failed audit event")

				By("5. Validate audit event includes circuit breaker error details")
				Expect(failedEvent.EventType).To(Equal("gateway.crd.failed"))
				Expect(failedEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeFailure))
				Expect(failedEvent.CorrelationID).To(Equal(fingerprintCorrelationID))

				payload, ok := extractGatewayPayload(failedEvent)
				Expect(ok).To(BeTrue(), "Payload should be GatewayAuditPayload")
				Expect(payload.EventType).To(Equal("gateway.crd.failed"))

				// BR-GATEWAY-093: Verify circuit breaker error details
				errorDetails, ok := payload.ErrorDetails.Get()
				Expect(ok).To(BeTrue(), "ErrorDetails should be present for circuit breaker failure")

				Expect(errorDetails.Code).To(Equal("ERR_CIRCUIT_BREAKER_OPEN"),
					"Error code should indicate circuit breaker is open")
				Expect(errorDetails.Message).To(ContainSubstring("circuit breaker"),
					"Error message should mention circuit breaker")
				Expect(errorDetails.Message).To(ContainSubstring("fail-fast"),
					"Error message should explain fail-fast behavior")
				Expect(errorDetails.Component).To(Equal(ogenclient.ErrorDetailsComponentGateway),
					"Error component should be 'gateway'")
				Expect(errorDetails.RetryPossible).To(BeTrue(),
					"Circuit breaker errors are transient and retryable")

				GinkgoWriter.Printf("✅ Circuit breaker error details validated: %s (retry_possible: %v)\n",
					errorDetails.Code, errorDetails.RetryPossible)
			})
		})
	})
})

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HELPER FUNCTIONS FOR AUDIT EMISSION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Note: ErrorInjectableK8sClient is defined in 29_k8s_api_failure_integration_test.go
// and is shared across Gateway integration tests for K8s error injection

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
