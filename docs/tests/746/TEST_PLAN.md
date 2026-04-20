# Test Plan: Parser No-Matching-Workflow Misclassification Fix

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-746-v1.0
**Feature**: Fix parser misclassification of "no matching workflow" as `llm_parsing_error`
**Version**: 1.0
**Created**: 2026-04-19
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/746-no-matching-workflow-misclassified`

---

## 1. Introduction

### 1.1 Purpose

Validate that the KA parser correctly classifies LLM responses where no workflow matches as `no_matching_workflows` instead of `llm_parsing_error`, achieving behavioral parity with HAPI v1.2.1's fallback chain (result_parser.py lines 483-510).

### 1.2 Objectives

1. **camelCase RCA alias**: `parseLLMFormat` extracts RCA data from `rootCauseAnalysis` (camelCase) in addition to `root_cause_analysis` (snake_case)
2. **Guard relaxation**: `parseLLMFormat` returns a valid result when JSON is valid with recognizable content but no `rca_summary`/`workflow_id`
3. **No-workflow fallback**: Outcome routing derives `HumanReviewNeeded=true, HumanReviewReason="no_matching_workflows"` when no workflow is selected and no specific outcome overrides
4. **No regressions**: All existing parser tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/parser/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on parser package |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| #746 scenario classified correctly | `no_matching_workflows` | Golden transcript reproduction test |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-197: Human Review Required Flag (`needs_human_review` field definition)
- BR-HAPI-197.2: "No Workflows Matched" as a trigger for `needs_human_review=true`
- BR-HAPI-200: Investigation inconclusive outcome routing
- Issue #746: KA workflow selection parser misclassifies 'no matching workflow' as llm_parsing_error

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- HAPI v1.2.1 `result_parser.py` lines 483-510 (authoritative behavior reference)
- HAPI v1.2.1 `PHASE3_SECTIONS` (prompt_builder.py lines 943-968)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Relaxed guard lets malformed JSON through | Incomplete results downstream | Medium | UT-KA-746-004 | Require at least one recognizable signal (confidence > 0 or RCA present) |
| R2 | camelCase alias conflicts with existing tags | JSON unmarshal ambiguity | Low | UT-KA-746-001 | Go json package allows distinct tags on separate fields |
| R3 | No-workflow fallback fires when it shouldn't | False human escalation | Medium | UT-KA-746-006, UT-KA-746-007 | Only triggers when WorkflowID == "" AND no other outcome matched |
| R4 | Outcome routing interaction with existing inconclusive path | Double HR classification | Low | UT-KA-746-005 | Explicit ordering: existing outcome routing runs first, fallback only fills gaps |

### 3.1 Risk-to-Test Traceability

- R1: UT-KA-746-004 (adversarial: unrecognizable JSON still rejected)
- R2: UT-KA-746-001, UT-KA-746-002 (camelCase alias extraction)
- R3: UT-KA-746-006, UT-KA-746-007 (no false positives on existing outcomes)
- R4: UT-KA-746-005 (inconclusive with RCA still produces no_matching_workflows)

---

## 4. Scope

### 4.1 Features to be Tested

- **`parseLLMFormat`** (`internal/kubernautagent/parser/parser.go`): camelCase RCA alias, guard relaxation
- **`applyOutcomeRouting`** (`internal/kubernautagent/parser/parser.go`): no-workflow fallback
- **`Parse`** (`internal/kubernautagent/parser/parser.go`): end-to-end #746 golden transcript

### 4.2 Features Not to be Tested

- **Schema changes**: Not modifying `investigationResultSchemaJSON` (HAPI parity: `needs_human_review` not in LLM schema)
- **Section-header path**: KA forces `submit_result` structured output; section-header is HAPI legacy
- **Handler/server mapping**: `mapHumanReviewReason` already correctly maps `no_matching_workflows` (line 433-434)
- **AIAnalysis response processor**: Separate service, out of scope

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| HR is parser-derived, not LLM-reported | HAPI v1.2.1 parity: `needs_human_review` not in PHASE3_SECTIONS schema |
| No-workflow fallback in `applyOutcomeRouting` | Matches HAPI's `elif selected_workflow is None` -> `no_matching_workflows` |
| camelCase alias via separate struct field | Follows existing `RemediationTargetAlt` pattern (line 199) |
| Require confidence > 0 as minimum recognizable signal | Prevents truly empty/garbage JSON from passing the guard |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `parseLLMFormat`, `applyOutcomeRouting`, `Parse` (pure logic, no I/O)

### 5.2 Two-Tier Minimum

- **Unit tests**: Cover parser logic, outcome routing, camelCase alias extraction
- **Integration**: Not required -- parser is pure logic with no I/O dependencies

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, per-tier coverage >=80%, no regressions in existing parser tests.

**FAIL**: Any P0 test fails, coverage below 80%, or existing tests regress.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/parser/parser.go` | `parseLLMFormat`, `applyOutcomeRouting`, `Parse` | ~60 changed |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-197.2 | No Workflows Matched -> needs_human_review=true | P0 | Unit | UT-KA-746-003 | Pending |
| BR-HAPI-197.2 | Golden transcript reproduction (#746 scenario) | P0 | Unit | UT-KA-746-008 | Pending |
| BR-HAPI-200 | Parser-derived HR for inconclusive + no workflow | P0 | Unit | UT-KA-746-005 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/parser/parser.go` -- >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-746-001` | camelCase `rootCauseAnalysis` extracted by parseLLMFormat | Pending |
| `UT-KA-746-002` | camelCase RCA with snake_case workflow: both extracted | Pending |
| `UT-KA-746-003` | No workflow selected + confidence > 0 -> no_matching_workflows | Pending |
| `UT-KA-746-004` | Truly unrecognizable JSON (no confidence, no RCA, no workflow) still rejected | Pending |
| `UT-KA-746-005` | investigation_outcome=inconclusive + RCA + no workflow -> no_matching_workflows | Pending |
| `UT-KA-746-006` | Existing: workflow selected -> no false HR escalation | Pending |
| `UT-KA-746-007` | Existing: problem_resolved outcome -> no HR escalation | Pending |
| `UT-KA-746-008` | Golden transcript: exact #746 audit JSON -> no_matching_workflows | Pending |

### Tier Skip Rationale

- **Integration**: Parser is pure logic with no I/O boundaries. All behavior testable at unit tier.
- **E2E**: Would require Kind cluster + LLM. #746 scenario validation deferred to manual Kind test.

---

## 9. Test Cases

### UT-KA-746-001: camelCase rootCauseAnalysis extracted

**BR**: BR-HAPI-197.2
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `rootCauseAnalysis` (camelCase) containing summary, severity, contributing_factors
2. **When**: `Parse()` is called
3. **Then**: `RCASummary`, `Severity`, `ContributingFactors` are populated from camelCase RCA

### UT-KA-746-002: camelCase RCA + snake_case workflow

**BR**: BR-HAPI-197.2
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `rootCauseAnalysis` (camelCase) and `selected_workflow` (snake_case)
2. **When**: `Parse()` is called
3. **Then**: Both RCA and workflow fields are extracted correctly

### UT-KA-746-003: No workflow + confidence -> no_matching_workflows

**BR**: BR-HAPI-197.2
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `root_cause_analysis.summary` and `confidence` but no `selected_workflow`
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded=true`, `HumanReviewReason="no_matching_workflows"`

### UT-KA-746-004: Unrecognizable JSON still rejected

**BR**: BR-HAPI-197.2 (defense-in-depth)
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with only unrecognized keys (`{"foo": "bar"}`)
2. **When**: `Parse()` is called
3. **Then**: Error returned ("no recognized fields")

### UT-KA-746-005: Inconclusive + RCA + no workflow -> no_matching_workflows

**BR**: BR-HAPI-200
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `root_cause_analysis.summary`, `investigation_outcome: "inconclusive"`, no `selected_workflow`
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded=true`, `HumanReviewReason="no_matching_workflows"` (existing behavior preserved)

### UT-KA-746-006: Workflow selected -> no false HR

**BR**: BR-HAPI-197.2 (no regression)
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `root_cause_analysis.summary` and `selected_workflow.workflow_id`
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded=false`, `WorkflowID` populated

### UT-KA-746-007: Problem resolved -> no HR escalation

**BR**: BR-HAPI-200 (no regression)
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: JSON with `root_cause_analysis.summary`, `investigation_outcome: "problem_resolved"`, no workflow
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded=false`, warning contains "Problem self-resolved"

### UT-KA-746-008: Golden transcript -- exact #746 audit JSON

**BR**: BR-HAPI-197.2
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: Exact JSON from #746 audit evidence (camelCase `rootCauseAnalysis`, `needsHumanReview: true`, `confidence: 0.98`, no `selected_workflow`)
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded=true`, `HumanReviewReason="no_matching_workflows"`, `Confidence=0.98`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (pure logic)
- **Location**: `test/unit/kubernautagent/parser/`

---

## 11. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/parser/... -ginkgo.v

# Specific #746 tests
go test ./test/unit/kubernautagent/parser/... -ginkgo.focus="746"

# Coverage
go test ./test/unit/kubernautagent/parser/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 12. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-19 | Initial test plan |
