# Test Plan: AA Phase=Completed for no_matching_workflows

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-768-v1
**Feature**: AA controller sets Phase=Completed (not Failed) when humanReviewReason=no_matching_workflows
**Version**: 1.0
**Created**: 2026-04-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/768-769-aa-no-matching-workflows`

---

## 1. Introduction

### 1.1 Purpose

This test plan ensures that when the LLM correctly determines no workflow matches an incident
and returns `humanReviewReason: no_matching_workflows`, the AA controller classifies the
AIAnalysis CR as `Phase=Completed` (not `Failed`). The investigation was successful — it
correctly concluded that no automated remediation is available. This also validates that
the RO correctly routes Completed+NeedsHumanReview cases to the ManualReviewRequired path.

### 1.2 Objectives

1. **Phase correctness**: AA sets `Phase=Completed` for `no_matching_workflows` responses
2. **Condition correctness**: `InvestigationComplete=True`, `AnalysisComplete=True`, `WorkflowResolved=False` with reason `NoMatchingWorkflows`
3. **RO routing**: RO routes `Completed+NeedsHumanReview+nil SelectedWorkflow` to `handleManualReviewCompleted`, producing `RR.Outcome=ManualReviewRequired`
4. **Audit correctness**: `aianalysis.analysis.completed` event emitted (not `.failed`), with `event_outcome=success`
5. **K8s events**: `AnalysisCompleted` + `HumanReviewRequired` events emitted (not `AnalysisFailed`)
6. **Backward compatibility**: Other `humanReviewReason` values (e.g., `parameter_validation_failed`, `low_confidence`) remain `Phase=Failed`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/aianalysis/...` |
| Integration test pass rate | 100% | `go test ./test/integration/aianalysis/...` |
| RO unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| RO integration test pass rate | 100% | `go test ./test/integration/remediationorchestrator/...` |
| Backward compatibility | 0 regressions | Existing tests pass (with documented assertion updates) |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #768: AA controller: phase should be Completed (not Failed) when humanReviewReason=no_matching_workflows
- BR-HAPI-197: Check needs_human_review before proceeding
- BR-AI-050: AIAnalysis must detect terminal states
- BR-ORCH-036: Manual review notification
- BR-ORCH-037: Workflow not needed handling

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #769: AA controller discards rootCauseAnalysis (companion fix, separate test plan)
- Issue #760: llm_parsing_error misclassification (predecessor fix)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | RO falls through to WFE creation with nil SelectedWorkflow after Phase change | Critical — panic/broken RR | High without RO fix | UT-RO-768-001, IT-RO-768-001 | Add NeedsHumanReview guard in RO reconciler + handler |
| R2 | Other humanReviewReasons (e.g. llm_parsing_error) accidentally routed to Completed | High — misclassification | Medium | UT-AA-768-005 | Explicit `== "no_matching_workflows"` check |
| R3 | Prometheus dashboards lose NoMatchingWorkflows failure signal | Medium — observability gap | Certain | N/A (external) | Document in PR; key on NeedsHumanReview instead |
| R4 | `IsWorkflowResolutionFailed` / `RequiresManualReview` RO helpers return wrong values | Low — test-only today | Certain | UT-RO-768-003 | Update helpers in REFACTOR phase |

### 3.1 Risk-to-Test Traceability

- R1: UT-RO-768-001 (handler routes correctly), IT-RO-768-001 (RO integration with Completed+NeedsHumanReview)
- R2: UT-AA-768-005 (other reasons remain Failed)
- R4: UT-RO-768-003 (helper accuracy)

---

## 4. Scope

### 4.1 Features to be Tested

- **AA Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): New handler for no_matching_workflows → Phase=Completed
- **RO Handler** (`pkg/remediationorchestrator/handler/aianalysis.go`): Completed+NeedsHumanReview routing to ManualReviewCompleted
- **RO Reconciler** (`internal/controller/remediationorchestrator/reconciler.go`): Completed branch guard for NeedsHumanReview
- **AA Conditions** (`pkg/aianalysis/conditions.go`): New `ReasonNoMatchingWorkflows` constant
- **AA Audit** (`pkg/aianalysis/audit/audit.go`): RecordAnalysisComplete for no_matching_workflows

### 4.2 Features Not to be Tested

- **KA investigator**: No changes needed — KA already returns correct `humanReviewReason`
- **Notification controller**: Not affected by Phase change (due diligence confirmed)
- **EM controller**: Not affected (no AIAnalysis phase references)
- **Mock LLM**: No changes needed

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Reuse `ReasonAnalysisCompleted` for Status.Reason | Analysis completed; SubReason=NoMatchingWorkflows distinguishes it. Avoids CRD schema change. |
| New condition reason `ReasonNoMatchingWorkflows` in conditions.go | Per issue #768 requirement; condition string not a CRD enum (no schema change) |
| Reuse existing `handleManualReviewCompleted` in RO | Battle-tested path for RR.Outcome=ManualReviewRequired |
| Check `humanReviewReason == "no_matching_workflows"` explicitly | Prevents other failure reasons from being promoted to Completed |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in response_processor.go, handler/aianalysis.go, conditions.go
- **Integration**: >=80% of integration-testable code in AA + RO controller wiring

### 5.2 Two-Tier Minimum

Every business requirement covered by Unit + Integration tests.

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass; per-tier coverage >=80%; no regressions; RR outcome unchanged (ManualReviewRequired).

**FAIL**: Any P0 test fails; Phase still shows Failed for no_matching_workflows; RO routes to WFE creation.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | `handleNoMatchingWorkflowsCompleted` (new), routing in `handleWorkflowResolutionFailureFromIncident` | ~60 |
| `pkg/remediationorchestrator/handler/aianalysis.go` | `handleCompleted` (updated), `IsWorkflowResolutionFailed`, `RequiresManualReview` | ~30 |
| `pkg/aianalysis/conditions.go` | `ReasonNoMatchingWorkflows` constant | ~3 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `handleAnalyzingPhase` Completed branch | ~20 |
| AA controller wiring (investigating → response processor) | Full pipeline | ~50 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-197 | Human review handling | P0 | Unit | UT-AA-768-001 | Pending |
| BR-HAPI-197 | Human review handling | P0 | Unit | UT-AA-768-002 | Pending |
| BR-HAPI-197 | Human review handling | P0 | Integration | IT-AA-768-001 | Pending |
| BR-AI-050 | Terminal state detection | P0 | Unit | UT-AA-768-003 | Pending |
| BR-AI-050 | Terminal state detection | P0 | Unit | UT-AA-768-004 | Pending |
| BR-HAPI-197 | Non-regression: other reasons | P0 | Unit | UT-AA-768-005 | Pending |
| BR-ORCH-036 | RO manual review routing | P0 | Unit | UT-RO-768-001 | Pending |
| BR-ORCH-036 | RO manual review routing | P0 | Unit | UT-RO-768-002 | Pending |
| BR-ORCH-036 | RO helper accuracy | P1 | Unit | UT-RO-768-003 | Pending |
| BR-ORCH-036 | RO integration routing | P0 | Integration | IT-RO-768-001 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AA-768-001` | AA sets Phase=Completed when KA returns needsHumanReview=true + humanReviewReason=no_matching_workflows | Pending |
| `UT-AA-768-002` | AA sets InvestigationComplete=True, AnalysisComplete=True for no_matching_workflows | Pending |
| `UT-AA-768-003` | AA sets WorkflowResolved=False with reason NoMatchingWorkflows (not WorkflowResolutionFailed) | Pending |
| `UT-AA-768-004` | AA calls RecordAnalysisComplete (not RecordAnalysisFailed) for no_matching_workflows | Pending |
| `UT-AA-768-005` | AA preserves Phase=Failed for other humanReviewReasons (parameter_validation_failed, low_confidence, etc.) | Pending |
| `UT-RO-768-001` | RO handleCompleted routes NeedsHumanReview+nil SelectedWorkflow to handleManualReviewCompleted | Pending |
| `UT-RO-768-002` | RO sets RR.Outcome=ManualReviewRequired when AI Phase=Completed+NeedsHumanReview | Pending |
| `UT-RO-768-003` | RequiresManualReview helper returns true for Completed+NeedsHumanReview+nil workflow | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AA-768-001` | Full AA pipeline: mock KA returns no_matching_workflows → AA CR shows Phase=Completed, correct conditions | Pending |
| `IT-RO-768-001` | RO reconciler with Completed+NeedsHumanReview AI → RR.Outcome=ManualReviewRequired, notification created | Pending |

### Tier Skip Rationale

- **E2E**: Covered by existing E2E suites that exercise the full pipeline with mock-LLM scenarios. The mock-LLM `MOCK_NO_WORKFLOW_FOUND` scenario will naturally validate the change once AA/RO code is updated.

---

## 9. Test Cases

### UT-AA-768-001: Phase=Completed for no_matching_workflows

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_no_matching_test.go`

**Preconditions**:
- ResponseProcessor with mock audit client and metrics

**Test Steps**:
1. **Given**: IncidentResponse with NeedsHumanReview=true, HumanReviewReason=no_matching_workflows, Confidence=0.98, RCA present
2. **When**: ProcessIncidentResponse is called
3. **Then**: analysis.Status.Phase == PhaseCompleted

**Expected Results**:
1. Phase is Completed (not Failed)
2. Reason is AnalysisCompleted
3. SubReason is NoMatchingWorkflows
4. NeedsHumanReview is true
5. HumanReviewReason is "no_matching_workflows"

### UT-AA-768-005: Other reasons remain Failed

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_no_matching_test.go`

**Preconditions**:
- ResponseProcessor with mock audit client and metrics

**Test Steps**:
1. **Given**: Table of IncidentResponses with HumanReviewReason = {parameter_validation_failed, image_mismatch, low_confidence, llm_parsing_error, investigation_inconclusive, rca_incomplete}
2. **When**: ProcessIncidentResponse is called for each
3. **Then**: analysis.Status.Phase == PhaseFailed for all entries

### UT-RO-768-001: RO routes Completed+NeedsHumanReview to ManualReviewCompleted

**BR**: BR-ORCH-036
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go`

**Preconditions**:
- AIAnalysisHandler with mock notification creator and client

**Test Steps**:
1. **Given**: AIAnalysis with Phase=Completed, NeedsHumanReview=true, SelectedWorkflow=nil, HumanReviewReason=no_matching_workflows
2. **When**: HandleAIAnalysisStatus is called
3. **Then**: RR.Status.Outcome == "ManualReviewRequired", RR.Status.OverallPhase == PhaseCompleted

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Audit client (mock), metrics (real)
- **Location**: `test/unit/aianalysis/`, `test/unit/remediationorchestrator/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (K8s API), httptest (mock KA)
- **Location**: `test/integration/aianalysis/`, `test/integration/remediationorchestrator/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (TDD RED)**: Write UT-AA-768-001 through UT-AA-768-005 and UT-RO-768-001 through UT-RO-768-003 — all must fail
2. **Phase 2 (TDD GREEN)**: Implement AA handler + RO routing — all unit tests pass
3. **Phase 3 (TDD REFACTOR)**: Clean up helpers, update existing tests with new assertions
4. **Phase 4**: Write IT-AA-768-001, IT-RO-768-001 — integration tests pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/768/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (AA) | `test/unit/aianalysis/response_processor_no_matching_test.go` | Phase correctness |
| Unit test suite (RO) | `test/unit/remediationorchestrator/aianalysis_handler_test.go` | Routing correctness |
| Integration test suite | `test/integration/aianalysis/`, `test/integration/remediationorchestrator/` | Wiring correctness |

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `investigating_handler_test.go:392-414` (no_matching_workflows row) | `Phase == Failed`, `Reason == WorkflowResolutionFailed` | `Phase == Completed`, `Reason == AnalysisCompleted` | #768 semantic change |
| `error_handling_integration_test.go:100-149` | `Phase == Failed`, audit `Phase == "Failed"` | `Phase == Completed`, audit `Phase == "Completed"` | #768 |
| `approval_context_integration_test.go:186-268` (no_workflow_found row) | `Phase == "Failed"` | `Phase == "Completed"` | #768 |
| `approval_context_integration_test.go:271-336` (zero confidence row) | `Phase == "Failed"` | `Phase == "Completed"` | #768 |
| `response_processor_status_test.go:185-215` (UT-AA-610-004) | Conditions: InvestigationComplete=False | InvestigationComplete=True | #768 |
| `test/unit/remediationorchestrator/aianalysis_handler_test.go` (UT-RO-550-*) | AI `Phase = "Failed"` | AI `Phase = "Completed"` for no_matching_workflows entries | #768 |
| `test/integration/remediationorchestrator/needs_human_review_integration_test.go:313-376` | AI `Phase: "Failed"` | AI `Phase: "Completed"` | #768 |
| `test/unit/remediationorchestrator/controller/test_helpers.go:501-511` | `newAIAnalysisWorkflowResolutionFailed` uses PhaseFailed | Add variant for Completed+NeedsHumanReview | #768 |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-21 | Initial test plan |
