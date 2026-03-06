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

var _ = Describe("Hash Compute Deferral Integration (DD-EM-004, BR-EM-010)", func() {

	// ========================================
	// IT-EM-251-001: Async-managed target — EM defers then computes
	// BR: BR-EM-010.1, BR-RO-103
	//
	// Business outcome: When the RO detects an async-managed target (GitOps or
	// operator CRD) and sets HashComputeDelay in Config, the EM must NOT compute
	// the hash immediately. Once the deferral window (creation + HashComputeDelay)
	// elapses, the EM computes the hash and completes the assessment normally.
	// This ensures the hash captures the spec AFTER the external controller reconciles.
	// ========================================
	It("IT-EM-251-001: should defer hash computation until HashComputeDelay elapses, then complete", func() {
		ns := createTestNamespace("em-251-001")
		defer deleteTestNamespace(ns)

		deferralDuration := 8 * time.Second

		By("Creating EA with HashComputeDelay 8s (simulates async CRD target)")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-251-001",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-251-001",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					HashComputeDelay:      &metav1.Duration{Duration: deferralDuration},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())
		GinkgoWriter.Printf("Created EA with Config.HashComputeDelay=%s (deferral=%s)\n",
			ea.Spec.Config.HashComputeDelay.Duration, deferralDuration)

		By("Verifying hash is NOT computed during the deferral window")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Consistently(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Components.HashComputed).To(BeFalse(),
				"BR-EM-010.1: hash must NOT be computed while deferral window is active")
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Waiting for the deferral window to elapse and hash to be computed")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
				"BR-EM-010.1: hash must be computed after deferral window elapses")
		}, timeout, interval).Should(Succeed())

		By("Verifying the assessment completed normally after deferral")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"Post-remediation hash must use canonical format after deferred computation")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HaveLen(71),
			"Hash must be 71 chars: 'sha256:' (7) + 64 hex digits")
		Expect(fetchedEA.Status.Components.CurrentSpecHash).To(
			Equal(fetchedEA.Status.Components.PostRemediationSpecHash),
			"Drift detection baseline must be established after deferred hash computation")
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"Health component must still be assessed after deferred hash")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"Alert component must still be assessed after deferred hash")
		Expect(fetchedEA.Status.AssessmentReason).To(
			BeElementOf("full", "partial", "metrics_timed_out"),
			"Assessment must complete with a definitive reason despite hash deferral")

		GinkgoWriter.Printf("Assessment completed: hash=%s, reason=%s\n",
			fetchedEA.Status.Components.PostRemediationSpecHash[:23]+"...",
			fetchedEA.Status.AssessmentReason)
	})

	// ========================================
	// IT-EM-251-002: Sync target — hash computed immediately (backward compat)
	// BR: BR-EM-010.1
	//
	// Business outcome: When Config.HashComputeDelay is nil (sync target, e.g., Deployment
	// patched directly by kubectl), the EM computes the hash on the first reconcile
	// without any deferral. This is the existing behavior for all pre-#251 EAs.
	// ========================================
	It("IT-EM-251-002: should compute hash immediately when HashComputeDelay is nil (sync target)", func() {
		ns := createTestNamespace("em-251-002")
		defer deleteTestNamespace(ns)

		By("Creating EA without HashComputeDelay (sync target, backward compatible)")
		ea := createEffectivenessAssessment(ns, "ea-251-002", "rr-251-002")

		By("Verifying the EA completes with hash computed (no deferral)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Spec.Config.HashComputeDelay).To(BeNil(),
			"BR-EM-010.1: sync target EA must have nil HashComputeDelay")
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"BR-EM-010.1: hash must be computed immediately for sync targets")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"Hash must use canonical sha256: format")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HaveLen(71))
		Expect(fetchedEA.Status.AssessmentReason).To(
			BeElementOf("full", "partial", "metrics_timed_out"),
			"Assessment must complete normally for sync targets")
	})

	// ========================================
	// IT-EM-251-003: Elapsed deferral — hash computed immediately
	// BR: BR-EM-010.1
	//
	// Business outcome: When HashComputeDelay is set but creation + HashComputeDelay
	// is already in the past (e.g., 1ns duration—effectively zero—or controller
	// restart after deferral elapsed), the EM computes the hash immediately rather
	// than requeueing. This prevents indefinite deferral and ensures assessments
	// always complete.
	// ========================================
	It("IT-EM-251-003: should compute hash immediately when HashComputeDelay is effectively elapsed", func() {
		ns := createTestNamespace("em-251-003")
		defer deleteTestNamespace(ns)

		By("Creating EA with HashComputeDelay 1ns (elapsed deferral—creation + 1ns < now)")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-251-003",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-251-003",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					HashComputeDelay:      &metav1.Duration{Duration: 1 * time.Nanosecond},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying the EA completes with hash computed (elapsed deferral treated as immediate)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"BR-EM-010.1: elapsed deferral must not block hash computation")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"Hash must use canonical sha256: format")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HaveLen(71))
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"All components must be assessed when deferral window already elapsed")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"All components must be assessed when deferral window already elapsed")
		Expect(fetchedEA.Status.AssessmentReason).To(
			BeElementOf("full", "partial", "metrics_timed_out"),
			"Assessment must complete with a definitive reason")
	})
})
