# TESTING_GUIDELINES.md - Version 2.0.0 Update

**Date**: 2025-12-13
**Type**: BREAKING CHANGE
**Status**: ✅ **IMPLEMENTED**

---

## 📋 **CHANGELOG**

### Version 2.0.0 (2025-12-13) - BREAKING CHANGE

#### **ADDED**
- ✅ New mandatory anti-pattern section: `time.Sleep()` is ABSOLUTELY FORBIDDEN
- ✅ Comprehensive `Eventually()` patterns and best practices
- ✅ Timeout configuration guidelines by test tier
- ✅ Migration examples from `time.Sleep()` to `Eventually()`
- ✅ Enforcement rules and CI checks
- ✅ Linter configuration for detecting anti-patterns

#### **BREAKING**
- 🚨 `time.Sleep()` is now **ABSOLUTELY FORBIDDEN** for waiting on asynchronous operations
- 🚨 All tests **MUST** use `Eventually()` for async waits
- 🚨 CI pipelines **MUST** detect and flag `time.Sleep()` anti-patterns

---

## 🚨 **NEW ANTI-PATTERN: time.Sleep() in Tests**

### **Policy**

**MANDATORY**: `time.Sleep()` calls are **ABSOLUTELY FORBIDDEN** in ALL test tiers for waiting on asynchronous operations, with **NO EXCEPTIONS**.

### **Why This Matters**

| Problem | Impact |
|---------|--------|
| **Flaky tests** | Fixed sleep durations cause intermittent failures |
| **Slow tests** | Always wait full duration even if condition met earlier |
| **Race conditions** | Sleep doesn't guarantee condition is met |
| **CI instability** | Different machine speeds cause test failures |

---

## ❌ **FORBIDDEN Patterns**

### **Pattern 1: Sleep Before Assertions**
```go
// ❌ FORBIDDEN
time.Sleep(5 * time.Second)
err := k8sClient.Get(ctx, key, &crd)
Expect(err).ToNot(HaveOccurred())
```

### **Pattern 2: Sleep After Processing**
```go
// ❌ FORBIDDEN
for i := 0; i < 20; i++ {
    time.Sleep(50 * time.Millisecond)
    sendRequest()
}
time.Sleep(5 * time.Second)  // Wait for all to process
```

### **Pattern 3: Sleep for Cache Sync**
```go
// ❌ FORBIDDEN
time.Sleep(100 * time.Millisecond)
list := k8sClient.List(ctx, &crdList)
```

---

## ✅ **REQUIRED Patterns**

### **Pattern 1: Eventually() for CRD Operations**
```go
// ✅ REQUIRED
Eventually(func() error {
    return k8sClient.Get(ctx, key, &crd)
}, 30*time.Second, 1*time.Second).Should(Succeed())
```

### **Pattern 2: Eventually() for Status Checks**
```go
// ✅ REQUIRED
Eventually(func() string {
    _ = k8sClient.Get(ctx, key, &crd)
    return crd.Status.Phase
}, 30*time.Second, 1*time.Second).Should(Equal("Ready"))
```

### **Pattern 3: Eventually() for Complex Searches**
```go
// ✅ REQUIRED
Eventually(func() *remediationv1alpha1.RemediationRequest {
    var crdList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &crdList, client.InNamespace(ns))
    if err != nil {
        return nil
    }

    for i := range crdList.Items {
        if crdList.Items[i].Spec.SignalLabels["process_id"] == processID {
            return &crdList.Items[i]
        }
    }
    return nil
}, 30*time.Second, 1*time.Second).ShouldNot(BeNil())
```

---

## 📊 **Timeout Configuration by Test Tier**

| Test Tier | Timeout | Interval | Rationale |
|-----------|---------|----------|-----------|
| **Unit** | 1-5s | 10-100ms | Fast, no I/O |
| **Integration** | 30-60s | 1-2s | Real K8s API |
| **E2E** | 2-5m | 5-10s | Full infrastructure |

---

## ✅ **Acceptable time.Sleep() Use Cases**

**ONLY acceptable when testing timing behavior itself:**

```go
// ✅ Acceptable: Testing rate limiting
It("should rate limit requests", func() {
    start := time.Now()
    // Make requests
    duration := time.Since(start)
    Expect(duration).To(BeNumerically(">=", expectedMinDuration))
})

// ✅ Acceptable: Intentional request staggering
for i := 0; i < 20; i++ {
    time.Sleep(50 * time.Millisecond)  // Create storm scenario
    sendRequest()
}
// But then use Eventually() to wait for processing!
Eventually(func() bool {
    return allRequestsProcessed()
}, 30*time.Second, 1*time.Second).Should(BeTrue())
```

---

## 🔍 **Migration Example: Storm Detection Test**

### **BEFORE (Anti-pattern)**
```go
// Send 20 alerts
for i := 1; i <= 20; i++ {
    time.Sleep(50 * time.Millisecond)
    sendAlert(i)
}

// ❌ BAD: Fixed sleep before checking
time.Sleep(5 * time.Second)

err := k8sClient.List(ctx, &crdList, client.InNamespace(ns))
Expect(err).ToNot(HaveOccurred())

// Search for CRD
for i := range crdList.Items {
    if crdList.Items[i].Spec.SignalLabels["process_id"] == processID {
        testRR = &crdList.Items[i]
        break
    }
}
Expect(testRR).ToNot(BeNil())  // May fail if processing not complete
```

### **AFTER (Correct pattern)**
```go
// Send 20 alerts
for i := 1; i <= 20; i++ {
    time.Sleep(50 * time.Millisecond)  // ✅ Acceptable: intentional stagger
    sendAlert(i)
}

// ✅ GOOD: Eventually() waits until condition is met
Eventually(func() *remediationv1alpha1.RemediationRequest {
    var crdList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &crdList, client.InNamespace(ns))
    if err != nil {
        return nil  // Retry on error
    }

    processIDStr := fmt.Sprintf("%d", processID)
    for i := range crdList.Items {
        if crdList.Items[i].Spec.SignalLabels["process_id"] == processIDStr {
            return &crdList.Items[i]
        }
    }
    return nil  // Not found yet, retry
}, 30*time.Second, 1*time.Second).ShouldNot(BeNil(),
    "Should find RemediationRequest with process_id=%s", processIDStr)
```

### **Benefits**
- ✅ **Faster**: Returns immediately when CRD is found (not after 5s)
- ✅ **Reliable**: Retries until found or timeout (no race conditions)
- ✅ **Clear**: Failure message shows what was not found
- ✅ **CI-stable**: Works across different machine speeds

---

## 🚨 **Enforcement**

### **CI Pipeline Check**
```bash
# Detect time.Sleep() before assertions
if grep -A 5 "time\.Sleep" test/ --include="*_test.go" | \
   grep -E "Expect|Should|Get|List|Create|Update" | \
   grep -v "^Binary"; then
    echo "⚠️  WARNING: Detected time.Sleep() anti-pattern"
    echo "   Use Eventually() instead"
    exit 1
fi
```

### **Linter Rule**
```yaml
# .golangci.yml
linters-settings:
  forbidigo:
    forbid:
      - pattern: 'time\.Sleep\([^)]+\)\s*\n\s*(Expect|Should|err\s*:?=)'
        msg: "time.Sleep() before assertions is forbidden - use Eventually()"
```

---

## 📋 **Action Items for Existing Tests**

### **Immediate**
- [ ] Review all test files for `time.Sleep()` usage
- [ ] Identify async wait patterns (sleep before Get/List/assertions)
- [ ] Migrate to `Eventually()` patterns

### **CI/CD**
- [ ] Add CI check for `time.Sleep()` anti-patterns
- [ ] Update linter configuration
- [ ] Add pre-commit hook (optional)

### **Documentation**
- [x] Update TESTING_GUIDELINES.md to v2.0.0
- [x] Add changelog
- [x] Document migration examples
- [x] Specify acceptable use cases

---

## 📊 **Impact Assessment**

### **Tests Affected**
Based on current triage:
- **Gateway Integration**: 1 test (storm detection) ← **PRIMARY EXAMPLE**
- **Other Services**: TBD (needs audit)

### **Migration Effort**
- **Per Test**: 5-10 minutes
- **Risk**: Low (test-only changes)
- **Benefits**: High (eliminates flakiness)

---

## 🎯 **Summary**

```
╔════════════════════════════════════════════════════════════╗
║       TESTING_GUIDELINES.md - Version 2.0.0                ║
╠════════════════════════════════════════════════════════════╣
║ Status:        ✅ UPDATED                                  ║
║ Type:          BREAKING CHANGE                             ║
║ New Policy:    time.Sleep() ABSOLUTELY FORBIDDEN           ║
║ Required:      Eventually() for all async waits            ║
║ Impact:        All test tiers (unit, integration, E2E)     ║
║ Enforcement:   CI checks + linter rules                    ║
╚════════════════════════════════════════════════════════════╝
```

---

**Status**: ✅ **COMPLETE** - TESTING_GUIDELINES.md updated to v2.0.0

