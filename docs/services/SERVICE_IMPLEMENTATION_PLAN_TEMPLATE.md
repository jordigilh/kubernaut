# [Service Name] - Implementation Plan Template

**Version**: v1.3 - INTEGRATION TEST DECISION TREE
**Last Updated**: 2025-10-12
**Timeline**: [X] days
**Status**: Ready for Implementation
**Change Log**:
- v1.3: Added Integration Test Environment Decision Tree (KIND/envtest/Podman/Mocks)
- v1.2: Added Kind Cluster Test Template for integration tests
- v1.1: Added table-driven testing patterns (see [TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md](./TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md))

---

## 🎯 Quick Reference

**Use this template for**: All Kubernaut stateless services and CRD controllers
**Based on**: Gateway Service (proven success) + Dynamic Toolset Service enhancements
**Methodology**: APDC-TDD with Integration-First Testing
**Success Rate**: Gateway achieved 95% test coverage, 100% business requirement coverage

---

## Document Purpose

This template incorporates lessons learned from:
1. **Gateway Service**: Production-ready implementation (21/22 tests passing, 95% coverage)
2. **Dynamic Toolset Service**: Enhanced with additional best practices
3. **Critical Gaps Identified**: From Gateway post-implementation triage
4. **Kind Cluster Template**: Standardized integration testing (81% setup reduction) ⭐ **v1.2**

**Key Improvements Over Ad-Hoc Planning**:
- Integration-first testing (catches issues 2 days earlier)
- Schema validation before testing (prevents test failures)
- Daily progress tracking (better communication)
- BR coverage matrix (ensures 100% requirement coverage)
- Production readiness checklist (reduces deployment issues)
- File organization strategy (cleaner git history)
- Table-driven testing pattern (25-40% less test code) ⭐ **v1.1**
- Kind cluster test template (15 lines vs 80+, complete imports) ⭐ **v1.2**
- **Integration test decision tree (KIND/envtest/Podman/Mocks)** ⭐ **v1.3**
- **Test environment matched to service needs (avoids over-engineering)** ⭐ **v1.3**

---

## Prerequisites Checklist

Before starting Day 1, ensure:
- [ ] Service specifications complete (overview, API spec, implementation docs)
- [ ] Business requirements documented (BR-[CATEGORY]-XXX format)
- [ ] Architecture decisions approved
- [ ] Dependencies identified
- [ ] Success criteria defined
- [ ] **Integration test environment determined** (see decision tree below) ⭐ **v1.3**
- [ ] **Required test infrastructure available** (KIND/envtest/Podman/none) ⭐ **v1.3**

---

## 🔍 Integration Test Environment Decision (v1.3) ⭐ NEW

**CRITICAL**: Determine your integration test environment **before Day 1** using this decision tree.

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

### Classification Guide

#### 🔴 KIND Required
**Use When**:
- Writes CRDs or Kubernetes resources
- Needs RBAC enforcement
- Uses TokenReview API for authentication
- Requires ServiceAccount permissions testing

**Examples**: Gateway Service, Dynamic Toolset Service (V2)

**Prerequisites**:
- [ ] KIND cluster available (`make bootstrap-dev`)
- [ ] Kind template documentation reviewed ([KIND_CLUSTER_TEST_TEMPLATE.md](../testing/KIND_CLUSTER_TEST_TEMPLATE.md))

---

#### 🟡 ENVTEST Required
**Use When**:
- Reads from Kubernetes (logs, events, resources)
- Needs field selectors (e.g., `.spec.nodeName=worker`)
- Writes ConfigMaps/Services (but no RBAC needed)
- Testing with CRDs (no RBAC validation)

**Examples**: Dynamic Toolset Service (V1), HolmesGPT API Service

**Prerequisites**:
- [ ] `setup-envtest` installed (`go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`)
- [ ] Binaries downloaded (`setup-envtest use 1.31.0`)

---

#### 🟢 PODMAN Required
**Use When**:
- No Kubernetes operations
- Needs PostgreSQL, Redis, or other databases
- External service dependencies

**Examples**: Data Storage Service, Context API Service

**Prerequisites**:
- [ ] Docker/Podman available
- [ ] testcontainers-go configured

---

#### ⚪ HTTP MOCKS Only
**Use When**:
- No Kubernetes operations
- No database dependencies
- Only HTTP API calls to other services

**Examples**: Effectiveness Monitor Service, Notification Service

**Prerequisites**:
- [ ] None (uses Go stdlib `net/http/httptest`)

---

### Quick Classification Examples

| Service Type | Kubernetes Ops | Databases | Test Env |
|--------------|---------------|-----------|----------|
| Writes CRDs + RBAC | ✅ Write + RBAC | ❌ | 🔴 KIND |
| Writes ConfigMaps only | ✅ Write (no RBAC) | ❌ | 🟡 ENVTEST |
| Reads K8s (field selectors) | ✅ Read (complex) | ❌ | 🟡 ENVTEST |
| Reads K8s (simple) | ✅ Read (simple) | ❌ | Fake Client |
| HTTP API + PostgreSQL | ❌ | ✅ | 🟢 PODMAN |
| HTTP API only | ❌ | ❌ | ⚪ HTTP MOCKS |

### Update Your Plan

Once determined, update all instances of `[TEST_ENVIRONMENT]` in this plan with your choice:
- Replace `[TEST_ENVIRONMENT]` with: **KIND** | **ENVTEST** | **PODMAN** | **HTTP_MOCKS**
- Update prerequisites checklist above
- Review setup requirements in Day 8 (Integration Test Setup)

**Reference Documentation**:
- [Integration Test Environment Decision Tree](../../testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md)
- [Stateless Services Integration Test Strategy](../stateless/INTEGRATION_TEST_STRATEGY.md)
- [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)

---

## Timeline Overview

| Phase | Days | Focus | Key Deliverables |
|-------|------|-------|------------------|
| **Foundation** | 1 | Types, interfaces, K8s client | Package structure, interfaces |
| **Core Logic** | 2-6 | Business logic components | All components implemented |
| **Integration** | 7 | Server, API, metrics | Complete service |
| **Testing** | 8-10 | Integration + Unit tests | 70%+ coverage |
| **Finalization** | 11-12 | E2E, docs, production readiness | Ready for deployment |

**Total**: 11-12 days (with buffer)

---

## Day-by-Day Breakdown

### Day 1: Foundation (8h)

#### ANALYSIS Phase (1h)
**Search existing patterns:**
```bash
codebase_search "Kubernetes client initialization in-cluster config"
codebase_search "[Service functionality] implementations"
grep -r "relevant patterns" pkg/ cmd/ --include="*.go"
```

**Map business requirements:**
- List all BR-[CATEGORY]-XXX requirements
- Identify critical path requirements
- Note any missing specifications

#### PLAN Phase (1h)
**TDD Strategy:**
- Unit tests: [Component list] (70%+ coverage target)
- Integration tests: [Scenario list] (>50% coverage target)
- E2E tests: [Workflow list] (<10% coverage target)

**Integration points:**
- Main app: `cmd/[service]/main.go`
- Business logic: `pkg/[service]/`
- Tests: `test/unit/[service]/`, `test/integration/[service]/`

**Success criteria:**
- [Performance metric 1] (target: X)
- [Performance metric 2] (target: Y)
- [Functional requirement] verified

#### DO-DISCOVERY (6h)
**Create package structure:**
```bash
mkdir -p cmd/[service]
mkdir -p pkg/[service]/{component1,component2,component3}
mkdir -p internal/[service]/{helpers}
mkdir -p test/unit/[service]
mkdir -p test/integration/[service]
mkdir -p test/e2e/[service]
```

**Create foundational files:**
- `pkg/[service]/types.go` - Core type definitions
- `pkg/[service]/[interface1].go` - Primary interface
- `pkg/[service]/[interface2].go` - Secondary interface
- `internal/[service]/k8s/client.go` - Kubernetes client wrapper (if needed)
- `cmd/[service]/main.go` - Basic skeleton

**Validation:**
- [ ] All packages created
- [ ] Types defined
- [ ] Interfaces defined
- [ ] Main.go compiles
- [ ] Zero lint errors

**EOD Documentation:**
- [ ] Create `implementation/phase0/01-day1-complete.md`
- [ ] Document architecture decisions
- [ ] Note any deviations from plan

---

### Days 2-6: Core Implementation (5 days, 8h each)

**Pattern for Each Component:**

#### DO-RED: Write Tests First (1.5-2h per component)
**File**: `test/unit/[service]/[component]_test.go`

**⭐ RECOMMENDED: Use Table-Driven Tests (DescribeTable) whenever possible**

**Pattern 1: Table-Driven Tests for Multiple Similar Scenarios** (Preferred)
```go
var _ = Describe("BR-[CATEGORY]-XXX: [Component Name]", func() {
    // Use DescribeTable for multiple test cases with same logic
    DescribeTable("should handle various input scenarios",
        func(input InputType, expectedOutput OutputType, expectError bool) {
            result, err := component.Method(input)

            if expectError {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).ToNot(HaveOccurred())
                Expect(result).To(Equal(expectedOutput))
            }
        },
        Entry("scenario 1 description", input1, output1, false),
        Entry("scenario 2 description", input2, output2, false),
        Entry("scenario 3 with error", input3, nil, true),
        // Easy to add more scenarios - just add Entry lines!
    )
})
```

**Pattern 2: Traditional Tests for Unique Logic** (When needed)
```go
var _ = Describe("BR-[CATEGORY]-XXX: [Component Name]", func() {
    Context("when [unique condition]", func() {
        It("should [behavior]", func() {
            // Test implementation for unique scenario
        })
    })
})
```

**When to Use Table-Driven Tests**:
- ✅ Testing same logic with different inputs/outputs
- ✅ Testing multiple detection/validation scenarios
- ✅ Testing various error conditions
- ✅ Testing different configuration permutations
- ✅ Testing boundary conditions and edge cases

**When to Use Traditional Tests**:
- Complex setup that varies significantly per test
- Unique test logic that doesn't fit table pattern
- One-off tests with complex assertions

**Benefits**:
- 25-40% less code through elimination of duplication
- Easier to add new test cases (just add Entry)
- Better test organization and readability
- Consistent assertion patterns

**Reference**: See Dynamic Toolset detector tests for examples:
- `test/unit/toolset/prometheus_detector_test.go`
- `test/unit/toolset/grafana_detector_test.go`

**Validation:**
- [ ] Tests written (prefer table-driven where applicable)
- [ ] Tests fail (expected)
- [ ] Business requirements referenced (BR-XXX-XXX)
- [ ] Entry names clearly describe scenarios

#### DO-GREEN: Minimal Implementation (1.5-2h per component)
**File**: `pkg/[service]/[component].go`

**Validation:**
- [ ] Tests pass
- [ ] No extra features
- [ ] Integration point identified

#### DO-REFACTOR: Extract Common Patterns (2-3h per day)
**Common Refactorings:**
- Extract shared utilities
- Standardize error handling
- Extract validation logic
- Create helper functions

**Validation:**
- [ ] Code DRY (Don't Repeat Yourself)
- [ ] Patterns consistent
- [ ] Tests still pass

**Day-Specific Focus:**
- **Day 2**: [Component set 1]
- **Day 3**: [Component set 2]
- **Day 4**: [Component set 3] + **EOD: Create 02-day4-midpoint.md** ⭐
- **Day 5**: [Component set 4]
- **Day 6**: [Component set 5] + **DO-REFACTOR: Error handling philosophy doc** ⭐

---

### Day 7: Server + API + Metrics (8h)

#### HTTP Server Implementation (3h)
- Server struct with router
- Route registration
- Middleware stack
- Health/readiness endpoints

#### Metrics Implementation (2h)
- Prometheus metrics definition (10+ metrics minimum)
- Metric recording in business logic
- Metrics endpoint exposure

#### Main Application Integration (2h)
- Component wiring in main.go
- Configuration loading
- Graceful shutdown

#### Critical EOD Checkpoints (1h) ⭐
- [ ] **Schema Validation**: Create `design/01-[schema]-validation.md`
- [ ] **Test Infrastructure Setup**: Create test suite skeleton
- [ ] **Status Documentation**: Create `03-day7-complete.md`
- [ ] **Testing Strategy**: Create `testing/01-integration-first-rationale.md`

**Why These Matter**: Gateway found these prevented 2+ days of debugging

---

### Day 8: Integration-First Testing ⭐ (8h)

**CRITICAL CHANGE FROM TRADITIONAL TDD**: Integration tests BEFORE unit tests

#### Morning: 5 Critical Integration Tests (4h) ⭐

**Test Infrastructure Setup**: Choose based on your `[TEST_ENVIRONMENT]` decision ⭐ **v1.3**

<details>
<summary><strong>🔴 KIND Setup (if [TEST_ENVIRONMENT] = KIND)</strong></summary>

Use **Kind Cluster Test Template** for standardized integration tests:

**Documentation**: [Kind Cluster Test Template Guide](../testing/KIND_CLUSTER_TEST_TEMPLATE.md)

```go
package myservice

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (Kind)")
}

var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
	// Use Kind template for standardized test setup
	// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
	suite = kind.Setup("[service]-test", "kubernaut-system")

	// Additional setup if needed (PostgreSQL, Redis, etc.)
	// suite.WaitForPostgreSQLReady(60 * time.Second)
})

var _ = AfterSuite(func() {
	suite.Cleanup()
})

// Integration Test Pattern
Describe("Integration Test [N]: [Scenario]", func() {
	var component *[service].Component

	BeforeEach(func() {
		// Setup real components using Kind cluster resources
		// Example: Deploy test services
		// svc, err := suite.DeployPrometheusService("[service]-test")

		// Initialize component with real dependencies
		component = [service].NewComponent(suite.Client, logger)
	})

	It("should [end-to-end behavior]", func() {
		// Complete workflow test using real Kind cluster resources
		// Example test assertion
		result, err := component.Process(suite.Context, input)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal("success"))
	})
})
```

**Key Benefits of Kind Template**:
- ✅ **15 lines setup** vs 80+ custom (81% reduction)
- ✅ **Complete imports** (copy-pasteable)
- ✅ **Kind cluster DNS** (no port-forwarding)
- ✅ **Automatic cleanup** (`suite.Cleanup()`)
- ✅ **Consistent pattern** (aligned with Gateway, Dynamic Toolset V2)
- ✅ **30+ helper methods** (services, ConfigMaps, database, wait utilities)

</details>

---

<details>
<summary><strong>🟡 ENVTEST Setup (if [TEST_ENVIRONMENT] = ENVTEST)</strong></summary>

Use **envtest** for Kubernetes API server testing without full cluster:

**Prerequisites**:
```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use 1.31.0
```

**Documentation**: [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)

```go
package myservice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (envtest)")
}

var (
	cfg       *rest.Config
	k8sClient kubernetes.Interface
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Start envtest with CRDs if needed
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd")},
		ErrorIfCRDPathMissing: false, // Set true if CRDs required
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test K8s API operations]", func() {
		// Create test resources
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}
		_, err := k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Test your service logic
		component := [service].NewComponent(k8sClient)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of envtest**:
- ✅ **Real API server** validation (schema, field selectors)
- ✅ **CRD support** (register definitions + use controller-runtime client)
- ✅ **Fast setup** (~3 seconds vs ~60 seconds for KIND)
- ✅ **Standard K8s client** (same as production)
- ⚠️ **No RBAC/TokenReview** (use KIND if needed)

</details>

---

<details>
<summary><strong>🟢 PODMAN Setup (if [TEST_ENVIRONMENT] = PODMAN)</strong></summary>

Use **testcontainers-go** for PostgreSQL/Redis/database testing:

**Prerequisites**: Docker or Podman installed

**Documentation**: [Podman Integration Test Template](../../testing/PODMAN_INTEGRATION_TEST_TEMPLATE.md)

```go
package myservice

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (Podman)")
}

var (
	postgresContainer testcontainers.Container
	redisContainer    testcontainers.Container
	dbURL             string
	redisAddr         string
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
	Expect(err).NotTo(HaveOccurred())

	// Get database URL
	host, _ := postgresContainer.Host(ctx)
	port, _ := postgresContainer.MappedPort(ctx, "5432")
	dbURL = fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	// Start Redis container (if needed)
	// ... similar pattern
})

var _ = AfterSuite(func() {
	ctx := context.Background()
	if postgresContainer != nil {
		postgresContainer.Terminate(ctx)
	}
	if redisContainer != nil {
		redisContainer.Terminate(ctx)
	}
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test database operations]", func() {
		// Test your service with real database
		component := [service].NewComponent(dbURL, redisAddr)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of Podman**:
- ✅ **Real databases** (PostgreSQL, Redis, etc.)
- ✅ **Fast startup** (~1-2 seconds)
- ✅ **Automatic cleanup** (testcontainers-go)
- ✅ **No Kubernetes** (simpler for pure HTTP APIs)

</details>

---

<details>
<summary><strong>⚪ HTTP MOCKS Setup (if [TEST_ENVIRONMENT] = HTTP_MOCKS)</strong></summary>

Use **net/http/httptest** for mocking external HTTP APIs:

**Prerequisites**: None (Go stdlib)

```go
package myservice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (HTTP Mocks)")
}

var (
	mockDataStorageAPI    *httptest.Server
	mockMonitoringAPI     *httptest.Server
)

var _ = BeforeSuite(func() {
	// Mock Data Storage API
	mockDataStorageAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/audit/actions" {
			json.NewEncoder(w).Encode(mockActions)
		}
	}))

	// Mock Monitoring API
	mockMonitoringAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/metrics" {
			json.NewEncoder(w).Encode(mockMetrics)
		}
	}))
})

var _ = AfterSuite(func() {
	if mockDataStorageAPI != nil {
		mockDataStorageAPI.Close()
	}
	if mockMonitoringAPI != nil {
		mockMonitoringAPI.Close()
	}
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test HTTP API interactions]", func() {
		// Test your service with mocked APIs
		component := [service].NewComponent(mockDataStorageAPI.URL, mockMonitoringAPI.URL)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of HTTP Mocks**:
- ✅ **Zero infrastructure** (no KIND, databases, containers)
- ✅ **Instant startup** (milliseconds)
- ✅ **Easy failure simulation** (return errors, timeouts)
- ✅ **Perfect for pure HTTP API services**

</details>

---

**Required Tests**:
1. **Test 1**: Basic flow (input → processing → output) - 90 min
2. **Test 2**: Deduplication/Caching logic - 45 min
3. **Test 3**: Error recovery scenario - 60 min
4. **Test 4**: Data persistence/state management - 45 min
5. **Test 5**: Authentication/Authorization - 30 min

**Validation After Integration Tests**:
- [ ] Architecture validated
- [ ] Integration issues found early
- [ ] Timing/concurrency issues identified
- [ ] Ready for unit test details

#### Afternoon: Unit Tests Part 1 (4h)
- Focus on components tested in integration tests
- Fill in edge cases
- Add negative test cases

**Metrics Validation Checkpoint**:
```bash
curl http://localhost:9090/metrics | grep [service]_
```

---

### Day 9: Unit Tests Part 2 (8h)

#### Morning: Unit Tests - [Component Group 1] (4h)
- Edge cases
- Error conditions
- Boundary values

#### Afternoon: Unit Tests - [Component Group 2] (4h)
- Mock dependencies
- Timeout scenarios
- Concurrent access

**EOD: Create BR Coverage Matrix** ⭐
**File**: `implementation/testing/BR-COVERAGE-MATRIX.md`

```markdown
| BR | Requirement | Unit Tests | Integration Tests | Coverage |
|----|-------------|------------|-------------------|----------|
| BR-XXX-001 | [Requirement] | 3 | 1 | 100% ✅ |
| BR-XXX-002 | [Requirement] | 2 | 1 | 100% ✅ |
...
```

**Validation**:
- [ ] Unit test coverage > 70%
- [ ] All BRs mapped to tests
- [ ] No untested BRs (or justified as skipped)

---

### Day 10: Advanced Integration + E2E Tests (8h)

#### Advanced Integration Tests (4h)
- Concurrent request scenarios
- Resource exhaustion
- Long-running operations
- Failure recovery

#### E2E Test Setup (2h)
- Kind cluster setup
- Service deployment
- Dependencies deployment

#### E2E Test Execution (2h)
- Complete workflow tests
- Real environment validation

---

### Day 11: Documentation (8h)

#### Implementation Documentation (4h)
- Complete service overview
- API documentation
- Configuration reference
- Integration guide

#### Design Decision Documentation (2h)
- Create DD-XXX entries in `DESIGN_DECISIONS.md`
- Document alternatives considered
- Explain rationale for choices

#### Testing Documentation (2h)
- Testing strategy documentation
- Test coverage report
- Known limitations

---

### Day 12: CHECK Phase + Production Readiness ⭐ (8h)

#### CHECK Phase Validation (2h)
**Checklist**:
- [ ] All business requirements met
- [ ] Build passes without errors
- [ ] All tests passing
- [ ] Metrics exposed and validated
- [ ] Health checks functional
- [ ] Authentication working
- [ ] Documentation complete
- [ ] No lint errors

#### Production Readiness Checklist (2h) ⭐
**File**: `implementation/PRODUCTION_READINESS_REPORT.md`

```markdown
## Production Readiness Assessment

### Functional Validation
- [ ] All critical paths tested
- [ ] Error handling validated
- [ ] Graceful degradation tested
- [ ] Resource limits appropriate
- [ ] Performance targets met

### Operational Validation
- [ ] Metrics comprehensive
- [ ] Logging structured
- [ ] Health checks reliable
- [ ] Graceful shutdown tested
- [ ] RBAC permissions minimal

### Deployment Validation
- [ ] Deployment manifests complete
- [ ] ConfigMaps defined
- [ ] Secrets documented
- [ ] Resource requests/limits set
- [ ] Liveness/readiness probes configured
```

#### File Organization (1h) ⭐
**File**: `implementation/FILE_ORGANIZATION_PLAN.md`

Categorize all files:
- Production implementation (pkg/, cmd/)
- Unit tests (test/unit/)
- Integration tests (test/integration/)
- E2E tests (test/e2e/)
- Configuration (deploy/)
- Documentation (docs/)

**Git commit strategy**:
```
Commit 1: Foundation (types, interfaces)
Commit 2: Component 1
Commit 3: Component 2
...
Commit N: Tests
Commit N+1: Documentation
Commit N+2: Deployment manifests
```

#### Performance Benchmarking (1h) ⭐
**File**: `implementation/PERFORMANCE_REPORT.md`

```bash
go test -bench=. -benchmem ./pkg/[service]/...
```

Validate:
- [ ] Latency targets met
- [ ] Throughput targets met
- [ ] Memory usage acceptable
- [ ] CPU usage acceptable

#### Troubleshooting Guide (1h) ⭐
**File**: `implementation/TROUBLESHOOTING_GUIDE.md`

For each common issue:
- **Symptoms**: What the user sees
- **Diagnosis**: How to investigate
- **Resolution**: How to fix

#### Confidence Assessment (1h) ⭐
**File**: `implementation/CONFIDENCE_ASSESSMENT.md`

```markdown
## Confidence Assessment

**Implementation Accuracy**: X% (target 90%+)
**Evidence**: [spec compliance, code review results]

**Test Coverage**:
- Unit: X% (target 70%+)
- Integration: X% (target 50%+)
- E2E: X% (target <10%)

**Business Requirement Coverage**: X% (target 100%)
**Mapped BRs**: [count]
**Untested BRs**: [count with justification]

**Production Readiness**: X% (target 95%+)
**Risks**: [list with mitigations]
```

#### Handoff Summary (Last Step) ⭐
**File**: `implementation/00-HANDOFF-SUMMARY.md`

Executive summary including:
- What was accomplished
- Current state
- Next steps
- Key files
- Key decisions
- Lessons learned
- Troubleshooting tips

---

## Critical Checkpoints (From Gateway Learnings)

### ✅ Checkpoint 1: Integration-First Testing (Day 8)
**Why**: Catches architectural issues 2 days earlier
**Action**: Write 5 integration tests before unit tests
**Evidence**: Gateway caught function signature mismatches early

### ✅ Checkpoint 2: Schema Validation (Day 7 EOD)
**Why**: Prevents test failures from schema mismatches
**Action**: Validate 100% field alignment before testing
**Evidence**: Gateway added missing CRD fields, avoided test failures

### ✅ Checkpoint 3: BR Coverage Matrix (Day 9 EOD)
**Why**: Ensures all requirements have test coverage
**Action**: Map every BR to tests, justify any skipped
**Evidence**: Gateway achieved 100% BR coverage

### ✅ Checkpoint 4: Production Readiness (Day 12)
**Why**: Reduces production deployment issues
**Action**: Complete comprehensive readiness checklist
**Evidence**: Gateway deployment went smoothly

### ✅ Checkpoint 5: Daily Status Docs (Days 1, 4, 7, 12)
**Why**: Better progress tracking and handoffs
**Action**: Create progress documentation at key milestones
**Evidence**: Gateway handoff was smooth

---

## Documentation Standards

### Daily Status Documents

**Day 1**: `01-day1-complete.md`
- Package structure created
- Types and interfaces defined
- Build successful
- Confidence assessment

**Day 4**: `02-day4-midpoint.md`
- Components completed so far
- Integration status
- Any blockers
- Confidence assessment

**Day 7**: `03-day7-complete.md`
- Core implementation complete
- Server and metrics implemented
- Schema validation complete
- Test infrastructure ready
- Confidence assessment

**Day 12**: `00-HANDOFF-SUMMARY.md`
- Executive summary
- Complete file inventory
- Key decisions
- Lessons learned
- Next steps

### Design Decision Documents

**Pattern**: Create DD-XXX entries for significant decisions

**Template**:
```markdown
## DD-XXX: [Decision Title]

### Status
**[Status Emoji] [Status]** (YYYY-MM-DD)

### Context & Problem
[What problem are we solving?]

### Alternatives Considered
1. **Alternative A**: [Pros/Cons]
2. **Alternative B**: [Pros/Cons]
3. **Alternative C**: [Pros/Cons]

### Decision
**APPROVED: Alternative X**

**Rationale**:
1. [Reason 1]
2. [Reason 2]

### Consequences
**Positive**: [Benefits]
**Negative**: [Trade-offs + Mitigations]
```

---

## Testing Strategy

### Test Distribution (From Gateway Success)

| Type | Coverage | Purpose |
|------|----------|---------|
| **Unit** | 70%+ | Component logic, edge cases |
| **Integration** | >50% | Component interactions, real dependencies |
| **E2E** | <10% | Complete workflows, production-like |

### Integration-First Order ⭐

**Traditional (DON'T DO THIS)**:
```
Days 7-8: Unit tests (40+ tests)
Days 9-10: Integration tests (12+ tests)
```

**Integration-First (DO THIS)** ✅:
```
Day 8 Morning: 5 critical integration tests
Day 8 Afternoon: Unit tests - Component Group 1
Day 9 Morning: Unit tests - Component Group 2
Day 9 Afternoon: Unit tests - Component Group 3
Day 10: Advanced integration + E2E tests
```

**Why This Works Better**:
- Validates architecture before details
- Catches integration issues early (cheaper to fix)
- Provides confidence for unit test details
- Follows TDD spirit (prove it works, then refine)

### Table-Driven Testing Pattern ⭐ (RECOMMENDED)

**Why Table-Driven Tests?**
Based on Dynamic Toolset Service implementation:
- **38% code reduction** in test files (1,612 lines → 1,001 lines)
- **25-40% faster** to add new test cases
- **Better maintainability**: Change logic once, all entries benefit
- **Clearer coverage**: Easy to see all scenarios at a glance

**Implementation Pattern**:

#### Pattern 1: Success Scenarios
```go
DescribeTable("should detect [Service] services",
    func(name, namespace string, labels map[string]string, ports []corev1.ServicePort, expectedEndpoint string) {
        service := &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:      name,
                Namespace: namespace,
                Labels:    labels,
            },
            Spec: corev1.ServiceSpec{Ports: ports},
        }

        result, err := detector.Detect(ctx, service)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).ToNot(BeNil())
        Expect(result.Endpoint).To(Equal(expectedEndpoint))
    },
    Entry("with standard label", "svc-1", "ns-1",
        map[string]string{"app": "myapp"},
        []corev1.ServicePort{{Port: 8080}},
        "http://svc-1.ns-1.svc.cluster.local:8080"),
    Entry("with name-based detection", "myapp-server", "ns-2",
        nil,
        []corev1.ServicePort{{Port: 8080}},
        "http://myapp-server.ns-2.svc.cluster.local:8080"),
    // Easy to add more - just add Entry!
)
```

#### Pattern 2: Negative Scenarios
```go
DescribeTable("should NOT detect non-matching services",
    func(name string, labels map[string]string, ports []corev1.ServicePort) {
        service := &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:   name,
                Labels: labels,
            },
            Spec: corev1.ServiceSpec{Ports: ports},
        }

        result, err := detector.Detect(ctx, service)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).To(BeNil())
    },
    Entry("for different service type", "other-svc",
        map[string]string{"app": "other"},
        []corev1.ServicePort{{Port: 9090}}),
    Entry("for service without ports", "no-ports",
        map[string]string{"app": "myapp"},
        []corev1.ServicePort{}),
)
```

#### Pattern 3: Health Check Scenarios
```go
DescribeTable("should validate health status",
    func(statusCode int, body string, expectSuccess bool) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(statusCode)
            w.Write([]byte(body))
        }))
        defer server.Close()

        err := checker.HealthCheck(ctx, server.URL)
        if expectSuccess {
            Expect(err).ToNot(HaveOccurred())
        } else {
            Expect(err).To(HaveOccurred())
        }
    },
    Entry("with 200 OK", http.StatusOK, "", true),
    Entry("with 204 No Content", http.StatusNoContent, "", true),
    Entry("with 503 Unavailable", http.StatusServiceUnavailable, "", false),
)
```

#### Pattern 4: Setup Functions for Complex Cases
```go
DescribeTable("should handle error conditions",
    func(setupServer func() string) {
        endpoint := setupServer()
        err := component.Process(endpoint)
        Expect(err).To(HaveOccurred())
    },
    Entry("for connection refused", func() string {
        return "http://localhost:9999"
    }),
    Entry("for timeout", func() string {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            time.Sleep(10 * time.Second)
        }))
        DeferCleanup(server.Close)
        return server.URL
    }),
    Entry("for invalid URL", func() string {
        return "not-a-valid-url"
    }),
)
```

**Best Practices**:
1. Use descriptive Entry names that document the scenario
2. Keep table logic simple and consistent
3. Use traditional It() for truly unique scenarios
4. Group related scenarios in same DescribeTable
5. Use DeferCleanup for resource cleanup in Entry setup functions

**Reference Examples**:
- Excellent examples in `test/unit/toolset/*_detector_test.go`
- 73 tests consolidated from 77, 38% less code
- All tests passing with 100% coverage maintained

### Test Naming Convention

```go
// Business requirement reference in test description
Describe("BR-[CATEGORY]-XXX: [Requirement]", func() {
    // Prefer table-driven tests for multiple scenarios
    DescribeTable("should [behavior]",
        func(params...) {
            // Test logic
        },
        Entry("scenario 1", ...),
        Entry("scenario 2", ...),
    )

    // Use traditional It() for unique scenarios
    Context("when [unique condition]", func() {
        It("should [expected behavior]", func() {
            // Test implementation
        })
    })
})
```

---

## Performance Targets

Define service-specific targets:

| Metric | Target | Measurement |
|--------|--------|-------------|
| API Latency (p95) | < Xms | HTTP request duration |
| API Latency (p99) | < Yms | HTTP request duration |
| Throughput | > Z req/s | Requests per second |
| Memory Usage | < XMB | Per replica |
| CPU Usage | < X cores | Average |
| [Service-specific] | [Target] | [How measured] |

---

## Common Pitfalls to Avoid

### ❌ Don't Do This:
1. **Skip integration tests until end**: Costs 2+ days in debugging
2. **Write all unit tests first**: Wastes time on wrong details
3. **Skip schema validation**: Causes test failures later
4. **No daily status docs**: Makes handoffs difficult
5. **Skip BR coverage matrix**: Results in untested requirements
6. **No production readiness check**: Causes deployment issues
7. **Repetitive test code**: Copy-paste It blocks for similar scenarios
8. **No table-driven tests**: Results in 25-40% more code

### ✅ Do This Instead:
1. **Integration-first testing**: Validates architecture early
2. **5 critical integration tests Day 8**: Proves core functionality
3. **Schema validation Day 7**: Prevents test failures
4. **Daily progress docs**: Smooth handoffs and communication
5. **BR coverage matrix Day 9**: Ensures 100% requirement coverage
6. **Production checklist Day 12**: Smooth deployment
7. **Table-driven tests**: Use DescribeTable for multiple similar scenarios ⭐
8. **DRY test code**: Extract common test logic, parameterize with Entry

---

## Success Criteria

### Implementation Complete When:
- [ ] All business requirements implemented
- [ ] Build passes without errors
- [ ] Zero lint errors
- [ ] Unit test coverage > 70%
- [ ] Integration test coverage > 50%
- [ ] E2E tests passing
- [ ] All metrics exposed
- [ ] Health checks functional
- [ ] Documentation complete
- [ ] Production readiness validated

### Quality Indicators:
- **Code Quality**: No lint errors, follows Go idioms
- **Test Quality**: BDD style, clear assertions, business requirement references
- **Test Organization**: Table-driven tests for similar scenarios, 25-40% less test code
- **Test Maintainability**: Easy to add new cases (just add Entry), consistent patterns
- **Documentation Quality**: Complete, accurate, helpful
- **Production Readiness**: Deployment manifests complete, observability comprehensive

---

## Makefile Targets

Create consistent development commands:

```makefile
# Testing
.PHONY: test-unit-[service]
test-unit-[service]:
	go test -v ./test/unit/[service]/...

.PHONY: test-integration-[service]
test-integration-[service]:
	go test -v ./test/integration/[service]/...

.PHONY: test-e2e-[service]
test-e2e-[service]:
	go test -v ./test/e2e/[service]/...

# Coverage
.PHONY: test-coverage-[service]
test-coverage-[service]:
	go test -cover -coverprofile=coverage.out ./pkg/[service]/...
	go tool cover -html=coverage.out

# Build
.PHONY: build-[service]
build-[service]:
	go build -o bin/[service] ./cmd/[service]

# Linting
.PHONY: lint-[service]
lint-[service]:
	golangci-lint run ./pkg/[service]/... ./cmd/[service]/...

# Deployment
.PHONY: deploy-kind-[service]
deploy-kind-[service]:
	kubectl apply -f deploy/[service]/
```

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-10-11 | Initial template based on Gateway + Dynamic Toolset learnings | AI Assistant |

---

## Related Documents

- [00-core-development-methodology.mdc](.cursor/rules/00-core-development-methodology.mdc) - APDC-TDD methodology
- [Gateway Implementation](docs/services/stateless/gateway-service/implementation/) - Reference implementation
- [Dynamic Toolset Implementation](docs/services/stateless/dynamic-toolset/implementation/) - Enhanced patterns
- [PLAN_TRIAGE_VS_GATEWAY.md](docs/services/stateless/dynamic-toolset/implementation/PLAN_TRIAGE_VS_GATEWAY.md) - Gap analysis

---

**Template Status**: ✅ Ready for Use
**Success Rate**: Based on Gateway's 95% test coverage, 100% BR coverage
**Estimated Effort Savings**: 2-3 days per service (from early issue detection)

---

## Version History

### v1.1 (2025-10-11)
**Added: Table-Driven Testing Pattern** ⭐

**Changes**:
- Added comprehensive table-driven testing guidance in DO-RED section
- Added "Table-Driven Testing Pattern" subsection in Testing Strategy
- Provided 4 complete code pattern examples (success, negative, health checks, setup functions)
- Updated Common Pitfalls section with table-driven testing guidance
- Updated Success Criteria with test organization quality indicators
- Added references to Dynamic Toolset detector test examples

**Impact**:
- 25-40% less test code expected
- Better test maintainability
- Easier to extend test coverage

**Reference**: [TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md](./TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md)

---

### v1.0 (Initial Release)
**Base Template from Gateway + Dynamic Toolset Learnings**

**Included**:
- APDC-TDD methodology integration
- Integration-first testing strategy
- 12-day implementation timeline
- 5 critical checkpoints
- Daily progress documentation
- BR coverage matrix
- Production readiness checklist
- Performance benchmarking guidance

**Based On**:
- Gateway Service (proven success: 95% test coverage)
- Dynamic Toolset enhancements
- Gateway post-implementation triage

