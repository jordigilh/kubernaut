# Test Plan: WorkflowExecution Controller Validation Observability (SOC2)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-659-v1
**Feature**: Emit `WorkflowValidationFailed` Kubernetes events (and aligned observability) for all validation-class failures in the WorkflowExecution controller, closing gaps vs spec/catalog paths for SOC2-aligned monitoring and incident response
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3` (or feature branch for #659)

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

The WorkflowExecution controller already emits `WorkflowFailed` events and audit records for many failure paths. Validation-class failures (dependency validation, unsupported engine) currently reach operators primarily through generic `WorkflowFailed` via `MarkFailedWithReason`, weakening distinguishability for monitoring, alerting, and SOC2 control evidence (CC7.2 monitoring, CC7.3 incident response, CC8.1 change management). This test plan defines unit and integration scenarios to enforce **human-approved** behavior: add `WorkflowValidationFailed` for all validation-class failures, consistent with spec validation and catalog resolution paths.

### 1.2 Objectives

1. **Event reason uniformity**: Dependency validation failure and unsupported engine failure emit `WorkflowValidationFailed` **before** terminal status updates that use `MarkFailedWithReason`, matching spec/catalog patterns.
2. **Regression guards**: Catalog resolution failure and spec validation failure continue to emit `WorkflowValidationFailed` (existing behavior locked by tests).
3. **Running-phase observability**: `reconcileRunning` engine resolution failure emits an appropriate event and log level per product standards (P1).
4. **Integration fidelity**: Full `reconcilePending` with failing `DependencyValidator` drains fake recorder and asserts `WorkflowValidationFailed` reason string.
5. **Coverage**: >=80% of controller **validation paths** exercised by this plan’s tiers combined with existing suite.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/workflowexecution/...` (focus on #659 / controller) |
| Integration test pass rate | 100% | `go test ./test/integration/workflowexecution/...` (IT-WE-659-001) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on validation-path logic in controller (shared with existing tests) |
| Integration-testable code coverage | >=80% | Reconcile paths with fake client + recorder |
| Backward compatibility | 0 unintended regressions | Existing controller/integration tests updated only where event reason intentionally changes |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-WE-005: Failure audit trail for workflow execution
- BR-WE-095: Kubernetes event observability for workflow execution
- DD-EVENT-001: Event reason naming and emission conventions (validation vs generic failure)
- SOC2 Trust Services Criteria: CC7.2, CC7.3, CC8.1 (monitoring, incident response, change management)
- Issue #659: WE Controller observability audit for SOC2 compliance

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
| R1 | Duplicate or conflicting events (validation + failed) confuse operators | Alert noise, unclear root cause | Medium | UT-WE-659-001..004, IT-WE-659-001 | Assert event type/reason and ordering; align with spec/catalog single-emission pattern |
| R2 | Audit code / reason taxonomy drift from K8s event | SOC2 evidence inconsistency | Medium | All P0 tests | Assert recorder reason matches DD-EVENT-001; cross-check audit if tested in suite |
| R3 | Integration test flakiness from timing | False CI failures | Low | IT-WE-659-001 | No `time.Sleep`; deterministic fake recorder drain |
| R4 | Missed path (new validation without event) | Gap returns | Medium | Audit matrix + UT-WE-659-001..004 | Table-driven review of controller branches |

### 3.1 Risk-to-Test Traceability

- **R1, R2** → UT-WE-659-001, UT-WE-659-002, UT-WE-659-003, UT-WE-659-004, IT-WE-659-001
- **R3** → IT-WE-659-001 (explicit anti-pattern: no sleep; sync via reconcile completion)
- **R4** → Manual audit matrix in Section 4 + P0 regression rows

**Audit matrix (code analysis baseline)**:

| Path | Has Event before MarkFailed? | Event Reason (target) |
|------|------------------------------|------------------------|
| Spec validation fail | YES | WorkflowValidationFailed |
| Catalog resolution fail | YES | WorkflowValidationFailed |
| Unsupported engine | NO → **fix** | WorkflowValidationFailed |
| Dependency validation fail | NO → **fix** | WorkflowValidationFailed |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **WorkflowExecution reconciler** (`internal/controller/workflowexecution/workflowexecution_controller.go` ~1739 lines): Event emission via `r.Recorder.Event` on validation-class failures; interaction with `MarkFailed` / `MarkFailedWithReason`
- **Pending reconcile path**: Dependency validator failure → `WorkflowValidationFailed`
- **Engine selection path**: Unsupported engine → `WorkflowValidationFailed`
- **Regression paths**: Catalog resolution failure, spec validation failure
- **Running reconcile** (P1): Engine resolution failure → appropriate event + log level

### 4.2 Features Not to be Tested

- **Unrelated reconciler phases** (e.g. success paths without validation failure): Covered by existing suites unless regression detected
- **External cloud audit sinks**: Unit/integration use fake recorders; end-to-end export is out of scope
- **Full SOC2 audit pack**: This plan validates technical observability behavior, not auditor procedures

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add `WorkflowValidationFailed` before `MarkFailedWithReason` on gap paths | Aligns unsupported engine and dependency validation with spec/catalog human decision |
| REFACTOR: `emitValidationFailed` helper | Reduces drift; matches existing spec/catalog emission style |
| IT-WE-659-001 uses envtest/fake recorder pattern | Satisfies integration tier without mocks of business logic |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** validation-path branches in the controller test harness (existing `controller_test.go` patterns)
- **Integration**: >=80% of **integration-testable** reconcile wiring with fake Kubernetes client and event recorder
- **E2E**: Not mandatory for this issue; optional future plan if product requires live cluster event verification

### 5.2 Two-Tier Minimum

BR-WE-005 and BR-WE-095 are covered by **Unit + Integration**: unit tests assert recorder invocations on isolated reconciler setup; integration test asserts full `reconcilePending` with failing `DependencyValidator` and drained events.

### 5.3 Business Outcome Quality Bar

Tests validate **operator-visible signals**: Kubernetes event **reason** and message content suitable for monitoring dashboards and incident triage, not only that `MarkFailed` ran.

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9** — When is this test plan considered passed or failed?

**PASS** — all of the following must be true:

1. All P0 tests pass: UT-WE-659-001, UT-WE-659-002, UT-WE-659-003, UT-WE-659-004, IT-WE-659-001
2. UT-WE-659-005 (P1) passes or is explicitly waived with reviewer approval
3. Validation-path coverage >=80% per project measurement approach
4. No use of `Skip()`, `time.Sleep`, or nil-only assertions
5. Unsupported engine and dependency validation paths emit `WorkflowValidationFailed` before terminal failure handling

**FAIL** — any of the following:

1. Any P0 test fails
2. Validation paths still emit only generic `WorkflowFailed` where `WorkflowValidationFailed` is required
3. Integration test cannot deterministically observe event reason
4. New duplicate/conflicting events without documented acceptance

### 5.5 Suspension & Resumption Criteria

> **IEEE 829 §10** — When should testing stop? When can it resume?

**Suspend testing when**:

- `envtest` or controller suite infrastructure broken cluster-wide
- Merge conflict in `workflowexecution_controller.go` blocks consistent event API
- More than five unrelated failures in workflowexecution test packages

**Resume testing when**:

- Infrastructure restored
- Controller branch rebased and compiles
- Root cause for cascading failures fixed

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | Validation branches: spec validation, catalog resolution, dependency validation, engine support checks; helpers after REFACTOR (e.g. `emitValidationFailed`) | ~1739 (subset: validation paths) |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending` (full path with DependencyValidator) | (subset) |
| `test/integration/workflowexecution/` | Suite setup, fake recorder, reconciler wiring | Per existing suite |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | Branch HEAD for #659 | After GREEN/REFACTOR |
| Related issues | #659 | Observability / SOC2 alignment |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-005 | Failure audit / status failure alignment | P0 | Unit | UT-WE-659-001, UT-WE-659-002 | Pending |
| BR-WE-005 | Failure audit / status failure alignment | P0 | Integration | IT-WE-659-001 | Pending |
| BR-WE-095 | K8s event observability | P0 | Unit | UT-WE-659-001..004 | Pending |
| BR-WE-095 | K8s event observability | P0 | Integration | IT-WE-659-001 | Pending |
| BR-WE-095 | Running-phase engine resolution observability | P1 | Unit | UT-WE-659-005 | Pending |
| DD-EVENT-001 | Event reason conventions | P0 | Unit/IT | UT-WE-659-001..004, IT-WE-659-001 | Pending |

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

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `WE`
- **ISSUE**: `659`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `workflowexecution_controller.go` validation and running-path observability — >=80% validation-path coverage combined with existing tests.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-WE-659-001 (P0) | Dependency validation failure → `WorkflowValidationFailed` event emitted | Pending |
| UT-WE-659-002 (P0) | Unsupported engine → `WorkflowValidationFailed` event emitted | Pending |
| UT-WE-659-003 (P0) | Catalog resolution failure → `WorkflowValidationFailed` (regression guard) | Pending |
| UT-WE-659-004 (P0) | Spec validation failure → `WorkflowValidationFailed` (regression guard) | Pending |
| UT-WE-659-005 (P1) | `reconcileRunning` engine resolution failure → appropriate event + log level | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Full pending reconcile with real controller wiring pattern from `test/integration/workflowexecution/` — >=80% for exercised reconcile path.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-WE-659-001 (P0) | `reconcilePending` with failing DependencyValidator → drain fake recorder; assert `WorkflowValidationFailed` reason | Pending |

### Tier 3: E2E Tests (if applicable)

**Testable code scope**: Not required for #659 closure.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| — | Optional future: live cluster event assertion | Deferred |

### Tier Skip Rationale (if any tier is omitted)

- **E2E**: Deferred. Unit + integration provide sufficient evidence for event reason emission; E2E adds cluster cost without unique coverage for this change if integration already drains recorder on real reconciler.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-WE-659-001 (P0): Dependency validation failure → WorkflowValidationFailed

**BR**: BR-WE-005, BR-WE-095
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go` (or established controller unit test file)

**Preconditions**:

- Reconciler constructed with fake client and **fake event recorder**
- Dependency validator returns error (or dependency manifest invalid per test setup)

**Test Steps**:

1. **Given**: WorkflowExecution resource triggers pending reconcile path that runs dependency validation
2. **When**: Reconcile processes validation failure
3. **Then**: Recorder receives an event with reason `WorkflowValidationFailed` **before** or in conjunction with status update per spec/catalog ordering (must not be only generic `WorkflowFailed` for validation class)

**Expected Results**:

1. Event reason string equals `WorkflowValidationFailed` (exact constant per `DD-EVENT-001`)
2. `MarkFailedWithReason` may still run; validation event must be present for operators

**Acceptance Criteria**:

- **Behavior**: Monitoring can filter validation failures distinctly
- **Correctness**: Aligns with audit matrix row "Dependency validation fail"
- **Accuracy**: Event targets correct object reference (workflow execution)

**Dependencies**: Existing controller unit test harness

**TDD note**: RED expects `WorkflowValidationFailed`; current code may only emit `WorkflowFailed` until GREEN.

---

### UT-WE-659-002 (P0): Unsupported engine → WorkflowValidationFailed

**BR**: BR-WE-005, BR-WE-095
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**: Workflow spec references engine type controller does not support

**Test Steps**:

1. **Given** / **When**: Reconcile hits unsupported engine branch
2. **Then**: `WorkflowValidationFailed` recorded

**Expected Results**: Same as UT-WE-659-001 for reason string

**Acceptance Criteria**: Closes audit matrix gap "Unsupported engine"

**Dependencies**: UT-WE-659-001 patterns

---

### UT-WE-659-003 (P0): Catalog resolution failure — regression

**BR**: BR-WE-095
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**: Catalog resolver returns error (not found or DS error per existing test harness)

**Test Steps**: Trigger catalog resolution failure path; assert `WorkflowValidationFailed`

**Expected Results**: Preserves existing behavior (no regression to generic-only failure)

**Acceptance Criteria**: Audit matrix row "Catalog resolution fail" remains YES

**Dependencies**: None

---

### UT-WE-659-004 (P0): Spec validation failure — regression

**BR**: BR-WE-095
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**: Invalid spec fields fail validation

**Test Steps**: Reconcile; assert `WorkflowValidationFailed`

**Expected Results**: Existing behavior locked

**Acceptance Criteria**: Audit matrix row "Spec validation fail" remains YES

**Dependencies**: None

---

### UT-WE-659-005 (P1): reconcileRunning engine resolution failure

**BR**: BR-WE-095
**Priority**: P1
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**: Running phase; engine resolution fails (mock or injected error per harness)

**Test Steps**: Reconcile; assert appropriate event reason and log level (per product standard — document exact reason in implementation if not `WorkflowValidationFailed`)

**Expected Results**: Observable signal for operators; logs match severity guidelines

**Acceptance Criteria**: No silent failure; no nil-only assertions

**Dependencies**: Clarify expected reason in GREEN (validation vs execution failure) per code review

---

### IT-WE-659-001 (P0): reconcilePending + DependencyValidator — recorder drain

**BR**: BR-WE-005, BR-WE-095
**Priority**: P0
**Type**: Integration
**File**: `test/integration/workflowexecution/` (e.g. `reconciler_test.go` or new file aligned with suite conventions)

**Preconditions**:

- envtest/suite setup per existing workflowexecution integration tests
- Fake recorder attached to manager/controller
- DependencyValidator wired to fail for test scenario

**Test Steps**:

1. **Given**: WorkflowExecution CR triggers pending reconcile
2. **When**: Dependency validation fails inside reconcile
3. **Then**: Drain fake recorder; assert at least one event with reason `WorkflowValidationFailed`

**Expected Results**:

1. Deterministic assertion without `time.Sleep`
2. Event associated with correct object

**Acceptance Criteria**: SOC2-oriented monitoring can rely on integration-verified path

**Dependencies**: Suite bootstrap from `suite_test.go`

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Kubernetes client fakes, fake event recorder (external boundary); **no** mocking of `pkg/` business logic per strategy
- **Location**: `test/unit/workflowexecution/`
- **Resources**: Standard developer machine

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks of business logic; use envtest/env fake clients per [No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- **Infrastructure**: Existing workflowexecution integration suite (envtest)
- **Location**: `test/integration/workflowexecution/`
- **Resources**: Comparable to existing integration jobs (CPU/memory per CI)

### 10.3 E2E Tests (if applicable)

- Not required for TP-659-v1.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Project `go.mod` | Build and test |
| Ginkgo CLI | v2.x | BDD test runner |
| envtest | Controller-runtime compatible | Integration reconciler tests |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Fake recorder API in unit tests | Code | Existing | Cannot assert events | Add helper consistent with suite |
| Integration suite green | Infra | Required | IT-WE-659-001 blocked | Fix suite before adding case |

### 11.2 Execution Order (TDD)

1. **RED**: UT-WE-659-001, UT-WE-659-002 expect `WorkflowValidationFailed` (currently fail if only `WorkflowFailed`); add IT-WE-659-001 RED if feasible in parallel
2. **GREEN**: `r.Recorder.Event(..., WorkflowValidationFailed, ...)` before `MarkFailedWithReason` on dependency and unsupported-engine paths
3. **REFACTOR**: Extract `emitValidationFailed` (or equivalent) aligned with spec/catalog pattern; UT-WE-659-003/004 as regression guards
4. **Check**: UT-WE-659-005; full `go test` workflowexecution unit + integration packages

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/659/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `test/unit/workflowexecution/controller_test.go` (or sibling) | P0/P1 scenarios |
| Integration test | `test/integration/workflowexecution/` | IT-WE-659-001 |
| Coverage / CI | Pipeline artifacts | Validation-path coverage |

---

## 13. Execution

```bash
# Unit tests — workflowexecution
go test ./test/unit/workflowexecution/... -ginkgo.v

# Focus by scenario (adjust to match Describe/Context text)
go test ./test/unit/workflowexecution/... -ginkgo.focus="659" -ginkgo.v

# Integration tests
go test ./test/integration/workflowexecution/... -ginkgo.v

# Focus IT-WE-659-001 when named in description
go test ./test/integration/workflowexecution/... -ginkgo.focus="IT-WE-659" -ginkgo.v
```

---

## 14. Existing Tests Requiring Updates (if applicable)

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| Controller unit tests for unsupported engine / dependency failure | May assert only `WorkflowFailed` or status condition | Add or update to expect `WorkflowValidationFailed` on recorder | Close observability gap #659 |
| Tests that count total events | Fixed counts | Update counts if new validation event added | Avoid brittle over-count failures |
| Integration reconciler tests | No validation event assertion | Add IT-WE-659-001 assertions | Tier coverage |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #659 |
