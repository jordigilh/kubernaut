# FOLLOWUP: DS Workflow Search Still Broken

**Date**: 2025-12-11
**From**: HAPI Team
**To**: Data Storage Team
**Reference**: RESPONSE_DS_SEED_DATA_SCHEMA_MISMATCH.md
**Status**: ✅ **BOTH ISSUES TRIAGED**
**DS Response**: See RESPONSE_DS_WORKFLOW_REPOSITORY_STRUCT_MISMATCH.md

---

## Assessment of DS Response

### What Was Fixed ✅
The seed data schema mismatch (`alert_*` → `signal_*`) has been fixed correctly.

### What Is Still Broken ❌

**The DS response incorrectly claims workflow tests are unblocked (lines 89-92).**

Workflow search still returns **500 Internal Server Error**:

```bash
$ curl -X POST http://localhost:18121/api/v1/workflows/search \
    -H "Content-Type: application/json" \
    -d '{"query": "OOMKilled", "top_k": 5}'

{"detail":"Failed to search workflows","status":500}
```

---

## Root Cause: SECOND Issue Not Addressed

### DS Logs Show:
```
ERROR datastorage repository/workflow_repository.go:690
failed to search workflows
{"error": "missing destination name execution_bundle in *[]repository.workflowWithScore"}
```

### Issue
The Go `workflowWithScore` struct in `pkg/datastorage/repository/workflow_repository.go` doesn't have an `execution_bundle` field mapped, but the SQL query returns it.

**This is a separate issue from the seed data fix.**

---

## Handoff Request Already Created

See: `REQUEST_DS_WORKFLOW_REPOSITORY_STRUCT_MISMATCH.md`

**Priority**: P1 (blocks ALL workflow search functionality)

---

## DS Team Triage Summary ✅

| Issue | Status | DS Response | Root Cause | Fix Owner |
|-------|--------|-------------|------------|-----------|
| Seed data `alert_*` → `signal_*` | ✅ **FIXED** | RESPONSE_DS_SEED_DATA_SCHEMA_MISMATCH.md | Seed file outdated | DS Team |
| Go struct `execution_bundle` mismatch | ✅ **TRIAGED** | RESPONSE_DS_WORKFLOW_REPOSITORY_STRUCT_MISMATCH.md | **Missing Migration 018 in HAPI env** | HAPI Team |

### Resolution for Issue #2

**Root Cause**: HAPI test environment has not applied Migration 018 (`execution_bundle` → `container_image` rename)

**Fix** (HAPI Team):
```bash
# Apply missing migration 018
podman exec -i <postgres_container> psql -U slm_user -d action_history < migrations/018_rename_execution_bundle_to_container_image.sql
```

**Why**: Go struct expects `container_image` (migration 018 schema), but HAPI database still has `execution_bundle` (migration 017 schema)

**35 HAPI tests remain blocked** until Migration 018 is applied in HAPI environment.

---

**Verification Command** (after DS fixes both issues):
```bash
curl -X POST http://localhost:18090/api/v1/workflows/search \
  -H "Content-Type: application/json" \
  -d '{"query": "OOMKilled", "top_k": 5}'

# Should return workflow results, NOT 500 error
```

