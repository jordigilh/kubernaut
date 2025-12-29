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

// Package phase provides phase constants and state machine logic for Notification service.
// Phase constants are exported from the API package (api/notification/v1alpha1)
// for external consumer usage.
//
// This package re-exports them for internal NT convenience and provides
// state machine logic (IsTerminal, CanTransition, Validate).
//
// Reference Implementation: pkg/remediationorchestrator/phase/types.go
// Pattern: Controller Refactoring Pattern Library - Terminal State Logic (P1)
package phase

import (
	"fmt"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Phase is an alias for the API-exported NotificationPhase type.
// This allows internal NT code to continue using `phase.Phase` without changes.
//
// 🏛️ Single Source of Truth: api/notification/v1alpha1/NotificationPhase
type Phase = notificationv1.NotificationPhase

// Re-export API constants for internal NT convenience.
// External consumers should import from api/notification/v1alpha1 directly.
const (
	// Pending - Initial state, waiting to start delivery
	Pending = notificationv1.NotificationPhasePending

	// Sending - Delivery in progress
	Sending = notificationv1.NotificationPhaseSending

	// Retrying - Partial failure with retries remaining (non-terminal state)
	// Some channels succeeded, some failed, but max attempts not exhausted yet
	Retrying = notificationv1.NotificationPhaseRetrying

	// Sent - All channels delivered successfully (terminal state)
	Sent = notificationv1.NotificationPhaseSent

	// PartiallySent - Some channels succeeded, some failed, all retries exhausted (terminal state)
	PartiallySent = notificationv1.NotificationPhasePartiallySent

	// Failed - All channels failed delivery (terminal state)
	Failed = notificationv1.NotificationPhaseFailed
)

// ========================================
// TERMINAL STATE LOGIC (P1 PATTERN)
// 📋 Refactoring: Controller Refactoring Pattern Library §2
// Reference: pkg/remediationorchestrator/phase/types.go lines 74-82
// ========================================
//
// IsTerminal returns true if the phase is a terminal state.
//
// Terminal phases are those where no further reconciliation is needed:
// - Sent: All deliveries succeeded
// - PartiallySent: Partial success, no more retries
// - Failed: All deliveries failed, no more retries
//
// Business Requirements:
// - BR-NOT-050: CRD Lifecycle Management
// - BR-NOT-053: At-Least-Once Delivery (terminal after exhaustion)
// - BR-NOT-054: Delivery Retry with Exponential Backoff
//
// This replaces 4 duplicated terminal state checks in the controller:
// 1. handleTerminalStateCheck() method (32 lines)
// 2. Reconcile() post-update check (lines 180-181)
// 3. Reconcile() post-re-read check (lines 195-196)
// 4. Phase transition validation
//
// Pattern Benefits:
// - ✅ Single source of truth for terminal states
// - ✅ Prevents inconsistency (missed PartiallySent in some checks)
// - ✅ Reduces controller by ~50 lines
// - ✅ Easy to maintain (add terminal phase once, applies everywhere)
// ========================================
func IsTerminal(p Phase) bool {
	switch p {
	case Sent, PartiallySent, Failed:
		return true
	default:
		return false
	}
}

// GetTerminalPhases returns all terminal phases for documentation/testing.
func GetTerminalPhases() []Phase {
	return []Phase{Sent, PartiallySent, Failed}
}

// ValidTransitions defines the state machine.
// Key: current phase, Value: list of valid target phases
//
// Business Requirements:
// - BR-NOT-050: CRD Lifecycle Management
// - BR-NOT-053: At-Least-Once Delivery
// - BR-NOT-054: Delivery Retry with Exponential Backoff
//
// State Machine:
// "" (initial) → Pending → Sending → {Sent, Retrying, PartiallySent, Failed}
//                                      ↓         ↓
//                                    (all OK)  (some failed, retries remain)
//                                              ↓
//                                         {Sent, PartiallySent, Failed}
//                                         ↓      ↓           ↓
//                                      (retry    (retries    (all failed)
//                                       success)  exhausted)
//
// Terminal states have no valid transitions (once terminal, always terminal)
var ValidTransitions = map[Phase][]Phase{
	"":       {Pending},                         // Initial phase transition on CRD creation
	Pending:  {Sending, Failed},                 // Can fail during initialization
	Sending:  {Sent, Retrying, PartiallySent, Failed}, // Initial delivery results
	Retrying: {Sent, Retrying, PartiallySent, Failed}, // Retry can succeed, keep retrying, or exhaust
	// Terminal states - no transitions allowed
	Sent:          {},
	PartiallySent: {},
	Failed:        {},
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
	case Pending, Sending, Retrying, Sent, PartiallySent, Failed:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}

