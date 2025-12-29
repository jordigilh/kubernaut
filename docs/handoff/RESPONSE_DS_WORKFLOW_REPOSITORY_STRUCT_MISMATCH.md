# RESPONSE: Data Storage - Workflow Repository Struct Mismatch

**Date**: 2025-12-11
**From**: Data Storage Team (DS Service Scope)
**To**: HAPI Team
**Status**: ⚠️ **ROOT CAUSE IDENTIFIED - Migration Order Issue**
**Priority**: P1 (Blocks all workflow search functionality)

---

## Root Cause Analysis

The error `missing destination name execution_bundle` indicates that **Migration 018 has NOT been applied** in the HAPI test environment.

### Migration Timeline

| Migration | Action | Expected Column Name |
|-----------|--------|---------------------|
| **017** | ADD COLUMN `execution_bundle` | `execution_bundle` |
| **018** | RENAME `execution_bundle` → `container_image` | `container_image` |

### Current State

**Database** (HAPI environment): Has `execution_bundle` column (Migration 017 applied, 018 NOT applied)
**Go Struct** (`RemediationWorkflow`): Has `container_image` field (expecting Migration 018 schema)

**Result**: Struct scanning fails because:
```
SQL: SELECT * FROM remediation_workflow_catalog → returns execution_bundle column
Go:  ContainerImage *string `db:"container_image"` → expects container_image column
```

---

## Evidence

### Migration 017 (Applied in HAPI env)
```sql
-- migrations/017_add_workflow_schema_fields.sql
ALTER TABLE remediation_workflow_catalog
ADD COLUMN execution_bundle TEXT;
```

### Migration 018 (NOT Applied in HAPI env)
```sql
-- migrations/018_rename_execution_bundle_to_container_image.sql
ALTER TABLE remediation_workflow_catalog
RENAME COLUMN execution_bundle TO container_image;
```

### Go Struct (Current)
```go
// pkg/datastorage/models/workflow.go
type RemediationWorkflow struct {
    ContainerImage  *string `db:"container_image"` // ✅ Expects migration 018
    // NO execution_bundle field                    // ❌ Missing migration 017 compat
}
```

---

## Fix Options

### Option A: Apply Missing Migration (RECOMMENDED)

**For HAPI Team**:
```bash
# Apply migration 018 to align with current schema
podman exec -i <postgres_container> psql -U slm_user -d action_history < migrations/018_rename_execution_bundle_to_container_image.sql
```

**Why This is Correct**:
- ✅ Aligns HAPI environment with current schema (DD-WORKFLOW-002 v2.4)
- ✅ No code changes needed in Data Storage service
- ✅ Matches production schema design

---

### Option B: Add Backward Compatibility to Go Struct

**For DS Team** (if HAPI can't migrate immediately):
```go
type RemediationWorkflow struct {
    // V1.0 compatibility (migration 017)
    ExecutionBundle *string `db:"execution_bundle"`

    // V1.1+ (migration 018) - NEW name per DD-WORKFLOW-002 v2.4
    ContainerImage  *string `db:"container_image"`
    ContainerDigest *string `db:"container_digest"`
}
```

**Why This is NOT Recommended**:
- ❌ Maintains legacy schema in production code
- ❌ Violates DD-WORKFLOW-002 v2.4 authority
- ❌ Temporary workaround for migration issue
- ❌ Code debt that needs cleanup later

---

### Option C: Check Migration State

**Diagnostic Command** (run in HAPI environment):
```bash
# Check if migration 018 was applied
podman exec <postgres_container> psql -U slm_user -d action_history -c "
  SELECT column_name
  FROM information_schema.columns
  WHERE table_name = 'remediation_workflow_catalog'
  AND column_name IN ('execution_bundle', 'container_image')
  ORDER BY column_name;
"

# Expected output after migration 018:
# column_name
# --------------
# container_image

# Current output in HAPI (migration 017 only):
# column_name
# ----------------
# execution_bundle
```

---

## Resolution Steps

### Step 1: Verify Migration State (HAPI Team)
```bash
podman exec <postgres_container> psql -U slm_user -d action_history -c "\d+ remediation_workflow_catalog" | grep -E "execution_bundle|container_image"
```

### Step 2A: If Missing Migration 018 (Apply It)
```bash
# Apply migration 018
podman exec -i <postgres_container> psql -U slm_user -d action_history < migrations/018_rename_execution_bundle_to_container_image.sql

# Verify
podman exec <postgres_container> psql -U slm_user -d action_history -c "
  SELECT column_name FROM information_schema.columns
  WHERE table_name = 'remediation_workflow_catalog'
  AND column_name = 'container_image';
"
# Should return: container_image
```

### Step 2B: If Migration Was Applied But Reverted
Check migration history:
```bash
podman exec <postgres_container> psql -U slm_user -d action_history -c "
  SELECT version_id, applied_at
  FROM goose_db_version
  WHERE version_id IN (17, 18)
  ORDER BY version_id;
"
```

### Step 3: Test Fix
```bash
curl -X POST http://localhost:18090/api/v1/workflows/search \
  -H "Content-Type: application/json" \
  -d '{"query": "OOMKilled", "top_k": 5}'

# Should return workflow results, NOT 500 error
```

---

## Impact Assessment

### Blocked Functionality
- ✅ Seed data fixed (signal_* columns) → `RESPONSE_DS_SEED_DATA_SCHEMA_MISMATCH.md`
- ❌ Workflow search blocked (execution_bundle mismatch) → **THIS ISSUE**

### Dependent Systems
| System | Status | Action Required |
|--------|--------|----------------|
| **HAPI** | ❌ Blocked | Apply migration 018 |
| **Data Storage API** | ✅ Working | No changes needed |
| **WorkflowExecution** | ⚠️ Unknown | Verify migration state |

---

## Why This Happened

### Root Cause: Migration Gap

The HAPI test environment appears to have:
1. ✅ Applied seed data fix (alert_* → signal_*)
2. ✅ Applied migration 017 (added execution_bundle)
3. ❌ **NOT applied migration 018** (rename to container_image)

### Prevention

**For All Teams Using Data Storage**:
```bash
# Before running integration tests, verify migrations are current
podman exec <postgres_container> psql -U slm_user -d action_history -c "
  SELECT MAX(version_id) as current_migration
  FROM goose_db_version;
"
# Should return: 18 (or higher)
```

---

## Confidence Assessment

**Confidence**: 95%

**Why High Confidence**:
- ✅ Error message explicitly states "execution_bundle"
- ✅ Migration timeline clearly shows 017 added, 018 renamed
- ✅ Go struct matches migration 018 schema
- ✅ Diagnostic commands will confirm hypothesis

**Remaining 5%**:
- Migration 018 could have been applied then reverted
- Database could have custom modifications
- Multiple database instances with different states

---

## Timeline

**Immediate Action** (HAPI Team): Verify migration state (Step 1)
**Resolution ETA**: < 5 minutes once migration state confirmed
**Testing ETA**: < 10 minutes after fix applied

---

## Summary Table

| Issue | Root Cause | Fix | Owner |
|-------|-----------|-----|-------|
| Seed data `alert_*` | Outdated seed file | ✅ Fixed | DS Team |
| Struct mismatch `execution_bundle` | Missing migration 018 | Apply migration | HAPI Team |

**Both issues must be resolved** for HAPI integration tests to pass.

---

**Status**: Awaiting HAPI team to verify migration state and apply migration 018 if needed.








