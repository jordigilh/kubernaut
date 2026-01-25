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

// Integration tests for BR-ORCH-044 (Operational Metrics)
// These tests validate that RO exposes Prometheus metrics for observability and SLO tracking.
//
// Business Requirement: BR-ORCH-044 (Operational Metrics)
// Design Decision: DD-METRICS-001 (Prometheus metrics exposure)
//
// Test Strategy (following AIAnalysis pattern):
// - Metrics verified via REGISTRY INSPECTION (not HTTP endpoint)
// - Direct access to controller-runtime's Prometheus registry
// - HTTP endpoint testing deferred to E2E tier (Kind cluster)
//
// Defense-in-Depth:
// - Unit tests: Mock metrics recording (limited validation)
// - Integration tests: Real Prometheus registry inspection (this file)
// - E2E tests: HTTP /metrics endpoint scraping and alerting rules

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

var _ = Describe("Operational Metrics Integration Tests (BR-ORCH-044)", Serial, Ordered, func() {
	// Integration Test Strategy (following AIAnalysis pattern):
	// - Metrics verified via REGISTRY INSPECTION (not HTTP endpoint)
	// - Direct access to controller-runtime's Prometheus registry
	// - HTTP endpoint testing deferred to E2E tier (Kind cluster)

	var testNamespace string

	BeforeEach(func() {
		testNamespace = createTestNamespace("metrics")
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	// Helper to gather all metrics from controller-runtime registry
	gatherMetrics := func() (map[string]*dto.MetricFamily, error) {
		families, err := ctrlmetrics.Registry.Gather()
		if err != nil {
			return nil, err
		}
		result := make(map[string]*dto.MetricFamily)
		for _, family := range families {
			result[family.GetName()] = family
		}
		return result, nil
	}

	// Helper to check if a metric exists in the registry
	metricExists := func(name string) bool {
		families, err := gatherMetrics()
		if err != nil {
			return false
		}
		_, exists := families[name]
		return exists
	}

	// Helper to get counter value with specific labels
	getCounterValue := func(name string, labels map[string]string) float64 {
		families, err := gatherMetrics()
		if err != nil {
			return -1
		}
		family, exists := families[name]
		if !exists {
			return -1
		}
		for _, m := range family.GetMetric() {
			labelMatch := true
			for wantKey, wantValue := range labels {
				found := false
				for _, l := range m.GetLabel() {
					if l.GetName() == wantKey && l.GetValue() == wantValue {
						found = true
						break
					}
				}
				if !found {
					labelMatch = false
					break
				}
			}
			if labelMatch {
				return m.GetCounter().GetValue()
			}
		}
		return -1
	}

	// Helper to check if histogram has samples with specific labels
	histogramHasSamples := func(name string, labels map[string]string) bool {
		families, err := gatherMetrics()
		if err != nil {
			return false
		}
		family, exists := families[name]
		if !exists {
			return false
		}
		for _, m := range family.GetMetric() {
			labelMatch := true
			for wantKey, wantValue := range labels {
				found := false
				for _, l := range m.GetLabel() {
					if l.GetName() == wantKey && l.GetValue() == wantValue {
						found = true
						break
					}
				}
				if !found {
					labelMatch = false
					break
				}
			}
			if labelMatch {
				return m.GetHistogram().GetSampleCount() > 0
			}
		}
		return false
	}

	Context("M-INT-1: reconcile_total Counter (BR-ORCH-044)", func() {
		It("should register and increment reconcile_total counter metric", func() {
			// Verify metric is registered
			Expect(metricExists(rometrics.MetricNameReconcileTotal)).To(BeTrue(),
				"reconcile_total should be registered in Prometheus registry")

			// Create RemediationRequest to trigger reconciliation
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-reconcile-total",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: GenerateTestFingerprint(testNamespace, "m-int-1"),
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Wait for reconciliation to occur
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Verify metric was incremented via registry inspection
			Eventually(func() float64 {
				return getCounterValue(rometrics.MetricNameReconcileTotal, map[string]string{
					"namespace": testNamespace,
				})
			}, timeout, interval).Should(BeNumerically(">", 0),
				"reconcile_total counter should be incremented after reconciliation")
		})
	})

	Context("M-INT-2: reconcile_duration Histogram (BR-ORCH-044)", func() {
		It("should register and record reconcile_duration histogram metric", func() {
			// Verify metric is registered
			Expect(metricExists(rometrics.MetricNameReconcileDuration)).To(BeTrue(),
				"reconcile_duration should be registered in Prometheus registry")

			// Create RemediationRequest to trigger reconciliation
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-reconcile-duration",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: GenerateTestFingerprint(testNamespace, "m-int-2"),
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Wait for reconciliation to occur
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Verify histogram recorded samples via registry inspection
			Eventually(func() bool {
				return histogramHasSamples(rometrics.MetricNameReconcileDuration, map[string]string{
					"namespace": testNamespace,
				})
			}, timeout, interval).Should(BeTrue(),
				"reconcile_duration histogram should have recorded samples")
		})
	})

	Context("M-INT-3: phase_transitions_total Counter (BR-ORCH-044)", func() {
		It("should register and increment phase_transitions_total counter metric", func() {
			// Verify metric is registered
			Expect(metricExists(rometrics.MetricNamePhaseTransitionsTotal)).To(BeTrue(),
				"phase_transitions_total should be registered in Prometheus registry")

			// Create RemediationRequest to trigger phase transitions
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-phase-transitions",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: GenerateTestFingerprint(testNamespace, "m-int-3"),
					SignalName:        "test-signal",
					Severity:          "critical",
					SignalType:        "test-type",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNamespace,
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Wait for phase transition (Pending → Processing)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Verify metric was incremented via registry inspection
			Eventually(func() float64 {
				return getCounterValue(rometrics.MetricNamePhaseTransitionsTotal, map[string]string{
					"namespace": testNamespace,
					"to_phase":  string(remediationv1.PhaseProcessing),
				})
			}, timeout, interval).Should(BeNumerically(">", 0),
				"phase_transitions_total counter should be incremented after phase transition")
		})
	})

	// M-INT-4: timeouts_total Counter
	// Note: This test was migrated to unit tests due to CreationTimestamp immutability
	// See: test/unit/remediationorchestrator/timeout_detector_test.go
	// Reason: Integration tests cannot manipulate CreationTimestamp to simulate timeouts
	//         without actual time passing. Unit tests can test the timeout detection logic
	//         directly with controlled timestamps.
	// Documentation: docs/handoff/RO_TIMEOUT_TESTS_MIGRATION_COMPLETE_DEC_24_2025.md
})
