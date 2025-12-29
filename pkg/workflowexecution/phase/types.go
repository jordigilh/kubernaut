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

// Package phase provides phase constants and state machine logic for WorkflowExecution.
// Phase constants are re-exported from the API package (api/workflowexecution/v1alpha1)
// following the Viceversa Pattern from RemediationOrchestrator.
//
// This package provides:
// - Phase type alias for internal convenience
// - Re-exported phase constants
// - IsTerminal() function (P1 - Terminal State Logic pattern)
// - State machine logic with ValidTransitions map (P0 - Phase State Machine pattern)
//
// Per Controller Refactoring Pattern Library:
// - P1: Terminal State Logic - Prevents unnecessary reconciliation
// - P0: Phase State Machine - Prevents invalid transitions
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
package phase

import (
	"fmt"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Phase is an alias for the string type used in the API.
// This allows internal WE code to use `phase.Phase` for type safety.
//
// 🏛️ Single Source of Truth: api/workflowexecution/v1alpha1 phase constants
type Phase string

// Re-export API constants for internal WE convenience.
// External consumers should use api/workflowexecution/v1alpha1 constants directly.
const (
	// Pending - Initial state when WorkflowExecution is first created
	Pending Phase = workflowexecutionv1alpha1.PhasePending

	// Running - PipelineRun is actively executing
	Running Phase = workflowexecutionv1alpha1.PhaseRunning

	// Completed - Workflow executed successfully (terminal state)
	Completed Phase = workflowexecutionv1alpha1.PhaseCompleted

	// Failed - Workflow execution failed (terminal state)
	Failed Phase = workflowexecutionv1alpha1.PhaseFailed
)

// ========================================
// P1: Terminal State Logic Pattern
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================

// IsTerminal returns true if the phase is a terminal state.
// Terminal states require no further reconciliation.
//
// Terminal phases:
// - Completed: Workflow executed successfully
// - Failed: Workflow execution failed
//
// Non-terminal phases:
// - Pending: Waiting for PipelineRun creation
// - Running: PipelineRun actively executing
func IsTerminal(p Phase) bool {
	switch p {
	case Completed, Failed:
		return true
	default:
		return false
	}
}

// ========================================
// P0: Phase State Machine Pattern
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================

// ValidTransitions defines the state machine for WorkflowExecution phases.
// Key: current phase, Value: list of valid target phases
//
// Workflow Execution Flow:
// 1. Pending → Running (PipelineRun created)
// 2. Running → Completed (PipelineRun succeeded)
// 3. Running → Failed (PipelineRun failed or timed out)
//
// Terminal states (Completed, Failed) have no valid transitions.
var ValidTransitions = map[Phase][]Phase{
	Pending: {Running, Failed}, // Can transition to Running or fail immediately (e.g., PipelineRun creation error)
	Running: {Completed, Failed}, // Can complete successfully or fail
	// Terminal states - no transitions allowed
	Completed: {},
	Failed:    {},
}

// CanTransition checks if transition from current phase to target phase is valid.
// Returns true if the transition is allowed by the state machine.
//
// Example:
//
//	if phase.CanTransition(phase.Pending, phase.Running) {
//	    // Valid transition
//	}
func CanTransition(current, target Phase) bool {
	validTargets, ok := ValidTransitions[current]
	if !ok {
		return false
	}
	for _, v := range validTargets {
		if v == target {
			return true
		}
	}
	return false
}

// Validate checks if a phase value is valid.
// Returns an error if the phase is not one of the defined constants.
func Validate(p Phase) error {
	switch p {
	case Pending, Running, Completed, Failed:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}

