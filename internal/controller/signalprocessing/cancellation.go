package signalprocessing

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

// reconcileParentCancellation stops SP before it can perform further signal
// processing. SP has no external execution backend, so a status transition and
// positive acknowledgement are sufficient for this child.
func (r *SignalProcessingReconciler) reconcileParentCancellation(
	ctx context.Context,
	sp *signalprocessingv1.SignalProcessing,
	logger logr.Logger,
) (bool, error) {
	// Production SignalProcessing objects are controller-owned by their
	// RemediationRequest. Without that persisted identity, a legacy or synthetic
	// object is not safe to classify as an orphan and normal processing must
	// continue without a parent lookup.
	if !hasRemediationRequestOwner(sp) {
		return false, nil
	}

	stopRequested, parentMissing, err := r.parentCancellationRequested(ctx, sp)
	if err != nil {
		return false, err
	}
	if !stopRequested {
		return false, nil
	}

	message := "parent RemediationRequest cancellation observed"
	if parentMissing {
		message = "parent RemediationRequest is missing; stopping orphaned SignalProcessing"
	}
	if err := r.acknowledgeCancellation(ctx, sp, message); err != nil {
		logger.Error(err, "Failed to acknowledge parent cancellation")
		return true, err
	}
	return true, nil
}

func hasRemediationRequestOwner(sp *signalprocessingv1.SignalProcessing) bool {
	for _, owner := range sp.OwnerReferences {
		if owner.Kind == "RemediationRequest" && owner.Name == sp.Spec.RemediationRequestRef.Name {
			return true
		}
	}
	return false
}

func (r *SignalProcessingReconciler) parentCancellationRequested(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (bool, bool, error) {
	parentNamespace := sp.Spec.RemediationRequestRef.Namespace
	if parentNamespace == "" {
		parentNamespace = sp.Namespace
	}

	parent := &remediationv1.RemediationRequest{}
	err := r.Get(ctx, client.ObjectKey{Name: sp.Spec.RemediationRequestRef.Name, Namespace: parentNamespace}, parent)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, false, fmt.Errorf("get parent RemediationRequest for cancellation: %w", err)
	}
	if err == nil {
		if !cancellation.ParentStopRequested(parent) {
			return false, false, nil
		}
		if !cancellation.MatchesCancellationIdentity(
			cancellation.ParentIdentity{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
			cancellation.ReferenceIdentity{
				Name:      sp.Spec.RemediationRequestRef.Name,
				Namespace: parentNamespace,
				UID:       types.UID(sp.Spec.RemediationRequestRef.UID),
			},
			sp.OwnerReferences,
			"RemediationRequest",
		) {
			return false, false, fmt.Errorf("validate SignalProcessing parent identity: %w", cancellation.ErrIdentityMismatch)
		}
	}
	return true, apierrors.IsNotFound(err), nil
}

func (r *SignalProcessingReconciler) acknowledgeCancellation(ctx context.Context, sp *signalprocessingv1.SignalProcessing, message string) error {
	if cancellation.IsAcknowledged(sp.Status.Conditions) && isTerminal(sp.Status.Phase) {
		return nil
	}

	if !isTerminal(sp.Status.Phase) {
		sp.Status.Phase = signalprocessingv1.PhaseCancelled
		completionTime := metav1.Now()
		sp.Status.CompletionTime = &completionTime
	}
	sp.Status.ObservedGeneration = sp.Generation
	cancellation.SetAcknowledged(&sp.Status.Conditions, sp.Generation, message)
	return r.Status().Update(ctx, sp)
}

func mapRemediationRequestToSignalProcessing(_ context.Context, rr *remediationv1.RemediationRequest) []ctrl.Request {
	if rr == nil || rr.Status.GetPhaseProgress().SignalProcessingRef == nil {
		return nil
	}
	ref := rr.Status.GetPhaseProgress().SignalProcessingRef
	namespace := ref.Namespace
	if namespace == "" {
		namespace = rr.Namespace
	}
	return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: namespace, Name: ref.Name}}}
}

func isTerminal(current signalprocessingv1.SignalProcessingPhase) bool {
	return current == signalprocessingv1.PhaseCompleted ||
		current == signalprocessingv1.PhaseFailed ||
		current == signalprocessingv1.PhaseCancelled
}
