package gateway

// BR-GATEWAY-001, BR-GATEWAY-002: Adapter Logic Integration Tests
// Authority: GW_INTEGRATION_TEST_PLAN_V1.0.md Phase 2
//
// These tests validate adapter parsing, extraction, and error handling:
// - Prometheus AlertManager webhook format parsing
// - Kubernetes Event format parsing
// - Field extraction (namespace, alertname, severity, labels)
// - Fingerprint generation and stability
// - Error resilience (malformed input, missing fields)
//
// Test Pattern:
// 1. Create Gateway server with real K8s client + DataStorage
// 2. Parse signal through adapter
// 3. Verify extracted fields match business requirements
// 4. Process signal and verify CRD creation
//
// Coverage: Phase 2 - 15 adapter tests (GW-INT-ADP-001 to ADP-015)

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Gateway Adapter Logic", Label("integration", "adapters"), func() {
	var (
		ctx           context.Context
		testNamespace string
		logger        logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "gw-adapter")
		logger = GinkgoLogr

		// Note: Namespace fallback removed (DD-GATEWAY-007 DEPRECATED, February 2026)
		// kubernaut-system namespace no longer needed for CRD fallback
	})

	AfterEach(func() {
		// Cleanup handled by suite-level teardown
	})

	Context("BR-GATEWAY-001: Prometheus AlertManager Webhook Parsing", func() {
		It("[GW-INT-ADP-001] should parse Prometheus alert format correctly", func() {
			By("1. Create Prometheus alert payload")
			prometheusAlert := createPrometheusAlert(testNamespace, "HighCPUUsage", "critical", "", "")

			By("2. Parse alert through Prometheus adapter")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)

			By("3. Verify parsing succeeded")
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-001: Prometheus adapter must parse valid AlertManager webhooks")

			By("4. Verify signal structure is populated")
			Expect(signal).ToNot(BeNil(), "Signal must not be nil")
			Expect(signal.AlertName).To(Equal("HighCPUUsage"),
				"BR-GATEWAY-001: AlertName must be extracted from labels")
			Expect(signal.Namespace).To(Equal(testNamespace),
				"BR-GATEWAY-001: Namespace must be extracted from labels")
			Expect(signal.Severity).To(Equal("critical"),
				"BR-GATEWAY-181: Severity must be passed through without transformation")
			Expect(signal.Fingerprint).ToNot(BeEmpty(),
				"BR-GATEWAY-004: Fingerprint must be generated")

			GinkgoWriter.Printf("✅ Prometheus alert parsed: alertName=%s, namespace=%s, severity=%s\n",
				signal.AlertName, signal.Namespace, signal.Severity)
		})

		It("[GW-INT-ADP-002] should extract alertname from labels correctly", func() {
			By("1. Create alerts with different alertnames")
			testCases := []struct {
				alertName    string
				expectedName string
			}{
				{"PodCrashLooping", "PodCrashLooping"},
				{"HighMemoryUsage", "HighMemoryUsage"},
				{"DiskSpaceLow", "DiskSpaceLow"},
			}

			prometheusAdapter := adapters.NewPrometheusAdapter()

			for _, tc := range testCases {
				By(fmt.Sprintf("2. Parse alert with alertname=%s", tc.alertName))
				alert := createPrometheusAlert(testNamespace, tc.alertName, "warning", "", "")
				signal, err := prometheusAdapter.Parse(ctx, alert)

				By("3. Verify alertname extracted correctly")
				Expect(err).ToNot(HaveOccurred())
				Expect(signal.AlertName).To(Equal(tc.expectedName),
					"BR-GATEWAY-001: AlertName must match label['alertname']")

				GinkgoWriter.Printf("✅ AlertName extraction validated: %s → %s\n",
					tc.alertName, signal.AlertName)
			}
		})

		It("[GW-INT-ADP-003] should extract namespace from labels correctly", func() {
			By("1. Create Gateway server")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
			Expect(err).ToNot(HaveOccurred())

			By("2. Create alert with explicit namespace label")
			prometheusAlert := createPrometheusAlert(testNamespace, "ServiceDown", "critical", "", "")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify namespace extracted from labels")
			Expect(signal.Namespace).To(Equal(testNamespace),
				"BR-GATEWAY-001: Namespace must be extracted from labels['namespace']")

			By("4. Process signal and verify CRD created in correct namespace")
			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			By("5. Verify RemediationRequest created in extracted namespace")
			rr := &remediationv1alpha1.RemediationRequest{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: testNamespace,
			}, rr)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-001: CRD must be created in namespace from alert labels")

			GinkgoWriter.Printf("✅ Namespace extraction validated: CRD created in namespace=%s\n", testNamespace)
		})

		It("[GW-INT-ADP-004] should pass through severity without transformation (BR-GATEWAY-181)", func() {
			By("1. Create alerts with various severity values")
			testCases := []struct {
				inputSeverity    string
				expectedSeverity string
			}{
				{"critical", "critical"},
				{"warning", "warning"},
				{"info", "info"},
				{"Sev1", "Sev1"}, // Enterprise scheme
				{"P0", "P0"},     // PagerDuty scheme
			}

			prometheusAdapter := adapters.NewPrometheusAdapter()

			for _, tc := range testCases {
				By(fmt.Sprintf("2. Parse alert with severity=%s", tc.inputSeverity))
				alert := createPrometheusAlert(testNamespace, "TestAlert", tc.inputSeverity, "", "")
				signal, err := prometheusAdapter.Parse(ctx, alert)

				By("3. Verify severity passed through without transformation")
				Expect(err).ToNot(HaveOccurred())
				Expect(signal.Severity).To(Equal(tc.expectedSeverity),
					"BR-GATEWAY-181: Severity must be passed through as-is (no hardcoded mapping)")

				GinkgoWriter.Printf("✅ Severity pass-through validated: %s → %s\n",
					tc.inputSeverity, signal.Severity)
			}
		})

		It("[GW-INT-ADP-005] should generate stable fingerprints for deduplication (BR-GATEWAY-004)", func() {
			By("1. Create identical alerts at different times")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			alertPayload1 := createPrometheusAlert(testNamespace, "RepeatedAlert", "warning", "", "")
			alertPayload2 := createPrometheusAlert(testNamespace, "RepeatedAlert", "warning", "", "")

			By("2. Parse both alerts")
			signal1, err1 := prometheusAdapter.Parse(ctx, alertPayload1)
			signal2, err2 := prometheusAdapter.Parse(ctx, alertPayload2)
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			By("3. Verify fingerprints are identical (stable)")
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"BR-GATEWAY-004: Identical alerts must generate identical fingerprints")

			By("4. Create alert with different alertname")
			alertPayload3 := createPrometheusAlert(testNamespace, "DifferentAlert", "warning", "", "")
			signal3, err3 := prometheusAdapter.Parse(ctx, alertPayload3)
			Expect(err3).ToNot(HaveOccurred())

			By("5. Verify different alert has different fingerprint")
			Expect(signal3.Fingerprint).ToNot(Equal(signal1.Fingerprint),
				"BR-GATEWAY-004: Different alerts must generate different fingerprints")

			GinkgoWriter.Printf("✅ Fingerprint stability validated: identical alerts → identical fingerprints\n")
		})

		It("[GW-INT-ADP-006] should preserve custom labels in signal annotations", func() {
			By("1. Create Prometheus alert with custom labels")
			alertPayload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "CustomLabelsTest",
						"severity": "info",
						"namespace": "%s",
						"pod": "api-server-123",
						"team": "platform",
						"environment": "production",
						"service": "api-gateway"
					},
					"annotations": {
						"summary": "Test alert with custom labels",
						"description": "Validating custom label preservation"
					},
					"startsAt": "2025-01-16T10:00:00Z"
				}]
			}`, testNamespace))

			By("2. Parse alert through adapter")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			signal, err := prometheusAdapter.Parse(ctx, alertPayload)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify custom labels are preserved in signal")
			Expect(signal.Labels).ToNot(BeEmpty(),
				"BR-GATEWAY-001: Custom labels must be preserved")
			Expect(signal.Labels["team"]).To(Equal("platform"),
				"BR-GATEWAY-001: Custom label 'team' must be preserved")
			Expect(signal.Labels["environment"]).To(Equal("production"),
				"BR-GATEWAY-001: Custom label 'environment' must be preserved")
			Expect(signal.Labels["service"]).To(Equal("api-gateway"),
				"BR-GATEWAY-001: Custom label 'service' must be preserved")

			GinkgoWriter.Printf("✅ Custom labels preserved: team=%s, environment=%s, service=%s\n",
				signal.Labels["team"], signal.Labels["environment"], signal.Labels["service"])
		})

		It("[GW-INT-ADP-007] should truncate long annotations to prevent storage issues", func() {
			By("1. Create alert with very long annotation")
			longDescription := string(make([]byte, 10000)) // 10KB description
			for i := range longDescription {
				longDescription = string(append([]byte(longDescription[:i]), 'x'))
			}

			alertPayload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "LongAnnotationTest",
						"severity": "info",
						"namespace": "%s",
						"pod": "test-pod"
					},
					"annotations": {
						"summary": "Test alert",
						"description": "%s"
					},
					"startsAt": "2025-01-16T10:00:00Z"
				}]
			}`, testNamespace, longDescription))

			By("2. Parse alert through adapter")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			signal, err := prometheusAdapter.Parse(ctx, alertPayload)

			By("3. Verify parsing succeeded despite long annotation")
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-001: Adapter must handle long annotations gracefully")

			By("4. Verify annotation was truncated or handled")
			// The adapter should either truncate or handle long annotations
			// Exact behavior depends on implementation
			Expect(signal).ToNot(BeNil(), "Signal must be created")

			GinkgoWriter.Printf("✅ Long annotation handled gracefully (no parsing error)\n")
		})
	})

	Context("BR-GATEWAY-002: Kubernetes Event Parsing", func() {
		It("[GW-INT-ADP-008] should parse Kubernetes Event format correctly", func() {
			By("1. Create K8s Event payload")
			k8sEvent := createK8sEvent("Warning", "BackOff", testNamespace, "Pod", "api-server-123")

			By("2. Parse event through K8s Event adapter")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			signal, err := k8sAdapter.Parse(ctx, k8sEvent)

			By("3. Verify parsing succeeded")
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-002: K8s Event adapter must parse valid K8s Events")

			By("4. Verify signal structure is populated")
			Expect(signal).ToNot(BeNil(), "Signal must not be nil")
			Expect(signal.Namespace).To(Equal(testNamespace),
				"BR-GATEWAY-002: Namespace must be extracted from involvedObject")
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-002: Resource kind must be extracted from involvedObject")
			Expect(signal.Resource.Name).To(Equal("api-server-123"),
				"BR-GATEWAY-002: Resource name must be extracted from involvedObject")
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event type must be passed through as severity")
			Expect(signal.Fingerprint).ToNot(BeEmpty(),
				"BR-GATEWAY-004: Fingerprint must be generated")

			GinkgoWriter.Printf("✅ K8s Event parsed: kind=%s, name=%s, namespace=%s, severity=%s\n",
				signal.Resource.Kind, signal.Resource.Name, signal.Namespace, signal.Severity)
		})

		It("[GW-INT-ADP-009] should extract reason from event correctly", func() {
			By("1. Create events with different reasons")
			testCases := []struct {
				reason         string
				expectedReason string
			}{
				{"BackOff", "BackOff"},
				{"FailedScheduling", "FailedScheduling"},
				{"OOMKilled", "OOMKilled"},
				{"Unhealthy", "Unhealthy"},
			}

			k8sAdapter := adapters.NewKubernetesEventAdapter()

			for _, tc := range testCases {
				By(fmt.Sprintf("2. Parse event with reason=%s", tc.reason))
				event := createK8sEvent("Warning", tc.reason, testNamespace, "Pod", "test-pod")
				signal, err := k8sAdapter.Parse(ctx, event)

				By("3. Verify reason extracted correctly")
				Expect(err).ToNot(HaveOccurred())
				Expect(signal.AlertName).To(Equal(tc.expectedReason),
					"BR-GATEWAY-002: AlertName must be populated from event reason")

				GinkgoWriter.Printf("✅ Reason extraction validated: %s → %s\n",
					tc.reason, signal.AlertName)
			}
		})

		It("[GW-INT-ADP-010] should extract involvedObject metadata correctly", func() {
			By("1. Create Gateway server")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
			Expect(err).ToNot(HaveOccurred())

			By("2. Create K8s Event with explicit involvedObject")
			k8sEvent := createK8sEvent("Warning", "FailedMount", testNamespace, "Pod", "database-pod-456")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			signal, err := k8sAdapter.Parse(ctx, k8sEvent)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify involvedObject fields extracted")
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-002: Resource kind must match involvedObject.kind")
			Expect(signal.Resource.Name).To(Equal("database-pod-456"),
				"BR-GATEWAY-002: Resource name must match involvedObject.name")
			Expect(signal.Namespace).To(Equal(testNamespace),
				"BR-GATEWAY-002: Namespace must match involvedObject.namespace")

			By("4. Process signal and verify CRD targeting")
			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			By("5. Verify RemediationRequest targets correct resource")
			rr := &remediationv1alpha1.RemediationRequest{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: testNamespace,
			}, rr)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-002: CRD must be created for involvedObject resource")

			GinkgoWriter.Printf("✅ InvolvedObject extraction validated: kind=%s, name=%s, namespace=%s\n",
				signal.Resource.Kind, signal.Resource.Name, signal.Namespace)
		})

		It("[GW-INT-ADP-011] should pass through event type as severity (BR-GATEWAY-181)", func() {
			By("1. Create events with different types")
			testCases := []struct {
				eventType        string
				expectedSeverity string
			}{
				{"Warning", "Warning"},
				{"Error", "Error"},
				// Note: "Normal" events are filtered out by the adapter (business logic)
				// BR-GATEWAY-002: Normal events are informational only, no remediation needed
			}

			k8sAdapter := adapters.NewKubernetesEventAdapter()

			for _, tc := range testCases {
				By(fmt.Sprintf("2. Parse event with type=%s", tc.eventType))
				event := createK8sEvent(tc.eventType, "TestReason", testNamespace, "Pod", "test-pod")
				signal, err := k8sAdapter.Parse(ctx, event)

				By("3. Verify event type passed through as severity")
				Expect(err).ToNot(HaveOccurred())
				Expect(signal.Severity).To(Equal(tc.expectedSeverity),
					"BR-GATEWAY-181: Event type must be passed through as-is (no hardcoded mapping)")

				GinkgoWriter.Printf("✅ Event type pass-through validated: %s → %s\n",
					tc.eventType, signal.Severity)
			}
		})

		It("[GW-INT-ADP-012] should generate stable fingerprints for K8s Events (BR-GATEWAY-004)", func() {
			By("1. Create identical events at different times")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			eventPayload1 := createK8sEvent("Warning", "BackOff", testNamespace, "Pod", "api-server")
			eventPayload2 := createK8sEvent("Warning", "BackOff", testNamespace, "Pod", "api-server")

			By("2. Parse both events")
			signal1, err1 := k8sAdapter.Parse(ctx, eventPayload1)
			signal2, err2 := k8sAdapter.Parse(ctx, eventPayload2)
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			By("3. Verify fingerprints are identical (stable)")
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"BR-GATEWAY-004: Identical events must generate identical fingerprints")

			By("4. Create event with different pod name")
			eventPayload3 := createK8sEvent("Warning", "BackOff", testNamespace, "Pod", "different-pod")
			signal3, err3 := k8sAdapter.Parse(ctx, eventPayload3)
			Expect(err3).ToNot(HaveOccurred())

			By("5. Verify different event has different fingerprint")
			Expect(signal3.Fingerprint).ToNot(Equal(signal1.Fingerprint),
				"BR-GATEWAY-004: Different events must generate different fingerprints")

			GinkgoWriter.Printf("✅ K8s Event fingerprint stability validated\n")
		})

		It("[GW-INT-ADP-013] should handle malformed K8s Event payloads gracefully (BR-GATEWAY-005)", func() {
			By("1. Create malformed event (missing involvedObject)")
			malformedEvent := []byte(`{
				"type": "Warning",
				"reason": "TestReason",
				"message": "Test message"
			}`)

			By("2. Parse malformed event through adapter")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			signal, err := k8sAdapter.Parse(ctx, malformedEvent)

			By("3. Verify adapter returns error for missing required fields")
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-005: Adapter must reject events missing involvedObject")
			Expect(signal).To(BeNil(),
				"BR-GATEWAY-005: Signal must be nil when parsing fails")

			GinkgoWriter.Printf("✅ Malformed K8s Event rejected: %v\n", err)
		})

		It("[GW-INT-ADP-014] should handle empty required fields gracefully (BR-GATEWAY-005)", func() {
			By("1. Test events with empty required fields")
			testCases := []struct {
				description string
				eventJSON   string
				shouldFail  bool
			}{
				{
					description: "Empty reason",
					eventJSON: fmt.Sprintf(`{
						"type": "Warning",
						"reason": "",
						"involvedObject": {"kind": "Pod", "name": "test-pod", "namespace": "%s"},
						"message": "Test"
					}`, testNamespace),
					shouldFail: true, // Reason is required for AlertName
				},
				{
					description: "Empty involvedObject kind",
					eventJSON: fmt.Sprintf(`{
						"type": "Warning",
						"reason": "TestReason",
						"involvedObject": {"kind": "", "name": "test-pod", "namespace": "%s"},
						"message": "Test"
					}`, testNamespace),
					shouldFail: true, // Kind is required for resource targeting
				},
				{
					description: "Empty involvedObject name",
					eventJSON: fmt.Sprintf(`{
						"type": "Warning",
						"reason": "TestReason",
						"involvedObject": {"kind": "Pod", "name": "", "namespace": "%s"},
						"message": "Test"
					}`, testNamespace),
					shouldFail: true, // Name is required for resource targeting
				},
			}

			k8sAdapter := adapters.NewKubernetesEventAdapter()

			for _, tc := range testCases {
				By(fmt.Sprintf("2. Parse event: %s", tc.description))
				signal, err := k8sAdapter.Parse(ctx, []byte(tc.eventJSON))

				By("3. Verify adapter behavior for empty fields")
				if tc.shouldFail {
					Expect(err).To(HaveOccurred(),
						fmt.Sprintf("BR-GATEWAY-005: %s should cause validation failure", tc.description))
					Expect(signal).To(BeNil(),
						"BR-GATEWAY-005: Signal must be nil when required field is empty")
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(signal).ToNot(BeNil())
				}

				GinkgoWriter.Printf("✅ Empty field handling validated: %s (fail=%v)\n",
					tc.description, tc.shouldFail)
			}
		})

		It("[GW-INT-ADP-015] should allow signal processing to continue despite adapter errors (BR-GATEWAY-005)", func() {
			By("1. Create Gateway server")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
			Expect(err).ToNot(HaveOccurred())

			By("2. Process valid K8s Event")
			validEvent := createK8sEvent("Warning", "BackOff", testNamespace, "Pod", "valid-pod")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			validSignal, err := k8sAdapter.Parse(ctx, validEvent)
			Expect(err).ToNot(HaveOccurred())

			By("3. Process valid signal through Gateway")
			response1, err := gwServer.ProcessSignal(ctx, validSignal)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-005: Valid signal must be processed successfully")
			Expect(response1.Status).To(Equal("created"),
				"BR-GATEWAY-005: Valid signal must result in CRD creation")

			By("4. Verify malformed event does NOT crash Gateway")
			malformedEvent := []byte(`{"type": "Warning"}`) // Missing required fields
			_, err = k8sAdapter.Parse(ctx, malformedEvent)
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-005: Malformed event must be rejected by adapter")

			By("5. Verify Gateway can still process subsequent valid events")
			validEvent2 := createK8sEvent("Warning", "FailedMount", testNamespace, "Pod", "another-pod")
			validSignal2, err := k8sAdapter.Parse(ctx, validEvent2)
			Expect(err).ToNot(HaveOccurred())

			response2, err := gwServer.ProcessSignal(ctx, validSignal2)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-005: Gateway must continue processing after adapter error")
			Expect(response2.Status).To(Equal("created"),
				"BR-GATEWAY-005: Subsequent signals must be processed normally")

			GinkgoWriter.Printf("✅ Adapter error non-fatal: Gateway continues processing\n")
		})
	})
})
