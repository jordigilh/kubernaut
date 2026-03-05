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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

var _ = Describe("Health Check — Issue #275: assessHealth must use RemediationTarget (BR-EM-001)", func() {

	// ========================================================================
	// IT-EM-275-001: SignalTarget is a deleted Pod, RemediationTarget is a
	// healthy Deployment — health score must be 1.0
	//
	// Bug: assessHealth() passes ea.Spec.SignalTarget to getTargetHealthStatus().
	// After a GracefulRestart, the original pod (SignalTarget) is deleted and
	// replaced by new pods in a new ReplicaSet. The Pod lookup fails
	// (TargetExists=false → score 0.0), even though the Deployment
	// (RemediationTarget) has all healthy pods.
	//
	// Fix: assessHealth() should use ea.Spec.RemediationTarget instead.
	// ========================================================================
	It("IT-EM-275-001: should score 1.0 when SignalTarget pod is deleted but RemediationTarget deployment is healthy (#275)", func() {
		ns := createTestNamespace("em-275-001")
		defer deleteTestNamespace(ns)

		By("Creating healthy pods labeled for Deployment 'test-app' (post-restart replicas)")
		for _, podName := range []string{"test-app-new-rs-abc", "test-app-new-rs-def"} {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: ns,
					Labels:    map[string]string{"app": "test-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "registry.k8s.io/pause:3.9"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "main", Ready: true, RestartCount: 0},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		}

		By("Creating EA with SignalTarget=Pod (deleted) and RemediationTarget=Deployment (healthy)")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-275-001",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-275-001",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind:      "Pod",
					Name:      "test-app-old-rs-xyz",
					Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying health score is 1.0 (Deployment pods healthy, not 0.0 from deleted Pod)")
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil())
		Expect(*fetchedEA.Status.Components.HealthScore).To(Equal(1.0),
			"health should use RemediationTarget (Deployment with 2 healthy pods), not SignalTarget (deleted Pod)")
	})

	// ========================================================================
	// IT-EM-275-002: SignalTarget and RemediationTarget are the same Deployment
	// — regression guard ensuring no change in behavior for same-target EAs.
	// ========================================================================
	It("IT-EM-275-002: should still score 1.0 when SignalTarget and RemediationTarget are the same Deployment (#275 regression)", func() {
		ns := createTestNamespace("em-275-002")
		defer deleteTestNamespace(ns)

		By("Creating a healthy pod for Deployment 'test-app'")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-pod",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 0},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Creating EA where both targets are the same Deployment (common case)")
		ea := createEffectivenessAssessment(ns, "ea-275-002", "rr-275-002")

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil())
		Expect(*fetchedEA.Status.Components.HealthScore).To(Equal(1.0),
			"same-target Deployment EA should still score 1.0 (regression guard)")
	})
})
