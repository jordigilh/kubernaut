# TRIAGE: Data Storage E2E Tests - Embedding References

**Date**: 2025-12-11
**Service**: Data Storage
**Type**: E2E Test Cleanup
**Status**: ⚠️ **REQUIRES DECISION**

---

## 🎯 **DISCOVERY**

**E2E Test Results**: 2/7 passed, **5 failed** (all with HTTP 500 "Failed to create workflow")

**Root Cause**: E2E tests reference obsolete embedding functionality removed in V1.0 label-only architecture.

---

## 📊 **EMBEDDING REFERENCE COUNT**

| File | Embedding Refs | Status | Recommendation |
|------|----------------|--------|----------------|
| `05_embedding_service_integration_test.go` | **76** | ❌ **OBSOLETE** | **DELETE** |
| `04_workflow_search_test.go` | **18** | ⚠️ **PARTIAL** | **UPDATE** |
| `07_workflow_version_management_test.go` | **0** | ✅ **CLEAN** | **FIX** (schema issue) |
| `06_workflow_search_audit_test.go` | **0** | ✅ **CLEAN** | **FIX** (schema issue) |
| `03_query_api_timeline_test.go` | **0** | ✅ **CLEAN** | **FIX** (schema issue) |
| `01_happy_path_test.go` | **0** | ✅ **PASSING** | **KEEP** |
| `02_dlq_fallback_test.go` | **0** | ✅ **PASSING** | **KEEP** |
| `datastorage_e2e_suite_test.go` | **1** | ⚠️ **COMMENT** | **UPDATE** |

---

## 🔍 **DETAILED ANALYSIS**

### **File 1: `05_embedding_service_integration_test.go`** ❌ **DELETE**

**Embedding References**: 76 (100% of test purpose)

**Test Purpose**: "Complete Journey - Embedding Service Integration"
- Tests automatic embedding generation
- Tests embedding-based search
- Tests embedding updates
- Tests embedding persistence

**Sample Code**:
```go
// Line 205: Tests embedding generation
// NOTE: No "embedding" field provided - should be generated automatically

// Line 232: Verifies embedding was generated
embedding := result["embedding"]
Expect(embedding).ToNot(BeNil(), "Embedding should be auto-generated")
```

**Status**: ❌ **100% OBSOLETE** (entire file tests removed functionality)

**Recommendation**: **DELETE** (same as integration test `workflow_semantic_search_test.go`)

**Confidence**: 100%

---

### **File 2: `04_workflow_search_test.go`** ⚠️ **UPDATE**

**Embedding References**: 18

**Test Purpose**: "Workflow Search with Hybrid Weighted Scoring"

**What's Obsolete**:
- Hybrid scoring **with embeddings** (V1.0 removed embeddings)
- Semantic similarity scoring
- Embedding-based search

**What's Still Relevant**:
- Label-based matching ✅
- DetectedLabels wildcard weighting ✅
- CustomLabels wildcard weighting ✅ (just added)
- Score ranking ✅

**Test Structure** (lines 140-302):
```go
// Step 1: Seed workflows with various labels
// Step 2: Search workflows by labels
// Step 3: Verify ranking based on hybrid scoring
```

**Problem**: Line 205 says "NOTE: No 'embedding' field provided - should be generated automatically"

**Recommendation**: **UPDATE** to test label-only scoring:
1. Remove embedding references
2. Update to V1.0 label-only scoring expectations
3. Test DetectedLabels + CustomLabels wildcard weighting
4. Keep label matching and ranking logic

**Effort**: ~30 minutes

**Confidence**: 85%

---

### **File 3: `07_workflow_version_management_test.go`** ✅ **FIX**

**Embedding References**: 0

**Test Purpose**: "Workflow Version Management (DD-WORKFLOW-002 v3.0)"
- Tests workflow v1.0.0 creation with UUID
- Tests version is_latest_version logic

**Problem**: HTTP 500 when creating workflow

**Root Cause Analysis**:
- Line 164-201: Uses old label schema (7 labels: `risk_tolerance`, `business_category`)
- DD-WORKFLOW-001 v1.4+: Only 5 mandatory labels (removed `risk_tolerance`, `business_category`)
- These should be in `custom_labels`, not `labels`

**Recommendation**: **UPDATE** label schema from 7 → 5 mandatory + 2 custom

**Effort**: ~10 minutes

**Confidence**: 95%

---

### **File 4: `06_workflow_search_audit_test.go`** ✅ **FIX**

**Embedding References**: 0

**Test Purpose**: "Workflow Search Audit Trail"
- Tests audit event generation during search
- Tests remediation_id correlation

**Problem**: HTTP 500 when creating workflow (cascade from workflow creation failure)

**Root Cause**: Same as File 3 (old label schema)

**Recommendation**: **UPDATE** label schema from 7 → 5 mandatory + 2 custom

**Effort**: ~10 minutes

**Confidence**: 95%

---

### **File 5: `03_query_api_timeline_test.go`** ✅ **FIX**

**Embedding References**: 0

**Test Purpose**: "Query API Timeline - Multi-Filter Retrieval"
- Tests multi-dimensional filtering
- Tests pagination

**Problem**: HTTP 500 when creating workflow (cascade from workflow creation failure)

**Root Cause**: Same as File 3 (old label schema)

**Recommendation**: **UPDATE** label schema from 7 → 5 mandatory + 2 custom

**Effort**: ~10 minutes

**Confidence**: 95%

---

### **File 6: `01_happy_path_test.go`** ✅ **PASSING**

**Status**: ✅ **PASSED** (no issues)

**Recommendation**: **KEEP AS-IS**

---

### **File 7: `02_dlq_fallback_test.go`** ✅ **PASSING**

**Status**: ✅ **PASSED** (no issues)

**Recommendation**: **KEEP AS-IS**

---

### **File 8: `datastorage_e2e_suite_test.go`** ⚠️ **UPDATE**

**Embedding References**: 1 (comment only)

**Location**: Line 38 - "PostgreSQL with pgvector (for audit events storage)"

**Recommendation**: Update comment to "PostgreSQL 16 (V1.0 label-only)"

**Effort**: ~1 minute

**Confidence**: 100%

---

## 📋 **ROOT CAUSE: Two Issues**

### **Issue 1**: Obsolete Embedding Tests (2 files)
- `05_embedding_service_integration_test.go` (76 refs)
- `04_workflow_search_test.go` (18 refs, partially obsolete)

### **Issue 2**: Old Label Schema (3 files)
- `07_workflow_version_management_test.go`
- `06_workflow_search_audit_test.go`
- `03_query_api_timeline_test.go`

All use 7 mandatory labels (obsolete per DD-WORKFLOW-001 v1.4+)

---

## 🎯 **RECOMMENDED ACTIONS**

### **OPTION A: Comprehensive E2E Cleanup** ✅ **RECOMMENDED**

**Delete** (1 file):
1. `05_embedding_service_integration_test.go` (100% obsolete)

**Update** (4 files):
1. `04_workflow_search_test.go` - Remove embedding refs, test label-only scoring
2. `07_workflow_version_management_test.go` - Fix label schema (7 → 5)
3. `06_workflow_search_audit_test.go` - Fix label schema (7 → 5)
4. `03_query_api_timeline_test.go` - Fix label schema (7 → 5)

**Minor Fix** (1 file):
5. `datastorage_e2e_suite_test.go` - Update pgvector comment

**Effort**: ~60 minutes total

**Result**: All E2E tests should pass

---

### **OPTION B: Delete All Embedding-Related E2E Tests** (Faster)

**Delete** (2 files):
1. `05_embedding_service_integration_test.go` (100% obsolete)
2. `04_workflow_search_test.go` (hybrid scoring with embeddings)

**Update** (3 files):
1. `07_workflow_version_management_test.go` - Fix label schema
2. `06_workflow_search_audit_test.go` - Fix label schema
3. `03_query_api_timeline_test.go` - Fix label schema

**Effort**: ~30 minutes

**Result**: 5/7 E2E tests should pass (75% coverage)

**Trade-off**: Lose E2E coverage for label-only scoring

---

### **OPTION C: Skip E2E Tests** ❌ **NOT RECOMMENDED**

**Rationale**: Integration tests (123/135) already validate label-only functionality. E2E tests validate end-to-end flow in Kind cluster.

---

## 📊 **DECISION MATRIX**

| Criteria | Option A | Option B |
|----------|----------|----------|
| **Test Coverage** | ✅ 100% (7/7) | ⚠️ 71% (5/7) |
| **Effort** | ⚠️ 60 min | ✅ 30 min |
| **V1.0 Value** | ✅ High (tests label-only scoring E2E) | ⚠️ Medium (basic CRUD only) |
| **Risk** | ❌ Low | ⚠️ Medium (no E2E test for scoring) |

---

## 🚨 **QUESTION TO USER**

**Which option do you prefer?**

**Option A**: Update all E2E tests (~60 min, 100% coverage, tests label-only scoring E2E)
**Option B**: Delete embedding tests, fix schema (~30 min, 71% coverage, no scoring E2E test)

**My Recommendation**: **Option A** (comprehensive E2E coverage validates label-only architecture end-to-end)

---

**Completed By**: DataStorage Team (AI Assistant - Claude)
**Date**: 2025-12-11
**Status**: ⏸️ **AWAITING USER DECISION**
**Confidence**: 95%
