# DD-API-001 Violations Triage - COMPLETE

**Date**: December 26, 2025
**Status**: ✅ Triage Complete
**Duration**: ~15 minutes
**Next Step**: Fix 6 confirmed violations across 3 files

---

## 🎯 **Triage Summary**

**Total Violations Found**: **6 raw HTTP calls** across **3 files**

| File | Service | Violations | Lines | Status | Action |
|------|---------|------------|-------|--------|--------|
| `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go` | RO | ✅ 0 | - | **ALREADY FIXED** | None needed |
| `test/e2e/notification/01_notification_lifecycle_audit_test.go` | Notification | ❌ 2 | 303, 355 | 🔴 **CRITICAL** | **FIX REQUIRED** |
| `test/integration/remediationorchestrator/audit_trace_integration_test.go` | RO | ❌ 1 | 176 | 🟡 **HIGH** | **FIX REQUIRED** |
| `test/integration/gateway/audit_integration_test.go` | Gateway | ❌ 3 | 200, 401, 564 | 🟡 **HIGH** | **FIX REQUIRED** |

**DataStorage Tests**: 3 files using raw HTTP - ✅ **ACCEPTABLE** (they own the API)

---

## 📊 **Detailed Findings**

### **1. RemediationOrchestrator E2E** ✅ **ALREADY FIXED**

**File**: `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`
**Status**: ✅ **COMPLIANT**

**Evidence**:
```go
// Lines 127-160: Already using OpenAPI client!
// ✅ DD-API-001: Helper using OpenAPI generated client (MANDATORY)
queryAuditEvents := func(correlationID string) ([]dsgen.AuditEvent, int, error) {
    eventCategory := "orchestration"
    limit := 100

    // ✅ MANDATORY: Use generated client with type-safe parameters
    resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: &eventCategory,
        Limit:         &limit,
    })
    // ...
}
```

**Comment**: File already follows DD-API-001. Original triage document was outdated.

---

### **2. Notification E2E** 🔴 **CRITICAL** (2 violations)

**File**: `test/e2e/notification/01_notification_lifecycle_audit_test.go`
**Violations**: 2 helper functions using raw HTTP

#### **Violation 1: queryAuditEventCount** (line 303)
```go
// ❌ WRONG: Raw HTTP with manual URL construction
func queryAuditEventCount(baseURL, correlationID, eventType string) int {
    url := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", baseURL, correlationID)
    if eventType != "" {
        url += "&event_type=" + eventType
    }

    resp, err := http.Get(url)  // ❌ Line 303
    // ... manual JSON parsing ...
}
```

#### **Violation 2: queryAuditEvents** (line 355)
```go
// ❌ WRONG: Raw HTTP with manual URL construction
func queryAuditEvents(baseURL, correlationID string) []audit.AuditEvent {
    url := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", baseURL, correlationID)

    resp, err := http.Get(url)  // ❌ Line 355
    // ... manual JSON parsing ...
}
```

**Impact**:
- ❌ No type safety for audit query parameters
- ❌ No validation of `event_category` parameter (ADR-034 v1.2 requirement)
- ❌ Manual JSON parsing duplicates OpenAPI client logic
- ❌ Will break silently if DataStorage API changes

**Fix Required**: Replace both functions with OpenAPI client calls

---

### **3. RemediationOrchestrator Integration** 🟡 **HIGH** (1 violation)

**File**: `test/integration/remediationorchestrator/audit_trace_integration_test.go`
**Violations**: 1 helper function using raw HTTP

#### **Violation: queryAuditEvents** (line 176)
```go
// ❌ WRONG: Raw HTTP with manual URL construction
queryAuditEvents := func(correlationID string, eventType string) ([]AuditEvent, error) {
    url := fmt.Sprintf("%s%s?correlation_id=%s",
        dataStorageURL, auditAPIPath, correlationID)

    if eventType != "" {
        url += fmt.Sprintf("&event_type=%s", eventType)
    }

    resp, err := httpClient.Get(url)  // ❌ Line 176
    // ... manual JSON parsing ...
}
```

**Note**: This test file is SEPARATE from `audit_emission_integration_test.go` and follows the **CORRECT PATTERN** (creates RRs, verifies audit side effects). It just needs to use the OpenAPI client for querying.

**Fix Required**: Replace helper function with OpenAPI client calls

---

### **4. Gateway Integration** 🟡 **HIGH** (3 violations)

**File**: `test/integration/gateway/audit_integration_test.go`
**Violations**: 3 inline raw HTTP calls in test scenarios

#### **Violation 1: signal.received event query** (line 200)
```go
// ❌ WRONG: Raw HTTP with manual URL construction
queryURL := fmt.Sprintf("%s/api/v1/audit/events?service=gateway&correlation_id=%s&event_type=gateway.signal.received",
    dataStorageURL, correlationID)

Eventually(func() int {
    auditResp, err := http.Get(queryURL)  // ❌ Line 200
    // ... manual JSON parsing ...
}
```

#### **Violation 2: signal.deduplicated event query** (line 401)
```go
// ❌ WRONG: Raw HTTP with manual URL construction
queryURL := fmt.Sprintf("%s/api/v1/audit/events?service=gateway&event_type=gateway.signal.deduplicated&correlation_id=%s",
    dataStorageURL, correlationID)

Eventually(func() int {
    auditResp, err := http.Get(queryURL)  // ❌ Line 401
    // ... manual JSON parsing ...
}
```

#### **Violation 3: crd.created event query** (line 564)
```go
// ❌ WRONG: Raw HTTP with manual URL construction
queryURL := fmt.Sprintf("%s/api/v1/audit/events?service=gateway&event_type=gateway.crd.created&correlation_id=%s",
    dataStorageURL, correlationID)

Eventually(func() int {
    auditResp, err := http.Get(queryURL)  // ❌ Line 564
    // ... manual JSON parsing ...
}
```

**Note**: Gateway tests follow the **CORRECT PATTERN** (send webhooks, verify audit side effects). Per the audit anti-pattern triage, Gateway was identified as a reference implementation. However, the OpenAPI client usage was overlooked.

**Fix Required**: Replace all 3 inline HTTP calls with OpenAPI client calls

---

## 🔧 **Fix Plan**

### **Priority 1: Notification E2E** (🔴 CRITICAL)
**Estimated Time**: 45 minutes

**Steps**:
1. Add OpenAPI client import
2. Create `dsClient` in `BeforeEach`
3. Replace `queryAuditEventCount` function with OpenAPI client
4. Replace `queryAuditEvents` function with OpenAPI client
5. Test to ensure audit queries work

**Code Pattern**:
```go
// At top of file
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

var (
    dsClient *dsgen.ClientWithResponses
)

// In BeforeEach
dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
Expect(err).NotTo(HaveOccurred())

// Replace queryAuditEventCount
func queryAuditEventCount(correlationID string, eventType *string) int {
    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: ptr.To("notification"),
    }
    if eventType != nil {
        params.EventType = eventType
    }

    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil || resp.JSON200 == nil {
        return 0
    }

    if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
        return *resp.JSON200.Pagination.Total
    }
    return 0
}

// Replace queryAuditEvents
func queryAuditEvents(correlationID string) []dsgen.AuditEvent {
    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: ptr.To("notification"),
    }

    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil || resp.JSON200 == nil || resp.JSON200.Data == nil {
        return nil
    }

    return *resp.JSON200.Data
}
```

---

### **Priority 2: Gateway Integration** (🟡 HIGH)
**Estimated Time**: 30 minutes

**Steps**:
1. Add OpenAPI client to `BeforeSuite`
2. Replace 3 inline `http.Get` calls with OpenAPI client
3. Use `dsgen.AuditEvent` type instead of manual struct
4. Test to ensure audit queries work

**Code Pattern** (for each violation):
```go
// Replace inline http.Get
eventType := "gateway.signal.received"  // or deduplicated, crd.created
service := "gateway"
params := &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventType:     &eventType,
    Service:       &service,
    EventCategory: ptr.To("gateway"),
}

resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
if err != nil || resp.JSON200 == nil {
    return 0
}

total := 0
if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
    total = *resp.JSON200.Pagination.Total
}
```

---

### **Priority 3: RO Integration** (🟡 HIGH)
**Estimated Time**: 30 minutes

**Steps**:
1. Add OpenAPI client to `BeforeEach`
2. Replace `queryAuditEvents` helper with OpenAPI client
3. Use `dsgen.AuditEvent` type instead of custom `AuditEvent` struct
4. Test to ensure audit queries work

**Code Pattern**:
```go
// Replace queryAuditEvents helper
queryAuditEvents := func(correlationID string, eventType string) ([]dsgen.AuditEvent, error) {
    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: ptr.To("orchestration"),
    }
    if eventType != "" {
        params.EventType = &eventType
    }

    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    if err != nil {
        return nil, err
    }

    if resp.JSON200 == nil {
        return nil, fmt.Errorf("non-200 response: %d", resp.StatusCode())
    }

    if resp.JSON200.Data == nil {
        return []dsgen.AuditEvent{}, nil
    }

    return *resp.JSON200.Data, nil
}
```

---

## ✅ **Success Criteria**

This remediation is successful when:
- ✅ All 6 raw HTTP calls replaced with OpenAPI client
- ✅ Zero instances of `http.Get(url)` for DataStorage audit queries in these 3 files
- ✅ All tests pass with type-safe OpenAPI client
- ✅ All queries include `event_category` parameter (ADR-034 v1.2)
- ✅ Tests are resilient to DataStorage API evolution

---

## 📋 **Related Documents**

- **DD-API-001**: OpenAPI Generated Client MANDATORY for V1.0
- **ADR-034 v1.2**: event_category is mandatory for audit queries
- **AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md**: Testing patterns (Gateway and RO Integration follow correct pattern)

---

## 📊 **Time Estimates**

| Task | Estimated Time | Actual Time | Status |
|------|----------------|-------------|--------|
| **Triage** | 1 hour | 15 minutes | ✅ Complete |
| **Fix NT E2E** | 45 minutes | TBD | ⏳ Pending |
| **Fix Gateway Integration** | 30 minutes | TBD | ⏳ Pending |
| **Fix RO Integration** | 30 minutes | TBD | ⏳ Pending |
| **Total** | 2h 45min | TBD | ⏳ In Progress |

---

## 🎯 **Immediate Next Steps**

1. ✅ Triage complete - 6 violations confirmed
2. ⏳ Fix Notification E2E (Priority 1 - CRITICAL)
3. ⏳ Fix Gateway Integration (Priority 2 - HIGH)
4. ⏳ Fix RO Integration (Priority 3 - HIGH)
5. ⏳ Update DD_API_001_VIOLATIONS_TRIAGE_DEC_26_2025.md with final results
6. ⏳ Commit with: "fix(tests): DD-API-001 compliance - Convert all audit queries to OpenAPI client"

---

**Document Status**: ✅ Active
**Created**: 2025-12-26
**Last Updated**: 2025-12-26
**Priority Level**: 0 - CRITICAL (DD-API-001 is mandatory for V1.0)
**Authority**: Enforces DD-API-001 OpenAPI client mandate




