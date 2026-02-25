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

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ═══════════════════════════════════════════════════════════════════════════
// VERSION: v1alpha1-v1.0
// Last Updated: December 14, 2025
// ═══════════════════════════════════════════════════════════════════════════
//
// CHANGELOG:
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
// - Proposal: docs/handoff/TRIAGE_RO_CENTRALIZED_ROUTING_PROPOSAL.md
// - WE Team Input: docs/handoff/QUESTIONS_FOR_WE_TEAM_RO_ROUTING.md
// - Confidence Assessment: docs/handoff/CONFIDENCE_ASSESSMENT_RO_CENTRALIZED_ROUTING_V2.md (98%)
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
// Reference: docs/handoff/RO_VICEVERSA_PATTERN_IMPLEMENTATION.md
//
// +kubebuilder:validation:Enum=Pending;Processing;Analyzing;AwaitingApproval;Executing;Blocked;Completed;Failed;TimedOut;Skipped;Cancelled
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

// BlockReason represents the reason why a RemediationRequest is blocked (non-terminal).
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
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

	// EffectivenessAssessmentRef tracks the EffectivenessAssessment CRD created for this remediation.
	// Set by the RO after creating the EA CRD on terminal phase transitions.
	// Reference: ADR-EM-001
	// +optional
	EffectivenessAssessmentRef *corev1.ObjectReference `json:"effectivenessAssessmentRef,omitempty"`

	// PreRemediationSpecHash is the canonical spec hash of the target resource captured
	// by the RO BEFORE launching the remediation workflow. This enables the EM to compare
	// pre vs post-remediation state without querying DataStorage audit events.
	// Set once by the RO during the transition to WorkflowExecution phase; immutable after.
	// Reference: ADR-EM-001, DD-EM-002
	// +optional
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="preRemediationSpecHash is immutable once set"
	PreRemediationSpecHash string `json:"preRemediationSpecHash,omitempty"`

	// Approval notification tracking (BR-ORCH-001)
	// Prevents duplicate notifications when AIAnalysis requires approval
	ApprovalNotificationSent bool `json:"approvalNotificationSent,omitempty"`

	// ========================================
	// SKIPPED PHASE TRACKING (DD-RO-001, DD-RO-002)
	// BR-ORCH-032, BR-ORCH-033, BR-ORCH-034
	// V1.0 Update: Enhanced for centralized routing (DD-RO-002)
	// ========================================

	// SkipReason indicates why this remediation was skipped
	// Valid values:
	// - "ResourceBusy": Another workflow executing on same target
	// - "RecentlyRemediated": Target recently remediated, cooldown period active
	// - "ExponentialBackoff": Pre-execution failures, backoff window active
	// - "ExhaustedRetries": Max consecutive failures reached
	// - "PreviousExecutionFailed": Previous execution failed during workflow run
	// Only set when OverallPhase = "Skipped" or "Failed"
	// Reference: DD-RO-002 (centralized routing responsibility)
	SkipReason string `json:"skipReason,omitempty"`

	// SkipMessage provides human-readable details about why remediation was skipped
	// Examples:
	// - "Same workflow executed recently. Cooldown: 3m15s remaining"
	// - "Another workflow is running on target: wfe-abc123"
	// - "Backoff active. Next allowed: 2025-12-15T10:30:00Z"
	// Only set when OverallPhase = "Skipped" or "Failed"
	// Reference: DD-RO-002 (centralized routing responsibility)
	// +optional
	SkipMessage string `json:"skipMessage,omitempty"`

	// BlockingWorkflowExecution references the WorkflowExecution causing the block
	// Set for block reasons: ResourceBusy, RecentlyRemediated, ExponentialBackoff
	// Nil for: ConsecutiveFailures, DuplicateInProgress
	// Enables operators to investigate the blocking WFE for troubleshooting
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockingWorkflowExecution string `json:"blockingWorkflowExecution,omitempty"`

	// DuplicateOf references the parent RemediationRequest that this is a duplicate of
	// V1.0: Set when OverallPhase = "Blocked" with BlockReason = "DuplicateInProgress"
	// Old behavior: Set when OverallPhase = "Skipped" due to resource lock deduplication
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
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
	// BLOCKED PHASE TRACKING (DD-RO-002 Blocked Phase Semantics)
	// V1.0 Update: Unified blocking for all temporary wait states
	// Prevents Gateway RR flood by keeping phase non-terminal
	// See: docs/architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md
	// ========================================

	// BlockReason indicates why this remediation is blocked (non-terminal)
	// Valid values:
	// - "ConsecutiveFailures": Max consecutive failures reached, in cooldown (BR-ORCH-042)
	// - "ResourceBusy": Another workflow is using the target resource
	// - "RecentlyRemediated": Target recently remediated, cooldown active (DD-WE-001)
	// - "ExponentialBackoff": Pre-execution failures, backoff window active (DD-WE-004)
	// - "DuplicateInProgress": Duplicate of an active remediation
	// Only set when OverallPhase = "Blocked"
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockReason string `json:"blockReason,omitempty"`

	// BlockMessage provides human-readable details about why remediation is blocked
	// Examples:
	// - "Another workflow is running on target deployment/my-app: wfe-abc123"
	// - "Recently remediated. Cooldown: 3m15s remaining"
	// - "Backoff active. Next retry: 2025-12-15T10:30:00Z"
	// - "Duplicate of active remediation rr-original-abc123"
	// - "3 consecutive failures. Cooldown expires: 2025-12-15T11:00:00Z"
	// Only set when OverallPhase = "Blocked"
	// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockMessage string `json:"blockMessage,omitempty"`

	// BlockedUntil indicates when blocking expires (time-based blocks)
	// Set for: ConsecutiveFailures, RecentlyRemediated, ExponentialBackoff
	// Nil for: ResourceBusy, DuplicateInProgress (event-based, cleared when condition resolves)
	// After this time passes, RR will retry or transition to Failed (for ConsecutiveFailures)
	// Reference: BR-ORCH-042, DD-RO-002-ADDENDUM (Blocked Phase Semantics)
	// +optional
	BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

	// ========================================
	// EXPONENTIAL BACKOFF (DD-WE-004, V1.0)
	// ========================================

	// NextAllowedExecution indicates when this RR can be retried after exponential backoff.
	// Set when RR fails due to pre-execution failures (infrastructure, validation, etc.).
	// Implements progressive delay: 1m, 2m, 4m, 8m, capped at 10m.
	// Formula: min(Base × 2^(failures-1), Max)
	// Nil means no exponential backoff is active.
	// Reference: DD-WE-004 (Exponential Backoff Cooldown)
	// +optional
	NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`

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
	// NOTIFICATION LIFECYCLE TRACKING (BR-ORCH-029/030/031)
	// ========================================

	// NotificationStatus tracks the delivery status of notification(s) for this remediation.
	// Values: "Pending", "InProgress", "Sent", "Failed", "Cancelled"
	//
	// Status Mapping from NotificationRequest.Status.Phase:
	// - NotificationRequest Pending → "Pending"
	// - NotificationRequest Sending → "InProgress"
	// - NotificationRequest Sent → "Sent"
	// - NotificationRequest Failed → "Failed"
	// - NotificationRequest deleted by user → "Cancelled"
	//
	// For bulk notifications (BR-ORCH-034), this reflects the status of the consolidated notification.
	//
	// Reference: BR-ORCH-030 (notification status tracking)
	// +optional
	// +kubebuilder:validation:Enum=Pending;InProgress;Sent;Failed;Cancelled
	NotificationStatus string `json:"notificationStatus,omitempty"`

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

	// ========================================
	// OPERATOR MUTATION TRACKING (SOC2 CC8.1)
	// ========================================

	// LastModifiedBy tracks the last operator who modified this RR's status.
	// Populated by RemediationRequest mutating webhook.
	// Reference: BR-AUTH-001 (SOC2 CC8.1 Operator Attribution), Gap #8 Extension
	// +optional
	LastModifiedBy string `json:"lastModifiedBy,omitempty"`

	// LastModifiedAt tracks when the last status modification occurred.
	// Populated by RemediationRequest mutating webhook.
	// +optional
	LastModifiedAt *metav1.Time `json:"lastModifiedAt,omitempty"`

	// CurrentProcessingRef references the current SignalProcessing CRD
	CurrentProcessingRef *corev1.ObjectReference `json:"currentProcessingRef,omitempty"`

	// ========================================
	// WORKFLOW REFERENCES (BR-AUDIT-005 Gap #5-6)
	// ========================================

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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.overallPhase`
// +kubebuilder:printcolumn:name="Outcome",type=string,JSONPath=`.status.outcome`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
