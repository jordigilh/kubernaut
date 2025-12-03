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
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// SignalProcessingSpec defines the desired state of SignalProcessing.
// Phase 1 Enhancement: Self-contained CRD pattern - contains all data from RemediationRequest
// No external CRD reads required during reconciliation (performance, reliability, isolation)
type SignalProcessingSpec struct {
	// ========================================
	// PARENT REFERENCE (Audit/Lineage Only)
	// ========================================
	// Reference to parent RemediationRequest CRD for audit trail and lineage
	// SignalProcessor does NOT read RemediationRequest - all data is self-contained
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
	// Environment value provided by Rego policies - no enum enforcement
	// Examples: "production", "staging", "development", "qa-eu", "canary"
	// GAP-C1-01 FIX: Changed from Enum=prod;staging;dev to free-text
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Environment string `json:"environment"`

	// Priority value provided by Rego policies - no enum enforcement
	// Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)
	// GAP-C1-02 FIX: Changed from Enum=P0;P1;P2 + Pattern to free-text
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
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
	// Uses shared type for API contract alignment (RO Team decision)
	Deduplication sharedtypes.DeduplicationInfo `json:"deduplication"`

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

	// Storm type classification
	// GAP-C1-05 FIX: Added field for contract alignment with RemediationRequest
	// Values: "rate" (frequency-based storm) or "pattern" (similar alerts storm)
	// +kubebuilder:validation:MaxLength=63
	StormType string `json:"stormType,omitempty"`

	// Time window used for storm detection
	// GAP-C1-06 FIX: Added field for contract alignment with RemediationRequest
	// Format: duration string (e.g., "5m", "1h")
	// +kubebuilder:validation:MaxLength=63
	StormWindow string `json:"stormWindow,omitempty"`

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

// EnrichmentConfiguration specifies how to enrich signal context
type EnrichmentConfiguration struct {
	// Enable cluster state enrichment (Kubernetes API queries)
	EnableClusterState bool `json:"enableClusterState,omitempty"`

	// Enable metrics enrichment (Prometheus/monitoring queries)
	EnableMetrics bool `json:"enableMetrics,omitempty"`

	// Enable historical enrichment (vector DB/time-series queries)
	EnableHistorical bool `json:"enableHistorical,omitempty"`
}

// SignalProcessingStatus defines the observed state of SignalProcessing.
// DD-CONTRACT-002: Structured types replace unstructured ContextData map[string]string
type SignalProcessingStatus struct {
	// Phase tracking: "pending", "enriching", "completed", "failed"
	Phase string `json:"phase,omitempty"`

	// Structured enrichment results (DD-CONTRACT-002)
	// Replaces ContextData map[string]string anti-pattern
	// GAP-C3-04 FIX: Uses shared types from pkg/shared/types/enrichment.go
	EnrichmentResults sharedtypes.EnrichmentResults `json:"enrichmentResults,omitempty"`

	// Timestamps
	StartTime   *metav1.Time `json:"startTime,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// ========================================
// TYPE ALIASES FOR CONVENIENCE
// ========================================
// These aliases allow SignalProcessing code to reference types without
// the sharedtypes prefix while maintaining single source of truth.
// GAP-C3-04: Shared types in pkg/shared/types/enrichment.go

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

// ContainerStatus alias - use sharedtypes.ContainerStatus in new code
type ContainerStatus = sharedtypes.ContainerStatus

// DeploymentDetails alias - use sharedtypes.DeploymentDetails in new code
type DeploymentDetails = sharedtypes.DeploymentDetails

// NodeDetails alias - use sharedtypes.NodeDetails in new code
type NodeDetails = sharedtypes.NodeDetails

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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SignalProcessing is the Schema for the signalprocessings API.
// DD-SIGNAL-PROCESSING-001: Renamed from RemediationProcessing per ADR-015
type SignalProcessing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalProcessingSpec   `json:"spec,omitempty"`
	Status SignalProcessingStatus `json:"status,omitempty"`
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
