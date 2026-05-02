# Test Plan: Parallel Tool Execution Within Agentic Loop Turns

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-970-v1.1
**Feature**: Parallel tool execution in KA investigator agentic loop using errgroup
**Version**: 1.1
**Created**: 2026-05-02
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/970-parallel-tool-execution`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the Kubernaut Agent's agentic loop can execute multiple
LLM-requested tool calls concurrently within a single turn, reducing wall-time latency
while preserving correctness, safety, auditability, and message ordering guarantees.

The change introduces concurrency into a previously sequential code path. This plan
provides defense-in-depth assurance that the `AnomalyDetector` is thread-safe, tool
results maintain declaration order, audit events are deterministic, and budget enforcement
prevents overruns — even under adversarial concurrent access.

### 1.2 Objectives

1. **Thread Safety**: `AnomalyDetector` survives concurrent access from N goroutines without data races, panics, or lost counter updates
2. **Budget Integrity**: `MaxToolCallsPerTool` and `MaxTotalToolCalls` are never exceeded, even when multiple goroutines attempt admission simultaneously
3. **Message Ordering**: Tool result messages in the LLM conversation maintain LLM declaration order regardless of goroutine completion order
4. **Audit Determinism**: `tool_call_index` in audit events matches LLM declaration order; audit event counts are deterministic per DD-TESTING-001
5. **Latency Reduction**: Wall-time for N concurrent I/O-bound tool calls is measurably less than N × sequential execution time
6. **Backward Compatibility**: All existing anomaly, investigator, and parser tests pass without modification
7. **Race-Free Execution**: All new and existing tests pass under Go's `-race` detector

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-kubernautagent` |
| Integration test pass rate | 100% | `make test-integration-kubernautagent` |
| Unit-testable code coverage (anomaly.go) | >=80% | `go test -coverprofile` on `anomaly.go` |
| Integration-testable code coverage (investigator.go) | >=80% | `go test -coverprofile` on `investigator.go` tool execution path |
| Race detector clean | 0 warnings | `go test -race ./test/unit/kubernautagent/investigator/...` |
| Backward compatibility | 0 regressions | All pre-existing tests pass without modification |
| Wall-time reduction (IT-KA-970-001) | parallel < 0.6 × sequential | Ratio-based timing assertion (CI-agnostic) |
| E2E parallel tool execution | 100% pass | `make test-e2e-kubernautagent` with multi-tool scenario |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-PERFORMANCE-970**: Parallel tool execution to reduce investigation wall-time
- **DD-HAPI-019-003**: Security architecture — I7 anomaly detector thresholds
- **DD-WORKFLOW-016**: Action-type workflow indexing — pagination exemptions
- **DD-TESTING-001**: Audit event validation standards — deterministic counts
- **DD-HAPI-017**: Three-step workflow discovery — Holmes SDK parallelism note
- Issue #970: feat: parallel tool execution within agentic loop turns

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes and How to Avoid Them](https://100go.co/) — Concurrency chapters #55-#74
- [Test Plan: Issue #934](../934/TEST_PLAN.md) — Prior KA test plan for alignment gate

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Concurrent map writes in AnomalyDetector cause fatal panic | Process crash during investigation | High (without mutex) | UT-KA-970-001, UT-KA-970-002 | Add sync.Mutex; validate with -race |
| R2 | TOCTOU budget overrun: N goroutines pass CheckToolCall simultaneously | More tool calls executed than configured max | Medium | UT-KA-970-005, IT-KA-970-002 | Serialize admission before parallel dispatch |
| R3 | Tool result messages appended out of LLM declaration order | LLM receives garbled conversation history, produces wrong analysis | High (without indexed slots) | IT-KA-970-001, UT-KA-970-006 | Indexed result slots: `results[i]` |
| R4 | Audit events emitted in completion order instead of declaration order | Non-deterministic audit trail violates DD-TESTING-001 | Medium | IT-KA-970-001 | Emit audit events post-Wait in declaration order |
| R5 | Goroutine leak on context cancellation | Memory leak, investigation never completes | Low | IT-KA-970-003 | errgroup.WithContext ensures cancellation propagation |
| R6 | Deadlock in AnomalyDetector mutex (e.g., CheckToolCall calls checkSuspiciousArgs under lock) | Process hang | Low | UT-KA-970-003 | checkSuspiciousArgs reads only immutable fields; no nested lock acquisition |
| R7 | Cross-session AnomalyDetector race (pre-existing, compounded by #970) | Counter corruption across concurrent investigations | Medium | (separate issue) | Mutex improves current state; file separate issue for per-investigation isolation |
| R8 | Summarizer concurrent LLM calls exceed provider rate limits | Tool result truncated or error | Low | (operational) | Existing rate limiting in client-go + provider; best-effort summarization |

### 3.1 Risk-to-Test Traceability

| Risk | Test Coverage |
|------|---------------|
| R1 (concurrent map writes) | UT-KA-970-001 (concurrent CheckToolCall), UT-KA-970-002 (interleaved CheckToolCall + RecordFailure) |
| R2 (TOCTOU budget overrun) | UT-KA-970-005 (admission serialization), IT-KA-970-002 (budget enforcement under parallel execution) |
| R3 (message ordering) | IT-KA-970-001 (ordering assertion), UT-KA-970-006 (indexed slot correctness) |
| R4 (audit ordering) | IT-KA-970-001 (tool_call_index assertion) |
| R5 (goroutine leak) | IT-KA-970-003 (context cancellation propagation) |
| R6 (deadlock) | UT-KA-970-003 (concurrent CheckToolCall + Reset) — would deadlock/timeout if lock is held |
| R7 (cross-session) | No direct test (pre-existing; filed as separate issue) |
| R8 (rate limits) | No direct test (operational; monitored in production) |

---

## 4. Scope

### 4.1 Features to be Tested

- **AnomalyDetector thread safety** (`internal/kubernautagent/investigator/anomaly.go`): Mutex protection of `CheckToolCall`, `RecordFailure`, `TotalExceeded`, `Reset` under concurrent access
- **Parallel tool execution in runLLMLoop** (`internal/kubernautagent/investigator/investigator.go`): errgroup-based concurrent `executeTool` with serialized budget admission, indexed result slots, and ordered audit emission
- **Budget enforcement under concurrency**: `MaxToolCallsPerTool` and `MaxTotalToolCalls` correctness when admission is serialized
- **Mock LLM multi-tool-call support** (`test/services/mock-llm/`): Extend response builder, scenario config, and handler to emit multiple `ToolCalls` in a single assistant message
- **E2E parallel tool execution** (`test/e2e/kubernautagent/`): End-to-end validation with deployed KA, mock LLM emitting multi-tool responses, and Kind cluster

### 4.2 Features Not to be Tested

- **LangChain `Message.ToolCalls` mapping gap**: Pre-existing adapter bug unrelated to parallelism; filed as separate issue
- **Cross-session AnomalyDetector isolation**: Pre-existing singleton lifecycle concern; mutex improves but does not fully resolve; filed as separate issue
- **Provider rate limiting behavior**: Operational concern validated in production, not in automated tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `sync.Mutex` over `sync.RWMutex` for AnomalyDetector | Critical section is small (~5 map/counter ops); RWMutex adds complexity for negligible benefit. Per 100 Go Mistakes #57: parallel goroutines accessing shared mutable state need mutexes. |
| Serialize admission, parallelize I/O | Prevents TOCTOU budget overrun (R2). Budget check is O(1), so serialization overhead is negligible vs tool I/O latency. |
| Indexed result slots (`results[i]`) | Go idiom for preserving order in concurrent work. Each goroutine writes only to its own slot — no synchronization needed for the slice itself (distinct indices). |
| Audit events emitted post-Wait in declaration order | Matches DD-TESTING-001 determinism requirements. `tool_call_index` remains semantically identical to pre-#970 behavior. |
| `errgroup.WithContext` over raw `sync.WaitGroup` | Provides context cancellation propagation (R5) and cleaner error handling. Already a dependency (`golang.org/x/sync v0.20.0`). Per 100 Go Mistakes #62: every goroutine must have a plan to stop. |
| `executeTool` split into admission + execution | `executeTool` currently combines `CheckToolCall` + `Execute` + sanitize + summarize. Splitting allows admission under lock and execution in parallel goroutines. |
| Ratio-based timing assertion (not absolute threshold) | `parallel_time < sequential_time * 0.6` is CI-agnostic and self-calibrating. Absolute thresholds (e.g., `< 250ms`) are flaky under CI load. |
| `atomic.Int32` for goroutine lifecycle tracking | Deterministic goroutine leak detection: tool handler increments on entry, decrements on exit. `Eventually()` asserts counter returns to 0. Avoids noisy `runtime.NumGoroutine()`. |
| Mock LLM `BuildMultiToolCallResponse` (additive) | Backward-compatible extension. Existing `BuildToolCallResponse` unchanged. New `tool_calls:` YAML key alongside existing singular `tool_call:`. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `anomaly.go` (pure logic: counter management, budget checks, mutex protection)
- **Integration**: >=80% of `investigator.go` tool execution path (I/O: LLM loop, tool dispatch, audit emission)
- **E2E**: Critical journey validation — multi-tool-call scenario with deployed KA + mock LLM in Kind cluster

### 5.2 Three-Tier Coverage

Every business requirement is covered by 3 test tiers (UT + IT + E2E):
- **Unit tests**: Validate AnomalyDetector thread safety in isolation (fast, deterministic)
- **Integration tests**: Validate parallel tool execution end-to-end through `Investigate()` with mock LLM client
- **E2E tests**: Validate parallel tool execution with deployed KA + mock LLM + Kind cluster

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "The investigation completes faster" (IT-KA-970-001 wall-time assertion)
- "The tool budget is never exceeded" (UT-KA-970-005, IT-KA-970-002)
- "The audit trail is deterministic and correct" (IT-KA-970-001 audit assertion)
- "The system does not crash under concurrent load" (UT-KA-970-001 race detector)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. All tests pass with `-race` flag (0 data race warnings)
5. No regressions in existing anomaly, investigator, parser, or E2E test suites
6. Ratio-based timing assertion in IT-KA-970-001: `parallel_time < sequential_time * 0.6`
7. E2E-KA-970-001 passes with multi-tool-call mock LLM scenario

**FAIL** — any of the following:

1. Any P0 test fails
2. Race detector reports any data race
3. Per-tier coverage falls below 80%
4. Existing tests that were passing before the change now fail
5. Budget enforcement test (IT-KA-970-002) allows more tool calls than configured max

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Code does not compile (`go build ./...` fails)
- Race detector reports systematic data races (stop and investigate root cause)
- Deadlock detected in AnomalyDetector mutex (stop and redesign lock strategy)

**Resume testing when**:

- Build fixed and green
- Race condition root cause identified and fixed
- Lock strategy validated with dedicated deadlock test

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/anomaly.go` | `CheckToolCall`, `RecordFailure`, `TotalExceeded`, `Reset`, `checkSuspiciousArgs`, `isExempt` | ~140 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `runLLMLoop` (tool execution block, lines ~1014-1048), `executeTool` (lines ~1196-1253) | ~90 |
| `internal/kubernautagent/audit/emitter.go` | `StoreBestEffort` (concurrent emission path) | ~10 |

### 6.3 Mock LLM Extension (test infrastructure)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `test/services/mock-llm/response/openai.go` | `BuildMultiToolCallResponse` (new) | ~30 |
| `test/services/mock-llm/config/overrides.go` | `ScenarioOverride.ToolCalls` (new field) | ~10 |
| `test/services/mock-llm/scenarios/types.go` | `MockScenarioConfig.MultiToolCalls` (new field) | ~10 |
| `test/services/mock-llm/handlers/openai.go` | Multi-tool-call dispatch branch | ~15 |
| `test/infrastructure/shared_e2e.go` | `parallel_tool_test` scenario in ConfigMap YAML | ~10 |

### 6.4 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `main` HEAD | Branch `feat/970-parallel-tool-execution` from `origin/main` |
| Dependency: `golang.org/x/sync` | v0.20.0 | `errgroup` — already in go.mod |
| Dependency: Issue #937 | Merged | slog-to-logr migration — logger types are `logr.Logger` |
| Dependency: Issue #971 | Merged | Prompt batching directive — no code conflict |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Unit | UT-KA-970-001 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Unit | UT-KA-970-002 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Unit | UT-KA-970-003 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Unit | UT-KA-970-004 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Unit | UT-KA-970-005 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P1 | Unit | UT-KA-970-006 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Integration | IT-KA-970-001 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | Integration | IT-KA-970-002 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P1 | Integration | IT-KA-970-003 | Pending |
| BR-HAPI-433-004 | I7 anomaly detection budget enforcement | P0 | Unit | UT-KA-970-005 | Pending |
| BR-HAPI-433-004 | I7 anomaly detection budget enforcement | P0 | Integration | IT-KA-970-002 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P0 | E2E | E2E-KA-970-001 | Pending |
| BR-PERFORMANCE-970 | Parallel tool execution reduces wall-time | P1 | E2E | E2E-KA-970-002 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-970-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **KA**: Kubernaut Agent
- **970**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/investigator/anomaly.go` — >=80% coverage

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-KA-970-001` | AnomalyDetector survives concurrent CheckToolCall from 10 goroutines without panic or data race | P0 | Pending |
| `UT-KA-970-002` | AnomalyDetector produces consistent counters under interleaved CheckToolCall + RecordFailure | P0 | Pending |
| `UT-KA-970-003` | AnomalyDetector survives concurrent CheckToolCall + Reset without panic or deadlock | P0 | Pending |
| `UT-KA-970-004` | TotalExceeded returns consistent result under concurrent CheckToolCall | P0 | Pending |
| `UT-KA-970-005` | Budget is never exceeded: N concurrent admissions against budget=M (N>M) rejects exactly N-M calls | P0 | Pending |
| `UT-KA-970-006` | Existing sequential behavior is preserved (all pre-#970 anomaly tests remain green) | P1 | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/investigator/investigator.go` tool execution path — >=80% coverage

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `IT-KA-970-001` | 4 tool calls execute in parallel with wall-time < 2x single-tool duration; messages in declaration order; audit `tool_call_index` matches declaration order | P0 | Pending |
| `IT-KA-970-002` | Budget enforcement rejects Nth call even under parallel execution (MaxTotalToolCalls honored) | P0 | Pending |
| `IT-KA-970-003` | Context cancellation during parallel execution terminates all in-flight goroutines | P1 | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full KA stack deployed in Kind + mock LLM with multi-tool-call support

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `E2E-KA-970-001` | Deployed KA processes multi-tool-call LLM response and returns correct investigation result with audit trail | P0 | Pending |
| `E2E-KA-970-002` | Existing E2E scenarios (67 specs) remain green with mock LLM multi-tool-call extension deployed | P1 | Pending |

### Tier Skip Rationale

- No tiers skipped. All three tiers (UT, IT, E2E) are covered.

---

## 9. Test Cases

### UT-KA-970-001: Concurrent CheckToolCall — No Panic, No Data Race

**BR**: BR-PERFORMANCE-970
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Preconditions**:
- AnomalyDetector with `MaxToolCallsPerTool=100`, `MaxTotalToolCalls=1000`

**Test Steps**:
1. **Given**: An AnomalyDetector configured with generous budgets
2. **When**: 10 goroutines each call `CheckToolCall` 50 times concurrently with distinct tool names
3. **Then**: No panic occurs, no data race detected (must pass with `-race`), `TotalExceeded` returns a consistent value

**Expected Results**:
1. Test completes without panic
2. `-race` detector reports 0 warnings
3. Total call count equals 500 (10 goroutines x 50 calls)

**Acceptance Criteria**:
- **Behavior**: Concurrent access does not crash the process
- **Correctness**: All 500 calls are admitted (budget is 1000)
- **Safety**: No goroutine observes corrupted map state

---

### UT-KA-970-002: Interleaved CheckToolCall + RecordFailure — Counter Consistency

**BR**: BR-PERFORMANCE-970
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Preconditions**:
- AnomalyDetector with `MaxRepeatedFailures=100`

**Test Steps**:
1. **Given**: An AnomalyDetector with generous failure limits
2. **When**: 5 goroutines call `CheckToolCall` while 5 goroutines call `RecordFailure`, all concurrently
3. **Then**: No panic, no data race; failure tracker maintains internally consistent state

**Expected Results**:
1. Test completes without panic
2. `-race` detector reports 0 warnings

---

### UT-KA-970-003: Concurrent CheckToolCall + Reset — No Deadlock

**BR**: BR-PERFORMANCE-970
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Preconditions**:
- AnomalyDetector with default config

**Test Steps**:
1. **Given**: An AnomalyDetector with default config
2. **When**: 5 goroutines call `CheckToolCall` while 1 goroutine calls `Reset` periodically, all concurrently for 200ms
3. **Then**: No deadlock (test completes within 5s timeout), no panic, no data race

**Expected Results**:
1. Test completes within timeout (not hung/deadlocked)
2. `-race` detector reports 0 warnings

---

### UT-KA-970-004: TotalExceeded Consistency Under Concurrent Access

**BR**: BR-PERFORMANCE-970
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Preconditions**:
- AnomalyDetector with `MaxTotalToolCalls=50`

**Test Steps**:
1. **Given**: An AnomalyDetector with total budget of 50
2. **When**: 10 goroutines each call `CheckToolCall` 10 times (100 total attempts against budget=50)
3. **Then**: `TotalExceeded` returns true after exactly 50 calls are admitted; remaining calls rejected

**Expected Results**:
1. Exactly 50 calls return `Allowed=true`
2. Exactly 50 calls return `Allowed=false`
3. `TotalExceeded()` returns `true` after the 50th admitted call

---

### UT-KA-970-005: Serialized Admission Prevents Budget Overrun

**BR**: BR-HAPI-433-004
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Preconditions**:
- AnomalyDetector with `MaxTotalToolCalls=5`

**Test Steps**:
1. **Given**: An AnomalyDetector with tight budget (5 total calls)
2. **When**: 20 goroutines simultaneously attempt `CheckToolCall`
3. **Then**: Exactly 5 return `Allowed=true`, exactly 15 return `Allowed=false`

**Expected Results**:
1. No more than 5 admitted calls (budget integrity)
2. No panic or data race

**Acceptance Criteria**:
- **Budget Integrity**: The configured maximum is never exceeded, even under concurrent pressure

---

### UT-KA-970-006: Backward Compatibility — Existing Tests Remain Green

**BR**: BR-PERFORMANCE-970
**Priority**: P1
**Type**: Unit
**File**: All existing files in `test/unit/kubernautagent/investigator/anomaly_test.go` and `anomaly_exempt_test.go`

**Test Steps**:
1. **Given**: The mutex has been added to AnomalyDetector
2. **When**: All existing anomaly tests are executed
3. **Then**: All pass without modification (the mutex is transparent to single-goroutine usage)

**Expected Results**:
1. 0 failures in existing anomaly tests
2. 0 skips

---

### IT-KA-970-001: Parallel Execution — Ordering, Timing, and Audit

**BR**: BR-PERFORMANCE-970
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_parallel_test.go`

**Preconditions**:
- Mock LLM client returning 4 tool calls in a single response
- Each tool has 100ms artificial delay (simulating K8s API latency)
- Tool registry with 4 registered fake tools

**Test Steps**:
1. **Given**: An Investigator configured with a mock LLM that returns 4 tool calls, each with 100ms delay
2. **When**: `Investigate()` is called and wall-time for the tool execution batch is measured
3. **Then**:
   a. `parallel_time < sequential_baseline * 0.6` (ratio-based, CI-agnostic — proves parallelism)
   b. Tool result messages in the conversation are in LLM declaration order (tool_call[0] before tool_call[1] before ...)
   c. Audit events have `tool_call_index` matching declaration order (0, 1, 2, 3)

**Timing Approach**: The test measures `sequential_baseline` by summing individual tool delays
(4 x 100ms = 400ms). The parallel execution must complete in less than 60% of that baseline
(< 240ms). This ratio-based approach is self-calibrating and immune to CI clock jitter.

**Expected Results**:
1. Ratio assertion passes (proves parallelism occurred)
2. Messages in correct order (proves indexed slots work)
3. Audit events in correct order with correct indices

**Acceptance Criteria**:
- **Latency**: Ratio-based wall-time reduction (`parallel < 0.6 * sequential`)
- **Correctness**: LLM receives tool results in the order it requested them
- **Auditability**: Audit trail is deterministic and declaration-ordered

---

### IT-KA-970-002: Budget Enforcement Under Parallel Execution

**BR**: BR-HAPI-433-004
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_parallel_test.go`

**Preconditions**:
- AnomalyDetector with `MaxTotalToolCalls=3`
- Mock LLM returning 5 tool calls in a single response

**Test Steps**:
1. **Given**: An Investigator with budget of 3 total tool calls, LLM requests 5
2. **When**: `Investigate()` is called
3. **Then**: Exactly 3 tools are executed (results contain real output); 2 are rejected (results contain budget error JSON)

**Expected Results**:
1. 3 tool result messages contain real tool output
2. 2 tool result messages contain `"per-tool call limit exceeded"` or `"total tool call limit exceeded"` error
3. Investigation completes (does not crash)

---

### IT-KA-970-003: Context Cancellation Propagation

**BR**: BR-PERFORMANCE-970
**Priority**: P1
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_parallel_test.go`

**Preconditions**:
- Mock LLM returning 4 tool calls, each with 500ms delay
- Context with 50ms timeout
- `fakeTool` instrumented with `atomic.Int32` active-goroutine counter (increment on entry, decrement on exit)

**Test Steps**:
1. **Given**: An Investigator with tools that take 500ms each, instrumented with atomic counter
2. **When**: `Investigate()` is called with a context that times out after 50ms
3. **Then**:
   a. `Investigate()` returns error within `Eventually()` (not 2000ms)
   b. Error contains context cancellation or deadline exceeded
   c. `Eventually()` asserts atomic counter returns to 0 (all goroutines exited)

**Goroutine Lifecycle Tracking**: Instead of noisy `runtime.NumGoroutine()`, the `fakeTool`
handler uses `atomic.Int32` to track active executions. This is deterministic:
- `counter.Add(1)` on tool entry
- `defer counter.Add(-1)` on tool exit
- `Eventually(counter.Load).Should(Equal(int32(0)))` after Investigate returns

**Expected Results**:
1. `Investigate()` returns error promptly (context cancellation)
2. Active goroutine counter returns to 0 (no goroutine leak)
3. No `time.Sleep()` used — only `Eventually()` for async assertions

---

### E2E-KA-970-001: Deployed KA Processes Multi-Tool-Call Response

**BR**: BR-PERFORMANCE-970
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/kubernautagent/parallel_tools_e2e_test.go`

**Preconditions**:
- Kind cluster with KA deployed
- Mock LLM deployed with `scenarios.yaml` containing a scenario with `tool_calls:` (multiple entries)
- DataStorage deployed for audit persistence

**Test Steps**:
1. **Given**: Mock LLM configured with scenario `parallel_tool_test` that returns 3 tool calls
   (`kubectl_describe`, `kubectl_events`, `kubectl_logs`) in a single assistant response
2. **When**: `POST /api/v1/incident/analyze` is sent with a signal matching `parallel_tool_test`
3. **Then**:
   a. Investigation completes successfully (status 200, session completed)
   b. Investigation result contains expected RCA/workflow fields
   c. Audit events in DataStorage contain 3 `aiagent.llm.tool_call` events with `tool_call_index` 0, 1, 2

**Expected Results**:
1. Investigation succeeds end-to-end
2. All 3 tool calls are executed and results incorporated
3. Audit trail is correct and complete

**Acceptance Criteria**:
- **Behavior**: Multi-tool-call responses are processed correctly in production deployment
- **Auditability**: Audit events persisted to DataStorage with correct indices
- **Integration**: No regressions in KA <-> mock LLM <-> DataStorage chain

---

### E2E-KA-970-002: Existing E2E Scenarios Remain Green (Backward Compatibility)

**BR**: BR-PERFORMANCE-970
**Priority**: P1
**Type**: E2E
**File**: All existing files in `test/e2e/kubernautagent/`

**Test Steps**:
1. **Given**: Mock LLM deployed with the extended `BuildMultiToolCallResponse` capability
2. **When**: Full E2E suite is executed (`make test-e2e-kubernautagent`)
3. **Then**: All 67 existing E2E specs pass without modification

**Expected Results**:
1. 0 failures in existing E2E suite
2. 0 skips
3. Mock LLM extension is backward-compatible (existing scenarios use singular `tool_call:`)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Race Detector**: `-race` flag required for all UT-KA-970-* tests
- **Mocks**: None — AnomalyDetector is pure logic
- **Location**: `test/unit/kubernautagent/investigator/`
- **Resources**: Minimal CPU; concurrency tests use 10-20 goroutines

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockLLMClient` (in-process mock returning multi-tool-call responses), `fakeTool` (returns canned output with optional delay)
- **Infrastructure**: None — all in-process
- **Location**: `test/integration/kubernautagent/investigator/`
- **Resources**: Minimal; 100ms artificial delays for timing tests

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster, deployed KA, deployed mock LLM (extended), deployed DataStorage
- **Mocks**: Mock LLM only (per LLM mocking policy — all other services are REAL)
- **Location**: `test/e2e/kubernautagent/`
- **Resources**: Kind cluster (~2 CPU, ~4GB RAM), Docker daemon

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build, test, `-race` detector |
| Ginkgo CLI | v2.x | Test runner |
| `golang.org/x/sync` | v0.20.0 | `errgroup` for parallel execution |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #937 (slog-to-logr) | Code | Merged | Logger type mismatch | N/A — already resolved |
| Issue #971 (prompt batching) | Code | Merged | No conflict | N/A |
| `golang.org/x/sync` | Dependency | In go.mod | Cannot use errgroup | N/A — v0.20.0 available |

### 11.2 Execution Order (TDD Phases)

1. **Phase 1 (TDD RED — AnomalyDetector)**: UT-KA-970-001 through UT-KA-970-005
   - Write failing concurrent tests; expect panic/race
2. **Phase 2 (TDD GREEN — AnomalyDetector)**: Add `sync.Mutex` to pass Phase 1 tests
3. **CHECKPOINT 1**: Adversarial audit of mutex implementation
4. **Phase 3 (TDD RED — Parallel Execution)**: IT-KA-970-001 through IT-KA-970-003
   - Write failing integration tests; timing assertion fails (sequential)
5. **Phase 4 (TDD GREEN — Parallel Execution)**: Implement errgroup in `runLLMLoop`
6. **CHECKPOINT 2**: Adversarial audit of parallel execution implementation
7. **Phase 3b (TDD RED — E2E)**: E2E-KA-970-001
   - Extend mock LLM with `BuildMultiToolCallResponse`; write failing E2E test
8. **Phase 4b (TDD GREEN — E2E)**: Add multi-tool-call scenario to ConfigMap; pass E2E
9. **CHECKPOINT 2b**: E2E backward compatibility audit (67 existing specs green)
10. **Phase 5 (TDD REFACTOR)**: 100 Go Mistakes validation, lint, -race in Makefile
11. **CHECKPOINT 3**: Final adversarial audit
12. **Phase 6 (Validation)**: Full suite execution, confidence assessment

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/970/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/investigator/anomaly_test.go` | New `Describe` block for concurrency tests |
| Integration test suite | `test/integration/kubernautagent/investigator/investigator_parallel_test.go` | New file for parallel execution tests |
| E2E test suite | `test/e2e/kubernautagent/parallel_tools_e2e_test.go` | New file for E2E parallel tool validation |
| Mock LLM extension | `test/services/mock-llm/response/openai.go` + 3 files | Multi-tool-call response builder + config |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (with race detector)
go test -race ./test/unit/kubernautagent/investigator/... -v -count=1

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -v -count=1

# E2E tests
make test-e2e-kubernautagent

# Full KA unit suite
make test-unit-kubernautagent

# Specific test by ID
go test ./test/unit/kubernautagent/investigator/... -ginkgo.focus="UT-KA-970"
go test ./test/e2e/kubernautagent/... -ginkgo.focus="E2E-KA-970"

# Coverage
go test ./test/unit/kubernautagent/investigator/... -coverprofile=coverage-anomaly.out
go tool cover -func=coverage-anomaly.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| IT-KA-433W-012 (`investigator_anomaly_test.go`) | 11 tool calls processed sequentially; 11th rejected | No change needed — budget enforcement is preserved | Admission serialization produces identical rejection behavior |
| IT-KA-433W-013 (`investigator_anomaly_test.go`) | 3 identical failures trigger abort | No change needed — failure tracking is preserved | Mutex protects failureTracker |
| IT-KA-433W-014 (`investigator_anomaly_test.go`) | >30 tool calls return HumanReviewNeeded | No change needed — TotalExceeded check is post-batch | Same check runs after parallel batch completes |
| IT-KA-860-001 (`investigator_anomaly_test.go`) | 12 pagination calls allowed | No change needed — pagination exemption logic unchanged | isExempt and isPaginationCall are read-only |

---

## 15. 100 Go Mistakes Refactoring Checklist

The following mistakes from [100 Go Mistakes and How to Avoid Them](https://100go.co/) are
directly relevant to this change and MUST be validated during the TDD REFACTOR phase:

| # | Mistake | Relevance | Validation |
|---|---------|-----------|------------|
| 55 | Mixing up concurrency and parallelism | Tool execution is I/O-bound parallel work | Confirm errgroup is used for parallelism (not channels) |
| 56 | Thinking concurrency is always faster | Only parallelize I/O-bound tools | IT-KA-970-001 timing assertion proves benefit |
| 57 | When to use channels or mutexes | Parallel goroutines sharing AnomalyDetector state | Confirm mutex (not channels) protects shared mutable state |
| 58 | Data races vs race conditions | AnomalyDetector map + counter access | -race flag validates; UT-KA-970-001 exercises |
| 59 | Workload type (CPU vs I/O) | Tool calls are I/O-bound (K8s API) | No GOMAXPROCS limit on goroutine count |
| 60 | Misunderstanding Go contexts | errgroup.WithContext for cancellation | IT-KA-970-003 validates cancellation propagation |
| 61 | Propagating inappropriate context | errgroup context derived from parent | Confirm gctx is used in goroutines, not raw ctx |
| 62 | Starting goroutine without knowing when to stop | errgroup.Wait guarantees cleanup | Verify g.Wait() is always called |
| 63 | Goroutines and loop variables | `i, tc := i, tc` capture pattern | Code review during REFACTOR |
| 67 | Channel size | No channels used (mutex + errgroup) | N/A — confirmed no channels |
| 68 | Side effects with string formatting | No fmt under lock | Verify no fmt.Sprintf/Errorf called while holding mutex |
| 69 | Append under race | `results[i]` indexed writes (distinct indices, no append) | Verify no shared slice append |
| 71 | Misusing sync.WaitGroup | Using errgroup instead (higher-level) | Confirm errgroup, not raw WaitGroup |
| 72 | Forgetting about sync.Cond | Not applicable — no condition variable needed | N/A |
| 74 | Copying sync types | AnomalyDetector passed by pointer | Verify *AnomalyDetector (not value copy) |

---

## 16. Checkpoint Definitions

### CHECKPOINT 1: Post-Mutex Implementation (after Phase 2)

**Trigger**: All UT-KA-970-* tests GREEN with `-race`

**Adversarial Audit Scope**:
1. **Deadlock analysis**: Can any public method path acquire the mutex twice? (checkSuspiciousArgs reads immutable fields — safe)
2. **Lock scope**: Is the critical section minimal? No I/O or blocking calls under lock?
3. **Performance**: Does mutex contention under 10 concurrent goroutines cause measurable slowdown? (Benchmark if uncertain)
4. **Backward compatibility**: All existing anomaly tests still pass?
5. **Security**: Does the mutex prevent the R1 (fatal panic) and R2 (budget overrun) risks?

**Gate**: All 5 items pass → proceed to Phase 3. Any failure → stop and fix.

### CHECKPOINT 2: Post-Parallel Execution Implementation (after Phase 4)

**Trigger**: All IT-KA-970-* tests GREEN

**Adversarial Audit Scope**:
1. **Message ordering**: Are tool results guaranteed to be in declaration order? (Indexed slots verified)
2. **Audit trail**: Are audit events emitted post-Wait in declaration order? (DD-TESTING-001 compliance)
3. **Budget enforcement**: Does serialized admission prevent TOCTOU overrun? (UT-KA-970-005 exercises)
4. **Goroutine lifecycle**: Does errgroup.WithContext guarantee all goroutines terminate on context cancel?
5. **Sentinel handling**: Does sentinel pre-scan still short-circuit before parallel dispatch?
6. **Existing integration tests**: IT-KA-433W-012/013/014 and IT-KA-860-001 still pass?
7. **Security**: No new attack surface (no external input parsing in goroutines)?

**Gate**: All 7 items pass → proceed to Phase 3b (E2E). Any failure → stop and fix.

### CHECKPOINT 2b: E2E Backward Compatibility Audit (after Phase 4b)

**Trigger**: E2E-KA-970-001 GREEN

**Adversarial Audit Scope**:
1. **Backward compatibility**: All 67 existing E2E specs pass with mock LLM extension deployed
2. **Mock LLM**: `BuildMultiToolCallResponse` does not affect `BuildToolCallResponse` behavior
3. **ConfigMap**: Existing `tool_call:` (singular) YAML still works alongside new `tool_calls:` (plural)
4. **Audit persistence**: DataStorage contains correct `tool_call_index` values for multi-tool scenario
5. **No flakiness**: E2E-KA-970-001 passes 3 consecutive runs

**Gate**: All 5 items pass → proceed to Phase 5. Any failure → stop and fix.

### CHECKPOINT 3: Post-Refactor Final Audit (after Phase 5)

**Trigger**: REFACTOR complete, lint clean, -race in Makefile

**Adversarial Audit Scope**:
1. **100 Go Mistakes**: All items in Section 15 validated
2. **Lint**: `golangci-lint run --timeout=5m` clean (0 new warnings)
3. **Build**: `go build ./...` clean
4. **Full unit suite**: `make test-unit-kubernautagent` — 0 failures, 0 skips
5. **Full integration suite**: `make test-integration-kubernautagent` — 0 failures
6. **Full E2E suite**: `make test-e2e-kubernautagent` — 0 failures, 0 skips
7. **Coverage**: Per-tier >=80%
8. **CHANGELOG**: Updated with #970 entry
9. **Separate issues filed**: Cross-session detector isolation + LangChain adapter gap

**Gate**: All 8 items pass → proceed to commit and PR. Any failure → stop and fix.

---

## 17. Anti-Pattern Compliance

Per `TESTING_GUIDELINES.md`, the following anti-patterns are explicitly avoided:

| Anti-Pattern | Status | Evidence |
|--------------|--------|----------|
| `Skip()` / `XIt` / pending tests | FORBIDDEN | No Skip() in any test case |
| `time.Sleep()` for synchronization | FORBIDDEN | IT-KA-970-001 uses wall-time measurement, not Sleep-then-assert. IT-KA-970-003 uses context timeout. |
| Direct audit infrastructure testing | FORBIDDEN | IT-KA-970-001 validates audit as a side effect of `Investigate()`, not by calling `StoreAudit` directly |
| Mocking business logic | FORBIDDEN | Only external dependencies mocked (LLM client, tool registry) |
| HTTP endpoint testing in integration tier | FORBIDDEN | Integration tests call `Investigate()` directly, not HTTP handlers |

---

## 18. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-02 | Initial test plan |
| 1.1 | 2026-05-02 | Added E2E tier (E2E-KA-970-001/002), mock LLM multi-tool-call extension, ratio-based timing assertion, atomic counter goroutine tracking, CHECKPOINT 2b |
