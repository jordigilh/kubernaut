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

package v1alpha1

// EvidenceFailureClass is a closed, server-owned terminal evidence class.
type EvidenceFailureClass string

const (
	EvidenceFailureNone                       EvidenceFailureClass = "none"
	EvidenceFailureHistoryBasedIneffective    EvidenceFailureClass = "history_based_ineffective_remediation"
	EvidenceFailureRCAIncomplete              EvidenceFailureClass = "rca_incomplete"
	EvidenceFailureTransportUnavailable       EvidenceFailureClass = "evidence_transport_unavailable"
	EvidenceFailureSchemaClientRequestFailure EvidenceFailureClass = "schema_client_request_failure"
)

// EvidenceSource identifies a bounded evidence producer.
type EvidenceSource string

const (
	EvidenceSourceRemediationHistory EvidenceSource = "remediation_history"
	EvidenceSourceOwnerChain         EvidenceSource = "owner_chain"
	EvidenceSourceDetectedLabels     EvidenceSource = "detected_labels"
	EvidenceSourceMCP                EvidenceSource = "mcp"
	EvidenceSourceContextTool        EvidenceSource = "context_tool"
)

// EvidenceOutcome is a closed source/terminal outcome.
type EvidenceOutcome string

const (
	EvidenceOutcomeSuccess              EvidenceOutcome = "success"
	EvidenceOutcomePartial              EvidenceOutcome = "partial"
	EvidenceOutcomeSkipped              EvidenceOutcome = "skipped"
	EvidenceOutcomeValidEmpty           EvidenceOutcome = "valid_empty"
	EvidenceOutcomeTransportUnavailable EvidenceOutcome = "transport_unavailable"
	EvidenceOutcomeSchemaClientFailure  EvidenceOutcome = "schema_client_failure"
	EvidenceOutcomeCanceled             EvidenceOutcome = "canceled"
	EvidenceOutcomeSourcesTruncated     EvidenceOutcome = "sources_truncated"
)

// EvidenceSourceStatus preserves one bounded status per target/source pair.
type EvidenceSourceStatus struct {
	// +kubebuilder:validation:MaxLength=8
	Version string `json:"version"`
	// +kubebuilder:validation:Enum=none;history_based_ineffective_remediation;rca_incomplete;evidence_transport_unavailable;schema_client_request_failure
	Class EvidenceFailureClass `json:"class"`
	// +kubebuilder:validation:Enum=remediation_history;owner_chain;detected_labels;mcp;context_tool
	Source EvidenceSource `json:"source"`
	// +kubebuilder:validation:Enum=success;partial;skipped;valid_empty;transport_unavailable;schema_client_failure;canceled;sources_truncated
	Outcome EvidenceOutcome `json:"outcome"`
	// +kubebuilder:validation:MaxLength=253
	TargetRef string `json:"targetRef,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	ClusterID string `json:"clusterID,omitempty"`
	// +kubebuilder:validation:MaxLength=128
	LogicalTool string `json:"logicalTool,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Attempts int `json:"attempts,omitempty"`
	// +kubebuilder:validation:MaxLength=128
	LastFailureCode string `json:"lastFailureCode,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	CorrelationID string `json:"correlationID,omitempty"`
}

// EvidenceStatus is the bounded optional evidence projection on AgentSession.
type EvidenceStatus struct {
	// +kubebuilder:validation:MaxLength=8
	Version string `json:"version"`
	// +kubebuilder:validation:Enum=none;history_based_ineffective_remediation;rca_incomplete;evidence_transport_unavailable;schema_client_request_failure
	Class EvidenceFailureClass `json:"class"`
	// +kubebuilder:validation:Enum=remediation_history;owner_chain;detected_labels;mcp;context_tool
	Source EvidenceSource `json:"source"`
	// +kubebuilder:validation:Enum=success;partial;skipped;valid_empty;transport_unavailable;schema_client_failure;canceled;sources_truncated
	Outcome EvidenceOutcome `json:"outcome"`
	// +kubebuilder:validation:MaxLength=253
	TargetRef string `json:"targetRef,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	ClusterID string `json:"clusterID,omitempty"`
	// +kubebuilder:validation:MaxLength=128
	LogicalTool string `json:"logicalTool,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Attempts int `json:"attempts,omitempty"`
	// +kubebuilder:validation:MaxLength=128
	LastFailureCode string `json:"lastFailureCode,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	CorrelationID string `json:"correlationID,omitempty"`
	// +kubebuilder:validation:MaxItems=8
	Sources          []EvidenceSourceStatus `json:"sources,omitempty"`
	SourcesTruncated bool                   `json:"sourcesTruncated,omitempty"`
}
