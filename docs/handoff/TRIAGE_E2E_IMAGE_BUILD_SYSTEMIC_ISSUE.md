# TRIAGE: E2E Image Build Systemic Issue

**Date**: 2025-12-15
**Triage Type**: Infrastructure bug analysis
**Severity**: 🔴 **CRITICAL** - Blocks all E2E tests for multiple services
**Status**: 🔴 **SYSTEMIC ISSUE IDENTIFIED**

---

## 🎯 **Executive Summary**

**Problem**: E2E test infrastructure has a **systemic image naming inconsistency** that blocks tests across multiple services.

**Impact**:
- ❌ AIAnalysis E2E tests fail (BeforeSuite)
- ❌ WorkflowExecution E2E tests likely fail
- ✅ Data Storage E2E tests work (builds correctly)
- ✅ Gateway E2E tests work (builds correctly)

**Root Cause**: Services that **depend on Data Storage** build the DS image **without `localhost/` prefix** but try to load it **with `localhost/` prefix**.

---

## 🔍 **DETAILED ANALYSIS**

### **Pattern Identified**:

#### ✅ **CORRECT** Pattern (Data Storage, Gateway):
```go
// BUILD with localhost/ prefix
podman build -t localhost/kubernaut-datastorage:e2e-test ...

// LOAD with localhost/ prefix (matches build)
loadImageToKind(clusterName, "kubernaut-datastorage:e2e-test", ...)
// Which internally does: podman save localhost/kubernaut-datastorage:e2e-test
```

#### ❌ **BROKEN** Pattern (AIAnalysis, WorkflowExecution):
```go
// BUILD WITHOUT localhost/ prefix
podman build -t kubernaut-datastorage:latest ...

// LOAD WITH localhost/ prefix (MISMATCH!)
loadImageToKind(clusterName, "kubernaut-datastorage:latest", ...)
// Which internally does: podman save localhost/kubernaut-datastorage:latest
// ❌ ERROR: Image doesn't exist with that name!
```

---

## 📊 **SERVICE-BY-SERVICE BREAKDOWN**

### **1. AIAnalysis E2E** (`test/infrastructure/aianalysis.go`)

**Dependencies Built**:
1. ❌ **Data Storage**: Built as `kubernaut-datastorage:latest` (line 450)
2. ❌ **HolmesGPT-API**: Built as `localhost/kubernaut-holmesgpt-api:latest` (line 587, FIXED)
3. ❌ **AIAnalysis Controller**: NOT built (assumes exists as `localhost/kubernaut-aianalysis:latest`)

**Issues**:
```go
// Line 450: WRONG - no localhost/ prefix
buildCmd := exec.Command("podman", "build", "-t", "kubernaut-datastorage:latest",
    "-f", "docker/data-storage.Dockerfile", ".")

// Line 461: Tries to load with localhost/ prefix
loadImageToKind(clusterName, "kubernaut-datastorage:latest", writer)
// This will fail because image is kubernaut-datastorage:latest, not localhost/...
```

**Missing**:
- AIAnalysis controller image is NEVER built
- Assumes `localhost/kubernaut-aianalysis:latest` already exists

---

### **2. WorkflowExecution E2E** (`test/infrastructure/workflowexecution.go`)

**Dependencies Built**:
1. ❌ **Data Storage**: Built as `kubernaut-datastorage:latest` (line 579)

**Issues**:
```go
// Line 579: WRONG - no localhost/ prefix
buildCmd := exec.Command("podman", "build", "-t", "kubernaut-datastorage:latest",
    "-f", "docker/data-storage.Dockerfile", ".")

// Line 598: Tries to load with localhost/ prefix
loadImageToKind(clusterName, "kubernaut-datastorage:latest", output)
// This will fail because image is kubernaut-datastorage:latest, not localhost/...
```

---

### **3. Data Storage E2E** (`test/infrastructure/datastorage.go`) ✅

**Dependencies Built**:
1. ✅ **Data Storage**: Built as `localhost/kubernaut-datastorage:e2e-test` (line 1047)

**Correct Pattern**:
```go
// Line 1047: CORRECT - with localhost/ prefix
buildCmd := exec.Command("podman", "build",
    "-t", "localhost/kubernaut-datastorage:e2e-test",
    "-f", "docker/datastorage-ubi9.Dockerfile",
    ".")

// Loads correctly with matching name
```

**Why It Works**: Consistent naming throughout

---

### **4. Gateway E2E** (`test/infrastructure/gateway_e2e.go`) ✅

**Dependencies Built**:
1. ✅ **Gateway**: Built as `localhost/kubernaut-gateway:e2e-test` (line 333)
2. ✅ **Data Storage**: Uses shared `deployDataStorage` from aianalysis.go (BROKEN!)

**Issues**:
```go
// Line 272: Reuses AIAnalysis's broken deployDataStorage function
deployDataStorage("gateway-e2e", kubeconfigPath, writer)
// This will fail with the same Data Storage build bug
```

**Gateway's Own Image**: ✅ Built correctly with `localhost/` prefix

---

## 🔴 **SYSTEMIC PROBLEMS**

### **Problem 1: Inconsistent Image Naming**

**Root Cause**: No standardized build function for dependencies

**Affected Services**:
- AIAnalysis E2E (Data Storage, HolmesGPT-API)
- WorkflowExecution E2E (Data Storage)
- Gateway E2E (Data Storage via shared function)

### **Problem 2: Missing Image Builds**

**Services NOT Building Their Own Images**:
- ❌ AIAnalysis controller (`localhost/kubernaut-aianalysis:latest`)
- ❌ WorkflowExecution controller (likely)
- ❌ SignalProcessing controller (likely)
- ❌ Notification controller (likely)

**Impact**: E2E tests assume images already exist from prior builds

### **Problem 3: No `--no-cache` Flag**

**Current Behavior**: Uses cached layers from previous builds

**Problem**: E2E tests may test stale code if images aren't rebuilt

**Missing**: `--no-cache` flag on all `podman build` commands

---

## ⚠️ **SCOPE LIMITATION**

**AIAnalysis Team Scope**: This triage identified a systemic issue but **only fixes AIAnalysis service**.

**Other Teams**: WorkflowExecution, Gateway, SignalProcessing, Notification, and RO teams must fix their own services.

---

## ✅ **FIXES APPLIED (AIAnalysis Service Only)**

### **AIAnalysis Service Fixes** ✅ **COMPLETE**

**Fixed in `test/infrastructure/aianalysis.go`**:
1. ✅ Data Storage image: Added `localhost/` prefix + `--no-cache`
2. ✅ HolmesGPT-API image: Added `--no-cache` (prefix already fixed)
3. ✅ AIAnalysis controller image: Added `localhost/` prefix + `--no-cache`

**Result**: AIAnalysis E2E tests should now work correctly.

---

## 📋 **RECOMMENDED FIXES FOR OTHER TEAMS**

### **Fix 1: Standardize Data Storage Image Build** 🔴 **CRITICAL** (Other Teams)

Create a shared, correct Data Storage build function:

```go
// In test/infrastructure/shared.go (new file)
func buildAndLoadDataStorage(clusterName, kubeconfigPath string, writer io.Writer) error {
    projectRoot := getProjectRoot()

    // Build with localhost/ prefix
    fmt.Fprintln(writer, "  Building Data Storage image...")
    buildCmd := exec.Command("podman", "build",
        "--no-cache",  // Always build fresh
        "-t", "localhost/kubernaut-datastorage:e2e-test",
        "-f", "docker/datastorage-ubi9.Dockerfile",
        ".")
    buildCmd.Dir = projectRoot
    buildCmd.Stdout = writer
    buildCmd.Stderr = writer
    if err := buildCmd.Run(); err != nil {
        return fmt.Errorf("failed to build Data Storage: %w", err)
    }

    // Load with matching name
    if err := loadImageToKind(clusterName, "kubernaut-datastorage:e2e-test", writer); err != nil {
        return fmt.Errorf("failed to load Data Storage: %w", err)
    }

    return nil
}
```

**Update All Services**:
- AIAnalysis: Replace `deployDataStorage` build logic
- WorkflowExecution: Replace `deployDataStorageWithConfig` build logic
- Gateway: Use new shared function

---

### **Fix 2: Build Service-Specific Images** 🔴 **CRITICAL**

**AIAnalysis** must build its own controller image:

```go
func buildAndLoadAIAnalysisController(clusterName string, writer io.Writer) error {
    projectRoot := getProjectRoot()

    fmt.Fprintln(writer, "  Building AIAnalysis controller image...")
    buildCmd := exec.Command("podman", "build",
        "--no-cache",  // Always build fresh
        "-t", "localhost/kubernaut-aianalysis:latest",
        "-f", "docker/aianalysis.Dockerfile",
        ".")
    buildCmd.Dir = projectRoot
    buildCmd.Stdout = writer
    buildCmd.Stderr = writer
    if err := buildCmd.Run(); err != nil {
        return fmt.Errorf("failed to build AIAnalysis controller: %w", err)
    }

    if err := loadImageToKind(clusterName, "kubernaut-aianalysis:latest", writer); err != nil {
        return fmt.Errorf("failed to load AIAnalysis controller: %w", err)
    }

    return nil
}
```

**Apply Same Pattern** to:
- WorkflowExecution
- SignalProcessing
- Notification
- RemediationOrchestrator

---

### **Fix 3: Add `--no-cache` Flag** 🟡 **HIGH PRIORITY**

**All `podman build` commands** should use `--no-cache`:

```go
buildCmd := exec.Command("podman", "build",
    "--no-cache",  // Ensure fresh build
    "-t", "localhost/kubernaut-service:tag",
    "-f", "docker/service.Dockerfile",
    ".")
```

**Why**: E2E tests should always test the **latest code**, not cached layers

---

### **Fix 4: Update `loadImageToKind`** 🟢 **NICE TO HAVE**

Make `loadImageToKind` more robust:

```go
func loadImageToKind(clusterName, imageName string, writer io.Writer) error {
    // Ensure imageName has localhost/ prefix for Podman
    if !strings.HasPrefix(imageName, "localhost/") {
        imageName = "localhost/" + imageName
    }

    // ... rest of function
}
```

**OR** (Better): **Validate at call site** that image name is correct

---

## 📋 **AFFECTED FILES**

| File | Issue | Status | Owner |
|---|---|---|---|
| `test/infrastructure/aianalysis.go` | DS build: no localhost/ prefix | ✅ **FIXED** | AIAnalysis team |
| `test/infrastructure/aianalysis.go` | AIAnalysis controller: no --no-cache | ✅ **FIXED** | AIAnalysis team |
| `test/infrastructure/aianalysis.go` | HolmesGPT-API: no --no-cache | ✅ **FIXED** | AIAnalysis team |
| `test/infrastructure/workflowexecution.go` | DS build: no localhost/ prefix | ⏸️ **NOT FIXED** | WorkflowExecution team |
| `test/infrastructure/workflowexecution.go` | WFE controller: not built | ⏸️ **NOT FIXED** | WorkflowExecution team |
| `test/infrastructure/gateway_e2e.go` | DS via shared function (broken) | ⏸️ **NOT FIXED** | Gateway team |
| `test/infrastructure/datastorage.go` | ✅ Correct pattern | **Use as reference** | N/A |
| **Other services** | No --no-cache flag | ⏸️ **NOT FIXED** | Respective teams |

---

## 🎯 **PRIORITY FIXES**

### **Immediate** (Unblock E2E tests):

1. 🔴 **Fix AIAnalysis Data Storage build** (add `localhost/` prefix)
2. 🔴 **Fix WorkflowExecution Data Storage build** (add `localhost/` prefix)
3. 🔴 **Build AIAnalysis controller image** (add build function)

### **Short-Term** (Complete E2E test suite):

4. 🟡 **Create shared Data Storage build function**
5. 🟡 **Add `--no-cache` to all builds**
6. 🟡 **Build all service-specific controller images**

### **Long-Term** (Architecture improvement):

7. 🟢 **Create standardized image build library**
8. 🟢 **Add image build validation tests**
9. 🟢 **Document image naming standards**

---

## 💡 **ROOT CAUSE ANALYSIS**

### **Why Did This Happen?**

1. **Inconsistent Patterns**: Different services evolved independently
2. **Copy-Paste Development**: Bugs propagated across services
3. **Missing Validation**: No checks that images exist before loading
4. **Implicit Dependencies**: Tests assume images built externally

### **How to Prevent**:

1. ✅ **Standardized Build Library**: All services use same functions
2. ✅ **Image Build Tests**: Validate image builds before E2E tests
3. ✅ **Documentation**: Clear image naming standards
4. ✅ **Code Review**: Check for localhost/ prefix consistency

---

## 📊 **IMPACT ASSESSMENT**

### **Current State**:

| Service | E2E Tests Status | Root Cause |
|---|---|---|
| **Data Storage** | ✅ Working | Correct image naming |
| **Gateway** | ❌ Likely broken | Uses broken shared DS function |
| **AIAnalysis** | ❌ Broken | DS + HolmesGPT + missing controller build |
| **WorkflowExecution** | ❌ Likely broken | DS build + missing controller build |
| **SignalProcessing** | ❓ Unknown | Needs investigation |
| **Notification** | ❓ Unknown | Needs investigation |
| **RemediationOrchestrator** | ❓ Unknown | Needs investigation |

### **After Fixes**:

| Service | E2E Tests Status | Action Required |
|---|---|---|
| **Data Storage** | ✅ Working | None (reference implementation) |
| **Gateway** | ✅ Will work | Use fixed shared DS function |
| **AIAnalysis** | ✅ Will work | Fix DS + build controller |
| **WorkflowExecution** | ✅ Will work | Fix DS + build controller |
| **SignalProcessing** | 🟡 To verify | Add controller build |
| **Notification** | 🟡 To verify | Add controller build |
| **RemediationOrchestrator** | 🟡 To verify | Add controller build |

---

## 🚀 **RECOMMENDED ACTION PLAN**

### **Phase 1: AIAnalysis Fixes** ✅ **COMPLETE**

1. ✅ Fixed AIAnalysis Data Storage build (localhost/ prefix + --no-cache)
2. ✅ Fixed AIAnalysis HolmesGPT-API build (--no-cache)
3. ✅ Fixed AIAnalysis controller build (localhost/ prefix + --no-cache)

### **Phase 2: Other Teams** (NOT AIAnalysis scope)

4. ⏸️ WorkflowExecution team: Fix Data Storage build
5. ⏸️ Gateway team: Fix shared Data Storage function
6. ⏸️ Other controller teams: Add build functions for their services

### **Phase 2: Shared Infrastructure** (1-2 hours)

5. Create shared Data Storage build function
6. Update all services to use shared function
7. Add --no-cache flag to all builds

### **Phase 3: Complete Coverage** (2-3 hours)

8. Add build functions for all controllers
9. Verify all E2E tests pass
10. Document image naming standards

**Total Estimated Time**: **5-8 hours**

---

## ✅ **SUCCESS CRITERIA**

- [ ] All E2E tests build required images from scratch
- [ ] All images use `localhost/` prefix consistently
- [ ] All builds use `--no-cache` flag
- [ ] Shared infrastructure (DS, Redis, PostgreSQL) has standardized build functions
- [ ] Each service builds its own controller image
- [ ] Documentation updated with image naming standards

---

## 📚 **RELATED DOCUMENTS**

- `docs/handoff/AA_SESSION_FINAL_STATUS.md` - AIAnalysis E2E blocker
- Commit `93564b8e` - HolmesGPT-API image name fix (partial)

---

**Maintained By**: Infrastructure Team
**Last Updated**: December 15, 2025
**Status**: 🔴 **SYSTEMIC ISSUE - REQUIRES COORDINATED FIX**
**Priority**: **P0 - BLOCKS ALL E2E TESTS**

