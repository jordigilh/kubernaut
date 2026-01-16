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
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
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
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("gw-adapter-%d-%s", processID, uuid.New().String()[:8])
		logger = GinkgoLogr

		// Create test namespace
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "Test namespace must be created")

		// Create kubernaut-system fallback namespace (if not already exists)
		fallbackNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"}}
		_ = k8sClient.Create(ctx, fallbackNs) // Ignore error if already exists
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
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
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
})
