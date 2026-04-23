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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// ════════════════════════════════════════════════════════════════════════════
// Issue #803: handleBlocked NotificationRequest Creation Tests
// ════════════════════════════════════════════════════════════════════════════
//
// Business Requirements: BR-ORCH-036 (manual review notification),
//                        BR-ORCH-042.5 (notification on block)
//
// These tests validate that handleBlocked creates a ManualReview
// NotificationRequest for IneffectiveChain blocks and does NOT create
// notifications for other block reasons.
// ════════════════════════════════════════════════════════════════════════════

var _ = Describe("Issue #803: handleBlocked NotificationRequest Creation", Label("BR-ORCH-036"), func() {

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-803-003: handleBlocked with IneffectiveChain creates ManualReview NR
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-803-003: should create ManualReview NR and append to NotificationRequestRefs when IneffectiveChain blocks", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-803-003", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-803-003", "ai-test-rr-803-003", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-803-003", "default", "test-rr-803-003", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-803-003", "default", "test-rr-803-003")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonIneffectiveChain),
				Message:      "3 consecutive ineffective remediations detected. Escalating to manual review.",
				RequeueAfter: 4 * time.Hour,
			},
		}

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			mockRouting,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-803-003", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-manual-review-test-rr-803-003",
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "ManualReview NR should exist after IneffectiveChain block")
		Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceRoutingEngine))
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))

		updatedRR := &remediationv1.RemediationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-803-003", Namespace: "default"}, updatedRR)
		Expect(err).ToNot(HaveOccurred())

		foundRef := false
		for _, ref := range updatedRR.Status.NotificationRequestRefs {
			if ref.Name == "nr-manual-review-test-rr-803-003" {
				foundRef = true
				break
			}
		}
		Expect(foundRef).To(BeTrue(), "NotificationRequestRefs should contain the manual review NR ref")
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-803-004: handleBlocked with IneffectiveChain emits NotificationCreated event
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-803-004: should emit NotificationCreated K8s event when IneffectiveChain creates NR", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-803-004", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-803-004", "ai-test-rr-803-004", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-803-004", "default", "test-rr-803-004", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-803-004", "default", "test-rr-803-004")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonIneffectiveChain),
				Message:      "Ineffective chain detected",
				RequeueAfter: 4 * time.Hour,
			},
		}

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			mockRouting,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-803-004", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonNotificationCreated)).To(BeTrue(),
			"Expected NotificationCreated event for IneffectiveChain NR, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-803-005: Non-IneffectiveChain blocks do NOT create NR
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-803-005: should NOT create ManualReview NR when blocked for ConsecutiveFailures", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-803-005", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-803-005", "ai-test-rr-803-005", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-803-005", "default", "test-rr-803-005", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-803-005", "default", "test-rr-803-005")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonConsecutiveFailures),
				Message:      "3 consecutive failures. Blocked for 1 hour.",
				RequeueAfter: 1 * time.Hour,
			},
		}

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			mockRouting,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-803-005", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nrList := &notificationv1.NotificationRequestList{}
		err = fakeClient.List(ctx, nrList)
		Expect(err).ToNot(HaveOccurred())
		for _, nr := range nrList.Items {
			Expect(nr.Name).ToNot(HavePrefix("nr-manual-review-"),
				"No ManualReview NR should exist for ConsecutiveFailures block, found: %s", nr.Name)
		}
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-803-006: Re-reconcile idempotency for IneffectiveChain NR
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-803-006: should NOT duplicate NR or events on re-reconcile of IneffectiveChain block", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-803-006", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-803-006", "ai-test-rr-803-006", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-803-006", "default", "test-rr-803-006", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-803-006", "default", "test-rr-803-006")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonIneffectiveChain),
				Message:      "Ineffective chain detected",
				RequeueAfter: 4 * time.Hour,
			},
		}

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			mockRouting,
		)

		// First reconcile
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-803-006", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		// Drain events from first reconcile
		drainEvents(recorder)

		// Second reconcile (re-reconcile on requeue)
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-803-006", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		// Verify still only 1 NR
		nrList := &notificationv1.NotificationRequestList{}
		err = fakeClient.List(ctx, nrList)
		Expect(err).ToNot(HaveOccurred())
		manualReviewCount := 0
		for _, nr := range nrList.Items {
			if nr.Name == "nr-manual-review-test-rr-803-006" {
				manualReviewCount++
			}
		}
		Expect(manualReviewCount).To(Equal(1), "Should still have exactly 1 ManualReview NR after re-reconcile")

		// Verify no duplicate NotificationCreated event from second reconcile
		evts := drainEvents(recorder)
		notifCreatedCount := 0
		for _, evt := range evts {
			if containsEvent([]string{evt}, events.EventReasonNotificationCreated) {
				notifCreatedCount++
			}
		}
		Expect(notifCreatedCount).To(Equal(0),
			"Should NOT emit NotificationCreated on re-reconcile (already exists), got events: %v", evts)
	})
})
