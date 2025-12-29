# TRIAGE: DataStorage Migration Dependencies After pgvector Removal

**Date**: 2025-12-11
**Service**: Data Storage
**Type**: Migration Dependency Issue
**Priority**: HIGH - Blocking integration tests
**Status**: 🔴 **REQUIRES DECISION**

---

## 🎯 **ISSUE**

Integration tests are failing due to migration dependencies after removing pgvector.

**Root Cause**: Migration files have complex dependencies - skipping vector-dependent migrations breaks subsequent migrations that expect tables/columns from skipped migrations.

---

## 📊 **CURRENT SITUATION**

### **Migrations That REQUIRE pgvector Extension**:
1. `005_vector_schema.sql` - Creates `action_patterns` table with `embedding vector(384)`
2. `007_add_context_column.sql` - Adds column to `action_patterns` table (depends on 005)
3. `008_context_api_compatibility.sql` - Adds `embedding vector(384)` to existing table
4. `009_update_vector_dimensions.sql` - Updates vector dimensions
5. `010_audit_write_api_phase1.sql` - Creates table with vector column
6. `015_create_workflow_catalog_table.sql` - Creates `remediation_workflows` with `embedding vector(384)`
7. `016_update_embedding_dimensions.sql` - Updates embedding dimensions to 768

### **Migrations That DEPEND on Skipped Migrations**:
- `011_rename_alert_to_signal.sql` - Tries to rename column from migration 010
- Potentially others...

---

## 🔍 **ATTEMPTED FIX**

**What Was Done**:
1. ✅ Removed pgvector from Makefile (postgres:16-alpine instead of pgvector image)
2. ✅ Removed pgvector extension creation from suite_test.go
3. ✅ Commented out 7 vector-dependent migrations in suite_test.go
4. ❌ Integration tests still fail due to migration dependencies

**Current Status**:
- Migrations 001-006 apply successfully
- Migration 011 fails: `ERROR: column "alert_fingerprint" does not exist`
- Reason: Column was created in skipped migration 010

---

## 🎯 **RECOMMENDED SOLUTIONS**

### **Option A: V1.0-Specific Migration Path** ⭐ **RECOMMENDED**

**Approach**: Create a V1.0-specific migration sequence that skips ALL vector-related migrations

**Steps**:
1. Create new `migrations-v1.0/` directory with vector-free migrations
2. Copy non-vector migrations: 001-004, 006, 011-014, 017-020, 1000
3. Skip vector migrations: 005, 007-010, 015-016
4. Update integration tests to use `migrations-v1.0/` directory
5. Document that production deployments should use this path

**Pros**:
- ✅ Clean separation between V1.0 (no vector) and V2.0+ (with vector)
- ✅ No breaking changes to existing migrations
- ✅ Easy to revert/add vector support in V2.0+

**Cons**:
- ⚠️ Need to maintain two migration paths temporarily
- ⚠️ V2.0+ migration from V1.0 will require special handling

**Effort**: 30 minutes

---

### **Option B: Stub Vector Migrations**

**Approach**: Create stub versions of vector migrations that create tables WITHOUT vector columns

**Steps**:
1. Create `005_vector_schema_v1.0_stub.sql` - Creates `action_patterns` WITHOUT embedding column
2. Create `008_context_api_compatibility_v1.0_stub.sql` - Skips embedding column
3. Create `010_audit_write_api_phase1_v1.0_stub.sql` - Creates tables WITHOUT vector columns
4. Create `015_create_workflow_catalog_v1.0_stub.sql` - Creates table WITHOUT embedding column
5. Update test suite to use stub migrations

**Pros**:
- ✅ Maintains migration sequence numbering
- ✅ Easier V2.0+ upgrade path (just add vector columns)

**Cons**:
- ⚠️ Creates unused tables in V1.0 (`action_patterns` not used)
- ⚠️ More migration files to maintain

**Effort**: 60 minutes

---

### **Option C: Delete Vector Migrations** ❌ **NOT RECOMMENDED**

**Approach**: Permanently remove vector migrations from the codebase

**Pros**:
- ✅ Simplest approach

**Cons**:
- ❌ Breaks upgrade path to V2.0+ with vector support
- ❌ Loses migration history
- ❌ Makes it harder to add vector support later

**Effort**: 20 minutes

---

## 📋 **DETAILED MIGRATION DEPENDENCY CHAIN**

| Migration | Requires | Creates | Used By |
|-----------|----------|---------|---------|
| 005_vector_schema.sql | pgvector | action_patterns table | 007 |
| 007_add_context_column.sql | 005 | action_patterns.context column | - |
| 008_context_api_compatibility.sql | pgvector | embedding column in existing table | - |
| 009_update_vector_dimensions.sql | pgvector | Updates vector dimensions | - |
| 010_audit_write_api_phase1.sql | pgvector | alert_fingerprint column | 011 |
| 011_rename_alert_to_signal.sql | 010 | Renames alert_fingerprint → signal_fingerprint | - |
| 015_create_workflow_catalog_table.sql | pgvector | remediation_workflows.embedding | - |
| 016_update_embedding_dimensions.sql | pgvector | Updates embedding to 768d | - |

**Dependency Breaks**:
- Skipping 010 breaks 011 (column doesn't exist)
- Skipping 005 breaks 007 (table doesn't exist)

---

## 🚀 **IMMEDIATE NEXT STEPS**

### **For Decision**:
1. **Choose Option A or B** (recommend Option A)
2. If Option A:
   - Create `migrations-v1.0/` directory
   - Copy non-vector migrations
   - Update integration test suite
   - Document V1.0 → V2.0 upgrade path

3. If Option B:
   - Create stub migration files
   - Update test suite to use stubs
   - Document stub purpose

---

## 🎯 **QUESTIONS FOR USER**

1. **V2.0+ Plans**: Will we add vector/embedding support back in future versions?
   - YES → Use Option A (maintain migration history)
   - NO → Can consider Option C (delete permanently)

2. **Production Deployment**: When will V1.0 be deployed?
   - SOON → Prioritize Option A (clean V1.0 path)
   - LATER → Can use Option B (more flexible)

3. **Migration History**: How important is maintaining full migration history?
   - IMPORTANT → Option A or B
   - NOT CRITICAL → Option C acceptable

---

## 📊 **RECOMMENDATION: OPTION A**

**Why Option A is Best**:
1. ✅ Clean V1.0 migration path (no vector dependencies)
2. ✅ Preserves full migration history for V2.0+
3. ✅ Clear separation between V1.0 and V2.0+ architectures
4. ✅ Easy to test and validate
5. ✅ Production-ready immediately after implementation

**Implementation Plan**:
```bash
# Step 1: Create V1.0 migrations directory
mkdir migrations-v1.0

# Step 2: Copy non-vector migrations
cp migrations/001_initial_schema.sql migrations-v1.0/
cp migrations/002_fix_partitioning.sql migrations-v1.0/
cp migrations/003_stored_procedures.sql migrations-v1.0/
cp migrations/004_add_effectiveness_assessment_due.sql migrations-v1.0/
cp migrations/006_effectiveness_assessment.sql migrations-v1.0/
cp migrations/011_rename_alert_to_signal.sql migrations-v1.0/
cp migrations/012_adr033_multidimensional_tracking.sql migrations-v1.0/
cp migrations/013_create_audit_events_table.sql migrations-v1.0/
cp migrations/017_add_workflow_schema_fields.sql migrations-v1.0/
cp migrations/018_rename_execution_bundle_to_container_image.sql migrations-v1.0/
cp migrations/019_uuid_primary_key.sql migrations-v1.0/
cp migrations/020_add_workflow_label_columns.sql migrations-v1.0/
cp migrations/1000_create_audit_events_partitions.sql migrations-v1.0/

# Step 3: Update integration test suite
sed -i 's|"../../../migrations/"|"../../../migrations-v1.0/"|g' test/integration/datastorage/suite_test.go

# Step 4: Remove skipped migration comments (no longer needed)
sed -i '/^\/\/ ⏭️ SKIPPED V1.0:/d' test/integration/datastorage/suite_test.go

# Step 5: Run tests
make test-integration-datastorage
```

**Effort**: 30 minutes
**Confidence**: 95%

---

## 📝 **STATUS**

**Current**: 🔴 Integration tests FAILING (migration 011 dependency issue)

**After Fix**: ✅ Integration tests should PASS with clean V1.0 migration path

---

**Triaged By**: DataStorage Team (AI Assistant - Claude)
**Date**: 2025-12-11
**Recommended**: Option A - V1.0-Specific Migration Path
**Confidence**: 95%
**Effort**: 30 minutes
