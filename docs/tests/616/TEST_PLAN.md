# Test Plan: Issue #616 — Remediation Feedback Loop Restoration

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-616-v1.1
**Feature**: Fix two bugs breaking the remediation history feedback loop (DS query + HAPI enrichment) and add RO observability
**Version**: 1.1
**Created**: 2026-04-03
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fixes for two bugs that together break the remediation history feedback loop (DD-HAPI-016), plus observability improvements that prevented diagnosis. Without these fixes, the LLM operates without knowledge of prior remediation failures and the RO guardrails see zero history entries, allowing ineffective remediations to repeat indefinitely.

### 1.2 Objectives

1. **DS query fix**: `QueryROEventsBySpecHash` matches `currentSpecHash` against BOTH `pre_remediation_spec_hash` (RO events) AND `post_remediation_spec_hash` (EM events via correlation_id join)
2. **HAPI enrichment fix**: `history_fetcher` initializes successfully with `TypedDict` config (no `.to_dict()` crash)
3. **RO observability**: Audit timer tick reduced from INFO to V(1); routing path logs at V(1) when `dsClient` is nil and DS query results
4. **Zero regressions**: All 47 existing DS unit tests, 8 DS integration tests, 5 DS E2E tests, and 59 HAPI Python tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/...` + `pytest kubernaut-agent/tests/unit/` |
| Integration test pass rate | 100% | `go test ./test/integration/datastorage/...` + `pytest kubernaut-agent/tests/integration/` |
| Unit-testable code coverage (DS logic) | >=80% | Coverage on `remediation_history_logic.go`, `remediation_history_repository.go` |
| Unit-testable code coverage (HAPI) | >=80% | Coverage on `remediation_history_client.py`, enrichment wiring |
| Backward compatibility | 0 regressions | All existing remediation history tests pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-HAPI-016**: Remediation History Context Enrichment (P0 CRITICAL)
- **DD-HAPI-016 v1.3**: Remediation History Context — two-tier query, three-way hash comparison
- **ADR-055**: LLM-Driven Context Enrichment (tool-based history discovery)
- **Issue #616**: RO routing guardrails not triggering on Kind — ineffective chain / recurrence detection

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [DD-HAPI-016 Test Plan](../../testing/DD-HAPI-016/TEST_PLAN.md) (existing feature tests)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Post-hash subquery degrades DS query performance | Slow history API responses, LLM timeout | Medium | IT-DS-616-003 | New expression index on `post_remediation_spec_hash`; integration test validates query returns within acceptable time |
| R2 | SQL OR clause correctness — both pre-hash and post-hash matches must appear in results | Missing entries from one path, incomplete chain | Medium | UT-DS-616-003, IT-DS-616-003 | Unit and integration tests verify entries from both match paths appear; SQL OR on unique rows cannot produce duplicates |
| R3 | Existing tests hardcoded to pre-hash-only behavior break | False regression | Low | All existing UT/IT | Existing tests use pre-hash matching which still works; new tests validate post-hash path |
| R4 | Audit tick verbosity change hides real flush issues | Operators miss audit pipeline problems | Low | UT-AUDIT-616-001 | Keep flush errors at ERROR level; only reduce tick metadata to V(1) |
| R5 | HAPI enrichment fix exposes new failures in enrichment pipeline | Phase 2 enrichment fails for different reasons | Low | UT-HAPI-616-001, UT-HAPI-616-002 | Test both happy path and error paths |

---

## 4. Scope

### 4.1 Features to be Tested

- **DS Query** (`pkg/datastorage/repository/remediation_history_repository.go`): `QueryROEventsBySpecHash` expanded to match post-hash via EM correlation
- **DS Logic** (`pkg/datastorage/server/remediation_history_logic.go`): `CorrelateTier1Chain` produces correct `hashMatch` for post-hash-matched entries
- **DS Handler** (`pkg/datastorage/server/remediation_history_handler.go`): Full flow returns entries for both pre and post hash matches
- **HAPI Enrichment** (`kubernaut-agent/src/extensions/incident/llm_integration.py`): `history_fetcher` init with TypedDict config
- **RO Routing Observability** (`pkg/remediationorchestrator/routing/blocking.go`): Logging on `dsClient==nil` and DS query results
- **Audit Tick Verbosity** (`pkg/audit/store.go`): Timer tick reduced from INFO to V(1)

### 4.2 Features Not to be Tested

- **Prompt builder logic** (`remediation_history_prompt.py`): Already has 39 unit tests; no changes in this issue
- **LLM behavior with history context**: E2E validation of LLM workflow selection with history — deferred, requires full pipeline with real LLM
- **DS schema migration execution**: Migration deployment is operational, not code-testable
- **RO guardrail threshold tuning**: Thresholds are correct; the issue was data availability, not algorithm design

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Modify existing `QueryROEventsBySpecHash` rather than adding new method | Single method used by both Tier 1 and Tier 2; fix applies to both |
| Use SQL subquery for post-hash matching rather than two-round-trip approach | Single DB round-trip; PostgreSQL optimizer handles subquery efficiently with index |
| Test post-hash via real DB in integration tier, not unit tier | SQL query logic is inherently I/O; unit tests would only test mock behavior |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (logic in `remediation_history_logic.go`, Python enrichment wiring)
- **Integration**: >=80% of integration-testable code (SQL queries, handler flow, DB adapter)
- **E2E**: Existing 5 E2E tests provide regression gate; no new E2E tests for this fix

### 5.2 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. Per-tier coverage meets >=80% on modified files
3. All 47 existing DS unit tests, 8 DS integration tests, 5 DS E2E tests pass
4. All 59 existing HAPI Python tests pass
5. History API returns non-empty chain when `currentSpecHash` matches a `post_remediation_spec_hash`

**FAIL**:
1. Any P0 test fails
2. History API returns empty chain for post-hash match scenario
3. Existing tests regress

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/remediation_history_logic.go` | `CorrelateTier1Chain`, `ComputeHashMatch` | ~96 |
| `pkg/audit/store.go` | `backgroundWriter` (tick log line) | ~10 |
| `pkg/remediationorchestrator/routing/blocking.go` | `CheckIneffectiveRemediationChain` (nil/log paths) | ~30 |
| `kubernaut-agent/src/extensions/incident/llm_integration.py` | `history_fetcher` init block (lines 811-850) | ~40 |

### 6.2 Integration-Testable Code (I/O, DB, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/remediation_history_repository.go` | `QueryROEventsBySpecHash` (modified) | ~28 |
| `pkg/datastorage/server/remediation_history_handler.go` | `HandleGetRemediationHistoryContext` | ~170 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-016 | History API returns chain for post-hash match | P0 | Unit | UT-DS-616-001 | Pending |
| BR-HAPI-016 | History API returns chain for pre-hash match (regression gate) | P0 | Unit | UT-DS-616-002 | Pending |
| BR-HAPI-016 | History API returns entries from both pre-hash and post-hash match paths | P0 | Unit | UT-DS-616-003 | Pending |
| BR-HAPI-016 | History API returns empty when no hash matches | P0 | Unit | UT-DS-616-004 | Pending |
| BR-HAPI-016 | SQL query returns entries for post-hash match via EM correlation | P0 | Integration | IT-DS-616-001 | Pending |
| BR-HAPI-016 | SQL query returns entries for pre-hash match (existing behavior) | P0 | Integration | IT-DS-616-002 | Pending |
| BR-HAPI-016 | SQL query returns union of pre+post matches for different correlation_ids | P0 | Integration | IT-DS-616-003 | Pending |
| BR-HAPI-016 | Full handler flow returns non-empty tier1.chain for post-hash scenario | P0 | Integration | IT-DS-616-004 | Pending |
| BR-HAPI-016 | HAPI history_fetcher initializes with TypedDict config | P0 | Unit | UT-HAPI-616-001 | Pending |
| BR-HAPI-016 | HAPI history_fetcher creates working async closure | P0 | Unit | UT-HAPI-616-002 | Pending |
| BR-HAPI-016 | HAPI history_fetcher gracefully skips when DS not configured | P1 | Unit | UT-HAPI-616-003 | Pending |
| BR-HAPI-016 | RO logs V(1) Info when dsClient is nil | P1 | Unit | UT-RO-616-001 | Pending |
| BR-HAPI-016 | RO logs INFO with entry count after DS query | P1 | Unit | UT-RO-616-002 | Pending |
| BR-HAPI-016 | Audit timer tick at V(1) verbosity, not INFO | P1 | Unit | UT-AUDIT-616-001 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-616-{SEQUENCE}`

- **DS**: DataStorage service (Go)
- **HAPI**: HolmesGPT API (Python)
- **RO**: Remediation Orchestrator (Go)
- **AUDIT**: Shared audit store (Go)

### Tier 1: Unit Tests

#### DS Unit Tests

**File**: `test/unit/datastorage/remediation_history_query_fix_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-616-001` | CorrelateTier1Chain produces correct entries when RO events were found via post-hash EM correlation (hashMatch = postRemediation for the linking entry) | Pending |
| `UT-DS-616-002` | CorrelateTier1Chain still produces correct entries for pre-hash matches (regression gate — existing behavior preserved) | Pending |
| `UT-DS-616-003` | CorrelateTier1Chain produces correct entries when both pre-hash and post-hash match paths contribute RO events with different correlation_ids | Pending |
| `UT-DS-616-004` | CorrelateTier1Chain returns empty when no hash matches in either path | Pending |

#### HAPI Unit Tests

**File**: `kubernaut-agent/tests/unit/test_history_fetcher_init.py`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-616-001` | history_fetcher initializes successfully when app_config is a TypedDict (plain dict) — no AttributeError | Pending |
| `UT-HAPI-616-002` | history_fetcher creates a callable async closure that invokes query_remediation_history | Pending |
| `UT-HAPI-616-003` | history_fetcher is None when DATA_STORAGE_URL not in config (graceful skip) | Pending |

#### RO Observability Unit Tests

**File**: `test/unit/remediationorchestrator/routing/observability_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-616-001` | CheckIneffectiveRemediationChain logs V(1) Info when dsClient is nil (operator visibility under verbose logging) | Pending |
| `UT-RO-616-002` | CheckIneffectiveRemediationChain logs INFO with entry count and target after successful DS query | Pending |

#### Audit Unit Test

**File**: `test/unit/audit/timer_tick_verbosity_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AUDIT-616-001` | Audit store timer tick log is at V(1) verbosity, not plain INFO — operator logs survive >2h pod lifetime | Pending |

### Tier 2: Integration Tests

**File**: `test/integration/datastorage/remediation_history_query_fix_integration_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-616-001` | Insert RO event with pre_hash=A + EM event with post_hash=B for same correlation_id. Query with currentSpecHash=B returns the RO event (post-hash path) | Pending |
| `IT-DS-616-002` | Insert RO event with pre_hash=A. Query with currentSpecHash=A returns the RO event (pre-hash path — existing behavior) | Pending |
| `IT-DS-616-003` | Insert RO+EM events where pre_hash=A (rr-001) and post_hash=A (rr-002) for different correlation_ids. Query with currentSpecHash=A returns both entries | Pending |
| `IT-DS-616-004` | Full handler: insert complete audit chain (RO + all EM components), query via HTTP-like handler call, verify tier1.chain is non-empty with correct effectivenessScore and hashMatch | Pending |

### Tier Skip Rationale

- **E2E**: Existing 5 E2E tests in `test/e2e/datastorage/25_remediation_history_api_test.go` provide regression gate. The fix is in the SQL query layer; E2E tests exercise the full stack including this query. No new E2E tests needed for this specific fix.

---

## 9. Test Cases

### UT-DS-616-001: Post-hash match produces correct chain entries

**BR**: BR-HAPI-016
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/remediation_history_query_fix_test.go`

**Preconditions**:
- Mock RO events: 1 event with `pre_remediation_spec_hash=sha256:aaa`, `correlation_id=rr-001`
- Mock EM events: `effectiveness.hash.computed` with `post_remediation_spec_hash=sha256:bbb` for `correlation_id=rr-001`
- `currentSpecHash=sha256:bbb`

**Test Steps**:
1. **Given**: RO events found via post-hash EM correlation (simulating the expanded query result)
2. **When**: `CorrelateTier1Chain(roEvents, emEvents, "sha256:bbb")` is called
3. **Then**: Returns 1 entry with `hashMatch=postRemediation` and correct `effectivenessScore`

**Expected Results**:
1. Chain has exactly 1 entry
2. `entry.HashMatch` is `postRemediation` (currentHash matches post-hash)
3. `entry.RemediationUID` is `rr-001`
4. EM-sourced fields (effectivenessScore, signalResolved, healthChecks) are populated

### UT-HAPI-616-001: history_fetcher initializes with TypedDict config

**BR**: BR-HAPI-016
**Priority**: P0
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_history_fetcher_init.py`

**Preconditions**:
- `app_config` is a plain dict (TypedDict at runtime): `{"data_storage_url": "http://ds:8080", "service_name": "test"}`
- `create_remediation_history_api` is mocked to return a mock API instance

**Test Steps**:
1. **Given**: app_config is a TypedDict/dict (not a Pydantic model)
2. **When**: The history_fetcher init block executes (lines 811-850 of llm_integration.py)
3. **Then**: No AttributeError raised; `history_fetcher_fn` is not None

**Expected Results**:
1. `create_remediation_history_api` called with the dict directly (no `.to_dict()`)
2. `history_fetcher_fn` is a callable async function
3. No `history_fetcher_init_failed` log emitted

### IT-DS-616-001: SQL query returns entries for post-hash match

**BR**: BR-HAPI-016
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/remediation_history_query_fix_integration_test.go`

**Preconditions**:
- Real PostgreSQL with schema migrations applied
- Insert `remediation.workflow_created` event: `correlation_id=rr-001`, `pre_remediation_spec_hash=sha256:aaa`, `target_resource=default/Deployment/app`
- Insert `effectiveness.hash.computed` event: `correlation_id=rr-001`, `post_remediation_spec_hash=sha256:bbb`

**Test Steps**:
1. **Given**: Audit events exist where post-hash matches but pre-hash does not
2. **When**: `QueryROEventsBySpecHash(ctx, "sha256:bbb", since, until)` is called
3. **Then**: Returns 1 RO event with `correlation_id=rr-001`

**Expected Results**:
1. Query returns exactly 1 row
2. Row is the `remediation.workflow_created` event (not the EM event)
3. `correlation_id` is `rr-001`
4. `event_data` contains `pre_remediation_spec_hash=sha256:aaa`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (Go), pytest (Python)
- **Mocks**: Mock repository for handler tests; mock `create_remediation_history_api` for HAPI tests
- **Location**: `test/unit/datastorage/`, `test/unit/remediationorchestrator/routing/`, `test/unit/audit/`, `kubernaut-agent/tests/unit/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Real PostgreSQL (existing `suite_test.go` pattern in `test/integration/datastorage/`)
- **Location**: `test/integration/datastorage/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| PostgreSQL | 15+ | Integration test database |
| Python | 3.11+ | HAPI tests |
| pytest | 7.x | Python test runner |

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (RED)**: Write all failing unit tests (DS + HAPI + RO + Audit)
2. **Phase 2 (RED)**: Write all failing integration tests (DS)
3. **Phase 3 (GREEN)**: Implement DS query fix + new migration + HAPI one-line fix + observability changes
4. **Phase 4 (REFACTOR)**: Extract shared query builder if warranted; clean up logging format

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/616/TEST_PLAN.md` | Strategy and test design |
| DS unit tests | `test/unit/datastorage/remediation_history_query_fix_test.go` | 4 Ginkgo tests |
| HAPI unit tests | `kubernaut-agent/tests/unit/test_history_fetcher_init.py` | 3 pytest tests |
| RO observability tests | `test/unit/remediationorchestrator/routing/observability_test.go` | 2 Ginkgo tests |
| Audit verbosity test | `test/unit/audit/timer_tick_verbosity_test.go` | 1 Ginkgo test |
| DS integration tests | `test/integration/datastorage/remediation_history_query_fix_integration_test.go` | 4 Ginkgo tests |
| DB migration | `migrations/004_add_post_hash_index.sql` | Expression index on `post_remediation_spec_hash` (no CONCURRENTLY — goose runs in transactions) |

---

## 13. Execution

```bash
# DS unit tests
go test ./test/unit/datastorage/... -ginkgo.v -ginkgo.focus="616"

# DS integration tests
go test ./test/integration/datastorage/... -ginkgo.v -ginkgo.focus="616"

# RO observability tests
go test ./test/unit/remediationorchestrator/routing/... -ginkgo.v -ginkgo.focus="616"

# Audit verbosity test
go test ./test/unit/audit/... -ginkgo.v -ginkgo.focus="616"

# HAPI unit tests
cd kubernaut-agent && python -m pytest tests/unit/test_history_fetcher_init.py -v

# Full regression
go test ./test/unit/datastorage/... -ginkgo.v
go test ./test/integration/datastorage/... -ginkgo.v
cd kubernaut-agent && python -m pytest tests/ -v
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-RH-009..012 | `ExpectQuery` regex matched old single-table query | Updated regex prefix to `SELECT ... FROM` (not `FROM audit_events`) to match new subquery wrapper | Issue #616 changed QueryROEventsBySpecHash to use DISTINCT ON subquery |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-03 | Initial test plan |
| 1.1 | 2026-04-03 | Due diligence: UT-DS-616-003 phantom dedup replaced with union behavior; WARN to V(1) for dsClient nil; DEBUG to V(1) for audit tick; migration 004 without CONCURRENTLY; DS unit and Python tests reframed as regression gates |
| 1.2 | 2026-04-03 | Implementation: UT-AUDIT-616-001 removed (testing framework, not business logic); UT-RH-009..012 sqlmock regex updated; V(1) structured logging added to QueryROEventsBySpecHash |
