# Test Plan: Session-State Hygiene Fixes for AF Interactive Investigation (#1912, #1915) — `main` Port

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1912-1915-main-v1.0
**Feature**: Backport of two independent session-state bugs, originally fixed and fully TDD-verified on `release/v1.5` (PR #1920): (1) `driverActive` never cleared after a session-terminal tool, causing incorrect reinvocation eligibility (#1912, tracked on `main` by #1913); (2) interactive-mode investigations wrongly report "no workflows found" instead of auto-discovering, because `prompt.txt` never surfaced the already-implemented `full_remediation` mode (#1915, tracked on `main` by #1917).
**Version**: 1.0
**Created**: 2026-08-04
**Author**: Cursor Agent (session-driven, user-directed)
**Status**: Complete
**Branch**: `fix/1912-1915-driver-active-full-remediation-main` (based on `main`)

---

## 1. Introduction

### 1.1 Purpose

This plan documents the `main` port of the two fixes already fully designed, implemented, and tested on `release/v1.5` (see [`release/v1.5`'s own TP-1912-1915-v1.0](https://github.com/jordigilh/kubernaut/blob/release/v1.5/docs/tests/1912-1915/TEST_PLAN.md) for the original root-cause analysis, risk assessment, and BR/FedRAMP matrix — not duplicated here). This plan's scope is narrower and specific to the port: **what differs structurally between `main` and `release/v1.5` for these two fixes, and why the port is not a straight cherry-pick.**

1. **#1912 port**: `main`'s `phase_guard.go` was refactored into small named helper functions during the #1901 port of DD-AF-011 (`phaseGuardBefore`, `phaseGuardAfter`, `recordDriverEntryState`, `recordInteractionMode`, `storeActiveRRID`, `syncActiveContextRegistry`, `toolCallSucceeded`, `refreshActiveContext`, `recordDiscoverWorkflowsCheckpoint`, `driverIsActive`, `injectStoredRRID`) — a structurally different (though behaviorally equivalent) shape from `v1.5`'s single inline closure. The fix itself (clear `driverActive`/`af_active_rr_id`/`af_active_session_id` on a successful session-terminal tool) is ported as a new named helper (`clearDriverSessionState`) called from `phaseGuardAfter`'s `isTerminal` branch, consistent with `main`'s existing decomposition style — not as a verbatim copy of `v1.5`'s inline block.
2. **#1915 port**: `main`'s `prompt.txt` never received the #1915 fix at all (unlike #1899/DD-AF-011, which *was* ported via #1901). Preflight confirmed `main`'s pre-port Mode Detection baseline was itself independently under-instructed: its "Interactive Mode (human in the loop)" section correctly never declared `interaction_mode` for a plain "investigate" request (matching this Decision's AC-6 fail-safe), but the shared "CRITICAL — Phase 1 to Phase 2" section still unconditionally said "proceed to Phase 2 without waiting" — the identical #1915 contradiction found on `v1.5` pre-fix. `main` additionally had a design quirk not present on `v1.5`: its "Full Interactive Remediation (investigate AND fix with user oversight)" section conflated the interactive-pause and autonomous-interactive sub-cases into one ambiguous block ("In autonomous-interactive mode (user said 'fix' but context implies oversight)..."). The port resolves this by applying `v1.5`'s exact proven disambiguation (plain "investigate"/"investigate and fix" phrasing → `full_remediation` default; explicit RCA-only opt-out; unambiguous `full_remediation_autonomous` sub-case) rather than preserving `main`'s own ambiguous baseline framing.

Both ports land in a single PR against `main`, mirroring the original `v1.5` PR's scope (both fixes touch the same session-state test files).

### 1.2 Objectives

Identical business objectives to the `v1.5` plan (O1–O8); restated here only where the port introduces a `main`-specific nuance:

1. **O1–O3 (#1912)**: identical to `v1.5` — `driverActive` cleared on session-terminal success, not on failure, and `NeedsReinvocationCtx` respects the cleared flag regardless of stale CRD phase.
2. **O1b (#1912, main-specific)**: the fix must apply uniformly across every entry in `main`'s `sessionTerminalTools` map, which — unlike `v1.5` — additionally includes `kubernaut_complete_no_action` (`main`-only, from #1496). No special-casing is introduced; `clearDriverSessionState` is called unconditionally in the shared `isTerminal` branch, so it covers `kubernaut_complete_no_action` for free.
3. **O4–O6 (#1915)**: identical business intent to `v1.5` — `full_remediation` becomes the prompt-declared default for plain investigate requests, RCA-only remains an explicit opt-out, and the fix is prompt-content-only.
4. **O4b (#1915, main-specific)**: the port must additionally verify against `main`'s own pre-existing `prompt_test.go` inventory (18 pre-existing `Describe` blocks / ~50 `It`s spanning #1275, #1332, #1407, #1408, #1430, #1899), which is a strict superset of the assertions `v1.5`'s R2 risk already required tracing — confirmed identical exact-substring dependencies apply (`"Proceed to Phase 2"`, `"just investigate"`/`"investigate only"`, `"Do NOT call kubernaut_discover_workflows"`, `"self-resolved"`, `interaction_mode: "full_remediation_autonomous"`, `"consent gate will block workflow discovery/selection"`, `omit interaction_mode`, `"Autonomous mode"`, `"highest-confidence workflow"`, the `MUST automatically proceed to Phase 2.*without waiting` negative-regex).
5. **O7 (pyramid invariant, #1912)**: the IT test (`IT-AF-1912-001`) is re-derived against `main`'s actual API surface — `main` uses `types.LLMConfig`/`types.LLMProviderGemini` (`pkg/shared/types`) where `v1.5` used `config.LLMConfig`/`config.LLMProviderGemini` (`pkg/apifrontend/config`) for `launcher.NewModelFromConfig`. The new file also reuses `main`'s pre-existing `reinvocationTaskResult`/`reinvocationRPCResponse`/`reinvocationArtifactText` types (already defined in `reinvocation_race_test.go`, same `apifrontend_test` package) instead of redefining them, avoiding a duplicate-symbol compile error.
6. **O8 (no regression, both)**: `main`'s pre-existing E2E-FP-1899-001/002 (ported via #1901) continue to pass unmodified.

### 1.3 Success Metrics

Identical to `v1.5`'s plan, re-run against `main`:

| Metric | Target | Measurement |
|---|---|---|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/session/... ./pkg/apifrontend/agent/... -args -ginkgo.focus="1912\|1915"` |
| Full `pkg/apifrontend` regression | 0 | `go test ./pkg/apifrontend/...` (23 packages, confirmed green) |
| `go build ./...` / `go vet` on new IT/E2E files | 0 errors | confirmed clean against `main`'s actual API surface |
| Integration/E2E test pass rate | 100% (execution blocked in this sandbox — see §10.3, same Podman limitation as the `v1.5` session) | `go test ./test/integration/apifrontend/... -args -ginkgo.focus="1912"` |
| DD-AF-011 regression | 0 | E2E-FP-1899-001/002 unmodified (diff-confirmed) |

---

## 2. References

### 2.1 Authority

- Issue #1912 (original, `release/v1.5`) / Issue #1913 (`main` tracking clone — this plan)
- Issue #1915 (original, `release/v1.5`) / Issue #1917 (`main`/v1.6 tracking clone — this plan)
- [`release/v1.5`'s TP-1912-1915-v1.0](https://github.com/jordigilh/kubernaut/blob/release/v1.5/docs/tests/1912-1915/TEST_PLAN.md) — full root-cause analysis, risk assessment, and BR/FedRAMP matrix (not duplicated here)
- PR #1920 — the original `release/v1.5` fix, merged
- [DD-AF-011](../../architecture/decisions/DD-AF-011-phase-transition-consent-guard.md) — Errata section documents both bugs and this port

### 2.2 Cross-References (main-specific locations)

- `pkg/apifrontend/agent/phase_guard.go` — `clearDriverSessionState` (new helper), called from `phaseGuardAfter`'s `isTerminal` branch
- `pkg/apifrontend/session/reinvoke.go` — byte-identical to `v1.5`; no code change needed, only the regression test (`UT-AF-1912-004`)
- `pkg/apifrontend/agent/prompt.txt` — Mode Detection, CRITICAL Phase 1→2, Phase 2/3 sections rewritten to `v1.5`'s proven post-fix wording
- `test/integration/apifrontend/reinvocation_terminal_test.go` (new on `main`) — adapted to `types.LLMConfig`, reuses `reinvocation_race_test.go`'s existing helper types
- `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go`, `test/infrastructure/{shared_e2e,fullpipeline_e2e}.go` — byte-identical port (these files did not structurally diverge between branches)

---

## 3. Risks & Mitigations (port-specific)

| ID | Risk | Impact | Probability | Mitigation |
|---|---|---|---|---|
| R1 | `main`'s refactored `phase_guard.go` might have subtly different control flow than `v1.5`'s inline closure, such that a naive line-for-line copy compiles but behaves differently | Silent behavioral regression | Low | Read `main`'s full `phase_guard.go` before editing; confirmed `isTerminal`/`isEntry`/`isDiscoverWorkflows` dispatch is logically identical to `v1.5`, just extracted into named functions; fix added as a new named helper following the same pattern, called at the same logical point (`isTerminal` branch) |
| R2 | `main`'s `prompt_test.go` has 18 pre-existing `Describe` blocks (more than `v1.5` had at the time of the original fix, since `main` and `v1.5` have independently accumulated some prompt-content tests) — a wording change could regress an assertion never encountered on `v1.5` | Silent regression in unrelated prompt-content coverage | Medium | Full inventory of `main`'s `prompt_test.go` read and every substring/regex dependency traced (see §1.1) before editing; full suite (195 specs) run green after the edit |
| R3 | `main`'s `InvokeActionArgs`/`InvokeActionResult`/`launcher.A2AConfig`/`launcher.NewModelFromConfig` might have diverged from `v1.5`'s shapes in ways the new IT test needs to account for | Compile failure or silently wrong mock behavior | Low (disproven) | Each type/function signature explicitly checked against `main`'s source (`ka/config.go`, `launcher/launcher.go`, `launcher/model.go`) before writing the IT test; `go vet` confirms clean compile |
| R4 | Podman unavailable in this sandbox (same nested-virtualization constraint as the original `v1.5` session) | New IT/E2E tests not execution-verified locally | Confirmed (environment limitation) | Same mitigation as `v1.5`: `go build`/`go vet` clean, logic traced against production source, structurally mirrors the already-established `IT-AF-1776-001` pattern; flagged for CI verification |

---

## 4. Scope

### 4.1 Features to be Tested (main port)

- `clearDriverSessionState` (new helper in `phase_guard.go`), wired into `phaseGuardAfter`'s `isTerminal` branch.
- `NeedsReinvocationCtx` regression proof (`reinvoke.go`, unmodified — `UT-AF-1912-004` only).
- `prompt.txt`'s rewritten Mode Detection / CRITICAL / Phase 2/3 sections, verified against the full `main` `prompt_test.go` inventory (not just the `v1.5`-known subset).
- `IT-AF-1912-001` re-derived against `main`'s `types.LLMConfig` API.
- `E2E-FP-1912-001`, `noReinvocationAfterCompleteScenarioYAML`, `terminal-1912` namespace — byte-identical port (no structural divergence in these files).

### 4.2 Features Not to be Tested

- The original `release/v1.5` root-cause analysis and design rationale — already covered by the `v1.5` plan (§2.1 reference); not re-derived here.
- `main`'s own #1901 (DD-AF-011/#1899) mechanism — unmodified by this port, re-verified only via the pre-existing E2E-FP-1899-001/002 regression baseline.

---

## 5. Approach

Identical two-tier-minimum (pyramid invariant) approach as `v1.5`: #1912 gets UT + IT (real wiring) + E2E; #1915 gets UT (prompt content) + IT (traceability cross-reference to pre-existing #1899 IT coverage on `main`) + E2E (reuses `main`'s pre-existing E2E-FP-1899-002, no new scenario needed).

**Pass/Fail Criteria**: identical to `v1.5` — all new UT pass (confirmed, executed: 195/195 in `pkg/apifrontend/agent`, 123/123 in `pkg/apifrontend/session`); full pre-existing `pkg/apifrontend/...` suite passes with zero regressions (confirmed, executed, 23/23 packages); new IT/E2E build cleanly (`go build`/`go vet`, confirmed) and are logically traced against `main`'s actual production source, pending CI execution (R4).

---

## 6. BR / FedRAMP Coverage Matrix

Identical BR/control mapping to `v1.5`'s plan §7 (BR-SESS-020/022, BR-SESS-013, BR-INTERACTIVE-001, AC-2, AC-6, SI-4, SI-10) — not restated here; the business requirements and control objectives are unchanged by the port, only the file locations and exact code shape differ (§2.2 above).

## 7. Test Scenarios

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
| `IT-AF-1912-001` | Real production wiring (`main`'s `types.LLMConfig` API): investigate (`full_remediation_autonomous`) → complete → text-only turn does not trigger a 4th (reinvoked) LLM call | Written, build-clean (`go vet` confirmed); CI execution pending (R4) |
| `IT-AF-1915-001` | `full_remediation` auto-discovers but still pauses before select (`phase_guard_test.go`, BR/audit traceability cross-reference to pre-existing `main` #1899 IT coverage, ported verbatim from `v1.5`) | Pass |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| `E2E-FP-1912-001` | Investigate (`full_remediation_autonomous`) + complete in one turn reaches a clean terminal RR state with no `WorkflowExecution` ever created | Written, build-clean; CI execution pending (R4) |

---

## 8. Execution

```bash
# Unit tests
go test ./pkg/apifrontend/agent/... ./pkg/apifrontend/session/... -args -ginkgo.focus="1912|1915"

# Full apifrontend regression
go test ./pkg/apifrontend/...

# Integration tests (requires Podman + envtest; see §3 R4)
go test ./test/integration/apifrontend/... -args -ginkgo.focus="IT-AF-1912"

# E2E (requires a running/creatable fullpipeline Kind cluster + Podman; see §3 R4)
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1912"

# DD-AF-011 regression check (must still pass unmodified)
ginkgo -v ./test/e2e/fullpipeline/... --label-filter="issue-1899"
```

---

## 9. Test Deliverables

| Deliverable | Location | Description |
|---|---|---|
| This test plan | `docs/tests/1912-1915/TEST_PLAN.md` (`main`) | Port-specific strategy; defers to `v1.5`'s plan for original root-cause analysis |
| Design decision errata update | `docs/architecture/decisions/DD-AF-011-phase-transition-consent-guard.md` (Errata section, backport note added) | Cross-branch traceability |
| Unit tests | `pkg/apifrontend/agent/phase_guard_test.go`, `pkg/apifrontend/session/reinvoke_test.go`, `pkg/apifrontend/agent/prompt_test.go` | Ginkgo BDD |
| Integration tests | `test/integration/apifrontend/reinvocation_terminal_test.go` (new on `main`) | Ginkgo BDD, `types.LLMConfig`-adapted |
| E2E tests | `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` (extended) | Ginkgo BDD, real FP cluster |
| Production code | `pkg/apifrontend/agent/phase_guard.go` (`clearDriverSessionState` added), `pkg/apifrontend/agent/prompt.txt` (rewritten) | Go / prompt text |
| E2E infrastructure | `test/infrastructure/shared_e2e.go`, `test/infrastructure/fullpipeline_e2e.go` (both modified) | Go |

---

## 10. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-08-04 | `main` port of the completed `release/v1.5` #1912 + #1915 fixes (PR #1920), adapted to `main`'s refactored `phase_guard.go` and independently-under-instructed `prompt.txt` baseline |
