# Test Plan: Audit Event Retention Enforcement (#485)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-485-v1.0
**Feature**: Time-based audit event retention with per-event `retention_days` (default 2555 / ~7 years), `legal_hold` exemption, category policies via `audit_retention_policies`, structured logging in `retention_operations`, optional partition drops when enabled by Helm (opt-in, **disabled by default**)
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan validates Issue #485: enforcing audit data retention while respecting **legal hold** (`legal_hold=TRUE` blocks deletion via trigger `prevent_legal_hold_deletion`) and policy precedence (`audit_retention_policies` vs per-event values). The `retention_operations` table exists but foreign key relationships (e.g., to `action_histories`) may require schema change without backward-compatible constraint — human decision: **schema may be modified freely** to support retention logging. Efficient partition drops depend on Issues **#235/#620** (automated partition management). Retention worker is **disabled by default** (Helm flag) and **opt-in**.

### 1.2 Objectives

1. **Eligibility**: Rows are eligible for purge when `event_timestamp + retention_days < now` (UTC), absent legal hold and subject to policy merge rules.
2. **Legal hold**: Time-eligible rows with `legal_hold=true` are **never** deleted.
3. **Policy merge**: Defaults vs per-category / per-event overrides behave per BR-AUDIT-009.
4. **Safe default**: With flag **off**, **no** deletes occur (P0).
5. **Purge correctness**: Synthetic expired rows removed; mixed months delete only eligible subset.
6. **Partition path**: When all rows in a month partition are expired and none held, **DROP PARTITION** path works.
7. **Audit trail**: Each run writes structured records to `retention_operations`.
8. **Scheduling**: Short-interval behavior validated via injected clock/ticker in tests.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/...` (retention package) |
| Integration test pass rate | 100% | `go test ./test/integration/...` with real Postgres |
| Unit-testable code coverage | >=80% | Eligibility predicate, policy merge, scheduling math |
| Integration-testable code coverage | >=80% | Worker, SQL DELETE, partition drop, operation log |
| Safety | Zero deletes with flag off | IT-DS-485-001 |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-AUDIT-009: Retention policies for audit data
- BR-AUDIT-004: Immutability / integrity of audit records (holds, no silent loss)
- Issue #485: Audit event retention enforcement
- Issue #235 / #620: Partition creation (enables efficient partition drops)
- Schema: `retention_days`, `legal_hold`, `audit_retention_policies`, `retention_operations`, trigger `prevent_legal_hold_deletion`

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
| R1 | Accidental production purge | Data loss / legal exposure | High | IT-DS-485-001 | Default off; P0 proves no deletes |
| R2 | Legal hold bypass | Compliance violation | High | IT-DS-485-003 | Integration with trigger + app filter |
| R3 | Partition DROP too aggressive | Drops month with held rows | High | IT-DS-485-004, IT-DS-485-005 | Pre-check holds; mixed-month row delete |
| R4 | Clock skew | Wrong eligibility | Medium | UT-DS-485-001, IT-DS-485-007 | Injected clock; UTC |
| R5 | `retention_operations` FK blocks logging | Worker fails mid-run | Medium | IT-DS-485-006 | Schema change per human decision |

### 3.1 Risk-to-Test Traceability

- **R1**: IT-DS-485-001 (flag off).
- **R2**: IT-DS-485-003 + trigger interaction.
- **R3**: IT-DS-485-004 / IT-DS-485-005.
- **R5**: IT-DS-485-006 asserts successful inserts per run.

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Eligibility predicate** (pure): timestamp + retention vs now; legal hold; policy merge.
- **Configurable worker**: Helm flag gating; ticker/interval; batch DELETE SQL with hold filter.
- **Partition drop path**: When #235/#620 partitions exist and month fully eligible.
- **retention_operations**: Structured operation log each run.
- **Legal hold trigger**: DELETE blocked at DB layer when `legal_hold=TRUE`.

### 4.2 Features Not to be Tested

- **Non-audit tables**: Unless BR explicitly extends scope.
- **Cross-region replication lag**: Operational concern; not simulated here.
- **E2E full Helm lifecycle**: Covered by integration + chart values unit tests if present; full cluster E2E optional/deferred.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Disabled by default | Safety / BR-AUDIT-004 alignment |
| Schema changes allowed for `retention_operations` | Human decision; unblock logging |
| Depend on #235/#620 | Partition drop efficiency |
| Repository + metrics in REFACTOR | Clean boundaries after GREEN |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% eligibility, policy merge, schedule helpers.
- **Integration**: >=80% worker, SQL, partition ops, logging — real Postgres, mock time via injected clock in process.

### 5.2 Two-Tier Minimum

BR-AUDIT-009 and BR-AUDIT-004 covered by **UT** (rules) + **IT** (DB + trigger + worker).

### 5.3 Business Outcome Quality Bar

Validate: “No silent data loss; holds honored; retention runs auditable; opt-in only.”

### 5.4 Pass/Fail Criteria

**PASS** — all of:

1. P0 integration tests pass: IT-DS-485-001, IT-DS-485-002, IT-DS-485-003.
2. Unit tests UT-DS-485-001–003 pass.
3. Coverage >=80% on retention modules.
4. `retention_operations` receives expected rows in IT-DS-485-006.

**FAIL** — any of:

1. Deletes occur with flag off.
2. Held row deleted or partition dropped while held rows exist.
3. Eligibility unit tests disagree with SQL filter semantics.

### 5.5 Suspension & Resumption Criteria

**Suspend**: #235 not landed when implementing partition DROP IT; Postgres down; schema migration conflict.
**Resume**: Partitions available; migrations applied; build green.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| TBD `pkg/datastorage/.../retention*.go` | `IsEligibleForPurge`, policy merge | TBD |
| TBD | Schedule window calculator (with injected clock) | TBD |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| TBD worker | Ticker loop, batch delete, partition drop | TBD |
| `cmd/data-storage/main.go` (or worker cmd) | Flag wiring, start/stop | TBD |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Schema | Migrations HEAD | `retention_operations` FK may change |
| Partitions | #235/#620 | Required for IT-DS-485-004 |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-009 | Retention policies / eligibility | P0 | Unit | UT-DS-485-001, UT-DS-485-002, UT-DS-485-003 | Pending |
| BR-AUDIT-004 | Integrity / no improper deletion | P0 | Integration | IT-DS-485-001, IT-DS-485-003 | Pending |
| BR-AUDIT-009 | Expired row removal | P0 | Integration | IT-DS-485-002 | Pending |
| BR-AUDIT-009 | Partition efficiency | P1 | Integration | IT-DS-485-004 | Pending |
| BR-AUDIT-004 | Mixed eligibility | P1 | Integration | IT-DS-485-005 | Pending |
| BR-AUDIT-009 | Retention operations log | P1 | Integration | IT-DS-485-006 | Pending |
| BR-AUDIT-009 | Scheduled runs | P1 | Integration | IT-DS-485-007 | Pending |

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

**Testable code scope**: Eligibility predicate, legal hold exemption in logic layer, policy precedence — >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-485-001` | `event_timestamp + retention_days < now` ⇒ eligible (baseline) | Pending |
| `UT-DS-485-002` | Time-eligible but `legal_hold=true` ⇒ **not** eligible | Pending |
| `UT-DS-485-003` | Category policy merge: default vs per-event override precedence | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Worker, SQL, partitions, `retention_operations`, Helm flag — >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-485-001` (P0) | Flag off ⇒ **no** deletes | Pending |
| `IT-DS-485-002` (P0) | Synthetic old event, `legal_hold=false` ⇒ row removed | Pending |
| `IT-DS-485-003` (P0) | Expired + `legal_hold=true` ⇒ row remains | Pending |
| `IT-DS-485-004` | Partition drop: month fully expired, no holds ⇒ DROP partition | Pending |
| `IT-DS-485-005` | Mixed month: DELETE eligible only | Pending |
| `IT-DS-485-006` | Each run writes structured record to `retention_operations` | Pending |
| `IT-DS-485-007` | Short interval via injected clock/ticker | Pending |

### Tier 3: E2E Tests (if applicable)

Not required for core retention semantics; integration with Postgres is sufficient.

### Tier Skip Rationale (if any tier is omitted)

- **E2E**: Opt-in Helm flag and worker behavior validated via IT + config unit tests.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-DS-485-001: Eligibility predicate — time expired

**BR**: BR-AUDIT-009
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/...`

**Preconditions**:
- Fixed `now` from clock interface.

**Test Steps**:
1. **Given**: `event_timestamp` and `retention_days` such that `event_timestamp + retention_days < now`, `legal_hold=false`, default policy.
2. **When**: Predicate evaluated.
3. **Then**: Eligible == true.

**Expected Results**:
1. Matches SQL `WHERE` filter semantics used by worker.

**Acceptance Criteria**:
- **Correctness**: Aligns with DELETE statement boundary (strict vs non-strict per design).

**Dependencies**: None.

---

### UT-DS-485-002: Legal hold exemption

**BR**: BR-AUDIT-009 / BR-AUDIT-004
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/...`

**Test Steps**:
1. **Given**: Time-eligible row attributes with `legal_hold=true`.
2. **When**: Predicate evaluated.
3. **Then**: Eligible == false.

**Expected Results**:
1. Predicate excludes row even if timestamp old.

**Acceptance Criteria**:
- **Behavior**: Hold always wins in application layer (defense in depth with DB trigger).

**Dependencies**: None.

---

### UT-DS-485-003: Category policy merge

**BR**: BR-AUDIT-009
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/...`

**Test Steps**:
1. **Given**: `audit_retention_policies` default retention X; per-event or category override Y.
2. **When**: Effective retention computed.
3. **Then**: Precedence matches BR (document expected: override wins or default wins — **implement per BR text**).

**Expected Results**:
1. Table-driven cases for default-only, override-only, conflict.

**Acceptance Criteria**:
- **Accuracy**: Effective retention matches product rules.

**Dependencies**: BR-AUDIT-009 exact precedence table.

---

### IT-DS-485-001 (P0): Disabled by default

**BR**: BR-AUDIT-004
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Preconditions**:
- Config/Helm values: retention worker **disabled**; DB contains synthetic expired rows.

**Test Steps**:
1. **Given**: Worker started with flag off (or not started).
2. **When**: Observation window elapses / tick fired in no-op mode.
3. **Then**: Row counts unchanged; `retention_operations` has no new run rows (or worker never connects).

**Expected Results**:
1. Zero DELETEs executed.

**Acceptance Criteria**:
- **Safety**: Opt-in contract honored.

**Dependencies**: Config wiring.

---

### IT-DS-485-002 (P0): Expired row delete

**BR**: BR-AUDIT-009
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Preconditions**:
- Flag **on**; synthetic row with old `event_timestamp`, `legal_hold=false`.

**Test Steps**:
1. **Given**: Row inserted and visible.
2. **When**: Retention worker runs one cycle with `now` injected forward.
3. **Then**: Row no longer present; other non-eligible rows remain.

**Expected Results**:
1. `SELECT` returns 0 for primary key.

**Acceptance Criteria**:
- **Behavior**: Purge works end-to-end.

**Dependencies**: IT-DS-485-001 inverse (flag on).

---

### IT-DS-485-003 (P0): Legal hold — row remains

**BR**: BR-AUDIT-004
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Preconditions**:
- Expired row with `legal_hold=true`; trigger `prevent_legal_hold_deletion` installed.

**Test Steps**:
1. **Given**: Eligible by time only.
2. **When**: Worker attempts delete (or direct SQL mirrors worker).
3. **Then**: Row still exists; DB may raise on direct delete — app must not remove held rows.

**Expected Results**:
1. Count unchanged; optional assert error on naive DELETE (if test uses direct SQL).

**Acceptance Criteria**:
- **Integrity**: Hold enforced.

**Dependencies**: Schema trigger.

---

### IT-DS-485-004: Partition drop path

**BR**: BR-AUDIT-009
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Preconditions**:
- #235/#620 partitions exist; target month partition contains only eligible rows, none held.

**Test Steps**:
1. **Given**: Month partition ready for drop per implementation rules.
2. **When**: Retention run executes partition drop branch.
3. **Then**: Partition detached/dropped; parent remains healthy.

**Expected Results**:
1. `pg_inherits` / catalog reflects removal.

**Acceptance Criteria**:
- **Behavior**: Efficient reclaim path works.

**Dependencies**: Issues #235 / #620.

---

### IT-DS-485-005: Mixed month

**BR**: BR-AUDIT-004
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Test Steps**:
1. **Given**: Same month partition with mix of eligible and held (or ineligible) rows.
2. **When**: Row-level DELETE phase runs.
3. **Then**: Only eligible non-held rows removed; partition not dropped if any row remains.

**Expected Results**:
1. Counts per category match expected.

**Acceptance Criteria**:
- **Integrity**: No over-deletion.

**Dependencies**: IT-DS-485-002/003 patterns.

---

### IT-DS-485-006: Operation log

**BR**: BR-AUDIT-009
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Test Steps**:
1. **Given**: Worker enabled.
2. **When**: One or more retention cycles complete.
3. **Then**: `retention_operations` contains structured records (counts, window, status, timestamps).

**Expected Results**:
1. At least one row per run with required columns populated.

**Acceptance Criteria**:
- **Auditability**: Ops can trace what the worker did.

**Dependencies**: Schema for `retention_operations` finalized.

---

### IT-DS-485-007: Schedule — injected clock/ticker

**BR**: BR-AUDIT-009
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/...`

**Test Steps**:
1. **Given**: Short ticker interval or manual tick channel; clock advanced between ticks.
2. **When**: Multiple cycles executed in test harness.
3. **Then**: Eligibility changes with mocked time; worker respects interval without waiting wall-clock duration.

**Expected Results**:
1. Deterministic multi-cycle behavior in <1s wall time.

**Acceptance Criteria**:
- **Behavior**: Testable scheduling.

**Dependencies**: Injectable `time.Ticker` / clock abstraction.

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Clock only
- **Location**: `test/unit/datastorage/` (TBD)
- **Resources**: Minimal

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for Postgres; **mock time** via injected clock in application code (not a DB mock)
- **Infrastructure**: Real PostgreSQL; migrations including triggers and policies
- **Location**: `test/integration/datastorage/`
- **Resources**: CI Postgres service

### 10.3 E2E Tests (if applicable)

- Not required for this plan’s core.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Project standard | Build and test |
| Ginkgo CLI | v2.x | BDD runner |
| PostgreSQL | Compatible with audit schema | DELETE, triggers, PARTITION |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| #235 / #620 | Partitions | Open/Merged | IT-DS-485-004 blocked | Row-only deletes until partitions land |
| Schema migration | DB | Required | IT-DS-485-006 blocked | Land migration in same PR series |

### 11.2 Execution Order

1. **RED**: Integration tests with real Postgres + mock clock — fail until worker + SQL exist.
2. **GREEN**: Config flag (default off); worker with ticker; SQL DELETE with hold filter; basic logging.
3. **REFACTOR**: Repository layer, metrics, transaction boundaries.

**TDD sequence**: RED (IT fails without retention logic) → GREEN (flag, worker, SQL) → REFACTOR (repository, metrics, transactions).

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/485/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `test/unit/datastorage/` | Eligibility + policy merge |
| Integration tests | `test/integration/datastorage/` | Worker, SQL, partitions, logs |
| Coverage report | CI artifact | Per-tier coverage |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/datastorage/... -ginkgo.v

# Integration tests
go test ./test/integration/datastorage/... -ginkgo.v

# Focus retention IDs
go test ./test/integration/datastorage/... -ginkgo.focus="IT-DS-485" -ginkgo.v

# Coverage
go test ./test/unit/datastorage/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| Helm `values.yaml` / schema | No retention flag | Add opt-in flag default `false` | IT-DS-485-001 |
| Migrations | `retention_operations` FK | Adjust per #485 schema decision | IT-DS-485-006 |
| DS integration suite | No worker | Start worker in tests when flag on | New behavior |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #485 |
