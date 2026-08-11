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
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	"google.golang.org/adk/agent/llmagent"
	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
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

// errDiscoveryRequiredBeforeDecision is returned by phaseGuardBefore's
// #2098 ordering guard when kubernaut_present_decision is attempted before
// kubernaut_discover_workflows has succeeded, in an interaction mode where
// that ordering is load-bearing. Mirrors errCheckpointBlocked's
// reject-and-let-the-model-retry pattern (DD-AF-011, #1899) rather than
// enforceGroundingGuard's mutate-and-continue pattern: unlike a fabrication
// risk, there is no honest content to substitute here, so the correct
// remedy is for the model to call discover_workflows and retry.
const errDiscoveryRequiredBeforeDecision = "kubernaut_discover_workflows has not been called yet -- call kubernaut_discover_workflows first, then retry kubernaut_present_decision"

// noGroundedContentSummary is the fixed, honest payload #2023's grounding
// guard substitutes for present_decision's summary when the most recent
// kubernaut_investigate produced no real content to report. It deliberately
// names the possible reasons in generic, model-agnostic terms rather than
// echoing the specific error string, since the override applies uniformly
// across scope rejections (#2022), tool errors, session_active, and the
// fail-closed "investigate never called" default -- so it must never imply
// a diagnosis more specific than "nothing was found."
const noGroundedContentSummary = "No investigation content is available for this remediation. " +
	"The prior investigation attempt did not produce any root-cause findings to report " +
	"(the target may be outside Kubernaut's management scope, a required tool call may have " +
	"failed, or no investigation has actually completed yet)."

// presentDecisionTool is the name registered by ka_tools.go's
// NewPresentDecisionTool -- kept as a named constant here (rather than a
// literal) since #2023's grounding guard and prompt.txt's naming-accuracy
// fix both depend on this exact string staying in sync with the tool
// registration.
const presentDecisionTool = "kubernaut_present_decision"

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
		// #2023: mutates args in place (never short-circuits the call) so
		// present_decision still executes and the AU-3 structured artifact
		// is always emitted -- only a fabricated narrative is blocked.
		if t.Name() == presentDecisionTool {
			// #2098: reject-and-retry ordering guard, checked BEFORE the
			// #2023 grounding guard runs -- a premature call must not even
			// reach enforceGroundingGuard's mutation/masking logic.
			if presentDecisionRequiresDiscoveryFirst(ctx.State()) {
				logr.FromContextOrDiscard(ctx).Info("phase-guard blocked present_decision",
					"tool", t.Name(), "reason", "discover_workflows_not_yet_succeeded")
				return map[string]any{"error": errDiscoveryRequiredBeforeDecision}, nil
			}
			enforceGroundingGuard(ctx, args)
			return nil, nil
		}

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

		// #2023: track whether THIS kubernaut_investigate call produced
		// real, groundable RCA content -- regardless of success/failure, so
		// a failed/rejected call correctly overwrites a stale grounded=true
		// left by an earlier one in the same session. present_decision's
		// before-callback (enforceGroundingGuard) reads this immediately
		// before it runs. Computed unconditionally here (ahead of the
		// isSuccess/isEntry early-returns below) precisely because a hard
		// failure must still be recorded, not skipped.
		if toolName == "kubernaut_investigate" {
			if state := ctx.State(); state != nil {
				grounded := investigateHasGroundedContent(resp, isSuccess)
				if err := state.Set(session.StateKeyGroundedContentAvailable, grounded); err != nil {
					logr.FromContextOrDiscard(ctx).Error(err, "phase-guard failed to persist grounded_content_available state")
				}

				// Unconditional, mirroring the grounded flag above: a
				// rejected/rca-less call must clear a stale rca left by an
				// earlier successful one in the same session (hardening
				// beyond #2023's original binary gate -- see
				// enforceGroundingGuard).
				//
				// #2068: a Provisional rca (AF's own severity-triage guess,
				// synthesized when KA hasn't genuinely investigated yet --
				// see ka_investigate_mcp.go's two InvestigateRCA fallback
				// construction sites) is deliberately excluded here. It is
				// not "extracted from the KA complete event" the way this
				// struct's own doc comment describes, so it must not be
				// cached as an authoritative fact to substitute into
				// present_decision -- same treatment as "no structured rca
				// at all" (UT-AF-2023-011).
				var rca *tools.InvestigateRCA
				if isSuccess {
					if decoded := decodeInvestigateRCA(resp["rca"]); decoded != nil && !decoded.Provisional {
						rca = decoded
					}
				}
				if err := state.Set(session.StateKeyGroundedRCA, rca); err != nil {
					logr.FromContextOrDiscard(ctx).Error(err, "phase-guard failed to persist grounded_rca state")
				}
			}
		}

		// Refresh idle timer for any successful tool call to keep the
		// active session alive during ongoing engagement (#1446, AU-3).
		if registry != nil && isSuccess && !isEntry && !isTerminal {
			if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
				registry.Refresh(identity.Username)
			}
		}

		// #1997: phaseGuardAfter only ever records a side effect (session
		// state, ActiveContextRegistry) -- it never modifies resp/callErr.
		// Every exit path below returns (nil, nil), the ADK AfterToolCallback
		// convention for "pass through unchanged" (google.golang.org/adk,
		// internal/llminternal/base_flow.go: invokeAfterToolCallbacks treats
		// a non-nil result as an explicit override that stops the chain,
		// falling back to the original fResult/fErr only once every
		// callback has returned nil). Re-returning the original resp/callErr
		// here looked harmless but silently skipped every AfterToolCallback
		// registered after this one (afterLog) for any tool call at all,
		// not just the ones this guard cares about.
		if !isEntry && !isTerminal && !isDiscoverWorkflows {
			return nil, nil
		}
		if !isSuccess {
			return nil, nil
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
				// #2098: unblocks presentDecisionRequiresDiscoveryFirst's
				// ordering guard for the remainder of this driver session.
				if err := state.Set(session.StateKeyDiscoverWorkflowsSucceeded, true); err != nil {
					logger.Error(err, "phase-guard failed to persist discover_workflows_succeeded state")
				}
			}
			return nil, nil
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

					// #2098: a fresh investigation must not inherit a
					// discover_workflows_succeeded flag left by a prior RR
					// in the same chat session.
					if err := state.Set(session.StateKeyDiscoverWorkflowsSucceeded, false); err != nil {
						logger.Error(err, "phase-guard failed to reset discover_workflows_succeeded state")
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

		return nil, nil
	}

	return before, after
}

// presentDecisionRequiresDiscoveryFirst implements #2098's ordering guard:
// it reports whether kubernaut_present_decision must be rejected because
// discover_workflows has not yet succeeded in this driver session, in an
// interaction mode where that ordering actually matters.
//
// Phase2Blocked == false means the declared mode does NOT gate
// discover_workflows behind a human-confirmation checkpoint -- true for
// full_remediation/full_remediation_autonomous, where the model is expected
// to call discover_workflows autonomously before presenting a decision.
// Reusing Phase2Blocked (rather than re-deriving interaction mode here)
// means this gate automatically inherits #1918's rcaConcludedNotActionable
// exemption for free: that override forces Phase2Blocked=true precisely
// because no workflow discovery is expected in that case either.
//
// Fails open (never blocks) when state is nil or Phase2Blocked was never
// set, matching InteractionModeInteractive's own fail-safe default
// (interactionModeFromState) -- a present_decision call with no prior
// investigate at all is enforceGroundingGuard's concern, not this gate's.
func presentDecisionRequiresDiscoveryFirst(state adksession.State) bool {
	if state == nil {
		return false
	}
	phase2Blocked := true
	if v, err := state.Get(session.StateKeyPhase2Blocked); err == nil {
		if b, ok := v.(bool); ok {
			phase2Blocked = b
		}
	}
	if phase2Blocked {
		return false
	}
	succeeded := false
	if v, err := state.Get(session.StateKeyDiscoverWorkflowsSucceeded); err == nil {
		succeeded, _ = v.(bool)
	}
	return !succeeded
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

// investigateHasGroundedContent reports whether a kubernaut_investigate
// response carries real RCA content the model may legitimately summarize in
// present_decision, as opposed to a rejection, failure, or empty result that
// must never be dressed up with invented findings (#2023). isSuccess is
// passed in (rather than recomputed) so this matches the exact success
// determination already made above in the caller.
//
// "session_active" is deliberately excluded from groundedness even though
// it is a legitimate different-user state with its own dedicated fallback
// card (#1922): the CALLING agent still has no fresh RCA of its own to
// report, so present_decision must not fabricate one on its behalf.
//
// Hardening beyond the original QE report: a summary/rca can be
// syntactically present yet still not trustworthy when KA's own #1096
// shadow-agent full-context grounding review flagged this investigation as
// not aligned (reasoning drift, unsupported conclusions, or distributed
// prompt-injection influence -- see
// internal/kubernautagent/alignment/prompt/grounding.go). That verdict is
// already computed and streamed to AF today (EventTypeAlignmentVerdict) but
// was previously only surfaced as a human-facing SSE notification
// (launcher.MetaTypeAlignmentCheckFailed); this treats it as a groundedness
// input too, closing embellishment cases KA's own reviewer already caught.
// Only takes effect when a deployment has ai.alignmentCheck enabled --
// alignment_verdict is absent otherwise, so this is additive, never a
// behavior change for deployments that leave the shadow review off.
func investigateHasGroundedContent(resp map[string]any, isSuccess bool) bool {
	if !isSuccess || resp == nil {
		return false
	}
	if status, _ := resp["status"].(string); status == "unmanaged" || status == "session_active" {
		return false
	}
	if verdict, ok := resp["alignment_verdict"].(map[string]any); ok {
		if result, _ := verdict["result"].(string); result != "" && result != "aligned" {
			return false
		}
	}
	if summary, _ := resp["summary"].(string); strings.TrimSpace(summary) != "" {
		return true
	}
	if rca, ok := resp["rca"]; ok && rca != nil {
		return true
	}
	return false
}

// decodeInvestigateRCA converts a kubernaut_investigate response's "rca"
// value (map[string]any, per the ADK AfterToolCallback's untyped resp
// contract) into the same tools.InvestigateRCA type ka_investigate_mcp.go
// constructs it from, so callers work with named, typed fields (Severity,
// Provisional, ...) instead of probing map keys by hand. Returns nil when v
// isn't a non-empty map or fails to decode.
func decodeInvestigateRCA(v any) *tools.InvestigateRCA {
	raw, ok := v.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var rca tools.InvestigateRCA
	if err := json.Unmarshal(b, &rca); err != nil {
		return nil
	}
	return &rca
}

// canonicalGroundedRCA converts KA's InvestigateRCA (tools package, KA's own
// wire-format field names total_tool_calls/total_llm_turns) into
// present_decision's RCAData shape (tools package, tool_calls_count/
// llm_turns) -- the two have always used different names for these two
// fields since InvestigateRCA mirrors KA's EventTypeComplete payload (a
// contract this package doesn't own) while RCAData is AF's own LLM-facing
// present_decision schema; renaming them is unavoidable however it's
// expressed, so this is done via explicit field assignment rather than a
// generic map copy. rca_summary/is_actionable/has_workflow/provisional have
// no slot in RCAData and are intentionally dropped. Returns nil when rca is
// nil or carries no severity at all, so callers can distinguish "nothing to
// pass through" from "pass through an empty object".
func canonicalGroundedRCA(rca *tools.InvestigateRCA) *tools.RCAData {
	if rca == nil || rca.Severity == "" {
		return nil
	}
	return &tools.RCAData{
		Severity:       rca.Severity,
		Confidence:     rca.Confidence,
		CausalChain:    rca.CausalChain,
		Target:         rca.Target,
		ToolCallsCount: rca.TotalToolCalls,
		LLMTurns:       rca.TotalLLMTurns,
	}
}

// enforceGroundingGuard implements #2023's harness-side fabrication guard.
// Immediately before kubernaut_present_decision executes, it checks whether
// the most recent kubernaut_investigate call (tracked via
// session.StateKeyGroundedContentAvailable in the after-callback above)
// produced real, groundable content. When it did not -- including the
// fail-closed default when the state key was never set at all, e.g.
// present_decision called without any prior investigate -- this overwrites
// args["summary"], args["rca"], and args["options"] in place with a fixed,
// honest "no data" payload, mirroring #2022's own safe-default posture.
//
// This deliberately mutates args rather than short-circuiting the call: the
// AU-3 structured-artifact mandate (#1408) requires present_decision to
// still run and emit an investigation_summary artifact in every scenario --
// only a fabricated narrative is blocked here, never the artifact itself.
//
// Hardening beyond the original QE report: even when grounded, args["rca"]
// is deterministically overwritten with the exact structured facts
// (severity/confidence/causal_chain/target/tool_calls_count/llm_turns) KA's
// kubernaut_investigate response reported (session.StateKeyGroundedRCA),
// rather than trusting the LLM's own transcription of those same facts into
// present_decision. This closes a narrower gap than the grounded/ungrounded
// gate above: a technically-grounded session where the model still alters a
// number or claim while copying it. The free-text args["summary"] is
// deliberately left model-authored once grounded -- it may legitimately
// synthesize reasoning the pure structured facts don't capture -- and
// args["rca"] is left untouched when KA reported no structured rca at all
// (nothing authoritative to substitute) -- and, per #2068, when the only
// "rca" available is AF's own Provisional severity-triage guess rather than
// a genuine KA finding (session.StateKeyGroundedRCA is never populated with
// a Provisional rca in the first place; see the after-callback above).
func enforceGroundingGuard(ctx tool.Context, args map[string]any) {
	if args == nil {
		return
	}
	state := ctx.State()
	if state == nil {
		return
	}

	grounded := false
	if v, err := state.Get(session.StateKeyGroundedContentAvailable); err == nil {
		grounded, _ = v.(bool)
	}
	if grounded {
		if v, err := state.Get(session.StateKeyGroundedRCA); err == nil {
			if rca, ok := v.(*tools.InvestigateRCA); ok {
				if canonical := canonicalGroundedRCA(rca); canonical != nil {
					args["rca"] = canonical
				}
			}
		}
		// #2073: when no authoritative InvestigateRCA was available to fully
		// substitute above (summary-only grounding, a Provisional-severity
		// triage guess, or a malformed investigate rca payload), args["rca"]
		// is still whatever the LLM supplied. tool_calls_count/llm_turns are
		// no longer schema-required (ka_tools.go RCAData omitempty), but the
		// LLM is still never instructed how to compute them -- backfilling
		// an honest zero (rather than leaving whatever value the LLM
		// invented) prevents a fabricated-looking count from reaching the
		// AU-3 structured artifact. severity/confidence/causal_chain/target
		// remain LLM-authored here -- unlike the two bookkeeping fields,
		// there is genuinely nothing authoritative in AF's state to
		// substitute for them in this branch (see canonicalGroundedRCA doc).
		if _, isRCAData := args["rca"].(*tools.RCAData); !isRCAData {
			if rcaMap, ok := args["rca"].(map[string]any); ok {
				rcaMap["tool_calls_count"] = 0
				rcaMap["llm_turns"] = 0
			}
		}
		repairPresentDecisionOptions(ctx, args)
		return
	}

	logr.FromContextOrDiscard(ctx).Info("grounding-guard overriding present_decision content: no groundable investigation content available")

	args["summary"] = noGroundedContentSummary
	// #1396 (ka_tools.go PresentDecisionArgs): rca is a required property in
	// present_decision's ADK-generated JSON schema -- deleting the key
	// entirely (rather than zeroing it) makes ADK's own schema validation
	// reject the call with "required: missing properties: [rca]" before it
	// ever reaches this tool's handler, silently defeating the AU-3
	// mandate this guard exists to preserve (present_decision must still
	// execute and emit its structured artifact even when ungrounded).
	args["rca"] = emptyRCAPayload
	args["options"] = []any{}
}

// repairPresentDecisionOptions defensively repairs args["options"] when it
// arrives as a JSON-encoded string instead of the native array
// present_decision's schema requires (#2092): live evidence showed a model
// emitting a fully-correct options payload, just double-encoded -- the
// array serialized once, then that whole JSON blob wrapped again as a
// string value. ADK's functionTool.Run (ConvertToWithJSONSchema,
// google.golang.org/adk@v1.5.1/tool/functiontool/function.go) re-marshals
// args and validates the result against the inferred schema immediately
// after this callback returns, and a string where the schema declares
// "type: array" fails that validation before HandlePresentDecision ever
// runs -- the same "mutate args before schema validation" ordering
// enforceGroundingGuard already relies on for rca/summary/options above.
//
// Only called from the grounded branch: the ungrounded branch above already
// unconditionally overwrites options with a clean empty slice, so it can
// never carry a stringified value in the first place.
//
// Left untouched (and thus still schema-rejected, surfacing a real,
// actionable error) when the string is empty or does not parse as a JSON
// array -- this must never mask genuinely malformed input as an
// empty-but-valid payload (SI-11).
func repairPresentDecisionOptions(ctx tool.Context, args map[string]any) {
	raw, ok := args["options"].(string)
	if !ok {
		return
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return
	}
	var parsed []any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		logr.FromContextOrDiscard(ctx).Info(
			"present_decision options arrived as a string that is not valid JSON; leaving as-is for schema validation to reject",
			"error", err.Error())
		return
	}
	args["options"] = parsed
}

// emptyRCAPayload is the zero-value RCAData (tools.RCAData) shape,
// satisfying present_decision's required "rca" property while carrying no
// fabricated findings. CausalChain is omitted (its json tag carries
// omitempty) rather than an empty slice, matching RCAData's own zero value.
var emptyRCAPayload = map[string]any{
	"severity":         "",
	"confidence":       0,
	"target":           "",
	"tool_calls_count": 0,
	"llm_turns":        0,
}
