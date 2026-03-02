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

// Package executor defines the Strategy pattern interface for workflow execution backends.
// Each execution engine (Tekton, K8s Job) implements this interface to handle the
// lifecycle of its execution resource (PipelineRun, Job).
//
// Authority: BR-WE-014 (Kubernetes Job Execution Backend)
// Design: Strategy pattern dispatch based on spec.executionEngine
package executor

import (
	"context"
	"fmt"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// CreateOptions carries optional configuration for execution resource creation.
// Using a struct allows adding new fields without breaking the interface.
// DD-WE-006: Dependencies are passed here, queried from DS by the reconciler.
type CreateOptions struct {
	Dependencies *models.WorkflowDependencies
}

// Executor defines the interface for workflow execution backends.
// Both TektonExecutor and JobExecutor implement this interface.
//
// Lifecycle:
//  1. Create - Creates the execution resource (PipelineRun or Job) in the execution namespace
//  2. GetStatus - Polls the execution resource for current status
//  3. Cleanup - Deletes the execution resource during WFE deletion
type Executor interface {
	// Create builds and creates the execution resource (PipelineRun or Job)
	// in the specified execution namespace. Returns the name of the created resource.
	//
	// The execution resource name MUST be deterministic based on targetResource
	// to provide atomic resource locking (DD-WE-003).
	//
	// DD-WE-006: opts.Dependencies carries schema-declared infrastructure dependencies
	// to be mounted as volumes (Job) or workspace bindings (Tekton).
	Create(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string, opts CreateOptions) (string, error)

	// GetStatus retrieves the current status of the execution resource.
	// Returns an ExecutionResult that maps the backend-specific status to WFE phases.
	//
	// Returns nil result with nil error if the execution resource is not found
	// (may have been externally deleted - BR-WE-007).
	GetStatus(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) (*ExecutionResult, error)

	// Cleanup deletes the execution resource during WFE deletion (finalizer).
	// Returns nil if the resource doesn't exist (idempotent).
	Cleanup(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) error

	// Engine returns the execution engine identifier ("tekton" or "job")
	Engine() string
}

// ExecutionResult represents the mapped status of an execution resource.
// Both Tekton PipelineRun conditions and K8s Job conditions are mapped
// to this common structure.
type ExecutionResult struct {
	// Phase maps to WFE phase constants (Pending, Running, Completed, Failed)
	Phase string

	// Reason is a machine-readable reason for the current phase
	Reason string

	// Message is a human-readable description of the current state
	Message string

	// Summary contains the lightweight execution status for WFE status field
	Summary *workflowexecutionv1alpha1.ExecutionStatusSummary
}

// Registry maps execution engine names to Executor implementations.
// Used by the controller to dispatch to the correct executor.
type Registry struct {
	executors map[string]Executor
}

// NewRegistry creates a new executor registry.
func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[string]Executor),
	}
}

// Register adds an executor for the given engine name.
func (r *Registry) Register(engine string, executor Executor) {
	r.executors[engine] = executor
}

// Get returns the executor for the given engine name.
// Returns an error if no executor is registered for the engine.
func (r *Registry) Get(engine string) (Executor, error) {
	exec, ok := r.executors[engine]
	if !ok {
		return nil, fmt.Errorf("unsupported execution engine: %q (registered: %v)", engine, r.Engines())
	}
	return exec, nil
}

// Engines returns the list of registered engine names.
func (r *Registry) Engines() []string {
	engines := make([]string, 0, len(r.executors))
	for engine := range r.executors {
		engines = append(engines, engine)
	}
	return engines
}
