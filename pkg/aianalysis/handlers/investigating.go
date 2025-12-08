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
package handlers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

const (
	// MaxRetries for transient errors before marking as Failed
	MaxRetries = 5

	// BaseDelay for exponential backoff
	BaseDelay = 30 * time.Second

	// MaxDelay caps the backoff delay
	MaxDelay = 480 * time.Second

	// RetryCountAnnotation stores retry count in CRD annotations
	RetryCountAnnotation = "aianalysis.kubernaut.ai/retry-count"
)

// HolmesGPTClientInterface defines the interface for HolmesGPT-API calls
type HolmesGPTClientInterface interface {
	Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)
}

// InvestigatingHandler handles the Investigating phase
// BR-AI-007: Call HolmesGPT-API and process response
type InvestigatingHandler struct {
	log      logr.Logger
	hgClient HolmesGPTClientInterface
}

// NewInvestigatingHandler creates a new InvestigatingHandler
func NewInvestigatingHandler(hgClient HolmesGPTClientInterface, log logr.Logger) *InvestigatingHandler {
	return &InvestigatingHandler{
		hgClient: hgClient,
		log:      log.WithName("investigating-handler"),
	}
}

// Handle processes the Investigating phase
// BR-AI-007: Call HolmesGPT-API and update status
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Investigating phase", "name", analysis.Name)

	// Build request from spec
	req := h.buildRequest(analysis)

	// Call HolmesGPT-API
	resp, err := h.hgClient.Investigate(ctx, req)
	if err != nil {
		return h.handleError(ctx, analysis, err)
	}

	// Process successful response
	return h.processResponse(ctx, analysis, resp)
}

// buildRequest constructs the HolmesGPT-API request from AIAnalysis spec
func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *client.IncidentRequest {
	spec := analysis.Spec.AnalysisRequest.SignalContext

	return &client.IncidentRequest{
		Context: fmt.Sprintf("Incident in %s environment, target: %s/%s/%s, signal: %s",
			spec.Environment,
			spec.TargetResource.Namespace,
			spec.TargetResource.Kind,
			spec.TargetResource.Name,
			spec.SignalType,
		),
	}
}

// handleError processes errors from HolmesGPT-API
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.IsTransient() {
		// Transient error - check retry count
		retryCount := h.getRetryCount(analysis)

		if retryCount >= MaxRetries {
			// Max retries exceeded - mark as Failed
			h.log.Info("Max retries exceeded", "retryCount", retryCount)
			now := metav1.Now()
			analysis.Status.Phase = aianalysis.PhaseFailed
			analysis.Status.CompletedAt = &now // Per crd-schema.md: set on terminal state
			analysis.Status.Message = fmt.Sprintf("HolmesGPT-API max retries exceeded: %v", err)
			analysis.Status.Reason = "MaxRetriesExceeded"
			return ctrl.Result{}, nil
		}

		// Schedule retry with backoff
		delay := calculateBackoff(retryCount)
		h.setRetryCount(analysis, retryCount+1)

		h.log.Info("Transient error, scheduling retry",
			"retryCount", retryCount+1,
			"delay", delay.String(),
		)
		return ctrl.Result{RequeueAfter: delay}, err
	}

	// Permanent error - mark as Failed immediately
	h.log.Info("Permanent error", "error", err)
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.CompletedAt = &now // Per crd-schema.md: set on terminal state
	analysis.Status.Message = fmt.Sprintf("HolmesGPT-API error: %v", err)
	analysis.Status.Reason = "APIError"
	return ctrl.Result{}, nil
}

// processResponse handles successful HolmesGPT-API response
// BR-AI-008: Capture all response fields including RCA, workflow, and alternatives
// BR-HAPI-197: Check needs_human_review before proceeding
// Per HolmesGPT-API team (Dec 5, 2025): /incident/analyze returns ALL analysis results
func (h *InvestigatingHandler) processResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	h.log.Info("Processing successful response",
		"confidence", resp.Confidence,
		"targetInOwnerChain", resp.TargetInOwnerChain,
		"warningsCount", len(resp.Warnings),
		"hasSelectedWorkflow", resp.SelectedWorkflow != nil,
		"alternativeWorkflowsCount", len(resp.AlternativeWorkflows),
		"needsHumanReview", resp.NeedsHumanReview,
	)

	// BR-HAPI-197: Check if workflow resolution failed
	// When needs_human_review=true, AIAnalysis MUST NOT proceed to create WorkflowExecution
	if resp.NeedsHumanReview {
		return h.handleWorkflowResolutionFailure(ctx, analysis, resp)
	}

	// BR-HAPI-200 Outcome A: Problem confidently resolved, no workflow needed
	// Detection: needs_human_review=false AND selected_workflow=null AND confidence >= 0.7
	// This scenario happens when:
	// - Pod recovered automatically (OOMKilled then restarted)
	// - Alert resolved before investigation completed (transient condition)
	// - Resource self-healed (deployment rollout succeeded after initial failure)
	if resp.SelectedWorkflow == nil && resp.Confidence >= 0.7 {
		return h.handleProblemResolved(ctx, analysis, resp)
	}

	// Store HAPI response metadata
	targetInOwnerChain := resp.TargetInOwnerChain
	analysis.Status.TargetInOwnerChain = &targetInOwnerChain
	analysis.Status.Warnings = resp.Warnings

	// Store root cause analysis (if present)
	if resp.RootCauseAnalysis != nil {
		analysis.Status.RootCause = resp.RootCauseAnalysis.Summary
		analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:             resp.RootCauseAnalysis.Summary,
			Severity:            resp.RootCauseAnalysis.Severity,
			ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
		}
	}

	// Store selected workflow (DD-CONTRACT-002)
	if resp.SelectedWorkflow != nil {
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      resp.SelectedWorkflow.WorkflowID,
			Version:         resp.SelectedWorkflow.Version,
			ContainerImage:  resp.SelectedWorkflow.ContainerImage,
			ContainerDigest: resp.SelectedWorkflow.ContainerDigest,
			Confidence:      resp.SelectedWorkflow.Confidence,
			Parameters:      resp.SelectedWorkflow.Parameters,
			Rationale:       resp.SelectedWorkflow.Rationale,
		}
	}

	// Store alternative workflows (INFORMATIONAL ONLY - NOT for execution)
	// Per HolmesGPT-API team: Alternatives are for CONTEXT, not EXECUTION
	if len(resp.AlternativeWorkflows) > 0 {
		alternatives := make([]aianalysisv1.AlternativeWorkflow, 0, len(resp.AlternativeWorkflows))
		for _, alt := range resp.AlternativeWorkflows {
			alternatives = append(alternatives, aianalysisv1.AlternativeWorkflow{
				WorkflowID:     alt.WorkflowID,
				ContainerImage: alt.ContainerImage,
				Confidence:     alt.Confidence,
				Rationale:      alt.Rationale,
			})
		}
		analysis.Status.AlternativeWorkflows = alternatives
	}

	// Reset retry count on success
	h.setRetryCount(analysis, 0)

	// Transition to Analyzing phase
	analysis.Status.Phase = aianalysis.PhaseAnalyzing
	analysis.Status.Message = "Investigation complete, starting analysis"

	return ctrl.Result{Requeue: true}, nil
}

// getRetryCount reads retry count from annotations
func (h *InvestigatingHandler) getRetryCount(analysis *aianalysisv1.AIAnalysis) int {
	if analysis.Annotations == nil {
		return 0
	}
	countStr, ok := analysis.Annotations[RetryCountAnnotation]
	if !ok {
		return 0
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0
	}
	return count
}

// setRetryCount writes retry count to annotations
func (h *InvestigatingHandler) setRetryCount(analysis *aianalysisv1.AIAnalysis, count int) {
	if analysis.Annotations == nil {
		analysis.Annotations = make(map[string]string)
	}
	analysis.Annotations[RetryCountAnnotation] = strconv.Itoa(count)
}

// calculateBackoff computes exponential backoff with jitter
func calculateBackoff(attemptCount int) time.Duration {
	delay := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attemptCount)))
	if delay > MaxDelay {
		delay = MaxDelay
	}
	// Add jitter (±10%)
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))
	return jitter
}

// ========================================
// BR-HAPI-197: WORKFLOW RESOLUTION FAILURE HANDLING
// When HolmesGPT-API returns needs_human_review=true, we MUST:
// 1. NOT proceed to Analyzing phase
// 2. Set structured failure reason (Reason + SubReason)
// 3. Preserve partial response for operator context
// ========================================

// handleWorkflowResolutionFailure handles needs_human_review=true responses
// BR-HAPI-197: Workflow resolution failed, human must intervene
func (h *InvestigatingHandler) handleWorkflowResolutionFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	h.log.Info("Workflow resolution failed, requires human review",
		"warnings", resp.Warnings,
		"humanReviewReason", resp.HumanReviewReason,
		"hasPartialWorkflow", resp.SelectedWorkflow != nil,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.CompletedAt = &now // Per crd-schema.md: set on terminal state
	analysis.Status.Reason = "WorkflowResolutionFailed"

	// Use HumanReviewReason enum if available (preferred), else fallback to warning parsing
	if resp.HumanReviewReason != nil && *resp.HumanReviewReason != "" {
		analysis.Status.SubReason = h.mapEnumToSubReason(*resp.HumanReviewReason)
	} else {
		// Backward compatibility: parse warnings if enum not available
		analysis.Status.SubReason = mapWarningsToSubReason(resp.Warnings)
	}

	// Build message from validation attempts history for detailed operator notification
	if len(resp.ValidationAttemptsHistory) > 0 {
		var attemptDetails []string
		for _, attempt := range resp.ValidationAttemptsHistory {
			attemptDetails = append(attemptDetails,
				fmt.Sprintf("Attempt %d: %s", attempt.Attempt, strings.Join(attempt.Errors, "; ")))
		}
		analysis.Status.Message = strings.Join(attemptDetails, " | ")

		// Store validation attempts history for audit/debugging
		analysis.Status.ValidationAttemptsHistory = h.convertValidationAttempts(resp.ValidationAttemptsHistory)
	} else {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Preserve partial response for operator context (BR-HAPI-197.4)
	if resp.SelectedWorkflow != nil {
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     resp.SelectedWorkflow.WorkflowID,
			ContainerImage: resp.SelectedWorkflow.ContainerImage,
			Confidence:     resp.SelectedWorkflow.Confidence,
			Rationale:      resp.SelectedWorkflow.Rationale,
		}
	}

	// Preserve RCA for operator context
	if resp.RootCauseAnalysis != nil {
		analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:             resp.RootCauseAnalysis.Summary,
			Severity:            resp.RootCauseAnalysis.Severity,
			ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
		}
	}

	// Reset retry count (this is a terminal failure, not transient)
	h.setRetryCount(analysis, 0)

	return ctrl.Result{}, nil // Terminal - no requeue
}

// handleProblemResolved handles BR-HAPI-200 Outcome A: problem self-resolved
// This is a TERMINAL SUCCESS state - no workflow execution needed
// Triggered when: needs_human_review=false AND selected_workflow=null AND confidence >= 0.7
func (h *InvestigatingHandler) handleProblemResolved(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	h.log.Info("Problem confirmed resolved, no workflow needed",
		"confidence", resp.Confidence,
		"warnings", resp.Warnings,
	)

	// Set terminal success state per BR-HAPI-200
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseCompleted
	analysis.Status.CompletedAt = &now // Per crd-schema.md: set on terminal state
	analysis.Status.Reason = "WorkflowNotNeeded"
	analysis.Status.SubReason = "ProblemResolved"

	// Use analysis text if available, else construct from warnings
	// Note: HAPI JSON uses "investigation_summary" but Go client maps to "Analysis"
	if resp.Analysis != "" {
		analysis.Status.Message = resp.Analysis
	} else if len(resp.Warnings) > 0 {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	} else {
		analysis.Status.Message = "Problem self-resolved. No remediation required."
	}

	analysis.Status.Warnings = resp.Warnings

	// Store RCA if available (for context/audit)
	if resp.RootCauseAnalysis != nil {
		analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:             resp.RootCauseAnalysis.Summary,
			Severity:            resp.RootCauseAnalysis.Severity,
			ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
		}
	}

	// Reset retry count on success
	h.setRetryCount(analysis, 0)

	return ctrl.Result{}, nil // Terminal - no requeue
}

// mapEnumToSubReason maps HAPI HumanReviewReason enum to CRD SubReason
// This is the preferred method - direct enum-to-enum mapping (Dec 6, 2025)
// Updated Dec 7, 2025: Added investigation_inconclusive per BR-HAPI-200
func (h *InvestigatingHandler) mapEnumToSubReason(reason string) string {
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
	h.log.Info("Unknown human_review_reason, defaulting to WorkflowNotFound", "reason", reason)
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

// convertValidationAttempts converts client.ValidationAttempt to CRD ValidationAttempt
// DD-HAPI-002 v1.4: Maps HAPI response to CRD status for audit/debugging
func (h *InvestigatingHandler) convertValidationAttempts(attempts []client.ValidationAttempt) []aianalysisv1.ValidationAttempt {
	result := make([]aianalysisv1.ValidationAttempt, 0, len(attempts))
	for _, a := range attempts {
		// Parse ISO timestamp to metav1.Time
		ts, err := time.Parse(time.RFC3339, a.Timestamp)
		if err != nil {
			h.log.Info("Failed to parse validation attempt timestamp, using current time",
				"timestamp", a.Timestamp, "error", err)
			ts = time.Now()
		}

		result = append(result, aianalysisv1.ValidationAttempt{
			Attempt:    a.Attempt,
			WorkflowID: a.WorkflowID,
			IsValid:    a.IsValid,
			Errors:     a.Errors,
			Timestamp:  metav1.NewTime(ts),
		})
	}
	return result
}
