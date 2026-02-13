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

// DD-EVENT-001: EffectivenessMonitor K8s Event Observability Integration Tests
//
// BR-EM-005: Phase transition events
//
// These tests validate event emission in the context of the envtest framework
// with real EventRecorder (k8sManager.GetEventRecorderFor). They use the
// pattern: create EA → wait for Completed → list corev1.Events filtered
// by involvedObject.name → assert expected event reasons.
package effectivenessmonitor

import (
	"context"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// listEventsForObject returns corev1.Events for the given object name in the namespace,
// sorted by FirstTimestamp for deterministic ordering.
func listEventsForObject(ctx context.Context, c client.Client, objectName, namespace string) []corev1.Event {
	eventList := &corev1.EventList{}
	_ = c.List(ctx, eventList, client.InNamespace(namespace))
	var filtered []corev1.Event
	for _, evt := range eventList.Items {
		if evt.InvolvedObject.Name == objectName {
			filtered = append(filtered, evt)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].FirstTimestamp.Before(&filtered[j].FirstTimestamp)
	})
	return filtered
}

func eventReasons(evts []corev1.Event) []string {
	reasons := make([]string, len(evts))
	for i, e := range evts {
		reasons[i] = e.Reason
	}
	return reasons
}

func containsReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

var _ = Describe("K8s Event Observability (BR-EM-005, DD-EVENT-001)", func() {

	// ========================================
	// IT-EM-KE-001: K8s events recorded during reconcile
	// ========================================
	It("IT-EM-KE-001: should emit K8s events during reconcile lifecycle", func() {
		ns := createTestNamespace("em-ke-001")
		defer deleteTestNamespace(ns)

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-ke-001", "rr-ke-001")

		By("Waiting for the EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Listing K8s events for the EA")
		Eventually(func() bool {
			evts := listEventsForObject(ctx, k8sClient, ea.Name, ns)
			reasons := eventReasons(evts)
			// Should see at least AssessmentStarted and EffectivenessAssessed/RemediationIneffective
			return containsReason(reasons, "AssessmentStarted")
		}, 10*time.Second, interval).Should(BeTrue(),
			"should emit AssessmentStarted event")
	})

	// ========================================
	// IT-EM-KE-002: EffectivenessAssessed event on successful completion
	// ========================================
	It("IT-EM-KE-002: should emit EffectivenessAssessed event on successful completion", func() {
		ns := createTestNamespace("em-ke-002")
		defer deleteTestNamespace(ns)

		By("Creating a target pod so health score >= threshold")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-ke",
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

		By("Ensuring mock AM returns resolved (score 1.0)")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})

		By("Creating an EA with low scoring threshold so score >= threshold")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-ke-002",
				Namespace: ns,
				Labels: map[string]string{
					"kubernaut.ai/correlation-id": "rr-ke-002",
				},
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-ke-002",
				TargetResource: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					ScoringThreshold:    0.1, // Low threshold
					PrometheusEnabled:   true,
					AlertManagerEnabled: true,
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for the EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying EffectivenessAssessed event emitted")
		Eventually(func() bool {
			evts := listEventsForObject(ctx, k8sClient, ea.Name, ns)
			reasons := eventReasons(evts)
			return containsReason(reasons, "EffectivenessAssessed")
		}, 10*time.Second, interval).Should(BeTrue(),
			"should emit EffectivenessAssessed event when score >= threshold")
	})

	// ========================================
	// IT-EM-KE-003: RemediationIneffective event when score below threshold
	// ========================================
	It("IT-EM-KE-003: should emit RemediationIneffective when score below threshold", func() {
		ns := createTestNamespace("em-ke-003")
		defer deleteTestNamespace(ns)

		By("Ensuring NO target pod exists (health score = 0.0)")
		// No pod created -> health score 0.0

		By("Ensuring mock AM returns active alert (alert score = 0.0)")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{
			infrastructure.NewFiringAlert("HighCPU", map[string]string{
				"namespace": ns,
			}),
		})

		By("Creating an EA with high scoring threshold")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-ke-003",
				Namespace: ns,
				Labels: map[string]string{
					"kubernaut.ai/correlation-id": "rr-ke-003",
				},
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-ke-003",
				TargetResource: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					ScoringThreshold:    0.9, // High threshold - scores will be below this
					PrometheusEnabled:   true,
					AlertManagerEnabled: true,
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for the EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying RemediationIneffective event emitted")
		Eventually(func() bool {
			evts := listEventsForObject(ctx, k8sClient, ea.Name, ns)
			reasons := eventReasons(evts)
			return containsReason(reasons, "RemediationIneffective")
		}, 10*time.Second, interval).Should(BeTrue(),
			"should emit RemediationIneffective event when score < threshold")

		By("Restoring mock AM to default")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
	})

	// ========================================
	// IT-EM-KE-004: No K8s events emitted when EA already Completed (idempotency)
	// ========================================
	It("IT-EM-KE-004: should not emit duplicate events on re-reconcile of completed EA", func() {
		ns := createTestNamespace("em-ke-004")
		defer deleteTestNamespace(ns)

		By("Creating an EA and waiting for completion")
		ea := createEffectivenessAssessment(ns, "ea-ke-004", "rr-ke-004")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Recording initial event count")
		time.Sleep(2 * time.Second) // Allow events to propagate
		initialEvents := listEventsForObject(ctx, k8sClient, ea.Name, ns)
		initialCount := len(initialEvents)

		By("Waiting additional time for any spurious reconciles")
		time.Sleep(5 * time.Second)

		By("Verifying no additional events were emitted")
		finalEvents := listEventsForObject(ctx, k8sClient, ea.Name, ns)
		finalCount := len(finalEvents)

		Expect(finalCount).To(Equal(initialCount),
			"no additional events should be emitted for already-completed EA")
	})
})
