/*
Copyright 2026 Jordi Gil.

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

package types

// Phase represents a stage in the investigation flow.
// Per DD-HAPI-019-002: RCA uses K8s+Prom tools, Discovery uses workflow tools,
// Validation uses no tools (I4 per-phase tool scoping).
type Phase string

const (
	PhaseRCA               Phase = "rca"
	PhaseWorkflowDiscovery Phase = "workflow_discovery"
	PhaseValidation        Phase = "validation"
)

// PhaseToolMap defines which tool names are available in each phase (I4).
type PhaseToolMap map[Phase][]string

// InvestigationResult holds the final output of an investigation.
// Fields align with the OpenAPI IncidentResponse schema in api/openapi.json.
type InvestigationResult struct {
	// Core RCA output
	RCASummary        string                 `json:"rca_summary"`
	Severity          string                 `json:"severity,omitempty"`
	WorkflowID        string                 `json:"workflow_id,omitempty"`
	RemediationTarget RemediationTarget      `json:"remediation_target,omitempty"`
	Parameters        map[string]interface{} `json:"parameters,omitempty"`
	Confidence        float64                `json:"confidence"`

	// Workflow selection (GAP-009: OpenAPI selected_workflow includes execution_bundle)
	ExecutionBundle       string `json:"execution_bundle,omitempty"`
	ExecutionBundleDigest string `json:"execution_bundle_digest,omitempty"`
	ExecutionEngine       string `json:"execution_engine,omitempty"`
	ServiceAccountName    string `json:"service_account_name,omitempty"`
	WorkflowVersion       string `json:"workflow_version,omitempty"`
	WorkflowRationale     string `json:"workflow_rationale,omitempty"`

	// Human review fields (GAP-013: HumanReviewReason for OpenAPI enum mapping)
	HumanReviewNeeded bool   `json:"human_review_needed"`
	HumanReviewReason string `json:"human_review_reason,omitempty"`
	Reason            string `json:"reason,omitempty"`

	// Outcome routing (is_actionable already existed, GAP-002 population in P3)
	IsActionable *bool `json:"is_actionable,omitempty"`

	// InvestigationOutcome preserves the raw investigation_outcome string from
	// the LLM (e.g., "actionable", "inconclusive", "problem_resolved"). Used
	// for Phase 1→3 propagation so the investigator can merge Phase 1 outcomes
	// as fallbacks when Phase 3 does not produce one (HAPI parity: #715).
	InvestigationOutcome string `json:"investigation_outcome,omitempty"`

	// Investigation analysis narrative from Phase 1 for Phase 3 context (#724).
	InvestigationAnalysis string `json:"investigation_analysis,omitempty"`

	// RCA detail
	SignalName          string   `json:"signal_name,omitempty"`
	ContributingFactors []string `json:"contributing_factors,omitempty"`

	// Observability
	Warnings       []string               `json:"warnings,omitempty"`
	DetectedLabels map[string]interface{} `json:"detected_labels,omitempty"`

	// Alternative workflows for operator context (GAP-009: OpenAPI AlternativeWorkflow schema)
	AlternativeWorkflows []AlternativeWorkflow `json:"alternative_workflows,omitempty"`

	// History of validation attempts during self-correction (DD-HAPI-002 v1.2).
	// Populated by Validator.SelfCorrect when validation fails and retries occur.
	ValidationAttemptsHistory []ValidationAttemptRecord `json:"validation_attempts_history,omitempty"`
}

// ValidationAttemptRecord captures a single validation attempt during
// the LLM self-correction loop. Maps to OpenAPI ValidationAttempt schema.
type ValidationAttemptRecord struct {
	Attempt    int      `json:"attempt"`
	WorkflowID string   `json:"workflow_id,omitempty"`
	IsValid    bool     `json:"is_valid"`
	Errors     []string `json:"errors,omitempty"`
	Timestamp  string   `json:"timestamp"`
}

// AlternativeWorkflow represents a workflow considered but not selected.
// Matches the OpenAPI AlternativeWorkflow schema (ADR-045 v1.2).
// Alternatives are for CONTEXT and AUDIT only, not for automatic fallback execution.
type AlternativeWorkflow struct {
	WorkflowID      string  `json:"workflow_id"`
	ExecutionBundle string  `json:"execution_bundle,omitempty"`
	Confidence      float64 `json:"confidence"`
	Rationale       string  `json:"rationale"`
}

// RemediationTarget identifies the K8s resource to remediate.
type RemediationTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SignalContext holds the input signal data for an investigation.
// Fields align with the OpenAPI IncidentRequest schema in api/openapi.json.
type SignalContext struct {
	// Required fields
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`

	// Identifiers (GAP-008: RemediationID for audit correlation per DD-WORKFLOW-002)
	IncidentID    string `json:"incident_id,omitempty"`
	RemediationID string `json:"remediation_id,omitempty"`

	// Resource targeting
	ResourceKind string `json:"resource_kind,omitempty"`
	ResourceName string `json:"resource_name,omitempty"`

	// Environment context
	ClusterName      string `json:"cluster_name,omitempty"`
	Environment      string `json:"environment,omitempty"`
	Priority         string `json:"priority,omitempty"`
	RiskTolerance    string `json:"risk_tolerance,omitempty"`
	SignalSource     string `json:"signal_source,omitempty"`
	BusinessCategory string `json:"business_category,omitempty"`
	Description      string `json:"description,omitempty"`
	SignalMode       string `json:"signal_mode,omitempty"`

	// Timestamps and dedup (GAP-014: from IncidentRequest OpenAPI schema)
	FiringTime      string `json:"firing_time,omitempty"`
	ReceivedTime    string `json:"received_time,omitempty"`
	IsDuplicate     *bool  `json:"is_duplicate,omitempty"`
	OccurrenceCount *int   `json:"occurrence_count,omitempty"`
}
