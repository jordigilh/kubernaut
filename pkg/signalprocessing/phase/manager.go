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

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager manages phase transitions with validation.
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 1 (Phase State Machine)
//
// TODO: Complete phase manager implementation (Phase 2 refactoring)
// - Implement TransitionTo with state machine validation
// - Add atomic status update integration
// - Add phase transition audit event recording
// - Update controller to use Manager.TransitionTo instead of direct status updates
// - Add comprehensive unit tests for all phase transitions
//
// Estimated effort: 1-2 days
// Expected benefits: Centralized phase transition logic, runtime validation, audit integration
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManager creates a new phase manager.
func NewManager(client client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: client,
		scheme: scheme,
	}
}

// CurrentPhase returns the current phase of the SignalProcessing.
func (m *Manager) CurrentPhase(sp *signalprocessingv1alpha1.SignalProcessing) Phase {
	return Phase(sp.Status.Phase)
}

// TransitionTo transitions the SignalProcessing to a new phase with validation.
// Returns an error if the transition is invalid or the status update fails.
//
// TODO: Complete implementation
// - Validate transition using CanTransition()
// - Update status.Phase atomically
// - Record phase transition audit event
// - Update lastTransitionTime
// - Set appropriate conditions
//
// Example usage:
//   err := m.TransitionTo(ctx, sp, phase.Enriching, "EnrichmentStarted", "K8s context enrichment initiated")
func (m *Manager) TransitionTo(
	ctx context.Context,
	sp *signalprocessingv1alpha1.SignalProcessing,
	targetPhase Phase,
	reason, message string,
) error {
	currentPhase := m.CurrentPhase(sp)

	// Validate transition
	if !CanTransition(currentPhase, targetPhase) {
		return fmt.Errorf("invalid phase transition from %s to %s", currentPhase, targetPhase)
	}

	// TODO: Implement atomic status update with phase transition
	// TODO: Record phase transition audit event
	// TODO: Update conditions with reason and message

	return fmt.Errorf("phase manager not fully implemented yet - use StatusManager.AtomicStatusUpdate for now")
}













