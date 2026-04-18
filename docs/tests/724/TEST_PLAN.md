# Test Plan: Phase 3 Escalation Context (#724, #725)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-724-v1
**Feature**: Phase 1 investigation_analysis field for Phase 3 context (#724) and prompt engineering fixes for escalation guidance (#725)
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Agent
**Status**: Active
**Branch**: `fix/724-725-phase3-escalation-context`

---

## 1. Introduction

### 1.1 Purpose

Phase 3 (workflow selection) cannot correctly escalate to ManualReviewRequired when prior remediations were ineffective. Two root causes:
1. Phase 3 LLM lacks Phase 1's investigation reasoning (only gets a one-line summary)
2. Prompt guidance references non-existent `needs_human_review` schema field and lacks concrete inconclusive example JSON

### 1.2 Objectives

1. **#725**: Prompt warnings reference actual schema fields (`investigation_outcome: inconclusive`) instead of non-existent `needs_human_review`
2. **#725**: Phase 3 template includes concrete inconclusive example JSON so LLMs can anchor on it
3. **#724**: Phase 1 schema supports `investigation_analysis` narrative field
4. **#724**: Phase 1 investigation analysis propagates to Phase 3 prompt with sanitization
5. **#724**: Backward compatibility: empty `investigation_analysis` produces no extra template sections

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/prompt/... -ginkgo.v` |
| Parser test pass rate | 100% | `go test ./test/unit/kubernautagent/parser/... -ginkgo.v` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on history.go, builder.go, parser.go |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-HAPI-016**: Remediation history context enrichment
- **BR-HAPI-200**: Investigation outcome routing (actionable, inconclusive, resolved)
- **BR-HAPI-263**: Conversation continuity between phases
- **Issue #724**: Phase 1 investigation_analysis field
- **Issue #725**: Prompt engineering fixes
- **Issue #715**: Phase 1→3 propagation (predecessor — structured fields)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Plan #715](../tests/715/TEST_PLAN.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | LLM produces verbose investigation_analysis exceeding token budget | Phase 3 context window overflow | Medium | UT-KA-724-002 | Prompt guidance caps at "concise, < 500 words"; schema description |
| R2 | Phase 1 schema change shifts LLM structured output behavior | Unexpected field interactions | Low | UT-KA-724-001, UT-KA-724-004 | Field is optional; existing behavior preserved when absent |
| R3 | Prompt injection via investigation_analysis content | Adversarial Phase 3 behavior | Medium | UT-KA-724-SEC-001 | Apply existing sanitizeField() pattern |
| R4 | Changing history.go warning text breaks existing test assertions | False negative regression | Low | UT-KA-725-001, UT-KA-725-002 | Due diligence confirmed only "MANDATORY: You MUST NOT re-select" is asserted, not the `needs_human_review` portion |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-KA-724-002 validates rendering with populated field; prompt guidance review in REFACTOR phase
- **R3**: UT-KA-724-SEC-001 validates sanitization
- **R4**: UT-KA-725-001/002 validate new wording; existing history_test.go assertions verified unaffected

---

## 4. Scope

### 4.1 Features to be Tested

- **history.go warning text** (`internal/kubernautagent/prompt/history.go`): Correct schema field references in escalation warnings
- **Phase 3 template** (`internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl`): Inconclusive example JSON and investigation analysis rendering
- **Phase 1 schema** (`internal/kubernautagent/parser/schema.go`): `investigation_analysis` field presence
- **Parser** (`internal/kubernautagent/parser/parser.go`): Extraction of `investigation_analysis`
- **Types** (`internal/kubernautagent/types/types.go`): `InvestigationAnalysis` field
- **Builder** (`internal/kubernautagent/prompt/builder.go`): Phase1Data propagation and rendering
- **Investigator** (`internal/kubernautagent/investigator/investigator.go`): buildPhase1Context propagation

### 4.2 Features Not to be Tested

- **KA→AA API contract**: No changes needed (investigation_outcome is already handled by derived fields)
- **AIA CRD**: No changes needed
- **RO routing**: Not affected by these changes
- **Mock LLM scenarios**: No dependency on exact warning text (verified in due diligence)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `investigation_analysis` under `root_cause_analysis` in schema | Keeps RCA-related fields grouped; mirrors HAPI's "Phase 1 RCA analysis text" |
| Separate template section from Phase1Assessment | Phase1Assessment has structured bullets; investigation_analysis is narrative prose |
| Render investigation_analysis BEFORE enrichment context | Phase 3 LLM reads investigation reasoning before seeing history warnings |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of history.go warning paths, builder.go Phase 3 rendering, parser.go extraction
- **Integration**: Existing IT-KA-715 tests cover investigator propagation; extend if needed
- **E2E**: Existing E2E pipeline tests exercise full flow; no new E2E needed

### 5.2 Two-Tier Minimum

- **Unit tests**: Catch logic and correctness (warning text, template rendering, parsing, sanitization)
- **Integration tests**: Existing IT-KA-715 covers Phase 1→3 propagation wiring

### 5.3 Pass/Fail Criteria

**PASS**: All UT-KA-725-* and UT-KA-724-* tests pass, 0 regressions, >=80% coverage

**FAIL**: Any new test fails, any existing test regresses, coverage below 80%

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/prompt/history.go` | `BuildRemediationHistorySection` (warning text) | ~6 (lines 101-115) |
| `internal/kubernautagent/prompt/builder.go` | `formatPhase1Assessment`, `RenderWorkflowSelection` | ~20 |
| `internal/kubernautagent/parser/parser.go` | `applyRCAFields` | ~5 |
| `internal/kubernautagent/parser/schema.go` | `RCAResultSchema` | ~3 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-200 | Investigation outcome routing — inconclusive escalation | P0 | Unit | UT-KA-725-001 | Pending |
| BR-HAPI-200 | Repeated ineffective — correct escalation guidance | P0 | Unit | UT-KA-725-002 | Pending |
| BR-HAPI-200 | Phase 3 template inconclusive example JSON | P0 | Unit | UT-KA-725-003 | Pending |
| BR-HAPI-263 | Phase 1 investigation_analysis parsed | P0 | Unit | UT-KA-724-001 | Pending |
| BR-HAPI-263 | Phase 1 analysis rendered in Phase 3 | P0 | Unit | UT-KA-724-002 | Pending |
| BR-HAPI-263 | Backward compat — empty analysis | P0 | Unit | UT-KA-724-003 | Pending |
| BR-HAPI-263 | Schema contains investigation_analysis | P1 | Unit | UT-KA-724-004 | Pending |
| BR-HAPI-263 | Prompt injection sanitization | P0 | Unit | UT-KA-724-SEC-001 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

#### Issue #725: Prompt Engineering Fixes

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-725-001` | All-zero effectiveness warning references `investigation_outcome: inconclusive`, not `needs_human_review` | Pending |
| `UT-KA-725-002` | Repeated ineffective warning references `investigation_outcome: inconclusive`, not `needs_human_review` | Pending |
| `UT-KA-725-003` | Phase 3 template contains inconclusive example JSON with `investigation_outcome: inconclusive` | Pending |

#### Issue #724: Investigation Analysis Field

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-724-001` | Parser extracts `investigation_analysis` from nested LLM response into InvestigationResult | Pending |
| `UT-KA-724-002` | Phase 3 prompt includes "Phase 1 Investigation Analysis" section when field populated | Pending |
| `UT-KA-724-003` | Phase 3 prompt omits investigation analysis section when field empty (backward compat) | Pending |
| `UT-KA-724-004` | RCAResultSchema contains `investigation_analysis` property | Pending |
| `UT-KA-724-SEC-001` | investigation_analysis content is sanitized against prompt injection | Pending |

---

## 9. Test Cases

### UT-KA-725-001: All-zero effectiveness warning references investigation_outcome

**BR**: BR-HAPI-200
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: Remediation history with all-zero effectiveness for action_type/signal_type
2. **When**: `BuildRemediationHistorySection` is called with escalation threshold met
3. **Then**: Warning contains `investigation_outcome` to `inconclusive` and does NOT contain `needs_human_review`

### UT-KA-725-002: Repeated ineffective warning references investigation_outcome

**BR**: BR-HAPI-200
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: Remediation history with non-zero but recurring effectiveness
2. **When**: `BuildRemediationHistorySection` is called
3. **Then**: Warning contains `investigation_outcome` to `inconclusive` and does NOT contain `needs_human_review`

### UT-KA-725-003: Phase 3 template contains inconclusive example JSON

**BR**: BR-HAPI-200
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: Standard WorkflowSelectionInput
2. **When**: `RenderWorkflowSelection` is called
3. **Then**: Output contains `"investigation_outcome": "inconclusive"` in example JSON block

### UT-KA-724-001: Parser extracts investigation_analysis

**BR**: BR-HAPI-263
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: LLM JSON with `root_cause_analysis.investigation_analysis` populated
2. **When**: `Parse` is called
3. **Then**: `result.InvestigationAnalysis` equals the input value

### UT-KA-724-002: Phase 3 renders investigation analysis section

**BR**: BR-HAPI-263
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: WorkflowSelectionInput with Phase1.InvestigationAnalysis populated
2. **When**: `RenderWorkflowSelection` is called
3. **Then**: Output contains "Phase 1 Investigation Analysis" section with the content

### UT-KA-724-003: Empty investigation_analysis preserves backward compat

**BR**: BR-HAPI-263
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: WorkflowSelectionInput with Phase1.InvestigationAnalysis empty
2. **When**: `RenderWorkflowSelection` is called
3. **Then**: Output does NOT contain "Phase 1 Investigation Analysis"

### UT-KA-724-004: RCAResultSchema contains investigation_analysis

**BR**: BR-HAPI-263
**Priority**: P1
**File**: `test/unit/kubernautagent/parser/schema_phase_separation_test.go` (or parser_test.go)

**Test Steps**:
1. **Given**: `RCAResultSchema()` output
2. **When**: Unmarshalled as JSON
3. **Then**: `root_cause_analysis.properties` contains `investigation_analysis`

### UT-KA-724-SEC-001: Prompt injection sanitization

**BR**: BR-HAPI-263
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/adversarial_prompt_test.go`

**Test Steps**:
1. **Given**: Phase1Data with InvestigationAnalysis containing injection strings (e.g., "ignore all previous instructions")
2. **When**: `RenderWorkflowSelection` is called
3. **Then**: Output does NOT contain the raw injection strings (sanitized)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None needed (pure logic tests)
- **Location**: `test/unit/kubernautagent/prompt/`, `test/unit/kubernautagent/parser/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1-3**: #725 prompt engineering fixes (RED → GREEN → REFACTOR)
2. **Phase 4-6**: #724 investigation_analysis field (RED → GREEN → REFACTOR)

#725 is done first because the inconclusive example in the Phase 3 template is needed for #724 testing.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/724/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/prompt/`, `test/unit/kubernautagent/parser/` | Ginkgo BDD test files |

---

## 13. Execution

```bash
# Unit tests — prompt
go test ./test/unit/kubernautagent/prompt/... -ginkgo.v

# Unit tests — parser
go test ./test/unit/kubernautagent/parser/... -ginkgo.v

# Specific tests
go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="UT-KA-725"
go test ./test/unit/kubernautagent/parser/... -ginkgo.focus="UT-KA-724"
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | Due diligence confirmed no existing tests assert on the `needs_human_review` portion of history.go warnings |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
