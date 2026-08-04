# DD-AF-011: Phase-Transition Consent Guard

**Status**: Accepted
**Date**: 2026-08-03
**Issue**: [#1899](https://github.com/jordigilh/kubernaut/issues/1899) (`release/v1.5`), [#1901](https://github.com/jordigilh/kubernaut/issues/1901) (`main` port, tracking clone)
**Related**: [DD-AF-006](DD-AF-006-approval-consent-guard.md) (approval consent guard — structural-rejection precedent this design generalizes)

## Context

A console-team report (#1899) showed AF auto-proceeding into an investigation/remediation phase transition the user never explicitly confirmed, in interactive mode. Investigation identified three independent, compounding root causes, none of which were structurally enforced anywhere in code — whether a given phase transition should wait for a human or auto-proceed was purely emergent from the LLM's own interpretation of prompt prose:

1. **`prompt.txt`'s "Alert Prioritization" section unconditionally directed auto-investigation**, contradicting "Observation Mode" — the LLM sometimes decided to auto-investigate when the user had only asked a read-only question (e.g. "list active alerts").
2. **`reinvoke.go`'s `NeedsReinvocationCtx` blindly nudged continuation on any text-only turn end**, with no awareness of whether an investigation had ever legitimately started. This is the literal #1899 repro: a "free turn" after a read-only query gave the LLM an unprompted opportunity to act.
3. **A more severe risk, discovered during design review**: the identical defect class exists one phase later. After `kubernaut_discover_workflows` returns a recommended workflow, nothing structurally stopped the LLM from calling `kubernaut_select_workflow` — with a guessed workflow/parameters — without genuine user confirmation. This is unapproved-*remediation* risk (creates a `WorkflowExecution` against a live resource), not just unapproved-investigation.

**Architectural root cause**: `InvestigateMCPArgs` had no mode field, and `phase_guard.go`'s existing tool gate only checked "is a driver session active", never "is this the right moment, for this declared level of autonomy, for this specific tool". Two different users — one wanting "just tell me what's wrong" and one wanting "diagnose and fix it end to end, unattended" — were indistinguishable to the harness.

## Decision

### Structural mode signal, not prompt-inferred intent

Add `InteractionMode string` to `InvestigateMCPArgs` (`pkg/apifrontend/tools/ka_investigate_mcp.go`), enum `"interactive" | "full_remediation" | "full_remediation_autonomous"`, mapping onto the prompt's pre-existing (previously implicit) mode taxonomy:

| Mode | Behavior |
|---|---|
| `interactive` (default / omitted — fail-safe, AC-6 least privilege) | Every phase transition waits for genuine user confirmation. |
| `full_remediation` | Auto-proceeds through workflow discovery, but still waits for user confirmation before executing (`select_workflow`). |
| `full_remediation_autonomous` | Auto-proceeds through discovery **and** execution — only appropriate when the user explicitly requested full, unattended remediation. |

An omitted or unrecognized value always resolves to `interactive` (`session.ValidInteractionMode`) — never silently upgraded to a more autonomous mode.

### Defense-in-depth layers (DD-AF-006 precedent, generalized)

[DD-AF-006](DD-AF-006-approval-consent-guard.md) already rejected prompt-only enforcement for `kubernaut_approve` ("Prompt instructions can be circumvented... not sufficient as a sole control") and established a structural-rejection pattern. This design generalizes that exact mechanism to phase transitions:

| Layer | Mechanism | Failure mode it prevents |
|---|---|---|
| 1. Mode declaration | `interaction_mode` arg on `kubernaut_investigate`, persisted to `session.State` (`af_interaction_mode`) by `phaseGuardAfter` | Autonomy level is a structural fact, not an LLM-remembered convention |
| 2. **Primary — dynamic tool-list filtering** | New `checkpointToolFilter` `BeforeModelCallback` (`pkg/apifrontend/agent/checkpoint_tool_filter.go`) deletes `kubernaut_discover_workflows`/`kubernaut_select_workflow` from `LLMRequest.Tools` while their checkpoint flag (`af_phase2_blocked`/`af_phase3_blocked`) is set | The model never sees the gated tool as an option at all — stronger than reject-on-attempt |
| 3. Backstop — hard-reject gate | `phaseGuardBefore` (`pkg/apifrontend/agent/phase_guard.go`) hard-rejects the same tools if a call ever slips past layer 2 (provider quirk, race) | Defends against the primary layer being bypassed, mirroring `errNoActiveDriver`'s existing pattern |
| 4. Reinvocation gate | `NeedsReinvocationCtx` (`pkg/apifrontend/session/reinvoke.go`) never nudges continuation while a checkpoint is blocked, and never nudges before any driver session is active at all | Closes the literal #1899 "free turn" repro — a text-only turn with no investigation ever started is the model legitimately answering a question, not a stalled investigation |
| 5. Prompt fix | `prompt.txt`'s "Alert Prioritization" section made explicitly subordinate to "Observation Mode"; `kubernaut_investigate_alert` gated on a prior investigate/fix/diagnose trigger | Closes the residual single-genuine-turn attack vector (no reinvocation involved) where the LLM could still misread a `list_alerts` response as investigation consent |

Layer 2 mirrors `historySanitizer`'s existing `BeforeModelCallback` slot in `root.go` (`BeforeModelCallbacks: []llmagent.BeforeModelCallback{historySanitizer, checkpointToolFilter}`) and ADK's own `functioncallmodifier` plugin precedent for mutating `LLMRequest.Tools` (`map[string]any` keyed by tool name).

### Checkpoint flags are cleared only at the genuine top-level entry point

`af_phase2_blocked`/`af_phase3_blocked` are cleared exactly once per real inbound A2A message, at the top of `reinvokingRunner.Run` (`pkg/apifrontend/launcher/reinvoking_runner.go`) — never inside the internal reinvocation loop, which re-enters with a synthetic continuation message rather than a genuine user turn. Clearing inside the loop would immediately erase a checkpoint the current turn just legitimately set.

**Empirically confirmed via spike** (throwaway `go test` against real `google.golang.org/adk@v1.5.1`, deleted after running): `sessionService.Get()`/`Create()` return **copies** of the session — a direct `.State().Set(...)` on the returned handle silently no-ops against the canonical stored session. The only durable write path is `sessionService.AppendEvent(ctx, sess, event)` with `event.Actions.StateDelta` set, which is what ADK's own `base_flow.go` does automatically inside real tool callbacks (via a bound `EventActions`). Because `reinvokingRunner.Run` sits *outside* any tool callback, it must call `AppendEvent` explicitly — confirmed safe against a bare `StateDelta`-only event polluting LLM-visible history (`contents_processor.go` skips events with nil/empty content when building model-facing `Contents`).

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `interaction_mode` arg | LLM calls `kubernaut_investigate` | `pkg/apifrontend/tools/ka_investigate_mcp.go` (`InvestigateMCPArgs`) | IT-AF-1899-001 |
| Mode + flag persistence | `phaseGuardAfter` after investigate/discover_workflows success | `pkg/apifrontend/agent/phase_guard.go` | IT-AF-1899-002/003 |
| **Primary: dynamic tool-list filter** | `checkpointToolFilter`, appended to `NewRootAgent`'s `BeforeModelCallbacks` | `pkg/apifrontend/agent/root.go` (registration) + `pkg/apifrontend/agent/checkpoint_tool_filter.go` | IT-AF-1899-004a..e |
| Backstop: hard-reject gate | `phaseGuardBefore` for discover_workflows/select_workflow | `pkg/apifrontend/agent/phase_guard.go` | IT-AF-1899-005/005b/005c |
| Checkpoint clear on new user turn | Top of `reinvokingRunner.Run`, via `sessionService.Get` + `AppendEvent(StateDelta)` | `pkg/apifrontend/launcher/reinvoking_runner.go` | IT-AF-1899-006/007 |
| Reinvocation gate consults flags | `NeedsReinvocationCtx` | `pkg/apifrontend/session/reinvoke.go` | UT-AF-1899-001..004 |
| Prompt fix | Alert Prioritization scoped subordinate to Observation Mode | `pkg/apifrontend/agent/prompt.txt` | UT-AF-1899-010..013, proven jointly by E2E-FP-1899-001/002 |
| E2E journey proof | Full A2A/session stack, both phase boundaries | `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` | E2E-FP-1899-001/002 |

Shared state-key/mode constants live in `pkg/apifrontend/session/consent.go` (not `pkg/apifrontend/agent`, where the tool-filter/phase-guard callbacks are implemented) because `pkg/apifrontend/session/reinvoke.go` also needs them, and both `pkg/apifrontend/agent` and `pkg/apifrontend/launcher` already import `pkg/apifrontend/session` — avoiding an import cycle.

## Alternatives Considered

### A: Prompt-only fix (shrink/reword the "wait for user" prose)

Rejected as a sole control for the same reason DD-AF-006 rejected it for `kubernaut_approve`: prompt instructions can be circumvented by adversarial prompts, model drift across provider updates, or simply an LLM that "forgets" a paragraph many turns back. The prompt fix is still included (layer 5) as defense-in-depth for the one attack surface no structural tool-gate can reach — a single genuine turn where the model itself chooses to over-interpret user intent — but it is not the primary control.

### B: Reinvocation-gate-only fix (fix root cause #2, skip the tool-filter/hard-reject layers)

Rejected: closes the literal #1899 repro (blind reinvocation after a read-only query) but leaves the more severe Phase 2→3 risk (#1899 review finding) completely unaddressed — nothing would stop a same-turn, non-reinvoked `select_workflow` call once the model had already decided (correctly or not) to proceed that far.

### C: Narrow patch (v1.5-only, revert-friendly) vs. structural fix (chosen)

A narrower, `v1.5`-only patch confined to the reinvocation gate was considered and initially presented as an option. Rejected in favor of the full structural fix (this document) because the narrow patch does not close the Phase 2→3 risk class, and the structural fix's design was confirmed portable to `main` (#1901) with no `v1.5`-only assumptions baked in.

## Consequences

### Positive

- Closes both the literal #1899 repro and the more severe Phase 2→3 risk found during design review, with the same mechanism.
- Whether a phase transition requires human confirmation is now a structural, testable fact (`session.State` flags), not emergent LLM behavior.
- Four independent layers (mode declaration, tool-list filtering, hard-reject backstop, reinvocation-gate awareness) plus a prompt-level fix mean a single provider quirk or prompt-injection attempt cannot silently regress the guarantee.
- `full_remediation`/`full_remediation_autonomous` users see zero behavior change — the gate only tightens the `interactive` default path.

### Negative

- `InvestigateMCPArgs` gains a new LLM-visible argument; any external tooling that hand-constructs `kubernaut_investigate` calls without setting `interaction_mode` now defaults to the most conservative (`interactive`) behavior, which may require an explicit mode declaration to restore prior full-autonomy behavior for legitimate autonomous callers.
- Every existing mock-LLM/E2E fixture that scripted a same-turn auto-chain past `kubernaut_investigate` (`08`/`15`/`16`/`17_af_a2a_*_test.go`, per the DISCOVERY sub-phase) required an explicit `interaction_mode` declaration to keep passing under the new fail-safe default.

## Test Coverage

| Tier | IDs | Validates |
|---|---|---|
| UT | UT-AF-1899-001..004 | `NeedsReinvocationCtx` never nudges past a blocked checkpoint or before a driver session exists |
| UT | UT-AF-1899-010..013 | `prompt.txt`'s Alert Prioritization section is subordinate to Observation Mode and gates `kubernaut_investigate_alert` on a prior investigate/fix trigger |
| IT | IT-AF-1899-001 | `interaction_mode` is a JSON-visible, omittable argument on `kubernaut_investigate` |
| IT | IT-AF-1899-002/002b/002c, 003/003b/003c | Mode + phase2/phase3-blocked flags are correctly persisted (including fail-safe on unrecognized mode, and a failed `discover_workflows` never sets phase3_blocked) |
| IT | IT-AF-1899-004a..e | `checkpointToolFilter` removes the correct gated tool(s) from `LLMRequest.Tools`, is a no-op when nothing is blocked, and does not panic on nil state/tools |
| IT | IT-AF-1899-005/005b/005c | `phaseGuardBefore` hard-rejects gated tools while blocked, and allows `select_workflow` when the harness never blocked phase 3 |
| IT | IT-AF-1899-006/007 | Checkpoint flags clear exactly once at a genuine top-level `Run()` call, and a mid-turn re-block correctly suppresses reinvocation without being wiped by `Run()` itself afterward |
| E2E | E2E-FP-1899-001 | Full journey, Phase 1→2: a same-turn fire-and-forget `discover_workflows` attempt is structurally blocked (no `WorkflowExecution` created); the journey completes once every subsequent phase is genuinely confirmed by the user |
| E2E | E2E-FP-1899-002 | Full journey, Phase 2→3 (the more severe risk): `discover_workflows` auto-proceeds as `full_remediation` authorizes, but a same-turn fire-and-forget `select_workflow` attempt is blocked identically; completes once the user genuinely selects a workflow |
| E2E (regression, happy path) | E2E-FP-1853-002 (`17_af_a2a_full_interactive_remediation_test.go`) | `full_remediation_autonomous` still auto-chains investigate→discover_workflows→select_workflow→watch in one turn — the gate does not break the legitimate fully-autonomous-interactive flow |
| E2E (regression, happy path) | E2E-FP-1189-003 family (`08_af_a2a_interactive_test.go`) | Default `interactive` mode's genuine multi-turn journey (each turn is a new top-level `Run()`, clearing checkpoints as it goes) still completes end to end |

Full test design, risk analysis, and BR/FedRAMP coverage matrix: [`docs/testing/1899/TEST_PLAN.md`](../../testing/1899/TEST_PLAN.md).

## FedRAMP Controls

| Control | Application | Evidence |
|---|---|---|
| AC-6 (Least Privilege) | The LLM cannot execute `kubernaut_discover_workflows`/`kubernaut_select_workflow` without a structural confirmation gate scoped to the declared autonomy level — mirrors DD-AF-006's AC-6 mapping for `kubernaut_approve` | IT-AF-1899-004a/004b, E2E-FP-1899-001/002 |
| SI-10 (Information Input Validation) | Tool-call preconditions (phase-appropriate for the declared mode) are structurally validated before execution, not prompt-trusted; unrecognized `interaction_mode` values fail safe to `interactive` | IT-AF-1899-002c |
| AU-3 (Content of Audit Records) | Blocked-attempt error responses are structured and logged (`phase-guard blocked tool` with `reason: checkpoint_blocked`) for observability of blocked attempts | `phase_guard.go` `before` callback |

## Rollout

1. Implemented and fully TDD-verified on `release/v1.5` first (where #1899 and its production users are), branch `fix/1899-phase-transition-consent-guard`.
2. Verified end to end (UT + IT + the new E2E-FP-1899-001/002 journeys) on `v1.5` before porting.
3. Ported to `main` for #1901, adapted to `main`'s already-refactored `reinvoking_runner.go` structure (not a straight cherry-pick, per the confirmed structural diff between branches).

## Errata

### #1912: `driverActive` never cleared on session-terminal tools

The original layer-4 design ("Reinvocation gate", above) correctly closed the "no driver session ever started" case, but the `isTerminal` branch of `phaseGuardAfter` (`pkg/apifrontend/agent/phase_guard.go`) only cleared the `ActiveContextRegistry` entry on a successful `kubernaut_complete`/`kubernaut_cancel` — it never reset `af_interactive_driver_active` (`session.StateKeyDriverActive`) itself. Because `NeedsReinvocationCtx`'s `driverActive(state)` helper reads that flag directly (not the registry), a session that had already been genuinely completed or cancelled by the user could still read as "driver active" to the reinvocation gate. If a later text-only turn in the same chat session then hit `checkpointBlocked(state) == false` (e.g. no DD-AF-011 checkpoint remained set — the common case once a session-terminal tool has already run), the gate would incorrectly re-fire, synthesizing a "continue the investigation" nudge back into a session the user had already closed.

**Fix**: `phaseGuardAfter`'s `isTerminal` branch now also clears `af_interactive_driver_active` (and, for hygiene, `af_active_rr_id`/`af_active_session_id`) alongside the existing registry clear. This does not change the Decision's mode/checkpoint semantics above — it closes a state-hygiene gap in layer 1 (mode declaration/state persistence) that layer 4 (reinvocation gate) depended on being correct.

**Test coverage**: UT-AF-1912-001/002/003 (`phase_guard_test.go`), UT-AF-1912-004 (`reinvoke_test.go`), IT-AF-1912-001 (`test/integration/apifrontend/reinvocation_terminal_test.go`, real production wiring via `NewRootAgent`/`NewA2AHandler`, mirroring IT-AF-1776-001's pattern), E2E-FP-1912-001 (`18_af_a2a_consent_gate_test.go`).

### #1915: `full_remediation` was implemented and tested, but never prompt-wired as the default

The Decision above already specifies `full_remediation` as a fully legitimate mode (auto-discover, pause before execute) with its own harness-level test coverage (IT-AF-1899-002b/003/005b). What this document did not anticipate: `prompt.txt`'s "Mode Detection" section never actually instructed the model to *declare* `full_remediation` for a plain "investigate" request — a plain investigate always omitted `interaction_mode`, resolving to the fail-safe `interactive` default (correctly, per this Decision's own AC-6 rationale). The prompt's separate "CRITICAL — Phase 1 to Phase 2" section then unconditionally told the model to "proceed to Phase 2 without waiting" regardless of which mode had actually been declared — contradicting the harness, which had already removed `kubernaut_discover_workflows` from the model's tool list for bare `interactive` mode. The model, unable to find the tool, incorrectly concluded "no matching workflows" instead of correctly pausing.

**UX resolution** (confirmed with the maintainer): workflow discovery presenting options for the user to choose from is not itself a consequential action requiring a fresh consent gate — only *executing* a workflow is. Per the original progressive-flow intent (#1407: "auto-proceed from investigation to discovery for audit completeness"), `full_remediation` is now the prompt-declared **default** for a plain investigate request. Bare `interactive` (RCA-only, no auto-discovery) remains available, but only via an explicit opt-out phrase ("just investigate", "investigate only", "don't suggest fixes") — this supersedes this document's original framing (Decision table above) of `interactive` as the assumed default path for a plain investigate request; `interactive` remains the correct *fail-safe* when `interaction_mode` is omitted or invalid, but plain "investigate" phrasing itself no longer omits it.

**Fix**: `prompt.txt` only — no harness/code changes, since the underlying mechanism was already correct and tested. See `pkg/apifrontend/agent/prompt.txt`'s "Mode Detection and Flow Selection" and "CRITICAL — Phase 1 to Phase 2 decision" sections.

**Test coverage**: UT-AF-1915-001..004 (`prompt_test.go`, proving the prompt content maps plain investigate to `full_remediation` and the opt-out phrases to omitted/`interactive`), IT-AF-1915-001 (`phase_guard_test.go`, BR/audit traceability cross-reference to the pre-existing IT-AF-1899-002b/003 mechanism coverage). No new E2E scenario was added for #1915 specifically: the `full_remediation` harness mechanism itself is already proven end to end by E2E-FP-1899-002, and the prompt-instruction-to-live-LLM-decision link is out of scope for the mock-LLM E2E infrastructure (consistent with every other `prompt.txt` directive in this test suite, which is verified via content assertions rather than live-LLM behavior).

**Related**: [kubernaut-console#47](https://github.com/jordigilh/kubernaut-console/issues/47) (Console welcome-message keyword clarity for the three modes) was filed as a related follow-up, out of scope for this fix. [kubernaut#1918](https://github.com/jordigilh/kubernaut/issues/1918) (harness-enforced actionability gate, see below) was initially filed as a deferred spike here but was subsequently scoped down and implemented directly.

Full test design, risk analysis, and BR/FedRAMP coverage matrix for both errata fixes: [`docs/tests/1912-1915/TEST_PLAN.md`](../../tests/1912-1915/TEST_PLAN.md).

### #1918: Harness-enforced actionability gate (guarded-D)

#1915's fix made `full_remediation` the prompt-declared default, so auto-discovery (Phase 1→2) now depends on the model correctly reading KA's RCA narrative and declining to proceed when the RCA concluded no remediation is warranted. That reasoning-dependent judgment call is exactly the kind of case this Decision's own rationale (AC-6: the LLM cannot execute a phase transition without a structural gate) argues should not be prompt-trusted — a lower-reasoning model could misread an ambiguous or borderline "not actionable" narrative and call `kubernaut_discover_workflows` anyway, and `full_remediation`/`full_remediation_autonomous`'s existing checkpoint state has no independent way to catch that: the checkpoint is keyed purely on which mode was declared, not on what the RCA itself concluded.

KA's investigator already computes this exact condition internally (`is_actionable=false && workflow_id==""`) as its own short-circuit guard before workflow discovery (`internal/kubernautagent/investigator/investigator.go`) — the signal already exists, it just never crossed the KA→AF wire.

**Fix**: `IsActionable *bool` / `HasWorkflow bool` were added to KA's `rcaEventPayload` (`internal/kubernautagent/session/rca_event_payload.go`) and mirrored onto AF's `InvestigateRCA` (`pkg/apifrontend/tools/ka_investigate_mcp.go`). `phaseGuardAfter`'s `kubernaut_investigate` branch (`pkg/apifrontend/agent/phase_guard.go`) now inspects this structured signal via a new `rcaConcludedNotActionable` helper: when KA's RCA explicitly computed `is_actionable=false` with no workflow already identified, `StateKeyPhase2Blocked` is forced to `true` regardless of the declared `interaction_mode` — this only ever *tightens* an autonomy grant the model already made, never loosens `interactive` mode's own blocked default, and never overrides a genuinely actionable RCA (a missing `rca` payload or missing/non-bool `is_actionable` — e.g. an older KA version — leaves the existing mode-based checkpoint untouched, so the gate only fires on a genuine computed `false`, never on absence). `phaseGuardBefore`'s existing DD-AF-011 hard-reject on `StateKeyPhase2Blocked` requires no changes — the override reuses the same enforcement path already proven by #1899.

This is the "guarded-D" form of the harness-fix option considered and initially set aside during #1918's triage (see the issue's discussion history): scoped to the single condition KA's own investigator already treats as authoritative, rather than a general-purpose confidence/disagreement gate.

**Test coverage**: UT-KA-1918-001..003 (`marshal_rca_subset_test.go`, `is_actionable`/`has_workflow` marshaling), UT-AF-1918-004 (`ka_investigate_rca_test.go`, mirrored unmarshaling), IT-AF-1918-001..006 (`phase_guard_test.go`, the override's mode/actionability/edge-case matrix), IT-AF-1918-001 (`test/integration/apifrontend/actionability_gate_1918_test.go`, real production wiring, mock KA returns `is_actionable=false`, asserts `kubernaut_discover_workflows` is hard-rejected before reaching KA), E2E-FP-1918-001 (`18_af_a2a_consent_gate_test.go`, full journey: `full_remediation_autonomous` declared against a not-actionable signal, a same-turn `discover_workflows` attempt is structurally blocked, no `WorkflowExecution` is ever created).

**Backport note**: implemented and fully TDD-verified on `release/v1.5` first (PR #1925, tracked by #1918), then ported to `main` (this port, tracked by #1924) adapted to `main`'s already-refactored `phase_guard.go` structure (same "not a straight cherry-pick" caveat as the #1912/#1915 backport note above): `main` had already extracted the interaction-mode logic into a standalone `recordInteractionMode(state, inputArgs, logger)` helper that did not receive the `kubernaut_investigate` response payload, so `resp` was threaded through to let `rcaConcludedNotActionable(resp)` — ported verbatim, a pure function with no dependency on the surrounding refactor — inspect it. The new IT test (`actionability_gate_1918_test.go`) was adapted for `main`'s `LLMConfig`/`LLMProviderGemini` having moved from `pkg/apifrontend/config` to `pkg/shared/types`, and `ka_investigate_rca_test.go`'s new assertions for `main`'s `BridgeEventsCollectSummary` three-return-value signature (`release/v1.5` has two). Every other touched file had an unchanged base between the two branches and was ported directly.

**Backport note**: both fixes above were implemented and fully TDD-verified on `release/v1.5` first (PR #1920, tracked by #1912/#1915), then ported to `main` (this port, tracked by #1913/#1917) adapted to `main`'s already-refactored `phaseGuardAfter`/`recordDriverEntryState`/`syncActiveContextRegistry` helper-function structure (see this doc's earlier #1901 Rollout note above — same "not a straight cherry-pick" caveat applies here) and to `main`'s own pre-existing (and differently-worded, but equally under-instructed) `prompt.txt` Mode Detection baseline. `main` additionally carries `kubernaut_complete_no_action` in `sessionTerminalTools` (#1496's `main`-only fix, not present on `v1.5`); the `driverActive`-clearing fix applies uniformly to every entry in that map, so it correctly covers `kubernaut_complete_no_action` on `main` too, with no special-casing required.
