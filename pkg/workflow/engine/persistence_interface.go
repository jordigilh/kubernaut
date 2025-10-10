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

package engine

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/workflow/shared"
)

// WorkflowPersistence defines the interface for workflow state persistence implementations
// Supporting business requirements:
// - BR-WF-004: MUST provide workflow state management and persistence
// - BR-REL-004: MUST recover workflow state after system restarts
// - BR-DATA-011: MUST persist workflow execution state reliably
// - BR-DATA-012: MUST support state snapshots and checkpointing
// - BR-DATA-013: MUST implement state recovery and restoration capabilities
// - BR-CONS-001: Complete interface implementations for workflow engine constructors
type WorkflowPersistence interface {
	// Core State Management - BR-WF-004
	SaveWorkflowState(ctx context.Context, execution *RuntimeWorkflowExecution) error
	LoadWorkflowState(ctx context.Context, executionID string) (*RuntimeWorkflowExecution, error)
	DeleteWorkflowState(ctx context.Context, executionID string) error

	// Recovery Operations - BR-REL-004, BR-DATA-013
	RecoverWorkflowStates(ctx context.Context) ([]*RuntimeWorkflowExecution, error)
	GetStateAnalytics(ctx context.Context) (*shared.StateAnalytics, error)

	// Checkpointing - BR-DATA-012
	CreateCheckpoint(ctx context.Context, execution *RuntimeWorkflowExecution, name string) (*shared.WorkflowCheckpoint, error)
	RestoreFromCheckpoint(ctx context.Context, checkpointID string) (*RuntimeWorkflowExecution, error)
	ValidateCheckpoint(ctx context.Context, checkpointID string) (bool, error)
}

// WorkflowPersistenceFactory creates persistence implementations based on configuration
// Supporting BR-CONS-001: Complete interface implementations for workflow engine constructors
type WorkflowPersistenceFactory interface {
	CreatePersistence(persistenceType string, config map[string]interface{}) (WorkflowPersistence, error)
	GetSupportedTypes() []string
	GetCapabilities(persistenceType string) (*shared.WorkflowPersistenceCapabilities, error)
}

// PersistenceHealthChecker provides health monitoring for persistence implementations
// Following project guideline: Always handle errors, never ignore them
type PersistenceHealthChecker interface {
	IsHealthy(ctx context.Context) error
	GetMetrics(ctx context.Context) (*shared.PersistenceMetrics, error)
}
