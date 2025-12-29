# WE-BUG-002: Audit Event Type Mismatch

**Date**: 2025-12-27
**Severity**: HIGH
**Status**: IDENTIFIED - FIX READY
**Component**: WorkflowExecution Controller + Integration Tests

---

## 🐛 Bug Summary

**Problem**: WorkflowExecution controller creates audit events with `event_type = "workflowexecution.workflow.started"`, but integration tests query for `event_type = "workflow.started"`, causing 100% test failure rate.

**Impact**: 2 audit flow integration tests fail 100% of the time despite:
- ✅ Controller creating audit events correctly
- ✅ Audit store writing events to DataStorage correctly
- ✅ HTTP queries succeeding (status 200)
- ❌ **Query parameters don't match stored data**

---

## 📊 Test Results

```
Ran 2 audit flow integration tests: 0 Passed | 2 Failed
- ❌ "should emit 'workflow.started' audit event to Data Storage"
- ❌ "should track workflow lifecycle through audit events"
Both timeout after 20 seconds (Expected: >=1 events, Got: 0)
```

---

## 🔍 Root Cause Analysis

### The Mismatch

**Controller Code** (`internal/controller/workflowexecution/audit.go:100`):
```go
audit.SetEventType(event, "workflowexecution."+action)
```

**Controller Call** (`workflowexecution_controller.go:340`):
```go
r.recordAuditEventWithCondition(ctx, wfe, "workflow.started", "success")
```

**Result**: `event_type = "workflowexecution.workflow.started"` ✅ (stored in database)

**Test Query** (`audit_flow_integration_test.go:153`):
```go
eventType := "workflow.started"  // NO PREFIX!
```

**Result**: Queries for `event_type = "workflow.started"` ❌ (doesn't match)

---

## 🕵️ Investigation Timeline

### Evidence 1: Audit Events ARE Being Created
```
2025-12-27T19:06:01 DEBUG Audit event recorded
  {"action": "workflow.started", "wfe": "audit-test-wfe-1766880361", "outcome": "success"}
```

### Evidence 2: Events ARE Being Written to DataStorage
```
2025-12-27T19:06:02 DEBUG ✅ Wrote audit batch
  {"batch_size": 1, "attempt": 1, "write_duration": "13.156166ms"}
```

### Evidence 3: Test Queries Succeed (but return 0 results)
```
- No HTTP errors logged
- No "Failed to query audit events" messages
- No "Audit query returned status != 200" messages
- Queries succeed but return 0 events
```

### Evidence 4: The Query Mismatch
**Stored in database**:
```json
{
  "event_type": "workflowexecution.workflow.started",
  "event_category": "workflow",
  "event_action": "workflow.started",
  "correlation_id": "audit-test-wfe-1766880361"
}
```

**Test queries for**:
```json
{
  "event_type": "workflow.started",  // ❌ MISMATCH!
  "event_category": "workflow",      // ✅ Matches
  "correlation_id": "audit-test-wfe-1766880361"  // ✅ Matches
}
```

---

## 💡 Why This Wasn't Caught Earlier

1. **No E2E validation**: E2E tests don't query DataStorage directly
2. **Manual testing**: Likely didn't use exact query parameters
3. **Recent refactoring**: Audit infrastructure recently changed
4. **Buffer flush timing focus**: Investigation initially focused on timing, not query correctness

---

## ✅ Proposed Fix

### Option A: Remove Prefix from Controller (Recommended)

**Rationale**: The action already includes service context (`"workflow.started"`), duplicating with `"workflowexecution."` prefix is redundant.

**Changes**:
```diff
--- a/internal/controller/workflowexecution/audit.go
+++ b/internal/controller/workflowexecution/audit.go
@@ -97,7 +97,9 @@ func (r *WorkflowExecutionReconciler) RecordAuditEvent(

 	// Build audit event per ADR-034 schema
 	event := audit.NewAuditEventRequest()
 	event.Version = "1.0"
-	audit.SetEventType(event, "workflowexecution."+action)
+	// Event type = action (e.g., "workflow.started")
+	// Service context is in event_category and actor fields
+	audit.SetEventType(event, action)
 	audit.SetEventCategory(event, "workflow")
 	audit.SetEventAction(event, action)
```

**Pros**:
- ✅ Matches test expectations
- ✅ Simpler event types
- ✅ Event category already provides service context
- ✅ Consistent with other services (if they follow same pattern)

**Cons**:
- ⚠️ Changes stored event_type format
- ⚠️ May affect existing queries (if any exist in production)

---

### Option B: Update Test to Match Controller (Alternative)

**Rationale**: Keep controller behavior as-is, fix test queries.

**Changes**:
```diff
--- a/test/integration/workflowexecution/audit_flow_integration_test.go
+++ b/test/integration/workflowexecution/audit_flow_integration_test.go
@@ -150,7 +150,7 @@ var _ = Describe("WorkflowExecution Audit Flow Integration Tests", func() {

 		By("4. Query Data Storage for 'workflow.started' audit event (SIDE EFFECT)")
 		// ✅ DD-API-001: Use OpenAPI client with type-safe parameters
-		eventType := "workflow.started"
+		eventType := "workflowexecution.workflow.started"  // Full event type with service prefix
 		eventCategory := "workflow"
 		var auditEvents []dsgen.AuditEvent
 		Eventually(func() int {
```

**Pros**:
- ✅ No controller changes
- ✅ Preserves current event_type format
- ✅ Minimal risk

**Cons**:
- ❌ More verbose event types
- ❌ Redundant prefix (service context already in category)

---

## 🎯 Recommendation

**Choose Option A** (Remove Prefix from Controller):
1. Simpler event types
2. Less redundancy
3. Test expectations are more intuitive
4. Event category already provides service context

**Validation**:
1. Apply fix
2. Run audit flow integration tests
3. Verify 2/2 tests pass
4. Run full integration suite (69 tests)
5. Verify no regressions

---

## 📋 Files to Modify

### Option A (Recommended):
1. `internal/controller/workflowexecution/audit.go` (Line 100)
   - Remove `"workflowexecution."+` prefix

### Option B (Alternative):
1. `test/integration/workflowexecution/audit_flow_integration_test.go` (Line 153)
   - Change `"workflow.started"` to `"workflowexecution.workflow.started"`
2. Same file (Line ~220) - Second test also queries for event_type
   - Update query for lifecycle events

---

## 🔄 After Fix

**Expected Test Results**:
```
✅ 69 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Validation**:
- Events written: `event_type = "workflow.started"`
- Test queries for: `event_type = "workflow.started"`
- ✅ Match!

---

## 📚 Related Documentation

- `WE_INTEGRATION_TESTS_STATUS_DEC_27_2025.md` - Test failures documented
- `DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md` - Initial (incorrect) diagnosis
- `DS_RESPONSE_AUDIT_BUFFER_FLUSH_TIMING_DEC_27_2025.md` - Buffer timing analysis

---

## 🎓 Lessons Learned

1. **Query parameter validation**: When tests fail with 0 results, verify query parameters match stored data
2. **Event type schemas**: Document expected event_type format explicitly
3. **Integration test coverage**: Validate actual stored data, not just logs
4. **Debugging methodology**: Check ALL parts of the path (creation → storage → query → match)

---

**Priority**: HIGH - Blocks audit flow validation, but controller business logic works correctly
**Estimated Fix Time**: 5 minutes + 3 minutes test validation
**Risk Level**: LOW - Simple string matching fix



