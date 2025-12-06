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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecutionHandler handles WE status changes for Remediation Orchestrator.
// Reference: BR-ORCH-032 (skip handling), BR-ORCH-033 (duplicate tracking),
//            BR-ORCH-036 (manual review notification), DD-WE-004 (exponential backoff)
type WorkflowExecutionHandler struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkflowExecutionHandler creates a new WorkflowExecutionHandler.
func NewWorkflowExecutionHandler(c client.Client, s *runtime.Scheme) *WorkflowExecutionHandler {
	return &WorkflowExecutionHandler{
		client: c,
		scheme: s,
	}
}

// HandleSkipped handles WE Skipped phase per DD-WE-004 and BR-ORCH-032.
// Reference: BR-ORCH-032 (skip handling), BR-ORCH-033 (duplicate tracking), BR-ORCH-036 (manual review)
func (h *WorkflowExecutionHandler) HandleSkipped(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", we.Status.SkipDetails.Reason,
	)

	reason := we.Status.SkipDetails.Reason

	switch reason {
	case "ResourceBusy":
		// DUPLICATE: Another workflow running - requeue
		logger.Info("WE skipped: ResourceBusy - tracking as duplicate, requeueing")
		rr.Status.OverallPhase = "Skipped"
		rr.Status.SkipReason = reason
		if we.Status.SkipDetails.ConflictingWorkflow != nil {
			rr.Status.DuplicateOf = we.Status.SkipDetails.ConflictingWorkflow.Name
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case "RecentlyRemediated":
		// DUPLICATE: Cooldown active - requeue with fixed interval
		// Per WE Team Response Q6: RO should NOT calculate backoff, let WE re-evaluate
		logger.Info("WE skipped: RecentlyRemediated - tracking as duplicate, requeueing")
		rr.Status.OverallPhase = "Skipped"
		rr.Status.SkipReason = reason
		if we.Status.SkipDetails.RecentRemediation != nil {
			rr.Status.DuplicateOf = we.Status.SkipDetails.RecentRemediation.Name
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil

	case "ExhaustedRetries":
		// NOT A DUPLICATE: Manual review required
		logger.Info("WE skipped: ExhaustedRetries - manual intervention required")
		return h.handleManualReviewRequired(ctx, rr, we, sp, reason,
			"Retry limit exceeded - 5+ consecutive pre-execution failures")

	case "PreviousExecutionFailed":
		// NOT A DUPLICATE: Manual review required (cluster state unknown)
		logger.Info("WE skipped: PreviousExecutionFailed - manual intervention required")
		return h.handleManualReviewRequired(ctx, rr, we, sp, reason,
			"Previous execution failed during workflow run - cluster state may be inconsistent")

	default:
		logger.Error(nil, "Unknown skip reason", "reason", reason)
		return ctrl.Result{}, fmt.Errorf("unknown skip reason: %s", reason)
	}
}

// handleManualReviewRequired handles skip reasons requiring manual intervention.
// Reference: BR-ORCH-032 v1.1, BR-ORCH-036
func (h *WorkflowExecutionHandler) handleManualReviewRequired(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
	skipReason string,
	message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update RR status - FAILED, not Skipped (per BR-ORCH-032 v1.1)
	rr.Status.OverallPhase = "Failed"
	rr.Status.SkipReason = skipReason
	rr.Status.RequiresManualReview = true
	rr.Status.DuplicateOf = "" // NOT a duplicate
	rr.Status.Message = we.Status.SkipDetails.Message

	logger.Info("Manual review required",
		"skipReason", skipReason,
		"message", message,
	)

	// TODO: Create manual review notification (BR-ORCH-036)
	// This will be implemented in a subsequent test

	// NO requeue - manual intervention required
	return ctrl.Result{}, nil
}

// MapSkipReasonToSeverity maps skip reason to severity label per Notification team guidance.
// PreviousExecutionFailed = critical (cluster state unknown)
// ExhaustedRetries = high (infrastructure issue, but state is known)
// Reference: BR-ORCH-036
func (h *WorkflowExecutionHandler) MapSkipReasonToSeverity(skipReason string) string {
	switch skipReason {
	case "PreviousExecutionFailed":
		return "critical"
	case "ExhaustedRetries":
		return "high"
	default:
		return "medium"
	}
}

// MapSkipReasonToPriority maps skip reason to NotificationPriority per Notification team guidance.
// Reference: BR-ORCH-036
func (h *WorkflowExecutionHandler) MapSkipReasonToPriority(skipReason string) string {
	switch skipReason {
	case "PreviousExecutionFailed":
		return "critical"
	case "ExhaustedRetries":
		return "high"
	default:
		return "medium"
	}
}

