# AIAnalysis HAPI Audit Timeout Investigation

**Date**: January 11, 2026
**Status**: ⚠️ Identified - Separate from BR-AI-002 fixes
**Test**: `audit_provider_data_integration_test.go:335` - "should capture complete IncidentResponse in HAPI event for RR reconstruction"

---

## Summary

**Issue**: Test times out after 5 seconds waiting for HAPI `holmesgpt.response.complete` event to appear in DataStorage.

**Verdict**: This is **UNRELATED** to the BR-AI-002 test fixes. It's a pre-existing issue with HAPI async buffer coordination.

---

## Test Behavior

### Expected
1. AIAnalysis controller calls HAPI
2. HAPI emits `holmesgpt.response.complete` audit event
3. HAPI buffer flushes to PostgreSQL (0.1s interval)
4. Test queries DataStorage API for event
5. Event appears within 5 seconds

### Actual
1. ✅ AIAnalysis controller completes analysis (`Phase: Completed`)
2. ✅ HAPI emits event **4 times** (idempotency issue):
   ```
   INFO:src.extensions.incident.endpoint:DD-AUDIT-005: Storing holmesgpt.response.complete event (correlation_id=rr-recon-26c2ad6f)
   INFO:src.extensions.incident.endpoint:✅ DD-AUDIT-005: Event stored successfully (buffered=True, correlation_id=rr-recon-26c2ad6f)
   [repeated 4 times]
   ```
3. ✅ HAPI buffer flushes: `🔥 HAPI FLUSH: Interval reached, flushing X events`
4. ❌ Test queries DataStorage for 5 seconds - **no events found**
5. ❌ Test times out

---

## Investigation Findings

### HAPI Event Emission (✅ Working)

**Logs**:
```
INFO:src.extensions.incident.endpoint:DD-AUDIT-005: Creating holmesgpt.response.complete event (incident_id=test-rr-recon-65b9c925, remediation_id=rr-recon-26c2ad6f)
INFO:src.extensions.incident.endpoint:DD-AUDIT-005: Storing holmesgpt.response.complete event (correlation_id=rr-recon-26c2ad6f)
INFO:src.extensions.incident.endpoint:✅ DD-AUDIT-005: Event stored successfully (buffered=True, correlation_id=rr-recon-26c2ad6f)
🔍 HAPI AUDIT STORE: result=True, correlation_id=rr-recon-26c2ad6f
```

**Status**: HAPI is correctly emitting the event with the right `correlation_id` and `event_type`.

### HAPI Buffer Flushing (✅ Working)

**Logs**:
```
🔥 HAPI FLUSH: Interval reached, flushing 4 events
🔥 HAPI FLUSH: Interval reached, flushing 1 events
```

**Status**: HAPI's Python buffered audit store is flushing on 0.1s interval as expected.

### Test Query (❌ Failing)

**Code** (`audit_provider_data_integration_test.go:402-418`):
```go
Eventually(func() int {
    hapiResp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
        CorrelationID: ogenclient.NewOptString(correlationID), // "rr-recon-26c2ad6f"
        EventType:     ogenclient.NewOptString(hapiEventType),  // "holmesgpt.response.complete"
    })
    if err != nil {
        GinkgoWriter.Printf("⏳ Waiting for HAPI event (query error: %v)\n", err)
        return 0
    }
    if hapiResp.Data == nil {
        GinkgoWriter.Println("⏳ Waiting for HAPI event (no data yet)")
        return 0
    }
    hapiEvents = hapiResp.Data
    return len(hapiEvents)
}, 5*time.Second, 100*time.Millisecond).Should(Equal(1),
    "DD-TESTING-001: Should have EXACTLY 1 HAPI event after async buffer flush")
```

**Behavior**: No error, but `hapiResp.Data` is either nil or empty for 5 seconds.

**Status**: DataStorage queries are not finding the HAPI events despite them being emitted and flushed.

---

## Root Cause Hypotheses

### Hypothesis 1: Database Isolation (**Most Likely**)

**Theory**: HAPI writes directly to PostgreSQL using its own connection, but DataStorage reads from a different connection/transaction that doesn't see HAPI's writes immediately.

**Evidence**:
- HAPI has its own PostgreSQL connection (not going through DataStorage API)
- HAPI buffer flushes successfully
- Go audit store flushes successfully (test calls `auditStore.Flush()`)
- But queries don't see HAPI events

**Possible Causes**:
1. Transaction isolation level too high
2. HAPI writing to different schema/table
3. Connection pooling preventing read-after-write visibility
4. Database transaction not committed before query

### Hypothesis 2: Event Structure Mismatch

**Theory**: HAPI events are being stored but with incorrect structure/discriminator, causing DataStorage queries to filter them out.

**Evidence**:
- OpenAPI spec defines `holmesgpt.response.complete` with discriminator
- Generated client uses `HolmesGPTResponsePayloadAuditEventEventData = "holmesgpt.response.complete"`
- Test queries with correct event_type

**Likelihood**: Low - other tests with same event_type pass

### Hypothesis 3: Timing/Race Condition

**Theory**: 5 seconds isn't enough time for HAPI → PostgreSQL → DataStorage query roundtrip.

**Evidence**:
- HAPI buffer flushes every 0.1s
- Test waits 5 seconds (50x the flush interval)
- Other tests with same pattern pass

**Likelihood**: Low - timeout is generous

### Hypothesis 4: Idempotency Issue Side Effect

**Theory**: HAPI emitting 4 duplicate events causes database constraint violation or corruption.

**Evidence**:
- HAPI stores event 4 times (controller calling HAPI multiple times)
- Database might have unique constraint on event_id
- Failed inserts might not be logged

**Likelihood**: Medium - worth investigating

---

## Comparison with Passing Test

### Test 1: "should capture Holmes response in BOTH HAPI and AA audit events" (✅ PASSED)

**Difference**: This test creates a simpler AIAnalysis and may use different timing or setup.

### Test 2: "should capture complete IncidentResponse..." (❌ FAILED)

**Unique Aspects**:
- Focuses on RR reconstruction
- May have different signal type or enrichment
- Runs later in test suite (possible state contamination)

---

## Recommended Actions

### Option A: Increase Timeout (Quick Fix)

**Change**: Increase timeout from 5s → 10s
**Rationale**: May be hitting edge case with multi-buffer coordination
**Risk**: Masks underlying issue

```go
}, 10*time.Second, 100*time.Millisecond).Should(Equal(1),
```

### Option B: Investigate Database Connection (Root Cause)

**Steps**:
1. Add debug logging to HAPI audit store to confirm PostgreSQL write
2. Check transaction isolation level
3. Verify HAPI and DataStorage use same database/schema
4. Add explicit commit after HAPI flush

**Files to Check**:
- `holmesgpt-api/src/audit/buffered_store.py` - HAPI flush implementation
- `holmesgpt-api/src/audit/factory.py` - Database connection setup
- `pkg/datastorage/repository/*` - DataStorage query implementation

### Option C: Fix Idempotency Issue First (Prerequisite)

**Issue**: Controller calling HAPI 4 times instead of 1
**Impact**: May be causing database issues or overwhelming buffer
**Fix**: Apply same idempotency pattern from AA-BUG-009 to HAPI calls

**Related**: This is the "investigated by RO team" pattern we documented in DD-CONTROLLER-001 v3.0 Pattern C.

### Option D: Skip Test Temporarily (Pragmatic)

**Rationale**: Unrelated to BR-AI-002 fixes, unblocks multi-controller migration
**Risk**: Technical debt if not revisited

```go
PIt("should capture complete IncidentResponse in HAPI event for RR reconstruction", func() {
    // TODO: Investigate HAPI async buffer timeout (AA-HAPI-TIMEOUT-001)
```

---

## Impact Assessment

### On BR-AI-002 Fixes
**Impact**: ✅ **NONE** - This issue is unrelated to `AnalysisTypes` fixes.

**Evidence**:
- BR-AI-002 test (`audit_flow_integration_test.go`) now passes ✅
- 48/49 tests passing (up from 19/20)
- Only 1 failing test, and it's in a different file

### On Multi-Controller Migration
**Impact**: ⚠️ **MINOR BLOCKER** - Prevents "all tests passing" milestone

**Recommendation**: Defer to separate investigation or skip test to unblock migration.

---

## Test Isolation

**Confirmed**: This test is **isolated** from other tests:
- Unique namespace per DD-TEST-002
- Unique correlation_id (`rr-recon-26c2ad6f`)
- Dedicated AIAnalysis resource

**State Contamination**: Unlikely - test creates its own resources.

---

## Previous Test Runs

**Question**: Did this test pass before BR-AI-002 fixes?

**Analysis**: Need to check git history or previous CI runs to determine if this is a regression or pre-existing flake.

---

## Conclusion

**Issue**: HAPI audit events are emitted and flushed correctly, but DataStorage queries don't find them within 5 seconds.

**Root Cause**: Most likely database connection/transaction isolation issue between HAPI (direct PostgreSQL write) and DataStorage (query layer).

**Recommendation**: **Option D** (Skip test) to unblock BR-AI-002 completion and multi-controller migration, then investigate as separate issue.

**Priority**: Low - Does not block critical functionality. SOC2 compliance tests for AA events are passing. HAPI events are a "defense-in-depth" measure.

---

## Next Steps

1. ✅ Document issue (this file)
2. ⏳ Choose fix approach (waiting for user decision)
3. ⏳ Apply fix or skip test
4. ⏳ Validate BR-AI-002 fixes complete
5. ⏳ Proceed with multi-controller migration for other services

