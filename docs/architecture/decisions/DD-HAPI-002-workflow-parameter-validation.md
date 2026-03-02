# DD-HAPI-002: Workflow Response Validation Architecture

**Date**: December 1, 2025
**Status**: ✅ APPROVED (Updated v1.3)
**Deciders**: Architecture Team, Workflow Engine Team, HolmesGPT-API Team
**Version**: 1.3
**Related**: DD-WORKFLOW-002, DD-HAPI-001, BR-HAPI-191, BR-AI-023, DD-WE-006, GitHub Issue #241

---

## ⚠️ v1.3 UPDATE (March 2, 2026)

**Change**: Added **Step 3b: Undeclared Parameter Stripping** as a mandatory post-validation filter.

**Problem** (GitHub Issue #241): The LLM returns parameters not declared in the workflow schema (e.g., `GIT_PASSWORD`, `GIT_USERNAME`). These hallucinated parameters flow unfiltered through AIAnalysis, RO, and into the WFE where `buildEnvVars` injects them as container environment variables. This is a security risk — the LLM must not provide credentials or arbitrary env vars.

**New Validation Step**:

| Validation Type | Description | V1.0 |
|-----------------|-------------|------|
| **Undeclared Parameter Stripping** | Remove any parameter keys not declared in the workflow schema | ✅ NEW |

**Behavior**:
- After Step 3 (Parameter Schema Validation), all keys in the `params` dict that are **not** declared in the workflow's `parameters` schema are deleted in-place
- If the workflow has **no parameter schema**, all params are stripped (nothing declared = nothing allowed)
- Stripping does **not** cause a validation failure — it is silent filtering with a warning log for each stripped key
- The caller's `selected_workflow["parameters"]` dict is mutated directly (Python dict reference semantics), so no changes are needed to `ValidationResult` or `result_parser.py`

**Security Rationale**: The LLM must never provide credentials. Credentials are handled exclusively via DD-WE-006 dependency mounts (Secrets, ConfigMaps). Stripping undeclared params is defense-in-depth against LLM hallucination of sensitive values.

**Follow-up**: WE controller defense-in-depth filtering (separate issue) — the WE controller already has the schema from OCI extraction and can filter `wfe.Spec.Parameters` before `buildEnvVars`/`convertParameters` as a second layer of protection.

---

## ⚠️ v1.2 UPDATE (December 5, 2025)

**Change**: Expanded from parameter-only validation to **comprehensive workflow response validation**.

**Validation Scope**:

| Validation Type | Description | V1.0 |
|-----------------|-------------|------|
| **Workflow Existence** | Verify `workflow_id` exists in catalog | ✅ NEW |
| **Container Image Consistency** | Verify `container_image` matches catalog for `workflow_id` | ✅ NEW |
| **Parameter Schema** | Verify parameters conform to schema (type, required, length, enum) | ✅ |

**Implementation Model**: **Automatic validation** (not a separate LLM tool call)
- Validation happens when HAPI parses the LLM's JSON response
- If invalid, error returned to LLM for self-correction
- LLM context preserved for retry

---

## ⚠️ v1.1 UPDATE (December 1, 2025)

**Change**: Removed Workflow Engine validation layer.

**New Architecture**: HolmesGPT-API is the **sole** validator.

| Layer | Responsibility | Status |
|-------|---------------|--------|
| **HolmesGPT-API** | Comprehensive workflow response validation with LLM self-correction | ✅ **SOLE VALIDATOR** |
| ~~Workflow Engine~~ | ~~Defense-in-depth re-validation~~ | ❌ **REMOVED** |
| **Tekton Tasks** | Runtime K8s state validation (namespace exists, RBAC, etc.) | ✅ Unchanged |

**Rationale**:
1. If validation fails at WE → must restart entire RCA flow (expensive, poor UX)
2. If validation fails at HAPI → LLM can self-correct in same session (cheap, good UX)
3. Edge cases (HAPI bugs, API bypass) should be fixed at source, not duplicated
4. Simplifies WE architecture - no Data Storage dependency for schema access

**Cancelled**: BR-WE-001 (Defense-in-Depth Parameter Validation)

---

## Context

### Problem Statement

When the LLM returns a workflow recommendation (workflow ID, container image, parameters), the response must be validated before returning to AIAnalysis. Three validations are required:

1. **Workflow Existence**: Does the `workflow_id` exist in the catalog? (Hallucination detection)
2. **Container Image Consistency**: Does the `container_image` match the catalog for that `workflow_id`?
3. **Parameter Schema**: Do the parameters conform to the workflow's schema?

The question is: **where should this validation happen?**

### Key Constraint: LLM Context Preservation

The LLM chat session contains valuable context:
- Root Cause Analysis reasoning
- Workflow selection rationale
- Parameter derivation logic

**If validation fails after the chat session ends:**
- ❌ Context is lost
- ❌ Must restart RCA from scratch
- ❌ Non-deterministic: may get different results

**If validation happens inside the chat session:**
- ✅ LLM can self-correct with full context
- ✅ Immediate retry without starting over
- ✅ LLM understands WHY it chose those parameters

### Workflow Immutability

Workflows in Kubernaut are **immutable**:
- Identified by `container_image` + SHA digest
- Once created, schema never changes
- Even if disabled, selected workflows still execute

**Implication**: No "schema drift" risk between validation and execution.

---

## Decision

### Primary Validation: HolmesGPT-API (In Chat Session)

HolmesGPT-API validates the complete workflow response **inside the LLM chat session** using automatic validation logic. This is NOT a separate tool - validation happens when HAPI parses the LLM's JSON response. If validation fails, errors are returned to the LLM for self-correction.

### Validation Sequence (Automatic)

When the LLM returns a workflow recommendation in JSON format:

1. **Workflow Existence**: Call `GET /api/v1/workflows/{workflow_id}` on Data Storage
2. **Container Image Consistency**: Compare `container_image` with catalog value
3. **Parameter Schema Validation**: Validate parameters against workflow schema
3b. **Undeclared Parameter Stripping**: Remove any parameter keys not declared in the schema (in-place, silent with warning log)

If ANY validation fails → return errors to LLM → LLM self-corrects → retry (max 3 attempts)

---

## Architecture (v1.2 - Comprehensive Validation)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 WORKFLOW RESPONSE VALIDATION FLOW (v1.3)                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  HolmesGPT-API - LLM Chat Session (SOLE VALIDATOR)                    │  │
│  │                                                                       │  │
│  │  1. LLM performs RCA                                                  │  │
│  │  2. LLM calls search_workflow_catalog → selects workflow             │  │
│  │  3. LLM returns JSON response with workflow recommendation           │  │
│  │     ┌─────────────────────────────────────────────────────────────┐  │  │
│  │     │  AUTOMATIC VALIDATION (not a separate tool)                 │  │  │
│  │     │                                                             │  │  │
│  │     │  Parse LLM JSON response:                                   │  │  │
│  │     │  {                                                          │  │  │
│  │     │    "workflow_id": "restart-pod-v1",                         │  │  │
│  │     │    "container_image": "ghcr.io/kubernaut/restart:v1",       │  │  │
│  │     │    "parameters": { "namespace": "prod", "delay": 30 }       │  │  │
│  │     │  }                                                          │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  ┌─────────────────────────────────────────────────────┐   │  │  │
│  │     │  │  STEP 1: Workflow Existence Validation              │   │  │  │
│  │     │  │  ─────────────────────────────────────              │   │  │  │
│  │     │  │  GET /api/v1/workflows/{workflow_id}                │   │  │  │
│  │     │  │  ├─ 200 OK → workflow exists ✅                     │   │  │  │
│  │     │  │  └─ 404 → "Workflow not found, select different" ❌ │   │  │  │
│  │     │  └─────────────────────────────────────────────────────┘   │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  ┌─────────────────────────────────────────────────────┐   │  │  │
│  │     │  │  STEP 2: Container Image Consistency                │   │  │  │
│  │     │  │  ───────────────────────────────────                │   │  │  │
│  │     │  │  Compare: LLM_image vs Catalog_image                │   │  │  │
│  │     │  │  ├─ Match ✅                                        │   │  │  │
│  │     │  │  ├─ LLM_image is null → use Catalog_image ✅        │   │  │  │
│  │     │  │  └─ Mismatch → "Image mismatch, correct it" ❌      │   │  │  │
│  │     │  └─────────────────────────────────────────────────────┘   │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  ┌─────────────────────────────────────────────────────┐   │  │  │
│  │     │  │  STEP 3: Parameter Schema Validation                │   │  │  │
│  │     │  │  ──────────────────────────────────                 │   │  │  │
│  │     │  │  For each parameter in workflow schema:             │   │  │  │
│  │     │  │  ├─ Required: Is it present?                        │   │  │  │
│  │     │  │  ├─ Type: string, int, bool, float?                 │   │  │  │
│  │     │  │  ├─ Length: min/max string length?                  │   │  │  │
│  │     │  │  ├─ Range: min/max numeric value?                   │   │  │  │
│  │     │  │  └─ Enum: Is value in allowed list?                 │   │  │  │
│  │     │  │                                                     │   │  │  │
│  │     │  │  ├─ All valid ✅                                    │   │  │  │
│  │     │  │  └─ Errors → "Fix parameters: [errors]" ❌          │   │  │  │
│  │     │  └─────────────────────────────────────────────────────┘   │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  ┌─────────────────────────────────────────────────────┐   │  │  │
│  │     │  │  STEP 3b: Undeclared Parameter Stripping (v1.3)     │   │  │  │
│  │     │  │  ───────────────────────────────────────────        │   │  │  │
│  │     │  │  Strip keys NOT in schema (in-place mutation):      │   │  │  │
│  │     │  │  ├─ Schema exists: keep only declared param keys    │   │  │  │
│  │     │  │  ├─ No schema: strip ALL keys                       │   │  │  │
│  │     │  │  └─ Log warning for each stripped key               │   │  │  │
│  │     │  │                                                     │   │  │  │
│  │     │  │  (Silent — does NOT cause validation failure)        │   │  │  │
│  │     │  └─────────────────────────────────────────────────────┘   │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  ┌─────────────┐    ┌─────────────────────────────────┐    │  │  │
│  │     │  │  ALL VALID  │    │   ANY INVALID                   │    │  │  │
│  │     │  │  ─────────  │    │   ───────────                   │    │  │  │
│  │     │  │  Return     │    │   Return errors to LLM          │    │  │  │
│  │     │  │  response   │    │   LLM self-corrects             │    │  │  │
│  │     │  │             │    │   Retry (max 3 attempts)        │    │  │  │
│  │     │  └─────────────┘    └─────────────────────────────────┘    │  │  │
│  │     │                                                             │  │  │
│  │     │  If 3 attempts fail → return needs_human_review: true       │  │  │
│  │     └─────────────────────────────────────────────────────────────┘  │  │
│  │  4. Return validated workflow + parameters                           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                         │                                   │
│                                         ▼                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  AIAnalysis CRD Controller                                            │  │
│  │  ─────────────────────────                                            │  │
│  │  Receives validated response - no validation needed                   │  │
│  │  (Defense-in-depth optional - see BR_MAPPING.md)                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                         │                                   │
│                                         ▼                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Workflow Engine (NO VALIDATION - pass-through to Tekton)             │  │
│  │                                                                       │  │
│  │  ✅ Trusts HAPI validation                                            │  │
│  │  ✅ Creates Tekton PipelineRun with parameters as-is                  │  │
│  │  ✅ No Data Storage dependency                                        │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                         │                                   │
│                                         ▼                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Tekton Tasks (RUNTIME VALIDATION)                                    │  │
│  │                                                                       │  │
│  │  Validate against live K8s state:                                     │  │
│  │  • Namespace exists                                                   │  │
│  │  • Resource exists                                                    │  │
│  │  • RBAC permissions OK                                                │  │
│  │  • Container image pullable (infrastructure validation)               │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation

### Automatic Validation in Response Parser

Validation is **NOT** a separate tool. It happens automatically when HAPI parses the LLM's JSON response.

```python
# holmesgpt-api/src/extensions/incident.py

class WorkflowResponseValidator:
    """
    Validates workflow response from LLM automatically.

    This is NOT a separate tool - validation happens when parsing
    the LLM's JSON response, enabling self-correction while context
    is still available.
    """

    def __init__(self, data_storage_client: DataStorageClient):
        self.ds_client = data_storage_client

    async def validate(
        self,
        workflow_id: str,
        container_image: Optional[str],
        parameters: Dict[str, Any]
    ) -> ValidationResult:
        """
        Comprehensive workflow response validation.

        Returns:
            ValidationResult with is_valid=True or errors list
        """
        errors = []

        # STEP 1: Workflow Existence Validation
        workflow = await self._validate_workflow_exists(workflow_id)
        if workflow is None:
            errors.append(
                f"Workflow '{workflow_id}' not found in catalog. "
                f"Please select a different workflow from the search results."
            )
            return ValidationResult(is_valid=False, errors=errors)

        # STEP 2: Container Image Consistency
        image_errors = self._validate_container_image(
            container_image,
            workflow.container_image,
            workflow_id
        )
        errors.extend(image_errors)

        # STEP 3: Parameter Schema Validation
        param_errors = self._validate_parameters(parameters, workflow.schema)
        errors.extend(param_errors)

        if errors:
            return ValidationResult(
                is_valid=False,
                errors=errors,
                schema_hint=self._format_schema_hint(workflow.schema)
            )

        return ValidationResult(
            is_valid=True,
            validated_container_image=workflow.container_image  # Always use catalog value
        )

    async def _validate_workflow_exists(self, workflow_id: str) -> Optional[Workflow]:
        """
        STEP 1: Validate workflow exists in catalog.

        Calls: GET /api/v1/workflows/{workflow_id}
        """
        try:
            return await self.ds_client.get_workflow(workflow_id)
        except WorkflowNotFoundError:
            return None

    def _validate_container_image(
        self,
        llm_image: Optional[str],
        catalog_image: str,
        workflow_id: str
    ) -> List[str]:
        """
        STEP 2: Validate container image consistency.

        Cases:
        - LLM provides matching image → OK
        - LLM provides null/empty → Use catalog image (OK)
        - LLM provides mismatched image → Error (hallucination)
        """
        errors = []

        if llm_image is None or llm_image == "":
            # LLM didn't specify - we'll use catalog value
            return []

        if llm_image != catalog_image:
            errors.append(
                f"Container image mismatch for workflow '{workflow_id}': "
                f"you provided '{llm_image}' but catalog has '{catalog_image}'. "
                f"Please use the correct image from the workflow catalog."
            )

        return errors

    def _validate_parameters(
        self,
        params: Dict[str, Any],
        schema: WorkflowSchema
    ) -> List[str]:
        """
        STEP 3: Validate parameters against workflow schema.

        Validates:
        - Required parameters present
        - Type correctness (string, int, bool, float)
        - String length constraints (min/max)
        - Numeric range constraints (min/max)
        - Enum value validation
        """
        errors = []

        for param_def in schema.parameters:
            value = params.get(param_def.name)

            # Required check
            if param_def.required and value is None:
                errors.append(f"Missing required parameter: '{param_def.name}'")
                continue

            if value is None:
                continue  # Optional and not provided - OK

            # Type check
            type_error = self._validate_type(value, param_def.type, param_def.name)
            if type_error:
                errors.append(type_error)
                continue  # Skip other checks if type is wrong

            # String length check
            if param_def.type == "string":
                if param_def.min_length and len(value) < param_def.min_length:
                    errors.append(
                        f"Parameter '{param_def.name}': length must be >= {param_def.min_length}, "
                        f"got {len(value)}"
                    )
                if param_def.max_length and len(value) > param_def.max_length:
                    errors.append(
                        f"Parameter '{param_def.name}': length must be <= {param_def.max_length}, "
                        f"got {len(value)}"
                    )

            # Numeric range check
            if param_def.type in ("int", "float"):
                if param_def.minimum is not None and value < param_def.minimum:
                    errors.append(
                        f"Parameter '{param_def.name}': must be >= {param_def.minimum}, got {value}"
                    )
                if param_def.maximum is not None and value > param_def.maximum:
                    errors.append(
                        f"Parameter '{param_def.name}': must be <= {param_def.maximum}, got {value}"
                    )

            # Enum check
            if param_def.enum and value not in param_def.enum:
                errors.append(
                    f"Parameter '{param_def.name}': must be one of {param_def.enum}, got '{value}'"
                )

        return errors

    def _validate_type(self, value: Any, expected_type: str, param_name: str) -> Optional[str]:
        """Validate parameter type."""
        type_map = {
            "string": str,
            "int": int,
            "float": (int, float),
            "bool": bool
        }

        expected = type_map.get(expected_type)
        if expected is None:
            return None  # Unknown type - skip validation

        if not isinstance(value, expected):
            return (
                f"Parameter '{param_name}': expected {expected_type}, "
                f"got {type(value).__name__}"
            )

        return None
```

### Integration with Incident Parser

```python
# In _parse_investigation_result function

async def _parse_investigation_result(
    self,
    analysis: str,
    owner_chain: List[str],
    data_storage_client: DataStorageClient
) -> Dict[str, Any]:
    """Parse LLM response with automatic validation."""

    json_match = re.search(r'```json\s*(\{.*?\})\s*```', analysis, re.DOTALL)
    if not json_match:
        return {"error": "No JSON found in response"}

    json_data = json.loads(json_match.group(1))

    # Extract workflow recommendation
    workflow_id = json_data.get("workflow_id")
    container_image = json_data.get("container_image")
    parameters = json_data.get("parameters", {})

    if workflow_id:
        # AUTOMATIC VALIDATION
        validator = WorkflowResponseValidator(data_storage_client)
        validation = await validator.validate(workflow_id, container_image, parameters)

        if not validation.is_valid:
            # Return errors to LLM for self-correction
            return {
                "validation_failed": True,
                "errors": validation.errors,
                "schema_hint": validation.schema_hint,
                "message": "Workflow validation failed. Please correct and retry."
            }

        # Use validated container image from catalog
        container_image = validation.validated_container_image

    # ... rest of parsing ...
```

### LLM Prompt for Self-Correction

```
## Workflow Recommendation Format

When recommending a workflow, provide your response in JSON format:

```json
{
  "workflow_id": "restart-pod-v1",
  "container_image": null,  // Leave null - system will populate from catalog
  "parameters": {
    "namespace": "production",
    "delay_seconds": 30
  },
  "confidence": 0.85,
  "rationale": "..."
}
```

IMPORTANT: Your response will be automatically validated against the workflow catalog.
If validation fails, you will receive error messages and must correct your recommendation.

Validation checks:
1. workflow_id must exist in the catalog (from your search results)
2. container_image must match the catalog (recommend leaving it null)
3. parameters must conform to the workflow's schema (type, required, length, range)

You have up to 3 attempts to provide a valid recommendation. If validation fails
after 3 attempts, set needs_human_review: true in your response.
```

---

## Why NOT Other Services

### ❌ NOT Remediation Orchestrator

| Reason | Explanation |
|--------|-------------|
| Single Responsibility | RO coordinates flow, doesn't validate execution details |
| No Schema Access | Would need to fetch schema (duplication) |
| Wrong Abstraction Level | RO decides WHAT to do, not HOW to validate |

### ❌ NOT AIAnalysis CRD Controller

| Reason | Explanation |
|--------|-------------|
| Bridge Layer | Should be thin - transform and forward |
| Duplication | Would duplicate HolmesGPT-API validation |
| Late Stage | After LLM session - can't self-correct |
| Expensive Recovery | Must restart entire RCA if validation fails |

### ❌ NOT Workflow Engine

| Reason | Explanation |
|--------|-------------|
| Too Late | After LLM session ends - can't self-correct |
| Expensive | Must restart entire RCA flow on failure |
| No Schema Access | Would need Data Storage dependency |

---

## Validation Types by Layer (v1.2)

| Layer | Validation Type | Examples | LLM Can Self-Correct? |
|-------|-----------------|----------|----------------------|
| **HolmesGPT-API** | Workflow existence | `workflow_id` in catalog | ✅ Yes |
| **HolmesGPT-API** | Image consistency | `container_image` matches catalog | ✅ Yes |
| **HolmesGPT-API** | Parameter schema | Required, types, length, range, enum | ✅ Yes |
| **Tekton Tasks** | Runtime validation | Namespace exists, RBAC OK, image pullable | ❌ No (infrastructure) |

---

## Confidence Assessment

**Confidence: 95%**

| Factor | Confidence | Notes |
|--------|------------|-------|
| LLM context preservation | +30% | Major benefit - avoids costly restarts |
| Self-correction loop | +25% | LLM can fix errors with context |
| Workflow immutability | +20% | No schema drift risk |
| Defense-in-depth at WE | +15% | Catches edge cases |
| Proven pattern | +5% | Similar to OpenAI function calling |

### Remaining 5% Uncertainty

- LLM may fail to self-correct in edge cases (mitigated by WE validation)
- Added complexity (mitigated by test coverage)

---

## Consequences

### Positive

1. **Preserves LLM Context**: Self-correction without restarting RCA
2. **Faster Feedback**: Validation errors caught early
3. **Better UX**: Users don't wait for execution to discover parameter errors
4. **Defense-in-Depth**: WE catches what HolmesGPT-API misses

### Negative

1. **Extra Data Storage Call**: HolmesGPT-API fetches workflow schema
2. **Added Complexity**: New tool, validation logic in two places
3. **LLM Token Usage**: Validation loop adds tokens

### Neutral

1. **Double Validation**: Acceptable for safety
2. **Implementation Effort**: Moderate (~2-3 days for tool + WE validation)

---

## Related Business Requirement

**BR-HAPI-191: Workflow Parameter Validation in Chat Session**

See: `docs/requirements/BR-HAPI-191-workflow-parameter-validation.md`

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.3 | 2026-03-02 | **SECURITY**: Added Step 3b — undeclared parameter stripping (GitHub Issue #241). LLM-hallucinated params (e.g., GIT_PASSWORD) are silently removed before reaching execution. No-schema workflows have all params stripped. |
| 1.2 | 2025-12-05 | **EXPANDED**: Added workflow existence and container image validation. Changed from tool-based to automatic validation. Added comprehensive implementation design. |
| 1.1 | 2025-12-01 | **SIMPLIFIED**: Removed WE validation layer. HAPI is now sole validator. BR-WE-001 cancelled. |
| 1.0 | 2025-12-01 | Initial decision - HolmesGPT-API primary, WE defense-in-depth |

