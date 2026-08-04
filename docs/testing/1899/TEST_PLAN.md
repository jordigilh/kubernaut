# Test Plan: AF Phase-Transition Consent Guard (DD-AF-011)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1899-v1.0
**Feature**: Harness-enforced consent gate preventing AF's A2A agent from auto-proceeding past a phase transition (investigation → workflow discovery → workflow execution) the user never genuinely confirmed, replacing prompt-only "wait for the user" instructions.
**Version**: 1.0
**Created**: 2026-08-03
**Author**: Cursor Agent (session-driven, user-directed)
**Status**: Complete
**Branch**: `fix/1899-phase-transition-consent-guard` (based on `release/v1.5`)

---

## 1. Introduction

### 1.1 Purpose

Issue [#1899](https://github.com/jordigilh/kubernaut/issues/1899) reported AF auto-proceeding into a phase transition the user never explicitly confirmed while in interactive mode. Investigation confirmed three independent, compounding root causes (see [DD-AF-011](../../architecture/decisions/DD-AF-011-phase-transition-consent-guard.md) for full analysis):

1. `prompt.txt`'s "Alert Prioritization" section unconditionally directed auto-investigation, contradicting "Observation Mode".
2. `NeedsReinvocationCtx` blindly nudged continuation on any text-only turn end, with no awareness of whether an investigation had legitimately started — the literal #1899 repro.
3. **A more severe risk, found during design review**: the identical defect class exists one phase later — nothing stopped `kubernaut_select_workflow` (which creates a `WorkflowExecution` against a live resource) from being called with a guessed workflow, without genuine user confirmation.

This plan proves a structural, harness-enforced consent gate (DD-AF-011) closes all three, following the [DD-AF-006](../../architecture/decisions/DD-AF-006-approval-consent-guard.md) structural-rejection precedent, without breaking any existing `full_remediation`/`full_remediation_autonomous` auto-chaining behavior.

### 1.2 Objectives

1. **O1**: `kubernaut_investigate` accepts a new, LLM-visible `interaction_mode` argument (`interactive` | `full_remediation` | `full_remediation_autonomous`), omittable and fail-safe to `interactive` (AC-6 least privilege).
2. **O2**: A successful `kubernaut_investigate`/`kubernaut_discover_workflows` call persists the declared mode and the corresponding phase-2/phase-3 "blocked" checkpoint flags to `session.State`.
3. **O3**: While a checkpoint flag is set, the gated tool (`kubernaut_discover_workflows`/`kubernaut_select_workflow`) is structurally absent from the model's tool list (`checkpointToolFilter`, primary layer) and hard-rejected if attempted anyway (`phaseGuardBefore`, backstop layer).
4. **O4**: Checkpoint flags clear only at a genuine top-level user turn (`reinvokingRunner.Run`'s entry point), never inside the internal reinvocation loop — proven both by direct unit test and by the empirically-validated `AppendEvent`/`StateDelta` persistence mechanism (see DD-AF-011 spike findings).
5. **O5**: `NeedsReinvocationCtx` never nudges continuation while a checkpoint is blocked, and never nudges before any driver (investigation) session is active at all.
6. **O6**: `prompt.txt`'s Alert Prioritization section is explicitly subordinate to Observation Mode and no longer issues an unconditional auto-investigate directive.
7. **O7 (pyramid invariant — E2E proves the journey)**: the full A2A/session stack structurally blocks a same-turn fire-and-forget attempt at each gated phase transition, and the journey completes normally once the user genuinely confirms each phase — for both the Phase 1→2 and the more severe Phase 2→3 case.
8. **O8 (no regression)**: existing `full_remediation_autonomous` (auto-chain through both discovery and execution) and default `interactive` (multi-turn, user-driven) journeys continue to complete correctly under the new fail-safe default.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/session/... ./pkg/apifrontend/agent/... -ginkgo.focus="1899"` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/agent/... ./pkg/apifrontend/tools/... ./pkg/apifrontend/launcher/... -ginkgo.focus="1899"` |
| E2E journey pass rate | 2/2 new + 0 regressions | `ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1899"` + full FP suite regression spot-check |
| Full `pkg/apifrontend` regression | 0 | `go test ./pkg/apifrontend/...` |
| Backward compatibility | 0 regressions | `full_remediation_autonomous` (E2E-FP-1853-002) and default-`interactive` (E2E-FP-1189 family / `08_af_a2a_interactive_test.go`) journeys unaffected |

---

## 2. References

### 2.1 Authority

- BR-INTERACTIVE-001: Interactive Investigation Sessions (SREs directing AI investigation/remediation in real time — the consent gate is the structural enforcement of "in real time, with genuine human direction")
- Issue #1899: AF auto-proceeds past a phase transition without genuine user confirmation
- Issue #1901: `main` tracking clone of #1899 (this plan's design is ported there after v1.5 verification)
- [DD-AF-006](../../architecture/decisions/DD-AF-006-approval-consent-guard.md): approval consent guard — the structural-rejection precedent this design generalizes
- [DD-AF-011](../../architecture/decisions/DD-AF-011-phase-transition-consent-guard.md): this feature's own design decision record (root-cause analysis, layered design, spike findings)

### 2.2 Cross-References

- `pkg/apifrontend/agent/phase_guard.go` (pre-existing `phaseGuardBefore`/`phaseGuardAfter` hard-reject pattern, extended here)
- `pkg/apifrontend/agent/root.go` (`BeforeModelCallbacks` registration slot, `historySanitizer` precedent)
- `pkg/apifrontend/launcher/reinvoking_runner.go` (BR-SESS-013 reinvocation loop, extended with checkpoint-clear-on-genuine-turn)
- `pkg/apifrontend/session/reinvoke.go` (`NeedsReinvocationCtx`, extended with mode/flag awareness)
- `test/e2e/fullpipeline/17_af_a2a_full_interactive_remediation_test.go` (#1853 precedent this plan's E2E journeys are modeled on — same single-message auto-chain pattern, asserting structural absence instead of presence)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|---|---|---|---|---|---|
| R1 | Shared-state-store assumption (`sessionService.Get().Session.State().Set()` persists durably) is wrong | Checkpoint-clear silently no-ops; stale blocked flags leak across turns or genuine turns never unblock | High if unvalidated | IT-AF-1899-006/007 | **Resolved by spike** (empirical `go test` against real `google.golang.org/adk@v1.5.1`): `Get()`/`Create()` return copies; only `AppendEvent`+`Actions.StateDelta` durably persists. `clearCheckpointFlags` implemented against the confirmed-correct mechanism from the start |
| R2 | Every existing mock-LLM/E2E fixture exercising `kubernaut_investigate` with no mode field now defaults to `interactive` and is unexpectedly blocked mid-chain | Silent CI regression across `08`/`15`/`16`/`17_af_a2a_*_test.go` and 5 `test/services/mock-llm/*` fixture files | High (confirmed pre-existing gap during DISCOVERY) | All pre-existing AF A2A E2E/UT fixtures | DISCOVERY sub-phase enumerated and classified every `kubernaut_investigate` call site by intended mode *before* GREEN; fixtures requiring auto-chain behavior explicitly declare `full_remediation`/`full_remediation_autonomous` |
| R3 | `checkpointToolFilter` (primary layer) and `phaseGuardBefore` (backstop layer) drift out of sync — e.g. a new gated tool added to one map but not the other | A tool is filtered from the model's list but not hard-rejected (or vice versa), silently weakening one of the two defense-in-depth layers | Low | IT-AF-1899-004a/004b, IT-AF-1899-005/005b | Both layers key off the identical `session.StateKeyPhase2Blocked`/`StateKeyPhase3Blocked` constants from the single shared `pkg/apifrontend/session/consent.go` source of truth, not independently duplicated string literals |
| R4 | A checkpoint re-blocked mid-turn (e.g. `discover_workflows` succeeds and sets `phase3_blocked` again) is incorrectly cleared by `reinvokingRunner.Run`'s own checkpoint-clear step immediately afterward, silently defeating the just-set block | The Phase 2→3 gate would never actually hold — the more severe risk this design exists to close | Medium (subtle timing bug class) | IT-AF-1899-007 | Explicit test proves a mid-turn re-block suppresses reinvocation for the rest of that turn **and** is not wiped by `Run()`'s own checkpoint-clear afterward (clear only runs once, at entry, before the inner runner is ever invoked) |
| R5 | `interaction_mode` validation accepts an unrecognized/malformed value and silently upgrades to a more permissive mode instead of failing safe | Privilege escalation via a malformed or adversarial argument value (SI-10 violation) | Low | IT-AF-1899-002c | `session.ValidInteractionMode` allow-lists exactly 3 values; any other value (including empty string beyond the intentional default) resolves to `interactive` |
| R6 | E2E tests script the mock-LLM to *misbehave* (attempt the gated tool anyway) to prove the harness's own structural gate, but the mock-LLM's `NextToolCall` chaining was capped at a single hop, unable to script the full multi-hop attempt sequences these journeys need | E2E-FP-1899-001/002 cannot be written at all without first fixing the chaining engine | Confirmed (blocking dependency, not a v1.5-native gap) | E2E-FP-1899-001/002 | Ported the N-deep `NextToolCall` chaining engine + `$from_tool`/fallback-argument resolution already fixed on `main` for issue #1853 (see `docs/testing/1853/TEST_PLAN.md`) to `release/v1.5` before writing these E2E tests |

### 3.1 Risk-to-Test Traceability

R1 is the highest-severity risk and is closed by direct empirical evidence (see DD-AF-011 §"Spike findings") before any production code was written on top of the assumption — IT-AF-1899-006/007 then prove the corrected mechanism behaves correctly under test. R2 is closed by the DISCOVERY sub-phase's fixture audit, not discovered as a surprise CI failure. R4 (the subtlest correctness risk) has a dedicated test (IT-AF-1899-007) that specifically distinguishes "suppressed reinvocation" from "flag incorrectly cleared" — these are easy to conflate and a naive test could pass for the wrong reason. R6 is closed structurally (a separate, verified port) before GREEN begins on the E2E tier.

---

## 4. Scope

### 4.1 Features to be Tested

- **`InteractionMode` field on `InvestigateMCPArgs`** (`pkg/apifrontend/tools/ka_investigate_mcp.go`): JSON-visible, omittable LLM argument.
- **Mode + checkpoint-flag persistence** (`pkg/apifrontend/agent/phase_guard.go`): `af_interaction_mode`, `af_phase2_blocked`, `af_phase3_blocked` correctly set/left unset per the mode taxonomy.
- **`checkpointToolFilter` `BeforeModelCallback`** (new, `pkg/apifrontend/agent/checkpoint_tool_filter.go`): primary structural-absence enforcement layer.
- **`phaseGuardBefore` hard-reject backstop** (extended, `pkg/apifrontend/agent/phase_guard.go`): defense-in-depth for a call slipping past the tool filter.
- **Checkpoint-clear-on-genuine-turn** (`pkg/apifrontend/launcher/reinvoking_runner.go`): `clearCheckpointFlags`, called once at `Run()`'s entry.
- **Mode/flag-aware reinvocation gate** (`pkg/apifrontend/session/reinvoke.go`): `NeedsReinvocationCtx` extended with `driverActive`/`checkpointBlocked` checks.
- **`prompt.txt` Alert Prioritization fix**: subordination to Observation Mode, gated `kubernaut_investigate_alert` directive.
- **E2E-FP-1899-001/002** (`test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go`): full-journey proof for both gated phase transitions.
- **Ported #1853 mock-LLM N-deep `NextToolCall` chaining engine** (`test/services/mock-llm/`): enabling dependency for the above E2E tests on `release/v1.5` (already independently tested; see `docs/testing/1853/TEST_PLAN.md`).

### 4.2 Features Not to be Tested

- **The `main` branch port (#1901)**: tracked separately; this plan covers `release/v1.5` verification only, per the DD-AF-011 rollout order (verify on v1.5 first, then port).
- **`kubernaut_approve`'s existing consent guard (DD-AF-006)**: unrelated, unmodified by this change; not retested here.
- **Live LLM provider behavior**: all UT/IT/E2E tests use the mock-LLM or direct unit-level callback invocation — no live Claude/Gemini/GPT calls are exercised or required to prove the structural gate (the gate's entire purpose is to hold even when the model itself misbehaves).

### 4.3 Design Decisions

| Decision | Rationale |
|---|---|
| Structural mode signal + two-layer tool gating (filter + hard-reject), not prompt-only | Mirrors DD-AF-006's rejection of prompt-only enforcement for `kubernaut_approve` — the same reasoning applies here: prompt instructions are circumventable, structural absence is not |
| Shared state-key constants centralized in `pkg/apifrontend/session/consent.go` | `reinvoke.go`'s `NeedsReinvocationCtx` and `phase_guard.go`/`checkpoint_tool_filter.go` all need the identical keys; centralizing avoids duplicated string literals (R3) and import cycles (both `agent` and `launcher` already import `session`) |
| Checkpoint clear via `AppendEvent`+`StateDelta`, not direct `.State().Set()` | Empirically the only mechanism that durably persists against the real ADK `InMemoryService` (see DD-AF-011 spike findings) — a direct `.Set()` call was confirmed via spike to silently no-op |
| E2E scenarios script the mock-LLM to deliberately misbehave (fire-and-forget attempt at the gated tool) rather than relying on well-behaved LLM output | Proves the server-side structural gate holds even under adversarial/non-compliant model output — the exact threat model DD-AF-006 and this design exist to defend against, not merely "does a well-behaved LLM avoid the tool" |
| Port #1853's mock-LLM chaining engine to `release/v1.5` rather than writing a `v1.5`-only, shallower chaining mechanism | Avoids maintaining two divergent mock-LLM implementations across branches; the `main` fix was already fully proven (own test plan, own live-cluster validation) — porting is lower-risk than re-deriving |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of new/modified logic in `reinvoke.go` (`driverActive`, `checkpointBlocked`, mode-aware `NeedsReinvocationCtx`) and `prompt.txt`'s content assertions.
- **Integration**: every Wiring Manifest row in DD-AF-011 has at least one IT proving production wiring — mode arg round-trip, flag persistence (including fail-safe/failure-path branches), tool-filter callback behavior (including nil-safety), hard-reject backstop, and checkpoint-clear timing.
- **E2E**: both new consent-gate journeys (Phase 1→2, Phase 2→3) proven end-to-end against the real AF binary, real ADK tool-calling loop, and the real FP mock-LLM ConfigMap — required by the pyramid invariant ("E2E proves the journey") and FedRAMP's E2E control-objective coverage mandate for AC-6/SI-10, not satisfied by IT-level wiring proof alone.

### 5.2 Two-Tier Minimum (Pyramid Invariant)

UT (logic) + IT (wiring) + E2E (journey) are all provided — this feature does not stop at IT-level wiring proof; the two new E2E specs are the structural-gate equivalent of DD-AF-006's `ADV-AF-1415-001/002` adversarial proofs.

### 5.4 Pass/Fail Criteria

**PASS**: all new UT/IT/E2E pass; the full pre-existing `pkg/apifrontend/...` suite passes with zero regressions; all pre-existing AF A2A E2E fixtures (`08`/`15`/`16`/`17_af_a2a_*_test.go`) pass unmodified in intent (only mode-declaration additions, no behavioral rewrites); `go build ./...`, `go vet ./...`, and `gofmt -l` are clean on all changed/new files.

**FAIL**: any new test fails, any pre-existing test regresses, or any Wiring Manifest row lacks a passing IT.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `pkg/apifrontend/session/reinvoke.go` | `NeedsReinvocationCtx`, `driverActive`, `checkpointBlocked` | ~40 |
| `pkg/apifrontend/agent/prompt.txt` | Alert Prioritization section (content assertions via `prompt_test.go`) | ~15 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `pkg/apifrontend/tools/ka_investigate_mcp.go` | `InvestigateMCPArgs.InteractionMode` (JSON tag) | ~5 |
| `pkg/apifrontend/agent/phase_guard.go` | `newPhaseGuard`'s `before`/`after` closures, `interactionModeFromState` | ~60 |
| `pkg/apifrontend/agent/checkpoint_tool_filter.go` | `checkpointToolFilter`, `checkpointFlagSet` | ~40 |
| `pkg/apifrontend/launcher/reinvoking_runner.go` | `Run`, `needsReinvocation`, `clearCheckpointFlags` | ~50 |
| `pkg/apifrontend/session/consent.go` | State-key/mode constants, `ValidInteractionMode` | ~15 |

### 6.3 E2E-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` | Both `Describe` blocks (Phase 1→2, Phase 2→3) | ~260 |
| `test/infrastructure/shared_e2e.go` | `consentGatePhase2AttemptScenarioYAML`, `consentGatePhase3AttemptScenarioYAML` | ~75 |
| `test/infrastructure/fullpipeline_e2e.go` | `afRemediateNS` map (2 new keys: `consent-phase2`, `consent-phase3`) | ~10 |

---

## 7. BR / FedRAMP Coverage Matrix

| Authority | Description | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-INTERACTIVE-001, AC-6 | `interaction_mode` is a JSON-visible, omittable argument, fail-safe to `interactive` | P0 | Integration | IT-AF-1899-001 | Pass |
| BR-INTERACTIVE-001, AC-6, SI-10 | Mode + phase2/phase3-blocked flags correctly persisted, including fail-safe on unrecognized mode and no-op on a failed `discover_workflows` | P0 | Integration | IT-AF-1899-002/002b/002c, 003/003b/003c | Pass |
| AC-6 | `checkpointToolFilter` removes exactly the correct gated tool(s), no-ops when nothing blocked, and does not panic on nil state/tools | P0 | Integration | IT-AF-1899-004a..e | Pass |
| AC-6 (defense-in-depth) | `phaseGuardBefore` hard-rejects gated tools while blocked; allows `select_workflow` when phase 3 was never blocked | P0 | Integration | IT-AF-1899-005/005b/005c | Pass |
| AC-6 | Checkpoint flags clear exactly once at a genuine top-level `Run()` call; a mid-turn re-block is not wiped by `Run()` itself | P0 | Integration | IT-AF-1899-006/007 | Pass |
| AC-6 | `NeedsReinvocationCtx` never nudges past a blocked checkpoint or before a driver session exists; still nudges for legitimate continuation | P0 | Unit | UT-AF-1899-001..004 | Pass |
| AC-6, SI-10 | `prompt.txt` Alert Prioritization is subordinate to Observation Mode; `kubernaut_investigate_alert` gated on a prior investigate/fix trigger | P1 | Unit | UT-AF-1899-010..013 | Pass |
| BR-INTERACTIVE-001, AC-6, SI-10 | **Full journey, Phase 1→2**: same-turn fire-and-forget `discover_workflows` attempt is structurally blocked (no `WorkflowExecution` created); journey completes once every phase is genuinely confirmed | P0 | E2E | E2E-FP-1899-001 | Pass |
| BR-INTERACTIVE-001, AC-6, SI-10 | **Full journey, Phase 2→3** (more severe risk): `discover_workflows` auto-proceeds as mode authorizes, but same-turn fire-and-forget `select_workflow` is blocked identically; completes once the user genuinely selects a workflow | P0 | E2E | E2E-FP-1899-002 | Pass |
| AC-6 (regression) | `full_remediation_autonomous` still auto-chains investigate→discover_workflows→select_workflow→watch in one turn under the new gate | P0 | E2E | E2E-FP-1853-002 (`17_af_a2a_full_interactive_remediation_test.go`, updated) | Pass |
| AC-6 (regression) | Default `interactive` mode's genuine multi-turn journey still completes end to end | P1 | E2E | E2E-FP-1189-003 family (`08_af_a2a_interactive_test.go`) | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `UT-AF-1899-001` | `NeedsReinvocationCtx` does NOT nudge when no driver session is active (literal #1899 repro) | Pass |
| `UT-AF-1899-002` | Does NOT nudge while `af_phase2_blocked` is set, even with an active driver | Pass |
| `UT-AF-1899-003` | Does NOT nudge while `af_phase3_blocked` is set, even with an active driver | Pass |
| `UT-AF-1899-004` | DOES nudge with an active driver and no checkpoint blocked (regression: legitimate continuation still works) | Pass |
| `UT-AF-1899-010` | Alert Prioritization declares itself subordinate to Observation Mode | Pass |
| `UT-AF-1899-011` | Prompt explicitly forbids treating a bare list/show/status response's `prioritized` field as implicit investigate consent | Pass |
| `UT-AF-1899-012` | `kubernaut_investigate_alert` directive is gated on a prior investigate/fix/diagnose trigger, not a purely informational message | Pass |
| `UT-AF-1899-013` | The old unconditional `kubernaut_investigate_alert` directive text does not survive verbatim | Pass |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `IT-AF-1899-001` | `interaction_mode` serializes under the correct JSON key when declared, and is omitted (not forced) when empty | Pass |
| `IT-AF-1899-002` | Successful investigate with no `interaction_mode` defaults to `interactive` and blocks phase 2 | Pass |
| `IT-AF-1899-002b` | `interaction_mode=full_remediation` does NOT block phase 2 | Pass |
| `IT-AF-1899-002c` | An unrecognized `interaction_mode` value fails safe to `interactive` | Pass |
| `IT-AF-1899-003` | Successful `discover_workflows` blocks phase 3 unless mode is `full_remediation_autonomous` | Pass |
| `IT-AF-1899-003b` | `full_remediation_autonomous` does NOT block phase 3 | Pass |
| `IT-AF-1899-003c` | A failed `discover_workflows` does not set `phase3_blocked` | Pass |
| `IT-AF-1899-004a` | `checkpointToolFilter` removes `kubernaut_discover_workflows` when `af_phase2_blocked` is set | Pass |
| `IT-AF-1899-004b` | Removes `kubernaut_select_workflow` when `af_phase3_blocked` is set | Pass |
| `IT-AF-1899-004c` | Leaves the tool list untouched when no checkpoint is blocked | Pass |
| `IT-AF-1899-004d` | Handles a nil `State` without panicking (fail-safe: defers to hard-reject backstop) | Pass |
| `IT-AF-1899-004e` | Handles a nil/empty `Tools` map without panicking | Pass |
| `IT-AF-1899-005` | `phaseGuardBefore` hard-rejects `discover_workflows` while phase2 is blocked, even with an active driver | Pass |
| `IT-AF-1899-005b` | Hard-rejects `select_workflow` while phase3 is blocked | Pass |
| `IT-AF-1899-005c` | Allows `select_workflow` when the harness never blocked phase 3 (`full_remediation_autonomous`) | Pass |
| `IT-AF-1899-006` | A genuine top-level `Run()` call clears leftover checkpoint flags before invoking the inner runner | Pass |
| `IT-AF-1899-007` | A checkpoint re-blocked mid-turn correctly suppresses reinvocation and is NOT wiped by `Run()` itself afterward | Pass |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `E2E-FP-1899-001` | Phase 1→2: a single message declares `interaction_mode=interactive`, then a same-turn fire-and-forget `discover_workflows` attempt is structurally blocked (investigate ran, but no `WorkflowExecution` exists); a genuine follow-up user turn per phase then completes the full pipeline | Pass |
| `E2E-FP-1899-002` | Phase 2→3 (more severe risk): a single message declares `interaction_mode=full_remediation` (legitimately authorizing auto-chain into `discover_workflows`), then a same-turn fire-and-forget `select_workflow` attempt is blocked identically; a genuine follow-up selection then completes the pipeline | Pass |

### Tier Skip Rationale

None — this feature has full UT/IT/E2E coverage per the pyramid invariant; no tier is skipped.

---

## 9. Test Cases (P0 detail)

### E2E-FP-1899-001: Phase-Transition Consent Gate — Phase 1→2

**BR**: BR-INTERACTIVE-001, AC-6, SI-10
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go`

**Test Steps**:
1. **Given**: an isolated namespace with a zero-replica `memory-eater` `Deployment`, and a mock-LLM keyword scenario (`af_consent_gate_phase2_1899`) scripted to chain `kubernaut_remediate` → `kubernaut_investigate` (declaring `interaction_mode: "interactive"`) → a same-turn fire-and-forget `kubernaut_discover_workflows` attempt.
2. **When**: a single A2A message ("create and investigate then sneak workflow discovery...") is sent.
3. **Then**: the turn completes gracefully (HTTP 200, no JSON-RPC error); a `RemediationRequest` is created (investigate ran); polling confirms **no `WorkflowExecution` exists** for that RR (the fire-and-forget attempt never reached KA).
4. **When**: three genuine follow-up turns are sent on the same task — "discover available workflows", "select workflow \<uuid\>", "watch remediation progress".
5. **Then**: each genuine turn succeeds with no JSON-RPC error, and the `WorkflowExecution` for the RR reaches completion.

**Acceptance Criteria**: the gate blocks the fire-and-forget attempt but is not a permanent stall — it lifts correctly on each subsequent genuine user turn.

### E2E-FP-1899-002: Phase-Transition Consent Gate — Phase 2→3

**BR**: BR-INTERACTIVE-001, AC-6, SI-10
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go`

**Test Steps**:
1. **Given**: the same isolated-namespace setup, with a scenario (`af_consent_gate_phase3_1899`) scripted to chain `kubernaut_remediate` → `kubernaut_investigate` (declaring `interaction_mode: "full_remediation"`, legitimately authorizing discovery) → `kubernaut_discover_workflows` (succeeds) → a same-turn fire-and-forget `kubernaut_select_workflow` attempt with a real (but unconfirmed) workflow UUID.
2. **When**: a single A2A message ("create and investigate then sneak workflow selection...") is sent.
3. **Then**: the turn completes gracefully; the RR exists and `discover_workflows` succeeded (mode-authorized), but polling confirms **no `WorkflowExecution` exists** for the RR.
4. **When**: two genuine follow-up turns are sent — "select workflow \<uuid\>", "watch remediation progress".
5. **Then**: both succeed with no JSON-RPC error, and the `WorkflowExecution` reaches completion.

**Acceptance Criteria**: the harness lets the model auto-proceed exactly as far as the declared mode authorizes (through discovery) and no further — the more consequential action (workflow execution) always requires genuine confirmation.

### IT-AF-1899-007: checkpoint re-blocked mid-turn is not wiped by `Run()` itself

**BR**: AC-6
**Priority**: P0
**Type**: Integration
**File**: `pkg/apifrontend/launcher/reinvoking_runner_test.go`

**Test Steps**:
1. **Given**: a fake inner runner whose first invocation ends in a text-only turn end that would normally trigger BR-SESS-013 reinvocation, and whose side effect re-sets `af_phase2_blocked` (simulating a mid-turn `kubernaut_investigate` success under `interactive` mode).
2. **When**: `reinvokingRunner.Run` is invoked once (a single genuine top-level call).
3. **Then**: exactly one inner call occurs (reinvocation is correctly suppressed because the checkpoint is now blocked); the final persisted state still shows the checkpoint blocked — `Run()`'s own entry-point checkpoint-clear step does not fire a second time and erase what the turn just legitimately set.

**Acceptance Criteria**: distinguishes "reinvocation correctly suppressed" from "flag incorrectly cleared" — the subtlest correctness risk in this design (R4).

---

## 10. Environmental Needs

### 10.1 Unit & Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: none for UT; IT uses in-process fakes (`mapState`, `stubCallbackContext`, `checkpointRunnerCall`/`checkpointObservingRunner`) — no external dependencies, no real ADK session service
- **Location**: `pkg/apifrontend/{session,agent,tools,launcher}/`

### 10.2 E2E Tests

- **Framework**: Ginkgo/Gomega BDD, real Kind-based `fullpipeline` cluster infrastructure (`test/infrastructure/fullpipeline_e2e*.go`)
- **Dependency**: real AF binary, real ADK tool-calling loop, real mock-LLM binary (with the ported #1853 N-deep `NextToolCall` chaining engine)
- **Isolation**: dedicated namespaces (`consent-phase2`, `consent-phase3`) per the established per-test isolation convention

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Resolution |
|---|---|---|---|---|
| #1853 mock-LLM N-deep `NextToolCall` chaining engine | In-repo (originally `main`-only) | Ported to `release/v1.5` as part of this effort | E2E-FP-1899-001/002 could not script the multi-hop fire-and-forget attempts these journeys require | Preflighted and cherry-picked (`44e7b19a4`) onto this branch, with merge conflicts resolved to exclude `main`-only, unrelated features (streaming, fleet) |

### 11.2 Execution Order

1. **DISCOVERY**: enumerate and classify every `kubernaut_investigate` call site in mock-llm/E2E fixtures by intended mode.
2. **RED**: failing tests for the mode arg, flag persistence, tool-filter callback, hard-reject backstop, checkpoint-clear timing, and reinvocation-gate awareness (UT/IT) — plus the two E2E journeys — all written against the not-yet-implemented gate.
3. **GREEN**: wire every Wiring Manifest row (DD-AF-011); update mock-llm/E2E fixtures per the DISCOVERY classification; fix the `prompt.txt` Alert Prioritization conflict; CHECKPOINT W.
4. **REFACTOR**: cleanup pass — confirmed no anti-pattern-checklist violations in new/modified files; `go build ./...`, `go vet ./...`, `gofmt -l` all clean.
5. **Verification**: full `pkg/apifrontend/...` and `test/services/mock-llm/...` suites green; E2E dry-run discovery confirmed.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|---|---|---|
| This test plan | `docs/testing/1899/TEST_PLAN.md` | Strategy and test design |
| Design decision record | `docs/architecture/decisions/DD-AF-011-phase-transition-consent-guard.md` | Root-cause analysis, layered design, spike findings, alternatives |
| Unit tests | `pkg/apifrontend/session/reinvoke_test.go`, `pkg/apifrontend/agent/prompt_test.go` | Ginkgo BDD |
| Integration tests | `pkg/apifrontend/tools/ka_investigate_mcp_test.go`, `pkg/apifrontend/agent/phase_guard_test.go`, `pkg/apifrontend/agent/checkpoint_tool_filter_test.go`, `pkg/apifrontend/launcher/reinvoking_runner_test.go` | Ginkgo BDD |
| E2E tests | `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` | Ginkgo BDD, real FP cluster |
| Production code | `pkg/apifrontend/session/consent.go` (new), `pkg/apifrontend/agent/checkpoint_tool_filter.go` (new), `pkg/apifrontend/{tools/ka_investigate_mcp,agent/{phase_guard,root,prompt.txt},launcher/reinvoking_runner,session/reinvoke}.go` (modified) | Go |
| Ported dependency | `test/services/mock-llm/{handlers/chain.go (new), types.go, config/overrides.go, registry_default.go, handlers/{gemini,openai}.go, response/gemini.go}` | #1853 N-deep chaining engine, ported from `main` |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/apifrontend/session/... -ginkgo.focus="1899"

# Integration tests
go test ./pkg/apifrontend/agent/... ./pkg/apifrontend/tools/... ./pkg/apifrontend/launcher/... -ginkgo.focus="1899"

# Full apifrontend regression
go test ./pkg/apifrontend/...

# E2E (requires a running/creatable fullpipeline Kind cluster)
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1899"
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|---|---|---|---|---|
| `interaction_mode` arg | LLM calls `kubernaut_investigate` | JSON-visible on `InvestigateMCPArgs` | IT-AF-1899-001 | Pass |
| Mode + flag persistence | `phaseGuardAfter` | `session.State` flags set | IT-AF-1899-002/003 (+b/c variants) | Pass |
| `checkpointToolFilter` | `NewRootAgent`'s `BeforeModelCallbacks` (`root.go`) | `LLMRequest.Tools` filtered | IT-AF-1899-004a..e | Pass |
| `phaseGuardBefore` backstop | `BeforeToolCallback` chain | Tool call hard-rejected | IT-AF-1899-005/005b/005c | Pass |
| Checkpoint clear | `reinvokingRunner.Run` entry | `AppendEvent`/`StateDelta` applied | IT-AF-1899-006/007 | Pass |
| Reinvocation gate | `reinvokingRunner.needsReinvocation` → `session.NeedsReinvocationCtx` | Reinvocation suppressed/allowed | UT-AF-1899-001..004 | Pass |
| Full journey (Phase 1→2) | Real A2A `message/stream` | Blocked, then completed on genuine turns | E2E-FP-1899-001 | Pass |
| Full journey (Phase 2→3) | Real A2A `message/stream` | Blocked, then completed on genuine turns | E2E-FP-1899-002 | Pass |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|---|---|---|---|
| `pkg/apifrontend/agent/phase_guard_test.go` (`UT-AF-1307-*`, `UT-AF-SESS-020-*`) | `kubernaut_investigate` calls with no mode field | Added `interaction_mode: "full_remediation"` where the test's original intent required phase 2 to proceed unblocked | New fail-safe `interactive` default would otherwise block these pre-existing tests' assumed behavior |
| `pkg/apifrontend/launcher/reinvoking_runner_test.go` (`UT-AF-REINV-002`) | Reinvocation fires without an explicit driver-active state | Added `session.StateKeyDriverActive: true` to test setup | New `driverActive` check in `NeedsReinvocationCtx` requires an explicit driver session, closing the literal #1899 repro |
| `pkg/apifrontend/session/reinvoke_test.go` (`UT-AF-230-008`) | Event-count assertion coupled to state setup | Refactored to a `fakeState` helper decoupling state manipulation from event counting | Adding state-dependent checks to `NeedsReinvocationCtx` incidentally affected an unrelated event-count assertion; decoupling restores test independence |
| `test/e2e/fullpipeline/{08,09}_af_a2a_*_test.go` | `fpWaitForRRWithTargetNS(nameSubstring, targetNS, timeout)` | Signature simplified to `fpWaitForRRWithTargetNS(targetNS, timeout)` (`nameSubstring` was always `"memory-eater"`) | Incidental helper cleanup while adding new call sites for the consent-gate tests |
| `test/e2e/fullpipeline/17_af_a2a_full_interactive_remediation_test.go` | Scenario declared no `interaction_mode` | `fullInteractiveRemediationScenarioYAML` now declares `interaction_mode: "full_remediation_autonomous"` | Without it, the new fail-safe `interactive` default would block this test's 4-deep auto-chain at `discover_workflows` |

---

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-08-03 | Initial test plan, documenting the completed DD-AF-011 implementation on `release/v1.5` |
