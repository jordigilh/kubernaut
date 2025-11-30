# Response: Constraint Label Filtering in Workflow Search

**From**: Data Storage Service Team
**To**: SignalProcessing Service Team
**Date**: November 30, 2025
**Status**: ✅ RESPONSE PROVIDED
**In Response To**: [HANDOFF_REQUEST_DATA_STORAGE_CONSTRAINT_FILTERING.md](../../crd-controllers/01-signalprocessing/HANDOFF_REQUEST_DATA_STORAGE_CONSTRAINT_FILTERING.md)

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **1.0** | Nov 30, 2025 | Data Storage Team | Initial response |

---

## Executive Summary

**Good news**: The Data Storage workflow search API **already supports arbitrary label filtering** through its JSONB `labels` column. Constraint labels like `constraint.kubernaut.io/cost-constrained` can be stored and filtered **today** with the current implementation.

---

## Answers to Questions

### Q1: Does the workflow search API support filtering by `constraint.kubernaut.io/*` prefixed labels?

**Answer: ✅ YES** (with minor enhancement needed)

**Current State**:
- The `labels` column is stored as **JSONB** in PostgreSQL
- JSONB supports arbitrary key-value pairs
- Any label key can be stored, including `constraint.kubernaut.io/*` prefixed labels

**Current Implementation** (`pkg/datastorage/repository/workflow_repository.go`):
```go
// Labels are stored as JSONB and can contain ANY key-value pairs
labels JSONB NOT NULL DEFAULT '{}'::jsonb
```

**What's Needed**:
The current search API has **hardcoded filter fields** (6 mandatory + 6 optional labels). To support arbitrary constraint labels, we need to add a `custom_labels` filter parameter.

**Proposed Enhancement** (Option A - Pre-filter):
```go
// WorkflowSearchFilters - add custom_labels field
type WorkflowSearchFilters struct {
    // ... existing fields ...

    // CustomLabels allows filtering by arbitrary labels (including constraints)
    // Example: {"constraint.kubernaut.io/cost-constrained": "true"}
    CustomLabels map[string]string `json:"custom_labels,omitempty"`
}
```

**SQL Query Enhancement**:
```sql
-- For each custom label, add WHERE clause
WHERE labels->>'constraint.kubernaut.io/cost-constrained' = 'true'
```

**Timeline**: ~2-4 hours implementation, can be prioritized for V1.0 if needed.

---

### Q2: Are constraint labels stored as workflow metadata in Data Storage?

**Answer: ✅ YES**

**Current Schema** (migration 015):
```sql
-- Labels stored as JSONB - supports ANY key-value pairs
labels JSONB NOT NULL DEFAULT '{}'::jsonb
```

**Example Workflow with Constraint Labels**:
```json
{
  "workflow_name": "oom-recovery-cost-aware",
  "version": "1.0.0",
  "labels": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "constraint.kubernaut.io/cost-constrained": "true",
    "constraint.kubernaut.io/requires-approval": "false"
  }
}
```

**Confirmation**: The JSONB column accepts **any valid JSON object**, so constraint labels will be stored alongside mandatory labels without schema changes.

---

### Q3: Does the search API support arbitrary label filtering (beyond the 6 mandatory labels)?

**Answer: 🟡 PARTIAL (enhancement needed)**

**Current Support**:

| Label Type | Example | Filterable? | Notes |
|------------|---------|-------------|-------|
| **Mandatory (2)** | `signal-type: OOMKilled`, `severity: critical` | ✅ Yes | Mandatory WHERE clause |
| **Optional (6)** | `environment: production`, `priority: p0` | ✅ Yes | Boost scoring (+0.05 to +0.10) |
| **Custom** | `kubernaut.io/team: payments` | ❌ Not yet | Needs `custom_labels` filter |
| **Constraint** | `constraint.kubernaut.io/cost-constrained: true` | ❌ Not yet | Needs `custom_labels` filter |

**Current Filter Fields** (`pkg/datastorage/models/workflow.go`):
```go
type WorkflowSearchFilters struct {
    // MANDATORY (required for search)
    SignalType string  `json:"signal-type" validate:"required"`
    Severity   string  `json:"severity" validate:"required"`

    // OPTIONAL (boost scoring)
    ResourceManagement *string `json:"resource-management,omitempty"`
    GitOpsTool         *string `json:"gitops-tool,omitempty"`
    Environment        *string `json:"environment,omitempty"`
    BusinessCategory   *string `json:"business-category,omitempty"`
    Priority           *string `json:"priority,omitempty"`
    RiskTolerance      *string `json:"risk-tolerance,omitempty"`
}
```

**Proposed Enhancement**:
Add `custom_labels` map field to support arbitrary filtering:
```go
// CustomLabels allows filtering by arbitrary labels (including constraints)
CustomLabels map[string]string `json:"custom_labels,omitempty"`
```

---

### Q4: What is the filtering strategy?

**Answer: Option A (Pre-filter) - RECOMMENDED**

| Option | Description | Data Storage Recommendation |
|--------|-------------|-----------------------------|
| **A) Pre-filter** | Data Storage filters by constraints in SQL | ✅ **RECOMMENDED** |
| B) Post-filter | HolmesGPT-API filters results locally | ❌ Inefficient |
| C) LLM context only | No explicit filtering | ❌ Unreliable |

**Why Option A?**

1. **Efficiency**: PostgreSQL JSONB queries are fast with GIN indexes
2. **Scalability**: Filtering happens in database, not in memory
3. **Consistency**: Same filtering logic for all consumers
4. **Index Support**: We already have a GIN index on labels:
   ```sql
   CREATE INDEX idx_workflow_labels ON remediation_workflow_catalog USING GIN (labels);
   ```

**Implementation Approach**:
```sql
-- Pre-filter constraint labels in WHERE clause
SELECT * FROM remediation_workflow_catalog
WHERE labels->>'signal-type' = 'OOMKilled'
  AND labels->>'severity' = 'critical'
  AND labels->>'constraint.kubernaut.io/cost-constrained' = 'true'  -- Constraint filter
ORDER BY final_score DESC
LIMIT 10;
```

---

## Recommended Implementation Plan

### Phase 1: Add `custom_labels` Filter (V1.0)

**Scope**: Add support for arbitrary label filtering

**Changes Required**:

1. **Model** (`pkg/datastorage/models/workflow.go`):
   ```go
   type WorkflowSearchFilters struct {
       // ... existing fields ...

       // CustomLabels allows filtering by arbitrary labels
       // Keys can include constraint.kubernaut.io/* prefixes
       CustomLabels map[string]string `json:"custom_labels,omitempty"`
   }
   ```

2. **Repository** (`pkg/datastorage/repository/workflow_repository.go`):
   ```go
   // Build WHERE clause for custom labels
   if request.Filters.CustomLabels != nil {
       for key, value := range request.Filters.CustomLabels {
           whereConditions = append(whereConditions,
               fmt.Sprintf("labels->>'%s' = $%d", key, argIndex))
           args = append(args, value)
           argIndex++
       }
   }
   ```

3. **Handler** (`pkg/datastorage/server/workflow_handlers.go`):
   - Parse `custom_labels` from request body
   - Pass to repository

**Estimated Effort**: 2-4 hours

### Phase 2: Constraint Label Documentation (V1.0)

**Scope**: Document constraint label conventions

1. Update DD-WORKFLOW-001 to include constraint label schema
2. Add examples in API documentation
3. Create validation for constraint label format

---

## API Contract Update

### Current Search Request
```json
{
  "query": "OOMKilled memory increase",
  "filters": {
    "signal-type": "OOMKilled",
    "severity": "critical"
  }
}
```

### Enhanced Search Request (with constraints)
```json
{
  "query": "OOMKilled memory increase",
  "filters": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "custom_labels": {
      "constraint.kubernaut.io/cost-constrained": "true",
      "constraint.kubernaut.io/high-availability": "true"
    }
  }
}
```

---

## Impact on SignalProcessing

### What SignalProcessing Can Do Now

1. **Store constraint labels**: Workflows can be created with constraint labels today
2. **Pass constraints to HolmesGPT**: Include in CustomLabels output

### What Requires Data Storage Enhancement

1. **Filter by constraints**: Needs `custom_labels` filter implementation (~2-4h)

### Recommended SignalProcessing Action

**Proceed with V1.0 implementation**:
- Emit `constraint.kubernaut.io/*` labels in `CustomLabels`
- Document constraint label schema
- Data Storage will add `custom_labels` filter support

---

## Timeline

| Task | Owner | Effort | Priority |
|------|-------|--------|----------|
| Add `custom_labels` filter | Data Storage | 2-4h | P1 (V1.0) |
| Update API documentation | Data Storage | 1h | P1 (V1.0) |
| Update DD-WORKFLOW-001 | Architecture | 1h | P1 (V1.0) |
| Integration testing | Both teams | 2h | P1 (V1.0) |

**Total**: ~6-8 hours

---

## Questions for SignalProcessing

1. **Constraint label format**: Should we validate the `constraint.kubernaut.io/` prefix, or accept any label key?

2. **Boost scoring for constraints**: Should constraint labels participate in hybrid scoring (boost/penalty), or only act as hard filters?
   - **Current recommendation**: Hard filters only (no boost/penalty)

3. **Constraint absence handling**: When `constraint.kubernaut.io/cost-constrained` is absent, should we:
   - A) Include the workflow (absence = no constraint)
   - B) Exclude the workflow (explicit `false` required)
   - **Current recommendation**: Option A (absence = no constraint)

---

## Related Documents

| Document | Relevance |
|----------|-----------|
| [DD-WORKFLOW-001 v1.3](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | Mandatory label schema |
| [DD-WORKFLOW-002 v3.0](../../../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md) | Workflow catalog API |
| [DD-WORKFLOW-004](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-scoring.md) | Hybrid scoring |
| [Migration 015](../../../../migrations/015_create_workflow_catalog_table.sql) | JSONB labels schema |

---

**Contact**: Data Storage Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

