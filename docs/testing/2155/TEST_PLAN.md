# Test Plan: Takeover races ahead of a still-finishing autonomous investigation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2155-v2.0
**Feature**: Close the race between `TransitionToUserDriving`'s synchronous status flip
and the autonomous investigation goroutine's own (slightly later) result write, by having
`handleTakeover` wait on a real completion signal (`WaitForCompletionByRemediationID`)
instead of retrying context reconstruction on a fixed sleep schedule.
**Version**: 2.0 (v1.0's fixed 5×100ms retry design was replaced — see §16 changelog)
**Created**: 2026-08-15
**Author**: AI agent (Cursor) + jgil
**Status**: Complete
**Branch**: `fix/2155-takeover-reconstruction-race`

---

## 1. Introduction

### 1.1 Purpose

`E2E-1293-006` (`test/e2e/fullpipeline/10_interactive_investigation_test.go`) intermittently
failed in CI with "0 prior turns reconstructed" during interactive takeover
(https://github.com/jordigilh/kubernaut/actions/runs/31883551415/job/95012656186?pr=2153).
RCA (captured in issue #2155) identified a race: `handleTakeover`
(`internal/kubernautagent/mcp/tools/investigate_takeover.go`) calls
`transitionAutonomousToUserDriving`, which synchronously cancels the in-flight
autonomous investigation's context and flips its session status to `user_driving` —
then, in the very next line, reads context via `storeReconstructedContext`. If the
investigation goroutine had already produced a valid result and only needed a few more
milliseconds to land it via `storePartialResult`
(`internal/kubernautagent/session/manager.go` `handleInvestigationSuccess`), the
immediate read observes zero turns even though the investigation succeeded moments
later. This is architecturally distinct from the #1425 fix (which protects the *stored
result* itself via `Store.SetResult`'s first-write-wins) because `TransitionToUserDriving`
bypasses `Store.Update` and thus the deterministic-ordering guarantee `Store.Update`
provides; the race is in `handleTakeover`'s *own read*, not in whether the result is
eventually stored at all.

This plan originally documented a RED→GREEN TDD sequence that closed the race with a
short bounded retry (5×100ms). That design was replaced (see §16) after review
identified the retry budget as an arbitrary, unfounded guess that unconditionally
taxed the common "no prior investigation" case with the full budget. The v2.0 design
closes the same race deterministically instead of probabilistically: rather than
guessing how long the investigation goroutine might still take and sleeping on a fixed
schedule, `handleTakeover` now waits on a real completion signal — a `done` channel on
`session.Session`, closed by the investigation goroutine's own deferred cleanup only
after it has fully finished mutating session state (including the `storePartialResult`
fallback write). This is a Go memory-model happens-before guarantee (channel
close/receive), not a timing guess.

### 1.2 Objectives

1. **Deterministic reproduction**: an integration test forces the investigation's
   result-write to land strictly *after* takeover's status transition, exercising a real
   `session.Manager`-backed completion signal end to end (not a mocked channel), so the
   test proves the actual synchronization primitive, not a stand-in for it.
2. **No arbitrary bound on the common path**: waiting on the real signal costs nothing
   when there's nothing to wait for (an already-closed channel is returned immediately
   for a genuinely fresh investigation, a pending/never-launched session, or a session
   whose investigation already fully finished) — there is no fixed-schedule "tax" on
   these paths, unlike the fixed-retry design this replaces (see §16).
3. **Scoped blast radius**: only `handleTakeover`'s call site (plus the new
   `Session.done`/`Manager.WaitForCompletionByRemediationID` primitive it depends on) is
   changed. `handleStart` (fresh session, no result to race against) and
   `discover_workflows` (called well after the user's first interactive message, by
   which point the race window has closed) are unaffected.
4. **Compliance-control regression check**: prove the one FedRAMP/SOC2-mapped audit
   event on this code path (`aiagent.interactive.started`, AU-2/CC8.1 per
   `docs/services/stateless/kubernaut-agent/security/AUDIT_EVENT_CATALOG.md`) keeps its
   correct `session_id`/`acting_user` attribution regardless of how long the wait for the
   completion signal takes.

### 1.4 FedRAMP/SOC2 Control Objective Assessment (added retroactively, see §16)

**Finding**: this bug and its fix are **not** tied to any documented FedRAMP/SOC2 control
objective for audit-completeness. `BR-INTERACTIVE-010` (the governing BR) contains no
compliance-control references at all; the design authority for takeover
(`docs/architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md`) frames
context-reconstruction correctness as a **UX** concern ("must feel like joining a Slack
thread") and a **security** concern distinct from compliance — SEC-TAKEOVER-001,
preventing "investigation hacking" via poisoned LLM context flowing back into
autonomous execution. The separate, unrelated "reconstruction" concept that *is*
CC8.1-mapped (ADR-034: reconstructing a full `RemediationRequest` from the audit trail
for SOC2 forensic purposes) is unaffected by this bug: the autonomous investigation's
own audit trail is written independently of whether the *interactive* session's
in-memory LLM context successfully reloads it, so CC8.1 reconstruction-from-audit-trail
holds regardless of this race.

The one audit event actually on this code path, `aiagent.interactive.started`
(AU-2/CC8.1 — user identity attribution for session start, per the audit event catalog),
is emitted *before* the (possibly retried) reconstruction call and carries no
reconstruction-count data — so it cannot be corrupted by this race. `UT-KA-2155-AUDIT-001`
(added below) proves this explicitly rather than leaving it as an inference from reading
the call order in `handleTakeover`.

**Conclusion**: no #2141-style audit-schema gap exists here. This is a legitimate
business-logic/decision-quality bug (an SRE taking over sees an artificially empty
conversation history) with one adjacent, now-explicitly-tested compliance control
(AU-2/CC8.1 identity attribution) confirmed unaffected.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/mcp/tools/... -run TestMCPToolsUnit -args -ginkgo.focus=2155` |
| Integration test pass rate | 100% | `bin/ginkgo --focus="IT-KA-2155-001" ./test/integration/kubernautagent/mcp/...` |
| Backward compatibility | 0 regressions | `go test ./internal/kubernautagent/...` |
| No arbitrary latency tax | 0 (was: 400ms in the v1.0 retry design) added latency when there's nothing to wait for | `UT-KA-2155-002` |
| Race-detector clean | 0 data races on the new `Session.done` channel | `go test -race ./internal/kubernautagent/mcp/tools/... ./internal/kubernautagent/session/...` |
| Full-suite regression (not just focused runs) | 0 hangs/timeouts | `bin/ginkgo ./test/integration/kubernautagent/mcp/...` (full, unfocused) |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-INTERACTIVE-010: Interactive takeover context reconstruction
- Issue #2155: takeover races ahead of a still-finishing autonomous investigation
- Issue #1425 (prior, related but distinct fix): result preservation through takeover via `Store.SetResult`
- PR #2153 (where the CI flake was first triaged) / run 31883551415, job 95012656186

### 2.2 Cross-References

- `internal/kubernautagent/session/manager.go` — `handleInvestigationSuccess`, `storePartialResult`
- `internal/kubernautagent/session/manager_interactive.go` — `TransitionToUserDriving`
- `test/integration/kubernautagent/mcp/takeover_test.go` — `IT-KA-1425-001` (related, does not cover this race)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Waiting on the signal adds latency to every takeover, even when there is no race | User-visible slowdown | Low | UT-KA-2155-002 | `WaitForCompletionByRemediationID` returns an already-closed channel when there's nothing to wait for (no investigation, or one that already fully finished); the `select` resolves immediately in that case |
| R2 | The wait ignores session/inactivity cancellation and blocks | Hung takeover call | Low | UT-KA-2155-003 | `select` on `ctx.Done()` alongside the completion channel; `handleTakeover` already wraps `ctx` with `withInactivityCancel` (#1949) |
| R3 | `Session.done` is closed more than once (panic) or never closed (goroutine leak/hang) | Panic on double-close, or a takeover that hangs forever | Low | `go test -race`, UT-KA-2155-001/002/003 | `close(done)` is a single `defer` registered once per investigation launch in `attachInvestigationContext`/`runInvestigation`, guaranteed to run exactly once via Go's defer semantics (including on panic, via `recoverPanic`'s defer ordering) |
| R4 | A test double (or real production caller) whose `InvestigateFunc` doesn't respect `ctx` cancellation will hang `handleTakeover`'s wait, since it now blocks on a real completion signal instead of reading once and returning | Hung takeover call, worse than the old design's bounded 400ms | Medium (found in practice — see §16 v2.1/v1.5-port §16 v2.1) | Full, unfocused `bin/ginkgo ./test/integration/kubernautagent/mcp/...` run | Production `RunFullInvestigation`/LLM calls already respect `ctx` (HTTP/gRPC cancellation propagates); test doubles simulating "investigation still running" must do the same — verified by running the *full* suite, not just the new focused test, after any change to this wait's semantics |
| R5 (superseded, was primary risk in v1.0) | A fixed retry budget is an unfounded guess with no principled bound, and unconditionally taxes the "no prior investigation" case with its full budget | Correctness bug masked as "good enough," plus a real latency regression | N/A (design replaced) | N/A | Replaced by the completion-signal design (§16); no longer applicable |

### 3.1 Risk-to-Test Traceability

R1/R2 are covered by unit tests exercising `handleTakeover`'s wait behavior in isolation
(via a controllable `WaitForCompletionByRemediationID` mock). R3 is covered by the race
detector plus the full unit/integration suite exercising real goroutine lifecycles. R4
was only caught by running the *full* (unfocused) integration suite — the
`IT-KA-2155-001`-focused run used during initial GREEN validation did not exercise the
now-latent `IT-KA-1425-001` hang; see §16 v2.1.

---

## 4. Scope

### 4.1 Features to be Tested

- **`Session.done` / `Manager.WaitForCompletionByRemediationID`**
  (`internal/kubernautagent/session/{store,manager,manager_query}.go`): the new
  completion signal. Validates: closed exactly once after the investigation goroutine
  fully finishes mutating session state (including the `storePartialResult` fallback),
  and resolves to an already-closed channel when there's nothing to wait for.
- **`handleTakeover`** (`internal/kubernautagent/mcp/tools/investigate_takeover.go`):
  wiring — the takeover path now `select`s on the completion signal (bounded only by
  the existing request `ctx`, already wrapped with `withInactivityCancel`) before
  reading reconstructed context, closing the race against a real
  `session.Manager`-driven autonomous investigation goroutine with no new arbitrary
  timeout.

### 4.2 Features Not to be Tested

- **`handleStart`'s reconstruction call**: unaffected by this fix (return value already
  discarded there; no prior investigation to race against for a fresh session).
- **`discover_workflows`' reconstruction call**: unaffected; by the time it runs the user
  has already exchanged at least one interactive message, so the race window from
  takeover has long closed.
- **release/v1.5 backport**: ported as issue #2156 / PR #2158, using the same
  completion-signal design (adapted to `release/v1.5`'s consolidated `investigate.go`
  file layout). Documented separately in `docs/testing/2156/TEST_PLAN.md` on that branch.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| A real completion signal (`done chan struct{}` on `Session`) instead of a fixed retry/sleep schedule | The fixed retry (v1.0 of this plan) had no principled bound — 5×100ms was a guess, not a measurement — and unconditionally taxed the "no prior investigation" case with the full budget (proven by the retired `UT-KA-2155-002`, which asserted all 5 attempts were burned in exactly that case). A channel close/receive is a real Go memory-model happens-before guarantee: `handleTakeover` observes the investigation goroutine's state exactly when it's actually safe to, never more and never less. |
| `done` closed via `defer` registered first (so it runs last, per Go's LIFO defer order) in the investigation goroutine | Guarantees `close(done)` fires only after every other deferred cleanup in that goroutine (`recoverPanic`, which itself calls `Store.Update` on panic) has completed — so `done` firing is a true "fully finished mutating state" signal, not just "the happy path returned" |
| `WaitForCompletionByRemediationID` (looked up by remediation ID, re-scanning the store) rather than threading a specific session ID through `TransitionToUserDriving`'s return value | Mirrors the existing `GetLatestRCASummaryByRemediationID` "latest session for this RR" lookup pattern already used by the same code path; handles both the `TransitionToUserDriving` and `ForceTransitionToUserDriving` (terminal-session force-through) cases uniformly without new interface surface on those methods |
| Bounded only by the existing request `ctx` (already carries the inactivity-cancel wrapper, #1949) — no new timeout constant introduced | The investigation goroutine's own runtime is already bounded upstream by `ToolCallTimeout`/LLM `TimeoutSeconds`, so it cannot hang forever underneath the wait; inventing a second, shorter arbitrary timeout here would just reintroduce the same "how do we know it's enough" problem this design replaces |
| Deterministic IT reproduction via a goroutine that closes an unblock channel exactly 50ms after the investigation reaches `StatusRunning`, ahead of `tool.Handle`'s synchronous takeover call | Avoids a flaky sleep-based test in either direction: pre-fix, the single immediate read always loses (read happens in low single-digit ms); post-fix, the wait resolves the instant the real `done` channel closes (~50ms in, when the gate releases), regardless of exact scheduling jitter |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of the new wait decision logic in `handleTakeover`: waits for the signal
  before reading, adds no latency when there's nothing to wait for, aborts immediately on
  context cancellation. Plus `go test -race` coverage of `Session.done`'s lifecycle.
- **Integration**: the real race, through the real production entry point
  (`InvestigateTool.Handle` → `handleTakeover`), against a real `session.Manager` and its
  real `WaitForCompletionByRemediationID` implementation (not a mocked channel).
- **E2E**: no new E2E test added — the pre-existing `E2E-1293-006` already exercises this
  exact code path end-to-end and was the original flake; it is expected to stop flaking
  once this fix lands, and CI will confirm over subsequent runs.

### 5.2 Two-Tier Minimum

UT (logic) + IT (wiring) — both present. See Section 8.

### 5.3 Business Outcome Quality Bar

The business outcome under test is: "when a user takes over an autonomous investigation
that finishes moments after the takeover's status transition, the interactive session
still has that investigation's context available for its first `discover_workflows`/message
turn" — not merely "the completion signal was awaited."

### 5.4 Pass/Fail Criteria

**PASS**:
1. `UT-KA-2155-001/002/003` all pass.
2. `IT-KA-2155-001` passes against a real `session.Manager`, reproducing and then
   closing the exact race from the CI flake.
3. No regressions: `go build ./...`, `go vet ./...`, and the **full, unfocused**
   `bin/ginkgo ./test/integration/kubernautagent/mcp/...` suite (not just the new
   focused test — see R4/§16 v2.1) all pass, and
   `golangci-lint run ./internal/kubernautagent/mcp/tools/... ./test/integration/kubernautagent/mcp/...`
   is clean.
4. `UT-KA-2155-AUDIT-001` passes, confirming AU-2/CC8.1 attribution is unaffected.

**FAIL**: any of the above not met.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/mcp/tools/investigate_takeover.go` | `handleTakeover` (wait-on-signal call site) | ~8 |
| `internal/kubernautagent/session/manager_query.go` | `WaitForCompletionByRemediationID` | ~25 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/session/manager.go` | `attachInvestigationContext`/`runInvestigation` (`done` channel creation/close wiring) | ~10 |
| `internal/kubernautagent/mcp/tools/investigate_takeover.go` | `handleTakeover` → `Manager.WaitForCompletionByRemediationID` (real end-to-end wiring) | ~8 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTERACTIVE-010 | Context reconstruction on takeover must wait for a same-moment-completing investigation's real completion signal, not just read an already-completed one | P0 | Unit | UT-KA-2155-001 | Pass |
| BR-INTERACTIVE-010 | Waiting must add no latency when there is genuinely no prior investigation to wait for | P1 | Unit | UT-KA-2155-002 | Pass |
| BR-KA-267 / #1949 | The wait must honor session-inactivity cascade cancellation | P1 | Unit | UT-KA-2155-003 | Pass |
| BR-INTERACTIVE-010 SC-3 | End-to-end: takeover through the real production entry point, against a real `session.Manager`'s real completion signal, must not lose context to this race | P0 | Integration | IT-KA-2155-001 | Pass |
| AU-2, CC8.1 (audit identity attribution, per `AUDIT_EVENT_CATALOG.md`) | `aiagent.interactive.started`'s `session_id`/`acting_user` attribution must remain correct regardless of how long the wait for the completion signal takes | P1 | Unit | UT-KA-2155-AUDIT-001 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `internal/kubernautagent/mcp/tools/reconstruction_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-2155-001` | Takeover waits for the completion signal to fire before reading, and returns the turns that landed by then, instead of reading too early | Pass |
| `UT-KA-2155-002` | Takeover adds no latency when there is nothing to wait for (a genuinely empty investigation history still resolves to 0, but fast) | Pass |
| `UT-KA-2155-003` | Takeover aborts the wait immediately (not after any fixed budget) when the context is already cancelled | Pass |

**File**: `internal/kubernautagent/mcp/tools/takeover_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-2155-AUDIT-001` | The AU-2/CC8.1-mapped `aiagent.interactive.started` audit event keeps correct `session_id`/`acting_user`/`correlation_id` attribution even while the takeover is still waiting on the completion signal | Pass |

### Tier 2: Integration Tests

**File**: `test/integration/kubernautagent/mcp/takeover_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-2155-001` | A takeover whose target investigation completes 50ms after the status transition still reports >=1 reconstructed turn, through the real `InvestigateTool.Handle` → `handleTakeover` → real `session.Manager.WaitForCompletionByRemediationID` path (real completion signal, not mocked) | Pass |

### Tier Skip Rationale

- **E2E**: not added as a new test. `E2E-1293-006` (`test/e2e/fullpipeline/10_interactive_investigation_test.go`)
  already covers this exact journey and was the original CI flake; it is the regression
  detector for this fix rather than a new scenario to author.

---

## 9. Test Cases

### IT-KA-2155-001: Takeover races ahead of a still-finishing autonomous investigation

**BR**: BR-INTERACTIVE-010 SC-3
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/mcp/takeover_test.go`

**Test Steps**:
1. **Given**: a real `session.Manager`-backed autonomous investigation is `StatusRunning`
   for `rr-2155-it-001`, blocked on an unbuffered gate channel.
2. **Given**: a goroutine is scheduled to close the gate exactly 50ms after the
   investigation reaches `StatusRunning` — i.e., strictly after takeover's status
   transition fires (which happens synchronously, in low single-digit ms).
3. **When**: `InvestigateTool.Handle(ActionTakeover)` is called for `rr-2155-it-001`.
4. **Then**: the response's `Response` field matches `[1-9]\d* prior turns reconstructed`
   (not `0 prior turns reconstructed`).

**RED (pre-fix) result**: `0 prior turns reconstructed` — confirmed
`2026-08-15 16:08:13` (`/tmp/ka_it_2155_red.log`, 1 of 102 specs, FAIL).

**GREEN (v1.0 retry design) result**: passes in 0.108s — confirmed `2026-08-15 16:18:19`
(`/tmp/ka_it_2155_green2.log`, 1 of 102 specs, SUCCESS).

**GREEN (v2.0 completion-signal design) result**: passes; full suite (102/102 specs)
confirmed clean, including `IT-KA-1425-001` after its fix (see §16 v2.1) — a
focused-only run would not have caught that latent hang.

**Dependencies**: envtest (Leases), Podman (PostgreSQL/Redis/DataStorage + Mock LLM containers).

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Location**: `internal/kubernautagent/mcp/tools/reconstruction_test.go`
- **Mocks**: `interactiveAutoMgr.waitCh` / `takeoverAutoMgr.waitCh` (in-package test double fields exposing a controllable channel for `WaitForCompletionByRemediationID`) and `mockContextReconstructor` (DS-backed audit reconstruction call stub)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (Lease CRD), Podman (PostgreSQL, Redis, DataStorage, Mock LLM) — reuses the existing `test/integration/kubernautagent/mcp` suite bootstrap
- **Location**: `test/integration/kubernautagent/mcp/takeover_test.go`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — self-contained fix in `internal/kubernautagent/mcp/tools` and
`internal/kubernautagent/session`.

### 11.2 Execution Order

1. **Phase 1 (v1.0, retired)**: `IT-KA-2155-001` added (RED, confirmed failing), then
   `storeReconstructedContextWithRetry` (fixed 5×100ms retry) added and wired into
   `handleTakeover` (GREEN); `UT-KA-2155-001/002/003` added, isolating the retry
   decision logic.
2. **Phase 2 (v2.0, current)**: replaced the retry design after review flagged it as an
   unfounded/arbitrary bound (see §16):
   - Added `done chan struct{}` to `session.Session` (`store.go`), closed exactly once by
     the investigation goroutine's own deferred cleanup (`manager.go`
     `attachInvestigationContext`/`runInvestigation`).
   - Added `Manager.WaitForCompletionByRemediationID` (`manager_query.go`).
   - Added `WaitForCompletionByRemediationID` to the `AutonomousSessionManager` interface
     and every test double implementing it.
   - Updated `handleTakeover` to `select` on the completion signal (bounded only by the
     existing request `ctx`) instead of retrying on a fixed schedule; removed
     `storeReconstructedContextWithRetry` and its constants entirely.
3. **Phase 3**: rewrote `UT-KA-2155-001/002/003` and `UT-KA-2155-AUDIT-001` against the
   new wait-based behavior (via `Handle(ActionTakeover)` + a controllable `waitCh`
   test-double field, rather than testing a removed retry helper directly); `IT-KA-2155-001`
   required only a doc-comment update — it already exercised the real production entry
   point end to end, and now exercises the real completion signal too.
4. **Phase 4 (found during full-suite verification)**: a full, unfocused
   `bin/ginkgo ./test/integration/kubernautagent/mcp/...` run (rather than the
   `IT-KA-2155-001`-focused run used in Phase 3) revealed `IT-KA-1425-001` hanging —
   its investigation function ignored `ctx` cancellation, which the new wait-based
   design now depends on. Fixed by making that test respect `ctx.Done()`, matching real
   production behavior; see §16 v2.1.
5. **Phase 5 (REFACTOR)**: N/A — this was a targeted redesign of the fix itself in
   response to review feedback, not a refactor of otherwise-complete GREEN-phase code.
6. **Phase 6 (VERIFY)**: `go build ./...`, `go vet ./...`, `golangci-lint run` (0 issues),
   `go test -race ./internal/kubernautagent/mcp/tools/... ./internal/kubernautagent/session/...`
   (pass), and the full `bin/ginkgo ./test/integration/kubernautagent/mcp/...` suite
   (102/102 specs pass, ~254s).

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/2155/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `internal/kubernautagent/mcp/tools/reconstruction_test.go` | UT-KA-2155-001/002/003 |
| Unit test (compliance) | `internal/kubernautagent/mcp/tools/takeover_test.go` | UT-KA-2155-AUDIT-001 |
| Integration test | `test/integration/kubernautagent/mcp/takeover_test.go` | IT-KA-2155-001 (plus `IT-KA-1425-001` fix, §16 v2.1) |
| Fix | `internal/kubernautagent/session/store.go`, `manager.go`, `manager_query.go`; `internal/kubernautagent/mcp/tools/investigate.go`, `investigate_autonomous.go`, `investigate_takeover.go`, `export_test.go` | `Session.done`, `Manager.WaitForCompletionByRemediationID`, `ClosedChan`, `handleTakeover` call-site update |

---

## 13. Execution

```bash
# Unit tests
go test ./internal/kubernautagent/mcp/tools/... -run TestMCPToolsUnit -args -ginkgo.focus="2155" -ginkgo.v

# Integration test (requires KUBEBUILDER_ASSETS + Podman) -- run the FULL suite, not
# just the focused test, per R4/§16 v2.1
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path)"
bin/ginkgo ./test/integration/kubernautagent/mcp/...

# Regression
go build ./...
go vet ./internal/kubernautagent/...
golangci-lint run --timeout=5m ./internal/kubernautagent/mcp/tools/... ./test/integration/kubernautagent/mcp/...
```

---

## 14. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `Manager.WaitForCompletionByRemediationID` | `InvestigateTool.Handle(ActionTakeover)` (MCP tool call) → `handleTakeover`'s `select` | `InvestigateOutput.Response` ("N prior turns reconstructed") | `IT-KA-2155-001` | Pass |
| `Session.done` close (`runInvestigation`'s deferred cleanup) | Investigation goroutine completion (success, failure, or panic) | Unblocks the `select` above via channel close | `IT-KA-2155-001`, `IT-KA-1425-001` | Pass |

---

## 15. Existing Tests Requiring Updates

`IT-KA-1425-001` (same file) required a fix (see §16 v2.1): its investigation
function blocked on an artificial gate channel that ignored `ctx` cancellation,
which now hangs `handleTakeover`'s wait indefinitely since it legitimately waits
for the goroutine's real completion signal rather than reading once and moving on.
Updated to respect `ctx.Done()` like real production `RunFullInvestigation`/LLM
calls do -- the test's actual assertions (result preserved through takeover,
`discover_workflows` finds it) are unchanged. `IT-KA-TAKE-001` continues to pass
unmodified (its investigation function already selects on `ctx.Done()`).

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-15 | Initial test plan, documented alongside RED→GREEN implementation. Design: fixed 5×100ms retry budget on `handleTakeover`'s reconstruction read. |
| 1.1 | 2026-08-15 | Added §1.4 FedRAMP/SOC2 control objective assessment and `UT-KA-2155-AUDIT-001`, closing the compliance-verification pass requested after initial implementation |
| 2.0 | 2026-08-15 | **Design replaced** following review feedback that the fixed retry budget was arbitrary and did not actually solve the underlying synchronization problem: (1) it had no principled bound — 5×100ms was a guess, not derived from any measurement of how long the goroutine's own state-mutation could take; (2) it proved the "genuinely no prior investigation" case unconditionally paid the *entire* 400ms budget (the retired `UT-KA-2155-002` asserted exactly this), directly contradicting the v1.0 code comment's claim that this case "still returns on the first attempt." Replaced with a real completion signal: `session.Session` gained a `done chan struct{}`, closed exactly once by the investigation goroutine's own deferred cleanup only after it fully finishes mutating state (including the `storePartialResult` fallback). `handleTakeover` now `select`s on `Manager.WaitForCompletionByRemediationID(rrID)` (bounded only by the existing request `ctx`, no new arbitrary timeout) instead of retrying on a schedule. This closes the same race deterministically (a Go memory-model happens-before guarantee) rather than probabilistically, and eliminates the latency tax on the empty-history case entirely. Ported to `release/v1.5` as issue #2156 / PR #2158 using the same design. |
| 2.1 | 2026-08-15 | **Found and fixed a latent hang exposed by v2.0's design change**: a full-suite run (not just the `IT-KA-2155-001`-focused run used during Phase 3 GREEN validation) revealed `IT-KA-1425-001` timing out after ~110s (and, independently, an identical hang confirmed on the `release/v1.5` port). Its investigation function blocked on an artificial `gate` channel that ignored `ctx` entirely, so `TransitionToUserDriving`'s cancellation had no way to unblock it — under the v1.0 retry design this didn't matter (a single immediate read just returned 0 and moved on), but under v2.0's real wait-for-completion design, `handleTakeover` now blocks until the goroutine's `done` channel closes, which never happened. Fixed by making the test's investigation function respect `ctx.Done()` (as real production `RunFullInvestigation`/LLM calls do), removing the artificial gate. Full suite (102/102 specs) and `go vet`/`golangci-lint` reconfirmed clean after the fix; same fix applied to the `release/v1.5` port (PR #2158). |
