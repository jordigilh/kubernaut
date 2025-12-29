# DD-HAPI-001: Custom Labels Auto-Append Architecture

**Date**: November 30, 2025
**Status**: ✅ APPROVED
**Deciders**: Architecture Team
**Version**: 1.0
**Related**: DD-WORKFLOW-002 v3.2, DD-WORKFLOW-004 v2.1, DD-LLM-001 v1.1, HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md

---

## Context

### Problem Statement

Custom labels (`custom_labels`) are customer-defined filtering constraints extracted by SignalProcessing via Rego policies. These labels must be passed to Data Storage for workflow filtering during MCP `search_workflow_catalog` calls.

**Two architectural options exist**:

1. **LLM-Prompted**: Include custom_labels in LLM prompt, expect LLM to include them in MCP tool call
2. **Auto-Append**: HolmesGPT-API automatically appends custom_labels to MCP tool calls (invisible to LLM)

### Analysis

| Approach | Reliability | Prompt Bloat | LLM Cognitive Load | Risk |
|----------|-------------|--------------|---------------------|------|
| **LLM-Prompted** | ~80-90% | Higher | Higher | LLM may forget/omit |
| **Auto-Append** | **100%** | None | None | None |

**Key Insight**: Custom labels are **filtering constraints**, not **investigation context**. The LLM doesn't need to "think" about them—they're operational metadata that must be passed through unchanged.

---

## Decision

**Use Auto-Append Architecture**: HolmesGPT-API automatically appends `custom_labels` from the original request context to MCP tool calls. Custom labels are invisible to the LLM.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ AIAnalysis Controller                                                        │
│ ───────────────────                                                          │
│ POST /api/v1/incident/analyze                                               │
│ {                                                                            │
│   "remediation_id": "req-2025-11-30-abc123",                                │
│   "enrichment_results": {                                                    │
│     "customLabels": {           ←─── Custom labels from SignalProcessing    │
│       "constraint": ["cost-constrained"],                                    │
│       "team": ["name=payments"]                                             │
│     }                                                                        │
│   }                                                                          │
│ }                                                                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ HolmesGPT-API                                                                │
│ ─────────────                                                                │
│                                                                              │
│ 1. Extract custom_labels from request                                        │
│                                                                              │
│ 2. Create toolset with custom_labels:                                        │
│    WorkflowCatalogToolset(                                                   │
│        remediation_id="req-2025-11-30-abc123",                              │
│        custom_labels={"constraint": ["cost-constrained"], ...}  ←── STORED  │
│    )                                                                         │
│                                                                              │
│ 3. LLM investigates and calls MCP tool:                                      │
│    search_workflow_catalog(query="OOMKilled critical")   ←── NO custom_labels│
│                                                                              │
│ 4. Tool auto-appends custom_labels before calling Data Storage:              │
│    filters = {                                                               │
│        "signal-type": "OOMKilled",                                          │
│        "severity": "critical",                                               │
│        "custom_labels": self._custom_labels  ←── AUTO-APPENDED              │
│    }                                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Data Storage Service                                                         │
│ ────────────────────                                                         │
│ POST /api/v1/workflows/search                                               │
│ {                                                                            │
│   "query": "OOMKilled critical",                                            │
│   "filters": {                                                               │
│     "signal-type": "OOMKilled",                                             │
│     "severity": "critical",                                                  │
│     "custom_labels": {          ←─── Received from HolmesGPT-API            │
│       "constraint": ["cost-constrained"],                                    │
│       "team": ["name=payments"]                                             │
│     }                                                                        │
│   }                                                                          │
│ }                                                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation

### HolmesGPT-API Changes

#### 1. WorkflowCatalogToolset Constructor

```python
class WorkflowCatalogToolset(Toolset):
    def __init__(
        self,
        enabled: bool = True,
        remediation_id: Optional[str] = None,
        custom_labels: Optional[Dict[str, List[str]]] = None  # NEW
    ):
        # Store custom_labels for auto-append
        self._custom_labels = custom_labels or {}

        super().__init__(
            name="workflow/catalog",
            tools=[SearchWorkflowCatalogTool(
                remediation_id=remediation_id,
                custom_labels=custom_labels  # Pass to tool
            )],
            ...
        )
```

#### 2. SearchWorkflowCatalogTool Constructor

```python
class SearchWorkflowCatalogTool(Tool):
    def __init__(
        self,
        data_storage_url: Optional[str] = None,
        remediation_id: Optional[str] = None,
        custom_labels: Optional[Dict[str, List[str]]] = None  # NEW
    ):
        # Store for auto-append (same pattern as remediation_id)
        object.__setattr__(self, '_custom_labels', custom_labels or {})
        ...
```

#### 3. Auto-Append in _search_workflows

```python
def _search_workflows(self, query: str, filters: Dict, top_k: int) -> List[Dict]:
    search_filters = self._build_filters_from_query(query, filters)

    # Auto-append custom_labels (invisible to LLM)
    if self._custom_labels:
        search_filters["custom_labels"] = self._custom_labels

    request_data = {
        "query": query,
        "filters": search_filters,
        "top_k": top_k,
        "remediation_id": self._remediation_id
    }
    ...
```

#### 4. Toolset Registration (in incident/recovery analysis)

```python
def analyze_incident(request_data: dict, config: AppConfig) -> dict:
    # Extract custom_labels from enrichment_results
    enrichment_results = request_data.get("enrichment_results", {})
    custom_labels = enrichment_results.get("customLabels")  # camelCase from K8s

    # Create toolset with custom_labels for auto-append
    workflow_toolset = WorkflowCatalogToolset(
        enabled=True,
        remediation_id=request_data.get("remediation_id"),
        custom_labels=custom_labels  # Auto-appended to all MCP calls
    )
    ...
```

---

## MCP Tool Contract (Updated)

### What LLM Provides

```json
{
  "tool": "search_workflow_catalog",
  "parameters": {
    "query": "OOMKilled critical",
    "filters": {
      "environment": "production",
      "priority": "P0"
    },
    "top_k": 5
  }
}
```

**Note**: `custom_labels` is NOT in the LLM's parameters—it's auto-appended by HolmesGPT-API.

### What Data Storage Receives

```json
{
  "query": "OOMKilled critical",
  "filters": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "environment": "production",
    "priority": "P0",
    "custom_labels": {
      "constraint": ["cost-constrained"],
      "team": ["name=payments"]
    }
  },
  "remediation_id": "req-2025-11-30-abc123",
  "top_k": 5
}
```

---

## LLM Prompt Impact

### What Changes

| Aspect | Before (LLM-Prompted) | After (Auto-Append) |
|--------|----------------------|---------------------|
| **custom_labels in prompt** | Required | ❌ Not needed |
| **LLM guidance** | "Include custom_labels in MCP call" | None required |
| **Prompt length** | Longer | Shorter |
| **LLM cognitive load** | Higher | Lower |

### What Stays the Same

- LLM still determines `signal_type` and `severity` from RCA
- LLM still passes through `environment`, `priority` from input
- LLM still constructs query in `<signal_type> <severity>` format

---

## Rationale

### Why Auto-Append is Better

1. **100% Reliable**: Custom labels are always included, no LLM "forgetting"
2. **Simpler LLM Contract**: LLM focuses on RCA, not operational metadata
3. **Consistent with remediation_id**: Same pattern already used for audit correlation
4. **No Prompt Bloat**: Don't need to explain custom_labels structure to LLM
5. **Future-Proof**: If custom_labels structure changes, only HolmesGPT-API needs updates

### Why NOT LLM-Prompted

1. **Unreliable**: LLM might forget/omit custom_labels (80-90% success rate)
2. **Prompt Complexity**: Must explain map[string][]string structure to LLM
3. **Cognitive Overhead**: LLM must track and reproduce custom_labels correctly
4. **Error-Prone**: LLM might transform/interpret labels (violates pass-through principle)

---

## Cross-References

### DD-WORKFLOW-002 v3.2 Clarification

DD-WORKFLOW-002 v3.2 documents `custom_labels` as an MCP parameter (lines 219-243). This DD **clarifies** that:

- `custom_labels` is **NOT provided by the LLM**
- `custom_labels` is **auto-appended by HolmesGPT-API**
- The MCP tool specification in DD-WORKFLOW-002 describes the **Data Storage contract**, not the LLM contract

### DD-LLM-001 v1.1 No Changes Needed

DD-LLM-001 correctly omits `custom_labels` from LLM parameters. This DD confirms that omission is intentional.

### DD-WORKFLOW-004 v2.1 Alignment

DD-WORKFLOW-004 v2.1 establishes the "Pass-Through Principle":

> "Kubernaut does NOT validate DetectedLabels or CustomLabels values - they are passed through to Data Storage for workflow matching."

This DD implements that principle via auto-append architecture.

---

## Consequences

### Positive

✅ **100% reliable** custom labels filtering
✅ **Simpler LLM contract** (no custom_labels parameter)
✅ **Shorter prompts** (no custom_labels guidance needed)
✅ **Consistent pattern** (same as remediation_id)
✅ **Maintainable** (structure changes don't affect LLM)

### Negative

⚠️ **HolmesGPT-API coupling**: Must know about custom_labels structure
⚠️ **Request context dependency**: Toolset must be created with request context

### Mitigations

- Use constructor injection pattern (proven with remediation_id)
- Document structure in HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md
- Unit tests for auto-append behavior

---

## Testing Strategy

### Unit Tests

- [ ] `WorkflowCatalogToolset` accepts `custom_labels` in constructor
- [ ] `SearchWorkflowCatalogTool` stores `custom_labels` internally
- [ ] `_search_workflows` auto-appends `custom_labels` to filters
- [ ] Empty `custom_labels` is NOT appended (omitted from filters)
- [ ] `custom_labels` structure preserved (map[string][]string)

### Integration Tests (Expected to Fail Until Data Storage Implements)

- [ ] `POST /api/v1/incident/analyze` with `enrichment_results.customLabels`
- [ ] Verify `custom_labels` appears in Data Storage request
- [ ] Verify workflow filtering respects custom labels constraints

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-30 | Initial version - Auto-append architecture approved |

---

**Document Version**: 1.0
**Last Updated**: November 30, 2025
**Status**: ✅ APPROVED
**Next Review**: After Data Storage implements custom_labels filtering

