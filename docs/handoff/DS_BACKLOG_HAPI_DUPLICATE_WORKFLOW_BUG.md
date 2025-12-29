# DS Team Backlog: HAPI Duplicate Workflow Bug
**Date Received**: December 27, 2025
**Reporter**: HolmesGPT-API (HAPI) Team
**Status**: 📋 **BACKLOG** - Queued after current audit timing work
**Priority**: P1 (High - HTTP compliance violation)

---

## 🎯 **QUICK SUMMARY**

**Bug**: DataStorage returns **500 Internal Server Error** when creating duplicate workflows, instead of RFC 9110-compliant **409 Conflict**.

**Impact**:
- ❌ Violates HTTP standards (RFC 9110)
- ❌ Blocks HAPI integration tests (requires workaround)
- ❌ Poor API client experience (can't distinguish server errors from conflicts)

**Estimated Fix Time**: 1.75 days
**Complexity**: LOW (< 10 lines of code change)

---

## 📋 **BUG DETAILS**

**Full Report**: `/docs/bugs/DS-BUG-001-DUPLICATE-WORKFLOW-500-ERROR.md`

### Current Behavior ❌
```bash
# Create workflow twice
curl -X POST /api/v1/workflows -d '{"workflow_name": "test", "version": "1.0.0", ...}'

# First: 201 Created ✅
# Second: 500 Internal Server Error ❌ (SHOULD BE 409 Conflict)
```

### Expected Behavior ✅
```bash
# Second request should return:
HTTP/1.1 409 Conflict
{
  "type": "https://kubernaut.ai/problems/conflict",
  "title": "Workflow Already Exists",
  "status": 409,
  "detail": "Workflow 'test' version '1.0.0' already exists"
}
```

---

## 🔍 **ROOT CAUSE**

**File**: `pkg/datastorage/server/workflow_handlers.go`
**Function**: `HandleCreateWorkflow`
**Lines**: 83-90

**Problem**: Handler doesn't distinguish PostgreSQL duplicate key errors (SQLSTATE 23505) from other database errors.

```go
// Current code (line 83-90)
if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
    h.logger.Error(err, "Failed to create workflow", ...)
    // ❌ Returns 500 for ALL errors (including duplicates)
    response.WriteRFC7807Error(w, http.StatusInternalServerError, ...)
    return
}
```

---

## 🔧 **RECOMMENDED FIX**

### Option 1: Use pgx Error Types (Preferred)

```go
import (
    "errors"
    "github.com/jackc/pgx/v5/pgconn"
)

if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
    // Check for PostgreSQL unique constraint violation
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) && pgErr.Code == "23505" {
        detail := fmt.Sprintf("Workflow '%s' version '%s' already exists",
                            workflow.WorkflowName, workflow.Version)
        response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
            "Workflow Already Exists", detail, h.logger)
        return
    }

    // Other errors remain 500
    h.logger.Error(err, "Failed to create workflow", ...)
    response.WriteRFC7807Error(w, http.StatusInternalServerError, ...)
    return
}
```

### Option 2: String Matching (Simpler but less robust)

```go
if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
    // Check for duplicate key error
    if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
        detail := fmt.Sprintf("Workflow '%s' version '%s' already exists",
                            workflow.WorkflowName, workflow.Version)
        response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
            "Workflow Already Exists", detail, h.logger)
        return
    }

    // Other errors remain 500
    h.logger.Error(err, "Failed to create workflow", ...)
    response.WriteRFC7807Error(w, http.StatusInternalServerError, ...)
    return
}
```

---

## ✅ **ACCEPTANCE CRITERIA**

After fix:
1. ✅ Duplicate workflow creation returns 409 Conflict
2. ✅ Error message includes workflow name and version
3. ✅ RFC 7807 problem details format used
4. ✅ Logs use INFO level for duplicates (not ERROR)
5. ✅ Other database errors still return 500
6. ✅ Integration test added for duplicate workflow scenario
7. ✅ OpenAPI spec updated with 409 response

---

## 📊 **PRIORITY ASSESSMENT**

### Why P1 (High Priority)?
- ❌ Violates HTTP standards (RFC 9110 Section 15.5.10)
- ❌ Blocks HAPI integration tests (requires full database reset)
- ❌ Poor API client experience
- ✅ Easy fix (< 10 lines of code)
- ✅ No breaking changes

### Why NOT P0 (Critical)?
- ✅ Workaround exists (database reset before tests)
- ✅ Not causing production outages
- ✅ Not blocking other teams' critical work

---

## 🗓️ **RECOMMENDED TIMELINE**

**Total Effort**: 1.75 days

| Phase | Duration | Tasks |
|-------|----------|-------|
| Analysis | 0.5 days | Confirm root cause, check similar issues |
| Implementation | 0.5 days | Fix handler, add integration test |
| Testing | 0.5 days | Verify fix, test edge cases |
| Documentation | 0.25 days | Update OpenAPI spec, changelog |

**Start Date**: After audit timing bug is resolved
**Target Completion**: Within next sprint

---

## 🔗 **RELATED ISSUES TO CHECK**

While fixing, also check:
1. **Other Create Operations**: Incident creation, audit event creation, etc.
2. **Update Operations**: Workflow updates with version conflicts
3. **Error Code Patterns**: Search for other `http.StatusInternalServerError` misuses

---

## 📝 **QUESTIONS FOR DS TEAM (When Ready)**

1. **PostgreSQL Error Handling**: Prefer string matching or `pgconn.PgError` types?
2. **Logging Level**: INFO or WARN for duplicate workflow attempts?
3. **Similar Issues**: Other endpoints with similar error handling?
4. **Testing**: Existing integration tests to update?
5. **OpenAPI Spec**: Where is canonical spec stored?

---

## 🚦 **CURRENT STATUS**

**Status**: 📋 **BACKLOG**

**Current Work**:
1. 🚨 **ACTIVE**: Audit buffer flush timing bug (blocking **7 services**)
2. 🔜 **NEXT**: Add audit client integration tests

**This Issue**:
- Queued for next available sprint slot
- Will be addressed after audit timing work completes
- HAPI team has workaround (database reset before tests)

---

## 📬 **COMMUNICATION**

**To HAPI Team**:
> "Thanks for the detailed bug report! We've queued this as P1 for the next sprint. Currently working on an audit timing bug that's blocking 5 services. Your workaround (database reset) should suffice until we implement the proper 409 response. ETA: Within next sprint (1-2 weeks)."

**Questions for HAPI Team**:
- Is the database reset workaround acceptable for now?
- Are there other Data Storage API issues we should know about?
- Would you like to be notified when this is fixed?

---

**Document Version**: 1.0
**Last Updated**: December 27, 2025
**Full Bug Report**: `/docs/bugs/DS-BUG-001-DUPLICATE-WORKFLOW-500-ERROR.md`

