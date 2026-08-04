# Test Plan: Session-State Hygiene Fixes for AF Interactive Investigation (#1912, #1915)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1912-1915-v1.0
**Feature**: Two independent session-state bugs in the AF interactive-investigation harness, fixed together on `release/v1.5`: (1) `driverActive` never cleared after a session-terminal tool, causing incorrect reinvocation eligibility (#1912); (2) interactive-mode investigations wrongly report "no workflows found" instead of auto-discovering, because `prompt.txt` never surfaced the already-implemented `full_remediation` mode (#1915).
**Version**: 1.0
**Created**: 2026-08-04
**Author**: Cursor Agent (session-driven, user-directed)
**Status**: Complete
**Branch**: `fix/1912-1915-driver-active-full-remediation-v1.5` (based on `release/v1.5`)

---

## 1. Introduction

### 1.1 Purpose

Two independently-reported bugs in AF's DD-AF-011 interactive-investigation harness are fixed together in one PR, since both touch the same session-state machinery and the same test files:

1. **#1912**: `phaseGuardAfter`'s `isTerminal` branch (`pkg/apifrontend/agent/phase_guard.go`) clears the `ActiveContextRegistry` entry on a successful `kubernaut_complete`/`kubernaut_cancel`, but never resets `af_interactive_driver_active` (`session.StateKeyDriverActive`) itself. `NeedsReinvocationCtx` (`pkg/apifrontend/session/reinvoke.go`) reads that flag directly, so a session already genuinely completed or cancelled could still read as "driver active" — if no DD-AF-011 checkpoint remained blocked, a later text-only turn could be incorrectly reinvoked back into a closed session.
2. **#1915**: `prompt.txt`'s "Mode Detection" section never told the model that `full_remediation` mode exists, so a plain "investigate X" request always omitted `interaction_mode`, resolving to the fail-safe `interactive` default and removing `kubernaut_discover_workflows` from the model's tool list. The prompt's separate "CRITICAL — Phase 1 to Phase 2" section then unconditionally said "proceed to Phase 2" regardless, contradicting the harness. The model, unable to find the tool, incorrectly concluded "no matching workflows" instead of pausing correctly.

Both fixes are scoped to `release/v1.5` first (where production users are today), backported to `main` via existing tracking issues (#1913 for #1912, #1917 for #1915). See [DD-AF-011's Errata section](../../architecture/decisions/DD-AF-011-phase-transition-consent-guard.md#errata) for full root-cause analysis of both.

### 1.2 Objectives

1. **O1 (#1912)**: A successful `kubernaut_complete`/`kubernaut_cancel` clears `af_interactive_driver_active` (and, for hygiene, `af_active_rr_id`/`af_active_session_id`) alongside the existing `ActiveContextRegistry` clear.
2. **O2 (#1912)**: `NeedsReinvocationCtx` correctly returns `false` once `driverActive` is cleared, even if the session's CRD phase has not yet re-synced away from `Active`, and even when no DD-AF-011 checkpoint remains blocked.
3. **O3 (#1912, regression)**: A failed `kubernaut_complete`/`kubernaut_cancel` does NOT clear `driverActive` — the driver session is still legitimately active.
4. **O4 (#1915)**: `prompt.txt`'s Mode Detection section declares `interaction_mode: "full_remediation"` for the default plain-investigate trigger phrases, reserving bare `interactive` (RCA-only, no auto-discovery) for an explicit opt-out phrase ("just investigate", "investigate only", "don't suggest fixes").
5. **O5 (#1915)**: The "CRITICAL — Phase 1 to Phase 2" section and the (renamed) "Full Interactive Remediation — Autonomous Selection" section no longer contradict each other or the harness — tool availability is explicitly tied to the mode already declared, not framed as the model's free choice.
6. **O6 (#1915, no regression)**: The underlying `full_remediation` harness mechanism (auto-discover, pause before select) was already correct and tested (IT-AF-1899-002b/003/005b) — this fix is prompt-content-only, no harness/code changes.
7. **O7 (pyramid invariant, #1912)**: A real-production-wiring integration test (mirroring IT-AF-1776-001's established pattern) and an E2E journey both prove the fix holds against the actual `NewRootAgent`/`NewA2AHandler` stack, not just the isolated callback logic.
8. **O8 (no regression, both)**: E2E-FP-1899-001/002 (Phase 1→2 and Phase 2→3 consent-gate journeys) continue to pass unmodified — neither fix touches `checkpointToolFilter`, `phaseGuardBefore`, or `reinvoking_runner.go`'s `clearCheckpointFlags`.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/session/... ./pkg/apifrontend/agent/... -args -ginkgo.focus="1912\|1915"` |
| Integration test pass rate | 100% (execution blocked in this sandbox — see §10.3) | `go test ./test/integration/apifrontend/... -args -ginkgo.focus="1912"` |
| E2E journey pass rate | 1 new (#1912) + 0 new for #1915 (reuses E2E-FP-1899-002) + 0 regressions | `ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1912"` + full FP suite regression spot-check |
| Full `pkg/apifrontend` regression | 0 | `go test ./pkg/apifrontend/...` |
| DD-AF-011 regression | 0 | E2E-FP-1899-001/002 unmodified and still passing (structurally unaffected — see §4.2) |

---

## 2. References

### 2.1 Authority

- BR-INTERACTIVE-001: Interactive Investigation Sessions
- BR-SESS-020/022: `ActiveContextRegistry` multi-turn session continuity and idle-timeout tracking
- BR-SESS-013: Reinvocation loop for stalled mid-investigation turns
- Issue #1912: `driverActive` never cleared after a session-terminal tool (this plan, `release/v1.5`)
- Issue #1913: `main` tracking clone of #1912
- Issue #1915: interactive-mode investigations wrongly report "no workflows found" (this plan, `release/v1.5`)
- Issue #1917: `main`/v1.6 tracking clone of #1915
- Issue #1407: Progressive Flow — auto-proceed from investigation to discovery for audit completeness (the original design intent #1915's fix restores)
- [DD-AF-011](../../architecture/decisions/DD-AF-011-phase-transition-consent-guard.md): phase-transition consent guard — both bugs are gaps discovered in this design's implementation, documented in its Errata section
- [docs/testing/1899/TEST_PLAN.md](../1899/TEST_PLAN.md): DD-AF-011's own test plan — the mechanism this plan's #1915 fix re-exposes was already built and tested there

### 2.2 Cross-References

- `pkg/apifrontend/agent/phase_guard.go` (`phaseGuardAfter`'s `isTerminal` branch — #1912 fix location)
- `pkg/apifrontend/session/reinvoke.go` (`NeedsReinvocationCtx`, `driverActive` — #1912 regression proof)
- `pkg/apifrontend/agent/prompt.txt` (Mode Detection, CRITICAL Phase 1→2, Phase 2/3 sections — #1915 fix location)
- `test/integration/apifrontend/reinvocation_race_test.go` (IT-AF-1776-001 — the established real-production-wiring pattern the new #1912 IT test mirrors)
- `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` (E2E-FP-1899-001/002 — regression baseline; E2E-FP-1912-001 added here)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|---|---|---|---|---|---|
| R1 | Clearing `af_active_rr_id`/`af_active_session_id` alongside `driverActive` breaks some other code path that reads them after session termination | Regression in an unrelated feature reading stale-but-expected rr_id/session_id state | Low | Full `pkg/apifrontend/...` suite | `mcpDependentTools`'s `before()` check gates purely on `driverActive`, not on these companion keys being present — confirmed via code read before clearing them; full regression suite run to confirm |
| R2 | The #1915 prompt.txt rewrite accidentally breaks one of the ~15 existing `ContainSubstring`/`MatchRegexp`/`NotTo` assertions in `prompt_test.go` that pin exact wording (from #1407, #1430, #1899, #1332) | Silent regression in prompt-content test coverage for unrelated, previously-fixed issues | Medium (dense, overlapping substring constraints) | All pre-existing `prompt_test.go` specs | Full existing `prompt_test.go` inventory read before editing; every constrained substring (`"Proceed to Phase 2"`, `"just investigate"`/`"investigate only"`, `"Do NOT call kubernaut_discover_workflows"`, `"self-resolved"`, `interaction_mode: "full_remediation_autonomous"`, `"consent gate will block workflow discovery/selection"`, `omit interaction_mode`, the `MUST automatically proceed to Phase 2.*without waiting` negative-regex) individually traced and preserved verbatim; full suite run green after the edit (187/187 passing) |
| R3 | Full `pkg/apifrontend` integration test suite (which includes the new IT-AF-1912-001) and the FP E2E suite both require Podman-built container images (Mock LLM, DataStorage, KA) via a shared `SynchronizedBeforeSuite` — unavailable in this sandbox (nested virtualization does not boot the Podman machine here) | New IT/E2E tests could not be execution-verified in this development session | Confirmed (environment limitation, not a code defect) | IT-AF-1912-001, E2E-FP-1912-001 | Both tests were written to mirror an already-established, presumably-passing pattern (IT-AF-1776-001 for the IT tier; `consentGatePhase2/3AttemptScenarioYAML` conventions for the E2E tier) exactly; both `go build`/`go vet` clean; logic independently traced against the actual production source (`phase_guard.go`, `reinvoke.go`, `ka_investigate_mcp.go`, `ka_interactive.go`) line by line. Flagged explicitly for CI verification before merge — see §10.3 |
| R4 | #1915's mechanism (`full_remediation` mode) might have actually been genuinely untested prior to this fix, making the fix riskier than assessed | Fix could regress an unknown-unknown behavior | Low (disproven) | IT-AF-1899-002b/003/005b | Preflight initially mis-assessed this as "zero test references" (grepped only for the Go constant name `InteractionModeFullRemediation`, missing the string-literal `"full_remediation"` usages) — corrected during implementation: `full_remediation`'s own harness gating (auto-discover, pause-before-select) was already thoroughly covered by 3 pre-existing #1899 IT tests. This lowered, not raised, the risk profile of the #1915 fix (pure prompt-content change against an already-correct, already-tested mechanism) |

### 3.1 Risk-to-Test Traceability

R2 is the highest-density risk given how many prior issues' fixes left exact-substring assertions embedded in the same file being edited; it was closed by an explicit pre-edit inventory (this plan's own research phase) rather than discovered as a CI surprise, and confirmed by a full 187/187 green run plus a deliberate RED verification (temporarily reverting `prompt.txt` to its pre-fix content and confirming all 4 new UT-AF-1915-* assertions fail, then restoring and confirming green again). R3 is disclosed rather than silently worked around; R4 corrects and documents a preflight assessment error found mid-implementation, per this project's confidence-gate methodology.

---

## 4. Scope

### 4.1 Features to be Tested

- **`driverActive` clearing on session-terminal tools** (`pkg/apifrontend/agent/phase_guard.go`): `phaseGuardAfter`'s `isTerminal` branch.
- **`NeedsReinvocationCtx` correctness post-clear** (`pkg/apifrontend/session/reinvoke.go`): no regression to existing checkpoint/driver-active logic.
- **`prompt.txt` Mode Detection, CRITICAL Phase 1→2, Phase 2, and Phase 3 sections** (`pkg/apifrontend/agent/prompt.txt`): the #1915 rewrite.
- **New IT-AF-1912-001**: real production wiring (`agentpkg.NewRootAgent`, `launcher.NewA2AHandler`) proof that a session ended by `kubernaut_complete` is never reinvoked.
- **New E2E-FP-1912-001**: full A2A/K8s stack proof of the same, using the established consent-gate test conventions.

### 4.2 Features Not to be Tested

- **The `main` branch ports (#1913, #1917)**: tracked separately; this plan covers `release/v1.5` verification only.
- **DD-AF-011's own consent-gate mechanism (`checkpointToolFilter`, `phaseGuardBefore`, `clearCheckpointFlags`)**: unmodified by either fix; re-verified only via the existing E2E-FP-1899-001/002 regression baseline, not retested from scratch here.
- **#1496 (`kubernaut_complete_no_action` registry-clear gap)**: investigated as part of this session's preflight, found to be a separate, already client-mitigated (Console #29/DD-008) issue on both branches; deliberately left untracked/deferred per explicit confirmation, not part of this fix.
- **Live LLM provider behavior for #1915**: per the established convention in this test suite (see `prompt_test.go`'s own #1899 doc comment: "a live-LLM behavioral guarantee is out of scope for a prompt-content test"), the prompt fix is verified via content assertions, not a live model call.

### 4.3 Design Decisions

| Decision | Rationale |
|---|---|
| Clear `driverActive` unconditionally in the `isTerminal` branch, independent of `registry != nil` | The registry-clear block is gated on `registry != nil` for backward compatibility (BR-SESS-020-025), but `driverActive` is read directly by `NeedsReinvocationCtx` regardless of whether a registry is configured — gating the fix on registry presence would leave the bug live in any deployment without an `ActiveContextRegistry` configured |
| Also clear `af_active_rr_id`/`af_active_session_id` for hygiene, not strictly required to fix #1912 | `mcpDependentTools`'s `before()` check gates purely on `driverActive`, so leaving these stale would be harmless functionally — but leaving them set past session termination is confusing for any future debugging/observability and costs nothing to clear |
| #1915 fix is prompt-content-only, no harness/code changes | The underlying `full_remediation` mechanism was already correct and tested (IT-AF-1899-002b/003/005b, discovered during a corrected preflight pass) — the gap was purely that `prompt.txt` never instructed the model to use it |
| Plain "investigate" defaults to `full_remediation` (not a fresh consent prompt asking the user to opt in) | Confirmed with the maintainer: presenting discovered workflows for the user to choose from is not itself a consequential action requiring a fresh gate — only executing a workflow is. This also restores the original #1407 "progressive flow" design intent (auto-proceed to discovery for audit completeness) that #1899's later, stricter default had inadvertently suppressed for the common case |
| No new E2E scenario written specifically for #1915 | The `full_remediation` harness mechanism is already proven end-to-end by the pre-existing E2E-FP-1899-002; the prompt-instruction-to-live-LLM-decision link cannot be exercised by the mock-LLM E2E infrastructure (mock-LLM scripts exact tool-call arguments directly, bypassing prompt reasoning entirely) |
| New #1912 tests span UT, a real-production-wiring IT (not just an isolated-callback IT), and E2E | #1912 is specifically about behavior at the seam between two components (`phase_guard.go`'s state-write and `reinvoke.go`'s state-read) that only manifests correctly when wired through the real ADK agent loop — an isolated-callback-only UT could pass while the real wiring still had a gap, so the pyramid invariant's IT tier was deliberately elevated to real-wiring here (mirroring IT-AF-1776-001, a previous fix for the same seam) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of the new logic paths in `phase_guard.go`'s `isTerminal` branch and `prompt.txt`'s content assertions.
- **Integration**: the #1912 wiring point (`phaseGuardAfter` → `NeedsReinvocationCtx`, both reached only through the real ADK agent loop) has a passing IT proving production wiring, per the Wiring Manifest below.
- **E2E**: #1912's user-facing consequence (a closed session must never be resurrected into a consequential action) is proven end-to-end against the real AF binary and real FP mock-LLM. #1915 relies on the pre-existing E2E-FP-1899-002 journey (no new scenario needed — see §4.3).

### 5.2 Two-Tier Minimum (Pyramid Invariant)

#1912: UT (logic) + IT (real wiring) + E2E (journey) all provided. #1915: UT (prompt content) + IT (traceability cross-reference to pre-existing #1899 IT coverage) + E2E (reuses pre-existing #1899 journey) — no tier skipped; the E2E tier's specific test asset is shared with #1899 by design (see §4.3), not omitted.

### 5.3 Pass/Fail Criteria

**PASS**: all new UT pass (confirmed, executed); the full pre-existing `pkg/apifrontend/agent/...` and `pkg/apifrontend/session/...` suites pass with zero regressions (confirmed, executed); the new IT/E2E tests build cleanly (`go build`/`go vet`, confirmed) and are logically traced against production source, pending CI execution (see R3/§10.3); E2E-FP-1899-001/002 remain unmodified (confirmed via diff).

**FAIL**: any new UT fails, any pre-existing test regresses, or the new IT/E2E tests fail once CI infrastructure is available.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `pkg/apifrontend/agent/phase_guard.go` | `newPhaseGuard`'s `after` closure, `isTerminal` branch | ~10 (new) |
| `pkg/apifrontend/session/reinvoke.go` | `NeedsReinvocationCtx`, `driverActive` (regression proof only, unmodified) | — |
| `pkg/apifrontend/agent/prompt.txt` | Mode Detection, CRITICAL Phase 1→2, Phase 2/3 sections (content assertions via `prompt_test.go`) | ~40 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `test/integration/apifrontend/reinvocation_terminal_test.go` (new) | `IT-AF-1912-001`, real `agentpkg.NewRootAgent`/`launcher.NewA2AHandler` wiring | ~220 |

### 6.3 E2E-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` | New `Describe` block, `E2E-FP-1912-001` | ~85 |
| `test/infrastructure/shared_e2e.go` | `noReinvocationAfterCompleteScenarioYAML` (new) | ~35 |
| `test/infrastructure/fullpipeline_e2e.go` | `afRemediateNS` map (1 new key: `terminal-1912`) | ~5 |

---

## 7. BR / FedRAMP Coverage Matrix

| Authority | Description | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-SESS-020/022, AC-2 | `driverActive` cleared on `kubernaut_complete`/`kubernaut_cancel` success, alongside the registry | P0 | Unit | UT-AF-1912-001/002 | Pass |
| BR-SESS-020/022, AC-2 | `driverActive` NOT cleared on `kubernaut_complete` failure — driver session still legitimately active | P0 | Unit | UT-AF-1912-003 | Pass |
| BR-SESS-013, AC-2, AC-6 | `NeedsReinvocationCtx` does not nudge once `driverActive` is cleared, even if phase still reads `Active` | P0 | Unit | UT-AF-1912-004 | Pass |
| BR-SESS-013, AC-2, AC-6 | Real production wiring: a session ended by `kubernaut_complete` is never reinvoked, even with no DD-AF-011 checkpoint left blocking | P0 | Integration | IT-AF-1912-001 | Written, build-clean; execution pending CI (R3) |
| BR-SESS-013, AC-2, AC-6 | Full journey: investigate + complete in one turn reaches a clean terminal RR state with no `WorkflowExecution` ever created | P0 | E2E | E2E-FP-1912-001 | Written, build-clean; execution pending CI (R3) |
| BR-INTERACTIVE-001, AU-3, SI-4 | Mode Detection declares `full_remediation` for the default plain-investigate trigger | P0 | Unit | UT-AF-1915-001 | Pass |
| BR-INTERACTIVE-001, AC-6 | An explicit RCA-only opt-out phrase still maps to omitted `interaction_mode` (bare `interactive`) | P0 | Unit | UT-AF-1915-002 | Pass |
| SI-4 | CRITICAL Phase 1→2 section ties the `discover_workflows` decision to the already-declared mode, not the model's free choice | P0 | Unit | UT-AF-1915-003 | Pass |
| SI-4 | Autonomous-interactive sub-case is unambiguously distinct from the new `full_remediation` default | P1 | Unit | UT-AF-1915-004 | Pass |
| BR-INTERACTIVE-001, AC-6 (traceability) | `full_remediation` auto-discovers but still pauses before select — BR/audit cross-reference to pre-existing #1899 mechanism coverage | P1 | Integration | IT-AF-1915-001 | Pass |
| AC-6 (regression) | E2E-FP-1899-001/002 (Phase 1→2, Phase 2→3 consent gates) unmodified and still passing | P0 | E2E | E2E-FP-1899-001/002 | Unmodified (diff-confirmed); execution pending CI (R3) |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `UT-AF-1912-001` | Clears `driverActive` on `kubernaut_complete` success, alongside the registry | Pass |
| `UT-AF-1912-002` | Clears `driverActive` on `kubernaut_cancel` success, alongside the registry | Pass |
| `UT-AF-1912-003` | Does NOT clear `driverActive` on `kubernaut_complete` failure | Pass |
| `UT-AF-1912-004` | `NeedsReinvocationCtx` does not nudge once `driverActive` is cleared, even if phase still reads `Active` | Pass |
| `UT-AF-1915-001` | Mode Detection declares `full_remediation` for the default plain-investigate trigger | Pass |
| `UT-AF-1915-002` | An explicit RCA-only opt-out phrase still maps to omitted `interaction_mode` | Pass |
| `UT-AF-1915-003` | CRITICAL Phase 1→2 section explains tool availability is harness-gated by the already-declared mode | Pass |
| `UT-AF-1915-004` | Autonomous-interactive sub-case is unambiguously distinct from the new `full_remediation` default | Pass |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `IT-AF-1912-001` | Real production wiring: investigate (`full_remediation_autonomous`) → complete → text-only turn does not trigger a 4th (reinvoked) LLM call | Written, build-clean; CI execution pending (R3) |
| `IT-AF-1915-001` | `full_remediation` auto-discovers but still pauses before select (BR/audit traceability for #1915, mechanism already proven by IT-AF-1899-002b/003) | Pass |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `E2E-FP-1912-001` | Investigate (`full_remediation_autonomous`) + complete in one turn reaches a clean terminal RR state with no `WorkflowExecution` ever created — no errant reinvocation resurrects the closed session | Written, build-clean; CI execution pending (R3) |

### Tier Skip Rationale

No tier is skipped for #1912. For #1915, the E2E tier deliberately reuses the pre-existing E2E-FP-1899-002 journey rather than adding a new scenario — see Design Decision in §4.3 (the prompt-instruction-to-live-LLM-decision link is untestable via the mock-LLM E2E infrastructure by construction, consistent with every other `prompt.txt` directive in this suite).

---

## 9. Test Cases (P0 detail)

### IT-AF-1912-001: No reinvocation after `kubernaut_complete`, real production wiring

**BR**: BR-SESS-013, AC-2, AC-6
**Priority**: P0
**Type**: Integration
**File**: `test/integration/apifrontend/reinvocation_terminal_test.go`

**Test Steps**:
1. **Given**: a real `agentpkg.NewRootAgent` + `launcher.NewA2AHandler` stack (mirroring `IT-AF-1776-001`'s established pattern), backed by an `httptest`-served mock LLM scripted to return exactly 3 turns: (1) `kubernaut_investigate` declaring `interaction_mode: "full_remediation_autonomous"`, (2) `kubernaut_complete`, (3) plain closing text.
2. **When**: a single `message/send` A2A call is made.
3. **Then**: the task completes successfully with the turn-3 closing text as the final artifact, and exactly 3 LLM calls were made — no 4th (reinvoked) call.

**Acceptance Criteria**: pre-fix, `driverActive` would remain stuck `true` after `kubernaut_complete` with no DD-AF-011 checkpoint left blocking (`full_remediation_autonomous` clears both `Phase2Blocked` and, since `discover_workflows` is never called, `Phase3Blocked` is never set) — the ONLY signal left to prevent an errant reinvocation. This isolates the #1912 defect precisely.

### E2E-FP-1912-001: No Reinvocation After Session-Terminal Tool

**BR**: BR-SESS-013, AC-2, AC-6
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go`

**Test Steps**:
1. **Given**: an isolated namespace with a zero-replica `memory-eater` `Deployment`, and a mock-LLM keyword scenario (`af_no_reinvocation_after_complete_1912`) scripted to chain `kubernaut_remediate` → `kubernaut_investigate` (declaring `interaction_mode: "full_remediation_autonomous"`) → `kubernaut_complete`.
2. **When**: a single A2A message ("create and investigate then complete and go silent...") is sent.
3. **Then**: the turn completes gracefully (HTTP 200, no JSON-RPC error); a `RemediationRequest` is created and reaches a clean state; polling confirms **no `WorkflowExecution` exists** for that RR.

**Acceptance Criteria**: even in the worst case where an errant reinvocation were to fire, it must never cause a consequential action (no `WorkflowExecution` ever appears) — the real AF/A2A stack's session-terminal handling is safe end to end.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: none — in-process fakes (`mapState`, `fakeState`, `statefulToolContext`) already established by the #1899/#1307/#1446 test suites in the same files
- **Location**: `pkg/apifrontend/{session,agent}/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD, real `agentpkg.NewRootAgent`/`launcher.NewA2AHandler` production wiring, `httptest`-served mock LLM (no external dependencies)
- **Location**: `test/integration/apifrontend/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD, real Kind-based `fullpipeline` cluster infrastructure (`test/infrastructure/fullpipeline_e2e*.go`)
- **Dependency**: real AF binary, real ADK tool-calling loop, real mock-LLM binary; Podman for building the Mock LLM/DataStorage/KA container images used by the shared `SynchronizedBeforeSuite`
- **Isolation**: dedicated namespace (`terminal-1912`) per the established per-test isolation convention
- **Known limitation (this session)**: both `IT-AF-1912-001`'s package (`test/integration/apifrontend`) and the FP E2E suite share a `SynchronizedBeforeSuite` that requires Podman to build container images. In this development sandbox, the Podman machine (AppleHV) repeatedly failed to boot (`connection refused` after `podman machine start` reports success — consistent with nested-virtualization not being available in this environment), so neither `IT-AF-1912-001` nor `E2E-FP-1912-001` could be execution-verified here. Both were instead verified by: (a) `go build`/`go vet` passing cleanly, (b) line-by-line tracing against the actual production source they exercise, and (c) structural pattern-matching against an already-established, presumably-passing sibling test (`IT-AF-1776-001` for the IT tier; `consentGatePhase2/3AttemptScenarioYAML` conventions for the E2E tier). **Recommendation**: run `make test-integration-apifrontend` and `make test-e2e-fullpipeline` in CI (or any environment with working Podman/Kind) before merge.

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None new. Both fixes build entirely on already-merged DD-AF-011 infrastructure (`phase_guard.go`, `reinvoke.go`, `checkpointToolFilter`, the mock-LLM `NextToolCall` chaining engine already ported to `release/v1.5` for #1899).

### 11.2 Execution Order

1. **DISCOVERY**: re-verified #1915's `full_remediation` mechanism test coverage (corrected an initial preflight under-assessment — see R4); confirmed `E2E-FP-1899-001/002` are structurally unaffected by both fixes.
2. **RED**: failing UT written first for #1912 (`UT-AF-1912-001/002` against `phase_guard_test.go`) and #1915 (`UT-AF-1915-001..004` against `prompt_test.go`, confirmed failing against the pre-fix `prompt.txt` via a temporary revert-and-restore).
3. **GREEN**: `phase_guard.go`'s `isTerminal` branch and `prompt.txt`'s Mode Detection/CRITICAL/Phase 2/3 sections updated; full existing suites re-run green (187/187 in `pkg/apifrontend/agent`).
4. **REFACTOR**: N/A for #1912 (minimal, targeted fix, nothing left to clean up); N/A for #1915 (prompt-content rewrite is the change itself, not a post-hoc cleanup of GREEN-phase code).
5. **Verification**: full `pkg/apifrontend/...` suite green; new IT/E2E written and build-clean, flagged for CI execution (R3).

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|---|---|---|
| This test plan | `docs/tests/1912-1915/TEST_PLAN.md` | Strategy and test design |
| Design decision errata | `docs/architecture/decisions/DD-AF-011-phase-transition-consent-guard.md` (Errata section) | Root-cause analysis for both gaps found in the original DD-AF-011 implementation |
| Unit tests | `pkg/apifrontend/agent/phase_guard_test.go`, `pkg/apifrontend/session/reinvoke_test.go`, `pkg/apifrontend/agent/prompt_test.go` | Ginkgo BDD |
| Integration tests | `test/integration/apifrontend/reinvocation_terminal_test.go` (new) | Ginkgo BDD |
| E2E tests | `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` (extended) | Ginkgo BDD, real FP cluster |
| Production code | `pkg/apifrontend/agent/phase_guard.go` (modified), `pkg/apifrontend/agent/prompt.txt` (modified) | Go / prompt text |
| E2E infrastructure | `test/infrastructure/shared_e2e.go`, `test/infrastructure/fullpipeline_e2e.go` (both modified) | Go |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/apifrontend/agent/... ./pkg/apifrontend/session/... -args -ginkgo.focus="1912|1915"

# Full apifrontend agent/session regression
go test ./pkg/apifrontend/agent/... ./pkg/apifrontend/session/...

# Integration tests (requires Podman + envtest; see §10.3)
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.35.0 -p path)" \
  go test ./test/integration/apifrontend/... -args -ginkgo.focus="IT-AF-1912"

# E2E (requires a running/creatable fullpipeline Kind cluster + Podman; see §10.3)
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1912"

# DD-AF-011 regression check (must still pass unmodified)
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1899"
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|---|---|---|---|---|
| `driverActive` clear on session-terminal tool | `phaseGuardAfter`, registered on `NewRootAgent`'s `AfterToolCallbacks` (`root.go`) | `session.State` flag cleared | UT-AF-1912-001/002 (isolated callback) + IT-AF-1912-001 (real `NewRootAgent`/`NewA2AHandler` wiring) | UT: Pass. IT: written, build-clean, CI execution pending (R3) |
| `NeedsReinvocationCtx` consults cleared flag | `reinvokingRunner`'s reinvocation check → `session.NeedsReinvocationCtx` | Reinvocation suppressed | UT-AF-1912-004 | Pass |
| `full_remediation` mode declaration | Mode Detection section of `prompt.txt`, consumed by the LLM when constructing the `kubernaut_investigate` call | `interaction_mode` argument on the tool call | UT-AF-1915-001 (content assertion) — live-LLM behavioral proof out of scope, per established convention | Pass |
| `full_remediation`'s harness gating (pre-existing, unmodified) | `phaseGuardAfter`'s `isEntry`/`isDiscoverWorkflows` branches | `Phase2Blocked`/`Phase3Blocked` flags | IT-AF-1899-002b/003/005b (pre-existing) + IT-AF-1915-001 (traceability cross-reference) | Pass |
| Full journey (#1912) | Real A2A `message/send` | Clean terminal RR, no `WorkflowExecution` | E2E-FP-1912-001 | Written, build-clean, CI execution pending (R3) |
| Full journey (#1915, reused) | Real A2A `message/send` | `full_remediation` auto-chains through discovery, pauses before select | E2E-FP-1899-002 (pre-existing, unmodified) | Pass (unmodified; re-run recommended in CI per R3) |

---

## 15. Existing Tests Requiring Updates

None. Both fixes were designed specifically to avoid touching any pre-existing test's assertions:

- #1912's fix only adds new state-clearing side effects in a branch (`isTerminal`) that no pre-existing test asserted the *absence* of this behavior for — confirmed by the full `pkg/apifrontend/...` suite passing unmodified.
- #1915's `prompt.txt` rewrite was constrained line-by-line against every existing `prompt_test.go` substring assertion (§3, R2) — zero pre-existing assertions required modification; only new `Describe` blocks were added.

---

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-08-04 | Initial test plan, documenting the completed #1912 + #1915 fixes on `release/v1.5` |
