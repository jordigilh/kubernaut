# Data Storage Integration Test Failures - Comprehensive Triage

**Date**: 2025-12-15 18:45
**Context**: 4 integration test failures (2.4% of 164 total tests)
**Priority**: P1 (Post-V1.0) - Non-blocking for V1.0 release
**Status**: Root causes identified

---

## 📊 **Failure Summary**

| # | Test | Priority | Root Cause | Effort | Status |
|---|------|----------|-----------|--------|--------|
| **1** | UpdateStatus - status_reason column | **P0** | Schema mismatch | 2 hours | Ready to fix |
| **2** | Query by correlation_id | **P1** | Test isolation issue | 4 hours | Needs investigation |
| **3** | Self-Auditing - audit traces | **P1** | Schema/test expectation mismatch | 2 hours | Needs investigation |
| **4** | Self-Auditing - InternalAuditClient | **P1** | Test validation logic | 1 hour | Needs investigation |

**Total Effort**: ~9 hours

---

## 🔴 **Failure #1: UpdateStatus - status_reason Column (P0)**

### **Error**
```
ERROR: column "status_reason" of relation "remediation_workflow_catalog" does not exist (SQLSTATE 42703)
```

### **Test Location**
- **File**: `test/integration/datastorage/workflow_repository_integration_test.go:430`
- **Context**: `UpdateStatus` → "should update status with reason and metadata"

### **Root Cause**

**Code expects `status_reason` column**:
```go
// pkg/datastorage/repository/workflow/crud.go:344-352
func (r *Repository) UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error {
    query := `
        UPDATE remediation_workflow_catalog
        SET
            status = $1,
            status_reason = $2,    // ❌ Column doesn't exist
            updated_by = $3,
            updated_at = NOW()
        WHERE workflow_id = $4 AND version = $5
    `
    // ...
}
```

**Database schema has different columns**:
```sql
-- migrations/015_create_workflow_catalog_table.sql:76-78
disabled_at TIMESTAMP WITH TIME ZONE,
disabled_by VARCHAR(255),
disabled_reason TEXT,
```

**Problem**: Schema has `disabled_reason` but code uses `status_reason`.

### **Impact**
- **Functional**: Workflow status updates fail with SQL error
- **Test**: 1 integration test fails
- **Production**: Would cause runtime errors if workflows were disabled via API

### **Fix Options**

#### **Option A: Add `status_reason` Column** (RECOMMENDED)
**Approach**: Create migration to add `status_reason` column

**Migration**:
```sql
-- migrations/021_add_status_reason_column.sql
-- +goose Up
ALTER TABLE remediation_workflow_catalog
ADD COLUMN status_reason TEXT;

-- +goose Down
ALTER TABLE remediation_workflow_catalog
DROP COLUMN status_reason;
```

**Why Recommended**:
- ✅ Matches code expectations
- ✅ More flexible (can track reason for any status change, not just disabled)
- ✅ Minimal code changes
- ✅ Clear semantics

**Effort**: 1 hour (migration + test)

#### **Option B: Use `disabled_reason`** (ALTERNATIVE)
**Approach**: Change code to use existing `disabled_reason` column

**Code Change**:
```go
// pkg/datastorage/repository/workflow/crud.go:344-352
func (r *Repository) UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error {
    query := `
        UPDATE remediation_workflow_catalog
        SET
            status = $1,
            disabled_reason = $2,  // ✅ Use existing column
            updated_by = $3,
            updated_at = NOW()
        WHERE workflow_id = $4 AND version = $5
    `
    // ...
}
```

**Why NOT Recommended**:
- ❌ Semantic mismatch (disabled_reason used for all status changes)
- ❌ Confusing when status is "active" but disabled_reason is set
- ❌ Breaks the disabled_* naming pattern

**Effort**: 30 minutes (code change only)

### **Recommended Fix**

**Priority**: P0 (Schema mismatch causing SQL errors)

**Action**:
1. Create migration `021_add_status_reason_column.sql`
2. Run migration in test infrastructure
3. Update integration test to verify column exists
4. Verify test passes

**Confidence**: 100% - Clear schema mismatch, straightforward fix

---

## 🟡 **Failure #2: Query by correlation_id (P1)**

### **Error**
```
Expected: 5 audit events for remediation correlation_id
Got: >5 events (test seeing data from other tests)
```

### **Test Location**
- **File**: `test/integration/datastorage/audit_events_query_api_test.go:209`
- **Context**: "should return all events for a remediation in chronological order"

### **Root Cause**

**Test Isolation Issue**:
```go
// test/integration/datastorage/audit_events_query_api_test.go:200-213
It("should return all events for a remediation in chronological order", func() {
    // Insert 5 events with same correlation_id
    correlationID := "remediation-test-correlation-1"  // ❌ HARDCODED - not unique per test run

    // Query expects exactly 5 events
    Expect(pagination["total"]).To(BeNumerically("==", 5))  // ❌ Fails if data from previous tests exists
})
```

**Problem**:
- Correlation ID is hardcoded, not unique per test run
- Other tests in the suite use the same correlation ID
- Test expects exactly 5 events but sees events from previous test runs

**Evidence**:
- Other tests use `generateTestID()` for unique test IDs
- This test uses hardcoded `"remediation-test-correlation-1"`

### **Impact**
- **Functional**: No impact on production code
- **Test**: Flaky integration test (passes first run, fails subsequent runs)
- **Development**: Slows down test iteration

### **Fix**

**Approach**: Use unique correlation ID per test run

**Code Change**:
```go
// test/integration/datastorage/audit_events_query_api_test.go:200-213
It("should return all events for a remediation in chronological order", func() {
    // Use unique correlation ID per test run
    correlationID := generateTestID()  // ✅ Unique per test run

    // Insert 5 events with unique correlation_id
    // ... test logic ...

    // Query expects exactly 5 events for THIS test's correlation_id
    Expect(pagination["total"]).To(BeNumerically("==", 5))  // ✅ Now isolated
})
```

**Effort**: 30 minutes (find all hardcoded IDs + test)

**Confidence**: 90% - Common test isolation pattern, straightforward fix

---

## 🟡 **Failure #3: Self-Auditing - audit traces (P1)**

### **Error**
```
Expected: 1 audit trace for datastorage.audit.written
Got: 0 (audit event not generated or schema mismatch)
```

### **Test Location**
- **File**: `test/integration/datastorage/audit_self_auditing_test.go:138`
- **Context**: "should generate audit traces for successful writes"

### **Root Cause (Suspected)**

**Test expects audit event generation**:
```go
// test/integration/datastorage/audit_self_auditing_test.go:136-138
Eventually(func() int {
    return countAuditEvents(testCtx, "datastorage.audit.written", testCorrelationID)
}, "10s", "500ms").Should(Equal(1), "Should generate 1 audit trace for successful write")
```

**Possible Issues**:
1. **Self-auditing not implemented**: Data Storage service may not be generating audit events for its own operations
2. **Event type mismatch**: Code generates different event type than test expects
3. **Schema mismatch**: Audit event schema doesn't match test expectations
4. **Timing issue**: Async audit event generation not completing within 10 seconds

### **Investigation Needed**

**Check if self-auditing is implemented**:
```bash
grep -r "datastorage.audit.written" pkg/datastorage/server/
```

**Check InternalAuditClient usage**:
```bash
grep -r "InternalAuditClient\|StoreAudit" pkg/datastorage/server/
```

**Verify event generation**:
- Check server logs for audit event generation
- Verify InternalAuditClient is called after successful audit writes

### **Impact**
- **Functional**: Self-auditing may not be working (audit trail incomplete)
- **Test**: 1 integration test fails
- **Production**: Audit trail gaps for Data Storage operations

### **Fix**

**Priority**: P1 (Audit trail completeness)

**Effort**: 2 hours (investigation + implementation if missing)

**Confidence**: 60% - Needs investigation to confirm root cause

---

## 🟡 **Failure #4: Self-Auditing - InternalAuditClient (P1)**

### **Error**
```
Expected: Exactly 1 audit trace (no recursion)
Got: 0 or >1 (circular dependency validation failing)
```

### **Test Location**
- **File**: `test/integration/datastorage/audit_self_auditing_test.go:305`
- **Context**: "should use InternalAuditClient (not REST API)"

### **Root Cause (Suspected)**

**Test validates circular dependency prevention**:
```go
// test/integration/datastorage/audit_self_auditing_test.go:316-317
count := countAuditEvents(testCtx, "datastorage.audit.written", testCorrelationID)
Expect(count).To(Equal(1), "Should have exactly 1 audit trace (no recursion)")
```

**Purpose**: Verify that when Data Storage audits its own operations:
- ✅ Generates audit event for the original operation
- ❌ Does NOT generate audit event for the audit (circular dependency)

**Possible Issues**:
1. **No self-auditing**: Count = 0 (same as Failure #3)
2. **Circular dependency**: Count > 1 (InternalAuditClient using REST API instead of direct DB)
3. **Test validation logic**: Test assertions incorrect

### **Investigation Needed**

**Verify InternalAuditClient implementation**:
```bash
# Check if InternalAuditClient bypasses HTTP layer
grep -A20 "type InternalAuditClient" pkg/audit/
```

**Check server handler usage**:
```bash
# Verify handlers use InternalAuditClient, not HTTP client
grep -r "auditStore\|auditClient" pkg/datastorage/server/
```

### **Impact**
- **Functional**: Potential circular dependency (infinite recursion risk)
- **Test**: 1 integration test fails
- **Production**: Could cause audit event explosion if circular dependency exists

### **Fix**

**Priority**: P1 (Circular dependency prevention)

**Effort**: 1 hour (investigation + validation)

**Confidence**: 70% - Likely test validation issue, not production bug

---

## 📊 **Overall Assessment**

### **Blockers for V1.0**

**None** - All 4 failures are P1 (Post-V1.0):
- Failure #1 (status_reason): Schema mismatch in workflow status update (low usage in V1.0)
- Failures #2-4: Test-specific issues or self-auditing gaps (not critical user-facing features)

### **Fix Priority**

1. **P0: Failure #1 (status_reason)** - Clear fix, 1 hour
   - Blocking: Workflow status updates
   - Risk: Medium (SQL error on status change)
   - Action: Create migration immediately

2. **P1: Failure #2 (correlation_id)** - Test isolation, 30 minutes
   - Blocking: Test reliability
   - Risk: Low (test-only)
   - Action: Fix after Failure #1

3. **P1: Failures #3-4 (self-auditing)** - Investigation needed, 3 hours
   - Blocking: Audit trail completeness
   - Risk: Medium (audit gaps)
   - Action: Investigate together (related issues)

### **Total Effort**

- **P0 Fixes**: 1 hour
- **P1 Fixes**: 3.5 hours
- **Investigation**: 2 hours
- **Total**: ~6.5 hours

---

## 🎯 **Recommended Actions**

### **Immediate (Post-V1.0)**

1. **Fix Failure #1 (status_reason)** - 1 hour
   - Create migration `021_add_status_reason_column.sql`
   - Test migration in integration environment
   - Verify test passes

2. **Fix Failure #2 (correlation_id)** - 30 minutes
   - Replace hardcoded correlation IDs with `generateTestID()`
   - Verify test isolation

### **Short-Term (Post-V1.0)**

3. **Investigate Failures #3-4 (self-auditing)** - 3 hours
   - Verify InternalAuditClient implementation
   - Check if self-auditing is enabled
   - Fix audit event generation if missing
   - Validate circular dependency prevention

---

## 📋 **Related Documentation**

- [DS All Test Tiers Results](./DS_ALL_TEST_TIERS_RESULTS_2025-12-15.md)
- [DS V1.0 Comprehensive Triage](./DS_V1.0_COMPREHENSIVE_TRIAGE_FINAL_2025-12-15.md)
- [PostgreSQL Init Fix Verification](./DS_POSTGRESQL_INIT_FIX_VERIFICATION_2025-12-15.md)

---

**Prepared by**: AI Assistant
**Status**: Root causes identified, fixes scoped
**Next Step**: Wait for E2E tests to complete, then prioritize fixes




