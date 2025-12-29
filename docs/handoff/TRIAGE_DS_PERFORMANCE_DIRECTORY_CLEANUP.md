# TRIAGE: Data Storage Performance Directory - Remaining Files

**Date**: 2025-12-11
**Directory**: `test/performance/datastorage/`
**Context**: After deleting `workflow_search_perf_test.go`, assess remaining files
**Decision**: **UPDATE** (Not DELETE)

---

## 📊 **DIRECTORY CONTENTS**

| File | Purpose | Embedding Refs | Decision |
|------|---------|---------------|----------|
| `workflow_search_perf_test.go` | Tests V1.5 hybrid scoring | 20+ | ✅ **DELETED** |
| `benchmark_test.go` | Tests ListIncidents API | 0 | ✅ **KEEP** |
| `suite_test.go` | Test suite setup | 4 (minor) | ⚠️ **UPDATE** |
| `README.md` | Documentation | Multiple | ⚠️ **UPDATE** |

---

## 🔍 **FILE-BY-FILE ANALYSIS**

### **1. `benchmark_test.go`** ✅ **KEEP AS-IS**

#### **Purpose**
Tests ListIncidents API performance (not workflow search):
- `BenchmarkListIncidentsLatency` - P50/P95/P99 latency
- `BenchmarkLargeResultSet` - 1000 record queries
- `BenchmarkConcurrentRequests` - 10 concurrent workers
- `BenchmarkFilteredQueries` - Various filter combinations

#### **Embedding References**: **ZERO** ✅
```bash
# Verified:
grep -i "embedding\|pgvector\|SearchBy" benchmark_test.go
# Result: No matches
```

#### **Business Value**: **HIGH**
- Tests `/api/v1/incidents` endpoint (used in production)
- Validates BR-STORAGE-027 performance requirements
- No dependencies on workflow search or embeddings
- Provides performance regression detection

#### **Decision**: ✅ **KEEP AS-IS** (No changes needed)

**Confidence**: 100%

---

### **2. `suite_test.go`** ⚠️ **UPDATE**

#### **Purpose**
Performance test suite setup (BeforeSuite/AfterSuite).

#### **Embedding References**: **4 (minor)**
- Line 41: Comment "Reuses integration test Podman PostgreSQL (with pgvector)"
- Lines 110-115: Checks for pgvector extension availability
- Line 118: Comment about mock embedding client
- Lines 119-120: Initializes `workflowRepo` with mock embedding client

#### **Current State**
```go
// Line 119-120:
mockEmbeddingClient := testutil.NewMockEmbeddingClient()
workflowRepo = repository.NewWorkflowRepository(db, logger, mockEmbeddingClient)
```

#### **Issue**
- `workflowRepo` is only used by `workflow_search_perf_test.go` (now deleted)
- `benchmark_test.go` does NOT use `workflowRepo` (uses HTTP API directly)
- Mock embedding client is unnecessary (no tests use it)

#### **Required Changes**
1. **Remove variable declaration**:
   ```go
   // DELETE:
   workflowRepo *repository.WorkflowRepository
   ```

2. **Remove repository initialization**:
   ```go
   // DELETE lines 118-120:
   // Initialize repository with mock embedding client (not needed for query performance tests)
   mockEmbeddingClient := testutil.NewMockEmbeddingClient()
   workflowRepo = repository.NewWorkflowRepository(db, logger, mockEmbeddingClient)
   ```

3. **Update pgvector check** (OPTIONAL - can keep for future):
   ```go
   // Lines 110-115: KEEP (pgvector still in DB, just not used for search)
   // Verify pgvector extension is available
   var hasExtension bool
   err = db.Get(&hasExtension, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')")
   ```

4. **Update comment** (line 41):
   ```go
   // BEFORE:
   // - Reuses integration test Podman PostgreSQL (with pgvector)

   // AFTER:
   // - Reuses integration test Podman PostgreSQL
   ```

#### **Decision**: ⚠️ **UPDATE** (Remove unused workflowRepo)

**Confidence**: 100%

**Effort**: 5 minutes

---

### **3. `README.md`** ⚠️ **UPDATE**

#### **Purpose**
Documentation for performance testing approach and targets.

#### **Embedding References**: **Multiple**
- Line 5: "hybrid weighted scoring SQL query performance"
- Lines 19-26: Performance targets for semantic search
- Lines 197-224: SQL query examples with embedding vectors
- Line 253: Reference to "SEMANTIC_SEARCH_HYBRID_SCORING_IMPLEMENTATION.md"
- Line 255: Reference to "DD-WORKFLOW-004-hybrid-weighted-label-scoring.md"
- Line 256: "BR-STORAGE-013 (Semantic search with hybrid weighted scoring)"

#### **Current State**
Documentation describes V1.5 hybrid scoring (embeddings + labels) which was removed.

#### **Required Changes**
1. **Update Overview** (line 5):
   ```markdown
   <!-- BEFORE -->
   Performance tests for the Data Storage Service, specifically validating the hybrid weighted scoring SQL query performance at realistic scale.

   <!-- AFTER -->
   Performance tests for the Data Storage Service API endpoints (ListIncidents).

   Note: Workflow search performance tests removed (V1.0 uses label-only scoring, no embeddings).
   V1.1+ will add NEW performance tests designed for label-only architecture.
   ```

2. **Remove/Update Sections**:
   - Remove "Hybrid Weighted Scoring" references
   - Remove embedding-related SQL examples (lines 197-224)
   - Update business requirement reference (line 256)

3. **Add V1.0 Context**:
   ```markdown
   ## V1.0 Performance Testing Scope

   Current performance tests focus on:
   - ListIncidents API latency (P50/P95/P99)
   - Large result set performance (1000 records)
   - Concurrent request handling (10 QPS)

   ## V1.0 Workflow Search

   Workflow search performance testing deferred to V1.1+ because:
   - V1.0 uses label-only scoring (no embeddings)
   - SQL CASE statements for wildcard weighting (different performance profile)
   - Need to establish NEW performance baselines for label-only architecture
   ```

#### **Decision**: ⚠️ **UPDATE** (Rewrite to reflect V1.0 scope)

**Confidence**: 95%

**Effort**: 15 minutes

---

## 🎯 **OVERALL RECOMMENDATION: UPDATE FILES (NOT DELETE)**

### **Why UPDATE, Not DELETE Directory**

#### **1. `benchmark_test.go` is Valuable** ✅
- Tests ListIncidents API (production endpoint)
- No embedding dependencies
- Provides performance regression detection
- Business value: HIGH

#### **2. Infrastructure is Reusable**
- `suite_test.go` provides test setup for future V1.0 performance tests
- Connection to PostgreSQL useful for NEW label-only tests
- Just needs cleanup (remove unused workflowRepo)

#### **3. Directory Structure Valid**
- Standard Go testing convention: `test/performance/{service}/`
- Follows project structure (test/unit, test/integration, test/performance)
- Keep structure for future V1.0 performance tests

---

## 📋 **IMPLEMENTATION PLAN**

### **Phase 1: Update suite_test.go** (5 minutes)
1. Remove `workflowRepo` variable declaration
2. Remove `mockEmbeddingClient` initialization
3. Remove repository instantiation
4. Update pgvector comment (optional)

### **Phase 2: Update README.md** (15 minutes)
1. Update overview to reflect V1.0 scope
2. Remove hybrid scoring references
3. Remove embedding SQL examples
4. Add V1.0 context section
5. Update business requirement references

### **Total Effort**: 20 minutes

---

## ✅ **DECISION MATRIX SUMMARY**

| File | Embedding Refs | Business Value | Decision | Confidence |
|------|---------------|----------------|----------|------------|
| `workflow_search_perf_test.go` | 20+ | NONE (obsolete) | ✅ DELETED | 95% |
| `benchmark_test.go` | 0 | HIGH (ListIncidents) | ✅ KEEP | 100% |
| `suite_test.go` | 4 (minor) | MEDIUM (infrastructure) | ⚠️ UPDATE | 100% |
| `README.md` | Multiple | MEDIUM (docs) | ⚠️ UPDATE | 95% |

**Overall Decision**: **UPDATE DIRECTORY** (not delete)

---

## 📊 **CONFIDENCE ASSESSMENT: 98%**

**High Confidence Because**:
1. ✅ `benchmark_test.go` has zero embedding dependencies (keep as-is)
2. ✅ `suite_test.go` only needs minor cleanup (remove unused workflowRepo)
3. ✅ `README.md` needs updates, but directory structure is valid
4. ✅ Infrastructure reusable for future V1.0 performance tests

**2% Risk**:
- ⚠️ Minor documentation updates needed

---

**Triaged By**: AI Assistant (Claude)
**Decision**: UPDATE (not DELETE)
**Confidence**: 98%
**Date**: 2025-12-11
