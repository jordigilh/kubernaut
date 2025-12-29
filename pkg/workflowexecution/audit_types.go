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

import "time"

// ========================================
// WorkflowExecution Audit Type Safety
// Per DD-AUDIT-004 and 02-go-coding-standards.mdc
// ========================================
//
// This file implements type-safe audit event payloads for WorkflowExecution,
// eliminating `map[string]interface{}` usage per coding standards.
//
// **Violations Fixed**:
// - ❌ BEFORE: map[string]interface{} with runtime-only field validation
// - ✅ AFTER: Structured types with compile-time validation
//
// **Benefits**:
// - ✅ Type Safety: Compile-time validation of all fields
// - ✅ Coding Standards: Zero map[string]interface{} in business logic
// - ✅ Maintainability: Refactor-safe, IDE autocomplete support
// - ✅ Documentation: Struct definition is authoritative schema
// - ✅ Test Coverage: 100% field validation possible
//
// **References**:
// - DD-AUDIT-004: Audit Type Safety Specification
// - ADR-032: Data Access Layer Isolation (audit mandate)
// - 02-go-coding-standards.mdc: Type System Guidelines
// ========================================

// WorkflowExecutionAuditPayload is the type-safe audit event payload structure
// for WorkflowExecution events (workflow.started, workflow.completed, workflow.failed)
//
// This replaces the previous map[string]interface{} approach with compile-time
// validated struct fields per DD-AUDIT-004.
//
// **Event Types Using This Payload**:
// - workflowexecution.workflow.started (action: "workflow.started")
// - workflowexecution.workflow.completed (action: "workflow.completed")
// - workflowexecution.workflow.failed (action: "workflow.failed")
//
// **Business Requirements**:
// - BR-WE-005: Audit Trail (all workflow lifecycle events)
// - BR-STORAGE-001: Complete audit trail with no data loss
type WorkflowExecutionAuditPayload struct {
	// ========================================
	// Core Workflow Fields (5 fields - always present)
	// ========================================

	// WorkflowID is the ID of the workflow being executed
	// Example: "kubectl-restart-deployment", "helm-rollback"
	WorkflowID string `json:"workflow_id"`

	// WorkflowVersion is the version of the workflow being executed
	// Example: "v1.0.0", "v2.1.3"
	WorkflowVersion string `json:"workflow_version"`

	// TargetResource is the Kubernetes resource being acted upon
	// Format: {namespace}/{kind}/{name} (namespaced) or {kind}/{name} (cluster-scoped)
	// Examples: "payment/deployment/payment-api", "node/worker-node-1"
	TargetResource string `json:"target_resource"`

	// Phase is the current phase of the WorkflowExecution
	// Values: "Pending", "Running", "Completed", "Failed"
	Phase string `json:"phase"`

	// ContainerImage is the Tekton PipelineRun container image
	// Example: "ghcr.io/kubernaut/kubectl-actions:v1.28"
	ContainerImage string `json:"container_image"`

	// ExecutionName is the name of the WorkflowExecution CRD
	// Example: "restart-payment-api-2025-12-17-abc123"
	ExecutionName string `json:"execution_name"`

	// ========================================
	// Timing Fields (3 fields - conditional)
	// Present when: Phase is Running, Completed, or Failed
	// ========================================

	// StartedAt is when the PipelineRun started execution
	// Present when: Phase is Running, Completed, or Failed
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the PipelineRun finished (success or failure)
	// Present when: Phase is Completed or Failed
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Duration is the human-readable execution duration
	// Format: "2m30s", "45s", "1h15m30s"
	// Present when: Phase is Completed or Failed
	Duration string `json:"duration,omitempty"`

	// ========================================
	// Failure Fields (3 fields - conditional)
	// Present when: Phase is Failed
	// ========================================

	// FailureReason is the categorized failure reason
	// Values: "OOMKilled", "DeadlineExceeded", "Forbidden", "ImagePullBackOff",
	//         "ConfigurationError", "ResourceExhausted", "TaskFailed", "Unknown"
	// Present when: Phase is Failed
	FailureReason string `json:"failure_reason,omitempty"`

	// FailureMessage is the detailed failure message from Tekton
	// Present when: Phase is Failed
	FailureMessage string `json:"failure_message,omitempty"`

	// FailedTaskName is the name of the failed TaskRun (if identified)
	// Present when: Phase is Failed and a specific TaskRun failed
	FailedTaskName string `json:"failed_task_name,omitempty"`

	// ========================================
	// PipelineRun Reference (1 field - conditional)
	// Present when: Phase is Running, Completed, or Failed
	// ========================================

	// PipelineRunName is the name of the associated Tekton PipelineRun
	// Example: "restart-payment-api-2025-12-17-abc123-run"
	// Present when: PipelineRun has been created
	PipelineRunName string `json:"pipelinerun_name,omitempty"`
}

// ========================================
// AUDIT PATTERN V2.2 - ZERO UNSTRUCTURED DATA
// ========================================
//
// Per DD-AUDIT-004 v1.3 (Dec 17, 2025):
// - ❌ DO NOT create custom ToMap() methods
// - ❌ DO NOT use audit.StructToMap() (deprecated)
// - ✅ USE direct audit.SetEventData() with structured types
//
// **Design Rationale** (V2.2):
// - Zero Unstructured Data: No map[string]interface{} anywhere
// - Direct Assignment: SetEventData() accepts interface{} directly
// - Type Safety: Structured Go types with compile-time validation
// - Simplicity: 67% code reduction (3 lines → 1 line)
// - Polymorphic API: Multiple services, same endpoint, different payloads
//
// **Usage Pattern** (V2.2):
//
//	payload := WorkflowExecutionAuditPayload{
//	    WorkflowID:     "kubectl-restart",
//	    TargetResource: "payment/deployment/payment-api",
//	    Phase:          "Running",
//	    // ...
//	}
//	audit.SetEventData(event, payload)  // Direct assignment!
//
// **Migration History**:
// - Dec 17, 2025 (Early): Removed custom ToMap() → used audit.StructToMap()
// - Dec 17, 2025 (Later): Migrated to V2.2 direct assignment pattern
//
// **Authority**: DD-AUDIT-004 v1.3, DD-AUDIT-002 v2.2
// ========================================
