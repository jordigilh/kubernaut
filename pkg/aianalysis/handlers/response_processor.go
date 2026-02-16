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
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// ResponseProcessor handles processing of HolmesGPT-API responses
// BR-AI-008: Capture all response fields including RCA, workflow, and alternatives
// BR-HAPI-197: Check needs_human_review before proceeding
// BR-HAPI-200: Handle problem_resolved outcomes
type ResponseProcessor struct {
	log         logr.Logger
	metrics     *metrics.Metrics
	auditClient AuditClientInterface
}

// NewResponseProcessor creates a new ResponseProcessor
func NewResponseProcessor(log logr.Logger, m *metrics.Metrics, auditClient AuditClientInterface) *ResponseProcessor {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	return &ResponseProcessor{
		log:         log.WithName("response-processor"),
		metrics:     m,
		auditClient: auditClient,
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

	// BR-HAPI-197: Check if HAPI explicitly requires human review (Layer 1 - Primary)
	// CRITICAL: This MUST be checked FIRST. HAPI's explicit needs_human_review=true
	// takes priority over all other classification logic.
	if needsHumanReview {
		return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
	}

	// BR-HAPI-200.6 Outcome A: Problem confidently resolved, no workflow needed
	// Detection per BR-HAPI-200.6: needs_human_review=false AND selected_workflow=null AND confidence >= 0.7
	// Defense-in-depth (Layer 2): also verify no warning signals that indicate an active
	// problem (inconclusive investigation, no matching workflows). This catches edge cases
	// where the LLM incorrectly overrides needs_human_review=false but HAPI still appends
	// diagnostic warnings from its investigation_outcome parsing.
	if !hasSelectedWorkflow && resp.Confidence >= 0.7 && !hasNoWorkflowWarningSignal(resp.Warnings) {
		return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
	}

	// BR-AI-050 + Issue #29: No workflow found (terminal failure requiring human review)
	// Reached when: (a) confidence < 0.7 with no workflow, OR
	// (b) confidence >= 0.7 but warning signals indicate an active problem (defense-in-depth)
	if !hasSelectedWorkflow {
		return p.handleNoWorkflowTerminalFailure(ctx, analysis, resp)
	}

	// BR-HAPI-197 AC-4 + Issue #28: AIAnalysis applies confidence threshold (V1.0: 70%)
	// HAPI returns confidence but does NOT enforce thresholds - AIAnalysis owns this logic
	const confidenceThreshold = 0.7 // TODO V1.1: Make configurable per BR-HAPI-198

	if hasSelectedWorkflow && resp.Confidence < confidenceThreshold {
		return p.handleLowConfidenceFailure(ctx, analysis, resp)
	}

	// All checks passed - store HAPI response metadata and continue processing
	analysis.Status.Warnings = resp.Warnings
	analysis.Status.InvestigationID = resp.IncidentID

	// Store TargetInOwnerChain (data quality indicator for policy evaluation)
	if resp.TargetInOwnerChain.Set {
		targetInChain := resp.TargetInOwnerChain.Value
		analysis.Status.TargetInOwnerChain = &targetInChain
	}

	// Store root cause analysis (if present) - Issue #97: uses centralized helper with affectedResource
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCause = rca.Summary
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// Store selected workflow (DD-CONTRACT-002)
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			sw := &aianalysisv1.SelectedWorkflow{
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				ActionType:      GetStringFromMap(swMap, "action_type"),
				Version:         GetStringFromMap(swMap, "version"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				ContainerDigest: GetStringFromMap(swMap, "container_digest"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
				ExecutionEngine: GetStringFromMap(swMap, "execution_engine"),
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

	// BR-HAPI-197: No human review needed for successful workflow selection
	analysis.Status.NeedsHumanReview = false

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

	// BR-HAPI-197: Check if recovery requires human review (validation failures)
	// This handles cases where HAPI flagged validation failures explicitly
	if needsHumanReview {
		return p.handleWorkflowResolutionFailureFromRecovery(ctx, analysis, resp)
	}

	// Check if recovery is not possible
	if !resp.CanRecover {
		return p.handleRecoveryNotPossible(ctx, analysis, resp)
	}

	// BR-AI-050 + Issue #29: No workflow found (terminal failure)
	// When confidence < 0.7 and no workflow, this is a terminal failure
	if !hasSelectedWorkflow {
		return p.handleNoWorkflowTerminalFailureFromRecovery(ctx, analysis, resp)
	}

	// BR-HAPI-197 AC-4 + Issue #28: AIAnalysis applies confidence threshold (V1.0: 70%)
	// HAPI returns confidence but does NOT enforce thresholds - AIAnalysis owns this logic
	const confidenceThreshold = 0.7 // TODO V1.1: Make configurable per BR-HAPI-198

	if hasSelectedWorkflow && resp.AnalysisConfidence < confidenceThreshold {
		return p.handleLowConfidenceFailureFromRecovery(ctx, analysis, resp)
	}

	// All checks passed - store HAPI response metadata and continue processing
	analysis.Status.Warnings = resp.Warnings
	analysis.Status.InvestigationID = resp.IncidentID

	// Store selected workflow (DD-CONTRACT-002)
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			sw := &aianalysisv1.SelectedWorkflow{
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				ActionType:      GetStringFromMap(swMap, "action_type"),
				Version:         GetStringFromMap(swMap, "version"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				ContainerDigest: GetStringFromMap(swMap, "container_digest"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
				ExecutionEngine: GetStringFromMap(swMap, "execution_engine"),
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

	// BR-HAPI-197: Store human review flag and reason in CRD status
	analysis.Status.NeedsHumanReview = true
	if humanReviewReason != "" {
		analysis.Status.HumanReviewReason = humanReviewReason
	}

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

	// Record failure metric
	subReason := "HumanReviewRequired"
	if resp.HumanReviewReason.IsSet() {
		subReason = string(resp.HumanReviewReason.Value)
	}
	p.metrics.RecordFailure("WorkflowResolutionFailed", subReason)

	// DD-AUDIT-003: Record analysis failure audit event
	failureErr := fmt.Errorf("workflow resolution failed: %s", humanReviewReason)
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

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
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
				ExecutionEngine: GetStringFromMap(swMap, "execution_engine"),
			}
		}
	}

	// Preserve RCA if available - Issue #97: uses centralized helper with affectedResource
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
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

	// BR-HAPI-197: No human review needed for resolved problems
	analysis.Status.NeedsHumanReview = false

	if resp.Analysis != "" {
		analysis.Status.Message = resp.Analysis
	} else if len(resp.Warnings) > 0 {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	} else {
		analysis.Status.Message = "Problem self-resolved. No remediation required."
	}

	analysis.Status.Warnings = resp.Warnings

	// Store RCA if available - Issue #97: uses centralized helper with affectedResource
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// BR-HAPI-200: Record analysis completion audit event
	// This is a successful completion even though no workflow was selected
	p.auditClient.RecordAnalysisComplete(ctx, analysis)

	return ctrl.Result{}, nil
}

// handleNoWorkflowTerminalFailure handles terminal failure when no workflow selected with low confidence
// Issue #29: BR-AI-050 - AIAnalysis must detect terminal failure per BR-HAPI-197 AC-4
func (p *ResponseProcessor) handleNoWorkflowTerminalFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	p.log.Info("No workflow selected, terminal failure",
		"confidence", resp.Confidence,
		"warnings", resp.Warnings,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = aianalysis.ReasonWorkflowResolutionFailed
	analysis.Status.SubReason = "NoMatchingWorkflows" // Maps to CRD SubReason enum
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197 AC-4: AIAnalysis sets needs_human_review for terminal failures
	analysis.Status.NeedsHumanReview = true
	analysis.Status.HumanReviewReason = "no_matching_workflows"

	// Build operator-friendly message
	analysis.Status.Message = "No workflow selected for remediation"
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Store RCA if available (for human review context) - Issue #97: centralized helper
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// BR-AI-050: Emit audit event for terminal failure
	failureErr := fmt.Errorf("no workflow selected: no matching workflows found")
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoMatchingWorkflows").Inc()
	p.metrics.RecordFailure("WorkflowResolutionFailed", "NoMatchingWorkflows")

	return ctrl.Result{}, nil // Terminal - no requeue
}

// handleLowConfidenceFailure handles workflow selection with confidence below threshold
// Issue #28: BR-HAPI-197 AC-4 - AIAnalysis applies confidence threshold (not HAPI)
func (p *ResponseProcessor) handleLowConfidenceFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	const confidenceThreshold = 0.7 // V1.0: global 70% default

	p.log.Info("Low confidence workflow, requires human review",
		"confidence", resp.Confidence,
		"threshold", confidenceThreshold,
		"warnings", resp.Warnings,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = aianalysis.ReasonWorkflowResolutionFailed
	analysis.Status.SubReason = "LowConfidence" // Maps to CRD SubReason enum
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197 AC-4: AIAnalysis sets needs_human_review for low confidence
	analysis.Status.NeedsHumanReview = true
	analysis.Status.HumanReviewReason = "low_confidence"

	// Build operator-friendly message
	analysis.Status.Message = fmt.Sprintf("Workflow confidence %.2f below threshold %.2f (low_confidence)", resp.Confidence, confidenceThreshold)
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Store workflow info for human review (partial information for operator context)
	if resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				Version:         GetStringFromMap(swMap, "version"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				ContainerDigest: GetStringFromMap(swMap, "container_digest"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
				ExecutionEngine: GetStringFromMap(swMap, "execution_engine"),
			}
			// Map parameters if present
			if paramsRaw, ok := swMap["parameters"]; ok {
				if paramsMapIface, ok := paramsRaw.(map[string]interface{}); ok {
					analysis.Status.SelectedWorkflow.Parameters = convertMapToStringMap(paramsMapIface)
				}
			}
		}
	}

	// Store RCA if available (for human review context) - Issue #97: centralized helper
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// Store alternative workflows if available (for human review context)
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

	// BR-AI-050: Emit audit event for low confidence failure
	failureErr := fmt.Errorf("low confidence: %.2f below threshold %.2f", resp.Confidence, confidenceThreshold)
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LowConfidence").Inc()
	p.metrics.RecordFailure("WorkflowResolutionFailed", "LowConfidence")

	return ctrl.Result{}, nil // Terminal - no requeue
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

	// BR-HAPI-197: Store human review flag and reason in CRD status (recovery flow)
	analysis.Status.NeedsHumanReview = true
	if humanReviewReason != "" {
		analysis.Status.HumanReviewReason = humanReviewReason
	}

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

	// Record failure metric
	subReason := "HumanReviewRequired"
	if humanReviewReason != "" {
		subReason = humanReviewReason
	}
	p.metrics.RecordFailure("WorkflowResolutionFailed", subReason)

	// DD-AUDIT-003: Record analysis failure audit event
	failureErr := fmt.Errorf("recovery workflow resolution failed: %s", humanReviewReason)
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

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

// handleNoWorkflowTerminalFailureFromRecovery handles terminal failure when no workflow selected with low confidence (recovery flow)
// Issue #29: BR-AI-050 - AIAnalysis must detect terminal failure per BR-HAPI-197 AC-4
func (p *ResponseProcessor) handleNoWorkflowTerminalFailureFromRecovery(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	p.log.Info("No workflow selected, terminal failure (recovery flow)",
		"confidence", resp.AnalysisConfidence,
		"warnings", resp.Warnings,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = aianalysis.ReasonWorkflowResolutionFailed
	analysis.Status.SubReason = "NoMatchingWorkflows" // Maps to CRD SubReason enum
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197 AC-4: AIAnalysis sets needs_human_review for terminal failures
	analysis.Status.NeedsHumanReview = true
	analysis.Status.HumanReviewReason = "no_matching_workflows"

	// Build operator-friendly message
	analysis.Status.Message = "No recovery workflow selected for remediation"
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// BR-AI-050: Emit audit event for terminal failure
	failureErr := fmt.Errorf("no recovery workflow selected: no matching workflows found")
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoMatchingWorkflows").Inc()
	p.metrics.RecordFailure("WorkflowResolutionFailed", "NoMatchingWorkflows")

	return ctrl.Result{}, nil // Terminal - no requeue
}

// handleLowConfidenceFailureFromRecovery handles workflow selection with confidence below threshold (recovery flow)
// Issue #28: BR-HAPI-197 AC-4 - AIAnalysis applies confidence threshold (not HAPI)
func (p *ResponseProcessor) handleLowConfidenceFailureFromRecovery(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	const confidenceThreshold = 0.7 // V1.0: global 70% default

	p.log.Info("Low confidence recovery workflow, requires human review",
		"confidence", resp.AnalysisConfidence,
		"threshold", confidenceThreshold,
		"warnings", resp.Warnings,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = aianalysis.ReasonWorkflowResolutionFailed
	analysis.Status.SubReason = "LowConfidence" // Maps to CRD SubReason enum
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197 AC-4: AIAnalysis sets needs_human_review for low confidence
	analysis.Status.NeedsHumanReview = true
	analysis.Status.HumanReviewReason = "low_confidence"

	// Build operator-friendly message
	analysis.Status.Message = fmt.Sprintf("Recovery workflow confidence %.2f below threshold %.2f (low_confidence)", resp.AnalysisConfidence, confidenceThreshold)
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Store workflow info for human review (partial information for operator context)
	if resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:      GetStringFromMap(swMap, "workflow_id"),
				Version:         GetStringFromMap(swMap, "version"),
				ContainerImage:  GetStringFromMap(swMap, "container_image"),
				ContainerDigest: GetStringFromMap(swMap, "container_digest"),
				Confidence:      GetFloat64FromMap(swMap, "confidence"),
				Rationale:       GetStringFromMap(swMap, "rationale"),
				ExecutionEngine: GetStringFromMap(swMap, "execution_engine"),
			}
			// Map parameters if present
			if paramsRaw, ok := swMap["parameters"]; ok {
				if paramsMapIface, ok := paramsRaw.(map[string]interface{}); ok {
					analysis.Status.SelectedWorkflow.Parameters = convertMapToStringMap(paramsMapIface)
				}
			}
		}
	}

	// BR-AI-050: Emit audit event for low confidence failure
	failureErr := fmt.Errorf("low confidence recovery: %.2f below threshold %.2f", resp.AnalysisConfidence, confidenceThreshold)
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LowConfidence").Inc()
	p.metrics.RecordFailure("WorkflowResolutionFailed", "LowConfidence")

	return ctrl.Result{}, nil // Terminal - no requeue
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

	// BR-HAPI-197: Recovery not possible requires human review
	analysis.Status.NeedsHumanReview = true
	// Note: No HumanReviewReason set because HAPI determined recovery impossible (different from workflow resolution failure)

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("RecoveryNotPossible", "NoRecoveryStrategy").Inc()

	// Record failure metric
	p.metrics.RecordFailure("RecoveryNotPossible", "NoRecoveryStrategy")

	// DD-AUDIT-003: Record analysis failure audit event
	failureErr := fmt.Errorf("recovery not possible: no recovery strategy available")
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

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

// hasNoWorkflowWarningSignal checks if HAPI's warnings contain signals that indicate
// the absence of a selected workflow is due to an active problem (inconclusive investigation
// or no matching workflows in the catalog), NOT because the problem self-resolved.
//
// This is a defense-in-depth check (Layer 2) for BR-HAPI-200.6. It catches edge cases
// where the LLM incorrectly sets needs_human_review=false but HAPI's result_parser
// still appends diagnostic warnings from investigation_outcome processing.
//
// Warning signals checked (from HAPI result_parser.py):
//   - "inconclusive": investigation_outcome == "inconclusive"
//   - "no workflows matched": selected_workflow is None and outcome is not "resolved"
//   - "human review recommended": general HAPI safety signal
func hasNoWorkflowWarningSignal(warnings []string) bool {
	for _, w := range warnings {
		lower := strings.ToLower(w)
		if strings.Contains(lower, "inconclusive") ||
			strings.Contains(lower, "no workflows matched") ||
			strings.Contains(lower, "human review recommended") {
			return true
		}
	}
	return false
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

// ExtractRootCauseAnalysis extracts RCA from an IncidentResponse, including affectedResource.
// Issue #97: Centralizes RCA extraction (was duplicated in 5 handler functions) and adds
// affectedResource extraction so RO's resolveEffectivenessTarget can target the correct resource.
func ExtractRootCauseAnalysis(rcaData interface{}) *aianalysisv1.RootCauseAnalysis {
	rcaMap := GetMapFromOptNil(rcaData)
	if rcaMap == nil {
		return nil
	}
	rca := &aianalysisv1.RootCauseAnalysis{
		Summary:             GetStringFromMap(rcaMap, "summary"),
		Severity:            GetStringFromMap(rcaMap, "severity"),
		ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),
	}

	// Issue #97: Extract affectedResource from RCA (populated by HAPI from owner chain)
	if arRaw, ok := rcaMap["affectedResource"]; ok {
		if arMap, ok := arRaw.(map[string]interface{}); ok {
			kind, _ := arMap["kind"].(string)
			name, _ := arMap["name"].(string)
			ns, _ := arMap["namespace"].(string)
			if kind != "" && name != "" {
				rca.AffectedResource = &aianalysisv1.AffectedResource{
					Kind:      kind,
					Name:      name,
					Namespace: ns,
				}
			}
		}
	}

	return rca
}
