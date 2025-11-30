# Handoff Request: Label Passthrough from Gateway to SignalProcessing

**From**: SignalProcessing Service Team
**To**: Gateway Service Team
**Date**: November 30, 2025
**Priority**: P3 (Informational / Alignment)
**Status**: 🟡 AWAITING RESPONSE
**Context**: Follow-up from [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v3.0

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **1.0** | Nov 30, 2025 | SignalProcessing Team | Initial handoff request |

---

## Summary

SignalProcessing V1.0 will implement **DetectedLabels** (auto-detected) and **CustomLabels** (Rego-derived) as part of the enrichment phase. This handoff clarifies the label flow from Gateway through SignalProcessing.

**We need confirmation that the current Gateway → SignalProcessing interface is sufficient.**

---

## Context

### Current Label Flow Understanding

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Gateway Service                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  Receives: Prometheus/Grafana webhook                                        │
│  Extracts: signal_type, severity (from alert labels)                        │
│  Sets: Placeholder priority (P2) - refined by SignalProcessing              │
│  Creates: RemediationRequest CRD                                             │
│                                                                              │
│  ❓ QUESTION: Does Gateway pass through any namespace labels?                │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SignalProcessing (V1.0)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│  Receives: RemediationRequest reference                                      │
│  Enriches: K8s context (namespace, pod, deployment, node)                   │
│  Auto-detects: DetectedLabels (GitOps, PDB, HPA, etc.)                      │
│  Rego-derives: CustomLabels (risk-tolerance, team, constraints)             │
│                                                                              │
│  Output: EnrichmentResults with all labels                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Questions for Gateway Team

### Q1: What labels does Gateway currently extract from webhooks?

**Current understanding** (per DD-CATEGORIZATION-001):

| Label | Source | Set by Gateway? |
|-------|--------|-----------------|
| `signal_type` | Alert labels | ✅ Yes |
| `severity` | Alert labels | ✅ Yes |
| `component` | Alert labels | ✅ Yes (if available) |
| `priority` | Derived | ✅ Yes (placeholder P2) |
| `namespace` | Alert labels | ✅ Yes |
| Namespace labels | K8s API? | ❓ Unknown |

**Question**: Does Gateway query K8s API for namespace labels, or does SignalProcessing need to fetch all namespace metadata?

---

### Q2: Does Gateway pass through alert annotations?

Prometheus alerts can include annotations that might be useful for Rego policies:

```yaml
# Prometheus alert example
annotations:
  summary: "Pod OOMKilled"
  description: "Container exceeded memory limits"
  runbook_url: "https://runbooks.company.com/oom"
  kubernaut.io/team: "payments"  # Custom annotation
```

**Question**: Are alert annotations included in the RemediationRequest spec, or are they discarded by Gateway?

---

### Q3: Is the current SignalProcessingSpec sufficient for label extraction?

SignalProcessing V1.0 needs to build a Rego input with:

| Field | Source | Available in Spec? |
|-------|--------|-------------------|
| `namespace.name` | RemediationRequest | ✅ Yes |
| `namespace.labels` | K8s API query | ❓ SignalProcessing fetches |
| `pod.name` | RemediationRequest | ✅ Yes |
| `pod.labels` | K8s API query | ❓ SignalProcessing fetches |
| `signal.type` | RemediationRequest | ✅ Yes |
| `signal.severity` | RemediationRequest | ✅ Yes |

**Confirmation needed**: SignalProcessing will query K8s API for labels/annotations. Gateway only provides signal metadata and resource identifiers.

---

### Q4: Any plans to add label extraction to Gateway?

Per DD-CATEGORIZATION-001, categorization is consolidated in SignalProcessing. However:

**Question**: Are there any planned Gateway changes that might affect label flow?

| Scenario | Impact on SignalProcessing |
|----------|---------------------------|
| Gateway adds namespace label passthrough | SignalProcessing could skip K8s query |
| Gateway remains unchanged | SignalProcessing queries K8s for all labels |
| Gateway adds Rego support | Potential duplication with SignalProcessing |

---

## Current Assumption

Based on DD-CATEGORIZATION-001, SignalProcessing assumes:

1. **Gateway provides**: Signal metadata (type, severity, namespace, resource identifiers)
2. **Gateway does NOT provide**: Namespace labels, pod labels, annotations
3. **SignalProcessing fetches**: All K8s context via API queries

**If this assumption is incorrect, please clarify.**

---

## No Changes Required (Likely)

This handoff is primarily for **alignment and confirmation**. SignalProcessing does not expect Gateway changes for V1.0.

However, if Gateway already passes through labels that SignalProcessing will re-query, we should avoid duplicate API calls.

---

## Related Documents

| Document | Relevance |
|----------|-----------|
| [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) | SignalProcessing V1.0 label implementation |
| [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) | Gateway/SignalProcessing responsibility split |
| [Gateway Processing](../../../pkg/gateway/processing/) | Current Gateway implementation |

---

## Response Requested

Please confirm or correct our understanding (Q1-Q4 above).

**Deadline**: Informational - no blocking dependencies, but early clarification appreciated.

---

**Contact**: SignalProcessing Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

