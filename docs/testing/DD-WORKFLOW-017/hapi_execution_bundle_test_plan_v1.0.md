# HAPI Execution Bundle Consumption Test Plan (Issue #89 Phase 2)

**Design Decision**: DD-WORKFLOW-017 - Workflow Lifecycle Component Interactions
**Business Requirement**: BR-HAPI-196 (Container Image Consistency), BR-HAPI-191 (Parameter Validation)
**Service**: HolmesGPT-API (HAPI)
**Version**: 1.0
**Date**: February 17, 2026
**Status**: ACTIVE
**Prerequisite**: Phase 1 DataStorage changes (see `execution_bundle_test_plan_v1.0.md`)

---

## Test Plan Overview

### Scope

This test plan covers HAPI's consumption of the `execution_bundle` field introduced by
Issue #89 Phase 1 (DataStorage). The DataStorage API now returns `execution_bundle` alongside
`schema_image` (renamed from `container_image`). HAPI must:

- **Read** `execution_bundle` from the DataStorage workflow catalog response
- **Validate** LLM-provided `execution_bundle` against the catalog value
- **Write** the validated `execution_bundle` into the `selected_workflow` dict (used downstream by AA/RO/WFE)
- **Expose** `execution_bundle` in discovery tool responses for LLM context

### Services Under Test

1. **WorkflowResponseValidator** (`holmesgpt-api/src/validation/workflow_response_validator.py`):
   Reads `workflow.execution_bundle` from DS catalog, validates against LLM value, sets
   `validated_execution_bundle` on `ValidationResult`.

2. **Incident Result Parser** (`holmesgpt-api/src/extensions/incident/result_parser.py`):
   Writes `selected_workflow["execution_bundle"]` from `ValidationResult.validated_execution_bundle`.

3. **Discovery Tools** (`holmesgpt-api/src/toolsets/workflow_discovery.py`):
   Step 2 (ListWorkflows) and Step 3 (GetWorkflow) expose `execution_bundle` in LLM-facing responses.

4. **DS Client Model** (`holmesgpt-api/src/clients/datastorage/`):
   `RemediationWorkflow` and `WorkflowDiscoveryEntry` models must expose `execution_bundle`.

### Out of Scope

- DataStorage validation of `execution.bundle` (covered by Phase 1 test plan)
- Downstream CRD field renames in AA/WFE (covered by Phase 3)
- Recovery result parser (`holmesgpt-api/src/extensions/recovery/result_parser.py`) -- deferred
- OCI 1.1 subject/referrers integration (Issue #105)
- Python integration tests against a running DataStorage (deferred to integration tier)

### Design Decisions

**Decision date**: 2026-02-17

**Context**: After Phase 1, DataStorage returns two distinct OCI references:
- `schema_image` / `schema_digest`: The OCI image containing `/workflow-schema.yaml` (registration artifact)
- `execution_bundle` / `execution_bundle_digest`: The operator's Tekton bundle / Job image (execution artifact)

HAPI must consume `execution_bundle` as the **execution artifact** that flows downstream to
AIAnalysis and WorkflowExecution. The old `container_image` field on HAPI's validator and
result parser must be renamed to `execution_bundle`.

**Decision**: No backward compatibility fallback is needed (`container_image` is fully replaced
by `execution_bundle`). The LLM prompt and tool responses will use `execution_bundle`.

**Rationale**:
- No official release has shipped; all consumers are internal
- `execution_bundle` is semantically precise (vs. ambiguous `container_image`)
- The LLM needs to see the correct field name in tool responses to avoid hallucination

---

## Test Scenario Naming Convention

**Format**: `{TIER}-HAPI-017-{SEQUENCE}`

Per project convention:
- `UT-HAPI-017-0xx` -- HAPI unit tests (Python, pytest)

Existing HAPI test scenarios for DD-HAPI-017 cover three-step discovery protocol.
This test plan extends with execution_bundle-specific scenarios.

---

## Triage Principles Applied

1. **End-to-end field rename traceability**: Every scenario traces the rename from DS API response
   (`execution_bundle`) through HAPI validation to the output `selected_workflow` dict.
2. **Catalog-as-truth**: `validated_execution_bundle` always comes from the DS catalog, never from
   the LLM. Tests must verify the LLM value is overridden by the catalog value.
3. **Exact field name assertions**: Tests assert on the literal dict key `"execution_bundle"`,
   not `"container_image"`. This catches incomplete renames.
4. **Mock isolation**: Validator tests mock the DS client to return controlled `execution_bundle`
   values. Discovery tool tests mock the HTTP responses. No real DS required.

---

## Defense-in-Depth Coverage

| Tier | BR Coverage | Code Coverage Target | Scenarios | Focus |
|------|-------------|---------------------|-----------|-------|
| **Unit** | 100% of execution_bundle rename in HAPI | Validator, result parser, discovery tool response mapping | 8 | Field rename correctness, catalog-as-truth |
| **Integration** | DS API contract | Deferred | TBD | Real DS returning `execution_bundle` |

---

## 1. Unit Tests -- Validator (Python)

**Location**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py` (extend existing)
**Framework**: pytest
**SUT**: `WorkflowResponseValidator.validate()` and `ValidationResult`
**Mock**: DS client (`get_workflow_by_id` returns a workflow with `execution_bundle` set)

**Shared preconditions for all validator tests**:
- `WorkflowResponseValidator` instantiated with a mock DS client
- Mock DS client returns a workflow object with:
  - `workflow_id = "test-wf-001"`
  - `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"`
  - All other required fields populated

---

### UT-HAPI-017-001: Validator reads `execution_bundle` from catalog and sets `validated_execution_bundle`

- **BR**: BR-HAPI-196 (Execution Bundle Consistency), DD-WORKFLOW-017
- **Business Outcome**: When HAPI validates a selected workflow, it always uses the catalog's
  `execution_bundle` as the source of truth for downstream execution, regardless of what the LLM provided.
  This ensures supply-chain integrity by never trusting LLM-originated image references.
- **Preconditions**:
  - Mock DS client returns workflow with `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."`
  - LLM provides `execution_bundle = None` (null/empty -- most common case)
- **Given**:
  - A workflow exists in the catalog with a digest-pinned `execution_bundle`
  - The LLM response does not include an `execution_bundle` value
- **When**: `validator.validate(workflow_id="test-wf-001", execution_bundle=None, parameters={...})` is called
- **Then**:
  - `result.is_valid` is `True`
  - `result.validated_execution_bundle` equals the catalog value exactly:
    `"quay.io/kubernaut/bundles/restart@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"`
  - `result.errors` is empty
- **Exit Criteria**:
  - `validated_execution_bundle` is populated from catalog even when LLM provides nothing
  - The validator does not treat missing LLM `execution_bundle` as an error
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_001_validator_reads_execution_bundle_from_catalog():
      mock_workflow = create_mock_workflow(
          execution_bundle="quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      )
      mock_ds_client = Mock()
      mock_ds_client.get_workflow_by_id.return_value = mock_workflow

      validator = WorkflowResponseValidator(mock_ds_client)
      result = validator.validate(
          workflow_id="test-wf-001",
          execution_bundle=None,
          parameters={"NAMESPACE": "default"}
      )

      assert result.is_valid is True
      assert result.validated_execution_bundle == mock_workflow.execution_bundle
      assert len(result.errors) == 0
  ```
- **Anti-Pattern Warnings**:
  - Do not assert on `validated_container_image` -- that field is removed
  - Do not allow `validated_execution_bundle` to be `None` when the catalog has a value

---

### UT-HAPI-017-002: Validator passes when LLM `execution_bundle` matches catalog exactly

- **BR**: BR-HAPI-196 (Execution Bundle Consistency)
- **Business Outcome**: When the LLM correctly echoes the catalog's `execution_bundle`, validation
  passes without errors. This confirms the LLM is using discovery tool data faithfully.
- **Preconditions**:
  - Mock DS client returns workflow with known `execution_bundle`
  - LLM provides the exact same `execution_bundle` string
- **Given**:
  - Catalog `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."`
  - LLM `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."` (exact match)
- **When**: `validator.validate(workflow_id="test-wf-001", execution_bundle=<same>, parameters={...})` is called
- **Then**:
  - `result.is_valid` is `True`
  - `result.validated_execution_bundle` equals the catalog value
  - `result.errors` is empty
- **Exit Criteria**:
  - Exact string match between LLM and catalog passes validation
  - No warnings or errors emitted
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_002_validator_passes_on_matching_execution_bundle():
      bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      mock_workflow = create_mock_workflow(execution_bundle=bundle)
      mock_ds_client = Mock()
      mock_ds_client.get_workflow_by_id.return_value = mock_workflow

      validator = WorkflowResponseValidator(mock_ds_client)
      result = validator.validate(
          workflow_id="test-wf-001",
          execution_bundle=bundle,
          parameters={"NAMESPACE": "default"}
      )

      assert result.is_valid is True
      assert result.validated_execution_bundle == bundle
  ```
- **Anti-Pattern Warnings**:
  - Do not use `assertIn` or substring matching; require exact equality

---

### UT-HAPI-017-003: Validator reports error when LLM `execution_bundle` differs from catalog

- **BR**: BR-HAPI-196 (Execution Bundle Consistency -- hallucination detection)
- **Business Outcome**: When the LLM fabricates or modifies the `execution_bundle` (hallucination),
  validation fails with a clear error message. This prevents execution of untrusted bundles.
  The `validated_execution_bundle` is still set to the catalog value for self-correction.
- **Preconditions**:
  - Mock DS client returns workflow with known `execution_bundle`
  - LLM provides a different `execution_bundle` (hallucinated value)
- **Given**:
  - Catalog `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."`
  - LLM `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:999999..."` (different digest)
- **When**: `validator.validate(workflow_id="test-wf-001", execution_bundle=<hallucinated>, parameters={...})` is called
- **Then**:
  - `result.is_valid` is `False`
  - `result.errors` contains at least one entry mentioning `"execution_bundle"` and `"mismatch"` (or equivalent)
  - `result.validated_execution_bundle` still equals the catalog value (for self-correction)
  - `result.schema_hint` is not None (provides correction guidance to LLM)
- **Exit Criteria**:
  - Hallucinated `execution_bundle` is rejected
  - Error message is actionable (mentions both the LLM value and catalog value)
  - Catalog value is still available via `validated_execution_bundle` for retry
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_003_validator_rejects_mismatched_execution_bundle():
      catalog_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      llm_bundle = "quay.io/kubernaut/bundles/restart@sha256:999999..."
      mock_workflow = create_mock_workflow(execution_bundle=catalog_bundle)
      mock_ds_client = Mock()
      mock_ds_client.get_workflow_by_id.return_value = mock_workflow

      validator = WorkflowResponseValidator(mock_ds_client)
      result = validator.validate(
          workflow_id="test-wf-001",
          execution_bundle=llm_bundle,
          parameters={"NAMESPACE": "default"}
      )

      assert result.is_valid is False
      assert any("execution_bundle" in e.lower() or "mismatch" in e.lower() for e in result.errors)
      assert result.validated_execution_bundle == catalog_bundle
      assert result.schema_hint is not None
  ```
- **Anti-Pattern Warnings**:
  - Do not silently accept mismatched values
  - Do not clear `validated_execution_bundle` on validation failure; it's needed for self-correction

---

### UT-HAPI-017-004: `ValidationResult` exposes `validated_execution_bundle` (not `validated_container_image`)

- **BR**: DD-WORKFLOW-017 (Field Rename Compliance)
- **Business Outcome**: The `ValidationResult` dataclass uses the renamed field
  `validated_execution_bundle` exclusively. No references to the deprecated
  `validated_container_image` field exist. This ensures downstream consumers
  (result parser, AA response processor) read the correct field.
- **Preconditions**:
  - `ValidationResult` dataclass imported
- **Given**:
  - `ValidationResult` is constructed with `validated_execution_bundle="quay.io/..."`
- **When**: The dataclass is inspected
- **Then**:
  - `result.validated_execution_bundle` returns the set value
  - `hasattr(result, "validated_container_image")` is `False`
- **Exit Criteria**:
  - Rename is complete -- old field is removed, not aliased
  - No backward-compat shim exists
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_004_validation_result_uses_execution_bundle_field():
      result = ValidationResult(
          is_valid=True,
          errors=[],
          validated_execution_bundle="quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      )
      assert result.validated_execution_bundle == "quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      assert not hasattr(result, "validated_container_image")
  ```
- **Anti-Pattern Warnings**:
  - Do not add backward-compat aliases or `@property` shims for the old field name

---

## 2. Unit Tests -- Incident Result Parser (Python)

**Location**: `holmesgpt-api/tests/unit/test_llm_self_correction.py` (extend existing)
**Framework**: pytest
**SUT**: `parse_and_validate_investigation_result()`
**Mock**: DS client, LLM investigation response JSON

---

### UT-HAPI-017-005: Result parser writes `execution_bundle` into `selected_workflow`

- **BR**: BR-HAPI-196 (Execution Bundle Consistency), DD-WORKFLOW-017
- **Business Outcome**: After validation, the `selected_workflow` dict contains
  `"execution_bundle"` (not `"container_image"`) with the catalog-validated value.
  This is the value that flows to AIAnalysis status and WorkflowExecution spec.
- **Preconditions**:
  - Mock DS client returns workflow with `execution_bundle = "quay.io/kubernaut/bundles/restart@sha256:abcdef..."`
  - LLM response JSON contains `selected_workflow` with `workflow_id` and valid parameters
- **Given**:
  - LLM investigation result JSON:
    ```json
    {
      "rca": {"summary": "OOM detected", "severity": "critical", "contributing_factors": ["memory leak"]},
      "selected_workflow": {
        "workflow_id": "test-wf-001",
        "confidence": 0.95,
        "parameters": {"NAMESPACE": "default", "POD_NAME": "app-1"}
      }
    }
    ```
- **When**: `parse_and_validate_investigation_result(investigation, request_data, ds_client)` is called
- **Then**:
  - Returned `selected_workflow["execution_bundle"]` equals the catalog value
  - `"container_image"` key does NOT exist in the `selected_workflow` dict
  - `validation_result` is `None` (success -- no validation errors)
- **Exit Criteria**:
  - Key name is `"execution_bundle"`, not `"container_image"`
  - Value comes from `validated_execution_bundle`, not from LLM
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_005_result_parser_writes_execution_bundle():
      mock_ds_client = create_mock_ds_client(
          execution_bundle="quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      )
      investigation = create_mock_investigation(
          selected_workflow={"workflow_id": "test-wf-001", "confidence": 0.95, "parameters": {...}}
      )

      result_data, validation_result = parse_and_validate_investigation_result(
          investigation, request_data, mock_ds_client
      )

      assert "execution_bundle" in result_data["selected_workflow"]
      assert "container_image" not in result_data["selected_workflow"]
      assert result_data["selected_workflow"]["execution_bundle"] == "quay.io/kubernaut/bundles/restart@sha256:abcdef..."
      assert validation_result is None  # success
  ```
- **Anti-Pattern Warnings**:
  - Do not check for both `"execution_bundle"` and `"container_image"` keys -- only the new one should exist
  - Do not treat the LLM's value as authoritative -- always use the catalog value

---

### UT-HAPI-017-006: Result parser includes `execution_bundle` for alternative workflows

- **BR**: BR-HAPI-196 (Execution Bundle Consistency)
- **Business Outcome**: Alternative workflows in the LLM response also use the `"execution_bundle"`
  key (not `"container_image"`), maintaining consistent field naming across the response.
- **Preconditions**:
  - LLM response includes `alternative_workflows` array with `execution_bundle` values
- **Given**:
  - LLM investigation result JSON includes:
    ```json
    {
      "selected_workflow": { "workflow_id": "wf-001", ... },
      "alternative_workflows": [
        { "workflow_id": "wf-002", "execution_bundle": "quay.io/alt@sha256:...", "confidence": 0.7, "rationale": "Alternative" }
      ]
    }
    ```
- **When**: `parse_and_validate_investigation_result(investigation, request_data, ds_client)` is called
- **Then**:
  - Each entry in `result_data["alternative_workflows"]` has key `"execution_bundle"` (not `"container_image"`)
- **Exit Criteria**:
  - Alternative workflows use `"execution_bundle"` key consistently
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_006_alternatives_use_execution_bundle_key():
      # ... setup with alternative_workflows in LLM JSON ...
      result_data, _ = parse_and_validate_investigation_result(investigation, request_data, ds_client)

      for alt in result_data.get("alternative_workflows", []):
          assert "execution_bundle" in alt
          assert "container_image" not in alt
  ```
- **Anti-Pattern Warnings**:
  - Do not skip alternatives -- they are part of the contract with downstream consumers

---

## 3. Unit Tests -- Discovery Tools (Python)

**Location**: `holmesgpt-api/tests/unit/test_workflow_discovery_tools.py` (extend existing)
**Framework**: pytest
**SUT**: `ListWorkflowsTool`, `GetWorkflowTool`
**Mock**: HTTP responses from DataStorage API

---

### UT-HAPI-017-007: ListWorkflows (Step 2) response includes `execution_bundle`

- **BR**: BR-HAPI-017-001 (Three-Step Discovery), DD-WORKFLOW-017
- **Business Outcome**: When the LLM calls the `list_workflows` tool (Step 2 of discovery),
  each workflow entry in the response includes `execution_bundle` so the LLM can reference it
  in its selected_workflow response. This replaces `containerImage` / `container_image`.
- **Preconditions**:
  - Mock HTTP response returns DataStorage `WorkflowDiscoveryResponse` with `execution_bundle` field
- **Given**:
  - DS API response for `GET /api/v1/workflows/actions/RestartPod`:
    ```json
    {
      "actionType": "RestartPod",
      "workflows": [
        {
          "workflowId": "wf-001",
          "workflowName": "restart-pod-v1",
          "name": "restart-pod-v1",
          "description": {"what": "Restarts a pod", "whenToUse": "OOMKilled"},
          "version": "1.0.0",
          "schema_image": "quay.io/kubernaut/schemas/restart@sha256:...",
          "execution_bundle": "quay.io/kubernaut/bundles/restart@sha256:abcdef..."
        }
      ],
      "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": false}
    }
    ```
- **When**: `ListWorkflowsTool._run(action_type="RestartPod", ...)` is called
- **Then**:
  - Tool output contains `"execution_bundle"` for each workflow entry
  - `"containerImage"` / `"container_image"` does NOT appear in the tool output
- **Exit Criteria**:
  - LLM-facing discovery response uses `execution_bundle`
  - Old field name is not present
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_007_list_workflows_includes_execution_bundle(mock_http):
      mock_http.get(
          f"{DS_BASE_URL}/api/v1/workflows/actions/RestartPod",
          json={"actionType": "RestartPod", "workflows": [{"workflowId": "wf-001", "execution_bundle": "quay.io/..."}], ...}
      )
      tool = ListWorkflowsTool(ds_base_url=DS_BASE_URL)
      result = tool._run(action_type="RestartPod")

      assert "execution_bundle" in result
      assert "containerImage" not in result
      assert "container_image" not in result
  ```
- **Anti-Pattern Warnings**:
  - Discovery tools pass DS responses as-is; do not add transformation logic
  - Assert on the serialized string output, not on parsed JSON (tools return strings to LLM)

---

### UT-HAPI-017-008: GetWorkflow (Step 3) response includes `execution_bundle`

- **BR**: BR-HAPI-017-001 (Three-Step Discovery), DD-WORKFLOW-017
- **Business Outcome**: When the LLM calls the `get_workflow` tool (Step 3 of discovery),
  the full workflow response includes `execution_bundle` and `schema_image` (renamed from
  `container_image`). The LLM uses this for its final workflow selection response.
- **Preconditions**:
  - Mock HTTP response returns DataStorage `RemediationWorkflow` with `execution_bundle` and `schema_image`
- **Given**:
  - DS API response for `GET /api/v1/workflows/{workflowId}`:
    ```json
    {
      "workflow_id": "uuid-001",
      "workflow_name": "restart-pod-v1",
      "schema_image": "quay.io/kubernaut/schemas/restart@sha256:...",
      "schema_digest": "sha256:...",
      "execution_bundle": "quay.io/kubernaut/bundles/restart@sha256:abcdef...",
      "execution_bundle_digest": "abcdef...",
      "execution_engine": "tekton",
      "parameters": {"schema": {"parameters": [...]}}
    }
    ```
- **When**: `GetWorkflowTool._run(workflow_id="uuid-001", ...)` is called
- **Then**:
  - Tool output contains `"execution_bundle"` and `"schema_image"`
  - `"container_image"` and `"container_digest"` do NOT appear in the tool output
- **Exit Criteria**:
  - Full workflow response uses renamed fields
  - Old field names are absent
- **Implementation Hint**:
  ```python
  def test_ut_hapi_017_008_get_workflow_includes_execution_bundle(mock_http):
      mock_http.get(
          f"{DS_BASE_URL}/api/v1/workflows/uuid-001",
          json={"workflow_id": "uuid-001", "execution_bundle": "quay.io/...", "schema_image": "quay.io/...", ...}
      )
      tool = GetWorkflowTool(ds_base_url=DS_BASE_URL)
      result = tool._run(workflow_id="uuid-001")

      assert "execution_bundle" in result
      assert "schema_image" in result
      assert "container_image" not in result
      assert "container_digest" not in result
  ```
- **Anti-Pattern Warnings**:
  - Do not parse the tool string output back to JSON for assertion; the raw string IS the LLM input
  - Ensure the mock response matches the actual DS API contract from Phase 1

---

## Traceability Matrix

| Scenario | BR | Component | Input | Expected Output |
|----------|----|-----------|----|-------|
| UT-HAPI-017-001 | BR-HAPI-196 | Validator | LLM `execution_bundle=None` | `validated_execution_bundle` = catalog value |
| UT-HAPI-017-002 | BR-HAPI-196 | Validator | LLM `execution_bundle` = catalog | `is_valid=True`, `validated_execution_bundle` = catalog |
| UT-HAPI-017-003 | BR-HAPI-196 | Validator | LLM `execution_bundle` != catalog | `is_valid=False`, error mentions mismatch |
| UT-HAPI-017-004 | DD-WORKFLOW-017 | ValidationResult | Dataclass construction | `validated_execution_bundle` exists, `validated_container_image` removed |
| UT-HAPI-017-005 | BR-HAPI-196 | Result Parser | Valid LLM JSON | `selected_workflow["execution_bundle"]` = catalog value |
| UT-HAPI-017-006 | BR-HAPI-196 | Result Parser | LLM JSON with alternatives | `alternative_workflows[*]["execution_bundle"]` present |
| UT-HAPI-017-007 | BR-HAPI-017-001 | Discovery | ListWorkflows response | `execution_bundle` in output, no `containerImage` |
| UT-HAPI-017-008 | BR-HAPI-017-001 | Discovery | GetWorkflow response | `execution_bundle` + `schema_image` in output |

---

## Implementation Order (TDD)

### RED Phase
1. Write all 8 test scenarios as failing tests
2. Tests fail because:
   - `ValidationResult` still has `validated_container_image` (not `validated_execution_bundle`)
   - Validator `.validate()` still accepts `container_image` parameter (not `execution_bundle`)
   - Result parser writes `"container_image"` key (not `"execution_bundle"`)
   - DS mock responses use old field names

### GREEN Phase
1. Rename `ValidationResult.validated_container_image` -> `validated_execution_bundle`
2. Rename `validate()` parameter `container_image` -> `execution_bundle`
3. Rename `_validate_container_image()` -> `_validate_execution_bundle()`
4. Update result parser to read/write `"execution_bundle"` key
5. Update alternative workflow dict key from `"container_image"` to `"execution_bundle"`
6. Update DS client model `RemediationWorkflow.container_image` -> `execution_bundle`
7. Update mock workflow fixtures in existing tests

### REFACTOR Phase
1. Remove any remaining `container_image` references in HAPI codebase
2. Update existing test fixtures that reference old field names
3. Verify `grep -r "container_image" holmesgpt-api/` returns zero hits (excluding vendor/generated)

---

## Relationship to Other Test Plans

| Test Plan | Phase | Dependency |
|-----------|-------|------------|
| `execution_bundle_test_plan_v1.0.md` (this dir) | Phase 1 - DataStorage | **Prerequisite**: DS must return `execution_bundle` in API |
| This plan | Phase 2 - HAPI | Consumes DS `execution_bundle` |
| Phase 3 (TBD) | AA/WFE CRD rename | Consumes HAPI output `selected_workflow["execution_bundle"]` |
