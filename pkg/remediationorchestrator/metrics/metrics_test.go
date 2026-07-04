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

// Business Requirement: DD-METRICS-001 (dependency-injected metrics pattern)
// Purpose: Characterization tests for Metrics construction (CHAR-RO-1532) --
// establishes coverage for NewMetrics/NewMetricsWithRegistry before
// complexity-lint decomposition (Wave B, #1532).
package metrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Metrics Suite")
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	Expect(c.Write(&m)).To(Succeed())
	return m.GetCounter().GetValue()
}

func gaugeValue(g prometheus.Gauge) float64 {
	var m dto.Metric
	Expect(g.Write(&m)).To(Succeed())
	return m.GetGauge().GetValue()
}

var _ = Describe("NewMetricsWithRegistry (CHAR-RO-1532)", func() {
	var (
		reg *prometheus.Registry
		m   *metrics.Metrics
	)

	BeforeEach(func() {
		reg = prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(reg)
	})

	It("constructs all metric collectors non-nil", func() {
		Expect(m.ReconcileDurationSeconds).NotTo(BeNil())
		Expect(m.PhaseTransitionsTotal).NotTo(BeNil())
		Expect(m.ChildCRDCreationsTotal).NotTo(BeNil())
		Expect(m.NoActionNeededTotal).NotTo(BeNil())
		Expect(m.DuplicatesSkippedTotal).NotTo(BeNil())
		Expect(m.TimeoutsTotal).NotTo(BeNil())
		Expect(m.BlockedTotal).NotTo(BeNil())
		Expect(m.CurrentBlockedGauge).NotTo(BeNil())
		Expect(m.ApprovalDecisionsTotal).NotTo(BeNil())
		Expect(m.OverrideAppliedTotal).NotTo(BeNil())
		Expect(m.OverrideValidationRejectedTotal).NotTo(BeNil())
	})

	It("registers the pre-initialized collectors with the provided registry", func() {
		// Vec collectors only appear in Gather() once at least one labeled
		// child has been materialized. NewMetricsWithRegistry pre-initializes
		// child_crd_creations/no_action_needed/duplicates_skipped/timeouts/
		// blocked/current_blocked/approval_decisions with a zero-value child
		// (see DD-METRICS-001 "metrics visibility" requirement below), but
		// leaves reconcile_duration_seconds, phase_transitions_total, and the
		// override_* counters uninitialized until first observed/incremented.
		families, err := reg.Gather()
		Expect(err).NotTo(HaveOccurred())

		names := make(map[string]bool, len(families))
		for _, f := range families {
			names[f.GetName()] = true
		}
		Expect(names).To(HaveKey(metrics.MetricNameChildCRDCreationsTotal))
		Expect(names).To(HaveKey(metrics.MetricNameNoActionNeededTotal))
		Expect(names).To(HaveKey(metrics.MetricNameDuplicatesSkippedTotal))
		Expect(names).To(HaveKey(metrics.MetricNameTimeoutsTotal))
		Expect(names).To(HaveKey(metrics.MetricNameBlockedTotal))
		Expect(names).To(HaveKey(metrics.MetricNameCurrentBlocked))
		Expect(names).To(HaveKey(metrics.MetricNameApprovalDecisionsTotal))
		Expect(names).NotTo(HaveKey(metrics.MetricNameReconcileDuration))
		Expect(names).NotTo(HaveKey(metrics.MetricNamePhaseTransitionsTotal))
		Expect(names).NotTo(HaveKey(metrics.MetricNameOverrideAppliedTotal))
		Expect(names).NotTo(HaveKey(metrics.MetricNameOverrideValidationRejectedTotal))
	})

	It("makes the not-yet-observed collectors reachable and usable even though absent from Gather() until first use", func() {
		m.OverrideAppliedTotal.WithLabelValues("manual", "prod").Inc()
		families, err := reg.Gather()
		Expect(err).NotTo(HaveOccurred())

		names := make(map[string]bool, len(families))
		for _, f := range families {
			names[f.GetName()] = true
		}
		Expect(names).To(HaveKey(metrics.MetricNameOverrideAppliedTotal))
	})

	It("pre-initializes zero-value series so they appear before first increment", func() {
		Expect(counterValue(m.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", "default"))).To(Equal(0.0))
		Expect(counterValue(m.NoActionNeededTotal.WithLabelValues("default", "Completed"))).To(Equal(0.0))
		Expect(counterValue(m.DuplicatesSkippedTotal.WithLabelValues("default", "test_signal"))).To(Equal(0.0))
		Expect(counterValue(m.TimeoutsTotal.WithLabelValues("default", "Pending"))).To(Equal(0.0))
		Expect(counterValue(m.BlockedTotal.WithLabelValues("default", "ConsecutiveFailures"))).To(Equal(0.0))
		Expect(gaugeValue(m.CurrentBlockedGauge.WithLabelValues("default"))).To(Equal(0.0))
		Expect(counterValue(m.ApprovalDecisionsTotal.WithLabelValues("Approved", "default"))).To(Equal(0.0))
		Expect(counterValue(m.ApprovalDecisionsTotal.WithLabelValues("Rejected", "default"))).To(Equal(0.0))
		Expect(counterValue(m.ApprovalDecisionsTotal.WithLabelValues("Expired", "default"))).To(Equal(0.0))
	})

	It("RecordApprovalDecision increments the approval_decisions_total counter for the given decision/namespace", func() {
		m.RecordApprovalDecision("Approved", "prod")
		m.RecordApprovalDecision("Approved", "prod")
		m.RecordApprovalDecision("Rejected", "prod")

		Expect(counterValue(m.ApprovalDecisionsTotal.WithLabelValues("Approved", "prod"))).To(Equal(2.0))
		Expect(counterValue(m.ApprovalDecisionsTotal.WithLabelValues("Rejected", "prod"))).To(Equal(1.0))
	})
})

var _ = Describe("NewMetrics (CHAR-RO-1532)", func() {
	It("constructs and registers with controller-runtime's global registry without panicking", func() {
		Expect(func() { metrics.NewMetrics() }).NotTo(Panic())
	})
})
