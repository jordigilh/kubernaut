# Handoff: Custom Labels Pass-Through

**Date**: 2025-11-30
**From**: SignalProcessing Team
**To**: HolmesGPT-API Team
**Priority**: 🟢 LOW - Minimal changes required

---

## Summary

SignalProcessing has finalized the custom labels extraction design. HolmesGPT-API needs to **pass through** custom labels from the signal context to Data Storage without transformation.

**Key Principle**: HolmesGPT-API is a **conduit, not a transformer**. Custom labels flow unchanged.

---

## What HolmesGPT-API Receives

### From Signal Context (via RemediationRequest CRD)

```json
{
  "signalContext": {
    "signalType": "OOMKilled",
    "severity": "critical",
    "customLabels": {
      "constraint": ["cost-constrained", "stateful-safe"],
      "team": ["name=payments"],
      "region": ["zone=us-east-1"]
    }
  }
}
```

### Structure

```python
# Custom labels structure
custom_labels: dict[str, list[str]]

# Key = subdomain (filter dimension)
# Value = list of strings (boolean keys or "key=value" pairs)
```

---

## Required Changes

### 1. Pass-Through to Data Storage MCP Tool

When calling the workflow catalog search tool, include `custom_labels` in filters:

```python
# WorkflowCatalogTool._invoke()
def _invoke(self, query: str, **kwargs) -> ToolResult:
    filters = {
        "signal-type": self.signal_type,
        "severity": self.severity,
        # ... other mandatory labels ...

        # Pass through custom labels unchanged
        "custom_labels": self.custom_labels  # map[string][]string
    }

    response = self.data_storage_client.search_workflows(
        query=query,
        filters=filters
    )
```

### 2. Include in LLM Context (Optional)

Custom labels can be expressed in the LLM prompt for context:

```python
def build_context_prompt(signal_context: dict) -> str:
    prompt_parts = [
        f"Signal Type: {signal_context['signalType']}",
        f"Severity: {signal_context['severity']}",
    ]

    # Optional: Include custom labels in natural language
    if custom_labels := signal_context.get('customLabels'):
        for subdomain, values in custom_labels.items():
            prompt_parts.append(f"  {subdomain}: {', '.join(values)}")

    return "\n".join(prompt_parts)
```

**Example output**:
```
Signal Type: OOMKilled
Severity: critical
Custom Labels:
  constraint: cost-constrained, stateful-safe
  team: name=payments
```

---

## Label Types

| Type | Format | Example | Meaning |
|------|--------|---------|---------|
| **Boolean** | `key` only | `"cost-constrained"` | Constraint is active |
| **Key-Value** | `key=value` | `"name=payments"` | Specific value |

**Note**: HolmesGPT-API does NOT need to parse these - just pass through.

---

## What NOT to Do

| ❌ Don't | ✅ Do |
|----------|-------|
| Parse `key=value` strings | Pass through as-is |
| Validate subdomain names | Accept any subdomain |
| Transform the structure | Preserve `map[string][]string` |
| Add prefixes | SignalProcessing already extracted |

---

## Data Flow

```
SignalProcessing CRD                    HolmesGPT-API                     Data Storage
─────────────────────                   ─────────────                     ────────────
customLabels:                           customLabels:                     custom_labels:
  constraint:                    →        constraint:               →       constraint:
    - cost-constrained                      - cost-constrained                - cost-constrained
  team:                                   team:                             team:
    - name=payments                         - name=payments                   - name=payments

         (UNCHANGED)                           (UNCHANGED)
```

---

## Python Type Hints

```python
from typing import Dict, List

# Custom labels type
CustomLabels = Dict[str, List[str]]

class SignalContext:
    signal_type: str
    severity: str
    custom_labels: CustomLabels  # Optional
```

---

## Questions?

If you have questions about:
- Label format/meaning → Contact SignalProcessing Team
- Data Storage query behavior → Contact Data Storage Team

---

## References

- **HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md**: Full extraction design
- **DD-WORKFLOW-001 v1.5**: Label schema
- **DD-WORKFLOW-004 v2.2**: Scoring (custom labels = filters only)

---

## Action Items

| # | Action | Owner | Priority |
|---|--------|-------|----------|
| 1 | Update `SearchWorkflowCatalogTool` to pass `custom_labels` | HolmesGPT-API | P2 |
| 2 | Update type hints for `CustomLabels` | HolmesGPT-API | P3 |
| 3 | (Optional) Include custom labels in LLM context prompt | HolmesGPT-API | P3 |

---

---

## Questions from HolmesGPT-API Team

**Date**: 2025-11-30
**Status**: ✅ ANSWERED

### Q1: Field Location in Request Payload

The handoff shows custom labels in `signalContext.customLabels`, but our current implementation receives enrichment data in `enrichment_results.customLabels`.

**Which is the source of truth?**
- A) `request.signalContext.customLabels` (as shown in handoff example)
- B) `request.enrichment_results.customLabels` (current model structure)

**Answer**: ✅ **B) `enrichment_results.customLabels`**

The handoff example was simplified. The actual data flow is:

```
SignalProcessing CRD                    AIAnalysis                         HolmesGPT-API
─────────────────────                   ──────────                         ─────────────
status:                                 passes to API:                     receives:
  enrichmentResults:            →         enrichment_results:        →       enrichment_results:
    customLabels:                           customLabels:                      customLabels:
      constraint: [...]                       constraint: [...]                  constraint: [...]
```

**Source**: `api/signalprocessing/v1alpha1/signalprocessing_types.go` - `EnrichmentResults.CustomLabels`

---

### Q2: Empty Custom Labels Behavior

When no custom labels are present, what should HolmesGPT-API receive?
- A) `null` / `None`
- B) Empty object `{}`
- C) Field omitted entirely

**Answer**: ✅ **C) Field omitted entirely**

This follows Go's `omitempty` JSON serialization and Kubernetes conventions:

```go
CustomLabels map[string][]string `json:"customLabels,omitempty"`
//                                                    ^^^^^^^^^
```

**Behavior**:
- If no Rego labels extracted → field is **not present** in JSON
- HolmesGPT-API should check: `if custom_labels := enrichment_results.get('customLabels'):`
- Treat missing/None as "no custom label filtering"

---

### Q3: Data Storage API Contract

Has the Data Storage team confirmed their `/api/v1/workflows/search` endpoint accepts `custom_labels` as `Dict[str, List[str]]` in the filters payload?

The handoff shows this format but doesn't confirm Data Storage is ready to receive it.

**Answer**: ✅ **Handoff sent, awaiting implementation**

Data Storage team has received their handoff document:
- **Document**: `docs/services/stateless/datastorage/HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md`
- **Status**: Implementation required (P1 priority in their action items)

**Contract from our side is confirmed**:
```json
{
  "filters": {
    "signal-type": "OOMKilled",
    "custom_labels": {
      "constraint": ["cost-constrained"],
      "team": ["name=payments"]
    }
  }
}
```

**Recommendation**: Coordinate with Data Storage team on implementation timeline. HolmesGPT-API can prepare the pass-through logic now; Data Storage will implement the query handling.

---

### Q4: Field Naming Convention

What casing does the CRD/API use?
- A) `customLabels` (camelCase - JSON convention)
- B) `custom_labels` (snake_case - Python convention)

This affects serialization between services.

**Answer**: ✅ **Both, at different layers**

| Layer | Convention | Field Name | Rationale |
|-------|------------|------------|-----------|
| **CRD (Go/K8s JSON)** | camelCase | `customLabels` | Kubernetes API convention |
| **Python internal** | snake_case | `custom_labels` | PEP 8 convention |
| **Data Storage REST API** | snake_case | `custom_labels` | REST API convention |

**Serialization Guidance**:

```python
# Receiving from AIAnalysis (camelCase from K8s)
enrichment_results = request.enrichment_results
custom_labels = enrichment_results.get('customLabels')  # camelCase

# Sending to Data Storage (snake_case REST convention)
filters = {
    "custom_labels": custom_labels  # snake_case
}
```

**Note**: If using Pydantic models with `alias`, you can handle this automatically:
```python
class EnrichmentResults(BaseModel):
    custom_labels: Optional[Dict[str, List[str]]] = Field(None, alias='customLabels')
```

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2025-11-30 | Answered Q1-Q4 from HolmesGPT-API team |
| 1.1 | 2025-11-30 | Added questions from HolmesGPT-API team |
| 1.0 | 2025-11-30 | Initial handoff - pass-through design |

