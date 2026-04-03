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

// Package controller provides the Kubernetes controller for RemediationRequest CRDs.
//
// This file implements consecutive failure blocking logic per BR-ORCH-042.
// When a signal fingerprint fails ≥3 consecutive times, RO holds the RR in a
// non-terminal Blocked phase for a cooldown period before allowing retry.
//
// Business Requirements:
// - BR-ORCH-042: Consecutive Failure Blocking with Automatic Cooldown
// - BR-GATEWAY-185 v1.1: Field selector on spec.signalFingerprint (not labels)
//
// Design Decision:
// - DD-GATEWAY-011 v1.3: Blocking logic moved from Gateway to RO
//
// TDD Implementation:
// - RED: Tests in test/unit/remediationorchestrator/blocking_test.go
// - GREEN: Constants + methods implementation
// - Tests validated: Unit (constants), Integration (methods)
package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// ========================================
// BLOCKING CONFIGURATION CONSTANTS
// BR-GATEWAY-185 v1.1
// Validated by: test/unit/remediationorchestrator/blocking_test.go
// ========================================

// FingerprintFieldIndex is the field index key for spec.signalFingerprint.
// Used for O(1) lookups. Set up in SetupWithManager().
// Reference: BR-GATEWAY-185 v1.1
const FingerprintFieldIndex = "spec.signalFingerprint"

// Issue #91: Field index for child CRD lookups by parent RemediationRequest name
const RemediationRequestRefNameIndex = "spec.remediationRequestRef.name"

// ========================================
// BLOCKING LOGIC METHODS
// Validated by: test/integration/remediationorchestrator/blocking_integration_test.go
// ========================================

// countConsecutiveFailures counts consecutive Failed RRs for a fingerprint.
// It lists all RRs with the same fingerprint using field selector on
// spec.signalFingerprint (immutable, full 64-char), sorts by creation time
// (newest first), and counts consecutive Failed phases until it hits a
// Completed or non-terminal phase.
//
// Reference: BR-ORCH-042.1, BR-GATEWAY-185 v1.1
//
// Returns 0 if:
// - No RRs exist for fingerprint
// - List operation fails (conservative - don't block on error)
// - Most recent RR is Completed (reset counter)
func (r *Reconciler) countConsecutiveFailures(ctx context.Context, fingerprint string) int {
	logger := log.FromContext(ctx).WithValues("fingerprint", fingerprint)

	// List all RRs with matching fingerprint using field selector
	// BR-GATEWAY-185 v1.1: Use spec.signalFingerprint (immutable, 64 chars)
	// NOT labels (mutable, truncated to 63 chars)
	rrList := &remediationv1.RemediationRequestList{}
	if err := r.client.List(ctx, rrList,
		client.MatchingFields{FingerprintFieldIndex: fingerprint},
	); err != nil {
		logger.Error(err, "Failed to list RRs for consecutive failure count - assuming 0")
		return 0 // Conservative: don't block on error
	}

	if len(rrList.Items) == 0 {
		return 0
	}

	// Sort by creation timestamp, newest first (AC-042-1-3: chronological order)
	sort.Slice(rrList.Items, func(i, j int) bool {
		return rrList.Items[i].CreationTimestamp.After(rrList.Items[j].CreationTimestamp.Time)
	})

	consecutiveFailures := 0
	for _, rr := range rrList.Items {
		switch rr.Status.OverallPhase {
		case phase.Failed:
			// Issue #190: Skip inherited/deduplicated failures — they don't represent
			// actual remediation attempts and should not count toward blocking.
			if rr.Status.FailurePhase != nil && *rr.Status.FailurePhase == remediationv1.FailurePhaseDeduplicated {
				continue
			}
			consecutiveFailures++

		case phase.Completed:
			// Completed RR - success resets the counter (AC-042-1-2)
			logger.V(1).Info("Found Completed RR, resetting failure count",
				"consecutiveFailures", consecutiveFailures,
				"completedRR", rr.Name,
			)
			return consecutiveFailures

		case phase.Blocked:
			// Blocked RR - skip (don't double-count the blocking trigger)
			continue

		case phase.Skipped:
			// Skipped RR - not a remediation failure, skip
			// Skipped means resource lock prevented execution, not remediation failure
			continue

		default:
			// Active/in-progress phases - skip (not terminal)
			continue
		}
	}

	logger.V(1).Info("Counted consecutive failures",
		"consecutiveFailures", consecutiveFailures,
		"totalRRsChecked", len(rrList.Items),
	)
	return consecutiveFailures
}

// handleBlockedPhase handles the Blocked phase.
// Checks if cooldown has expired and transitions to terminal Failed if so.
// Gateway sees Blocked as "active" so won't create new RRs until expiry.
//
// Reference: BR-ORCH-042.3
func (r *Reconciler) handleBlockedPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Blocks without auto-expiry: event-based blocks need periodic rechecks,
	// manual blocks stay until explicitly cleared.
	if rr.Status.BlockedUntil == nil {
		switch remediationv1.BlockReason(rr.Status.BlockReason) {
		case remediationv1.BlockReasonResourceBusy:
			return r.recheckResourceBusyBlock(ctx, rr)
		case remediationv1.BlockReasonDuplicateInProgress:
			return r.recheckDuplicateBlock(ctx, rr)
		default:
			logger.V(1).Info("RR is manually blocked, no auto-expiry")
			return ctrl.Result{}, nil
		}
	}

	// Check if cooldown has expired
	if time.Now().After(rr.Status.BlockedUntil.Time) {
		// BR-SCOPE-010: UnmanagedResource blocks re-validate scope instead of failing.
		// Bug #266: Previously all timed blocks went to Failed on expiry.
		if remediationv1.BlockReason(rr.Status.BlockReason) == remediationv1.BlockReasonUnmanagedResource {
			return r.handleUnmanagedResourceExpiry(ctx, rr)
		}

		logger.Info("Blocked cooldown expired, transitioning to terminal Failed")

		// BR-ORCH-042: Record cooldown expiry (CurrentBlockedGauge decrement)
		r.Metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()

		blockReason := "unknown"
		if rr.Status.BlockReason != "" {
			blockReason = string(rr.Status.BlockReason)
		}

		return r.transitionToFailedTerminal(ctx, rr, remediationv1.FailurePhaseBlocked,
			fmt.Errorf("cooldown expired after blocking due to %s", blockReason))
	}

	// Still in cooldown - requeue at exact expiry time
	requeueAfter := time.Until(rr.Status.BlockedUntil.Time)
	logger.V(1).Info("Still blocked, requeueing at expiry",
		"blockedUntil", rr.Status.BlockedUntil.Format(time.RFC3339),
		"requeueAfter", requeueAfter,
	)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// transitionToFailedTerminal is the terminal Failed transition that skips blocking check.
// Used when transitioning from Blocked after cooldown expiry.
// This prevents infinite loops: Failed -> Blocked -> Failed -> Blocked...
func (r *Reconciler) transitionToFailedTerminal(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	startTime := rr.CreationTimestamp.Time

	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Failed
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &failureReason
		// Clear blocking fields since we're transitioning to terminal
		rr.Status.BlockedUntil = nil
		// Keep BlockReason for audit trail

		// BR-ORCH-043: Set Ready condition (terminal blocked)
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonNotReady, "Remediation blocked", r.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to terminal Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Failed: %w", err)
	}

	// Emit audit event (DD-AUDIT-003)
	durationMs := time.Since(startTime).Milliseconds()
	r.emitFailureAudit(ctx, rr, failurePhase, failureErr, durationMs)

	logger.Info("Remediation failed (terminal)",
		"failurePhase", failurePhase,
		"reason", failureReason,
	)
	return ctrl.Result{}, nil
}

// handleUnmanagedResourceExpiry re-validates scope when an UnmanagedResource block expires.
// If still unmanaged: re-block via handleBlocked (emits routing.blocked audit, updates status)
//   with incremented ConsecutiveFailureCount for backoff progression.
// If now managed: clear block fields, transition to Pending, emit phase transition audit.
//
// Reference: BR-SCOPE-010, ADR-053 (Resource Scope Management), Bug #266
func (r *Reconciler) handleUnmanagedResourceExpiry(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("UnmanagedResource block expired — re-validating scope")

	blocked := r.routingEngine.CheckUnmanagedResource(ctx, rr)

	if blocked != nil {
		// Still unmanaged — increment failure count (persisted in status update) then re-block.
		// ConsecutiveFailureCount drives backoff progression in CheckUnmanagedResource.
		logger.Info("Resource still unmanaged — re-blocking with increased backoff",
			"newBackoff", blocked.RequeueAfter)

		// Persist the failure count increment first, then re-block via handleBlocked.
		// handleBlocked's UpdateRemediationRequestStatus refetches the RR, so we must
		// persist the increment in a separate update to avoid it being lost.
		err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.ConsecutiveFailureCount++
			return nil
		})
		if err != nil {
			logger.Error(err, "Failed to increment ConsecutiveFailureCount for re-block")
			return ctrl.Result{}, fmt.Errorf("failed to increment failure count for scope re-block: %w", err)
		}

		return r.handleBlocked(ctx, rr, blocked, string(remediationv1.PhaseBlocked), "")
	}

	// Now managed — transition to Pending for re-processing
	logger.Info("Resource is now managed — unblocking and transitioning to Pending")

	oldPhase := string(rr.Status.OverallPhase)
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Pending
		rr.Status.BlockReason = ""
		rr.Status.BlockMessage = ""
		rr.Status.BlockedUntil = nil
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition from Blocked to Pending after scope unblock")
		return ctrl.Result{}, fmt.Errorf("failed to unblock after scope re-validation: %w", err)
	}

	r.Metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()

	// ADR-032 §1: Audit the phase transition (Blocked → Pending)
	r.emitPhaseTransitionAudit(ctx, rr, oldPhase, string(phase.Pending))

	return ctrl.Result{Requeue: true}, nil
}

// recheckResourceBusyBlock handles Blocked RRs with BlockReason=ResourceBusy.
// Uses apiReader to check if the blocking WFE has reached a terminal phase.
// If cleared, resets the RR to Analyzing so routing checks can re-run.
//
// Reference: DD-RO-002 (Resource Locking), DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *Reconciler) recheckResourceBusyBlock(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.BlockingWorkflowExecution == "" {
		logger.Info("ResourceBusy block has no blocking WFE reference, clearing")
		return r.clearEventBasedBlock(ctx, rr, phase.Analyzing)
	}

	wfe := &workflowexecutionv1.WorkflowExecution{}
	err := r.apiReader.Get(ctx, client.ObjectKey{
		Name:      rr.Status.BlockingWorkflowExecution,
		Namespace: rr.Namespace,
	}, wfe)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Blocking WFE no longer exists, clearing ResourceBusy block",
				"blockingWFE", rr.Status.BlockingWorkflowExecution)
			return r.clearEventBasedBlock(ctx, rr, phase.Analyzing)
		}
		logger.Error(err, "Failed to check blocking WFE status")
		return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
	}

	if wfe.Status.Phase == workflowexecutionv1.PhaseCompleted ||
		wfe.Status.Phase == workflowexecutionv1.PhaseFailed {
		logger.Info("Blocking WFE reached terminal phase, clearing ResourceBusy block",
			"blockingWFE", wfe.Name, "wfePhase", wfe.Status.Phase)
		return r.clearEventBasedBlock(ctx, rr, phase.Analyzing)
	}

	logger.V(1).Info("Blocking WFE still active, requeueing",
		"blockingWFE", wfe.Name, "wfePhase", wfe.Status.Phase)
	return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
}

// recheckDuplicateBlock handles Blocked RRs with BlockReason=DuplicateInProgress.
// Uses apiReader to check if the original RR has reached a terminal phase.
// If cleared, resets the RR to Pending so the full pipeline can re-run.
//
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *Reconciler) recheckDuplicateBlock(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.DuplicateOf == "" {
		logger.Info("DuplicateInProgress block has no original RR reference, clearing")
		return r.clearEventBasedBlock(ctx, rr, phase.Pending)
	}

	originalRR := &remediationv1.RemediationRequest{}
	err := r.apiReader.Get(ctx, client.ObjectKey{
		Name:      rr.Status.DuplicateOf,
		Namespace: rr.Namespace,
	}, originalRR)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Original RR no longer exists, clearing DuplicateInProgress block",
				"duplicateOf", rr.Status.DuplicateOf)
			return r.clearEventBasedBlock(ctx, rr, phase.Pending)
		}
		logger.Error(err, "Failed to check original RR status")
		return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
	}

	if IsTerminalPhase(originalRR.Status.OverallPhase) {
		logger.Info("Original RR reached terminal phase, clearing DuplicateInProgress block",
			"duplicateOf", originalRR.Name, "originalPhase", originalRR.Status.OverallPhase)
		return r.clearEventBasedBlock(ctx, rr, phase.Pending)
	}

	logger.V(1).Info("Original RR still active, requeueing",
		"duplicateOf", originalRR.Name, "originalPhase", originalRR.Status.OverallPhase)
	return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
}

// clearEventBasedBlock clears event-based block fields and resets the RR phase
// so the reconciliation pipeline can resume.
func (r *Reconciler) clearEventBasedBlock(ctx context.Context, rr *remediationv1.RemediationRequest, resumePhase phase.Phase) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = resumePhase
		rr.Status.BlockReason = ""
		rr.Status.BlockMessage = ""
		rr.Status.BlockingWorkflowExecution = ""
		rr.Status.DuplicateOf = ""
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to clear event-based block")
		return ctrl.Result{}, fmt.Errorf("failed to clear event-based block: %w", err)
	}

	r.Metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
	return ctrl.Result{Requeue: true}, nil
}
