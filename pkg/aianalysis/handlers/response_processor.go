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
// This file contains response processing logic extracted from investigating.go.
//
// Refactoring: P1.1 - Extract ResponseProcessor (Dec 20, 2025)
// Purpose: Separate response processing concerns from handler orchestration
// Benefits: Improved testability, reduced file size, single responsibility
package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

// ResponseProcessor handles processing of HolmesGPT-API responses
// BR-AI-008: Capture all response fields including RCA, workflow, and alternatives
// BR-HAPI-197: Check needs_human_review before proceeding
// BR-HAPI-200: Handle problem_resolved outcomes
type ResponseProcessor struct {
	log     logr.Logger
	metrics *metrics.Metrics
}

// NewResponseProcessor creates a new ResponseProcessor
func NewResponseProcessor(log logr.Logger, m *metrics.Metrics) *ResponseProcessor {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	return &ResponseProcessor{
		log:     log.WithName("response-processor"),
		metrics: m,
	}
}

// ProcessIncidentResponse processes the IncidentResponse from generated client
// BR-AI-009: Reset failure counter on successful API call
// BR-HAPI-197: Check needs_human_review before proceeding
func (p *ResponseProcessor) ProcessIncidentResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	// BR-AI-009: Reset failure counter on successful API call
	analysis.Status.ConsecutiveFailures = 0

	// Check if NeedsHumanReview is set
	needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null

	p.log.Info("Processing successful incident response",
		"confidence", resp.Confidence,
		"warningsCount", len(resp.Warnings),
		"hasSelectedWorkflow", hasSelectedWorkflow,
		"needsHumanReview", needsHumanReview,
	)

	// BR-AI-OBSERVABILITY-004: Record confidence score for AI quality tracking
	p.metrics.RecordConfidenceScore(analysis.Spec.AnalysisRequest.SignalContext.SignalType, resp.Confidence)

	// BR-HAPI-197: Check if workflow resolution failed
	if needsHumanReview {
		return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
	}

	// BR-HAPI-200 Outcome A: Problem confidently resolved, no workflow needed
	if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
		return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
	}

	// Store HAPI response metadata
	analysis.Status.Warnings = resp.Warnings
	analysis.Status.InvestigationID = resp.IncidentID

	// Store TargetInOwnerChain (data quality indicator for policy evaluation)
	if resp.TargetInOwnerChain.Set {
		targetInChain := resp.TargetInOwnerChain.Value
		analysis.Status.TargetInOwnerChain = &targetInChain
	}

	// Store root cause analysis (if present) - convert from map[string]jx.Raw
	if len(resp.RootCauseAnalysis) > 0 {
		rcaMap := GetMapFromOptNil(resp.RootCauseAnalysis)
		if rcaMap != nil {
			analysis.Status.RootCause = GetStringFromMap(rcaMap, "summary")
			analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:             GetStringFromMap(rcaMap, "summary"),
				Severity:            GetStringFromMap(rcaMap, "severity"),
				ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),
			}
		}
	}

	// Store selected workflow (DD-CONTRACT-002)
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			sw := &aianalysisv1.SelectedWorkflow{
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				Version:         GetStringFromMap(swMap, "version"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				ContainerDigest: GetStringFromMap(swMap, "container_digest"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
			}
			// Map parameters if present (map[string]string)
			if paramsRaw, ok := swMap["parameters"]; ok {
				if paramsMapIface, ok := paramsRaw.(map[string]interface{}); ok {
					sw.Parameters = convertMapToStringMap(paramsMapIface)
				}
			}
			analysis.Status.SelectedWorkflow = sw
		}
	}

	// Store alternative workflows (INFORMATIONAL ONLY - NOT for execution)
	if len(resp.AlternativeWorkflows) > 0 {
		alternatives := make([]aianalysisv1.AlternativeWorkflow, 0, len(resp.AlternativeWorkflows))
		for _, alt := range resp.AlternativeWorkflows {
			containerImage := ""
			if alt.ContainerImage.Set && !alt.ContainerImage.Null {
				containerImage = alt.ContainerImage.Value
			}
			alternatives = append(alternatives, aianalysisv1.AlternativeWorkflow{
				WorkflowID:     alt.WorkflowID,
				ContainerImage: containerImage,
				Confidence:     alt.Confidence,
				Rationale:      alt.Rationale,
			})
		}
		analysis.Status.AlternativeWorkflows = alternatives
	}

	// Set InvestigationComplete condition
	aianalysis.SetInvestigationComplete(analysis, true, "HolmesGPT-API investigation completed successfully")

	// Transition to Analyzing phase
	// Note: ObservedGeneration set by InvestigatingHandler before returning
	analysis.Status.Phase = aianalysis.PhaseAnalyzing
	analysis.Status.Message = "Investigation complete, starting analysis"

	// DD-AUDIT-003: Phase transition audit recorded by InvestigatingHandler (investigating.go:177)
	// NOT recorded here to avoid duplicates - handler records after status is committed

	return ctrl.Result{Requeue: true}, nil
}

// ProcessRecoveryResponse processes the RecoveryResponse from generated client
// BR-AI-082: Handle recovery flow responses
// BR-HAPI-197: Check needs_human_review before proceeding
func (p *ResponseProcessor) ProcessRecoveryResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	// BR-AI-009: Reset failure counter on successful API call
	analysis.Status.ConsecutiveFailures = 0

	// Check if NeedsHumanReview is set (BR-HAPI-197)
	needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null

	// DEBUG: Log raw field values for BR-HAPI-197 investigation
	p.log.Info("🔍 DEBUG: Recovery response received from HAPI",
		"NeedsHumanReview.Set", resp.NeedsHumanReview.Set,
		"NeedsHumanReview.Value", resp.NeedsHumanReview.Value,
		"HumanReviewReason.Set", resp.HumanReviewReason.Set,
		"HumanReviewReason.Null", resp.HumanReviewReason.Null,
		"HumanReviewReason.Value", resp.HumanReviewReason.Value,
		"needsHumanReview_computed", needsHumanReview,
	)

	p.log.Info("Processing successful recovery response",
		"canRecover", resp.CanRecover,
		"confidence", resp.AnalysisConfidence,
		"warningsCount", len(resp.Warnings),
		"hasSelectedWorkflow", hasSelectedWorkflow,
		"needsHumanReview", needsHumanReview,
	)

	// BR-AI-OBSERVABILITY-004: Record confidence score for AI quality tracking
	p.metrics.RecordConfidenceScore(analysis.Spec.AnalysisRequest.SignalContext.SignalType, resp.AnalysisConfidence)

	// BR-HAPI-197: Check if recovery requires human review
	// This takes precedence over other checks as HAPI has determined it cannot provide reliable recommendations
	if needsHumanReview {
		return p.handleWorkflowResolutionFailureFromRecovery(ctx, analysis, resp)
	}

	// Check if recovery is not possible
	if !resp.CanRecover {
		return p.handleRecoveryNotPossible(ctx, analysis, resp)
	}

	// Check if no workflow was selected (might need human review)
	if !hasSelectedWorkflow {
		return p.handleRecoveryNotPossible(ctx, analysis, resp)
	}

	// Store HAPI response metadata
	analysis.Status.Warnings = resp.Warnings
	analysis.Status.InvestigationID = resp.IncidentID

	// Store selected workflow (DD-CONTRACT-002)
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			sw := &aianalysisv1.SelectedWorkflow{
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				Version:         GetStringFromMap(swMap, "version"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				ContainerDigest: GetStringFromMap(swMap, "container_digest"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
			}
			// Map parameters if present (map[string]string)
			if paramsRaw, ok := swMap["parameters"]; ok {
				if paramsMapIface, ok := paramsRaw.(map[string]interface{}); ok {
					sw.Parameters = convertMapToStringMap(paramsMapIface)
				}
			}
			analysis.Status.SelectedWorkflow = sw
		}
	}

	// Set InvestigationComplete condition
	aianalysis.SetInvestigationComplete(analysis, true, "HolmesGPT-API recovery investigation completed successfully")

	// Transition to Analyzing phase
	// Note: ObservedGeneration set by InvestigatingHandler before returning
	analysis.Status.Phase = aianalysis.PhaseAnalyzing
	analysis.Status.Message = "Recovery investigation complete, starting analysis"

	// DD-AUDIT-003: Phase transition audit recorded by InvestigatingHandler (investigating.go:177)
	// NOT recorded here to avoid duplicates - handler records after status is committed

	return ctrl.Result{Requeue: true}, nil
}

// PopulateRecoveryStatusFromRecovery populates RecoveryStatus from RecoveryResponse
// BR-AI-082: Extract recovery_analysis from HAPI response
func (p *ResponseProcessor) PopulateRecoveryStatusFromRecovery(analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) bool {
	if !resp.RecoveryAnalysis.Set || resp.RecoveryAnalysis.Null {
		return false
	}

	// Convert recovery_analysis from map[string]jx.Raw
	raMap := GetMapFromOptNil(resp.RecoveryAnalysis.Value)
	if raMap == nil {
		return false
	}

	// Initialize RecoveryStatus per CRD schema
	analysis.Status.RecoveryStatus = &aianalysisv1.RecoveryStatus{}

	// Extract PreviousAttemptAssessment (matches CRD fields)
	if prevMap := GetMapFromMapSafe(raMap, "previous_attempt_assessment"); prevMap != nil {
		analysis.Status.RecoveryStatus.PreviousAttemptAssessment = &aianalysisv1.PreviousAttemptAssessment{
			FailureUnderstood:     GetBoolFromMap(prevMap, "failure_understood"),
			FailureReasonAnalysis: GetStringFromMap(prevMap, "failure_reason_analysis"),
		}
	}

	// Extract state change information
	if raMap["state_changed"] == true {
		analysis.Status.RecoveryStatus.StateChanged = true
	}
	if currentSignal := GetStringFromMap(raMap, "current_signal_type"); currentSignal != "" {
		analysis.Status.RecoveryStatus.CurrentSignalType = currentSignal
	}

	return true
}

// handleWorkflowResolutionFailureFromIncident handles workflow resolution failure from IncidentResponse
// BR-HAPI-197: Workflow resolution failed, human must intervene
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromIncident(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null
	humanReviewReason := ""
	if resp.HumanReviewReason.Set && !resp.HumanReviewReason.Null {
		humanReviewReason = string(resp.HumanReviewReason.Value)
	}

	p.log.Info("Workflow resolution failed, requires human review",
		"warnings", resp.Warnings,
		"humanReviewReason", humanReviewReason,
		"hasPartialWorkflow", hasSelectedWorkflow,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = "WorkflowResolutionFailed"
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

	// Record failure metric
	subReason := "HumanReviewRequired"
	if resp.HumanReviewReason.IsSet() {
		subReason = string(resp.HumanReviewReason.Value)
	}
	p.metrics.RecordFailure("WorkflowResolutionFailed", subReason)

	// Map HumanReviewReason enum to SubReason
	if humanReviewReason != "" {
		analysis.Status.SubReason = p.mapEnumToSubReason(humanReviewReason)
	} else {
		analysis.Status.SubReason = mapWarningsToSubReason(resp.Warnings)
	}

	// Handle ValidationAttemptsHistory from generated types
	var messageParts []string
	if len(resp.ValidationAttemptsHistory) > 0 {
		for _, genAttempt := range resp.ValidationAttemptsHistory {
			attempt := aianalysisv1.ValidationAttempt{
				Attempt: genAttempt.Attempt,
				IsValid: genAttempt.IsValid,
				Errors:  genAttempt.Errors,
			}
			if genAttempt.WorkflowID.Set {
				attempt.WorkflowID = genAttempt.WorkflowID.Value
			}
			// Parse timestamp string to metav1.Time
			if parsedTime, err := time.Parse(time.RFC3339, genAttempt.Timestamp); err == nil {
				attempt.Timestamp = metav1.NewTime(parsedTime)
			} else {
				// Fallback to current time if parsing fails
				attempt.Timestamp = metav1.Now()
			}
			analysis.Status.ValidationAttemptsHistory = append(analysis.Status.ValidationAttemptsHistory, attempt)

			// Build operator-friendly message from validation attempts
			if len(genAttempt.Errors) > 0 {
				messageParts = append(messageParts, fmt.Sprintf("Attempt %d: %s", genAttempt.Attempt, strings.Join(genAttempt.Errors, ", ")))
			}
		}
	}

	// Build message: validation attempts + warnings
	if len(messageParts) > 0 {
		analysis.Status.Message = strings.Join(messageParts, "; ")
		if len(resp.Warnings) > 0 {
			analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
		}
	} else {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Preserve partial response if available
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     GetStringFromMap(swMap, "workflow_id"),
				ContainerImage: GetStringFromMap(swMap, "container_image"),
				Confidence:     GetFloat64FromMap(swMap, "confidence"),
				Rationale:      GetStringFromMap(swMap, "rationale"),
			}
		}
	}

	// Preserve RCA if available
	if len(resp.RootCauseAnalysis) > 0 {
		rcaMap := GetMapFromOptNil(resp.RootCauseAnalysis)
		if rcaMap != nil {
			analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:             GetStringFromMap(rcaMap, "summary"),
				Severity:            GetStringFromMap(rcaMap, "severity"),
				ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),
			}
		}
	}

	return ctrl.Result{}, nil // Terminal - no requeue
}

// handleProblemResolvedFromIncident handles problem self-resolved from IncidentResponse
// BR-HAPI-200: Problem confirmed resolved, no workflow needed
func (p *ResponseProcessor) handleProblemResolvedFromIncident(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	p.log.Info("Problem confirmed resolved, no workflow needed",
		"confidence", resp.Confidence,
		"warnings", resp.Warnings,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseCompleted
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = "WorkflowNotNeeded"
	analysis.Status.SubReason = "ProblemResolved"
	analysis.Status.InvestigationID = resp.IncidentID

	if resp.Analysis != "" {
		analysis.Status.Message = resp.Analysis
	} else if len(resp.Warnings) > 0 {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	} else {
		analysis.Status.Message = "Problem self-resolved. No remediation required."
	}

	analysis.Status.Warnings = resp.Warnings

	// Store RCA if available
	if len(resp.RootCauseAnalysis) > 0 {
		rcaMap := GetMapFromOptNil(resp.RootCauseAnalysis)
		if rcaMap != nil {
			analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:             GetStringFromMap(rcaMap, "summary"),
				Severity:            GetStringFromMap(rcaMap, "severity"),
				ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),
			}
		}
	}

	return ctrl.Result{}, nil
}

// handleWorkflowResolutionFailureFromRecovery handles workflow resolution failure from RecoveryResponse
// BR-HAPI-197: Recovery workflow resolution failed, human must intervene
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromRecovery(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null
	humanReviewReason := ""
	if resp.HumanReviewReason.Set && !resp.HumanReviewReason.Null {
		humanReviewReason = resp.HumanReviewReason.Value
	}

	p.log.Info("Recovery workflow resolution failed, requires human review",
		"warnings", resp.Warnings,
		"humanReviewReason", humanReviewReason,
		"hasPartialWorkflow", hasSelectedWorkflow,
		"canRecover", resp.CanRecover,
		"confidence", resp.AnalysisConfidence,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = "WorkflowResolutionFailed"
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

	// Record failure metric
	subReason := "HumanReviewRequired"
	if humanReviewReason != "" {
		subReason = humanReviewReason
	}
	p.metrics.RecordFailure("WorkflowResolutionFailed", subReason)

	// Map HumanReviewReason to SubReason
	if humanReviewReason != "" {
		analysis.Status.SubReason = p.mapEnumToSubReason(humanReviewReason)
	} else {
		analysis.Status.SubReason = mapWarningsToSubReason(resp.Warnings)
	}

	// Build comprehensive message
	var messageParts []string
	messageParts = append(messageParts, "HolmesGPT-API could not provide reliable recovery workflow recommendation")

	if humanReviewReason != "" {
		messageParts = append(messageParts, fmt.Sprintf("(reason: %s)", humanReviewReason))
	}

	if len(resp.Warnings) > 0 {
		messageParts = append(messageParts, fmt.Sprintf("Warnings: %s", strings.Join(resp.Warnings, "; ")))
	}

	analysis.Status.Message = strings.Join(messageParts, " ")
	analysis.Status.Warnings = resp.Warnings

	return ctrl.Result{}, nil
}

// handleRecoveryNotPossible handles when recovery is not possible
// BR-AI-082: Recovery flow - no recovery strategy available
func (p *ResponseProcessor) handleRecoveryNotPossible(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	p.log.Info("Recovery not possible",
		"confidence", resp.AnalysisConfidence,
		"warnings", resp.Warnings,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = "RecoveryNotPossible"
	analysis.Status.SubReason = "NoRecoveryStrategy"

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("RecoveryNotPossible", "NoRecoveryStrategy").Inc()

	// Record failure metric
	p.metrics.RecordFailure("RecoveryNotPossible", "NoRecoveryStrategy")
	analysis.Status.InvestigationID = resp.IncidentID
	analysis.Status.Message = "HAPI determined recovery is not possible for this failure"
	analysis.Status.Warnings = resp.Warnings

	return ctrl.Result{}, nil
}

// mapEnumToSubReason maps HAPI HumanReviewReason enum to CRD SubReason
// This is the preferred method - direct enum-to-enum mapping (Dec 6, 2025)
// Updated Dec 7, 2025: Added investigation_inconclusive per BR-HAPI-200
func (p *ResponseProcessor) mapEnumToSubReason(reason string) string {
	mapping := map[string]string{
		"workflow_not_found":          "WorkflowNotFound",
		"image_mismatch":              "ImageMismatch",
		"parameter_validation_failed": "ParameterValidationFailed",
		"no_matching_workflows":       "NoMatchingWorkflows",
		"low_confidence":              "LowConfidence",
		"llm_parsing_error":           "LLMParsingError",
		"investigation_inconclusive":  "InvestigationInconclusive", // BR-HAPI-200
	}
	if subReason, ok := mapping[reason]; ok {
		return subReason
	}
	p.log.Info("Unknown human_review_reason, defaulting to WorkflowNotFound", "reason", reason)
	return "WorkflowNotFound"
}

// mapWarningsToSubReason extracts SubReason from HAPI warnings
// DEPRECATED: Use mapEnumToSubReason when HumanReviewReason is available
// Kept for backward compatibility with older HAPI versions
func mapWarningsToSubReason(warnings []string) string {
	warningsStr := strings.ToLower(strings.Join(warnings, " "))

	switch {
	case strings.Contains(warningsStr, "not found") || strings.Contains(warningsStr, "does not exist"):
		return "WorkflowNotFound"
	case strings.Contains(warningsStr, "no workflows matched") || strings.Contains(warningsStr, "no matching"):
		return "NoMatchingWorkflows"
	case strings.Contains(warningsStr, "confidence") && strings.Contains(warningsStr, "below"):
		return "LowConfidence"
	case strings.Contains(warningsStr, "parameter validation") || strings.Contains(warningsStr, "missing required"):
		return "ParameterValidationFailed"
	case strings.Contains(warningsStr, "image mismatch") || strings.Contains(warningsStr, "container image"):
		return "ImageMismatch"
	case strings.Contains(warningsStr, "parse") || strings.Contains(warningsStr, "invalid json"):
		return "LLMParsingError"
	default:
		return "WorkflowNotFound" // Default to most common case
	}
}

