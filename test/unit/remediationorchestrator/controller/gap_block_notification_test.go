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
// BR-ORCH-036 GAP-6: Block Reason Notification Tests (PR C / #810)
// ════════════════════════════════════════════════════════════════════════════
//
// Business Requirements: BR-ORCH-036 (notification coverage), BR-ORCH-042.5 (notification on block)
// Issue: #810
//
// Six non-IneffectiveChain block reasons enter PhaseBlocked with no NR.
// These tests validate NR creation for each block reason:
// - ConsecutiveFailures, UnmanagedResource → Escalation NR (High)
// - DuplicateInProgress, ResourceBusy, RecentlyRemediated, ExponentialBackoff → StatusUpdate NR (Low)
// ════════════════════════════════════════════════════════════════════════════

var _ = Describe("BR-ORCH-036 GAP-6: Block reason notifications (#810)", func() {

	// ════════════════════════════════════════════════════════════════════════
	// Escalation block reasons (operator action likely needed)
	// ════════════════════════════════════════════════════════════════════════

	It("UT-RO-810-001: should create Escalation NR when blocked for ConsecutiveFailures", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-001"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		blockedUntil := time.Now().Add(1 * time.Hour)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonConsecutiveFailures),
				Message:      "3 consecutive failures detected. Blocked for 1 hour.",
				RequeueAfter: 1 * time.Hour,
				BlockedUntil: &blockedUntil,
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
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-block-consecutivefailures-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "Escalation NR should exist after ConsecutiveFailures block")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
		Expect(nr.Spec.Body).To(ContainSubstring("ConsecutiveFailures"))

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonNotificationCreated)).To(BeTrue(),
			"Expected NotificationCreated event, got: %v", evts)
	})

	It("UT-RO-810-002: should create Escalation NR when blocked for UnmanagedResource", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-002"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		blockedUntil := time.Now().Add(5 * time.Second)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonUnmanagedResource),
				Message:      "Resource not managed by Kubernaut",
				RequeueAfter: 5 * time.Second,
				BlockedUntil: &blockedUntil,
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
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-block-unmanagedresource-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "Escalation NR should exist after UnmanagedResource block")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
	})

	// ════════════════════════════════════════════════════════════════════════
	// Informational block reasons (transient / auto-clearing)
	// ════════════════════════════════════════════════════════════════════════

	It("UT-RO-810-003: should create StatusUpdate NR when blocked for DuplicateInProgress", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-003"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:  string(remediationv1.BlockReasonDuplicateInProgress),
				Message: "Duplicate RR rr-original is already being processed",
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
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-block-duplicateinprogress-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "StatusUpdate NR should exist after DuplicateInProgress block")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
	})

	It("UT-RO-810-004: should create StatusUpdate NR when blocked for ResourceBusy", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-004"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:  string(remediationv1.BlockReasonResourceBusy),
				Message: "Another WFE is running on the target",
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
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-block-resourcebusy-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "StatusUpdate NR should exist after ResourceBusy block")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
	})

	It("UT-RO-810-005: should create StatusUpdate NR when blocked for RecentlyRemediated", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-005"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		blockedUntil := time.Now().Add(5 * time.Minute)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonRecentlyRemediated),
				Message:      "Same workflow executed within cooldown window",
				RequeueAfter: 5 * time.Minute,
				BlockedUntil: &blockedUntil,
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
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-block-recentlyremediated-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "StatusUpdate NR should exist after RecentlyRemediated block")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
	})

	It("UT-RO-810-006: should create StatusUpdate NR when blocked for ExponentialBackoff", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-006"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		blockedUntil := time.Now().Add(2 * time.Minute)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonExponentialBackoff),
				Message:      "Infrastructure failure, backing off",
				RequeueAfter: 2 * time.Minute,
				BlockedUntil: &blockedUntil,
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
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-block-exponentialbackoff-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "StatusUpdate NR should exist after ExponentialBackoff block")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
	})

	// ════════════════════════════════════════════════════════════════════════
	// Idempotency test
	// ════════════════════════════════════════════════════════════════════════

	It("UT-RO-810-007: should NOT duplicate block NR on re-reconcile", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-810-007"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		blockedUntil := time.Now().Add(1 * time.Hour)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonConsecutiveFailures),
				Message:      "3 consecutive failures.",
				RequeueAfter: 1 * time.Hour,
				BlockedUntil: &blockedUntil,
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

		blockNRCount := 0
		for _, nr := range nrList.Items {
			if nr.Name == "nr-block-consecutivefailures-"+rrName {
				blockNRCount++
			}
		}
		Expect(blockNRCount).To(Equal(1), "Should have exactly 1 block NR after re-reconcile")

		evts := drainEvents(recorder)
		notifCreatedCount := 0
		for _, evt := range evts {
			if containsEvent([]string{evt}, events.EventReasonNotificationCreated) {
				notifCreatedCount++
			}
		}
		Expect(notifCreatedCount).To(Equal(0),
			"Should NOT emit NotificationCreated on re-reconcile, got events: %v", evts)
	})
})
