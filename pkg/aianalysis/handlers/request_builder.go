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
// P1.2 Refactoring: RequestBuilder extracts HolmesGPT-API request construction logic
// from InvestigatingHandler to improve maintainability and testability.
package handlers

import (
	"fmt"

	"github.com/go-logr/logr"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// RequestBuilder constructs HolmesGPT-API requests from AIAnalysis CRD specs.
// P1.2 Refactoring: Extracted from InvestigatingHandler for single responsibility.
//
// Responsibilities:
// - Map AIAnalysis CRD spec to HAPI OpenAPI generated types
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
// BR-AI-080: Build request with all required HAPI fields
// ========================================

// BuildIncidentRequest constructs an IncidentRequest from AIAnalysis CRD spec.
// Uses generated OpenAPI types for type-safe HAPI contract compliance.
//
// Parameters:
// - analysis: AIAnalysis CRD containing signal context and enrichment
//
// Returns:
// - *client.IncidentRequest: Type-safe request for HAPI /incident/analyze endpoint
func (b *RequestBuilder) BuildIncidentRequest(analysis *aianalysisv1.AIAnalysis) *client.IncidentRequest {
	spec := analysis.Spec.AnalysisRequest.SignalContext
	enrichment := spec.EnrichmentResults

	// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name for correlation consistency
	// Priority: RemediationRequestRef.Name (human-readable) > RemediationID (fallback for backward compatibility)
	correlationID := analysis.Spec.RemediationID // Fallback
	if analysis.Spec.RemediationRequestRef.Name != "" {
		correlationID = analysis.Spec.RemediationRequestRef.Name // Preferred
	}

	// BR-AI-080: Build request with all required HAPI fields using generated types
	req := &client.IncidentRequest{
		// REQUIRED fields per HAPI OpenAPI spec
		IncidentID:        analysis.Name,    // Q1: Use CR name
		RemediationID:     correlationID,    // DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name for audit correlation
		SignalType:        spec.SignalType,
		Severity:          client.Severity(spec.Severity),
		SignalSource:      "kubernaut",
		ResourceNamespace: spec.TargetResource.Namespace,
		ResourceKind:      spec.TargetResource.Kind,
		ResourceName:      spec.TargetResource.Name,
		ErrorMessage:      "", // Populated from enrichment if available
		Environment:       spec.Environment,
		Priority:          spec.BusinessPriority,
		RiskTolerance:     getOrDefault(enrichment.CustomLabels, "risk_tolerance", "medium"),
		BusinessCategory:  getOrDefault(enrichment.CustomLabels, "business_category", "standard"),
		ClusterName:       getOrDefault(enrichment.CustomLabels, "cluster_name", "default"),
	}

	// Map enrichment results for richer HolmesGPT-API context
	req.EnrichmentResults.SetTo(b.buildEnrichmentResults(enrichment))

	// BR-AI-084: Pass signal mode to HAPI for prompt strategy switching (ADR-054)
	// "reactive" triggers RCA investigation; "predictive" triggers predict & prevent strategy
	if spec.SignalMode != "" {
		req.SignalMode.SetTo(client.SignalMode(spec.SignalMode))
	}

	return req
}

// ========================================
// RECOVERY REQUEST CONSTRUCTION
// BR-AI-082: RecoveryRequest with previous execution context
// DD-RECOVERY-002: Direct recovery flow implementation
// ========================================

// BuildRecoveryRequest constructs a RecoveryRequest from AIAnalysis CRD spec.
// Uses generated OpenAPI types with opt pattern for optional fields.
//
// Parameters:
// - analysis: AIAnalysis CRD containing recovery context
//
// Returns:
// - *client.RecoveryRequest: Type-safe request for HAPI /recovery/analyze endpoint
func (b *RequestBuilder) BuildRecoveryRequest(analysis *aianalysisv1.AIAnalysis) *client.RecoveryRequest {
	spec := analysis.Spec.AnalysisRequest.SignalContext
	enrichment := spec.EnrichmentResults

	// DEBUG: Log what we're reading from the CRD
	b.log.Info("🔍 DEBUG: Reading from CRD",
		"crdName", analysis.Name,
		"spec.SignalType", spec.SignalType,
		"previousExecutionsCount", len(analysis.Spec.PreviousExecutions),
	)
	if len(analysis.Spec.PreviousExecutions) > 0 {
		b.log.Info("🔍 DEBUG: Previous execution signal type",
			"previousSignalType", analysis.Spec.PreviousExecutions[0].OriginalRCA.SignalType,
		)
	}

	// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name for correlation consistency
	// Priority: RemediationRequestRef.Name (human-readable) > RemediationID (fallback for backward compatibility)
	correlationID := analysis.Spec.RemediationID // Fallback
	if analysis.Spec.RemediationRequestRef.Name != "" {
		correlationID = analysis.Spec.RemediationRequestRef.Name // Preferred
	}

	req := &client.RecoveryRequest{
		// REQUIRED fields
		IncidentID:    analysis.Name,
		RemediationID: correlationID, // DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name for audit correlation
	}

	// Set recovery-specific fields using generated opt types
	req.IsRecoveryAttempt.SetTo(true)
	if analysis.Spec.RecoveryAttemptNumber > 0 {
		req.RecoveryAttemptNumber.SetTo(analysis.Spec.RecoveryAttemptNumber)
	}

	// Set environment fields using opt types
	req.Environment.SetTo(spec.Environment)
	req.Priority.SetTo(spec.BusinessPriority)
	req.RiskTolerance.SetTo(getOrDefault(enrichment.CustomLabels, "risk_tolerance", "medium"))
	req.BusinessCategory.SetTo(getOrDefault(enrichment.CustomLabels, "business_category", "standard"))

	// Optional signal context (may have changed since initial)
	// DEBUG: Log BEFORE SetTo
	b.log.Info("🔍 DEBUG: BEFORE SetTo",
		"spec.SignalType", spec.SignalType,
		"isEmpty", spec.SignalType == "",
		"req.SignalType.Set", req.SignalType.Set,
	)

	req.SignalType.SetTo(spec.SignalType)

	// DEBUG: Log AFTER SetTo
	b.log.Info("🔍 DEBUG: AFTER SetTo",
		"crdName", analysis.Name,
		"req.SignalType.Set", req.SignalType.Set,
		"req.SignalType.Value", req.SignalType.Value,
		"requestPointer", fmt.Sprintf("%p", req),
	)

	req.Severity.SetTo(client.Severity(spec.Severity))
	req.ResourceNamespace.SetTo(spec.TargetResource.Namespace)
	req.ResourceKind.SetTo(spec.TargetResource.Kind)
	req.ResourceName.SetTo(spec.TargetResource.Name)

	// DEBUG: Log signal type being sent to HAPI (BR-HAPI-197 investigation)
	b.log.Info("🔍 DEBUG: Building recovery request",
		"signalType", spec.SignalType,
		"signalTypeQuoted", fmt.Sprintf("%q", spec.SignalType),
		"isRecoveryAttempt", true,
		"recoveryAttemptNumber", analysis.Spec.RecoveryAttemptNumber,
	)

	// Map previous execution context (most recent only - API supports single execution)
	if len(analysis.Spec.PreviousExecutions) > 0 {
		// Use the most recent execution (last in the array)
		mostRecent := analysis.Spec.PreviousExecutions[len(analysis.Spec.PreviousExecutions)-1]
		req.PreviousExecution.SetTo(b.buildPreviousExecution(mostRecent))
	}

	return req
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// buildPreviousExecution maps CRD PreviousExecution to client.PreviousExecution
func (b *RequestBuilder) buildPreviousExecution(prev aianalysisv1.PreviousExecution) client.PreviousExecution {
	// Map OriginalRCA
	originalRCA := client.OriginalRCA{
		Summary:             prev.OriginalRCA.Summary,
		SignalType:          prev.OriginalRCA.SignalType,
		Severity:            client.Severity(prev.OriginalRCA.Severity),
		ContributingFactors: prev.OriginalRCA.ContributingFactors,
	}

	// Map SelectedWorkflow
	selectedWorkflow := client.SelectedWorkflowSummary{
		WorkflowID:     prev.SelectedWorkflow.WorkflowID,
		Version:        prev.SelectedWorkflow.Version,
		ContainerImage: prev.SelectedWorkflow.ContainerImage,
		Rationale:      prev.SelectedWorkflow.Rationale,
	}
	// Note: Parameters mapping happens in ResponseProcessor when reading from HAPI

	// Map ExecutionFailure
	failure := client.ExecutionFailure{
		FailedStepIndex: prev.Failure.FailedStepIndex,
		FailedStepName:  prev.Failure.FailedStepName,
		Reason:          prev.Failure.Reason,
		Message:         prev.Failure.Message,
		FailedAt:        prev.Failure.FailedAt.Time.Format("2006-01-02T15:04:05Z07:00"), // ISO 8601
		ExecutionTime:   prev.Failure.ExecutionTime,
	}

	// Map ExitCode if present
	if prev.Failure.ExitCode != nil {
		failure.ExitCode.SetTo(int(*prev.Failure.ExitCode))
	}

	return client.PreviousExecution{
		WorkflowExecutionRef: prev.WorkflowExecutionRef,
		OriginalRca:          originalRCA,
		SelectedWorkflow:     selectedWorkflow,
		Failure:              failure,
		// NaturalLanguageSummary is optional and provided by WorkflowExecution
	}
}

// buildEnrichmentResults maps shared EnrichmentResults to client.EnrichmentResults
func (b *RequestBuilder) buildEnrichmentResults(enrichment sharedtypes.EnrichmentResults) client.EnrichmentResults {
	result := client.EnrichmentResults{}

	// Map DetectedLabels if present
	if enrichment.DetectedLabels != nil {
		dl := enrichment.DetectedLabels
		detectedLabels := client.DetectedLabels{
			FailedDetections: dl.FailedDetections,
		}

		// Map boolean fields using OptBool
		detectedLabels.GitOpsManaged.SetTo(dl.GitOpsManaged)
		detectedLabels.PdbProtected.SetTo(dl.PDBProtected)
		detectedLabels.HpaEnabled.SetTo(dl.HPAEnabled)
		detectedLabels.Stateful.SetTo(dl.Stateful)
		detectedLabels.HelmManaged.SetTo(dl.HelmManaged)
		detectedLabels.NetworkIsolated.SetTo(dl.NetworkIsolated)

		// Map string fields using OptString
		if dl.GitOpsTool != "" {
			detectedLabels.GitOpsTool.SetTo(dl.GitOpsTool)
		}
		if dl.ServiceMesh != "" {
			detectedLabels.ServiceMesh.SetTo(dl.ServiceMesh)
		}

		result.DetectedLabels.SetTo(detectedLabels)
	}

	// Map CustomLabels if present
	if len(enrichment.CustomLabels) > 0 {
		// client.EnrichmentResultsCustomLabels is map[string][]string
		customLabels := client.EnrichmentResultsCustomLabels(enrichment.CustomLabels)
		result.CustomLabels.SetTo(customLabels)
	}

	// Map KubernetesContext if present (simplified - core fields only)
	// client.EnrichmentResultsKubernetesContext is map[string]jx.Raw
	// Note: Full mapping of all KubernetesContext fields can be added as needed
	if enrichment.KubernetesContext != nil {
		// For now, pass through essential fields only
		// HolmesGPT-API can handle the structured types or use default processing
		// Future: Complete mapping of PodDetails, DeploymentDetails, NodeDetails, etc.
		result.KubernetesContext.SetToNull() // Mark as present but empty for now
	}

	// ADR-055: OwnerChain mapping removed. Context enrichment (owner chain,
	// spec hash, remediation history) is now performed post-RCA by the LLM
	// via the get_resource_context tool.

	// EnrichmentQuality is not set (removed per Dec 2, 2025 decision)

	return result
}

// getOrDefault gets a value from custom labels or returns default
func getOrDefault(labels map[string][]string, key, defaultVal string) string {
	if values, ok := labels[key]; ok && len(values) > 0 {
		return values[0]
	}
	return defaultVal
}
