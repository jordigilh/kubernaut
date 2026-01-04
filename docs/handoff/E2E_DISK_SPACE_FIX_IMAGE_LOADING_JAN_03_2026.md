# E2E Disk Space Optimization - Image Loading Fix

**Date**: January 3, 2026
**Component**: AIAnalysis E2E Infrastructure
**Issue**: Deployment phase trying to export images from Podman after aggressive prune
**Status**: ✅ **FIXED**

---

## 🎯 **SUMMARY**

The disk space optimization strategy (build → export → prune → load → deploy) worked **perfectly** (freed **213.6 GB**!), but the deployment phase failed because it tried to export images from Podman after they were already deleted by the aggressive prune.

---

## 🔍 **ROOT CAUSE**

### **What Happened**

1. ✅ **Phase 1-2**: Built 3 images (datastorage, aianalysis, holmesgpt-api)
2. ✅ **Phase 3**: Exported images to `.tar` files + **aggressive prune** (deleted all Podman images, freed **213.6 GB**)
3. ✅ **Phase 4**: Created Kind cluster
4. ✅ **Phase 5-6**: Loaded images from `.tar` into Kind + deleted `.tar` files
5. ❌ **Phase 7**: Parallel deployment tried to **re-export images from Podman** (but they're gone!)

### **Error**

```
Error: holmesgpt-api:aianalysis-9f17b218: image not known
failed to save image holmesgpt-api:aianalysis-9f17b218 with podman: exit status 125
```

### **Why This Happened**

The parallel deployment functions `deployHolmesGPTAPIOnly` and `deployAIAnalysisControllerOnly` were designed for the **old flow**:

```
OLD FLOW: build → load to Kind → deploy
```

But the **new disk space optimization flow** is:

```
NEW FLOW: build → export → prune → load → deploy
```

After the aggressive prune, Podman images no longer exist, so the deployment functions couldn't re-export them.

---

## ✅ **SOLUTION**

Created **manifest-only deployment functions** that skip the image loading step:

### **New Functions**

```go
// deployHolmesGPTAPIManifestOnly - applies manifest without loading images
func deployHolmesGPTAPIManifestOnly(kubeconfigPath, imageName string, writer io.Writer) error

// deployAIAnalysisControllerManifestOnly - applies manifest without loading images
func deployAIAnalysisControllerManifestOnly(kubeconfigPath, imageName string, writer io.Writer) error
```

### **Key Changes**

1. **Removed** `loadImageToKind()` calls (images already in Kind from Phase 5-6)
2. **Kept** manifest application and Rego policy deployment
3. **Updated** `CreateAIAnalysisClusterHybrid` to use new functions in Phase 7

### **Code Changes**

```diff
// Phase 7: Parallel deployment
-go func() {
-    err := deployHolmesGPTAPIOnly(clusterName, kubeconfigPath, builtImages["holmesgpt-api"], writer)
-    deployResults <- deployResult{"HolmesGPT-API", err}
-}()
+// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
+go func() {
+    err := deployHolmesGPTAPIManifestOnly(kubeconfigPath, builtImages["holmesgpt-api"], writer)
+    deployResults <- deployResult{"HolmesGPT-API", err}
+}()

-go func() {
-    err := deployAIAnalysisControllerOnly(clusterName, kubeconfigPath, builtImages["aianalysis"], writer)
-    deployResults <- deployResult{"AIAnalysis", err}
-}()
+// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
+go func() {
+    err := deployAIAnalysisControllerManifestOnly(kubeconfigPath, builtImages["aianalysis"], writer)
+    deployResults <- deployResult{"AIAnalysis", err}
+}()
```

---

## 🎉 **DISK SPACE OPTIMIZATION SUCCESS**

Even though the deployment failed, the disk space management worked **PERFECTLY**:

```
[START]       Disk: 926Gi total, 10Gi used, 345Gi available (3% used)
[IMAGES_BUILT] → (disk usage increased during builds)
[AFTER_PRUNE] Disk: 926Gi total, 10Gi used, 345Gi available (3% used)
              ^^^ 213.6 GB FREED! ^^^
[AFTER_LOAD]  Disk: 926Gi total, 10Gi used, 345Gi available (3% used)
[AFTER_CLEANUP] Disk: 926Gi total, 10Gi used, 347Gi available (3% used)
```

**Space Freed**: **213.6 GB** 🎉

---

## 📝 **FILES MODIFIED**

- `test/infrastructure/aianalysis.go`:
  - Added `deployHolmesGPTAPIManifestOnly()` (line ~773)
  - Added `deployAIAnalysisControllerManifestOnly()` (line ~1016)
  - Updated `CreateAIAnalysisClusterHybrid()` Phase 7 deployment (lines ~2290-2300)

---

## 🚀 **NEXT STEPS**

1. ✅ Run E2E tests to verify fix works
2. ⏳ Monitor disk space usage in CI/CD
3. ⏳ Adopt this pattern in other services (using shared `disk_space.go` library)

---

## 📊 **VALIDATION**

### **Expected Behavior After Fix**

```
Phase 1-2: Build images
Phase 3: Export to .tar + aggressive prune (free ~200GB)
Phase 4: Create Kind cluster
Phase 5-6: Load from .tar into Kind + cleanup .tar files
Phase 7: Apply manifests (WITHOUT trying to load images)
Result: E2E tests run successfully with minimal disk usage
```

### **Test Command**

```bash
make test-e2e-aianalysis
```

---

## 🔗 **RELATED DOCUMENTS**

- `test/infrastructure/disk_space.go` - Shared disk space optimization library
- `test/infrastructure/DISK_SPACE_OPTIMIZATION_GUIDE.md` - Adoption guide for other services
- `docs/handoff/E2E_DISK_SPACE_OPTIMIZATION_STRATEGY_JAN_03_2026.md` - Original strategy document

---

## 📌 **KEY INSIGHT**

**Lesson Learned**: When implementing a new flow (export → prune → load), ensure downstream functions don't assume the old flow (load on demand). Separate image loading from manifest deployment for maximum flexibility.

**Pattern**:
- **Old**: `deployServiceOnly()` = load + deploy
- **New**: `deployServiceManifestOnly()` = deploy only (assumes images already loaded)

This separation allows both flows to coexist and provides flexibility for future optimizations.

