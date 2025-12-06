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

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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
	// This field is immutable and used for querying all occurrences of the same problem
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="signalFingerprint is immutable"
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

	// Priority value provided by Rego policies - no enum enforcement
	// Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)
	// Operators can define custom priority schemes via Rego policies
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
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

	// ========================================
	// TARGET RESOURCE IDENTIFICATION
	// ========================================

	// TargetResource identifies the Kubernetes resource that triggered this signal.
	// Populated by Gateway from NormalizedSignal.Resource - REQUIRED.
	// Used by SignalProcessing for context enrichment and RO for workflow routing.
	// For Kubernetes signals, this contains Kind, Name, Namespace of the affected resource.
	// +kubebuilder:validation:Required
	TargetResource ResourceIdentifier `json:"targetResource"`

	// Temporal Data
	// When the signal first started firing (from upstream source)
	FiringTime metav1.Time `json:"firingTime"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

	// Deduplication Metadata
	// Tracking information for duplicate signal suppression
	// Uses shared type for API contract alignment with SignalProcessing CRD
	Deduplication sharedtypes.DeduplicationInfo `json:"deduplication"`

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

// ResourceIdentifier uniquely identifies a Kubernetes resource.
// Used for target resource identification across CRDs.
// Per Gateway Team response (RESPONSE_TARGET_RESOURCE_SCHEMA.md), this is populated
// by Gateway from NormalizedSignal.Resource and passed through to SignalProcessing.
type ResourceIdentifier struct {
	// Kind of the Kubernetes resource (e.g., "Pod", "Deployment", "Node", "StatefulSet")
	Kind string `json:"kind"`

	// Name of the Kubernetes resource instance
	Name string `json:"name"`

	// Namespace of the Kubernetes resource (empty for cluster-scoped resources like Node)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RemediationRequestStatus defines the observed state of RemediationRequest.
type RemediationRequestStatus struct {
	// Phase tracking for orchestration
	// Valid values: "pending", "processing", "analyzing", "executing", "recovering",
	//               "completed", "failed", "timeout", "Skipped"
	OverallPhase string `json:"overallPhase,omitempty"`

	// Human-readable message describing current status
	Message string `json:"message,omitempty"`

	// Timestamps
	StartTime   *metav1.Time `json:"startTime,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// References to downstream CRDs
	SignalProcessingRef      *corev1.ObjectReference `json:"signalProcessingRef,omitempty"`
	RemediationProcessingRef *corev1.ObjectReference `json:"remediationProcessingRef,omitempty"`
	AIAnalysisRef            *corev1.ObjectReference `json:"aiAnalysisRef,omitempty"`
	WorkflowExecutionRef     *corev1.ObjectReference `json:"workflowExecutionRef,omitempty"`

	// NotificationRequestRefs tracks all notification CRDs created for this remediation.
	// Provides audit trail for compliance and instant visibility for debugging.
	// Reference: BR-ORCH-035
	// +optional
	NotificationRequestRefs []corev1.ObjectReference `json:"notificationRequestRefs,omitempty"`

	// Approval notification tracking (BR-ORCH-001)
	// Prevents duplicate notifications when AIAnalysis requires approval
	ApprovalNotificationSent bool `json:"approvalNotificationSent,omitempty"`

	// ========================================
	// SKIPPED PHASE TRACKING (DD-RO-001)
	// BR-ORCH-032, BR-ORCH-033, BR-ORCH-034
	// ========================================

	// SkipReason indicates why this remediation was skipped
	// Valid values: "ResourceBusy" (another workflow executing on same target),
	//               "RecentlyRemediated" (target recently remediated, cooldown period)
	// Only set when OverallPhase = "Skipped"
	SkipReason string `json:"skipReason,omitempty"`

	// DuplicateOf references the parent RemediationRequest that this is a duplicate of
	// Only set when OverallPhase = "Skipped" due to resource lock deduplication
	DuplicateOf string `json:"duplicateOf,omitempty"`

	// DuplicateCount tracks the number of duplicate remediations that were skipped
	// because this RR's workflow was already executing (resource lock)
	// Only populated on parent RRs that have duplicates
	DuplicateCount int `json:"duplicateCount,omitempty"`

	// DuplicateRefs lists the names of RemediationRequests that were skipped
	// because they targeted the same resource as this RR
	// Only populated on parent RRs that have duplicates
	DuplicateRefs []string `json:"duplicateRefs,omitempty"`

	// ========================================
	// FAILURE/TIMEOUT TRACKING
	// ========================================

	// FailurePhase indicates which phase failed (e.g., "ai_analysis", "workflow_execution")
	// Only set when OverallPhase = "failed"
	FailurePhase *string `json:"failurePhase,omitempty"`

	// FailureReason provides a human-readable reason for the failure
	// Only set when OverallPhase = "failed"
	FailureReason *string `json:"failureReason,omitempty"`

	// RequiresManualReview indicates that this remediation cannot proceed automatically
	// and requires operator intervention. Set when:
	// - WE skip reason is "ExhaustedRetries" (5+ consecutive pre-execution failures)
	// - WE skip reason is "PreviousExecutionFailed" (execution failure, cluster state unknown)
	// - AIAnalysis WorkflowResolutionFailed with LowConfidence or WorkflowNotFound
	// Reference: BR-ORCH-032, BR-ORCH-036, DD-WE-004
	// +optional
	RequiresManualReview bool `json:"requiresManualReview,omitempty"`

	// TimeoutPhase indicates which phase timed out
	// Only set when OverallPhase = "timeout"
	TimeoutPhase *string `json:"timeoutPhase,omitempty"`

	// TimeoutTime records when the timeout occurred
	// Only set when OverallPhase = "timeout"
	TimeoutTime *metav1.Time `json:"timeoutTime,omitempty"`

	// RetentionExpiryTime indicates when this CRD should be cleaned up (24 hours after completion)
	RetentionExpiryTime *metav1.Time `json:"retentionExpiryTime,omitempty"`

	// ========================================
	// RECOVERY TRACKING
	// ========================================

	// RecoveryAttempts tracks the number of recovery attempts for this remediation
	RecoveryAttempts int `json:"recoveryAttempts,omitempty"`

	// CurrentProcessingRef references the current SignalProcessing CRD (may differ during recovery)
	CurrentProcessingRef *corev1.ObjectReference `json:"currentProcessingRef,omitempty"`
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
