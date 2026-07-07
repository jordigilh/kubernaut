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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// fetchMetricsAssessedPayload queries the audit trail for the
// effectiveness.metrics.assessed event matching correlationID and returns
// its typed metric_deltas sub-object. Used to prove the audit event content
// (not just Score/MetricsAssessed) is populated for cluster-scoped targets
// (Issue #193 audit gap, DD-EM-005 v1.1).
func fetchMetricsAssessedPayload(correlationID string) ogenclient.EffectivenessAssessmentAuditPayloadMetricDeltas {
	var event *ogenclient.AuditEvent
	Eventually(func() bool {
		resp, err := dsClients.OpenAPIClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(correlationID),
			Limit:         ogenclient.NewOptInt(100),
		})
		if err != nil {
			GinkgoWriter.Printf("Audit query error: %v\n", err)
			return false
		}
		for i := range resp.Data {
			if resp.Data[i].EventType == "effectiveness.metrics.assessed" {
				event = &resp.Data[i]
				return true
			}
		}
		return false
	}, 30*time.Second, 2*time.Second).Should(BeTrue(),
		"effectiveness.metrics.assessed event must exist in audit trail")

	payload, ok := event.EventData.GetEffectivenessAssessmentAuditPayload()
	Expect(ok).To(BeTrue(), "Event data must be EffectivenessAssessmentAuditPayload")
	Expect(payload.MetricDeltas.Set).To(BeTrue(), "metric_deltas sub-object must be set")
	return payload.MetricDeltas.Value
}

// promQuerySent reports whether any query_range request logged so far carried
// a "query" param containing substr. The mock's canned response ignores query
// content, so this is the only reliable way to prove which PromQL the
// reconciler actually dispatched (i.e. that Kind-dispatch routed to the new
// cluster-scoped builders, not the namespace-scoped ones).
func promQuerySent(substr string) bool {
	for _, req := range mockProm.GetRequestLog() {
		if req.Path != "/api/v1/query_range" {
			continue
		}
		for _, q := range req.Query["query"] {
			if strings.Contains(q, substr) {
				return true
			}
		}
	}
	return false
}

// createClusterScopedEA creates an EA CRD for a cluster-scoped target (Node,
// PersistentVolume) — SignalTarget/RemediationTarget both carry an empty
// Namespace, mirroring how the Remediation Orchestrator populates these
// fields for Kind=Node/PersistentVolume today (Issue #193, DD-EM-005).
func createClusterScopedEA(name, correlationID, kind, resourceName string) *eav1.EffectivenessAssessment {
	target := eav1.TargetResource{
		Kind: kind,
		Name: resourceName,
		// Namespace intentionally empty: cluster-scoped resource.
	}
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           correlationID,
			RemediationRequestPhase: "Completed",
			SignalTarget:            target,
			RemediationTarget:       target,
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
			},
		},
	}
	Expect(k8sClient.Create(ctx, ea)).To(Succeed())
	GinkgoWriter.Printf("✅ Created cluster-scoped EffectivenessAssessment: %s/%s (Kind=%s, Name=%s)\n",
		ea.Namespace, ea.Name, kind, resourceName)
	return ea
}

var _ = Describe("Cluster-Scoped Resource Assessment Integration (Issue #193, BR-EM-002, BR-EM-003, DD-EM-005)", func() {

	BeforeEach(func() {
		// Restore mock Prometheus/AlertManager to known-good defaults before each test,
		// since other Describe blocks in this suite mutate shared package-level mocks.
		now := float64(time.Now().Unix())
		preRemediationTime := now - 60
		mockProm.SetQueryRangeHandler(nil)
		mockProm.SetQueryRangeResponse(infrastructure.NewPromMatrixResponse(
			map[string]string{"__name__": "kube_node_status_condition"},
			[][]interface{}{
				{preRemediationTime, "1.000000"}, // pre-remediation: condition firing (e.g. NotReady=1)
				{now, "0.000000"},                // post-remediation: condition cleared (improvement)
			},
		))
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
		mockAM.ResetRequestLog()
		mockProm.ResetRequestLog()
	})

	// ========================================
	// IT-EM-193-001: Kind=Node metrics dispatch -> MetricsAssessed=true
	// ========================================
	It("IT-EM-193-001: should assess metrics for a cluster-scoped Node target", func() {
		ea := createClusterScopedEA("ea-193-node-mc", "rr-193-node-mc", "Node", "worker-1")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue(),
			"Node targets must be assessed via kube_node_status_condition, not the namespace-scoped queries")
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil())
		Expect(*fetchedEA.Status.Components.MetricsScore).To(BeNumerically(">", 0.0),
			"condition cleared (1 -> 0) should score as improvement for a LowerIsBetter metric")

		By("Verifying the reconciler actually dispatched the Node-scoped PromQL, not the namespace-scoped fallback")
		Expect(promQuerySent(`node="worker-1"`)).To(BeTrue(),
			"assessMetrics must dispatch to buildNodeMetricQuerySpecs for Kind=Node, which queries by node label")
		Expect(promQuerySent(`namespace=""`)).To(BeFalse(),
			"the namespace-scoped query builder (which produces an empty namespace filter) must not be used for Kind=Node")

		By("Verifying the effectiveness.metrics.assessed audit event's metric_deltas sub-object is populated (audit gap fix, DD-EM-005 v1.1)")
		Expect(auditStore.Flush(ctx)).To(Succeed())
		md := fetchMetricsAssessedPayload("rr-193-node-mc")
		Expect(md.NodeNotReadyBefore.Set).To(BeTrue(), "node_not_ready_before must be populated for Kind=Node")
		Expect(md.NodeNotReadyBefore.Value).To(BeNumerically("~", 1.0, 0.001))
		Expect(md.NodeNotReadyAfter.Set).To(BeTrue())
		Expect(md.NodeNotReadyAfter.Value).To(BeNumerically("~", 0.0, 0.001))
		Expect(md.NodeMemoryPressureBefore.Set).To(BeTrue())
		Expect(md.NodeDiskPressureBefore.Set).To(BeTrue())
	})

	// ========================================
	// IT-EM-193-002: Kind=PersistentVolume metrics dispatch -> MetricsAssessed=true
	// ========================================
	It("IT-EM-193-002: should assess metrics for a cluster-scoped PersistentVolume target", func() {
		ea := createClusterScopedEA("ea-193-pv-mc", "rr-193-pv-mc", "PersistentVolume", "pvc-abc123")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue(),
			"PersistentVolume targets must be assessed via kube_persistentvolume_* queries, not the namespace-scoped queries")
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil())

		By("Verifying the reconciler actually dispatched the PV-scoped PromQL, not the namespace-scoped fallback")
		Expect(promQuerySent(`persistentvolume="pvc-abc123"`)).To(BeTrue(),
			"assessMetrics must dispatch to buildPVMetricQuerySpecs for Kind=PersistentVolume, which queries by persistentvolume label")
		Expect(promQuerySent(`namespace=""`)).To(BeFalse(),
			"the namespace-scoped query builder (which produces an empty namespace filter) must not be used for Kind=PersistentVolume")

		By("Verifying the effectiveness.metrics.assessed audit event's metric_deltas sub-object is populated (audit gap fix, DD-EM-005 v1.1)")
		Expect(auditStore.Flush(ctx)).To(Succeed())
		md := fetchMetricsAssessedPayload("rr-193-pv-mc")
		Expect(md.PvPhaseFailedBefore.Set).To(BeTrue(), "pv_phase_failed_before must be populated for Kind=PersistentVolume")
		Expect(md.PvPhaseFailedAfter.Set).To(BeTrue())
		Expect(md.PvPhasePendingBefore.Set).To(BeTrue())
		Expect(md.PvUsageRatioBefore.Set).To(BeTrue())
		Expect(md.PvUsageRatioBefore.Value).To(BeNumerically("~", 1.0, 0.001))
	})

	// ========================================
	// IT-EM-193-003: Kind=Node alert matcher precision -> AlertLabels populated
	// ========================================
	It("IT-EM-193-003: should scope AlertManager matchers to the specific Node via AlertLabels", func() {
		ea := createClusterScopedEA("ea-193-node-alert", "rr-193-node-alert", "Node", "worker-1")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue())

		By("Verifying the AlertManager request carried a node-specific matcher")
		requests := mockAM.GetRequestLog()
		found := false
		for _, req := range requests {
			if req.Path != "/api/v2/alerts" {
				continue
			}
			for _, f := range req.Query["filter"] {
				if f == `node="worker-1"` {
					found = true
				}
			}
		}
		Expect(found).To(BeTrue(),
			"assessAlert must populate AlertContext.AlertLabels so buildMatchers emits a node=\"worker-1\" filter, "+
				"distinguishing this Node from any other Node firing the same alertname")
	})
})
