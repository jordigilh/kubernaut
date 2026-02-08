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

// Package audit provides audit event manager for Remediation Orchestrator.
//
// Business Requirements:
// - BR-STORAGE-001: Complete audit trail with no data loss
// - DD-AUDIT-003: All services must emit audit events
//
// Authority: ADR-034 (Unified Audit Table), DD-AUDIT-002 (Audit Shared Library)
// Pattern: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §7 (Audit Manager P3)
package audit

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit" // BR-AUDIT-005 Gap #7: Standardized error details
)

// ServiceName is the canonical service identifier for audit events.
// Follows naming convention: "<service>-controller" (consistent with notification-controller, signalprocessing-controller)
const ServiceName = "remediationorchestrator-controller"

// Event category for RO audit events (ADR-034 v1.2: Service-level category)
const (
	CategoryOrchestration = "orchestration" // Service-level identifier per ADR-034 v1.2
)

// Event type constants for RemediationOrchestrator audit events (from OpenAPI spec)
const (
	EventTypeLifecycleStarted      = "orchestrator.lifecycle.started"
	EventTypeLifecycleCreated      = "orchestrator.lifecycle.created" // Gap #8: BR-AUDIT-005 (TimeoutConfig capture)
	EventTypeLifecycleCompleted    = "orchestrator.lifecycle.completed"
	EventTypeLifecycleFailed       = "orchestrator.lifecycle.failed"
	EventTypeLifecycleTransitioned = "orchestrator.lifecycle.transitioned" // Replaces "orchestrator.phase.transitioned"
	EventTypeApprovalRequested     = "orchestrator.approval.requested"
	EventTypeApprovalApproved      = "orchestrator.approval.approved"
	EventTypeApprovalRejected      = "orchestrator.approval.rejected"
	EventTypeManualReview          = "orchestrator.remediation.manual_review"
	EventTypeRoutingBlocked        = "orchestrator.routing.blocked"
)

// Event category constant (from OpenAPI spec)
const (
	EventCategoryOrchestration = "orchestration"
)

// Event actions for RO audit events (per DD-AUDIT-003)
const (
	ActionStarted           = "started"
	ActionTransitioned      = "transitioned"
	ActionCompleted         = "completed"
	ActionFailed            = "failed"
	ActionApprovalRequested = "approval_requested"
	ActionApproved          = "approved"
	ActionRejected          = "rejected"
	ActionExpired           = "expired"
	ActionManualReview      = "manual_review_required"
	ActionBlocked           = "blocked"   // Routing blocked (DD-RO-002)
	ActionUnblocked         = "unblocked" // Routing unblocked (future)
)

// TimeoutConfig mirrors remediationv1alpha1.TimeoutConfig for audit purposes.
// This avoids importing the entire remediation API package into the audit manager.
// Per BR-AUDIT-005 Gap #8: Captures timeout configuration for RR reconstruction.
type TimeoutConfig struct {
	Global     *metav1.Duration
	Processing *metav1.Duration
	Analyzing  *metav1.Duration
	Executing  *metav1.Duration
}

// Manager provides audit event building for RO.
// Per CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §7 (Audit Manager Pattern P3)
type Manager struct {
	serviceName string
}

// NewManager creates a new Manager instance.
func NewManager(serviceName string) *Manager {
	return &Manager{
		serviceName: serviceName,
	}
}

// LifecycleStartedData is the event_data for lifecycle started events.
// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
type LifecycleStartedData struct {
	RRName    string `json:"rr_name"`
	Namespace string `json:"namespace"`
}

// BuildRemediationCreatedEvent builds an audit event for RR creation with timeout config (Gap #8).
// Per BR-AUDIT-005 v2.0 Gap #8: Captures TimeoutConfig for RR reconstruction.
// Per ADR-034 v1.2: Uses orchestrator.lifecycle.created naming convention.
// This event is emitted when RemediationRequest is FIRST reconciled by Orchestrator.
//
// Event Data includes:
// - timeout_config: {global, processing, analyzing, executing} (from status, populated by RO)
// - rr_name: RemediationRequest name
// - namespace: Kubernetes namespace
//
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildRemediationCreatedEvent(
	correlationID string,
	namespace string,
	rrName string,
	timeoutConfig *TimeoutConfig,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeLifecycleCreated) // Gap #8: Per ADR-034 v1.2 naming convention
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, "created")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)

	// Gap #8: Build timeout_config structure for audit
	// Convert TimeoutConfig to OptTimeoutConfig (ogen union type)
	var timeoutConfigOpt api.OptTimeoutConfig
	if timeoutConfig != nil {
		// Build TimeoutConfig structure with all fields as OptString
		tc := api.TimeoutConfig{}
		if timeoutConfig.Global != nil && timeoutConfig.Global.Duration > 0 {
			tc.Global.SetTo(timeoutConfig.Global.Duration.String())
		}
		if timeoutConfig.Processing != nil && timeoutConfig.Processing.Duration > 0 {
			tc.Processing.SetTo(timeoutConfig.Processing.Duration.String())
		}
		if timeoutConfig.Analyzing != nil && timeoutConfig.Analyzing.Duration > 0 {
			tc.Analyzing.SetTo(timeoutConfig.Analyzing.Duration.String())
		}
		if timeoutConfig.Executing != nil && timeoutConfig.Executing.Duration > 0 {
			tc.Executing.SetTo(timeoutConfig.Executing.Duration.String())
		}
		timeoutConfigOpt.SetTo(tc)
	}

	// Use ogen union constructor (OGEN-MIGRATION)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType:     api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated, // Gap #8: Corrected per ADR-034
		RrName:        rrName,
		Namespace:     namespace,
		TimeoutConfig: timeoutConfigOpt, // Gap #8: Capture TimeoutConfig for RR reconstruction
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCreatedAuditEventRequestEventData(payload)

	return event, nil
}

// BuildLifecycleStartedEvent builds an audit event for remediation lifecycle started.
// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildLifecycleStartedEvent(
	correlationID string,
	namespace string,
	rrName string,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeLifecycleStarted)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionStarted)
	audit.SetEventOutcome(event, audit.OutcomePending) // Lifecycle started, outcome not yet determined
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)

	// Use ogen union constructor (OGEN-MIGRATION)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted,
		RrName:    rrName,
		Namespace: namespace,
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleStartedAuditEventRequestEventData(payload)

	return event, nil
}

// PhaseTransitionData is the event_data for phase transition events.
// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
type PhaseTransitionData struct {
	FromPhase string `json:"from_phase"`
	ToPhase   string `json:"to_phase"`
	Namespace string `json:"namespace"`
	RRName    string `json:"rr_name"`
}

// BuildPhaseTransitionEvent builds an audit event for phase transitions.
// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildPhaseTransitionEvent(
	correlationID string,
	namespace string,
	rrName string,
	fromPhase string,
	toPhase string,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeLifecycleTransitioned)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionTransitioned)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)

	// Event data (DD-AUDIT-002 V2.2: Direct struct assignment, zero unstructured data)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleTransitioned,
		FromPhase: api.OptString{Value: fromPhase, Set: true},
		ToPhase:   api.OptString{Value: toPhase, Set: true},
		Namespace: namespace,
		RrName:    rrName,
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleTransitionedAuditEventRequestEventData(payload)

	return event, nil
}

// CompletionData is the event_data for remediation lifecycle completion events.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1)
type CompletionData struct {
	Outcome       string `json:"outcome"`
	DurationMs    int64  `json:"duration_ms"`
	Namespace     string `json:"namespace"`
	RRName        string `json:"rr_name"`
	FailurePhase  string `json:"failure_phase,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// BuildCompletionEvent builds an audit event for remediation lifecycle completion.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildCompletionEvent(
	correlationID string,
	namespace string,
	rrName string,
	outcome string,
	durationMs int64,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeLifecycleCompleted)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionCompleted)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)
	audit.SetDuration(event, int(durationMs))

	// Event data (DD-AUDIT-002 V2.2: Direct struct assignment, zero unstructured data)
	// Note: `outcome` parameter is CRD-level (Remediated/NoActionRequired/ManualReviewRequired)
	// but OpenAPI enum expects ["Success", "Failed", "Pending"] - always use "Success" for completion events
	payload := api.RemediationOrchestratorAuditPayload{
		EventType:  api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted,
		Outcome:    api.OptRemediationOrchestratorAuditPayloadOutcome{Value: api.RemediationOrchestratorAuditPayloadOutcomeSuccess, Set: true},
		DurationMs: api.OptInt64{Value: durationMs, Set: true},
		Namespace:  namespace,
		RrName:     rrName,
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(payload)

	return event, nil
}

// BuildFailureEvent builds an audit event for remediation lifecycle failure.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1) with failure outcome
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
// BR-AUDIT-005 Gap #7: Now includes standardized error_details for SOC2 compliance
//
// F-6 SOC2 Audit Fix: Changed failureErr from string to error type.
// Uses ClassifyError for typed error classification instead of string matching.
func (m *Manager) BuildFailureEvent(
	correlationID string,
	namespace string,
	rrName string,
	failurePhase string,
	failureErr error,
	durationMs int64,
) (*ogenclient.AuditEventRequest, error) {
	// F-6: Use typed error classification instead of strings.Contains
	classification := ClassifyError(failureErr)

	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	// BR-AUDIT-005 Gap #7: Build standardized error_details
	errorMessage := fmt.Sprintf("Remediation failed at phase '%s': %s", failurePhase, failureReason)

	errorDetails := sharedaudit.NewErrorDetails(
		"remediationorchestrator",
		classification.Code,
		errorMessage,
		classification.RetryPossible,
	)

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeLifecycleCompleted)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionCompleted)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)
	audit.SetDuration(event, int(durationMs))

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.RemediationOrchestratorAuditPayload{
		EventType:     api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted, // DD-AUDIT-003: lifecycle.completed with failure outcome
		RrName:        rrName,
		Namespace:     namespace,
		Outcome:       api.OptRemediationOrchestratorAuditPayloadOutcome{Value: api.RemediationOrchestratorAuditPayloadOutcomeFailed, Set: true},
		DurationMs:    api.OptInt64{Value: durationMs, Set: true},
		FailurePhase:  ToOptFailurePhase(failurePhase),
		FailureReason: ToOptFailureReason(failureReason),
		ErrorDetails:  toOptErrorDetails(errorDetails), // Gap #7: Standardized error_details for SOC2 compliance
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(payload) // DD-AUDIT-003: Use lifecycle.completed discriminator

	return event, nil
}

// ApprovalData is the event_data for approval-related events.
type ApprovalData struct {
	RARName       string `json:"rar_name"`
	RRName        string `json:"rr_name"`
	Namespace     string `json:"namespace"`
	Decision      string `json:"decision,omitempty"`
	DecidedBy     string `json:"decided_by,omitempty"`
	Message       string `json:"message,omitempty"`
	RequiredBy    string `json:"required_by,omitempty"`
	WorkflowID    string `json:"workflow_id,omitempty"`
	ConfidenceStr string `json:"confidence,omitempty"`
}

// BuildApprovalRequestedEvent builds an audit event for approval requested.
// Related to ADR-040 (RemediationApprovalRequest)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildApprovalRequestedEvent(
	correlationID string,
	namespace string,
	rrName string,
	rarName string,
	workflowID string,
	confidence string,
	requiredBy time.Time,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeApprovalRequested)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionApprovalRequested)
	audit.SetEventOutcome(event, audit.OutcomePending) // Approval outcome is pending until decision made
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationApprovalRequest", rarName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)

	// Event data (DD-AUDIT-002 V2.2: Direct struct assignment, zero unstructured data)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType:     api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRequested, // Fixed: was lifecycle.transitioned
		RarName:       api.OptString{Value: rarName, Set: true},
		RrName:        rrName,
		Namespace:     namespace,
		RequiredBy:    api.OptDateTime{Value: requiredBy, Set: true},
		WorkflowID:    api.OptString{Value: workflowID, Set: true},
		ConfidenceStr: api.OptString{Value: confidence, Set: true},
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorApprovalRequestedAuditEventRequestEventData(payload)

	return event, nil
}

// BuildApprovalDecisionEvent builds an audit event for approval decision.
// Related to ADR-040 (RemediationApprovalRequest)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
//
// REFACTOR-RO-AUD-001: Refactored to use ApprovalDecisionMapping type (eliminates triple switch)
// REFACTOR-RO-AUD-002: Refactored to use DetermineActor helper
func (m *Manager) BuildApprovalDecisionEvent(
	correlationID string,
	namespace string,
	rrName string,
	rarName string,
	decision string,
	decidedBy string,
	message string,
) (*ogenclient.AuditEventRequest, error) {
	// REFACTOR-RO-AUD-001: Get decision mapping (replaces triple switch statement)
	mapping, _ := GetApprovalDecisionMapping(decision)

	// REFACTOR-RO-AUD-002: Determine actor using extracted helper
	actorType, actorID := DetermineActor(decidedBy, m.serviceName)

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, string(mapping.PayloadEventType))
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, mapping.Action)
	audit.SetEventOutcome(event, mapping.Outcome)
	audit.SetActor(event, actorType, actorID)
	audit.SetResource(event, "RemediationApprovalRequest", rarName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)

	// Event data (DD-AUDIT-002 V2.2: Direct struct assignment, zero unstructured data)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: mapping.PayloadEventType,
		RarName:   api.OptString{Value: rarName, Set: true},
		RrName:    rrName,
		Namespace: namespace,
		Decision:  ToOptDecision(decision),
		// Note: DecidedBy and Message are intentionally NOT in payload per ADR-034 v1.7
		// Two-Event Pattern: DecidedBy is captured by AuthWebhook (webhook category event)
		// Orchestration event focuses on WHAT/WHY (business context), not WHO (attribution)
	}

	// REFACTOR-RO-AUD-001: Use mapping discriminator wrapper (replaces switch statement)
	event.EventData = mapping.DiscriminatorWrapper(payload)

	return event, nil
}

// ManualReviewData is the event_data for manual review required events.
type ManualReviewData struct {
	RRName        string `json:"rr_name"`
	Namespace     string `json:"namespace"`
	Reason        string `json:"reason"`
	SubReason     string `json:"sub_reason,omitempty"`
	NotificationN string `json:"notification_name,omitempty"`
}

// BuildManualReviewEvent builds an audit event for manual review required.
// Related to BR-ORCH-036 (Manual Review Notifications)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildManualReviewEvent(
	correlationID string,
	namespace string,
	rrName string,
	reason string,
	subReason string,
	notificationName string,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeManualReview)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionManualReview)
	audit.SetEventOutcome(event, audit.OutcomePending)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)
	audit.SetSeverity(event, "warning")

	// Event data (DD-AUDIT-002 V2.2: Direct struct assignment, zero unstructured data)
	// F-3 SOC2 Fix: Use matching discriminator so outer event_type == EventData.Type
	payload := api.RemediationOrchestratorAuditPayload{
		EventType:        api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorRemediationManualReview,
		RrName:           rrName,
		Namespace:        namespace,
		Reason:           api.OptString{Value: reason, Set: true},
		SubReason:        api.OptString{Value: subReason, Set: true},
		NotificationName: api.OptString{Value: notificationName, Set: true},
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorRemediationManualReviewAuditEventRequestEventData(payload)

	return event, nil
}

// RoutingBlockedData is the event_data for routing blocked events.
// Per DD-RO-002: Routing Engine blocking conditions
// Per ADR-032 §1: All phase transitions must be audited
type RoutingBlockedData struct {
	BlockReason         string  `json:"block_reason"`
	BlockMessage        string  `json:"block_message"`
	FromPhase           string  `json:"from_phase"`
	ToPhase             string  `json:"to_phase"`
	WorkflowID          string  `json:"workflow_id,omitempty"`
	TargetResource      string  `json:"target_resource"`
	RequeueAfterSeconds int     `json:"requeue_after_seconds"`
	BlockedUntil        *string `json:"blocked_until,omitempty"`
	BlockingWFE         string  `json:"blocking_wfe,omitempty"`
	DuplicateOf         string  `json:"duplicate_of,omitempty"`
	ConsecutiveFailures int32   `json:"consecutive_failures,omitempty"`
	BackoffSeconds      int     `json:"backoff_seconds,omitempty"`
	RecentWFE           string  `json:"recent_wfe,omitempty"`
}

// BuildRoutingBlockedEvent builds an audit event for routing blocked decisions.
// Per DD-RO-002: Centralized Routing Engine blocking conditions
// Per ADR-032 §1: All phase transitions must be audited (Pending/Analyzing → Blocked)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) BuildRoutingBlockedEvent(
	correlationID string,
	namespace string,
	rrName string,
	fromPhase string,
	blockData *RoutingBlockedData,
) (*ogenclient.AuditEventRequest, error) {
	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeRoutingBlocked)
	audit.SetEventCategory(event, CategoryOrchestration)
	audit.SetEventAction(event, ActionBlocked)
	audit.SetEventOutcome(event, audit.OutcomePending)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "RemediationRequest", rrName)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, namespace)

	// Event data (DD-AUDIT-002 V2.2: Direct struct assignment, zero unstructured data)
	// F-3 SOC2 Fix: Use matching discriminator so outer event_type == EventData.Type
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorRoutingBlocked,
		RrName:    rrName,
		Namespace: namespace,
	}
	event.EventData = api.NewAuditEventRequestEventDataOrchestratorRoutingBlockedAuditEventRequestEventData(payload)

	return event, nil
}

// ========================================
// OGEN-MIGRATION: Helper functions for type conversion
// ========================================

// toOptFailurePhase converts string to ogen enum type.
func ToOptFailurePhase(phase string) api.OptRemediationOrchestratorAuditPayloadFailurePhase {
	if phase == "" {
		return api.OptRemediationOrchestratorAuditPayloadFailurePhase{}
	}

	var result api.OptRemediationOrchestratorAuditPayloadFailurePhase
	switch phase {
	case "SignalProcessing", "signal_processing": // Controller uses snake_case
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailurePhaseSignalProcessing)
	case "AIAnalysis", "ai_analysis": // Controller uses snake_case
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailurePhaseAIAnalysis)
	case "WorkflowExecution", "workflow_execution": // Controller uses snake_case
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailurePhaseWorkflowExecution)
	case "Approval", "approval": // Controller uses snake_case
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailurePhaseApproval)
	}
	return result
}

// toOptFailureReason converts string to ogen enum type.
func ToOptFailureReason(reason string) api.OptRemediationOrchestratorAuditPayloadFailureReason {
	if reason == "" {
		return api.OptRemediationOrchestratorAuditPayloadFailureReason{}
	}

	var result api.OptRemediationOrchestratorAuditPayloadFailureReason
	switch reason {
	case "Timeout":
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailureReasonTimeout)
	case "ValidationError":
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailureReasonValidationError)
	case "InfrastructureError":
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailureReasonInfrastructureError)
	case "ApprovalRejected":
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailureReasonApprovalRejected)
	case "Unknown":
		result.SetTo(api.RemediationOrchestratorAuditPayloadFailureReasonUnknown)
	}
	return result
}

// toOptDecision converts string to ogen enum type.
func ToOptDecision(decision string) api.OptRemediationOrchestratorAuditPayloadDecision {
	if decision == "" {
		return api.OptRemediationOrchestratorAuditPayloadDecision{}
	}

	var result api.OptRemediationOrchestratorAuditPayloadDecision
	switch decision {
	case "Approved":
		result.SetTo(api.RemediationOrchestratorAuditPayloadDecisionApproved)
	case "Rejected":
		result.SetTo(api.RemediationOrchestratorAuditPayloadDecisionRejected)
	case "Pending":
		result.SetTo(api.RemediationOrchestratorAuditPayloadDecisionPending)
	}
	return result
}

// toOptErrorDetails converts sharedaudit.ErrorDetails to api.OptErrorDetails.
//
// **Refactoring**: 2026-01-22 - Use shared helper from pkg/shared/audit/ogen_helpers.go
// **Authority**: api/openapi/data-storage-v1.yaml (ErrorDetails schema)
func toOptErrorDetails(errorDetails *sharedaudit.ErrorDetails) api.OptErrorDetails {
	// Use shared helper for type-safe conversion
	// **Pattern**: Eliminates switch statement duplication across services
	return sharedaudit.ToOgenOptErrorDetails(errorDetails)
}
