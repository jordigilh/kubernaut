# Test Plan: Takeover races ahead of a still-finishing autonomous investigation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2155-v1.0
**Feature**: Bound-retry context reconstruction on takeover to close the race between
`TransitionToUserDriving`'s synchronous status flip and the autonomous investigation
goroutine's own (slightly later) result write.
**Version**: 1.0
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

This plan documents the RED→GREEN TDD sequence used to reproduce the race
deterministically and close it with a short bounded retry.

### 1.2 Objectives

1. **Deterministic reproduction**: an integration test forces the investigation's
   result-write to land strictly *after* takeover's status transition and strictly
   *before* the retry budget is exhausted, so the test is not a timing-dependent flake
   in either direction (RED fails every run pre-fix; GREEN passes every run post-fix).
2. **Bounded fix**: the retry adds no latency on the common path (result already
   present, or a genuinely fresh investigation with no history) and caps worst-case
   added latency at `reconstructionRetryAttempts * reconstructionRetryInterval` (400ms)
   only in the narrow race window.
3. **Scoped blast radius**: only `handleTakeover`'s call site is changed.
   `handleStart` (fresh session, no result to race against) and
   `discover_workflows` (called well after the user's first interactive message, by
   which point the race window has closed) are unaffected.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/mcp/tools/... -run TestMCPToolsUnit -args -ginkgo.focus=2155` |
| Integration test pass rate | 100% | `bin/ginkgo --focus="IT-KA-2155-001" ./test/integration/kubernautagent/mcp/...` |
| Backward compatibility | 0 regressions | `go test ./internal/kubernautagent/...` |
| RED confirmed before fix | test fails deterministically | recorded below |

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
| R1 | Retry adds latency to every takeover, even when there is no race | User-visible slowdown | Low | UT-KA-2155-001/002 | Retry only fires when the first read finds 0 turns; common path (result present or genuinely empty) returns on attempt 1 |
| R2 | Retry loop ignores session/inactivity cancellation and blocks | Hung takeover call | Low | UT-KA-2155-003 | `select` on `ctx.Done()` between attempts; `handleTakeover` already wraps `ctx` with `withInactivityCancel` (#1949) |
| R3 | Retry masks a genuine "no prior investigation" case by waiting the full budget every time | Latency regression for the common cold-start case | Medium | UT-KA-2155-002 | Bounded to 5 attempts / 400ms; only `handleTakeover`'s call site retries (not `handleStart`) |

### 3.1 Risk-to-Test Traceability

All three risks (R1–R3) are directly covered: R1/R2 by unit tests exercising the retry
helper's decision logic and cancellation handling in isolation; R3 is bounded by
construction (fixed attempt budget) and proven by `UT-KA-2155-002`.

---

## 4. Scope

### 4.1 Features to be Tested

- **`storeReconstructedContextWithRetry`** (`internal/kubernautagent/mcp/tools/investigate_autonomous.go`):
  bounded-retry wrapper around `storeReconstructedContext`. Validates: retries until a
  non-zero result appears, gives up after the attempt budget, and aborts immediately on
  context cancellation.
- **`handleTakeover`** (`internal/kubernautagent/mcp/tools/investigate_takeover.go`):
  wiring — the takeover path now calls the retrying wrapper, closing the race against a
  real `session.Manager`-driven autonomous investigation goroutine.

### 4.2 Features Not to be Tested

- **`handleStart`'s reconstruction call**: unaffected by this fix (return value already
  discarded there; no prior investigation to race against for a fresh session).
- **`discover_workflows`' reconstruction call**: unaffected; by the time it runs the user
  has already exchanged at least one interactive message, so the race window from
  takeover has long closed.
- **release/v1.5 backport**: confirmed present (same architecture, same test exists) but
  not observed as an active CI flake there; tracked separately in #2155 for backport
  after `main` soaks.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Retry only in `handleTakeover`, not inside `storeReconstructedContext` itself | Keeps the latency cost scoped to the one call site actually exposed to the race; `handleStart`/`discover_workflows` callers would pay for a race they can't hit |
| Fixed attempt budget (5 × 100ms = 400ms) instead of a context-deadline-based loop | Simple, predictable worst case; consistent with the existing `ForceTransitionToUserDriving` retry-on-not-found pattern already in this file's sibling functions |
| Deterministic IT reproduction via a goroutine that closes an unblock channel exactly 50ms after the investigation reaches `StatusRunning`, ahead of `tool.Handle`'s synchronous takeover call | Avoids a flaky sleep-based test in either direction: pre-fix, the single immediate read always loses (read happens in low single-digit ms); post-fix, the second retry attempt (~100–105ms in) always wins (write lands at ~50ms) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of the new retry decision logic (`storeReconstructedContextWithRetry`):
  succeed-on-retry, exhaust-budget, cancel-aborts-early.
- **Integration**: the real race, through the real production entry point
  (`InvestigateTool.Handle` → `handleTakeover`), against a real `session.Manager`.
- **E2E**: no new E2E test added — the pre-existing `E2E-1293-006` already exercises this
  exact code path end-to-end and was the original flake; it is expected to stop flaking
  once this fix lands, and CI will confirm over subsequent runs.

### 5.2 Two-Tier Minimum

UT (logic) + IT (wiring) — both present. See Section 8.

### 5.3 Business Outcome Quality Bar

The business outcome under test is: "when a user takes over an autonomous investigation
that finishes moments after the takeover's status transition, the interactive session
still has that investigation's context available for its first `discover_workflows`/message
turn" — not merely "the retry function was called."

### 5.4 Pass/Fail Criteria

**PASS**:
1. `UT-KA-2155-001/002/003` all pass.
2. `IT-KA-2155-001` passes against a real `session.Manager`, reproducing and then
   closing the exact race from the CI flake.
3. No regressions: `go test ./internal/kubernautagent/...` and
   `golangci-lint run ./internal/kubernautagent/mcp/tools/...` both clean.
4. RED confirmed: `IT-KA-2155-001` deterministically fails against the pre-fix code
   (recorded in Section 9).

**FAIL**: any of the above not met.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/mcp/tools/investigate_autonomous.go` | `storeReconstructedContextWithRetry` | ~25 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/mcp/tools/investigate_takeover.go` | `handleTakeover` (call-site change) | ~1 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTERACTIVE-010 | Context reconstruction on takeover must observe a same-moment-completing investigation, not just an already-completed one | P0 | Unit | UT-KA-2155-001 | Pass |
| BR-INTERACTIVE-010 | Retry must not run forever when there is genuinely no prior investigation | P1 | Unit | UT-KA-2155-002 | Pass |
| BR-KA-267 / #1949 | Retry must honor session-inactivity cascade cancellation | P1 | Unit | UT-KA-2155-003 | Pass |
| BR-INTERACTIVE-010 SC-3 | End-to-end: takeover through the real production entry point must not lose context to this race | P0 | Integration | IT-KA-2155-001 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `internal/kubernautagent/mcp/tools/reconstruction_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-2155-001` | Retry observes a delayed result once it lands, instead of giving up on the first empty read | Pass |
| `UT-KA-2155-002` | Retry gives up after exactly the configured attempt budget when there is genuinely no result | Pass |
| `UT-KA-2155-003` | Retry aborts immediately (not after the full budget) when the context is already cancelled | Pass |

### Tier 2: Integration Tests

**File**: `test/integration/kubernautagent/mcp/takeover_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-2155-001` | A takeover whose target investigation completes 50ms after the status transition still reports >=1 reconstructed turn, through the real `InvestigateTool.Handle` → `handleTakeover` → real `session.Manager` path | Pass |

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

**GREEN (post-fix) result**: passes in 0.108s — confirmed `2026-08-15 16:18:19`
(`/tmp/ka_it_2155_green2.log`, 1 of 102 specs, SUCCESS).

**Dependencies**: envtest (Leases), Podman (PostgreSQL/Redis/DataStorage + Mock LLM containers).

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Location**: `internal/kubernautagent/mcp/tools/reconstruction_test.go`
- **Mocks**: `delayedReconstructor` (in-package test double controlling per-call turn availability) — the only "external dependency" being simulated is the DS-backed audit reconstruction call

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (Lease CRD), Podman (PostgreSQL, Redis, DataStorage, Mock LLM) — reuses the existing `test/integration/kubernautagent/mcp` suite bootstrap
- **Location**: `test/integration/kubernautagent/mcp/takeover_test.go`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — self-contained fix in `internal/kubernautagent/mcp/tools`.

### 11.2 Execution Order

1. **Phase 1 (RED)**: `IT-KA-2155-001` added, confirmed failing against pre-fix code.
2. **Phase 2 (GREEN)**: `storeReconstructedContextWithRetry` added; `handleTakeover`
   updated to call it; `IT-KA-2155-001` confirmed passing.
3. **Phase 3 (UNIT)**: `UT-KA-2155-001/002/003` added, isolating the retry decision logic.
4. **Phase 4 (REFACTOR)**: N/A — GREEN's implementation is the complete, minimal fix;
   no follow-up cleanup was needed beyond the doc comments explaining the race.
5. **Phase 5 (VERIFY)**: `go build ./...`, `golangci-lint run`, full
   `go test ./internal/kubernautagent/...` regression pass.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/2155/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `internal/kubernautagent/mcp/tools/reconstruction_test.go` | UT-KA-2155-001/002/003 |
| Integration test | `test/integration/kubernautagent/mcp/takeover_test.go` | IT-KA-2155-001 |
| Fix | `internal/kubernautagent/mcp/tools/investigate_autonomous.go`, `investigate_takeover.go` | `storeReconstructedContextWithRetry` + call-site update |

---

## 13. Execution

```bash
# Unit tests
go test ./internal/kubernautagent/mcp/tools/... -run TestMCPToolsUnit -args -ginkgo.focus="2155" -ginkgo.v

# Integration test (requires KUBEBUILDER_ASSETS + Podman)
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path)"
bin/ginkgo --focus="IT-KA-2155-001" -v ./test/integration/kubernautagent/mcp/...

# Regression
go test ./internal/kubernautagent/...
golangci-lint run --timeout=5m ./internal/kubernautagent/mcp/tools/...
```

---

## 14. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `storeReconstructedContextWithRetry` | `InvestigateTool.Handle(ActionTakeover)` (MCP tool call) | `InvestigateOutput.Response` ("N prior turns reconstructed") | `IT-KA-2155-001` | Pass |

---

## 15. Existing Tests Requiring Updates

None. `IT-KA-1425-001` and `IT-KA-TAKE-001` (same file) continue to pass unmodified —
they test a different aspect of takeover (result preservation via `Store.SetResult`,
and basic status transition) and don't race the reconstruction read against the
investigation's completion.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-15 | Initial test plan, documented alongside RED→GREEN implementation |
