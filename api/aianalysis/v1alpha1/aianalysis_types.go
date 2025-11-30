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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ========================================
// AIAnalysisSpec - Design Decision: DD-CONTRACT-002
// V1.0: HolmesGPT-API only (no LLM config fields)
// Recovery: DD-RECOVERY-002 (direct recovery flow)
// ========================================

// AIAnalysisSpec defines the desired state of AIAnalysis.
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

	// Signal severity: critical, warning, info
	// +kubebuilder:validation:Enum=critical;warning;info
	Severity string `json:"severity"`

	// Signal type (e.g., OOMKilled, CrashLoopBackOff)
	// +kubebuilder:validation:Required
	SignalType string `json:"signalType"`

	// Environment classification
	// +kubebuilder:validation:Enum=production;staging;development
	Environment string `json:"environment"`

	// Business priority
	// +kubebuilder:validation:Enum=P0;P1;P2;P3
	BusinessPriority string `json:"businessPriority"`

	// Risk tolerance for this signal
	// +kubebuilder:validation:Enum=low;medium;high
	RiskTolerance string `json:"riskTolerance,omitempty"`

	// Business category
	BusinessCategory string `json:"businessCategory,omitempty"`

	// Target resource identification
	TargetResource TargetResource `json:"targetResource"`

	// Complete enrichment results from SignalProcessing
	// +kubebuilder:validation:Required
	EnrichmentResults EnrichmentResults `json:"enrichmentResults"`
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

// EnrichmentResults contains all enrichment data from SignalProcessing
// DD-CONTRACT-002: Matches SignalProcessing.Status.EnrichmentResults
type EnrichmentResults struct {
	// Kubernetes resource context (pod status, node conditions, etc.)
	KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

	// Auto-detected cluster characteristics - NO CONFIG NEEDED
	// SignalProcessing detects these from K8s resources automatically
	// Used by HolmesGPT-API for: workflow filtering + LLM context
	DetectedLabels *DetectedLabels `json:"detectedLabels,omitempty"`

	// OwnerChain: K8s ownership traversal from signal source resource
	// DD-WORKFLOW-001 v1.7: Used by HolmesGPT-API for 100% safe DetectedLabels validation
	// SignalProcessing traverses metadata.ownerReferences to build this chain
	// Example: Pod → ReplicaSet → Deployment
	// Empty chain = orphan resource (no owners)
	// HolmesGPT-API uses this to validate DetectedLabels applicability when RCA
	// identifies a different resource than the original signal source
	OwnerChain []OwnerChainEntry `json:"ownerChain,omitempty"`

	// Custom labels from Rego policies - CUSTOMER DEFINED
	// Key = subdomain/category (e.g., "constraint", "team", "region")
	// Value = list of label values (boolean keys or "key=value" pairs)
	// Example: {"constraint": ["cost-constrained", "stateful-safe"], "team": ["name=payments"]}
	// Passed through to HolmesGPT-API for workflow filtering + LLM context
	CustomLabels map[string][]string `json:"customLabels,omitempty"`

	// Overall enrichment quality score (0.0-1.0)
	// 1.0 = all enrichments successful, 0.0 = all failed
	// CONSUMER: Remediation Orchestrator (RO) - NOT for LLM/HolmesGPT
	// PURPOSE: RO uses this to detect degraded mode (< 0.8) and notify operators
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}

// OwnerChainEntry represents a single entry in the K8s ownership chain
// DD-WORKFLOW-001 v1.7: SignalProcessing traverses ownerReferences to build this
// Example chain for a Pod owned by Deployment:
//
//	[0]: {Namespace: "prod", Kind: "ReplicaSet", Name: "api-7d8f9c6b5"}
//	[1]: {Namespace: "prod", Kind: "Deployment", Name: "api"}
type OwnerChainEntry struct {
	// Namespace of the owner resource (empty for cluster-scoped resources like Node)
	Namespace string `json:"namespace,omitempty"`
	// Kind of the owner resource (e.g., ReplicaSet, Deployment, StatefulSet, DaemonSet)
	Kind string `json:"kind"`
	// Name of the owner resource
	Name string `json:"name"`
}

// DetectedLabels contains auto-detected cluster characteristics
// SignalProcessing populates these automatically from K8s resources
// Used by HolmesGPT-API for:
//   - Workflow filtering (deterministic SQL WHERE)
//   - LLM context (natural language in prompt)
type DetectedLabels struct {
	// ========================================
	// GITOPS MANAGEMENT
	// ========================================
	// True if namespace/deployment is managed by GitOps controller
	// Detection: ArgoCD annotations, Flux labels
	GitOpsManaged bool `json:"gitOpsManaged"`
	// GitOps tool managing this resource
	// +kubebuilder:validation:Enum=argocd;flux;""
	GitOpsTool string `json:"gitOpsTool,omitempty"`

	// ========================================
	// WORKLOAD PROTECTION
	// ========================================
	// True if PodDisruptionBudget exists for this workload
	PDBProtected bool `json:"pdbProtected"`
	// True if HorizontalPodAutoscaler targets this workload
	HPAEnabled bool `json:"hpaEnabled"`

	// ========================================
	// WORKLOAD CHARACTERISTICS
	// ========================================
	// True if StatefulSet or has PVCs attached
	Stateful bool `json:"stateful"`
	// True if managed by Helm (has helm.sh/chart label)
	HelmManaged bool `json:"helmManaged"`

	// ========================================
	// SECURITY POSTURE
	// ========================================
	// True if NetworkPolicy exists in namespace
	NetworkIsolated bool `json:"networkIsolated"`
	// Pod Security Standard level from namespace label
	// +kubebuilder:validation:Enum=privileged;baseline;restricted;""
	PodSecurityLevel string `json:"podSecurityLevel,omitempty"`
	// Service mesh if detected (from sidecar or namespace labels)
	// +kubebuilder:validation:Enum=istio;linkerd;""
	ServiceMesh string `json:"serviceMesh,omitempty"`
}

// KubernetesContext contains Kubernetes resource context
// Imported structure matching SignalProcessing types
type KubernetesContext struct {
	Namespace       string            `json:"namespace"`
	NamespaceLabels map[string]string `json:"namespaceLabels,omitempty"`
	PodDetails      *PodDetails       `json:"podDetails,omitempty"`
	NodeDetails     *NodeDetails      `json:"nodeDetails,omitempty"`
}

// PodDetails contains pod-level context
type PodDetails struct {
	Name              string            `json:"name"`
	Phase             string            `json:"phase"`
	Labels            map[string]string `json:"labels,omitempty"`
	RestartCount      int32             `json:"restartCount"`
	CreationTimestamp string            `json:"creationTimestamp"`
}

// NodeDetails contains node-level context
type NodeDetails struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

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

// AIAnalysisStatus defines the observed state of AIAnalysis.
type AIAnalysisStatus struct {
	// Phase tracking (no "Approving" phase - RO orchestrates approval)
	// +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Recommending;Completed;Failed
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`

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
	// APPROVAL SIGNALING
	// ========================================
	// True if approval is required (confidence < 80% or policy requires)
	ApprovalRequired bool `json:"approvalRequired"`
	// Reason why approval is required (when ApprovalRequired=true)
	ApprovalReason string `json:"approvalReason,omitempty"`
	// Rich context for approval notification
	ApprovalContext *ApprovalContext `json:"approvalContext,omitempty"`

	// ========================================
	// INVESTIGATION DETAILS
	// ========================================
	// HolmesGPT investigation ID for correlation
	// +kubebuilder:validation:MaxLength=253
	InvestigationID string `json:"investigationId,omitempty"`
	// LLM tokens consumed
	// +kubebuilder:validation:Minimum=0
	TokensUsed int `json:"tokensUsed,omitempty"`
	// Investigation duration in seconds
	// +kubebuilder:validation:Minimum=0
	InvestigationTime int64 `json:"investigationTime,omitempty"`

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
	// Severity determined by RCA
	// +kubebuilder:validation:Enum=critical;high;medium;low
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
