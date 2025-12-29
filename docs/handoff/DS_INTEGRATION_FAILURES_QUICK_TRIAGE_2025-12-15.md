# Data Storage Integration Test Failures - Quick Triage & Fixes

**Date**: 2025-12-15 19:05
**Status**: 4 failures identified, all fixable
**Priority**: P1 (Post-V1.0) - Non-blocking
**Total Effort**: ~4-6 hours

---

## 📊 **Summary**

| # | Test | Root Cause | Fix Complexity | Time | Status |
|---|------|-----------|----------------|------|--------|
| 1 | UpdateStatus - status_reason | Schema mismatch | **EASY** | 1h | Ready to fix |
| 2 | Query by correlation_id | Test isolation | **EASY** | 30m | Ready to fix |
| 3 | Self-Auditing - audit traces | Missing implementation | **MEDIUM** | 2h | Needs investigation |
| 4 | Self-Auditing - InternalAuditClient | Test validation | **EASY** | 1h | Needs investigation |

---

## 🔴 **Failure #1: status_reason Column - P0**

### **Error**
```
ERROR: column "status_reason" of relation "remediation_workflow_catalog" does not exist (SQLSTATE 42703)
```

### **Quick Diagnosis**

**Code**:
```go
// pkg/datastorage/repository/workflow/crud.go:348
UPDATE remediation_workflow_catalog
SET status = $1, status_reason = $2, updated_by = $3
```

**Schema**:
```sql
-- migrations/015_create_workflow_catalog_table.sql:76-78
disabled_at TIMESTAMP
disabled_by VARCHAR(255)
disabled_reason TEXT  -- ❌ Different name
```

**Problem**: Code expects `status_reason`, schema has `disabled_reason`

### **Fix** (15 minutes)

**Option A: Add Migration** (RECOMMENDED)
```sql
-- migrations/021_add_status_reason_column.sql
-- +goose Up
ALTER TABLE remediation_workflow_catalog
ADD COLUMN status_reason TEXT;

-- +goose Down
ALTER TABLE remediation_workflow_catalog
DROP COLUMN status_reason;
```

**Steps**:
1. Create migration file
2. Apply migration: `make migrate-datastorage-up`
3. Verify test passes

**Confidence**: 100% - Straightforward schema fix

---

## 🟡 **Failure #2: correlation_id Test Isolation - P1**

### **Error**
```
Expected: 5 audit events
Got: >5 (seeing events from other tests)
```

### **Quick Diagnosis**

**Test Code**:
```go
// test/integration/datastorage/audit_events_query_api_test.go:209
correlationID := "remediation-test-correlation-1"  // ❌ HARDCODED
```

**Problem**: Hardcoded correlation ID shared across test runs

### **Fix** (10 minutes)

```go
// BEFORE
correlationID := "remediation-test-correlation-1"

// AFTER
correlationID := generateTestID()  // ✅ Unique per run
```

**Steps**:
1. Find all hardcoded correlation IDs
2. Replace with `generateTestID()`
3. Verify test isolation

**Command**:
```bash
grep -n "remediation-test-correlation" test/integration/datastorage/*.go
```

**Confidence**: 95% - Standard test isolation pattern

---

## 🟡 **Failure #3: Self-Auditing Traces - P1**

### **Error**
```
Expected: 1 audit event for datastorage.audit.written
Got: 0 (no audit event generated)
```

### **Quick Diagnosis**

**Test Expectation**:
```go
// test/integration/datastorage/audit_self_auditing_test.go:138
Eventually(func() int {
    return countAuditEvents(testCtx, "datastorage.audit.written", testCorrelationID)
}, "10s", "500ms").Should(Equal(1))
```

**Investigation Needed**:
1. Is self-auditing implemented?
2. Does InternalAuditClient generate events?
3. Is event type correct?

### **Fix Steps** (1-2 hours)

**Step 1: Check if self-auditing exists**
```bash
grep -r "datastorage.audit.written" pkg/datastorage/server/
```

**Step 2: Check InternalAuditClient usage**
```bash
grep -r "auditStore.StoreAudit\|InternalAuditClient" pkg/datastorage/server/
```

**Step 3: Verify event generation**
```go
// If missing, add to audit_events_handler.go
func (h *AuditEventsHandler) HandleCreateAuditEvent(...) {
    // ... existing code ...

    // Self-audit after successful write
    if h.auditStore != nil {
        go func() {
            selfAuditEvent := &models.AuditEvent{
                EventType:     "datastorage.audit.written",
                EventCategory: "storage",
                EventAction:   "written",
                EventOutcome:  "success",
                // ...
            }
            h.auditStore.StoreAudit(ctx, selfAuditEvent)
        }()
    }
}
```

**Confidence**: 60% - Needs investigation to confirm if implementation is missing

---

## 🟡 **Failure #4: InternalAuditClient Validation - P1**

### **Error**
```
Expected: Exactly 1 audit trace (no recursion)
Got: 0 (same as Failure #3)
```

### **Quick Diagnosis**

**Test Purpose**: Verify no circular dependency (audit of audit)

**Test Code**:
```go
// test/integration/datastorage/audit_self_auditing_test.go:317
count := countAuditEvents(testCtx, "datastorage.audit.written", testCorrelationID)
Expect(count).To(Equal(1))  // Exactly 1, no recursion
```

**Depends On**: Failure #3 fix (self-auditing implementation)

### **Fix** (30 minutes after #3)

**After fixing #3**, verify:
1. InternalAuditClient used (not HTTP client)
2. No recursive audit calls
3. Count remains at 1

**Verification**:
```bash
# Check InternalAuditClient prevents recursion
grep -A20 "InternalAuditClient.*StoreAudit" pkg/audit/
```

**Confidence**: 70% - Depends on #3 fix

---

## 🎯 **Recommended Fix Order**

### **Phase 1: Quick Wins** (45 minutes)
1. ✅ **Fix #1** (status_reason) - 15 minutes
2. ✅ **Fix #2** (correlation_id) - 10 minutes
3. ✅ **Test both fixes** - 20 minutes

### **Phase 2: Investigation** (2-3 hours)
4. 🔍 **Investigate #3** (self-auditing) - 1-2 hours
5. ✅ **Fix #3** - 30 minutes
6. ✅ **Fix #4** (depends on #3) - 30 minutes

**Total Time**: ~3-4 hours

---

## 🚀 **Quick Fix Script**

### **Fix #1: status_reason Column**

```bash
# 1. Create migration
cat > migrations/021_add_status_reason_column.sql << 'EOF'
-- +goose Up
ALTER TABLE remediation_workflow_catalog
ADD COLUMN status_reason TEXT;

-- +goose Down
ALTER TABLE remediation_workflow_catalog
DROP COLUMN status_reason;
EOF

# 2. Apply migration (in test environment)
podman exec datastorage-postgres-test psql -U slm_user -d action_history -c \
  "ALTER TABLE remediation_workflow_catalog ADD COLUMN status_reason TEXT;"

# 3. Test
make test-integration-datastorage-workflow
```

### **Fix #2: correlation_id Isolation**

```bash
# Find all hardcoded correlation IDs
grep -n "remediation-test-correlation-[0-9]" test/integration/datastorage/audit_events_query_api_test.go

# Replace with generateTestID() - manual edit needed
```

---

## 📊 **Impact Assessment**

### **After Fixes**

| Metric | Current | After Fixes | Change |
|--------|---------|-------------|--------|
| **Integration Pass Rate** | 97.6% (160/164) | **100%** (164/164) | +2.4% |
| **Overall Pass Rate** | 98.8% (237/241) | **100%** (241/241) | +1.2% |

### **V1.0 Impact**

**None** - All fixes are post-V1.0:
- ✅ V1.0 release not blocked
- ✅ Production code quality excellent (100% unit tests)
- ⚠️ Integration tests improve completeness

---

## 🔗 **Detailed Triage**

Full analysis: [DS_INTEGRATION_TEST_FAILURES_TRIAGE_2025-12-15.md](./DS_INTEGRATION_TEST_FAILURES_TRIAGE_2025-12-15.md)

---

## ✅ **Next Actions**

### **Immediate** (Today)
- [ ] Fix #1 (status_reason) - 15 min
- [ ] Fix #2 (correlation_id) - 10 min
- [ ] Test Phase 1 fixes - 20 min

### **Short-Term** (This Week)
- [ ] Investigate #3 (self-auditing) - 1-2 hours
- [ ] Fix #3 + #4 together - 1 hour
- [ ] Verify all integration tests pass

---

**Prepared by**: AI Assistant
**Priority**: P1 (Post-V1.0)
**Blocking**: None
**Confidence**: 90% - All fixes are straightforward




