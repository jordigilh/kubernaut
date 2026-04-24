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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

func noopBlockedCallbacks() prodcontroller.BlockedCallbacks {
	return prodcontroller.BlockedCallbacks{
		RecheckResourceBusyBlock:      func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) { return ctrl.Result{}, nil },
		RecheckDuplicateBlock:         func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) { return ctrl.Result{}, nil },
		HandleUnmanagedResourceExpiry: func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) { return ctrl.Result{}, nil },
		TransitionToFailedTerminal:    func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) { return ctrl.Result{}, nil },
	}
}

var _ = Describe("Issue #666: BlockedHandler (BR-ORCH-042)", func() {

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	newHandler := func(cbs prodcontroller.BlockedCallbacks) *prodcontroller.BlockedHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewBlockedHandler(m, cbs)
	}

	blockedRR := func(name string, reason remediationv1.BlockReason, blockedUntil *time.Time) *remediationv1.RemediationRequest {
		rr := newRemediationRequest(name, "default", remediationv1.PhaseBlocked)
		rr.Status.BlockReason = reason
		if blockedUntil != nil {
			t := metav1.NewTime(*blockedUntil)
			rr.Status.BlockedUntil = &t
		}
		return rr
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-BLK-H-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.BlockedHandler{}
		})

		It("UT-BLK-H-002: Phase() returns Blocked", func() {
			h := newHandler(noopBlockedCallbacks())
			Expect(h.Phase()).To(Equal(phase.Blocked))
		})
	})

	// ========================================
	// BlockedUntil nil paths
	// ========================================
	Describe("BlockedUntil nil (event-based blocks)", func() {
		It("UT-BLK-H-003: ResourceBusy → delegates to recheckResourceBusyBlock callback", func() {
			rr := blockedRR("blk-busy", remediationv1.BlockReasonResourceBusy, nil)
			rr.Status.BlockingWorkflowExecution = "wfe-busy"

			recheckCalled := false
			cbs := noopBlockedCallbacks()
			cbs.RecheckResourceBusyBlock = func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) {
				recheckCalled = true
				return ctrl.Result{Requeue: true}, nil
			}

			h := newHandler(cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(recheckCalled).To(BeTrue(), "should delegate to recheckResourceBusyBlock")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueImmediately).To(BeTrue())
		})

		It("UT-BLK-H-004: DuplicateInProgress → delegates to recheckDuplicateBlock callback", func() {
			rr := blockedRR("blk-dup", remediationv1.BlockReasonDuplicateInProgress, nil)
			rr.Status.DuplicateOf = "original-rr"

			recheckCalled := false
			cbs := noopBlockedCallbacks()
			cbs.RecheckDuplicateBlock = func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) {
				recheckCalled = true
				return ctrl.Result{Requeue: true}, nil
			}

			h := newHandler(cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(recheckCalled).To(BeTrue(), "should delegate to recheckDuplicateBlock")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueImmediately).To(BeTrue())
		})

		It("UT-BLK-H-005: manual/unknown block reason → NoOp (no auto-expiry)", func() {
			rr := blockedRR("blk-manual", "ManualHold", nil)

			h := newHandler(noopBlockedCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(BeZero(), "manual blocks have no auto-expiry")
			Expect(intent.RequeueImmediately).To(BeFalse())
		})
	})

	// ========================================
	// BlockedUntil expired
	// ========================================
	Describe("BlockedUntil expired", func() {
		It("UT-BLK-H-006: UnmanagedResource → delegates to handleUnmanagedResourceExpiry (BR-SCOPE-010)", func() {
			past := time.Now().Add(-1 * time.Minute)
			rr := blockedRR("blk-unmanaged", remediationv1.BlockReasonUnmanagedResource, &past)

			unmanagedCalled := false
			cbs := noopBlockedCallbacks()
			cbs.HandleUnmanagedResourceExpiry = func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) {
				unmanagedCalled = true
				return ctrl.Result{Requeue: true}, nil
			}

			h := newHandler(cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmanagedCalled).To(BeTrue(), "should delegate to handleUnmanagedResourceExpiry")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
		})

		It("UT-BLK-H-007: other reason → Failed terminal + metrics gauge decrement (BR-ORCH-042)", func() {
			past := time.Now().Add(-1 * time.Minute)
			rr := blockedRR("blk-expired", remediationv1.BlockReasonConsecutiveFailures, &past)

			failedCalled := false
			var capturedFailurePhase remediationv1.FailurePhase
			cbs := noopBlockedCallbacks()
			cbs.TransitionToFailedTerminal = func(_ context.Context, _ *remediationv1.RemediationRequest, fp remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				capturedFailurePhase = fp
				return ctrl.Result{}, nil
			}

			h := newHandler(cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedCalled).To(BeTrue(), "should call transitionToFailedTerminal")
			Expect(capturedFailurePhase).To(Equal(remediationv1.FailurePhaseBlocked))
			Expect(intent.Type).To(Equal(phase.TransitionNone), "transition done in callback")
		})
	})

	// ========================================
	// BlockedUntil in future
	// ========================================
	Describe("BlockedUntil in future", func() {
		It("UT-BLK-H-008: still in cooldown → RequeueAfter at exact expiry", func() {
			future := time.Now().Add(5 * time.Minute)
			rr := blockedRR("blk-future", remediationv1.BlockReasonConsecutiveFailures, &future)

			h := newHandler(noopBlockedCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(BeNumerically(">", 4*time.Minute))
			Expect(intent.RequeueAfter).To(BeNumerically("<=", 5*time.Minute))
		})
	})

	// ========================================
	// CHECKPOINT 3b audit: error propagation
	// ========================================
	Describe("Error propagation (CHECKPOINT 3b audit)", func() {
		It("UT-BLK-H-009: recheckResourceBusyBlock error propagates", func() {
			rr := blockedRR("blk-busy-err", remediationv1.BlockReasonResourceBusy, nil)

			cbs := noopBlockedCallbacks()
			cbs.RecheckResourceBusyBlock = func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) {
				return ctrl.Result{}, fmt.Errorf("apiserver unavailable")
			}

			h := newHandler(cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiserver unavailable"))
		})

		It("UT-BLK-H-010: recheckDuplicateBlock error propagates", func() {
			rr := blockedRR("blk-dup-err", remediationv1.BlockReasonDuplicateInProgress, nil)

			cbs := noopBlockedCallbacks()
			cbs.RecheckDuplicateBlock = func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) {
				return ctrl.Result{}, fmt.Errorf("get original failed")
			}

			h := newHandler(cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get original failed"))
		})

		It("UT-BLK-H-011: transitionToFailedTerminal error propagates on expired cooldown", func() {
			past := time.Now().Add(-1 * time.Minute)
			rr := blockedRR("blk-fail-err", remediationv1.BlockReasonConsecutiveFailures, &past)

			cbs := noopBlockedCallbacks()
			cbs.TransitionToFailedTerminal = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				return ctrl.Result{}, fmt.Errorf("status update conflict")
			}

			h := newHandler(cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status update conflict"))
		})
	})

	// ========================================
	// CHECKPOINT 3b audit: RequeueAfter from callbacks (resultToIntent)
	// ========================================
	Describe("resultToIntent RequeueAfter conversion (CHECKPOINT 3b audit)", func() {
		It("UT-BLK-H-012: callback returning RequeueAfter maps to intent.RequeueAfter", func() {
			rr := blockedRR("blk-requeue", remediationv1.BlockReasonResourceBusy, nil)

			cbs := noopBlockedCallbacks()
			cbs.RecheckResourceBusyBlock = func(_ context.Context, _ *remediationv1.RemediationRequest) (ctrl.Result, error) {
				return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
			}

			h := newHandler(cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
			Expect(intent.RequeueImmediately).To(BeFalse())
		})
	})
})
