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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AIAnalysisSpec defines the desired state of AIAnalysis.
// +kubebuilder:validation:XValidation:rule="self.temperature >= 0.0 && self.temperature <= 1.0",message="Temperature must be between 0.0 and 1.0"
type AIAnalysisSpec struct {
	// Parent reference to RemediationRequest
	RemediationRequestRef string `json:"remediationRequestRef"`

	// Analysis input
	SignalType    string            `json:"signalType"`
	SignalContext map[string]string `json:"signalContext"` // Enriched context from SignalProcessing (legacy format)
	// TODO: Migrate to EnrichmentResults per DD-CONTRACT-002 when SignalProcessing controller is updated

	// Analysis configuration
	// +kubebuilder:validation:Enum=openai;anthropic;local;holmesgpt
	LLMProvider string `json:"llmProvider"` // "openai", "anthropic", "local"
	// +kubebuilder:validation:MaxLength=253
	LLMModel string `json:"llmModel"` // "gpt-4", "claude-3", etc.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100000
	MaxTokens int `json:"maxTokens"` // Token limit
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Temperature    float64 `json:"temperature"`    // 0.0-1.0
	IncludeHistory bool    `json:"includeHistory"` // Include historical patterns
}

// ApprovalContext contains rich context for approval notifications (BR-AI-059)
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
}

// RecommendedAction describes a remediation action with rationale
type RecommendedAction struct {
	// Action type (from 29 canonical actions)
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

// AIAnalysisStatus defines the observed state of AIAnalysis.
type AIAnalysisStatus struct {
	// Phase tracking
	// +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Recommending;Approving;Completed;Failed;Rejected
	Phase   string `json:"phase"` // "Pending", "Investigating", "Approving", "Completed", "Failed", "Rejected"
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`

	// Timestamps
	StartedAt   *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Analysis results
	RootCause string `json:"rootCause,omitempty"` // Identified root cause
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Confidence        float64 `json:"confidence,omitempty"`        // 0.0-1.0
	RecommendedAction string  `json:"recommendedAction,omitempty"` // Suggested remediation
	RequiresApproval  bool    `json:"requiresApproval"`            // Manual approval needed

	// Investigation details
	// +kubebuilder:validation:MaxLength=253
	InvestigationID string `json:"investigationId,omitempty"` // HolmesGPT investigation ID
	// +kubebuilder:validation:Minimum=0
	TokensUsed int `json:"tokensUsed,omitempty"` // LLM tokens consumed
	// +kubebuilder:validation:Minimum=0
	InvestigationTime int64 `json:"investigationTime,omitempty"` // Duration in seconds

	// Approval fields (BR-AI-059, BR-AI-060)
	// ApprovalRequestName links to AIApprovalRequest CRD
	ApprovalRequestName string `json:"approvalRequestName,omitempty"`
	// ApprovalRequestedAt timestamp when approval was requested
	ApprovalRequestedAt *metav1.Time `json:"approvalRequestedAt,omitempty"`
	// ApprovalContext contains rich context for notifications
	ApprovalContext *ApprovalContext `json:"approvalContext,omitempty"`

	// Approval decision tracking (BR-AI-060)
	// ApprovalStatus: "Approved" | "Rejected" | "Pending"
	// +kubebuilder:validation:Enum=Approved;Rejected;Pending
	ApprovalStatus string `json:"approvalStatus,omitempty"`
	// ApprovedBy: email/username of approver
	ApprovedBy string `json:"approvedBy,omitempty"`
	// RejectedBy: email/username of rejecter
	RejectedBy string `json:"rejectedBy,omitempty"`
	// ApprovalTime: timestamp of approval decision
	ApprovalTime *metav1.Time `json:"approvalTime,omitempty"`
	// RejectionReason: why approval was rejected
	RejectionReason string `json:"rejectionReason,omitempty"`
	// ApprovalMethod: "kubectl" | "dashboard" | "slack-button" | "email-link"
	ApprovalMethod string `json:"approvalMethod,omitempty"`
	// ApprovalJustification: optional operator comment
	ApprovalJustification string `json:"approvalJustification,omitempty"`
	// ApprovalDuration: time from request to decision (e.g., "2m15s")
	ApprovalDuration string `json:"approvalDuration,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
