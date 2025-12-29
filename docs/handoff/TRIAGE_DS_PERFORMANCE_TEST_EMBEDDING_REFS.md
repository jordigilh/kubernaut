# TRIAGE: Data Storage Performance Test - Embedding References

**Date**: 2025-12-11
**File**: `test/performance/datastorage/workflow_search_perf_test.go`
**Severity**: HIGH (Blocks performance testing)
**Type**: Test Code Out-of-Sync with V1.0 Implementation
**Decision**: **DELETE** (Obsolete functionality)

---

## 🚨 **ISSUE SUMMARY**

Performance test file tests **V1.5 hybrid scoring** (embeddings + labels) which was **removed** in favor of V1.0 label-only scoring.

---

## 📊 **ANALYSIS**

### **File Purpose**
Tests performance of workflow search with semantic search (embeddings) and hybrid weighted scoring.

### **Business Requirements Tested**
- BR-STORAGE-013: Semantic search with hybrid weighted scoring must be performant
- Performance targets: P50 <100ms, P95 <200ms, P99 <500ms, 10 QPS

### **Technology Stack**
- pgvector (line 27) - Vector database extension
- Embedding generation (line 413-427)
- `SearchByEmbedding` method (lines 109, 151, 185, 220, 272)

---

## 🔍 **EMBEDDING REFERENCES DETECTED**

### **Count: 20+ references**

| Line | Type | Reference |
|------|------|-----------|
| 27 | Import | `github.com/pgvector/pgvector-go` |
| 38 | Comment | "Semantic search with hybrid weighted scoring" |
| 84 | Code | `embedding := generateTestEmbedding(...)` |
| 84 | Code | `embeddingVec := pgvector.NewVector(embedding)` |
| 100 | Field | `Embedding: &embeddingVec` |
| 109 | Method | `workflowRepo.SearchByEmbedding(...)` |
| 128 | Code | `embedding := generateTestEmbedding(...)` |
| 128 | Code | `embeddingVec := pgvector.NewVector(embedding)` |
| 142 | Field | `Embedding: &embeddingVec` |
| 151 | Method | `workflowRepo.SearchByEmbedding(...)` |
| 169 | Code | `embedding := generateTestEmbedding(...)` |
| 169 | Code | `embeddingVec := pgvector.NewVector(embedding)` |
| 176 | Field | `Embedding: &embeddingVec` |
| 185 | Method | `workflowRepo.SearchByEmbedding(...)` |
| 203 | Code | `embedding := generateTestEmbedding(...)` |
| 203 | Code | `embeddingVec := pgvector.NewVector(embedding)` |
| 211 | Field | `Embedding: &embeddingVec` |
| 220 | Method | `workflowRepo.SearchByEmbedding(...)` |
| 247 | Code | `embedding := generateTestEmbedding(...)` |
| 247 | Code | `embeddingVec := pgvector.NewVector(embedding)` |
| 264 | Field | `Embedding: &embeddingVec` |
| 272 | Method | `workflowRepo.SearchByEmbedding(...)` |
| 350 | Code | `embedding := generateTestEmbedding(...)` |
| 350 | Code | `embeddingVec := pgvector.NewVector(embedding)` |
| 392 | Field | `Embedding: &embeddingVec` |
| 413-427 | Function | `generateTestEmbedding()` - Full implementation |

---

## ⚖️ **DELETE vs FIX DECISION MATRIX**

### **Option A: DELETE** ✅ **RECOMMENDED**

#### **Justification**
1. **Tests Obsolete Functionality**
   - Tests V1.5 hybrid scoring (embeddings + labels)
   - V1.0 uses label-only scoring (no embeddings)
   - Business requirement BR-STORAGE-013 targets semantic search, which was removed

2. **Extensive Rewrites Required**
   - 20+ embedding references across entire file
   - All 5 test cases depend on `SearchByEmbedding`
   - Helper functions (seedWorkflows, generateTestEmbedding) all embedding-based
   - Would require 100% rewrite

3. **No Quick Fix Path**
   - Cannot simply comment out embedding code
   - Core test logic depends on semantic similarity scoring
   - Performance characteristics changed (SQL label matching vs vector similarity)

4. **Business Value Unclear**
   - V1.0 deliberately removed embeddings to **increase** correctness (81% → 95%)
   - Performance targets (P50 <100ms) may not apply to label-only SQL queries
   - Would need new performance targets for label-only scoring

#### **Impact of Deletion**
- ✅ Eliminates obsolete code
- ✅ Prevents confusion about what's tested
- ✅ Clear signal that performance testing needs redesign for V1.0
- ⚠️ Lose historical performance benchmarks (acceptable, architecture changed)

---

### **Option B: FIX/REWRITE** ❌ **NOT RECOMMENDED**

#### **What Would Be Required**
1. **Complete Rewrite** (90%+ of file):
   ```go
   // BEFORE (V1.5 - Embedding-based):
   searchReq := &models.WorkflowSearchRequest{
       Query:     "OOMKilled critical",
       Embedding: &embeddingVec,  // REMOVE
       Filters:   filters,
       TopK:      10,
   }
   result, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)  // CHANGE

   // AFTER (V1.0 - Label-only):
   searchReq := &models.WorkflowSearchRequest{
       Filters:  filters,  // Required, not optional
       MinScore: 0.5,       // New field
       TopK:     10,
   }
   result, err := workflowRepo.SearchByLabels(testCtx, searchReq)  // New method
   ```

2. **New Performance Targets**:
   - Label-only SQL queries have different performance profile
   - Need to establish NEW baselines for:
     - SQL CASE statement performance
     - Wildcard matching overhead
     - Label boost/penalty calculation
   - Cannot reuse embedding-based targets

3. **New Test Scenarios**:
   - Test wildcard matching performance (exact vs wildcard)
   - Test DetectedLabels boost/penalty calculation
   - Test mandatory label filtering performance
   - All scenarios would be NEW (not adaptations)

#### **Effort Estimate**
- **Time**: 4-6 hours (complete rewrite)
- **Complexity**: HIGH (new test design required)
- **Value**: MEDIUM (performance testing valuable, but V1.0 not performance-critical yet)

---

## 🎯 **RECOMMENDATION: DELETE**

### **Why DELETE is Correct Decision**

#### **1. Tests Removed Functionality**
The file tests semantic search with embeddings, which was **intentionally removed** because:
- Indeterministic LLM keywords **decreased** correctness (81% → 95% after removal)
- Authority: `CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md` (92% confidence)
- Design Decision: `DD-WORKFLOW-004` v1.5 (Label-Only Scoring)

#### **2. Complete Rewrite Required**
- 20+ embedding references throughout
- Core test logic depends on vector similarity
- No salvageable code (helper functions, test setup, all embedding-based)

#### **3. Business Value Unclear**
- V1.0 prioritizes **correctness** over performance
- Performance testing valuable, but needs NEW design for label-only scoring
- Current targets (P50 <100ms) may not apply to SQL label queries

#### **4. Clean Slate Better Than Patches**
- Deleting signals "needs redesign" clearly
- Fixing creates false impression tests are current
- V1.1+ can add NEW performance tests designed for label-only architecture

---

## ✅ **DECISION: DELETE**

### **Confidence: 95%**

**High Confidence Because**:
1. ✅ Tests obsolete functionality (V1.5 hybrid scoring removed)
2. ✅ Extensive rewrites required (20+ embedding references)
3. ✅ No quick fix path (core logic depends on embeddings)
4. ✅ Business value unclear (V1.0 prioritizes correctness, not performance)
5. ✅ Clean slate better than patches (signals redesign needed)

**5% Risk**:
- ⚠️ Lose historical performance benchmarks (acceptable, architecture changed)

---

## 📝 **IMPLEMENTATION**

### **Action**
```bash
rm test/performance/datastorage/workflow_search_perf_test.go
```

### **Rationale**
- File tests V1.5 hybrid scoring (embeddings + labels)
- V1.0 uses label-only scoring (no embeddings)
- Complete rewrite required (not a fix)
- Performance testing valuable, but needs NEW design for V1.0

### **Follow-up** (V1.1+)
Create NEW performance tests for label-only scoring:
- Test SQL CASE statement performance
- Test wildcard matching overhead
- Test DetectedLabels boost/penalty calculation
- Establish NEW performance baselines for V1.0 architecture

---

## 📊 **IMPACT ASSESSMENT**

### **Test Coverage Impact**
| Test Type | Before | After | Status |
|-----------|--------|-------|--------|
| Performance Tests | 1 file (5 tests) | 0 files | ⚠️ Gap identified |
| Unit Tests | ✅ Passing | ✅ Passing | No impact |
| Integration Tests | ✅ Running | ✅ Running | No impact |

**Coverage Gap**: Performance testing needs NEW implementation for V1.0 label-only scoring.

### **Business Impact**
- ⚠️ No performance validation for V1.0 label-only scoring
- ✅ No impact on correctness (unit/integration tests validate)
- ✅ No impact on production (V1.0 not performance-critical yet)

### **Risk Assessment**
| Risk | Severity | Mitigation |
|------|----------|------------|
| Unknown V1.0 performance | LOW | V1.0 prioritizes correctness; performance testing deferred to V1.1+ |
| No performance benchmarks | LOW | Can establish NEW baselines in V1.1+ |
| Regression detection | LOW | Integration tests validate functionality |

---

## 🚀 **NEXT STEPS**

### **Immediate (P0)**
1. ✅ **DELETE**: `test/performance/datastorage/workflow_search_perf_test.go`
2. 📝 **Document**: Performance testing gap in V1.0

### **Follow-up (V1.1+ / P2)**
3. 🔜 **Design NEW Performance Tests** for label-only scoring:
   - Establish performance targets for SQL label matching
   - Test wildcard matching performance
   - Test DetectedLabels boost/penalty overhead
   - Validate scaling (1K, 5K, 10K workflows)

---

## ✅ **ACCEPTANCE CRITERIA**

- [ ] File deleted
- [ ] No compilation errors after deletion
- [ ] Performance testing gap documented
- [ ] Follow-up task created for V1.1+ performance tests

---

## 🔗 **RELATED DOCUMENTATION**

- **Embedding Removal**: `API_IMPACT_REMOVE_EMBEDDINGS.md`
- **Design Decision**: `DD-WORKFLOW-004-hybrid-weighted-label-scoring.md` (V1.5 - Label-Only)
- **Confidence Assessment**: `CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md` (92% confidence to remove)
- **Status Summary**: `STATUS_DS_EMBEDDING_REMOVAL_COMPLETE.md`

---

## 📊 **CONFIDENCE ASSESSMENT: 95%**

**DELETE is the correct decision because**:
- ✅ Tests obsolete V1.5 functionality
- ✅ Complete rewrite required (not a fix)
- ✅ Business value unclear for V1.0
- ✅ Clean slate signals need for redesign

**Recommendation**: **DELETE FILE** and create follow-up task for V1.1+ NEW performance tests designed for label-only architecture.

---

**Triaged By**: AI Assistant (Claude)
**Decision**: DELETE
**Confidence**: 95%
**Date**: 2025-12-11
