# Test Plan: Anti-Confirmation-Bias Investigation Guardrails

**Feature**: Add generic guardrails to HAPI investigation prompt preventing premature LLM conclusions
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: cherry-pick to `main` via `release/v1.1.0-rc2`

**Authority**:
- [BR-HAPI-214](../../services/stateless/holmesgpt-api/BUSINESS_REQUIREMENTS.md): Anti-Confirmation-Bias Investigation Guardrails
- [Issue #462](https://github.com/jordigilh/kubernaut/issues/462): Forward RR.spec.signalAnnotations to HAPI + anti-confirmation-bias guardrail

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)

---

## 1. Scope

### In Scope

- `holmesgpt-api/src/extensions/incident/prompt_builder.py`: `create_incident_investigation_prompt()` -- add `## Investigation Guardrails` section and pre-conclusion gates in Outcome A and Outcome D

### Out of Scope

- Part A (SignalDescription pipeline) -- covered in [TEST_PLAN_PART_A.md](TEST_PLAN_PART_A.md), ships in v1.2
- LLM behavioral validation (whether guardrails improve investigation quality) -- requires production evaluation
- Signal-type-specific guardrails -- intentionally generic per user decision

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Two generic guardrails only (no signal-type-specific rules) | Avoids prompt bloat; data-driven refinements can be added later if generic guardrails prove insufficient |
| Guardrails apply to both reactive and proactive modes | Confirmation bias risk is mode-independent |
| Pre-conclusion gates in Outcome A and Outcome D only | These are the two outcomes where premature conclusions are most dangerous (dismissing real issues) |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `create_incident_investigation_prompt()` guardrail code paths (pure logic: text generation, mode branching)
- **Integration**: Tier skip -- see rationale below

### 2-Tier Minimum

This change is entirely within prompt text generation (pure logic: string concatenation with conditionals). The function takes a `dict` and returns a `str`. No I/O, no K8s, no HTTP, no database. The existing integration tests for prompt sanitization (`llm_integration.py`) will exercise the guardrail text through the sanitization pipeline without new integration scenarios.

### Business Outcome Quality Bar

Tests validate **business outcomes**: "when an investigation prompt is generated, guardrails preventing premature conclusions are present" -- not "the string concatenation works."

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | `create_incident_investigation_prompt()` (guardrail section ~10 lines, pre-conclusion gates ~4 lines) | ~14 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| N/A -- pure text generation | -- | -- |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-214 | Guardrail section present in reactive investigation prompt | P0 | Unit | UT-HAPI-214-001 | Pending |
| BR-HAPI-214 | Guardrail section present in proactive investigation prompt | P0 | Unit | UT-HAPI-214-002 | Pending |
| BR-HAPI-214 | Pre-conclusion gate in Outcome A (resolved) | P0 | Unit | UT-HAPI-214-003 | Pending |
| BR-HAPI-214 | Pre-conclusion gate in Outcome D (not actionable) | P0 | Unit | UT-HAPI-214-004 | Pending |
| BR-HAPI-214 | Guardrail includes exhaustive verification mandate | P1 | Unit | UT-HAPI-214-005 | Pending |
| BR-HAPI-214 | Guardrail includes contradicting evidence search | P1 | Unit | UT-HAPI-214-006 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `HAPI` (HolmesGPT API)
- **BR_NUMBER**: `214`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `create_incident_investigation_prompt()` guardrail section -- target >=80% code path coverage (reactive, proactive, Outcome A gate, Outcome D gate)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-214-001` | Reactive investigation prompt includes guardrails against premature conclusions | Pending |
| `UT-HAPI-214-002` | Proactive investigation prompt includes guardrails against premature conclusions | Pending |
| `UT-HAPI-214-003` | Outcome A (resolved) includes pre-conclusion verification gate | Pending |
| `UT-HAPI-214-004` | Outcome D (not actionable) includes pre-conclusion verification gate | Pending |
| `UT-HAPI-214-005` | Guardrail text mandates exhaustive resource verification | Pending |
| `UT-HAPI-214-006` | Guardrail text requires contradicting evidence search | Pending |

### Tier Skip Rationale

- **Integration**: `create_incident_investigation_prompt()` is pure text generation (takes `dict`, returns `str`). No I/O, no K8s API, no HTTP. The existing integration path (`llm_integration.py` -> `sanitize_for_llm(create_incident_investigation_prompt(...))`) is not modified and exercises the guardrail text through sanitization. No new integration scenarios needed.
- **E2E**: Prompt text changes do not require E2E validation. The E2E pipeline tests exercise the full investigation flow; re-running them after this change validates no regression.

---

## 6. Test Cases (Detail)

### UT-HAPI-214-001: Guardrail section present in reactive prompt

**BR**: BR-HAPI-214
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

**Given**: A standard reactive incident request (`signal_mode` absent or `"reactive"`)
**When**: `create_incident_investigation_prompt()` is called
**Then**: The returned prompt contains an `## Investigation Guardrails` section

**Acceptance Criteria**:
- Prompt contains the string `"## Investigation Guardrails"`
- Section appears between Phase 1 investigation instructions and Phase 2 RCA

---

### UT-HAPI-214-002: Guardrail section present in proactive prompt

**BR**: BR-HAPI-214
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

**Given**: A proactive incident request (`signal_mode = "proactive"`)
**When**: `create_incident_investigation_prompt()` is called
**Then**: The returned prompt contains an `## Investigation Guardrails` section

**Acceptance Criteria**:
- Prompt contains the string `"## Investigation Guardrails"`
- Guardrail content is identical for reactive and proactive modes (generic, not mode-specific)

---

### UT-HAPI-214-003: Pre-conclusion gate in Outcome A (resolved)

**BR**: BR-HAPI-214
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

**Given**: A standard incident request
**When**: `create_incident_investigation_prompt()` is called
**Then**: The Outcome A (Problem Self-Resolved) section includes a pre-conclusion verification gate

**Acceptance Criteria**:
- The text near `"Outcome A"` or `"Problem Self-Resolved"` contains a reference to the Investigation Guardrails
- The gate text requires confirming guardrail compliance before classifying as resolved

---

### UT-HAPI-214-004: Pre-conclusion gate in Outcome D (not actionable)

**BR**: BR-HAPI-214
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

**Given**: A standard incident request
**When**: `create_incident_investigation_prompt()` is called
**Then**: The Outcome D (Alert Not Actionable) section includes a pre-conclusion verification gate

**Acceptance Criteria**:
- The text near `"Outcome D"` or `"Alert Not Actionable"` contains a reference to the Investigation Guardrails
- The gate text requires confirming guardrail compliance before classifying as not actionable

---

### UT-HAPI-214-005: Guardrail includes exhaustive verification mandate

**BR**: BR-HAPI-214
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

**Given**: A standard incident request
**When**: `create_incident_investigation_prompt()` is called
**Then**: The guardrail section includes text mandating inspection of ALL mentioned resources

**Acceptance Criteria**:
- Guardrail text contains language about inspecting all resources/data sources mentioned in the signal
- Text conveys that partial evidence (e.g., some pods healthy) does not rule out other resources being the cause

---

### UT-HAPI-214-006: Guardrail includes contradicting evidence search

**BR**: BR-HAPI-214
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

**Given**: A standard incident request
**When**: `create_incident_investigation_prompt()` is called
**Then**: The guardrail section includes text requiring search for contradicting evidence

**Acceptance Criteria**:
- Guardrail text contains language about searching for evidence that contradicts the hypothesis
- Text conveys that the LLM must look for counter-evidence before finalizing conclusions

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest (Python, mandatory for holmesgpt-api)
- **Mocks**: None needed -- `create_incident_investigation_prompt()` is a pure function
- **Test Helpers**: Minimal `dict` payloads shaped like `IncidentRequest` (same pattern as `test_prompt_generation_adr041.py`)
- **Location**: `holmesgpt-api/tests/unit/test_investigation_guardrails.py`

---

## 8. Execution

```bash
# All HAPI unit tests
cd holmesgpt-api && python -m pytest tests/unit/ -v

# Specific guardrail tests
cd holmesgpt-api && python -m pytest tests/unit/test_investigation_guardrails.py -v

# By test ID pattern
cd holmesgpt-api && python -m pytest tests/unit/test_investigation_guardrails.py -k "HAPI_214" -v
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan -- 6 unit test scenarios for anti-confirmation-bias guardrails |
