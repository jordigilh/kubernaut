# Data Storage Refactoring - Continuation Plan

**Date**: 2025-12-13
**Status**: ⏸️ **Checkpoint** - ~3 hours invested, ~25-36 hours remaining
**Current Phase**: Phase 2 (Extract & Consolidate) - IN PROGRESS

---

## ✅ **Completed Work (3 hours)**

### **Phase 1: Cleanup (2 hours)** ✅
1. ✅ Removed embedding directory (859 lines)
2. ✅ Deleted legacy client.go (321 lines)
3. ✅ Updated workflow_repository.go (removed embedding client)
4. ✅ Updated server.go (removed embedding parameter)
5. ✅ Cleaned 6 TODOs with V1.0 context
6. ✅ All packages compile successfully

**Total removed**: 1,180 lines

### **Phase 2: Extract & Consolidate (1 hour so far)** ⏳
1. ✅ Created response helpers package
   - `pkg/datastorage/server/response/rfc7807.go` (93 lines)
   - `pkg/datastorage/server/response/json.go` (68 lines)
   - Total: 161 lines of reusable helpers

---

## ⏳ **In Progress: Phase 2 Continuation**

### **Step 2.2: Migrate Handlers to Response Helpers**
**Status**: ⏸️ NOT STARTED
**Effort**: 1-2 hours

**Files to Update** (6 files):
1. `pkg/datastorage/server/handler.go`
2. `pkg/datastorage/server/aggregation_handlers.go`
3. `pkg/datastorage/server/audit_events_handler.go`
4. `pkg/datastorage/server/audit_events_batch_handler.go`
5. `pkg/datastorage/server/audit_handlers.go`
6. `pkg/datastorage/server/workflow_handlers.go`

**Migration Pattern**:
```go
// BEFORE (duplicated in every handler)
func (h *Handler) writeRFC7807Error(w http.ResponseWriter, status int, errorType, title, detail string) {
    problemDetail := map[string]interface{}{
        "type":   fmt.Sprintf("https://api.kubernaut.io/problems/%s", errorType),
        "title":  title,
        "status": status,
        "detail": detail,
    }
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(problemDetail)
}

// AFTER (use shared helper)
import "github.com/jordigilh/kubernaut/pkg/datastorage/server/response"

// Replace all h.writeRFC7807Error calls with:
response.WriteRFC7807Error(w, status, errorType, title, detail, h.logger)
```

**Tasks**:
- [ ] Update import statements in all 6 files
- [ ] Replace `h.writeRFC7807Error` with `response.WriteRFC7807Error`
- [ ] Remove local `writeRFC7807Error` method from each handler
- [ ] Replace manual JSON encoding with `response.WriteJSON`
- [ ] Test compilation after each file

---

### **Step 2.3: Consolidate Validation Logic**
**Status**: ⏸️ NOT STARTED
**Effort**: 1-2 hours

**Create**: `pkg/datastorage/server/validation/request_validation.go`

**Current Duplication**:
```go
// Duplicated in multiple handlers:
func parseLimit(limitStr string) (int, error) { ... }
func parseOffset(offsetStr string) (int, error) { ... }
func isValidSeverity(sev string) bool { ... }
```

**Consolidation Plan**:
```go
// pkg/datastorage/server/validation/request_validation.go
package validation

// ParseLimit validates and parses limit query parameter
func ParseLimit(limitStr string, defaultLimit, maxLimit int) (int, error)

// ParseOffset validates and parses offset query parameter
func ParseOffset(offsetStr string) (int, error)

// ValidateSeverity checks if severity value is valid
func ValidateSeverity(severity string) bool

// ValidateUUID checks if string is valid UUID format
func ValidateUUID(id string) bool
```

**Tasks**:
- [ ] Create validation package
- [ ] Extract validation functions
- [ ] Update handlers to use shared validation
- [ ] Remove duplicate validation code

---

## 📋 **Phase 3: Split Large Files (8-12 hours)**

### **Task 3.1: Split workflow_repository.go (4-6h)**
**File**: `pkg/datastorage/repository/workflow_repository.go` (1,173 lines)

**Create Directory**: `pkg/datastorage/repository/workflow/`

**Split Into**:
1. `crud.go` - Create, Update, Delete, Get operations (~250 lines)
2. `search.go` - SearchByLabels logic (~300 lines)
3. `versioning.go` - Version management (~200 lines)
4. `validation.go` - Schema validation (~150 lines)
5. `sql_builder.go` - Shared SQL construction (~150 lines)
6. `repository.go` - Main struct and constructor (~50 lines)

**Steps**:
1. Create `workflow/` subdirectory
2. Move CRUD methods to `crud.go`
3. Move search logic to `search.go`
4. Extract versioning logic to `versioning.go`
5. Extract validation to `validation.go`
6. Create shared SQL builder
7. Update imports in server.go
8. Test compilation
9. Run integration tests

---

### **Task 3.2: Split audit_events_handler.go (3-4h)**
**File**: `pkg/datastorage/server/audit_events_handler.go` (990 lines)

**Create Directory**: `pkg/datastorage/server/audit/`

**Split Into**:
1. `create_handler.go` - POST /api/v1/audit/events (~250 lines)
2. `batch_handler.go` - POST /api/v1/audit/events/batch (~200 lines)
3. `query_handler.go` - GET /api/v1/audit/events (~200 lines)
4. `validation.go` - Shared validation logic (~150 lines)
5. `types.go` - Response types (~100 lines)

**Steps**:
1. Create `audit/` subdirectory
2. Move create handler to `create_handler.go`
3. Move batch handler to `batch_handler.go`
4. Move query handler to `query_handler.go`
5. Extract shared validation
6. Extract response types
7. Update imports in server.go
8. Test compilation
9. Run E2E tests

---

### **Task 3.3: Split DLQ Client (2-3h)**
**File**: `pkg/datastorage/dlq/client.go` (599 lines)

**Split Into**:
1. `client.go` - Core DLQ operations (Push, Pop, Len) (~250 lines)
2. `monitoring.go` - Capacity monitoring logic (~200 lines)
3. `metrics.go` - Prometheus metrics export (~150 lines)

**Steps**:
1. Move monitoring logic to `monitoring.go`
2. Move Prometheus metrics to `metrics.go`
3. Keep core operations in `client.go`
4. Update imports
5. Test compilation
6. Run integration tests

---

## 📋 **Phase 4: SQL Query Builder (4-5 hours)**

### **Task 4.1: Create SQL Builder Package (2-3h)**
**Create**: `pkg/datastorage/repository/sql/`

**Files**:
1. `builder.go` - Fluent SQL builder API
2. `filters.go` - WHERE clause construction
3. `pagination.go` - LIMIT/OFFSET handling
4. `types.go` - Query parameter types

**Features**:
- Type-safe query construction
- Parameterized queries (prevent SQL injection)
- Fluent API (chainable methods)
- WHERE clause builder
- ORDER BY support
- LIMIT/OFFSET pagination

**Example API**:
```go
query := sql.NewBuilder().
    Select("*").
    From("workflows").
    Where(sql.Eq("signal_type", filters.SignalType)).
    Where(sql.Eq("severity", filters.Severity)).
    OrderBy("created_at", sql.DESC).
    Limit(filters.Limit).
    Offset(filters.Offset).
    Build()
```

---

### **Task 4.2: Migrate Repositories (2-3h)**
**Files to Update**:
1. `repository/workflow_repository.go` (or workflow/*.go after split)
2. `repository/audit_events_repository.go`
3. `repository/action_trace_repository.go`

**Migration Steps per Repository**:
1. Identify all SQL query construction
2. Replace string concatenation with builder API
3. Test SQL output matches original queries
4. Run integration tests
5. Verify performance (no regression)

---

## 📋 **Phase 5: Validation (1-2 hours)**

### **Task 5.1: Run Full Test Suite**
```bash
# Unit tests
go test ./pkg/datastorage/... -v

# Integration tests
go test ./test/integration/datastorage/... -v

# E2E tests
make test-e2e-datastorage
```

**Expected Results**:
- ✅ All unit tests pass
- ✅ All integration tests pass
- ✅ All 85/85 E2E tests pass
- ✅ No performance regression

---

### **Task 5.2: Update Documentation**
**Files to Update**:
1. Update `DATASTORAGE_V1.0_FINAL_DELIVERY_2025-12-13.md`
2. Create `DS_REFACTORING_COMPLETE_V1.1.md`
3. Update code comments with refactoring notes
4. Document new package structure

---

## 📊 **Progress Tracking**

| Phase | Tasks | Status | Time | Remaining |
|-------|-------|--------|------|-----------|
| **Phase 1** | Cleanup | ✅ Complete | 2h | 0h |
| **Phase 2** | Extract & Consolidate | ⏳ In Progress | 1h | 2-3h |
| **Phase 3** | Split Large Files | ⏸️ Pending | 0h | 8-12h |
| **Phase 4** | SQL Query Builder | ⏸️ Pending | 0h | 4-5h |
| **Phase 5** | Validation | ⏸️ Pending | 0h | 1-2h |
| **TOTAL** | | | **3h** | **15-22h** |

**Updated Estimate**: 18-25 hours remaining (down from 28-39h)

---

## 🎯 **Continuation Instructions**

### **Immediate Next Steps** (Pick up where we left off)

1. **Complete Phase 2.2**: Migrate handlers to response helpers (1-2h)
   - Start with `handler.go`
   - Update imports
   - Replace `writeRFC7807Error` calls
   - Test compilation
   - Repeat for remaining 5 files

2. **Complete Phase 2.3**: Consolidate validation (1-2h)
   - Create validation package
   - Extract validation functions
   - Update handlers

3. **Proceed to Phase 3**: Split large files (8-12h)
   - Start with workflow_repository.go (biggest impact)
   - Then audit_events_handler.go
   - Then dlq/client.go

4. **Proceed to Phase 4**: SQL query builder (4-5h)
   - Design and implement builder API
   - Migrate repositories

5. **Finish with Phase 5**: Validation (1-2h)
   - Run all tests
   - Update documentation

---

## ✅ **Success Criteria**

**Code Quality**:
- [ ] All packages compile without errors
- [ ] All tests pass (unit + integration + E2E)
- [ ] No performance regression
- [ ] Reduced code duplication (30%+ reduction)

**Maintainability**:
- [ ] No file >600 lines (except generated code)
- [ ] Clear separation of concerns
- [ ] Consistent error handling
- [ ] Type-safe SQL queries

**Documentation**:
- [ ] Refactoring documented
- [ ] New package structure documented
- [ ] Code comments updated
- [ ] V1.1 summary created

---

## 🔗 **Related Documents**

- `TRIAGE_DS_REFACTORING_OPPORTUNITIES_V1.1.md` - Original triage
- `DS_REFACTORING_PROGRESS_SESSION_1.md` - Session 1 progress
- `DATASTORAGE_V1.0_FINAL_DELIVERY_2025-12-13.md` - V1.0 baseline

---

**Document Version**: 1.0
**Last Updated**: 2025-12-13
**Status**: ⏸️ CHECKPOINT - Ready to continue with Phase 2.2

