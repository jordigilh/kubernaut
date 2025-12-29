# WorkflowExecution P1: Raw HTTP to OpenAPI Client Refactoring - IN PROGRESS

**Date**: December 20, 2025
**Status**: 🚧 **IN PROGRESS** (E2E complete, Integration partial)
**Author**: AI Assistant
**Service**: WorkflowExecution (CRD Controller)
**Task**: P1 Enhancement - Refactor audit test queries from raw HTTP to OpenAPI client

---

## 🎯 **Objective**

Refactor all raw HTTP audit queries (`http.Get()`) to use type-safe OpenAPI client (`dsgen.NewClientWithResponses()`), per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md best practices.

### **Benefits**
1. **Type Safety**: Automatic deserialization to `dsgen.AuditEvent` (no `map[string]interface{}` casting)
2. **Contract Validation**: OpenAPI client ensures API contract compliance
3. **Maintainability**: Compile-time errors if API schema changes
4. **Consistency**: Matches pattern used by SignalProcessing, AIAnalysis, Gateway services

---

## ✅ **Completed: E2E Tests** (3/3 queries refactored)

### **File**: `test/e2e/workflowexecution/02_observability_test.go`

**Status**: ✅ **COMPLETE** - All raw HTTP queries replaced with OpenAPI client

#### **Changes Made**:
1. ✅ Added imports: `dsgen` and `testutil`
2. ✅ Removed conversion helper (`convertHTTPResponseToAuditEvent`) - no longer needed
3. ✅ Refactored 3 audit queries to use `dsgen.NewClientWithResponses()`
4. ✅ Updated all response handling to use typed `dsgen.AuditEvent`
5. ✅ No linter errors

#### **Query Locations Refactored**:
- Line 390: First workflow audit query (workflow.started + completed/failed)
- Line 497: Second workflow audit query (workflow.failed with detailed validation)
- Line 602: Third workflow audit query (WorkflowExecutionAuditPayload field validation)

#### **Pattern Used**:
```go
// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
auditClient, err := dsgen.NewClientWithResponses(dataStorageServiceURL)
Expect(err).ToNot(HaveOccurred(), "Failed to create OpenAPI audit client")

eventCategory := "workflow"
var auditEvents []dsgen.AuditEvent
Eventually(func() int {
    resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
        EventCategory: &eventCategory,
        CorrelationId: &wfe.Name,
    })
    if err != nil {
        return 0
    }

    if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
        return 0
    }

    if resp.JSON200.Data != nil {
        auditEvents = *resp.JSON200.Data
    }
    return len(auditEvents)
}, 60*time.Second).Should(BeNumerically(">=", 2))
```

---

## 🚧 **In Progress: Integration Tests** (1/4 queries refactored)

### **File**: `test/integration/workflowexecution/reconciler_test.go`

**Status**: 🚧 **PARTIAL** - 1/4 queries refactored

#### **Changes Made**:
1. ✅ Added import: `dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"`
2. ✅ Added `context` import for OpenAPI client calls
3. ✅ Aliased prometheus testutil to avoid conflict: `prometheusTestutil`
4. ✅ Refactored query #1: workflow.started audit event (line 400)
5. ⏳ **TODO**: Refactor queries #2, #3, #4

#### **Query Locations**:
- ✅ Line 400: workflow.started audit event - **REFACTORED**
- ⏳ Line 466: workflow.completed audit event - **TODO**
- ⏳ Line 530: workflow.failed audit event - **TODO**
- ⏳ Line 592: Correlation ID verification audit query - **TODO**

#### **Pattern Established** (same as E2E):
```go
// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
auditClient, err := dsgen.NewClientWithResponses(dataStorageBaseURL)
Expect(err).ToNot(HaveOccurred(), "Failed to create OpenAPI audit client")

eventCategory := "workflow"
var startedEvent *dsgen.AuditEvent
Eventually(func() bool {
    resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
        EventCategory: &eventCategory,
        CorrelationId: &wfe.Name,
    })
    if err != nil {
        return false
    }

    if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil || resp.JSON200.Data == nil {
        return false
    }

    // Find specific event type
    for i := range *resp.JSON200.Data {
        if (*resp.JSON200.Data)[i].EventType == "workflowexecution.workflow.started" {
            startedEvent = &(*resp.JSON200.Data)[i]
            return true
        }
    }
    return false
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
```

---

## 📊 **Progress Summary**

| Test Tier | File | Queries | Refactored | Remaining | Status |
|-----------|------|---------|------------|-----------|--------|
| **E2E** | `test/e2e/workflowexecution/02_observability_test.go` | 3 | 3 | 0 | ✅ **COMPLETE** |
| **Integration** | `test/integration/workflowexecution/reconciler_test.go` | 4 | 1 | 3 | 🚧 **PARTIAL** |
| **Total** | 2 files | 7 | 4 | 3 | **57% complete** |

---

## 🔍 **Validation Status**

### **Current Validation Output**:
```bash
$ make validate-maturity

Checking: workflowexecution (crd-controller)
  ✅ Metrics wired
  ✅ Metrics registered
  ✅ EventRecorder present
  ✅ Graceful shutdown
  ✅ Audit integration
  ✅ Audit uses OpenAPI client
  ✅ Audit uses testutil validator
  ⚠️  Audit tests use raw HTTP (refactor to OpenAPI) (P1)  # ← Still shows due to integration tests
```

**Reason for Warning**: Validation script detects raw HTTP in `test/integration/workflowexecution/reconciler_test.go` (lines 466, 530, 592)

**Expected After Completion**: Warning will disappear when all 3 remaining integration test queries are refactored.

---

## 📝 **Remaining Work**

### **Integration Test Query #2: workflow.completed** (line 466)
**Location**: `test/integration/workflowexecution/reconciler_test.go:466-520`

**Current Code**:
```go
By("Querying DataStorage API for workflow.completed audit event")
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s&event_category=workflow",
    dataStorageBaseURL, wfe.Name)

var completedEvent map[string]interface{}
Eventually(func() bool {
    resp, err := http.Get(auditQueryURL)
    // ... raw HTTP parsing ...
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
```

**Required Refactoring**: Same pattern as query #1 - replace `http.Get` with `auditClient.QueryAuditEventsWithResponse()`

---

### **Integration Test Query #3: workflow.failed** (line 530)
**Location**: `test/integration/workflowexecution/reconciler_test.go:530-580`

**Current Code**:
```go
By("Querying DataStorage API for workflow.failed audit event")
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s&event_category=workflow",
    dataStorageBaseURL, wfe.Name)

var failedEvent map[string]interface{}
Eventually(func() bool {
    resp, err := http.Get(auditQueryURL)
    // ... raw HTTP parsing ...
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
```

**Required Refactoring**: Same pattern as query #1

---

### **Integration Test Query #4: Correlation ID verification** (line 592)
**Location**: `test/integration/workflowexecution/reconciler_test.go:592-620`

**Current Code**:
```go
By("Querying DataStorage API for audit events with correlation ID")
auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=test-corr-12345&event_category=workflow",
    dataStorageBaseURL)

Eventually(func() bool {
    resp, err := http.Get(auditQueryURL)
    // ... raw HTTP parsing ...
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
```

**Required Refactoring**: Same pattern as query #1, but with custom correlation ID

---

## 🎯 **Next Steps**

1. **Complete Integration Test Refactoring** (3 queries remaining)
   - Refactor query #2: workflow.completed
   - Refactor query #3: workflow.failed
   - Refactor query #4: Correlation ID verification

2. **Verify Validation**
   - Run `make validate-maturity`
   - Confirm P1 warning disappears

3. **Run Tests**
   - Execute integration test suite: `make test-integration-workflowexecution`
   - Verify all audit queries work with OpenAPI client

---

## 📚 **Reference Examples**

### **SignalProcessing Service** (Complete Reference)
- **File**: `test/integration/signalprocessing/audit_integration_test.go`
- **Pattern**: Uses `dsgen.NewClientWithResponses()` with `EventCategory` parameter
- **Lines**: 152-180

### **AIAnalysis Service** (Complete Reference)
- **File**: `test/integration/aianalysis/audit_integration_test.go`
- **Helper Function**: `queryAuditEventsViaAPI()` (lines 90-119)
- **Pattern**: Reusable helper for typed audit queries

### **Gateway Service** (Complete Reference)
- **File**: `test/e2e/gateway/15_audit_trace_validation_test.go`
- **Pattern**: E2E usage with testutil.ValidateAuditEvent
- **Lines**: 186-229

---

## ✅ **Success Criteria**

- ✅ All E2E tests use OpenAPI client (3/3 complete)
- 🚧 All integration tests use OpenAPI client (1/4 complete)
- ⏳ No raw HTTP audit queries detected by validation script
- ⏳ All tests pass with OpenAPI client
- ⏳ No linter errors

**Estimated Effort for Completion**: 20-30 minutes (3 similar refactorings)

---

**Priority**: P1 (Enhancement - not blocking V1.0 release, but improves code quality and maintainability)

**Confidence**: 90% - Pattern is proven and working in E2E tests, just needs mechanical application to remaining integration tests

