# DD-TEST-002: Parallel Test Execution Standard

**Status**: ✅ **APPROVED**
**Date**: 2025-11-28
**Related**: DD-TEST-001 (Port Allocation), ADR-005 (Integration Test Coverage)
**Applies To**: ALL Kubernaut services (universal standard)
**Confidence**: 95%

---

## Context & Problem

Kubernaut has 11 services with comprehensive test suites. Sequential test execution leads to:

1. **Long CI/CD pipelines** - 30+ minutes for full test suite
2. **Slow developer feedback** - Tests take too long during development
3. **Resource underutilization** - Modern CI runners have 4+ cores

**Key Questions**:
1. How many concurrent test processes should we run?
2. What isolation is required for parallel execution?
3. How do we prevent test interference?

---

## Decision

**APPROVED: 4 Concurrent Processes as Standard**

### Configuration

```bash
# Unit Tests
go test -v -p 4 ./test/unit/[service]/...
ginkgo -p -procs=4 -v ./test/unit/[service]/...

# Integration Tests
go test -v -p 4 ./test/integration/[service]/...
ginkgo -p -procs=4 -v ./test/integration/[service]/...

# E2E Tests
go test -v -p 4 ./test/e2e/[service]/...
ginkgo -p -procs=4 -v ./test/e2e/[service]/...

# All tests at once
ginkgo -p -procs=4 -v ./test/unit/[service]/... ./test/integration/[service]/... ./test/e2e/[service]/...
```

### Rationale

| Concurrency | Pros | Cons | Verdict |
|-------------|------|------|---------|
| **1 (sequential)** | Simple, no isolation needed | Slowest, underutilizes CPU | ❌ |
| **2** | Low interference risk | Still slow | ❌ |
| **4** | Balanced speed/safety | Standard CI runner capacity | ✅ **CHOSEN** |
| **8+** | Fastest | Risk of resource contention | ❌ |

**Why 4**:
- Standard GitHub Actions runner has 4 cores
- Balances speed and resource usage
- Matches common developer machine configuration
- Proven stable across Gateway, Notification, Data Storage implementations

---

## Isolation Requirements

### Unit Tests

| Requirement | Implementation | Enforced By |
|-------------|----------------|-------------|
| **No shared state** | Use `fake.NewClientBuilder()` per test | ADR-004 |
| **Unique contexts** | `context.Background()` per test | Test framework |
| **Independent assertions** | No global variables | Code review |

**Example**:
```go
var _ = Describe("Component", func() {
    var (
        ctx        context.Context
        fakeClient client.Client  // Fresh per test
    )

    BeforeEach(func() {
        ctx = context.Background()
        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()
    })

    // Tests are fully isolated
})
```

### Integration Tests

| Requirement | Implementation | Enforced By |
|-------------|----------------|-------------|
| **Unique namespace** | UUID-based namespace per test | Test setup |
| **Independent resources** | All resources in test namespace | RBAC |
| **Cleanup on teardown** | AfterEach deletes namespace | Test framework |

**Example**:
```go
var _ = Describe("Controller Integration", func() {
    var (
        ctx           context.Context
        testNamespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        // Unique namespace enables parallel execution
        testNamespace = fmt.Sprintf("test-%s", uuid.New().String()[:8])
        Expect(k8sClient.Create(ctx, &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
        })).To(Succeed())
    })

    AfterEach(func() {
        // Clean up namespace and all resources
        Expect(k8sClient.Delete(ctx, &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
        })).To(Succeed())
    })
})
```

### E2E Tests

| Requirement | Implementation | Enforced By |
|-------------|----------------|-------------|
| **Unique namespace** | UUID-based namespace per test | Test setup |
| **Unique NodePort** | Per DD-TEST-001 allocation | Infrastructure config |
| **No port-forward** | Use NodePort (stable) | Kind config |
| **Hybrid parallel setup** | Build images parallel → Create cluster → Load → Deploy | Infrastructure layer |

**Port Allocation Reference** (per DD-TEST-001):

| Service | API NodePort | Metrics NodePort |
|---------|--------------|------------------|
| Gateway | 30080 | 30090 |
| Data Storage | 30081 | 30181 |
| Signal Processing | 30082 | 30182 |
| Remediation Orchestrator | 30083 | 30183 |
| AIAnalysis | 30084 | 30184 |
| Remediation Execution | 30085 | 30185 |
| Notification | 30086 | 30186 |
| Toolset | 30087 | 30187 |

#### E2E Infrastructure Setup Strategy (AUTHORITATIVE)

**Standard**: **Hybrid Parallel Setup** (Dec 25, 2025)

All E2E test suites MUST use the hybrid parallel infrastructure setup:

```
PHASE 1: Build images in PARALLEL (fastest)
  ├── Service image (with coverage if enabled)
  ├── Dependencies (DataStorage, Redis, etc.)
  └── Wait for ALL builds to complete

PHASE 2: Create Kind cluster (after builds complete)
  ├── Install CRDs
  └── Create namespaces

PHASE 3: Load images into cluster (parallel)
  ├── Load service image
  └── Load dependency images

PHASE 4: Deploy services in PARALLEL with concurrent kubectl (MANDATORY)
  ├── Launch ALL kubectl apply commands concurrently (goroutines)
  ├── Collect all deployment results before proceeding
  └── Single wait for all services ready (Kubernetes reconciles dependencies)
```

**Rationale**:
- **Build Parallel**: Maximizes CPU utilization, reduces build time from ~7min to ~2-3min
- **Cluster After Builds**: Prevents Kind cluster timeout issues (no idle time waiting for builds)
- **Load Immediately**: Fresh cluster, no stale containers, reliable image loading
- **Deploy Parallel**: Fastest service startup (3-5s per service vs 15-25s sequential)
  - Kubernetes handles dependency ordering and reconciliation automatically
  - Concurrent `kubectl apply` is safe (manifests are idempotent)
  - Single readiness check after all manifests reduces wait time

**Benefits**:
- ✅ **Speed**: 5-6 minutes total (vs 7-8 minutes sequential)
- ✅ **Reliability**: 100% success rate (no Kind timeouts)
- ✅ **Simplicity**: Clear phases, easy to debug

**Implementation Example** (Gateway - AUTHORITATIVE):
```go
// test/infrastructure/gateway_e2e_hybrid.go
func SetupGatewayInfrastructureHybridWithCoverage(
    ctx context.Context,
    clusterName, kubeconfigPath string,
    writer io.Writer,
) error {
    // PHASE 1: Build images in parallel
    go buildGatewayImage(writer)
    go buildDataStorageImage(writer)
    waitForBuilds()

    // PHASE 2: Create cluster
    createKindCluster(clusterName, kubeconfigPath, writer)

    // PHASE 3: Load images in parallel
    go loadGatewayImage(clusterName, writer)
    go loadDataStorageImage(clusterName, writer)
    waitForLoads()

    // ═══════════════════════════════════════════════════════════════════════
    // PHASE 4: Deploy services in PARALLEL (MANDATORY PATTERN)
    // ═══════════════════════════════════════════════════════════════════════
    fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")

    type deployResult struct {
        name string
        err  error
    }
    deployResults := make(chan deployResult, 3) // Buffer = number of deployments

    // Launch ALL kubectl apply commands concurrently
    go func() {
        err := deployPostgreSQLRedis(ctx, namespace, kubeconfigPath, writer)
        deployResults <- deployResult{"PostgreSQL/Redis", err}
    }()
    go func() {
        err := deployDataStorageOnly(clusterName, kubeconfigPath, dsImage, writer)
        deployResults <- deployResult{"DataStorage", err}
    }()
    go func() {
        err := deployGatewayOnly(clusterName, kubeconfigPath, gwImage, writer)
        deployResults <- deployResult{"Gateway", err}
    }()

    // Collect ALL results before proceeding (MANDATORY)
    var deployErrors []error
    for i := 0; i < 3; i++ {
        result := <-deployResults
        if result.err != nil {
            fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
            deployErrors = append(deployErrors, result.err)
        } else {
            fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
        }
    }

    if len(deployErrors) > 0 {
        return fmt.Errorf("one or more service deployments failed: %v", deployErrors)
    }
    fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

    // Single wait for ALL services ready (Kubernetes handles dependencies)
    fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready...")
    if err := waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
        return fmt.Errorf("services not ready: %w", err)
    }

    return nil
}
```

**Anti-Pattern (FORBIDDEN)**: Old parallel approach (create cluster + build simultaneously)
```go
// ❌ WRONG: Creates cluster before builds complete
// Problem: Cluster sits idle 5-10min waiting for builds, times out
go createKindCluster()
go buildImages()
// Cluster may timeout before images are ready!
```

**Anti-Pattern (FORBIDDEN)**: Sequential Phase 4 deployment
```go
// ❌ WRONG: Sequential deployment wastes time
// Problem: 15-25 seconds per service = 45-75 seconds total for 3 services
deployPostgresRedis()      // Wait 15-20s
deployDataStorage()         // Wait 10-15s
deployGateway()            // Wait 10-15s
waitForAllServicesReady()  // Wait again!
// Total: 35-50+ seconds wasted on sequential kubectl apply
```

**Anti-Pattern (FORBIDDEN)**: Multiple readiness checks in Phase 4
```go
// ❌ WRONG: Checking readiness after EACH deployment
deployPostgresRedis()
waitForPostgresReady()     // Unnecessary wait
deployDataStorage()
waitForDataStorageReady()  // Unnecessary wait
deployGateway()
waitForGatewayReady()      // Unnecessary wait
// Problem: Wastes time - Kubernetes reconciles dependencies automatically
```

#### Phase 4 Implementation Checklist (MANDATORY)

**All E2E infrastructure functions MUST implement Phase 4 parallel deployment:**

✅ **Parallel Deployment Requirements**:
- [ ] Use goroutines to launch ALL `kubectl apply` commands concurrently
- [ ] Create buffered channel `deployResult` with buffer size = number of deployments
- [ ] Collect ALL results from channel before proceeding (no early returns)
- [ ] Check for errors AFTER all deployments complete
- [ ] Single `waitForAllServicesReady()` call after all manifests applied
- [ ] NO per-service readiness checks (Kubernetes handles this)

✅ **Performance Expectations**:
- Sequential: 15-25s per service → 45-75s for 3 services
- Parallel: 3-5s total → **10x faster**

✅ **Validation Commands**:
```bash
# Check for sequential deployment anti-pattern
grep -A 10 "deployPostgres\|deployDataStorage\|deployGateway" test/infrastructure/*.go | \
  grep -E "waitFor.*Ready|Eventually.*Pod.*Running" | \
  grep -v "waitForAllServicesReady"
# Should return: 0 results (no per-service waits)

# Check for parallel deployment pattern
grep -B 5 -A 5 "deployResults.*chan" test/infrastructure/*.go
# Should show: buffered channels + goroutines + result collection
```

✅ **Refactoring Guidance**:

If your service currently uses sequential Phase 4 deployment:

1. **Identify deployment functions** (e.g., `deployPostgresRedis()`, `deployDataStorage()`, `deployGateway()`)
2. **Create result channel**: `deployResults := make(chan deployResult, N)` where N = number of services
3. **Launch concurrent deployments**:
   ```go
   go func() {
       err := deployServiceA(...)
       deployResults <- deployResult{"ServiceA", err}
   }()
   // Repeat for all services
   ```
4. **Collect all results**: `for i := 0; i < N; i++ { result := <-deployResults; ... }`
5. **Single readiness check**: `waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer)`
6. **Remove per-service waits**: Delete any `waitForServiceAReady()` calls

**Estimated refactoring time**: 15-20 minutes per service

#### E2E Dockerfile Optimization (REQUIRED)

All service Dockerfiles MUST be optimized for fast E2E builds:

**Standard**: Use latest UBI9 base images, **NO** `dnf update`

```dockerfile
# ✅ CORRECT: Fast builds (~2 minutes)
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

# Install build dependencies (NO dnf update)
RUN dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

```dockerfile
# ❌ WRONG: Slow builds (~10 minutes)
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Upgrades ~58 packages on every build!
RUN dnf update -y && \
    dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

**Rationale**:
- Latest base images (`:1.25`, `:latest`) already have current packages
- `dnf update -y` adds 5-10 minutes to every build
- E2E tests run frequently, slow builds = slow feedback
- Parallel builds amplify the problem (multiple slow builds competing for resources)

**Impact**:
- **Before optimization**: 10 minutes per build (58 package upgrades)
- **After optimization**: 2 minutes per build (0 package upgrades)
- **Improvement**: 81% faster builds

**Validation Command**:
```bash
# Check for dnf update in Dockerfiles
grep -r "dnf update" docker/

# Should return: 0 results (no dnf update allowed)
```

---

## Anti-Patterns (FORBIDDEN)

### ❌ Shared Test Namespaces

```go
// ❌ WRONG: Shared namespace causes test interference
const testNamespace = "test-namespace"

var _ = Describe("Test A", func() {
    It("creates pod-1", func() {
        // Creates pod in shared namespace
    })
})

var _ = Describe("Test B", func() {
    It("lists pods", func() {
        // May see pod-1 from Test A - interference!
    })
})
```

### ❌ Fixed Resource Names

```go
// ❌ WRONG: Fixed names cause conflicts in parallel execution
pod := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "test-pod",  // Will conflict when tests run in parallel
        Namespace: testNamespace,
    },
}
```

**✅ CORRECT**:
```go
// ✅ RIGHT: Unique names per test
pod := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("test-pod-%s", uuid.New().String()[:8]),
        Namespace: testNamespace,
    },
}
```

### ❌ Global Test Fixtures

```go
// ❌ WRONG: Global fixtures cause race conditions
var globalClient client.Client

func init() {
    globalClient = createClient()
}
```

**✅ CORRECT**:
```go
// ✅ RIGHT: Per-suite fixtures with proper synchronization
var _ = SynchronizedBeforeSuite(func() []byte {
    // First process only - setup shared infra
    return nil
}, func(data []byte) {
    // All processes - setup local state
})
```

### ❌ Sequential Test Execution

```go
// ❌ WRONG: Missing parallel flags
go test ./test/unit/...  // Runs sequentially
ginkgo ./test/unit/...   // Runs sequentially
```

**✅ CORRECT**:
```go
// ✅ RIGHT: Always use parallel flags
go test -p 4 ./test/unit/...
ginkgo -procs=4 ./test/unit/...
```

---

## Makefile Targets

All test Makefile targets MUST include parallel flags:

```makefile
# Unit tests (parallel)
.PHONY: test-unit
test-unit:
	go test -v -p 4 -race ./test/unit/$(SERVICE)/...

# Integration tests (parallel)
.PHONY: test-integration
test-integration:
	go test -v -p 4 -race ./test/integration/$(SERVICE)/...

# E2E tests (parallel)
.PHONY: test-e2e
test-e2e:
	go test -v -p 4 ./test/e2e/$(SERVICE)/...

# All tests (parallel)
.PHONY: test-all
test-all:
	ginkgo -p -procs=4 -v --race \
		./test/unit/$(SERVICE)/... \
		./test/integration/$(SERVICE)/... \
		./test/e2e/$(SERVICE)/...
```

---

## CI/CD Configuration

### GitHub Actions

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Run Tests (4 parallel processes)
        run: |
          go test -v -p 4 -race ./test/unit/...
          go test -v -p 4 -race ./test/integration/...
          go test -v -p 4 ./test/e2e/...
```

---

## Performance Impact

Based on Gateway and Data Storage implementations:

| Test Tier | Sequential | Parallel (4) | Improvement |
|-----------|------------|--------------|-------------|
| Unit (100+ tests) | ~45s | ~15s | **3x faster** |
| Integration (50+ tests) | ~120s | ~40s | **3x faster** |
| E2E (37 tests) | ~480s | ~480s (8min) | **Same** (but see below) |
| **Total** | ~645s | ~535s | **1.2x faster** |

### E2E Infrastructure Setup Performance (**VALIDATED Dec 25, 2025**)

Hybrid parallel approach significantly improves E2E infrastructure setup time:

| Approach | Build Strategy | Setup Time | Result | Reliability |
|----------|---------------|------------|---------|-------------|
| **Old Sequential** | Gateway (10min) → DataStorage (10min) | ~20-25min | ✅ SUCCESS | 100% |
| **Old Parallel** | Gateway ‖ DataStorage ‖ Cluster (idle) | ~12min | ❌ **TIMEOUT** | 0% (cluster crash) |
| **Hybrid Parallel** ✅ | Gateway ‖ DataStorage → Cluster | **~5min** | ✅ **SUCCESS** | **100%** |

**Validated Metrics (Gateway E2E with mandatory dnf update)**:
- **Image Builds**: 5min (parallel with dnf update) vs 20min (sequential) → **4x faster**
- **Cluster Creation**: 15s (created after builds complete) → **No timeout issues**
- **Image Loading**: 30s (parallel) → **Reliable**
- **Service Deployment (Sequential)**: 45-75s (old approach)
- **Service Deployment (Parallel)**: 3-5s (new approach) → **10x faster**
- **Total Setup**: **~5 minutes** (optimized with parallel Phase 4) ✅
- **Test Execution**: 324 seconds (5.4 minutes) with 34/37 passing (91.9%)
- **Total E2E Time**: **~10 minutes** (setup + tests) vs 25+ minutes (old approaches)

**Key Learning**: Hybrid parallel setup with parallel Phase 4 deployment is **4x faster** AND **100% reliable**. The secret is:
1. Build images in parallel BEFORE creating the Kind cluster (prevents idle timeout)
2. Deploy ALL services concurrently with concurrent `kubectl apply` (10x faster)
3. Single readiness check after all manifests applied (Kubernetes reconciles dependencies)

---

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Parallel test pass rate** | 100% | No flaky tests due to isolation |
| **Speed improvement** | ≥2.5x | Time reduction vs sequential |
| **No test interference** | 0 incidents | Tests don't affect each other |

---

## Cross-References

1. **DD-TEST-001**: Port Allocation Strategy (NodePort for E2E)
2. **DD-E2E-001**: Parallel Image Builds for E2E Testing
3. **DD-INTEGRATION-001**: Local Image Builds for Integration Tests (includes parallel build guidance)
4. **ADR-004**: Fake Kubernetes Client (unit test isolation)
5. **ADR-005**: Integration Test Coverage (>50% target)
6. **SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md**: Template references this standard

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2025-12-26 (Phase 4 Parallel Deployment mandate added)
**Next Review**: After V1.0 implementation complete

