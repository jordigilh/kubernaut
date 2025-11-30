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
The current search API has **hardcoded filter fields** (5 mandatory labels per DD-WORKFLOW-001 v1.7). To support DetectedLabels and CustomLabels (pass-through), we need to add JSONB filter parameters.

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

**Example Workflow with Constraint Labels** (v1.6 - snake_case per DD-WORKFLOW-001):
```json
{
  "workflow_name": "oom-recovery-cost-aware",
  "version": "1.0.0",
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

**Confirmation**: The JSONB column accepts **any valid JSON object**, so constraint labels will be stored alongside mandatory labels without schema changes.

---

### Q3: Does the search API support arbitrary label filtering (beyond the 5 mandatory labels)?

**Answer: 🟡 PARTIAL (enhancement needed)**

**Current Support** (per DD-WORKFLOW-001 v1.7):

| Label Type | Example | Filterable? | Notes |
|------------|---------|-------------|-------|
| **Mandatory (5)** | `signal_type: OOMKilled`, `severity: critical`, `component: pod`, `environment: production`, `priority: P0` | ✅ Yes | Structured columns with WHERE clause |
| **Custom** | `custom_labels: {"constraint": ["cost-constrained"]}` | ✅ Yes | JSONB containment filter |

**Filter Fields** (`pkg/datastorage/models/workflow.go`) - snake_case per DD-WORKFLOW-001 v1.7:
```go
type WorkflowSearchFilters struct {
    // 5 MANDATORY (structured columns per DD-WORKFLOW-001 v1.7)
    SignalType  string `json:"signal_type" validate:"required"`
    Severity    string `json:"severity" validate:"required,oneof=critical high medium low"`
    Component   string `json:"component" validate:"required"`
    Environment string `json:"environment" validate:"required"`
    Priority    string `json:"priority" validate:"required"`

    // CUSTOM LABELS (JSONB - includes risk_tolerance, business_category, constraints, etc.)
    CustomLabels map[string][]string `json:"custom_labels,omitempty"`
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

**Implementation Approach** (per DD-WORKFLOW-001 v1.7 - structured columns + JSONB):
```sql
-- Pre-filter using structured columns for mandatory labels + JSONB for custom labels
SELECT * FROM remediation_workflow_catalog
WHERE signal_type = 'OOMKilled'            -- Structured column
  AND severity = 'critical'                 -- Structured column
  AND component = 'pod'                     -- Structured column
  AND (environment = 'production' OR environment = '*')  -- Wildcard support
  AND (priority = 'P0' OR priority = '*')   -- Wildcard support
  AND custom_labels @> '{"constraint": ["cost-constrained"]}'  -- JSONB containment
ORDER BY confidence DESC
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

### Current Search Request (v1.6 - snake_case per DD-WORKFLOW-001)
```json
{
  "query": "OOMKilled memory increase",
  "filters": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "component": "pod",
    "environment": "production",
    "priority": "P0"
  }
}
```

### Enhanced Search Request (with custom labels including constraints)
```json
{
  "query": "OOMKilled memory increase",
  "filters": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "component": "pod",
    "environment": "production",
    "priority": "P0"
  },
  "custom_labels": {
    "constraint": ["cost-constrained", "high-availability"],
    "risk_tolerance": ["low"]
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
| [DD-WORKFLOW-001 v1.7](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) | Mandatory label schema (snake_case API fields) |
| [DD-WORKFLOW-002 v3.0](../../../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md) | Workflow catalog API |
| [DD-WORKFLOW-004](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-scoring.md) | Hybrid scoring |
| [Migration 015](../../../../migrations/015_create_workflow_catalog_table.sql) | JSONB labels schema |

---

**Contact**: Data Storage Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

