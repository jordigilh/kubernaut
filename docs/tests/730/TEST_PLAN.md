# Test Plan: DS Workflow Catalog PK Collision Recovery (#730)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-730-v1.0
**Feature**: Handle PK collision gracefully when re-registering a workflow version whose content already exists in the catalog
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/722-em-false-positive-remediated`

---

## 1. Introduction

### 1.1 Purpose

When the authwebhook reconciler re-registers a workflow CRD version whose content already exists in the DB (as a Superseded row), the cross-version supersession in `SupersedeAndCreate` supersedes the currently Active version and then fails to INSERT the incoming version due to PK collision (deterministic UUID from content hash). Although #707 made `SupersedeAndCreate` transactional (the supersede is rolled back), the handler returns 500 to the authwebhook. This creates CRD/catalog state divergence and error noise.

This test plan validates a graceful recovery: when the INSERT fails with PK collision on a content-hash-derived UUID, re-activate the existing row instead of failing.

### 1.2 Objectives

1. **PK collision recovery**: When INSERT fails with 23505 on the primary key, the existing Superseded row with that workflow_id is re-activated to Active status
2. **Transaction atomicity preserved**: The supersede + re-activate is atomic (no visibility gap)
3. **Idempotent response**: Handler returns 200 (or 201) with the re-activated workflow, not 500
4. **No impact on normal registration**: Happy path (new content, no PK collision) unchanged
5. **retryOnUniqueViolation alignment**: After cross-version supersede failure, retry looks for ANY Active workflow by name (not just the incoming version)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/...` |
| Integration test pass rate | 100% | `go test ./test/integration/datastorage/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `workflow/crud.go` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on `workflow_handlers.go` |
| Backward compatibility | 0 regressions | Existing workflow tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- DD-WORKFLOW-017: Workflow lifecycle component interactions — Phase 1 (Registration)
- ADR-058: Webhook-driven workflow registration
- BR-WORKFLOW-006: RemediationWorkflow CRD
- Issue #730: DS workflow catalog version downgrade race condition
- Issue #707: Non-atomic supersede (fixed by `SupersedeAndCreate`)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing integration tests: `test/integration/datastorage/workflow_supersession_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Re-activate sets wrong status/metadata on existing row | Catalog entry has stale data | Medium | UT-DS-730-003, IT-DS-730-002 | Test verifies re-activated row has correct status, content_hash, is_latest_version |
| R2 | Concurrent registrations race on re-activate path | Multiple rows become Active for same name | Medium | IT-DS-730-003 | Transaction isolation + partial unique index enforcement |
| R3 | PK collision from non-content source (manual UUID) | Unexpected re-activation of wrong row | Low | UT-DS-730-004 | Only re-activate when collision is on deterministic content-hash UUID |
| R4 | is_latest_version flag inconsistency after recovery | Discovery returns empty despite Active row | High | UT-DS-730-005 | Test verifies is_latest_version is set correctly on re-activated row |
| R5 | Re-activate a Disabled row (security) | Workflow intentionally disabled comes back | High | UT-DS-730-006 | Re-activate WHERE status = 'Superseded' only; Disabled rows are NOT re-activated |
| R6 | Partial unique index conflict on re-activate | Another Active row with same (name, version) exists | Medium | IT-DS-730-003 | DB constraint prevents; test verifies error handling |
| R7 | No unit test directory for crud.go | Coverage gap | Medium | IT-DS-730-* | Integration tests with real PostgreSQL cover the transactional behavior |

---

## 4. Scope

### 4.1 Features to be Tested

- **`SupersedeAndCreate` PK collision handling** (`pkg/datastorage/repository/workflow/crud.go`): When INSERT fails with 23505 on PK, attempt to UPDATE the existing row to Active within the same transaction
- **`handleDuplicateWorkflow` cross-version recovery** (`pkg/datastorage/server/workflow_handlers.go`): Handler returns success when PK collision is recovered
- **`retryOnUniqueViolation` improvement** (`pkg/datastorage/server/workflow_handlers.go`): After cross-version failure, look for ANY Active by workflow_name (not just incoming version)

### 4.2 Features Not to be Tested

- **Same-version duplicate handling**: Already working correctly (content hash comparison)
- **Version ordering/comparison**: Not required — any version can supersede any other (deliberate rollback is valid)
- **Authwebhook reconciler**: Upstream CRD handling, not changed

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Re-activate existing row on PK collision | The content is identical (same hash → same UUID); re-activating is semantically correct |
| Keep transaction atomic | SupersedeAndCreate already has tx boundary; re-activate replaces INSERT within same tx |
| No version comparison | User may intentionally activate an older version (GitOps rollback) |
| Update is_latest_version on re-activate | Discovery requires Active + is_latest_version = true |
| Only re-activate Superseded rows | Disabled rows are intentionally disabled by operator; PK collision with Disabled row should fail (not silently re-enable) |
| Use ON CONFLICT DO UPDATE (Option B) | Avoids TOCTOU race; single statement; constrained by WHERE status = 'Superseded' |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `SupersedeAndCreate` and PK collision recovery path in `crud.go`
- **Integration**: >=80% of `handleDuplicateWorkflow` cross-version path in `workflow_handlers.go`

### 5.2 Pass/Fail Criteria

**PASS**: All tests pass, PK collision returns re-activated workflow (not 500), is_latest_version consistent, no regressions in existing workflow tests.

**FAIL**: Any test fails, handler returns 500 on PK collision, or is_latest_version inconsistent after recovery.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/crud.go` | `SupersedeAndCreate` (PK collision branch) | ~120 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | `handleDuplicateWorkflow`, `retryOnUniqueViolation` | ~100 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-006 | PK collision on re-registration recovers to Active | P0 | Unit | UT-DS-730-001 | Pending |
| BR-WORKFLOW-006 | Supersede of current Active + re-activate is atomic | P0 | Unit | UT-DS-730-002 | Pending |
| DD-WORKFLOW-017 | Re-activated row has correct status and is_latest_version | P0 | Unit | UT-DS-730-003 | Pending |
| DD-WORKFLOW-017 | Normal supersede+create (no PK collision) unchanged | P0 | Unit | UT-DS-730-004 | Pending |
| #730 | Handler returns 200/201 on PK collision recovery | P0 | Integration | IT-DS-730-001 | Pending |
| #730 | Full scenario: seed v1.2.6, re-register v1.2.4 (exists), catalog has Active entry | P0 | Integration | IT-DS-730-002 | Pending |
| #730 | retryOnUniqueViolation finds Active by workflow_name after cross-version failure | P1 | Integration | IT-DS-730-003 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `pkg/datastorage/repository/workflow/crud.go` — >=80% coverage of `SupersedeAndCreate`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-730-001` | SupersedeAndCreate recovers from PK collision by re-activating existing Superseded row | Pending |
| `UT-DS-730-002` | Transaction atomicity: supersede + re-activate commits together or rolls back together | Pending |
| `UT-DS-730-003` | Re-activated row has status=Active, is_latest_version=true, correct content_hash | Pending |
| `UT-DS-730-004` | Normal path (no PK collision) creates new row as before | Pending |
| `UT-DS-730-005` | is_latest_version cleared on other rows and set on re-activated row | Pending |
| `UT-DS-730-006` | PK collision with Disabled row does NOT re-activate (security) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `pkg/datastorage/server/workflow_handlers.go` — >=80% coverage of cross-version path

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-730-001` | POST /workflows with PK collision returns 200/201 with re-activated workflow | Pending |
| `IT-DS-730-002` | Full #730 scenario: v1.2.6 Active, re-register v1.2.4 (PK exists), catalog shows Active entry | Pending |
| `IT-DS-730-003` | Concurrent re-registration of same old version does not create duplicate Active rows | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — E2E scenario requires Kind cluster with authwebhook; coverage provided by integration tests against real PostgreSQL

---

## 9. Test Cases

### UT-DS-730-001: PK collision recovery in SupersedeAndCreate

**BR**: BR-WORKFLOW-006
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/workflow/crud_test.go`

**Test Steps**:
1. **Given**: DB has workflow `wf-1` v1.2.4 (status=Superseded, workflow_id=UUID-A derived from content hash) and `wf-1` v1.2.6 (status=Active, workflow_id=UUID-B)
2. **When**: `SupersedeAndCreate(UUID-B, "1.2.6", reason, newWorkflow{WorkflowID: UUID-A, Version: "1.2.4", Status: "Active"})` is called
3. **Then**: v1.2.6 is Superseded, UUID-A row is updated to Active (not a new INSERT), transaction commits successfully

### UT-DS-730-002: Atomicity on failure

**BR**: BR-WORKFLOW-006
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/workflow/crud_test.go`

**Test Steps**:
1. **Given**: Same setup as UT-DS-730-001 but the re-activate UPDATE fails (simulated DB error)
2. **When**: `SupersedeAndCreate` is called
3. **Then**: Transaction rolls back; v1.2.6 remains Active; v1.2.4 remains Superseded

### UT-DS-730-004: Normal path unchanged

**BR**: DD-WORKFLOW-017
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/workflow/crud_test.go`

**Test Steps**:
1. **Given**: DB has `wf-1` v1.2.4 (Active). New workflow v1.2.6 has a NEW content hash (no PK collision).
2. **When**: `SupersedeAndCreate(UUID-A, "1.2.4", reason, newWorkflow{Version: "1.2.6"})` is called
3. **Then**: v1.2.4 is Superseded, v1.2.6 is inserted as Active — standard behavior

### IT-DS-730-002: Full #730 scenario

**BR**: #730
**Priority**: P0
**Type**: Integration
**File**: `test/integration/datastorage/workflow_supersession_test.go`

**Test Steps**:
1. **Given**: Register `increase-memory-limits-v1` v1.2.4 via POST /workflows (Active)
2. **When**: Register same workflow v1.2.6 (new content, supersedes v1.2.4)
3. **Then**: v1.2.6 is Active, v1.2.4 is Superseded
4. **When**: Re-register v1.2.4 (same content as step 1, PK collision with existing Superseded row)
5. **Then**: Response is 200/201; catalog has exactly one Active entry; `GET /workflows?action_type=IncreaseMemoryLimits` returns the workflow

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `sqlx.DB` via sqlmock or test database
- **Location**: `test/unit/datastorage/workflow/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: Real PostgreSQL (programmatic Go container or existing test DB)
- **Location**: `test/integration/datastorage/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **TDD RED**: Write UT-DS-730-001 through UT-DS-730-005 and IT-DS-730-001 through IT-DS-730-003 — all fail
2. **TDD GREEN**: Implement PK collision recovery in `SupersedeAndCreate` (ON CONFLICT or catch 23505 + UPDATE); update `retryOnUniqueViolation` to look for Active by name
3. **TDD REFACTOR**: Extract recovery logic, improve logging, verify existing workflow tests pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/730/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/datastorage/workflow/crud_test.go` | Ginkgo BDD test file |
| Integration test suite | `test/integration/datastorage/workflow_supersession_test.go` | Ginkgo BDD additions |

---

## 13. Execution

```bash
go test ./test/unit/datastorage/workflow/... -ginkgo.v
go test ./test/integration/datastorage/... -ginkgo.v -ginkgo.focus="UT-DS-730\|IT-DS-730"
go test ./test/unit/datastorage/workflow/... -coverprofile=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/integration/datastorage/workflow_supersession_test.go` | Cross-version supersede + create succeeds | Add PK collision scenario | Existing tests only cover happy path |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
