# Test Plan: Workflow Discovery Case-Insensitive Label Matching

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-595-v1.0
**Feature**: Case-insensitive environment/severity/priority JSONB array matching in workflow discovery
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc2`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for Issue #595: the PostgreSQL JSONB `?` operator used for
`environment` and `severity` label matching in workflow discovery is case-sensitive, causing
valid workflows to be silently filtered out when query values differ in case from stored values.
The fix normalizes all JSONB array label filters to case-insensitive matching. This plan ensures
the fix is correct, complete, and introduces no regressions.

### 1.2 Objectives

1. **Case-insensitive environment matching**: Workflows stored with `["production"]` are discovered when queried with `Production`, `PRODUCTION`, or `production`
2. **Case-insensitive severity matching**: Workflows stored with `["critical"]` are discovered when queried with `Critical`, `CRITICAL`, or `critical`
3. **Case-insensitive priority array matching**: Workflows stored with `["P0"]` in the array branch are discovered when queried with `p0`, `P0`, or `p0`
4. **Wildcard preservation**: Workflows stored with `["*"]` wildcards continue to match any query value
5. **No regressions**: All existing discovery tests continue to pass without modification

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/datastorage/repository/workflow/... -v` |
| Integration test pass rate | 100% | `go test ./test/integration/datastorage/... -ginkgo.focus="595"` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `buildContextFilterSQL` |
| Backward compatibility | 0 regressions | Existing `discovery_filter_test.go` and `workflow_discovery_repository_test.go` pass |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-HAPI-017-001**: Three-Step Tool Implementation (workflow discovery)
- **DD-WORKFLOW-016**: Action-Type Workflow Catalog Indexing
- **DD-WORKFLOW-001 v2.5/v2.8**: Environment/severity JSONB array filters with wildcard support
- **Issue #595**: DataStorage: workflow discovery environment/severity filters are case-sensitive
- **demo-scenarios#267**: v1.2.0-rc1 scenario testing failure that discovered the bug

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Issue #464 Test Plan](../464/TEST_PLAN.md) — prior wildcard fix in the same function

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `EXISTS` subquery degrades query performance | Slower workflow discovery for HAPI | Low | IT-DS-595-003/004 | JSONB arrays are 2-5 elements; negligible overhead. Integration tests validate execution time implicitly |
| R2 | Wildcard `*` matching breaks after SQL refactor | All-wildcard workflows no longer match | Medium | UT-DS-595-005, IT-DS-595-005 | Explicit wildcard test cases in both tiers |
| R3 | Priority scalar branch regresses (ELSE path) | Priority filter silently breaks for scalar-stored values | Low | UT-DS-595-006 | Test validates scalar branch unchanged |
| R4 | Existing integration tests break due to SQL change | False regressions | Low | IT-DS-017-001-* | Existing tests use matching case; SQL change is transparent |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by IT-DS-595-003, IT-DS-595-004 (prove query returns results against real DB)
- **R2**: Mitigated by UT-DS-595-005 (SQL structure), IT-DS-595-005 (DB execution with wildcards)
- **R3**: Mitigated by UT-DS-595-006 (priority scalar branch SQL preserved)
- **R4**: Mitigated by running full existing test suite

---

## 4. Scope

### 4.1 Features to be Tested

- **`buildContextFilterSQL`** (`pkg/datastorage/repository/workflow/discovery.go:267-365`): Core filter SQL generation — environment, severity, and priority array branches must produce case-insensitive SQL
- **`ListActions`** (`discovery.go:46`): Step 1 discovery — validates case-insensitive filters propagate through to action type counts
- **`ListWorkflowsByActionType`** (`discovery.go:129`): Step 2 discovery — validates case-insensitive filters propagate through to workflow results
- **`GetWorkflowWithContextFilters`** (`discovery.go:217`): Step 3 discovery — validates security gate respects case-insensitive matching

### 4.2 Features Not to be Tested

- **Component filter**: Already case-insensitive (uses `LOWER()` on both sides) — no change needed
- **DetectedLabels filter**: Boolean and string fields not affected (exact match semantics correct)
- **Scoring SQL** (`scoring.go`): Not affected by this change
- **REST handler layer** (`workflow_handlers.go`): Integration tier; no logic change there

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Fix in `buildContextFilterSQL` only (not callers) | DataStorage is the authority for label matching per issue #595; consumers should not normalize |
| Use `EXISTS (SELECT 1 FROM jsonb_array_elements_text(...) WHERE LOWER(elem) = LOWER($N))` | PostgreSQL-idiomatic pattern for case-insensitive JSONB array containment |
| Harden severity and priority proactively | Severity matches by coincidence today (both lowercase); priority array branch has identical vulnerability |
| Keep wildcard `?` with literal `'*'` unchanged | `*` is case-insensitive by nature; no need for LOWER() on wildcard check |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `buildContextFilterSQL` (pure SQL generation logic, ~100 lines)
- **Integration**: >=80% of discovery repository methods exercised with case-mismatch scenarios against real PostgreSQL

### 5.2 Two-Tier Minimum

Every test scenario is covered by at least 2 tiers:
- **Unit tests**: Validate generated SQL contains `LOWER(elem) = LOWER($N)` and `jsonb_array_elements_text` patterns
- **Integration tests**: Validate actual PostgreSQL execution returns correct workflows with mixed-case inputs

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "A workflow registered with `environment: ["production"]` is discoverable when HAPI queries with `environment=Production`"
- Not: "the SQL string contains LOWER"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage >=80% on `buildContextFilterSQL`
4. All existing `discovery_filter_test.go` and `workflow_discovery_repository_test.go` tests pass

**FAIL** — any of the following:

1. Any P0 test fails
2. Existing tests regress
3. Wildcard matching broken

### 5.5 Suspension & Resumption Criteria

**Suspend**: PostgreSQL test infrastructure unavailable (integration tests), build broken
**Resume**: Infrastructure restored, build green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/discovery.go` | `buildContextFilterSQL` | ~100 (lines 267-365) |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/discovery.go` | `ListActions`, `ListWorkflowsByActionType`, `GetWorkflowWithContextFilters` | ~200 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc2` HEAD | Current working branch |
| PostgreSQL | 15+ | Required for `jsonb_array_elements_text` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-017-001 | Environment case-insensitive SQL generation | P0 | Unit | UT-DS-595-001 | Pending |
| BR-HAPI-017-001 | Severity case-insensitive SQL generation | P0 | Unit | UT-DS-595-002 | Pending |
| BR-HAPI-017-001 | Priority array branch case-insensitive SQL generation | P0 | Unit | UT-DS-595-003 | Pending |
| BR-HAPI-017-001 | Combined filters case-insensitive SQL generation | P0 | Unit | UT-DS-595-004 | Pending |
| BR-HAPI-017-001 | Wildcard `*` still matches with new SQL pattern | P0 | Unit | UT-DS-595-005 | Pending |
| BR-HAPI-017-001 | Priority scalar branch unchanged | P1 | Unit | UT-DS-595-006 | Pending |
| BR-HAPI-017-001 | Environment case-insensitive matching against real DB | P0 | Integration | IT-DS-595-001 | Pending |
| BR-HAPI-017-001 | Severity case-insensitive matching against real DB | P0 | Integration | IT-DS-595-002 | Pending |
| BR-HAPI-017-001 | Priority array case-insensitive matching against real DB | P0 | Integration | IT-DS-595-003 | Pending |
| BR-HAPI-017-001 | All filters mixed-case matching against real DB | P0 | Integration | IT-DS-595-004 | Pending |
| BR-HAPI-017-001 | Wildcard workflows match any case against real DB | P0 | Integration | IT-DS-595-005 | Pending |
| BR-HAPI-017-001 | GetWorkflowWithContextFilters respects case-insensitive security gate | P1 | Integration | IT-DS-595-006 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-DS-595-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `buildContextFilterSQL` in `discovery.go` (>=80% coverage)
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-595-001` | Environment filter SQL uses case-insensitive JSONB array matching | Pending |
| `UT-DS-595-002` | Severity filter SQL uses case-insensitive JSONB array matching | Pending |
| `UT-DS-595-003` | Priority array branch SQL uses case-insensitive JSONB array matching | Pending |
| `UT-DS-595-004` | Combined filters all produce case-insensitive SQL | Pending |
| `UT-DS-595-005` | Wildcard `*` fallback preserved in all JSONB array filters | Pending |
| `UT-DS-595-006` | Priority scalar branch (ELSE) remains case-sensitive exact match | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `ListActions`, `ListWorkflowsByActionType`, `GetWorkflowWithContextFilters` against real PostgreSQL
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-595-001` | Workflow with `environment: ["production"]` is found when queried with `Production` (PascalCase) | Pending |
| `IT-DS-595-002` | Workflow with `severity: ["critical"]` is found when queried with `Critical` (PascalCase) | Pending |
| `IT-DS-595-003` | Workflow with `priority: ["P0"]` (array) is found when queried with `p0` (lowercase) | Pending |
| `IT-DS-595-004` | Workflow with all-lowercase labels is found when queried with all-PascalCase filters | Pending |
| `IT-DS-595-005` | Workflow with wildcard `["*"]` labels matches any mixed-case query value | Pending |
| `IT-DS-595-006` | `GetWorkflowWithContextFilters` security gate matches case-insensitively | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — the fix is in SQL generation only; integration tests with real PostgreSQL provide sufficient DB-level validation. Full E2E would require deploying DataStorage + HAPI + Signal Processing, which is disproportionate for this surgical fix.

---

## 9. Test Cases

### UT-DS-595-001: Environment filter uses case-insensitive matching

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

**Test Steps**:
1. **Given**: A `WorkflowDiscoveryFilters` with `Environment: "Production"` (PascalCase)
2. **When**: `buildContextFilterSQL` is called
3. **Then**: The returned SQL contains `jsonb_array_elements_text(labels->'environment')` and `LOWER(elem) = LOWER($N)`, NOT `labels->'environment' ? $N`

**Acceptance Criteria**:
- **Behavior**: SQL fragment enables case-insensitive environment matching
- **Correctness**: SQL uses EXISTS/jsonb_array_elements_text/LOWER pattern
- **Accuracy**: Argument value is preserved as-is (normalization happens in SQL, not in Go)

---

### UT-DS-595-002: Severity filter uses case-insensitive matching

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

**Test Steps**:
1. **Given**: A `WorkflowDiscoveryFilters` with `Severity: "Critical"` (PascalCase)
2. **When**: `buildContextFilterSQL` is called
3. **Then**: The returned SQL contains `jsonb_array_elements_text(labels->'severity')` and `LOWER(elem) = LOWER($N)`, NOT `labels->'severity' ? $N`

**Acceptance Criteria**:
- **Behavior**: SQL fragment enables case-insensitive severity matching
- **Correctness**: SQL uses EXISTS/jsonb_array_elements_text/LOWER pattern
- **Accuracy**: Argument value preserved; wildcard `? '*'` fallback retained

---

### UT-DS-595-003: Priority array branch uses case-insensitive matching

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

**Test Steps**:
1. **Given**: A `WorkflowDiscoveryFilters` with `Priority: "p0"` (lowercase)
2. **When**: `buildContextFilterSQL` is called
3. **Then**: The THEN (array) branch SQL contains `jsonb_array_elements_text(labels->'priority')` and `LOWER(elem) = LOWER($N)`, NOT `labels->'priority' ? $N`

**Acceptance Criteria**:
- **Behavior**: Priority array branch uses case-insensitive matching
- **Correctness**: CASE WHEN structure preserved; THEN branch uses EXISTS pattern
- **Accuracy**: ELSE (scalar) branch may remain as-is since scalar values are compared with `=`

---

### UT-DS-595-004: Combined filters all produce case-insensitive SQL

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

**Test Steps**:
1. **Given**: `WorkflowDiscoveryFilters` with `Severity: "Critical"`, `Component: "Deployment"`, `Environment: "Staging"`, `Priority: "p1"`
2. **When**: `buildContextFilterSQL` is called
3. **Then**: SQL contains case-insensitive patterns for severity, environment, and priority; component uses existing `LOWER()` pattern

**Acceptance Criteria**:
- **Behavior**: All four mandatory filters generate correct SQL
- **Correctness**: 4 args produced, no `?` operator on severity/environment
- **Accuracy**: Component filter unchanged (already correct)

---

### UT-DS-595-005: Wildcard `*` fallback preserved

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

**Test Steps**:
1. **Given**: `WorkflowDiscoveryFilters` with `Environment: "staging"`, `Severity: "high"`
2. **When**: `buildContextFilterSQL` is called
3. **Then**: SQL still contains `labels->'environment' ? '*'` and `labels->'severity' ? '*'` wildcard fallbacks

**Acceptance Criteria**:
- **Behavior**: Wildcard workflows continue to be discoverable
- **Correctness**: `? '*'` preserved alongside new `EXISTS` pattern
- **Accuracy**: Both exact match AND wildcard paths present in SQL

---

### UT-DS-595-006: Priority scalar branch unchanged

**BR**: BR-HAPI-017-001
**Priority**: P1
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

**Test Steps**:
1. **Given**: `WorkflowDiscoveryFilters` with `Priority: "P1"`
2. **When**: `buildContextFilterSQL` is called
3. **Then**: The ELSE (scalar) branch still uses `labels->>'priority' = $N` pattern

**Acceptance Criteria**:
- **Behavior**: Scalar priority matching preserved
- **Correctness**: ELSE branch uses `->>'priority' =` (text extraction), not `?` (containment)

---

### IT-DS-595-001: Environment case-insensitive matching against real DB

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

**Preconditions**:
- PostgreSQL running with `remediation_workflow_catalog` table
- Workflow created with `environment: ["production"]` (lowercase, as stored by DS webhook)

**Test Steps**:
1. **Given**: A workflow with `labels.environment = ["production"]` stored in DB
2. **When**: `ListWorkflowsByActionType` is called with filter `Environment: "Production"` (PascalCase, as from Signal Processing)
3. **Then**: The workflow IS returned in results (count > 0)

**Expected Results**:
1. Query returns exactly 1 workflow
2. Returned workflow matches the created workflow ID

---

### IT-DS-595-002: Severity case-insensitive matching against real DB

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

**Preconditions**:
- Workflow created with `severity: ["critical"]` (lowercase)

**Test Steps**:
1. **Given**: A workflow with `labels.severity = ["critical"]` stored in DB
2. **When**: `ListWorkflowsByActionType` is called with filter `Severity: "Critical"` (PascalCase)
3. **Then**: The workflow IS returned in results

**Expected Results**:
1. Query returns exactly 1 workflow
2. Case mismatch does not cause silent filtering

---

### IT-DS-595-003: Priority array case-insensitive matching against real DB

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

**Preconditions**:
- Workflow created with `priority: ["P0", "P1"]` (JSONB array, uppercase)

**Test Steps**:
1. **Given**: A workflow with `labels.priority = ["P0", "P1"]` stored in DB
2. **When**: `ListWorkflowsByActionType` is called with filter `Priority: "p0"` (lowercase)
3. **Then**: The workflow IS returned in results

**Expected Results**:
1. Query returns exactly 1 workflow
2. Array containment check is case-insensitive

---

### IT-DS-595-004: All filters mixed-case matching against real DB

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

**Preconditions**:
- Workflow with all-lowercase labels: `severity: ["critical"]`, `component: "deployment"`, `environment: ["production"]`, `priority: "P0"`

**Test Steps**:
1. **Given**: A workflow with lowercase/mixed-case labels stored in DB
2. **When**: `ListActions` and `ListWorkflowsByActionType` called with PascalCase filters: `Severity: "Critical"`, `Component: "Deployment"`, `Environment: "Production"`, `Priority: "p0"`
3. **Then**: Both Step 1 and Step 2 discovery return the workflow

**Expected Results**:
1. `ListActions` returns action type with workflow count >= 1
2. `ListWorkflowsByActionType` returns the workflow
3. The exact reproduction scenario from Issue #595 is fixed

---

### IT-DS-595-005: Wildcard workflows match any mixed-case query

**BR**: BR-HAPI-017-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

**Preconditions**:
- Workflow with wildcard labels: `environment: ["*"]`, `severity: ["*"]`

**Test Steps**:
1. **Given**: An all-wildcard workflow stored in DB
2. **When**: `ListWorkflowsByActionType` called with `Environment: "Production"`, `Severity: "Critical"`
3. **Then**: The wildcard workflow IS returned

**Expected Results**:
1. Wildcard matching unbroken by SQL refactor
2. `["*"]` continues to match any query value regardless of case

---

### IT-DS-595-006: GetWorkflowWithContextFilters case-insensitive security gate

**BR**: BR-HAPI-017-001
**Priority**: P1
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

**Preconditions**:
- Workflow with `environment: ["production"]` stored in DB

**Test Steps**:
1. **Given**: A workflow with lowercase environment label
2. **When**: `GetWorkflowWithContextFilters` called with matching workflow ID but `Environment: "Production"` (PascalCase)
3. **Then**: The workflow IS returned (security gate passes)

**Expected Results**:
1. Security gate does not block case-mismatched queries
2. Returns non-nil workflow pointer

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Standard Go `testing` (existing pattern in `discovery_filter_test.go`)
- **Mocks**: None — `buildContextFilterSQL` is a pure function
- **Location**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks — real PostgreSQL
- **Infrastructure**: PostgreSQL 15+ (provides `jsonb_array_elements_text`)
- **Location**: `test/integration/datastorage/workflow_discovery_case_insensitive_test.go`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25 | Build and test |
| PostgreSQL | 15 | `jsonb_array_elements_text` support |
| Ginkgo CLI | v2.x | Integration test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all infrastructure (PostgreSQL, test suite) already exists.

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write failing unit tests (UT-DS-595-001 through UT-DS-595-006) and failing integration tests (IT-DS-595-001 through IT-DS-595-006)
2. **Phase 2 (GREEN)**: Fix `buildContextFilterSQL` — replace `?` with `EXISTS/jsonb_array_elements_text/LOWER` for environment, severity, and priority array branch
3. **Phase 3 (REFACTOR)**: Clean up SQL formatting, verify no dead code, ensure consistent pattern across all three JSONB array filters

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/595/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `pkg/datastorage/repository/workflow/discovery_filter_test.go` | New tests appended |
| Integration test suite | `test/integration/datastorage/workflow_discovery_case_insensitive_test.go` | New file |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/datastorage/repository/workflow/... -v -run "595"

# Integration tests (requires PostgreSQL)
go test ./test/integration/datastorage/... -ginkgo.v -ginkgo.focus="595"

# All discovery tests (regression check)
go test ./pkg/datastorage/repository/workflow/... -v
go test ./test/integration/datastorage/... -ginkgo.v -ginkgo.focus="Discovery"

# Coverage
go test ./pkg/datastorage/repository/workflow/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep buildContextFilterSQL
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `UT-DS-464-005` (`discovery_filter_test.go:246`) | Checks `labels->'environment' ? '*'` exists | None — wildcard path preserved | Wildcard fallback unchanged |
| `UT-DS-464-006` (`discovery_filter_test.go:263`) | Checks all 4 mandatory filters | May need update if SQL structure assertion checks for `?` on severity/environment | The `?` operator is replaced by `EXISTS` for exact-match path |

**Note**: Existing unit tests check SQL **structure** (string containment). Tests that assert `labels->'severity' ? $N` or `labels->'environment' ? $N` will need updating since those patterns are being replaced. Specifically:

- `TestBuildContextFilterSQL_Issue464_EnvironmentWildcard` — asserts `labels->'environment' ? '*'` (unchanged, still passes)
- `TestBuildContextFilterSQL_Issue464_AllMandatoryWildcards` — asserts `labels->'severity' ? '*'` (unchanged, still passes)
- **No existing test asserts the exact-match `?` pattern** — they only check wildcard `? '*'`, so no regressions expected

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
