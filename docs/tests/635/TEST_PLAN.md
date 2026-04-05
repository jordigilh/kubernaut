# Test Plan: RR kubectl Column Layout — ALERT, CONFIDENCE, Composite TARGET, Human-readable WORKFLOW

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-635-v1.0
**Feature**: Improve RR kubectl output with meaningful default columns (ALERT, CONFIDENCE, composite TARGET, human-readable WORKFLOW)
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc6`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

Operators triage remediations via `kubectl get rr`. The current output is noisy: it shows
opaque UUIDs for workflows, redundant namespace/kind columns, and omits critical fields
(alert name, AI confidence). This test plan validates that the revised column layout delivers
the information operators need at a glance, using composite `Kind/Name` fields, a
human-readable workflow display name, and an AI confidence column.

### 1.2 Objectives

1. **New status fields exist and are typed correctly**: `TargetDisplay`, `Confidence`, `WorkflowDisplayName`, `SignalTargetDisplay` fields added to `RemediationRequestStatus`
2. **Display helpers produce correct composites**: `FormatResourceDisplay("Deployment", "web-frontend")` → `"Deployment/web-frontend"`; `FormatWorkflowDisplay("GitRevertCommit", "git-revert-v2")` → `"GitRevertCommit:git-revert-v2"`
3. **RO populates new fields at correct lifecycle points**: After AIAnalysis completes and WFE is created, the new fields are set on the RR status
4. **Printer columns reference valid JSONPaths**: Default view shows Phase, Outcome, TARGET, ALERT, WORKFLOW, CONFIDENCE, Age; wide view adds SOURCE, SIGNAL TARGET, NAMESPACE, REASON

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on display helpers |
| Backward compatibility | 0 regressions | Existing RO tests pass without modification |
| CRD generation | Clean | `make manifests` produces valid CRD YAML |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-PLATFORM-635: RR kubectl output — improve column layout, add ALERT/CONFIDENCE, human-readable WORKFLOW
- Issue #635: UX: RR kubectl output — improve column layout, add ALERT/CONFIDENCE, human-readable WORKFLOW
- Issue #636: RR REASON column shows "Ready" during active phases (prerequisite, completed)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- CRD types: `api/remediation/v1alpha1/remediationrequest_types.go`
- Conditions package: `pkg/remediationrequest/conditions.go`
- RO reconciler: `internal/controller/remediationorchestrator/reconciler.go`

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | JSONPath in printer columns references non-existent field | `kubectl get rr` shows `<none>` for new columns | Medium | UT-RO-635-001 through -005 | Unit tests validate field existence; `make manifests` regenerates CRD |
| R2 | RO fails to populate display fields during WFE creation | New columns always empty in live clusters | Medium | UT-RO-635-006, -007, -008 | Unit tests for display helpers; integration coverage via existing RO tests |
| R3 | DeepCopy not regenerated after adding new fields | Runtime panics on status updates | Low | Build validation | `make generate` as part of REFACTOR phase |
| R4 | Existing tests assert on old printer column layout | Test regression | Low | Checkpoint B | Grep for old column names in test assertions |

### 3.1 Risk-to-Test Traceability

- **R1** → UT-RO-635-001 (field existence), UT-RO-635-002 (kubebuilder markers — manual verification)
- **R2** → UT-RO-635-006 (FormatResourceDisplay), UT-RO-635-007 (FormatWorkflowDisplay), UT-RO-635-008 (FormatConfidence)
- **R3** → Checkpoint B (build validation after `make generate`)
- **R4** → Checkpoint B (regression scan)

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Display helpers** (`pkg/remediationrequest/display.go`): Pure functions that format composite display strings for printer columns
- **Status fields** (`api/remediation/v1alpha1/remediationrequest_types.go`): New `TargetDisplay`, `Confidence`, `WorkflowDisplayName`, `SignalTargetDisplay` fields
- **Kubebuilder markers** (`api/remediation/v1alpha1/remediationrequest_types.go`): Updated `+kubebuilder:printcolumn` annotations
- **RO field population** (`internal/controller/remediationorchestrator/reconciler.go`): Setting new fields when WFE is created

### 4.2 Features Not to be Tested

- **REASON column semantics**: Covered by Issue #636 (already implemented)
- **DeduplicationStatus changes**: Out of scope (Gateway-owned)
- **E2E kubectl output verification**: Deferred — CRD column layout is validated by unit tests on types + `make manifests`

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Store display composites as status fields (not computed at query time) | JSONPath in `additionalPrinterColumns` cannot concatenate multiple fields; must reference a single field |
| Use `string` for Confidence (not float64) | Printer columns render floats inconsistently; string gives exact control (e.g., "0.97") |
| ALERT reads from `spec.signalName` directly | Already available in spec, no new status field needed |
| NAMESPACE reads from `status.remediationTarget.namespace` directly | Already populated by RO, no new status field needed |
| Display helpers are pure functions in `pkg/remediationrequest/` | Unit-testable, reusable, decoupled from controller logic |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (display helpers: `FormatResourceDisplay`, `FormatWorkflowDisplay`, `FormatConfidence`)
- **Integration**: Deferred — RO field population is covered by existing integration tests that will naturally exercise the new code paths
- **E2E**: Deferred — validated by `make manifests` producing correct CRD YAML

### 5.2 Two-Tier Minimum

- **Unit tests**: Validate display helpers produce correct format strings for all input combinations
- **Build validation**: `make manifests && make generate` ensures CRD schema and deepcopy are correct

Tier skip: Integration and E2E are deferred because the new code is purely additive (display fields set alongside existing `SelectedWorkflowRef` and `RemediationTarget` writes) and existing integration tests exercise those code paths.

### 5.3 Business Outcome Quality Bar

Each test answers: "Does the operator see the right information in `kubectl get rr`?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% threshold for display helpers
3. No regressions in existing RO test suites
4. `make manifests` and `make generate` succeed cleanly
5. Generated CRD YAML contains the expected `additionalPrinterColumns` entries

**FAIL** — any of the following:

1. Any P0 test fails
2. Coverage below 80% on display helper functions
3. Existing tests that were passing before the change now fail
4. `make manifests` or `make generate` fails

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken — code does not compile
- `make generate` fails (deepcopy generation broken)

**Resume testing when**:
- Build fixed and green

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationrequest/display.go` (NEW) | `FormatResourceDisplay`, `FormatWorkflowDisplay`, `FormatConfidence` | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | Status update blocks at WFE creation (2 locations) | ~10 added lines |
| `api/remediation/v1alpha1/remediationrequest_types.go` | New status fields + kubebuilder markers | ~20 added lines |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc6` HEAD | Branch for v1.2.0-rc6 fixes |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-635 | FormatResourceDisplay produces `Kind/Name` | P0 | Unit | UT-RO-635-001 | Pending |
| BR-PLATFORM-635 | FormatResourceDisplay handles empty Kind/Name | P0 | Unit | UT-RO-635-002 | Pending |
| BR-PLATFORM-635 | FormatWorkflowDisplay produces `ActionType:WorkflowID` | P0 | Unit | UT-RO-635-003 | Pending |
| BR-PLATFORM-635 | FormatWorkflowDisplay handles missing ActionType | P0 | Unit | UT-RO-635-004 | Pending |
| BR-PLATFORM-635 | FormatConfidence produces string from float64 | P0 | Unit | UT-RO-635-005 | Pending |
| BR-PLATFORM-635 | New status fields exist in RemediationRequestStatus | P0 | Unit | UT-RO-635-006 | Pending |
| BR-PLATFORM-635 | FormatResourceDisplay for cluster-scoped resources (no namespace) | P1 | Unit | UT-RO-635-007 | Pending |
| BR-PLATFORM-635 | FormatConfidence edge cases (0.0, 1.0, negative) | P1 | Unit | UT-RO-635-008 | Pending |

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

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `RO` (Remediation Orchestrator)
- **BR_NUMBER**: 635
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/remediationrequest/display.go` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-635-001` | Operator sees `Deployment/web-frontend` in TARGET column | Pending |
| `UT-RO-635-002` | Missing Kind or Name produces graceful fallback (empty string or partial) | Pending |
| `UT-RO-635-003` | Operator sees `GitRevertCommit:git-revert-v2` in WORKFLOW column | Pending |
| `UT-RO-635-004` | Missing ActionType shows just WorkflowID; missing both shows empty | Pending |
| `UT-RO-635-005` | Operator sees `0.97` in CONFIDENCE column (string from float64) | Pending |
| `UT-RO-635-006` | Status struct has TargetDisplay, Confidence, WorkflowDisplayName, SignalTargetDisplay fields | Pending |
| `UT-RO-635-007` | Cluster-scoped resource (Node) produces `Node/worker-1` (no namespace prefix) | Pending |
| `UT-RO-635-008` | Confidence edge cases: 0.0 → "0.00", 1.0 → "1.00", -1 → "" | Pending |

### Tier Skip Rationale

- **Integration**: RO field population is exercised by existing integration tests that create WFEs. Adding new fields alongside existing writes is purely additive and does not change control flow.
- **E2E**: Column layout is validated by CRD generation (`make manifests`). Live `kubectl get rr` testing is a manual verification step, not automated E2E.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-RO-635-001: FormatResourceDisplay standard namespaced resource

**BR**: BR-PLATFORM-635
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Preconditions**:
- `FormatResourceDisplay` function exists in `pkg/remediationrequest/`

**Test Steps**:
1. **Given**: Kind="Deployment", Name="web-frontend"
2. **When**: `FormatResourceDisplay("Deployment", "web-frontend")` is called
3. **Then**: Returns `"Deployment/web-frontend"`

**Expected Results**:
1. Return value equals `"Deployment/web-frontend"`

**Acceptance Criteria**:
- **Behavior**: Composites Kind/Name with `/` separator
- **Correctness**: Exact string match

### UT-RO-635-002: FormatResourceDisplay with empty inputs

**BR**: BR-PLATFORM-635
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Preconditions**:
- `FormatResourceDisplay` function exists

**Test Steps**:
1. **Given**: Kind="", Name="web-frontend"
2. **When**: `FormatResourceDisplay("", "web-frontend")` is called
3. **Then**: Returns `"web-frontend"` (graceful fallback)
4. **Given**: Kind="Deployment", Name=""
5. **When**: `FormatResourceDisplay("Deployment", "")` is called
6. **Then**: Returns `""` (no name means no display)
7. **Given**: Kind="", Name=""
8. **When**: `FormatResourceDisplay("", "")` is called
9. **Then**: Returns `""`

**Expected Results**:
1. Empty name → empty string
2. Empty kind → just name
3. Both empty → empty string

### UT-RO-635-003: FormatWorkflowDisplay standard workflow

**BR**: BR-PLATFORM-635
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Preconditions**:
- `FormatWorkflowDisplay` function exists

**Test Steps**:
1. **Given**: ActionType="GitRevertCommit", WorkflowID="git-revert-v2"
2. **When**: `FormatWorkflowDisplay("GitRevertCommit", "git-revert-v2")` is called
3. **Then**: Returns `"GitRevertCommit:git-revert-v2"`

### UT-RO-635-004: FormatWorkflowDisplay with missing components

**BR**: BR-PLATFORM-635
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Test Steps**:
1. **Given**: ActionType="", WorkflowID="git-revert-v2"
2. **When**: Called
3. **Then**: Returns `"git-revert-v2"` (fallback to ID only)
4. **Given**: ActionType="GitRevertCommit", WorkflowID=""
5. **When**: Called
6. **Then**: Returns `""` (no ID means no display)

### UT-RO-635-005: FormatConfidence standard value

**BR**: BR-PLATFORM-635
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Test Steps**:
1. **Given**: Confidence=0.97
2. **When**: `FormatConfidence(0.97)` is called
3. **Then**: Returns `"0.97"`

### UT-RO-635-006: New status fields exist in RemediationRequestStatus

**BR**: BR-PLATFORM-635
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Test Steps**:
1. **Given**: A `RemediationRequestStatus` struct
2. **When**: Set `TargetDisplay`, `Confidence`, `WorkflowDisplayName`, `SignalTargetDisplay`
3. **Then**: Fields hold the assigned values (compile-time + runtime validation)

### UT-RO-635-007: FormatResourceDisplay cluster-scoped resource

**BR**: BR-PLATFORM-635
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Test Steps**:
1. **Given**: Kind="Node", Name="worker-1"
2. **When**: `FormatResourceDisplay("Node", "worker-1")` is called
3. **Then**: Returns `"Node/worker-1"`

### UT-RO-635-008: FormatConfidence edge cases

**BR**: BR-PLATFORM-635
**Priority**: P1
**Type**: Unit
**File**: `test/unit/remediationorchestrator/remediationrequest/display_test.go`

**Test Steps**:
1. 0.0 → `"0.00"`
2. 1.0 → `"1.00"`
3. -1.0 → `""` (invalid, returns empty)
4. 0.5 → `"0.50"`

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — pure function tests
- **Location**: `test/unit/remediationorchestrator/remediationrequest/`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-gen | latest | `make manifests` / `make generate` |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #636 | Code | Completed | REASON column would show generic values | N/A — already done |

### 11.2 Execution Order

1. **Phase 1 — TDD RED**: Write all 8 failing unit tests (UT-RO-635-001 through -008)
2. **Checkpoint A**: Verify all tests fail for the correct reason (undefined symbols / field access)
3. **Phase 2 — TDD GREEN**: Add display helpers + new status fields + kubebuilder markers (minimal)
4. **Checkpoint B**: All tests pass, build clean, no regressions, `make manifests` succeeds
5. **Phase 3 — TDD REFACTOR**: Update RO reconciler to populate new fields; code quality pass
6. **Checkpoint C**: Full due diligence — lint, coverage, anti-pattern scan, CRD YAML validation

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/635/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/remediationorchestrator/remediationrequest/display_test.go` | Ginkgo BDD test file |
| Display helpers | `pkg/remediationrequest/display.go` | Pure formatting functions |
| Updated CRD types | `api/remediation/v1alpha1/remediationrequest_types.go` | New status fields + markers |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/remediationorchestrator/remediationrequest/... -ginkgo.v

# Specific test by ID
go test ./test/unit/remediationorchestrator/remediationrequest/... -ginkgo.focus="UT-RO-635"

# Coverage
go test ./test/unit/remediationorchestrator/remediationrequest/... -coverprofile=coverage.out
go tool cover -func=coverage.out

# CRD regeneration
make manifests
make generate
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None expected | N/A | N/A | New fields are purely additive; no existing assertions reference old printer column layout |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
