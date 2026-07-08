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

package controller

import (
	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention
)

// Issue #193 / DD-EM-005: EM cluster-scoped resource assessment.
//
// buildNodeMetricQuerySpecs and buildPVMetricQuerySpecs are the deterministic,
// kube-state-metrics-backed PromQL query builders for cluster-scoped targets
// (Node, PersistentVolume). clusterScopedAlertLabelKey maps a cluster-scoped
// Kind to the label key AlertManager alerts carry for that resource, so
// assessAlert can populate alert.AlertContext.AlertLabels for precise matching.
var _ = Describe("cluster-scoped metric/alert-label builders (BR-EM-003, BR-EM-002, DD-EM-005)", func() {

	// UT-EM-193-001
	Describe("buildNodeMetricQuerySpecs", func() {
		It("produces Ready/MemoryPressure/DiskPressure PromQL specs keyed by node name, all LowerIsBetter", func() {
			specs := buildNodeMetricQuerySpecs("worker-1")

			Expect(specs).To(HaveLen(3))
			for _, s := range specs {
				Expect(s.LowerIsBetter).To(BeTrue(),
					"a firing Node condition (NotReady/MemoryPressure/DiskPressure) is 1, so lower is better after remediation")
				Expect(s.Query).To(ContainSubstring(`node="worker-1"`))
				Expect(s.Query).To(ContainSubstring("kube_node_status_condition"))
			}

			names := make([]string, len(specs))
			for i, s := range specs {
				names[i] = s.Name
			}
			Expect(names).To(ConsistOf(
				"kube_node_status_condition_ready",
				"kube_node_status_condition_memorypressure",
				"kube_node_status_condition_diskpressure",
			))
		})
	})

	// UT-EM-193-002
	Describe("buildPVMetricQuerySpecs", func() {
		It("produces Failed/Pending phase specs and a usage-join ratio spec keyed by PV name", func() {
			specs := buildPVMetricQuerySpecs("pvc-abc123")

			Expect(specs).To(HaveLen(3))

			byName := make(map[string]metricQuerySpec, len(specs))
			for _, s := range specs {
				byName[s.Name] = s
			}

			Expect(byName).To(HaveKey("kube_persistentvolume_status_phase_failed"))
			Expect(byName["kube_persistentvolume_status_phase_failed"].LowerIsBetter).To(BeTrue())
			Expect(byName["kube_persistentvolume_status_phase_failed"].Query).To(ContainSubstring(`persistentvolume="pvc-abc123"`))
			Expect(byName["kube_persistentvolume_status_phase_failed"].Query).To(ContainSubstring(`phase="Failed"`))

			Expect(byName).To(HaveKey("kube_persistentvolume_status_phase_pending"))
			Expect(byName["kube_persistentvolume_status_phase_pending"].LowerIsBetter).To(BeTrue())
			Expect(byName["kube_persistentvolume_status_phase_pending"].Query).To(ContainSubstring(`phase="Pending"`))

			Expect(byName).To(HaveKey("kubelet_volume_stats_used_bytes_ratio"))
			usageSpec := byName["kubelet_volume_stats_used_bytes_ratio"]
			Expect(usageSpec.LowerIsBetter).To(BeTrue())
			Expect(usageSpec.Query).To(ContainSubstring("kubelet_volume_stats_used_bytes"))
			Expect(usageSpec.Query).To(ContainSubstring(`kube_persistentvolume_claim_ref{persistentvolume="pvc-abc123"}`))
		})
	})

	// UT-EM-193-003, UT-EM-193-004, UT-EM-193-005
	Describe("clusterScopedAlertLabelKey", func() {
		It("maps Node to the kube-state-metrics 'node' label key", func() {
			key, ok := clusterScopedAlertLabelKey("Node")
			Expect(ok).To(BeTrue())
			Expect(key).To(Equal("node"))
		})

		It("maps PersistentVolume to the kube-state-metrics 'persistentvolume' label key", func() {
			key, ok := clusterScopedAlertLabelKey("PersistentVolume")
			Expect(ok).To(BeTrue())
			Expect(key).To(Equal("persistentvolume"))
		})

		It("returns ok=false for an unrecognized cluster-scoped Kind", func() {
			key, ok := clusterScopedAlertLabelKey("SomeUnknownKind")
			Expect(ok).To(BeFalse())
			Expect(key).To(BeEmpty())
		})
	})

	// UT-EM-193-008, UT-EM-193-009, UT-EM-193-010
	// populateMetricsAssessResult must map the 6 new cluster-scoped
	// metricQuerySpec.Name values into metricsAssessResult fields so the
	// effectiveness.metrics.assessed audit event's metric_deltas sub-object
	// is populated for Node/PersistentVolume assessments (Issue #193 audit
	// gap, DD-EM-005 v1.1).
	Describe("populateMetricsAssessResult (cluster-scoped mapping)", func() {
		It("UT-EM-193-008: maps Node condition query results into Node*  fields", func() {
			mr := &metricsAssessResult{}
			results := []metricQueryResult{
				{Spec: metricQuerySpec{Name: "kube_node_status_condition_ready"}, PreValue: 1.0, PostValue: 0.0, Available: true},
				{Spec: metricQuerySpec{Name: "kube_node_status_condition_memorypressure"}, PreValue: 1.0, PostValue: 0.0, Available: true},
				{Spec: metricQuerySpec{Name: "kube_node_status_condition_diskpressure"}, PreValue: 0.0, PostValue: 0.0, Available: true},
			}

			populateMetricsAssessResult(mr, results)

			Expect(mr.NodeNotReadyBefore).ToNot(BeNil())
			Expect(*mr.NodeNotReadyBefore).To(BeNumerically("~", 1.0, 0.001))
			Expect(mr.NodeNotReadyAfter).ToNot(BeNil())
			Expect(*mr.NodeNotReadyAfter).To(BeNumerically("~", 0.0, 0.001))

			Expect(mr.NodeMemoryPressureBefore).ToNot(BeNil())
			Expect(*mr.NodeMemoryPressureBefore).To(BeNumerically("~", 1.0, 0.001))
			Expect(mr.NodeMemoryPressureAfter).ToNot(BeNil())

			Expect(mr.NodeDiskPressureBefore).ToNot(BeNil())
			Expect(mr.NodeDiskPressureAfter).ToNot(BeNil())
		})

		It("UT-EM-193-009: maps PersistentVolume phase/usage query results into PV* fields", func() {
			mr := &metricsAssessResult{}
			results := []metricQueryResult{
				{Spec: metricQuerySpec{Name: "kube_persistentvolume_status_phase_failed"}, PreValue: 1.0, PostValue: 0.0, Available: true},
				{Spec: metricQuerySpec{Name: "kube_persistentvolume_status_phase_pending"}, PreValue: 0.0, PostValue: 0.0, Available: true},
				{Spec: metricQuerySpec{Name: "kubelet_volume_stats_used_bytes_ratio"}, PreValue: 0.95, PostValue: 0.60, Available: true},
			}

			populateMetricsAssessResult(mr, results)

			Expect(mr.PVPhaseFailedBefore).ToNot(BeNil())
			Expect(*mr.PVPhaseFailedBefore).To(BeNumerically("~", 1.0, 0.001))
			Expect(mr.PVPhaseFailedAfter).ToNot(BeNil())
			Expect(*mr.PVPhaseFailedAfter).To(BeNumerically("~", 0.0, 0.001))

			Expect(mr.PVPhasePendingBefore).ToNot(BeNil())
			Expect(mr.PVPhasePendingAfter).ToNot(BeNil())

			Expect(mr.PVUsageRatioBefore).ToNot(BeNil())
			Expect(*mr.PVUsageRatioBefore).To(BeNumerically("~", 0.95, 0.001))
			Expect(mr.PVUsageRatioAfter).ToNot(BeNil())
			Expect(*mr.PVUsageRatioAfter).To(BeNumerically("~", 0.60, 0.001))
		})

		It("UT-EM-193-010: leaves cluster-scoped fields nil when the query is unavailable (graceful degradation)", func() {
			mr := &metricsAssessResult{}
			results := []metricQueryResult{
				{Spec: metricQuerySpec{Name: "kube_node_status_condition_ready"}, Available: false},
			}

			populateMetricsAssessResult(mr, results)

			Expect(mr.NodeNotReadyBefore).To(BeNil())
			Expect(mr.NodeNotReadyAfter).To(BeNil())
		})
	})
})
