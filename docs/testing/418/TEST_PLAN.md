# Test Plan: RW Finalizer for DS Catalog Consistency (#418)

**Feature**: Adopt Kubernetes finalizer pattern to guarantee DS catalog consistency on RemediationWorkflow deletion
**Version**: 1.0
**Created**: 2026-03-16
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/hapi-revert-ubi9-and-timeout`

**Authority**:
- BR-WORKFLOW-006: Kubernetes-native workflow registration via CRD + AW bridge
- BR-WORKFLOW-007: ActionType CRD lifecycle management
- BR-WORKFLOW-007.3: AT DELETE denied when dependent workflows exist
- [GitHub Issue #418](https://github.com/jordigilh/kubernaut/issues/418)

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Plan #300 (ActionType CRD)](../300/TEST_PLAN.md) — parent feature test plan
- [Test Plan #299 (RemediationWorkflow CRD)](../299/TEST_PLAN.md) — RW webhook lifecycle

---

## 1. Scope

### In Scope

- **RemediationWorkflowReconciler**: Finalizer add/remove lifecycle, DS disable on deletion, AT count refresh
- **Fix A (conditional goroutine)**: Webhook's `refreshActionTypeWorkflowCount` goroutine only fires when `DisableWorkflow` succeeds
- **E2E business outcome**: AT DELETE succeeds after RW removal (validates DS catalog consistency end-to-end)
- **RBAC**: AuthWebhook service account gains `update`/`patch` on `remediationworkflows` and `remediationworkflows/finalizers`

### Out of Scope

- RW CREATE webhook flow (unchanged; fire-and-forget goroutine for CREATE cross-update is acceptable)
- ActionType handler logic (unchanged; already queries DS directly)
- DS repository/handler internals (tested by #300 test plan)
- Helm chart RBAC validation (verified by existing smoke tests)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Finalizer controller in authwebhook binary | AW already has controller-runtime manager, DS client, and K8s client; no new binary needed |
| Controller adds finalizer (not mutating webhook) | Self-healing: controller reconciles existing RWs on upgrade; no new webhook config |
| Webhook keeps best-effort DS disable on DELETE | Defense-in-depth: fast-path disable in webhook, guaranteed disable in controller; idempotent |
| Fix A: conditional goroutine | Prevents writing stale count when DS is unavailable; finalizer handles the guaranteed path |
| E2E validates business outcome (AT DELETE succeeds) | DS is source of truth; CRD `activeWorkflowCount` is a display-only cache |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (reconciler logic, conditional goroutine)
- **E2E**: >=80% of full lifecycle code (finalizer add → RW delete → DS disabled → AT deletable)

### 2-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests** validate reconciler logic and conditional goroutine behavior with mocked DS and fake K8s
- **E2E tests** validate the full finalizer lifecycle in a Kind cluster with real services

### Business Outcome Quality Bar

Tests validate business outcomes: "Is the DS catalog consistent after RW deletion?" and "Can the operator delete an ActionType after removing its workflows?" — not "Was the finalizer added?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/rw_reconciler.go` | `Reconcile`, `reconcileDelete`, `refreshActionTypeWorkflowCount` | ~120 |
| `pkg/authwebhook/remediationworkflow_handler.go` | `handleDelete` (Fix A: conditional goroutine) | ~45 |

### E2E-Testable Code (full stack)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/rw_reconciler.go` | Full reconciler lifecycle | ~120 |
| `cmd/authwebhook/main.go` | Controller registration wiring | ~15 |
| `charts/kubernaut/templates/authwebhook/authwebhook.yaml` | RBAC for finalizer add/remove | ~10 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-006 | Finalizer added to RW on creation | P0 | Unit | UT-AW-418-001 | Pass |
| BR-WORKFLOW-006 | Finalizer removal after DS disable completes | P0 | Unit | UT-AW-418-002 | Pass |
| BR-WORKFLOW-006 | DS disable retried on transient failure | P0 | Unit | UT-AW-418-003 | Pass |
| BR-WORKFLOW-006 | Empty WorkflowID: finalizer removed without DS call | P1 | Unit | UT-AW-418-004 | Pass |
| BR-WORKFLOW-007 | AT activeWorkflowCount refreshed from DS after RW deletion | P1 | Unit | UT-AW-418-005 | Pass |
| BR-WORKFLOW-007 | AT count refresh failure does not block finalizer removal | P1 | Unit | UT-AW-418-006 | Pass |
| BR-WORKFLOW-006 | Fix A: goroutine fires only when DisableWorkflow succeeds | P0 | Unit | UT-AW-418-007 | Pass |
| BR-WORKFLOW-006 | Fix A: goroutine does NOT fire when DisableWorkflow fails | P0 | Unit | UT-AW-418-008 | Pass |
| BR-WORKFLOW-006 | RW not deleted from K8s until finalizer is removed | P0 | E2E | E2E-AW-418-001 | Pass |
| BR-WORKFLOW-007.3 | AT DELETE succeeds after RW removal (DS consistent) | P0 | E2E | E2E-AW-418-002 | Pass (via E2E-AT-300-003) |
| BR-WORKFLOW-007 | AT activeWorkflowCount=0 after RW deleted via finalizer | P1 | E2E | E2E-AW-418-003 | Pass (via E2E-AT-300-003) |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-AW-418-{SEQUENCE}`

- **TIER**: `UT` (Unit), `E2E` (End-to-End)
- **AW**: AuthWebhook service abbreviation
- **418**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/authwebhook/rw_reconciler.go` (100%), `remediationworkflow_handler.go` handleDelete (Fix A)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AW-418-001` | New RW CRD receives the catalog-cleanup finalizer so it cannot be silently deleted | Pass |
| `UT-AW-418-002` | On deletion, DS workflow is disabled and finalizer is removed — RW can be garbage collected | Pass |
| `UT-AW-418-003` | Transient DS failure during deletion causes requeue (retry) — RW stays until DS confirms | Pass |
| `UT-AW-418-004` | RW with empty WorkflowID: finalizer removed without DS call (no orphan, no crash) | Pass |
| `UT-AW-418-005` | After RW deletion, parent AT's activeWorkflowCount is refreshed from DS (not stale) | Pass |
| `UT-AW-418-006` | AT count refresh failure does not prevent finalizer removal (deletion not blocked by display cache) | Pass |
| `UT-AW-418-007` | Webhook DELETE: when DS disable succeeds, count refresh goroutine fires (fast-path) | Pass |
| `UT-AW-418-008` | Webhook DELETE: when DS disable fails, count refresh goroutine does NOT fire (no stale write) | Pass |

### Tier 3: E2E Tests

**Testable code scope**: Full finalizer lifecycle in Kind cluster with real AW, DS, PostgreSQL

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-AW-418-001` | RW deletion is blocked until the finalizer controller completes DS cleanup | Pass |
| `E2E-AW-418-002` | Operator can delete an ActionType after removing all its dependent workflows (DS consistent) | Pass (via E2E-AT-300-003) |
| `E2E-AW-418-003` | After RW deleted via finalizer, `kubectl get at` shows correct WORKFLOWS count (0) | Pass (via E2E-AT-300-003) |

### Tier Skip Rationale

- **Integration**: The reconciler's testable surface is K8s wiring + DS HTTP calls. Unit tests cover logic exhaustively with mocked dependencies. E2E tests cover the full stack. An integration tier with envtest would test the same boundaries as E2E but with mocked DS, offering limited additional coverage for this focused change. Deferred to future hardening.

---

## 6. Test Cases (Detail)

### UT-AW-418-001: Finalizer added to new RW

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: A RemediationWorkflow CRD exists without the `remediationworkflow.kubernaut.ai/catalog-cleanup` finalizer
**When**: The reconciler processes the RW
**Then**: The finalizer is present in `.metadata.finalizers` and the reconciler requeues

**Acceptance Criteria**:
- `controllerutil.ContainsFinalizer(rw, RWFinalizerName)` returns true after reconcile
- Reconcile result has `Requeue: true`

---

### UT-AW-418-002: Deletion disables DS workflow and removes finalizer

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW with `deletionTimestamp` set, finalizer present, `Status.WorkflowID = "uuid-123"`
**When**: The reconciler processes the deletion
**Then**: `DisableWorkflow("uuid-123", ...)` is called on the DS client, the finalizer is removed

**Acceptance Criteria**:
- DS client's `DisableWorkflow` called exactly once with the correct WorkflowID
- Finalizer no longer present in `.metadata.finalizers`
- Reconcile result has no requeue

---

### UT-AW-418-003: DS failure causes requeue

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW with `deletionTimestamp` set, finalizer present, `Status.WorkflowID = "uuid-123"`
**When**: `DisableWorkflow` returns an error (simulating DS unavailability)
**Then**: The reconciler returns `RequeueAfter: 5s`, the finalizer is NOT removed

**Acceptance Criteria**:
- DS client's `DisableWorkflow` called exactly once
- Finalizer still present in `.metadata.finalizers`
- Reconcile result has `RequeueAfter: 5 * time.Second`

---

### UT-AW-418-004: Empty WorkflowID skips DS disable

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW with `deletionTimestamp` set, finalizer present, `Status.WorkflowID = ""`
**When**: The reconciler processes the deletion
**Then**: `DisableWorkflow` is NOT called, the finalizer is removed

**Acceptance Criteria**:
- DS client's `DisableWorkflow` NOT called
- Finalizer removed from `.metadata.finalizers`

---

### UT-AW-418-005: AT activeWorkflowCount refreshed from DS

**BR**: BR-WORKFLOW-007
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW referencing ActionType "RestartPod" being deleted; DS returns `activeWorkflowCount=0`; AT CRD "restart-pod" exists with `Spec.Name = "RestartPod"`
**When**: The reconciler processes the deletion and DS disable succeeds
**Then**: The AT CRD's `Status.ActiveWorkflowCount` is updated to 0

**Acceptance Criteria**:
- `GetActiveWorkflowCount("RestartPod")` called on DS
- AT CRD `.status.activeWorkflowCount` equals 0

---

### UT-AW-418-006: AT count refresh failure does not block deletion

**BR**: BR-WORKFLOW-007
**Type**: Unit
**File**: `test/unit/authwebhook/rw_reconciler_test.go`

**Given**: An RW being deleted; DS disable succeeds; `GetActiveWorkflowCount` returns an error
**When**: The reconciler processes the deletion
**Then**: The finalizer is still removed (deletion not blocked by display-cache failure)

**Acceptance Criteria**:
- Finalizer removed despite AT count refresh failure
- Reconcile result has no error

---

### UT-AW-418-007: Fix A — goroutine fires on DS success

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/rw_handler_delete_test.go`

**Given**: RW DELETE admission request; `DisableWorkflow` succeeds
**When**: The webhook's `handleDelete` processes the request
**Then**: `refreshActionTypeWorkflowCount` goroutine is invoked (AT count refresh runs)

**Acceptance Criteria**:
- `GetActiveWorkflowCount` called (proving the goroutine ran)
- Admission response is Allowed

---

### UT-AW-418-008: Fix A — goroutine skipped on DS failure

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/rw_handler_delete_test.go`

**Given**: RW DELETE admission request; `DisableWorkflow` returns an error
**When**: The webhook's `handleDelete` processes the request
**Then**: `refreshActionTypeWorkflowCount` goroutine is NOT invoked (no stale count written)

**Acceptance Criteria**:
- `GetActiveWorkflowCount` NOT called (goroutine did not fire)
- Admission response is still Allowed (best-effort DELETE)

---

### E2E-AW-418-001: Finalizer present after RW creation

**BR**: BR-WORKFLOW-006
**Type**: E2E
**File**: `test/e2e/authwebhook/03_actiontype_lifecycle_test.go`

**Given**: An RW CRD created in a Kind cluster with the AW reconciler running
**When**: The finalizer controller processes the new RW
**Then**: The `remediationworkflow.kubernaut.ai/catalog-cleanup` finalizer is present in `.metadata.finalizers`

**Acceptance Criteria**:
- `rw.Finalizers` contains `remediationworkflow.kubernaut.ai/catalog-cleanup`

---

### E2E-AW-418-002: AT DELETE succeeds after RW removal (business outcome)

**BR**: BR-WORKFLOW-007.3
**Type**: E2E
**File**: `test/e2e/authwebhook/03_actiontype_lifecycle_test.go` (covered by E2E-AT-300-003)

**Given**: ActionType CRD exists; RW referencing it exists
**When**: RW is deleted (waits for full deletion via finalizer), then AT DELETE is attempted
**Then**: AT DELETE succeeds (DS no longer sees active dependent workflows)

**Acceptance Criteria**:
- `k8sClient.Delete(at)` returns no error
- This validates end-to-end DS catalog consistency without checking the display cache

---

### E2E-AW-418-003: AT activeWorkflowCount correct after finalizer deletion

**BR**: BR-WORKFLOW-007
**Type**: E2E
**File**: `test/e2e/authwebhook/03_actiontype_lifecycle_test.go` (covered by E2E-AT-300-003)

**Given**: ActionType with `activeWorkflowCount=1`; RW deleted via finalizer controller
**When**: RW fully deleted from K8s (finalizer removed)
**Then**: AT CRD's `status.activeWorkflowCount` is 0

**Acceptance Criteria**:
- `kubectl get at` shows WORKFLOWS=0
- The count is updated by the finalizer controller's sequential flow (not a fire-and-forget goroutine)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: DS client (mock `WorkflowCatalogClient` and `ActionTypeWorkflowCounter`), K8s client (`fake.NewClientBuilder()`)
- **Location**: `test/unit/authwebhook/`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: Kind cluster with CRDs, AW (webhook + reconciler), DS, PostgreSQL
- **Location**: `test/e2e/authwebhook/`

---

## 8. Risks & Mitigations

Pre-execution validation performed against mandatory checkpoints (00-kubernaut-core-rules, 01-ai-assistant-behavior).

### Pre-Execution Validation

| Checkpoint | Result | Detail |
|------------|--------|--------|
| A: Type references | PASS | All struct fields (`rw.Status.WorkflowID`, `rw.Spec.ActionType`, `at.Spec.Name`, `at.Status.ActiveWorkflowCount`, reconciler struct fields) verified against type definitions |
| B: Existing patterns | PASS | Existing `mockWorkflowCatalogClient` with function-injection pattern found in `test/unit/authwebhook/remediationworkflow_handler_test.go`; will extend, not reinvent |
| C: Business integration | PASS | Reconciler wired in `cmd/authwebhook/main.go` (lines 224-234); `DSClientAdapter` implements both `WorkflowCatalogClient` and `ActionTypeWorkflowCounter` |
| D: Build validation | PASS | `go build ./...` passes after removing unused `"fmt"` import in `rw_reconciler.go` |

### Identified Risks

| # | Risk | Severity | Mitigation | Status |
|---|------|----------|------------|--------|
| 1 | Unused `"fmt"` import in `rw_reconciler.go` caused build failure | High | Removed import; build verified clean | Mitigated |
| 2 | No existing mock for `ActionTypeWorkflowCounter` interface | Medium | Will add `mockActionTypeWorkflowCounter` using same function-injection pattern as existing mocks | Plan Ready |
| 3 | Fake K8s client does not set `DeletionTimestamp` automatically on Delete | Medium | Tests set `DeletionTimestamp` manually on test objects + add finalizer before reconcile, matching standard controller-runtime test patterns | Plan Ready |
| 4 | E2E tests share `authwebhook-e2e` namespace with `Ordered` execution | Low | New RW fixtures use unique `e2e-418-` prefix names to avoid collisions with existing #300 tests | Plan Ready |
| 5 | Existing RWs may lack finalizer on upgrade | Low | Controller adds finalizer on reconcile of existing RWs (self-healing); E2E tests create fresh RWs so controller processes them immediately | No Action |
| 6 | Existing `E2E-AT-300-003` may need refactoring | Low | With the finalizer, "wait for RW deletion" now implicitly synchronizes DS cleanup. Existing test structure validates the correct business outcome — no code change required | No Action |

### Confidence Assessment

```
Confidence: 92%
Type Safety:          ✅ All referenced fields exist in type definitions
TDD Compliance:       ✅ Test plan created before implementation tests
Integration Status:   ✅ Business code integrated in cmd/authwebhook/main.go
Build Status:         ✅ go build ./... passes after import fix
```

---

## 9. Execution

```bash
# Unit tests (reconciler + Fix A)
go test ./test/unit/authwebhook/... -ginkgo.focus="418" -v

# E2E tests (finalizer lifecycle)
make test-e2e-authwebhook GINKGO_LABEL="actiontype"

# Specific test by ID
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AW-418-003"
go test ./test/e2e/authwebhook/... -ginkgo.focus="E2E-AW-418-002"
```

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2026-03-16 | All 11 scenarios pass (8 UT + 1 E2E dedicated + 2 E2E via #300); status → Active |
| 1.1 | 2026-03-16 | Added Section 8: Risks & Mitigations from pre-execution validation |
| 1.0 | 2026-03-16 | Initial test plan |
