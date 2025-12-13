# REQUEST: Data Storage - Workflow Repository Struct Mismatch

**Date**: 2025-12-11
**From**: HAPI Team (Triage Session)
**To**: Data Storage Team
**Priority**: P1 (Blocks all workflow search functionality)
**Blocking**: All HAPI workflow catalog integration tests

---

## Issue Summary

The Data Storage workflow search endpoint returns 500 errors due to a Go struct field mismatch.

### Error
```
ERROR: missing destination name execution_bundle in *[]repository.workflowWithScore
```

### Root Cause
The Go repository struct expects `execution_bundle` field, but the database schema was updated to use `container_image`.

**Evidence** (from DS logs):
```
2025-12-11T16:31:14.535Z ERROR datastorage repository/workflow_repository.go:690
failed to search workflows {"query": "OOMKilled", "error": "missing destination name execution_bundle in *[]repository.workflowWithScore"}
```

---

## Affected Files

| File | Issue |
|------|-------|
| `pkg/datastorage/repository/workflow_repository.go:690` | SQL query returns `execution_bundle` but Go struct doesn't have it |
| `pkg/datastorage/repository/types.go` (likely) | `workflowWithScore` struct missing `execution_bundle` field |

---

## Database Schema (Current)

```sql
-- From \\d+ remediation_workflow_catalog
container_image   | text  | DD-WORKFLOW-002 v2.4: OCI image reference
execution_bundle  | text  | ADR-043: OCI bundle or execution reference (V1.1+). NULL for V1.0
```

Both columns exist, but the Go struct may not have all fields mapped.

---

## Required Fix

**Option A**: Add `execution_bundle` field to Go struct
```go
type workflowWithScore struct {
    // existing fields...
    ContainerImage   sql.NullString `db:"container_image"`
    ExecutionBundle  sql.NullString `db:"execution_bundle"`  // ADD THIS
    // ...
}
```

**Option B**: Update SQL query to not select `execution_bundle` if not needed

---

## Impact

### Blocked Functionality
- ALL workflow search requests return 500
- HAPI cannot fetch workflows from Data Storage
- 35 HAPI integration tests failing

### API Affected
```
POST /api/v1/workflows/search -> 500 Internal Server Error
```

---

## Verification

After fix:
```bash
curl -X POST http://localhost:18090/api/v1/workflows/search \
  -H "Content-Type: application/json" \
  -d '{"query": "OOMKilled", "top_k": 5}'

# Should return workflow results, not 500 error
```

---

## Timeline Request

This is a **P1 blocker** - all workflow functionality is broken.

---

**Triage Confidence**: 98%
- Clear error message in logs
- Line number identified (workflow_repository.go:690)
- Standard Go struct scanning issue






