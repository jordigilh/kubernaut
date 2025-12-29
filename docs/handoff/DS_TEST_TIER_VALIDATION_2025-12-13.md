# Data Storage - Test Tier Validation Results

**Date**: 2025-12-13
**Context**: Post-OpenAPI Migration Validation
**Status**: ⚠️ **Integration Tests Need Field Name Updates**

---

## 🎯 **Executive Summary**

**Good News**: The OpenAPI migration is working correctly. The handler properly rejects invalid requests.

**Issue Found**: Integration tests are using legacy field names (pre-ADR-034) that are no longer supported after the OpenAPI migration removed backward compatibility logic.

**Action Needed**: Update 19 integration tests to use correct ADR-034 field names.

---

## 📊 **Test Tier Results**

### **TIER 1: Unit Tests** ✅ **PASS**
- **Result**: 16/16 tests passing (100%)
- **Package**: `pkg/datastorage/scoring`
- **Status**: ✅ No regressions detected
- **Duration**: < 1 second (cached)

### **TIER 2: Integration Tests** ⚠️ **PARTIAL PASS**
- **Result**: 127/146 tests passing (87%)
- **Failed**: 19/146 tests (13%)
- **Skipped**: 3/146 tests
- **Duration**: 254.5 seconds
- **Status**: ⚠️ Tests need field name updates (not a regression)

### **TIER 3: E2E Tests** ⏸️ **NOT RUN YET**
- **Status**: Pending
- **Reason**: Waiting for integration test fix decision

---

## 🔍 **Root Cause Analysis**

### **The Issue**

Integration tests are using **legacy field names** from before ADR-034:

| Legacy Field (Tests Use) | ADR-034 Field (Required) | OpenAPI Spec |
|--------------------------|--------------------------|--------------|
| `"outcome"` | `"event_outcome"` | ✅ Required |
| `"operation"` | `"event_action"` | ✅ Required |
| `"service"` | `"event_category"` | ✅ Required |

### **Error Example**

```json
{
  "type": "https://api.kubernaut.io/problems/validation_error",
  "title": "Validation Error",
  "status": 400,
  "detail": "event_outcome must be one of: success, failure, pending (got: )"
}
```

**What Happened**:
1. Test sends `"outcome": "success"` (legacy field)
2. OpenAPI unmarshaling looks for `"event_outcome"` (ADR-034 field)
3. Field is missing → defaults to empty string
4. Validation fails: empty string is not a valid enum value

---

## 📋 **Failed Test Breakdown**

### **Category 1: Audit Write API Tests** (8 failures)
1. `when Gateway service writes a signal received event`
2. `when AI Analysis service writes an analysis completed event`
3. `when Workflow service writes a workflow completed event`
4. `when request is missing required field event_type`
5. `when request body is missing required 'version' field`
6. `when multiple events are written with same correlation_id`
7. `when event references non-existent parent_event_id`
8. `when batch contains one invalid event` (batch API)

**File**: `test/integration/datastorage/audit_events_write_api_test.go`

### **Category 2: Audit Query API Tests** (6 failures)
1. `Query by correlation_id`
2. `Query by event_type`
3. `Query by service`
4. `Query by time range (relative)`
5. `Query by time range (absolute)`
6. `Query with multiple filters` & `Pagination`

**File**: `test/integration/datastorage/audit_events_query_api_test.go`

### **Category 3: Self-Auditing Tests** (3 failures)
1. `should generate audit traces for successful writes`
2. `should not block business operations if audit fails`
3. `should use InternalAuditClient (not REST API)`

**File**: `test/integration/datastorage/audit_self_auditing_test.go`

### **Category 4: Metrics Tests** (1 failure)
1. `should emit audit_traces_total metric on successful write`

**File**: `test/integration/datastorage/metrics_integration_test.go`

### **Category 5: Batch API Tests** (1 failure)
1. `should reject entire batch with 400 Bad Request (atomic)`

**File**: `test/integration/datastorage/audit_events_batch_write_api_test.go`

---

## 🎯 **Why This is NOT a Regression**

### **1. OpenAPI Migration Works Correctly** ✅
- Handler correctly rejects requests with missing required fields
- Type validation working as designed
- RFC 7807 error responses correct
- All business logic preserved

### **2. Backward Compatibility Was Intentionally Removed** ✅
- User confirmed: "no need for backwards support. We haven't released"
- OpenAPI migration removed legacy field name support
- This was the correct decision

### **3. Tests Reveal Pre-Existing Technical Debt** ✅
- Tests were written before ADR-034 was fully enforced
- Tests relied on backward compatibility logic
- Now that backward compatibility is removed, tests need updating

---

## 🔧 **Fix Required**

### **Option A: Update All Integration Tests** (Recommended)
**Time**: ~2 hours
**Effort**: Systematic field name replacement across 5 test files

**Changes Needed**:
```go
// BEFORE (legacy)
eventPayload := map[string]interface{}{
    "service":   "gateway",
    "outcome":   "success",
    "operation": "signal_received",
}

// AFTER (ADR-034)
eventPayload := map[string]interface{}{
    "event_category": "gateway",
    "event_outcome":  "success",
    "event_action":   "signal_received",
}
```

**Files to Update**:
1. `test/integration/datastorage/audit_events_write_api_test.go` (8 tests)
2. `test/integration/datastorage/audit_events_query_api_test.go` (6 tests)
3. `test/integration/datastorage/audit_self_auditing_test.go` (3 tests)
4. `test/integration/datastorage/metrics_integration_test.go` (1 test)
5. `test/integration/datastorage/audit_events_batch_write_api_test.go` (1 test)

---

### **Option B: Use OpenAPI Client in Tests** (Better Long-Term)
**Time**: ~4 hours
**Effort**: Refactor tests to use `dsclient.AuditEventRequest` type

**Advantages**:
- ✅ Type-safe test code
- ✅ Compiler catches field name errors
- ✅ Aligned with OpenAPI spec
- ✅ Easier to maintain

**Disadvantages**:
- ⏱️ More time-consuming
- 🔄 Requires more test refactoring

---

### **Option C: Run E2E Tests First, Then Decide**
**Time**: ~30 minutes to run E2E tests
**Rationale**: Check if E2E tests have the same issue

**Recommendation**: Do this first to assess full scope

---

## 📊 **Impact Assessment**

### **Current State**
- **Unit Tests**: ✅ 100% passing
- **Integration Tests**: ⚠️ 87% passing (19 failures)
- **E2E Tests**: ⏸️ Not run yet
- **Production Code**: ✅ Works correctly

### **After Fix (Option A)**
- **Unit Tests**: ✅ 100% passing
- **Integration Tests**: ✅ 100% passing (estimated)
- **E2E Tests**: ⏸️ May need similar fixes
- **Production Code**: ✅ No changes needed

### **After Fix (Option B)**
- **Unit Tests**: ✅ 100% passing
- **Integration Tests**: ✅ 100% passing + type-safe
- **E2E Tests**: ⏸️ May need similar fixes
- **Production Code**: ✅ No changes needed

---

## 🎯 **Recommendation**

### **Phase 1: Assessment** (30 minutes)
1. Run E2E tests to see full scope of issue
2. Document E2E test results
3. Assess total test files needing updates

### **Phase 2: Fix** (2-4 hours depending on option)
**Option A** (Recommended for now):
- Quick fix: Update field names in test payloads
- Systematic replacement across all failing tests
- Re-run integration tests to verify

**Option B** (Future enhancement):
- Defer to V1.1: Refactor tests to use OpenAPI client
- Better long-term maintainability
- Can be done incrementally

---

## 📚 **Related Documentation**

1. **ADR-034**: Unified Audit Table Design (canonical field names)
2. **OpenAPI Spec**: `api/openapi/data-storage-v1.yaml` (authoritative)
3. **Migration Complete**: `DS_OPENAPI_MIGRATION_COMPLETE_2025-12-13.md`
4. **Testing Strategy**: `03-testing-strategy.mdc` (integration coverage >50%)

---

## ✅ **Success Criteria**

This issue is resolved when:
- ✅ All integration tests pass (146/146)
- ✅ All E2E tests pass
- ✅ Tests use correct ADR-034 field names
- ✅ No backward compatibility references remain

---

## 🚀 **Next Steps**

**Immediate**:
1. Run E2E tests to assess full scope
2. Choose fix approach (Option A or B)
3. Execute fix systematically
4. Re-run all 3 test tiers to verify

**Documentation**:
1. Update test files with correct field names
2. Add comments referencing ADR-034
3. Update any test documentation

---

**Document Version**: 1.0
**Last Updated**: 2025-12-13
**Status**: ⚠️ **Action Required** - Test field name updates needed

