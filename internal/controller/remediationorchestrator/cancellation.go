package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	effectivenessv1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

const (
	remediationRequestCancellationFinalizer = cancellation.RemediationRequestFinalizer
	remediationCancellationMaxRetries       = cancellation.MaxCancellationRetries
)

type cancellationChildStateFn func(
	context.Context,
	*remediationv1.RemediationRequest,
	*corev1.ObjectReference,
) (bool, string, error)

// reconcileCancellation coordinates child shutdown without writing child
// phases. Each child controller owns its own transition and reports verified
// stop through CancellationAcknowledged=True/ CancellationStopped.
func (r *Reconciler) reconcileCancellation(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	logger logr.Logger,
) (ctrl.Result, error) {
	if !cancellationRequested(rr) {
		return ctrl.Result{}, nil
	}

	if err := r.trackCancellationNotificationDeletion(ctx, rr); err != nil {
		return ctrl.Result{}, err
	}

	wasCancelled := rr.Status.OverallPhase == remediationv1.PhaseCancelled
	previousPhase := rr.Status.OverallPhase
	r.initializeCancellationStatus(ctx, rr, previousPhase)

	// NotificationRequests are deliberately excluded from child stop fan-out,
	// but cancellation must still create one and keep it alive until delivery is
	// terminal. This is the human-in-the-loop path for issue #2588.
	notificationReady, notificationFailure, notificationCreationFailed, err := r.prepareCancellationNotification(ctx, rr, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	// A terminal parent may still be held by the cancellation finalizer while
	// the cancellation notification is being delivered. Do not regress its
	// phase; only release the finalizer after notification terminal handling.
	if wasCancelled {
		return r.reconcilePreviouslyCancelled(ctx, rr, notificationReady)
	}

	allAcknowledged, firstFailure, err := r.cancellationChildrenState(ctx, rr)
	if err != nil {
		return ctrl.Result{}, err
	}

	if allAcknowledged && notificationReady {
		return r.completeCancellation(ctx, rr, previousPhase, logger)
	}

	r.recordCancellationProgress(ctx, rr, allAcknowledged, notificationFailure, notificationCreationFailed, firstFailure, logger)
	if err := r.client.Status().Update(ctx, rr); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist RemediationRequest cancellation progress: %w", err)
	}
	status := rr.Status.EnsureCancellation()
	logger.Info("RemediationRequest cancellation still waiting",
		"retryCount", status.RetryCount,
		"lastError", status.LastError,
	)
	if status.State == remediationv1.CancellationStateFailed {
		// Stop automatic polling after the bounded aggregate retry budget. Child
		// and NotificationRequest watches will wake RO when manual remediation
		// changes the observable state.
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(status.RetryCount)}, nil
}

func (r *Reconciler) initializeCancellationStatus(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	previousPhase remediationv1.RemediationPhase,
) {
	status := rr.Status.EnsureCancellation()
	cancellationIntentPending := status.State == "" ||
		status.State == remediationv1.CancellationStateRequested
	if status.RequestID == "" {
		status.RequestID = fmt.Sprintf("%s/%s:cancellation", rr.Namespace, rr.Name)
	}
	if status.RequestedAt == nil {
		now := metav1.Now()
		status.RequestedAt = &now
	}
	if previousPhase != remediationv1.PhaseCancelling && previousPhase != remediationv1.PhaseCancelled {
		rr.Status.OverallPhase = remediationv1.PhaseCancelling
	}
	if status.Source == "" {
		status.Source = remediationv1.CancellationSourceSystem
		if !rr.DeletionTimestamp.IsZero() {
			status.Source = remediationv1.CancellationSourceDeletion
		}
	}
	if previousPhase != remediationv1.PhaseCancelling && previousPhase != remediationv1.PhaseCancelled {
		r.emitPhaseTransitionAudit(ctx, rr, string(previousPhase), string(remediationv1.PhaseCancelling))
	}
	if cancellationIntentPending {
		fromPhase := string(previousPhase)
		if previousPhase == remediationv1.PhaseCancelling {
			fromPhase = "cancellation.requested"
		}
		r.emitCancellationLifecycleAudit(ctx, rr, "request", fromPhase, string(remediationv1.PhaseCancelling))
	}
	if status.State == "" || status.State == remediationv1.CancellationStateRequested {
		status.State = remediationv1.CancellationStateInProgress
	}
}

func (r *Reconciler) prepareCancellationNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	logger logr.Logger,
) (bool, string, bool, error) {
	if cancellationNotificationDeleted(rr) {
		return true, "", false, nil
	}
	name, err := r.ensureCancellationNotification(ctx, rr, logger)
	if err != nil {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"notification.creation.failed",
			"cancellation.notification.requested",
			"cancellation.notification.failed",
			"",
			err,
		)
		logger.Error(err, "Failed to create cancellation notification, will retry")
		return false, err.Error(), true, nil
	}
	ready, failure, err := r.cancellationNotificationState(ctx, rr, name)
	if err != nil {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"notification.delivery.failed",
			"cancellation.notification.requested",
			"cancellation.notification.failed",
			"",
			err,
		)
		return false, "", false, err
	}
	if ready && failure != "" {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"notification.delivery.failed",
			"cancellation.notification.requested",
			"cancellation.notification.failed",
			"",
			errors.New(failure),
		)
	}
	return ready, failure, false, err
}

func (r *Reconciler) reconcilePreviouslyCancelled(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	notificationReady bool,
) (ctrl.Result, error) {
	if !notificationReady {
		status := rr.Status.EnsureCancellation()
		return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(status.RetryCount)}, nil
	}
	if controllerutil.ContainsFinalizer(rr, remediationRequestCancellationFinalizer) {
		controllerutil.RemoveFinalizer(rr, remediationRequestCancellationFinalizer)
		if err := r.client.Update(ctx, rr); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove completed cancellation finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) cancellationChildrenState(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (bool, string, error) {
	allAcknowledged := true
	firstFailure := ""
	if acknowledged, err := r.cancelRARIfNeeded(ctx, rr); err != nil {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"child.remediationapprovalrequest.failed",
			"cancellation.remediationapprovalrequest.requested",
			"cancellation.remediationapprovalrequest.failed",
			"Approval",
			err,
		)
		return false, "", err
	} else {
		allAcknowledged = allAcknowledged && acknowledged
	}
	if ref := rr.Status.GetPhaseProgress().SignalProcessingRef; ref != nil {
		acknowledged, failure, err := r.observeCancellationChild(ctx, rr, ref, r.signalProcessingCancellationState,
			"child.signalprocessing.acknowledged", "cancellation.signalprocessing.requested", "cancellation.signalprocessing.acknowledged")
		if err != nil {
			return false, "", err
		}
		allAcknowledged, firstFailure = mergeCancellationChildResult(allAcknowledged, firstFailure, acknowledged, failure)
	}
	if ref := rr.Status.GetPhaseProgress().AIAnalysisRef; ref != nil {
		acknowledged, failure, err := r.observeCancellationChild(ctx, rr, ref, r.aiAnalysisCancellationState,
			"child.aianalysis.acknowledged", "cancellation.aianalysis.requested", "cancellation.aianalysis.acknowledged")
		if err != nil {
			return false, "", err
		}
		allAcknowledged, firstFailure = mergeCancellationChildResult(allAcknowledged, firstFailure, acknowledged, failure)
	}
	if ref := rr.Status.GetPhaseProgress().WorkflowExecutionRef; ref != nil {
		acknowledged, failure, err := r.observeWorkflowExecutionCancellation(ctx, rr, ref)
		if err != nil {
			return false, "", err
		}
		allAcknowledged, firstFailure = mergeCancellationChildResult(allAcknowledged, firstFailure, acknowledged, failure)
	}
	if ref := rr.Status.GetPhaseProgress().EffectivenessAssessmentRef; ref != nil {
		acknowledged, failure, err := r.observeCancellationChild(ctx, rr, ref, r.effectivenessCancellationState,
			"child.effectivenessassessment.acknowledged", "cancellation.effectivenessassessment.requested", "cancellation.effectivenessassessment.acknowledged")
		if err != nil {
			return false, "", err
		}
		allAcknowledged, firstFailure = mergeCancellationChildResult(allAcknowledged, firstFailure, acknowledged, failure)
	}
	return allAcknowledged, firstFailure, nil
}

func (r *Reconciler) observeWorkflowExecutionCancellation(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ref *corev1.ObjectReference,
) (bool, string, error) {
	acknowledged, failure, backendVerified, err := r.workflowExecutionCancellationState(ctx, rr, ref)
	if err != nil {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"child.workflowexecution.failed",
			"cancellation.workflowexecution.requested",
			"cancellation.workflowexecution.failed",
			"WorkflowExecution",
			err,
		)
		return false, "", err
	}
	if failure != "" {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"backend.workflowexecution.termination_failed",
			"cancellation.workflowexecution.stop_requested",
			"cancellation.workflowexecution.failed",
			"WorkflowExecution",
			errors.New(failure),
		)
	}
	if acknowledged {
		r.emitCancellationLifecycleAudit(ctx, rr, "child.workflowexecution.acknowledged", "cancellation.workflowexecution.requested", "cancellation.workflowexecution.acknowledged")
	}
	if backendVerified {
		r.emitCancellationLifecycleAudit(ctx, rr, "backend.workflowexecution.termination_verified", "cancellation.workflowexecution.stop_requested", "cancellation.workflowexecution.termination_verified")
	}
	return acknowledged, failure, nil
}

func (r *Reconciler) observeCancellationChild(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ref *corev1.ObjectReference,
	stateFn cancellationChildStateFn,
	stage, fromPhase, toPhase string,
) (bool, string, error) {
	acknowledged, failure, err := stateFn(ctx, rr, ref)
	if err != nil {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			cancellationFailureStage(stage),
			fromPhase,
			toPhase,
			cancellationFailurePhase(stage),
			err,
		)
		return false, "", err
	}
	if failure != "" {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			cancellationFailureStage(stage),
			fromPhase,
			toPhase,
			cancellationFailurePhase(stage),
			errors.New(failure),
		)
	}
	if acknowledged {
		r.emitCancellationLifecycleAudit(ctx, rr, stage, fromPhase, toPhase)
	}
	return acknowledged, failure, nil
}

func mergeCancellationChildResult(allAcknowledged bool, firstFailure string, acknowledged bool, failure string) (bool, string) {
	return allAcknowledged && acknowledged, firstFailureOr(firstFailure, failure)
}

func (r *Reconciler) completeCancellation(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	previousPhase remediationv1.RemediationPhase,
	logger logr.Logger,
) (ctrl.Result, error) {
	now := metav1.Now()
	rr.Status.OverallPhase = remediationv1.PhaseCancelled
	rr.Status.CompletedAt = &now
	rr.Status.ObservedGeneration = rr.Generation
	rr.Status.Message = "RemediationRequest cancellation acknowledged by all children"
	status := rr.Status.EnsureCancellation()
	status.State = remediationv1.CancellationStateCompleted
	status.LastError = ""
	r.emitPhaseTransitionAudit(ctx, rr, string(previousPhase), string(remediationv1.PhaseCancelled))
	if err := r.client.Status().Update(ctx, rr); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist RemediationRequest cancellation completion: %w", err)
	}
	if controllerutil.ContainsFinalizer(rr, remediationRequestCancellationFinalizer) {
		controllerutil.RemoveFinalizer(rr, remediationRequestCancellationFinalizer)
		if err := r.client.Update(ctx, rr); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove RemediationRequest cancellation finalizer: %w", err)
		}
	}
	logger.Info("RemediationRequest cancellation completed")
	return ctrl.Result{}, nil
}

func (r *Reconciler) recordCancellationProgress(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	allAcknowledged bool,
	notificationFailure string,
	notificationCreationFailed bool,
	firstFailure string,
	logger logr.Logger,
) {
	status := rr.Status.EnsureCancellation()
	if allAcknowledged {
		// Child work is stopped. Keep the parent in Cancelling until the
		// cancellation notification has finished its own delivery lifecycle.
		status.LastError = notificationFailure
		if notificationCreationFailed && notificationFailure != "" && status.State != remediationv1.CancellationStateFailed {
			status.RetryCount++
			if status.RetryCount >= remediationCancellationMaxRetries {
				r.emitCancellationExhaustedAudit(ctx, rr, notificationFailure)
				status.State = remediationv1.CancellationStateFailed
				r.ensureCancellationFailureNotification(ctx, rr, notificationFailure, logger)
				rr.Status.Message = "Cancellation notification could not be created or delivered; manual review required"
				return
			}
		}
		rr.Status.Message = "Cancellation acknowledged by children; waiting for notification delivery"
		return
	}
	if firstFailure != "" && status.State != remediationv1.CancellationStateFailed {
		status.RetryCount++
	}
	if firstFailure != "" {
		status.LastError = firstFailure
	}
	if status.State != remediationv1.CancellationStateFailed && status.RetryCount >= remediationCancellationMaxRetries {
		r.emitCancellationExhaustedAudit(ctx, rr, firstFailure)
		status.State = remediationv1.CancellationStateFailed
		r.ensureCancellationFailureNotification(ctx, rr, firstFailure, logger)
		rr.Status.Message = "Cancellation could not verify all child stops; manual review required"
		return
	}
	rr.Status.Message = "Cancellation requested; waiting for child acknowledgements"
}

// ensureCancellationNotification creates and tracks the deterministic
// cancellation NotificationRequest. Tracking is kept on the caller's object so
// the aggregate cancellation status and notification ref are persisted in one
// status update.
func (r *Reconciler) ensureCancellationNotification(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (string, error) {
	createdName, err := r.notificationCreator.CreateCancellationNotification(ctx, rr)
	if err != nil {
		return "", err
	}
	if !hasNotificationRef(rr, createdName) {
		ref := r.buildNotificationRef(ctx, createdName, rr.Namespace)
		rr.Status.EnsureCompletionStatus().NotificationRequestRefs = append(rr.Status.EnsureCompletionStatus().NotificationRequestRefs, ref)
	}
	r.emitCancellationLifecycleAudit(ctx, rr, "notification.created", "cancellation.notification.requested", "cancellation.notification.created")
	logger.V(1).Info("Cancellation NotificationRequest is tracked", "notification", createdName)
	return createdName, nil
}

func (r *Reconciler) trackCancellationNotificationDeletion(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) error {
	name := fmt.Sprintf("nr-cancellation-%s", rr.Name)
	if !hasNotificationRef(rr, name) || cancellationNotificationDeleted(rr) {
		return nil
	}

	nr := &notificationv1.NotificationRequest{}
	err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get cancellation NotificationRequest before recreation guard: %w", err)
	}
	if r.notificationHandler == nil {
		return fmt.Errorf("notification handler is not configured for cancellation notification deletion")
	}
	return r.handleNotificationDeletion(ctx, rr)
}

func cancellationNotificationDeleted(rr *remediationv1.RemediationRequest) bool {
	name := fmt.Sprintf("nr-cancellation-%s", rr.Name)
	if !hasNotificationRef(rr, name) {
		return false
	}
	completion := rr.Status.GetCompletionStatus()
	if completion.NotificationStatus != "Cancelled" {
		return false
	}
	condition := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
	return condition != nil &&
		condition.Status == metav1.ConditionFalse &&
		condition.Reason == remediationrequest.ReasonUserCancelled
}

// cancellationNotificationState reports whether the cancellation notification
// has reached a terminal delivery phase. Failed and partially-sent delivery are
// terminal outcomes: the failure remains queryable in NotificationRequest
// status and must not silently block RR cleanup forever.
func (r *Reconciler) cancellationNotificationState(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	name string,
) (bool, string, error) {
	nr := &notificationv1.NotificationRequest{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr); err != nil {
		if apierrors.IsNotFound(err) {
			return false, "cancellation notification is not available", nil
		}
		return false, "", fmt.Errorf("get cancellation NotificationRequest: %w", err)
	}
	if r.notificationHandler != nil {
		if err := r.notificationHandler.UpdateNotificationStatus(ctx, rr, nr); err != nil {
			return false, "", fmt.Errorf("track cancellation notification status: %w", err)
		}
	}
	switch nr.Status.Phase {
	case notificationv1.NotificationPhaseSent:
		return true, "", nil
	case notificationv1.NotificationPhasePartiallySent,
		notificationv1.NotificationPhaseFailed:
		failure := nr.Status.Message
		if failure == "" {
			failure = fmt.Sprintf("cancellation notification reached terminal phase %s", nr.Status.Phase)
		}
		return true, failure, nil
	default:
		return false, "cancellation notification delivery is pending", nil
	}
}

// ensureCancellationFailureNotification publishes manual-review evidence once
// the bounded child-stop retry budget is exhausted. It is idempotent through
// the existing deterministic manual-review notification name.
func (r *Reconciler) ensureCancellationFailureNotification(ctx context.Context, rr *remediationv1.RemediationRequest, failure string, logger logr.Logger) {
	name := fmt.Sprintf("nr-manual-review-%s", rr.Name)
	if hasNotificationRef(rr, name) {
		return
	}
	if failure == "" {
		failure = "Cancellation stop verification exhausted its retry budget"
	}
	notificationName, err := r.notificationCreator.CreateManualReviewNotification(ctx, rr, &creator.ManualReviewContext{
		Source:    notificationv1.ReviewSourceWorkflowExecution,
		Reason:    "CancellationFailed",
		SubReason: "CancellationStopFailed",
		Message:   failure,
	})
	if err != nil {
		r.emitCancellationFailureAudit(
			ctx,
			rr,
			"manual_review.notification.failed",
			"cancellation.retry.exhausted",
			"cancellation.manual_review.notification_failed",
			"",
			err,
		)
		logger.Error(err, "Failed to create cancellation manual-review notification")
		return
	}
	ref := r.buildNotificationRef(ctx, notificationName, rr.Namespace)
	rr.Status.EnsureCompletionStatus().RequiresManualReview = true
	if !hasNotificationRef(rr, notificationName) {
		rr.Status.EnsureCompletionStatus().NotificationRequestRefs = append(rr.Status.EnsureCompletionStatus().NotificationRequestRefs, ref)
	}
}

func (r *Reconciler) cancelRARIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest) (bool, error) {
	rar := &remediationv1.RemediationApprovalRequest{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: "rar-" + rr.Name, Namespace: rr.Namespace}, rar); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("get RemediationApprovalRequest during cancellation: %w", err)
	}
	parentNamespace := rar.Spec.RemediationRequestRef.Namespace
	if parentNamespace == "" {
		parentNamespace = rr.Namespace
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID},
		cancellation.ReferenceIdentity{
			Name:      rar.Spec.RemediationRequestRef.Name,
			Namespace: parentNamespace,
			UID:       rar.Spec.RemediationRequestRef.UID,
		},
		rar.OwnerReferences,
		"RemediationRequest",
	) {
		return false, fmt.Errorf("validate remediationapprovalrequest parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	if rar.Status.Decision != remediationv1.ApprovalDecisionPending {
		return true, nil
	}

	now := metav1.Now()
	rar.Status.Decision = remediationv1.ApprovalDecisionCancelled
	rar.Status.DecidedBy = "system"
	rar.Status.DecidedAt = &now
	rar.Status.DecisionMessage = "Approval cancelled because the parent RemediationRequest was cancelled"
	rar.Status.ObservedGeneration = rar.Generation
	remediationapprovalrequest.SetApprovalCancelled(rar, rar.Status.DecisionMessage, r.Metrics)
	if err := r.client.Status().Update(ctx, rar); err != nil {
		return false, fmt.Errorf("system-cancel RemediationApprovalRequest: %w", err)
	}
	return true, nil
}

// A terminal child has no remaining work, so it counts as implicitly
// acknowledged: it never sets CancellationAcknowledged because it finished
// normally (BR-ORCH-047, #2588). Without this, deleting a parent whose child
// already completed hangs forever behind the retained cancellation finalizer
// (CI run 43971993157). phase.IsTerminal covers every child kind's terminal
// set (Completed/Failed/Cancelled; children never use TimedOut/Skipped).
func childPhaseIsTerminal(childPhase string) bool {
	return phase.IsTerminal(phase.Phase(childPhase))
}

func (r *Reconciler) signalProcessingCancellationState(ctx context.Context, rr *remediationv1.RemediationRequest, ref *corev1.ObjectReference) (bool, string, error) {
	sp := &signalprocessingv1.SignalProcessing{}
	if err := r.client.Get(ctx, cancellationChildObjectKey(rr, ref), sp); err != nil {
		if apierrors.IsNotFound(err) {
			return true, "", nil
		}
		return false, "", fmt.Errorf("get SignalProcessing cancellation state: %w", err)
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID},
		cancellation.ReferenceIdentity{
			Name:      sp.Spec.RemediationRequestRef.Name,
			Namespace: sp.Spec.RemediationRequestRef.Namespace,
			UID:       types.UID(sp.Spec.RemediationRequestRef.UID),
		},
		sp.OwnerReferences,
		"RemediationRequest",
	) {
		return false, "", fmt.Errorf("validate SignalProcessing parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	if childPhaseIsTerminal(string(sp.Status.Phase)) {
		return true, "", nil
	}
	acknowledged, failure := childCancellationState(sp.Status.Conditions)
	return acknowledged, failure, nil
}

func (r *Reconciler) aiAnalysisCancellationState(ctx context.Context, rr *remediationv1.RemediationRequest, ref *corev1.ObjectReference) (bool, string, error) {
	analysis := &aianalysisv1.AIAnalysis{}
	if err := r.client.Get(ctx, cancellationChildObjectKey(rr, ref), analysis); err != nil {
		if apierrors.IsNotFound(err) {
			return true, "", nil
		}
		return false, "", fmt.Errorf("get AIAnalysis cancellation state: %w", err)
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID},
		cancellation.ReferenceIdentity{
			Name:      analysis.Spec.RemediationRequestRef.Name,
			Namespace: analysis.Spec.RemediationRequestRef.Namespace,
			UID:       analysis.Spec.RemediationRequestRef.UID,
		},
		analysis.OwnerReferences,
		"RemediationRequest",
	) {
		return false, "", fmt.Errorf("validate AIAnalysis parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	if childPhaseIsTerminal(analysis.Status.Phase) {
		return true, "", nil
	}
	acknowledged, failure := childCancellationState(analysis.Status.Conditions)
	return acknowledged, failure, nil
}

func (r *Reconciler) workflowExecutionCancellationState(ctx context.Context, rr *remediationv1.RemediationRequest, ref *corev1.ObjectReference) (bool, string, bool, error) {
	wfe := &workflowexecutionv1.WorkflowExecution{}
	if err := r.client.Get(ctx, cancellationChildObjectKey(rr, ref), wfe); err != nil {
		if apierrors.IsNotFound(err) {
			return true, "", false, nil
		}
		return false, "", false, fmt.Errorf("get WorkflowExecution cancellation state: %w", err)
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID},
		cancellation.ReferenceIdentity{
			Name:      wfe.Spec.RemediationRequestRef.Name,
			Namespace: wfe.Spec.RemediationRequestRef.Namespace,
			UID:       wfe.Spec.RemediationRequestRef.UID,
		},
		wfe.OwnerReferences,
		"RemediationRequest",
	) {
		return false, "", false, fmt.Errorf("validate WorkflowExecution parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	if childPhaseIsTerminal(wfe.Status.Phase) {
		return true, "", false, nil
	}
	acknowledged, failure := childCancellationState(wfe.Status.Conditions)
	// The WFE cancellation controller sets CancellationAcknowledged only after
	// RequestStop and VerifyStopped return StopStateStopped. That acknowledgement
	// is the backend-verification evidence even when recovery discovered the
	// backend by AWX label or deterministic Job/Tekton identity and no
	// ExecutionRef was persisted.
	backendVerified := acknowledged
	return acknowledged, failure, backendVerified, nil
}

func (r *Reconciler) effectivenessCancellationState(ctx context.Context, rr *remediationv1.RemediationRequest, ref *corev1.ObjectReference) (bool, string, error) {
	ea := &effectivenessv1.EffectivenessAssessment{}
	if err := r.client.Get(ctx, cancellationChildObjectKey(rr, ref), ea); err != nil {
		if apierrors.IsNotFound(err) {
			return true, "", nil
		}
		return false, "", fmt.Errorf("get EffectivenessAssessment cancellation state: %w", err)
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID},
		cancellation.ReferenceIdentity{Name: ea.Spec.CorrelationID, Namespace: ea.Namespace},
		ea.OwnerReferences,
		"RemediationRequest",
	) {
		return false, "", fmt.Errorf("validate EffectivenessAssessment parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	if childPhaseIsTerminal(ea.Status.Phase) {
		return true, "", nil
	}
	acknowledged, failure := childCancellationState(ea.Status.Conditions)
	return acknowledged, failure, nil
}

func cancellationChildObjectKey(rr *remediationv1.RemediationRequest, ref *corev1.ObjectReference) client.ObjectKey {
	namespace := ref.Namespace
	if namespace == "" {
		namespace = rr.Namespace
	}
	return client.ObjectKey{Name: ref.Name, Namespace: namespace}
}

func childCancellationState(conditions []metav1.Condition) (bool, string) {
	if cancellation.IsAcknowledged(conditions) {
		return true, ""
	}
	condition := meta.FindStatusCondition(conditions, cancellation.ConditionAcknowledged)
	if condition != nil && cancellation.IsFailed(conditions) {
		return false, condition.Message
	}
	return false, ""
}

func firstFailureOr(existing, candidate string) string {
	if existing != "" {
		return existing
	}
	return candidate
}

func cancellationFailureStage(stage string) string {
	return strings.TrimSuffix(stage, ".acknowledged") + ".failed"
}

func cancellationFailurePhase(stage string) string {
	switch {
	case strings.Contains(stage, "signalprocessing"):
		return "SignalProcessing"
	case strings.Contains(stage, "aianalysis"):
		return "AIAnalysis"
	case strings.Contains(stage, "workflowexecution"):
		return "WorkflowExecution"
	case strings.Contains(stage, "remediationapprovalrequest"):
		return "Approval"
	default:
		return ""
	}
}

func (r *Reconciler) emitCancellationExhaustedAudit(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	failure string,
) {
	if failure == "" {
		failure = "cancellation stop verification exhausted its retry budget"
	}
	failureErr := errors.New("cancellation retry budget exhausted: " + failure)
	r.emitCancellationFailureAudit(
		ctx,
		rr,
		"cancellation.retry.exhausted",
		"cancellation.in_progress",
		"cancellation.failed",
		"",
		failureErr,
	)
	r.emitCancellationLifecycleAudit(
		ctx,
		rr,
		"cancellation.manual_review.required",
		"cancellation.failed",
		"cancellation.manual_review.required",
	)
}
