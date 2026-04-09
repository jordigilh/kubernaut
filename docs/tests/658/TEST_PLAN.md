# Test Plan: WorkflowQuerier HTTP 500 Error Classification

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-658-v1
**Feature**: Correct classification of Data Storage (DS) responses in `OgenWorkflowQuerier` so HTTP 500 is never reported as catalog/workflow "not found"
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3` (or feature branch for #658)

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan exists to prevent misleading operator and automation behavior when DS returns HTTP 500 for workflow catalog queries. Today, five methods in `OgenWorkflowQuerier` use type assertions that misclassify `*GetWorkflowByIDInternalServerError` as a "not found" style outcome, corrupting error taxonomy used downstream (audit codes, Kubernetes event reasons, and `MarkFailedWithReason` classification). The plan defines unit-level scenarios and TDD phases so fixes are provably correct without relying on integration I/O.

### 1.2 Objectives

1. **Correct 500 handling**: When DS returns ogen type `*GetWorkflowByIDInternalServerError`, every affected querier method returns an error that clearly indicates server/catalog failure, not "workflow not found."
2. **Preserved 404 semantics**: When DS returns `*GetWorkflowByIDNotFound`, errors continue to indicate that the workflow/catalog entry was not found.
3. **Unexpected type safety**: Any response type outside the expected 200/404/500 sum-type branches yields an explicit `unexpected response type %T` error.
4. **Deduplication guard**: If a central helper is introduced in REFACTOR, all five code paths use the same classification logic (single source of truth).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/workflowexecution/...` (focus on WorkflowQuerier / #658 scenarios) |
| Integration test pass rate | N/A (skipped) | See Section 8 — Tier Skip Rationale |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/workflowexecution/client/` (querier-focused) |
| Integration-testable code coverage | N/A | Not in scope for this issue |
| Backward compatibility | 0 regressions | Existing `workflow_querier_test.go` and related suites pass |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- DD-WE-006: Catalog query correctness (workflow metadata resolution from DS)
- DD-API-001: Typed OpenAPI (ogen) client usage for DS communication
- Issue #658: WorkflowQuerier misleading error classification when DS returns HTTP 500

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
| R1 | Downstream components treat "not found" differently from "server error" | Wrong remediation, masked outages, incorrect audit | High | UT-WE-658-001..006 | Assert error strings/wrapping or sentinel semantics consistent with server failure, not `IsNotFound`-style messaging |
| R2 | Fix only one of five methods | Partial misclassification remains | Medium | UT-WE-658-007 | Table or shared helper test; exercise all entry points |
| R3 | ogen sum-type extended in future | New branch unhandled | Low | UT-WE-658-008 | Default branch in type switch with `%T` |
| R4 | Regression on 404 path | Operators see false "server" errors | Medium | UT-WE-658-002 | Explicit 404 scenario remains green |

### 3.1 Risk-to-Test Traceability

- **R1** → UT-WE-658-001, UT-WE-658-003, UT-WE-658-004, UT-WE-658-005, UT-WE-658-006 (500 paths per method family).
- **R2** → UT-WE-658-007 (central helper / shared classification).
- **R3** → UT-WE-658-008 (unexpected response type).
- **R4** → UT-WE-658-002 (404 unchanged).

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **OgenWorkflowQuerier** (`pkg/workflowexecution/client/workflow_querier.go`): Response handling for `GetWorkflowByID` (or equivalent) ogen union: `*RemediationWorkflow` (200), `*GetWorkflowByIDNotFound` (404), `*GetWorkflowByIDInternalServerError` (500), and default/unexpected types.
- **Production call sites (behavioral contract)**: Logic consumed by `resolveWorkflowCatalog` (Pending) and `resolveExecutionEngine` / related paths (Running, Terminal, Delete) must receive correctly classified errors (validated indirectly via querier unit tests).

### 4.2 Features Not to be Tested

- **DS HTTP server implementation**: External service; mocked via test doubles returning ogen response types.
- **Full controller reconcile loops**: Covered under separate integration/E2E plans; this issue is unit-testable classification logic.
- **ogen-generated client internals** (`pkg/agentclient/` ogen files): Generated code; not modified by this fix.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unit-only tier for #658 | Classification is pure mapping from ogen sum type to error; no real network or envtest required |
| RED uses mock returning `*GetWorkflowByIDInternalServerError` | Matches production ogen types; avoids stringly HTTP stubs |
| REFACTOR may introduce central helper | Removes five copy-paste type switches; UT-WE-658-007 locks behavior |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code in `pkg/workflowexecution/client/` relevant to `workflow_querier.go` (querier methods and any extracted helper).
- **Integration**: Skipped for this test plan — see Section 8 (Tier Skip Rationale).
- **E2E**: Not required for sum-type error mapping.

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least two test tiers where applicable. For this issue, **integration tier is explicitly skipped** with documented rationale (Section 8): the behavior under test is unit-testable pure logic with no I/O. **Dual coverage** is achieved via (1) per-method 500 tests and (2) regression 404 + unexpected-type tests, plus optional central-helper assertion.

### 5.3 Business Outcome Quality Bar

Tests validate **operator-observable error semantics**: a DS outage or internal error must not be indistinguishable from "workflow not in catalog." Assertions must reflect business meaning (server/catalog failure vs not found), not merely "an error occurred."

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9** — When is this test plan considered passed or failed?

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures): UT-WE-658-001 through UT-WE-658-006, UT-WE-658-008
2. UT-WE-658-007 passes if a central helper exists; if helper is deferred, document exception and ensure R2 mitigated by explicit tests per method
3. Per-tier unit coverage on querier package meets >=80% threshold
4. No regressions in existing `test/unit/workflowexecution/workflow_querier_test.go`
5. No use of `Skip()`, `time.Sleep` for synchronization, or NULL-only assertions (anti-patterns)

**FAIL** — any of the following:

1. Any P0 test fails
2. Unit coverage falls below 80% on the targeted package scope
3. Existing workflow querier tests fail without approved assertion updates (Section 14)
4. HTTP 500 still classified as "not found" in any of the five methods

### 5.5 Suspension & Resumption Criteria

> **IEEE 829 §10** — When should testing stop? When can it resume?

**Suspend testing when**:

- Code does not compile; unit tests cannot execute
- ogen types for `GetWorkflowByID` responses are renamed or regenerated incompatibly without updating tests
- More than three unrelated failures appear in the same run (investigate root cause)

**Resume testing when**:

- Build is green
- ogen regeneration and adapter updates are complete
- Root cause for cascading failures is fixed

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `pkg/workflowexecution/client/workflow_querier.go` | `ResolveWorkflowCatalogMetadata`, `GetWorkflowExecutionEngine`, `GetWorkflowExecutionBundle`, `GetWorkflowDependencies`, `GetWorkflowEngineConfig`, plus any `classifyGetWorkflowByIDResponse`-style helper | ~291 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| — | Not in scope for TP-658 | — |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | Branch HEAD for #658 | After GREEN/REFACTOR merge target |
| ogen response types | `*RemediationWorkflow`, `*GetWorkflowByIDNotFound`, `*GetWorkflowByIDInternalServerError` | As generated in `pkg/agentclient/` |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR / DD ID | Description | Priority | Tier | Test ID | Status |
|------------|-------------|----------|------|---------|--------|
| DD-WE-006 | Catalog query correctness | P0 | Unit | UT-WE-658-001, UT-WE-658-002 | Pending |
| DD-API-001 | Typed ogen client for DS | P0 | Unit | UT-WE-658-001..006, UT-WE-658-008 | Pending |
| DD-WE-006 | Engine/bundle/deps/config query correctness | P0 | Unit | UT-WE-658-003..006 | Pending |
| (Maintainability) | Single classification helper | P1 | Unit | UT-WE-658-007 | Pending |

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

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` (issue-scoped plan; aligns with traceability to #658)

- **TIER**: `UT` (Unit)
- **SERVICE**: `WE` (WorkflowExecution)
- **ISSUE**: `658`
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/workflowexecution/client/workflow_querier.go` — >=80% coverage of querier package (unit-testable subset).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-WE-658-001 | `ResolveWorkflowCatalogMetadata` + DS 500 → error indicates server/catalog failure, **not** "not found" | Pending |
| UT-WE-658-002 | `ResolveWorkflowCatalogMetadata` + DS 404 → error correctly indicates not found | Pending |
| UT-WE-658-003 | `GetWorkflowExecutionEngine` + DS 500 → server error classification | Pending |
| UT-WE-658-004 | `GetWorkflowExecutionBundle` + DS 500 → server error classification | Pending |
| UT-WE-658-005 | `GetWorkflowDependencies` + DS 500 → server error classification | Pending |
| UT-WE-658-006 | `GetWorkflowEngineConfig` + DS 500 → server error classification | Pending |
| UT-WE-658-007 | Central helper deduplication (if implemented) — all methods use same classification | Pending |
| UT-WE-658-008 | Unexpected response type → error containing `unexpected response type` and `%T` | Pending |

### Tier 2: Integration Tests

**Testable code scope**: None for this plan.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| — | — | — |

### Tier 3: E2E Tests (if applicable)

**Testable code scope**: Not applicable.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| — | — | — |

### Tier Skip Rationale (if any tier is omitted)

- **Integration**: Skipped. Error classification is **unit-testable pure logic**: mock client returns concrete ogen pointer types; no real DS, no envtest, no HTTP integration needed. Blast radius (audit/events) is indirect and covered by controller plans when behavior changes propagate.
- **E2E**: Skipped. Not required for sum-type mapping correctness.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-WE-658-001: ResolveWorkflowCatalogMetadata — DS 500

**BR / DD**: DD-WE-006, DD-API-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**:

- Test double implements workflow querier dependency such that `GetWorkflowByID` returns `*GetWorkflowByIDInternalServerError` (or equivalent ogen 500 type).

**Test Steps**:

1. **Given**: Mock returns internal server error response type for catalog ID lookup
2. **When**: `ResolveWorkflowCatalogMetadata` is invoked
3. **Then**: Returned error MUST NOT assert or imply "not found" / missing catalog as the primary classification; it MUST indicate server/catalog retrieval failure

**Expected Results**:

1. Error is non-nil
2. Error message or wrapped cause is consistent with internal/server failure semantics (not not-found wording used for 404 path)

**Acceptance Criteria**:

- **Behavior**: Callers can distinguish outage/server failure from missing workflow
- **Correctness**: 500 ogen branch is handled explicitly in type switch (or helper)
- **Accuracy**: No conflation with 404 handling

**Dependencies**: ogen types available in test package; existing test patterns in `workflow_querier_test.go`

---

### UT-WE-658-002: ResolveWorkflowCatalogMetadata — DS 404

**BR / DD**: DD-WE-006
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**:

- Mock returns `*GetWorkflowByIDNotFound`

**Test Steps**:

1. **Given**: DS indicates workflow ID not found
2. **When**: `ResolveWorkflowCatalogMetadata` is invoked
3. **Then**: Error indicates not found (existing behavior preserved)

**Expected Results**:

1. Error is non-nil
2. Semantics match pre-fix 404 behavior (regression guard)

**Acceptance Criteria**:

- **Behavior**: Missing catalog entry still reported as not found
- **Correctness**: 404 branch unchanged aside from shared refactor

**Dependencies**: UT-WE-658-001 (ordering optional; logically independent)

---

### UT-WE-658-003: GetWorkflowExecutionEngine — DS 500

**BR / DD**: DD-WE-006, DD-API-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**: Mock returns `*GetWorkflowByIDInternalServerError`

**Test Steps**:

1. **Given** / **When** / **Then**: Same pattern as UT-WE-658-001 for `GetWorkflowExecutionEngine`

**Expected Results**: Server error classification; not "not found"

**Acceptance Criteria**: Align with UT-WE-658-001 criteria

**Dependencies**: None

---

### UT-WE-658-004: GetWorkflowExecutionBundle — DS 500

**BR / DD**: DD-WE-006, DD-API-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**: Mock returns `*GetWorkflowByIDInternalServerError`

**Test Steps**: Invoke `GetWorkflowExecutionBundle`; assert server error classification

**Expected Results**: Non-nil error; not misclassified as not found

**Acceptance Criteria**: Same as UT-WE-658-001

**Dependencies**: None

---

### UT-WE-658-005: GetWorkflowDependencies — DS 500

**BR / DD**: DD-WE-006, DD-API-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**: Mock returns `*GetWorkflowByIDInternalServerError`

**Test Steps**: Invoke `GetWorkflowDependencies`; assert server error classification

**Expected Results**: Non-nil error; not misclassified as not found

**Acceptance Criteria**: Same as UT-WE-658-001

**Dependencies**: None

---

### UT-WE-658-006: GetWorkflowEngineConfig — DS 500

**BR / DD**: DD-WE-006, DD-API-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**: Mock returns `*GetWorkflowByIDInternalServerError`

**Test Steps**: Invoke `GetWorkflowEngineConfig`; assert server error classification

**Expected Results**: Non-nil error; not misclassified as not found

**Acceptance Criteria**: Same as UT-WE-658-001

**Dependencies**: None

---

### UT-WE-658-007: Central helper — single classification (REFACTOR)

**BR / DD**: Maintainability / DD-API-001
**Priority**: P1
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go` (or `_test` in same package if testing unexported helper)

**Preconditions**: REFACTOR introduces shared function (e.g. `classifyGetWorkflowByIDResponse`)

**Test Steps**:

1. **Given**: Helper used by all five methods
2. **When**: Each public method receives 500 / 404 / success via mock
3. **Then**: All paths delegate to same classification outcomes

**Expected Results**: No drift between methods; optional direct unit tests on helper if exported or `export_test` pattern used per project conventions

**Acceptance Criteria**: DRY compliance without behavior change

**Dependencies**: GREEN phase complete

---

### UT-WE-658-008: Unexpected response type

**BR / DD**: DD-API-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**: Mock returns a value that is not 200/404/500 expected types (e.g. wrong concrete type or nil wrapper per test design)

**Test Steps**:

1. **Given**: Unexpected type from client
2. **When**: Any of the five methods processes the response
3. **Then**: Error matches pattern `unexpected response type %T` (or project-standard equivalent)

**Expected Results**: Fail-fast, debuggable error

**Acceptance Criteria**: Default branch of type switch is covered

**Dependencies**: None

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock workflow/DS client interface returning ogen-generated response pointers only (external dependency boundary)
- **Location**: `test/unit/workflowexecution/workflow_querier_test.go`
- **Resources**: Standard developer machine

### 10.2 Integration Tests

- **Framework**: N/A (tier skipped)
- **Mocks**: N/A
- **Infrastructure**: N/A
- **Location**: N/A

### 10.3 E2E Tests (if applicable)

- Not applicable.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Project `go.mod` | Build and test |
| Ginkgo CLI | v2.x | BDD test runner |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| ogen types for GetWorkflowByID | Code | Available on branch | Cannot compile RED tests | Regenerate client from OpenAPI |
| Issue #658 implementation branch | Code | Open | Tests target wrong behavior | Rebase after merge |

### 11.2 Execution Order (TDD)

1. **RED**: Add tests UT-WE-658-001..006, UT-WE-658-008 using mock returning `*GetWorkflowByIDInternalServerError` and unexpected types; confirm failures on current misclassification
2. **GREEN**: Implement type switch (or helper) distinguishing 200 / 404 / 500 / default
3. **REFACTOR**: Deduplicate five blocks into central helper; add UT-WE-658-007
4. **Check**: Coverage >=80% on `pkg/workflowexecution/client` for querier scope; full unit package regression

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/658/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/workflowexecution/workflow_querier_test.go` | Ginkgo BDD additions for #658 |
| Coverage report | CI artifact / local `coverage.out` | Per-tier coverage for client package |

---

## 13. Execution

```bash
# Unit tests — workflow querier suite
go test ./test/unit/workflowexecution/... -ginkgo.v

# Focus by issue/scenario description (adjust Describe/Context strings to match implementation)
go test ./test/unit/workflowexecution/... -ginkgo.focus="658" -ginkgo.v

# Coverage (package under test)
go test ./pkg/workflowexecution/client/... -coverprofile=/tmp/we_client.cover.out
go tool cover -func=/tmp/we_client.cover.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `workflow_querier_test.go` (500 paths if any) | May expect generic error or not cover 500 | Assert explicit server-error semantics | Align with new classification |
| Any test assuming 500 → "not found" | Misaligned | Update to expect server/catalog failure | Correct business semantics |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #658 |
