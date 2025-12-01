# DD-HAPI-002: Workflow Parameter Validation Architecture

**Date**: December 1, 2025
**Status**: ✅ APPROVED
**Deciders**: Architecture Team, Workflow Engine Team, HolmesGPT-API Team
**Version**: 1.0
**Related**: DD-WORKFLOW-002, DD-HAPI-001, BR-HAPI-191

---

## Context

### Problem Statement

When HolmesGPT-API selects a workflow and suggests parameters, those parameters must be validated against the workflow's parameter schema before execution. The question is: **where should this validation happen?**

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

HolmesGPT-API validates workflow parameters **inside the LLM chat session** using a new `validate_workflow_parameters` tool. If validation fails, the LLM can self-correct.

### Secondary Validation: Workflow Engine (Defense-in-Depth)

Workflow Engine re-validates parameters before creating Tekton resources as a safety net for edge cases where the LLM fails to self-correct.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        PARAMETER VALIDATION FLOW                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  HolmesGPT-API - LLM Chat Session (PRIMARY - 95% of issues)          │  │
│  │                                                                       │  │
│  │  1. LLM performs RCA                                                  │  │
│  │  2. LLM calls search_workflow_catalog → selects workflow             │  │
│  │  3. LLM suggests parameters based on RCA                             │  │
│  │  4. LLM calls validate_workflow_parameters                           │  │
│  │     ┌─────────────────────────────────────────────────────────────┐  │  │
│  │     │  VALIDATION LOOP                                            │  │  │
│  │     │                                                             │  │  │
│  │     │  validate_workflow_parameters(workflow_id, params)          │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  Fetch schema from Data Storage                             │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  Validate: required, types, enums, ranges                   │  │  │
│  │     │          │                                                  │  │  │
│  │     │          ▼                                                  │  │  │
│  │     │  ┌─────────────┐    ┌─────────────────────────────────┐    │  │  │
│  │     │  │   VALID     │    │   INVALID                       │    │  │  │
│  │     │  │   ───────   │    │   ───────                       │    │  │  │
│  │     │  │   Continue  │    │   Return errors to LLM          │    │  │  │
│  │     │  │             │    │   LLM self-corrects             │    │  │  │
│  │     │  │             │    │   Retry (max 3 attempts)        │    │  │  │
│  │     │  └─────────────┘    └─────────────────────────────────┘    │  │  │
│  │     │                                                             │  │  │
│  │     └─────────────────────────────────────────────────────────────┘  │  │
│  │  5. Return validated workflow + parameters                           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                         │                                   │
│                                         ▼                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Remediation Orchestrator                                             │  │
│  │  ──────────────────────                                               │  │
│  │  Coordination only - no parameter validation                          │  │
│  │  (Single Responsibility: orchestration, not validation)               │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                         │                                   │
│                                         ▼                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Workflow Engine (DEFENSE-IN-DEPTH - 5% edge cases)                   │  │
│  │                                                                       │  │
│  │  Re-validate before creating Tekton resources:                        │  │
│  │  • Required parameters present                                        │  │
│  │  • Type validation                                                    │  │
│  │  • Enum validation                                                    │  │
│  │  • Catches LLM self-correction failures                              │  │
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
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation

### New Tool: `validate_workflow_parameters`

```python
class ValidateWorkflowParametersTool(Tool):
    """
    Validates workflow parameters against the workflow schema.

    Called by LLM after selecting a workflow and before returning
    the final recommendation. Enables self-correction on validation errors.
    """

    name = "validate_workflow_parameters"
    description = """
    Validate workflow parameters against the workflow schema.
    You MUST call this tool after selecting a workflow and suggesting parameters.
    If validation fails, review the errors and correct your parameters.
    """

    parameters = {
        "workflow_id": ToolParameter(
            type="string",
            description="The workflow ID to validate against",
            required=True
        ),
        "parameters": ToolParameter(
            type="object",
            description="The parameters to validate",
            required=True
        )
    }

    def invoke(self, workflow_id: str, parameters: dict) -> ToolResult:
        # 1. Fetch workflow schema from Data Storage
        schema = self._fetch_workflow_schema(workflow_id)

        # 2. Validate parameters
        errors = self._validate(parameters, schema)

        if errors:
            return ToolResult(
                status="invalid",
                errors=errors,
                schema_hint=self._format_schema_hint(schema),
                message="Parameters invalid. Please correct and retry."
            )

        return ToolResult(
            status="valid",
            message="All parameters validated successfully."
        )

    def _validate(self, params: dict, schema: WorkflowSchema) -> List[str]:
        errors = []

        for param_def in schema.parameters:
            value = params.get(param_def.name)

            # Required check
            if param_def.required and value is None:
                errors.append(f"Missing required parameter: {param_def.name}")
                continue

            if value is None:
                continue  # Optional and not provided

            # Type check
            if not self._validate_type(value, param_def.type):
                errors.append(
                    f"Parameter '{param_def.name}': expected {param_def.type}, "
                    f"got {type(value).__name__}"
                )

            # Enum check
            if param_def.enum and value not in param_def.enum:
                errors.append(
                    f"Parameter '{param_def.name}': must be one of {param_def.enum}, "
                    f"got '{value}'"
                )

            # Range check (for numeric types)
            if param_def.minimum is not None and value < param_def.minimum:
                errors.append(
                    f"Parameter '{param_def.name}': must be >= {param_def.minimum}"
                )
            if param_def.maximum is not None and value > param_def.maximum:
                errors.append(
                    f"Parameter '{param_def.name}': must be <= {param_def.maximum}"
                )

        return errors
```

### LLM Prompt Addition

```
## Workflow Parameter Validation

After selecting a workflow and determining parameters, you MUST validate them:

1. Call `validate_workflow_parameters` with the workflow_id and your suggested parameters
2. If validation returns errors:
   - Review each error message
   - Correct the parameters based on the error
   - Call `validate_workflow_parameters` again
3. Only return your final recommendation after parameters pass validation

You have up to 3 validation attempts. If you cannot produce valid parameters
after 3 attempts, return a recommendation with `needs_human_review: true`.
```

### Workflow Engine Validation (Defense-in-Depth)

```go
// pkg/workflowengine/validation.go

func (e *WorkflowEngine) ValidateParametersBeforeExecution(
    ctx context.Context,
    workflowDef *v1alpha1.WorkflowDefinition,
    params map[string]string,
) error {
    var errs []string

    for _, paramDef := range workflowDef.Spec.Parameters {
        value, exists := params[paramDef.Name]

        // Required check
        if paramDef.Required && !exists {
            errs = append(errs, fmt.Sprintf("missing required parameter: %s", paramDef.Name))
            continue
        }

        if !exists {
            continue // Optional, not provided
        }

        // Type validation
        if err := validateType(value, paramDef.Type); err != nil {
            errs = append(errs, fmt.Sprintf("parameter %s: %v", paramDef.Name, err))
        }

        // Enum validation
        if len(paramDef.Enum) > 0 && !contains(paramDef.Enum, value) {
            errs = append(errs, fmt.Sprintf(
                "parameter %s: must be one of %v, got %s",
                paramDef.Name, paramDef.Enum, value,
            ))
        }
    }

    if len(errs) > 0 {
        return &ValidationError{
            Message: "workflow parameter validation failed",
            Errors:  errs,
        }
    }

    return nil
}
```

---

## Why NOT Other Services

### ❌ NOT Remediation Orchestrator

| Reason | Explanation |
|--------|-------------|
| Single Responsibility | RO coordinates flow, doesn't validate execution details |
| No Schema Access | Would need to fetch schema (duplication with WE) |
| Wrong Abstraction Level | RO decides WHAT to do, not HOW to validate |

### ❌ NOT AIAnalysis CRD Controller

| Reason | Explanation |
|--------|-------------|
| Bridge Layer | Should be thin - transform and forward |
| Duplication | Would duplicate HolmesGPT-API validation |
| Late Stage | After LLM session - can't self-correct |

---

## Validation Types by Layer

| Layer | Validation Type | Examples |
|-------|-----------------|----------|
| **HolmesGPT-API** | Schema validation | Required fields, types, enums, ranges |
| **Workflow Engine** | Schema re-validation | Same as above (defense-in-depth) |
| **Tekton Tasks** | Runtime validation | Namespace exists, resource exists, RBAC OK |

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
| 1.0 | 2025-12-01 | Initial decision - HolmesGPT-API primary, WE defense-in-depth |

