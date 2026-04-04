# Test Plan: ResponseProcessor Terminal Handler Status Completeness

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-610-v1.0
**Feature**: Add missing TotalAnalysisTime, status conditions, and approval metadata to all 5 ResponseProcessor terminal handlers
**Version**: 1.0
**Created**: 2026-04-03
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that all 5 terminal handlers in `response_processor.go` set the same status fields and conditions that the main `analyzing.go` success path sets. The gap was discovered when E2E-AA-607-001 asserted `TotalAnalysisTime > 0` on the `handleNotActionableFromIncident` path — a path that never calculated it.

Due diligence revealed the gap extends to 5 fields across all 5 handlers, affecting CRD completeness, observability, and operator tooling.

### 1.2 Objectives

1. **TotalAnalysisTime**: All 5 handlers calculate `TotalAnalysisTime` from `StartedAt`/`CompletedAt` when `StartedAt` is non-nil
2. **SetInvestigationComplete**: Both Completed handlers (`ProblemResolved`, `NotActionable`) call `SetInvestigationComplete(true, ...)` — the 3 Failed handlers already call it with `false`
3. **SetAnalysisComplete**: All 5 handlers call `SetAnalysisComplete` (true for Completed, false for Failed)
4. **SetWorkflowResolved**: All 5 handlers call `SetWorkflowResolved(false, ...)` with the appropriate reason
5. **SetApprovalRequired**: All 5 handlers call `SetApprovalRequired(false, "NotApplicable", ...)` — none go through Rego evaluation

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/aianalysis/... -ginkgo.focus="610"` |
| Unit-testable code coverage | >=80% | Coverage of the 5 handler functions in response_processor.go |
| Backward compatibility | 0 regressions | All existing AA unit, integration, and E2E tests pass |
| E2E-AA-607-001 | PASS | `TotalAnalysisTime > 0` assertion passes without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #610: ResponseProcessor terminal handlers missing TotalAnalysisTime, status conditions
- Issue #607: Not-actionable confidence gate (discovered the gap via E2E-AA-607-001)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- `pkg/aianalysis/handlers/analyzing.go` (reference implementation for correct status completeness)
- `pkg/aianalysis/conditions.go` (condition setter API)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Existing tests assert specific condition counts or absence of conditions | Test regression | Low | All existing AA tests | Due diligence confirmed no existing tests assert conditions for ResponseProcessor paths |
| R2 | TotalAnalysisTime=0 for sub-second unit test execution | False negative in unit tests | Medium | UT-AA-610-001 through 005 | Set `StartedAt` to 5 seconds in the past in all test fixtures |
| R3 | Condition ordering causes reconciliation side effects | Unexpected controller behavior | Low | E2E tests | Due diligence confirmed no downstream code reads AA conditions; follow analyzing.go ordering (conditions before phase) |
| R4 | SetApprovalRequired breaks assumptions in existing tests | Test regression | Very Low | Existing AA tests | No existing test asserts ApprovalRequired condition for ResponseProcessor paths |
| R5 | handleLowConfidenceFailure has a selected workflow — SetWorkflowResolved(false) may confuse operators | Observability confusion | Low | UT-AA-610-005 | Use distinct message: "Workflow confidence below threshold, resolution failed" |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by running full existing test suite after implementation
- **R2**: Mitigated by UT-AA-610-001 through 005 using `StartedAt` 5 seconds in the past
- **R3**: Mitigated by following exact ordering from `analyzing.go`
- **R5**: Mitigated by UT-AA-610-005 asserting the condition message

---

## 4. Scope

### 4.1 Features to be Tested

- **`handleWorkflowResolutionFailureFromIncident`** (`pkg/aianalysis/handlers/response_processor.go:270`): TotalAnalysisTime, SetAnalysisComplete(false), SetWorkflowResolved(false), SetApprovalRequired(false)
- **`handleProblemResolvedFromIncident`** (`response_processor.go:380`): TotalAnalysisTime, SetInvestigationComplete(true), SetAnalysisComplete(true), SetWorkflowResolved(false), SetApprovalRequired(false)
- **`handleNotActionableFromIncident`** (`response_processor.go:423`): TotalAnalysisTime, SetInvestigationComplete(true), SetAnalysisComplete(true), SetWorkflowResolved(false), SetApprovalRequired(false)
- **`handleNoWorkflowTerminalFailure`** (`response_processor.go:467`): TotalAnalysisTime, SetAnalysisComplete(false), SetWorkflowResolved(false), SetApprovalRequired(false)
- **`handleLowConfidenceFailure`** (`response_processor.go:516`): TotalAnalysisTime, SetAnalysisComplete(false), SetWorkflowResolved(false), SetApprovalRequired(false)

### 4.2 Features Not to be Tested

- **`analyzing.go` main success path**: Already has all fields set and tested by UT-AA-TAT-001 and E2E 03_full_flow_test.go
- **`investigating.go` error paths**: These set `CompletedAt` and `SetInvestigationComplete(false)` but are failure-before-HAPI paths — separate concern
- **Condition setter logic** (`conditions.go`): Already tested in `conditions_test.go`

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test via `ProcessIncidentResponse` public API, not private handlers | Validates the full dispatch chain; mirrors real controller behavior |
| Set `StartedAt` to 5 seconds in the past | Avoids sub-second truncation to 0 for `TotalAnalysisTime` (R2) |
| Assert all 5 conditions in each test (not just the new ones) | Proves completeness at the handler level; catches regressions if condition logic changes |
| Include `SetApprovalRequired(false)` despite not being in original issue | Due diligence Finding 1: analyzing.go sets it on all paths; terminal handlers should too |
| Completed handlers call `SetInvestigationComplete(true)` | Investigation succeeded — it determined no workflow is needed. This is success, not failure. |
| All handlers call `SetWorkflowResolved(false, ...)` | No handler in ResponseProcessor resolves a workflow — that only happens in analyzing.go after Rego pass |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of the 5 handler functions in `response_processor.go` (lines ~270-607)
- **Integration**: Tier skip — no I/O or wiring changes; all changes are pure status field assignments
- **E2E**: Existing E2E-AA-607-001 validates the `handleNotActionableFromIncident` path end-to-end

### 5.2 Two-Tier Minimum

- **Unit tests**: 6 new tests covering all 5 handlers plus a StartedAt=nil edge case
- **E2E tests**: E2E-AA-607-001 (existing, currently failing) serves as the second tier for the NotActionable path

### 5.3 Business Outcome Quality Bar

Each test validates: "When the AA controller completes or fails an analysis via a ResponseProcessor terminal handler, the operator/dashboard/audit system sees complete, consistent status metadata identical in structure to the main success path."

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All 6 P0 unit tests pass
2. E2E-AA-607-001 passes (TotalAnalysisTime > 0)
3. All existing AA unit, integration, and E2E tests pass (0 regressions)
4. >=80% unit-testable code coverage of the 5 handler functions

**FAIL** — any of the following:

1. Any P0 test fails
2. E2E-AA-607-001 still fails
3. Any existing test regresses

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken or existing test regression discovered during implementation.
**Resume**: Root cause identified and resolved.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | `handleWorkflowResolutionFailureFromIncident` | ~270-377 |
| `pkg/aianalysis/handlers/response_processor.go` | `handleProblemResolvedFromIncident` | ~380-421 |
| `pkg/aianalysis/handlers/response_processor.go` | `handleNotActionableFromIncident` | ~423-465 |
| `pkg/aianalysis/handlers/response_processor.go` | `handleNoWorkflowTerminalFailure` | ~467-513 |
| `pkg/aianalysis/handlers/response_processor.go` | `handleLowConfidenceFailure` | ~516-607 |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc3` HEAD | Branch for RC 1.2.0-rc3 |
| Dependency: #607 fix | Merged in branch | `handleNotActionableFromIncident` already exists |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #610-AC1 | All 5 handlers set TotalAnalysisTime when StartedAt is non-nil | P0 | Unit | UT-AA-610-001 through 005 | Pending |
| #610-AC2 | ProblemResolved and NotActionable call SetInvestigationComplete(true) | P0 | Unit | UT-AA-610-002, UT-AA-610-003 | Pending |
| #610-AC3 | All 5 handlers call SetAnalysisComplete (true for Completed, false for Failed) | P0 | Unit | UT-AA-610-001 through 005 | Pending |
| #610-AC4 | All 5 handlers call SetWorkflowResolved(false, ...) | P0 | Unit | UT-AA-610-001 through 005 | Pending |
| #610-AC5 | All 5 handlers call SetApprovalRequired(false, "NotApplicable", ...) | P0 | Unit | UT-AA-610-001 through 005 | Pending |
| #610-AC6 | TotalAnalysisTime remains 0 when StartedAt is nil | P0 | Unit | UT-AA-610-006 | Pending |
| #610-E2E | E2E-AA-607-001 TotalAnalysisTime > 0 passes | P0 | E2E | E2E-AA-607-001 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-AA-610-{SEQUENCE}` for unit tests.

### Tier 1: Unit Tests

**Testable code scope**: 5 handler functions in `response_processor.go` (lines ~270-607). Target: >=80% coverage of these functions.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AA-610-001` | handleWorkflowResolutionFailureFromIncident sets TotalAnalysisTime, all 4 conditions (InvestigationComplete=false, AnalysisComplete=false, WorkflowResolved=false, ApprovalRequired=false) | Pending |
| `UT-AA-610-002` | handleProblemResolvedFromIncident sets TotalAnalysisTime, all 4 conditions (InvestigationComplete=true, AnalysisComplete=true, WorkflowResolved=false/WorkflowNotNeeded, ApprovalRequired=false) | Pending |
| `UT-AA-610-003` | handleNotActionableFromIncident sets TotalAnalysisTime, all 4 conditions (InvestigationComplete=true, AnalysisComplete=true, WorkflowResolved=false/WorkflowNotNeeded, ApprovalRequired=false) | Pending |
| `UT-AA-610-004` | handleNoWorkflowTerminalFailure sets TotalAnalysisTime, all 4 conditions (InvestigationComplete=false, AnalysisComplete=false, WorkflowResolved=false, ApprovalRequired=false) | Pending |
| `UT-AA-610-005` | handleLowConfidenceFailure sets TotalAnalysisTime, all 4 conditions (InvestigationComplete=false, AnalysisComplete=false, WorkflowResolved=false, ApprovalRequired=false) | Pending |
| `UT-AA-610-006` | TotalAnalysisTime remains 0 when StartedAt is nil (defensive edge case) | Pending |

### Tier 2: Integration Tests

**Tier skip rationale**: The fix adds pure status field assignments with no I/O, wiring, or cross-component interaction. Unit tests via `ProcessIncidentResponse` provide full behavioral coverage. E2E-AA-607-001 provides the second tier.

### Tier 3: E2E Tests

**Testable code scope**: Existing E2E-AA-607-001 validates the NotActionable path end-to-end.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-AA-607-001` | AIAnalysis completing via NotActionable path has TotalAnalysisTime > 0 | Pending (blocked by #610 fix) |

---

## 9. Test Cases

### UT-AA-610-001: handleWorkflowResolutionFailureFromIncident — complete status metadata

**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_status_test.go`

**Preconditions**:
- AIAnalysis with `StartedAt` set to 5 seconds ago
- IncidentResponse with `NeedsHumanReview=true` (triggers this handler)

**Test Steps**:
1. **Given**: An AIAnalysis in Analyzing phase with `StartedAt` set 5 seconds ago
2. **When**: `ProcessIncidentResponse` is called with a response where `NeedsHumanReview=true`
3. **Then**: The handler routes to `handleWorkflowResolutionFailureFromIncident` and:
   - `TotalAnalysisTime > 0`
   - Condition `InvestigationComplete` is `False` with reason `InvestigationFailed`
   - Condition `AnalysisComplete` is `False` with reason `AnalysisFailed`
   - Condition `WorkflowResolved` is `False` with reason `WorkflowResolutionFailed`
   - Condition `ApprovalRequired` is `False` with reason `NotApplicable`

### UT-AA-610-002: handleProblemResolvedFromIncident — complete status metadata

**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_status_test.go`

**Preconditions**:
- AIAnalysis with `StartedAt` set to 5 seconds ago
- IncidentResponse with `NeedsHumanReview=false`, no workflow, `Confidence >= 0.7`, `problem_resolved` warning signal, no `no_workflow` warning

**Test Steps**:
1. **Given**: An AIAnalysis in Analyzing phase with `StartedAt` set 5 seconds ago
2. **When**: `ProcessIncidentResponse` is called with a problem-resolved response
3. **Then**: The handler routes to `handleProblemResolvedFromIncident` and:
   - `TotalAnalysisTime > 0`
   - Condition `InvestigationComplete` is `True` with reason `InvestigationSucceeded`
   - Condition `AnalysisComplete` is `True` with reason `AnalysisSucceeded`
   - Condition `WorkflowResolved` is `False` with reason `WorkflowNotNeeded`
   - Condition `ApprovalRequired` is `False` with reason `NotApplicable`

### UT-AA-610-003: handleNotActionableFromIncident — complete status metadata

**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_status_test.go`

**Preconditions**:
- AIAnalysis with `StartedAt` set to 5 seconds ago
- IncidentResponse with `NeedsHumanReview=false`, no workflow, `alert_not_actionable` warning, `IsActionable=false`

**Test Steps**:
1. **Given**: An AIAnalysis in Analyzing phase with `StartedAt` set 5 seconds ago
2. **When**: `ProcessIncidentResponse` is called with a not-actionable response
3. **Then**: The handler routes to `handleNotActionableFromIncident` and:
   - `TotalAnalysisTime > 0`
   - Condition `InvestigationComplete` is `True` with reason `InvestigationSucceeded`
   - Condition `AnalysisComplete` is `True` with reason `AnalysisSucceeded`
   - Condition `WorkflowResolved` is `False` with reason `WorkflowNotNeeded`
   - Condition `ApprovalRequired` is `False` with reason `NotApplicable`

### UT-AA-610-004: handleNoWorkflowTerminalFailure — complete status metadata

**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_status_test.go`

**Preconditions**:
- AIAnalysis with `StartedAt` set to 5 seconds ago
- IncidentResponse with `NeedsHumanReview=false`, no workflow, `Confidence < 0.7`

**Test Steps**:
1. **Given**: An AIAnalysis in Analyzing phase with `StartedAt` set 5 seconds ago
2. **When**: `ProcessIncidentResponse` is called with a no-workflow response
3. **Then**: The handler routes to `handleNoWorkflowTerminalFailure` and:
   - `TotalAnalysisTime > 0`
   - Condition `InvestigationComplete` is `False` with reason `InvestigationFailed`
   - Condition `AnalysisComplete` is `False` with reason `AnalysisFailed`
   - Condition `WorkflowResolved` is `False` with reason `WorkflowResolutionFailed`
   - Condition `ApprovalRequired` is `False` with reason `NotApplicable`

### UT-AA-610-005: handleLowConfidenceFailure — complete status metadata

**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_status_test.go`

**Preconditions**:
- AIAnalysis with `StartedAt` set to 5 seconds ago
- IncidentResponse with `NeedsHumanReview=false`, selected workflow present, `Confidence=0.3` (below 0.7 threshold)

**Test Steps**:
1. **Given**: An AIAnalysis in Analyzing phase with `StartedAt` set 5 seconds ago
2. **When**: `ProcessIncidentResponse` is called with a low-confidence workflow response
3. **Then**: The handler routes to `handleLowConfidenceFailure` and:
   - `TotalAnalysisTime > 0`
   - Condition `InvestigationComplete` is `False` with reason `InvestigationFailed`
   - Condition `AnalysisComplete` is `False` with reason `AnalysisFailed`
   - Condition `WorkflowResolved` is `False` with reason `WorkflowResolutionFailed`
   - Condition `ApprovalRequired` is `False` with reason `NotApplicable`

### UT-AA-610-006: TotalAnalysisTime remains 0 when StartedAt is nil

**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_status_test.go`

**Preconditions**:
- AIAnalysis with `StartedAt = nil`
- IncidentResponse that triggers any terminal handler (e.g., NeedsHumanReview=true)

**Test Steps**:
1. **Given**: An AIAnalysis in Analyzing phase with `StartedAt` not set (nil)
2. **When**: `ProcessIncidentResponse` is called
3. **Then**: `TotalAnalysisTime == 0` (defensive: no panic, no negative values)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `noopAuditClient` (existing pattern from `investigating_handler_test.go`)
- **Location**: `test/unit/aianalysis/response_processor_status_test.go`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| #607 fix | Code | Merged in branch | UT-AA-610-003 depends on handleNotActionableFromIncident existing | Already present |

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write all 6 unit tests — all fail because handlers don't set the fields yet
2. **Phase 2 (GREEN)**: Add missing field assignments to all 5 handlers
3. **Phase 3 (REFACTOR)**: Extract shared helper for common status completion logic; verify E2E-AA-607-001 passes

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/610/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/aianalysis/response_processor_status_test.go` | 6 Ginkgo BDD tests |
| Production fix | `pkg/aianalysis/handlers/response_processor.go` | Missing fields added to all 5 handlers |

---

## 13. Execution

```bash
# Unit tests (focused)
go test ./test/unit/aianalysis/... -ginkgo.focus="610" -ginkgo.v

# All AA unit tests (regression check)
go test ./test/unit/aianalysis/... -ginkgo.v

# Coverage
go test ./test/unit/aianalysis/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep response_processor
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| E2E-AA-607-001 (`10_not_actionable_e2e_test.go:132`) | `TotalAnalysisTime > 0` | None — will pass after fix | Currently failing due to the bug this plan addresses |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-03 | Initial test plan |
