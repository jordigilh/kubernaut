# E2E DataStorage Image Tag Pattern Audit - December 26, 2025

**Date**: December 26, 2025  
**Issue**: Services using incorrect DataStorage image tag patterns  
**Impact**: Testing against cached/old DataStorage code instead of fresh builds  
**Status**: 🔧 IN PROGRESS

---

## 🎯 **Correct Pattern (Per User Requirement)**

Each service MUST build its OWN fresh DataStorage image with latest code:

```
Phase 0: Generate dynamic tag ONCE (before any build)
Phase 1: Build DataStorage with that dynamic tag
Phase 3: Load DataStorage with that dynamic tag  
Phase 4: Deploy DataStorage with that dynamic tag
```

**Result**: `localhost/datastorage:workflowexecution-a1b2c3d4` (single tag, fresh build)

---

## ❌ **WRONG Pattern: Shared Cached Image (Phase 3.5 Re-tagging)**

```
Phase 1: Build with fixed tag "e2e-test-datastorage"
Phase 3: Load with fixed tag "e2e-test-datastorage"
Phase 3.5: Re-tag to dynamic tag "workflowexecution-a1b2c3d4"
Phase 4: Deploy with dynamic tag
```

**Problem**: All services share the SAME cached `e2e-test-datastorage` image!
- If DataStorage code changes, ALL services still use the old cached image
- Defeats the purpose of building at runtime
- Tests run against STALE DataStorage code ❌

---

## ❌ **WRONG Pattern: Tag Mismatch (Deploy-Time Generation)**

```
Phase 1: Build with fixed tag "e2e-test-datastorage"
Phase 3: Load with fixed tag "e2e-test-datastorage"
Phase 4: Generate NEW dynamic tag + deploy
```

**Problem**: Deploy generates a DIFFERENT tag than what was loaded!
- Build/Load: `e2e-test-datastorage`
- Deploy: `workflowexecution-xyz123` (newly generated, doesn't exist!)
- Kubernetes can't find the image → Pod fails to start ❌

---

## 📊 **Audit Results**

| Service | Pattern | Issue | Status |
|---------|---------|-------|--------|
| **WorkflowExecution** | ✅ Correct (Phase 0 generation) | None | ✅ FIXED |
| **RemediationOrchestrator** | ❌ Phase 3.5 Re-tagging | Shared cached image | 🔧 TO FIX |
| **Gateway** | ❌ Deploy-time generation | Tag mismatch | 🔧 TO FIX |
| **SignalProcessing** | ❌ Deploy-time generation | Tag mismatch | 🔧 TO FIX |

---

## 🔍 **Detailed Findings**

### 1. RemediationOrchestrator (WRONG: Phase 3.5 Re-tagging)

**Current Code**:
```go
// Phase 1: Build with fixed tag
buildDataStorageImage(writer) 
  → builds "localhost/kubernaut-datastorage:e2e-test-datastorage"

// Phase 3: Generate dynamic tag (AFTER build!)
dataStorageImageName := GenerateInfraImageName("datastorage", "remediationorchestrator")

// Phase 3: Load with fixed tag
loadDataStorageImage(clusterName, writer)
  → loads "e2e-test-datastorage"

// Phase 3.5: Re-tag to dynamic
tagDataStorageImageInKind(clusterName, dataStorageImageName, writer)
  → creates alias "remediationorchestrator-a1b2c3d4" pointing to SAME image

// Phase 4: Deploy with dynamic tag
deployDataStorageServiceInNamespace(..., dataStorageImageName, ...)
```

**Problem**: All services share the `e2e-test-datastorage` image!

---

### 2. Gateway (WRONG: Deploy-Time Generation)

**Current Code**:
```go
// Phase 1: Build with fixed tag
buildDataStorageImage(writer)
  → builds "localhost/kubernaut-datastorage:e2e-test-datastorage"

// Phase 3: Load with fixed tag
loadDataStorageImage(clusterName, writer)
  → loads "e2e-test-datastorage"

// Phase 4: Generate dynamic tag AT DEPLOY TIME (WRONG!)
deployDataStorageServiceInNamespace(..., GenerateInfraImageName("datastorage", "gateway"), ...)
  → looks for "gateway-xyz123" (doesn't exist!)
```

**Problem**: Tag generated at deploy time doesn't match loaded image!

---

### 3. SignalProcessing (WRONG: Deploy-Time Generation)

**Current Code**: Same pattern as Gateway
- Builds with fixed tag
- Loads with fixed tag
- Generates dynamic tag AT DEPLOY TIME (mismatch!)

---

## ✅ **Correct Implementation (WorkflowExecution)**

```go
// Phase 0: Generate tag BEFORE building
dataStorageImageName := GenerateInfraImageName("datastorage", "workflowexecution")

// Phase 1: Build with dynamic tag
buildDataStorageImageWithTag(dataStorageImageName, writer)
  → builds "localhost/datastorage:workflowexecution-a1b2c3d4"

// Phase 3: Load with SAME dynamic tag
loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)
  → loads "workflowexecution-a1b2c3d4"

// Phase 4: Deploy with SAME dynamic tag
deployDataStorageServiceInNamespace(..., dataStorageImageName, ...)
  → uses "workflowexecution-a1b2c3d4" ✅
```

**Benefits**:
- ✅ Each service builds FRESH DataStorage with latest code
- ✅ Tag is consistent across build → load → deploy
- ✅ No shared cached images
- ✅ Parallel E2E isolation maintained

---

## 🔧 **Required Changes**

### All Services Must:

1. **Add Phase 0**: Generate dynamic tag BEFORE Phase 1
2. **Modify Phase 1**: Use `buildDataStorageImageWithTag(tag, writer)`
3. **Modify Phase 3**: Use `loadDataStorageImageWithTag(cluster, tag, writer)`
4. **Remove Phase 3.5**: No re-tagging needed
5. **Phase 4**: Use the tag from Phase 0

---

## 📋 **Implementation Checklist**

- [x] WorkflowExecution: Implement Phase 0 tag generation
- [ ] RemediationOrchestrator: Remove Phase 3.5, add Phase 0
- [ ] Gateway: Add Phase 0, fix deploy-time generation
- [ ] SignalProcessing: Add Phase 0, fix deploy-time generation
- [ ] Remove `tagDataStorageImageInKind()` function (obsolete)
- [ ] Update all documentation
- [ ] Validate all services build fresh DataStorage images

---

## 🎯 **Success Criteria**

Each service MUST:
1. ✅ Generate dynamic tag ONCE in Phase 0
2. ✅ Build DataStorage with that specific tag
3. ✅ Load DataStorage with that specific tag
4. ✅ Deploy DataStorage with that specific tag
5. ✅ NO shared cached images across services
6. ✅ NO re-tagging operations (Phase 3.5)

---

**Status**: 🔧 IN PROGRESS  
**Priority**: HIGH (correctness of E2E tests)  
**Next Steps**: Fix RemediationOrchestrator, Gateway, SignalProcessing

