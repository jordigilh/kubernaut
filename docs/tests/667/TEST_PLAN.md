# Test Plan: DataStorage Concurrency Safety

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-667-v2.0
**Feature**: Fix concurrency hazards in DataStorage — deadlock lock-ordering, missing retries, unbounded batches, context propagation
**Version**: 2.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Complete
**Branch**: `development/v1.3_part4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that all CRITICAL and HIGH concurrency hazards identified in
the DataStorage adversarial audit (Issue #667) are fixed without regressions. It provides
behavioral assurance that batch writes do not deadlock, retryable PostgreSQL errors are
handled transparently, DB queries propagate caller context, and the HTTP batch API
enforces a maximum payload size.

### 1.2 Objectives

1. **Lock ordering**: `CreateBatch` acquires advisory locks in deterministic (sorted) `correlation_id` order, eliminating the deadlock vector
2. **Retry coverage**: `txretry` retries on both `40001` (serialization_failure) and `40P01` (deadlock_detected)
3. **Context propagation**: `queryEffectivenessEvents` and `DBAdapter.Query` accept `context.Context` and cancel DB queries when the caller context is cancelled
4. **Batch size enforcement**: HTTP batch API rejects payloads exceeding `MaxBatchSize` with RFC7807 400 response
5. **Coverage**: >=80% of unit-testable and integration-testable code per tier on affected files
6. **Regression safety**: Zero pre-existing test failures introduced

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-datastorage` |
| Integration test pass rate | 100% | `make test-integration-datastorage` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-STORAGE-040]: Concurrent batch writes must not deadlock (lock ordering)
- [BR-STORAGE-041]: Retryable PostgreSQL errors (40001, 40P01) must be retried transparently
- [BR-STORAGE-042]: All DB queries must propagate caller context for cancellation and timeout
- [BR-STORAGE-043]: HTTP batch API must enforce a maximum batch size
- Issue #667: DataStorage concurrency safety — deadlock lock-ordering, missing retries, unbounded batches, context propagation

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Concurrency Audit Report](../../../.cursor/plans/ds_concurrency_audit_report_1be6c261.plan.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Non-deterministic map iteration causes deadlock in production | Data loss, 500 errors, cascading timeouts | High | UT-DS-040-001/002, IT-DS-040-001/002 | Sort correlation IDs before advisory lock acquisition loop |
| R2 | `40P01` deadlock errors not retried by `txretry` | Silent permanent failure for serializable transactions | Medium | UT-DS-041-001 to 004 | Extend retry predicate to match `40P01` |
| R3 | DB queries ignore context — cancelled requests hold pool connections | Pool exhaustion under load | Medium | IT-DS-042-001 to 003 | Propagate `context.Context` through `QueryContext` |
| R4 | Unbounded batch amplifies lock duration and deadlock window | Long transactions, increased deadlock probability | Medium | IT-DS-043-001 to 003, UT-DS-043-001/002 | Enforce `MaxBatchSize` with 400 rejection |
| R5 | Signature changes break downstream callers | Compilation failures | Low | Checkpoint 2 ripple audit | Verify all call sites updated; `go build ./...` gate |

### 3.1 Risk-to-Test Traceability

| Risk | Mitigating Tests |
|------|-----------------|
| R1 (C1: lock ordering) | UT-DS-040-001, UT-DS-040-002, IT-DS-040-001, IT-DS-040-002 |
| R2 (H1: txretry gap) | UT-DS-041-001, UT-DS-041-002, UT-DS-041-003, UT-DS-041-004 |
| R3 (H2/H3: context) | IT-DS-042-001, IT-DS-042-002, IT-DS-042-003 |
| R4 (H4: batch size) | UT-DS-043-001, UT-DS-043-002, IT-DS-043-001, IT-DS-043-002, IT-DS-043-003 |
| R5 (signature ripple) | Checkpoint 2 compile-time ripple audit (not a test scenario) |

---

## 4. Scope

### 4.1 Features to be Tested

- **Lock ordering** (`pkg/datastorage/repository/audit_events_repository.go`): `CreateBatch` acquires advisory locks in sorted `correlation_id` order
- **Retry predicate** (`pkg/datastorage/repository/txretry/retry.go`): `WithSerializableRetry` retries on both `40001` and `40P01`
- **Context propagation** (`pkg/datastorage/server/effectiveness_handler.go`): `queryEffectivenessEvents` uses `QueryContext` with caller context
- **Context propagation** (`pkg/datastorage/adapter/db_adapter.go`): `DBAdapter.Query` uses `QueryContext` with caller context
- **Batch size limit** (`pkg/datastorage/server/audit_events_batch_handler.go`, `pkg/datastorage/config/config.go`): `MaxBatchSize` config and HTTP 400 guard

### 4.2 Features Not to be Tested

- **M1 (statement_timeout)**: Deferred to REFACTOR phase; may be addressed as a non-behavioral config addition. If not addressed, tracked as follow-up.
- **M3 (MaxOpenConns default)**: Deferred to REFACTOR phase; config validation enhancement.
- **M4 (DLQ worker timeout)**: Deferred to REFACTOR phase; worker-specific context timeout.
- **M5 (startup DDL deadline)**: Deferred to REFACTOR phase; startup-specific context timeout.
- **L1 (single-event contention)**: By design — single advisory lock per transaction cannot deadlock itself.
- **L2 (PurgeSQL vs FK)**: `PurgeSQL` has no executor in this codebase; future risk when retention worker is wired.
- **E2E tier**: Deferred — deadlock reproduction requires specific timing that CI parallelism in integration tests already provides.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Extract `sortedCorrelationIDs` as a testable pure function | Enables unit testing of sort logic independently from DB; keeps `CreateBatch` changes minimal |
| Rename `isSerializationFailure` to `isRetryablePostgresError` | Accurately reflects expanded scope (40001 + 40P01) without changing API contract |
| Default `MaxBatchSize` to 500 | Balances throughput (500 events/request) with lock-duration safety; matches DLQ worker's `MaxBatchSize: 10` philosophy of bounded work |
| Use `sort.Strings` on correlation ID keys | Lexicographic order is deterministic and consistent across all Go runtime versions |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (`txretry/retry.go`, config `Validate`/`MaxBatchSize`, `sortedCorrelationIDs`)
- **Integration**: >=80% of **integration-testable** code (`CreateBatch`, `queryEffectivenessEvents`, `DBAdapter.Query`, batch handler guard)
- **E2E**: Deferred — deadlock reproduction timing; CI parallelism validates lock ordering at integration tier

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:

| BR | Unit | Integration | Rationale |
|----|------|-------------|-----------|
| BR-STORAGE-040 | UT-DS-040-001/002 | IT-DS-040-001/002 | Pure sort logic (UT) + DB lock ordering under concurrency (IT) |
| BR-STORAGE-041 | UT-DS-041-001 to 004 | IT-DS-041-001 | Retry predicate logic (UT) + real serializable tx stress (IT) |
| BR-STORAGE-042 | — | IT-DS-042-001 to 003 | Context propagation is I/O behavior; unit tier not applicable (signature change only). Tier skip documented below. |
| BR-STORAGE-043 | UT-DS-043-001/002 | IT-DS-043-001 to 003 | Config validation (UT) + HTTP handler enforcement (IT) |

**Tier Skip**: BR-STORAGE-042 has no unit tests because context propagation is purely an I/O concern — the behavior change is replacing `db.Query` with `db.QueryContext`, which can only be verified with a real database connection. The 3 integration tests provide full coverage.

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "Batch writes complete without deadlock under concurrent calls" (not "sort function is called")
- "Oversized payload is rejected with 400" (not "if-statement evaluates to true")
- "Cancelled request releases DB connection" (not "QueryContext is called")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing DataStorage test suites
5. `go build ./...` and `go vet ./...` clean

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail
4. `CreateBatch` can still deadlock under concurrent calls

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Build broken — code does not compile; tests cannot execute
- PostgreSQL test infrastructure unavailable (integration tests)
- Pre-existing test failures detected that are unrelated to this change (investigate before continuing)

**Resume testing when**:

- Build fixed and green
- Test infrastructure restored
- Pre-existing failures triaged and documented

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/txretry/retry.go` | `WithSerializableRetry`, `isRetryablePostgresError` (renamed), `backoff` | ~70 |
| `pkg/datastorage/config/config.go` | `Validate` (MaxBatchSize defaulting/validation) | ~30 new |
| `pkg/datastorage/repository/audit_events_repository.go` | `sortedCorrelationIDs` (extracted pure function) | ~10 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/audit_events_repository.go` | `CreateBatch` (sorted iteration) | ~239 |
| `pkg/datastorage/server/effectiveness_handler.go` | `queryEffectivenessEvents` (context + QueryContext) | ~34 |
| `pkg/datastorage/adapter/db_adapter.go` | `Query` (context + QueryContext) | ~169 |
| `pkg/datastorage/server/audit_events_batch_handler.go` | `handleCreateAuditEventsBatch` (size guard) | ~116 |
| `pkg/datastorage/server/server.go` | `NewServer` / `Server` struct (MaxBatchSize wiring) | ~10 |
| `pkg/datastorage/server/handler.go` | `DBInterface.Query` signature | ~5 |
| `pkg/datastorage/mocks/mock_db.go` | `MockDB.Query` signature alignment | ~5 |
| `cmd/datastorage/main.go` | Config-to-ServerDeps wiring | ~5 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3_part4` HEAD | Branch |
| Issue | #667 | DataStorage concurrency safety |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-STORAGE-040 | Concurrent batch writes must not deadlock | P0 | Unit | UT-DS-040-001 | Pass |
| BR-STORAGE-040 | Concurrent batch writes must not deadlock | P0 | Unit | UT-DS-040-002 | Pass |
| BR-STORAGE-040 | Concurrent batch writes must not deadlock | P0 | Integration | IT-DS-040-001 | Pass (CI) |
| BR-STORAGE-040 | Concurrent batch writes must not deadlock | P0 | Integration | IT-DS-040-002 | Pass (CI) |
| BR-STORAGE-041 | Retryable PG errors retried transparently | P0 | Unit | UT-DS-041-001 | Pass |
| BR-STORAGE-041 | Retryable PG errors retried transparently | P0 | Unit | UT-DS-041-002 | Pass |
| BR-STORAGE-041 | Retryable PG errors retried transparently | P0 | Unit | UT-DS-041-003 | Pass |
| BR-STORAGE-041 | Retryable PG errors retried transparently | P1 | Unit | UT-DS-041-004 | Pass |
| BR-STORAGE-042 | DB queries propagate caller context | P0 | Integration | IT-DS-042-001 | Pass (CI) |
| BR-STORAGE-042 | DB queries propagate caller context | P0 | Integration | IT-DS-042-002 | Pass (CI) |
| BR-STORAGE-042 | DB queries propagate caller context | P1 | Integration | IT-DS-042-003 | Pass (CI) |
| BR-STORAGE-043 | HTTP batch API enforces max batch size | P0 | Unit | UT-DS-043-001 | Pass |
| BR-STORAGE-043 | HTTP batch API enforces max batch size | P0 | Unit | UT-DS-043-002 | Pass |
| BR-STORAGE-043 | HTTP batch API enforces max batch size | P0 | Integration | IT-DS-043-001 | Pass (CI) |
| BR-STORAGE-043 | HTTP batch API enforces max batch size | P1 | Integration | IT-DS-043-002 | Pass (CI) |
| BR-STORAGE-043 | HTTP batch API enforces max batch size | P1 | Integration | IT-DS-043-003 | Pass (CI) |
| BR-STORAGE-041 | Retryable PG errors retried transparently | P1 | Integration | IT-DS-041-001 | Deferred |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-DS-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **DS**: DataStorage service abbreviation
- **BR_NUMBER**: Business requirement number (040-043)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/datastorage/repository/txretry/retry.go`, `pkg/datastorage/config/config.go`, extracted `sortedCorrelationIDs` helper. Target: >=80% coverage.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-040-001` | Sorted correlation ID extraction returns keys in lexicographic order from a multi-key map | Pass |
| `UT-DS-040-002` | Sorted correlation ID extraction returns a single-element slice for a single-key map | Pass |
| `UT-DS-041-001` | Retry wrapper retries on `40P01` (deadlock_detected) and succeeds on second attempt | Pass |
| `UT-DS-041-002` | Retry wrapper retries on `40001` (serialization_failure) — backward-compatible behavior | Pass |
| `UT-DS-041-003` | Retry wrapper does NOT retry on non-retryable errors (e.g., `23505` unique violation) | Pass |
| `UT-DS-041-004` | Retry wrapper respects `maxRetries` limit and returns last error after exhaustion | Pass |
| `UT-DS-043-001` | Config validation applies default `MaxBatchSize` (500) when field is zero | Pass |
| `UT-DS-043-002` | Config validation rejects negative `MaxBatchSize` with descriptive error | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `CreateBatch`, `queryEffectivenessEvents`, `DBAdapter.Query`, `handleCreateAuditEventsBatch`. Target: >=80% coverage.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-040-001` | Concurrent `CreateBatch` calls with overlapping correlation IDs complete without deadlock | Pass (CI) |
| `IT-DS-040-002` | `CreateBatch` with single correlation ID produces correct hash chain (regression guard) | Pass (CI) |
| `IT-DS-042-001` | Effectiveness query cancels DB operation when caller context is cancelled | Pass (CI) |
| `IT-DS-042-002` | Adapter query cancels DB operation when caller context is cancelled | Pass (CI) |
| `IT-DS-042-003` | Effectiveness query with valid context returns correct events | Pass (CI) |
| `IT-DS-043-001` | Batch API returns 400 RFC7807 when payload exceeds configured maximum | Pass (CI) |
| `IT-DS-043-002` | Batch API accepts and persists payload at exactly the configured maximum | Pass (CI) |
| `IT-DS-043-003` | Batch API with zero `MaxBatchSize` in config uses default (500) and enforces it | Pass (CI) |
| `IT-DS-041-001` | Serializable `Disable` under concurrent load retries on deadlock and succeeds | Deferred |

### Tier Skip Rationale

- **E2E**: Deferred — Kind cluster E2E tests exercise full batch API path but deadlock reproduction requires specific timing. CI parallelism in integration tests is sufficient to validate lock ordering. E2E coverage will be validated when the retention worker (#667 L2) is wired.
- **BR-STORAGE-042 Unit**: Context propagation is purely I/O behavior (replacing `db.Query` with `db.QueryContext`). Unit tests cannot verify DB cancellation without a real connection. The 3 integration tests provide complete coverage.

---

## 9. Test Cases

### UT-DS-040-001: Sorted correlation ID extraction — multi-key map

**BR**: BR-STORAGE-040
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/sorted_correlation_ids_test.go`

**Preconditions**:
- `sortedCorrelationIDs` function exists and is exported (or package-level accessible)

**Test Steps**:
1. **Given**: A map with keys `{"charlie", "alpha", "bravo"}` (any value type)
2. **When**: `sortedCorrelationIDs` is called with the map
3. **Then**: Returns `["alpha", "bravo", "charlie"]`

**Expected Results**:
1. Slice length equals map key count (3)
2. Elements are in lexicographic ascending order

**Acceptance Criteria**:
- **Behavior**: Returns sorted keys
- **Correctness**: Order is deterministic regardless of map internal ordering
- **Accuracy**: No keys lost or duplicated

### UT-DS-040-002: Sorted correlation ID extraction — single-key map

**BR**: BR-STORAGE-040
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/sorted_correlation_ids_test.go`

**Preconditions**:
- `sortedCorrelationIDs` function exists

**Test Steps**:
1. **Given**: A map with single key `{"only-one"}`
2. **When**: `sortedCorrelationIDs` is called
3. **Then**: Returns `["only-one"]`

**Expected Results**:
1. Slice length is 1
2. Single element matches the only key

### UT-DS-041-001: Retry on 40P01 deadlock_detected

**BR**: BR-STORAGE-041
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/serializable_retry_test.go`

**Preconditions**:
- `txretry` package with retry function

**Test Steps**:
1. **Given**: A function that returns `*pgconn.PgError{Code: "40P01"}` on first call, `nil` on second
2. **When**: `WithRetry(ctx, 3, fn)` is called
3. **Then**: Returns `nil` (success on retry)

**Expected Results**:
1. Function was called exactly 2 times
2. Return value is `nil`

### UT-DS-041-002: Retry on 40001 serialization_failure (backward compat)

**BR**: BR-STORAGE-041
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/serializable_retry_test.go`

**Test Steps**:
1. **Given**: A function that returns `*pgconn.PgError{Code: "40001"}` on first call, `nil` on second
2. **When**: `WithRetry(ctx, 3, fn)` is called
3. **Then**: Returns `nil` (success on retry)

**Expected Results**:
1. Function was called exactly 2 times
2. Existing behavior is preserved

### UT-DS-041-003: No retry on non-retryable error

**BR**: BR-STORAGE-041
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/serializable_retry_test.go`

**Test Steps**:
1. **Given**: A function that returns `*pgconn.PgError{Code: "23505"}` (unique violation)
2. **When**: `WithRetry(ctx, 3, fn)` is called
3. **Then**: Returns the `23505` error immediately

**Expected Results**:
1. Function was called exactly 1 time
2. Returned error has code `23505`

### UT-DS-041-004: Retry exhaustion returns last error

**BR**: BR-STORAGE-041
**Priority**: P1
**Type**: Unit
**File**: `test/unit/datastorage/serializable_retry_test.go`

**Test Steps**:
1. **Given**: A function that always returns `*pgconn.PgError{Code: "40P01"}`
2. **When**: `WithRetry(ctx, 2, fn)` is called (max 2 retries)
3. **Then**: Returns the `40P01` error after 3 total attempts

**Expected Results**:
1. Function was called exactly 3 times (initial + 2 retries)
2. Returned error has code `40P01`

### UT-DS-043-001: Config default MaxBatchSize

**BR**: BR-STORAGE-043
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/config_test.go`

**Test Steps**:
1. **Given**: A valid config with `MaxBatchSize` set to 0 (zero value)
2. **When**: `Validate()` is called
3. **Then**: `MaxBatchSize` is set to 500 (default)

**Expected Results**:
1. No validation error
2. Config field reads 500

### UT-DS-043-002: Config rejects negative MaxBatchSize

**BR**: BR-STORAGE-043
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/config_test.go`

**Test Steps**:
1. **Given**: A config with `MaxBatchSize` set to -1
2. **When**: `Validate()` is called
3. **Then**: Returns an error containing "MaxBatchSize"

**Expected Results**:
1. Validation error is returned
2. Error message references the invalid field

### IT-DS-040-001: Concurrent CreateBatch without deadlock

**BR**: BR-STORAGE-040
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/create_batch_lock_ordering_test.go`

**Preconditions**:
- PostgreSQL test database available
- `audit_events` table and `audit_event_lock_id` function exist

**Test Steps**:
1. **Given**: Two batches sharing correlation IDs `{"corr-a", "corr-b"}`, each with 5 events
2. **When**: Both batches are submitted to `CreateBatch` concurrently using `errgroup`
3. **Then**: Both calls complete without error within 10 seconds

**Expected Results**:
1. Both `CreateBatch` calls return `nil` error
2. All 10 events are persisted (5 per batch)
3. No `40P01` deadlock error observed

### IT-DS-040-002: Single-correlation CreateBatch hash chain regression

**BR**: BR-STORAGE-040
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/create_batch_lock_ordering_test.go`

**Test Steps**:
1. **Given**: A batch of 3 events all with the same `correlation_id`
2. **When**: `CreateBatch` is called
3. **Then**: All 3 events are persisted with a valid hash chain

**Expected Results**:
1. 3 events returned with non-empty `event_hash`
2. Each event's `previous_event_hash` chains to the prior event's `event_hash`

### IT-DS-042-001: Effectiveness query cancellation on context cancel

**BR**: BR-STORAGE-042
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/context_propagation_test.go`

**Test Steps**:
1. **Given**: A context that is cancelled immediately after creation
2. **When**: `queryEffectivenessEvents(cancelledCtx, correlationID)` is called
3. **Then**: Returns a context-related error

**Expected Results**:
1. Error is non-nil
2. Error wraps or references context cancellation

### IT-DS-042-002: DBAdapter query cancellation on context cancel

**BR**: BR-STORAGE-042
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/context_propagation_test.go`

**Test Steps**:
1. **Given**: A context that is cancelled immediately after creation
2. **When**: `dbAdapter.Query(cancelledCtx, querySpec)` is called
3. **Then**: Returns a context-related error

**Expected Results**:
1. Error is non-nil
2. Error wraps or references context cancellation

### IT-DS-042-003: Effectiveness query returns correct events with valid context

**BR**: BR-STORAGE-042
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/context_propagation_test.go`

**Test Steps**:
1. **Given**: Pre-seeded effectiveness events for a known `correlation_id`
2. **When**: `queryEffectivenessEvents(validCtx, correlationID)` is called
3. **Then**: Returns the expected events in timestamp order

**Expected Results**:
1. Events match seeded data
2. Event order is ascending by timestamp

### IT-DS-043-001: Batch API rejects oversized payload

**BR**: BR-STORAGE-043
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/batch_size_limit_test.go`

**Test Steps**:
1. **Given**: Server configured with `MaxBatchSize=5`
2. **When**: POST to `/api/v1/audit/events/batch` with 6 events
3. **Then**: Response is 400 with RFC7807 body

**Expected Results**:
1. HTTP status is 400
2. Response body contains `"type"` field (RFC7807)
3. Response body detail references batch size limit

### IT-DS-043-002: Batch API accepts payload at maximum size

**BR**: BR-STORAGE-043
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/batch_size_limit_test.go`

**Test Steps**:
1. **Given**: Server configured with `MaxBatchSize=5`
2. **When**: POST to `/api/v1/audit/events/batch` with exactly 5 valid events
3. **Then**: Response is 201 and all events are persisted

**Expected Results**:
1. HTTP status is 201
2. Response body contains 5 created events

### IT-DS-043-003: Batch API with zero MaxBatchSize uses default

**BR**: BR-STORAGE-043
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/batch_size_limit_test.go`

**Test Steps**:
1. **Given**: Server configured with `MaxBatchSize=0` (triggers default 500)
2. **When**: POST to `/api/v1/audit/events/batch` with 501 events
3. **Then**: Response is 400

**Expected Results**:
1. HTTP status is 400
2. Default of 500 was applied and enforced

### IT-DS-041-001: Serializable Disable retries on deadlock

**BR**: BR-STORAGE-041
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/create_batch_lock_ordering_test.go`

**Test Steps**:
1. **Given**: An active action type with no dependent workflows
2. **When**: Two concurrent `Disable` calls are made for the same action type
3. **Then**: Both complete successfully (at least one via retry)

**Expected Results**:
1. No `40P01` error propagated to caller
2. Action type is disabled

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `*pgconn.PgError` constructed directly for retry predicate tests
- **Location**: `test/unit/datastorage/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see No-Mocks Policy)
- **Infrastructure**: Real PostgreSQL via DataStorage integration test suite (`test/integration/datastorage/suite_test.go`)
- **Location**: `test/integration/datastorage/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| PostgreSQL | 15+ | Integration test database |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PostgreSQL test DB | Infrastructure | Available | IT tests cannot run | Use `make test-integration-datastorage` which handles setup |
| Migration scripts | Code | Merged | `audit_event_lock_id` function missing | Run migrations in test suite setup |

### 11.2 Execution Order

1. **Phase 1**: TDD RED — Unit tests (UT-DS-040-001 through UT-DS-043-002)
2. **Phase 2**: TDD RED — Integration tests (IT-DS-040-001 through IT-DS-041-001)
3. **CHECKPOINT 1**: Adversarial audit of RED phase
4. **Phase 3**: TDD GREEN — Fix C1 (lock ordering)
5. **Phase 4**: TDD GREEN — Fix H1 (txretry 40P01)
6. **Phase 5**: TDD GREEN — Fix H2/H3 (context propagation)
7. **Phase 6**: TDD GREEN — Fix H4 (batch size limit)
8. **CHECKPOINT 2**: Adversarial audit of GREEN phase
9. **Phase 7**: TDD REFACTOR — Cleanup, docs, M1-M5 medium findings
10. **CHECKPOINT 3**: Final adversarial audit

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/667/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/datastorage/` | Ginkgo BDD test files |
| Integration test suite | `test/integration/datastorage/` | Ginkgo BDD test files |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
make test-unit-datastorage

# Integration tests
make test-integration-datastorage

# Specific test by ID
go test ./test/unit/datastorage/... -ginkgo.focus="UT-DS-040"

# Coverage (unit tier)
go test ./test/unit/datastorage/... -coverprofile=coverage_ut.out \
  --coverpkg=github.com/jordigilh/kubernaut/pkg/datastorage/repository/txretry/...,github.com/jordigilh/kubernaut/pkg/datastorage/config/...
go tool cover -func=coverage_ut.out

# Coverage (integration tier)
go test ./test/integration/datastorage/... -coverprofile=coverage_it.out \
  --coverpkg=github.com/jordigilh/kubernaut/pkg/datastorage/repository/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/...,github.com/jordigilh/kubernaut/pkg/datastorage/adapter/...
go tool cover -func=coverage_it.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/datastorage/serializable_retry_test.go` | Tests `isSerializationFailure` for `40001` only | Add tests for `40P01` and rename references | Function renamed to `isRetryablePostgresError` |
| `test/unit/datastorage/handlers_test.go` | `MockDB.Query` without context | Update mock signature to include `context.Context` | `DBInterface.Query` signature change (H3) |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan — 17 tests across 2 tiers for Issue #667 |
| 2.0 | 2026-04-06 | All 8 unit tests pass; 8 integration tests compile and pass (CI). IT-DS-041-001 deferred (P1 stress). Status: Complete. |
