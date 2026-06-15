package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
)

// BuildProgressSnapshot constructs the structured payload for an execution_progress
// artifact event. The returned map is suitable for use as an a2a.DataPart.Data field.
func BuildProgressSnapshot(currentPhase, rrName, startedAt, completedAt string) map[string]any {
	payload := map[string]any{
		"type":           "execution_progress",
		"schema_version": "1.0",
		"rr_name":        rrName,
		"current_phase":  currentPhase,
		"started_at":     startedAt,
	}
	if completedAt != "" {
		payload["completed_at"] = completedAt
	}
	return payload
}

// EANameForRR returns the deterministic EffectivenessAssessment name for a
// given RemediationRequest. This convention is established by the RO in
// pkg/remediationorchestrator/creator/effectivenessassessment.go.
func EANameForRR(rrName string) string {
	return fmt.Sprintf("ea-%s", rrName)
}

// FetchStabilizationWindow retrieves the stabilizationWindow from an
// EffectivenessAssessment CRD using the typed client. Returns empty string on
// any failure (graceful degradation: RBAC missing, EA not yet created, or
// field absent).
func FetchStabilizationWindow(ctx context.Context, reader crclient.Reader, cb *resilience.K8sCircuitBreaker, namespace, eaName string) string {
	if reader == nil || eaName == "" {
		return ""
	}
	logger := logr.FromContextOrDiscard(ctx)

	ea := &eav1alpha1.EffectivenessAssessment{}
	key := types.NamespacedName{Namespace: namespace, Name: eaName}

	var getErr error
	if cb != nil {
		getErr = cb.Execute(ctx, func(ctx context.Context) error {
			return reader.Get(ctx, key, ea)
		})
	} else {
		getErr = reader.Get(ctx, key, ea)
	}
	if getErr != nil {
		logger.V(1).Info("EA fetch for stabilizationWindow failed (graceful fallback)",
			"ea_name", eaName, "namespace", namespace, "error", getErr)
		return ""
	}
	return ea.Spec.Config.StabilizationWindow.Duration.String()
}
