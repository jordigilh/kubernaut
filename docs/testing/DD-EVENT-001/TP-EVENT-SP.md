# Test Plan: SignalProcessing K8s Event Observability

**Service**: SignalProcessing (SP)
**Version**: 1.0
**Created**: 2026-02-11
**Author**: AI Assistant
**Status**: Draft
**Issue**: #75
**BR**: BR-SP-095
**DD**: DD-EVENT-001 v1.1

---

## 1. Scope

### In Scope

- BR-SP-095: K8s Event Observability for SignalProcessing controller
- 6 events total: 1 existing (v1.0) + 5 new (v1.1)
- Defense-in-depth: UT (FakeRecorder) + IT (corev1.EventList / business flow trails)

### Out of Scope

- Audit event testing (covered by BR-SP-090 pattern)
- Metrics testing (separate maturity requirement)

---

## 2. Test Infrastructure

### FakeRecorder Wiring Required

**Status**: SP does NOT currently have FakeRecorder in unit tests.

The SignalProcessing controller test infrastructure must be updated to inject `record.FakeRecorder` into the reconciler (or equivalent test harness) before event assertion tests can be implemented. Reference implementations exist in:

- `test/unit/aianalysis/controller_test.go` (FakeRecorder already wired)
- `test/unit/workflowexecution/controller_test.go` (FakeRecorder already wired)

**Prerequisite**: Wire FakeRecorder into SP unit test setup before implementing UT-SP-095-* scenarios.

---

## 3. Event Inventory

| Reason Constant | Type | Priority | Emission Point | Status |
|---|---|---|---|---|
| `PolicyEvaluationFailed` | Warning | P2 | reconcileClassifying: Rego severity mapping failed | Implemented v1.0 |
| `SignalProcessed` | Normal | P1 | reconcileCategorizing: Categorizing → Completed (terminal success) | Planned v1.1 |
| `SignalEnriched` | Normal | P2 | reconcileEnriching: K8s context enrichment completed | Planned v1.1 |
| `EnrichmentDegraded` | Warning | P4 | reconcileEnriching: K8s enrichment returned degraded/partial results | Planned v1.1 |
| `PhaseTransition` | Normal | P3 | Pending → Enriching | Planned v1.1 |
| `PhaseTransition` | Normal | P3 | Enriching → Classifying | Planned v1.1 |
| `PhaseTransition` | Normal | P3 | Classifying → Categorizing | Planned v1.1 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Test Type | Test ID (DD-TEST-006) | Status |
|---|---|---|---|---|
| BR-SP-095 | SignalProcessed on Categorizing → Completed | Unit | UT-SP-095-01 | ⏸️ Pending |
| BR-SP-095 | SignalEnriched on enrichment completion | Unit | UT-SP-095-02 | ⏸️ Pending |
| BR-SP-095 | EnrichmentDegraded on degraded K8s enrichment | Unit | UT-SP-095-03 | ⏸️ Pending |
| BR-SP-095 | PhaseTransition on Pending → Enriching | Unit | UT-SP-095-04 | ⏸️ Pending |
| BR-SP-095 | PhaseTransition on Enriching → Classifying | Unit | UT-SP-095-05 | ⏸️ Pending |
| BR-SP-095 | PhaseTransition on Classifying → Categorizing | Unit | UT-SP-095-06 | ⏸️ Pending |
| BR-SP-095 | PolicyEvaluationFailed (existing, verify assertion) | Unit | UT-SP-095-07 | ⏸️ Pending |
| BR-SP-095 | Happy path event trail | Integration | IT-SP-095-01 | ⏸️ Pending |
| BR-SP-095 | Rego policy failure event trail | Integration | IT-SP-095-02 | ⏸️ Pending |
| BR-SP-095 | Degraded enrichment event trail | Integration | IT-SP-095-03 | ⏸️ Pending |

---

## 5. Test Cases

### Unit Tests (FakeRecorder)

**File**: `test/unit/signalprocessing/reconciler/phase_transitions_test.go` (extend) or new file `test/unit/signalprocessing/events_test.go`

**Note**: FakeRecorder must be wired into SP test infrastructure before these tests can run. See Section 2.

#### UT-SP-095-01: SignalProcessed on Categorizing → Completed

**BR**: BR-SP-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that successful categorizing transition emits SignalProcessed event
**Preconditions**: SignalProcessing in Categorizing phase, reconciliation transitions to Completed
**Steps**:
1. Create SignalProcessing with Phase=Categorizing
2. Mock CategorizingHandler to transition to Completed
3. Call reconcileCategorizing
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "SignalProcessed" + "Categorizing" + "Completed"

#### UT-SP-095-02: SignalEnriched on enrichment completion

**BR**: BR-SP-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that K8s context enrichment completion emits SignalEnriched event
**Preconditions**: SignalProcessing in Enriching phase, enricher returns success
**Steps**:
1. Create SignalProcessing with Phase=Enriching
2. Mock Enricher to return successful enrichment
3. Call reconcileEnriching
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "SignalEnriched"

#### UT-SP-095-03: EnrichmentDegraded on degraded K8s enrichment

**BR**: BR-SP-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that degraded/partial enrichment emits EnrichmentDegraded Warning event
**Preconditions**: SignalProcessing in Enriching phase, enricher returns degraded/partial results
**Steps**:
1. Create SignalProcessing with Phase=Enriching
2. Mock Enricher to return degraded or partial results
3. Call reconcileEnriching
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "EnrichmentDegraded"

#### UT-SP-095-04: PhaseTransition on Pending → Enriching

**BR**: BR-SP-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that Pending → Enriching transition emits PhaseTransition event
**Preconditions**: SignalProcessing transitioning from Pending to Enriching
**Steps**:
1. Create SignalProcessing in Pending phase
2. Trigger transition to Enriching
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Pending" + "Enriching"

#### UT-SP-095-05: PhaseTransition on Enriching → Classifying

**BR**: BR-SP-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that Enriching → Classifying transition emits PhaseTransition event
**Preconditions**: SignalProcessing transitioning from Enriching to Classifying
**Steps**:
1. Create SignalProcessing in Enriching phase
2. Trigger transition to Classifying
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Enriching" + "Classifying"

#### UT-SP-095-06: PhaseTransition on Classifying → Categorizing

**BR**: BR-SP-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that Classifying → Categorizing transition emits PhaseTransition event
**Preconditions**: SignalProcessing transitioning from Classifying to Categorizing
**Steps**:
1. Create SignalProcessing in Classifying phase
2. Trigger transition to Categorizing
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Classifying" + "Categorizing"

#### UT-SP-095-07: PolicyEvaluationFailed (existing, verify assertion exists)

**BR**: BR-SP-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that Rego severity mapping failure emits PolicyEvaluationFailed Warning event (assertion must exist or be added)
**Preconditions**: SignalProcessing in Classifying phase, Rego policy evaluation fails (severity mapping)
**Steps**:
1. Create SignalProcessing with Phase=Classifying
2. Mock Rego engine to return severity mapping failure
3. Call reconcileClassifying
4. Read from recorder.Events channel (or verify existing assertion in reconciler tests)
**Expected Result**: Event string contains "Warning" + "PolicyEvaluationFailed" + "Rego severity mapping failed"

### Integration Tests (corev1.EventList)

**File**: `test/integration/signalprocessing/events_test.go` (new)

#### IT-SP-095-01: Happy path event trail

**BR**: BR-SP-095
**Type**: Integration
**Category**: Happy Path (Business Flow)
**Description**: Complete SP lifecycle produces correct event sequence
**Preconditions**: envtest with SP controller, all phases complete successfully
**Steps**:
1. Create SignalProcessing CRD
2. Wait for Phase=Completed
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain (in order): PhaseTransition (Pending→Enriching), SignalEnriched, PhaseTransition (Enriching→Classifying), PhaseTransition (Classifying→Categorizing), SignalProcessed

#### IT-SP-095-02: Rego policy failure trail

**BR**: BR-SP-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: Rego policy failure during classification produces correct event sequence
**Preconditions**: envtest with SP controller, Rego policy/severity mapping configured to fail
**Steps**:
1. Create SignalProcessing CRD
2. Configure Rego policy to fail (e.g., invalid severity mapping)
3. Wait for reconciliation (Classifying phase)
4. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: PhaseTransition, PolicyEvaluationFailed

#### IT-SP-095-03: Degraded enrichment trail

**BR**: BR-SP-095
**Type**: Integration
**Category**: Degraded Path (Business Flow)
**Description**: Degraded/partial K8s enrichment continues to completion with correct event sequence
**Preconditions**: envtest with SP controller, enricher returns degraded/partial results
**Steps**:
1. Create SignalProcessing CRD
2. Arrange enricher to return degraded or partial enrichment
3. Wait for Phase=Completed (processing continues with partial data)
4. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: PhaseTransition, EnrichmentDegraded, SignalProcessed

---

## 6. Coverage Targets

| Metric | Target | Actual |
|---|---|---|
| Unit Test Coverage (all 7 scenarios) | 100% | ⏸️ |
| IT Business Flow Coverage | 3 flows | ⏸️ |
| BR Coverage | 100% | ⏸️ |

---

## 7. Test File Locations

| Test Category | File |
|---|---|
| Unit Tests | `test/unit/signalprocessing/reconciler/phase_transitions_test.go` (extend) or `test/unit/signalprocessing/events_test.go` (new) |
| Integration Tests | `test/integration/signalprocessing/events_test.go` (new) |

---

## 8. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | | | ⏸️ |
| Reviewer | | | ⏸️ |
| Approver | | | ⏸️ |
