# Appendix A: Integration Test Environment Decision

**Part of**: Signal Processing Implementation Plan V1.22
**Parent Document**: [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md)
**Last Updated**: 2025-12-04
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0

---

## 🔍 Integration Test Environment Decision

**CRITICAL**: This decision was made **before Day 1** using the decision tree below.

### Decision Tree

```
Does your service WRITE to Kubernetes (create/modify CRDs or resources)?
├─ YES → Does it need RBAC or TokenReview API?
│        ├─ YES → Use KIND (full K8s cluster)
│        └─ NO → Use ENVTEST (API server only)
│
└─ NO → Does it READ from Kubernetes?
         ├─ YES → Need field selectors or CRDs?
         │        ├─ YES → Use ENVTEST
         │        └─ NO → Use FAKE CLIENT
         │
         └─ NO → Use PODMAN (external services only)
                 or HTTP MOCKS (if no external deps)
```

---

## Signal Processing Decision: 🟡 **ENVTEST**

### Rationale

| Question | Answer | Implication |
|----------|--------|-------------|
| Writes to Kubernetes? | ✅ YES (CRD status updates) | Needs K8s API |
| Needs RBAC enforcement? | ❌ NO (controller-runtime handles RBAC) | ENVTEST sufficient |
| Uses TokenReview API? | ❌ NO | ENVTEST sufficient |
| Needs field selectors? | ✅ YES (for K8s enrichment queries) | ENVTEST required |
| External databases? | ❌ NO (uses Data Storage API) | HTTP mocks for Data Storage |

**Final Decision**: **ENVTEST** with HTTP mocks for Data Storage Service

---

## Classification Guide

### 🔴 KIND Required
**Use When**:
- Writes CRDs or Kubernetes resources
- Needs RBAC enforcement
- Uses TokenReview API for authentication
- Requires ServiceAccount permissions testing

**Examples**: Gateway Service, Dynamic Toolset Service (V2)

**Prerequisites**:
- [ ] KIND cluster available (`make bootstrap-dev`)
- [ ] Kind template documentation reviewed

---

### 🟡 ENVTEST Required ✅ **SIGNAL PROCESSING USES THIS**
**Use When**:
- Reads from Kubernetes (logs, events, resources)
- Needs field selectors (e.g., `.spec.nodeName=worker`)
- Writes ConfigMaps/Services (but no RBAC needed)
- Testing with CRDs (no RBAC validation)

**Examples**: Signal Processing, AIAnalysis, WorkflowExecution

**Prerequisites**:
- [x] `setup-envtest` installed (`go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`)
- [x] Binaries downloaded (`setup-envtest use 1.31.0`)

---

### 🟢 PODMAN Required
**Use When**:
- No Kubernetes operations
- Needs PostgreSQL, Redis, or other databases
- External service dependencies

**Examples**: Data Storage Service, Context API Service

**Prerequisites**:
- [ ] Docker/Podman available
- [ ] testcontainers-go configured

---

### ⚪ HTTP MOCKS Only
**Use When**:
- No Kubernetes operations
- No database dependencies
- Only HTTP API calls to other services

**Examples**: Effectiveness Monitor Service, Notification Service

**Prerequisites**:
- [ ] None (uses Go stdlib `net/http/httptest`)

---

## Quick Classification Examples

| Service Type | Kubernetes Ops | Databases | Test Env |
|--------------|---------------|-----------|----------|
| Writes CRDs + RBAC | ✅ Write + RBAC | ❌ | 🔴 KIND |
| Writes ConfigMaps only | ✅ Write (no RBAC) | ❌ | 🟡 ENVTEST |
| Reads K8s (field selectors) | ✅ Read (complex) | ❌ | 🟡 ENVTEST |
| Reads K8s (simple) | ✅ Read (simple) | ❌ | Fake Client |
| HTTP API + PostgreSQL | ❌ | ✅ | 🟢 PODMAN |
| HTTP API only | ❌ | ❌ | ⚪ HTTP MOCKS |
| **Signal Processing** | ✅ CRD + Read | ❌ (HTTP to Data Storage) | 🟡 **ENVTEST** |

---

## Signal Processing Test Infrastructure

### Unit Tests (Fake Client)
```go
// test/unit/signalprocessing/enricher_test.go
import (
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnricher(t *testing.T) {
    // Use fake client for unit tests
    client := fake.NewClientBuilder().
        WithScheme(scheme).
        WithObjects(existingPod).
        Build()

    enricher := NewEnricher(client, logger)
    // ...
}
```

### Integration Tests (ENVTEST)
```go
// test/integration/signalprocessing/suite_test.go
import (
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

var testEnv *envtest.Environment
var k8sClient client.Client

var _ = BeforeSuite(func() {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    // Create client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    By("tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### E2E Tests (KIND)
```go
// test/e2e/signalprocessing/suite_test.go
// Uses full KIND cluster for E2E validation
// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
```

---

## Test Environment Matrix

| Test Type | Environment | K8s Resources | External Services |
|-----------|-------------|---------------|-------------------|
| **Unit** | Fake Client | Mocked | Mocked |
| **Integration** | ENVTEST | Real API Server | HTTP Mocks |
| **E2E** | KIND | Full Cluster | Real Services |

---

## Reference Documentation

- [Integration Test Environment Decision Tree](../../../../../testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md)
- [envtest Setup Requirements](../../../../../testing/ENVTEST_SETUP_REQUIREMENTS.md)
- [DD-TEST-001: Port Allocation Strategy](../../../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)
- [ADR-004: Fake K8s Client](../../../../../architecture/decisions/ADR-004-fake-kubernetes-client.md)

---

## Related Documents

- [Main Implementation Plan](../IMPLEMENTATION_PLAN.md)
- [Appendix B: CRD Controller Patterns](APPENDIX_B_CRD_CONTROLLER_PATTERNS.md)
- [Appendix C: Confidence Methodology](APPENDIX_C_CONFIDENCE_METHODOLOGY.md)
- [Appendix D: ADR/DD Reference Matrix](APPENDIX_D_ADR_DD_REFERENCE_MATRIX.md)
- [Testing Strategy](../../testing-strategy.md)

