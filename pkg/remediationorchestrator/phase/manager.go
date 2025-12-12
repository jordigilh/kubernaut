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

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Manager implements phase state machine logic for RemediationRequest.
//
// Business Requirements:
// - BR-ORCH-025: Phase state transitions
// - BR-ORCH-026: Terminal state identification
type Manager struct{}

// NewManager creates a new phase manager.
func NewManager() *Manager {
	return &Manager{}
}

// CurrentPhase returns the current phase of a RemediationRequest.
// Returns Pending if OverallPhase is empty (initial state).
func (m *Manager) CurrentPhase(rr *remediationv1.RemediationRequest) Phase {
	if rr.Status.OverallPhase == "" {
		return Pending
	}
	return Phase(rr.Status.OverallPhase)
}

// TransitionTo transitions a RemediationRequest to the target phase.
// Returns an error if the transition is invalid per the state machine.
func (m *Manager) TransitionTo(rr *remediationv1.RemediationRequest, target Phase) error {
	current := m.CurrentPhase(rr)

	if !CanTransition(current, target) {
		return fmt.Errorf("invalid phase transition from %s to %s", current, target)
	}

	rr.Status.OverallPhase = target
	return nil
}
