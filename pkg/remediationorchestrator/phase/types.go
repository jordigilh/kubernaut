// Package phase provides phase management for the Remediation Orchestrator.
// It defines the phase state machine and transition rules.
//
// Business Requirements:
// - BR-ORCH-025: Core orchestration phases
// - BR-ORCH-026: Approval orchestration
// - BR-ORCH-027, BR-ORCH-028: Timeout handling
// - BR-ORCH-032: Skipped phase for resource lock deduplication
package phase

import (
	"fmt"
)

// Phase represents the orchestration phase of a RemediationRequest
type Phase string

const (
	// Pending - Initial state, waiting to start
	Pending Phase = "Pending"

	// Processing - SignalProcessing CRD created, awaiting completion
	Processing Phase = "Processing"

	// Analyzing - AIAnalysis CRD created, awaiting completion
	Analyzing Phase = "Analyzing"

	// AwaitingApproval - Waiting for human approval (BR-ORCH-001, BR-ORCH-026)
	AwaitingApproval Phase = "AwaitingApproval"

	// Executing - WorkflowExecution CRD created, awaiting completion
	Executing Phase = "Executing"

	// Completed - All phases completed successfully
	Completed Phase = "Completed"

	// Failed - A phase failed
	Failed Phase = "Failed"

	// TimedOut - A phase exceeded timeout (BR-ORCH-027, BR-ORCH-028)
	TimedOut Phase = "TimedOut"

	// Skipped - WorkflowExecution was skipped due to resource lock (BR-ORCH-032)
	Skipped Phase = "Skipped"
)

// ValidTransitions defines the state machine for phase transitions.
// Key: current phase, Value: list of valid target phases
var ValidTransitions = map[Phase][]Phase{
	Pending:          {Processing},
	Processing:       {Analyzing, Failed, TimedOut},
	Analyzing:        {AwaitingApproval, Executing, Failed, TimedOut},
	AwaitingApproval: {Executing, Failed, TimedOut},
	Executing:        {Completed, Failed, TimedOut, Skipped},
	// Terminal states - no transitions allowed
	Completed: {},
	Failed:    {},
	TimedOut:  {},
	Skipped:   {},
}

// terminalPhases is a set of terminal phases for O(1) lookup
var terminalPhases = map[Phase]struct{}{
	Completed: {},
	Failed:    {},
	TimedOut:  {},
	Skipped:   {},
}

// validPhases is a set of all valid phases for O(1) validation
var validPhases = map[Phase]struct{}{
	Pending:          {},
	Processing:       {},
	Analyzing:        {},
	AwaitingApproval: {},
	Executing:        {},
	Completed:        {},
	Failed:           {},
	TimedOut:         {},
	Skipped:          {},
}

// IsTerminal returns true if the phase is a terminal state.
// Terminal states: Completed, Failed, TimedOut, Skipped
func IsTerminal(p Phase) bool {
	_, ok := terminalPhases[p]
	return ok
}

// CanTransition checks if transition from current to target phase is valid.
// Returns false if the current phase is terminal or if the target is not
// in the list of valid transitions from the current phase.
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
	if _, ok := validPhases[p]; ok {
		return nil
	}
	return fmt.Errorf("invalid phase: %s", p)
}

// String returns the string representation of the phase
func (p Phase) String() string {
	return string(p)
}

// AllPhases returns all valid phases
func AllPhases() []Phase {
	return []Phase{
		Pending,
		Processing,
		Analyzing,
		AwaitingApproval,
		Executing,
		Completed,
		Failed,
		TimedOut,
		Skipped,
	}
}

// TerminalPhases returns all terminal phases
func TerminalPhases() []Phase {
	return []Phase{
		Completed,
		Failed,
		TimedOut,
		Skipped,
	}
}

