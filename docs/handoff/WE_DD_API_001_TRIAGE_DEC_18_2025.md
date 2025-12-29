# WorkflowExecution DD-API-001 Compliance Triage

**Document ID**: WE-DD-API-001-TRIAGE  
**Date**: December 18, 2025  
**Triaged By**: AI Assistant  
**Priority**: ⚠️ **MEDIUM** (V1.0 blocker per DD-API-001)  
**Related**: [DD-API-001-openapi-client-mandatory-v1.md](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)

---

## Executive Summary

**WorkflowExecution Service**: **PARTIALLY COMPLIANT** with DD-API-001

- ✅ **Integration Tests**: Using generated OpenAPI client (COMPLIANT)
- ❌ **E2E Tests**: Using direct HTTP for DataStorage queries (VIOLATION)

**Impact**: 3 test locations need migration (estimated 30-45 minutes)

**Severity**: Medium - Not blocking current WE work, but MUST be fixed before V1.0 release

---

## Compliance Assessment

### ✅ COMPLIANT: Integration Tests

**Location**: `test/integration/workflowexecution/`

**Evidence**:
```go
// suite_test.go:51
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

// reconciler_test.go:396, 432, 466
Expect(event.EventOutcome).To(Equal(dsgen.AuditEventRequestEventOutcomeSuccess))

// audit_datastorage_test.go:84
dsClient = audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
```

**Status**: ✅ **EXCELLENT** - Already using generated client with type-safe enums

---

### ❌ VIOLATION: E2E Tests

**Location**: `test/e2e/workflowexecution/02_observability_test.go`

**3 Violations Identified**:

#### **Violation 1: Line 312-317**
```go
// ❌ FORBIDDEN: Direct HTTP GET with manual URL construction
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
    dataStorageServiceURL, wfe.Name)

resp, err := http.Get(auditQueryURL)
// Manual JSON parsing to map[string]interface{}
var result struct {
    Data []map[string]interface{} `json:"data"`
}
```

**Issue**: Bypasses OpenAPI spec validation, no type safety, manual JSON parsing

---

#### **Violation 2: Line 421-426**
```go
// ❌ FORBIDDEN: Direct HTTP GET for workflow.failed event query
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
    dataStorageServiceURL, wfe.Name)

resp, err := http.Get(auditQueryURL)
// Manual JSON parsing with type assertions
eventData, ok := failedEvent["event_data"].(map[string]interface{})
```

**Issue**: Same violations as #1, plus error-prone type assertions

---

#### **Violation 3: Line 515-520**
```go
// ❌ FORBIDDEN: Direct HTTP GET for audit trail validation
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
    dataStorageServiceURL, wfe.Name)

resp, err := http.Get(auditQueryURL)
// Manual JSON parsing
var result struct {
    Data []map[string]interface{} `json:"data"`
}
```

**Issue**: Same violations as #1 and #2

---

## Risk Analysis

### Current Risks

**Type Safety** ❌:
- Field typos undetected (e.g., `event_category` vs `eventCategory`)
- Type mismatches discovered only at runtime
- Manual type assertions error-prone

**Schema Drift** ❌:
- If DataStorage API changes, tests won't catch it at compile time
- Missing parameters (like NT Team discovered) would go unnoticed
- False positive tests that would break in production

**Maintainability** ❌:
- Manual URL construction duplicated 3 times
- Manual JSON parsing duplicated 3 times
- API changes require manual updates in 3 locations

---

## DD-API-001 Context

### Why This Matters (From Notification Team Discovery)

**The Bug NT Team Found**:
- OpenAPI spec was missing 6 parameters (`event_category`, `event_outcome`, etc.)
- Generated client users (NT Team): **FOUND THE BUG** ✅ (compile error)
- Direct HTTP users (5 teams): **MISSED THE BUG** ❌ (tests passed with false positives)

**WE Would Have Missed It Too**:
If we had been using `event_category` parameter (which we should be!), our direct HTTP approach would have "worked" even though the OpenAPI spec was incomplete.

---

## Migration Plan

### Step 1: Update E2E Suite Setup

**File**: `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`

**Add**:
```go
import (
    dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

var (
    // ... existing vars ...
    dsClient *dsgen.ClientWithResponses
)

// In BeforeSuite, after DataStorage is ready:
var err error
dsClient, err = dsgen.NewClientWithResponses("http://localhost:8081")
Expect(err).ToNot(HaveOccurred())
```

**Time**: 5 minutes

---

### Step 2: Migrate Violation 1 (Line 312-340)

**File**: `test/e2e/workflowexecution/02_observability_test.go`

**Replace**:
```go
// ❌ BEFORE: Direct HTTP (30 lines)
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
    dataStorageServiceURL, wfe.Name)

var auditEvents []map[string]interface{}
Eventually(func() int {
    resp, err := http.Get(auditQueryURL)
    if err != nil {
        GinkgoWriter.Printf("⚠️ Audit query failed: %v\n", err)
        return 0
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        GinkgoWriter.Printf("⚠️ Audit query returned %d\n", resp.StatusCode)
        return 0
    }

    body, _ := io.ReadAll(resp.Body)
    var result struct {
        Data []map[string]interface{} `json:"data"`
    }
    if err := json.Unmarshal(body, &result); err != nil {
        GinkgoWriter.Printf("⚠️ Failed to parse audit response: %v\n", err)
        return 0
    }

    auditEvents = result.Data
    return len(auditEvents)
}, 60*time.Second).Should(BeNumerically(">=", 2), ...)
```

**With**:
```go
// ✅ AFTER: Generated client (12 lines + type safety)
correlationID := wfe.Name
eventCategory := "workflowexecution"  // ← NEW: More precise filtering
params := &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventCategory: &eventCategory,  // ← Leverages NT Team's fix
}

var auditEvents []dsgen.AuditEvent
Eventually(func() int {
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil || resp.JSON200 == nil {
        return 0
    }
    auditEvents = resp.JSON200.Data
    return len(auditEvents)
}, 60*time.Second).Should(BeNumerically(">=", 2), ...)
```

**Benefits**:
- ✅ 60% fewer lines (30 → 12)
- ✅ Type-safe structs (`dsgen.AuditEvent` vs `map[string]interface{}`)
- ✅ Compile-time validation (field typos caught immediately)
- ✅ Leverages `event_category` parameter (NT Team's fix)
- ✅ No manual JSON parsing or error handling

**Time**: 10 minutes

---

### Step 3: Migrate Violation 2 (Line 421-454)

**Replace**:
```go
// ❌ BEFORE: Direct HTTP + manual type assertions
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
    dataStorageServiceURL, wfe.Name)

var failedEvent map[string]interface{}
Eventually(func() bool {
    resp, err := http.Get(auditQueryURL)
    // ... manual parsing ...
    
    for _, event := range result.Data {
        if eventType, ok := event["event_action"].(string); ok {
            if eventType == "workflowexecution.workflow.failed" {
                failedEvent = event
                return true
            }
        }
    }
    return false
}, 60*time.Second).Should(BeTrue(), ...)
```

**With**:
```go
// ✅ AFTER: Generated client with type-safe filtering
correlationID := wfe.Name
eventCategory := "workflowexecution"
eventType := "workflowexecution.workflow.failed"
params := &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventCategory: &eventCategory,
    EventType:     &eventType,  // ← Filter server-side
}

var failedEvent *dsgen.AuditEvent
Eventually(func() bool {
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil || resp.JSON200 == nil || len(resp.JSON200.Data) == 0 {
        return false
    }
    failedEvent = &resp.JSON200.Data[0]  // ← Type-safe, no assertions
    return true
}, 60*time.Second).Should(BeTrue(), ...)

// ✅ Type-safe field access (no more type assertions!)
Expect(failedEvent.EventOutcome).To(Equal(dsgen.AuditEventEventOutcomeFailure))
Expect(failedEvent.EventData).ToNot(BeNil())
```

**Benefits**:
- ✅ Server-side filtering (faster queries)
- ✅ No manual looping through events
- ✅ No type assertions (`failedEvent.EventOutcome` vs `failedEvent["event_outcome"].(string)`)
- ✅ Compile-time validation of enum values

**Time**: 15 minutes

---

### Step 4: Migrate Violation 3 (Line 515-560)

**Replace** (similar pattern):
```go
// ❌ BEFORE: Direct HTTP + manual validation
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
    dataStorageServiceURL, wfe.Name)
```

**With** (same as Step 3):
```go
// ✅ AFTER: Generated client
correlationID := wfe.Name
eventCategory := "workflowexecution"
params := &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventCategory: &eventCategory,
}

var auditEvents []dsgen.AuditEvent
Eventually(func() int {
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil || resp.JSON200 == nil {
        return 0
    }
    auditEvents = resp.JSON200.Data
    return len(auditEvents)
}, 60*time.Second).Should(BeNumerically(">=", 2), ...)

// ✅ Type-safe event iteration
var startedEvent *dsgen.AuditEvent
for i := range auditEvents {
    if auditEvents[i].EventType == "workflowexecution.workflow.started" {
        startedEvent = &auditEvents[i]
        break
    }
}
Expect(startedEvent).ToNot(BeNil())

// ✅ Type-safe field access
Expect(startedEvent.EventData.WorkflowId).To(Equal(wfe.Spec.WorkflowRef.WorkflowID))
```

**Time**: 10 minutes

---

### Step 5: Validation

**Run E2E Tests**:
```bash
make test-e2e-workflowexecution
```

**Expected**:
- ✅ All tests compile (type safety validated)
- ✅ All tests pass (functional behavior preserved)
- ✅ Queries faster (server-side filtering with `event_category`)
- ✅ No direct HTTP usage (`grep -r "http\.Get.*audit" test/e2e/workflowexecution/` returns 0)

**Time**: 5 minutes

---

## Benefits Summary

### Code Quality

**Before** (Direct HTTP):
- 90 lines across 3 locations
- Manual URL construction (error-prone)
- Manual JSON parsing (verbose)
- Type assertions (runtime errors)
- No compile-time validation

**After** (Generated Client):
- 36 lines across 3 locations (60% reduction)
- Type-safe parameter structs
- Auto-generated parsing
- Compile-time field validation
- Leverages `event_category` filter

**Net**: 54 lines removed, type safety added

---

### Maintainability

**API Changes**:
- Before: Manual updates in 3 locations
- After: Regenerate client (automatic)

**Parameter Changes**:
- Before: Undetected (false positives)
- After: Compile error (caught immediately)

**Refactoring**:
- Before: Find/replace in 3 locations
- After: Single client interface

---

### Performance

**Query Efficiency**:
- Before: Client-side filtering (fetch all, filter locally)
- After: Server-side filtering (`event_category` + `event_type`)

**Expected Improvement**:
- Fewer events transferred over network
- Faster test execution (less parsing)
- Lower DataStorage load

---

## Timeline Estimate

| Task | Duration | Owner |
|---|----|-----|
| Step 1: Suite setup | 5 min | WE Team |
| Step 2: Migrate violation 1 | 10 min | WE Team |
| Step 3: Migrate violation 2 | 15 min | WE Team |
| Step 4: Migrate violation 3 | 10 min | WE Team |
| Step 5: Validation | 5 min | WE Team |
| **Total** | **45 minutes** | **WE Team** |

**Confidence**: 90% (same pattern as NT Team's successful migration)

---

## Success Criteria

### Immediate (Post-Migration)

- ✅ All E2E tests compile with generated client
- ✅ All E2E tests pass (9/9 or better)
- ✅ No direct HTTP usage in E2E tests
- ✅ Leverages `event_category` parameter

### Long-Term (V1.0 Release)

- ✅ WE service 100% compliant with DD-API-001
- ✅ No false positive tests (contract validated at compile time)
- ✅ Automatic API change detection (regenerate client)
- ✅ Type-safe audit event queries

---

## Related Work

### NT Team's Discovery

The Notification Team found the OpenAPI spec gap that would have affected WE's E2E tests if we had been using `event_category` parameter. Their migration to generated client FOUND the bug that 5 other teams (including us in E2E) MISSED.

**Reference**: [NT_DS_API_QUERY_ISSUE_DEC_18_2025.md](../handoff/NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)

### DS Team's Fix

DataStorage Team added 6 missing parameters to OpenAPI spec and regenerated the client. WE E2E tests can now leverage these parameters for more precise queries.

**Reference**: [DD-API-001 Lines 982-1112](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)

---

## Dependencies

### Prerequisites

- ✅ DataStorage OpenAPI spec updated (completed by DS Team)
- ✅ Generated client includes all 6 parameters (completed by DS Team)
- ✅ WE integration tests already using generated client (reference example)

### No Blockers

- No external dependencies
- No API changes needed
- Migration can proceed immediately

---

## Risk Assessment

### Migration Risks

**Low Risk**:
- ✅ Integration tests already using generated client (proven pattern)
- ✅ NT Team successfully migrated (reference example)
- ✅ Generated client is stable and well-tested
- ✅ Behavioral changes minimal (same API, different client)

**Mitigation**:
- Run E2E tests after each violation migration
- Compare audit event counts before/after
- Rollback plan: Revert to direct HTTP if issues (low probability)

---

## Recommendation

**PRIORITY**: ⚠️ **MEDIUM-HIGH**

**Action**: Migrate E2E tests to generated client **BEFORE V1.0 release**

**Rationale**:
1. DD-API-001 is a V1.0 blocker (MANDATORY for all services)
2. 45-minute migration is low effort, high value
3. Prevents false positive tests (contract enforcement)
4. Leverages NT Team's hard-won fix (6 new parameters)
5. Type safety prevents runtime errors

**Timing**:
- **Option A**: Migrate now (next session, 45 min)
- **Option B**: After investigating failing workflow issue (2 issues resolved in sequence)

---

## Next Steps

### Immediate

1. ⏸️ Continue investigating failing workflow E2E test (current TODO)
2. ⏳ Schedule DD-API-001 migration (45 minutes)

### V1.0 Checklist

- [ ] Complete failing workflow investigation
- [ ] Migrate E2E tests to generated client
- [ ] Run full E2E suite (expect 9/9 passing)
- [ ] Update WE service status in DD-API-001 (Line 354)

---

## Confidence Assessment

**Overall Confidence**: 90%

**Justification**:
- ✅ Integration tests prove generated client works for WE
- ✅ NT Team's migration validates the approach
- ✅ DS Team's fix is complete and tested
- ✅ Migration pattern is straightforward (proven by NT)
- ✅ Timeline estimate based on NT Team's actual experience

**Remaining 10% Uncertainty**: E2E-specific quirks (e.g., Kind cluster networking, DataStorage NodePort access)

---

**Status**: ⚠️ **TRIAGED** - Migration plan ready, awaiting execution
**Priority**: MEDIUM-HIGH (V1.0 blocker, but not blocking current work)
**Next**: Resume failing workflow investigation, then schedule migration

**Last Updated**: December 18, 2025 (AI Assistant)

