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
// BR-ORCH-036 Gap Remediation: Terminal Failure Notification Tests (PR B)
// ════════════════════════════════════════════════════════════════════════════
//
// Business Requirements: BR-ORCH-036 (notification on terminal failure)
// Issues: #808 (GAP-4), #809 (GAP-5)
//
// GAP-4: transitionToFailed creates Escalation NR with double-NR guard
// GAP-5: transitionToFailedTerminal creates Escalation NR on cooldown expiry
// ════════════════════════════════════════════════════════════════════════════

var _ = Describe("BR-ORCH-036 GAP-4: transitionToFailed Escalation NR (#808)", func() {

	It("UT-RO-808-001: should create Escalation NR when SP fails and transitions to Failed", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-808-001"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseProcessing, "sp-"+rrName, "", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		procStart := metav1.Now()
		rr.Status.ProcessingStartTime = &procStart

		sp := newSignalProcessingFailed("sp-"+rrName, "default", rrName, "Signal processing failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-escalation-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "Escalation NR should exist after SP failure transition")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
	})

	It("UT-RO-808-002: should emit NotificationCreated event for escalation NR", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-808-002"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseProcessing, "sp-"+rrName, "", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		procStart := metav1.Now()
		rr.Status.ProcessingStartTime = &procStart

		sp := newSignalProcessingFailed("sp-"+rrName, "default", rrName, "SP failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonNotificationCreated)).To(BeTrue(),
			"Expected NotificationCreated event for escalation NR, got: %v", evts)
	})

	It("UT-RO-808-003: should NOT create Escalation NR when ManualReview NR already exists (double-NR guard)", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-808-003"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseExecuting, "sp-"+rrName, "ai-"+rrName, "we-"+rrName)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		execStart := metav1.Now()
		rr.Status.ExecutingStartTime = &execStart

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)
		we := newWorkflowExecutionFailed("we-"+rrName, "default", rrName, "Pipeline failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, we).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		// WFE failure creates ManualReview NR (GAP-3), then transitionToFailed
		// should NOT create a duplicate Escalation NR.
		nrList := &notificationv1.NotificationRequestList{}
		err = fakeClient.List(ctx, nrList)
		Expect(err).ToNot(HaveOccurred())

		manualReviewCount := 0
		escalationCount := 0
		for _, nr := range nrList.Items {
			if nr.Spec.Type == notificationv1.NotificationTypeManualReview {
				manualReviewCount++
			}
			if nr.Spec.Type == notificationv1.NotificationTypeEscalation {
				escalationCount++
			}
		}
		Expect(manualReviewCount).To(Equal(1), "Should have exactly 1 ManualReview NR from WFE failure")
		Expect(escalationCount).To(Equal(0), "Should NOT have Escalation NR when ManualReview NR exists (double-NR guard)")
	})

	It("UT-RO-808-004: should NOT duplicate Escalation NR on re-reconcile", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-808-004"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseProcessing, "sp-"+rrName, "", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		procStart := metav1.Now()
		rr.Status.ProcessingStartTime = &procStart

		sp := newSignalProcessingFailed("sp-"+rrName, "default", rrName, "SP failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		// First reconcile
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		drainEvents(recorder)

		// Second reconcile
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nrList := &notificationv1.NotificationRequestList{}
		err = fakeClient.List(ctx, nrList)
		Expect(err).ToNot(HaveOccurred())
		escalationCount := 0
		for _, nr := range nrList.Items {
			if nr.Name == "nr-escalation-"+rrName {
				escalationCount++
			}
		}
		Expect(escalationCount).To(Equal(1), "Should have exactly 1 Escalation NR after re-reconcile")
	})
})

var _ = Describe("BR-ORCH-036 GAP-5: transitionToFailedTerminal Escalation NR (#809)", func() {

	It("UT-RO-809-001: should create Escalation NR when cooldown expires and transitions to terminal Failed", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-809-001"
		rr := newRemediationRequest(rrName, "default", remediationv1.PhaseBlocked)
		rr.Status.StartTime = &metav1.Time{Time: time.Now().Add(-30 * time.Minute)}
		expiredTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		rr.Status.BlockedUntil = &expiredTime
		rr.Status.BlockReason = remediationv1.BlockReasonConsecutiveFailures

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-escalation-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "Escalation NR should exist after cooldown expiry")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonNotificationCreated)).To(BeTrue(),
			"Expected NotificationCreated event for cooldown expiry escalation, got: %v", evts)
	})
})
