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

// RemediationProcessingSpec defines the desired state of RemediationProcessing.
// Phase 1 Enhancement: Self-contained CRD pattern - contains all data from RemediationRequest
// No external CRD reads required during reconciliation (performance, reliability, isolation)
type RemediationProcessingSpec struct {
	// ========================================
	// PARENT REFERENCE (Audit/Lineage Only)
	// ========================================
	// Reference to parent RemediationRequest CRD for audit trail and lineage
	// RemediationProcessor does NOT read RemediationRequest - all data is self-contained
	RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

	// ========================================
	// SIGNAL IDENTIFICATION (From RemediationRequest)
	// ========================================
	// Core signal identity copied from RemediationRequest
	// Unique fingerprint for deduplication (SHA256 of signal key fields)
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
	SignalFingerprint string `json:"signalFingerprint"`

	// Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
	// +kubebuilder:validation:MaxLength=253
	SignalName string `json:"signalName"`

	// Severity level: "critical", "warning", "info"
	// +kubebuilder:validation:Enum=critical;warning;info
	Severity string `json:"severity"`

	// ========================================
	// SIGNAL CLASSIFICATION (From RemediationRequest)
	// ========================================
	// Environment: "prod", "staging", "dev"
	// +kubebuilder:validation:Enum=prod;staging;dev
	Environment string `json:"environment"`

	// Priority assigned by Gateway (P0=critical, P1=high, P2=normal)
	// +kubebuilder:validation:Enum=P0;P1;P2
	// +kubebuilder:validation:Pattern="^P[0-2]$"
	Priority string `json:"priority"`

	// Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", etc.
	SignalType string `json:"signalType"`

	// Adapter that ingested the signal (e.g., "prometheus-adapter", "k8s-event-adapter")
	// +kubebuilder:validation:MaxLength=63
	SignalSource string `json:"signalSource,omitempty"`

	// Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"
	// +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
	TargetType string `json:"targetType"`

	// ========================================
	// SIGNAL METADATA (From RemediationRequest)
	// ========================================
	// Signal labels extracted from provider-specific data
	// For Prometheus: alert.Labels (e.g., {"alertname": "HighMemory", "namespace": "prod"})
	SignalLabels map[string]string `json:"signalLabels,omitempty"`

	// Signal annotations extracted from provider-specific data
	// For Prometheus: alert.Annotations (e.g., {"summary": "Memory usage above 90%"})
	SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

	// ========================================
	// TARGET RESOURCE (From RemediationRequest)
	// ========================================
	// Target resource identification (extracted from providerData by RemediationOrchestrator)
	TargetResource ResourceIdentifier `json:"targetResource"`

	// ========================================
	// TIMESTAMPS (From RemediationRequest)
	// ========================================
	// When the signal first started firing (from upstream source)
	FiringTime metav1.Time `json:"firingTime,omitempty"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

	// ========================================
	// DEDUPLICATION (From RemediationRequest)
	// ========================================
	// Deduplication and correlation context
	Deduplication DeduplicationContext `json:"deduplication"`

	// ========================================
	// PROVIDER DATA (From RemediationRequest)
	// ========================================
	// Provider-specific fields in raw JSON format
	// Controllers parse this based on targetType/signalType if needed
	ProviderData []byte `json:"providerData,omitempty"`

	// Complete original webhook payload for debugging and audit
	OriginalPayload []byte `json:"originalPayload,omitempty"`

	// ========================================
	// STORM DETECTION (From RemediationRequest)
	// ========================================
	// True if this signal is part of a detected alert storm
	IsStorm bool `json:"isStorm,omitempty"`

	// Number of alerts in the storm
	StormAlertCount int `json:"stormAlertCount,omitempty"`

	// ========================================
	// CONFIGURATION (Processor-Specific)
	// ========================================
	// Optional enrichment configuration specific to this processing
	EnrichmentConfig *EnrichmentConfiguration `json:"enrichmentConfig,omitempty"`
}

// ResourceIdentifier identifies the target resource for remediation
type ResourceIdentifier struct {
	// Resource kind (e.g., "Pod", "Deployment", "StatefulSet")
	Kind string `json:"kind"`

	// Resource name
	Name string `json:"name"`

	// Resource namespace
	Namespace string `json:"namespace"`
}

// DeduplicationContext provides correlation and deduplication information
type DeduplicationContext struct {
	// Timestamp when this signal fingerprint was first seen
	FirstOccurrence metav1.Time `json:"firstOccurrence"`

	// Timestamp when this signal fingerprint was last seen
	LastOccurrence metav1.Time `json:"lastOccurrence"`

	// Total count of occurrences of this signal
	OccurrenceCount int `json:"occurrenceCount"`

	// Optional correlation ID for grouping related signals
	CorrelationID string `json:"correlationID,omitempty"`
}

// EnrichmentConfiguration specifies how to enrich signal context
type EnrichmentConfiguration struct {
	// Enable cluster state enrichment (Kubernetes API queries)
	EnableClusterState bool `json:"enableClusterState,omitempty"`

	// Enable metrics enrichment (Prometheus/monitoring queries)
	EnableMetrics bool `json:"enableMetrics,omitempty"`

	// Enable historical enrichment (vector DB/time-series queries)
	EnableHistorical bool `json:"enableHistorical,omitempty"`
}

// RemediationProcessingStatus defines the observed state of RemediationProcessing.
type RemediationProcessingStatus struct {
	// Phase tracking
	Phase string `json:"phase,omitempty"` // "pending", "enriching", "completed", "failed"

	// Enriched context data from processing
	ContextData map[string]string `json:"contextData,omitempty"`

	// Timestamps
	StartTime   *metav1.Time `json:"startTime,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemediationProcessing is the Schema for the remediationprocessings API.
type RemediationProcessing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationProcessingSpec   `json:"spec,omitempty"`
	Status RemediationProcessingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemediationProcessingList contains a list of RemediationProcessing.
type RemediationProcessingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationProcessing `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemediationProcessing{}, &RemediationProcessingList{})
}
