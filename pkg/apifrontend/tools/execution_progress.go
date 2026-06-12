package tools

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var eaGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "effectivenessassessments"}

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

// FetchStabilizationWindow retrieves the stabilizationWindow from an
// EffectivenessAssessment CRD. Returns empty string on any failure (graceful
// degradation: RBAC missing, EA not yet created, or field absent).
func FetchStabilizationWindow(ctx context.Context, client dynamic.Interface, namespace, eaName string) string {
	if client == nil || eaName == "" {
		return ""
	}
	logger := logr.FromContextOrDiscard(ctx)

	ea, err := client.Resource(eaGVR).Namespace(namespace).Get(ctx, eaName, metav1.GetOptions{})
	if err != nil {
		logger.V(1).Info("EA fetch for stabilizationWindow failed (graceful fallback)",
			"ea_name", eaName, "namespace", namespace, "error", err)
		return ""
	}
	return extractStabilizationWindow(ea)
}

func extractStabilizationWindow(ea *unstructured.Unstructured) string {
	sw, found, err := unstructured.NestedString(ea.Object, "spec", "config", "stabilizationWindow")
	if !found || err != nil {
		return ""
	}
	return sw
}

// extractEARef reads status.effectivenessAssessmentRef.name from an RR object.
// Returns empty string if the reference is absent.
func extractEARef(obj *unstructured.Unstructured) string {
	name, found, err := unstructured.NestedString(obj.Object, "status", "effectivenessAssessmentRef", "name")
	if !found || err != nil {
		return ""
	}
	return name
}
