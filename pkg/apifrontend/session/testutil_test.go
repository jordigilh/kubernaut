package session_test

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

func createRequestWithDefaults(sessionID, userID string, state map[string]any) adksession.CreateRequest {
	return adksession.CreateRequest{
		AppName:   "kubernaut-apifrontend",
		UserID:    userID,
		SessionID: sessionID,
		State:     state,
	}
}

// setSessionCRDPhase sets status.phase on the IS CRD (AA controller ownership in production).
func setSessionCRDPhase(ctx context.Context, k8s client.Client, namespace, sessionID string, phase v1alpha1.SessionPhase) error {
	var crd v1alpha1.InvestigationSession
	if err := k8s.Get(ctx, types.NamespacedName{Name: sessionID, Namespace: namespace}, &crd); err != nil {
		return err
	}
	crd.Status.Phase = phase
	return k8s.Status().Update(ctx, &crd)
}
