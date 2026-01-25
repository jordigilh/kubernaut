# WorkflowExecution Migration - Partial Completion

**Date**: January 7, 2026  
**Status**: ⚠️ **PARTIAL** - PHASE 1 and 3 migrated, deployment functions need complex refactoring  
**Next Step**: Refactor deployment functions or defer to next session

---

## What Was Completed

### ✅ PHASE 1 (Build) - COMPLETE
**File**: `test/infrastructure/workflowexecution_e2e_hybrid.go` (lines 96-152)

**Changes Applied**:
- ✅ Removed PHASE 0 manual tag generation
- ✅ Replaced `BuildWorkflowExecutionImageWithCoverage()` with `BuildImageForKind()`
- ✅ Replaced `buildDataStorageImageWithTag()` with `BuildImageForKind()`
- ✅ Added `builtImages map[string]string` to track image names
- ✅ Updated build result structure to include `imageName`

### ✅ PHASE 3 (Load) - COMPLETE
**File**: `test/infrastructure/workflowexecution_e2e_hybrid.go` (lines 215-265)

**Changes Applied**:
- ✅ Replaced `LoadWorkflowExecutionCoverageImage()` with `LoadImageToKind()`
- ✅ Replaced `loadDataStorageImageWithTag()` with `LoadImageToKind()`
- ✅ Uses `builtImages` map for image names
- ✅ Updated load result structure

### ✅ PHASE 4 (Deployment) - PARTIAL
**File**: `test/infrastructure/workflowexecution_e2e_hybrid.go` (lines 267-308)

**Changes Applied**:
- ✅ Updated DataStorage deployment to use `builtImages["DataStorage"]`

---

## ⚠️ What Remains - COMPLEX DEPLOYMENT REFACTORING

### Issue: Redundant Build/Load in Deployment Function

**Problem**:
The `DeployWorkflowExecutionController()` function (lines 603-749) currently:
1. **Builds the image again** (lines 632-666) - REDUNDANT
2. **Loads it to Kind again** (lines 668-688) - REDUNDANT
3. Uses hardcoded image name in deployment (line 978)

This conflicts with consolidated API where we already built and loaded in PHASE 1 and 3.

### Required Changes

#### 1. Update `DeployWorkflowExecutionController()` Signature
**Current** (line 603):
```go
func DeployWorkflowExecutionController(ctx context.Context, namespace, kubeconfigPath string, output io.Writer) error {
```

**Needed**:
```go
func DeployWorkflowExecutionController(ctx context.Context, namespace, kubeconfigPath, imageName string, output io.Writer) error {
```

#### 2. Remove Redundant Build/Load Logic
**Remove lines 632-700**:
- Image building (lines 632-666)
- Tar save/load (lines 668-688)
- Podman cleanup (lines 690-700)

**Keep lines 702-748**:
- Static resource application
- Cleanup of existing deployment/service
- Call to `deployWorkflowExecutionControllerDeployment()`

#### 3. Update `deployWorkflowExecutionControllerDeployment()` Signature
**Current** (line 932):
```go
func deployWorkflowExecutionControllerDeployment(ctx context.Context, namespace, kubeconfigPath string, output io.Writer) error {
```

**Needed**:
```go
func deployWorkflowExecutionControllerDeployment(ctx context.Context, namespace, kubeconfigPath, imageName string, output io.Writer) error {
```

#### 4. Update Hardcoded Image in Deployment Spec
**Current** (line 978):
```go
Image: "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution",
```

**Needed**:
```go
Image: imageName, // Per Consolidated API Migration (January 2026)
```

#### 5. Update PHASE 4 Call
**Current** (line 306):
```go
err := DeployWorkflowExecutionController(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer)
```

**Needed**:
```go
wfeImage := builtImages["WorkflowExecution (coverage)"]
err := DeployWorkflowExecutionController(ctx, WorkflowExecutionNamespace, kubeconfigPath, wfeImage, writer)
```

---

## Risk Assessment

### Complexity Level: **MEDIUM-HIGH**
- Multiple function signatures to change
- Nested function calls
- Large function with many moving parts

### Validation Required: **MANDATORY**
- Compile test after changes
- Run WorkflowExecution E2E tests
- Verify pods use correct dynamic image

### Estimated Time: **20-30 minutes**
- Function refactoring: 10-15 min
- Compilation fixes: 5 min
- Validation: 5-10 min

---

## Decision Point

### Option A: Complete Now (~20-30 min)
- Finish WorkflowExecution migration
- Proceed to AIAnalysis
- Complete all 3 remaining services

### Option B: Defer WorkflowExecution
- Move to AIAnalysis (simpler?)
- Return to WorkflowExecution later
- Risk: Different patterns across services

### Option C: Stop Here
- Document current state
- HandoffEOF
wc -l /Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/handoff/WFE_MIGRATION_PARTIAL_STATUS_JAN07.md
