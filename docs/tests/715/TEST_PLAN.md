# Test Plan: Phase 1-to-Phase 3 Context Propagation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-715-v1
**Feature**: Propagate Phase 1 structured RCA fields and assessment into Phase 3 workflow selection for HAPI parity
**Version**: 1.0
**Created**: 2026-04-17
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/715-phase1-to-phase3-propagation`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that KA correctly propagates structured Phase 1 RCA fields and assessment data (investigation_outcome, confidence) into the Phase 3 workflow selection prompt, and merges Phase 1 values as fallbacks into the final result when Phase 3 does not produce them. This restores HAPI parity lost during the two-session split.

### 1.2 Objectives

1. **Prompt injection**: Phase 3 prompt contains a structured "Phase 1 Assessment" section with RCA severity, contributing factors, investigation_outcome, and confidence from Phase 1
2. **Backward compatibility**: When Phase 1 context is nil (e.g., nil enricher, parse failure fallback), Phase 3 renders identically to the pre-change output
3. **Fallback merge**: When Phase 3 does not produce `investigation_outcome` or `confidence`, Phase 1 values propagate to the final result (HAPI `result.setdefault` pattern)
4. **Phase 3 precedence**: When Phase 3 explicitly sets `investigation_outcome` or `confidence`, Phase 3 values win over Phase 1 fallbacks
5. **InvestigationOutcome preservation**: The raw `investigation_outcome` string from LLM is stored on `InvestigationResult` for downstream propagation

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/prompt/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| Build | 0 errors | `go build ./...` |
| Lint | 0 new errors | `go vet` + linter |
| Backward compatibility | 0 regressions | All existing prompt + investigator tests pass |

---

## 2. References

### 2.1 Authority

- **BR-HAPI-200**: Special investigation outcomes (investigation_outcome routing)
- **DD-HAPI-006 v1.6**: Three-phase RCA architecture
- **Issue #715**: Phase 1 context not propagated to Phase 3 — incorrect escalation for repeated ineffective remediations

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [TP-700-v1](../../tests/700/TEST_PLAN.md) — Phase separation test plan (prerequisite)
- HAPI `llm_integration.py` at `cdcca916a~1` — authoritative Phase 1→3 propagation

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `investigation_outcome` raw string lost after parsing | Phase 1 outcome cannot be propagated | High | UT-KA-715-004, IT-KA-715-001..003 | Add `InvestigationOutcome` field to `InvestigationResult`; store in `applyFlatFields` |
| R2 | Fallback merge silently overrides Phase 3 decision | Incorrect escalation when Phase 3 explicitly disagrees | Medium | IT-KA-715-003 | Phase 3 values always take precedence; Phase 1 is setdefault only |
| R3 | Existing prompt tests break from new section | CI regression | Medium | UT-KA-700-006, UT-KA-433-PRM-007 | Verified: no ABSENCE assertions for Phase 1 content in Phase 3 tests |
| R4 | `ApplyInvestigationOutcome` unexported | Cannot apply Phase 1 outcome in investigator | Low | IT-KA-715-002 | Export function from parser package |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by UT-KA-715-004 (parser preserves raw string)
- **R2**: Mitigated by IT-KA-715-003 (Phase 3 precedence)
- **R3**: Verified during preflight — no blocking assertions
- **R4**: Mitigated by exporting function in GREEN phase

---

## 4. Scope

### 4.1 Features to be Tested

- **Prompt builder** (`internal/kubernautagent/prompt/builder.go`): `RenderWorkflowSelection` accepts Phase 1 context and renders structured assessment section
- **Phase 3 template** (`internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl`): New `Phase 1 Assessment` section
- **Investigator** (`internal/kubernautagent/investigator/investigator.go`): Captures Phase 1 fields, passes to Phase 3, merges fallbacks
- **Parser** (`internal/kubernautagent/parser/parser.go`): Preserves raw `investigation_outcome` on result
- **Types** (`internal/kubernautagent/types/types.go`): New `InvestigationOutcome` field

### 4.2 Features Not to be Tested

- **`can_recover` field**: HAPI propagates this but KA has no use case yet. Deferred to separate issue.
- **Schema/template/parser enum mismatch**: Pre-existing issue where schema enum and parser switch values differ. Separate issue.
- **Mock LLM scenarios**: Mock LLM uses keyword matching, not prompt-structure matching. No expected regressions.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Structured fields, not full LLM text | Phase 3 needs assessment data, not verbose narrative. Reduces token usage and prevents Phase 3 re-investigation. |
| `setdefault` merge pattern | Matches HAPI behavior: Phase 3 values always take precedence over Phase 1 fallbacks |
| `Phase1Data` as separate parameter | Semantically distinct from enrichment data; keeps `EnrichmentData` clean |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of prompt builder Phase 1 context rendering + parser `InvestigationOutcome` preservation
- **Integration**: >=80% of investigator Phase 1→3 propagation and fallback merge

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass, 0 regressions, build clean.
**FAIL**: Any P0 test fails, any existing test regresses.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/prompt/builder.go` | `RenderWorkflowSelection`, `workflowTemplateData` | ~40 |
| `internal/kubernautagent/parser/parser.go` | `applyFlatFields` | ~5 |
| `internal/kubernautagent/types/types.go` | `InvestigationResult.InvestigationOutcome` | ~2 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `Investigate`, `runRCA`, `runWorkflowSelection` | ~30 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-200 | Phase 1 assessment propagated to Phase 3 prompt | P0 | Unit | UT-KA-715-001 | Pending |
| BR-HAPI-200 | Nil Phase 1 context backward compatibility | P0 | Unit | UT-KA-715-002 | Pending |
| BR-HAPI-200 | investigation_outcome + confidence in prompt | P0 | Unit | UT-KA-715-003 | Pending |
| BR-HAPI-200 | Parser preserves raw investigation_outcome | P0 | Unit | UT-KA-715-004 | Pending |
| BR-HAPI-200 | Phase 3 prompt contains Phase 1 structured data | P0 | Integration | IT-KA-715-001 | Pending |
| BR-HAPI-200 | Phase 1 inconclusive fallback merge | P0 | Integration | IT-KA-715-002 | Pending |
| BR-HAPI-200 | Phase 3 explicit outcome takes precedence | P0 | Integration | IT-KA-715-003 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-715-001` | Phase 3 prompt includes structured Phase 1 assessment (severity, contributing factors, remediation target) | Pending |
| `UT-KA-715-002` | Phase 3 prompt renders identically when Phase 1 context is nil (backward compat) | Pending |
| `UT-KA-715-003` | Phase 3 prompt includes investigation_outcome and confidence from Phase 1 | Pending |
| `UT-KA-715-004` | Parser stores raw investigation_outcome string on InvestigationResult | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-715-001` | Full Investigate() — Phase 3 system prompt contains Phase 1 RCA structured data | Pending |
| `IT-KA-715-002` | Full Investigate() — Phase 1 inconclusive + Phase 3 no outcome = final HumanReviewNeeded=true | Pending |
| `IT-KA-715-003` | Full Investigate() — Phase 3 explicitly sets actionable = Phase 3 wins | Pending |

### Tier Skip Rationale

- **E2E**: Phase 1→3 propagation is internal investigator plumbing. E2E coverage comes from existing full-pipeline tests that exercise the investigator end-to-end. No new E2E needed.

---

## 9. Test Cases

### UT-KA-715-001: Phase 3 prompt includes Phase 1 assessment

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: Builder with Phase 1 context containing severity="high", contributing_factors=["memory leak", "no HPA"], remediation_target=Deployment/api-server
2. **When**: `RenderWorkflowSelection` called with Phase 1 context
3. **Then**: Rendered prompt contains "Phase 1 Assessment" section with severity, contributing factors, and remediation target

### UT-KA-715-002: Nil Phase 1 context backward compatibility

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: Builder with nil Phase 1 context
2. **When**: `RenderWorkflowSelection` called with nil Phase 1 data
3. **Then**: Rendered prompt does NOT contain "Phase 1 Assessment" section

### UT-KA-715-003: investigation_outcome and confidence in prompt

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: Phase 1 context with investigation_outcome="inconclusive", confidence=0.45
2. **When**: `RenderWorkflowSelection` called
3. **Then**: Rendered prompt contains "inconclusive" and "0.45"

### UT-KA-715-004: Parser preserves raw investigation_outcome

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `"investigation_outcome":"inconclusive"`
2. **When**: `Parse()` called
3. **Then**: `result.InvestigationOutcome` equals "inconclusive"

### IT-KA-715-001: Phase 3 prompt contains Phase 1 data

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_phase_propagation_test.go`

**Test Steps**:
1. **Given**: Mock LLM returns Phase 1 with severity="high", contributing_factors=["memory leak"]
2. **When**: `Investigate()` runs both phases
3. **Then**: Phase 3 system prompt (captured from mock) contains "Phase 1 Assessment", "high", "memory leak"

### IT-KA-715-002: Phase 1 inconclusive fallback merge

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_phase_propagation_test.go`

**Test Steps**:
1. **Given**: Phase 1 returns investigation_outcome=inconclusive, confidence=0.4; Phase 3 returns workflow_id with no investigation_outcome
2. **When**: `Investigate()` runs
3. **Then**: Final result has `HumanReviewNeeded=true` (from Phase 1 fallback)

### IT-KA-715-003: Phase 3 explicit outcome takes precedence

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_phase_propagation_test.go`

**Test Steps**:
1. **Given**: Phase 1 returns investigation_outcome=inconclusive; Phase 3 explicitly returns investigation_outcome=actionable with workflow
2. **When**: `Investigate()` runs
3. **Then**: Final result has `IsActionable=true`, `HumanReviewNeeded=false` (Phase 3 wins)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None (pure logic)
- **Location**: `test/unit/kubernautagent/prompt/`, `test/unit/kubernautagent/parser/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `mockLLMClient` (external LLM API), `fakeK8sClient` (K8s API), `fakeDataStorageClient` (DB)
- **Location**: `test/integration/kubernautagent/investigator/`

---

## 11. Execution Order

1. **Phase 1 (RED)**: Write failing UT-KA-715-001..004 and IT-KA-715-001..003
2. **Phase 2 (GREEN)**: Implement types, parser, builder, investigator changes
3. **Phase 3 (REFACTOR)**: Extract helpers, improve documentation

---

## 12. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/prompt/... -ginkgo.v -ginkgo.focus="UT-KA-715"
go test ./test/unit/kubernautagent/parser/... -ginkgo.v -ginkgo.focus="UT-KA-715"

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v -ginkgo.focus="IT-KA-715"

# All existing tests (regression check)
go test ./test/unit/kubernautagent/prompt/... -ginkgo.v
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v
```

---

## 13. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `UT-KA-433-018` (builder_test.go:99) | `RenderWorkflowSelection(signal, "OOMKilled root cause", enrichData)` | Add nil Phase 1 parameter | Signature change |
| `UT-KA-686-008/009` (builder_test.go:178,204) | `RenderWorkflowSelection(signal, "OOMKilled root cause", nil)` | Add nil Phase 1 parameter | Signature change |
| `UT-KA-700-006` (prompt_phase_separation_test.go) | `RenderWorkflowSelection(...)` | Add nil Phase 1 parameter | Signature change |
| `UT-KA-433-PRM-005..007` (adversarial_prompt_test.go) | `RenderWorkflowSelection(...)` | Add nil Phase 1 parameter | Signature change |
| `IT-KA-433-007` (investigator_test.go:227) | Phase 3 content contains "memory leak" | Should still pass (Phase 1 context adds data, doesn't remove) | Verify no regression |

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-17 | Initial test plan |
