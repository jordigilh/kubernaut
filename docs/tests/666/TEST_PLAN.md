# Test Plan: RO Phase Handler Registry Refactoring

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-666-v1
**Feature**: Migrate RO reconciler from monolithic state machine to Phase Handler Registry
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant + Jordi Gil
**Status**: Active
**Branch**: `feature/v1.0-remaining-bugs-demos`

---

## 1. Introduction

### 1.1 Purpose

This test plan provides behavioral assurance for the incremental extraction of 8 phase
handlers from the monolithic `reconciler.go` (~4,000 lines) into independently testable
`PhaseHandler` implementations dispatched by a thin registry. The plan ensures that every
code path in the original reconciler is characterized before extraction, that each extracted
handler preserves exact behavioral fidelity, and that the integration-tier wiring maintains
correctness under the K8s reconciler model.

### 1.2 Objectives

1. **Behavioral Fidelity**: Every extracted handler produces identical `ctrl.Result` and
   status mutations as the original monolithic code for all known input combinations.
2. **Characterization Coverage**: >=80% of unit-testable code in each handler is covered
   by characterization + handler unit tests before and after extraction.
3. **Integration Wiring**: Registry dispatch, `ApplyTransition`, and reconciler lifecycle
   validated via envtest integration tests with >=80% of integration-testable code covered.
4. **Zero Regressions**: All pre-existing unit (234 specs), integration (118 specs), and
   E2E tests pass at every milestone boundary.
5. **Anti-Pattern Compliance**: No `time.Sleep()`, no `Skip()`, no direct audit store
   testing, no mocking of internal business logic, no HTTP endpoint testing at unit tier.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/controller/...` |
| Integration test pass rate | 100% (excl. pre-existing IT-RO-614-001) | `go test ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | Handler files + `phase/` + `config/` |
| Integration-testable code coverage | >=80% | `internal/controller/remediationorchestrator/` wiring |
| Backward compatibility | 0 regressions | Pre-existing 234 unit + 118 integration specs |
| reconciler.go line count | <400 lines | `wc -l` after M4 complete |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-ORCH-025**: Phase transitions auditable via TransitionIntent
- **BR-ORCH-026**: Status aggregation unchanged
- **BR-ORCH-027/028**: Timeout handling stays in dispatcher
- **BR-ORCH-029**: Workflow execution lifecycle
- **BR-ORCH-031**: Lock management during transitions
- **BR-ORCH-036**: AI analysis completion routing
- **BR-ORCH-037**: Workflow resolution and creation
- **BR-ORCH-042**: Blocked phase handling (cooldown, recheck, expiry)
- **BR-ORCH-044**: Metrics emission centralized
- **BR-ORCH-045**: Completion notifications after Outcome set
- **BR-ORCH-095**: Override resolution for approval flow
- **BR-EM-010**: EA gates completion (Verifying phase)
- **BR-GATEWAY-185**: Dedup/blocked phase routing
- **BR-SCOPE-010**: UnmanagedResource blocks re-validate scope
- **ADR-EM-001**: EA lifecycle tracking
- Issue #666: refactor(ro): Migrate RO reconciler to Phase Handler Registry pattern

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [APDC Framework](../../development/methodology/APDC_FRAMEWORK.md)
- [DD-RO-002-ADDENDUM: Blocked Phase Semantics](../../architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `trackEffectivenessStatus` mutates `rr.Status.OverallPhase` inside status closure — handler pattern expects return-based transitions | Handler returns stale NoOp while phase was already mutated to Completed | Medium | UT-VER-H-012 | Callback injection: handler delegates to reconciler method, checks phase post-call |
| R2 | `handleAnalyzingPhase` has ~20 distinct return paths including lock contention, stale cache, override resolution | Missed path in extraction causes silent behavioral change | High | UT-ANZ-H-*, IT-RO-CHAR-ANZ-* | Pre-extraction characterization tests for all paths; CHECKPOINT 3c adversarial audit |
| R3 | Shared WFE creation flow between Analyzing + AwaitingApproval may be more entangled than estimated | Extracting shared utility breaks one or both consumers | Medium | UT-WEC-*, IT-RO-CHAR-ANZ-*, IT-RO-CHAR-APR-* | Extract utility with test coverage first; wire into both handlers; run characterization suite |
| R4 | Registry dispatch swallows errors instead of propagating (regression observed in M4: UT-RO-190-019) | Controller-runtime backoff broken for transient errors | High | UT-RO-190-019, UT-RO-CHAR-EXE-* | Error propagation validated in existing characterization tests; pattern documented |
| R5 | `handleBlockedPhase` calls `recheckDuplicateBlock` which depends on cross-RR queries | Test isolation difficult; fake client may not replicate production behavior | Medium | UT-BLK-H-*, IT-RO-614-001 | Integration tier covers recheck paths with envtest; unit tier uses controlled fakes |
| R6 | `handleAwaitingApprovalPhase` RAR deadline expiry path modifies RAR status directly | Status update on foreign CRD (RAR) is I/O — unit tests need careful mock setup | Low | UT-APR-H-010, UT-APR-H-011 | Fake client with StatusSubresource; integration tier validates real K8s behavior |

### 3.1 Risk-to-Test Traceability

- **R1 (CRITICAL)**: Mitigated by UT-VER-H-012 (trackEffectivenessStatus phase mutation detection)
- **R2 (HIGH)**: Mitigated by UT-ANZ-H-001 through UT-ANZ-H-020 (all 20 paths) + CHECKPOINT 3c
- **R3 (MEDIUM)**: Mitigated by UT-WEC-001 through UT-WEC-006 (shared creation utility)
- **R4 (HIGH)**: Mitigated by existing UT-RO-190-019 + UT-RO-CHAR-EXE-002
- **R5 (MEDIUM)**: Mitigated by UT-BLK-H-001 through UT-BLK-H-008 + IT-RO-614-001
- **R6 (LOW)**: Mitigated by UT-APR-H-010, UT-APR-H-011 + IT-RO-CHAR-APR-*

---

## 4. Scope

### 4.1 Features to be Tested

- **Verifying Handler** (`internal/controller/remediationorchestrator/verifying_handler.go`):
  EA lifecycle tracking, verification deadline management, safety-net timeout, notification retry
- **Blocked Handler** (`internal/controller/remediationorchestrator/blocked_handler.go`):
  Cooldown expiry, recheck logic (ResourceBusy, DuplicateInProgress), UnmanagedResource
- **Shared WFE Creation** (`internal/controller/remediationorchestrator/wfe_creation.go`):
  Workflow resolution, lock acquisition, WFE CRD creation, status patching
- **Analyzing Handler** (`internal/controller/remediationorchestrator/analyzing_handler.go`):
  AI analysis result routing, approval flow, direct-to-execution, WorkflowNotNeeded
- **AwaitingApproval Handler** (`internal/controller/remediationorchestrator/awaiting_approval_handler.go`):
  RAR decision handling (Approved/Rejected/Expired), deadline management, override resolution
- **Registry Wiring** (`internal/controller/remediationorchestrator/reconciler.go`):
  Dispatch for all 8 handlers, dead code removal, `reconciler.go` < 400 lines
- **ApplyTransition** (`internal/controller/remediationorchestrator/apply_transition.go`):
  All 7 `TransitionType` variants dispatched correctly (already tested, maintain)
- **TransitionIntent** (`pkg/remediationorchestrator/phase/transition.go`):
  Type constructors, validation, helpers (already tested, maintain)

### 4.2 Features Not to be Tested

- **E2E tier**: Refactoring is internal; E2E tests validate deployed behavior which is unchanged.
  Existing E2E suite serves as regression gate only.
- **Performance/load**: Out of scope per v1.0 constraints. Load test previously removed from suite.
- **IT-RO-614-001**: Pre-existing failure unrelated to #666; tracked separately.
- **Notification, DataStorage, Gateway controllers**: Unaffected by RO-internal refactoring.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Callback injection for heavy reconciler deps | Avoids pulling entire reconciler into handler; preserves testability with lightweight mocks |
| Characterization tests before extraction | Safety net catches behavioral drift during refactoring |
| One handler per extraction PR | Limits blast radius; each step independently verifiable |
| `TransitionIntent` return-based pattern | Declarative transitions enable independent handler testing without reconciler wiring |
| `trackEffectivenessStatus` phase mutation detection | Legacy code mutates phase in status closure; handler detects post-call mutation rather than refactoring the deep method |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `TESTING_GUIDELINES.md` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (handler logic, phase types, transition constructors, config)
- **Integration**: >=80% of integration-testable code (reconciler wiring, envtest K8s API, status updates)
- **E2E**: Regression only (existing suite); no new E2E tests for this refactoring

### 5.2 Two-Tier Minimum

Every handler extraction is covered by at least 2 test tiers:
- **Unit tests**: Handler logic in isolation with fake clients and mock callbacks
- **Integration tests**: Full reconciler wiring in envtest with real K8s API

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "RR in Verifying phase with expired EA transitions to Completed with Outcome=Remediated"
- NOT "handler calls trackEffectivenessStatus" (implementation detail)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold for handler files
4. All 234 pre-existing unit specs pass (zero regressions)
5. All 118 pre-existing integration specs pass (excl. IT-RO-614-001)
6. `reconciler.go` < 400 lines after M4 complete

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on handler files
3. Pre-existing tests regress
4. Behavioral change detected by characterization tests

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Characterization test detects behavioral drift (stop, investigate, fix before proceeding)
- Build broken (code does not compile)
- envtest infrastructure unavailable (stale processes, port conflicts)

**Resume testing when**:
- Behavioral drift root-caused and fixed
- Build green
- envtest processes cleaned up

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/phase/transition.go` | TransitionIntent constructors, Validate, helpers | ~200 |
| `pkg/remediationorchestrator/phase/handler.go` | PhaseHandler interface, Registry | ~60 |
| `internal/controller/remediationorchestrator/verifying_handler.go` | Handle, Phase | ~150 |
| `internal/controller/remediationorchestrator/blocked_handler.go` | Handle, Phase | ~100 |
| `internal/controller/remediationorchestrator/analyzing_handler.go` | Handle, Phase | ~340 |
| `internal/controller/remediationorchestrator/awaiting_approval_handler.go` | Handle, Phase | ~360 |
| `internal/controller/remediationorchestrator/wfe_creation.go` | CreateWFE (shared utility) | ~100 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | Reconcile, NewReconciler, registry dispatch | ~400 (target) |
| `internal/controller/remediationorchestrator/apply_transition.go` | ApplyTransition, ToBlockingCondition | ~90 |
| `internal/controller/remediationorchestrator/effectiveness_tracking.go` | trackEffectivenessStatus, completeVerificationIfNeeded | ~140 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/v1.0-remaining-bugs-demos` HEAD | Active development branch |
| Dependency: TransitionIntent (M2) | Completed | 60 tests passing |
| Dependency: ApplyTransition (M2) | Completed | 12 tests passing |
| Dependency: Executing/Pending/Processing handlers (M3.1-3.3) | Completed | 33 tests passing |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-025 | Phase transitions auditable via TransitionIntent | P0 | Unit | UT-TI-001 through UT-TI-040 | Pass |
| BR-ORCH-025 | Phase transitions auditable via TransitionIntent | P0 | Unit | UT-AT-001 through UT-AT-012 | Pass |
| BR-ORCH-026 | Status aggregation unchanged | P0 | Integration | IT-RO-CHAR-* | Pass |
| BR-ORCH-042 | Blocked phase handling | P0 | Unit | UT-BLK-H-001 through UT-BLK-H-008 | Pending |
| BR-ORCH-042 | Blocked phase handling | P0 | Unit | UT-RO-CHAR-BLK-001, BLK-002 | Pass |
| BR-ORCH-042 | Blocked phase handling | P1 | Integration | IT-RO-CHAR-BLK-* | Pass (pre-existing) |
| BR-ORCH-044 | Metrics emission centralized | P1 | Unit | UT-VER-H-010 (metrics in safety-net) | Pass |
| BR-ORCH-045 | Completion notifications after Outcome set | P0 | Unit | UT-VER-H-003, UT-VER-H-004 | Pass |
| BR-EM-010 | EA gates completion | P0 | Unit | UT-VER-H-007 through UT-VER-H-014 | Pass |
| BR-EM-010 | EA gates completion | P0 | Unit | UT-VERIFY-001 through UT-VERIFY-006 | Pass |
| BR-EM-010 | EA gates completion | P0 | Unit | UT-RO-CHAR-VER-001 through VER-004 | Pass |
| BR-ORCH-036 | AI analysis completion routing | P0 | Unit | UT-ANZ-H-001 through UT-ANZ-H-020 | Pending |
| BR-ORCH-036 | AI analysis completion routing | P0 | Unit | UT-RO-CHAR-ANZ-001 through ANZ-004 | Pass |
| BR-ORCH-037 | Workflow resolution and creation | P0 | Unit | UT-WEC-001 through UT-WEC-006 | Pending |
| BR-ORCH-026 | RAR approval/rejection/expiry | P0 | Unit | UT-APR-H-001 through UT-APR-H-012 | Pending |
| BR-ORCH-026 | RAR approval/rejection/expiry | P0 | Unit | UT-RO-CHAR-APR-001 through APR-004 | Pass |
| BR-ORCH-095 | Override resolution for approval flow | P1 | Unit | UT-APR-H-005 (override permanent) | Pending |
| BR-SCOPE-010 | UnmanagedResource recheck | P1 | Unit | UT-BLK-H-006 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}` for BR-linked tests.
Format: `{TIER}-{HANDLER}-{SEQUENCE}` for handler-specific tests (used throughout #666).

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (Remediation Orchestrator)
- **HANDLER**: `VER-H` (Verifying), `BLK-H` (Blocked), `ANZ-H` (Analyzing), `APR-H` (AwaitingApproval), `WEC` (WFE Creation), `AT` (ApplyTransition), `TI` (TransitionIntent)

### Tier 1: Unit Tests

**Testable code scope**: Handler files + `phase/` package. >=80% coverage target.

#### 8.1 VerifyingHandler (`verifying_handler.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-VER-H-001` | VerifyingHandler implements PhaseHandler interface | P0 | Pass |
| `UT-VER-H-002` | Phase() returns Verifying | P0 | Pass |
| `UT-VER-H-003` | EnsureNotificationsCreated called when Outcome is set (BR-ORCH-045) | P0 | Pass |
| `UT-VER-H-004` | EnsureNotificationsCreated NOT called when Outcome is empty | P1 | Pass |
| `UT-VER-H-005` | EA ref nil, creation callback still leaves nil → requeue at RequeueResourceBusy | P0 | Pass |
| `UT-VER-H-006` | EA ref nil, creation callback sets ref → proceeds to deadline check | P1 | Pass |
| `UT-VER-H-007` | EA Get error returns requeue at RequeueResourceBusy | P0 | Pass |
| `UT-VER-H-008` | ValidityDeadline set → VerificationDeadline populated (+ buffer) | P0 | Pass |
| `UT-VER-H-009` | ValidityDeadline nil, age < timeout → requeue | P1 | Pass |
| `UT-VER-H-010` | Safety-net timeout: age > verifying timeout → Completed/VerificationTimedOut + metrics | P0 | Pass |
| `UT-VER-H-011` | Expired VerificationDeadline → Completed/VerificationTimedOut + audit emitted | P0 | Pass |
| `UT-VER-H-012` | trackEffectivenessStatus mutates phase to Completed → handler returns NoOp + audit | P0 | Pass |
| `UT-VER-H-013` | trackEffectivenessStatus error is non-fatal → requeue | P1 | Pass |
| `UT-VER-H-014` | Non-terminal EA with active deadline → requeue at RequeueResourceBusy | P0 | Pass |

#### 8.2 BlockedHandler (`blocked_handler.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-BLK-H-001` | BlockedHandler implements PhaseHandler interface | P0 | Pending |
| `UT-BLK-H-002` | Phase() returns Blocked | P0 | Pending |
| `UT-BLK-H-003` | BlockedUntil nil + ResourceBusy → delegates to recheckResourceBusy callback | P0 | Pending |
| `UT-BLK-H-004` | BlockedUntil nil + DuplicateInProgress → delegates to recheckDuplicateBlock callback | P0 | Pending |
| `UT-BLK-H-005` | BlockedUntil nil + manual block (unknown reason) → NoOp (no auto-expiry) | P1 | Pending |
| `UT-BLK-H-006` | BlockedUntil expired + UnmanagedResource → delegates to handleUnmanagedResourceExpiry (BR-SCOPE-010) | P0 | Pending |
| `UT-BLK-H-007` | BlockedUntil expired + other reason → Failed terminal + metrics gauge decrement (BR-ORCH-042) | P0 | Pending |
| `UT-BLK-H-008` | BlockedUntil in future → RequeueAfter at exact expiry time | P0 | Pending |

#### 8.3 Shared WFE Creation Utility (`wfe_creation.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-WEC-001` | Successful WFE creation returns Advance intent to Executing + WFE ref set on RR | P0 | Pending |
| `UT-WEC-002` | Lock acquisition failure → requeue at RequeueGenericError | P0 | Pending |
| `UT-WEC-003` | Lock contention (ResourceBusy) → Block intent with ResourceBusy reason | P1 | Pending |
| `UT-WEC-004` | WFE Create error → requeue + lock released | P0 | Pending |
| `UT-WEC-005` | Routing engine blocks post-analysis → Block intent (BR-ORCH-042) | P0 | Pending |
| `UT-WEC-006` | Status patch error after WFE create is non-fatal → still returns Advance | P1 | Pending |

#### 8.4 AnalyzingHandler (`analyzing_handler.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-ANZ-H-001` | AnalyzingHandler implements PhaseHandler interface | P0 | Pending |
| `UT-ANZ-H-002` | Phase() returns Analyzing | P0 | Pending |
| `UT-ANZ-H-003` | No AI ref → requeue at RequeueGenericError | P0 | Pending |
| `UT-ANZ-H-004` | AI Get NotFound → requeue | P0 | Pending |
| `UT-ANZ-H-005` | AI Get error → requeue | P0 | Pending |
| `UT-ANZ-H-006` | AI Completed + WorkflowNotNeeded → Advance to Completed (BR-ORCH-036) | P0 | Pending |
| `UT-ANZ-H-007` | AI Completed + approval required → RAR created + Advance to AwaitingApproval | P0 | Pending |
| `UT-ANZ-H-008` | AI Completed + direct execution → WFE created + Advance to Executing (BR-ORCH-037) | P0 | Pending |
| `UT-ANZ-H-009` | AI Completed + stale cache (phase mismatch) → requeue (no-op) | P1 | Pending |
| `UT-ANZ-H-010` | AI Completed + missing RemediationTarget → Failed | P0 | Pending |
| `UT-ANZ-H-011` | AI Completed + routing blocked post-analysis → Block intent | P0 | Pending |
| `UT-ANZ-H-012` | AI Completed + lock acquisition failure → requeue | P1 | Pending |
| `UT-ANZ-H-013` | AI Completed + lock contention → Block intent with ResourceBusy | P1 | Pending |
| `UT-ANZ-H-014` | AI Completed + WFE creation failure → requeue + lock released | P0 | Pending |
| `UT-ANZ-H-015` | AI Completed + pre-hash hard conflict → Failed | P1 | Pending |
| `UT-ANZ-H-016` | AI Completed + pre-hash soft conflict → requeue | P1 | Pending |
| `UT-ANZ-H-017` | AI Failed → Failed transition + event emitted | P0 | Pending |
| `UT-ANZ-H-018` | AI in progress (Pending/Investigating) → requeue | P0 | Pending |
| `UT-ANZ-H-019` | AI unknown phase → requeue | P1 | Pending |
| `UT-ANZ-H-020` | RAR creation failure → requeue (non-fatal) | P1 | Pending |

#### 8.5 AwaitingApprovalHandler (`awaiting_approval_handler.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-APR-H-001` | AwaitingApprovalHandler implements PhaseHandler interface | P0 | Pending |
| `UT-APR-H-002` | Phase() returns AwaitingApproval | P0 | Pending |
| `UT-APR-H-003` | RAR Get NotFound → requeue | P0 | Pending |
| `UT-APR-H-004` | RAR Get error → requeue | P0 | Pending |
| `UT-APR-H-005` | RAR Approved + override permanent → WFE created + Advance to Executing (BR-ORCH-095) | P0 | Pending |
| `UT-APR-H-006` | RAR Approved + override transient error → requeue | P1 | Pending |
| `UT-APR-H-007` | RAR Approved + routing blocked → Block intent | P0 | Pending |
| `UT-APR-H-008` | RAR Approved + lock failure → requeue | P1 | Pending |
| `UT-APR-H-009` | RAR Approved + WFE creation failure → requeue + lock released | P0 | Pending |
| `UT-APR-H-010` | RAR Rejected → Failed transition (BR-ORCH-026) | P0 | Pending |
| `UT-APR-H-011` | RAR Expired → Failed transition | P0 | Pending |
| `UT-APR-H-012` | RAR pending + deadline passed → expire RAR + Failed | P0 | Pending |
| `UT-APR-H-013` | RAR pending + deadline active → update TimeRemaining + requeue | P1 | Pending |

#### 8.6 Already-Completed Handlers (Pass — retroactive documentation)

**ExecutingHandler** (`executing_handler.go`) — 15 unit tests (Pass)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-EXE-001` | Implements PhaseHandler interface | P0 | Pass |
| `UT-EXE-002` | Phase() returns Executing | P0 | Pass |
| `UT-EXE-003` | WFE ref nil → requeue | P0 | Pass |
| `UT-EXE-004` | WFE Get error → requeue | P0 | Pass |
| `UT-EXE-005` | WFE Completed (success) → Verifying intent | P0 | Pass |
| `UT-EXE-006` | WFE Failed → Failed intent | P0 | Pass |
| `UT-EXE-007` | WFE TimedOut → Failed intent | P0 | Pass |
| `UT-EXE-008` | Dedup: original WFE Completed → InheritedCompleted | P0 | Pass |
| `UT-EXE-009` | Dedup: original WFE Failed → InheritedFailed | P0 | Pass |
| `UT-EXE-010` | Dedup: original WFE still running → requeue | P1 | Pass |
| `UT-EXE-011` | Dedup: original WFE Get error → requeue | P1 | Pass |
| `UT-EXE-012` | AggregateStatus error → requeue (non-fatal) | P1 | Pass |
| `UT-EXE-013` | WFE Running → requeue at 10s | P1 | Pass |
| `UT-EXE-014` | WFE empty phase → requeue at 10s | P1 | Pass |
| `UT-EXE-015` | WFE unknown phase → requeue at 10s | P1 | Pass |

**PendingHandler** (`pending_handler.go`) — 7 unit tests (Pass)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-PND-H-001` | Implements PhaseHandler interface | P0 | Pass |
| `UT-PND-H-002` | Phase() returns Pending | P0 | Pass |
| `UT-PND-H-003` | Routing engine error → requeue at RequeueGenericError | P0 | Pass |
| `UT-PND-H-004` | Routing blocked → Block intent | P0 | Pass |
| `UT-PND-H-005` | Routing allows → SP created + Advance to Processing | P0 | Pass |
| `UT-PND-H-006` | SP creation error → requeue | P0 | Pass |
| `UT-PND-H-007` | Namespace terminating → NoOp | P1 | Pass |

**ProcessingHandler** (`processing_handler.go`) — 11 unit tests (Pass)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-PRC-H-001` | Implements PhaseHandler interface | P0 | Pass |
| `UT-PRC-H-002` | Phase() returns Processing | P0 | Pass |
| `UT-PRC-H-003` | SP ref nil → requeue | P0 | Pass |
| `UT-PRC-H-004` | SP Get error → requeue | P0 | Pass |
| `UT-PRC-H-005` | SP Completed → Advance to Analyzing | P0 | Pass |
| `UT-PRC-H-006` | SP Failed → Failed intent | P0 | Pass |
| `UT-PRC-H-007` | SP Cancelled → Failed intent | P0 | Pass |
| `UT-PRC-H-008` | SP Pending → requeue at 10s | P0 | Pass |
| `UT-PRC-H-009` | SP Processing → requeue at 10s | P1 | Pass |
| `UT-PRC-H-010` | SP empty phase → requeue at 10s | P1 | Pass |
| `UT-PRC-H-011` | SP unknown phase → requeue at 10s | P1 | Pass |

**TransitionIntent** (`phase/transition.go`) — 40 unit tests (Pass)
**ApplyTransition** (`apply_transition.go`) — 12 unit tests (Pass)
**Characterization Suite** (`characterization_test.go`) — 24 unit tests (Pass)

### Tier 2: Integration Tests

**Testable code scope**: `internal/controller/remediationorchestrator/` reconciler wiring. >=80% coverage target.

Integration tests use envtest (real K8s API) and validate full reconciler dispatch through the registry.

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `IT-RO-666-001` | Verifying phase: EA terminal → RR transitions to Completed via registry dispatch | P0 | Pending |
| `IT-RO-666-002` | Verifying phase: deadline expiry → Completed/VerificationTimedOut via registry | P0 | Pending |
| `IT-RO-666-003` | Blocked phase: cooldown expiry → Failed terminal via registry dispatch | P0 | Pending |
| `IT-RO-666-004` | Blocked phase: DuplicateInProgress recheck unblocks → resumes via registry | P1 | Pending |
| `IT-RO-666-005` | Analyzing phase: AI Completed + direct execution → Executing via registry | P0 | Pending |
| `IT-RO-666-006` | Analyzing phase: AI Completed + approval → AwaitingApproval via registry | P0 | Pending |
| `IT-RO-666-007` | AwaitingApproval phase: RAR Approved → Executing via registry | P0 | Pending |
| `IT-RO-666-008` | AwaitingApproval phase: RAR Rejected → Failed via registry | P0 | Pending |
| `IT-RO-666-009` | Full pipeline: Pending → Processing → Analyzing → Executing → Verifying → Completed (all via registry) | P0 | Pending |
| `IT-RO-666-010` | Error propagation: handler error flows to controller-runtime backoff | P0 | Pending |

### Tier Skip Rationale

- **E2E**: This is an internal refactoring with no behavioral change visible at the deployment level. Existing E2E suite provides regression coverage. No new E2E tests needed.

---

## 9. Test Cases

### UT-VER-H-010: Safety-net timeout transitions to Completed

**BR**: BR-EM-010, BR-ORCH-044
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/verifying_handler_test.go`

**Preconditions**:
- RR in Verifying phase with Outcome="Remediated"
- EA ref set, EA exists but ValidityDeadline is nil
- RR CreationTimestamp is 15 minutes ago
- TimeoutConfig.Verifying = 10 minutes

**Test Steps**:
1. **Given**: RR in Verifying phase older than verifying timeout, EA without ValidityDeadline
2. **When**: Handle() is called
3. **Then**: Returns NoOp intent (status already updated in-handler), metrics incremented, timeout audit emitted

**Expected Results**:
1. `intent.Type == TransitionNone` (status update performed in-handler)
2. `EmitVerificationTimedOutAudit` callback invoked
3. RR status updated: `OverallPhase=Completed, Outcome=VerificationTimedOut, CompletedAt!=nil`

**Acceptance Criteria**:
- **Behavior**: Handler transitions RR to Completed on safety-net timeout
- **Correctness**: Outcome is "VerificationTimedOut", not "Remediated"
- **Accuracy**: Metrics gauge incremented for Verifying→Completed transition

---

### UT-ANZ-H-008: AI Completed + direct execution creates WFE

**BR**: BR-ORCH-036, BR-ORCH-037
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/analyzing_handler_test.go`

**Preconditions**:
- RR in Analyzing phase with AI ref set
- AI analysis Completed with RemediationTarget and no approval required
- Routing engine allows execution
- Lock acquisition succeeds

**Test Steps**:
1. **Given**: RR in Analyzing with completed AI analysis, approval not required
2. **When**: Handle() is called
3. **Then**: WFE created, RR status patched with WFE ref, returns Advance to Executing

**Expected Results**:
1. `intent.Type == TransitionAdvance`
2. `intent.TargetPhase == phase.Executing`
3. WFE CRD created in K8s with correct owner reference

**Acceptance Criteria**:
- **Behavior**: Direct-to-execution path bypasses approval when not required
- **Correctness**: WFE ref set on RR status before transition
- **Accuracy**: Lock acquired before WFE creation, released on error path

---

### UT-BLK-H-007: Cooldown expired → Failed terminal

**BR**: BR-ORCH-042
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/blocked_handler_test.go`

**Preconditions**:
- RR in Blocked phase with BlockedUntil in the past
- BlockReason is NOT UnmanagedResource

**Test Steps**:
1. **Given**: RR blocked with expired cooldown (non-UnmanagedResource)
2. **When**: Handle() is called
3. **Then**: Returns Failed intent with FailurePhaseBlocked, metrics gauge decremented

**Expected Results**:
1. `intent.Type == TransitionFailed`
2. `intent.FailurePhase == FailurePhaseBlocked`
3. CurrentBlockedGauge decremented for namespace

**Acceptance Criteria**:
- **Behavior**: Expired cooldown transitions to terminal Failed (not retry)
- **Correctness**: FailurePhase is Blocked, not generic
- **Accuracy**: Metrics gauge accurately decremented

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Callback functions for heavy reconciler deps (notifications, EA creation, audit emission, tracking); `fake.NewClientBuilder()` for K8s API
- **Location**: `test/unit/remediationorchestrator/controller/`
- **Resources**: Minimal (no external deps)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (real K8s API via envtest)
- **Infrastructure**: envtest (kube-apiserver + etcd), Podman for PostgreSQL/Redis (audit store)
- **Location**: `test/integration/remediationorchestrator/`
- **Resources**: ~2GB RAM for envtest + containers

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| envtest | 1.35.0 | K8s API simulation |
| Podman | 4.x | PostgreSQL/Redis for integration tests |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| TransitionIntent (M2) | Code | Merged | All handler tests blocked | N/A (completed) |
| ApplyTransition (M2) | Code | Merged | Registry wiring blocked | N/A (completed) |
| Executing/Pending/Processing (M3.1-3.3) | Code | Merged | Pattern reference unavailable | N/A (completed) |

### 11.2 Execution Order

1. **Phase 1** (M3.4): VerifyingHandler — characterization tests (RED) → handler (GREEN) → wire (REFACTOR)
2. **Phase 2** (M3.5): BlockedHandler — characterization tests → handler → wire
3. **CHECKPOINT 3b**: Midpoint audit after 5/8 handlers
4. **Phase 3** (M3.6): Shared WFE creation utility — unit tests → implementation
5. **Phase 4** (M3.7): AnalyzingHandler — characterization tests → handler → wire
6. **Phase 5** (M3.8): AwaitingApprovalHandler — characterization tests → handler → wire
7. **CHECKPOINT 3c**: Complex handler audit (Analyzing/AwaitingApproval)
8. **Phase 6** (M4): Wire all remaining handlers, remove legacy switch, validate reconciler.go < 400 lines
9. **CHECKPOINT 4**: Full test pyramid 3x, architecture invariants
10. **Phase 7** (M5): ADR + Documentation
11. **Phase 8**: Integration tests IT-RO-666-001 through IT-RO-666-010

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/666/TEST_PLAN.md` | Strategy and test design |
| VerifyingHandler unit tests | `test/unit/remediationorchestrator/controller/verifying_handler_test.go` | 14 Ginkgo BDD specs |
| BlockedHandler unit tests | `test/unit/remediationorchestrator/controller/blocked_handler_test.go` | 8 Ginkgo BDD specs |
| WFE Creation unit tests | `test/unit/remediationorchestrator/controller/wfe_creation_test.go` | 6 Ginkgo BDD specs |
| AnalyzingHandler unit tests | `test/unit/remediationorchestrator/controller/analyzing_handler_test.go` | 20 Ginkgo BDD specs |
| AwaitingApprovalHandler unit tests | `test/unit/remediationorchestrator/controller/awaiting_approval_handler_test.go` | 13 Ginkgo BDD specs |
| Integration wiring tests | `test/integration/remediationorchestrator/registry_dispatch_integration_test.go` | 10 Ginkgo BDD specs |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests — all #666 handler tests
go test ./test/unit/remediationorchestrator/controller/... -v -count=1 --ginkgo.focus="Issue #666"

# Unit tests — specific handler
go test ./test/unit/remediationorchestrator/controller/... -v -count=1 --ginkgo.focus="UT-VER-H"
go test ./test/unit/remediationorchestrator/controller/... -v -count=1 --ginkgo.focus="UT-BLK-H"
go test ./test/unit/remediationorchestrator/controller/... -v -count=1 --ginkgo.focus="UT-ANZ-H"
go test ./test/unit/remediationorchestrator/controller/... -v -count=1 --ginkgo.focus="UT-APR-H"

# Characterization tests
go test ./test/unit/remediationorchestrator/controller/... -v -count=1 --ginkgo.focus="UT-RO-CHAR"

# Full unit suite (regression check)
go test ./test/unit/remediationorchestrator/controller/... -count=1

# Integration tests
go test ./test/integration/remediationorchestrator/... -v -count=1 --ginkgo.focus="IT-RO-666"

# Coverage
go test ./test/unit/remediationorchestrator/controller/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None anticipated | N/A | N/A | Refactoring preserves behavior; characterization suite guards against regressions |

If a characterization test fails during extraction, it indicates a behavioral change that must be investigated and fixed — not a test update.

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan covering all 8 handler extractions + registry wiring |
