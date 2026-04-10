# Test Plan: Effectiveness Monitor — Reconciler Decomposition (Maintainability)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-254-v1
**Feature**: Pure structural refactor — split `internal/controller/effectivenessmonitor/reconciler.go` (~1868 lines, `Reconcile` ~527 lines) into focused files (<500 lines each) without behavior change
**Version**: 1.0
**Created**: 2026-04-09
**Author**: Kubernaut Engineering
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

Issue #254 reduces risk and maintenance cost of the Effectiveness Monitor controller by decomposing an oversized reconciler into cohesive modules while **preserving identical observable behavior**: status fields, `ctrl.Result`, requeue timing, conditions, events, audit emissions, and metrics. This test plan defines **golden snapshots** and **invariant tests** that catch accidental semantic drift during extraction, and establishes **existing suites as the regression gate** (35 unit + 19 integration files unchanged).

### 1.2 Objectives

1. **Zero behavior drift**: Pre-refactor vs post-refactor `Reconcile` outcomes match for representative paths (WFP, Stabilizing, spec drift, partial scope grace).
2. **Invariant preservation**: Exactly **one** `Status().Update` per happy-path reconcile where that invariant holds today; alert deferral requeue remains capped as today.
3. **File-size compliance**: Target layout of 12 files, each under 500 lines (excluding generated code).
4. **Regression gate**: All **35** existing unit test files and **19** integration test files pass **without modification** (pure refactor discipline).
5. **Coverage policy**: Per-tier >=80%; primary evidence from existing tests; new tests target extracted helpers and golden paths.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/effectivenessmonitor/...` (+ any new UT files) |
| Integration test pass rate | 100% | `go test ./test/integration/effectivenessmonitor/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on EM controller packages |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable EM code |
| Existing test modification count | **0** files changed | `git diff --name-only test/` empty for unplanned edits |
| Golden snapshot stability | Pass after REFACTOR | UT-EM-254-001–002 byte-stable or knowingly updated once with review |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-EM-001 through BR-EM-012: Effectiveness Monitor business requirements (scope per BR library)
- BR-AUDIT-006: Audit event semantics for EM (as applicable to `emitAuditEvent` and related paths)
- Issue #254: Decompose EM reconciler from ~1.9k lines

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
| R1 | Status update ordering change | Flapping conditions, duplicate events, etcd write amplification | High | UT-EM-254-003, regression IT suite | Single orchestrated `reconcile_status.go`; assert one Update on happy path |
| R2 | Shared local flags diverge (`componentsChanged`, `pendingTransition`, `alertDeferred`, `scope`) | Subtle behavior change across phases | High | UT-EM-254-001–006, all IT | Introduce `reconcileContext` struct in `reconcile_orchestrate.go`; golden tests |
| R3 | Duplicate `ComputeDerivedTiming` blocks | Inconsistent requeue intervals | Medium | UT-EM-254-004 | REFACTOR consolidates timing; deferral cap tested explicitly |
| R4 | Event / audit emission order change | Compliance or operator confusion | Medium | UT-EM-254-001–002, BR-AUDIT-006 mapping | Snapshot includes event list ordering where stable |
| R5 | “Pure refactor” scope creep | New behavior slips in | Medium | Regression gate | Code review + zero existing test file edits policy |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-EM-254-003 + full integration regression
- **R2**: UT-EM-254-001–006 + `reconcileContext` code review checklist
- **R3**: UT-EM-254-004
- **R4**: UT-EM-254-001–002 (events in snapshot), BR-AUDIT-006 row in Section 7

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Decomposed EM reconciler** (`internal/controller/effectivenessmonitor/`):
  1. `reconciler.go` — core type, DI, thin `Reconcile`
  2. `reconcile_orchestrate.go` — `reconcileContext` + main flow
  3. `reconcile_validity_phase.go` — WFP / Stabilizing / Assessing transitions
  4. `reconcile_spec_drift.go` — hash mismatch / `AssessmentReasonSpecDrift`
  5. `reconcile_components.go` — hash / health / alert / metrics pipeline
  6. `reconcile_status.go` — atomic `Status().Update` + post-update events
  7. `assess_components.go` — assessHealth / Hash / Alert / Metrics
  8. `target_resources.go` — `getTargetSpec`, health status, ConfigMap hashes
  9. `pod_health.go` — `FilterActivePods`, `ComputePodHealthStats` (**remain exported**)
  10. `completion.go` — complete/fail assessment, reason mapping
  11. `events.go` — all `emit*` + `emitAuditEvent`
  12. `scope.go` — `assessmentScope`, `determineAssessmentScope`, `validateEASpec`

### 4.2 Features Not to be Tested

- **Functional EM feature additions**: No new BR scope beyond decomposition
- **Cross-controller refactors**: WE, RO, DS out of scope
- **Performance benchmarking**: Unless regression detected by existing metrics tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| 12 files, <500 lines each | Meets readability goals; aligns with single responsibility |
| `reconcileContext` for shared state | Prevents accidental parameter drift vs many locals |
| Golden snapshots before GREEN | RED captures baseline; failures during extract signal regression |
| No edits to existing 35+19 test files | Enforces “pure refactor” contract |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable EM controller logic; **existing 35 files** are primary; new golden/helper tests add targeted coverage
- **Integration**: >=80% of integration-testable EM surfaces; **existing 19 files** unchanged
- **E2E**: Not expanded for #254; integration + unit provide sufficient safety for internal split

### 5.2 Two-Tier Minimum

- **Unit**: Golden snapshots + invariants (status update count, requeue cap)
- **Integration**: Full existing suite as regression gate (no mocks policy unchanged)

### 5.3 Business Outcome Quality Bar

Operators and auditors observe **the same** assessment progression, conditions, deferral behavior, and audit trail as before decomposition.

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. UT-EM-254-001 through UT-EM-254-006 pass
2. **All** existing EM unit + integration tests pass with **zero** test file modifications (except **new** files added for RED)
3. Per-tier coverage >=80% on scoped packages
4. No new linter suppressions without justification

**FAIL** — any of the following:

1. Any golden snapshot mismatch without reviewed baseline update
2. More than one `Status().Update` on happy path where UT-EM-254-003 asserts one
3. Any pre-existing test file edited to “make refactor pass” (violates pure refactor rule)
4. Coverage drop below 80% on any tier for EM scope

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- `main` / target branch does not compile
- Baseline snapshots not captured (RED incomplete)
- Widespread flaky IT (>3 unrelated flakes in one run)

**Resume testing when**:

- Build green
- Baseline stored in repo with review
- Infra stable

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/controller/effectivenessmonitor/reconcile_validity_phase.go` | Phase transition helpers | <500 |
| `internal/controller/effectivenessmonitor/reconcile_spec_drift.go` | Spec drift detection | <500 |
| `internal/controller/effectivenessmonitor/assess_components.go` | Assessment helpers | <500 |
| `internal/controller/effectivenessmonitor/pod_health.go` | `FilterActivePods`, `ComputePodHealthStats` | <500 |
| `internal/controller/effectivenessmonitor/completion.go` | Completion / fail mapping | <500 |
| `internal/controller/effectivenessmonitor/scope.go` | Scope validation | <500 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | Thin `Reconcile` entry | <500 |
| `internal/controller/effectivenessmonitor/reconcile_orchestrate.go` | Main orchestration | <500 |
| `internal/controller/effectivenessmonitor/reconcile_components.go` | Component pipeline | <500 |
| `internal/controller/effectivenessmonitor/reconcile_status.go` | Status update + events | <500 |
| `internal/controller/effectivenessmonitor/target_resources.go` | Target resolution | <500 |
| `internal/controller/effectivenessmonitor/events.go` | Event emission | <500 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | #254 branch |
| Pre-refactor baseline | Git tag or commit hash | Record when snapshots taken |
| Existing tests | 35 UT + 19 IT files | Enumeration in CI job optional |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps business requirements to test scenarios.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-001–012 | EM assessment lifecycle & components | P0 | Unit | UT-EM-254-001–006 | Pending |
| BR-EM-001–012 | EM assessment lifecycle & components | P0 | Integration | Regression: 19 IT files | Pending |
| BR-AUDIT-006 | Audit events | P0 | Unit | UT-EM-254-001–002 (events/audit in snapshot) | Pending |
| BR-AUDIT-006 | Audit events | P1 | Integration | Regression IT suite | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTOR**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` — **EM** service, Issue **254**.

### Tier 1: Unit Tests

**Testable code scope**: EM controller packages post-split — >=80% merged with existing coverage.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-254-001` | Golden snapshot — **WFP** path: status + `Result` + events match baseline | Pending |
| `UT-EM-254-002` | Golden snapshot — **Stabilizing** path preserved | Pending |
| `UT-EM-254-003` | **One** `Status().Update` per happy-path reconcile | Pending |
| `UT-EM-254-004` | Alert deferral requeue — capped timing preserved | Pending |
| `UT-EM-254-005` | Spec drift early exit — `AssessmentReasonSpecDrift`, same conditions/events | Pending |
| `UT-EM-254-006` | Partial scope grace — DS partial scope, health+hash done, grace requeue | Pending |

### Tier 2: Integration Tests

**Testable code scope**: **Regression gate** — all 19 existing integration test files.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `REGRESSION-EM-254-IT` | All existing IT files pass unchanged | Pending |

### Tier 3: E2E Tests (if applicable)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| — | Not expanded for #254 | N/A |

### Tier Skip Rationale (if any tier is omitted)

- **E2E**: Existing EM E2E (if any) unchanged; decomposition is internal to controller binary.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-EM-254-001: Golden snapshot — WFP path

**BR**: BR-EM-001–012 (representative)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/reconciler_golden_wfp_test.go` (new file only)

**Preconditions**:

- Baseline captured **before** file extraction (RED phase) using fixed fixture EA object + fake client setup consistent with existing EM unit patterns

**Test Steps**:

1. **Given**: Seeded `EffectivenessAssessment` (or equivalent CR) and dependencies for **WaitingForPods** / WFP path
2. **When**: `Reconcile` executes once
3. **Then**: Serialized snapshot of selected status fields, `ctrl.Result`, and emitted events matches stored golden file

**Expected Results**:

1. Golden file match (or intentional update with PR justification)
2. No extra unexpected event types

**Acceptance Criteria**:

- **Behavior**: WFP transitions unchanged
- **Correctness**: Field-level equality for chosen snapshot schema
- **Accuracy**: Event order stable or normalized per snapshot design doc in test file comment

**Dependencies**: None

---

### UT-EM-254-002: Golden snapshot — Stabilizing path

**BR**: BR-EM-001–012
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/reconciler_golden_stabilizing_test.go` (new)

**Preconditions**:

- Fixture drives reconciler into **Stabilizing** phase (per current EM semantics)

**Test Steps**:

1. **Given**: Stabilizing fixture
2. **When**: `Reconcile` runs
3. **Then**: Snapshot matches baseline

**Expected Results**:

1. Golden match for status + Result + events
2. Requeue duration consistent with baseline if part of snapshot

**Acceptance Criteria**:

- **Behavior**: Stabilizing logic unchanged post-split
- **Correctness**: Same conditions and phase fields as baseline
- **Accuracy**: Metrics hooks (if in snapshot) unchanged

**Dependencies**: UT-EM-254-001 infrastructure (shared helpers)

---

### UT-EM-254-003: Atomic status update count

**BR**: BR-EM-001–012
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/reconciler_status_invariant_test.go` (new)

**Preconditions**:

- Fake client tracks `Update` calls to `EffectivenessAssessment` status subresource (pattern aligned with existing EM tests)

**Test Steps**:

1. **Given**: Happy-path fixture (no error injection)
2. **When**: Single `Reconcile`
3. **Then**: Exactly **one** status update observed

**Expected Results**:

1. `Update` count == 1
2. Optional: Subresource path matches `Status()` writer

**Acceptance Criteria**:

- **Behavior**: No accidental double patch of status
- **Correctness**: Matches pre-refactor invariant
- **Accuracy**: If exception paths exist, separate test documents >1 updates only where pre-existing

**Dependencies**: UT-EM-254-001

---

### UT-EM-254-004: Alert deferral requeue cap

**BR**: BR-EM-001–012
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/reconciler_alert_deferral_test.go` (new)

**Preconditions**:

- Fixture forces alert deferral branch (`alertDeferred`) with capped backoff

**Test Steps**:

1. **Given**: Deferral scenario
2. **When**: `Reconcile` returns
3. **Then**: `Result.RequeueAfter` (or equivalent) equals baseline cap

**Expected Results**:

1. Requeue duration matches golden or explicit constant from pre-refactor commit
2. No unbounded exponential growth beyond cap

**Acceptance Criteria**:

- **Behavior**: Operator-facing deferral timing unchanged
- **Correctness**: Exact duration equality vs baseline
- **Accuracy**: Document clock assumptions (no real sleep)

**Dependencies**: None

---

### UT-EM-254-005: Spec drift early exit

**BR**: BR-EM-001–012
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/reconciler_spec_drift_test.go` (new)

**Preconditions**:

- Spec hash mismatch vs status / expected hash to trigger `AssessmentReasonSpecDrift`

**Test Steps**:

1. **Given**: Drifted spec vs recorded hash
2. **When**: `Reconcile` runs
3. **Then**: Early exit with `AssessmentReasonSpecDrift`; conditions and events match snapshot

**Expected Results**:

1. Reason field matches
2. No full component pipeline side effects beyond documented early exit

**Acceptance Criteria**:

- **Behavior**: Same early-exit semantics as monolith
- **Correctness**: Condition types/status/reason stable
- **Accuracy**: Events include same audit markers if applicable (BR-AUDIT-006)

**Dependencies**: UT-EM-254-001 snapshot harness

---

### UT-EM-254-006: Partial scope grace

**BR**: BR-EM-001–012
**Priority**: P1
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/reconciler_partial_scope_test.go` (new)

**Preconditions**:

- DS **partial scope**: health + hash complete; grace period triggers requeue

**Test Steps**:

1. **Given**: Partial scope fixture with grace timer
2. **When**: `Reconcile` runs
3. **Then**: Grace requeue and intermediate status match baseline

**Expected Results**:

1. Same `pendingTransition` / scope-related fields as pre-refactor
2. Requeue honors grace

**Acceptance Criteria**:

- **Behavior**: No premature completion under partial scope
- **Correctness**: Same branch as monolith for `scope` locals factored into `reconcileContext`
- **Accuracy**: Aligns with `scope.go` extraction

**Dependencies**: UT-EM-254-001

---

### REGRESSION-EM-254-IT: All integration tests unchanged

**BR**: BR-EM-001–012, BR-AUDIT-006
**Priority**: P0
**Type**: Integration (suite)
**File**: Existing `test/integration/effectivenessmonitor/**/*.go` (**no edits**)

**Preconditions**:

- CI or local env per existing EM integration docs

**Test Steps**:

1. **Given**: Post-refactor binary
2. **When**: `go test ./test/integration/effectivenessmonitor/...`
3. **Then**: All tests pass; **zero** changes to test sources

**Expected Results**:

1. 19 integration files pass unchanged
2. No new `-ginkgo.skip` or pending specs

**Acceptance Criteria**:

- **Behavior**: External EM behavior unchanged
- **Correctness**: Full IT suite green
- **Accuracy**: proves wiring across new files

**Dependencies**: GREEN + REFACTOR complete

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External deps only; use existing fake client patterns from EM unit tests
- **Location**: `test/unit/effectivenessmonitor/` (**new files only** for #254)
- **Resources**: Standard CI

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Per [INTEGRATION_E2E_NO_MOCKS_POLICY.md](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- **Infrastructure**: Same as current EM integration (envtest / cluster per suite)
- **Location**: `test/integration/effectivenessmonitor/`
- **Resources**: Unchanged from baseline

### 10.3 E2E Tests (if applicable)

- Not expanded for #254.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Project `go.mod` | Build and test |
| Ginkgo CLI | v2.x | BDD runner |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Stable pre-refactor commit | Git | Required for baseline | Cannot diff golden | Tag commit before first extraction |
| EM fake client patterns | Code | Existing | Slower test authoring | Reuse helpers from current UTs |

### 11.2 Execution Order (TDD)

1. **RED**: Add UT-EM-254-001–006 (**new files only**); capture golden files from monolith reconciler
2. **GREEN**: Extract files/methods per proposed layout; keep **all** tests green without editing existing 35+19 files
3. **REFACTOR**: Rename for clarity; remove duplicate `ComputeDerivedTiming` blocks; consolidate; re-run full suite

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/254/TEST_PLAN.md` | Strategy |
| New unit tests | `test/unit/effectivenessmonitor/*_test.go` | Golden + invariant tests |
| Golden files | `test/unit/effectivenessmonitor/testdata/` (TBD) | Serialized baselines |
| Decomposed source | `internal/controller/effectivenessmonitor/*.go` | 12 files |
| Coverage report | CI | >=80% per tier |

---

## 13. Execution

```bash
# New golden / invariant tests
go test ./test/unit/effectivenessmonitor/... -ginkgo.v

# Full unit regression (35 files — count may vary; run full package)
go test ./test/unit/effectivenessmonitor/... -ginkgo.v

# Integration regression (19 files)
go test ./test/integration/effectivenessmonitor/... -ginkgo.v

# Focus single ID
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="UT-EM-254-001"

# Coverage
go test ./internal/controller/effectivenessmonitor/... -coverprofile=coverage-em.out
go tool cover -func=coverage-em.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

> **Pure refactor rule**: **None** by design. If a test file appears to “need” a change, treat it as a **behavior change** and stop — revert extraction hunk and fix production code.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| — | — | — | No modifications to existing tests |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #254 |

---

## 16. Preflight Findings (2026-04-10)

### 16.1 Test File Count Corrections

| Location | Plan Said | Actual | Action |
|----------|----------|--------|--------|
| `test/unit/effectivenessmonitor/` | 35 | **34** | Corrected |
| `test/integration/effectivenessmonitor/` | 19 | **23** | Corrected |

### 16.2 Status().Update Invariant Clarification

The plan's UT-EM-254-003 ("single Status().Update per reconcile") is **overstated**. The reconciler has multiple `Status().Update` sites across different code paths:
- WFP persist (~line 320)
- Stabilizing phase transitions (~line 360, ~396)
- Alert deferral persist (~line 713)
- Main Step 9 Assessing pipeline (~line 739)
- Completion/fail helpers (~line 1498, 1528, 1551)

**Mitigation**: Redefine UT-EM-254-003 as a **per-path matrix**: e.g. "Assessing path with component changes → exactly one Step 9 update" rather than a global single-update claim. Document pre-refactor update counts per path as the source of truth.

### 16.3 Golden Snapshot Strategy

- **Clock injection**: Use controlled `metav1.Time` for `CreationTimestamp`; set EA creation to a fixed past time
- **Normalize**: Strip `resourceVersion`, `generation`; fix `CompletedAt`, `LastTransitionTime`, `ValidityDeadline` to stable values or assert inequalities
- **`RequeueAfter`**: Assert against expected duration from frozen anchor or use bounded assertions
- **Events**: Assert with deterministic branching; sort by reason if order varies
- **Testdata path**: `test/unit/effectivenessmonitor/testdata/254/`

### 16.4 Key Risk: Phase Local Semantics

`currentPhase` is captured once at Reconcile entry (~line 264) and **not refreshed** after in-memory status changes. Extraction must preserve this "read-once" semantics. If validity phase code is refactored into a function that re-reads phase from the EA object, behavior drifts silently.

**Mitigation**: Add UT-EM-254-007 (characterization test) that asserts `currentPhase` is the entry-time value throughout the reconcile cycle.

### 16.5 Validated File Layout

All 12 proposed files stay under 500 lines. Largest is `assess_components.go` (~350-420 LOC). Extraction order (leaf-first):

1. `scope.go` — no dependencies
2. `pod_health.go` — pure functions
3. `target_resources.go` — needs Reconciler receiver
4. `assess_components.go` — depends on target_resources + pod_health
5. `events.go` — depends on assess result types
6. `completion.go` — depends on events
7. `reconcile_spec_drift.go` — depends on target_resources, completion
8. `reconcile_validity_phase.go` — depends on completion, events
9. `reconcile_components.go` — depends on assess, events, completion, scope
10. `reconcile_status.go` — depends on events
11. `reconcile_orchestrate.go` — wires context + calls above
12. `reconciler.go` — shrink to types, DI, thin Reconcile, manager setup

### 16.6 Updated Confidence: 78%

Structurally feasible; all files under 500 lines. Main risks: Status().Update invariant was overstated (corrected), time-dependent golden snapshots need clock strategy, and phase-local semantics must be preserved.

---

## 17. Adversarial Audit Findings & Resolutions (2026-04-10)

### 17.1 RESOLVED: §1.2/§5.4 single Status().Update claim

**Finding**: §1.2 and §5.4 still claim a single `Status().Update` on the happy path. Reality: multiple per-path updates (WFP ~320, Stabilizing ~360/396, alert deferral ~713, Step 9 ~739, completions ~1498/1528/1551). No two execute in the same Reconcile invocation, but the "single update" wording is misleading.

**Resolution**: Rewrite §1.2, §3 R1, §5.4 to say "single update **per code path**" with a per-path matrix documenting expected update count for each branch:

| Path | Status().Update site | Count per Reconcile |
|------|---------------------|---------------------|
| WFP persist | ~320 | 1 (then return) |
| Stabilizing (from WFP) | ~360 | 1 (then return) |
| Stabilizing + timing | ~396 | 1 (then return) |
| Alert deferral (Step 7b) | ~713 | 1 (then return at ~721) |
| Step 9 (Assessing pipeline) | ~739 | 1 (then return) |
| completeAssessment | ~1498 | 1 (separate call stack) |
| failAssessment | ~1528 | 1 (separate call stack) |

**Confirmed**: No two `Status().Update()` calls execute in the same Reconcile invocation.

### 17.2 RESOLVED: `currentPhase` behavioral landmine

**Finding**: `currentPhase` is read once from `ea.Status.Phase` at entry (~line 264) and never updated. Later mutations to `ea.Status.Phase` in memory don't affect `currentPhase`. Extraction that replaces `currentPhase` with fresh reads of `ea.Status.Phase` would change which branches run.

**Resolution**: 
- `reconcileContext` struct defines `EntryPhase` as **immutable** (set once at construction, never updated)
- All branch decisions that currently use `currentPhase` use `ctx.EntryPhase` instead
- UT-EM-254-007 (characterization test) asserts entry-phase semantics are preserved
- Code review checklist item: "Does any extracted method read `ea.Status.Phase` directly instead of `ctx.EntryPhase`?"

### 17.3 RESOLVED: Hash deferral exits without Status().Update

**Finding**: If Step 6 sets `ea.Status.Phase = Assessing` in memory, then hash deferral at ~553 returns without any `Status().Update`. The phase transition is lost until the next reconcile.

**Resolution**: This is **intentional behavior** — the hash hasn't been computed yet, so persisting "Assessing" prematurely would be incorrect. Document this as an invariant in the extraction:
- Add comment: "Hash deferral intentionally skips status persist; phase transition is persisted only after all components are evaluated"
- Add UT: Assert no `Status().Update` on hash deferral return path (golden snapshot must match baseline)
- Extraction guard: the hash deferral method must NOT add a "helpful flush"

### 17.4 RESOLVED: UT-EM-254-007 missing from §8

**Finding**: Proposed in preflight §16.4 but not added to the §8 scenario table.

**Resolution**: Add to §8:

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-254-007` | `currentPhase` (EntryPhase) immutability: branch decisions match monolith when `ea.Status.Phase` is mutated in memory but entry phase was X | Pending |

### 17.5 RESOLVED: `resolutionTimeSeconds` non-determinism in golden snapshots

**Finding**: `resolutionTimeSeconds` uses `time.Since(ea.Spec.RemediationCreatedAt.Time)` — non-deterministic.

**Resolution**: Golden harness must:
1. Freeze `RemediationCreatedAt` to a known past time
2. Either mock `time.Now` at the reconciler boundary or normalize `resolutionTimeSeconds` in the snapshot comparison (replace with `<RESOLUTION_TIME>` placeholder)
3. Similarly normalize `RequeueAfter` durations to bounded assertions (e.g., `> 0 && < maxDeadline`)

### 17.6 CORRECTED: Alert deferral fall-through (audit claim was wrong)

**Original audit claim**: If `alertDeferred.ShouldDefer` is true but `componentsChanged || pendingTransition` is false, execution falls through to Step 9.

**Preflight verification (lines 706-721)**: This is **incorrect**. If the Step 7b compound condition (lines 709-711: `alertDeferred.ShouldDefer && health && hash && (metrics || !prometheusEnabled)`) is **true**, execution **always returns at line 721** — the inner `Status().Update` (712-716) is optional but the return is unconditional.

Fall-through to Step 9 only occurs when the compound condition is **false** — e.g., `ShouldDefer` is true but metrics aren't yet assessed. In that case, the alert section set `alertDeferred` but 7b didn't trigger, and execution continues normally to Step 8-9. This is a **narrower** edge case than claimed.

**Resolution**: Document the corrected control flow. The UT for this edge case should test: alert deferred + metrics not yet assessed → Step 7b compound condition false → execution continues to Step 9 (components may change from other sections).

### 17.7 Hidden Wires: Complete Cross-Section Local Variable Map

For extraction reference, the audit identified 12 cross-section local variables:

| Variable | Set at | Used later by | Immutable? |
|----------|--------|---------------|------------|
| `logger` | After fetch | All logging | No (WithValues) |
| `ea` | Fetch | All mutations; single object pointer | No (mutated) |
| `currentPhase` | ~264 | WFP, Stabilizing, Assessing gates | **Yes** (must be) |
| `stabilizationAnchor` | After terminal check | Validity, WFP hash deadline | Yes |
| `isAsync` | After terminal check | Validity, WFP, stabilizing | Yes |
| `windowState` | Validity/stabilization | Branches Steps 4-6 | Yes |
| `hashDeadline` | Alias of stabilizationAnchor | WFP requeue | Yes |
| `pendingTransition` | Step 6 | Steps 7b, 9 | Progressive |
| `scope` | After spec drift | Partial grace, early completion | Yes |
| `componentsChanged` | Component pipeline | Steps 7b, 9 | Progressive |
| `alertDeferred` | Alert branch | Step 7b | Yes (after set) |
| `completing` | After allComponentsDone | Step 9, events | Yes (after set) |

### 17.8 Ordering Constraints (10 documented)

1. Fetch → validate → terminal → validity/WFP/expired/stabilizing → active
2. Expired `completeAssessment` before stabilizing/active (mutually exclusive)
3. WFP persist → emit → return (no fall-through)
4. Stabilizing persist → optional emit → requeue return
5. Step 6 in-memory Assessing before Step 6.5 drift
6. Component order: hash → health → partial-grace → alert → metrics
7. `componentsChanged` reflects all mutations before Steps 7b/9
8. Step 7b: optional update at ~713 then return (never reaches Step 9)
9. Step 8 (completing) before Step 9
10. Step 9 update success before emit events

### 17.9 Early Returns (17 documented)

Lines ~244, 247, 259, 269, 330, 340, 362-363, 397-398, 405, 418, 500, 519, 553, 620, 622, 715-716, 721, 744-745, 757, 762.

Each is a potential control flow break if extraction changes the return path. Extraction methods must return `(ctrl.Result, error)` to preserve early-return semantics.

### 17.10 Test Regression Risk Assessment

- **Low risk**: Tests use package `effectivenessmonitor` — only exported identifiers. Moving methods between files in the same package doesn't change receivers or exports.
- **No line-number references** in tests.
- **No gomock/monkey patching** of Reconciler methods.
- **File-name-based test runners**: `go test ./test/unit/effectivenessmonitor/...` is path-based; splitting `reconciler.go` doesn't affect discovery.

### 17.11 Updated Confidence: 86%

All critical findings resolved without requiring user input:
- Status update invariant corrected to per-path matrix
- `currentPhase` immutability documented and test-guarded
- Hash deferral "no persist" behavior documented as intentional
- Golden snapshot normalization strategy for time-dependent fields
- Complete hidden wire map (12 variables) and ordering constraints (10) documented
- 17 early returns cataloged for extraction safety
- UT-EM-254-007 added to scenario table
- Low regression risk confirmed

Remaining risks:
- Golden snapshot setup complexity (clock injection, field normalization)
- Extraction of ~1868 lines into 12 files is mechanically complex (but leaf-first order mitigates)

---

## 18. Targeted Preflight Verification (2026-04-10)

### 18.1 VERIFIED: `currentPhase` exact line numbers

- Declaration: **line 263** (`currentPhase := ea.Status.Phase`)
- Normalization: **line 265** (`currentPhase = eav1.PhasePending`)
- Read sites: **lines 264, 267, 302, 356, 368, 415, 416, 417-418** (8 total)
- All reads gate branch decisions; no reads after Step 6 mutations

### 18.2 VERIFIED: `ea.Status.Phase` write sites

In-memory mutations: **lines 308, 357, 384, 439, 1521, 1572**. Confirms `currentPhase` diverges from `ea.Status.Phase` after any write — extraction must preserve read-once semantics.

### 18.3 VERIFIED: All flag assignments and reads

| Variable | Assignment lines | Read lines |
|----------|-----------------|------------|
| `pendingTransition` | 414, 454 | 416-418, 712, 732, 738, 748 |
| `componentsChanged` | 527, 562, 593, 676, 681, 703 | 712, 731-732, 738, 741 |
| `completing` | 725, 727 | 738, 741, 751 |
| `scope` | 515 | 516-519, 521-524, 612 |
| `alertDeferred` | 528 (decl), 631 (assign) | 632, 635, 709, 718-719 |

### 18.4 VERIFIED: completion/fail methods call Status().Update internally

- `completeAssessment` (line 1492): calls `setCompletionFields` → `Status().Update` at 1498-1502
- `failAssessment` (line 1517): sets `PhaseFailed` at 1521 → `Status().Update` at 1528-1532
- `completeAssessmentWithReason` (line 1546): calls `setCompletionFields` → `Status().Update` at 1551-1555

All return `(ctrl.Result, error)`, not the updated object.

### 18.5 VERIFIED: Hash deferral returns without Status().Update

Line 553: `return ctrl.Result{RequeueAfter: ...}, nil` — confirmed no Status().Update in this path.

### 18.6 Updated Confidence: 95%

All hidden wires verified against exact line numbers. Alert deferral fall-through corrected (narrower than claimed). Completion methods confirmed as self-contained (internal Status().Update). Extraction safety validated.

---
