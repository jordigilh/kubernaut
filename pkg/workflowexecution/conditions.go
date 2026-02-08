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

package workflowexecution

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/conditions"
)

// ========================================
// KUBERNETES CONDITIONS (BR-WE-006)
// 📋 Design Decision: BR-WE-006 | ✅ Approved Design | Confidence: 95%
// See: docs/handoff/REQUEST_WE_KUBERNETES_CONDITIONS_IMPLEMENTATION.md
// ========================================
//
// Kubernetes Conditions provide standardized status information for WorkflowExecution CRD.
// These conditions surface execution resource state (PipelineRun or Job) to operators via `kubectl describe`.
//
// WHY BR-WE-006?
// - ✅ Observability: Operators see execution state without querying backend directly
// - ✅ Contract Compliance: DD-CONTRACT-001 v1.4 requires Conditions field
// - ✅ Consistency: Aligns with AIAnalysis and Kubernetes API conventions
// - ✅ Backend-Agnostic: Conditions work for both Tekton PipelineRun and K8s Job backends
//
// CONDITION LIFECYCLE:
// 1. ResourceLocked → Check if target resource is busy (PhaseSkipped)
// 2. ExecutionCreated → Execution resource created in kubernaut-workflows namespace
// 3. ExecutionRunning → Execution resource started (PipelineRun or Job)
// 4. ExecutionComplete → Execution succeeded or failed
// 5. AuditRecorded → Audit event written to Data Storage (BR-WE-005)
// ========================================

// Condition types for WorkflowExecution
const (
	// ConditionExecutionCreated indicates the execution resource (PipelineRun or Job) was created
	// Phase Alignment: Pending → Running transition
	// Set after successful creation of the execution resource
	ConditionExecutionCreated = "ExecutionCreated"

	// ConditionExecutionRunning indicates the execution resource (PipelineRun or Job) is executing
	// Phase Alignment: PhaseRunning
	// Set when execution resource reports active/running status
	ConditionExecutionRunning = "ExecutionRunning"

	// ConditionExecutionComplete indicates the execution resource (PipelineRun or Job) finished
	// Phase Alignment: PhaseCompleted OR PhaseFailed
	// Set when execution resource reaches terminal state (Succeeded/Failed)
	ConditionExecutionComplete = "ExecutionComplete"

	// ConditionAuditRecorded indicates audit event was written to Data Storage
	// Phase Alignment: All phases (cross-cutting concern)
	// Per BR-WE-005: WorkflowExecution is P0 for audit traces
	ConditionAuditRecorded = "AuditRecorded"

	// ConditionResourceLocked indicates target resource is busy or recently remediated
	// Phase Alignment: PhaseSkipped
	// Per DD-WE-001/003: Resource locking prevents parallel execution
	ConditionResourceLocked = "ResourceLocked"
)

// Condition reasons for ExecutionCreated
const (
	// ReasonExecutionCreated - Execution resource created successfully
	ReasonExecutionCreated = "ExecutionCreated"

	// ReasonExecutionCreationFailed - Execution resource creation failed
	ReasonExecutionCreationFailed = "ExecutionCreationFailed"

	// ReasonQuotaExceeded - Kubernetes resource quota exceeded
	ReasonQuotaExceeded = "QuotaExceeded"

	// ReasonRBACDenied - Insufficient RBAC permissions
	ReasonRBACDenied = "RBACDenied"

	// ReasonImagePullFailed - Container image pull failed
	ReasonImagePullFailed = "ImagePullFailed"
)

// Condition reasons for ExecutionRunning
const (
	// ReasonExecutionStarted - Execution resource started
	ReasonExecutionStarted = "ExecutionStarted"

	// ReasonExecutionFailedToStart - Execution resource stuck in pending
	ReasonExecutionFailedToStart = "ExecutionFailedToStart"
)

// Condition reasons for ExecutionComplete
const (
	// ReasonExecutionSucceeded - Execution completed successfully
	ReasonExecutionSucceeded = "ExecutionSucceeded"

	// ReasonExecutionFailed - Execution failed
	ReasonExecutionFailed = "ExecutionFailed"

	// ReasonTaskFailed - Tekton Task failed
	ReasonTaskFailed = "TaskFailed"

	// ReasonDeadlineExceeded - Pipeline timeout
	ReasonDeadlineExceeded = "DeadlineExceeded"

	// ReasonOOMKilled - Out of memory
	ReasonOOMKilled = "OOMKilled"
)

// Condition reasons for AuditRecorded
const (
	// ReasonAuditSucceeded - Audit event written to Data Storage
	ReasonAuditSucceeded = "AuditSucceeded"

	// ReasonAuditFailed - Audit write failed (Data Storage unavailable)
	ReasonAuditFailed = "AuditFailed"
)

// Condition reasons for ResourceLocked
const (
	// ReasonTargetResourceBusy - Another workflow is executing on target
	ReasonTargetResourceBusy = "TargetResourceBusy"

	// ReasonRecentlyRemediated - Same workflow executed recently (cooldown active)
	ReasonRecentlyRemediated = "RecentlyRemediated"

	// ReasonPreviousExecutionFailed - Previous execution failed (retry blocked)
	ReasonPreviousExecutionFailed = "PreviousExecutionFailed"
)

// SetCondition sets or updates a condition on the WorkflowExecution status
// This is the low-level function used by all condition setters
//
// This function delegates to pkg/shared/conditions.Set() for the actual implementation,
// ensuring consistent behavior across all services.
func SetCondition(wfe *workflowexecutionv1.WorkflowExecution, conditionType string, status metav1.ConditionStatus, reason, message string) {
	conditions.Set(&wfe.Status.Conditions, conditionType, status, reason, message)
}

// GetCondition returns the condition with the specified type, or nil if not found
//
// This function delegates to pkg/shared/conditions.Get() for the actual implementation,
// ensuring consistent behavior across all services.
func GetCondition(wfe *workflowexecutionv1.WorkflowExecution, conditionType string) *metav1.Condition {
	return conditions.Get(wfe.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if the condition exists and has status True
//
// This function delegates to pkg/shared/conditions.IsTrue() for the actual implementation,
// ensuring consistent behavior across all services.
func IsConditionTrue(wfe *workflowexecutionv1.WorkflowExecution, conditionType string) bool {
	return conditions.IsTrue(wfe.Status.Conditions, conditionType)
}

// ========================================
// HIGH-LEVEL CONDITION SETTERS
// ========================================

// SetExecutionCreated sets the ExecutionCreated condition
//
// Usage:
//
//	// Success case
//	SetExecutionCreated(wfe, true, ReasonExecutionCreated,
//	    fmt.Sprintf("Execution resource %s created in %s namespace", name, namespace))
//
//	// Failure case
//	SetExecutionCreated(wfe, false, ReasonQuotaExceeded,
//	    "Failed to create execution resource: pods exceeded quota")
func SetExecutionCreated(wfe *workflowexecutionv1.WorkflowExecution, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}
	SetCondition(wfe, ConditionExecutionCreated, status, reason, message)
}

// SetExecutionRunning sets the ExecutionRunning condition
//
// Usage:
//
//	// Execution started
//	SetExecutionRunning(wfe, true, ReasonExecutionStarted,
//	    "Execution resource running (task 2 of 5)")
//
//	// Execution failed to start
//	SetExecutionRunning(wfe, false, ReasonExecutionFailedToStart,
//	    "Execution resource stuck in pending: node pressure")
func SetExecutionRunning(wfe *workflowexecutionv1.WorkflowExecution, running bool, reason, message string) {
	status := metav1.ConditionTrue
	if !running {
		status = metav1.ConditionFalse
	}
	SetCondition(wfe, ConditionExecutionRunning, status, reason, message)
}

// SetExecutionComplete sets the ExecutionComplete condition
//
// Usage:
//
//	// Execution succeeded
//	SetExecutionComplete(wfe, true, ReasonExecutionSucceeded,
//	    "All tasks completed successfully in 45s")
//
//	// Execution failed
//	SetExecutionComplete(wfe, false, ReasonTaskFailed,
//	    "Task apply-memory-increase failed: kubectl apply failed with exit code 1")
func SetExecutionComplete(wfe *workflowexecutionv1.WorkflowExecution, succeeded bool, reason, message string) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(wfe, ConditionExecutionComplete, status, reason, message)
}

// SetAuditRecorded sets the AuditRecorded condition
//
// Usage:
//
//	// Audit succeeded
//	SetAuditRecorded(wfe, true, ReasonAuditSucceeded,
//	    "Audit event workflowexecution.workflow.completed recorded to DataStorage")
//
//	// Audit failed
//	SetAuditRecorded(wfe, false, ReasonAuditFailed,
//	    "Failed to record audit event: DataStorage unavailable")
func SetAuditRecorded(wfe *workflowexecutionv1.WorkflowExecution, succeeded bool, reason, message string) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(wfe, ConditionAuditRecorded, status, reason, message)
}

// SetResourceLocked sets the ResourceLocked condition
//
// Usage:
//
//	// Resource busy
//	SetResourceLocked(wfe, true, ReasonTargetResourceBusy,
//	    "Another workflow (workflow-exec-xyz789) is executing on target deployment/payment-api")
//
//	// Cooldown active
//	SetResourceLocked(wfe, true, ReasonRecentlyRemediated,
//	    "Same workflow executed on target 30s ago (cooldown: 5m)")
//
//	// Resource available (not typically set, but supported)
//	SetResourceLocked(wfe, false, "ResourceAvailable",
//	    "Target resource is available for execution")
func SetResourceLocked(wfe *workflowexecutionv1.WorkflowExecution, locked bool, reason, message string) {
	status := metav1.ConditionTrue
	if !locked {
		status = metav1.ConditionFalse
	}
	SetCondition(wfe, ConditionResourceLocked, status, reason, message)
}
