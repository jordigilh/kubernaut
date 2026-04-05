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
		By("Injecting gauge series spanning 8 minutes into Prometheus (high rate before, low rate after)")
		// The EM query is sum(rate(container_cpu_usage_seconds_total{ns}[5m])).
		// rate()[5m] needs the underlying data to span enough time so that the
		// 5-minute sliding window captures different rate regions at early vs late
		// evaluation points in the QueryRange.
		//
		// High-rate phase (-8m to -4m): counter increases at ~1.0/s
		// Low-rate phase  (-4m to  0m): counter increases at ~0.2/s
		//
		// At early QueryRange eval points, rate()[5m] ≈ 0.84/s (high).
		// At late eval points, rate()[5m] ≈ 0.36/s (low).
		// Since CPU is LowerIsBetter, lower post = improvement → score > 0.
		now := time.Now()
		labels := map[string]string{
			"namespace": testNS,
			"pod":       "target-pod",
			"container": "workload",
		}
		var series []infrastructure.TestMetric
		// High rate: 9 points at 30s intervals from -8m to -4m (+30 per 30s = 1.0/s)
		for i := 0; i <= 8; i++ {
			series = append(series, infrastructure.TestMetric{
				Name:      "container_cpu_usage_seconds_total",
				Labels:    labels,
				Value:     100.0 + float64(i)*30.0,
				Timestamp: now.Add(-8*time.Minute + time.Duration(i)*30*time.Second),
			})
		}
		// Low rate: 8 points at 30s intervals from -3m30s to 0m (+6 per 30s = 0.2/s)
		for i := 1; i <= 8; i++ {
			series = append(series, infrastructure.TestMetric{
				Name:      "container_cpu_usage_seconds_total",
				Labels:    labels,
				Value:     340.0 + float64(i)*6.0,
				Timestamp: now.Add(-4*time.Minute + time.Duration(i)*30*time.Second),
			})
		}
		err := infrastructure.InjectMetrics(prometheusURL, series)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject metric series")

		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-mc-improve")
		correlationID := uniqueName("corr-mc-improve")

		By("Seeding workflowexecution.execution.started event (no_execution guard)")
		seedWorkflowStartedEvent(correlationID)
		By("Seeding workflowexecution.workflow.completed event (full scope, #573 G4)")
		seedWorkflowCompletedEvent(correlationID)

		// ADR-EM-001 v1.4: Component isolation is at EM config level, not per-EA.
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(name, eav1.PhaseCompleted)

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
		By("Injecting gauge series spanning 8 minutes with constant rate into Prometheus")
		// Constant rate of increase (0.5/s) throughout, so rate()[5m] returns
		// approximately the same value at every evaluation point → PreValue ≈ PostValue → score ≈ 0.
		now := time.Now()
		labels := map[string]string{
			"namespace": testNS,
			"pod":       "target-pod",
			"container": "workload",
		}
		var series []infrastructure.TestMetric
		// 17 points at 30s intervals from -8m to 0m (+15 per 30s = 0.5/s constant)
		for i := 0; i <= 16; i++ {
			series = append(series, infrastructure.TestMetric{
				Name:      "container_cpu_usage_seconds_total",
				Labels:    labels,
				Value:     100.0 + float64(i)*15.0,
				Timestamp: now.Add(-8*time.Minute + time.Duration(i)*30*time.Second),
			})
		}
		err := infrastructure.InjectMetrics(prometheusURL, series)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject metric series")

		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-mc-nochange")
		correlationID := uniqueName("corr-mc-nochange")

		By("Seeding workflowexecution.execution.started event (no_execution guard)")
		seedWorkflowStartedEvent(correlationID)
		By("Seeding workflowexecution.workflow.completed event (full scope, #573 G4)")
		seedWorkflowCompletedEvent(correlationID)

		// ADR-EM-001 v1.4: Component isolation is at EM config level, not per-EA.
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(name, eav1.PhaseCompleted)

		By("Verifying metrics were assessed with score 0.0 (no change)")
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(),
			"Metrics component should be assessed")
		Expect(ea.Status.Components.MetricsScore).ToNot(BeNil(),
			"Metrics score should be set")
		Expect(*ea.Status.Components.MetricsScore).To(BeNumerically("~", 0.0, 0.05),
			"Metrics score should be ~0.0 when no improvement is detected")
	})
})
