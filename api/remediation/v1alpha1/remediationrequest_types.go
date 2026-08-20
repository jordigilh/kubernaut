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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ═══════════════════════════════════════════════════════════════════════════
// VERSION: v1alpha1-v1.1
// Last Updated: August 19, 2026
// ═══════════════════════════════════════════════════════════════════════════
//
// CHANGELOG:
//
// ## V1.1 (August 19, 2026) - God-Struct Decomposition (issue #2206)
//
// ### Changed (RemediationRequestStatus):
// - 40 of 50 top-level fields grouped into 5 pointer sub-structs, each with a
//   nil-safe GetX()/EnsureX() accessor pair (mirrors the AIAnalysisStatus
//   precedent from issue #2205):
//   * PhaseProgress: per-phase timestamps + phase-owned child CRD refs
//     (SignalProcessingRef, AIAnalysisRef, ExecutingStartTime, etc.)
//   * RoutingStatus: dedup/blocking/skip decisions
//     (SkipReason, DuplicateOf, ConsecutiveFailureCount, BlockReason, etc.)
//   * CompletionStatus: terminal outcome + failure/timeout attribution
//     (Outcome, FailureReason, FailurePhase, TimeoutPhase, etc.)
//   * OperatorAudit: manual-intervention audit trail
//     (LastModifiedBy, LastModifiedAt, PreRemediationSpecHash, etc.)
//   * WorkflowSelection: AI-selected workflow display metadata
//     (SelectedWorkflowRef, TargetDisplay, Confidence, etc.)
// - +kubebuilder:printcolumn JSONPaths updated to the new nested paths
//   (e.g. .status.completionStatus.outcome).
//
// ### Design Decision: Deployment strategy eliminates upgrade risk
// - Fresh-install-only deployment strategy (no in-place upgrade path for
//   this internal, unexposed v1alpha1 CRD) means there is no read-modify-write
//   window where an old controller binary could round-trip a CR through the
//   new schema and silently drop status data written to the old flat fields.
//
// ### Rationale:
// - 50 top-level fields exceeded the God-struct anti-pattern threshold
//   (AGENTS.md: 15+ fields); grouping by functional concern restores
//   single-responsibility per sub-struct and matches #2205's precedent.
//
// ## V1.0 (December 14, 2025) - Centralized Routing Enhancement
//
// ### Added (RemediationRequestStatus):
// - SkipMessage: Human-readable details about skip reason
// - BlockingWorkflowExecution: Reference to WFE causing skip
//
// ### Enhanced (RemediationRequestStatus):
// - SkipReason: Added values for centralized routing
//   * ExponentialBackoff (pre-execution failures)
//   * ExhaustedRetries (max consecutive failures)
//   * PreviousExecutionFailed (execution-time failure)
//
// ### Design Decision: DD-RO-002 - Centralized Routing Responsibility
// - RemediationOrchestrator now makes ALL routing decisions
// - WorkflowExecution simplified to pure executor (no routing logic)
// - All skip information consolidated in RemediationRequest.Status
//
// ### Rationale:
// - Clean separation: RO routes, WE executes
// - Single source of truth for skip reasons (RR.Status)
// - Improved debuggability (-66% debug time)
// - Consistent skip reason format (100% consistency)
//
// ### Related Changes:
// - WorkflowExecution.Status.SkipDetails removed (V1.0)
// - WorkflowExecution.Status.Phase "Skipped" removed (V1.0)
//
// ### References:
// - Implementation Plan: docs/implementation/V1.0_RO_CENTRALIZED_ROUTING_IMPLEMENTATION_PLAN.md
// - Design Decision: docs/architecture/decisions/DD-RO-002-centralized-routing-responsibility.md
//
// ═══════════════════════════════════════════════════════════════════════════

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
//
// +kubebuilder:validation:Enum=Pending;Processing;Analyzing;AwaitingApproval;Executing;Verifying;Blocked;Completed;Failed;TimedOut;Skipped;Cancelled
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

	// PhaseVerifying indicates remediation succeeded and EffectivenessAssessment is running.
	// Non-terminal: Gateway deduplicates signals while EA assesses remediation effectiveness.
	// RO transitions to Completed when EA reaches a terminal state or VerificationDeadline expires.
	// Reference: #280 (duplicate RR prevention for proactive alerts)
	PhaseVerifying RemediationPhase = "Verifying"

	// PhaseBlocked indicates remediation cannot proceed due to external blocking condition.
	// This is a NON-terminal phase (Gateway deduplicates, prevents RR flood).
	// V1.0: Unified blocking for 6 scenarios (DD-RO-002-ADDENDUM Blocked Phase Semantics):
	// - ConsecutiveFailures: After cooldown → Failed (BR-ORCH-042)
	// - ResourceBusy: When resource available → Proceeds to execute
	// - RecentlyRemediated: After cooldown → Proceeds to execute (DD-WE-001)
	// - ExponentialBackoff: After backoff window → Retries execution (DD-WE-004)
	// - DuplicateInProgress: When original completes → Inherits outcome
	// - UnmanagedResource: Retries until scope label added or RR times out (BR-SCOPE-001)
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
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

// SkipReason represents the reason why a RemediationRequest was skipped.
// Reference: DD-RO-002 (centralized routing responsibility)
// +kubebuilder:validation:Enum=RecentlyRemediated;ResourceBusy;ExhaustedRetries;PreviousExecutionFailed
type SkipReason string

const (
	SkipReasonRecentlyRemediated      SkipReason = "RecentlyRemediated"
	SkipReasonResourceBusy            SkipReason = "ResourceBusy"
	SkipReasonExhaustedRetries        SkipReason = "ExhaustedRetries"
	SkipReasonPreviousExecutionFailed SkipReason = "PreviousExecutionFailed"
)

// FailurePhase represents the orchestration phase where a failure occurred.
// BR-COMMON-001: PascalCase for CRD phase values.
// +kubebuilder:validation:Enum=Configuration;SignalProcessing;AIAnalysis;Approval;WorkflowExecution;Blocked;Deduplicated
type FailurePhase string

const (
	FailurePhaseConfiguration     FailurePhase = "Configuration"
	FailurePhaseSignalProcessing  FailurePhase = "SignalProcessing"
	FailurePhaseAIAnalysis        FailurePhase = "AIAnalysis"
	FailurePhaseApproval          FailurePhase = "Approval"
	FailurePhaseWorkflowExecution FailurePhase = "WorkflowExecution"
	FailurePhaseBlocked           FailurePhase = "Blocked"
	// FailurePhaseDeduplicated indicates an RR that inherited a failure from a
	// deduplicated WorkflowExecution collision (Issue #190). Excluded from
	// consecutive failure counting per BR-ORCH-042.
	FailurePhaseDeduplicated FailurePhase = "Deduplicated"
)

// BlockReason represents the reason why a RemediationRequest is blocked (non-terminal).
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
// +kubebuilder:validation:Enum=ConsecutiveFailures;DuplicateInProgress;ResourceBusy;RecentlyRemediated;ExponentialBackoff;UnmanagedResource;IneffectiveChain
type BlockReason string

const (
	// BlockReasonConsecutiveFailures indicates remediation failed 3+ times consecutively.
	// This is a temporary block with a 1-hour cooldown period.
	// Reference: BR-ORCH-042
	BlockReasonConsecutiveFailures BlockReason = "ConsecutiveFailures"

	// BlockReasonDuplicateInProgress indicates another RR with the same fingerprint is active.
	// This prevents Gateway RR flood by keeping the duplicate in non-terminal Blocked state.
	// Reference: DD-RO-002-ADDENDUM
	BlockReasonDuplicateInProgress BlockReason = "DuplicateInProgress"

	// BlockReasonResourceBusy indicates another WorkflowExecution is running on the same target.
	// This prevents concurrent modifications to the same Kubernetes resource.
	// Reference: DD-RO-002, DD-WE-001
	BlockReasonResourceBusy BlockReason = "ResourceBusy"

	// BlockReasonRecentlyRemediated indicates the same workflow+target was executed recently.
	// This enforces a cooldown period (default 5 minutes) to prevent redundant executions.
	// Reference: DD-WE-001
	BlockReasonRecentlyRemediated BlockReason = "RecentlyRemediated"

	// BlockReasonExponentialBackoff indicates pre-execution failures require a backoff period.
	// This implements graduated retry for transient infrastructure failures.
	// Reference: DD-WE-004
	BlockReasonExponentialBackoff BlockReason = "ExponentialBackoff"

	// BlockReasonUnmanagedResource indicates the target resource is not managed by Kubernaut.
	// The resource or namespace does not have the kubernaut.ai/managed=true label.
	// RO will retry with exponential backoff (5s → 10s → ... → 5min) until RR times out.
	// Reference: BR-SCOPE-001, FR-SCOPE-003
	BlockReasonUnmanagedResource BlockReason = "UnmanagedResource"

	// BlockReasonIneffectiveChain indicates consecutive remediations for the same target
	// have been ineffective (resource keeps reverting or health doesn't improve).
	// Escalates to human review via NotificationRequest.
	// Reference: BR-ORCH-042, Issue #214
	BlockReasonIneffectiveChain BlockReason = "IneffectiveChain"
)

// ========================================
// TIMEOUT CONFIGURATION
// ========================================

// TimeoutConfig provides fine-grained timeout configuration for remediations.
// Supports both global workflow timeout and per-phase timeouts for granular control.
//
// Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
// Design Decision: DD-TIMEOUT-001
type TimeoutConfig struct {
	// Global timeout for entire remediation workflow.
	// Overrides controller-level default (1 hour).
	// Reference: BR-ORCH-027, AC-027-4
	// +optional
	// +kubebuilder:validation:Format=duration
	Global *metav1.Duration `json:"global,omitempty"`

	// Processing phase timeout (SignalProcessing enrichment).
	// Overrides controller-level default (5 minutes).
	// Reference: BR-ORCH-028, AC-028-5
	// +optional
	// +kubebuilder:validation:Format=duration
	Processing *metav1.Duration `json:"processing,omitempty"`

	// Analyzing phase timeout (AIAnalysis investigation).
	// Overrides controller-level default (10 minutes).
	// Reference: BR-ORCH-028, AC-028-5
	// +optional
	// +kubebuilder:validation:Format=duration
	Analyzing *metav1.Duration `json:"analyzing,omitempty"`

	// Executing phase timeout (WorkflowExecution remediation).
	// Overrides controller-level default (30 minutes).
	// Reference: BR-ORCH-028, AC-028-5
	// +optional
	// +kubebuilder:validation:Format=duration
	Executing *metav1.Duration `json:"executing,omitempty"`
}

// RemediationRequestSpec defines the desired state of RemediationRequest.
//
// ADR-001: Spec Immutability
// RemediationRequest represents an immutable event (signal received, remediation required).
// Once created (by Gateway or external source), spec cannot be modified to ensure:
// - Audit trail integrity (remediation matches original signal)
// - No signal metadata tampering during remediation lifecycle
// - Consistent signal data across all child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution)
//
// Cancellation: Delete the RemediationRequest CRD (Kubernetes-native pattern).
// Status updates: Controllers update .status fields (not affected by spec immutability).
//
// Note: Individual field immutability (e.g., signalFingerprint) is redundant with full spec immutability,
// but retained for explicit documentation of critical fields.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-001)"
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
	// Severity level (external value from signal provider)
	// Examples: "Sev1", "P0", "critical", "HIGH", "warning"
	// SignalProcessing will normalize via Rego policy (DD-SEVERITY-001)
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	Severity string `json:"severity"`

	// NOTE: Environment and Priority fields REMOVED per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
	// These are now owned by SignalProcessing and available in:
	// - SignalProcessingStatus.EnvironmentClassification.Environment
	// - SignalProcessingStatus.PriorityAssignment.Priority
	// RO reads these from SP status, not from RR spec.

	// Signal type: "alert" (generic signal type; adapter-specific values are deprecated)
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

	// ========================================
	// MULTI-CLUSTER IDENTIFICATION (ADR-065)
	// ========================================

	// ClusterID is the unique identifier of the cluster where the signal originated.
	// Corresponds to the Backend CR name in the Envoy AI Gateway.
	// Used by RO and WE for multi-cluster routing of remediation workflows.
	// Empty string indicates the local (hub) cluster.
	// Reference: ADR-065 (Multi-Cluster Federation)
	// +optional
	// +kubebuilder:validation:MaxLength=253
	ClusterID string `json:"clusterID,omitempty"`

	// Temporal Data
	// When the signal first started firing (from upstream source)
	FiringTime metav1.Time `json:"firingTime"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

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
	ProviderData string `json:"providerData,omitempty"`

	// ========================================
	// AUDIT/DEBUG
	// ========================================

	// Complete original webhook payload for debugging and audit
	// Issue #96: stored as string to avoid base64 encoding in CEL validation
	OriginalPayload string `json:"originalPayload,omitempty"`

	// ========================================
	// WORKFLOW CONFIGURATION
	// ========================================

	// NOTE: TimeoutConfig moved to RemediationRequestStatus per Gap #8
	// Rationale: Operators need to adjust timeouts mid-remediation (mutable)
	// Reference: BR-AUDIT-005 Gap #8, BR-AUTH-001 (SOC2 CC8.1)
	// See: RemediationRequestStatus.TimeoutConfig
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

	// APIVersion disambiguates the resource's API group when the Kind exists in
	// multiple groups (e.g. Route in route.openshift.io vs serving.knative.dev).
	// Format: "group/version" (e.g. "route.openshift.io/v1"). Issue #1040.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
}

// String returns the resource identifier in the format used by WorkflowExecution.
// Format: "namespace/kind/name" for namespaced resources, "kind/name" for cluster-scoped.
// Kind is lowercase to match WorkflowExecution format.
func (r ResourceIdentifier) String() string {
	kind := r.Kind
	if kind != "" {
		// Convert first character to lowercase for consistent format
		kind = string(kind[0]|0x20) + kind[1:]
	}
	if r.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", r.Namespace, kind, r.Name)
	}
	return fmt.Sprintf("%s/%s", kind, r.Name)
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

	// ╔════════════════════════════════════════════════════════════════╗
	// ║  RO-OWNED SECTION                                              ║
	// ║  Remediation Orchestrator has exclusive write access           ║
	// ╚════════════════════════════════════════════════════════════════╝

	// ObservedGeneration is the most recent generation observed by the controller.
	// Used to prevent duplicate reconciliations and ensure idempotency.
	// Per DD-CONTROLLER-001: Standard pattern for all Kubernetes controllers.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

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

	// PhaseProgress tracks per-phase start times and downstream CRD references
	// accumulated as the remediation progresses through SignalProcessing,
	// AIAnalysis, WorkflowExecution, and EffectivenessAssessment.
	// God-struct decomposition (issue #2206, following #2205's AIAnalysisStatus
	// precedent). Use GetPhaseProgress()/EnsurePhaseProgress() for nil-safe access.
	// +optional
	PhaseProgress *PhaseProgress `json:"phaseProgress,omitempty"`

	// RoutingStatus tracks centralized routing decisions: skip/block reasons,
	// deduplication tracking, and consecutive-failure cooldown state.
	// Reference: DD-RO-001, DD-RO-002, DD-RO-002-ADDENDUM, BR-ORCH-032/033/034/042.
	// God-struct decomposition (issue #2206). Use GetRoutingStatus()/
	// EnsureRoutingStatus() for nil-safe access.
	// +optional
	RoutingStatus *RoutingStatus `json:"routingStatus,omitempty"`

	// CompletionStatus tracks terminal-phase outcome details: failure/timeout
	// reason, manual-review flag, and notification tracking.
	// Reference: BR-ORCH-029/030/031/037.
	// God-struct decomposition (issue #2206). Use GetCompletionStatus()/
	// EnsureCompletionStatus() for nil-safe access.
	// +optional
	CompletionStatus *CompletionStatus `json:"completionStatus,omitempty"`

	// OperatorAudit tracks SOC2 CC8.1 operator-mutation attribution and the
	// pre-remediation spec hash captured before workflow execution.
	// Reference: BR-AUTH-001, ADR-EM-001, DD-EM-002, Gap #8 Extension.
	// God-struct decomposition (issue #2206). Use GetOperatorAudit()/
	// EnsureOperatorAudit() for nil-safe access.
	// +optional
	OperatorAudit *OperatorAudit `json:"operatorAudit,omitempty"`

	// WorkflowSelection tracks the AI-selected workflow, its execution reference,
	// the resolved remediation target, and kubectl display strings.
	// Reference: BR-AUDIT-005 Gap #5-6, BR-KA-212, Issue #387, Issue #635.
	// God-struct decomposition (issue #2206). Use GetWorkflowSelection()/
	// EnsureWorkflowSelection() for nil-safe access.
	// +optional
	WorkflowSelection *WorkflowSelection `json:"workflowSelection,omitempty"`

	// RetentionExpiryTime indicates when this CRD should be cleaned up (24 hours after completion)
	RetentionExpiryTime *metav1.Time `json:"retentionExpiryTime,omitempty"`

	// Conditions represent observations of RemediationRequest state.
	// Standard condition types:
	// - "NotificationDelivered": True if notification sent successfully, False if cancelled/failed
	//   - Reason "DeliverySucceeded": Notification sent
	//   - Reason "UserCancelled": User deleted NotificationRequest before delivery
	//   - Reason "DeliveryFailed": NotificationRequest failed to deliver
	//
	// Conditions follow Kubernetes API conventions (KEP-1623).
	// Reference: BR-ORCH-029 (user cancellation), BR-ORCH-030 (status tracking)
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ========================================
	// TIMEOUT CONFIGURATION (BR-ORCH-027/028, Gap #8)
	// ========================================

	// TimeoutConfig provides operational timeout overrides for this remediation.
	// OWNER: Remediation Orchestrator (sets defaults on first reconcile)
	// MUTABLE BY: Operators (can adjust mid-remediation via kubectl edit)
	// Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
	// Gap #8: Moved from spec to status to enable operator mutability + audit trail
	// +optional
	TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}

// OutcomeRemediated is the Status.Outcome value for a successfully remediated
// and EA-verified RemediationRequest (BR-ORCH-037). See the Outcome field enum
// above for the full set of valid values.
const OutcomeRemediated = "Remediated"

// ========================================
// GOD-STRUCT DECOMPOSITION SUB-STRUCTS (Issue #2206)
// Following #2205's AIAnalysisStatus precedent: pointer bundles, nil until
// their owning phase populates them, each with a GetX()/EnsureX() nil-safe
// accessor pair so read call sites never repeat a nil-guard.
// ========================================

// PhaseProgress tracks per-phase start times (BR-ORCH-028 per-phase timeout
// detection) and the downstream CRD references accumulated as a remediation
// progresses through SignalProcessing, AIAnalysis, WorkflowExecution, and
// EffectivenessAssessment.
type PhaseProgress struct {
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

	// VerificationDeadline is the deadline for the Verifying phase.
	// Computed by RO as EA.Status.ValidityDeadline + 30s buffer.
	// If exceeded, RR transitions to Completed with Outcome "VerificationTimedOut".
	// Reference: #280 (Verifying phase timeout)
	// +optional
	VerificationDeadline *metav1.Time `json:"verificationDeadline,omitempty"`

	// SignalProcessingRef references the SignalProcessing CRD.
	// +optional
	SignalProcessingRef *corev1.ObjectReference `json:"signalProcessingRef,omitempty"`

	// RemediationProcessingRef references the RemediationProcessing CRD.
	// +optional
	RemediationProcessingRef *corev1.ObjectReference `json:"remediationProcessingRef,omitempty"`

	// AIAnalysisRef references the AIAnalysis CRD.
	// +optional
	AIAnalysisRef *corev1.ObjectReference `json:"aiAnalysisRef,omitempty"`

	// WorkflowExecutionRef references the WorkflowExecution CRD.
	// +optional
	WorkflowExecutionRef *corev1.ObjectReference `json:"workflowExecutionRef,omitempty"`

	// EffectivenessAssessmentRef tracks the EffectivenessAssessment CRD created for this remediation.
	// Set by the RO after creating the EA CRD on terminal phase transitions.
	// Reference: ADR-EM-001
	// +optional
	EffectivenessAssessmentRef *corev1.ObjectReference `json:"effectivenessAssessmentRef,omitempty"`

	// CurrentProcessingRef references the current SignalProcessing CRD
	// +optional
	CurrentProcessingRef *corev1.ObjectReference `json:"currentProcessingRef,omitempty"`
}

// GetPhaseProgress returns Status.PhaseProgress, or a zero-value *PhaseProgress
// if nil, so read call sites can chain field access without repeating a nil-guard.
func (s *RemediationRequestStatus) GetPhaseProgress() *PhaseProgress {
	if s.PhaseProgress == nil {
		return &PhaseProgress{}
	}
	return s.PhaseProgress
}

// EnsurePhaseProgress lazily initializes Status.PhaseProgress and returns it,
// for write call sites that need a non-nil target to mutate.
func (s *RemediationRequestStatus) EnsurePhaseProgress() *PhaseProgress {
	if s.PhaseProgress == nil {
		s.PhaseProgress = &PhaseProgress{}
	}
	return s.PhaseProgress
}

// RoutingStatus tracks centralized routing decisions made by the Remediation
// Orchestrator: skip/block reasons, deduplication tracking, and
// consecutive-failure backoff cooldown state.
// Reference: DD-RO-001, DD-RO-002, DD-RO-002-ADDENDUM (Blocked Phase Semantics),
// BR-ORCH-032/033/034/042, DD-WE-004 (Exponential Backoff).
type RoutingStatus struct {
	// SkipReason indicates why this remediation was skipped.
	// Only set when OverallPhase = Skipped or Failed.
	// Reference: DD-RO-002 (centralized routing responsibility)
	// +optional
	// +kubebuilder:validation:Enum=RecentlyRemediated;ResourceBusy;ExhaustedRetries;PreviousExecutionFailed
	SkipReason SkipReason `json:"skipReason,omitempty"`

	// SkipMessage provides human-readable details about why remediation was skipped
	// Only set when OverallPhase = "Skipped" or "Failed"
	// Reference: DD-RO-002 (centralized routing responsibility)
	// +optional
	SkipMessage string `json:"skipMessage,omitempty"`

	// BlockingWorkflowExecution references the WorkflowExecution causing the block
	// Set for block reasons: ResourceBusy, RecentlyRemediated, ExponentialBackoff
	// Nil for: ConsecutiveFailures, DuplicateInProgress
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockingWorkflowExecution string `json:"blockingWorkflowExecution,omitempty"`

	// DuplicateOf references the parent RemediationRequest that this is a duplicate of
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	DuplicateOf string `json:"duplicateOf,omitempty"`

	// DuplicateCount tracks the number of duplicate remediations that were skipped
	// because this RR's workflow was already executing (resource lock)
	// +optional
	DuplicateCount int `json:"duplicateCount,omitempty"`

	// DuplicateRefs lists the names of RemediationRequests that were skipped
	// because they targeted the same resource as this RR
	// +optional
	DuplicateRefs []string `json:"duplicateRefs,omitempty"`

	// DeduplicatedByWE stores the name of the original WorkflowExecution whose
	// outcome this RR is waiting to inherit (Issue #190).
	// +optional
	DeduplicatedByWE string `json:"deduplicatedByWE,omitempty"`

	// BlockReason indicates why this remediation is blocked (non-terminal)
	// Only set when OverallPhase = "Blocked"
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	// +kubebuilder:validation:Enum=ConsecutiveFailures;DuplicateInProgress;ResourceBusy;RecentlyRemediated;ExponentialBackoff;UnmanagedResource;IneffectiveChain
	BlockReason BlockReason `json:"blockReason,omitempty"`

	// BlockMessage provides human-readable details about why remediation is blocked
	// Only set when OverallPhase = "Blocked"
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockMessage string `json:"blockMessage,omitempty"`

	// BlockedUntil indicates when blocking expires (time-based blocks)
	// Reference: BR-ORCH-042, DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

	// NextAllowedExecution indicates when this RR can be retried after exponential backoff.
	// Reference: DD-WE-004 (Exponential Backoff Cooldown), BR-ORCH-042.6 (#1091)
	// +optional
	NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`

	// ConsecutiveFailureCount tracks how many times this fingerprint has failed consecutively.
	// Reference: BR-ORCH-042
	// +optional
	ConsecutiveFailureCount int32 `json:"consecutiveFailureCount,omitempty"`
}

// GetRoutingStatus returns Status.RoutingStatus, or a zero-value *RoutingStatus
// if nil, so read call sites can chain field access without repeating a nil-guard.
func (s *RemediationRequestStatus) GetRoutingStatus() *RoutingStatus {
	if s.RoutingStatus == nil {
		return &RoutingStatus{}
	}
	return s.RoutingStatus
}

// EnsureRoutingStatus lazily initializes Status.RoutingStatus and returns it,
// for write call sites that need a non-nil target to mutate.
func (s *RemediationRequestStatus) EnsureRoutingStatus() *RoutingStatus {
	if s.RoutingStatus == nil {
		s.RoutingStatus = &RoutingStatus{}
	}
	return s.RoutingStatus
}

// CompletionStatus tracks terminal-phase outcome details: failure/timeout
// reason, manual-review escalation flag, and notification delivery tracking.
// Reference: BR-ORCH-029/030/031 (notification lifecycle), BR-ORCH-037 (outcome).
type CompletionStatus struct {
	// FailurePhase indicates which orchestration phase failed.
	// Only set when OverallPhase = Failed.
	// +optional
	// +kubebuilder:validation:Enum=Configuration;SignalProcessing;AIAnalysis;Approval;WorkflowExecution;Blocked;Deduplicated
	FailurePhase *FailurePhase `json:"failurePhase,omitempty"`

	// FailureReason provides a human-readable reason for the failure
	// Only set when OverallPhase = "failed"
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// RequiresManualReview indicates that this remediation cannot proceed automatically
	// and requires operator intervention.
	// Reference: BR-ORCH-032, BR-ORCH-036, DD-WE-004
	// +optional
	RequiresManualReview bool `json:"requiresManualReview,omitempty"`

	// Outcome indicates the remediation result when completed.
	// Reference: BR-ORCH-037, BR-KA-200, BR-EM-012
	// +optional
	// +kubebuilder:validation:Enum=Remediated;NoActionRequired;ManualReviewRequired;VerificationTimedOut;Inconclusive;DryRun
	Outcome string `json:"outcome,omitempty"`

	// TimeoutPhase indicates which orchestration phase timed out.
	// Only set when OverallPhase = TimedOut.
	// +optional
	TimeoutPhase *RemediationPhase `json:"timeoutPhase,omitempty"`

	// TimeoutTime records when the timeout occurred
	// Only set when OverallPhase = "timeout"
	// +optional
	TimeoutTime *metav1.Time `json:"timeoutTime,omitempty"`

	// NotificationRequestRefs tracks all notification CRDs created for this remediation.
	// Reference: BR-ORCH-035
	// +optional
	NotificationRequestRefs []corev1.ObjectReference `json:"notificationRequestRefs,omitempty"`

	// NotificationStatus tracks the delivery status of notification(s) for this remediation.
	// Values: "Pending", "InProgress", "Sent", "Failed", "Cancelled"
	// Reference: BR-ORCH-030 (notification status tracking)
	// +optional
	// +kubebuilder:validation:Enum=Pending;InProgress;Sent;Failed;Cancelled
	NotificationStatus string `json:"notificationStatus,omitempty"`

	// ApprovalNotificationSent prevents duplicate notifications when AIAnalysis requires approval
	// Reference: BR-ORCH-001
	// +optional
	ApprovalNotificationSent bool `json:"approvalNotificationSent,omitempty"`
}

// GetCompletionStatus returns Status.CompletionStatus, or a zero-value
// *CompletionStatus if nil, so read call sites can chain field access without
// repeating a nil-guard.
func (s *RemediationRequestStatus) GetCompletionStatus() *CompletionStatus {
	if s.CompletionStatus == nil {
		return &CompletionStatus{}
	}
	return s.CompletionStatus
}

// EnsureCompletionStatus lazily initializes Status.CompletionStatus and
// returns it, for write call sites that need a non-nil target to mutate.
func (s *RemediationRequestStatus) EnsureCompletionStatus() *CompletionStatus {
	if s.CompletionStatus == nil {
		s.CompletionStatus = &CompletionStatus{}
	}
	return s.CompletionStatus
}

// OperatorAudit tracks SOC2 CC8.1 operator-mutation attribution and the
// pre-remediation spec hash captured before workflow execution.
// Reference: BR-AUTH-001 (SOC2 CC8.1 Operator Attribution), ADR-EM-001, DD-EM-002.
type OperatorAudit struct {
	// LastModifiedBy tracks the last operator who modified this RR's status.
	// Populated by RemediationRequest mutating webhook.
	// Reference: BR-AUTH-001 (SOC2 CC8.1 Operator Attribution), Gap #8 Extension
	// +optional
	LastModifiedBy string `json:"lastModifiedBy,omitempty"`

	// LastModifiedAt tracks when the last status modification occurred.
	// Populated by RemediationRequest mutating webhook.
	// +optional
	LastModifiedAt *metav1.Time `json:"lastModifiedAt,omitempty"`

	// PreRemediationSpecHash is the canonical spec hash of the target resource captured
	// by the RO BEFORE launching the remediation workflow. This enables the EM to compare
	// pre vs post-remediation state without querying DataStorage audit events.
	// Set once by the RO during the transition to WorkflowExecution phase; immutable after.
	// Reference: ADR-EM-001, DD-EM-002
	// +optional
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="preRemediationSpecHash is immutable once set"
	PreRemediationSpecHash string `json:"preRemediationSpecHash,omitempty"`
}

// GetOperatorAudit returns Status.OperatorAudit, or a zero-value *OperatorAudit
// if nil, so read call sites can chain field access without repeating a nil-guard.
func (s *RemediationRequestStatus) GetOperatorAudit() *OperatorAudit {
	if s.OperatorAudit == nil {
		return &OperatorAudit{}
	}
	return s.OperatorAudit
}

// EnsureOperatorAudit lazily initializes Status.OperatorAudit and returns it,
// for write call sites that need a non-nil target to mutate.
func (s *RemediationRequestStatus) EnsureOperatorAudit() *OperatorAudit {
	if s.OperatorAudit == nil {
		s.OperatorAudit = &OperatorAudit{}
	}
	return s.OperatorAudit
}

// WorkflowSelection tracks the AI-selected workflow, its execution reference,
// the resolved remediation target, and the kubectl-printer display strings
// derived from them.
// Reference: BR-AUDIT-005 Gap #5-6 (Workflow Selection/Execution Reference),
// BR-KA-212, Issue #387 (RemediationTarget), Issue #635 (display columns).
type WorkflowSelection struct {
	// SelectedWorkflowRef captures the workflow selected by AI for this remediation.
	// Populated from workflowexecution.selection.completed audit event.
	// Reference: BR-AUDIT-005 Gap #5 (Workflow Selection)
	// +optional
	SelectedWorkflowRef *WorkflowReference `json:"selectedWorkflowRef,omitempty"`

	// ExecutionRef references the WorkflowExecution CRD for this remediation.
	// Populated from workflowexecution.execution.started audit event.
	// Reference: BR-AUDIT-005 Gap #6 (Execution Reference)
	// +optional
	ExecutionRef *corev1.ObjectReference `json:"executionRef,omitempty"`

	// RemediationTarget identifies the Kubernetes resource the LLM determined should be
	// remediated. Populated from AIAnalysis.Status.RootCauseAnalysis.RemediationTarget.
	// May differ from Spec.TargetResource (e.g., Deployment vs Pod).
	// Reference: BR-KA-212, Issue #387
	// +optional
	RemediationTarget *ResourceIdentifier `json:"remediationTarget,omitempty"`

	// TargetDisplay is the Kubernetes-idiomatic Kind/Name of the RCA target
	// (e.g., "Deployment/web-frontend"). Populated when RemediationTarget is set.
	// +optional
	TargetDisplay string `json:"targetDisplay,omitempty"`

	// Confidence is the AI analysis confidence score as a display string
	// (e.g., "0.97"). Populated from AIAnalysis.SelectedWorkflow.Confidence.
	// +optional
	Confidence string `json:"confidence,omitempty"`

	// WorkflowDisplayName is the human-readable workflow identifier
	// (e.g., "GitRevertCommit:git-revert-v2"). Populated from AIAnalysis.SelectedWorkflow.
	// +optional
	WorkflowDisplayName string `json:"workflowDisplayName,omitempty"`

	// SignalTargetDisplay is the Kubernetes-idiomatic Kind/Name of the signal target
	// (e.g., "Pod/web-frontend-cdbdbc4f8-6kn6j"). Populated from Spec.TargetResource.
	// +optional
	SignalTargetDisplay string `json:"signalTargetDisplay,omitempty"`
}

// GetWorkflowSelection returns Status.WorkflowSelection, or a zero-value
// *WorkflowSelection if nil, so read call sites can chain field access
// without repeating a nil-guard.
func (s *RemediationRequestStatus) GetWorkflowSelection() *WorkflowSelection {
	if s.WorkflowSelection == nil {
		return &WorkflowSelection{}
	}
	return s.WorkflowSelection
}

// EnsureWorkflowSelection lazily initializes Status.WorkflowSelection and
// returns it, for write call sites that need a non-nil target to mutate.
func (s *RemediationRequestStatus) EnsureWorkflowSelection() *WorkflowSelection {
	if s.WorkflowSelection == nil {
		s.WorkflowSelection = &WorkflowSelection{}
	}
	return s.WorkflowSelection
}

// WorkflowReference captures workflow catalog information for audit trail.
// Used in RemediationRequestStatus.SelectedWorkflowRef (Gap #5).
type WorkflowReference struct {
	// WorkflowID is the catalog lookup key
	WorkflowID string `json:"workflowId"`

	// Version of the workflow
	Version string `json:"version"`

	// ExecutionBundle resolved from workflow catalog
	// OCI bundle reference for Tekton PipelineRun
	ExecutionBundle string `json:"executionBundle"`

	// ExecutionBundleDigest for audit trail and reproducibility
	// +optional
	ExecutionBundleDigest string `json:"executionBundleDigest,omitempty"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rr
// +kubebuilder:selectablefield:JSONPath=.spec.signalFingerprint
// +kubebuilder:selectablefield:JSONPath=.spec.signalType
// +kubebuilder:selectablefield:JSONPath=.spec.severity
// +kubebuilder:selectablefield:JSONPath=.spec.clusterID
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.overallPhase`
// +kubebuilder:printcolumn:name="Outcome",type=string,JSONPath=`.status.completionStatus.outcome`
// +kubebuilder:printcolumn:name="Alert",type=string,JSONPath=`.spec.signalName`
// +kubebuilder:printcolumn:name="RCA Target",type=string,JSONPath=`.status.workflowSelection.targetDisplay`
// +kubebuilder:printcolumn:name="Workflow",type=string,JSONPath=`.status.workflowSelection.workflowDisplayName`
// +kubebuilder:printcolumn:name="Confidence",type=string,JSONPath=`.status.workflowSelection.confidence`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterID`,priority=1
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.signalSource`,priority=1
// +kubebuilder:printcolumn:name="Signal NS",type=string,JSONPath=`.spec.targetResource.namespace`,priority=1
// +kubebuilder:printcolumn:name="Signal Target",type=string,JSONPath=`.status.workflowSelection.signalTargetDisplay`,priority=1
// +kubebuilder:printcolumn:name="RCA NS",type=string,JSONPath=`.status.workflowSelection.remediationTarget.namespace`,priority=1

// RemediationRequest is the Schema for the remediationrequests API.
// DD-CRD-003: Printer columns for operational triage
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
