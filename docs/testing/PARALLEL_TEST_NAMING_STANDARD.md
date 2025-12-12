# Parallel Test Resource Naming Standard

**Date**: 2025-12-11
**Status**: ✅ **MANDATORY** - Enforced for all integration and E2E tests
**Issue**: Resource name collisions in parallel test execution (Ginkgo `-procs=4`)
**Decision**: [DD-TEST-004](../architecture/decisions/DD-TEST-004-unique-resource-naming-strategy.md) - Official design decision
**Notice**: [NOTICE_PARALLEL_TEST_NAMING_REQUIREMENT.md](../handoff/NOTICE_PARALLEL_TEST_NAMING_REQUIREMENT.md) - Team notification

---

## 🚨 **The Problem**

### **Second-Precision Timestamps Cause Collisions**

When running tests in parallel (`ginkgo -procs=4`), tests execute simultaneously. Using second-precision timestamps creates identical resource names:

```go
// ❌ BAD - Causes collisions in parallel execution
func randomSuffix() string {
    return time.Now().Format("20060102150405")  // Returns: "20251211180347"
}

// Process 1 creates: "integration-test-20251211180347"
// Process 2 creates: "integration-test-20251211180347"  ← COLLISION!
// Process 3 creates: "integration-test-20251211180347"  ← COLLISION!
// Process 4 creates: "integration-test-20251211180347"  ← COLLISION!
```

### **Symptoms**

```
Expected success, but got an error:
    aianalyses.aianalysis.kubernaut.ai "integration-test-20251211180347" already exists
```

Tests fail with "already exists" errors because multiple parallel processes create resources with identical names within the same second.

---

## ✅ **The Solution: Nanosecond Precision**

### **Use `testutil.UniqueTestSuffix()`**

```go
// ✅ GOOD - Guaranteed unique in parallel execution
import "github.com/jordigilh/kubernaut/pkg/testutil"

func TestSomething() {
    name := fmt.Sprintf("integration-test-%s", testutil.UniqueTestSuffix())
    // Returns: "integration-test-1765494131234567890"
    // Each call guaranteed unique (nanosecond precision)
}
```

### **Or use `testutil.UniqueTestName()`**

```go
// ✅ EVEN BETTER - Convenience function
analysis := &aianalysisv1alpha1.AIAnalysis{
    ObjectMeta: metav1.ObjectMeta{
        Name:      testutil.UniqueTestName("integration-test"),
        Namespace: "default",
    },
}
```

---

## 📊 **Comparison**

| Approach | Precision | Collision Risk | Parallel Safe | Example Output |
|----------|-----------|----------------|---------------|----------------|
| ❌ `time.Now().Format("20060102150405")` | 1 second | **HIGH** (same second) | **NO** | `20251211180347` |
| ✅ `time.Now().UnixNano()` | 1 nanosecond | **NONE** | **YES** | `1765494131234567890` |
| ✅ `testutil.UniqueTestSuffix()` | 1 nanosecond | **NONE** | **YES** | `1765494131234567890` |
| ✅ `testutil.UniqueTestName("prefix")` | 1 nanosecond | **NONE** | **YES** | `prefix-1765494131234567890` |

---

## 🔍 **Detection and Migration**

### **Finding Violations**

Search for second-precision timestamp patterns:

```bash
# Find all second-precision timestamp usages
rg 'time\.Now\(\)\.Format\("20060102' test/

# Find custom suffix functions that might have the issue
rg 'func.*[Rr]andom.*\(\)|func.*suffix.*\(\)' test/ -A 3
```

### **Migration Pattern**

**Before** (collision-prone):
```go
func randomSuffix() string {
    return time.Now().Format("20060102150405")
}

name := fmt.Sprintf("test-resource-%s", randomSuffix())
```

**After** (parallel-safe):
```go
import "github.com/jordigilh/kubernaut/pkg/testutil"

name := testutil.UniqueTestName("test-resource")
// Or:
name := fmt.Sprintf("test-resource-%s", testutil.UniqueTestSuffix())
```

---

## 📝 **Usage Guidelines**

### **When to Use**

Use `testutil.UniqueTestName()` or `testutil.UniqueTestSuffix()` for:
- ✅ Kubernetes resource names (Pods, Services, CRDs)
- ✅ Namespaces in parallel tests
- ✅ Database records (if using timestamp-based IDs)
- ✅ File names in shared test directories
- ✅ Any resource that might conflict in parallel execution

### **When NOT to Use**

Don't use for:
- ❌ Human-readable test output (long number is hard to read)
- ❌ Sequential tests (if `-procs=1`, second precision is fine)
- ❌ Resources with guaranteed unique prefixes (e.g., per-process namespaces)

### **Best Practices**

1. **Default to nanosecond precision** for all test resources
2. **Use `testutil` helpers** instead of custom functions
3. **Document exceptions** if using second precision (with justification)
4. **Test in parallel** (`make test -procs=4`) to catch collisions early

---

## 🎯 **Enforcement**

### **Code Review Checklist**

When reviewing test code, check:
- [ ] No `time.Now().Format("20060102150405")` in test code
- [ ] Resource names use `testutil.UniqueTestName()` or `testutil.UniqueTestSuffix()`
- [ ] Custom suffix functions use nanosecond precision
- [ ] Tests pass in parallel (`ginkgo -procs=4`)

### **Pre-Commit Hook** (Optional)

```bash
#!/bin/bash
# Check for second-precision timestamps in test code
if grep -r 'time\.Now()\.Format("20060102' test/; then
    echo "❌ ERROR: Second-precision timestamp found in test code"
    echo "Use testutil.UniqueTestSuffix() instead to avoid parallel test collisions"
    exit 1
fi
```

---

## 📚 **Examples**

### **Integration Tests**

```go
package myservice_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/testutil"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MyService Integration", func() {
    It("should create resource with unique name", func() {
        resource := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{
                Name:      testutil.UniqueTestName("test-pod"),
                Namespace: "default",
            },
        }
        Expect(k8sClient.Create(ctx, resource)).To(Succeed())
    })
})
```

### **E2E Tests**

```go
package e2e_test

import (
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("E2E Workflow", func() {
    It("should process workflow end-to-end", func() {
        workflowName := testutil.UniqueTestName("e2e-workflow")

        // Create workflow
        workflow := createWorkflow(workflowName)
        Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

        // Verify processing
        Eventually(func() string {
            _ = k8sClient.Get(ctx, client.ObjectKey{Name: workflowName}, workflow)
            return workflow.Status.Phase
        }).Should(Equal("Completed"))
    })
})
```

---

## 🔗 **Related Documentation**

- [03-testing-strategy.mdc](../rules/03-testing-strategy.mdc) - Defense-in-depth testing approach
- [PARALLEL_TESTING_ENABLEMENT.md](../test/integration/gateway/PARALLEL_TESTING_ENABLEMENT.md) - Parallel test patterns
- [DD-TEST-001](../docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md) - Port allocation (related pattern)

---

## 📈 **Impact**

### **Before Fix**
- ❌ 4 tests failing with "already exists" errors
- ❌ Flaky parallel test execution
- ❌ `47/51` tests passing (92%)

### **After Fix**
- ✅ 0 name collisions
- ✅ Reliable parallel test execution
- ✅ Target: `51/51` tests passing (100%)

---

## 🔗 **Related Documentation**

### **Primary References**
- **[DD-TEST-004](../architecture/decisions/DD-TEST-004-unique-resource-naming-strategy.md)** - ⭐ **OFFICIAL DESIGN DECISION**
- **[NOTICE_PARALLEL_TEST_NAMING_REQUIREMENT.md](../handoff/NOTICE_PARALLEL_TEST_NAMING_REQUIREMENT.md)** - Team notification

### **Supporting Documentation**
- [03-testing-strategy.mdc](../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth testing approach
- [PARALLEL_TESTING_ENABLEMENT.md](../test/integration/gateway/PARALLEL_TESTING_ENABLEMENT.md) - Parallel test patterns
- [DD-TEST-001](../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) - Port allocation (related pattern)
- [DD-TEST-002](../architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md) - Parallel execution standard
- [SUCCESS_AIANALYSIS_INTEGRATION_TESTS.md](../handoff/SUCCESS_AIANALYSIS_INTEGRATION_TESTS.md) - Implementation case study

---

**Status**: ✅ **IMPLEMENTED** - All AIAnalysis integration and E2E tests updated
**Next**: Apply pattern to remaining test suites (Gateway, Notification, etc.)
