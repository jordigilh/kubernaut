# Test Plan: HAPI LLM Response Parsing Failure (#624)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-624-v1.1
**Feature**: Fix Pattern 2B nested JSON truncation, audit event normalization, and Phase 1 structured output
**Version**: 1.1
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc4`

---

## 1. Introduction

### 1.1 Purpose

Validates three fixes: (A) Pattern 2B regex in `result_parser.py` correctly extracts nested JSON objects using balanced brace counting, (B) early-exit result dicts are normalized to include all required `IncidentResponseData` fields before audit event creation, and (C) Phase 1 RCA calls request structured output via `PHASE1_SECTIONS` to eliminate Sonnet 4.6 retry loops.

### 1.2 Objectives

1. **Pattern 2B correctness**: Nested JSON in `# root_cause_analysis` and `# selected_workflow` sections is extracted completely (not truncated by non-greedy regex)
2. **Audit normalization**: All result dicts passed to `create_aiagent_response_complete_event` contain required fields (`incident_id`, `analysis`, `confidence`, `timestamp`)
3. **Graceful degradation**: Unbalanced braces produce empty dict, not crash
4. **Phase 1 structured output**: `PHASE1_SECTIONS` enables JSON mode for Phase 1 RCA, eliminating retry loops caused by free-text markdown parsing failures on Sonnet 4.6

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `pytest tests/unit/test_pattern_2b_nested_json.py tests/unit/test_ensure_response_shape.py` |
| Integration test pass rate | 100% | `pytest tests/integration/test_audit_normalization_integration.py` |

---

## 2. References

### 2.1 Authority

- BR-HAPI-200: Investigation outcomes
- BR-AUDIT-005: AI Provider Data audit
- Issue #372: Structured output retry
- Issue #624: HAPI LLM escalation response fails Pydantic validation

### 2.2 Cross-References

- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Circular import between `result_parser` and `llm_integration` | Import error | High | All | Extract `extract_balanced_json` into shared `json_utils.py` module |
| R2 | `IncidentResponseData.from_dict()` expects camelCase | Wrong key format | Medium | UT-HAPI-624-006/007 | `populate_by_name: True` in Pydantic config accepts both snake_case and camelCase |
| R3 | Existing Pattern 2B tests break | Regression | Low | Existing tests | Balanced extraction is a superset of non-greedy for simple cases |

---

## 4. Scope

### 4.1 Features to be Tested

- **Pattern 2B extraction** (`result_parser.py`): `# root_cause_analysis` and `# selected_workflow` sections with nested JSON
- **Response shape normalizer** (`result_parser.py`): `ensure_incident_response_shape()` backfills missing required fields
- **Audit event wiring** (`endpoint.py`): Normalizer called before `create_aiagent_response_complete_event`

### 4.2 Features Added in v1.1

- **Phase 1 structured output** (`prompt_builder.py`): `PHASE1_SECTIONS` constant defining Phase 1 expected keys
- **Phase 1 request wiring** (`llm_integration.py`): `InvestigateRequest` passes `sections=PHASE1_SECTIONS`
- **Phase 1 section consumption** (`llm_integration.py`): Phase 1 result processing reads from `sections` dict

### 4.3 Features Not to be Tested

- **Pattern 1 (```json``` block)**: Not affected by this fix
- **Pattern 2A (complete JSON dict)**: Not affected by this fix
- **LLM prompt instructions**: Out of scope for code fix

### 4.4 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Shared `json_utils.py` module | Avoids circular import between `llm_integration` and `result_parser` |
| Snake_case keys in normalizer | Matches early-exit dict convention; Pydantic `populate_by_name` accepts both |
| `PHASE1_SECTIONS` follows `PHASE3_SECTIONS` pattern | Proven approach; SDK enables structured output when sections are provided |
| Phase 1 sections include `can_recover` | Phase 1 propagates `can_recover` to final response (BR-HAPI-200) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-200 | Pattern 2B nested RCA extraction | P0 | Unit | UT-HAPI-624-001 | Pending |
| BR-HAPI-200 | Pattern 2B nested workflow extraction | P0 | Unit | UT-HAPI-624-002 | Pending |
| BR-HAPI-200 | Graceful fallback on unbalanced braces | P0 | Unit | UT-HAPI-624-003 | Pending |
| BR-AUDIT-005 | Normalizer fills missing fields | P0 | Unit | UT-HAPI-624-004 | Pending |
| BR-AUDIT-005 | Normalizer preserves existing fields | P0 | Unit | UT-HAPI-624-005 | Pending |
| BR-AUDIT-005 | Enrichment failure dict validated | P0 | Unit | UT-HAPI-624-006 | Pending |
| BR-AUDIT-005 | Phase-1 exhaustion dict validated | P0 | Unit | UT-HAPI-624-007 | Pending |
| BR-HAPI-200 | Phase 1 sections contain required keys | P0 | Unit | UT-HAPI-624-009 | Pending |
| BR-HAPI-200 | Phase 1 sections do not use SDK defaults | P0 | Unit | UT-HAPI-624-010 | Pending |
| BR-HAPI-200 | Phase 1 sections enable structured output in SDK | P0 | Unit | UT-HAPI-624-011 | Pending |
| BR-HAPI-200 | Full parse flow with nested Pattern 2B | P0 | Integration | IT-HAPI-624-001 | Pending |
| BR-AUDIT-005 | Audit event creation with normalized dict | P0 | Integration | IT-HAPI-624-002 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `kubernaut-agent/tests/unit/test_pattern_2b_nested_json.py`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-624-001` | Pattern 2B extracts nested RCA JSON correctly | Pending |
| `UT-HAPI-624-002` | Pattern 2B extracts nested workflow JSON correctly | Pending |
| `UT-HAPI-624-003` | Unbalanced braces return empty dict gracefully | Pending |

**File**: `kubernaut-agent/tests/unit/test_ensure_response_shape.py`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-624-004` | Missing fields filled with safe defaults | Pending |
| `UT-HAPI-624-005` | Existing fields preserved unchanged | Pending |
| `UT-HAPI-624-006` | Enrichment failure dict passes IncidentResponseData validation | Pending |
| `UT-HAPI-624-007` | Phase-1 exhaustion dict passes IncidentResponseData validation | Pending |

**File**: `kubernaut-agent/tests/unit/test_phase1_structured_output.py`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-624-009` | `PHASE1_SECTIONS` contains keys consumed by Phase 1 result processing (`root_cause_analysis`, `confidence`, `investigation_outcome`, `can_recover`) | Pending |
| `UT-HAPI-624-010` | `PHASE1_SECTIONS` does NOT contain SDK DEFAULT_SECTIONS keys (ensures custom sections override) | Pending |
| `UT-HAPI-624-011` | SDK `get_output_format_for_investigation(PHASE1_SECTIONS)` produces valid `json_schema` response format (proves structured output is enabled) | Pending |

### Tier 2: Integration Tests

**File**: `kubernaut-agent/tests/integration/test_audit_normalization_integration.py`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-624-001` | Full parse_and_validate with nested Pattern 2B produces valid response | Pending |
| `IT-HAPI-624-002` | Audit event creation succeeds with normalized early-exit dict | Pending |

---

## 10. Environmental Needs

- **Framework**: pytest
- **Location**: `kubernaut-agent/tests/unit/`, `kubernaut-agent/tests/integration/`
- **Configuration**: `kubernaut-agent/pytest.ini`

---

## 11. Execution

```bash
cd kubernaut-agent
pytest tests/unit/test_pattern_2b_nested_json.py tests/unit/test_ensure_response_shape.py -v
pytest tests/integration/test_audit_normalization_integration.py -v
```

---

## 12. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
| 1.1 | 2026-04-04 | Added Phase 1 structured output scenarios (UT-HAPI-624-009/010/011) per demo team finding on Sonnet 4.6 |
