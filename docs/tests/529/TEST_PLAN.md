# Test Plan: #529 — Three-Phase RCA Flow with EnrichmentService, LLM-Provided affectedResource, and Conversation Continuity

**Feature**: Redesign HAPI RCA flow into three phases — Phase 1 (RCA + affectedResource), Phase 2 (HAPI-driven EnrichmentService), Phase 3 (Workflow Selection with enrichment context)
**Version**: 2.0
**Created**: 2026-03-25
**Updated**: 2026-03-25
**Author**: AI Assistant (Cursor)
**Status**: In Progress
**Branch**: `fix/1.1.0-rc9`

**Authority**:
- [#529]: HAPI: Redesign RCA flow with LLM-provided affectedResource, conversation continuity, and HAPI-driven enrichment
- [BR-HAPI-261]: LLM-Provided `affectedResource` in RCA Response
- [BR-HAPI-263]: Conversation Continuity Across Investigation Phases
- [BR-HAPI-264]: Post-RCA Infrastructure Label Detection via EnrichmentService
- [BR-HAPI-265]: Infrastructure Labels in Workflow Discovery Context
- [DD-HAPI-006 v1.6]: affectedResource in RCA — LLM-provided with HAPI owner resolution
- [DD-HAPI-002 v1.4]: Three-phase loop structure (RCA, enrichment, workflow selection)
- [DD-HAPI-016 v1.1]: Remediation history context enrichment (DS endpoint contract)
- [ADR-055 v1.5]: LLM-driven context enrichment (EnrichmentService architecture)
- [ADR-056 v1.7]: Post-RCA label computation (EnrichmentService as sole authoritative source)

**Dropped BRs**:
- ~~[BR-HAPI-260]~~: Dedicated `get_remediation_history` tool — DROPPED. Existing resource context tools already return history; EnrichmentService provides authoritative history in Phase 2.
- ~~[BR-HAPI-262]~~: RCA History Verification Enforcement — DROPPED. HAPI always provides verified history in Phase 2 regardless of LLM behavior.

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [#524 Test Plan](../524/TEST_PLAN.md) — predecessor; #529 changes injection source from session_state root_owner to EnrichmentResult

---

## 1. Scope

### In Scope

- **LLM-provided `affectedResource` parsing** (`result_parser.py`): `_parse_affected_resource()` extracts and validates two structure types — namespaced `{kind, name, namespace}` and cluster `{kind, name}`.
- **EnrichmentService** (`enrichment_service.py` NEW): HAPI-driven Phase 2 service that takes parsed `affectedResource`, resolves K8s owner chain to root owner, detects infrastructure labels, fetches remediation history from DataStorage, and returns an `EnrichmentResult`. Retries infrastructure calls with exponential backoff (3 retries, 1s/2s/4s). Fails hard with `rca_incomplete` after retry exhaustion.
- **Conversation continuity** (`investigation.py`, `models.py`, `tool_calling_llm.py`, `llm_integration.py`): Holmes SDK `investigate_issues` accepts `previous_messages`, returns `messages` on `InvestigationResult`. Phase 1 messages flow to Phase 3 via `previous_messages` for full conversation context.
- **Three-phase orchestration** (`llm_integration.py`): Phase 1 (RCA + affectedResource validation), Phase 2 (EnrichmentService), Phase 3 (workflow selection with enrichment context). Shared retry budget: `MAX_VALIDATION_ATTEMPTS = 3` across all phases.
- **Resource context tool refactor** (`resource_context.py`): Strip `session_state` writes (`root_owner`, `resource_scope`, `detected_labels`). `_detect_labels_if_needed` still runs and returns `detected_infrastructure` to LLM but no longer writes to session_state. Tools are purely informational in Phase 1.
- **Target resource injection update** (`llm_integration.py`): `_inject_target_resource` rewired to accept `EnrichmentResult.root_owner` instead of reading `session_state["root_owner"]`.
- **Labels in workflow actions** (`workflow_discovery.py`, `prompt_builder.py`): `list_available_actions` response includes detected infrastructure labels from EnrichmentService. Phase 3 prompt includes label context.
- **Prompt update** (`prompt_builder.py`): Phase 1 prompt instructs LLM to include `affectedResource` in RCA output (no workflow selection). Phase 3 prompt provides enrichment context and requests workflow selection.

### Out of Scope

- **Go controller changes**: AIAnalysis reads `affectedResource` from HAPI response unchanged.
- **DataStorage API changes**: DS endpoints remain the same; caller is EnrichmentService.
- **Real LLM validation**: Deferred to post-merge task.
- **New E2E test scenarios**: Existing E2E tests validate regression safety after Mock LLM update.

### Resource Context Tools: Role Change (Not Deprecated)

`get_namespaced_resource_context` and `get_cluster_resource_context` remain available as optional Phase 1 investigation tools. What changes:

- **No longer write to `session_state`** — `root_owner`, `resource_scope`, `detected_labels` no longer stored
- **No longer drive `affectedResource` injection** — `affectedResource` comes from the LLM's Phase 1 RCA response, owner-resolved by EnrichmentService
- **Label detection still runs** — `_detect_labels_if_needed` still executes and returns `detected_infrastructure` in the tool response for LLM informational use, but does NOT write to `session_state`
- **EnrichmentService is authoritative** — Phase 2 provides the authoritative owner resolution, label detection, and history fetch

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (parsing, enrichment service, tool refactor, injection, conversation threading, orchestration)
- **Integration**: >=80% of integration-testable code (three-phase loop, DS client wiring, cross-component flow)
- **E2E**: Deferred — existing E2E tests validate regression safety; new E2E scenario tracked as follow-up

### 2-Tier Minimum

Every BR covered by at least Unit + Integration tests.

---

## 3. Testable Code Inventory

### Unit-Testable Code (Pure Logic, No I/O)

| File | Component | Estimated Lines |
|------|-----------|-----------------|
| `holmesgpt-api/src/extensions/incident/enrichment_service.py` (NEW) | EnrichmentService: owner resolution dispatch, label detection, history fetch, retry logic, EnrichmentResult construction | ~180 |
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | Three-phase orchestration, conversation continuity threading, updated `_inject_target_resource`, shared retry budget | ~120 |
| `holmesgpt-api/src/extensions/incident/result_parser.py` | `_parse_affected_resource` for two structure types | ~30 |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Phase 1 prompt (affectedResource instructions), Phase 3 prompt (enrichment context + workflow selection) | ~40 |
| `holmesgpt-api/src/toolsets/resource_context.py` | Strip session_state writes, keep label detection return | ~30 |
| `holmesgpt-api/src/toolsets/workflow_discovery.py` | Labels in `list_available_actions` response format | ~20 |
| `dependencies/holmesgpt/holmes/core/investigation.py` | `previous_messages` parameter, messages in result | ~15 |
| `dependencies/holmesgpt/holmes/core/models.py` | `InvestigationResult.messages` field | ~5 |

**Total unit-testable**: ~440 lines new/modified

### Integration-Testable Code (I/O, Wiring, Cross-Component)

| File | Component |
|------|-----------|
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | Full three-phase loop with conversation continuity, EnrichmentService calls (~200 lines of loop logic) |
| `holmesgpt-api/src/extensions/incident/enrichment_service.py` | K8s client owner resolution, DS client history fetch, retry behavior |

---

## 4. BR Coverage Matrix

### Tier 1: Unit Tests (22 Scenarios)

| ID | BR | Business Outcome Under Test |
|----|----|-----------------------------|
| `UT-HAPI-261-001` | BR-HAPI-261 | Parser extracts valid namespaced `affectedResource` `{kind, name, namespace}` |
| `UT-HAPI-261-002` | BR-HAPI-261 | Parser extracts valid cluster `affectedResource` `{kind, name}` (no namespace) |
| `UT-HAPI-261-003` | BR-HAPI-261 | Parser rejects `affectedResource` with missing `kind` or `name` |
| `UT-HAPI-261-004` | BR-HAPI-261 | Parser rejects `affectedResource` with wrong type (string instead of object) |
| `UT-HAPI-261-005` | BR-HAPI-261 | Parser returns `None` when `affectedResource` absent from RCA response |
| `UT-HAPI-261-006` | BR-HAPI-261 | `_inject_target_resource` uses `EnrichmentResult.root_owner` for conditional `TARGET_RESOURCE_*` injection |
| `UT-HAPI-263-001` | BR-HAPI-263 | SDK `investigate_issues(previous_messages=...)` accepts and seeds conversation |
| `UT-HAPI-263-002` | BR-HAPI-263 | SDK `InvestigationResult` includes full `messages` list from LLM conversation |
| `UT-HAPI-263-003` | BR-HAPI-263 | HAPI threads Phase 1 messages to Phase 3 via `previous_messages` |
| `UT-HAPI-263-004` | BR-HAPI-263 | First attempt passes `None` for `previous_messages` (no prior context); SDK behaves identically to current |
| `UT-HAPI-264-001` | BR-HAPI-264 | EnrichmentService detects labels for resolved root owner in Phase 2 |
| `UT-HAPI-265-001` | BR-HAPI-265 | `list_available_actions` response includes detected infrastructure labels from EnrichmentService |
| `UT-HAPI-265-002` | BR-HAPI-265 | Phase 3 prompt includes detected labels in enrichment context |
| `UT-529-E-001` | BR-HAPI-264 | EnrichmentService resolves owner chain (Pod -> Deployment) for affectedResource |
| `UT-529-E-002` | BR-HAPI-264 | EnrichmentService fetches remediation history for resolved root owner |
| `UT-529-E-003` | BR-HAPI-264 | EnrichmentService retries K8s API with exponential backoff (3 retries, 1s/2s/4s) |
| `UT-529-E-004` | BR-HAPI-264 | EnrichmentService retries DS client with exponential backoff |
| `UT-529-E-005` | BR-HAPI-264 | EnrichmentService fails hard (`rca_incomplete`) after retry exhaustion |
| `UT-529-E-006` | BR-HAPI-264 | EnrichmentService returns complete `EnrichmentResult` (root_owner + labels + history) |
| `UT-529-RC-001` | #529 | `get_namespaced_resource_context` no longer writes `root_owner`/`resource_scope` to `session_state` |
| `UT-529-RC-002` | #529 | `get_cluster_resource_context` no longer writes `root_owner`/`resource_scope` to `session_state` |
| `UT-529-RC-003` | #529 | Resource context tools still return correct data to LLM (including `detected_infrastructure`) |

### Tier 2: Integration Tests (5 Scenarios)

| ID | BR | Business Outcome Under Test |
|----|----|-----------------------------|
| `IT-HAPI-261-001` | BR-HAPI-261 | End-to-end: Phase 1 (LLM provides affectedResource) -> Phase 2 (EnrichmentService resolves) -> Phase 3 (workflow selected with enrichment context) |
| `IT-HAPI-263-001` | BR-HAPI-263 | Full three-phase flow preserves conversation context: Phase 1 messages flow to Phase 3 |
| `IT-HAPI-264-001` | BR-HAPI-264 | EnrichmentService labels influence Phase 3 workflow selection |
| `IT-HAPI-265-001` | BR-HAPI-265 | LLM receives label context from both Phase 3 prompt and `list_available_actions` tool |
| `IT-529-E-001` | BR-HAPI-264 | EnrichmentService infrastructure retry and fail-hard behavior with real-ish clients |

---

## 5. Risk Mitigation Matrix

| ID | Risk | Severity | Mitigation | Test Coverage |
|----|------|----------|------------|---------------|
| R1 | `_inject_target_resource` temporarily broken between G4 (strip writes) and G5 (rewire to EnrichmentResult) | HIGH | G4 REFACTOR seeds `session_state` in existing injection tests as bridge; G5 GREEN rewires signature. Do NOT run E2E between CP5 and CP6. | `UT-HAPI-261-006`: validates EnrichmentResult injection |
| R2 | ~22 existing tests depend on `session_state` writes from resource context tools | HIGH | G4 RED inventories all affected tests. G4 REFACTOR updates them: invert write assertions, keep return-value assertions. | `UT-529-RC-001/002/003`: validate stripped writes |
| R3 | SDK backward compatibility — `previous_messages` may break existing callers | HIGH | Parameter is optional (defaults to `None`); `messages` in `InvestigationResult` is additive (new field) | `UT-HAPI-263-004`: validates `None` default |
| R4 | LLM reliability — may not provide `affectedResource` or provide wrong structure | HIGH | Shared retry budget (3 attempts across phases); fail with `rca_incomplete` on exhaustion | `UT-HAPI-261-003/004/005`: invalid/missing parsing; `IT-HAPI-261-001`: full flow |
| R5 | EnrichmentService infrastructure failures (K8s API, DS) | MEDIUM | Exponential backoff retries (3 retries, 1s/2s/4s); fail hard after exhaustion | `UT-529-E-003/004/005`: retry and fail-hard behavior; `IT-529-E-001`: integration retry |
| R6 | Context window overflow — long conversations across Phase 1 -> Phase 3 | MEDIUM | Holmes SDK `ToolCallingLLM.call()` already has message truncation logic | `UT-HAPI-263-003`: validates message threading |
| R7 | Regression on existing deployment-scoped flows | HIGH | Existing E2E tests remain green; resource context tools still available | Existing E2E suite (OOM, crashloop, rollback) |
| R8 | DD-HAPI-016 version mismatch (test plan cited v1.3, actual v1.1) | LOW | Fixed in this version of test plan | N/A |

---

## 6. TDD Implementation Groups

Tests are organized into 8 dependency-ordered TDD groups. Each group follows RED -> GREEN -> REFACTOR with commits after every phase.

### Group 0: Mock LLM 3-Phase Refactor (Prerequisite)

- `test/services/mock-llm/src/server.py`: Refactor for 3-phase protocol — Phase 1 returns RCA + `affectedResource` (no workflow), Phase 3 returns workflow selection given enrichment context. Remove `_has_remediation_history_tool` (BR-260 dropped).

### Group 1: SDK Conversation Continuity (BR-HAPI-263 SDK)

- Tests: UT-263-001, UT-263-002, UT-263-004
- Files: `models.py`, `investigation.py`, `tool_calling_llm.py`

### Group 2: affectedResource Parsing (BR-HAPI-261)

- Tests: UT-261-001 through UT-261-005
- Files: `result_parser.py`, `prompt_builder.py`

### Group 3: EnrichmentService Core (BR-HAPI-264 + Phase 2)

- Tests: UT-529-E-001 through UT-529-E-006, UT-264-001
- Files: `enrichment_service.py` (NEW)

### Group 4: Resource Context Tool Refactor (#529)

- Tests: UT-529-RC-001 through UT-529-RC-003
- Files: `resource_context.py`
- WARNING: ~22 existing tests need updating in REFACTOR phase (R1, R2)

### Group 5: Three-Phase Orchestration (BR-HAPI-263 HAPI + BR-HAPI-261 injection)

- Tests: UT-263-003, UT-261-006, plus orchestration flow scenarios
- Files: `llm_integration.py`
- Resolves R1: `_inject_target_resource` rewired to `EnrichmentResult.root_owner`

### Group 6: Labels in Workflow Actions (BR-HAPI-265)

- Tests: UT-265-001, UT-265-002
- Files: `workflow_discovery.py`, `prompt_builder.py`

### Group 7: Integration Tests (All BRs)

- Tests: IT-261-001, IT-263-001, IT-264-001, IT-265-001, IT-529-E-001
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

All 27 test scenarios have defined business outcomes (section 4). Risk-mitigation tests (R1-R8) are mapped to specific test IDs in section 5.

- **Unit**: 22 scenarios covering ~440 lines of unit-testable code -> target >=80%
- **Integration**: 5 scenarios covering three-phase loop, EnrichmentService wiring, and cross-component flow -> target >=80%
- **2-Tier Minimum**: Every active BR covered by at least Unit + Integration
- **Risk tests**: UT-529-E-003/004/005 (retry/fail-hard), UT-529-RC-001/002 (stripped writes), UT-HAPI-263-004 (backward compat)

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial test plan: 34 scenarios across 6 BRs (BR-260 through BR-265) |
| 2.0 | 2026-03-25 | Major revision: BR-260 and BR-262 dropped. Added EnrichmentService scenarios (UT-529-E), resource context refactor scenarios (UT-529-RC). Renumbered TDD groups (G0-G7). Updated risk matrix. Fixed DD-HAPI-016 version to v1.1. 27 scenarios across 4 active BRs. |
