# Handoff: Custom Labels in AI Analysis Context

**Date**: 2025-11-30
**From**: SignalProcessing Team
**To**: AI Analysis Team
**Priority**: 🟢 LOW - Informational

---

## Summary

SignalProcessing has finalized the custom labels extraction design. This document informs AI Analysis how custom labels will appear in the `SignalProcessingStatus.EnrichmentResults`.

**Note**: AI Analysis receives custom labels as **context** for HolmesGPT. No transformation required.

---

## What AI Analysis Receives

### From SignalProcessing CRD Status

```yaml
apiVersion: signalprocessing.kubernaut.io/v1alpha1
kind: SignalProcessing
status:
  phase: Completed
  enrichmentResults:
    kubernetesContext:
      # ... K8s context ...
    detectedLabels:
      gitOpsManaged: true
      gitOpsTool: "argocd"
      pdbProtected: true
    customLabels:
      constraint:
        - cost-constrained
        - stateful-safe
      team:
        - name=payments
      region:
        - zone=us-east-1
```

### Structure

```go
type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    DetectedLabels    *DetectedLabels    `json:"detectedLabels,omitempty"`

    // CustomLabels: Operator-defined labels via Rego
    // Key = subdomain (category)
    // Value = list of label values (boolean keys or "key=value" pairs)
    CustomLabels map[string][]string `json:"customLabels,omitempty"`

    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}
```

---

## Label Types

| Type | Format | Example | Meaning |
|------|--------|---------|---------|
| **Boolean** | `key` only | `"cost-constrained"` | Constraint is active |
| **Key-Value** | `key=value` | `"name=payments"` | Specific value |

---

## How AI Analysis Uses Custom Labels

### 1. Pass to HolmesGPT-API

When creating the AI analysis request, include custom labels in the signal context:

```go
analysisRequest := &AnalysisRequest{
    SignalContext: SignalContext{
        SignalType:   signalProcessing.Status.SignalType,
        Severity:     signalProcessing.Status.Severity,
        CustomLabels: signalProcessing.Status.EnrichmentResults.CustomLabels,
    },
}
```

### 2. Natural Language Context (Optional)

Custom labels can be expressed in natural language for LLM context:

```
This incident has the following custom constraints:
- constraint: cost-constrained, stateful-safe
- team: payments
- region: us-east-1
```

---

## Distinction: DetectedLabels vs CustomLabels

| Aspect | DetectedLabels | CustomLabels |
|--------|----------------|--------------|
| **Source** | Auto-detected from K8s | Rego policy output |
| **Configuration** | None required | Operator defines Rego |
| **Structure** | Flat struct with typed fields | `map[string][]string` |
| **Purpose** | Context for LLM | Context + workflow filtering |

**Example**:
```yaml
detectedLabels:          # Auto-detected, no config
  gitOpsManaged: true
  pdbProtected: true

customLabels:            # Operator-defined via Rego
  constraint:
    - cost-constrained
  team:
    - name=payments
```

---

## What NOT to Do

| ❌ Don't | ✅ Do |
|----------|-------|
| Parse or validate custom labels | Pass through as-is |
| Assume specific subdomain names | Accept any subdomain |
| Mix with DetectedLabels | Keep separate |

---

## References

- **HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md**: Full extraction design
- **HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0**: Rego policy design
- **DD-WORKFLOW-001 v1.5**: Label schema

---

## Action Items

| # | Action | Owner | Priority |
|---|--------|-------|----------|
| 1 | Update AIAnalysis spec to include `CustomLabels` in signal context | AI Analysis | P2 |
| 2 | (Optional) Express custom labels in LLM prompt | AI Analysis | P3 |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-30 | Initial handoff - custom labels context |

