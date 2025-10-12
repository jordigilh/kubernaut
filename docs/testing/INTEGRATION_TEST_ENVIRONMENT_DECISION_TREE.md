# Integration Test Environment Decision Tree

**Date**: 2025-10-12
**Status**: ✅ APPROVED
**Authority**: Architecture Team
**Related**: [ADR-003-KIND-INTEGRATION-ENVIRONMENT.md](../architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md)

---

## Executive Summary

**Principle**: Use the **simplest integration test environment** that validates your service's actual dependencies.

- **Podman/Testcontainers**: For stateless services with ONLY external dependencies (databases, Redis, message queues)
- **Kind Cluster**: For services that interact with Kubernetes APIs or CRDs

**Why This Matters**:
- Kind cluster setup: 30-60 seconds per test run
- Podman containers: 2-5 seconds per test run
- **6-12x faster feedback loop** for services that don't need Kubernetes

---

## 🌳 Decision Tree

```
START: Which integration test environment should I use?
│
├─ Does your service interact with Kubernetes APIs?
│  ├─ YES → Does it CREATE or MODIFY K8s resources? (Pods, Services, ConfigMaps, etc.)
│  │        ├─ YES → Use KIND (need real K8s API server with RBAC)
│  │        └─ NO → Does it only READ K8s resources? (list Pods, get Services, etc.)
│  │                 ├─ YES → Use FAKE CLIENT or ENVTEST (in-memory K8s API)
│  │                 └─ NO → Continue to CRD check
│  │
│  └─ NO → Does your service use CRDs or watch Kubernetes resources?
│           ├─ YES → Use KIND (CRDs require real Kubernetes API server)
│           └─ NO → Does your service depend on RBAC/ServiceAccounts?
│                    ├─ YES → Use KIND (RBAC requires real Kubernetes)
│                    └─ NO → Use PODMAN (stateless service with external deps only)
```

---

## 📊 Service Type Classification

| Service Type | Integration Environment | Rationale | Example |
|-------------|------------------------|-----------|---------|
| **CRD Controller** | **KIND (Required)** | Must interact with Kubernetes API, watch CRDs, update status | Remediation Request Controller |
| **Stateless + K8s Write** | **KIND (Required)** | Creates/modifies K8s resources (Pods, Services, ConfigMaps) | Dynamic Toolset Service |
| **Stateless + K8s Read** | **FAKE CLIENT or ENVTEST (Recommended)** | Only READS K8s resources (list Pods, get Services) - no writes | Metrics collector, Status reporter |
| **Stateless + External Deps** | **PODMAN (Recommended)** | Only needs databases, Redis, message queues - no K8s interaction | Gateway Service, Data Storage Service |
| **AI/ML Service** | **PODMAN (Recommended)** | Only needs AI APIs, databases, vector DBs - no K8s interaction | Context Optimization Service |
| **Webhook Validator** | **KIND (Required)** | Validates K8s admission requests, needs ServiceAccount auth | Validation Webhook |
| **Pure Business Logic** | **Unit Tests Only** | No external dependencies or K8s APIs | Embedding generators, validators |

---

## ✅ Use PODMAN When...

### Criteria
Your service meets **ALL** of these:
1. ✅ Does NOT create/modify Kubernetes resources
2. ✅ Does NOT use CRDs
3. ✅ Does NOT require RBAC/ServiceAccount authentication
4. ✅ Only depends on: databases, Redis, message queues, HTTP APIs
5. ✅ Kubernetes integration is only for deployment (not business logic)

### Examples

#### Gateway Service ✅ PODMAN
**Why**: Only needs Redis for deduplication
```yaml
Dependencies:
  - Redis (containerized via Podman)
  - TokenReview API (can be mocked)

Integration Tests:
  - Redis signal deduplication
  - Rate limiting with Redis
  - Signal forwarding
  - Health checks

NO Kubernetes API calls in business logic
```

#### Data Storage Service ✅ PODMAN
**Why**: Only needs PostgreSQL and Vector DB
```yaml
Dependencies:
  - PostgreSQL (containerized via Podman)
  - Vector DB (containerized via Podman)
  - Redis cache (containerized via Podman)

Integration Tests:
  - Database writes
  - Dual-write transactions
  - Embedding generation
  - Query API

NO Kubernetes API calls in business logic
```

#### Context Optimization Service ✅ PODMAN
**Why**: Only needs AI APIs and databases
```yaml
Dependencies:
  - PostgreSQL (containerized via Podman)
  - Redis cache (containerized via Podman)
  - AI/LLM API (can be mocked HTTP server)

Integration Tests:
  - Context analysis
  - Optimization logic
  - Database persistence
  - Cache behavior

NO Kubernetes API calls in business logic
```

---

## 🧪 Use IN-MEMORY KUBERNETES (Fake Client or envtest) When...

### Criteria
Your service meets **ALL** of these:
1. ✅ Reads Kubernetes resources (list Pods, get Services, get Nodes, etc.)
2. ✅ Does NOT create or modify Kubernetes resources (or only simple creates)
3. ✅ Does NOT use CRDs (or only simple CRDs with envtest)
4. ✅ Does NOT require real RBAC (can use fake permissions)
5. ✅ Needs Kubernetes API behavior without full cluster overhead

### Why This Is Better Than Mocking
- ✅ **Tests real K8s client behavior** (not mocked methods)
- ✅ **No manual mocks to maintain** when K8s API changes
- ✅ **In-memory, fast** (milliseconds for fake, ~2s for envtest vs 30-60s for Kind)
- ✅ **More realistic than mocks** but simpler than Kind

### Two Options: Fake Client vs envtest

#### 🎯 Decision Guide: Which One Should I Use?

```
Does your test need any of these features?
├─ Custom Resource Definitions (CRDs)
├─ API server validation (schema validation, field validation)
├─ Field selectors (e.g., list Pods by nodeName)
├─ Label selectors with complex expressions
├─ Server-side filtering
├─ Watches that behave like real API server
│
├─ YES to any → Use ENVTEST (real API server + controller-runtime client)
│                ⚠️ Requires setup-envtest (~70MB binaries)
│                ✅ Full CRD support with schema validation
└─ NO to all → Use FAKE CLIENT (simpler and faster)
                ✅ Zero prerequisites, instant setup
                ❌ No CRD support
```

#### Option A: Fake Client (Start Here) ⭐

**What It Is**:
- Purely in-memory object store
- NO API server - just stores objects in memory
- Uses fake client from `k8s.io/client-go/kubernetes/fake`

**When to Use**:
- ✅ Simple LIST operations (list all Pods)
- ✅ Simple GET operations (get specific Pod)
- ✅ You control all test data (pre-populate objects)
- ✅ No API server validation needed
- ✅ Speed is critical (< 1 second)

**Limitations**:
- ❌ No schema validation (invalid objects won't be rejected)
- ❌ No field selectors (can't filter by `.spec.nodeName`)
- ❌ No server-side filtering
- ❌ Watches work but are simplified
- ❌ No admission webhooks or controllers

**Setup Example**:
```go
import "k8s.io/client-go/kubernetes/fake"

// Create fake client with pre-populated objects
fakeClient := fake.NewSimpleClientset(
    &corev1.Pod{...},
    &corev1.Service{...},
)

// Use like normal client
pods, err := fakeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
```

**Perfect For**:
- Metrics collectors that just read Pod/Node data
- Status reporters that query resource status
- Simple integration tests for READ-only services

---

#### Option B: envtest (When You Need More) ⭐ Uses REAL Kubernetes Client!

**What It Is**:
- Real Kubernetes API server running in-process
- You use the **STANDARD Kubernetes client** (`k8s.io/client-go/kubernetes`) - SAME as production!
- The API server is just lightweight and runs in your test process
- From `sigs.k8s.io/controller-runtime/pkg/envtest`

**Key Point**:
```go
// envtest gives you a real rest.Config
cfg, _ := testEnv.Start()

// You use the SAME client constructor as production!
k8sClient, _ := kubernetes.NewForConfig(cfg)
// ↑ This is the standard K8s client - NOT a fake client!
```

**When to Use**:
- ✅ Need API server validation (reject invalid objects)
- ✅ Need field selectors (`.spec.nodeName=worker-1`)
- ✅ Need complex label selectors
- ✅ Need watches that behave exactly like K8s
- ✅ **Testing with CRDs** (register CRD definitions + use controller-runtime client)
- ✅ Need server-side filtering
- ✅ Want to use the SAME client code as production

**Limitations**:
- ❌ No RBAC (can't test ServiceAccount permissions)
- ❌ No real controllers (no kubelet, scheduler, etc.)
- ❌ No admission webhooks (unless you implement them)
- ⚠️ Slower startup (~2 seconds vs < 1 second)
- ⚠️ **Requires setup-envtest** to download API server binaries (kube-apiserver + etcd)

**Setup Example - Uses Standard K8s Client**:
```go
package myservice_test

import (
    "context"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    // Standard K8s client imports - SAME as production!
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"

    // envtest just provides the environment
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
    cfg       *rest.Config       // Real K8s config
    k8sClient kubernetes.Interface // Standard K8s client - NOT fake!
    testEnv   *envtest.Environment
)

var _ = BeforeSuite(func() {
    By("bootstrapping test environment")

    // envtest starts a real API server
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{"./config/crd"},
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).ToNot(HaveOccurred())

    // Use STANDARD Kubernetes client constructor
    // This is EXACTLY the same as: kubernetes.NewForConfig(clusterConfig)
    k8sClient, err = kubernetes.NewForConfig(cfg)
    Expect(err).ToNot(HaveOccurred())

    // The client works EXACTLY like talking to a real cluster!
})

var _ = Describe("Resource Monitor", func() {
    It("should list Pods with field selectors", func() {
        ctx := context.Background()

        // Create a Pod using standard client
        pod := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-pod",
                Namespace: "default",
            },
            Spec: corev1.PodSpec{
                NodeName: "worker-1",
                Containers: []corev1.Container{{
                    Name:  "nginx",
                    Image: "nginx",
                }},
            },
        }
        _, err := k8sClient.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
        Expect(err).ToNot(HaveOccurred())

        // Query with field selector - works because we have real API server!
        pods, err := k8sClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
            FieldSelector: "spec.nodeName=worker-1",
        })
        Expect(err).ToNot(HaveOccurred())
        Expect(pods.Items).To(HaveLen(1))
        Expect(pods.Items[0].Name).To(Equal("test-pod"))
    })
})
```

**Perfect For**:
- Controllers that watch resources
- Services that need field selectors
- **Testing with CRDs** (register definitions + use controller-runtime client)
- When you need "real K8s behavior" but not full cluster

---

#### **✅ CRD Support in envtest**

**Important**: envtest **fully supports CRDs**! You just need to:

1. **Register CRD definitions** (YAML files)
2. **Use controller-runtime client** (not just standard K8s client)

**Example with CRDs**:
```go
package mycontroller_test

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    // Import your CRD types
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
)

var (
    cfg       *rest.Config
    k8sClient client.Client    // controller-runtime client (supports CRDs!)
    testEnv   *envtest.Environment
)

var _ = BeforeSuite(func() {
    By("bootstrapping test environment")

    // Register your CRD scheme
    err := remediationv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    // Point to CRD YAML files
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "config", "crd", "bases"),
        },
        ErrorIfCRDPathMissing: true,
    }

    // Start API server with CRDs installed
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Use controller-runtime client (supports CRDs!)
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())
})

var _ = Describe("RemediationRequest Controller", func() {
    It("should create and watch RemediationRequest CRDs", func() {
        ctx := context.Background()

        // Create a CRD instance
        remediation := &remediationv1.RemediationRequest{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-remediation",
                Namespace: "default",
            },
            Spec: remediationv1.RemediationRequestSpec{
                AlertName: "HighMemoryUsage",
                Severity:  "critical",
            },
        }

        // Create using controller-runtime client
        err := k8sClient.Create(ctx, remediation)
        Expect(err).ToNot(HaveOccurred())

        // Fetch using controller-runtime client
        fetched := &remediationv1.RemediationRequest{}
        err = k8sClient.Get(ctx, client.ObjectKey{
            Name:      "test-remediation",
            Namespace: "default",
        }, fetched)
        Expect(err).ToNot(HaveOccurred())
        Expect(fetched.Spec.AlertName).To(Equal("HighMemoryUsage"))

        // You can also use standard K8s client for core resources
        pods, err := k8sClient.List(ctx, &corev1.PodList{})
        Expect(err).ToNot(HaveOccurred())
    })
})
```

**Key Points**:
- ✅ **CRD YAML files** must exist in `CRDDirectoryPaths`
- ✅ **Register scheme** with `YourCRDType.AddToScheme(scheme.Scheme)`
- ✅ **Use `client.Client`** from controller-runtime (not just `kubernetes.Interface`)
- ✅ **Works exactly like real cluster** - CRDs are validated by API server

---

#### **⚠️ IMPORTANT: envtest Requires setup-envtest**

Unlike Fake Client (which needs nothing), **envtest requires downloading API server binaries**:

**What setup-envtest Does**:
- Downloads `kube-apiserver` binary (~50MB)
- Downloads `etcd` binary (~20MB)
- Manages multiple Kubernetes versions (1.29, 1.30, 1.31, etc.)
- Provides path to binaries via `KUBEBUILDER_ASSETS` environment variable

**Installation**:
```bash
# Install the tool
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download Kubernetes 1.31 binaries
setup-envtest use 1.31.0 -p path

# Or via Makefile
make setup-envtest
```

**In Tests** (automatic with BeforeSuite):
```go
var _ = BeforeSuite(func() {
    By("bootstrapping test environment")

    // envtest will look for KUBEBUILDER_ASSETS environment variable
    // If not set, it will try to find binaries in default locations
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{"./config/crd"},
    }

    cfg, err = testEnv.Start() // This needs the binaries!
    Expect(err).ToNot(HaveOccurred())
})
```

**CI/CD Setup**:
```yaml
# .github/workflows/test.yml
- name: Setup envtest
  run: |
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    setup-envtest use 1.31.0 --bin-dir ./testbin -p path

- name: Run tests
  run: make test
  env:
    KUBEBUILDER_ASSETS: ${{ github.workspace }}/testbin/k8s/1.31.0-linux-amd64
```

**Key Differences from Fake Client**:

| Aspect | Fake Client | envtest |
|--------|-------------|---------|
| **Binary Download** | ❌ None | ✅ ~70MB binaries required |
| **Setup Tool** | ❌ Not needed | ✅ `setup-envtest` required |
| **CI Setup** | ✅ Instant | ⚠️ Download step needed |
| **Disk Space** | ✅ Minimal | ⚠️ ~70MB per K8s version |
| **Startup Time** | ✅ < 1 sec | ⚠️ ~2 sec (API server start) |

**Why This Matters**:
- 🔴 **CI/CD Impact**: CI jobs need to download binaries (adds ~10-30 seconds)
- 🔴 **Developer Setup**: New developers must run `make setup-envtest` before tests work
- 🔴 **Disk Space**: Each Kubernetes version takes ~70MB
- 🟢 **Caching**: Binaries can be cached in CI (saves time on subsequent runs)

**Recommendation**:
- If you DON'T need field selectors/watches → Use **Fake Client** (zero setup)
- If you DO need field selectors/watches → Use **envtest** (accept setup cost)

---

### 📊 Fake Client vs envtest Comparison

| Feature | Fake Client | envtest | Notes |
|---------|------------|---------|-------|
| **Prerequisites** | ❌ None | ⚠️ **setup-envtest** | envtest needs binary download |
| **Binary Size** | ❌ None | ⚠️ ~70MB (per K8s version) | kube-apiserver + etcd |
| **Setup Time** | < 1 second | ~2 seconds | Fake is faster |
| **API Server** | ❌ No (in-memory) | ✅ Yes (in-process) | envtest has real API server |
| **Client Type** | Fake client | **Real client** | envtest uses real K8s client |
| **Schema Validation** | ❌ No | ✅ Yes | envtest rejects invalid objects |
| **Field Selectors** | ❌ No | ✅ Yes | envtest: `spec.nodeName=worker` |
| **Label Selectors** | ✅ Basic | ✅ Full | Both support labels, envtest more accurate |
| **Watches** | ⚠️ Simplified | ✅ Real | envtest watches behave like K8s |
| **CRDs** | ❌ No | ✅ **Full Support** | Register CRDs + use controller-runtime client |
| **Pre-populate Data** | ✅ Easy | ⚠️ Must create via API | Fake: pass objects to constructor |
| **Test Complexity** | Low | Medium | Fake is simpler |
| **Realism** | Medium | High | envtest closer to real K8s |

---

### 💡 Recommendation: Start with Fake Client

**Default Choice**: Start with **Fake Client** unless you have a specific reason to use envtest.

**Why**:
- ✅ Simpler setup (2 lines of code)
- ✅ Faster tests (< 1 second)
- ✅ Easier to pre-populate test data
- ✅ Good enough for 80% of READ-only scenarios

**Upgrade to envtest when**:
- ❌ Tests fail due to missing field selectors
- ❌ Need to test with CRDs
- ❌ Need exact watch behavior
- ❌ Need API server validation in tests

### Examples

#### Metrics Collector Service 🧪 FAKE CLIENT
**Why**: Only reads Pod metrics and node status
```yaml
Dependencies:
  - Kubernetes fake client (in-memory)

Integration Tests:
  - List Pods across namespaces
  - Get Node resource usage
  - Query Service endpoints
  - Calculate cluster metrics

NO resource creation/modification
```

**Test Setup**:
```go
package metrics_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/metrics"
)

func TestMetricsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Integration Suite (Fake K8s Client)")
}

var (
	fakeClient *fake.Clientset
	collector  *metrics.Collector
)

var _ = BeforeSuite(func() {
	// Create fake Kubernetes client with test data
	fakeClient = fake.NewSimpleClientset(
		// Pre-populate with test Pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod-1",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod-2",
				Namespace: "kube-system",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Pre-populate with test Nodes
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
	)

	// Initialize collector with fake client
	collector = metrics.NewCollector(fakeClient)
})

var _ = Describe("BR-METRICS-001: Pod Metrics Collection", func() {
	It("should list all Pods across namespaces", func() {
		ctx := context.Background()

		// Collector reads from fake client
		pods, err := collector.ListAllPods(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(pods).To(HaveLen(2))
		Expect(pods[0].Name).To(Equal("test-pod-1"))
		Expect(pods[1].Name).To(Equal("test-pod-2"))
	})

	It("should calculate cluster resource usage", func() {
		ctx := context.Background()

		// Collector reads node metrics
		usage, err := collector.GetClusterResourceUsage(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(usage.TotalCPU).To(Equal(int64(4)))
		Expect(usage.TotalMemory).To(Equal(int64(8 * 1024 * 1024 * 1024)))
	})
})
```

**Benefits**:
- Setup: **< 1 second** (vs 30-60s for Kind)
- No external dependencies
- Full control over test data
- Tests real client-go behavior

#### Status Reporter Service 🧪 FAKE CLIENT
**Why**: Only reads resource status, doesn't modify
```yaml
Dependencies:
  - Kubernetes fake client (in-memory)

Integration Tests:
  - Get Deployment status
  - Get Pod readiness
  - Get Service endpoints
  - Report cluster health

NO resource creation/modification
```

#### Resource Monitor 🧪 ENVTEST (if needing API server features)
**Why**: Needs API server validation but no real cluster
```yaml
Dependencies:
  - envtest (in-memory API server)

Integration Tests:
  - List resources with field selectors
  - Watch resource changes
  - Test API server validation
  - Test admission controllers (if any)

NO resource creation, just reads + watches
```

**envtest Setup** (for more complex scenarios):
```go
package monitor_test

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/pkg/monitor"
)

func TestMonitorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Monitor Integration Suite (envtest)")
}

var (
	cfg       *rest.Config
	k8sClient kubernetes.Interface
	testEnv   *envtest.Environment
	monitor   *monitor.ResourceMonitor
)

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	k8sClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())

	monitor = monitor.NewResourceMonitor(k8sClient)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("BR-MONITOR-001: Resource Watching", func() {
	It("should watch Pod status changes", func() {
		ctx := context.Background()

		// Create a Pod (for test data)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "watch-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "test", Image: "nginx"}},
			},
		}
		_, err := k8sClient.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Monitor watches (doesn't modify)
		events, err := monitor.WatchPodStatus(ctx, "default", "watch-pod")
		Expect(err).ToNot(HaveOccurred())
		Expect(events).ToNot(BeNil())
	})
})
```

---

## ⚙️ Use KIND When...

### Criteria
Your service meets **ANY** of these:
1. ⚙️ Creates/modifies Kubernetes resources (Pods, Services, ConfigMaps, Secrets)
2. ⚙️ Uses Custom Resource Definitions (CRDs)
3. ⚙️ Watches Kubernetes resources
4. ⚙️ Requires RBAC/ServiceAccount authentication with real TokenReview
5. ⚙️ Implements admission webhooks (ValidatingWebhookConfiguration, MutatingWebhookConfiguration)

### Examples

#### Remediation Request Controller ⚙️ KIND
**Why**: CRD controller that watches and updates RemediationRequest CRDs
```yaml
Kubernetes Dependencies:
  - RemediationRequest CRD (watch, update status)
  - Pod creation/deletion
  - ConfigMap reads
  - RBAC permissions

Integration Tests MUST:
  - Create RemediationRequest CRs
  - Watch for status updates
  - Verify Pod creation
  - Test RBAC restrictions
```

#### Dynamic Toolset Service ⚙️ KIND
**Why**: Creates ConfigMaps dynamically based on detected services
```yaml
Kubernetes Dependencies:
  - Service discovery (list Services)
  - ConfigMap creation (dynamic toolsets)
  - Prometheus/Grafana services in cluster

Integration Tests MUST:
  - Deploy Prometheus/Grafana to Kind
  - Verify Service detection
  - Verify ConfigMap creation
  - Test updates on Service changes
```

#### Workflow Engine Controller ⚙️ KIND
**Why**: Creates/manages workflow Pods, watches their status
```yaml
Kubernetes Dependencies:
  - Workflow CRD (watch, update)
  - Pod creation (workflow steps)
  - Pod status watching
  - ConfigMap/Secret mounting

Integration Tests MUST:
  - Create Workflow CRs
  - Verify Pod creation
  - Watch Pod completion
  - Update Workflow status
```

---

## 🏗️ Implementation Patterns

### Pattern 1: Podman Integration Tests

**Setup (5-10 lines)**:
```go
package myservice_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "My Service Integration Suite (Podman)")
}

var (
	postgresContainer testcontainers.Container
	redisContainer    testcontainers.Container
	dbURL            string
	redisAddr        string
)

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresReq := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections"),
	}
	var err error
	postgresContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	Expect(err).ToNot(HaveOccurred())

	// Get connection details
	host, _ := postgresContainer.Host(ctx)
	port, _ := postgresContainer.MappedPort(ctx, "5432")
	dbURL = fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	// Start Redis container (similar pattern)
	// ...

	GinkgoWriter.Println("✅ Podman integration test environment ready!")
})

var _ = AfterSuite(func() {
	ctx := context.Background()
	if postgresContainer != nil {
		_ = postgresContainer.Terminate(ctx)
	}
	if redisContainer != nil {
		_ = redisContainer.Terminate(ctx)
	}
})
```

**Benefits**:
- ✅ **Fast**: 2-5 seconds startup
- ✅ **Isolated**: Each test run gets fresh containers
- ✅ **CI-friendly**: Works in CI without Kind cluster
- ✅ **Simple**: No Kubernetes complexity

---

### Pattern 2: Kind Cluster Integration Tests

**Setup (15 lines using template)**:
```go
package myservice_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/myservice"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "My Service Integration Suite (Kind)")
}

var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
	// Use Kind template for K8s-dependent services
	suite = kind.Setup("myservice-test", "kubernaut-system")

	// Deploy any required K8s resources
	// suite.DeployPrometheusService("myservice-test")

	GinkgoWriter.Println("✅ Kind integration test environment ready!")
})

var _ = AfterSuite(func() {
	suite.Cleanup()
})
```

**When Required**:
- ⚙️ Service creates/modifies K8s resources
- ⚙️ Service uses CRDs
- ⚙️ Service requires RBAC/ServiceAccount auth

---

## 🎯 Decision Matrix

| Service Characteristic | Podman | Fake Client / envtest | Kind | Justification |
|-----------------------|--------|----------------------|------|---------------|
| **Uses PostgreSQL/MySQL** | ✅ | ❌ | ⚠️ | Podman sufficient unless K8s PVC/StatefulSet testing needed |
| **Uses Redis/Memcached** | ✅ | ❌ | ⚠️ | Podman sufficient unless K8s Service discovery needed |
| **Uses message queues (RabbitMQ, Kafka)** | ✅ | ❌ | ⚠️ | Podman sufficient unless K8s Operators involved |
| **Uses AI/LLM APIs** | ✅ | ❌ | ❌ | Podman + mock HTTP server sufficient |
| **Reads K8s resources (Pods, Services)** | ❌ | ✅ | ⚠️ | Fake client best for READ-only, Kind if need real cluster state |
| **Creates K8s resources** | ❌ | ⚠️ | ✅ | Kind required for real RBAC, fake client for simple creates |
| **Uses CRDs** | ❌ | ⚠️ | ✅ | Kind required for CRD registration, envtest for simple cases |
| **Watches K8s resources** | ❌ | ✅ | ✅ | Fake client for simple watches, Kind for real watch streams |
| **Requires ServiceAccount auth** | ❌ | ❌ | ✅ | Kind required for testing real TokenReview |
| **Implements webhooks** | ❌ | ⚠️ | ✅ | Kind required for admission controller, envtest for validation only |

**Legend**:
- ✅ Recommended
- ⚠️ Depends on test scope
- ❌ Not suitable

---

## 📋 Service-by-Service Recommendations

### Implemented Services

| Service | Environment | Rationale | Change Needed? |
|---------|------------|-----------|----------------|
| **Gateway** | ~~Kind~~ → **Podman** | Only uses Redis, no K8s API calls | ✅ **Migrate to Podman** |
| **Dynamic Toolset** | **Kind** | Creates ConfigMaps, discovers Services | ❌ Keep Kind |
| **Data Storage** | ~~Kind~~ → **Podman** | Only uses PostgreSQL/VectorDB/Redis, no K8s API calls | ✅ **Migrate to Podman** |

### Planned Services

| Service | Recommended Environment | Rationale |
|---------|------------------------|-----------|
| **Context Optimization** | **Podman** | AI APIs + PostgreSQL + Redis, no K8s |
| **Pattern Recognition** | **Podman** | ML models + PostgreSQL + Redis, no K8s |
| **Multi-Cluster Manager** | **Kind** | Manages multiple K8s clusters, CRDs |
| **Remediation Request Controller** | **Kind** | CRD controller, creates Pods |
| **Workflow Engine Controller** | **Kind** | CRD controller, manages workflow Pods |
| **Action Executor Controller** | **Kind** | CRD controller, executes K8s actions |
| **Safety Validator** | **Kind** | Admission webhook, validates K8s requests |

---

## 🔄 Migration Path (Gateway and Data Storage)

### Current State (Inefficient)
- Gateway: Kind cluster for Redis integration tests (30-60s startup)
- Data Storage: Kind cluster for PostgreSQL integration tests (30-60s startup)

### Target State (Efficient)
- Gateway: Podman + Redis container (2-5s startup)
- Data Storage: Podman + PostgreSQL + VectorDB containers (2-5s startup)

### Migration Steps

1. **Create Podman-based test suite** (2-3 hours per service)
   - Add testcontainers-go dependency
   - Create container request configs
   - Update BeforeSuite/AfterSuite

2. **Validate test coverage maintained** (1 hour per service)
   - All integration tests passing
   - Same scenarios covered
   - Performance validated

3. **Update documentation** (30 min per service)
   - Update testing-strategy.md
   - Update README.md prerequisites
   - Update CI/CD pipelines

4. **Archive Kind-based tests** (15 min per service)
   - Move to archived/ directory
   - Document migration reason

---

## 🚀 Performance Impact

### Gateway Service (Example)

**Before (Kind)**:
```
Integration test startup: 45 seconds
Test execution: 12 seconds
Total: 57 seconds
```

**After (Podman)**:
```
Integration test startup: 3 seconds
Test execution: 12 seconds
Total: 15 seconds
```

**Improvement**: **74% faster** (42 seconds saved per test run)

**Developer Impact**:
- 10 test runs/day = 7 minutes saved/day
- 50 test runs/week = 35 minutes saved/week
- **~2.5 hours saved per developer per month**

---

## 📖 Template Integration

### Template Checklist (Day 1)

```markdown
## Prerequisites Checklist

Before starting Day 1, ensure:
- [ ] Service specifications complete
- [ ] Business requirements documented
- [ ] Architecture decisions approved
- [ ] Dependencies identified
- [ ] **Integration test environment decided** ⭐ NEW

### Integration Test Environment Decision

**Decision Tree** (see [INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md]()):

1. Does your service interact with Kubernetes APIs (create/modify resources)?
   - ✅ YES → Use Kind cluster (see [KIND_CLUSTER_TEST_TEMPLATE.md]())
   - ❌ NO → Continue to #2

2. Does your service use CRDs or watch Kubernetes resources?
   - ✅ YES → Use Kind cluster
   - ❌ NO → Continue to #3

3. Does your service require RBAC/ServiceAccount authentication?
   - ✅ YES → Use Kind cluster
   - ❌ NO → Use Podman/Testcontainers (see [PODMAN_INTEGRATION_TEST_TEMPLATE.md]())

**Your Decision**: [ Kind / Podman ] (document rationale below)

**Rationale**:
[Why this environment is appropriate for this service's dependencies]
```

---

## 🎓 Best Practices

### DO ✅
1. **Use Podman for stateless services** with only external dependencies
2. **Use Kind for services** that interact with Kubernetes APIs
3. **Measure startup time** - if > 10 seconds, question if Kind is needed
4. **Mock K8s client** if service only reads K8s data (no writes)
5. **Document environment choice** in service testing-strategy.md

### DON'T ❌
1. **Don't use Kind by default** - evaluate if Podman is sufficient
2. **Don't mix environments** - pick one per service
3. **Don't skip environment decision** - make explicit choice early
4. **Don't over-engineer** - simplest environment that validates dependencies
5. **Don't ignore CI/CD impact** - Podman is faster in pipelines

---

## 📊 Summary

**Principle**: **Right tool for the right job**

---

## 📊 Complete Comparison Table

| Feature | Fake Client | envtest | Podman | Kind |
|---------|------------|---------|--------|------|
| **Startup Time** | < 1 second | ~2 seconds | 2-5 seconds | 30-60 seconds |
| **What It Provides** | In-memory K8s objects | In-process API server | Real containers | Full K8s cluster |
| **Use For** | Simple K8s reads | K8s reads + validation | External deps (DB, Redis) | K8s writes, CRDs, RBAC |
| **API Server** | ❌ No | ✅ Yes (in-process) | ❌ No | ✅ Yes (full) |
| **Watches** | ⚠️ Limited | ✅ Yes | ❌ No | ✅ Yes |
| **CRDs** | ❌ No | ⚠️ Simple only | ❌ No | ✅ Yes (full) |
| **RBAC** | ❌ No | ❌ No | ❌ No | ✅ Yes |
| **Webhooks** | ❌ No | ⚠️ Validation only | ❌ No | ✅ Yes (full) |
| **PostgreSQL/Redis** | ❌ No | ❌ No | ✅ Yes | ⚠️ Can deploy |
| **CI/CD Speed** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ |
| **Complexity** | Low | Medium | Low | High |
| **Maintenance** | Low | Low | Low | Medium |

### Quick Decision Guide

```
Does your service WRITE to Kubernetes (create/modify resources)?
├─ YES → Use KIND (need real API server + RBAC)
└─ NO → Does it READ from Kubernetes?
         ├─ YES → Need API server validation/watches?
         │        ├─ YES → Use ENVTEST (in-process API server)
         │        └─ NO → Use FAKE CLIENT (simplest)
         └─ NO → Use PODMAN (external dependencies only)
```

---

**Four Options** (from simplest to most complex):
1. **Fake Client** (< 1s): In-memory K8s objects, simple reads
2. **envtest** (~2s): In-process K8s API server, reads + validation
3. **Podman** (2-5s): Real containers for databases/Redis, no K8s
4. **Kind** (30-60s): Full K8s cluster for writes/CRDs/RBAC

**Expected Impact**:
- 2-3 services migrate to Podman (Gateway, Data Storage, Context Optimization)
- Services that READ K8s can use Fake Client or envtest (much faster than Kind)
- 50-75% faster integration test feedback loop
- Simpler CI/CD pipelines for stateless services
- Kind reserved for services that truly need full Kubernetes (writes, CRDs, RBAC)

---

## Related Documents

- [ADR-003-KIND-INTEGRATION-ENVIRONMENT.md](../architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md) - Kind as primary K8s environment
- [KIND_CLUSTER_TEST_TEMPLATE.md](./KIND_CLUSTER_TEST_TEMPLATE.md) - Kind test template
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) - Overall testing strategy
- [INTEGRATION_E2E_NO_MOCKS_POLICY.md](./INTEGRATION_E2E_NO_MOCKS_POLICY.md) - No mocks policy

---

**Status**: ✅ APPROVED
**Date**: 2025-10-12
**Authority**: Architecture Team
**Implementation**: Required for all future services, optional migration for existing

