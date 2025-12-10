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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// AIAnalysisHandler handles AIAnalysis CRD status changes for the Remediation Orchestrator.
// Reference: BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
type AIAnalysisHandler struct {
	client              client.Client
	scheme              *runtime.Scheme
	notificationCreator *creator.NotificationCreator
}

// NewAIAnalysisHandler creates a new AIAnalysisHandler.
func NewAIAnalysisHandler(c client.Client, s *runtime.Scheme, nc *creator.NotificationCreator) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		client:              c,
		scheme:              s,
		notificationCreator: nc,
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

	// Update RR status using retry to preserve Gateway fields (DD-GATEWAY-011, BR-ORCH-038)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Refetch to get latest resourceVersion
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Update only RO-owned fields
		rr.Status.OverallPhase = "Completed"
		rr.Status.Outcome = "NoActionRequired"
		rr.Status.Message = ai.Status.Message
		now := metav1.Now()
		rr.Status.CompletedAt = &now

		return h.client.Status().Update(ctx, rr)
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
	metrics.NoActionNeededTotal.WithLabelValues(reason, rr.Namespace).Inc()

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

	// Update RR status with notification reference using retry (DD-GATEWAY-011, BR-ORCH-038)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs,
			corev1.ObjectReference{
				Kind:      "NotificationRequest",
				Name:      notifName,
				Namespace: rr.Namespace,
			})
		rr.Status.ApprovalNotificationSent = true

		return h.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status with approval notification ref")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Record metric
	metrics.ApprovalNotificationsTotal.WithLabelValues(rr.Namespace).Inc()

	logger.Info("Created approval notification", "notificationName", notifName)
	return ctrl.Result{}, nil
}

// handleFailed processes AIAnalysis Failed phase.
// Handles WorkflowResolutionFailed and other failure scenarios (BR-ORCH-036).
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

	// BR-ORCH-036: Handle WorkflowResolutionFailed
	if ai.Status.Reason == "WorkflowResolutionFailed" {
		return h.handleWorkflowResolutionFailed(ctx, rr, ai)
	}

	// Other failures (APIError, Timeout, etc.) - propagate failure to RR
	logger.Info("AIAnalysis failed with non-recoverable error")
	return h.propagateFailure(ctx, rr, ai)
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

	// Create manual review notification
	notifName, err := h.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
	if err != nil {
		logger.Error(err, "Failed to create manual review notification")
		return ctrl.Result{}, fmt.Errorf("failed to create manual review notification: %w", err)
	}

	// Update RR status using retry (DD-GATEWAY-011, BR-ORCH-038)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Update failure tracking
		rr.Status.OverallPhase = "Failed"
		rr.Status.Outcome = "ManualReviewRequired"
		failurePhase := "ai_analysis"
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &ai.Status.Message
		rr.Status.RequiresManualReview = true

		// Track notification reference (BR-ORCH-035)
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs,
			corev1.ObjectReference{
				Kind:      "NotificationRequest",
				Name:      notifName,
				Namespace: rr.Namespace,
			})

		return h.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for WorkflowResolutionFailed")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Record metric (BR-ORCH-036)
	metrics.ManualReviewNotificationsTotal.WithLabelValues(
		string(creator.ManualReviewSourceAIAnalysis),
		ai.Status.Reason,
		ai.Status.SubReason,
		rr.Namespace,
	).Inc()

	logger.Info("Created manual review notification for WorkflowResolutionFailed",
		"notificationName", notifName,
		"subReason", ai.Status.SubReason,
	)

	return ctrl.Result{}, nil
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

	// Update RR status using retry (DD-GATEWAY-011, BR-ORCH-038)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		rr.Status.OverallPhase = "Failed"
		failurePhase := "ai_analysis"
		rr.Status.FailurePhase = &failurePhase
		failureReason := fmt.Sprintf("%s: %s", ai.Status.Reason, ai.Status.Message)
		rr.Status.FailureReason = &failureReason

		return h.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to propagate AIAnalysis failure to RR")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	logger.Info("Propagated AIAnalysis failure to RemediationRequest",
		"reason", ai.Status.Reason,
	)

	return ctrl.Result{}, nil
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


