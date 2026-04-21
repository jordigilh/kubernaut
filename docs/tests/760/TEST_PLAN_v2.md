# Test Plan: Split Workflow Submit Tools + Parse-Level Retry (#760 v2)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-760-v2
**Feature**: Split submit_result into submit_result_with_workflow / submit_result_no_workflow for workflow selection, with parse-level retry for LLMs that return text instead of tool calls
**Version**: 1.0
**Created**: 2026-04-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/760-workflow-submit-split`
**Supersedes**: TP-760-v1 (typed parse errors — necessary but insufficient; see §1.1)

---

## 1. Introduction

### 1.1 Purpose

TP-760-v1 introduced typed parse errors (`ErrNoJSON`, `ErrNoRecognizedFields`) and the
investigator classified `ErrNoJSON` (free text) as `no_matching_workflows`. However, the
demo team reported the fix was insufficient in `v1.3.0-rc8`: the LLM was returning
*valid JSON with unrecognized fields* (`ErrNoRecognizedFields`) or markdown text
containing false-positive JSON after a `todo_write` tool call. The `ErrNoRecognizedFields`
path fell through to a generic `llm_parsing_error`.

This v2 plan validates a two-pronged fix:
1. **Split submit tools**: Replace the single `submit_result` tool in the workflow
   selection phase with two intent-specific tools (`submit_result_with_workflow`,
   `submit_result_no_workflow`), making the LLM's intent unambiguous through tool
   selection itself.
2. **Parse-level retry**: When the LLM's workflow selection response cannot be parsed
   (any parse error), trigger up to 2 correction retries with only the two submit tools
   enabled. If retries are exhausted, classify as `no_matching_workflows` (not
   `llm_parsing_error`).

### 1.2 Objectives

1. **Split tools**: Workflow selection phase exposes `submit_result_with_workflow` and `submit_result_no_workflow`; RCA phase retains single `submit_result`
2. **Sentinel detection**: `runLLMLoop` recognizes all three sentinel tool names (`submit_result`, `submit_result_with_workflow`, `submit_result_no_workflow`)
3. **No-workflow path**: `submit_result_no_workflow` produces `no_matching_workflows` classification
4. **Parse-level retry**: Text/unrecognized responses trigger up to 2 correction retries before fallback
5. **Fallback**: Exhausted retries classify as `no_matching_workflows`, not `llm_parsing_error`
6. **Backward compat**: RCA phase, catalog self-correction, and existing happy-path workflows unaffected

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on parser/schema.go, errors.go |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on investigator.go |
| Backward compatibility | 0 regressions | All existing investigator and parser tests pass |

---

## 2. References

### 2.1 Authority

- BR-HAPI-197: Human review reason classification (no_matching_workflows vs llm_parsing_error)
- DD-HAPI-019: Framework Isolation Pattern (LLM client interface)
- Issue #760: KA misclassifies intentional workflow decline as llm_parsing_error
- TP-760-v1: Original typed parse errors fix (superseded by this plan)

### 2.2 Cross-References

- PR #761: Original free-text decline fix (ErrNoJSON path only)
- `internal/kubernautagent/investigator/investigator.go`: runWorkflowSelection, runLLMLoop
- `internal/kubernautagent/parser/schema.go`: Tool schemas
- `internal/kubernautagent/parser/errors.go`: Typed parse errors (from v1)
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | LLM ignores both submit tools on retry | Fallback to no_matching_workflows instead of structured result | Medium | IT-KA-760-009 | Exhaustion fallback classifies correctly; correction message includes concrete examples |
| R2 | OutputSchema conflicts with split tool Parameters | LLM confused between text and tool schemas | Low | IT-KA-760-006 | OutputSchema is text-mode hint; tool Parameters are authoritative for tool calls |
| R3 | Existing IT-KA-760-002 (garbage JSON) behavior change | Test expects NOT no_matching_workflows, but retries now exhaust to no_matching_workflows | Medium | IT-KA-760-002 | Redesign test to validate retry behavior before fallback |
| R4 | Phase tool count assertions break | WorkflowDiscovery tool count changes from 5 to 6 | Low | UT-KA-433-015 | Update assertion |
| R5 | Correction retry messages contaminate conversation history | Retry adds messages that affect subsequent catalog self-correction | Low | IT-KA-760-011 | Parse retry happens before catalog validation; message state is isolated |
| R6 | Sentinel name collision with existing tool names | `submit_result_with_workflow` could shadow a registry tool | Very Low | UT-KA-760-013 | Sentinel names are reserved; registry tools cannot use `submit_result` prefix |

### 3.1 Risk-to-Test Traceability

- **R1**: IT-KA-760-009 validates exhaustion fallback
- **R3**: IT-KA-760-002 updated to expect retry + fallback behavior
- **R4**: UT-KA-433-015 assertion updated
- **R5**: IT-KA-760-011 validates catalog self-correction still works with split tools

---

## 4. Scope

### 4.1 Features to be Tested

- **Tool definitions** (`investigator.go:toolDefinitionsForPhase`): Split submit tools for workflow selection phase
- **Sentinel detection** (`investigator.go:runLLMLoop`): Recognize `submit_result_with_workflow` and `submit_result_no_workflow`
- **No-workflow schema** (`parser/schema.go`): New `NoWorkflowResultSchema()`
- **No-workflow classification** (`investigator.go:runWorkflowSelection`): `submit_result_no_workflow` → `no_matching_workflows`
- **Parse-level retry** (`investigator.go:runWorkflowSelection`): Correction loop for text/unrecognized responses
- **Fallback classification** (`investigator.go:runWorkflowSelection`): Exhausted retries → `no_matching_workflows`

### 4.2 Features Not to be Tested

- **RCA phase**: Retains single `submit_result`, no changes
- **Catalog self-correction**: Existing mechanism unchanged (tested for non-regression only)
- **LLM client/transport layer**: No changes to `Chat()` or `StructuredOutputTransport`
- **Parser logic**: No changes to `Parse()`, `parseLLMFormat()`, `applyOutcomeRouting()`

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Two submit tools instead of one | Tool name = intent; LLMs are better at tool selection than optional-field JSON |
| Single `Chat()` call per retry (not `runLLMLoop`) | LLM has already decided; only formatting is wrong. No re-investigation needed |
| Only submit tools available during retry | Prevents LLM from using investigation tools during correction |
| Exhaustion → `no_matching_workflows` | If LLM can't format after 3 attempts, its intent (decline) is clear from text |
| `submit_result_no_workflow` requires only `root_cause_analysis` + optional `reasoning` | The tool name itself signals "no workflow"; only need to know why |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `schema.go` (`NoWorkflowResultSchema`, `WithWorkflowResultSchema`)
- **Integration**: >=80% of `investigator.go` changes (split tools, retry, fallback, sentinel detection)

### 5.2 Two-Tier Minimum

- **Unit tests**: Schema validation, tool definition composition
- **Integration tests**: Full investigator flow with mock LLM exercising all paths

### 5.3 Pass/Fail Criteria

**PASS**: All tests pass, existing tests unbroken, >=80% coverage per tier.
**FAIL**: Any P0 test fails, regressions in existing tests, coverage below 80%.

### 5.4 Suspension & Resumption Criteria

**Suspend**: Build broken, existing investigator tests fail.
**Resume**: Build fixed, all existing tests green.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/parser/schema.go` | `NoWorkflowResultSchema()`, `WithWorkflowResultSchema()` | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `toolDefinitionsForPhase`, `runLLMLoop`, `runWorkflowSelection` | ~80 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/760-workflow-submit-split` HEAD | Branched from main post-rc8 |
| Dependency: TP-760-v1 | Merged in main | Typed parse errors (`ErrNoJSON`, `ErrNoRecognizedFields`) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-197 | `submit_result_no_workflow` → `no_matching_workflows` | P0 | Integration | IT-KA-760-004 | Pending |
| BR-HAPI-197 | `submit_result_with_workflow` → normal workflow path | P0 | Integration | IT-KA-760-005 | Pending |
| BR-HAPI-197 | Text response triggers parse-level retry | P0 | Integration | IT-KA-760-006 | Pending |
| BR-HAPI-197 | Unrecognized JSON triggers retry | P0 | Integration | IT-KA-760-007 | Pending |
| BR-HAPI-197 | Retry succeeds on 2nd attempt | P0 | Integration | IT-KA-760-008 | Pending |
| BR-HAPI-197 | Retries exhausted → `no_matching_workflows` | P0 | Integration | IT-KA-760-009 | Pending |
| BR-HAPI-197 | RCA phase still uses single `submit_result` | P0 | Integration | IT-KA-760-010 | Pending |
| BR-HAPI-197 | Catalog self-correction still works with split tools | P1 | Integration | IT-KA-760-011 | Pending |
| BR-HAPI-197 | `NoWorkflowResultSchema` is valid JSON Schema | P1 | Unit | UT-KA-760-010 | Pending |
| BR-HAPI-197 | `WithWorkflowResultSchema` is valid JSON Schema | P1 | Unit | UT-KA-760-011 | Pending |
| BR-HAPI-197 | WorkflowDiscovery phase exposes 2 submit tools (not bare `submit_result`) | P1 | Unit | UT-KA-760-012 | Pending |
| BR-HAPI-197 | WorkflowDiscovery phase does NOT include bare `submit_result` | P1 | Unit | UT-KA-760-013 | Pending |
| BR-HAPI-197 | RCA phase tool list is unchanged | P1 | Unit | UT-KA-760-014 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `parser/schema.go` — schema factory functions

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-760-010` | `NoWorkflowResultSchema` returns valid JSON with `root_cause_analysis` and `reasoning` fields | Pending |
| `UT-KA-760-011` | `WithWorkflowResultSchema` returns valid JSON with `workflow_id`, `root_cause_analysis`, and `confidence` fields | Pending |
| `UT-KA-760-012` | `toolDefinitionsForPhase(WorkflowDiscovery)` includes `submit_result_with_workflow` and `submit_result_no_workflow` | Pending |
| `UT-KA-760-013` | `toolDefinitionsForPhase(WorkflowDiscovery)` does NOT include bare `submit_result` | Pending |
| `UT-KA-760-014` | `toolDefinitionsForPhase(RCA)` includes `submit_result` (unchanged) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `investigator.go` — split tools, sentinel detection, retry loop, fallback

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-760-004` | LLM calls `submit_result_no_workflow` → `no_matching_workflows`, RCA preserved | Pending |
| `IT-KA-760-005` | LLM calls `submit_result_with_workflow` → `workflow_id` parsed, catalog validation proceeds | Pending |
| `IT-KA-760-006` | LLM returns free text → correction retry sent with only submit tools → LLM uses `submit_result_no_workflow` on retry → `no_matching_workflows` | Pending |
| `IT-KA-760-007` | LLM returns unrecognized JSON → correction retry → LLM uses `submit_result_no_workflow` → `no_matching_workflows` | Pending |
| `IT-KA-760-008` | LLM returns text on 1st attempt, uses `submit_result_with_workflow` on 2nd → workflow parsed | Pending |
| `IT-KA-760-009` | LLM returns text on all 3 attempts → exhaustion → `no_matching_workflows` (not `llm_parsing_error`) | Pending |
| `IT-KA-760-010` | RCA phase: LLM returns text content → parsed as RCA summary (unaffected by split) | Pending |
| `IT-KA-760-011` | LLM calls `submit_result_with_workflow` with invalid workflow → catalog self-correction still works | Pending |

### Tier Skip Rationale

- **E2E**: Requires Kind cluster + live LLM. Deferred to manual validation with demo team.

---

## 9. Test Cases

### IT-KA-760-004: submit_result_no_workflow → no_matching_workflows

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_decline_test.go`

**Preconditions**:
- Mock LLM returns RCA via text, then calls `submit_result_no_workflow`

**Test Steps**:
1. **Given**: Mock LLM response 1 returns valid RCA JSON; response 2 returns a tool call to `submit_result_no_workflow` with `{"root_cause_analysis": {"summary": "ResourceQuota exhausted"}, "reasoning": "No workflow handles namespace quota adjustments"}`
2. **When**: `Investigate()` is called
3. **Then**: Result has `HumanReviewReason == "no_matching_workflows"`, `HumanReviewNeeded == true`, RCA summary preserved

**Expected Results**:
1. `result.HumanReviewNeeded` is true
2. `result.HumanReviewReason` equals `"no_matching_workflows"`
3. `result.RCASummary` contains "ResourceQuota"

---

### IT-KA-760-005: submit_result_with_workflow → workflow parsed

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_decline_test.go`

**Preconditions**:
- Mock LLM returns RCA, then calls `submit_result_with_workflow`

**Test Steps**:
1. **Given**: Mock LLM response 1 returns valid RCA JSON; response 2 returns a tool call to `submit_result_with_workflow` with `{"root_cause_analysis": {"summary": "OOMKilled"}, "selected_workflow": {"workflow_id": "oom-increase-memory", "confidence": 0.95}}`
2. **When**: `Investigate()` is called
3. **Then**: Result has `WorkflowID == "oom-increase-memory"`, `Confidence ~= 0.95`

**Expected Results**:
1. `result.WorkflowID` equals `"oom-increase-memory"`
2. `result.Confidence` is approximately 0.95
3. `result.HumanReviewNeeded` is false

---

### IT-KA-760-006: Text → retry → submit_result_no_workflow

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_decline_test.go`

**Preconditions**:
- Mock LLM returns RCA, then returns free text on 1st workflow attempt, then calls `submit_result_no_workflow` on retry

**Test Steps**:
1. **Given**: Mock LLM response 1 = valid RCA; response 2 = free text "No workflow handles this"; response 3 = tool call `submit_result_no_workflow` with `{"root_cause_analysis": {"summary": "Quota exhausted"}, "reasoning": "No matching workflow"}`
2. **When**: `Investigate()` is called
3. **Then**: Result has `HumanReviewReason == "no_matching_workflows"`

**Expected Results**:
1. `result.HumanReviewNeeded` is true
2. `result.HumanReviewReason` equals `"no_matching_workflows"`
3. Mock LLM receives at least 3 calls (RCA + initial workflow + retry)

---

### IT-KA-760-009: All retries exhausted → no_matching_workflows

**BR**: BR-HAPI-197
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_decline_test.go`

**Preconditions**:
- Mock LLM returns RCA, then returns free text for all 3 workflow selection attempts (initial + 2 retries)

**Test Steps**:
1. **Given**: Mock LLM response 1 = valid RCA; responses 2, 3, 4 = free text (no tool calls, no JSON)
2. **When**: `Investigate()` is called
3. **Then**: Result has `HumanReviewReason == "no_matching_workflows"` (NOT `llm_parsing_error`), `HumanReviewNeeded == true`

**Expected Results**:
1. `result.HumanReviewNeeded` is true
2. `result.HumanReviewReason` equals `"no_matching_workflows"`
3. `result.Reason` mentions retry exhaustion

---

## 10. Environmental Needs

### 10.1 Unit Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None (pure schema validation)
- **Location**: `test/unit/kubernautagent/parser/`, `test/unit/kubernautagent/investigator/`

### 10.2 Integration Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `mockLLMClient` (external LLM dependency)
- **Location**: `test/integration/kubernautagent/investigator/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| TP-760-v1 | Code | Merged | Typed parse errors (`ErrNoJSON`, `ErrNoRecognizedFields`) used by retry detection | N/A — already on main |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write failing tests for split tools, sentinel detection, retry loop, fallback
2. **Phase 2 (TDD GREEN)**: Implement split tools, sentinel detection, `NoWorkflowResultSchema`, `WithWorkflowResultSchema`, retry loop
3. **Phase 3 (TDD REFACTOR)**: Clean up, audit, update existing tests
4. **Checkpoint**: Adversarial audit, verify all paths, build/lint pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/760/TEST_PLAN_v2.md` | Strategy and test design |
| Schema additions | `internal/kubernautagent/parser/schema.go` | `NoWorkflowResultSchema`, `WithWorkflowResultSchema` |
| Unit test additions | `test/unit/kubernautagent/parser/schema_test.go` | Schema validation tests |
| Integration test additions | `test/integration/kubernautagent/investigator/investigator_decline_test.go` | Decline, retry, fallback flow tests |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/parser/... -ginkgo.v
go test ./test/unit/kubernautagent/investigator/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v

# Specific test by ID
go test ./test/integration/kubernautagent/investigator/... -ginkgo.focus="IT-KA-760"

# Full build
go build ./...
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| IT-KA-760-001 (`investigator_decline_test.go:72`) | Free text → `no_matching_workflows` (direct) | Free text → retry → exhaustion → `no_matching_workflows` | Retry mechanism intercepts before direct classification; mock must supply 3 text responses (initial + 2 retries) |
| IT-KA-760-002 (`investigator_decline_test.go:98`) | Garbage JSON → `HumanReviewReason NOT no_matching_workflows` | Garbage JSON → retries exhausted → `HumanReviewReason == no_matching_workflows` | Retry mechanism changes behavior: all parse failures get retried, exhaustion falls back to `no_matching_workflows` |
| IT-KA-686-002 (`investigator_test.go:454`) | Workflow discovery phase includes `submit_result` | Workflow discovery phase includes `submit_result_with_workflow` and `submit_result_no_workflow` | `submit_result` replaced by split tools in workflow discovery |
| UT-KA-433-015 (`investigator_test.go:54`) | WorkflowDiscovery has 4 tools | WorkflowDiscovery has 4 tools (unchanged count — registry tools stay the same; submit tools are appended separately) | Verify tool count expectation still holds or update |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-21 | Initial v2 test plan — split submit tools + parse-level retry |
