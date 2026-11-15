package workflowexecution

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

const workflowExecutionCancellationMaxRetries = cancellation.MaxCancellationRetries

// reconcileParentCancellation stops the external execution before WFE claims
// cancellation. It also handles direct WFE deletion, because a deletion
// request is itself an authoritative stop request for the backend.
func (r *WorkflowExecutionReconciler) reconcileParentCancellation(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	parentStopRequested, err := r.parentCancellationRequested(ctx, wfe)
	if err != nil {
		return ctrl.Result{}, false, err
	}
	if !parentStopRequested {
		return ctrl.Result{}, false, nil
	}

	if cancellationAlreadyAcknowledged(wfe) {
		return ctrl.Result{}, true, nil
	}
	if isTerminalWFEPhase(wfe.Status.Phase) {
		return r.reconcileTerminalCancellation(ctx, wfe, logger)
	}
	if wfe.Status.CancellationRetryCount >= workflowExecutionCancellationMaxRetries {
		return r.finalizeCancellationRetryExhaustion(ctx, wfe, logger)
	}

	// Count one bounded cancellation attempt before resolving the executor so
	// configuration failures cannot bypass the retry budget.
	before := wfe.DeepCopy().Status
	wfe.Status.Phase = workflowexecutionv1.PhaseCancelling
	wfe.Status.CancellationRetryCount++

	// Already-terminal execution outcomes are preserved and do not need a new
	// stop request; the terminal backend observation is the acknowledgement.
	cancellable, err := r.cancellationExecutor(wfe)
	if err != nil {
		return r.recordCancellationFailureFromStatus(ctx, wfe, logger, before, err)
	}

	verification, err := requestAndVerifyStop(ctx, cancellable, wfe, r.ExecutionNamespace)
	if err != nil {
		return r.recordCancellationFailureFromStatus(ctx, wfe, logger, before, err)
	}

	backendStopped := applyStopVerification(wfe, verification)
	if !backendStopped && wfe.Status.CancellationRetryCount >= workflowExecutionCancellationMaxRetries {
		finalizeCancellationFailure(wfe, "cancellation retry budget exhausted before backend termination was verified")
	}
	if err := r.updateCancellationStatus(ctx, wfe, before); err != nil {
		return ctrl.Result{}, true, err
	}
	if backendStopped && !wfe.DeletionTimestamp.IsZero() {
		result, err := r.reconcileDelete(ctx, wfe, true)
		return result, true, err
	}
	if cancellation.IsFailed(wfe.Status.Conditions) {
		logger.Error(nil, "WorkflowExecution cancellation retry budget exhausted")
		return ctrl.Result{}, true, nil
	}
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(wfe.Status.CancellationRetryCount)}, true, nil
}

func cancellationAlreadyAcknowledged(wfe *workflowexecutionv1.WorkflowExecution) bool {
	return cancellation.IsAcknowledged(wfe.Status.Conditions) &&
		isTerminalWFEPhase(wfe.Status.Phase) &&
		wfe.DeletionTimestamp.IsZero()
}

func (r *WorkflowExecutionReconciler) reconcileTerminalCancellation(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	if !wfe.DeletionTimestamp.IsZero() {
		// A terminal phase preserves the business outcome, but deletion still
		// requires a current backend stop verification before acknowledgement.
		result, handled, err := r.reconcileDeleteExecutionStop(ctx, wfe, logger)
		if err != nil || handled {
			return result, true, err
		}
	}
	if terminalBackendVerificationRequired(wfe) {
		return r.reconcileTerminalBackendCancellation(ctx, wfe, logger)
	}

	before := wfe.DeepCopy().Status
	wfe.Status.ObservedGeneration = wfe.Generation
	cancellation.SetAcknowledged(&wfe.Status.Conditions, wfe.Generation, "terminal workflow outcome preserved; no execution backend was recorded")
	if err := r.updateCancellationStatus(ctx, wfe, before); err != nil {
		return ctrl.Result{}, true, err
	}
	if !wfe.DeletionTimestamp.IsZero() {
		result, err := r.reconcileDelete(ctx, wfe, true)
		return result, true, err
	}
	return ctrl.Result{}, true, nil
}

// terminalBackendVerificationRequired distinguishes a terminal WFE that never
// recorded an execution identity from one that did. The former is positive
// evidence that no backend was created; the latter must still be stopped and
// verified before cancellation is acknowledged.
func terminalBackendVerificationRequired(wfe *workflowexecutionv1.WorkflowExecution) bool {
	return wfe.Status.ExecutionRef != nil ||
		wfe.Status.AWXLaunchLabel != "" ||
		weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)
}

func (r *WorkflowExecutionReconciler) reconcileTerminalBackendCancellation(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	if wfe.Status.CancellationRetryCount >= workflowExecutionCancellationMaxRetries {
		return r.finalizeTerminalCancellationRetryExhaustion(ctx, wfe, logger)
	}

	before := wfe.DeepCopy().Status
	wfe.Status.CancellationRetryCount++
	cancellable, err := r.cancellationExecutor(wfe)
	if err != nil {
		return r.recordTerminalCancellationFailure(ctx, wfe, logger, before, err)
	}
	verification, err := requestAndVerifyStop(ctx, cancellable, wfe, r.ExecutionNamespace)
	if err != nil {
		return r.recordTerminalCancellationFailure(ctx, wfe, logger, before, err)
	}

	backendStopped := applyStopVerification(wfe, verification)
	if !backendStopped && wfe.Status.CancellationRetryCount >= workflowExecutionCancellationMaxRetries {
		finalizeTerminalCancellationFailure(wfe, "cancellation retry budget exhausted before backend termination was verified")
	}
	if err := r.updateCancellationStatus(ctx, wfe, before); err != nil {
		return ctrl.Result{}, true, err
	}
	if backendStopped {
		return ctrl.Result{}, true, nil
	}
	if cancellation.IsFailed(wfe.Status.Conditions) {
		logger.Error(nil, "Terminal WorkflowExecution cancellation could not verify backend termination")
		return ctrl.Result{}, true, nil
	}
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(wfe.Status.CancellationRetryCount)}, true, nil
}

func (r *WorkflowExecutionReconciler) recordTerminalCancellationFailure(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
	before workflowexecutionv1.WorkflowExecutionStatus,
	err error,
) (ctrl.Result, bool, error) {
	wfe.Status.FailureReason = err.Error()
	cancellation.SetFailed(&wfe.Status.Conditions, wfe.Generation, err.Error())
	if updateErr := r.updateCancellationStatus(ctx, wfe, before); updateErr != nil {
		logger.Error(updateErr, "Failed to record terminal WorkflowExecution cancellation failure")
		return ctrl.Result{}, true, updateErr
	}
	logger.Error(err, "Terminal WorkflowExecution backend cancellation is not verified")
	if wfe.Status.CancellationRetryCount >= workflowExecutionCancellationMaxRetries {
		return ctrl.Result{}, true, nil
	}
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(wfe.Status.CancellationRetryCount)}, true, nil
}

func (r *WorkflowExecutionReconciler) finalizeTerminalCancellationRetryExhaustion(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	if !cancellation.IsFailed(wfe.Status.Conditions) {
		before := wfe.DeepCopy().Status
		finalizeTerminalCancellationFailure(wfe, "cancellation retry budget exhausted before backend termination was verified")
		if err := r.updateCancellationStatus(ctx, wfe, before); err != nil {
			logger.Error(err, "Failed to persist exhausted terminal WorkflowExecution cancellation")
			return ctrl.Result{}, true, err
		}
	}
	logger.Error(nil, "Terminal WorkflowExecution cancellation retry budget exhausted")
	return ctrl.Result{}, true, nil
}

func finalizeTerminalCancellationFailure(wfe *workflowexecutionv1.WorkflowExecution, message string) {
	if wfe.Status.FailureReason == "" {
		wfe.Status.FailureReason = message
	}
	cancellation.SetFailed(&wfe.Status.Conditions, wfe.Generation, wfe.Status.FailureReason)
}

func (r *WorkflowExecutionReconciler) parentCancellationRequested(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
) (bool, error) {
	if !wfe.DeletionTimestamp.IsZero() {
		return true, nil
	}
	ref := wfe.Spec.RemediationRequestRef
	if ref.Name == "" {
		return false, nil
	}
	parentNamespace := ref.Namespace
	if parentNamespace == "" {
		parentNamespace = wfe.Namespace
	}
	parent := &remediationv1.RemediationRequest{}
	err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: parentNamespace}, parent)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("get parent RemediationRequest for cancellation: %w", err)
	}
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if !cancellation.ParentStopRequested(parent) {
		return false, nil
	}
	if !cancellation.MatchesCancellationIdentity(
		cancellation.ParentIdentity{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
		cancellation.ReferenceIdentity{
			Name:      ref.Name,
			Namespace: parentNamespace,
			UID:       ref.UID,
		},
		wfe.OwnerReferences,
		"RemediationRequest",
	) {
		return false, fmt.Errorf("validate workflowexecution parent identity: %w", cancellation.ErrIdentityMismatch)
	}
	return true, nil
}

func (r *WorkflowExecutionReconciler) cancellationExecutor(wfe *workflowexecutionv1.WorkflowExecution) (weexecutor.CancellableExecutor, error) {
	if r.ExecutorRegistry == nil {
		return nil, fmt.Errorf("executor registry is not configured")
	}
	engine := wfe.Spec.WorkflowRef.ExecutionEngine
	exec, err := r.ExecutorRegistry.Get(engine)
	if err != nil {
		return nil, fmt.Errorf("get executor for cancellation: %w", err)
	}
	cancellable, ok := exec.(weexecutor.CancellableExecutor)
	if !ok {
		return nil, fmt.Errorf("executor %q does not support verified cancellation", engine)
	}
	return cancellable, nil
}

func requestAndVerifyStop(
	ctx context.Context,
	exec weexecutor.CancellableExecutor,
	wfe *workflowexecutionv1.WorkflowExecution,
	executionNamespace string,
) (weexecutor.StopVerification, error) {
	if err := exec.RequestStop(ctx, wfe, executionNamespace); err != nil {
		return weexecutor.StopVerification{}, fmt.Errorf("request backend stop: %w", err)
	}
	verification, err := exec.VerifyStopped(ctx, wfe, executionNamespace)
	if err != nil {
		return weexecutor.StopVerification{}, fmt.Errorf("verify backend stop: %w", err)
	}
	return verification, nil
}

func applyStopVerification(
	wfe *workflowexecutionv1.WorkflowExecution,
	verification weexecutor.StopVerification,
) bool {
	if verification.State == weexecutor.StopStateStopped {
		if !isTerminalWFEPhase(wfe.Status.Phase) {
			wfe.Status.Phase = workflowexecutionv1.PhaseCancelled
			wfe.Status.CompletionTime = pointerToTime(metav1.Now())
		}
		wfe.Status.ObservedGeneration = wfe.Generation
		cancellation.SetAcknowledged(&wfe.Status.Conditions, wfe.Generation, verification.Message)
		return true
	}
	wfe.Status.FailureReason = verification.Message
	if verification.State == weexecutor.StopStateUnknown {
		cancellation.SetFailed(&wfe.Status.Conditions, wfe.Generation, verification.Message)
	} else {
		cancellation.SetRequested(&wfe.Status.Conditions, wfe.Generation, verification.Message)
	}
	return false
}

func (r *WorkflowExecutionReconciler) recordCancellationFailureFromStatus(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
	before workflowexecutionv1.WorkflowExecutionStatus,
	err error,
) (ctrl.Result, bool, error) {
	wfe.Status.Phase = workflowexecutionv1.PhaseCancelling
	wfe.Status.FailureReason = err.Error()
	cancellation.SetFailed(&wfe.Status.Conditions, wfe.Generation, err.Error())
	if updateErr := r.updateCancellationStatus(ctx, wfe, before); updateErr != nil {
		logger.Error(updateErr, "Failed to record WorkflowExecution cancellation failure")
		return ctrl.Result{}, true, updateErr
	}
	logger.Error(err, "WorkflowExecution backend cancellation is not verified")
	if wfe.Status.CancellationRetryCount >= workflowExecutionCancellationMaxRetries {
		logger.Error(nil, "WorkflowExecution cancellation retry budget exhausted")
		return ctrl.Result{}, true, nil
	}
	return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(wfe.Status.CancellationRetryCount)}, true, nil
}

func (r *WorkflowExecutionReconciler) finalizeCancellationRetryExhaustion(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	if !cancellation.IsFailed(wfe.Status.Conditions) {
		before := wfe.DeepCopy().Status
		finalizeCancellationFailure(wfe, "cancellation retry budget exhausted before backend termination was verified")
		if err := r.updateCancellationStatus(ctx, wfe, before); err != nil {
			logger.Error(err, "Failed to persist exhausted WorkflowExecution cancellation")
			return ctrl.Result{}, true, err
		}
	}
	logger.Error(nil, "WorkflowExecution cancellation retry budget exhausted")
	return ctrl.Result{}, true, nil
}

func finalizeCancellationFailure(wfe *workflowexecutionv1.WorkflowExecution, message string) {
	wfe.Status.Phase = workflowexecutionv1.PhaseCancelling
	if wfe.Status.FailureReason == "" {
		wfe.Status.FailureReason = message
	}
	cancellation.SetFailed(&wfe.Status.Conditions, wfe.Generation, wfe.Status.FailureReason)
}

// ensureExecutionStoppedForDelete is the deletion-finalizer gate. It is kept
// separate from status reconciliation so a direct Kubernetes delete cannot
// bypass verified backend termination.
func (r *WorkflowExecutionReconciler) ensureExecutionStoppedForDelete(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	logger logr.Logger,
) (bool, error) {
	if r.ExecutorRegistry == nil {
		return false, fmt.Errorf("executor registry is not configured; cannot verify backend termination")
	}
	exec, err := r.ExecutorRegistry.Get(wfe.Spec.WorkflowRef.ExecutionEngine)
	if err != nil {
		return false, fmt.Errorf("get executor for deletion cancellation: %w", err)
	}
	cancellable, ok := exec.(weexecutor.CancellableExecutor)
	if !ok {
		return false, fmt.Errorf("executor %q does not support verified cancellation", exec.Engine())
	}
	if err := cancellable.RequestStop(ctx, wfe, r.ExecutionNamespace); err != nil {
		return false, fmt.Errorf("request backend stop before finalizer removal: %w", err)
	}
	verification, err := cancellable.VerifyStopped(ctx, wfe, r.ExecutionNamespace)
	if err != nil {
		return false, fmt.Errorf("verify backend stop before finalizer removal: %w", err)
	}
	if verification.State != weexecutor.StopStateStopped {
		logger.Info("External execution is not yet stopped", "engine", exec.Engine(), "state", verification.State, "message", verification.Message)
		return false, nil
	}
	return true, nil
}

func (r *WorkflowExecutionReconciler) updateCancellationStatus(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	before workflowexecutionv1.WorkflowExecutionStatus,
) error {
	if apiequality.Semantic.DeepEqual(before, wfe.Status) {
		return nil
	}
	return r.Status().Update(ctx, wfe)
}

func mapRemediationRequestToWorkflowExecution(_ context.Context, rr *remediationv1.RemediationRequest) []ctrl.Request {
	if rr == nil || rr.Status.GetPhaseProgress().WorkflowExecutionRef == nil {
		return nil
	}
	ref := rr.Status.GetPhaseProgress().WorkflowExecutionRef
	namespace := ref.Namespace
	if namespace == "" {
		namespace = rr.Namespace
	}
	return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: namespace, Name: ref.Name}}}
}

func workflowExecutionEventPredicate() predicate.Predicate {
	generationChanged := predicate.GenerationChangedPredicate{}
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if _, ok := e.ObjectNew.(*remediationv1.RemediationRequest); ok {
				return true
			}
			return generationChanged.Update(e)
		},
	}
}

func isTerminalWFEPhase(current string) bool {
	return current == workflowexecutionv1.PhaseCompleted ||
		current == workflowexecutionv1.PhaseFailed ||
		current == workflowexecutionv1.PhaseCancelled
}

func pointerToTime(value metav1.Time) *metav1.Time {
	return &value
}
