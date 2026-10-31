package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// handleInvestigationSessionFailure maps an AF InvestigationSession failure to
// its RR using spec.remediationRequestRef.name. Expected parent-driven
// cancellation is not treated as a new RR failure (DD-2582).
func (r *Reconciler) handleInvestigationSessionFailure(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (bool, error) {
	var sessions investigationsessionv1.InvestigationSessionList
	if err := r.client.List(ctx, &sessions,
		client.InNamespace(rr.Namespace),
		client.MatchingFields{"spec.remediationRequestRef.name": rr.Name},
	); err != nil {
		// Isolated legacy unit reconcilers may not register the optional AF CRD;
		// production wiring registers it before SetupWithManager. Do not turn
		// those tests into unrelated scheme failures.
		if strings.Contains(err.Error(), "no kind is registered") {
			return false, nil
		}
		return true, fmt.Errorf("list InvestigationSessions for RR %s/%s: %w", rr.Namespace, rr.Name, err)
	}
	for i := range sessions.Items {
		is := &sessions.Items[i]
		if is.Status.Phase != investigationsessionv1.SessionPhaseFailed {
			continue
		}
		failure := fmt.Errorf("InvestigationSession %s failed: %s", is.Name, is.Status.Message)
		logger.Error(failure, "InvestigationSession failure propagated to RemediationRequest")
		_, err := r.transitionToFailed(ctx, rr, remediationv1.FailurePhaseAIAnalysis, failure)
		return true, err
	}
	return false, nil
}
