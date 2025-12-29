# CRITICAL: Infrastructure Build Failure - Blocking All Integration Tests

**Date**: December 23, 2025, 6:50 PM
**From**: SignalProcessing Team
**To**: Infrastructure/Gateway Team
**Priority**: 🔴 **CRITICAL** - Blocking ALL integration test execution
**Status**: 🚨 **BLOCKING**

---

## 🚨 **Critical Issue**

**ALL integration tests are currently broken** due to undefined function references in `test/infrastructure/` package:

```
# github.com/jordigilh/kubernaut/test/infrastructure
../../infrastructure/gateway_e2e.go:130:18: undefined: buildDataStorageImage
../../infrastructure/gateway_e2e.go:132:24: undefined: loadDataStorageImage
../../infrastructure/notification.go:317:12: undefined: buildDataStorageImage
../../infrastructure/notification.go:324:12: undefined: loadDataStorageImage
../../infrastructure/signalprocessing.go:132:12: undefined: buildDataStorageImage
../../infrastructure/signalprocessing.go:315:18: undefined: buildDataStorageImage
../../infrastructure/signalprocessing.go:460:18: undefined: buildDataStorageImage
../../infrastructure/workflowexecution_parallel.go:178:10: undefined: buildDataStorageImage
```

---

## 🔍 **Root Cause**

Functions were renamed with service-specific suffixes but callers weren't updated:

### **What Exists** (in `signalprocessing.go`):
```go
func loadDataStorageImageForSP(writer io.Writer) error {
```

### **What's Being Called** (in multiple files):
```go
buildDataStorageImage(...)  // ❌ UNDEFINED
loadDataStorageImage(...)   // ❌ UNDEFINED
```

---

## 📋 **Affected Files**

1. `test/infrastructure/gateway_e2e.go` (lines 130, 132, 279, 281)
2. `test/infrastructure/notification.go` (lines 317, 324)
3. `test/infrastructure/signalprocessing.go` (lines 132, 315, 460)
4. `test/infrastructure/workflowexecution_parallel.go` (line 178)

---

## ✅ **Required Fix**

**Option A**: Revert to shared function names (recommended for DRY principle):
```go
// In datastorage_bootstrap.go or shared.go:
func buildDataStorageImage(serviceName string, writer io.Writer) (string, error) {
    // Shared implementation
}

func loadDataStorageImage(tag string, clusterName string, writer io.Writer) error {
    // Shared implementation
}
```

**Option B**: Update all callers to use service-specific names:
```go
// In gateway_e2e.go:
buildDataStorageImageForGateway(...)
loadDataStorageImageForGateway(...)

// In notification.go:
buildDataStorageImageForNT(...)
loadDataStorageImageForNT(...)

// etc.
```

**Recommendation**: **Option A** - These functions are identical across services, so they should be shared.

---

## 🎯 **Impact**

**Services Unable to Run Integration Tests**:
- ❌ SignalProcessing
- ❌ Gateway
- ❌ Notification
- ❌ WorkflowExecution
- ❌ (Likely all services using DataStorage bootstrap)

**Blocked Work**:
- SignalProcessing parallel execution validation
- Gateway E2E testing
- Notification integration testing
- WorkflowExecution testing

---

## 🚀 **Validation Command**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-signalprocessing  # Should compile without errors
```

---

## 📝 **Likely Cause**

Recent refactoring to make functions service-specific (for better organization) but:
1. Not all files were updated
2. Or merge conflict resolution missed some callers
3. Or incomplete commit pushed

---

## ⏰ **Urgency**

**CRITICAL**: This blocks **all integration testing** across **all services**. Should be fixed immediately.

---

**Contact**: SignalProcessing Team
**Discovered**: December 23, 2025, 6:50 PM
**Context**: While validating parallel execution fixes for SP integration tests




