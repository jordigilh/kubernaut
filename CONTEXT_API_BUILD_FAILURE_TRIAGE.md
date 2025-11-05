# Context API Build Failure Triage

**Date**: 2025-11-05
**Triage Scope**: Context API unit and integration test build failures
**Root Cause**: Data Storage Service "alert" → "signal" terminology migration

---

## 🚨 **EXECUTIVE SUMMARY**

**Status**: ⚠️ **BLOCKING BUILD FAILURES** (3 compilation errors)
**Impact**: Context API cannot build, all unit and integration tests blocked
**Root Cause**: Data Storage Service renamed "alert" terminology to "signal" (completed in commit history)
**Affected Files**: 4 production files, 9 test files

**Recommendation**: ✅ **FIX NOW** (95% confidence) - Simple terminology update, unrelated to ADR-033

---

## 📋 **FAILURE ANALYSIS**

### **Compilation Errors** (BLOCKING)

```
pkg/contextapi/query/executor.go:620:13: inc.AlertName undefined (type *client.Incident has no field or method AlertName)
pkg/contextapi/query/executor.go:629:47: inc.AlertFingerprint undefined (type *client.Incident has no field or method AlertFingerprint)
pkg/contextapi/query/executor.go:635:26: inc.AlertSeverity undefined (type *client.Incident has no field or method AlertSeverity)
```

**Root Cause**: Data Storage Service migrated from "alert" to "signal" terminology:
- `AlertName` → `SignalName`
- `AlertFingerprint` → `SignalFingerprint`
- `AlertSeverity` → `SignalSeverity`

**Evidence**: Data Storage client generated code (`pkg/datastorage/client/generated.go`) shows:
```go
type Incident struct {
    SignalName        string                  `json:"signal_name"`
    SignalFingerprint *string                 `json:"signal_fingerprint,omitempty"`
    SignalSeverity    IncidentSignalSeverity  `json:"signal_severity"`
    // ... (no AlertName, AlertFingerprint, AlertSeverity fields)
}
```

---

## 📂 **AFFECTED FILES**

### **Production Code** (4 files)
1. ✅ `pkg/contextapi/query/executor.go` - 3 field references (lines 620, 629, 635)
2. ⚠️ `pkg/contextapi/models/incident.go` - Unknown usage (needs inspection)
3. ⚠️ `pkg/contextapi/query/types.go` - 1 field reference (line 59)
4. ⚠️ `pkg/contextapi/cache/manager.go` - Unknown usage (needs inspection)

### **Test Code** (9 files)
1. ⚠️ `test/unit/contextapi/executor_datastorage_migration_test.go`
2. ⚠️ `test/unit/contextapi/cache_thrashing_test.go`
3. ⚠️ `test/unit/contextapi/cache_size_limits_test.go`
4. ⚠️ `test/integration/contextapi/helpers.go`
5. ⚠️ `test/unit/contextapi/vector_search_test.go.v1x` (disabled)
6. ⚠️ `test/unit/contextapi/query_builder_test.go.v1x` (disabled)
7. ⚠️ `test/unit/contextapi/models_test.go.v1x` (disabled)
8. ⚠️ `test/unit/contextapi/cache_test.go.v1x` (disabled)
9. ⚠️ `test/unit/contextapi/cache_fallback_test.go.v1x` (disabled)

**Note**: 5 test files are disabled (`.v1x` extension) - likely already known to be broken

---

## 🎯 **DECISION MATRIX: FIX NOW vs. DEFER TO ADR-033**

### **Option A: Fix Now** ⭐ **RECOMMENDED**

**Rationale**:
1. **Blocking Issue**: Cannot build Context API at all (0% functionality)
2. **Simple Fix**: Straightforward terminology update (Alert → Signal)
3. **Unrelated to ADR-033**: Signal terminology is orthogonal to playbook catalog
4. **Low Risk**: No business logic changes, just field name updates
5. **Enables ADR-033 Work**: Must have working Context API to implement ADR-033 features

**Effort Estimate**: 30-45 minutes
- 15 min: Update 4 production files
- 15 min: Update 4 active test files
- 10 min: Run tests and verify
- 5 min: Commit

**Confidence**: **95%** - This is a prerequisite fix, not ADR-033 work

**Pros**:
- ✅ Unblocks Context API development
- ✅ Enables ADR-033 implementation
- ✅ Simple, low-risk change
- ✅ No business logic impact
- ✅ Aligns with Data Storage terminology

**Cons**:
- ⚠️ Requires immediate attention (30-45 min)

---

### **Option B: Defer to ADR-033** ❌ **NOT RECOMMENDED**

**Rationale**:
1. ❌ Cannot implement ADR-033 features without working Context API
2. ❌ Blocking issue prevents any Context API work
3. ❌ Signal terminology is unrelated to ADR-033 (playbook catalog)
4. ❌ Would accumulate more broken code during ADR-033 implementation

**Confidence**: **5%** - Not viable, blocks all Context API work

**Pros**:
- None

**Cons**:
- ❌ Blocks all Context API development
- ❌ Cannot implement ADR-033 features
- ❌ Accumulates technical debt
- ❌ Increases risk of merge conflicts

---

## 📊 **ADR-033 IMPACT ANALYSIS**

### **ADR-033 Changes to Context API**

Based on ADR-033, Context API will need:

1. **New Query Dimensions** (from Data Storage):
   - `incident_type` (NEW - ADR-033 primary dimension)
   - `playbook_id` (NEW - ADR-033 secondary dimension)
   - `playbook_version` (NEW - ADR-033 secondary dimension)
   - `action_type` (EXISTING - ADR-033 tertiary dimension)

2. **New Aggregation Endpoints**:
   - `GET /api/v1/success-rate/incident-type` (BR-STORAGE-031-01)
   - `GET /api/v1/success-rate/playbook` (BR-STORAGE-031-02)
   - `GET /api/v1/success-rate/multi-dimensional` (BR-STORAGE-031-05)

3. **New Response Models**:
   - `IncidentTypeSuccessRateResponse`
   - `PlaybookSuccessRateResponse`
   - `MultiDimensionalSuccessRateResponse`

### **Signal Terminology vs. ADR-033**

| Change Type | Signal Terminology | ADR-033 Playbook Catalog |
|---|---|---|
| **Scope** | Field name updates | New business logic + schema |
| **Complexity** | Simple (find/replace) | Complex (new features) |
| **Risk** | Low (no logic changes) | Medium (new dimensions) |
| **Effort** | 30-45 minutes | 3-5 days (per implementation plan) |
| **Dependency** | **Blocks ADR-033** | Depends on signal terminology |
| **Related** | ❌ Orthogonal | ❌ Independent |

**Conclusion**: Signal terminology fix is a **prerequisite** for ADR-033, not part of it.

---

## 🔧 **RECOMMENDED FIX STRATEGY**

### **Phase 1: Production Code** (15 minutes)

1. **`pkg/contextapi/query/executor.go`** (3 changes)
   ```go
   // Line 620
   - Name: inc.AlertName,
   + Name: inc.SignalName,

   // Line 629
   - AlertFingerprint: stringPtrToString(inc.AlertFingerprint),
   + AlertFingerprint: stringPtrToString(inc.SignalFingerprint),

   // Line 635
   - Severity: string(inc.AlertSeverity),
   + Severity: string(inc.SignalSeverity),
   ```

2. **`pkg/contextapi/query/types.go`** (1 change)
   ```go
   // Line 59
   - AlertFingerprint: r.AlertFingerprint,
   + AlertFingerprint: r.SignalFingerprint,
   ```

3. **`pkg/contextapi/models/incident.go`** (inspect and update)
4. **`pkg/contextapi/cache/manager.go`** (inspect and update)

### **Phase 2: Test Code** (15 minutes)

1. **`test/unit/contextapi/executor_datastorage_migration_test.go`**
2. **`test/unit/contextapi/cache_thrashing_test.go`**
3. **`test/unit/contextapi/cache_size_limits_test.go`**
4. **`test/integration/contextapi/helpers.go`**

**Note**: Skip `.v1x` files (already disabled)

### **Phase 3: Validation** (10 minutes)

```bash
# Build Context API
go build ./pkg/contextapi/...

# Run unit tests
go test ./test/unit/contextapi/... -v

# Run integration tests
make test-integration-contextapi
```

### **Phase 4: Commit** (5 minutes)

```bash
git add pkg/contextapi/ test/unit/contextapi/ test/integration/contextapi/
git commit -m "fix(context-api): Update alert → signal terminology

Aligns Context API with Data Storage Service signal terminology migration.

BREAKING CHANGE: Data Storage Service renamed alert fields to signal fields:
- AlertName → SignalName
- AlertFingerprint → SignalFingerprint
- AlertSeverity → SignalSeverity

This is a prerequisite fix for ADR-033 implementation.

Files Updated:
- pkg/contextapi/query/executor.go (3 field references)
- pkg/contextapi/query/types.go (1 field reference)
- pkg/contextapi/models/incident.go (field references)
- pkg/contextapi/cache/manager.go (field references)
- test/unit/contextapi/*.go (4 test files)
- test/integration/contextapi/helpers.go

Test Results:
- Unit Tests: X passed
- Integration Tests: Y passed

Confidence: 95% - Simple terminology update, no business logic changes"
```

---

## 📊 **CONFIDENCE ASSESSMENT**

### **Fix Now (Option A)** - **95% Confidence** ⭐

**Reasoning**:
1. **Clear Root Cause**: Data Storage signal terminology migration (verified in generated code)
2. **Simple Fix**: Find/replace field names (no logic changes)
3. **Blocking Issue**: Cannot build Context API without this fix
4. **Prerequisite for ADR-033**: Must have working Context API to implement playbook catalog
5. **Low Risk**: No business logic impact, just field name updates
6. **Quick Turnaround**: 30-45 minutes total effort

**Risk Assessment**:
- **Low Risk**: No business logic changes
- **Low Complexity**: Straightforward field name updates
- **High Value**: Unblocks all Context API development

**Success Criteria**:
- ✅ Context API builds successfully
- ✅ Unit tests pass
- ✅ Integration tests pass (or pre-existing failures documented)
- ✅ No new lint errors

---

### **Defer to ADR-033 (Option B)** - **5% Confidence** ❌

**Reasoning**:
1. ❌ Cannot implement ADR-033 without working Context API
2. ❌ Signal terminology is unrelated to playbook catalog
3. ❌ Blocks all Context API development
4. ❌ Accumulates technical debt

**Risk Assessment**:
- **High Risk**: Blocks all Context API work
- **High Complexity**: Would require fixing during ADR-033 implementation
- **Low Value**: Delays unrelated work

---

## ✅ **FINAL RECOMMENDATION**

**Action**: ✅ **FIX NOW** (Option A)
**Confidence**: **95%**
**Effort**: 30-45 minutes
**Priority**: **CRITICAL** (blocking issue)

**Rationale**:
1. Simple, low-risk terminology update
2. Prerequisite for ADR-033 implementation
3. Unblocks all Context API development
4. No business logic changes
5. Aligns with Data Storage terminology

**Next Steps After Fix**:
1. ✅ Context API builds successfully
2. ✅ Unit and integration tests pass
3. ⏭️ Begin ADR-033 implementation (new aggregation endpoints)
4. ⏭️ Implement Context API support for multi-dimensional success tracking

---

## 🔗 **REFERENCES**

- **Data Storage Signal Migration**: Completed in previous commits
- **Data Storage Client**: `pkg/datastorage/client/generated.go`
- **ADR-033**: `docs/architecture/decisions/ADR-033-remediation-playbook-catalog.md`
- **Implementation Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.3.md`

---

**Triage Completed By**: AI Assistant
**Triage Date**: 2025-11-05
**Approval Required**: Technical Lead

