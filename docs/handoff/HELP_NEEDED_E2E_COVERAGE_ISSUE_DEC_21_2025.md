# Go E2E Coverage Collection Issue - External Help Needed

**Date**: December 21, 2025
**Status**: 🔴 **BLOCKED - Coverage files not being written**
**Context**: Kubernetes E2E testing with Kind cluster
**Go Version**: 1.22

---

## 🎯 What We're Trying to Achieve

Collect code coverage from a Go binary running in a Kubernetes (Kind) cluster during E2E tests, using Go 1.20+'s coverage profiling feature for compiled binaries.

**Reference**: [Go Blog: Integration Test Coverage](https://go.dev/blog/integration-test-coverage)

---

## 📚 How Go Coverage for Binaries Works

### Normal Flow (Go 1.20+)

1. **Build** with coverage instrumentation:
   ```bash
   GOFLAGS=-cover go build -o myapp ./cmd/myapp/
   ```

2. **Run** with `GOCOVERDIR` environment variable:
   ```bash
   GOCOVERDIR=/path/to/coverdata ./myapp
   ```

3. **Graceful shutdown** (SIGTERM, not SIGKILL):
   - Binary flushes coverage counters to files on exit
   - Creates `covcounters.*` and `covmeta.*` files in `$GOCOVERDIR`

4. **Extract** coverage report:
   ```bash
   go tool covdata textfmt -i=/path/to/coverdata -o coverage.txt
   go tool cover -html=coverage.txt -o coverage.html
   ```

---

## 🐛 The Problem

**Symptom**: Coverage files are **never created** in the mounted volume.

**Expected**:
```
./coverdata/
├── covcounters.6a0f8f2d.2025122118.001234
└── covmeta.6a0f8f2d
```

**Actual**:
```
./coverdata/
(empty directory)
```

**Error**:
```
$ go tool covdata percent -i=./coverdata
warning: no applicable files found in input directories
```

---

## ✅ What's Been Verified

### 1. Binary Build ✅
```dockerfile
# Dockerfile excerpt
ARG GOFLAGS=""
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=${GOFLAGS} go build \
    -ldflags='-extldflags "-static"' \
    -o data-storage \
    ./cmd/datastorage/main.go
```

**Build Command**:
```bash
podman build \
  --build-arg GOFLAGS=-cover \
  -t localhost/kubernaut-datastorage:e2e-test \
  -f docker/data-storage.Dockerfile .
```

**Build Output Confirms**:
```
✅ Building with coverage instrumentation (no symbol stripping)...
✅ Build completed successfully
```

### 2. Kubernetes Deployment ✅

**Environment Variable**:
```yaml
env:
- name: GOCOVERDIR
  value: /coverdata
```

**Verified in Pod**:
```bash
$ kubectl exec <pod> -- env | grep GOCOVERDIR
GOCOVERDIR=/coverdata
```

**Volume Mount**:
```yaml
volumeMounts:
- name: coverdata
  mountPath: /coverdata

volumes:
- name: coverdata
  hostPath:
    path: /coverdata  # Mapped from Kind worker node
    type: DirectoryOrCreate
```

**Verified in Pod**:
```bash
$ kubectl exec <pod> -- ls -la /coverdata
drwxrwxrwx 2 root root 64 Dec 21 18:13 /coverdata
```

### 3. Volume Propagation (Host → Kind → Pod) ✅

**Kind Cluster Config**:
```yaml
nodes:
- role: worker
  extraMounts:
  - hostPath: ${PWD}/coverdata  # Host machine
    containerPath: /coverdata    # Kind worker node
    readOnly: false
```

**Data Flow**:
```
Host: ./coverdata (mode 777)
  ↓ (Kind extraMount)
Kind Node: /coverdata
  ↓ (Pod hostPath volume)
Container: /coverdata (readable/writable by container user)
```

### 4. Graceful Shutdown ✅

**Shutdown Method**:
```bash
# Scale deployment to 0 to trigger graceful SIGTERM
kubectl scale deployment datastorage -n datastorage-e2e --replicas=0

# Wait for coverage flush
sleep 10
```

**Pod Terminates Cleanly**:
- No SIGKILL
- No crash logs
- Exit code 0

### 5. Tests Pass ✅
```
Ran 84 of 84 Specs in 293.582 seconds
SUCCESS! -- 84 Passed | 0 Failed
```

---

## 🔧 What We've Tried

### Attempt 1: Remove Symbol Stripping

**Theory**: `-ldflags='-w -s'` might strip coverage metadata
**Action**: Build conditionally without `-w -s` when `GOFLAGS=-cover`
**Result**: ❌ No change - still no coverage files

**Current Build Logic**:
```dockerfile
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
        # Coverage build: NO symbol stripping
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=${GOFLAGS} go build \
          -ldflags='-extldflags "-static"' \
          -o data-storage \
          ./cmd/datastorage/main.go; \
    else \
        # Production build: WITH symbol stripping
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
          -ldflags='-w -s -extldflags "-static"' \
          -o data-storage \
          ./cmd/datastorage/main.go; \
    fi
```

### Attempt 2: Verify Binary Contains Coverage Instrumentation

**Action**: Extract binary from container and check for coverage symbols
**Command**:
```bash
# Extract binary from built image
podman create --name temp-extract localhost/kubernaut-datastorage:e2e-test
podman cp temp-extract:/usr/local/bin/data-storage /tmp/ds-binary
podman rm temp-extract

# Check for coverage symbols
go tool nm /tmp/ds-binary | grep -i cover
```

**Expected**: Should show symbols like:
```
123456 D runtime/coverage.counters
234567 T runtime/coverage.WriteCounters
345678 D github.com/jordigilh/kubernaut/pkg/datastorage.goCover_...
```

**Status**: 🔴 **NEEDS VERIFICATION** (haven't extracted binary yet)

### Attempt 3: Check Permissions

**Action**: Verify container can write to `/coverdata`
**Command**:
```bash
kubectl exec <pod> -- touch /coverdata/test.txt
kubectl exec <pod> -- ls -la /coverdata/test.txt
```

**Expected**: File created successfully
**Status**: 🔴 **NEEDS VERIFICATION**

---

## 🔍 Comparison to Go Blog Post Example

The [official Go blog post](https://go.dev/blog/integration-test-coverage) demonstrates coverage collection with a **simple scenario**:

| Aspect | Blog Post Example | Our Setup | Complexity Diff |
|--------|-------------------|-----------|-----------------|
| **Build** | `go build -cover -o mdtool.exe .` | Multi-stage Docker build with `GOFLAGS=-cover` | 🔴 **Complex** |
| **Binary** | Single-stage, local build | Copied from builder→runtime stage | 🔴 **Complex** |
| **Linking** | Default (dynamic) | `-extldflags "-static"` | 🔴 **Complex** |
| **Base Image** | N/A (local execution) | `gcr.io/distroless/static-debian12:nonroot` | 🔴 **Complex** |
| **Execution** | Direct: `./mdtool.exe` | Kubernetes pod in Kind cluster | 🔴 **Complex** |
| **User** | Current user | `nonroot` (uid 65532) | 🔴 **Complex** |
| **Storage** | Local directory | HostPath volume through Kind worker | 🔴 **Complex** |

**Blog Post Success**:
```bash
# Build
go build -cover -o mdtool.exe .

# Run with coverage
GOCOVERDIR=covdatafiles ./mdtool.exe +x +a file.md

# Result: 381 coverage files written ✅
ls covdatafiles | wc
    381     381   27401
```

**Our Setup**:
```bash
# Build (inside Docker with static linking)
CGO_ENABLED=0 GOFLAGS=-cover go build \
  -ldflags='-extldflags "-static"' \
  -o data-storage ./cmd/datastorage/

# Run (in Kubernetes pod, distroless container)
GOCOVERDIR=/coverdata /usr/local/bin/data-storage

# Result: 0 coverage files written ❌
ls ./coverdata
(empty)
```

### Key Differences That May Cause Issues

1. **Multi-Stage Docker Build**: Binary is built in one stage, copied to another
   - Could coverage metadata be lost during the copy?
   - The blog post builds and runs in the same environment

2. **Static Linking**: We use `-extldflags "-static"` for distroless compatibility
   - Does the coverage runtime require dynamic linking?
   - The blog post uses default linking (likely dynamic on their platform)

3. **Distroless Base Image**: Minimal runtime environment
   - Missing standard libraries or tools?
   - The blog post likely runs on a full OS with complete Go runtime

4. **Kubernetes Complexity**: Volume mounts through Kind worker node
   - More layers where things can fail: Host → Kind Node → Pod → Container
   - The blog post writes directly to host filesystem

5. **Cross-Stage Binary Copy**: Binary copied with `COPY --from=builder`
   - Does Docker's COPY preserve coverage instrumentation metadata?
   - The blog post builds and executes in place

## 🤔 Potential Root Causes

### Theory 1: Multi-Stage Build Loses Coverage Metadata ⚠️ (NEW)
**Hypothesis**: `COPY --from=builder` in Docker might strip coverage instrumentation
**Evidence**: Coverage works in blog post's single-stage local build, but not our multi-stage Docker build
**Test**: Build and run coverage binary locally (outside Docker) using Test 2 below
**Reference**: Docker COPY may not preserve all ELF metadata needed for coverage

### Theory 2: Static Linking Breaks Coverage ⚠️
**Hypothesis**: `-extldflags "-static"` might interfere with coverage runtime
**Evidence**: Coverage runtime may require dynamic linking for proper initialization
**Test**: Build without `-extldflags "-static"` (may break distroless container)

### Theory 3: Distroless Image Missing Coverage Dependencies 🔍
**Hypothesis**: Distroless lacks libraries or tools needed by coverage runtime
**Evidence**: Distroless is minimal; coverage may need more than we provide
**Test**: Try with full Debian base image instead of distroless
**Reference**: Blog post likely runs on full OS, not minimal container

### Theory 4: Coverage Runtime Not Initializing 🔍
**Hypothesis**: Binary starts but coverage runtime fails silently
**Evidence**: No logs or errors indicating coverage init failure
**Test**: Add debug logging to check if coverage package initializes (Test 4 below)

### Theory 5: Graceful Shutdown Not Triggering Coverage Flush 🔍
**Hypothesis**: Signal handling or shutdown sequence prevents coverage flush
**Evidence**: Binary exits cleanly but coverage flush doesn't execute
**Test**: Add explicit coverage flush before shutdown

### Theory 6: Kind/Kubernetes Environment Issue 🔍
**Hypothesis**: Kubernetes environment variables or cgroup restrictions block coverage
**Evidence**: Coverage works in blog post's local environment but not in Kind
**Test**: Run same binary outside Kind with `GOCOVERDIR` set (Test 2 & 3 below)

---

## 🔬 Diagnostic Tests Needed

### Test 0: Simple Local Build (Baseline - Matches Blog Post) 🎯

This test replicates the **exact scenario from the Go blog post** to verify coverage works in the simple case:

```bash
# Navigate to datastorage directory
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Build LOCALLY with coverage (like blog post)
mkdir -p /tmp/blog-test-cov
GOFLAGS=-cover go build -o /tmp/ds-blog-test ./cmd/datastorage/

# Run with GOCOVERDIR (like blog post)
GOCOVERDIR=/tmp/blog-test-cov /tmp/ds-blog-test --help

# Check if coverage files created
ls -la /tmp/blog-test-cov/

# Expected: covcounters.* and covmeta.* files (like blog post's 381 files)
# If PRESENT: Coverage works in simple case → issue is Docker/K8s specific
# If EMPTY: Coverage broken even in simple case → deeper Go/build issue
```

**This is the MOST IMPORTANT test** - if this fails, it means coverage is broken even in the blog post's simple scenario, indicating a fundamental build issue. If it succeeds, it confirms the problem is specific to Docker multi-stage builds or Kubernetes environment.

### Test 1: Verify Coverage Symbols in Binary
```bash
# Extract binary from container
podman create --name temp localhost/kubernaut-datastorage:e2e-test
podman cp temp:/usr/local/bin/data-storage /tmp/ds-binary
podman rm temp

# Check for coverage symbols
go tool nm /tmp/ds-binary | grep -E "(cover|goCover)" | head -20

# Expected: Should show coverage-related symbols
# If EMPTY: Binary not properly instrumented
```

### Test 2: Local Coverage Test (Outside Kubernetes)
```bash
# Build locally with coverage
mkdir -p /tmp/localcov
GOFLAGS=-cover go build -o /tmp/ds-local ./cmd/datastorage/

# Run with GOCOVERDIR
GOCOVERDIR=/tmp/localcov /tmp/ds-local --help

# Check if coverage files created
ls -la /tmp/localcov/

# Expected: covcounters.* and covmeta.* files
# If EMPTY: Coverage not working even locally
# If PRESENT: Issue is specific to Kubernetes/container environment
```

### Test 3: Container Coverage Test (Podman, No Kubernetes)
```bash
# Run container locally with coverage
mkdir -p /tmp/containercov
chmod 777 /tmp/containercov

podman run --rm \
  -e GOCOVERDIR=/coverdata \
  -v /tmp/containercov:/coverdata:Z \
  localhost/kubernaut-datastorage:e2e-test \
  --help

# Check if coverage files created
ls -la /tmp/containercov/

# Expected: covcounters.* and covmeta.* files
# If EMPTY: Issue with container environment (permissions, volume mount)
# If PRESENT: Issue is specific to Kubernetes/Kind
```

### Test 4: Check Coverage Init at Runtime
Add to `cmd/datastorage/main.go`:
```go
import (
    "fmt"
    "os"
    _ "runtime/coverage" // Force import
)

func main() {
    // Debug: Check if coverage is enabled
    if coverDir := os.Getenv("GOCOVERDIR"); coverDir != "" {
        fmt.Fprintf(os.Stderr, "COVERAGE ENABLED: GOCOVERDIR=%s\n", coverDir)

        // Test write
        if f, err := os.Create(filepath.Join(coverDir, "coverage-test.txt")); err != nil {
            fmt.Fprintf(os.Stderr, "COVERAGE ERROR: Cannot write to GOCOVERDIR: %v\n", err)
        } else {
            f.WriteString("Coverage directory is writable\n")
            f.Close()
            fmt.Fprintf(os.Stderr, "COVERAGE OK: Test file written successfully\n")
        }
    }

    // ... rest of application
}
```

---

## 📊 Environment Details

### Build Environment
- **OS**: macOS (darwin 24.6.0)
- **Podman**: Version 4.x
- **Go**: 1.22
- **Build Context**: Dockerfile with multi-stage build

### Runtime Environment
- **Kubernetes**: Kind (Kubernetes in Docker)
- **Container Base**: `gcr.io/distroless/static-debian12:nonroot` (static, minimal)
- **Container User**: `nonroot` (uid 65532)
- **Binary Type**: CGO_ENABLED=0, statically linked

### Dockerfile Stages
```dockerfile
# Stage 1: Builder
FROM golang:1.22-alpine AS builder
ARG GOFLAGS=""
# ... build with GOFLAGS ...

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/data-storage /usr/local/bin/data-storage
USER nonroot
ENTRYPOINT ["/usr/local/bin/data-storage"]
```

---

## ❓ Questions for SME

### Primary Questions (Based on Blog Post Comparison)

1. **Multi-Stage Docker Builds**: Does `COPY --from=builder` in Docker preserve the coverage instrumentation metadata embedded by `go build -cover`?
   - The blog post builds and runs in the same environment
   - We build in golang:1.22-alpine, copy to distroless/static-debian12
   - Could the copy operation strip ELF sections needed for coverage?

2. **Static Linking vs Coverage**: Does `-ldflags='-extldflags "-static"'` conflict with Go's coverage runtime?
   - The blog post likely uses default (dynamic) linking
   - Coverage runtime might need dynamic linking for proper initialization
   - Can coverage work with fully static binaries?

3. **Distroless Compatibility**: Are there known limitations with coverage in distroless containers?
   - The blog post runs on a full OS
   - Distroless lacks many standard libraries and tools
   - Does coverage runtime depend on libraries not present in distroless?

### Secondary Questions

4. **Coverage Instrumentation Verification**: How can we verify a binary is properly instrumented with coverage?
   - Is `go tool nm <binary> | grep cover` sufficient?
   - What specific symbols should we expect to see?

5. **Signal Handling**: Does Kubernetes' pod termination (SIGTERM → 30s → SIGKILL) give enough time for coverage flush?
   - Is there a way to explicitly flush coverage data before exit?
   - Can we add a manual flush call in our shutdown sequence?

6. **Volume Permissions**: Directory is `drwxrwxrwx`, container runs as `nonroot` (uid 65532)
   - Could there still be permission issues preventing write?
   - Would running as root in Kind (test environment) help diagnose?

---

## 🎯 Side-by-Side: Blog Post (Works) vs Our Setup (Fails)

### ✅ Blog Post Example (mdtool) - WORKS

```bash
# Simple build
$ go build -cover -o mdtool.exe .

# Simple run
$ GOCOVERDIR=covdatafiles ./mdtool.exe +x +a testdata/README.md
(processes file successfully)

# Result
$ ls covdatafiles | wc
    381     381   27401  ← ✅ Coverage files written!

$ go tool covdata percent -i=covdatafiles
gitlab.com/golang-commonmark/mdtool coverage: 48.1% of statements
```

**Why it works**:
- Single environment (local machine)
- Default linking (dynamic)
- Full OS (not container)
- Direct filesystem access
- No multi-stage build

### ❌ Our Setup (datastorage) - FAILS

```bash
# Complex build (Docker multi-stage)
$ podman build --build-arg GOFLAGS=-cover \
    -f docker/data-storage.Dockerfile \
    -t localhost/kubernaut-datastorage:e2e-test .
✅ Build succeeds with coverage

# Complex run (Kubernetes in Kind)
$ kubectl apply -f datastorage-deployment.yaml  # With GOCOVERDIR=/coverdata
$ kubectl wait --for=condition=ready pod/datastorage-xxx
✅ Pod runs successfully, processes 84 E2E tests

# Graceful shutdown
$ kubectl scale deployment datastorage --replicas=0
✅ Pod terminates cleanly (SIGTERM, not SIGKILL)

# Result
$ ls ./coverdata
(empty directory)  ← ❌ NO coverage files!

$ go tool covdata percent -i=./coverdata
warning: no applicable files found in input directories
```

**Why it might fail**:
- Multi-stage Docker build (binary copied between stages)
- Static linking (`-extldflags "-static"`)
- Distroless container (minimal runtime)
- Volume mount through Kind worker node (3-layer path)
- Kubernetes environment (cgroups, security contexts)

### 🔑 Key Insight

The **exact same Go coverage feature** works in the blog post's simple scenario but fails in our complex containerized Kubernetes environment. This strongly suggests the issue is related to one or more of:
1. Docker multi-stage build losing coverage metadata
2. Static linking incompatibility with coverage runtime
3. Distroless image missing required coverage dependencies
4. Kubernetes volume/environment complexity

---

## 📞 Contact for Follow-up

- This issue is blocking E2E coverage measurement for our Kubernetes services
- We have a working test suite (84/84 passing) but cannot measure coverage
- We're following the **official [Go blog post methodology](https://go.dev/blog/integration-test-coverage)** which works in simple cases
- Our complex Docker/Kubernetes environment breaks the coverage collection
- Any guidance on debugging this further would be greatly appreciated

**Status**: Ready for external review and diagnosis

---

## 📚 References

1. **[Go Blog: Code coverage for Go integration tests](https://go.dev/blog/integration-test-coverage)** (March 2023)
   - Official Go team guide to integration test coverage
   - Simple example (mdtool) works perfectly
   - Our setup follows this methodology but fails in containerized environment

2. **[Go Coverage Documentation](https://go.dev/doc/go1.20#coverage)** (Go 1.20 release notes)

3. **[Go Coverage Tutorial](https://go.dev/testing/coverage/)** (Official Go coverage guide)

---

**Document Created**: 2025-12-21 19:00 EST
**Last Updated**: 2025-12-21 19:15 EST (Added blog post comparison)
**Audience**: External Go/Kubernetes SMEs
**Next Step**: Share with Go coverage experts for root cause analysis

