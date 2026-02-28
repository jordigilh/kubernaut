/*
Copyright 2026 Jordi Gil.

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

// Package phase provides phase constants and state machine logic for the Effectiveness Monitor.
// Phase constants are re-exported from the API package (api/effectivenessassessment/v1alpha1)
// for internal EM convenience and provides state machine logic (IsTerminal, CanTransition, Validate).
//
// Per Viceversa Pattern: API defines constants, this package re-exports + adds state machine.
//
// Business Requirements:
// - BR-EM-005: Phase state transitions (Pending -> Stabilizing -> Assessing -> Completed/Failed)
package phase

import (
	"fmt"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// Phase is an alias for the API-exported phase type.
// This allows internal EM code to use `phase.Phase` for convenience.
//
// Single Source of Truth: api/effectivenessassessment/v1alpha1
type Phase = string

// Re-export API constants for internal EM convenience.
// External consumers should import from api/effectivenessassessment/v1alpha1 directly.
const (
	// Pending - EA created by RO, EM has not reconciled yet.
	Pending = eav1.PhasePending
	// Stabilizing - EM is waiting for stabilization window to elapse.
	Stabilizing = eav1.PhaseStabilizing
	// Assessing - EM is actively performing assessment checks.
	Assessing = eav1.PhaseAssessing
	// Completed - All assessment checks finished (or validity expired).
	Completed = eav1.PhaseCompleted
	// Failed - Assessment could not be performed.
	Failed = eav1.PhaseFailed
)

// ValidTransitions defines the state machine for EA phases.
// Key: current phase, Value: list of valid target phases.
var ValidTransitions = map[Phase][]Phase{
	Pending:     {Stabilizing, Assessing, Failed}, // Stabilizing when StabilizationWindow > 0; Assessing directly when 0
	Stabilizing: {Assessing, Failed},
	Assessing:   {Completed, Failed},
	// Terminal states - no transitions allowed.
	Completed: {},
	Failed:    {},
}

// IsTerminal returns true if the phase is a terminal state.
func IsTerminal(p Phase) bool {
	switch p {
	case Completed, Failed:
		return true
	default:
		return false
	}
}

// CanTransition checks if transition from current to target is valid.
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
func Validate(p Phase) error {
	switch p {
	case Pending, Stabilizing, Assessing, Completed, Failed:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}
