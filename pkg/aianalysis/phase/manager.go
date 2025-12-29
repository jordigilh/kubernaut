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
	"context"
	"fmt"

	"github.com/go-logr/logr"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Manager handles phase transitions with validation and audit logging.
//
// Pattern: P0 - Phase State Machine
// This manager ensures all phase transitions follow the ValidTransitions map.
type Manager struct {
	logger logr.Logger
}

// NewManager creates a new phase manager.
func NewManager(logger logr.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// TransitionResult holds the result of a phase transition attempt.
type TransitionResult struct {
	// Allowed indicates if the transition was valid per state machine
	Allowed bool
	// OldPhase is the phase before transition
	OldPhase Phase
	// NewPhase is the phase after transition
	NewPhase Phase
	// Error contains validation error if transition was not allowed
	Error error
}

// Transition validates and executes a phase transition.
//
// This method:
// 1. Validates the target phase is valid
// 2. Checks if transition is allowed per ValidTransitions
// 3. Updates the analysis status.phase
// 4. Logs the transition for audit trail
//
// Returns:
//   - TransitionResult: Details of the transition attempt
//
// Business Requirements:
//   - BR-AI-050: Phase transitions must be auditable
//   - BR-AI-060: Invalid transitions must be rejected
func (m *Manager) Transition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, target Phase) TransitionResult {
	oldPhase := Phase(analysis.Status.Phase)
	newPhase := target

	// Validate target phase
	if err := Validate(newPhase); err != nil {
		m.logger.Error(err, "Invalid target phase",
			"analysis", analysis.Name,
			"current_phase", oldPhase,
			"target_phase", target,
		)
		return TransitionResult{
			Allowed:  false,
			OldPhase: oldPhase,
			NewPhase: newPhase,
			Error:    fmt.Errorf("invalid target phase: %w", err),
		}
	}

	// Check if transition is allowed
	if !CanTransition(oldPhase, newPhase) {
		err := fmt.Errorf("invalid phase transition: %s → %s (not allowed per state machine)", oldPhase, newPhase)
		m.logger.Error(err, "Phase transition rejected",
			"analysis", analysis.Name,
			"current_phase", oldPhase,
			"target_phase", newPhase,
		)
		return TransitionResult{
			Allowed:  false,
			OldPhase: oldPhase,
			NewPhase: newPhase,
			Error:    err,
		}
	}

	// Execute transition
	analysis.Status.Phase = string(newPhase)

	m.logger.Info("Phase transition",
		"analysis", analysis.Name,
		"old_phase", oldPhase,
		"new_phase", newPhase,
		"is_terminal", IsTerminal(newPhase),
	)

	return TransitionResult{
		Allowed:  true,
		OldPhase: oldPhase,
		NewPhase: newPhase,
		Error:    nil,
	}
}

// GetCurrent returns the current phase of the analysis.
func (m *Manager) GetCurrent(analysis *aianalysisv1.AIAnalysis) Phase {
	return Phase(analysis.Status.Phase)
}

// IsInTerminalState checks if the analysis is in a terminal state.
//
// Terminal states require no further reconciliation.
//
// Pattern: P1 - Terminal State Logic
func (m *Manager) IsInTerminalState(analysis *aianalysisv1.AIAnalysis) bool {
	current := Phase(analysis.Status.Phase)
	return IsTerminal(current)
}

// GetNextPhases returns the list of valid next phases from the current phase.
//
// Useful for:
// - Determining available transitions
// - Test validation
// - UI state machines
func (m *Manager) GetNextPhases(analysis *aianalysisv1.AIAnalysis) []Phase {
	current := Phase(analysis.Status.Phase)
	validTargets, ok := ValidTransitions[current]
	if !ok {
		return []Phase{}
	}
	return validTargets
}

// Initialize sets the analysis to the initial Pending phase.
//
// This should be called when creating a new AIAnalysis CR.
func (m *Manager) Initialize(analysis *aianalysisv1.AIAnalysis) {
	if analysis.Status.Phase == "" {
		analysis.Status.Phase = string(Pending)
		m.logger.Info("Initialized phase",
			"analysis", analysis.Name,
			"phase", Pending,
		)
	}
}


