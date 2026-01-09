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

// Package audit provides audit event generation for the AIAnalysis controller.
// DD-AUDIT-002 V2.2: Uses shared pkg/audit library with zero unstructured data.
// DD-AUDIT-003: Implements service-specific audit event types.
// DD-AUDIT-004 V1.3: Direct struct assignment to SetEventData (no map conversion).
package audit

import (
	"context"
	"strings"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit" // BR-AUDIT-005 Gap #7: Standardized error details
)

// Event type constants (per DD-AUDIT-003)
const (
	EventTypeAnalysisCompleted = "aianalysis.analysis.completed"
	EventTypePhaseTransition   = "aianalysis.phase.transition"
	EventTypeHolmesGPTCall     = "aianalysis.holmesgpt.call"
	EventTypeApprovalDecision  = "aianalysis.approval.decision"
	EventTypeRegoEvaluation    = "aianalysis.rego.evaluation"
	EventTypeError             = "aianalysis.error.occurred"
)

// Event category constant (per DD-AUDIT-003)
// FIXED: Changed from "aianalysis" to "analysis" to match OpenAPI schema enum (api/openapi/data-storage-v1.yaml:832-920)
const (
	EventCategoryAIAnalysis = "analysis"
)

// Event action constants (per DD-AUDIT-003)
const (
	EventActionAnalysisComplete = "analysis_complete"
	EventActionPhaseTransition  = "phase_transition"
	EventActionError            = "error"
	EventActionHolmesGPTCall    = "holmesgpt_call"
	EventActionApprovalDecision = "approval_decision"
	EventActionPolicyEvaluation = "policy_evaluation"
)

// Actor constants (per DD-AUDIT-003)
const (
	ActorTypeService            = "service"
	ActorIDAIAnalysisController = "aianalysis-controller"
)

// AuditClient handles audit event storage using pkg/audit shared library
type AuditClient struct {
	store audit.AuditStore // Uses shared library interface
	log   logr.Logger
}

// NewAuditClient creates a new audit client
func NewAuditClient(store audit.AuditStore, log logr.Logger) *AuditClient {
	return &AuditClient{
		store: store,
		log:   log.WithName("audit"),
	}
}

// RecordAnalysisComplete records analysis completion event
// This is the primary audit event for AIAnalysis (per DD-AUDIT-003)
//
// Uses OpenAPI-generated types (DD-AUDIT-004 V2.0) - eliminates duplicate type definitions.
func (c *AuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// Build structured payload using OpenAPI-generated type
	// Single source of truth: api/openapi/data-storage-v1.yaml
	payload := &ogenclient.AIAnalysisAuditPayload{
		EventType:        EventTypeAnalysisCompleted,
		AnalysisName:     analysis.Name,
		Namespace:        analysis.Namespace,
		Phase:            ogenclient.AIAnalysisAuditPayloadPhase(analysis.Status.Phase),
		ApprovalRequired: analysis.Status.ApprovalRequired,
		DegradedMode:     analysis.Status.DegradedMode,
		WarningsCount:    len(analysis.Status.Warnings),
	}

	// Optional fields
	if analysis.Status.ApprovalReason != "" {
		payload.ApprovalReason = &analysis.Status.ApprovalReason
	}
	if analysis.Status.SelectedWorkflow != nil {
		confidence := float32(analysis.Status.SelectedWorkflow.Confidence)
		payload.Confidence = &confidence
		payload.WorkflowId = &analysis.Status.SelectedWorkflow.WorkflowID
	}
	if analysis.Status.TargetInOwnerChain != nil {
		payload.TargetInOwnerChain = analysis.Status.TargetInOwnerChain
	}
	if analysis.Status.Reason != "" {
		payload.Reason = &analysis.Status.Reason
	}
	if analysis.Status.SubReason != "" {
		payload.SubReason = &analysis.Status.SubReason
	}

	// DD-AUDIT-005: Add provider response summary (consumer perspective)
	// This complements the holmesgpt.response.complete event (provider perspective)
	if analysis.Status.InvestigationID != "" {
		summary := &ogenclient.ProviderResponseSummary{
			IncidentId:       analysis.Status.InvestigationID,
			AnalysisPreview:  truncateString(analysis.Status.RootCause, 500),
			NeedsHumanReview: determineNeedsHumanReview(analysis),
			WarningsCount:    len(analysis.Status.Warnings),
		}
		if analysis.Status.SelectedWorkflow != nil {
			summary.SelectedWorkflowId = &analysis.Status.SelectedWorkflow.WorkflowID
		}
		payload.ProviderResponseSummary = summary
	}

	// Determine outcome
	var apiOutcome ogenclient.AuditEventRequestEventOutcome
	if analysis.Status.Phase == "Failed" {
		apiOutcome = audit.OutcomeFailure
	} else {
		apiOutcome = audit.OutcomeSuccess
	}

	// Build audit event (DD-AUDIT-002 V2.2: Direct struct assignment)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeAnalysisCompleted)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionAnalysisComplete) // Fixed: Must match test contract
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	// Fire-and-forget (per Risk #4 / DD-AUDIT-002)
	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write audit event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationId,
		)
		// Don't fail reconciliation on audit failure (graceful degradation)
	}
}

// RecordPhaseTransition records a phase transition event
//
// Uses OpenAPI-generated types (DD-AUDIT-004 V2.0).
func (c *AuditClient) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
	// Idempotency check: Only record if phase actually changed
	if from == to {
		c.log.V(1).Info("Skipping phase transition audit - phase unchanged",
			"phase", from,
			"name", analysis.Name,
			"namespace", analysis.Namespace)
		return
	}

	// Build structured payload using OpenAPI-generated type
	payload := &ogenclient.AIAnalysisPhaseTransitionPayload{
		OldPhase: from,
		NewPhase: to,
	}

	// Build audit event (DD-AUDIT-002 V2.2: Direct struct assignment)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypePhaseTransition)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionPhaseTransition)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write phase transition audit")
	}
}

// RecordError records an error event
//
// Uses OpenAPI-generated types (DD-AUDIT-004 V2.0).
func (c *AuditClient) RecordError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, phase string, err error) {
	// Build structured payload using OpenAPI-generated type
	payload := &ogenclient.AIAnalysisErrorPayload{
		Phase:        phase,
		ErrorMessage: err.Error(),
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeError)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionError)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	// Note: error_message DB column is populated by Data Storage from event_data JSON
	// Per ADR-034: Error details stored in event_data for query-ability

	if storeErr := c.store.StoreAudit(ctx, event); storeErr != nil {
		c.log.Error(storeErr, "Failed to write error audit")
	}
}

// RecordHolmesGPTCall records a HolmesGPT-API call event
//
// Uses OpenAPI-generated types (DD-AUDIT-004 V2.0).
func (c *AuditClient) RecordHolmesGPTCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
	// Build structured payload using OpenAPI-generated type
	payload := &ogenclient.AIAnalysisHolmesGPTCallPayload{
		Endpoint:       endpoint,
		HttpStatusCode: int32(statusCode),
		DurationMs:     int32(durationMs),
	}

	// Determine outcome
	var apiOutcome ogenclient.AuditEventRequestEventOutcome
	if statusCode >= 400 {
		apiOutcome = audit.OutcomeFailure
	} else {
		apiOutcome = audit.OutcomeSuccess
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeHolmesGPTCall)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionHolmesGPTCall) // Fixed: Must match test contract
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetDuration(event, durationMs)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write HolmesGPT call audit")
	}
}

// RecordApprovalDecision records an approval decision event
//
// Uses OpenAPI-generated types (DD-AUDIT-004 V2.0).
func (c *AuditClient) RecordApprovalDecision(ctx context.Context, analysis *aianalysisv1.AIAnalysis, decision string, reason string) {
	// Derive boolean flags from decision string
	approvalRequired := decision == "requires_approval"
	autoApproved := decision == "auto_approved"

	// Build structured payload using OpenAPI-generated type
	payload := &ogenclient.AIAnalysisApprovalDecisionPayload{
		ApprovalRequired: approvalRequired,
		ApprovalReason:   reason,
		AutoApproved:     autoApproved,
		Decision:         decision,
		Reason:           reason,
		Environment:      analysis.Spec.AnalysisRequest.SignalContext.Environment,
	}

	// Conditional fields (type-safe pointers)
	if analysis.Status.SelectedWorkflow != nil {
		payload.Confidence = &analysis.Status.SelectedWorkflow.Confidence
		payload.WorkflowId = &analysis.Status.SelectedWorkflow.WorkflowID
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeApprovalDecision)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionApprovalDecision)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write approval decision audit")
	}
}

// RecordRegoEvaluation records a Rego policy evaluation event
//
// Uses OpenAPI-generated types (DD-AUDIT-004 V2.0).
func (c *AuditClient) RecordRegoEvaluation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome string, degraded bool, durationMs int, reason string) {
	// Build structured payload using OpenAPI-generated type
	payload := &ogenclient.AIAnalysisRegoEvaluationPayload{
		Outcome:    outcome,
		Degraded:   degraded,
		DurationMs: int32(durationMs),
		Reason:     reason,
	}

	// Map outcome to OpenAPI enum
	var apiOutcome ogenclient.AuditEventRequestEventOutcome
	switch outcome {
	case "allow", "success", "requires_approval", "auto_approved": // Rego policy decision outcomes
		apiOutcome = audit.OutcomeSuccess
	case "deny", "failure":
		apiOutcome = audit.OutcomeFailure
	default:
		apiOutcome = audit.OutcomePending
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeRegoEvaluation)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionPolicyEvaluation)
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetDuration(event, durationMs)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write Rego evaluation audit")
	}
}

// ========================================
// HELPER FUNCTIONS (DD-AUDIT-005)
// ========================================

// truncateString truncates a string to the specified length, adding "..." if truncated
// Used for audit event preview fields to limit event payload size
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// determineNeedsHumanReview infers needs_human_review flag from AIAnalysis status
//
// DD-AUDIT-005: Consumer perspective on whether Holmes recommended human review
// This is inferred from AIAnalysis status fields since we don't store the raw Holmes response
//
// Logic:
// - Failed + SubReason set = needs human review
// - ApprovalRequired + certain approval reasons = needs human review
// - No selected workflow = needs human review
func determineNeedsHumanReview(analysis *aianalysisv1.AIAnalysis) bool {
	// Failed analyses typically need human review
	if analysis.Status.Phase == "Failed" {
		return true
	}

	// No workflow selected = needs human review
	if analysis.Status.SelectedWorkflow == nil {
		return true
	}

	// Approval required due to low confidence or validation issues
	if analysis.Status.ApprovalRequired {
		// High-severity approval reasons suggest human review needed
		highSeverityReasons := map[string]bool{
			"WorkflowNotFound":              true,
			"NoMatchingWorkflows":           true,
			"LowConfidence":                 true,
			"LLMParsingError":               true,
			"InvestigationInconclusive":     true,
		}
		if highSeverityReasons[analysis.Status.SubReason] {
			return true
		}
	}

	return false
}

// RecordAnalysisFailed records an audit event for analysis failure.
//
// This method implements BR-AUDIT-005 Gap #7: Standardized error details
// for SOC2 compliance and RR reconstruction.
//
// Parameters:
// - ctx: Context for the operation
// - analysis: AIAnalysis CRD that failed
// - err: Error that caused the failure (e.g., Holmes API error)
//
// Example Usage:
//
//	err := callHolmesAPI(ctx, analysis)
//	if err != nil {
//	    if auditErr := c.RecordAnalysisFailed(ctx, analysis, err); auditErr != nil {
//	        logger.Error(auditErr, "Failed to record analysis failure audit")
//	    }
//	    return err
//	}
//
// Event Structure:
// - event_type: "aianalysis.analysis.failed"
// - event_category: "analysis"
// - event_outcome: "failure"
// - event_data.error_details: Standardized ErrorDetails structure
func (c *AuditClient) RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error {
	// Imports needed at top of file
	// sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"

	// Determine error details based on error type
	var errorDetails *sharedaudit.ErrorDetails

	// Check if it's a Holmes API/upstream error (common case)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	if err != nil && (strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "context deadline exceeded")) {
		errorDetails = sharedaudit.NewErrorDetails(
			"aianalysis",
			"ERR_UPSTREAM_TIMEOUT",
			errMsg,
			true, // Timeout is transient
		)
	} else if err != nil && strings.Contains(errMsg, "invalid response") {
		errorDetails = sharedaudit.NewErrorDetails(
			"aianalysis",
			"ERR_UPSTREAM_INVALID_RESPONSE",
			errMsg,
			false, // Invalid response may not be retryable
		)
	} else if err != nil {
		// Generic upstream error
		errorDetails = sharedaudit.NewErrorDetails(
			"aianalysis",
			"ERR_UPSTREAM_FAILURE",
			errMsg,
			true, // Assume upstream errors are transient
		)
	} else {
		// No error provided (shouldn't happen, but handle gracefully)
		errorDetails = sharedaudit.NewErrorDetails(
			"aianalysis",
			"ERR_INTERNAL_UNKNOWN",
			"Analysis failed with unknown error",
			false,
		)
	}

	// Build audit event per DD-AUDIT-002 V2.0: OpenAPI types
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, "aianalysis.analysis.failed")
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, "failed")
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", "aianalysis-controller")
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, string(analysis.UID))
	audit.SetNamespace(event, analysis.Namespace)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := aianalysis.AIAnalysisAuditPayload{
		AnalysisName: analysis.Name,
		Namespace:    analysis.Namespace,
		Phase:        string(analysis.Status.Phase),
		ErrorDetails: errorDetails, // Gap #7: Standardized error_details for SOC2 compliance
	}
	audit.SetEventData(event, payload)

	// Store audit event
	return c.store.StoreAudit(ctx, event)
}

