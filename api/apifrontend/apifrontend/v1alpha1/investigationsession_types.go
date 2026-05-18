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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SessionPhase represents the lifecycle state of an InvestigationSession.
type SessionPhase string

// SessionPhase values.
const (
	SessionPhaseActive       SessionPhase = "Active"
	SessionPhaseDisconnected SessionPhase = "Disconnected"
	SessionPhaseCompleted    SessionPhase = "Completed"
	SessionPhaseCancelled    SessionPhase = "Cancelled"
	SessionPhaseFailed       SessionPhase = "Failed"
)

// SessionJoinMode indicates how the session was initiated.
type SessionJoinMode string

// SessionJoinMode values.
const (
	SessionJoinModeStart    SessionJoinMode = "start"
	SessionJoinModeTakeover SessionJoinMode = "takeover"
)

// ConnectionState represents the SSE connection state.
type ConnectionState string

// ConnectionState values.
const (
	ConnectionStateConnected    ConnectionState = "Connected"
	ConnectionStateDisconnected ConnectionState = "Disconnected"
)

// SessionUser captures the authenticated user's identity from JWT.
// Named SessionUser (not UserIdentity) to avoid collision with internal/auth.UserIdentity.
type SessionUser struct {
	// Username from JWT sub claim.
	Username string `json:"username"`
	// Groups from JWT groups claim (RBAC-relevant only).
	Groups []string `json:"groups,omitempty"`
}

// InvestigationSessionSpec defines the desired state (immutable after creation).
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable"
type InvestigationSessionSpec struct {
	// RemediationRequestRef references the RR this session investigates.
	// The InvestigationSession MUST be created in the same namespace as the RR.
	// OwnerReference is set to this RR for cascade deletion.
	RemediationRequestRef ObjectRef `json:"remediationRequestRef"`

	// A2ATaskID is the A2A task identifier for client reconnection.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	A2ATaskID string `json:"a2aTaskID"`

	// UserIdentity captures the authenticated user from the originating JWT.
	UserIdentity SessionUser `json:"userIdentity"`

	// JoinMode indicates whether the user started or joined the investigation.
	// +kubebuilder:validation:Enum=start;takeover
	JoinMode SessionJoinMode `json:"joinMode"`
}

// ObjectRef is a reference to a namespaced Kubernetes object.
type ObjectRef struct {
	// Name of the referenced object.
	Name string `json:"name"`
	// Namespace of the referenced object.
	Namespace string `json:"namespace"`
}

// InvestigationSessionStatus defines the observed state (mutable, AF-only).
type InvestigationSessionStatus struct {
	// Phase is the current lifecycle state.
	// +kubebuilder:validation:Enum=Active;Disconnected;Completed;Cancelled;Failed
	Phase SessionPhase `json:"phase,omitempty"`

	// AIAnalysisRef is the name of the discovered AIAnalysis CRD.
	AIAnalysisRef string `json:"aiAnalysisRef,omitempty"`

	// KACorrelationID is the correlation ID from KA's /analyze response.
	KACorrelationID string `json:"kaCorrelationID,omitempty"`

	// ConnectionState tracks the SSE connection state.
	ConnectionState ConnectionState `json:"connectionState,omitempty"`

	// StartedAt is the session creation timestamp.
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the timestamp when the session reached a terminal state.
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// DisconnectedAt is the last disconnect timestamp.
	DisconnectedAt *metav1.Time `json:"disconnectedAt,omitempty"`

	// ReconnectedAt is the last reconnect timestamp.
	ReconnectedAt *metav1.Time `json:"reconnectedAt,omitempty"`

	// Message is a human-readable description of the current state.
	Message string `json:"message,omitempty"`

	// Conditions provide detailed status information.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Task ID",type=string,JSONPath=`.spec.a2aTaskID`
// +kubebuilder:printcolumn:name="User",type=string,JSONPath=`.spec.userIdentity.username`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=isess

// InvestigationSession links an A2A task to kubernaut pipeline CRDs.
// It enables session persistence across AF restarts and user reconnections.
// OwnerReference to the associated RemediationRequest ensures automatic
// garbage collection when the RR's retention TTL expires.
type InvestigationSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InvestigationSessionSpec   `json:"spec,omitempty"`
	Status InvestigationSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InvestigationSessionList contains a list of InvestigationSession.
type InvestigationSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InvestigationSession `json:"items"`
}
