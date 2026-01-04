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

package phase

import (
	"fmt"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Manager implements phase state machine logic.
// Per Controller Refactoring Pattern Library (P0: Phase State Machine)
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md lines 180-221
type Manager struct{}

// NewManager creates a new phase manager.
func NewManager() *Manager {
	return &Manager{}
}

// CurrentPhase returns the current phase.
// Returns Pending if phase is empty (initial state).
func (m *Manager) CurrentPhase(obj *workflowexecutionv1alpha1.WorkflowExecution) Phase {
	if obj.Status.Phase == "" {
		return Pending
	}
	return Phase(obj.Status.Phase)
}

// TransitionTo transitions to the target phase.
// Returns an error if the transition is invalid per the state machine.
//
// Special case: If already in target phase, this is a no-op (returns nil).
// This handles cases where phase is empty ("") which maps to Pending.
//
// Example usage:
//
//	if err := phaseManager.TransitionTo(wfe, phase.Running); err != nil {
//	    logger.Error(err, "Invalid phase transition")
//	    return ctrl.Result{}, err
//	}
func (m *Manager) TransitionTo(obj *workflowexecutionv1alpha1.WorkflowExecution, target Phase) error {
	current := m.CurrentPhase(obj)

	// No-op if already in target phase (handles "" → Pending case)
	if current == target {
		obj.Status.Phase = string(target)
	obj.Status.ObservedGeneration = obj.Generation // DD-CONTROLLER-001
		return nil
	}

	if !CanTransition(current, target) {
		return fmt.Errorf("invalid phase transition from %s to %s", current, target)
	}

	obj.Status.ObservedGeneration = obj.Generation // DD-CONTROLLER-001
	obj.Status.Phase = string(target)
	return nil
}
