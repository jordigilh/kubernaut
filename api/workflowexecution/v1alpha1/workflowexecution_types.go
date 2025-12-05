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

// ========================================
// WorkflowExecution CRD Types
// Version: 4.0 - Aligned with ADR-044, DD-CONTRACT-001 v1.4, ADR-043
// See: docs/services/crd-controllers/03-workflowexecution/crd-schema.md
// ========================================

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
// Simplified per ADR-044 - Tekton handles step orchestration
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

// WorkflowExecutionStatus defines the observed state
// Simplified per ADR-044 - just tracks PipelineRun status
// Enhanced per DD-CONTRACT-001 v1.3 - rich failure details for recovery flow
// Enhanced per DD-CONTRACT-001 v1.4 - resource locking and Skipped phase
type WorkflowExecutionStatus struct {
	// Phase tracks current execution stage
	// Skipped: Resource is busy (another workflow running) or recently remediated
	// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;Skipped
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

	// PipelineRunRef references the created Tekton PipelineRun
	// +optional
	PipelineRunRef *corev1.LocalObjectReference `json:"pipelineRunRef,omitempty"`

	// PipelineRunStatus mirrors key PipelineRun status fields
	// +optional
	PipelineRunStatus *PipelineRunStatusSummary `json:"pipelineRunStatus,omitempty"`

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
	// RESOURCE LOCKING (v3.1)
	// DD-CONTRACT-001 v1.4: Prevents parallel workflows on same target
	// ========================================

	// SkipDetails contains information about why execution was skipped
	// Populated when Phase=Skipped
	// Enables RO to understand why workflow didn't execute
	// +optional
	SkipDetails *SkipDetails `json:"skipDetails,omitempty"`

	// Conditions provide detailed status information
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ========================================
// SKIP DETAILS (v3.1)
// DD-CONTRACT-001 v1.4: Resource locking prevents parallel/redundant execution
// ========================================

// SkipDetails contains information about why a WorkflowExecution was skipped
// Provides context for notifications and audit trail
type SkipDetails struct {
	// Reason explains why execution was skipped
	// +kubebuilder:validation:Enum=ResourceBusy;RecentlyRemediated
	Reason string `json:"reason"`

	// Message is a human-readable explanation
	Message string `json:"message"`

	// SkippedAt is when the skip decision was made
	SkippedAt metav1.Time `json:"skippedAt"`

	// ConflictingWorkflow contains details about the blocking workflow
	// Populated when Reason=ResourceBusy
	// +optional
	ConflictingWorkflow *ConflictingWorkflowRef `json:"conflictingWorkflow,omitempty"`

	// RecentRemediation contains details about the recent execution
	// Populated when Reason=RecentlyRemediated
	// +optional
	RecentRemediation *RecentRemediationRef `json:"recentRemediation,omitempty"`
}

// ConflictingWorkflowRef identifies the workflow blocking this execution
type ConflictingWorkflowRef struct {
	// Name of the conflicting WorkflowExecution CRD
	Name string `json:"name"`

	// WorkflowID of the conflicting workflow
	WorkflowID string `json:"workflowId"`

	// StartedAt when the conflicting workflow started
	StartedAt metav1.Time `json:"startedAt"`

	// TargetResource is the resource being remediated (for audit trail)
	// Format: "namespace/kind/name" or "kind/name" for cluster-scoped
	TargetResource string `json:"targetResource"`
}

// RecentRemediationRef identifies the recent execution that caused skip
type RecentRemediationRef struct {
	// Name of the recent WorkflowExecution CRD
	Name string `json:"name"`

	// WorkflowID that was executed
	WorkflowID string `json:"workflowId"`

	// CompletedAt when the workflow completed
	CompletedAt metav1.Time `json:"completedAt"`

	// Outcome of the recent execution (Completed/Failed)
	Outcome string `json:"outcome"`

	// TargetResource is the resource that was remediated
	// Format: "namespace/kind/name" or "kind/name" for cluster-scoped
	TargetResource string `json:"targetResource"`

	// CooldownRemaining is how long until this target can be remediated again
	// Format: Go duration string (e.g., "4m30s")
	// +optional
	CooldownRemaining string `json:"cooldownRemaining,omitempty"`
}

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
	// +kubebuilder:validation:Enum=OOMKilled;DeadlineExceeded;Forbidden;ResourceExhausted;ConfigurationError;ImagePullBackOff;Unknown
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
}

// PipelineRunStatusSummary captures key PipelineRun status fields
// Lightweight summary to avoid duplicating full Tekton status
type PipelineRunStatusSummary struct {
	// Status from PipelineRun (Unknown, True, False)
	Status string `json:"status"`

	// Reason from PipelineRun (e.g., "Succeeded", "Failed", "Running")
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message from PipelineRun
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

const (
	// PhasePending indicates the WorkflowExecution is waiting to start
	PhasePending = "Pending"

	// PhaseRunning indicates the PipelineRun is executing
	PhaseRunning = "Running"

	// PhaseCompleted indicates the workflow completed successfully
	PhaseCompleted = "Completed"

	// PhaseFailed indicates the workflow failed
	PhaseFailed = "Failed"

	// PhaseSkipped indicates the workflow was skipped due to resource lock
	PhaseSkipped = "Skipped"
)

// ========================================
// SKIP REASON CONSTANTS
// ========================================

const (
	// SkipReasonResourceBusy indicates another workflow is running on the target
	SkipReasonResourceBusy = "ResourceBusy"

	// SkipReasonRecentlyRemediated indicates same workflow+target was recently executed
	SkipReasonRecentlyRemediated = "RecentlyRemediated"
)

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

