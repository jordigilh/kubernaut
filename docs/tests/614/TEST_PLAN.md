# Test Plan: RO-level DuplicateInProgress Outcome Inheritance

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-614-v1
**Feature**: When a DuplicateInProgress-blocked RR is unblocked, inherit the original RR's terminal outcome instead of re-running the full pipeline
**Version**: 1.0
**Created**: 2026-03-04
**Author**: Kubernaut Development
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

When a `DuplicateInProgress`-blocked RR is unblocked (original RR reaches terminal), the current implementation resets the duplicate to `Pending` via `clearEventBasedBlock` and re-runs the full orchestration pipeline (Pending → Processing → Analyzing → Executing). This is redundant when the original RR already remediated the same signal. The CRD documentation (DD-RO-002-ADDENDUM line 103) states "Inherits outcome" but the implementation does not match. This test plan validates that duplicates inherit the original RR's terminal outcome (Completed or Failed) directly, without re-running the pipeline.

### 1.2 Objectives

1. **Outcome inheritance — Completed**: When the original RR reaches `Completed`, the duplicate transitions to `Completed` with `Outcome="InheritedCompleted"` and emits audit + notification side effects.
2. **Outcome inheritance — Failed**: When the original RR reaches `Failed`, the duplicate transitions to `Failed` with `FailurePhaseDeduplicated` and emits audit side effects.
3. **Dangling reference**: When the original RR is deleted, the duplicate transitions to `Failed` with `FailurePhaseDeduplicated` and a clear message.
4. **Metrics correctness**: `CurrentBlockedGauge` is decremented when leaving Blocked phase via inheritance. `PhaseTransitionsTotal` is recorded.
5. **Method generalization**: `transitionToInheritedCompleted` and `transitionToInheritedFailed` accept `sourceRef`/`sourceKind` parameters, producing correct K8s events and audit for both WE-level (#190) and RR-level (#614) sources.
6. **Regression safety**: Existing #190 WE-level dedup behavior and existing `recheckDuplicateBlock` edge cases (active original, empty DuplicateOf) remain unchanged.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Integration test pass rate | 100% | `go test ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | All existing #190 tests pass without modification to assertions |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- [DD-RO-002-ADDENDUM](../../../docs/architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md): Blocked Phase Semantics — line 103 states DuplicateInProgress "Inherits outcome"
- [BR-ORCH-042](../../../docs/requirements/BR-ORCH-042-consecutive-failure-blocking.md): Consecutive failure blocking with cooldown
- [BR-ORCH-045]: Completion notifications
- [ADR-032 §1]: Mandatory audit for all lifecycle terminal transitions
- Issue #614: RO-level DuplicateInProgress outcome inheritance
- Issue #190: WE/RO dedup phase with result inheritance (prerequisite, completed)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [#190 Test Plan](../190/TEST_PLAN.md) — predecessor; #614 was explicitly deferred from #190

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Original RR deleted before outcome read | Duplicate RR stuck in Blocked forever | Low | UT-RO-614-003, IT-RO-614-002 | Detect NotFound → transitionToInheritedFailed with clear message |
| R2 | Refactoring transition methods breaks #190 | Regression in WE-level dedup inheritance | Medium | UT-RO-614-011, UT-RO-614-012 | Regression guard tests; verify all #190 UTs pass after refactor |
| R3 | ensureNotificationsCreated fails on minimal-data RR | Completion notification errors for RR that never went through Analyzing/Executing | Low | UT-RO-614-008 | ensureNotificationsCreated is designed for graceful degradation; test confirms |
| R4 | CurrentBlockedGauge not decremented on inheritance | Metric drift: gauge never reaches zero | Medium | UT-RO-614-004, IT-RO-614-003 | Explicit gauge decrement before transition call |
| R5 | Block fields left stale after inheritance | Confusing operator experience: Completed RR with BlockReason still set | Low | UT-RO-614-001, UT-RO-614-002 | Keep BlockReason as audit trail (matches transitionToFailedTerminal pattern at blocking.go:222) |
| R6 | Concurrent reconciles during inheritance | Duplicate side effects (metrics, events, audit) | Low | Covered by existing idempotency guards | transitionToInherited* already has oldPhase != targetPhase guards (#190 F-4) |

### 3.1 Risk-to-Test Traceability

- **R1 (Low)**: UT-RO-614-003 (unit) + IT-RO-614-002 (integration) — dangling reference path
- **R2 (Medium)**: UT-RO-614-011/012 (regression guards) + full #190 test suite re-run
- **R3 (Low)**: UT-RO-614-008 — notification creation on minimal-data RR
- **R4 (Medium)**: UT-RO-614-004 + IT-RO-614-003 — gauge decrement

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **`recheckDuplicateBlock`** (`internal/controller/remediationorchestrator/blocking.go` lines 342-379):
  - **Current**: Calls `clearEventBasedBlock(ctx, rr, phase.Pending)` when original RR is terminal
  - **New**: Calls `transitionToInheritedCompleted/Failed` to inherit the original's outcome
  - Handles: Completed, Failed, Deleted (NotFound), and still-active original RR

- **Generalized transition methods** (`internal/controller/remediationorchestrator/reconciler.go` lines 1583-1672):
  - **Current**: `transitionToInheritedCompleted(ctx, rr)` and `transitionToInheritedFailed(ctx, rr, err)` hardcode WFE references in K8s events and logs via `rr.Status.DeduplicatedByWE`
  - **New**: `transitionToInheritedCompleted(ctx, rr, sourceRef, sourceKind)` and `transitionToInheritedFailed(ctx, rr, err, sourceRef, sourceKind)` — parameterized source for correct provenance in events, audit, and logs

- **`handleDedupResultPropagation`** (`internal/controller/remediationorchestrator/reconciler.go` lines 1545-1581):
  - Updated call sites to pass `(rr.Status.DeduplicatedByWE, "WorkflowExecution")` to generalized methods

### 4.2 Features Not to be Tested

- **WE-level collision classification**: Completed in #190; no changes
- **WE controller**: No modifications for #614
- **CRD type changes**: No new types/fields needed — `FailurePhaseDeduplicated`, `DuplicateOf` already exist
- **CRD regeneration**: No `make generate` needed
- **Notification delivery mechanics**: Only verifying `ensureNotificationsCreated` is called; delivery covered by NT tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Option A: Generalize existing methods | Single code path for all inheritance; parameterized `sourceRef`/`sourceKind` avoids near-duplicate code |
| Keep BlockReason as audit trail | Matches `transitionToFailedTerminal` pattern (blocking.go:222); BlockReason provides diagnostic value for operators |
| Decrement `CurrentBlockedGauge` before transition | RR is leaving Blocked phase; gauge must reflect this regardless of target terminal phase |
| Reuse `FailurePhaseDeduplicated` for RR-level inheritance | Same semantic: failure was inherited, not from direct remediation. `countConsecutiveFailures` skip already handles this |
| `ensureNotificationsCreated` called for inherited Completed | Consistent with WE-level inheritance. Function handles missing AIAnalysis/WFE gracefully (no-op on missing refs) |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (recheckDuplicateBlock outcome branches, generalized transition method event/audit emission)
- **Integration**: >=80% of integration-testable code (full Blocked → terminal lifecycle via envtest reconciler)
- **E2E**: Deferred — DuplicateInProgress blocking requires routing engine; covered at IT level

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT):
- **Unit tests**: Isolated `recheckDuplicateBlock` branching, transition method parameterization, gauge decrement
- **Integration tests**: Full reconciliation lifecycle with envtest, Blocked → terminal via `handleBlockedPhase`

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "Duplicate RR automatically inherits Completed when the original succeeds — no redundant pipeline re-run"
- "Duplicate RR automatically inherits Failed when the original fails — operator sees FailurePhaseDeduplicated"
- "If original RR is deleted, duplicate does not hang — it fails with a clear message"
- "K8s events on inherited outcome name the original RR for operator traceability"
- "Blocked gauge accurately reflects the number of currently-blocked RRs"

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9**

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing #190 test suite (all 31 UTs pass)
5. No regressions in existing blocking test suite

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Any existing #190 test fails after method refactoring (regression)
4. `CurrentBlockedGauge` is not decremented on inheritance (metric leak)

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- `go build ./...` fails after refactoring transition method signatures
- envtest infrastructure cannot start
- Cascading failures: more than 3 tests fail for the same root cause

**Resume testing when**:
- Build compiles cleanly
- envtest infrastructure restored
- Root cause identified and fix deployed

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `transitionToInheritedCompleted` (event/audit/notification logic), `transitionToInheritedFailed` (event/audit logic) | ~90 (parameterized event/log strings) |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/blocking.go` | `recheckDuplicateBlock` (outcome branches: Completed, Failed, NotFound) | ~40 |
| `internal/controller/remediationorchestrator/reconciler.go` | `handleDedupResultPropagation` (updated call sites) | ~5 (parameter changes only) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | After #190 commits |
| Dependency: #190 | Merged | WE/RO dedup phase; transition methods being refactored |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-RO-002-A | DuplicateInProgress inherits Completed outcome | P0 | Unit | UT-RO-614-001 | Pending |
| DD-RO-002-A | DuplicateInProgress inherits Failed outcome | P0 | Unit | UT-RO-614-002 | Pending |
| DD-RO-002-A | DuplicateInProgress — original RR deleted → Failed | P0 | Unit | UT-RO-614-003 | Pending |
| BR-ORCH-042 | CurrentBlockedGauge decremented on inheritance | P0 | Unit | UT-RO-614-004 | Pending |
| DD-RO-002-A | K8s event names original RR (sourceKind=RemediationRequest) | P0 | Unit | UT-RO-614-005 | Pending |
| ADR-032 §1 | Audit event emitted for inherited completion | P0 | Unit | UT-RO-614-006 | Pending |
| ADR-032 §1 | Audit event emitted for inherited failure | P0 | Unit | UT-RO-614-007 | Pending |
| BR-ORCH-045 | Completion notification created for inherited Completed | P0 | Unit | UT-RO-614-008 | Pending |
| DD-RO-002-A | Original RR still active → requeue (regression guard) | P1 | Unit | UT-RO-614-009 | Pending |
| DD-RO-002-A | DuplicateOf empty → clear to Pending (regression guard) | P1 | Unit | UT-RO-614-010 | Pending |
| BR-ORCH-025 | #190 WE-level transitionToInheritedCompleted still works (regression) | P1 | Unit | UT-RO-614-011 | Pending |
| BR-ORCH-025 | #190 WE-level transitionToInheritedFailed still works (regression) | P1 | Unit | UT-RO-614-012 | Pending |
| DD-RO-002-A | Full lifecycle: Blocked → original Completed → inherits Completed | P0 | Integration | IT-RO-614-001 | Pending |
| DD-RO-002-A | Full lifecycle: Blocked → original Failed → inherits Failed | P0 | Integration | IT-RO-614-002 | Pending |
| BR-ORCH-042 | Consecutive failure exclusion for RR-level inherited failures | P0 | Integration | IT-RO-614-003 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (RemediationOrchestrator)
- **ISSUE**: 614
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `recheckDuplicateBlock` outcome branching, generalized transition method event/audit/notification emission, gauge decrement. Target: >=80% of new + modified unit-testable code.

#### New Behavior Tests (P0)

| ID | Business Outcome Under Test | Target Function | Phase |
|----|----------------------------|-----------------|-------|
| `UT-RO-614-001` | Blocked/DuplicateInProgress RR inherits Completed when original RR completes: OverallPhase=Completed, Outcome="InheritedCompleted", CompletedAt set | `recheckDuplicateBlock` → `transitionToInheritedCompleted` | Pending |
| `UT-RO-614-002` | Blocked/DuplicateInProgress RR inherits Failed when original RR fails: OverallPhase=Failed, FailurePhase=Deduplicated, CompletedAt set | `recheckDuplicateBlock` → `transitionToInheritedFailed` | Pending |
| `UT-RO-614-003` | Blocked/DuplicateInProgress RR transitions to Failed when original RR is deleted: FailurePhase=Deduplicated, FailureReason mentions original RR name | `recheckDuplicateBlock` (NotFound branch) | Pending |
| `UT-RO-614-004` | CurrentBlockedGauge decremented when RR leaves Blocked via inheritance (both Completed and Failed paths) | `recheckDuplicateBlock` (gauge side effect) | Pending |
| `UT-RO-614-005` | K8s event for inherited Completed mentions "RemediationRequest <name>" (not "WorkflowExecution") | `transitionToInheritedCompleted` with sourceKind="RemediationRequest" | Pending |
| `UT-RO-614-006` | Audit event (orchestrator.lifecycle.completed) emitted for RR-level inherited Completed | `transitionToInheritedCompleted` → `emitCompletionAudit` | Pending |
| `UT-RO-614-007` | Audit event (orchestrator.lifecycle.failed) emitted for RR-level inherited Failed | `transitionToInheritedFailed` → `emitFailureAudit` | Pending |
| `UT-RO-614-008` | `ensureNotificationsCreated` called for RR-level inherited Completed (graceful on minimal-data RR) | `transitionToInheritedCompleted` → `ensureNotificationsCreated` | Pending |

#### Regression Guards (P1)

| ID | Business Outcome Under Test | Target Function | Phase |
|----|----------------------------|-----------------|-------|
| `UT-RO-614-009` | Original RR still active (non-terminal) → requeue, no transition (unchanged behavior) | `recheckDuplicateBlock` (active branch) | Pending |
| `UT-RO-614-010` | DuplicateOf is empty → clear to Pending via clearEventBasedBlock (unchanged fallback) | `recheckDuplicateBlock` (empty DuplicateOf) | Pending |
| `UT-RO-614-011` | #190 WE-level transitionToInheritedCompleted emits K8s event with "WorkflowExecution" source after refactor | `transitionToInheritedCompleted` with sourceKind="WorkflowExecution" | Pending |
| `UT-RO-614-012` | #190 WE-level transitionToInheritedFailed emits K8s event with "WorkflowExecution" source after refactor | `transitionToInheritedFailed` with sourceKind="WorkflowExecution" | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Full `handleBlockedPhase` → `recheckDuplicateBlock` → `transitionToInherited*` lifecycle via envtest reconciler. Target: >=80% of new integration-testable code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-614-001` | Full lifecycle: RR in Blocked/DuplicateInProgress → original RR reaches Completed → duplicate inherits Completed (Outcome=InheritedCompleted, CompletedAt set) | Pending |
| `IT-RO-614-002` | Full lifecycle: RR in Blocked/DuplicateInProgress → original RR deleted → duplicate inherits Failed (FailurePhaseDeduplicated, FailureReason mentions original) | Pending |
| `IT-RO-614-003` | Consecutive failure exclusion: RR-level inherited failure (FailurePhaseDeduplicated from DuplicateInProgress) does not increment consecutive count | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. The DuplicateInProgress blocking path requires the routing engine (`CheckPreAnalysisConditions`) to detect the duplicate, which needs a full Gateway → RO pipeline in Kind. IT with envtest provides equivalent behavioral coverage for the `recheckDuplicateBlock` → inheritance path.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-RO-614-001: Blocked/DuplicateInProgress — original RR Completed → inherits Completed

**BR**: DD-RO-002-ADDENDUM (DuplicateInProgress inherits outcome)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Duplicate RR `rr-dup` in `PhaseBlocked` with `BlockReason=DuplicateInProgress`, `DuplicateOf=rr-original`
- Original RR `rr-original` in `PhaseCompleted` with `Outcome=Completed`
- Fake client contains both RRs; apiReader returns `rr-original` on Get

**Test Steps**:
1. **Given**: Duplicate RR blocked as DuplicateInProgress, original RR Completed
2. **When**: `recheckDuplicateBlock(ctx, rr-dup)` is called
3. **Then**: Duplicate RR transitions to Completed with InheritedCompleted outcome

**Expected Results**:
1. `rr-dup.Status.OverallPhase = Completed`
2. `rr-dup.Status.Outcome = "InheritedCompleted"`
3. `rr-dup.Status.CompletedAt` is set (not zero)
4. `rr-dup.Status.BlockReason` preserved as audit trail
5. No error returned

**Acceptance Criteria**:
- **Behavior**: Duplicate inherits Completed instead of re-running pipeline from Pending
- **Correctness**: Outcome is "InheritedCompleted", not the original's outcome string

### UT-RO-614-002: Blocked/DuplicateInProgress — original RR Failed → inherits Failed

**BR**: DD-RO-002-ADDENDUM
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Duplicate RR `rr-dup` in `PhaseBlocked` with `BlockReason=DuplicateInProgress`, `DuplicateOf=rr-original`
- Original RR `rr-original` in `PhaseFailed` with `FailurePhase=WorkflowExecution`

**Test Steps**:
1. **Given**: Duplicate RR blocked, original RR Failed
2. **When**: `recheckDuplicateBlock(ctx, rr-dup)` is called
3. **Then**: Duplicate RR transitions to Failed with FailurePhaseDeduplicated

**Expected Results**:
1. `rr-dup.Status.OverallPhase = Failed`
2. `rr-dup.Status.FailurePhase = Deduplicated` (NOT the original's FailurePhase)
3. `rr-dup.Status.FailureReason` contains "rr-original" for traceability
4. `rr-dup.Status.CompletedAt` is set
5. No error returned

**Acceptance Criteria**:
- **Behavior**: Inherited failure is distinguishable (Deduplicated, not WorkflowExecution)
- **Correctness**: FailureReason references the original RR name

### UT-RO-614-003: Blocked/DuplicateInProgress — original RR deleted → inherits Failed

**BR**: DD-RO-002-ADDENDUM
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Duplicate RR `rr-dup` in `PhaseBlocked` with `DuplicateOf=rr-deleted`
- No RR named `rr-deleted` exists (simulate deletion)

**Test Steps**:
1. **Given**: Duplicate RR blocked, original RR deleted
2. **When**: `recheckDuplicateBlock(ctx, rr-dup)` is called
3. **Then**: Duplicate transitions to Failed with clear message

**Expected Results**:
1. `rr-dup.Status.OverallPhase = Failed`
2. `rr-dup.Status.FailurePhase = Deduplicated`
3. `rr-dup.Status.FailureReason` contains "rr-deleted" and indicates deletion/not-found
4. `rr-dup.Status.CompletedAt` is set

**Acceptance Criteria**:
- **Behavior**: RR does not hang when original is gone
- **Correctness**: Failure message is actionable for operators

### UT-RO-614-004: CurrentBlockedGauge decremented on inheritance

**BR**: BR-ORCH-042
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- `CurrentBlockedGauge` initialized at value 1.0
- Duplicate RR in Blocked, original RR in Completed

**Test Steps**:
1. **Given**: Gauge at 1.0, duplicate blocked
2. **When**: `recheckDuplicateBlock` inherits Completed
3. **Then**: Gauge decremented to 0.0

**Expected Results**:
1. `CurrentBlockedGauge` counter value is 0.0 after inheritance

**Acceptance Criteria**:
- **Behavior**: Gauge accurately reflects blocked count; no metric leak

### UT-RO-614-005: K8s event mentions original RemediationRequest

**BR**: DD-RO-002-ADDENDUM
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Fake K8s event recorder
- `recheckDuplicateBlock` triggered with original RR completing

**Test Steps**:
1. **Given**: Fake recorder, original RR Completed
2. **When**: Inheritance transition emits K8s event
3. **Then**: Event message contains "RemediationRequest rr-original"

**Expected Results**:
1. Event type: Normal, reason: InheritedCompleted
2. Event message contains "RemediationRequest" (not "WorkflowExecution")
3. Event message contains "rr-original"

**Acceptance Criteria**:
- **Behavior**: Operator can identify the source kind and name from K8s events

### UT-RO-614-006: Audit event emitted for inherited completion

**BR**: ADR-032 §1
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- `rr-dup` has `StartTime` set
- Original RR Completed

**Test Steps**:
1. **Given**: RR with StartTime, original Completed
2. **When**: `recheckDuplicateBlock` inherits Completed
3. **Then**: `emitCompletionAudit` called with outcome="InheritedCompleted"

**Expected Results**:
1. Audit event type: `orchestrator.lifecycle.completed`
2. Outcome: "InheritedCompleted"
3. Duration computed from StartTime

**Acceptance Criteria**:
- **Behavior**: SOC 2 audit trail maintained for all terminal transitions

### UT-RO-614-007: Audit event emitted for inherited failure

**BR**: ADR-032 §1
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Original RR Failed

**Test Steps**:
1. **Given**: Original RR Failed
2. **When**: `recheckDuplicateBlock` inherits Failed
3. **Then**: `emitFailureAudit` called with failurePhase=Deduplicated

**Expected Results**:
1. Audit event type: `orchestrator.lifecycle.failed`
2. FailurePhase: Deduplicated
3. Error references original RR name

### UT-RO-614-008: ensureNotificationsCreated called for inherited Completed

**BR**: BR-ORCH-045
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Duplicate RR blocked (minimal data — no AIAnalysis, no WFE)
- Original RR Completed

**Test Steps**:
1. **Given**: Minimal-data blocked RR, original Completed
2. **When**: `recheckDuplicateBlock` inherits Completed
3. **Then**: `ensureNotificationsCreated` is called without error

**Expected Results**:
1. No panic on missing AIAnalysis/WFE references
2. Notification creation succeeds (or gracefully no-ops)

**Acceptance Criteria**:
- **Behavior**: Completion notifications work for RRs that never reached Executing

### UT-RO-614-009: Original RR still active → requeue (regression guard)

**BR**: DD-RO-002-ADDENDUM
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- Duplicate RR in Blocked with `DuplicateOf=rr-active`
- Original RR `rr-active` in `PhaseExecuting` (non-terminal)

**Test Steps**:
1. **Given**: Duplicate blocked, original still executing
2. **When**: `recheckDuplicateBlock(ctx, rr-dup)` is called
3. **Then**: Returns requeue, no phase transition

**Expected Results**:
1. `rr-dup.Status.OverallPhase` remains `Blocked`
2. Result has `RequeueAfter > 0`
3. No error returned

### UT-RO-614-010: DuplicateOf empty → clear to Pending (regression guard)

**BR**: DD-RO-002-ADDENDUM
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

**Preconditions**:
- RR in Blocked with `BlockReason=DuplicateInProgress` but `DuplicateOf=""`

**Test Steps**:
1. **Given**: Blocked RR with empty DuplicateOf
2. **When**: `recheckDuplicateBlock(ctx, rr)` is called
3. **Then**: Clears block and transitions to Pending (existing fallback)

**Expected Results**:
1. `rr.Status.OverallPhase = Pending`
2. Block fields cleared

### UT-RO-614-011: #190 WE-level inherited Completed still references WFE in events

**BR**: BR-ORCH-025
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR with `DeduplicatedByWE = "original-wfe"` in Executing
- Original WFE in Completed

**Test Steps**:
1. **Given**: WE-level dedup RR, original WFE Completed
2. **When**: `handleDedupResultPropagation` triggers `transitionToInheritedCompleted(ctx, rr, "original-wfe", "WorkflowExecution")`
3. **Then**: K8s event contains "WorkflowExecution original-wfe"

**Expected Results**:
1. Event message contains "WorkflowExecution" (not "RemediationRequest")
2. Event message contains "original-wfe"
3. All #190 behavior preserved

### UT-RO-614-012: #190 WE-level inherited Failed still references WFE in events

**BR**: BR-ORCH-025
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR with `DeduplicatedByWE` in Executing
- Original WFE in Failed

**Test Steps**:
1. **Given**: WE-level dedup RR, original WFE Failed
2. **When**: `handleDedupResultPropagation` triggers `transitionToInheritedFailed(ctx, rr, err, "original-wfe", "WorkflowExecution")`
3. **Then**: K8s event contains "WorkflowExecution original-wfe"

---

### IT-RO-614-001: Full lifecycle — Blocked/DuplicateInProgress → original Completed → inherits

**BR**: DD-RO-002-ADDENDUM
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/dedup_propagation_integration_test.go`

**Preconditions**:
- envtest cluster running with RR CRD
- Reconciler started

**Test Steps**:
1. **Given**: Create `rr-original` in `PhaseCompleted` with `Outcome=Completed`
2. **Given**: Create `rr-dup` in `PhaseBlocked` with `BlockReason=DuplicateInProgress`, `DuplicateOf=rr-original`
3. **When**: Reconciler processes `rr-dup` → `handleBlockedPhase` → `recheckDuplicateBlock`
4. **Then**: `rr-dup` transitions to `Completed`

**Expected Results**:
1. `rr-dup.Status.OverallPhase = Completed`
2. `rr-dup.Status.Outcome = "InheritedCompleted"`
3. `rr-dup.Status.CompletedAt` is set

### IT-RO-614-002: Full lifecycle — Blocked/DuplicateInProgress → original deleted → inherits Failed

**BR**: DD-RO-002-ADDENDUM
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/dedup_propagation_integration_test.go`

**Preconditions**:
- envtest cluster with RR CRD
- No `rr-deleted` exists

**Test Steps**:
1. **Given**: Create `rr-dup` in `PhaseBlocked` with `DuplicateOf=rr-deleted`
2. **When**: Reconciler processes `rr-dup`
3. **Then**: `rr-dup` transitions to Failed

**Expected Results**:
1. `rr-dup.Status.OverallPhase = Failed`
2. `rr-dup.Status.FailurePhase = Deduplicated`
3. `rr-dup.Status.FailureReason` mentions "rr-deleted"

### IT-RO-614-003: Consecutive failure exclusion for RR-level inherited failures

**BR**: BR-ORCH-042
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/dedup_propagation_integration_test.go`

**Preconditions**:
- envtest cluster with field indexes

**Test Steps**:
1. **Given**: Create 2 Failed RRs (same fingerprint): 1 with FailurePhaseDeduplicated (from DuplicateInProgress), 1 with FailurePhaseWorkflowExecution
2. **When**: `countConsecutiveFailures` called for that fingerprint
3. **Then**: Returns 1 (excludes the inherited failure)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fake.NewClientBuilder()` for K8s API (both client and apiReader); `record.NewFakeRecorder` for K8s events
- **Location**: `test/unit/remediationorchestrator/controller/`
- **Resources**: Minimal (no cluster, no I/O)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: envtest with RR + WE CRDs installed
- **Location**: `test/integration/remediationorchestrator/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-runtime | v0.18+ | envtest framework |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| #190 | Code | Merged | Transition methods don't exist; entire #614 blocked | N/A (already merged) |

### 11.2 Execution Order

1. **Phase 1**: Generalize transition method signatures (REFACTOR of #190 code)
2. **Phase 2**: Implement recheckDuplicateBlock inheritance — happy paths (RED → GREEN)
3. **Phase 3**: Edge cases + observability (RED → GREEN)
4. **Audit Checkpoint 1**: Build + full UT suite verification
5. **Phase 4**: Regression guards (RED → GREEN)
6. **Phase 5**: Integration tests (RED → GREEN → REFACTOR)
7. **Audit Checkpoint 2**: Comprehensive audit — CRD, audit, metrics, documentation
8. **Phase 6**: Documentation

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/614/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (new behavior) | `test/unit/remediationorchestrator/controller/dedup_blocking_test.go` | recheckDuplicateBlock inheritance tests (UT-RO-614-001..010) |
| Unit test suite (regression) | `test/unit/remediationorchestrator/controller/dedup_handler_test.go` | Updated #190 tests + regression guards (UT-RO-614-011, 012) |
| Integration test suite | `test/integration/remediationorchestrator/dedup_propagation_integration_test.go` | Full lifecycle tests (IT-RO-614-001..003) |

---

## 13. Execution

```bash
# Unit tests (all #614)
go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="UT-RO-614"

# Regression: verify #190 tests still pass
go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="UT-RO-190"

# Integration tests
go test ./test/integration/remediationorchestrator/... -ginkgo.v -ginkgo.focus="IT-RO-614"

# Coverage
go test ./test/unit/remediationorchestrator/... -coverprofile=ro-unit-coverage.out
go tool cover -func=ro-unit-coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/remediationorchestrator/controller/dedup_handler_test.go` — UT-RO-190-005 (inherited Completed) | Calls `transitionToInheritedCompleted(ctx, rr)` | Update call in test setup/verification to match new signature `(ctx, rr, sourceRef, sourceKind)` | Method signature change |
| `test/unit/remediationorchestrator/controller/dedup_handler_test.go` — UT-RO-190-006 (inherited Failed) | Calls `transitionToInheritedFailed(ctx, rr, err)` | Update to `(ctx, rr, err, sourceRef, sourceKind)` | Method signature change |
| `test/unit/remediationorchestrator/controller/dedup_handler_test.go` — UT-RO-190-015 (K8s event provenance) | Asserts event contains `rr.Status.DeduplicatedByWE` | Update assertion to verify "WorkflowExecution" appears in event message | Event message format changes from hardcoded WFE to parameterized source |
| `test/unit/remediationorchestrator/controller/dedup_handler_test.go` — UT-RO-190-017 (inherited Failed event) | Asserts event contains WFE name | Same as above | Event message parameterization |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
