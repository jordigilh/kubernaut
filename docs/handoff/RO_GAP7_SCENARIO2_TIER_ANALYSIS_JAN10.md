# Gap #7 Scenario 2: Test Tier Analysis - Jan 10, 2026

## 🎯 **Executive Summary**

**Recommendation**: **REMOVE** Scenario 2 from integration tests and **ENHANCE** unit tests instead.

**Rationale**:
- Gap #7 tests error_details standardization, not K8s API error handling
- Error code mapping is **pure business logic** (string matching)
- Integration test Scenario 1 already validates end-to-end flow
- K8s API error simulation requires complex infrastructure (envtest RBAC mocking)
- Scenario 2 provides **low value** for **high complexity**

---

## 📊 **Current Coverage Analysis**

### **What Gap #7 Tests** (BR-AUDIT-005)
✅ **Error Details Standardization**: Audit events contain structured error_details
- `component`: Service name ("remediationorchestrator")
- `code`: Error code (e.g., "ERR_INVALID_TIMEOUT_CONFIG")
- `message`: Human-readable error message
- `retry_possible`: Boolean indicating if retry is advised

### **Current Test Coverage**

| Tier | Test | Status | Coverage |
|------|------|--------|----------|
| **Unit** | `BuildFailureEvent` basic | ✅ PASSING | Error_details structure |
| **Unit** | `BuildFailureEvent` timeout | ✅ PASSING | `ERR_TIMEOUT_REMEDIATION` code |
| **Integration** | Scenario 1: Timeout config | ✅ PASSING | End-to-end with `ERR_INVALID_TIMEOUT_CONFIG` |
| **Integration** | Scenario 2: CRD creation | ⚠️ PLACEHOLDER | **Not implemented** |

---

## 🔍 **Gap Analysis: What's Missing?**

### **Error Code Mapping Logic** (In `manager.go:233-252`)

```go
switch {
case strings.Contains(failureReason, "ERR_INVALID_TIMEOUT_CONFIG"):
    errorCode = "ERR_INVALID_TIMEOUT_CONFIG"  // ✅ Tested (Integration Scenario 1)
    retryPossible = false
case strings.Contains(failureReason, "invalid") || strings.Contains(failureReason, "configuration"):
    errorCode = "ERR_INVALID_CONFIG"          // ❌ NOT TESTED
    retryPossible = false
case strings.Contains(failureReason, "timeout"):
    errorCode = "ERR_TIMEOUT_REMEDIATION"     // ✅ Tested (Unit test)
    retryPossible = true
case strings.Contains(failureReason, "not found") || strings.Contains(failureReason, "create"):
    errorCode = "ERR_K8S_CREATE_FAILED"       // ❌ NOT TESTED (Scenario 2 target)
    retryPossible = true
default:
    errorCode = "ERR_INTERNAL_ORCHESTRATION"  // ❌ NOT TESTED
    retryPossible = true
}
```

### **Missing Unit Test Coverage**
- ❌ `ERR_INVALID_CONFIG` code mapping
- ❌ `ERR_K8S_CREATE_FAILED` code mapping
- ❌ `ERR_INTERNAL_ORCHESTRATION` default case
- ❌ `retry_possible` flag correctness for each error type

---

## 🚫 **Why Scenario 2 (Integration) is Wrong Tier**

### **Problem 1: Infrastructure Complexity**
Simulating K8s API errors in envtest requires:
- RBAC policy manipulation (ServiceAccount with restricted permissions)
- API server mocking or webhook injection
- Dynamic namespace permission changes
- Complex test setup/teardown

**Estimated Effort**: 4-8 hours
**Maintenance Burden**: High (brittle, K8s version-dependent)

### **Problem 2: Low Value**
- Scenario 2 tests **same code path** as Scenario 1:
  1. Controller detects failure
  2. Calls `BuildFailureEvent()`
  3. Stores audit event
  4. Test queries and validates error_details

**What's different?** Only the `failureReason` string input.

### **Problem 3: Business Logic is Unit-Testable**
Error code mapping is **pure function**:
```go
Input:  failureReason = "failed to create SignalProcessing: forbidden"
Output: errorCode = "ERR_K8S_CREATE_FAILED", retryPossible = true
```

This is **ideal for unit testing** - no K8s cluster needed.

---

## ✅ **Recommended Solution**

### **Step 1: Remove Scenario 2 (Integration Test)**
**File**: `test/integration/remediationorchestrator/audit_errors_integration_test.go`

**Action**: Delete lines 157-203 (Scenario 2 test)

**Rationale**:
- Reduces test complexity
- Eliminates placeholder `Fail()` that blocks test suite
- Integration Scenario 1 already validates end-to-end flow

---

### **Step 2: Add Comprehensive Unit Tests**
**File**: `test/unit/remediationorchestrator/audit/manager_test.go`

**Add new test context**:

```go
Context("BR-AUDIT-005 Gap #7: Error Code Mapping", func() {
    It("should map invalid config errors to ERR_INVALID_CONFIG", func() {
        event, err := manager.BuildFailureEvent(
            "correlation-001",
            "default",
            "rr-test",
            "configuration",
            "invalid workflow selector",
            1000,
        )
        Expect(err).ToNot(HaveOccurred())

        // Validate error_details
        eventDataBytes, _ := json.Marshal(event.EventData)
        var eventData map[string]interface{}
        _ = json.Unmarshal(eventDataBytes, &eventData)

        errorDetails := eventData["error_details"].(map[string]interface{})
        Expect(errorDetails["code"]).To(Equal("ERR_INVALID_CONFIG"))
        Expect(errorDetails["retry_possible"]).To(BeFalse(), "Invalid config is permanent")
    })

    It("should map K8s creation errors to ERR_K8S_CREATE_FAILED", func() {
        event, err := manager.BuildFailureEvent(
            "correlation-002",
            "default",
            "rr-test",
            "signal_processing",
            "failed to create SignalProcessing: forbidden",
            1000,
        )
        Expect(err).ToNot(HaveOccurred())

        eventDataBytes, _ := json.Marshal(event.EventData)
        var eventData map[string]interface{}
        _ = json.Unmarshal(eventDataBytes, &eventData)

        errorDetails := eventData["error_details"].(map[string]interface{})
        Expect(errorDetails["code"]).To(Equal("ERR_K8S_CREATE_FAILED"))
        Expect(errorDetails["retry_possible"]).To(BeTrue(), "K8s errors are transient")
    })

    It("should map unknown errors to ERR_INTERNAL_ORCHESTRATION", func() {
        event, err := manager.BuildFailureEvent(
            "correlation-003",
            "default",
            "rr-test",
            "unknown",
            "unexpected panic in reconciler",
            1000,
        )
        Expect(err).ToNot(HaveOccurred())

        eventDataBytes, _ := json.Marshal(event.EventData)
        var eventData map[string]interface{}
        _ = json.Unmarshal(eventDataBytes, &eventData)

        errorDetails := eventData["error_details"].(map[string]interface{})
        Expect(errorDetails["code"]).To(Equal("ERR_INTERNAL_ORCHESTRATION"))
        Expect(errorDetails["retry_possible"]).To(BeTrue(), "Default to retryable")
    })

    It("should map 'not found' errors to ERR_K8S_CREATE_FAILED", func() {
        event, err := manager.BuildFailureEvent(
            "correlation-004",
            "default",
            "rr-test",
            "workflow_execution",
            "WorkflowExecution not found after creation",
            1000,
        )
        Expect(err).ToNot(HaveOccurred())

        eventDataBytes, _ := json.Marshal(event.EventData)
        var eventData map[string]interface{}
        _ = json.Unmarshal(eventDataBytes, &eventData)

        errorDetails := eventData["error_details"].(map[string]interface{})
        Expect(errorDetails["code"]).To(Equal("ERR_K8S_CREATE_FAILED"))
        Expect(errorDetails["retry_possible"]).To(BeTrue())
    })
})
```

---

## 📊 **Coverage After Changes**

| Error Code | Unit Test | Integration Test | Total |
|------------|-----------|------------------|-------|
| `ERR_INVALID_TIMEOUT_CONFIG` | ✅ | ✅ Scenario 1 | ✅✅ |
| `ERR_TIMEOUT_REMEDIATION` | ✅ | - | ✅ |
| `ERR_INVALID_CONFIG` | ✅ NEW | - | ✅ |
| `ERR_K8S_CREATE_FAILED` | ✅ NEW | - | ✅ |
| `ERR_INTERNAL_ORCHESTRATION` | ✅ NEW | - | ✅ |

**Result**: 100% error code coverage with **simple, maintainable tests**

---

## ⏱️ **Implementation Estimate**

| Task | Effort | Priority |
|------|--------|----------|
| Delete Scenario 2 | 5 min | HIGH |
| Add unit tests (4 new tests) | 30 min | HIGH |
| Run unit tests | 2 min | HIGH |
| Run integration tests | 3 min | HIGH |
| **Total** | **40 min** | **HIGH** |

---

## ✅ **Success Criteria**

After implementation:
- ✅ All 5 error codes have unit test coverage
- ✅ Integration tests pass without placeholder failures
- ✅ Test suite runs faster (no complex K8s error simulation)
- ✅ Maintainability improved (unit tests > integration tests)
- ✅ BR-AUDIT-005 Gap #7 fully implemented and tested

---

## 📚 **Design Decision**

**DD-TEST-008**: Error Code Mapping Belongs in Unit Tests

**Context**: Gap #7 Scenario 2 attempts to test error code mapping via K8s API error simulation

**Decision**: Test error code mapping logic in **unit tests**, not integration tests

**Rationale**:
1. **Business Logic Isolation**: Error code mapping is pure function (string → enum)
2. **Complexity Avoidance**: K8s API error simulation requires complex infrastructure
3. **Test Pyramid**: Unit tests (70%+) should cover business logic
4. **Integration Value**: Integration test Scenario 1 already validates end-to-end flow
5. **Maintainability**: Unit tests are faster, simpler, and more reliable

**Consequences**:
- ✅ Reduced integration test complexity
- ✅ Faster test execution (no K8s error simulation setup)
- ✅ Better alignment with test pyramid principles
- ✅ 100% error code coverage with maintainable tests

---

**Status**: ✅ **Recommendation Ready for Implementation**
**Next Action**: User approval to proceed with changes
**Confidence**: 95% - Standard test tier refactoring
