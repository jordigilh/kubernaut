# SignalProcessing Unit Tests Triage Report

**Date**: December 16, 2025
**Service**: SignalProcessing
**Authoritative References**:
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)
- [testing-strategy.md (WE reference)](../services/crd-controllers/03-workflowexecution/testing-strategy.md)
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)

---

## 📊 **Executive Summary**

| Violation Type | Count | Severity | Files Affected |
|----------------|-------|----------|----------------|
| **Package Naming** | 4 files | 🟡 MEDIUM | 4 |
| **BR-* Prefix in Unit Tests** | 86 instances | 🔴 HIGH | 9 |
| **time.Sleep() Usage** | 17 instances | 🟠 MEDIUM-HIGH | 8 |
| **Skip() Usage** | 0 | ✅ COMPLIANT | 0 |
| **NULL-TESTING Anti-Pattern** | 66+ instances | 🔴 HIGH | 10+ |
| **Weak Assertions (>0, NotEmpty)** | 6 instances | 🟡 MEDIUM | 4 |

---

## 🔴 **VIOLATION 1: Package Naming (4 files)**

### Guideline
Per [testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md) lines 243-258:
```go
// test/unit/workflowexecution/controller_test.go
package workflowexecution  // ← Correct: No _test suffix
```

### Violations Found

| File | Current | Expected |
|------|---------|----------|
| `audit_client_test.go` | `package signalprocessing_test` | `package signalprocessing` |
| `rego_security_wrapper_test.go` | `package signalprocessing_test` | `package signalprocessing` |
| `rego_engine_test.go` | `package signalprocessing_test` | `package signalprocessing` |
| `label_detector_test.go` | `package signalprocessing_test` | `package signalprocessing` |

### Compliant Files ✅
- `conditions_test.go` - `package signalprocessing` ✅
- `controller_shutdown_test.go` - `package signalprocessing` ✅
- `controller_error_handling_test.go` - `package signalprocessing` ✅
- `environment_classifier_test.go` - `package signalprocessing` ✅
- `priority_engine_test.go` - `package signalprocessing` ✅
- `business_classifier_test.go` - `package signalprocessing` ✅
- `degraded_test.go` - `package signalprocessing` ✅
- `config_test.go` - `package signalprocessing` ✅
- `ownerchain_builder_test.go` - `package signalprocessing` ✅
- `cache_test.go` - `package signalprocessing` ✅
- `enricher_test.go` - `package signalprocessing` ✅
- `suite_test.go` - `package signalprocessing` ✅
- `metrics_test.go` - `package signalprocessing` ✅

### Remediation
```bash
# Fix package declarations
sed -i '' 's/package signalprocessing_test/package signalprocessing/' \
  test/unit/signalprocessing/audit_client_test.go \
  test/unit/signalprocessing/rego_security_wrapper_test.go \
  test/unit/signalprocessing/rego_engine_test.go \
  test/unit/signalprocessing/label_detector_test.go
```

**Effort**: ~15 minutes (may require import adjustments)

---

## 🔴 **VIOLATION 2: BR-* Prefix in Unit Tests (86 instances)**

### Guideline
Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) lines 231-238:
> **Unit Tests Should NOT:**
> - Use BR-* prefixes (those are for business tests)
> - Test business outcomes
> - Validate SLAs

Per [testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md) lines 68-75:
| Aspect | Business Requirement Tests | Unit Tests |
|--------|----------------------------|------------|
| **Naming** | BR-WE-XXX prefix | Function/method name |

### Violations by File

| File | BR References | Example |
|------|---------------|---------|
| `conditions_test.go` | BR-SP-110, BR-SP-001, BR-SP-051, BR-SP-070, BR-SP-071, BR-SP-002, BR-SP-080 | `Describe("SignalProcessing Conditions (BR-SP-110)"` |
| `environment_classifier_test.go` | BR-SP-051, BR-SP-052 | `Context("Happy Path: BR-SP-051 Namespace Label Classification"` |
| `priority_engine_test.go` | BR-SP-070, BR-SP-071, BR-SP-072 | `Context("Happy Path: BR-SP-070 Priority Assignment"` |
| `audit_client_test.go` | BR-SP-090 | `Describe("BR-SP-090: RecordSignalProcessed"` |
| `business_classifier_test.go` | BR-SP-002, BR-SP-080, BR-SP-081 | `Context("BR-SP-080: Confidence Tier Detection"` |
| `rego_security_wrapper_test.go` | BR-SP-104 | `It("CL-SEC-01: should strip labels with kubernaut.ai/ prefix (BR-SP-104)"` |
| `rego_engine_test.go` | BR-SP-102 | `It("CL-HP-01: should extract team from namespace label (BR-SP-102)"` |
| `label_detector_test.go` | BR-SP-101, BR-SP-103 | `It("DL-HP-01: should detect ArgoCD GitOps management (BR-SP-101)"` |
| `ownerchain_builder_test.go` | BR-SP-100 | `Context("BR-SP-100: Happy Path - Standard Owner Chains"` |

### Correct Pattern (per guidelines)
```go
// ❌ WRONG: BR-* prefix in unit test
Describe("BR-SP-090: RecordSignalProcessed", func() { ... })

// ✅ CORRECT: Function/method name as test description
Describe("RecordSignalProcessed", func() { ... })

// ✅ ACCEPTABLE: BR reference in comment only (for traceability)
// Tests implementation for BR-SP-090 audit trail requirement
Describe("RecordSignalProcessed", func() { ... })
```

### Remediation Strategy

**Option A: Remove BR-* from test names, keep in comments**
```go
// Before
var _ = Describe("SignalProcessing Conditions (BR-SP-110)", func() {

// After
// Tests implementation for BR-SP-110: Kubernetes Conditions
var _ = Describe("SignalProcessing Conditions", func() {
```

**Option B: Move BR-tests to E2E tier**
Tests with BR-* prefixes that validate business outcomes should be moved to `test/e2e/signalprocessing/business_requirements_test.go`.

### Recommended Approach
- **Keep in Unit Tests**: Implementation correctness tests (rename to remove BR-* from test names)
- **Move to E2E/BR Tests**: Tests that validate business outcomes, SLAs, or cross-component behavior

**Effort**: ~2-3 hours (requires careful review of each test's purpose)

---

## 🟠 **VIOLATION 3: time.Sleep() Usage (17 instances)**

### Guideline
Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) lines 443-631:
> **MANDATORY**: `time.Sleep()` calls are **ABSOLUTELY FORBIDDEN** in ALL test tiers for waiting on asynchronous operations, with **NO EXCEPTIONS**.

**Acceptable uses:**
- Testing timing behavior itself (e.g., timeout tests)
- Rate limiting tests
- Intentional staggering for storm scenarios

### Violations by File

| File | Count | Context | Acceptable? |
|------|-------|---------|-------------|
| `controller_shutdown_test.go` | 10 | Testing context cancellation and graceful shutdown timing | ⚠️ NEEDS REVIEW |
| `controller_error_handling_test.go` | 1 | Testing retry timing | ⚠️ NEEDS REVIEW |
| `environment_classifier_test.go` | 1 | `time.Sleep(10 * time.Millisecond) // Ensure timeout` | ✅ ACCEPTABLE (timeout test) |
| `priority_engine_test.go` | 1 | `time.Sleep(10 * time.Millisecond) // Ensure timeout` | ✅ ACCEPTABLE (timeout test) |
| `business_classifier_test.go` | 1 | Testing timeout behavior | ✅ ACCEPTABLE (timeout test) |
| `rego_engine_test.go` | 1 | `time.Sleep(10 * time.Millisecond) // Ensure context is cancelled` | ✅ ACCEPTABLE (testing cancellation) |
| `ownerchain_builder_test.go` | 1 | Testing timeout behavior | ⚠️ NEEDS REVIEW |
| `cache_test.go` | 1 | Testing TTL expiration | ⚠️ NEEDS REVIEW |

### Detailed Analysis of Suspicious Uses

#### `controller_shutdown_test.go` (10 instances) - 🔴 HIGH PRIORITY
```go
// Line 63: Inside goroutine simulating work
time.Sleep(10 * time.Millisecond)

// Line 69: Before checking results
time.Sleep(20 * time.Millisecond)  // ❌ Should use Eventually()

// Line 308: Waiting for goroutine
time.Sleep(500 * time.Millisecond)  // ❌ Should use Eventually()
```

**Verdict**: Mixed - some are simulating work timing (acceptable), others are waiting for async operations (violation).

#### `cache_test.go` (1 instance)
```go
// Line 101: Testing TTL expiration
time.Sleep(100 * time.Millisecond)
```

**Verdict**: ⚠️ NEEDS REVIEW - If testing cache TTL behavior, should use Eventually() to wait for expiration.

### Remediation

```go
// ❌ BEFORE: Forbidden pattern
time.Sleep(20 * time.Millisecond)
err := k8sClient.Get(ctx, key, &crd)
Expect(err).ToNot(HaveOccurred())

// ✅ AFTER: Correct pattern
Eventually(func() error {
    return k8sClient.Get(ctx, key, &crd)
}, 1*time.Second, 10*time.Millisecond).Should(Succeed())
```

**Effort**: ~1-2 hours (requires context-sensitive review)

---

## ✅ **COMPLIANT: No Skip() Usage**

All 17 SP unit test files are compliant - no `Skip()` calls found.

---

## 🔴 **VIOLATION 4: NULL-TESTING Anti-Pattern (66+ instances)**

### Guideline
Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) lines 281-286:
> **Unit Tests Must:**
> - Test edge cases and error conditions
> - **Provide clear developer feedback**
> - **Maintain high code coverage** (meaningful coverage, not null checks)

Per [00-project-guidelines.mdc](../../.cursor/rules/00-project-guidelines.mdc):
> **Testing Anti-Patterns to AVOID:**
> - **NULL-TESTING**: Weak assertions (not nil, > 0, empty checks)

### Anti-Pattern Definition
**NULL-TESTING** is when a test only verifies that something is "not nil" or "exists" without validating its **behavior** or **correctness**.

### Examples Found

#### 🔴 **metrics_test.go** - Severe NULL-TESTING (4 tests)
```go
// ❌ ANTI-PATTERN: Only checks counter exists, not that it incremented
It("should increment processing total counter", func() {
    m.IncrementProcessingTotal("enriching", "success")
    m.IncrementProcessingTotal("enriching", "success")

    // Verify counter incremented (basic check - not null-testing)  ← COMMENT LIES!
    Expect(m.ProcessingTotal).NotTo(BeNil())  // ❌ This IS null-testing!
})

// ✅ CORRECT: Verifies actual counter value changed
It("should increment processing total counter", func() {
    before := testutil.GetCounterValue(m.ProcessingTotal, "enriching", "success")
    m.IncrementProcessingTotal("enriching", "success")
    after := testutil.GetCounterValue(m.ProcessingTotal, "enriching", "success")

    Expect(after - before).To(Equal(float64(1)))  // ✅ Verifies behavior
})
```

#### 🟡 **label_detector_test.go** - Mixed Pattern (22 instances)
```go
// ⚠️ ACCEPTABLE (followed by behavior assertions):
Expect(labels).NotTo(BeNil())  // Nil guard
Expect(labels.GitOpsManaged).To(BeTrue())  // ✅ Actual behavior check

// ❌ ANTI-PATTERN (standalone nil check):
detector = detection.NewLabelDetector(fakeClient, logger)
Expect(detector).NotTo(BeNil())  // ❌ Only checks creation, not behavior
```

#### 🟡 **enricher_test.go** - Mixed Pattern (19 instances)
```go
// ⚠️ MOSTLY ACCEPTABLE: Most nil checks are followed by property assertions
Expect(result).NotTo(BeNil())
Expect(result.NamespaceLabels).To(HaveKeyWithValue("environment", "staging"))  // ✅
```

### Severity by File

| File | Nil/NotNil Checks | Standalone (Anti-Pattern) | Followed by Assertions (OK) |
|------|-------------------|---------------------------|----------------------------|
| `metrics_test.go` | 6 | **6 (100%)** 🔴 | 0 |
| `label_detector_test.go` | 22 | 3 | 19 |
| `enricher_test.go` | 19 | 2 | 17 |
| `conditions_test.go` | 2 | 0 | 2 |
| `ownerchain_builder_test.go` | 5 | 1 | 4 |
| `cache_test.go` | 1 | 1 | 0 |
| Others | 11 | ~5 | ~6 |

### Remediation

**metrics_test.go requires complete rewrite:**
```go
// ✅ CORRECT: Verify metric behavior through registry inspection
It("should increment processing total counter", func() {
    // Get baseline
    families, _ := prometheus.DefaultGatherer.Gather()
    beforeCount := getMetricValue(families, "signalprocessing_processing_total",
        "phase", "enriching", "result", "success")

    // Execute
    m.IncrementProcessingTotal("enriching", "success")

    // Verify increment
    families, _ = prometheus.DefaultGatherer.Gather()
    afterCount := getMetricValue(families, "signalprocessing_processing_total",
        "phase", "enriching", "result", "success")

    Expect(afterCount - beforeCount).To(Equal(float64(1)))
})
```

**Effort**: ~3-4 hours (metrics_test.go needs significant rewrite)

---

## 🟡 **VIOLATION 5: Weak Assertions (6 instances)**

### Guideline
Per [00-project-guidelines.mdc](../../.cursor/rules/00-project-guidelines.mdc):
> **AVOID** null-testing anti-pattern (not nil, **> 0**, empty checks)

### Violations Found

| File | Line | Pattern | Issue |
|------|------|---------|-------|
| `controller_error_handling_test.go` | 118 | `BeNumerically(">", 0)` | Weak - should check specific value |
| `priority_engine_test.go` | 672 | `NotTo(BeEmpty())` | Weak - should check specific priority |
| `priority_engine_test.go` | 796 | `NotTo(BeEmpty())` | Weak - should check specific priority |
| `enricher_test.go` | 154, 470, 515 | `NotTo(BeEmpty())` | Acceptable (followed by specific checks) |

### Examples

```go
// ❌ WEAK: Only checks something was returned
Expect(result.Priority).NotTo(BeEmpty())

// ✅ STRONG: Checks specific expected behavior
Expect(result.Priority).To(Equal("P3"))
// OR at minimum:
Expect(result.Priority).To(BeOneOf("P1", "P2", "P3", "P4", "P5"))
```

**Effort**: ~30 minutes

---

## 🟢 **COMPLIANT: Business Outcome vs Implementation Focus**

### Positive Finding
Most SP unit tests correctly focus on **implementation correctness** rather than **business outcomes**.

**Evidence**: 13 files contain the correct header comment:
```go
// Unit tests validate implementation correctness, not business value delivery.
```

### Files with Correct Focus ✅
- `controller_shutdown_test.go`
- `controller_error_handling_test.go`
- `environment_classifier_test.go`
- `priority_engine_test.go`
- `business_classifier_test.go`
- `degraded_test.go`
- `config_test.go`
- `cache_test.go`
- `enricher_test.go`
- `suite_test.go`
- `metrics_test.go`

### Minor Concern: Test Names
Some test names include business terms (e.g., "SLA", "business unit") but this is acceptable because:
1. They test the **implementation** of business classification logic
2. They don't test **business outcomes** (e.g., "should reduce costs by 20%")
3. The SLA/business references are to **data fields**, not business KPIs

---

## 📋 **Remediation Priority**

| Priority | Violation | Impact | Effort | Action |
|----------|-----------|--------|--------|--------|
| 1️⃣ **P0** | NULL-TESTING in metrics_test.go | Critical (tests don't verify behavior) | 3-4h | Rewrite with registry inspection |
| 2️⃣ **P1** | BR-* prefix in unit tests | High (violates test taxonomy) | 2-3h | Rename tests, move BR-tests to E2E |
| 3️⃣ **P2** | time.Sleep() violations | Medium-High (flaky tests) | 1-2h | Replace with Eventually() |
| 4️⃣ **P3** | Weak assertions (>0, NotEmpty) | Medium (weak validation) | 30min | Replace with specific assertions |
| 5️⃣ **P4** | Package naming | Medium (convention violation) | 15min | Fix package declarations |

---

## 🎯 **Recommended Remediation Plan**

### Phase 1: metrics_test.go Rewrite (3-4 hours) - **CRITICAL**
1. Rewrite all 4 metrics tests to verify actual metric values
2. Use Prometheus registry inspection pattern
3. Verify counter increments, histogram observations, etc.
4. This file currently provides **zero test coverage** (only null checks)

### Phase 2: Package Naming (15 min)
1. Fix 4 files with `_test` suffix
2. Verify imports still resolve
3. Run `go test ./test/unit/signalprocessing/...`

### Phase 3: time.Sleep() Cleanup (1-2 hours)
1. Review each time.Sleep() in context
2. Replace async waits with Eventually()
3. Keep acceptable uses (timeout tests, work simulation)
4. Document rationale in code comments

### Phase 4: BR-* Prefix Remediation (2-3 hours)
1. Review each BR-* prefixed test
2. Determine if it tests implementation (keep as unit, rename) or business outcome (move to E2E)
3. Remove BR-* from unit test names
4. Add BR reference as comment for traceability
5. Move true business outcome tests to E2E tier

### Phase 5: Weak Assertions (30 min)
1. Replace `BeNumerically(">", 0)` with specific expected values
2. Replace standalone `NotTo(BeEmpty())` with specific content checks
3. Verify assertions test actual behavior

---

## 📊 **Post-Remediation Target State**

| Aspect | Current | Target |
|--------|---------|--------|
| NULL-TESTING | 66+ instances | <10 (nil guards only, followed by assertions) |
| Weak assertions | 6 instances | 0 instances |
| Package naming | 4 violations | 0 violations |
| BR-* in unit tests | 86 instances | 0 instances (comments only) |
| time.Sleep() | 17 instances | <5 instances (acceptable uses only) |
| Skip() | 0 | 0 |
| metrics_test.go coverage | 0% (null tests only) | >80% (behavior verification) |

---

## 🔗 **Related Documents**

- [DD-TEST-002: Parallel Test Execution](../../architecture/decisions/DD-TEST-002-parallel-test-execution.md)
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)
- [SP Testing Strategy](../services/crd-controllers/01-signalprocessing/testing-strategy.md) (if exists)

---

**Report Generated By**: AI Assistant
**Reviewed By**: _pending_
**Approval Status**: ⏳ PENDING REVIEW

