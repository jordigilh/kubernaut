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

// Package audit provides structured event data types for AIAnalysis audit events.
//
// Authority: DD-AUDIT-004 (Structured Types for Audit Event Payloads)
// Related: AA_AUDIT_TYPE_SAFETY_VIOLATION_TRIAGE.md
//
// These types provide compile-time type safety for audit event payloads,
// addressing the project coding standard requirement to avoid map[string]interface{}.
package audit

// AnalysisCompletePayload is the structured payload for analysis completion events.
//
// Business Requirements:
// - BR-AI-001: AI Analysis CRD lifecycle management
// - BR-STORAGE-001: Complete audit trail
type AnalysisCompletePayload struct {
	// Core Status Fields
	Phase            string `json:"phase"`                     // Current phase (Completed, Failed)
	ApprovalRequired bool   `json:"approval_required"`         // Whether manual approval is required
	ApprovalReason   string `json:"approval_reason,omitempty"` // Reason for approval requirement
	DegradedMode     bool   `json:"degraded_mode"`             // Whether operating in degraded mode
	WarningsCount    int    `json:"warnings_count"`            // Number of warnings encountered

	// Workflow Selection (conditional - present when workflow selected)
	Confidence         *float64 `json:"confidence,omitempty"`            // Workflow selection confidence (0.0-1.0)
	WorkflowID         *string  `json:"workflow_id,omitempty"`           // Selected workflow identifier
	TargetInOwnerChain *bool    `json:"target_in_owner_chain,omitempty"` // Whether target is in owner chain

	// Failure Information (conditional - present on failure)
	Reason    string `json:"reason,omitempty"`     // Primary failure reason
	SubReason string `json:"sub_reason,omitempty"` // Detailed failure sub-reason
}

// PhaseTransitionPayload is the structured payload for phase transition events.
//
// Business Requirements:
// - BR-AI-001: Phase state machine tracking
// - DD-AUDIT-003: Phase transition audit trail
type PhaseTransitionPayload struct {
	OldPhase string `json:"old_phase"` // Previous phase
	NewPhase string `json:"new_phase"` // New phase
}

// HolmesGPTCallPayload is the structured payload for HolmesGPT-API call events.
//
// Business Requirements:
// - BR-AI-006: HolmesGPT-API integration tracking
// - DD-AUDIT-003: External API call audit
type HolmesGPTCallPayload struct {
	Endpoint       string `json:"endpoint"`          // API endpoint called (e.g., "/api/v1/investigate")
	HTTPStatusCode int    `json:"http_status_code"` // HTTP status code (200, 500, etc.)
	DurationMs     int    `json:"duration_ms"`      // Call duration in milliseconds
}

// ApprovalDecisionPayload is the structured payload for approval decision events.
//
// Business Requirements:
// - BR-AI-011: Data quality approval decisions
// - BR-AI-013: Production approval requirements
type ApprovalDecisionPayload struct {
	ApprovalRequired bool   `json:"approval_required"` // Whether manual approval is required
	ApprovalReason   string `json:"approval_reason"`   // Reason for approval requirement
	AutoApproved     bool   `json:"auto_approved"`     // Whether auto-approved
	Decision         string `json:"decision"`          // Decision made (e.g., "auto-approved", "requires_approval")
	Reason           string `json:"reason"`            // Reason for decision
	Environment      string `json:"environment"`       // Environment context (production, staging, etc.)

	// Workflow Context (conditional - present when workflow selected)
	Confidence *float64 `json:"confidence,omitempty"`  // Workflow confidence level
	WorkflowID *string  `json:"workflow_id,omitempty"` // Selected workflow identifier
}

// RegoEvaluationPayload is the structured payload for Rego policy evaluation events.
//
// Business Requirements:
// - BR-AI-030: Rego policy evaluation tracking
// - DD-AUDIT-003: Policy decision audit trail
type RegoEvaluationPayload struct {
	Outcome    string `json:"outcome"`     // Evaluation outcome (e.g., "allow", "deny")
	Degraded   bool   `json:"degraded"`    // Whether evaluation ran in degraded mode
	DurationMs int    `json:"duration_ms"` // Evaluation duration in milliseconds
	Reason     string `json:"reason"`      // Reason for the evaluation outcome
}

// ErrorPayload is the structured payload for error events.
//
// Business Requirements:
// - BR-AI-009: Error tracking and diagnosis
// - DD-AUDIT-003: Comprehensive error audit trail
type ErrorPayload struct {
	Phase string `json:"phase"` // Phase in which error occurred
	Error string `json:"error"` // Error message
}
