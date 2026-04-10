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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
	emtiming "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/timing"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
)

// reconcileContext holds shared mutable state for a single reconciliation pass.
// Prevents local variable drift across the extracted orchestration methods.
type reconcileContext struct {
	ea                  *eav1.EffectivenessAssessment
	currentPhase        string
	stabilizationAnchor metav1.Time
	isAsync             bool
	windowState         validity.WindowState
	pendingTransition   bool
	componentsChanged   bool
	alertDeferred       alert.AlertDeferralResult
	scope               assessmentScope
}

// reconcileActive is the main orchestration method for non-terminal EAs.
// Called after fetch, validation, and terminal checks in Reconcile.
func (r *Reconciler) reconcileActive(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error) {
	ea := rctx.ea

	// Compute stabilization anchor and window state.
	rctx.stabilizationAnchor = ea.CreationTimestamp
	rctx.isAsync = ea.Spec.Config.HashComputeDelay != nil && ea.Spec.Config.HashComputeDelay.Duration > 0
	if rctx.isAsync {
		rctx.stabilizationAnchor = metav1.NewTime(ea.CreationTimestamp.Add(ea.Spec.Config.HashComputeDelay.Duration))
	}

	if ea.Status.ValidityDeadline != nil {
		rctx.windowState = r.validityChecker.Check(
			rctx.stabilizationAnchor,
			ea.Spec.Config.StabilizationWindow.Duration,
			*ea.Status.ValidityDeadline,
		)
	} else {
		stabilizationEnd := rctx.stabilizationAnchor.Add(ea.Spec.Config.StabilizationWindow.Duration)
		if time.Now().Before(stabilizationEnd) {
			rctx.windowState = validity.WindowStabilizing
		} else {
			rctx.windowState = validity.WindowActive
		}
	}

	// Steps 3b-5: Handle WFP, expired, and stabilizing phases.
	if result, done, err := r.handleValidityPhases(ctx, rctx); done {
		return result, err
	}

	// Step 6: Transition to Assessing phase.
	if result, done, err := r.handleAssessingTransition(ctx, rctx); done {
		return result, err
	}

	// Step 6.5: Spec drift guard.
	if result, done, err := r.handleSpecDrift(ctx, rctx); done {
		return result, err
	}

	// Steps 6b-7b: Component pipeline.
	if result, done, err := r.runComponentPipeline(ctx, rctx); done {
		return result, err
	}

	// Steps 8-10: Completion check, atomic status update, requeue.
	return r.finalizeReconcile(ctx, rctx)
}

// capRequeueAtDeadline ensures the requeue interval does not overshoot the
// ValidityDeadline. Returns the original interval if no deadline is set or
// the deadline is further away (BR-EM-007, Issue #591).
func (r *Reconciler) capRequeueAtDeadline(ea *eav1.EffectivenessAssessment, interval time.Duration) time.Duration {
	if ea.Status.ValidityDeadline != nil {
		remaining := r.validityChecker.TimeUntilExpired(ea.Status.ValidityDeadline.Time)
		if remaining > 0 && remaining < interval {
			return remaining
		}
	}
	return interval
}

// computeAndSetDerivedTiming computes derived timing fields and applies them to
// the EA status. Returns true if the ValidityWindow was extended by the runtime guard.
func (r *Reconciler) computeAndSetDerivedTiming(ctx context.Context, ea *eav1.EffectivenessAssessment) bool {
	logger := log.FromContext(ctx)
	dt := emtiming.ComputeDerivedTiming(ea.CreationTimestamp, ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, ea.Spec.Config.HashComputeDelay, ea.Spec.Config.AlertCheckDelay)
	if dt.Extended {
		logger.Info("Runtime guard: extended ValidityDeadline (StabilizationWindow >= ValidityWindow)",
			"originalValidity", r.Config.ValidityWindow,
			"effectiveValidity", dt.EffectiveValidity,
			"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
		)
	}
	deadline := dt.ValidityDeadline
	checkAfter := dt.CheckAfter
	alertCheckAfter := dt.AlertCheckAfter

	ea.Status.ValidityDeadline = &deadline
	ea.Status.PrometheusCheckAfter = &checkAfter
	ea.Status.AlertManagerCheckAfter = &alertCheckAfter

	return dt.Extended
}
