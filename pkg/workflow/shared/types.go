<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package shared

import "time"

// Shared workflow execution types following project guideline #17:
// Move local types to shared types package to avoid import cycles

// WorkflowExecutionStatus represents the status of workflow execution
type WorkflowExecutionStatus string

const (
	WorkflowExecutionStatusPending   WorkflowExecutionStatus = "pending"
	WorkflowExecutionStatusRunning   WorkflowExecutionStatus = "running"
	WorkflowExecutionStatusCompleted WorkflowExecutionStatus = "completed"
	WorkflowExecutionStatusFailed    WorkflowExecutionStatus = "failed"
	WorkflowExecutionStatusCancelled WorkflowExecutionStatus = "cancelled"
	WorkflowExecutionStatusPaused    WorkflowExecutionStatus = "paused"
)

// WorkflowStepStatus represents the status of workflow step execution
type WorkflowStepStatus string

const (
	WorkflowStepStatusPending   WorkflowStepStatus = "pending"
	WorkflowStepStatusRunning   WorkflowStepStatus = "running"
	WorkflowStepStatusCompleted WorkflowStepStatus = "completed"
	WorkflowStepStatusFailed    WorkflowStepStatus = "failed"
	WorkflowStepStatusSkipped   WorkflowStepStatus = "skipped"
)

// WorkflowCheckpoint represents a workflow state checkpoint (BR-DATA-012)
// Supports state snapshots and checkpointing business requirements
type WorkflowCheckpoint struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	StateHash   string    `json:"state_hash"`
	CreatedAt   time.Time `json:"created_at"`
	// Following guideline: Use structured field values
	Metadata map[string]string `json:"metadata"`
}

// StateAnalytics represents workflow state analytics (BR-REL-004, BR-DATA-013)
// Supports recovery monitoring and business metrics
type StateAnalytics struct {
	TotalExecutions      int           `json:"total_executions"`
	ActiveExecutions     int           `json:"active_executions"`
	CompletedExecutions  int           `json:"completed_executions"`
	FailedExecutions     int           `json:"failed_executions"`
	RecoverySuccessRate  float64       `json:"recovery_success_rate"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	LastUpdated          time.Time     `json:"last_updated"`
}

// WorkflowPersistenceCapabilities represents capabilities of persistence implementations
// Supporting BR-CONS-001: Interface completeness requirements
type WorkflowPersistenceCapabilities struct {
	SupportsCheckpoints  bool `json:"supports_checkpoints"`
	SupportsRecovery     bool `json:"supports_recovery"`
	SupportsAnalytics    bool `json:"supports_analytics"`
	SupportsDistribution bool `json:"supports_distribution"`
	SupportsEncryption   bool `json:"supports_encryption"`
	SupportsCompression  bool `json:"supports_compression"`
}

// PersistenceMetrics represents performance metrics for persistence operations
// Supporting monitoring and optimization requirements
type PersistenceMetrics struct {
	SaveLatency        time.Duration `json:"save_latency"`
	LoadLatency        time.Duration `json:"load_latency"`
	RecoveryLatency    time.Duration `json:"recovery_latency"`
	StorageUtilization float64       `json:"storage_utilization"`
	ErrorRate          float64       `json:"error_rate"`
	LastMeasured       time.Time     `json:"last_measured"`
}
