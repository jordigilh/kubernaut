# Test Plan: Conditional Pagination Stripping for Discovery Tools

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-688-v1.0
**Feature**: Strip pagination metadata from tool responses when all results fit in one page
**Version**: 1.0
**Created**: 2026-04-14
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/684-vertex-ai-claude`

---

## 1. Introduction

### 1.1 Purpose

Validate that the KA discovery tools (`list_available_actions`, `list_workflows`) conditionally strip the `pagination` envelope from DataStorage responses when all results fit in a single page (`hasMore: false`). This prevents the LLM from wasting tool calls attempting to paginate, saving tokens and tool budget while preserving pagination metadata when results genuinely span multiple pages.

### 1.2 Objectives

1. **Pagination stripped when complete**: When DataStorage returns `hasMore: false`, the `pagination` field is removed from the tool output returned to the LLM.
2. **Pagination preserved when incomplete**: When DataStorage returns `hasMore: true`, the `pagination` field is preserved so the LLM knows it is seeing a subset.
3. **Schema honesty**: `list_workflows` no longer advertises `offset`/`limit` parameters that `Execute()` never honors.
4. **Zero regressions**: Existing tool behavior (data fields, error handling) is unchanged.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/tools/custom/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `StripPaginationIfComplete` |
| Backward compatibility | 0 regressions | Existing custom tools tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #688: `list_available_actions` pagination broken: hasMore=true but offset parameter ignored
- DD-WORKFLOW-016: Action type and workflow indexing design
- DD-HAPI-019-003: Security architecture (I7 anomaly detection tool budget)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Golden transcript gist: `81e59c8f9696581dcf9d4da383a6b3e2` (evidence of 3 wasted calls)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Stripping pagination when hasMore=true causes LLM to miss results | LLM never discovers workflows beyond first page | Low | UT-KA-688-001 | Explicit test: pagination preserved when hasMore=true |
| R2 | Malformed JSON from DataStorage causes panic | Tool returns empty/error instead of result | Low | UT-KA-688-003 | Defensive: invalid JSON returns input unchanged |
| R3 | Removing offset/limit from list_workflows schema breaks existing LLM prompts | LLM passes offset/limit, tool fails to parse | Low | UT-KA-433-171 | Execute already ignores args for these fields; schema now honest |
| R4 | Data fields altered during JSON re-marshal | Tool result loses precision or field ordering | Medium | UT-KA-688-001, UT-KA-688-002 | Tests verify all data fields preserved after stripping |

### 3.1 Risk-to-Test Traceability

- **R1** (Critical): UT-KA-688-001 "should preserve pagination when hasMore is true"
- **R2**: UT-KA-688-003 "should return input unchanged for invalid JSON"
- **R3**: UT-KA-433-171 updated to verify offset/limit are absent
- **R4**: UT-KA-688-001, UT-KA-688-002 verify data arrays preserved

---

## 4. Scope

### 4.1 Features to be Tested

- **`StripPaginationIfComplete()`** (`pkg/kubernautagent/tools/custom/tools.go`): Conditional pagination removal based on `hasMore` field
- **`listActionsTool.Execute()`** wiring: Calls `StripPaginationIfComplete` on result
- **`listWorkflowsTool.Execute()`** wiring: Calls `StripPaginationIfComplete` on result
- **`listWorkflowsSchemaJSON`**: Removal of `offset`/`limit` parameters

### 4.2 Features Not to be Tested

- **DataStorage server pagination** (`HandleListAvailableActions`, `ParsePagination`): Unchanged, covered by existing DS tests
- **Ogen client**: Generated code, not modified
- **`get_workflow` tool**: Not affected by pagination changes

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Conditional strip vs always strip | Preserves LLM's ability to see pagination metadata when results genuinely span multiple pages |
| Pure function vs method on tool | `StripPaginationIfComplete` is an exported standalone function for direct unit testing without DS mock |
| Remove offset/limit from schema | The tool never honored these parameters; advertising them caused the LLM to waste calls trying to paginate |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `StripPaginationIfComplete` function (pure logic, all branches)
- **Integration**: Deferred — pagination stripping is pure JSON transformation with no I/O boundary
- **E2E**: Covered implicitly by existing demo scenario (crashloop)

### 5.2 Two-Tier Minimum

Unit tests cover the pure logic function. Integration-tier coverage is provided by the existing custom tools integration tests which exercise `Execute()` against a real DS httptest server.

### 5.3 Pass/Fail Criteria

**PASS** — all of the following:
1. All UT-KA-688-* tests pass
2. All existing UT-KA-433-17* tests pass (updated assertions)
3. >=80% branch coverage on `StripPaginationIfComplete`
4. No regressions in `test/unit/kubernautagent/tools/custom/`
5. `go build ./...` succeeds

**FAIL** — any of the following:
1. Any UT-KA-688-* test fails
2. Existing tests regress
3. Coverage below 80% on the new function

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/custom/tools.go` | `StripPaginationIfComplete` | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/custom/tools.go` | `listActionsTool.Execute`, `listWorkflowsTool.Execute` | ~20 each |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/684-vertex-ai-claude` HEAD | Commit `619d10f0b` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #688 | Strip pagination when hasMore=false (action types) | P0 | Unit | UT-KA-688-001 | Pass |
| #688 | Preserve pagination when hasMore=true | P0 | Unit | UT-KA-688-001 | Pass |
| #688 | Strip pagination when hasMore=false (workflows) | P0 | Unit | UT-KA-688-002 | Pass |
| #688 | Defensive: absent pagination field | P1 | Unit | UT-KA-688-003 | Pass |
| #688 | Defensive: invalid JSON input | P1 | Unit | UT-KA-688-003 | Pass |
| #688 | Schema honesty: no offset/limit in list_workflows | P0 | Unit | UT-KA-433-171 | Pass |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-KA-688-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `pkg/kubernautagent/tools/custom/tools.go` — `StripPaginationIfComplete` (>=80% branch coverage)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-688-001` | Pagination stripped when complete (hasMore=false), preserved when incomplete (hasMore=true) | Pass |
| `UT-KA-688-002` | Pagination stripped from workflow discovery responses when complete | Pass |
| `UT-KA-688-003` | Defensive edge cases: absent pagination, invalid JSON | Pass |

### Tier Skip Rationale

- **Integration**: `StripPaginationIfComplete` is a pure JSON transformation with no I/O. The wiring (calling it from `Execute`) is a one-line change exercised by existing integration tests. No new integration tests needed.
- **E2E**: Pagination behavior validated end-to-end by demo team's crashloop scenario in Kind.

---

## 9. Test Cases

### UT-KA-688-001: Conditional pagination stripping for action type responses

**BR**: #688
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**: None (pure function test)

**Test Steps**:
1. **Given**: JSON response with `hasMore: false`, `totalCount: 2`, `limit: 10`, and 2 action types
2. **When**: `StripPaginationIfComplete` is called
3. **Then**: Returned JSON contains `actionTypes` but NOT `pagination`

**Additional case**:
1. **Given**: JSON response with `hasMore: true`, `totalCount: 16`, `limit: 10`, and 10 action types
2. **When**: `StripPaginationIfComplete` is called
3. **Then**: Returned JSON contains BOTH `actionTypes` AND `pagination`

### UT-KA-688-002: Conditional pagination stripping for workflow responses

**BR**: #688
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**: None (pure function test)

**Test Steps**:
1. **Given**: JSON response with `hasMore: false`, `actionType: "RestartDeployment"`, and 1 workflow
2. **When**: `StripPaginationIfComplete` is called
3. **Then**: Returned JSON contains `workflows` and `actionType` but NOT `pagination`

### UT-KA-688-003: Defensive edge cases

**BR**: #688
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/custom/custom_tools_test.go`

**Preconditions**: None (pure function test)

**Test Steps (absent pagination)**:
1. **Given**: JSON response with no `pagination` field
2. **When**: `StripPaginationIfComplete` is called
3. **Then**: Input returned unchanged

**Test Steps (invalid JSON)**:
1. **Given**: Non-JSON string input
2. **When**: `StripPaginationIfComplete` is called
3. **Then**: Input returned unchanged (no panic)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — `StripPaginationIfComplete` is a pure function
- **Location**: `test/unit/kubernautagent/tools/custom/`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all code is already committed.

### 11.2 Execution Order

#### Phase 1: TDD RED
Write all failing tests (UT-KA-688-001 through UT-KA-688-003). Verify they fail for the correct reason (undefined function or wrong assertion).

**Checkpoint 1**: All tests fail as expected, no regressions in existing suite.

#### Phase 2: TDD GREEN
Implement `StripPaginationIfComplete` and wire into both tools. Minimal implementation to pass all tests.

**Checkpoint 2**: All tests pass, `go build ./...` clean, zero lint errors. Adversarial audit: data integrity, defensive behavior, no false stripping.

#### Phase 3: TDD REFACTOR
Review for dead code, naming clarity, documentation. Remove `offset`/`limit` from `list_workflows` schema. Update existing test assertions.

**Checkpoint 3**: All suites pass, lint clean, schema honesty verified, no dead code.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/688/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/tools/custom/custom_tools_test.go` | 5 new Ginkgo BDD specs |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/tools/custom/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/tools/custom/... -ginkgo.focus="UT-KA-688"

# Coverage
go test ./test/unit/kubernautagent/tools/custom/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-KA-433-171 | Asserts `offset` and `limit` in list_workflows schema | Assert `offset` and `limit` are NOT in schema | Schema updated: tool never honored these params (#688) |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-14 | Initial test plan. All phases (RED/GREEN/REFACTOR) complete. |
