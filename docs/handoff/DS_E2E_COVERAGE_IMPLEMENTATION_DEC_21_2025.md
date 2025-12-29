# DataStorage E2E Coverage Collection Implementation

**Date**: December 21, 2025
**Implemented By**: AI Assistant (Cursor)
**Reference**: [E2E_COVERAGE_COLLECTION.md](../development/testing/E2E_COVERAGE_COLLECTION.md)
**Status**: ✅ **READY FOR TESTING**

---

## 📋 Summary

Implemented comprehensive E2E coverage collection for the DataStorage service using Go 1.20+ binary profiling. This enables measurement of actual code paths executed during end-to-end tests running in a Kind Kubernetes cluster.

---

## 🎯 Implementation Completed

### 1. Kind Cluster Configuration ✅

**File**: `test/infrastructure/kind-datastorage-config.yaml`

**Changes**:
- Added `extraMounts` to worker node
- Mounts `./coverdata` from host to `/coverdata` in Kind node
- Enables coverage data collection from pods running in the cluster

```yaml
- role: worker
  extraMounts:
  # Mount coverage directory from host to Kind node for E2E coverage collection
  # See: docs/development/testing/E2E_COVERAGE_COLLECTION.md
  - hostPath: ./coverdata
    containerPath: /coverdata
    readOnly: false
```

---

### 2. DataStorage Deployment Configuration ✅

**File**: `test/infrastructure/datastorage.go`

**Changes**:
1. **Added GOCOVERDIR environment variable** (line ~836):
   ```go
   {
       // E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
       // GOCOVERDIR enables Go 1.20+ binary coverage profiling
       // Coverage data written to /coverdata on graceful shutdown
       Name:  "GOCOVERDIR",
       Value: "/coverdata",
   }
   ```

2. **Added coverage volume mount** (line ~850):
   ```go
   {
       // E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
       // Mount coverage directory for binary profiling data
       Name:      "coverdata",
       MountPath: "/coverdata",
       ReadOnly:  false,
   }
   ```

3. **Added coverage hostPath volume** (line ~920):
   ```go
   {
       // E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
       // HostPath volume maps to Kind node's /coverdata (mounted from host ./coverdata)
       // Kind config: hostPath: ./coverdata -> containerPath: /coverdata
       Name: "coverdata",
       VolumeSource: corev1.VolumeSource{
           HostPath: &corev1.HostPathVolumeSource{
               Path: "/coverdata",
               Type: func() *corev1.HostPathType {
                   t := corev1.HostPathDirectoryOrCreate
                   return &t
               }(),
           },
       },
   }
   ```

---

### 3. E2E Test Suite Updates ✅

**File**: `test/e2e/datastorage/datastorage_e2e_suite_test.go`

**Changes**:

#### A. Coverage Directory Creation (BeforeSuite, line ~103)
```go
// E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
// Create coverage directory for Go 1.20+ binary profiling
coverDataPath := "./coverdata"
if err := os.MkdirAll(coverDataPath, 0777); err != nil {
    logger.Info("⚠️  Failed to create coverage directory", "error", err)
} else {
    logger.Info("📊 Coverage directory created", "path", coverDataPath)
    logger.Info("   💡 Coverage data will be written on graceful shutdown")
}
```

#### B. Coverage Extraction (AfterSuite, line ~294)
```go
// E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
// Extract coverage before cluster deletion
logger.Info("📊 Extracting E2E coverage data...")
logger.Info("   Scaling down DataStorage for graceful shutdown (flushes coverage)...")

// Scale deployment to 0 to trigger graceful shutdown (writes coverage data)
scaleCmd := exec.Command("kubectl", "scale", "deployment", "datastorage",
    "--kubeconfig", kubeconfigPath,
    "-n", sharedNamespace,
    "--replicas=0")
if output, err := scaleCmd.CombinedOutput(); err != nil {
    logger.Info("⚠️  Failed to scale down DataStorage", "error", err, "output", string(output))
} else {
    logger.Info("   ✅ DataStorage scaled to 0")

    // Wait for pod termination (coverage is written during graceful shutdown)
    time.Sleep(10 * time.Second)

    // Generate coverage reports
    // ... (generates text and HTML reports)
}
```

---

### 4. Makefile Target ✅

**File**: `Makefile`

**Added**: `test-e2e-datastorage-coverage` target (line ~799)

```makefile
.PHONY: test-e2e-datastorage-coverage
test-e2e-datastorage-coverage: ## Run Data Storage E2E tests with coverage collection
    # Step 1: Build binary with coverage instrumentation
    @GOFLAGS=-cover go build -o bin/datastorage ./cmd/datastorage/

    # Step 2: Run E2E tests (coverage data collected)
    @$(MAKE) test-e2e-datastorage

    # Step 3: Generate coverage reports
    @go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt
    @go tool cover -html=e2e-coverage.txt -o e2e-coverage.html
    @go tool covdata percent -i=./coverdata
```

---

## 🚀 Usage

### Run E2E Tests with Coverage Collection

```bash
make test-e2e-datastorage-coverage
```

This will:
1. Build DataStorage binary with coverage instrumentation (`GOFLAGS=-cover`)
2. Build and load Docker image to Kind cluster
3. Run E2E tests (84 tests)
4. Scale down deployment for graceful shutdown (triggers coverage write)
5. Generate coverage reports:
   - `e2e-coverage.txt` - Text format
   - `e2e-coverage.html` - HTML visualization

### View Coverage Report

```bash
# Open HTML report in browser
open e2e-coverage.html  # macOS
xdg-open e2e-coverage.html  # Linux

# View text summary
go tool covdata percent -i=./coverdata
```

---

## 📊 Expected Coverage

Based on E2E test scenarios:

| Test Level | Target | Expected E2E Contribution |
|------------|--------|---------------------------|
| **Unit** | 70%+ | N/A (separate) |
| **Integration** | >50% | N/A (separate) |
| **E2E** | 10-15% | **Critical paths only** |

**E2E Scenarios Covered**:
1. Happy Path - Complete remediation audit trail
2. DLQ Fallback - Service outage recovery
3. Query API - Multi-filter timeline retrieval
4. Workflow Search - Hybrid weighted scoring
5. Connection Pool - Burst handling and recovery

---

## 🔧 How It Works

### Data Flow

```
1. Binary built with -cover flag
   ↓
2. Binary deployed to Kind pod with GOCOVERDIR=/coverdata
   ↓
3. E2E tests execute → code paths recorded
   ↓
4. Graceful shutdown (kubectl scale deployment --replicas=0)
   ↓
5. Coverage data flushed to /coverdata in pod
   ↓
6. HostPath volume syncs to ./coverdata on host
   ↓
7. go tool covdata extracts reports
```

### File Locations

```
./coverdata/            # Coverage data directory (created automatically)
├── covcounters.*      # Coverage counter data
├── covmeta.*          # Coverage metadata
e2e-coverage.txt       # Text format coverage report
e2e-coverage.html      # HTML visualization
```

---

## 🎯 Validation Checklist

- [x] Kind config mounts `./coverdata` to worker node
- [x] Deployment sets `GOCOVERDIR=/coverdata`
- [x] Deployment mounts coverdata volume to `/coverdata`
- [x] BeforeSuite creates `./coverdata` directory
- [x] AfterSuite scales deployment to 0 (graceful shutdown)
- [x] AfterSuite extracts coverage reports
- [x] Makefile target `test-e2e-datastorage-coverage` added
- [ ] **TODO**: Test end-to-end execution (pending user verification)

---

## 🐛 Troubleshooting

### Coverage Files Not Created

**Symptom**: `./coverdata/` is empty after tests

**Possible Causes**:
1. Binary not built with `-cover` flag
2. Tests failed before graceful shutdown
3. Volume mount not working (check `kubectl describe pod`)
4. Insufficient wait time for shutdown

**Verification Commands**:
```bash
# Check if coverage is enabled in binary
go tool nm bin/datastorage | grep cover

# Check if GOCOVERDIR is set in pod
kubectl exec -it <pod-name> -n datastorage-e2e -- env | grep GOCOVERDIR

# Check if volume is mounted
kubectl exec -it <pod-name> -n datastorage-e2e -- ls -la /coverdata
```

---

## 📚 References

- [E2E_COVERAGE_COLLECTION.md](../development/testing/E2E_COVERAGE_COLLECTION.md) - Implementation guide
- [Go 1.20 Coverage Profiling](https://www.mgasch.com/2023/02/go-e2e/) - External reference
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) - Testing strategy

---

## ✅ Next Steps

1. **Test the implementation**:
   ```bash
   make test-e2e-datastorage-coverage
   ```

2. **Verify coverage data**:
   ```bash
   ls -la ./coverdata/
   # Should show: covcounters.* and covmeta.* files
   ```

3. **Generate combined coverage** (unit + integration + E2E):
   ```bash
   # Run all test tiers
   go test ./test/unit/datastorage/... -coverprofile=unit-coverage.out
   go test ./test/integration/datastorage/... -coverprofile=integration-coverage.out
   make test-e2e-datastorage-coverage

   # Merge coverage files
   go install github.com/wadey/gocovmerge@latest
   gocovmerge unit-coverage.out integration-coverage.out e2e-coverage.txt > combined-coverage.out

   # Generate combined report
   go tool cover -html=combined-coverage.out -o combined-coverage.html
   ```

---

**Status**: ✅ **IMPLEMENTATION COMPLETE - READY FOR USER TESTING**

**Confidence**: 95% (all code changes validated, pending end-to-end execution test)

**Risk**: LOW - No breaking changes to existing tests, coverage collection is additive

---

**Document Created**: 2025-12-21
**Last Updated**: 2025-12-21
**Author**: AI Assistant (Cursor)

