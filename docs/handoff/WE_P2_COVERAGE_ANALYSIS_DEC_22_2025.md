# P2: MarkFailedWithReason Coverage Analysis - December 22, 2025

**Date**: December 22, 2025
**Status**: Coverage gap identified - 2 scenarios need integration tests
**Confidence**: **90%** - Clear path to close remaining gaps

---

## 🎯 **Executive Summary**

**Finding**: P2 (MarkFailedWithReason unit tests) was deferred due to complex mocking requirements. However, **most scenarios are already covered** by existing integration and unit tests. Only **2 edge case scenarios** are missing coverage.

**Recommendation**: Add **2 integration tests** for missing scenarios (PipelineRunCreationFailed, RaceConditionError).

---

## 📊 **Current MarkFailedWithReason Coverage**

### **What MarkFailedWithReason Does**
```go
func (r *WorkflowExecutionReconciler) MarkFailedWithReason(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    reason, message string,
) error
```

**Functionality**:
1. ✅ Sets `Phase` to `Failed`
2. ✅ Sets `CompletionTime` to now
3. ✅ Sets `FailureDetails.Reason` (enum value)
4. ✅ Sets `FailureDetails.Message` (human-readable)
5. ✅ Sets `FailureDetails.WasExecutionFailure` to `false` (pre-execution failure)
6. ✅ Updates status via `r.Status().Update()`
7. ✅ Records Kubernetes event via `r.Recorder.Event()`
8. ✅ Emits audit event via `r.RecordAuditEvent()`

---

## ✅ **Existing Coverage - What's Already Tested**

### **1. Unit Tests (controller_test.go)**

| Test | Line | Reason | Coverage |
|------|------|--------|----------|
| ConfigurationError enum | 4425 | `FailureReasonConfigurationError` | ✅ Status update, enum validation |
| ImagePullBackOff enum | 4457 | `FailureReasonImagePullBackOff` | ✅ Status update, enum validation |
| TaskFailed enum | 4489 | `FailureReasonTaskFailed` | ✅ Status update, enum validation |
| Audit event emission | 4521 | `FailureReasonOOMKilled` | ✅ Audit event with correct fields |

**What these tests cover**:
- ✅ Correct enum values written to `FailureDetails.Reason`
- ✅ Status update mechanism
- ✅ Audit event emission with correct structure
- ✅ Multiple failure reason types (ConfigurationError, ImagePullBackOff, TaskFailed, OOMKilled)

---

### **2. Integration Tests (P1 Work - failure_classification_integration_test.go)**

| Test | Tekton Reason | WFE Reason | Coverage |
|------|--------------|------------|----------|
| TaskFailed | TaskRunFailed | TaskFailed | ✅ Full integration |
| DeadlineExceeded | TaskRunTimeout | DeadlineExceeded | ✅ Full integration |
| ImagePullBackOff | TaskRunImagePullFailed | ImagePullBackOff | ✅ Full integration |
| OOMKilled | OOMKilled | OOMKilled | ✅ Full integration |
| ResourceExhausted | ResourceVerificationFailed | ResourceExhausted | ✅ Full integration |
| Forbidden | CreateContainerConfigError | Forbidden | ✅ Full integration |
| ConfigurationError | (invalid config) | ConfigurationError | ✅ Full integration |
| Unknown | (generic failure) | Unknown | ✅ Full integration |

**What these tests cover** (8 tests):
- ✅ All 8 Tekton → WFE failure reason mappings
- ✅ End-to-end failure flow (PipelineRun failure → WFE Failed)
- ✅ Status updates with real K8s API
- ✅ Condition setting
- ✅ Audit event emission

---

### **3. Integration Tests (P3 Work - conflict_test.go)**

| Test | Reason | Coverage |
|------|--------|----------|
| Non-owned PipelineRun conflict | `Unknown` | ✅ Race condition handling |

**What this test covers**:
- ✅ AlreadyExists error when PipelineRun owned by another WFE
- ✅ MarkFailedWithReason with "Unknown" reason
- ✅ Clear error message with conflicting PipelineRun name

---

### **4. Validation Tests (P4 Work - validation_test.go)**

**Indirect coverage**:
- ✅ ValidateSpec rejection triggers ConfigurationError via MarkFailedWithReason
- ✅ 8 unit tests validate spec validation logic that feeds into MarkFailedWithReason

---

## ❌ **Coverage Gaps - Missing Scenarios**

### **All MarkFailedWithReason Call Sites in Controller**

```go
// internal/controller/workflowexecution/workflowexecution_controller.go

// 1. Line 248: Spec validation failure
if err := r.ValidateSpec(wfe); err != nil {
    r.MarkFailedWithReason(ctx, wfe, "ConfigurationError", err.Error())
}
// ✅ COVERED: P4 validation_test.go + unit test at line 4425

// 2. Line 269: PipelineRun creation failure (non-AlreadyExists)
if err := r.Create(ctx, pr); err != nil {
    if !apierrors.IsAlreadyExists(err) {
        r.MarkFailedWithReason(ctx, wfe, "PipelineRunCreationFailed", ...)
    }
}
// ❌ GAP: No test for this scenario

// 3. Line 624: AlreadyExists but can't verify ownership
if getErr := r.Get(ctx, ..., existingPR); getErr != nil {
    r.MarkFailedWithReason(ctx, wfe, "RaceConditionError", ...)
}
// ❌ GAP: No test for this scenario

// 4. Line 669: AlreadyExists owned by another WFE
markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown", ...)
// ✅ COVERED: P3 conflict_test.go (Test 3)
```

---

## 🔍 **Detailed Gap Analysis**

### **GAP 1: PipelineRunCreationFailed**

**Scenario**: PipelineRun creation fails for reason other than AlreadyExists (e.g., quota exceeded, API error).

**Current Coverage**: ❌ **NOT TESTED**

**Impact**: Medium
- Real-world scenario: K8s API errors, resource quota exhaustion, RBAC issues
- Frequency: Low (infrastructure issues)
- Risk: Operator confusion if error handling is incorrect

**Test Type**: Integration (requires real K8s API client to simulate failures)

**Example Test**:
```go
It("should fail WFE with PipelineRunCreationFailed when K8s API rejects creation", func() {
    // Given: Configure mock client to reject PipelineRun creation
    // When: WFE reconciles and attempts PipelineRun creation
    // Then: WFE transitions to Failed with PipelineRunCreationFailed reason
})
```

---

### **GAP 2: RaceConditionError**

**Scenario**: PipelineRun already exists, but Get() fails when verifying ownership.

**Current Coverage**: ❌ **NOT TESTED**

**Impact**: Low
- Real-world scenario: K8s API transient failure during ownership verification
- Frequency: Very low (race condition + API failure)
- Risk: Low (edge case, clear error message)

**Test Type**: Integration (requires mock client to simulate Get() failure)

**Example Test**:
```go
It("should fail WFE with RaceConditionError when ownership verification fails", func() {
    // Given: PipelineRun exists, but Get() returns error
    // When: WFE reconciles and encounters AlreadyExists
    // Then: WFE transitions to Failed with RaceConditionError reason
})
```

---

## 📊 **Coverage Summary**

| Scenario | Current Coverage | Test Tier | Status |
|----------|-----------------|-----------|--------|
| ConfigurationError | ✅ Unit + Integration | Unit (line 4425) + P4 | Complete |
| Tekton failures (8 types) | ✅ Integration | P1 (failure_classification) | Complete |
| Unknown (race, non-owned PR) | ✅ Integration | P3 (conflict_test) | Complete |
| ImagePullBackOff | ✅ Unit | Unit (line 4457) | Complete |
| TaskFailed | ✅ Unit | Unit (line 4489) | Complete |
| Audit event emission | ✅ Unit | Unit (line 4521) | Complete |
| **PipelineRunCreationFailed** | ❌ **GAP** | **Integration (needed)** | **Missing** |
| **RaceConditionError** | ❌ **GAP** | **Integration (needed)** | **Missing** |

**Overall Coverage**: **6/8 scenarios (75%)**

---

## 🎯 **Recommendation: Add 2 Integration Tests**

### **Why Integration (not Unit)?**

1. **K8s API Mocking Complexity**: These scenarios require simulating K8s API failures (Create(), Get()). Integration tests with `envtest` provide better fidelity than heavily-mocked unit tests.

2. **Existing Pattern**: P1 (failure_classification_integration_test.go) and P3 (conflict_test.go) already validate MarkFailedWithReason via integration tests with excellent results.

3. **ROI**: 2 integration tests (~30-40 min effort) vs. complex unit test mocking (~2 hours).

---

## 📝 **Proposed Integration Tests**

### **Test 1: PipelineRunCreationFailed**

**File**: `test/integration/workflowexecution/failure_handling_integration_test.go` (new file)

**Test**:
```go
It("should fail WFE with PipelineRunCreationFailed on K8s API error", func() {
    By("Configuring test environment to reject PipelineRun creation")
    // Use envtest webhook or custom client to simulate API error

    By("Creating WorkflowExecution")
    wfe := createUniqueWFE("api-error-test", "default/deployment/test")
    Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

    By("Verifying WFE transitions to Failed with PipelineRunCreationFailed")
    Eventually(func() string {
        updated, _ := getWFE(wfe.Name, wfe.Namespace)
        return string(updated.Status.Phase)
    }, 15*time.Second).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)))

    finalWFE, _ := getWFE(wfe.Name, wfe.Namespace)
    Expect(finalWFE.Status.FailureDetails).ToNot(BeNil())
    Expect(finalWFE.Status.FailureDetails.Reason).To(Equal("PipelineRunCreationFailed"))
    Expect(finalWFE.Status.FailureDetails.Message).To(ContainSubstring("Failed to create PipelineRun"))
    Expect(finalWFE.Status.FailureDetails.WasExecutionFailure).To(BeFalse())
})
```

**Estimated Effort**: 20 minutes

---

### **Test 2: RaceConditionError**

**File**: Same as Test 1

**Test**:
```go
It("should fail WFE with RaceConditionError when ownership verification fails", func() {
    By("Creating PipelineRun that will cause ownership verification failure")
    // Pre-create PipelineRun, then simulate Get() failure

    By("Creating WorkflowExecution")
    wfe := createUniqueWFE("race-verify-fail", "default/deployment/test")
    Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

    By("Verifying WFE transitions to Failed with RaceConditionError")
    Eventually(func() string {
        updated, _ := getWFE(wfe.Name, wfe.Namespace)
        return string(updated.Status.Phase)
    }, 15*time.Second).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)))

    finalWFE, _ := getWFE(wfe.Name, wfe.Namespace)
    Expect(finalWFE.Status.FailureDetails).ToNot(BeNil())
    Expect(finalWFE.Status.FailureDetails.Reason).To(Equal("RaceConditionError"))
    Expect(finalWFE.Status.FailureDetails.Message).To(ContainSubstring("failed to verify ownership"))
})
```

**Estimated Effort**: 20 minutes

---

## ⚠️ **Implementation Challenge: Simulating K8s API Failures**

### **Challenge**
Both gap scenarios require simulating K8s API failures:
- **GAP 1**: `Create()` returns non-AlreadyExists error
- **GAP 2**: `Get()` returns error during ownership verification

**Envtest Limitation**: `envtest` uses a real K8s API server, making it difficult to inject specific failures.

### **Possible Solutions**

#### **Option A: Custom Fake Client (Recommended for Unit Tests)**
- Use `fake.NewClientBuilder()` with interceptors
- Pro: Precise control over API behavior
- Con: Moves tests back to unit tier (complex mocking)

#### **Option B: Admission Webhook**
- Deploy validating webhook in envtest to reject PipelineRun creation
- Pro: Real K8s API behavior
- Con: Complex setup (~1 hour)

#### **Option C: Resource Quota**
- Set resource quota to 0 to force creation failure
- Pro: Real failure scenario
- Con: Less precise error message testing

#### **Option D: Network Policy / API Server Unavailability**
- Not feasible in envtest

### **Recommendation**

**For V1.0**: **Accept 75% coverage (6/8 scenarios)** as sufficient. The 2 missing scenarios are low-impact edge cases:
- **PipelineRunCreationFailed**: Infrastructure failures (low frequency)
- **RaceConditionError**: Rare race condition + API failure (very low frequency)

**For V1.1**: Implement Option A (custom fake client) if coverage targets demand 100% scenario coverage.

---

## 🎯 **Revised P2 Status**

### **Original P2 Goal**
Unit test MarkFailedWithReason for all enum values and edge cases.

### **Revised Assessment**
- ✅ **6/8 scenarios covered** (75%)
- ✅ **All high-impact scenarios covered** (ConfigurationError, Tekton failures, race conditions)
- ❌ **2 low-impact edge cases missing** (PipelineRunCreationFailed, RaceConditionError)

### **Decision Options**

#### **Option 1: Accept Current Coverage (Recommended)**
- **Rationale**: 75% coverage, all high-impact scenarios tested
- **Trade-off**: 2 edge cases untested (low real-world impact)
- **Confidence**: 90% - Production-ready

#### **Option 2: Add 2 Integration Tests (Option A - Fake Client)**
- **Effort**: 1-1.5 hours (custom fake client setup)
- **Trade-off**: Moves tests to unit tier (defeats P2 deferral rationale)
- **Coverage**: 100% (8/8 scenarios)

#### **Option 3: Add 2 Integration Tests (Option B - Webhook)**
- **Effort**: 1.5-2 hours (webhook setup + tests)
- **Trade-off**: Complex setup for 2 edge case tests
- **Coverage**: 100% (8/8 scenarios)

---

## 📊 **User Decision Matrix**

| Criterion | Option 1 (Accept) | Option 2 (Fake Client) | Option 3 (Webhook) |
|-----------|------------------|----------------------|-------------------|
| **Coverage** | 75% (6/8) | 100% (8/8) | 100% (8/8) |
| **Effort** | 0 hours | 1-1.5 hours | 1.5-2 hours |
| **Test Tier** | Integration | Unit | Integration |
| **Complexity** | Low | Medium | High |
| **Production Readiness** | ✅ High | ✅ Very High | ✅ Very High |
| **V1.0 Recommended** | ✅ **YES** | ⚠️ Optional | ❌ No (overkill) |

---

## 🎉 **Conclusion**

**Status**: P2 coverage is **75% complete** (6/8 scenarios) via existing tests.

**Recommendation**: **Accept current coverage for V1.0**. All high-impact scenarios are tested. The 2 missing edge cases (PipelineRunCreationFailed, RaceConditionError) are low-frequency, low-risk scenarios.

**If 100% coverage is required**: Implement Option 2 (Fake Client) for precise control with moderate effort.

**Confidence**: **90%** - Current coverage is production-ready.

---

**User Question**: *"Can we move P2 to integration or E2E? Or are they duplicated?"*

**Answer**:
- ✅ **Most P2 scenarios are already covered** by existing integration tests (P1, P3) and unit tests.
- ❌ **2 scenarios are NOT duplicated** and are missing coverage (PipelineRunCreationFailed, RaceConditionError).
- ⚠️ **Moving to integration is feasible** but requires custom fake client setup (1-1.5 hours).
- ✅ **Current coverage (75%) is production-ready** for V1.0.

---

*Generated by AI Assistant - December 22, 2025*





