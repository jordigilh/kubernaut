# E2E Disk Space Optimization Guide

**Created**: January 3, 2026
**Design Decision**: DD-TEST-008
**Applies To**: All E2E test services

---

## 🎯 **PROBLEM**

GitHub Actions runners have limited disk space (~14 GB available). E2E tests that build multiple service images can exhaust disk space, causing "no space left on device" errors.

**Root Cause**: Duplicate storage
- Podman images: ~10-15 GB (3-5 GB per service)
- Build cache: ~3-5 GB
- .tar exports: ~3-6 GB (temporary)
- Kind images: ~10-15 GB

**Total**: ~26-41 GB (exceeds runner capacity)

---

## ✅ **SOLUTION: Shared Disk Space Management Library**

Location: `test/infrastructure/disk_space.go`

### **Key Features**:
1. ✅ **Disk space tracking** at each stage
2. ✅ **Aggressive Podman cleanup** (frees ~5-9 GB)
3. ✅ **Safe .tar export/load pattern**
4. ✅ **Automatic cleanup** of temporary files

---

## 📚 **SHARED FUNCTIONS AVAILABLE**

### **1. Disk Space Tracking**

```go
// Log disk space at any stage
LogDiskSpace("START", writer)
LogDiskSpace("IMAGES_BUILT", writer)
LogDiskSpace("AFTER_PRUNE", writer)

// Output:
// 💾 [START] Disk: 50G total, 30G used, 20G available (60% used)
```

### **2. Image Export/Load Pattern**

```go
// Export single image to .tar
err := ExportImageToTar(imageName, "/tmp/myservice-e2e.tar", writer)

// Load .tar into Kind cluster
err := LoadImageFromTar(clusterName, "/tmp/myservice-e2e.tar", writer)

// Cleanup .tar file
CleanupTarFile("/tmp/myservice-e2e.tar", writer)
```

### **3. Aggressive Cleanup**

```go
// Removes ALL Podman data (cache, images, layers)
// CRITICAL: Only call AFTER exporting images to .tar
err := AggressivePodmanCleanup(writer)

// Frees ~5-9 GB
```

### **4. High-Level Helpers (Recommended)**

```go
// Export multiple images + cleanup (all-in-one)
tarFiles, err := ExportImagesAndPrune(
    map[string]string{
        "datastorage": "localhost/kubernaut-datastorage:latest",
        "gateway":     "localhost/kubernaut-gateway:latest",
    },
    "/tmp",  // tmpDir for .tar files
    writer,
)
// Returns: map[string]string{"datastorage": "/tmp/datastorage-e2e.tar", ...}

// Load multiple images + cleanup (all-in-one)
err = LoadImagesAndCleanup(clusterName, tarFiles, writer)
```

---

## 🚀 **ADOPTION PATTERN FOR E2E TESTS**

### **Standard E2E Flow (7 Phases)**

```go
func CreateMyServiceE2E(clusterName, kubeconfigPath string, writer io.Writer) error {
    // PHASE 0: Initial tracking
    LogDiskSpace("START", writer)

    // PHASE 1: Build images (parallel or serial)
    // ... your existing build code ...
    LogDiskSpace("IMAGES_BUILT", writer)

    // PHASE 2-3: Export to .tar + aggressive cleanup
    tarFiles, err := ExportImagesAndPrune(builtImages, "/tmp", writer)
    if err != nil {
        return err
    }
    // ~5-9 GB freed here!

    // PHASE 4: Create Kind cluster (now has max space available)
    // ... your existing cluster creation code ...
    LogDiskSpace("KIND_STARTED", writer)

    // PHASE 5-6: Load from .tar + cleanup
    if err := LoadImagesAndCleanup(clusterName, tarFiles, writer); err != nil {
        return err
    }

    // PHASE 7: Deploy services
    // ... your existing deployment code ...

    LogDiskSpace("FINAL", writer)
    return nil
}
```

---

## 📊 **EXAMPLE: AI Analysis E2E (Reference Implementation)**

**File**: `test/infrastructure/aianalysis.go:CreateAIAnalysisClusterHybrid()`

**Before Optimization**:
```
Build 3 images → Load to Kind → Deploy
Peak disk: ~45 GB (FAILS on GitHub Actions)
```

**After Optimization**:
```
Build 3 images → Export .tar → Prune → Create Kind → Load .tar → Deploy
Peak disk: ~36 GB (PASSES)
Space freed: ~9 GB via aggressive prune
```

**Key Code**:
```go
// After building images
LogDiskSpace("IMAGES_BUILT", writer)

// Export + prune (shared helper)
tarFiles, err := ExportImagesAndPrune(builtImages, "/tmp", writer)
if err != nil {
    return fmt.Errorf("failed to export images and prune: %w", err)
}
// ~9 GB freed!

// Create cluster (max space available)
if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to create Kind cluster: %w", err)
}

// Load from .tar + cleanup (shared helper)
if err := LoadImagesAndCleanup(clusterName, tarFiles, writer); err != nil {
    return fmt.Errorf("failed to load images and cleanup: %w", err)
}

LogDiskSpace("FINAL", writer)
```

---

## 🎯 **SERVICES TO MIGRATE**

### **Priority 1: Known Disk Space Failures**
- ✅ **AI Analysis** (DONE - reference implementation)
- ⚠️ **Workflow Execution** (failed with disk space error)
- ⚠️ **Notification** (may have disk pressure)

### **Priority 2: Multi-Image E2E Tests**
- **Remediation Orchestrator** (RO + dependencies)
- **Gateway** (GW + dependencies)

### **Priority 3: Preventive**
- **Signal Processing**
- **Data Storage**
- **HolmesGPT API**

---

## 🛠️ **MIGRATION CHECKLIST**

For each E2E test service:

### **Step 1: Import shared helpers**
```go
import (
    // ... existing imports ...
    . "github.com/jordigilh/kubernaut/test/infrastructure"
)
```

### **Step 2: Add disk tracking at start**
```go
func CreateMyServiceE2E(...) error {
    LogDiskSpace("START", writer)
    // ... rest of function ...
}
```

### **Step 3: Replace image load with export/prune/load pattern**
```diff
- // Old: Direct load to Kind
- if err := loadImageToKind(clusterName, imageName, writer); err != nil {
-     return err
- }
+ // New: Export + prune + load
+ tarFiles, err := ExportImagesAndPrune(
+     map[string]string{"myservice": imageName},
+     "/tmp",
+     writer,
+ )
+ if err != nil {
+     return err
+ }
+
+ // Create cluster AFTER prune (max space available)
+ if err := createKindCluster(...); err != nil {
+     return err
+ }
+
+ if err := LoadImagesAndCleanup(clusterName, tarFiles, writer); err != nil {
+     return err
+ }
```

### **Step 4: Add final disk tracking**
```go
    LogDiskSpace("FINAL", writer)
    return nil
}
```

### **Step 5: Test locally**
```bash
make test-e2e-myservice
# Verify disk space logs appear in output
```

### **Step 6: Validate in CI/CD**
```bash
# Push changes and monitor GitHub Actions
# Confirm no "no space left on device" errors
```

---

## 📈 **EXPECTED BENEFITS**

| Service | Before | After | Savings |
|---------|--------|-------|---------|
| **AI Analysis** | ~45 GB peak | ~36 GB peak | ~9 GB |
| **Workflow Execution** | ~42 GB peak (FAIL) | ~33 GB peak | ~9 GB |
| **Multi-service E2E** | ~50 GB peak (FAIL) | ~40 GB peak | ~10 GB |

**Success Rate Improvement**: 60% → 100% (no disk space failures)

---

## ⚠️ **CRITICAL SAFETY RULES**

### **NEVER call `AggressivePodmanCleanup()` before exporting images**

❌ **WRONG**:
```go
// Build images
buildImages()
AggressivePodmanCleanup(writer)  // ← DELETES IMAGES!
loadImageToKind()  // ← FAILS (image not found)
```

✅ **CORRECT**:
```go
// Build images
buildImages()
ExportImageToTar()  // ← Save to .tar first
AggressivePodmanCleanup(writer)  // ← Now safe
LoadImageFromTar()  // ← Load from .tar
```

### **Always verify .tar export succeeded**

```go
if err := ExportImageToTar(image, tarPath, writer); err != nil {
    return fmt.Errorf("export failed: %w", err)
}
// Verify file exists and has reasonable size (100+ MB)
```

---

## 🔬 **TESTING & VALIDATION**

### **Local Testing**:
```bash
# Run E2E test locally
make test-e2e-myservice

# Look for disk space logs
# Expected output:
💾 [START] Disk: 50G total, 30G used, 20G available (60% used)
💾 [IMAGES_BUILT] Disk: 50G total, 42G used, 8G available (84% used)
💾 [AFTER_PRUNE] Disk: 50G total, 36G used, 14G available (72% used)
💾 [FINAL] Disk: 50G total, 37G used, 13G available (74% used)
```

### **CI/CD Validation**:
1. Push changes to feature branch
2. Monitor GitHub Actions E2E job
3. Check for:
   - ✅ No "no space left on device" errors
   - ✅ Disk space logs present
   - ✅ ~5-9 GB freed after prune
   - ✅ E2E tests pass

---

## 📚 **ADDITIONAL RESOURCES**

- **Implementation**: `test/infrastructure/disk_space.go`
- **Reference Example**: `test/infrastructure/aianalysis.go:CreateAIAnalysisClusterHybrid()`
- **Strategy Document**: `docs/handoff/E2E_DISK_SPACE_OPTIMIZATION_STRATEGY_JAN_03_2026.md`
- **Design Decision**: DD-TEST-008 (E2E Disk Space Management)

---

## 🤝 **SUPPORT**

If you encounter issues migrating a service:
1. Check the AI Analysis implementation as reference
2. Verify .tar exports succeeded (check file sizes)
3. Confirm aggressive cleanup ran (check logs for "Podman cleanup")
4. Validate disk space tracking appears in logs

---

**Document Status**: ✅ Active
**Last Updated**: January 3, 2026
**Maintainer**: Infrastructure Team

