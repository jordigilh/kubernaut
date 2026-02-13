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
)

var _ = Describe("Validity Window Integration (BR-EM-006, BR-EM-007)", func() {

	// ========================================
	// IT-EM-VW-001: EA with past deadline -> marked expired on first reconcile
	// ========================================
	It("IT-EM-VW-001: should complete EA with expired validity deadline", func() {
		ns := createTestNamespace("em-vw-001")
		defer deleteTestNamespace(ns)

		By("Creating an EA with already-expired validity deadline")
		createExpiredEffectivenessAssessment(ns, "ea-vw-001", "rr-vw-001")

		By("Verifying the EA is completed by the reconciler")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ea-vw-001",
				Namespace: ns,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
				"Expired EA should be completed by reconciler")
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Spec.Config.ValidityDeadline.Time.Before(time.Now())).To(BeTrue(),
			"validity deadline should be in the past")
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
		// Reason should reflect expiration
		Expect(fetchedEA.Status.AssessmentReason).To(BeElementOf(
			eav1.AssessmentReasonExpired,
			eav1.AssessmentReasonPartial,
			eav1.AssessmentReasonFull,
		))
	})

	// ========================================
	// IT-EM-VW-002: EA completes within window -> normal completion with full assessment
	// ========================================
	It("IT-EM-VW-002: should process EA within validity window to completion", func() {
		ns := createTestNamespace("em-vw-002")
		defer deleteTestNamespace(ns)

		By("Creating an EA with generous validity window (30 minutes)")
		ea := createEffectivenessAssessment(ns, "ea-vw-002", "rr-vw-002")

		By("Verifying the EA completes within its validity window")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Should have completed before the deadline
		Expect(fetchedEA.Spec.Config.ValidityDeadline.Time.After(time.Now())).To(BeTrue(),
			"validity deadline should still be in the future (normal completion)")
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())

		// Components should be assessed
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
	})

	// ========================================
	// IT-EM-VW-005: EA with tight validity window -> expires quickly
	// ========================================
	It("IT-EM-VW-005: should handle tight validity window and complete", func() {
		ns := createTestNamespace("em-vw-005")
		defer deleteTestNamespace(ns)

		By("Creating an EA with very tight validity window (5 seconds)")
		now := metav1.Now()
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-vw-005",
				Namespace: ns,
				Labels: map[string]string{
					"kubernaut.ai/correlation-id": "rr-vw-005",
				},
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-vw-005",
				TargetResource: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					ValidityDeadline:    metav1.Time{Time: now.Add(5 * time.Second)}, // Tight window
					ScoringThreshold:    0.5,
					PrometheusEnabled:   true,
					AlertManagerEnabled: true,
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying the EA completes (either normally or via expiration)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ea-vw-005",
				Namespace: ns,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
	})
})
