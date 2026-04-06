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
		By("Injecting gauge series into Prometheus (memory improvement)")
		// The EM queries both CPU (with rate()) and memory (raw sum()).
		// Injecting memory gauge data with a clear before→after drop ensures a
		// positive score regardless of rate() semantics on OTLP gauge data.
		//
		// Memory query: sum(container_memory_working_set_bytes{namespace="..."})
		// LowerIsBetter=true → lower PostValue = improvement.
		//
		// We inject 5 points: high memory early, dropping to low memory later.
		// Samples[0] (early) ≈ 500MB, Samples[len-1] (late) ≈ 200MB → score > 0.
		now := time.Now()
		labels := map[string]string{
			"namespace": testNS,
			"pod":       "target-pod",
			"container": "workload",
		}
		series := []infrastructure.TestMetric{
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 500_000_000, Timestamp: now.Add(-20 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 450_000_000, Timestamp: now.Add(-15 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 350_000_000, Timestamp: now.Add(-10 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 250_000_000, Timestamp: now.Add(-5 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 200_000_000, Timestamp: now},
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
		By("Injecting gauge series into Prometheus (stable memory)")
		// Stable memory across all data points → PreValue ≈ PostValue → score ≈ 0.
		now := time.Now()
		labels := map[string]string{
			"namespace": testNS,
			"pod":       "target-pod",
			"container": "workload",
		}
		series := []infrastructure.TestMetric{
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 300_000_000, Timestamp: now.Add(-20 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 300_000_000, Timestamp: now.Add(-15 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 300_000_000, Timestamp: now.Add(-10 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 300_000_000, Timestamp: now.Add(-5 * time.Second)},
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 300_000_000, Timestamp: now},
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
