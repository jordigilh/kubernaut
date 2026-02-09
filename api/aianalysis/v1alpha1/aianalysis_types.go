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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ========================================
// AIAnalysisSpec - Design Decision: DD-CONTRACT-002
// V1.0: HolmesGPT-API only (no LLM config fields)
// Recovery: DD-RECOVERY-002 (direct recovery flow)
// ========================================

// AIAnalysisSpec defines the desired state of AIAnalysis.
//
// ADR-001: Spec Immutability
// AIAnalysis represents an immutable event (AI investigation).
// Once created by RemediationOrchestrator, spec cannot be modified to ensure:
// - Audit trail integrity (AI investigation matches original RCA request)
// - No tampering with RCA targets post-HAPI validation
// - No workflow selection modification after AI recommendation
//
// To re-analyze, delete and recreate the AIAnalysis CRD.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-001)"
type AIAnalysisSpec struct {
	// ========================================
	// PARENT REFERENCE (Audit/Lineage)
	// ========================================
	// Reference to parent RemediationRequest CRD for audit trail
	// +kubebuilder:validation:Required
	RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

	// Remediation ID for audit correlation (DD-WORKFLOW-002 v2.2)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RemediationID string `json:"remediationId"`

	// ========================================
	// ANALYSIS REQUEST (DD-CONTRACT-002)
	// ========================================
	// Complete analysis request with structured context
	// +kubebuilder:validation:Required
	AnalysisRequest AnalysisRequest `json:"analysisRequest"`

	// ========================================
	// RECOVERY FIELDS (DD-RECOVERY-002)
	// Populated by RO when this is a recovery attempt
	// ========================================
	// True if this AIAnalysis is for a recovery attempt (not initial incident)
	IsRecoveryAttempt bool `json:"isRecoveryAttempt,omitempty"`

	// Recovery attempt number (1, 2, 3...) - only set when IsRecoveryAttempt=true
	// +kubebuilder:validation:Minimum=1
	RecoveryAttemptNumber int `json:"recoveryAttemptNumber,omitempty"`

	// Previous execution history (only set when IsRecoveryAttempt=true)
	// Contains details about ALL previous attempts - allows LLM to:
	// 1. Avoid repeating failed approaches
	// 2. Learn from multiple failures
	// 3. Consider re-trying earlier approaches after later failures
	// Ordered chronologically: index 0 = first attempt, last index = most recent
	PreviousExecutions []PreviousExecution `json:"previousExecutions,omitempty"`

	// ========================================
	// TIMEOUT CONFIGURATION (REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md)
	// Replaces deprecated annotation-based timeout (security + validation)
	// Passed through from RR.Status.TimeoutConfig.AIAnalysisTimeout by RO (Gap #8: moved to Status)
	// ========================================
	// Optional timeout configuration for this analysis
	// If nil, AIAnalysis controller uses defaults (Investigating: 60s, Analyzing: 5s)
	// +optional
	TimeoutConfig *AIAnalysisTimeoutConfig `json:"timeoutConfig,omitempty"`
}

// AIAnalysisTimeoutConfig defines timeout settings for AIAnalysis phases
// Per REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md - Option A approved
type AIAnalysisTimeoutConfig struct {
	// Timeout for Investigating phase (HolmesGPT-API call)
	// Default: 60s if not specified
	// +optional
	InvestigatingTimeout metav1.Duration `json:"investigatingTimeout,omitempty"`

	// Timeout for Analyzing phase (Rego policy evaluation)
	// Default: 5s if not specified
	// +optional
	AnalyzingTimeout metav1.Duration `json:"analyzingTimeout,omitempty"`
}

// AnalysisRequest contains the structured analysis request
// DD-CONTRACT-002: Self-contained context for AIAnalysis
type AnalysisRequest struct {
	// Signal context from SignalProcessing enrichment
	// +kubebuilder:validation:Required
	SignalContext SignalContextInput `json:"signalContext"`

	// Analysis types to perform (e.g., "investigation", "root-cause", "workflow-selection")
	// +kubebuilder:validation:MinItems=1
	AnalysisTypes []string `json:"analysisTypes"`
}

// SignalContextInput contains enriched signal context from SignalProcessing
// DD-CONTRACT-002: Structured types replace map[string]string anti-pattern
type SignalContextInput struct {
	// Signal fingerprint for correlation
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=64
	Fingerprint string `json:"fingerprint"`

	// Signal severity: critical, high, medium, low, unknown (normalized by SignalProcessing Rego - DD-SEVERITY-001 v1.1)
	// +kubebuilder:validation:Enum=critical;high;medium;low;unknown
	Severity string `json:"severity"`

	// Signal type (e.g., OOMKilled, CrashLoopBackOff)
	// Normalized by SignalProcessing: predictive types mapped to base types (BR-SP-106)
	// +kubebuilder:validation:Required
	SignalType string `json:"signalType"`

	// SignalMode indicates whether this is a reactive or predictive signal.
	// BR-AI-084: Predictive Signal Mode Prompt Strategy
	// Copied from SignalProcessing status by RemediationOrchestrator.
	// Used by HAPI to switch investigation prompt (RCA vs. predict & prevent).
	// +kubebuilder:validation:Enum=reactive;predictive
	// +optional
	SignalMode string `json:"signalMode,omitempty"`

	// Environment classification
	// GAP-C3-01 FIX: Changed from enum to free-text (values defined by Rego policies)
	// Examples: "production", "staging", "development", "qa-eu", "canary"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Environment string `json:"environment"`

	// Business priority
	// GAP-C3-01 RELATED: Changed from enum to free-text for consistency
	// Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	BusinessPriority string `json:"businessPriority"`

	// GAP-C3-02 FIX: RiskTolerance REMOVED - now in CustomLabels via Rego policies
	// Per DD-WORKFLOW-001 v1.4: risk_tolerance is customer-derived, not system-controlled

	// GAP-C3-03 FIX: BusinessCategory REMOVED - now in CustomLabels via Rego policies
	// Per DD-WORKFLOW-001 v1.4: business_category is customer-derived, not mandatory

	// Target resource identification
	TargetResource TargetResource `json:"targetResource"`

	// Complete enrichment results from SignalProcessing
	// GAP-C3-04 FIX: Uses shared types from pkg/shared/types/enrichment.go
	// +kubebuilder:validation:Required
	EnrichmentResults sharedtypes.EnrichmentResults `json:"enrichmentResults"`
}

// TargetResource identifies the Kubernetes resource being remediated
type TargetResource struct {
	// Resource kind (e.g., Pod, Deployment, StatefulSet)
	Kind string `json:"kind"`
	// Resource name
	Name string `json:"name"`
	// Resource namespace
	Namespace string `json:"namespace"`
}

// ========================================
// TYPE ALIASES FOR CONVENIENCE (GAP-C3-04 FIX)
// ========================================
// These aliases allow AIAnalysis code to reference types without
// the sharedtypes prefix while maintaining single source of truth.
// Authoritative types in pkg/shared/types/enrichment.go

// EnrichmentResults alias - use sharedtypes.EnrichmentResults in new code
type EnrichmentResults = sharedtypes.EnrichmentResults

// OwnerChainEntry alias - use sharedtypes.OwnerChainEntry in new code
type OwnerChainEntry = sharedtypes.OwnerChainEntry

// DetectedLabels alias - use sharedtypes.DetectedLabels in new code
type DetectedLabels = sharedtypes.DetectedLabels

// KubernetesContext alias - use sharedtypes.KubernetesContext in new code
type KubernetesContext = sharedtypes.KubernetesContext

// PodDetails alias - use sharedtypes.PodDetails in new code
type PodDetails = sharedtypes.PodDetails

// NodeDetails alias - use sharedtypes.NodeDetails in new code
type NodeDetails = sharedtypes.NodeDetails

// DeploymentDetails alias - use sharedtypes.DeploymentDetails in new code
type DeploymentDetails = sharedtypes.DeploymentDetails

// ContainerStatus alias - use sharedtypes.ContainerStatus in new code
type ContainerStatus = sharedtypes.ContainerStatus

// ResourceList alias - use sharedtypes.ResourceList in new code
type ResourceList = sharedtypes.ResourceList

// NodeCondition alias - use sharedtypes.NodeCondition in new code
type NodeCondition = sharedtypes.NodeCondition

// ServiceSummary alias - use sharedtypes.ServiceSummary in new code
type ServiceSummary = sharedtypes.ServiceSummary

// ServicePort alias - use sharedtypes.ServicePort in new code
type ServicePort = sharedtypes.ServicePort

// IngressSummary alias - use sharedtypes.IngressSummary in new code
type IngressSummary = sharedtypes.IngressSummary

// IngressRule alias - use sharedtypes.IngressRule in new code
type IngressRule = sharedtypes.IngressRule

// ConfigMapSummary alias - use sharedtypes.ConfigMapSummary in new code
type ConfigMapSummary = sharedtypes.ConfigMapSummary

// ========================================
// RECOVERY CONTEXT (DD-RECOVERY-002)
// ========================================

// PreviousExecution contains context from a failed workflow execution
// DD-RECOVERY-002: Used when IsRecoveryAttempt=true
type PreviousExecution struct {
	// Reference to the failed WorkflowExecution CRD
	// +kubebuilder:validation:Required
	WorkflowExecutionRef string `json:"workflowExecutionRef"`

	// Original RCA from initial AIAnalysis
	// +kubebuilder:validation:Required
	OriginalRCA OriginalRCA `json:"originalRCA"`

	// Selected workflow that was executed and failed
	// +kubebuilder:validation:Required
	SelectedWorkflow SelectedWorkflowSummary `json:"selectedWorkflow"`

	// Structured failure information with Kubernetes reason codes
	// +kubebuilder:validation:Required
	Failure ExecutionFailure `json:"failure"`
}

// OriginalRCA summarizes the original root cause analysis
type OriginalRCA struct {
	// Brief RCA summary
	Summary string `json:"summary"`
	// Signal type determined by original RCA
	SignalType string `json:"signalType"`
	// Severity determined by original RCA
	Severity string `json:"severity"`
	// Contributing factors
	ContributingFactors []string `json:"contributingFactors,omitempty"`
}

// SelectedWorkflowSummary describes the workflow that was executed
type SelectedWorkflowSummary struct {
	// Workflow identifier
	WorkflowID string `json:"workflowId"`
	// Workflow version
	Version string `json:"version"`
	// Container image used
	ContainerImage string `json:"containerImage"`
	// Parameters passed to workflow
	Parameters map[string]string `json:"parameters,omitempty"`
	// Why this workflow was selected
	Rationale string `json:"rationale"`
}

// ExecutionFailure contains structured failure information
// Uses Kubernetes reason codes as API contract (DD-RECOVERY-003)
type ExecutionFailure struct {
	// Which step failed (0-indexed)
	FailedStepIndex int `json:"failedStepIndex"`
	// Name of the failed step
	FailedStepName string `json:"failedStepName"`
	// Kubernetes reason code (e.g., OOMKilled, DeadlineExceeded)
	// NOT natural language - structured enum-like value
	// +kubebuilder:validation:Required
	Reason string `json:"reason"`
	// Human-readable error message (for logging/debugging)
	Message string `json:"message"`
	// Exit code if applicable
	ExitCode *int32 `json:"exitCode,omitempty"`
	// When the failure occurred
	FailedAt metav1.Time `json:"failedAt"`
	// How long the execution ran before failing (e.g., "2m34s")
	ExecutionTime string `json:"executionTime"`
}

// ========================================
// APPROVAL CONTEXT (BR-AI-059, BR-AI-076)
// ========================================

// ApprovalContext contains rich context for approval notifications
type ApprovalContext struct {
	// Reason why approval is required
	Reason string `json:"reason"`
	// ConfidenceScore from AI analysis (0.0-1.0)
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	ConfidenceScore float64 `json:"confidenceScore"`
	// ConfidenceLevel: "low" | "medium" | "high"
	// +kubebuilder:validation:Enum=low;medium;high
	ConfidenceLevel string `json:"confidenceLevel"`
	// InvestigationSummary from HolmesGPT analysis
	InvestigationSummary string `json:"investigationSummary"`
	// EvidenceCollected that led to this conclusion
	EvidenceCollected []string `json:"evidenceCollected,omitempty"`
	// RecommendedActions with rationale
	RecommendedActions []RecommendedAction `json:"recommendedActions"`
	// AlternativesConsidered with pros/cons
	AlternativesConsidered []AlternativeApproach `json:"alternativesConsidered,omitempty"`
	// WhyApprovalRequired explains the need for human review
	WhyApprovalRequired string `json:"whyApprovalRequired"`
	// PolicyEvaluation contains Rego policy evaluation details (BR-AI-030)
	PolicyEvaluation *PolicyEvaluation `json:"policyEvaluation,omitempty"`
}

// PolicyEvaluation contains Rego policy evaluation results
type PolicyEvaluation struct {
	// Policy name that was evaluated
	PolicyName string `json:"policyName"`
	// Rules that matched
	MatchedRules []string `json:"matchedRules,omitempty"`
	// Decision: approved, manual_review_required, denied
	Decision string `json:"decision"`
}

// RecommendedAction describes a remediation action with rationale
type RecommendedAction struct {
	// Action type
	Action string `json:"action"`
	// Rationale explaining why this action is recommended
	Rationale string `json:"rationale"`
}

// AlternativeApproach describes an alternative approach with pros/cons
type AlternativeApproach struct {
	// Approach description
	Approach string `json:"approach"`
	// ProsCons analysis
	ProsCons string `json:"prosCons"`
}

// ========================================
// AIAnalysisStatus (DD-CONTRACT-002)
// ========================================

// AIAnalysis phase constants
const (
	// PhasePending is the initial phase when AIAnalysis is first created
	PhasePending = "Pending"
	// PhaseInvestigating calls HolmesGPT-API for investigation
	PhaseInvestigating = "Investigating"
	// PhaseAnalyzing evaluates Rego policies for approval determination
	PhaseAnalyzing = "Analyzing"
	// PhaseCompleted indicates successful completion
	PhaseCompleted = "Completed"
	// PhaseFailed indicates a permanent failure
	PhaseFailed = "Failed"
)

// AIAnalysisStatus defines the observed state of AIAnalysis.
type AIAnalysisStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// Used to prevent duplicate reconciliations and ensure idempotency.
	// Per DD-CONTROLLER-001: Standard pattern for all Kubernetes controllers.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase tracking (no "Approving" or "Recommending" phase - simplified 4-phase flow)
	// Per reconciliation-phases.md v2.0: Pending → Investigating → Analyzing → Completed/Failed
	// +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed;Failed
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
	// Reason provides the umbrella failure category (e.g., "WorkflowResolutionFailed")
	Reason string `json:"reason,omitempty"`
	// SubReason provides specific failure cause within the Reason category
	// BR-HAPI-197: Maps to needs_human_review triggers from HolmesGPT-API
	// BR-HAPI-200: Added InvestigationInconclusive, ProblemResolved for new investigation outcomes
	// +kubebuilder:validation:Enum=WorkflowNotFound;ImageMismatch;ParameterValidationFailed;NoMatchingWorkflows;LowConfidence;LLMParsingError;ValidationError;TransientError;PermanentError;InvestigationInconclusive;ProblemResolved;MaxRetriesExceeded
	// +optional
	SubReason string `json:"subReason,omitempty"`

	// Timestamps
	StartedAt   *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// ========================================
	// ROOT CAUSE ANALYSIS RESULTS
	// ========================================
	// Identified root cause
	RootCause string `json:"rootCause,omitempty"`
	// Root cause analysis details
	RootCauseAnalysis *RootCauseAnalysis `json:"rootCauseAnalysis,omitempty"`

	// ========================================
	// SELECTED WORKFLOW (DD-CONTRACT-002)
	// ========================================
	// Selected workflow for execution (populated when phase=Completed)
	SelectedWorkflow *SelectedWorkflow `json:"selectedWorkflow,omitempty"`

	// ========================================
	// ALTERNATIVE WORKFLOWS (Dec 2025)
	// ========================================
	// Alternative workflows considered but not selected.
	// INFORMATIONAL ONLY - NOT for automatic execution.
	// Helps operators make informed approval decisions and provides audit trail.
	// Per HolmesGPT-API team: Alternatives are for CONTEXT, not EXECUTION.
	// +optional
	AlternativeWorkflows []AlternativeWorkflow `json:"alternativeWorkflows,omitempty"`

	// ========================================
	// APPROVAL SIGNALING
	// ========================================
	// True if approval is required (confidence < 80% or policy requires)
	ApprovalRequired bool `json:"approvalRequired"`
	// Reason why approval is required (when ApprovalRequired=true)
	ApprovalReason string `json:"approvalReason,omitempty"`
	// Rich context for approval notification
	ApprovalContext *ApprovalContext `json:"approvalContext,omitempty"`

	// ========================================
	// HUMAN REVIEW SIGNALING (BR-HAPI-197)
	// Set by HAPI when AI cannot produce reliable result
	// ========================================
	// True if human review required (HAPI decision: RCA incomplete/unreliable)
	// BR-HAPI-197: Triggers NotificationRequest creation in RO
	// BR-HAPI-212: Set when workflow selected but affectedResource missing
	NeedsHumanReview bool `json:"needsHumanReview"`
	// Reason why human review needed (when NeedsHumanReview=true)
	// BR-HAPI-197: Maps to HAPI's human_review_reason enum values
	// BR-HAPI-212: Includes "rca_incomplete" for missing affectedResource
	// +kubebuilder:validation:Enum=workflow_not_found;image_mismatch;parameter_validation_failed;no_matching_workflows;low_confidence;llm_parsing_error;investigation_inconclusive;rca_incomplete
	// +optional
	HumanReviewReason string `json:"humanReviewReason,omitempty"`

	// ========================================
	// INVESTIGATION DETAILS
	// ========================================
	// HolmesGPT investigation ID for correlation
	// +kubebuilder:validation:MaxLength=253
	InvestigationID string `json:"investigationId,omitempty"`
	// NOTE: TokensUsed REMOVED (Dec 2025)
	// Reason: LLM token tracking is HAPI's responsibility (they call the LLM)
	// Observability: HAPI exposes holmesgpt_llm_token_usage_total Prometheus metric
	// Correlation: Use InvestigationID to link AIAnalysis CRD to HAPI metrics
	// Design Decision: DD-COST-001 - Cost observability is provider's responsibility
	// Investigation duration in seconds
	// +kubebuilder:validation:Minimum=0
	InvestigationTime int64 `json:"investigationTime,omitempty"`

	// ========================================
	// HAPI RESPONSE METADATA (Dec 2025)
	// ========================================
	// Whether the RCA-identified target resource was found in OwnerChain
	// If false, DetectedLabels may be from different scope than affected resource
	// Used for: Rego policy input, audit trail, operator notifications, metrics
	TargetInOwnerChain *bool `json:"targetInOwnerChain,omitempty"`
	// Non-fatal warnings from HolmesGPT-API (e.g., OwnerChain validation, low confidence)
	Warnings []string `json:"warnings,omitempty"`
	// ValidationAttemptsHistory contains complete history of all HAPI validation attempts
	// Per DD-HAPI-002 v1.4: HAPI retries up to 3 times with LLM self-correction
	// This field provides audit trail for operator notifications and debugging
	// +optional
	ValidationAttemptsHistory []ValidationAttempt `json:"validationAttemptsHistory,omitempty"`

	// ========================================
	// OPERATIONAL STATUS
	// ========================================
	// DegradedMode indicates if the analysis ran with degraded capabilities
	// (e.g., Rego policy evaluation failed, using safe defaults)
	DegradedMode bool `json:"degradedMode,omitempty"`
	// TotalAnalysisTime is the total duration of the analysis in seconds
	// +kubebuilder:validation:Minimum=0
	TotalAnalysisTime int64 `json:"totalAnalysisTime,omitempty"`
	// ConsecutiveFailures tracks retry attempts for exponential backoff
	// BR-AI-009: Reset to 0 on success, increment on transient failure
	// Used with pkg/shared/backoff for retry logic with jitter
	// +kubebuilder:validation:Minimum=0
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

	// ========================================
	// RECOVERY STATUS (DD-RECOVERY-002)
	// ========================================
	// Recovery-specific status fields (only populated for recovery attempts)
	RecoveryStatus *RecoveryStatus `json:"recoveryStatus,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RootCauseAnalysis contains detailed RCA results
type RootCauseAnalysis struct {
	// Brief summary of root cause
	Summary string `json:"summary"`
	// Severity determined by RCA (normalized per DD-SEVERITY-001 v1.1)
	// DD-SEVERITY-001 v1.1: Aligned with HAPI/workflow catalog (critical, high, medium, low, unknown)
	// +kubebuilder:validation:Enum=critical;high;medium;low;unknown
	Severity string `json:"severity"`
	// Signal type determined by RCA (may differ from input)
	SignalType string `json:"signalType"`
	// Contributing factors
	ContributingFactors []string `json:"contributingFactors,omitempty"`
}

// SelectedWorkflow contains the AI-selected workflow for execution
// DD-CONTRACT-002: Output format for RO to create WorkflowExecution
type SelectedWorkflow struct {
	// Workflow identifier (catalog lookup key)
	// +kubebuilder:validation:Required
	WorkflowID string `json:"workflowId"`
	// Workflow version
	// +kubebuilder:validation:Required
	Version string `json:"version"`
	// Container image (OCI bundle) - resolved by HolmesGPT-API
	// +kubebuilder:validation:Required
	ContainerImage string `json:"containerImage"`
	// Container digest for audit trail
	ContainerDigest string `json:"containerDigest,omitempty"`
	// Confidence score (0.0-1.0)
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Confidence float64 `json:"confidence"`
	// Workflow parameters (UPPER_SNAKE_CASE keys per DD-WORKFLOW-003)
	Parameters map[string]string `json:"parameters,omitempty"`
	// Rationale explaining why this workflow was selected
	Rationale string `json:"rationale"`
	// ExecutionEngine specifies the backend engine for workflow execution.
	// Populated from HolmesGPT-API workflow recommendation.
	// "tekton" creates a Tekton PipelineRun; "job" creates a Kubernetes Job.
	// When empty, defaults to "tekton" for backwards compatibility.
	// +kubebuilder:validation:Enum=tekton;job
	// +optional
	ExecutionEngine string `json:"executionEngine,omitempty"`
}

// AlternativeWorkflow contains alternative workflows considered but not selected.
// INFORMATIONAL ONLY - NOT for automatic execution.
// Helps operators understand AI reasoning during approval decisions.
// Per HolmesGPT-API team (Dec 5, 2025): Alternatives are for CONTEXT, not EXECUTION.
type AlternativeWorkflow struct {
	// Workflow identifier (catalog lookup key)
	// +kubebuilder:validation:Required
	WorkflowID string `json:"workflowId"`
	// Container image (OCI bundle) - resolved by HolmesGPT-API
	ContainerImage string `json:"containerImage,omitempty"`
	// Confidence score (0.0-1.0) - shows why it wasn't selected
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Confidence float64 `json:"confidence"`
	// Rationale explaining why this workflow was considered
	Rationale string `json:"rationale"`
}

// ValidationAttempt contains details of a single HAPI validation attempt
// Per DD-HAPI-002 v1.4: HAPI retries up to 3 times with LLM self-correction
// Each attempt feeds validation errors back to the LLM for correction
type ValidationAttempt struct {
	// Attempt number (1, 2, or 3)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	Attempt int `json:"attempt"`
	// WorkflowID that the LLM tried in this attempt
	WorkflowID string `json:"workflowId"`
	// Whether validation passed (always false for failed attempts in history)
	IsValid bool `json:"isValid"`
	// Validation errors encountered
	Errors []string `json:"errors,omitempty"`
	// When this attempt occurred
	Timestamp metav1.Time `json:"timestamp"`
}

// RecoveryStatus contains recovery-specific status information
// DD-RECOVERY-002: Tracks recovery attempt progress
type RecoveryStatus struct {
	// Assessment of why previous attempt failed
	PreviousAttemptAssessment *PreviousAttemptAssessment `json:"previousAttemptAssessment,omitempty"`
	// Whether the signal type changed due to the failed workflow
	StateChanged bool `json:"stateChanged"`
	// Current signal type (may differ from original after failed workflow)
	CurrentSignalType string `json:"currentSignalType,omitempty"`
}

// PreviousAttemptAssessment contains analysis of the failed attempt
type PreviousAttemptAssessment struct {
	// Whether the failure was understood
	FailureUnderstood bool `json:"failureUnderstood"`
	// Analysis of why the failure occurred
	FailureReasonAnalysis string `json:"failureReasonAnalysis"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Confidence",type=number,JSONPath=`.status.selectedWorkflow.confidence`
// +kubebuilder:printcolumn:name="ApprovalRequired",type=boolean,JSONPath=`.status.approvalRequired`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIAnalysis is the Schema for the aianalyses API.
type AIAnalysis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIAnalysisSpec   `json:"spec,omitempty"`
	Status AIAnalysisStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIAnalysisList contains a list of AIAnalysis.
type AIAnalysisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIAnalysis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIAnalysis{}, &AIAnalysisList{})
}
