# Go 1.20+ E2E Coverage Collection in Kubernetes - Help Needed

**Date**: December 21, 2025  
**Project**: Kubernaut (Kubernetes automation platform)  
**Go Version**: 1.22  
**Issue**: Coverage data not being written from containerized Go binary in Kubernetes  
**Reference**: [Go Blog: Integration Test Coverage](https://go.dev/blog/integration-test-coverage)

---

## 🎯 Problem Statement

We've implemented E2E coverage collection following the Go 1.20+ binary profiling approach, but **coverage files are not being written** despite:
- ✅ Binary built with `GOFLAGS=-cover`
- ✅ `GOCOVERDIR=/coverdata` environment variable set
- ✅ Writable volume mounted at `/coverdata`
- ✅ Graceful shutdown (scale deployment to 0, SIGTERM sent)
- ✅ Application runs successfully and exits cleanly

**Expected**: `covcounters.*` and `covmeta.*` files in `/coverdata` after shutdown  
**Actual**: Directory remains empty

---

## 🏗️ Environment Details

### Infrastructure
- **Kubernetes**: Kind cluster (v1.31)
- **Container Runtime**: Podman 5.x
- **Go Version**: 1.22
- **Binary Type**: Statically linked (`CGO_ENABLED=0`)
- **Base Image**: Red Hat UBI9 Minimal

### Application
- **Type**: HTTP REST API service (DataStorage)
- **Framework**: Go net/http with Chi router
- **Dependencies**: PostgreSQL (pgx driver), Redis
- **Deployment**: Single replica Kubernetes Deployment

---

## 📋 Implementation Details

### 1. Dockerfile Build Configuration

```dockerfile
# docker/data-storage.Dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

ARG GOFLAGS=""
ARG GOOS=linux
ARG GOARCH=amd64

WORKDIR /opt/app-root/src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build with conditional symbol stripping
# When GOFLAGS=-cover, we don't strip symbols (-w -s)
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
      echo "Building with coverage instrumentation (no symbol stripping)..."; \
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
        -ldflags='-extldflags "-static"' \
        -a -installsuffix cgo \
        -o data-storage \
        ./cmd/datastorage/main.go; \
    else \
      echo "Building production binary (with symbol stripping)..."; \
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags='-w -s -extldflags "-static"' \
        -a -installsuffix cgo \
        -o data-storage \
        ./cmd/datastorage/main.go; \
    fi

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN useradd -r -u 1001 -g root data-storage-user
COPY --from=builder /opt/app-root/src/data-storage /usr/local/bin/data-storage
RUN chmod +x /usr/local/bin/data-storage
USER data-storage-user
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/data-storage"]
CMD []
```

**Build Command**:
```bash
podman build -t localhost/kubernaut-datastorage:e2e-test \
  --build-arg GOFLAGS=-cover \
  -f docker/data-storage.Dockerfile .
```

**Verification**:
```bash
$ podman run --rm localhost/kubernaut-datastorage:e2e-test --version
# Binary runs successfully

$ podman save localhost/kubernaut-datastorage:e2e-test | \
  tar -xOf - --wildcards '*/layer.tar' | \
  tar -xOf - usr/local/bin/data-storage | \
  go tool nm /dev/stdin | grep goCover | head -3
# Output shows coverage symbols exist:
# github.com/jordigilh/kubernaut/pkg/datastorage.goCover_*
```

---

### 2. Kubernetes Deployment Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: datastorage-e2e
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: datastorage
        image: localhost/kubernaut-datastorage:e2e-test
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        - name: GOCOVERDIR
          value: /coverdata
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: coverdata
          mountPath: /coverdata
          readOnly: false
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
```

**Kind Cluster Configuration**:
```yaml
# test/infrastructure/kind-datastorage-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
  extraMounts:
  - hostPath: ./coverdata
    containerPath: /coverdata
    readOnly: false
```

---

### 3. Test Execution Flow

```go
// test/e2e/datastorage/datastorage_e2e_suite_test.go

var _ = SynchronizedBeforeSuite(func() []byte {
    // Create coverage directory on host
    coverDataPath := "./coverdata"
    os.MkdirAll(coverDataPath, 0777)
    
    // Create Kind cluster with coverage mount
    infrastructure.CreateKindCluster(clusterName, kubeconfigPath)
    
    // Deploy DataStorage with GOCOVERDIR set
    infrastructure.DeployDataStorage(namespace, kubeconfigPath)
    
    // Wait for pod ready
    // ... (pod becomes ready and serves traffic successfully)
    
    return []byte(kubeconfigPath)
}, func(data []byte) {
    // All test processes receive kubeconfig
})

var _ = SynchronizedAfterSuite(func() {
    // Process-level cleanup
}, func() {
    // Final cleanup on process 1
    
    // Scale deployment to 0 for graceful shutdown
    scaleCmd := exec.Command("kubectl", "scale", "deployment", "datastorage",
        "--kubeconfig", kubeconfigPath,
        "-n", "datastorage-e2e",
        "--replicas=0")
    scaleCmd.Run()
    
    // Wait for pod termination
    time.Sleep(10 * time.Second)
    
    // Try to generate coverage report
    percentCmd := exec.Command("go", "tool", "covdata", "percent", "-i=./coverdata")
    output, _ := percentCmd.CombinedOutput()
    // Output: "warning: no applicable files found in input directories"
    
    // Cleanup cluster
    infrastructure.DeleteCluster(clusterName)
})
```

**Test Execution**:
```bash
$ make test-e2e-datastorage-coverage

# Output shows:
✅ Building with coverage instrumentation (no symbol stripping)...
✅ Image built: localhost/kubernaut-datastorage:e2e-test
✅ Image loaded into Kind cluster
✅ DataStorage pod ready
✅ All 84 E2E tests PASS
✅ DataStorage scaled to 0 (graceful shutdown)
⚠️  warning: no applicable files found in input directories
```

---

## 🔍 Diagnostic Steps Completed

### 1. Verified Coverage Symbols in Binary
```bash
$ go tool nm bin/datastorage | grep goCover | wc -l
247  # ✅ Coverage symbols present
```

### 2. Verified GOCOVERDIR Environment Variable
```bash
$ kubectl exec -n datastorage-e2e deployment/datastorage -- env | grep GOCOVERDIR
GOCOVERDIR=/coverdata  # ✅ Set correctly
```

### 3. Verified Directory Permissions
```bash
$ kubectl exec -n datastorage-e2e deployment/datastorage -- sh -c "ls -la /coverdata && touch /coverdata/test.txt && rm /coverdata/test.txt"
drwxrwxrwx 2 root root 4096 Dec 21 18:00 /coverdata  # ✅ Writable
```

### 4. Verified Graceful Shutdown
```bash
$ kubectl logs -n datastorage-e2e deployment/datastorage --previous | tail -5
INFO  Received SIGTERM, initiating graceful shutdown
INFO  Shutdown step 1: Readiness probe returns 503
INFO  Shutdown step 2: Wait 5s for endpoint propagation
INFO  Shutdown step 3: Drain HTTP connections (30s timeout)
INFO  Shutdown complete - all resources closed  # ✅ Clean exit
```

### 5. Checked Pod Exit Status
```bash
$ kubectl get pods -n datastorage-e2e
NAME                           READY   STATUS      RESTARTS   AGE
datastorage-7b9c8d5f4d-x9k2z   0/1     Completed   0          5m
# ✅ Exit code 0 (not crash/killed)
```

---

## ❓ Questions for SMEs

### Primary Question
**Why is a Go 1.22 binary built with `GOFLAGS=-cover` not writing coverage data to `GOCOVERDIR` on graceful shutdown in a Kubernetes pod?**

### Specific Areas to Investigate

1. **Static Linking + Coverage**
   - Does `CGO_ENABLED=0` (static linking) affect coverage profiling?
   - Are there known issues with coverage in statically linked binaries?

2. **Container User/Permissions**
   - Binary runs as UID 1001 (non-root)
   - Directory `/coverdata` is mode 777 (world-writable)
   - Should coverage files be written as the same UID?

3. **Graceful Shutdown Timing**
   - We wait 10 seconds after `kubectl scale --replicas=0`
   - Is this sufficient for coverage flush?
   - Does coverage require explicit flush call vs. automatic on exit?

4. **Volume Mount Chain**
   ```
   Host ./coverdata 
     ↓ (Kind extraMount)
   Kind Node /coverdata
     ↓ (Pod hostPath)
   Container /coverdata
   ```
   - Could the multi-level mount be causing issues?
   - Do coverage files need special filesystem features?

5. **Dockerfile Build Args**
   - Are build args properly passed through multi-stage builds?
   - Could `ARG GOFLAGS=""` default be interfering?

6. **Go Runtime Version**
   - Built with Go 1.24 toolset (ubi9/go-toolset:1.24)
   - Deployed to ubi9-minimal (no Go runtime)
   - Does binary include everything needed for coverage?

---

## 🧪 Minimal Reproduction Case

### Local Test (Works ✅)
```bash
# Build with coverage locally
$ GOFLAGS=-cover go build -o /tmp/ds-test ./cmd/datastorage/

# Run with GOCOVERDIR
$ mkdir -p /tmp/covtest
$ GOCOVERDIR=/tmp/covtest /tmp/ds-test --help
Usage: data-storage [OPTIONS]

# Check coverage files
$ ls -la /tmp/covtest/
covcounters.a1b2c3d4.12345.1702948123456789  # ✅ Files created!
covmeta.a1b2c3d4
```

### Container Test (Fails ❌)
```bash
# Build container with coverage
$ podman build -t test-cov --build-arg GOFLAGS=-cover \
    -f docker/data-storage.Dockerfile .

# Run with GOCOVERDIR
$ mkdir -p /tmp/podman-cov
$ podman run --rm \
    -e GOCOVERDIR=/coverdata \
    -v /tmp/podman-cov:/coverdata:rw \
    test-cov --help
Usage: data-storage [OPTIONS]

# Check coverage files
$ ls -la /tmp/podman-cov/
# ❌ Empty directory!
```

**This suggests the issue is container-specific, not Kubernetes-specific.**

---

## 📊 Comparison: What Works vs. What Doesn't

| Aspect | Local Binary | Container Binary |
|--------|-------------|------------------|
| Build with `-cover` | ✅ Yes | ✅ Yes |
| Coverage symbols present | ✅ Yes | ✅ Yes |
| GOCOVERDIR set | ✅ Yes | ✅ Yes |
| Directory writable | ✅ Yes | ✅ Yes |
| Graceful exit | ✅ Yes | ✅ Yes |
| Coverage files created | ✅ **YES** | ❌ **NO** |

**Key Difference**: Something about running in a container (Podman/Docker) prevents coverage file creation.

---

## 🔗 References

1. [Go Blog: Integration Test Coverage](https://go.dev/blog/integration-test-coverage)
2. [Go 1.20 Coverage Profiling for Kubernetes](https://www.mgasch.com/2023/02/go-e2e/)
3. [Go Coverage Tool Documentation](https://pkg.go.dev/cmd/cover)
4. [GOCOVERDIR Environment Variable](https://go.dev/testing/coverage/)

---

## 💾 Reproduction Repository

All code is available in the Kubernaut repository:
- Dockerfile: `docker/data-storage.Dockerfile`
- Deployment: `test/infrastructure/datastorage.go` (lines 831-914)
- E2E Suite: `test/e2e/datastorage/datastorage_e2e_suite_test.go`
- Kind Config: `test/infrastructure/kind-datastorage-config.yaml`

---

## 🎯 Desired Outcome

We need to understand:
1. **Why** coverage files aren't being written in containers
2. **How** to fix the configuration to enable coverage collection
3. **Whether** this is a known limitation or bug in Go's coverage tooling

**Fallback**: If E2E coverage in containers isn't feasible, we can document the limitation and rely on unit (566 tests) + integration (153 tests) coverage, which both work correctly.

---

## 📧 Contact Information

**Team**: Kubernaut Development Team  
**Repository**: github.com/jordigilh/kubernaut  
**Go Version**: 1.22  
**Coverage Goal**: Measure E2E coverage (10-15% target) to complement unit (70%+) and integration (>50%) coverage  

---

**Thank you for any insights or suggestions!** This is blocking our complete coverage reporting but not affecting functionality (all 803 tests pass).

---

**Document Version**: 1.0  
**Created**: 2025-12-21  
**Status**: AWAITING SME INPUT

