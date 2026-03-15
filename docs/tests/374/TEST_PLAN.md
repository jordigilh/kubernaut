# Test Plan: WFE Job Name Collision Fix (#374)

**Feature**: Pre-execution cleanup of completed Jobs to unblock sequential remediation cycles
**Version**: 1.1
**Created**: 2026-03-14
**Author**: Kubernaut Architecture Team
**Status**: Active
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-WE-009]: Resource Locking -- Prevent Parallel Execution
- [BR-WE-010]: Cooldown -- Prevent Redundant Sequential Execution
- [BR-WE-011]: Lock Release -- Prevent Permanent Resource Blocking
- [DD-WE-003]: Resource Lock Persistence via Deterministic Naming

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `pkg/workflowexecution/executor/job.go`: New `IsCompleted(ctx, targetResource, namespace)` method to check if an existing Job is in a terminal state
- `internal/controller/workflowexecution/workflowexecution_controller.go`: New `handleJobAlreadyExists()` method and modified `reconcilePending()` AlreadyExists handler to clean up completed Jobs before retrying

### Out of Scope

- Tekton PipelineRun AlreadyExists handling (already has its own `HandleAlreadyExists` path)
- TTL controller behavior (K8s built-in, not testable in isolation)
- Cooldown period logic (unchanged, already covered by existing tests)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `IsCompleted` on `JobExecutor` only (not on `Executor` interface) | Job-specific concern; Tekton already has `HandleAlreadyExists` with its own logic |
| `IsCompleted` takes `targetResource string` (not full WFE) | Only the target resource is needed to derive the deterministic Job name; simpler API |
| Delete with `PropagationPolicy=Background` | Consistent with existing `Cleanup()` method; avoids blocking on pod deletion |
| Retry `Create()` after cleanup | Ensures the new WFE proceeds immediately instead of waiting for next reconcile |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code -- `IsCompleted()` pure condition-checking logic (all 4 branches: succeeded, running, failed, not-found)
- **Integration**: >=80% of **integration-testable** code -- `handleJobAlreadyExists()` controller wiring (K8s I/O: Get, Delete, Create) and full `reconcilePending()` AlreadyExists branch

### 2-Tier Minimum

Every business requirement gap is covered by 2 test tiers (UT + IT):
- **Unit tests** validate `IsCompleted()` return values for each Job state (pure logic, no I/O)
- **Integration tests** validate the full controller wiring: `handleJobAlreadyExists()` cleanup+retry and end-to-end sequential remediation lifecycle

### Business Outcome Quality Bar

Tests validate that operators see remediation succeed on repeated cycles (not just that code paths execute).

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/job.go` | `IsCompleted` | ~20 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `handleJobAlreadyExists()`, `reconcilePending()` AlreadyExists branch | ~50 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-009 | Prevent parallel execution (running Job blocks new WFE) | P0 | Unit | UT-WE-374-002 | Pass |
| BR-WE-011 | Completed Job cleaned up, new WFE proceeds | P0 | Unit | UT-WE-374-001 | Pass |
| BR-WE-011 | Failed Job cleaned up, new WFE proceeds | P0 | Unit | UT-WE-374-003 | Pass |
| BR-WE-011 | Race condition: Job disappears between check and delete | P1 | Unit | UT-WE-374-004 | Pass |
| BR-WE-009 | Running Job blocks concurrent WFE (full lifecycle) | P0 | Integration | IT-WE-374-002 | Compile-verified |
| BR-WE-011 | WFE1 completes, WFE2 succeeds on same target (full lifecycle) | P0 | Integration | IT-WE-374-001 | Compile-verified |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: WE (WorkflowExecution)
- **BR_NUMBER**: 374 (Issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/workflowexecution/executor/job.go` -- `IsCompleted()` method, 100% branch coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-374-001` | Completed Job is cleaned up and new WFE proceeds -- operator sees remediation succeed on repeated cycles | Pass |
| `UT-WE-374-002` | Running Job blocks new WFE -- concurrent lock is preserved, operator sees "target resource locked" | Pass |
| `UT-WE-374-003` | Failed Job is cleaned up and new WFE proceeds -- stale failed Jobs do not permanently lock a target | Pass |
| `UT-WE-374-004` | Job not found after AlreadyExists (race) -- controller handles gracefully | Pass |

### Tier 2: Integration Tests

**Testable code scope**: Full controller reconciliation with real K8s API (envtest), covering `handleJobAlreadyExists()` wiring

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-374-001` | Full lifecycle: WFE1 completes -> WFE2 for same target succeeds (no manual cleanup needed) | Compile-verified |
| `IT-WE-374-002` | Concurrent lock preserved: WFE1 running -> WFE2 for same target is locked | Compile-verified |

---

## 6. Test Cases (Detail)

### UT-WE-374-001: Completed Job cleanup enables new WFE

**BR**: BR-WE-011
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_test.go`

**Given**: A completed Job exists in the execution namespace for a target resource
**When**: `IsCompleted(ctx, targetResource, namespace)` is called
**Then**: Returns `(true, nil)` indicating the Job can be safely cleaned up

**Acceptance Criteria**:
- `IsCompleted()` returns `true` for Jobs with `JobComplete` condition = True
- Job state is checked via `job.Status.Conditions`

### UT-WE-374-002: Running Job preserves concurrent lock

**BR**: BR-WE-009
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_test.go`

**Given**: A running Job (no terminal conditions, Active > 0) exists in the execution namespace
**When**: `IsCompleted(ctx, targetResource, namespace)` is called
**Then**: Returns `(false, nil)` indicating the Job is still active and the lock is valid

**Acceptance Criteria**:
- `IsCompleted()` returns `false` for Jobs with no terminal conditions
- The WFE should be marked failed with "target resource locked" by the caller

### UT-WE-374-003: Failed Job cleanup enables new WFE

**BR**: BR-WE-011
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_test.go`

**Given**: A failed Job (JobFailed condition = True) exists in the execution namespace
**When**: `IsCompleted(ctx, targetResource, namespace)` is called
**Then**: Returns `(true, nil)` indicating the Job can be safely cleaned up

**Acceptance Criteria**:
- `IsCompleted()` returns `true` for Jobs with `JobFailed` condition = True
- Failed Jobs do not permanently lock the target resource

### UT-WE-374-004: Race condition -- Job disappears

**BR**: BR-WE-011
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_test.go`

**Given**: No Job exists in the execution namespace (race: deleted between AlreadyExists and IsCompleted)
**When**: `IsCompleted(ctx, targetResource, namespace)` is called
**Then**: Returns `(false, NotFoundError)` -- caller retries Create which should succeed

**Acceptance Criteria**:
- `IsCompleted()` returns a NotFound error when Job does not exist
- Controller can handle this gracefully by retrying Create

### IT-WE-374-001: Sequential remediation succeeds after completion

**BR**: BR-WE-011
**Type**: Integration
**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go`

**Given**: WFE1 targeting resource R created and completed (Job cleaned up by controller)
**When**: WFE2 targeting the same resource R is created
**Then**: WFE2 succeeds (Job created, runs, completes)

**Acceptance Criteria**:
- WFE1 reaches Completed phase
- WFE2 reaches Running phase (Job created successfully)
- WFE2 reaches Completed phase after Job success simulation
- No manual cleanup required between WFE1 and WFE2

### IT-WE-374-002: Concurrent lock preserved for running Job

**BR**: BR-WE-009
**Type**: Integration
**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go`

**Given**: WFE1 targeting resource R is Running (Job actively executing)
**When**: WFE2 targeting the same resource R is created
**Then**: WFE2 fails with "target resource locked" (concurrent execution prevented)

**Acceptance Criteria**:
- WFE1 remains in Running phase
- WFE2 transitions to Failed phase
- WFE2 failure message contains "already exists" or "target resource locked"

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fake.NewClientBuilder()` for K8s API (pre-created Job objects in various states)
- **Location**: `test/unit/workflowexecution/executor_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (real envtest K8s API)
- **Infrastructure**: EnvTest (real kube-apiserver + etcd), PostgreSQL, Redis, DataStorage
- **Location**: `test/integration/workflowexecution/job_lifecycle_integration_test.go`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/workflowexecution/... -ginkgo.focus="Issue #374"

# Integration tests
make test-integration-workflowexecution -ginkgo.focus="Issue #374"

# Specific test by ID
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-374-001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-14 | Initial test plan |
| 1.1 | 2026-03-14 | Moved to `docs/tests/374/` per project convention; fixed `IsCompleted` signature; clarified unit vs integration coverage tiers; added `handleJobAlreadyExists` to integration-testable inventory |
