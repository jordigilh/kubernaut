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

// Package phase provides phase constants and state machine logic for AIAnalysis.
//
// This package implements the Phase State Machine pattern from the Controller
// Refactoring Pattern Library, providing:
// - Type-safe phase constants
// - ValidTransitions map for state machine logic
// - IsTerminal() function for terminal state checks
// - CanTransition() for transition validation
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// Pattern: P0 - Phase State Machine, P1 - Terminal State Logic
package phase

import (
	"fmt"
)

// Phase represents an AIAnalysis reconciliation phase.
// Phases follow a strict state machine with defined transitions.
type Phase string

// Phase constants define the AIAnalysis reconciliation lifecycle.
const (
	// Pending - Initial state, waiting to start investigation
	Pending Phase = "Pending"

	// Investigating - HolmesGPT API investigation in progress
	// Business Requirement: BR-AI-010 (Investigation phase)
	Investigating Phase = "Investigating"

	// Analyzing - Rego policy evaluation and approval decision in progress
	// Business Requirement: BR-AI-030 (Analysis phase)
	Analyzing Phase = "Analyzing"

	// Completed - Analysis completed successfully (terminal state)
	// Business Requirement: BR-AI-050 (Completion)
	Completed Phase = "Completed"

	// Failed - Analysis failed permanently (terminal state)
	// Business Requirement: BR-AI-060 (Error handling)
	Failed Phase = "Failed"
)

// IsTerminal returns true if the phase is a terminal state.
//
// Terminal states:
// - Completed: Analysis finished successfully
// - Failed: Analysis failed permanently
//
// Non-terminal states require further reconciliation.
//
// Pattern: P1 - Terminal State Logic
func IsTerminal(p Phase) bool {
	switch p {
	case Completed, Failed:
		return true
	default:
		return false
	}
}

// ValidTransitions defines the AIAnalysis state machine.
// Key: current phase, Value: list of valid target phases
//
// State Machine:
//   Pending → Investigating
//   Investigating → Analyzing, Failed
//   Analyzing → Completed, Failed
//   Completed → (terminal, no transitions)
//   Failed → (terminal, no transitions)
//
// Pattern: P0 - Phase State Machine
var ValidTransitions = map[Phase][]Phase{
	// Initial state transitions
	Pending: {Investigating},

	// Investigation can proceed to analysis or fail
	Investigating: {Analyzing, Failed},

	// Analysis can complete successfully or fail
	Analyzing: {Completed, Failed},

	// Terminal states - no transitions allowed
	Completed: {},
	Failed:    {},
}

// CanTransition checks if transition from current to target phase is valid.
//
// Returns:
//   - true: Transition is allowed per state machine
//   - false: Transition violates state machine rules
//
// Example:
//   CanTransition(Pending, Investigating)  // true
//   CanTransition(Investigating, Completed) // false (must go through Analyzing)
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
//
// Returns:
//   - nil: Phase is valid
//   - error: Phase is not recognized
func Validate(p Phase) error {
	switch p {
	case Pending, Investigating, Analyzing, Completed, Failed:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}

// AllPhases returns all valid phase values.
// Useful for testing and validation.
func AllPhases() []Phase {
	return []Phase{
		Pending,
		Investigating,
		Analyzing,
		Completed,
		Failed,
	}
}

// String returns the string representation of the phase.
func (p Phase) String() string {
	return string(p)
}


