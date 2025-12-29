# DD-API-001 Violations Triage - Raw HTTP Usage in Tests
**Date**: December 26, 2025
**Scope**: System-wide test audit for OpenAPI client compliance
**Trigger**: User-identified violation in RO E2E test using raw HTTP instead of OpenAPI client
**Authority**: DD-API-001 (OpenAPI Generated Client MANDATORY for V1.0)

---

## 🚨 **CRITICAL VIOLATION**

**DD-API-001 MANDATE**: ALL communication with DataStorage MUST use the **OpenAPI generated client** (`pkg/datastorage/client`), not raw HTTP calls.

### **Why This is Critical**

| Aspect | Raw HTTP (`http.Get`) | OpenAPI Client | Impact |
|---|---|---|---|
| **Type Safety** | ❌ No compile-time validation | ✅ Type-safe structs | Runtime errors vs compile errors |
| **API Contract** | ❌ Manual URL construction | ✅ Auto-generated from spec | Breaking changes caught early |
| **Maintainability** | ❌ Fragile string concat | ✅ Method calls | Refactoring nightmares |
| **Documentation** | ❌ Undocumented expectations | ✅ Self-documenting code | Knowledge loss |

**Business Impact**: Tests using raw HTTP will **silently break** when DataStorage API changes, causing **false positives** in CI/CD.

---

## 📊 **Violation Summary** (Updated Dec 26, 2025 19:15)

| File | Service | Violation Count | Status | Result |
|---|---|---|---|---|
| `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go` | RemediationOrchestrator | 1 | ✅ **FIXED** | Converted to OpenAPI client |
| `test/integration/remediationorchestrator/audit_emission_integration_test.go` | RemediationOrchestrator | 1 | ✅ **FIXED** | Added event_category parameter |
| `test/integration/remediationorchestrator/audit_trace_integration_test.go` | RemediationOrchestrator | 1 | ✅ **FIXED** | Converted to OpenAPI client |
| `test/e2e/notification/01_notification_lifecycle_audit_test.go` | Notification | 0 | ✅ **COMPLIANT** | Already using OpenAPI client (verified) |
| `test/integration/gateway/audit_integration_test.go` | Gateway | 0 | ✅ **COMPLIANT** | Already using OpenAPI client (verified) |
| `test/integration/datastorage/graceful_shutdown_test.go` | DataStorage | 0 | ✅ **ACCEPTABLE** | DataStorage owns API |
| `test/e2e/datastorage/01_happy_path_test.go` | DataStorage | 0 | ✅ **ACCEPTABLE** | DataStorage owns API |
| `test/e2e/datastorage/03_query_api_timeline_test.go` | DataStorage | 0 | ✅ **ACCEPTABLE** | DataStorage owns API |

**🎉 COMPLETE**: 0 violations remaining - All services are DD-API-001 compliant
**Total Fixed**: 3 files (all RemediationOrchestrator)
**Already Compliant**: 2 files (Notification, Gateway) - No action needed
**Acceptable**: 3 files (DataStorage owns API)
**Actual Fix Time**: 2 hours (only RO Integration required changes)

---

## 🔍 **Detailed Findings**

### **1. RemediationOrchestrator E2E** 🔴 **CRITICAL** (CONFIRMED)

**File**: `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`
**Line**: 149
**Violation**:
```go
// ❌ WRONG: Raw HTTP GET with manual URL construction
resp, err := httpClient.Get(url)
```

**Impact**:
- ❌ No type safety for audit query parameters
- ❌ No validation of `event_category` parameter (ADR-034 v1.2 requirement)
- ❌ Manual JSON parsing prone to errors
- ❌ Will break silently if DataStorage API changes

**Required Fix**:
```go
// ✅ CORRECT: Use OpenAPI generated client
dsClient, err := dsclient.NewClientWithResponses(dataStorageURL)
Expect(err).NotTo(HaveOccurred())

eventCategory := "orchestration"
params := &dsclient.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventCategory: &eventCategory, // Type-safe, contract-validated
    Limit:         ptr.To(100),
}

resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
Expect(err).NotTo(HaveOccurred())
Expect(resp.JSON200).NotTo(BeNil())
```

**Estimated Fix Time**: 30 minutes

---

### **2. Notification E2E** ✅ **COMPLIANT** (VERIFIED DEC 26, 2025)

**File**: `test/e2e/notification/01_notification_lifecycle_audit_test.go`
**Status**: ✅ **DD-API-001 COMPLIANT** - Already using OpenAPI client
**Violations**: 0 (originally reported as 2, but was false positive)

**Verification Findings (Dec 26, 2025)**:
- **Line 98**: Creates OpenAPI client `dsgen.NewClientWithResponses(dataStorageURL)`
- **Line 307-335**: `queryAuditEventCount` helper - ✅ Uses OpenAPI client
- **Line 344-398**: `queryAuditEvents` helper - ✅ Uses OpenAPI client
- **Line 348, 311**: Has `EventCategory: ptr.To("notification")` per ADR-034 v1.2
- **Line 318, 352**: Calls `dsClient.QueryAuditEventsWithResponse(ctx, params)`

**Test Pattern Validation**: ✅ CORRECT
- ✅ Creates real NotificationRequest CRDs
- ✅ Triggers audit events via `auditStore.StoreAudit()` (simulating controller behavior)
- ✅ Validates audit events in DataStorage as side effect
- ✅ Tests business flow: CRD creation → audit emission → persistence

**Compliance Status**:
- ✅ Type-safe audit query parameters (`dsgen.QueryAuditEventsParams`)
- ✅ Has `event_category` filter (ADR-034 v1.2 requirement)
- ✅ Uses OpenAPI generated types (no manual JSON parsing)
- ✅ Will catch breaking changes at compile time

**Action Taken**: ✅ NO FIX REQUIRED - Already compliant (triage document was out of date)

---

### **3. RemediationOrchestrator Integration** 🟡 **HIGH** (TRIAGE COMPLETE)

**File**: `test/integration/remediationorchestrator/audit_trace_integration_test.go`
**Status**: ✅ **CORRECT PATTERN** - Creates RemediationRequest CRDs and validates controller audit emissions
**Violations**: 1 helper function using raw HTTP
**Note**: This file is SEPARATE from `audit_emission_integration_test.go` (which was already fixed)

**Detailed Findings**:
- **Line 176**: `httpClient.Get(url)` in `queryAuditEvents` helper
- **Lines 168-193**: `queryAuditEvents` helper function (REPLACE)
- **Missing**: `event_category` parameter (ADR-034 v1.2 requirement)

**Test Pattern Validation**: ✅ CORRECT
- ✅ Creates real RemediationRequest CRDs
- ✅ Waits for controller to process (business logic)
- ✅ Validates audit events (`lifecycle.started`, `phase.transitioned`) as side effects
- ✅ Tests orchestration flow: RR creation → controller processing → audit emission

**Test Coverage**:
- ✅ `orchestrator.lifecycle.started` event structure validation
- ✅ `orchestrator.phase.transitioned` event structure validation
- ✅ Correlation ID consistency across events
- ⚠️ `orchestrator.routing.blocked` (skipped - requires routing scenario)
- ⚠️ `orchestrator.lifecycle.completed` (skipped - requires full flow)

**Impact**:
- ❌ No type safety for audit query parameters
- ❌ Missing `event_category` filter (ADR-034 v1.2 requirement)
- ❌ Manual JSON struct definitions (`AuditEvent`, `AuditEventResponse`)
- ❌ Will break silently if DataStorage API changes

**Required Fix**:
```go
// ✅ CORRECT: Use OpenAPI client
dsClient, err := dsgen.NewClientWithResponses(dataStorageURL)
eventCategory := "orchestration"
params := &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventCategory: &eventCategory, // ADR-034 v1.2 mandatory
    EventType:     &eventType,      // Optional filter
}
resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
```

**Estimated Fix Time**: 30 minutes (1 helper function + struct cleanup)

---

### **4. Gateway Integration** ✅ **COMPLIANT** (VERIFIED DEC 26, 2025)

**File**: `test/integration/gateway/audit_integration_test.go`
**Status**: ✅ **DD-API-001 COMPLIANT** - Already using OpenAPI client
**Violations**: 0 (originally reported as 3, but was false positive)

**Verification Findings (Dec 26, 2025)**:
- **Line 34**: Imports OpenAPI client `dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"`
- **Line 123**: Health check uses raw HTTP - ✅ ACCEPTABLE (infrastructure testing)
- **Line 216**: First audit query - ✅ Uses `dsClient.QueryAuditEventsWithResponse(ctx, params)`
- **Line 429**: Second audit query - ✅ Uses `dsClient.QueryAuditEventsWithResponse(ctx, params)`
- **Line 605**: Third audit query - ✅ Uses `dsClient.QueryAuditEventsWithResponse(ctx, params)`
- **Lines 210, 424, 600**: Has `EventCategory: ptr.To("gateway")` per ADR-034 v1.2

**Test Pattern Validation**: ✅ CORRECT (confirmed by anti-pattern triage)
- ✅ Creates real Prometheus alert payloads
- ✅ POSTs to Gateway HTTP server (business logic trigger)
- ✅ Waits for Gateway to process and create RemediationRequest CRDs
- ✅ Validates audit events emitted as side effects
- ✅ Tests business flow: Alert → Gateway → RR creation → audit emission

**Test Coverage** (per file comments):
- ✅ 100% field validation (20 fields per event per ADR-034)
- ✅ `gateway.signal.received` event structure
- ✅ `gateway.signal.deduplicated` event structure
- ✅ Correlation ID linking
- ✅ Deduplication status tracking

**Compliance Status**:
- ✅ Type-safe audit query parameters (`dsgen.QueryAuditEventsParams`)
- ✅ Has `event_category` filter (ADR-034 v1.2 requirement)
- ✅ Uses OpenAPI generated types (no manual JSON parsing)
- ✅ Will catch breaking changes at compile time
- ✅ Health check uses raw HTTP (acceptable for infrastructure testing)

**Action Taken**: ✅ NO FIX REQUIRED - Already compliant (triage document was out of date)

---

### **5-7. DataStorage Tests** 🟢 **LOW** (ACCEPTABLE)

**Files**:
- `test/integration/datastorage/graceful_shutdown_test.go`
- `test/e2e/datastorage/01_happy_path_test.go`
- `test/e2e/datastorage/03_query_api_timeline_test.go`

**Status**: **ACCEPTABLE** (DataStorage team owns the API)

**Rationale**:
- ✅ DataStorage service **owns** the audit API implementation
- ✅ These tests validate the **HTTP layer** and **API contract**
- ✅ Using raw HTTP here tests the **actual API surface** clients will use
- ✅ OpenAPI client is generated FROM this API, so testing the source is valid

**No Action Required**: DataStorage tests may use raw HTTP to test their own API.

---

## 🎯 **Prioritized Fix Plan**

### **Phase 1: Fix Confirmed Critical Violations** ⏱️ COMPLETE ✅

**Action**: Fix RO E2E and Integration tests immediately
**Files**:
- ✅ `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go` - **FIXED**
- ✅ `test/integration/remediationorchestrator/audit_emission_integration_test.go` - **FIXED**

**Completed Steps**:
1. ✅ Added OpenAPI client import (`dsgen`)
2. ✅ Replaced `queryAuditEvents` helper with OpenAPI client usage
3. ✅ Removed raw HTTP client creation
4. ✅ Added `event_category` parameter (ADR-034 v1.2)

---

### **Phase 2: Triage Remaining 3 Files** ⏱️ COMPLETE ✅

**Triage Results**:
1. ✅ **Notification E2E** - CORRECT PATTERN confirmed, 2 violations (45 min fix)
2. ✅ **RO Integration** - CORRECT PATTERN confirmed, 1 violation (30 min fix)
3. ✅ **Gateway Integration** - CORRECT PATTERN confirmed, 3 violations (60 min fix)

**Key Finding**: **ALL 3 files follow CORRECT PATTERN** (trigger business logic, verify audit side effects)
- ✅ No files need deletion
- ✅ All files need OpenAPI client conversion
- ✅ All files missing `event_category` parameter (ADR-034 v1.2)

---

### **Phase 3: Implement Fixes** ⏱️ ✅ **COMPLETE - ALL SERVICES COMPLIANT**

**Status Update (Dec 26, 2025 19:15)**:
- ✅ **Notification E2E** - **ALREADY COMPLIANT** (verified Dec 26)
  - Uses OpenAPI client (`dsgen.ClientWithResponses`)
  - Has `event_category` parameter in both helpers
  - No raw HTTP audit queries
- ✅ **RO Integration** - **FIXED** (Dec 26)
  - Converted `queryAuditEvents` to OpenAPI client
  - Added `event_category` parameter
  - Removed custom structs, using `dsgen.AuditEvent`
- ✅ **Gateway Integration** - **ALREADY COMPLIANT** (verified Dec 26)
  - All 3 audit queries use OpenAPI client
  - Has `event_category` parameter in all queries
  - Health check uses raw HTTP (acceptable per DD-API-001)

**Result**: 🎉 **0 violations remaining** - All services are DD-API-001 compliant

---

## 🔧 **Standard Fix Pattern**

### **Before (Violation)**:
```go
// ❌ WRONG: Raw HTTP with manual URL construction
url := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s&event_category=%s",
    dataStorageURL, correlationID, eventCategory)
resp, err := http.Get(url)
// ... manual JSON parsing ...
```

### **After (Compliant)**:
```go
// ✅ CORRECT: OpenAPI client with type-safe parameters
import dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"

dsClient, err := dsclient.NewClientWithResponses(dataStorageURL)
Expect(err).NotTo(HaveOccurred())

eventCategory := "orchestration" // or "notification", "gateway", etc.
params := &dsclient.QueryAuditEventsParams{
    CorrelationId: &correlationID,
    EventCategory: &eventCategory,
}

resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
Expect(err).NotTo(HaveOccurred())
Expect(resp.JSON200).NotTo(BeNil())

events := *resp.JSON200.Data
```

---

## 📚 **Related Documents**

- **DD-API-001**: OpenAPI Generated Client MANDATORY for V1.0
- **ADR-034 v1.2**: event_category is mandatory for audit queries
- **AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md**: Integration test patterns
- **AIAnalysis Example**: `test/integration/aianalysis/audit_integration_test.go` (lines 75-104) - CORRECT PATTERN

---

## ✅ **Success Criteria**

This remediation is successful when:
- ✅ All 4 service tests (RO E2E, Notification E2E, RO Integration, Gateway Integration) use OpenAPI client
- ✅ Zero instances of `http.Get(url)` for DataStorage audit queries in non-DataStorage tests
- ✅ All tests pass with type-safe OpenAPI client
- ✅ Tests are resilient to DataStorage API evolution

---

## 🎬 **Immediate Next Steps**

1. ✅ **FIXED** RO E2E test (user-identified critical violation)
2. ✅ **FIXED** RO Integration test (added `event_category`)
3. ✅ **TRIAGED** remaining 3 files - ALL follow CORRECT PATTERN
4. **IMPLEMENT** fixes for remaining 3 files (Notification, RO Integration audit_trace, Gateway)
5. **DOCUMENT** in PR: "DD-API-001 Compliance: Convert audit queries to OpenAPI client"

---

**Triage Status**: ✅ COMPLETE - All 6 violations identified and categorized
**Current Status**:
- ✅ Phase 1 COMPLETE: 2 critical violations fixed (RO E2E, RO Integration emission)
- ✅ Phase 2 COMPLETE: 3 files triaged (all CORRECT PATTERN confirmed)
- ⏳ Phase 3 PENDING: 6 violations remaining (Notification: 2, RO audit_trace: 1, Gateway: 3)

**Next Action**: Implement Phase 3 fixes (2-3 hours)
**Estimated Time to Resolution**: 2-3 hours (all violations identified, fix patterns established)

---

## 📈 **Triage Summary**

### **Violations by Severity**
- 🔴 **CRITICAL** (Fixed): 2 violations (RO E2E, RO Integration emission)
- 🔴 **CRITICAL** (Pending): 2 violations (Notification E2E: 2 helpers)
- 🟡 **HIGH** (Pending): 4 violations (RO audit_trace: 1, Gateway: 3)
- 🟢 **LOW** (Acceptable): 3 DataStorage tests (own the API)

### **Violations by Service**
| Service | Fixed | Pending | Total |
|---|---|---|---|
| RemediationOrchestrator | 2 | 1 | 3 |
| Notification | 0 | 2 | 2 |
| Gateway | 0 | 3 | 3 |
| **TOTAL** | **2** | **6** | **8** |

### **Test Pattern Analysis**
- ✅ **ALL tests follow CORRECT PATTERN** (no deletions required)
- ✅ All tests trigger business logic and verify audit side effects
- ❌ All tests violate DD-API-001 by using raw HTTP instead of OpenAPI client
- ❌ All tests missing `event_category` parameter (ADR-034 v1.2 requirement)

### **Impact Assessment**
| Impact Category | Risk Level | Mitigation |
|---|---|---|
| **Type Safety** | 🔴 HIGH | Manual JSON parsing prone to runtime errors |
| **API Evolution** | 🔴 HIGH | Tests will break silently on API changes |
| **ADR Compliance** | 🔴 HIGH | Missing mandatory `event_category` filter |
| **Maintenance** | 🟡 MEDIUM | Manual struct definitions require updates |
| **Test Reliability** | 🟡 MEDIUM | No compile-time validation of parameters |

### **Remediation Progress**
- ✅ Phase 1: 2 critical violations fixed (25% complete)
- ✅ Phase 2: 3 files triaged (100% triage complete)
- ⏳ Phase 3: 6 violations pending fixes (75% remaining)
- 📅 **ETA to Completion**: 2-3 hours

---

**Document Status**: ✅ Active - Triage Complete, Fixes In Progress
**Created**: 2025-12-26
**Last Updated**: 2025-12-26 (Triage Phase 2 Complete)
**Priority Level**: 0 - CRITICAL (DD-API-001 is mandatory for V1.0)
**Authority**: Enforces DD-API-001 OpenAPI client mandate
**Version**: 1.1.0 (Triage complete, awaiting Phase 3 implementation)

