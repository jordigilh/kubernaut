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
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
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
	spCreator           *creator.SignalProcessingCreator
	aiAnalysisCreator   *creator.AIAnalysisCreator
	weCreator           *creator.WorkflowExecutionCreator
	// Audit integration (DD-AUDIT-003, BR-STORAGE-001)
	auditStore   audit.AuditStore
	auditHelpers *roaudit.Helpers
}

// NewReconciler creates a new Reconciler with all dependencies.
// The auditStore parameter is optional - if nil, audit events will not be emitted.
func NewReconciler(c client.Client, s *runtime.Scheme, auditStore audit.AuditStore) *Reconciler {
	nc := creator.NewNotificationCreator(c, s)
	return &Reconciler{
		client:              c,
		scheme:              s,
		statusAggregator:    aggregator.NewStatusAggregator(c),
		aiAnalysisHandler:   handler.NewAIAnalysisHandler(c, s, nc),
		notificationCreator: nc,
		spCreator:           creator.NewSignalProcessingCreator(c, s),
		aiAnalysisCreator:   creator.NewAIAnalysisCreator(c, s),
		weCreator:           creator.NewWorkflowExecutionCreator(c, s),
		auditStore:          auditStore,
		auditHelpers:        roaudit.NewHelpers(roaudit.ServiceName),
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

	// Initialize phase if empty (new RemediationRequest from Gateway)
	// Per DD-GATEWAY-011: RO owns status.overallPhase, Gateway creates instances without status
	if rr.Status.OverallPhase == "" {
		logger.Info("Initializing new RemediationRequest", "name", rr.Name)
		rr.Status.OverallPhase = string(phase.Pending)
		rr.Status.StartTime = &metav1.Time{Time: startTime}
		if err := r.client.Status().Update(ctx, rr); err != nil {
			logger.Error(err, "Failed to initialize RemediationRequest status")
			return ctrl.Result{}, err
		}
		// Requeue immediately to process the Pending phase
		return ctrl.Result{Requeue: true}, nil
	}

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
	// TODO(BR-ORCH-042): Add case phase.Blocked handler here
	default:
		logger.Info("Unknown phase", "phase", rr.Status.OverallPhase)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// handlePendingPhase handles the initial Pending phase.
// Creates SignalProcessing CRD and transitions to Processing.
// Per DD-AUDIT-003: Emits orchestrator.lifecycle.started (P1)
// Per BR-ORCH-025: Pass-through data to SignalProcessing CRD.
// Per BR-ORCH-031: Sets owner reference for cascade deletion.
func (r *Reconciler) handlePendingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("Handling Pending phase - creating SignalProcessing")

	// Emit lifecycle started audit event (DD-AUDIT-003 P1)
	r.emitLifecycleStartedAudit(ctx, rr)

	// Create SignalProcessing CRD (BR-ORCH-025, BR-ORCH-031)
	spName, err := r.spCreator.Create(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to create SignalProcessing CRD")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	logger.Info("Created SignalProcessing CRD", "spName", spName)

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
// V1.0: Operator approves via `kubectl patch rar <name> --subresource=status -p '{"status":{"decision":"Approved"}}'`
// Audit trail: K8s audit log captures who made the patch.
// V1.1: Will add CEL validation requiring decidedBy when decision is set.
// Reference: ADR-040, BR-ORCH-026
func (r *Reconciler) handleAwaitingApprovalPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Check if RemediationApprovalRequest exists
	rarName := fmt.Sprintf("rar-%s", rr.Name)
	rar := &remediationv1.RemediationApprovalRequest{}
	err := r.client.Get(ctx, client.ObjectKey{Name: rarName, Namespace: rr.Namespace}, rar)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// RAR should have been created when transitioning to AwaitingApproval
			// This is unexpected - log warning and requeue
			logger.Info("RemediationApprovalRequest not found, will be created by approval handler")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		logger.Error(err, "Failed to get RemediationApprovalRequest")
		return ctrl.Result{}, err
	}

	// Check the decision
	switch rar.Status.Decision {
	case remediationv1.ApprovalDecisionApproved:
		logger.Info("Approval granted via RemediationApprovalRequest",
			"decidedBy", rar.Status.DecidedBy,
			"message", rar.Status.DecisionMessage,
		)
		return r.transitionPhase(ctx, rr, phase.Executing)

	case remediationv1.ApprovalDecisionRejected:
		logger.Info("Approval rejected via RemediationApprovalRequest",
			"decidedBy", rar.Status.DecidedBy,
			"message", rar.Status.DecisionMessage,
		)
		reason := "Rejected by operator"
		if rar.Status.DecisionMessage != "" {
			reason = rar.Status.DecisionMessage
		}
		return r.transitionToFailed(ctx, rr, "approval", reason)

	case remediationv1.ApprovalDecisionExpired:
		logger.Info("Approval expired (timeout)")
		return r.transitionToFailed(ctx, rr, "approval", "Approval request expired (timeout)")

	default:
		// Still pending - check if deadline passed (V1.0 timeout handling)
		if time.Now().After(rar.Spec.RequiredBy.Time) {
			logger.Info("Approval deadline passed, marking as expired")
			// Update RAR status to Expired (best effort)
			rar.Status.Decision = remediationv1.ApprovalDecisionExpired
			rar.Status.DecidedBy = "system"
			now := metav1.Now()
			rar.Status.DecidedAt = &now
			if updateErr := r.client.Status().Update(ctx, rar); updateErr != nil {
				logger.Error(updateErr, "Failed to update RAR status to Expired")
			}
			return r.transitionToFailed(ctx, rr, "approval", "Approval request expired (timeout)")
		}

		// Still waiting for approval
		logger.V(1).Info("Waiting for approval decision",
			"rarName", rarName,
			"requiredBy", rar.Spec.RequiredBy.Format(time.RFC3339),
		)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
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
	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	metrics.PhaseTransitionsTotal.WithLabelValues(oldPhase, string(newPhase), rr.Namespace).Inc()

	// Emit audit event (DD-AUDIT-003)
	r.emitPhaseTransitionAudit(ctx, rr, oldPhase, string(newPhase))

	logger.Info("Phase transition successful", "from", oldPhase, "to", newPhase)
	return ctrl.Result{Requeue: true}, nil
}

// transitionToCompleted transitions the RR to Completed phase.
func (r *Reconciler) transitionToCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	oldPhase := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

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

	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	metrics.PhaseTransitionsTotal.WithLabelValues(oldPhase, string(phase.Completed), rr.Namespace).Inc()

	// Emit audit event (DD-AUDIT-003)
	durationMs := time.Since(startTime).Milliseconds()
	r.emitCompletionAudit(ctx, rr, outcome, durationMs)

	logger.Info("Remediation completed successfully", "outcome", outcome)
	return ctrl.Result{}, nil
}

// transitionToFailed transitions the RR to Failed phase.
// TODO(BR-ORCH-042): Add consecutive failure blocking check here
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase, failureReason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	_ = logger // Suppress unused variable warning until blocking logic is implemented

	oldPhase := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

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

	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	metrics.PhaseTransitionsTotal.WithLabelValues(oldPhase, string(phase.Failed), rr.Namespace).Inc()

	// Emit audit event (DD-AUDIT-003)
	durationMs := time.Since(startTime).Milliseconds()
	r.emitFailureAudit(ctx, rr, failurePhase, failureReason, durationMs)

	logger.Info("Remediation failed", "failurePhase", failurePhase, "reason", failureReason)
	return ctrl.Result{}, nil
}

// ========================================
// AUDIT EVENT EMISSION (DD-AUDIT-003)
// ========================================

// emitLifecycleStartedAudit emits an audit event for remediation lifecycle started.
// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitLifecycleStartedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	if r.auditStore == nil {
		return // Audit disabled
	}

	logger := log.FromContext(ctx)
	correlationID := string(rr.UID)

	event, err := r.auditHelpers.BuildLifecycleStartedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
	)
	if err != nil {
		logger.Error(err, "Failed to build lifecycle started audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store lifecycle started audit event")
	}
}

// emitPhaseTransitionAudit emits an audit event for phase transitions.
// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitPhaseTransitionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, fromPhase, toPhase string) {
	if r.auditStore == nil {
		return // Audit disabled
	}

	logger := log.FromContext(ctx)
	correlationID := string(rr.UID)

	event, err := r.auditHelpers.BuildPhaseTransitionEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		fromPhase,
		toPhase,
	)
	if err != nil {
		logger.Error(err, "Failed to build phase transition audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store phase transition audit event")
	}
}

// emitCompletionAudit emits an audit event for remediation completion.
func (r *Reconciler) emitCompletionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string, durationMs int64) {
	if r.auditStore == nil {
		return
	}

	logger := log.FromContext(ctx)
	correlationID := string(rr.UID)

	event, err := r.auditHelpers.BuildCompletionEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		outcome,
		durationMs,
	)
	if err != nil {
		logger.Error(err, "Failed to build completion audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store completion audit event")
	}
}

// emitFailureAudit emits an audit event for remediation failure.
func (r *Reconciler) emitFailureAudit(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase, failureReason string, durationMs int64) {
	if r.auditStore == nil {
		return
	}

	logger := log.FromContext(ctx)
	correlationID := string(rr.UID)

	event, err := r.auditHelpers.BuildFailureEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		failurePhase,
		failureReason,
		durationMs,
	)
	if err != nil {
		logger.Error(err, "Failed to build failure audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store failure audit event")
	}
}

// SetupWithManager sets up the controller with the Manager.
// TODO(BR-ORCH-042): Add field index on spec.signalFingerprint for consecutive failure lookups
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationRequest{}).
		Complete(r)
}
