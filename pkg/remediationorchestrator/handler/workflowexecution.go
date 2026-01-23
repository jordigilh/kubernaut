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
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// WorkflowExecutionHandler handles WorkflowExecution CRD status updates.
// This handler encapsulates all WorkflowExecution-specific logic for consistency with other service handlers.
//
// Reference:
// - BR-ORCH-025: WorkflowExecution orchestration
// - BR-ORCH-032: Failure handling
// - DD-PERF-001: Atomic status updates with conditions
// - Handler Consistency Refactoring (2026-01-22): Extract inline logic for maintainability
//
// Historical Note:
// Previous implementation had HandleSkipped() and HandleFailed() methods that were never called by the reconciler.
// This refactored version extracts the actual inline logic from handleExecutingPhase() into a dedicated handler
// for consistency with AIAnalysisHandler and SignalProcessingHandler patterns.
type WorkflowExecutionHandler struct {
	client                client.Client
	scheme                *runtime.Scheme
	metrics               *metrics.Metrics
	transitionToFailed    func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error)
	transitionToCompleted func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error)
}

// NewWorkflowExecutionHandler creates a new WorkflowExecutionHandler.
//
// Parameters:
// - c: Kubernetes client for API operations
// - s: Scheme for runtime type information
// - m: Metrics for observability (DD-METRICS-001)
// - ttf: Callback to reconciler's transitionToFailed() for audit emission
// - ttc: Callback to reconciler's transitionToCompleted() for audit emission
//
// The transition callbacks allow the handler to trigger terminal state transitions
// while preserving the reconciler's responsibility for audit event emission (DD-AUDIT-003).
func NewWorkflowExecutionHandler(
	c client.Client,
	s *runtime.Scheme,
	m *metrics.Metrics,
	ttf func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error),
	ttc func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error),
) *WorkflowExecutionHandler {
	return &WorkflowExecutionHandler{
		client:                c,
		scheme:                s,
		metrics:               m,
		transitionToFailed:    ttf,
		transitionToCompleted: ttc,
	}
}

// HandleStatus processes WorkflowExecution CRD status updates and triggers appropriate RemediationRequest transitions.
//
// Flow:
// 1. Check WorkflowExecution phase
// 2. If Completed → set WorkflowExecutionComplete condition + transition RR to Completed
// 3. If Failed → set WorkflowExecutionComplete condition (false) + transition RR to Failed
// 4. If empty phase → handle as missing/deleted WE (error case)
// 5. If Pending/Running → requeue and wait
//
// Reference:
// - BR-ORCH-025: Executing phase waits for WorkflowExecution completion
// - DD-PERF-001: Atomic status updates with conditions
// - DD-AUDIT-003: Terminal state transitions emit audit events via callbacks
//
// Returns:
// - ctrl.Result: Requeue configuration
// - error: Nil on success, error on terminal state transitions
func (h *WorkflowExecutionHandler) HandleStatus(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"wePhase", we.Status.Phase,
	)

	switch we.Status.Phase {
	case workflowexecutionv1.PhaseCompleted:
		logger.Info("WorkflowExecution completed, transitioning to Completed")

		// Note: WorkflowExecutionComplete condition is set by the reconciler
		// via DD-PERF-001 atomic status update before calling transitionToCompleted.
		// This handler focuses on phase transition logic only.

		// Delegate to reconciler's transitionToCompleted() for audit emission (DD-AUDIT-003)
		return h.transitionToCompleted(ctx, rr, "Remediated") // CRD enum: Remediated, NoActionRequired, ManualReviewRequired

	case workflowexecutionv1.PhaseFailed:
		logger.Info("WorkflowExecution failed, transitioning to Failed")

		// Note: WorkflowExecutionComplete condition (false) is set by the reconciler
		// via DD-PERF-001 atomic status update before calling transitionToFailed.
		// This handler focuses on phase transition logic only.

		// Delegate to reconciler's transitionToFailed() for audit emission (DD-AUDIT-003)
		return h.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution failed")

	case "":
		// Empty phase means WE was just created but controller hasn't set phase yet
		// Requeue to check status again after controller processes it
		logger.V(1).Info("WorkflowExecution has empty phase, waiting for controller to process")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	case workflowexecutionv1.PhasePending, workflowexecutionv1.PhaseRunning:
		// Still in progress, requeue to check status again
		logger.V(1).Info("WorkflowExecution still in progress, requeuing")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	default:
		// Unknown phase - log warning and requeue
		// WorkflowExecution controller might have introduced a new phase
		logger.Info("WorkflowExecution has unknown phase, requeuing",
			"phase", we.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

