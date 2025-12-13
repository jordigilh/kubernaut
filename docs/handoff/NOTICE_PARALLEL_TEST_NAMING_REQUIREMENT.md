# NOTICE: Mandatory Test Resource Naming Pattern for Parallel Execution

**Date**: 2025-12-11
**Type**: 🚨 **CRITICAL** - Affects ALL Test Suites
**Action Required**: ✅ **YES** - Update your tests
**Deadline**: Next sprint
**Severity**: **HIGH** - Flaky tests block CI/CD

---

## 📢 **What's Changing**

**ALL test suites MUST use `pkg/testutil.UniqueTestName()` for resource naming to prevent parallel test collisions.**

**Before** (Causes Failures):
```go
// ❌ Second-precision timestamps cause collisions
func randomSuffix() string {
    return time.Now().Format("20060102150405")
}
name := "test-resource-" + randomSuffix()
```

**After** (Reliable):
```go
// ✅ Gateway-aligned pattern (nanosecond + seed + counter)
import "github.com/jordigilh/kubernaut/pkg/testutil"

name := testutil.UniqueTestName("test-resource")
```

---

## 🎯 **Why This Matters**

### **The Problem**

When tests run in parallel (Ginkgo `-procs=4`), multiple processes create resources simultaneously. Second-precision timestamps return the **same value** within the same second:

```
Time: 18:03:47 (second precision)

Process 1 creates: "integration-test-20251211180347" ✅
Process 2 creates: "integration-test-20251211180347" ❌ COLLISION!
Process 3 creates: "integration-test-20251211180347" ❌ COLLISION!
Process 4 creates: "integration-test-20251211180347" ❌ COLLISION!

Error: aianalyses.aianalysis.kubernaut.ai "integration-test-20251211180347" already exists
```

### **Real Impact**

**AIAnalysis Service** (Before Fix):
- ❌ 21 tests failing with "already exists" errors
- ❌ 59% pass rate (30/51 tests)
- ❌ Flaky CI/CD pipelines
- ❌ Developer frustration

**AIAnalysis Service** (After Fix):
- ✅ 0 name collision errors
- ✅ 98% pass rate (50/51 tests)
- ✅ Reliable parallel execution
- ✅ **+20 tests fixed** by naming change alone

---

## 📋 **Action Items by Team**

### **All Teams: Immediate Actions**

1. **Read the Design Decision**: [DD-TEST-004](../architecture/decisions/DD-TEST-004-unique-resource-naming-strategy.md)
2. **Find violations in your code**:
   ```bash
   # In your service directory
   rg 'time\.Now\(\)\.Format\("20060102' test/
   rg 'func.*[Rr]andom.*\(\)|func.*suffix.*\(\)' test/ -A 3
   ```
3. **Update your tests** (see migration guide below)
4. **Verify tests pass in parallel**:
   ```bash
   make test-integration-yourservice  # Should use -procs=4
   ```

### **Service-Specific Status**

| Service | Status | Action Required | Priority |
|---------|--------|-----------------|----------|
| **AIAnalysis** | ✅ Migrated | None - Reference implementation | ✅ Complete |
| **Gateway** | ⚠️ Uses pattern | Optionally adopt `testutil` helpers | Low |
| **Notification** | ❌ Not migrated | **REQUIRED** - Update tests | **HIGH** |
| **RemediationOrchestrator** | ❌ Not migrated | **REQUIRED** - Update tests | **HIGH** |
| **WorkflowExecution** | ❌ Not migrated | **REQUIRED** - Update tests | **HIGH** |
| **SignalProcessing** | ❌ Not migrated | **REQUIRED** - Update tests | **HIGH** |
| **DataStorage** | ❌ Not migrated | **REQUIRED** - Update tests | **HIGH** |
| **E2E Tests** | ⚠️ Partially migrated | Update remaining tests | Medium |

---

## 🔧 **How to Migrate Your Tests**

### **Step 1: Import the Package**

```go
import "github.com/jordigilh/kubernaut/pkg/testutil"
```

### **Step 2: Replace Custom Functions**

**Before**:
```go
// Remove these custom functions
func randomSuffix() string {
    return time.Now().Format("20060102150405")
}

func uniqueName(prefix string) string {
    return fmt.Sprintf("%s-%d", prefix, time.Now().Unix())
}
```

**After**:
```go
// Use testutil instead - no custom function needed!
```

### **Step 3: Update Resource Creation**

**Before**:
```go
analysis := &aianalysisv1alpha1.AIAnalysis{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "integration-test-" + randomSuffix(),
        Namespace: "default",
    },
}
```

**After**:
```go
analysis := &aianalysisv1alpha1.AIAnalysis{
    ObjectMeta: metav1.ObjectMeta{
        Name:      testutil.UniqueTestName("integration-test"),
        Namespace: "default",
    },
}
```

### **Step 4: Verify Compilation**

```bash
go test -c ./test/integration/yourservice/... 2>&1
```

### **Step 5: Run Tests in Parallel**

```bash
cd test/integration/yourservice
ginkgo -v --procs=4
```

---

## 📚 **Available Functions**

### **`UniqueTestName(prefix)`** ⭐ **RECOMMENDED**

Standard pattern for most cases:

```go
name := testutil.UniqueTestName("test-pod")
// Returns: "test-pod-1765494131234567890-12345-42"
```

### **`UniqueTestSuffix()`**

For custom formatting:

```go
suffix := testutil.UniqueTestSuffix()
// Returns: "1765494131234567890-12345-42"
name := fmt.Sprintf("custom-format-%s", suffix)
```

### **`UniqueTestNameWithProcess(prefix)`**

Includes process ID for debugging:

```go
name := testutil.UniqueTestNameWithProcess("test-alert")
// Returns: "test-alert-p2-1765494131234567890-12345-42"
```

---

## 🔍 **How It Works**

### **Three-Way Uniqueness** (Gateway Pattern)

```
Name = prefix-<nanoseconds>-<random-seed>-<counter>
       └────┬────┘  └──────┬──────┘  └────┬─────┘
      Human-readable  Primary    Run      Sequential
                    uniqueness  isolation   safety
```

**Components**:
1. **Nanosecond Timestamp** (`1765494131234567890`)
   - Primary uniqueness mechanism
   - Prevents same-moment collisions
   - Provides temporal ordering in logs

2. **Random Seed** (`12345`)
   - Isolates different test runs
   - Unique per test execution
   - Handles same-nanosecond scenarios

3. **Atomic Counter** (`42`)
   - Sequential safety
   - Thread-safe incrementing
   - Handles rapid loops

### **Why Not UUID?**

We considered `uuid.New()` but rejected it because:
- ❌ No temporal ordering (harder to debug)
- ❌ Random strings don't correlate with test runs
- ❌ Gateway already uses three-way pattern successfully
- ✅ Three-way pattern is simpler and proven

---

## ✅ **Success Criteria**

Your migration is complete when:
- [ ] No `time.Now().Format("20060102150405")` in test code
- [ ] All custom `randomSuffix()` functions removed
- [ ] All resource names use `testutil.UniqueTestName()` or equivalent
- [ ] Tests pass reliably with `-procs=4`
- [ ] No "already exists" errors in test runs

---

## 📖 **Documentation**

### **Must Read**
1. **[DD-TEST-004](../architecture/decisions/DD-TEST-004-unique-resource-naming-strategy.md)** - Design Decision (THIS IS THE SOURCE OF TRUTH)
2. **[PARALLEL_TEST_NAMING_STANDARD.md](../testing/PARALLEL_TEST_NAMING_STANDARD.md)** - Detailed usage guide

### **Reference Implementations**
- **AIAnalysis**: `test/integration/aianalysis/reconciliation_test.go` - Best example
- **Gateway**: `test/integration/gateway/adapter_interaction_test.go` - Original pattern
- **Utility**: `pkg/testutil/naming.go` - Implementation

### **Related**
- [DD-TEST-001](../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) - Port allocation
- [DD-TEST-002](../architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md) - Parallel execution
- [SUCCESS_AIANALYSIS_INTEGRATION_TESTS.md](./SUCCESS_AIANALYSIS_INTEGRATION_TESTS.md) - Case study

---

## 🚨 **FAQs**

### **Q: Do I need to migrate ALL my tests?**
**A**: Yes, for any test that creates Kubernetes resources (Pods, CRDs, Services, etc.) or might run in parallel.

### **Q: What if my tests use namespaces for isolation?**
**A**: Namespace isolation prevents inter-namespace collisions, but you still need unique names **within** the namespace.

### **Q: Can I keep my custom function and just use nanoseconds?**
**A**: No. Use `testutil` helpers for consistency. The three-way pattern (nanoseconds + seed + counter) is proven and required.

### **Q: What about E2E tests?**
**A**: E2E tests also run in parallel. Same requirements apply.

### **Q: Will this break my tests temporarily?**
**A**: No. Adding `testutil.UniqueTestName()` is backward compatible. Tests will just work better!

### **Q: How do I test my migration?**
**A**: Run tests with `-procs=4` multiple times:
```bash
for i in {1..5}; do
  echo "Run $i"
  ginkgo -v --procs=4 ./test/integration/yourservice/
done
```

### **Q: What if I find a test that's not fixed?**
**A**: Either fix it yourself or create a ticket and assign to your team lead.

---

## 📞 **Support**

### **Questions**
- **Slack**: `#kubernaut-testing` channel
- **Email**: kubernaut-dev@example.com
- **Office Hours**: Tuesdays 2-3pm PST

### **Report Issues**
If you find bugs in `pkg/testutil/naming.go`:
- File issue: https://github.com/kubernaut/kubernaut/issues
- Label: `testing`, `bug`
- Assign: `@testing-team`

### **Migration Help**
If you're stuck migrating your tests:
- Reference: AIAnalysis implementation (see above)
- Ask in `#kubernaut-testing`
- Tag `@testing-champions` for help

---

## 📅 **Timeline**

| Phase | Deadline | Status |
|-------|----------|--------|
| **Phase 1: Core Implementation** | 2025-12-11 | ✅ **COMPLETE** |
| **Phase 2: AIAnalysis Migration** | 2025-12-11 | ✅ **COMPLETE** |
| **Phase 3: Team Notification** | 2025-12-11 | ✅ **COMPLETE** (this doc) |
| **Phase 4: Service Migrations** | **End of Sprint** | ⏳ **IN PROGRESS** |
| **Phase 5: Pre-commit Hook** | Q1 2026 | 📋 **PLANNED** |

---

## 🎯 **Enforcement**

### **Code Review Checklist**

Reviewers MUST check:
- [ ] No `time.Now().Format("20060102150405")` in test code
- [ ] Resource names use `testutil.UniqueTestName()` or equivalent
- [ ] Tests verified to pass with `-procs=4`
- [ ] No custom `randomSuffix()` functions added

### **Pre-commit Hook** (Coming Soon)

```bash
#!/bin/bash
# Will be added to .git/hooks/pre-commit
if grep -r 'time\.Now()\.Format("20060102' test/; then
    echo "❌ ERROR: Second-precision timestamp found"
    echo "Use testutil.UniqueTestName() instead"
    exit 1
fi
```

---

## 🏆 **Recognition**

**Thank you** to teams who migrate quickly! We'll track progress and recognize top performers:
- 🥇 **Gold**: Migrated within 1 week
- 🥈 **Silver**: Migrated within 2 weeks
- 🥉 **Bronze**: Migrated within sprint

Progress tracked in: `#kubernaut-testing` channel

---

## 📊 **Progress Tracking**

Update this section as services migrate:

| Service | Owner | Status | PR Link | Date Completed |
|---------|-------|--------|---------|----------------|
| AIAnalysis | @ai-team | ✅ Complete | [#1234](#) | 2025-12-11 |
| Gateway | @gateway-team | ⏳ In Progress | TBD | TBD |
| Notification | @notification-team | 📋 Planned | TBD | TBD |
| RemediationOrchestrator | @ro-team | 📋 Planned | TBD | TBD |
| WorkflowExecution | @we-team | 📋 Planned | TBD | TBD |
| SignalProcessing | @sp-team | 📋 Planned | TBD | TBD |
| DataStorage | @ds-team | 📋 Planned | TBD | TBD |

---

## 🚀 **Bottom Line**

**Three simple steps**:
1. Import `pkg/testutil`
2. Replace `randomSuffix()` with `testutil.UniqueTestName()`
3. Verify tests pass with `-procs=4`

**Time investment**: ~30 minutes per service
**Payoff**: Reliable tests, faster CI/CD, happier developers

**Questions?** Ask in `#kubernaut-testing`!

---

**Issued by**: Testing Team
**Date**: 2025-12-11
**Priority**: 🚨 **HIGH**
**Action Required**: ✅ **YES**

**Status**: 🔔 **ACTIVE NOTICE** - Please acknowledge receipt in `#kubernaut-testing`
