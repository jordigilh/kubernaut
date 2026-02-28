# Approved Integration Test Architecture - Hybrid Envtest/Kind

**Date**: 2025-10-14
**Status**: ✅ **APPROVED** - Hybrid Envtest/Kind Approach
**Decision**: Option 1 from ENVTEST_VS_KIND_ASSESSMENT.md
**Overall Confidence**: **96%**

---

## Executive Summary

**Approved Architecture**: **Hybrid Envtest/Kind + Podman**

```
┌─────────────────────────────────────────────────────────────┐
│             Integration Test Infrastructure                  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Service 1: Remediation Processor                           │
│  ┌────────────────────────────────────────┐                 │
│  │  Envtest (Kubernetes API)              │                 │
│  │  + Podman (PostgreSQL + Redis)         │                 │
│  │  Confidence: 98%                       │                 │
│  └────────────────────────────────────────┘                 │
│                                                              │
│  Service 2: Workflow Execution                              │
│  ┌────────────────────────────────────────┐                 │
│  │  Envtest (Kubernetes API)              │                 │
│  │  No external dependencies              │                 │
│  │  Confidence: 95%                       │                 │
│  └────────────────────────────────────────┘                 │
│                                                              │
│  Service 3: Kubernetes Executor (DEPRECATED - ADR-025)      │
│  ┌────────────────────────────────────────┐                 │
│  │  Kind (Full Kubernetes)                │                 │
│  │  Real Job execution                    │                 │
│  │  Confidence: 95%                       │                 │
│  └────────────────────────────────────────┘                 │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Overall Confidence**: **96%** (weighted average)
**Speed Improvement**: **1.5x faster** than all Kind
**Additional Setup Effort**: **+2 hours**

---

## Approved Architecture Details

### Service 1: Remediation Processor

**Infrastructure**: **Envtest + Podman**
**Confidence**: **98%** ✅

**Components**:
- **Envtest**: Lightweight Kubernetes API server for CRD management
- **Podman - PostgreSQL**: Port 5433, pgvector extension
- **Podman - Redis**: Port 6380

**Rationale**:
- Only manages CRDs (no Job execution needed)
- Enriches data via PostgreSQL queries
- Fast iteration (Envtest startup: 1-2s vs Kind: 30-60s)

**Make Target**:
```makefile
test-integration-remediationprocessor: bootstrap-envtest-podman-remediationprocessor
	ENVTEST=1 go test -v -timeout 5m \
		-tags=integration \
		./test/integration/remediationprocessing/...
```

---

### Service 2: Workflow Execution

**Infrastructure**: **Envtest only**
**Confidence**: **95%** ✅

**Components**:
- **Envtest**: Kubernetes API server for CRD orchestration
- **No external dependencies**

**Rationale**:
- Orchestrates child CRDs (doesn't execute Jobs)
- Tests watch-based coordination
- Very fast (no external dependencies to start)

**Make Target**:
```makefile
test-integration-workflowexecution: install-envtest
	ENVTEST=1 go test -v -timeout 5m \
		-tags=integration \
		./test/integration/workflowexecution/...
```

---

### Service 3: Kubernetes Executor (DEPRECATED - ADR-025)

**Infrastructure**: **Kind cluster**
**Confidence**: **95%** ✅

**Components**:
- **Kind**: Full Kubernetes cluster with scheduler + kubelet
- **Real Job execution**: Jobs actually run and complete
- **Test fixtures**: Deployments, Pods for testing

**Rationale**:
- **CRITICAL**: Needs to test actual Job execution
- Captures rollback info from Job logs
- Validates Rego policies with real Job behavior

**Make Target**:
```makefile
test-integration-kubernetesexecutor: bootstrap-integration-env-kubernetesexecutor
	go test -v -timeout 10m \
		-tags=integration \
		./test/integration/kubernetesexecution/...
```

---

## Complete Makefile Targets

```makefile
# ==========================================
# Integration Test Infrastructure
# ==========================================

# Install Envtest binaries (one-time setup)
.PHONY: install-envtest
install-envtest:
	@echo "Installing envtest binaries..."
	@which setup-envtest > /dev/null || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	@setup-envtest use -p path
	@echo "✅ Envtest binaries installed"

# ==========================================
# Service 1: Remediation Processor (Envtest + Podman)
# ==========================================

.PHONY: test-unit-remediationprocessor
test-unit-remediationprocessor:
	@echo "Running unit tests for Remediation Processor..."
	go test -v -race -coverprofile=coverage-unit-remediation.out \
		./pkg/remediationprocessing/... \
		./test/unit/remediationprocessing/...

.PHONY: test-integration-remediationprocessor
test-integration-remediationprocessor: bootstrap-envtest-podman-remediationprocessor
	@echo "Running integration tests for Remediation Processor (Envtest + Podman)..."
	ENVTEST=1 go test -v -race -timeout 5m \
		-tags=integration \
		-coverprofile=coverage-integration-remediation.out \
		./test/integration/remediationprocessing/...
	$(MAKE) cleanup-envtest-podman-remediationprocessor

.PHONY: bootstrap-envtest-podman-remediationprocessor
bootstrap-envtest-podman-remediationprocessor: check-podman install-envtest
	@echo "Setting up Envtest + Podman for Remediation Processor..."

	@# Start PostgreSQL with pgvector
	@echo "Starting PostgreSQL..."
	@podman rm -f test-postgres-remediation 2>/dev/null || true
	@podman run -d \
		--name test-postgres-remediation \
		-e POSTGRES_USER=remediation_user \
		-e POSTGRES_PASSWORD=remediation_test \
		-e POSTGRES_DB=remediation_test \
		-p 5433:5432 \
		pgvector/pgvector:pg16

	@# Start Redis
	@echo "Starting Redis..."
	@podman rm -f test-redis-remediation 2>/dev/null || true
	@podman run -d \
		--name test-redis-remediation \
		-p 6380:6379 \
		redis:7-alpine

	@# Wait for services
	@echo "Waiting for services to be ready..."
	@timeout 30 bash -c 'until podman exec test-postgres-remediation pg_isready; do sleep 1; done'
	@timeout 30 bash -c 'until podman exec test-redis-remediation redis-cli ping | grep -q PONG; do sleep 1; done'

	@# Initialize database schema
	@echo "Initializing remediation_audit schema..."
	@podman exec test-postgres-remediation psql -U remediation_user -d remediation_test -c "CREATE EXTENSION IF NOT EXISTS vector;"
	@cat db/migrations/remediation_audit_schema.sql | podman exec -i test-postgres-remediation psql -U remediation_user -d remediation_test

	@echo "✅ Envtest + Podman environment ready for Remediation Processor"

.PHONY: cleanup-envtest-podman-remediationprocessor
cleanup-envtest-podman-remediationprocessor:
	@echo "Cleaning up Envtest + Podman environment..."
	@podman rm -f test-postgres-remediation test-redis-remediation 2>/dev/null || true
	@echo "✅ Cleanup complete"

# ==========================================
# Service 2: Workflow Execution (Envtest only)
# ==========================================

.PHONY: test-unit-workflowexecution
test-unit-workflowexecution:
	@echo "Running unit tests for Workflow Execution..."
	go test -v -race -coverprofile=coverage-unit-workflow.out \
		./pkg/workflowexecution/... \
		./test/unit/workflowexecution/...

.PHONY: test-integration-workflowexecution
test-integration-workflowexecution: install-envtest
	@echo "Running integration tests for Workflow Execution (Envtest)..."
	ENVTEST=1 go test -v -race -timeout 5m \
		-tags=integration \
		-coverprofile=coverage-integration-workflow.out \
		./test/integration/workflowexecution/...

# No bootstrap/cleanup needed - Envtest handles CRDs in test setup

# ==========================================
# Service 3: Kubernetes Executor (Kind) (DEPRECATED - ADR-025)
# ==========================================

.PHONY: test-unit-kubernetesexecutor
test-unit-kubernetesexecutor:
	@echo "Running unit tests for Kubernetes Executor..."
	go test -v -race -coverprofile=coverage-unit-executor.out \
		./pkg/kubernetesexecution/... \
		./test/unit/kubernetesexecution/...

.PHONY: test-integration-kubernetesexecutor
test-integration-kubernetesexecutor: bootstrap-integration-env-kubernetesexecutor
	@echo "Running integration tests for Kubernetes Executor (Kind)..."
	go test -v -race -timeout 10m \
		-tags=integration \
		-coverprofile=coverage-integration-executor.out \
		./test/integration/kubernetesexecution/...
	$(MAKE) cleanup-integration-env-kubernetesexecutor

.PHONY: bootstrap-integration-env-kubernetesexecutor
bootstrap-integration-env-kubernetesexecutor:
	@echo "Setting up Kind cluster for Kubernetes Executor..."
	@if ! kind get clusters | grep -q kubernaut-test; then \
		$(MAKE) create-test-cluster; \
	fi
	@kubectl apply -f api/kubernetesexecution/v1alpha1/kubernetesexecution_crd.yaml
	@kubectl create namespace test-executor || true
	@kubectl apply -f test/integration/kubernetesexecution/fixtures/test-resources.yaml
	@kubectl create configmap rego-policies \
		--from-file=pkg/kubernetesexecution/policies/ \
		-n test-executor \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "✅ Kind environment ready for Kubernetes Executor"

.PHONY: cleanup-integration-env-kubernetesexecutor
cleanup-integration-env-kubernetesexecutor:
	@echo "Cleaning up Kind environment..."
	@kubectl delete namespace test-executor --ignore-not-found=true
	@echo "✅ Cleanup complete"

# ==========================================
# Shared Infrastructure
# ==========================================

.PHONY: create-test-cluster
create-test-cluster:
	@echo "Creating Kind test cluster..."
	@kind create cluster --name kubernaut-test --config hack/kind-config.yaml
	@kubectl wait --for=condition=Ready nodes --all --timeout=60s
	@echo "✅ Kind cluster ready"

.PHONY: delete-test-cluster
delete-test-cluster:
	@echo "Deleting Kind test cluster..."
	@kind delete cluster --name kubernaut-test

.PHONY: check-podman
check-podman:
	@which podman > /dev/null || { \
		echo "❌ Podman not found. Please install:"; \
		echo "  macOS: brew install podman && podman machine init && podman machine start"; \
		echo "  Linux: sudo apt-get install podman"; \
		exit 1; \
	}

.PHONY: force-cleanup-all-test-containers
force-cleanup-all-test-containers:
	@echo "Force cleanup of all test containers..."
	@podman ps -a --filter name=test- -q | xargs -r podman rm -f 2>/dev/null || true
	@echo "✅ All test containers removed"

# ==========================================
# Aggregate Targets
# ==========================================

.PHONY: test-unit-all-phase3
test-unit-all-phase3: test-unit-remediationprocessor test-unit-workflowexecution test-unit-kubernetesexecutor

.PHONY: test-integration-all-phase3
test-integration-all-phase3: test-integration-remediationprocessor test-integration-workflowexecution test-integration-kubernetesexecutor

.PHONY: test-all-phase3
test-all-phase3: test-unit-all-phase3 test-integration-all-phase3
```

---

## Test Suite Structure

### Remediation Processor: Envtest Pattern

**File**: `test/integration/remediationprocessing/suite_test.go`

```go
package remediationprocessing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestRemediationProcessingIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Remediation Processing Integration Suite (Envtest)")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping Envtest environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "api", "remediation", "v1alpha1"),
			filepath.Join("..", "..", "..", "api", "remediationprocessing", "v1alpha1"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register schemes
	err = remediationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = remediationprocessingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create Kubernetes client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Podman containers (PostgreSQL + Redis) started by Makefile
	// Environment variables set:
	// - DB_HOST=localhost
	// - DB_PORT=5433
	// - REDIS_HOST=localhost
	// - REDIS_PORT=6380
})

var _ = AfterSuite(func() {
	cancel()

	By("tearing down the Envtest environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	// Podman cleanup handled by Makefile
})
```

---

### Workflow Execution: Envtest Pattern

**File**: `test/integration/workflowexecution/suite_test.go`

```go
package workflowexecution

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1" // DEPRECATED - ADR-025
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestWorkflowExecutionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Execution Integration Suite (Envtest)")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping Envtest environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "api", "workflowexecution", "v1alpha1"),
			filepath.Join("..", "..", "..", "api", "kubernetesexecution", "v1alpha1"), // DEPRECATED - ADR-025
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register schemes
	err = workflowv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = kubernetesexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create Kubernetes client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// No external dependencies for Workflow Execution
})

var _ = AfterSuite(func() {
	cancel()

	By("tearing down the Envtest environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
```

---

### Kubernetes Executor: Kind Pattern (Existing) (DEPRECATED - ADR-025)

**File**: `test/integration/kubernetesexecution/suite_test.go`

```go
package kubernetesexecution

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1"
)

var (
	k8sClient client.Client
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestKubernetesExecutionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernetes Execution Integration Suite (Kind)")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("connecting to Kind cluster")
	// Uses KUBECONFIG environment variable pointing to Kind cluster
	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	// Register scheme
	err = kubernetesexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create Kubernetes client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Kind cluster and test resources set up by Makefile
})

var _ = AfterSuite(func() {
	cancel()
	// Kind cluster cleanup handled by Makefile
})
```

---

## Performance Metrics

### Expected Test Execution Times

| Service | Setup | Test Execution | Cleanup | **Total** |
|---------|-------|----------------|---------|-----------|
| **Remediation Processor** (Envtest + Podman) | 5s | 15s | 2s | **22s** |
| **Workflow Execution** (Envtest) | 2s | 10s | 1s | **13s** |
| **Kubernetes Executor** (Kind) (DEPRECATED - ADR-025) | 35s | 60s | 5s | **100s** |
| **All 3 Services** | 42s | 85s | 8s | **135s** |

**vs All Kind** (180s total): **1.3x faster** ✅

---

## Implementation Checklist

### Phase 0: Shared Infrastructure (2 hours)

- [ ] **Install setup-envtest**: `go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`
- [ ] **Add Makefile targets**: All targets from specification above
- [ ] **Create hack/kind-config.yaml**: Kind cluster configuration
- [ ] **Test Envtest installation**: `setup-envtest use -p path`
- [ ] **Document dependencies**: Update README with Envtest requirement

### Service 1: Remediation Processor (1 hour)

- [ ] **Create suite_test.go**: Envtest setup with CRD paths
- [ ] **Test Podman connection**: Verify PostgreSQL + Redis accessible
- [ ] **Validate schema**: Ensure `remediation_audit` table exists
- [ ] **Run integration tests**: Verify Envtest + Podman work together
- [ ] **Document environment variables**: DB_HOST, DB_PORT, REDIS_HOST, REDIS_PORT

### Service 2: Workflow Execution (30 minutes)

- [ ] **Create suite_test.go**: Envtest setup with CRD paths
- [ ] **Test CRD creation**: Verify WorkflowExecution + KubernetesExecution CRDs
- [ ] **Test watch mechanism**: Verify status propagation
- [ ] **Run integration tests**: Verify Envtest orchestration
- [ ] **Document no external deps**: Note simplicity

### Service 3: Kubernetes Executor (30 minutes) (DEPRECATED - ADR-025)

- [ ] **Keep Kind approach**: No changes needed (already planned)
- [ ] **Verify Job execution**: Ensure real Jobs run
- [ ] **Test rollback capture**: Verify Job log parsing
- [ ] **Document Kind requirement**: Note why Kind is necessary

---

## CI Integration

### GitHub Actions Workflow

**File**: `.github/workflows/integration-tests-phase3.yml`

```yaml
name: Integration Tests - Phase 3 (Hybrid)

on:
  pull_request:
    paths:
      - 'pkg/remediationprocessing/**'
      - 'pkg/workflowexecution/**'
      - 'pkg/kubernetesexecution/**'
      - 'test/integration/**'

jobs:
  test-remediation-envtest:
    name: Remediation Processor (Envtest + Podman)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Install Podman
        run: |
          sudo apt-get update
          sudo apt-get install -y podman

      - name: Install Envtest
        run: |
          go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
          setup-envtest use -p path

      - name: Run Integration Tests
        run: make test-integration-remediationprocessor

      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage-integration-remediation.out
          flags: integration-remediation

  test-workflow-envtest:
    name: Workflow Execution (Envtest)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Install Envtest
        run: |
          go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
          setup-envtest use -p path

      - name: Run Integration Tests
        run: make test-integration-workflowexecution

      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage-integration-workflow.out
          flags: integration-workflow

  test-executor-kind:
    name: Kubernetes Executor (Kind) (DEPRECATED - ADR-025)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Install Kind
        run: |
          curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.30.0/kind-linux-amd64
          chmod +x ./kind
          sudo mv ./kind /usr/local/bin/kind

      - name: Run Integration Tests
        run: make test-integration-kubernetesexecutor

      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage-integration-executor.out
          flags: integration-executor
```

---

## Dependencies

### Developer Machine Requirements

**All Services**:
- Go 1.22+
- kubectl

**Remediation Processor**:
- Podman (`brew install podman` on macOS)
- Envtest binaries (auto-installed via `make install-envtest`)

**Workflow Execution**:
- Envtest binaries (auto-installed)

**Kubernetes Executor** (DEPRECATED - ADR-025):
- Kind (`brew install kind` on macOS)

---

## Benefits of Approved Architecture

### Speed Improvements

✅ **Remediation Processor**: 2.5x faster (22s vs 55s with all Kind)
✅ **Workflow Execution**: 3.5x faster (13s vs 45s with all Kind)
✅ **Overall Pipeline**: 1.3x faster (135s vs 180s with all Kind)

### Developer Experience

✅ **Fast Iteration**: Envtest starts in 1-2s (vs 30-60s for Kind)
✅ **Resource Efficient**: Lower CPU/memory usage for Envtest services
✅ **Selective Testing**: Run only affected services quickly
✅ **Clear Separation**: Infrastructure matches test requirements

### Confidence Levels

✅ **High Confidence**: 96% overall (98% + 95% + 95%)
✅ **Real Execution**: Kubernetes Executor (DEPRECATED - ADR-025) tests actual Job runs
✅ **CRD Orchestration**: Envtest perfect for CRD-only controllers

---

## Updated Effort Estimates

### Total Implementation Effort (with all improvements)

| Component | Base | Infrastructure | Envtest Setup | Edge Cases (Option B) | **Total** |
|-----------|------|----------------|---------------|----------------------|-----------|
| **Base Implementation** | 98h | +14h | +2h | 0h | **114h** |
| **Edge Cases (Option C)** | 98h | +14h | +2h | +20h | **134h** |
| **Edge Cases (Option A)** | 98h | +14h | +2h | +32h | **146h** |

**With Hybrid Envtest/Kind (Option 1)**: Effort estimates remain the same, +2h for Envtest patterns already included.

---

## Success Criteria

### Integration Tests Must:

✅ **Test CRD Coordination**: Watch-based status propagation
✅ **Test Business Logic**: Real business components, not mocks
✅ **Execute Quickly**: <3 minutes for all 3 services
✅ **Run in CI**: GitHub Actions compatible
✅ **Be Maintainable**: Clear patterns, documented setup

### Remediation Processor:
- Envtest CRD operations validated
- PostgreSQL queries execute correctly
- Redis caching works
- Owner references cascade properly

### Workflow Execution:
- Dependency resolution works
- Parallel execution coordinated
- Status propagation through watches
- Rollback logic triggered correctly

### Kubernetes Executor (DEPRECATED - ADR-025):
- Real Jobs execute (kubectl scale, etc.)
- Rego policies enforce correctly
- Rollback info captured from Job logs
- RBAC violations detected

---

## Documentation Updates Required

- [ ] **README.md**: Add Envtest installation instructions
- [ ] **CONTRIBUTING.md**: Document test execution commands
- [ ] **Implementation Plans**: Update with approved infrastructure
- [ ] **CI Documentation**: Document hybrid approach
- [ ] **Troubleshooting Guide**: Common Envtest + Kind issues

---

## Final Approval Summary

**Decision**: ✅ **Hybrid Envtest/Kind Architecture Approved**

**Architecture**:
- Remediation Processor: Envtest + Podman (98% confidence)
- Workflow Execution: Envtest (95% confidence)
- Kubernetes Executor (DEPRECATED - ADR-025): Kind (95% confidence)

**Overall Confidence**: **96%**
**Speed Improvement**: **1.3-1.5x faster**
**Additional Effort**: **+2 hours**

**Next Steps**:
1. Implement Phase 0 shared infrastructure (2 hours)
2. Add service-specific patterns during Day 1 of each service
3. Update implementation plans with approved architecture
4. Proceed with edge case testing (decision pending: Option A/B/C)

---

**Document Version**: 1.0
**Last Updated**: 2025-10-14
**Status**: ✅ **APPROVED AND LOCKED**
**Approved By**: User
**Implementation Priority**: P0 - Required before Phase 1

