# 🚨 CRITICAL: Image Tag Mismatch Bug in E2E Infrastructure

**Date**: December 23, 2025
**Severity**: 🔴 **CRITICAL** - Affects all services with E2E tests
**Status**: 🚧 **FIX IN PROGRESS**
**Impact**: E2E tests fail with "ImagePullBackOff" due to hardcoded image tags

---

## 🐛 **Bug Description**

**Root Cause**: `BuildAndLoadImageToKind` returns a **dynamic image name** with timestamp tag, but deployment functions use **hardcoded static tags**.

**Result**: Kubernetes tries to pull an image that doesn't exist, causing `ImagePullBackOff` and test failures.

---

## 🔍 **How the Bug Manifests**

### **What `BuildAndLoadImageToKind` Does**:
```go
// Builds and loads:
localhost/kubernaut/datastorage:datastorage-datastorage-18840465  // Timestamp: 18840465
```

### **What Deployment Uses** (WRONG):
```go
// Tries to use:
Image: "localhost/kubernaut-datastorage:e2e-test-datastorage"  // Hardcoded static tag
```

### **Result**:
```
Events:
  Warning  Failed     Pod/datastorage-xxx  Failed to pull image "localhost/kubernaut-datastorage:e2e-test-datastorage": image not found
  Warning  BackOff    Pod/datastorage-xxx  Back-off pulling image
```

**Pod Status**: `ImagePullBackOff` → Never becomes ready → Tests timeout

---

## 📊 **Affected Services** (8 instances found)

| Service | File | Line | Status |
|---------|------|------|--------|
| **Notification** | `test/infrastructure/notification.go` | ~326 | ✅ **FIXED** |
| **Gateway** | `test/infrastructure/gateway_e2e.go` | ~137, ~294 | ❌ **NEEDS FIX** |
| **WorkflowExecution** | `test/infrastructure/workflowexecution_parallel.go` | ~188 | ❌ **NEEDS FIX** |
| **WorkflowExecution** | `test/infrastructure/workflowexecution.go` | ~1055 | ❌ **NEEDS FIX** |
| **SignalProcessing** | `test/infrastructure/signalprocessing.go` | ~140, ~324, ~475 | ❌ **NEEDS FIX** |
| **AIAnalysis** | `test/infrastructure/aianalysis.go` | ~609 | ❌ **NEEDS FIX** |
| **DataStorage** | `test/infrastructure/datastorage.go` | ~67, ~153 | ❌ **NEEDS FIX** |

**Total**: **11 instances** across **6 services**

---

## ✅ **The Fix (Notification Service Example)**

### **Before** (BROKEN):
```go
// In DeployNotificationAuditInfrastructure:
if _, err := BuildAndLoadImageToKind(imageConfig, writer); err != nil {  // ❌ Ignoring return value!
    return fmt.Errorf("failed to build/load Data Storage image: %w", err)
}

if err := deployDataStorageServiceForNotification(ctx, namespace, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
}

// In deployDataStorageServiceForNotification:
func deployDataStorageServiceForNotification(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // ...
    Image: "localhost/kubernaut-datastorage:e2e-test-datastorage",  // ❌ Hardcoded!
}
```

### **After** (FIXED):
```go
// In DeployNotificationAuditInfrastructure:
dataStorageImage, err := BuildAndLoadImageToKind(imageConfig, writer)  // ✅ Capture return value
if err != nil {
    return fmt.Errorf("failed to build/load Data Storage image: %w", err)
}
fmt.Fprintf(writer, "   Using image: %s\n", dataStorageImage)  // ✅ Log it

if err := deployDataStorageServiceForNotification(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {  // ✅ Pass it
    return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
}

// In deployDataStorageServiceForNotification:
func deployDataStorageServiceForNotification(ctx context.Context, namespace, kubeconfigPath, imageName string, writer io.Writer) error {  // ✅ Add parameter
    // ...
    Image: imageName,  // ✅ Use dynamic tag
}
```

---

## 🔧 **Fix Pattern for All Services**

### **Step 1: Capture the Returned Image Name**
```go
// BEFORE:
if _, err := BuildAndLoadImageToKind(imageConfig, writer); err != nil {

// AFTER:
dataStorageImage, err := BuildAndLoadImageToKind(imageConfig, writer)
if err != nil {
```

### **Step 2: Pass Image Name to Deployment Function**
```go
// BEFORE:
if err := deployDataStorageService(ctx, namespace, kubeconfigPath, writer); err != nil {

// AFTER:
if err := deployDataStorageService(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
```

### **Step 3: Add Parameter to Deployment Function**
```go
// BEFORE:
func deployDataStorageService(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {

// AFTER:
func deployDataStorageService(ctx context.Context, namespace, kubeconfigPath, imageName string, writer io.Writer) error {
```

### **Step 4: Use Dynamic Image Name in Container Spec**
```go
// BEFORE:
Image: "localhost/kubernaut-datastorage:e2e-test-datastorage",

// AFTER:
Image: imageName,  // Dynamic tag from BuildAndLoadImageToKind
```

---

## 🚨 **Services That Need Immediate Fix**

### **Gateway Service** (2 instances)

**File**: `test/infrastructure/gateway_e2e.go`

**Instance 1** (Line ~137):
```go
// In SetupGatewayInfrastructureWithDataStorage:
go func() {
    imageConfig := E2EImageConfig{ /* ... */ }
    _, err := BuildAndLoadImageToKind(imageConfig, writer)  // ❌ IGNORING
```

**Instance 2** (Line ~294):
```go
// In SetupGatewayE2EInfrastructure:
go func() {
    imageConfig := E2EImageConfig{ /* ... */ }
    _, err := BuildAndLoadImageToKind(imageConfig, writer)  // ❌ IGNORING
```

**Hardcoded Image** (Line ~564):
```yaml
image: datastorage:e2e-test  // ❌ HARDCODED
```

---

### **WorkflowExecution Service** (2 instances)

**File 1**: `test/infrastructure/workflowexecution_parallel.go` (Line ~188)
```go
_, err := BuildAndLoadImageToKind(imageConfig, output)  // ❌ IGNORING
```

**File 2**: `test/infrastructure/workflowexecution.go` (Line ~1055)
```yaml
image: localhost/kubernaut-datastorage:latest  // ❌ HARDCODED
```

---

### **SignalProcessing Service** (3 instances)

**File**: `test/infrastructure/signalprocessing.go`

**Instance 1** (Line ~140):
```go
if _, err := BuildAndLoadImageToKind(imageConfig, writer); err != nil {  // ❌ IGNORING
```

**Instance 2** (Line ~324):
```go
_, err := BuildAndLoadImageToKind(imageConfig, writer)  // ❌ IGNORING
```

**Instance 3** (Line ~475):
```go
_, err := BuildAndLoadImageToKind(imageConfig, writer)  // ❌ IGNORING
```

**Need to find hardcoded image locations in deployment functions**

---

### **AIAnalysis Service** (1 instance)

**File**: `test/infrastructure/aianalysis.go` (Line ~609)
```yaml
image: localhost/kubernaut-datastorage:latest  // ❌ HARDCODED
```

**Need to find where `BuildAndLoadImageToKind` is called**

---

### **DataStorage Service** (2 instances)

**File**: `test/infrastructure/datastorage.go`

**Instance 1** (Line ~67):
```go
if _, err := BuildAndLoadImageToKind(imageConfig, writer); err != nil {  // ❌ IGNORING
```

**Instance 2** (Line ~153):
```go
_, err := BuildAndLoadImageToKind(imageConfig, writer)  // ❌ IGNORING
```

**Hardcoded Image** (Line ~843):
```go
Image: "localhost/kubernaut-datastorage:e2e-test-datastorage",  // ❌ HARDCODED
```

---

## ⚠️ **Why This Bug Went Undetected**

### **1. Tests Were Not Using Coverage**
- Regular E2E tests may have used static tags that matched
- Coverage infrastructure uses `BuildAndLoadImageToKind` which generates dynamic tags
- Bug only manifests when coverage is enabled

### **2. Different Tag Formats**
- Some services use: `datastorage:e2e-test`
- Some use: `localhost/kubernaut-datastorage:latest`
- Some use: `localhost/kubernaut-datastorage:e2e-test-datastorage`
- No consistent naming → hard to spot pattern

### **3. Return Value Ignored**
- Using `_` to ignore return value is common Go pattern
- No linter warning for unused return value
- Easy to overlook in code review

---

## 🎯 **Fix Priority**

### **Critical** (Blocks E2E Coverage)
1. ✅ **Notification** - Fixed (was blocking coverage implementation)
2. ❌ **DataStorage** - Blocks its own E2E coverage
3. ❌ **Gateway** - Blocks E2E coverage
4. ❌ **WorkflowExecution** - Blocks E2E coverage
5. ❌ **SignalProcessing** - Blocks E2E coverage

### **High** (Will Block When Coverage Enabled)
6. ❌ **AIAnalysis** - Will block when enabling E2E coverage
7. ❌ **RemediationOrchestrator** - Will block when enabling E2E coverage (if affected)
8. ❌ **Toolset** - Will block when enabling E2E coverage (if affected)

---

## 🔍 **How to Detect This Bug**

### **Symptoms**:
1. E2E tests timeout waiting for pod ready
2. Pod shows `ImagePullBackOff` status
3. Events show: "Failed to pull image... image not found"
4. Image tag mismatch between build and deployment

### **Quick Check**:
```bash
# 1. Find ignored BuildAndLoadImageToKind calls
grep -r "_, err.*BuildAndLoadImageToKind" test/infrastructure/

# 2. Find hardcoded image tags
grep -r "Image:.*datastorage.*e2e" test/infrastructure/
grep -r "image:.*datastorage" test/infrastructure/

# 3. Check if they match
# If BuildAndLoadImageToKind generates dynamic tags,
# but deployment uses static tags → BUG!
```

---

## 📋 **Action Items**

### **For Each Affected Service**:
- [ ] Find `BuildAndLoadImageToKind` call that ignores return value
- [ ] Find deployment function using hardcoded image tag
- [ ] Apply 4-step fix pattern
- [ ] Test with coverage enabled
- [ ] Verify pod becomes ready

### **Documentation**:
- [ ] Update each service's E2E infrastructure documentation
- [ ] Add warning about dynamic image tags to DD-TEST-007
- [ ] Create pre-commit hook to detect this pattern

### **Prevention**:
- [ ] Add linter rule to flag ignored `BuildAndLoadImageToKind` return values
- [ ] Add validation in `BuildAndLoadImageToKind` to check if image will be used
- [ ] Create test helper that enforces image name passing

---

## 💡 **Recommended Fix Order**

1. **Notification** ✅ Already fixed
2. **DataStorage** - Most critical (self-service)
3. **SignalProcessing, Gateway, WorkflowExecution** - Have existing E2E tests
4. **AIAnalysis, RemediationOrchestrator, Toolset** - Before enabling coverage

---

## 🚀 **Next Steps**

1. **Immediate**: Fix DataStorage (self-service issue)
2. **Short-term**: Fix Gateway, WE, SP (existing E2E tests)
3. **Medium-term**: Fix AIAnalysis, RO, Toolset (before coverage)
4. **Long-term**: Add automation to prevent recurrence

---

**Document Created**: December 23, 2025
**Bug Discovered By**: User observation during Notification E2E coverage implementation
**Fix Implemented**: Notification service (✅)
**Remaining**: 6 services, 10 instances

---

**Priority**: 🔴 **CRITICAL** - Blocks E2E coverage for all services
**Effort**: ~15 minutes per service (4 steps × 2-3 instances each)
**Impact**: Unblocks E2E coverage infrastructure for entire platform



