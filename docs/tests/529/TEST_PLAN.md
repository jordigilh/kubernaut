# Test Plan: #529 — RCA Flow Redesign with Dedicated History Tool, LLM-Provided affectedResource, and Conversation Continuity

**Feature**: Redesign HAPI RCA flow — dedicated `get_remediation_history` tool, LLM-provided `affectedResource`, history verification enforcement, conversation continuity, post-RCA label detection, labels in workflow actions
**Version**: 1.0
**Created**: 2026-03-25
**Author**: AI Assistant (Cursor)
**Status**: In Progress
**Branch**: `fix/1.1.0-rc9`

**Authority**:
- [#529]: HAPI: Redesign RCA flow with dedicated history tool, LLM-provided affectedResource, and conversation continuity
- [BR-HAPI-260]: Dedicated `get_remediation_history` LLM Tool
- [BR-HAPI-261]: LLM-Provided `affectedResource` in RCA Response
- [BR-HAPI-262]: RCA History Verification Enforcement
- [BR-HAPI-263]: Conversation Continuity in Self-Correction Loop
- [BR-HAPI-264]: Post-RCA Infrastructure Label Detection
- [BR-HAPI-265]: Infrastructure Labels in Workflow Actions Response
- [DD-HAPI-006 v1.6]: affectedResource in RCA — LLM-provided with history verification
- [DD-HAPI-016 v1.3]: Dedicated history tool decoupled from resource context
- [ADR-056]: Post-RCA label computation timing

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [#524 Test Plan](../524/TEST_PLAN.md) — predecessor; #529 changes injection source from root_owner to LLM-provided affectedResource

---

## 1. Scope

### In Scope

- **New tool: `get_remediation_history`** (`remediation_history_tool.py` NEW): Dedicated LLM tool that accepts `kind`, `name`, optional `namespace`. Internally resolves K8s owner chain for history lookup, queries DataStorage, returns only remediation history (not root owner). Tracks queried resources in `session_state["queried_history_resources"]`.
- **LLM-provided `affectedResource` parsing** (`result_parser.py`): `_parse_affected_resource()` extracts and validates two structure types — namespaced `{kind, name, namespace}` and cluster `{kind, name}`.
- **History verification enforcement** (`llm_integration.py`): `_validate_history_queried()` rejects RCA if LLM did not call `get_remediation_history` for the declared `affectedResource`. Self-correction with conversation continuity.
- **Conversation continuity** (`investigation.py`, `models.py`, `tool_calling_llm.py`, `llm_integration.py`): Holmes SDK `investigate_issues` accepts `previous_messages`, returns `messages` on `InvestigationResult`. HAPI threads messages through self-correction retries.
- **Post-RCA label detection** (`resource_context.py`, `llm_integration.py`): Remove `_detect_labels_if_needed` from resource context tool. Detect labels after RCA validation using validated `affectedResource`. Extra retry when labels change workflow context.
- **Labels in workflow actions** (`workflow_discovery.py`, `prompt_builder.py`): Ensure `list_available_actions` response includes detected infrastructure labels. Phase 2 retry prompt includes label context.
- **Target resource injection update** (`llm_integration.py`): `_inject_target_resource` uses LLM-provided `affectedResource` instead of `session_state["root_owner"]`.
- **Prompt update** (`prompt_builder.py`): Phase 5 response format instructs LLM to include `affectedResource` in RCA output.

### Out of Scope

- **Go controller changes**: AIAnalysis reads `affectedResource` from HAPI response unchanged.
- **DataStorage API changes**: DS endpoints remain the same; only the caller changes (tool instead of resource context).
- **Real LLM validation**: Deferred to post-merge task. Existing E2E suite validates regression safety.
- **New E2E test scenarios**: Existing deployment-scoped E2E tests (OOM, crashloop, rollback) validate regression safety after Mock LLM update.

### Resource Context Tools: Role Change (Not Deprecated)

`get_namespaced_resource_context` and `get_cluster_resource_context` remain available as investigation aids. What changes:

- **No longer drive `affectedResource` injection** — `affectedResource` comes from the LLM's RCA response
- **No longer perform label detection** — label detection moves to post-RCA
- **`root_owner` session_state** — still populated if tools are called, but LLM's explicit declaration takes precedence

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (tool invocation, parsing, verification, injection, label detection, conversation threading)
- **Integration**: >=80% of integration-testable code (self-correction loop, DS client wiring, cross-component flow)
- **E2E**: Deferred — existing E2E tests validate regression safety; new E2E scenario tracked as follow-up

### 2-Tier Minimum

Every BR covered by at least Unit + Integration tests.

---

## 3. Testable Code Inventory

### Unit-Testable Code (Pure Logic, No I/O)

| File | Component | Estimated Lines |
|------|-----------|-----------------|
| `holmesgpt-api/src/toolsets/remediation_history_tool.py` (NEW) | Tool invoke logic, owner chain resolution dispatch, session_state tracking | ~120 |
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | `_validate_history_queried` (NEW), conversation continuity threading, post-RCA label detection trigger, updated `_inject_target_resource` | ~100 |
| `holmesgpt-api/src/extensions/incident/result_parser.py` | `_parse_affected_resource` for two structure types | ~30 |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | History tool guidance, verification feedback, label context in Phase 2, affectedResource in Phase 5 | ~40 |
| `holmesgpt-api/src/toolsets/workflow_discovery.py` | Labels in `list_available_actions` response format | ~20 |
| `dependencies/holmesgpt/holmes/core/investigation.py` | `previous_messages` parameter, messages in result | ~15 |
| `dependencies/holmesgpt/holmes/core/models.py` | `InvestigationResult.messages` field | ~5 |
| `holmesgpt-api/src/extensions/llm_config.py` | `register_remediation_history_tool` registration | ~10 |
| `holmesgpt-api/src/toolsets/resource_context.py` | Remove `_detect_labels_if_needed` from `_invoke_async` | ~20 |

**Total unit-testable**: ~360 lines new/modified

### Integration-Testable Code (I/O, Wiring, Cross-Component)

| File | Component |
|------|-----------|
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | Full self-correction loop with conversation continuity, DS client calls (~200 lines of loop logic) |
| `holmesgpt-api/src/toolsets/remediation_history_tool.py` | K8s client owner resolution, DS client history fetch |

---

## 4. BR Coverage Matrix

### Tier 1: Unit Tests (28 Scenarios)

| ID | BR | Business Outcome Under Test |
|----|----|-----------------------------|
| `UT-HAPI-260-001` | BR-HAPI-260 | History tool invoked with namespaced resource returns remediation chain from DS |
| `UT-HAPI-260-002` | BR-HAPI-260 | History tool invoked with cluster-scoped resource (no namespace) returns history |
| `UT-HAPI-260-003` | BR-HAPI-260 | History tool internally resolves Pod to root Deployment for history lookup |
| `UT-HAPI-260-004` | BR-HAPI-260 | History tool tracks queried resource in `session_state["queried_history_resources"]` |
| `UT-HAPI-260-005` | BR-HAPI-260 | History tool returns only history (not root owner) to LLM |
| `UT-HAPI-260-006` | BR-HAPI-260 | History tool handles DS client unavailable gracefully (empty response, no crash) |
| `UT-HAPI-260-007` | BR-HAPI-260 | Owner chain resolution failure: tool falls back to original resource for history lookup |
| `UT-HAPI-260-008` | BR-HAPI-260 | Multiple history queries accumulate correctly in `queried_history_resources` |
| `UT-HAPI-261-001` | BR-HAPI-261 | Parser extracts valid namespaced `affectedResource` `{kind, name, namespace}` |
| `UT-HAPI-261-002` | BR-HAPI-261 | Parser extracts valid cluster `affectedResource` `{kind, name}` (no namespace) |
| `UT-HAPI-261-003` | BR-HAPI-261 | Parser rejects `affectedResource` with missing `kind` or `name` |
| `UT-HAPI-261-004` | BR-HAPI-261 | Parser rejects `affectedResource` with wrong type (string instead of object) |
| `UT-HAPI-261-005` | BR-HAPI-261 | Parser returns `None` when `affectedResource` absent from RCA response |
| `UT-HAPI-261-006` | BR-HAPI-261 | `_inject_target_resource` uses LLM-provided `affectedResource` for conditional `TARGET_RESOURCE_*` injection |
| `UT-HAPI-262-001` | BR-HAPI-262 | History verification passes when `affectedResource` matches a queried resource |
| `UT-HAPI-262-002` | BR-HAPI-262 | History verification fails when `affectedResource` NOT in queried resources |
| `UT-HAPI-262-003` | BR-HAPI-262 | History verification fails when no resources were queried at all |
| `UT-HAPI-262-004` | BR-HAPI-262 | Verification feedback message includes the unqueried resource for LLM self-correction |
| `UT-HAPI-262-005` | BR-HAPI-262 | After MAX_VALIDATION_ATTEMPTS exhausted: `needs_human_review=true`, `human_review_reason=history_not_queried` |
| `UT-HAPI-263-001` | BR-HAPI-263 | SDK `investigate_issues(previous_messages=...)` accepts and seeds conversation |
| `UT-HAPI-263-002` | BR-HAPI-263 | SDK `InvestigationResult` includes full `messages` list from LLM conversation |
| `UT-HAPI-263-003` | BR-HAPI-263 | HAPI threads messages from attempt N to attempt N+1 in self-correction loop |
| `UT-HAPI-263-004` | BR-HAPI-263 | First attempt passes `None` for `previous_messages` (no prior context); SDK behaves identically to current |
| `UT-HAPI-264-001` | BR-HAPI-264 | Labels NOT detected during resource context tool calls (clean separation) |
| `UT-HAPI-264-002` | BR-HAPI-264 | Labels detected after RCA validation using validated `affectedResource` |
| `UT-HAPI-264-003` | BR-HAPI-264 | No extra retry when labels do not change workflow selection criteria |
| `UT-HAPI-265-001` | BR-HAPI-265 | `list_available_actions` response includes detected infrastructure labels |
| `UT-HAPI-265-002` | BR-HAPI-265 | Phase 2 retry prompt includes detected labels when label-triggered retry occurs |

### Tier 2: Integration Tests (6 Scenarios)

| ID | BR | Business Outcome Under Test |
|----|----|-----------------------------|
| `IT-HAPI-260-001` | BR-HAPI-260 | History tool calls DS client with owner-resolved resource, returns structured chain |
| `IT-HAPI-261-001` | BR-HAPI-261 | End-to-end: LLM provides affectedResource -> parsed -> history verified -> workflow validated -> labels detected -> final selection |
| `IT-HAPI-262-001` | BR-HAPI-262 | History not queried -> rejection -> retry with feedback -> LLM queries history -> success |
| `IT-HAPI-263-001` | BR-HAPI-263 | Full self-correction loop preserves conversation context across 3 retry attempts |
| `IT-HAPI-264-001` | BR-HAPI-264 | Post-RCA label detection triggers extra retry when labels affect workflow selection |
| `IT-HAPI-265-001` | BR-HAPI-265 | LLM receives label context from both prompt text and `list_available_actions` tool |

---

## 5. Risk Mitigation Matrix

| ID | Risk | Severity | Mitigation | Test Coverage |
|----|------|----------|------------|---------------|
| R1 | SDK backward compatibility — `previous_messages` may break existing callers | HIGH | Parameter is optional (defaults to `None`); `messages` in `InvestigationResult` is additive (new field) | `UT-HAPI-263-004`: validates `None` default; `UT-HAPI-263-001`: validates non-None path |
| R2 | LLM reliability — may not provide `affectedResource` or provide wrong structure | HIGH | Self-correction loop with conversation continuity; max 3 attempts; escalation to `needs_human_review=true` with `human_review_reason=rca_incomplete` | `UT-HAPI-261-003/004/005`: invalid/missing parsing; `IT-HAPI-262-001`: full retry-to-success flow |
| R3 | Owner chain resolution failure — K8s API unavailable or resource deleted | MEDIUM | Graceful fallback: if owner resolution fails, use original LLM-provided resource for history lookup; log warning | `UT-HAPI-260-007`: owner resolution failure falls back to original resource |
| R4 | Context window overflow — long conversations across retries exceed LLM context | MEDIUM | Holmes SDK `ToolCallingLLM.call()` already has message truncation logic; no HAPI changes needed | `UT-HAPI-263-003`: validates message threading; truncation is SDK-internal |
| R5 | Stale `queried_history_resources` — may accumulate incorrectly across retries | LOW | `session_state` is shared dict for entire `analyze_incident` call; resources only added, never removed; reset on new incident | `UT-HAPI-260-004`: validates tracking; `UT-HAPI-260-008`: multiple queries accumulate correctly |
| R6 | Label detection extra retry cost — post-RCA labels may always force a retry | MEDIUM | Only retries if labels actually change workflow selection criteria; conversation continuity makes retry cheap | `IT-HAPI-264-001`: validates label-triggered retry; `UT-HAPI-264-003`: no retry when labels don't change selection |
| R7 | Regression on existing deployment-scoped flows | HIGH | Existing E2E tests remain green; resource context tools still available as investigation aids | Existing E2E suite (OOM, crashloop, rollback) validates regression |
| R8 | History tool called for wrong resource | MEDIUM | Prompt Phase 3 guides LLM to query history for root cause resource; verification enforces the link | `UT-HAPI-262-002`: verification catches mismatch; `IT-HAPI-262-001`: retry flow corrects mismatch |

---

## 6. TDD Implementation Groups

Tests are organized into 10 dependency-ordered TDD groups. Each group follows RED -> GREEN -> REFACTOR with commits after every phase.

### Group 0: Mock LLM Update (Prerequisite)
- `test/services/mock-llm/src/server.py`: `include_affected_resource=True` default, `_has_remediation_history_tool`, 5-step discovery flow

### Group 1: SDK Conversation Continuity (BR-HAPI-263 SDK)
- Tests: UT-263-001, UT-263-002, UT-263-004
- Files: `models.py`, `investigation.py`, `tool_calling_llm.py`

### Group 2: Dedicated History Tool (BR-HAPI-260)
- Tests: UT-260-001 through UT-260-008
- Files: `remediation_history_tool.py` (NEW), `llm_config.py`

### Group 3: affectedResource Parsing (BR-HAPI-261)
- Tests: UT-261-001 through UT-261-005
- Files: `result_parser.py`, `prompt_builder.py`

### Group 4: History Verification (BR-HAPI-262)
- Tests: UT-262-001 through UT-262-005
- Files: `llm_integration.py`

### Group 5: Conversation Continuity HAPI (BR-HAPI-263 HAPI)
- Tests: UT-263-003
- Files: `llm_integration.py`

### Group 6: TARGET_RESOURCE Injection (BR-HAPI-261)
- Tests: UT-261-006
- Files: `llm_integration.py`, `test_target_resource_injection.py` (existing)

### Group 7: Post-RCA Label Detection (BR-HAPI-264)
- Tests: UT-264-001, UT-264-002, UT-264-003
- Files: `resource_context.py`, `llm_integration.py`

### Group 8: Labels in Workflow Actions (BR-HAPI-265)
- Tests: UT-265-001, UT-265-002
- Files: `workflow_discovery.py`, `prompt_builder.py`

### Group 9: Integration Tests (All BRs)
- Tests: IT-260-001, IT-261-001, IT-262-001, IT-263-001, IT-264-001, IT-265-001
- Files: Cross-component wiring tests

---

## 7. Anti-Pattern Compliance

- No `time.Sleep()` — all async waits use `Eventually()`
- No `Skip()` — all tests implemented or removed
- No direct audit infrastructure testing — test business logic with audit side effects
- Mocks: Only external dependencies (DS client, K8s API, LLM) mocked in unit tests; integration tests use real DS
- Python tests use pytest (HAPI convention); Go tests use Ginkgo/Gomega BDD

---

## 8. Test Infrastructure

- **Unit Tests**: pytest, mocked DS client / K8s client / Holmes SDK
- **Integration Tests**: pytest, real DataStorage (programmatic container), mocked LLM
- **Location**: `holmesgpt-api/tests/unit/` and `holmesgpt-api/tests/integration/`

---

## 9. Coverage Summary

All 34 test scenarios have defined business outcomes (section 4). Key scenarios have detailed Given/When/Then specifications in the TDD implementation plan. Risk-mitigation tests (R1-R8) are mapped to specific test IDs in section 5.

- **Unit**: 28 scenarios covering ~360 lines of unit-testable code -> target >=80%
- **Integration**: 6 scenarios covering self-correction loop, DS client wiring, and cross-component flow -> target >=80%
- **2-Tier Minimum**: Every BR covered by at least Unit + Integration
- **Risk tests**: 4 additional unit scenarios (UT-260-007, UT-260-008, UT-264-003, UT-262-005) specifically targeting identified risks
