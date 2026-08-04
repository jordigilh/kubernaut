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

package agent

import (
	"github.com/go-logr/logr"
	"google.golang.org/adk/agent/llmagent"
	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

const (
	// stateKeyDriverActive/stateKeyActiveRRID mirror
	// session.StateKeyDriverActive/session.StateKeyActiveRRID -- kept as
	// local aliases (rather than a broad rename) to minimize churn in this
	// file; both packages read/write the identical state keys.
	stateKeyDriverActive  = session.StateKeyDriverActive
	stateKeyActiveRRID    = session.StateKeyActiveRRID
	stateKeyActiveSession = "af_active_session_id"
)

const errNoActiveDriver = "interactive session not active — you must call kubernaut_investigate first to establish a driver session before using this tool"

// errCheckpointBlocked is returned by phaseGuardBefore's DD-AF-011 (#1899)
// backstop layer when a phase-gated tool is attempted while its checkpoint
// flag is still set. checkpointToolFilter (a BeforeModelCallback) is the
// primary layer that keeps the model from even seeing the tool as an
// option; this is defense-in-depth for the rare case a call slips through.
const errCheckpointBlocked = "this action requires explicit user confirmation first -- wait for the user's next message before proceeding"

// checkpointGatedTools maps each phase-gated tool to the checkpoint flag
// that, when set, must hard-reject it (DD-AF-011, #1899).
var checkpointGatedTools = map[string]string{
	"kubernaut_discover_workflows": session.StateKeyPhase2Blocked,
	"kubernaut_select_workflow":    session.StateKeyPhase3Blocked,
}

// mcpDependentTools are tools that require an active interactive driver session
// (i.e., a successful kubernaut_investigate) before they can be called. Without
// this prerequisite, KA rejects them with not_driving errors.
var mcpDependentTools = map[string]bool{
	"kubernaut_discover_workflows": true,
	"kubernaut_select_workflow":    true,
	"kubernaut_message":            true,
	"kubernaut_complete":           true,
	"kubernaut_cancel":             true,
	"kubernaut_status":             true,
}

// driverEntryTools are tools that establish the interactive driver session.
// After a successful call to one of these, mcpDependentTools are unblocked.
// kubernaut_investigate is included because it handles both fresh investigations
// and takeover of autonomous sessions (consolidated per #1332).
var driverEntryTools = map[string]bool{
	"kubernaut_investigate": true,
	"kubernaut_reconnect":   true,
}

// sessionTerminalTools end the active investigation session.
// A successful call clears the ActiveContextRegistry entry (BR-SESS-022).
var sessionTerminalTools = map[string]bool{
	"kubernaut_complete": true,
	"kubernaut_cancel":   true,
}

// newPhaseGuard returns a BeforeToolCallback that blocks MCP-dependent tools
// unless a successful takeover/reconnect has been recorded in session state,
// and an AfterToolCallback that records successful takeover/reconnect.
// When registry is non-nil, the after-callback also manages the
// ActiveContextRegistry for multi-turn session continuity (BR-SESS-020).
func newPhaseGuard(registry *launcher.ActiveContextRegistry) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	before := func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
		if !mcpDependentTools[t.Name()] {
			return nil, nil
		}

		logger := logr.FromContextOrDiscard(ctx)
		state := ctx.State()
		if state == nil {
			logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
			return map[string]any{"error": errNoActiveDriver}, nil
		}

		active, err := state.Get(stateKeyDriverActive)
		if err != nil || active == nil {
			logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
			return map[string]any{"error": errNoActiveDriver}, nil
		}
		if b, ok := active.(bool); !ok || !b {
			logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
			return map[string]any{"error": errNoActiveDriver}, nil
		}

		if args != nil {
			if rrID, _ := args["rr_id"].(string); rrID == "" {
				if storedRRID, sErr := state.Get(stateKeyActiveRRID); sErr == nil {
					if s, ok := storedRRID.(string); ok && s != "" {
						args["rr_id"] = s
						logger.Info("phase-guard injected rr_id from state",
							"tool", t.Name(), "rr_id", s)
					}
				}
			}
		}

		// DD-AF-011 (#1899) backstop layer: hard-reject phase-gated tools
		// whose checkpoint flag is still set, even though a driver is
		// active. checkpointToolFilter (BeforeModelCallback) is the primary
		// layer that should already keep these tools out of the model's
		// tool list; this defends against a call slipping through.
		if flagKey, gated := checkpointGatedTools[t.Name()]; gated {
			if blocked, _ := state.Get(flagKey); blocked == true {
				logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "checkpoint_blocked")
				return map[string]any{"error": errCheckpointBlocked}, nil
			}
		}

		return nil, nil
	}

	after := func(ctx tool.Context, t tool.Tool, inputArgs map[string]any, resp map[string]any, callErr error) (map[string]any, error) {
		toolName := t.Name()
		isEntry := driverEntryTools[toolName]
		isTerminal := sessionTerminalTools[toolName]
		// DD-AF-011 (#1899): discover_workflows is neither a driver-entry nor
		// a session-terminal tool, but its success still needs to persist
		// the phase-3 checkpoint flag.
		isDiscoverWorkflows := toolName == "kubernaut_discover_workflows"

		isSuccess := callErr == nil && resp != nil
		if isSuccess {
			if errVal, ok := resp["error"]; ok && errVal != nil {
				isSuccess = false
			}
		}

		// Refresh idle timer for any successful tool call to keep the
		// active session alive during ongoing engagement (#1446, AU-3).
		if registry != nil && isSuccess && !isEntry && !isTerminal {
			if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
				registry.Refresh(identity.Username)
			}
		}

		if !isEntry && !isTerminal && !isDiscoverWorkflows {
			return resp, callErr
		}
		if !isSuccess {
			return resp, callErr
		}

		logger := logr.FromContextOrDiscard(ctx)
		state := ctx.State()

		if isDiscoverWorkflows {
			if state != nil {
				mode := interactionModeFromState(state)
				blocked := mode != session.InteractionModeFullRemediationAutonomous
				if err := state.Set(session.StateKeyPhase3Blocked, blocked); err != nil {
					logger.Error(err, "phase-guard failed to persist phase3_blocked state")
				}
			}
			return resp, callErr
		}

		if isEntry {
			if state != nil {
				if err := state.Set(stateKeyDriverActive, true); err != nil {
					logger.Error(err, "phase-guard failed to set driver state")
				}

				// Prefer rr_id from response (kubernaut_investigate returns it).
				// Fall back to input args (kubernaut_reconnect takes it as input
				// but does not echo it in the response).
				if rrID, ok := resp["rr_id"].(string); ok && rrID != "" {
					if err := state.Set(stateKeyActiveRRID, rrID); err != nil {
						logger.Error(err, "phase-guard failed to store rr_id in state")
					}
				} else if inputArgs != nil {
					if rrID, ok := inputArgs["rr_id"].(string); ok && rrID != "" {
						if err := state.Set(stateKeyActiveRRID, rrID); err != nil {
							logger.Error(err, "phase-guard failed to store rr_id from input args")
						}
					}
				}

				if sessionID, ok := resp["session_id"].(string); ok && sessionID != "" {
					if err := state.Set(stateKeyActiveSession, sessionID); err != nil {
						logger.Error(err, "phase-guard failed to store session_id in state")
					}
				}

				// DD-AF-011 (#1899): only kubernaut_investigate carries the
				// interaction_mode signal (kubernaut_reconnect resumes an
				// already-established session and leaves any prior mode/
				// checkpoint state untouched).
				if toolName == "kubernaut_investigate" {
					mode := session.InteractionModeInteractive
					if inputArgs != nil {
						if raw, ok := inputArgs["interaction_mode"].(string); ok && session.ValidInteractionMode(raw) {
							mode = raw
						}
					}
					if err := state.Set(session.StateKeyInteractionMode, mode); err != nil {
						logger.Error(err, "phase-guard failed to persist interaction mode")
					}
					blocked := mode == session.InteractionModeInteractive

					// #1918: harness-enforced actionability gate. Independent
					// of the model's own reading of the RCA narrative, force
					// phase2_blocked=true when KA's structured signal says
					// the RCA concluded no remediation is warranted (the
					// same is_actionable=false && no-workflow condition
					// investigator.go's own internal short-circuit guard
					// already treats as authoritative). This only ever
					// tightens an autonomy grant already made by the model
					// (full_remediation/full_remediation_autonomous) -- it
					// never loosens interactive mode's own blocked default,
					// and never overrides a genuinely actionable RCA.
					if rcaConcludedNotActionable(resp) {
						blocked = true
						logger.Info("phase-guard forcing phase2_blocked: RCA concluded not actionable with no workflow",
							"tool", toolName, "declared_mode", mode)
					}

					if err := state.Set(session.StateKeyPhase2Blocked, blocked); err != nil {
						logger.Error(err, "phase-guard failed to persist phase2_blocked state")
					}
				}
			}
		}

		// #1912: a session-terminal tool ending the driver session must clear
		// driverActive alongside the registry entry below. Leaving it true
		// left NeedsReinvocationCtx (reinvoke.go) permanently able to treat a
		// finished session as still driving, incorrectly nudging reinvocation
		// on any later text-only turn in the same chat session.
		if isTerminal && state != nil {
			if err := state.Set(stateKeyDriverActive, false); err != nil {
				logger.Error(err, "phase-guard failed to clear driver state")
			}
			if err := state.Set(stateKeyActiveRRID, ""); err != nil {
				logger.Error(err, "phase-guard failed to clear active rr_id state")
			}
			if err := state.Set(stateKeyActiveSession, ""); err != nil {
				logger.Error(err, "phase-guard failed to clear active session state")
			}
		}

		if registry != nil {
			if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
				if isEntry {
					registry.Set(identity.Username, ctx.SessionID())
				} else if isTerminal {
					registry.Clear(identity.Username)
				}
			}
		}

		return resp, callErr
	}

	return before, after
}

// interactionModeFromState reads the DD-AF-011 (#1899) interaction mode
// persisted by a prior kubernaut_investigate success, failing safe to
// InteractionModeInteractive when unset or invalid (AC-6, SI-10).
func interactionModeFromState(state adksession.State) string {
	if state == nil {
		return session.InteractionModeInteractive
	}
	v, err := state.Get(session.StateKeyInteractionMode)
	if err != nil {
		return session.InteractionModeInteractive
	}
	s, ok := v.(string)
	if !ok || !session.ValidInteractionMode(s) {
		return session.InteractionModeInteractive
	}
	return s
}

// NewPhaseGuardForTest exports the phase guard without registry for unit testing.
func NewPhaseGuardForTest() (
	func(tool.Context, tool.Tool, map[string]any) (map[string]any, error),
	func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error),
) {
	return newPhaseGuard(nil)
}

// NewPhaseGuardWithRegistryForTest exports the phase guard with registry for
// session continuity integration testing (BR-SESS-020).
func NewPhaseGuardWithRegistryForTest(registry *launcher.ActiveContextRegistry) (
	func(tool.Context, tool.Tool, map[string]any) (map[string]any, error),
	func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error),
) {
	return newPhaseGuard(registry)
}

// rcaConcludedNotActionable inspects a kubernaut_investigate response's
// nested "rca" payload (tools.InvestigateRCA, marshaled to map[string]any by
// the ADK function-tool framework) for the #1918 harness-enforced
// actionability gate. It returns true only when KA's RCA explicitly computed
// is_actionable=false AND no workflow was already identified -- mirroring
// investigator.go's own internal short-circuit guard
// (actionable=false && workflow_id=="") so AF never second-guesses a case KA
// itself would have already treated as ambiguous. A missing rca payload or a
// missing/non-bool is_actionable key (older KA versions, or no RCA at all)
// returns false: the gate must only override on a genuine computed false,
// never on absence.
func rcaConcludedNotActionable(resp map[string]any) bool {
	rcaRaw, ok := resp["rca"]
	if !ok || rcaRaw == nil {
		return false
	}
	rca, ok := rcaRaw.(map[string]any)
	if !ok {
		return false
	}
	isActionable, ok := rca["is_actionable"].(bool)
	if !ok || isActionable {
		return false
	}
	hasWorkflow, _ := rca["has_workflow"].(bool)
	return !hasWorkflow
}
