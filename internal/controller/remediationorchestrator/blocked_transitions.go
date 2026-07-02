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

// handleBlocked and its helpers (K8s event, ineffective-chain escalation,
// notification, status update, metrics) for routing-blocked RemediationRequests
// (DD-RO-002, DD-RO-002-ADDENDUM). Split out of terminal_transitions.go per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file
// under the 700-line convention threshold. Pure structural move — no
// behavior change.
package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

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
	r.emitBlockedK8sEvent(rr, blocked)

	isIneffectiveChain := remediationv1.BlockReason(blocked.Reason) == remediationv1.BlockReasonIneffectiveChain

	// Issue #803: Create ManualReview NotificationRequest for IneffectiveChain blocks (BR-ORCH-036).
	if isIneffectiveChain {
		r.escalateIneffectiveChainToManualReview(ctx, rr, blocked, logger)
	}

	// GAP-6 / #810: Create block notification for non-IneffectiveChain block reasons (BR-ORCH-036, BR-ORCH-042.5).
	if !isIneffectiveChain {
		r.createBlockNotification(ctx, rr, blocked, logger)
	}

	// Update RR status to Blocked phase (REFACTOR-RO-001: using retry helper)
	if err := r.updateBlockedStatus(ctx, rr, blocked, isIneffectiveChain); err != nil {
		logger.Error(err, "Failed to update blocked status")
		return ctrl.Result{}, fmt.Errorf("failed to update blocked status: %w", err)
	}

	r.recordBlockedMetrics(rr, blocked)

	logger.Info("RemediationRequest blocked",
		"reason", blocked.Reason,
		"requeueAfter", blocked.RequeueAfter)

	// Requeue after specified duration
	return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
}

// emitBlockedK8sEvent emits the K8s event corresponding to a routing block
// reason (BR-ORCH-095, DD-EVENT-001). Extracted from handleBlocked per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) emitBlockedK8sEvent(rr *remediationv1.RemediationRequest, blocked *routing.BlockingCondition) {
	if r.Recorder == nil {
		return
	}
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

// escalateIneffectiveChainToManualReview creates a ManualReview
// NotificationRequest for IneffectiveChain blocks (Issue #803, BR-ORCH-036).
// Previously, this relied on a non-existent "notification controller
// watching for ManualReviewRequired". Extracted from handleBlocked per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) escalateIneffectiveChainToManualReview(ctx context.Context, rr *remediationv1.RemediationRequest, blocked *routing.BlockingCondition, logger logr.Logger) {
	logger.Info("Ineffective chain detected - escalating to manual review",
		"remediationRequest", rr.Name)

	nrName := fmt.Sprintf("nr-manual-review-%s", rr.Name)
	if hasNotificationRef(rr, nrName) {
		return
	}
	reviewCtx := &creator.ManualReviewContext{
		Source:  notificationv1.ReviewSourceRoutingEngine,
		Reason:  "IneffectiveChain",
		Message: blocked.Message,
	}
	notifName, notifErr := r.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
	if notifErr != nil {
		logger.Error(notifErr, "Failed to create manual review notification for IneffectiveChain block")
		return
	}
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

// createBlockNotification creates a block NotificationRequest for
// non-IneffectiveChain block reasons (GAP-6 / Issue #810, BR-ORCH-036,
// BR-ORCH-042.5). Extracted from handleBlocked per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) createBlockNotification(ctx context.Context, rr *remediationv1.RemediationRequest, blocked *routing.BlockingCondition, logger logr.Logger) {
	blockNRName := fmt.Sprintf("nr-block-%s-%s", strings.ToLower(blocked.Reason), rr.Name)
	if hasNotificationRef(rr, blockNRName) {
		return
	}
	blockCtx := &creator.BlockNotificationContext{
		BlockReason:  blocked.Reason,
		BlockMessage: blocked.Message,
	}
	notifName, notifErr := r.notificationCreator.CreateBlockNotification(ctx, rr, blockCtx)
	if notifErr != nil {
		logger.Error(notifErr, "Failed to create block notification (non-critical)", "blockReason", blocked.Reason)
		return
	}
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

// updateBlockedStatus atomically transitions the RR status into the Blocked
// phase, stamping time-based, WFE-based, and duplicate-tracking block fields
// (REFACTOR-RO-001, Issue #214). Extracted from handleBlocked per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) updateBlockedStatus(ctx context.Context, rr *remediationv1.RemediationRequest, blocked *routing.BlockingCondition, isIneffectiveChain bool) error {
	return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseBlocked
		rr.Status.BlockReason = remediationv1.BlockReason(blocked.Reason)
		rr.Status.BlockMessage = blocked.Message

		// Set time-based block fields (nil clears any prior value)
		if blocked.BlockedUntil != nil {
			rr.Status.BlockedUntil = &metav1.Time{Time: *blocked.BlockedUntil}
		} else {
			rr.Status.BlockedUntil = nil
		}

		// Set WFE-based block fields ("" clears any prior value)
		rr.Status.BlockingWorkflowExecution = blocked.BlockingWorkflowExecution

		// Set duplicate tracking ("" clears any prior value)
		rr.Status.DuplicateOf = blocked.DuplicateOf

		// Issue #214: Set ManualReviewRequired for IneffectiveChain blocks
		if isIneffectiveChain {
			rr.Status.Outcome = "ManualReviewRequired"
			rr.Status.RequiresManualReview = true
		}

		return nil
	})
}

// recordBlockedMetrics tracks Prometheus counters for a routing block
// (BR-ORCH-042, BR-ORCH-044, DD-METRICS-001). Extracted from handleBlocked
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) recordBlockedMetrics(rr *remediationv1.RemediationRequest, blocked *routing.BlockingCondition) {
	// Metric expects: []string{"namespace", "reason"}
	r.Metrics.BlockedTotal.WithLabelValues(rr.Namespace, blocked.Reason).Inc()

	// BR-ORCH-044: Track duplicate skips specifically
	if blocked.DuplicateOf != "" {
		r.Metrics.DuplicatesSkippedTotal.WithLabelValues(rr.Namespace, rr.Spec.SignalFingerprint).Inc()
	}

	// V1.0: Basic counter, future enhancement: add duration histogram
	r.Metrics.PhaseTransitionsTotal.WithLabelValues(
		string(rr.Status.OverallPhase),     // from_phase
		string(remediationv1.PhaseBlocked), // to_phase
		rr.Namespace,
	).Inc()
}
