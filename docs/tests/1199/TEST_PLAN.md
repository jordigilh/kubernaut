# Test Plan: Data Store Bidirectional Correlation Query for Task-to-RR Mapping

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1199-v1.0
**Feature**: Data Store bidirectional correlation query (task_id ↔ rr_name)
**Version**: 1.0
**Created**: 2026-05-20
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/1199-ds-bidirectional-correlation`
**Parent Issue**: [#1189](https://github.com/jordigilh/kubernaut/issues/1189) — AC 13

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the Data Store bidirectional correlation query that enables
lookups between A2A `task_id` and `rr_name`/`rr_namespace`. This closes the final
acceptance criterion (AC 13) from Issue #1189.

### 1.2 Feature Description

PR #1193 implemented AF-side audit enrichment: `enrichRRDetail()` populates `rr_name`
and `rr_namespace` on `EventA2ATaskCompleted`/`EventA2ATaskFailed` from the shared
`CreateContext`. However, a data pipeline gap prevents this data from reaching PostgreSQL:
`buildEventData()` in `store_adapter.go` drops these fields during conversion to the
typed OpenAPI payload.

This issue addresses three concerns:
1. **Schema fix**: Add `rr_name`/`rr_namespace` optional fields to the
   `ApifrontendA2ATaskCompletedPayload` and `ApifrontendA2ATaskFailedPayload` OpenAPI schemas
2. **Store adapter wiring**: Pass `rr_name`/`rr_namespace` from the detail map into the typed payload
3. **Query API extension**: Add `detail_key`/`detail_value` filter parameters to `QueryAuditEvents`

### 1.3 Objectives

1. Validate `buildEventData()` persists `rr_name`/`rr_namespace` in `event_data` JSONB for
   task-completed and task-failed events
2. Validate `AuditEventsQueryBuilder` produces correct SQL for JSONB field filtering via `->>` operator
3. Validate `handleQueryAuditEvents` parses and applies `detail_key`/`detail_value` query params
4. Validate bidirectional correlation: task_id → rr_name and rr_name → task_id
5. Validate no regression on existing query filters, pagination, and event mapping

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/audit/... ./pkg/datastorage/query/... -ginkgo.v` |
| Unit test code coverage (modified files) | >=80% | `go test -coverprofile` |
| Integration test pass rate | 100% | `make test-integration-datastorage` |
| Race detector | 0 races | `go test -race` |
| Build success | 0 errors | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run --timeout=5m` |
| BR coverage | All 5 ACs | Coverage matrix in Section 7 |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #1199](https://github.com/jordigilh/kubernaut/issues/1199) — Bidirectional correlation query
- [Issue #1189](https://github.com/jordigilh/kubernaut/issues/1189) — 4-Phase Interactive Remediation Journey (parent)
- [PR #1193](https://github.com/jordigilh/kubernaut/pull/1193) — AF-side audit enrichment
- [ADR-034](docs/architecture/decisions/ADR-034-unified-audit-table-design.md) — Unified audit table design
- [DD-STORAGE-010](docs/architecture/decisions/) — Query API pagination strategy
- [TESTING_GUIDELINES.md](docs/development/business-requirements/TESTING_GUIDELINES.md) — Per-tier coverage >=80%
- [ANTI_PATTERN_DETECTION.md](docs/testing/ANTI_PATTERN_DETECTION.md) — Forbidden test patterns
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — TDD REFACTOR reference

### 2.2 Implementation Files

| File | Role |
|------|------|
| `api/openapi/data-storage-v1.yaml` | Add `rr_name`/`rr_namespace` to A2A payloads; add `detail_key`/`detail_value` query params |
| `pkg/datastorage/ogen-client/` | Regenerated client (mechanical) |
| `pkg/apifrontend/audit/store_adapter.go` | Wire `rr_name`/`rr_namespace` into `EventA2ATaskCompleted`/`Failed` payloads |
| `pkg/datastorage/query/audit_events_builder.go` | Add `WithEventDataFilter(key, value)` method |
| `pkg/datastorage/server/audit_events_handler.go` | Parse `detail_key`/`detail_value` from query params |

### 2.3 Existing Related Tests

| File | Test IDs | Relationship |
|------|----------|-------------|
| `pkg/apifrontend/audit/store_adapter_test.go` | UT-AF-1156-011, -012 | Event type mapping for A2A completed/failed (baseline) |
| `pkg/apifrontend/launcher/enrichrr_test.go` | UT-AF-1189-040..043 | enrichRRDetail unit tests (prerequisite, merged) |
| `pkg/datastorage/query/benchmark_test.go` | — | Query builder benchmark baseline |
| `test/e2e/fullpipeline/06_af_audit_trace_test.go` | E2E-FP-AF-001 | AF audit trace in full pipeline |
| `test/integration/datastorage/audit_events_schema_test.go` | — | JSONB containment query precedent |

### 2.4 Proven Codebase Patterns

| Pattern | Evidence | Location |
|---------|----------|----------|
| Optional fields on discriminated union member | `DurationMs OptInt` on `ApifrontendA2ATaskCompletedPayload` | `oas_schemas_gen.go:3963` |
| `OptString` for `rr_name`/`rr_namespace` | `RrName OptString` on `ApifrontendKADelegatedPayload` | `oas_schemas_gen.go:5022` |
| Parameterized JSONB `->>` in production SQL | `event_data->>'pre_remediation_spec_hash' = $1` | `remediation_history_repository.go:200` |
| GIN index on `event_data` | `CREATE INDEX idx_audit_events_event_data_gin ON audit_events USING GIN (event_data)` | `001_v1_schema.sql:449` |
| `capturingStore` for unit-testing audit payloads | Captures `*ogenclient.AuditEventRequest`, typed accessors for assertions | `store_adapter_test.go:15-44` |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Ogen regeneration produces unexpected diff | Build failure | Low | All | `make gen-diff` verifies deterministic output; version pinned at `v1.20.1` |
| R2 | `->>` on unindexed JSONB key slow at scale | Query latency >1s | Low | IT-DS-1199-010..011 | GIN index provides baseline; expression index deferred to follow-up |
| R3 | `detail_key` user input enables SQL injection | Security breach | Low | UT-DS-1199-007, -008 | Key allowlisted OR parameterized via `->>` with `$N` (proven pattern) |
| R4 | `rr_name`/`rr_namespace` empty for tasks that don't create RRs | False negatives in correlation queries | None | UT-AF-1199-003 | Fields are `OptString` (optional); query returns empty result set for non-RR tasks |
| R5 | Concurrent writes to `event_data` JSONB during high A2A throughput | Data corruption | None | — | `event_data` is written once at INSERT; no UPDATE path for audit events (immutable) |

---

## 4. Scope

### 4.1 Features to be Tested

- **Store adapter wiring**: `buildEventData()` passes `rr_name`/`rr_namespace` from detail map to typed payload
- **Query builder**: `WithEventDataFilter(key, value)` generates `AND event_data->>'key' = $N`
- **Query handler**: `parseQueryFilters()` extracts `detail_key`/`detail_value` from HTTP request
- **Bidirectional lookup**: task_id → rr_name and rr_name → task_id via `QueryAuditEvents`
- **OpenAPI spec**: Updated schemas and query parameters

### 4.2 Features Not to be Tested

- **Ogen codegen internals**: Generated code is mechanical; tested by ogen's own suite
- **GIN index performance tuning**: Deferred to follow-up; existing index is sufficient for GA
- **AF enrichment logic**: Already tested by UT-AF-1189-040..043 (merged)
- **E2E full pipeline**: Requires Kind cluster with AF + DS + DEX; deferred to E2E suite update

### 4.3 Design Decisions

- **`->>` over `@>`**: Text extraction with parameterized `$N` is proven in production (`remediation_history_repository.go`). Containment (`@>`) with parameterized JSONB is unproven in this codebase. Trade-off: `->>` is slightly less GIN-optimal but fully safe and tested.
- **Key sanitization**: `detail_key` restricted to `[a-zA-Z0-9_]` pattern, consistent with `sanitizeJSONBKey()` in `workflow/scoring.go`. Reject invalid keys with 400 Bad Request.
- **Optional fields**: `rr_name`/`rr_namespace` are `OptString` (not required) because not every A2A task creates an RR.

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: TESTING_GUIDELINES.md — Per-Tier Testable Code Coverage (>=80% per tier).

| Tier | Scope | Target | Code Subset |
|------|-------|--------|-------------|
| Unit | `buildEventData` wiring, `WithEventDataFilter`, key sanitization | >=80% | Pure logic: payload construction, SQL building, input validation |
| Integration | Round-trip write→query with real PostgreSQL | >=80% | I/O: HTTP handler, DB query, parameterized SQL |
| E2E | Deferred (requires Kind + AF + DS) | N/A | Full stack |

### 5.2 TDD Phases

| Phase | Description | Deliverables | Checkpoint |
|-------|-------------|-------------|------------|
| **Phase 0: PREREQUISITE** | OpenAPI schema update + ogen codegen | Updated spec, regenerated client | CHECKPOINT 0 |
| **Phase 1: TDD RED** | Write all failing tests | `store_adapter_test.go` additions, `audit_events_builder_test.go`, `audit_events_handler_test.go` | CHECKPOINT 1 |
| **Phase 2: TDD GREEN** | Minimal implementation to pass all tests | `store_adapter.go` fix, `audit_events_builder.go` extension, `audit_events_handler.go` extension | CHECKPOINT 2 |
| **Phase 3: TDD REFACTOR** | Code quality: 100 Go Mistakes audit, lint, dedup | Cleaned code, no new lint errors | CHECKPOINT 3 |

### 5.3 Anti-Pattern Compliance

Per ANTI_PATTERN_DETECTION.md:

- Test business outcomes, not implementation details (no NULL-TESTING)
- No `Skip()` or pending tests
- No `time.Sleep()` without approved exception
- Use table-driven tests where appropriate
- All test descriptions include test ID (e.g., `UT-AF-1199-001`)
- Mock only external dependencies (DB is external for unit tests, real for integration)

---

## 6. Test Design Specification

### 6.1 Unit Tests — Store Adapter Wiring (Tier 1)

**Test file**: `pkg/apifrontend/audit/store_adapter_test.go` (extend existing)

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-AF-1199-001 | `EventA2ATaskCompleted` with `rr_name`/`rr_namespace` in detail map → payload contains `RrName`/`RrNamespace` as `OptString` | AC 1 | Happy Path |
| UT-AF-1199-002 | `EventA2ATaskFailed` with `rr_name`/`rr_namespace` in detail map → payload contains `RrName`/`RrNamespace` as `OptString` | AC 2 | Happy Path |
| UT-AF-1199-003 | `EventA2ATaskCompleted` without `rr_name` in detail map → `RrName.Set` is false (optional field not populated) | AC 1 | Edge Case |
| UT-AF-1199-004 | `EventA2ATaskFailed` without `rr_name` in detail map → `RrName.Set` is false | AC 2 | Edge Case |

### 6.2 Unit Tests — Query Builder JSONB Filter (Tier 1)

**Test file**: `pkg/datastorage/query/audit_events_builder_test.go` (new or extend)

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-DS-1199-005 | `WithEventDataFilter("task_id", "task-abc")` → SQL contains `AND event_data->>'task_id' = $N` with correct arg | AC 1, AC 2 | Happy Path |
| UT-DS-1199-006 | `WithEventDataFilter` combined with `WithEventType` and `WithSince` → all filters chained correctly, arg indices sequential | AC 1 | Combination |
| UT-DS-1199-007 | `WithEventDataFilter("invalid!key", "val")` → returns validation error (key contains non-alphanumeric chars) | AC 3 | Security |
| UT-DS-1199-008 | `WithEventDataFilter("", "val")` → returns validation error (empty key) | AC 3 | Edge Case |
| UT-DS-1199-009 | `WithEventDataFilter("task_id", "")` → filter not applied (empty value treated as no-op) | AC 1 | Edge Case |
| UT-DS-1199-012 | `BuildCount()` with `WithEventDataFilter` → COUNT query also includes JSONB filter | AC 1 | Happy Path |

### 6.3 Integration Tests — Bidirectional Correlation (Tier 2)

**Test file**: `test/integration/datastorage/audit_correlation_test.go` (new)

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| IT-DS-1199-010 | Write audit event with `task_id` + `rr_name` in `event_data`, query by `detail_key=task_id&detail_value=task-abc` → returns event with `rr_name` in `event_data` | AC 1 | Happy Path |
| IT-DS-1199-011 | Write audit event with `task_id` + `rr_name` in `event_data`, query by `detail_key=rr_name&detail_value=rr-oom-web` → returns event with `task_id` in `event_data` | AC 2 | Happy Path |
| IT-DS-1199-013 | Query with `detail_key=task_id&detail_value=nonexistent` → returns empty data array, pagination total=0 | AC 1 | Not Found |
| IT-DS-1199-014 | Query combining `detail_key=task_id` + `event_type=apifrontend.a2a.task_completed` → returns only matching event type | AC 1 | Combination |
| IT-DS-1199-015 | Query with `detail_key` but no `detail_value` → 400 Bad Request (RFC 7807) | AC 4 | Validation |
| IT-DS-1199-016 | Query with `detail_value` but no `detail_key` → 400 Bad Request (RFC 7807) | AC 4 | Validation |
| IT-DS-1199-017 | Query with invalid `detail_key` (contains special chars) → 400 Bad Request | AC 3 | Security |

### 6.4 Unit Tests — Handler Query Param Parsing (Tier 1)

**Test file**: `pkg/datastorage/server/audit_events_handler_test.go` (new or extend)

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-DS-1199-018 | `parseQueryFilters` with `detail_key=task_id&detail_value=abc` → filters struct has `detailKey="task_id"`, `detailValue="abc"` | AC 4 | Happy Path |
| UT-DS-1199-019 | `parseQueryFilters` with no `detail_key`/`detail_value` → filters struct has empty strings (no filter applied) | AC 4 | Edge Case |
| UT-DS-1199-020 | `parseQueryFilters` with `detail_key` only → returns validation error | AC 4 | Validation |

---

## 7. BR Coverage Matrix

| AC | Description | Test Type | Test IDs | Status |
|----|-------------|-----------|----------|--------|
| AC 1 | DS API supports querying by `detail.task_id` → returns events with `rr_name` | Unit + IT | UT-AF-1199-001, -003, UT-DS-1199-005..006, -009, -012, IT-DS-1199-010, -013, -014 | **This plan** |
| AC 2 | DS API supports querying by `detail.rr_name` → returns events with `task_id` | Unit + IT | UT-AF-1199-002, -004, IT-DS-1199-011 | **This plan** |
| AC 3 | Queries indexed for reasonable performance | Unit | UT-DS-1199-007, -008, IT-DS-1199-017 | **This plan** (validation); GIN index exists |
| AC 4 | OpenAPI spec updated | Unit + IT | UT-DS-1199-018..020, IT-DS-1199-015, -016 | **This plan** |
| AC 5 | Integration test validates both query directions | IT | IT-DS-1199-010, -011 | **This plan** |

---

## 8. Test Case Specifications

### 8.1 UT-AF-1199-001: EventA2ATaskCompleted payload includes rr_name/rr_namespace

**AC**: AC 1
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**: `StoreAdapter` instantiated with `capturingStore`

**Steps**:
1. **Given**: An `audit.Event` with `Type: EventA2ATaskCompleted` and `Detail: {"session_id":"s1", "task_id":"task-abc", "rr_name":"rr-oom-web", "rr_namespace":"production"}`
2. **When**: `adapter.Emit(ctx, event)` is called
3. **Then**: Captured event's `EventData` is `ApifrontendA2ATaskCompletedPayload`

**Expected Result**:
- `payload.TaskID` equals `"task-abc"`
- `payload.RrName.Value` equals `"rr-oom-web"`
- `payload.RrName.Set` is `true`
- `payload.RrNamespace.Value` equals `"production"`
- `payload.RrNamespace.Set` is `true`

### 8.2 UT-DS-1199-005: WithEventDataFilter generates correct SQL

**AC**: AC 1, AC 2
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**: `AuditEventsQueryBuilder` instantiated with default options

**Steps**:
1. **Given**: Builder with `WithEventDataFilter("task_id", "task-abc")`
2. **When**: `Build()` is called
3. **Then**: SQL string and args are correct

**Expected Result**:
- SQL contains `event_data->>'task_id' = $1`
- `args[0]` equals `"task-abc"`
- Subsequent filters (limit, offset) use `$2`, `$3` (sequential arg indices)

### 8.3 IT-DS-1199-010: Bidirectional query task_id → rr_name

**AC**: AC 1, AC 5
**Type**: Integration
**Category**: Happy Path
**Priority**: P0

**Preconditions**: Real PostgreSQL with `audit_events` table. Test event inserted.

**Steps**:
1. **Given**: An audit event inserted with `event_type='apifrontend.a2a.task_completed'`, `event_data='{"event_type":"apifrontend.a2a.task_completed","task_id":"task-abc","rr_name":"rr-oom-web","rr_namespace":"production"}'`
2. **When**: HTTP GET `/api/v1/audit/events?detail_key=task_id&detail_value=task-abc`
3. **Then**: Response contains the matching event

**Expected Result**:
- HTTP 200 with `data` array length >= 1
- First event's `event_data` contains `rr_name: "rr-oom-web"`
- First event's `event_data` contains `task_id: "task-abc"`

### 8.4 UT-DS-1199-007: Invalid detail_key rejected

**AC**: AC 3
**Type**: Unit
**Category**: Security
**Priority**: P1

**Preconditions**: `AuditEventsQueryBuilder` instantiated

**Steps**:
1. **Given**: Builder with `WithEventDataFilter("task_id'; DROP TABLE--", "val")`
2. **When**: `Build()` is called
3. **Then**: Returns validation error

**Expected Result**:
- Error message indicates invalid key format
- No SQL generated

---

## 9. Checkpoint Specifications

### CHECKPOINT 0 — After Phase 0 (Prerequisite: Schema + Codegen)

**Gate criteria**: OpenAPI spec updated, ogen client regenerated, build passes.

**Preflight Check**:
- [ ] `rr_name` (optional string) added to `ApifrontendA2ATaskCompletedPayload` in OpenAPI spec
- [ ] `rr_namespace` (optional string) added to `ApifrontendA2ATaskCompletedPayload` in OpenAPI spec
- [ ] Same two fields added to `ApifrontendA2ATaskFailedPayload`
- [ ] `detail_key` and `detail_value` query params added to `queryAuditEvents` operation
- [ ] `make generate-datastorage-client` succeeds
- [ ] `go build ./...` succeeds with regenerated client
- [ ] `make gen-diff` confirms generated files are deterministic
- [ ] Generated `ApifrontendA2ATaskCompletedPayload` struct has `RrName OptString` and `RrNamespace OptString`
- [ ] Generated `QueryAuditEventsParams` struct has `DetailKey OptString` and `DetailValue OptString`

**GA Readiness Audit (Phase 0)**:

| Dimension | Check | Status |
|-----------|-------|--------|
| Schema completeness | Payload schemas have optional `rr_name`/`rr_namespace` | Pending |
| Codegen integrity | `make gen-diff` clean | Pending |
| Build success | `go build ./...` zero errors | Pending |
| Backward compatibility | N/A (not required) | N/A |

---

### CHECKPOINT 1 — After Phase 1 (TDD RED)

**Gate criteria**: All tests written and verified to FAIL (compile but red).

#### 9-Category Audit

| # | Category | Tests That Satisfy | Notes |
|---|----------|--------------------|-------|
| 1 | **Observability wiring** | UT-AF-1199-001..004 validate audit event payloads carry correlation data | No new audit events; extends existing |
| 2 | **Adversarial inputs** | UT-DS-1199-007 (SQL injection key), UT-DS-1199-008 (empty key), IT-DS-1199-017 (special chars) | All external inputs covered |
| 3 | **Resource bounds** | UT-DS-1199-009 (empty value no-op) | Builder is per-call; no growing state |
| 4 | **Concurrency** | N/A — query builder is stateless, audit events are immutable inserts | No shared mutable state |
| 5 | **Nil/zero edge cases** | UT-AF-1199-003, -004 (missing rr_name), UT-DS-1199-008 (empty key), UT-DS-1199-009 (empty value) | All nil/zero paths covered |
| 6 | **Error-path observability** | UT-DS-1199-007, -008 (validation errors), IT-DS-1199-015, -016 (400 Bad Request) | Errors produce RFC 7807 responses |
| 7 | **Cross-phase integration** | IT-DS-1199-010, -011 (write→query round trip), -014 (combined filters) | Full HTTP→DB→HTTP chain |
| 8 | **Spec compliance** | UT-DS-1199-018..020 (OpenAPI param parsing), UT-DS-1199-005 (SQL matches ADR-034 column names) | Verified against OpenAPI spec |
| 9 | **API surface hygiene** | No new exported types beyond `WithEventDataFilter` on existing builder | Minimal surface growth |

**Preflight Check**:
- [ ] All 20 test specs compile
- [ ] All 20 test specs FAIL (red)
- [ ] No `Skip()` or pending tests
- [ ] Test descriptions include test IDs
- [ ] All test files use Ginkgo/Gomega BDD framework
- [ ] No NULL-TESTING anti-patterns (all assertions test business outcomes)
- [ ] Confidence >= 95%? If not, escalate findings before proceeding

---

### CHECKPOINT 2 — After Phase 2 (TDD GREEN)

**Gate criteria**: All tests PASS. `go build ./...` succeeds. `go vet ./...` clean.

#### 9-Category Audit (Re-verify)

| # | Category | Verification |
|---|----------|-------------|
| 1 | **Observability wiring** | Run UT-AF-1199-001..004: payload fields populated correctly |
| 2 | **Adversarial inputs** | Run UT-DS-1199-007, -008, IT-DS-1199-017: all reject invalid input |
| 3 | **Resource bounds** | Code review: no maps/slices that grow across calls |
| 4 | **Concurrency** | Run `go test -race ./pkg/datastorage/query/...`: zero races |
| 5 | **Nil/zero edge cases** | Run UT-AF-1199-003, -004, UT-DS-1199-008, -009: all pass |
| 6 | **Error-path observability** | Run IT-DS-1199-015, -016: 400 responses with RFC 7807 body |
| 7 | **Cross-phase integration** | Run IT-DS-1199-010, -011: bidirectional queries return correct events |
| 8 | **Spec compliance** | Verify SQL uses `->>` (not `->`) for text extraction from JSONB |
| 9 | **API surface hygiene** | Verify no unexported helper functions leaked to public API |

**Preflight Check**:
- [ ] All 20 tests PASS
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` clean
- [ ] `go test -race ./pkg/apifrontend/audit/... ./pkg/datastorage/query/...` — zero races
- [ ] Existing store adapter tests (UT-AF-1156-*) still pass (regression)
- [ ] Existing query builder tests still pass (regression)
- [ ] Confidence >= 95%? If not, escalate findings before proceeding

**GA Readiness Audit (Phase 2)**:

| Dimension | Check | Status |
|-----------|-------|--------|
| Data pipeline integrity | `rr_name`/`rr_namespace` flow from detail map → payload → event_data JSONB | Pending |
| Query infrastructure | `->>` filter with parameterized `$N`, arg indices correct | Pending |
| Security | Key validation rejects non-alphanumeric input | Pending |
| Performance | GIN index exists; no sequential scan for JSONB queries | Verified (pre-existing) |
| Test coverage | >=80% on modified files per tier | Pending |

---

### CHECKPOINT 3 — After Phase 3 (TDD REFACTOR)

**Gate criteria**: All tests still PASS. Code quality validated against 100 Go Mistakes.

#### 100 Go Mistakes Audit

| Mistake # | Title | Check | Status |
|-----------|-------|-------|--------|
| #1 | Unintended variable shadowing | No `err :=` inside `if` blocks that shadow outer `err` | Pending |
| #2 | Unnecessary nested code | Use early returns for validation errors | Pending |
| #3 | Misusing init functions | No `init()` in new code | Pending |
| #9 | Being confused about when to use generics | No generics needed; concrete types throughout | Pending |
| #16 | Not using linters effectively | `golangci-lint run --timeout=5m` | Pending |
| #21 | Inefficient slice initialization | Pre-size `args` slice in builder with `make([]interface{}, 0, N)` | Pending |
| #28 | Maps and memory leaks | No persistent maps in new code; builder is per-call | Pending |
| #36 | Not understanding the concept of a rune | Key validation regex operates on ASCII; no rune issues | Pending |
| #39 | Under-optimized string concatenation | Builder uses `fmt.Sprintf` for SQL; consistent with existing code | Pending |
| #48 | Forgetting about `context.Context` | Handler passes `ctx` to DB query; builder is context-free (pure) | Pending |
| #53 | Not handling defer errors | No new defers in builder; handler defers follow existing pattern | Pending |
| #54 | Not closing resources | No new resources opened | Pending |
| #77 | JSON handling mistakes | No JSON parsing in new code (ogen handles serialization) | Pending |
| #84 | Not using testing utility packages | Tests use Ginkgo/Gomega (project standard) | Pending |
| #100 | Not understanding Go diagnostics tooling | `go vet`, `golangci-lint` pass | Pending |

#### 9-Category Re-Audit (Refactored Code)

All 9 categories re-verified against refactored code.

**Preflight Check**:
- [ ] All 20 tests still PASS
- [ ] `golangci-lint run --timeout=5m` — zero new errors
- [ ] `go vet ./...` — clean
- [ ] All `Expect` assertions include business-outcome context
- [ ] No duplicated code patterns (key validation DRY with `sanitizeJSONBKey` if applicable)
- [ ] Builder method follows existing `With*()` naming convention
- [ ] Confidence >= 95%? If not, escalate findings before proceeding

**Final GA Readiness Audit (Phase 3)**:

| Dimension | Check | Status |
|-----------|-------|--------|
| Data pipeline integrity | End-to-end: enrichRRDetail → buildEventData → JSONB → query → response | Pending |
| Schema completeness | OpenAPI spec, generated types, handler, builder all aligned | Pending |
| Query infrastructure | SQL correct, arg indices sequential, `->>` with `$N` | Pending |
| Security | Key sanitization, parameterized values, no interpolation | Pending |
| Performance | GIN index, partition pruning with time filters | Verified |
| Observability | Existing handler logging covers new filter | Verified |
| Test coverage | >=80% per tier on modified files | Pending |
| Lint compliance | Zero new errors from golangci-lint | Pending |
| 100 Go Mistakes | All applicable checks pass | Pending |

---

## 10. Implementation Phases (TDD)

### Phase 0: PREREQUISITE — Schema + Codegen

**Files to modify**:
1. `api/openapi/data-storage-v1.yaml` — Add optional `rr_name`/`rr_namespace` to `ApifrontendA2ATaskCompletedPayload` and `ApifrontendA2ATaskFailedPayload`; add `detail_key`/`detail_value` to `queryAuditEvents` params
2. `pkg/datastorage/ogen-client/` — `make generate-datastorage-client`

**Expected state**: Build passes with new generated types. No behavioral changes yet.

**CHECKPOINT 0 gate**: Schema + codegen preflight before proceeding.

### Phase 1: TDD RED — Write Failing Tests

**Files to create/modify**:
1. `pkg/apifrontend/audit/store_adapter_test.go` — Add UT-AF-1199-001..004
2. `pkg/datastorage/query/audit_events_builder_test.go` — Add UT-DS-1199-005..009, -012
3. `test/integration/datastorage/audit_correlation_test.go` — Add IT-DS-1199-010..011, -013..017
4. `pkg/datastorage/server/audit_events_handler_test.go` — Add UT-DS-1199-018..020

**Expected state**: All tests compile but FAIL (no implementation yet).

**CHECKPOINT 1 gate**: 9-category audit + preflight before proceeding.

### Phase 2: TDD GREEN — Minimal Implementation

**Files to modify**:
1. `pkg/apifrontend/audit/store_adapter.go` — Wire `rr_name`/`rr_namespace` in `buildEventData()` for `EventA2ATaskCompleted` and `EventA2ATaskFailed` (~4 lines)
2. `pkg/datastorage/query/audit_events_builder.go` — Add `WithEventDataFilter(key, value)` method + key validation (~25 lines)
3. `pkg/datastorage/server/audit_events_handler.go` — Parse `detail_key`/`detail_value` in `parseQueryFilters()`, apply in `buildQueryFromFilters()` (~10 lines)

**Expected state**: All tests PASS. `go build ./...` succeeds.

**CHECKPOINT 2 gate**: 9-category audit + regression check + GA readiness audit before proceeding.

### Phase 3: TDD REFACTOR — Code Quality

**Activities**:
1. 100 Go Mistakes audit (table in Section 9)
2. Extract key validation to shared utility if duplicated with `sanitizeJSONBKey`
3. `golangci-lint run --timeout=5m`
4. `go vet ./...`
5. Review `->>` SQL generation for consistency with `remediation_history_repository.go`
6. Ensure `BuildCount()` includes JSONB filter (parity with `Build()`)

**Expected state**: All tests still PASS. No new lint errors. Code quality improved.

**CHECKPOINT 3 gate**: 9-category re-audit + 100 Go Mistakes verification + final GA readiness audit.

---

## 11. Coverage Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Unit test coverage (store_adapter.go changes) | >=80% | Pending |
| Unit test coverage (audit_events_builder.go changes) | >=80% | Pending |
| Unit test coverage (audit_events_handler.go changes) | >=80% | Pending |
| Integration test coverage (correlation query) | >=80% | Pending |
| Race detector | 0 races | Pending |
| Lint compliance | 0 new errors | Pending |
| Regression (existing tests) | 100% pass | Pending |

---

## 12. Execution Commands

```bash
# Phase 0: Schema + codegen
make generate-datastorage-client
go build ./...
make gen-diff

# Phase 1-3: Unit tests (store adapter)
go test -race -count=1 ./pkg/apifrontend/audit/... -ginkgo.v -ginkgo.focus="1199"

# Phase 1-3: Unit tests (query builder)
go test -race -count=1 ./pkg/datastorage/query/... -ginkgo.v -ginkgo.focus="1199"

# Phase 1-3: Integration tests (requires PostgreSQL)
make test-integration-datastorage -- -ginkgo.focus="1199"

# Full regression
go test -race -count=1 ./pkg/apifrontend/audit/... -ginkgo.v
go test -race -count=1 ./pkg/datastorage/query/... -ginkgo.v

# Coverage
go test -coverprofile=coverage.out ./pkg/apifrontend/audit/... ./pkg/datastorage/query/... && go tool cover -func=coverage.out

# Build + lint
go build ./...
go vet ./...
golangci-lint run --timeout=5m
```

---

## 13. Dependencies

| Dependency | Version | Usage |
|------------|---------|-------|
| `github.com/ogen-go/ogen` | v1.20.1 | OpenAPI client generation (pinned in `gen.go`) |
| `github.com/jackc/pgx/v5` | v5.9.2 | PostgreSQL driver (parameterized `->>` queries) |
| `github.com/onsi/ginkgo/v2` | latest | BDD test framework |
| `github.com/onsi/gomega` | latest | Assertion library |
| No new external dependencies | — | All changes use existing packages |

---

## 14. Sign-off

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Assistant | 2026-05-20 | Draft |
| Technical Review | | | Pending |
| QE Review | | | Pending |

---

## 15. Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-20 | AI Assistant | Initial test plan for Issue #1199 |
