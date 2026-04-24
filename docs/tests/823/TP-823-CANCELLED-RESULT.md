# Test Plan: CancelledResult + Between-Turn Checkpoint

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-823-CR-v1.0
**Feature**: CancelledResult LoopResult type, between-turn checkpoint in runLLMLoop, context-carried event sink infrastructure
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Active
**Branch**: `feature/pr3-cancelled-result`

---

## 1. Introduction

### 1.1 Purpose

PR 1 and PR 1.5 established session cancellation infrastructure and audit trails. When
`CancelInvestigation` is called, the context passed to the investigation goroutine is
cancelled. However, the investigator's `runLLMLoop` does not distinguish cancellation from
generic errors, and retry paths (`retryRCASubmit`, `retryWorkflowSubmit`) actively mask
`context.Canceled` via `continue` on `client.Chat` errors. This means cancellation can
appear as a degraded investigation result instead of a clean abort with preserved state.

This test plan covers PR3: adding a `CancelledResult` type to the sealed `LoopResult`
interface, introducing between-turn cancellation checkpoints in `runLLMLoop`, fast-abort
guards in retry loops, `CancelledResult` handling in all three `LoopResult` type switches,
phase-level short-circuiting in `Investigate`, and context-carried event sink infrastructure
in the session Manager.

### 1.2 Objectives

1. **Clean cancellation**: `runLLMLoop` returns `CancelledResult` (not an error) when context is cancelled, preserving accumulated messages, turn count, phase, and token usage
2. **Fast abort**: Retry loops (`retryRCASubmit`, `retryWorkflowSubmit`) detect cancelled context and abort immediately instead of masking
3. **Phase short-circuit**: `Investigate` stops after RCA if cancelled, avoiding unnecessary workflow selection LLM calls
4. **Partial result preservation**: Cancelled investigations store a partial `InvestigationResult` with `Cancelled: true` on the session for snapshot retrieval (BR-SESSION-002)
5. **Zero regression**: All existing v1.4, PR1, PR1.5, and PR2 tests pass without modification
6. **Event sink infrastructure**: Context-carried event sink helpers (`WithEventSink`/`EventSinkFromContext`) are wired and round-trip tested (events emitted in PR4)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/investigator/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| Unit-testable code coverage (investigator.go new paths) | >=80% | `go test -coverprofile -coverpkg=.../investigator` |
| Integration-testable code coverage (manager.go new paths) | >=80% | `go test -coverprofile -coverpkg=.../session` |
| Backward compatibility | 0 regressions | Full test suite passes: `go build ./... && make test` |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-SESSION-001**: Operator can cancel an autonomous investigation in progress
- **BR-SESSION-002**: Cancelled investigation preserves accumulated context (snapshot)
- **BR-SESSION-005**: All session control actions are audited (cancel, observe)
- **BR-SESSION-007**: Streaming event types are runtime-agnostic, forward-compatible with Goose ACP
- **ADR-038**: Async Buffered Audit Ingestion — fire-and-forget semantics
- **Issue #823**: Session Store Cancellation Infrastructure (parent issue)
- **TP-823-v1.0**: Companion test plan for PR1 cancellation infrastructure
- **TP-823-AUDIT-v1.0**: Companion test plan for PR1.5 audit trail
- **TP-823-OAS-ENDPOINTS-v1.0**: Companion test plan for PR2 OAS endpoints

### 2.2 Business Requirements — Working Definitions

| BR ID | Definition |
|-------|-----------|
| BR-SESSION-001 | Operator can cancel a running investigation; the system MUST abort promptly (within 1 turn boundary) |
| BR-SESSION-002 | Cancelled investigation preserves accumulated context: messages, turn count, phase, token usage |
| BR-SESSION-007 | Event types are runtime-agnostic; context-carried event sink decouples emission from transport |

### 2.3 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [PR1 Test Plan](TEST_PLAN.md)
- [PR1.5 Audit Test Plan](TP-823-AUDIT.md)
- [PR2 OAS Endpoints Test Plan](TP-823-OAS-ENDPOINTS.md)
- [v1.5 Streaming Plan](/.cursor/plans/ka_session_streaming_42173e7a.plan.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Retry loops mask `context.Canceled` via `continue` — cancelled investigation appears as degraded result | Violates BR-SESSION-002; misleading partial results stored | High | UT-KA-823-C03, UT-KA-823-C04 | Add `ctx.Err()` fast-abort at top of each retry iteration |
| R2 | Three `LoopResult` type switches missing `CancelledResult` case — falls to default/empty content | Cancelled result misinterpreted as parse failure, enters retry, compounds R1 | High | UT-KA-823-C01, UT-KA-823-C05, UT-KA-823-C06 | Add explicit `case *CancelledResult` to all three switches |
| R3 | `StartInvestigation` goroutine discards partial result when `Store.Update` is rejected by terminal guard | Session has no stored result; snapshot returns empty | High | IT-KA-823-C01, IT-KA-823-C02 | Add `storePartialResult` that stores result without changing status |
| R4 | `Investigate` proceeds to workflow selection after cancelled RCA | Wasted LLM calls, delayed cancellation, incorrect result | Medium | UT-KA-823-C07 | Add `if rcaResult.Cancelled { return rcaResult, nil }` |
| R5 | `correctionFn` re-enters `runLLMLoop` which returns `CancelledResult` but closure lacks case | Parse error instead of cancellation error propagated through `SelfCorrect` | Medium | UT-KA-823-C06 | Add `case *CancelledResult` returning `nil, context.Canceled` |
| R6 | `Chat` error handler returns generic error wrapping `context.Canceled` instead of `CancelledResult` | Callers see error instead of structured cancellation outcome | Medium | UT-KA-823-C02 | Add `errors.Is(err, context.Canceled)` guard returning `CancelledResult` |
| R7 | `executeTool` blocks during cancellation until tool completes | Cancellation latency up to tool execution duration | Low | N/A (documented limitation) | Between-turn checkpoint catches after tool returns; document in PR3 |
| R8 | Event sink context value round-trip failure | Events not delivered to subscribers in PR4 | Low | UT-KA-823-C08, UT-KA-823-C09 | Simple unit test for `WithEventSink`/`EventSinkFromContext` |

### 3.1 Risk-to-Test Traceability

| Risk | Mitigating Tests |
|------|-----------------|
| R1 (CRITICAL) | UT-KA-823-C03, UT-KA-823-C04 — verify retry loops abort immediately on cancelled context |
| R2 (CRITICAL) | UT-KA-823-C01, UT-KA-823-C05, UT-KA-823-C06 — verify `CancelledResult` handled in all three switches |
| R3 (HIGH) | IT-KA-823-C01, IT-KA-823-C02 — verify partial result stored on session after cancel |
| R4 (HIGH) | UT-KA-823-C07 — verify `Investigate` short-circuits after cancelled RCA |
| R5 (MEDIUM) | UT-KA-823-C06 — verify self-correction propagates cancellation |
| R6 (MEDIUM) | UT-KA-823-C02 — verify `Chat` error path returns `CancelledResult` for `context.Canceled` |
| R8 (LOW) | UT-KA-823-C08, UT-KA-823-C09 — event sink context round-trip |

---

## 4. Scope

### 4.1 Features to be Tested

- **CancelledResult type** (`investigator.go`): New sealed `LoopResult` variant carrying accumulated state — validates BR-SESSION-002 (snapshot preserves context)
- **Between-turn checkpoint** (`investigator.go`, `runLLMLoop`): `ctx.Err()` check at loop top — validates BR-SESSION-001 (prompt abort)
- **Chat error path** (`investigator.go`, `runLLMLoop`): `context.Canceled` produces `CancelledResult` instead of error — validates BR-SESSION-001
- **Retry fast-abort** (`investigator.go`, `retryRCASubmit`, `retryWorkflowSubmit`): `ctx.Err()` guard at retry top — validates BR-SESSION-001
- **Phase handlers** (`investigator.go`, `runRCA`, `runWorkflowSelection`): `case *CancelledResult` — validates BR-SESSION-002
- **Self-correction propagation** (`investigator.go`, `correctionFn`): `case *CancelledResult` — validates BR-SESSION-001
- **Investigate short-circuit** (`investigator.go`, `Investigate`): Cancel between phases — validates BR-SESSION-001
- **InvestigationResult.Cancelled** (`types/types.go`): New field — validates BR-SESSION-002
- **Event sink helpers** (`session/manager.go`): `WithEventSink`/`EventSinkFromContext` — validates BR-SESSION-007
- **Partial result storage** (`session/manager.go`): `storePartialResult` on cancelled session — validates BR-SESSION-002

### 4.2 Features Not to be Tested

- **SSE stream endpoint**: Deferred to PR4 (turn-level event emission)
- **Event emission from runLLMLoop**: Deferred to PR4 (event sink infrastructure wired but no events emitted)
- **Token-level streaming**: Deferred to PR5/PR6
- **Tool-level cancellation (RR-3)**: `executeTool` respects `ctx` via `registry.Execute`, but cancellation during long-running tool calls (kubectl exec, Prometheus queries) is between-turn only — the loop detects cancellation after the tool call returns. Intra-tool cancellation requires tool-level ctx propagation, tracked for a future PR. See `runLLMLoop` GoDoc.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `CancelledResult` returns `nil` error, not wrapped `context.Canceled` | Callers dispatch on the concrete type via type switch, consistent with sealed `LoopResult` pattern. Error path reserved for infrastructure failures. |
| Retry fast-abort returns `nil` (not a special value) | Matches existing retry contract: `nil` means "retry did not produce a result." Caller's `CancelledResult` case handles the upstream. |
| Between-turn checkpoint (not mid-turn) | Cancellation at turn boundaries is deterministic and testable. Mid-tool cancellation depends on tool implementation and is out of scope. |
| `storePartialResult` stores on cancelled session without status change | `CancelInvestigation` already set `StatusCancelled`. The goroutine only needs to attach the partial result. No status race. |
| Event sink on context (not on struct) | Decouples investigator from session infrastructure. Investigator doesn't import `session` package. Context-carried values are idiomatic Go. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of new cancellation paths in `investigator.go` (between-turn checkpoint, Chat error path, retry fast-abort, phase handler switches, `CancelledResult` type)
- **Integration**: >=80% of new manager paths in `manager.go` (partial result storage, event sink wiring)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Investigator-level cancellation logic with mock LLM client
- **Integration tests**: Full session manager + investigator cancel flow

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "Operator cancels investigation → investigation aborts within 1 turn → partial state preserved in snapshot"
- NOT "function `runLLMLoop` returns `CancelledResult`" (implementation detail)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage meets >=80% threshold on new paths
4. No regressions: full existing test suite passes (`go build ./... && make test`)
5. Feature-specific: cancelled investigation has `Cancelled: true` and preserves messages/turn/phase/tokens

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken: code does not compile
- PR2 changes not available on `development/v1.5`
- Cascading failures: more than 3 tests fail for the same root cause

**Resume testing when**:
- Build fixed and green on CI
- Blocking condition resolved

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `CancelledResult` type, `runLLMLoop` checkpoint + Chat error path, `retryRCASubmit` fast-abort, `retryWorkflowSubmit` fast-abort, `runRCA` CancelledResult case, `runWorkflowSelection` CancelledResult case, `correctionFn` CancelledResult case, `Investigate` short-circuit | ~80 |
| `internal/kubernautagent/types/types.go` | `InvestigationResult.Cancelled`, `InvestigationResult.CancelledPhase`, `InvestigationResult.CancelledAtTurn` | ~10 |
| `internal/kubernautagent/audit/emitter.go` | `EventTypeInvestigationCancelled`, `ActionInvestigationCancelled` | ~5 |
| `internal/kubernautagent/session/store.go` | `Store.SetResult` | ~10 |
| `internal/kubernautagent/session/manager.go` | `WithEventSink`, `EventSinkFromContext`, `eventSinkKey` | ~15 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/session/manager.go` | `StartInvestigation` goroutine cancelled-result handling, `storePartialResult` | ~25 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/pr3-cancelled-result` HEAD | Branched from `development/v1.5` |
| Dependency: PR1 | Merged to `development/v1.5` | Session cancellation infrastructure |
| Dependency: PR1.5 | Merged to `development/v1.5` | Audit trail |
| Dependency: PR2 | Merged to `development/v1.5` | OAS endpoints + `IsTerminal` export |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SESSION-001 | Operator cancel aborts investigation within 1 turn boundary | P0 | Unit | UT-KA-823-C01 | Pass |
| BR-SESSION-001 | Cancelled `Chat` call produces clean abort, not error | P0 | Unit | UT-KA-823-C02 | Pass |
| BR-SESSION-001 | RCA retry fast-aborts on cancelled context | P0 | Unit | UT-KA-823-C03 | Pass |
| BR-SESSION-001 | Workflow retry fast-aborts on cancelled context | P0 | Unit | UT-KA-823-C04 | Pass |
| BR-SESSION-002 | `CancelledResult` carries accumulated messages, turn, phase, tokens | P0 | Unit | UT-KA-823-C01 | Pass |
| BR-SESSION-002 | `runRCA` returns partial `InvestigationResult` with `Cancelled: true` | P0 | Unit | UT-KA-823-C05 | Pass |
| BR-SESSION-002 | `runWorkflowSelection` returns partial result on cancel | P0 | Unit | UT-KA-823-C06 | Pass |
| BR-SESSION-001 | `Investigate` short-circuits after cancelled RCA | P0 | Unit | UT-KA-823-C07 | Pass |
| BR-SESSION-007 | Event sink round-trip via context | P1 | Unit | UT-KA-823-C08 | Pass |
| BR-SESSION-007 | Missing event sink returns nil (no panic) | P1 | Unit | UT-KA-823-C09 | Pass |
| BR-SESSION-002 | Cancelled session stores partial result for snapshot | P0 | Integration | IT-KA-823-C01 | Pass |
| BR-SESSION-001 | Cancel during multi-turn investigation aborts and preserves state | P0 | Integration | IT-KA-823-C02 | Pass |
| BR-SESSION-002 | Non-cancelled investigation behaves identically to v1.4 | P0 | Integration | IT-KA-823-C03 | Pass |
| BR-SESSION-001 | Self-correction cancelled mid-correction propagates cleanly | P1 | Unit | UT-KA-823-C10 | Pass |
| BR-AUDIT-005 | Cancellation emits `aiagent.investigation.cancelled` with phase and turn | P0 | Unit | UT-KA-823-C11 | Pass |
| BR-SESSION-002 | `Store.SetResult` attaches partial result without status change | P1 | Unit | UT-KA-823-008 | Pass |
| BR-AUDIT-005 | `EventTypeInvestigationCancelled` registered in `AllEventTypes` | P1 | Unit | UT-KA-823-A04 | Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-823-C{SEQUENCE}` (C for CancelledResult)

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/investigator/investigator.go` (new cancellation paths), `internal/kubernautagent/session/manager.go` (event sink helpers)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-823-C01 | Operator cancels during multi-turn loop → investigation aborts at next turn boundary with accumulated messages, turn count, phase, and token usage preserved | Pass |
| UT-KA-823-C02 | Operator cancels while LLM call is in progress → `Chat` returns `context.Canceled` → clean abort with state preserved (not an error) | Pass |
| UT-KA-823-C03 | Operator cancels during RCA parse retry → retry aborts immediately (no further LLM calls), cancellation propagates to caller | Pass |
| UT-KA-823-C04 | Operator cancels during workflow parse retry → retry aborts immediately, cancellation propagates | Pass |
| UT-KA-823-C05 | Operator cancels during RCA phase → `runRCA` returns partial `InvestigationResult` with `Cancelled: true` and RCA summary from accumulated messages | Pass |
| UT-KA-823-C06 | Operator cancels during workflow selection → `runWorkflowSelection` returns partial result with `Cancelled: true` and RCA summary preserved | Pass |
| UT-KA-823-C07 | Operator cancels during RCA → `Investigate` does NOT proceed to workflow selection; returns partial result immediately | Pass |
| UT-KA-823-C08 | Event sink attached to context is retrievable by `EventSinkFromContext` (round-trip) | Pass |
| UT-KA-823-C09 | `EventSinkFromContext` on context without sink returns nil (no panic, no allocation) | Pass |
| UT-KA-823-C10 | Operator cancels during self-correction loop → cancellation propagates through `SelfCorrect` as error, workflow selection returns cancelled result | Pass |
| UT-KA-823-C11 | Cancellation emits `aiagent.investigation.cancelled` audit event with phase, turn, and correlationID (RR-4) | Pass |
| UT-KA-823-008 | `Store.SetResult` attaches result to cancelled session without changing status; no-op for non-existent session (RR-2) | Pass |
| UT-KA-823-A04 | `EventTypeInvestigationCancelled` registered in `AllEventTypes`, well-formed via `NewEvent`, non-empty action constant | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/session/manager.go` (partial result storage, full cancel flow)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-823-C01 | Operator cancels session → session status is `cancelled` AND partial `InvestigationResult` with `Cancelled: true` is stored → snapshot endpoint returns meaningful data | Pass |
| IT-KA-823-C02 | Operator cancels during multi-turn investigation → investigation goroutine finishes, partial result stored, event channel closed | Pass |
| IT-KA-823-C03 | Non-cancelled investigation produces identical result to v1.4 behavior (regression guard) | Pass |

### Tier Skip Rationale

- **E2E**: Deferred — requires full HTTP stack with real SSE endpoint (PR4). Integration tests with session manager provide equivalent coverage for PR3 scope.

---

## 9. Test Cases

### UT-KA-823-C01: Between-turn cancellation checkpoint

**BR**: BR-SESSION-001, BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- Mock LLM client configured to respond with tool calls on turns 0-2
- Context cancelled after turn 1 completes

**Test Steps**:
1. **Given**: Investigator with mock client, max turns = 10, context with cancel function
2. **When**: `runLLMLoop` is called; mock client responds with tool calls; context cancelled after turn 1
3. **Then**: `runLLMLoop` returns `*CancelledResult` (not error) with `Turn == 2`, `Phase == "rca"`, accumulated messages from turns 0-1, and non-zero token count

**Expected Results**:
1. Return type is `*CancelledResult`
2. `CancelledResult.Turn` equals the turn where cancellation was detected
3. `CancelledResult.Messages` contains messages from completed turns
4. `CancelledResult.Tokens` reflects token usage from completed turns
5. No further `client.Chat` calls after cancellation

**Acceptance Criteria**:
- **Behavior**: Investigation aborts at the next turn boundary
- **Correctness**: Accumulated state matches completed turns exactly
- **Accuracy**: Token count equals sum of per-turn usage

---

### UT-KA-823-C02: Chat error path — context.Canceled

**BR**: BR-SESSION-001, BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- Mock LLM client returns `context.Canceled` (wrapped) on next `Chat` call

**Test Steps**:
1. **Given**: Investigator with mock client that returns `fmt.Errorf("langchaingo chat: %w", context.Canceled)`
2. **When**: `runLLMLoop` is called
3. **Then**: Returns `*CancelledResult` with messages accumulated before the failed call, not `(nil, error)`

**Expected Results**:
1. Return type is `*CancelledResult` (nil error)
2. Messages contain all messages up to the failed turn
3. Turn number reflects the turn that was interrupted

---

### UT-KA-823-C03: RCA retry fast-abort

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- `retryRCASubmit` called with already-cancelled context

**Test Steps**:
1. **Given**: Cancelled context, mock client configured (but should NOT be called)
2. **When**: `retryRCASubmit` is invoked
3. **Then**: Returns `nil` immediately; mock client `Chat` was never called

**Expected Results**:
1. Returns `nil`
2. Mock client records 0 `Chat` invocations
3. No audit events emitted for the retry

---

### UT-KA-823-C04: Workflow retry fast-abort

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- `retryWorkflowSubmit` called with already-cancelled context

**Test Steps**:
1. **Given**: Cancelled context, mock client
2. **When**: `retryWorkflowSubmit` is invoked
3. **Then**: Returns `nil` immediately; 0 `Chat` calls

---

### UT-KA-823-C05: runRCA handles CancelledResult

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- Mock client triggers cancellation during RCA phase `runLLMLoop`

**Test Steps**:
1. **Given**: Investigator with mock client; context cancelled during RCA
2. **When**: `runRCA` is called
3. **Then**: Returns `*InvestigationResult` with `Cancelled == true` and non-empty `RCASummary` (from accumulated messages)

---

### UT-KA-823-C06: runWorkflowSelection handles CancelledResult

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- Mock client triggers cancellation during workflow selection phase

**Test Steps**:
1. **Given**: Investigator with mock client; context cancelled during workflow selection
2. **When**: `runWorkflowSelection` is called
3. **Then**: Returns `*InvestigationResult` with `Cancelled == true`, preserving `rcaSummary`

---

### UT-KA-823-C07: Investigate short-circuits after cancelled RCA

**BR**: BR-SESSION-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- Mock client triggers cancellation during RCA

**Test Steps**:
1. **Given**: Investigator; context cancelled during RCA
2. **When**: `Investigate` is called
3. **Then**: Returns partial `InvestigationResult` with `Cancelled == true`; `runWorkflowSelection` is never called (verified by mock client call count)

---

### UT-KA-823-C08: Event sink context round-trip

**BR**: BR-SESSION-007
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/session/event_sink_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: Background context and a buffered `chan InvestigationEvent`
2. **When**: `WithEventSink(ctx, ch)` creates derived context; `EventSinkFromContext(derived)` retrieves
3. **Then**: Retrieved channel is the same instance as the original

---

### UT-KA-823-C09: Missing event sink returns nil

**BR**: BR-SESSION-007
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/session/event_sink_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: Plain `context.Background()` (no event sink attached)
2. **When**: `EventSinkFromContext(ctx)` is called
3. **Then**: Returns `nil`; no panic

---

### UT-KA-823-C10: Self-correction cancellation propagation

**BR**: BR-SESSION-001
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/cancel_test.go`

**Preconditions**:
- Investigator with mock client and catalog fetcher
- Validation fails, triggering self-correction
- Context cancelled during self-correction's `runLLMLoop` call

**Test Steps**:
1. **Given**: Mock client responds with invalid workflow; validator rejects; correction starts
2. **When**: Context cancelled during correction `runLLMLoop`
3. **Then**: `SelfCorrect` returns error wrapping `context.Canceled`; `runWorkflowSelection` returns cancelled result

---

### IT-KA-823-C01: Cancelled session stores partial result

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_cancel_test.go`

**Preconditions**:
- Session manager with real store
- Investigation function that performs multi-turn work with mock LLM

**Test Steps**:
1. **Given**: `StartInvestigation` launched with investigation function
2. **When**: `CancelInvestigation` called while investigation is running
3. **Then**: Session status is `cancelled`; session result is `*InvestigationResult` with `Cancelled == true` and non-nil `PartialMessages`

---

### IT-KA-823-C02: Cancel during multi-turn with event channel

**BR**: BR-SESSION-001, BR-SESSION-002
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_cancel_test.go`

**Preconditions**:
- Session manager with investigation function and subscriber

**Test Steps**:
1. **Given**: `StartInvestigation` launched; `Subscribe` called to get event channel
2. **When**: `CancelInvestigation` called
3. **Then**: Event channel is eventually closed; session has partial result; investigation goroutine has exited

---

### IT-KA-823-C03: Non-cancelled investigation regression guard

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_cancel_test.go`

**Preconditions**:
- Session manager with investigation function that completes normally

**Test Steps**:
1. **Given**: `StartInvestigation` launched with normal completion
2. **When**: Investigation completes without cancellation
3. **Then**: Session status is `completed`; result is `*InvestigationResult` with `Cancelled == false`; token usage matches expected

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock LLM client (implements `llm.Client`), mock result parser, mock audit store, mock catalog fetcher/validator
- **Location**: `test/unit/kubernautagent/investigator/cancel_test.go`, `test/unit/kubernautagent/session/event_sink_test.go`
- **Pattern**: Follow existing mock patterns in `test/unit/kubernautagent/investigator/wiring_test.go` and `test/integration/kubernautagent/investigator/investigator_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for session infrastructure. Mock LLM client only (investigation function uses mock LLM).
- **Infrastructure**: In-memory session store (real `Store` + `Manager`)
- **Location**: `test/integration/kubernautagent/session/manager_cancel_test.go`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PR1 (session cancellation) | Code | Merged | UT/IT blocked | N/A |
| PR1.5 (audit trail) | Code | Merged | UT blocked (audit mocks) | N/A |
| PR2 (OAS endpoints, IsTerminal export) | Code | Merged | UT blocked (handler tests) | N/A |

### 11.2 Execution Order (TDD Phases)

1. **CHECKPOINT 0**: Verify baseline — `go build ./...`, `make test`, count existing tests
2. **TDD RED**: Write all failing tests (UT-KA-823-C01 through C10, IT-KA-823-C01 through C03)
3. **CHECKPOINT 1**: Verify all new tests fail (RED) and all existing tests still pass
4. **TDD GREEN**: Implement `CancelledResult`, between-turn checkpoint, Chat error path, retry fast-abort, phase handlers, event sink, partial result storage
5. **CHECKPOINT 2**: Verify all new tests pass (GREEN) and all existing tests still pass
6. **TDD REFACTOR**: GoDoc, code organization, error message clarity
7. **CHECKPOINT 3**: Verify coverage >=80%, full regression, race detection (`-race`), adversarial audit

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/823/TP-823-CANCELLED-RESULT.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/investigator/cancel_test.go` | Cancellation unit tests |
| Event sink tests | `test/unit/kubernautagent/session/event_sink_test.go` | Context-carried event sink |
| Integration test suite | `test/integration/kubernautagent/session/manager_cancel_test.go` | Full cancel flow tests |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (investigator cancel)
go test ./test/unit/kubernautagent/investigator/... -ginkgo.v -ginkgo.focus="UT-KA-823-C"

# Unit tests (event sink)
go test ./test/unit/kubernautagent/session/... -ginkgo.v -ginkgo.focus="UT-KA-823-C"

# Integration tests
go test ./test/integration/kubernautagent/session/... -ginkgo.v -ginkgo.focus="IT-KA-823-C"

# Full regression
go build ./... && make test

# Coverage (unit, investigator)
go test ./test/unit/kubernautagent/investigator/... -coverprofile=cover_inv.out -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/investigator
go tool cover -func=cover_inv.out

# Coverage (integration, session manager)
go test ./test/integration/kubernautagent/session/... -coverprofile=cover_sess.out -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/session
go tool cover -func=cover_sess.out

# Race detection
go test ./test/unit/kubernautagent/investigator/... -race
go test ./test/integration/kubernautagent/session/... -race
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/integration/kubernautagent/investigator/*_test.go` | Type switches on `LoopResult` (if any in test helpers) | Add `case *CancelledResult` if test helpers dispatch on LoopResult | New variant in sealed interface |
| `test/unit/kubernautagent/investigator/wiring_test.go` | May test `runLLMLoop` error handling | Verify existing tests still pass with new error path for `context.Canceled` | Chat error path now returns `CancelledResult` instead of error for `context.Canceled` |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan based on rigorous due diligence of F1-F10 findings |
