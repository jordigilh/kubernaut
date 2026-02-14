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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

var _ = Describe("Health Check Integration (BR-EM-001)", func() {

	// ========================================
	// IT-EM-HC-001: EA for healthy pod -> health score 1.0
	// ========================================
	It("IT-EM-HC-001: should score 1.0 for healthy running pod with all replicas ready", func() {
		ns := createTestNamespace("em-hc-001")
		defer deleteTestNamespace(ns)

		By("Creating a healthy target pod with 'app' label matching the EA target")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-healthy",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Simulating pod readiness via status update")
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 0,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Creating an EA targeting the healthy pod")
		ea := createEffectivenessAssessment(ns, "ea-hc-001", "rr-hc-001")

		By("Verifying the EA completes with health score 1.0")
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
			"healthy pod with all replicas ready and no restarts should score 1.0")
	})

	// ========================================
	// IT-EM-HC-002: EA for unhealthy pod (not ready) -> health score < 1.0
	// ========================================
	It("IT-EM-HC-002: should score < 1.0 for pod that is running but not ready", func() {
		ns := createTestNamespace("em-hc-002")
		defer deleteTestNamespace(ns)

		By("Creating a pod that is running but not ready")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-not-ready",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Simulating pod running but NOT ready")
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        false,
					RestartCount: 0,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Creating an EA targeting the not-ready pod")
		ea := createEffectivenessAssessment(ns, "ea-hc-002", "rr-hc-002")

		By("Verifying the EA completes with health score 0.0 (no ready replicas)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil())
		// Not ready pod: readyReplicas=0, totalReplicas=1 -> score 0.0
		Expect(*fetchedEA.Status.Components.HealthScore).To(Equal(0.0),
			"pod running but not ready should score 0.0")
	})

	// ========================================
	// IT-EM-HC-003: EA for pod not running -> health score 0.0
	// ========================================
	It("IT-EM-HC-003: should score 0.0 for pod in Pending phase", func() {
		ns := createTestNamespace("em-hc-003")
		defer deleteTestNamespace(ns)

		By("Creating a pod in Pending phase (no container statuses)")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-pending",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Simulating Pending pod (no container statuses)")
		pod.Status = corev1.PodStatus{
			Phase:             corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Creating an EA targeting the pending pod")
		ea := createEffectivenessAssessment(ns, "ea-hc-003", "rr-hc-003")

		By("Verifying the EA completes with health score 0.0")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil())
		Expect(*fetchedEA.Status.Components.HealthScore).To(Equal(0.0),
			"pending pod with no ready containers should score 0.0")
	})

	// ========================================
	// IT-EM-HC-004: EA target pod has restart delta > 0 -> health score 0.75
	// ========================================
	It("IT-EM-HC-004: should score 0.75 for healthy pod with restarts", func() {
		ns := createTestNamespace("em-hc-004")
		defer deleteTestNamespace(ns)

		By("Creating a pod that is ready but has restarts")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-restarts",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Simulating pod ready with restart count > 0")
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 3, // Restarts since remediation
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Creating an EA targeting the pod with restarts")
		ea := createEffectivenessAssessment(ns, "ea-hc-004", "rr-hc-004")

		By("Verifying the EA completes with health score 0.75")
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
			"all pods ready but with restarts should score 0.75")
	})

	// ========================================
	// IT-EM-HC-005: Health event payload verified (correlation_id, sub-checks, score)
	// ========================================
	It("IT-EM-HC-005: should include correlation ID and complete component data in status", func() {
		ns := createTestNamespace("em-hc-005")
		defer deleteTestNamespace(ns)

		correlationID := fmt.Sprintf("rr-hc-005-%d", time.Now().UnixNano())

		By("Creating a target pod")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-payload",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
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

		By("Creating an EA with a unique correlationID")
		ea := createEffectivenessAssessment(ns, "ea-hc-005", correlationID)

		By("Verifying the EA completes and has correct correlation data")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Verify correlation ID preserved in spec
		Expect(fetchedEA.Spec.CorrelationID).To(Equal(correlationID))

		// Verify health component data
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil())

		// Verify assessment metadata
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
		Expect(fetchedEA.Status.AssessmentReason).NotTo(BeEmpty())
		Expect(fetchedEA.Status.Message).NotTo(BeEmpty())
	})

	// ========================================
	// IT-EM-HC-006: Target resource deleted between EA creation and assessment -> health score 0.0
	// ========================================
	It("IT-EM-HC-006: should score 0.0 when target pod does not exist", func() {
		ns := createTestNamespace("em-hc-006")
		defer deleteTestNamespace(ns)

		By("Creating an EA without any matching target pod (no pod with app=test-app)")
		ea := createEffectivenessAssessment(ns, "ea-hc-006", "rr-hc-006")

		By("Verifying the EA completes with health score 0.0 (target not found)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil())
		Expect(*fetchedEA.Status.Components.HealthScore).To(Equal(0.0),
			"target not found should score 0.0")
	})
})
