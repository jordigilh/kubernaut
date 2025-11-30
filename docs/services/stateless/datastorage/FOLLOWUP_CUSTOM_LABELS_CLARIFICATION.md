# Follow-up: Custom Labels Architecture Clarification

**From**: Data Storage Service Team
**To**: Signal Processing Service Team
**Date**: November 30, 2025
**Priority**: P2 (Blocking for custom_labels implementation)
**Status**: ✅ ANSWERED
**Context**: Follow-up from [RESPONSE_CONSTRAINT_FILTERING.md](RESPONSE_CONSTRAINT_FILTERING.md)

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **1.0** | Nov 30, 2025 | Data Storage Team | Initial follow-up questions |

---

## Summary

During implementation planning for the `custom_labels` filter in the Data Storage workflow search API, we need clarification on the custom labels architecture. We understand that:

1. Custom labels are **customer-defined** (not hardcoded by kubernaut)
2. Signal Processing **collects and passes through** these labels
3. Data Storage **matches** workflows with the same labels
4. Custom labels must **not overlap** with mandatory/predetermined labels

We need more details to implement this correctly.

---

## Questions for Signal Processing Team

### Q1: Is there an ADR or DD that defines the custom labels architecture?

We're looking for documentation that explains:
- How customers define custom labels (annotations? CRDs? config?)
- The label key format/convention (e.g., `kubernaut.io/*` prefix required?)
- Validation rules for custom label keys
- Examples of customer-defined labels

**Please provide**: Link to ADR/DD or relevant documentation

#### ✅ ANSWER (SignalProcessing Team)

**Authoritative Documents**:

| Document | Purpose |
|----------|---------|
| [DD-WORKFLOW-001 v1.6](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | 5 mandatory labels + customer-derived labels (snake_case) |
| [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0](../../crd-controllers/01-signalprocessing/HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) | **PRIMARY**: Rego-based label extraction architecture |
| [DD-WORKFLOW-004 v2.1](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md) | Scoring strategy (custom labels = hard filter only) |

**Key Points**:
- Custom labels are **customer-defined via Rego policies** (ConfigMap in `kubernaut-system`)
- No CRD needed - Rego policies extract labels from namespace/workload annotations
- Label naming convention documented in HANDOFF v3.0 (see Q5 for prefixes)

---

### Q2: What is the exact format of custom labels passed to Data Storage?

**Example A**: Flat key-value in labels (❌ DEPRECATED)
```json
{
  "labels": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "cost-constrained": "true",
    "team": "payments"
  }
}
```

**Example B**: Structured columns + separate custom_labels field (✅ CURRENT - DD-WORKFLOW-001 v1.6)
```json
{
  "signal_type": "OOMKilled",
  "severity": "critical",
  "component": "pod",
  "environment": "production",
  "priority": "P0",
  "custom_labels": {
    "constraint": ["cost-constrained"],
    "team": ["name=payments"]
  }
}
```

**Question**: Which format does Signal Processing output?

#### ✅ ANSWER (SignalProcessing Team)

**Answer: Example B (Separate fields)**

SignalProcessing outputs in `EnrichmentResults`:

```go
type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    DetectedLabels    *DetectedLabels    `json:"detectedLabels,omitempty"`  // Auto-detected
    CustomLabels      map[string]string  `json:"customLabels,omitempty"`    // Rego-derived
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}
```

**Data Flow to HolmesGPT-API → Data Storage** (per DD-WORKFLOW-001 v1.6):

The 5 mandatory labels (`signal_type`, `severity`, `component`, `environment`, `priority`) are passed as structured filter fields (snake_case). Custom labels are passed separately in `custom_labels` map using subdomain format.

```json
{
  "filters": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "component": "pod",
    "environment": "production",
    "priority": "P0"
  },
  "custom_labels": {
    "risk_tolerance": ["high"],
    "team": ["name=payments"],
    "constraint": ["cost-constrained"]
  }
}
```

**Rationale**: Separation allows:
1. Type-safe validation of mandatory labels
2. Flexible arbitrary key-value for custom labels
3. Clear distinction between system-controlled and customer-defined

---

### Q3: What is the reserved/mandatory label list?

We need the complete list of label keys that are **reserved** and cannot be used as custom labels:

**Our current understanding** (from DD-WORKFLOW-001 v1.6):

| Reserved Label | Type | Storage | Notes |
|----------------|------|---------|-------|
| `signal_type` | Mandatory | Structured column | System-determined |
| `severity` | Mandatory | Structured column | System-determined |
| `component` | Mandatory | Structured column | System-determined |
| `environment` | Mandatory | Structured column | Rego-configurable |
| `priority` | Mandatory | Structured column | Rego-configurable |

**Custom Labels** (stored in `custom_labels` JSONB):

| Custom Label | Example | Notes |
|--------------|---------|-------|
| `risk_tolerance` | `["low"]` | Customer-derived via Rego |
| `business_category` | `["payment-service"]` | Customer-derived via Rego |
| `constraint` | `["cost-constrained"]` | Customer-defined |
| `team` | `["name=payments"]` | Customer-defined |

**Question**: Is this list complete? Are there other reserved keys?

#### ✅ ANSWER (SignalProcessing Team)

**⚠️ UPDATE**: DD-WORKFLOW-001 is now **v1.6** (snake_case API standardization).

**Complete Reserved Label List (DD-WORKFLOW-001 v1.6)**:

| Reserved Label | Type | Who Sets | Notes |
|----------------|------|----------|-------|
| `signal_type` | **Mandatory** | Signal Processing (auto) | Cannot be overridden |
| `severity` | **Mandatory** | Signal Processing (auto) | Cannot be overridden |
| `component` | **Mandatory** | Signal Processing (auto) | Cannot be overridden |
| `environment` | **Mandatory** | Signal Processing (system) | Cannot be overridden |
| `priority` | **Mandatory** | Signal Processing (system) | Cannot be overridden |

**Customer-Derived Labels (NOT reserved, but with conventions)**:

| Label | Type | Notes |
|-------|------|-------|
| `risk_tolerance` | Customer-derived | Via Rego (v1.4 change) |
| `business_category` | Customer-derived | Via Rego |
| `team` | Customer-derived | Via Rego |
| `region` | Customer-derived | Via Rego |
| `constraint.kubernaut.io/*` | Customer-derived | Constraint labels |

**Security Wrapper Deny List** (per HANDOFF v3.0):

```rego
system_labels := {
    "kubernaut.io/priority",
    "kubernaut.io/severity",
    "kubernaut.io/signal_type",
    "kubernaut.io/component",
    "kubernaut.io/environment"
}
```

**Data Storage Validation**: Only need to validate the 5 mandatory labels are present. Custom labels are arbitrary and should NOT be validated against a fixed list.

---

### Q4: How do customers define custom labels?

**Options we're considering**:

| Option | How Customer Defines | Example |
|--------|---------------------|---------|
| **A) Namespace annotations** | `kubernaut.io/cost-constrained: "true"` on namespace | Rego extracts from namespace |
| **B) Workload annotations** | `kubernaut.io/team: "payments"` on Deployment | Rego extracts from workload |
| **C) ConfigMap** | Central config with label definitions | Rego reads from ConfigMap |
| **D) CRD** | Custom resource defining labels | Controller processes CRD |

**Question**: Which mechanism(s) does Signal Processing support?

#### ✅ ANSWER (SignalProcessing Team)

**Answer: Options A, B, and C (Rego-based extraction)**

Per [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0](../../crd-controllers/01-signalprocessing/HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md):

| Option | Supported | Notes |
|--------|-----------|-------|
| **A) Namespace annotations** | ✅ V1.0 | Primary mechanism |
| **B) Workload annotations** | ✅ V1.0 | Pod, Deployment, Node |
| **C) ConfigMap** | ✅ V1.0 | Rego policy stored in ConfigMap |
| **D) CRD** | ❌ Not planned | Too complex, Rego is sufficient |

**How It Works**:

1. **Customer writes Rego policy** → stored in ConfigMap `signal-processing-policies` in `kubernaut-system`
2. **Rego policy extracts labels** from K8s context (namespace, pod, deployment, node)
3. **SignalProcessing evaluates policy** and outputs `CustomLabels` map

**Example Rego Policy** (customer-defined):

```rego
package signalprocessing.labels

# Extract team from namespace label
labels["kubernaut.io/team"] = team {
    team := input.namespace.labels["kubernaut.io/team"]
    team != ""
}

# Derive risk-tolerance from environment
labels["kubernaut.io/risk-tolerance"] = "critical" {
    input.namespace.labels["environment"] == "production"
}
```

**No default policy shipped** - customers define their own based on their infrastructure.

---

### Q5: What validation does Signal Processing perform?

Before passing custom labels to downstream services, does Signal Processing validate:

- [ ] Key format (e.g., must match regex `^[a-z0-9-]+$`)?
- [ ] Key prefix (e.g., must start with `kubernaut.io/`)?
- [ ] No overlap with reserved keys?
- [ ] Value format (e.g., string only, max length)?
- [ ] Maximum number of custom labels?

**Question**: What validation rules should Data Storage also enforce?

#### ✅ ANSWER (SignalProcessing Team)

**SignalProcessing Validation**:

| Validation | Performed? | Notes |
|------------|------------|-------|
| Key format | ⚠️ Partial | Rego output is string keys |
| Key prefix | ✅ **YES** | Security wrapper validates prefixes |
| No overlap with reserved | ✅ **YES** | Security wrapper strips system labels |
| Value format | ⚠️ Partial | Rego outputs strings |
| Max number of labels | ❌ NO | Not enforced |

**Label Naming Convention** (per HANDOFF v3.0):

| Prefix | Purpose | Example |
|--------|---------|---------|
| `kubernaut.io/*` | Standard custom labels | `kubernaut.io/team`, `kubernaut.io/risk-tolerance` |
| `constraint.kubernaut.io/*` | Workflow constraints | `constraint.kubernaut.io/cost-constrained` |
| `custom.kubernaut.io/*` | Explicit customer-defined | `custom.kubernaut.io/business-unit` |

**Security Wrapper** (programmatic enforcement):

```rego
# SignalProcessing strips system labels from customer Rego output
system_labels := {
    "kubernaut.io/priority",
    "kubernaut.io/severity",
    "kubernaut.io/signal_type",
    "kubernaut.io/component",
    "kubernaut.io/environment"
}
```

**Data Storage Validation Recommendation**:

| Validation | Enforce? | Notes |
|------------|----------|-------|
| Key format | ✅ YES | `^[a-zA-Z0-9._/-]+$` (K8s label format) |
| Key prefix | ❌ NO | Any prefix allowed (SignalProcessing handles security) |
| Reserved keys in custom_labels | ❌ NO | SignalProcessing already strips |
| Value format | ✅ YES | String only, max 63 chars (K8s limit) |
| Max custom labels | ⚠️ Optional | Suggest 20 max for performance |

---

### Q6: How should Data Storage handle custom labels in workflow search?

**Our proposed implementation**:

```go
// Search request with custom labels
{
  "query": "OOMKilled memory issue",
  "filters": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "custom_labels": {
      "cost-constrained": "true",
      "team": "payments"
    }
  }
}

// SQL WHERE clause (generic, no hardcoded keys)
WHERE labels->>'signal-type' = 'OOMKilled'
  AND labels->>'severity' = 'critical'
  AND labels->>'cost-constrained' = 'true'
  AND labels->>'team' = 'payments'
```

**Question**: Is this the correct approach? Any concerns?

#### ✅ ANSWER (SignalProcessing Team)

**Answer: ✅ Correct approach with one clarification**

**Use full label keys** (with prefix) in the SQL query:

```sql
-- CORRECT: Use full label keys as stored in workflow labels
WHERE labels->>'signal-type' = 'OOMKilled'
  AND labels->>'severity' = 'critical'
  AND labels->>'kubernaut.io/team' = 'payments'  -- Full key with prefix
  AND labels->>'constraint.kubernaut.io/cost-constrained' = 'true'
```

**Implementation Notes**:

1. **Workflow labels storage**: Workflows store custom labels with full keys (including prefix)
2. **Signal custom labels**: SignalProcessing outputs full keys (e.g., `kubernaut.io/team`)
3. **Match logic**: Exact key-value match between signal custom labels and workflow labels

**GIN Index Usage**:

```sql
-- Your existing GIN index supports this
CREATE INDEX idx_workflow_labels ON remediation_workflow_catalog USING GIN (labels);

-- Query pattern for custom labels
WHERE labels @> '{"kubernaut.io/team": "payments"}'::jsonb
  AND labels @> '{"constraint.kubernaut.io/cost-constrained": "true"}'::jsonb
```

**Alternative (JSONB containment)** - may be faster for multiple custom labels:

```sql
WHERE labels @> $custom_labels_json::jsonb
-- Where $custom_labels_json = '{"kubernaut.io/team": "payments", "constraint.kubernaut.io/cost-constrained": "true"}'
```

---

### Q7: Should custom labels participate in boost/penalty scoring?

**Current design**:
- Mandatory labels (`signal-type`, `severity`): Hard WHERE filter
- Optional labels (`resource-management`, etc.): Boost/penalty scoring
- Custom labels: **Hard WHERE filter only** (no scoring)

**Question**: Is this correct? Or should custom labels also have scoring impact?

#### ✅ ANSWER (SignalProcessing Team)

**Answer: ✅ Correct - Hard filter only for V1.0**

Per [DD-WORKFLOW-004 v2.1](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md):

| Label Type | V1.0 Behavior | Rationale |
|------------|---------------|-----------|
| **5 Mandatory** | Hard WHERE filter | Must match exactly |
| **DetectedLabels** | LLM context only | Inform LLM reasoning, no filtering |
| **Custom Labels** | **Hard WHERE filter** | Exact match required |

**Why No Scoring for Custom Labels?**

1. **Customer-defined keys**: We don't know what keys customers will use
2. **No predefined weights**: Boost/penalty requires predefined weight per key
3. **Simplicity**: Hard filter is deterministic, scoring requires configuration
4. **V2.0+ consideration**: Custom label scoring deferred (requires customer config)

**V2.0+ (Future)**:

```yaml
# Potential V2.0+ customer configuration for custom label scoring
custom_label_scoring:
  "kubernaut.io/team":
    match_boost: 0.05
    mismatch_penalty: -0.02
  "constraint.kubernaut.io/cost-constrained":
    match_boost: 0.10  # Higher priority for constraints
```

**V1.0 Recommendation**: Hard filter only - if custom label is in request, workflow MUST have matching label.

---

## Impact on Data Storage Implementation

| Question | Blocks |
|----------|--------|
| Q1 (ADR reference) | Understanding overall architecture |
| Q2 (Format) | Model definition |
| Q3 (Reserved list) | Validation logic |
| Q4 (Customer definition) | Documentation |
| Q5 (Validation) | Input validation |
| Q6 (Search handling) | Repository implementation |
| Q7 (Scoring) | Scoring algorithm |

---

## Proposed Timeline

Once we have answers:
- **Model changes**: 30 min
- **Repository changes**: 1 hour
- **Validation logic**: 30 min
- **Tests**: 1 hour
- **Documentation**: 30 min
- **Total**: ~3.5 hours

---

## Related Documents

| Document | Relevance |
|----------|-----------|
| [HANDOFF_REQUEST_DATA_STORAGE_CONSTRAINT_FILTERING.md](../../crd-controllers/01-signalprocessing/HANDOFF_REQUEST_DATA_STORAGE_CONSTRAINT_FILTERING.md) | Original handoff |
| [RESPONSE_CONSTRAINT_FILTERING.md](RESPONSE_CONSTRAINT_FILTERING.md) | Our initial response |
| [DD-WORKFLOW-001 v1.6](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | **Mandatory label schema (5 labels, snake_case)** |
| [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0](../../crd-controllers/01-signalprocessing/HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) | **PRIMARY: Custom labels architecture** |
| [DD-WORKFLOW-004 v2.1](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md) | Scoring strategy |

---

## ✅ Summary of Answers

| Question | Answer |
|----------|--------|
| **Q1**: ADR/DD for custom labels | HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0 |
| **Q2**: Format | Separate `custom_labels` field (Example B) |
| **Q3**: Reserved list | 5 mandatory labels (v1.4), security wrapper strips |
| **Q4**: Customer definition | Rego policies (A, B, C supported) |
| **Q5**: Validation | K8s label format, 63 char max, no prefix enforcement |
| **Q6**: Search handling | ✅ Correct (use full keys with prefix) |
| **Q7**: Scoring | Hard filter only (V1.0), scoring deferred to V2.0+ |

---

**Contact**: Data Storage Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

