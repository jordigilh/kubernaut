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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// ════════════════════════════════════════════════════════════════════════════
// DD-EVENT-001 v1.1: RemediationOrchestrator K8s Event Emission Tests
// ════════════════════════════════════════════════════════════════════════════
//
// Business Requirement: BR-ORCH-095 (K8s Event Observability)
// Design Decision: DD-EVENT-001 v1.1 (Controller Kubernetes Event Registry)
// Test Plan: TP-EVENT-RO.md
//
// These tests validate that the RO controller emits Kubernetes Events at
// the correct lifecycle points using the centralized event reason constants
// from pkg/shared/events/reasons.go.
//
// Strategy: Unit tests use FakeRecorder to capture events, then assert
// event reason strings appear in the channel.
// ════════════════════════════════════════════════════════════════════════════

// drainEvents reads all pending events from a FakeRecorder channel.
func drainEvents(recorder *record.FakeRecorder) []string {
	var collected []string
	for {
		select {
		case evt := <-recorder.Events:
			collected = append(collected, evt)
		default:
			return collected
		}
	}
}

// containsEvent checks if any event string contains the given reason substring.
func containsEvent(evts []string, reason string) bool {
	for _, evt := range evts {
		if strings.Contains(evt, reason) {
			return true
		}
	}
	return false
}

var _ = Describe("DD-EVENT-001: RemediationOrchestrator K8s Event Emission", func() {

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-01: RemediationCreated event on new RR accepted
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-01: should emit RemediationCreated when new RR is accepted", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create new RR with empty phase (first reconcile)
		rr := newRemediationRequest("test-rr-created", "default", "")
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

		// When: First reconcile initializes the RR
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-created", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		// Then: RemediationCreated event should be emitted
		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonRemediationCreated)).To(BeTrue(),
			"Expected RemediationCreated event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-02: RemediationCompleted event on successful completion
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-02: should emit RemediationCompleted when RR completes successfully", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in Executing phase with completed WE child
		rr := newRemediationRequestWithChildRefs("test-rr-completed", "default",
			remediationv1.PhaseExecuting, "sp-test-rr-completed", "ai-test-rr-completed", "we-test-rr-completed")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		we := newWorkflowExecutionCompleted("we-test-rr-completed", "default", "test-rr-completed")
		ai := newAIAnalysisCompleted("ai-test-rr-completed", "default", "test-rr-completed", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-completed", "default", "test-rr-completed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we, ai, sp).
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
			NamespacedName: types.NamespacedName{Name: "test-rr-completed", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonRemediationCompleted)).To(BeTrue(),
			"Expected RemediationCompleted event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-03: RemediationFailed event on failure
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-03: should emit RemediationFailed when RR fails", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in Executing phase with failed WE child
		rr := newRemediationRequestWithChildRefs("test-rr-failed", "default",
			remediationv1.PhaseExecuting, "sp-test-rr-failed", "ai-test-rr-failed", "we-test-rr-failed")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		we := newWorkflowExecutionFailed("we-test-rr-failed", "default", "test-rr-failed", "Job failed")
		ai := newAIAnalysisCompleted("ai-test-rr-failed", "default", "test-rr-failed", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-failed", "default", "test-rr-failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we, ai, sp).
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
			NamespacedName: types.NamespacedName{Name: "test-rr-failed", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonRemediationFailed)).To(BeTrue(),
			"Expected RemediationFailed event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-04: RemediationTimeout event on global timeout
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-04: should emit RemediationTimeout when global timeout expires", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR that has exceeded global timeout (started 2 hours ago)
		rr := newRemediationRequestWithTimeout("test-rr-timeout", "default",
			remediationv1.PhaseProcessing, -2*time.Hour)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-timeout", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonRemediationTimeout)).To(BeTrue(),
			"Expected RemediationTimeout event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-05: ApprovalRequired event when RAR is created
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-05: should emit ApprovalRequired when low confidence triggers approval", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in Analyzing phase with completed AI that requires approval (low confidence)
		rr := newRemediationRequestWithChildRefs("test-rr-approval", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-approval", "ai-test-rr-approval", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		// Low confidence (0.4 < 0.8) triggers approval
		ai := newAIAnalysisCompleted("ai-test-rr-approval", "default", "test-rr-approval", 0.4, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-approval", "default", "test-rr-approval")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&remediationv1.RemediationApprovalRequest{},
			).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-approval", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonApprovalRequired)).To(BeTrue(),
			"Expected ApprovalRequired event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-06: ApprovalGranted event when RAR is approved
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-06: should emit ApprovalGranted when RAR is approved", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in AwaitingApproval with approved RAR
		rr := newRemediationRequestWithChildRefs("test-rr-granted", "default",
			remediationv1.PhaseAwaitingApproval, "sp-test-rr-granted", "ai-test-rr-granted", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-granted", "default", "test-rr-granted", 0.4, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-granted", "default", "test-rr-granted")
		rar := newRemediationApprovalRequestApproved("rar-test-rr-granted", "default", "test-rr-granted", "admin@example.com")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, rar).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&remediationv1.RemediationApprovalRequest{},
			).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-granted", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonApprovalGranted)).To(BeTrue(),
			"Expected ApprovalGranted event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-07: ApprovalRejected event when RAR is rejected
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-07: should emit ApprovalRejected when RAR is rejected", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-rejected", "default",
			remediationv1.PhaseAwaitingApproval, "sp-test-rr-rejected", "ai-test-rr-rejected", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-rejected", "default", "test-rr-rejected", 0.4, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-rejected", "default", "test-rr-rejected")
		rar := newRemediationApprovalRequestRejected("rar-test-rr-rejected", "default", "test-rr-rejected", "admin@example.com", "Risk too high")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, rar).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&remediationv1.RemediationApprovalRequest{},
			).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-rejected", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonApprovalRejected)).To(BeTrue(),
			"Expected ApprovalRejected event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-08: ApprovalExpired event when RAR deadline passes
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-08: should emit ApprovalExpired when RAR deadline passes", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-expired", "default",
			remediationv1.PhaseAwaitingApproval, "sp-test-rr-expired", "ai-test-rr-expired", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-expired", "default", "test-rr-expired", 0.4, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-expired", "default", "test-rr-expired")
		rar := newRemediationApprovalRequestExpired("rar-test-rr-expired", "default", "test-rr-expired")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, rar).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&remediationv1.RemediationApprovalRequest{},
			).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-expired", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonApprovalExpired)).To(BeTrue(),
			"Expected ApprovalExpired event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-09: EscalatedToManualReview event on manual review escalation
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-09: should emit EscalatedToManualReview when AI analysis requires manual review", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in Analyzing phase with failed AI that has NeedsHumanReview
		rr := newRemediationRequestWithChildRefs("test-rr-escalated", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-escalated", "ai-test-rr-escalated", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		// Failed AI with NeedsHumanReview triggers manual review escalation
		ai := newAIAnalysisFailed("ai-test-rr-escalated", "default", "test-rr-escalated", "Insufficient context for automated analysis")
		ai.Status.NeedsHumanReview = true
		sp := newSignalProcessingCompleted("sp-test-rr-escalated", "default", "test-rr-escalated")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
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
			NamespacedName: types.NamespacedName{Name: "test-rr-escalated", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonEscalatedToManualReview)).To(BeTrue(),
			"Expected EscalatedToManualReview event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-11: NotificationCreated event when notification is created
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-11: should emit NotificationCreated when timeout notification is created", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR that has exceeded global timeout - this triggers notification creation
		rr := newRemediationRequestWithTimeout("test-rr-notif", "default",
			remediationv1.PhaseProcessing, -2*time.Hour)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-notif", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonNotificationCreated)).To(BeTrue(),
			"Expected NotificationCreated event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-12: CooldownActive event when routing blocks with cooldown
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-12: should emit CooldownActive when routing blocks due to cooldown", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in Analyzing phase with completed AI (high confidence, auto-approve)
		rr := newRemediationRequestWithChildRefs("test-rr-cooldown", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-cooldown", "ai-test-rr-cooldown", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-cooldown", "default", "test-rr-cooldown", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-cooldown", "default", "test-rr-cooldown")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		// Use a mock routing engine that returns RecentlyRemediated block
		mockRouting := &MockBlockingRoutingEngine{
			BlockCondition: &MockBlockCondition{
				Reason:       string(remediationv1.BlockReasonRecentlyRemediated),
				Message:      "Recently remediated. Cooldown active.",
				RequeueAfter: 5 * time.Minute,
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
			NamespacedName: types.NamespacedName{Name: "test-rr-cooldown", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonCooldownActive)).To(BeTrue(),
			"Expected CooldownActive event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-13: ConsecutiveFailureBlocked event
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-13: should emit ConsecutiveFailureBlocked when routing blocks due to consecutive failures", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rr := newRemediationRequestWithChildRefs("test-rr-consec", "default",
			remediationv1.PhaseAnalyzing, "sp-test-rr-consec", "ai-test-rr-consec", "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-test-rr-consec", "default", "test-rr-consec", 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-test-rr-consec", "default", "test-rr-consec")

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
			NamespacedName: types.NamespacedName{Name: "test-rr-consec", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonConsecutiveFailureBlocked)).To(BeTrue(),
			"Expected ConsecutiveFailureBlocked event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// UT-RO-095-14: PhaseTransition event on intermediate phase changes
	// ════════════════════════════════════════════════════════════════════════
	It("UT-RO-095-14: should emit PhaseTransition on intermediate phase change", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		// Create RR in Pending phase with StartTime set (past initialization)
		rr := newRemediationRequest("test-rr-transition", "default", remediationv1.PhasePending)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

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

		// When: Reconcile transitions Pending -> Processing
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-transition", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonPhaseTransition)).To(BeTrue(),
			"Expected PhaseTransition event, got: %v", evts)
	})

	// ════════════════════════════════════════════════════════════════════════
	// Constant verification tests (compile-time checks)
	// ════════════════════════════════════════════════════════════════════════
	Context("Event reason constants exist (compile-time verification)", func() {
		It("should have all RO event reason constants defined", func() {
			// These are compile-time checks - if constants don't exist, this won't compile
			Expect(events.EventReasonRemediationCreated).To(Equal("RemediationCreated"))
			Expect(events.EventReasonRemediationCompleted).To(Equal("RemediationCompleted"))
			Expect(events.EventReasonRemediationFailed).To(Equal("RemediationFailed"))
			Expect(events.EventReasonRemediationTimeout).To(Equal("RemediationTimeout"))
			Expect(events.EventReasonApprovalRequired).To(Equal("ApprovalRequired"))
			Expect(events.EventReasonApprovalGranted).To(Equal("ApprovalGranted"))
			Expect(events.EventReasonApprovalRejected).To(Equal("ApprovalRejected"))
			Expect(events.EventReasonApprovalExpired).To(Equal("ApprovalExpired"))
			Expect(events.EventReasonEscalatedToManualReview).To(Equal("EscalatedToManualReview"))
			Expect(events.EventReasonNotificationCreated).To(Equal("NotificationCreated"))
			Expect(events.EventReasonCooldownActive).To(Equal("CooldownActive"))
			Expect(events.EventReasonConsecutiveFailureBlocked).To(Equal("ConsecutiveFailureBlocked"))
			Expect(events.EventReasonPhaseTransition).To(Equal("PhaseTransition"))
		})
	})
})

// ════════════════════════════════════════════════════════════════════════════
// MockBlockingRoutingEngine - routing engine that returns blocking conditions
// ════════════════════════════════════════════════════════════════════════════

// MockBlockCondition represents a mock blocking condition
type MockBlockCondition struct {
	Reason       string
	Message      string
	RequeueAfter time.Duration
}

// MockBlockingRoutingEngine returns a configured blocking condition
type MockBlockingRoutingEngine struct {
	BlockCondition *MockBlockCondition
}

func (m *MockBlockingRoutingEngine) CheckBlockingConditions(ctx context.Context, rr *remediationv1.RemediationRequest, workflowID string) (*routing.BlockingCondition, error) {
	if m.BlockCondition != nil {
		return &routing.BlockingCondition{
			Blocked:      true,
			Reason:       m.BlockCondition.Reason,
			Message:      m.BlockCondition.Message,
			RequeueAfter: m.BlockCondition.RequeueAfter,
		}, nil
	}
	return nil, nil
}

func (m *MockBlockingRoutingEngine) CheckUnmanagedResource(ctx context.Context, rr *remediationv1.RemediationRequest) *routing.BlockingCondition {
	return nil
}

func (m *MockBlockingRoutingEngine) Config() routing.Config {
	return routing.Config{
		ConsecutiveFailureThreshold: 3,
		ConsecutiveFailureCooldown:  3600,
		RecentlyRemediatedCooldown:  300,
	}
}

func (m *MockBlockingRoutingEngine) CalculateExponentialBackoff(consecutiveFailures int32) time.Duration {
	return time.Duration(consecutiveFailures) * time.Minute
}
