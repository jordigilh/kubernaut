# Test Plan: AA Preserves rootCauseAnalysis for no_matching_workflows

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-769-v1
**Feature**: AA controller preserves rootCauseAnalysis and rootCause when humanReviewReason=no_matching_workflows
**Version**: 1.0
**Created**: 2026-04-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/768-769-aa-no-matching-workflows`

---

## 1. Introduction

### 1.1 Purpose

This test plan ensures that the AA controller preserves the `rootCauseAnalysis` and
`rootCause` fields on the AIAnalysis CR status when KA returns a successful investigation
with `humanReviewReason: no_matching_workflows`. Currently, the `rootCause` field is not
populated (showing `N/A`) and the RCA data present in the KA response is lost during
the error-path processing. The RCA and workflow selection are independent phases — a
successful RCA should always be preserved regardless of whether a workflow was selected.

### 1.2 Objectives

1. **RootCause populated**: `status.rootCause` contains the RCA summary string from the KA response
2. **RootCauseAnalysis preserved**: `status.rootCauseAnalysis` contains the full RCA struct (summary, severity, contributingFactors, remediationTarget)
3. **Audit trail complete**: `ProviderResponseSummary.AnalysisPreview` contains the RCA summary (first 500 chars)
4. **RO notification context**: Manual review notification includes RCA for human reviewer context
5. **Backward compatibility**: RCA preservation for other paths (problem_resolved, not_actionable, normal success) unchanged

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/aianalysis/...` |
| Integration test pass rate | 100% | `go test ./test/integration/aianalysis/...` |
| RCA field accuracy | 100% | Both rootCause and rootCauseAnalysis populated correctly |
| Audit AnalysisPreview accuracy | Non-empty | ProviderResponseSummary.AnalysisPreview contains RCA summary |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #769: AA controller discards rootCauseAnalysis when humanReviewReason=no_matching_workflows
- BR-AI-008: Capture all response fields including RCA, workflow, and alternatives
- BR-AUDIT-005: RR reconstruction requires audit trail
- DD-AUDIT-005: Provider response summary

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #768: AA phase should be Completed (companion fix, separate test plan)
- Issue #97: Centralized RCA extraction helper

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | RCA extraction fails for edge-case payloads (nil, empty map, missing fields) | Medium — rootCause shows empty | Low | UT-AA-769-003 | Test with various RCA shapes including minimal and empty |
| R2 | Audit AnalysisPreview truncation breaks for very long RCA summaries | Low — cosmetic | Low | UT-AA-769-004 | Uses existing `truncateString(s, 500)` helper |
| R3 | RO manual review notification loses RCA context | Medium — operator impact | Low | UT-RO-769-001 | Verify `populateManualReviewContext` reads from updated fields |

---

## 4. Scope

### 4.1 Features to be Tested

- **AA Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): `rootCause` and `rootCauseAnalysis` populated in new handler
- **AA Audit** (`pkg/aianalysis/audit/audit.go`): `ProviderResponseSummary.AnalysisPreview` from `rootCause`
- **RO Notification** (`pkg/remediationorchestrator/handler/aianalysis.go`): `populateManualReviewContext` includes RCA

### 4.2 Features Not to be Tested

- **KA parser**: KA already returns correct RCA in the response
- **ExtractRootCauseAnalysis helper**: Already tested in existing unit tests (Issue #97)
- **RCA for other paths**: problem_resolved, not_actionable, normal success — unchanged

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Set `RootCause = rca.Summary` in new handler | Mirrors success path (response_processor.go:156); needed for audit AnalysisPreview |
| Reuse `ExtractRootCauseAnalysis` helper | Centralized extraction per Issue #97; includes remediationTarget |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of new handler code covering RCA population
- **Integration**: >=80% of integration-testable code verifying RCA on CR status

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass; `rootCause` and `rootCauseAnalysis` populated on CR; audit AnalysisPreview non-empty.

**FAIL**: `rootCause` shows "N/A" or empty; `rootCauseAnalysis` is nil; audit AnalysisPreview is empty.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | `handleNoMatchingWorkflowsCompleted` RCA block | ~15 |
| `pkg/aianalysis/audit/audit.go` | `RecordAnalysisComplete` ProviderResponseSummary | ~10 (existing) |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| AA controller → response processor → audit | Full pipeline with RCA | ~30 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-008 | RCA field capture | P0 | Unit | UT-AA-769-001 | Pending |
| BR-AI-008 | RCA full struct | P0 | Unit | UT-AA-769-002 | Pending |
| BR-AI-008 | RCA edge cases | P1 | Unit | UT-AA-769-003 | Pending |
| BR-AUDIT-005 | Audit AnalysisPreview | P0 | Unit | UT-AA-769-004 | Pending |
| BR-ORCH-036 | RO notification RCA context | P0 | Unit | UT-RO-769-001 | Pending |
| BR-AI-008 | Integration: RCA on CR | P0 | Integration | IT-AA-769-001 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AA-769-001` | AA sets rootCause = RCA summary string when humanReviewReason=no_matching_workflows | Pending |
| `UT-AA-769-002` | AA sets rootCauseAnalysis with full struct (summary, severity, contributingFactors, remediationTarget) | Pending |
| `UT-AA-769-003` | AA handles nil/empty RCA gracefully — rootCause remains empty, no panic | Pending |
| `UT-AA-769-004` | Audit RecordAnalysisComplete includes AnalysisPreview with RCA summary (truncated to 500 chars) | Pending |
| `UT-RO-769-001` | RO manual review notification context includes RCA summary for human reviewer | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AA-769-001` | Full AA pipeline: mock KA returns no_matching_workflows with RCA → CR status shows rootCause + rootCauseAnalysis | Pending |

### Tier Skip Rationale

- **E2E**: Covered by existing E2E suites. The mock-LLM `MOCK_NO_WORKFLOW_FOUND` scenario naturally includes RCA data that will surface on the CR once the AA code is fixed.

---

## 9. Test Cases

### UT-AA-769-001: rootCause populated from RCA summary

**BR**: BR-AI-008
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_no_matching_test.go`

**Preconditions**:
- ResponseProcessor with mock audit client and metrics

**Test Steps**:
1. **Given**: IncidentResponse with NeedsHumanReview=true, HumanReviewReason=no_matching_workflows, RootCauseAnalysis containing summary="The namespace-quota ResourceQuota is exhausted", severity="medium", contributingFactors=["quota caps memory"]
2. **When**: ProcessIncidentResponse is called
3. **Then**: analysis.Status.RootCause == "The namespace-quota ResourceQuota is exhausted"

**Expected Results**:
1. rootCause contains the RCA summary string
2. rootCause is NOT "N/A" or empty

### UT-AA-769-002: rootCauseAnalysis full struct preserved

**BR**: BR-AI-008
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_no_matching_test.go`

**Preconditions**:
- Same as UT-AA-769-001

**Test Steps**:
1. **Given**: Same IncidentResponse as UT-AA-769-001 with remediationTarget={kind:Deployment, name:api-server, namespace:demo-quota}
2. **When**: ProcessIncidentResponse is called
3. **Then**: analysis.Status.RootCauseAnalysis is non-nil with all fields populated

**Expected Results**:
1. RootCauseAnalysis.Summary matches input
2. RootCauseAnalysis.Severity == "medium"
3. RootCauseAnalysis.ContributingFactors length > 0
4. RootCauseAnalysis.RemediationTarget.Kind == "Deployment"
5. RootCauseAnalysis.RemediationTarget.Name == "api-server"

### UT-AA-769-003: Nil RCA handled gracefully

**BR**: BR-AI-008
**Priority**: P1
**Type**: Unit
**File**: `test/unit/aianalysis/response_processor_no_matching_test.go`

**Test Steps**:
1. **Given**: IncidentResponse with NeedsHumanReview=true, HumanReviewReason=no_matching_workflows, RootCauseAnalysis=nil (empty)
2. **When**: ProcessIncidentResponse is called
3. **Then**: analysis.Status.RootCause == "" (empty, not panic), analysis.Status.RootCauseAnalysis == nil

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Audit client (mock)
- **Location**: `test/unit/aianalysis/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest, httptest
- **Location**: `test/integration/aianalysis/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|------------|------|--------|--------|------------|
| Issue #768 (Phase=Completed) | Code | Same branch | RCA tests depend on new handler | Implement together |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write UT-AA-769-001 through UT-AA-769-004 and UT-RO-769-001 — all must fail
2. **Phase 2 (TDD GREEN)**: Implement RCA population in new handler — all unit tests pass
3. **Phase 3 (TDD REFACTOR)**: Verify audit AnalysisPreview; clean up
4. **Phase 4**: Write IT-AA-769-001 — integration test passes

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/769/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/aianalysis/response_processor_no_matching_test.go` | RCA preservation |
| Integration test suite | `test/integration/aianalysis/` | Full pipeline RCA verification |

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `response_processor_status_test.go` (UT-AA-610-004) | Does not assert RootCause | Add RootCause assertion for no_matching_workflows path | #769 |
| `investigating_handler_test.go:476-498` | RootCauseAnalysis != nil (already) | Verify RootCause is also populated | #769 |
| `test/unit/remediationorchestrator/aianalysis_handler_test.go` (UT-RO-550-010) | Notification RCA from RootCause | Verify RootCause is now populated (was empty before) | #769 |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-21 | Initial test plan |
