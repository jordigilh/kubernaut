# Test Plan: HAPI Self-Correction Retry Loop — Structured Output Detection

**Feature**: Fix retry loop treating LLM format failures as valid (no retries triggered)
**Version**: 1.0
**Created**: 2026-03-04
**Author**: Kubernaut AI Team
**Status**: Complete
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-HAPI-002]: Incident Analysis
- [BR-HAPI-197]: needs_human_review field
- [DD-HAPI-002 v1.2]: Workflow Response Validation with self-correction
- [GitHub #372]: Self-correction retry loop treats LLM format failure as valid

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)

---

## 1. Scope

### In Scope

- `result_parser.py`: `parse_and_validate_investigation_result()` — detecting when no structured output was parsed and returning a failed `ValidationResult` instead of `None`
- `prompt_builder.py`: `build_validation_error_feedback()` — format-specific feedback prompt for structured output failures
- Regression coverage for all three legitimate "no workflow" outcomes (A: resolved, B: inconclusive, C: no automated fix)

### Out of Scope

- Full retry loop integration (requires async HolmesGPT SDK mocking; covered by E2E)
- Changes to `llm_integration.py` retry loop logic itself (the `is_valid = validation_result is None or validation_result.is_valid` line works correctly once the parser returns proper `ValidationResult`)
- Prompt engineering changes to the base investigation prompt

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Option A: Return failed `ValidationResult` from parser | Integrates cleanly with existing self-correction architecture; no changes needed to the retry loop itself |
| Distinguish via `json_data is None` | All legitimate outcomes (A/B/C) produce parseable structured output; only format failures leave `json_data` as `None` |
| Format-specific feedback in REFACTOR | Existing generic feedback works but is suboptimal; targeted messaging improves LLM self-correction success rate |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`parse_and_validate_investigation_result` parsing logic, `build_validation_error_feedback` branching)

### 2-Tier Minimum

- **Unit tests**: Catch the parser logic bug and verify all outcome paths (fast, isolated)
- **Integration tier**: Not applicable for this fix — the parser function is pure logic with no I/O; the only external dependency (`data_storage_client`) is passed as `None` for all test scenarios where no workflow is selected

### Business Outcome Quality Bar

Tests validate business outcomes: "Does the system retry when the LLM fails to follow the output format?" and "Does the system correctly accept deliberate no-workflow decisions without unnecessary retries?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/extensions/incident/result_parser.py` | `parse_and_validate_investigation_result` | ~380 |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | `build_validation_error_feedback` | ~50 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-002 | Incident analysis produces structured response | P0 | Unit | UT-HAPI-372-001 | Pass |
| BR-HAPI-002 | Markdown-only response detected as format failure | P0 | Unit | UT-HAPI-372-002 | Pass |
| BR-HAPI-002 | Outcome A (resolved) not affected by fix | P0 | Unit | UT-HAPI-372-003 | Pass |
| BR-HAPI-002 | Outcome B (inconclusive) not affected by fix | P0 | Unit | UT-HAPI-372-004 | Pass |
| BR-HAPI-002 | Outcome C (no automated fix) not affected by fix | P0 | Unit | UT-HAPI-372-005 | Pass |
| BR-HAPI-197 | Valid workflow passes validation unchanged | P0 | Unit | UT-HAPI-372-006 | Pass |
| DD-HAPI-002 | Format failure error is actionable for retry | P1 | Unit | UT-HAPI-372-007 | Pass |
| DD-HAPI-002 | Format-specific feedback prompt for self-correction | P1 | Unit | UT-HAPI-372-008 | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `UT-HAPI-372-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `result_parser.py:parse_and_validate_investigation_result`, `prompt_builder.py:build_validation_error_feedback` (100% of changed code)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-372-001` | Plain-text narrative without JSON markers returns failed ValidationResult so retry loop can prompt LLM to resubmit | RED |
| `UT-HAPI-372-002` | Rich markdown analysis (headers, bullets, code blocks without JSON) returns failed ValidationResult | RED |
| `UT-HAPI-372-003` | Outcome A (problem self-resolved) with structured output returns validation_result=None — no unnecessary retry | GREEN |
| `UT-HAPI-372-004` | Outcome B (investigation inconclusive) with structured output returns validation_result=None — no unnecessary retry | GREEN |
| `UT-HAPI-372-005` | Outcome C (no automated fix, structured JSON, selected_workflow=null) returns validation_result=None — no unnecessary retry | GREEN |
| `UT-HAPI-372-006` | Valid workflow selection with structured JSON passes validation unchanged | GREEN |
| `UT-HAPI-372-007` | Format failure ValidationResult contains actionable error message suitable for LLM feedback | GREEN |
| `UT-HAPI-372-008` | build_validation_error_feedback produces format-specific retry prompt for structured output errors | REFACTORED |

### Tier Skip Rationale

- **Integration**: Skipped — `parse_and_validate_investigation_result` is pure logic; the only dependency (`data_storage_client`) is `None` for all no-workflow scenarios. No I/O boundaries to test.
- **E2E**: Existing E2E tests cover the full pipeline. A new E2E scenario for #372 can be added separately if needed.

---

## 6. Test Cases (Detail)

### UT-HAPI-372-001: Plain-text format failure detected

**BR**: BR-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: LLM returns a plain-text narrative with no JSON markers (no ` ```json``` ` block, no `# section_header` format)
**When**: `parse_and_validate_investigation_result()` is called
**Then**: Returns `validation_result` with `is_valid=False` and error message containing "structured JSON output"

**Acceptance Criteria**:
- `validation_result` is not `None`
- `validation_result.is_valid` is `False`
- `validation_result.errors` contains exactly one error describing the format failure
- `result["warnings"]` contains "No workflows matched" (existing behavior preserved)
- `result["needs_human_review"]` is `True` (existing behavior preserved)

### UT-HAPI-372-002: Rich markdown format failure detected

**BR**: BR-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: LLM returns rich markdown with headers, bullet points, and analysis but no structured JSON
**When**: `parse_and_validate_investigation_result()` is called
**Then**: Returns `validation_result` with `is_valid=False`

**Acceptance Criteria**:
- Same as UT-HAPI-372-001 but with a more complex input to ensure pattern matching robustness

### UT-HAPI-372-003: Outcome A (resolved) not affected

**BR**: BR-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: LLM returns structured output with `investigation_outcome: resolved` and `selected_workflow: null`
**When**: `parse_and_validate_investigation_result()` is called
**Then**: `validation_result` is `None` (no retry needed)

**Acceptance Criteria**:
- `validation_result` is `None`
- `result["warnings"]` contains "Problem self-resolved"
- `result["needs_human_review"]` is `False`

### UT-HAPI-372-004: Outcome B (inconclusive) not affected

**BR**: BR-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: LLM returns structured output with `investigation_outcome: inconclusive` and `selected_workflow: null`
**When**: `parse_and_validate_investigation_result()` is called
**Then**: `validation_result` is `None`

**Acceptance Criteria**:
- `validation_result` is `None`
- `result["warnings"]` contains "Investigation inconclusive"
- `result["needs_human_review"]` is `True`
- `result["human_review_reason"]` is `"investigation_inconclusive"`

### UT-HAPI-372-005: Outcome C (no automated fix) not affected

**BR**: BR-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: LLM returns structured JSON with `selected_workflow: null` and no `investigation_outcome`
**When**: `parse_and_validate_investigation_result()` is called
**Then**: `validation_result` is `None`

**Acceptance Criteria**:
- `validation_result` is `None`
- `result["warnings"]` contains "No workflows matched"
- `result["needs_human_review"]` is `True`
- `result["human_review_reason"]` is `"no_matching_workflows"`

### UT-HAPI-372-006: Valid workflow passes unchanged

**BR**: BR-HAPI-197
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: LLM returns structured JSON with a valid `selected_workflow` that passes catalog validation
**When**: `parse_and_validate_investigation_result()` is called with a mock `data_storage_client`
**Then**: `validation_result` is `None` (cleared after successful validation)

**Acceptance Criteria**:
- `validation_result` is `None`
- `result["selected_workflow"]["workflow_id"]` matches input
- `result["needs_human_review"]` is `False`

### UT-HAPI-372-007: Format failure error is actionable

**BR**: DD-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: Format failure `ValidationResult` from UT-HAPI-372-001
**When**: Error message is examined
**Then**: Contains guidance that can be passed to LLM via `build_validation_error_feedback()`

**Acceptance Criteria**:
- Error message mentions "structured JSON output"
- Error message mentions expected format (` ```json``` ` or `# section_header`)
- Error is a non-empty string suitable for inclusion in retry prompt

### UT-HAPI-372-008: Format-specific feedback prompt

**BR**: DD-HAPI-002
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

**Given**: Validation error list containing the structured output format error
**When**: `build_validation_error_feedback()` is called with this error
**Then**: Produces a feedback prompt that instructs the LLM to use the correct output format

**Acceptance Criteria**:
- Feedback contains format-specific instructions (not generic "check workflow ID" language)
- Feedback mentions ` ```json``` ` code block requirement
- Feedback includes attempt counter

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest (HAPI standard)
- **Mocks**: `unittest.mock.Mock` for `InvestigationResult` (`.analysis` attribute), mock `data_storage_client` for UT-HAPI-372-006
- **Location**: `holmesgpt-api/tests/unit/test_structured_output_retry.py`

---

## 8. Execution

```bash
# All #372 tests
cd holmesgpt-api && python -m pytest tests/unit/test_structured_output_retry.py -v

# Specific test by ID
cd holmesgpt-api && python -m pytest tests/unit/test_structured_output_retry.py -v -k "372_001"

# Full HAPI unit suite (regression check)
cd holmesgpt-api && python -m pytest tests/unit/ -v
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
