# Multi-Environment Workflow Feature - Model 2 Pivot Summary

**Date**: January 28, 2026  
**Status**: ✅ In Progress (Phase 5/8 Complete)  
**Type**: Implementation Status / Handoff Document  
**Location**: `docs/handoff/WORKFLOW_MULTI_ENV_MODEL2_PIVOT_SUMMARY.md`  

---

## 🔄 **What Happened**

### **Original Implementation (Model 1) - INCORRECT** ❌
**Duration**: January 26-28, 2026  
**Version**: DD-WORKFLOW-001 v2.4, DD-WORKFLOW-004 v1.6, BR-STORAGE-040 v1.5

**What we built**:
- **Workflow Storage**: `environment: string` (single value)
- **Search Query**: `environment: []string` (array)
- **SQL**: `WHERE labels->>'environment' IN ('staging', 'production')`
- **Use Case**: HAPI searches multiple environments at once

**Problem**: Signal Processing ALWAYS sends single environment. HAPI never needs to search multiple environments. This was solving a non-existent problem.

---

### **Corrected Implementation (Model 2) - CORRECT** ✅
**Date**: January 28, 2026  
**Version**: DD-WORKFLOW-001 v2.5, DD-WORKFLOW-004 v2.0, BR-STORAGE-040 v2.0

**What we're building**:
- **Workflow Storage**: `environment: []string` (array - workflow declares target environments)
- **Search Query**: `environment: string` (single value from Signal Processing)
- **SQL**: `WHERE labels->'environment' ? 'production' OR labels->'environment' ? '*'`
- **Use Case**: Single workflow reusable across multiple environments

---

## ✅ **Correct Semantics (Model 2)**

### **Workflow Creation**
```json
POST /api/v1/workflows
{
  "labels": {
    "signal_type": "OOMKilled",
    "severity": "high",
    "component": "pod",
    "environment": ["staging", "production"],  // ✅ Works in BOTH
    "priority": "P1"
  }
}
```

### **Signal from Production**
1. **Signal Processing** extracts: `environment: "production"` (single value)
2. **HAPI** calls DataStorage:
   ```json
   POST /api/v1/workflows/search
   {
     "filters": {
       "environment": "production"  // ✅ Single value
     }
   }
   ```
3. **DataStorage** SQL:
   ```sql
   WHERE labels->'environment' ? 'production' OR labels->'environment' ? '*'
   ```
4. **Result**: Finds workflows with:
   - `["staging", "production"]` ✅
   - `["production"]` ✅
   - `["*"]` ✅
   - Does NOT find `["staging"]` ❌

---

## 📝 **Changes Made**

### **Phase 1: Documentation** ✅
- **BR-STORAGE-040**: v1.5 → v2.0 (corrected semantics)
- **DD-WORKFLOW-001**: v2.4 → v2.5 (added comprehensive changelog with examples)
- **DD-WORKFLOW-004**: v1.6 → v2.0 (added SQL examples)

### **Phase 2: Models** ✅
- `MandatoryLabels.Environment`: `string` → `[]string` (storage)
- `WorkflowSearchFilters.Environment`: `[]string` → `string` (search - REVERTED)

### **Phase 3: Repository/SQL** ✅
- Changed SQL from `IN (...)` to JSONB `?` operator
- Added wildcard support: `OR labels->'environment' ? '*'`
- Removed `pq.Array()` imports

### **Phase 4: Handlers** ✅
- `HandleListWorkflows`: Reverted query param parsing (single value)
- `validateCreateWorkflowRequest`: Updated validation logic
- Removed array parsing logic

### **Phase 5: OpenAPI + Clients** 🔄 In Progress
- Update OpenAPI spec
- Regenerate Go ogen client
- Regenerate Python HAPI client

### **Phase 6: HAPI Code** ⏳ Pending
- Remove array wrapping in `workflow_catalog.py`

### **Phase 7: Tests** ⏳ Pending
- Rewrite integration tests
- Create E2E test scenarios

### **Phase 8: Validation** ⏳ Pending
- Run full test suite
- Verify all services compile

---

## 🎯 **Key Design Principles**

1. **Workflow declares scope**: `environment: ["staging", "production"]`
2. **Signal determines context**: Signal from production → search `"production"`
3. **DataStorage matches**: Workflow array CONTAINS search value
4. **Wildcard support**: `["*"]` matches ALL environments
5. **Explicit validation**: `minItems: 1` (no default, author must declare)

---

## 📊 **Impact Assessment**

### **Breaking Changes**
- **MandatoryLabels** schema (storage)
- **WorkflowSearchFilters** schema (search - reverted to original)
- **SQL queries** (IN → JSONB ?)
- **OpenAPI spec** (both directions)

### **No Impact** (Pre-Release)
- No production deployments yet
- No backwards compatibility required
- No data migration needed

---

## 🔗 **References**

- **Business Requirements**: BR-STORAGE-040 v2.0
- **Design Decisions**: DD-WORKFLOW-001 v2.5, DD-WORKFLOW-004 v2.0
- **Implementation**: 
  - Models: `pkg/datastorage/models/workflow*.go`
  - Repository: `pkg/datastorage/repository/workflow/*.go`
  - Handlers: `pkg/datastorage/server/workflow_handlers.go`
  - OpenAPI: `api/openapi/data-storage-v1.yaml`
  - HAPI: `holmesgpt-api/src/toolsets/workflow_catalog.py`

---

## ✅ **Validation Checklist**

- [x] Documentation updated with comprehensive changelogs
- [x] Models reflect correct semantics
- [x] SQL uses JSONB `?` operator
- [x] Handlers updated for single-value search
- [ ] OpenAPI spec updated
- [ ] Clients regenerated
- [ ] HAPI code updated
- [ ] Tests rewritten
- [ ] Full test suite passing

---

**Status**: 50% Complete (4/8 phases done)  
**Next Step**: Update OpenAPI spec and regenerate clients
