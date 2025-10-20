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

// RemediationRequestSpec defines the desired state of RemediationRequest.
type RemediationRequestSpec struct {
	// ========================================
	// UNIVERSAL FIELDS (ALL SIGNALS)
	// These fields are populated for EVERY signal regardless of provider
	// ========================================

	// Core Signal Identification
	// Unique fingerprint for deduplication (SHA256 of alert/event key fields)
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
	SignalFingerprint string `json:"signalFingerprint"`

	// Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
	// +kubebuilder:validation:MaxLength=253
	SignalName string `json:"signalName"`

	// Signal Classification
	// Severity level: "critical", "warning", "info"
	// +kubebuilder:validation:Enum=critical;warning;info
	Severity string `json:"severity"`

	// Environment: dynamically configured via namespace labels
	// Accepts any non-empty string to support custom environment taxonomies
	// (e.g., "production", "staging", "development", "canary", "qa-eu", "blue", "green", etc.)
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Environment string `json:"environment"`

	// Priority assigned by Gateway (P0=critical, P1=high, P2=normal, P3=low)
	// Used by downstream Rego policies for remediation decisions
	// +kubebuilder:validation:Enum=P0;P1;P2;P3
	// +kubebuilder:validation:Pattern="^P[0-3]$"
	Priority string `json:"priority"`

	// Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", "datadog-monitor", etc.
	// Used for signal-aware remediation strategies
	SignalType string `json:"signalType"`

	// Adapter that ingested the signal (e.g., "prometheus-adapter", "k8s-event-adapter")
	// +kubebuilder:validation:MaxLength=63
	SignalSource string `json:"signalSource,omitempty"`

	// Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"
	// Indicates which infrastructure system the signal targets
	// +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
	TargetType string `json:"targetType"`

	// Temporal Data
	// When the signal first started firing (from upstream source)
	FiringTime metav1.Time `json:"firingTime"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

	// Deduplication Metadata
	// Tracking information for duplicate signal suppression
	Deduplication DeduplicationInfo `json:"deduplication"`

	// Storm Detection
	// True if this signal is part of a detected alert storm
	IsStorm bool `json:"isStorm,omitempty"`

	// Storm type: "rate" (frequency-based) or "pattern" (similar alerts)
	StormType string `json:"stormType,omitempty"`

	// Time window for storm detection (e.g., "5m")
	StormWindow string `json:"stormWindow,omitempty"`

	// Number of alerts in the storm
	StormAlertCount int `json:"stormAlertCount,omitempty"`

	// List of affected resources in an aggregated storm (e.g., "namespace:Pod:name")
	// Only populated for aggregated storm CRDs
	AffectedResources []string `json:"affectedResources,omitempty"`

	// ========================================
	// SIGNAL METADATA (PHASE 1 ADDITION)
	// ========================================
	// Signal labels and annotations extracted from provider-specific data
	// These are populated by Gateway Service after parsing providerData
	SignalLabels      map[string]string `json:"signalLabels,omitempty"`
	SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

	// ========================================
	// PROVIDER-SPECIFIC DATA
	// All provider-specific fields go here (INCLUDING Kubernetes)
	// ========================================

	// Provider-specific fields in raw JSON format
	// Gateway adapter populates this based on signal source
	// Controllers parse this based on targetType/signalType
	//
	// For Kubernetes (targetType="kubernetes"):
	//   {"namespace": "...", "resource": {"kind": "...", "name": "..."}, "alertmanagerURL": "...", ...}
	//
	// For AWS (targetType="aws"):
	//   {"region": "...", "accountId": "...", "instanceId": "...", "resourceType": "...", ...}
	//
	// For Datadog (targetType="datadog"):
	//   {"monitorId": 123, "host": "...", "tags": [...], "metricQuery": "...", ...}
	ProviderData []byte `json:"providerData,omitempty"`

	// ========================================
	// AUDIT/DEBUG
	// ========================================

	// Complete original webhook payload for debugging and audit
	// Stored as []byte to preserve exact format
	OriginalPayload []byte `json:"originalPayload,omitempty"`

	// ========================================
	// WORKFLOW CONFIGURATION
	// ========================================

	// Optional timeout overrides for this specific remediation
	TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}

// DeduplicationInfo tracks duplicate signal suppression
type DeduplicationInfo struct {
	// True if this signal is a duplicate of an active remediation
	IsDuplicate bool `json:"isDuplicate"`

	// Timestamp when this signal fingerprint was first seen
	FirstSeen metav1.Time `json:"firstSeen"`

	// Timestamp when this signal fingerprint was last seen
	LastSeen metav1.Time `json:"lastSeen"`

	// Total count of occurrences of this signal
	OccurrenceCount int `json:"occurrenceCount"`

	// Reference to previous RemediationRequest CRD (if duplicate)
	PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}

// TimeoutConfig allows per-remediation timeout customization
type TimeoutConfig struct {
	// Timeout for RemediationProcessing phase (default: 5m)
	RemediationProcessingTimeout metav1.Duration `json:"remediationProcessingTimeout,omitempty"`

	// Timeout for AIAnalysis phase (default: 10m)
	AIAnalysisTimeout metav1.Duration `json:"aiAnalysisTimeout,omitempty"`

	// Timeout for WorkflowExecution phase (default: 20m)
	WorkflowExecutionTimeout metav1.Duration `json:"workflowExecutionTimeout,omitempty"`

	// Overall workflow timeout (default: 1h)
	OverallWorkflowTimeout metav1.Duration `json:"overallWorkflowTimeout,omitempty"`
}

// RemediationRequestStatus defines the observed state of RemediationRequest.
type RemediationRequestStatus struct {
	// Phase tracking for orchestration
	OverallPhase string `json:"overallPhase,omitempty"` // "pending", "processing", "analyzing", "executing", "completed", "failed"

	// Timestamps
	StartTime   *metav1.Time `json:"startTime,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// References to downstream CRDs
	RemediationProcessingRef *corev1.ObjectReference `json:"remediationProcessingRef,omitempty"`
	AIAnalysisRef            *corev1.ObjectReference `json:"aiAnalysisRef,omitempty"`
	WorkflowExecutionRef     *corev1.ObjectReference `json:"workflowExecutionRef,omitempty"`

	// Approval notification tracking (BR-ORCH-001)
	// Prevents duplicate notifications when AIAnalysis requires approval
	ApprovalNotificationSent bool `json:"approvalNotificationSent,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemediationRequest is the Schema for the remediationrequests API.
type RemediationRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationRequestSpec   `json:"spec,omitempty"`
	Status RemediationRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemediationRequestList contains a list of RemediationRequest.
type RemediationRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemediationRequest{}, &RemediationRequestList{})
}
