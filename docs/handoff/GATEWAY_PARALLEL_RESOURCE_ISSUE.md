# Gateway Parallel Optimization - Resource Exhaustion Issue

**Date**: December 13, 2025
**Status**: 🔴 **BLOCKED** - Parallel builds exhaust system resources
**Root Cause**: Running 3 parallel image builds (Gateway + DataStorage + PostgreSQL/Redis) exceeds available memory/CPU

---

## 🚨 Issue

### Parallel Build Failure

**Error**:
```
github.com/jackc/pgx/v5/pgtype: /usr/lib/golang/pkg/tool/linux_arm64/compile: signal: killed
net/http: /usr/lib/golang/pkg/tool/linux_arm64/compile: signal: killed
Error: server probably quit: unexpected EOF
  ❌ DataStorage image failed: DS image build failed: podman build failed: exit status 125
  ❌ Gateway image failed: Gateway image build/load failed: podman build failed: exit status 125
```

**Root Cause**: System OOM (Out of Memory) killer terminating Go compiler processes

**Phase 2 Tasks** (running in parallel):
1. Gateway image build (~2 GB RAM)
2. DataStorage image build (~2 GB RAM)
3. PostgreSQL + Redis deployment (~500 MB RAM)

**Total**: ~4.5 GB RAM + CPU for 3 simultaneous Go compilations

---

## 📊 Resource Analysis

### Why This Happens

**Go Compilation is Resource-Intensive**:
- Each `go build` can use 1-3 GB RAM (depending on dependency graph)
- UBI9 `go-toolset:1.24` includes full Go toolchain
- ARM64 compilation may be more memory-intensive than AMD64
- Parallel builds multiply resource usage (2-3x)

**System Constraints**:
- Podman machine has limited resources
- Running 2 simultaneous Go builds + Kubernetes deployment
- System OOM killer terminates processes when memory exhausted

---

## 🛠️ Solutions

### Option A: Build Images Sequentially in Phase 2 (RECOMMENDED)
**What**: Keep parallel deployment, but build images sequentially
**Why**: Reduces peak memory usage while maintaining some parallelization benefit
**How**:

```go
// Phase 2a: Build Gateway image (sequential)
if err := buildAndLoadGatewayImage(clusterName, writer); err != nil {
    return fmt.Errorf("Gateway image build/load failed: %w", err)
}

// Phase 2b: Build DataStorage image (sequential)
if err := buildDataStorageImage(writer); err != nil {
    return fmt.Errorf("DS image build failed: %w", err)
}
if err := loadDataStorageImage(clusterName, writer); err != nil {
    return fmt.Errorf("DS image load failed: %w", err)
}

// Phase 2c: Deploy PostgreSQL + Redis (parallel with nothing)
results := make(chan result, 1)
go func() {
    var err error
    if pgErr := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); pgErr != nil {
        err = pgErr
    } else if redisErr := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); redisErr != nil {
        err = redisErr
    }
    results <- result{name: "PostgreSQL+Redis", err: err}
}()
```

**Expected Time**: ~6.5 min (vs ~7.6 min sequential baseline)
**Improvement**: 14% faster (not 27%, but safer)

---

### Option B: Pre-build Images, Only Load in Parallel
**What**: Build images once, cache them, only load in parallel during tests
**Why**: Separates resource-intensive builds from test runs
**How**:

```bash
# Pre-build script (run once)
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Build Gateway image
podman build -t localhost/kubernaut-gateway:e2e-test -f Dockerfile.gateway .

# Build DataStorage image
podman build -t localhost/kubernaut-datastorage:e2e-test -f docker/data-storage.Dockerfile .

# Now E2E tests only load pre-built images (fast, low memory)
```

**Expected Time**: ~4.5 min (builds excluded, only loads)
**Improvement**: 40% faster (best option if images rarely change)
**Trade-off**: Requires manual pre-build step

---

### Option C: Increase Podman Machine Resources
**What**: Allocate more RAM/CPU to Podman machine
**Why**: Allow parallel builds to succeed
**How**:

```bash
# Stop Podman machine
podman machine stop

# Recreate with more resources
podman machine rm podman-machine-default
podman machine init --cpus 4 --memory 8192 --disk-size 100

# Start machine
podman machine start
```

**Expected Time**: ~5.5 min (original parallel optimization)
**Improvement**: 27% faster
**Trade-off**: Requires system resources, may impact other work

---

### Option D: Use Sequential Setup (FALLBACK)
**What**: Abandon parallel optimization, use sequential setup
**Why**: Most reliable, works on all systems
**How**: Keep `CreateGatewayCluster` (deprecated but functional)

**Expected Time**: ~7.6 min (baseline)
**Improvement**: None
**Trade-off**: Slower, but 100% reliable

---

## 📋 Recommendation

### For Gateway Team: **Option A** (Sequential Builds, Parallel Deploy)

**Why**:
- ✅ Maintains some parallelization benefit (14% improvement)
- ✅ Reliable on all systems (no resource exhaustion)
- ✅ No manual pre-build step required
- ✅ No system reconfiguration needed
- ✅ Simpler than full parallel (fewer failure modes)

**Implementation**:
- Update `SetupGatewayInfrastructureParallel` Phase 2
- Build Gateway and DataStorage images sequentially
- Deploy PostgreSQL + Redis in parallel (lightweight)

**Expected Outcome**:
- Setup time: ~6.5 min (vs ~7.6 min baseline)
- Memory usage: ~2 GB peak (vs ~4.5 GB parallel)
- Reliability: High (no OOM kills)

---

### For CI/CD: **Option B** (Pre-built Images)

**Why**:
- ✅ Fastest option (40% improvement)
- ✅ CI systems can pre-build in separate jobs
- ✅ E2E tests only load images (fast, reliable)
- ✅ Better for repeated test runs

**CI Pipeline**:
```yaml
jobs:
  build-images:
    runs-on: ubuntu-latest
    steps:
      - name: Build Gateway image
        run: podman build -t gateway:e2e-test -f Dockerfile.gateway .
      - name: Build DataStorage image
        run: podman build -t datastorage:e2e-test -f docker/data-storage.Dockerfile .
      - name: Save images
        run: podman save gateway:e2e-test datastorage:e2e-test -o images.tar

  e2e-tests:
    needs: build-images
    runs-on: ubuntu-latest
    steps:
      - name: Load pre-built images
        run: podman load -i images.tar
      - name: Run E2E tests
        run: go test ./test/e2e/gateway/... -v
```

---

## 🔗 References

**Parallel Optimization**: `docs/handoff/GATEWAY_E2E_PARALLEL_OPTIMIZATION_COMPLETE.md`
**ADR-028 Compliance**: `docs/handoff/GATEWAY_ADR028_COMPLIANCE_FIX.md`
**SignalProcessing Pattern**: `test/infrastructure/signalprocessing.go:246`

---

**Status**: 🔴 **BLOCKED** - Awaiting decision on resource strategy
**Priority**: P2 (V1.2) - Optimization, not a blocker for Gateway functionality
**Owner**: Gateway Team
**Recommended**: Option A (Sequential builds, 14% improvement, high reliability)


