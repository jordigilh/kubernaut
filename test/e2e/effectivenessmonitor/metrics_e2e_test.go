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
		By("Injecting cumulative counter series into Prometheus (high rate before, low rate after)")
		// container_cpu_usage_seconds_total is a counter in production. We inject it as
		// OTLP Sum (cumulative, monotonic) with multiple data points so that
		// sum(rate(...[5m])) produces distinct pre/post values for the EM scorer.
		now := time.Now()
		labels := map[string]string{
			"namespace": testNS,
			"pod":       "target-pod",
			"container": "workload",
		}
		// "Before" phase: cumulative counter increasing at ~0.85/s
		// "After" phase: cumulative counter increasing at ~0.25/s
		counterSeries := []infrastructure.TestMetric{
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 100.0, Timestamp: now.Add(-20 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 104.25, Timestamp: now.Add(-15 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 108.50, Timestamp: now.Add(-10 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 109.75, Timestamp: now.Add(-5 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 111.00, Timestamp: now, IsCounter: true},
		}
		err := infrastructure.InjectMetrics(prometheusURL, counterSeries)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject counter series")

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
		By("Injecting cumulative counter series with constant rate into Prometheus")
		// Same rate before and after: counter increasing at ~0.50/s throughout.
		now := time.Now()
		labels := map[string]string{
			"namespace": testNS,
			"pod":       "target-pod",
			"container": "workload",
		}
		counterSeries := []infrastructure.TestMetric{
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 100.0, Timestamp: now.Add(-20 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 102.50, Timestamp: now.Add(-15 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 105.00, Timestamp: now.Add(-10 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 107.50, Timestamp: now.Add(-5 * time.Second), IsCounter: true},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: 110.00, Timestamp: now, IsCounter: true},
		}
		err := infrastructure.InjectMetrics(prometheusURL, counterSeries)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject counter series")

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
