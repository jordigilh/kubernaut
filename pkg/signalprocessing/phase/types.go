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

// Package phase provides phase constants and state machine logic for SignalProcessing.
// Phase constants are exported from the API package (api/signalprocessing/v1alpha1)
// for external consumer usage per the Viceversa Pattern.
//
// This package re-exports them for internal SP convenience and provides
// state machine logic (IsTerminal, CanTransition, Validate).
//
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 1 (Phase State Machine)
package phase

import (
	"fmt"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Phase is an alias for the API-exported SignalProcessingPhase type.
// This allows internal SP code to continue using `phase.Phase` without changes.
//
// 🏛️ Single Source of Truth: api/signalprocessing/v1alpha1/SignalProcessingPhase
type Phase = signalprocessingv1alpha1.SignalProcessingPhase

// Re-export API constants for internal SP convenience.
// External consumers should import from api/signalprocessing/v1alpha1 directly.
const (
	// Pending is the initial state when SignalProcessing is created.
	Pending = signalprocessingv1alpha1.PhasePending

	// Enriching is when K8s context enrichment is in progress.
	// Business Requirements: BR-SP-001 (K8s Context), BR-SP-100 (Owner Chain)
	Enriching = signalprocessingv1alpha1.PhaseEnriching

	// Classifying is when environment/priority classification is in progress.
	// Business Requirements: BR-SP-051-053 (Environment), BR-SP-070-072 (Priority)
	Classifying = signalprocessingv1alpha1.PhaseClassifying

	// Categorizing is when business categorization is in progress.
	// Business Requirements: BR-SP-002, BR-SP-080, BR-SP-081
	Categorizing = signalprocessingv1alpha1.PhaseCategorizing

	// Completed is the terminal success state.
	Completed = signalprocessingv1alpha1.PhaseCompleted

	// Failed is the terminal error state.
	Failed = signalprocessingv1alpha1.PhaseFailed
)

// IsTerminal returns true if the phase is a terminal state.
// Terminal states prevent further reconciliation.
//
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 2 (Terminal State Logic)
func IsTerminal(p Phase) bool {
	switch p {
	case Completed, Failed:
		return true
	default:
		return false
	}
}

// ValidTransitions defines the state machine.
// Key: current phase, Value: list of valid target phases
//
// State Machine Flow:
//   Pending → Enriching → Classifying → Categorizing → Completed
//                  ↓            ↓             ↓
//                Failed       Failed        Failed
//
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 1 (Phase State Machine)
var ValidTransitions = map[Phase][]Phase{
	Pending:      {Enriching},
	Enriching:    {Classifying, Failed},
	Classifying:  {Categorizing, Failed},
	Categorizing: {Completed, Failed},
	// Terminal states - no transitions allowed
	Completed: {},
	Failed:    {},
}

// CanTransition checks if transition from current to target is valid.
// Returns true if the transition is allowed by the state machine, false otherwise.
//
// Example:
//   phase.CanTransition(phase.Pending, phase.Enriching)      // true
//   phase.CanTransition(phase.Pending, phase.Classifying)    // false (skip not allowed)
//   phase.CanTransition(phase.Completed, phase.Enriching)    // false (terminal state)
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
// Returns an error if the phase is not recognized.
func Validate(p Phase) error {
	switch p {
	case Pending, Enriching, Classifying, Categorizing, Completed, Failed:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}













