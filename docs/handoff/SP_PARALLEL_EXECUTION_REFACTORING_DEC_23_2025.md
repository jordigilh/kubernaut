# SignalProcessing Parallel Execution Refactoring

**Date**: December 23, 2025
**Service**: SignalProcessing (SP)
**Status**: ✅ **REFACTORED** - Ready for parallel execution validation
**DD-TEST-002**: Compliance implementation complete

---

## 🎯 **Objective**

Refactor SignalProcessing integration tests to support parallel execution (`--procs=4`) per DD-TEST-002 standard, addressing all root causes documented in `TRIAGE_SP_INTEGRATION_TESTS_PARALLEL_FAILURES.md` (December 15, 2025).

---

## 📋 **Root Causes Addressed**

### **Root Cause 1: Nil Pointer Dereferences in AfterEach** ✅ FIXED

**Issue** (Line 64 in `hot_reloader_test.go`):
- Multiple processes accessing same cleanup code simultaneously
- `labelsPolicyFilePath` could be uninitialized in parallel processes
- No nil checks before accessing shared resources

**Fix Applied**:
```go
// BEFORE (vulnerable to race conditions + time.Sleep violation):
AfterEach(func() {
    By("Restoring original Rego policy to prevent test pollution")
    updateLabelsPolicyFile(originalLabelPolicy)
    time.Sleep(500 * time.Millisecond)  // ❌ Violates TESTING_GUIDELINES.md
})

// AFTER (thread-safe with nil check, no time.Sleep violation):
AfterEach(func() {
    // Skip if policy file path not initialized (parallel process safety)
    if labelsPolicyFilePath == "" {
        return
    }

    By("Restoring original Rego policy to prevent test pollution")
    updateLabelsPolicyFile(originalLabelPolicy)
    // Note: updateLabelsPolicyFile already includes 2s wait for fsnotify
    // No additional sleep needed (per TESTING_GUIDELINES.md - time.Sleep forbidden)
})
```

**File Modified**: `test/integration/signalprocessing/hot_reloader_test.go`

---

### **Root Cause 2: Shared Infrastructure Premature Shutdown** ✅ FIXED

**Issue**:
- Process 1 cleaned up shared infrastructure (DataStorage, PostgreSQL, Redis) while Processes 2-4 still running
- DataStorage container stopped mid-test
- Port 18094 became unavailable
- Connection refused errors in audit batch writes

**Fix Applied**:
```go
// BEFORE (race condition - all processes cleanup immediately):
var _ = AfterSuite(func() {
    // All cleanup happens in every process
    if dsInfra != nil {
        infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
    }
    // ... other cleanup
})

// AFTER (synchronized - shared cleanup waits for all processes):
var _ = SynchronizedAfterSuite(
    // ALL PROCESSES: Per-process cleanup
    func() {
        if cancel != nil {
            cancel()
        }
    },
    // PROCESS 1 ONLY: Shared infrastructure cleanup (runs ONCE after all processes finish)
    func() {
        if auditStore != nil {
            auditStore.Close()
        }
        if testEnv != nil {
            testEnv.Stop()
        }
        if dsInfra != nil {
            infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
        }
    },
)
```

**File Modified**: `test/integration/signalprocessing/suite_test.go`

**Pattern**: Follows Ginkgo's `SynchronizedAfterSuite` pattern for proper parallel test coordination

---

### **Root Cause 3: ConfigMap Cache Timing Issues** ✅ FIXED

**Issue**:
- Tests accessed ConfigMaps before controller manager cache synced
- "cache is not started, can not read objects" errors
- Used `time.Sleep(2 * time.Second)` anti-pattern

**Fix Applied**:
```go
// BEFORE (anti-pattern - fixed sleep):
By("Starting the controller manager")
go func() {
    defer GinkgoRecover()
    err = k8sManager.Start(ctx)
    Expect(err).ToNot(HaveOccurred(), "failed to run manager")
}()

// Wait for manager to be ready
time.Sleep(2 * time.Second)

// AFTER (proper cache sync wait with Eventually):
By("Starting the controller manager")
go func() {
    defer GinkgoRecover()
    err = k8sManager.Start(ctx)
    Expect(err).ToNot(HaveOccurred(), "failed to run manager")
}()

// DD-TEST-002: Wait for cache sync before tests run (prevents "cache not started" errors)
By("Waiting for manager cache to sync")
Eventually(func() bool {
    // Check if cache is synced by attempting to list namespaces
    var nsList corev1.NamespaceList
    err := k8sClient.List(ctx, &nsList)
    return err == nil
}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
    "Manager cache should sync within 30 seconds")
```

**File Modified**: `test/integration/signalprocessing/suite_test.go`

**Pattern**: Standard controller-runtime cache sync verification

---

## 🛠️ **Files Modified**

| File | Changes | Lines Modified |
|------|---------|---------------|
| `test/integration/signalprocessing/hot_reloader_test.go` | Added nil check in AfterEach | 94-101 |
| `test/integration/signalprocessing/suite_test.go` | Implemented SynchronizedAfterSuite | 500-554 |
| `test/integration/signalprocessing/suite_test.go` | Replaced sleep with cache sync wait | 471-483 |
| `Makefile` | Updated `--procs=1` → `--procs=4` | 963 |
| `Makefile` | Updated test-signalprocessing-all comments | 1027 |

---

## 📊 **Expected Impact**

### **Test Execution Performance**

| Metric | Before (Serial) | After (Parallel) | Improvement |
|--------|-----------------|------------------|-------------|
| **Parallel Processes** | 1 | 4 | 4x concurrency |
| **Integration Test Duration** | 132s (2.2 min) | ~35-40s (0.6-0.7 min) | **3-3.5x faster** |
| **CI/CD Pipeline Time** | Baseline | -90s saved | **~25% faster** |
| **Resource Utilization** | 25% (1/4 cores) | 100% (4/4 cores) | **4x better** |

### **DD-TEST-002 Compliance**

| Test Tier | Before | After | Status |
|-----------|--------|-------|--------|
| **Unit** | `--procs=4` | `--procs=4` | ✅ Compliant |
| **Integration** | `--procs=1` ❌ | `--procs=4` ✅ | **NOW COMPLIANT** |
| **E2E** | `--procs=4` | `--procs=4` | ✅ Compliant |

---

## ✅ **Validation Checklist**

### **Pre-Validation** (Completed)

- [x] Root Cause 1: AfterEach nil check implemented
- [x] Root Cause 2: SynchronizedAfterSuite implemented
- [x] Root Cause 3: Cache sync wait implemented
- [x] Makefile updated to use `--procs=4`
- [x] Documentation updated

### **Test Validation** (Pending)

- [ ] Run integration tests with `--procs=4`
- [ ] Verify all 88 specs pass (target: 100% success rate vs 1.6% before)
- [ ] Verify no flaky failures across 3 consecutive runs
- [ ] Verify duration reduces to ~35-40s (from 132s)
- [ ] Verify no "cache not started" errors in logs
- [ ] Verify no connection refused errors to DataStorage
- [ ] Verify no nil pointer dereferences in AfterEach

---

## 🧪 **Validation Commands**

### **Run Integration Tests (Parallel Mode)**
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Single run
make test-integration-signalprocessing

# Verify no flaky failures (3 consecutive runs)
for i in {1..3}; do
    echo "=== Run $i/3 ==="
    make test-integration-signalprocessing || exit 1
done
```

### **Monitor for Specific Issues**

**Check for cache errors**:
```bash
make test-integration-signalprocessing 2>&1 | grep "cache is not started"
# Expected: No output (issue fixed)
```

**Check for connection refused**:
```bash
make test-integration-signalprocessing 2>&1 | grep "connection refused"
# Expected: No output (issue fixed)
```

**Check for nil pointer dereferences**:
```bash
make test-integration-signalprocessing 2>&1 | grep "nil pointer dereference"
# Expected: No output (issue fixed)
```

---

## 📈 **Success Criteria**

| Criterion | Target | Measurement |
|-----------|--------|-------------|
| **Spec Success Rate** | 100% (88/88 specs) | All specs pass |
| **Duration** | ≤45s | Faster than 132s baseline |
| **Flaky Test Rate** | 0% | 3 consecutive clean runs |
| **Cache Errors** | 0 | No "cache not started" messages |
| **Connection Errors** | 0 | No "connection refused" messages |
| **Nil Pointer Errors** | 0 | No nil dereference panics |

---

## 🔗 **Related Documents**

1. **DD-TEST-002**: Parallel Test Execution Standard (`docs/architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md`)
2. **Root Cause Analysis**: TRIAGE_SP_INTEGRATION_TESTS_PARALLEL_FAILURES.md (December 15, 2025)
3. **Compliance Assessment**: SP_DD_TEST_002_COMPLIANCE_ASSESSMENT_DEC_23_2025.md (this refactoring addresses all gaps)

---

## 🎯 **Next Steps**

1. **Immediate**: Run validation tests to verify refactoring success
2. **Short-Term**: Update DD-TEST-002 compliance document with results
3. **Long-Term**: Apply similar refactoring patterns to other services with serial integration tests

---

## 📝 **Technical Notes**

### **Ginkgo Parallel Execution Patterns**

**SynchronizedBeforeSuite** (Already implemented):
```go
var _ = SynchronizedBeforeSuite(
    func() []byte { /* Process 1 ONLY - setup shared infra */ },
    func(data []byte) { /* ALL processes - setup local state */ },
)
```

**SynchronizedAfterSuite** (Newly implemented):
```go
var _ = SynchronizedAfterSuite(
    func() { /* ALL processes - per-process cleanup */ },
    func() { /* Process 1 ONLY - shared infra cleanup */ },
)
```

### **Thread Safety Patterns**

1. **Nil Checks**: Always validate shared resource initialization before access
2. **Mutex Protection**: Use `policyFileWriteMu` for file write synchronization
3. **Cache Sync**: Use `Eventually` with actual operations, not fixed sleeps
4. **Resource Isolation**: Each test uses unique namespaces (already implemented)

---

**Document Owner**: SignalProcessing Team
**Created**: December 23, 2025
**Status**: Ready for validation
**Priority**: 🔴 **HIGH** - DD-TEST-002 compliance requirement

