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
	"github.com/jordigilh/kubernaut/pkg/agentclient"
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
// INCIDENT REQUEST CONSTRUCTION
// BR-AI-080: Build request with all required KA fields
// ========================================

// BuildIncidentRequest constructs an IncidentRequest from AIAnalysis CRD spec.
// Uses generated OpenAPI types for type-safe KA contract compliance.
//
// Parameters:
// - analysis: AIAnalysis CRD containing signal context and enrichment
//
// Returns:
// - *client.IncidentRequest: Type-safe request for KA /incident/analyze endpoint
func (b *RequestBuilder) BuildIncidentRequest(analysis *aianalysisv1.AIAnalysis) *agentclient.IncidentRequest {
	spec := analysis.Spec.AnalysisRequest.SignalContext
	enrichment := spec.EnrichmentResults

	// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name for correlation consistency
	// Priority: RemediationRequestRef.Name (human-readable) > RemediationID (fallback for backward compatibility)
	correlationID := analysis.Spec.RemediationID // Fallback
	if analysis.Spec.RemediationRequestRef.Name != "" {
		correlationID = analysis.Spec.RemediationRequestRef.Name // Preferred
	}

	// BR-AI-080: Build request with all required KA fields using generated types
	// Issue #113: CustomLabels now on enrichment.KubernetesContext
	customLabels := getCustomLabels(enrichment)
	req := &agentclient.IncidentRequest{
		// REQUIRED fields per KA OpenAPI spec
		IncidentID:         analysis.Name, // Q1: Use CR name
		RemediationID:      correlationID, // DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name for audit correlation
		SignalName:         spec.SignalName,
		Severity:           agentclient.Severity(spec.Severity),
		SignalSource:       "kubernaut",
		ResourceNamespace:  spec.TargetResource.Namespace,
		ResourceKind:       spec.TargetResource.Kind,
		ResourceName:       spec.TargetResource.Name,
		ResourceAPIVersion: spec.TargetResource.APIVersion, // #2064: was omitted, KA never received it
		ErrorMessage:       "",                             // Populated from enrichment if available
		Environment:        spec.Environment,
		Priority:           spec.BusinessPriority,
		RiskTolerance:      getOrDefault(customLabels, "risk_tolerance", "medium"),
		BusinessCategory:   getOrDefault(customLabels, "business_category", "standard"),
		ClusterName:        clusterNameFor(analysis.Spec.ClusterID, customLabels),
	}

	// Map enrichment results for richer KA context
	req.EnrichmentResults.SetTo(b.buildEnrichmentResults(enrichment))

	// BR-AI-084: Pass signal mode to KA for prompt strategy switching (ADR-054)
	// "reactive" triggers RCA investigation; "proactive" triggers proactive prevention strategy
	if spec.SignalMode != "" {
		req.SignalMode.SetTo(agentclient.SignalMode(spec.SignalMode))
	}

	// #462: Forward signal annotations for alert-author context in KA prompt
	if len(spec.SignalAnnotations) > 0 {
		req.SignalAnnotations.SetTo(agentclient.IncidentRequestSignalAnnotations(spec.SignalAnnotations))
	}

	// BR-FLEET-003 (#1511): forward the optional cluster business classification.
	// Distinct from ClusterName above (the raw cluster identifier) -- omitted
	// entirely (not even an empty string) for non-fleet deployments/unregistered
	// clusters so KA's discovery tool call sends no `cluster` filter, per the
	// mandatory-field matching semantics in DD-FLEET-002.
	if spec.Cluster != "" {
		req.Cluster.SetTo(spec.Cluster)
	}

	return req
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

// buildEnrichmentResults maps shared EnrichmentResults to client.EnrichmentResults
func (b *RequestBuilder) buildEnrichmentResults(enrichment sharedtypes.EnrichmentResults) agentclient.EnrichmentResults {
	result := agentclient.EnrichmentResults{}

	// ADR-056: DetectedLabels removed from EnrichmentResults.
	// DetectedLabels are now computed by KA post-RCA and returned
	// in the response (stored in PostRCAContext).

	// Map CustomLabels if present (Issue #113: CustomLabels now on KubernetesContext)
	if enrichment.KubernetesContext != nil && len(enrichment.KubernetesContext.CustomLabels) > 0 {
		customLabels := agentclient.EnrichmentResultsCustomLabels(enrichment.KubernetesContext.CustomLabels)
		result.CustomLabels.SetTo(customLabels)
	}

	// Map KubernetesContext if present (simplified - core fields only)
	// client.EnrichmentResultsKubernetesContext is map[string]jx.Raw
	// Note: Full mapping of all KubernetesContext fields can be added as needed
	if enrichment.KubernetesContext != nil {
		// For now, pass through essential fields only
		// KA can handle the structured types or use default processing
		// Future: Complete mapping of PodDetails, DeploymentDetails, NodeDetails, etc.
		result.KubernetesContext.SetToNull() // Mark as present but empty for now
	}

	// Map BusinessClassification if present (BR-SP-002, BR-SP-080, BR-SP-081)
	if enrichment.BusinessClassification != nil {
		bc := enrichment.BusinessClassification
		clientBC := agentclient.BusinessClassification{}
		if bc.BusinessUnit != "" {
			clientBC.BusinessUnit.SetTo(bc.BusinessUnit)
		}
		if bc.ServiceOwner != "" {
			clientBC.ServiceOwner.SetTo(bc.ServiceOwner)
		}
		if bc.Criticality != "" {
			clientBC.Criticality.SetTo(string(bc.Criticality))
		}
		if bc.SLARequirement != "" {
			clientBC.SlaRequirement.SetTo(string(bc.SLARequirement))
		}
		result.BusinessClassification.SetTo(clientBC)
	}

	return result
}

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

// clusterNameFor resolves the wire IncidentRequest's cluster_name field --
// documented (openapi.json) as "the raw cluster identifier" KA uses to
// resolve its per-investigation fleet tool overlay via
// MapIncidentRequestToSignal -> SignalContext.ClusterID
// (internal/kubernautagent/server/handler.go, DD-FLEET-005).
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
