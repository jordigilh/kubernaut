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
	SignalContext map[string]string `json:"signalContext"` // Enriched context from RemediationProcessing

	// Analysis configuration
	LLMProvider    string  `json:"llmProvider"`    // "openai", "anthropic", "local"
	LLMModel       string  `json:"llmModel"`       // "gpt-4", "claude-3", etc.
	MaxTokens      int     `json:"maxTokens"`      // Token limit
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Temperature    float64 `json:"temperature"`    // 0.0-1.0
	IncludeHistory bool    `json:"includeHistory"` // Include historical patterns
}

// AIAnalysisStatus defines the observed state of AIAnalysis.
type AIAnalysisStatus struct {
	// Phase tracking
	Phase   string `json:"phase"`   // "Pending", "Investigating", "Completed", "Failed"
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`

	// Timestamps
	StartedAt   *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Analysis results
	RootCause         string  `json:"rootCause,omitempty"`         // Identified root cause
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Confidence        float64 `json:"confidence,omitempty"`        // 0.0-1.0
	RecommendedAction string  `json:"recommendedAction,omitempty"` // Suggested remediation
	RequiresApproval  bool    `json:"requiresApproval"`            // Manual approval needed

	// Investigation details
	InvestigationID   string `json:"investigationId,omitempty"`   // HolmesGPT investigation ID
	TokensUsed        int    `json:"tokensUsed,omitempty"`        // LLM tokens consumed
	InvestigationTime int64  `json:"investigationTime,omitempty"` // Duration in seconds

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
