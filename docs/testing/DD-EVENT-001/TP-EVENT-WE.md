# Test Plan: WorkflowExecution K8s Event Observability

**Service**: WorkflowExecution (WE)
**Version**: 1.0
**Created**: 2026-02-11
**Author**: AI Assistant
**Status**: Draft
**Issue**: #74
**BR**: BR-WE-095
**DD**: DD-EVENT-001 v1.1

---

## 1. Scope

### In Scope

- BR-WE-095: K8s Event Observability for WorkflowExecution controller
- 11 events total: 6 existing (v1.0) + 5 new (v1.1)
- Defense-in-depth: UT (FakeRecorder) + IT (corev1.EventList / business flow trails)

### Out of Scope

- Audit event testing (covered by BR-SP-090 pattern)
- Metrics testing (separate maturity requirement)

---

## 2. Event Inventory

| Reason Constant | Type | Priority | Emission Point | Status |
|---|---|---|---|---|
| `ExecutionCreated` | Normal | P1 | reconcilePending: Job/PipelineRun created | Implemented v1.0 |
| `WorkflowCompleted` | Normal | P1 | MarkCompleted: Running → Completed | Implemented v1.0 |
| `WorkflowFailed` | Warning | P1 | MarkFailed/MarkFailedWithReason: → Failed (2 sites) | Implemented v1.0 |
| `LockReleased` | Normal | P1 | ReconcileTerminal: cooldown expired | Implemented v1.0 |
| `WorkflowExecutionDeleted` | Normal | P1 | ReconcileDelete: finalizer cleanup | Implemented v1.0 |
| `PipelineRunCreated` | Normal | P1 | HandleAlreadyExists: PR adopted | Implemented v1.0 |
| `CooldownActive` | Normal | P2 | reconcilePending: cooldown active, execution deferred | Planned v1.1 |
| `WorkflowValidated` | Normal | P2 | reconcilePending: spec validation passed (before execution) | Planned v1.1 |
| `WorkflowValidationFailed` | Warning | P2 | reconcilePending: spec validation failed | Planned v1.1 |
| `CleanupFailed` | Warning | P4 | ReconcileTerminal/ReconcileDelete: exec.Cleanup error | Planned v1.1 |
| `PhaseTransition` | Normal | P3 | Intermediate phase changes (shared constant) | Planned v1.1 |

---

## 3. BR Coverage Matrix

| BR ID | Description | Test Type | Test ID | Status |
|---|---|---|---|---|
| BR-WE-095 | ExecutionCreated event (existing) | Unit | — | ✅ Asserted in controller_test.go |
| BR-WE-095 | WorkflowCompleted event (existing) | Unit | — | ✅ Asserted in controller_test.go |
| BR-WE-095 | WorkflowFailed event (existing) | Unit | — | ✅ Asserted in controller_test.go |
| BR-WE-095 | LockReleased event (existing) | Unit | — | ✅ Asserted in controller_test.go |
| BR-WE-095 | WorkflowExecutionDeleted event (existing) | Unit | — | ✅ Asserted in controller_test.go |
| BR-WE-095 | PipelineRunCreated event (existing) | Unit | — | ✅ Asserted in controller_test.go |
| BR-WE-095 | CooldownActive event | Unit | UT-WE-095-01 | ⏸️ Pending |
| BR-WE-095 | WorkflowValidated event | Unit | UT-WE-095-02 | ⏸️ Pending |
| BR-WE-095 | WorkflowValidationFailed event | Unit | UT-WE-095-03 | ⏸️ Pending |
| BR-WE-095 | CleanupFailed event (ReconcileTerminal) | Unit | UT-WE-095-04 | ⏸️ Pending |
| BR-WE-095 | CleanupFailed event (ReconcileDelete) | Unit | UT-WE-095-05 | ⏸️ Pending |
| BR-WE-095 | PhaseTransition event | Unit | UT-WE-095-06 | ⏸️ Pending |
| BR-WE-095 | Job happy path event trail | Integration | IT-WE-095-01 | ⏸️ Pending |
| BR-WE-095 | Validation failure event trail | Integration | IT-WE-095-02 | ⏸️ Pending |
| BR-WE-095 | Cooldown blocking event trail | Integration | IT-WE-095-03 | ⏸️ Pending |
| BR-WE-095 | Job failure event trail | Integration | IT-WE-095-04 | ⏸️ Pending |

---

## 4. Test Cases

### Unit Tests (FakeRecorder)

**File**: `test/unit/workflowexecution/controller_test.go` (extend existing — FakeRecorder already wired with assertions for 6 existing events)

#### Existing v1.0 Event Assertions (Reference)

The following events are already asserted in `test/unit/workflowexecution/controller_test.go`:

- **ExecutionCreated**: reconcilePending when Job/PipelineRun created
- **WorkflowCompleted**: MarkCompleted on Running → Completed
- **WorkflowFailed**: MarkFailed/MarkFailedWithReason on terminal failure
- **LockReleased**: ReconcileTerminal when cooldown expired
- **WorkflowExecutionDeleted**: ReconcileDelete finalizer cleanup
- **PipelineRunCreated**: HandleAlreadyExists when PR adopted

#### New v1.1 Unit Test Scenarios

#### UT-WE-095-01: CooldownActive on cooldown blocking

**BR**: BR-WE-095
**Type**: Unit
**Category**: Decision Point
**Description**: Verify that cooldown blocking emits CooldownActive event
**Preconditions**: WorkflowExecution in Pending phase, cooldown active (lock not yet released)
**Steps**:
1. Create WorkflowExecution with Phase=Pending
2. Arrange cooldown active (lock held, not yet expired)
3. Call reconcilePending
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "CooldownActive" + "execution deferred"

#### UT-WE-095-02: WorkflowValidated on successful validation

**BR**: BR-WE-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that successful spec validation emits WorkflowValidated event
**Preconditions**: WorkflowExecution in Pending phase, spec valid
**Steps**:
1. Create WorkflowExecution with Phase=Pending and valid spec
2. Call reconcilePending (before execution starts)
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "WorkflowValidated"

#### UT-WE-095-03: WorkflowValidationFailed on spec validation failure

**BR**: BR-WE-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that spec validation failure emits WorkflowValidationFailed Warning event
**Preconditions**: WorkflowExecution in Pending phase, spec invalid
**Steps**:
1. Create WorkflowExecution with Phase=Pending and invalid spec
2. Call reconcilePending
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "WorkflowValidationFailed"

#### UT-WE-095-04: CleanupFailed on exec.Cleanup error (ReconcileTerminal)

**BR**: BR-WE-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that exec.Cleanup error in ReconcileTerminal emits CleanupFailed Warning event
**Preconditions**: WorkflowExecution terminal, exec.Cleanup returns error
**Steps**:
1. Create WorkflowExecution in terminal phase (Completed/Failed)
2. Mock exec.Cleanup to return error
3. Call ReconcileTerminal
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "CleanupFailed"

#### UT-WE-095-05: CleanupFailed on exec.Cleanup error (ReconcileDelete)

**BR**: BR-WE-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that exec.Cleanup error in ReconcileDelete emits CleanupFailed Warning event
**Preconditions**: WorkflowExecution deletion in progress, exec.Cleanup returns error
**Steps**:
1. Create WorkflowExecution with DeletionTimestamp set
2. Mock exec.Cleanup to return error
3. Call ReconcileDelete
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "CleanupFailed"

#### UT-WE-095-06: PhaseTransition on Pending → Running

**BR**: BR-WE-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that intermediate phase change emits PhaseTransition event with descriptive message
**Preconditions**: WorkflowExecution transitioning Pending → Running
**Steps**:
1. Create WorkflowExecution in Pending phase
2. Trigger transition to Running (e.g., Job/PipelineRun observed)
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Pending" + "Running"

### Integration Tests (Business Flow Event Trails)

**File**: `test/integration/workflowexecution/events_test.go` (new)

**Note**: Existing `test/integration/workflowexecution/reconciler_test.go` already asserts on LockReleased event.

#### IT-WE-095-01: Job happy path event trail

**BR**: BR-WE-095
**Type**: Integration
**Category**: Happy Path (Business Flow)
**Description**: Job lifecycle happy path produces correct event sequence
**Preconditions**: envtest with WE controller, Job execution enabled
**Steps**:
1. Create WorkflowExecution CRD (Job backend)
2. Wait for Phase=Completed
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain (in order): ExecutionCreated, WorkflowCompleted, LockReleased

#### IT-WE-095-02: Validation failure event trail

**BR**: BR-WE-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: Spec validation failure produces correct event sequence
**Preconditions**: envtest with WE controller
**Steps**:
1. Create WorkflowExecution CRD with invalid spec
2. Wait for reconciliation
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: WorkflowValidationFailed

#### IT-WE-095-03: Cooldown blocking event trail

**BR**: BR-WE-095
**Type**: Integration
**Category**: Decision Point (Business Flow)
**Description**: Cooldown active produces CooldownActive event
**Preconditions**: envtest with WE controller, lock held (cooldown active)
**Steps**:
1. Create WorkflowExecution CRD while cooldown active
2. Wait for reconciliation
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: CooldownActive

#### IT-WE-095-04: Job failure event trail

**BR**: BR-WE-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: Job failure produces correct event sequence
**Preconditions**: envtest with WE controller, Job execution fails
**Steps**:
1. Create WorkflowExecution CRD (Job backend)
2. Arrange Job to fail
3. Wait for Phase=Failed
4. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: ExecutionCreated, WorkflowFailed

---

## 5. Coverage Targets

| Metric | Target | Actual |
|---|---|---|
| Unit Test Coverage (all 12 events) | 100% | 6/12 (v1.0) + 6/6 (v1.1) |
| IT Business Flow Coverage | 4 flows | ⏸️ |
| BR Coverage | 100% | ⏸️ |

---

## 6. Test File Locations

| Test Category | File |
|---|---|
| Unit Tests | `test/unit/workflowexecution/controller_test.go` (extend) |
| Integration Tests | `test/integration/workflowexecution/events_test.go` (new) |
| Integration (existing) | `test/integration/workflowexecution/reconciler_test.go` (LockReleased asserted) |

---

## 7. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | | | ⏸️ |
| Reviewer | | | ⏸️ |
| Approver | | | ⏸️ |
