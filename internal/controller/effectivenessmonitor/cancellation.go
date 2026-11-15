package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

// reconcileParentCancellation prevents assessment work after the parent RR
// stops. EA has no dispatched execution backend, so its own status transition
// is the complete child shutdown protocol.
func (r *Reconciler) reconcileParentCancellation(
	ctx context.Context,
	ea *eav1.EffectivenessAssessment,
	logger logr.Logger,
) (ctrl.Result, bool, error) {
	// Production EAs are controller-owned by their RemediationRequest. Older
	// synthetic fixtures and externally-created assessments may only carry the
	// correlation ID; without the owner identity, fail closed and leave normal
	// assessment reconciliation untouched.
	if !hasRemediationRequestOwner(ea) {
		return ctrl.Result{}, false, nil
	}

	parent := &remediationv1.RemediationRequest{}
	err := r.Get(ctx, client.ObjectKey{Name: ea.Spec.CorrelationID, Namespace: ea.Namespace}, parent)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, true, fmt.Errorf("get parent RemediationRequest for EA cancellation: %w", err)
	}
	if err == nil {
		if !cancellation.ParentStopRequested(parent) {
			return ctrl.Result{}, false, nil
		}
		if !cancellation.MatchesCancellationIdentity(
			cancellation.ParentIdentity{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
			cancellation.ReferenceIdentity{Name: ea.Spec.CorrelationID, Namespace: ea.Namespace},
			ea.OwnerReferences,
			"RemediationRequest",
		) {
			return ctrl.Result{}, true, fmt.Errorf("validate effectivenessassessment parent identity: %w", cancellation.ErrIdentityMismatch)
		}
	}

	message := "parent RemediationRequest cancellation observed"
	if apierrors.IsNotFound(err) {
		message = "parent RemediationRequest is missing; stopping orphaned EffectivenessAssessment"
	}

	if phase.IsTerminal(ea.Status.Phase) {
		if cancellation.IsAcknowledged(ea.Status.Conditions) {
			return ctrl.Result{}, true, nil
		}
		cancellation.SetAcknowledged(&ea.Status.Conditions, ea.Generation, message)
		ea.Status.Message = message
		if updateErr := r.Status().Update(ctx, ea); updateErr != nil {
			logger.Error(updateErr, "Failed to acknowledge terminal EA cancellation")
			return ctrl.Result{}, true, updateErr
		}
		return ctrl.Result{}, true, nil
	}

	if ea.Status.Phase != eav1.PhaseCancelling {
		ea.Status.Phase = eav1.PhaseCancelling
		ea.Status.Message = message
		ea.Status.CancellationRetryCount++
		cancellation.SetRequested(&ea.Status.Conditions, ea.Generation, message)
		if updateErr := r.Status().Update(ctx, ea); updateErr != nil {
			logger.Error(updateErr, "Failed to record EA cancellation request")
			return ctrl.Result{}, true, updateErr
		}
		return ctrl.Result{RequeueAfter: cancellation.RetryBackoff(ea.Status.CancellationRetryCount)}, true, nil
	}

	ea.Status.Phase = eav1.PhaseCancelled
	ea.Status.CompletedAt = pointerToTime(metav1.Now())
	ea.Status.Message = message
	cancellation.SetAcknowledged(&ea.Status.Conditions, ea.Generation, message)
	if updateErr := r.Status().Update(ctx, ea); updateErr != nil {
		logger.Error(updateErr, "Failed to acknowledge EA cancellation")
		return ctrl.Result{}, true, updateErr
	}
	return ctrl.Result{}, true, nil
}

func hasRemediationRequestOwner(ea *eav1.EffectivenessAssessment) bool {
	for _, owner := range ea.OwnerReferences {
		if owner.Kind == "RemediationRequest" && owner.Name == ea.Spec.CorrelationID {
			return true
		}
	}
	return false
}

func mapRemediationRequestToEffectivenessAssessment(_ context.Context, rr *remediationv1.RemediationRequest) []ctrl.Request {
	if rr == nil {
		return nil
	}
	return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: rr.Namespace, Name: "ea-" + rr.Name}}}
}

func effectivenessAssessmentEventPredicate() predicate.Predicate {
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

func pointerToTime(value metav1.Time) *metav1.Time {
	return &value
}
