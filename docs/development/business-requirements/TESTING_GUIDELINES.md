# Testing Guidelines: Business Requirements vs Unit Tests

This document provides clear guidance on **when** and **how** to use each type of test in the kubernaut system.

## 🎯 **Decision Framework**

### Quick Decision Tree
```
📝 QUESTION: What are you trying to validate?

├─ 💼 "Does it solve the business problem?"
│  ├─ User-facing functionality ────────────► BUSINESS REQUIREMENT TEST
│  ├─ Performance/reliability requirements ─► BUSINESS REQUIREMENT TEST
│  ├─ Business value delivery ─────────────► BUSINESS REQUIREMENT TEST
│  └─ Cross-component workflows ───────────► BUSINESS REQUIREMENT TEST
│
└─ 🔧 "Does the code work correctly?"
   ├─ Function/method behavior ────────────► UNIT TEST
   ├─ Error handling & edge cases ────────► UNIT TEST
   ├─ Internal component logic ───────────► UNIT TEST
   └─ Code correctness & robustness ──────► UNIT TEST
```

## 📊 **Test Type Comparison**

| Aspect | Business Requirement Tests | Unit Tests |
|--------|----------------------------|------------|
| **Purpose** | Validate business value delivery | Validate business behavior + implementation correctness |
| **Focus** | External behavior & outcomes | Internal code mechanics |
| **Audience** | Business stakeholders + developers | Developers |
| **Metrics** | Business KPIs (accuracy, cost, time) | Technical metrics (coverage, performance) |
| **Dependencies** | Realistic/controlled mocks | Minimal mocks |
| **Execution Time** | Slower (seconds to minutes) | Fast (milliseconds) |
| **Change Frequency** | Stable (business requirements) | Higher (implementation changes) |

## 🏗️ **When to Use Business Requirement Tests**

### ✅ **Use Business Requirements Tests For:**

#### 1. **User-Facing Features**
```go
// ✅ GOOD: Tests user-visible behavior
Describe("BR-AI-001: System Must Reduce Alert Noise by 80%", func() {
    It("should dramatically reduce duplicate alerts through correlation", func() {
        // Given: 100 similar alerts per hour (baseline)
        // When: Alert correlation is enabled
        // Then: Alert volume should be <20 alerts per hour
    })
})
```

#### 2. **Performance & Reliability Requirements**
```go
// ✅ GOOD: Tests business SLA compliance
Describe("BR-WF-003: Workflows Must Complete Within 30-Second SLA", func() {
    It("should process standard operations within performance threshold", func() {
        // Validates business requirement for operational responsiveness
    })
})
```

#### 3. **Business Value Delivery**
```go
// ✅ GOOD: Tests measurable business improvement
Describe("BR-AI-002: System Must Improve Accuracy by 25% Over 30 Days", func() {
    It("should demonstrate measurable learning and improvement", func() {
        // Tests quantifiable business value delivery
    })
})
```

#### 4. **Cross-Component Workflows**
```go
// ✅ GOOD: Tests end-to-end business processes
Describe("BR-INT-001: Alert-to-Resolution Must Complete Under 5 Minutes", func() {
    It("should handle complete alert lifecycle within business SLA", func() {
        // Tests complete business process across multiple components
    })
})
```

### ❌ **Don't Use Business Requirements Tests For:**

#### 1. **Implementation Details**
```go
// ❌ BAD: Tests internal implementation
Describe("validateWorkflowSteps function", func() {
    It("should return ValidationError for invalid step", func() {
        // This tests code behavior, not business value
    })
})
```

#### 2. **Technical Edge Cases**
```go
// ❌ BAD: Tests technical error handling
Describe("ProcessPendingAssessments with nil context", func() {
    It("should return context error", func() {
        // This tests defensive programming, not business requirements
    })
})
```

## 🔧 **When to Use Unit Tests**

### ✅ **Use Unit Tests For:**

#### 1. **Function/Method Behavior**
```go
// ✅ GOOD: Tests specific function behavior
Describe("ValidationEngine.ValidateWorkflow", func() {
    It("should detect circular dependencies", func() {
        workflow := createWorkflowWithCircularDeps()
        err := validator.ValidateWorkflow(workflow)
        Expect(err).To(MatchError(CircularDependencyError))
    })
})
```

#### 2. **Error Handling & Edge Cases**
```go
// ✅ GOOD: Tests error conditions
Describe("EffectivenessAssessor.ProcessPendingAssessments", func() {
    Context("when repository is unavailable", func() {
        It("should return repository error", func() {
            mockRepo.SetError("connection failed")
            err := assessor.ProcessPendingAssessments(ctx)
            Expect(err).To(MatchError(ContainSubstring("connection failed")))
        })
    })
})
```

#### 3. **Internal Logic Validation**
```go
// ✅ GOOD: Tests internal computation
Describe("calculateConfidenceAdjustment", func() {
    It("should reduce confidence proportionally to failure rate", func() {
        failureRate := 0.8
        adjustment := calculateConfidenceAdjustment(failureRate)
        Expect(adjustment).To(BeNumerically("<", 0))
    })
})
```

#### 4. **Interface Compliance**
```go
// ✅ GOOD: Tests interface contracts
Describe("MockEffectivenessRepository", func() {
    It("should implement EffectivenessRepository interface", func() {
        var repo EffectivenessRepository = NewMockEffectivenessRepository()
        Expect(repo).ToNot(BeNil())
    })
})
```

### ❌ **Don't Use Unit Tests For:**

#### 1. **Business Value Validation**
```go
// ❌ BAD: Tries to test business value with unit test
Describe("ProcessPendingAssessments", func() {
    It("should improve system accuracy", func() {
        // Business outcomes need business requirement tests
    })
})
```

#### 2. **End-to-End Workflows**
```go
// ❌ BAD: Complex integration in unit test
Describe("CompleteAlertResolution", func() {
    It("should process alert from detection to resolution", func() {
        // This belongs in business requirement or integration tests
    })
})
```

## 📋 **Testing Strategies by Component**

### AI & ML Components

#### Business Requirements Tests:
- Learning and adaptation over time
- Recommendation accuracy improvements
- Response time SLAs
- Business value delivery (cost reduction, time savings)

#### Unit Tests:
- Algorithm correctness
- Model training edge cases
- Data validation and preprocessing
- Error handling for invalid inputs

### Workflow Engine

#### Business Requirements Tests:
- End-to-end workflow execution
- Performance SLAs (30-second completion)
- Rollback and recovery capabilities
- Real Kubernetes operations

#### Unit Tests:
- Step validation logic
- Dependency resolution algorithms
- Error propagation between steps
- Configuration parsing

### Infrastructure & Platform

#### Business Requirements Tests:
- System scalability (handle 10K alerts/hour)
- Reliability and uptime requirements
- Performance under load
- Cost efficiency improvements

#### Unit Tests:
- Connection pool management
- Resource allocation algorithms
- Health check implementations
- Configuration validation

## 🔄 **Test Development Workflow**

### 1. **Start with Business Requirements**
```go
// Step 1: Define business requirement
Describe("BR-AI-001: System Must Learn From Failures", func() {
    // Define what business outcome is expected
})
```

### 2. **Build Supporting Unit Tests**
```go
// Step 2: Test the implementation that delivers business value
Describe("EffectivenessAssessor.ProcessPendingAssessments", func() {
    // Test the mechanics that make business requirement possible
})
```

### 3. **Validate Integration Points**
```go
// Step 3: Ensure components work together for business value
// (Integration tests or broader business requirement tests)
```

## 🎯 **Quality Gates**

### Business Requirement Tests Must:
- [ ] **Map to documented business requirements** (BR-XXX-### IDs)
- [ ] **Be understandable by non-technical stakeholders**
- [ ] **Measure business value** (accuracy, performance, cost)
- [ ] **Use realistic data and scenarios**
- [ ] **Validate end-to-end outcomes**
- [ ] **Include business success criteria**

### Unit Tests Must:
- [ ] **Focus on implementation correctness**
- [ ] **Execute quickly** (<100ms per test)
- [ ] **Have minimal external dependencies**
- [ ] **Test edge cases and error conditions**
- [ ] **Provide clear developer feedback**
- [ ] **Maintain high code coverage**

## 📊 **Success Metrics**

### Business Requirements Test Success:
- **90%** of tests validate business requirements rather than implementation
- **Business stakeholders** can understand test results
- **Business value** is measurable and tracked
- **SLA compliance** is validated continuously

### Unit Test Success:
- **95%** code coverage for critical components
- **<10ms** average test execution time
- **Fast feedback** for developers during development
- **Reliable detection** of implementation regressions

## 🚀 **Migration Strategy**

### Converting Existing Tests

#### 1. **Identify Test Purpose**
Ask: "What is this test really validating?"

#### 2. **Business Value Test → Keep as Business Requirement**
```go
// Keep in business-requirements/
It("should reduce alert noise by 80%", func() {
    // This validates business value
})
```

#### 3. **Implementation Test → Keep as Unit Test**
```go
// Keep in pkg/component/
It("should return error for invalid input", func() {
    // This validates implementation correctness
})
```

#### 4. **Mixed Tests → Split**
```go
// BEFORE: Mixed concerns
It("should process assessments and improve accuracy", func() {
    // Tests both implementation AND business value
})

// AFTER: Separated
// Unit Test:
It("should process assessments without error", func() {
    // Tests implementation
})

// Business Requirement Test:
It("should improve recommendation accuracy through learning", func() {
    // Tests business value
})
```

## 💡 **Pro Tips**

### 1. **Start with Business Requirements**
Always begin with "What business problem are we solving?" before writing code or tests.

### 2. **Use the Right Granularity**
- **Business tests**: Coarse-grained, end-to-end scenarios
- **Unit tests**: Fine-grained, focused on specific functions

### 3. **Choose Appropriate Mocks**
- **Business tests**: Realistic mocks that simulate real behavior
- **Unit tests**: Simple mocks that isolate the component under test

### 4. **LLM Mocking Policy (Cost Constraint)**

**E2E tests must use all real services EXCEPT the LLM.**

| Test Type | Infrastructure (DB, APIs) | LLM |
|-----------|---------------------------|-----|
| **Unit Tests** | Mock ✅ | Mock ✅ |
| **Integration Tests** | Mock ✅ | Mock ✅ |
| **E2E Tests** | **REAL** ❌ No mocking | Mock ✅ (cost) |

**Rationale**: LLM API calls incur significant costs per request. Mocking the LLM in E2E tests:
- Prevents runaway costs during test runs
- Allows deterministic, repeatable tests
- Still validates the complete integration with real infrastructure

**E2E Test Requirements**:
```python
# ✅ CORRECT: Real Data Storage, mock LLM only
@pytest.mark.e2e
def test_audit_events_persisted(data_storage_url, mock_llm):
    # data_storage_url → connects to REAL Data Storage service
    # mock_llm → mocked due to cost
    pass

# ❌ WRONG: Mocking infrastructure in E2E
@pytest.mark.e2e
def test_audit_events(mock_data_storage, mock_llm):
    # This is NOT an E2E test - it's an integration test
    pass
```

**If Data Storage is unavailable, E2E tests should FAIL, not skip.**

---

## 🚫 **Skip() is FORBIDDEN in Tests**

### Policy: Tests MUST Fail, Not Skip

**MANDATORY**: `Skip()` calls are **FORBIDDEN** in all test tiers. Tests must explicitly **FAIL** when required dependencies are unavailable.

#### Rationale

| Issue | Impact |
|-------|--------|
| **False confidence** | Skipped tests show "green" but don't validate anything |
| **Hidden dependencies** | Missing infrastructure goes undetected in CI |
| **Compliance gaps** | Audit tests skipped = audit not validated |
| **Silent failures** | Production issues not caught by test suite |

#### FORBIDDEN Patterns

```go
// ❌ FORBIDDEN: Skipping when service unavailable
BeforeEach(func() {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        Skip("Data Storage not available")  // ← FORBIDDEN
    }
})

// ❌ FORBIDDEN: Environment variable opt-out
if os.Getenv("SKIP_EXPENSIVE_TESTS") == "true" {
    Skip("Skipping expensive tests")  // ← FORBIDDEN
}

// ❌ FORBIDDEN: Skipping in integration/E2E tests
It("should persist audit events", func() {
    if !isDataStorageRunning() {
        Skip("DS not running")  // ← FORBIDDEN
    }
})
```

#### REQUIRED Patterns

```go
// ✅ REQUIRED: Fail with clear error message
BeforeEach(func() {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf(
            "REQUIRED: Data Storage not available at %s\n"+
            "Start with: podman-compose -f podman-compose.test.yml up -d",
            dataStorageURL))
    }
})

// ✅ REQUIRED: Assert dependency availability
It("should persist audit events", func() {
    Expect(isDataStorageRunning()).To(BeTrue(),
        "Data Storage REQUIRED - start infrastructure first")
    // ... test logic
})
```

#### Exception: Only ONE Acceptable Skip

The **ONLY** acceptable use of `Skip()` is in test files explicitly marked as **experimental** or **future work**:

```go
// ✅ ACCEPTABLE: Clearly marked experimental feature not yet implemented
var _ = Describe("Future Feature X", Label("experimental", "v2.0"), func() {
    BeforeEach(func() {
        Skip("Feature X not implemented - see ROADMAP.md")
    })
})
```

#### Enforcement

CI pipelines MUST:
1. **Fail builds** with any `Skip()` calls in non-experimental tests
2. **Report skipped tests** as errors, not warnings
3. **Block merges** with skipped compliance-critical tests

```bash
# CI check for forbidden Skip() usage
grep -r "Skip(" test/ --include="*_test.go" | \
  grep -v "experimental" | \
  grep -v "v2.0" && \
  echo "ERROR: Forbidden Skip() found" && exit 1
```

---

### 5. **Measure What Matters**
- **Business tests**: Business KPIs and stakeholder success criteria
- **Unit tests**: Technical correctness and edge case handling

### 6. **Make Tests Sustainable**
- **Business tests**: Should remain stable as business requirements are stable
- **Unit tests**: Should be fast and provide immediate developer feedback

## 🐳 **Integration Test Infrastructure**

### Podman Compose for Integration Tests

Integration tests require real service dependencies (HolmesGPT-API, Data Storage, PostgreSQL, Redis). Use `podman-compose` to spin up these services locally.

#### Available Infrastructure

| Service | Image | Port | Purpose |
|---------|-------|------|---------|
| **PostgreSQL** | `quay.io/jordigilh/pgvector:pg16` | 5432 | Audit trail storage (pgvector) |
| **Redis** | `quay.io/jordigilh/redis:7-alpine` | 6379 | Caching layer |
| **Data Storage** | Built from `docker/data-storage.Dockerfile` | 8080 | Audit persistence API |
| **HolmesGPT-API** | Built from `holmesgpt-api/Dockerfile` | 8081 | AI analysis service |

#### Running Integration Tests

```bash
# Start infrastructure (from project root)
podman-compose -f podman-compose.test.yml up -d

# Wait for services to be healthy
podman-compose -f podman-compose.test.yml ps

# Run integration tests
make test-container-integration

# Run specific service integration tests
go test ./test/integration/aianalysis/... -v

# Tear down
podman-compose -f podman-compose.test.yml down -v
```

#### Environment Configuration

Integration tests detect running services via environment variables:

```bash
# Set by podman-compose or manually for local development
export HOLMESGPT_API_URL=http://localhost:8081
export DATASTORAGE_URL=http://localhost:8080
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export REDIS_HOST=localhost
export REDIS_PORT=6379
```

#### Test Tier Infrastructure Matrix

| Test Tier | K8s Environment | Services | Infrastructure |
|-----------|-----------------|----------|----------------|
| **Unit** | None | Mocked | None required |
| **Integration** | envtest | Real (podman-compose) | `podman-compose.test.yml` |
| **E2E** | KIND cluster | Real (deployed to KIND) | KIND + Helm/manifests |

#### Mock LLM in All Tiers

**LLM is mocked across ALL test tiers** due to cost constraints. HolmesGPT-API uses its internal mock LLM server for deterministic responses.

```yaml
# podman-compose.test.yml - holmesgpt-api service
environment:
  - LLM_PROVIDER=mock
  - MOCK_LLM_ENABLED=true
```

---

## 🔐 **Kubeconfig Isolation Policy**

### E2E Test Kubeconfig Standard

**MANDATORY**: All E2E tests MUST use service-specific kubeconfig files to prevent conflicts and enable parallel test execution.

#### Naming Convention

| Element | Pattern | Example |
|---------|---------|---------|
| **Kubeconfig Path** | `~/.kube/{service}-e2e-config` | `~/.kube/gateway-e2e-config` |
| **Cluster Name** | `{service}-e2e` | `gateway-e2e` |
| **Environment Variable** | `KUBECONFIG=~/.kube/{service}-e2e-config` | - |

#### Service-Specific Paths

| Service | Kubeconfig Path | Cluster Name |
|---------|-----------------|--------------|
| Gateway | `~/.kube/gateway-e2e-config` | `gateway-e2e` |
| SignalProcessing | `~/.kube/signalprocessing-e2e-config` | `signalprocessing-e2e` |
| AIAnalysis | `~/.kube/aianalysis-e2e-config` | `aianalysis-e2e` |
| WorkflowExecution | `~/.kube/workflowexecution-e2e-config` | `workflowexecution-e2e` |
| Notification | `~/.kube/notification-e2e-config` | `notification-e2e` |
| DataStorage | `~/.kube/datastorage-e2e-config` | `datastorage-e2e` |
| RemediationOrchestrator | `~/.kube/ro-e2e-config` | `ro-e2e` |

#### Implementation Pattern

```go
// test/e2e/{service}/{service}_e2e_suite_test.go

var _ = SynchronizedBeforeSuite(func() []byte {
    homeDir, _ := os.UserHomeDir()

    // Standard kubeconfig location: ~/.kube/{service}-e2e-config
    // Per docs/development/business-requirements/TESTING_GUIDELINES.md
    kubeconfigPath := fmt.Sprintf("%s/.kube/%s-e2e-config", homeDir, serviceName)

    // Create Kind cluster with explicit kubeconfig path
    err := infrastructure.CreateCluster(clusterName, kubeconfigPath, GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())

    // Set KUBECONFIG environment variable
    os.Setenv("KUBECONFIG", kubeconfigPath)

    return []byte(kubeconfigPath)
}, func(data []byte) {
    kubeconfigPath = string(data)
    os.Setenv("KUBECONFIG", kubeconfigPath)
})
```

#### Shell Commands

```bash
# Create Kind cluster with explicit kubeconfig
kind create cluster \
  --name {service}-e2e \
  --config test/infrastructure/kind-{service}-config.yaml \
  --kubeconfig ~/.kube/{service}-e2e-config

# Set KUBECONFIG for subsequent commands
export KUBECONFIG=~/.kube/{service}-e2e-config

# Verify cluster access
kubectl cluster-info

# Cleanup
kind delete cluster --name {service}-e2e
rm -f ~/.kube/{service}-e2e-config
```

#### Why This Matters

1. **Isolation**: Prevents kubeconfig collisions when multiple E2E tests run on same machine
2. **Clarity**: Kubeconfig filename identifies which service owns it
3. **Safety**: Reduces risk of accidentally using wrong cluster credentials
4. **Discoverability**: Easy to list all E2E configs: `ls ~/.kube/*-e2e-config`
5. **Parallel Execution**: Multiple service E2E tests can run simultaneously

#### Anti-Patterns to Avoid

```go
// ❌ WRONG: Generic name that can conflict
kubeconfigPath = "~/.kube/kind-config"

// ❌ WRONG: Using cluster name instead of service name
kubeconfigPath = fmt.Sprintf("~/.kube/kind-%s", clusterName)

// ❌ WRONG: Hardcoded path without service identifier
kubeconfigPath = "/tmp/kubeconfig"

// ✅ CORRECT: Service-specific E2E config
kubeconfigPath = fmt.Sprintf("%s/.kube/%s-e2e-config", homeDir, serviceName)
```
