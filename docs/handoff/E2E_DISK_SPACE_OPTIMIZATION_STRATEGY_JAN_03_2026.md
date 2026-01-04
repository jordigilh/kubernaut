# E2E Disk Space Optimization Strategy

**Date**: January 3, 2026 15:30 PST
**Context**: AI Analysis E2E tests failing due to disk space exhaustion
**Goal**: Aggressive disk space management with tracking at each stage

---

## 🎯 **PROBLEM STATEMENT**

### **Current Issues**:
- ❌ AI Analysis E2E tests fail with "no space left on device"
- ❌ Image builds consume ~10-15 GB total (3 services × 3-5 GB each)
- ❌ Duplicate storage: Podman cache + Kind images = 2x disk usage
- ❌ GitHub Actions runners have limited disk space (~14 GB available)

### **Current Fixes Applied**:
1. ✅ Podman image cleanup after Kind load (commits 2db193760, 47e4fc784)
2. ✅ `.tar` file deletion after Kind load

### **Remaining Challenge**:
- ⚠️ **Podman build cache** still consumes significant space during parallel builds
- ⚠️ **No visibility** into disk space at each stage for diagnostics

---

## 💡 **PROPOSED SOLUTION**

### **Strategy: Aggressive Cleanup + Disk Space Tracking**

```
PHASE 1: Build images (parallel)         → Track disk space
PHASE 2: Export images to .tar files     → Track disk space
PHASE 3: Podman system prune (AGGRESSIVE) → Track disk space freed
PHASE 4: Create Kind cluster              → Track disk space
PHASE 5: Load images from .tar into Kind  → Track disk space
PHASE 6: Delete .tar files                → Track disk space
```

**Key Innovation**: `podman system prune -a` AFTER builds but BEFORE Kind starts

**Benefits**:
1. ✅ **Removes build cache** (~3-5 GB freed)
2. ✅ **Removes intermediate layers** (~2-4 GB freed)
3. ✅ **Keeps final images as .tar files** (safe for Kind load)
4. ✅ **Total savings**: ~5-9 GB (enough to prevent failures)
5. ✅ **Diagnostic visibility**: Track disk space at every stage

---

## 🛠️ **IMPLEMENTATION**

### **Helper Function: Disk Space Tracker**

```go
// getDiskSpaceInfo returns disk space info in human-readable format
func getDiskSpaceInfo() (total, used, available string, err error) {
	// Use 'df -h /' to get root filesystem stats
	cmd := exec.Command("df", "-h", "/")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get disk space: %w", err)
	}

	// Parse output (skip header line)
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return "", "", "", fmt.Errorf("unexpected df output")
	}

	// Example output:
	// Filesystem      Size  Used Avail Use% Mounted on
	// /dev/sda1        50G   35G   15G  70% /
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return "", "", "", fmt.Errorf("unexpected df fields")
	}

	return fields[1], fields[2], fields[3], nil
}

// logDiskSpace logs disk space at a specific stage
func logDiskSpace(stage string, writer io.Writer) {
	total, used, available, err := getDiskSpaceInfo()
	if err != nil {
		fmt.Fprintf(writer, "  ⚠️  [%s] Failed to get disk space: %v\n", stage, err)
		return
	}

	fmt.Fprintf(writer, "  💾 [%s] Disk: %s total, %s used, %s available\n",
		stage, total, used, available)
}
```

### **Modified CreateAIAnalysisClusterHybrid Flow**

```go
func CreateAIAnalysisClusterHybrid(clusterName, kubeconfigPath string, writer io.Writer) error {
	ctx := context.Background()
	namespace := "kubernaut-system"

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 AIAnalysis E2E Infrastructure (HYBRID PARALLEL + DISK OPTIMIZATION)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ═══════════════════════════════════════════════════════════════════════
	// STAGE 0: Initial disk space
	// ═══════════════════════════════════════════════════════════════════════
	logDiskSpace("START", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")

	// ... existing parallel build code ...

	fmt.Fprintln(writer, "\n✅ All images built!")
	logDiskSpace("IMAGES_BUILT", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Export images to .tar files (prepare for Kind load)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 2: Exporting images to .tar files...")

	tarFiles := make(map[string]string)
	for name, image := range builtImages {
		tarPath := fmt.Sprintf("/tmp/%s-e2e.tar", name)
		tarFiles[name] = tarPath

		fmt.Fprintf(writer, "  📦 Exporting %s...\n", name)
		saveCmd := exec.Command("podman", "save", "-o", tarPath, image)
		saveCmd.Stdout = writer
		saveCmd.Stderr = writer
		if err := saveCmd.Run(); err != nil {
			return fmt.Errorf("failed to export %s image: %w", name, err)
		}
		fmt.Fprintf(writer, "  ✅ %s exported to %s\n", name, tarPath)
	}

	logDiskSpace("TAR_EXTRACTED", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: AGGRESSIVE PODMAN CLEANUP (before Kind starts)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n🗑️  PHASE 3: Aggressive Podman cleanup (before Kind)...")
	fmt.Fprintln(writer, "  ⚠️  This removes ALL Podman data (images, cache, layers)")
	fmt.Fprintln(writer, "  ✅ Safe: Final images are preserved as .tar files")

	// Run podman system prune -a (removes everything except running containers)
	pruneCmd := exec.Command("podman", "system", "prune", "-a", "-f")
	pruneCmd.Stdout = writer
	pruneCmd.Stderr = writer
	if err := pruneCmd.Run(); err != nil {
		fmt.Fprintf(writer, "  ⚠️  Prune failed (non-fatal): %v\n", err)
	} else {
		fmt.Fprintln(writer, "  ✅ Podman cache cleared")
	}

	logDiskSpace("AFTER_PRUNE", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Create Kind cluster (AFTER cleanup)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 4: Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	fmt.Fprintln(writer, "📁 Creating namespace...")
	// ... existing namespace creation code ...

	fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	// ... existing CRD installation code ...

	logDiskSpace("KIND_STARTED", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Load images from .tar files into Kind
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 5: Loading images into Kind cluster...")

	for name, tarPath := range tarFiles {
		fmt.Fprintf(writer, "  📦 Loading %s from .tar...\n", name)
		loadCmd := exec.Command("kind", "load", "image-archive", tarPath, "--name", clusterName)
		loadCmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
		loadCmd.Stdout = writer
		loadCmd.Stderr = writer
		if err := loadCmd.Run(); err != nil {
			return fmt.Errorf("failed to load %s into Kind: %w", name, err)
		}
		fmt.Fprintf(writer, "  ✅ %s loaded into Kind\n", name)
	}

	logDiskSpace("IMAGES_LOADED", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Delete .tar files (final cleanup)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n🗑️  PHASE 6: Cleaning up .tar files...")
	for name, tarPath := range tarFiles {
		if err := os.Remove(tarPath); err != nil {
			fmt.Fprintf(writer, "  ⚠️  Failed to remove %s (non-fatal): %v\n", tarPath, err)
		} else {
			fmt.Fprintf(writer, "  ✅ Removed %s\n", tarPath)
		}
	}

	logDiskSpace("END", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7: Deploy infrastructure (PostgreSQL, Redis)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n🐘 Deploying PostgreSQL...")
	// ... existing PostgreSQL deployment ...

	fmt.Fprintln(writer, "🔴 Deploying Redis...")
	// ... existing Redis deployment ...

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 8: Deploy services (Data Storage, HolmesGPT-API, AIAnalysis)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n💾 Deploying Data Storage...")
	// ... existing deployment code ...

	fmt.Fprintln(writer, "🤖 Deploying HolmesGPT-API...")
	// ... existing deployment code ...

	fmt.Fprintln(writer, "🧠 Deploying AIAnalysis controller...")
	// ... existing deployment code ...

	fmt.Fprintln(writer, "⏳ Waiting for all services to be ready...")
	if err := waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ AIAnalysis E2E cluster ready!")
	logDiskSpace("FINAL", writer)
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}
```

---

## 📊 **EXPECTED DISK SPACE REPORT**

### **Example Output**:
```
💾 [START] Disk: 50G total, 30G used, 20G available

📦 PHASE 1: Building images in parallel...
  ✅ datastorage image built
  ✅ holmesgpt-api image built
  ✅ aianalysis image built
💾 [IMAGES_BUILT] Disk: 50G total, 42G used, 8G available

📦 PHASE 2: Exporting images to .tar files...
  ✅ datastorage exported to /tmp/datastorage-e2e.tar
  ✅ holmesgpt-api exported to /tmp/holmesgpt-api-e2e.tar
  ✅ aianalysis exported to /tmp/aianalysis-e2e.tar
💾 [TAR_EXTRACTED] Disk: 50G total, 45G used, 5G available

🗑️  PHASE 3: Aggressive Podman cleanup...
  ✅ Podman cache cleared
💾 [AFTER_PRUNE] Disk: 50G total, 36G used, 14G available  ← 9G freed!

📦 PHASE 4: Creating Kind cluster...
💾 [KIND_STARTED] Disk: 50G total, 37G used, 13G available

📦 PHASE 5: Loading images into Kind cluster...
  ✅ datastorage loaded into Kind
  ✅ holmesgpt-api loaded into Kind
  ✅ aianalysis loaded into Kind
💾 [IMAGES_LOADED] Disk: 50G total, 42G used, 8G available

🗑️  PHASE 6: Cleaning up .tar files...
  ✅ Removed /tmp/datastorage-e2e.tar
  ✅ Removed /tmp/holmesgpt-api-e2e.tar
  ✅ Removed /tmp/aianalysis-e2e.tar
💾 [END] Disk: 50G total, 36G used, 14G available  ← Back to post-prune level

✅ AIAnalysis E2E cluster ready!
💾 [FINAL] Disk: 50G total, 37G used, 13G available
```

---

## 🎯 **DISK SPACE SAVINGS BREAKDOWN**

| Stage | Action | Space Freed | Cumulative Savings |
|-------|--------|-------------|-------------------|
| **IMAGES_BUILT** | 3 images built | -12G | -12G (used) |
| **TAR_EXTRACTED** | 3 .tar files created | -3G | -15G (used) |
| **AFTER_PRUNE** | `podman system prune -a` | +9G | -6G (net) |
| **IMAGES_LOADED** | Images in Kind | -5G | -11G (used) |
| **END** | .tar files deleted | +6G | -5G (final) |

**Net Result**: ~5-6 GB final usage (vs ~15 GB without optimization)

---

## ⚠️ **RISKS & MITIGATIONS**

### **Risk 1: `podman system prune -a` removes ALL images**
- **Impact**: If .tar export fails, we lose the images
- **Mitigation**: Verify .tar files exist before pruning
- **Fallback**: Check .tar file sizes (should be > 100 MB)

### **Risk 2: .tar files consume 3-6 GB during PHASE 3-6**
- **Impact**: Temporary disk pressure during image load
- **Mitigation**: Aggressive prune first (frees 9 GB buffer)
- **Fallback**: Load images one-by-one, delete .tar after each

### **Risk 3: `df -h` parsing might fail on different systems**
- **Impact**: No disk space tracking
- **Mitigation**: Non-fatal error handling
- **Fallback**: Tests still run, just no visibility

---

## 🚀 **IMPLEMENTATION STEPS**

### **Step 1**: Add helper functions to `test/infrastructure/aianalysis.go`
```bash
# Add getDiskSpaceInfo() and logDiskSpace() functions
```

### **Step 2**: Modify `CreateAIAnalysisClusterHybrid()` with new flow
```bash
# Integrate PHASE 2 (tar export), PHASE 3 (prune), PHASE 6 (tar cleanup)
```

### **Step 3**: Test locally
```bash
make test-e2e-aianalysis
# Verify disk space tracking in output
```

### **Step 4**: Validate in GitHub Actions
```bash
# Push changes and monitor E2E run logs
# Confirm disk space reports at each stage
```

---

## 📈 **SUCCESS CRITERIA**

- ✅ AI Analysis E2E tests pass consistently (no disk space failures)
- ✅ Disk space report shows 9+ GB freed after prune
- ✅ Final disk usage < 40% of total capacity
- ✅ No manual intervention required (fully automated)

---

## 🔗 **ALTERNATIVE APPROACHES CONSIDERED**

### **Option A: Serial builds** (REJECTED)
- ❌ Slower (9-10 min vs 4 min parallel)
- ✅ Uses less peak disk space
- **Verdict**: Speed is more important in CI/CD

### **Option B: Prune AFTER each build** (REJECTED)
- ✅ Lower peak disk usage
- ❌ Removes images before export (breaks flow)
- **Verdict**: Need images for .tar export

### **Option C: Stream .tar directly to Kind** (FUTURE)
- ✅ No .tar files on disk
- ❌ Requires custom Kind load logic
- **Verdict**: Too complex for V1.0, consider for V2.0

---

## 📝 **NEXT STEPS**

1. **Implement helper functions** (getDiskSpaceInfo, logDiskSpace)
2. **Modify CreateAIAnalysisClusterHybrid** with new flow
3. **Test locally** to validate disk space tracking
4. **Push and validate** in GitHub Actions
5. **Monitor E2E success rate** (expect 100% after fix)

---

**Document Status**: ✅ Ready for Implementation
**Estimated Implementation Time**: 1-2 hours
**Estimated Testing Time**: 30 minutes (local + CI/CD)
**Risk Level**: LOW (non-breaking change, existing cleanup + tracking)


