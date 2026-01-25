# Notification Status Update Fix - Progress Report

**Date**: January 22, 2026  
**Status**: ✅ **Regression Fixed**, 🔧 **New Test Failure**  
**Test Results**: 116/117 passing (+1 from regression fix, -1 new failure)

---

## ✅ PRIMARY REGRESSION FIXED

**Test**: `Controller Retry Logic (BR-NOT-054) → should stop retrying after first success`  
**Status**: ✅ **PASSING**  
**Fix Applied**: Refined deduplication logic in `pkg/notification/status/manager.go`

### Fix Details

**Problem**: Deduplication logic was too aggressive, rejecting legitimate failed attempts with the same attempt number due to API propagation lag.

**Root Cause**:
```
Attempt 1: persisted=0, inFlight=1, total=1 → Assigned Attempt#=1 (failed)
Attempt 2: persisted=0, inFlight=1, total=1 → Assigned Attempt#=1 (failed)
                                              ↑ Same attempt# before status propagated

Deduplication logic saw:
- Same channel ✓
- Same attempt number ✓  ← TOO STRICT
- Same status ("failed") ✓
- Timestamp within 1 second ✓

Result: Attempt 2 incorrectly deduplicated as duplicate of Attempt 1
```

**Solution**: Removed attempt number from deduplication check, now only deduplicates truly identical attempts (same error message):

```go
// OLD (TOO STRICT):
if existing.Channel == attempt.Channel &&
    existing.Attempt == attempt.Attempt &&  ← Caused false positives
    existing.Status == attempt.Status &&
    abs(existing.Timestamp.Time.Sub(attempt.Timestamp.Time)) < time.Second

// NEW (CORRECT):
if existing.Channel == attempt.Channel &&
    existing.Status == attempt.Status &&
    existing.Error == attempt.Error &&  ← Only dedup truly identical attempts
    abs(existing.Timestamp.Time.Sub(attempt.Timestamp.Time)) < time.Second
```

**Verification**: Test now records all 3 attempts correctly (fail, fail, success).

---

## 🔧 NEW TEST FAILURE (Under Investigation)

**Test**: `Controller Audit Event Emission → should emit notification.message.failed when Slack delivery fails`  
**Status**: ❌ **TIMING OUT (30s)**  
**Location**: `test/integration/notification/controller_audit_emission_test.go:487`

### Observations

**Good News**:
- ✅ Audit event `notification.message.failed` IS being emitted correctly
- ✅ Delivery attempt IS being recorded in status
- ✅ Status persistence working correctly

**Issue**:
- Test times out after 30 seconds
- Likely a test assertion timing issue, not a business logic bug

### Evidence from Must-Gather

```
Line 34584: audit.audit-store - StoreAudit called
  event_type: "notification.message.failed"
  correlation_id: "audit-failed-1769119294146414000"
  
Line 34585: audit.audit-store - Validation passed
Line 34586: audit.audit-store - Event buffered successfully
  total_buffered: 23

Line 34610: status-manager - DD-STATUS-001: API reader refetch complete
  deliveryAttemptsBeforeUpdate: 0
  newAttemptsToAdd: 1

Line 34612: totalAttempts: 1, deliveryAttemptCount: 1 ✅ CORRECT
```

**Hypothesis**: The test might be using an `Eventually` assertion that's waiting for a condition that's already been met, or the test setup changed after the deduplication fix.

### Next Steps

1. Read the test code to understand exact assertion logic
2. Check if test timeout value needs adjustment
3. Verify test isn't checking for deprecated behavior

---

## 📊 Summary

| Metric | Before Fix | After Fix | Delta |
|---|---|---|---|
| **Tests Passing** | 116/117 | 116/117 | ±0 |
| **Regression Fixed** | ❌ | ✅ | +1 |
| **New Issue** | - | ❌ | -1 |
| **Net Progress** | - | - | **Even** |

### Key Achievements

1. ✅ **Root cause identified**: API propagation lag causing duplicate attempt numbers
2. ✅ **Optimistic locking confirmed working**: `RetryOnConflict` already in place
3. ✅ **Deduplication logic fixed**: Now only rejects truly identical attempts
4. ✅ **Regression test passing**: All 3 attempts recorded correctly

### Outstanding Work

1. 🔧 Investigate audit emission test timeout
2. 📝 Update `DD-PERF-001` with deduplication fix details
3. 🧪 Add concurrent status update test (as per triage doc)

---

## 🔗 Related Documents

- **Root Cause Analysis**: `NOTIFICATION_STATUS_RACE_REGRESSION_JAN22_2026.md`
- **Comprehensive Triage**: `COMPREHENSIVE_TEST_TRIAGE_JAN_22_2026.md`
- **Design Decision**: `DD-PERF-001` (atomic status updates)

---

**Next Action**: Investigate audit emission test failure, likely a test timing issue.
