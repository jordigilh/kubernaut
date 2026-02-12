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
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	hgptclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// P2.2 Refactoring: Constants moved to constants.go

// P1.3 Refactoring: HolmesGPTClientInterface moved to interfaces.go

// InvestigatingHandler handles the Investigating phase
// BR-AI-007: Call HolmesGPT-API and process response
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
// BR-AA-HAPI-064: Async session-based submit/poll/result flow
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
	useSessionMode  bool                 // BR-AA-HAPI-064: Enable async session-based flow
	recorder        record.EventRecorder // DD-EVENT-001: K8s event recorder for session lifecycle events
}

// InvestigatingHandlerOption is a functional option for InvestigatingHandler configuration.
type InvestigatingHandlerOption func(*InvestigatingHandler)

// WithSessionMode enables the async session-based submit/poll/result flow (BR-AA-HAPI-064).
// When enabled, the handler uses SubmitInvestigation/PollSession/GetSessionResult
// instead of the legacy synchronous Investigate/InvestigateRecovery methods.
func WithSessionMode() InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.useSessionMode = true
	}
}

// WithRecorder injects a Kubernetes EventRecorder for session lifecycle events (DD-EVENT-001).
// When set, the handler emits SessionCreated, SessionLost, and SessionRegenerationExceeded events.
func WithRecorder(r record.EventRecorder) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.recorder = r
	}
}

// P1.3 Refactoring: AuditClientInterface moved to interfaces.go

// NewInvestigatingHandler creates a new InvestigatingHandler
// Refactoring P1.1: Initializes ResponseProcessor
// Refactoring P1.2: Initializes RequestBuilder
// Refactoring P2.1: Initializes ErrorClassifier with configurable backoff parameters
// BR-AA-HAPI-064: Accepts functional options (e.g., WithSessionMode())
func NewInvestigatingHandler(hgClient HolmesGPTClientInterface, log logr.Logger, m *metrics.Metrics, auditClient AuditClientInterface, opts ...InvestigatingHandlerOption) *InvestigatingHandler {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	handlerLog := log.WithName("investigating-handler")
	h := &InvestigatingHandler{
		hgClient:        hgClient,
		metrics:         m,
		auditClient:     auditClient,
		log:             handlerLog,
		processor:       NewResponseProcessor(log, m, auditClient),   // P1.1: Initialize response processor (audit recorded by processor for failures)
		builder:         NewRequestBuilder(log),                      // P1.2: Initialize request builder
		errorClassifier: NewErrorClassifier(handlerLog),              // P2.1: Initialize error classifier
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes the Investigating phase
// BR-AI-007: Call HolmesGPT-API and update status
// BR-AI-083: Route to recovery endpoint when IsRecoveryAttempt=true
// BR-AA-HAPI-064: Async session-based flow when useSessionMode=true
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Investigating phase",
		"name", analysis.Name,
		"isRecoveryAttempt", analysis.Spec.IsRecoveryAttempt,
		"sessionMode", h.useSessionMode,
	)

	// AA-HAPI-001: Idempotency is handled at controller level (phase_handlers.go:125-130)
	// via AtomicStatusUpdate callback with APIReader refetch. No handler-level check needed.

	// BR-AA-HAPI-064: Use async session-based flow when enabled
	if h.useSessionMode {
		return h.handleSessionBased(ctx, analysis)
	}

	// ========================================
	// Legacy synchronous flow (will be deprecated)
	// ========================================

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
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeRateLimit, ErrorTypeTransient:
		// All transient/retryable errors map to "TransientError"
		return "TransientError"
	case ErrorTypePermanent, ErrorTypeAuthentication, ErrorTypeAuthorization, ErrorTypeConfiguration:
		// All non-retryable errors map to "PermanentError"
		return "PermanentError"
	default:
		// Fallback for unknown error types
		return "TransientError"
	}
}

// ========================================
// SESSION-BASED FLOW (BR-AA-HAPI-064)
// Async submit/poll/result pattern for HAPI communication
// ========================================

// handleSessionBased routes the session-based flow based on InvestigationSession state.
// BR-AA-HAPI-064: Non-blocking communication with HAPI via submit/poll/result
func (h *InvestigatingHandler) handleSessionBased(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession

	// SUBMIT: No session yet, or session ID cleared after loss (needs new submit)
	if session == nil || session.ID == "" {
		return h.handleSessionSubmit(ctx, analysis)
	}

	// POLL: Session exists with an active ID
	return h.handleSessionPoll(ctx, analysis)
}

// handleSessionSubmit submits a new investigation to HAPI and records the session ID.
// BR-AA-HAPI-064.1: Submit returns session ID for subsequent polling
// BR-AA-HAPI-064.9: Recovery submit routes to recovery endpoint
func (h *InvestigatingHandler) handleSessionSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	// Detect if this is a regeneration (session exists but ID was cleared after 404)
	isRegeneration := analysis.Status.InvestigationSession != nil &&
		analysis.Status.InvestigationSession.ID == "" &&
		analysis.Status.InvestigationSession.Generation > 0

	var sessionID string
	var err error

	// BR-AI-083: Route based on IsRecoveryAttempt
	if analysis.Spec.IsRecoveryAttempt {
		h.log.Info("Submitting recovery investigation to HAPI (session mode)",
			"attemptNumber", analysis.Spec.RecoveryAttemptNumber,
			"isRegeneration", isRegeneration,
		)
		req := h.builder.BuildRecoveryRequest(analysis)
		sessionID, err = h.hgClient.SubmitRecoveryInvestigation(ctx, req)
	} else {
		h.log.Info("Submitting incident investigation to HAPI (session mode)",
			"isRegeneration", isRegeneration,
		)
		req := h.builder.BuildIncidentRequest(analysis)
		sessionID, err = h.hgClient.SubmitInvestigation(ctx, req)
	}

	if err != nil {
		return h.handleError(ctx, analysis, err)
	}

	// Initialize or update InvestigationSession in CRD status
	now := metav1.Now()
	if analysis.Status.InvestigationSession == nil {
		analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
			ID:         sessionID,
			Generation: 0,
			CreatedAt:  &now,
			PollCount:  0,
		}
	} else {
		analysis.Status.InvestigationSession.ID = sessionID
		analysis.Status.InvestigationSession.CreatedAt = &now
		analysis.Status.InvestigationSession.PollCount = 0
		analysis.Status.InvestigationSession.LastPolled = nil
	}

	// Set condition: SessionCreated (first time) or SessionRegenerated (after loss)
	condReason := aianalysis.ReasonSessionCreated
	condMsg := fmt.Sprintf("Session %s created", sessionID)
	if isRegeneration {
		condReason = aianalysis.ReasonSessionRegenerated
		condMsg = fmt.Sprintf("Session %s regenerated (generation %d)", sessionID, analysis.Status.InvestigationSession.Generation)
	}
	aianalysis.SetInvestigationSessionReady(analysis, true, condReason, condMsg)

	// DD-AUDIT-003: Record submit audit event
	h.auditClient.RecordHolmesGPTSubmit(ctx, analysis, sessionID)

	// DD-EVENT-001: Emit SessionCreated K8s event for observability
	if h.recorder != nil {
		h.recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonSessionCreated,
			fmt.Sprintf("HAPI investigation session created (ID: %s, generation: %d)", sessionID, analysis.Status.InvestigationSession.Generation))
	}

	h.log.Info("HAPI session created",
		"sessionID", sessionID,
		"generation", analysis.Status.InvestigationSession.Generation,
		"isRegeneration", isRegeneration,
	)

	// Requeue for first poll at DefaultPollInterval
	return ctrl.Result{RequeueAfter: DefaultPollInterval}, nil
}

// handleSessionPoll polls the status of an active HAPI session.
// BR-AA-HAPI-064.2: Poll session status (pending/investigating/completed/failed)
func (h *InvestigatingHandler) handleSessionPoll(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession

	h.log.V(1).Info("Polling HAPI session",
		"sessionID", session.ID,
		"pollCount", session.PollCount,
	)

	status, err := h.hgClient.PollSession(ctx, session.ID)
	if err != nil {
		return h.handleSessionPollError(ctx, analysis, err)
	}

	switch status.Status {
	case "pending", "investigating":
		return h.handleSessionPollPending(ctx, analysis, status)
	case "completed":
		return h.handleSessionPollCompleted(ctx, analysis)
	case "failed":
		return h.handleSessionPollFailed(ctx, analysis, status)
	default:
		// Unknown status: treat as pending (requeue for re-poll)
		h.log.Info("Unknown session status, treating as pending", "status", status.Status)
		return h.handleSessionPollPending(ctx, analysis, status)
	}
}

// handleSessionPollPending handles poll results where investigation is still in progress.
// BR-AA-HAPI-064.8: Exponential backoff (10s base, 2x multiplier, 30s cap)
func (h *InvestigatingHandler) handleSessionPollPending(ctx context.Context, analysis *aianalysisv1.AIAnalysis, status *hgptclient.SessionStatus) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession

	// Compute backoff interval from poll count
	interval := h.computePollBackoff(session.PollCount)

	// Update poll tracking
	now := metav1.Now()
	session.LastPolled = &now
	session.PollCount++

	// Set condition: SessionActive
	aianalysis.SetInvestigationSessionReady(analysis, true, aianalysis.ReasonSessionActive,
		fmt.Sprintf("Session active, polling (status: %s, poll #%d)", status.Status, session.PollCount))

	h.log.V(1).Info("Session still in progress, requeuing",
		"status", status.Status,
		"nextPollIn", interval,
		"pollCount", session.PollCount,
	)

	return ctrl.Result{RequeueAfter: interval}, nil
}

// handleSessionPollCompleted handles poll results where investigation has completed.
// BR-AA-HAPI-064.3: Fetch result and process through ResponseProcessor
func (h *InvestigatingHandler) handleSessionPollCompleted(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession

	h.log.Info("HAPI session completed, fetching result",
		"sessionID", session.ID,
	)

	// Calculate investigation time from session creation
	var investigationTime int64
	if session.CreatedAt != nil {
		investigationTime = time.Since(session.CreatedAt.Time).Milliseconds()
	}

	if analysis.Spec.IsRecoveryAttempt {
		return h.handleSessionRecoveryResult(ctx, analysis, investigationTime)
	}
	return h.handleSessionIncidentResult(ctx, analysis, investigationTime)
}

// handleSessionIncidentResult fetches and processes the incident investigation result.
func (h *InvestigatingHandler) handleSessionIncidentResult(ctx context.Context, analysis *aianalysisv1.AIAnalysis, investigationTime int64) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession

	resp, err := h.hgClient.GetSessionResult(ctx, session.ID)
	if err != nil {
		return h.handleSessionGetResultError(ctx, analysis, err)
	}

	// Set investigation metadata
	analysis.Status.InvestigationTime = investigationTime
	analysis.Status.ObservedGeneration = analysis.Generation

	// DD-AUDIT-003: Record result retrieval audit event
	h.auditClient.RecordHolmesGPTResult(ctx, analysis, investigationTime)

	if resp == nil {
		return h.handleError(ctx, analysis, fmt.Errorf("received nil incident response from HolmesGPT-API session %s", session.ID))
	}

	// Delegate to ResponseProcessor (same as legacy flow)
	result, err := h.processor.ProcessIncidentResponse(ctx, analysis, resp)
	if err == nil {
		h.setRetryCount(analysis, 0)
	}
	return result, err
}

// handleSessionRecoveryResult fetches and processes the recovery investigation result.
func (h *InvestigatingHandler) handleSessionRecoveryResult(ctx context.Context, analysis *aianalysisv1.AIAnalysis, investigationTime int64) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession

	resp, err := h.hgClient.GetRecoverySessionResult(ctx, session.ID)
	if err != nil {
		return h.handleSessionGetResultError(ctx, analysis, err)
	}

	// Set investigation metadata
	analysis.Status.InvestigationTime = investigationTime
	analysis.Status.ObservedGeneration = analysis.Generation

	// DD-AUDIT-003: Record result retrieval audit event
	h.auditClient.RecordHolmesGPTResult(ctx, analysis, investigationTime)

	// BR-AI-082: Populate RecoveryStatus from recovery_analysis
	if resp != nil {
		wasPopulated := h.processor.PopulateRecoveryStatusFromRecovery(analysis, resp)
		if wasPopulated {
			if analysis.Status.RecoveryStatus != nil && analysis.Status.RecoveryStatus.PreviousAttemptAssessment != nil {
				h.metrics.RecordRecoveryStatusPopulated(
					analysis.Status.RecoveryStatus.PreviousAttemptAssessment.FailureUnderstood,
					analysis.Status.RecoveryStatus.StateChanged,
				)
			}
		} else {
			h.metrics.RecordRecoveryStatusSkipped()
		}
	}

	if resp == nil {
		return h.handleError(ctx, analysis, fmt.Errorf("received nil recovery response from HolmesGPT-API session %s", session.ID))
	}

	// Delegate to ResponseProcessor (same as legacy flow)
	result, err := h.processor.ProcessRecoveryResponse(ctx, analysis, resp)
	if err == nil {
		h.setRetryCount(analysis, 0)
	}
	return result, err
}

// handleSessionPollFailed handles poll results where investigation has failed on HAPI side.
// BR-AA-HAPI-064: Surface HAPI-side failure to operators via CRD status
func (h *InvestigatingHandler) handleSessionPollFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, status *hgptclient.SessionStatus) (ctrl.Result, error) {
	h.log.Info("HAPI session failed",
		"sessionID", analysis.Status.InvestigationSession.ID,
		"error", status.Error,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.CompletedAt = &now
	analysis.Status.ObservedGeneration = analysis.Generation
	analysis.Status.Message = status.Error
	if analysis.Status.Message == "" {
		analysis.Status.Message = "Investigation failed on HAPI side"
	}

	// Record failure audit
	failureErr := fmt.Errorf("HAPI session failed: %s", status.Error)
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	return ctrl.Result{}, nil
}

// handleSessionPollError handles errors during session polling (e.g., 404 session lost).
// BR-AA-HAPI-064.5: 404 triggers session regeneration, not standard retry
func (h *InvestigatingHandler) handleSessionPollError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	// Check for 404 (session lost) - triggers regeneration logic
	var apiErr *hgptclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return h.handleSessionLost(ctx, analysis)
	}

	// For other errors, use standard error classification and retry
	return h.handleError(ctx, analysis, err)
}

// handleSessionLost handles session loss (404 on poll) with regeneration logic.
// BR-AA-HAPI-064.5: Increment generation, clear ID, requeue for re-submit
// BR-AA-HAPI-064.6: Fail with SessionRegenerationExceeded if cap reached
func (h *InvestigatingHandler) handleSessionLost(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	session := analysis.Status.InvestigationSession
	session.Generation++
	session.ID = ""
	session.PollCount = 0
	session.LastPolled = nil

	h.log.Info("HAPI session lost, regenerating",
		"generation", session.Generation,
		"maxRegenerations", MaxSessionRegenerations,
	)

	// DD-AUDIT-003: Record session lost event
	h.auditClient.RecordHolmesGPTSessionLost(ctx, analysis, session.Generation)

	// DD-EVENT-001: Emit SessionLost K8s event for observability
	if h.recorder != nil {
		h.recorder.Event(analysis, corev1.EventTypeWarning, events.EventReasonSessionLost,
			fmt.Sprintf("HAPI session lost (generation %d), attempting regeneration", session.Generation))
	}

	// BR-AA-HAPI-064.6: Check if regeneration cap exceeded
	if session.Generation >= MaxSessionRegenerations {
		h.log.Info("Session regeneration cap exceeded",
			"generation", session.Generation,
			"cap", MaxSessionRegenerations,
		)

		now := metav1.Now()
		analysis.Status.Phase = aianalysis.PhaseFailed
		analysis.Status.CompletedAt = &now
		analysis.Status.ObservedGeneration = analysis.Generation
		analysis.Status.SubReason = "SessionRegenerationExceeded"
		analysis.Status.Message = fmt.Sprintf("Session regeneration cap exceeded (%d regenerations)", session.Generation)

		aianalysis.SetInvestigationSessionReady(analysis, false, aianalysis.ReasonSessionRegenerationExceeded,
			fmt.Sprintf("Regeneration cap (%d) exceeded", MaxSessionRegenerations))

		// Record failure audit
		failureErr := fmt.Errorf("session regeneration cap exceeded: %d regenerations", session.Generation)
		if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
			h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
		}

		// DD-EVENT-001: Emit SessionRegenerationExceeded K8s event for observability
		if h.recorder != nil {
			h.recorder.Event(analysis, corev1.EventTypeWarning, events.EventReasonSessionRegenerationExceeded,
				fmt.Sprintf("Max session regenerations (%d) exceeded, failing investigation", MaxSessionRegenerations))
		}

		return ctrl.Result{}, nil
	}

	// Set condition: SessionLost (not terminal yet)
	aianalysis.SetInvestigationSessionReady(analysis, false, aianalysis.ReasonSessionLost,
		fmt.Sprintf("Session lost, regenerating (generation %d/%d)", session.Generation, MaxSessionRegenerations))

	// Requeue immediately for re-submit
	return ctrl.Result{Requeue: true}, nil
}

// handleSessionGetResultError handles errors when fetching the session result.
// BR-AA-HAPI-064: 409 Conflict treated as transient (re-poll gracefully)
func (h *InvestigatingHandler) handleSessionGetResultError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	// Check for 409 Conflict - treat as transient, re-poll
	var apiErr *hgptclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
		h.log.Info("GetSessionResult returned 409 Conflict, treating as transient",
			"sessionID", analysis.Status.InvestigationSession.ID,
		)
		session := analysis.Status.InvestigationSession
		interval := h.computePollBackoff(session.PollCount)
		now := metav1.Now()
		session.LastPolled = &now
		session.PollCount++
		return ctrl.Result{RequeueAfter: interval}, nil
	}

	// For other errors, use standard error classification
	return h.handleError(ctx, analysis, err)
}

// computePollBackoff calculates the poll interval using exponential backoff.
// BR-AA-HAPI-064.8: DefaultPollInterval * 2^pollCount, capped at MaxPollInterval
func (h *InvestigatingHandler) computePollBackoff(pollCount int32) time.Duration {
	interval := DefaultPollInterval
	for i := int32(0); i < pollCount; i++ {
		interval *= time.Duration(PollBackoffMultiplier)
		if interval > MaxPollInterval {
			return MaxPollInterval
		}
	}
	return interval
}
