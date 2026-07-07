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

// HumanReviewReasonAlignmentCheckFailed is the sentinel value for
// InvestigationResult.HumanReviewReason when the shadow agent alignment
// check has flagged suspicious content. Used across session lifecycle
// (store, manager) and event emission to identify security escalations
// that must bypass interactive hold.
const HumanReviewReasonAlignmentCheckFailed = "alignment_check_failed"

// PhaseToolMap defines which tool names are available in each phase (I4).
type PhaseToolMap map[Phase][]string

// ReasoningSummary is the audit-safe subset of an LLM's reasoning/thinking
// output for the RCA that produced an InvestigationResult (BR-AI-086 AC6).
// Only the visible Text and a Redacted flag are carried here — the opaque,
// provider-specific replay Signature (llm.ReasoningBlock.Signature) never
// reaches this type by design: it has no forensic/reconstruction value
// (SOC2 CC8.1 cares about decision provenance, not protocol replay tokens)
// and would be a pure retention-cost liability (AU-11) with no compliance
// upside. See DD-LLM-005/BR-AI-086 AC6 for the full rationale.
type ReasoningSummary struct {
	// Text is the visible reasoning content that led to this result. Empty
	// when Redacted is true or when the provider returned no visible text.
	Text string `json:"text,omitempty"`
	// Redacted marks that the provider withheld the reasoning content
	// (e.g. Anthropic redacted_thinking) — reasoning occurred, but its
	// content could not be captured for audit.
	Redacted bool `json:"redacted,omitempty"`
}

// InvestigationResult holds the final output of an investigation.
// Fields align with the OpenAPI IncidentResponse schema in api/openapi.json.
//
// Wide DTO by design: reviewed in GO-ANTIPATTERN-AUDIT-2026-07-01 §4a (God Structs).
// It mirrors an external API schema field-for-field; splitting it would fragment
// the JSON serialization boundary for no behavioral gain, so it is intentionally
// not decomposed.
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

	// Adversarial Due Diligence (Issue #847 / DD-HAPI-847)
	CausalChain  []string            `json:"causal_chain,omitempty"`
	DueDiligence *DueDiligenceReview `json:"due_diligence,omitempty"`

	// Observability
	Warnings       []string               `json:"warnings,omitempty"`
	DetectedLabels map[string]interface{} `json:"detected_labels,omitempty"`

	// Alternative workflows for operator context (GAP-009: OpenAPI AlternativeWorkflow schema)
	AlternativeWorkflows []AlternativeWorkflow `json:"alternative_workflows,omitempty"`

	// History of validation attempts during self-correction (DD-HAPI-002 v1.2).
	// Populated by Validator.SelfCorrect when validation fails and retries occur.
	ValidationAttemptsHistory []ValidationAttemptRecord `json:"validation_attempts_history,omitempty"`

	// TotalLLMTurns tracks the total number of LLM inference calls made across
	// all investigation phases (RCA + workflow selection). Surfaced in the
	// structured decision payload for observability (#1396).
	TotalLLMTurns int `json:"total_llm_turns,omitempty"`

	// TotalToolCalls tracks the total number of tool executions dispatched
	// across all investigation phases. Surfaced in the structured decision
	// payload for observability (#1396).
	TotalToolCalls int `json:"total_tool_calls,omitempty"`

	// Cancelled indicates the investigation was aborted by operator action
	// (BR-SESSION-001). When true, the result contains partial accumulated
	// state up to the point of cancellation.
	Cancelled bool `json:"cancelled,omitempty"`

	// CancelledPhase records which investigation phase was active when
	// cancellation occurred ("rca" or "workflow_discovery").
	CancelledPhase string `json:"cancelled_phase,omitempty"`

	// CancelledAtTurn records which LLM conversation turn was active when
	// cancellation was detected. Together with CancelledPhase, this enables
	// full audit reconstruction of investigation progress (SOC2 CC8.1).
	CancelledAtTurn int `json:"cancelled_at_turn,omitempty"`

	// AccumulatedMessages holds the LLM conversation accumulated before
	// cancellation. Serialized into audit events for forensic post-mortem
	// RAG (BR-AUDIT-070). Only populated when Cancelled is true.
	AccumulatedMessages []map[string]interface{} `json:"accumulated_messages,omitempty"`

	// TokenUsage holds cumulative token counts at the point of result
	// construction. For cancelled investigations this captures spend up to
	// cancellation; for completed investigations it captures total spend.
	// Used for cost attribution and forensic audit (BR-AUDIT-070, CC6.1).
	TokenUsage *TokenUsageSummary `json:"token_usage,omitempty"`

	// AlignmentVerdict holds the shadow agent's alignment check result.
	// Populated by InvestigatorWrapper for ALL investigations (aligned or suspicious).
	// BR-AI-601, #1076: When CircuitBreakerActivated=true, the primary LLM results
	// may be incomplete or compromised; shadow findings are the primary content.
	AlignmentVerdict *AlignmentVerdictResult `json:"alignment_verdict,omitempty"`

	// BR-INTERACTIVE-010: When true, the session transitions to StatusUserDriving
	// instead of StatusCompleted. Set by the investigator when signal.Interactive=true
	// and RCA completes (Phase 2+3 skipped).
	InteractiveHold bool `json:"interactive_hold,omitempty"`

	// Reasoning carries an audit-safe summary of the LLM's reasoning/
	// thinking content that led to this RCA, when the model's reasoning
	// capability was enabled (BR-AI-086 AC6). Nil when reasoning was not
	// requested/returned — this is the default, unaffected behavior.
	Reasoning *ReasoningSummary `json:"reasoning,omitempty"`
}

// TokenUsageSummary holds cumulative token counts. Mirrors
// investigator.TokenUsageSummary for cross-package use.
type TokenUsageSummary struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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
	WorkflowID      string                 `json:"workflow_id"`
	ExecutionBundle string                 `json:"execution_bundle,omitempty"`
	Confidence      float64                `json:"confidence"`
	Rationale       string                 `json:"rationale"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
}

// AlignmentVerdictResult holds the shadow agent's overall verdict on an investigation.
// Produced by InvestigatorWrapper and consumed by KA handler (mapped to ogen IncidentResponse)
// and AA response processor (mapped to AIAnalysisStatus.AlignmentVerdict).
type AlignmentVerdictResult struct {
	Result                  string                    `json:"result"`
	CircuitBreakerActivated bool                      `json:"circuit_breaker_activated"`
	Summary                 string                    `json:"summary"`
	Flagged                 int                       `json:"flagged"`
	Total                   int                       `json:"total"`
	Findings                []AlignmentFinding        `json:"findings,omitempty"`
	GroundingReview         *AlignmentGroundingResult `json:"grounding_review,omitempty"`
}

// AlignmentGroundingResult holds the structured outcome of the full-context
// grounding review (#1096). Populated when grounding review is enabled.
type AlignmentGroundingResult struct {
	Grounded    bool   `json:"grounded"`
	Explanation string `json:"explanation"`
}

// AlignmentFinding captures a single suspicious step detected by the shadow agent.
type AlignmentFinding struct {
	StepIndex   int    `json:"step_index"`
	StepKind    string `json:"step_kind"`
	Tool        string `json:"tool,omitempty"`
	Explanation string `json:"explanation"`
}

// DueDiligenceReview captures the LLM's adversarial self-review across 8 dimensions.
// Enforced by JSON schema in rcaResultSchemaJSON (Issue #847).
type DueDiligenceReview struct {
	CausalCompleteness    string `json:"causal_completeness"`
	TargetAccuracy        string `json:"target_accuracy"`
	EvidenceSufficiency   string `json:"evidence_sufficiency"`
	AlternativeHypotheses string `json:"alternative_hypotheses"`
	ScopeCompleteness     string `json:"scope_completeness"`
	Proportionality       string `json:"proportionality"`
	RegressionAwareness   string `json:"regression_awareness"`
	ConfidenceCalibration string `json:"confidence_calibration"`
}

// RemediationTarget identifies the K8s resource to remediate.
type RemediationTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// APIVersion disambiguates the resource's API group when the Kind exists
	// in multiple groups (e.g. Route in route.openshift.io vs serving.knative.dev).
	// Format: "group/version" (e.g. "route.openshift.io/v1") or "version" for core (e.g. "v1").
	// Issue #1040.
	APIVersion string `json:"api_version,omitempty"`
}

// SignalContext holds the input signal data for an investigation.
// Fields align with the OpenAPI IncidentRequest schema in api/openapi.json.
//
// Wide DTO by design: reviewed in GO-ANTIPATTERN-AUDIT-2026-07-01 §4a (God Structs).
// It mirrors an external API schema field-for-field; splitting it would fragment
// the JSON serialization boundary for no behavioral gain, so it is intentionally
// not decomposed.
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
	ResourceKind       string `json:"resource_kind,omitempty"`
	ResourceName       string `json:"resource_name,omitempty"`
	ResourceAPIVersion string `json:"resource_api_version,omitempty"`

	// Environment context
	ClusterID        string `json:"cluster_id,omitempty"`
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

	// #462: Alert-author annotations and signal labels from IncidentRequest
	SignalAnnotations map[string]string `json:"signal_annotations,omitempty"`
	SignalLabels      map[string]string `json:"signal_labels,omitempty"`

	// DetectedLabelsJSON is a pre-marshaled JSON string of sharedtypes.DetectedLabels,
	// forwarded to DS catalog queries to activate GitOps-aware scoring. Issue #1052 / BR-AI-056.
	DetectedLabelsJSON string `json:"detected_labels_json,omitempty"`

	DeduplicationWindowMinutes *int   `json:"deduplication_window_minutes,omitempty"`
	FirstSeen                  string `json:"first_seen,omitempty"`
	LastSeen                   string `json:"last_seen,omitempty"`

	// BR-INTERACTIVE-010: When true, KA creates the session in pending state
	// without launching Investigate(). The session awaits MCP action=start.
	Interactive bool `json:"interactive,omitempty"`

	// ClusterClassification is the optional cluster business classification
	// (e.g. "production", "staging-eu") propagated from
	// AIAnalysis.Spec.AnalysisRequest.SignalContext.Cluster (BR-FLEET-003,
	// #1511). Empty for non-fleet deployments or unregistered clusters.
	// Passed as the `cluster` param on the workflow-discovery tool call.
	ClusterClassification string `json:"cluster_classification,omitempty"`
}

// ComponentGVK returns the fully-qualified apiVersion/Kind string for workflow
// component matching (Issue #1051). Returns empty string when either
// ResourceAPIVersion or ResourceKind is not set, preventing silent mismatch
// against workflow labels that use GVK format.
func (s SignalContext) ComponentGVK() string {
	if s.ResourceAPIVersion == "" || s.ResourceKind == "" {
		return ""
	}
	return s.ResourceAPIVersion + "/" + s.ResourceKind
}
