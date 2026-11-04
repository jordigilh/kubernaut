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
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedclock "github.com/jordigilh/kubernaut/pkg/clock"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
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
	RecordEvent               func(rr *remediationv1.RemediationRequest, eventType, reason, message string)
	UpdateRARConditions       func(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, decision string) error
	ResolveWorkflow           func(ctx context.Context, wo *remediationv1.WorkflowOverride, sw *aianalysisv1.SelectedWorkflow, ns string) (*aianalysisv1.SelectedWorkflow, bool, error)
	CheckResourceBusy         func(ctx context.Context, rr *remediationv1.RemediationRequest, targetResource string) (*routing.BlockingCondition, error)
	HandleBlocked             func(ctx context.Context, rr *remediationv1.RemediationRequest, bc *routing.BlockingCondition, fromPhase, workflowID string) (ctrl.Result, error)
	AcquireLock               func(ctx context.Context, target string) (bool, error)
	ReleaseLock               func(ctx context.Context, target string) error
	CapturePreRemediationHash func(ctx context.Context, kind, name, namespace, clusterID string) (hash string, degradedReason string, err error)
	ResolveDualTargets        func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult
	PersistPreHash            func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error
	TransitionToFailed        func(ctx context.Context, rr *remediationv1.RemediationRequest, fp remediationv1.FailurePhase, err error) (ctrl.Result, error)
	ExpireRAR                 func(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) (ExpiryOutcome, error)
	UpdateRARTimeRemaining    func(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error
	WFECallbacks              WFECreationCallbacks
}

// ExpiryOutcome is the authoritative result of the RO's pending-to-expired
// compare-and-confirm operation.
type ExpiryOutcome string

const (
	ExpiredConfirmed    ExpiryOutcome = "ExpiredConfirmed"
	DecisionWon         ExpiryOutcome = "DecisionWon"
	Unavailable         ExpiryOutcome = "Unavailable"
	systemDecisionActor               = "system"
)

// AwaitingApprovalHandler encapsulates the reconcile logic for the AwaitingApproval phase.
//
// Internalizes logic from reconciler.handleAwaitingApprovalPhase (reconciler.go).
//
// Reference: Issue #666, TP-666-v1 §8.5, ADR-040, BR-ORCH-026
type AwaitingApprovalHandler struct {
	k8sClient client.Client
	apiReader client.Reader
	m         *metrics.Metrics
	callbacks AwaitingApprovalCallbacks
	clock     sharedclock.Clock
}

func NewAwaitingApprovalHandler(
	k8sClient client.Client,
	m *metrics.Metrics,
	callbacks AwaitingApprovalCallbacks,
) *AwaitingApprovalHandler {
	return &AwaitingApprovalHandler{
		k8sClient: k8sClient,
		apiReader: k8sClient,
		m:         m,
		callbacks: callbacks,
		clock:     sharedclock.RealClock{},
	}
}

// SetAPIReader supplies the cache-bypassing reader used to confirm absence.
func (h *AwaitingApprovalHandler) SetAPIReader(reader client.Reader) {
	if reader != nil {
		h.apiReader = reader
	}
}

// SetClock injects the time source used for strict deadline and expiry
// decisions. Production wiring uses the shared real clock; tests use a fixed
// clock to prove before/equal/after boundaries.
func (h *AwaitingApprovalHandler) SetClock(value sharedclock.Clock) {
	if value != nil {
		h.clock = value
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
			return h.handleMissingRAR(ctx, rr, rarName)
		}
		logger.Error(err, "Failed to get RemediationApprovalRequest")
		return phase.TransitionIntent{}, err
	}

	switch rar.Status.Decision {
	case remediationv1.ApprovalDecisionApproved:
		if !isTimelyDecisionAt(rar, h.clock.Now()) {
			return h.unavailableDecision(ctx, rr, rar, "approved decision is missing valid actor or timing provenance")
		}
		return h.handleApproved(ctx, rr, rar)
	case remediationv1.ApprovalDecisionRejected:
		if !isTimelyDecisionAt(rar, h.clock.Now()) {
			return h.unavailableDecision(ctx, rr, rar, "rejected decision is missing valid actor or timing provenance")
		}
		return h.handleRejected(ctx, rr, rar)
	case remediationv1.ApprovalDecisionExpired:
		if !isConfirmedExpiredAt(rar, h.clock.Now()) {
			return h.unavailableDecision(ctx, rr, rar, "expired decision is not authoritatively confirmed")
		}
		return h.handleExpired(ctx, rr)
	case remediationv1.ApprovalDecisionCancelled:
		if !isTimelySystemCancellationAt(rar, h.clock.Now()) {
			return h.unavailableDecision(ctx, rr, rar, "cancelled decision has missing, invalid, or unverifiable provenance")
		}
		return h.handleCancelled(ctx, rr, rar)
	case remediationv1.ApprovalDecisionPending:
		return h.handlePending(ctx, rr, rar)
	default:
		return h.unavailableDecision(ctx, rr, rar, "approval request contains an unsupported decision")
	}
}

// handleMissingRAR confirms a cache miss before failing the approval phase.
func (h *AwaitingApprovalHandler) handleMissingRAR(ctx context.Context, rr *remediationv1.RemediationRequest, name string) (phase.TransitionIntent, error) {
	rar := &remediationv1.RemediationApprovalRequest{}
	err := h.apiReader.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)
	if err == nil {
		// Wait for the cache so deletion/finalizer handling runs before a decision is consumed.
		return phase.Requeue(config.RequeueGenericError, "waiting for approval request cache"), nil
	}
	if !apierrors.IsNotFound(err) {
		return phase.TransitionIntent{}, fmt.Errorf("confirm approval request absence: %w", err)
	}
	reason := fmt.Errorf("%s", approvalRequestMissingReason)
	log.FromContext(ctx).Error(reason, "Approval request is missing; manual review required", "remediationRequest", rr.Name)
	h.callbacks.RecordEvent(rr, "Warning", "RemediationFailed", reason.Error())
	result, transitionErr := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, reason)
	if transitionErr != nil {
		return phase.TransitionIntent{}, transitionErr
	}
	return resultToIntent(result, "approval request missing"), nil
}

func isTimelyDecisionAt(rar *remediationv1.RemediationApprovalRequest, now time.Time) bool {
	if rar == nil || rar.Spec.RequiredBy.IsZero() || rar.Status.DecidedAt == nil || rar.Status.DecidedAt.IsZero() {
		return false
	}
	if !rar.Status.DecidedAt.Before(&rar.Spec.RequiredBy) || rar.Status.DecidedAt.After(now) || rar.Status.Expired {
		return false
	}
	if rar.Status.Decision == remediationv1.ApprovalDecisionApproved || rar.Status.Decision == remediationv1.ApprovalDecisionRejected {
		return rar.Status.DecidedBy != "" && rar.Status.DecidedBy != systemDecisionActor
	}
	return rar.Status.Decision == remediationv1.ApprovalDecisionCancelled &&
		rar.Status.DecidedBy == systemDecisionActor && rar.Status.DecidedVia == ""
}

func isTimelySystemCancellationAt(rar *remediationv1.RemediationApprovalRequest, now time.Time) bool {
	return rar != nil && rar.Status.Decision == remediationv1.ApprovalDecisionCancelled && isTimelyDecisionAt(rar, now)
}

func isConfirmedExpiredAt(rar *remediationv1.RemediationApprovalRequest, now time.Time) bool {
	if rar == nil || rar.Status.Decision != remediationv1.ApprovalDecisionExpired || !rar.Status.Expired ||
		rar.Status.DecidedBy != systemDecisionActor || rar.Status.DecidedVia != "" || rar.Spec.RequiredBy.IsZero() || rar.Status.DecidedAt == nil || rar.Status.DecidedAt.IsZero() {
		return false
	}
	return !rar.Status.DecidedAt.Before(&rar.Spec.RequiredBy) && !rar.Status.DecidedAt.After(now)
}

func (h *AwaitingApprovalHandler) handleApproved(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	var safetyErr error
	if rar.Status.WorkflowOverride != nil && rar.Status.WorkflowOverride.WorkflowName != "" {
		safetyErr = fmt.Errorf("approved approval request contains a forbidden workflow switch")
	} else if err := remediationapprovalrequest.ValidateSnapshotPresence(&rar.Spec); err != nil {
		safetyErr = fmt.Errorf("approved approval request is missing required workflow snapshot data: %w", err)
	}
	if safetyErr != nil {
		logger.Error(safetyErr, "Cannot execute approval")
		h.callbacks.RecordEvent(rr, "Warning", "RemediationFailed", safetyErr.Error())
		result, err := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, safetyErr)
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "unsafe approval request"), nil
	}

	logger.Info("Approval granted via RemediationApprovalRequest",
		"decidedBy", rar.Status.DecidedBy)

	h.callbacks.RecordEvent(rr, "Normal", "ApprovalGranted",
		fmt.Sprintf("Approval granted by %s", rar.Status.DecidedBy))

	if err := h.callbacks.UpdateRARConditions(ctx, rr, rar, "approved"); err != nil {
		logger.Error(err, "Failed to update RAR conditions")
	}

	ai, resolvedAI, fetchIntent, done, err := h.resolveApprovedWorkflow(ctx, rr, rar, logger)
	if done {
		return fetchIntent, err
	}

	preHash := h.capturePostApprovalHash(ctx, rr, ai, rar.Spec.TargetContext, logger)

	// Lock scope must span from acquisition through WFE creation (prevents a
	// concurrent reconcile from creating a duplicate WFE for the same target
	// resource) — the lock is therefore acquired/released here rather than
	// inside checkApprovalTargetBusy.
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

		if intent, blocked, err := h.checkApprovalTargetBusy(ctx, rr, approvalTargetResource, logger); blocked {
			return intent, err
		}
	}

	executionRR := rr.DeepCopy()
	executionRR.Spec.TargetResource = rar.Spec.TargetContext.SignalTarget
	executionRR.Spec.ClusterID = rar.Spec.TargetContext.SignalClusterID
	intent, err := CreateWFEAndTransition(ctx, h.k8sClient, h.m, executionRR, resolvedAI, preHash, h.callbacks.WFECallbacks)
	rr.Status = executionRR.Status
	rr.ResourceVersion = executionRR.ResourceVersion
	return intent, err
}

// resolveApprovedWorkflow fetches the AIAnalysis backing an approved RR and
// resolves any operator-supplied WorkflowOverride against it (ADR-040
// invariant: AIAnalysisRef must be set once Approving is reached). Extracted
// from handleApproved per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
// Returns done=true when the caller must return (intent, err) immediately.
func (h *AwaitingApprovalHandler) resolveApprovedWorkflow(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, logger logr.Logger) (*aianalysisv1.AIAnalysis, *aianalysisv1.AIAnalysis, phase.TransitionIntent, bool, error) {
	frozen := rar.DeepCopy()
	ai := &aianalysisv1.AIAnalysis{}
	ai.Name = rar.Spec.AIAnalysisRef.Name
	ai.Namespace = rr.Namespace
	ai.Status.RCAResult = &aianalysisv1.RCAResult{
		SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
			WorkflowSnapshot: *frozen.Spec.Workflow,
			Parameters:       frozen.Spec.Parameters,
			ParameterSchema:  frozen.Spec.ParameterSchema,
			Confidence:       rar.Spec.Confidence,
			Rationale:        rar.Spec.RecommendedWorkflow.Rationale,
		},
	}
	if target := frozen.Spec.TargetContext.RemediationTarget; target != nil {
		ai.Status.RCAResult.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Kind: target.Kind, Name: target.Name, Namespace: target.Namespace, APIVersion: target.APIVersion,
			},
		}
	}

	resolvedWorkflow, overrideApplied, resolveErr := h.callbacks.ResolveWorkflow(
		ctx, rar.Status.WorkflowOverride, ai.Status.GetRCAResult().SelectedWorkflow, rr.Namespace)
	if resolveErr != nil {
		if override.IsOverrideNotFoundError(resolveErr) {
			logger.Error(resolveErr, "Override workflow not found, failing RR")
			h.callbacks.RecordEvent(rr, "Warning", "RemediationFailed",
				fmt.Sprintf("Override workflow not found: %v", resolveErr))
			result, err := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, resolveErr)
			if err != nil {
				return nil, nil, phase.TransitionIntent{}, true, err
			}
			return nil, nil, resultToIntent(result, "override not found"), true, nil
		}
		logger.Error(resolveErr, "Failed to resolve operator workflow override")
		return nil, nil, phase.Requeue(config.RequeueGenericError, "override resolution failed"), true, nil
	}

	resolvedAI := ai.DeepCopy()
	if overrideApplied {
		resolvedAI.Status.EnsureRCAResult().SelectedWorkflow = resolvedWorkflow
		h.callbacks.RecordEvent(rr, "Normal", "OperatorOverride",
			fmt.Sprintf("Operator override applied: workflow=%s", rar.Status.WorkflowOverride.WorkflowName))
		h.m.OverrideAppliedTotal.WithLabelValues(classifyOverride(rar.Status.WorkflowOverride), rr.Namespace).Inc()
	}

	return ai, resolvedAI, phase.TransitionIntent{}, false, nil
}

// capturePostApprovalHash captures the pre-remediation resource hash after
// approval. All failure modes here are non-fatal (best-effort): a failed or
// degraded capture is logged/recorded but does not block WFE creation.
// Extracted from handleApproved per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
func (h *AwaitingApprovalHandler) capturePostApprovalHash(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, target *remediationv1.ApprovalTargetContext, logger logr.Logger) string {
	targetRR := rr.DeepCopy()
	targetRR.Spec.TargetResource = target.SignalTarget
	targetRR.Spec.ClusterID = target.SignalClusterID
	remTarget := h.callbacks.ResolveDualTargets(targetRR, ai).Remediation
	preHash, degradedReason, hashErr := h.callbacks.CapturePreRemediationHash(
		ctx, remTarget.Kind, remTarget.Name, remTarget.Namespace, target.SignalClusterID)
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
	if preHash != "" && rr.Status.EnsureOperatorAudit().PreRemediationSpecHash == "" {
		if err := h.callbacks.PersistPreHash(ctx, rr, preHash); err != nil {
			logger.Error(err, "Failed to persist pre-remediation hash (non-fatal)")
		}
	}
	return preHash
}

// checkApprovalTargetBusy checks whether the approval target resource is
// busy (under an already-held distributed lock) and blocks the RR if so.
// Extracted from handleApproved per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520). The caller is responsible for the lock's acquire/release
// scope (must span through WFE creation). Returns blocked=true when the
// caller must return (intent, err) immediately.
func (h *AwaitingApprovalHandler) checkApprovalTargetBusy(ctx context.Context, rr *remediationv1.RemediationRequest, approvalTargetResource string, logger logr.Logger) (phase.TransitionIntent, bool, error) {
	busyBlock, busyErr := h.callbacks.CheckResourceBusy(ctx, rr, approvalTargetResource)
	if busyErr != nil && !errors.Is(busyErr, routing.ErrNotBlocked) {
		logger.Error(busyErr, "Failed to check resource busy (approval path)")
		return phase.Requeue(config.RequeueGenericError, "resource busy check failed"), true, nil
	}
	if busyBlock != nil {
		logger.Info("Target resource busy after approval - blocking")
		result, err := h.callbacks.HandleBlocked(ctx, rr, busyBlock, string(remediationv1.PhaseAwaitingApproval), "")
		if err != nil {
			return phase.TransitionIntent{}, true, err
		}
		return resultToIntent(result, "resource busy"), true, nil
	}
	return phase.TransitionIntent{}, false, nil
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

// handleCancelled is intentionally read-only. Cancellation propagation and
// parent/child shutdown are owned by #2588; this baseline only observes a
// trusted terminal RAR result.
func (h *AwaitingApprovalHandler) handleCancelled(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	log.FromContext(ctx).Info("Trusted system cancellation observed; leaving parent unchanged",
		"remediationRequest", rr.Name,
		"rar", rar.Name,
		"decidedAt", rar.Status.DecidedAt,
	)
	return phase.TransitionIntent{}, nil
}

func (h *AwaitingApprovalHandler) unavailableDecision(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, reason string) (phase.TransitionIntent, error) {
	log.FromContext(ctx).Error(nil, "RAR decision is unavailable for safe processing",
		"remediationRequest", rr.Name,
		"rar", rar.Name,
		"decision", rar.Status.Decision,
		"reason", reason,
	)
	return phase.Requeue(config.RequeueGenericError, reason), nil
}

func (h *AwaitingApprovalHandler) handlePending(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	now := h.clock.Now()
	if rar.Spec.RequiredBy.IsZero() {
		return h.unavailableDecision(ctx, rr, rar, "pending approval has no RequiredBy deadline")
	}
	if now.Before(rar.Spec.RequiredBy.Time) {
		return h.waitForPendingApproval(ctx, rar, logger)
	}
	return h.expirePendingApproval(ctx, rr, rar, logger)
}

func (h *AwaitingApprovalHandler) waitForPendingApproval(ctx context.Context, rar *remediationv1.RemediationApprovalRequest, logger logr.Logger) (phase.TransitionIntent, error) {
	if err := h.callbacks.UpdateRARTimeRemaining(ctx, rar); err != nil {
		logger.Error(err, "Failed to update RAR TimeRemaining (non-fatal)")
	}

	logger.V(1).Info("Waiting for approval decision", "rarName", rar.Name)
	return phase.Requeue(config.RequeueResourceBusy, "awaiting approval"), nil
}

func (h *AwaitingApprovalHandler) expirePendingApproval(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, logger logr.Logger) (phase.TransitionIntent, error) {
	now := h.clock.Now()
	logger.Info("Approval deadline passed, marking as expired")
	h.callbacks.RecordEvent(rr, "Warning", "ApprovalExpired",
		fmt.Sprintf("Approval deadline passed after %v without decision",
			now.Sub(rar.ObjectMeta.CreationTimestamp.Time).Round(time.Minute)))

	outcome, err := h.callbacks.ExpireRAR(ctx, rar)
	if err != nil || outcome == Unavailable {
		if err != nil {
			logger.Error(err, "RAR expiry outcome unavailable")
		}
		return phase.Requeue(config.RequeueGenericError, "RAR expiry outcome unavailable"), nil
	}
	if outcome == DecisionWon {
		return h.handleWinningExpiryDecision(ctx, rr, rar)
	}
	if outcome != ExpiredConfirmed {
		return phase.Requeue(config.RequeueGenericError, "unknown RAR expiry outcome"), nil
	}

	result, transitionErr := h.callbacks.TransitionToFailed(ctx, rr, remediationv1.FailurePhaseApproval, fmt.Errorf("approval request expired (timeout)"))
	if transitionErr != nil {
		return phase.TransitionIntent{}, transitionErr
	}
	return resultToIntent(result, "deadline passed"), nil
}

func (h *AwaitingApprovalHandler) handleWinningExpiryDecision(ctx context.Context, rr *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest) (phase.TransitionIntent, error) {
	if !isTimelyDecisionAt(rar, h.clock.Now()) {
		return h.unavailableDecision(ctx, rr, rar, "winning decision is missing valid actor or timing provenance")
	}
	switch rar.Status.Decision {
	case remediationv1.ApprovalDecisionApproved:
		return h.handleApproved(ctx, rr, rar)
	case remediationv1.ApprovalDecisionRejected:
		return h.handleRejected(ctx, rr, rar)
	case remediationv1.ApprovalDecisionCancelled:
		return h.handleCancelled(ctx, rr, rar)
	default:
		return phase.Requeue(config.RequeueGenericError, "RAR expiry decision outcome unavailable"), nil
	}
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
