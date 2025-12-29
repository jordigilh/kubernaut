# Triage: Consecutive Failure Unit Tests - Testing Guidelines Violations

**Date**: December 13, 2025
**File**: `test/unit/remediationorchestrator/consecutive_failure_test.go`
**Status**: ✅ **RESOLVED** - All violations fixed, tests passing
**Resolution**: See [`REFACTOR_CONSECUTIVE_FAILURE_TESTS_COMPLETE.md`](REFACTOR_CONSECUTIVE_FAILURE_TESTS_COMPLETE.md)
**References**:
- [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)
- [WorkflowExecution testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md)

---

## 📊 Summary

The consecutive failure unit tests violate **3 critical testing standards**:

| Violation | Severity | Impact | Lines Affected |
|-----------|----------|--------|----------------|
| **1. BR Prefix Misuse** | 🔴 CRITICAL | Confuses unit tests with business tests | All (60-732) |
| **2. No Table-Driven Tests** | 🟠 HIGH | Code duplication, maintenance burden | 120-587 |
| **3. AC-* Structure** | 🟡 MEDIUM | Wrong focus for unit tests | All describe blocks |

---

## 🚨 Violation #1: BR Prefix Misuse in Unit Tests

### **Rule Violated**

Per [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md):

> **Unit Tests Must:**
> - Focus on implementation correctness
> - NOT use BR-* prefixes (those are for business tests)
> - Test function/method behavior, not business outcomes

**BR prefixes are ONLY for E2E/Business Requirement tests**, which:
- Validate business value delivery
- Test end-to-end outcomes
- Measure business KPIs (SLAs, efficiency, cost)
- Live in `test/e2e/` or `test/business-requirements/`

### **Current Violations**

```go
// ❌ WRONG: BR prefix in unit test file
var _ = Describe("BR-ORCH-042: Consecutive Failure Blocking", func() {
    // Line 60
})

// ❌ WRONG: BR prefix in header comment
// BR-ORCH-042: Consecutive Failure Blocking with Automatic Cooldown
// Lines 38-58
```

**Impact**:
- Confuses developers about test purpose
- Makes it appear these are business requirement tests when they're not
- Violates naming conventions across the codebase

### **Correct Pattern (from WorkflowExecution)**

```go
// ✅ CORRECT: Unit test without BR prefix
var _ = Describe("ConsecutiveFailureBlocker", func() {
    Describe("CountConsecutiveFailures", func() {
        // Focus on method behavior, not BR
    })
})
```

**Reference**: [workflowexecution/testing-strategy.md lines 256-334](../services/crd-controllers/03-workflowexecution/testing-strategy.md)

---

## 🟠 Violation #2: No Table-Driven Tests

### **Rule Violated**

Per [WorkflowExecution testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md):

**Table-driven tests** are the standard for testing multiple scenarios with similar logic.

### **Current Violations**

#### **Example 1: Threshold Testing (Lines 285-373)**

```go
// ❌ WRONG: Repeated test logic for each scenario
It("should set BlockedUntil to now + cooldown duration", func() {
    // Given: 3 consecutive failures
    for i := 0; i < 3; i++ {
        rr := &remediationv1.RemediationRequest{...}
        Expect(fakeClient.Create(ctx, rr)).To(Succeed())
    }
    // ... test logic
})

It("should set BlockReason to consecutive_failures_exceeded", func() {
    // Given: 3 consecutive failures (REPEATED)
    for i := 0; i < 3; i++ {
        rr := &remediationv1.RemediationRequest{...}
        Expect(fakeClient.Create(ctx, rr)).To(Succeed())
    }
    // ... test logic
})
```

**Problem**: The setup code for "3 consecutive failures" is duplicated across multiple tests.

#### **Example 2: Edge Cases (Lines 587-707)**

```go
// ❌ WRONG: Individual tests for each threshold value
It("should not block if only 2 consecutive failures", func() {
    // Given: Only 2 consecutive failures
    for i := 0; i < 2; i++ { ... }
    // Then: Should NOT block
})

It("should handle different fingerprints independently", func() {
    // Given: Fingerprint A has 3 failures, Fingerprint B has 1 failure
    for i := 0; i < 3; i++ { ... } // Fingerprint A
    // ... Fingerprint B
})
```

**Problem**: Similar test patterns that should be parameterized.

### **Correct Pattern (from WorkflowExecution blocking_test.go)**

```go
// ✅ CORRECT: Table-driven test with DescribeTable
DescribeTable("should block signal when threshold is reached",
    func(failureCount int, shouldBlock bool, description string) {
        // Given: Create N consecutive failures
        for i := 0; i < failureCount; i++ {
            createFailedRR(ctx, fingerprint)
        }

        // When: Check if blocking should occur
        count, err := blocker.CountConsecutiveFailures(ctx, fingerprint)
        Expect(err).ToNot(HaveOccurred())

        // Then: Verify blocking decision
        if shouldBlock {
            Expect(count).To(BeNumerically(">=", 3))
        } else {
            Expect(count).To(BeNumerically("<", 3))
        }
    },
    Entry("1 failure - no block", 1, false, "Below threshold"),
    Entry("2 failures - no block", 2, false, "Below threshold"),
    Entry("3 failures - BLOCK (AC-042-1-1)", 3, true, "At threshold"),
    Entry("4 failures - BLOCK", 4, true, "Above threshold"),
    Entry("10 failures - BLOCK", 10, true, "Well above threshold"),
)
```

**Benefits**:
- ✅ Single test logic, multiple scenarios
- ✅ Clear tabular structure shows all cases
- ✅ Easy to add new scenarios
- ✅ Reduces code duplication by ~60%

**Reference**: [blocking_test.go lines 214-220](../../test/unit/remediationorchestrator/blocking_test.go) (if exists)

---

## 🟡 Violation #3: Acceptance Criteria Structure

### **Rule Violated**

Per [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md):

> **Unit Tests Focus On:**
> - Function/method behavior
> - Error handling & edge cases
> - Internal logic validation
> - Interface compliance

**NOT on acceptance criteria (AC-*), which are for business requirement tests.**

### **Current Violations**

```go
// ❌ WRONG: AC-* structure in unit tests
Describe("AC-042-1: Consecutive Failure Detection", func() {
    Context("AC-042-1-1: Count consecutive Failed RRs for same fingerprint", func() {
        // Line 120-121
    })
    Context("AC-042-1-2: Count resets on any Completed RR", func() {
        // Line 179
    })
})
```

**Problem**:
- AC-* labels belong in **business requirement tests** (E2E tests)
- Unit tests should focus on **method behavior**, not AC tracking

### **Correct Pattern**

```go
// ✅ CORRECT: Method-focused structure
Describe("ConsecutiveFailureBlocker", func() {
    Describe("CountConsecutiveFailures", func() {
        Context("when multiple failures exist for fingerprint", func() {
            It("should count only consecutive failures", func() {
                // Test method behavior
            })
        })

        Context("when Completed RR interrupts sequence", func() {
            It("should reset count at Completed RR", func() {
                // Test method behavior
            })
        })
    })

    Describe("BlockIfNeeded", func() {
        Context("when threshold is met", func() {
            DescribeTable("should block based on failure count",
                func(count int, shouldBlock bool) { ... },
                Entry("1 failure", 1, false),
                Entry("3 failures", 3, true),
            )
        })
    })
})
```

**Reference**: [workflowexecution/controller_test.go lines 256-334](../services/crd-controllers/03-workflowexecution/testing-strategy.md)

---

## 🔧 Recommended Refactoring

### **Priority 1: Remove BR Prefix (CRITICAL)**

```diff
- var _ = Describe("BR-ORCH-042: Consecutive Failure Blocking", func() {
+ var _ = Describe("ConsecutiveFailureBlocker", func() {

- // BR-ORCH-042: Consecutive Failure Blocking with Automatic Cooldown
+ // Consecutive Failure Blocking - Unit Tests for ConsecutiveFailureBlocker
+ //
+ // Business Context: Implements BR-ORCH-042 (see docs/requirements/)
+ // Test Focus: Implementation correctness (method behavior, edge cases)
```

**Rationale**: Unit tests validate **implementation**, not business requirements.

---

### **Priority 2: Add Table-Driven Tests (HIGH)**

#### **Refactor: Threshold Decision Tests**

```go
// ✅ CORRECT: Replace individual tests with DescribeTable
Describe("CountConsecutiveFailures", func() {
    DescribeTable("should count consecutive failures correctly",
        func(setup func(ctx context.Context, fingerprint string), expectedCount int) {
            // When: Execute setup
            setup(ctx, fingerprint)

            // Then: Verify count
            count, err := consecutiveBlock.CountConsecutiveFailures(ctx, fingerprint)
            Expect(err).ToNot(HaveOccurred())
            Expect(count).To(Equal(expectedCount))
        },
        Entry("no failures", func(ctx context.Context, fp string) {
            // No setup needed
        }, 0),
        Entry("1 failure", func(ctx context.Context, fp string) {
            createFailedRR(ctx, fp)
        }, 1),
        Entry("3 consecutive failures", func(ctx context.Context, fp string) {
            createFailedRR(ctx, fp)
            createFailedRR(ctx, fp)
            createFailedRR(ctx, fp)
        }, 3),
        Entry("failures interrupted by Completed", func(ctx context.Context, fp string) {
            createFailedRR(ctx, fp)  // Oldest
            createFailedRR(ctx, fp)
            createCompletedRR(ctx, fp) // Reset point
            createFailedRR(ctx, fp)
            createFailedRR(ctx, fp)  // Most recent
        }, 2), // Count only after reset
    )
})
```

#### **Refactor: Blocking Decision Tests**

```go
DescribeTable("BlockIfNeeded",
    func(priorFailures int, shouldBlock bool, expectedPhase remediationv1.RemediationPhase) {
        // Given: Prior failures
        for i := 0; i < priorFailures; i++ {
            createFailedRR(ctx, fingerprint)
        }

        // When: Check blocking
        newRR := createPendingRR(ctx, fingerprint)
        err := consecutiveBlock.BlockIfNeeded(ctx, newRR)

        // Then: Verify outcome
        Expect(err).ToNot(HaveOccurred())
        if shouldBlock {
            Expect(newRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
            Expect(newRR.Status.BlockedUntil).ToNot(BeNil())
        } else {
            Expect(newRR.Status.OverallPhase).ToNot(Equal(remediationv1.PhaseBlocked))
        }
    },
    Entry("0 prior failures - no block", 0, false, remediationv1.PhasePending),
    Entry("1 prior failure - no block", 1, false, remediationv1.PhasePending),
    Entry("2 prior failures - no block", 2, false, remediationv1.PhasePending),
    Entry("3 prior failures - BLOCK", 3, true, remediationv1.PhaseBlocked),
    Entry("5 prior failures - BLOCK", 5, true, remediationv1.PhaseBlocked),
)
```

**Benefits**:
- ✅ Reduces 150+ lines to ~40 lines
- ✅ All scenarios visible at a glance
- ✅ Easy to add new threshold values
- ✅ Consistent test logic

---

### **Priority 3: Restructure by Method (MEDIUM)**

```go
// ✅ CORRECT: Organize by method being tested
var _ = Describe("ConsecutiveFailureBlocker", func() {
    Describe("CountConsecutiveFailures", func() {
        DescribeTable("counting logic", ...)

        Context("field selector usage", func() {
            It("should use spec.signalFingerprint not labels", func() {
                // Tests field selector implementation
            })
        })

        Context("chronological ordering", func() {
            It("should count from most recent backwards", func() {
                // Tests ordering logic
            })
        })
    })

    Describe("BlockIfNeeded", func() {
        DescribeTable("threshold decisions", ...)

        Context("notification creation", func() {
            It("should create NotificationRequest when blocking", func() {
                // Tests notification side effect
            })
        })
    })
})

var _ = Describe("Reconciler.HandleBlockedPhase", func() {
    Describe("cooldown expiry", func() {
        DescribeTable("expiry timing", ...)

        It("should transition to Failed when expired", func() {
            // Tests phase transition logic
        })
    })

    Describe("requeue timing", func() {
        It("should calculate precise requeue duration", func() {
            // Tests requeue calculation
        })
    })
})

var _ = Describe("IsTerminalPhase", func() {
    DescribeTable("phase classification",
        func(phase remediationv1.RemediationPhase, isTerminal bool) {
            result := controller.IsTerminalPhase(phase)
            Expect(result).To(Equal(isTerminal))
        },
        Entry("Blocked is non-terminal", remediationv1.PhaseBlocked, false),
        Entry("Failed is terminal", remediationv1.PhaseFailed, true),
        Entry("Completed is terminal", remediationv1.PhaseCompleted, true),
        Entry("Pending is non-terminal", remediationv1.PhasePending, false),
    )
})
```

---

## 📋 Refactoring Checklist

### **Phase 1: Naming & Structure (30 min)**
- [ ] Remove "BR-ORCH-042" from Describe blocks → Use "ConsecutiveFailureBlocker"
- [ ] Remove "AC-042-X-X" from Context blocks → Use method names
- [ ] Update header comments to clarify "Unit Tests" vs "Business Tests"
- [ ] Organize by method: `CountConsecutiveFailures`, `BlockIfNeeded`, `HandleBlockedPhase`

### **Phase 2: Table-Driven Tests (45 min)**
- [ ] Convert threshold tests to `DescribeTable` (5 scenarios → 1 table)
- [ ] Convert blocking decision tests to `DescribeTable` (6 scenarios → 1 table)
- [ ] Convert phase classification tests to `DescribeTable` (3 tests → 1 table)
- [ ] Convert cooldown timing tests to `DescribeTable` (2 tests → 1 table)

### **Phase 3: Helper Functions (15 min)**
- [ ] Create `createFailedRR(ctx, fingerprint)` helper
- [ ] Create `createCompletedRR(ctx, fingerprint)` helper
- [ ] Create `createBlockedRR(ctx, fingerprint, blockedUntil)` helper
- [ ] Reduce duplication in setup code

### **Phase 4: Validation (10 min)**
- [ ] Run unit tests to ensure no regressions
- [ ] Verify all 57 tests still pass
- [ ] Check line count reduction (expect ~40% reduction)
- [ ] Verify table readability

**Total Estimated Time**: ~2 hours

---

## 📊 Impact Assessment

### **Before Refactoring**
- **Lines**: 732 lines
- **Test Structure**: Individual `It()` blocks
- **Duplication**: High (setup code repeated ~15 times)
- **Readability**: Medium (have to read each test individually)
- **Maintainability**: Low (adding scenario requires new test block)

### **After Refactoring (Estimated)**
- **Lines**: ~450 lines (**38% reduction**)
- **Test Structure**: Table-driven with `DescribeTable`
- **Duplication**: Low (setup code in helpers)
- **Readability**: High (all scenarios visible in tables)
- **Maintainability**: High (adding scenario = 1 new Entry)

### **Benefits**
1. ✅ **Compliance**: Aligns with testing guidelines
2. ✅ **Clarity**: Clear distinction between unit and BR tests
3. ✅ **Efficiency**: Faster test authoring (table entries vs full tests)
4. ✅ **Coverage**: Easier to spot missing scenarios in tables
5. ✅ **Maintenance**: Fewer lines to maintain

---

## 🎯 Example: Side-by-Side Comparison

### **Before (Current - 80 lines)**

```go
Context("AC-042-3-1: RO sets BlockedUntil when blocking", func() {
    It("should set BlockedUntil to now + cooldown duration", func() {
        // Given: 3 consecutive failures
        for i := 0; i < 3; i++ {
            rr := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name: "rr-cooldown-" + generateRandomString(5),
                    Namespace: namespace,
                    CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-i-1) * time.Minute)),
                },
                Spec: remediationv1.RemediationRequestSpec{
                    SignalFingerprint: fingerprint,
                    SignalName: "HighCPUUsage",
                },
                Status: remediationv1.RemediationRequestStatus{
                    OverallPhase: remediationv1.PhaseFailed,
                },
            }
            Expect(fakeClient.Create(ctx, rr)).To(Succeed())
        }

        // When: Block new RR
        newRR := &remediationv1.RemediationRequest{...}
        Expect(fakeClient.Create(ctx, newRR)).To(Succeed())
        err := consecutiveBlock.BlockIfNeeded(ctx, newRR)

        // Then: Verify BlockedUntil
        Expect(err).ToNot(HaveOccurred())
        Expect(newRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
        Expect(newRR.Status.BlockedUntil).ToNot(BeNil())
        // ... 15 more lines
    })

    It("should set BlockReason to consecutive_failures_exceeded", func() {
        // Given: 3 consecutive failures (REPEATED SETUP)
        for i := 0; i < 3; i++ { ... }
        // ... 30 more lines
    })
})
```

### **After (Refactored - 35 lines)**

```go
Describe("BlockIfNeeded", func() {
    DescribeTable("blocking behavior",
        func(priorFailures int, shouldBlock bool) {
            // Given: Prior failures
            createConsecutiveFailures(ctx, fingerprint, priorFailures)

            // When: Check blocking
            newRR := createPendingRR(ctx, fingerprint)
            err := consecutiveBlock.BlockIfNeeded(ctx, newRR)

            // Then: Verify
            Expect(err).ToNot(HaveOccurred())
            if shouldBlock {
                Expect(newRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
                Expect(newRR.Status.BlockedUntil).ToNot(BeNil())
                Expect(*newRR.Status.BlockReason).To(Equal("consecutive_failures_exceeded"))
                Expect(newRR.Status.BlockedUntil.Time).
                    To(BeTemporally("~", time.Now().Add(1*time.Hour), 10*time.Second))
            } else {
                Expect(newRR.Status.OverallPhase).ToNot(Equal(remediationv1.PhaseBlocked))
            }
        },
        Entry("2 failures - no block", 2, false),
        Entry("3 failures - block with reason", 3, true),
        Entry("5 failures - block", 5, true),
    )
})
```

**Reduction**: 80 lines → 35 lines (**56% reduction**)

---

## 🚦 Priority Assessment

| Issue | Fix Priority | Effort | Impact | Risk |
|-------|-------------|--------|--------|------|
| **BR Prefix Removal** | 🔴 CRITICAL | Low (30 min) | High (compliance) | None |
| **Table-Driven Tests** | 🟠 HIGH | Medium (45 min) | High (maintainability) | Low |
| **Method Structure** | 🟡 MEDIUM | Low (15 min) | Medium (readability) | None |

**Recommendation**: Complete all three in one refactoring session (~2 hours).

---

## ✅ Acceptance Criteria for Refactoring

Refactoring is complete when:
- [ ] **Zero "BR-ORCH-042" references** in Describe/Context blocks
- [ ] **Zero "AC-042-X-X" references** in Context blocks
- [ ] **≥5 DescribeTable usages** for repeated scenarios
- [ ] **All 57 tests still pass** after refactoring
- [ ] **Line count reduced by ≥30%** (target: 450 lines)
- [ ] **Helper functions created** for common setup
- [ ] **Organized by method** (CountConsecutiveFailures, BlockIfNeeded, etc.)
- [ ] **Follows WorkflowExecution** testing patterns

---

## 📚 References

1. **[TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)** - BR vs Unit test decision framework
2. **[WorkflowExecution testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md)** - Table-driven test examples
3. **[15-testing-coverage-standards.mdc](.cursor/rules/15-testing-coverage-standards.mdc)** - Testing coverage standards

---

**Document Version**: 1.0
**Last Updated**: December 13, 2025
**Maintained By**: Kubernaut RO Team
**Status**: 🚨 **ACTION REQUIRED** - Refactoring needed before V1.0 release

