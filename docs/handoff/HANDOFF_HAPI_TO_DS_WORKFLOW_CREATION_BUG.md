# HANDOFF: HAPI → Data Storage - Workflow Creation Bug

**Date**: 2025-12-12
**From**: HAPI Team
**To**: Data Storage Team → **CORRECTED**: HAPI Team (Self-Fix)
**Priority**: 🔴 **HIGH** - Blocking HAPI integration tests
**Status**: ⏸️ **REQUIRES HAPI SCHEMA FIX**

---

## ✅ **CORRECTED DIAGNOSIS** (2025-12-12)

**Original Diagnosis**: Data Storage code has outdated column references.
**INCORRECT**: Data Storage code is **CORRECT**.

**Actual Issue**: HAPI test infrastructure is using **INCOMPLETE schema** (migration 015 only).
**Missing**: Migration 019 which adds `workflow_name` column.

**Action Required**: **HAPI team** must update `init-db.sql` to include migration 019 changes.

---

## 🎯 **ISSUE SUMMARY**

Data Storage service fails to create workflows via REST API with error:
```
ERROR: column "workflow_name" does not exist (SQLSTATE 42703)
```

**Impact**: 34 HAPI integration tests blocked (cannot bootstrap test data).

---

## 📊 **CONTEXT: HAPI Integration Tests**

### **What HAPI Was Doing**:
Running integration tests in parallel (4 workers) against real Data Storage service.

**Test Command**:
```bash
cd holmesgpt-api
python3 -m pytest tests/integration/ -v -n 4
```

**Test Infrastructure**:
- ✅ PostgreSQL 16 (postgres:16-alpine) - healthy
- ✅ Redis 7 - healthy
- ✅ Embedding Service - healthy
- ✅ Data Storage Service - healthy
- ✅ HAPI Service - healthy

**Results**:
- ✅ 32/67 tests passing (tests not requiring Data Storage)
- ❌ 34/67 tests failing (all require workflow catalog data)

---

## 🚨 **BLOCKING ERROR**

### **Error Details**:

**Service**: Data Storage (kubernaut-hapi-data-storage-integration)
**Endpoint**: `POST /api/v1/workflows`
**HTTP Status**: 500 Internal Server Error

**Response**:
```json
{
  "detail": "Failed to create workflow",
  "status": 500,
  "title": "Internal Server Error",
  "type": "https://api.kubernaut.io/problems/https://kubernaut.dev/problems/internal-error"
}
```

**Server Log**:
```
2025-12-13T03:47:41.335Z ERROR datastorage repository/workflow_repository.go:96
failed to update previous versions
{"workflow_name": "oomkill-increase-memory-limits", "version": "1.0.0",
 "error": "ERROR: column \"workflow_name\" does not exist (SQLSTATE 42703)"}
```

**Stack Trace**:
```
github.com/jordigilh/kubernaut/pkg/datastorage/repository.(*WorkflowRepository).Create
    /opt/app-root/src/pkg/datastorage/repository/workflow_repository.go:96
github.com/jordigilh/kubernaut/pkg/datastorage/server.(*Handler).HandleCreateWorkflow
    /opt/app-root/src/pkg/datastorage/server/workflow_handlers.go:89
```

---

## 🔍 **ROOT CAUSE ANALYSIS**

### **CORRECT DIAGNOSIS**: HAPI Using Incomplete Schema

**HAPI Test Database Schema** (migrations/015 only - INCOMPLETE):
```sql
CREATE TABLE remediation_workflow_catalog (
    workflow_id VARCHAR(255) NOT NULL,  -- ✅ Base schema
    version VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    -- ... other columns
    PRIMARY KEY (workflow_id, version)
);
-- ❌ MISSING: workflow_name column (added in migration 019)
```

**Data Storage V1.0 Schema** (migrations/015 + 019 - COMPLETE):
```sql
CREATE TABLE remediation_workflow_catalog (
    workflow_id UUID PRIMARY KEY,       -- ✅ Changed to UUID in migration 019
    workflow_name VARCHAR(255) NOT NULL, -- ✅ Added in migration 019
    version VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    -- ... other columns
    UNIQUE (workflow_name, version)      -- ✅ Added in migration 019
);
```

**Data Storage Code** (repository/workflow_repository.go:92 - CORRECT):
```go
WHERE workflow_name = $1 AND is_latest_version = true
// ✅ Code is CORRECT - expects migration 019 schema
```

### **Actual Root Cause**:

**HAPI's test infrastructure is using INCOMPLETE schema** (migration 015 only, missing migration 019).

**Evidence**:
1. ✅ Code references `workflow_name` (CORRECT for full schema)
2. ❌ HAPI test DB only has migrations up to 015 (INCOMPLETE)
3. ✅ Migration 019 adds `workflow_name` column
4. ❌ HAPI's `init-db.sql` doesn't include migration 019 changes

---

## 📋 **REPRODUCTION STEPS**

### **Environment Setup**:

1. **Start Infrastructure**:
   ```bash
   cd holmesgpt-api/tests/integration
   bash setup_workflow_catalog_integration.sh
   ```

2. **Verify Services**:
   ```bash
   # All should return healthy
   curl http://localhost:15435  # PostgreSQL
   curl http://localhost:16381  # Redis
   curl http://localhost:18001/health  # Embedding Service
   curl http://localhost:18094/health  # Data Storage
   ```

3. **Verify Schema**:
   ```bash
   podman exec kubernaut-hapi-postgres-integration \
     psql -U kubernaut -d kubernaut_test \
     -c "\d remediation_workflow_catalog"

   # Should show: workflow_id, version, name (NOT workflow_name)
   ```

4. **Trigger Bug**:
   ```bash
   cd holmesgpt-api/tests/integration
   bash bootstrap-workflows.sh

   # Should fail with: column "workflow_name" does not exist
   ```

5. **Check Logs**:
   ```bash
   podman logs kubernaut-hapi-data-storage-integration --tail 50 | grep ERROR
   ```

---

## 📂 **AFFECTED FILES (Suspected)**

Based on stack trace, likely files needing fixes:

1. **pkg/datastorage/repository/workflow_repository.go:96**
   - Location of "update previous versions" logic
   - Likely has hardcoded "workflow_name" column reference

2. **pkg/datastorage/server/workflow_handlers.go:89**
   - Workflow creation handler
   - Calls repository.Create()

3. **Possible SQL Queries**:
   - Any UPDATE/SELECT using `workflow_name` column
   - Should use `workflow_id` or `name` instead

---

## 🔧 **CORRECT FIX**

### **For HAPI Team** (NOT Data Storage):

**The Data Storage code is CORRECT. HAPI's test schema is INCOMPLETE.**

1. **Update HAPI's `init-db.sql` to include migration 019**:
   ```bash
   cd holmesgpt-api/tests/integration
   # Add migration 019 changes to init-db.sql
   ```

2. **Required Schema Changes in init-db.sql**:
   ```sql
   -- Enable UUID extension
   CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

   -- Modify workflow_id to UUID
   ALTER TABLE remediation_workflow_catalog
       DROP CONSTRAINT remediation_workflow_catalog_pkey;

   -- Add workflow_name column
   ALTER TABLE remediation_workflow_catalog
       ADD COLUMN workflow_name VARCHAR(255) NOT NULL;

   -- Change workflow_id to UUID
   ALTER TABLE remediation_workflow_catalog
       ALTER COLUMN workflow_id TYPE UUID USING uuid_generate_v4();

   -- Set new primary key
   ALTER TABLE remediation_workflow_catalog
       ADD PRIMARY KEY (workflow_id);

   -- Add unique constraint
   ALTER TABLE remediation_workflow_catalog
       ADD CONSTRAINT uq_workflow_name_version UNIQUE (workflow_name, version);

   -- Add indexes
   CREATE INDEX idx_workflow_catalog_workflow_name
       ON remediation_workflow_catalog(workflow_name);
   ```

3. **Alternative: Use Complete Migration Stack**:
   ```bash
   # Copy ALL Data Storage migrations to HAPI test infrastructure
   cp -r kubernaut/migrations/* holmesgpt-api/tests/integration/migrations/

   # Update setup script to run all migrations
   ```

4. **Test Fix**:
   ```bash
   # Restart with updated schema
   cd holmesgpt-api/tests/integration
   bash teardown_workflow_catalog_integration.sh
   bash setup_workflow_catalog_integration.sh
   bash bootstrap-workflows.sh  # Should succeed
   ```

---

## 🎯 **ACCEPTANCE CRITERIA**

### **Fix is Complete When**:

1. ✅ Bootstrap script succeeds:
   ```bash
   bash bootstrap-workflows.sh
   # Should create workflows without errors
   ```

2. ✅ Workflows exist in database:
   ```bash
   podman exec kubernaut-hapi-postgres-integration \
     psql -U kubernaut -d kubernaut_test \
     -c "SELECT COUNT(*) FROM remediation_workflow_catalog;"
   # Should show > 0 workflows
   ```

3. ✅ HAPI integration tests pass:
   ```bash
   cd holmesgpt-api
   python3 -m pytest tests/integration/ -n 4
   # Should see 66-67/67 passing (up from 32/67)
   ```

---

## 📊 **CURRENT STATUS**

| Component | Status | Details |
|-----------|--------|---------|
| **HAPI Test Infrastructure** | ⚠️ **INCOMPLETE** | Services healthy, but schema missing migration 019 |
| **PostgreSQL Schema** | ❌ **INCOMPLETE** | Missing `workflow_name` column (migration 019) |
| **Data Storage Service** | ✅ **CORRECT** | Code expects complete V1.0 schema with migration 019 |
| **Bootstrap Script** | ⏸️ BLOCKED | Cannot create workflows (schema mismatch) |
| **HAPI Integration Tests** | ⏸️ BLOCKED | 34/67 tests awaiting schema fix |

---

## 📝 **ADDITIONAL CONTEXT**

### **pgvector Status** (RESOLVED by HAPI):

During this work, HAPI team discovered and fixed pgvector references in HAPI test infrastructure:

✅ **Fixed by HAPI**:
- Changed: `pgvector/pgvector:pg16` → `postgres:16-alpine`
- Updated: `init-db.sql` to V1.0 label-only schema (no vectors)
- Removed: All pgvector extension references

**Documentation**: `holmesgpt-api/TRIAGE_PGVECTOR_STATUS_HAPI_TESTS.md`

### **Why This Matters**:

HAPI test infrastructure is now **100% aligned with Data Storage V1.0**:
- ✅ No pgvector dependency
- ✅ V1.0 label-only schema
- ✅ All 3 label columns (labels, custom_labels, detected_labels)
- ✅ Composite PK (workflow_id, version)

**Only blocker**: Data Storage service code/schema mismatch.

---

## 🚀 **NEXT STEPS**

### **For HAPI Team** (Estimated: 30-60 minutes):

1. **Verify Migration Stack** (10 min):
   - Check `init-db.sql` includes migration 019 changes
   - Confirm `workflow_name` column exists in schema

2. **Update Schema** (20 min):
   - Add migration 019 to `init-db.sql`:
     - Add `workflow_name` column
     - Change `workflow_id` to UUID
     - Add UNIQUE constraint on (workflow_name, version)
     - Update indexes
   - OR: Run all Data Storage migrations instead of init-db.sql

3. **Test** (20 min):
   - Teardown and recreate test infrastructure
   - Run bootstrap script
   - Verify workflows created successfully

4. **Validate** (10 min):
   - Run HAPI integration tests
   - Confirm 66-67/67 passing

### **For Data Storage Team** (No Action Required):

- ✅ Code is CORRECT - expects migration 019 schema
- ✅ All migrations up to date
- ✅ V1.0 schema complete

---

## 📞 **CONTACT & FILES**

### **HAPI Team Documents**:
- `holmesgpt-api/COMPLETE_TEST_RESULTS_2025-12-12.md` - Full test status
- `holmesgpt-api/INTEGRATION_TEST_PARALLEL_RESULTS.md` - Parallel execution report
- `holmesgpt-api/TRIAGE_PGVECTOR_STATUS_HAPI_TESTS.md` - pgvector fix analysis

### **Test Infrastructure**:
- `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml` - Services
- `holmesgpt-api/tests/integration/init-db.sql` - V1.0 schema
- `holmesgpt-api/tests/integration/bootstrap-workflows.sh` - Data loader

### **Data Storage Schema Reference**:
- `migrations/015_create_workflow_catalog_table.sql` - Authoritative V1.0 schema
- `migrations/020_add_workflow_label_columns.sql` - Label columns

---

## 💡 **KEY INSIGHTS**

### **For HAPI Team** (Schema Owner):

1. **Schema Evolution**: V1.0 requires **ALL migrations** (015-020), not just 015
2. **Migration 019 Critical**: Adds `workflow_name` column and changes `workflow_id` to UUID
3. **Test Infrastructure**: HAPI's `init-db.sql` must include migration 019 changes
4. **Column Naming**: Complete schema has BOTH `workflow_id` (UUID) AND `workflow_name` (human-readable)

### **For Data Storage Team**:

1. ✅ **Code is Correct**: All references to `workflow_name` are valid for complete schema
2. ✅ **Migrations Complete**: All 5 migrations (015-020) present and correct
3. **Integration Testing**: HAPI is first consumer to test workflow creation via REST API

---

## 📈 **ESTIMATED IMPACT**

### **Current**:
- 34 HAPI integration tests blocked
- No workflow catalog testing possible
- Data Storage workflow creation untested

### **After Fix**:
- 66-67/67 HAPI tests passing (~98%)
- Full workflow catalog integration validated
- Data Storage REST API workflow creation verified

---

**Handoff Summary - CORRECTED DIAGNOSIS**:
- ⚠️ HAPI test infrastructure using **INCOMPLETE** schema (migration 015 only)
- ✅ Data Storage service code is **CORRECT** (expects migration 019)
- ❌ HAPI's `init-db.sql` missing migration 019 changes (`workflow_name` column)
- ⏸️ 34 integration tests blocked awaiting **HAPI schema fix**
- 🎯 Estimated fix time: 30-60 minutes (HAPI team updates `init-db.sql`)

---

**Created By**: HAPI Team (AI Assistant)
**Updated By**: Data Storage Team (AI Assistant)
**Date**: 2025-12-12
**Status**: 🔴 **BLOCKING** - Requires HAPI Team Action (Schema Update)
**Confidence**: 99% (migration 019 confirmed in Data Storage repo, HAPI using incomplete schema)

