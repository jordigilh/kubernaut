# Response: CustomLabels Pass-Through Validation

**Date**: 2025-11-30
**From**: HolmesGPT-API Team
**To**: SignalProcessing Team
**Subject**: Re: CustomLabels Pass-Through Validation
**Status**: ✅ All questions answered - Ready for implementation

---

## Original Questions

> **Context**: SignalProcessing will output CustomLabels as `map[string][]string` (subdomain → values).
>
> **Q1**: Is the current HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md sufficient for implementation?
>
> **Q2**: Any open questions about how CustomLabels flows from SignalProcessing → AIAnalysis → HolmesGPT-API → Data Storage?
>
> **Q3**: Does the search_workflow_catalog tool schema need updates to accept custom_labels filter?

---

## Q1: Is the Handoff Document Sufficient?

### ✅ YES - The document is comprehensive and up-to-date (v3.0)

The `HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md` provides everything needed for implementation:

| Section | Coverage |
|---------|----------|
| Data structure | ✅ `map[string][]string` fully documented |
| CRD field location | ✅ `status.enrichmentResults.customLabels` |
| Serialization rules | ✅ `omitempty` behavior documented |
| Data flow diagram | ✅ End-to-end flow shown |
| Type hints | ✅ Python and Go types provided |
| Integration status | ✅ Verified with 55+ tests |

### Key References

| Document | Purpose |
|----------|---------|
| **DD-HAPI-001** | Custom Labels Auto-Append Architecture (authoritative) |
| **DD-WORKFLOW-001 v1.8** | Label schema (snake_case convention) |
| **HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md v3.0** | Implementation guide |

---

## Q2: CustomLabels Data Flow

### ✅ NO OPEN QUESTIONS - Flow is fully implemented and verified

```
┌─────────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  SignalProcessing   │    │   AIAnalysis    │    │  HolmesGPT-API  │    │  Data Storage   │
│        CRD          │    │      CRD        │    │    (Python)     │    │    (Go/SQL)     │
├─────────────────────┤    ├─────────────────┤    ├─────────────────┤    ├─────────────────┤
│ status:             │    │ passes to API:  │    │ receives:       │    │ search filters: │
│   enrichmentResults:│ →  │   enrichment_   │ →  │   enrichment_   │ →  │   custom_labels:│
│     customLabels:   │    │   results:      │    │   results:      │    │     constraint: │
│       constraint:   │    │     customLabels│    │     customLabels│    │       - cost-   │
│         - cost-     │    │       constraint│    │       constraint│    │         constr  │
│           constrained    │         - cost- │    │         - cost- │    │                 │
│                     │    │           constr│    │           constr│    │                 │
├─────────────────────┤    ├─────────────────┤    ├─────────────────┤    ├─────────────────┤
│     (camelCase)     │    │   (camelCase)   │    │  (alias→snake)  │    │   (snake_case)  │
│     Go/K8s JSON     │    │    passthrough  │    │  auto-append    │    │   REST API      │
└─────────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Naming Convention at Each Layer

| Layer | Field Name | Reason |
|-------|------------|--------|
| **CRD (Go/K8s JSON)** | `customLabels` (camelCase) | Kubernetes API convention |
| **AIAnalysis → HolmesGPT-API** | `customLabels` (camelCase) | Passthrough from CRD |
| **Python internal** | `custom_labels` (snake_case) | Handled via Pydantic alias |
| **Data Storage REST API** | `custom_labels` (snake_case) | REST API convention |

### Go CRD Definition (SignalProcessing)

```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go
type EnrichmentResults struct {
    // ... other fields ...

    // CustomLabels extracted from Rego policies
    // +optional
    CustomLabels map[string][]string `json:"customLabels,omitempty"`
}
```

### Python Handling (HolmesGPT-API)

```python
# Already implemented in src/models/incident_models.py
class EnrichmentResults(BaseModel):
    customLabels: Optional[Dict[str, List[str]]] = Field(
        None,
        alias='customLabels'  # Accepts camelCase from K8s
    )
```

### Data Storage Request Format

```json
{
  "query": "OOMKilled critical",
  "filters": {
    "signal_type": "OOMKilled",
    "custom_labels": {
      "constraint": ["cost-constrained", "stateful-safe"],
      "team": ["name=payments"]
    }
  },
  "top_k": 5
}
```

### Empty Labels Behavior

| Scenario | CRD Output | HolmesGPT-API Behavior |
|----------|------------|------------------------|
| No custom labels extracted | Field **omitted** (`omitempty`) | No custom_labels filter applied |
| Empty map | Field **omitted** (`omitempty`) | No custom_labels filter applied |
| Has labels | Field **present** with data | Auto-appends to Data Storage query |

---

## Q3: Tool Schema Updates Required?

### ❌ NO - Custom labels are NOT exposed to the LLM

Per **DD-HAPI-001: Custom Labels Auto-Append Architecture**, custom labels are automatically appended by HolmesGPT-API and are invisible to the LLM.

### Why Auto-Append?

| Approach | Reliability | Status |
|----------|-------------|--------|
| ~~LLM provides custom_labels in tool call~~ | ~80-90% | ❌ **REJECTED** |
| **HolmesGPT-API auto-appends to query** | **100%** | ✅ **IMPLEMENTED** |

**Rationale**:
- Custom labels are operational metadata, not investigation context
- LLM shouldn't need to know/reproduce filter dimensions
- Auto-append guarantees 100% inclusion vs LLM potentially forgetting

### Current LLM Tool Parameters

The `search_workflow_catalog` tool does **NOT** include `custom_labels`:

```json
{
  "name": "search_workflow_catalog",
  "description": "Search for remediation workflows matching the RCA findings",
  "parameters": {
    "query": {
      "type": "string",
      "description": "Structured query: '<signal_type> <severity> [keywords]'"
    },
    "rca_resource": {
      "type": "object",
      "description": "RCA resource for DetectedLabels validation",
      "properties": {
        "signal_type": { "type": "string" },
        "kind": { "type": "string" },
        "namespace": { "type": "string" },
        "name": { "type": "string" }
      }
    }
  }
}
```

### Auto-Append Implementation

```python
# src/toolsets/workflow_catalog.py - SearchWorkflowCatalogTool._search_workflows()
def _search_workflows(self, query: str, filters: dict, ...):
    search_filters = {
        "signal_type": filters.get("signal_type", ""),
        "severity": filters.get("severity", ""),
        # ... other mandatory filters ...
    }

    # DD-HAPI-001: Auto-append custom_labels (invisible to LLM)
    if self._custom_labels:
        search_filters["custom_labels"] = self._custom_labels

    # Send to Data Storage
    response = requests.post(
        f"{self._data_storage_url}/api/v1/workflows/search",
        json={"query": query, "filters": search_filters, ...}
    )
```

---

## Summary for SignalProcessing Implementation

### Checklist

| Item | Status | Notes |
|------|--------|-------|
| Data structure | ✅ Ready | `map[string][]string` confirmed |
| CRD field location | ✅ Ready | `status.enrichmentResults.customLabels` |
| JSON serialization | ✅ Ready | Use `omitempty`, field omitted if empty |
| HolmesGPT-API | ✅ Implemented | Auto-appends to Data Storage query |
| Data Storage | ✅ Implemented | JSONB containment filtering |
| Integration tests | ✅ Passing | 55+ tests cover the full flow |

### What SignalProcessing Needs to Do

1. **Extract custom labels** via Rego policies
2. **Store in CRD status**: `status.enrichmentResults.customLabels`
3. **Use `omitempty`**: Omit field if no labels extracted
4. **Format**: `map[string][]string` (subdomain → list of values)

### Example CRD Output

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: SignalProcessing
metadata:
  name: sp-abc123
status:
  phase: Enriched
  enrichmentResults:
    kubernetesContext:
      # ... K8s details ...
    detectedLabels:
      gitOpsManaged: true
      gitOpsTool: "argocd"
    customLabels:                    # <-- YOUR OUTPUT
      constraint:
        - cost-constrained
        - stateful-safe
      team:
        - name=payments
      region:
        - zone=us-east-1
```

---

## References

| Document | Location |
|----------|----------|
| Custom Labels Handoff | `docs/services/stateless/holmesgpt-api/HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md` |
| Auto-Append Architecture | `docs/architecture/decisions/DD-HAPI-001-custom-labels-auto-append.md` |
| Label Schema | `docs/architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md` |
| MCP Workflow Architecture | `docs/architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md` |

---

## Questions?

If you have follow-up questions:
- **Label format/meaning**: Contact SignalProcessing Team (internal discussion)
- **Data Storage query behavior**: Contact Data Storage Team
- **HolmesGPT-API integration**: Contact HolmesGPT-API Team

**Conclusion**: ✅ **You can proceed with implementation. No HolmesGPT-API changes required.**

