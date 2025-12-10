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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
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

// ========================================
// BLOCK REASON CONSTANTS
// ========================================

// BlockReasonConsecutiveFailures indicates blocking due to ≥3 consecutive failures.
const BlockReasonConsecutiveFailures = "consecutive_failures_exceeded"

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
		case string(phase.Failed):
			// Failed RR - increment counter
			// Note: BR-ORCH-042 specifically says "Failed RRs" - TimedOut not counted
			consecutiveFailures++

		case string(phase.Completed):
			// Completed RR - success resets the counter (AC-042-1-2)
			logger.V(1).Info("Found Completed RR, resetting failure count",
				"consecutiveFailures", consecutiveFailures,
				"completedRR", rr.Name,
			)
			return consecutiveFailures

		case string(phase.Blocked):
			// Blocked RR - skip (don't double-count the blocking trigger)
			continue

		case string(phase.Skipped):
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

// shouldBlockSignal determines if a signal should be blocked based on failure history.
// Returns (shouldBlock, reason) tuple.
//
// Reference: BR-ORCH-042.1
func (r *Reconciler) shouldBlockSignal(ctx context.Context, fingerprint string) (bool, string) {
	consecutiveFailures := r.countConsecutiveFailures(ctx, fingerprint)

	// Note: We check >= threshold because this is called AFTER the current
	// failure has been recorded, so the count already includes this failure.
	if consecutiveFailures >= DefaultBlockThreshold {
		return true, BlockReasonConsecutiveFailures
	}
	return false, ""
}

// transitionToBlocked transitions the RR to Blocked phase with cooldown.
// This is a non-terminal phase - Gateway will see this RR as "active" and
// update deduplication instead of creating new RRs.
//
// Reference: BR-ORCH-042.2, BR-ORCH-042.3
func (r *Reconciler) transitionToBlocked(ctx context.Context, rr *remediationv1.RemediationRequest, reason string, cooldown time.Duration) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	blockedUntil := metav1.NewTime(time.Now().Add(cooldown))
	blockMessage := fmt.Sprintf("Signal blocked due to %s. Will unblock at %s",
		reason, blockedUntil.Format(time.RFC3339))

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Transition to Blocked phase
		rr.Status.OverallPhase = string(phase.Blocked)
		rr.Status.BlockedUntil = &blockedUntil
		rr.Status.BlockReason = &reason
		rr.Status.Message = blockMessage

		return r.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Blocked phase")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Blocked: %w", err)
	}

	// TODO(BR-ORCH-042): Add metrics after TDD metrics tests
	// metrics.BlockedTotal.WithLabelValues(rr.Namespace, reason).Inc()
	// metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Inc()

	logger.Info("Signal blocked due to consecutive failures",
		"reason", reason,
		"blockedUntil", blockedUntil.Format(time.RFC3339),
		"cooldownDuration", cooldown,
	)

	// Requeue at exactly blockedUntil time for efficient handling
	return ctrl.Result{RequeueAfter: cooldown}, nil
}

// handleBlockedPhase handles the Blocked phase.
// Checks if cooldown has expired and transitions to terminal Failed if so.
// Gateway sees Blocked as "active" so won't create new RRs until expiry.
//
// Reference: BR-ORCH-042.3
func (r *Reconciler) handleBlockedPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Manual block without auto-expiry
	if rr.Status.BlockedUntil == nil {
		logger.V(1).Info("RR is manually blocked, no auto-expiry")
		return ctrl.Result{}, nil
	}

	// Check if cooldown has expired
	if time.Now().After(rr.Status.BlockedUntil.Time) {
		logger.Info("Blocked cooldown expired, transitioning to terminal Failed")

		// TODO(BR-ORCH-042): Add metrics after TDD metrics tests
		// metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
		// metrics.BlockedCooldownExpiredTotal.Inc()

		// Get block reason for the failure message
		blockReason := "unknown"
		if rr.Status.BlockReason != nil {
			blockReason = *rr.Status.BlockReason
		}

		// Transition to terminal Failed (skip blocking check to avoid infinite loop)
		return r.transitionToFailedTerminal(ctx, rr, "blocked",
			fmt.Sprintf("Cooldown expired after blocking due to %s", blockReason))
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
func (r *Reconciler) transitionToFailedTerminal(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase, failureReason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	startTime := rr.CreationTimestamp.Time

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		rr.Status.OverallPhase = string(phase.Failed)
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &failureReason
		// Clear blocking fields since we're transitioning to terminal
		rr.Status.BlockedUntil = nil
		// Keep BlockReason for audit trail

		return r.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to transition to terminal Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Failed: %w", err)
	}

	// Emit audit event (DD-AUDIT-003)
	durationMs := time.Since(startTime).Milliseconds()
	r.emitFailureAudit(ctx, rr, failurePhase, failureReason, durationMs)

	logger.Info("Remediation failed (terminal)",
		"failurePhase", failurePhase,
		"reason", failureReason,
	)
	return ctrl.Result{}, nil
}

