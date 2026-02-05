# HAPI Audit Timeout Alignment Analysis
**Date:** January 31, 2026  
**Author:** AI Assistant  
**Status:** Evidence-Based Recommendation

---

## Executive Summary

User requested triage of HAPI Python audit flush timeout (5s → 10s) to ensure alignment with Go integration test patterns across the same tier (INT). Analysis reveals **partial alignment** - flush timeout is correct, but **polling timeout should increase from 10s to 30s** to match Go services.

---

## Evidence: Go Integration Test Timeout Patterns

### AIAnalysis Integration Tests (Most Similar to HAPI)

**File:** `test/integration/aianalysis/audit_flow_integration_test.go`

**Pattern (NT Pattern - "Flush on Each Retry"):**
```go
Eventually(func() bool {
    _ = auditStore.Flush(ctx)  // Flush on EACH retry
    
    params := ogenclient.QueryAuditEventsParams{
        CorrelationID: ogenclient.NewOptString(correlationID),
        EventCategory: ogenclient.NewOptString(eventCategory),
    }
    resp, err := dsClient.QueryAuditEvents(ctx, params)
    // ... error handling ...
    return len(allEvents) > 0
}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
    "Controller should generate complete audit trail")
```

**Timeouts:**
- **Polling timeout:** `30*time.Second` (30 seconds)
- **Poll interval:** `500*time.Millisecond` (0.5 seconds)
- **Flush timeout:** Implicit (controlled by context, typically 2-10s)

**Rationale (from comment):**
> "Ensures events buffered AFTER previous query are written to DataStorage"

**Why flush on each retry?**
- Controller may still be running and buffering events during polling
- Ensures eventual consistency with long-running operations

---

### Gateway Integration Tests

**File:** `test/integration/gateway/suite_test.go`

**Flush Context Timeout:**
```go
flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
```

**Timeouts:**
- **Flush timeout:** `10*time.Second`
- **Eventually polling:** `5-10*time.Second` with `500*time.Millisecond` interval

---

### AuthWebhook Integration Tests

**File:** `test/integration/authwebhook/suite_test.go`

**Flush Context Timeout:**
```go
flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
```

**Timeouts:**
- **Flush timeout:** `10*time.Second`

---

## Current HAPI Python Implementation

**File:** `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`

### Current Pattern (Single Flush, Then Poll)

```python
def query_audit_events_with_retry(
    data_storage_url: str,
    correlation_id: str,
    min_expected_events: int = 1,
    timeout_seconds: int = 10,  # ⚠️ MISALIGNMENT: Should be 30s
    poll_interval: float = 0.5,  # ✅ ALIGNED
    audit_store=None
) -> List[AuditEvent]:
    # Flush ONCE before polling
    success = audit_store.flush(timeout=10.0)  # ✅ ALIGNED with Gateway/AuthWebhook
    if not success:
        raise AssertionError("Audit flush timeout...")
    
    start_time = time.time()
    attempts = 0
    
    while time.time() - start_time < timeout_seconds:
        attempts += 1
        events = query_audit_events(data_storage_url, correlation_id, timeout=5)
        
        if len(events) >= min_expected_events:
            return events
        
        time.sleep(poll_interval)
```

**Timeouts:**
- **Polling timeout:** `10*time.Second` ⚠️ **MISALIGNED** (should be 30s)
- **Poll interval:** `0.5` seconds ✅ **ALIGNED**
- **Flush timeout:** `10.0` seconds ✅ **ALIGNED**

---

## Alignment Analysis

| Timeout Type | Go (AIAnalysis) | Go (Gateway/AuthWebhook) | HAPI (Python) | Status |
|--------------|-----------------|--------------------------|---------------|--------|
| **Polling timeout** | `30s` | `5-10s` | `10s` | ⚠️ **MISALIGNED** |
| **Poll interval** | `500ms` | `500ms-1s` | `500ms` | ✅ **ALIGNED** |
| **Flush timeout** | `2-10s` (implicit) | `10s` | `10s` | ✅ **ALIGNED** |

---

## Architectural Differences: Why Go Pattern Differs

### Go AIAnalysis Pattern
- **Controller-based:** `controller.Reconcile()` is called, which triggers async operations
- **Concurrent buffering:** Controller may buffer events WHILE we're polling
- **Solution:** Flush on EACH retry inside `Eventually()` to catch new events

### HAPI Python Pattern
- **Direct function call:** `analyze_incident()` completes BEFORE we start polling
- **Sequential execution:** All events are buffered by the time we call `flush()`
- **Solution:** Single flush + polling is sufficient (no concurrent buffering)

**Conclusion:** HAPI's "single flush, then poll" pattern is architecturally correct for direct function calls, but polling timeout should still match Go for consistency.

---

## Recommendations

### ✅ REQUIRED: Align Polling Timeout

**Change:**
```python
def query_audit_events_with_retry(
    data_storage_url: str,
    correlation_id: str,
    min_expected_events: int = 1,
    timeout_seconds: int = 30,  # ✅ Changed from 10s to 30s (matches Go AIAnalysis)
    poll_interval: float = 0.5,
    audit_store=None
) -> List[AuditEvent]:
```

**Rationale:**
1. **Consistency:** Matches Go AIAnalysis INT pattern (same tier, same audit validation)
2. **CI/CD resilience:** 30s provides buffer for slower CI/CD environments (user's point)
3. **Low risk:** Only affects test timeout, not production behavior
4. **Best practice:** `Eventually()` pattern with generous timeout + fast polling

**Impact:** +0-3 tests (may help with timing-sensitive failures)

**Confidence:** 95%

---

### ✅ KEEP: Current Flush Timeout

**Current:**
```python
success = audit_store.flush(timeout=10.0)
```

**Rationale:**
- Already aligned with Gateway/AuthWebhook (`10*time.Second`)
- Sufficient for parallel test execution (4 workers)
- Matches production recommendation

**Confidence:** 100%

---

### 🤔 OPTIONAL: Add Comment Explaining Pattern Difference

**Suggested comment:**
```python
def query_audit_events_with_retry(...):
    """
    Query Data Storage for audit events with explicit flush support.
    
    Pattern: flush() → poll with Eventually() → assert
    
    Difference from Go AIAnalysis:
    - Go: Flush on EACH retry (controller may buffer during polling)
    - Python: Flush ONCE (analyze_incident() completes before polling)
    
    Timeout alignment with Go integration tests:
    - Polling: 30s (matches AIAnalysis INT)
    - Poll interval: 500ms (matches Go Eventually pattern)
    - Flush: 10s (matches Gateway/AuthWebhook)
    """
```

---

## Implementation Plan

1. **Update `query_audit_events_with_retry` default timeout:**
   - Change `timeout_seconds: int = 10` → `timeout_seconds: int = 30`
   - Add comment explaining alignment rationale

2. **Update test calls (if any hardcoded 10s values):**
   - Search for `query_audit_events_with_retry(..., timeout_seconds=10)`
   - Update to `timeout_seconds=30` or rely on default

3. **Document in test file header:**
   - Add timeout alignment note to file docstring

---

## Expected Results

**Before:**
- Polling timeout: 10s
- May fail in slow CI/CD environments
- Inconsistent with Go AIAnalysis pattern

**After:**
- Polling timeout: 30s
- Resilient to CI/CD hardware constraints
- Aligned with Go AIAnalysis INT pattern
- Maintains fast poll interval (500ms) for responsiveness

---

## References

1. **Go AIAnalysis INT:** `test/integration/aianalysis/audit_flow_integration_test.go:218`
2. **Go Gateway INT:** `test/integration/gateway/suite_test.go:303`
3. **Go AuthWebhook INT:** `test/integration/authwebhook/suite_test.go:303`
4. **HAPI Python:** `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py:139`
5. **User Guidance:** "triage the timeout in go tests of the same tier to ensure alignment. When running in CI/CD we can expect longer times due to HW constrains..."

---

## Confidence Assessment

**Overall Confidence:** 95%

**Evidence Quality:** HIGH
- Direct comparison with Go INT tests (same tier)
- Authoritative source code inspection
- User-requested alignment analysis

**Risk Assessment:** LOW
- Only affects test timeout (not production code)
- Increases resilience (30s > 10s)
- No breaking changes

**Implementation Complexity:** TRIVIAL
- Single parameter change + optional comment
- No API changes, no dependencies affected
