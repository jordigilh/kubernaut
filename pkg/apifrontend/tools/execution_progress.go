package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
)

// BuildProgressSnapshot constructs the structured payload for an execution_progress
// artifact event. The returned map is suitable for use as an a2a.DataPart.Data field.
// clusterID is omitted entirely when empty (AU-3: avoids false attribution for
// local-hub RemediationRequests that have no fleet cluster identity).
func BuildProgressSnapshot(currentPhase, rrName, startedAt, completedAt, clusterID string) map[string]any {
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
	if clusterID != "" {
		payload["cluster_id"] = clusterID
	}
	return payload
}

// EANameForRR returns the deterministic EffectivenessAssessment name for a
// given RemediationRequest. This convention is established by the RO in
// pkg/remediationorchestrator/creator/effectivenessassessment.go.
func EANameForRR(rrName string) string {
	return fmt.Sprintf("ea-%s", rrName)
}

// ResolveEAName returns the EA name from the RR's EffectivenessAssessmentRef
// if populated, falling back to EANameForRR for backward compatibility.
func ResolveEAName(rr *remediationv1.RemediationRequest) string {
	if rr.Status.EffectivenessAssessmentRef != nil && rr.Status.EffectivenessAssessmentRef.Name != "" {
		return rr.Status.EffectivenessAssessmentRef.Name
	}
	return EANameForRR(rr.Name)
}

// EATimingMetadata holds timing fields extracted from an EffectivenessAssessment.
// All fields are empty strings when unavailable (graceful degradation).
type EATimingMetadata struct {
	StabilizationWindow string
	ValidityDeadline    string
}

// FetchEATimingMetadata retrieves timing metadata from an EA in a single GET.
// Returns zero-value EATimingMetadata on any failure (graceful degradation).
func FetchEATimingMetadata(ctx context.Context, reader crclient.Reader, cb *resilience.K8sCircuitBreaker, namespace, eaName string) EATimingMetadata {
	if reader == nil || eaName == "" {
		return EATimingMetadata{}
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
		logger.V(1).Info("EA timing metadata fetch failed (graceful fallback)",
			"ea_name", eaName, "namespace", namespace, "error", getErr)
		return EATimingMetadata{}
	}

	result := EATimingMetadata{
		StabilizationWindow: ea.Spec.Config.StabilizationWindow.Duration.String(),
	}
	if ea.Status.ValidityDeadline != nil {
		result.ValidityDeadline = ea.Status.ValidityDeadline.UTC().Format(time.RFC3339)
	}
	return result
}

// FetchStabilizationWindow retrieves the stabilizationWindow from an EA.
// Convenience wrapper around FetchEATimingMetadata for callers that only
// need the stabilization window.
func FetchStabilizationWindow(ctx context.Context, reader crclient.Reader, cb *resilience.K8sCircuitBreaker, namespace, eaName string) string {
	return FetchEATimingMetadata(ctx, reader, cb, namespace, eaName).StabilizationWindow
}

// FetchValidityDeadline retrieves the validity deadline from an EA.
// Convenience wrapper around FetchEATimingMetadata for callers that only
// need the validity deadline.
func FetchValidityDeadline(ctx context.Context, reader crclient.Reader, cb *resilience.K8sCircuitBreaker, namespace, eaName string) string {
	return FetchEATimingMetadata(ctx, reader, cb, namespace, eaName).ValidityDeadline
}
