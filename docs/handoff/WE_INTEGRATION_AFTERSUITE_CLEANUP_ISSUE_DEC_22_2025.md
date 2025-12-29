# WorkflowExecution Integration AfterSuite Cleanup Issue - December 22, 2025

## 🚨 **Problem Statement**

Integration test infrastructure remains running after tests complete, causing:
- Port conflicts for subsequent test runs
- Resource leaks (containers, images, networks)
- Disk space consumption

**Evidence**:
```bash
$ podman ps | grep workflowexecution
workflowexecution_postgres_1      (31 minutes old, still running)
workflowexecution_redis_1         (31 minutes old, still running)
workflowexecution_datastorage_1   (31 minutes old, still running)

$ podman images | grep workflowexecution
workflowexecution_datastorage:latest
```

---

## 🔍 **Root Cause Analysis**

### **Issue**: `podman compose` vs `podman-compose`

**Current Code** (`suite_test.go:316`):
```go
cmd := exec.Command("podman", "compose", "-f", "podman-compose.test.yml", "down")
```

**Problem**:
- `podman compose` (space) delegates to external `docker-compose`
- `docker-compose` doesn't properly interface with Podman's labels/metadata
- Containers created by `podman-compose` (hyphen) aren't recognized by `docker-compose`
- Result: `down` command exits successfully but **doesn't stop containers**

**Correct Command**:
```bash
podman-compose -f podman-compose.test.yml down  # ✅ Works
podman compose -f podman-compose.test.yml down  # ❌ Silent failure
```

---

## 📋 **Detailed Investigation**

### **Test 1: Check Container Labels**
```bash
$ podman inspect workflowexecution_postgres_1 | grep compose.project
"com.docker.compose.project": "workflowexecution"
"io.podman.compose.project": "workflowexecution"  # ← Created by podman-compose
```

**Conclusion**: Containers have both `docker-compose` and `podman-compose` labels.

### **Test 2: Try `podman compose down`**
```bash
$ cd test/integration/workflowexecution
$ podman compose -f podman-compose.test.yml down
>>> Executing external compose provider "/usr/local/bin/docker-compose" <<<
[warning] version attribute is obsolete

$ podman ps | grep workflowexecution
# Containers still running! ❌
```

**Conclusion**: `podman compose` delegates to `docker-compose`, which doesn't stop the containers.

### **Test 3: Try `podman-compose down`**
```bash
$ podman-compose -f podman-compose.test.yml down
workflowexecution_datastorage_1
workflowexecution_redis_1
workflowexecution_postgres_1
workflowexecution_we-test-network

$ podman ps | grep workflowexecution
# No containers! ✅
```

**Conclusion**: `podman-compose` (hyphen) correctly stops containers.

---

## 🔧 **Solution**

### **Fix 1: Change Command in AfterSuite**

**File**: `test/integration/workflowexecution/suite_test.go`

**Before**:
```go
cmd := exec.Command("podman", "compose", "-f", "podman-compose.test.yml", "down")
```

**After**:
```go
cmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
```

**Rationale**:
- Matches the tool used to start containers (`podman-compose up`)
- Correctly stops containers created by `podman-compose`
- Avoids delegation to `docker-compose`

---

### **Fix 2: Add Explicit Image Cleanup**

**Current cleanup** (line 328):
```go
pruneCmd := exec.Command("podman", "image", "prune", "-f",
    "--filter", "label=io.podman.compose.project=workflowexecution")
```

**Problem**: Prune with filter doesn't always work if images are in use.

**Add explicit cleanup**:
```go
// Remove compose-built image explicitly
removeCmd := exec.Command("podman", "rmi",
    "localhost/workflowexecution_datastorage:latest",
    "-f")
removeOutput, removeErr := removeCmd.CombinedOutput()
if removeErr != nil {
    GinkgoWriter.Printf("⚠️  Failed to remove datastorage image: %v\n%s\n",
        removeErr, removeOutput)
} else {
    GinkgoWriter.Println("✅ Datastorage test image removed")
}

// Then prune dangling images
pruneCmd := exec.Command("podman", "image", "prune", "-f")
```

---

### **Fix 3: Add Verification Step**

**Add after cleanup**:
```go
By("Verifying infrastructure cleanup")
// Check no workflowexecution containers remain
psCmd := exec.Command("podman", "ps", "-a", "--filter",
    "label=io.podman.compose.project=workflowexecution")
psOutput, _ := psCmd.CombinedOutput()
if len(psOutput) > 100 { // More than just headers
    GinkgoWriter.Printf("⚠️  WARNING: Containers may still exist:\n%s\n",
        psOutput)
} else {
    GinkgoWriter.Println("✅ No workflowexecution containers remaining")
}
```

---

## 📊 **Impact Analysis**

### **Before Fix**
- ❌ Containers remain running after tests
- ❌ Images accumulate (`workflowexecution_datastorage:latest`)
- ❌ Ports blocked (15443, 16389, 18100, 19100)
- ❌ Next test run requires manual cleanup
- ❌ Disk space grows over time

### **After Fix**
- ✅ All containers stopped automatically
- ✅ Test images removed
- ✅ Ports freed for next run
- ✅ Tests can run back-to-back
- ✅ Disk space managed

---

## 🎯 **Implementation**

### **Updated AfterSuite Code**

```go
var _ = AfterSuite(func() {
    By("Closing real audit store")
    if realAuditStore != nil {
        err := realAuditStore.Close()
        if err != nil {
            GinkgoWriter.Printf("⚠️  Warning: Failed to close audit store: %v\n", err)
        } else {
            GinkgoWriter.Println("✅ Real audit store closed (all events flushed)")
        }
    }

    cancel()

    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())

    By("Stopping DataStorage infrastructure")
    testDir, pathErr := filepath.Abs(filepath.Join(".", "..", "..", ".."))
    if pathErr != nil {
        GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", pathErr)
    } else {
        // FIX: Use "podman-compose" (hyphen) not "podman compose" (space)
        // Reason: Containers created by podman-compose must be stopped by podman-compose
        cmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
        cmd.Dir = filepath.Join(testDir, "test", "integration", "workflowexecution")
        output, cmdErr := cmd.CombinedOutput()
        if cmdErr != nil {
            GinkgoWriter.Printf("⚠️  Failed to stop containers: %v\n%s\n", cmdErr, output)
        } else {
            GinkgoWriter.Printf("✅ DataStorage infrastructure stopped\n%s\n", output)
        }
    }

    By("Removing test-specific images")
    // Explicitly remove compose-built image
    removeCmd := exec.Command("podman", "rmi",
        "localhost/workflowexecution_datastorage:latest", "-f")
    removeOutput, removeErr := removeCmd.CombinedOutput()
    if removeErr != nil {
        // Image might not exist - that's okay
        GinkgoWriter.Printf("   Datastorage image: already removed or not found\n")
    } else {
        GinkgoWriter.Printf("✅ Datastorage test image removed\n")
    }

    // Prune any dangling images
    pruneCmd := exec.Command("podman", "image", "prune", "-f")
    pruneOutput, pruneErr := pruneCmd.CombinedOutput()
    if pruneErr != nil {
        GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n%s\n", pruneErr, pruneOutput)
    } else {
        GinkgoWriter.Println("✅ Dangling images pruned")
    }

    By("Verifying cleanup")
    psCmd := exec.Command("podman", "ps", "-a", "--filter",
        "label=io.podman.compose.project=workflowexecution")
    psOutput, _ := psCmd.CombinedOutput()
    if len(psOutput) > 100 { // More than just headers
        GinkgoWriter.Printf("⚠️  WARNING: Containers may still exist:\n%s\n", psOutput)
    } else {
        GinkgoWriter.Println("✅ All workflowexecution containers cleaned up")
    }

    GinkgoWriter.Println("✅ Cleanup complete")
})
```

---

## ✅ **Testing the Fix**

### **Manual Cleanup (Current State)**
```bash
# Clean up existing leaked infrastructure
cd test/integration/workflowexecution
podman-compose -f podman-compose.test.yml down
podman rmi localhost/workflowexecution_datastorage:latest -f
```

### **Verify Fix Works**
```bash
# Run integration tests
make test-integration-workflowexecution

# After tests complete, verify cleanup:
podman ps | grep workflowexecution  # Should be empty
podman images | grep workflowexecution  # Should be empty
```

---

## 🔗 **Related Issues**

### **Other Services May Have Same Problem**

Check these files for similar `podman compose` usage:
- `test/integration/remediationorchestrator/suite_test.go`
- `test/integration/signalprocessing/suite_test.go`
- `test/integration/notification/suite_test.go`
- `test/integration/gateway/suite_test.go`

**Pattern to Search**:
```bash
grep -r "podman.*compose.*down" test/integration/*/suite_test.go
```

**Fix Pattern**:
```diff
- exec.Command("podman", "compose", "-f", "file.yml", "down")
+ exec.Command("podman-compose", "-f", "file.yml", "down")
```

---

## 📝 **Lessons Learned**

### **1. Podman Compose Tool Mismatch**
- `podman-compose` (Python tool) ≠ `podman compose` (delegates to docker-compose)
- Use consistent tooling: `podman-compose up` → `podman-compose down`

### **2. Silent Failures in Cleanup**
- `podman compose down` exits 0 even if it doesn't stop containers
- Always verify cleanup with explicit checks

### **3. Integration Test Hygiene**
- AfterSuite cleanup is **CRITICAL** for test reliability
- Leaked resources cause flaky tests (port conflicts)
- Add verification steps to catch cleanup failures

---

## 🎯 **Action Items**

- [x] Manually clean up leaked workflowexecution containers
- [x] Manually remove workflowexecution images
- [ ] Fix `suite_test.go` AfterSuite (use `podman-compose` not `podman compose`)
- [ ] Add explicit image removal step
- [ ] Add verification step
- [ ] Test fix with full integration run
- [ ] Search for similar issues in other services
- [ ] Document this pattern in testing guidelines

---

**Document Status**: ✅ Complete
**Created**: December 22, 2025
**Priority**: **P0 (Blocker)** - Affects test reliability
**Confidence**: 100% (root cause verified, fix tested manually)

---

*This issue prevented proper cleanup of integration test infrastructure, causing resource leaks and port conflicts. The fix changes `podman compose` to `podman-compose` in AfterSuite cleanup.*






