# Test Plan: Wildcard Mandatory Label Matching in Workflow Discovery

**Feature**: DataStorage workflow discovery returns 0 results when workflows use wildcard (`*`) mandatory labels
**Version**: 2.0
**Created**: 2026-03-04
**Updated**: 2026-03-20
**Author**: AI Assistant
**Status**: Implemented
**Branch**: `development/v1.2` (backport to `fix/1.1.0-rc3`)

**Authority**:
- [BR-HAPI-017-001]: Three-Step Workflow Discovery Tool Implementation
- [DD-WORKFLOW-001 v2.8]: Mandatory Workflow Label Schema — wildcard support for severity, component, environment, priority
- [DD-WORKFLOW-016]: Action-Type Workflow Catalog Indexing

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: #464

---

## 0. Root Cause Analysis

### Reported Symptom

The demo team reported that workflow discovery returned 0 results when workflows used wildcard (`*`) mandatory labels during rc2 validation.

### Actual Root Cause

The demo team identified the root cause as a **PostgreSQL image change from the Kind-based image to the OpenShift (OCP) version**, which uses a different default data directory. This caused the persistent volume mount to no longer align with the database's expected data path, effectively wiping the database on pod restart. The seeded workflow data was lost, resulting in 0 results from discovery queries — not a logic bug in wildcard matching.

### Defensive Fixes Found During Triage

Code review during triage identified two minor code quality issues unrelated to the root cause:

1. **Priority array branch missing wildcard fallback** (`buildContextFilterSQL`): The `CASE WHEN jsonb_typeof(labels->'priority') = 'array' THEN ...` branch only had `labels->'priority' ? $N` but was missing `OR labels->'priority' ? '*'`, making it inconsistent with the scalar branch. While `MandatoryLabels.Priority` is `string` today (not stored as array), the SQL should be defensive per DD-WORKFLOW-016 v2.1.

2. **`ValidateMandatoryLabels` rejecting `severity: ["*"]`**: DD-WORKFLOW-001 v2.8 explicitly restores `"*"` as a valid wildcard for severity, but the `allowedSeverities` map did not include `"*"`, causing schema validation to reject legitimate wildcard workflows on registration.

### Test Gap

Existing integration tests for workflow discovery (`IT-DS-017-001-001` through `IT-DS-017-001-006`) used only exact label values. Zero wildcard scenarios existed. This is the test gap that would have provided confidence about wildcard matching before rc2.

---

## 1. Scope

### In Scope

- **`buildContextFilterSQL` — priority array branch fix**: Add `OR labels->'priority' ? '*'` to the array CASE branch for defensive wildcard matching.
- **`ValidateMandatoryLabels` — severity wildcard acceptance**: Add `"*"` to the `allowedSeverities` map per DD-WORKFLOW-001 v2.8.
- **Integration test coverage gap**: 6 new integration tests validating wildcard label matching against real PostgreSQL for all three discovery steps (`ListActions`, `ListWorkflowsByActionType`, `GetWorkflowWithContextFilters`).
- **Unit test coverage**: 5 new SQL generation unit tests + 4 new validation unit tests confirming wildcard support.

### Out of Scope

- **PostgreSQL image fix**: The container image revert (`17-alpine` → `16-alpine`) and mount path correction are infrastructure changes handled separately in the Helm chart.
- **HAPI client-side logic**: The Python toolset (`workflow_discovery.py`) correctly sends query parameters; no code changes needed.
- **Detected labels wildcard matching**: Already tested in `discovery_filter_test.go` and `scoring_test.go`. Not part of this issue.
- **Custom labels**: Not referenced in the issue; existing coverage adequate.
- **Authwebhook registration path**: CRD → DataStorage registration is working (workflows are stored; the issue is query-side).

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Fix priority array branch in `buildContextFilterSQL` | The array CASE branch is missing `OR labels->'priority' ? '*'`, making it inconsistent with the scalar branch. Defensive fix per DD-WORKFLOW-016 v2.1. |
| Fix `ValidateMandatoryLabels` to accept `"*"` for severity | DD-WORKFLOW-001 v2.8 explicitly restores `"*"` wildcard for severity. The validator must align. |
| Integration tests use real PostgreSQL | Per no-mocks policy. Wildcard matching relies on PostgreSQL JSONB `?` operator and `jsonb_typeof()` behavior, which cannot be validated by SQL string assertions alone. This is exactly the gap that allowed confidence issues before rc2. |
| Skip UT-DS-464-004 (severity SQL regression) | Existing `TestBuildContextFilterSQL_SeverityWildcard` already covers this. Adding a duplicate would not increase coverage. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`buildContextFilterSQL` SQL generation logic, `ValidateMandatoryLabels`)
- **Integration**: >=80% of integration-testable code (`ListActions`, `ListWorkflowsByActionType`, `GetWorkflowWithContextFilters` executed against real PostgreSQL)

### 2-Tier Minimum

Both tiers are required:
- **Unit tests** catch SQL generation correctness (fast, isolated — validates the SQL string shape).
- **Integration tests** catch SQL execution correctness against real PostgreSQL JSONB operators (this is the tier that would have caught the test gap before rc2).

### Business Outcome Quality Bar

Each test validates: "When a workflow uses `*` wildcards in its mandatory labels, does the discovery query correctly match it for any value of that label?" — a direct operator-facing outcome, not a code path exercise.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/discovery.go` | `buildContextFilterSQL` (lines 267–364) | ~97 |
| `pkg/datastorage/models/workflow_schema.go` | `ValidateMandatoryLabels` (lines 504–525) | ~21 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/discovery.go` | `ListActions` (lines 46–123) | ~77 |
| `pkg/datastorage/repository/workflow/discovery.go` | `ListWorkflowsByActionType` (lines 129–211) | ~82 |
| `pkg/datastorage/repository/workflow/discovery.go` | `GetWorkflowWithContextFilters` (lines 217–254) | ~37 |

---

## 4. BR Coverage Matrix

| BR / DD | Description | Priority | Tier | Test ID | Status |
|---------|-------------|----------|------|---------|--------|
| DD-WORKFLOW-001 v2.8 | Wildcard component: `component: '*'` matches any component query value | P0 | Unit | UT-DS-464-001 | Pass |
| DD-WORKFLOW-001 v2.8 | Wildcard priority (scalar): `priority: '*'` matches any priority query value | P0 | Unit | UT-DS-464-002 | Pass |
| DD-WORKFLOW-016 v2.1 | Wildcard priority (array branch): array `["*"]` matches any priority query value | P0 | Unit | UT-DS-464-003 | Pass (was RED, fix applied) |
| DD-WORKFLOW-001 v2.8 | Wildcard severity: `severity: ["*"]` matches any severity query value | P0 | Unit | UT-DS-464-004 | Skipped (covered by existing `TestBuildContextFilterSQL_SeverityWildcard`) |
| DD-WORKFLOW-001 v2.8 | Wildcard environment: `environment: ["*"]` matches any environment query value | P0 | Unit | UT-DS-464-005 | Pass |
| DD-WORKFLOW-001 v2.8 | All four mandatory labels wildcarded matches any query combination | P0 | Unit | UT-DS-464-006 | Pass |
| DD-WORKFLOW-001 v2.8 | `ValidateMandatoryLabels` accepts `"*"` for severity | P0 | Unit | UT-DS-464-007 | Pass (was RED, fix applied) |
| BR-HAPI-017-001 | ListActions returns action types when workflows have wildcard component + priority | P0 | Integration | IT-DS-464-001 | Implemented (pending PostgreSQL run) |
| BR-HAPI-017-001 | ListActions returns action types when workflows have all wildcard mandatory labels | P0 | Integration | IT-DS-464-002 | Implemented (pending PostgreSQL run) |
| BR-HAPI-017-001 | ListWorkflowsByActionType returns workflows with wildcard labels matching specific query | P0 | Integration | IT-DS-464-003 | Implemented (pending PostgreSQL run) |
| BR-HAPI-017-001 | GetWorkflowWithContextFilters passes security gate for wildcard-labeled workflows | P0 | Integration | IT-DS-464-004 | Implemented (pending PostgreSQL run) |
| BR-HAPI-017-001 | Mixed labels (some exact, some wildcard) — partial match filters correctly | P0 | Integration | IT-DS-464-005 | Implemented (pending PostgreSQL run) |
| DD-WORKFLOW-001 v2.8 | Wildcard severity in DB matches specific severity query via JSONB `?` operator | P0 | Integration | IT-DS-464-006 | Implemented (pending PostgreSQL run) |

### Status Legend

- Pass: Implemented and passing in unit tests
- Implemented (pending PostgreSQL run): Test code written, dry-run verified, awaiting CI execution against real PostgreSQL
- Skipped: Covered by existing test; adding would be redundant

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-DS-464-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `DS` (DataStorage)
- **BR_NUMBER**: `464` (GitHub issue)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `buildContextFilterSQL` (~97 lines), `ValidateMandatoryLabels` (~21 lines) — target >=80% of SQL generation logic and validation paths.

**Existing coverage**: `scoring_test.go` has `TestBuildContextFilterSQL_SeverityWildcard` (SQL string shape only). `discovery_filter_test.go` covers DetectedLabels SQL generation. Neither tests mandatory label wildcard generation for component, priority, or the all-wildcards case.

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-DS-464-001` | SQL generated for component filter includes `labels->>'component' = '*'` wildcard fallback, so workflows authored with `component: '*'` can match any Kubernetes resource type | Pass |
| `UT-DS-464-002` | SQL generated for scalar priority includes `labels->>'priority' = '*'` wildcard fallback, so workflows authored with `priority: '*'` can match any business priority | Pass |
| `UT-DS-464-003` | SQL generated for array priority includes `labels->'priority' ? '*'` wildcard fallback (was missing), so array-stored `["*"]` priorities match any query value | Pass (was RED → fix applied → GREEN) |
| `UT-DS-464-004` | SQL generated for severity includes `labels->'severity' ? '*'` wildcard fallback | Skipped (covered by existing `TestBuildContextFilterSQL_SeverityWildcard`) |
| `UT-DS-464-005` | SQL generated for environment includes `labels->'environment' ? '*'` wildcard fallback, so workflows authored with `environment: ["*"]` can match any environment | Pass |
| `UT-DS-464-006` | When all 4 mandatory filters are provided, generated SQL includes wildcard fallbacks for every label, so all-wildcard workflows always appear in discovery | Pass |
| `UT-DS-464-007` | `ValidateMandatoryLabels` accepts `severity: ["*"]` without error (DD-WORKFLOW-001 v2.8 compliance). 4 sub-tests: wildcard-only, wildcard+explicit, invalid rejected, standard values accepted. | Pass (was RED → fix applied → GREEN) |

### Tier 2: Integration Tests

**Testable code scope**: `ListActions` (~77 lines), `ListWorkflowsByActionType` (~82 lines), `GetWorkflowWithContextFilters` (~37 lines) — target >=80% of discovery repository code exercised against real PostgreSQL.

**Existing coverage**: `workflow_discovery_repository_test.go` has IT-DS-017-001-001 through 006, all using exact label values. Zero wildcard scenarios.

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `IT-DS-464-001` | When a workflow has `component: '*'` and `priority: '*'`, ListActions returns its action type for a query with `component=Pod, priority=P1` — the LLM can discover remediation workflows regardless of Kubernetes resource type or priority | Implemented (pending PostgreSQL run) |
| `IT-DS-464-002` | When a workflow has all 4 mandatory labels wildcarded (`severity: ["*"]`, `component: '*'`, `environment: ["*"]`, `priority: '*'`), ListActions returns its action type for any combination of filter values — universal workflows are always discoverable | Implemented (pending PostgreSQL run) |
| `IT-DS-464-003` | ListWorkflowsByActionType returns a wildcard-labeled workflow when queried with specific filter values — the LLM can proceed to Step 2 and see the workflow details | Implemented (pending PostgreSQL run) |
| `IT-DS-464-004` | GetWorkflowWithContextFilters returns a wildcard-labeled workflow (not nil) when queried with specific filter values — the security gate does not false-reject wildcard workflows | Implemented (pending PostgreSQL run) |
| `IT-DS-464-005` | A workflow with mixed labels (`severity: ["critical","high"]`, `component: '*'`, `environment: ["production","staging","*"]`, `priority: '*'`) matches a query with `severity=critical, component=Pod, environment=staging, priority=P1` — the exact demo scenario from issue #464 | Implemented (pending PostgreSQL run) |
| `IT-DS-464-006` | A workflow with `severity: ["*"]` is matched by `severity=critical` query via PostgreSQL JSONB `?` operator — validates that `"*"` inside a JSONB array is correctly treated as a matchable element | Implemented (pending PostgreSQL run) |

### Tier Skip Rationale

- **E2E**: Not required for this issue. The bug is entirely in the DataStorage SQL layer, which is fully exercised by integration tests against real PostgreSQL. The E2E tier would only add the HTTP transport layer (already tested by IT-DS-017-001-001 through 006) without additional wildcard-specific value.

---

## 6. Test Cases (Detail)

### UT-DS-464-001: Wildcard component SQL generation

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`
**Function**: `TestBuildContextFilterSQL_Issue464_ComponentWildcard`
**Status**: Pass

**Given**: A `WorkflowDiscoveryFilters` with `Component: "Pod"`
**When**: `buildContextFilterSQL` is called
**Then**: The SQL string contains `labels->>'component' = '*'` as an OR condition alongside the exact-match `LOWER(labels->>'component') = LOWER($N)`

**Acceptance Criteria**:
- SQL contains both `LOWER(labels->>'component')` (exact) and `labels->>'component' = '*'` (wildcard)
- Exactly one arg appended for the component filter
- Arg value is `"Pod"`

### UT-DS-464-002: Wildcard scalar priority SQL generation

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`
**Function**: `TestBuildContextFilterSQL_Issue464_PriorityScalarWildcard`
**Status**: Pass

**Given**: A `WorkflowDiscoveryFilters` with `Priority: "P1"`
**When**: `buildContextFilterSQL` is called
**Then**: The SQL ELSE branch contains `labels->>'priority' = '*'` as an OR condition

**Acceptance Criteria**:
- SQL contains `labels->>'priority' = '*'`
- SQL uses CASE WHEN for array/scalar handling
- Exactly one arg appended for the priority filter

### UT-DS-464-003: Wildcard array priority SQL generation (defensive fix)

**Authority**: DD-WORKFLOW-016 v2.1
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`
**Function**: `TestBuildContextFilterSQL_Issue464_PriorityArrayWildcard`
**Status**: Pass (was RED → fix applied → GREEN)

**Given**: A `WorkflowDiscoveryFilters` with `Priority: "P1"`
**When**: `buildContextFilterSQL` is called
**Then**: The SQL array CASE branch contains `labels->'priority' ? '*'` as an OR condition alongside `labels->'priority' ? $N`

**Acceptance Criteria**:
- SQL array branch includes both `labels->'priority' ? $N` (exact) and `labels->'priority' ? '*'` (wildcard)
- This test confirmed the missing wildcard fallback (RED phase), then passed after fix (GREEN phase)

**Code fix**: Added `OR labels->'priority' ? '*'` in `pkg/datastorage/repository/workflow/discovery.go` line ~306.

### UT-DS-464-004: Wildcard severity SQL generation (skipped)

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Unit
**Status**: Skipped — covered by existing `TestBuildContextFilterSQL_SeverityWildcard` in `scoring_test.go`

### UT-DS-464-005: Wildcard environment SQL generation

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`
**Function**: `TestBuildContextFilterSQL_Issue464_EnvironmentWildcard`
**Status**: Pass

**Given**: A `WorkflowDiscoveryFilters` with `Environment: "staging"`
**When**: `buildContextFilterSQL` is called
**Then**: The SQL contains `labels->'environment' ? '*'` as an OR condition

**Acceptance Criteria**:
- SQL contains both `labels->'environment' ? $N` (exact) and `labels->'environment' ? '*'` (wildcard)
- Exactly one arg appended for the environment filter

### UT-DS-464-006: All-wildcards SQL generation

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Unit
**File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go`
**Function**: `TestBuildContextFilterSQL_Issue464_AllMandatoryWildcards`
**Status**: Pass

**Given**: A `WorkflowDiscoveryFilters` with all 4 mandatory filters: `Severity: "critical"`, `Component: "Pod"`, `Environment: "staging"`, `Priority: "P1"`
**When**: `buildContextFilterSQL` is called
**Then**: The SQL contains wildcard fallback conditions for all 4 labels and uses 4 positional args ($1–$4)

**Acceptance Criteria**:
- SQL contains 4 AND-joined conditions
- Each condition includes its respective wildcard fallback (`? '*'` for arrays, `= '*'` for scalars)
- Args count is 4, with values `["critical", "Pod", "staging", "P1"]`

### UT-DS-464-007: ValidateMandatoryLabels accepts severity wildcard

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Unit (Ginkgo/Gomega BDD)
**File**: `test/unit/datastorage/schema_severity_wildcard_test.go`
**Status**: Pass (was RED → fix applied → GREEN)

**Sub-tests** (4 `It` blocks under one `Context`):

1. **Should accept severity `["*"]` as a valid wildcard value**
   - Given: `WorkflowSchemaLabels` with `Severity: ["*"]`, `Component: "pod"`, `Environment: ["production"]`, `Priority: "P1"`
   - When: `ValidateMandatoryLabels()` is called
   - Then: No error returned

2. **Should accept severity `["critical", "*"]` alongside explicit values**
   - Given: `WorkflowSchemaLabels` with `Severity: ["critical", "*"]`
   - Then: No error returned

3. **Should still reject invalid severity values**
   - Given: `WorkflowSchemaLabels` with `Severity: ["invalid-value"]`
   - Then: Error returned

4. **Should still accept standard severity values**
   - Given: `WorkflowSchemaLabels` with `Severity: ["critical", "high", "medium", "low"]`
   - Then: No error returned

**Code fix**: Added `"*": true` to `allowedSeverities` map and updated error message in `pkg/datastorage/models/workflow_schema.go` line ~509.

### IT-DS-464-001: ListActions matches wildcard component + priority

**Authority**: BR-HAPI-017-001
**Type**: Integration (Ginkgo/Gomega BDD)
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Status**: Implemented (pending PostgreSQL run)

**Given**: A workflow with `component: '*'`, `priority: '*'`, `severity: ["critical"]`, `environment: ["production"]`, status `active`, `is_latest_version: true`, and a matching row in `action_type_taxonomy`
**When**: `ListActions` is called with filters `Component: "Pod"`, `Priority: "P1"`, `Severity: "critical"`, `Environment: "production"`
**Then**: Result contains 1 action type entry with `workflow_count >= 1`

**Acceptance Criteria**:
- `totalCount == 1`
- `entries[0].ActionType` matches the workflow's action type
- `entries[0].WorkflowCount >= 1`

### IT-DS-464-002: ListActions matches all-wildcard workflow

**Authority**: BR-HAPI-017-001
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Status**: Implemented (pending PostgreSQL run)

**Given**: A workflow with `severity: ["*"]`, `component: '*'`, `environment: ["*"]`, `priority: '*'`, status `active`, `is_latest_version: true`
**When**: `ListActions` is called with filters `Severity: "high"`, `Component: "Deployment"`, `Environment: "staging"`, `Priority: "P3"`
**Then**: Result contains 1 action type entry

**Acceptance Criteria**:
- `totalCount == 1`
- Workflow is matched regardless of the specific filter values chosen

### IT-DS-464-003: ListWorkflowsByActionType returns wildcard-labeled workflow

**Authority**: BR-HAPI-017-001
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Status**: Implemented (pending PostgreSQL run)

**Given**: A workflow with `component: '*'`, `priority: '*'`, `severity: ["critical"]`, `environment: ["production"]`, action type `ScaleReplicas`, status `active`
**When**: `ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)` is called with filters `Component: "Pod"`, `Priority: "P1"`, `Severity: "critical"`, `Environment: "production"`
**Then**: Result contains 1 workflow

**Acceptance Criteria**:
- `totalCount == 1`
- `workflows[0].ActionType == "ScaleReplicas"`

### IT-DS-464-004: GetWorkflowWithContextFilters passes security gate for wildcard workflow

**Authority**: BR-HAPI-017-001
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Status**: Implemented (pending PostgreSQL run)

**Given**: A workflow with `component: '*'`, `priority: '*'`, `severity: ["critical"]`, `environment: ["production"]`
**When**: `GetWorkflowWithContextFilters(ctx, workflowID, filters)` is called with filters `Component: "Pod"`, `Priority: "P1"`, `Severity: "critical"`, `Environment: "production"`
**Then**: Result is not nil — the security gate passes

**Acceptance Criteria**:
- Returned workflow is not nil
- Returned workflow ID matches the created workflow's ID

### IT-DS-464-005: Demo scenario — mixed wildcards + exact labels

**Authority**: BR-HAPI-017-001, Issue #464
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Status**: Implemented (pending PostgreSQL run)

**Given**: A workflow matching the exact demo scenario:
- `severity: ["critical", "high"]`
- `component: '*'`
- `environment: ["production", "staging", "*"]`
- `priority: '*'`

**When**: `ListActions` is called with the exact demo query: `Severity: "critical"`, `Component: "Pod"`, `Environment: "staging"`, `Priority: "P1"`
**Then**: Result contains 1 action type entry

**Acceptance Criteria**:
- `totalCount == 1` (the critical assertion — this is 0 in the original report)
- Workflow is matched via wildcard component (`*` matches `Pod`) and wildcard priority (`*` matches `P1`)

### IT-DS-464-006: Severity wildcard in JSONB array matches via `?` operator

**Authority**: DD-WORKFLOW-001 v2.8
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Status**: Implemented (pending PostgreSQL run)

**Given**: A workflow with `severity: ["*"]`, `component: "pod"`, `environment: ["production"]`, `priority: "P0"`
**When**: `ListActions` is called with filters `Severity: "critical"`, `Component: "pod"`, `Environment: "production"`, `Priority: "P0"`
**Then**: Result contains 1 action type entry

**Acceptance Criteria**:
- `totalCount == 1`
- PostgreSQL's `labels->'severity' ? '*'` correctly matches when the array contains `"*"` and the query value is `"critical"`

---

## 7. Test Infrastructure

### Unit Tests (SQL generation)

- **Framework**: Go standard `testing` package (consistent with existing `discovery_filter_test.go` and `scoring_test.go` which are in the `workflow` package and use `testing.T`)
- **Mocks**: None — `buildContextFilterSQL` is a pure function
- **File**: `pkg/datastorage/repository/workflow/discovery_filter_test.go` (appended to existing file)

### Unit Tests (validation)

- **Framework**: Ginkgo/Gomega BDD (consistent with existing `test/unit/datastorage/` test suite)
- **Mocks**: None — `ValidateMandatoryLabels` is a pure function on `WorkflowSchemaLabels`
- **File**: `test/unit/datastorage/schema_severity_wildcard_test.go` (new file, registered in existing Ginkgo suite)

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (per No-Mocks Policy)
- **Infrastructure**: Real PostgreSQL (same as existing `workflow_discovery_repository_test.go`)
- **File**: `test/integration/datastorage/workflow_discovery_repository_test.go` (appended to existing Describe block)
- **Test data**: New `createTestWorkflowWithArrayLabels` helper supports multi-value severity/environment arrays and wildcard values

---

## 8. Execution

```bash
# Unit tests (SQL generation) — all 5 Issue #464 tests
go test ./pkg/datastorage/repository/workflow/... -run="TestBuildContextFilterSQL_Issue464" -v

# Unit tests (validation) — UT-DS-464-007 (4 sub-tests)
go test ./test/unit/datastorage/... --ginkgo.focus="UT-DS-464-007" -v

# Integration tests (requires PostgreSQL)
make test-integration-datastorage
# Or focused:
go test ./test/integration/datastorage/... --ginkgo.focus="IT-DS-464" -v
```

---

## 9. Files Changed

### Code Fixes

| File | Change | Authority |
|------|--------|-----------|
| `pkg/datastorage/repository/workflow/discovery.go` | Added `OR labels->'priority' ? '*'` in priority array CASE branch | DD-WORKFLOW-016 v2.1 |
| `pkg/datastorage/models/workflow_schema.go` | Added `"*": true` to `allowedSeverities` map; updated error message | DD-WORKFLOW-001 v2.8 |

### Test Files

| File | Tests Added | Framework |
|------|-------------|-----------|
| `pkg/datastorage/repository/workflow/discovery_filter_test.go` | UT-DS-464-001, 002, 003, 005, 006 | Go `testing` |
| `test/unit/datastorage/schema_severity_wildcard_test.go` | UT-DS-464-007 (4 sub-tests) | Ginkgo/Gomega |
| `test/integration/datastorage/workflow_discovery_repository_test.go` | IT-DS-464-001 through 006 | Ginkgo/Gomega |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for issue #464 |
| 2.0 | 2026-03-20 | Updated after root cause analysis: PostgreSQL image swap was actual cause. Reduced scope to two defensive code fixes + integration test gap closure. Updated all test statuses to reflect TDD execution. Added Section 0 (Root Cause Analysis), Section 9 (Files Changed). Corrected UT-DS-464-007 file location. Marked UT-DS-464-004 as skipped (covered by existing test). |
