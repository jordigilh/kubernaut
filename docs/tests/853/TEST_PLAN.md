# Test Plan: Inter-Service HTTP Client Retry & Circuit-Breaker

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-853-v1
**Feature**: HTTP `RetryTransport` with exponential backoff and idle connection timeout reduction
**Version**: 1.0
**Created**: 2026-04-26
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/852-853-resilience`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the `RetryTransport` — an `http.RoundTripper` middleware that automatically retries failed HTTP requests on transient errors (connection reset, EOF, HTTP 502/503/504) with exponential backoff and jitter. It also covers the reduction of `IdleConnTimeout` from 90s to 15s to prevent stale connection reuse after pod rescheduling. Together, these changes address inter-service HTTP resilience gaps that caused cascading failures during OCP pod rescheduling events.

### 1.2 Objectives

1. **Transparent retry**: `RetryTransport` retries on connection errors and HTTP 502/503/504 without caller changes
2. **No retry on client errors**: HTTP 4xx responses are never retried (idempotency contract)
3. **Context-aware**: Retries abort immediately on context cancellation or deadline exceeded
4. **Body replay safety**: POST/PUT request bodies replayed safely via `req.GetBody()`
5. **Backoff with jitter**: Exponential backoff uses `pkg/shared/backoff` with 20% jitter to prevent thundering herd
6. **Stale connection prevention**: `IdleConnTimeout` reduced to 15s in both TLS and non-TLS transport paths
7. **Selective wiring**: Retry transport wired into exactly 4 client constructors; audit client explicitly excluded (double-retry prevention)
8. **>=80% unit-testable code coverage** on `RetryTransport` and `DefaultBaseTransportWithRetry`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/shared/transport/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/shared/transport/` |
| TLS unit test pass rate | 100% | `go test ./test/unit/shared/tls/...` |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| Race detector | 0 races | `go test -race` on all modified packages |
| Audit client NOT wired | Verified | Grep confirms no `DefaultBaseTransportWithRetry` in `pkg/audit/` |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-GATEWAY-190**: Distributed locking and connection resilience
- **Issue #853**: Inter-service HTTP clients lack retry/circuit-breaker
- **DD-AUDIT-003**: Audit store retry policy (application-level — must not double-retry)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://100go.co/) — Relevant: #46 (io.Reader input), #49 (error wrapping), #52 (handling error twice), #53 (not handling errors), #58 (race conditions), #78 (HTTP body leaks), #80 (not closing HTTP response body), #83 (not using -race flag)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Double retry in audit path | 9 total attempts (3 transport x 3 app-level) on DS failures | High if misconfigured | UT-RT-853-012 | Audit client explicitly excluded; verified by grep checkpoint |
| R2 | Body replay fails on non-replayable body | Silent data corruption or panic | Medium | UT-RT-853-008 | Skip retry if `req.GetBody == nil && req.Body != nil` |
| R3 | Response body leak on 5xx retry | File descriptor exhaustion | High if missed | UT-RT-853-010 | Drain and close `resp.Body` before retry |
| R4 | Jitter omission causes thundering herd | All pods retry simultaneously after DS restart | High if missed | UT-RT-853-009 | `backoff.Config{JitterPercent: 20}` — tested explicitly |
| R5 | `IdleConnTimeout` not reduced on TLS path | Stale connections persist when TLS_CA_FILE is set | Medium | UT-RT-853-014 | Both `DefaultBaseTransport` and `buildCATransport` updated; tested |
| R6 | Retry on non-idempotent mutations | Duplicate side effects (double POST) | Low | UT-RT-853-007 | All 4 callers verified: 3 GET-only, 1 POST via ogen (idempotent analyze) |
| R7 | Context deadline consumed by retries | Caller timeout hit during retry sleep | Medium | UT-RT-853-005 | Check `ctx.Err()` before each retry sleep; use `time.After` with `ctx.Done()` select |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-RT-853-012 (audit exclusion verification)
- **R2**: UT-RT-853-008 (GetBody nil guard)
- **R3**: UT-RT-853-010 (response body drain)
- **R4**: UT-RT-853-009 (jitter variance)
- **R5**: UT-RT-853-014 (IdleConnTimeout on TLS path)
- **R6**: UT-RT-853-007 (body replay via GetBody)
- **R7**: UT-RT-853-005 (context cancellation aborts retry)

---

## 4. Scope

### 4.1 Features to be Tested

- **`RetryTransport`** (`pkg/shared/transport/retry.go`): `http.RoundTripper` wrapper with retry logic
- **`RetryConfig`** (`pkg/shared/transport/retry.go`): Configuration struct with `MaxAttempts`, `Backoff`, `Logger`
- **`DefaultRetryConfig`** (`pkg/shared/transport/retry.go`): Sensible defaults function
- **`DefaultBaseTransportWithRetry`** (`pkg/shared/tls/tls.go`): Convenience wrapper combining `DefaultBaseTransport` + `RetryTransport`
- **`IdleConnTimeout` reduction** (`pkg/shared/tls/tls.go` + `pkg/shared/tls/ca_reloader.go`): 90s to 15s in both paths
- **Client wiring** (4 files): `DefaultBaseTransport()` replaced with `DefaultBaseTransportWithRetry()`
- **Audit exclusion** (`pkg/audit/openapi_client_adapter.go`): Verified NOT wired

### 4.2 Features Not to be Tested

- **Circuit breaker pattern**: Deferred to v1.5 — retry transport is the first resilience layer
- **`pkg/shared/backoff` internals**: Already has 100% coverage in `test/unit/shared/backoff/`
- **ogen-generated client internals**: Third-party code; `GetBody` behavior verified in analysis phase
- **Prometheus metric emission** (`kubernaut_http_retry_total`): Deferred to REFACTOR or follow-up

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `http.RoundTripper` interface | Transparent to all callers; Go idiomatic middleware pattern. Go mistake #6: interface on consumer side |
| `pkg/shared/transport/` package | New package — `tls` package name implies TLS-only; transport is generic. Go mistake #13: avoid `util` names |
| 3 attempts (1 initial + 2 retries) | Balances latency vs resilience; 100ms+200ms+400ms = 700ms worst case |
| 20% jitter | Recommended by `pkg/shared/backoff` for production anti-thundering herd |
| Retry only 502/503/504 (not 500/501) | 500 is generic server error (may not be transient); 501 is not implemented (permanent). 502/503/504 are proxy/overload errors (transient) |
| Exclude audit client | `BufferedAuditStore.writeBatchWithRetry` already retries with 1s/4s/9s backoff; transport retry would cause 3x3=9 attempts |
| `IdleConnTimeout: 15s` | Pod rescheduling takes ~5-10s; 15s ensures stale connections are closed before reuse. Previous 90s caused TCP RST on reused connections |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `pkg/shared/transport/retry.go` (pure logic: retry decision, backoff, body replay, response drain)
- **Unit**: >=80% of modified lines in `pkg/shared/tls/tls.go` and `ca_reloader.go`
- **Integration**: Existing IT suites for GW, RO, EM, KA cover end-to-end client behavior; no new IT required for transport layer
- **E2E**: Deferred to OCP validation step (optional, post-implementation)

### 5.2 Two-Tier Minimum

- **Unit tests**: Catch retry logic correctness (when to retry, backoff timing, body replay, response cleanup)
- **Integration tests**: Existing suites verify no regression in client behavior after wiring change

### 5.3 Business Outcome Quality Bar

Each test validates: "Does the inter-service call survive a transient failure transparently, without data loss, duplicate mutations, or excessive latency?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% on `pkg/shared/transport/`
4. No regressions in existing test suites across all services
5. `go test -race` passes on all modified packages
6. Audit client confirmed NOT wired with retry transport

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage below 80%
3. Audit client found using `DefaultBaseTransportWithRetry`
4. Race detector reports data race

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- `pkg/shared/backoff` has breaking changes on `main`
- Build broken in `pkg/shared/tls/` due to merge conflicts

**Resume testing when**:
- Dependencies stable and build green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/transport/retry.go` (new) | `RoundTrip`, `shouldRetry`, `NewRetryTransport`, `DefaultRetryConfig` | ~120 |
| `pkg/shared/tls/tls.go` | `DefaultBaseTransportWithRetry` (new), `DefaultBaseTransport` (modified) | ~20 |
| `pkg/shared/tls/ca_reloader.go` | `buildCATransport` (modified) | ~5 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/agentclient/client.go` | `NewKubernautAgentClient` (modified) | ~15 |
| `pkg/remediationorchestrator/routing/ds_history_adapter.go` | `NewDSHistoryAdapterFromConfig` (modified) | ~15 |
| `pkg/remediationorchestrator/routing/ds_workflow_adapter.go` | `NewDSWorkflowAdapterFromConfig` (modified) | ~15 |
| `pkg/effectivenessmonitor/client/ds_querier.go` | `NewOgenDataStorageQuerierWithTransport` (modified) | ~15 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/852-853-resilience` HEAD | Branch from `main` |
| `pkg/shared/backoff` | Existing (stable) | No changes required |
| Dependency: #852 | Same branch | No blocking dependency |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-190 | Retry on connection reset | P0 | Unit | UT-RT-853-002 | Pending |
| BR-GATEWAY-190 | Retry on HTTP 503 | P0 | Unit | UT-RT-853-003 | Pending |
| BR-GATEWAY-190 | No retry on success | P0 | Unit | UT-RT-853-001 | Pending |
| BR-GATEWAY-190 | No retry on client error (4xx) | P0 | Unit | UT-RT-853-004 | Pending |
| BR-GATEWAY-190 | Context cancellation aborts retry | P0 | Unit | UT-RT-853-005 | Pending |
| BR-GATEWAY-190 | Max attempts exhausted returns last error | P0 | Unit | UT-RT-853-006 | Pending |
| BR-GATEWAY-190 | Body replay via GetBody | P0 | Unit | UT-RT-853-007 | Pending |
| BR-GATEWAY-190 | Skip retry when GetBody nil | P0 | Unit | UT-RT-853-008 | Pending |
| BR-GATEWAY-190 | Jitter applied to backoff | P1 | Unit | UT-RT-853-009 | Pending |
| BR-GATEWAY-190 | Response body drained on 5xx | P0 | Unit | UT-RT-853-010 | Pending |
| BR-GATEWAY-190 | IdleConnTimeout 15s (non-TLS) | P0 | Unit | UT-RT-853-011 | Pending |
| DD-AUDIT-003 | Audit client NOT wired with retry | P0 | Unit | UT-RT-853-012 | Pending |
| BR-GATEWAY-190 | Retry on HTTP 502 and 504 | P1 | Unit | UT-RT-853-013 | Pending |
| BR-GATEWAY-190 | IdleConnTimeout 15s (TLS path) | P0 | Unit | UT-RT-853-014 | Pending |
| BR-GATEWAY-190 | No retry on HTTP 500 or 501 | P1 | Unit | UT-RT-853-015 | Pending |
| BR-GATEWAY-190 | DefaultBaseTransportWithRetry wraps correctly | P0 | Unit | UT-RT-853-016 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `RT` (RetryTransport — shared component)
- **ISSUE_NUMBER**: 853
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/shared/transport/retry.go`, `pkg/shared/tls/tls.go`, `pkg/shared/tls/ca_reloader.go` — >=80% coverage target

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-RT-853-001` | Successful request (200 OK) is not retried — single round-trip, no added latency | P0 | Pending |
| `UT-RT-853-002` | Connection reset (TCP RST / EOF) triggers retry; request succeeds on 2nd attempt | P0 | Pending |
| `UT-RT-853-003` | HTTP 503 (Service Unavailable) triggers retry; request succeeds on 2nd attempt | P0 | Pending |
| `UT-RT-853-004` | HTTP 400 (Bad Request) is NOT retried — client errors are permanent | P0 | Pending |
| `UT-RT-853-005` | Context cancellation during retry loop aborts immediately — no wasted resources | P0 | Pending |
| `UT-RT-853-006` | After exhausting max attempts, last error is returned to caller | P0 | Pending |
| `UT-RT-853-007` | POST request body is replayed correctly via `GetBody()` on retry | P0 | Pending |
| `UT-RT-853-008` | Request with body but nil `GetBody` is NOT retried — prevents data loss | P0 | Pending |
| `UT-RT-853-009` | Backoff durations include jitter (multiple runs produce varying delays) | P1 | Pending |
| `UT-RT-853-010` | Response body is drained and closed on retryable 5xx — prevents FD leak | P0 | Pending |
| `UT-RT-853-011` | `DefaultBaseTransport()` returns transport with `IdleConnTimeout` = 15s (non-TLS) | P0 | Pending |
| `UT-RT-853-012` | `pkg/audit/openapi_client_adapter.go` does NOT use `DefaultBaseTransportWithRetry` | P0 | Pending |
| `UT-RT-853-013` | HTTP 502 (Bad Gateway) and 504 (Gateway Timeout) both trigger retry | P1 | Pending |
| `UT-RT-853-014` | `buildCATransport()` (TLS path) returns transport with `IdleConnTimeout` = 15s | P0 | Pending |
| `UT-RT-853-015` | HTTP 500 and 501 are NOT retried — non-transient server errors | P1 | Pending |
| `UT-RT-853-016` | `DefaultBaseTransportWithRetry` wraps `DefaultBaseTransport` with `RetryTransport` | P0 | Pending |

### Tier Skip Rationale

- **Integration**: No new IT. The wiring change replaces `DefaultBaseTransport()` with `DefaultBaseTransportWithRetry()` — same interface, same behavior on success. Existing IT suites exercise the full client path.
- **E2E**: Deferred to optional OCP validation. Requires pod rescheduling to trigger real connection errors.

---

## 9. Test Cases

### UT-RT-853-001: No retry on 200 OK

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock `http.RoundTripper` that returns 200 OK on first call

**Test Steps**:
1. **Given**: `RetryTransport` wrapping a mock that returns `200 OK`
2. **When**: `RoundTrip(req)` called
3. **Then**: Mock called exactly once; response is 200 OK

**Acceptance Criteria**:
- **Behavior**: No unnecessary retries on success
- **Correctness**: Call count = 1
- **Accuracy**: Response passed through unmodified

### UT-RT-853-002: Retry on connection reset

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock `http.RoundTripper` that returns `syscall.ECONNRESET` on first call, then 200 OK

**Test Steps**:
1. **Given**: `RetryTransport` with max 3 attempts wrapping a mock: call 1 = ECONNRESET, call 2 = 200
2. **When**: `RoundTrip(req)` called
3. **Then**: Mock called exactly twice; final response is 200 OK

**Acceptance Criteria**:
- **Behavior**: Transient connection error triggers retry
- **Correctness**: Call count = 2; final response = 200

### UT-RT-853-003: Retry on HTTP 503

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock returns 503 on first call, 200 on second

**Test Steps**:
1. **Given**: Mock: call 1 = HTTP 503, call 2 = HTTP 200
2. **When**: `RoundTrip(req)` called
3. **Then**: Mock called exactly twice; final response is 200

**Acceptance Criteria**:
- **Behavior**: HTTP 503 is retryable
- **Correctness**: 503 response body drained and closed before retry

### UT-RT-853-004: No retry on HTTP 400

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock returns 400 on first call

**Test Steps**:
1. **Given**: Mock returns HTTP 400
2. **When**: `RoundTrip(req)` called
3. **Then**: Mock called once; response is 400

**Acceptance Criteria**:
- **Behavior**: Client errors are permanent, no retry
- **Correctness**: Response returned to caller unmodified

### UT-RT-853-005: Context cancellation aborts retry

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock returns connection error on every call
- Context cancelled before retry sleep completes

**Test Steps**:
1. **Given**: Context with cancel; mock always returns error
2. **When**: `RoundTrip(req)` called; cancel context during first retry sleep
3. **Then**: Returns context.Canceled error; mock called <= 2 times

**Acceptance Criteria**:
- **Behavior**: Retries abort immediately on context cancellation
- **Correctness**: No goroutine leak; error is `context.Canceled`

### UT-RT-853-006: Max attempts exhausted

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock returns connection error on all calls
- MaxAttempts = 3

**Test Steps**:
1. **Given**: Mock always returns `io.EOF`; MaxAttempts = 3
2. **When**: `RoundTrip(req)` called
3. **Then**: Mock called exactly 3 times; returns last `io.EOF` error

**Acceptance Criteria**:
- **Behavior**: Gives up after configured max attempts
- **Correctness**: Error returned is the final attempt's error, not a wrapped aggregate

### UT-RT-853-007: POST body replay via GetBody

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Request with `Body` = `bytes.NewReader(payload)` and `GetBody` set (as ogen does)
- Mock returns 503 on first call, 200 on second

**Test Steps**:
1. **Given**: POST request with JSON body; mock: call 1 = 503, call 2 = 200
2. **When**: `RoundTrip(req)` called
3. **Then**: On second call, mock receives request with same body content as first call

**Acceptance Criteria**:
- **Behavior**: Request body is replayed from `GetBody` on retry
- **Correctness**: Body content byte-identical on retry
- **Accuracy**: Original request body consumed on first attempt does not corrupt retry

### UT-RT-853-008: Skip retry when GetBody is nil

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Request with non-nil `Body` but nil `GetBody`
- Mock returns 503

**Test Steps**:
1. **Given**: Request with `Body = io.NopCloser(strings.NewReader("data"))`, `GetBody = nil`
2. **When**: Mock returns 503
3. **Then**: No retry attempted; 503 returned to caller

**Acceptance Criteria**:
- **Behavior**: Prevents data loss from non-replayable body
- **Correctness**: Call count = 1

### UT-RT-853-009: Jitter variance in backoff

**BR**: BR-GATEWAY-190
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- `RetryConfig` with `JitterPercent: 20` and `BasePeriod: 100ms`

**Test Steps**:
1. **Given**: Config with 20% jitter
2. **When**: Backoff calculated 20 times for same attempt count
3. **Then**: Not all durations are identical (statistical variance exists)

**Acceptance Criteria**:
- **Behavior**: Jitter prevents synchronized retries
- **Correctness**: At least 2 distinct durations in 20 samples

### UT-RT-853-010: Response body drained on 5xx retry

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Mock returns 503 with a body that tracks whether it was read and closed

**Test Steps**:
1. **Given**: Mock returns 503 with trackable body; then 200
2. **When**: `RoundTrip(req)` called
3. **Then**: First response body was fully read (drained) and `Close()` was called

**Acceptance Criteria**:
- **Behavior**: No file descriptor leak
- **Correctness**: `body.Read` reached EOF; `body.Close` called exactly once

### UT-RT-853-011: IdleConnTimeout 15s (non-TLS)

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Preconditions**:
- `TLS_CA_FILE` env var unset

**Test Steps**:
1. **Given**: `TLS_CA_FILE` not set
2. **When**: `DefaultBaseTransport()` called
3. **Then**: Returned `*http.Transport` has `IdleConnTimeout == 15 * time.Second`

**Acceptance Criteria**:
- **Behavior**: Stale connections closed within 15s
- **Correctness**: Type assertion to `*http.Transport` succeeds; field value matches

### UT-RT-853-012: Audit client NOT wired with retry

**BR**: DD-AUDIT-003
**Priority**: P0
**Type**: Unit (static analysis)
**File**: `test/unit/shared/transport/retry_test.go`

**Preconditions**:
- Source code of `pkg/audit/openapi_client_adapter.go` readable

**Test Steps**:
1. **Given**: Source file `pkg/audit/openapi_client_adapter.go`
2. **When**: Search for `DefaultBaseTransportWithRetry` in file content
3. **Then**: Zero occurrences found; file still uses `DefaultBaseTransport()`

**Acceptance Criteria**:
- **Behavior**: Audit client retries at application level only
- **Correctness**: No transport-level retry wiring

### UT-RT-853-013: Retry on HTTP 502 and 504

**BR**: BR-GATEWAY-190
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Test Steps**:
1. **Given**: Mock returns 502 then 200; separately mock returns 504 then 200
2. **When**: `RoundTrip(req)` called for each
3. **Then**: Both trigger retry; final response is 200

### UT-RT-853-014: IdleConnTimeout 15s (TLS path)

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/ca_reloader_test.go`

**Preconditions**:
- Valid CA certificate available for test

**Test Steps**:
1. **Given**: A CA pool from test certificate
2. **When**: `buildCATransport(pool)` called
3. **Then**: Returned `*http.Transport` has `IdleConnTimeout == 15 * time.Second`

**Acceptance Criteria**:
- **Correctness**: TLS path also uses reduced idle timeout

### UT-RT-853-015: No retry on HTTP 500 and 501

**BR**: BR-GATEWAY-190
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/transport/retry_test.go`

**Test Steps**:
1. **Given**: Mock returns 500; separately mock returns 501
2. **When**: `RoundTrip(req)` called
3. **Then**: No retry; response returned as-is

### UT-RT-853-016: DefaultBaseTransportWithRetry wraps correctly

**BR**: BR-GATEWAY-190
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Preconditions**:
- `TLS_CA_FILE` env var unset

**Test Steps**:
1. **Given**: `TLS_CA_FILE` not set
2. **When**: `DefaultBaseTransportWithRetry(logger)` called
3. **Then**: Returned `http.RoundTripper` is `*transport.RetryTransport` wrapping `*http.Transport`

---

## 10. Implementation Phases (TDD)

### Phase 1: TDD RED — RetryTransport Tests

**Goal**: Write all failing tests for the `RetryTransport` before any production code.

**Deliverables**:
- `test/unit/shared/transport/retry_test.go` with UT-RT-853-001 through UT-RT-853-010, UT-RT-853-013, UT-RT-853-015
- `test/unit/shared/transport/suite_test.go` (Ginkgo suite bootstrap)
- Tests reference `transport.RetryTransport`, `transport.RetryConfig`, `transport.NewRetryTransport` — all undefined

**Verification**: `go vet ./test/unit/shared/transport/...` fails (undefined symbols)

#### Checkpoint 1A: RED Phase Audit — RetryTransport

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Completeness** | Tests UT-RT-853-001 through 010, 013, 015 all have `It()` blocks | 12/12 present |
| **Anti-pattern: no `time.Sleep`** | No `time.Sleep()` for async wait (acceptable in backoff verification) | Only in jitter test (009), with `Eventually()` where needed |
| **Anti-pattern: no `Skip()`** | Zero `Skip()` calls | Confirmed |
| **Framework compliance** | All tests use Ginkgo/Gomega BDD | No `testing.T` |
| **Go mistake #46 (filename as input)** | Mock uses `http.RoundTripper` interface, not file paths | Confirmed |
| **Go mistake #5 (interface pollution)** | `RetryTransport` accepts `http.RoundTripper` (stdlib interface) | No custom interface |
| **Go mistake #53 (not handling errors)** | All mock setup errors handled | Confirmed |
| **Go mistake #80 (not closing body)** | Test verifies body close in UT-RT-853-010 | Present |
| **Security: no hardcoded secrets** | Test data uses `"test-payload"` not real tokens | Confirmed |
| **Security: mock transport isolation** | Each test uses fresh mock; no shared state between tests | Confirmed |
| **Adversarial: error injection** | Tests cover connection error, EOF, ECONNRESET, context cancel, 4xx, 5xx | At least 6 error paths |
| **Adversarial: boundary conditions** | MaxAttempts=1 (no retry), MaxAttempts=0 (default), nil body, empty body | Covered in tests |
| **Escalation gate** | Is `syscall.ECONNRESET` portable across darwin/linux? | Yes — Go's `net` package normalizes; use `errors.Is` |

---

### Phase 2: TDD RED — IdleConnTimeout & Wiring Tests

**Goal**: Write failing tests for `IdleConnTimeout` reduction and wiring verification.

**Deliverables**:
- UT-RT-853-011 in `test/unit/shared/tls/tls_test.go` (may be new `Describe` block in existing file)
- UT-RT-853-012 in `test/unit/shared/transport/retry_test.go` (audit exclusion grep)
- UT-RT-853-014 in `test/unit/shared/tls/ca_reloader_test.go`
- UT-RT-853-016 in `test/unit/shared/tls/tls_test.go`

**Verification**: Tests fail (IdleConnTimeout is still 90s; `DefaultBaseTransportWithRetry` undefined)

#### Checkpoint 1B: RED Phase Audit — IdleConnTimeout & Wiring

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Completeness** | UT-RT-853-011, 012, 014, 016 all present | 4/4 |
| **Existing test interference** | New `Describe` blocks don't break existing TLS tests | `go test ./test/unit/shared/tls/...` passes (new tests fail, old pass) |
| **Go mistake #22 (nil vs empty)** | `TLS_CA_FILE` unset tested, not just empty string | Both tested |
| **Security: env var cleanup** | Tests that set `TLS_CA_FILE` restore original value after | `t.Setenv` or `DeferCleanup` used |
| **Adversarial: singleton reset** | `ResetDefaultTransportForTesting()` called before each TLS test | Prevents cross-test pollution |
| **Escalation gate** | Does `buildCATransport` need to be exported for testing? | Check if `export_test.go` pattern is needed |

---

### Phase 3: TDD GREEN — RetryTransport Implementation

**Goal**: Implement minimal `RetryTransport` to make all RED tests pass.

**Deliverables**:
1. `pkg/shared/transport/retry.go`: `RetryTransport`, `RetryConfig`, `NewRetryTransport`, `DefaultRetryConfig`
2. Minimal `RoundTrip` implementation: retry loop, error classification, body replay, response drain

**Verification**: `go test ./test/unit/shared/transport/... -v` — all 12 RetryTransport tests pass

#### Checkpoint 2A: GREEN Phase Audit — RetryTransport

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Build** | `go build ./pkg/shared/transport/...` | Zero errors |
| **All RT tests pass** | `go test ./test/unit/shared/transport/... -v` | 12/12 pass |
| **Race detector** | `go test -race ./test/unit/shared/transport/...` | Zero races |
| **Go mistake #7 (returning interface)** | `NewRetryTransport` returns `*RetryTransport` (concrete), not `http.RoundTripper` | Confirmed |
| **Go mistake #8 (any says nothing)** | No `any` or `interface{}` in RetryTransport API | Confirmed |
| **Go mistake #49 (error wrapping)** | Errors wrapped with `%w` where caller may need `errors.Is` | Confirmed |
| **Go mistake #52 (handling error twice)** | `RoundTrip` returns error OR logs, not both (RoundTripper contract: no logging) | Confirmed — logging delegated to caller |
| **Go mistake #78 (HTTP body leak)** | `io.Copy(io.Discard, resp.Body)` + `resp.Body.Close()` on retryable 5xx | Code review confirms |
| **Go mistake #80 (not closing body)** | Every `resp.Body` from base transport is either returned to caller OR drained+closed | All paths reviewed |
| **Security: no panic** | No `panic()` in production code | Confirmed |
| **Security: request mutation** | `RoundTrip` does not mutate the original `*http.Request` (per `http.RoundTripper` contract) | Body replaced via `GetBody()`, new `io.ReadCloser` assigned |
| **Adversarial: nil response** | What if base transport returns `(nil, error)`? | Handled: only inspect `resp` when `err == nil` |
| **Adversarial: nil base transport** | What if `NewRetryTransport(nil, config)` called? | Panic or error on construction — documented |
| **Adversarial: zero MaxAttempts** | What if `MaxAttempts = 0`? | Default to 1 (single attempt, no retry) |
| **CHECKPOINT A** | All referenced types exist in `pkg/shared/transport/` | `read_file` confirms |
| **CHECKPOINT B** | Implementation uses `pkg/shared/backoff.Config.Calculate` | grep confirms |
| **Lint** | `golangci-lint run ./pkg/shared/transport/...` | Zero findings |
| **Escalation gate** | Should `RetryTransport` log retries? | Recommendation: log at V(1) via injected `Logger` for observability. Ask user if acceptable |

---

### Phase 4: TDD GREEN — IdleConnTimeout + Wiring

**Goal**: Reduce `IdleConnTimeout` to 15s and wire `DefaultBaseTransportWithRetry` into 4 client constructors.

**Deliverables**:
1. `pkg/shared/tls/tls.go`: Change `90 * time.Second` to `15 * time.Second` in `DefaultBaseTransport`
2. `pkg/shared/tls/tls.go`: Add `DefaultBaseTransportWithRetry(logger logr.Logger)` function
3. `pkg/shared/tls/ca_reloader.go`: Add `t.IdleConnTimeout = 15 * time.Second` in `buildCATransport`
4. Wire 4 constructors: replace `DefaultBaseTransport()` with `DefaultBaseTransportWithRetry(logger)`
5. Verify `pkg/audit/openapi_client_adapter.go` still uses `DefaultBaseTransport()`

**Verification**: All 16 tests pass; existing suites unaffected

#### Checkpoint 2B: GREEN Phase Audit — IdleConnTimeout + Wiring

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Build** | `go build ./...` (full codebase) | Zero errors |
| **All 16 tests pass** | `go test ./test/unit/shared/transport/... ./test/unit/shared/tls/...` | 16/16 pass |
| **Race detector** | `go test -race ./test/unit/shared/...` | Zero races |
| **Existing TLS tests** | `go test ./test/unit/shared/tls/...` (full suite) | Zero regressions |
| **Existing backoff tests** | `go test ./test/unit/shared/backoff/...` | Zero regressions |
| **Existing service tests** | `go test ./test/unit/kubernautagent/... ./test/unit/gateway/...` | Zero regressions |
| **CHECKPOINT C (business integration)** | All 4 constructors now use `DefaultBaseTransportWithRetry` | grep confirms 4 call sites |
| **Audit exclusion** | `grep -r "DefaultBaseTransportWithRetry" pkg/audit/` returns 0 results | Confirmed |
| **Go mistake #11 (functional options)** | `DefaultBaseTransportWithRetry` takes `logr.Logger` param (simple API, not over-engineered) | Confirmed |
| **Go mistake #15 (missing docs)** | `DefaultBaseTransportWithRetry` has godoc | Present |
| **Security: logger injection** | Logger passed explicitly, not global | Confirmed |
| **Security: no env var side effects** | No new env vars introduced | Confirmed |
| **Adversarial: constructor signature change** | 4 constructors that previously took no logger now need one — check all callers | All callers in `cmd/` already have logger available |
| **Adversarial: `DefaultBaseTransport` still works standalone** | Existing callers (including audit) not broken | `DefaultBaseTransport()` unchanged except IdleConnTimeout |
| **Lint** | `golangci-lint run ./pkg/shared/tls/... ./pkg/agentclient/... ./pkg/remediationorchestrator/... ./pkg/effectivenessmonitor/... ./pkg/audit/...` | Zero new findings |
| **Escalation gate** | Do any of the 4 constructors need logger threading from `cmd/` callers? | Check if logger is already available in constructor context |

---

### Phase 5: TDD REFACTOR

**Goal**: Improve code quality without changing behavior.

**Deliverables**:
1. Extract `isRetryableError(err error) bool` and `isRetryableStatusCode(code int) bool` as named functions if inline logic is complex
2. Add godoc to all exported symbols in `pkg/shared/transport/`
3. Add `// Go mistake #78, #80: drain and close response body` comment on the drain logic (non-obvious intent)
4. Consider extracting `retryableStatusCodes` as a package-level `map[int]bool` for readability
5. Verify consistent error messages across retry path

**Verification**: All 16 tests still pass; no behavior change

#### Checkpoint 3: REFACTOR Phase Audit

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **No behavior change** | All 16 tests pass | 16/16 |
| **Full regression** | `go build ./... && go test ./test/unit/...` | Zero failures |
| **Go mistake #2 (unnecessary nesting)** | `RoundTrip` happy path left-aligned | Confirmed |
| **Go mistake #4 (overusing getters)** | `RetryConfig` fields are exported directly (idiomatic Go) | No unnecessary getters |
| **Go mistake #13 (utility packages)** | Package named `transport`, not `util` or `shared` | Confirmed |
| **Go mistake #15 (missing docs)** | All exported symbols documented | `golint` clean |
| **Go mistake #39 (string concat)** | No string concatenation in loops (error messages) | Confirmed |
| **DRY** | No duplicated retry decision logic | Single `shouldRetry` function |
| **Code coverage** | `go test -coverprofile=cover.out ./test/unit/shared/transport/... && go tool cover -func=cover.out` | >=80% on `retry.go` |
| **Security: final review** | No exported fields that leak internal transport state | `RetryConfig` is config-only; `RetryTransport` exposes `RoundTrip` only |
| **Adversarial: future extensibility** | Can circuit-breaker be added later without API change? | Yes — another `RoundTripper` wrapper in same package |

---

## 11. Go Mistakes Guardrails

The following mistakes from [100 Go Mistakes](https://100go.co/) are specifically relevant to this implementation:

| # | Mistake | Relevance | Prevention |
|---|---------|-----------|------------|
| #2 | Unnecessary nesting | `RoundTrip` retry loop with multiple exit paths | Early returns, left-aligned happy path |
| #5 | Interface pollution | `RetryTransport` accepts stdlib `http.RoundTripper` | No custom interface created |
| #6 | Interface on producer side | Return concrete `*RetryTransport`, accept `http.RoundTripper` | Interface on consumer side |
| #7 | Returning interfaces | `NewRetryTransport` returns concrete type | Callers can wrap in `http.RoundTripper` |
| #8 | `any` says nothing | Config struct uses typed fields | No `any` usage |
| #11 | Functional options | Simple config struct, not over-engineered | `RetryConfig` struct + `DefaultRetryConfig()` |
| #13 | Utility packages | Package named `transport` | Meaningful name |
| #15 | Missing documentation | New package, new types | Godoc on all exports |
| #46 | Filename as function input | Mock transport via interface, not file I/O | `http.RoundTripper` mock |
| #49 | Error wrapping | Retry errors wrapped with context | `fmt.Errorf("retry attempt %d: %w", ...)` |
| #52 | Handling error twice | `RoundTrip` returns error, doesn't log it | Follows `http.RoundTripper` contract |
| #53 | Not handling errors | `io.Copy(io.Discard, resp.Body)` error | Explicitly ignore with `_ =` |
| #54 | Not handling defer errors | `resp.Body.Close()` in defer | Explicitly ignore: `_ = resp.Body.Close()` |
| #58 | Race conditions | Concurrent `RoundTrip` calls | No shared mutable state in `RetryTransport` |
| #74 | Copying sync types | `RetryTransport` has no sync fields | N/A — but verified |
| #78 | HTTP body leak | 5xx response body on retry | `io.Copy(io.Discard, body)` + `Close()` |
| #80 | Not closing HTTP body | Every response body path | Returned to caller OR drained+closed |
| #83 | Not using -race flag | Concurrent retry from multiple goroutines | `go test -race` in every checkpoint |

---

## 12. Environmental Needs

### 12.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Custom `http.RoundTripper` mock (call counter, configurable responses per call)
- **Mocks**: Trackable `io.ReadCloser` for body drain verification
- **Location**: `test/unit/shared/transport/retry_test.go`

### 12.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25.6 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| golangci-lint | latest | Lint checks |

---

## 13. Dependencies & Schedule

### 13.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| `pkg/shared/backoff` | Code | Stable | Cannot compute jittered backoff | N/A — already exists |
| `go-logr/logr` | Library | Stable | Cannot inject logger | N/A — already a dependency |

### 13.2 Execution Order

1. **Phase 1**: RED — RetryTransport tests (no dependencies)
2. **Phase 2**: RED — IdleConnTimeout & wiring tests (no dependencies)
3. **Phase 3**: GREEN — Implement RetryTransport (depends on Phase 1 tests)
4. **Phase 4**: GREEN — Wire IdleConnTimeout + constructors (depends on Phase 3)
5. **Phase 5**: REFACTOR — Code quality (depends on all GREEN phases)

---

## 14. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/853/TEST_PLAN.md` | Strategy and test design |
| RetryTransport unit tests | `test/unit/shared/transport/retry_test.go` | 12 Ginkgo BDD tests |
| TLS unit test additions | `test/unit/shared/tls/tls_test.go` | 2 new tests (011, 016) |
| CA reloader test addition | `test/unit/shared/tls/ca_reloader_test.go` | 1 new test (014) |
| Audit exclusion test | `test/unit/shared/transport/retry_test.go` | 1 grep-based test (012) |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 15. Execution

```bash
# RetryTransport unit tests
go test ./test/unit/shared/transport/... -ginkgo.v

# TLS unit tests (including new IdleConnTimeout tests)
go test ./test/unit/shared/tls/... -ginkgo.v

# All 853 tests by focus
go test ./test/unit/shared/... -ginkgo.focus="853" -ginkgo.v

# Race detector
go test -race ./test/unit/shared/transport/... ./test/unit/shared/tls/...

# Coverage
go test ./test/unit/shared/transport/... -coverprofile=transport_cover.out
go tool cover -func=transport_cover.out

# Full regression
go test ./test/unit/... -count=1
```

---

## 16. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/shared/tls/tls_test.go` | May assert `IdleConnTimeout == 90s` | Update to `15s` | Timeout reduced |
| No other changes expected | — | — | Wiring change is interface-compatible |

---

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-26 | Initial test plan |
