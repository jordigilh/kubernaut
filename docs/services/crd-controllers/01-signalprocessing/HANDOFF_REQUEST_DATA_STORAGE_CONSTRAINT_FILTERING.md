# Handoff Request: Constraint Label Filtering in Workflow Search

**From**: SignalProcessing Service Team
**To**: Data Storage Service Team
**Date**: November 30, 2025
**Priority**: P2 (Required for V1.0 label integration)
**Status**: 🟡 AWAITING RESPONSE
**Context**: Follow-up from [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v3.0, Question F2

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **1.0** | Nov 30, 2025 | SignalProcessing Team | Initial handoff request |

---

## Summary

SignalProcessing will produce **constraint labels** (e.g., `constraint.kubernaut.io/cost-constrained`, `constraint.kubernaut.io/high-availability`) as part of the `CustomLabels` output. These labels need to influence workflow selection.

**We need clarification on how Data Storage handles these labels in workflow search.**

---

## Context

### Label Flow Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SignalProcessing (V1.0)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│  Output:                                                                     │
│  - DetectedLabels (auto-detected from K8s)                                  │
│  - CustomLabels (Rego-derived, includes constraints)                        │
│                                                                              │
│  Example CustomLabels:                                                       │
│  {                                                                           │
│    "kubernaut.io/team": "payments",                                         │
│    "kubernaut.io/risk-tolerance": "high",                                   │
│    "constraint.kubernaut.io/cost-constrained": "true",                      │
│    "constraint.kubernaut.io/high-availability": "true"                      │
│  }                                                                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AIAnalysis                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│  Passes CustomLabels to HolmesGPT-API in analysis request                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            HolmesGPT-API                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  LLM decides which labels to include in MCP workflow search tool call       │
│                                                                              │
│  MCP Tool Call Example:                                                      │
│  {                                                                           │
│    "tool": "search_workflow_catalog",                                        │
│    "parameters": {                                                           │
│      "query": "OOMKilled memory increase",                                   │
│      "filters": {                                                            │
│        "signal_type": "OOMKilled",                                          │
│        "severity": "critical",                                               │
│        "constraint.kubernaut.io/cost-constrained": "true"  ← QUESTION       │
│      }                                                                       │
│    }                                                                         │
│  }                                                                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Data Storage API                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  POST /api/v1/workflows/search                                               │
│                                                                              │
│  ❓ QUESTION: Does this API support constraint label filtering?              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Questions for Data Storage Team

### Q1: Does the workflow search API support filtering by `constraint.kubernaut.io/*` prefixed labels?

**Example**: Can HolmesGPT-API pass this filter?

```json
{
  "filters": {
    "constraint.kubernaut.io/cost-constrained": "true"
  }
}
```

**Expected behavior**: Only return workflows that have `constraint.kubernaut.io/cost-constrained: true` in their metadata.

---

### Q2: Are constraint labels stored as workflow metadata in Data Storage?

When workflows are created via `POST /api/v1/workflows`, do they include constraint-related labels?

**Example workflow metadata**:
```yaml
metadata:
  labels:
    signal_type: OOMKilled
    severity: critical
    # Constraint labels - are these supported?
    constraint.kubernaut.io/cost-constrained: "true"
    constraint.kubernaut.io/requires-approval: "false"
```

---

### Q3: Does the search API support arbitrary label filtering (beyond the 6 mandatory labels)?

**Current understanding** (DD-WORKFLOW-001 v1.3): 6 mandatory labels are:
1. `signal_type`
2. `severity`
3. `component`
4. `environment`
5. `priority`
6. `risk_tolerance`

**Question**: Can the search API filter by labels **beyond** these 6?

| Label Type | Example | Filterable? |
|------------|---------|-------------|
| Mandatory (6) | `signal_type: OOMKilled` | ✅ Yes (confirmed) |
| Custom | `kubernaut.io/team: payments` | ❓ Unknown |
| Constraint | `constraint.kubernaut.io/cost-constrained: true` | ❓ Unknown |

---

### Q4: What is the filtering strategy?

If constraint filtering IS supported, which approach does Data Storage use?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A) Pre-filter** | Data Storage filters workflows by constraints before returning | Efficient, fewer results to process | Requires schema changes |
| **B) Post-filter** | HolmesGPT-API filters results after receiving from Data Storage | No Data Storage changes | Inefficient, returns unneeded workflows |
| **C) LLM context only** | Constraints inform LLM reasoning, no explicit filtering | Simplest | LLM may select inappropriate workflows |

**SignalProcessing Recommendation**: Option A (pre-filter) is preferred for efficiency.

---

## Impact Assessment

### If Constraint Filtering IS Supported

SignalProcessing can proceed with V1.0 implementation:
- Emit `constraint.kubernaut.io/*` labels in `CustomLabels`
- Document constraint label schema
- No additional work needed

### If Constraint Filtering IS NOT Supported

**Options**:

1. **Data Storage adds support** (Preferred)
   - Add `custom_labels` JSONB column to workflow schema
   - Support arbitrary label filtering in search API
   - Timeline impact: Unknown

2. **HolmesGPT-API handles post-filtering** (Fallback)
   - SignalProcessing emits constraints
   - HolmesGPT-API filters results locally
   - Less efficient but no Data Storage changes

3. **Constraints as LLM context only** (Minimal)
   - Constraints included in LLM prompt
   - No explicit filtering
   - LLM uses judgment

---

## Constraint Label Schema (Proposed)

For reference, here are the constraint labels SignalProcessing will emit:

| Label | Type | Description |
|-------|------|-------------|
| `constraint.kubernaut.io/cost-constrained` | `"true"` / absent | Namespace has budget limits |
| `constraint.kubernaut.io/high-availability` | `"true"` / absent | Workload requires HA-aware remediation |
| `constraint.kubernaut.io/gitops-only` | `"true"` / absent | Changes must go through GitOps |
| `constraint.kubernaut.io/approval-required` | `"true"` / absent | Human approval required |
| `constraint.kubernaut.io/read-only` | `"true"` / absent | Investigation only, no remediation |

**Convention**: Constraints are only present when `"true"`. Absence means no constraint.

---

## Response Requested

Please provide answers to Q1-Q4 above.

**Deadline**: Before SignalProcessing V1.0 implementation begins (blocking for constraint label design)

---

## Related Documents

| Document | Relevance |
|----------|-----------|
| [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) | Source of this handoff (F2) |
| [DD-WORKFLOW-001 v1.3](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | 6 mandatory labels |
| [DD-WORKFLOW-002](../../../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md) | Workflow catalog API contract |
| [HANDOFF_REQUEST_LABEL_SCHEMA_V1.3.md](../../stateless/datastorage/HANDOFF_REQUEST_LABEL_SCHEMA_V1.3.md) | Previous label schema handoff |

---

**Contact**: SignalProcessing Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

