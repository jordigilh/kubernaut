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
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// ========================================
// RESOURCE BUSY HANDLER (REFACTOR-RO-002)
// Business Requirement: BR-ORCH-032, BR-ORCH-033
// ========================================
//
// ResourceBusyHandler handles the "ResourceBusy" skip reason.
// This occurs when another WorkflowExecution is already running for the same resource.
//
// BEHAVIOR:
// - Marks RR as Skipped (duplicate)
// - Tracks parent RR via DuplicateOf field
// - Requeues after 30 seconds for retry
//
// WHY 30 seconds?
// - Short enough to retry quickly after resource becomes available
// - Long enough to avoid excessive reconciliation load
//
// Reference: BR-ORCH-032 (handle WE Skipped phase), BR-ORCH-033 (track duplicates)
type ResourceBusyHandler struct {
	ctx *Context
}

// NewResourceBusyHandler creates a new ResourceBusyHandler.
func NewResourceBusyHandler(ctx *Context) *ResourceBusyHandler {
	return &ResourceBusyHandler{ctx: ctx}
}

// Handle processes the ResourceBusy skip reason.
// Reference: BR-ORCH-032, BR-ORCH-033
func (h *ResourceBusyHandler) Handle(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", "ResourceBusy",
	)

	logger.Info("WE skipped: ResourceBusy - tracking as duplicate, requeueing")

	// ========================================
	// V1.0 TODO: HANDLER DEPRECATED (DD-RO-002)
	// This handler is part of the OLD routing flow (WE skips → reports to RO).
	// In V1.0, RO makes routing decisions BEFORE creating WFE, so WFE never skips.
	// This handler will be REMOVED in Days 2-3 when new routing logic is implemented.
	// ========================================

	// Temporary stub for Day 1 build compatibility
	// V1.0: WE.Status.SkipDetails removed (DD-RO-002)
	// This code path will not execute in V1.0 (WFE never created if should be skipped)
	err := helpers.UpdateRemediationRequestStatus(ctx, h.ctx.Client, h.ctx.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseSkipped
		rr.Status.SkipReason = "ResourceBusy"
		// V1.0: SkipDetails removed, skip information now in RR.Status
		// rr.Status.DuplicateOf would be set by RO routing logic before WFE creation

		// BR-ORCH-043: Set Ready condition (terminal skip - resource busy)
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonReady, "Skipped: resource busy", h.ctx.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for ResourceBusy")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// REFACTOR-RO-003: Using centralized timeout constant
	return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
}
