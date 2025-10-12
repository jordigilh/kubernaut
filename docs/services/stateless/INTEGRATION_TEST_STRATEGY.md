# Stateless Services Integration Test Strategy

**Version**: 1.0
**Last Updated**: 2025-10-12
**Status**: ✅ Approved

---

## Executive Summary

This document defines the **approved integration test strategy** for all Kubernaut stateless services. Each service uses the **minimal infrastructure** required to validate its integration points.

**Guiding Principle**: Use the **simplest test environment** that validates real integration behavior.

---

## Decision Framework

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

### ⚠️ **CRITICAL: Server Execution Location**

**The decision framework above is INCOMPLETE without considering where the server runs:**

```
After selecting environment (KIND/ENVTEST/etc), ask:

WHERE DOES THE SERVER RUN IN TESTS?
├─ IN-CLUSTER (deployed as Pod)
│  ├─ KIND → ✅ Can test health checks, DNS, auth, networking
│  └─ ENVTEST → ⚠️ Can't deploy pods, consider KIND
│
└─ LOCAL (test process)
   ├─ KIND → ⚠️ Can't reach cluster DNS (.svc.cluster.local)
   │         → Can't test health checks or auth
   │         → Same coverage as ENVTEST but more complex
   │         → USE ENVTEST INSTEAD
   │
   └─ ENVTEST → ✅ Simple, fast, identical coverage to KIND for local execution
```

**Key Insight**: If your server runs **in the test process** (local execution):
- ✅ Use **ENVTEST** - simpler setup, identical test coverage
- ❌ Avoid **KIND** - added complexity with **zero** benefit

KIND is **only beneficial** when server runs **in-cluster** (deployed as a Pod).

---

## Lesson Learned: Dynamic Toolset V1 Migration

**What Happened**:
- Dynamic Toolset V1 was migrated from envtest to KIND
- Migration took ~4-6 hours (echo servers, RBAC, test updates)
- **Result**: KIND provided **zero additional test coverage** over envtest

**Why**:
- Server runs **in test process** (local execution)
- Can't resolve cluster DNS (`.svc.cluster.local`)
- Health checks fail in KIND the same way they would in envtest
- Authentication can't be tested with local server execution

**Test Coverage Comparison** (V1, Local Server):

| Capability | envtest | KIND (local server) | Different? |
|------------|---------|---------------------|-----------|
| Service Discovery | ✅ Yes | ✅ Yes | ❌ **Same** |
| ConfigMap CRUD | ✅ Yes | ✅ Yes | ❌ **Same** |
| Health Checks | ❌ No backends | ❌ DNS fails | ❌ **Same (both fail)** |
| Authentication | ❌ No TokenReview | ❌ Can't reach from local | ❌ **Same (both fail)** |

**Conclusion**: For V1 with local server, envtest was the correct choice. KIND migration added complexity without benefit.

**Correct Approach for Future Services**:
1. **First**: Decide if server runs in-cluster or locally
2. **Then**: Choose test environment based on execution model
3. **Avoid**: Choosing KIND just because "infrastructure exists"

---

## Service Classification

### 🔴 KIND Required (Full Kubernetes Cluster)

| Service | Reason | Test Duration |
|---------|--------|---------------|
| **Gateway Service** | Writes CRDs + TokenReview auth + RBAC | ~2-3 min |

**Why KIND**:
- ✅ Full RBAC enforcement
- ✅ TokenReview API for authentication
- ✅ Real ServiceAccount permissions
- ✅ Complete Kubernetes API surface

**Setup Cost**:
- First run: ~60 seconds (cluster creation)
- Subsequent: ~10 seconds (cached cluster)
- CI/CD: Cache cluster image

---

### 🟡 ENVTEST (API Server, No RBAC)

| Service | Reason | Test Duration |
|---------|--------|---------------|
| **Dynamic Toolset Service (V1)** | Writes ConfigMaps + watches (no RBAC/leader election in V1) + **Server runs locally** | ~60 sec |
| **HolmesGPT API Service** | Reads K8s logs/events (may need field selectors) | ~5-10 sec |

**Why ENVTEST**:
- ✅ Real API server validation
- ✅ Full Kubernetes API (Service discovery, ConfigMap writes, watch events)
- ✅ Fast feedback loop (~3 seconds setup vs. ~60 seconds for KIND)
- ✅ Sufficient for V1 functionality (no RBAC, leader election, or TokenReview)
- ✅ **Server runs in test process** (local) - can't reach cluster-internal services
- ❌ No authentication/RBAC testing (not needed for V1)
- ❌ No service backends (health validation covered by unit tests)

**Setup Cost**:
- Requires `setup-envtest` (~70MB binaries)
- First run: ~10 seconds (binary download)
- Subsequent: ~2 seconds (API server start)

**Critical Decision Factor**: Dynamic Toolset V1 server runs **in the test process** (local execution), not deployed in-cluster. With local execution:
- ✅ envtest provides identical test coverage to KIND (service discovery + ConfigMap operations)
- ✅ envtest is simpler (no deployment management, RBAC, or echo servers)
- ❌ KIND provides no additional benefit (can't test health checks or auth with local server)

**Migration to KIND**: Dynamic Toolset will migrate to KIND in V2 **only if** deploying server in-cluster OR implementing leader election + RBAC.

**Lesson Learned**: Test strategy should be based on **where the server runs**, not what infrastructure exists. See "Decision Criteria" section below.

**Alternative**: HolmesGPT can start with **Fake Client**, upgrade to envtest if field selectors needed.

---

### 🟢 PODMAN (External Services Only)

| Service | Dependencies | Test Duration |
|---------|--------------|---------------|
| **Data Storage Service** | PostgreSQL + Redis + pgvector | ~3-5 sec |
| **Context API Service** | PostgreSQL + Redis + pgvector | ~3-5 sec |

**Why PODMAN**:
- ✅ No Kubernetes operations
- ✅ Real database behavior
- ✅ Fast container startup
- ✅ Shared containers between services

**Setup Cost**:
- First run: ~5-10 seconds (container pull)
- Subsequent: ~1-2 seconds (cached images)
- Cleanup: Automatic via testcontainers

---

### ⚪ HTTP MOCKS (No Infrastructure)

| Service | Mock Dependencies | Test Duration |
|---------|-------------------|---------------|
| **Effectiveness Monitor Service** | Data Storage API + Infrastructure Monitoring API | ~1-2 sec |
| **Notification Service** | SMTP + Slack/Teams webhooks | ~1-2 sec |

**Why HTTP MOCKS**:
- ✅ Pure HTTP API testing
- ✅ No external dependencies
- ✅ Instant test startup
- ✅ Easy to simulate failures

**Setup Cost**:
- Zero infrastructure
- Immediate execution

---

## Testing Strategy by Service

### 1. Gateway Service 🔴 KIND

**Integration Test Environment**: **KIND cluster with CRDs + RBAC**

**What to Test**:
```yaml
Critical Integration Points:
  - TokenReview authentication (validate ServiceAccount tokens)
  - RemediationRequest CRD creation (with RBAC checks)
  - Signal deduplication (Redis integration)
  - Environment classification (namespace label reads)
  - Priority assignment (Rego policy evaluation)

Test Environment:
  - Kind cluster: kubernaut-gateway-test
  - Redis: redis.integration.svc.cluster.local:6379
  - CRDs: config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml
  - RBAC: config/rbac/gateway_role.yaml
```

**Test Setup**:
```go
// Uses pkg/testutil/kind/ helper
suite := kind.NewIntegrationTestSuite(kind.Config{
    ClusterName: "kubernaut-gateway-test",
    Namespace:   "integration",
    CRDPaths:    []string{"../../config/crd/bases"},
})

// Redis available at redis.integration.svc.cluster.local:6379
```

**Reference**: `docs/services/stateless/gateway-service/testing-strategy.md`

---

### 2. Dynamic Toolset Service 🟡 ENVTEST (V1)

**Integration Test Environment**: **ENVTEST** (API Server Only)

**What to Test**:
```yaml
Critical Integration Points:
  - Service discovery (list Services with labels/annotations)
  - ConfigMap creation/update (with OwnerReferences)
  - ConfigMap operations (watch + CRUD operations)
  - Multi-namespace discovery
  - Service detection logic (labels, ports, annotations)

Test Environment:
  - envtest: Kubernetes API server (no RBAC, no TokenReview)
  - ConfigMap: kubernaut-toolset-config in kubernaut-system namespace
  - Server: Runs in test process (local execution)
  - Health checks: Validated by unit tests (80+ specs)
```

**Rationale for ENVTEST (V1)**:
- ✅ **Server Runs Locally**: Test process can't reach cluster-internal services
- ✅ **Simple Setup**: No deployment management, RBAC, or echo servers needed
- ✅ **Identical Coverage**: Tests same integration points as KIND would with local server
- ✅ **Fast Feedback**: ~3 second setup vs. ~60 seconds for KIND
- ❌ **No Health Checks**: Unit tests cover health check logic (mocked HTTP)
- ❌ **No Authentication**: V1 doesn't require TokenReview testing

**Why Not KIND for V1?**:
- ❌ Server runs **outside cluster** (in test process) → Can't resolve `.svc.cluster.local` DNS
- ❌ Can't test authentication with local server execution
- ❌ Health checks fail the same way in KIND as they would in envtest (no reachable backends)
- ❌ Added complexity (deployments, RBAC, echo servers) with **zero** additional test coverage

**Test Setup**:
```go
// Uses envtest
var _ = BeforeSuite(func() {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd")},
    }

    cfg, err = testEnv.Start()
    k8sClient, err = kubernetes.NewForConfig(cfg)

    // Create test services (no backends needed)
    createTestServices(ctx, k8sClient)
})
```

**Migration to KIND (V2)**:
Dynamic Toolset will migrate to KIND **only if** implementing:
- Server deployment in-cluster (pod-based execution)
- Leader election (multi-replica coordination)
- TokenReview authentication
- RBAC enforcement

**Reference**: `docs/services/stateless/dynamic-toolset/testing-strategy.md`

---

### 3. Data Storage Service 🟢 PODMAN

**Integration Test Environment**: **PostgreSQL + Redis + pgvector containers**

**What to Test**:
```yaml
Critical Integration Points:
  - PostgreSQL audit trail writes (with transactions)
  - Vector embeddings (pgvector extension)
  - Embedding cache (Redis with TTL)
  - Schema initialization (DDL execution)
  - Idempotent writes (duplicate detection)

Test Environment:
  - PostgreSQL: postgres:15-alpine with pgvector
  - Redis: redis:7-alpine
  - No Kubernetes operations
```

**Test Setup**:
```go
// Uses testcontainers-go
postgresContainer := testcontainers.GenericContainer{
    Image: "pgvector/pgvector:pg15",
    Env: map[string]string{
        "POSTGRES_DB": "testdb",
    },
    WaitStrategy: wait.ForLog("database system is ready"),
}

redisContainer := testcontainers.GenericContainer{
    Image: "redis:7-alpine",
}
```

**Reference**: `docs/services/stateless/data-storage/testing-strategy.md`

---

### 4. Context API Service 🟢 PODMAN

**Integration Test Environment**: **PostgreSQL + Redis + pgvector containers (shared)**

**What to Test**:
```yaml
Critical Integration Points:
  - PostgreSQL historical queries (with partitioning)
  - Vector semantic search (similarity queries)
  - Cache layer (Redis with multi-TTL)
  - Success rate calculations (SQL aggregations)
  - Partition management (time-based queries)

Test Environment:
  - PostgreSQL: Same as Data Storage (shared container)
  - Redis: Same as Data Storage (shared container)
  - No Kubernetes operations
```

**Reference**: `docs/services/stateless/context-api/testing-strategy.md`

---

### 5. HolmesGPT API Service 🟡 ENVTEST (or Fake Client)

**Integration Test Environment**: **Fake Client (start) → envtest (if needed)**

**What to Test**:
```yaml
Critical Integration Points:
  - Kubernetes log retrieval (Pod logs)
  - Event analysis (list K8s Events)
  - Resource inspection (get Pod/Deployment/etc)
  - ConfigMap polling (toolset configuration)
  - LLM provider integration (OpenAI/Claude/local)

Test Environment (Phase 1 - Fake Client):
  - Fake K8s client (pre-populated test data)
  - No field selectors needed initially
  - Mock LLM responses

Test Environment (Phase 2 - envtest, if needed):
  - Real API server (if field selectors required)
  - Requires setup-envtest
```

**Decision Point**: Start with **Fake Client**. Upgrade to **envtest** only if:
- HolmesGPT SDK requires field selectors
- Investigation logic needs complex K8s queries
- Testing reveals fake client limitations

**Reference**: `docs/services/stateless/holmesgpt-api/testing-strategy.md`

---

### 6. Effectiveness Monitor Service ⚪ HTTP MOCKS

**Integration Test Environment**: **HTTP mocks (no infrastructure)**

**What to Test**:
```yaml
Critical Integration Points:
  - Data Storage API client (action history queries)
  - Infrastructure Monitoring API client (metrics queries)
  - Graceful degradation (circuit breaker patterns)
  - Assessment calculation (effectiveness scoring)

Test Environment:
  - httptest.Server mocks for Data Storage
  - httptest.Server mocks for Infrastructure Monitoring
  - No real infrastructure needed
```

**Test Setup**:
```go
// Mock Data Storage API
mockDataStorage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if strings.Contains(r.URL.Path, "/actions") {
        json.NewEncoder(w).Encode(mockActions)
    }
}))

// Mock Infrastructure Monitoring
mockInfraMonitoring := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(mockMetrics)
}))
```

**Reference**: `docs/services/stateless/effectiveness-monitor/testing-strategy.md`

---

### 7. Notification Service ⚪ HTTP MOCKS

**Integration Test Environment**: **HTTP mocks (no infrastructure)**

**What to Test**:
```yaml
Critical Integration Points:
  - Multi-channel delivery (Email, Slack, Teams, SMS)
  - Template rendering (channel-specific formatting)
  - Sensitive data sanitization (regex patterns)
  - External service links (GitHub, Grafana, etc.)

Test Environment:
  - Mock SMTP server (optional: Mailhog container)
  - Mock webhook endpoints (httptest.Server)
  - No real external services
```

**Reference**: `docs/services/stateless/notification-service/testing-strategy.md`

---

## Setup Requirements Summary

| Environment | Prerequisites | First Run | Subsequent | CI/CD Cache |
|-------------|--------------|-----------|------------|-------------|
| **KIND** | Docker/Podman + `kind` CLI | ~60 sec | ~10 sec | Cluster image |
| **ENVTEST** | `setup-envtest` (~70MB) | ~10 sec | ~2 sec | Binaries |
| **PODMAN** | Docker/Podman | ~5-10 sec | ~1-2 sec | Container images |
| **HTTP MOCKS** | None | Instant | Instant | None |

---

## CI/CD Optimization

### GitHub Actions Example

```yaml
name: Integration Tests

jobs:
  test-kind-services:
    name: KIND Tests (Gateway)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Cache Kind cluster
        uses: actions/cache@v3
        with:
          path: ~/.kind
          key: kind-cluster-${{ runner.os }}

      - name: Run Gateway tests
        run: make test-integration-gateway

  test-podman-services:
    name: Podman Tests (Data Storage, Context API)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Cache container images
        uses: actions/cache@v3
        with:
          path: ~/.docker
          key: podman-images-${{ runner.os }}

      - name: Run Data Storage tests
        run: make test-integration-data-storage

      - name: Run Context API tests
        run: make test-integration-context-api

  test-mock-services:
    name: Mock Tests (Effectiveness Monitor, Notification)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Run Effectiveness Monitor tests
        run: make test-integration-effectiveness

      - name: Run Notification tests
        run: make test-integration-notification

  test-envtest-services:
    name: envtest Tests (Dynamic Toolset V1, HolmesGPT API)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Cache envtest binaries
        uses: actions/cache@v3
        with:
          path: ~/testbin
          key: envtest-${{ runner.os }}-1.31.0

      - name: Setup envtest
        run: |
          go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
          setup-envtest use 1.31.0 --bin-dir ./testbin -p path

      - name: Run Dynamic Toolset tests (envtest)
        run: make test-integration-toolset
        env:
          KUBEBUILDER_ASSETS: ${{ github.workspace }}/testbin/k8s/1.31.0-linux-amd64
        # Note: V1 uses envtest (local server execution)
        # V2 will migrate to KIND when deploying server in-cluster

      - name: Run HolmesGPT API tests
        run: make test-integration-holmesgpt
        env:
          KUBEBUILDER_ASSETS: ${{ github.workspace }}/testbin/k8s/1.31.0-linux-amd64
```

---

## Performance Targets

| Service | Environment | Setup Time | Test Duration | Total |
|---------|------------|-----------|---------------|-------|
| Gateway | KIND | ~10 sec | ~2-3 min | ~2.5 min |
| Dynamic Toolset (V1) | **envtest** | ~3 sec | ~60 sec | ~65 sec |
| Data Storage | Podman | ~2 sec | ~30 sec | ~40 sec |
| Context API | Podman | ~2 sec | ~30 sec | ~40 sec |
| HolmesGPT API | envtest/Fake | ~2 sec | ~10 sec | ~15 sec |
| Effectiveness Monitor | Mocks | Instant | ~5 sec | ~5 sec |
| Notification | Mocks | Instant | ~5 sec | ~5 sec |

**Total Parallel Execution**: ~2.5 minutes (Gateway on KIND, others in parallel on envtest/Podman/mocks)

**Note**: Dynamic Toolset V1 uses envtest (not KIND) because server runs locally in tests. KIND would provide identical test coverage with ~10x slower setup time.

---

## Migration Path

### Existing Services

1. **Gateway Service**: Already using KIND ✅
2. **Dynamic Toolset Service V1**: Using envtest ✅ (will migrate to KIND in V2 for leader election + RBAC)
3. **Data Storage Service**: New service (use Podman from day 1)
4. **Other Services**: Implement per classification above

### Dynamic Toolset V2 Migration (Future)

When implementing V2 features, migrate Dynamic Toolset from envtest to KIND:

**V2 Triggers** (any of these):
- Leader election implementation (multi-replica coordination)
- RBAC enforcement (ServiceAccount-based permissions)
- TokenReview authentication
- Network policies

**Migration Effort**: ~4-6 hours
- Create KIND manifests for mock services (Prometheus, Grafana, Jaeger, Elasticsearch)
- Update test suite to use KIND cluster
- Restore normal health check timeouts (remove fast-fail)
- Update expectations (health checks should pass with real backends)

### Future Services

All new stateless services must:
1. Reference this strategy document
2. Use the decision framework
3. Document chosen approach in `testing-strategy.md`
4. Include justification if deviating from recommendations

---

## References

- [Integration Test Environment Decision Tree](../../testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md)
- [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)
- [Podman Integration Test Template](../../testing/PODMAN_INTEGRATION_TEST_TEMPLATE.md)
- [Kind Cluster Test Template](../../../pkg/testutil/kind/README.md)

---

**Document Status**: ✅ Approved Strategy
**Last Updated**: 2025-10-12
**Maintainer**: Kubernaut Architecture Team


