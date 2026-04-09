# Test Plan: Automated Partition Creation — audit_events & resource_action_traces (#235 + #620)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-235-v1.0
**Feature**: Application-level startup enforcement of PostgreSQL monthly partitions for `audit_events` (by `event_date`) and `resource_action_traces` (by `action_timestamp`), with 3-month lookahead in UTC (Option A)
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan validates Issue #235 (with #620 alignment): today static partitions exist through 2028-12 in `pkg/shared/assets/migrations/001_v1_schema.sql`, while `create_monthly_partitions()` only targets `resource_action_traces` (RAT), rolls forward one month, and is never invoked. The human decision is **Option A** — ensure partitions at **application startup**, **3-month lookahead**, **UTC**. Integration test helper `createDynamicPartitions` currently covers only RAT; both tables use `_default` catch-all partitions.

### 1.2 Objectives

1. **Month range calculator**: Given a clock “today” and **N** months lookahead, compute the correct list of partition identities for **both** tables.
2. **Naming**: Partitions use **`YYYY_MM`** style naming with correct table-specific prefix conventions.
3. **Idempotency**: Repeated `ensure` calls do not error and do not create duplicate relations.
4. **Startup behavior**: On service start, missing partitions for **current month + 3 months** exist for **both** `audit_events` and `resource_action_traces`.
5. **Data path**: Inserts with timestamps falling in newly ensured partitions succeed (boundary validation).
6. **Catalog verification**: `pg_inherits` shows child partitions attached for **both** parent tables.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/...` (data-storage / partition package paths TBD) |
| Integration test pass rate | 100% | `go test ./test/integration/...` (data-storage + real Postgres) |
| Unit-testable code coverage | >=80% | Month calculator, naming, idempotent SQL generation |
| Integration-testable code coverage | >=80% | Startup hook, DB catalog checks |
| Regressions | 0 | Existing RAT-only helpers upgraded without breaking callers |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-AUDIT-029: Automatic partition management for audit storage
- Issue #235: Automated partition creation for `audit_events` and `resource_action_traces`
- Issue #620: Related partition / data-storage alignment (coordinated delivery)
- `pkg/shared/assets/migrations/001_v1_schema.sql`: Static partitions through 2028-12; `create_monthly_partitions()` definition

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | UTC vs local timezone in partition bounds | Wrong month boundary, data lands in `_default` | Medium | UT-DS-235-001, IT-DS-235-003 | Fix clock to UTC; tests use injected clock |
| R2 | `IF NOT EXISTS` omitted | Startup flaky or duplicate object errors | Medium | UT-DS-235-003, IT-DS-235-002 | Assert idempotent DDL; integration double-start |
| R3 | Only RAT ensured, audit_events forgotten | Audit writes fail after static range | High | IT-DS-235-001, IT-DS-235-004 | Explicit both-table IT coverage |
| R4 | Migration function `create_monthly_partitions` left as dead code | Confusion / dual paths | Low | Documentation + single ensure path in app | REFACTOR: one PartitionManager |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-DS-235-001 uses fixed instants in UTC; IT-DS-235-003 inserts boundary timestamp.
- **R3**: IT-DS-235-001 and IT-DS-235-004 require both parents.
- **R2**: UT-DS-235-003 and IT-DS-235-002 exercise double ensure.

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Partition month range calculator** (pure logic): Given today + N months, yields partition keys/names for both strategies (`event_date` / `action_timestamp` parents).
- **DDL ensure loop** at **data-storage** (or shared DB bootstrap) **startup**: `CREATE TABLE ... PARTITION OF ... FOR VALUES FROM ... TO ...` with **IF NOT EXISTS** (or equivalent idempotent pattern).
- **PostgreSQL catalog**: `pg_inherits` / information_schema checks for children of `audit_events` and `resource_action_traces`.
- **Insert boundary**: Rows routed to new partitions, not only `_default`.

### 4.2 Features Not to be Tested

- **Dropping** old partitions: Covered by Issue #485 test plan; #235 focuses on **creation**.
- **Non-Postgres** backends: Out of scope.
- **Retroactive backfill** of historical partitions beyond lookahead: Out of scope unless BR extends.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Option A — application startup | No external cron required; deterministic with service lifecycle |
| 3-month lookahead | Balances DDL churn vs operational safety |
| UTC | Consistent with partition definitions in migrations |
| Both tables in one ensure | BR-AUDIT-029; avoids RAT-only gap |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of month calculator, naming, and idempotent ensure planning logic.
- **Integration**: >=80% of startup wiring against real PostgreSQL (envtest or testcontainer pattern per existing DS tests).
- **E2E**: Not required for partition DDL; integration with real Postgres is sufficient.

### 5.2 Two-Tier Minimum

Each BR-aligned behavior is covered by **UT** (pure date math / naming) and **IT** (actual catalog + inserts).

### 5.3 Business Outcome Quality Bar

Prove: “Service starts cleanly; both audit and RAT partitions exist for now + 3 months; writes hit correct partitions; reruns are safe.”

### 5.4 Pass/Fail Criteria

**PASS** — all of:

1. All P0 tests pass: IT-DS-235-001, IT-DS-235-002, IT-DS-235-003, IT-DS-235-004, plus unit suite.
2. Coverage >=80% on new partition manager code.
3. No duplicate partition relations after double startup.

**FAIL** — any of:

1. Any P0 integration test fails.
2. Only one table gets partitions in IT-DS-235-004.
3. Second startup errors or creates duplicate inherit entries.

### 5.5 Suspension & Resumption Criteria

**Suspend**: Postgres unavailable; migration SQL conflict on branch; shared integration suite broken.
**Resume**: DS integration harness green; migrations applied cleanly.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| TBD `pkg/datastorage/.../partition*.go` | Month range calculator, partition name builder | TBD |
| TBD | `EnsurePartitions` plan builder (SQL statements, idempotent flags) | TBD |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/data-storage/main.go` (or bootstrap) | Startup hook invoking partition ensure | TBD |
| TBD | `PartitionManager.Ensure` executing against `sql.DB` | TBD |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Schema | `001_v1_schema.sql` HEAD | Static partitions through 2028-12; `_default` partitions |
| SQL function | `create_monthly_partitions()` | Pre-change: RAT-only, +1 month, uncalled |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-029 | Automatic partition management | P0 | Unit | UT-DS-235-001, UT-DS-235-002, UT-DS-235-003 | Pending |
| BR-AUDIT-029 | Startup ensure both tables | P0 | Integration | IT-DS-235-001 | Pending |
| BR-AUDIT-029 | Idempotent ensure | P1 | Integration | IT-DS-235-002 | Pending |
| BR-AUDIT-029 | Insert boundary | P1 | Integration | IT-DS-235-003 | Pending |
| BR-AUDIT-029 | Catalog inherits both parents | P0 | Integration | IT-DS-235-004 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-DS-{ISSUE}-{SEQUENCE}`.

### Tier 1: Unit Tests

**Testable code scope**: Month calculator, `YYYY_MM` naming, table prefix rules, idempotent ensure planning — >=80% coverage.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-235-001` | Given fixed “today” and N months, partition name list correct for **both** tables | Pending |
| `UT-DS-235-002` | Naming: `YYYY_MM` format and correct parent/table prefix in generated identifiers | Pending |
| `UT-DS-235-003` | Idempotency at plan/SQL level: second ensure generates no conflicting operations / safe no-ops | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Data-storage startup wiring, real Postgres, `pg_inherits` — >=80% coverage on integration-testable partition code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-235-001` (P0) | Startup ensure creates missing partitions for current + 3 months for **both** tables | Pending |
| `IT-DS-235-002` | Second startup: no error, no duplicate child relations | Pending |
| `IT-DS-235-003` | Boundary insert with timestamp in newly created partition succeeds | Pending |
| `IT-DS-235-004` | `pg_inherits` lists children for **audit_events** and **resource_action_traces** | Pending |

### Tier 3: E2E Tests (if applicable)

Not applicable.

### Tier Skip Rationale (if any tier is omitted)

- **E2E**: Real Postgres in integration tests satisfies storage contract; full Helm deploy deferred.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-DS-235-001: Month range calculator (both tables)

**BR**: BR-AUDIT-029
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/...` (TBD path aligned with implementation package)

**Preconditions**:
- Injected clock interface returns a known UTC instant (e.g., mid-month).

**Test Steps**:
1. **Given**: Today = T0, lookahead N = 3.
2. **When**: Calculator returns planned partitions for `audit_events` and `resource_action_traces`.
3. **Then**: Lists include months M0..M3 (inclusive framing per implementation spec) with correct year/month roll.

**Expected Results**:
1. Both tables receive the same calendar month sequence (partitioning strategy may differ only in parent name/prefix, not month set).

**Acceptance Criteria**:
- **Behavior**: Deterministic planning from clock.
- **Correctness**: Expected month labels match UTC calendar math.

**Dependencies**: None.

---

### UT-DS-235-002: Partition naming

**BR**: BR-AUDIT-029
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/...`

**Test Steps**:
1. **Given**: A calendar month (e.g., 2026-11).
2. **When**: Name builder runs for each table kind.
3. **Then**: String matches `YYYY_MM` pattern and includes correct table-specific prefix/suffix per schema conventions.

**Expected Results**:
1. Regex/table naming rules satisfied; no illegal identifiers.

**Acceptance Criteria**:
- **Accuracy**: Names align with migration naming for existing partitions.

**Dependencies**: `001_v1_schema.sql` naming audit.

---

### UT-DS-235-003: Idempotency (plan / SQL)

**BR**: BR-AUDIT-029
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/...`

**Test Steps**:
1. **Given**: Planned DDL statements for ensure.
2. **When**: `Ensure` called twice with same inputs.
3. **Then**: Second invocation produces only safe no-ops or uses `IF NOT EXISTS` such that executor would not error.

**Expected Results**:
1. No conflicting `CREATE` without guard; duplicate call flag or empty diff.

**Acceptance Criteria**:
- **Correctness**: Mathematical idempotence of ensure logic.

**Dependencies**: None.

---

### IT-DS-235-001 (P0): Startup ensure — both tables

**BR**: BR-AUDIT-029
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/...` (TBD)

**Preconditions**:
- Real Postgres (existing suite pattern); database migrated with base schema; optional teardown of future month partitions beyond test window to force creation.

**Test Steps**:
1. **Given**: Clean DB missing partitions for months in [now, now+3] for one or both parents.
2. **When**: Run startup ensure (same code path as service boot).
3. **Then**: Required child partitions exist for **audit_events** and **resource_action_traces**.

**Expected Results**:
1. Query against `pg_inherits` or catalog helper confirms expected child count/names.

**Acceptance Criteria**:
- **Behavior**: Both parents extended together.

**Dependencies**: #620 coordination if shared bootstrap changes.

---

### IT-DS-235-002: Idempotency — second startup

**BR**: BR-AUDIT-029
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Test Steps**:
1. **Given**: DB after successful IT-DS-235-001.
2. **When**: Startup ensure runs again.
3. **Then**: No error; no duplicate inheritance entries for same partition name.

**Expected Results**:
1. Logs structured (optional); SQL errors absent.

**Acceptance Criteria**:
- **Behavior**: Safe restart.

**Dependencies**: IT-DS-235-001.

---

### IT-DS-235-003: Boundary insert

**BR**: BR-AUDIT-029
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Test Steps**:
1. **Given**: Partition for target month ensured.
2. **When**: Insert `audit_events` row with `event_date` / RAT row with `action_timestamp` at boundary instant inside that partition’s range.
3. **Then**: Insert succeeds; row visible via `SELECT`; not relegated to `_default` (assert partition name via `tableoid` or explain plan if available).

**Expected Results**:
1. Successful commit; correct routing.

**Acceptance Criteria**:
- **Data accuracy**: Writes land in child partition.

**Dependencies**: IT-DS-235-001.

---

### IT-DS-235-004: pg_inherits — both tables

**BR**: BR-AUDIT-029
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Test Steps**:
1. **Given**: Post-ensure database.
2. **When**: Query `pg_inherits` joined to `pg_class` for each parent.
3. **Then**: Non-empty child set for **both** `audit_events` and `resource_action_traces` covering lookahead window.

**Expected Results**:
1. Each parent lists expected monthly children.

**Acceptance Criteria**:
- **Correctness**: Catalog matches plan.

**Dependencies**: IT-DS-235-001.

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Clock interface only (not external DB)
- **Location**: `test/unit/datastorage/` (or package under test)
- **Resources**: Minimal

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for Postgres (per no-mocks policy)
- **Infrastructure**: Real PostgreSQL — same harness as existing data-storage integration tests
- **Location**: `test/integration/datastorage/`
- **Resources**: Docker/Podman or CI-provided Postgres service

### 10.3 E2E Tests (if applicable)

- Not applicable.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Project standard | Build and test |
| Ginkgo CLI | v2.x | BDD runner |
| PostgreSQL | Schema-compatible with `001_v1_schema.sql` | Partition DDL |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #620 | Code | Open/Merged TBD | Bootstrap location may shift | Rebase IT paths |
| Existing `createDynamicPartitions` | Test helper | RAT-only | IT cannot validate audit_events | Extend helper per IT cases |

### 11.2 Execution Order

1. **RED**: Integration tests fail when DB lacks future partitions for **audit_events** (and assert RAT behavior preserved).
2. **GREEN**: Startup hook; ensure loop for **both** tables; `IF NOT EXISTS` (or equivalent).
3. **REFACTOR**: Extract `PartitionManager` with clock interface; structured logs.

**TDD sequence**: RED (integration fails without partitions) → GREEN (startup ensure both) → REFACTOR (PartitionManager, clock, logs).

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/235/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `test/unit/datastorage/` (TBD) | Calculator / naming / idempotence |
| Integration tests | `test/integration/datastorage/` | Startup + catalog + insert |
| Coverage report | CI artifact | Per-tier coverage |

---

## 13. Execution

```bash
# Unit tests (adjust package path when implemented)
go test ./test/unit/datastorage/... -ginkgo.v

# Integration tests
go test ./test/integration/datastorage/... -ginkgo.v

# Focus by ID
go test ./test/integration/datastorage/... -ginkgo.focus="IT-DS-235" -ginkgo.v

# Coverage
go test ./test/unit/datastorage/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| Integration helper `createDynamicPartitions` | RAT-only | Ensure **audit_events** + RAT; align with 3-month UTC lookahead | BR-AUDIT-029 |
| Any startup test assuming static partitions only | No ensure call | Invoke or assert new startup ensure | Option A |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issues #235 / #620 |
