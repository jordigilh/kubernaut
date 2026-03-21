# Test Plan: Graceful CRD Deletion When DataStorage Is Empty or Unreachable

**Feature**: AuthWebhook treats "not found" and connectivity errors as success during CRD deletion
**Version**: 1.0
**Created**: 2026-03-21
**Author**: AI Assistant
**Status**: Complete
**Branch**: `development/v1.2`

**Authority**:
- BR-WORKFLOW-007: ActionType CRD lifecycle management
- Issue #418: Finalizer-based reconciler for RW deletion
- Issue #469: CRDs stuck Terminating after helm reinstall

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)

---

## 1. Scope

### In Scope

- **ActionType webhook `handleDelete`** (`pkg/authwebhook/actiontype_handler.go`): Treat `DisableActionTypeNotFound` as successful cleanup so DELETE is allowed when DS has no record.
- **RW finalizer `reconcileDelete`** (`pkg/authwebhook/rw_reconciler.go`): Tolerate connection/transport errors from `DisableWorkflow` during deletion instead of requeueing forever.
- **DS client adapter** (`pkg/authwebhook/ds_client.go`): Verify `DisableActionTypeNotFound` return path.

### Out of Scope

- ActionType CREATE/UPDATE webhook paths (unchanged)
- RW admission webhook `handleDelete` (already best-effort, unaffected)
- Helm pre-delete hooks or finalizer removal scripts (deferred)

### Design Decisions

- Treat DS "not found" as cleanup-already-done (the catalog entry either never existed or was already removed)
- For RW finalizer, tolerate connection errors during deletion to unblock helm reinstall scenarios
- Preserve fail-closed behavior for non-deletion DS errors (400, 500, auth) to maintain catalog consistency during normal operation

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of the modified error-handling branches in `handleDelete` and `reconcileDelete`

### 2-Tier Minimum

- **Unit tests** cover the new error-handling branches directly
- **Integration tier skipped** — see rationale below

### Business Outcome Quality Bar

Tests validate that operators can successfully perform `helm uninstall` + `helm install` without CRDs getting stuck in Terminating state.

---

## 3. Testable Code Inventory

### Unit-Testable Code (error-handling branches)

- `pkg/authwebhook/actiontype_handler.go` — `handleDelete` not-found branch (~5 lines)
- `pkg/authwebhook/rw_reconciler.go` — `reconcileDelete` connection-error tolerance (~10 lines)
- `pkg/authwebhook/ds_client.go` — `DisableActionType` not-found case (~3 lines)

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-007 | AT DELETE allows when DS returns not-found | P0 | Unit | UT-AW-469-001 | Pass |
| BR-WORKFLOW-007 | AT DELETE still denied on DS server errors | P1 | Unit | UT-AW-469-002 | Pass |
| BR-WORKFLOW-007 | AT DELETE audit emitted for not-found allow | P2 | Unit | UT-AW-469-003 | Pass (covered by UT-AT-300-006) |
| #418 | RW finalizer removes on DS connection error | P0 | Unit | UT-AW-469-004 | Pass |
| #418 | RW finalizer removes on DS not-found (404) | P0 | Unit | UT-AW-469-005 | Pass (covered by UT-AW-418-002) |
| #418 | RW finalizer still retries on DS server error (500) | P1 | Unit | UT-AW-469-006 | Pass (covered by UT-AW-418-003) |

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `actiontype_handler.go:handleDelete`, `rw_reconciler.go:reconcileDelete`, `ds_client.go:DisableActionType`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AW-469-001` | ActionType DELETE succeeds when DS has no record of the action type | Pass |
| `UT-AW-469-002` | ActionType DELETE remains fail-closed when DS returns a server error | Pass |
| `UT-AW-469-003` | ActionType DELETE emits an allowed audit event when DS returns not-found | Pass (covered by UT-AT-300-006) |
| `UT-AW-469-004` | RW finalizer completes deletion when DS is unreachable (connection refused) | Pass |
| `UT-AW-469-005` | RW finalizer completes deletion when DS returns 404 for the workflow | Pass (covered by UT-AW-418-002) |
| `UT-AW-469-006` | RW finalizer retries when DS returns 500 (preserves catalog consistency) | Pass (covered by UT-AW-418-003) |

### Tier Skip Rationale

- **Integration**: The stuck-CRD scenario requires a full Helm lifecycle (uninstall + install) which is E2E scope. Unit tests with mock DS clients adequately cover the error-handling branch logic. The existing integration tests for AuthWebhook (`IT-AW-418-*`) cover the happy path wiring.

---

## 6. Test Cases (Detail)

### UT-AW-469-001: AT DELETE allowed when DS returns not-found

**BR**: BR-WORKFLOW-007
**Type**: Unit
**File**: `test/unit/authwebhook/actiontype_handler_test.go`

**Given**: An ActionType CRD exists in the cluster but DS has no record of it (empty DB after reinstall)
**When**: A DELETE admission request is received for the ActionType
**Then**: The webhook allows the DELETE (admission.Allowed), not denied

**Acceptance Criteria**:
- `DisableActionType` is called with correct `spec.name`
- Response is `Allowed` (not `Denied`)
- No error message in the response

### UT-AW-469-002: AT DELETE denied on DS server error

**BR**: BR-WORKFLOW-007
**Type**: Unit
**File**: `test/unit/authwebhook/actiontype_handler_test.go`

**Given**: DS is running but returns a 500 Internal Server Error on `DisableActionType`
**When**: A DELETE admission request is received for the ActionType
**Then**: The webhook denies the DELETE to preserve catalog consistency

**Acceptance Criteria**:
- Response is `Denied`
- Error message mentions data storage failure

### UT-AW-469-003: AT DELETE audit on not-found allow

**BR**: BR-WORKFLOW-007
**Type**: Unit
**File**: `test/unit/authwebhook/actiontype_handler_test.go`

**Given**: DS returns not-found for `DisableActionType`
**When**: DELETE is allowed
**Then**: An admitted-delete audit event is emitted

**Acceptance Criteria**:
- Audit event type is `EventTypeATAdmittedDelete`
- Audit payload includes the action type name

### UT-AW-469-004: RW finalizer completes on connection error

**BR**: Issue #418
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW CRD with `catalog-cleanup` finalizer and `deletionTimestamp` set; DS client returns a connection refused error
**When**: The reconciler runs `reconcileDelete`
**Then**: The finalizer is removed and the RW is deleted (no requeue)

**Acceptance Criteria**:
- Finalizer removed from the RW object
- `Reconcile` returns empty result (no `RequeueAfter`)
- DS `DisableWorkflow` was attempted (called once)

### UT-AW-469-005: RW finalizer completes on DS 404

**BR**: Issue #418
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW CRD with `catalog-cleanup` finalizer and `deletionTimestamp` set; DS `DisableWorkflow` returns nil error (ogen 404 path)
**When**: The reconciler runs `reconcileDelete`
**Then**: The finalizer is removed and the RW is deleted

**Acceptance Criteria**:
- Finalizer removed
- Empty result returned
- This is the existing behavior (regression guard)

### UT-AW-469-006: RW finalizer retries on DS 500

**BR**: Issue #418
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW CRD with `catalog-cleanup` finalizer and `deletionTimestamp` set; DS `DisableWorkflow` returns a server error (not connection/not-found)
**When**: The reconciler runs `reconcileDelete`
**Then**: The finalizer is kept and the result includes `RequeueAfter: 5s`

**Acceptance Criteria**:
- Finalizer still present on the RW
- `RequeueAfter` is 5 seconds
- This is the existing behavior (regression guard)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock DS client (existing `mockDSClient` / `mockActionTypeCatalogClient` in test files)
- **Location**: `test/unit/authwebhook/`

---

## 8. Execution

```bash
# Unit tests (AT webhook)
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AW-469"

# All authwebhook unit tests (regression)
go test ./test/unit/authwebhook/... -v -count=1
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-21 | Initial test plan |
| 1.1 | 2026-03-21 | Updated statuses: 001/002/004 Pass; 003/005/006 covered by existing tests |
