# Test Plan: Notification K8s Event Observability

**Service**: Notification (NT)
**Version**: 1.0
**Created**: 2026-02-11
**Author**: AI Assistant
**Status**: Draft
**Issue**: #76
**BR**: BR-NT-095
**DD**: DD-EVENT-001 v1.1

---

## 1. Scope

### In Scope

- BR-NT-095: K8s Event Observability for Notification controller
- 8 events total: 2 existing (v1.0) + 6 new (v1.1)
- Defense-in-depth: UT (FakeRecorder) + IT (corev1.EventList / business flow trails)
- Notification phase lifecycle: "" → Pending → Sending → {Sent | PartiallySent | Failed | Retrying → ...}

### Out of Scope

- Audit event testing (covered by BR-SP-090 pattern)
- Metrics testing (separate maturity requirement)

---

## 2. Test Infrastructure

### FakeRecorder Wiring Required

**Status**: NT does NOT currently have FakeRecorder in unit tests.

The Notification controller test infrastructure must be updated to inject `record.FakeRecorder` into the reconciler (or equivalent test harness) before event assertion tests can be implemented. Reference implementations exist in:

- `test/unit/aianalysis/controller_test.go` (FakeRecorder already wired)
- `test/unit/workflowexecution/controller_test.go` (FakeRecorder already wired)

**Prerequisite**: Wire FakeRecorder into NT unit test setup before implementing UT-NT-095-* scenarios.

---

## 3. Event Inventory

| Reason Constant | Type | Priority | Emission Point | Status |
|---|---|---|---|---|
| `ReconcileStarted` | Normal | P3 | Reconcile entry: reconciliation begins | Implemented v1.0 |
| `PhaseTransition` | Normal | P3 | handlePendingToSendingTransition: Pending → Sending | Implemented v1.0 |
| `NotificationSent` | Normal | P1 | transitionToSent: all channels delivered successfully | Planned v1.1 |
| `NotificationFailed` | Warning | P1 | transitionToFailed: permanent delivery failure | Planned v1.1 |
| `NotificationPartiallySent` | Normal | P1 | transitionToPartiallySent: some channels ok, others failed | Planned v1.1 |
| `CircuitBreakerOpen` | Warning | P2 | deliverToSlack: Slack circuit breaker tripped | Planned v1.1 |
| `NotificationRetrying` | Normal | P3 | transitionToRetrying: retrying after transient failure | Planned v1.1 |
| `PhaseTransition` | Normal | P3 | Additional intermediate transitions (shared constant, message content differs) | Planned v1.1 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Test Type | Test ID (DD-TEST-006) | Status |
|---|---|---|---|---|
| BR-NT-095 | NotificationSent on all channels succeed | Unit | UT-NT-095-01 | ⏸️ Pending |
| BR-NT-095 | NotificationFailed on permanent delivery failure | Unit | UT-NT-095-02 | ⏸️ Pending |
| BR-NT-095 | NotificationPartiallySent on partial success | Unit | UT-NT-095-03 | ⏸️ Pending |
| BR-NT-095 | CircuitBreakerOpen when Slack circuit breaker trips | Unit | UT-NT-095-04 | ⏸️ Pending |
| BR-NT-095 | NotificationRetrying on transient failure with retries remaining | Unit | UT-NT-095-05 | ⏸️ Pending |
| BR-NT-095 | PhaseTransition on Sending → Retrying (message content) | Unit | UT-NT-095-06 | ⏸️ Pending |
| BR-NT-095 | ReconcileStarted (verify existing, add assertion) | Unit | UT-NT-095-07 | ⏸️ Pending |
| BR-NT-095 | PhaseTransition on Pending → Sending (verify existing, add assertion) | Unit | UT-NT-095-08 | ⏸️ Pending |
| BR-NT-095 | All channels succeed: event trail | Integration | IT-NT-095-01 | ⏸️ Pending |
| BR-NT-095 | Partial success + retry exhaustion: event trail | Integration | IT-NT-095-02 | ⏸️ Pending |
| BR-NT-095 | All channels fail permanently: event trail | Integration | IT-NT-095-03 | ⏸️ Pending |

---

## 5. Test Cases

### Unit Tests (FakeRecorder)

**File**: `test/unit/notification/events_test.go` (new, needs FakeRecorder wiring)

**Note**: FakeRecorder must be wired into NT test infrastructure before these tests can run. See Section 2.

#### UT-NT-095-01: NotificationSent on all channels succeed

**BR**: BR-NT-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that successful delivery to all channels emits NotificationSent event
**Preconditions**: NotificationRequest in Sending phase, all channels deliver successfully
**Steps**:
1. Create NotificationRequest with Phase=Sending, spec.channels configured
2. Mock DeliveryOrchestrator to return success for all channels
3. Call reconcile (or targeted reconcile path) to trigger transitionToSent
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "NotificationSent" + "Successfully delivered"

#### UT-NT-095-02: NotificationFailed on permanent delivery failure

**BR**: BR-NT-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that permanent delivery failure (all retries exhausted) emits NotificationFailed Warning event
**Preconditions**: NotificationRequest in Sending phase, all channels return permanent errors or retries exhausted
**Steps**:
1. Create NotificationRequest with Phase=Sending
2. Mock DeliveryOrchestrator to return permanent failures or exhaust retries
3. Call reconcile to trigger transitionToFailed with permanent=true
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "NotificationFailed"

#### UT-NT-095-03: NotificationPartiallySent on partial success

**BR**: BR-NT-095
**Type**: Unit
**Category**: Partial Success
**Description**: Verify that partial delivery success (some channels ok, others failed after retry exhaustion) emits NotificationPartiallySent event
**Preconditions**: NotificationRequest in Sending phase, some channels succeed, others permanently fail
**Steps**:
1. Create NotificationRequest with Phase=Sending, multiple channels
2. Mock DeliveryOrchestrator: some succeed, others exhaust retries
3. Call reconcile to trigger transitionToPartiallySent
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "NotificationPartiallySent" + "Delivered to" + "others failed"

#### UT-NT-095-04: CircuitBreakerOpen when Slack circuit breaker trips

**BR**: BR-NT-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that Slack circuit breaker trip emits CircuitBreakerOpen Warning event
**Preconditions**: NotificationRequest with Slack channel, circuit breaker is open
**Steps**:
1. Create NotificationRequest with Slack channel
2. Mock CircuitBreaker to report open state (isSlackCircuitBreakerOpen returns true)
3. Call reconcile path that invokes Slack delivery
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "CircuitBreakerOpen"

#### UT-NT-095-05: NotificationRetrying on transient failure with retries remaining

**BR**: BR-NT-095
**Type**: Unit
**Category**: Retry Path
**Description**: Verify that transient failure with retries remaining emits NotificationRetrying event
**Preconditions**: NotificationRequest in Sending phase, some channels succeed, others fail with retries remaining
**Steps**:
1. Create NotificationRequest with Phase=Sending, multiple channels
2. Mock DeliveryOrchestrator: some succeed, some fail (retries remaining)
3. Call reconcile to trigger transitionToRetrying
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "NotificationRetrying" + "retrying"

#### UT-NT-095-06: PhaseTransition on Sending → Retrying (message content)

**BR**: BR-NT-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that Sending → Retrying transition emits PhaseTransition event with descriptive message
**Preconditions**: Notification transitioning from Sending to Retrying (partial failure, retries remain)
**Steps**:
1. Create NotificationRequest in Sending phase
2. Trigger transition to Retrying phase
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Sending" + "Retrying"

#### UT-NT-095-07: ReconcileStarted (verify existing, add assertion)

**BR**: BR-NT-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that reconciliation entry emits ReconcileStarted event (assertion must exist or be added)
**Preconditions**: Any NotificationRequest ready for reconcile
**Steps**:
1. Create NotificationRequest in any phase
2. Call Reconcile
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "ReconcileStarted" + "Started reconciling"

#### UT-NT-095-08: PhaseTransition on Pending → Sending (verify existing, add assertion)

**BR**: BR-NT-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that Pending → Sending transition emits PhaseTransition event (assertion must exist or be added)
**Preconditions**: NotificationRequest in Pending phase with channels to deliver
**Steps**:
1. Create NotificationRequest with Phase=Pending
2. Call reconcile to trigger handlePendingToSendingTransition
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Sending" + "Transitioned"

### Integration Tests (corev1.EventList)

**File**: `test/integration/notification/events_test.go` (new)

#### IT-NT-095-01: All channels succeed: ReconcileStarted → PhaseTransition → NotificationSent

**BR**: BR-NT-095
**Type**: Integration
**Category**: Happy Path (Business Flow)
**Description**: Complete NT happy path produces correct event sequence
**Preconditions**: envtest with NT controller, mock delivery services return success for all channels
**Steps**:
1. Create NotificationRequest CRD
2. Wait for Phase=Sent
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain (in order): ReconcileStarted, PhaseTransition (Pending→Sending), NotificationSent

#### IT-NT-095-02: Partial success + retry exhaustion: ReconcileStarted → PhaseTransition → NotificationRetrying → NotificationPartiallySent

**BR**: BR-NT-095
**Type**: Integration
**Category**: Retry Path (Business Flow)
**Description**: Partial success followed by retry exhaustion produces correct event sequence
**Preconditions**: envtest with NT controller, mock returns partial success then exhausts retries on failed channels
**Steps**:
1. Create NotificationRequest CRD with multiple channels
2. Configure mocks: first delivery partial success (some fail), retries exhaust on next attempts
3. Wait for Phase=PartiallySent
4. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: ReconcileStarted, PhaseTransition, NotificationRetrying (at least once), NotificationPartiallySent

#### IT-NT-095-03: All channels fail permanently: ReconcileStarted → PhaseTransition → NotificationFailed

**BR**: BR-NT-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: All channels fail permanently produces correct event sequence
**Preconditions**: envtest with NT controller, mock delivery returns permanent errors for all channels
**Steps**:
1. Create NotificationRequest CRD
2. Configure mocks to return permanent delivery failures
3. Wait for Phase=Failed
4. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: ReconcileStarted, PhaseTransition, NotificationFailed

---

## 6. Coverage Targets

| Metric | Target | Actual |
|---|---|---|
| Unit Test Coverage (all 8 scenarios) | 100% | ⏸️ |
| IT Business Flow Coverage | 3 flows | ⏸️ |
| BR Coverage | 100% | ⏸️ |

---

## 7. Test File Locations

| Test Category | File |
|---|---|
| Unit Tests | `test/unit/notification/events_test.go` (new, needs FakeRecorder wiring) |
| Integration Tests | `test/integration/notification/events_test.go` (new) |

---

## 8. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | | | ⏸️ |
| Reviewer | | | ⏸️ |
| Approver | | | ⏸️ |
