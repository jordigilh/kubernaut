# TRIAGE: OpenAPI Spec Incomplete - Missing Workflow Endpoints

**Date**: 2025-12-13
**Team**: HAPI
**Priority**: 🔴 **CRITICAL BLOCKER**
**Status**: ⚠️ **MIGRATION BLOCKED**

---

## 🚨 Critical Issue Discovered

**Problem**: The authoritative OpenAPI spec (`api/openapi/data-storage-v1.yaml`) is **INCOMPLETE**.

**Impact**: HAPI's OpenAPI client migration cannot be completed because the generated client is missing critical fields.

---

## 📊 Issue Analysis

### What's Missing

**OpenAPI Spec** (`api/openapi/data-storage-v1.yaml`):
- ✅ Has audit endpoints (`/api/v1/audit/events`)
- ✅ Has incident endpoints (`/api/v1/incidents`)
- ❌ **MISSING**: Workflow search endpoints (`/api/v1/workflows/search`)
- ❌ **MISSING**: Workflow CRUD endpoints (`/api/v1/workflows`)
- ❌ **MISSING**: `WorkflowSearchFilters` schema
- ❌ **MISSING**: `WorkflowSearchRequest` schema
- ❌ **MISSING**: `RemediationWorkflow` schema

### What Was Generated

**Generated Client** (`holmesgpt-api/src/clients/datastorage/models/workflow_search_filters.py`):

```python
class WorkflowSearchFilters(BaseModel):
    signal_type: Optional[StrictStr] = None
    severity: Optional[StrictStr] = None
    environment: Optional[StrictStr] = None
    # ❌ MISSING: component
    # ❌ MISSING: priority
    # ❌ MISSING: custom_labels
    # ❌ MISSING: detected_labels
```

**Expected Fields** (from Data Storage Go code):
```go
type WorkflowSearchFilters struct {
    SignalType     string                       `json:"signal_type"`     // ✅ Generated
    Severity       string                       `json:"severity"`        // ✅ Generated
    Component      string                       `json:"component"`       // ❌ MISSING
    Environment    string                       `json:"environment"`     // ✅ Generated
    Priority       string                       `json:"priority"`        // ❌ MISSING
    CustomLabels   map[string][]string          `json:"custom_labels"`   // ❌ MISSING
    DetectedLabels map[string]interface{}       `json:"detected_labels"` // ❌ MISSING
}
```

---

## 🔍 Root Cause

### Where the Spec Came From

The HAPI team was told by DS team:
> "The authoritative OpenAPI spec is now at `api/openapi/data-storage-v1.yaml`"

**However**: This spec file is **INCOMPLETE** - it only contains audit/incident endpoints, not workflow endpoints.

### What Happened

1. DS team consolidated specs to `api/openapi/data-storage-v1.yaml`
2. HAPI team generated client from this spec
3. Generated client is missing workflow endpoints and fields
4. Unit tests fail because `WorkflowSearchFilters` doesn't have `custom_labels`, `component`, `priority` fields

---

## 💥 Impact Assessment

### Immediate Impact

**HAPI OpenAPI Migration**: ⚠️ **BLOCKED**
- Generated client is incomplete
- Missing 4+ critical fields in `WorkflowSearchFilters`
- Missing entire workflow search API
- Unit tests fail (4 tests)

### Business Logic Impact

**Production Code**: ✅ **STILL WORKS**
- Business logic migration is complete
- Code uses the generated client
- **BUT**: Client is missing fields, so some features may not work

### Test Impact

**Unit Tests**: ❌ **4 FAILING**
- Tests expect `custom_labels` field
- Generated model doesn't have it
- Tests correctly identify the problem

---

## 🎯 Resolution Options

### Option A: DS Team Completes the Spec (RECOMMENDED)

**Action**: DS team adds workflow endpoints to `api/openapi/data-storage-v1.yaml`

**What Needs to be Added**:
1. `/api/v1/workflows/search` endpoint
2. `/api/v1/workflows` CRUD endpoints
3. `WorkflowSearchFilters` schema (with ALL fields)
4. `WorkflowSearchRequest` schema
5. `WorkflowSearchResponse` schema
6. `RemediationWorkflow` schema

**Timeline**: 2-4 hours (DS team)

**Impact**: HAPI can regenerate client with complete schema

**Pros**:
- ✅ Authoritative spec is complete
- ✅ All teams benefit from complete spec
- ✅ API contract fully documented
- ✅ Future client generations work correctly

**Cons**:
- ⏱️ Requires DS team time

---

### Option B: HAPI Uses Old Spec (WORKAROUND)

**Action**: HAPI uses `docs/services/stateless/data-storage/openapi/v3.yaml` (deprecated spec)

**Timeline**: 30 minutes (HAPI team)

**Impact**: HAPI can generate complete client, but using deprecated spec

**Pros**:
- ⚡ Quick fix
- ✅ Unblocks HAPI migration

**Cons**:
- ❌ Using deprecated spec
- ❌ Spec may be out of date
- ❌ Other teams can't use authoritative spec
- ❌ Future maintenance issues

---

### Option C: HAPI Manually Extends Generated Client (HACK)

**Action**: HAPI manually adds missing fields to generated models

**Timeline**: 1 hour (HAPI team)

**Impact**: Generated client is manually patched

**Pros**:
- ⚡ Quick fix
- ✅ Unblocks HAPI migration

**Cons**:
- ❌ Manual patches lost on regeneration
- ❌ Not maintainable
- ❌ Defeats purpose of OpenAPI client
- ❌ Technical debt

---

## 📋 Recommended Action

**Recommendation**: **OPTION A** - DS team completes the OpenAPI spec

**Rationale**:
1. Authoritative spec should be complete
2. All teams benefit from complete spec
3. Sustainable long-term solution
4. Proper API contract documentation

**Handoff to DS Team**:
- Request: Add workflow endpoints to `api/openapi/data-storage-v1.yaml`
- Reference: `docs/services/stateless/data-storage/openapi/v3.yaml` (has workflow endpoints)
- Fields needed: See "What's Missing" section above
- Priority: HIGH (blocks HAPI migration completion)

---

## 🔗 Evidence

### Test Failure Output

```
FAILED test_auto_append_custom_labels_to_filters - AssertionError: assert False
 +  where False = hasattr(WorkflowSearchFilters(...), 'custom_labels')

FAILED test_custom_labels_structure_preserved - AttributeError:
  'WorkflowSearchFilters' object has no attribute 'custom_labels'
```

### Generated Model

```python
# holmesgpt-api/src/clients/datastorage/models/workflow_search_filters.py
class WorkflowSearchFilters(BaseModel):
    signal_type: Optional[StrictStr] = None    # ✅ Present
    severity: Optional[StrictStr] = None       # ✅ Present
    environment: Optional[StrictStr] = None    # ✅ Present
    __properties: ClassVar[List[str]] = ["signal_type", "severity", "environment"]
    # ❌ MISSING: component, priority, custom_labels, detected_labels
```

### OpenAPI Spec Check

```bash
$ grep -i "workflow" api/openapi/data-storage-v1.yaml
# No results - workflow endpoints not in spec
```

---

## 📞 Next Steps

### For HAPI Team (Immediate)

1. ⏸️ **PAUSE** OpenAPI migration completion
2. 📝 **DOCUMENT** this issue (this triage)
3. 🤝 **HANDOFF** to DS team for spec completion
4. ⏳ **WAIT** for DS team to complete spec
5. 🔄 **REGENERATE** client once spec is complete

### For DS Team (Requested)

1. 📖 **REVIEW** `docs/services/stateless/data-storage/openapi/v3.yaml`
2. ➕ **ADD** workflow endpoints to `api/openapi/data-storage-v1.yaml`
3. ✅ **VALIDATE** spec with `openapi-generator validate`
4. 📢 **NOTIFY** HAPI team when complete

---

## 🎯 Success Criteria

**Spec is Complete When**:
- ✅ `/api/v1/workflows/search` endpoint defined
- ✅ `WorkflowSearchFilters` has all 7 fields
- ✅ `WorkflowSearchRequest` schema defined
- ✅ `RemediationWorkflow` schema defined
- ✅ Spec validates with `openapi-generator validate`
- ✅ Generated client has all expected fields

**Migration is Complete When**:
- ✅ Client regenerated from complete spec
- ✅ All 4 unit tests pass
- ✅ Business logic works with complete client
- ✅ No manual patches needed

---

## 📊 Current Status

| Component | Status | Blocker |
|---|---|---|
| OpenAPI Spec | ❌ INCOMPLETE | Missing workflow endpoints |
| Generated Client | ⚠️ PARTIAL | Missing fields |
| Business Logic | ✅ MIGRATED | Works with partial client |
| Unit Tests | ❌ 4 FAILING | Expect complete client |
| Integration Tests | ⏸️ PENDING | Waiting for complete client |

**Overall Status**: ⚠️ **BLOCKED - WAITING ON DS TEAM**

---

**Created**: 2025-12-13
**By**: HAPI Team
**Priority**: 🔴 CRITICAL
**Action Required**: DS team to complete OpenAPI spec

---

## 🔗 Related Documents

- `HAPI_OPENAPI_MIGRATION_COMPLETE.md` - Migration completion report
- `FINAL_HAPI_OPENAPI_MIGRATION_SUMMARY.md` - Migration summary
- `docs/services/stateless/data-storage/openapi/v3.yaml` - Old spec with workflow endpoints
- `api/openapi/data-storage-v1.yaml` - New incomplete spec


