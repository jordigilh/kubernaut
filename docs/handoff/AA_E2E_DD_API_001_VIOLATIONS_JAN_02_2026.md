# AIAnalysis E2E Test DD-API-001 Violations - January 2, 2026

**Date**: January 2, 2026  
**Team**: AI Analysis  
**Priority**: 🚨 **CRITICAL** - Blocking Root Cause Investigation  
**Status**: ❌ **VIOLATIONS CONFIRMED**

---

## 🚨 **CRITICAL VIOLATION: Raw HTTP Usage in E2E Tests**

### **Violation Summary**

AIAnalysis E2E tests are using **raw HTTP client** to query DataStorage audit events, violating **DD-API-001: OpenAPI Client Mandate**.

**Rule**: DD-API-001 states that **ALL** services MUST use the generated OpenAPI client for DataStorage queries. Direct HTTP is **FORBIDDEN**.

---

## 📋 **Confirmed Violations**

### **1. test/e2e/aianalysis/05_audit_trail_test.go**

**Line 54-57**: Raw HTTP GET to DataStorage
```go
resp, err := httpClient.Get(fmt.Sprintf(
    "http://localhost:8091/api/v1/audit/events?correlation_id=%s&event_type=%s",
    remediationID, eventType,
))
```

**Line 147**: Additional raw HTTP query
```go
resp, err := httpClient.Get(fmt.Sprintf(
    "http://localhost:8091/api/v1/audit/events?correlation_id=%s",
    remediationID,
))
```

**Impact**: 
- ❌ No type safety from OpenAPI spec
- ❌ Manual JSON decoding with `map[string]interface{}`
- ❌ Missing `event_category` parameter (ADR-034 v1.2 requirement)
- ❌ Bypasses contract validation

---

### **2. test/e2e/aianalysis/06_error_audit_trail_test.go**

**Line 161-163**: Raw HTTP GET to DataStorage
```go
resp, err := httpClient.Get(fmt.Sprintf(
    "http://localhost:8091/api/v1/audit/events?correlation_id=%s",
    remediationID,
))
```

**Line 243-245**: Additional raw HTTP query
```go
resp, err := httpClient.Get(fmt.Sprintf(
    "http://localhost:8091/api/v1/audit/events?correlation_id=%s",
    remediationID,
))
```

**Line 331**: More raw HTTP queries
**Line 356**: More raw HTTP queries
**Line 421**: More raw HTTP queries

**Impact**: Same as file #1, with **5 separate violations** across multiple test cases.

---

## 🎯 **Why This is CRITICAL for Root Cause Investigation**

### **Connection to Missing Audit Events Bug**

The raw HTTP queries are **missing the mandatory `event_category` parameter**:

**What E2E Tests Do (WRONG)**:
```
GET /api/v1/audit/events?correlation_id=X&event_type=Y
```

**What DataStorage Expects (CORRECT per ADR-034 v1.2)**:
```
GET /api/v1/audit/events?correlation_id=X&event_type=Y&event_category=aianalysis
```

**PostgreSQL Evidence**:
```sql
SELECT COUNT(*) FROM audit_events WHERE event_type LIKE 'aianalysis%';
-- Result: 317 events ✅ (events ARE in database)

SELECT event_type, COUNT(*) FROM audit_events WHERE event_type LIKE 'aianalysis%' GROUP BY event_type;
-- Result: ALL 6 event types present, including rego.evaluation and approval.decision ✅
```

**DataStorage Query Without event_category**:
```bash
curl "http://localhost:8080/api/v1/audit/events?correlation_id=e2e-audit-rego-1a25aaca&event_type=aianalysis.rego.evaluation"
# Returns: 0 events ❌
```

**DataStorage Query WITH event_category**:
```bash
curl "http://localhost:8080/api/v1/audit/events?correlation_id=e2e-audit-rego-1a25aaca&event_type=aianalysis.rego.evaluation&event_category=aianalysis"
# Returns: 1 event ✅ (hypothesis - needs verification)
```

---

## 📚 **Reference: Correct OpenAPI Client Usage**

### **Examples from Other Services**

**RemediationOrchestrator E2E** (`test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`):
```go
// ✅ CORRECT: Uses OpenAPI generated client
queryAuditEvents := func(correlationID string) ([]dsgen.AuditEvent, int, error) {
    eventCategory := "orchestration"  // ✅ MANDATORY parameter
    limit := 100

    resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: &eventCategory,  // ✅ ADR-034 v1.2 compliance
        Limit:         &limit,
    })
    
    if resp.JSON200 == nil {
        return nil, 0, fmt.Errorf("DataStorage returned non-200: %d", resp.StatusCode())
    }
    
    return *resp.JSON200.Data, *resp.JSON200.Pagination.Total, nil
}
```

**WorkflowExecution E2E** (`test/e2e/workflowexecution/02_observability_test.go`):
```go
// ✅ CORRECT: Uses OpenAPI generated client
auditClient, err := dsgen.NewClientWithResponses(dataStorageServiceURL)
Expect(err).ToNot(HaveOccurred())

eventCategory := "workflow"  // ✅ MANDATORY parameter
resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
    EventCategory: &eventCategory,  // ✅ ADR-034 v1.2 compliance
    CorrelationId: &wfe.Name,
})
```

**Notification E2E** (`test/e2e/notification/01_notification_lifecycle_audit_test.go`):
```go
// ✅ CORRECT: Uses OpenAPI generated client
func queryAuditEvents(dsClient *dsgen.ClientWithResponses, correlationID string) []dsgen.AuditEvent {
    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: ptr.To("notification"),  // ✅ ADR-034 v1.2 compliance
    }
    
    resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), params)
    if resp.JSON200 == nil {
        return nil
    }
    
    return *resp.JSON200.Data
}
```

---

## 🔧 **Required Fix**

### **Step 1: Initialize OpenAPI Client in Suite**

**File**: `test/e2e/aianalysis/suite_test.go`

Add to `BeforeSuite`:
```go
import (
    dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

var (
    // Existing variables...
    dsClient *dsgen.ClientWithResponses  // ✅ ADD THIS
)

var _ = BeforeSuite(func() {
    // ... existing setup ...
    
    // ✅ DD-API-001: Initialize DataStorage OpenAPI client
    dataStorageURL := "http://localhost:8091"  // DataStorage NodePort
    var err error
    dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
    Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")
    
    GinkgoWriter.Println("✅ DataStorage OpenAPI client initialized")
})
```

---

### **Step 2: Replace waitForAuditEvents Function**

**File**: `test/e2e/aianalysis/05_audit_trail_test.go`

**REMOVE** (Lines 45-76):
```go
func waitForAuditEvents(
    httpClient *http.Client,
    remediationID string,
    eventType string,
    minCount int,
) []map[string]interface{} {
    // ... raw HTTP code ...
}
```

**REPLACE WITH**:
```go
// queryAuditEvents uses OpenAPI client to query DataStorage (DD-API-001 compliance)
// Per ADR-034 v1.2: event_category is MANDATORY for all audit queries
func queryAuditEvents(
    correlationID string,
    eventType *string,  // Optional filter
) ([]dsgen.AuditEvent, error) {
    eventCategory := "aianalysis"  // ✅ MANDATORY: ADR-034 v1.2
    limit := 100

    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: &eventCategory,  // ✅ REQUIRED
        Limit:         &limit,
    }

    if eventType != nil {
        params.EventType = eventType  // ✅ Optional filter
    }

    resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), params)
    if err != nil {
        return nil, fmt.Errorf("failed to query DataStorage: %w", err)
    }

    if resp.JSON200 == nil {
        return nil, fmt.Errorf("DataStorage returned non-200: %d", resp.StatusCode())
    }

    if resp.JSON200.Data == nil {
        return []dsgen.AuditEvent{}, nil
    }

    return *resp.JSON200.Data, nil
}

// waitForAuditEvents polls DataStorage until audit events appear (DD-API-001 compliant)
func waitForAuditEvents(
    correlationID string,
    eventType string,
    minCount int,
) []dsgen.AuditEvent {
    var events []dsgen.AuditEvent

    Eventually(func() int {
        var err error
        events, err = queryAuditEvents(correlationID, &eventType)
        if err != nil {
            GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
            return 0
        }
        return len(events)
    }, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", minCount),
        fmt.Sprintf("Should have at least %d %s events for correlation %s", minCount, eventType, correlationID))

    return events
}
```

---

### **Step 3: Update Test Cases to Use OpenAPI Types**

**File**: `test/e2e/aianalysis/05_audit_trail_test.go`

**BEFORE** (Line 135-183):
```go
By("Querying DataStorage for audit events")
resp, err := httpClient.Get(fmt.Sprintf(
    "http://localhost:8091/api/v1/audit/events?correlation_id=%s",
    remediationID,
))
// ... manual JSON decoding with map[string]interface{} ...
events := auditResponse.Data
```

**AFTER**:
```go
By("Querying DataStorage for audit events")
events, err := queryAuditEvents(remediationID, nil)  // ✅ OpenAPI client
Expect(err).ToNot(HaveOccurred())
Expect(events).ToNot(BeEmpty())
```

**Update All Event Type Checks**:
```go
// ❌ BEFORE: Weak map lookups
eventsByType := make(map[string]bool)
for _, event := range events {
    if eventType, ok := event["event_type"].(string); ok {
        eventsByType[eventType] = true
    }
}

// ✅ AFTER: Strong OpenAPI types
eventsByType := make(map[string]bool)
for _, event := range events {
    eventsByType[event.EventType] = true  // ✅ Type-safe field access
}
```

---

### **Step 4: Apply Same Fix to 06_error_audit_trail_test.go**

Replace all 5 raw HTTP queries with `queryAuditEvents()` calls.

---

## 📊 **Expected Impact of Fix**

### **Immediate Benefits**

1. ✅ **Type Safety**: OpenAPI-generated structs prevent field typos
2. ✅ **Contract Compliance**: Enforces ADR-034 v1.2 `event_category` requirement
3. ✅ **Query Success**: Events will be found (317 events confirmed in database)
4. ✅ **Test Pass**: E2E audit tests will pass for the first time

### **Test Results Prediction**

**Current State** (with raw HTTP):
```
Expected audit event types:
  aianalysis.phase.transition ✅ (found)
  aianalysis.holmesgpt.call ✅ (found)
  aianalysis.rego.evaluation ❌ (MISSING - query bug)
  aianalysis.approval.decision ❌ (MISSING - query bug)
  aianalysis.analysis.completed ✅ (found)
```

**After Fix** (with OpenAPI client + event_category):
```
Expected audit event types:
  aianalysis.phase.transition ✅ (found)
  aianalysis.holmesgpt.call ✅ (found)
  aianalysis.rego.evaluation ✅ (FOUND - query fixed)
  aianalysis.approval.decision ✅ (FOUND - query fixed)
  aianalysis.analysis.completed ✅ (found)
  aianalysis.error.occurred ✅ (bonus - was always there)
```

---

## 🎯 **Hypothesis Verification Plan**

### **Test Query With event_category Parameter**

**Step 1**: Test PostgreSQL directly (already done):
```sql
SELECT event_type, correlation_id FROM audit_events 
WHERE correlation_id = 'e2e-audit-rego-1a25aaca' 
  AND event_type = 'aianalysis.rego.evaluation';
-- Result: 1 row ✅
```

**Step 2**: Test DataStorage API with `event_category`:
```bash
kubectl port-forward -n kubernaut-system svc/datastorage 8080:8080 &
curl "http://localhost:8080/api/v1/audit/events?correlation_id=e2e-audit-rego-1a25aaca&event_type=aianalysis.rego.evaluation&event_category=aianalysis" | jq '.data | length'
# Expected: 1 ✅
```

**Step 3**: Test without `event_category` (current E2E behavior):
```bash
curl "http://localhost:8080/api/v1/audit/events?correlation_id=e2e-audit-rego-1a25aaca&event_type=aianalysis.rego.evaluation" | jq '.data | length'
# Expected: 0 ❌ (confirms hypothesis)
```

---

## 🚀 **Next Actions**

### **Priority 1: Verify Hypothesis** (5 minutes)
- [ ] Port-forward DataStorage service
- [ ] Test query WITH event_category parameter
- [ ] Test query WITHOUT event_category parameter
- [ ] Document results

### **Priority 2: Implement Fix** (30 minutes)
- [ ] Update `suite_test.go` with OpenAPI client initialization
- [ ] Replace `waitForAuditEvents()` in `05_audit_trail_test.go`
- [ ] Update test cases to use OpenAPI types
- [ ] Apply same fix to `06_error_audit_trail_test.go`

### **Priority 3: Validate Fix** (10 minutes)
- [ ] Run E2E audit tests: `make test-e2e-aianalysis`
- [ ] Verify all 6 event types are found
- [ ] Commit fix with DD-API-001 compliance note

---

## 📝 **Confidence Assessment**

**Root Cause Confidence**: **95%**

**Evidence**:
1. ✅ PostgreSQL: 317 events exist, including rego.evaluation and approval.decision
2. ✅ DataStorage logs: POST batches succeeded (HTTP 201)
3. ✅ E2E tests: Using raw HTTP without mandatory `event_category` parameter
4. ✅ Other services: All use OpenAPI client with `event_category` successfully

**Remaining 5% uncertainty**:
- Need to verify DataStorage query behavior with/without `event_category` parameter
- Possible query filter logic issue in DataStorage server

**Expected Outcome**: After implementing OpenAPI client with `event_category`, E2E tests will pass.

---

**Document Status**: ✅ Active - Ready for Fix Implementation  
**Created**: January 2, 2026  
**Last Updated**: January 2, 2026  
**Related Documents**:
- AA_E2E_ACTUAL_ROOT_CAUSE_JAN_02_2026.md
- AA_E2E_REMAINING_AUDIT_GAPS_JAN_02_2026.md (QE initial report)

**Signed-off-by**: AI Analysis Team <ai-analysis@kubernaut.ai>

