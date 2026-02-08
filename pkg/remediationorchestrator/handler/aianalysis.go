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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// AIAnalysisHandler handles AIAnalysis CRD status changes for the Remediation Orchestrator.
// Reference: BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
type AIAnalysisHandler struct {
	client              client.Client
	scheme              *runtime.Scheme
	notificationCreator *creator.NotificationCreator
	Metrics             *metrics.Metrics
	transitionToFailed  func(context.Context, *remediationv1.RemediationRequest, string, error) (ctrl.Result, error)
}

// NewAIAnalysisHandler creates a new AIAnalysisHandler.
func NewAIAnalysisHandler(c client.Client, s *runtime.Scheme, nc *creator.NotificationCreator, m *metrics.Metrics, ttf func(context.Context, *remediationv1.RemediationRequest, string, error) (ctrl.Result, error)) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		client:              c,
		scheme:              s,
		notificationCreator: nc,
		Metrics:             m,
		transitionToFailed:  ttf,
	}
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
	err := helpers.UpdateRemediationRequestStatus(ctx, h.client, h.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Update only RO-owned fields
		rr.Status.OverallPhase = remediationv1.PhaseCompleted
		rr.Status.Outcome = "NoActionRequired"
		rr.Status.Message = ai.Status.Message
		now := metav1.Now()
		rr.Status.CompletedAt = &now
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

	logger.Info("Remediation completed - no action required",
		"outcome", "NoActionRequired",
		"reason", reason,
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
	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, h.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs,
			corev1.ObjectReference{
				Kind:      "NotificationRequest",
				Name:      notifName,
				Namespace: rr.Namespace,
			})
		rr.Status.ApprovalNotificationSent = true
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status with approval notification ref")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Record metric
	if h.Metrics != nil {
		h.Metrics.ApprovalNotificationsTotal.WithLabelValues(rr.Namespace).Inc()
	}

	logger.Info("Created approval notification", "notificationName", notifName)
	return ctrl.Result{}, nil
}

// handleFailed processes AIAnalysis Failed phase.
// Handles NeedsHumanReview (BR-HAPI-197), WorkflowResolutionFailed (BR-ORCH-036), and other failure scenarios.
func (h *AIAnalysisHandler) handleFailed(
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

	// BR-HAPI-197: Check NeedsHumanReview FIRST (takes precedence over WorkflowResolutionFailed)
	// This flag is set by HAPI when AI cannot produce a reliable result
	if ai.Status.NeedsHumanReview {
		return h.handleHumanReviewRequired(ctx, rr, ai)
	}

	// BR-ORCH-036: Handle WorkflowResolutionFailed
	if ai.Status.Reason == "WorkflowResolutionFailed" {
		return h.handleWorkflowResolutionFailed(ctx, rr, ai)
	}

	// Other failures (APIError, Timeout, etc.) - propagate failure to RR
	logger.Info("AIAnalysis failed with non-recoverable error")
	return h.propagateFailure(ctx, rr, ai)
}

// handleHumanReviewRequired processes AIAnalysis when HAPI explicitly requires human review (BR-HAPI-197).
// This triggers a manual review notification with the specific HumanReviewReason from HAPI.
func (h *AIAnalysisHandler) handleHumanReviewRequired(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
		"humanReviewReason", ai.Status.HumanReviewReason,
	)

	logger.Info("AIAnalysis requires human review - creating manual review notification")

	// Build manual review context with BR-HAPI-197 fields
	reviewCtx := &creator.ManualReviewContext{
		Source:            creator.ManualReviewSourceAIAnalysis,
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

	// Build manual review context
	reviewCtx := &creator.ManualReviewContext{
		Source:    creator.ManualReviewSourceAIAnalysis,
		Reason:    ai.Status.Reason,
		SubReason: ai.Status.SubReason,
		Message:   ai.Status.Message,
	}

	// Populate root cause and warnings (common pattern)
	h.populateManualReviewContext(reviewCtx, ai)

	// Create notification and update RR status (common pattern)
	return h.createManualReviewAndUpdateStatus(ctx, logger, rr, reviewCtx, ai.Status.Reason, ai.Status.SubReason)
}

// populateManualReviewContext adds root cause analysis and warnings to the review context.
// Common helper for handleHumanReviewRequired and handleWorkflowResolutionFailed.
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
	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, h.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Handler-specific status fields
		rr.Status.Outcome = "ManualReviewRequired"
		rr.Status.RequiresManualReview = true

		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs,
			corev1.ObjectReference{
				Kind:      "NotificationRequest",
				Name:      notifName,
				Namespace: rr.Namespace,
			})
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for manual review")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Record metric (BR-ORCH-036, BR-HAPI-197)
	if h.Metrics != nil {
		h.Metrics.ManualReviewNotificationsTotal.WithLabelValues(
			string(creator.ManualReviewSourceAIAnalysis),
			metricReason,
			metricSubReason,
			rr.Namespace,
		).Inc()
	}

	logger.Info("Created manual review notification",
		"notificationName", notifName,
		"reason", metricReason,
		"subReason", metricSubReason,
	)

	// Transition to Failed phase with audit emission (BR-AUDIT-005, DD-AUDIT-003)
	// Handler Consistency Refactoring (2026-01-22): Delegate to reconciler's transitionToFailed
	return h.transitionToFailed(ctx, rr, "ai_analysis", fmt.Errorf("%s", reviewCtx.Message))
}

// propagateFailure propagates AIAnalysis failure to RemediationRequest.
func (h *AIAnalysisHandler) propagateFailure(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"aiAnalysis", ai.Name,
	)

	// Prepare failure reason with comprehensive tracking
	failureReason := fmt.Sprintf("AIAnalysis failed: %s: %s", ai.Status.Reason, ai.Status.Message)

	// Update RR message field (DD-GATEWAY-011)
	// Note: Phase transition and audit emission handled by transitionToFailed() callback below
	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, h.client, h.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.Message = failureReason
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR message for AIAnalysis failure")
		// Continue to transitionToFailed even if message update fails
	}

	logger.Info("Propagating AIAnalysis failure to RemediationRequest",
		"reason", ai.Status.Reason,
	)

	// Transition to Failed phase with audit emission (BR-AUDIT-005, DD-AUDIT-003)
	// Handler Consistency Refactoring (2026-01-22): Delegate to reconciler's transitionToFailed
	return h.transitionToFailed(ctx, rr, "ai_analysis", fmt.Errorf("%s", failureReason))
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
