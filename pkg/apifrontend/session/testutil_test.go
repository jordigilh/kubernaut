package session_test

import (
	"context"

	adksession "google.golang.org/adk/session"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// setSessionCRDPhase sets status.phase to Active on the IS CRD (AA controller
// ownership in production), in the fixed "test-ns" namespace used across all
// session_test files.
func setSessionCRDPhase(ctx context.Context, k8s client.Client, sessionID string) error {
	var crd v1alpha1.InvestigationSession
	if err := k8s.Get(ctx, types.NamespacedName{Name: sessionID, Namespace: "test-ns"}, &crd); err != nil {
		return err
	}
	crd.Status.Phase = v1alpha1.SessionPhaseActive
	return k8s.Status().Update(ctx, &crd)
}
