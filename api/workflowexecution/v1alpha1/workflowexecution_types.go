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

// ═══════════════════════════════════════════════════════════════════════════
// VERSION: v1alpha1-v1.0-executor
// Last Updated: December 19, 2025
// Status: ✅ WE "Pure Executor" Complete + BR-WE-013 (SOC2 Compliance)
// ═══════════════════════════════════════════════════════════════════════════
//
// CHANGELOG:
//
// ## V1.0 (IN PROGRESS - Started December 14, 2025)
//
// ### ✅ BR-WE-013: Audit-Tracked Execution Block Clearing (December 19, 2025)
//
// **Added**: BlockClearanceDetails type and WorkflowExecutionStatus.BlockClearance field
// **Priority**: P0 (CRITICAL) - SOC2 Type II Compliance Requirement
// **Rationale**: Current v1.0 workaround (deleting WFE CRDs) violates SOC2 audit trail requirements
// **Compliance**: SOC2 CC7.3 (Immutability), CC7.4 (Completeness), CC8.1 (Attribution)
//
// **Fields Added**:
// - BlockClearanceDetails.ClearedAt (timestamp)
// - BlockClearanceDetails.ClearedBy (operator identity)
// - BlockClearanceDetails.ClearReason (reason for clearing)
// - BlockClearanceDetails.ClearMethod (Annotation|APIEndpoint|StatusField)
// - WorkflowExecutionStatus.BlockClearance (*BlockClearanceDetails)
//
// ### ✅ Phase 1: API Foundation (Day 1) - COMPLETE
//
// **Removed from api/workflowexecution/v1alpha1 (API Package)**:
// - SkipDetails type definition (moved to WE controller as temporary stubs)
// - ConflictingWorkflowRef type definition (moved to WE controller as temporary stubs)
// - RecentRemediationRef type definition (moved to WE controller as temporary stubs)
// - Phase value "Skipped" from enum (WFE now: Pending, Running, Completed, Failed)
// - Skip reason constants: ResourceBusy, RecentlyRemediated, ExhaustedRetries, PreviousExecutionFailed
//
// **Status**: Day 1 compatibility stubs created in WE controller (internal/controller/workflowexecution/v1_compat_stubs.go)
// These stubs are TEMPORARY and marked for removal in Phase 2 (Days 6-7).
//
// ### 🔄 Phase 2: RO Routing Logic (Days 2-5) - IN PROGRESS
//
// **Status**: RO Team working in parallel (Dec 17-20)
// **Planned Changes**:
// - RO will implement routing decision logic BEFORE creating WFE
// - 5 routing checks: resource lock, cooldown, exponential backoff, exhausted retries, previous execution failure
// - Field index on WorkflowExecution.spec.targetResource for efficient queries
// - Population of RemediationRequest.Status.skipMessage and blockingWorkflowExecution
//
// **Current State**: RO fixing integration tests (27/52 failing) + implementing routing logic
//
// ### ✅ Phase 3: WE Simplification (Days 6-7) - COMPLETE
//
// **Status**: WorkflowExecution controller is already in "pure executor" state
// **Verified**: December 17, 2025
// **Evidence**: docs/handoff/WE_PURE_EXECUTOR_VERIFICATION.md
//
// **Functions That DO NOT Exist** (Routing Removed):
// - ❌ CheckCooldown() - Not found in codebase
// - ❌ CheckResourceLock() - Not found in codebase
// - ❌ MarkSkipped() - Not found in codebase
// - ❌ FindMostRecentTerminalWFE() - Not found in codebase
// - ❌ v1_compat_stubs.go - File does not exist
//
// **Functions That DO Exist** (Pure Execution):
// - ✅ reconcilePending() - Create PipelineRun (no routing checks)
// - ✅ reconcileRunning() - Sync PipelineRun status
// - ✅ ReconcileTerminal() - Lock cleanup after cooldown
// - ✅ HandleAlreadyExists() - Execution-time collision handling
//
// **Result**: WE controller is a "pure executor" - RO makes ALL routing decisions
//
// ### 🎯 Design Decision: DD-RO-002 - Centralized Routing Responsibility (TARGET)
//
// **Target Architecture** (NOT YET IMPLEMENTED):
// - WorkflowExecution becomes a pure executor (no routing logic)
// - RemediationOrchestrator makes ALL routing decisions before creating WFE
// - If a workflow should be skipped, WFE is never created
// - Single source of truth for skip reasons (RR.Status)
//
// **Current Architecture** (AS OF DAYS 6-7 COMPLETION):
// - WorkflowExecution is a "pure executor" (NO routing logic)
// - RemediationOrchestrator makes ALL routing decisions before creating WFE
// - WFE has only 4 phases: Pending, Running, Completed, Failed (Skipped removed)
// - Skip reasons tracked ONLY in RR.Status (single source of truth)
//
// ### 📋 Related Changes (Planned):
// - RemediationRequest.Status gains skipMessage, blockingWorkflowExecution (fields added, not yet populated)
// - RO controller gains routing logic (planned for Days 2-5, not implemented)
//
// ### 📚 References:
// - Implementation Plan: docs/services/.../05-remediationorchestrator/implementation/V1.0_CENTRALIZED_ROUTING_IMPLEMENTATION_PLAN.md
// - Design Decision: docs/architecture/decisions/DD-RO-002-centralized-routing-responsibility.md (planned)
// - Proposal: docs/handoff/TRIAGE_RO_CENTRALIZED_ROUTING_PROPOSAL.md
// - Implementation Status: docs/handoff/TRIAGE_V1.0_IMPLEMENTATION_STATUS.md
// - Confidence Assessment: 98% (WE team validated PLAN, not implementation)
//
// ### ⏰ Timeline:
// - Day 1 (Dec 14-15): ✅ API Foundation Complete
// - Days 2-5: 🔄 RO Routing Logic (IN PROGRESS - RO Team Dec 17-20)
// - Days 6-7: ✅ WE Simplification (COMPLETE - Already in "pure executor" state)
// - Days 8-20: ⏳ Testing, Staging, Launch (PENDING)
// - Target: January 11, 2026 (on track with parallel development)
//
// ═══════════════════════════════════════════════════════════════════════════

// ========================================
// WorkflowExecution CRD Types
// Version: 4.0 - Aligned with ADR-044, DD-CONTRACT-001 v1.4, ADR-043
// See: docs/services/crd-controllers/03-workflowexecution/crd-schema.md
// ========================================

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
// Simplified per ADR-044 - Tekton handles step orchestration
//
// ADR-001: Spec Immutability
// WorkflowExecution represents an immutable event (workflow execution attempt).
// Once created by RemediationOrchestrator, spec cannot be modified to ensure:
// - Audit trail integrity (executed spec matches approved spec)
// - No parameter tampering after HAPI validation
// - No target resource changes after routing decisions
//
// To change execution parameters, delete and recreate the WorkflowExecution.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-001)"
type WorkflowExecutionSpec struct {
	// RemediationRequestRef references the parent RemediationRequest CRD
	RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

	// WorkflowRef contains the workflow catalog reference
	// Resolved from AIAnalysis.Status.SelectedWorkflow by RemediationOrchestrator
	WorkflowRef WorkflowRef `json:"workflowRef"`

	// TargetResource identifies the K8s resource being remediated
	// Used for resource locking (DD-WE-001) - prevents parallel workflows on same target
	// Format: "namespace/kind/name" for namespaced resources
	//         "kind/name" for cluster-scoped resources
	// Example: "payment/deployment/payment-api", "node/worker-node-1"
	TargetResource string `json:"targetResource"`

	// Parameters from LLM selection (per DD-WORKFLOW-003)
	// Keys are UPPER_SNAKE_CASE for Tekton PipelineRun params
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Confidence score from LLM (for audit trail)
	// +optional
	Confidence float64 `json:"confidence,omitempty"`

	// Rationale from LLM (for audit trail)
	// +optional
	Rationale string `json:"rationale,omitempty"`

	// ExecutionEngine specifies the backend engine for workflow execution.
	// "tekton" creates a Tekton PipelineRun; "job" creates a Kubernetes Job.
	// +kubebuilder:validation:Enum=tekton;job
	// +kubebuilder:default=tekton
	ExecutionEngine string `json:"executionEngine"`

	// ExecutionConfig contains minimal execution settings
	// +optional
	ExecutionConfig *ExecutionConfig `json:"executionConfig,omitempty"`
}

// WorkflowRef contains catalog-resolved workflow reference
type WorkflowRef struct {
	// WorkflowID is the catalog lookup key
	WorkflowID string `json:"workflowId"`

	// Version of the workflow
	Version string `json:"version"`

	// ContainerImage resolved from workflow catalog (Data Storage API)
	// OCI bundle reference for Tekton PipelineRun
	ContainerImage string `json:"containerImage"`

	// ContainerDigest for audit trail and reproducibility
	// +optional
	ContainerDigest string `json:"containerDigest,omitempty"`
}

// ExecutionConfig contains minimal execution settings
// Note: Most execution logic is delegated to Tekton (ADR-044)
type ExecutionConfig struct {
	// Timeout for the entire workflow (Tekton PipelineRun timeout)
	// Default: use global timeout from RemediationRequest or 30m
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// ServiceAccountName for the PipelineRun
	// Default: "kubernaut-workflow-runner"
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// WorkflowExecution phase constants
const (
	// PhasePending is the initial phase when WorkflowExecution is first created
	PhasePending = "Pending"
	// PhaseRunning indicates the PipelineRun is actively executing
	PhaseRunning = "Running"
	// PhaseCompleted indicates successful completion
	PhaseCompleted = "Completed"
	// PhaseFailed indicates a permanent failure
	PhaseFailed = "Failed"
)

// WorkflowExecutionStatus defines the observed state
// Simplified per ADR-044 - just tracks PipelineRun status
// Enhanced per DD-CONTRACT-001 v1.3 - rich failure details for recovery flow
// Enhanced per DD-CONTRACT-001 v1.4 - resource locking and Skipped phase
type WorkflowExecutionStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// Used to prevent duplicate reconciliations and ensure idempotency.
	// Per DD-CONTROLLER-001: Standard pattern for all Kubernetes controllers.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase tracks current execution stage
	// V1.0: Skipped phase removed - RO makes routing decisions before WFE creation
	// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// StartTime when execution started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime when execution completed (success or failure)
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Duration of the execution
	// +optional
	Duration string `json:"duration,omitempty"`

	// ExecutionRef references the created execution resource (PipelineRun or Job)
	// +optional
	ExecutionRef *corev1.LocalObjectReference `json:"executionRef,omitempty"`

	// ExecutionStatus mirrors key execution resource status fields
	// +optional
	ExecutionStatus *ExecutionStatusSummary `json:"executionStatus,omitempty"`

	// FailureReason explains why execution failed (if applicable)
	// DEPRECATED: Use FailureDetails for structured failure information
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// ========================================
	// ENHANCED FAILURE INFORMATION (v3.0)
	// DD-CONTRACT-001 v1.3: Rich failure data for recovery flow
	// Consumers: RO (for recovery AIAnalysis), Notification (for user alerts)
	// ========================================

	// FailureDetails contains structured failure information
	// Populated when Phase=Failed
	// RO uses this to populate AIAnalysis.Spec.PreviousExecutions for recovery
	// +optional
	FailureDetails *FailureDetails `json:"failureDetails,omitempty"`

	// ========================================
	// V1.0: RESOURCE LOCKING MOVED TO RO
	// DD-RO-002: RO makes routing decisions, SkipDetails removed from WFE
	// Skip information now in RemediationRequest.Status
	// ========================================

	// ========================================
	// EXPONENTIAL BACKOFF (v4.1) - DEPRECATED V1.0
	// DD-RO-002 Phase 3: Routing state moved to RemediationRequest.Status
	// WE is now a pure executor - routing logic owned by RO
	// ========================================

	// ConsecutiveFailures tracks pre-execution failures for this target resource
	// DEPRECATED (V1.0): Routing state moved to RR.Status.ConsecutiveFailureCount per DD-RO-002 Phase 3
	// This field is NO LONGER UPDATED by WE controller as of V1.0
	// Use RR.Status.ConsecutiveFailureCount for routing decisions
	// Will be REMOVED in V2.0
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

	// NextAllowedExecution is the timestamp when next execution is allowed
	// DEPRECATED (V1.0): Routing state moved to RR.Status.NextAllowedExecution per DD-RO-002 Phase 3
	// This field is NO LONGER UPDATED by WE controller as of V1.0
	// Use RR.Status.NextAllowedExecution for routing decisions
	// Will be REMOVED in V2.0
	// +optional
	NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`

	// ========================================
	// AUDIT-TRACKED EXECUTION BLOCK CLEARING (v4.2)
	// BR-WE-013: SOC2 Type II Compliance Requirement (v1.0)
	// Tracks operator clearing of PreviousExecutionFailed blocks
	// ========================================

	// BlockClearance tracks the clearing of PreviousExecutionFailed blocks
	// When set, allows new executions despite previous execution failure
	// Preserves audit trail of WHO cleared the block and WHY
	// +optional
	BlockClearance *BlockClearanceDetails `json:"blockClearance,omitempty"`

	// Conditions provide detailed status information
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ========================================
// V1.0: SKIP DETAILS REMOVED
// DD-RO-002: All skip information moved to RemediationRequest.Status
// Struct types removed: SkipDetails, ConflictingWorkflowRef, RecentRemediationRef
// ========================================

// ========================================
// FAILURE DETAILS (v3.0)
// DD-CONTRACT-001 v1.3: Structured failure information for recovery
// ========================================

// FailureDetails contains structured failure information for recovery
// Aligned with AIAnalysis.Spec.PreviousExecutions[].Failure
// Provides both structured data (for deterministic recovery) and natural language (for LLM context)
type FailureDetails struct {
	// FailedTaskIndex is 0-indexed position of failed task in pipeline
	// Used by RO to populate AIAnalysis.Spec.PreviousExecutions[].Failure.FailedStepIndex
	FailedTaskIndex int `json:"failedTaskIndex"`

	// FailedTaskName is the name of the failed Tekton Task
	// Used by RO to populate AIAnalysis.Spec.PreviousExecutions[].Failure.FailedStepName
	FailedTaskName string `json:"failedTaskName"`

	// FailedStepName is the name of the failed step within the task (if available)
	// Tekton tasks can have multiple steps; this identifies the specific step
	// +optional
	FailedStepName string `json:"failedStepName,omitempty"`

	// Reason is a Kubernetes-style reason code
	// Used for deterministic recovery decisions by RO
	// +kubebuilder:validation:Enum=OOMKilled;DeadlineExceeded;Forbidden;ResourceExhausted;ConfigurationError;ImagePullBackOff;TaskFailed;Unknown
	Reason string `json:"reason"`

	// Message is human-readable error message (for logging/UI/notifications)
	Message string `json:"message"`

	// ExitCode from container (if applicable)
	// Useful for script-based tasks that return specific exit codes
	// +optional
	ExitCode *int32 `json:"exitCode,omitempty"`

	// FailedAt is the timestamp when the failure occurred
	FailedAt metav1.Time `json:"failedAt"`

	// ExecutionTimeBeforeFailure is how long the workflow ran before failing
	// Format: Go duration string (e.g., "2m30s")
	ExecutionTimeBeforeFailure string `json:"executionTimeBeforeFailure"`

	// ========================================
	// NATURAL LANGUAGE SUMMARY
	// For LLM recovery context and user notifications
	// ========================================

	// NaturalLanguageSummary is a human/LLM-readable failure description
	// Generated by WE controller from structured data above
	// Used by:
	//   - RO: Included in AIAnalysis.Spec.PreviousExecutions for LLM context
	//   - Notification: Included in user-facing failure alerts
	NaturalLanguageSummary string `json:"naturalLanguageSummary"`

	// ========================================
	// EXECUTION VS PRE-EXECUTION FAILURE (v4.1)
	// DD-WE-004: Critical for retry/backoff decisions
	// ========================================

	// WasExecutionFailure indicates whether the failure occurred during workflow execution
	// true = workflow RAN and failed (non-idempotent actions may have occurred)
	// false = workflow failed BEFORE execution (validation, image pull, quota, etc.)
	// CRITICAL: Execution failures (true) block ALL future retries for this target
	//           Pre-execution failures (false) get exponential backoff
	// +optional
	WasExecutionFailure bool `json:"wasExecutionFailure,omitempty"`
}

// ========================================
// BLOCK CLEARANCE DETAILS (v4.2)
// BR-WE-013: Audit-Tracked Execution Block Clearing
// SOC2 Type II Compliance Requirement (v1.0)
// ========================================

// BlockClearanceDetails tracks the clearing of PreviousExecutionFailed blocks
// Required for SOC2 CC7.3 (Immutability), CC7.4 (Completeness), CC8.1 (Attribution)
// Preserves audit trail when operators clear execution blocks after investigation
type BlockClearanceDetails struct {
	// ClearedAt is the timestamp when the block was cleared
	// +optional
	ClearedAt metav1.Time `json:"clearedAt"`

	// ClearedBy is the Kubernetes user who cleared the block
	// Extracted from request context (if available) or annotation value
	// Format: username@domain or service-account:namespace:name
	// Example: "admin@kubernaut.ai" or "service-account:kubernaut-system:operator"
	ClearedBy string `json:"clearedBy"`

	// ClearReason is the operator-provided reason for clearing
	// Required for audit trail accountability
	// Example: "manual investigation complete, cluster state verified"
	ClearReason string `json:"clearReason"`

	// ClearMethod indicates how the block was cleared
	// Annotation: Via kubernaut.ai/clear-execution-block annotation
	// APIEndpoint: Via dedicated clearing API endpoint (future)
	// StatusField: Via direct status field update (future)
	// +kubebuilder:validation:Enum=Annotation;APIEndpoint;StatusField
	ClearMethod string `json:"clearMethod"`
}

// ExecutionStatusSummary captures key execution resource status fields
// Lightweight summary for both Tekton PipelineRun and K8s Job backends
type ExecutionStatusSummary struct {
	// Status of the execution resource (Unknown, True, False)
	Status string `json:"status"`

	// Reason from the execution resource (e.g., "Succeeded", "Failed", "Running")
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message from the execution resource
	// +optional
	Message string `json:"message,omitempty"`

	// CompletedTasks count
	// +optional
	CompletedTasks int `json:"completedTasks,omitempty"`

	// TotalTasks count (from pipeline spec)
	// +optional
	TotalTasks int `json:"totalTasks,omitempty"`
}

// ========================================
// PHASE CONSTANTS
// ========================================

// Phase constants already defined above (lines 199-208) - duplicate removed

// ========================================
// V1.0: SKIP REASON CONSTANTS REMOVED
// DD-RO-002: All skip reasons moved to RemediationRequest.Status.SkipReason
// Constants removed: SkipReasonResourceBusy, SkipReasonRecentlyRemediated,
//                    SkipReasonExhaustedRetries, SkipReasonPreviousExecutionFailed
// ========================================

// ========================================
// FAILURE REASON CONSTANTS
// ========================================

const (
	// FailureReasonOOMKilled indicates container was killed due to memory limits
	FailureReasonOOMKilled = "OOMKilled"

	// FailureReasonDeadlineExceeded indicates timeout was reached
	FailureReasonDeadlineExceeded = "DeadlineExceeded"

	// FailureReasonForbidden indicates RBAC/permission failure
	FailureReasonForbidden = "Forbidden"

	// FailureReasonResourceExhausted indicates cluster resource limits (quota, etc.)
	FailureReasonResourceExhausted = "ResourceExhausted"

	// FailureReasonConfigurationError indicates invalid parameters or config
	FailureReasonConfigurationError = "ConfigurationError"

	// FailureReasonImagePullBackOff indicates container image could not be pulled
	FailureReasonImagePullBackOff = "ImagePullBackOff"

	// FailureReasonTaskFailed indicates a Tekton task failed during execution
	// This is an execution failure (wasExecutionFailure=true)
	FailureReasonTaskFailed = "TaskFailed"

	// FailureReasonUnknown for unclassified failures
	FailureReasonUnknown = "Unknown"
)

// ========================================
// CRD DEFINITIONS
// ========================================

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=wfe
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="WorkflowID",type=string,JSONPath=`.spec.workflowRef.workflowId`
//+kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetResource`
//+kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.duration`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkflowExecution is the Schema for the workflowexecutions API
type WorkflowExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
	Status WorkflowExecutionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WorkflowExecutionList contains a list of WorkflowExecution
type WorkflowExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkflowExecution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkflowExecution{}, &WorkflowExecutionList{})
}
