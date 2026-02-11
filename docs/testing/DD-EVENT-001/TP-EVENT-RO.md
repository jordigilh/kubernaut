# Test Plan: RemediationOrchestrator K8s Event Observability

**Service**: RemediationOrchestrator (RO)
**Version**: 1.0
**Created**: 2026-02-11
**Author**: AI Assistant
**Status**: Draft
**Issue**: #77
**BR**: BR-ORCH-095
**DD**: DD-EVENT-001 v1.1

---

## 1. Scope

### In Scope

- BR-ORCH-095: K8s Event Observability for RemediationOrchestrator controller
- 14 events implemented by this team (0 existing + 14 new — RO has zero events today)
- Defense-in-depth: UT (FakeRecorder) + IT (corev1.EventList)

### Out of Scope

- Audit event testing (covered by BR-SP-090 pattern)
- Metrics testing (separate maturity requirement)
- Child CRD event testing (SignalProcessing, AIAnalysis, WorkflowExecution — owned by respective teams)

---

## 2. Event Inventory

| Reason Constant | Type | Priority | Emission Point | Status |
|---|---|---|---|---|
| `RemediationCreated` | Normal | P1 | Phase init: "" → Pending (accepted) | Planned |
| `RemediationCompleted` | Normal | P1 | transitionToCompleted (terminal success) | Planned |
| `RemediationFailed` | Warning | P1 | transitionToFailed (terminal failure) | Planned |
| `RemediationTimeout` | Warning | P1 | handleGlobalTimeout / handlePhaseTimeout | Planned |
| `ApprovalRequired` | Normal | P2 | handleAnalyzingPhase: RAR created | Planned |
| `ApprovalGranted` | Normal | P2 | handleAwaitingApprovalPhase: RAR approved | Planned |
| `ApprovalRejected` | Warning | P2 | handleAwaitingApprovalPhase: RAR rejected | Planned |
| `ApprovalExpired` | Warning | P2 | handleAwaitingApprovalPhase: RAR expired | Planned |
| `EscalatedToManualReview` | Warning | P2 | Unrecoverable failure triggers escalation | Planned |
| `RecoveryInitiated` | Normal | P2 | Recovery attempt started | Planned |
| `NotificationCreated` | Normal | P2 | NotificationRequest CRD created | Planned |
| `CooldownActive` | Normal | P2 | Remediation skipped due to cooldown | Planned |
| `ConsecutiveFailureBlocked` | Warning | P2 | Target blocked due to consecutive failures | Planned |
| `PhaseTransition` | Normal | P3 | Intermediate phase changes | Planned |

**RO phase lifecycle**: Pending → Processing → Analyzing → [AwaitingApproval] → Executing → Completed

**Terminal phases**: Completed, Failed, TimedOut, Skipped, Cancelled

---

## 3. BR Coverage Matrix

| BR ID | Description | Test Type | Test ID | Status |
|---|---|---|---|---|
| BR-ORCH-095 | RemediationCreated event on acceptance | Unit | UT-RO-095-01 | ⏸️ Pending |
| BR-ORCH-095 | RemediationCompleted event on successful lifecycle | Unit | UT-RO-095-02 | ⏸️ Pending |
| BR-ORCH-095 | RemediationFailed event on lifecycle failure | Unit | UT-RO-095-03 | ⏸️ Pending |
| BR-ORCH-095 | RemediationTimeout event on global timeout | Unit | UT-RO-095-04 | ⏸️ Pending |
| BR-ORCH-095 | ApprovalRequired event on RAR creation | Unit | UT-RO-095-05 | ⏸️ Pending |
| BR-ORCH-095 | ApprovalGranted event on RAR approval | Unit | UT-RO-095-06 | ⏸️ Pending |
| BR-ORCH-095 | ApprovalRejected event on RAR rejection | Unit | UT-RO-095-07 | ⏸️ Pending |
| BR-ORCH-095 | ApprovalExpired event on RAR deadline passed | Unit | UT-RO-095-08 | ⏸️ Pending |
| BR-ORCH-095 | EscalatedToManualReview event on unrecoverable failure | Unit | UT-RO-095-09 | ⏸️ Pending |
| BR-ORCH-095 | RecoveryInitiated event on recovery attempt | Unit | UT-RO-095-10 | ⏸️ Pending |
| BR-ORCH-095 | NotificationCreated event on NT CRD creation | Unit | UT-RO-095-11 | ⏸️ Pending |
| BR-ORCH-095 | CooldownActive event on cooldown blocking | Unit | UT-RO-095-12 | ⏸️ Pending |
| BR-ORCH-095 | ConsecutiveFailureBlocked event on threshold exceeded | Unit | UT-RO-095-13 | ⏸️ Pending |
| BR-ORCH-095 | PhaseTransition event on intermediate transitions | Unit | UT-RO-095-14 | ⏸️ Pending |
| BR-ORCH-095 | Happy path event trail (auto-approve) | Integration | IT-RO-095-01 | ⏸️ Pending |
| BR-ORCH-095 | Approval flow event trail | Integration | IT-RO-095-02 | ⏸️ Pending |
| BR-ORCH-095 | Timeout event trail | Integration | IT-RO-095-03 | ⏸️ Pending |
| BR-ORCH-095 | Consecutive failure blocking event trail | Integration | IT-RO-095-04 | ⏸️ Pending |

---

## 4. Infrastructure: FakeRecorder Wiring (CRITICAL)

**Current state**: RO unit tests pass `nil` for EventRecorder in reconciler construction.

**Blocker**: Event emission tests cannot be validated until FakeRecorder is wired.

### Required Changes

1. **`test/unit/remediationorchestrator/controller/test_helpers.go`**:
   - Add helper `NewTestRecorder() *record.FakeRecorder` returning `record.NewFakeRecorder(20)`
   - OR add `recorder *record.FakeRecorder` to shared test setup available to reconciler construction

2. **`test/unit/remediationorchestrator/controller/reconciler_test.go`**:
   - Replace `nil` EventRecorder parameter with `record.NewFakeRecorder(20)` in `prodcontroller.NewReconciler()` calls

3. **`test/unit/remediationorchestrator/controller/reconcile_phases_test.go`**:
   - Replace `nil` EventRecorder parameter with `record.NewFakeRecorder(20)` in reconciler construction

4. **`test/unit/remediationorchestrator/controller/helper_functions_test.go`** and **`audit_events_test.go`**:
   - Replace `nil` EventRecorder parameter with `record.NewFakeRecorder(20)` for consistency

**Dependency**: RO team is aware and will hold event implementation until this infrastructure is complete.

---

## 5. Test Cases

### Unit Tests (FakeRecorder)

**File**: `test/unit/remediationorchestrator/controller/reconcile_phases_test.go` (extend existing)

#### UT-RO-095-01: RemediationCreated on acceptance

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that reconciling a newly accepted RemediationRequest ("" → Pending or init) emits RemediationCreated event
**Preconditions**: RemediationRequest in Pending phase (accepted), FakeRecorder injected
**Steps**:
1. Create RemediationRequest with Phase=Pending (or initial acceptance)
2. Call Reconcile (or reconcilePending phase logic)
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "RemediationCreated" + acceptance/processing started message

#### UT-RO-095-02: RemediationCompleted on successful lifecycle

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that successful remediation completion emits RemediationCompleted event
**Preconditions**: RemediationRequest in Executing phase, WorkflowExecution completed
**Steps**:
1. Create RemediationRequest with Phase=Executing, completed WE child
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "RemediationCompleted"

#### UT-RO-095-03: RemediationFailed on lifecycle failure

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that terminal failure emits RemediationFailed Warning event
**Preconditions**: RemediationRequest with failed child (SP, AI, or WE)
**Steps**:
1. Create RemediationRequest with failed child CRD
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "RemediationFailed"

#### UT-RO-095-04: RemediationTimeout on global timeout

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that global or phase timeout emits RemediationTimeout Warning event
**Preconditions**: RemediationRequest with StartTime (or phase start) exceeding timeout threshold
**Steps**:
1. Create RemediationRequest with newRemediationRequestWithTimeout (past timeout)
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "RemediationTimeout"

#### UT-RO-095-05: ApprovalRequired on RAR creation

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Decision Point
**Description**: Verify that creating RemediationApprovalRequest emits ApprovalRequired event
**Preconditions**: RemediationRequest in Analyzing phase, AIAnalysis with ApprovalRequired=true
**Steps**:
1. Create RemediationRequest in Analyzing phase with low-confidence AIAnalysis
2. Call Reconcile (handleAnalyzingPhase creates RAR)
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "ApprovalRequired"

#### UT-RO-095-06: ApprovalGranted on RAR approval

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that RAR approval emits ApprovalGranted event
**Preconditions**: RemediationRequest in AwaitingApproval phase, RAR with Decision=Approved
**Steps**:
1. Create RR in AwaitingApproval with approved RAR
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "ApprovalGranted"

#### UT-RO-095-07: ApprovalRejected on RAR rejection

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that RAR rejection emits ApprovalRejected Warning event
**Preconditions**: RemediationRequest in AwaitingApproval phase, RAR with Decision=Rejected
**Steps**:
1. Create RR in AwaitingApproval with rejected RAR
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "ApprovalRejected"

#### UT-RO-095-08: ApprovalExpired on RAR deadline passed

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that expired RAR emits ApprovalExpired Warning event
**Preconditions**: RemediationRequest in AwaitingApproval phase, RAR with Decision=Expired
**Steps**:
1. Create RR in AwaitingApproval with expired RAR (newRemediationApprovalRequestExpired)
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "ApprovalExpired"

#### UT-RO-095-09: EscalatedToManualReview on unrecoverable failure

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that unrecoverable failure triggers EscalatedToManualReview Warning event
**Preconditions**: RemediationRequest with permanent/unrecoverable failure that triggers escalation
**Steps**:
1. Create RR with conditions triggering escalation path
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "EscalatedToManualReview"

#### UT-RO-095-10: RecoveryInitiated on recovery attempt

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Happy Path / Recovery
**Description**: Verify that recovery attempt start emits RecoveryInitiated event
**Preconditions**: RemediationRequest in state where recovery is triggered
**Steps**:
1. Create RR with recoverable failure condition
2. Call Reconcile (recovery initiated)
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "RecoveryInitiated"

#### UT-RO-095-11: NotificationCreated on NT CRD creation

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that creating NotificationRequest CRD emits NotificationCreated event
**Preconditions**: RemediationRequest in state where notification is created (e.g., approval requested)
**Steps**:
1. Create RR in Analyzing phase requiring approval (triggers NotificationRequest)
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "NotificationCreated"

#### UT-RO-095-12: CooldownActive on cooldown blocking

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Decision Point
**Description**: Verify that cooldown blocking emits CooldownActive event
**Preconditions**: MockRoutingEngine returns blocking condition due to cooldown
**Steps**:
1. Configure MockRoutingEngine to return cooldown blocking
2. Create RR in Pending
3. Call Reconcile
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "CooldownActive"

#### UT-RO-095-13: ConsecutiveFailureBlocked on threshold exceeded

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that consecutive failure threshold emits ConsecutiveFailureBlocked Warning event
**Preconditions**: MockRoutingEngine returns blocking condition due to consecutive failures
**Steps**:
1. Configure MockRoutingEngine to return consecutive-failure blocking
2. Create RR in Pending
3. Call Reconcile
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "ConsecutiveFailureBlocked"

#### UT-RO-095-14: PhaseTransition on intermediate transitions

**BR**: BR-ORCH-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that intermediate phase changes emit PhaseTransition event with descriptive message
**Preconditions**: RemediationRequest transitioning between non-terminal phases (Pending→Processing, Processing→Analyzing, Analyzing→AwaitingApproval, AwaitingApproval→Executing)
**Steps**:
1. Create RR in Pending phase
2. Trigger transition to Processing (or other intermediate)
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + source phase + target phase

### Integration Tests (corev1.EventList)

**File**: `test/integration/remediationorchestrator/events_test.go` (new)

#### IT-RO-095-01: Happy path event trail (auto-approve)

**BR**: BR-ORCH-095
**Type**: Integration
**Category**: Happy Path (Business Flow)
**Description**: Complete auto-approve lifecycle produces correct event sequence: RemediationCreated → PhaseTransition(x3) → RemediationCompleted
**Preconditions**: envtest with RO controller, high-confidence AIAnalysis (no approval)
**Steps**:
1. Create RemediationRequest CRD
2. Wait for Phase=Completed
3. List corev1.Events by involvedObject.name (RR)
**Expected Result**: Events contain (in order): RemediationCreated, PhaseTransition (Pending→Processing), PhaseTransition (Processing→Analyzing), PhaseTransition (Analyzing→Executing), RemediationCompleted

#### IT-RO-095-02: Approval flow event trail

**BR**: BR-ORCH-095
**Type**: Integration
**Category**: Decision Point (Business Flow)
**Description**: Approval-required flow produces correct event sequence: RemediationCreated → ApprovalRequired → ApprovalGranted → RemediationCompleted
**Preconditions**: envtest with RO controller, low-confidence AIAnalysis (approval required)
**Steps**:
1. Create RemediationRequest CRD (low confidence path)
2. Wait for ApprovalRequired (RAR created)
3. Approve RAR
4. Wait for Phase=Completed
5. List corev1.Events by involvedObject.name
**Expected Result**: Events contain (in order): RemediationCreated, ApprovalRequired, ApprovalGranted, RemediationCompleted

#### IT-RO-095-03: Timeout event trail

**BR**: BR-ORCH-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: Global timeout produces correct event sequence: RemediationCreated → RemediationTimeout
**Preconditions**: envtest with RO controller, RR with stale StartTime (past global timeout)
**Steps**:
1. Create RemediationRequest CRD with expired start time
2. Wait for reconciliation
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: RemediationCreated, RemediationTimeout

#### IT-RO-095-04: Consecutive failure blocking event trail

**BR**: BR-ORCH-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: Consecutive failure blocking produces correct event sequence: RemediationCreated → ConsecutiveFailureBlocked
**Preconditions**: envtest with RO controller, target at consecutive failure threshold (routing returns block)
**Steps**:
1. Seed state: target with N consecutive failures
2. Create RemediationRequest CRD for same target
3. Wait for reconciliation (blocked)
4. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: RemediationCreated, ConsecutiveFailureBlocked

---

## 6. Coverage Targets

| Metric | Target | Actual |
|---|---|---|
| Unit Test Coverage (our events) | 100% (14/14 events) | ⏸️ |
| IT Business Flow Coverage | 4 flows | ⏸️ |
| BR Coverage | 100% | ⏸️ |

---

## 7. Test File Locations

| Test Category | File |
|---|---|
| Unit Tests | `test/unit/remediationorchestrator/controller/reconcile_phases_test.go` (extend) |
| Integration Tests | `test/integration/remediationorchestrator/events_test.go` (new) |
| Infrastructure | `test/unit/remediationorchestrator/controller/test_helpers.go` (modify — FakeRecorder) |

---

## 8. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | | | ⏸️ |
| Reviewer | | | ⏸️ |
| Approver | | | ⏸️ |
