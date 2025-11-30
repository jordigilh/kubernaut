# Handoff: Custom Labels Query Structure

**Date**: 2025-11-30
**From**: SignalProcessing Team
**To**: Data Storage Team
**Priority**: 🟡 MEDIUM - Required for V1.0 custom label support

---

## Summary

SignalProcessing has finalized the custom labels extraction design. This document describes the **query structure** Data Storage will receive and how to implement filtering.

**Key Change**: Custom labels are now `map[string][]string` (subdomain → values), not `map[string]string`.

---

## What Data Storage Receives

### Structure

```go
// From HolmesGPT-API search request
type SearchFilters struct {
    // ... mandatory labels ...

    // CustomLabels: subdomain → list of label values
    // Key = subdomain (filter dimension)
    // Value = slice of strings (boolean keys or "key=value" pairs)
    CustomLabels map[string][]string `json:"custom_labels,omitempty"`
}
```

### Example Request (v1.6 - snake_case per DD-WORKFLOW-001)

```json
{
  "query": "OOMKilled critical pod memory exhaustion",
  "filters": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "component": "pod",
    "environment": "production",
    "priority": "P0"
  },
  "custom_labels": {
    "constraint": ["cost-constrained", "stateful-safe"],
    "team": ["name=payments"],
    "region": ["zone=us-east-1"],
    "risk_tolerance": ["low"]
  }
}
```

---

## Label Types

SignalProcessing extracts two types of labels:

| Type | Format | Example | Meaning |
|------|--------|---------|---------|
| **Boolean** | `key` only | `"cost-constrained"` | Presence = true |
| **Key-Value** | `key=value` | `"name=payments"` | Explicit value |

**Note**: `false` booleans are **omitted** by SignalProcessing (not passed to Data Storage).

---

## Required Changes

### 1. Database Schema

Store custom labels as JSONB with the new structure:

```sql
-- Workflow table
CREATE TABLE workflow_catalog (
    -- ... existing columns ...

    -- Custom labels: {"subdomain": ["value1", "value2"], ...}
    custom_labels JSONB
);

-- GIN index for efficient containment queries
CREATE INDEX idx_workflow_custom_labels ON workflow_catalog USING GIN (custom_labels);
```

### 2. Query Generation

Each subdomain becomes a **separate WHERE clause**:

```sql
-- Input: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
-- Output SQL:
SELECT * FROM workflow_catalog
WHERE
    signal_type = 'OOMKilled'
    AND severity = 'critical'
    -- Custom label filters (each subdomain is ANDed)
    AND custom_labels->'constraint' ? 'cost-constrained'
    AND custom_labels->'team' ? 'name=payments';
```

### 3. Multiple Values in Same Subdomain

If a subdomain has multiple values, they are **ORed** within the subdomain:

```sql
-- Input: {"constraint": ["cost-constrained", "stateful-safe"]}
-- Output SQL:
WHERE (
    custom_labels->'constraint' ? 'cost-constrained'
    OR custom_labels->'constraint' ? 'stateful-safe'
);
```

### 4. Go Implementation

```go
func buildCustomLabelFilter(customLabels map[string][]string) string {
    if len(customLabels) == 0 {
        return ""
    }

    var conditions []string
    for subdomain, values := range customLabels {
        if len(values) == 0 {
            continue
        }

        // Multiple values in same subdomain: OR them
        var subConditions []string
        for _, value := range values {
            // Use JSONB containment operator
            subConditions = append(subConditions,
                fmt.Sprintf("custom_labels->'%s' ? '%s'", subdomain, value))
        }

        if len(subConditions) == 1 {
            conditions = append(conditions, subConditions[0])
        } else {
            conditions = append(conditions,
                "("+strings.Join(subConditions, " OR ")+")")
        }
    }

    // Different subdomains: AND them
    return strings.Join(conditions, " AND ")
}
```

---

## Workflow Registration

When workflows are registered, their custom labels should be stored in the same format:

```json
// POST /api/v1/workflows
{
  "workflow_name": "oomkill-increase-memory",
  "custom_labels": {
    "constraint": ["cost-constrained"],
    "team": ["name=payments", "name=platform"]
  }
}
```

**Matching Logic**: A workflow matches if the signal's custom labels are a **subset** of the workflow's custom labels (per subdomain).

---

## Query Examples

### Example 1: Boolean Constraint

**Signal labels**: `{"constraint": ["cost-constrained"]}`

```sql
WHERE custom_labels->'constraint' ? 'cost-constrained'
```

### Example 2: Key-Value Team

**Signal labels**: `{"team": ["name=payments"]}`

```sql
WHERE custom_labels->'team' ? 'name=payments'
```

### Example 3: Multiple Subdomains

**Signal labels**: `{"constraint": ["cost-constrained"], "team": ["name=payments"]}`

```sql
WHERE custom_labels->'constraint' ? 'cost-constrained'
  AND custom_labels->'team' ? 'name=payments'
```

### Example 4: Multiple Values (OR within subdomain)

**Signal labels**: `{"team": ["name=payments", "name=platform"]}`

```sql
WHERE (custom_labels->'team' ? 'name=payments'
       OR custom_labels->'team' ? 'name=platform')
```

---

## No Scoring Impact (V1.0)

Per DD-WORKFLOW-004 v2.2, custom labels are **hard filters only** in V1.0:

- ✅ Custom labels filter workflows (reduce candidate set)
- ❌ Custom labels do NOT affect confidence score
- 🔮 V2.0+: Customer-configurable scoring weights

---

## Questions?

If you have questions about:
- Label format → Contact SignalProcessing Team
- Query semantics → Contact SignalProcessing Team
- Workflow registration format → Contact SignalProcessing Team

---

## References

- **HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md**: Full extraction design
- **DD-WORKFLOW-001 v1.6**: Label schema (snake_case field names)
- **DD-WORKFLOW-004 v2.2**: Scoring (no custom label impact in V1.0)
- **FOLLOWUP_CUSTOM_LABELS_CLARIFICATION.md**: Previous Q&A

---

## Action Items

| # | Action | Owner | Priority |
|---|--------|-------|----------|
| 1 | Update `custom_labels` column to support `map[string][]string` | Data Storage | P1 |
| 2 | Implement subdomain-based WHERE clause generation | Data Storage | P1 |
| 3 | Update workflow registration API to accept new format | Data Storage | P1 |
| 4 | Add GIN index for JSONB containment queries | Data Storage | P1 |
| 5 | Update API documentation | Data Storage | P2 |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-11-30 | Updated to DD-WORKFLOW-001 v1.6 (snake_case field names) |
| 1.0 | 2025-11-30 | Initial handoff - subdomain-based custom labels |

