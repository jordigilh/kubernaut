# DataStorage E2E Coverage Collection - Troubleshooting

**Date**: December 21, 2025
**Status**: ⚠️ **IMPLEMENTATION COMPLETE BUT COVERAGE NOT COLLECTING**
**Issue**: Coverage files not being written to `/coverdata` directory

---

## ✅ What's Been Implemented

### 1. Kind Cluster Configuration ✅
- `test/infrastructure/kind-datastorage-config.yaml` - Coverage volume mount added

### 2. Dockerfile Updates ✅
- `docker/data-storage.Dockerfile` - `ARG GOFLAGS` and usage in build command added

### 3. Infrastructure Code ✅
- `test/infrastructure/datastorage.go` - `E2E_COVERAGE` environment variable check added
- Docker build passes `--build-arg GOFLAGS=-cover` when `E2E_COVERAGE=true`

### 4. Deployment Manifest ✅
- `GOCOVERDIR=/coverdata` environment variable added
- Volume mount for `/coverdata` added
- HostPath volume pointing to `/coverdata` added

### 5. E2E Test Suite ✅
- `./coverdata` directory created in BeforeSuite
- Graceful shutdown (scale to 0) in AfterSuite
- Coverage report generation attempted

### 6. Makefile Target ✅
- `test-e2e-datastorage-coverage` target created
- Sets `E2E_COVERAGE=true` before running tests

---

## ⚠️ Current Problem

**Symptom**: `./coverdata/` directory remains empty after E2E tests complete

**Verification**:
```bash
$ ls -la ./coverdata/
total 0
drwxrwxrwx@   2 jgil  staff    64 Dec 21 18:13 .
drwxr-xr-x@ 221 jgil  staff  7072 Dec 21 18:16 ..
```

**Expected**: Should contain `covcounters.*` and `covmeta.*` files

---

## 🔍 Diagnostic Steps Completed

### 1. Verified Docker Build with Coverage ✅
```
✅ Setting E2E_COVERAGE=true to enable GOFLAGS=-cover in Dockerfile
✅ Building with coverage instrumentation (GOFLAGS=-cover)
✅ Build completed successfully
```

### 2. Verified Tests Pass ✅
```
Ran 84 of 84 Specs in 293.582 seconds
SUCCESS! -- 84 Passed | 0 Failed
```

### 3. Verified Graceful Shutdown ✅
```
✅ DataStorage scaled to 0
(waited 10 seconds for coverage flush)
```

### 4. Coverage Report Generation Attempted ⚠️
```
⚠️  warning: no applicable files found in input directories
```

---

## 🐛 Possible Root Causes

### Theory 1: Binary Stripped of Coverage Metadata
**Issue**: Dockerfile uses `-ldflags='-w -s'` which strips debug info and symbol table

**Current Build Command**:
```dockerfile
RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o data-storage \
	./cmd/datastorage/main.go
```

**Hypothesis**: The `-w -s` flags might be removing coverage instrumentation

**Test**: Remove `-w -s` when building with coverage

### Theory 2: Permission Issues
**Issue**: Container runs as user `1001` (data-storage-user) but `/coverdata` might not be writable

**Current Dockerfile**:
```dockerfile
USER data-storage-user  # uid 1001
```

**Hypothesis**: User 1001 cannot write to `/coverdata` volume

**Test**: Check pod logs for permission denied errors, or make `/coverdata` world-writable

### Theory 3: Volume Mount Not Working
**Issue**: HostPath volume might not be correctly mapping through Kind worker → host

**Data Flow**:
```
Host ./coverdata
  ↓ (Kind extraMount)
Kind Node /coverdata
  ↓ (Pod hostPath volume)
Container /coverdata
```

**Hypothesis**: One of these mappings is failing

**Test**: Exec into pod and check if `/coverdata` exists and is writable

### Theory 4: Coverage Not Enabled in Binary
**Issue**: Despite `GOFLAGS=-cover`, the binary might not have coverage support

**Test**: Extract binary from container and check for coverage symbols:
```bash
# Get binary from running pod
kubectl cp <pod-name>:/usr/local/bin/data-storage /tmp/ds-binary

# Check for coverage symbols
go tool nm /tmp/ds-binary | grep -i cover
```

### Theory 5: GOCOVERDIR Not Recognized
**Issue**: Environment variable `GOCOVERDIR` not being recognized by Go runtime

**Test**: Add debug logging to show environment variables in pod

---

## 🔧 Recommended Next Steps

### Step 1: Verify Binary Has Coverage Support
```bash
# After tests start, get the pod name
kubectl get pods -n datastorage-e2e

# Copy binary from pod
kubectl cp datastorage-e2e/<pod-name>:/usr/local/bin/data-storage /tmp/ds-e2e-binary

# Check for coverage symbols
go tool nm /tmp/ds-e2e-binary | grep goCover

# Expected: Should show coverage-related symbols like:
# github.com/jordigilh/kubernaut/pkg/datastorage.goCover_*
```

### Step 2: Check Pod Environment and Permissions
```bash
# Get pod name
POD=$(kubectl get pods -n datastorage-e2e -l app=datastorage -o name)

# Check GOCOVERDIR is set
kubectl exec -n datastorage-e2e $POD -- env | grep GOCOVERDIR

# Check /coverdata exists and is writable
kubectl exec -n datastorage-e2e $POD -- ls -la /coverdata
kubectl exec -n datastorage-e2e $POD -- touch /coverdata/test.txt
kubectl exec -n datastorage-e2e $POD -- rm /coverdata/test.txt
```

### Step 3: Try Without Symbol Stripping
Modify Dockerfile to NOT use `-w -s` when building with coverage:

```dockerfile
# In docker/data-storage.Dockerfile, change build command to:
RUN if [ -n "${GOFLAGS}" ]; then \
      # Coverage build: Don't strip symbols
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
        -ldflags='-extldflags "-static"' \
        -a -installsuffix cgo \
        -o data-storage \
        ./cmd/datastorage/main.go; \
    else \
      # Production build: Strip symbols for smaller size
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags='-w -s -extldflags "-static"' \
        -a -installsuffix cgo \
        -o data-storage \
        ./cmd/datastorage/main.go; \
    fi
```

### Step 4: Add Coverage Debug Logging
Add to E2E suite BeforeSuite:
```go
// After pod is ready, check coverage setup
time.Sleep(30 * time.Second) // Wait for pod to be fully running
checkCmd := exec.Command("kubectl", "exec",
    "-n", sharedNamespace,
    "deployment/datastorage",
    "--", "sh", "-c",
    "echo GOCOVERDIR=$GOCOVERDIR && ls -la /coverdata && id")
output, _ := checkCmd.CombinedOutput()
logger.Info("Coverage setup check", "output", string(output))
```

### Step 5: Manual Coverage Test
Create a minimal test case:
```bash
# Build locally with coverage
GOFLAGS=-cover go build -o /tmp/ds-coverage ./cmd/datastorage/

# Run with GOCOVERDIR
mkdir -p /tmp/testcover
GOCOVERDIR=/tmp/testcover /tmp/ds-coverage --help

# Check if coverage files created
ls -la /tmp/testcover/
```

---

## 📊 Reference Information

### Working Example from Go Blog
From [Go Blog: Integration Test Coverage](https://go.dev/blog/integration-test-coverage):

1. **Build** with `-cover`: `go build -cover -o myapp ./cmd/myapp`
2. **Run** with `GOCOVERDIR`: `GOCOVERDIR=/tmp/coverage ./myapp`
3. **Trigger graceful shutdown**: Send SIGTERM or scale to 0
4. **Extract** coverage: `go tool covdata textfmt -i=/tmp/coverage -o coverage.txt`

### Key Requirements
- ✅ Binary built with `-cover` flag
- ✅ `GOCOVERDIR` environment variable set
- ✅ Directory must be writable by process
- ✅ Graceful shutdown (not SIGKILL)
- ⚠️ **NOT verified**: Binary retains coverage metadata (might be stripped by `-w -s`)

---

## 📝 Files Modified

1. ✅ `test/infrastructure/kind-datastorage-config.yaml`
2. ✅ `docker/data-storage.Dockerfile`
3. ✅ `test/infrastructure/datastorage.go`
4. ✅ `test/e2e/datastorage/datastorage_e2e_suite_test.go`
5. ✅ `Makefile`

---

## ⏭️ Next Actions

1. **Immediate**: Try Step 1 (verify binary has coverage support)
2. **If binary lacks coverage**: Implement Step 3 (remove `-w -s` for coverage builds)
3. **If binary has coverage**: Try Step 2 (check permissions)
4. **If still failing**: Try Step 5 (manual local test)

---

**Status**: Implementation is complete but coverage collection is blocked by unresolved issue with coverage data not being written. Need to debug why binary with coverage instrumentation isn't writing coverage files on shutdown.

**Confidence**: 60% - Infrastructure is correct, likely a build configuration issue (symbol stripping)

**Risk**: LOW - No impact on normal E2E tests, coverage collection is additive feature

---

**Created**: 2025-12-21 18:40 EST
**Author**: AI Assistant (Cursor)

