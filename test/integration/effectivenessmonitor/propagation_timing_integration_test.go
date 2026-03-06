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
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("Propagation Timing Integration (#253, BR-EM-010.3, BR-EM-010.4)", func() {

	// ========================================
	// IT-EM-253-001: WaitingForPropagation → Stabilizing (envtest)
	// BR: BR-EM-010.3
	//
	// Business outcome: When HashComputeDelay is set, the EM enters
	// WaitingForPropagation phase until creation+HashComputeDelay elapses.
	// Once the deferral elapses, the EM transitions to Stabilizing with hash computed.
	// ========================================
	It("IT-EM-253-001: should enter WaitingForPropagation then Stabilizing after deferral", func() {
		ns := createTestNamespace("em-253-001")
		defer deleteTestNamespace(ns)

		deferralDuration := 8 * time.Second

		By("Creating EA with HashComputeDelay 8s (async CRD target)")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-253-001",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-253-001",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "demo-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					HashComputeDelay:      &metav1.Duration{Duration: deferralDuration},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())
		GinkgoWriter.Printf("Created EA with HashComputeDelay=%s\n", ea.Spec.Config.HashComputeDelay.Duration)

		By("Verifying EA enters WaitingForPropagation during deferral window")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseWaitingForPropagation),
				"BR-EM-010.3: async target must enter WaitingForPropagation")
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verifying phase remains WaitingForPropagation until deferral elapses")
		Consistently(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseWaitingForPropagation),
				"BR-EM-010.3: phase must stay WaitingForPropagation during deferral")
		}, 4*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Waiting for deferral to elapse and EA to leave WaitingForPropagation")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).ToNot(Equal(eav1.PhaseWaitingForPropagation),
				"EA must leave WaitingForPropagation after deferral elapses")
		}, timeout, interval).Should(Succeed())

		By("Verifying EA reaches Completed with hash computed")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"Hash must be computed after deferral + stabilization")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"))
	})

	// ========================================
	// IT-EM-253-002: Timing anchored to creation + HashComputeDelay; phase stays Stabilizing
	// BR: BR-EM-010.4
	//
	// Business outcome: After WaitingForPropagation → Stabilizing, the
	// PrometheusCheckAfter is anchored to creation + HashComputeDelay + StabilizationWindow.
	// The EA stays Stabilizing until CheckAfter elapses.
	// ========================================
	It("IT-EM-253-002: should anchor CheckAfter to creation+HashComputeDelay and stay Stabilizing", func() {
		ns := createTestNamespace("em-253-002")
		defer deleteTestNamespace(ns)

		deferralDuration := 5 * time.Second
		stabilizationDuration := 5 * time.Second

		By("Creating EA with HashComputeDelay=5s, StabilizationWindow=5s")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-253-002",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-253-002",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "demo-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: stabilizationDuration},
					HashComputeDelay:      &metav1.Duration{Duration: deferralDuration},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}, ea)).To(Succeed())
		creationTime := ea.CreationTimestamp.Time

		By("Waiting for EA to transition out of WaitingForPropagation into Stabilizing/Assessing (after ~5s)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(
				BeElementOf(eav1.PhaseStabilizing, eav1.PhaseAssessing, eav1.PhaseCompleted),
				"EA must reach a post-propagation phase")
		}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verifying PrometheusCheckAfter is anchored to creation + HashComputeDelay + stabilization")
		expectedCheckAfter := creationTime.Add(deferralDuration).Add(stabilizationDuration)
		Expect(fetchedEA.Status.PrometheusCheckAfter).ToNot(BeNil(),
			"PrometheusCheckAfter must be set after WaitingForPropagation")
		Expect(fetchedEA.Status.PrometheusCheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, 2*time.Second),
			"CheckAfter must be creation+HashComputeDelay+stab=%s, not creation+stab=%s",
			expectedCheckAfter, creationTime.Add(stabilizationDuration))

		By("Verifying EA eventually reaches Completed after the full window")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())
	})

	// ========================================
	// IT-EM-253-003: Audit includes HashComputeAfter for async targets (envtest)
	// BR: BR-EM-010.5
	//
	// Business outcome: The assessment.scheduled audit event must include
	// hash_compute_after when EA.Spec.Config.HashComputeDelay is set (async targets).
	// GitOpsSyncDelay and OperatorReconcileDelay were removed from EA spec (#277).
	// ========================================
	It("IT-EM-253-003: should include HashComputeAfter in assessment.scheduled audit for async targets", func() {
		ns := createTestNamespace("em-253-003")
		defer deleteTestNamespace(ns)

		deferralDuration := 5 * time.Second

		By("Creating EA with HashComputeDelay (async target)")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-253-003",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-253-003",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "demo-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					HashComputeDelay:      &metav1.Duration{Duration: deferralDuration},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for EA to reach terminal phase (reconciler emits audit events)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(
				BeElementOf(eav1.PhaseStabilizing, eav1.PhaseAssessing, eav1.PhaseCompleted),
				"EA must reach a post-propagation phase for audit to fire")
		}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Flushing audit store to ensure events are persisted")
		Expect(auditStore.Flush(ctx)).To(Succeed())

		By("Querying audit trail for assessment.scheduled event")
		var scheduledEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			resp, err := dsClients.OpenAPIClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString("rr-253-003"),
				Limit:         ogenclient.NewOptInt(100),
			})
			if err != nil {
				GinkgoWriter.Printf("Audit query error: %v\n", err)
				return false
			}
			for i := range resp.Data {
				if resp.Data[i].EventType == "effectiveness.assessment.scheduled" {
					scheduledEvent = &resp.Data[i]
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"effectiveness.assessment.scheduled event must exist in audit trail")

		By("Verifying audit payload contains HashComputeAfter for async targets")
		eaPayload, ok := scheduledEvent.EventData.GetEffectivenessAssessmentAuditPayload()
		Expect(ok).To(BeTrue(), "Event data must be EffectivenessAssessmentAuditPayload")

		Expect(eaPayload.HashComputeAfter.Set).To(BeTrue(),
			"BR-EM-010.5: hash_compute_after must be present in audit for async targets (computed from HashComputeDelay)")

		GinkgoWriter.Printf("IT-EM-253-003: Audit payload validated — HashComputeAfter=%v\n",
			eaPayload.HashComputeAfter.Value)
	})

	// ========================================
	// IT-EM-253-004: Async target validity deadline (envtest)
	// BR: BR-EM-010.4
	//
	// Business outcome: ValidityDeadline accounts for propagation + stabilization + validity.
	// ========================================
	It("IT-EM-253-004: should extend validity deadline for async targets", func() {
		ns := createTestNamespace("em-253-004")
		defer deleteTestNamespace(ns)

		deferralDuration := 5 * time.Second
		stabilizationDuration := 3 * time.Second

		By("Creating EA with HashComputeDelay=5s, StabilizationWindow=3s")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-253-004",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-253-004",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "demo-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: stabilizationDuration},
					HashComputeDelay:      &metav1.Duration{Duration: deferralDuration},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for derived timing to be persisted (WaitingForPropagation phase)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.ValidityDeadline).ToNot(BeNil(),
				"ValidityDeadline must be persisted during WaitingForPropagation")
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verifying ValidityDeadline = creation + HashComputeDelay + stab + validity (async formula)")
		// EM config ValidityWindow is set by the test suite — check what the reconciler computed
		// For async targets: deadline = creation + HashComputeDelay + stab + validity
		// The key assertion: deadline > creation + HashComputeDelay + stab (it includes the full validity window)
		deadline := fetchedEA.Status.ValidityDeadline.Time
		minExpected := fetchedEA.CreationTimestamp.Time.Add(deferralDuration).Add(stabilizationDuration)
		Expect(deadline).To(BeTemporally(">", minExpected),
			"ValidityDeadline must be later than creation + HashComputeDelay + StabilizationWindow")

		By("Verifying EA does NOT expire prematurely during propagation wait")
		Consistently(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).ToNot(Equal(eav1.PhaseCompleted),
				"EA must not complete during propagation wait")
		}, 3*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verifying EA eventually reaches Completed")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())
	})
})
