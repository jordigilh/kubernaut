package aianalysis

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

// reconcileParentCancellation requests AgentSession shutdown and waits until
// the session is gone before acknowledging AIAnalysis cancellation. The
// AgentSession dispatcher owns the in-memory investigation cancellation and
// removes its finalizer after observing deletion.
func (r *AIAnalysisReconciler) reconcileParentCancellation(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	parentName := analysis.Spec.RemediationRequestRef.Name
	if parentName == "" {
		return ctrl.Result{}, false, nil
	}
	stopRequested, parentMissing, err := r.parentCancellationRequested(ctx, analysis)
	if err != nil {
		return ctrl.Result{}, false, err
	}
	if !stopRequested {
		return ctrl.Result{}, false, nil
	}

	message := "parent RemediationRequest cancellation observed"
	if parentMissing {
		message = "parent RemediationRequest is missing; stopping orphaned AIAnalysis"
	}
	if cancellation.IsAcknowledged(analysis.Status.Conditions) && isTerminalPhase(analysis.Status.Phase) {
		if !analysis.DeletionTimestamp.IsZero() {
			// Let the normal deletion path remove AIAnalysis's own finalizer.
			return ctrl.Result{}, false, nil
		}
		return ctrl.Result{}, true, nil
	}

	if result, handled, err := r.reconcileAgentSessionCancellation(ctx, analysis, logger); handled {
		return result, true, err
	}

	if !isTerminalPhase(analysis.Status.Phase) {
		analysis.Status.Phase = aianalysisv1.PhaseCancelled
		analysis.Status.Reason = aianalysisv1.ReasonParentCancelled
		completedAt := metav1.Now()
		analysis.Status.CompletedAt = &completedAt
	}
	analysis.Status.ObservedGeneration = analysis.Generation
	analysis.Status.Message = message
	cancellation.SetAcknowledged(&analysis.Status.Conditions, analysis.Generation, message)
	if updateErr := r.Status().Update(ctx, analysis); updateErr != nil {
		logger.Error(updateErr, "Failed to acknowledge AIAnalysis parent cancellation")
		return ctrl.Result{}, true, updateErr
	}
	return ctrl.Result{}, true, nil
}

func (r *AIAnalysisReconciler) parentCancellationRequested(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
) (bool, bool, error) {
	parentName := analysis.Spec.RemediationRequestRef.Name
	parentNamespace := analysis.Spec.RemediationRequestRef.Namespace
	if parentNamespace == "" {
		parentNamespace = analysis.Namespace
	}
	parent := &remediationv1.RemediationRequest{}
	err := r.Get(ctx, client.ObjectKey{Name: parentName, Namespace: parentNamespace}, parent)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, false, fmt.Errorf("get parent RemediationRequest for cancellation: %w", err)
	}
	if apierrors.IsNotFound(err) {
		// Synthetic or legacy AIAnalysis objects may carry a correlation
		// reference without being controller-owned by a RemediationRequest
		// (mirrors the WFE rule from commit 2bcb59b1f). Only a persisted
		// controller owner proves that a missing parent represents an
		// orphaned production child; anything else keeps its normal
		// lifecycle instead of failing closed to cancellation.
		return hasRemediationRequestOwner(analysis), true, nil
	}
	if !cancellation.ParentStopRequested(parent) {
		return false, false, nil
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
		cancellation.ReferenceIdentity{
			Name:      parentName,
			Namespace: parentNamespace,
			UID:       analysis.Spec.RemediationRequestRef.UID,
		},
		analysis.OwnerReferences,
		"RemediationRequest",
	) {
		return false, false, fmt.Errorf("validate aianalysis parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	return true, false, nil
}

func (r *AIAnalysisReconciler) reconcileAgentSessionCancellation(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	if r.AgentSessionCreator == nil {
		return ctrl.Result{}, false, nil
	}
	if analysis.Status.CancellationRetryCount >= cancellation.MaxCancellationRetries {
		return r.finalizeAgentSessionCancellationExhaustion(ctx, analysis, logger)
	}
	if err := r.AgentSessionCreator.DeleteForCascadeCancel(ctx, analysis); err != nil {
		return r.recordAgentSessionCancellationFailure(
			ctx, analysis, logger, err, "record AIAnalysis cancellation failure",
			"Failed to request AgentSession cancellation")
	}

	session := &agentsessionv1.AgentSession{}
	sessionErr := r.Get(ctx, client.ObjectKey{Name: "as-" + analysis.Spec.RemediationRequestRef.Name, Namespace: analysis.Namespace}, session)
	if apierrors.IsNotFound(sessionErr) {
		return ctrl.Result{}, false, nil
	}
	if sessionErr != nil {
		return r.recordAgentSessionCancellationFailure(
			ctx, analysis, logger, sessionErr, "record AIAnalysis session verification failure",
			"Failed to verify AgentSession cancellation")
	}

	analysis.Status.Phase = aianalysisv1.PhaseCancelling
	if session.DeletionTimestamp.IsZero() {
		analysis.Status.Message = "AgentSession deletion requested; waiting for dispatcher observation"
	} else {
		analysis.Status.Message = "AgentSession deletion requested; waiting for dispatcher cleanup"
	}
	analysis.Status.CancellationRetryCount++
	if analysis.Status.CancellationRetryCount >= cancellation.MaxCancellationRetries {
		markAgentSessionCancellationExhausted(analysis)
	} else {
		cancellation.SetRequested(&analysis.Status.Conditions, analysis.Generation, analysis.Status.Message)
	}
	if err := r.Status().Update(ctx, analysis); err != nil {
		return ctrl.Result{}, true, err
	}
	if analysis.Status.CancellationRetryCount >= cancellation.MaxCancellationRetries {
		r.recordAgentSessionCancellationAudit(ctx, analysis)
		logger.Error(nil, "AIAnalysis AgentSession cancellation retry budget exhausted")
		return ctrl.Result{}, true, nil
	}
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(analysis.Status.CancellationRetryCount)}, true, nil
}

func (r *AIAnalysisReconciler) recordAgentSessionCancellationFailure(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	logger logr.Logger,
	failure error,
	updateMessage string,
	logMessage string,
) (ctrl.Result, bool, error) {
	analysis.Status.CancellationRetryCount++
	if analysis.Status.CancellationRetryCount >= cancellation.MaxCancellationRetries {
		markAgentSessionCancellationExhausted(analysis)
	} else {
		analysis.Status.Phase = aianalysisv1.PhaseCancelling
		cancellation.SetFailed(&analysis.Status.Conditions, analysis.Generation, failure.Error())
		analysis.Status.Message = failure.Error()
	}
	if err := r.Status().Update(ctx, analysis); err != nil {
		return ctrl.Result{}, true, fmt.Errorf("%s: %w", updateMessage, err)
	}
	r.recordAgentSessionCancellationAudit(ctx, analysis)
	logger.Error(failure, logMessage)
	if analysis.Status.CancellationRetryCount >= cancellation.MaxCancellationRetries {
		logger.Error(nil, "AIAnalysis AgentSession cancellation retry budget exhausted")
		return ctrl.Result{}, true, nil
	}
	// The failure is durable in status and audit evidence; return the explicit
	// backoff without an error so controller-runtime does not discard it.
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(analysis.Status.CancellationRetryCount)}, true, nil
}

func (r *AIAnalysisReconciler) finalizeAgentSessionCancellationExhaustion(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	if !cancellation.IsFailed(analysis.Status.Conditions) || !analysis.Status.GetReview().NeedsHumanReview {
		markAgentSessionCancellationExhausted(analysis)
		if err := r.Status().Update(ctx, analysis); err != nil {
			return ctrl.Result{}, true, fmt.Errorf("persist exhausted AIAnalysis AgentSession cancellation: %w", err)
		}
		r.recordAgentSessionCancellationAudit(ctx, analysis)
	}
	logger.Error(nil, "AIAnalysis AgentSession cancellation retry budget exhausted")
	return ctrl.Result{}, true, nil
}

func markAgentSessionCancellationExhausted(analysis *aianalysisv1.AIAnalysis) {
	analysis.Status.Phase = aianalysisv1.PhaseCancelling
	analysis.Status.Message = "AgentSession cancellation retry budget exhausted; manual review required"
	analysis.Status.SubReason = aianalysisv1.SubReasonMaxRetriesExceeded
	review := analysis.Status.EnsureReview()
	review.NeedsHumanReview = true
	review.HumanReviewReason = "operator_escalation"
	cancellation.SetFailed(&analysis.Status.Conditions, analysis.Generation, analysis.Status.Message)
}

func (r *AIAnalysisReconciler) recordAgentSessionCancellationAudit(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
) {
	if r.AuditClient != nil {
		r.AuditClient.RecordError(ctx, analysis, "Cancellation", fmt.Errorf("%s", analysis.Status.Message))
	}
}

func hasRemediationRequestOwner(analysis *aianalysisv1.AIAnalysis) bool {
	for _, owner := range analysis.OwnerReferences {
		if owner.Controller != nil && *owner.Controller &&
			owner.Kind == "RemediationRequest" &&
			owner.Name == analysis.Spec.RemediationRequestRef.Name {
			return true
		}
	}
	return false
}

func mapRemediationRequestToAIAnalysis(_ context.Context, rr *remediationv1.RemediationRequest) []ctrl.Request {
	if rr == nil || rr.Status.GetPhaseProgress().AIAnalysisRef == nil {
		return nil
	}
	ref := rr.Status.GetPhaseProgress().AIAnalysisRef
	namespace := ref.Namespace
	if namespace == "" {
		namespace = rr.Namespace
	}
	return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: namespace, Name: ref.Name}}}
}

func isTerminalPhase(current string) bool {
	return current == aianalysisv1.PhaseCompleted ||
		current == aianalysisv1.PhaseFailed ||
		current == aianalysisv1.PhaseCancelled
}
