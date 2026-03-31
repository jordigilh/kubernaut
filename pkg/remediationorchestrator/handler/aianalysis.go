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

package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// AIAnalysisHandler handles AIAnalysis CRD status changes for the Remediation Orchestrator.
// Reference: BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
type AIAnalysisHandler struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	notificationCreator    *creator.NotificationCreator
	Metrics                *metrics.Metrics
	transitionToFailed     func(context.Context, *remediationv1.RemediationRequest, remediationv1.FailurePhase, error) (ctrl.Result, error)
	noActionRequiredDelay  time.Duration
	notifySelfResolved    bool
}

// NewAIAnalysisHandler creates a new AIAnalysisHandler.
// noActionDelay controls how long after a NoActionRequired completion the Gateway
// should suppress new RR creation for the same signal fingerprint (Issue #314).
// Pass 0 to disable suppression.
func NewAIAnalysisHandler(c client.Client, s *runtime.Scheme, nc *creator.NotificationCreator, m *metrics.Metrics, ttf func(context.Context, *remediationv1.RemediationRequest, remediationv1.FailurePhase, error) (ctrl.Result, error), noActionDelay time.Duration) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		client:                c,
		scheme:                s,
		notificationCreator:   nc,
		Metrics:               m,
		transitionToFailed:    ttf,
		noActionRequiredDelay: noActionDelay,
	}
}

// SetNotifySelfResolved enables or disables self-resolved status-update notifications.
// BR-ORCH-037 AC-037-08: When true, handleWorkflowNotNeeded creates an informational NR.
// Called from cmd/remediationorchestrator/main.go via Reconciler.SetNotifySelfResolved.
//
// CONCURRENCY: Must be called before mgr.Start(); not safe for concurrent use with reconcile loops.
func (h *AIAnalysisHandler) SetNotifySelfResolved(enabled bool) {
	h.notifySelfResolved = enabled
}

// buildNotificationRef fetches the NotificationRequest by name to obtain its UID
// and returns a fully populated ObjectReference (BR-ORCH-035 AC-6).
// If the fetch fails, UID is omitted (best-effort; Name+Namespace still sufficient for lookup).
func (h *AIAnalysisHandler) buildNotificationRef(ctx context.Context, name, namespace string) corev1.ObjectReference {
	ref := corev1.ObjectReference{
		Kind:       "NotificationRequest",
		Name:       name,
		Namespace:  namespace,
		APIVersion: "notification.kubernaut.ai/v1alpha1",
	}
	nr := &notificationv1.NotificationRequest{}
	if err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, nr); err == nil {
		ref.UID = nr.UID
	}
	return ref
}

// HandleAIAnalysisStatus processes AIAnalysis status changes and updates the parent RemediationRequest.
// This is called by the RO reconciler when an AIAnalysis status changes.
// Returns (requeue, error)
func (h *AIAnalysisHandler) HandleAIAnalysisStatus(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"aiPhase", ai.Status.Phase,
	)

	switch ai.Status.Phase {
	case "Completed":
		return h.handleCompleted(ctx, rr, ai)
	case "Failed":
		return h.handleFailed(ctx, rr, ai)
	case "Pending", "Investigating", "Analyzing":
		// In progress - no action needed, will be handled on next status change
		logger.V(1).Info("AIAnalysis in progress", "phase", ai.Status.Phase)
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown AIAnalysis phase", "phase", ai.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// handleCompleted processes AIAnalysis Completed phase.
// Handles both normal completion (create WE) and WorkflowNotNeeded (BR-ORCH-037).
func (h *AIAnalysisHandler) handleCompleted(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"reason", ai.Status.Reason,
	)

	// BR-ORCH-037: Check for WorkflowNotNeeded (problem self-resolved)
	if ai.Status.Reason == "WorkflowNotNeeded" {
		return h.handleWorkflowNotNeeded(ctx, rr, ai)
	}

	// Check if approval is required (BR-ORCH-001)
	if ai.Status.ApprovalRequired {
		logger.Info("AIAnalysis requires approval, creating approval notification")
		return h.handleApprovalRequired(ctx, rr, ai)
	}

	// Normal completion - ready for WorkflowExecution
	// This is handled by the main reconciler which will create WE
	logger.Info("AIAnalysis completed successfully, ready for WorkflowExecution")
	return ctrl.Result{}, nil
}

// handleWorkflowNotNeeded processes AIAnalysis WorkflowNotNeeded (BR-ORCH-037).
// This occurs when the LLM determines the problem has self-resolved.
func (h *AIAnalysisHandler) handleWorkflowNotNeeded(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"subReason", ai.Status.SubReason,
	)

	logger.Info("AIAnalysis determined no workflow needed - problem self-resolved")

	// Update RR status (DD-GATEWAY-011, BR-ORCH-038)
	// REFACTOR-RO-001: Using retry helper to preserve Gateway fields
	err := helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Update only RO-owned fields
		rr.Status.OverallPhase = remediationv1.PhaseCompleted
		rr.Status.Outcome = "NoActionRequired"
		rr.Status.Message = ai.Status.Message
		now := metav1.Now()
		rr.Status.CompletedAt = &now

		// Issue #314: Suppress Gateway duplicate RR creation for the same signal
		// fingerprint. The Gateway's ShouldDeduplicate respects NextAllowedExecution
		// on terminal-phase RRs, preventing an infinite NoActionRequired loop.
		if h.noActionRequiredDelay > 0 {
			nextAllowed := metav1.NewTime(time.Now().Add(h.noActionRequiredDelay))
			rr.Status.NextAllowedExecution = &nextAllowed
		}

		// BR-ORCH-043: Set Ready condition (terminal success - no action required)
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonReady, "No action required", h.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for WorkflowNotNeeded")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Record metric (BR-ORCH-037)
	reason := "problem_resolved"
	if ai.Status.SubReason != "" {
		reason = ai.Status.SubReason
	}
	if h.Metrics != nil {
		h.Metrics.NoActionNeededTotal.WithLabelValues(reason, rr.Namespace).Inc()
	}

	// BR-ORCH-037 AC-037-08: Optional informational notification when configured.
	// Non-fatal: notification failure is logged but does not block handler completion.
	if h.notifySelfResolved {
		notifName, notifErr := h.notificationCreator.CreateSelfResolvedNotification(ctx, rr, ai)
		if notifErr != nil {
			logger.Error(notifErr, "Failed to create self-resolved notification (non-fatal)")
		} else {
		ref := h.buildNotificationRef(ctx, notifName, rr.Namespace)
		if updateErr := helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
			for _, existing := range rr.Status.NotificationRequestRefs {
				if existing.Name == ref.Name && existing.Namespace == ref.Namespace {
					return nil
				}
			}
			rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update RR with self-resolved notification ref (non-fatal)")
		}
		}
	}

	logger.Info("Remediation completed - no action required",
		"outcome", "NoActionRequired",
		"reason", reason,
		"nextAllowedExecution", rr.Status.NextAllowedExecution,
	)

	return ctrl.Result{}, nil
}

// handleApprovalRequired processes AIAnalysis when approval is needed (BR-ORCH-001).
func (h *AIAnalysisHandler) handleApprovalRequired(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
	)

	// Create approval notification
	notifName, err := h.notificationCreator.CreateApprovalNotification(ctx, rr, ai)
	if err != nil {
		logger.Error(err, "Failed to create approval notification")
		return ctrl.Result{}, fmt.Errorf("failed to create approval notification: %w", err)
	}

	// Update RR status with notification reference (DD-GATEWAY-011, BR-ORCH-038)
	// REFACTOR-RO-001: Using retry helper
	ref := h.buildNotificationRef(ctx, notifName, rr.Namespace)
	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		rr.Status.ApprovalNotificationSent = true
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status with approval notification ref")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	logger.Info("Created approval notification", "notificationName", notifName)
	return ctrl.Result{}, nil
}

// handleFailed processes AIAnalysis Failed phase.
// Handles NeedsHumanReview (BR-HAPI-197), WorkflowResolutionFailed (BR-ORCH-036),
// and infrastructure failures (BR-ORCH-036 v3.0: APIError, Timeout, MaxRetriesExceeded).
// All failure paths create escalation notifications before transitioning to Failed.
func (h *AIAnalysisHandler) handleFailed(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	// BR-HAPI-197: Check NeedsHumanReview FIRST (takes precedence over WorkflowResolutionFailed)
	// This flag is set by HAPI when AI cannot produce a reliable result
	if ai.Status.NeedsHumanReview {
		return h.handleHumanReviewRequired(ctx, rr, ai)
	}

	// BR-ORCH-036: Handle WorkflowResolutionFailed
	if ai.Status.Reason == "WorkflowResolutionFailed" {
		return h.handleWorkflowResolutionFailed(ctx, rr, ai)
	}

	// BR-ORCH-036 v3.0: Other failures (APIError, Timeout, MaxRetriesExceeded, etc.)
	// Create escalation notification and propagate failure to RR
	return h.propagateFailure(ctx, rr, ai)
}

// handleHumanReviewRequired processes AIAnalysis when HAPI explicitly requires human review (BR-HAPI-197).
// Issue #550: Routes to handleManualReviewCompleted when no workflow was selected (SelectedWorkflow=nil),
// or to the existing Failed path when a workflow was present but rejected (SelectedWorkflow!=nil).
func (h *AIAnalysisHandler) handleHumanReviewRequired(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	// Issue #550: No workflow selected + needs human review = valid completion, not failure
	if ai.Status.SelectedWorkflow == nil {
		return h.handleManualReviewCompleted(ctx, rr, ai)
	}

	// Existing path: has workflow but low confidence -> Failed
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"humanReviewReason", ai.Status.HumanReviewReason,
	)

	logger.Info("AIAnalysis requires human review (workflow present but rejected) - creating manual review notification")

	// Build manual review context with BR-HAPI-197 fields
	reviewCtx := &creator.ManualReviewContext{
		Source:            notificationv1.ReviewSourceAIAnalysis,
		Reason:            "HumanReviewRequired", // BR-HAPI-197 reason
		SubReason:         ai.Status.HumanReviewReason,
		Message:           ai.Status.Message,
		HumanReviewReason: ai.Status.HumanReviewReason, // BR-HAPI-197: Store for metadata
	}

	// Populate root cause and warnings (common pattern)
	h.populateManualReviewContext(reviewCtx, ai)

	// Create notification and update RR status (common pattern)
	return h.createManualReviewAndUpdateStatus(ctx, logger, rr, reviewCtx, "HumanReviewRequired", ai.Status.HumanReviewReason)
}

// handleManualReviewCompleted processes the case where HAPI requires human review but no workflow
// was selected (Issue #550). The RR transitions to Completed with Outcome=ManualReviewRequired,
// which is a valid terminal state (not a failure). This avoids inflating failure metrics and
// exponential backoff for cases where the LLM intentionally omitted a workflow.
func (h *AIAnalysisHandler) handleManualReviewCompleted(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"humanReviewReason", ai.Status.HumanReviewReason,
	)

	logger.Info("AIAnalysis requires human review (no workflow selected) - completing with ManualReviewRequired")

	// Build manual review context with BR-HAPI-197 fields
	reviewCtx := &creator.ManualReviewContext{
		Source:            notificationv1.ReviewSourceAIAnalysis,
		Reason:            "HumanReviewRequired",
		SubReason:         ai.Status.HumanReviewReason,
		Message:           ai.Status.Message,
		HumanReviewReason: ai.Status.HumanReviewReason,
	}
	h.populateManualReviewContext(reviewCtx, ai)

	// Create manual review notification
	notifName, err := h.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
	if err != nil {
		logger.Error(err, "Failed to create manual review notification")
		return ctrl.Result{}, fmt.Errorf("failed to create manual review notification: %w", err)
	}

	// Build notification reference for tracking (BR-ORCH-035 AC-6)
	ref := h.buildNotificationRef(ctx, notifName, rr.Namespace)

	// Update RR status to Completed with ManualReviewRequired outcome
	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseCompleted
		rr.Status.Outcome = "ManualReviewRequired"
		rr.Status.RequiresManualReview = true
		rr.Status.Message = ai.Status.Message
		now := metav1.Now()
		rr.Status.CompletedAt = &now

		// Reuse NoActionRequiredDelayHours for cooldown suppression
		if h.noActionRequiredDelay > 0 {
			nextAllowed := metav1.NewTime(time.Now().Add(h.noActionRequiredDelay))
			rr.Status.NextAllowedExecution = &nextAllowed
		}

		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)

		// BR-ORCH-043: Set Ready condition (terminal success - manual review required)
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonReady, "Manual review required", h.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for ManualReviewCompleted")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Record metric (reuses NoActionNeededTotal with reason=manual_review)
	if h.Metrics != nil {
		h.Metrics.NoActionNeededTotal.WithLabelValues("manual_review", rr.Namespace).Inc()
	}

	logger.Info("Remediation completed - manual review required",
		"outcome", "ManualReviewRequired",
		"notificationName", notifName,
		"nextAllowedExecution", rr.Status.NextAllowedExecution,
	)

	return ctrl.Result{}, nil
}

// handleWorkflowResolutionFailed processes AIAnalysis WorkflowResolutionFailed (BR-ORCH-036).
// This triggers a manual review notification.
func (h *AIAnalysisHandler) handleWorkflowResolutionFailed(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"subReason", ai.Status.SubReason,
	)

	logger.Info("AIAnalysis WorkflowResolutionFailed - creating manual review notification")

	reviewCtx := &creator.ManualReviewContext{
		Source:    notificationv1.ReviewSourceAIAnalysis,
		Reason:    string(ai.Status.Reason),
		SubReason: ai.Status.SubReason,
		Message:   ai.Status.Message,
	}

	h.populateManualReviewContext(reviewCtx, ai)

	return h.createManualReviewAndUpdateStatus(ctx, logger, rr, reviewCtx, string(ai.Status.Reason), ai.Status.SubReason)
}

// populateManualReviewContext adds root cause analysis and warnings to the review context.
// Common helper for handleHumanReviewRequired, handleManualReviewCompleted, and handleWorkflowResolutionFailed.
func (h *AIAnalysisHandler) populateManualReviewContext(reviewCtx *creator.ManualReviewContext, ai *aianalysisv1.AIAnalysis) {
	// Add root cause analysis if available
	if ai.Status.RootCauseAnalysis != nil {
		reviewCtx.RootCauseAnalysis = ai.Status.RootCauseAnalysis.Summary
	} else if ai.Status.RootCause != "" {
		reviewCtx.RootCauseAnalysis = ai.Status.RootCause
	}

	// Add warnings if available
	if ai.Status.Warnings != nil {
		reviewCtx.Warnings = ai.Status.Warnings
	}
}

// createManualReviewAndUpdateStatus creates a NotificationRequest and updates RemediationRequest status.
// Common helper for handleHumanReviewRequired and handleWorkflowResolutionFailed.
// Returns ctrl.Result and error suitable for returning from handler methods.
func (h *AIAnalysisHandler) createManualReviewAndUpdateStatus(
	ctx context.Context,
	logger logr.Logger,
	rr *remediationv1.RemediationRequest,
	reviewCtx *creator.ManualReviewContext,
	metricReason string,
	metricSubReason string,
) (ctrl.Result, error) {
	// Create manual review notification
	notifName, err := h.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
	if err != nil {
		logger.Error(err, "Failed to create manual review notification")
		return ctrl.Result{}, fmt.Errorf("failed to create manual review notification: %w", err)
	}

	// Update RR status with handler-specific fields (DD-GATEWAY-011, BR-ORCH-038)
	// Note: Phase transition and audit emission handled by transitionToFailed() callback below
	// REFACTOR-RO-001: Using retry helper
	ref := h.buildNotificationRef(ctx, notifName, rr.Namespace)
	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Handler-specific status fields
		rr.Status.Outcome = "ManualReviewRequired"
		rr.Status.RequiresManualReview = true

		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for manual review")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	logger.Info("Created manual review notification",
		"notificationName", notifName,
		"reason", metricReason,
		"subReason", metricSubReason,
	)

	// Transition to Failed phase with audit emission (BR-AUDIT-005, DD-AUDIT-003)
	// Handler Consistency Refactoring (2026-01-22): Delegate to reconciler's transitionToFailed
	return h.transitionToFailed(ctx, rr, remediationv1.FailurePhaseAIAnalysis, fmt.Errorf("AIAnalysis failed: %s", reviewCtx.Message))
}

// propagateFailure propagates AIAnalysis failure to RemediationRequest.
// BR-ORCH-036 v3.0: Creates an escalation notification for unrecoverable infrastructure failures
// (APIError, Timeout, MaxRetriesExceeded) before transitioning to Failed.
func (h *AIAnalysisHandler) propagateFailure(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"reason", ai.Status.Reason,
		"subReason", ai.Status.SubReason,
	)

	logger.Info("AIAnalysis infrastructure failure - creating escalation notification",
		"reason", ai.Status.Reason,
		"subReason", ai.Status.SubReason,
	)

	// BR-ORCH-036 v3.0: Build manual review context for infrastructure failures
	// (APIError, Timeout, MaxRetriesExceeded, etc.)
	reviewCtx := &creator.ManualReviewContext{
		Source:    notificationv1.ReviewSourceAIAnalysis,
		Reason:    string(ai.Status.Reason),
		SubReason: ai.Status.SubReason,
		Message:   ai.Status.Message,
	}

	h.populateManualReviewContext(reviewCtx, ai)

	return h.createManualReviewAndUpdateStatus(ctx, logger, rr, reviewCtx, string(ai.Status.Reason), ai.Status.SubReason)
}

// HandleRemediationTargetMissing handles the defense-in-depth case where AIAnalysis completed
// with a SelectedWorkflow but RemediationTarget is nil or has empty Kind/Name.
// This is the RO layer of the three-layer defense chain (HAPI -> AA -> RO) per DD-HAPI-006 v1.2
// and BR-ORCH-036 v4.0. Produces the same seamless response as handleHumanReviewRequired:
// Failed + ManualReviewRequired + NotificationRequest.
func (h *AIAnalysisHandler) HandleRemediationTargetMissing(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
	)

	logger.Info("RemediationTarget missing on completed AIAnalysis - creating manual review notification (DD-HAPI-006 defense-in-depth)")

	reviewCtx := &creator.ManualReviewContext{
		Source:    notificationv1.ReviewSourceAIAnalysis,
		Reason:    "RemediationTargetMissing",
		SubReason: "rca_resource_missing",
		Message:   "AIAnalysis completed with a selected workflow but the RCA remediation target is missing or empty. This indicates the AI identified a remediation action but could not determine the specific Kubernetes resource to target.",
	}

	h.populateManualReviewContext(reviewCtx, ai)

	return h.createManualReviewAndUpdateStatus(ctx, logger, rr, reviewCtx, "RemediationTargetMissing", "rca_resource_missing")
}

// IsWorkflowResolutionFailed checks if AIAnalysis failed due to workflow resolution issues.
// Helper function for use in reconciler.
func IsWorkflowResolutionFailed(ai *aianalysisv1.AIAnalysis) bool {
	return ai.Status.Phase == "Failed" && ai.Status.Reason == "WorkflowResolutionFailed"
}

// IsWorkflowNotNeeded checks if AIAnalysis determined no workflow is needed.
// Helper function for use in reconciler.
func IsWorkflowNotNeeded(ai *aianalysisv1.AIAnalysis) bool {
	return ai.Status.Phase == "Completed" && ai.Status.Reason == "WorkflowNotNeeded"
}

// RequiresManualReview checks if AIAnalysis requires manual review.
// Returns true for WorkflowResolutionFailed scenarios.
func RequiresManualReview(ai *aianalysisv1.AIAnalysis) bool {
	return IsWorkflowResolutionFailed(ai)
}
