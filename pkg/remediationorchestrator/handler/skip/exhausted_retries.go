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
// EXHAUSTED RETRIES HANDLER (REFACTOR-RO-002)
// Business Requirement: BR-ORCH-032, BR-ORCH-036
// ========================================
//
// ExhaustedRetriesHandler handles the "ExhaustedRetries" skip reason.
// This occurs when WE has attempted 5+ consecutive pre-execution failures.
//
// BEHAVIOR:
// - Marks RR as Failed (NOT Skipped - this is terminal)
// - Sets RequiresManualReview = true
// - Creates manual review notification
// - Does NOT requeue (terminal state)
//
// WHY Terminal?
// - 5+ failures indicate systematic issue (not transient)
// - Manual intervention required to diagnose root cause
// - Auto-retry would waste resources without fixing underlying problem
//
// Reference: BR-ORCH-032 (handle WE Skipped phase), BR-ORCH-036 (manual review notifications)
type ExhaustedRetriesHandler struct {
	ctx *Context
}

// NewExhaustedRetriesHandler creates a new ExhaustedRetriesHandler.
func NewExhaustedRetriesHandler(ctx *Context) *ExhaustedRetriesHandler {
	return &ExhaustedRetriesHandler{ctx: ctx}
}

// Handle processes the ExhaustedRetries skip reason.
// Reference: BR-ORCH-032, BR-ORCH-036
func (h *ExhaustedRetriesHandler) Handle(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", "ExhaustedRetries",
	)

	logger.Info("WE skipped: ExhaustedRetries - manual intervention required")

	// Prepare failure reason
	failureReason := "Retry limit exceeded - 5+ consecutive pre-execution failures"

	// Update RR status with handler-specific fields (BR-ORCH-032, BR-ORCH-036)
	// Note: Phase transition and audit emission handled by TransitionToFailedFunc callback below
	err := helpers.UpdateRemediationRequestStatus(ctx, h.ctx.Client, h.ctx.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.SkipReason = "ExhaustedRetries"
		rr.Status.RequiresManualReview = true
		rr.Status.Message = failureReason
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for ExhaustedRetries")
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
