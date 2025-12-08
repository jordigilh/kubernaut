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

// Package controller provides the Kubernetes controller for RemediationRequest CRDs.
//
// Business Requirements:
// - BR-ORCH-025: Phase state transitions
// - BR-ORCH-026: Status aggregation from child CRDs
// - BR-ORCH-027: Global timeout handling
// - BR-ORCH-028: Per-phase timeout handling
// - BR-ORCH-001: Approval notification creation
// - BR-ORCH-036: Manual review notification creation
// - BR-ORCH-037: WorkflowNotNeeded handling
// - BR-ORCH-038: Preserve Gateway deduplication data
package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// Reconciler reconciles RemediationRequest objects.
type Reconciler struct {
	client              client.Client
	scheme              *runtime.Scheme
	statusAggregator    *aggregator.StatusAggregator
	aiAnalysisHandler   *handler.AIAnalysisHandler
	notificationCreator *creator.NotificationCreator
	aiAnalysisCreator   *creator.AIAnalysisCreator
	weCreator           *creator.WorkflowExecutionCreator
}

// NewReconciler creates a new Reconciler with all dependencies.
func NewReconciler(c client.Client, s *runtime.Scheme) *Reconciler {
	nc := creator.NewNotificationCreator(c, s)
	return &Reconciler{
		client:              c,
		scheme:              s,
		statusAggregator:    aggregator.NewStatusAggregator(c),
		aiAnalysisHandler:   handler.NewAIAnalysisHandler(c, s, nc),
		notificationCreator: nc,
		aiAnalysisCreator:   creator.NewAIAnalysisCreator(c, s),
		weCreator:           creator.NewWorkflowExecutionCreator(c, s),
	}
}

// Reconcile implements the reconciliation loop for RemediationRequest.
// It handles phase transitions and delegates to appropriate handlers.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", req.NamespacedName)
	startTime := time.Now()

	// Fetch the RemediationRequest
	rr := &remediationv1.RemediationRequest{}
	if err := r.client.Get(ctx, req.NamespacedName, rr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("RemediationRequest not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch RemediationRequest")
		return ctrl.Result{}, err
	}

	// Record reconcile duration on exit
	defer func() {
		metrics.ReconcileDurationSeconds.WithLabelValues(
			rr.Namespace,
			rr.Status.OverallPhase,
		).Observe(time.Since(startTime).Seconds())
		metrics.ReconcileTotal.WithLabelValues(rr.Namespace, rr.Status.OverallPhase).Inc()
	}()

	// Skip terminal phases
	if phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
		logger.V(1).Info("RemediationRequest in terminal phase, skipping", "phase", rr.Status.OverallPhase)
		return ctrl.Result{}, nil
	}

	// Aggregate status from child CRDs
	aggregatedStatus, err := r.statusAggregator.AggregateStatus(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to aggregate status")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle based on current phase
	switch phase.Phase(rr.Status.OverallPhase) {
	case phase.Pending:
		return r.handlePendingPhase(ctx, rr)
	case phase.Processing:
		return r.handleProcessingPhase(ctx, rr, aggregatedStatus)
	case phase.Analyzing:
		return r.handleAnalyzingPhase(ctx, rr, aggregatedStatus)
	case phase.AwaitingApproval:
		return r.handleAwaitingApprovalPhase(ctx, rr)
	case phase.Executing:
		return r.handleExecutingPhase(ctx, rr, aggregatedStatus)
	default:
		logger.Info("Unknown phase", "phase", rr.Status.OverallPhase)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// handlePendingPhase handles the initial Pending phase.
// Creates SignalProcessing CRD and transitions to Processing.
func (r *Reconciler) handlePendingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("Handling Pending phase - creating SignalProcessing")

	// Transition to Processing phase
	return r.transitionPhase(ctx, rr, phase.Processing)
}

// handleProcessingPhase handles the Processing phase.
// Waits for SignalProcessing to complete, then creates AIAnalysis.
func (r *Reconciler) handleProcessingPhase(ctx context.Context, rr *remediationv1.RemediationRequest, agg *aggregator.AggregatedStatus) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "spPhase", agg.SignalProcessingPhase)

	switch agg.SignalProcessingPhase {
	case "Completed":
		logger.Info("SignalProcessing completed, transitioning to Analyzing")
		return r.transitionPhase(ctx, rr, phase.Analyzing)
	case "Failed":
		logger.Info("SignalProcessing failed, transitioning to Failed")
		return r.transitionToFailed(ctx, rr, "signal_processing", "SignalProcessing failed")
	case "":
		// SignalProcessing not created yet, requeue
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	default:
		// Still in progress
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// handleAnalyzingPhase handles the Analyzing phase.
// Waits for AIAnalysis to complete, then handles the result.
// Reference: BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
func (r *Reconciler) handleAnalyzingPhase(ctx context.Context, rr *remediationv1.RemediationRequest, agg *aggregator.AggregatedStatus) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "aiPhase", agg.AIAnalysisPhase)

	// Check if AIAnalysis exists
	if rr.Status.AIAnalysisRef == nil {
		logger.V(1).Info("AIAnalysis not created yet, waiting")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Fetch the AIAnalysis CRD for detailed status
	ai := &aianalysisv1.AIAnalysis{}
	err := r.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef.Name,
		Namespace: rr.Status.AIAnalysisRef.Namespace,
	}, ai)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AIAnalysis CRD not found, waiting for creation")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		logger.Error(err, "Failed to fetch AIAnalysis CRD")
		return ctrl.Result{}, err
	}

	// Delegate to AIAnalysisHandler for Completed/Failed phases
	// This handles BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
	switch ai.Status.Phase {
	case "Completed":
		// Check for WorkflowNotNeeded (BR-ORCH-037)
		if handler.IsWorkflowNotNeeded(ai) {
			logger.Info("AIAnalysis: WorkflowNotNeeded - delegating to handler")
			return r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)
		}

		// Check for approval required
		if ai.Status.ApprovalRequired {
			logger.Info("AIAnalysis completed with approval required")
			result, err := r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)
			if err != nil {
				return result, err
			}
			// Transition to AwaitingApproval
			return r.transitionPhase(ctx, rr, phase.AwaitingApproval)
		}

		// Normal completion - transition to Executing
		logger.Info("AIAnalysis completed, transitioning to Executing")
		return r.transitionPhase(ctx, rr, phase.Executing)

	case "Failed":
		// Handle all failure scenarios (BR-ORCH-036: manual review)
		logger.Info("AIAnalysis failed - delegating to handler")
		return r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)

	case "Pending", "Investigating", "Analyzing":
		// Still in progress
		logger.V(1).Info("AIAnalysis in progress", "phase", ai.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	default:
		logger.Info("Unknown AIAnalysis phase", "phase", ai.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// handleAwaitingApprovalPhase handles the AwaitingApproval phase.
// Waits for human approval before proceeding.
// Approval is granted via annotation: kubernaut.ai/approved=true
func (r *Reconciler) handleAwaitingApprovalPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Check if approval has been granted via annotation
	// Operators set this annotation to approve the remediation
	if rr.Annotations != nil && rr.Annotations["kubernaut.ai/approved"] == "true" {
		logger.Info("Approval granted via annotation, transitioning to Executing")
		return r.transitionPhase(ctx, rr, phase.Executing)
	}

	// Still waiting for approval
	logger.V(1).Info("Waiting for approval (set annotation kubernaut.ai/approved=true)")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleExecutingPhase handles the Executing phase.
// Waits for WorkflowExecution to complete.
func (r *Reconciler) handleExecutingPhase(ctx context.Context, rr *remediationv1.RemediationRequest, agg *aggregator.AggregatedStatus) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "wePhase", agg.WorkflowExecutionPhase)

	switch agg.WorkflowExecutionPhase {
	case "Completed":
		logger.Info("WorkflowExecution completed, transitioning to Completed")
		return r.transitionToCompleted(ctx, rr, "Remediated")
	case "Failed":
		logger.Info("WorkflowExecution failed, transitioning to Failed")
		return r.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution failed")
	case "":
		// WorkflowExecution not created yet, requeue
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	default:
		// Still in progress
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// transitionPhase transitions the RR to a new phase.
// Uses retry.RetryOnConflict to handle concurrent updates (BR-ORCH-038).
func (r *Reconciler) transitionPhase(ctx context.Context, rr *remediationv1.RemediationRequest, newPhase phase.Phase) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "newPhase", newPhase)

	oldPhase := rr.Status.OverallPhase

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Refetch to get latest resourceVersion (preserves Gateway fields per DD-GATEWAY-011)
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		// Update only RO-owned fields
		rr.Status.OverallPhase = string(newPhase)
		now := metav1.Now()

		// Set phase start times
		switch newPhase {
		case phase.Processing:
			rr.Status.ProcessingStartTime = &now
		case phase.Analyzing:
			rr.Status.AnalyzingStartTime = &now
		case phase.Executing:
			rr.Status.ExecutingStartTime = &now
		}

		return r.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to transition phase")
		return ctrl.Result{}, fmt.Errorf("failed to transition phase: %w", err)
	}

	// Record metric
	metrics.PhaseTransitionsTotal.WithLabelValues(rr.Namespace, oldPhase, string(newPhase)).Inc()

	logger.Info("Phase transition successful", "from", oldPhase, "to", newPhase)
	return ctrl.Result{Requeue: true}, nil
}

// transitionToCompleted transitions the RR to Completed phase.
func (r *Reconciler) transitionToCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		rr.Status.OverallPhase = string(phase.Completed)
		rr.Status.Outcome = outcome
		now := metav1.Now()
		rr.Status.CompletedAt = &now

		return r.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Completed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Completed: %w", err)
	}

	metrics.PhaseTransitionsTotal.WithLabelValues(rr.Namespace, rr.Status.OverallPhase, string(phase.Completed)).Inc()

	logger.Info("Remediation completed successfully", "outcome", outcome)
	return ctrl.Result{}, nil
}

// transitionToFailed transitions the RR to Failed phase.
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase, failureReason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return err
		}

		rr.Status.OverallPhase = string(phase.Failed)
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &failureReason

		return r.client.Status().Update(ctx, rr)
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Failed: %w", err)
	}

	metrics.PhaseTransitionsTotal.WithLabelValues(rr.Namespace, rr.Status.OverallPhase, string(phase.Failed)).Inc()

	logger.Info("Remediation failed", "failurePhase", failurePhase, "reason", failureReason)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationRequest{}).
		Complete(r)
}
