# Test Update Plan for Embedding Removal

**Date**: 2025-12-11
**Status**: 🔄 **IN PROGRESS** - Build complete, test updates needed

---

## ✅ **Completed**

### **Build Success** ✅
- ✅ Data Storage service builds successfully
- ✅ All compilation errors fixed
- ✅ Label-only search implementation complete

---

## ⏳ **Test Files Requiring Updates**

### **Unit Tests** (Current Failures: 11 compilation errors)

**File**: `test/unit/datastorage/workflow_search_audit_test.go`

**Errors**:
- 7x `unknown field Query` in `WorkflowSearchRequest`
- 3x `unknown field BaseSimilarity` in `WorkflowSearchResult`
- 2x `unknown field Query` in `WorkflowSearchResponse`
- 1x `eventData.Query.Text undefined`

**Changes Needed**:
1. Replace `Query: "..."` with `Filters: &models.WorkflowSearchFilters{...}`
2. Remove `BaseSimilarity: ...` fields
3. Remove `Query: ...` fields from response
4. Update assertions from `Query.Text` to `Filters.SignalType` (or similar)

**Estimated Effort**: 30-45 minutes to update all test cases

---

### **Integration Tests** (Unknown - need to run)

**Files** (potential):
- `test/integration/datastorage/workflow_semantic_search_test.go`
- `test/integration/datastorage/hybrid_scoring_test.go`

**Expected Changes**:
- Update search requests to use filters
- Update assertions to check label-only scoring
- Remove embedding fixture references

---

### **E2E Tests** (Unknown - need to run)

**Files** (potential):
- `test/e2e/datastorage/*.go`

**Expected Changes**:
- Update end-to-end scenarios to use label-only search
- Remove embedding service dependencies

---

## 🎯 **Recommendation**

### **Option A: Quick Fix (Continue Testing)**
- **Action**: Comment out failing unit tests temporarily
- **Pros**: Can proceed to integration/e2e tests immediately
- **Cons**: Incomplete test coverage
- **Time**: 5 minutes

### **Option B: Complete Fix (Thorough)**
- **Action**: Update all audit tests for V1.0 label-only search
- **Pros**: Complete test coverage, validates audit system
- **Cons**: Takes 30-45 minutes
- **Time**: 30-45 minutes

---

## 📝 **Next Steps**

**If Option A** (Quick Fix):
1. Skip audit unit tests (comment out)
2. Run integration tests
3. Run e2e tests
4. Come back to audit tests later

**If Option B** (Complete Fix):
1. Update workflow_search_audit_test.go
2. Run unit tests until passing
3. Run integration tests
4. Run e2e tests

---

**Recommendation**: **Option B** (Complete Fix) - better to have full test coverage validating the audit system works correctly with label-only search.
