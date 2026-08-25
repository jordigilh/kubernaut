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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// createFleetScopedEA creates an EA CRD with Spec.ClusterID set, simulating a
// remote-cluster fleet remediation (Issue #2274, BR-FLEET-054). When kind is
// "Deployment", the target is namespace-scoped; otherwise (Node,
// PersistentVolume) the target is cluster-scoped with an empty Namespace,
// mirroring createClusterScopedEA/createEffectivenessAssessment above but
// adding the ClusterID that neither of those helpers set.
func createFleetScopedEA(namespace, name, correlationID, clusterID, kind, resourceName string) *eav1.EffectivenessAssessment {
	target := eav1.TargetResource{Kind: kind, Name: resourceName}
	if kind == "Deployment" {
		target.Namespace = namespace
	}
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           correlationID,
			ClusterID:               clusterID,
			RemediationRequestPhase: "Completed",
			SignalTarget:            target,
			RemediationTarget:       target,
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
			},
		},
	}
	Expect(k8sClient.Create(ctx, ea)).To(Succeed())
	GinkgoWriter.Printf("✅ Created fleet-scoped EffectivenessAssessment: %s/%s (clusterID=%s, kind=%s)\n",
		ea.Namespace, ea.Name, clusterID, kind)
	return ea
}

// Issue #2274: EM's Prometheus/AlertManager queries must be scoped to the
// remediation's fleet cluster (ea.Spec.ClusterID), driven through a real
// Reconcile() -- proving production wiring, not just the unit-level query
// builders (Pyramid Invariant: UT proves logic, IT proves wiring).
var _ = Describe("Fleet Cluster-Scoped Query Wiring (BR-EM-002, BR-EM-003, DD-EM-005 v1.3, Issue #2274)", func() {

	BeforeEach(func() {
		now := float64(time.Now().Unix())
		preRemediationTime := now - 60
		mockProm.SetQueryRangeHandler(nil)
		mockProm.SetQueryRangeResponse(infrastructure.NewPromMatrixResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total"},
			[][]interface{}{
				{preRemediationTime, "0.500000"},
				{now, "0.250000"},
			},
		))
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
		mockAM.ResetRequestLog()
		mockProm.ResetRequestLog()
	})

	// IT-EM-2274-001: namespace-scoped target, fleet remote cluster -> PromQL carries cluster= matcher
	It("IT-EM-2274-001: scopes namespace-scoped PromQL queries to the remediation's fleet cluster", func() {
		ns := createTestNamespace(ctx, "em-2274-ns")
		defer deleteTestNamespace(ns)

		ea := createFleetScopedEA(ns, "ea-2274-ns", "rr-2274-ns", "remote-cluster", "Deployment", "test-app")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())

		By("Verifying the dispatched PromQL carries a cluster= matcher for the remote fleet cluster")
		Expect(promQuerySent(`cluster="remote-cluster"`)).To(BeTrue(),
			"a namespace-scoped remote-cluster remediation's PromQL must be scoped to that cluster (Issue #2274)")
	})

	// IT-EM-2274-002: cluster-scoped Node target, fleet remote cluster -> PromQL carries BOTH node= and cluster= matchers
	It("IT-EM-2274-002: scopes cluster-scoped (Node) PromQL queries to the remediation's fleet cluster", func() {
		ea := createFleetScopedEA("default", "ea-2274-node", "rr-2274-node", "remote-cluster", "Node", "worker-1")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())

		By("Verifying the dispatched PromQL carries both the Node matcher and the fleet cluster matcher")
		Expect(promQuerySent(`node="worker-1"`)).To(BeTrue())
		Expect(promQuerySent(`cluster="remote-cluster"`)).To(BeTrue(),
			"a Node-target remote-cluster remediation's PromQL must be scoped to that cluster (Issue #2274), "+
				"not just to the node name -- two clusters can both have a node named worker-1")
	})

	// IT-EM-2274-003: fleet remote cluster -> AlertManager query carries a cluster= filter
	It("IT-EM-2274-003: scopes the AlertManager alert-resolution query to the remediation's fleet cluster", func() {
		ea := createFleetScopedEA("default", "ea-2274-alert", "rr-2274-alert", "remote-cluster", "Deployment", "test-app")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue())

		By("Verifying the AlertManager request carried a cluster-specific matcher")
		requests := mockAM.GetRequestLog()
		found := false
		for _, req := range requests {
			if req.Path != pathV2Alerts {
				continue
			}
			for _, f := range req.Query["filter"] {
				if f == `cluster="remote-cluster"` {
					found = true
				}
			}
		}
		Expect(found).To(BeTrue(),
			"a remote-cluster remediation's AlertManager query must be scoped to that cluster (Issue #2274), "+
				"otherwise it can match an alert firing for a same-named resource on a different fleet cluster")
	})

	// UT/IT-EM-2274-BC-005: backward compatibility -- empty ClusterID (hub/local remediation)
	It("IT-EM-2274-BC-005: does not add a cluster= matcher for hub/local remediations (empty ClusterID)", func() {
		ns := createTestNamespace(ctx, "em-2274-bc")
		defer deleteTestNamespace(ns)

		ea := createFleetScopedEA(ns, "ea-2274-bc", "rr-2274-bc", "" /* empty ClusterID: hub/local */, "Deployment", "test-app")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())
		Expect(promQuerySent("cluster=")).To(BeFalse(),
			"hub/local remediations (empty ClusterID) must not introduce a cluster= matcher — backward compatibility")
	})
})
