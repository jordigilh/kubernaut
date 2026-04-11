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
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// AwaitingApprovalCallbacks provides the reconciler methods needed by AwaitingApprovalHandler.
//
// Reference: Issue #666, TP-666-v1 §8.5
type AwaitingApprovalCallbacks struct {
	RecordEvent        func(rr *remediationv1.RemediationRequest, eventType, reason, message string)
	UpdateRARConditions func(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, decision string) error
	ResolveWorkflow    func(ctx context.Context, wo *remediationv1.WorkflowOverride, sw *aianalysisv1.SelectedWorkflow, ns string) (*aianalysisv1.SelectedWorkflow, bool, error)
	CheckResourceBusy  func(ctx context.Context, rr *remediationv1.RemediationRequest, targetResource string) (*routing.BlockingCondition, error)
	HandleBlocked      func(ctx context.Context, rr *remediationv1.RemediationRequest, bc *routing.BlockingCondition, fromPhase, workflowID string) (ctrl.Result, error)
	AcquireLock        func(ctx context.Context, target string) (bool, error)
	ReleaseLock        func(ctx context.Context, target string) error
	CapturePreRemediationHash func(ctx context.Context, kind, name, namespace string) (hash string, degradedReason string, err error)
	ResolveDualTargets func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult
	PersistPreHash     func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error
	TransitionToFailed func(ctx context.Context, rr *remediationv1.RemediationRequest, fp remediationv1.FailurePhase, err error) (ctrl.Result, error)
	ExpireRAR          func(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error
	UpdateRARTimeRemaining func(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error
	WFECallbacks       WFECreationCallbacks
}

// AwaitingApprovalHandler encapsulates the reconcile logic for the AwaitingApproval phase.
//
// Internalizes logic from reconciler.handleAwaitingApprovalPhase (reconciler.go).
//
// Reference: Issue #666, TP-666-v1 §8.5, ADR-040, BR-ORCH-026
type AwaitingApprovalHandler struct {
	k8sClient client.Client
	m         *metrics.Metrics
	callbacks AwaitingApprovalCallbacks
}

func NewAwaitingApprovalHandler(
	k8sClient client.Client,
	m *metrics.Metrics,
	callbacks AwaitingApprovalCallbacks,
) *AwaitingApprovalHandler {
	return &AwaitingApprovalHandler{
		k8sClient: k8sClient,
		m:         m,
		callbacks: callbacks,
	}
}

func (h *AwaitingApprovalHandler) Phase() phase.Phase {
	return phase.AwaitingApproval
}

func (h *AwaitingApprovalHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	rarName := fmt.Sprintf("rar-%s", rr.Name)
	rar := &remediationv1.RemediationApprovalRequest{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{Name: rarName, Namespace: rr.Namespace}, rar)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("RemediationApprovalRequest not found, will be created by approval handler")
			return phase.Requeue(config.RequeueGenericError, "RAR not found"), nil
		}
		logger.Error(err, "Failed to get RemediationApprovalRequest")
		return phase.TransitionIntent{}, err
	}

	switch rar.Status.Decision {
	case remediationv1.ApprovalDecisionApproved:
		return h.handleApproved(ctx, rr, rar)
	case remediationv1.ApprovalDecisionRejected:
		return h.handleRejected(ctx, rr, rar)
	case remediationv1.ApprovalDecisionExpired:
		return h.handleExpired(ctx, rr)
	default:
		return h.handlePending(ctx, rr, rar)
	}
}

func (h *AwaitingApprovalHandler) handleApproved(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	logger.Info("Approval granted via RemediationApprovalRequest",
		"decidedBy", rar.Status.DecidedBy)

	h.callbacks.RecordEvent(rr, "Normal", "ApprovalGranted",
		fmt.Sprintf("Approval granted by %s", rar.Status.DecidedBy))

	if err := h.callbacks.UpdateRARConditions(ctx, rr, rar, "approved"); err != nil {
		logger.Error(err, "Failed to update RAR conditions")
	}

	if rr.Status.AIAnalysisRef == nil {
		logger.Error(nil, "AIAnalysisRef not set on RemediationRequest (ADR-040 invariant)")
		return phase.Requeue(config.RequeueGenericError, "AIAnalysisRef nil"), nil
	}

	ai := &aianalysisv1.AIAnalysis{}
	err := h.k8sClient.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef.Name,
		Namespace: rr.Status.AIAnalysisRef.Namespace,
	}, ai)
	if err != nil {
		logger.Error(err, "Failed to fetch AIAnalysis CRD")
		return phase.Requeue(config.RequeueGenericError, "AI fetch failed"), nil
	}

	resolvedWorkflow, overrideApplied, resolveErr := h.callbacks.ResolveWorkflow(
		ctx, rar.Status.WorkflowOverride, ai.Status.SelectedWorkflow, rr.Namespace)
	if resolveErr != nil {
		if override.IsOverrideNotFoundError(resolveErr) {
			logger.Error(resolveErr, "Override workflow not found, failing RR")
			h.callbacks.RecordEvent(rr, "Warning", "RemediationFailed",
				fmt.Sprintf("Override workflow not found: %v", resolveErr))
			result, err := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, resolveErr)
			if err != nil {
				return phase.TransitionIntent{}, err
			}
			return resultToIntent(result, "override not found"), nil
		}
		logger.Error(resolveErr, "Failed to resolve operator workflow override")
		return phase.Requeue(config.RequeueGenericError, "override resolution failed"), nil
	}

	resolvedAI := ai.DeepCopy()
	if overrideApplied {
		resolvedAI.Status.SelectedWorkflow = resolvedWorkflow
		h.callbacks.RecordEvent(rr, "Normal", "OperatorOverride",
			fmt.Sprintf("Operator override applied: workflow=%s", rar.Status.WorkflowOverride.WorkflowName))
		h.m.OverrideAppliedTotal.WithLabelValues(classifyOverride(rar.Status.WorkflowOverride), rr.Namespace).Inc()
	}

	remTarget := h.callbacks.ResolveDualTargets(rr, ai).Remediation
	preHash, degradedReason, hashErr := h.callbacks.CapturePreRemediationHash(
		ctx, remTarget.Kind, remTarget.Name, remTarget.Namespace)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to capture pre-remediation hash after approval (non-fatal)")
	}
	if degradedReason != "" {
		logger.Info("Pre-remediation hash capture degraded after approval",
			"degradedReason", degradedReason)
		h.callbacks.RecordEvent(rr, "Warning", "HashCaptureDegraded",
			fmt.Sprintf("Pre-remediation hash unavailable for %s/%s: %s", remTarget.Kind, remTarget.Name, degradedReason))
		remediationrequest.SetPreRemediationHashCaptured(rr, false, degradedReason, h.m)
	}
	if preHash != "" && rr.Status.PreRemediationSpecHash == "" {
		if err := h.callbacks.PersistPreHash(ctx, rr, preHash); err != nil {
			logger.Error(err, "Failed to persist pre-remediation hash (non-fatal)")
		}
	}

	approvalTargetResource := formatRemediationTargetString(ai)
	if approvalTargetResource != "" {
		acquired, lockErr := h.callbacks.AcquireLock(ctx, approvalTargetResource)
		if lockErr != nil {
			logger.Error(lockErr, "Distributed lock acquisition failed (approval path)")
			return phase.Requeue(config.RequeueGenericError, "lock acquisition failed"), nil
		}
		if !acquired {
			logger.V(1).Info("Lock contention on target resource (approval path), requeuing")
			return phase.Requeue(5*time.Second, "lock contention"), nil
		}
		defer func() {
			if releaseErr := h.callbacks.ReleaseLock(ctx, approvalTargetResource); releaseErr != nil {
				logger.Error(releaseErr, "Failed to release distributed lock (approval path)")
			}
		}()

		busyBlock, busyErr := h.callbacks.CheckResourceBusy(ctx, rr, approvalTargetResource)
		if busyErr != nil {
			logger.Error(busyErr, "Failed to check resource busy (approval path)")
			return phase.Requeue(config.RequeueGenericError, "resource busy check failed"), nil
		}
		if busyBlock != nil {
			logger.Info("Target resource busy after approval - blocking")
			result, err := h.callbacks.HandleBlocked(ctx, rr, busyBlock, string(remediationv1.PhaseAwaitingApproval), "")
			if err != nil {
				return phase.TransitionIntent{}, err
			}
			return resultToIntent(result, "resource busy"), nil
		}
	}

	return CreateWFEAndTransition(ctx, h.k8sClient, h.m, rr, resolvedAI, preHash, h.callbacks.WFECallbacks)
}

func (h *AwaitingApprovalHandler) handleRejected(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	logger.Info("Approval rejected via RemediationApprovalRequest",
		"decidedBy", rar.Status.DecidedBy)

	h.callbacks.RecordEvent(rr, "Warning", "ApprovalRejected",
		fmt.Sprintf("Approval rejected by %s: %s", rar.Status.DecidedBy, rar.Status.DecisionMessage))

	if err := h.callbacks.UpdateRARConditions(ctx, rr, rar, "rejected"); err != nil {
		logger.Error(err, "Failed to update RAR conditions")
	}

	reason := "Rejected by operator"
	if rar.Status.DecisionMessage != "" {
		reason = rar.Status.DecisionMessage
	}
	result, err := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, fmt.Errorf("%s", reason))
	if err != nil {
		return phase.TransitionIntent{}, err
	}
	return resultToIntent(result, "approval rejected"), nil
}

func (h *AwaitingApprovalHandler) handleExpired(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("Approval expired (timeout)")

	h.callbacks.RecordEvent(rr, "Warning", "ApprovalExpired",
		"Approval request expired without a decision")

	result, err := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, fmt.Errorf("approval request expired (timeout)"))
	if err != nil {
		return phase.TransitionIntent{}, err
	}
	return resultToIntent(result, "approval expired"), nil
}

func (h *AwaitingApprovalHandler) handlePending(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if time.Now().After(rar.Spec.RequiredBy.Time) {
		logger.Info("Approval deadline passed, marking as expired")
		h.callbacks.RecordEvent(rr, "Warning", "ApprovalExpired",
			fmt.Sprintf("Approval deadline passed after %v without decision",
				time.Since(rar.ObjectMeta.CreationTimestamp.Time).Round(time.Minute)))

		if err := h.callbacks.ExpireRAR(ctx, rar); err != nil {
			logger.Error(err, "Failed to update RAR status to Expired")
		}

		result, err := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, fmt.Errorf("approval request expired (timeout)"))
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "deadline passed"), nil
	}

	if err := h.callbacks.UpdateRARTimeRemaining(ctx, rar); err != nil {
		logger.Error(err, "Failed to update RAR TimeRemaining (non-fatal)")
	}

	logger.V(1).Info("Waiting for approval decision", "rarName", rar.Name)
	return phase.Requeue(config.RequeueResourceBusy, "awaiting approval"), nil
}

func classifyOverride(wo *remediationv1.WorkflowOverride) string {
	if wo == nil {
		return "none"
	}
	if wo.WorkflowName != "" && wo.Parameters != nil {
		return "both"
	}
	if wo.WorkflowName != "" {
		return "workflow"
	}
	return "params"
}
