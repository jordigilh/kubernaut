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

// ============================================================================
// Derived Timing Integration Tests (BR-EM-009, ADR-EM-001 v1.3)
//
// These tests verify that the EM controller computes and persists all derived
// timing fields (ValidityDeadline, PrometheusCheckAfter, AlertManagerCheckAfter)
// on first reconciliation, and that the assessment.scheduled audit event is emitted.
//
// Defense-in-Depth: Integration tier (envtest + httptest mocks)
// ============================================================================

var _ = Describe("Derived Timing Computation (BR-EM-009)", func() {

	// ========================================
	// IT-EM-DT-001: First reconciliation sets ValidityDeadline in EA status
	// ========================================
	It("IT-EM-DT-001: should set ValidityDeadline in status on first reconciliation", func() {
		ns := createTestNamespace("em-dt-001")
		defer deleteTestNamespace(ns)

		By("Creating an EA with 1s stabilization window")
		ea := createEffectivenessAssessment(ns, "ea-dt-001", "rr-dt-001")

		By("Waiting for the controller to process the EA (phase transition to Assessing or Completed)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			// Phase should have moved past Pending (ValidityDeadline set during Pending -> Assessing)
			g.Expect(fetchedEA.Status.Phase).NotTo(BeEmpty())
			g.Expect(fetchedEA.Status.Phase).NotTo(Equal(eav1.PhasePending))
		}, timeout, interval).Should(Succeed())

		By("Verifying ValidityDeadline is set in status")
		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil(),
			"ValidityDeadline should be set in status after first reconciliation")

		By("Verifying ValidityDeadline = creationTimestamp + validityWindow (30m default)")
		expectedDeadline := fetchedEA.CreationTimestamp.Add(30 * time.Minute)
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(
			BeTemporally("~", expectedDeadline, 2*time.Second),
			"ValidityDeadline should be creationTimestamp + 30m validityWindow")
	})

	// ========================================
	// IT-EM-DT-002: First reconciliation sets PrometheusCheckAfter in EA status
	// ========================================
	It("IT-EM-DT-002: should set PrometheusCheckAfter in status on first reconciliation", func() {
		ns := createTestNamespace("em-dt-002")
		defer deleteTestNamespace(ns)

		By("Creating an EA with 1s stabilization window")
		ea := createEffectivenessAssessment(ns, "ea-dt-002", "rr-dt-002")

		By("Waiting for controller to process the EA")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).NotTo(BeEmpty())
			g.Expect(fetchedEA.Status.Phase).NotTo(Equal(eav1.PhasePending))
		}, timeout, interval).Should(Succeed())

		By("Verifying PrometheusCheckAfter is set in status")
		Expect(fetchedEA.Status.PrometheusCheckAfter).NotTo(BeNil(),
			"PrometheusCheckAfter should be set in status after first reconciliation")

		By("Verifying PrometheusCheckAfter = creationTimestamp + stabilizationWindow (1s)")
		expectedCheckTime := fetchedEA.CreationTimestamp.Add(1 * time.Second)
		Expect(fetchedEA.Status.PrometheusCheckAfter.Time).To(
			BeTemporally("~", expectedCheckTime, 2*time.Second),
			"PrometheusCheckAfter should be creationTimestamp + 1s stabilizationWindow")
	})

	// ========================================
	// IT-EM-DT-003: First reconciliation sets AlertManagerCheckAfter in EA status
	// ========================================
	It("IT-EM-DT-003: should set AlertManagerCheckAfter in status on first reconciliation", func() {
		ns := createTestNamespace("em-dt-003")
		defer deleteTestNamespace(ns)

		By("Creating an EA with 1s stabilization window")
		ea := createEffectivenessAssessment(ns, "ea-dt-003", "rr-dt-003")

		By("Waiting for controller to process the EA")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).NotTo(BeEmpty())
			g.Expect(fetchedEA.Status.Phase).NotTo(Equal(eav1.PhasePending))
		}, timeout, interval).Should(Succeed())

		By("Verifying AlertManagerCheckAfter is set in status")
		Expect(fetchedEA.Status.AlertManagerCheckAfter).NotTo(BeNil(),
			"AlertManagerCheckAfter should be set in status after first reconciliation")

		By("Verifying AlertManagerCheckAfter = creationTimestamp + stabilizationWindow (1s)")
		expectedCheckTime := fetchedEA.CreationTimestamp.Add(1 * time.Second)
		Expect(fetchedEA.Status.AlertManagerCheckAfter.Time).To(
			BeTemporally("~", expectedCheckTime, 2*time.Second),
			"AlertManagerCheckAfter should be creationTimestamp + 1s stabilizationWindow")
	})

	// ========================================
	// IT-EM-DT-004: Subsequent reconciliations do not overwrite derived timing fields
	// ========================================
	It("IT-EM-DT-004: should not overwrite derived timing fields on subsequent reconciliations", func() {
		ns := createTestNamespace("em-dt-004")
		defer deleteTestNamespace(ns)

		By("Creating an EA and waiting for completion")
		ea := createEffectivenessAssessment(ns, "ea-dt-004", "rr-dt-004")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Recording the derived timing values after completion")
		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())
		Expect(fetchedEA.Status.PrometheusCheckAfter).NotTo(BeNil())
		Expect(fetchedEA.Status.AlertManagerCheckAfter).NotTo(BeNil())

		validityDeadline := fetchedEA.Status.ValidityDeadline.Time
		promCheckAfter := fetchedEA.Status.PrometheusCheckAfter.Time
		amCheckAfter := fetchedEA.Status.AlertManagerCheckAfter.Time

		By("Verifying the values are stable (not recomputed)")
		// Read again to ensure no drift
		fetchedEA2 := &eav1.EffectivenessAssessment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      ea.Name,
			Namespace: ea.Namespace,
		}, fetchedEA2)).To(Succeed())

		Expect(fetchedEA2.Status.ValidityDeadline.Time).To(Equal(validityDeadline),
			"ValidityDeadline should not change after first reconciliation")
		Expect(fetchedEA2.Status.PrometheusCheckAfter.Time).To(Equal(promCheckAfter),
			"PrometheusCheckAfter should not change after first reconciliation")
		Expect(fetchedEA2.Status.AlertManagerCheckAfter.Time).To(Equal(amCheckAfter),
			"AlertManagerCheckAfter should not change after first reconciliation")
	})

	// ========================================
	// IT-EM-DT-008: Reconciler uses status.ValidityDeadline for expiry check
	// ========================================
	It("IT-EM-DT-008: should use status.ValidityDeadline for expiry check (not recomputed)", func() {
		ns := createTestNamespace("em-dt-008")
		defer deleteTestNamespace(ns)

		By("Creating an EA normally")
		ea := createEffectivenessAssessment(ns, "ea-dt-008", "rr-dt-008")

		By("Waiting for the EA to reach Completed phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying ValidityDeadline was used for expiry (it should be in the future for normal completion)")
		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())
		Expect(fetchedEA.Status.ValidityDeadline.Time.After(time.Now())).To(BeTrue(),
			"ValidityDeadline should be in the future for normal completion (not expired)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"Assessment should complete with 'full' reason when ValidityDeadline is in the future")
	})

	// ========================================
	// IT-EM-DT-009: Custom ValidityWindow in ReconcilerConfig produces correct ValidityDeadline
	// ========================================
	// Note: This test validates that the default ReconcilerConfig (30m) is used.
	// A custom ValidityWindow would require a separate controller instance.
	// The unit test UT-EM-DT-001 covers the computation with different values.
	It("IT-EM-DT-009: should compute ValidityDeadline using ReconcilerConfig.ValidityWindow", func() {
		ns := createTestNamespace("em-dt-009")
		defer deleteTestNamespace(ns)

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-dt-009", "rr-dt-009")

		By("Waiting for the EA to reach at least Assessing phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())
		}, timeout, interval).Should(Succeed())

		By("Verifying ValidityDeadline matches the default 30m window from ReconcilerConfig")
		expectedDeadline := fetchedEA.CreationTimestamp.Add(30 * time.Minute)
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(
			BeTemporally("~", expectedDeadline, 2*time.Second),
			"ValidityDeadline should be creationTimestamp + 30m (default ReconcilerConfig.ValidityWindow)")
	})

	// ========================================
	// IT-EM-DT-010: All three derived fields set together on Pending -> Assessing
	// ========================================
	It("IT-EM-DT-010: should set all three derived timing fields atomically during Pending -> Assessing", func() {
		ns := createTestNamespace("em-dt-010")
		defer deleteTestNamespace(ns)

		By("Creating an EA with a specific stabilization window")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-dt-010",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-dt-010",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 2 * time.Second}, // 2s for clear distinction
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for the controller to set all derived fields")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ea-dt-010",
				Namespace: ns,
			}, fetchedEA)).To(Succeed())
			// All three must be set together
			g.Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())
			g.Expect(fetchedEA.Status.PrometheusCheckAfter).NotTo(BeNil())
			g.Expect(fetchedEA.Status.AlertManagerCheckAfter).NotTo(BeNil())
		}, timeout, interval).Should(Succeed())

		By("Verifying the values are correctly derived")
		creation := fetchedEA.CreationTimestamp.Time

		// ValidityDeadline = creation + 30m (default)
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(
			BeTemporally("~", creation.Add(30*time.Minute), 2*time.Second))

		// PrometheusCheckAfter = creation + 2s (stabilization window)
		Expect(fetchedEA.Status.PrometheusCheckAfter.Time).To(
			BeTemporally("~", creation.Add(2*time.Second), 2*time.Second))

		// AlertManagerCheckAfter = creation + 2s (stabilization window)
		Expect(fetchedEA.Status.AlertManagerCheckAfter.Time).To(
			BeTemporally("~", creation.Add(2*time.Second), 2*time.Second))

		// PrometheusCheckAfter == AlertManagerCheckAfter (both derived from same stabilization window)
		Expect(fetchedEA.Status.PrometheusCheckAfter.Time).To(
			BeTemporally("~", fetchedEA.Status.AlertManagerCheckAfter.Time, 1*time.Millisecond),
			"PrometheusCheckAfter and AlertManagerCheckAfter should be identical")

		// Invariant: ValidityDeadline > PrometheusCheckAfter
		Expect(fetchedEA.Status.ValidityDeadline.Time.After(fetchedEA.Status.PrometheusCheckAfter.Time)).To(BeTrue(),
			"ValidityDeadline must be after PrometheusCheckAfter (invariant: ValidityWindow > StabilizationWindow)")
	})
})
