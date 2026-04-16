# Test Plan: Enrichment-Driven `rca_incomplete` Detection

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-704-v1
**Feature**: Investigator sets `rca_incomplete` when enrichment owner chain resolution fails
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/700-needs-human-review-parser-derived`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that KA's investigator correctly detects enrichment owner chain resolution failures and sets `needs_human_review=true` with `human_review_reason="rca_incomplete"`, aligning with HAPI's authoritative behavior per DD-HAPI-006 v1.6 and BR-HAPI-261 acceptance criteria #7.

### 1.2 Objectives

1. **Owner chain error surfacing**: `EnrichmentResult.OwnerChainError` is populated when `GetOwnerChain` returns an error
2. **Investigator completeness gate**: Investigator checks final `enrichData.OwnerChainError` after re-enrichment and before workflow selection
3. **rca_incomplete HR derivation**: When owner chain resolution fails, `HumanReviewNeeded=true` and `HumanReviewReason="rca_incomplete"` are set
4. **Workflow phase skipped**: When `rca_incomplete` triggers, `runWorkflowSelection` is not invoked
5. **E2E restoration**: E2E-KA-433-ADV-016 assertion restored to expect `needs_human_review=true` with `rca_incomplete`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| E2E test pass rate | 100% | E2E-KA-433-ADV-016 passes with restored assertion |
| Build | 0 errors | `go build ./...` |
| Lint | 0 new errors | `golangci-lint run` |
| Backward compatibility | 0 regressions | All existing enrichment + investigator tests pass |

---

## 2. References

### 2.1 Authority

- **BR-HAPI-261**: LLM-Provided Affected Resource with Owner Resolution (AC #7: owner chain resolution fails → `rca_incomplete`)
- **BR-HAPI-264**: Post-RCA Infrastructure Label Detection (AC #6: label detection fails → `rca_incomplete`)
- **DD-HAPI-006 v1.6**: Three-phase RCA architecture — enrichment failure → `rca_incomplete`
- **Issue #704**: Enrichment-driven `rca_incomplete` detection

### 2.2 Cross-References

- **Issue #700**: Parser-driven escalation (prerequisite — HR fields removed from LLM, parser-derived only)
- **TP-700-v1**: Test plan for parser-driven escalation (E2E ADV-016 temporarily relaxed there)
- **IT-KA-433-ENR-004/005**: Existing enrichment failure integration tests (broken K8s scenarios)

---

## 3. Test Items

### 3.1 Production Code Under Test

| File | Change | Lines |
|------|--------|-------|
| `internal/kubernautagent/enrichment/enricher.go` | Add `OwnerChainError` field to `EnrichmentResult`; populate in `Enrich()` | ~3 lines |
| `internal/kubernautagent/investigator/investigator.go` | Add completeness check after re-enrichment, before workflow selection | ~12 lines |

### 3.2 Test Code

| File | Change | Test ID |
|------|--------|---------|
| `test/integration/kubernautagent/investigator/investigator_test.go` | New integration test | IT-KA-704-001 |
| `test/e2e/kubernautagent/adversarial_parity_e2e_test.go` | Restore assertion | E2E-KA-433-ADV-016 |

---

## 4. Test Scenarios

### IT-KA-704-001: Owner chain failure triggers rca_incomplete

**Tier**: Integration
**BR**: BR-HAPI-261 AC#7
**Preconditions**: `fakeK8sClient` configured with `err: fmt.Errorf("resource not found")`

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create investigator with failing K8s client | Enricher created with error-returning client |
| 2 | Mock LLM returns valid RCA (phase 1) | RCA completes normally |
| 3 | Investigator runs enrichment | `OwnerChainError` set on `EnrichmentResult` |
| 4 | Investigator checks completeness | Early return before workflow selection |
| 5 | Assert result | `HumanReviewNeeded=true`, `HumanReviewReason="rca_incomplete"` |
| 6 | Assert RCA preserved | `RCASummary` populated from phase 1 |
| 7 | Assert workflow skipped | `WorkflowID` is empty |

### E2E-KA-433-ADV-016: rca_incomplete via enrichment failure (restored)

**Tier**: E2E
**BR**: BR-HAPI-261 AC#7
**Preconditions**: Kind cluster without target Pods; Mock LLM `mock_rca_incomplete` scenario

| Step | Action | Expected |
|------|--------|----------|
| 1 | Send incident with `mock_rca_incomplete` keyword | Mock LLM returns actionable RCA |
| 2 | Enrichment targets non-existent Pod | `GetOwnerChain` returns error |
| 3 | Investigator detects enrichment failure | Sets `rca_incomplete` |
| 4 | Assert response | `needs_human_review=true`, `human_review_reason="rca_incomplete"` |

---

## 5. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `enrichData == nil` (no enricher) bypasses check | Low | High | Check guards on `enrichData != nil` |
| Existing enrichment tests affected | Low | Medium | Tests use different K8s clients; `OwnerChainError` only set on actual error |
| Mid-walk failures not detected | Known | Low | Out of scope per #704; partial chain is acceptable |

---

## 6. TDD Phase Tracking

| Phase | Status | Description |
|-------|--------|-------------|
| RED | Complete | IT-KA-704-001 written, E2E ADV-016 restored — both failed for right reason |
| GREEN | Complete | `OwnerChainError` added, `Enrich()` populates, investigator gates — all tests pass |
| REFACTOR | Complete | Doc comments on `OwnerChainError` field |
