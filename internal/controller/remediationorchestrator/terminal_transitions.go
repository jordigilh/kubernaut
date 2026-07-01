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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
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
func (r *Reconciler) transitionToInheritedCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, sourceRef, sourceKind string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	oldPhase := rr.Status.OverallPhase
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		rr.Status.OverallPhase = phase.Completed
		rr.Status.Outcome = "Remediated"
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

// handleBlocked updates RR status when routing is blocked and requeues appropriately.
// This function is called when CheckBlockingConditions() finds a blocking condition.
//
// Reference: DD-RO-002 (Centralized Routing), DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *Reconciler) handleBlocked(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	blocked *routing.BlockingCondition,
	fromPhase string,
	workflowID string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"blockReason", blocked.Reason,
		"blockMessage", blocked.Message,
	)

	// Emit routing blocked audit event (DD-RO-002, ADR-032 §1)
	r.emitRoutingBlockedAudit(ctx, rr, fromPhase, blocked, workflowID)

	// DD-EVENT-001: Emit K8s event based on blocking reason (BR-ORCH-095)
	if r.Recorder != nil {
		switch remediationv1.BlockReason(blocked.Reason) {
		case remediationv1.BlockReasonRecentlyRemediated, remediationv1.BlockReasonExponentialBackoff:
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonCooldownActive,
				fmt.Sprintf("Remediation deferred: %s", blocked.Message))
		case remediationv1.BlockReasonConsecutiveFailures:
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonConsecutiveFailureBlocked,
				fmt.Sprintf("Target blocked: %s", blocked.Message))
		case remediationv1.BlockReasonIneffectiveChain:
			r.Recorder.Event(rr, corev1.EventTypeWarning, "IneffectiveChainDetected",
				fmt.Sprintf("Escalating to manual review: %s", blocked.Message))
		}
	}

	// Issue #803: Create ManualReview NotificationRequest for IneffectiveChain blocks (BR-ORCH-036).
	// Previously, this relied on a non-existent "notification controller watching for ManualReviewRequired".
	if remediationv1.BlockReason(blocked.Reason) == remediationv1.BlockReasonIneffectiveChain {
		logger.Info("Ineffective chain detected - escalating to manual review",
			"remediationRequest", rr.Name)

		nrName := fmt.Sprintf("nr-manual-review-%s", rr.Name)
		if !hasNotificationRef(rr, nrName) {
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceRoutingEngine,
				Reason:  "IneffectiveChain",
				Message: blocked.Message,
			}
			notifName, notifErr := r.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
			if notifErr != nil {
				logger.Error(notifErr, "Failed to create manual review notification for IneffectiveChain block")
			} else {
				logger.Info("Created manual review notification for IneffectiveChain block", "notification", notifName)
				ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
				if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
					return nil
				}); refErr != nil {
					logger.Error(refErr, "Failed to persist IneffectiveChain NR ref (non-critical)", "notification", notifName)
				}
				if r.Recorder != nil {
					r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Manual review notification created: %s", notifName))
				}
			}
		}
	}

	// GAP-6 / #810: Create block notification for non-IneffectiveChain block reasons (BR-ORCH-036, BR-ORCH-042.5).
	if remediationv1.BlockReason(blocked.Reason) != remediationv1.BlockReasonIneffectiveChain {
		blockNRName := fmt.Sprintf("nr-block-%s-%s", strings.ToLower(blocked.Reason), rr.Name)
		if !hasNotificationRef(rr, blockNRName) {
			blockCtx := &creator.BlockNotificationContext{
				BlockReason:  blocked.Reason,
				BlockMessage: blocked.Message,
			}
			notifName, notifErr := r.notificationCreator.CreateBlockNotification(ctx, rr, blockCtx)
			if notifErr != nil {
				logger.Error(notifErr, "Failed to create block notification (non-critical)", "blockReason", blocked.Reason)
			} else {
				logger.Info("Created block notification", "notification", notifName, "blockReason", blocked.Reason)
				ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
				if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
					return nil
				}); refErr != nil {
					logger.Error(refErr, "Failed to persist block NR ref (non-critical)", "notification", notifName)
				}
				if r.Recorder != nil {
					r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Block notification created: %s", notifName))
				}
			}
		}
	}

	// Update RR status to Blocked phase (REFACTOR-RO-001: using retry helper)
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseBlocked
		rr.Status.BlockReason = remediationv1.BlockReason(blocked.Reason)
		rr.Status.BlockMessage = blocked.Message

		// Set time-based block fields
		if blocked.BlockedUntil != nil {
			rr.Status.BlockedUntil = &metav1.Time{Time: *blocked.BlockedUntil}
		} else {
			rr.Status.BlockedUntil = nil // Clear if not set
		}

		// Set WFE-based block fields
		if blocked.BlockingWorkflowExecution != "" {
			rr.Status.BlockingWorkflowExecution = blocked.BlockingWorkflowExecution
		} else {
			rr.Status.BlockingWorkflowExecution = "" // Clear if not set
		}

		// Set duplicate tracking
		if blocked.DuplicateOf != "" {
			rr.Status.DuplicateOf = blocked.DuplicateOf
		} else {
			rr.Status.DuplicateOf = "" // Clear if not set
		}

		// Issue #214: Set ManualReviewRequired for IneffectiveChain blocks
		if remediationv1.BlockReason(blocked.Reason) == remediationv1.BlockReasonIneffectiveChain {
			rr.Status.Outcome = "ManualReviewRequired"
			rr.Status.RequiresManualReview = true
		}

		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to update blocked status")
		return ctrl.Result{}, fmt.Errorf("failed to update blocked status: %w", err)
	}

	// BR-ORCH-042: Track blocking metrics (DD-METRICS-001)
	// Metric expects: []string{"namespace", "reason"}
	r.Metrics.BlockedTotal.WithLabelValues(rr.Namespace, blocked.Reason).Inc()

	// BR-ORCH-044: Track duplicate skips specifically
	if blocked.DuplicateOf != "" {
		r.Metrics.DuplicatesSkippedTotal.WithLabelValues(rr.Namespace, rr.Spec.SignalFingerprint).Inc()
	}

	// Emit metric (using existing metrics package)
	// V1.0: Basic counter, future enhancement: add duration histogram
	r.Metrics.PhaseTransitionsTotal.WithLabelValues(
		string(rr.Status.OverallPhase),     // from_phase
		string(remediationv1.PhaseBlocked), // to_phase
		rr.Namespace,
	).Inc()

	logger.Info("RemediationRequest blocked",
		"reason", blocked.Reason,
		"requeueAfter", blocked.RequeueAfter)

	// Requeue after specified duration
	return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
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

	if !phase.CanTransition(phase.Phase(oldPhase), newPhase) {
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
		// Update only RO-owned fields (preserves Gateway fields per DD-GATEWAY-011)
		rr.Status.OverallPhase = newPhase
		rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track processed generation
		now := metav1.Now()

		// Set phase start times
		switch newPhase {
		case phase.Processing:
			rr.Status.ProcessingStartTime = &now
		case phase.Analyzing:
			rr.Status.AnalyzingStartTime = &now
		case phase.Executing:
			rr.Status.ExecutingStartTime = &now
		}

		// Issue #636: Set Ready condition with phase-specific reason so that
		// `kubectl get rr` REASON column reflects the current pipeline stage.
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

	// Requeue with delay to check progress of child CRDs
	// Different phases have different check intervals
	var requeueAfter time.Duration
	switch newPhase {
	case phase.Processing, phase.Analyzing, phase.Executing:
		// Check child CRD progress every 5 seconds
		requeueAfter = 5 * time.Second
	default:
		// Quick requeue for other phases (using RequeueAfter instead of deprecated Requeue)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// transitionToVerifying transitions the RR to Verifying phase (#280).
// After WFE completes successfully, the RR enters Verifying (non-terminal) while the
// EffectivenessAssessment runs. The Gateway deduplicates signals during this window.
// RO transitions to Completed when EA reaches a terminal state (handleVerifyingPhase)
// or when VerificationDeadline expires.
// #281: Notification creation is delegated to ensureNotificationsCreated (idempotent).
// If creation fails here, handleVerifyingPhase retries on subsequent reconciles.
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
	// NOTE: This RR transitions to Failed (terminal state).
	// FUTURE RRs with same fingerprint will be blocked in Pending phase (routing check).
	if failurePhase != remediationv1.FailurePhaseBlocked {
		// Count consecutive failures BEFORE this one (current failure not yet recorded)
		consecutiveFailures := r.countConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)

		// +1 for this failure (not yet in status)
		if consecutiveFailures+1 >= r.getConsecutiveFailureThreshold() {
			logger.Info("Consecutive failure threshold reached, future RRs will be blocked",
				"consecutiveFailures", consecutiveFailures+1,
				"threshold", r.getConsecutiveFailureThreshold(),
				"fingerprint", rr.Spec.SignalFingerprint,
			)
			// Do NOT transition this RR to Blocked - it failed and should go to Failed.
			// The routing engine will block FUTURE RRs for this fingerprint.
		}
	}

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
	// Guard: skip if caller already created a ManualReview or Escalation NR.
	manualReviewNR := fmt.Sprintf("nr-manual-review-%s", rr.Name)
	escalationNR := fmt.Sprintf("nr-escalation-%s", rr.Name)
	if !hasNotificationRef(rr, manualReviewNR) && !hasNotificationRef(rr, escalationNR) {
		escCtx := &creator.EscalationContext{
			FailurePhase:  string(failurePhase),
			FailureReason: failureReason,
		}
		notifName, notifErr := r.notificationCreator.CreateEscalationNotification(ctx, rr, escCtx)
		if notifErr != nil {
			logger.Error(notifErr, "Failed to create escalation notification (non-critical)")
		} else {
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
	}

	// Capture old phase for metrics and audit
	oldPhaseBeforeTransition := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Failed
		rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track final generation
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &failureReason
		now := metav1.Now()
		rr.Status.CompletedAt = &now // #265 F3: CompletedAt on all terminal transitions

		// BR-ORCH-043: Set Ready condition (terminal failure)
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonRemediationFailed, "Remediation failed", r.Metrics)

		// DD-WE-004 V1.0: Set exponential backoff for pre-execution failures
		// Only applies when BELOW consecutive failure threshold (at threshold → 1-hour fixed block)
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

// handleGlobalTimeout transitions the RR to TimedOut phase when global timeout exceeded.
// BR-ORCH-027: Global Timeout Management
// Business Value: Prevents stuck remediations from consuming resources indefinitely
// Default timeout: 1 hour from CreationTimestamp
func (r *Reconciler) handleGlobalTimeout(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	timeoutPhase := remediationv1.RemediationPhase(rr.Status.OverallPhase)
	oldPhase := rr.Status.OverallPhase

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		now := metav1.Now()
		rr.Status.TimeoutTime = &now
		rr.Status.TimeoutPhase = &timeoutPhase
		rr.Status.CompletedAt = &now // #265 F3: CompletedAt on all terminal transitions

		// BR-ORCH-043: Set Ready condition (terminal timeout)
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonRemediationTimedOut, "Remediation timed out", r.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to TimedOut")
		return ctrl.Result{}, fmt.Errorf("failed to transition to TimedOut: %w", err)
	}

	// Record metrics (BR-ORCH-044)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(remediationv1.PhaseTimedOut), rr.Namespace).Inc()
		r.Metrics.TimeoutsTotal.WithLabelValues(rr.Namespace, string(timeoutPhase)).Inc()
	}

	// Per DD-AUDIT-003: Emit timeout event (lifecycle.completed with outcome=failure)
	if rr.Status.StartTime != nil {
		durationMs := time.Since(rr.Status.StartTime.Time).Milliseconds()
		r.emitTimeoutAudit(ctx, rr, "global", string(timeoutPhase), durationMs)
	}

	// DD-EVENT-001: Emit K8s event for global timeout (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationTimeout,
			fmt.Sprintf("Global timeout exceeded during %s phase", string(timeoutPhase)))
	}

	logger.Info("Remediation timed out (global timeout exceeded)",
		"timeoutPhase", timeoutPhase,
		"creationTimestamp", rr.CreationTimestamp)

	// ========================================
	// CREATE TIMEOUT NOTIFICATION (BR-ORCH-027)
	// Business Value: Operators notified for manual intervention
	// ========================================

	// Create notification for timeout escalation
	notificationName := fmt.Sprintf("timeout-%s", rr.Name)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notificationName,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityCritical,
			Severity: rr.Spec.Severity,
			Subject:  fmt.Sprintf("Remediation Timeout: %s", rr.Spec.SignalName),
			Body: r.notificationCreator.BuildGlobalTimeoutBody(
				rr.Spec.SignalName,
				rr.Name,
				string(timeoutPhase),
				r.getEffectiveGlobalTimeout(rr).String(),
				rr.Status.StartTime.Format(time.RFC3339),
				rr.Status.TimeoutTime.Format(time.RFC3339),
			),
			Context: buildTimeoutContext(rr.Name, string(timeoutPhase), "", rr.Spec.TargetResource),
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference on timeout notification")
		// Continue without notification - timeout transition is primary goal
		return ctrl.Result{}, nil
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on timeout notification")
		// Log error but don't fail timeout transition - timeout is primary goal
		return ctrl.Result{}, nil
	}

	// Create notification (non-blocking - timeout transition is primary goal)
	if err := r.client.Create(ctx, nr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Timeout notification already exists (concurrent create), continuing", "notificationName", notificationName)
		} else {
			logger.Error(err, "Failed to create timeout notification",
				"notificationName", notificationName)
			return ctrl.Result{}, nil
		}
	}

	logger.Info("Created timeout notification",
		"notificationName", notificationName,
		"priority", nr.Spec.Priority,
		"timeoutPhase", timeoutPhase)

	// DD-EVENT-001: Emit K8s event for notification creation (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
			fmt.Sprintf("Timeout notification created: %s", notificationName))
	}

	// Track notification in status (Recommendation #2, BR-ORCH-035)
	// REFACTOR-RO-001: Using retry helper
	err = helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Add notification to tracking list (BR-ORCH-035)
		notifRef := corev1.ObjectReference{
			Kind:       "NotificationRequest",
			Namespace:  nr.Namespace,
			Name:       nr.Name,
			UID:        nr.UID,
			APIVersion: "notification.kubernaut.ai/v1alpha1",
		}
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, notifRef)
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to track notification in status (non-critical)",
			"notificationName", notificationName)
		// Don't fail - notification was created successfully, tracking is best-effort
	} else {
		logger.V(1).Info("Tracked notification in status",
			"notificationName", notificationName,
			"totalNotifications", len(rr.Status.NotificationRequestRefs)+1)
	}

	// Issue #240: EA is NOT created on global timeout. See transitionToVerifying.

	return ctrl.Result{}, nil
}

// createEffectivenessAssessmentIfNeeded creates an EA CRD if the eaCreator is wired.
// ADR-EM-001: EA creation is ALWAYS non-fatal. The terminal phase transition must succeed
// even if EA creation fails. Errors are logged but not propagated.
// BR-HAPI-191: Resolves the target from AIAnalysis.RemediationTarget when available,
// so the EA assesses the resource the workflow actually modified (not the signal Pod).
// Batch 3: After creating the EA, persists the EffectivenessAssessmentRef on the RR status
// so that trackEffectivenessStatus can find the EA for condition tracking.
func (r *Reconciler) createEffectivenessAssessmentIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest) {
	if r.eaCreator == nil {
		return
	}

	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// DD-EM-003: Resolve dual targets for the EA.
	// Signal target: from RR (always available).
	// Remediation target: from AIAnalysis RemediationTarget (when available), else RR fallback.
	var dualTarget *creator.DualTarget
	var isGitOpsManaged bool
	var ai *aianalysisv1.AIAnalysis
	if rr.Status.AIAnalysisRef != nil {
		ai = &aianalysisv1.AIAnalysis{}
		if err := r.client.Get(ctx, client.ObjectKey{
			Name:      rr.Status.AIAnalysisRef.Name,
			Namespace: rr.Status.AIAnalysisRef.Namespace,
		}, ai); err != nil {
			logger.V(1).Info("Could not fetch AIAnalysis for target resolution (non-fatal), using RR target",
				"error", err)
			ai = nil
		} else {
			dualTarget = resolveDualTargets(rr, ai)
			// DD-EM-004, BR-RO-103.2: Read GitOps detection from RCA pipeline.
			if ai.Status.PostRCAContext != nil &&
				ai.Status.PostRCAContext.DetectedLabels != nil &&
				ai.Status.PostRCAContext.DetectedLabels.GitOpsManaged {
				isGitOpsManaged = true
			}
		}
	}

	// DD-EM-004 v2.0, BR-RO-103, Issue #253, #277: Detect async-managed targets.
	// Compute Duration-based hashComputeDelay for the EA Config.
	var hashComputeDelay *metav1.Duration
	remediationKind := rr.Spec.TargetResource.Kind
	if dualTarget != nil {
		remediationKind = dualTarget.Remediation.Kind
	}

	isCRD := false
	gvk, err := k8sutil.ResolveGVKForKind(r.restMapper, remediationKind)
	if err != nil {
		logger.V(1).Info("Cannot resolve GVK for kind, treating as sync target for hash timing",
			"kind", remediationKind, "error", err)
	} else if !creator.IsBuiltInGroup(gvk.Group) {
		isCRD = true
	}

	asyncCfg := r.getAsyncPropagation()
	propagationDelay := asyncCfg.ComputePropagationDelay(isGitOpsManaged, isCRD)
	if propagationDelay > 0 {
		hashComputeDelay = &metav1.Duration{Duration: propagationDelay}
		logger.Info("Async-managed target detected, setting hash check delay",
			"kind", remediationKind,
			"gitOps", isGitOpsManaged,
			"isCRD", isCRD,
			"hashComputeDelay", propagationDelay)
	}

	// #277: Detect proactive signals via AIAnalysis.Spec.AnalysisRequest.SignalContext.SignalMode.
	// Proactive alerts (e.g. predict_linear) need extra time to resolve.
	var alertCheckDelay *metav1.Duration
	if ai != nil && ai.Spec.AnalysisRequest.SignalContext.SignalMode == "proactive" {
		if asyncCfg.ProactiveAlertDelay > 0 {
			alertCheckDelay = &metav1.Duration{Duration: asyncCfg.ProactiveAlertDelay}
			logger.Info("Proactive signal detected, setting alert check delay",
				"signalMode", ai.Spec.AnalysisRequest.SignalContext.SignalMode,
				"alertCheckDelay", asyncCfg.ProactiveAlertDelay)
		}
	}

	name, err := r.eaCreator.CreateEffectivenessAssessment(ctx, rr, dualTarget, hashComputeDelay, alertCheckDelay)
	if err != nil {
		logger.Error(err, "Failed to create EffectivenessAssessment (non-fatal per ADR-EM-001)")
		return
	}
	logger.Info("EffectivenessAssessment created", "eaName", name, "rrPhase", rr.Status.OverallPhase)

	// #277: Emit orchestrator.ea.created audit event with propagation delay breakdown.
	r.emitEACreatedAudit(ctx, rr, name, hashComputeDelay, alertCheckDelay, isGitOpsManaged, isCRD)

	// ADR-EM-001, Batch 3: Persist EA ref on RR status for trackEffectivenessStatus.
	// Uses helpers.UpdateRemediationRequestStatus for atomic persistence (same pattern
	// as NT ref tracking in handleGlobalTimeout).
	// GAP-2 (ADR-EM-001 Section 9.4.15): Also set initial EffectivenessAssessed=False /
	// AssessmentInProgress so operators can distinguish "no EA yet" from "EA in progress."
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       name,
			Namespace:  rr.Namespace,
			APIVersion: eav1.GroupVersion.String(),
		}
		meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:    ConditionEffectivenessAssessed,
			Status:  metav1.ConditionFalse,
			Reason:  "AssessmentInProgress",
			Message: fmt.Sprintf("EffectivenessAssessment %s created, assessment in progress", name),
		})
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist EA ref on RR status (non-critical)", "eaName", name)
	}
}
