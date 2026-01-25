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

package skip

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
)

// ========================================
// PREVIOUS EXECUTION FAILED HANDLER (REFACTOR-RO-002)
// Business Requirement: BR-ORCH-032, BR-ORCH-036
// ========================================
//
// PreviousExecutionFailedHandler handles the "PreviousExecutionFailed" skip reason.
// This occurs when a previous WE failed during execution (not pre-execution).
//
// BEHAVIOR:
// - Marks RR as Failed (NOT Skipped - this is terminal)
// - Sets RequiresManualReview = true
// - Creates manual review notification with CRITICAL severity
// - Does NOT requeue (terminal state)
//
// WHY Terminal?
// - Execution failure means cluster state may be inconsistent
// - Manual verification required before retry
// - Auto-retry could worsen cluster state
// - CRITICAL severity because cluster integrity is at risk
//
// Reference: BR-ORCH-032 (handle WE Skipped phase), BR-ORCH-036 (manual review notifications)
type PreviousExecutionFailedHandler struct {
	ctx *Context
}

// NewPreviousExecutionFailedHandler creates a new PreviousExecutionFailedHandler.
func NewPreviousExecutionFailedHandler(ctx *Context) *PreviousExecutionFailedHandler {
	return &PreviousExecutionFailedHandler{ctx: ctx}
}

// Handle processes the PreviousExecutionFailed skip reason.
// Reference: BR-ORCH-032, BR-ORCH-036
func (h *PreviousExecutionFailedHandler) Handle(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", "PreviousExecutionFailed",
	)

	logger.Info("WE skipped: PreviousExecutionFailed - manual intervention required")

	// Prepare failure reason
	failureReason := "Previous execution failed during workflow run - cluster state may be inconsistent"

	// Update RR status with handler-specific fields (BR-ORCH-032, BR-ORCH-036)
	// Note: Phase transition and audit emission handled by TransitionToFailedFunc callback below
	err := helpers.UpdateRemediationRequestStatus(ctx, h.ctx.Client, h.ctx.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.SkipReason = "PreviousExecutionFailed"
		rr.Status.RequiresManualReview = true
		rr.Status.Message = failureReason
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for PreviousExecutionFailed")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// Create manual review notification (BR-ORCH-036)
	_, err = h.ctx.NotificationCreator.CreateManualReviewNotification(
		ctx,
		rr,
		we,
		sp,
	)
	if err != nil {
		logger.Error(err, "Failed to create manual review notification")
		// Continue even if notification fails - don't block the skip handling
	}

	// Transition to Failed phase with audit emission (BR-AUDIT-005, DD-AUDIT-003)
	// Handler Consistency Refactoring (2026-01-22): Delegate to reconciler's transitionToFailed
	return h.ctx.TransitionToFailedFunc(ctx, rr, "workflow_execution", failureReason)
}
