# Dynamic Toolset Integration Testing Infrastructure Evolution

## Current State (V1) - Fake Kubernetes Client ✅

### Infrastructure
- **Kubernetes Client**: `fake.NewSimpleClientset()` (in-memory)
- **Test Duration**: ~40 seconds for 12 specs
- **Confidence**: 95%

### Rationale
The Dynamic Toolset V1 is a **stateless HTTP service** that performs simple Kubernetes operations:
- `List()` - List all Services across namespaces
- `Get()` - Read Service metadata (labels, annotations, ports)
- `Create()/Update()` - Manage ConfigMaps
- `ServerVersion()` - Readiness checks

All of these operations are **fully supported** by the fake Kubernetes client, making it the appropriate choice per the testing strategy defined in `docs/testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md`.

### What Works Well
✅ **Fast**: < 1 second overhead per test
✅ **Reliable**: Zero flakiness, deterministic behavior
✅ **Simple**: No infrastructure dependencies
✅ **Sufficient**: Covers all V1 functionality

---

## Future State (V1.1+) - envtest with Real Kubernetes API Server ⚠️

### When to Migrate
**MANDATORY MIGRATION** when implementing **BR-TOOLSET-044: ToolsetConfig CRD** (V1.1)

### Why Migration is Required

When the Dynamic Toolset transitions from a stateless HTTP service to a **CRD-based Kubernetes controller** in V1.1, the fake client will **no longer be sufficient** because:

#### Fake Client Limitations for CRD Controllers
| Requirement | Fake Client | envtest | Impact |
|-------------|-------------|---------|--------|
| **CRD Support** | ❌ No CRDs | ✅ Full CRD support | **BLOCKING** |
| **Schema Validation** | ❌ No validation | ✅ Full validation | **BLOCKING** |
| **Watches** | ⚠️ Simplified | ✅ Real watches | **BLOCKING** |
| **Admission Webhooks** | ❌ Not supported | ✅ Supported | **BLOCKING** |
| **Controller Reconciliation** | ❌ No reconcile loop | ✅ Full reconcile | **BLOCKING** |
| **Status Subresource** | ⚠️ Limited | ✅ Full support | **BLOCKING** |

#### V1.1 Controller Requirements (BR-TOOLSET-044)
The ToolsetConfig CRD controller will need:
1. **CRD Installation** - `ToolsetConfig` custom resource
2. **Watch-based Reconciliation** - React to CRD spec changes
3. **Status Updates** - Update `.status.discoveredServices[]` with per-service health
4. **Schema Validation** - Validate discovery interval (1m-1h), namespace filters, etc.
5. **Controller-Runtime Integration** - Use `controller-runtime` manager

**None of these are supported by fake client** ❌

### Migration Plan (V1.1)

#### Step 1: Add envtest Infrastructure (1-2 days)
```go
// test/integration/toolset/suite_test.go (V1.1)
import (
    "sigs.k8s.io/controller-runtime/pkg/envtest"
    "k8s.io/client-go/kubernetes"
)

var (
    testEnv   *envtest.Environment
    k8sClient kubernetes.Interface
    cfg       *rest.Config
)

var _ = BeforeSuite(func() {
    // Start envtest (real K8s API server)
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).ToNot(HaveOccurred())

    // Use REAL Kubernetes client (not fake!)
    k8sClient, err = kubernetes.NewForConfig(cfg)
    Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
    err := testEnv.Stop()
    Expect(err).ToNot(HaveOccurred())
})
```

#### Step 2: Update Integration Tests (2-3 days)
- Replace `fake.NewSimpleClientset()` with `k8sClient` from envtest
- Add CRD creation/update tests
- Add controller reconciliation tests
- Add status update validation tests

#### Step 3: Add Controller-Specific Tests (3-4 days)
```go
// New tests for V1.1 CRD controller
var _ = Describe("BR-TOOLSET-044: ToolsetConfig CRD Controller", func() {
    It("should reconcile on spec changes", func() {
        // Create ToolsetConfig CRD
        config := &toolsetv1.ToolsetConfig{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-config",
                Namespace: "kubernaut-system",
            },
            Spec: toolsetv1.ToolsetConfigSpec{
                DiscoveryInterval: "10m",
                Namespaces:        []string{"monitoring"},
            },
        }

        err := k8sClient.Create(ctx, config)
        Expect(err).ToNot(HaveOccurred())

        // Wait for controller to reconcile
        Eventually(func() bool {
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(config), config)
            return err == nil && len(config.Status.DiscoveredServices) > 0
        }, "10s", "1s").Should(BeTrue())
    })
})
```

#### Step 4: Performance Optimization (1 day)
- envtest is slower than fake client (~5-10 seconds startup)
- Use `BeforeSuite` to start envtest once (not per-test)
- Reuse test environment across tests
- Expected duration: ~60-90 seconds (vs 40 seconds with fake client)

---

## Migration Trigger

### ✅ Keep Fake Client (Current V1)
- Service is stateless HTTP server
- No CRDs involved
- Simple LIST/GET operations
- Fast, reliable, sufficient
- **No parallel execution constraints**

### ⚠️ Migrate to envtest (V1.1 - BR-TOOLSET-044 OR Parallel Execution Constraints)

#### Trigger 1: CRD Controller Implementation (V1.1)
**TRIGGER**: When implementing ToolsetConfig CRD controller
**TIMELINE**: V1.1 development phase
**EFFORT**: 6-10 days
**RISK**: Low (well-documented migration path)

#### Trigger 2: Parallel Execution Constraints ⚠️ NEW
**TRIGGER**: When parallel test execution encounters infrastructure limitations
**EXAMPLES**:
- **K8s API Rate Limiting**: Real K8s clusters (Kind, Minikube) throttle concurrent requests
- **Client-Side Throttling**: Go Kubernetes client rate limiting across parallel processes
- **Resource Contention**: Multiple test processes competing for shared infrastructure
- **Test Flakiness**: Infrastructure-related failures that don't occur with in-memory testing

**EVIDENCE FROM GATEWAY SERVICE** (November 2025):
- 4 parallel test processes × 100 concurrent requests = 400 QPS
- Kind cluster with tuned K8s API limits (800 QPS) still showed client-side throttling
- p95 latency: 62 seconds (vs < 1s expected)
- Pass rate: 96% (vs 100% target) due to throttling-related timeouts
- **Solution**: Migrated to envtest → eliminated throttling, achieved 100% pass rate

**WHEN TO MIGRATE FOR PARALLEL EXECUTION**:
1. ✅ Parallel tests show "client-side throttling" messages
2. ✅ p95 latency > 5 seconds due to K8s API delays
3. ✅ Pass rate < 98% due to infrastructure timeouts
4. ✅ Tuning K8s API server limits doesn't resolve throttling
5. ✅ Need to support 4+ parallel test processes for CI/CD speed

**BENEFITS OF ENVTEST FOR PARALLEL EXECUTION**:
- ✅ **No Throttling**: In-memory K8s API server, no rate limits
- ✅ **Better Isolation**: Each test can have independent API server instance
- ✅ **Faster**: No Docker/networking overhead
- ✅ **Deterministic**: No infrastructure-related flakiness
- ✅ **CI/CD Ready**: Reliable 100% pass rate with parallel execution

**EFFORT**: 6-10 days (same as CRD migration)
**RISK**: Low (proven solution for Gateway service)

---

## Decision Authority

**Current Decision** (V1): Use fake client
- **Authority**: `docs/testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md`
- **Confidence**: 95%
- **Status**: ✅ **APPROVED**

**Future Decision** (V1.1): Migrate to envtest
- **Authority**: BR-TOOLSET-044 (ToolsetConfig CRD requirement)
- **Confidence**: 98% (CRD controllers REQUIRE envtest)
- **Status**: 📋 **PLANNED**

---

## Case Study: Gateway Service envtest Migration (November 2025)

### Problem
Gateway integration tests with 4 parallel processors hit K8s API throttling limits:
- **Infrastructure**: Kind cluster with tuned API limits (800 QPS capacity)
- **Load**: 4 parallel processes × 100 concurrent requests = 400 QPS
- **Result**: Client-side throttling despite 2x server capacity

### Symptoms
```
I1122 13:57:08 Waited for 11.001254208s due to client-side throttling
```
- p95 latency: **62 seconds** (target: < 1s)
- Pass rate: **96%** (119/124 tests)
- 5 failures due to CRD creation timeouts

### Solutions Attempted

#### Option A: K8s API Server Tuning ❌ FAILED
**Approach**: Increase Kind cluster API limits
```yaml
apiServer:
  extraArgs:
    max-requests-inflight: "800"           # 2x default
    max-mutating-requests-inflight: "400"  # 2x default
controllerManager:
  extraArgs:
    kube-api-qps: "100"    # 5x default
    kube-api-burst: "200"  # 6.6x default
```
**Result**: Still throttling at **client level**, not server level
**Conclusion**: Go Kubernetes client has built-in rate limiting that can't be bypassed

#### Option B: envtest Migration ✅ SUCCESS
**Approach**: Replace Kind cluster with in-memory K8s API server
**Architecture**:
```
Integration Test → Gateway Server → envtest (in-memory K8s)
                                  → Redis (Podman)
                                  → PostgreSQL (Podman)
                                  → Data Storage Service
```

**Results**:
- ✅ p95 latency: 62s → **< 1s** (60x improvement)
- ✅ Pass rate: 96% → **100%** (124/124 tests)
- ✅ No throttling messages
- ✅ Test duration: 3 min → **2 min** (33% faster)
- ✅ 4 parallel processors supported without issues

### Lessons Learned

1. **Client-Side Throttling is Real**: Even with unlimited server capacity, Go clients have rate limits
2. **envtest Eliminates Throttling**: In-memory API server bypasses all rate limiting
3. **Infrastructure Tuning Has Limits**: Can't solve client-side problems with server-side config
4. **Parallel Testing Requires envtest**: For 4+ parallel processes, envtest is mandatory
5. **Migration is Straightforward**: Well-documented path, 1-2 hours implementation

### When to Use envtest for Integration Tests

| Scenario | Use Fake Client | Use envtest |
|----------|----------------|-------------|
| Stateless HTTP service | ✅ Yes | ❌ No (overkill) |
| Simple K8s operations (LIST/GET) | ✅ Yes | ❌ No (overkill) |
| CRD controllers | ❌ No (not supported) | ✅ Yes (required) |
| **Parallel execution (4+ processes)** | ⚠️ Maybe (if no throttling) | ✅ **Yes (recommended)** |
| **High concurrent load (100+ requests)** | ⚠️ Maybe (if no throttling) | ✅ **Yes (recommended)** |
| **CI/CD with strict SLOs** | ⚠️ Maybe (if reliable) | ✅ **Yes (more reliable)** |

### Recommendation
**For services with high parallel test load**: Start with fake client, but **plan for envtest migration** if you see:
- Client-side throttling messages
- p95 latency > 5 seconds
- Pass rate < 98% due to timeouts
- Need for 4+ parallel processors

---

## References

### Current Architecture (V1)
- `docs/testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md` - Testing infrastructure decision tree
- `docs/architecture/decisions/ADR-004-fake-kubernetes-client.md` - Fake client rationale
- `test/integration/toolset/` - Current integration tests (fake client)

### Future Architecture (V1.1)
- `docs/requirements/BR-TOOLSET-044-ToolsetConfig-CRD.md` - CRD requirement
- `docs/architecture/decisions/DD-TOOLSET-001-REST-API-Deprecation.md` - V1.1 CRD migration plan
- `docs/testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md` - envtest guidance (Option B)

---

## Summary

| Aspect | V1 (Current) | V1.1 (Future) | Parallel Execution Constraint |
|--------|--------------|---------------|-------------------------------|
| **Service Type** | Stateless HTTP | CRD Controller | Any (HTTP or Controller) |
| **Test Infrastructure** | Fake Client | envtest | envtest |
| **Migration Trigger** | N/A | CRD requirement | K8s API throttling |
| **CRD Support** | N/A | ToolsetConfig CRD | Optional |
| **Parallel Processes** | 1-2 | 4+ | 4+ |
| **Test Duration** | ~40s | ~60-90s | ~40-60s (faster than Kind) |
| **Pass Rate** | 95%+ | 98%+ | 100% (no throttling) |
| **Confidence** | 95% | 98% | 99% |
| **Migration Effort** | N/A | 6-10 days | 6-10 days |
| **Status** | ✅ Production | 📋 Planned V1.1 | ✅ **Proven (Gateway)** |

**Conclusions**:
1. The current fake client approach is **correct and appropriate** for V1 (low parallel load)
2. Migration to envtest will be **mandatory** when implementing the CRD controller in V1.1
3. Migration to envtest is **also acceptable** when parallel execution encounters K8s API throttling (proven by Gateway service)
4. **New guideline**: For services requiring 4+ parallel test processes with high concurrent load, envtest is the **recommended** solution regardless of CRD usage

---

**Document Version**: 1.0
**Last Updated**: November 10, 2025
**Status**: ✅ **APPROVED**

