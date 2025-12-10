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

// Package phase provides phase constants and state machine logic for RO.
package phase

import "fmt"

// Phase represents the orchestration phase of a RemediationRequest
type Phase string

const (
	// Pending - Initial state, waiting to start
	Pending Phase = "Pending"

	// Processing - SignalProcessing CRD created, awaiting completion
	Processing Phase = "Processing"

	// Analyzing - AIAnalysis CRD created, awaiting completion
	Analyzing Phase = "Analyzing"

	// AwaitingApproval - Waiting for human approval (BR-ORCH-001)
	AwaitingApproval Phase = "AwaitingApproval"

	// Executing - WorkflowExecution CRD created, awaiting completion
	Executing Phase = "Executing"

	// Completed - All phases completed successfully (terminal state)
	Completed Phase = "Completed"

	// Failed - A phase failed (terminal state)
	Failed Phase = "Failed"

	// TimedOut - A phase exceeded timeout (terminal state)
	// Reference: BR-ORCH-027 (global), BR-ORCH-028 (per-phase)
	TimedOut Phase = "TimedOut"

	// Skipped - WorkflowExecution was skipped due to resource lock (terminal state)
	// Reference: BR-ORCH-032
	Skipped Phase = "Skipped"

	// Blocked - Remediation blocked due to consecutive failures (NON-terminal state)
	// New signals for same fingerprint will update deduplication but not create new RRs.
	// RO will transition to Failed after BlockedUntil cooldown expires.
	// Reference: BR-ORCH-042, DD-GATEWAY-011 v1.3
	Blocked Phase = "Blocked"
)

// IsTerminal returns true if the phase is a terminal state.
func IsTerminal(p Phase) bool {
	switch p {
	case Completed, Failed, TimedOut, Skipped:
		return true
	default:
		return false
	}
}

// ValidTransitions defines the state machine.
// Key: current phase, Value: list of valid target phases
var ValidTransitions = map[Phase][]Phase{
	Pending:          {Processing},
	Processing:       {Analyzing, Failed, TimedOut},
	Analyzing:        {AwaitingApproval, Executing, Completed, Failed, TimedOut}, // Completed for WorkflowNotNeeded (BR-ORCH-037)
	AwaitingApproval: {Executing, Failed, TimedOut},
	Executing:        {Completed, Failed, TimedOut, Skipped},
	// Blocked is NON-terminal: allows transition to Failed after cooldown (BR-ORCH-042)
	Blocked: {Failed},
	// Terminal states - no transitions allowed
	Completed: {},
	Failed:    {Blocked}, // Failed can transition to Blocked when consecutive failure threshold reached (BR-ORCH-042)
	TimedOut:  {},
	Skipped:   {},
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
	case Pending, Processing, Analyzing, AwaitingApproval, Executing, Completed, Failed, TimedOut, Skipped, Blocked:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}
