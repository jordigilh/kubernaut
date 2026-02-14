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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
)

// ============================================================================
// SPEC DRIFT GUARD INTEGRATION TESTS (DD-EM-002 v1.1)
// BR-EM-004: Spec drift detection during effectiveness assessment
//
// Strategy for IT-EM-SD-001 (drift detection):
// With mock Prometheus/AlertManager in INT, all components resolve in a single
// reconcile — the EA completes with "full" before we can modify the Deployment.
// To test drift, we:
//   1. Let the EA complete naturally with "full" (hash computed from spec A)
//   2. Patch the EA status back to Assessing with a FAKE PostRemediationSpecHash
//      This follows the same pattern as createExpiredEffectivenessAssessment in
//      suite_test.go: Status().Update() triggers reconciliation via the watch
//      (EM controller has no GenerationChangedPredicate).
//   3. The reconciler re-hashes the target (spec A) and detects mismatch => drift
// ============================================================================

var _ = Describe("Spec Drift Guard (DD-EM-002 v1.1)", func() {

	// Helper: create a standard test Deployment in the given namespace.
	createTestDeployment := func(ns string) *appsv1.Deployment {
		replicas := int32(1)
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: ns,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test-app"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "nginx:1.25",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deploy)).To(Succeed())
		return deploy
	}

	// ========================================
	// IT-EM-SD-001: Spec modification after hash -> EA completes with spec_drift
	// ========================================
	It("IT-EM-SD-001: should complete with spec_drift when target spec changes after hash computation", func() {
		ns := createTestNamespace("em-sd-001")
		defer deleteTestNamespace(ns)

		By("1. Creating a target Deployment in the namespace")
		createTestDeployment(ns)

		By("2. Creating an EA and waiting for it to complete naturally")
		ea := createEffectivenessAssessment(ns, "ea-sd-001", "rr-sd-001")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())
		GinkgoWriter.Printf("EA completed naturally with reason: %s, hash: %s\n",
			fetchedEA.Status.AssessmentReason, fetchedEA.Status.Components.PostRemediationSpecHash)

		realHash := fetchedEA.Status.Components.PostRemediationSpecHash
		Expect(realHash).NotTo(BeEmpty(), "PostRemediationSpecHash should have been computed")

		By("3. Patching EA status back to Assessing with a FAKE PostRemediationSpecHash")
		// This simulates: the RO recorded a hash from a DIFFERENT spec version
		// and the resource was subsequently modified (drift).
		// Pattern: same as createExpiredEffectivenessAssessment — Status().Update()
		// triggers reconciliation via the watch (no GenerationChangedPredicate).
		fakeOldHash := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		Expect(fakeOldHash).NotTo(Equal(realHash), "Fake hash must differ from real hash")

		// Use Eventually to handle 409 conflicts with the reconciler
		Eventually(func(g Gomega) {
			// Re-fetch to get latest resourceVersion
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())

			// Reset to Assessing with fake hash — reconciler will re-process
			deadline := metav1.NewTime(time.Now().Add(30 * time.Minute))
			checkAfter := metav1.NewTime(time.Now().Add(-1 * time.Second)) // already past

			fetchedEA.Status.Phase = eav1.PhaseAssessing
			fetchedEA.Status.CompletedAt = nil
			fetchedEA.Status.AssessmentReason = ""
			fetchedEA.Status.Message = ""
			fetchedEA.Status.ValidityDeadline = &deadline
			fetchedEA.Status.PrometheusCheckAfter = &checkAfter
			fetchedEA.Status.AlertManagerCheckAfter = &checkAfter
			fetchedEA.Status.Components.HashComputed = true
			fetchedEA.Status.Components.PostRemediationSpecHash = fakeOldHash
			fetchedEA.Status.Components.CurrentSpecHash = ""
			// Reset component assessed flags so reconciler re-evaluates
			fetchedEA.Status.Components.HealthAssessed = false
			fetchedEA.Status.Components.AlertAssessed = false
			fetchedEA.Status.Components.MetricsAssessed = false
			fetchedEA.Status.Components.HealthScore = nil
			fetchedEA.Status.Components.AlertScore = nil
			fetchedEA.Status.Components.MetricsScore = nil
			// Clear conditions
			fetchedEA.Status.Conditions = nil

			g.Expect(k8sClient.Status().Update(ctx, fetchedEA)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		GinkgoWriter.Println("Status patched: Phase=Assessing, PostRemediationSpecHash=fake")

		By("4. Waiting for EA to complete with spec_drift reason")
		// The Status().Update() triggers reconciliation via the watch.
		// The reconciler runs Step 6.5, computes currentHash from real spec,
		// detects mismatch with fakeOldHash, and completes with spec_drift.
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
			g.Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonSpecDrift))
		}, timeout, interval).Should(Succeed())

		By("5. Verifying CurrentSpecHash matches real hash (not fake)")
		Expect(fetchedEA.Status.Components.CurrentSpecHash).NotTo(BeEmpty(),
			"CurrentSpecHash should be set by drift guard")
		Expect(fetchedEA.Status.Components.CurrentSpecHash).To(Equal(realHash),
			"CurrentSpecHash should match the real Deployment spec hash")
		Expect(fetchedEA.Status.Components.CurrentSpecHash).NotTo(Equal(fakeOldHash),
			"CurrentSpecHash should differ from the fake PostRemediationSpecHash")

		By("6. Verifying SpecIntegrity condition is False with SpecDrifted reason")
		specCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionSpecIntegrity)
		Expect(specCond).NotTo(BeNil(), "SpecIntegrity condition should be set")
		Expect(specCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(specCond.Reason).To(Equal(conditions.ReasonSpecDrifted))

		By("7. Verifying AssessmentComplete condition is True with SpecDrift reason")
		completeCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAssessmentComplete)
		Expect(completeCond).NotTo(BeNil(), "AssessmentComplete condition should be set")
		Expect(completeCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(completeCond.Reason).To(Equal(conditions.ReasonSpecDrift))

		GinkgoWriter.Printf("IT-EM-SD-001: EA completed with spec_drift. Fake: %s, Current: %s\n",
			fakeOldHash, fetchedEA.Status.Components.CurrentSpecHash)
	})

	// ========================================
	// IT-EM-SD-002: No spec change -> EA completes normally (no drift)
	// ========================================
	It("IT-EM-SD-002: should complete normally when target spec is unchanged", func() {
		ns := createTestNamespace("em-sd-002")
		defer deleteTestNamespace(ns)

		By("1. Creating a target Deployment")
		createTestDeployment(ns)

		By("2. Creating an EA targeting the Deployment (spec unchanged)")
		ea := createEffectivenessAssessment(ns, "ea-sd-002", "rr-sd-002")

		By("3. Waiting for EA to complete (no spec change — should complete normally)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("4. Verifying assessment completed with full (not spec_drift)")
		Expect(fetchedEA.Status.AssessmentReason).NotTo(Equal(eav1.AssessmentReasonSpecDrift),
			"Assessment should NOT complete with spec_drift when spec is unchanged")

		By("5. Verifying SpecIntegrity condition is True (spec unchanged)")
		specCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionSpecIntegrity)
		Expect(specCond).NotTo(BeNil(), "SpecIntegrity condition should be set")
		Expect(specCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(specCond.Reason).To(Equal(conditions.ReasonSpecUnchanged))

		By("6. Verifying AssessmentComplete condition has a non-drift reason")
		completeCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAssessmentComplete)
		Expect(completeCond).NotTo(BeNil(), "AssessmentComplete condition should be set")
		Expect(completeCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(completeCond.Reason).NotTo(Equal(conditions.ReasonSpecDrift),
			"Condition reason should not be SpecDrift when spec is unchanged")
	})

	// ========================================
	// IT-EM-SD-003: No target resource -> hash from empty spec, no drift
	// ========================================
	It("IT-EM-SD-003: should complete normally when target resource does not exist", func() {
		ns := createTestNamespace("em-sd-003")
		defer deleteTestNamespace(ns)

		By("1. Creating an EA targeting a non-existent Deployment (no Deployment created)")
		ea := createEffectivenessAssessment(ns, "ea-sd-003", "rr-sd-003")

		By("2. Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("3. Verifying no spec drift (empty spec -> empty spec = same hash)")
		Expect(fetchedEA.Status.AssessmentReason).NotTo(Equal(eav1.AssessmentReasonSpecDrift),
			"Non-existent target should not trigger spec drift")
	})
})
