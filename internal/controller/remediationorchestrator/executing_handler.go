/*
Copyright 2026 Jordi Gil.

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

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

const executingRequeueInterval = 10 * time.Second

// ExecutingHandler encapsulates the reconcile logic for the Executing phase.
// It monitors the WorkflowExecution CRD and returns a TransitionIntent based on its status.
//
// Internalizes logic from:
//   - reconciler.handleExecutingPhase
//   - reconciler.handleDedupResultPropagation
//   - handler.WorkflowExecutionHandler.HandleStatus
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type ExecutingHandler struct {
	client        client.Client
	apiReader     client.Reader
	aggregator    *aggregator.StatusAggregator
	statusManager *status.Manager
	metrics       *metrics.Metrics
}

// NewExecutingHandler creates a new ExecutingHandler with all required dependencies.
func NewExecutingHandler(
	c client.Client,
	apiReader client.Reader,
	agg *aggregator.StatusAggregator,
	sm *status.Manager,
	m *metrics.Metrics,
) *ExecutingHandler {
	return &ExecutingHandler{
		client:        c,
		apiReader:     apiReader,
		aggregator:    agg,
		statusManager: sm,
		metrics:       m,
	}
}

// Phase returns the phase this handler manages.
func (h *ExecutingHandler) Phase() phase.Phase {
	return phase.Executing
}

// Handle processes a RemediationRequest in the Executing phase.
// It monitors the associated WorkflowExecution CRD and returns a
// TransitionIntent based on its status.
func (h *ExecutingHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.WorkflowExecutionRef == nil {
		logger.Error(nil, "Executing phase but no WorkflowExecutionRef - corrupted state")
		return phase.Fail(remediationv1.FailurePhaseWorkflowExecution,
			fmt.Errorf("WorkflowExecution not found"),
			"corrupted state: no WorkflowExecutionRef"), nil
	}

	agg, err := h.aggregator.AggregateStatus(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to aggregate status")
		return phase.Requeue(config.RequeueGenericError, "aggregate status failed"), nil
	}

	if agg.WorkflowExecutionPhase == "" && !agg.AllChildrenHealthy {
		logger.Error(nil, "WorkflowExecution CRD not found",
			"workflowExecutionRef", rr.Status.WorkflowExecutionRef.Name)
		return phase.Fail(remediationv1.FailurePhaseWorkflowExecution,
			fmt.Errorf("WorkflowExecution not found"),
			"child CRD missing"), nil
	}

	we := &workflowexecutionv1.WorkflowExecution{}
	if err := h.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.WorkflowExecutionRef.Name,
		Namespace: rr.Status.WorkflowExecutionRef.Namespace,
	}, we); err != nil {
		logger.Error(err, "Failed to fetch WorkflowExecution CRD")
		return phase.Fail(remediationv1.FailurePhaseWorkflowExecution,
			fmt.Errorf("WorkflowExecution not found: %w", err),
			"failed to fetch WE CRD"), nil
	}

	if rr.Status.DeduplicatedByWE != "" {
		return h.handleDedupResultPropagation(ctx, rr)
	}

	h.setWorkflowConditions(ctx, rr, we)

	return h.handleWEStatus(ctx, rr, we)
}

// handleWEStatus translates WE status into a TransitionIntent.
// Internalizes logic from handler.WorkflowExecutionHandler.HandleStatus.
func (h *ExecutingHandler) handleWEStatus(ctx context.Context, rr *remediationv1.RemediationRequest, we *workflowexecutionv1.WorkflowExecution) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"wePhase", we.Status.Phase,
	)

	switch we.Status.Phase {
	case workflowexecutionv1.PhaseCompleted:
		logger.Info("WorkflowExecution completed, transitioning to Verifying")
		return phase.Verify("Remediated", "WorkflowExecution completed successfully"), nil

	case workflowexecutionv1.PhaseFailed:
		if we.Status.FailureDetails != nil &&
			we.Status.FailureDetails.Reason == workflowexecutionv1.FailureReasonDeduplicated {
			logger.Info("WorkflowExecution failed as Deduplicated, setting DeduplicatedByWE",
				"originalWFE", we.Status.DeduplicatedBy)

			rr.Status.DeduplicatedByWE = we.Status.DeduplicatedBy
			if err := h.client.Status().Update(ctx, rr); err != nil {
				return phase.TransitionIntent{}, fmt.Errorf("failed to set DeduplicatedByWE: %w", err)
			}
			return phase.Requeue(executingRequeueInterval, "WFE deduplicated, waiting for original"), nil
		}

		logger.Info("WorkflowExecution failed, transitioning to Failed")
		return phase.Fail(remediationv1.FailurePhaseWorkflowExecution,
			fmt.Errorf("WorkflowExecution failed"),
			"WorkflowExecution failed"), nil

	case "":
		logger.V(1).Info("WorkflowExecution has empty phase, waiting for controller to process")
		return phase.Requeue(executingRequeueInterval, "WE phase empty, waiting for controller"), nil

	case workflowexecutionv1.PhasePending, workflowexecutionv1.PhaseRunning:
		logger.V(1).Info("WorkflowExecution still in progress, requeuing")
		return phase.Requeue(executingRequeueInterval, "WE in progress"), nil

	default:
		logger.Info("WorkflowExecution has unknown phase, requeuing", "phase", we.Status.Phase)
		return phase.Requeue(executingRequeueInterval, "WE unknown phase"), nil
	}
}

// handleDedupResultPropagation fetches the original WFE referenced by DeduplicatedByWE
// and returns a TransitionIntent to propagate its terminal result.
func (h *ExecutingHandler) handleDedupResultPropagation(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"originalWFE", rr.Status.DeduplicatedByWE,
	)

	originalWFE := &workflowexecutionv1.WorkflowExecution{}
	err := h.apiReader.Get(ctx, client.ObjectKey{
		Name:      rr.Status.DeduplicatedByWE,
		Namespace: rr.Namespace,
	}, originalWFE)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(nil, "Original WFE deleted (dangling DeduplicatedByWE reference)")
			return phase.InheritFail(
				fmt.Errorf("original WorkflowExecution %q deleted before completion", rr.Status.DeduplicatedByWE),
				rr.Status.DeduplicatedByWE, "WorkflowExecution",
				"original WFE deleted"), nil
		}
		return phase.TransitionIntent{}, fmt.Errorf("failed to fetch original WFE %q: %w", rr.Status.DeduplicatedByWE, err)
	}

	switch originalWFE.Status.Phase {
	case workflowexecutionv1.PhaseCompleted:
		return phase.InheritComplete(rr.Status.DeduplicatedByWE, "WorkflowExecution",
			"original WFE completed"), nil
	case workflowexecutionv1.PhaseFailed:
		return phase.InheritFail(
			fmt.Errorf("original WorkflowExecution %q failed: %s", originalWFE.Name, originalWFE.Status.FailureReason),
			rr.Status.DeduplicatedByWE, "WorkflowExecution",
			"original WFE failed"), nil
	default:
		logger.Info("Original WFE still in progress, requeuing",
			"phase", originalWFE.Status.Phase)
		return phase.Requeue(executingRequeueInterval, "original WFE in progress"), nil
	}
}

// setWorkflowConditions sets the WorkflowExecutionComplete condition (best-effort).
func (h *ExecutingHandler) setWorkflowConditions(ctx context.Context, rr *remediationv1.RemediationRequest, we *workflowexecutionv1.WorkflowExecution) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	switch we.Status.Phase {
	case workflowexecutionv1.PhaseCompleted:
		if err := h.statusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetWorkflowExecutionComplete(rr, true,
				remediationrequest.ReasonWorkflowSucceeded,
				"WorkflowExecution completed successfully", h.metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update WorkflowExecutionComplete condition")
		}
	case workflowexecutionv1.PhaseFailed:
		if err := h.statusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetWorkflowExecutionComplete(rr, false,
				remediationrequest.ReasonWorkflowFailed,
				"WorkflowExecution failed", h.metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update WorkflowExecutionComplete condition")
		}
	}
}
