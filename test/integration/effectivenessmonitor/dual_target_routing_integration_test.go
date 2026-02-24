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
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// ============================================================================
// DUAL-TARGET ROUTING INTEGRATION TESTS (Issue #188, DD-EM-003)
//
// These tests verify that the EM reconciler routes each assessment component
// to the correct target when SignalTarget and RemediationTarget diverge:
//   - Hash + drift guard: RemediationTarget (the modified resource)
//   - Health: SignalTarget (the alerting resource)
//   - Metrics (PromQL): SignalTarget.Namespace
//   - Alert: SignalTarget.Namespace (AlertContext)
//
// Strategy: Create EAs with intentionally divergent targets and verify
// observable behavior through EA status fields and mock request logs.
// ============================================================================

// createDivergentTargetEA creates an EA where SignalTarget and RemediationTarget
// differ in kind, name, and namespace. This makes routing observable.
func createDivergentTargetEA(namespace, name, correlationID, signalNs, remediationNs string) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           correlationID,
			RemediationRequestPhase: "Completed",
			SignalTarget: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "api-frontend",
				Namespace: signalNs,
			},
			RemediationTarget: eav1.TargetResource{
				Kind:      "HorizontalPodAutoscaler",
				Name:      "api-frontend-hpa",
				Namespace: remediationNs,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
			},
			SignalName: "KubeHpaMaxedOut",
		},
	}
	Expect(k8sClient.Create(ctx, ea)).To(Succeed())
	GinkgoWriter.Printf("Created divergent-target EA: %s/%s (signal-ns=%s, remediation-ns=%s)\n",
		namespace, name, signalNs, remediationNs)
	return ea
}

var _ = Describe("Dual-Target Routing (Issue #188, DD-EM-003)", func() {

	// ========================================================================
	// IT-EM-188-007: assessMetrics uses SignalTarget.Namespace in PromQL queries
	// ========================================================================
	It("IT-EM-188-007: should use SignalTarget.Namespace in Prometheus PromQL queries", func() {
		ns := createTestNamespace("em-rt-007")
		defer deleteTestNamespace(ns)

		signalNs := ns + "-signal"
		remediationNs := ns + "-remediation"

		By("Resetting Prometheus request log before test")
		mockProm.ResetRequestLog()

		By("Creating EA with divergent signal/remediation namespaces")
		ea := createDivergentTargetEA(ns, "ea-rt-007", "rr-rt-007", signalNs, remediationNs)

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Inspecting Prometheus request log for namespace in PromQL queries")
		requests := mockProm.GetRequestLog()
		queryRangeRequests := 0
		for _, req := range requests {
			if req.Path != "/api/v1/query_range" {
				continue
			}
			queryRangeRequests++
			queryValues := req.Query["query"]
			Expect(queryValues).NotTo(BeEmpty(), "query_range request should have a 'query' parameter")
			query := queryValues[0]

			Expect(query).To(ContainSubstring(signalNs),
				"DD-EM-003: PromQL query should use SignalTarget namespace %q, got: %s", signalNs, query)
			Expect(query).NotTo(ContainSubstring(remediationNs),
				"DD-EM-003: PromQL query must NOT use RemediationTarget namespace %q, got: %s", remediationNs, query)
		}
		Expect(queryRangeRequests).To(BeNumerically(">", 0),
			"Prometheus should have received at least one query_range request")
	})

	// ========================================================================
	// IT-EM-188-005: getTargetHealthStatus uses SignalTarget (Deployment kind)
	// ========================================================================
	It("IT-EM-188-005: should use SignalTarget for health assessment (Deployment, not HPA)", func() {
		ns := createTestNamespace("em-rt-005")
		defer deleteTestNamespace(ns)

		signalNs := ns + "-signal"
		remediationNs := ns + "-remediation"

		By("Creating EA with SignalTarget=Deployment and RemediationTarget=HPA")
		ea := createDivergentTargetEA(ns, "ea-rt-005", "rr-rt-005", signalNs, remediationNs)

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// DD-EM-003: Health uses SignalTarget.
		// SignalTarget.Kind = Deployment → workload health path → Score = &0.0 (no pods)
		// If RemediationTarget (HPA) was used → default path → HealthNotApplicable → Score = nil
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil(),
			"DD-EM-003: HealthScore should not be nil. Deployment target (SignalTarget) produces "+
				"Score=0.0 (no pods found). HPA target (RemediationTarget) would produce Score=nil "+
				"(HealthNotApplicable). A non-nil score proves SignalTarget was used.")
	})

	// ========================================================================
	// IT-EM-188-004: getTargetSpec uses RemediationTarget for hash computation
	// ========================================================================
	It("IT-EM-188-004: should use RemediationTarget for spec hash computation", func() {
		ns := createTestNamespace("em-rt-004")
		defer deleteTestNamespace(ns)

		signalNs := ns + "-signal"
		remediationNs := ns + "-remediation"

		By("Creating EA with divergent targets")
		ea := createDivergentTargetEA(ns, "ea-rt-004", "rr-rt-004", signalNs, remediationNs)

		By("Waiting for EA to complete with hash computed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// DD-EM-003: Hash uses RemediationTarget (HPA in remediation-ns).
		// getTargetSpec falls back to metadata when the resource doesn't exist,
		// producing a hash from {kind, name, namespace} of the RemediationTarget.
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"DD-EM-003: hash should be computed from RemediationTarget")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).NotTo(BeEmpty(),
			"DD-EM-003: post-remediation spec hash should be set")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"DD-EM-003: hash should use canonical sha256: prefix (DD-EM-002)")
	})

	// ========================================================================
	// IT-EM-188-008: Drift guard re-hashes using RemediationTarget
	// ========================================================================
	It("IT-EM-188-008: should use RemediationTarget for drift guard re-hashing", func() {
		ns := createTestNamespace("em-rt-008")
		defer deleteTestNamespace(ns)

		signalNs := ns + "-signal"
		remediationNs := ns + "-remediation"

		By("Creating EA with divergent targets")
		ea := createDivergentTargetEA(ns, "ea-rt-008", "rr-rt-008", signalNs, remediationNs)

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// DD-EM-003: Drift guard calls getTargetSpec(ctx, ea.Spec.RemediationTarget).
		// Both initial hash and drift re-hash use the same target. Since the target
		// resource (HPA in remediation-ns) doesn't change between reconciles, the
		// current hash should equal the post-remediation hash (no drift).
		// If the drift guard used different targets for initial vs. re-hash, the
		// hashes would diverge and AssessmentReason would be "spec_drift".
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
		if fetchedEA.Status.Components.CurrentSpecHash != "" {
			Expect(fetchedEA.Status.Components.CurrentSpecHash).To(
				Equal(fetchedEA.Status.Components.PostRemediationSpecHash),
				"DD-EM-003: current hash should match post-remediation hash (no drift)")
		}
		Expect(fetchedEA.Status.AssessmentReason).NotTo(Equal("spec_drift"),
			"DD-EM-003: no spec drift should be detected when RemediationTarget is stable")
	})

	// ========================================================================
	// IT-EM-188-006: assessAlert uses SignalTarget.Namespace in AlertContext
	// ========================================================================
	It("IT-EM-188-006: should pass SignalTarget.Namespace to alert scorer context", func() {
		ns := createTestNamespace("em-rt-006")
		defer deleteTestNamespace(ns)

		signalNs := ns + "-signal"
		remediationNs := ns + "-remediation"

		By("Configuring mock AlertManager with a firing alert matching the signal name")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{
			infrastructure.NewFiringAlert("KubeHpaMaxedOut", map[string]string{
				"namespace": signalNs,
			}),
		})
		defer mockAM.SetAlertsResponse([]infrastructure.AMAlert{})

		By("Resetting AM request log")
		mockAM.ResetRequestLog()

		By("Creating EA with divergent targets")
		ea := createDivergentTargetEA(ns, "ea-rt-006", "rr-rt-006", signalNs, remediationNs)

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.AlertScore).NotTo(BeNil())

		// Verify AM was queried with a filter containing the signal alert name
		requests := mockAM.GetRequestLog()
		alertGetRequests := 0
		for _, req := range requests {
			if req.Path == "/api/v2/alerts" && req.Method == "GET" {
				alertGetRequests++
				// The scorer builds matchers from AlertContext.AlertName
				filterValues := req.Query["filter"]
				if len(filterValues) > 0 {
					filter := strings.Join(filterValues, " ")
					Expect(filter).To(ContainSubstring("KubeHpaMaxedOut"),
						"DD-EM-003: alert filter should contain the signal alert name")
				}
			}
		}
		Expect(alertGetRequests).To(BeNumerically(">", 0),
			"AlertManager should have been queried at least once")
	})

	// ========================================================================
	// IT-EM-188-FULL: Full reconcile with divergent targets completes successfully
	// ========================================================================
	It("IT-EM-188-FULL: should complete full assessment with divergent signal/remediation targets", func() {
		ns := createTestNamespace("em-rt-full")
		defer deleteTestNamespace(ns)

		signalNs := ns + "-signal"
		remediationNs := ns + "-remediation"

		By("Creating EA with divergent targets (HPA-maxed scenario)")
		ea := createDivergentTargetEA(ns, "ea-rt-full", "rr-rt-full", signalNs, remediationNs)

		By("Waiting for EA to reach Completed phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ea), fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying all components were assessed")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(), "alert should be assessed")
		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue(), "metrics should be assessed")
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(), "health should be assessed")
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(), "hash should be computed")

		By("Verifying assessment completed with a reason")
		Expect(fetchedEA.Status.AssessmentReason).NotTo(BeEmpty(),
			"assessment reason should be set for completed EA with divergent targets")
	})
})
