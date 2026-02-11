# Test Plan: AIAnalysis K8s Event Observability

**Service**: AIAnalysis (AA)
**Version**: 1.0
**Created**: 2026-02-05
**Author**: AI Assistant
**Status**: Draft
**Issue**: #72 (our events), #73 (session events, implemented)
**BR**: BR-AA-095, BR-AA-HAPI-064.6 (session events)
**DD**: DD-EVENT-001 v1.1

---

## 1. Scope

### In Scope

- BR-AA-095: K8s Event Observability for AIAnalysis controller
- 6 events implemented by this team
- 3 session events implemented (originally delegated to Issue #64 team, completed by current team)
- Defense-in-depth: UT (FakeRecorder) + IT (corev1.EventList)

### Out of Scope

- Audit event testing (covered by BR-SP-090 pattern)
- Metrics testing (separate maturity requirement)

---

## 2. Event Inventory

| Reason Constant | Type | Priority | Emission Point | Owner | Status |
|---|---|---|---|---|---|
| `AIAnalysisCreated` | Normal | P1 | reconcilePending: Pending → Investigating | This team | Implemented (v1.0) |
| `InvestigationComplete` | Normal | P1 | reconcileInvestigating: Investigating → Analyzing | This team | Planned |
| `AnalysisCompleted` | Normal | P1 | reconcileAnalyzing: Analyzing → Completed | This team | Planned |
| `AnalysisFailed` | Warning | P1 | Any → Failed (terminal) | This team | Planned |
| `ApprovalRequired` | Normal | P2 | AnalyzingHandler: approval needed | This team | Planned |
| `HumanReviewRequired` | Warning | P2 | InvestigatingHandler: needs_human_review=true | This team | Planned |
| `PhaseTransition` | Normal | P3 | All intermediate transitions (shared constant) | This team | Planned |
| `SessionCreated` | Normal | P2 | Session submitted to HAPI | This team | Implemented |
| `SessionLost` | Warning | P2 | Session 404 on poll | This team | Implemented |
| `SessionRegenerationExceeded` | Warning | P2 | Max regenerations exceeded | This team | Implemented |

---

## 3. BR Coverage Matrix

| BR ID | Description | Test Type | Test ID | Status |
|---|---|---|---|---|
| BR-AA-095 | AIAnalysisCreated event (existing) | Unit | UT-AA-095-01 | ⏸️ Pending |
| BR-AA-095 | InvestigationComplete event | Unit | UT-AA-095-02 | ⏸️ Pending |
| BR-AA-095 | AnalysisCompleted event | Unit | UT-AA-095-03 | ⏸️ Pending |
| BR-AA-095 | AnalysisFailed event (investigation failure) | Unit | UT-AA-095-04 | ⏸️ Pending |
| BR-AA-095 | AnalysisFailed event (analyzing failure) | Unit | UT-AA-095-05 | ⏸️ Pending |
| BR-AA-095 | ApprovalRequired event | Unit | UT-AA-095-06 | ⏸️ Pending |
| BR-AA-095 | HumanReviewRequired event | Unit | UT-AA-095-07 | ⏸️ Pending |
| BR-AA-095 | PhaseTransition event (intermediate) | Unit | UT-AA-095-08 | ⏸️ Pending |
| BR-AA-095 | Happy path event trail | Integration | IT-AA-095-01 | ⏸️ Pending |
| BR-AA-095 | Investigation failure event trail | Integration | IT-AA-095-02 | ⏸️ Pending |
| BR-AA-095 | Human review event trail | Integration | IT-AA-095-03 | ⏸️ Pending |

---

## 4. Test Cases

### Unit Tests (FakeRecorder)

**File**: `test/unit/aianalysis/controller_test.go` (extend existing -- FakeRecorder already wired)

#### UT-AA-095-01: AIAnalysisCreated event on Pending → Investigating

**BR**: BR-AA-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that reconciling a Pending AIAnalysis emits AIAnalysisCreated event
**Preconditions**: AIAnalysis in Pending phase, FakeRecorder injected
**Steps**:
1. Create AIAnalysis with Phase=Pending
2. Call reconcilePending
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "AIAnalysisCreated" + "processing started"

#### UT-AA-095-02: InvestigationComplete event on Investigating → Analyzing

**BR**: BR-AA-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that successful investigation emits InvestigationComplete event
**Preconditions**: AIAnalysis in Investigating phase, mock HAPI returns success
**Steps**:
1. Create AIAnalysis with Phase=Investigating
2. Mock InvestigatingHandler to transition to Analyzing
3. Call reconcileInvestigating
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "InvestigationComplete"

#### UT-AA-095-03: AnalysisCompleted event on Analyzing → Completed

**BR**: BR-AA-095
**Type**: Unit
**Category**: Happy Path
**Description**: Verify that successful analysis emits AnalysisCompleted event
**Preconditions**: AIAnalysis in Analyzing phase, mock AnalyzingHandler returns Completed
**Steps**:
1. Create AIAnalysis with Phase=Analyzing
2. Mock AnalyzingHandler to transition to Completed
3. Call reconcileAnalyzing
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "AnalysisCompleted"

#### UT-AA-095-04: AnalysisFailed event on investigation failure

**BR**: BR-AA-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that investigation failure emits AnalysisFailed Warning event
**Preconditions**: AIAnalysis in Investigating phase, mock HAPI returns permanent error
**Steps**:
1. Create AIAnalysis with Phase=Investigating
2. Mock InvestigatingHandler to transition to Failed
3. Call reconcileInvestigating
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "AnalysisFailed"

#### UT-AA-095-05: AnalysisFailed event on analyzing failure

**BR**: BR-AA-095
**Type**: Unit
**Category**: Error Handling
**Description**: Verify that analyzing failure emits AnalysisFailed Warning event
**Preconditions**: AIAnalysis in Analyzing phase, mock AnalyzingHandler returns Failed
**Steps**:
1. Create AIAnalysis with Phase=Analyzing
2. Mock AnalyzingHandler to transition to Failed
3. Call reconcileAnalyzing
4. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "AnalysisFailed"

#### UT-AA-095-06: ApprovalRequired event

**BR**: BR-AA-095
**Type**: Unit
**Category**: Decision Point
**Description**: Verify that approval-required decision emits ApprovalRequired event
**Preconditions**: AIAnalysis in Analyzing phase, AnalyzingHandler determines approval needed
**Steps**:
1. Create AIAnalysis with low-confidence HAPI response
2. Call reconcileAnalyzing
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "ApprovalRequired"

#### UT-AA-095-07: HumanReviewRequired event

**BR**: BR-AA-095
**Type**: Unit
**Category**: Decision Point
**Description**: Verify that human review flag emits HumanReviewRequired Warning event
**Preconditions**: AIAnalysis in Investigating phase, HAPI response has needs_human_review=true
**Steps**:
1. Create AIAnalysis with mock HAPI returning needs_human_review=true
2. Call reconcileInvestigating
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Warning" + "HumanReviewRequired"

#### UT-AA-095-08: PhaseTransition event (intermediate)

**BR**: BR-AA-095
**Type**: Unit
**Category**: Observability
**Description**: Verify that intermediate phase changes emit PhaseTransition event with descriptive message
**Preconditions**: AIAnalysis transitioning between non-terminal phases
**Steps**:
1. Create AIAnalysis in Pending phase
2. Trigger transition to Investigating
3. Read from recorder.Events channel
**Expected Result**: Event string contains "Normal" + "PhaseTransition" + "Pending" + "Investigating"

### Integration Tests (corev1.EventList)

**File**: `test/integration/aianalysis/events_test.go` (new)

#### IT-AA-095-01: Happy path event trail

**BR**: BR-AA-095
**Type**: Integration
**Category**: Happy Path (Business Flow)
**Description**: Complete AA lifecycle produces correct event sequence
**Preconditions**: envtest with AA controller, mock HAPI
**Steps**:
1. Create AIAnalysis CRD
2. Wait for Phase=Completed
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain (in order): AIAnalysisCreated, InvestigationComplete, AnalysisCompleted

#### IT-AA-095-02: Investigation failure event trail

**BR**: BR-AA-095
**Type**: Integration
**Category**: Error Handling (Business Flow)
**Description**: Failed investigation produces correct event sequence
**Preconditions**: envtest with AA controller, mock HAPI returning permanent error
**Steps**:
1. Create AIAnalysis CRD
2. Wait for Phase=Failed
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: AIAnalysisCreated, AnalysisFailed

#### IT-AA-095-03: Human review event trail

**BR**: BR-AA-095
**Type**: Integration
**Category**: Decision Point (Business Flow)
**Description**: Human review flag produces correct event sequence
**Preconditions**: envtest with AA controller, mock HAPI returning needs_human_review=true
**Steps**:
1. Create AIAnalysis CRD
2. Wait for reconciliation
3. List corev1.Events by involvedObject.name
**Expected Result**: Events contain: AIAnalysisCreated, HumanReviewRequired

---

## 5. Session Event Tests (Implemented)

Originally delegated to the Issue #64 team, these tests were implemented by the current team. They map to BR-AA-HAPI-064.6 and the existing test plan at `docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md`.

| Test ID | Event | Description | Status |
|---|---|---|---|
| UT-AA-064-001 | SessionCreated | Session submitted to HAPI (K8s event assertion added) | Implemented |
| UT-AA-064-008 | SessionLost | Session 404 triggers regeneration (K8s event assertion added) | Implemented |
| UT-AA-064-010 | SessionRegenerationExceeded | Max regenerations exceeded (K8s event assertion added) | Implemented |
| IT-AA-064-01 | Session lifecycle trail | SessionCreated → SessionLost → SessionRegenerationExceeded → AnalysisFailed | Covered by IT-AA-095-02 |

**File**: `test/unit/aianalysis/investigating_handler_session_test.go` (FakeRecorder wired via `WithRecorder`)

---

## 6. Coverage Targets

| Metric | Target | Actual |
|---|---|---|
| Unit Test Coverage (our events) | 100% (8/8 events) | ⏸️ |
| IT Business Flow Coverage | 3 flows | ⏸️ |
| BR Coverage | 100% | ⏸️ |

---

## 7. Test File Locations

| Test Category | File |
|---|---|
| Unit Tests | `test/unit/aianalysis/controller_test.go` (extend) |
| Integration Tests | `test/integration/aianalysis/events_test.go` (new) |

---

## 8. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | | | ⏸️ |
| Reviewer | | | ⏸️ |
| Approver | | | ⏸️ |
