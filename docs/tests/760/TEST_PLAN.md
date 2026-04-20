# Test Plan: Typed Parse Errors for Workflow Selection Decline Classification

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-760-v1
**Feature**: Typed parse errors distinguish LLM workflow decline from genuine parsing failures
**Version**: 1.0
**Created**: 2026-04-20
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/760-workflow-decline-misclassification`

---

## 1. Introduction

### 1.1 Purpose

When the LLM intentionally declines to select a workflow during the workflow selection
phase (e.g., no catalog workflow addresses ResourceQuota exhaustion), the KA parser
returns a generic `fmt.Errorf`, and the investigator's error path misclassifies it as
`llm_parsing_error` instead of `no_matching_workflows`. This test plan validates that
typed parse errors allow the investigator to distinguish deliberate declines from
genuine parsing failures, achieving HAPI v1.2.1 state machine parity:
"No workflow + RCA present -> `no_matching_workflows`."

### 1.2 Objectives

1. **Typed error taxonomy**: `Parse()` returns `ErrNoJSON`, `ErrNoRecognizedFields`, or
   `ErrEmptyContent` instead of `fmt.Errorf`, preserving the `error` interface.
2. **Investigator dispatch**: `runWorkflowSelection` uses `errors.As` to classify
   `ErrNoJSON` as `no_matching_workflows` (HAPI parity) and other errors as
   `llm_parsing_error`.
3. **Backward compatibility**: All existing parser tests pass without modification
   (except `UT-KA-746-004` which asserts on error text substring — preserved by
   `ErrNoRecognizedFields.Error()`).
4. **Integration coverage**: End-to-end investigator path validates audit trail and
   `HumanReviewReason` for the workflow decline scenario.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/parser/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `parser/errors.go`, `parser/parser.go` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on `investigator/investigator.go` |
| Backward compatibility | 0 regressions | All existing parser and investigator tests pass |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-197.2: `needs_human_review` field semantics — "No Workflows Matched" trigger
- BR-HAPI-200.6: Decision tree — `selected_workflow` null -> `no_matching_workflows`
- Issue #760: KA misclassifies intentional workflow decline as `llm_parsing_error`
- Issue #746: Original `llm_parsing_error` fix for RCA phase (predecessor)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- HAPI v1.2.1 `result_parser.py` lines 483-510 (external, referenced in docs/tests/746/)
- HAPI state machine: `docs/tests/433/TP-433-ADV.md` lines 520-529

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Typed errors break existing tests that check `err.Error()` | Test regression | Low | UT-KA-760-007 | `Error()` methods preserve original substrings; only 1 test (`UT-KA-746-004`) checks error text |
| R2 | `ErrNoJSON` during RCA classified incorrectly | False `no_matching_workflows` in RCA phase | Low | UT-KA-760-005 | Only `runWorkflowSelection` uses `errors.As`; `runRCA` is unchanged (treats parse errors as summary text) |
| R3 | LLM produces garbage text misclassified as decline | False `no_matching_workflows` for genuine garbage | Medium | UT-KA-760-003, IT-KA-760-002 | `ErrNoJSON` is only interpreted as decline in workflow selection context (after successful RCA); garbage during RCA flows through existing path |
| R4 | Self-correction closure in `runWorkflowSelection` affected by typed errors | Validator loop breaks | Low | IT-KA-760-003 | Closure returns `(result, error)` to `SelfCorrect`; validator checks `Validate()` not parse error type |
| R5 | `mapHumanReviewReason` still overrides explicit `HumanReviewReason` | `llm_parsing_error` via substring match | Low | UT-KA-760-006 | Fix sets `HumanReviewReason` directly; `mapHumanReviewReason` only consults `Reason` when `HumanReviewReason` is empty |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-KA-760-007 validates backward compatibility of error text
- **R2**: UT-KA-760-005 validates RCA path is unaffected
- **R3**: UT-KA-760-003 + IT-KA-760-002 validate garbage vs decline classification
- **R4**: IT-KA-760-003 validates self-correction path still works
- **R5**: UT-KA-760-006 validates `HumanReviewReason` is set directly, not via mapper

---

## 4. Scope

### 4.1 Features to be Tested

- **Parser typed errors** (`internal/kubernautagent/parser/errors.go` — new file): Three
  error types (`ErrEmptyContent`, `ErrNoJSON`, `ErrNoRecognizedFields`) with `Error()`,
  `Unwrap()`, and `errors.As` support
- **Parser error returns** (`internal/kubernautagent/parser/parser.go`): Replace
  `fmt.Errorf` returns with typed errors at lines 43, 75, 77, 317
- **Investigator dispatch** (`internal/kubernautagent/investigator/investigator.go`):
  `runWorkflowSelection` parse-error path uses `errors.As` to classify errors

### 4.2 Features Not to be Tested

- **Prompt template changes**: Not needed; prompt already covers Outcome C
- **Schema changes**: `InvestigationResultSchema` already makes `selected_workflow` optional
- **`mapHumanReviewReason`**: No changes needed; fix sets `HumanReviewReason` directly
- **Validator `SelfCorrect`**: Separate error classification (`llm_parsing_error` for
  validation exhaustion) — already correct and covered by `IT-KA-433-AP-004`

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Typed errors in parser, classification in investigator | Parser is phase-agnostic; investigator has phase context. Idiomatic Go `errors.As` pattern |
| Three error types, not one generic | Distinguishes garbage JSON from free text from empty content, per user direction |
| `ErrNoJSON` -> `no_matching_workflows` only in workflow selection | RCA path treats all parse errors as summary text (existing behavior, HAPI parity) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `parser/errors.go` (new), `parser/parser.go` (modified error returns)
- **Integration**: >=80% of `investigator/investigator.go` workflow-selection error path

### 5.2 Two-Tier Minimum

- **Unit tests**: Parser error types, `errors.As` behavior, `Error()` text compatibility
- **Integration tests**: Full investigator flow with mock LLM returning free text / garbage
  during workflow selection

### 5.3 Business Outcome Quality Bar

Tests validate that operators see `no_matching_workflows` (not `llm_parsing_error`) when
the LLM deliberately declines workflow selection, and `llm_parsing_error` for genuine
parsing failures.

### 5.4 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier coverage >=80%
4. No regressions in existing parser/investigator test suites
5. `UT-KA-746-004` continues to pass with `ContainSubstring("no recognized fields")`

**FAIL**:
1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Any existing parser or investigator test regresses

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken or existing parser tests fail after error type changes.
**Resume**: Build fixed, all existing tests green.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/parser/errors.go` (new) | `ErrEmptyContent`, `ErrNoJSON`, `ErrNoRecognizedFields`, `Error()`, `Unwrap()` | ~40 |
| `internal/kubernautagent/parser/parser.go` | `Parse()` error return sites (lines 43, 75, 77, 317) | ~4 changed |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `runWorkflowSelection` error dispatch (lines 331-339) | ~12 changed |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/760-workflow-decline-misclassification` HEAD | Branched from main post-rc6 |
| Dependency: #746 fix | Merged in main | Parser `applyOutcomeRouting` and guard relaxation |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-197.2 | No Workflows Matched -> `needs_human_review=true`, `reason=no_matching_workflows` | P0 | Unit | UT-KA-760-001 | Pending |
| BR-HAPI-197.2 | LLM Parsing Error -> `reason=llm_parsing_error` for garbage JSON | P0 | Unit | UT-KA-760-002 | Pending |
| BR-HAPI-197.2 | Empty content -> `reason=llm_parsing_error` | P0 | Unit | UT-KA-760-003 | Pending |
| BR-HAPI-197.2 | `errors.As` correctly identifies each typed error | P0 | Unit | UT-KA-760-004 | Pending |
| BR-HAPI-197.2 | RCA path unaffected by typed errors | P0 | Unit | UT-KA-760-005 | Pending |
| BR-HAPI-197.2 | Investigator sets `HumanReviewReason` directly (bypasses mapper) | P0 | Unit | UT-KA-760-006 | Pending |
| BR-HAPI-197.2 | Backward compat: `ErrNoRecognizedFields.Error()` contains "no recognized fields" | P0 | Unit | UT-KA-760-007 | Pending |
| BR-HAPI-197.2 | Free text decline -> `no_matching_workflows` in full investigator flow | P0 | Integration | IT-KA-760-001 | Pending |
| BR-HAPI-197.2 | Garbage JSON -> `llm_parsing_error` in full investigator flow | P0 | Integration | IT-KA-760-002 | Pending |
| BR-HAPI-197.2 | Workflow selection with catalog validation still works | P1 | Integration | IT-KA-760-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-760-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `parser/errors.go`, `parser/parser.go` error return sites

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-760-001` | `Parse("")` returns `ErrEmptyContent` identifiable via `errors.As` | Pending |
| `UT-KA-760-002` | `Parse("free text with no JSON")` returns `ErrNoJSON` identifiable via `errors.As` | Pending |
| `UT-KA-760-003` | `Parse('{"foo":"bar"}')` returns `ErrNoRecognizedFields` identifiable via `errors.As` | Pending |
| `UT-KA-760-004` | `ErrNoJSON` is NOT `ErrNoRecognizedFields` (distinct types, no cross-match) | Pending |
| `UT-KA-760-005` | `ErrEmptyContent.Error()` contains "empty", `ErrNoJSON.Error()` contains "no JSON", `ErrNoRecognizedFields.Error()` contains "no recognized fields" | Pending |
| `UT-KA-760-006` | `ErrNoJSON.Content` preserves the raw LLM text for investigator logging | Pending |
| `UT-KA-760-007` | Existing `UT-KA-746-004` assertion `ContainSubstring("no recognized fields")` passes | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `investigator/investigator.go` `runWorkflowSelection` error path

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-760-001` | LLM returns free text during workflow selection -> result has `HumanReviewReason="no_matching_workflows"` and `HumanReviewNeeded=true` | Pending |
| `IT-KA-760-002` | LLM returns garbage JSON during workflow selection -> result has `HumanReviewReason` that maps to `llm_parsing_error` (generic parse failure path) | Pending |
| `IT-KA-760-003` | Workflow selection with catalog validator still self-corrects and returns valid workflow | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. The fix is confined to parser error types and investigator dispatch.
  E2E validation requires a real LLM declining workflow selection, which is
  non-deterministic. The unit + integration tiers provide sufficient behavioral assurance.

---

## 9. Test Cases

### UT-KA-760-001: Parse empty content returns ErrEmptyContent

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/typed_errors_test.go`

**Test Steps**:
1. **Given**: An empty string `""`
2. **When**: `Parse("")` is called
3. **Then**: Error is non-nil and `errors.As(err, &ErrEmptyContent{})` returns true

**Expected Results**:
1. `err != nil`
2. `errors.As(err, &parser.ErrEmptyContent{})` is true
3. `result` is nil

---

### UT-KA-760-002: Parse free text returns ErrNoJSON

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/typed_errors_test.go`

**Test Steps**:
1. **Given**: Free text: `"After reviewing the 21 registered workflows, none can adjust namespace quotas."`
2. **When**: `Parse(content)` is called
3. **Then**: Error is non-nil and `errors.As(err, &ErrNoJSON{})` returns true; `ErrNoJSON.Content` contains the original text

**Expected Results**:
1. `err != nil`
2. `errors.As(err, &noJSON)` is true
3. `noJSON.Content` equals the original input
4. `result` is nil

---

### UT-KA-760-003: Parse garbage JSON returns ErrNoRecognizedFields

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/typed_errors_test.go`

**Test Steps**:
1. **Given**: Valid JSON with no recognized fields: `{"foo": "bar", "baz": 42}`
2. **When**: `Parse(content)` is called
3. **Then**: Error is non-nil and `errors.As(err, &ErrNoRecognizedFields{})` returns true

**Expected Results**:
1. `err != nil`
2. `errors.As(err, &parser.ErrNoRecognizedFields{})` is true
3. `result` is nil

---

### UT-KA-760-004: Typed errors are distinct (no cross-match)

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/typed_errors_test.go`

**Test Steps**:
1. **Given**: An `ErrNoJSON` instance
2. **When**: `errors.As(err, &ErrNoRecognizedFields{})` is called
3. **Then**: Returns false (types are distinct)

**Expected Results**:
1. `ErrNoJSON` does NOT match `ErrNoRecognizedFields`
2. `ErrNoJSON` does NOT match `ErrEmptyContent`
3. `ErrEmptyContent` does NOT match `ErrNoJSON`

---

### UT-KA-760-005: Error() text preserves backward-compatible substrings

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/typed_errors_test.go`

**Test Steps**:
1. **Given**: Each typed error instance
2. **When**: `Error()` is called
3. **Then**: Text contains expected substrings for backward compatibility

**Expected Results**:
1. `ErrEmptyContent{}.Error()` contains "empty"
2. `ErrNoJSON{}.Error()` contains "no JSON found"
3. `ErrNoRecognizedFields{}.Error()` contains "no recognized fields"

---

### UT-KA-760-006: ErrNoJSON.Content preserves raw LLM text

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/typed_errors_test.go`

**Test Steps**:
1. **Given**: Free text input `"No workflow can address quota exhaustion"`
2. **When**: `Parse(content)` is called
3. **Then**: `ErrNoJSON.Content` equals the original input text

**Expected Results**:
1. `noJSON.Content` matches input verbatim

---

### UT-KA-760-007: Existing UT-KA-746-004 compatibility

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/parser_test.go` (existing)

**Test Steps**:
1. **Given**: Existing test `UT-KA-746-004` with `{"foo": "bar", "baz": 42}`
2. **When**: Test suite runs
3. **Then**: `Expect(err.Error()).To(ContainSubstring("no recognized fields"))` passes

**Expected Results**:
1. No modification needed to existing test
2. Assertion passes with typed error `Error()` method

---

### IT-KA-760-001: Free text decline -> no_matching_workflows in investigator

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go`

**Preconditions**:
- Mock LLM returns valid RCA JSON for Phase 1
- Mock LLM returns free text (no JSON, no tool calls) for Phase 3

**Test Steps**:
1. **Given**: Mock LLM configured with: response 1 = `{"rca_summary":"ResourceQuota exhausted","confidence":0.95}`, response 2 = `"After reviewing workflows, none can adjust namespace quotas."`
2. **When**: `inv.Investigate(ctx, signal)` is called
3. **Then**: Result has `HumanReviewNeeded=true`, `HumanReviewReason="no_matching_workflows"`, `RCASummary` preserved from Phase 1

**Expected Results**:
1. `result.HumanReviewNeeded` is true
2. `result.HumanReviewReason` equals `"no_matching_workflows"`
3. `result.RCASummary` equals `"ResourceQuota exhausted"`
4. `result.Reason` contains the free text or parse error context

---

### IT-KA-760-002: Garbage JSON -> llm_parsing_error path in investigator

**BR**: BR-HAPI-197.2
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go`

**Preconditions**:
- Mock LLM returns valid RCA JSON for Phase 1
- Mock LLM returns garbage JSON for Phase 3

**Test Steps**:
1. **Given**: Mock LLM configured with: response 1 = `{"rca_summary":"OOMKilled","confidence":0.9}`, response 2 = `{"foo":"bar","baz":42}`
2. **When**: `inv.Investigate(ctx, signal)` is called
3. **Then**: Result has `HumanReviewNeeded=true` and `Reason` contains "parse" (which maps to `llm_parsing_error` via `mapHumanReviewReason`)

**Expected Results**:
1. `result.HumanReviewNeeded` is true
2. `result.HumanReviewReason` is empty (generic error path, not explicit)
3. `result.Reason` contains "parse" substring

---

### IT-KA-760-003: Catalog validation self-correction still works

**BR**: BR-HAPI-197.2
**Priority**: P1
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go`

**Preconditions**:
- Mock LLM returns valid RCA, then invalid workflow, then valid workflow
- Catalog validator with allowlist

**Test Steps**:
1. **Given**: Mock LLM: response 1 = RCA, response 2 = `{"workflow_id":"unknown","confidence":0.8}`, response 3 = `{"workflow_id":"restart","confidence":0.7}`; validator allows `["restart","scale-up"]`
2. **When**: `inv.Investigate(ctx, signal)` is called
3. **Then**: Result has `WorkflowID="restart"` (self-corrected)

**Expected Results**:
1. `result.WorkflowID` equals `"restart"`
2. `result.HumanReviewNeeded` is false

**Dependencies**: Mirrors existing `IT-KA-433-AP-004` but validates no regression.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (pure parser logic)
- **Location**: `test/unit/kubernautagent/parser/typed_errors_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock LLM client (in-process, same as existing `IT-KA-433-AP-*` tests)
- **Infrastructure**: None (no envtest, no DB)
- **Location**: `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go`

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
| Issue #746 | Code | Merged | `applyOutcomeRouting` and guard relaxation | N/A — already on main |

### 11.2 Execution Order

1. **Phase 1 — TDD RED**: Write all failing tests (UT-KA-760-001 through 007, IT-KA-760-001 through 003)
2. **Checkpoint 1**: Adversarial/security audit on test design
3. **Phase 2 — TDD GREEN**: Implement `parser/errors.go`, modify `parser/parser.go` error returns, modify `investigator.go` dispatch
4. **Checkpoint 2**: Adversarial/security audit on implementation
5. **Phase 3 — TDD REFACTOR**: Extract constants, align patterns, ensure doc parity
6. **Checkpoint 3**: Final adversarial/security audit, build/lint/full test pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/760/TEST_PLAN.md` | Strategy and test design |
| Parser typed errors | `internal/kubernautagent/parser/errors.go` | New error types |
| Unit test suite | `test/unit/kubernautagent/parser/typed_errors_test.go` | Error type tests |
| Integration test additions | `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go` | Investigator flow tests |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/parser/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/parser/... -ginkgo.focus="UT-KA-760"
go test ./test/integration/kubernautagent/investigator/... -ginkgo.focus="IT-KA-760"

# Coverage
go test ./test/unit/kubernautagent/parser/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `UT-KA-746-004` (`parser_test.go:836`) | `Expect(err.Error()).To(ContainSubstring("no recognized fields"))` | None | `ErrNoRecognizedFields.Error()` preserves the substring |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-20 | Initial test plan |
