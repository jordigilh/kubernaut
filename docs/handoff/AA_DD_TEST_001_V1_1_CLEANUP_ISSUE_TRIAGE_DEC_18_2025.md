# AIAnalysis DD-TEST-001 v1.1 Cleanup Issue - Root Cause Analysis

**Date**: December 18, 2025
**Issue**: Integration test infrastructure containers not stopping after test completion
**Status**: ✅ **RESOLVED**

---

## 🔍 **Root Cause Analysis**

### **Symptom**
After running AIAnalysis integration tests, the following containers remained running:
- `aianalysis_postgres_test` (PostgreSQL on port 15434)
- `aianalysis_redis_test` (Redis on port 16380)
- `aianalysis_datastorage_test` (Data Storage on port 18091)
- `aianalysis_holmesgpt_test` (HolmesGPT-API on port 18120)

### **Root Cause**
**Issue 1**: Incorrect cleanup implementation in integration test suite

The initial DD-TEST-001 v1.1 implementation attempted to use `podman-compose` directly with a non-existent file:
```go
// ❌ INCORRECT - File doesn't exist
cmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
```

**Actual file name**: `podman-compose.yml` (not `podman-compose.test.yml`)

**Issue 2**: Redundant cleanup logic

The integration test suite was attempting to implement its own cleanup logic instead of using the existing infrastructure functions:
- `infrastructure.StopAIAnalysisIntegrationInfrastructure()` - Already handles stopping containers correctly
- The infrastructure functions use the correct compose file path and project name

### **Error Message**
```
STEP: Stopping AIAnalysis integration infrastructure (podman-compose)
⚠️  Failed to stop containers: exit status 1
CRITICAL:podman_compose:missing files: ['podman-compose.test.yml']
```

---

## ✅ **Solution**

### **1. Use Existing Infrastructure Functions**

**Before** (incorrect manual cleanup):
```go
By("Stopping AIAnalysis integration infrastructure (podman-compose)")
testDir, pathErr := filepath.Abs(filepath.Join(".", "..", "..", ".."))
if pathErr != nil {
	GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", pathErr)
} else {
	cmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
	cmd.Dir = filepath.Join(testDir, "test", "integration", "aianalysis")
	// ... error handling
}
```

**After** (correct - uses infrastructure function):
```go
By("Stopping AIAnalysis integration infrastructure")
err := infrastructure.StopAIAnalysisIntegrationInfrastructure(GinkgoWriter)
if err != nil {
	GinkgoWriter.Printf("⚠️  Warning: Error stopping infrastructure: %v\n", err)
}
```

### **2. Correct Label Filter for Image Pruning**

The compose project name is `aianalysis-integration`, not `aianalysis`:

```go
By("Cleaning up infrastructure images to prevent disk space issues")
// Per DD-TEST-001 v1.1: Use label-based filtering for AIAnalysis integration compose project
pruneCmd := exec.Command("podman", "image", "prune", "-f",
	"--filter", "label=io.podman.compose.project=aianalysis-integration")
```

### **3. Remove BeforeSuite Cleanup**

The BeforeSuite cleanup was also incorrect (looking for non-existent file). Since the infrastructure functions handle cleanup properly, the BeforeSuite cleanup is unnecessary:

**Removed**:
```go
By("Cleaning up stale containers from previous runs")
cleanupCmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
// ... this was looking for the wrong file
```

---

## 📋 **Infrastructure Function Details**

### **StartAIAnalysisIntegrationInfrastructure()**
Located in: `test/infrastructure/aianalysis.go:1508-1570`

**Correct behavior**:
- Uses compose file: `test/integration/aianalysis/podman-compose.yml`
- Uses project name: `aianalysis-integration`
- Command: `podman-compose -f {composeFile} -p aianalysis-integration up -d --build`

### **StopAIAnalysisIntegrationInfrastructure()**
Located in: `test/infrastructure/aianalysis.go:1572-1596`

**Correct behavior**:
- Uses compose file: `test/integration/aianalysis/podman-compose.yml`
- Uses project name: `aianalysis-integration`
- Command: `podman-compose -f {composeFile} -p aianalysis-integration down -v`

---

## 🔧 **Manual Cleanup (For Reference)**

If containers are stuck running, they can be manually stopped:

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman-compose -f test/integration/aianalysis/podman-compose.yml -p aianalysis-integration down -v
```

Or individually:
```bash
podman stop aianalysis_postgres_test aianalysis_redis_test aianalysis_datastorage_test aianalysis_holmesgpt_test
podman rm aianalysis_postgres_test aianalysis_redis_test aianalysis_datastorage_test aianalysis_holmesgpt_test
```

---

## ✅ **Verification**

### **Test Infrastructure Cleanup**
```bash
# After running integration tests
podman ps | grep aianalysis
# Expected: No output (all containers stopped)

# Check for orphaned images
podman images | grep "io.podman.compose.project=aianalysis-integration"
# Expected: Minimal or no output (images pruned)
```

### **Integration Test Execution**
```bash
make test-integration-aianalysis
# Expected output should show:
# - ✅ Infrastructure stopped
# - ✅ Infrastructure images pruned
# - ✅ Cleanup complete
```

---

## 📊 **Key Learnings**

### **1. Don't Duplicate Infrastructure Logic**
- ✅ Use existing `infrastructure.Start/Stop` functions
- ❌ Don't create parallel cleanup implementations in test suites

### **2. Verify File Paths**
- ✅ Check actual file names before referencing them
- ❌ Don't assume file naming conventions

### **3. Test Cleanup Code**
- ✅ Verify cleanup commands work manually first
- ❌ Don't assume cleanup works without testing

### **4. Use Correct Project Names**
- ✅ Match compose project name to what infrastructure functions use
- ❌ Don't use generic service names for label filtering

---

## 📚 **Related Files**

**Modified**:
- `test/integration/aianalysis/suite_test.go` - Fixed AfterSuite cleanup, removed BeforeSuite cleanup
- `docs/handoff/AA_DD_TEST_001_V1_1_CLEANUP_IMPLEMENTATION_DEC_18_2025.md` - Updated with correct implementation

**Reference**:
- `test/infrastructure/aianalysis.go` - Infrastructure functions (lines 1508-1596)
- `test/integration/aianalysis/podman-compose.yml` - Actual compose file
- `test/integration/workflowexecution/suite_test.go` - Reference implementation (similar pattern)

---

## ✅ **Resolution Status**

**Date Resolved**: December 18, 2025
**Status**: ✅ **COMPLETE**

**Changes Made**:
1. ✅ AfterSuite now calls `infrastructure.StopAIAnalysisIntegrationInfrastructure()`
2. ✅ Image pruning uses correct label: `io.podman.compose.project=aianalysis-integration`
3. ✅ Removed incorrect BeforeSuite cleanup
4. ✅ Manual cleanup executed to clear stuck containers

**Verification**:
- ✅ All 4 containers manually stopped and removed
- ✅ Integration tests pass (53/53)
- ✅ Cleanup code uses correct infrastructure functions
- ✅ No containers remain after test completion

---

**Impact**: This fix ensures DD-TEST-001 v1.1 compliance is fully implemented for AIAnalysis integration tests, with proper infrastructure cleanup preventing disk space issues and port conflicts.

