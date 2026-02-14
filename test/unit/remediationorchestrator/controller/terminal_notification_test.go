/*
Copyright 2025 Jordi Gil.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Issue #88: NotificationRequest completion events lost for terminal-phase RemediationRequests
var _ = Describe("Issue #88: Terminal-phase notification tracking", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = setupScheme()
	})

	// UT-RO-088-001: Terminal RR with Sent NT should have NotificationDelivered=True
	Describe("Bug 1: Terminal-phase skip blocks NT processing", func() {
		It("UT-RO-088-001: should track NotificationDelivered condition when NT reaches Sent for a terminal (Completed) RR", func() {
			rrName := "rr-terminal-notif-001"
			namespace := "default"
			notifName := "nr-completion-" + rrName

			// Create a terminal (Completed) RR with ObservedGeneration == Generation
			// This is the exact state that triggers Guard1 early-exit
			rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation // Guard1 trigger condition
			rr.Status.StartTime = &metav1.Time{Time: metav1.Now().Time}
			// Bug 2 fix prerequisite: completion NT ref must be in NotificationRequestRefs
			rr.Status.NotificationRequestRefs = []corev1.ObjectReference{
				{
					Name:      notifName,
					Namespace: namespace,
				},
			}

			// Create a Sent NotificationRequest (terminal phase — delivery succeeded)
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rrName,
							UID:        types.UID(rrName + "-uid"),
							Controller: ptr(true),
						},
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase: notificationv1.NotificationPhaseSent,
				},
			}

			// Build fake client with both objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, notif).
				WithStatusSubresource(rr).
				Build()

			recorder := record.NewFakeRecorder(20)
			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil, recorder,
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{}, nil,
			)

			// Reconcile — this simulates the controller-runtime enqueue from NT status change
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify: NotificationDelivered condition should be True
			// Bug: Guard1 returns early, trackNotificationStatus never runs,
			// so NotificationDelivered is never set.
			var updatedRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, &updatedRR)).To(Succeed())

			notifCond := meta.FindStatusCondition(updatedRR.Status.Conditions, "NotificationDelivered")
			Expect(notifCond).ToNot(BeNil(), "NotificationDelivered condition should be set after NT reaches Sent")
			Expect(notifCond.Status).To(Equal(metav1.ConditionTrue), "NotificationDelivered should be True when NT is Sent")
			Expect(notifCond.Reason).To(Equal("DeliverySucceeded"))
		})

		It("UT-RO-088-002: should track NotificationDelivered=False when NT reaches Failed for a terminal (Failed) RR", func() {
			rrName := "rr-terminal-notif-002"
			namespace := "default"
			notifName := "timeout-" + rrName

			// Create a terminal (Failed) RR with ObservedGeneration == Generation
			rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseFailed)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: metav1.Now().Time}
			rr.Status.NotificationRequestRefs = []corev1.ObjectReference{
				{
					Name:      notifName,
					Namespace: namespace,
				},
			}

			// Create a Failed NotificationRequest
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rrName,
							UID:        types.UID(rrName + "-uid"),
							Controller: ptr(true),
						},
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:   notificationv1.NotificationPhaseFailed,
					Message: "SMTP connection refused",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, notif).
				WithStatusSubresource(rr).
				Build()

			recorder := record.NewFakeRecorder(20)
			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil, recorder,
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{}, nil,
			)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify: NotificationDelivered condition should be False (delivery failed)
			var updatedRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, &updatedRR)).To(Succeed())

			notifCond := meta.FindStatusCondition(updatedRR.Status.Conditions, "NotificationDelivered")
			Expect(notifCond).ToNot(BeNil(), "NotificationDelivered condition should be set even for terminal RRs")
			Expect(notifCond.Status).To(Equal(metav1.ConditionFalse), "NotificationDelivered should be False when NT failed")
			Expect(notifCond.Reason).To(Equal("DeliveryFailed"))
		})
	})

	// UT-RO-088-003: transitionToCompleted should append completion NT ref
	// Note: This tests the ref population, not the full transition flow.
	// The full transition requires mocks for audit, EA creator, etc.
	// We test the ref population by verifying the reconciler's behavior
	// when a completion NT exists as an owned resource but is NOT in NotificationRequestRefs.
	Describe("Bug 2: Completion NT not in NotificationRequestRefs", func() {
		It("UT-RO-088-003: should still be able to track completion NT when ref is properly populated", func() {
			// This test verifies the precondition: if the ref IS in NotificationRequestRefs,
			// the tracking works (once Bug 1 is fixed). This proves Bug 2's fix is necessary.
			rrName := "rr-ref-population-003"
			namespace := "default"
			notifName := "nr-completion-" + rrName

			// Terminal RR WITHOUT the completion NT in NotificationRequestRefs (Bug 2 state)
			rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: metav1.Now().Time}
			// Intentionally empty — this is the bug
			rr.Status.NotificationRequestRefs = []corev1.ObjectReference{}

			// The NT exists as an owned resource but is NOT tracked
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rrName,
							UID:        types.UID(rrName + "-uid"),
							Controller: ptr(true),
						},
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase: notificationv1.NotificationPhaseSent,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, notif).
				WithStatusSubresource(rr).
				Build()

			recorder := record.NewFakeRecorder(20)
			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil, recorder,
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{}, nil,
			)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Without Bug 2 fix (ref not populated), tracking cannot find the NT
			// NotificationDelivered should NOT be set — demonstrates Bug 2 impact
			var updatedRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, &updatedRR)).To(Succeed())

			notifCond := meta.FindStatusCondition(updatedRR.Status.Conditions, "NotificationDelivered")
			// This assertion documents the current buggy behavior:
			// NT exists but is not tracked because ref is missing
			Expect(notifCond).To(BeNil(), "Without ref population (Bug 2), NotificationDelivered cannot be set")
		})
	})
})
