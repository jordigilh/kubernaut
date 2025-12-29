# BUG REPORT: DataStorage E2E Tests Overwriting ~/.kube/config

**Date**: December 16, 2025
**Severity**: 🔴 **HIGH** - Overwrites user's default kubeconfig
**Status**: ⚠️ **ACTIVE BUG**
**Reporter**: User observation during V1.0 test verification

---

## 📋 **Bug Description**

DataStorage E2E tests are overwriting `~/.kube/config` instead of creating an isolated kubeconfig file at `~/.kube/datastorage-e2e-config`.

**Impact**: Users lose their default Kubernetes cluster configuration when running DataStorage E2E tests.

---

## 🔍 **Root Cause Analysis**

### **Authoritative Standard** (TESTING_GUIDELINES.md)

```
Kubeconfig Path: ~/.kube/{service}-e2e-config
DataStorage:     ~/.kube/datastorage-e2e-config
Cluster Name:    datastorage-e2e
```

**Reference**: [TESTING_GUIDELINES.md lines 901-995](../../development/business-requirements/TESTING_GUIDELINES.md)

---

### **Current Implementation Analysis**

#### ✅ **Test Suite (CORRECT)**

**File**: `test/e2e/datastorage/datastorage_e2e_suite_test.go:115`

```go
// ✅ CORRECT: Suite is using the right path
kubeconfigPath = fmt.Sprintf("%s/.kube/datastorage-e2e-config", homeDir)
```

**Status**: Compliant with TESTING_GUIDELINES.md

---

#### ❌ **Infrastructure Code (BUG)**

**File**: `test/infrastructure/datastorage.go:1014-1016`

```go
// ❌ BUG: Missing --kubeconfig flag
cmd := exec.Command("kind", "create", "cluster",
    "--name", clusterName,
    "--config", configPath)  // ← Missing: "--kubeconfig", kubeconfigPath
```

**Problem**: Without the `--kubeconfig` flag, Kind uses the default `~/.kube/config` location, overwriting the user's configuration.

---

## 🔧 **Fix Required**

### **File**: `test/infrastructure/datastorage.go`

**Line 1014-1016** - Add `--kubeconfig` flag:

```go
// BEFORE (INCORRECT - Missing --kubeconfig flag)
cmd := exec.Command("kind", "create", "cluster",
    "--name", clusterName,
    "--config", configPath)

// AFTER (CORRECT - Add --kubeconfig flag)
cmd := exec.Command("kind", "create", "cluster",
    "--name", clusterName,
    "--config", configPath,
    "--kubeconfig", kubeconfigPath)  // ← ADD THIS LINE
```

**Effort**: 1 line change (5 seconds)

---

## ✅ **Verification Pattern (From Other Services)**

### **AIAnalysis Service** (CORRECT IMPLEMENTATION)

**File**: `test/infrastructure/aianalysis.go:292-297`

```go
// ✅ CORRECT: AIAnalysis uses --kubeconfig flag
cmd := exec.Command("kind", "create", "cluster",
    "--name", clusterName,
    "--config", configPath,
    "--kubeconfig", kubeconfigPath,  // ← Present and correct
)
```

### **SignalProcessing Service** (CORRECT IMPLEMENTATION)

**File**: `test/infrastructure/signalprocessing.go:448-453`

```go
// ✅ CORRECT: SignalProcessing uses --kubeconfig flag
cmd := exec.Command("kind", "create", "cluster",
    "--name", clusterName,
    "--config", configPath,
    "--kubeconfig", kubeconfigPath,  // ← Present and correct
)
```

### **WorkflowExecution Service** (CORRECT IMPLEMENTATION)

**File**: `test/infrastructure/workflowexecution.go:78-82`

```go
// ✅ CORRECT: WorkflowExecution uses --kubeconfig flag
createCmd := exec.Command("kind", "create", "cluster",
    "--name", clusterName,
    "--config", configPath,
    "--kubeconfig", kubeconfigPath,  // ← Present and correct
)
```

---

## 🎯 **Why This Was Missed**

1. **Test suite uses correct path** - The bug is in the infrastructure helper function
2. **Kind defaults to ~/.kube/config** - No error occurs, just silent overwrite
3. **E2E tests still pass** - Tests work regardless of kubeconfig location
4. **Only discovered during parallel test runs** - User noticed their main kubeconfig was overwritten

---

## 📊 **Impact Assessment**

| **Category** | **Impact** | **Severity** |
|--------------|------------|--------------|
| **Data Loss** | User's main kubeconfig overwritten | 🔴 **HIGH** |
| **Test Isolation** | Cannot run multiple E2E tests in parallel | 🟠 **MEDIUM** |
| **User Experience** | Unexpected cluster context changes | 🟠 **MEDIUM** |
| **Production** | No impact (only affects E2E test runs) | 🟢 **LOW** |

---

## 🚀 **Recommended Fix (Immediate)**

### **Step 1: Fix the Bug**

```bash
# Edit the file
vim test/infrastructure/datastorage.go

# Add --kubeconfig flag at line 1017 (after --config line)
```

### **Step 2: Verify Fix**

```bash
# 1. Check if user's kubeconfig will be preserved
cp ~/.kube/config ~/.kube/config.backup

# 2. Run DataStorage E2E tests
make test-e2e-datastorage

# 3. Verify isolated kubeconfig was created
ls -la ~/.kube/datastorage-e2e-config  # Should exist

# 4. Verify user's config was NOT overwritten
diff ~/.kube/config ~/.kube/config.backup  # Should be identical
```

### **Step 3: Test Parallel Execution**

```bash
# Run multiple E2E tests simultaneously (should not conflict)
make test-e2e-datastorage & make test-e2e-gateway &
```

---

## 📚 **Related Documentation**

- [TESTING_GUIDELINES.md - Kubeconfig Isolation Policy](../../development/business-requirements/TESTING_GUIDELINES.md)
- [REQUEST_GATEWAY_KUBECONFIG_STANDARDIZATION.md](./REQUEST_GATEWAY_KUBECONFIG_STANDARDIZATION.md)
- [REQUEST_WORKFLOWEXECUTION_KUBECONFIG_STANDARDIZATION.md](./REQUEST_WORKFLOWEXECUTION_KUBECONFIG_STANDARDIZATION.md)

---

## ✅ **Success Criteria**

- [x] **Bug identified** - Missing `--kubeconfig` flag in `createKindCluster` function
- [ ] **Fix applied** - Add `--kubeconfig` parameter to Kind command
- [ ] **Verification passed** - User's `~/.kube/config` is NOT overwritten
- [ ] **Isolated config created** - `~/.kube/datastorage-e2e-config` is created correctly
- [ ] **Tests pass** - E2E tests work with isolated kubeconfig

---

## 🔗 **Cross-Service Consistency Check**

| **Service** | **File** | **Status** | **Line** |
|------------|----------|------------|----------|
| AIAnalysis | `test/infrastructure/aianalysis.go` | ✅ **CORRECT** | 296 |
| SignalProcessing | `test/infrastructure/signalprocessing.go` | ✅ **CORRECT** | 452 |
| WorkflowExecution | `test/infrastructure/workflowexecution.go` | ✅ **CORRECT** | 81 |
| **DataStorage** | `test/infrastructure/datastorage.go` | ❌ **BUG** | 1014 |
| Gateway | `test/infrastructure/gateway_e2e.go` | ⚠️ **TODO: Verify** | - |
| RemediationOrchestrator | `test/infrastructure/remediationorchestrator.go` | ⚠️ **TODO: Verify** | - |
| Notification | `test/infrastructure/notification.go` | ⚠️ **TODO: Verify** | - |

---

**Priority**: 🔴 **P0 - Fix Before Next E2E Test Run**

**Effort**: 5 seconds (1 line addition)

**Risk**: None - Adding `--kubeconfig` flag is a safe, backward-compatible change

---

**Date**: December 16, 2025
**Discovered During**: V1.0 Final Test Verification
**Reported By**: User observing kubeconfig overwrite behavior




