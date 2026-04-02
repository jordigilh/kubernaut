# Test Plan: HAPI Audit Trace Correctness (#600 + Blast Radius)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-600-v1
**Feature**: Fix tool_call audit serialization bug + verify full HAPI audit trace correctness
**Version**: 1.0
**Created**: 2026-04-02
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

Issue #600 revealed that `audit_llm_response_and_tools` in `investigation_helpers.py` reads non-existent attributes (`name`, `arguments`) on `ToolCallResult`, causing all 76+ `aiagent.llm.tool_call` audit events to record `tool_name: "unknown"` and `tool_arguments: {}`. This test plan fixes that bug and expands the blast radius to verify that **all 7 HAPI audit event types** serialize their payloads correctly — proving no similar attribute-mismatch bugs exist elsewhere in the audit pipeline.

### 1.2 Objectives

1. **Bug fix validation**: `tool_name` and `tool_arguments` are correctly extracted from `ToolCallResult.tool_name` and `ToolCallResult.result.params` respectively
2. **Blast radius coverage**: All 7 audit event factories (`create_llm_request_event`, `create_llm_response_event`, `create_tool_call_event`, `create_validation_attempt_event`, `create_aiagent_response_complete_event`, `create_aiagent_response_failed_event`, `create_enrichment_completed_event`, `create_enrichment_failed_event`) produce correct payload field values — not just correct structure
3. **Caller-side fidelity**: The callers that transform SDK objects into audit arguments (e.g., `getattr` in `investigation_helpers.py`, `dict.get()` in `llm_integration.py`) pass the correct values to the factories
4. **Null/edge-case safety**: All audit paths handle None, empty, and missing values without data loss

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `pytest tests/unit/test_tool_call_audit_serialization.py -v` |
| Bug reproduction | Confirmed | UT-HAPI-600-001/002 assert buggy getattr returns 'unknown'/{} |
| Audit event coverage | 7/7 event types | All factory functions have value-correctness tests |
| Edge case coverage | 4+ edge cases | None params, empty tool_calls, missing fields |
| Backward compatibility | 0 regressions | Existing `test_audit_event_structure.py` passes unchanged |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-AUDIT-005**: Workflow Selection Audit Trail
- **ADR-032 §1**: Audit is MANDATORY — no silent data loss
- **ADR-034**: Unified Audit Table Design
- **ADR-038**: Asynchronous Buffered Audit Trace Ingestion
- **DD-AUDIT-005**: Hybrid Provider Data Capture
- Issue #600: v1.2 Bug: Audit tool_call events always record tool_name as 'unknown'
- Issue #442: SOC2 compliance: failed investigations must have audit trail
- Issue #533: SOC2 audit event gaps in Phase 2 enrichment flow

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing structural tests: `tests/unit/test_audit_event_structure.py` (11 tests — envelope + field presence)
- Existing integration tests: `tests/integration/test_llm_metrics_integration.py` (patches `audit_llm_response_and_tools`)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `ToolCallResult` field names change in HolmesGPT SDK update | tool_name silently regresses to 'unknown' again | Low | UT-HAPI-600-001 | Tests assert both buggy path and correct path explicitly |
| R2 | `result.params` is None on some tool types (e.g., parameter-less tools) | tool_arguments recorded as None instead of {} | Medium | UT-HAPI-600-004 | Explicit None-to-empty-dict fallback tested |
| R3 | Circular import prevents direct import of `audit_llm_response_and_tools` in tests | Test cannot call production code | High | All UT-HAPI-600 | Tests validate the getattr expressions and factory outputs directly; circular import documented as tech debt |
| R4 | Other audit callers (enrichment, response_complete) use `dict.get()` with wrong keys | Silent data loss in other event types | Medium | UT-HAPI-600-008 through 011 | Blast radius tests cover all 7 event type call sites |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by UT-HAPI-600-001 (asserts `getattr(tc, 'name', 'unknown') == 'unknown'` to prove bug)
- **R2**: Mitigated by UT-HAPI-600-004 (explicit None→{} fallback)
- **R3**: Documented in Section 4.3 Design Decisions
- **R4**: Mitigated by UT-HAPI-600-008 through UT-HAPI-600-011 (enrichment + response_complete/failed)

---

## 4. Scope

### 4.1 Features to be Tested

- **Tool call serialization** (`src/extensions/investigation_helpers.py:155-164`): The `getattr` calls that extract `tool_name` and `tool_arguments` from `ToolCallResult` — **root cause of #600**
- **Audit event factories** (`src/audit/events.py`): All 8 factory functions — verifying **payload values**, not just presence
- **Enrichment audit callers** (`src/extensions/incident/llm_integration.py:865-898`): `dict.get()` calls for `root_owner`, `affected_resource` fields
- **Response complete/failed callers** (`src/extensions/incident/endpoint.py:75-103`): `data.get()` calls for incident_id, remediation_id

### 4.2 Features Not to be Tested

- **BufferedAuditStore** (`src/audit/buffered_store.py`): Async batching/flushing — separate concern, has own tests
- **DataStorage HTTP API** (`src/clients/datastorage/`): Generated OpenAPI client — tested at integration level
- **ADR-034 envelope structure**: Already covered by `test_audit_event_structure.py` (11 existing tests)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test getattr expressions directly on real `ToolCallResult` objects | Avoids circular import (R3) while proving the bug and the fix with real SDK types |
| Use `create_tool_call_event` factory directly in tests | Factory is the boundary — if correct values go in, correct audit comes out |
| Access `event.event_data.actual_instance.<field>` for assertions | `AuditEventRequestEventData` is an OpenAPI discriminated union; `.actual_instance` unwraps to the specific payload model |
| Cover all 7 event types (not just tool_call) | Blast radius: same class of bug (wrong field name on SDK/domain objects) could exist in other callers |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in `src/audit/events.py` (8 factory functions) and `src/extensions/investigation_helpers.py` (tool_call serialization loop)
- **Integration**: Deferred — existing `test_llm_metrics_integration.py` covers the end-to-end audit pipeline flow; #600 is a pure logic bug testable at unit level
- **E2E**: Deferred — the audit pipeline E2E requires DataStorage + HAPI deployment; the fix is a 2-line attribute name change

### 5.2 Two-Tier Minimum

Unit tests cover the bug and blast radius. Integration tier is not applicable because:
1. The bug is a pure attribute name mismatch (no I/O, no wiring)
2. The existing integration test (`test_llm_metrics_integration.py`) patches `audit_llm_response_and_tools` entirely, so it cannot catch this class of bug
3. A future integration test could validate audit events end-to-end through DataStorage queries, but that is out of scope for a 2-line attribute fix

### 5.3 Business Outcome Quality Bar

Each test answers: "Does the operator see the correct tool name / arguments / owner / error in the audit trail?" — not "is the factory function called?"

### 5.4 Pass/Fail Criteria

**PASS**:
1. All 11+ tests pass (0 failures)
2. UT-HAPI-600-001 proves `getattr(tc, 'name', 'unknown')` returns `'unknown'` (bug confirmed)
3. After fix, all tests pass with correct field values
4. Existing `test_audit_event_structure.py` passes unchanged (0 regressions)

**FAIL**:
1. Any P0 test fails
2. Any audit event factory produces wrong field values
3. Existing audit structure tests regress

### 5.5 Suspension & Resumption Criteria

**Suspend**: Circular import in `investigation_helpers.py` cannot be worked around (R3)
**Resume**: Workaround found or tech debt resolved

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `src/extensions/investigation_helpers.py` | `audit_llm_response_and_tools` (lines 155-164 tool_call loop) | ~10 |
| `src/audit/events.py` | `create_tool_call_event`, `create_llm_request_event`, `create_llm_response_event`, `create_validation_attempt_event`, `create_aiagent_response_complete_event`, `create_aiagent_response_failed_event`, `create_enrichment_completed_event`, `create_enrichment_failed_event` | ~460 |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc3` HEAD | Branch from main (v1.2.0-rc2) |
| HolmesGPT SDK | `dependencies/holmesgpt` vendored | `ToolCallResult.tool_name`, `StructuredToolResult.params` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-005 | tool_call events must record actual tool_name | P0 | Unit | UT-HAPI-600-001 | Pending |
| BR-AUDIT-005 | tool_call events must record actual tool_arguments | P0 | Unit | UT-HAPI-600-002 | Pending |
| BR-AUDIT-005 | Multiple tool calls all captured correctly | P0 | Unit | UT-HAPI-600-003 | Pending |
| BR-AUDIT-005 | Null result.params defaults to {} | P0 | Unit | UT-HAPI-600-004 | Pending |
| BR-AUDIT-005 | LLM request audit captures model + toolsets | P1 | Unit | UT-HAPI-600-005 | Pending |
| BR-AUDIT-005 | LLM response audit captures analysis + token count | P1 | Unit | UT-HAPI-600-006 | Pending |
| BR-AUDIT-005 | Validation attempt captures workflow_id + errors | P1 | Unit | UT-HAPI-600-007 | Pending |
| BR-AUDIT-005 | Response complete audit captures full response_data | P1 | Unit | UT-HAPI-600-008 | Pending |
| BR-AUDIT-005 | Response failed audit captures error + phase | P1 | Unit | UT-HAPI-600-009 | Pending |
| BR-AUDIT-005 | Enrichment completed captures root_owner + labels | P1 | Unit | UT-HAPI-600-010 | Pending |
| BR-AUDIT-005 | Enrichment failed captures reason + affected_resource | P1 | Unit | UT-HAPI-600-011 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-HAPI-600-{SEQUENCE}` (all unit tier, HAPI service)

### Tier 1: Unit Tests

**Testable code scope**: `src/audit/events.py` (8 factories), `src/extensions/investigation_helpers.py` (tool_call loop), >=80% coverage target

#### Group A: #600 Bug Fix (P0)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-600-001` | Operator queries audit trail and sees actual tool name (e.g., "kubectl_describe"), not "unknown" | Pending |
| `UT-HAPI-600-002` | Operator queries audit trail and sees actual tool arguments (e.g., {"namespace": "default"}), not {} | Pending |
| `UT-HAPI-600-003` | When investigation uses 3 tools, audit trail contains 3 distinct tool_call events with correct names and arguments | Pending |
| `UT-HAPI-600-004` | Tools with no parameters (result.params=None) audit as tool_arguments={}, not None | Pending |

#### Group B: Blast Radius — All Event Types (P1)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-600-005` | LLM request audit event records the actual model name and enabled toolset names | Pending |
| `UT-HAPI-600-006` | LLM response audit event records analysis presence, length, preview, token count, and tool_call_count | Pending |
| `UT-HAPI-600-007` | Validation attempt audit event records workflow_id, attempt number, errors list, and human_review_reason | Pending |
| `UT-HAPI-600-008` | Response complete audit event records full response_data dict and token breakdown | Pending |
| `UT-HAPI-600-009` | Response failed audit event records error_message, phase, and duration_seconds | Pending |
| `UT-HAPI-600-010` | Enrichment completed audit event records root_owner kind/name/namespace, chain length, detected_labels, and history flag | Pending |
| `UT-HAPI-600-011` | Enrichment failed audit event records failure reason, detail, and affected_resource kind/name/namespace | Pending |

### Tier Skip Rationale

- **Integration**: The bug is a pure attribute name mismatch in Python code (no I/O boundary). Existing integration test (`test_llm_metrics_integration.py`) patches the function entirely. A new integration test would need a running DataStorage to query stored events — disproportionate effort for a 2-line attribute fix.
- **E2E**: Same rationale. The E2E fullpipeline tests do exercise the full audit chain, but adding a specific E2E for attribute names is not cost-effective.

---

## 9. Test Cases

### UT-HAPI-600-001: tool_name extracted from ToolCallResult.tool_name

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_tool_call_audit_serialization.py`

**Preconditions**:
- `ToolCallResult` constructed with `tool_name="kubectl_describe"`

**Test Steps**:
1. **Given**: A `ToolCallResult` with `tool_name="kubectl_describe"`
2. **When**: The buggy getattr expression `getattr(tc, 'name', 'unknown')` is evaluated
3. **Then**: It returns `'unknown'` (proving the bug — `.name` does not exist on `ToolCallResult`)
4. **And When**: The correct getattr expression `getattr(tc, 'tool_name', 'unknown')` is evaluated
5. **Then**: It returns `"kubectl_describe"`
6. **And When**: `create_tool_call_event` is called with the correct value
7. **Then**: `event.event_data.actual_instance.tool_name == "kubectl_describe"`

**Acceptance Criteria**:
- **Behavior**: Audit event records the actual tool name
- **Correctness**: `tool_name` field equals `"kubectl_describe"`, not `"unknown"`
- **Accuracy**: getattr fallback never triggers for valid `ToolCallResult` objects

### UT-HAPI-600-002: tool_arguments extracted from ToolCallResult.result.params

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_tool_call_audit_serialization.py`

**Preconditions**:
- `ToolCallResult` with `result.params={"namespace": "default", "name": "api-gateway"}`

**Test Steps**:
1. **Given**: A `ToolCallResult` with `.result.params={"namespace": "default", "name": "api-gateway"}`
2. **When**: The buggy getattr expression `getattr(tc, 'arguments', {})` is evaluated
3. **Then**: It returns `{}` (proving the bug — `.arguments` does not exist)
4. **And When**: The correct expression `getattr(tc.result, 'params', {}) if tc.result else {}` is evaluated
5. **Then**: It returns `{"namespace": "default", "name": "api-gateway"}`

**Acceptance Criteria**:
- **Behavior**: Audit event records the actual tool arguments
- **Correctness**: `tool_arguments` field matches the original params dict exactly

### UT-HAPI-600-003: Multiple tool calls all captured

**BR**: BR-AUDIT-005
**Priority**: P0

**Test Steps**:
1. **Given**: 3 `ToolCallResult` objects with distinct names and params
2. **When**: Each is serialized with the correct getattr pattern
3. **Then**: All 3 produce distinct, correct `tool_name` and `tool_arguments` values

### UT-HAPI-600-004: Null result.params defaults to empty dict

**BR**: BR-AUDIT-005
**Priority**: P0

**Test Steps**:
1. **Given**: A `ToolCallResult` where `result.params=None`
2. **When**: `getattr(tc.result, 'params', {}) or {}` is evaluated
3. **Then**: Returns `{}` (not `None`)
4. **And**: `create_tool_call_event` accepts `{}` without validation error

### UT-HAPI-600-005: LLM request values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: model="claude-3-5-sonnet", toolsets=["kubernetes/core", "prometheus"], mcp_servers=["kubectl"]
2. **When**: `create_llm_request_event` is called
3. **Then**: `event_data.model == "claude-3-5-sonnet"`, `event_data.toolsets_enabled == ["kubernetes/core", "prometheus"]`, `event_data.mcp_servers == ["kubectl"]`

### UT-HAPI-600-006: LLM response values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: has_analysis=True, analysis_length=1500, tool_call_count=5, tokens_used=3200
2. **When**: `create_llm_response_event` is called
3. **Then**: All 4 values match in `event_data`; preview is truncated at 500 chars if longer

### UT-HAPI-600-007: Validation attempt values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: attempt=2, max_attempts=3, is_valid=False, errors=["workflow_not_found"], workflow_id="rollback-v1", human_review_reason="no_matching_workflow"
2. **When**: `create_validation_attempt_event` is called
3. **Then**: All values match in `event_data`, `is_final_attempt=False`, `validation_errors="workflow_not_found"`

### UT-HAPI-600-008: Response complete values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: response_data with rca, selected_workflow, confidence=0.85; tokens (prompt=2000, completion=1200)
2. **When**: `create_aiagent_response_complete_event` is called
3. **Then**: `event_data.response_data` matches input dict; `event_data.total_prompt_tokens == 2000`

### UT-HAPI-600-009: Response failed values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: error_message="LLM timeout after 120s", phase="llm_analysis", duration_seconds=120.5
2. **When**: `create_aiagent_response_failed_event` is called
3. **Then**: All 3 values match in `event_data`

### UT-HAPI-600-010: Enrichment completed values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: root_owner={"kind": "Deployment", "name": "api-gw", "namespace": "prod"}, chain_length=3, detected_labels={"gitOpsManaged": True}, failed_detections=["hpaEnabled"], history=True
2. **When**: `create_enrichment_completed_event` is called
3. **Then**: `event_data.root_owner_kind == "Deployment"`, `event_data.root_owner_name == "api-gw"`, `event_data.root_owner_namespace == "prod"`, `event_data.owner_chain_length == 3`, `event_data.detected_labels_summary == {"gitOpsManaged": True}`, `event_data.failed_detections == ["hpaEnabled"]`, `event_data.remediation_history_fetched == True`

### UT-HAPI-600-011: Enrichment failed values

**BR**: BR-AUDIT-005
**Priority**: P1

**Test Steps**:
1. **Given**: reason="rca_incomplete", detail="Retry 3/3 exhausted", affected_resource={"kind": "Pod", "name": "web-abc", "namespace": "demo"}
2. **When**: `create_enrichment_failed_event` is called
3. **Then**: `event_data.reason == "rca_incomplete"`, `event_data.detail == "Retry 3/3 exhausted"`, `event_data.affected_resource_kind == "Pod"`, `event_data.affected_resource_name == "web-abc"`, `event_data.affected_resource_namespace == "demo"`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: pytest (HAPI Python service convention)
- **Mocks**: `MagicMock` for audit_store only (external I/O)
- **Real objects**: `ToolCallResult`, `StructuredToolResult`, `InvestigationResult` from `holmes.core.models`
- **Location**: `holmesgpt-api/tests/unit/test_tool_call_audit_serialization.py`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Python | 3.14 | Runtime |
| pytest | 9.x | Test runner |
| pydantic | v2.x | Model validation |
| holmesgpt SDK | vendored (`dependencies/holmesgpt`) | `ToolCallResult`, `StructuredToolResult` types |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| HolmesGPT SDK vendored | Code | Available | Cannot construct real `ToolCallResult` | Use MagicMock (less valuable) |
| Circular import in `investigation_helpers.py` | Tech debt | Known | Cannot import `audit_llm_response_and_tools` directly | Test getattr expressions + factory calls directly (current approach) |

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write all 11 tests. Group A (001-004) prove the bug via getattr assertions. Group B (005-011) verify blast radius.
2. **Phase 2 (GREEN)**: Fix `investigation_helpers.py` lines 161-162 — change `getattr(tool_call, 'name', 'unknown')` to `getattr(tool_call, 'tool_name', 'unknown')` and `getattr(tool_call, 'arguments', {})` to `getattr(tool_call.result, 'params', {}) if tool_call.result else {}`
3. **Phase 3 (REFACTOR)**: Review other `getattr` patterns for defensive consistency; add `or {}` fallback for None params

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/600/TEST_PLAN.md` | Strategy, blast radius analysis, and test design |
| Unit test suite | `holmesgpt-api/tests/unit/test_tool_call_audit_serialization.py` | 11 pytest tests (4 P0 + 7 P1) |

---

## 13. Execution

```bash
# All #600 tests
cd holmesgpt-api
source venv/bin/activate
PYTHONPATH="$PWD/src:$PWD/src/clients:$PWD/../dependencies/holmesgpt" \
  python -m pytest tests/unit/test_tool_call_audit_serialization.py -v --no-cov

# Specific test
PYTHONPATH="$PWD/src:$PWD/src/clients:$PWD/../dependencies/holmesgpt" \
  python -m pytest tests/unit/test_tool_call_audit_serialization.py -v --no-cov -k "ut_hapi_600_001"

# Existing audit structure tests (regression check)
PYTHONPATH="$PWD/src:$PWD/src/clients:$PWD/../dependencies/holmesgpt" \
  python -m pytest tests/unit/test_audit_event_structure.py -v --no-cov
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | — | — | Existing `test_audit_event_structure.py` tests validate structure (field presence), not values. No changes needed. |

---

## 15. Blast Radius Assessment

### Audit Callers Analyzed

| Caller | File:Lines | Pattern Used | Bug Found? |
|--------|-----------|--------------|------------|
| `audit_llm_response_and_tools` (tool_call loop) | `investigation_helpers.py:155-164` | `getattr(tc, 'name', 'unknown')` | **YES — #600** |
| `audit_llm_request` (config fields) | `investigation_helpers.py:96-103` | `config.model`, `config.toolsets.keys()` | No — fields exist on `Config` |
| `audit_llm_response_and_tools` (response) | `investigation_helpers.py:131-152` | `investigation_result.analysis`, `.tool_calls` | No — fields exist on `InvestigationResult` |
| `handle_validation_exhaustion` | `investigation_helpers.py:222-231` | Direct args (attempt, errors, workflow_id) | No — no getattr |
| Enrichment completed | `llm_integration.py:894-900` | `_root.get("kind")`, `_root.get("name")` | No — `_root` is a dict with known keys |
| Enrichment failed | `llm_integration.py:865-870` | `ef.reason`, `ef.detail` — direct attribute | No — `EnrichmentFailure` fields |
| Response complete | `endpoint.py:98-103` | `data.get("incident_id")`, `result.model_dump()` | No — dict + Pydantic dump |
| Response failed | `endpoint.py:75-80` | `data.get("incident_id")`, `str(exc)` | No — dict + exception |
| Toolset sanitization | `llm_config.py:78-81` | `getattr(toolset, 'name', 'unknown')` | No — `Toolset.name` exists (verified) |

**Conclusion**: Only `investigation_helpers.py:161-162` has the attribute mismatch bug. All other callers use correct field names. The blast radius tests (UT-HAPI-600-005 through 011) confirm this as defense-in-depth.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-02 | Initial test plan with blast radius analysis |
