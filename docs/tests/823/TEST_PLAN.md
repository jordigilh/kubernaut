# Test Plan: Session Store Cancellation Infrastructure (#823)

> **Template**: IEEE 829-2008 + Kubernaut Hybrid v2.0

**Test Plan Identifier**: TP-823-v1.0
**Feature**: Session cancellation infrastructure — terminal state guards, context-driven cancellation, and event channel scaffolding for live investigation streaming.
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant
**Status**: Active — All Checkpoints Passed
**Branch**: `feature/pr1-session-cancel-infra`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the cancellation infrastructure added to the Kubernaut Agent session store and manager as the first PR in the v1.5 streaming and cancellation feature set (#822). The changes introduce terminal state immutability, operator-initiated cancellation, and event channel scaffolding that subsequent PRs will use for live investigation streaming.

The test plan provides behavioral assurance that:
- Investigations reaching a terminal state (completed, cancelled, failed) are immutable
- Operators can cancel active investigations, stopping the running LLM analysis
- Event channels are scaffolded for future observer delivery without breaking the existing autonomous flow
- All changes are backward-compatible with the existing v1.4 session lifecycle

### 1.2 Objectives

1. **Terminal state immutability**: Completed, cancelled, and failed investigations reject all subsequent state changes
2. **Cancellation effectiveness**: Cancelling an active investigation propagates to the background goroutine and stops analysis
3. **Error semantics**: Cancelling or subscribing to nonexistent or already-terminal investigations returns clear, typed errors
4. **Observation scaffolding**: Event channels are created and closed with correct lifecycle, ready for PR 4
5. **Zero regression**: All 14 existing session tests (10 UT + 4 IT) pass without modification
6. **Per-tier coverage**: >=80% on both unit-testable and integration-testable session code

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test -race ./test/unit/kubernautagent/session/...` |
| Integration test pass rate | 100% | `go test -race ./test/integration/kubernautagent/session/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on store.go + types.go |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on manager.go |
| Backward compatibility | 0 regressions | All 14 existing tests pass without modification |
| Data races | 0 | `go test -race` on both tiers |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-SESSION-001: Investigation cancellation and terminal state immutability (working definition — see Section 2.3)
- BR-SESSION-002: Session state query accuracy (working definition — see Section 2.3)
- BR-SESSION-003: Live event observation for active investigations (working definition — see Section 2.3)
- BR-SESSION-007: Runtime-agnostic event types for SSE contract stability (working definition — see Section 2.3)
- Issue [#823](https://github.com/jordigilh/kubernaut/issues/823): Session Store Cancellation Infrastructure
- Issue [#822](https://github.com/jordigilh/kubernaut/issues/822): v1.5 Streaming and Cancellation (parent)
- PROPOSAL-EXT-003: Goose Runtime Evaluation

### 2.2 Cross-References

- [TP-433-v1.0](../433/TEST_PLAN.md) — Kubernaut Agent session management (parent test plan)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

### 2.3 Business Requirements — Working Definitions

These BRs do not yet have formal `BUSINESS_REQUIREMENTS.md` definitions in the session domain. The following working definitions are authoritative for this test plan and will be superseded by formal definitions when created.

| BR ID | Working Definition | Source |
|-------|-------------------|--------|
| BR-SESSION-001 | An operator can cancel an autonomous investigation in progress. The cancellation is final: once cancelled, the investigation state is immutable and cannot be overwritten by any concurrent process. This immutability extends to all terminal states (completed, failed, cancelled). | Inline in `manager_test.go`, `store_test.go` |
| BR-SESSION-002 | The session store accurately reflects the current state of every investigation. Querying a cancelled, completed, or failed investigation returns the correct terminal state. | Derived from BR-SESSION-001 state integrity guarantee; formalized here. |
| BR-SESSION-003 | Observers can subscribe to live events from an active investigation and are notified when the investigation concludes. | Inline in `manager_test.go` |
| BR-SESSION-007 | Investigation event types are runtime-agnostic, providing a stable SSE contract across runtime migrations (current LangChainGo to future Goose ACP). See PROPOSAL-EXT-003. | `types.go` comment |

**Action item**: Create `docs/services/kubernautagent/session/BUSINESS_REQUIREMENTS.md` with formal definitions after PR 1 merges.

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Cancel + goroutine race on status: concurrent cancel endpoint and investigation goroutine both attempt to write status | Corrupted session state or data race | High | UT-KA-823-001..004, IT-KA-823-001, IT-KA-823-004 | Terminal state guard under lock + `-race` flag on all tests |
| R2 | Cleanup TTL deletes a running session: housekeeping fires while investigation is active | Active investigation loses its session, goroutine orphaned | Medium | UT-KA-823-006 | `Cleanup()` skips `StatusRunning` sessions |
| R3 | Event channel backpressure blocks `runLLMLoop` in future PRs | Investigation stalls if channel is full (PR 4+ risk) | Low | IT-KA-823-005 | Buffered channel (64 events) with documented capacity. Non-blocking send deferred to PR 4. |
| R4 | Cancel/eventChan exposed via clone() | Caller obtains internal control references and interferes with investigation | Medium | UT-KA-823-007 | Fields unexported; `clone()` excludes them |
| R5 | Double-cancel panics: calling cancel() on already-cancelled context | Runtime panic crashes KA | Medium | IT-KA-823-003 | Terminal state guard checks status before calling cancel() |
| R6 | Manager accesses Store internal map directly (bypassing Store API) | Tight coupling makes future refactoring fragile | Low | IT-KA-823-001 | Documented in code; lock discipline verified at CHECKPOINT 3 |

### 3.1 Risk-to-Test Traceability

| Risk | Mitigating Tests |
|------|-----------------|
| R1 | UT-KA-823-001, UT-KA-823-002, UT-KA-823-003, UT-KA-823-004, IT-KA-823-001, IT-KA-823-004 |
| R2 | UT-KA-823-006 |
| R3 | IT-KA-823-005 |
| R4 | UT-KA-823-007 |
| R5 | IT-KA-823-003 |
| R6 | IT-KA-823-001 (verifies cancel propagates through Manager -> Store -> goroutine chain) |

---

## 4. Scope

### 4.1 Features to be Tested

- **Session state integrity** (`internal/kubernautagent/session/store.go`): Terminal state immutability guards on `Update()`, `StatusCancelled` constant, `Cleanup()` protection for running sessions, isolation of internal control fields from callers
- **Investigation lifecycle** (`internal/kubernautagent/session/manager.go`): Context-driven cancellation via `CancelInvestigation()`, event channel scaffolding via `Subscribe()`, goroutine cleanup on investigation exit
- **Event type definitions** (`internal/kubernautagent/session/types.go`): Runtime-agnostic event type constants and `InvestigationEvent` struct

### 4.2 Features Not to be Tested

- **HTTP endpoint for cancel** (`POST /api/v1/incident/session/{id}/cancel`): Deferred to PR 2
- **SSE streaming endpoint** (`GET /api/v1/incident/session/{id}/stream`): Deferred to PR 3-5
- **Event emission during LLM loop**: Deferred to PR 4
- **Authorization/authentication**: Deferred to PR 2 (KA endpoint auth)
- **Goose runtime integration**: Out of scope; event types designed for compatibility but no Goose code in this PR

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `context.WithCancel` on `context.Background()` for investigation goroutine | The investigation must outlive the HTTP request. Cancel is explicit via `CancelInvestigation()`, not via request context. |
| Buffered event channel (64 events) | Provides headroom for bursty LLM output without blocking. Non-blocking send deferred to PR 4. |
| Unexported `cancel`/`eventChan` on Session | These are internal control mechanisms. Exposing them would allow callers to bypass the Manager API. |
| `ErrSessionTerminal` sentinel error | Typed error allows callers to distinguish "session exists but is terminal" from "session not found" without string matching. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`store.go` pure logic: `isTerminal()`, `Update()` guard, `Cleanup()` skip, `clone()` isolation)
- **Integration**: >=80% of integration-testable code (`manager.go`: `StartInvestigation` with cancel context, `CancelInvestigation`, `Subscribe`, `closeEventChan`, goroutine lifecycle)
- **E2E**: Not applicable for this PR (no HTTP endpoints added)

### 5.2 Two-Tier Minimum

Every BR is covered by at least 2 tiers:

| BR | Unit Coverage | Integration Coverage |
|----|--------------|---------------------|
| BR-SESSION-001 | UT-KA-823-001..004, UT-KA-823-006, UT-KA-823-007 | IT-KA-823-001..004 |
| BR-SESSION-002 | UT-KA-823-005 | IT-KA-823-001 (state verified after cancel) |
| BR-SESSION-003 | — | IT-KA-823-005, IT-KA-823-006, IT-KA-823-007 |
| BR-SESSION-007 | Compile-time (type constants) | IT-KA-823-005 (event type in channel) |

**Note on BR-SESSION-003**: No unit test tier because event delivery is inherently cross-component (Manager + Store + goroutine). This is documented as a tier skip.

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — what the operator/observer gets — not implementation mechanics. Every test description answers "what does the operator/system experience?" not "what function is called?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage >= 80%
4. Zero regressions in existing 14 session tests
5. Zero data races under `-race`

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage below 80%
3. Any existing test regresses
4. Any data race detected

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Code does not compile (`go build ./...` fails)
- Existing session tests fail before new tests are written (baseline broken)
- Design question requires user input (escalate)

**Resume testing when**:

- Build restored
- Baseline tests passing
- Design question resolved

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Behaviors | Lines (approx) |
|------|-----------|-----------------|
| `internal/kubernautagent/session/store.go` | `isTerminal()`, terminal guard in `Update()`, `StatusCancelled`, `Cleanup()` skip for running, `clone()` isolation of unexported fields | ~156 (will grow ~30 lines) |
| `internal/kubernautagent/session/types.go` | `InvestigationEvent` struct, event type constants | ~30 (new file) |

### 6.2 Integration-Testable Code (I/O, goroutine lifecycle, cross-component)

| File | Behaviors | Lines (approx) |
|------|-----------|-----------------|
| `internal/kubernautagent/session/manager.go` | `StartInvestigation` with `context.WithCancel`, `CancelInvestigation`, `Subscribe`, `closeEventChan`, goroutine cleanup | ~73 (will grow ~60 lines) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.5` HEAD | Branch: `feature/pr1-session-cancel-infra` |
| Parent issue | #822 | v1.5 Streaming and Cancellation |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SESSION-001 | Terminal state immutability | P0 | Unit | UT-KA-823-001 | Pass |
| BR-SESSION-001 | Terminal state immutability | P0 | Unit | UT-KA-823-002 | Pass |
| BR-SESSION-001 | Terminal state immutability | P0 | Unit | UT-KA-823-003 | Pass |
| BR-SESSION-001 | Cancellation transition | P0 | Unit | UT-KA-823-004 | Pass |
| BR-SESSION-001 | Housekeeping safety | P1 | Unit | UT-KA-823-006 | Pass |
| BR-SESSION-001 | Session isolation | P1 | Unit | UT-KA-823-007 | Pass |
| BR-SESSION-001 | Cancel stops analysis | P0 | Integration | IT-KA-823-001 | Pass |
| BR-SESSION-001 | Cancel nonexistent | P0 | Integration | IT-KA-823-002 | Pass |
| BR-SESSION-001 | Cancel terminal | P0 | Integration | IT-KA-823-003 | Pass |
| BR-SESSION-001 | Post-cancel integrity | P0 | Integration | IT-KA-823-004 | Pass |
| BR-SESSION-002 | State query accuracy | P0 | Unit | UT-KA-823-005 | Pass |
| BR-SESSION-003 | Live event delivery | P0 | Integration | IT-KA-823-005 | Pass |
| BR-SESSION-003 | Subscription consistency | P1 | Integration | IT-KA-823-006 | Pass |
| BR-SESSION-003 | End-of-investigation notification | P0 | Integration | IT-KA-823-007 | Pass |
| BR-SESSION-003 | Subscribe error for unknown investigation | P1 | Integration | IT-KA-823-008 | Pass |
| BR-SESSION-003 | Subscribe error for concluded investigation | P1 | Integration | IT-KA-823-009 | Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `KA` (Kubernaut Agent)
- **ISSUE**: `823`
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/session/store.go`, `internal/kubernautagent/session/types.go` — >=80% coverage target

| ID | Business Acceptance Criterion | Phase |
|----|------------------------------|-------|
| UT-KA-823-001 | A completed investigation is immutable — no subsequent status change is accepted | Pass |
| UT-KA-823-002 | A cancelled investigation is immutable — no subsequent status change is accepted | Pass |
| UT-KA-823-003 | A failed investigation is immutable — no subsequent status change is accepted | Pass |
| UT-KA-823-004 | An active investigation can be cancelled by an operator | Pass |
| UT-KA-823-005 | Querying a cancelled investigation accurately reports the cancelled state | Pass |
| UT-KA-823-006 | Active investigations are never removed by housekeeping regardless of age | Pass |
| UT-KA-823-007 | Session metadata returned to callers cannot be used to interfere with active investigations | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/session/manager.go` — >=80% coverage target

| ID | Business Acceptance Criterion | Phase |
|----|------------------------------|-------|
| IT-KA-823-001 | Cancelling an investigation stops the running LLM analysis | Pass |
| IT-KA-823-002 | Cancelling a nonexistent investigation returns a clear not-found error | Pass |
| IT-KA-823-003 | Cancelling an already-completed investigation returns a clear terminal-state error | Pass |
| IT-KA-823-004 | After cancellation, the investigation cannot retroactively report failure | Pass |
| IT-KA-823-005 | An observer can receive live events from an active investigation | Pass |
| IT-KA-823-006 | Multiple subscriptions to the same investigation share a single event stream | Pass |
| IT-KA-823-007 | Observers are notified when an investigation concludes | Pass |
| IT-KA-823-008 | Subscribing to a nonexistent investigation returns a clear error | Pass |
| IT-KA-823-009 | Subscribing to a concluded investigation returns a clear error | Pass |

### Tier 3: E2E Tests

Not applicable for this PR. No HTTP endpoints are added.

### Tier Skip Rationale

- **E2E**: No HTTP endpoints or external service interactions in this PR. Cancel/Subscribe are internal APIs consumed by future PR 2-3 HTTP handlers.
- **BR-SESSION-003 Unit tier**: Event delivery is inherently cross-component (Manager creates channel, goroutine closes it, caller reads from it). Pure unit testing would require mocking the Manager, violating the no-mocks integration policy. Integration tier provides adequate coverage.

---

## 9. Test Cases

### UT-KA-823-001: Completed investigation is immutable

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created with standard TTL
- Session created and updated to `StatusCompleted`

**Test Steps**:
1. **Given**: A session in `StatusCompleted`
2. **When**: An attempt is made to change the status to `StatusRunning`
3. **Then**: The update is rejected with `ErrSessionTerminal`

**Expected Results**:
1. `Update()` returns `ErrSessionTerminal`
2. Session status remains `StatusCompleted` on subsequent `Get()`

**Acceptance Criteria**:
- **Behavior**: Completed investigations cannot change state
- **Correctness**: Error is the sentinel `ErrSessionTerminal`, not a generic error

---

### UT-KA-823-002: Cancelled investigation is immutable

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created with standard TTL
- Session created and updated to `StatusCancelled`

**Test Steps**:
1. **Given**: A session in `StatusCancelled`
2. **When**: An attempt is made to change the status to `StatusFailed`
3. **Then**: The update is rejected with `ErrSessionTerminal`

**Expected Results**:
1. `Update()` returns `ErrSessionTerminal`
2. Session status remains `StatusCancelled` on subsequent `Get()`

**Acceptance Criteria**:
- **Behavior**: Cancelled investigations cannot change state
- **Correctness**: Error is the sentinel `ErrSessionTerminal`

---

### UT-KA-823-003: Failed investigation is immutable

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created with standard TTL
- Session created and updated to `StatusFailed`

**Test Steps**:
1. **Given**: A session in `StatusFailed`
2. **When**: An attempt is made to change the status to `StatusCompleted`
3. **Then**: The update is rejected with `ErrSessionTerminal`

**Expected Results**:
1. `Update()` returns `ErrSessionTerminal`
2. Session status remains `StatusFailed` on subsequent `Get()`

**Acceptance Criteria**:
- **Behavior**: Failed investigations cannot change state
- **Correctness**: Error is the sentinel `ErrSessionTerminal`

---

### UT-KA-823-004: Active investigation can be cancelled

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created with standard TTL
- Session created and updated to `StatusRunning`

**Test Steps**:
1. **Given**: A session in `StatusRunning`
2. **When**: The status is changed to `StatusCancelled`
3. **Then**: The update succeeds

**Expected Results**:
1. `Update()` returns nil
2. Session status is `StatusCancelled` on subsequent `Get()`

**Acceptance Criteria**:
- **Behavior**: Running investigations accept cancellation
- **Correctness**: Status transitions cleanly

---

### UT-KA-823-005: Querying a cancelled investigation reports the cancelled state

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created with standard TTL
- Session created, set to Running, then updated to Cancelled

**Test Steps**:
1. **Given**: A session that has been cancelled
2. **When**: The session is retrieved by ID
3. **Then**: The returned session has `StatusCancelled`

**Expected Results**:
1. `Get()` returns a session with `Status == StatusCancelled`
2. No error

**Acceptance Criteria**:
- **Behavior**: State query reflects actual state
- **Accuracy**: Status field is `StatusCancelled`, not any other value

---

### UT-KA-823-006: Active investigations are never removed by housekeeping

**BR**: BR-SESSION-001
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created with very short TTL (1ms)
- Session created and updated to `StatusRunning`
- Sufficient time elapsed for TTL to expire

**Test Steps**:
1. **Given**: A running session whose TTL has expired
2. **When**: `Cleanup()` executes
3. **Then**: The running session is not removed

**Expected Results**:
1. `Cleanup()` returns 0 (no sessions removed)
2. `Get()` still returns the running session

**Acceptance Criteria**:
- **Behavior**: Housekeeping never disrupts active investigations
- **Correctness**: Only non-running, expired sessions are cleaned up

---

### UT-KA-823-007: Session metadata cannot be used to interfere with active investigations

**BR**: BR-SESSION-001
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Preconditions**:
- Session store created
- Session created with a cancel function and event channel installed (via Manager path)

**Test Steps**:
1. **Given**: A running session with internal control fields set
2. **When**: The session is retrieved via `Get()`
3. **Then**: The returned copy has nil/zero-valued cancel and event channel fields

**Expected Results**:
1. Returned session's unexported fields are zero-valued
2. Modifying the returned session has no effect on the stored session

**Acceptance Criteria**:
- **Behavior**: Internal control references are not exposed to callers
- **Correctness**: `clone()` produces an isolated copy

**Dependencies**: Requires access to unexported fields (test in same package or reflection-based assertion)

---

### IT-KA-823-001: Cancelling an investigation stops the running LLM analysis

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created with default store and logger
- Investigation started with a long-running function that respects context cancellation

**Test Steps**:
1. **Given**: A running investigation whose function blocks on `ctx.Done()`
2. **When**: `CancelInvestigation(id)` is called
3. **Then**: The investigation function's context is cancelled, and the session transitions to `StatusCancelled`

**Expected Results**:
1. `CancelInvestigation()` returns nil
2. Investigation function receives context cancellation (verified by `Eventually`)
3. Session status becomes `StatusCancelled`

**Acceptance Criteria**:
- **Behavior**: Cancel propagates to the running analysis
- **Correctness**: Session reaches `StatusCancelled` deterministically

---

### IT-KA-823-002: Cancelling a nonexistent investigation returns a clear error

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created with empty store

**Test Steps**:
1. **Given**: No investigation exists with the given ID
2. **When**: `CancelInvestigation("nonexistent-id")` is called
3. **Then**: The call returns `ErrSessionNotFound`

**Expected Results**:
1. Error matches `ErrSessionNotFound` sentinel

**Acceptance Criteria**:
- **Behavior**: Clear, typed error for unknown investigation
- **Correctness**: No panic, no state corruption

---

### IT-KA-823-003: Cancelling an already-completed investigation returns a clear error

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created
- Investigation started and completed successfully

**Test Steps**:
1. **Given**: An investigation that has already completed
2. **When**: `CancelInvestigation(id)` is called
3. **Then**: The call returns `ErrSessionTerminal`

**Expected Results**:
1. Error matches `ErrSessionTerminal` sentinel
2. Session status remains `StatusCompleted`

**Acceptance Criteria**:
- **Behavior**: Terminal investigations cannot be cancelled (R5 mitigation)
- **Correctness**: No panic from double cancel or post-terminal cancel

---

### IT-KA-823-004: After cancellation, the investigation cannot retroactively report failure

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created
- Investigation started with a function that returns an error after detecting context cancellation

**Test Steps**:
1. **Given**: A running investigation
2. **When**: `CancelInvestigation(id)` is called, and the investigation function subsequently returns an error
3. **Then**: The session status is `StatusCancelled`, not `StatusFailed`

**Expected Results**:
1. Session status is `StatusCancelled`
2. The goroutine's attempt to set `StatusFailed` is rejected by the terminal guard

**Acceptance Criteria**:
- **Behavior**: Cancellation takes precedence over the analysis outcome
- **Correctness**: Terminal state guard prevents race (R1 mitigation)

---

### IT-KA-823-005: An observer can receive live events from an active investigation

**BR**: BR-SESSION-003
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created
- Investigation started

**Test Steps**:
1. **Given**: A running investigation
2. **When**: `Subscribe(id)` is called
3. **Then**: A readable event channel is returned

**Expected Results**:
1. `Subscribe()` returns a non-nil channel and no error
2. The channel is open (not closed)

**Acceptance Criteria**:
- **Behavior**: Observers can subscribe to active investigations
- **Correctness**: Channel is usable for reading events

---

### IT-KA-823-006: Multiple subscriptions share a single event stream

**BR**: BR-SESSION-003
**Priority**: P1
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created
- Investigation started

**Test Steps**:
1. **Given**: A running investigation
2. **When**: `Subscribe(id)` is called twice
3. **Then**: Both calls return the same channel

**Expected Results**:
1. Channel reference from first call == channel reference from second call

**Acceptance Criteria**:
- **Behavior**: Single event stream per investigation (no fan-out duplication)
- **Correctness**: Channel identity is stable

---

### IT-KA-823-007: Observers are notified when an investigation concludes

**BR**: BR-SESSION-003
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created
- Investigation started with a function that completes quickly
- Observer subscribed to the investigation

**Test Steps**:
1. **Given**: An observer subscribed to an active investigation
2. **When**: The investigation completes (function returns)
3. **Then**: The event channel is closed

**Expected Results**:
1. Reading from the channel eventually returns the zero value (channel closed)

**Acceptance Criteria**:
- **Behavior**: Observers know when the investigation ends
- **Correctness**: Channel is closed exactly once, no panic from double-close

---

### IT-KA-823-008: Subscribing to a nonexistent investigation returns a clear error

**BR**: BR-SESSION-003
**Priority**: P1
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_test.go`

**Preconditions**:
- Manager created with empty store

**Test Steps**:
1. **Given**: No investigation exists with the given ID
2. **When**: `Subscribe("nonexistent-id")` is called
3. **Then**: The call returns nil channel and `ErrSessionNotFound`

**Expected Results**:
1. Channel is nil
2. Error matches `ErrSessionNotFound` sentinel

**Acceptance Criteria**:
- **Behavior**: Clear, typed error for unknown investigation
- **Correctness**: No panic, no dangling channel

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None
- **Location**: `test/unit/kubernautagent/session/`
- **Resources**: Standard (no external dependencies)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: None (in-process Manager + Store, no external services)
- **Location**: `test/integration/kubernautagent/session/`
- **Resources**: Standard (goroutine-based, no containers)

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| `go test -race` | Built-in | Data race detection |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. This PR has zero external dependencies.

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write all 14 failing tests with minimal stubs
2. **Phase 2 (GREEN)**: Implement minimal code to pass all 14 tests
3. **Phase 3 (REFACTOR)**: Improve code quality without changing behavior

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/823/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/session/store_test.go` | 7 new Ginkgo specs added to existing file |
| Integration test suite | `test/integration/kubernautagent/session/manager_test.go` | 7 new Ginkgo specs added to existing file |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test -race -v ./test/unit/kubernautagent/session/...

# Integration tests
go test -race -v ./test/integration/kubernautagent/session/...

# Specific test by ID
go test -race -v ./test/unit/kubernautagent/session/... -ginkgo.focus="UT-KA-823"

# Coverage (unit)
go test -coverprofile=cover-ut.out ./test/unit/kubernautagent/session/...
go tool cover -func=cover-ut.out

# Coverage (integration)
go test -coverprofile=cover-it.out ./test/integration/kubernautagent/session/...
go tool cover -func=cover-it.out
```

---

## 14. Existing Tests Requiring Updates

None. All 14 existing session tests (10 UT + 4 IT) must pass without modification. The terminal state guard in `Update()` does not affect existing tests because:
- Existing tests never attempt to update a terminal-state session
- `Cleanup()` behavior change (skip running) adds protection but doesn't break existing TTL tests (they test expired non-running sessions)

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan (14 scenarios: 7 UT + 7 IT) |
| 1.1 | 2026-04-24 | Added IT-KA-823-008 (subscribe nonexistent) from CHECKPOINT 1 gap analysis |
| 1.2 | 2026-04-24 | Added IT-KA-823-009 (subscribe after conclusion) from CHECKPOINT 3 rework loop |
| 1.3 | 2026-04-24 | All 16 scenarios passing. Coverage: 97.2% merged, ~99% UT on store.go, ~96% IT on manager.go |
