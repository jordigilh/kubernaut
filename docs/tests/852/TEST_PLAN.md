# Test Plan: Gateway `/readyz` Cache Sync Gate

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-852-v1
**Feature**: Gateway readiness probe gates on Kubernetes informer cache sync
**Version**: 1.0
**Created**: 2026-04-26
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/852-853-resilience`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the Gateway's `/readyz` endpoint returns HTTP 503 until the Kubernetes informer cache has fully synced. Without this gate, the readiness probe can pass before the cache is populated, causing the Gateway to serve requests that rely on stale or missing cached data. This is a defense-in-depth measure; the Helm chart's `initialDelaySeconds: 30` makes the race unlikely but not impossible under slow-start conditions or resource pressure.

### 1.2 Objectives

1. **Cache-unsynced rejection**: Gateway returns HTTP 503 with RFC 7807 body when `cacheReady` is false
2. **Cache-synced acceptance**: Gateway returns HTTP 200 when `cacheReady` is true (existing behavior preserved)
3. **Zero-value safety**: `cacheReady` defaults to false (Go `atomic.Bool` zero value), ensuring fail-closed behavior
4. **No regression**: All existing Gateway unit and integration tests pass without modification
5. **>=80% unit-testable code coverage** on the modified readiness path

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/gateway/...` |
| Integration test pass rate | 100% | `go test ./test/integration/gateway/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on readiness handler |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| Fail-closed on zero value | Verified | UT-GW-852-003 |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-GATEWAY-185**: Kubernetes cache integration for field-indexed queries
- **Issue #852**: Gateway `/readyz` probe does not gate on K8s cache sync
- **DD-GATEWAY-012**: Redis removal — Gateway is now K8s-native

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://100go.co/) — Relevant: #74 (copying sync types), #58 (race conditions with atomic), #77 (misusing sync.Cond vs atomic)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `cacheReady` not set in test constructors → existing tests fail with 503 | All GW integration tests break | High | All IT-GW-* | Set `cacheReady.Store(true)` in `NewServerForTesting` and `createServerWithClients` |
| R2 | Race between cache sync goroutine and readiness check | Flaky test or prod race | Low | UT-GW-852-001 | `atomic.Bool` is lock-free and race-safe; no mutex needed |
| R3 | RFC 7807 response body serialization differs from existing error responses | Inconsistent API contract | Medium | UT-GW-852-001 | Reuse existing `RFC7807Error` struct and pattern from shutdown handler |
| R4 | `atomic.Bool` copied in struct assignment (Go mistake #74) | Silent data race | Low | N/A | `Server` is always passed by pointer; `atomic.Bool` is safe as struct field when not copied |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by UT-GW-852-002 (verifies 200 after cacheReady=true) and IT regression suite
- **R2**: Mitigated by using `atomic.Bool` (race-detector clean); verified by `go test -race`
- **R3**: Mitigated by UT-GW-852-001 (asserts exact Content-Type and RFC 7807 structure)
- **R4**: Mitigated by code review checkpoint; `Server` never copied by value

---

## 4. Scope

### 4.1 Features to be Tested

- **Readiness handler** (`pkg/gateway/server.go:readinessHandler`): Returns 503 when `cacheReady` is false
- **Cache sync wiring** (`pkg/gateway/server.go:NewServerWithMetrics`): Sets `cacheReady.Store(true)` after `WaitForCacheSync`
- **Test constructors** (`pkg/gateway/server.go:NewServerForTesting`, `createServerWithClients`): Set `cacheReady.Store(true)` for backward compatibility

### 4.2 Features Not to be Tested

- **Cache sync timeout behavior**: Already tested by existing `WaitForCacheSync` timeout in `NewServerWithMetrics` (returns error on failure)
- **Helm chart probe configuration**: Operational concern, not code — `initialDelaySeconds: 30` validated during #852 triage on OCP
- **Full E2E readiness cycle**: Deferred — defense-in-depth measure with low probability; UT + IT coverage sufficient

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `atomic.Bool` over `sync.Mutex` | Lock-free, single-word atomic; readiness handler is hot path called every 5s by kubelet. Go mistake #77: prefer atomics for simple flags |
| Fail-closed (503 by default) | Go zero value of `atomic.Bool` is `false`; server starts rejecting probes until explicitly marked ready. Defense-in-depth principle |
| RFC 7807 error body | Consistent with existing shutdown and K8s-API-unreachable error responses in the same handler |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of readiness handler logic (pure logic: atomic check, JSON response)
- **Integration**: Existing IT suite covers readiness end-to-end; no new IT required (defense-in-depth)
- **E2E**: Deferred — `initialDelaySeconds: 30` makes race condition unreproducible in Kind

### 5.2 Two-Tier Minimum

- **Unit tests**: Catch logic and correctness errors (atomic flag, response body, status code)
- **Integration tests**: Existing tests verify readiness in full server context (backward compat)

### 5.3 Business Outcome Quality Bar

Each test validates: "Does the operator/kubelet receive the correct signal about Gateway readiness, preventing traffic routing to an unprepared instance?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage meets >=80% threshold on readiness handler
4. No regressions in existing Gateway test suites
5. `go test -race` passes on all Gateway tests

**FAIL** — any of the following:

1. Any P0 test fails
2. Existing readiness-related tests regress
3. Race detector reports data race on `cacheReady`

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Gateway `server.go` has unresolved merge conflicts from `main`
- Build broken — code does not compile

**Resume testing when**:
- Conflicts resolved and build green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/server.go` | `readinessHandler` (lines 1489-1556) | ~67 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/server.go` | `NewServerWithMetrics` (cache sync at lines 446-460) | ~15 |
| `pkg/gateway/server.go` | `NewServerForTesting`, `createServerWithClients` | ~120 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/852-853-resilience` HEAD | Branch from `main` |
| Dependency: Issue #853 | Same branch | Co-developed, no blocking dependency |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-185 | Cache sync gate on readiness | P0 | Unit | UT-GW-852-001 | Pending |
| BR-GATEWAY-185 | Readiness passes after cache sync | P0 | Unit | UT-GW-852-002 | Pending |
| BR-GATEWAY-185 | Fail-closed zero-value safety | P0 | Unit | UT-GW-852-003 | Pending |
| BR-GATEWAY-185 | Structured logging on cache-unsynced rejection | P1 | Unit | UT-GW-852-004 | Pending |
| BR-GATEWAY-185 | Shutdown takes priority over cache-unsynced | P1 | Unit | UT-GW-852-005 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `GW` (Gateway)
- **ISSUE_NUMBER**: 852
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/gateway/server.go:readinessHandler` — >=80% coverage target

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-GW-852-001` | Kubelet receives 503 + RFC 7807 when informer cache has not synced, preventing premature traffic routing | P0 | Pending |
| `UT-GW-852-002` | Kubelet receives 200 when cache is synced, allowing traffic routing (existing behavior preserved) | P0 | Pending |
| `UT-GW-852-003` | A freshly constructed Server (zero-value `cacheReady`) rejects readiness — fail-closed guarantee | P0 | Pending |
| `UT-GW-852-004` | Structured log entry emitted at V(1) when cache-unsynced rejection occurs | P1 | Pending |
| `UT-GW-852-005` | When both `isShuttingDown=true` and `cacheReady=false`, shutdown response takes priority (503 with shutdown message) | P1 | Pending |

### Tier Skip Rationale

- **Integration**: No new IT required. Existing IT suite exercises readiness handler through full server lifecycle. Test constructors will set `cacheReady=true` to preserve backward compatibility. Regression is caught by existing IT pass rate.
- **E2E**: Deferred. The race condition requires sub-30-second startup which is not reproducible in Kind with `initialDelaySeconds: 30`.

---

## 9. Test Cases

### UT-GW-852-001: Cache-unsynced readiness returns 503

**BR**: BR-GATEWAY-185
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/readiness_cache_gate_852_test.go`

**Preconditions**:
- Server constructed with `cacheReady` at zero value (false)
- `isShuttingDown` is false
- `apiReader` is set to a working mock client

**Test Steps**:
1. **Given**: A Gateway `Server` with `cacheReady` = false and `isShuttingDown` = false
2. **When**: HTTP GET request sent to `readinessHandler`
3. **Then**: Response status is 503, Content-Type is `application/problem+json`, body contains RFC 7807 error with detail mentioning "cache" or "not synced"

**Expected Results**:
1. HTTP status code 503 (Service Unavailable)
2. Response header `Content-Type: application/problem+json`
3. Body deserializes to RFC 7807 with non-empty `detail` field referencing cache sync

**Acceptance Criteria**:
- **Behavior**: Probe rejected before cache sync
- **Correctness**: Status code and content type match RFC 7807 contract
- **Accuracy**: Error detail distinguishable from shutdown or K8s-unreachable errors

### UT-GW-852-002: Cache-synced readiness returns 200

**BR**: BR-GATEWAY-185
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/readiness_cache_gate_852_test.go`

**Preconditions**:
- Server constructed with `cacheReady` set to true
- `isShuttingDown` is false
- `apiReader` returns success for namespace list

**Test Steps**:
1. **Given**: A Gateway `Server` with `cacheReady` = true
2. **When**: HTTP GET request sent to `readinessHandler`
3. **Then**: Response status is 200, body contains `{"status":"ready"}`

**Expected Results**:
1. HTTP status code 200
2. JSON body with `status: "ready"`

**Acceptance Criteria**:
- **Behavior**: Existing readiness behavior preserved
- **Correctness**: No regression from cache gate addition

### UT-GW-852-003: Zero-value cacheReady is fail-closed

**BR**: BR-GATEWAY-185
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/readiness_cache_gate_852_test.go`

**Preconditions**:
- Server struct initialized with Go zero values only (no explicit field setting)

**Test Steps**:
1. **Given**: A `Server{}` with only `apiReader` and `logger` set (all other fields at zero value)
2. **When**: `cacheReady.Load()` is called
3. **Then**: Returns `false`

**Expected Results**:
1. `atomic.Bool` zero value is `false` — no explicit initialization required for fail-closed

**Acceptance Criteria**:
- **Behavior**: Defense-in-depth: uninitialized server cannot pass readiness
- **Correctness**: Go language guarantee on zero values

### UT-GW-852-004: Structured log on cache-unsynced rejection

**BR**: BR-GATEWAY-185
**Priority**: P1
**Type**: Unit
**File**: `test/unit/gateway/readiness_cache_gate_852_test.go`

**Preconditions**:
- Server with `cacheReady` = false and a captured logger (e.g., `zap.NewDevelopment` or test sink)

**Test Steps**:
1. **Given**: Server with `cacheReady` = false and observable logger
2. **When**: `readinessHandler` called
3. **Then**: Log output contains "cache not synced" message at V(1) level

### UT-GW-852-005: Shutdown priority over cache-unsynced

**BR**: BR-GATEWAY-185
**Priority**: P1
**Type**: Unit
**File**: `test/unit/gateway/readiness_cache_gate_852_test.go`

**Preconditions**:
- Server with both `isShuttingDown` = true and `cacheReady` = false

**Test Steps**:
1. **Given**: Server shutting down with unsynced cache
2. **When**: `readinessHandler` called
3. **Then**: Response is 503 with shutdown message (not cache message)

**Acceptance Criteria**:
- **Behavior**: Shutdown signal takes precedence; kubelet sees graceful shutdown, not cache issue

---

## 10. Implementation Phases (TDD)

### Phase 1: TDD RED

**Goal**: Write all failing tests before any production code changes.

**Deliverables**:
- `test/unit/gateway/readiness_cache_gate_852_test.go` with UT-GW-852-001 through UT-GW-852-005
- All tests MUST fail (compile errors acceptable since `cacheReady` field does not yet exist)

**Verification**: `go vet ./test/unit/gateway/...` fails referencing `cacheReady`

#### Checkpoint 1: RED Phase Audit

Before proceeding to GREEN, perform the following checks:

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Completeness** | Every test in Section 9 has a corresponding `It()` block | 5/5 tests present |
| **Anti-pattern: no `time.Sleep`** | No `time.Sleep()` in test code | Zero occurrences |
| **Anti-pattern: no `Skip()`** | No `Skip()` calls | Zero occurrences |
| **Anti-pattern: no direct audit testing** | Tests exercise readiness behavior, not audit internals | Confirmed |
| **Framework compliance** | All tests use Ginkgo/Gomega BDD | No `testing.T` usage |
| **Go mistake #1 (variable shadowing)** | No shadowed variables in test helpers | `go vet -shadow` clean |
| **Go mistake #53 (not handling errors)** | All `err` returns checked in test setup | No ignored errors |
| **Security: no secrets in test data** | No hardcoded tokens, passwords, or certs | Manual review |
| **Adversarial: negative path coverage** | At least 60% of tests cover error/rejection paths | 3/5 = 60% (001, 003, 005) |
| **Escalation gate** | Any ambiguity in RFC 7807 body structure? | Escalate if existing pattern unclear |

---

### Phase 2: TDD GREEN

**Goal**: Minimal production code to make all RED tests pass.

**Deliverables**:
1. Add `cacheReady atomic.Bool` field to `Server` struct (~line 189)
2. Add `if !s.cacheReady.Load()` check in `readinessHandler` (after shutdown check)
3. Add `server.cacheReady.Store(true)` after `WaitForCacheSync` in `NewServerWithMetrics` (~line 460)
4. Add `server.cacheReady.Store(true)` in `NewServerForTesting` and `createServerWithClients`

**Verification**: `go test ./test/unit/gateway/... -run="852" -v` — all 5 tests pass

#### Checkpoint 2: GREEN Phase Audit

Before proceeding to REFACTOR, perform the following checks:

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Build** | `go build ./...` succeeds | Zero errors |
| **Race detector** | `go test -race ./test/unit/gateway/... -run="852"` | Zero races |
| **Existing tests** | `go test ./test/unit/gateway/...` (full suite) | Zero regressions |
| **Integration tests** | `go test ./test/integration/gateway/...` | Zero regressions (constructors set cacheReady=true) |
| **Go mistake #74 (copying sync type)** | `Server` never assigned by value (only pointer) | `go vet -copylocks` clean |
| **Go mistake #52 (handling error twice)** | Readiness handler either logs OR returns error, not both | Confirmed |
| **Go mistake #49 (error wrapping)** | Any new `fmt.Errorf` uses `%w` for wrappable errors | Confirmed |
| **Security: atomic correctness** | `cacheReady` accessed only via `.Load()` and `.Store()`, never direct field access | grep confirms |
| **Security: no information leak** | RFC 7807 error body does not expose internal paths or stack traces | Body review |
| **Adversarial: double-transition** | What happens if `cacheReady.Store(true)` is called twice? | Idempotent — atomic store is safe |
| **Adversarial: cache restart** | If cache goroutine dies and restarts, does `cacheReady` reset? | No — acceptable; cache failure already logs fatal. Documented |
| **CHECKPOINT A (type validation)** | `cacheReady` field verified in `Server` struct definition | `read_file` confirms |
| **CHECKPOINT C (business integration)** | `cacheReady` wired in `NewServerWithMetrics` (production path) | grep confirms in `cmd/gateway/` call chain |
| **Lint** | `golangci-lint run ./pkg/gateway/...` | Zero new findings |
| **Escalation gate** | Any test constructor missed? | List all `NewServer*` / `createServer*` callers |

---

### Phase 3: TDD REFACTOR

**Goal**: Improve code quality without changing behavior.

**Deliverables**:
1. Extract cache-unsynced RFC 7807 response into a helper if it duplicates shutdown pattern
2. Add godoc on `cacheReady` field explaining fail-closed semantics
3. Verify structured log message follows existing Gateway log patterns

**Verification**: All tests still pass; no behavior change

#### Checkpoint 3: REFACTOR Phase Audit

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **No behavior change** | `go test ./test/unit/gateway/... -run="852"` still passes | All 5 pass |
| **Full regression** | `go test ./test/unit/gateway/... && go test ./test/integration/gateway/...` | Zero failures |
| **Go mistake #15 (missing documentation)** | `cacheReady` field has godoc comment | Present |
| **Go mistake #2 (unnecessary nesting)** | Readiness handler happy path aligned left | Confirmed |
| **Go mistake #13 (utility packages)** | No new `util` or `helper` packages created | Confirmed |
| **DRY** | RFC 7807 error construction not duplicated | Shared helper or consistent inline |
| **Code coverage** | `go test -coverprofile=cover.out ./test/unit/gateway/... && go tool cover -func=cover.out` | >=80% on `readinessHandler` |
| **Security: final review** | No new exported fields or methods that leak internal state | Confirmed |

---

## 11. Go Mistakes Guardrails

The following mistakes from [100 Go Mistakes](https://100go.co/) are specifically relevant to this implementation:

| # | Mistake | Relevance | Prevention |
|---|---------|-----------|------------|
| #1 | Variable shadowing | Test helper variables | `go vet -shadow` in checkpoint |
| #2 | Unnecessary nesting | `readinessHandler` has 3 early-return paths | Keep happy path left-aligned |
| #15 | Missing code documentation | New `cacheReady` field | Godoc required in REFACTOR |
| #49 | Error wrapping | `fmt.Errorf` in cache sync failure | Use `%w` |
| #52 | Handling error twice | Handler logs AND returns to HTTP | Log OR return, not both |
| #53 | Not handling errors | `json.NewEncoder(w).Encode(...)` | Check or explicitly ignore with `_` |
| #58 | Race conditions | `atomic.Bool` concurrent access | Use atomic operations only |
| #74 | Copying sync types | `atomic.Bool` in struct | Server always passed by pointer |
| #77 | Misusing sync primitives | Flag vs mutex | `atomic.Bool` is correct for simple flag |

---

## 12. Environmental Needs

### 12.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `apiReader` (fake `client.Reader` returning success for namespace list)
- **Location**: `test/unit/gateway/readiness_cache_gate_852_test.go`

### 12.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25.6 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| golangci-lint | latest | Lint checks |

---

## 13. Execution

```bash
# Unit tests (852 only)
go test ./test/unit/gateway/... -ginkgo.focus="852" -v

# Full Gateway unit suite (regression)
go test ./test/unit/gateway/... -v

# Full Gateway integration suite (regression)
go test ./test/integration/gateway/... -v

# Race detector
go test -race ./test/unit/gateway/... -ginkgo.focus="852"

# Coverage
go test ./test/unit/gateway/... -coverprofile=cover.out
go tool cover -func=cover.out | grep readinessHandler
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| All IT-GW-* using `NewServerForTesting` | Readiness returns 200 | None — constructor sets `cacheReady=true` | Backward compat |
| All IT-GW-* using `createServerWithClients` | Readiness returns 200 | None — constructor sets `cacheReady=true` | Backward compat |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-26 | Initial test plan |
