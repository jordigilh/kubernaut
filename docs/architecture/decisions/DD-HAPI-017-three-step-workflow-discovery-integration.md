# DD-HAPI-017: Three-Step Workflow Discovery Integration

**Status**: ✅ APPROVED
**Decision Date**: 2026-02-05
**Version**: 1.4
**Confidence**: 92%
**Applies To**: HolmesGPT API (HAPI), DataStorage Service (DS)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-05 | Architecture Team | Initial design: replace `search_workflow_catalog` with three-step discovery tools for both incident and recovery flows. No migration -- pre-release greenfield replacement. |
| 1.1 | 2026-02-15 | Architecture Team | Remove `actualSuccessRate` and `totalExecutions` from `list_workflows` LLM-facing response. These global aggregate metrics are misleading for per-incident workflow selection (Section 7). Clarify that contextual remediation history via spec-hash matching (DD-HAPI-016) is the correct mechanism for outcome-aware decisions. |
| 1.2 | 2026-02-12 | Architecture Team | ADR-056 integration: Add Section 8 (tool-level flow enforcement requiring `get_resource_context` before workflow discovery). Remove `detected_labels` from LLM-facing tool parameters -- labels are now computed by HAPI internally and injected into DS queries transparently. Update BR-HAPI-017-001. Add DD-HAPI-018 cross-reference. |
| 1.3 | 2026-02-20 | Architecture Team | Surface detected labels to LLM as read-only `cluster_context` in `list_available_actions` tool response (Step 1). Labels remain HAPI-computed and are NOT LLM-managed parameters. The LLM receives them for informed action type reasoning (e.g., GitOps-managed, HPA-enabled). Add BR-HAPI-017-007. Update Section 8 prompt builder impact. See ADR-056 v1.3. |
| 1.4 | 2026-02-20 | Architecture Team | Phase 1 implementation: label detection moves from workflow_discovery (signal source) to `get_resource_context` (RCA target). When active labels are detected, `get_resource_context` returns `detected_infrastructure` for one-shot LLM RCA reassessment. Second calls resolve new target but skip label re-detection. Add BR-HAPI-017-008. Update Section 8 enforcement flow. See ADR-056 v1.4. |

---

## Context & Problem

### Current State

HAPI exposes a single LLM tool, `search_workflow_catalog`, implemented in `holmesgpt-api/src/toolsets/workflow_catalog.py` (class `SearchWorkflowCatalogTool`). This tool:

1. Accepts a structured query (`<signal_type> <severity> [keywords]`) and optional filters
2. Calls DS via `POST /api/v1/workflows/search` with label-based + semantic search
3. Returns a ranked list of workflows with confidence scores
4. Is registered in both the incident flow (`holmesgpt-api/src/extensions/incident/llm_integration.py`) and the recovery flow (`holmesgpt-api/src/extensions/recovery/llm_integration.py`)

### Problems

1. **Action/workflow conflation**: The LLM must select both the remediation action type and the specific workflow in a single search call. It has no structured way to first understand what categories of actions are available, then drill into specific workflows within a category.

2. **No comprehensive review**: The tool returns a ranked list, and the LLM typically picks the first result. There is no mechanism to force the LLM to review all available workflows before deciding -- leading to premature selection of less effective remediations.

3. **Semantic search noise**: The `signal_type`-based query format produces embedding similarity results that may not align with the structured `action_type` taxonomy introduced in DD-WORKFLOW-016. The taxonomy provides deterministic, curated descriptions that are more reliable than semantic matching.

4. **Recovery flow has no validation**: The incident flow validates the LLM's workflow selection with a 3-attempt self-correction loop (`WorkflowResponseValidator` + `MAX_VALIDATION_ATTEMPTS = 3`). The recovery flow uses the same tool but has no validation, creating an asymmetry where recovery remediations may execute with invalid parameters.

### Business Requirements

- **BR-WORKFLOW-016-002**: ListAvailableActions HAPI tool
- **BR-WORKFLOW-016-005**: ListWorkflows HAPI tool
- **BR-WORKFLOW-016-003**: GetWorkflow parameter lookup with security gate
- **BR-WORKFLOW-016-004**: LLM three-step discovery protocol in HAPI prompts

---

## Alternatives Considered

### Alternative 1: Coexistence (deploy new tools alongside old)

**Approach**: Deploy `list_available_actions`, `list_workflows`, and `get_workflow` alongside the existing `search_workflow_catalog`. Deprecate the old tool after validation in production.

**Pros**:
- Rollback path if new tools cause regressions
- Gradual migration reduces risk

**Cons**:
- LLM sees four tools instead of three, increasing cognitive load and potential for tool misuse
- Two code paths to maintain during coexistence window
- Prompt instructions must handle both old and new flows simultaneously

**Confidence**: 40% (rejected)

---

### Alternative 2: Feature flag toggle

**Approach**: Use a configuration flag to switch between old single-tool and new three-step tools at runtime.

**Pros**:
- Instant rollback via config change
- A/B testing possible

**Cons**:
- Two complete code paths in production
- Prompt builder must generate two different instruction sets
- Testing matrix doubles (old path + new path)

**Confidence**: 50% (rejected)

---

### Alternative 3: Direct replacement (APPROVED)

**Approach**: Remove `search_workflow_catalog` entirely. Replace with three new tools (`list_available_actions`, `list_workflows`, `get_workflow`). Apply to both incident and recovery flows. No coexistence period.

**Pros**:
- Single code path -- simplest to maintain and test
- LLM prompt contract is clear and unambiguous (three tools, one purpose each)
- No risk of LLM using deprecated tool
- Pre-release product -- no deployed consumers to break

**Cons**:
- No rollback to old behavior without code revert
- Both incident and recovery flows change simultaneously

**Confidence**: 92% (approved)

**Mitigation for cons**: Pre-release product with no production deployment. If issues arise, standard git revert is sufficient.

---

## Decision

**APPROVED: Alternative 3** -- Direct replacement of `search_workflow_catalog` with three-step discovery tools for both incident and recovery flows.

**Rationale**:
1. **Pre-release product**: No backward compatibility constraints. No deployed consumers.
2. **Reduced LLM cognitive load**: Three focused tools with clear responsibilities outperform one overloaded tool.
3. **Comprehensive workflow review**: Step 2 forces the LLM to review all workflows before selecting, preventing premature selection.
4. **Recovery flow parity**: Both flows should have the same discovery quality and validation guarantees.

**Key Insight**: The product has not been released. There are no migration, coexistence, or deploy-order concerns -- this is a greenfield replacement.

---

## Design

### 1. Tool Replacement

Three new tool classes replace `SearchWorkflowCatalogTool`:

| Removed Tool | New Tool | DS Endpoint | Responsibility |
|-------------|----------|-------------|----------------|
| `search_workflow_catalog` | `list_available_actions` | `GET /api/v1/workflows/actions` | Action type discovery from taxonomy |
| `search_workflow_catalog` | `list_workflows` | `GET /api/v1/workflows/actions/{action_type}` | Workflow selection within action type |
| `search_workflow_catalog` | `get_workflow` | `GET /api/v1/workflows/{workflow_id}` | Single workflow parameter schema lookup |

Each tool:
- Extends `Tool` with its own parameters, description, and `_invoke` method
- Passes signal context filters (severity, component, environment, priority, custom_labels) to DS
- **v1.2 (ADR-056)**: `detected_labels` is NOT an LLM-facing parameter. HAPI reads `detected_labels` from internal session state (populated by `get_resource_context`) and injects them into DS queries transparently. See Section 8.
- **v1.3**: `list_available_actions` (Step 1) surfaces detected labels as a read-only `cluster_context` section in the tool response. This gives the LLM explicit infrastructure context for action type selection without making labels a tool parameter. Steps 2 and 3 do not include `cluster_context`.
- Passes `remediationId` as a query parameter for audit correlation (BR-AUDIT-021)
- Returns a `StructuredToolResult` with clean LLM-rendered output (scores stripped)

The existing `SearchWorkflowCatalogTool` and `WorkflowCatalogToolset` classes are removed. A new toolset class registers all three tools for both incident and recovery flows.

The tool specifications (parameters, descriptions, DS endpoint contracts, pagination behavior) are defined authoritatively in DD-WORKFLOW-016. This DD does not redefine them.

### 2. Prompt Template Updates

Both prompt builders are updated to reference the three-step protocol:

**Affected files**:
- `holmesgpt-api/src/extensions/incident/prompt_builder.py`
- `holmesgpt-api/src/extensions/recovery/prompt_builder.py`

**Changes**:
- All references to `search_workflow_catalog` are removed
- New MCP filter instructions reference `list_available_actions`, `list_workflows`, `get_workflow`
- Three-step flow instructions are added per the DD-WORKFLOW-016 prompt contract:
  1. Complete RCA investigation first
  2. Call `list_available_actions` to discover action types
  3. Call `list_workflows` for the selected action type -- review ALL workflows before selecting
  4. Call `get_workflow` to retrieve the parameter schema for the selected workflow
  5. Populate parameters from RCA context
- Step 2 pagination instruction: LLM MUST request all pages before making a decision
- The DD-LLM-001 query format (`<signal_type> <severity>`) is superseded for workflow discovery -- the three tools use structured parameters, not free-text queries

### 3. DS Client Regeneration

HAPI's Python DS client (`holmesgpt-api/src/clients/datastorage/`) is generated from `api/openapi/data-storage-v1.yaml`. After the DS OpenAPI spec is updated to include the three new endpoints (per DD-WORKFLOW-016), the Python client is regenerated.

New client methods (generated):
- `list_available_actions(severity, component, environment, priority, custom_labels, detected_labels, remediation_id, offset, limit)`
- `list_workflows(action_type, severity, component, environment, priority, custom_labels, detected_labels, remediation_id, offset, limit)`
- `get_workflow(workflow_id, severity, component, environment, priority, custom_labels, detected_labels, remediation_id)`

**v1.2 note (ADR-056)**: The generated client methods still accept `detected_labels` as a parameter (the DS API requires it for filtering). However, HAPI tool implementations populate this parameter from internal session state, not from LLM input. The LLM tool schemas (`list_available_actions`, `list_workflows`, `get_workflow`) do NOT expose `detected_labels` as an LLM-facing parameter.

The Go ogen client (`pkg/datastorage/ogen-client/`) is also regenerated from the same spec.

### 4. Post-Selection Validation

**Current state**: `WorkflowResponseValidator` (`holmesgpt-api/src/validation/workflow_response_validator.py`) calls `get_workflow_by_id(workflow_id)` without signal context filters. The incident flow has a self-correction loop (`MAX_VALIDATION_ATTEMPTS = 3`) that retries on validation failure with feedback. The recovery flow has no validation.

**New design**:

- `WorkflowResponseValidator` calls `get_workflow(workflow_id, severity, component, environment, ...)` with full signal context filters, activating the DS security gate (DD-WORKFLOW-016). If the workflow doesn't match the context filters, DS returns 404 (DD-WORKFLOW-017 Phase 4).
- The recovery flow gains the same self-correction loop as the incident flow: `MAX_VALIDATION_ATTEMPTS = 3`, with feedback injected into the prompt on each retry.

**Why recovery needs validation**: Without validation, the recovery flow can execute workflows with invalid parameters, wrong action types, or workflows that don't match the signal context. This creates a safety gap where recovery remediations -- which are supposed to fix previously failed remediations -- may themselves fail due to preventable errors. Parity with the incident flow closes this gap.

### 5. Error Handling

Each new tool handles errors from the DS discovery endpoints:

| HTTP Status | Error Type (DD-004) | Tool Behavior |
|-------------|---------------------|---------------|
| 200 | -- | Return structured result to LLM |
| 400 | `validation-error` | Return error to LLM with detail message |
| 404 | `workflow-not-found` | Return error to LLM (Step 3 security gate failure) |
| 500 | `internal-error` | Raise exception, fail the tool invocation |
| 502/503/504 | `service-unavailable` | Raise exception with timeout/connectivity context |

Pagination handling (`hasMore` responses) is built into the `list_available_actions` and `list_workflows` tool implementations. The tool returns the pagination metadata to the LLM so it can request subsequent pages.

### 6. DS-Side Impact

The DS endpoint changes required by this design are documented here for blast radius awareness. DS implementation details are not prescribed by this DD.

**Endpoints removed**:
- `POST /api/v1/workflows/search` -- replaced by three GET endpoints per DD-WORKFLOW-016

**Endpoints added** (per DD-WORKFLOW-016):
- `GET /api/v1/workflows/actions` -- action type discovery with context filters
- `GET /api/v1/workflows/actions/{action_type}` -- workflow listing with context filters
- `GET /api/v1/workflows/{workflow_id}` -- gains context filter query parameters for security gate (extends existing endpoint)

**Audit events replaced** (per DD-WORKFLOW-014 v3.0):
- `workflow.catalog.search_completed` removed
- `workflow.catalog.actions_listed`, `workflow.catalog.workflows_listed`, `workflow.catalog.workflow_retrieved`, `workflow.catalog.selection_validated` added

**OpenAPI spec**: `api/openapi/data-storage-v1.yaml` must be updated before client regeneration.

**Dead code**: `HandleListWorkflowVersions` handler and `GetVersionsByName` repository function have no consumers and no OpenAPI definition. They should be removed.

### 7. LLM-Facing Data Boundaries: No Aggregate Success Metrics (v1.1)

**Decision**: The `list_workflows` response (Step 2) MUST NOT include `actualSuccessRate` or `totalExecutions` in the LLM-facing discovery flow. These fields are removed from the `WorkflowDiscoveryEntry` schema used by HAPI tools and the corresponding prompt instructions.

**Rationale**:

`actualSuccessRate` and `totalExecutions` are **global aggregate metrics** computed across all past executions of a workflow — across different signals, different target resources, different environments, and different root causes. Exposing them to the LLM during per-incident workflow selection creates several problems:

1. **Contextual irrelevance**: An 85% success rate measured against OOM signals in staging has no predictive value when the LLM is evaluating the workflow for a CrashLoopBackOff on a different Deployment in production. The conditions under which these figures were collected are not applicable to the current case.

2. **Selection bias**: The LLM will favor high-execution, high-success-rate workflows even when those stats are irrelevant to the current incident context. A workflow that is a perfect match for the current case but is newly registered (0 executions, no success rate) would be unfairly penalized.

3. **False confidence**: Aggregate success rates can mask important context — a workflow might have 95% success on simple cases but 20% success on the specific class of failure the LLM is currently investigating. The aggregate hides this distinction.

**These metrics remain available for operators** via the full `RemediationWorkflow` schema (e.g., `GET /api/v1/workflows/{id}` admin endpoints, dashboard views). They are valuable for operational monitoring and catalog curation — just not for per-incident LLM selection.

**The correct mechanism for outcome-aware LLM decisions** is the **Remediation History Context** (DD-HAPI-016), which provides contextual history scoped to the specific target resource via spec-hash matching:

- **Three-way hash comparison** (`ComputeHashMatch` in `pkg/datastorage/server/remediation_history_logic.go`):
  - `currentSpecHash == preRemediationSpecHash` → **Configuration regression detected** — the resource has reverted to a pre-remediation state, indicating the previous remediation was undone
  - `currentSpecHash == postRemediationSpecHash` → **Configuration unchanged** — the resource spec is the same as after the previous remediation
  - No match → **Spec has drifted** — the resource has been modified by other means since the last remediation

- **Tier 1 (recent history)**: Detailed remediation chain for the same `targetResource` (kind, name, namespace) within a recent time window. Includes: `workflowType`, `outcome` (success/failure/partial), `effectivenessScore`, `signalResolved`, `healthChecks`, `metricDeltas`, `assessmentReason`, `preRemediationSpecHash`, `postRemediationSpecHash`.

- **Tier 2 (wider history)**: Summary entries for the same `preRemediationSpecHash` across a broader time window, catching cases where the same configuration state has been seen before on different resource instances.

- **Causal chain detection** (`_detect_spec_drift_causal_chains` in `holmesgpt-api/src/extensions/remediation_history_prompt.py`): Detects when a `spec_drift` entry's `postRemediationSpecHash` matches a subsequent entry's `preRemediationSpecHash`, proving the drift led to a follow-up remediation.

- **Declining effectiveness trend detection**: Identifies patterns where successive remediations on the same target show decreasing effectiveness scores.

This contextual history is injected into the LLM prompt as the `## Remediation History Context (AUTO-DETECTED)` section before the workflow discovery phase, giving the LLM directly comparable, target-specific evidence to inform its selection rather than misleading global aggregates.

**Prompt changes (v1.1)**:
- Step 2 instruction changed from "Compare success rates, execution history, and descriptions" to "Compare workflow descriptions, version notes, and suitability for your RCA findings"
- Workflow Discovery Guidance updated to remove success rate references
- `ListWorkflowsTool` description and `additional_instructions` updated to remove success rate/execution count references

### 8. Tool-Level Flow Enforcement: `get_resource_context` Prerequisite (v1.2, ADR-056)

**Decision**: HAPI MUST enforce that `get_resource_context` is called before any workflow discovery tool (`list_available_actions`, `list_workflows`, `get_workflow`). This is a tool-level prerequisite check, not prompt-level guidance.

**Rationale**: ADR-056 relocates `DetectedLabels` computation from SP (signal source, pre-RCA) to HAPI (remediation target, post-RCA). The `get_resource_context` tool resolves the root owner and computes `DetectedLabels` for the actual target resource, storing them in internal session state. Workflow discovery tools read these labels from session state and inject them into DS queries. If `get_resource_context` has not been called, the labels are unavailable and workflow discovery would proceed without target-specific filtering -- potentially selecting inappropriate workflows.

Rather than degrading gracefully (proceeding without labels), HAPI enforces the correct flow by returning a structured error to the LLM, following the established self-correction pattern from DD-HAPI-002.

#### Session State Mechanism

A shared mutable Python dict is created per investigation and passed to both `ResourceContextToolset` and `WorkflowDiscoveryToolset` during per-request toolset registration:

```python
# Per-request setup in analyze_incident() / analyze_recovery()
session_state = {}  # shared mutable dict, scoped to one investigation

discovery_toolset = WorkflowDiscoveryToolset(
    ..., session_state=session_state
)
resource_context_toolset = ResourceContextToolset(
    ..., session_state=session_state
)
```

Holmes SDK creates tool instances once per investigation and reuses them for all tool calls within that conversation. The same `session_state` dict reference is accessible to both toolsets.

**No concurrency risk**: Phases 3b (`get_resource_context`) and 4 (workflow discovery) are sequential in the prompt. Holmes SDK only parallelizes tool calls within a single LLM response step, and these tools are in different prompt phases.

#### Enforcement Flow

1. **`get_resource_context._invoke()` (first call)**: After resolving the root owner, builds K8s context and computes labels per DD-HAPI-018. Writes to session state:
   ```python
   self._session_state["detected_labels"] = computed_labels  # DetectedLabels dict
   ```
   If label detection fails entirely (all K8s API queries fail), writes an empty dict as a sentinel:
   ```python
   self._session_state["detected_labels"] = {}  # sentinel: detection attempted, all failed
   ```
   **v1.4 (one-shot reassessment)**: If any label is "active" (boolean `True` or non-empty string, excluding `failedDetections`), includes `detected_infrastructure` in the tool response alongside `root_owner` + `remediation_history`. If all defaults, omits `detected_infrastructure`. The tool's `additional_instructions` guide the LLM to consider the infrastructure context for RCA reassessment.

2. **`get_resource_context._invoke()` (second call, after reassessment)**: If the LLM reassesses and identifies a different target, it may call `get_resource_context` again. The guard check (`"detected_labels" in session_state`) skips label re-detection. The tool resolves `root_owner` + `remediation_history` for the new target but does NOT include `detected_infrastructure` in the response and does NOT overwrite `session_state["detected_labels"]`. This prevents infinite loops.

3. **Discovery tool `_invoke()` methods** (`list_available_actions`, `list_workflows`, `get_workflow`): Read labels from session state (populated by step 1) and inject into DS query parameters transparently. If `session_state` has no `"detected_labels"` key (get_resource_context never called), proceed without label filtering -- graceful degradation.

4. **LLM self-correction**: If the LLM calls workflow discovery before `get_resource_context`, discovery proceeds without label filtering. The `additional_instructions` on discovery tools guide the LLM to call `get_resource_context` first for best results.

#### Failure Modes

| Scenario | Behavior |
|----------|----------|
| LLM calls `list_workflows` before `get_resource_context` | Flow error returned; LLM self-corrects |
| `get_resource_context` succeeds, all labels detected | Labels injected into DS queries normally |
| `get_resource_context` succeeds, some labels fail (RBAC) | `detected_labels` contains partial results + `failedDetections`; DS queries use available labels |
| `get_resource_context` succeeds, all label detections fail | Empty dict sentinel; DS queries proceed without label filters (acceptable -- equivalent to SP's all-fields-failed case) |
| `get_resource_context` itself fails (K8s API unreachable) | Tool returns error to LLM; LLM may retry or proceed to workflow discovery which will also return flow error |

#### Prompt Builder Impact (v1.3)

The prompt builder changes for ADR-056 are:

1. **Remove DetectedLabels from prompt context**: The `build_cluster_context_section()` and `build_mcp_filter_instructions()` functions in `prompt_builder.py` currently inject SP-computed `DetectedLabels` into the prompt. Once ADR-056 is implemented, these sections are removed -- HAPI computes and injects labels internally via `get_resource_context`.

2. **Phase 3b instruction update**: The `get_resource_context` instruction already exists in the prompt (Phase 3b). No change needed to the instruction text -- the tool's internal behavior changes (now also computes labels) but the LLM-facing contract (call with kind/name/namespace, receive root_owner + history) is unchanged.

3. **Phase 4 instruction update**: Remove any references to `detected_labels` as a parameter the LLM should provide. The "Signal context filters are automatically included" note already covers this.

4. **`cluster_context` in `list_available_actions` response (v1.3)**: The `list_available_actions` tool now surfaces computed `detected_labels` as a read-only `cluster_context` section in its response payload. This provides the LLM with infrastructure characteristics (e.g., `gitOpsManaged`, `hpaEnabled`, `pdbProtected`) that inform action type selection reasoning, without requiring the LLM to detect or query these independently. The `additional_instructions` on `ListAvailableActionsTool` explicitly reference the `cluster_context` section.

#### `list_available_actions` Response Schema (v1.3)

When `detected_labels` are available in session state, the response includes:

```json
{
  "actionTypes": [ ... ],
  "cluster_context": {
    "detected_labels": {
      "gitOpsManaged": true,
      "gitOpsTool": "argocd",
      "hpaEnabled": false,
      "pdbProtected": true,
      "helmManaged": false,
      "serviceMesh": "",
      "istioEnabled": false
    },
    "note": "These infrastructure characteristics were auto-detected for the remediation target resource. Consider them when selecting an action type."
  }
}
```

**Key rules**:
- `failedDetections` fields are **fully excluded** from `cluster_context.detected_labels` (the LLM should not reason about labels that could not be detected).
- `False` booleans are **preserved** (unlike DS query parameters which omit `False` values). The LLM benefits from knowing that a feature is explicitly absent (e.g., `hpaEnabled: false` means no HPA).
- If `detected_labels` is empty or not yet computed, `cluster_context` is omitted entirely.
- `cluster_context` appears **only** in `list_available_actions` responses, not in `list_workflows` or `get_workflow`.

---

## Blast Radius

### Production Code

| Component | File | Change | Owner |
|-----------|------|--------|-------|
| HAPI workflow tools | `holmesgpt-api/src/toolsets/workflow_catalog.py` | Replace `SearchWorkflowCatalogTool` with three tools | This DD |
| HAPI incident prompts | `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Rewrite workflow instructions | This DD |
| HAPI recovery prompts | `holmesgpt-api/src/extensions/recovery/prompt_builder.py` | Rewrite workflow instructions | This DD |
| HAPI validator | `holmesgpt-api/src/validation/workflow_response_validator.py` | Add context filters | This DD |
| HAPI recovery flow | `holmesgpt-api/src/extensions/recovery/llm_integration.py` | Add validation loop | This DD |
| DS workflow handlers | `pkg/datastorage/server/workflow_handlers.go` | Endpoint replacement | DD-WORKFLOW-016/017 |
| DS workflow repository | `pkg/datastorage/repository/workflow/` | Query refactoring | DD-WORKFLOW-016/017 |

### Generated Clients (Regeneration Required)

| Client | Location | Source |
|--------|----------|--------|
| Python OpenAPI client | `holmesgpt-api/src/clients/datastorage/` | `api/openapi/data-storage-v1.yaml` |
| Go ogen client | `pkg/datastorage/ogen-client/` | `api/openapi/data-storage-v1.yaml` |

### Test Infrastructure (Discovery Endpoint Changes -- This DD)

| Test | Location | Impact |
|------|----------|--------|
| DS E2E search tests | `test/e2e/datastorage/04_*`, `06_*`, `08_*` | Rewrite for three-step endpoints |
| DS E2E version tests | `test/e2e/datastorage/07_*` | Update search and list calls |
| Python integration tests | `holmesgpt-api/tests/integration/test_workflow_catalog_*.py` | Rewrite for new client methods |
| Python E2E tests | `holmesgpt-api/tests/e2e/test_workflow_catalog_*.py` | Rewrite for new client methods |
| Python test fixtures | `holmesgpt-api/tests/fixtures/workflow_fixtures.py` | Update create/search helpers |

### Test Infrastructure (Registration Payload Changes -- DD-WORKFLOW-017 Scope)

| Test | Location | Impact |
|------|----------|--------|
| Workflow seeding | `test/infrastructure/workflow_seeding.go` | CreateWorkflow payload changes to OCI pullspec |
| Workflow bundles | `test/infrastructure/workflow_bundles.go` | Same registration payload change |
| SAR access control E2E | `test/e2e/datastorage/23_sar_access_control_test.go` | CreateWorkflow payload change |

---

## Consequences

**Positive**:
- LLM workflow selection quality improves through structured three-step discovery
- Recovery flow gains validation parity with incident flow, closing a safety gap
- Single code path for both flows -- simpler to test and maintain
- Audit trail granularity increases from one event to four events per discovery flow
- `action_type` taxonomy replaces noisy semantic search for deterministic matching
- `action_type` is propagated from HAPI `selected_workflow` response into `AIAnalysis.Status.SelectedWorkflow.ActionType`, enabling RO audit events (`remediation.workflow_created`) and DS remediation history to carry the taxonomy value end-to-end (DD-CONTRACT-002, DD-WORKFLOW-016)

**Negative**:
- Recovery flow calls become longer due to validation loop (3 retries max) -- **Mitigation**: Better remediation quality justifies the latency increase; failed validations are rare with structured tools
- All existing workflow-related tests must be rewritten -- **Mitigation**: Pre-release product; test rewrite is a one-time cost

**Neutral**:
- DS client regeneration is a mechanical step after OpenAPI spec update
- DD-LLM-001 query format (`<signal_type> <severity>`) is partially superseded -- still valid for non-workflow tools

---

## Business Requirements

### BR-HAPI-017-001: Three-Step Tool Implementation

- **Category**: HAPI
- **Priority**: P0
- **Description**: MUST implement three LLM tools (`list_available_actions`, `list_workflows`, `get_workflow`) per DD-WORKFLOW-016, registered for both incident and recovery flows.
- **Acceptance Criteria**:
  - Three tool classes implemented, each with correct DS endpoint call and parameter mapping
  - All tools pass signal context filters (severity, component, environment, priority, custom_labels) to DS
  - **v1.2**: `detected_labels` is NOT an LLM-facing parameter. Tools read labels from internal session state (populated by `get_resource_context` per ADR-056) and inject into DS queries transparently
  - **v1.2**: All tools check session state for `get_resource_context` prerequisite (Section 8). If labels are absent, return flow error to LLM for self-correction
  - All tools pass `remediationId` as query parameter for audit correlation
  - Tools return `StructuredToolResult` with clean LLM-rendered output (finalScore stripped)
  - Toolset registered in both incident and recovery `llm_integration.py`
  - **v1.2**: `WorkflowDiscoveryToolset` accepts `session_state` dict shared with `ResourceContextToolset`

### BR-HAPI-017-002: Prompt Template Update

- **Category**: HAPI
- **Priority**: P0
- **Description**: MUST update incident and recovery prompt builders to reference the three-step discovery protocol per DD-WORKFLOW-016 prompt contract. All references to `search_workflow_catalog` are removed.
- **Acceptance Criteria**:
  - Incident prompt builder references `list_available_actions`, `list_workflows`, `get_workflow`
  - Recovery prompt builder references the same three tools
  - Step 2 instruction explicitly requires LLM to review ALL workflows before selecting
  - No references to `search_workflow_catalog` remain in any prompt builder

### BR-HAPI-017-003: Post-Selection Validation with Security Gate

- **Category**: HAPI
- **Priority**: P0
- **Description**: MUST pass full signal context filters to `get_workflow()` during post-selection validation, activating the DS security gate. A 404 response indicates the workflow does not match the signal context.
- **Acceptance Criteria**:
  - `WorkflowResponseValidator` calls `get_workflow` with all signal context filters
  - 404 from DS treated as validation failure (workflow doesn't match context)
  - Validation failure triggers self-correction loop feedback

### BR-HAPI-017-004: Recovery Flow Validation Loop

- **Category**: HAPI
- **Priority**: P0
- **Description**: MUST add the same self-correction validation loop to the recovery flow that exists in the incident flow (`MAX_VALIDATION_ATTEMPTS = 3`).
- **Acceptance Criteria**:
  - Recovery flow validates LLM workflow selection using `WorkflowResponseValidator`
  - On validation failure, feedback is injected into the prompt and the LLM retries
  - Maximum 3 validation attempts before marking as `needs_human_review`
  - `aiagent.workflow.validation_attempt` audit events emitted for each attempt

### BR-HAPI-017-005: remediationId Propagation

- **Category**: HAPI / Audit
- **Priority**: P0
- **Description**: MUST propagate `remediationId` as a query parameter on all three DS discovery calls and the post-selection validation call, enabling DS to emit correlated audit events per BR-AUDIT-021.
- **Acceptance Criteria**:
  - `remediationId` included in all four DS calls (Steps 1-3 + validation)
  - Empty `remediationId` handled gracefully (discovery proceeds, audit has empty correlation)

### BR-HAPI-017-006: search_workflow_catalog Removal

- **Category**: HAPI
- **Priority**: P0
- **Description**: `SearchWorkflowCatalogTool` class, `WorkflowCatalogToolset` class, and all references to `search_workflow_catalog` in prompt builders MUST be removed. No migration or coexistence.
- **Acceptance Criteria**:
  - `SearchWorkflowCatalogTool` class deleted
  - `WorkflowCatalogToolset` class deleted or replaced by new toolset
  - No references to `search_workflow_catalog` in any Python source file
  - `POST /api/v1/workflows/search` no longer called by HAPI

### BR-HAPI-017-007: cluster_context in list_available_actions Response

- **Category**: HAPI
- **Priority**: P1
- **Description**: MUST surface computed `detected_labels` as a read-only `cluster_context` section in the `list_available_actions` tool response, providing infrastructure context to the LLM for informed action type selection. Reverses the v1.2 decision to keep labels fully internal.
- **Acceptance Criteria**:
  - `list_available_actions` response includes `cluster_context.detected_labels` when labels exist in session state
  - `cluster_context.detected_labels` excludes all fields listed in `failedDetections`
  - `False` booleans are preserved in `cluster_context` (not filtered like DS query params)
  - `cluster_context` is omitted when `detected_labels` is empty or not yet computed
  - `cluster_context` includes a `note` field explaining label semantics to the LLM
  - `cluster_context` does NOT appear in `list_workflows` or `get_workflow` responses
  - `ListAvailableActionsTool.additional_instructions` references `cluster_context` for LLM guidance
- **Authority**: ADR-056 v1.3, DD-HAPI-017 v1.3, DD-HAPI-018 v1.1

### BR-HAPI-017-008: One-Shot Reassessment via detected_infrastructure in get_resource_context

- **Category**: HAPI
- **Priority**: P1
- **Description**: When `get_resource_context` computes labels for the RCA target and any labels are "active" (at least one boolean `true` or non-empty string, excluding `failedDetections`), the tool MUST include a `detected_infrastructure` section in its response alongside `root_owner` and `remediation_history`. This enables the LLM to reassess its RCA strategy before entering workflow discovery. Labels are computed once (one-shot); subsequent calls for revised targets resolve context but skip label re-detection.
- **Acceptance Criteria**:
  - `get_resource_context` response includes `detected_infrastructure.labels` when any label is active (boolean `true` or non-empty string)
  - `detected_infrastructure.labels` excludes `failedDetections` fields (same exclusion logic as `cluster_context`)
  - `detected_infrastructure` includes a `note` field guiding the LLM to consider infrastructure context for RCA
  - `detected_infrastructure` is omitted when all labels are defaults (all booleans `false`, all strings empty)
  - `detected_infrastructure` is omitted on second and subsequent `get_resource_context` calls (session_state sentinel prevents re-detection)
  - Second call still resolves `root_owner` + `remediation_history` for the new target
  - `session_state["detected_labels"]` is NOT overwritten on second call
  - Tool `additional_instructions` inform the LLM about reassessment and the no-re-detection rule
- **Authority**: ADR-056 v1.4, DD-HAPI-017 v1.4

---

## Related Decisions

| Document | Relationship |
|----------|-------------|
| **DD-WORKFLOW-016** | Authoritative source for three-step discovery protocol, tool specifications, DS endpoints, SQL, and prompt contract. This DD implements that protocol in HAPI. |
| **DD-WORKFLOW-017** | Workflow lifecycle component interactions. Phase 2 (Discovery) describes the component flow that this DD implements on the HAPI side. |
| **DD-WORKFLOW-014 v3.0** | Audit trail for workflow selection. Defines the four V3.0 step-specific DS audit events. HAPI propagates `remediationId` for correlation. |
| **DD-WORKFLOW-002** | **SUPERSEDED** by DD-WORKFLOW-016. Defined the old `search_workflow_catalog` tool and semantic search architecture. |
| **DD-LLM-001** | Query format `<signal_type> <severity>` -- **partially superseded** for workflow discovery. The three-step tools use structured parameters, not free-text queries. DD-LLM-001 remains valid for non-workflow tools. |
| **DD-HAPI-002** | Workflow parameter validation. `WorkflowResponseValidator` updated to use context filters. |
| **DD-004** | RFC 7807 error response standard. Error handling for DS responses follows this standard. |
| **BR-HAPI-250** | Workflow catalog toolset (superseded BR-HAPI-046-050). **Superseded** by BR-HAPI-017-001 through 006. |
| **BR-AUDIT-021-030 v2.0** | Workflow selection audit trail requirements. Updated for three-step protocol. |
| **ADR-056 v1.4** | Post-RCA label computation relocation. Section 8 of this DD implements the HAPI-side flow enforcement and label injection. v1.3 surfaces labels as read-only `cluster_context` in `list_available_actions`. v1.4 adds one-shot reassessment via `detected_infrastructure` in `get_resource_context` (BR-HAPI-017-008). |
| **DD-HAPI-018 v1.1** | DetectedLabels detection specification. Cross-language contract defining the 7 detection characteristics that HAPI implements per ADR-056. v1.1 adds `cluster_context` consumer guidance. |

---

## Review & Evolution

**When to Revisit**:
- If LLM tool calling patterns show the three-step flow is too many round trips for simple cases (consider a "fast path" single-tool for obvious action types)
- If recovery flow validation loop causes unacceptable latency in production
- If DD-WORKFLOW-016 protocol changes (new steps, changed pagination behavior)

**Success Metrics**:
- Workflow selection accuracy improves vs. single-tool baseline (measured by effectiveness scores from EM)
- Recovery flow validation catches invalid parameters before execution (measured by `validation_attempt` audit events)
- No increase in `needs_human_review` rate compared to incident flow

---

**Document Version**: 1.4
**Last Updated**: February 20, 2026
**Status**: ✅ APPROVED
**Authority**: HAPI workflow discovery integration (implements DD-WORKFLOW-016 protocol + ADR-056 flow enforcement)
**Confidence**: 92%

**Confidence Gap (8%)**:
- Recovery validation latency (~4%): Adding `MAX_VALIDATION_ATTEMPTS = 3` to recovery may increase call duration. Mitigated by the fact that validation failures are rare with structured three-step discovery.
- Prompt instruction effectiveness (~4%): The three-step instructions are more complex than the single-tool instruction. LLM compliance depends on prompt engineering quality. Mitigated by DD-WORKFLOW-016's tested prompt contract and tool-level flow enforcement (v1.2 Section 8).
