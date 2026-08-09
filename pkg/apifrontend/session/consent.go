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

package session

// DD-AF-011 (#1899): Phase-Transition Consent Guard.
//
// These constants define the harness-enforced consent gate's session.State
// backchannel: a structural signal for which interaction mode the LLM
// declared on kubernaut_investigate, and which phase transitions the
// harness has gated pending genuine user confirmation. They live in this
// package (rather than pkg/apifrontend/agent, where the tool-filter and
// phase-guard callbacks that read/write them are implemented) because
// pkg/apifrontend/session/reinvoke.go's NeedsReinvocationCtx also needs
// them, and both pkg/apifrontend/agent and pkg/apifrontend/launcher already
// import this package -- avoiding an import cycle.
const (
	// StateKeyDriverActive records whether an interactive driver session
	// (a successful kubernaut_investigate or kubernaut_reconnect) is active.
	// Mirrors pkg/apifrontend/agent/phase_guard.go's pre-existing
	// af_interactive_driver_active key.
	StateKeyDriverActive = "af_interactive_driver_active"

	// StateKeyActiveRRID records the RemediationRequest ID established by
	// the active driver session. Mirrors phase_guard.go's pre-existing
	// af_active_rr_id key.
	StateKeyActiveRRID = "af_active_rr_id"

	// StateKeyInteractionMode records the interaction_mode declared on the
	// most recent successful kubernaut_investigate call.
	StateKeyInteractionMode = "af_interaction_mode"

	// StateKeyPhase2Blocked is set true after a successful
	// kubernaut_investigate when the declared (or defaulted) interaction
	// mode requires waiting for genuine user confirmation before
	// kubernaut_discover_workflows may run.
	StateKeyPhase2Blocked = "af_phase2_blocked"

	// StateKeyPhase3Blocked is set true after a successful
	// kubernaut_discover_workflows when the declared interaction mode
	// requires waiting for genuine user confirmation before
	// kubernaut_select_workflow may run.
	StateKeyPhase3Blocked = "af_phase3_blocked"

	// StateKeyGroundedContentAvailable records whether the most recent
	// kubernaut_investigate call produced real, groundable RCA content
	// (#2023). phase_guard.go's harness-enforced grounding guard reads this
	// immediately before kubernaut_present_decision executes: when false --
	// including the fail-safe default when this key was never set -- the
	// model's summary/rca/options are overwritten with a fixed, honest
	// "no data" payload instead of trusted as-is, preventing a fabricated
	// narrative from ever reaching the audit trail. present_decision itself
	// still runs afterward, so the AU-3 structured-artifact mandate (#1408)
	// is preserved; only a fabricated narrative is blocked, never the
	// artifact.
	StateKeyGroundedContentAvailable = "af_grounded_content_available"
)

// Interaction mode values for InteractionMode / StateKeyInteractionMode.
//
// InteractionModeInteractive is the fail-safe default (AC-6 least
// privilege): every phase transition waits for genuine user confirmation.
// Any unrecognized/invalid value provided by the LLM must resolve to this
// mode, never to a more autonomous one (SI-10).
const (
	InteractionModeInteractive               = "interactive"
	InteractionModeFullRemediation           = "full_remediation"
	InteractionModeFullRemediationAutonomous = "full_remediation_autonomous"
)

// ValidInteractionMode reports whether mode is one of the recognized
// InteractionMode* values. Callers must fail safe to
// InteractionModeInteractive when this returns false.
func ValidInteractionMode(mode string) bool {
	switch mode {
	case InteractionModeInteractive, InteractionModeFullRemediation, InteractionModeFullRemediationAutonomous:
		return true
	default:
		return false
	}
}
