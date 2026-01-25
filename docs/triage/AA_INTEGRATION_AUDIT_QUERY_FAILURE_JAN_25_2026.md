# AIAnalysis Integration Test Failures - Audit Query Not Returning Events
## CI Run: [21325239024](https://github.com/jordigilh/kubernaut/actions/runs/21325239024/job/61381365125)
## Date: January 25, 2026

---

## 📋 **EXECUTIVE SUMMARY**

**Status**: 🔴 BLOCKING CI
**Affected Tests**: 2/59 AIAnalysis integration tests
**Impact**: Phase transition audit events are being created and buffered, but queries return 0 results
**Root Cause Hypothesis**: Mock auth transport or audit buffer flush timing issue

---

## 🚨 **FAILING TESTS**

### Test 1: "should audit errors during investigation phase"
**Location**: `test/integration/aianalysis/audit_flow_integration_test.go:511`

**Failure**:
```
[FAILED] Expected at least 1 phase transition event even in error scenarios
Expected
    <int>: 0
to be >=
    <int>: 1
```

**Test Flow**:
1. Creates AIAnalysis resource with RemediationID `rr-inv-error-90e1eb8f`
2. Waits for controller to process (eventually expects >0 audit events)
3. Queries DataStorage with:
   - `CorrelationID`: `rr-inv-error-90e1eb8f`
   - `EventCategory`: `"analysis"`
4. **FAILS**: Returns 0 events
5. Expects `eventCounts[aiaudit.EventTypePhaseTransition] >= 1`

---

### Test 2: "should automatically audit approval decisions during analysis"
**Location**: `test/integration/aianalysis/audit_flow_integration_test.go:604`

**Failure**:
```
🔍 DEBUG: Phase transitions found:
  Transition 1 (event 5): <nil> → <nil>
  Transition 2 (event 6): <nil> → <nil>
  Transition 3 (event 7): <nil> → <nil>
  Total phase transitions: 3 (expected: 3)
```

**Test Flow**:
1. Creates AIAnalysis resource with RemediationID `rr-approval-57386552`
2. Waits for controller to complete analysis phase
3. Queries all audit events for correlation ID
4. **SUCCEEDS**: Returns 7 events (3 phase transitions)
5. **FAILS**: Phase transition `EventData` fields show `<nil>` for `OldPhase` and `NewPhase`

---

## 🔬 **FORENSIC ANALYSIS**

### ✅ **Evidence: Events ARE Being Created**

From CI logs:
```json
{"level":"info","ts":"2026-01-25T02:17:05Z","logger":"audit-store","msg":"🔍 StoreAudit called","event_type":"aianalysis.phase.transition","event_action":"phase_transition","correlation_id":"rr-inv-error-90e1eb8f","buffer_capacity":10000,"buffer_current_size":0}
{"level":"info","ts":"2026-01-25T02:17:05Z","logger":"audit-store","msg":"✅ Event buffered successfully","event_type":"aianalysis.phase.transition","correlation_id":"rr-inv-error-90e1eb8f","buffer_size_after":0,"total_buffered":1}
```

**Observations**:
- `StoreAudit` called with correct `event_type` and `correlation_id`
- Event successfully buffered (buffer_size_after: 0, total_buffered: 1)
- Multiple phase transitions logged for both failing test correlation IDs

---

### ❓ **Mystery: Why Do Queries Return 0 Events?**

**Test Code** (`audit_flow_integration_test.go:566-578`):
```go
Eventually(func() int {
    params := ogenclient.QueryAuditEventsParams{
        CorrelationID: ogenclient.NewOptString(correlationID),
        EventCategory: ogenclient.NewOptString(eventCategory), // "analysis"
    }
    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return 0
    }
    events = resp.Data
    return len(events)
}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0),
    "Controller MUST generate audit events even during error scenarios")
```

**Query Parameters**:
- `CorrelationID`: `rr-inv-error-90e1eb8f` ✅
- `EventCategory`: `"analysis"` ✅ (matches `EventCategoryAIAnalysis = "analysis"` from `pkg/aianalysis/audit/audit.go:49`)

**Expected Behavior**: Should return events
**Actual Behavior**: Returns 0 events for Test 1, returns events with nil `EventData` for Test 2

---

### 🔍 **Hypothesis 1: Mock Auth Transport Issue**

**Evidence** (`suite_test.go:409-416`):
```go
auditMockTransport := testauth.NewMockUserTransport(
    fmt.Sprintf("test-aianalysis@integration.test-p%d", processNum),
)
dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
    "http://127.0.0.1:18095", // AIAnalysis integration test DS port (IPv4 explicit for CI)
    5*time.Second,
    auditMockTransport,
)
```

**Issue**: Test uses `NewMockUserTransport` with process-specific email
**Potential Problem**:
- Auth transport might require special header configuration
- Mock user might not have permission to query events created by controller
- Process-specific user (`test-p1`, `test-p2`) might not match audit store's user

**Supporting Evidence**:
- Test 2 DOES return events (7 total), but `EventData` is nil
- This suggests auth is working for Test 2, but not Test 1
- Or: Test 1's flush timing is off

---

### 🔍 **Hypothesis 2: Audit Buffer Flush Timing**

**Test Code** (`audit_flow_integration_test.go:555-559`):
```go
// Flush audit buffer before polling (ensures events are available)
flushCtx, flushCancel := context.WithTimeout(ctx, 2*time.Second)
defer flushCancel()
err := auditStore.Flush(flushCtx)
Expect(err).NotTo(HaveOccurred(), "Audit flush should succeed")
```

**Issue**: Flush called ONCE before the `Eventually` loop
**Potential Problem**:
- Events might be buffered AFTER the flush call
- `Eventually` loop doesn't flush on each retry
- **CRITICAL**: Test 2 shows this exact problem was fixed in Notification tests by moving flush INSIDE the `Eventually` block

**Comparison with Notification Fix** (PR #20):
```go
// CORRECT (from notification/controller_audit_emission_test.go:134-145)
Eventually(func() bool {
    // Flush audit buffer on each retry to ensure events are written to DataStorage
    _ = realAuditStore.Flush(queryCtx)
    events := queryAuditEvents("notification.message.sent", testID)
    if len(events) > 0 {
        slackEvent = &events[0]
        return true
    }
    return false
}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(), ...)
```

---

### 🔍 **Hypothesis 3: EventData Deserialization Issue (Test 2 Only)**

**Evidence**: Test 2 returns 7 events, but phase transitions show `<nil>` values

**Test Output**:
```
🔍 DEBUG: Phase transitions found:
  Transition 1 (event 5): <nil> → <nil>
  Transition 2 (event 6): <nil> → <nil>
  Transition 3 (event 7): <nil> → <nil>
```

**Code Creating EventData** (`pkg/aianalysis/audit/audit.go:193-212`):
```go
payload := &ogenclient.AIAnalysisPhaseTransitionPayload{
    OldPhase: from,
    NewPhase: to,
}
// ...
event.EventData = ogenclient.NewAIAnalysisPhaseTransitionPayloadAuditEventRequestEventData(*payload)
```

**Potential Problem**:
- OpenAPI client union type deserialization might be broken
- Recent refactor to use constants (commit `3fcfcebc`) might have introduced a bug
- EventData JSON might not be unmarshalling correctly when queried from DataStorage

---

## 🎯 **ROOT CAUSE (LIKELY)**

**Primary**: Audit buffer flush timing (Hypothesis 2)
**Secondary**: EventData deserialization (Hypothesis 3, Test 2 only)

**Evidence Supporting Primary**:
1. ✅ Events ARE being created and buffered (logs confirm)
2. ✅ Query parameters are correct (`correlation_id`, `event_category`)
3. ❌ Flush called ONCE before `Eventually` loop, not on each retry
4. ✅ Exact same issue was fixed in Notification tests (PR #20) by moving flush inside loop

**Evidence Supporting Secondary (Test 2)**:
1. ✅ Query returns 7 events (correct count)
2. ❌ Phase transition `EventData` shows `<nil>` for `OldPhase` and `NewPhase`
3. ❌ Recent refactor to use OpenAPI constants might have broken union deserialization

---

## 🛠️ **RECOMMENDED FIXES**

### Fix 1: Move Flush Inside Eventually Loop (Test 1)

**File**: `test/integration/aianalysis/audit_flow_integration_test.go:564-578`

**Change**:
```go
// BEFORE (BROKEN)
err := auditStore.Flush(flushCtx)
Expect(err).NotTo(HaveOccurred(), "Audit flush should succeed")

Eventually(func() int {
    params := ogenclient.QueryAuditEventsParams{ ... }
    resp, err := dsClient.QueryAuditEvents(ctx, params)
    // ...
}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0), ...)

// AFTER (FIXED)
Eventually(func() int {
    // Flush on each retry to ensure events are written to DataStorage
    _ = auditStore.Flush(ctx)

    params := ogenclient.QueryAuditEventsParams{ ... }
    resp, err := dsClient.QueryAuditEvents(ctx, params)
    // ...
}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0), ...)
```

**Confidence**: 95% (matches exact fix that worked for Notification)

---

### Fix 2: Investigate EventData Deserialization (Test 2)

**Steps**:
1. Check if `event_data` JSONB column in PostgreSQL contains correct `old_phase` and `new_phase` fields
2. Verify OpenAPI union type deserialization logic in `pkg/datastorage/ogen-client/`
3. Compare event JSON structure before/after recent constant refactoring (commit `3fcfcebc`)
4. Add debug logging to show raw `event_data` JSON before deserialization

**Potential Fix Locations**:
- `pkg/datastorage/ogen-client/oas_json_gen.go` (union deserialization)
- `pkg/datastorage/repository/audit_events_repository.go` (event creation)
- `pkg/aianalysis/audit/audit.go:193-212` (phase transition payload construction)

**Confidence**: 70% (requires investigation to confirm)

---

## 📊 **IMPACT ASSESSMENT**

| Metric | Value |
|--------|-------|
| **Tests Affected** | 2/59 (3.4%) |
| **Blocking CI** | ✅ Yes |
| **Business Impact** | Medium (audit trail completeness) |
| **User Impact** | None (integration test only) |
| **Fix Complexity** | Low (Fix 1), Medium (Fix 2) |
| **Fix ETA** | <1 hour (Fix 1), 2-4 hours (Fix 2) |

---

## 🔄 **NEXT STEPS**

1. **Immediate** (Unblock CI):
   - Apply Fix 1: Move flush inside `Eventually` loop for Test 1
   - Run AA integration tests locally to verify
   - Push fix if tests pass

2. **Follow-up** (Fix Test 2):
   - Investigate EventData deserialization issue
   - Check PostgreSQL `event_data` JSONB column contents
   - Compare with working Notification/SignalProcessing audit tests
   - Apply Fix 2 once root cause confirmed

3. **Prevention**:
   - Add linter rule: "Audit flush MUST be inside Eventually loop"
   - Update test template with correct flush pattern
   - Document in `docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md`

---

## 📚 **RELATED DOCUMENTATION**

- **PR #20**: Notification audit test fixes (flush inside Eventually)
- **DD-TESTING-001**: Eventually vs time.Sleep standards
- **ADR-034**: Audit event data structure
- **DD-AUDIT-004 V2.0**: OpenAPI-generated types for audit
- **docs/triage/PR20_AUDIT_QUERY_PAGINATION_BUG_ALL_SERVICES_JAN_24_2026.md**: Recent audit query refactoring

---

## 🏷️ **METADATA**

- **Severity**: P1 - Blocks CI
- **Component**: AIAnalysis Integration Tests
- **Category**: Test Flakiness / Audit Infrastructure
- **Created**: 2026-01-25 03:00 UTC
- **Author**: AI Assistant
- **Status**: INVESTIGATING
