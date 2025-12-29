# Appendix A: Integration Test Environment

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](../IMPLEMENTATION_PLAN_V1.1.md)
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0 §Integration Test Environment Decision
**Last Updated**: 2025-12-04

---

## 🔍 Integration Test Environment Decision

### Decision: KIND Cluster

**Selected Environment**: KIND (Kubernetes IN Docker)
**Rationale**: CRD controller with multi-CRD coordination requires real Kubernetes API

### Decision Tree Analysis

```
Is this a CRD controller?
├─ YES (RemediationOrchestrator)
│  └─ Does it coordinate multiple CRDs?
│     └─ YES (SignalProcessing, AIAnalysis, WorkflowExecution, NotificationRequest)
│        └─ ✅ Use KIND for full CRD coordination testing
```

---

## 📋 KIND Cluster Configuration

### Cluster Configuration (DD-TEST-001 Compliant)

**File**: `test/infrastructure/kind-remediationorchestrator-config.yaml`

```yaml
# Kind cluster configuration for Remediation Orchestrator E2E tests
# Port allocations per DD-TEST-001-port-allocation-strategy.md
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: kubernaut-ro-test
nodes:
- role: control-plane
  extraPortMappings:
  # RemediationOrchestrator E2E NodePort (DD-TEST-001)
  - containerPort: 30083    # RO E2E NodePort
    hostPort: 8083          # localhost:8083
    protocol: TCP
  # Metrics NodePort (DD-TEST-001)
  - containerPort: 30183    # Metrics NodePort
    hostPort: 9183          # localhost:9183 for Prometheus
    protocol: TCP
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        max-requests-inflight: "800"
        max-mutating-requests-inflight: "400"
    controllerManager:
      extraArgs:
        kube-api-qps: "100"
        kube-api-burst: "200"
- role: worker
```

### Port Allocation (DD-TEST-001)

| Service | Integration | E2E NodePort | Production | Purpose |
|---------|-------------|--------------|------------|---------|
| API | - | 30083 | 8083 | CRD controller (no HTTP API) |
| Metrics | - | 30183 | 9183 | Prometheus scraping |

**Note**: RO is a CRD controller with no HTTP API. Ports are for metrics only.

---

## 🧪 Test Environment Setup

### SynchronizedBeforeSuite Pattern

```go
// test/integration/remediationorchestrator/suite_test.go
package remediationorchestrator_test

import (
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

var (
    testEnv   *envtest.Environment
    k8sClient client.Client
    ctx       context.Context
    cancel    context.CancelFunc
)

func TestRemediationOrchestrator(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "RemediationOrchestrator Controller Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
    // First node: Setup test environment
    By("bootstrapping test environment")

    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
        ErrorIfCRDPathMissing: true,
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Register all CRD schemes
    err = remediationv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = signalprocessingv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = notificationv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())

    return nil
}, func(data []byte) {
    // All nodes: Use environment
    ctx, cancel = context.WithCancel(context.Background())
})

var _ = SynchronizedAfterSuite(func() {
    // All nodes: Cleanup
    cancel()
}, func() {
    // First node: Teardown
    By("tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

---

## 🔄 Test Isolation Patterns

### Unique Namespace Per Test

```go
// test/integration/remediationorchestrator/helpers_test.go
func createTestNamespace(ctx context.Context) string {
    namespace := fmt.Sprintf("test-ro-%s", uuid.New().String()[:8])

    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: namespace,
        },
    }

    Expect(k8sClient.Create(ctx, ns)).To(Succeed())

    DeferCleanup(func() {
        Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
    })

    return namespace
}
```

### Parallel Test Execution

```go
// test/integration/remediationorchestrator/parallel_test.go
var _ = Describe("Parallel Remediation Tests", func() {
    // Each test gets its own namespace
    var namespace string

    BeforeEach(func() {
        namespace = createTestNamespace(ctx)
    })

    // Tests can run in parallel safely
    It("should handle remediation A", func() {
        // Uses unique namespace
    })

    It("should handle remediation B", func() {
        // Uses different unique namespace
    })
})
```

---

## 🧪 Anti-Flaky Patterns

### EventuallyWithRetry for Status Checks

```go
// Wait for status with proper timeout
Eventually(func() string {
    rr := &remediationv1alpha1.RemediationRequest{}
    err := k8sClient.Get(ctx, client.ObjectKey{
        Name:      "test-remediation",
        Namespace: namespace,
    }, rr)
    if err != nil {
        return ""
    }
    return string(rr.Status.Phase)
}, 30*time.Second, 500*time.Millisecond).Should(Equal("Completed"))
```

### List-Based Verification

```go
// Use list instead of get for child CRDs
Eventually(func() int {
    spList := &signalprocessingv1alpha1.SignalProcessingList{}
    err := k8sClient.List(ctx, spList,
        client.InNamespace(namespace),
        client.MatchingLabels{"kubernaut.ai/remediation": rrName})
    if err != nil {
        return 0
    }
    return len(spList.Items)
}, 10*time.Second, 100*time.Millisecond).Should(Equal(1))
```

---

## 📊 Test Categories

| Category | Environment | Count | Parallel |
|----------|-------------|-------|----------|
| Unit Tests | In-memory | ~100 | Yes (4 procs) |
| Integration Tests | envtest | ~50 | Yes (4 procs) |
| E2E Tests | KIND | ~10 | No |

---

## 🔧 Makefile Targets

```makefile
# Run unit tests (parallel)
test-unit:
	go test -p 4 ./pkg/orchestrator/... -v

# Run integration tests (parallel with envtest)
test-integration:
	KUBEBUILDER_ASSETS="$(shell setup-envtest use -p path)" \
	go test -p 4 ./test/integration/remediationorchestrator/... -v

# Run E2E tests (KIND cluster)
test-e2e:
	kind create cluster --config test/e2e/kind-config.yaml --name kubernaut-test || true
	kubectl apply -f config/crd/bases/
	go test ./test/e2e/remediationorchestrator/... -v
	kind delete cluster --name kubernaut-test

# Run all tests
test-all: test-unit test-integration test-e2e
```

---

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](../IMPLEMENTATION_PLAN_V1.1.md)

