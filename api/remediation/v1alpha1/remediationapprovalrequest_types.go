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

// ========================================
// RemediationApprovalRequest CRD
// Design Decision: ADR-040
// Pattern: Follows Kubernetes CertificateSigningRequest
// ========================================

// ApprovalDecision represents the operator's decision on an approval request
// +kubebuilder:validation:Enum="";Approved;Rejected;Expired
type ApprovalDecision string

const (
	// ApprovalDecisionPending indicates no decision has been made yet
	ApprovalDecisionPending ApprovalDecision = ""
	// ApprovalDecisionApproved indicates the operator approved the remediation
	ApprovalDecisionApproved ApprovalDecision = "Approved"
	// ApprovalDecisionRejected indicates the operator rejected the remediation
	ApprovalDecisionRejected ApprovalDecision = "Rejected"
	// ApprovalDecisionExpired indicates the approval request timed out
	ApprovalDecisionExpired ApprovalDecision = "Expired"
)

// ObjectRef is a lightweight reference to another object in the same namespace
type ObjectRef struct {
	// Name of the referenced object
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RemediationApprovalRequestSpec defines the desired state of RemediationApprovalRequest.
//
// ADR-040: Spec Immutability
// ALL spec fields are immutable after CRD creation (follows CertificateSigningRequest pattern).
// This provides a complete audit trail and prevents race conditions.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-040)"
type RemediationApprovalRequestSpec struct {
	// ========================================
	// PARENT REFERENCES
	// ========================================

	// Reference to parent RemediationRequest CRD (owner)
	// RemediationRequest owns this CRD via ownerReferences (flat hierarchy per ADR-001)
	// +kubebuilder:validation:Required
	RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

	// Reference to the AIAnalysis that requires approval
	// Used by AIAnalysis controller for efficient field-indexed lookup (ADR-040)
	// +kubebuilder:validation:Required
	AIAnalysisRef ObjectRef `json:"aiAnalysisRef"`

	// ========================================
	// APPROVAL CONTEXT
	// ========================================

	// Confidence score from AI analysis (0.0-1.0)
	// Typically 0.6-0.79 triggers approval (below auto-approve threshold)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Confidence float64 `json:"confidence"`

	// Confidence level derived from score
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=low;medium;high
	ConfidenceLevel string `json:"confidenceLevel"`

	// Reason why approval is required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Reason string `json:"reason"`

	// Recommended workflow from AI analysis
	// +kubebuilder:validation:Required
	RecommendedWorkflow RecommendedWorkflowSummary `json:"recommendedWorkflow"`

	// Investigation summary from HolmesGPT
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InvestigationSummary string `json:"investigationSummary"`

	// Evidence collected during investigation
	// +optional
	EvidenceCollected []string `json:"evidenceCollected,omitempty"`

	// Recommended actions with rationale
	// +kubebuilder:validation:MinItems=1
	RecommendedActions []ApprovalRecommendedAction `json:"recommendedActions"`

	// Alternative approaches considered
	// +optional
	AlternativesConsidered []ApprovalAlternative `json:"alternativesConsidered,omitempty"`

	// Detailed explanation of why approval is required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	WhyApprovalRequired string `json:"whyApprovalRequired"`

	// Policy evaluation results if Rego policy triggered approval
	// +optional
	PolicyEvaluation *ApprovalPolicyEvaluation `json:"policyEvaluation,omitempty"`

	// ========================================
	// TIMEOUT CONFIGURATION
	// ========================================

	// Deadline for approval decision (approval expires after this time)
	// Calculated by RO using hierarchy: per-request → policy → namespace → default (15m)
	// +kubebuilder:validation:Required
	RequiredBy metav1.Time `json:"requiredBy"`
}

// RecommendedWorkflowSummary contains a summary of the recommended workflow
type RecommendedWorkflowSummary struct {
	// Workflow identifier from catalog
	// +kubebuilder:validation:Required
	WorkflowID string `json:"workflowId"`
	// Workflow version
	// +kubebuilder:validation:Required
	Version string `json:"version"`
	// Execution bundle OCI reference (digest-pinned)
	// +kubebuilder:validation:Required
	ExecutionBundle string `json:"executionBundle"`
	// Rationale for selecting this workflow
	// +kubebuilder:validation:Required
	Rationale string `json:"rationale"`
}

// ApprovalRecommendedAction describes a recommended action with rationale
type ApprovalRecommendedAction struct {
	// Action description
	// +kubebuilder:validation:Required
	Action string `json:"action"`
	// Rationale for this action
	// +kubebuilder:validation:Required
	Rationale string `json:"rationale"`
}

// ApprovalAlternative describes an alternative approach with pros/cons
type ApprovalAlternative struct {
	// Alternative approach description
	// +kubebuilder:validation:Required
	Approach string `json:"approach"`
	// Pros and cons analysis
	// +kubebuilder:validation:Required
	ProsCons string `json:"prosCons"`
}

// ApprovalPolicyEvaluation contains Rego policy evaluation results
type ApprovalPolicyEvaluation struct {
	// Policy name that was evaluated
	// +kubebuilder:validation:Required
	PolicyName string `json:"policyName"`
	// Rules that matched and triggered approval requirement
	// +optional
	MatchedRules []string `json:"matchedRules,omitempty"`
	// Policy decision
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=approved;manual_review_required;denied
	Decision string `json:"decision"`
}

// RemediationApprovalRequestStatus defines the observed state of RemediationApprovalRequest.
type RemediationApprovalRequestStatus struct {
	// ========================================
	// DECISION TRACKING
	// ========================================

	// Decision made by operator or system (timeout)
	// Empty string indicates pending decision
	// +optional
	Decision ApprovalDecision `json:"decision,omitempty"`

	// Who made the decision (username or "system" for timeout)
	// +optional
	DecidedBy string `json:"decidedBy,omitempty"`

	// When the decision was made
	// +optional
	DecidedAt *metav1.Time `json:"decidedAt,omitempty"`

	// Optional message from the decision maker
	// +optional
	DecisionMessage string `json:"decisionMessage,omitempty"`

	// ========================================
	// LIFECYCLE TRACKING
	// ========================================

	// Conditions represent the latest available observations
	// Standard condition types:
	// - "Approved" - Decision is Approved
	// - "Rejected" - Decision is Rejected
	// - "Expired" - Decision timed out
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Time when the approval request was created
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// Time remaining until expiration (human-readable, e.g., "5m30s")
	// Updated by controller periodically
	// +optional
	TimeRemaining string `json:"timeRemaining,omitempty"`

	// True if the approval request has expired
	// +optional
	Expired bool `json:"expired,omitempty"`

	// ========================================
	// AUDIT TRAIL
	// ========================================

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Reason for current state (machine-readable)
	// +optional
	Reason string `json:"reason,omitempty"`

	// Human-readable message about current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:selectablefield:JSONPath=.spec.aiAnalysisRef.name
// +kubebuilder:resource:shortName=rar;rars
// +kubebuilder:printcolumn:name="AIAnalysis",type=string,JSONPath=`.spec.aiAnalysisRef.name`
// +kubebuilder:printcolumn:name="Confidence",type=number,JSONPath=`.spec.confidence`
// +kubebuilder:printcolumn:name="Decision",type=string,JSONPath=`.status.decision`
// +kubebuilder:printcolumn:name="Expired",type=boolean,JSONPath=`.status.expired`
// +kubebuilder:printcolumn:name="RequiredBy",type=string,JSONPath=`.spec.requiredBy`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RemediationApprovalRequest is the Schema for the remediationapprovalrequests API.
//
// ADR-040: RemediationApprovalRequest CRD Architecture
// - Follows Kubernetes CertificateSigningRequest pattern (immutable spec, mutable status)
// - Owned by RemediationRequest (flat hierarchy per ADR-001)
// - AIAnalysis controller uses field index on spec.aiAnalysisRef.name for efficient lookup
// - Timeout expiration handled by dedicated controller
//
// Lifecycle:
// 1. RO creates when AIAnalysis.status.approvalRequired=true
// 2. Operator approves/rejects via status.conditions update
// 3. Dedicated controller detects decision or timeout
// 4. AIAnalysis controller watches and transitions phase accordingly
type RemediationApprovalRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationApprovalRequestSpec   `json:"spec,omitempty"`
	Status RemediationApprovalRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemediationApprovalRequestList contains a list of RemediationApprovalRequest.
type RemediationApprovalRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationApprovalRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemediationApprovalRequest{}, &RemediationApprovalRequestList{})
}
