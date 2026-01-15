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
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

// P2.2 Refactoring: Constants moved to constants.go

// P1.3 Refactoring: HolmesGPTClientInterface moved to interfaces.go

// InvestigatingHandler handles the Investigating phase
// BR-AI-007: Call HolmesGPT-API and process response
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
// Refactoring P1.1: Uses ResponseProcessor for response handling
// Refactoring P1.2: Uses RequestBuilder for request construction
// Refactoring P2.1: Uses ErrorClassifier for error classification and retry logic
type InvestigatingHandler struct {
	log             logr.Logger
	hgClient        HolmesGPTClientInterface
	metrics         *metrics.Metrics     // DD-METRICS-001: Injected metrics
	auditClient     AuditClientInterface // DD-AUDIT-003: Injected audit client
	processor       *ResponseProcessor   // P1.1: Response processing logic
	builder         *RequestBuilder      // P1.2: Request construction logic
	errorClassifier *ErrorClassifier     // P2.1: Error classification and retry logic
}

// P1.3 Refactoring: AuditClientInterface moved to interfaces.go

// NewInvestigatingHandler creates a new InvestigatingHandler
// Refactoring P1.1: Initializes ResponseProcessor
// Refactoring P1.2: Initializes RequestBuilder
// Refactoring P2.1: Initializes ErrorClassifier with configurable backoff parameters
func NewInvestigatingHandler(hgClient HolmesGPTClientInterface, log logr.Logger, m *metrics.Metrics, auditClient AuditClientInterface) *InvestigatingHandler {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	handlerLog := log.WithName("investigating-handler")
	return &InvestigatingHandler{
		hgClient:        hgClient,
		metrics:         m,
		auditClient:     auditClient,
		log:             handlerLog,
		processor:       NewResponseProcessor(log, m),   // P1.1: Initialize response processor (audit recorded by handler, not processor)
		builder:         NewRequestBuilder(log),         // P1.2: Initialize request builder
		errorClassifier: NewErrorClassifier(handlerLog), // P2.1: Initialize error classifier
	}
}

// Handle processes the Investigating phase
// BR-AI-007: Call HolmesGPT-API and update status
// BR-AI-083: Route to recovery endpoint when IsRecoveryAttempt=true
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Investigating phase",
		"name", analysis.Name,
		"isRecoveryAttempt", analysis.Spec.IsRecoveryAttempt,
	)

	// AA-HAPI-001: Idempotency is handled at controller level (phase_handlers.go:125-130)
	// via AtomicStatusUpdate callback with APIReader refetch. No handler-level check needed.

	// Track duration (per crd-schema.md: InvestigationTime)
	startTime := time.Now()

	// BR-AI-083: Route based on IsRecoveryAttempt
	if analysis.Spec.IsRecoveryAttempt {
		h.log.Info("Calling HolmesGPT-API recovery endpoint",
			"attemptNumber", analysis.Spec.RecoveryAttemptNumber,
		)
		recoveryReq := h.builder.BuildRecoveryRequest(analysis) // P1.2: Use request builder
		recoveryResp, err := h.hgClient.InvestigateRecovery(ctx, recoveryReq)

		investigationTime := time.Since(startTime).Milliseconds()

		// DD-AUDIT-003: Record HolmesGPT API call for audit trail
		statusCode := 200
		if err != nil {
			statusCode = 500 // Error case
		}
		h.auditClient.RecordHolmesGPTCall(ctx, analysis, "/api/v1/recovery/investigate", statusCode, int(investigationTime))

		if err != nil {
			return h.handleError(ctx, analysis, err)
		}

		// AA-HAPI-001: Set ObservedGeneration immediately after successful HAPI call
		// This prevents duplicate HAPI calls when controller reconciles before status persists
		// DD-CONTROLLER-001 v3.0 Pattern C: Set before phase transition
		analysis.Status.ObservedGeneration = analysis.Generation

		// Set investigation time on successful response
		analysis.Status.InvestigationTime = investigationTime

		// BR-AI-082: Populate RecoveryStatus if recovery_analysis present
		if recoveryResp != nil {
			wasPopulated := h.processor.PopulateRecoveryStatusFromRecovery(analysis, recoveryResp) // P1.1: Use processor
			if wasPopulated {
				// Record recovery status population metric
				if analysis.Status.RecoveryStatus != nil && analysis.Status.RecoveryStatus.PreviousAttemptAssessment != nil {
					h.metrics.RecordRecoveryStatusPopulated(
						analysis.Status.RecoveryStatus.PreviousAttemptAssessment.FailureUnderstood,
						analysis.Status.RecoveryStatus.StateChanged,
					)
				}
			} else {
				// Record skipped metric when HAPI doesn't return recovery_analysis
				h.metrics.RecordRecoveryStatusSkipped()
			}
		}

		// Process recovery response - must check for nil (CRITICAL: prevents panic)
		if recoveryResp == nil {
			return h.handleError(ctx, analysis, fmt.Errorf("received nil recovery response from HolmesGPT-API"))
		}
		// P1.1: Delegate to processor, reset retry count after success
		// DD-AUDIT-003: Phase transition audit recorded by controller AFTER AtomicStatusUpdate (phase_handlers.go)
		result, err := h.processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)
		if err == nil {
			h.setRetryCount(analysis, 0)
		}
		return result, err
	} else {
		req := h.builder.BuildIncidentRequest(analysis) // P1.2: Use request builder
		incidentResp, err := h.hgClient.Investigate(ctx, req)

		investigationTime := time.Since(startTime).Milliseconds()

		// DD-AUDIT-003: Record HolmesGPT API call for audit trail
		statusCode := 200
		if err != nil {
			statusCode = 500 // Error case
		}
		h.auditClient.RecordHolmesGPTCall(ctx, analysis, "/api/v1/incident/analyze", statusCode, int(investigationTime))

		if err != nil {
			return h.handleError(ctx, analysis, err)
		}

		// AA-HAPI-001: Set ObservedGeneration immediately after successful HAPI call
		// This prevents duplicate HAPI calls when controller reconciles before status persists
		// DD-CONTROLLER-001 v3.0 Pattern C: Set before phase transition
		analysis.Status.ObservedGeneration = analysis.Generation

		// Set investigation time on successful response
		analysis.Status.InvestigationTime = investigationTime

		// Process incident response - must check for nil (CRITICAL: prevents panic)
		if incidentResp == nil {
			return h.handleError(ctx, analysis, fmt.Errorf("received nil incident response from HolmesGPT-API"))
		}
		// P1.1: Delegate to processor, reset retry count after success
		// DD-AUDIT-003: Phase transition audit recorded by controller AFTER AtomicStatusUpdate (phase_handlers.go)
		result, err := h.processor.ProcessIncidentResponse(ctx, analysis, incidentResp)
		if err == nil {
			h.setRetryCount(analysis, 0)
		}
		return result, err
	}
}

// buildRequest constructs the HolmesGPT-API request from AIAnalysis spec using generated types
// BR-AI-080: Updated with all required HAPI fields per NOTICE_AIANALYSIS_HAPI_CONTRACT_MISMATCH.md
// Per crd-schema.md: Include enrichment data (owner chain, detected labels) for AI context
// P1.2 Refactoring: Request building methods moved to request_builder.go
// - BuildIncidentRequest (was buildRequest)
// - BuildRecoveryRequest (was buildRecoveryRequest)
// - buildPreviousExecution (private method in builder)
// - getOrDefault (helper function in builder)
// - strPtr (helper function in builder)

// handleError processes errors from HolmesGPT-API
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
// Refactoring P2.1: Uses ErrorClassifier for error classification and retry logic
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	// P2.1: Classify error type using error classifier
	classification := h.errorClassifier.ClassifyError(err)

	// Increment failure count before retry check
	analysis.Status.ConsecutiveFailures++

	// P2.1: Check if error should be retried based on classification and attempt count
	if h.errorClassifier.ShouldRetry(classification, int(analysis.Status.ConsecutiveFailures)) {
		// BR-AI-009: Retry transient errors with exponential backoff

		// P2.1: Use error classifier to calculate backoff duration
		backoffDuration := h.errorClassifier.GetRetryDelay(int(analysis.Status.ConsecutiveFailures))

		h.log.Info("Transient error - retrying with backoff",
			"error", err,
			"errorType", classification.ErrorType,
			"attempts", analysis.Status.ConsecutiveFailures,
			"backoff", backoffDuration,
		)

		// Update status to indicate retry
		analysis.Status.Message = fmt.Sprintf("Transient error (attempt %d/%d): %v",
			analysis.Status.ConsecutiveFailures, MaxRetries, err)
		analysis.Status.Reason = "TransientError"
		analysis.Status.SubReason = mapErrorTypeToSubReason(classification.ErrorType) // Map to valid CRD enum

		// Record metric for transient errors
		h.metrics.RecordFailure("TransientError", "Retrying")

		// Requeue with exponential backoff (error classifier handles jitter internally)
		return ctrl.Result{RequeueAfter: backoffDuration}, nil
	}

	// If we get here, either max retries exceeded or error is not retryable
	if classification.IsRetryable {
		// Max retries exceeded
		h.log.Info("Transient error exceeded max retries - failing permanently",
			"error", err,
			"errorType", classification.ErrorType,
			"attempts", analysis.Status.ConsecutiveFailures,
			"maxRetries", h.errorClassifier.GetMaxRetries(),
		)

		// Transition to permanent failure after max retries
		now := metav1.Now()
		analysis.Status.Phase = aianalysis.PhaseFailed
		analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
		analysis.Status.CompletedAt = &now
		analysis.Status.Message = fmt.Sprintf("Transient error exceeded max retries (%d attempts): %v",
			analysis.Status.ConsecutiveFailures, err)
		analysis.Status.Reason = "APIError"
		analysis.Status.SubReason = "MaxRetriesExceeded"

		// Record metric for max retries exceeded
		h.metrics.RecordFailure("APIError", "MaxRetriesExceeded")

		// BR-AUDIT-005 Gap #7: Record failure audit with standardized error details
		if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, err); auditErr != nil {
			h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
		}

		return ctrl.Result{}, nil
	}

	// BR-AI-010: Fail immediately on permanent errors
	h.log.Info("Permanent error - failing immediately",
		"error", err,
		"errorType", classification.ErrorType,
	)
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now                       // Per crd-schema.md: set on terminal state
	analysis.Status.Message = fmt.Sprintf("Permanent error: %v", err)
	analysis.Status.Reason = "APIError"
	analysis.Status.SubReason = mapErrorTypeToSubReason(classification.ErrorType) // Map to valid CRD enum

	// Record metric for permanent errors
	h.metrics.RecordFailure("APIError", string(classification.ErrorType))

	// BR-AUDIT-005 Gap #7: Record failure audit with standardized error details
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, err); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	return ctrl.Result{}, nil
}

// P1.1 Refactoring: Response processing methods moved to response_processor.go
// - ProcessIncidentResponse (was processIncidentResponse)
// - ProcessRecoveryResponse (was processRecoveryResponse)
// - PopulateRecoveryStatusFromRecovery (was populateRecoveryStatusFromRecovery)
// - handleWorkflowResolutionFailureFromIncident (private method in processor)
// - handleProblemResolvedFromIncident (private method in processor)
// - handleRecoveryNotPossible (private method in processor)
// - mapEnumToSubReason (private method in processor)

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

// ========================================
// BR-HAPI-197: WORKFLOW RESOLUTION FAILURE HANDLING
// When HolmesGPT-API returns needs_human_review=true, we MUST:
// 1. NOT proceed to Analyzing phase
// 2. Set structured failure reason (Reason + SubReason)
// 3. Preserve partial response for operator context
// ========================================

// handleWorkflowResolutionFailure handles needs_human_review=true responses
// BR-HAPI-197: Workflow resolution failed, human must intervene
// NOTE: Old handleWorkflowResolutionFailure and handleProblemResolved methods
// have been replaced by generated-type versions:
// - handleWorkflowResolutionFailureFromIncident (for IncidentResponse)
// - handleProblemResolvedFromIncident (for IncidentResponse)
// - handleRecoveryNotPossible (for RecoveryResponse)

// P1.1 Refactoring: mapEnumToSubReason and mapWarningsToSubReason moved to response_processor.go

// DD-HAPI-002 v1.4: Maps HAPI response to CRD status for audit/debugging
// NOTE: Old convertValidationAttempts and populateRecoveryStatus methods deleted
// - populateRecoveryStatus: Replaced by populateRecoveryStatusFromRecovery (for generated.RecoveryResponse)

// mapErrorTypeToSubReason maps error classifier ErrorType to valid AIAnalysis CRD SubReason enum values
// per config/crd/bases/kubernaut.ai_aianalyses.yaml line 134-144
func mapErrorTypeToSubReason(errorType ErrorType) string {
	switch errorType {
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeRateLimit:
		// All transient errors map to "TransientError"
		return "TransientError"
	case ErrorTypePermanent:
		return "PermanentError"
	default:
		// Fallback for unknown error types
		return "TransientError"
	}
}
