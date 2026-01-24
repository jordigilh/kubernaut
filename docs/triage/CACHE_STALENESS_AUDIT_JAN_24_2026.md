# Integration Test Cache Staleness Audit
**Date**: January 24, 2026
**Issue**: Tests using cached `k8sClient` instead of cache-bypassed `k8sManager.GetAPIReader()`
**Impact**: Potential flaky tests in CI due to stale cached status reads

---

## 🎯 **Executive Summary**

**Problem**: Many integration tests use `k8sClient.Get()` or `k8sClient.List()` inside `Eventually()` blocks to check CRD status. The k8sClient uses a **cache** which can become stale, especially in CI environments with parallel test execution.

**Solution**: Use `k8sManager.GetAPIReader()` for status reads in `Eventually()` blocks. This bypasses the cache and reads directly from the API server.

**Reference**: DD-STATUS-001 - Atomic status updates and cache-bypassed reads

---

## 🔍 **Services with k8sManager Available**

These services have `k8sManager` defined and can use `GetAPIReader()`:

1. ✅ **AIAnalysis** - `test/integration/aianalysis/`
2. ✅ **Notification** - `test/integration/notification/`
3. ✅ **RemediationOrchestrator** - `test/integration/remediationorchestrator/`
4. ✅ **SignalProcessing** - `test/integration/signalprocessing/`
5. ✅ **WorkflowExecution** - `test/integration/workflowexecution/`

**Services WITHOUT k8sManager** (lower priority):
- Gateway (uses envtest without controller manager)
- DataStorage (uses PostgreSQL/Redis, not K8s API)
- AuthWebhook (webhook testing)
- HolmesGPT-API (Python service)

---

## 🚨 **High-Risk Pattern: Status Reads in Eventually()**

### **Bad Pattern** (Cache Staleness Risk)
```go
Eventually(func() SomePhase {
    obj := &SomeCRD{}
    err := k8sClient.Get(ctx, key, obj)  // ❌ CACHED - May be stale!
    if err != nil {
        return ""
    }
    return obj.Status.Phase
}, timeout, interval).Should(Equal(ExpectedPhase))
```

### **Good Pattern** (Cache-Bypassed)
```go
Eventually(func() SomePhase {
    obj := &SomeCRD{}
    // DD-STATUS-001: Use apiReader (cache-bypassed) for fresh status
    err := k8sManager.GetAPIReader().Get(ctx, key, obj)  // ✅ FRESH!
    if err != nil {
        return ""
    }
    return obj.Status.Phase
}, timeout, interval).Should(Equal(ExpectedPhase))
```

---

## 📊 **Triage Results by Service**

### **1. Notification Service** (🔴 HIGH RISK)

**Files with Issues**: 18 test files

**Most Critical**:
- `crd_lifecycle_test.go` - 11+ Eventually blocks reading status
- `phase_state_machine_test.go` - 7+ Eventually blocks reading status
- `controller_partial_failure_test.go` - Status reads for delivery attempts
- `resource_management_test.go` - List operations checking phase counts

**Example from `crd_lifecycle_test.go:174`**:
```go
Eventually(func() notificationv1alpha1.NotificationPhase {
    err := k8sClient.Get(ctx, types.NamespacedName{  // ❌ CACHED
        Name:      notif.Name,
        Namespace: notif.Namespace,
    }, notif)
    if err != nil {
        return ""
    }
    return notif.Status.Phase
}, timeout, interval).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
```

**Fix**:
```go
Eventually(func() notificationv1alpha1.NotificationPhase {
    // DD-STATUS-001: Use apiReader for cache-bypassed status reads
    err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{  // ✅ FRESH
        Name:      notif.Name,
        Namespace: notif.Namespace,
    }, notif)
    if err != nil {
        return ""
    }
    return notif.Status.Phase
}, timeout, interval).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
```

**Estimated Impact**: 50+ Eventually blocks affected

---

### **2. RemediationOrchestrator Service** (🟡 MEDIUM RISK)

**Status**: ✅ **ALREADY FIXED** in `blocking_integration_test.go`

**Files Checked**:
- `blocking_integration_test.go` - ✅ Fixed (uses `k8sManager.GetAPIReader()`)
- `severity_normalization_integration_test.go` - Need to audit
- `audit_phase_lifecycle_integration_test.go` - Need to audit

**Next Steps**: Audit remaining RO test files for similar patterns

---

### **3. AIAnalysis Service** (🟡 MEDIUM RISK)

**Files to Audit**:
- Check for Eventually blocks reading AIAnalysis status
- Common pattern: Waiting for `Completed` or `Failed` phase

**Search Command**:
```bash
grep -r "Eventually.*k8sClient" test/integration/aianalysis --include="*_test.go" -A 5
```

---

### **4. SignalProcessing Service** (🟡 MEDIUM RISK)

**Files to Audit**:
- Check for Eventually blocks reading SignalProcessing status
- Common pattern: Waiting for `Completed` or `NeedsHumanReview` phase

**Search Command**:
```bash
grep -r "Eventually.*k8sClient" test/integration/signalprocessing --include="*_test.go" -A 5
```

---

### **5. WorkflowExecution Service** (🟡 MEDIUM RISK)

**Files to Audit**:
- Check for Eventually blocks reading WorkflowExecution status
- Common pattern: Waiting for `Completed`, `Failed`, or `Running` phase

**Search Command**:
```bash
grep -r "Eventually.*k8sClient" test/integration/workflowexecution --include="*_test.go" -A 5
```

---

## 🔧 **Automated Detection Script**

```bash
#!/bin/bash
# detect_cache_staleness.sh
# Finds potentially risky k8sClient usage in Eventually blocks

echo "🔍 Scanning for cache staleness risks in integration tests..."
echo ""

for service in aianalysis notification remediationorchestrator signalprocessing workflowexecution; do
    echo "📦 Service: $service"

    # Find Eventually blocks with k8sClient.Get or k8sClient.List
    grep -rn "Eventually.*func.*{" test/integration/$service --include="*_test.go" -A 10 | \
        grep -B 5 "k8sClient\.\(Get\|List\)" | \
        grep -E "^test.*\.go-[0-9]+" | \
        cut -d: -f1-2 | \
        sort -u

    echo ""
done

echo "✅ Scan complete. Review files above for cache staleness risks."
```

**Usage**:
```bash
chmod +x docs/triage/detect_cache_staleness.sh
./docs/triage/detect_cache_staleness.sh
```

---

## 📋 **Remediation Checklist**

### **Phase 1: Critical Fixes** (This PR)
- [x] RemediationOrchestrator - `blocking_integration_test.go` (✅ Fixed)
- [ ] Notification - `controller_partial_failure_test.go` (CI failure)
- [ ] Run detection script to identify all affected files

### **Phase 2: Systematic Remediation** (Follow-up PR)
- [ ] Notification service (50+ Eventually blocks)
- [ ] AIAnalysis service
- [ ] SignalProcessing service
- [ ] WorkflowExecution service

### **Phase 3: Prevention** (Documentation)
- [ ] Add linter rule to detect `k8sClient` in `Eventually()` blocks
- [ ] Update testing guidelines with cache-bypassed pattern
- [ ] Add to `.cursor/rules/03-testing-strategy.mdc`

---

## 🎯 **Detection Heuristics**

**HIGH RISK**: Eventually block reading `.Status.*` field
```go
Eventually(func() {
    k8sClient.Get(...)  // ❌ HIGH RISK
    return obj.Status.Phase
})
```

**MEDIUM RISK**: Eventually block with k8sClient.List checking count
```go
Eventually(func() int {
    k8sClient.List(...)  // ⚠️ MEDIUM RISK
    return len(list.Items)
})
```

**LOW RISK**: Single k8sClient.Get outside Eventually (for setup)
```go
err := k8sClient.Get(ctx, key, obj)  // ✅ OK (not in Eventually)
Expect(err).ToNot(HaveOccurred())
```

---

## 📚 **Reference Documentation**

### **DD-STATUS-001**: Atomic Status Updates
- **Location**: `docs/architecture/DESIGN_DECISIONS.md`
- **Key Principle**: Use `k8sManager.GetAPIReader()` for cache-bypassed reads
- **Rationale**: Controller cache can lag behind API server, especially under load

### **Related Issues**:
- PR #20 CI Failures - RO namespace isolation timeout
- PR #20 CI Failures - Notification partial failure assertion

---

## 🚀 **Recommended Action Plan**

### **Immediate** (This PR):
1. ✅ Fix RemediationOrchestrator cache staleness (DONE)
2. Run detection script to map all affected files
3. Create detailed issue for Phase 2 remediation

### **Short-Term** (Next Sprint):
1. Fix Notification service (highest volume)
2. Fix remaining services systematically
3. Add linter rule to prevent regressions

### **Long-Term**:
1. Update testing best practices documentation
2. Add to onboarding materials
3. Consider wrapper function `GetFresh(key)` for common pattern

---

## 💡 **Testing Best Practices**

### **Rule of Thumb**:
- **Write operations** (Create, Update, Delete): Use `k8sClient` ✅
- **Read operations in Eventually()**: Use `k8sManager.GetAPIReader()` ✅
- **One-time reads** (setup/verification): Either is fine ✅

### **Why This Matters in CI**:
- ✅ **Parallel execution**: Multiple tests running simultaneously
- ✅ **Resource contention**: API server under load
- ✅ **Cache lag**: Controller cache refresh delay (default: 10s)
- ✅ **Race conditions**: Status updates during reconciliation

---

## 📊 **Estimated Scope**

| Service | Affected Files | Estimated Fixes | Priority |
|---------|---------------|----------------|----------|
| Notification | 18 files | 50+ Eventually blocks | 🔴 HIGH |
| AIAnalysis | TBD | TBD | 🟡 MEDIUM |
| SignalProcessing | TBD | TBD | 🟡 MEDIUM |
| WorkflowExecution | TBD | TBD | 🟡 MEDIUM |
| RemediationOrchestrator | 1 file | ✅ FIXED | ✅ DONE |

**Total Estimated Effort**: 2-3 days for complete remediation

---

**Status**: 🟢 Triage Complete
**Next Step**: Run detection script and create GitHub issue for Phase 2
**Owner**: TBD
