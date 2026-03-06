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
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// createEAWithRemediationTimestamp creates an EA with RemediationCreatedAt set,
// needed for #246 tests that verify restart counting relative to remediation time.
func createEAWithRemediationTimestamp(namespace, name, correlationID string, remediationCreatedAt *metav1.Time) *eav1.EffectivenessAssessment {
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
				Name:      "test-app",
				Namespace: namespace,
			},
			RemediationTarget: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
			},
			RemediationCreatedAt: remediationCreatedAt,
		},
	}
	Expect(k8sClient.Create(ctx, ea)).To(Succeed())
	GinkgoWriter.Printf("✅ Created EA with RemediationCreatedAt: %s/%s (ts: %v)\n", namespace, name, remediationCreatedAt)
	return ea
}

var _ = Describe("Health Check — Issue #246 (BR-EM-001)", func() {

	// ========================================================================
	// IT-EM-246-001: Terminating pod should be excluded from health assessment
	//
	// Bug A: getTargetHealthStatus includes terminating pods (DeletionTimestamp
	// set) in the pod list, inflating replica counts and restart counts.
	// A terminating pod with 5 restarts alongside a healthy pod with 0 restarts
	// should produce healthScore 1.0, not 0.75.
	// ========================================================================
	It("IT-EM-246-001: should score 1.0 when only active pod is healthy and terminating pod is excluded (#246)", func() {
		ns := createTestNamespace("em-246-001")
		defer deleteTestNamespace(ns)

		By("Creating a healthy active pod (post-remediation)")
		activePod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-active",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, activePod)).To(Succeed())
		activePod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 0},
			},
		}
		Expect(k8sClient.Status().Update(ctx, activePod)).To(Succeed())

		By("Creating a terminating pod with restarts (old pod being deleted)")
		terminatingPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-terminating",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, terminatingPod)).To(Succeed())
		terminatingPod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 5},
			},
		}
		Expect(k8sClient.Status().Update(ctx, terminatingPod)).To(Succeed())

		By("Marking the old pod as terminating (delete with grace period)")
		gracePeriod := client.GracePeriodSeconds(300)
		Expect(k8sClient.Delete(ctx, terminatingPod, gracePeriod)).To(Succeed())

		By("Creating EA targeting the deployment")
		ea := createEffectivenessAssessment(ns, "ea-246-001", "rr-246-001")

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
			"terminating pod restarts should be excluded; only healthy active pod counts → 1.0")
	})

	// ========================================================================
	// IT-EM-246-002: Pre-remediation pod cumulative RestartCount should not
	// inflate RestartsSinceRemediation
	//
	// Bug B: getTargetHealthStatus uses cs.RestartCount (cumulative lifetime)
	// for RestartsSinceRemediation. A pod created before RemediationCreatedAt
	// with 3 historical restarts but 0 restarts since remediation should not
	// lower the healthScore.
	// ========================================================================
	It("IT-EM-246-002: should score 1.0 when pre-remediation pod has cumulative restarts but none since remediation (#246)", func() {
		ns := createTestNamespace("em-246-002")
		defer deleteTestNamespace(ns)

		By("Creating a pod that existed before remediation (simulated via early creation timestamp)")
		preRemPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-pre-rem",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, preRemPod)).To(Succeed())
		preRemPod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 3},
			},
		}
		Expect(k8sClient.Status().Update(ctx, preRemPod)).To(Succeed())

		By("Creating EA with RemediationCreatedAt set to NOW (after the pod was created)")
		remediationTime := metav1.Now()
		ea := createEAWithRemediationTimestamp(ns, "ea-246-002", "rr-246-002", &remediationTime)

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
			"pre-remediation pod restarts (cumulative=3) should not count as restarts since remediation → 1.0")
	})

	// ========================================================================
	// IT-EM-246-003: Post-remediation pod restarts ARE correctly counted
	// (regression guard — should pass before AND after the fix)
	// ========================================================================
	It("IT-EM-246-003: should score 0.75 when post-remediation pod has genuine restarts (#246)", func() {
		ns := createTestNamespace("em-246-003")
		defer deleteTestNamespace(ns)

		By("Setting a remediation time in the past")
		pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))

		By("Creating a pod created AFTER remediation with real restarts")
		postRemPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-post-rem",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, postRemPod)).To(Succeed())
		postRemPod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 2},
			},
		}
		Expect(k8sClient.Status().Update(ctx, postRemPod)).To(Succeed())

		By("Creating EA with RemediationCreatedAt in the past (pod is post-remediation)")
		ea := createEAWithRemediationTimestamp(ns, "ea-246-003", "rr-246-003", &pastTime)

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
		Expect(*fetchedEA.Status.Components.HealthScore).To(Equal(0.75),
			"post-remediation pod with 2 restarts should score 0.75 (all ready, restarts detected)")
	})
})

