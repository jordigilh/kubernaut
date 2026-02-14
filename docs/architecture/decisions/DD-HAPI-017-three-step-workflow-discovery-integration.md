# DD-HAPI-017: Three-Step Workflow Discovery Integration

**Status**: ✅ APPROVED
**Decision Date**: 2026-02-05
**Version**: 1.0
**Confidence**: 92%
**Applies To**: HolmesGPT API (HAPI), DataStorage Service (DS)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-05 | Architecture Team | Initial design: replace `search_workflow_catalog` with three-step discovery tools for both incident and recovery flows. No migration -- pre-release greenfield replacement. |

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
- Passes the full signal context filters (severity, component, environment, priority, custom_labels, detected_labels) to DS
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
  - All tools pass full signal context filters (severity, component, environment, priority, custom_labels, detected_labels)
  - All tools pass `remediationId` as query parameter for audit correlation
  - Tools return `StructuredToolResult` with clean LLM-rendered output (finalScore stripped)
  - Toolset registered in both incident and recovery `llm_integration.py`

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

**Document Version**: 1.0
**Last Updated**: February 5, 2026
**Status**: ✅ APPROVED
**Authority**: HAPI workflow discovery integration (implements DD-WORKFLOW-016 protocol)
**Confidence**: 92%

**Confidence Gap (8%)**:
- Recovery validation latency (~4%): Adding `MAX_VALIDATION_ATTEMPTS = 3` to recovery may increase call duration. Mitigated by the fact that validation failures are rare with structured three-step discovery.
- Prompt instruction effectiveness (~4%): The three-step instructions are more complex than the single-tool instruction. LLM compliance depends on prompt engineering quality. Mitigated by DD-WORKFLOW-016's tested prompt contract.
