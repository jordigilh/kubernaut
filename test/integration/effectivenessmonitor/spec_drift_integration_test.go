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
//   1. Create EA with a stabilization window so the reconciler enters Stabilizing
//      phase and schedules a RequeueAfter
//   2. Patch the EA status to Assessing with a FAKE PostRemediationSpecHash
//      while the RequeueAfter timer is pending (within the stabilization window)
//   3. When the RequeueAfter fires, the reconciler re-hashes the target and
//      detects the mismatch => drift
//
// Note: The EM controller uses GenerationChangedPredicate (Issue #1466) to
// filter status-only watch events. Status().Update() alone does NOT trigger
// reconciliation. The RequeueAfter timer from the Stabilizing phase provides
// the guaranteed reconciliation trigger.
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
		ns := createTestNamespace(ctx, "em-sd-001")
		defer deleteTestNamespace(ns)

		By("1. Creating a target Deployment in the namespace")
		createTestDeployment(ns)

		By("2. Creating an EA with stabilization window to allow status injection")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-sd-001",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-sd-001",
				RemediationRequestPhase: "Verifying",
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
					StabilizationWindow: metav1.Duration{Duration: 5 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("3. Waiting for EA to enter Stabilizing (RequeueAfter scheduled)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseStabilizing))
		}, timeout, interval).Should(Succeed())
		GinkgoWriter.Println("EA entered Stabilizing — RequeueAfter timer is pending")

		By("4. Injecting Assessing state with a FAKE PostRemediationSpecHash")
		fakeOldHash := "sha256:0000000000000000000000000000000000000000000000000000000000000000"

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())

			deadline := metav1.NewTime(time.Now().Add(30 * time.Minute))
			checkAfter := metav1.NewTime(time.Now().Add(-1 * time.Second))

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
			fetchedEA.Status.Components.HealthAssessed = false
			fetchedEA.Status.Components.AlertAssessed = false
			fetchedEA.Status.Components.MetricsAssessed = false
			fetchedEA.Status.Components.HealthScore = nil
			fetchedEA.Status.Components.AlertScore = nil
			fetchedEA.Status.Components.MetricsScore = nil
			fetchedEA.Status.Conditions = nil

			g.Expect(k8sClient.Status().Update(ctx, fetchedEA)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		GinkgoWriter.Println("Status injected: Phase=Assessing, PostRemediationSpecHash=fake")

		By("5. Waiting for EA to complete with spec_drift reason")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
			g.Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonSpecDrift))
		}, timeout, interval).Should(Succeed())

		By("6. Verifying CurrentSpecHash was computed and differs from fake")
		Expect(fetchedEA.Status.Components.CurrentSpecHash).NotTo(BeEmpty(),
			"CurrentSpecHash should be set by drift guard")
		Expect(fetchedEA.Status.Components.CurrentSpecHash).NotTo(Equal(fakeOldHash),
			"CurrentSpecHash should differ from the fake PostRemediationSpecHash")

		By("7. Verifying SpecIntegrity condition is False with SpecDrifted reason")
		specCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionSpecIntegrity)
		Expect(specCond).NotTo(BeNil(), "SpecIntegrity condition should be set")
		Expect(specCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(specCond.Reason).To(Equal(conditions.ReasonSpecDrifted))

		By("8. Verifying AssessmentComplete condition is True with SpecDrift reason")
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
		ns := createTestNamespace(ctx, "em-sd-002")
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
		ns := createTestNamespace(ctx, "em-sd-003")
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
