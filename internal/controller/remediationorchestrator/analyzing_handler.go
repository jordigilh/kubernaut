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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// TargetRef is a minimal struct to avoid importing routing or AI types.
type TargetRef struct {
	Kind      string
	Name      string
	Namespace string
}

// DualTargetResult holds the signal and remediation targets.
type DualTargetResult struct {
	Remediation TargetRef
}

// AnalyzingCallbacks provides reconciler methods needed by the AnalyzingHandler.
//
// Reference: Issue #666, TP-666-v1 §8.4
type AnalyzingCallbacks struct {
	AtomicStatusUpdate             func(ctx context.Context, rr *remediationv1.RemediationRequest, fn func() error) error
	IsWorkflowNotNeeded            func(ai *aianalysisv1.AIAnalysis) bool
	HandleWorkflowNotNeeded        func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (ctrl.Result, error)
	CreateApproval                 func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error)
	HandleAIAnalysisStatus         func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (ctrl.Result, error)
	HandleRemediationTargetMissing func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (ctrl.Result, error)
	EmitApprovalRequestedAudit     func(ctx context.Context, rr *remediationv1.RemediationRequest, confidence float64, workflowID string)
	RecordEvent                    func(rr *remediationv1.RemediationRequest, eventType string, reason string, message string)
	FetchFreshRR                   func(ctx context.Context, key client.ObjectKey) (*remediationv1.RemediationRequest, error)
	CheckPostAnalysisConditions    func(ctx context.Context, rr *remediationv1.RemediationRequest, workflowID, targetResource, preHash, actionType string) (*routing.BlockingCondition, error)
	HandleBlocked                  func(ctx context.Context, rr *remediationv1.RemediationRequest, bc *routing.BlockingCondition, fromPhase, workflowID string) (ctrl.Result, error)
	AcquireLock                    func(ctx context.Context, target string) (bool, error)
	ReleaseLock                    func(ctx context.Context, target string) error
	CapturePreRemediationHash      func(ctx context.Context, kind, name, namespace string) (hash string, degradedReason string, err error)
	ResolveDualTargets             func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult
	PersistPreHash                 func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error
	IsDryRun                       func() bool // #712, #736: returns true when dry-run mode is enabled
	WFECallbacks                   WFECreationCallbacks
}

// AnalyzingHandler encapsulates the reconcile logic for the Analyzing phase.
//
// Internalizes logic from reconciler.handleAnalyzingPhase (reconciler.go).
//
// Reference: Issue #666, TP-666-v1 §8.4, BR-ORCH-036, BR-ORCH-037
type AnalyzingHandler struct {
	k8sClient client.Client
	m         *metrics.Metrics
	callbacks AnalyzingCallbacks
}

func NewAnalyzingHandler(
	k8sClient client.Client,
	m *metrics.Metrics,
	callbacks AnalyzingCallbacks,
) *AnalyzingHandler {
	return &AnalyzingHandler{
		k8sClient: k8sClient,
		m:         m,
		callbacks: callbacks,
	}
}

func (h *AnalyzingHandler) Phase() phase.Phase {
	return phase.Analyzing
}

func (h *AnalyzingHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.AIAnalysisRef == nil {
		logger.V(1).Info("AIAnalysis not created yet, waiting")
		return phase.Requeue(config.RequeueGenericError, "AI ref not set"), nil
	}

	ai := &aianalysisv1.AIAnalysis{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef.Name,
		Namespace: rr.Status.AIAnalysisRef.Namespace,
	}, ai)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AIAnalysis CRD not found, waiting for creation")
			return phase.Requeue(config.RequeueGenericError, "AI not found"), nil
		}
		logger.Error(err, "Failed to fetch AIAnalysis CRD")
		return phase.TransitionIntent{}, err
	}

	switch ai.Status.Phase {
	case "Completed":
		return h.handleCompleted(ctx, rr, ai)
	case "Failed":
		return h.handleFailed(ctx, rr, ai)
	case "Pending", "Investigating", "Analyzing":
		logger.V(1).Info("AIAnalysis in progress", "phase", ai.Status.Phase)
		return phase.Requeue(10*time.Second, "AI in progress"), nil
	default:
		logger.Info("Unknown AIAnalysis phase", "phase", ai.Status.Phase)
		return phase.Requeue(10*time.Second, "AI unknown phase"), nil
	}
}

func (h *AnalyzingHandler) handleCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Set AIAnalysisComplete condition (best-effort)
	if err := h.callbacks.AtomicStatusUpdate(ctx, rr, func() error {
		remediationrequest.SetAIAnalysisComplete(rr, true,
			remediationrequest.ReasonAIAnalysisSucceeded,
			"AIAnalysis completed successfully", h.m)
		return nil
	}); err != nil {
		logger.Error(err, "Failed to update AIAnalysisComplete condition")
	}

	if h.callbacks.IsWorkflowNotNeeded(ai) {
		logger.Info("AIAnalysis: WorkflowNotNeeded - delegating to handler")
		result, err := h.callbacks.HandleWorkflowNotNeeded(ctx, rr, ai)
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "workflowNotNeeded"), nil
	}

	// #805 / BR-ORCH-036: NeedsHumanReview with no workflow → ManualReview NR
	if ai.Status.NeedsHumanReview && ai.Status.SelectedWorkflow == nil {
		logger.Info("AIAnalysis completed with NeedsHumanReview (no workflow) - delegating to handler")
		result, err := h.callbacks.HandleAIAnalysisStatus(ctx, rr, ai)
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "needsHumanReview"), nil
	}

	// #712, #736: Dry-run intercept — stop pipeline before creating WFE or RAR
	if h.callbacks.IsDryRun() {
		logger.Info("Dry-run mode: completing without execution or verification")
		return phase.CompleteWithoutVerification("dry-run mode enabled"), nil
	}

	if ai.Status.ApprovalRequired {
		return h.handleApprovalRequired(ctx, rr, ai)
	}

	return h.handleDirectExecution(ctx, rr, ai)
}

func (h *AnalyzingHandler) handleApprovalRequired(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	rarName, err := h.callbacks.CreateApproval(ctx, rr, ai)
	if err != nil {
		logger.Error(err, "Failed to create RemediationApprovalRequest")
		return phase.Requeue(config.RequeueGenericError, "RAR creation failed"), nil
	}
	logger.Info("Created RemediationApprovalRequest", "rarName", rarName)

	if ai.Status.SelectedWorkflow != nil {
		h.callbacks.RecordEvent(rr, "Normal", "ApprovalRequired",
			fmt.Sprintf("Human approval required (confidence %.0f%%): %s",
				ai.Status.SelectedWorkflow.Confidence*100, ai.Status.ApprovalReason))
	}

	result, err := h.callbacks.HandleAIAnalysisStatus(ctx, rr, ai)
	if err != nil {
		return phase.TransitionIntent{}, err
	}
	_ = result

	oldPhase := rr.Status.OverallPhase

	intent := phase.Advance(phase.AwaitingApproval, "approval required")

	if oldPhase != phase.AwaitingApproval && ai.Status.SelectedWorkflow != nil {
		h.callbacks.EmitApprovalRequestedAudit(ctx, rr, ai.Status.SelectedWorkflow.Confidence, ai.Status.SelectedWorkflow.WorkflowID)
	}

	return intent, nil
}

func (h *AnalyzingHandler) handleDirectExecution(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Stale-cache guard: refetch RR from API server
	freshRR, err := h.callbacks.FetchFreshRR(ctx, client.ObjectKeyFromObject(rr))
	if err == nil {
		if freshRR.Status.OverallPhase != phase.Analyzing {
			logger.Info("Phase already advanced past Analyzing (stale cache), no-op",
				"freshPhase", freshRR.Status.OverallPhase)
			return phase.NoOp("stale cache"), nil
		}
		if freshRR.Status.WorkflowExecutionRef != nil {
			logger.Info("WFE already created but phase still Analyzing, completing transition",
				"wfeName", freshRR.Status.WorkflowExecutionRef.Name)
			return phase.Advance(phase.Executing, "WFE already exists"), nil
		}
	}

	// Validate RemediationTarget
	if ai.Status.RootCauseAnalysis == nil ||
		ai.Status.RootCauseAnalysis.RemediationTarget == nil ||
		ai.Status.RootCauseAnalysis.RemediationTarget.Kind == "" ||
		ai.Status.RootCauseAnalysis.RemediationTarget.Name == "" {
		logger.Error(fmt.Errorf("RCA RemediationTarget missing on completed AIAnalysis"),
			"Failing RR with ManualReviewRequired per DD-HAPI-006 v1.2 / BR-ORCH-036 v4.0",
			"aianalysis", ai.Name)
		h.callbacks.RecordEvent(rr, "Warning", "EscalatedToManualReview",
			"RemediationTarget missing on completed AIAnalysis - manual investigation required")
		result, err := h.callbacks.HandleRemediationTargetMissing(ctx, rr, ai)
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "remediationTargetMissing"), nil
	}

	// Capture pre-remediation hash
	var workflowID, actionType string
	if ai.Status.SelectedWorkflow != nil {
		workflowID = ai.Status.SelectedWorkflow.WorkflowID
		actionType = ai.Status.SelectedWorkflow.ActionType
	}
	targetResource := formatRemediationTargetString(ai)
	remTarget := h.callbacks.ResolveDualTargets(rr, ai).Remediation

	preHash, degradedReason, hashErr := h.callbacks.CapturePreRemediationHash(
		ctx, remTarget.Kind, remTarget.Name, remTarget.Namespace)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to capture pre-remediation hash (terminal)")
		if updateErr := helpers.UpdateRemediationRequestStatus(ctx, h.k8sClient, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.OverallPhase = remediationv1.PhaseFailed
			reason := fmt.Sprintf("Cannot determine target resource state: %v", hashErr)
			rr.Status.FailureReason = &reason
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update RR to Failed after hash error")
			return phase.TransitionIntent{}, updateErr
		}
		return phase.Fail(remediationv1.FailurePhaseAIAnalysis, hashErr, "pre-hash computation failed"), nil
	}
	if degradedReason != "" {
		logger.Info("Pre-remediation hash capture degraded, EA may be non-functional",
			"degradedReason", degradedReason)
		h.callbacks.RecordEvent(rr, "Warning", "HashCaptureDegraded",
			fmt.Sprintf("Pre-remediation hash unavailable for %s/%s: %s", remTarget.Kind, remTarget.Name, degradedReason))
		remediationrequest.SetPreRemediationHashCaptured(rr, false, degradedReason, h.m)
	}
	if preHash != "" && rr.Status.PreRemediationSpecHash == "" {
		if err := h.callbacks.PersistPreHash(ctx, rr, preHash); err != nil {
			logger.Error(err, "Failed to persist pre-remediation hash on RR status (non-fatal)")
		}
	}

	// Lock acquisition
	acquired, lockErr := h.callbacks.AcquireLock(ctx, targetResource)
	if lockErr != nil {
		logger.Error(lockErr, "Distributed lock acquisition failed", "target", targetResource)
		return phase.Requeue(config.RequeueGenericError, "lock acquisition failed"), nil
	}
	if !acquired {
		logger.V(1).Info("Lock contention on target resource, requeuing", "target", targetResource)
		return phase.Requeue(5*time.Second, "lock contention"), nil
	}
	defer func() {
		if releaseErr := h.callbacks.ReleaseLock(ctx, targetResource); releaseErr != nil {
			logger.Error(releaseErr, "Failed to release distributed lock", "target", targetResource)
		}
	}()

	// Routing checks
	blocked, err := h.callbacks.CheckPostAnalysisConditions(ctx, rr, workflowID, targetResource, preHash, actionType)
	if err != nil {
		logger.Error(err, "Failed to check routing conditions")
		return phase.Requeue(config.RequeueGenericError, "routing check failed"), nil
	}
	if blocked != nil {
		logger.Info("Routing blocked - will not create WorkflowExecution",
			"reason", blocked.Reason, "message", blocked.Message)
		result, err := h.callbacks.HandleBlocked(ctx, rr, blocked, string(remediationv1.PhaseAnalyzing), workflowID)
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "routing blocked"), nil
	}

	logger.Info("Routing checks passed, creating WorkflowExecution")
	return CreateWFEAndTransition(ctx, h.k8sClient, h.m, rr, ai, preHash, h.callbacks.WFECallbacks)
}

func (h *AnalyzingHandler) handleFailed(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Set AIAnalysisComplete=False condition (best-effort)
	if err := h.callbacks.AtomicStatusUpdate(ctx, rr, func() error {
		remediationrequest.SetAIAnalysisComplete(rr, false,
			remediationrequest.ReasonAIAnalysisFailed,
			"AIAnalysis failed", h.m)
		return nil
	}); err != nil {
		logger.Error(err, "Failed to update AIAnalysisComplete condition")
	}

	if ai.Status.NeedsHumanReview {
		h.callbacks.RecordEvent(rr, "Warning", "EscalatedToManualReview",
			fmt.Sprintf("AI analysis requires manual review: %s", ai.Status.Message))
	}

	logger.Info("AIAnalysis failed - delegating to handler")
	result, err := h.callbacks.HandleAIAnalysisStatus(ctx, rr, ai)
	if err != nil {
		return phase.TransitionIntent{}, err
	}
	return resultToIntent(result, "AI failed"), nil
}
