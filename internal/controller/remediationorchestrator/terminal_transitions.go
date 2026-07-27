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
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// errPhaseConflict is returned when a concurrent reconcile changed the phase
// between read and write. Non-retryable: RetryOnConflict only retries k8s
// Conflict errors, so this propagates immediately to the caller for graceful handling.
var errPhaseConflict = errors.New("phase changed by concurrent reconcile")

// transitionToInheritedCompleted transitions the RR to Completed with outcome "Remediated".
// Used when an original resource (WFE or RR) that caused deduplication completes successfully.
// The outcome is "Remediated" (not a separate "InheritedCompleted") because the CRD enum
// only allows Remediated|NoActionRequired|ManualReviewRequired|VerificationTimedOut, and
// the dedup lineage is already preserved via DeduplicatedByWE/DuplicateOf fields + K8s events.
// sourceRef identifies the original resource name; sourceKind is "WorkflowExecution" or "RemediationRequest".
//
//nolint:unparam // ctrl.Result is always the zero value here, but the signature is required by ApplyTransition's uniform dispatch (apply_transition.go: wrapTransitionResult wraps all sibling transitionTo* results identically) (Issue #1546 Tier 4)
func (r *Reconciler) transitionToInheritedCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, sourceRef, sourceKind string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	oldPhase := rr.Status.OverallPhase
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		rr.Status.OverallPhase = phase.Completed
		rr.Status.Outcome = remediationv1.OutcomeRemediated
		rr.Status.CompletedAt = &now
		rr.Status.ObservedGeneration = rr.Generation
		if sourceKind == "RemediationRequest" {
			rr.Status.BlockReason = ""
			rr.Status.BlockMessage = ""
			rr.Status.DuplicateOf = ""
		}
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonReady,
			fmt.Sprintf("Inherited completion from original %s", sourceKind), r.Metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to inherited Completed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to inherited Completed: %w", err)
	}

	if oldPhase != phase.Completed {
		if r.Metrics != nil {
			r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(phase.Completed), rr.Namespace).Inc()
		}
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonInheritedCompleted,
				fmt.Sprintf("Remediation inherited Completed from original %s %s", sourceKind, sourceRef))
		}

		if rr.Status.StartTime != nil {
			r.emitCompletionAudit(ctx, rr, "InheritedCompleted", time.Since(rr.Status.StartTime.Time).Milliseconds())
		}

		// F-3: Only create notifications for WFE-level inheritance — DuplicateInProgress
		// RRs never reached AIAnalysis phase, so ensureNotificationsCreated would fail.
		if sourceKind == "WorkflowExecution" {
			r.ensureNotificationsCreated(ctx, rr)
		}
	}

	logger.Info("RR inherited Completed",
		"inheritedFrom", sourceRef, "sourceKind", sourceKind, "outcome", "InheritedCompleted")
	return ctrl.Result{}, nil
}

// transitionToCompletedWithoutVerification transitions the RR to Completed with outcome "DryRun".
// Used when dry-run mode is enabled: the pipeline stops after AI analysis without creating WFE or EA.
// Does NOT reset ConsecutiveFailureCount — dry-run did not prove remediation works.
// Sets NextAllowedExecution to suppress Gateway re-triggering (only if later than existing value).
// #712, #736: ADR-RO-001
//
//nolint:unparam // ctrl.Result is always the zero value here; signature required by ApplyTransition's uniform dispatch (see transitionToInheritedCompleted)
func (r *Reconciler) transitionToCompletedWithoutVerification(ctx context.Context, rr *remediationv1.RemediationRequest, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Idempotency guard: refetch from API server to avoid duplicate transitions
	freshRR := &remediationv1.RemediationRequest{}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), freshRR); err == nil {
		if freshRR.Status.OverallPhase == phase.Completed {
			logger.V(1).Info("RR already Completed (idempotent no-op)")
			return ctrl.Result{}, nil
		}
	}

	oldPhase := rr.Status.OverallPhase
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		rr.Status.OverallPhase = phase.Completed
		rr.Status.Outcome = "DryRun"
		rr.Status.CompletedAt = &now
		rr.Status.ObservedGeneration = rr.Generation

		// Set NextAllowedExecution for GW suppression, only if later than existing value
		dryRunNAE := metav1.NewTime(now.Add(r.getDryRunHoldPeriod()))
		if rr.Status.NextAllowedExecution == nil || dryRunNAE.After(rr.Status.NextAllowedExecution.Time) {
			rr.Status.NextAllowedExecution = &dryRunNAE
		}

		remediationrequest.SetReady(rr, true, "DryRun",
			fmt.Sprintf("Dry-run mode: pipeline stopped after AI analysis (%s)", reason), r.Metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to CompletedWithoutVerification")
		return ctrl.Result{}, fmt.Errorf("failed to transition to CompletedWithoutVerification: %w", err)
	}

	if oldPhase != phase.Completed {
		if r.Metrics != nil {
			r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(phase.Completed), rr.Namespace).Inc()
			r.Metrics.NoActionNeededTotal.WithLabelValues("DryRun", rr.Namespace).Inc()
		}
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, "DryRunCompleted",
				fmt.Sprintf("Dry-run mode: completed without execution or verification (%s)", reason))
		}
		if rr.Status.StartTime != nil {
			r.emitCompletionAudit(ctx, rr, "DryRun", time.Since(rr.Status.StartTime.Time).Milliseconds())
		}
	}

	logger.Info("RR completed without verification (dry-run)",
		"outcome", "DryRun", "reason", reason,
		"nextAllowedExecution", rr.Status.NextAllowedExecution)
	return ctrl.Result{}, nil
}

// transitionToInheritedFailed transitions the RR to Failed with FailurePhaseDeduplicated.
// Used when the original resource (WFE or RR) fails or is deleted (dangling reference).
// Does NOT increment ConsecutiveFailureCount — inherited failures are excluded from blocking.
// sourceRef identifies the original resource name; sourceKind is "WorkflowExecution" or "RemediationRequest".
//
//nolint:unparam // ctrl.Result is always the zero value here; signature required by ApplyTransition's uniform dispatch (see transitionToInheritedCompleted)
func (r *Reconciler) transitionToInheritedFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failureErr error, sourceRef, sourceKind string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	oldPhase := rr.Status.OverallPhase
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		failPhase := remediationv1.FailurePhaseDeduplicated
		rr.Status.OverallPhase = phase.Failed
		rr.Status.FailurePhase = &failPhase
		rr.Status.FailureReason = &failureReason
		rr.Status.CompletedAt = &now
		rr.Status.ObservedGeneration = rr.Generation
		if sourceKind == "RemediationRequest" {
			rr.Status.BlockReason = ""
			rr.Status.BlockMessage = ""
			rr.Status.DuplicateOf = ""
		}
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonNotReady,
			fmt.Sprintf("Inherited failure from original %s", sourceKind), r.Metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to inherited Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to inherited Failed: %w", err)
	}

	if oldPhase != phase.Failed {
		if r.Metrics != nil {
			r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(phase.Failed), rr.Namespace).Inc()
		}
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonInheritedFailed,
				fmt.Sprintf("Remediation inherited Failed from original %s %s: %s", sourceKind, sourceRef, failureReason))
		}

		durationMs := time.Since(rr.CreationTimestamp.Time).Milliseconds()
		r.emitFailureAudit(ctx, rr, remediationv1.FailurePhaseDeduplicated, failureErr, durationMs)
	}

	logger.Info("RR inherited Failed",
		"inheritedFrom", sourceRef, "sourceKind", sourceKind, "reason", failureReason)
	return ctrl.Result{}, nil
}

// transitionPhase transitions the RR to a new phase.
// REFACTOR-RO-001: Using retry helper for status updates (BR-ORCH-038).
func (r *Reconciler) transitionPhase(ctx context.Context, rr *remediationv1.RemediationRequest, newPhase phase.Phase) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "newPhase", newPhase)

	oldPhase := rr.Status.OverallPhase

	// ========================================
	// IDEMPOTENCY CHECK (Prevents Duplicate Audit Events)
	// Per RO_AUDIT_DUPLICATION_RISK_ANALYSIS_JAN_01_2026.md - Option C
	// ========================================
	// Without GenerationChangedPredicate, controller reconciles on annotation/label changes.
	// This check prevents duplicate audit emissions when phase hasn't actually changed.
	// ADR-032 §1: Audit integrity requires exactly-once emission per state change.
	if oldPhase == newPhase {
		logger.V(1).Info("Phase transition skipped - already in target phase",
			"currentPhase", oldPhase,
			"requestedPhase", newPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	if !phase.CanTransition(oldPhase, newPhase) {
		logger.Error(nil, "Invalid phase transition rejected by state machine",
			"currentPhase", oldPhase,
			"requestedPhase", newPhase)
		return ctrl.Result{}, fmt.Errorf("invalid phase transition from %s to %s", oldPhase, newPhase)
	}

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Phase conflict guard: the reconcile loop selects a phase handler based on
		// the informer cache (potentially stale). This updateFn runs after
		// UpdateRemediationRequestStatus refetches the actual etcd state. If the
		// phase has diverged, another reconcile already changed it — abort to
		// avoid overwriting a legitimate state change (e.g., Blocked → Processing).
		if rr.Status.OverallPhase != oldPhase {
			return fmt.Errorf("%w: expected %s, got %s", errPhaseConflict,
				oldPhase, rr.Status.OverallPhase)
		}
		r.applyPhaseTransitionFields(rr, newPhase)
		return nil
	})
	if err != nil {
		if errors.Is(err, errPhaseConflict) {
			logger.Info("Phase conflict detected — requeueing for fresh state",
				"expectedPhase", oldPhase,
				"actualPhase", rr.Status.OverallPhase,
				"targetPhase", newPhase)
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		logger.Error(err, "Failed to transition phase")
		return ctrl.Result{}, fmt.Errorf("failed to transition phase: %w", err)
	}

	// Record metric
	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(newPhase), rr.Namespace).Inc()
	}

	// Emit audit event (DD-AUDIT-003)
	r.emitPhaseTransitionAudit(ctx, rr, string(oldPhase), string(newPhase))

	// DD-EVENT-001: Emit K8s event for phase transition (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, newPhase))
	}

	logger.Info("Phase transition successful", "from", oldPhase, "to", newPhase)

	// Requeue with delay to check progress of child CRDs (different phases
	// have different check intervals).
	return ctrl.Result{RequeueAfter: requeueDelayForPhase(newPhase)}, nil
}

// applyPhaseTransitionFields updates the RO-owned status fields for a phase
// transition (preserving Gateway-owned fields per DD-GATEWAY-011): the new
// phase, observed generation (DD-CONTROLLER-001), the phase-specific start
// time, and the phase-specific Ready condition reason (Issue #636, so
// `kubectl get rr` REASON reflects the current pipeline stage). Extracted
// from transitionPhase (Wave 6 6e-i GREEN: cyclomatic-complexity
// remediation) — pure code motion, no behavior change.
func (r *Reconciler) applyPhaseTransitionFields(rr *remediationv1.RemediationRequest, newPhase phase.Phase) {
	rr.Status.OverallPhase = newPhase
	rr.Status.ObservedGeneration = rr.Generation
	now := metav1.Now()

	switch newPhase {
	case phase.Processing:
		rr.Status.ProcessingStartTime = &now
	case phase.Analyzing:
		rr.Status.AnalyzingStartTime = &now
	case phase.Executing:
		rr.Status.ExecutingStartTime = &now
	}

	switch newPhase {
	case phase.Processing:
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonProcessing, "Signal processing in progress", r.Metrics)
	case phase.Analyzing:
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonAnalyzing, "AI analysis in progress", r.Metrics)
	case phase.AwaitingApproval:
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonAwaitingApproval, "Waiting for human approval", r.Metrics)
	case phase.Executing:
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonExecuting, "Workflow execution in progress", r.Metrics)
	}
}

// requeueDelayForPhase returns transitionPhase's post-transition requeue
// delay: child-CRD-progress phases (Processing/Analyzing/Executing) get a
// 5s check interval, all other phases get a quick 100ms requeue. Extracted
// from transitionPhase (Wave 6 6e-i GREEN: cyclomatic-complexity
// remediation) — pure code motion, no behavior change.
func requeueDelayForPhase(newPhase phase.Phase) time.Duration {
	switch newPhase {
	case phase.Processing, phase.Analyzing, phase.Executing:
		return 5 * time.Second
	default:
		return 100 * time.Millisecond
	}
}

// transitionToVerifying transitions the RR to Verifying phase (#280).
// After WFE completes successfully, the RR enters Verifying (non-terminal) while the
// EffectivenessAssessment runs. The Gateway deduplicates signals during this window.
// RO transitions to Completed when EA reaches a terminal state (handleVerifyingPhase)
// or when VerificationDeadline expires.
// #281: Notification creation is delegated to ensureNotificationsCreated (idempotent).
// If creation fails here, handleVerifyingPhase retries on subsequent reconciles.
//
//nolint:unparam // ctrl.Result is always the zero value here; signature required by ApplyTransition's uniform dispatch (see transitionToInheritedCompleted)
func (r *Reconciler) transitionToVerifying(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	// RO-AUDIT-IDEMPOTENCY: Refetch via apiReader (cache-bypassed) before the phase
	// check. Pattern: mirrors transitionToFailed and RAR audit deduplication.
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
		logger.Error(err, "Failed to refetch RemediationRequest via apiReader for idempotency check")
		return ctrl.Result{}, err
	}

	// #280: Idempotency — skip if already in Verifying or Completed
	if rr.Status.OverallPhase == phase.Verifying || rr.Status.OverallPhase == phase.Completed {
		logger.V(1).Info("Already in Verifying/Completed phase (confirmed via apiReader), skipping transition",
			"phase", rr.Status.OverallPhase)
		return ctrl.Result{}, nil
	}

	oldPhaseBeforeTransition := rr.Status.OverallPhase

	// #280: Transition to Verifying (not Completed). CompletedAt and Outcome are set later
	// when EA finishes or VerificationDeadline expires.
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Verifying
		rr.Status.ObservedGeneration = rr.Generation

		// BR-ORCH-043: Set Ready condition (remediation succeeded, verification pending)
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonVerifying, "Remediation completed, verifying effectiveness", r.Metrics)

		// DD-WE-004 V1.0: Reset exponential backoff on success
		if rr.Status.NextAllowedExecution != nil {
			logger.Info("Clearing exponential backoff after successful remediation",
				"previousNextAllowed", rr.Status.NextAllowedExecution.Format(time.RFC3339),
				"previousConsecutiveFailures", rr.Status.ConsecutiveFailureCount)
			rr.Status.NextAllowedExecution = nil
		}
		rr.Status.ConsecutiveFailureCount = 0

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Verifying")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Verifying: %w", err)
	}

	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhaseBeforeTransition), string(phase.Verifying), rr.Namespace).Inc()
	}

	// Emit audit and K8s event for Verifying transition
	if oldPhaseBeforeTransition != phase.Verifying {
		r.emitVerifyingStartedAudit(ctx, rr)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonRemediationCompleted,
				"Remediation succeeded, entering verification phase (#280)")
		}
	}

	// #304: Notification creation deferred until after Outcome is set (completeVerificationIfNeeded).
	// ensureNotificationsCreated is called from handleVerifyingPhase after EA terminal transition
	// or timeout sets Outcome. Previously called here with empty Outcome (BR-ORCH-045 violation).

	// #280: Create EA — if this fails, handleVerifyingPhase will retry on next reconcile
	r.createEffectivenessAssessmentIfNeeded(ctx, rr)

	logger.Info("Remediation succeeded, entered Verifying phase", "outcome", outcome)
	return ctrl.Result{}, nil
}

// handleVerifyingPhase is now handled by VerifyingHandler via the phase registry.
// See verifying_handler.go (Issue #666, TP-666-v1 §8.1).

// VerificationDeadlineBuffer is the grace period added to EA.Status.ValidityDeadline
// when computing VerificationDeadline. Allows for clock skew and final status propagation.
const VerificationDeadlineBuffer = 30 * time.Second

// transitionToFailed transitions the RR to Failed phase.
// BR-ORCH-042: Before transitioning to terminal Failed, checks if this failure
// triggers consecutive failure blocking (≥3 consecutive failures for same fingerprint).
// If blocking is triggered, transitions to non-terminal Blocked phase instead.
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// F-6: Derive string reason from error for status fields and logging
	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	// BR-ORCH-042: Log consecutive failures for observability
	r.logConsecutiveFailureThresholdReached(ctx, rr, failurePhase, logger)

	// RO-AUDIT-IDEMPOTENCY: Refetch via apiReader (cache-bypassed) before the phase
	// check. The informer cache is eventually consistent — a second reconcile may
	// start with stale cache showing non-Failed phase after the first reconcile has
	// already transitioned, causing duplicate orchestrator.lifecycle.completed events.
	// Pattern: mirrors RAR audit deduplication (remediation_approval_request.go).
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
		logger.Error(err, "Failed to refetch RemediationRequest via apiReader for idempotency check")
		return ctrl.Result{}, err
	}

	if rr.Status.OverallPhase == phase.Failed {
		logger.V(1).Info("Already in Failed phase (confirmed via apiReader), skipping transition")
		return ctrl.Result{}, nil
	}

	// GAP-4 / #808: Create escalation NR for terminal failures (BR-ORCH-036).
	r.createEscalationNotificationIfNeeded(ctx, rr, failurePhase, failureReason, logger)

	// Capture old phase for metrics and audit
	oldPhaseBeforeTransition := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		r.applyFailedTransitionFields(rr, failurePhase, failureReason, logger)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Failed: %w", err)
	}

	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhaseBeforeTransition), string(phase.Failed), rr.Namespace).Inc()
	}

	// Emit audit event (DD-AUDIT-003)
	// IDEMPOTENCY: Only emit if phase actually changed (prevents duplicate events on reconcile retries)
	// Race condition protection: oldPhaseBeforeTransition captured before status update ensures
	// only the reconcile that successfully transitioned will emit the audit event
	if oldPhaseBeforeTransition != phase.Failed {
		durationMs := time.Since(startTime).Milliseconds()
		r.emitFailureAudit(ctx, rr, failurePhase, failureErr, durationMs)

		// DD-EVENT-001: Emit K8s event for failure (BR-ORCH-095)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationFailed,
				fmt.Sprintf("Remediation failed during %s: %s", failurePhase, failureReason))
		}
	}

	// Issue #240: EA is NOT created on failure paths. EA should only be created
	// when WFE completes successfully (transitionToVerifying), because failed/timed-out
	// remediations may have partially applied or no changes, making EA unreliable.

	logger.Info("Remediation failed", "failurePhase", failurePhase, "reason", failureReason)
	return ctrl.Result{}, nil
}

// applyFailedTransitionFields updates the RR status fields for a Failed
// transition: overall phase/observed generation/failure phase+reason/
// CompletedAt, the terminal-failure Ready condition (BR-ORCH-043), and
// DD-WE-004 V1.0's exponential backoff for pre-execution failures (only set
// below the routing engine's consecutive-failure threshold — at or above
// threshold, the routing engine's CheckConsecutiveFailures blocks with a
// fixed cooldown instead). Extracted from transitionToFailed (Wave 6 6e-i
// GREEN: funlen remediation) — pure code motion, no behavior change.
func (r *Reconciler) applyFailedTransitionFields(rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureReason string, logger logr.Logger) {
	rr.Status.OverallPhase = phase.Failed
	rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track final generation
	rr.Status.FailurePhase = &failurePhase
	rr.Status.FailureReason = &failureReason
	now := metav1.Now()
	rr.Status.CompletedAt = &now // #265 F3: CompletedAt on all terminal transitions

	// BR-ORCH-043: Set Ready condition (terminal failure)
	remediationrequest.SetReady(rr, false, remediationrequest.ReasonRemediationFailed, "Remediation failed", r.Metrics)

	// Increment consecutive failures (this happens for all failures, not just pre-execution)
	rr.Status.ConsecutiveFailureCount++

	// Calculate and set exponential backoff if below threshold
	// (At threshold, routing engine's CheckConsecutiveFailures will block with fixed cooldown)
	if rr.Status.ConsecutiveFailureCount < int32(r.routingEngine.Config().ConsecutiveFailureThreshold) {
		// Calculate backoff: 1min → 2min → 4min → 8min → 10min (capped)
		backoff := r.routingEngine.CalculateExponentialBackoff(rr.Status.ConsecutiveFailureCount)
		if backoff > 0 {
			nextAllowed := metav1.NewTime(time.Now().Add(backoff))
			rr.Status.NextAllowedExecution = &nextAllowed
			logger.Info("Set exponential backoff for failure",
				"consecutiveFailures", rr.Status.ConsecutiveFailureCount,
				"backoff", backoff.Round(time.Second),
				"nextAllowedExecution", nextAllowed.Format(time.RFC3339))
		}
	}
}

// logConsecutiveFailureThresholdReached logs (observability only, no state
// change) when this failure would push the fingerprint's consecutive-failure
// count to or past the routing engine's blocking threshold (BR-ORCH-042).
// NOTE: This RR still transitions to Failed (terminal state); the routing
// engine blocks FUTURE RRs with the same fingerprint, not this one. Extracted
// from transitionToFailed per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) logConsecutiveFailureThresholdReached(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, logger logr.Logger) {
	if failurePhase == remediationv1.FailurePhaseBlocked {
		return
	}
	// Count consecutive failures BEFORE this one (current failure not yet recorded)
	consecutiveFailures := r.countConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)
	// +1 for this failure (not yet in status)
	if consecutiveFailures+1 >= r.getConsecutiveFailureThreshold() {
		logger.Info("Consecutive failure threshold reached, future RRs will be blocked",
			"consecutiveFailures", consecutiveFailures+1,
			"threshold", r.getConsecutiveFailureThreshold(),
			"fingerprint", rr.Spec.SignalFingerprint,
		)
	}
}

// createEscalationNotificationIfNeeded creates an escalation
// NotificationRequest for a terminal failure (GAP-4 / Issue #808,
// BR-ORCH-036), unless the caller already created a ManualReview or
// Escalation NR for this RR. Extracted from transitionToFailed per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) createEscalationNotificationIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureReason string, logger logr.Logger) {
	manualReviewNR := fmt.Sprintf("nr-manual-review-%s", rr.Name)
	escalationNR := fmt.Sprintf("nr-escalation-%s", rr.Name)
	if hasNotificationRef(rr, manualReviewNR) || hasNotificationRef(rr, escalationNR) {
		return
	}
	escCtx := &creator.EscalationContext{
		FailurePhase:  string(failurePhase),
		FailureReason: failureReason,
	}
	notifName, notifErr := r.notificationCreator.CreateEscalationNotification(ctx, rr, escCtx)
	if notifErr != nil {
		logger.Error(notifErr, "Failed to create escalation notification (non-critical)")
		return
	}
	logger.Info("Created escalation notification for terminal failure", "notification", notifName)
	ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist escalation NR ref (non-critical)", "notification", notifName)
	}
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
			fmt.Sprintf("Escalation notification created: %s", notifName))
	}
}
