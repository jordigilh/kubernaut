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

	ctrl "sigs.k8s.io/controller-runtime"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

// ApplyTransition is the centralized bridge between PhaseHandler.Handle() output
// and the reconciler's existing transition methods. It translates a TransitionIntent
// into the appropriate status mutation + ctrl.Result.
//
// Mapping:
//
//	TransitionAdvance            → transitionPhase(ctx, rr, intent.TargetPhase)
//	TransitionFailed             → transitionToFailed(ctx, rr, intent.FailurePhase, intent.FailureErr)
//	TransitionBlocked            → handleBlocked(ctx, rr, bc, fromPhase, workflowID)
//	TransitionVerifying          → transitionToVerifying(ctx, rr, intent.Outcome)
//	TransitionInheritedCompleted → transitionToInheritedCompleted(ctx, rr, sourceRef, sourceKind)
//	TransitionInheritedFailed    → transitionToInheritedFailed(ctx, rr, failureErr, sourceRef, sourceKind)
//	TransitionNone               → ctrl.Result based on RequeueImmediately / RequeueAfter / neither
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
func (r *Reconciler) ApplyTransition(ctx context.Context, rr *remediationv1.RemediationRequest, intent phase.TransitionIntent) (ctrl.Result, error) {
	if err := intent.Validate(); err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid transition intent: %w", err)
	}

	switch intent.Type {
	case phase.TransitionAdvance:
		res, err := r.transitionPhase(ctx, rr, intent.TargetPhase)
		if err != nil {
			return res, fmt.Errorf("applyTransition(%s→%s): %w", intent.Type, intent.TargetPhase, err)
		}
		return res, nil

	case phase.TransitionFailed:
		res, err := r.transitionToFailed(ctx, rr, intent.FailurePhase, intent.FailureErr)
		return wrapTransitionResult(intent.Type, res, err)

	case phase.TransitionBlocked:
		bc := ToBlockingCondition(intent.Block)
		res, err := r.handleBlocked(ctx, rr, bc, string(intent.Block.FromPhase), intent.Block.WorkflowID)
		return wrapTransitionResult(intent.Type, res, err)

	case phase.TransitionVerifying:
		res, err := r.transitionToVerifying(ctx, rr, intent.Outcome)
		return wrapTransitionResult(intent.Type, res, err)

	case phase.TransitionInheritedCompleted:
		res, err := r.transitionToInheritedCompleted(ctx, rr, intent.SourceRef, intent.SourceKind)
		return wrapTransitionResult(intent.Type, res, err)

	case phase.TransitionInheritedFailed:
		res, err := r.transitionToInheritedFailed(ctx, rr, intent.FailureErr, intent.SourceRef, intent.SourceKind)
		return wrapTransitionResult(intent.Type, res, err)

	case phase.TransitionCompletedWithoutVerification:
		res, err := r.transitionToCompletedWithoutVerification(ctx, rr, intent.Reason)
		return wrapTransitionResult(intent.Type, res, err)

	case phase.TransitionNone:
		return transitionNoneResult(intent), nil

	default:
		return ctrl.Result{}, fmt.Errorf("unhandled transition type: %s", intent.Type)
	}
}

// wrapTransitionResult wraps a non-nil err with the "applyTransition(<type>)"
// context prefix shared by every ApplyTransition case except TransitionAdvance
// (which also includes the target phase). Extracted from ApplyTransition
// (Wave 6 6e-i GREEN: cyclomatic-complexity remediation) — pure code motion,
// no behavior change; moves each case's "if err != nil" branch out of
// ApplyTransition's own cyclomatic count.
func wrapTransitionResult(transitionType phase.TransitionType, res ctrl.Result, err error) (ctrl.Result, error) {
	if err != nil {
		return res, fmt.Errorf("applyTransition(%s): %w", transitionType, err)
	}
	return res, nil
}

// transitionNoneResult implements the TransitionNone case of ApplyTransition:
// requeue immediately, requeue after a delay, or do neither. Extracted from
// ApplyTransition (Wave 6 6e-i GREEN: cyclomatic-complexity remediation) —
// pure code motion, no behavior change.
func transitionNoneResult(intent phase.TransitionIntent) ctrl.Result {
	if intent.RequeueImmediately {
		return ctrl.Result{Requeue: true}
	}
	if intent.RequeueAfter > 0 {
		return ctrl.Result{RequeueAfter: intent.RequeueAfter}
	}
	return ctrl.Result{}
}

// ToBlockingCondition converts a phase.BlockMeta to a routing.BlockingCondition
// for dispatch to handleBlocked. Exported for testing.
// Panics are prevented: returns nil if bm is nil.
func ToBlockingCondition(bm *phase.BlockMeta) *routing.BlockingCondition {
	if bm == nil {
		return nil
	}
	return &routing.BlockingCondition{
		Blocked:                   true,
		Reason:                    bm.Reason,
		Message:                   bm.Message,
		RequeueAfter:              bm.RequeueAfter,
		BlockedUntil:              bm.BlockedUntil,
		BlockingWorkflowExecution: bm.BlockingWorkflowExecution,
		DuplicateOf:               bm.DuplicateOf,
	}
}
