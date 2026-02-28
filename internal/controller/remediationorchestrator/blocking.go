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
// BR-ORCH-042.1, BR-ORCH-042.3, BR-GATEWAY-185 v1.1
// Validated by: test/unit/remediationorchestrator/blocking_test.go
// ========================================

// DefaultBlockThreshold is the number of consecutive failures before blocking.
// Reference: BR-ORCH-042.1
const DefaultBlockThreshold = 3

// DefaultCooldownDuration is how long to block before allowing retry.
// Reference: BR-ORCH-042.3
const DefaultCooldownDuration = 1 * time.Hour

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
			// Failed RR - increment counter
			// Note: BR-ORCH-042 specifically says "Failed RRs" - TimedOut not counted
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
		logger.Info("Blocked cooldown expired, transitioning to terminal Failed")

		// BR-ORCH-042: Record cooldown expiry metrics (TDD validated)
		r.Metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
		r.Metrics.BlockedCooldownExpiredTotal.Inc()

		// Get block reason for the failure message
		blockReason := "unknown"
		if rr.Status.BlockReason != "" {
			blockReason = rr.Status.BlockReason
		}

		// Transition to terminal Failed (skip blocking check to avoid infinite loop)
		return r.transitionToFailedTerminal(ctx, rr, "blocked",
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
func (r *Reconciler) transitionToFailedTerminal(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase string, failureErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	startTime := rr.CreationTimestamp.Time

	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
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

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
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
