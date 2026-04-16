# Test Plan: Parser-Driven Escalation (Issue #700)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-700-v1
**Feature**: Remove LLM-driven `needs_human_review` / `human_review_reason`; derive exclusively from parser/investigator
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/700-parser-driven-escalation`

---

## 1. Introduction

### 1.1 Purpose

Validate that `needs_human_review` and `human_review_reason` are never set by the LLM in any phase. These fields must be derived by the parser/investigator based on `investigation_outcome` and structural signals (RCA present, workflow absent), strictly mirroring HAPI v1.2.1 authoritative behavior.

### 1.2 Objectives

1. **Schema honesty**: `InvestigationResultSchema()` must NOT expose `needs_human_review` or `human_review_reason` to the LLM
2. **Parser derivation**: `applyInvestigationOutcome("inconclusive")` derives the correct HR reason from context: `no_matching_workflows` (RCA + no workflow) vs `investigation_inconclusive` (fallback)
3. **LLM field rejection**: Parser must ignore LLM-provided `needs_human_review` / `human_review_reason` fields
4. **Mock fidelity**: Mock LLM responses must NOT emit `needs_human_review` / `human_review_reason`
5. **HAPI parity**: AA integration tests (authoritative) pass without modification
6. **Defense-in-depth**: `problem_resolved` contradiction override (#301) remains exercisable

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/parser/... ./test/unit/mockllm/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| E2E test pass rate | 100% | `go test ./test/e2e/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on parser package |
| Backward compatibility | 0 regressions | AA IT tests pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-197: `needs_human_review` escalation for no matching workflows
- BR-HAPI-200: Parser-derived outcome routing (not LLM-driven)
- DD-HAPI-002 v1.2: Investigation result parsing specification
- DD-HAPI-006: Enrichment-driven `rca_incomplete` (deferred to follow-up issue)
- Issue #700: Structural issues between HAPI and KA prompts
- Issue #701: Two-phase LLM invocation fix (merged, prerequisite)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Parser derivation rule picks wrong reason due to whitespace-only RCASummary | Incorrect HR reason | Very Low | UT-KA-700-PDE-001 | Go `!= ""` treats whitespace as non-empty; `investigation_inconclusive` fallback is still valid |
| R2 | `problem_resolved` contradiction override becomes unreachable via mock | Dead code accumulates | Known | UT-KA-700-PDE-003 | Dedicated unit test exercises `applyFlatFields` directly with synthetic contradictory inputs |
| R3 | E2E ADV-016 temp relaxation masks real regression | False green in CI | Low | E2E-KA-433-ADV-016 | Comment links to enrichment follow-up issue; restored in same PR |
| R4 | AA IT tests break due to incorrect derivation rule | Release blocker | Medium | Authoritative AA IT tests | Derivation rule verified: `inconclusive` + RCA + no workflow = `no_matching_workflows` |

---

## 4. Scope

### 4.1 Features to be Tested

- **Schema** (`internal/kubernautagent/parser/schema.go`): `InvestigationResultSchema()` must NOT include `needs_human_review` / `human_review_reason`
- **Parser derivation** (`internal/kubernautagent/parser/parser.go`): `applyInvestigationOutcome` context-aware HR reason derivation
- **Parser field rejection** (`internal/kubernautagent/parser/parser.go`): `applyFlatFields` must NOT propagate LLM `needs_human_review`
- **Prompt template** (`internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl`): Must NOT instruct LLM to set HR fields
- **Mock response builder** (`test/services/mock-llm/response/openai.go`): Must NOT emit `needs_human_review` / `human_review_reason`
- **Mock scenarios** (`test/services/mock-llm/scenarios/scenario_mock_keywords.go`): All 3 HR scenarios updated

### 4.2 Features Not to be Tested

- **Enrichment-driven `rca_incomplete`**: Deferred to follow-up issue (BR-HAPI-261/264). Same PR, separate TDD cycle.
- **Self-correction exhaustion** (`UT-KA-433-027`): Unchanged — `SelfCorrect` sets HR independently of LLM fields.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Derive `no_matching_workflows` from `inconclusive` + RCA + no workflow | Matches HAPI behavior; authoritative AA IT tests assert this exact reason |
| Keep `problem_resolved` contradiction override as defense-in-depth | HAPI-authoritative; exercises via direct unit test |
| Temp relax E2E ADV-016 for `rca_incomplete` | Enrichment follow-up restores in same PR |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of parser derivation logic (`applyFlatFields`, `applyInvestigationOutcome`, `extractSections`)
- **Integration**: >=80% of investigator pipeline (phase separation, HR field handling)
- **E2E**: Container contract validation for all 3 affected mock scenarios

### 5.2 Two-Tier Minimum

Every business requirement covered by UT + IT minimum. E2E provides additional regression safety.

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, per-tier coverage >=80%, AA IT tests unmodified and green.
**FAIL**: Any P0 test fails, coverage below 80%, or AA IT test modified.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/parser/schema.go` | `InvestigationResultSchema()` | ~55 |
| `internal/kubernautagent/parser/parser.go` | `applyFlatFields`, `applyInvestigationOutcome`, `extractSections` | ~90 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `Investigate` pipeline | ~200 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-200 | Parser derives HR from investigation_outcome, not LLM | P0 | Unit | UT-KA-700-PDE-001 | Pending |
| BR-HAPI-200 | Parser fallback: investigation_inconclusive when no RCA | P0 | Unit | UT-KA-700-PDE-002 | Pending |
| BR-HAPI-200 | LLM needs_human_review fields ignored by parser | P0 | Unit | UT-KA-433-PRS-009 (rewrite) | Pending |
| BR-HAPI-200 | LLM explicit needs_human_review NOT preserved | P0 | Unit | UT-KA-433-OUT-004 (rewrite) | Pending |
| BR-HAPI-200 | Schema does not expose HR fields to LLM | P0 | Unit | UT-KA-700-002 (flip) | Pending |
| BR-HAPI-200 | Schema validation in parser_test | P1 | Unit | UT-KA-SCHEMA-001 (update) | Pending |
| BR-HAPI-197 | no_matching_workflows derived for inconclusive+RCA+no workflow | P0 | Unit | UT-KA-433-OUT-003 (update) | Pending |
| BR-HAPI-200 | problem_resolved contradiction override | P1 | Unit | UT-KA-700-PDE-003 | Pending |
| BR-HAPI-200 | Mock response does not emit HR fields | P0 | Unit | UT-MOCK-030-003 (update) | Pending |
| BR-HAPI-200 | Pipeline does not abort on RCA HR | P0 | Integration | IT-KA-700-002 (update) | Pending |
| BR-HAPI-200 | E2E rca_incomplete temp relaxation | P1 | E2E | E2E-KA-433-ADV-016 (temp) | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-700-PDE-001 | Parser derives `no_matching_workflows` from `inconclusive` + RCA present + no workflow | Pending |
| UT-KA-700-PDE-002 | Parser derives `investigation_inconclusive` from `inconclusive` when no RCA context | Pending |
| UT-KA-700-PDE-003 | `problem_resolved` contradiction override clears HR via `applyFlatFields` directly | Pending |
| UT-KA-700-002 | Schema does NOT expose `needs_human_review` / `human_review_reason` | Pending |
| UT-KA-SCHEMA-001 | Schema validation: required keys present, HR keys absent | Pending |
| UT-KA-433-PRS-009 | Parser ignores LLM-set `needs_human_review`; HR derived from outcome only | Pending |
| UT-KA-433-OUT-003 | `inconclusive` + RCA summary + no workflow = `no_matching_workflows` | Pending |
| UT-KA-433-OUT-004 | LLM `needs_human_review=true` with workflow must NOT be preserved | Pending |
| UT-MOCK-030-003 | Mock response must NOT contain `needs_human_review` / `human_review_reason` | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-700-002 | Pipeline proceeds to workflow selection when RCA has HR fields (HR stripped) | Pending |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-KA-433-ADV-016 | `rca_incomplete` scenario: temp expect `needs_human_review=false` (enrichment follow-up restores) | Pending |

---

## 9. Test Cases

### UT-KA-700-PDE-001: Parser derives no_matching_workflows

**BR**: BR-HAPI-197, BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/adversarial_parser_test.go`

**Test Steps**:
1. **Given**: JSON with `rca_summary` non-empty, no `workflow_id`, `investigation_outcome: "inconclusive"`
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded == true`, `HumanReviewReason == "no_matching_workflows"`

### UT-KA-700-PDE-002: Parser derives investigation_inconclusive fallback

**BR**: BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/adversarial_parser_test.go`

**Test Steps**:
1. **Given**: JSON with `workflow_id` present, `investigation_outcome: "inconclusive"`, no `rca_summary`
2. **When**: `Parse()` is called
3. **Then**: `HumanReviewNeeded == true`, `HumanReviewReason == "investigation_inconclusive"`

### UT-KA-700-PDE-003: problem_resolved contradiction override

**BR**: BR-HAPI-200 (#301)
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/adversarial_parser_test.go`

**Test Steps**:
1. **Given**: `flatLLMFields` with `InvestigationOutcome: "problem_resolved"`, `NeedsHumanReview: true`, `HumanReviewReason: "contradictory_signals"`
2. **When**: `applyFlatFields()` is called on a result
3. **Then**: `HumanReviewNeeded == false`, `HumanReviewReason == ""`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None (parser is pure logic)
- **Location**: `test/unit/kubernautagent/parser/`, `test/unit/mockllm/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Mock LLM client (in-process)
- **Location**: `test/integration/kubernautagent/investigator/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Kind cluster with Mock LLM
- **Location**: `test/e2e/kubernautagent/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (TDD RED)**: Write/update all failing tests
2. **Checkpoint 1**: Adversarial audit — verify tests fail for the right reason
3. **Phase 2 (TDD GREEN)**: Implement production changes
4. **Checkpoint 2**: All tests pass + regression check
5. **Phase 3 (TDD REFACTOR)**: Clean up dead code, update comments
6. **Checkpoint 3**: Final regression gate

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-KA-700-002 (`schema_phase_separation_test.go:78-100`) | Schema HAS `needs_human_review` | Flip: assert ABSENT | Schema honesty |
| UT-KA-SCHEMA-001 (`parser_test.go:387-408`) | `HaveKey("needs_human_review")` | Remove assertion | Schema honesty |
| UT-KA-433-PRS-009 (`adversarial_parser_test.go:185-199`) | LLM HR fields extracted | Rewrite: parser ignores LLM fields | LLM field rejection |
| UT-KA-433-OUT-003 (`adversarial_parser_test.go:233-246`) | Expects `investigation_inconclusive` | Change to `no_matching_workflows` | New derivation rule |
| UT-KA-433-OUT-004 (`adversarial_parser_test.go:248-263`) | LLM HR preserved | Rewrite: LLM HR NOT preserved | LLM field rejection |
| UT-MOCK-030-003 (`response_test.go:136-152`) | Response contains HR fields | Assert HR fields absent | Mock fidelity |
| IT-KA-700-002 (`investigator_phase_separation_test.go:217-241`) | rcaSubmitArgs has HR JSON | Remove HR from fixture | Pipeline behavior |
| E2E-KA-433-ADV-016 (`adversarial_parity_e2e_test.go:308-324`) | `needs_human_review=true` | Temp: expect `false` | Enrichment deferred |

---

## 13. Execution

```bash
# Unit tests (parser + mock)
go test ./test/unit/kubernautagent/parser/... -ginkgo.v
go test ./test/unit/mockllm/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v

# E2E tests
go test ./test/e2e/kubernautagent/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/parser/... -ginkgo.focus="UT-KA-700-PDE"

# Coverage
go test ./test/unit/kubernautagent/parser/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
