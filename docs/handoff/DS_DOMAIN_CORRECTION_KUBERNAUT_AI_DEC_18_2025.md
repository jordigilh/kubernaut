# DataStorage: Domain Correction - kubernaut.ai

**Date**: December 18, 2025
**Service**: DataStorage
**Priority**: **MEDIUM** (Consistency Issue)
**Issue**: RFC 7807 error type URIs use wrong domain

---

## 📋 Issue Summary

DataStorage RFC 7807 error responses use **inconsistent and incorrect domains**:
- ❌ `https://kubernaut.io/problems/...` (wrong domain)
- ❌ `https://api.kubernaut.io/problems/...` (wrong subdomain + wrong domain)
- ✅ Should be: `https://kubernaut.ai/problems/...` (correct domain)

**Impact**: Minor - Error responses functional but use incorrect domain

---

## 🔍 Problem Details

### **Discovered By**
WorkflowExecution integration tests revealed error responses using wrong domain.

### **Current State**
DataStorage returns error responses like:
```json
{
  "type": "https://kubernaut.io/problems/database-error",
  "title": "Database Error",
  "status": 500,
  "detail": "Failed to write audit events batch to database"
}
```

### **Should Be**
```json
{
  "type": "https://kubernaut.ai/problems/database-error",
  "title": "Database Error",
  "status": 500,
  "detail": "Failed to write audit events batch to database"
}
```

---

## 📊 Inconsistency Analysis

### **Pattern 1: `kubernaut.io/problems/*`**
**Files**:
- `pkg/datastorage/server/audit_events_batch_handler.go:172`
```go
Type: "https://kubernaut.io/problems/database-error",
```

### **Pattern 2: `kubernaut.io/errors/*`**
**Files**:
- `pkg/datastorage/server/response/rfc7807.go:57`
```go
// DD-004: Use kubernaut.io/errors/* (NOT api.kubernaut.io/problems/*)
Type: fmt.Sprintf("https://kubernaut.io/errors/%s", errorType),
```

**Note**: Comment suggests this was an intentional decision, but domain is still wrong.

### **Pattern 3: `api.kubernaut.io/problems/*`**
**Files**:
- `pkg/datastorage/server/middleware/openapi.go:204`
- `pkg/datastorage/server/helpers/validation.go:57, 68, 95`
- `pkg/datastorage/server/aggregation_handlers.go:71, 87, 101, 110, 137, 193, 211, 225, 234, 262, 359, 373`

```go
Type: "https://api.kubernaut.io/problems/validation-error",
Type: "https://api.kubernaut.io/problems/internal-error",
```

### **Correct Pattern (Should Be)**
**All locations** should use:
```go
Type: "https://kubernaut.ai/problems/database-error",
Type: "https://kubernaut.ai/problems/validation-error",
Type: "https://kubernaut.ai/problems/internal-error",
```

---

## 🎯 Required Changes

### **Step 1: Global Search and Replace**

**Replace**:
- `https://kubernaut.io/problems/` → `https://kubernaut.ai/problems/`
- `https://kubernaut.io/errors/` → `https://kubernaut.ai/problems/`
- `https://api.kubernaut.io/problems/` → `https://kubernaut.ai/problems/`

**Note**: Also consolidate `/errors/` → `/problems/` for consistency.

### **Step 2: Update DD-004 Comment**

**File**: `pkg/datastorage/server/response/rfc7807.go:55-57`

**Current**:
```go
// DD-004: Use kubernaut.io/errors/* (NOT api.kubernaut.io/problems/*)
// Context API Lesson: Wrong domain caused 6 test failures
Type: fmt.Sprintf("https://kubernaut.io/errors/%s", errorType),
```

**Should Be**:
```go
// DD-004: Use kubernaut.ai/problems/* (correct domain, consistent path)
// RFC 7807 Problem Type URIs MUST use actual domain (kubernaut.ai)
Type: fmt.Sprintf("https://kubernaut.ai/problems/%s", errorType),
```

### **Step 3: Verify Test Compatibility**

After changes, run:
```bash
make test-unit-datastorage
make test-integration-datastorage
make test-e2e-datastorage
```

Verify no tests depend on the old domains.

---

## 📋 Files Requiring Changes

### **High Priority (User-Facing Errors)**
1. `pkg/datastorage/server/audit_events_batch_handler.go:172`
   - Database error response
   - User-facing (returned to clients)

2. `pkg/datastorage/server/helpers/validation.go:57, 68, 95`
   - Validation error responses
   - User-facing (400 errors)

3. `pkg/datastorage/server/middleware/openapi.go:204`
   - OpenAPI validation errors
   - User-facing (400 errors)

### **Medium Priority (Internal/Aggregation)**
4. `pkg/datastorage/server/aggregation_handlers.go` (14 occurrences)
   - Query validation errors
   - Internal server errors
   - Aggregation endpoint errors

### **Low Priority (Framework)**
5. `pkg/datastorage/server/response/rfc7807.go:57`
   - Helper function for error generation
   - Update once, fixes all usages

---

## 🔍 Testing Strategy

### **Unit Tests**
1. Update any tests asserting on error type URIs
2. Search for `kubernaut.io` in test files
3. Replace with `kubernaut.ai`

### **Integration Tests**
1. Verify error responses have correct domain
2. Check WorkflowExecution integration tests still pass
3. Verify all services can parse errors correctly

### **E2E Tests**
1. Trigger error conditions (validation, database, internal)
2. Verify error responses use `kubernaut.ai`
3. Confirm clients handle errors correctly

---

## ⚠️  Impact Assessment

### **Breaking Change?**
**NO** - This is a metadata-only change.

**Rationale**:
- RFC 7807 `type` field is a URI for documentation/categorization
- Clients should not depend on the exact domain
- Error handling should check `status` code, not `type` URI
- No functional behavior changes

### **Client Impact**
**Minimal** - Well-behaved clients won't break.

**Affected Clients**:
- ✅ WorkflowExecution: Uses OpenAPIClientAdapter, parses JSON errors correctly
- ✅ Other services: Should only check `status` code, not `type` URI
- ❌ Poorly-designed clients: May have hardcoded domain checks (should be fixed)

### **Migration Path**
**Immediate** - Can be changed without client coordination.

**Reason**:
- `type` field is informational, not functional
- Status codes remain unchanged
- Error structure remains unchanged
- Only the URI domain changes

---

## 📝 Related Documents

### **RFC 7807 Specification**
The `type` field should be:
> A URI reference [RFC3986] that identifies the problem type. This specification encourages that, when dereferenced, it provide human-readable documentation for the problem type.

**Key Points**:
1. Should be a valid URI
2. Should use the actual service domain
3. Should be dereferenceable (optional but recommended)
4. Is for human readability, not programmatic checks

### **DD-004 Decision**
**Current**: Mentions avoiding `api.kubernaut.io`
**Should Be Updated**: Use `kubernaut.ai` (actual domain)

---

## 🎯 Acceptance Criteria

### **Completion Checklist**
- [ ] All error type URIs use `https://kubernaut.ai/problems/*`
- [ ] No `kubernaut.io` references in error responses
- [ ] No `api.kubernaut.io` references in error responses
- [ ] DD-004 comment updated with correct rationale
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] E2E tests pass
- [ ] WorkflowExecution integration tests pass (verify no regression)

### **Verification**
```bash
# Search for old domains in DataStorage code
grep -r "kubernaut.io" pkg/datastorage/
grep -r "api.kubernaut.io" pkg/datastorage/

# Should return no results (except comments/documentation)

# Run all DataStorage tests
make test-unit-datastorage
make test-integration-datastorage
make test-e2e-datastorage

# Verify WorkflowExecution still works
cd test/integration/workflowexecution
podman-compose -f podman-compose.test.yml up -d --build
cd ../../..
make test-integration-workflowexecution
```

---

## 📞 Priority & Assignment

**Priority**: **MEDIUM**
**Effort**: **LOW** (1-2 hours)
**Assigned To**: DataStorage Team
**Blocking**: No (WorkflowExecution tests still pass with wrong domain)

**Recommended Timeline**:
- Fix before V1.0 production release
- Not urgent, but should be corrected for consistency
- Can be bundled with other DataStorage fixes

---

**Summary**: DataStorage uses inconsistent and incorrect domains in RFC 7807 error type URIs. Should standardize on `https://kubernaut.ai/problems/*` across all error responses. This is a low-risk metadata change that improves consistency and correctness.

