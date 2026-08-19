/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package handlers implements phase handlers for the AIAnalysis controller.
//
// P1.2 Refactoring: RequestBuilder extracts KA request construction logic
// from InvestigatingHandler to improve maintainability and testability.
package handlers

import (
	"encoding/json"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// RequestBuilder constructs KA requests from AIAnalysis CRD specs.
// P1.2 Refactoring: Extracted from InvestigatingHandler for single responsibility.
//
// Responsibilities:
// - Map AIAnalysis CRD spec to KA OpenAPI generated types
// - Handle optional field population with generated opt types
// - Provide consistent request enrichment patterns
type RequestBuilder struct {
	log logr.Logger
}

// NewRequestBuilder creates a new RequestBuilder instance.
func NewRequestBuilder(log logr.Logger) *RequestBuilder {
	return &RequestBuilder{
		log: log.WithName("request-builder"),
	}
}

// ========================================
// AGENTSESSION SPEC CONSTRUCTION
// DD-AA-KA-001, BR-AA-KA-065.2: 1:1, lossless translation of the retired
// agentclient.IncidentRequest into AgentSessionSpec.
// ========================================

// BuildAgentSessionSpec constructs an AgentSessionSpec from AIAnalysis CRD
// spec -- the CRD-native replacement for BuildIncidentRequest, reading the
// exact same source fields (BR-AA-KA-065.2) so removing the HTTP channel
// loses no content KA previously received.
//
// Parameters:
// - analysis: AIAnalysis CRD containing signal context and enrichment
//
// Returns:
// - agentsessionv1.AgentSessionSpec: immutable spec for the AgentSession AA creates
func (b *RequestBuilder) BuildAgentSessionSpec(analysis *aianalysisv1.AIAnalysis) agentsessionv1.AgentSessionSpec {
	spec := analysis.Spec.AnalysisRequest.SignalContext
	enrichment := spec.EnrichmentResults

	// DD-AUDIT-CORRELATION-001: same correlationID precedence as BuildIncidentRequest.
	correlationID := analysis.Spec.RemediationID
	if analysis.Spec.RemediationRequestRef.Name != "" {
		correlationID = analysis.Spec.RemediationRequestRef.Name
	}

	customLabels := getCustomLabels(enrichment)
	out := agentsessionv1.AgentSessionSpec{
		RemediationRequestRef: agentsessionv1.ObjectRef{
			Name: analysis.Spec.RemediationRequestRef.Name,
			// AgentSession MUST be created in the same namespace as the RR;
			// corev1.ObjectReference.Namespace is not reliably populated for
			// same-namespace refs, so use AIAnalysis's own namespace instead
			// (every RO-created child CRD already lives in rr.Namespace).
			Namespace: analysis.Namespace,
		},
		IncidentID:         analysis.Name, // Q1: use CR name
		RemediationID:      correlationID,
		SignalName:         spec.SignalName,
		Severity:           spec.Severity,
		SignalSource:       "kubernaut",
		ResourceNamespace:  spec.TargetResource.Namespace,
		ResourceKind:       spec.TargetResource.Kind,
		ResourceName:       spec.TargetResource.Name,
		ResourceAPIVersion: spec.TargetResource.APIVersion,
		Environment:        spec.Environment,
		Priority:           spec.BusinessPriority,
		RiskTolerance:      getOrDefault(customLabels, "risk_tolerance", "medium"),
		BusinessCategory:   getOrDefault(customLabels, "business_category", "standard"),
		ClusterName:        clusterNameFor(analysis.Spec.ClusterID, customLabels),
		SignalMode:         spec.SignalMode,
		EnrichmentResults:  marshalEnrichmentResults(enrichment),
		// #2170 (DD-AA-KA-001 Amendment N): propagate verbatim so KA's
		// dispatcher can independently self-enforce the same absolute
		// deadline AA already enforces (checkInvestigationTimeout,
		// DD-TIMEOUT-002/#2176) -- the only way to bound KA's investigation
		// now that HTTP polling's CancelSession RPC is gone.
		TimesOutAt: analysis.Spec.TimesOutAt,
	}

	// #462: Forward signal annotations for alert-author context in KA prompt.
	if len(spec.SignalAnnotations) > 0 {
		out.SignalAnnotations = spec.SignalAnnotations
	}

	// BR-FLEET-003 (#1511): forward the optional cluster business classification,
	// omitted entirely (not even an empty string) for non-fleet/unregistered
	// clusters, same as BuildIncidentRequest's req.Cluster.SetTo guard.
	if spec.Cluster != "" {
		out.Cluster = spec.Cluster
	}

	return out
}

// marshalEnrichmentResults marshals the full sharedtypes.EnrichmentResults
// into raw JSON for AgentSessionSpec.EnrichmentResults. Unlike the retired
// ogen buildEnrichmentResults (which dropped KubernetesContext content via
// SetToNull()), this carries the complete struct -- a strict superset, never
// lossy. Returns nil when enrichment is empty or marshaling fails (never
// panics on a malformed result), same defensive pattern as mapping.go's
// marshalJSON on KA's side.
func marshalEnrichmentResults(enrichment sharedtypes.EnrichmentResults) *apiextensionsv1.JSON {
	if enrichment.KubernetesContext == nil && enrichment.BusinessClassification == nil {
		return nil
	}
	raw, err := json.Marshal(enrichment)
	if err != nil || len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// getCustomLabels returns CustomLabels from EnrichmentResults (Issue #113: now on KubernetesContext).
func getCustomLabels(enrichment sharedtypes.EnrichmentResults) map[string][]string {
	if enrichment.KubernetesContext != nil {
		return enrichment.KubernetesContext.CustomLabels
	}
	return nil
}

// getOrDefault gets a value from custom labels or returns default
func getOrDefault(labels map[string][]string, key, defaultVal string) string {
	if values, ok := labels[key]; ok && len(values) > 0 {
		return values[0]
	}
	return defaultVal
}

// clusterNameFor resolves AgentSessionSpec.ClusterName -- the raw cluster
// identifier KA uses to resolve its per-investigation fleet tool overlay via
// internal/kubernautagent/agentsession/mapping.go's MapSpecToSignal ->
// SignalContext.ClusterID (DD-FLEET-004).
//
// BR-FLEET-054: RemediationOrchestrator already propagates
// RemediationRequest.Spec.ClusterID onto AIAnalysis.Spec.ClusterID
// (pkg/remediationorchestrator/creator/aianalysis.go) for fleet-target
// signals -- that authoritative value takes priority here. The
// customLabels-derived "cluster_name" fallback is preserved for hub-local
// signals (clusterID empty) so existing non-fleet behavior is unchanged.
func clusterNameFor(clusterID string, customLabels map[string][]string) string {
	if clusterID != "" {
		return clusterID
	}
	return getOrDefault(customLabels, "cluster_name", "default")
}
