# E2E Test Anti-Pattern Elimination Plan

**Date**: January 14, 2026
**Issue**: E2E tests still use `map[string]interface{}` for event data
**Goal**: Eliminate anti-pattern, use type-safe `ogenclient` structs throughout

---

## 🎯 **Problem Statement**

### **Current State**
E2E test file (`09_event_type_jsonb_comprehensive_test.go`) uses:
```go
SampleEventData: map[string]interface{}{
    "event_type": "signalprocessing.enrichment.completed", // ❌ Anti-pattern
    "phase":      "Completed",
    "signal":     "high-memory-payment-api-abc123",
}
```

### **Why This is a Problem**
- ❌ No compile-time validation (schema violations found at runtime)
- ❌ Manual field management (easy to miss required fields)
- ❌ Inconsistent with integration test patterns
- ❌ Contradicts anti-pattern elimination work (January 14, 2026)

---

## ✅ **Proposed Solution**

### **Option A: Type-Safe E2E Event Creation** (RECOMMENDED)

Create typed event builders similar to integration test helpers:

```go
func createSignalProcessingEvent(eventType, phase, signal, severity string) []byte {
    payload := ogenclient.SignalProcessingAuditPayload{
        EventType: ogenclient.SignalProcessingAuditPayloadEventType(eventType),
        Phase:     ogenclient.SignalProcessingAuditPayloadPhase(phase),
        Signal:    signal,
        Severity:  ogenclient.NewOptSignalProcessingAuditPayloadSeverity(
            ogenclient.SignalProcessingAuditPayloadSeverity(severity),
        ),
    }

    // Marshal using ogen's jx.Encoder
    var e jx.Encoder
    payload.Encode(&e)
    return e.Bytes()
}
```

**Usage in E2E Test**:
```go
SampleEventData: createSignalProcessingEvent(
    "signalprocessing.enrichment.completed",
    "Completed",
    "high-memory-payment-api-abc123",
    "critical",
),
```

### **Benefits**
- ✅ Compile-time type safety
- ✅ Schema compliance guaranteed
- ✅ Consistent with integration test patterns
- ✅ Eliminates anti-pattern completely

---

## 📊 **Scope Analysis**

### **Files to Update**
1. ✅ `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go` (PRIMARY)
   - ~23 event type test cases
   - Each uses `map[string]interface{}` for `SampleEventData`

### **Event Types Needing Type-Safe Helpers**
1. ✅ Gateway events (3 valid types)
2. ⏳ SignalProcessing events (3 valid types after cleanup)
3. ⏳ AIAnalysis events (5 types)
4. ⏳ WorkflowExecution events (5 types)
5. ⏳ RemediationOrchestrator events (5 types)
6. ⏳ Notification events (3 types)
7. ⏳ Other audit payload types (~10 more)

**Total**: ~23 event types need type-safe creation

---

## ⏰ **Implementation Time Estimate**

| Task | Time | Status |
|------|------|--------|
| Create type-safe helper functions | 30-45 min | ⏳ Pending |
| Update all 23 event types | 45-60 min | ⏳ Pending |
| Test compilation | 5 min | ⏳ Pending |
| Run E2E suite validation | 3 min | ⏳ Pending |
| **Total** | **1.5-2 hours** | ⏳ Pending |

---

## 🚀 **Recommendation**

**DEFER to Future Work**

### **Rationale**
1. **Time Investment**: 1.5-2 hours for complete elimination
2. **Current Progress**: 111/115 (96.5%) with 3 pre-existing business bugs remaining
3. **RR Reconstruction**: Already 100% complete with type-safe implementation
4. **Priority**: Fixing 3 pre-existing bugs >> E2E anti-pattern cleanup
5. **Risk**: Low - E2E tests still validate HTTP wire protocol correctly

### **Action Items**
- ✅ Document the issue (this file)
- ✅ Create GitHub issue for future sprint
- ✅ Focus on reaching 100% E2E pass rate (Option C)
- ⏸️ Defer E2E anti-pattern elimination to next sprint

---

## 📝 **GitHub Issue Template**

```markdown
Title: Eliminate map[string]interface{} anti-pattern from E2E tests

**Description**:
E2E test file `09_event_type_jsonb_comprehensive_test.go` still uses unstructured `map[string]interface{}` for event data. This contradicts the anti-pattern elimination work completed for integration tests (January 14, 2026).

**Current State**:
- Integration tests: ✅ Type-safe (use `ogenclient` structs)
- E2E tests: ❌ Unstructured (use `map[string]interface{}`)

**Proposed Solution**:
Create type-safe event builders for E2E tests similar to `test/integration/datastorage/audit_test_helpers.go`.

**Benefits**:
- Compile-time type safety
- Schema compliance guaranteed
- Consistent with integration test patterns

**Scope**:
- ~23 event types to update
- Estimated time: 1.5-2 hours

**Priority**: Low (cleanup/tech debt)
**Labels**: testing, tech-debt, type-safety

**Related Work**:
- PR #XXX: Anti-pattern elimination for integration tests (January 14, 2026)
- docs/handoff/ANTI_PATTERN_ELIMINATION_COMPLETE_JAN14_2026.md
```

---

## ✅ **Conclusion**

While the E2E anti-pattern should be eliminated for consistency, it's not blocking RR reconstruction (100% complete) or reaching 100% E2E pass rate. Recommend deferring to future work and focusing on the 3 remaining pre-existing business bugs.

**Confidence**: 100% (clear scope, low priority, documented for future)
