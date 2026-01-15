# E2E Anti-Pattern Triage - JSONB Comprehensive Test

**Date**: January 14, 2026  
**File**: `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go`  
**Issue**: Test uses `map[string]interface{}` despite availability of type-safe helpers

---

## 🔍 **Discovery**

### **Helper Functions Already Exist**
File: `test/e2e/datastorage/helpers.go`

✅ **Type-Safe Payload Constructors** (lines 184-244):
- `newMinimalGatewayPayload()` → `ogenclient.GatewayAuditPayload`
- `newMinimalAIAnalysisPayload()` → `ogenclient.AIAnalysisAuditPayload`
- `newMinimalWorkflowPayload()` → `ogenclient.WorkflowExecutionAuditPayload`
- `newMinimalGenericPayload()` → `ogenclient.WorkflowSearchAuditPayload`

✅ **Type-Safe Event Creation** (line 251):
- `createAuditEventOpenAPI(ctx, client, ogenclient.AuditEventRequest)`
- Returns `string` (event_id)
- Handles all response types (201, 202, 400, 500)

### **Current Test Implementation**
File: `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go`

❌ **Raw HTTP + Unstructured Maps** (lines 590-613):
```go
auditEvent := map[string]interface{}{
    "version":         "1.0",
    "event_type":      tc.EventType,
    "event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
    "event_category":  tc.EventCategory,
    "event_action":    tc.EventAction,
    "event_outcome":   "success",
    "actor_type":      "service",
    "actor_id":        fmt.Sprintf("%s-service", tc.Service),
    "resource_type":   "Test",
    "resource_id":     fmt.Sprintf("test-%s-%s", tc.Service, uuid.New().String()[:8]),
    "correlation_id":  fmt.Sprintf("test-gap-1.1-%s", tc.EventType),
    "event_data":      eventDataWithDiscriminator, // Also map[string]interface{}
}

payloadBytes, err := json.Marshal(auditEvent)
resp, err := http.Post(dataStorageURL+"/api/v1/audit/events", "application/json", bytes.NewReader(payloadBytes))
```

---

## 🎯 **Root Cause Analysis**

### **Why Helpers Weren't Used**
1. **"Minimal" vs "Comprehensive"**: Existing helpers create minimal payloads for basic API testing
2. **JSONB Query Requirements**: Test needs specific field values to validate JSONB queries
3. **Data-Driven Design**: Test uses `eventTypeTestCase` struct with `map[string]interface{}` for flexibility
4. **Historical Context**: Test was written before anti-pattern elimination effort (January 14, 2026)

### **Why This is an Anti-Pattern**
❌ **No Compile-Time Validation**:
- Missing required fields discovered at runtime (HTTP 400 errors)
- Schema violations found during test execution (multiple iterations needed)

❌ **Maintenance Burden**:
- OpenAPI schema changes require manual test updates
- No IDE autocomplete for payload fields
- No type checking for enum values

❌ **Inconsistency**:
- Integration tests use type-safe helpers (`test/integration/datastorage/audit_test_helpers.go`)
- E2E tests use unstructured maps
- Contradicts "DD-API-001: OpenAPI Client Mandate"

---

## ✅ **Proposed Solution**

### **Option A: Extend Existing Helpers with JSONB-Specific Fields** (RECOMMENDED)

Create "comprehensive" payload constructors that accept field values for JSONB queries:

```go
// test/e2e/datastorage/helpers.go

func newSignalProcessingPayload(eventType, phase, signal, severity string) ogenclient.AuditEventRequestEventData {
    return ogenclient.AuditEventRequestEventData{
        Type: ogenclient.AuditEventRequestEventDataSignalprocessingEnrichmentCompletedAuditEventRequestEventData,
        SignalProcessingAuditPayload: ogenclient.SignalProcessingAuditPayload{
            EventType: ogenclient.SignalProcessingAuditPayloadEventType(eventType),
            Phase:     ogenclient.SignalProcessingAuditPayloadPhase(phase),
            Signal:    signal,
            Severity: ogenclient.NewOptSignalProcessingAuditPayloadSeverity(
                ogenclient.SignalProcessingAuditPayloadSeverity(severity),
            ),
            // Add other JSONB-testable fields as needed
        },
    }
}
```

**Usage in Test**:
```go
eventData := newSignalProcessingPayload(
    "signalprocessing.enrichment.completed",
    "Completed",
    "high-memory-payment-api-abc123",
    "critical",
)

auditEvent := ogenclient.AuditEventRequest{
    Version:        "1.0",
    EventType:      tc.EventType,
    EventTimestamp: time.Now().UTC(),
    EventCategory:  tc.EventCategory,
    EventAction:    tc.EventAction,
    EventOutcome:   "success",
    ActorType:      "service",
    ActorID:        fmt.Sprintf("%s-service", tc.Service),
    ResourceType:   "Test",
    ResourceID:     fmt.Sprintf("test-%s-%s", tc.Service, uuid.New().String()[:8]),
    CorrelationID:  fmt.Sprintf("test-gap-1.1-%s", tc.EventType),
    EventData:      eventData,
}

eventID := createAuditEventOpenAPI(ctx, client, auditEvent)
```

### **Benefits**
✅ Compile-time type safety  
✅ Schema compliance guaranteed  
✅ Reuses existing `createAuditEventOpenAPI` helper  
✅ Consistent with integration test patterns  
✅ Extends (not replaces) existing helpers

---

## 📊 **Scope Assessment**

### **Event Types in Test** (24 total after cleanup)
1. ✅ Gateway events (3 types) - Existing helper available (`newMinimalGatewayPayload`)
2. ⏳ SignalProcessing events (3 types) - **Need new helper**
3. ⏳ AIAnalysis events (5 types) - Existing helper available (`newMinimalAIAnalysisPayload`)
4. ⏳ WorkflowExecution events (5 types) - Existing helper available (`newMinimalWorkflowPayload`)
5. ⏳ RemediationOrchestrator events (5 types) - **Need new helper**
6. ⏳ Notification events (3 types) - **Need new helper**

### **New Helpers Needed**
1. `newSignalProcessingPayload()` - For enrichment/categorization events
2. `newRemediationOrchestratorPayload()` - For lifecycle events
3. `newNotificationPayload()` - For notification events
4. Extend existing helpers with optional field parameters for JSONB testing

---

## ⏰ **Implementation Time Estimate**

| Task | Time | Status |
|------|------|--------|
| Create 3 new payload constructors | 20-30 min | ⏳ Pending |
| Extend 3 existing helpers with optional params | 20-30 min | ⏳ Pending |
| Refactor test to use type-safe helpers | 45-60 min | ⏳ Pending |
| Update test data structure (`eventTypeTestCase`) | 15-20 min | ⏳ Pending |
| Test compilation + E2E validation | 10 min | ⏳ Pending |
| **Total** | **2-2.5 hours** | ⏳ Pending |

---

## 🚦 **Recommendation**

### **DEFER to Future Sprint** (SAME AS INITIAL PLAN)

**Rationale**:
1. **Current Progress**: 111/115 E2E tests passing (96.5%)
2. **RR Reconstruction**: ✅ 100% complete with type-safe implementation
3. **Priority**: Fixing 3 pre-existing business bugs >> E2E anti-pattern cleanup
4. **Risk**: Low - Test currently passes and validates correct behavior
5. **Time**: 2-2.5 hours better spent on business bug fixes

### **Immediate Actions**
- ✅ Document the issue (this file)
- ✅ Create GitHub issue for future sprint
- ✅ Continue with Option B+C: Fix remaining E2E failures to reach 100%
- ⏸️ Defer E2E anti-pattern elimination to next sprint

---

## 📝 **GitHub Issue Template**

```markdown
Title: Refactor JSONB comprehensive test to use type-safe helpers

**Description**:
E2E test `09_event_type_jsonb_comprehensive_test.go` uses raw `http.Post` with `map[string]interface{}` for event data, despite availability of type-safe helpers in `test/e2e/datastorage/helpers.go`.

**Current State**:
- Helper functions exist: `newMinimalGatewayPayload()`, `createAuditEventOpenAPI()`
- Test uses: `map[string]interface{}` + `http.Post()`

**Proposed Solution**:
1. Create comprehensive payload constructors (extend existing helpers with optional params)
2. Refactor test to use `ogenclient.AuditEventRequest` + `createAuditEventOpenAPI()`
3. Maintain JSONB query validation functionality

**Benefits**:
- Compile-time type safety (schema violations caught at build time)
- Consistent with integration test patterns
- Aligned with DD-API-001 (OpenAPI Client Mandate)

**Scope**:
- 24 event types to refactor
- 3 new payload constructors needed
- Estimated time: 2-2.5 hours

**Priority**: Low (cleanup/tech debt)
**Labels**: testing, tech-debt, type-safety, e2e

**Related Work**:
- PR #XXX: Anti-pattern elimination for integration tests (January 14, 2026)
- docs/handoff/ANTI_PATTERN_ELIMINATION_COMPLETE_JAN14_2026.md
- docs/handoff/E2E_ANTI_PATTERN_ELIMINATION_PLAN_JAN14_2026.md
```

---

## ✅ **Conclusion**

**Type-safe helpers ARE available** in `test/e2e/datastorage/helpers.go`, but the JSONB comprehensive test predates the anti-pattern elimination effort and uses raw HTTP + unstructured maps.

**Recommendation**: Defer refactoring to future sprint and focus on reaching 100% E2E pass rate by fixing the 3 remaining pre-existing business bugs.

**Confidence**: 100% (clear scope, existing patterns established, low priority)

---

**Next Steps**: Continue with Option B (fix signalprocessing test) + Option C (investigate remaining failures)
