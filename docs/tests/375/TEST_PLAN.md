# Test Plan: WFE Target Resource Lock Release After Terminal Phase

**Feature**: Fix target resource lock not released after WFE completion, blocking multi-cycle remediation
**Version**: 1.0
**Created**: 2026-03-14
**Author**: Kubernaut AI Team
**Status**: Draft
**Branch**: `feature/v1.0-bugfixes-demos`

**Authority**:
- [BR-WE-009]: Resource Locking -- Prevent Parallel Execution
- [BR-WE-010]: Cooldown -- Prevent Redundant Sequential Execution
- [DD-WE-003 v1.1]: Resource Lock Persistence (Deterministic Name) -- Lock Lifecycle + Pre-Execution Cleanup
- [GitHub #375]: WFE target resource lock not released after completion
- [GitHub #374]: Job name collision on repeated remediation of same target

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **`MarkCompleted` / `MarkFailed` return values**: Both methods must schedule a `RequeueAfter` so `ReconcileTerminal` runs to clean up the execution resource (root cause fix)
- **`ReconcileTerminal` cleanup for Job backend**: Verify that after cooldown expires, the Job execution resource is deleted, releasing the lock

### Out of Scope

- **Pre-execution stale Job cleanup in `reconcilePending`** — handled by [#374](https://github.com/jordigilh/kubernaut/issues/374) and its [test plan](../../testing/374/TEST_PLAN.md). That fix adds defense-in-depth by detecting and cleaning stale completed/failed Jobs at `AlreadyExists` time. It complements this fix (#375) which addresses the root cause.
- Tekton PipelineRun locking mechanics (separate code path with `HandleAlreadyExists`; not affected by #375)
- `GenerationChangedPredicate` removal or modification (the event filter is correct for its purpose; the fix is in scheduling explicit requeues)
- Changes to cooldown period defaults or configuration (BR-WE-010 is already working correctly)
- Job TTL alignment (cosmetic improvement, not the root cause)

### Coordination with #374

Issues #375 and #374 are complementary:

- **#375 (this plan)**: Fixes the **normal lifecycle** — `MarkCompleted`/`MarkFailed` schedule `RequeueAfter` so `ReconcileTerminal` runs and calls `exec.Cleanup()`.
- **#374**: Adds **defense-in-depth** — `reconcilePending` detects stale terminal Jobs at `AlreadyExists` time and cleans them up. Handles edge cases where `ReconcileTerminal` was missed (controller restart, lost requeues).

Suggested merge order: **#375 first** (root cause), **#374 second** (safety net).

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Fix via `RequeueAfter` in terminal methods, not predicate changes | `RequeueAfter` is immune to `GenerationChangedPredicate` filtering; the predicate (WE-BUG-001) prevents real duplicate reconcile issues and must stay |
| Pre-execution stale Job cleanup deferred to #374 | Separate concern (defense-in-depth); this plan focuses on the root cause (missing requeue) |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`MarkCompleted`/`MarkFailed` return value logic, pre-execution stale resource detection logic)
- **Integration**: >=80% of integration-testable code (multi-cycle WFE reconciliation with Job backend through envtest)

### 2-Tier Minimum

- **Unit tests**: Validate the `ctrl.Result` return values and stale resource detection logic in isolation (fast, deterministic)
- **Integration tests**: Validate the full multi-cycle WFE lifecycle with envtest: create WFE -> complete -> verify lock release -> create second WFE -> verify success

### Business Outcome Quality Bar

Tests validate the operator-facing outcome: "After a remediation completes, the system can remediate the same target again" -- not just "RequeueAfter is set". Each test scenario verifies that multi-cycle remediation flows are unblocked.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `MarkCompleted` return value | ~5 (change: return `ctrl.Result{RequeueAfter: cooldown}`) |
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `MarkFailed` return value | ~5 (change: return `ctrl.Result{RequeueAfter: cooldown}`) |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `ReconcileTerminal` (cooldown wait + `exec.Cleanup`) | ~80 |
| `pkg/workflowexecution/executor/job.go` | `Cleanup`, `Create` | ~40 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-009 | Lock released after successful completion enables sequential execution | P0 | Unit | UT-WE-375-001 | Pending |
| BR-WE-009 | Lock released after failure enables sequential execution | P0 | Unit | UT-WE-375-002 | Pending |
| BR-WE-010 | Cooldown enforced via ReconcileTerminal requeue (default fallback) | P0 | Unit | UT-WE-375-003 | Pending |
| BR-WE-009 | Multi-cycle remediation: second WFE succeeds after first completes | P0 | Integration | IT-WE-375-001 | Pending |
| BR-WE-010 | Multi-cycle remediation blocked during cooldown, succeeds after | P0 | Integration | IT-WE-375-002 | Pending |

**Note**: Pre-execution stale Job cleanup tests (stale completed/failed Job detection, running Job lock preservation, race conditions) are covered by [#374's test plan](../../testing/374/TEST_PLAN.md) — see UT-WE-374-001 through UT-WE-374-004 and IT-WE-374-001/002.

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-WE-375-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `WE` (WorkflowExecution)
- **BR_NUMBER**: `375` (GitHub issue)
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `MarkCompleted`, `MarkFailed` return values; stale resource detection logic. Target >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-375-001` | After successful remediation, the controller schedules lock release (RequeueAfter set to cooldown period in MarkCompleted) | Pending |
| `UT-WE-375-002` | After failed remediation, the controller schedules lock release (RequeueAfter set to cooldown period in MarkFailed) | Pending |
| `UT-WE-375-003` | Default cooldown period (5m) used when CooldownPeriod is zero in MarkCompleted/MarkFailed | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Full WFE lifecycle through envtest with Job backend. Target >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-375-001` | Operator can remediate the same target resource across two consecutive WFE cycles (Cycle 1 completes -> lock released -> Cycle 2 succeeds) | Pending |
| `IT-WE-375-002` | Cooldown enforcement: a second WFE created within cooldown window is deferred (Pending), then proceeds after cooldown expires | Pending |

### Tier Skip Rationale (if any tier is omitted)

- **E2E**: Deferred. The fix is controller-internal (return values and reconcile logic). E2E would require deploying the controller to Kind and running a multi-cycle scenario with real Jobs. The existing `memory-escalation` demo scenario in OCP validation serves as the live E2E validation.
- **Pre-execution cleanup tests**: Deferred to [#374 test plan](../../testing/374/TEST_PLAN.md). That plan covers `IsCompleted()`, stale Job detection, running Job lock preservation, and race conditions (UT-WE-374-001 through UT-WE-374-004, IT-WE-374-001/002).

---

## 6. Test Cases (Detail)

### UT-WE-375-001: MarkCompleted schedules lock release requeue

**BR**: BR-WE-009, DD-WE-003 v1.1
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Given**: A WFE in Running phase with a configured CooldownPeriod of 1 minute
**When**: `MarkCompleted` is called with a valid PipelineRun
**Then**: The returned `ctrl.Result` has `RequeueAfter` equal to the configured cooldown period (1 minute)

**Acceptance Criteria**:
- `result.RequeueAfter` equals 1 minute (not zero)
- WFE phase is Completed (existing behavior preserved)
- CompletionTime is set (existing behavior preserved)

---

### UT-WE-375-002: MarkFailed schedules lock release requeue

**BR**: BR-WE-009, DD-WE-003 v1.1
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Given**: A WFE in Running phase with a configured CooldownPeriod of 1 minute
**When**: `MarkFailed` is called with a valid PipelineRun
**Then**: The returned `ctrl.Result` has `RequeueAfter` equal to the configured cooldown period (1 minute)

**Acceptance Criteria**:
- `result.RequeueAfter` equals 1 minute (not zero)
- WFE phase is Failed (existing behavior preserved)
- FailureDetails populated (existing behavior preserved)

---

### UT-WE-375-003: Default cooldown used when CooldownPeriod is zero

**BR**: BR-WE-010
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Given**: A WFE in Running phase with CooldownPeriod set to 0 (default)
**When**: `MarkCompleted` is called
**Then**: The returned `ctrl.Result` has `RequeueAfter` equal to `DefaultCooldownPeriod` (5 minutes)

**Acceptance Criteria**:
- `result.RequeueAfter` equals 5 minutes (DefaultCooldownPeriod)
- Fallback logic is consistent with ReconcileTerminal's cooldown calculation

---

### IT-WE-375-001: Multi-cycle remediation succeeds after lock release

**BR**: BR-WE-009, DD-WE-003 v1.1
**Type**: Integration
**File**: `test/integration/workflowexecution/lock_release_test.go`

**Given**: A WFE (Cycle 1) targeting `test-ns/Deployment/test-app` has completed and its Job exists in the execution namespace
**When**: ReconcileTerminal runs after cooldown expires, then a new WFE (Cycle 2) is created for the same target
**Then**: Cycle 2's Job is created successfully and the WFE transitions to Running

**Acceptance Criteria**:
- Cycle 1 Job is deleted by ReconcileTerminal after cooldown
- `LockReleased` event emitted for Cycle 1
- Cycle 2 WFE reaches Running phase
- Cycle 2 Job exists in execution namespace with the same deterministic name
- No "target resource locked" error

---

### IT-WE-375-002: Cooldown enforcement with eventual lock release

**BR**: BR-WE-010
**Type**: Integration
**File**: `test/integration/workflowexecution/lock_release_test.go`

**Given**: A WFE (Cycle 1) completed 1 second ago (well within 1 minute test cooldown)
**When**: A new WFE (Cycle 2) is created for the same target immediately
**Then**: Cycle 2 is deferred in Pending phase with `CooldownActive` event, then eventually proceeds after cooldown expires

**Acceptance Criteria**:
- Cycle 2 starts in Pending (deferred by cooldown)
- `CooldownActive` event emitted for Cycle 2
- After cooldown expires + ReconcileTerminal cleans up Cycle 1 Job, Cycle 2 transitions to Running
- End-to-end timing respects the configured cooldown period

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fake.NewClientBuilder()` for K8s client; mock AuditStore, mock EventRecorder
- **Location**: `test/unit/workflowexecution/controller_test.go`
- **Patterns**: Follow existing `MarkCompleted`/`MarkFailed` test patterns (lines 911-1034 and 1040-1180)

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see No-Mocks Policy)
- **Infrastructure**: envtest (real K8s API server) with Job backend
- **Location**: `test/integration/workflowexecution/lock_release_test.go`
- **Patterns**: Follow existing `job_lifecycle_integration_test.go` and `cooldown_config_test.go` patterns

---

## 8. Execution

```bash
# Unit tests (all WE unit tests)
go test ./test/unit/workflowexecution/... -v

# Unit tests (issue 375 only)
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-375"

# Integration tests (issue 375 only)
go test ./test/integration/workflowexecution/... -ginkgo.focus="IT-WE-375"

# Integration tests (all WE integration tests)
go test ./test/integration/workflowexecution/... -v
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-14 | Initial test plan for #375 (WFE lock release) |
