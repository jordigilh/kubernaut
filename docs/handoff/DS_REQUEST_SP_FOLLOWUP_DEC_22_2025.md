# 🙏 DataStorage → SP Team: Follow-up Request

**Date**: December 22, 2025
**From**: DataStorage Team
**To**: Signal Processing Team
**Status**: 🟡 **Path Fix Applied** | 🔴 **Still No Coverage Files**

---

## ✅ SP Team's Fix Applied Successfully

**Root Cause Identified by SP Team**:
> "SignalProcessing uses `/coverdata` which matches the Kind extraMounts. DataStorage uses `/tmp/coverage` which doesn't exist on the Kind node."

**Actions Taken**:
- ✅ Changed `GOCOVERDIR` from `/tmp/coverage` → `/coverdata`
- ✅ Changed hostPath volume from `/tmp/coverage` → `/coverdata`
- ✅ Changed volume mount from `/tmp/coverage` → `/coverdata`
- ✅ Changed `podman cp` extraction from `/tmp/coverage` → `/coverdata`

**Verification**:
```bash
# Infrastructure logs confirm:
🔍 DD-TEST-007: E2E_COVERAGE=true (enabled=true)
✅ Adding GOCOVERDIR=/coverdata to DataStorage deployment
```

---

## ❌ Problem Still Persists

**Symptom**: Coverage files still not being written

**Evidence**:
```bash
2025-12-22T08:58:26.188   ✅ Coverage files extracted from Kind node
2025-12-22T08:58:26.380   ✅ E2E Coverage Report:
2025-12-22T08:58:26.380   warning: no applicable files found in input directories

# Check directories:
$ ls -laR ./coverdata/
./coverdata/:
total 0
drwxr-xr-x  2 jgil  staff  64 Dec 22 08:55 .
drwxr-xr-x 19 jgil  staff 608 Dec 22 08:58 ..
# ^^ EMPTY!
```

**What This Means**:
- `podman cp` succeeds (no error)
- Directory exists in Kind node (would fail otherwise)
- But directory is empty (no `.covcounters` or `.covmeta` files)

---

## 🔍 What We Need Help With

### Question 1: Is DataStorage Binary Actually Instrumented?

**Our build command** (from logs):
```
Building with coverage instrumentation (no symbol stripping)...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOFLAGS=-cover go build \
  -ldflags='-extldflags "-static"' \
  -a -installsuffix cgo \
  -o data-storage \
  ./cmd/datastorage/main.go
```

**Questions for SP Team**:
1. Does SignalProcessing use the exact same build flags?
2. Do you have `-a -installsuffix cgo` in your build?
3. Does static linking (`-extldflags "-static"`) interfere with coverage?

### Question 2: Runtime Environment

**Our deployment env vars**:
```yaml
env:
- name: CONFIG_PATH
  value: /etc/datastorage/config.yaml
- name: GOCOVERDIR
  value: /coverdata
```

**Questions for SP Team**:
1. Do you have any other environment variables set for coverage?
2. Does the order of env vars matter?
3. Do you need to set `GOCOVERDIR` before the service starts?

### Question 3: Directory Permissions

**Our setup**:
- HostPath volume: `/coverdata` (DirectoryOrCreate)
- Volume mount: `/coverdata` (readOnly: false)
- Container user: 1001 (non-root)

**Questions for SP Team**:
1. Does SignalProcessing run as root (uid 0) in E2E tests?
2. Do you set `securityContext.runAsUser: 0` for coverage?
3. Does `/coverdata` need specific permissions (777, 755, etc.)?

### Question 4: Graceful Shutdown Timing

**Our shutdown process**:
```go
// Step 1: Scale to 0
kubectl scale deployment datastorage --replicas=0

// Step 2: Wait 10 seconds
time.Sleep(10 * time.Second)

// Step 3: Extract coverage
podman cp datastorage-e2e-worker:/coverdata/. ./coverdata
```

**Questions for SP Team**:
1. How long do you wait after scaling to 0?
2. Do you poll for pod termination instead of sleep?
3. Do you verify coverage files exist before extraction?

### Question 5: Can We See Your Working Setup?

**Would be super helpful**:
1. Your exact `go build` command for coverage
2. Your deployment manifest snippet (env vars + volumes)
3. Your extraction code from suite_test.go
4. Any gotchas or special configuration you needed

---

## 📊 Our Complete Setup (for Reference)

### 1. Dockerfile Build
```dockerfile
ARG GOFLAGS=""
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
      echo "Building with coverage instrumentation (no symbol stripping)..."; \
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
        -ldflags='-extldflags "-static"' \
        -a -installsuffix cgo \
        -o data-storage \
        ./cmd/datastorage/main.go; \
    fi
```

### 2. Kind Cluster Config
```yaml
- role: worker
  extraMounts:
  - hostPath: ./coverdata
    containerPath: /coverdata
    readOnly: false
```

### 3. Deployment Manifest
```yaml
env:
- name: GOCOVERDIR
  value: /coverdata
volumeMounts:
- name: coverage
  mountPath: /coverdata
  readOnly: false
volumes:
- name: coverage
  hostPath:
    path: /coverdata
    type: DirectoryOrCreate
```

### 4. Extraction Code
```go
// Scale to 0
scaleCmd := exec.Command("kubectl", "scale", "deployment", "datastorage", "--replicas=0")
scaleCmd.Run()

// Wait 10s
time.Sleep(10 * time.Second)

// Extract
cpCmd := exec.Command("podman", "cp",
    "datastorage-e2e-worker:/coverdata/.",
    "./coverdata")
cpCmd.Run()
```

---

## 🎯 What Would Really Help

**Option A**: Quick comparison of your setup vs ours
- Point out any differences in build flags, env vars, volumes, etc.

**Option B**: Pair debug session
- Exec into running DataStorage pod during test
- Check if `/coverdata` exists and is writable
- Verify `GOCOVERDIR` is set in running process

**Option C**: Share your working code
- Even just snippets of the key parts would be hugely helpful
- We can then compare line-by-line to find the difference

---

## 🙏 Thank You!

Your help identifying the `/coverdata` vs `/tmp/coverage` mismatch was a game-changer! We're so close now - the infrastructure is all correct, but the DataStorage binary just isn't writing the files. Any insights you can share would be amazing!

---

**Next Steps**:
1. Await SP team guidance on binary instrumentation / runtime environment
2. Debug why DataStorage binary doesn't write coverage despite `GOCOVERDIR` being set
3. Compare with SignalProcessing's working setup to find the missing piece

---

**Status**: 🟡 Infrastructure Correct, Service Not Writing Files
**Blocker**: DataStorage service doesn't write coverage despite `GOCOVERDIR=/coverdata`
**Request**: SP team help comparing our setup with their working implementation

---

**End of Request**









