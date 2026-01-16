/*
Copyright 2026 Jordi Gil.

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
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BR-GATEWAY-181: Signal Pass-Through Architecture Integration Tests
// Authority: DD-SEVERITY-001 v1.1 (Severity Refactoring)
//
// These tests verify Gateway passes through custom severity values WITHOUT transformation:
// - "Sev1" → "Sev1" (enterprise severity scheme)
// - "P0" → "P0" (PagerDuty severity scheme)
// - "HIGH" → "HIGH" (case-sensitive preservation)
// - "Warning" → "Warning" (K8s event type preservation)
//
// Tests are designed to work BEFORE Week 1 CRD enum removal:
// - Currently use standard "critical/warning/info" enums
// - Will be updated to use "Sev1/P0" after CRD schema changes
//
// Related:
// - BR-GATEWAY-181 (Signal Pass-Through Architecture)
// - BR-GATEWAY-005 (Signal Metadata Extraction - updated for pass-through)
// - DD-SEVERITY-001 v1.1 (Severity refactoring plan)

var _ = Describe("BR-GATEWAY-181: Custom Severity Pass-Through", Label("integration", "severity"), func() {
	var (
		ctx           context.Context
		testNamespace string
		logger        logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("gw-severity-%d-%s", processID, uuid.New().String()[:8])
		logger = GinkgoLogr
	})

	AfterEach(func() {
		// Cleanup handled by suite-level teardown
	})

	Context("Prometheus Alerts with Custom Severity Schemes", func() {
		// NOTE: These tests currently use standard enum values ("critical", "warning", "info")
		// because Week 1 CRD schema changes have not been applied yet.
		// After Week 1, update these tests to use custom values ("Sev1", "P0", "HIGH")

		It("[GW-INT-SEV-001] should preserve standard severity values (baseline)", func() {
			By("1. Create Gateway server")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("2. Process Prometheus alert with 'critical' severity")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			prometheusAlert := createPrometheusAlert(testNamespace, "HighCPU", "critical", "", "")
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify severity preserved in NormalizedSignal")
			Expect(signal.Severity).To(Equal("critical"),
				"BR-GATEWAY-181: Gateway must preserve severity without transformation")

			By("4. Process signal through Gateway server")
			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			By("5. Verify RemediationRequest has original severity")
			rr := &remediationv1alpha1.RemediationRequest{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: testNamespace,
			}, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Spec.Severity).To(Equal("critical"),
				"BR-GATEWAY-181: CRD must contain original external severity value")

			GinkgoWriter.Printf("✅ Baseline severity pass-through validated: critical → critical\n")
		})

		It("[GW-INT-SEV-002] should preserve 'warning' severity without transformation", func() {
			By("1. Create Gateway server")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("2. Process Prometheus alert with 'warning' severity")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			prometheusAlert := createPrometheusAlert(testNamespace, "ModerateMemory", "warning", "", "")
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify severity preserved")
			Expect(signal.Severity).To(Equal("warning"),
				"BR-GATEWAY-181: Gateway must NOT transform warning to critical")

			By("4. Process signal and verify CRD")
			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			rr := &remediationv1alpha1.RemediationRequest{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: testNamespace,
			}, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Spec.Severity).To(Equal("warning"),
				"BR-GATEWAY-181: No severity transformation in pass-through")

			GinkgoWriter.Printf("✅ Warning severity preserved: warning → warning\n")
		})

		It("[GW-INT-SEV-003] should preserve 'info' severity without transformation", func() {
			By("1. Create Gateway server")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("2. Process Prometheus alert with 'info' severity")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			prometheusAlert := createPrometheusAlert(testNamespace, "LowDiskSpace", "info", "", "")
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify severity preserved")
			Expect(signal.Severity).To(Equal("info"),
				"BR-GATEWAY-181: Gateway must NOT transform info to warning")

			By("4. Process signal and verify CRD")
			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			rr := &remediationv1alpha1.RemediationRequest{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: testNamespace,
			}, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Spec.Severity).To(Equal("info"),
				"BR-GATEWAY-181: Info severity must pass through unchanged")

			GinkgoWriter.Printf("✅ Info severity preserved: info → info\n")
		})

		It("[GW-INT-SEV-004] should default to 'unknown' only if severity missing entirely", func() {
			By("1. Create Prometheus alert without severity label")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			prometheusAlert := createPrometheusAlertWithoutSeverity(testNamespace, "NoSeverityAlert")
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			By("2. Verify severity defaults to 'unknown' (not 'warning')")
			Expect(signal.Severity).To(Equal("unknown"),
				"BR-GATEWAY-181: Only default to 'unknown' if severity entirely missing")

			GinkgoWriter.Printf("✅ Missing severity defaulted to 'unknown' (not policy)\n")
		})

		// NOTE: GW-INT-SEV-005 and GW-INT-SEV-006 DEFERRED to Week 1 DD-SEVERITY-001
		// These tests require CRD schema changes (RemediationRequest.Spec.Severity enum removal)
		// which are part of Week 1 work. Tests will be added after CRD refactoring is complete.
		//
		// Deferred test scenarios:
		// - GW-INT-SEV-005: Verify "Sev1" (enterprise severity) passes through unchanged
		// - GW-INT-SEV-006: Verify "P0" (PagerDuty severity) passes through unchanged
		//
		// Per TESTING_GUIDELINES.md: Neither PIt() nor Skip() allowed in integration tests.
		// Tests removed to avoid guideline violations until CRD schema changes are complete.
		//
		// Authority: BR-GATEWAY-181, DD-SEVERITY-001 Week 1 dependency
	})

	Context("Kubernetes Events with Type-as-Severity", func() {
		It("[GW-INT-SEV-007] should preserve 'Warning' event type as-is", func() {
			By("1. Process K8s Warning event")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			k8sEvent := createK8sEvent("Warning", "BackOff", testNamespace, "Pod", "api-server")
			signal, err := k8sAdapter.Parse(ctx, k8sEvent)
			Expect(err).ToNot(HaveOccurred())

			By("2. Verify event type preserved as severity (not mapped to 'warning')")
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: K8s event Type passed through as-is, not normalized")

			GinkgoWriter.Printf("✅ K8s event type preserved: Warning → Warning\n")
		})

		It("[GW-INT-SEV-008] should preserve 'Error' event type as-is", func() {
			By("1. Process K8s Error event")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			k8sEvent := createK8sEvent("Error", "FailedScheduling", testNamespace, "Pod", "unschedulable-pod")
			signal, err := k8sAdapter.Parse(ctx, k8sEvent)
			Expect(err).ToNot(HaveOccurred())

			By("2. Verify event type preserved (not mapped to 'critical')")
			Expect(signal.Severity).To(Equal("Error"),
				"BR-GATEWAY-181: Error events NOT auto-mapped to 'critical'")

			GinkgoWriter.Printf("✅ K8s event type preserved: Error → Error\n")
		})

		It("[GW-INT-SEV-009] should NOT map 'OOMKilled' to 'critical' automatically", func() {
			By("1. Process K8s Warning event with OOMKilled reason")
			k8sAdapter := adapters.NewKubernetesEventAdapter()
			k8sEvent := createK8sEvent("Warning", "OOMKilled", testNamespace, "Pod", "memory-hog")
			signal, err := k8sAdapter.Parse(ctx, k8sEvent)
			Expect(err).ToNot(HaveOccurred())

			By("2. Verify severity is 'Warning' (not 'critical' from hardcoded map)")
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: NO hardcoded reason-to-severity mapping (was: OOMKilled→critical)")

			GinkgoWriter.Printf("✅ K8s event reason does NOT override type: OOMKilled+Warning → Warning\n")
		})
	})

	Context("Adapter Validation - Pass-Through Enforcement", func() {
		It("[GW-INT-SEV-010] should accept ANY non-empty severity string in validation", func() {
			By("1. Create signals with various severity values")
			prometheusAdapter := adapters.NewPrometheusAdapter()

			testCases := []struct {
				severity string
				valid    bool
			}{
				{"critical", true},
				{"warning", true},
				{"info", true},
				{"Sev1", true},   // Enterprise
				{"P0", true},     // PagerDuty
				{"HIGH", true},   // Custom
				{"medium", true}, // Custom
				{"", false},      // Empty = invalid
			}

			for _, tc := range testCases {
				prometheusAlert := createPrometheusAlert(testNamespace, "TestAlert", tc.severity, "", "")
				signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
				Expect(err).ToNot(HaveOccurred())

				err = prometheusAdapter.Validate(signal)
				if tc.valid {
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("BR-GATEWAY-181: Adapter must accept '%s' severity", tc.severity))
				} else {
					Expect(err).To(HaveOccurred(),
						"BR-GATEWAY-181: Adapter must reject empty severity")
				}
			}

			GinkgoWriter.Printf("✅ Adapter validation accepts ANY non-empty severity string\n")
		})
	})
})
