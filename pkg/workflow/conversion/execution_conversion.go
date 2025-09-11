package conversion

import (
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// ExecutionConverter provides conversion between different workflow execution types
type ExecutionConverter struct{}

// NewExecutionConverter creates a new execution converter
func NewExecutionConverter() *ExecutionConverter {
	return &ExecutionConverter{}
}

// RuntimeToRecord converts a RuntimeWorkflowExecution to a WorkflowExecutionRecord for analytics
// This is now trivial since RuntimeWorkflowExecution embeds WorkflowExecutionRecord
func (ec *ExecutionConverter) RuntimeToRecord(rwe *engine.RuntimeWorkflowExecution) *types.WorkflowExecutionRecord {
	if rwe == nil {
		return nil
	}

	// Zero-cost conversion: just return the embedded struct
	record := rwe.WorkflowExecutionRecord
	// Sync the status field (engine uses enum, analytics uses string)
	record.Status = string(rwe.OperationalStatus)
	return &record
}

// RecordToRuntime converts a WorkflowExecutionRecord to a minimal RuntimeWorkflowExecution
// Note: This creates a minimal runtime execution with defaults for fields not available in the record.
// This is primarily useful for testing and migration scenarios.
func (ec *ExecutionConverter) RecordToRuntime(wer *types.WorkflowExecutionRecord) *engine.RuntimeWorkflowExecution {
	if wer == nil {
		return nil
	}

	var duration engine.ExecutionStatus
	switch wer.Status {
	case "pending":
		duration = engine.ExecutionStatusPending
	case "running":
		duration = engine.ExecutionStatusRunning
	case "completed":
		duration = engine.ExecutionStatusCompleted
	case "failed":
		duration = engine.ExecutionStatusFailed
	case "cancelled":
		duration = engine.ExecutionStatusCancelled
	default:
		duration = engine.ExecutionStatusPending
	}

	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: *wer,     // Embed the record directly
		OperationalStatus:       duration, // Set operational status with enum type
		Input:                   nil,      // Not available in record
		Output:                  nil,      // Not available in record
		Context:                 nil,      // Not available in record
		Steps:                   nil,      // Not available in record
		CurrentStep:             0,        // Default
		Duration:                0,        // Would need calculation
		Error:                   "",       // Not available in record
	}
}

// BatchRuntimeToRecord converts multiple RuntimeWorkflowExecution to WorkflowExecutionRecord
func (ec *ExecutionConverter) BatchRuntimeToRecord(executions []*engine.RuntimeWorkflowExecution) []*types.WorkflowExecutionRecord {
	if executions == nil {
		return nil
	}

	records := make([]*types.WorkflowExecutionRecord, len(executions))
	for i, exec := range executions {
		records[i] = ec.RuntimeToRecord(exec)
	}
	return records
}

// BatchRecordToRuntime converts multiple WorkflowExecutionRecord to RuntimeWorkflowExecution
func (ec *ExecutionConverter) BatchRecordToRuntime(records []*types.WorkflowExecutionRecord) []*engine.RuntimeWorkflowExecution {
	if records == nil {
		return nil
	}

	executions := make([]*engine.RuntimeWorkflowExecution, len(records))
	for i, record := range records {
		executions[i] = ec.RecordToRuntime(record)
	}
	return executions
}
