# Signal Processing DD-TESTING-001 Violations Triage

**Service**: Signal Processing (SP)
**File**: `test/integration/signalprocessing/audit_integration_test.go`
**Date**: 2026-01-04
**Status**: 3 violations identified, fixes required

---

## 📊 **Violation Summary**

| Line(s) | Violation Type | Severity | Current Code | Required Fix |
|---|---|---|---|---|
| 645 | Non-deterministic count | P1 | `BeNumerically(">=", 4)` | `Equal(4)` with event type counting |
| 769-770 | Non-deterministic count | P1 | `BeNumerically(">=", 1)` | Deterministic event type validation |
| 780-782 | Weak null-testing | P2 | `ToNot(BeNil())` | Validate structured event_data fields |

---

## 🔍 **Detailed Violation Analysis**

### **Violation 1: Phase Transition Count (Lines 629-646)**

**Current Code**:
```go
Eventually(func() int {
    resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
        EventType:     &eventType,
        CorrelationId: &correlationID,
    })
    // ... error handling ...
    return 0
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 4),
    "BR-SP-090: SignalProcessing MUST emit at least 4 phase.transition events")
```

**DD-TESTING-001 Violation**:
- Uses `BeNumerically(">=", 4)` instead of `Equal(4)`
- Hides duplicate events (test passes with 5, 6, 7... events)
- Violates deterministic count validation mandate (DD-TESTING-001 line 256-299)

**Fix Strategy**:
```go
// Step 1: Wait for at least 4 events to appear (polling - BeNumerically OK here)
Eventually(func() int {
    // ... query logic ...
    return len(auditEvents)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 4),
    "BR-SP-090: Should have at least 4 phase.transition events")

// Step 2: Count events by type for deterministic validation
eventCounts := make(map[string]int)
for _, event := range auditEvents {
    eventCounts[event.EventType]++
}

// Step 3: Assert exact expected count (DD-TESTING-001 compliant)
Expect(eventCounts["signalprocessing.phase.transition"]).To(Equal(4),
    "BR-SP-090: MUST emit exactly 4 phase transitions: Pending→Enriching→Classifying→Categorizing→Completed")
```

**Business Logic**:
- SP phases: Pending → Enriching → Classifying → Categorizing → Completed
- **4 transitions total** (5 states = 4 transitions)
- Business requirement: Exactly 4 phase transitions per successful processing

**Confidence**: 95% (follows DD-TESTING-001 pattern, matches business requirement)

---

### **Violation 2: Error Event Count (Lines 769-770)**

**Current Code**:
```go
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Should have audit events even with errors (degraded mode processing)")
```

**DD-TESTING-001 Violation**:
- Uses `BeNumerically(">=", 1)` for final assertion
- Non-deterministic validation (test passes with 1, 2, 3... events)
- Violates mandate for exact event counts (DD-TESTING-001 line 256-299)

**Fix Strategy**:
```go
// Step 1: Wait for events to appear (polling - BeNumerically OK here)
Eventually(func() int {
    // ... query logic ...
    return len(auditEvents)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
    "Should have audit events even with errors (degraded mode processing)")

// Step 2: Count events by type for deterministic validation
eventCounts := make(map[string]int)
for _, event := range auditEvents {
    eventCounts[event.EventType]++
}

// Step 3: Validate EITHER error event OR completion event (deterministic)
// Business logic: In degraded mode, either emit error OR complete with degraded=true
hasErrorEvent := eventCounts["signalprocessing.error.occurred"] >= 1
hasCompletionEvent := eventCounts["signalprocessing.signal.processed"] >= 1

Expect(hasErrorEvent || hasCompletionEvent).To(BeTrue(),
    "BR-SP-090: MUST emit either error event OR degraded mode completion event")

// Step 4: Validate exactly 1 of the expected event types
if hasErrorEvent {
    Expect(eventCounts["signalprocessing.error.occurred"]).To(Equal(1),
        "BR-SP-090: Should emit exactly 1 error event per error occurrence")
} else {
    Expect(eventCounts["signalprocessing.signal.processed"]).To(Equal(1),
        "BR-SP-090: Should emit exactly 1 completion event (degraded mode)")
}
```

**Business Logic**:
- Error handling can result in:
  - **Option A**: Explicit error event (`error.occurred`)
  - **Option B**: Completion in degraded mode (`signal.processed` with degraded=true)
- **Not both** - mutually exclusive outcomes
- Validate exactly 1 of these events exists

**Confidence**: 90% (business logic requires verification of degraded mode behavior)

---

### **Violation 3: event_data Null-Testing (Lines 780-782)**

**Current Code**:
```go
Expect(event.EventData).ToNot(BeNil(),
    "Error event should contain event data with error details")
break
```

**DD-TESTING-001 Violation**:
- Weak null-testing assertion (only checks not nil)
- Doesn't validate event_data structure per DD-AUDIT-004
- Violates structured content validation mandate (DD-TESTING-001 line 303-334)

**Fix Strategy**:
```go
// Cast event_data to map for structured validation
eventData, ok := event.EventData.(map[string]interface{})
Expect(ok).To(BeTrue(), "event_data should be a JSON object")

// Validate required error fields per DD-AUDIT-004
Expect(eventData).To(HaveKey("error_message"),
    "Error event should contain error_message field")
Expect(eventData).To(HaveKey("error_type"),
    "Error event should contain error_type field")

// Validate field values
errorMessage := eventData["error_message"].(string)
Expect(errorMessage).ToNot(BeEmpty(),
    "Error message should not be empty")

errorType := eventData["error_type"].(string)
Expect([]string{"enrichment_error", "classification_error", "k8s_api_error"}).To(ContainElement(errorType),
    "Error type should be a known error category")
```

**Business Logic**:
- Error events MUST contain structured error information
- Required fields: `error_message`, `error_type`, optionally `error_code`
- Per DD-AUDIT-004: Structured error payload for incident response

**Confidence**: 85% (requires verification of actual error payload structure in SP)

---

## 🎯 **Fix Implementation Order**

1. **Violation 1** (Phase Transition Count) - **20 minutes**
   - Straightforward fix, matches AI Analysis pattern
   - High confidence in business requirement (4 transitions)

2. **Violation 2** (Error Event Count) - **15 minutes**
   - Requires understanding degraded mode behavior
   - May need to inspect controller error handling logic

3. **Violation 3** (event_data Validation) - **10 minutes**
   - Simple structured validation
   - May need to verify error payload schema

**Total Time**: ~45 minutes

---

## 📋 **Compliance Checklist**

After fixes applied:

- [ ] All event counts use `Equal(N)` for deterministic validation
- [ ] All `BeNumerically(">=")` replaced with event type counting
- [ ] All weak null-testing replaced with structured field validation
- [ ] All fixes follow DD-TESTING-001 patterns (lines 256-334)
- [ ] Local test run confirms fixes work
- [ ] CI run confirms SP integration tests pass

---

## 🔗 **Related Issues**

- **SP CI Failure**: Phase transition test timed out after 120s (run 20687479052)
- **Root Cause**: Data Storage buffer flush timing issue + non-deterministic validation
- **Long-term Fix**: Investigate Data Storage audit buffer flush (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)

---

## 📊 **Expected Outcome**

**Before**: 1 SP test failing due to timeout + non-deterministic validation
**After**: SP tests pass with deterministic DD-TESTING-001 compliant validation

**Confidence**: 90% that fixes will resolve CI failure

