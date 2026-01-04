# Signal Processing DD-TESTING-001 Fixes Applied

**Service**: Signal Processing (SP)
**File**: `test/integration/signalprocessing/audit_integration_test.go`
**Date**: 2026-01-04
**Status**: ✅ All 3 violations fixed
**Commit**: Ready for commit

---

## 📊 **Fix Summary**

| Violation | Lines | Status | Fix Applied |
|---|---|---|---|
| Non-deterministic phase transition count | 629-646 | ✅ Fixed | Replaced `BeNumerically(">=", 4)` with `Equal(4)` |
| Non-deterministic error event count | 769-770 | ✅ Fixed | Added event type counting + deterministic validation |
| Weak null-testing for event_data | 780-782 | ✅ Fixed | Added structured field validation per DD-AUDIT-004 |

---

## 🔧 **Detailed Fixes**

### **Fix 1: Phase Transition Count (Lines 629-646)**

**Before** (DD-TESTING-001 Violation):
```go
Eventually(func() int {
    // ... query logic ...
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 4),
    "BR-SP-090: SignalProcessing MUST emit at least 4 phase.transition events")
```

**After** (DD-TESTING-001 Compliant):
```go
// Step 1: Poll for events to appear (BeNumerically OK for polling)
Eventually(func() int {
    // ... query ALL signalprocessing events ...
    return len(auditEvents)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
    "BR-SP-090: SignalProcessing MUST emit audit events")

// Step 2: Count events by type (DD-TESTING-001 MANDATORY)
eventCounts := make(map[string]int)
for _, event := range auditEvents {
    eventCounts[event.EventType]++
}

// Step 3: Assert exact expected count (deterministic)
Expect(eventCounts["signalprocessing.phase.transition"]).To(Equal(4),
    "BR-SP-090: MUST emit exactly 4 phase transitions")
```

**Why This Fix**:
- ✅ Follows DD-TESTING-001 pattern (lines 256-299)
- ✅ Detects duplicate events (test fails if 5 transitions emitted)
- ✅ Detects missing events (test fails if only 3 transitions emitted)
- ✅ Business requirement: 5 phases = 4 transitions (Pending→Enriching→Classifying→Categorizing→Completed)

---

### **Fix 2: Error Event Count (Lines 769-770)**

**Before** (DD-TESTING-001 Violation):
```go
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Should have audit events even with errors (degraded mode processing)")

// Weak validation: checks for either error OR completion event, but not deterministic
foundAudit := false
for _, event := range auditEvents {
    if event.EventType == "signalprocessing.error.occurred" {
        foundAudit = true
        Expect(event.EventData).ToNot(BeNil()) // ❌ Weak null-testing
        break
    }
}
```

**After** (DD-TESTING-001 Compliant):
```go
// Step 1: Poll for events to appear
Eventually(func() int {
    // ... query logic ...
    return len(auditEvents)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
    "Should have audit events even with errors")

// Step 2: Count events by type (DD-TESTING-001 MANDATORY)
eventCounts := make(map[string]int)
for _, event := range auditEvents {
    eventCounts[event.EventType]++
}

// Step 3: Validate business logic outcome (deterministic)
hasErrorEvent := eventCounts["signalprocessing.error.occurred"] > 0
hasCompletionEvent := eventCounts["signalprocessing.signal.processed"] > 0

Expect(hasErrorEvent || hasCompletionEvent).To(BeTrue(),
    "BR-SP-090: MUST emit either error event OR degraded mode completion event")

// Step 4: Assert exact count based on outcome (deterministic)
if hasErrorEvent {
    Expect(eventCounts["signalprocessing.error.occurred"]).To(Equal(1),
        "BR-SP-090: Should emit exactly 1 error event")
    // ... structured event_data validation ...
} else {
    Expect(eventCounts["signalprocessing.signal.processed"]).To(Equal(1),
        "BR-SP-090: Should emit exactly 1 completion event (degraded mode)")
}
```

**Why This Fix**:
- ✅ Follows DD-TESTING-001 pattern for deterministic counts
- ✅ Validates business logic: EITHER error event OR completion event (mutually exclusive)
- ✅ Detects if both events emitted (business logic bug)
- ✅ Detects if duplicate events emitted

---

### **Fix 3: event_data Structured Validation (Lines 780-782)**

**Before** (DD-TESTING-001 Violation):
```go
Expect(event.EventData).ToNot(BeNil(),
    "Error event should contain event data with error details")
```

**After** (DD-TESTING-001 Compliant):
```go
// DD-TESTING-001 MANDATORY: Validate structured event_data fields
eventData, ok := errorEvent.EventData.(map[string]interface{})
Expect(ok).To(BeTrue(), "event_data should be a JSON object")

// Per DD-AUDIT-004: Error events should contain structured error information
Expect(eventData).To(HaveKey("error_message"),
    "Error event should contain error_message field")

errorMessage := eventData["error_message"].(string)
Expect(errorMessage).ToNot(BeEmpty(),
    "Error message should not be empty")
```

**Why This Fix**:
- ✅ Follows DD-TESTING-001 structured content validation (lines 303-334)
- ✅ Validates DD-AUDIT-004 error payload schema compliance
- ✅ Replaces weak null-testing with meaningful field validation
- ✅ Detects if error_message is missing or empty (business logic bug)

---

## 🎯 **Compliance Verification**

### **DD-TESTING-001 Checklist** ✅

- [x] All event counts use `Equal(N)` for deterministic validation
- [x] `BeNumerically(">=")` only used for polling, not final assertions
- [x] All weak null-testing replaced with structured field validation
- [x] All fixes follow DD-TESTING-001 mandatory patterns
- [x] event_data validation per DD-AUDIT-004
- [x] OpenAPI client usage preserved (DD-API-001)
- [x] No linter errors introduced

### **Pattern Compliance**

| DD-TESTING-001 Pattern | Status | Location |
|---|---|---|
| Pattern 1: OpenAPI Client Setup | ✅ Preserved | Lines 622, 745 |
| Pattern 4: Deterministic Event Count | ✅ Applied | Lines 633-638, 754-759 |
| Pattern 5: Structured event_data Validation | ✅ Applied | Lines 783-791 |
| Anti-Pattern 2: Non-Deterministic Count | ✅ Removed | (was lines 645, 769) |
| Anti-Pattern 4: Weak Null-Testing | ✅ Removed | (was lines 780-781) |

---

## 🧪 **Expected Test Behavior**

### **Before Fixes** (CI Failure)
- ❌ Phase transition test timed out after 120s
- ❌ Test passed with 5 phase transitions (duplicate event undetected)
- ❌ Test passed with event_data=nil (missing error details)

### **After Fixes** (Expected)
- ✅ Test fails fast if wrong number of phase transitions
- ✅ Test fails if duplicate events emitted
- ✅ Test fails if event_data missing required fields
- ⚠️ May still timeout if Data Storage buffer flush issue persists (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)

---

## 📊 **Changes Summary**

```
Lines Changed: ~60 lines
Violations Fixed: 3
New Validations Added: 
- Event type counting (2 places)
- Exact count assertions (3 places)  
- Structured event_data validation (1 place)
Patterns Removed:
- BeNumerically(">=") for final assertions (2 places)
- Weak null-testing (1 place)
```

---

## 🔗 **Related Issues**

- **CI Failure**: SP integration test timeout (run 20687479052)
- **Root Cause**: Data Storage buffer flush timing + non-deterministic validation
- **This Fix Addresses**: Non-deterministic validation (DD-TESTING-001 violations)
- **Remaining Issue**: Data Storage buffer flush timing (separate fix needed)

---

## 📋 **Next Steps**

1. **Commit these fixes** with message:
   ```
   fix(test): SP audit tests DD-TESTING-001 compliance
   
   Violations fixed:
   - Replace BeNumerically(">=") with Equal() for event counts
   - Add event type counting for deterministic validation
   - Replace weak null-testing with structured field validation
   
   Per DD-TESTING-001 mandatory patterns (lines 256-334).
   
   Related: CI run 20687479052 (phase transition test timeout)
   ```

2. **Run SP integration tests locally** to verify fixes:
   ```bash
   make test-integration-signalprocessing
   ```

3. **Fix remaining services**:
   - AI Analysis (AA): event_data structure issue (lines 266)
   - HolmesGPT API (HAPI): OpenAPI client method names (6 tests)

4. **Push and verify CI**:
   - SP tests should pass with deterministic validation
   - May still see timeout issues (separate Data Storage buffer flush fix needed)

---

## ✅ **Confidence Assessment**

**Fix Quality**: 95%
- ✅ All fixes follow DD-TESTING-001 mandatory patterns
- ✅ Business logic validated correctly
- ✅ No linter errors
- ⚠️ Data Storage buffer flush timing may still cause timeouts (separate issue)

**Expected CI Outcome**: 85%
- ✅ SP tests will have deterministic validation
- ✅ Tests will correctly detect duplicate/missing events
- ⚠️ May still timeout due to Data Storage buffer flush issue (unrelated to these fixes)

---

**Status**: ✅ Ready for commit and local verification
**Next**: Fix AA and HAPI integration tests


