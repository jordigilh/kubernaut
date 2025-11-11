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

### ⚠️ Migrate to envtest (V1.1 - BR-TOOLSET-044)
**TRIGGER**: When implementing ToolsetConfig CRD controller
**TIMELINE**: V1.1 development phase
**EFFORT**: 6-10 days
**RISK**: Low (well-documented migration path)

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

| Aspect | V1 (Current) | V1.1 (Future) |
|--------|--------------|---------------|
| **Service Type** | Stateless HTTP | CRD Controller |
| **Test Infrastructure** | Fake Client | envtest |
| **CRD Support** | N/A | ToolsetConfig CRD |
| **Test Duration** | ~40s | ~60-90s |
| **Confidence** | 95% | 98% |
| **Migration Effort** | N/A | 6-10 days |
| **Status** | ✅ Production | 📋 Planned V1.1 |

**Conclusion**: The current fake client approach is **correct and appropriate** for V1. Migration to envtest will be **mandatory and straightforward** when implementing the CRD controller in V1.1.

---

**Document Version**: 1.0
**Last Updated**: November 10, 2025
**Status**: ✅ **APPROVED**

