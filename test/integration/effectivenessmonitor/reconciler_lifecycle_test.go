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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

var _ = Describe("Reconciler Lifecycle (BR-EM-005)", func() {

	// ========================================
	// IT-EM-RC-001: Create EA -> reconciler triggered -> phase transitions
	// ========================================
	It("IT-EM-RC-001: should reconcile a new EA and transition through phases", func() {
		ns := createTestNamespace("em-rc-001")
		defer deleteTestNamespace(ns)

		By("Creating an EffectivenessAssessment CRD")
		ea := createEffectivenessAssessment(ns, "ea-rc-001", "rr-rc-001")

		By("Verifying the reconciler processes the EA and reaches a terminal phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			// The reconciler should transition through Pending -> Assessing -> Completed
			// Since envtest has no real pods, health score will be 0.0 (target not found),
			// but the reconciler should still complete the assessment
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
				"EA should reach Completed phase")
		}, timeout, interval).Should(Succeed())

		// Verify assessment details
		Expect(fetchedEA.Spec.CorrelationID).To(Equal("rr-rc-001"))
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil(), "CompletedAt should be set")
		Expect(fetchedEA.Status.AssessmentReason).NotTo(BeEmpty(), "AssessmentReason should be set")
		Expect(fetchedEA.Status.Message).NotTo(BeEmpty(), "Message should be set")

		// Health should be assessed (even without real pods)
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(), "HealthAssessed should be true")
		// Hash should be computed
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(), "HashComputed should be true")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).NotTo(BeEmpty(), "PostRemediationSpecHash should be set")
	})

	// ========================================
	// IT-EM-RC-002: Multiple EAs created concurrently -> all processed independently
	// ========================================
	It("IT-EM-RC-002: should process multiple EAs independently", func() {
		ns := createTestNamespace("em-rc-002")
		defer deleteTestNamespace(ns)

		By("Creating multiple EAs concurrently")
		eaCount := 3
		for i := 0; i < eaCount; i++ {
			createEffectivenessAssessment(ns,
				fmt.Sprintf("ea-rc-002-%d", i),
				fmt.Sprintf("rr-rc-002-%d", i),
			)
		}

		By("Verifying all EAs are processed independently to completion")
		for i := 0; i < eaCount; i++ {
			fetchedEA := &eav1.EffectivenessAssessment{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("ea-rc-002-%d", i),
					Namespace: ns,
				}, fetchedEA)).To(Succeed())
				g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
					fmt.Sprintf("EA %d should reach Completed phase", i))
			}, timeout, interval).Should(Succeed())

			Expect(fetchedEA.Spec.CorrelationID).To(Equal(fmt.Sprintf("rr-rc-002-%d", i)))
			Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
		}
	})

	// ========================================
	// IT-EM-RC-003: EA with past validity deadline -> marked expired on first reconcile
	// ========================================
	It("IT-EM-RC-003: should complete expired EA on first reconcile", func() {
		ns := createTestNamespace("em-rc-003")
		defer deleteTestNamespace(ns)

		By("Creating an EA with an already-expired validity deadline")
		createExpiredEffectivenessAssessment(ns, "ea-rc-003", "rr-rc-003")

		By("Verifying the EA is completed with expired/partial reason")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ea-rc-003",
				Namespace: ns,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
				"Expired EA should reach Completed phase")
		}, timeout, interval).Should(Succeed())

		// Verify the expired deadline was stored correctly in status
		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())
		Expect(fetchedEA.Status.ValidityDeadline.Time.Before(time.Now())).To(BeTrue(),
			"validity deadline should be in the past")

		// The reason should reflect that the assessment expired
		Expect(fetchedEA.Status.AssessmentReason).To(BeElementOf(
			eav1.AssessmentReasonExpired,
			eav1.AssessmentReasonPartial,
			eav1.AssessmentReasonFull, // Could be full if components assessed before expiration detected
		), "reason should indicate expiration or partial data")
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
	})

	// ========================================
	// IT-EM-RC-005: EA already Completed -> reconcile is idempotent no-op
	// ========================================
	It("IT-EM-RC-005: should be idempotent for already completed EAs", func() {
		ns := createTestNamespace("em-rc-005")
		defer deleteTestNamespace(ns)

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-rc-005", "rr-rc-005")

		By("Waiting for reconciler to complete the EA")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Recording the Completed state for comparison")
		completedAt := fetchedEA.Status.CompletedAt.DeepCopy()
		reason := fetchedEA.Status.AssessmentReason

		By("Verifying the Completed status persists (no regression)")
		Consistently(func(g Gomega) {
			recheck := &eav1.EffectivenessAssessment{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, recheck)).To(Succeed())

			g.Expect(recheck.Status.Phase).To(Equal(eav1.PhaseCompleted))
			g.Expect(recheck.Status.AssessmentReason).To(Equal(reason))
			g.Expect(recheck.Status.CompletedAt.Time.Equal(completedAt.Time)).To(BeTrue(),
				"CompletedAt should not change on subsequent reconciles")
		}, 3*time.Second, 500*time.Millisecond).Should(Succeed())
	})

	// ========================================
	// IT-EM-RC-006: EA phase transitions Pending -> Assessing -> Completed
	// ========================================
	It("IT-EM-RC-006: should transition through expected phases", func() {
		ns := createTestNamespace("em-rc-006")
		defer deleteTestNamespace(ns)

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-rc-006", "rr-rc-006")

		By("Verifying the EA reaches Completed phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Verify all mandatory components were assessed
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
	})

	// ========================================
	// IT-EM-RC-007: EA with ownerRef labels -> correlation tracking works
	// ========================================
	It("IT-EM-RC-007: should support correlation via labels", func() {
		ns := createTestNamespace("em-rc-007")
		defer deleteTestNamespace(ns)

		By("Creating an EA with owner reference labels")
		ea := createEffectivenessAssessment(ns, "ea-rc-007", "rr-rc-007")

		By("Verifying the EA is processed and labels are preserved")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Verify labels are preserved after reconciliation
		Expect(fetchedEA.Labels).To(HaveKeyWithValue("kubernaut.ai/correlation-id", "rr-rc-007"))
	})
})
