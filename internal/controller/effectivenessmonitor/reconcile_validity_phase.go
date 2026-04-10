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

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// handleValidityPhases handles Steps 3b-5: WFP, expired, and stabilizing phases.
// Returns (result, true, err) when reconciliation should terminate early.
func (r *Reconciler) handleValidityPhases(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	// Step 3b: WaitingForPropagation
	if result, done, err := r.handleWaitingForPropagation(ctx, rctx); done {
		return result, true, err
	}

	// Step 4: Expired
	if result, done, err := r.handleExpired(ctx, rctx); done {
		return result, true, err
	}

	// Step 5: Stabilizing
	if result, done, err := r.handleStabilizing(ctx, rctx); done {
		return result, true, err
	}

	return ctrl.Result{}, false, nil
}

// handleWaitingForPropagation handles Step 3b: WFP phase for async targets.
// For async targets where the hash deferral deadline is still in the future,
// enter/stay in WaitingForPropagation. Requeue until deadline elapses.
func (r *Reconciler) handleWaitingForPropagation(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)
	ea := rctx.ea

	hashDeadline := rctx.stabilizationAnchor
	if !(rctx.isAsync && time.Now().Before(hashDeadline.Time)) {
		return ctrl.Result{}, false, nil
	}

	if rctx.currentPhase == eav1.PhasePending || rctx.currentPhase == "" {
		extended := r.computeAndSetDerivedTiming(ctx, ea)
		if extended {
			r.emitValidityExtendedEvent(ea)
		}

		ea.Status.Phase = eav1.PhaseWaitingForPropagation

		logger.Info("Async target: entered WaitingForPropagation (BR-EM-010.3)",
			"hashComputeDelay", ea.Spec.Config.HashComputeDelay.Duration,
			"validityDeadline", ea.Status.ValidityDeadline,
			"checkAfter", ea.Status.PrometheusCheckAfter,
			"alertCheckAfter", ea.Status.AlertManagerCheckAfter,
		)

		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to persist WaitingForPropagation phase")
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, true, err
		}

		r.emitScheduledEventIfFirst(ctx, ea)
	}

	remaining := time.Until(hashDeadline.Time)
	logger.Info("Waiting for async propagation to complete", "remaining", remaining)
	return ctrl.Result{RequeueAfter: remaining}, true, nil
}

// handleExpired handles Step 4: validity window expiration.
func (r *Reconciler) handleExpired(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	if rctx.windowState != validity.WindowExpired {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)
	ea := rctx.ea

	logger.Info("Validity window expired, completing with available data")
	r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonAssessmentExpired,
		fmt.Sprintf("Validity window expired for correlation %s; completing with available data",
			ea.Spec.CorrelationID))
	r.Metrics.RecordValidityExpiration()

	result, err := r.completeAssessment(ctx, ea)
	return result, true, err
}

// handleStabilizing handles Step 5: stabilization window.
// Persists derived timing if not yet set, transitions WFP→Stabilizing, and requeues.
func (r *Reconciler) handleStabilizing(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	if rctx.windowState != validity.WindowStabilizing {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)
	ea := rctx.ea

	remaining := r.validityChecker.TimeUntilStabilized(
		rctx.stabilizationAnchor,
		ea.Spec.Config.StabilizationWindow.Duration,
	)

	// Issue #253: WaitingForPropagation → Stabilizing transition.
	if rctx.currentPhase == eav1.PhaseWaitingForPropagation {
		ea.Status.Phase = eav1.PhaseStabilizing
		logger.Info("Async target: WaitingForPropagation → Stabilizing (HCA elapsed)",
			"remaining", remaining)
		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to persist WaitingForPropagation → Stabilizing transition")
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, true, err
		}
	}

	// BR-EM-009: Persist derived timing during stabilization.
	if ea.Status.ValidityDeadline == nil && (rctx.currentPhase == eav1.PhasePending || rctx.currentPhase == "") {
		extended := r.computeAndSetDerivedTiming(ctx, ea)
		if extended {
			r.emitValidityExtendedEvent(ea)
		}

		ea.Status.Phase = eav1.PhaseStabilizing

		logger.Info("Transitioned to Stabilizing, persisted derived timing (BR-EM-009)",
			"creationTimestamp", ea.CreationTimestamp,
			"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
			"validityDeadline", ea.Status.ValidityDeadline,
		)

		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to persist Stabilizing phase and derived timing")
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, true, err
		}

		r.emitScheduledEventIfFirst(ctx, ea)
	}

	logger.Info("Stabilization window active, requeueing", "remaining", remaining)
	return ctrl.Result{RequeueAfter: remaining}, true, nil
}

// handleAssessingTransition handles Step 6: Pending/Stabilizing → Assessing.
// Sets phase and derived timing in-memory (deferred to Step 9 atomic update).
func (r *Reconciler) handleAssessingTransition(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	ea := rctx.ea
	cp := rctx.currentPhase

	if cp != eav1.PhasePending && cp != eav1.PhaseWaitingForPropagation && cp != eav1.PhaseStabilizing && cp != "" {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)

	if !phase.CanTransition(cp, eav1.PhaseAssessing) {
		logger.Error(nil, "Invalid phase transition", "from", cp, "to", eav1.PhaseAssessing)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, true,
			fmt.Errorf("invalid phase transition: %s -> %s", cp, eav1.PhaseAssessing)
	}

	extended := r.computeAndSetDerivedTiming(ctx, ea)
	if extended {
		r.emitValidityExtendedEvent(ea)
	}

	ea.Status.Phase = eav1.PhaseAssessing

	logger.Info("Computed derived timing (BR-EM-009)",
		"creationTimestamp", ea.CreationTimestamp,
		"configuredValidityWindow", r.Config.ValidityWindow,
		"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
		"validityDeadline", ea.Status.ValidityDeadline,
		"prometheusCheckAfter", ea.Status.PrometheusCheckAfter,
		"alertManagerCheckAfter", ea.Status.AlertManagerCheckAfter,
	)

	rctx.pendingTransition = true
	return ctrl.Result{}, false, nil
}
