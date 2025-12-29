# Notification Integration Test - Audit Store Parallel Execution Issue

**Date**: December 23, 2025
**Status**: 🔍 **ROOT CAUSE IDENTIFIED**
**Previous Issue**: Parallel infrastructure conflicts - ✅ **RESOLVED**
**Current Issue**: Shared audit store causing "send on closed channel" panics

---

## Progress Summary

### ✅ Resolved: Parallel Infrastructure Conflicts
- **Fix Applied**: Converted `BeforeSuite` to `SynchronizedBeforeSuite`
- **Result**: Infrastructure properly shared across 4 parallel processes
- **Validation**: No more "network already exists" or "port already in use" errors
- **Test Results**: 52/65 tests passing before timeout

### ❌ New Issue: Audit Store Shared State

**Error Pattern**:
```
panic: send on closed channel [recovered]
github.com/jordigilh/kubernaut/pkg/audit.(*BufferedAuditStore).StoreAudit
```

**Root Cause**:
The `realAuditStore` global variable is shared across all 4 parallel processes. When one process closes it in `AfterSuite`, other processes still try to use it, causing panics.

---

## Technical Analysis

### Current Code Pattern (INCORRECT for Parallel Tests)

```go:test/integration/notification/suite_test.go
var (
    realAuditStore audit.AuditStore // ❌ SHARED across ALL processes
)

var _ = SynchronizedBeforeSuite(func() []byte {
    // Process 1: Creates shared infrastructure
    ...
}, func(data []byte) {
    // ALL processes: Create audit store
    realAuditStore, err = audit.NewBufferedStore(...) // ❌ Shared global
    ...
})

var _ = AfterSuite(func() {
    // ❌ ALL processes run this - first one closes the shared store!
    if realAuditStore != nil {
        realAuditStore.Close()
    }
})
```

### Problem Sequence

1. Process 1 completes first, runs `AfterSuite`
2. Process 1 closes `realAuditStore` (shared global)
3. Process 2, 3, 4 still running tests
4. Process 2, 3, 4 try to send audit events → **PANIC: send on closed channel**

---

## Solution Options

### Option A: Per-Process Audit Store (Recommended)

Each process gets its own audit store instance:

```go
var (
    processAuditStores = make(map[int]audit.AuditStore) // Map by process ID
    auditStoreMutex    sync.Mutex
)

func getProcessAuditStore() audit.AuditStore {
    auditStoreMutex.Lock()
    defer auditStoreMutex.Unlock()

    pid := GinkgoParallelProcess()
    if store, ok := processAuditStores[pid]; ok {
        return store
    }

    // Create new store for this process
    store, err := audit.NewBufferedStore(...)
    Expect(err).ToNot(HaveOccurred())
    processAuditStores[pid] = store
    return store
}
```

### Option B: Reference-Counted Audit Store

Track how many processes are using the store:

```go
var (
    realAuditStore audit.AuditStore
    storeRefCount  int32 // atomic
)

// In SynchronizedBeforeSuite second function:
atomic.AddInt32(&storeRefCount, 1)

// In AfterSuite:
if atomic.AddInt32(&storeRefCount, -1) == 0 {
    // Last process - safe to close
    realAuditStore.Close()
}
```

### Option C: SynchronizedAfterSuite for Cleanup

Use `SynchronizedAfterSuite` instead of `AfterSuite`:

```go
var _ = SynchronizedAfterSuite(func() {
    // FIRST: Each process does per-process cleanup
    // NO audit store closing here

}, func() {
    // SECOND: Only process 1 - close shared resources
    if realAuditStore != nil {
        realAuditStore.Close()
    }
})
```

---

## Recommended Approach

**Use Option C: `SynchronizedAfterSuite`**

**Rationale**:
- ✅ Simplest implementation
- ✅ Matches `SynchronizedBeforeSuite` pattern
- ✅ No need for complex process tracking
- ✅ Already used by other services (Gateway, DataStorage)

---

## Implementation Checklist

- [ ] Convert `AfterSuite` to `SynchronizedAfterSuite`
- [ ] Move `realAuditStore.Close()` to second function (process 1 only)
- [ ] Move mock server cleanup to first function (all processes)
- [ ] Test with `--procs=4` to validate fix
- [ ] Confirm all 129 tests pass

---

## Current Test Results

```
✅ Infrastructure Setup: WORKING (no conflicts)
✅ Tests Passing: 52/65 before timeout
❌ Tests Failing: 13 (all audit-related)
⏱️  Runtime: 5m11s (timed out at 5m)
```

**Failed Test Categories**:
- Controller Audit Event Emission (9 failures)
- Notification Audit Integration Tests (4 failures)

All failures have the same root cause: "send on closed channel" in audit store.

---

**Next Action**: Implement Option C (`SynchronizedAfterSuite`) to fix audit store cleanup race condition



