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

// Package v1alpha1 contains API Schema definitions for the signalprocessing v1alpha1 API group
// Design Decision: DD-SIGNAL-PROCESSING-001 - CRD Naming per ADR-015
// Design Decision: DD-CONTRACT-002 - Structured types for AIAnalysis integration
// Implementation Plan: Day 2 - CRD Types aligned with IMPLEMENTATION_PLAN.md
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=`.status.severity`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environmentClassification.environment`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.status.priorityAssignment.priority`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalProcessing is the Schema for the signalprocessings API.
// DD-SIGNAL-PROCESSING-001: Renamed from RemediationProcessing per ADR-015
type SignalProcessing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalProcessingSpec   `json:"spec,omitempty"`
	Status SignalProcessingStatus `json:"status,omitempty"`
}

// SignalProcessingSpec defines the desired state of SignalProcessing.
// Implementation Plan Day 2: Aligned with IMPLEMENTATION_PLAN.md structure
//
// ADR-001: Spec Immutability
// SignalProcessing represents an immutable event (signal enrichment).
// Once created by RemediationOrchestrator, spec cannot be modified to ensure:
// - Audit trail integrity (processed signal matches original signal)
// - No signal data tampering during enrichment
// - Consistent context passed to AIAnalysis
//
// To reprocess a signal, delete and recreate the SignalProcessing CRD.
//
// +kubebuilder:validation:XValidation:rule="self.remediationRequestRef.name != ''",message="remediationRequestRef.name is required for audit trail correlation"
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-001)"
type SignalProcessingSpec struct{
	// Reference to parent RemediationRequest
	RemediationRequestRef ObjectReference `json:"remediationRequestRef"`

	// Signal data (copied from RemediationRequest for processing)
	Signal SignalData `json:"signal"`

	// Configuration for processing
	EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`
}

// ObjectReference contains enough information to let you locate the referenced object.
type ObjectReference struct {
	// API version of the referent
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind of the referent
	Kind string `json:"kind,omitempty"`
	// Name of the referent
	Name string `json:"name"`
	// Namespace of the referent
	Namespace string `json:"namespace,omitempty"`
	// UID of the referent
	UID string `json:"uid,omitempty"`
}

// SignalData contains all signal information copied from RemediationRequest.
// This makes SignalProcessing self-contained for processing.
type SignalData struct {
	// Unique fingerprint for deduplication (SHA256 of signal key fields)
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
	Fingerprint string `json:"fingerprint"`

	// Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Severity level (external/raw value from monitoring system)
	// DD-SEVERITY-001: No enum restriction - allows external severity schemes (Sev1-4, P0-P4, etc.)
	// Normalized severity is stored in Status.Severity
	Severity string `json:"severity"`

	// Signal type: "alert" (generic signal type; adapter-specific values like "prometheus-alert" or "kubernetes-event" are deprecated)
	Type string `json:"type"`

	// Adapter that ingested the signal
	// +kubebuilder:validation:MaxLength=63
	Source string `json:"source,omitempty"`

	// Target system type
	// +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
	TargetType string `json:"targetType"`

	// Target resource identification
	TargetResource ResourceIdentifier `json:"targetResource"`

	// Signal labels extracted from provider-specific data
	Labels map[string]string `json:"labels,omitempty"`

	// Signal annotations extracted from provider-specific data
	Annotations map[string]string `json:"annotations,omitempty"`

	// When the signal first started firing
	FiringTime *metav1.Time `json:"firingTime,omitempty"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

	// Provider-specific fields in raw JSON format (issue #96: string to avoid base64)
	ProviderData string `json:"providerData,omitempty"`
}

// ResourceIdentifier identifies the target resource for remediation.
type ResourceIdentifier struct {
	// Resource kind (e.g., "Pod", "Deployment", "StatefulSet")
	Kind string `json:"kind"`
	// Resource name
	Name string `json:"name"`
	// Resource namespace
	Namespace string `json:"namespace"`
}

// EnrichmentConfig specifies enrichment settings.
type EnrichmentConfig struct {
	// Enable cluster state enrichment
	EnableClusterState bool `json:"enableClusterState,omitempty"`
	// Enable metrics enrichment
	EnableMetrics bool `json:"enableMetrics,omitempty"`
	// Enable historical enrichment
	EnableHistorical bool `json:"enableHistorical,omitempty"`
	// Timeout for enrichment operations
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// SignalProcessingPhase represents the current phase of SignalProcessing reconciliation.
// BR-SP-051: Phase State Machine
// BR-COMMON-001: Capitalized phase values per Kubernetes API conventions
// +kubebuilder:validation:Enum=Pending;Enriching;Classifying;Categorizing;Completed;Failed
type SignalProcessingPhase string

const (
	// PhasePending is the initial state when SignalProcessing is created.
	PhasePending SignalProcessingPhase = "Pending"
	// PhaseEnriching is when K8s context enrichment is in progress.
	PhaseEnriching SignalProcessingPhase = "Enriching"
	// PhaseClassifying is when environment/priority classification is in progress.
	PhaseClassifying SignalProcessingPhase = "Classifying"
	// PhaseCategorizing is when business categorization is in progress.
	PhaseCategorizing SignalProcessingPhase = "Categorizing"
	// PhaseCompleted is the terminal success state.
	PhaseCompleted SignalProcessingPhase = "Completed"
	// PhaseFailed is the terminal error state.
	PhaseFailed SignalProcessingPhase = "Failed"
)

// SignalProcessingStatus defines the observed state of SignalProcessing.
// Implementation Plan Day 2: Aligned with IMPLEMENTATION_PLAN.md structure
type SignalProcessingStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// Used to prevent duplicate reconciliations and ensure idempotency.
	// Per DD-CONTROLLER-001: Standard pattern for all Kubernetes controllers.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase: Pending, Enriching, Classifying, Categorizing, Completed, Failed
	Phase SignalProcessingPhase `json:"phase,omitempty"`

	// Processing timestamps
	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Enrichment results
	KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
	RecoveryContext   *RecoveryContext   `json:"recoveryContext,omitempty"`

	// Categorization results (DD-CATEGORIZATION-001)
	EnvironmentClassification *EnvironmentClassification `json:"environmentClassification,omitempty"`
	PriorityAssignment        *PriorityAssignment        `json:"priorityAssignment,omitempty"`
	BusinessClassification    *BusinessClassification    `json:"businessClassification,omitempty"`

	// Severity determination (DD-SEVERITY-001 v1.1)
	// Normalized severity determined by Rego policy: "critical", "high", "medium", "low", or "unknown"
	// Aligned with HAPI/workflow catalog severity levels for consistency across platform
	// Enables downstream services (AIAnalysis, RemediationOrchestrator, Notification)
	// to interpret alert urgency without understanding external severity schemes.
	// +kubebuilder:validation:Enum=critical;high;medium;low;unknown
	// +optional
	Severity string `json:"severity,omitempty"`

	// PolicyHash is the SHA256 hash of the Rego policy used for severity determination
	// Provides audit trail and policy version tracking for compliance requirements
	// Expected format: 64-character hexadecimal string (SHA256 hash)
	// +optional
	PolicyHash string `json:"policyHash,omitempty"`

	// SignalMode indicates whether this is a reactive or predictive signal.
	// BR-SP-106: Predictive Signal Mode Classification
	// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
	// Set during the Classifying phase alongside severity, environment, and priority.
	// All signals MUST be classified — "reactive" is the default for unmapped types.
	// +kubebuilder:validation:Enum=reactive;predictive
	// +optional
	SignalMode string `json:"signalMode,omitempty"`

	// SignalName is the normalized signal name after predictive-to-base mapping.
	// BR-SP-106: Signal Name Normalization
	// For predictive signals (e.g., "PredictedOOMKill"), this is the base name (e.g., "OOMKilled").
	// For reactive signals, this matches Spec.Signal.Name unchanged.
	// This is the AUTHORITATIVE signal name for all downstream consumers (RO, AA, HAPI).
	// +optional
	SignalName string `json:"signalName,omitempty"`

	// SourceSignalName preserves the pre-normalization signal name for audit trail.
	// BR-SP-106: Audit trail preservation (SOC2 CC7.4)
	// Only populated for predictive signals (e.g., "PredictedOOMKill").
	// Empty for reactive signals.
	// +optional
	SourceSignalName string `json:"sourceSignalName,omitempty"`

	// Conditions for detailed status
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Error information
	Error string `json:"error,omitempty"`

	// ConsecutiveFailures tracks the number of consecutive transient failures.
	// Used with shared backoff for exponential retry delays (DD-SHARED-001).
	// Reset to 0 on successful phase transition.
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

	// LastFailureTime records when the last failure occurred.
	// Used to determine if enough time has passed for retry.
	// +optional
	LastFailureTime *metav1.Time `json:"lastFailureTime,omitempty"`
}

// ========================================
// SHARED TYPE ALIASES (Issue #113)
// ========================================
// These types alias the authoritative definitions in pkg/shared/types/enrichment.go.
// This eliminates CRD-to-shared-type duplication and conversion layers.
// Pattern matches AIAnalysis CRD (api/aianalysis/v1alpha1/aianalysis_types.go).

// KubernetesContext aliases the lean, classification-focused shared type.
// Contains: Namespace, Workload (generic), OwnerChain, CustomLabels, DegradedMode.
type KubernetesContext = sharedtypes.KubernetesContext

// NamespaceContext aliases the shared namespace context type.
type NamespaceContext = sharedtypes.NamespaceContext

// WorkloadDetails aliases the generic workload type (kind, name, labels, annotations).
// Replaces per-type fields: PodDetails, DeploymentDetails, StatefulSetDetails, etc.
type WorkloadDetails = sharedtypes.WorkloadDetails

// OwnerChainEntry aliases the shared owner chain entry type.
type OwnerChainEntry = sharedtypes.OwnerChainEntry

// BusinessClassification aliases the shared business classification type.
type BusinessClassification = sharedtypes.BusinessClassification

// RecoveryContext holds context for recovery attempts.
// DD-001: Recovery Context Enrichment
type RecoveryContext struct {
	// Previous remediation attempt ID
	PreviousRemediationID string `json:"previousRemediationId,omitempty"`
	// Number of previous attempts
	AttemptCount int32 `json:"attemptCount,omitempty"`
	// Last failure reason
	LastFailureReason string `json:"lastFailureReason,omitempty"`
	// Time since first failure
	TimeSinceFirstFailure *metav1.Duration `json:"timeSinceFirstFailure,omitempty"`
}

// EnvironmentClassification from DD-CATEGORIZATION-001.
// BR-SP-051-053: Environment Classification (Updated per BR-SP-080 V2.0)
// DD-WORKFLOW-001 v2.2: 4 canonical environments (production, staging, development, test)
// DD-SP-001 V1.1: Removed Confidence field (redundant with source)
// BR-SP-080 V2.0: Removed signal-labels source (security vulnerability)
type EnvironmentClassification struct {
	// Environment: production, staging, development, test
	Environment string `json:"environment"`
	// Source of classification: namespace-labels, rego-inference, default
	// Valid sources per BR-SP-080 V2.0 (signal-labels removed for security)
	Source string `json:"source"`
	// When classification was performed
	ClassifiedAt metav1.Time `json:"classifiedAt"`
}

// PriorityAssignment from DD-CATEGORIZATION-001.
// BR-SP-070-072: Priority Assignment (Updated per BR-SP-080 V2.0)
// DD-SP-001 V1.1: Removed Confidence field (redundant with source)
type PriorityAssignment struct {
	// Priority level: P0, P1, P2, P3
	Priority string `json:"priority"`
	// Source of assignment: rego-policy, severity-fallback, default
	// Per BR-SP-071: severity-fallback used when Rego fails (severity-only fallback)
	Source string `json:"source"`
	// Which Rego rule matched (if applicable)
	PolicyName string `json:"policyName,omitempty"`
	// When assignment was performed
	AssignedAt metav1.Time `json:"assignedAt"`
}

// +kubebuilder:object:root=true

// SignalProcessingList contains a list of SignalProcessing.
type SignalProcessingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SignalProcessing `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SignalProcessing{}, &SignalProcessingList{})
}
