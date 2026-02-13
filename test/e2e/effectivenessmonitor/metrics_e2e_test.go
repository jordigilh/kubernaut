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

package effectivenessmonitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E Tests: Metric Comparison with Real Prometheus
//
// These tests inject metrics into the real Prometheus deployed in the Kind cluster
// via the OTLP HTTP JSON endpoint and verify that the EM correctly scores them.
//
// Scenarios:
//   - E2E-EM-MC-001: Improvement detected -> metrics score > 0
//   - E2E-EM-MC-002: No change detected -> metrics score 0.0

var _ = Describe("EffectivenessMonitor Metric Comparison E2E Tests", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("em-mc-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================================================
	// E2E-EM-MC-001: Metrics Improvement
	// ========================================================================
	It("E2E-EM-MC-001: should produce metrics score > 0 when improvement is detected", func() {
		By("Injecting 'before' metrics (high CPU) into Prometheus")
		beforeTime := time.Now().Add(-5 * time.Minute)
		beforeMetrics := []infrastructure.TestMetric{
			{
				Name: "container_cpu_usage_seconds_total",
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"container": "workload",
				},
				Value:     0.85, // 85% CPU before remediation
				Timestamp: beforeTime,
			},
		}
		err := infrastructure.InjectMetrics(prometheusURL, beforeMetrics)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'before' metrics")

		By("Injecting 'after' metrics (low CPU) into Prometheus")
		afterTime := time.Now()
		afterMetrics := []infrastructure.TestMetric{
			{
				Name: "container_cpu_usage_seconds_total",
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"container": "workload",
				},
				Value:     0.25, // 25% CPU after remediation (improvement)
				Timestamp: afterTime,
			},
		}
		err = infrastructure.InjectMetrics(prometheusURL, afterMetrics)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'after' metrics")

		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-mc-improve")
		correlationID := uniqueName("corr-mc-improve")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
			withAlertManagerDisabled(), // Focus on metric comparison
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Verifying metrics were assessed with score > 0 (improvement)")
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(),
			"Metrics component should be assessed")
		Expect(ea.Status.Components.MetricsScore).ToNot(BeNil(),
			"Metrics score should be set")
		Expect(*ea.Status.Components.MetricsScore).To(BeNumerically(">", 0.0),
			"Metrics score should be > 0 when improvement is detected")
	})

	// ========================================================================
	// E2E-EM-MC-002: No Metrics Change
	// ========================================================================
	It("E2E-EM-MC-002: should produce metrics score 0.0 when no change is detected", func() {
		By("Injecting 'before' metrics into Prometheus")
		beforeTime := time.Now().Add(-5 * time.Minute)
		sameValue := 0.50 // Same CPU before and after
		beforeMetrics := []infrastructure.TestMetric{
			{
				Name: "container_cpu_usage_seconds_total",
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"container": "workload",
				},
				Value:     sameValue,
				Timestamp: beforeTime,
			},
		}
		err := infrastructure.InjectMetrics(prometheusURL, beforeMetrics)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'before' metrics")

		By("Injecting 'after' metrics with same values into Prometheus")
		afterTime := time.Now()
		afterMetrics := []infrastructure.TestMetric{
			{
				Name: "container_cpu_usage_seconds_total",
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"container": "workload",
				},
				Value:     sameValue, // Same value = no improvement
				Timestamp: afterTime,
			},
		}
		err = infrastructure.InjectMetrics(prometheusURL, afterMetrics)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'after' metrics")

		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-mc-nochange")
		correlationID := uniqueName("corr-mc-nochange")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
			withAlertManagerDisabled(), // Focus on metric comparison
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Verifying metrics were assessed with score 0.0 (no change)")
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(),
			"Metrics component should be assessed")
		Expect(ea.Status.Components.MetricsScore).ToNot(BeNil(),
			"Metrics score should be set")
		Expect(*ea.Status.Components.MetricsScore).To(Equal(0.0),
			"Metrics score should be 0.0 when no improvement is detected")
	})
})
