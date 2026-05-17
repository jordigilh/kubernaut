# MCP Tool Contract: `discover_workflows` and `select_workflow`

**Version**: 1.0
**Date**: May 2026
**Status**: Authoritative
**GitHub Issue**: [#1169](https://github.com/jordigilh/kubernaut/issues/1169)
**Related BRs**: BR-INTERACTIVE-001, BR-INTERACTIVE-005, BR-INTERACTIVE-009

---

## Overview

The `kubernaut_investigate` MCP tool supports a `discover_workflows` action that triggers KA's Phase 3 (workflow selection) LLM call. The response surfaces a **recommended** workflow and zero or more **alternative** workflows, each with per-workflow `parameters` populated by the LLM.

A subsequent `kubernaut_select_workflow` call selects one of the discovered workflows for execution. The per-workflow parameters from discovery are merged into the final `InvestigationResult` delivered to the HTTP session store.

---

## `discover_workflows` Action

### Request

```json
{
  "rr_id": "<remediation-request-id>",
  "action": "discover_workflows"
}
```

### Response

The tool returns a JSON envelope with `status: "workflows_discovered"` and a `response` field containing a JSON string with the discovery results.

```json
{
  "status": "workflows_discovered",
  "response": "<inner-json-string>"
}
```

### Inner Response JSON Schema

```json
{
  "recommended": {
    "workflow_id": "string",
    "execution_bundle": "string (optional)",
    "confidence": "number (0-1)",
    "rationale": "string",
    "parameters": { "PARAM_NAME": "value", ... }
  },
  "alternatives": [
    {
      "workflow_id": "string",
      "execution_bundle": "string (optional)",
      "confidence": "number (0-1)",
      "rationale": "string",
      "parameters": { "PARAM_NAME": "value", ... }
    }
  ]
}
```

### Parameter Semantics

| Scenario | JSON representation |
|----------|-------------------|
| LLM provides parameters | `"parameters": {"KEY": "val"}` |
| LLM provides empty parameters | `"parameters": {}` |
| LLM omits parameters | `"parameters"` key absent from JSON |
| LLM provides null parameters | `"parameters": null` |

- Parameters are `map[string]interface{}` internally but are typically string-valued in LLM output.
- Parameter keys are **not validated** against the workflow catalog schema in v1.5. See [#1170](https://github.com/jordigilh/kubernaut/issues/1170) for the validation regression tracking.
- Downstream, AA normalizes `map[string]interface{}` to `map[string]string` via `convertMapToStringMap` before CRD/WFE consumption.

---

## `select_workflow` Tool

### Request

```json
{
  "rr_id": "<remediation-request-id>",
  "workflow_id": "<workflow-id-from-discovery>"
}
```

### Gating Rules (v1.5)

1. `discover_workflows` **must** be called before `select_workflow`.
2. `workflow_id` **must** match either `recommended.workflow_id` or one of `alternatives[].workflow_id`.
3. Any `message` action after `discover_workflows` invalidates the discovery results.

### Parameter Merge Behavior

When `select_workflow` builds the final `InvestigationResult`:

1. If the selected `workflow_id` matches the **recommended** workflow, `recommended.parameters` are used.
2. If the selected `workflow_id` matches an **alternative**, that alternative's `parameters` are used.
3. If the selected `workflow_id` is not found in discovery (graceful fallback), the RCA's original parameters are preserved.
4. If `discovery` is `nil` (legacy path), the RCA's original parameters pass through unchanged.
5. Parameter maps are **cloned** (Go mistake #78) to prevent aliasing between the final result and stored session state.

---

## Backward Compatibility

- Workflows without parameters: the `parameters` key is omitted from the JSON response (Go `omitempty` on `map[string]interface{}`).
- Existing clients that do not read `parameters` are unaffected.
- The `select_workflow` gating rules are unchanged.

---

## Related Documents

- [BR-INTERACTIVE.md](../requirements/BR-INTERACTIVE.md) — Business requirements for interactive mode
- [ADR-045](../architecture/decisions/ADR-045-aianalysis-holmesgpt-api-contract.md) — AIAnalysis HolmesGPT API contract
- [#1170](https://github.com/jordigilh/kubernaut/issues/1170) — Parameter validation regression from HAPI migration
