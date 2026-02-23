# Custom Labels Extraction Design - V1.0

**Date**: 2025-11-30
**From**: SignalProcessing Team
**Status**: ✅ **APPROVED** - Ready for Implementation

---

## Summary

This document defines the **authoritative design** for custom label extraction in Kubernaut. SignalProcessing extracts user-defined labels and passes them as a structured `map[string][]string` to downstream services.

**Key Principle**: Kubernaut is a **conduit, not a transformer**. The subdomain structure flows unchanged from Rego to Data Storage.

---

## Label Format

### Raw Label Format (Rego/K8s)

```
<subdomain>.kubernaut.io/<key>[:<value>]
```

| Component | Description | Example |
|-----------|-------------|---------|
| `subdomain` | Category/dimension for filtering | `constraint`, `team`, `region` |
| `.kubernaut.io/` | Kubernaut namespace (hidden from downstream) | *(internal)* |
| `key` | Label identifier | `cost-constrained`, `name`, `zone` |
| `value` | Optional value (empty = boolean true) | `payments`, `us-east-1` |

### Examples

| Raw Label | Subdomain | Key | Value |
|-----------|-----------|-----|-------|
| `constraint.kubernaut.io/cost-constrained` | `constraint` | `cost-constrained` | *(boolean true)* |
| `constraint.kubernaut.io/cost-constrained:true` | `constraint` | `cost-constrained` | *(boolean true)* |
| `team.kubernaut.io/name:payments` | `team` | `name` | `payments` |
| `region.kubernaut.io/zone:us-east-1` | `region` | `zone` | `us-east-1` |
| `compliance.kubernaut.io/pci` | `compliance` | `pci` | *(boolean true)* |

---

## Extraction Rules

### SignalProcessing Extraction Logic

| Input Value | Output | Rationale |
|-------------|--------|-----------|
| Empty string `""` | `key` only | Boolean true (implicit) |
| `"true"` | `key` only | Boolean true (normalized) |
| `"false"` | *(omitted)* | Not included in output |
| Any other value | `key=value` | Key-value pair |

### Output Structure

```go
// CRD Status: EnrichmentResults.CustomLabels
type CustomLabels map[string][]string

// Key = subdomain (filter dimension)
// Value = slice of extracted labels (boolean keys or key=value pairs)
```

### Extraction Example

**Input (Rego output / K8s labels)**:
```yaml
labels:
  constraint.kubernaut.io/cost-constrained: ""
  constraint.kubernaut.io/stateful-safe: "true"
  constraint.kubernaut.io/experimental: "false"   # Will be omitted
  team.kubernaut.io/name: "payments"
  region.kubernaut.io/zone: "us-east-1"
  compliance.kubernaut.io/pci: ""
```

**Output (CRD Status)**:
```json
{
  "customLabels": {
    "constraint": ["cost-constrained", "stateful-safe"],
    "team": ["name=payments"],
    "region": ["zone=us-east-1"],
    "compliance": ["pci"]
  }
}
```

---

## What Kubernaut Hides

The `.kubernaut.io/` suffix is an **implementation detail** hidden from all downstream consumers:

| Layer | Sees | Doesn't See |
|-------|------|-------------|
| Rego Policy | Full label format | N/A (defines it) |
| SignalProcessing CRD | `map[subdomain][]string` | `.kubernaut.io/` |
| HolmesGPT-API | `map[subdomain][]string` | `.kubernaut.io/` |
| Data Storage | `map[subdomain][]string` | `.kubernaut.io/` |

---

## Query Behavior

### Subdomain = Hard Filter Dimension

Each subdomain becomes a **separate WHERE clause** in Data Storage queries:

| CustomLabels | Data Storage Query |
|--------------|-------------------|
| `{"constraint": ["cost-constrained"]}` | `custom_labels->'constraint' ? 'cost-constrained'` |
| `{"team": ["name=payments"]}` | `custom_labels->'team' ? 'name=payments'` |
| `{"constraint": ["cost-constrained"], "team": ["name=payments"]}` | Both conditions ANDed |

### Matching Semantics

| Label Type | Storage | Match |
|------------|---------|-------|
| Boolean (`cost-constrained`) | `["cost-constrained"]` | `? 'cost-constrained'` |
| Key-Value (`name=payments`) | `["name=payments"]` | `? 'name=payments'` |

---

## Operator Freedom

Operators define their own subdomains via Rego policies. Kubernaut does **NOT** enforce or validate subdomain names.

### Recommended Conventions (Documentation Only)

| Subdomain | Use Case | Example Labels |
|-----------|----------|----------------|
| `constraint` | Workflow constraints | `cost-constrained`, `stateful-safe` |
| `team` | Ownership | `name=payments`, `name=platform` |
| `region` | Geographic | `zone=us-east-1`, `zone=eu-west-1` |
| `compliance` | Regulatory | `pci`, `hipaa`, `sox` |

**Operators MAY define any subdomain** - these are suggestions, not requirements.

---

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         REGO POLICY                                 │
│  Outputs: constraint.kubernaut.io/cost-constrained: ""              │
│           team.kubernaut.io/name: "payments"                        │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      SIGNAL PROCESSING                              │
│  Extracts subdomain, hides .kubernaut.io/                           │
│  Outputs: {"constraint": ["cost-constrained"], "team": ["name=..."]}│
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (pass-through, no transformation)
┌─────────────────────────────────────────────────────────────────────┐
│                       HOLMESGPT-API                                 │
│  Receives: {"constraint": ["cost-constrained"], "team": [...]}      │
│  Passes to Data Storage MCP tool call                               │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (pass-through, no transformation)
┌─────────────────────────────────────────────────────────────────────┐
│                       DATA STORAGE                                  │
│  Receives: {"constraint": ["cost-constrained"], "team": [...]}      │
│  Generates: WHERE custom_labels->'constraint' ? 'cost-constrained'  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Industry Alignment

This design follows the **"Conduit, Not Transformer"** pattern used by:

| System | Pattern |
|--------|---------|
| Kubernetes | Labels pass unchanged through the system |
| OpenTelemetry | Baggage/context propagation |
| Envoy/Istio | Header pass-through |
| AWS EventBridge | Event routing without transformation |

---

## Rego Package Convention

Custom label Rego policies must use `package signalprocessing.customlabels` with a `labels` rule that outputs `map[string][]string`:

```rego
package signalprocessing.customlabels

import rego.v1

labels["risk_tolerance"] := [rt] if {
    rt := input.kubernetes.namespace.labels["kubernaut.ai/risk-tolerance"]
    rt != ""
}
```

The engine evaluates `data.signalprocessing.customlabels.labels`. Policies using other package names will not produce output. See `HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md` for the full design rationale.

---

## References

- **DD-WORKFLOW-001 v1.8**: Mandatory label schema + custom labels (snake_case)
- **DD-WORKFLOW-004 v2.2**: Hybrid weighted scoring (custom labels as filters)
- **HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0**: Rego policy design

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-11-30 | Updated to DD-WORKFLOW-001 v1.8 (snake_case field names) |
| 1.0 | 2025-11-30 | Initial design - subdomain extraction, boolean normalization |

