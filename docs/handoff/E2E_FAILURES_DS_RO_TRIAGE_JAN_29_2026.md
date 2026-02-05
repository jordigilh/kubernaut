# E2E Failures Triage: DataStorage & RemediationOrchestrator
**Date**: January 29, 2026  
**CI Run**: [21695363602](https://github.com/jordigilh/kubernaut/actions/runs/21695363602)  
**Status**: 🔴 2 services failing (DS, RO), 1 flaky

---

## Executive Summary

Both DataStorage and RemediationOrchestrator E2E failures are **audit query race conditions** caused by asynchronous event buffering/flushing behavior. The failures did NOT reproduce locally with 12 concurrent processes, but occur consistently in CI/CD with only 4 concurrent processes.

### Root Causes Identified
1. **DS Pagination Test**: Off-by-one event count (74/75) after 30s polling - audit buffer flush race
2. **RO Approval Test**: Webhook audit events intermittently return 0 results - eventual consistency issue
3. **Common Pattern**: All failures involve audit event queries immediately after event creation

---

## DataStorage E2E Failure

### Test Details
- **Test**: `Audit Events Query API > Pagination > should return correct subset with limit and offset`
- **File**: `test/e2e/datastorage/13_audit_query_api_test.go:646`
- **Failure**: Timeout after 30s - expected ≥75 events, found only 74

### Code Analysis

```go
// Line 607-610: Insert 75 events
for i := 0; i < 75; i++ {
    err := createTestAuditEvent(dataStorageURL, "gateway", "signal.received", correlationID)
    Expect(err).ToNot(HaveOccurred())
}

// Line 620-647: Poll for events (30s timeout)
Eventually(func() float64 {
    resp, err := HTTPClient.Get(fmt.Sprintf("%s?correlation_id=%s&limit=50&offset=0", baseURL, correlationID))
    // ... parse pagination.total ...
    return total
}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 75),
    "should have at least 75 events after write completes")
```

### Root Cause Analysis

**Primary Suspect**: Audit store buffered write path
- Events are written via HTTP POST to DataStorage `/v1/audit/events` endpoint
- DataStorage uses a buffered audit store (1s flush interval per `pkg/audit/store.go`)
- Query goes through HTTP GET to DataStorage `/v1/audit/events` endpoint (different path)
- **Race**: Query may execute before final buffer flush completes

**Evidence**:
- Test creates 75 events sequentially (not batched)
- Buffering strategy: 100 events OR 1s timer (whichever first)
- With 75 events: needs timer flush (1s delay)
- Query at 30s finds only 74 events = **1 event still in buffer**

**Why CI/CD ≠ Local**:
- Local: 12 processes, higher contention → longer event creation time → more natural delay before query
- CI/CD: 4 processes, lower contention → faster event creation → query races with final flush

### Proposed Fix Options

**Option A: Add explicit flush after event creation**
```go
// After line 610: Force flush before querying
By("Waiting for audit buffer flush (1s interval)")
time.Sleep(2 * time.Second) // 2x buffer interval for safety
```

**Option B: Use batch write API instead**
```go
// Replace sequential writes with batch
events := make([]AuditEvent, 75)
for i := 0; i < 75; i++ {
    events[i] = createAuditEventPayload(...)
}
err := createBatchAuditEvents(dataStorageURL, events) // Triggers immediate flush
```

**Option C: Increase timeout + better logging**
```go
Eventually(func() (float64, error) {
    resp, err := HTTPClient.Get(...)
    total := parsePaginationTotal(resp)
    if total < 75 {
        return total, fmt.Errorf("still waiting: %d/75 events", int(total))
    }
    return total, nil
}, 60*time.Second, 1*time.Second).Should(BeNumerically(">=", 75))
```

**Recommendation**: **Option A** (simplest, most reliable)

---

## RemediationOrchestrator E2E Failures

### Test 1: Audit Trail Emission
- **Test**: `BR-AUDIT-006: RAR Audit Trail E2E > E2E-RO-AUD006-001 > should emit complete audit trail for approval decision`
- **File**: `test/e2e/remediationorchestrator/approval_e2e_test.go:218`
- **Failure**: Timeout (120s) - expects [1 webhook, 1 orchestration] event, found [0, 1]

### Test 2: Audit Trail Persistence  
- **Test**: `BR-AUDIT-006: RAR Audit Trail E2E > E2E-RO-AUD006-003 > should query audit events after RAR CRD is deleted [BeforeEach]`
- **File**: `test/e2e/remediationorchestrator/approval_e2e_test.go:467`
- **Failure**: Same as Test 1 - BeforeEach setup times out querying for events

### Code Analysis

```go
// Line 190-219: Query for audit events
Eventually(func() (int, int) {
    // Query webhook events with all 3 filters (correlationID + EventCategory + EventType)
    webhookResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
        CorrelationID: dsgen.NewOptString(correlationID),
        EventCategory: dsgen.NewOptString("webhook"),
        EventType:     dsgen.NewOptString("webhook.remediationapprovalrequest.decided"),
        Limit:         dsgen.NewOptInt(100),
    })
    if err != nil {
        return 0, 0 // ERROR RETURNS 0 - hides error details!
    }
    webhookEvents = webhookResp.Data

    // Query orchestration events (also with all 3 filters)
    orchestrationResp, err := dsClient.QueryAuditEvents(...)
    if err != nil {
        return len(webhookEvents), 0 // ERROR RETURNS partial - hides error!
    }
    orchestrationApprovalEvents = orchestrationResp.Data
    
    return len(webhookEvents), len(orchestrationApprovalEvents)
}, e2eTimeout, e2eInterval).Should(Equal([2]int{1, 1}))
```

### Root Cause Analysis

**Primary Issue**: Error swallowing in Eventually block
- CI logs show: `"🔍 DEBUG: Webhook query returned 0 events"` → then `"returned 1 events"` → then back to `"returned 0 events"`
- Pattern suggests: Query sometimes returns 0, sometimes 1 - NOT eventual consistency, but **query errors**

**Evidence from Logs**:
```
🔍 DEBUG: Webhook query returned 0 events
🔍 DEBUG: Orchestration query returned 1 events
🔍 DEBUG: Returning counts: webhook=0, orchestration=1 (expecting [1, 1])

🔍 DEBUG: Webhook query returned 1 events
    [0] CorrelationID=e2e-rar-persist-1d463f35, EventType=webhook.remediationapprovalrequest.decided
🔍 DEBUG: Orchestration query returned 1 events
🔍 DEBUG: Returning counts: webhook=1, orchestration=1 (expecting [1, 1])

🔍 DEBUG: Webhook query returned 1 events
... (continues flipping between 0 and 1)
```

**Hypothesis**: Query errors are being swallowed
- When `err != nil`, function returns `0, 0` or `len(webhookEvents), 0`
- Eventually sees `0` and retries, but error details are lost
- Query might be timing out, hitting database contention, or hitting audit buffer race

**Why CI/CD ≠ Local** (Same as DS):
- Local: 12 processes create natural backpressure → events flush before query
- CI/CD: 4 processes → faster RAR approval → query races with AuthWebhook audit event creation

### Proposed Fix Options

**Option A: Expose errors in Eventually** (RECOMMENDED)
```go
Eventually(func() ([2]int, error) {
    webhookResp, err := dsClient.QueryAuditEvents(...)
    if err != nil {
        return [2]int{0, 0}, fmt.Errorf("webhook query failed: %w", err)
    }
    webhookEvents = webhookResp.Data

    orchestrationResp, err := dsClient.QueryAuditEvents(...)
    if err != nil {
        return [2]int{len(webhookEvents), 0}, fmt.Errorf("orchestration query failed: %w", err)
    }
    orchestrationApprovalEvents = orchestrationResp.Data
    
    counts := [2]int{len(webhookEvents), len(orchestrationApprovalEvents)}
    if counts != [2]int{1, 1} {
        return counts, fmt.Errorf("incomplete: webhook=%d, orchestration=%d", counts[0], counts[1])
    }
    return counts, nil
}, e2eTimeout, e2eInterval).Should(Equal([2]int{1, 1}))
```

**Option B: Add explicit wait after RAR approval**
```go
// After line 178: Force audit flush delay
Expect(k8sClient.Status().Update(ctx, testRAR)).To(Succeed())
GinkgoWriter.Printf("✅ E2E: Approved RAR %s\n", testRAR.Name)

By("Waiting for audit events to be flushed (2s buffer window)")
time.Sleep(2 * time.Second) // AuthWebhook + RO both write audit events

By("Querying DataStorage for RAR audit events")
```

**Option C: Increase Eventually timeout**
```go
// Change from 120s to 180s
Eventually(func() (int, int) {
    ...
}, 180*time.Second, 2*time.Second).Should(Equal([2]int{1, 1}))
```

**Recommendation**: **Option A + Option B**
- Option A: Expose errors for better diagnostics (will reveal if queries are actually failing)
- Option B: Add explicit 2s delay after RAR approval (aligns with DS buffering behavior)

---

## Additional Observations

### DS Flaky Test
- Summary shows `1 Flaked` test but wasn't detailed in failure output
- Likely in `14_audit_batch_write_api_test.go` (per previous logs showing line 195 failure)
- Related to same buffering issue

### Must-Gather Unavailable
- Both DS and RO show: `"❌ No Kind cluster found - tests failed before cluster creation"`
- **BUT** this is misleading - clusters were created and tests ran (150s for DS, 241s for RO)
- **Actual cause**: Cluster already deleted by `SynchronizedAfterSuite` before must-gather step
- **Fix needed**: Must-gather should run BEFORE cluster deletion in AfterSuite

---

## Recommended Action Plan

### Phase 1: Quick Wins (DataStorage)
1. ✅ Add 2s sleep after event creation in pagination test
2. ✅ Increase timeout to 60s with better error logging
3. ✅ Run DS E2E locally with 4 processes to reproduce

### Phase 2: RO Diagnostic Enhancement
1. ✅ Expose errors in Eventually blocks (Option A)
2. ✅ Add 2s delay after RAR approval (Option B)
3. ✅ Increase query timeout to 180s
4. ✅ Run RO E2E locally with 4 processes to reproduce

### Phase 3: Systemic Fix (if Phase 1/2 insufficient)
1. Investigate audit store buffering strategy
2. Consider adding "flush" endpoint for E2E tests
3. Add test-only synchronous write mode

---

## Files to Modify

### DataStorage
- `test/e2e/datastorage/13_audit_query_api_test.go` (line 610-647)

### RemediationOrchestrator
- `test/e2e/remediationorchestrator/approval_e2e_test.go` (line 178-219, line 467)

### Must-Gather Fix
- `test/e2e/datastorage/datastorage_e2e_suite_test.go` (AfterSuite ordering)
- `test/e2e/remediationorchestrator/suite_test.go` (AfterSuite ordering)

---

## Next Steps

1. Implement Phase 1 fixes for DS pagination test
2. Implement Phase 2 fixes for RO approval test  
3. Push and validate in CI/CD
4. If still failing, proceed to Phase 3 (systemic audit buffering fix)

**User Approval Required**: Which phase should I implement first? (Recommend: Phase 1 + Phase 2 in parallel)
