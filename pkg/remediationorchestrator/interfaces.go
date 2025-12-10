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

package remediationorchestrator

import (
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// PhaseManager manages phase transitions and validation for RemediationRequest.
//
// Business Requirements:
// - BR-ORCH-025: Phase state transitions must follow defined rules
// - BR-ORCH-026: Terminal states must be correctly identified
type PhaseManager interface {
	// CurrentPhase returns the current phase of the remediation.
	// Returns phase.Pending if OverallPhase is empty.
	CurrentPhase(rr *remediationv1.RemediationRequest) phase.Phase

	// TransitionTo transitions to the target phase with validation.
	// Returns an error if the transition is invalid.
	TransitionTo(rr *remediationv1.RemediationRequest, target phase.Phase) error
}


