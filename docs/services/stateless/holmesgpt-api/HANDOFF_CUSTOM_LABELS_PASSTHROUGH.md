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

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-30 | Initial handoff - pass-through design |

