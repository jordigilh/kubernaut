# DataStorage DD-TEST-007 - Ready for E2E Coverage Testing

**Date**: December 21, 2025
**Service**: DataStorage
**Status**: ✅ Implementation Complete, Ready for Testing
**Container Runtime**: Podman

---

## ✅ What Was Implemented

Successfully applied **DD-TEST-007: E2E Coverage Capture Standard** from SignalProcessing team to DataStorage service. All components verified and ready for E2E testing.

---

## 🔧 Changes Applied

### 1. E2E Test Suite (`test/e2e/datastorage/datastorage_e2e_suite_test.go`)
- ✅ Added coverage mode detection (`E2E_COVERAGE=true`)
- ✅ Added `coverageMode` and `coverDir` variables
- ✅ Updated `SynchronizedBeforeSuite` to create coverage directory
- ✅ Updated `SynchronizedAfterSuite` to:
  - Scale deployment to 0 (triggers graceful shutdown)
  - Extract coverage using `podman cp datastorage-e2e-worker:/tmp/coverage/. ./coverdata`
  - Generate coverage reports (text + HTML)

### 2. Infrastructure (`test/infrastructure/datastorage.go`)
- ✅ Modified `deployDataStorageServiceInNamespace` to:
  - Conditionally add `GOCOVERDIR=/tmp/coverage` env var (only if `E2E_COVERAGE=true`)
  - Removed hostPath volume mounts (DD-TEST-007 uses `podman cp` extraction)

### 3. Dockerfile (`docker/data-storage.Dockerfile`)
- ✅ Already had correct implementation:
  - Accepts `GOFLAGS` build arg
  - Conditionally removes `-w -s` symbol stripping when `GOFLAGS=-cover`

### 4. Makefile
- ✅ Already had `test-e2e-datastorage-coverage` target
- ✅ Sets `E2E_COVERAGE=true` before running tests

---

## 🧪 Verification Performed

### 1. Build Test ✅
```bash
E2E_COVERAGE=true podman build --build-arg GOFLAGS=-cover \
  -t localhost/kubernaut-datastorage:e2e-test-verify \
  -f docker/data-storage.Dockerfile .
```
**Result**: Build successful

### 2. Coverage Instrumentation Test ✅
```bash
podman run --rm localhost/kubernaut-datastorage:e2e-test-verify \
  /bin/sh -c "..."
```
**Result**: Binary detects `GOCOVERDIR` and warns when not set
```
warning: GOCOVERDIR not set, no coverage data emitted
```
This confirms coverage instrumentation is working!

### 3. Lint Check ✅
**Result**: No linter errors in modified files

---

## 🎯 Key DD-TEST-007 Insights Applied

### The Problem
Original approach tried to use hostPath volume mounts to share coverage files between pod and host. This failed with Podman Kind provider because coverage files were trapped inside the Kind node container.

### The Solution
DD-TEST-007 approach:
1. **Write coverage INSIDE Kind node**: Coverage files go to `/tmp/coverage` in pod
2. **Explicit extraction**: Use `podman cp` to extract from Kind node container to host
3. **No volume mounts needed**: Simpler, more reliable

### Why It Works
```
Pod writes to /tmp/coverage
     ↓
Files exist in Kind node container filesystem
     ↓
podman cp datastorage-e2e-worker:/tmp/coverage/. ./coverdata
     ↓
Files now on host for go tool covdata to process
```

---

## 🚀 Ready to Test

### Run E2E Tests with Coverage
```bash
make test-e2e-datastorage-coverage
```

### Expected Output
1. **Build Phase**: Image built with `GOFLAGS=-cover`
2. **Deploy Phase**: `GOCOVERDIR=/tmp/coverage` set in pod
3. **Test Phase**: E2E tests run, coverage accumulates
4. **Shutdown Phase**: Deployment scaled to 0, coverage written
5. **Extract Phase**: `podman cp` extracts coverage files
6. **Report Phase**: Coverage reports generated

### Success Criteria
- [ ] `./coverdata/` directory contains coverage files (`.covcounters`, `.covmeta`)
- [ ] `e2e-coverage.txt` generated
- [ ] `e2e-coverage.html` generated
- [ ] Coverage percentage > 0%

---

## 📁 Files Modified

```
test/e2e/datastorage/datastorage_e2e_suite_test.go
test/infrastructure/datastorage.go
docs/handoff/DS_DD_TEST_007_IMPLEMENTATION_DEC_21_2025.md (created)
docs/handoff/DS_DD_TEST_007_READY_FOR_TESTING.md (this file)
```

---

## 📊 Expected Coverage Collection Flow

```
1. make test-e2e-datastorage-coverage
   └─→ E2E_COVERAGE=true

2. Build DataStorage image
   └─→ GOFLAGS=-cover (no symbol stripping)

3. Deploy to Kind cluster
   └─→ GOCOVERDIR=/tmp/coverage set

4. Run E2E tests
   └─→ Coverage data accumulates in memory

5. Scale deployment to 0
   └─→ Graceful shutdown writes coverage to /tmp/coverage

6. Extract from Kind node
   └─→ podman cp datastorage-e2e-worker:/tmp/coverage/. ./coverdata

7. Generate reports
   └─→ e2e-coverage.txt, e2e-coverage.html

8. Display summary
   └─→ Coverage percentage in console
```

---

## 🎓 What We Learned from DD-TEST-007

1. **Container Filesystems Are Isolated**: Files written in Kind pods stay in Kind node container, must be explicitly extracted

2. **Podman cp is the Key**: No volume mounts needed, just extract coverage files after shutdown

3. **Symbol Stripping Breaks Coverage**: `-w -s` linker flags must be removed when building with `-cover`

4. **Graceful Shutdown Required**: Coverage is written on process exit, must scale to 0 (not delete pod)

5. **Go Runtime Handles Coverage**: When `GOCOVERDIR` set, Go runtime automatically writes coverage on exit

---

## 🔗 References

- **DD-TEST-007**: `docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md`
- **Implementation Guide**: `docs/handoff/DS_DD_TEST_007_IMPLEMENTATION_DEC_21_2025.md`
- **Go Coverage Blog**: https://go.dev/blog/integration-test-coverage

---

## 🙏 Thank You to SP Team

This implementation would not have been possible without the detailed documentation and working implementation from the SignalProcessing team. DD-TEST-007 provided the critical insight about using `podman cp` to extract coverage files from the Kind node.

---

## 🎯 Next Action for User

**Run the E2E coverage test:**
```bash
make test-e2e-datastorage-coverage
```

This will verify that coverage collection works end-to-end and produce coverage reports for the DataStorage service.

---

**Status**: ✅ Ready for Testing
**Confidence**: 95% (verified build, instrumentation, and code changes; awaiting full E2E test)

---

**End of Status Report**









