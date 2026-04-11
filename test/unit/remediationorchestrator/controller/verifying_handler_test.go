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

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

func noopCallbacks() prodcontroller.VerifyingCallbacks {
	return prodcontroller.VerifyingCallbacks{
		EnsureNotificationsCreated:            func(_ context.Context, _ *remediationv1.RemediationRequest) {},
		CreateEffectivenessAssessmentIfNeeded: func(_ context.Context, _ *remediationv1.RemediationRequest) {},
		TrackEffectivenessStatus:              func(_ context.Context, _ *remediationv1.RemediationRequest) error { return nil },
		EmitVerificationTimedOutAudit:         func(_ context.Context, _ *remediationv1.RemediationRequest) {},
		EmitVerificationCompletedAudit:        func(_ context.Context, _ *remediationv1.RemediationRequest) {},
		EmitCompletionAudit:                   func(_ context.Context, _ *remediationv1.RemediationRequest, _ string, _ int64) {},
	}
}

var _ = Describe("Issue #666: VerifyingHandler (BR-EM-010)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
	})

	newHandler := func(c client.Client, cbs prodcontroller.VerifyingCallbacks) *prodcontroller.VerifyingHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewVerifyingHandler(
			c, m, prodcontroller.TimeoutConfig{Verifying: 10 * time.Minute}, cbs,
		)
	}

	// helper: create a Verifying-phase RR with EA ref and active deadline
	verifyingRRWithDeadline := func(name string, deadline time.Time) *remediationv1.RemediationRequest {
		rr := newRemediationRequest(name, "default", remediationv1.PhaseVerifying)
		rr.Status.Outcome = "Remediated"
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind: "EffectivenessAssessment", Name: "ea-" + name, Namespace: "default",
		}
		dl := metav1.NewTime(deadline)
		rr.Status.VerificationDeadline = &dl
		return rr
	}

	// helper: create a minimal EA
	minimalEA := func(name string, eaPhase string) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status:     eav1.EffectivenessAssessmentStatus{Phase: eaPhase},
		}
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-VER-H-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.VerifyingHandler{}
		})

		It("UT-VER-H-002: Phase() returns Verifying", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := newHandler(c, noopCallbacks())
			Expect(h.Phase()).To(Equal(phase.Verifying))
		})
	})

	// ========================================
	// Step 0: Notification retry (BR-ORCH-045)
	// ========================================
	Describe("Notification retry (BR-ORCH-045)", func() {
		It("UT-VER-H-003: calls EnsureNotificationsCreated when Outcome is set", func() {
			rr := verifyingRRWithDeadline("ver-notif", time.Now().Add(10*time.Minute))
			ea := minimalEA("ea-ver-notif", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			notifCalled := false
			cbs := noopCallbacks()
			cbs.EnsureNotificationsCreated = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				notifCalled = true
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(notifCalled).To(BeTrue(), "EnsureNotificationsCreated should be called when Outcome is set")
		})

		It("UT-VER-H-004: does NOT call EnsureNotificationsCreated when Outcome is empty", func() {
			rr := verifyingRRWithDeadline("ver-no-notif", time.Now().Add(10*time.Minute))
			rr.Status.Outcome = "" // clear outcome
			ea := minimalEA("ea-ver-no-notif", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			notifCalled := false
			cbs := noopCallbacks()
			cbs.EnsureNotificationsCreated = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				notifCalled = true
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(notifCalled).To(BeFalse(), "EnsureNotificationsCreated should NOT be called when Outcome is empty")
		})
	})

	// ========================================
	// Step 1: EA creation retry
	// ========================================
	Describe("EA creation retry", func() {
		It("UT-VER-H-005: EA ref nil, creation callback still leaves nil → requeue at RequeueResourceBusy", func() {
			rr := newRemediationRequest("ver-ea-nil", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			eaCreateCalled := false
			cbs := noopCallbacks()
			cbs.CreateEffectivenessAssessmentIfNeeded = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				eaCreateCalled = true
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(eaCreateCalled).To(BeTrue(), "should attempt EA creation")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})

		It("UT-VER-H-006: EA ref nil, creation callback sets ref → proceeds to step 2", func() {
			rr := newRemediationRequest("ver-ea-created", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"

			ea := minimalEA("ea-ver-ea-created", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			cbs := noopCallbacks()
			cbs.CreateEffectivenessAssessmentIfNeeded = func(_ context.Context, rr *remediationv1.RemediationRequest) {
				rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
					Kind: "EffectivenessAssessment", Name: "ea-ver-ea-created", Namespace: "default",
				}
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			// Proceeds to step 2; ValidityDeadline nil, age < timeout → requeue
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})
	})

	// ========================================
	// Step 2: VerificationDeadline population
	// ========================================
	Describe("VerificationDeadline population", func() {
		It("UT-VER-H-007: EA Get error returns requeue at RequeueResourceBusy", func() {
			rr := newRemediationRequest("ver-ea-err", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-missing", Namespace: "default",
			}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			h := newHandler(c, noopCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})

		It("UT-VER-H-008: ValidityDeadline set → VerificationDeadline populated with buffer", func() {
			rr := newRemediationRequest("ver-dl-set", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-ver-dl-set", Namespace: "default",
			}

			validityDeadline := metav1.NewTime(time.Now().Add(5 * time.Minute))
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-ver-dl-set", Namespace: "default"},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhasePending,
					ValidityDeadline: &validityDeadline,
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			h := newHandler(c, noopCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			// After setting deadline, continues to step 3/4 → EA still in progress → requeue
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
			Expect(rr.Status.VerificationDeadline).ToNot(BeNil(),
				"VerificationDeadline should be populated from EA.ValidityDeadline + buffer")
		})

		It("UT-VER-H-009: ValidityDeadline nil, age < timeout → requeue", func() {
			rr := newRemediationRequest("ver-no-dl", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-ver-no-dl", Namespace: "default",
			}

			ea := minimalEA("ea-ver-no-dl", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			h := newHandler(c, noopCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})

		It("UT-VER-H-010: safety-net timeout → Completed/VerificationTimedOut + metrics + audit", func() {
			rr := newRemediationRequest("ver-safety", "default", remediationv1.PhaseVerifying)
			rr.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-15 * time.Minute))
			rr.Status.Outcome = "Remediated"
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-ver-safety", Namespace: "default",
			}

			ea := minimalEA("ea-ver-safety", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			timedOutAuditCalled := false
			notifCalled := false
			cbs := noopCallbacks()
			cbs.EmitVerificationTimedOutAudit = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				timedOutAuditCalled = true
			}
			cbs.EnsureNotificationsCreated = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				notifCalled = true
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone), "status update performed in-handler")
			Expect(timedOutAuditCalled).To(BeTrue(), "should emit timeout audit")
			Expect(notifCalled).To(BeTrue(), "should create notifications after Outcome is set")
		})
	})

	// ========================================
	// Step 3: Deadline expiry
	// ========================================
	Describe("Deadline expiry", func() {
		It("UT-VER-H-011: expired VerificationDeadline → Completed/VerificationTimedOut + audit", func() {
			rr := verifyingRRWithDeadline("ver-expired", time.Now().Add(-1*time.Minute))
			ea := minimalEA("ea-ver-expired", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			timedOutAuditCalled := false
			notifCalled := false
			cbs := noopCallbacks()
			cbs.EmitVerificationTimedOutAudit = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				timedOutAuditCalled = true
			}
			cbs.EnsureNotificationsCreated = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				notifCalled = true
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone), "status update already applied in-handler")
			Expect(timedOutAuditCalled).To(BeTrue())
			Expect(notifCalled).To(BeTrue())
		})
	})

	// ========================================
	// Step 4: EA terminal → Completed
	// ========================================
	Describe("EA terminal status", func() {
		It("UT-VER-H-012: trackEffectivenessStatus mutates phase to Completed → NoOp + completed audit", func() {
			rr := verifyingRRWithDeadline("ver-ea-done", time.Now().Add(10*time.Minute))
			ea := minimalEA("ea-ver-ea-done", eav1.PhaseCompleted)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			completedAuditCalled := false
			notifCalled := false
			cbs := noopCallbacks()
			cbs.TrackEffectivenessStatus = func(_ context.Context, rr *remediationv1.RemediationRequest) error {
				// Simulate what trackEffectivenessStatus does: mutate OverallPhase
				rr.Status.OverallPhase = phase.Completed
				rr.Status.Outcome = "Remediated"
				return nil
			}
			cbs.EmitVerificationCompletedAudit = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				completedAuditCalled = true
			}
			cbs.EnsureNotificationsCreated = func(_ context.Context, _ *remediationv1.RemediationRequest) {
				notifCalled = true
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone), "already transitioned by trackEffectivenessStatus")
			Expect(completedAuditCalled).To(BeTrue(), "should emit verification completed audit")
			Expect(notifCalled).To(BeTrue(), "should create notifications after phase mutated to Completed")
		})

		It("UT-VER-H-013: trackEffectivenessStatus error is non-fatal → requeue", func() {
			rr := verifyingRRWithDeadline("ver-track-err", time.Now().Add(10*time.Minute))
			ea := minimalEA("ea-ver-track-err", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			cbs := noopCallbacks()
			cbs.TrackEffectivenessStatus = func(_ context.Context, _ *remediationv1.RemediationRequest) error {
				return fmt.Errorf("simulated tracking error")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred(), "tracking error is non-fatal")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})
	})

	// ========================================
	// Step 5: EA in progress → requeue
	// ========================================
	Describe("EA in progress", func() {
		It("UT-VER-H-014: non-terminal EA with active deadline → requeue at RequeueResourceBusy", func() {
			rr := verifyingRRWithDeadline("ver-in-prog", time.Now().Add(10*time.Minute))
			ea := minimalEA("ea-ver-in-prog", eav1.PhasePending)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			h := newHandler(c, noopCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})
	})
})
