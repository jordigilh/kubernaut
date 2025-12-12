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

// ========================================
// REMEDIATION PHASE CONSTANTS
// ========================================

// RemediationPhase represents the orchestration phase of a RemediationRequest.
// These constants are exported for external consumers (e.g., Gateway) to enable
// type-safe cross-service integration per the Viceversa Pattern.
//
// 🏛️ BR-COMMON-001: Capitalized phase values per Kubernetes API conventions.
// 🏛️ Viceversa Pattern: Consumers use these constants for compile-time safety.
//
// Reference: docs/requirements/BR-COMMON-001-phase-value-format-standard.md
// Reference: docs/handoff/RO_VICEVERSA_PATTERN_IMPLEMENTATION.md
//
// +kubebuilder:validation:Enum=Pending;Processing;Analyzing;AwaitingApproval;Executing;Blocked;Completed;Failed;TimedOut;Skipped
type RemediationPhase string

const (
	// PhasePending is the initial state when RemediationRequest is created.
	PhasePending RemediationPhase = "Pending"

	// PhaseProcessing indicates SignalProcessing is enriching the signal.
	PhaseProcessing RemediationPhase = "Processing"

	// PhaseAnalyzing indicates AIAnalysis is determining remediation workflow.
	PhaseAnalyzing RemediationPhase = "Analyzing"

	// PhaseAwaitingApproval indicates human approval is required.
	// Reference: BR-ORCH-001 (manual approval workflow)
	PhaseAwaitingApproval RemediationPhase = "AwaitingApproval"

	// PhaseExecuting indicates WorkflowExecution is running remediation.
	PhaseExecuting RemediationPhase = "Executing"

	// PhaseBlocked indicates remediation is in cooldown after consecutive failures.
	// This is a NON-terminal phase - RO will transition to Failed after cooldown.
	// Reference: BR-ORCH-042 (consecutive failure blocking)
	PhaseBlocked RemediationPhase = "Blocked"

	// PhaseCompleted is the terminal success state.
	PhaseCompleted RemediationPhase = "Completed"

	// PhaseFailed is the terminal failure state.
	PhaseFailed RemediationPhase = "Failed"

	// PhaseTimedOut is the terminal timeout state.
	// Reference: BR-ORCH-027 (global timeout), BR-ORCH-028 (per-phase timeout)
	PhaseTimedOut RemediationPhase = "TimedOut"

	// PhaseSkipped is the terminal state when remediation was not needed.
	// Reference: BR-ORCH-032 (resource lock deduplication)
	PhaseSkipped RemediationPhase = "Skipped"

	// PhaseCancelled is the terminal state when remediation was manually cancelled.
	// Gateway treats this as terminal (allows new RR creation for retry)
	// Reference: DD-GATEWAY-009 (state-based deduplication), BR-GATEWAY-183 (cancelled state handling)
	PhaseCancelled RemediationPhase = "Cancelled"
)

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

	// NOTE: Environment and Priority fields REMOVED per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
	// These are now owned by SignalProcessing and available in:
	// - SignalProcessingStatus.EnvironmentClassification.Environment
	// - SignalProcessingStatus.PriorityAssignment.Priority
	// RO reads these from SP status, not from RR spec.

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

	// Deduplication Metadata (DEPRECATED per DD-GATEWAY-011)
	// Tracking information for duplicate signal suppression
	// Uses shared type for API contract alignment with SignalProcessing CRD
	// DD-GATEWAY-011: DEPRECATED - Moved to status.deduplication
	// Gateway Team Fix (2025-12-12): Made optional to unblock Gateway integration tests
	// RO Team: See docs/handoff/NOTICE_GW_CRD_SCHEMA_FIX_SPEC_DEDUPLICATION.md
	Deduplication sharedtypes.DeduplicationInfo `json:"deduplication,omitempty"`

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
	// ╔════════════════════════════════════════════════════════════════╗
	// ║  GATEWAY-OWNED SECTION (DD-GATEWAY-011)                        ║
	// ║  Gateway Service has exclusive write access to these fields    ║
	// ╚════════════════════════════════════════════════════════════════╝

	// Deduplication tracks signal occurrence for this remediation.
	// OWNER: Gateway Service (exclusive write access)
	// Reference: DD-GATEWAY-011, BR-GATEWAY-181
	// +optional
	Deduplication *DeduplicationStatus `json:"deduplication,omitempty"`

	// StormAggregation tracks storm detection for this remediation.
	// OWNER: Gateway Service (exclusive write access)
	// Reference: DD-GATEWAY-011, DD-GATEWAY-008 v2.0
	// +optional
	StormAggregation *StormAggregationStatus `json:"stormAggregation,omitempty"`

	// ╔════════════════════════════════════════════════════════════════╗
	// ║  RO-OWNED SECTION                                              ║
	// ║  Remediation Orchestrator has exclusive write access           ║
	// ╚════════════════════════════════════════════════════════════════╝

	// Phase tracking for orchestration.
	// Uses typed RemediationPhase constants for type safety and cross-service integration.
	//
	// 🏛️ BR-COMMON-001: Capitalized phase values per Kubernetes API conventions.
	// Reference: BR-ORCH-042 (Blocked phase for consecutive failure cooldown)
	OverallPhase RemediationPhase `json:"overallPhase,omitempty"`

	// Human-readable message describing current status
	Message string `json:"message,omitempty"`

	// Timestamps
	StartTime   *metav1.Time `json:"startTime,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// ========================================
	// PHASE START TIME TRACKING (BR-ORCH-028)
	// ========================================

	// ProcessingStartTime is when SignalProcessing phase started.
	// Used for per-phase timeout detection (default: 5 minutes).
	// Reference: BR-ORCH-028
	// +optional
	ProcessingStartTime *metav1.Time `json:"processingStartTime,omitempty"`

	// AnalyzingStartTime is when AIAnalysis phase started.
	// Used for per-phase timeout detection (default: 10 minutes).
	// Reference: BR-ORCH-028
	// +optional
	AnalyzingStartTime *metav1.Time `json:"analyzingStartTime,omitempty"`

	// ExecutingStartTime is when WorkflowExecution phase started.
	// Used for per-phase timeout detection (default: 30 minutes).
	// Reference: BR-ORCH-028
	// +optional
	ExecutingStartTime *metav1.Time `json:"executingStartTime,omitempty"`

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
	// CONSECUTIVE FAILURE BLOCKING (BR-ORCH-042)
	// DD-GATEWAY-011 v1.3: Blocking moved from Gateway to RO
	// ========================================

	// BlockedUntil indicates when the blocked state expires and new attempts are allowed.
	// Set when OverallPhase = "Blocked" due to consecutive failures (≥3).
	// After this time passes, RO will transition the RR to "Failed" and allow
	// new RRs for the same fingerprint.
	// Reference: BR-ORCH-042, DD-GATEWAY-011 v1.3
	// +optional
	BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

	// BlockReason provides context for why this remediation was blocked.
	// Only set when OverallPhase = "Blocked".
	// Example: "3 consecutive failures for fingerprint abc123; cooldown 1h expires at 2025-12-10T15:00:00Z"
	// Reference: BR-ORCH-042
	// +optional
	BlockReason *string `json:"blockReason,omitempty"`

	// ConsecutiveFailureCount tracks how many times this fingerprint has failed consecutively.
	// Updated by RO when RR transitions to Failed phase.
	// Reset to 0 when RR completes successfully.
	// Reference: BR-ORCH-042
	// +optional
	ConsecutiveFailureCount int32 `json:"consecutiveFailureCount,omitempty"`

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

	// ========================================
	// COMPLETION OUTCOME (BR-ORCH-037)
	// ========================================

	// Outcome indicates the remediation result when completed.
	// Values:
	// - "Remediated": Workflow executed successfully
	// - "NoActionRequired": AIAnalysis determined no action needed (problem self-resolved)
	// - "ManualReviewRequired": Requires operator intervention
	// Reference: BR-ORCH-037, BR-HAPI-200
	// +optional
	// +kubebuilder:validation:Enum=Remediated;NoActionRequired;ManualReviewRequired
	Outcome string `json:"outcome,omitempty"`

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

// ========================================
// GATEWAY-OWNED STATUS TYPES (DD-GATEWAY-011)
// These types track Gateway-specific state
// ========================================

// DeduplicationStatus tracks signal occurrence for deduplication.
// OWNER: Gateway Service (exclusive write access)
// Reference: DD-GATEWAY-011, BR-GATEWAY-181
type DeduplicationStatus struct {
	// FirstSeenAt is when this signal fingerprint was first observed
	// +optional
	FirstSeenAt *metav1.Time `json:"firstSeenAt,omitempty"`
	// LastSeenAt is when this signal fingerprint was last observed
	// +optional
	LastSeenAt *metav1.Time `json:"lastSeenAt,omitempty"`
	// OccurrenceCount tracks how many times this signal has been seen
	// +optional
	OccurrenceCount int32 `json:"occurrenceCount,omitempty"`
}

// StormAggregationStatus tracks storm detection for this remediation.
// OWNER: Gateway Service (exclusive write access)
// Reference: DD-GATEWAY-011, DD-GATEWAY-008 v2.0
type StormAggregationStatus struct {
	// IsPartOfStorm indicates if this signal is part of a detected storm
	// +optional
	IsPartOfStorm bool `json:"isPartOfStorm,omitempty"`
	// StormID is the unique identifier for the storm this signal belongs to
	// +optional
	StormID string `json:"stormId,omitempty"`
	// AggregatedCount is the number of signals aggregated in this storm
	// +optional
	AggregatedCount int32 `json:"aggregatedCount,omitempty"`
	// StormDetectedAt is when the storm was first detected
	// +optional
	StormDetectedAt *metav1.Time `json:"stormDetectedAt,omitempty"`
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
