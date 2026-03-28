# Test Plan: [FEATURE_NAME]

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-[ISSUE_NUMBER]-v[VERSION]
**Feature**: [One-line description of what is being delivered]
**Version**: 1.0
**Created**: [YYYY-MM-DD]
**Author**: [Name]
**Status**: Draft | Approved | Active | Suspended | Complete
**Branch**: `[branch-name]`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

[Why this test plan exists. What problem does it solve? What confidence does it provide?
One paragraph.]

### 1.2 Objectives

[Measurable goals. An LLM or developer reading this section should know exactly what
"done" looks like for the test effort as a whole.]

1. **[Objective 1]**: [Measurable criterion — e.g., "All 15 Python Mock LLM scenarios produce identical responses in the Go implementation"]
2. **[Objective 2]**: [Measurable criterion]
3. **[Objective N]**: [Measurable criterion]

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/[service]/...` |
| Integration test pass rate | 100% | `go test ./test/integration/[service]/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- [BR-XXX-NNN]: [Business requirement title]
- [ADR-NNN / DD-XXX-NNN]: [Architecture/design decision title]
- Issue #[NNN]: [Issue title]

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.
>
> Risks are placed BEFORE test design because they should DRIVE which tests are written
> and at what priority. High-risk areas need more test coverage and higher-priority tests.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | [What could go wrong] | [Consequence if it happens] | High/Medium/Low | [Test IDs that mitigate this risk] | [How the risk is addressed in test design or implementation] |

### 3.1 Risk-to-Test Traceability

[For each High or Critical risk, identify the specific test(s) that provide coverage.
If a risk has no mitigating test, flag it as a coverage gap.]

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **[Component/module 1]** (`path/to/code`): [What is being tested and what business outcome it validates]
- **[Component/module 2]** (`path/to/code`): [What is being tested and what business outcome it validates]

### 4.2 Features Not to be Tested

- **[Component/module]**: [Why excluded — e.g., separate issue, deferred to future release, covered by other test plans]

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| [Key design choice for test strategy] | [Why this approach was chosen over alternatives] |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (pure logic: validators, parsers, builders, types, engines)
- **Integration**: >=80% of **integration-testable** code (I/O: HTTP handlers, DB adapters, filesystem, controller wiring)
- **E2E**: [>=80% of full service code OR "container contract only" OR "deferred" — explain]

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (typically UT + IT):
- **Unit tests**: Catch logic and correctness errors (fast feedback, isolated)
- **Integration tests**: Catch wiring, data fidelity, and behavior errors across component boundaries

If a tier is skipped, the rationale must be documented in Section 8 (Tier Skip Rationale).

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — behavior, correctness, and data accuracy — not
just code path coverage. Each test scenario answers: "what does the user/operator/system
get?" not "what function is called?"

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9** — When is this test plan considered passed or failed?

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions approved by reviewer
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing test suites that interact with the tested component
5. [Feature-specific pass criterion — e.g., "All 15 Python scenarios produce byte-identical JSON responses in Go"]

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail (regression)
4. [Feature-specific fail criterion]

### 5.5 Suspension & Resumption Criteria

> **IEEE 829 §10** — When should testing stop? When can it resume?

**Suspend testing when**:

- [Blocking dependency — e.g., "Issue #NNN has not landed and test X depends on it"]
- [Infrastructure unavailable — e.g., "Kind cluster cannot be provisioned"]
- [Build broken — "Code does not compile; unit tests cannot execute"]
- [Cascading failures — "More than N tests fail for the same root cause; stop and investigate"]

**Resume testing when**:

- [Blocking condition resolved — e.g., "#NNN merged and available on branch"]
- [Infrastructure restored]
- [Build fixed and green on CI]
- [Root cause identified and fix deployed]

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/[path]/[file].go` | `FunctionA`, `FunctionB` | ~NNN |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/[path]/[file].go` | `HandlerA`, `HandlerB` | ~NNN |
| `cmd/[service]/main.go` | Startup wiring | ~NNN |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.X` HEAD | [Branch or tag] |
| Dependency: [name] | [version or issue #] | [e.g., "Requires #548 merged"] |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-XXX-NNN | [Requirement description] | P0 | Unit | UT-SVC-NNN-001 | Pending |
| BR-XXX-NNN | [Requirement description] | P0 | Integration | IT-SVC-NNN-001 | Pending |

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

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: 2-4 char abbreviation (AA, DS, GW, MOCK, NOT, RO, SP, WE)
- **BR_NUMBER**: Business requirement number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: [List the files/packages and >=80% coverage target]

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SVC-NNN-001` | [What the user/operator/system gets when this works correctly] | Pending |

### Tier 2: Integration Tests

**Testable code scope**: [List the files/packages and >=80% coverage target]

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-SVC-NNN-001` | [What the user/operator/system gets when this works correctly] | Pending |

### Tier 3: E2E Tests (if applicable)

**Testable code scope**: [List what is validated end-to-end]

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-SVC-NNN-001` | [What the user/operator/system gets when this works correctly] | Pending |

### Tier Skip Rationale (if any tier is omitted)

- **[Tier]**: [Why this tier is not applicable or deferred, and what alternative coverage exists]

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.
>
> For large plans (>20 tests), provide detailed cases for P0 tests and summarize P1/P2.
> For individual test case format, see [Test Case Specification Template](TEST_CASE_SPECIFICATION_TEMPLATE.md).

### UT-SVC-NNN-001: [Short name]

**BR**: BR-XXX-NNN
**Priority**: P0
**Type**: Unit
**File**: `test/unit/[service]/[file]_test.go`

**Preconditions**:
- [System state required before test execution]

**Test Steps**:
1. **Given**: [Precondition / system state]
2. **When**: [Action or event]
3. **Then**: [Observable business outcome with measurable criteria]

**Expected Results**:
1. [Observable outcome with measurable criteria]
2. [Data accuracy assertion]

**Acceptance Criteria**:
- **Behavior**: [What the system does]
- **Correctness**: [What values/states are produced]
- **Accuracy**: [What data integrity is maintained]

**Dependencies**: [Other test IDs, issues, or services this depends on]

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: [External dependencies only — list what is mocked and why]
- **Location**: `test/unit/[service]/`
- **Resources**: [CPU/memory requirements, if any]

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: [What real services are needed: httptest, envtest, PostgreSQL, etc.]
- **Location**: `test/integration/[service]/`
- **Resources**: [CPU/memory requirements, if any]

### 10.3 E2E Tests (if applicable)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: [Kind cluster, Docker, Podman, etc.]
- **Location**: `test/e2e/[service]/`
- **Resources**: [Cluster size, Docker daemon, disk space, etc.]

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | [version] | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Docker/Podman | [version] | Container tests (if applicable) |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #NNN | Code | [Open/Merged] | [Which tests are blocked] | [Stub/mock/skip strategy] |

### 11.2 Execution Order

[If tests must be implemented in a specific order due to dependencies or TDD sequencing,
document the recommended order here.]

1. **Phase 1**: [Core functionality tests — e.g., "Unit tests for DAG engine"]
2. **Phase 2**: [Integration tests that depend on Phase 1 code]
3. **Phase 3**: [E2E / container tests that depend on working image]

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/[issue]/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/[service]/` | Ginkgo BDD test files |
| Integration test suite | `test/integration/[service]/` | Ginkgo BDD test files |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/[service]/... -ginkgo.v

# Integration tests
go test ./test/integration/[service]/... -ginkgo.v

# Specific test by ID
go test ./test/unit/[service]/... -ginkgo.focus="UT-SVC-NNN"

# Coverage
go test ./test/unit/[service]/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| [Test ID or file:line] | [What it currently asserts] | [What it needs to assert after the change] | [Why the change is needed] |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | [YYYY-MM-DD] | Initial test plan |
