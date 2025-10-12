# Dynamic Toolset Service - Test Setup Assessment

**Version**: v1.0
**Created**: October 10, 2025
**Status**: ⏸️ Pre-Implementation

---

## Test Environment Requirements

### Infrastructure Needs

#### Kind Cluster
**Purpose**: Integration and E2E testing
**Configuration**:
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: kubernaut-toolset-test
nodes:
- role: control-plane
- role: worker
```

**Setup**:
```bash
# Create Kind cluster for Dynamic Toolset testing
kind create cluster --name kubernaut-toolset-test --config config/kind/toolset-test-cluster.yaml

# Install test services (Prometheus, Grafana)
kubectl apply -f test/integration/toolset/testdata/mock-services.yaml --context kind-kubernaut-toolset-test
```

#### Mock Services
**Purpose**: Simulate Prometheus, Grafana, Jaeger services for discovery testing
**Required Services**:
1. **Prometheus**: Service with `app=prometheus` label, port 9090
2. **Grafana**: Service with `app=grafana` label, port 3000
3. **Jaeger**: Service with `app=jaeger` label, port 16686
4. **Elasticsearch**: Service with `app=elasticsearch` label, port 9200

**Test Data File**: `test/integration/toolset/testdata/mock-services.yaml`

#### No External Dependencies
**Advantage**: Dynamic Toolset Service has no Redis or external dependencies
**Result**: Simpler test setup than Gateway

---

## Test Framework

### Unit Tests
**Framework**: Ginkgo + Gomega
**Coverage Target**: >70%

**Setup**:
```go
// test/unit/toolset/suite_test.go
package toolset_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestToolset(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Dynamic Toolset Unit Tests")
}
```

**Run**:
```bash
go test ./test/unit/toolset/... -v
```

### Integration Tests
**Framework**: Ginkgo + Gomega + Kind cluster
**Coverage Target**: >50%

**Setup**:
```go
// test/integration/toolset/suite_test.go
package toolset_test

import (
    "context"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

var k8sClient *kubernetes.Clientset

var _ = BeforeSuite(func() {
    // Load Kind cluster kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("",
        os.Getenv("KUBECONFIG"))
    Expect(err).ToNot(HaveOccurred())

    // Create Kubernetes client
    k8sClient, err = kubernetes.NewForConfig(config)
    Expect(err).ToNot(HaveOccurred())

    // Deploy mock services
    deployMockServices()
})

func TestIntegrationToolset(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Dynamic Toolset Integration Tests")
}
```

**Run**:
```bash
export KUBECONFIG=$(kind get kubeconfig --name kubernaut-toolset-test)
go test ./test/integration/toolset/... -v
```

### E2E Tests
**Framework**: Ginkgo + Gomega + Kind cluster + HolmesGPT API
**Coverage Target**: <10%

**Setup**:
```go
// test/e2e/toolset/suite_test.go
package toolset_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
    // Deploy Dynamic Toolset Service
    deployDynamicToolsetService()

    // Deploy HolmesGPT API (consumer of toolsets)
    deployHolmesGPTAPI()

    // Wait for services to be ready
    waitForServicesReady()
})

func TestE2EToolset(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Dynamic Toolset E2E Tests")
}
```

**Run**:
```bash
make test-e2e-toolset
```

---

## Test Data

### Mock Kubernetes Services

**File**: `test/integration/toolset/testdata/mock-services.yaml`

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: monitoring
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus-server
  namespace: monitoring
  labels:
    app: prometheus
spec:
  type: ClusterIP
  ports:
  - name: web
    port: 9090
    targetPort: 9090
  selector:
    app: prometheus
---
apiVersion: v1
kind: Deployment
metadata:
  name: prometheus-server
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:v2.40.0
        ports:
        - containerPort: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: monitoring
  labels:
    app: grafana
spec:
  type: ClusterIP
  ports:
  - name: service
    port: 3000
    targetPort: 3000
  selector:
    app: grafana
---
# Similar deployments for Jaeger and Elasticsearch
```

### Mock Health Check Endpoints

**Purpose**: Ensure mock services respond to health checks

**Implementation**:
- Prometheus: `GET /prometheus/-/healthy` → 200 OK
- Grafana: `GET /grafana/api/health` → 200 OK
- Jaeger: `GET /jaeger/` → 200 OK
- Elasticsearch: `GET /elasticsearch/` → 200 OK

---

## RBAC Requirements

### ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dynamic-toolset-test
  namespace: kubernaut-system
```

### ClusterRole

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dynamic-toolset-test
rules:
# Service discovery
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch"]

# ConfigMap management
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Health checks (optional)
- apiGroups: [""]
  resources: ["pods", "endpoints"]
  verbs: ["get", "list"]
```

### ClusterRoleBinding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dynamic-toolset-test
subjects:
- kind: ServiceAccount
  name: dynamic-toolset-test
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: dynamic-toolset-test
  apiGroup: rbac.authorization.k8s.io
```

---

## Test Execution Strategy

### Unit Tests
**When**: Every commit
**CI**: GitHub Actions
**Duration**: < 2 minutes

```bash
# Fast unit tests
go test ./test/unit/toolset/... -v -cover
```

### Integration Tests
**When**: Before merge to main
**CI**: GitHub Actions (with Kind cluster)
**Duration**: < 5 minutes

```bash
# Setup Kind cluster
kind create cluster --name kubernaut-toolset-test

# Run integration tests
export KUBECONFIG=$(kind get kubeconfig --name kubernaut-toolset-test)
go test ./test/integration/toolset/... -v

# Cleanup
kind delete cluster --name kubernaut-toolset-test
```

### E2E Tests
**When**: Pre-release validation
**CI**: Manual or nightly
**Duration**: < 10 minutes

```bash
# Full E2E test suite
make test-e2e-toolset
```

---

## Test Coverage Goals

| Test Type | Coverage Target | Focus |
|-----------|----------------|-------|
| **Unit** | >70% | Service detectors, ConfigMap generation, business logic |
| **Integration** | >50% | Kubernetes service discovery, ConfigMap reconciliation |
| **E2E** | <10% | Complete discovery flow, HolmesGPT API integration |

---

## Test Automation

### Makefile Targets

```makefile
# Dynamic Toolset test targets
.PHONY: test-unit-toolset
test-unit-toolset:
	go test ./test/unit/toolset/... -v -cover

.PHONY: test-integration-toolset
test-integration-toolset:
	# Setup Kind cluster
	kind create cluster --name kubernaut-toolset-test || true
	# Deploy mock services
	kubectl apply -f test/integration/toolset/testdata/mock-services.yaml --context kind-kubernaut-toolset-test
	# Wait for services
	kubectl wait --for=condition=ready pod -l app=prometheus -n monitoring --timeout=60s --context kind-kubernaut-toolset-test
	# Run tests
	export KUBECONFIG=$$(kind get kubeconfig --name kubernaut-toolset-test) && \
	go test ./test/integration/toolset/... -v

.PHONY: test-e2e-toolset
test-e2e-toolset:
	# Deploy Dynamic Toolset + HolmesGPT API
	kubectl apply -f deploy/dynamic-toolset/ --context kind-kubernaut-toolset-test
	kubectl wait --for=condition=ready pod -l app=dynamic-toolset -n kubernaut-system --timeout=120s --context kind-kubernaut-toolset-test
	# Run E2E tests
	go test ./test/e2e/toolset/... -v

.PHONY: test-toolset
test-toolset: test-unit-toolset test-integration-toolset

.PHONY: cleanup-toolset-test
cleanup-toolset-test:
	kind delete cluster --name kubernaut-toolset-test
```

---

## Confidence Assessment

**Test Setup Confidence**: 95% (Very High)

**Rationale**:
- Simpler than Gateway (no Redis, no CRDs to install)
- Kind cluster setup is well-established
- Mock services are straightforward to deploy
- RBAC requirements are minimal

**Risk Factors**:
- Mock services may not perfectly match real Prometheus/Grafana behavior
- Health check endpoints may vary across versions

**Mitigation**:
- Document exact mock service versions
- Add version compatibility tests

---

**Document Status**: ✅ Test Setup Assessment Complete
**Last Updated**: October 10, 2025
**Next Step**: Create BR test strategy document

