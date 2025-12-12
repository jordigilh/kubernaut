# TRIAGE: Data Storage Integration Tests - Embedding References

**Date**: 2025-12-11
**Service**: Data Storage
**Severity**: HIGH (BLOCKING INTEGRATION TESTS)
**Type**: Test Code Out-of-Sync with Implementation
**Reporter**: AI Assistant (during systematic testing validation)

---

## 🚨 **ISSUE SUMMARY**

Integration tests are failing because 6 test files still reference embedding-related code that was removed during the V1.0 label-only scoring implementation.

### **Build Error**
```
test/integration/datastorage/hybrid_scoring_test.go:103:5: unknown field Embedding in struct literal
test/integration/datastorage/hybrid_scoring_test.go:154:34: workflowRepo.SearchByEmbedding undefined
```

### **Root Cause**
During embedding removal implementation, integration tests were not updated to reflect the new label-only API.

---

## 📊 **AFFECTED FILES**

| File | References | Action Required |
|------|-----------|----------------|
| `hybrid_scoring_test.go` | `Embedding`, `SearchByEmbedding`, `pgvector` | **DELETE** - Tests obsolete V1.5 hybrid scoring |
| `workflow_semantic_search_test.go` | `Embedding`, `SearchByEmbedding` | **DELETE** or **REWRITE** for label-only search |
| `workflow_catalog_test.go` | `Embedding` field | **UPDATE** - Remove embedding references |
| `schema_validation_test.go` | `Embedding`, `pgvector` | **UPDATE** - Remove embedding schema tests |
| `suite_test.go` | `embeddingClient` | **UPDATE** - Remove embedding client setup |
| `server_wiring_test.go` | `embeddingService` | **UPDATE** - Remove embedding service wiring |

---

## 🎯 **DECISION MATRIX**

### **Files to DELETE** (Obsolete functionality)
1. **`hybrid_scoring_test.go`** - Tests V1.5 hybrid scoring (embeddings + labels)
   - **Rationale**: V1.0 is pure label-only scoring, no hybrid approach
   - **Business Value**: NONE (tests removed functionality)

2. **`workflow_semantic_search_test.go`** - Tests embedding-based semantic search
   - **Rationale**: V1.0 uses label-only search, no semantic search
   - **Business Value**: NONE (tests removed functionality)

### **Files to UPDATE** (Infrastructure/Setup)
3. **`workflow_catalog_test.go`** - Tests workflow CRUD operations
   - **Issue**: Creates workflows with `Embedding` field
   - **Fix**: Remove `Embedding` field from test workflow literals
   - **Business Value**: HIGH (validates catalog persistence)

4. **`schema_validation_test.go`** - Tests database schema
   - **Issue**: Validates embedding column and pgvector extension
   - **Fix**: Remove embedding column validation (column still exists in DB but unused)
   - **Business Value**: MEDIUM (validates schema integrity)

5. **`suite_test.go`** - Test suite setup/teardown
   - **Issue**: Initializes `embeddingClient`
   - **Fix**: Pass `nil` for embedding client in repository initialization
   - **Business Value**: HIGH (required for all tests)

6. **`server_wiring_test.go`** - Tests service wiring
   - **Issue**: Tests embedding service integration
   - **Fix**: Remove embedding service wiring tests
   - **Business Value**: MEDIUM (validates service initialization)

---

## 🔧 **IMPLEMENTATION PLAN**

### **Phase 1: DELETE Obsolete Test Files (IMMEDIATE)**

#### **Step 1.1: Delete hybrid_scoring_test.go**
```bash
rm test/integration/datastorage/hybrid_scoring_test.go
```
**Justification**: Tests V1.5 hybrid scoring which doesn't exist in V1.0

#### **Step 1.2: Delete workflow_semantic_search_test.go**
```bash
rm test/integration/datastorage/workflow_semantic_search_test.go
```
**Justification**: Tests embedding-based semantic search which was removed

### **Phase 2: UPDATE Infrastructure Files (IMMEDIATE)**

#### **Step 2.1: Update suite_test.go**
Remove embedding client initialization:
```go
// REMOVE:
embeddingClient = embedding.NewClient(...)

// UPDATE repository initialization:
workflowRepo = repository.NewWorkflowRepository(db, logger, nil) // nil for embedding client
```

#### **Step 2.2: Update workflow_catalog_test.go**
Remove `Embedding` field from workflow test data:
```go
// REMOVE embedding field assignments
workflow := &models.RemediationWorkflow{
    // ...
    // Embedding: createTestEmbedding(), // REMOVE THIS LINE
}
```

#### **Step 2.3: Update schema_validation_test.go**
Remove or comment out embedding column validation:
```go
// REMOVE or COMMENT OUT:
// - Tests for embedding column existence
// - Tests for pgvector extension
// - HNSW index tests
```

#### **Step 2.4: Update server_wiring_test.go**
Remove embedding service wiring tests:
```go
// REMOVE tests that validate:
// - Embedding service initialization
// - Embedding client wiring
```

---

## 🧪 **VALIDATION PLAN**

### **After Phase 1 (DELETE)**
```bash
# Verify files are deleted
ls test/integration/datastorage/hybrid_scoring_test.go 2>&1 # Should error
ls test/integration/datastorage/workflow_semantic_search_test.go 2>&1 # Should error
```

### **After Phase 2 (UPDATE)**
```bash
# Verify build succeeds
make test-integration-datastorage
```
**Expected**: Integration tests compile and run

### **Test Coverage Verification**
```bash
# Verify remaining tests provide adequate coverage
ginkgo --dry-run test/integration/datastorage/
```
**Expected**: Workflow catalog, audit, and API tests remain

---

## 📋 **ACCEPTANCE CRITERIA**

- [ ] `hybrid_scoring_test.go` deleted
- [ ] `workflow_semantic_search_test.go` deleted
- [ ] `suite_test.go` passes `nil` for embedding client
- [ ] `workflow_catalog_test.go` has no `Embedding` field references
- [ ] `schema_validation_test.go` has no embedding validation tests
- [ ] `server_wiring_test.go` has no embedding service tests
- [ ] Integration tests compile without errors
- [ ] Integration tests run without failures
- [ ] No `SearchByEmbedding` references in test code

---

## 🎯 **BUSINESS IMPACT**

### **Test Coverage Before**
- ✅ Workflow catalog CRUD
- ✅ Audit event generation
- ✅ Hybrid scoring (embeddings + labels) ← OBSOLETE
- ✅ Semantic search (embeddings) ← OBSOLETE
- ✅ Schema validation (including embeddings)
- ✅ API endpoints

### **Test Coverage After**
- ✅ Workflow catalog CRUD (RETAINED)
- ✅ Audit event generation (RETAINED)
- ✅ Label-only scoring (NEW - needs separate test)
- ✅ Label-only search (NEW - needs separate test)
- ✅ Schema validation (UPDATED - no embedding tests)
- ✅ API endpoints (RETAINED)

**Coverage Gap**: Need NEW integration tests for label-only scoring and search (deferred to follow-up)

---

## ⏱️ **EFFORT ESTIMATE**

| Phase | Task | Effort |
|-------|------|--------|
| Phase 1 | Delete 2 obsolete test files | 2 minutes |
| Phase 2.1 | Update suite_test.go | 3 minutes |
| Phase 2.2 | Update workflow_catalog_test.go | 5 minutes |
| Phase 2.3 | Update schema_validation_test.go | 5 minutes |
| Phase 2.4 | Update server_wiring_test.go | 5 minutes |
| Validation | Run integration tests | 5 minutes |
| **TOTAL** | | **25 minutes** |

---

## 🚀 **PRIORITY JUSTIFICATION**

### **Why IMMEDIATE (P0)**
1. **Blocking**: Integration tests cannot run (build failures)
2. **CI/CD Impact**: Prevents automated testing pipeline
3. **Simple Fix**: Clear action plan with low risk
4. **Confidence**: High (95%) - straightforward test code updates

---

## 🔗 **RELATED DOCUMENTATION**

- **Context**: `docs/handoff/DS_EMBEDDING_REMOVAL_IMPLEMENTATION_COMPLETE.md`
- **API Changes**: `docs/handoff/API_IMPACT_REMOVE_EMBEDDINGS.md`
- **Design Decision**: `docs/architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md`

---

## ✅ **RESOLUTION STATUS**

**Status**: 🔴 **IDENTIFIED** - Ready for immediate implementation

**Next Action**: Execute Phase 1 (Delete obsolete test files)

---

**Triage Completed By**: AI Assistant (Claude)
**Approved For Implementation**: Pending user confirmation
**Estimated Resolution Time**: 25 minutes
