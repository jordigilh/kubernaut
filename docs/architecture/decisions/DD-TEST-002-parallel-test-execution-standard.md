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
| E2E (20+ tests) | ~180s | ~60s | **3x faster** |
| **Total** | ~345s | ~115s | **3x faster** |

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
2. **ADR-004**: Fake Kubernetes Client (unit test isolation)
3. **ADR-005**: Integration Test Coverage (>50% target)
4. **SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md**: Template references this standard

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2025-11-28
**Next Review**: After V1.0 implementation complete

