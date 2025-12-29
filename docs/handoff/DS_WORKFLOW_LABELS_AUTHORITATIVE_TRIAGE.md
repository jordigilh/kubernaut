# 🔍 **Workflow Labels - Authoritative Documentation Triage**

**Date**: 2025-12-17
**Service**: DataStorage
**Objective**: Determine if workflow labels need custom label support for V1.0
**Authority**: DD-WORKFLOW-001 v1.6 (Mandatory Label Schema)
**Status**: ✅ **TRIAGE COMPLETE**

---

## 🎯 **Key Finding: V1.0 Scope is Mandatory Labels Only**

**From DD-WORKFLOW-001 (lines 1100-1104)**:

> **Key Requirements**:
> 1. **V1.0 Scope**: Support mandatory labels only (no custom labels)
> 2. **Deterministic Filtering**: Labels must enable SQL-based filtering before semantic search
> 3. **Signal Matching**: Labels must align with Signal Processing categorization output
> 4. **Future Extensibility**: Schema must support custom labels in V1.1

**Verdict**: ✅ **V1.0 does NOT require custom labels** - Deferred to V1.1

---

## 📊 **Authoritative Schema (DD-WORKFLOW-001 lines 1160-1199)**

### **V1.4 Schema with 5 Mandatory Structured Labels + Custom Labels JSONB**

```sql
CREATE TYPE severity_enum AS ENUM ('critical', 'high', 'medium', 'low', '*');
CREATE TYPE environment_enum AS ENUM ('production', 'staging', 'development', 'test', '*');
CREATE TYPE priority_enum AS ENUM ('P0', 'P1', 'P2', 'P3', '*');

CREATE TABLE workflow_catalog (
    workflow_id       TEXT NOT NULL,
    version           TEXT NOT NULL,
    title             TEXT NOT NULL,
    description       TEXT,

    -- 5 Mandatory structured labels (V1.4) - 1:1 matching with wildcard support
    -- Group A: Auto-populated from K8s/Prometheus
    signal_type       TEXT NOT NULL,              -- OOMKilled, CrashLoopBackOff, NodeNotReady
    severity          severity_enum NOT NULL,     -- critical, high, medium, low
    component         TEXT NOT NULL,              -- pod, deployment, node, service, pvc
    -- Group B: Rego-configurable
    environment       environment_enum NOT NULL,  -- production, staging, development, test, '*'
    priority          priority_enum NOT NULL,     -- P0, P1, P2, P3, '*'

    -- Validation constraints
    CHECK (signal_type ~ '^[A-Za-z0-9-]+$'),  -- Exact K8s event reason (no transformation)
    CHECK (component ~ '^[a-z0-9-]+$'),

    -- Custom labels (user-defined via Rego, stored in JSONB)
    -- Format: map[subdomain][]string per V1.5
    -- Examples: risk_tolerance, business_category, team, region
    custom_labels     JSONB,

    embedding         vector(384),
    status            TEXT NOT NULL DEFAULT 'active',

    PRIMARY KEY (workflow_id, version)
);

-- Composite index for efficient label filtering (5 mandatory labels per v1.4)
CREATE INDEX idx_workflow_labels ON workflow_catalog (
    signal_type, severity, component, environment, priority
);

-- GIN index for custom label queries
CREATE INDEX idx_workflow_custom_labels ON workflow_catalog USING GIN (custom_labels);
```

---

## 🔍 **What This Means for DataStorage V1.0**

### **Current Implementation (Incorrect)**

**File**: `pkg/datastorage/models/workflow.go`

```go
// Current: Uses JSONB for ALL labels (unstructured)
type RemediationWorkflow struct {
    Labels json.RawMessage `json:"labels" db:"labels"` // JSONB in PostgreSQL
}

func (w *RemediationWorkflow) GetLabelsMap() (map[string]interface{}, error) {
    var labels map[string]interface{}
    if err := json.Unmarshal(w.Labels, &labels); err != nil {
        return nil, err
    }
    return labels, nil
}
```

**Problem**:
- ❌ **Mixes mandatory and custom labels** in one JSONB column
- ❌ **No type safety** for mandatory labels
- ❌ **Slower queries** (JSONB parsing vs indexed columns)
- ❌ **No validation** of mandatory label enums

---

### **V1.0 Correct Implementation (Mandatory Labels Only)**

**Recommendation**: Use structured types for 5 mandatory labels, **defer custom labels to V1.1**

```go
// V1.0: Structured mandatory labels (no custom labels)
type WorkflowMandatoryLabels struct {
    SignalType  string `json:"signal_type" db:"signal_type" validate:"required"`
    Severity    string `json:"severity" db:"severity" validate:"required,oneof=critical high medium low *"`
    Component   string `json:"component" db:"component" validate:"required"`
    Environment string `json:"environment" db:"environment" validate:"required,oneof=production staging development test *"`
    Priority    string `json:"priority" db:"priority" validate:"required,oneof=P0 P1 P2 P3 *"`
}

type RemediationWorkflow struct {
    // ... other fields ...

    // V1.0: Mandatory labels only (structured)
    Labels WorkflowMandatoryLabels `json:"labels" db:"labels"`

    // V1.1: Custom labels (defer to V1.1)
    // CustomLabels json.RawMessage `json:"custom_labels,omitempty" db:"custom_labels"`
}
```

**Benefits for V1.0**:
- ✅ **Type safety**: Compile-time validation of mandatory labels
- ✅ **Faster queries**: Indexed columns instead of JSONB parsing
- ✅ **Enum validation**: Severity/environment/priority validated
- ✅ **Compliance**: Matches DD-WORKFLOW-001 V1.0 scope

---

### **V1.1 Future Implementation (Add Custom Labels)**

```go
// V1.1: Add custom labels support (hybrid approach)
type RemediationWorkflow struct {
    // ... other fields ...

    // V1.0: Mandatory labels (structured)
    Labels WorkflowMandatoryLabels `json:"labels" db:"labels"`

    // V1.1: Custom labels (JSONB for flexibility)
    CustomLabels map[string]interface{} `json:"custom_labels,omitempty" db:"custom_labels"`
}
```

---

## 📊 **V1.0 vs V1.1 Comparison**

| Aspect | V1.0 (Mandatory Only) | V1.1 (Mandatory + Custom) |
|--------|----------------------|---------------------------|
| **Mandatory Labels** | 5 structured fields | 5 structured fields |
| **Custom Labels** | ❌ Not supported | ✅ JSONB column |
| **Type Safety** | ✅ 100% for mandatory | ✅ Mandatory only |
| **Query Performance** | ✅ Fast (indexed) | ✅ Mandatory fast, JSONB slower |
| **Validation** | ✅ Enum validation | ✅ Enum for mandatory |
| **Flexibility** | ❌ Limited to 5 labels | ✅ Unlimited custom |

---

## 🎯 **V1.0 Action Plan**

### **Phase 1: Fix Workflow Labels for V1.0 (4-6 hours)**

1. ✅ **Create WorkflowMandatoryLabels struct** in `pkg/datastorage/models/workflow.go`
2. ✅ **Update RemediationWorkflow struct** to use structured labels
3. ✅ **Update database queries** to use structured columns
4. ✅ **Add enum validation** for severity/environment/priority
5. ✅ **Update tests** to use structured labels
6. ✅ **Remove GetLabelsMap/SetLabelsFromMap** methods (no longer needed)

### **Phase 2: Add Custom Labels for V1.1 (Deferred)**

7. ⏭️ **Add custom_labels JSONB column** to database schema
8. ⏭️ **Add CustomLabels field** to RemediationWorkflow struct
9. ⏭️ **Update API endpoints** to accept custom labels
10. ⏭️ **Add GIN index** for JSONB queries

---

## 📋 **Migration Path**

### **Database Migration (V1.0)**

```sql
-- V1.0: Split JSONB labels into structured columns
ALTER TABLE remediation_workflow_catalog
    ADD COLUMN signal_type TEXT,
    ADD COLUMN severity TEXT,
    ADD COLUMN component TEXT,
    ADD COLUMN environment TEXT,
    ADD COLUMN priority TEXT;

-- Migrate existing JSONB data to structured columns
UPDATE remediation_workflow_catalog
SET
    signal_type = (labels->>'signal_type'),
    severity = (labels->>'severity'),
    component = (labels->>'component'),
    environment = (labels->>'environment'),
    priority = (labels->>'priority');

-- Make mandatory columns NOT NULL
ALTER TABLE remediation_workflow_catalog
    ALTER COLUMN signal_type SET NOT NULL,
    ALTER COLUMN severity SET NOT NULL,
    ALTER COLUMN component SET NOT NULL,
    ALTER COLUMN environment SET NOT NULL,
    ALTER COLUMN priority SET NOT NULL;

-- Add indexes for fast filtering
CREATE INDEX idx_workflow_mandatory_labels ON remediation_workflow_catalog (
    signal_type, severity, component, environment, priority
);

-- Drop old JSONB labels column (V1.0 scope)
ALTER TABLE remediation_workflow_catalog DROP COLUMN labels;
```

---

## 🚨 **Critical Decision for V1.0**

### **Question**: Should we implement V1.0 or V1.1 schema for initial release?

**Option A: V1.0 Schema (Mandatory Labels Only)** ✅ RECOMMENDED
- ✅ **Matches DD-WORKFLOW-001 V1.0 scope**
- ✅ **Simpler implementation** (4-6 hours)
- ✅ **Type-safe** for all V1.0 features
- ✅ **Faster queries** (no JSONB overhead)
- ⚠️ **Migration needed for V1.1** (add custom_labels column)

**Option B: V1.1 Schema (Mandatory + Custom Labels)**
- ✅ **Future-proof** (no migration needed)
- ⚠️ **More complex** (6-8 hours)
- ⚠️ **Hybrid type safety** (mandatory structured, custom unstructured)
- ⚠️ **Implementing V1.1 features in V1.0** (scope creep)

---

## 🎯 **Recommendation**

**Implement Option A: V1.0 Schema (Mandatory Labels Only)**

**Rationale**:
1. ✅ **Authoritative Documentation**: DD-WORKFLOW-001 explicitly states "V1.0 Scope: Support mandatory labels only (no custom labels)"
2. ✅ **Zero Technical Debt**: Pure structured types for V1.0 scope
3. ✅ **Type Safety**: 100% compile-time validation for V1.0 features
4. ✅ **Performance**: Fastest queries for V1.0 use cases
5. ✅ **Clean Migration**: V1.1 adds custom_labels column without breaking changes

**V1.0 Implementation**:
- ✅ 5 structured label columns (signal_type, severity, component, environment, priority)
- ❌ No custom_labels column (defer to V1.1)
- ✅ Type-safe Go structs
- ✅ Enum validation

**V1.1 Migration**:
- Add `custom_labels JSONB` column
- Add GIN index for JSONB queries
- Update Go struct to include `CustomLabels` field
- No breaking changes to mandatory labels

---

## 📚 **Authoritative References**

1. **DD-WORKFLOW-001 v1.6** - Mandatory Label Schema (lines 1100-1199)
   - V1.0 scope: Mandatory labels only
   - V1.1 extensibility: Custom labels via JSONB

2. **ADR-043** - Workflow Schema Definition Standard (lines 167-170)
   - Custom labels are OPTIONAL
   - Format: `[custom_key]: string`

3. **DD-STORAGE-008** - Playbook Catalog Schema (lines 391)
   - Current implementation: `Labels map[string]string` (incorrect for V1.0)

---

## ✅ **Conclusion**

**For V1.0**: Use **pure structured types** (no JSONB custom labels)

**Justification**:
- ✅ Matches DD-WORKFLOW-001 V1.0 scope
- ✅ Zero technical debt
- ✅ Type-safe
- ✅ Fast queries
- ✅ Clean V1.1 migration path

**Next Step**: Implement WorkflowMandatoryLabels struct and migrate from JSONB labels to structured columns.

---

**Confidence Assessment**: **100%**
**Authority**: DD-WORKFLOW-001 v1.6 (authoritative label schema)
**User Request Met**: ✅ YES - Custom labels are V1.1 feature, not V1.0



