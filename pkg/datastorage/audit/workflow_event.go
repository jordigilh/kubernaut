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

package audit

// WorkflowEventData represents Workflow service event_data structure.
//
// Workflow Service creates audit events for:
// - Workflow execution lifecycle (started, executing, completed, failed)
// - Step execution tracking
// - Approval decisions
// - Execution outcomes
//
// Business Requirement: BR-STORAGE-033-010
type WorkflowEventData struct {
	WorkflowID       string `json:"workflow_id"`                 // Workflow identifier
	ExecutionID      string `json:"execution_id,omitempty"`      // Unique execution identifier
	Phase            string `json:"phase,omitempty"`             // "pending", "executing", "completed", "failed"
	CurrentStep      int    `json:"current_step,omitempty"`      // Current step number
	TotalSteps       int    `json:"total_steps,omitempty"`       // Total number of steps
	StepName         string `json:"step_name,omitempty"`         // Current step name
	DurationMs       int64  `json:"duration_ms,omitempty"`       // Execution duration
	Outcome          string `json:"outcome,omitempty"`           // "success", "failed", "cancelled"
	ApprovalRequired bool   `json:"approval_required"`           // Whether approval is required
	ApprovalDecision string `json:"approval_decision,omitempty"` // "approved", "rejected"
	Approver         string `json:"approver,omitempty"`          // Approver identifier
	ErrorMessage     string `json:"error_message,omitempty"`     // Error message if failed
}

// WorkflowEventBuilder builds Workflow-specific event data.
//
// Usage:
//
//	eventData, err := audit.NewWorkflowEvent("workflow.completed").
//	    WithWorkflowID("workflow-increase-memory").
//	    WithExecutionID("exec-2025-001").
//	    WithPhase("completed").
//	    WithCurrentStep(5, 5).
//	    WithDuration(45000).
//	    WithOutcome("success").
//	    Build()
//
// Business Requirement: BR-STORAGE-033-011
type WorkflowEventBuilder struct {
	*BaseEventBuilder
	workflowData WorkflowEventData
}

// NewWorkflowEvent creates a new Workflow event builder.
//
// Parameters:
// - eventType: Specific Workflow event type (e.g., "workflow.started", "workflow.completed", "workflow.failed")
//
// Example:
//
//	builder := audit.NewWorkflowEvent("workflow.started")
func NewWorkflowEvent(eventType string) *WorkflowEventBuilder {
	return &WorkflowEventBuilder{
		BaseEventBuilder: NewEventBuilder("workflow", eventType),
		workflowData:     WorkflowEventData{},
	}
}

// WithWorkflowID sets the workflow identifier.
//
// Example:
//
//	builder.WithWorkflowID("workflow-increase-memory-limits")
func (b *WorkflowEventBuilder) WithWorkflowID(workflowID string) *WorkflowEventBuilder {
	b.workflowData.WorkflowID = workflowID
	return b
}

// WithExecutionID sets the unique execution identifier.
//
// Example:
//
//	builder.WithExecutionID("exec-2025-11-18-001")
func (b *WorkflowEventBuilder) WithExecutionID(executionID string) *WorkflowEventBuilder {
	b.workflowData.ExecutionID = executionID
	return b
}

// WithPhase sets the workflow execution phase.
//
// Valid phases:
// - "pending": Waiting to start
// - "executing": Currently executing
// - "completed": Successfully completed
// - "failed": Execution failed
// - "cancelled": Execution cancelled
//
// Example:
//
//	builder.WithPhase("executing")
//
// Business Requirement: BR-STORAGE-033-011
func (b *WorkflowEventBuilder) WithPhase(phase string) *WorkflowEventBuilder {
	b.workflowData.Phase = phase
	return b
}

// WithCurrentStep sets the current step information.
//
// Parameters:
// - currentStep: Current step number (1-indexed)
// - totalSteps: Total number of steps in workflow
//
// Example:
//
//	builder.WithCurrentStep(3, 5) // Step 3 of 5
//
// Business Requirement: BR-STORAGE-033-011
func (b *WorkflowEventBuilder) WithCurrentStep(currentStep, totalSteps int) *WorkflowEventBuilder {
	b.workflowData.CurrentStep = currentStep
	b.workflowData.TotalSteps = totalSteps
	return b
}

// WithStepName sets the current step name.
//
// Example:
//
//	builder.WithStepName("increase_memory_limits")
func (b *WorkflowEventBuilder) WithStepName(stepName string) *WorkflowEventBuilder {
	b.workflowData.StepName = stepName
	return b
}

// WithDuration sets execution duration in milliseconds.
//
// Example:
//
//	builder.WithDuration(45000) // 45 seconds
//
// Business Requirement: BR-STORAGE-033-011
func (b *WorkflowEventBuilder) WithDuration(durationMs int64) *WorkflowEventBuilder {
	b.workflowData.DurationMs = durationMs
	return b
}

// WithOutcome sets the workflow execution outcome.
//
// Valid outcomes:
// - "success": Workflow completed successfully
// - "failed": Workflow failed
// - "cancelled": Workflow was cancelled
//
// Example:
//
//	builder.WithOutcome("success")
//
// Business Requirement: BR-STORAGE-033-012
func (b *WorkflowEventBuilder) WithOutcome(outcome string) *WorkflowEventBuilder {
	b.workflowData.Outcome = outcome
	return b
}

// WithApprovalRequired sets whether approval is required.
//
// Example:
//
//	builder.WithApprovalRequired(true)
//
// Business Requirement: BR-STORAGE-033-012
func (b *WorkflowEventBuilder) WithApprovalRequired(required bool) *WorkflowEventBuilder {
	b.workflowData.ApprovalRequired = required
	return b
}

// WithApprovalDecision sets the approval decision and approver.
//
// Parameters:
// - decision: Approval decision ("approved", "rejected")
// - approver: Approver identifier (email, username, etc.)
//
// Example:
//
//	builder.WithApprovalDecision("approved", "sre-team@example.com")
//
// Business Requirement: BR-STORAGE-033-012
func (b *WorkflowEventBuilder) WithApprovalDecision(decision, approver string) *WorkflowEventBuilder {
	b.workflowData.ApprovalDecision = decision
	b.workflowData.Approver = approver
	return b
}

// WithErrorMessage sets the error message if workflow failed.
//
// Example:
//
//	builder.WithErrorMessage("Failed to apply resource: connection timeout")
func (b *WorkflowEventBuilder) WithErrorMessage(errorMessage string) *WorkflowEventBuilder {
	b.workflowData.ErrorMessage = errorMessage
	return b
}

// Build constructs the final event_data JSONB.
//
// Returns:
// - map[string]interface{}: JSONB-ready event data
// - error: JSON marshaling error (should not occur for valid inputs)
//
// Example:
//
//	eventData, err := builder.Build()
//	if err != nil {
//	    return fmt.Errorf("failed to build Workflow event: %w", err)
//	}
func (b *WorkflowEventBuilder) Build() (map[string]interface{}, error) {
	// Add Workflow-specific data to base event
	b.WithCustomField("workflow", b.workflowData)
	return b.BaseEventBuilder.Build()
}
