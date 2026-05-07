# Test Plan: SA Token Refresh and Audit 401 Handling

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1055-1056-v1
**Feature**: Fix SA token cached forever (#1055) and audit batches silently dropped on 401 (#1056)
**Version**: 1.0
**Created**: 2026-05-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/1055-1056-sa-token-refresh-audit-401`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates two coupled bug fixes shipping on a single branch:

**Bug A (Issue #1055)**: `bearerTransport` in `cmd/kubernautagent/main.go` reads the
ServiceAccount token once at startup via `os.ReadFile` and stores it as a static string.
After ~1 hour when kubelet rotates the projected token, all DataStorage and audit calls
fail with 401 Unauthorized. The fix replaces `bearerTransport` with the existing
`AuthTransport` pattern (5-minute TTL cache + 401 cache invalidation).

**Bug B (Issue #1056)**: `pkg/audit/errors.go` treats ALL 4xx HTTP errors as non-retryable
("invalid data"), including 401/403 (authentication/authorization). Audit batches are
immediately dropped with a misleading "invalid data" log message. The fix reclassifies
401/403 as retryable auth errors and adds distinct logging.

### 1.2 Objectives

1. **Token refresh**: `AuthTransport` re-reads the SA token from disk when the 5-minute cache expires or when a 401 response invalidates the cache
2. **401 retry**: Audit store retries on 401/403 errors instead of silently dropping batches
3. **Observability**: Auth errors produce distinct log messages differentiating them from data errors (400/422)
4. **Pattern convergence**: KA uses the same `AuthTransport` as all other services (eliminates one-off `bearerTransport`)
5. **No regressions**: All existing tests pass; `bearerTransport` removal does not affect integration test types

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/shared/auth/... ./test/unit/audit/...` |
| Build success | 0 errors | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run --timeout=5m` |
| `bearerTransport` in production | 0 references | `grep -r 'bearerTransport' cmd/ pkg/ --include='*.go'` |
| Race detector | 0 races | `go test -race ./test/unit/shared/auth/...` |

---

## 2. References

### 2.1 Authority (governing documents)

- **Issue #1055**: kubernaut-agent: SA token cached in memory, expires after 1h causing silent 401 failures
- **Issue #1056**: kubernaut-agent: audit event batches silently dropped on non-retryable 401 errors
- **DD-AUTH-005**: DataStorage Client Authentication Pattern
- **DD-AUDIT-002**: Audit Shared Library Design
- **ADR-032**: No Audit Loss

### 2.2 Cross-References

- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Go Coding Standards](../../../.cursor/rules/02-go-coding-standards.mdc)
- [100 Go Mistakes](https://100go.co) — TDD REFACTOR validation reference

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | 401 invalidation under concurrent storm causes excessive file reads | Performance degradation | Low | UT-AT-1055-005 | Benign: multiple goroutines re-read ~1KB file; cache re-populates on next read. Validated under -race. |
| R2 | `NewOpenAPIClientAdapterWithTransport` double-wraps auth | Double Bearer header | Low | UT-AT-1055-001 | Preflight verified: adapter skips wrapping when transport is non-nil. |
| R3 | Integration test `bearerTransport` type breaks | Test compilation failure | None | N/A | Integration tests define their own independent `bearerTransport` type in a separate package. |
| R4 | `os` import becomes unused after removing `os.ReadFile` calls | Build failure | None | N/A | 23+ other `os.*` usages remain in main.go. |
| R5 | Existing 401/403 `IsRetryable=false` assertions in errors_test.go | Test flips needed | Certain | UT-AE-1056-001/002 | Intentional: flip assertions from `BeFalse` to `BeTrue` in RED phase. |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by UT-AT-1055-005 (10+ goroutine race test)
- **R2**: Preflight code review of `NewOpenAPIClientAdapterWithTransport` nil-check logic
- **R5**: Mitigated by UT-AE-1056-001/002 (explicit assertion flips)

---

## 4. Scope

### 4.1 Features to be Tested

- **AuthTransport custom path constructor** (`pkg/shared/auth/transport.go`): `NewServiceAccountTransportWithPath` reads from configurable path
- **401 cache invalidation** (`pkg/shared/auth/transport.go`): `RoundTrip` zeroes cache time on 401 response
- **Auth error classification** (`pkg/audit/errors.go`): `IsAuthError()` for 401/403; `IsRetryable()` returns true for auth errors
- **Audit store retry on auth errors** (`pkg/audit/store.go`): `writeBatchWithRetry` retries 401/403 instead of dropping
- **Auth error logging** (`pkg/audit/store.go`): Distinct log message for auth vs data errors
- **bearerTransport removal** (`cmd/kubernautagent/main.go`): Replaced by `AuthTransport` in `initDSClients` and `buildAuditStore`

### 4.2 Features Not to be Tested

- **Integration test `bearerTransport`**: Independent type in `test/integration/datastorage/`, unaffected
- **E2E token rotation**: Requires Kind cluster with projected SA volumes; deferred to post-merge
- **Prometheus metrics for auth retry**: Follow-up item (SRE-M1), not blocking

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: TESTING_GUIDELINES.md — Per-Tier Testable Code Coverage (>=80% per tier).

- **Unit**: >=80% of unit-testable code (transport constructors, cache logic, error classification, retry paths)
- **Integration**: Existing integration tests unaffected (different `bearerTransport` type)
- **E2E**: Deferred — requires Kind cluster with token rotation; validated post-merge

### 5.2 TDD Phases

| Phase | Description | Tests | Checkpoint |
|-------|-------------|-------|------------|
| RED | Write failing tests for all new behavior | UT-AT-1055-001..005, UT-AE-1056-001..009, UT-AS-1056-001..002 | CHECKPOINT 1 |
| GREEN | Minimal implementation to pass all tests | Production code changes | CHECKPOINT 2 |
| REFACTOR | 100 Go Mistakes audit, code cleanup, lint | N/A (quality pass) | CHECKPOINT 3 |

---

## 6. Test Design Specification

### 6.1 AuthTransport Tests (Tier 1 — Unit)

**Test file**: `test/unit/shared/auth/transport_test.go`

| Test ID | Description | Input | Expected | BR |
|---------|-------------|-------|----------|-----|
| UT-AT-1055-001 | Custom path constructor reads from specified path | Token file at temp path; `NewServiceAccountTransportWithPath(tempPath, base)` | `Authorization: Bearer <token>` header injected | #1055 |
| UT-AT-1055-002 | 401 response invalidates token cache | RoundTrip returns 401; next call checks cache | Next getServiceAccountToken re-reads from disk | #1055 |
| UT-AT-1055-003 | Token re-read after invalidation picks up new content | Write "token-v1" to file, get 401, write "token-v2", next request | Second request uses "token-v2" | #1055 |
| UT-AT-1055-004 | Non-401 responses do not invalidate cache | RoundTrip returns 200, 500 | Cache remains valid (no file re-read) | #1055 |
| UT-AT-1055-005 | Concurrent RoundTrip under 401 storm | 10+ goroutines, all getting 401, with -race | No data races, no panics | #1055 |

### 6.2 Audit Error Tests (Tier 1 — Unit)

**Test file**: `test/unit/audit/errors_test.go`

| Test ID | Description | Input | Expected | BR |
|---------|-------------|-------|----------|-----|
| UT-AE-1056-001 | 401 is retryable | `HTTPError{401}` | `IsRetryable() == true` | #1056 |
| UT-AE-1056-002 | 403 is retryable | `HTTPError{403}` | `IsRetryable() == true` | #1056 |
| UT-AE-1056-003 | 400 is NOT retryable | `HTTPError{400}` | `IsRetryable() == false` | #1056 |
| UT-AE-1056-004 | 422 is NOT retryable | `HTTPError{422}` | `IsRetryable() == false` | #1056 |
| UT-AE-1056-005 | IsAuthError(401) | `HTTPError{401}` | `IsAuthError() == true` | #1056 |
| UT-AE-1056-006 | IsAuthError(403) | `HTTPError{403}` | `IsAuthError() == true` | #1056 |
| UT-AE-1056-007 | IsAuthError(400) | `HTTPError{400}` | `IsAuthError() == false` | #1056 |
| UT-AE-1056-008 | IsAuthError(500) | `HTTPError{500}` | `IsAuthError() == false` | #1056 |
| UT-AE-1056-009 | Package-level IsAuthError with wrapped 401 | `fmt.Errorf("ctx: %w", HTTPError{401})` | `IsAuthError() == true` | #1056 |

### 6.3 Audit Store Retry Tests (Tier 1 — Unit)

**Test file**: `test/unit/audit/store_test.go`

| Test ID | Description | Input | Expected | BR |
|---------|-------------|-------|----------|-----|
| UT-AS-1056-001 | 401 error retries and succeeds on 2nd attempt | Mock returns HTTPError{401} once, then succeeds | `AttemptCount() >= 2`, `BatchCount() == 1` | #1056 |
| UT-AS-1056-002 | Auth error logging contains diagnostic context | Mock returns HTTPError{401}; capture error logs | Log contains "auth" or "token" context | #1056 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass Criteria

- All 16 test cases pass
- `go build ./...` succeeds with zero errors
- `go test -race ./test/unit/shared/auth/...` reports zero races
- `golangci-lint run --timeout=5m` introduces zero new errors
- `grep -r 'bearerTransport' cmd/ pkg/ --include='*.go'` returns zero hits

### 7.2 Fail Criteria

- Any test case fails
- Build errors in any package
- Race detector reports data races
- New lint errors introduced

### 7.3 Suspension Criteria

- If `AuthTransport` changes break other services' tests, suspend and investigate shared component impact

---

## 8. Checkpoint Audit Categories

At each checkpoint (1, 2, 3), audit all 9 categories before advancing:

1. **Observability wiring**: No new metrics defined. Existing `audit_events_dropped_total` covered by store_test.go.
2. **Adversarial inputs**: Empty path, path traversal, Unicode path for transport; boundary status codes (0, -1, 399, 600) for errors.
3. **Resource bounds**: `tokenCache` is a single string, not a growing structure. N/A.
4. **Concurrency**: UT-AT-1055-005 covers 10+ goroutine race test under -race.
5. **Nil/zero edge cases**: Nil base transport, zero tokenCacheTime, zero StatusCode.
6. **Error-path observability**: Auth error log includes status_code, attempt, batch_size.
7. **Cross-phase integration**: Phase 2D wires AuthTransport into initDSClients/buildAuditStore.
8. **Spec compliance**: http.RoundTripper contract (no request mutation, no body consumption). RFC 6750 Bearer.
9. **API surface hygiene**: `invalidateTokenCache` unexported. `IsAuthError` mirrors existing `Is4xxError` pattern.

---

## 9. Deliverables

| Deliverable | Location |
|-------------|----------|
| Test plan | `docs/tests/1055-1056/TEST_PLAN.md` |
| AuthTransport tests | `test/unit/shared/auth/transport_test.go` |
| Audit error tests | `test/unit/audit/errors_test.go` |
| Audit store tests | `test/unit/audit/store_test.go` |
| AuthTransport implementation | `pkg/shared/auth/transport.go` |
| Audit error implementation | `pkg/audit/errors.go` |
| Audit store logging | `pkg/audit/store.go` |
| bearerTransport removal | `cmd/kubernautagent/main.go` |
