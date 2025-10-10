# Ginkgo Table-Driven Test Refactoring Triage

**Date**: October 9, 2025
**Scope**: All new tests created in current session
**Goal**: Identify tests that should be refactored to use Ginkgo's `DescribeTable`/`Entry` pattern

---

## 📊 **Test Files Created This Session**

| File | Lines | Tests | Status |
|------|-------|-------|--------|
| `test/unit/remediation/timeout_helpers_test.go` | 104 | 15 | ✅ **Already refactored** |
| `test/integration/remediation/controller_orchestration_test.go` | 507 | 5 | ⚠️ **Needs analysis** |
| `test/unit/remediation/suite_test.go` | 27 | 0 | ✅ **Suite setup only** |

---

## 🔍 **Triage Analysis**

### ✅ **File 1: `test/unit/remediation/timeout_helpers_test.go`**

**Status**: **Already refactored with table-driven tests**

**Pattern**: Tests pure helper function `IsPhaseTimedOut()` with multiple input combinations

**Refactoring Result**:
- **Before**: 218 lines, 12 individual `It()` blocks
- **After**: 104 lines, 15 test cases (1 edge case + 14 table entries)
- **Reduction**: 54% code reduction
- **Benefit**: Much easier to add new test cases

**Table Structure**:
```go
DescribeTable("phase timeout detection",
    func(phase string, elapsed time.Duration, expectedTimeout bool) {
        // Test logic
    },
    Entry("pending phase: TIMEOUT after 1 minute", "pending", 1*time.Minute, true),
    Entry("processing phase: NO timeout at 2 minutes", "processing", 2*time.Minute, false),
    // ... 12 more entries
)
```

**Why this worked**:
- ✅ Pure function with clear input → output mapping
- ✅ High repetition in test structure (same assertions, different data)
- ✅ Simple parameterization (phase, elapsed time, expected result)

---

### ⚠️ **File 2: `test/integration/remediation/controller_orchestration_test.go`**

**Status**: **NOT suitable for table-driven tests**

**Test Structure**:
```go
Describe("Task 1.1: AIAnalysis CRD Creation", func() {
    It("should create AIAnalysis CRD when RemediationProcessing phase is 'completed'", ...)
    It("should include enriched context from RemediationProcessing in AIAnalysis spec", ...)
    It("should NOT create AIAnalysis CRD when RemediationProcessing phase is 'enriching'", ...)
})

Describe("Task 1.2: WorkflowExecution CRD Creation", func() {
    It("should create WorkflowExecution CRD when AIAnalysis phase is 'completed'", ...)
    It("should NOT create WorkflowExecution CRD when AIAnalysis phase is 'Analyzing'", ...)
})
```

**Analysis**: These are **integration tests** testing full controller reconciliation loops

**Why table-driven tests DON'T work here**:

#### ❌ **Reason 1: Complex, unique setup for each test**
Each test has different setup requirements:
- Test 1: Creates RemediationRequest, lets controller create RemediationProcessing, updates status
- Test 2: Same as Test 1 but waits for RemediationProcessing creation first
- Test 3: Creates RemediationRequest with RemediationProcessing in "enriching" state
- Test 4: Creates RemediationRequest, AIAnalysis, updates AIAnalysis status
- Test 5: Creates RemediationRequest, AIAnalysis in "Analyzing" state

**Code snippet** (Test 1 setup):
```go
// 85 lines of unique setup
remediationRequest := &remediationv1alpha1.RemediationRequest{...}
k8sClient.Create(ctx, remediationRequest)

// Wait for controller to create RemediationProcessing
Eventually(func() error {
    return k8sClient.Get(ctx, ..., remediationProcessing)
}, timeout, interval).Should(Succeed())

remediationProcessing.Status.Phase = "completed"
k8sClient.Status().Update(ctx, remediationProcessing)
```

This setup **cannot be parameterized** easily because each test has different orchestration steps.

#### ❌ **Reason 2: Different assertions for each test**

- Test 1: Asserts AIAnalysis created, has correct parent ref, has owner references
- Test 2: Asserts AIAnalysis has enriched context data from RemediationProcessing
- Test 3: Asserts AIAnalysis NOT created (negative test)
- Test 4: Asserts WorkflowExecution created with correct workflow definition
- Test 5: Asserts WorkflowExecution NOT created (negative test)

**These assertions are fundamentally different** and cannot be unified into a single table-driven test function.

#### ❌ **Reason 3: Testing different behaviors, not variations of same behavior**

Table-driven tests work best when testing **the same behavior with different inputs**.

These tests are testing **different behaviors**:
- Test 1-2: "Controller creates child CRD when parent completes"
- Test 3: "Controller does NOT create child CRD when parent not completed"
- Test 4: "Controller creates WorkflowExecution with AI recommendations"
- Test 5: "Controller does NOT create WorkflowExecution when AIAnalysis not completed"

Each test validates a **different requirement** with **different success criteria**.

#### ❌ **Reason 4: Integration tests test workflows, not pure functions**

**Integration tests** should test **user journeys and workflows**, not data variations:
- Test 1: "Happy path: RemediationProcessing → AIAnalysis"
- Test 2: "Happy path with context enrichment"
- Test 3: "Negative case: Don't create AIAnalysis too early"
- Test 4: "Happy path: AIAnalysis → WorkflowExecution"
- Test 5: "Negative case: Don't create WorkflowExecution too early"

These are **distinct scenarios**, not **parameter variations** of the same scenario.

---

## 📋 **When to Use Table-Driven Tests**

### ✅ **GOOD Candidates for DescribeTable**

Table-driven tests work best for:

| Pattern | Example | Why It Works |
|---------|---------|--------------|
| **Pure function testing** | `IsPhaseTimedOut(phase, elapsed)` | Clear input → output mapping |
| **Validation functions** | `ValidateSignalFingerprint(fingerprint)` | Test valid/invalid inputs |
| **Data transformation** | `MapRemediationToProcessing(rr)` | Same logic, different data |
| **Edge case testing** | Boundary conditions (null, empty, max) | Systematic edge case coverage |
| **Error handling** | Different error scenarios | Same error handling, different triggers |

**Characteristics**:
- ✅ High repetition in test structure
- ✅ Same assertions, different inputs
- ✅ Pure functions or simple methods
- ✅ Minimal setup variation
- ✅ Clear parameterization

### ❌ **BAD Candidates for DescribeTable**

Keep as individual `It()` blocks when:

| Anti-Pattern | Example | Why It Doesn't Work |
|--------------|---------|---------------------|
| **Complex setup** | Integration tests with multiple CRD creations | Setup cannot be parameterized |
| **Different assertions** | Some tests check creation, others check non-creation | Cannot unify into single test function |
| **Testing workflows** | Multi-step orchestration tests | Each test is a distinct user journey |
| **State-dependent** | Tests that depend on controller reconciliation | Timing and state make parameterization hard |
| **Different behaviors** | Positive vs negative tests | Fundamentally different validation logic |

**Characteristics**:
- ❌ Low repetition (each test is unique)
- ❌ Different assertions per test
- ❌ Complex, varied setup
- ❌ Testing different behaviors
- ❌ Integration/E2E tests

---

## 🎯 **Refactoring Guidelines**

### **Step 1: Identify Candidates**

Ask these questions:

1. **Is this testing a pure function or helper method?**
   - ✅ Yes → Good candidate
   - ❌ No → Keep as individual test

2. **Do all tests have the same assertion pattern?**
   - ✅ Yes → Good candidate
   - ❌ No → Keep as individual test

3. **Can the setup be parameterized with 3-5 variables?**
   - ✅ Yes → Good candidate
   - ❌ No → Keep as individual test

4. **Are you testing input variations, not behavior variations?**
   - ✅ Yes → Good candidate
   - ❌ No → Keep as individual test

### **Step 2: Refactor Pattern**

```go
// BEFORE: Individual It() blocks
It("should timeout processing after 6 minutes", func() {
    remediation := createRemediation("processing", 6*time.Minute)
    Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeTrue())
})

It("should NOT timeout processing at 2 minutes", func() {
    remediation := createRemediation("processing", 2*time.Minute)
    Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
})

// AFTER: Table-driven
DescribeTable("phase timeout detection",
    func(phase string, elapsed time.Duration, expectedTimeout bool) {
        remediation := createRemediation(phase, elapsed)
        Expect(reconciler.IsPhaseTimedOut(remediation)).To(Equal(expectedTimeout))
    },
    Entry("processing: TIMEOUT after 6 minutes", "processing", 6*time.Minute, true),
    Entry("processing: NO timeout at 2 minutes", "processing", 2*time.Minute, false),
)
```

### **Step 3: Validate Readability**

After refactoring, ask:
- ✅ Is it easier to understand?
- ✅ Is it easier to add new test cases?
- ✅ Did we eliminate significant duplication?

If not, **keep the original structure**.

---

## 📊 **Session Summary**

| Metric | Count |
|--------|-------|
| **Test files created** | 3 |
| **Test files suitable for table-driven** | 1 (33%) |
| **Test files already refactored** | 1 |
| **Lines of test code saved** | 114 lines (54% reduction) |
| **Integration test files** | 1 (not suitable) |

---

## ✅ **Recommendations**

### **For Current Tests**

1. **`timeout_helpers_test.go`**: ✅ **Already refactored** - excellent example
2. **`controller_orchestration_test.go`**: ⚠️ **Keep as-is** - integration tests should remain individual scenarios

### **For Future Tests**

When writing new tests:

1. **Pure helper functions** → Start with `DescribeTable` from the beginning
2. **Integration tests** → Write as individual `It()` blocks for each scenario
3. **Validation functions** → Use `DescribeTable` for edge cases and input variations
4. **Controller methods** → If testing logic in isolation → `DescribeTable`; if testing reconciliation → individual tests

### **General Rule**

> **"If you find yourself copy-pasting an `It()` block and only changing 2-3 values, use `DescribeTable`. If each test is structurally different, keep them separate."**

---

## 🔗 **References**

- **Ginkgo Documentation**: https://onsi.github.io/ginkgo/#table-specs
- **Example**: `test/unit/remediation/timeout_helpers_test.go` (lines 64-101)
- **Best Practice**: Keep integration tests as narratives, refactor unit tests as tables

---

## 📈 **Future Opportunities**

When Phase 2.2 (failure recovery) and Phase 3 (metrics/events) are implemented, look for these patterns:

### **Good Candidates for Table-Driven Tests**

1. **Failure classification functions**:
   ```go
   DescribeTable("failure classification",
       func(errorType string, retryable bool, backoffDuration time.Duration),
       Entry("transient error: retryable", "NetworkTimeout", true, 1*time.Minute),
       Entry("permanent error: not retryable", "InvalidSpec", false, 0),
   )
   ```

2. **Metric validation functions**:
   ```go
   DescribeTable("metric value validation",
       func(metricName string, value float64, valid bool),
       Entry("phase_duration: valid", "phase_duration_seconds", 120.5, true),
       Entry("phase_duration: negative invalid", "phase_duration_seconds", -1.0, false),
   )
   ```

3. **Event message formatting**:
   ```go
   DescribeTable("event message formatting",
       func(phase string, reason string, expectedMessage string),
       Entry("timeout in processing", "processing", "Timeout", "Processing phase exceeded 5m timeout"),
       Entry("timeout in analyzing", "analyzing", "Timeout", "Analyzing phase exceeded 10m timeout"),
   )
   ```

---

**Conclusion**: The table-driven refactoring for `timeout_helpers_test.go` was the correct approach and saved 114 lines of code while improving maintainability. The integration tests should remain as individual scenarios because they test different workflows, not input variations.

