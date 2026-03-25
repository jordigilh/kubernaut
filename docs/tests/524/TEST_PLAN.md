# Test Plan: #524 — Cluster-Scoped Resource Context and Relaxed Canonical Param Injection

**Feature**: Add `get_cluster_resource_context` tool, rename `get_resource_context` to `get_namespaced_resource_context`, relax HAPI canonical param enforcement to conditional inject-if-declared
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant (Cursor)
**Status**: Draft
**Branch**: `fix/1.1.0-rc9`

**Authority**:
- [#524]: HAPI target_resource auto-injection unconditionally overwrites params for node-scoped workflows
- [BR-496 v2]: HAPI-Owned Target Resource Identity (superseded in part by #524)
- [DD-HAPI-006]: affectedResource in RCA — HAPI-owned via canonical params
- [ADR-055]: LLM-Driven Context Enrichment (Post-RCA)
- [DD-WORKFLOW-003]: Parameterized Actions — TARGET_RESOURCE_* convention

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [BR-496 Test Plan](../496/TEST_PLAN.md) — predecessor; #524 supersedes validation Step 0 and injection behavior
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 1. Scope

### In Scope

- **New tool: `get_cluster_resource_context`** (`resource_context.py`): Cluster-scoped resolution for Nodes, PVs, Namespaces — no namespace parameter, no owner chain walk, resource IS the root_owner. Stores `root_owner` and `resource_scope: "cluster"` in `session_state`.
- **Tool rename: `get_resource_context` → `get_namespaced_resource_context`** (`resource_context.py`): Symmetric naming for clarity. Stores `resource_scope: "namespaced"` in `session_state`.
- **Relaxed canonical param enforcement** (`workflow_response_validator.py`): Remove mandatory Step 0 (`_validate_canonical_params`). HAPI no longer rejects workflows missing TARGET_RESOURCE_* declarations. Injection is conditional: populate only params that exist in the workflow schema.
- **Conditional injection logic** (`llm_integration.py`): `_inject_target_resource` reads `resource_scope` from `session_state` to decide whether to inject `TARGET_RESOURCE_NAMESPACE`. Injects only params declared in the workflow schema.
- **Phase 3b prompt update** (`prompt_builder.py`): Document both tools with symmetric names and when-to-use guidance.
- **Post-selection validation guard** (`llm_integration.py`): When a node-scoped workflow is selected but the last resource context call used the namespaced tool, flag for retry with a prompt nudge.
- **Toolset registration** (`llm_config.py`): Register both tools in the `resource_context` toolset.

### Out of Scope

- **Workflow schema changes**: `remove-taint-v1` already declares `TARGET_RESOURCE_NAMESPACE: required: false` — no schema migration needed.
- **Go controller changes**: Go reads `affectedResource` from HAPI response unchanged; no behavioral change.
- **DataStorage changes**: DS does not enforce TARGET_RESOURCE_* conventions.
- **Mock LLM scenario updates**: Mock LLM already supports tool call routing; new tool name is additive.
- **Existing E2E tests**: Deployment-scoped E2E tests (OOM, crashloop, rollback) are unaffected — `get_namespaced_resource_context` behaves identically to the old `get_resource_context`.
- **Go code comments**: 7 Go files reference old tool name in comments only — updated in docs/regen pass, not functional.

### Risk Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Mock LLM tool name matching | CRITICAL | Update `test/services/mock-llm/src/server.py` — `_has_resource_context_tool` must match `get_namespaced_resource_context` or `get_cluster_resource_context`; `_generate_tool_call` emits new tool name |
| `CANONICAL_TARGET_PARAMS` in conftest | HIGH | Keep constant, update comment from "must declare" to "commonly declared" — tests using it still pass, new tests without it also pass |
| Schema not available at injection call site | MEDIUM | Store `workflow_schema` in `session_state` during validation step — validator already fetches schema; injection reads it from session_state |
| 4 existing tests assert old behavior | MEDIUM | UT-496-007/008/009 updated to expect `is_valid=True`; UT-496-016 updated to assert new tool name |
| Two tools in one toolset (SDK compat) | MEDIUM | Verified: `WorkflowDiscoveryToolset` already registers 3 tools per toolset — pattern is established |
| CRD/OpenAPI description text | LOW | Update Go type comment in `aianalysis_types.go` → `make manifests` regenerates; OpenAPI source model → regenerate |
| Go code comments | LOW | Update during docs pass — non-functional |

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Two tools with symmetric names (`get_namespaced_resource_context` / `get_cluster_resource_context`) | Eliminates LLM ambiguity about when to provide namespace; self-documenting |
| `resource_scope` in session_state | Clean signal for injection logic — no fragile heuristics about root_owner shape |
| Conditional param injection (inject-if-declared) | Workflows own their contract; HAPI doesn't force params the workflow doesn't need |
| Remove mandatory Step 0 validation | Cluster-scoped workflows legitimately omit TARGET_RESOURCE_NAMESPACE; mandatory enforcement was too rigid |
| Post-selection validation guard | Safety net for LLM using wrong tool variant; mirrors existing self-correction patterns |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (tool invocation, injection logic, validation logic, prompt construction, session_state tracking)
- **E2E**: >=80% of full pipeline (deferred — no new E2E scenario for #524; existing E2E tests validate deployment-scoped paths remain green)

### 2-Tier Minimum

- **Unit tests** (Python pytest): Catch logic and correctness errors in tool behavior, injection, validation relaxation, prompt text, and session_state wiring
- **E2E tests** (Go Ginkgo in Kind): Existing deployment-scoped E2E validates regression safety. Node-scoped E2E deferred to `pending-taint` scenario hardening.

### Tier Skip Rationale

- **Integration (Tier 2)**: Skipped. All modified code is Python (HAPI). Python integration tests are not part of the kubernaut test infrastructure — cross-component behavior is validated by Go E2E tests in Kind. The 2-tier minimum is satisfied by Unit (pytest) + existing E2E (regression).
- **E2E (new scenario)**: Deferred. Node-scoped E2E (`pending-taint` scenario) requires Kind cluster with tainted nodes and the `remove-taint-v1` workflow deployed. This is tracked as a follow-up scenario validation, not a blocker for the code changes.

### Business Outcome Quality Bar

Tests validate **business outcomes**:
- Node-scoped workflow receives correct `TARGET_RESOURCE_KIND: Node` and `TARGET_RESOURCE_NAME: <node-name>` with no namespace injected
- Deployment-scoped workflows continue receiving all 3 TARGET_RESOURCE_* params as before (regression safety)
- Workflows that don't declare TARGET_RESOURCE_NAMESPACE pass validation and execute correctly
- LLM sees both tools with clear descriptions and selects the correct one based on resource scope
- Mismatch between tool variant and workflow scope triggers retry with actionable guidance

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/toolsets/resource_context.py` | New `GetClusterResourceContextTool.__init__`, `_invoke`, `_invoke_async`; rename `GetResourceContextTool` → `GetNamespacedResourceContextTool`, update tool name string | ~80 new, ~10 modified |
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | `_inject_target_resource` — conditional injection based on `resource_scope` + workflow schema declaration; new `_build_validation_guard_nudge` | ~30 modified, ~15 new |
| `holmesgpt-api/src/validation/workflow_response_validator.py` | Remove `_validate_canonical_params` (Step 0); modify `HAPI_MANAGED_PARAMS` usage | ~30 removed/modified |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Phase 3b — both tools with when-to-use guidance | ~15 modified |
| `holmesgpt-api/src/extensions/llm_config.py` | `register_resource_context_toolset` — register both tools in toolset | ~10 modified |

### E2E-Testable Code (full pipeline, regression)

| Component | What is tested |
|-----------|----------------|
| Existing deployment-scoped E2E | Validates rename from `get_resource_context` → `get_namespaced_resource_context` does not break deployment-scoped flow |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #524-A | Cluster-scoped tool returns resource as root_owner with no namespace | P0 | Unit | UT-HAPI-524-001 | Pending |
| #524-A | Cluster-scoped tool stores `resource_scope: "cluster"` in session_state | P0 | Unit | UT-HAPI-524-002 | Pending |
| #524-A | Cluster-scoped tool skips owner chain walk (Node is root_owner) | P0 | Unit | UT-HAPI-524-003 | Pending |
| #524-A | Cluster-scoped tool handles resource-not-found gracefully | P1 | Unit | UT-HAPI-524-004 | Pending |
| #524-A | Cluster-scoped tool computes spec hash for cluster-scoped resource | P1 | Unit | UT-HAPI-524-005 | Pending |
| #524-A | Cluster-scoped tool fetches remediation history without namespace | P1 | Unit | UT-HAPI-524-006 | Pending |
| #524-A | Cluster-scoped tool detects labels with workload defaults for cluster-scoped | P2 | Unit | UT-HAPI-524-007 | Pending |
| #524 | Renamed tool `get_namespaced_resource_context` has correct name attribute | P0 | Unit | UT-HAPI-524-010 | Pending |
| #524 | Renamed tool stores `resource_scope: "namespaced"` in session_state | P0 | Unit | UT-HAPI-524-011 | Pending |
| #524 | Renamed tool behavior is identical to old `get_resource_context` | P0 | Unit | UT-HAPI-524-012 | Pending |
| #524 | Toolset contains both tools (namespaced + cluster) | P0 | Unit | UT-HAPI-524-013 | Pending |
| #524-C | Validator passes workflow missing TARGET_RESOURCE_NAMESPACE | P0 | Unit | UT-HAPI-524-020 | Pending |
| #524-C | Validator passes workflow missing all 3 canonical params | P0 | Unit | UT-HAPI-524-021 | Pending |
| #524-C | Validator passes workflow declaring only TARGET_RESOURCE_NAME + KIND | P0 | Unit | UT-HAPI-524-022 | Pending |
| #524-C | Validator still skips required-check for HAPI_MANAGED_PARAMS | P0 | Unit | UT-HAPI-524-023 | Pending |
| #524-B | Injection skips TARGET_RESOURCE_NAMESPACE when resource_scope is "cluster" | P0 | Unit | UT-HAPI-524-030 | Pending |
| #524-B | Injection skips TARGET_RESOURCE_NAMESPACE when workflow schema doesn't declare it | P0 | Unit | UT-HAPI-524-031 | Pending |
| #524-B | Injection populates all 3 params for namespaced scope with full schema | P0 | Unit | UT-HAPI-524-032 | Pending |
| #524-B | Injection populates only declared params (NAME + KIND only) | P0 | Unit | UT-HAPI-524-033 | Pending |
| #524-B | Injection populates zero params when workflow declares none of the 3 | P1 | Unit | UT-HAPI-524-034 | Pending |
| #524-B | affectedResource namespace omitted for cluster-scoped root_owner | P0 | Unit | UT-HAPI-524-035 | Pending |
| #524 | Prompt contains `get_namespaced_resource_context` (not old name) | P0 | Unit | UT-HAPI-524-040 | Pending |
| #524 | Prompt contains `get_cluster_resource_context` | P0 | Unit | UT-HAPI-524-041 | Pending |
| #524 | Prompt does not contain old `get_resource_context` tool name | P0 | Unit | UT-HAPI-524-042 | Pending |
| #524 | Prompt explains when to use each tool (namespaced vs cluster-scoped) | P1 | Unit | UT-HAPI-524-043 | Pending |
| #524-5 | Validation guard: node-scoped workflow + namespaced tool → retry nudge | P0 | Unit | UT-HAPI-524-050 | Pending |
| #524-5 | Validation guard: node-scoped workflow + cluster tool → no nudge | P0 | Unit | UT-HAPI-524-051 | Pending |
| #524-5 | Validation guard: deployment-scoped workflow + namespaced tool → no nudge | P0 | Unit | UT-HAPI-524-052 | Pending |
| #524-5 | Validation guard: resource_scope missing from session → no nudge (graceful) | P1 | Unit | UT-HAPI-524-053 | Pending |
| #524 | Registration: both tools injected into SDK toolset manager | P1 | Unit | UT-HAPI-524-060 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios — TDD Execution Plan

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `HAPI` (HolmesGPT API)
- **ISSUE**: 524
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `resource_context.py` (tools), `llm_integration.py` (injection + guard), `workflow_response_validator.py` (relaxed validation), `prompt_builder.py` (prompt), `llm_config.py` (registration) — target >=80% of new/modified logic

---

#### TDD Group 1: Cluster-Scoped Resource Context Tool

**File under test**: `holmesgpt-api/src/toolsets/resource_context.py`
**Test file**: `holmesgpt-api/tests/unit/test_resource_context_tool.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-001` | When the LLM calls `get_cluster_resource_context(kind="Node", name="worker-3")`, the tool returns `root_owner: {kind: "Node", name: "worker-3"}` with no namespace field — the Node IS the root_owner, no owner chain walk |
| `UT-HAPI-524-002` | After calling `get_cluster_resource_context`, `session_state["resource_scope"]` is `"cluster"` — downstream injection logic knows to skip namespace |
| `UT-HAPI-524-003` | Cluster-scoped tool does NOT call `resolve_owner_chain` — Nodes, PVs have no ownerReferences to walk |
| `UT-HAPI-524-004` | When K8s API returns resource-not-found for a cluster-scoped resource, the tool returns a graceful error (not a crash) |
| `UT-HAPI-524-005` | Cluster-scoped tool calls `compute_spec_hash` for the cluster resource to support remediation history correlation |
| `UT-HAPI-524-006` | Cluster-scoped tool passes `resource_namespace=""` to history_fetcher — history lookup for cluster-scoped resources uses `kind/name` only |
| `UT-HAPI-524-007` | Cluster-scoped tool label detection produces workload defaults (`false` for gitOpsManaged, helmManaged, stateful, etc.) since Nodes are not workload-managed |

All 7 tests fail because `GetClusterResourceContextTool` does not exist yet.

##### GREEN Phase — Minimal implementation

Implement `GetClusterResourceContextTool` in `resource_context.py`:
1. Parameters: `kind` (required), `name` (required) — no `namespace` parameter
2. `_invoke_async`: Skip `resolve_owner_chain`. GET the resource directly via `_k8s_client._get_resource_metadata(kind, name, "")`. Construct `root_owner: {kind, name}` (no namespace key).
3. Store `session_state["root_owner"] = root_owner` and `session_state["resource_scope"] = "cluster"`
4. Compute spec hash via `compute_spec_hash(kind, name, "")`
5. Fetch history via `history_fetcher(resource_kind=kind, resource_name=name, resource_namespace="", current_spec_hash=hash)`
6. Run label detection (one-shot, same as namespaced tool)
7. Return `{root_owner, remediation_history, detected_infrastructure?}`

All 7 tests pass.

##### REFACTOR Phase

- Extract shared result-building logic between namespaced and cluster tools into a common `_build_result` helper
- Ensure logging events use `"cluster_resource_context_resolved"` for audibility

---

#### TDD Group 2: Tool Rename (`get_resource_context` → `get_namespaced_resource_context`)

**File under test**: `holmesgpt-api/src/toolsets/resource_context.py`
**Test file**: `holmesgpt-api/tests/unit/test_resource_context_tool.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-010` | The namespaced tool's `name` attribute is `"get_namespaced_resource_context"` (not the old `"get_resource_context"`) — LLM sees the new name in its tool list |
| `UT-HAPI-524-011` | After calling `get_namespaced_resource_context`, `session_state["resource_scope"]` is `"namespaced"` — injection logic knows to include namespace |
| `UT-HAPI-524-012` | The renamed tool's behavior (owner chain resolution, root_owner, history, labels) is identical to the old `get_resource_context` — no regression |
| `UT-HAPI-524-013` | `ResourceContextToolset.tools` contains exactly 2 tools: `get_namespaced_resource_context` and `get_cluster_resource_context` |

UT-524-010 fails because tool name is still `"get_resource_context"`. UT-524-011 fails because `resource_scope` is not written to session_state. UT-524-013 fails because toolset has only 1 tool.

##### GREEN Phase — Minimal implementation

1. Rename class `GetResourceContextTool` → `GetNamespacedResourceContextTool`
2. Change `name="get_resource_context"` → `name="get_namespaced_resource_context"` in `__init__`
3. Add `self._session_state["resource_scope"] = "namespaced"` in `_invoke_async` after storing root_owner
4. Update `ResourceContextToolset.__init__` to create both tools and pass `tools=[namespaced_tool, cluster_tool]`

All 4 tests pass.

##### REFACTOR Phase

- Update tool descriptions to emphasize "namespaced resources" for the renamed tool
- Ensure `get_parameterized_one_liner` reflects the new tool name

---

#### TDD Group 3: Relaxed Canonical Param Validation

**File under test**: `holmesgpt-api/src/validation/workflow_response_validator.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-020` | Workflow declaring only TARGET_RESOURCE_NAME and TARGET_RESOURCE_KIND (no NAMESPACE) passes validation — cluster-scoped workflows are valid |
| `UT-HAPI-524-021` | Workflow declaring zero canonical params passes validation — minimal workflows that don't need target identity are valid |
| `UT-HAPI-524-022` | Workflow declaring only TARGET_RESOURCE_NAME and TARGET_RESOURCE_KIND with operational params passes validation — mixed schema is valid |
| `UT-HAPI-524-023` | Required-check is still skipped for any declared HAPI_MANAGED_PARAMS — LLM is not expected to provide values for these |

UT-524-020 and UT-524-021 fail because `_validate_canonical_params` (Step 0) rejects workflows missing any of the 3 mandatory params.

##### GREEN Phase — Minimal implementation

1. Remove the `_validate_canonical_params` method entirely
2. Remove the Step 0 call in `validate()` (lines 180-182)
3. Keep `HAPI_MANAGED_PARAMS` frozenset — still used by Step 3 to skip required-check and by schema stripping

All 4 tests pass. Existing tests UT-HAPI-496-007 through 009 (mandatory rejection) become **obsolete** and must be updated to expect `is_valid=True`.

##### REFACTOR Phase

- Remove orphaned test assertions that expected mandatory rejection
- Update test docstrings to reflect new contract ("optional declaration")

---

#### TDD Group 4: Conditional Injection Logic

**File under test**: `holmesgpt-api/src/extensions/incident/llm_integration.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-030` | When `resource_scope` is `"cluster"`, injection skips TARGET_RESOURCE_NAMESPACE — Nodes have no namespace, and the workflow parameter is not populated |
| `UT-HAPI-524-031` | When workflow schema does not declare TARGET_RESOURCE_NAMESPACE (regardless of scope), injection skips it — respects workflow contract |
| `UT-HAPI-524-032` | When `resource_scope` is `"namespaced"` and workflow declares all 3 params, all 3 are injected — full backward compatibility |
| `UT-HAPI-524-033` | When workflow declares only NAME and KIND (no NAMESPACE), injection populates only those 2 — partial schema is honored |
| `UT-HAPI-524-034` | When workflow declares none of the 3 canonical params, no TARGET_RESOURCE_* are injected — workflow doesn't need target identity |
| `UT-HAPI-524-035` | For cluster-scoped root_owner, `affectedResource` dict has `kind` and `name` but no `namespace` key — Go sees correct shape |

UT-524-030 and UT-524-031 fail because current injection unconditionally injects namespace when `ns` is truthy. UT-524-033 and UT-524-034 fail because current injection doesn't check workflow schema.

##### GREEN Phase — Minimal implementation

Modify `_inject_target_resource`:
1. Accept optional `workflow_schema` parameter (list of param defs from DS, passed by caller)
2. Build `declared_params = {p["name"] for p in workflow_schema}` if schema available, else fall back to injecting all applicable params
3. Read `resource_scope = session_state.get("resource_scope", "namespaced")`
4. For each of NAME, KIND: inject if declared in schema (or no schema available)
5. For NAMESPACE: inject only if `resource_scope == "namespaced"` AND param is declared in schema (or no schema available) AND `ns` is truthy

All 6 tests pass.

##### REFACTOR Phase

- Extract param-declaration check into a helper `_should_inject_param(param_name, declared_params, resource_scope)`
- Add structured logging for which params were injected vs skipped

---

#### TDD Group 5: Prompt Update

**File under test**: `holmesgpt-api/src/extensions/incident/prompt_builder.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-040` | Prompt contains `get_namespaced_resource_context` — LLM is instructed to use the renamed tool for namespaced resources |
| `UT-HAPI-524-041` | Prompt contains `get_cluster_resource_context` — LLM is instructed to use the new tool for cluster-scoped resources |
| `UT-HAPI-524-042` | Prompt does NOT contain bare `get_resource_context` (the old tool name) — prevents LLM from calling a nonexistent tool |
| `UT-HAPI-524-043` | Prompt contains guidance distinguishing namespaced vs cluster-scoped resource selection (e.g., "Node", "PV", "Namespace" examples) |

UT-524-040 fails because prompt still says `get_resource_context`. UT-524-041 fails because cluster tool is not mentioned. UT-524-042 fails because old name is still present.

##### GREEN Phase — Minimal implementation

Update Phase 3b in `prompt_builder.py`:
- Replace single tool instruction with dual-tool guidance
- Include namespaced examples (Deployment, StatefulSet) and cluster-scoped examples (Node, PV, Namespace)
- Remove the old `get_resource_context` name entirely

All 4 tests pass. Existing test UT-HAPI-496-016 (asserts `get_resource_context` in prompt) must be updated to assert `get_namespaced_resource_context`.

##### REFACTOR Phase

- Verify prompt text is clear and concise for LLM consumption
- Ensure no orphaned references to old tool name remain

---

#### TDD Group 6: Post-Selection Validation Guard

**File under test**: `holmesgpt-api/src/extensions/incident/llm_integration.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-050` | When a node-scoped workflow (actionType=RemoveTaint) is selected but `resource_scope` is `"namespaced"`, validation produces a nudge message directing the LLM to call `get_cluster_resource_context` |
| `UT-HAPI-524-051` | When a node-scoped workflow is selected and `resource_scope` is `"cluster"`, no nudge is produced — tool/workflow match is correct |
| `UT-HAPI-524-052` | When a deployment-scoped workflow (actionType=RollbackDeployment) is selected and `resource_scope` is `"namespaced"`, no nudge is produced — tool/workflow match is correct |
| `UT-HAPI-524-053` | When `resource_scope` is missing from session_state, no nudge is produced — graceful degradation, don't block on missing metadata |

All 4 tests fail because validation guard logic does not exist.

##### GREEN Phase — Minimal implementation

Add `_check_scope_mismatch(result, session_state)` in `llm_integration.py`:
1. If `session_state.get("resource_scope")` is None → return None (no nudge)
2. Extract `action_type` from `result.get("selected_workflow", {}).get("action_type", "")`
3. Define `NODE_SCOPED_ACTION_TYPES = {"RemoveTaint", "CordonDrain"}` (extensible set)
4. If `action_type in NODE_SCOPED_ACTION_TYPES` and `resource_scope == "namespaced"` → return nudge string
5. Otherwise → return None

Call `_check_scope_mismatch` after `_inject_target_resource` in `analyze_incident`. If nudge is returned, set `needs_human_review=True` and `human_review_reason="scope_mismatch"`, and log a warning. (Future: integrate with self-correction loop for retry.)

All 4 tests pass.

##### REFACTOR Phase

- Make `NODE_SCOPED_ACTION_TYPES` configurable or loaded from DS taxonomy
- Add structured logging event `"scope_mismatch_detected"`

---

#### TDD Group 7: Toolset Registration

**File under test**: `holmesgpt-api/src/extensions/llm_config.py`
**Test file**: `holmesgpt-api/tests/unit/test_session_state_wiring.py`

##### RED Phase — Write failing test

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-524-060` | After registration, the SDK toolset manager contains a toolset named `"resource_context"` with 2 tools: `get_namespaced_resource_context` and `get_cluster_resource_context` |

Test fails because toolset currently contains only 1 tool.

##### GREEN Phase — Minimal implementation

`register_resource_context_toolset` already creates `ResourceContextToolset(...)`. Since Group 2 updates the toolset to contain both tools, this test passes after Group 2's implementation.

##### REFACTOR Phase

- Verify `session_state` is shared across both tools within the toolset

---

#### Obsolete Tests (must update)

The following existing tests from BR-496 are **superseded** by #524 and must be updated:

| Old Test ID | Old Assertion | New Assertion |
|-------------|---------------|---------------|
| `UT-HAPI-496-007` | Validator rejects workflow missing TARGET_RESOURCE_NAME | Validator **passes** workflow missing TARGET_RESOURCE_NAME |
| `UT-HAPI-496-008` | Validator rejects workflow missing TARGET_RESOURCE_KIND | Validator **passes** workflow missing TARGET_RESOURCE_KIND |
| `UT-HAPI-496-009` | Validator rejects workflow missing TARGET_RESOURCE_NAMESPACE | Validator **passes** workflow missing TARGET_RESOURCE_NAMESPACE |
| `UT-HAPI-496-016` | Prompt contains `get_resource_context` | Prompt contains `get_namespaced_resource_context` |

These tests are not deleted — their assertions are updated to reflect the new contract.

---

## 6. Test Cases (Detail)

### UT-HAPI-524-001: Cluster-scoped tool returns Node as root_owner

**BR**: #524-A
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resource_context_tool.py`

**Given**: A mock K8s client where `_get_resource_metadata("Node", "worker-3", "")` returns a Node object with no ownerReferences
**When**: `GetClusterResourceContextTool._invoke_async(kind="Node", name="worker-3")` is called
**Then**: Result data contains `root_owner: {kind: "Node", name: "worker-3"}` with NO `namespace` key

**Acceptance Criteria**:
- `root_owner["kind"]` == "Node"
- `root_owner["name"]` == "worker-3"
- `"namespace"` key is NOT present in `root_owner`
- `resolve_owner_chain` is NOT called

### UT-HAPI-524-002: Cluster-scoped tool stores resource_scope

**BR**: #524-A
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_resource_context_tool.py`

**Given**: A session_state dict and a mock K8s client returning a Node
**When**: `GetClusterResourceContextTool._invoke_async(kind="Node", name="worker-3")` is called
**Then**: `session_state["resource_scope"]` == `"cluster"` and `session_state["root_owner"]` == `{kind: "Node", name: "worker-3"}`

**Acceptance Criteria**:
- Both keys are present in session_state
- `resource_scope` is exactly the string `"cluster"`

### UT-HAPI-524-030: Injection skips namespace for cluster scope

**BR**: #524-B
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: `session_state` with `resource_scope: "cluster"` and `root_owner: {kind: "Node", name: "worker-3"}`. Result with `selected_workflow` whose schema declares TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, and TARGET_RESOURCE_NAMESPACE.
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `parameters["TARGET_RESOURCE_NAME"]` == "worker-3", `parameters["TARGET_RESOURCE_KIND"]` == "Node", `"TARGET_RESOURCE_NAMESPACE"` is NOT in parameters

**Acceptance Criteria**:
- NAME and KIND are injected from root_owner
- NAMESPACE is NOT injected despite being declared in schema (cluster scope overrides)
- `affectedResource` has kind and name but no namespace key

### UT-HAPI-524-031: Injection skips namespace when schema doesn't declare it

**BR**: #524-B
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: `session_state` with `resource_scope: "namespaced"` and `root_owner: {kind: "Deployment", name: "api", namespace: "prod"}`. Result with `selected_workflow` whose schema declares only TARGET_RESOURCE_NAME and TARGET_RESOURCE_KIND (no NAMESPACE).
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `parameters["TARGET_RESOURCE_NAME"]` == "api", `parameters["TARGET_RESOURCE_KIND"]` == "Deployment", `"TARGET_RESOURCE_NAMESPACE"` is NOT in parameters

**Acceptance Criteria**:
- NAME and KIND are injected
- NAMESPACE is NOT injected because workflow doesn't declare it (even though root_owner has one)

### UT-HAPI-524-050: Validation guard detects scope mismatch

**BR**: #524-5
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: `session_state` with `resource_scope: "namespaced"`. Result with `selected_workflow: {action_type: "RemoveTaint"}`.
**When**: `_check_scope_mismatch(result, session_state)` is called
**Then**: Returns a non-None nudge string containing `get_cluster_resource_context`

**Acceptance Criteria**:
- Nudge message mentions `get_cluster_resource_context`
- Nudge message mentions that the workflow targets a Node
- Function returns a string (not None)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Python pytest + pytest-asyncio (HAPI convention)
- **Mocks**: `unittest.mock.AsyncMock` for K8s client, `MagicMock` for history_fetcher
- **Location**: `holmesgpt-api/tests/unit/`
- **Anti-patterns avoided**:
  - No `time.sleep()` — all assertions are synchronous or use `await`
  - No `Skip()` / `pytest.skip()` — all tests are implemented
  - No mocking of business logic — only external deps (K8s API, history API)
  - No testing audit infrastructure directly — test business outcomes that produce audit side effects

---

## 8. Execution

```bash
# All HAPI unit tests
cd holmesgpt-api && python -m pytest tests/unit/ -v

# Specific test by ID pattern
python -m pytest tests/unit/test_resource_context_tool.py -v -k "524"

# Injection + validation tests
python -m pytest tests/unit/test_target_resource_injection.py -v -k "524"

# Full regression (existing + new)
python -m pytest tests/unit/ -v --tb=short
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for #524 |
