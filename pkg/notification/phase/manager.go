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

// ========================================
// PHASE STATE MACHINE MANAGER (P0 PATTERN)
// 📋 Controller Refactoring Pattern: Phase State Machine
// Reference: pkg/remediationorchestrator/phase/manager.go
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================
//
// This manager provides a clean abstraction over phase transitions,
// ensuring all transitions are validated through the ValidTransitions map.
//
// BENEFITS:
// - ✅ Single source of truth for state machine logic
// - ✅ Compile-time safety (invalid transitions caught immediately)
// - ✅ Self-documenting (ValidTransitions map shows all valid flows)
// - ✅ Easier testing (mock-friendly interface)
// - ✅ Consistent error messages for invalid transitions
//
// Business Requirements:
// - BR-NOT-050: CRD Lifecycle Management
// - BR-NOT-053: At-Least-Once Delivery
// - BR-NOT-054: Delivery Retry with Exponential Backoff
// ========================================

package phase

import (
	"fmt"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Manager implements phase state machine logic for NotificationRequest.
//
// This manager provides:
// - CurrentPhase(): Get current phase with Pending fallback for initial state
// - TransitionTo(): Validate and perform phase transition
// - IsInTerminalState(): Check if CRD has reached terminal phase
//
// Usage Example:
//
//	phaseMgr := phase.NewManager()
//	currentPhase := phaseMgr.CurrentPhase(notification)
//	if err := phaseMgr.TransitionTo(notification, phase.Sending); err != nil {
//	    return fmt.Errorf("phase transition failed: %w", err)
//	}
type Manager struct{}

// NewManager creates a new phase manager.
//
// This manager is stateless, so a single instance can be reused
// across multiple NotificationRequest reconciliations.
func NewManager() *Manager {
	return &Manager{}
}

// CurrentPhase returns the current phase of a NotificationRequest.
// Returns Pending if Status.Phase is empty (initial state).
//
// This ensures we always have a valid phase to work with, even on
// newly created CRDs where Status.Phase hasn't been set yet.
func (m *Manager) CurrentPhase(notification *notificationv1.NotificationRequest) Phase {
	if notification.Status.Phase == "" {
		return Pending
	}
	return Phase(notification.Status.Phase)
}

// TransitionTo transitions a NotificationRequest to the target phase.
// Returns an error if the transition is invalid per the state machine.
//
// This method:
// 1. Gets current phase (with Pending fallback)
// 2. Validates transition using CanTransition()
// 3. Updates Status.Phase if valid
// 4. Returns error if invalid
//
// NOTE: This method only updates the in-memory object. The caller is
// responsible for persisting the change via client.Status().Update().
//
// Usage Example:
//
//	if err := phaseMgr.TransitionTo(notification, phase.Sending); err != nil {
//	    return fmt.Errorf("invalid transition: %w", err)
//	}
//	// Phase is updated in memory, now persist:
//	if err := r.Status().Update(ctx, notification); err != nil {
//	    return err
//	}
func (m *Manager) TransitionTo(notification *notificationv1.NotificationRequest, target Phase) error {
	current := m.CurrentPhase(notification)

	if !CanTransition(current, target) {
		return fmt.Errorf("invalid phase transition from %s to %s (see ValidTransitions map in pkg/notification/phase/types.go)", current, target)
	}

	notification.Status.Phase = notificationv1.NotificationPhase(target)
	return nil
}

// IsInTerminalState checks if the NotificationRequest has reached a terminal phase.
// Terminal phases are: Sent, PartiallySent, Failed
//
// This is a convenience method that combines CurrentPhase() and IsTerminal().
//
// Usage Example:
//
//	if phaseMgr.IsInTerminalState(notification) {
//	    return nil // No further reconciliation needed
//	}
func (m *Manager) IsInTerminalState(notification *notificationv1.NotificationRequest) bool {
	return IsTerminal(m.CurrentPhase(notification))
}













