# Testing Guidelines: Business Requirements vs Unit Tests

**Version**: 2.5.0
**Last Updated**: 2025-12-26
**Status**: Active

This document provides clear guidance on **when** and **how** to use each type of test in the kubernaut system.

---

## 📋 **Changelog**

### Version 2.5.0 (2025-12-26)
- **CRITICAL**: Added ANTI-PATTERN section for direct audit infrastructure testing
- **DISCOVERED**: System-wide triage found 21+ tests across 3 services following wrong pattern
- **DOCUMENTED**: Correct pattern (business logic with audit side effects) vs wrong pattern (direct audit store calls)
- **EXAMPLES**: Real-world correct implementations (SignalProcessing, Gateway) and deleted wrong examples
- **REFERENCE**: Links to [Audit Infrastructure Testing Anti-Pattern Triage](../../handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md)
- **ENFORCEMENT**: CI check guidance to detect wrong pattern in code review

### Version 2.4.0 (2025-12-22)
- **ADDED**: Code Coverage Targets section (70%/50%/50% empirically validated)
- **CLARIFIED**: BR Coverage vs Code Coverage distinction
- **EMPIRICAL DATA**: DataStorage and SignalProcessing demonstrate E2E achieves 50%+ code coverage
- **KEY INSIGHT**: 50%+ of codebase tested in ALL 3 tiers with overlapping defense layers
- **UPDATED**: Coverage targets table with BR coverage (overlapping) and code coverage (cumulative)

### Version 2.3.0 (2025-12-21)
- **ADDED**: E2E Coverage Collection section (Go 1.20+ binary coverage profiling)
- **REFERENCE**: New procedural guide `E2E_COVERAGE_COLLECTION.md` in `docs/development/testing/`
- **TARGET**: Updated E2E code coverage from 10-15% to 50% based on empirical data

### Version 2.2.0 (2025-12-21)
- **CRITICAL**: Added `podman-compose` race condition warning for integration tests
- **RECOMMENDED**: Sequential startup pattern for multi-service dependencies
- **DOCUMENTED**: Known issues affecting RO, NT, and potentially other services
- **ADDED**: Root cause analysis and working solution from DS team
- **REFERENCE**: Links to infrastructure debugging documents

### Version 2.0.0 (2025-12-13)
- **BREAKING**: Added mandatory anti-pattern for `time.Sleep()` in tests
- **ADDED**: Synchronization anti-patterns section
- **REQUIRED**: All tests MUST use `Eventually()` for waiting on asynchronous operations
- **FORBIDDEN**: `time.Sleep()` is now absolutely forbidden in all test tiers

### Version 1.0.0 (Initial)
- Initial testing guidelines
- Business requirement vs unit test decision framework
- Skip() forbidden policy
- Integration test infrastructure guidelines
- Kubeconfig isolation policy

---

## 🎯 **Defense-in-Depth Testing Strategy**

### Coverage Targets: BR Coverage vs Code Coverage

Kubernaut uses **defense-in-depth** with overlapping BR coverage and cumulative code coverage:

#### Business Requirement (BR) Coverage - OVERLAPPING

| Tier | BR Coverage Target | Purpose |
|------|-------------------|---------|
| **Unit** | **70%+ of ALL BRs** | Ensure all unit-testable business requirements implemented |
| **Integration** | **>50% of ALL BRs** | Validate cross-service coordination and CRD operations |
| **E2E** | **<10% BR coverage** | Critical user journeys only |

**Key**: Same BRs tested at multiple tiers (e.g., retry logic in unit, integration, AND E2E)

#### Code Coverage - CUMULATIVE (~100% combined)

| Tier | Code Coverage Target | What It Validates |
|------|---------------------|-------------------|
| **Unit** | **70%+** | Algorithm correctness, edge cases, error handling |
| **Integration** | **50%** | Cross-component flows, CRD operations, real K8s API |
| **E2E** | **50%** | Full stack: main.go, reconciliation, business logic, metrics, audit |

**Empirical Validation**: DataStorage and SignalProcessing services demonstrate E2E tests achieve **50%+ code coverage** due to full stack execution.

**Key Insight**: With 70%/50%/50% code coverage targets, **50%+ of codebase is tested in ALL 3 tiers** - ensuring bugs must slip through multiple defense layers to reach production.

**Example**: Retry logic (BR-NOT-052)
- **Unit (70%)**: Algorithm correctness (30s → 480s exponential backoff) - tests `pkg/notification/retry/policy.go`
- **Integration (50%)**: Real K8s reconciliation loop - tests same code with envtest
- **E2E (50%)**: Deployed controller in Kind - tests same code in production-like environment

If the exponential backoff calculation has a bug, it must slip through **ALL 3 defense layers** to reach production!

---

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
- [ ] **Use test case IDs from test plan** (if test plan exists) OR **map to BR-XXX-XXX**

#### Test Case ID Convention

When a **test plan document exists** for a service (e.g., `docs/services/{service}/unit-test-plan.md`), tests SHOULD use the test case ID from the plan instead of BR IDs in test descriptions:

```go
// ✅ PREFERRED: Test case ID from test plan (when plan exists)
Context("ENRICH-DS-01: enriching DaemonSet signal", func() {
    It("should enrich DaemonSet with full context", func() {
        // Test plan documents BR mapping: ENRICH-DS-01 → BR-SP-001
    })
})

// ✅ ALSO VALID: Direct BR mapping (when no test plan exists)
Context("BR-SP-001: K8s enrichment", func() {
    It("should enrich DaemonSet with full context", func() {})
})
```

**Rationale**: Test plans provide traceability between test cases and BRs. Using test case IDs:
- Keeps test descriptions concise
- Centralizes BR mapping in the test plan document
- Makes test plan updates easier without modifying test code

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

### 5. **Metrics Testing Strategy by Tier**

**Per DD-TEST-001**: CRD Controllers use envtest (no HTTP server). Metrics testing strategy differs by tier.

**Per DD-METRICS-001**: CRD Controllers MUST use dependency-injected metrics with `NewMetricsWithRegistry()` for test isolation. Stateless services SHOULD also support `NewMetricsWithRegistry()` for test isolation (same principle, adapted for HTTP services).

**References**:
- [DD-METRICS-001: Controller Metrics Wiring Pattern](../../architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md) (CRD controllers)
- Stateless services use similar pattern with `promauto.With(registry)` instead of controller-runtime registry

| Test Tier | Metrics Testing Approach | Infrastructure | Metrics Creation |
|-----------|--------------------------|----------------|------------------|
| **Unit** | Registry inspection (metric exists, naming, types) | Fresh Prometheus registry | `NewMetricsWithRegistry(testRegistry)` |
| **Integration** | Registry inspection (metric values after operations) | controller-runtime registry | `NewMetricsWithRegistry(testRegistry)` |
| **E2E** | HTTP endpoint (`/metrics` accessible) | Deployed controller with Service | `NewMetrics()` (production) |

#### CRD Controllers (AIAnalysis, Notification, RO, etc.)

```go
// ✅ CORRECT: Integration test - verify via registry inspection (NO HTTP server)
// Per DD-METRICS-001: Use test-specific registry for isolation
It("should register all business metrics", func() {
    // Create test-specific registry (DD-METRICS-001)
    testRegistry := prometheus.NewRegistry()
    testMetrics := metrics.NewMetricsWithRegistry(testRegistry)

    // Record metrics via injected metrics instance
    testMetrics.RecordReconciliation("Pending", "success")

    // Verify via registry inspection (NOT HTTP endpoint)
    families, err := testRegistry.Gather()
    Expect(err).ToNot(HaveOccurred())

    // Check metric exists
    found := false
    for _, family := range families {
        if family.GetName() == "aianalysis_reconciler_reconciliations_total" {
            found = true
            break
        }
    }
    Expect(found).To(BeTrue())
})

// ❌ WRONG: Using global controller-runtime registry in tests
It("should register all business metrics", func() {
    metrics.RecordReconciliation("Pending", "success")  // ❌ Global metrics

    // ❌ Pollutes global registry, causes test interference
    families, err := ctrlmetrics.Registry.Gather()
    Expect(err).ToNot(HaveOccurred())
})

// ❌ WRONG: Starting HTTP server in integration test for CRD controller
BeforeAll(func() {
    // This violates DD-TEST-001: No integration ports for CRD controllers
    metricsServer = &http.Server{Addr: ":19184"}  // ❌
})
```

#### Stateless Services (Data Storage, Gateway, HolmesGPT-API)

```go
// ✅ CORRECT: Unit test - test-specific registry (similar to CRD controllers)
It("should register all business metrics", func() {
    // Create test-specific registry for isolation
    testRegistry := prometheus.NewRegistry()
    testMetrics := metrics.NewMetricsWithRegistry("datastorage", "api", testRegistry)

    // Record metrics via injected metrics instance
    testMetrics.AuditTracesTotal.WithLabelValues("signalprocessing", "success").Inc()

    // Verify via registry inspection
    families, err := testRegistry.Gather()
    Expect(err).ToNot(HaveOccurred())

    found := false
    for _, family := range families {
        if family.GetName() == "datastorage_api_audit_traces_total" {
            found = true
            break
        }
    }
    Expect(found).To(BeTrue())
})

// ✅ CORRECT: Integration test - HTTP endpoint available via podman-compose
It("should expose metrics endpoint", func() {
    // Service runs in container with port mapping (per DD-TEST-001)
    resp, err := http.Get("http://localhost:18090/metrics")  // Data Storage port
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(200))
})
```

**Rationale**:
- **CRD Controllers**: Use envtest (no HTTP server) → verify metrics via registry
- **Stateless Services**: Use test registry for units, HTTP for integration
- **Both Service Types**: Use `NewMetricsWithRegistry()` for test isolation
- **E2E (Both)**: Deploy to Kind cluster → verify metrics via NodePort

---

## 🚫 **time.Sleep() is ABSOLUTELY FORBIDDEN in Tests**

### Policy: Tests MUST Use Eventually(), NEVER time.Sleep()

**MANDATORY**: `time.Sleep()` calls are **ABSOLUTELY FORBIDDEN** in ALL test tiers for waiting on asynchronous operations, with **NO EXCEPTIONS**.

#### Rationale

| Issue | Impact |
|-------|--------|
| **Flaky tests** | Fixed sleep durations cause intermittent failures |
| **Slow tests** | Always wait full duration even if condition met earlier |
| **Race conditions** | Sleep doesn't guarantee condition is met |
| **CI instability** | Different machine speeds cause test failures |
| **False confidence** | Tests pass locally but fail in CI |
| **Poor debugging** | No clear feedback on what condition failed |

**Key Insight**: `time.Sleep()` represents a **guess** about timing. `Eventually()` represents a **verification** that a condition is met.

#### FORBIDDEN Patterns (NO EXCEPTIONS)

```go
// ❌ FORBIDDEN: Sleeping to wait for CRD creation
time.Sleep(5 * time.Second)
err := k8sClient.Get(ctx, key, &crd)
Expect(err).ToNot(HaveOccurred())

// ❌ FORBIDDEN: Sleeping to wait for status update
time.Sleep(2 * time.Second)
Expect(crd.Status.Phase).To(Equal("Ready"))

// ❌ FORBIDDEN: Sleeping to wait for processing
for i := 0; i < 20; i++ {
    time.Sleep(50 * time.Millisecond)
    // send request
}
time.Sleep(5 * time.Second)  // Wait for all to process

// ❌ FORBIDDEN: Sleeping for cache synchronization
time.Sleep(100 * time.Millisecond)
list := k8sClient.List(ctx, &crdList)

// ❌ FORBIDDEN: Even short sleeps for "stability"
time.Sleep(10 * time.Millisecond)  // "Let the cache settle"
```

#### REQUIRED Patterns

```go
// ✅ REQUIRED: Eventually() for CRD creation
Eventually(func() error {
    return k8sClient.Get(ctx, key, &crd)
}, 30*time.Second, 1*time.Second).Should(Succeed())

// ✅ REQUIRED: Eventually() for status updates
Eventually(func() string {
    _ = k8sClient.Get(ctx, key, &crd)
    return crd.Status.Phase
}, 30*time.Second, 1*time.Second).Should(Equal("Ready"))

// ✅ REQUIRED: Eventually() for list operations
Eventually(func() int {
    _ = k8sClient.List(ctx, &crdList, client.InNamespace(ns))
    return len(crdList.Items)
}, 30*time.Second, 1*time.Second).Should(BeNumerically(">", 0))

// ✅ REQUIRED: Eventually() with custom matcher
Eventually(func() *remediationv1alpha1.RemediationRequest {
    var crdList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &crdList, client.InNamespace(ns))
    if err != nil {
        return nil
    }

    for i := range crdList.Items {
        if crdList.Items[i].Spec.SignalLabels["process_id"] == processID {
            return &crdList.Items[i]
        }
    }
    return nil
}, 30*time.Second, 1*time.Second).ShouldNot(BeNil(),
    "Should find RemediationRequest with process_id=%s", processID)
```

#### Eventually() Best Practices

```go
// ✅ Pattern 1: Simple condition check
Eventually(func() bool {
    // Check condition, return true when met
    return condition
}, timeout, interval).Should(BeTrue())

// ✅ Pattern 2: Get and check in one
Eventually(func() string {
    obj := &MyType{}
    _ = k8sClient.Get(ctx, key, obj)  // Ignore error, Eventually will retry
    return obj.Status.Phase
}, timeout, interval).Should(Equal("Ready"))

// ✅ Pattern 3: Complex object search
Eventually(func() *MyType {
    var list MyTypeList
    if err := k8sClient.List(ctx, &list); err != nil {
        return nil  // Return nil on error, Eventually will retry
    }

    for i := range list.Items {
        if list.Items[i].MatchesCriteria() {
            return &list.Items[i]
        }
    }
    return nil
}, timeout, interval).ShouldNot(BeNil())

// ✅ Pattern 4: Count-based conditions
Eventually(func() int {
    var list MyTypeList
    _ = k8sClient.List(ctx, &list)
    return len(list.Items)
}, timeout, interval).Should(BeNumerically(">=", expectedCount))
```

#### Timeout Configuration Guidelines

| Test Tier | Typical Timeout | Interval | Rationale |
|-----------|-----------------|----------|-----------|
| **Unit** | 1-5 seconds | 10-100ms | Fast, no I/O |
| **Integration** | 30-60 seconds | 1-2 seconds | Real K8s API, slower |
| **E2E** | 2-5 minutes | 5-10 seconds | Full infrastructure |

```go
// Unit tests: Fast, in-memory operations
Eventually(func() bool {
    return condition
}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

// Integration tests: Real K8s API calls
Eventually(func() error {
    return k8sClient.Get(ctx, key, &obj)
}, 30*time.Second, 1*time.Second).Should(Succeed())

// E2E tests: Full infrastructure with network delays
Eventually(func() bool {
    return checkDeploymentReady(namespace, deployment)
}, 5*time.Minute, 10*time.Second).Should(BeTrue())
```

#### Why No Exceptions?

1. **Reliability**: `Eventually()` retries until condition is met (up to timeout)
2. **Speed**: Returns immediately when condition is satisfied (no unnecessary waiting)
3. **Clarity**: Test failure messages show what condition was not met
4. **Debugging**: Clear timeout vs condition failure distinction
5. **CI Stability**: Works across different machine speeds

#### Acceptable Use of time.Sleep()

**ONLY acceptable in these specific scenarios:**

```go
// ✅ Acceptable: Rate limiting test
It("should rate limit requests", func() {
    start := time.Now()
    // Make requests
    duration := time.Since(start)
    Expect(duration).To(BeNumerically(">=", expectedMinDuration))
})

// ✅ Acceptable: Testing timing behavior
It("should timeout after 5 seconds", func() {
    start := time.Now()
    err := operationWithTimeout(5 * time.Second)
    duration := time.Since(start)
    Expect(duration).To(BeNumerically("~", 5*time.Second, 500*time.Millisecond))
})

// ✅ Acceptable: Staggering requests for specific test scenario
for i := 0; i < 20; i++ {
    time.Sleep(50 * time.Millisecond)  // Intentional stagger
    sendRequest()  // Create storm scenario
}
// But then use Eventually() to wait for processing!
Eventually(func() bool {
    return allRequestsProcessed()
}, 30*time.Second, 1*time.Second).Should(BeTrue())
```

**Rule**: `time.Sleep()` is ONLY acceptable when testing timing behavior itself, NEVER for waiting on asynchronous operations.

#### Best Practice: Configuration-Based Timing (Test 14 Example)

**Real-World Example**: Gateway E2E Test 14 demonstrates proper configuration-based timing:

```go
// ✅ BEST PRACTICE: Align sleep with actual configuration
// test/e2e/gateway/14_deduplication_ttl_expiration_test.go

// Note: E2E environment uses 10s TTL (minimum allowed per config validation)
// See: test/e2e/gateway/gateway-deployment.yaml and pkg/gateway/config/config.go:368
// Production uses 5m TTL. This test validates TTL expiration behavior.
testLogger.Info("Step 3: Wait for deduplication TTL to expire")
testLogger.Info("  Waiting 15 seconds for TTL expiration (10s E2E TTL + 5s buffer)...")
time.Sleep(15 * time.Second) // E2E TTL is 10s (see gateway-deployment.yaml), 5s buffer for clock skew
```

**Key Principles**:
1. **Configuration-Driven**: Sleep duration derived from actual E2E config (`DEDUPLICATION_TTL=10s`)
2. **Documented Reasoning**: Comments explain why this duration and reference configuration sources
3. **Environment-Aware**: Acknowledges production uses different TTL (5m)
4. **Buffer Calculation**: Explicit buffer for clock skew and Kubernetes eventual consistency
5. **Traceability**: Points to config files (`gateway-deployment.yaml`) and validation code

**Impact**: Reduced Test 14 execution time from 70s to 15s (73% faster) while maintaining correctness.

**Common Mistake**: Hardcoding arbitrary durations without configuration backing:
```go
// ❌ BAD: Arbitrary hardcoded wait
time.Sleep(70 * time.Second) // "Wait for TTL" - based on incorrect assumption

// ✅ GOOD: Configuration-backed wait with clear reasoning
time.Sleep(15 * time.Second) // E2E TTL is 10s (see gateway-deployment.yaml), 5s buffer
```

#### Enforcement

CI pipelines MUST:
1. **Detect** `time.Sleep()` followed by assertions
2. **Flag** suspicious patterns in code review
3. **Require** justification for any `time.Sleep()` usage

```bash
# CI check for forbidden time.Sleep() patterns
# Allow: time.Sleep in request staggering or timing tests
# Forbid: time.Sleep before assertions or API calls
if grep -A 5 "time\.Sleep" test/ --include="*_test.go" | grep -E "Expect|Should|Get|List|Create|Update" | grep -v "^Binary"; then
    echo "⚠️  WARNING: Detected time.Sleep() before assertions/API calls"
    echo "   Review for anti-pattern: Use Eventually() instead"
    echo "   See: docs/development/business-requirements/TESTING_GUIDELINES.md"
fi
```

#### Linter Rule

Add to `.golangci.yml`:
```yaml
linters-settings:
  forbidigo:
    forbid:
      - pattern: 'time\.Sleep\([^)]+\)\s*\n\s*(Expect|Should|err\s*:?=)'
        msg: "time.Sleep() before assertions is forbidden - use Eventually() for async operations"
```

#### Migration Examples

```go
// BEFORE (❌ Anti-pattern)
for i := 1; i <= 20; i++ {
    time.Sleep(50 * time.Millisecond)
    sendAlert(i)
}
time.Sleep(5 * time.Second)  // Wait for processing

err := k8sClient.List(ctx, &crdList, client.InNamespace(ns))
Expect(err).ToNot(HaveOccurred())

// AFTER (✅ Correct pattern)
for i := 1; i <= 20; i++ {
    time.Sleep(50 * time.Millisecond)  // Acceptable: intentional stagger
    sendAlert(i)
}

// Use Eventually() to wait for processing completion
Eventually(func() int {
    var crdList remediationv1alpha1.RemediationRequestList
    _ = k8sClient.List(ctx, &crdList, client.InNamespace(ns))
    return len(crdList.Items)
}, 30*time.Second, 1*time.Second).Should(BeNumerically(">", 0))
```

---

## 🚫 **Skip() is ABSOLUTELY FORBIDDEN in Tests**

### Policy: Tests MUST Fail, NEVER Skip

**MANDATORY**: `Skip()` calls are **ABSOLUTELY FORBIDDEN** in ALL test tiers, with **NO EXCEPTIONS**.

#### Rationale

| Issue | Impact |
|-------|--------|
| **False confidence** | Skipped tests show "green" but don't validate anything |
| **Hidden dependencies** | Missing infrastructure goes undetected in CI |
| **Compliance gaps** | Audit tests skipped = audit not validated |
| **Silent failures** | Production issues not caught by test suite |
| **Architectural violations** | Services running without required dependencies |

**Key Insight**: If a service can run without a dependency, that dependency is optional. If it's required (like Data Storage for audit compliance per DD-AUDIT-003), then tests MUST fail when it's unavailable.

#### FORBIDDEN Patterns (NO EXCEPTIONS)

```go
// ❌ FORBIDDEN: Skipping when service unavailable
BeforeEach(func() {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        Skip("Data Storage not available")  // ← FORBIDDEN
    }
})

// ❌ FORBIDDEN: Environment variable opt-out
if os.Getenv("SKIP_DATASTORAGE_TESTS") == "true" {
    Skip("Skipping Data Storage tests")  // ← FORBIDDEN
}

// ❌ FORBIDDEN: Skipping in integration/E2E tests
It("should persist audit events", func() {
    if !isDataStorageRunning() {
        Skip("DS not running")  // ← FORBIDDEN
    }
})

// ❌ FORBIDDEN: Even "experimental" or "future work" skips
var _ = Describe("Future Feature X", func() {
    BeforeEach(func() {
        Skip("Feature X not implemented")  // ← FORBIDDEN - use Pending() or don't write the test
    })
})

// ❌ FORBIDDEN: Conditional skips based on availability
if !dsAvailable {
    Skip("Data Storage not available")  // ← FORBIDDEN
}
```

#### REQUIRED Patterns

```go
// ✅ REQUIRED: Fail with clear error message
BeforeEach(func() {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf(
            "REQUIRED: Data Storage not available at %s\n"+
            "  Per DD-AUDIT-003: This service MUST have audit capability\n"+
            "  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n\n"+
            "  Start with: podman-compose -f podman-compose.test.yml up -d",
            dataStorageURL))
    }
})

// ✅ REQUIRED: Assert dependency availability
It("should persist audit events", func() {
    Expect(isDataStorageRunning()).To(BeTrue(),
        "Data Storage REQUIRED - start infrastructure first")
    // ... test logic
})

// ✅ REQUIRED: For unimplemented features, use Pending() or PDescribe()
PDescribe("Future Feature X", func() {
    // Pending tests are clearly marked as not-yet-implemented
    // They show up as "Pending" (yellow) not "Passed" (green)
})
```

#### Why No Exceptions?

1. **Architectural Enforcement**: If WorkflowExecution can run without Data Storage, audit is effectively optional - which violates DD-AUDIT-003
2. **CI Integrity**: Skipped tests in CI mean features are not validated
3. **Developer Discipline**: Forces proper infrastructure setup before running tests
4. **Compliance**: Audit trails are compliance-critical - can't be skipped

#### Alternatives to Skip()

| Instead of Skip() | Use This |
|-------------------|----------|
| Feature not implemented | `PDescribe()` / `PIt()` (Pending) |
| Dependency unavailable | `Fail()` with clear error message |
| Expensive test | Run in separate CI job, don't skip |
| Flaky test | Fix it or mark with `FlakeAttempts()` |
| Platform-specific | Use build tags (`// +build linux`) |

#### Enforcement

CI pipelines MUST:
1. **Fail builds** with ANY `Skip()` calls in test files
2. **Report skipped tests** as build failures
3. **Block merges** with any `Skip()` usage

```bash
# CI check for forbidden Skip() usage - NO EXCEPTIONS
if grep -r "Skip(" test/ --include="*_test.go" | grep -v "^Binary"; then
    echo "❌ ERROR: Skip() is ABSOLUTELY FORBIDDEN in tests"
    echo "   Use Fail() for missing dependencies"
    echo "   Use PDescribe()/PIt() for unimplemented features"
    exit 1
fi
```

#### Linter Rule

Add to `.golangci.yml`:
```yaml
linters-settings:
  forbidigo:
    forbid:
      - pattern: 'ginkgo\.Skip\('
        msg: "Skip() is forbidden - use Fail() for missing deps, PDescribe() for unimplemented"
      - pattern: '\.Skip\('
        msg: "Skip() is forbidden - use Fail() for missing deps, PDescribe() for unimplemented"
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

### ⚠️ **CRITICAL: `podman-compose` Race Condition Warning**

> **📋 Authoritative Reference**: See **DD-TEST-002** for full decision documentation
> **🛠️ Implementation Guide**: See `docs/development/testing/INTEGRATION_TEST_INFRASTRUCTURE_SETUP.md`

**Status**: 🔴 **KNOWN ISSUE** - Affects multiple services (RO, NT, potentially others)
**Severity**: HIGH - Causes container crashes (exit 137 - SIGKILL)
**Root Cause Identified**: December 20, 2025 by DataStorage team
**Decision Documented**: December 21, 2025 in DD-TEST-002

#### The Problem

`podman-compose up -d` starts **all services simultaneously**, causing race conditions when services have startup dependencies:

```bash
podman-compose up -d
  ├── PostgreSQL starts (takes 10-15s to be ready) ⏱️
  ├── Redis starts (takes 2-3s to be ready) ⏱️
  └── DataStorage starts (tries to connect IMMEDIATELY) ⚡
      ↓
      ❌ Connection fails (PostgreSQL not ready yet)
      ↓
      🔄 Container crashes and restarts repeatedly
      ↓
      💀 Podman kills after restart limit → SIGKILL (exit 137)
```

**Symptoms**:
- Health checks fail with "lookup postgres: no such host"
- Containers show "Up (healthy)" but HTTP server never starts
- Exit code 137 (SIGKILL) after repeated failures
- DNS resolution errors within podman network

**Why `depends_on: service_healthy` Doesn't Work**:
```yaml
# ❌ THIS IS IGNORED BY PODMAN-COMPOSE:
datastorage:
  depends_on:
    postgres:
      condition: service_healthy  # Podman-compose doesn't respect this
```

#### ✅ **Recommended Solution: Sequential Startup**

**DO NOT use `podman-compose` for services with startup dependencies.**

Instead, use **sequential `podman run` commands** with explicit health checks:

```bash
#!/bin/bash
# test/integration/{service}/setup-infrastructure.sh

set -e

# 1. Stop any existing containers
podman stop {service}_postgres_1 {service}_redis_1 {service}_datastorage_1 2>/dev/null || true
podman rm {service}_postgres_1 {service}_redis_1 {service}_datastorage_1 2>/dev/null || true

# 2. Create network
podman network create {service}_test-network 2>/dev/null || true

# 3. Start PostgreSQL FIRST
echo "Starting PostgreSQL..."
podman run -d \
  --name {service}_postgres_1 \
  --network {service}_test-network \
  -p 15432:5432 \
  -e POSTGRES_USER=slm_user \
  -e POSTGRES_PASSWORD=test_password \
  -e POSTGRES_DB=test_db \
  postgres:16-alpine

# 4. WAIT for PostgreSQL to be ready (critical!)
echo "Waiting for PostgreSQL..."
for i in {1..30}; do
  podman exec {service}_postgres_1 pg_isready -U slm_user && break
  sleep 1
done

# 5. Start Redis SECOND
echo "Starting Redis..."
podman run -d \
  --name {service}_redis_1 \
  --network {service}_test-network \
  -p 16379:6379 \
  redis:7-alpine

# 6. WAIT for Redis to be ready
echo "Waiting for Redis..."
for i in {1..10}; do
  podman exec {service}_redis_1 redis-cli ping | grep -q PONG && break
  sleep 1
done

# 7. Start DataStorage LAST
echo "Starting DataStorage..."
podman run -d \
  --name {service}_datastorage_1 \
  --network {service}_test-network \
  -p 18080:8080 \
  -e DB_HOST={service}_postgres_1 \
  -e REDIS_HOST={service}_redis_1 \
  datastorage:latest

# 8. WAIT for DataStorage health check
echo "Waiting for DataStorage..."
for i in {1..30}; do
  curl -s http://127.0.0.1:18080/health | grep -q "ok" && break
  sleep 1
done

echo "✅ Infrastructure ready!"
```

**Update your test suite's BeforeSuite**:

```go
// Use Eventually() with 30s timeout (DS team proven pattern)
Eventually(func() int {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        GinkgoWriter.Printf("  Health check failed: %v\n", err)
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK),
    "DataStorage should be healthy within 30 seconds")
```

**Why 30 seconds?**: Cold start on macOS Podman can take 15-20 seconds.

#### 📊 **Affected Services**

| Service | Status | Notes |
|---------|--------|-------|
| **DataStorage** | ✅ FIXED | Uses sequential startup (Dec 20, 2025) |
| **RemediationOrchestrator** | ⚠️ KNOWN ISSUE | Documented in SHARED_RO_DS_INTEGRATION_DEBUG_DEC_20_2025.md |
| **Notification** | ⚠️ KNOWN ISSUE | Documented in NT_INTEGRATION_TEST_INFRASTRUCTURE_ISSUES_DEC_21_2025.md |
| **Other Services** | ⚠️ AT RISK | Review if using `podman-compose` with multi-service dependencies |

#### 🎯 **When `podman-compose` is Safe**

`podman-compose` is acceptable for:
- ✅ **Single-service setups** (no startup dependencies)
- ✅ **Developer convenience** for local testing (not CI)
- ✅ **E2E tests using Kind clusters** (different orchestration)

`podman-compose` is **NOT safe** for:
- ❌ **Multi-service integration tests** with startup dependencies
- ❌ **CI/CD pipelines** requiring deterministic startup
- ❌ **Services that connect to databases** at initialization

#### 📚 **References**

**Authoritative Documents**:
- **DD-TEST-002**: Integration Test Container Orchestration Pattern (Authoritative decision)
  - `docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md`
- **Implementation Guide**: `docs/development/testing/INTEGRATION_TEST_INFRASTRUCTURE_SETUP.md`

**Historical Debugging Sessions** (for context):
- RO Team: `docs/handoff/SHARED_RO_DS_INTEGRATION_DEBUG_DEC_20_2025.md` (Dec 20, 2025)
- NT Team: `docs/handoff/NT_INTEGRATION_TEST_INFRASTRUCTURE_ISSUES_DEC_21_2025.md` (Dec 21, 2025)

**Working Implementation**:
- DataStorage: `test/integration/datastorage/suite_test.go` (sequential startup reference)

**Issue Timeline**: December 20-21, 2025
**Teams Affected**: DataStorage (fixed), RemediationOrchestrator (pending), Notification (pending)

---

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

---

## 📊 **E2E Coverage Collection (Go 1.20+)**

### Overview

Go 1.20+ supports coverage profiling for compiled binaries, enabling E2E coverage measurement for controllers running in Kind clusters.

**Target**: 10-15% E2E coverage (validates deployment wiring and critical paths)

**Implementation Guide**: See [E2E_COVERAGE_COLLECTION.md](../testing/E2E_COVERAGE_COLLECTION.md)

### Quick Reference

```bash
# Build with coverage
GOFLAGS=-cover go build -o bin/{service}-controller ./cmd/{service}/

# Run E2E tests (controller writes to GOCOVERDIR)
make test-e2e-{service}

# Extract coverage after graceful shutdown
go tool covdata percent -i=./coverdata
go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt
go tool cover -html=e2e-coverage.txt -o e2e-coverage.html
```

**Key Requirements**:
1. Build binary with `GOFLAGS=-cover`
2. Set `GOCOVERDIR=/coverdata` in controller deployment
3. Mount hostPath volume from Kind node to container
4. **Graceful shutdown** (scale to 0) before extracting coverage

---

## 🎯 **V1.0 Service Maturity Testing Requirements** ⭐ NEW (v2.1.0)

### Version History

| Version | Date | Changes |
|---------|------|---------|
| v2.1.0 | 2025-12-19 | Added V1.0 Service Maturity Testing Requirements |

### Overview

**MANDATORY**: All services must have tests that verify V1.0 maturity features. A service without these tests is **NOT** considered production-ready.

**Reference Documents**:
- [V1.0 Service Maturity Triage](../../handoff/V1_0_SERVICE_MATURITY_TRIAGE_DEC_19_2025.md)
- [Service Implementation Template](../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#v10-mandatory-maturity-checklist)
- [V1.0 Maturity Test Plan Template](../testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

> ⚠️ **Living Document Notice**: This section is a living document. Developers MUST triage new ADRs and DDs created after this update for additional testing requirements. If found, they MUST be added to this guideline and communicated via handoff.

---

### 📐 **Metrics Naming Convention (DD-005)**

**Reference**: [DD-005: Observability Standards](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)

**Format**: `{service}_{component}_{metric_name}_{unit}`

| Element | Description | Examples |
|---------|-------------|----------|
| `{service}` | Service prefix | `signalprocessing_`, `workflowexecution_`, `gateway_` |
| `{component}` | Logical component | `reconciler_`, `http_`, `enricher_`, `audit_` |
| `{metric_name}` | Descriptive name | `reconciliations`, `requests`, `duration` |
| `{unit}` | Unit suffix (optional) | `_total`, `_seconds`, `_bytes` |

**Examples by Service Type**:

```go
// CRD Controller Examples
signalprocessing_reconciler_reconciliations_total{phase="Completed", result="success"}
signalprocessing_enricher_duration_seconds{resource_kind="Pod"}
workflowexecution_reconciler_pipelinerun_creations_total
aianalysis_reconciler_analysis_duration_seconds{phase="Investigating"}

// Stateless HTTP Examples
gateway_http_requests_total{method="POST", path="/api/v1/signals", status="201"}
gateway_http_request_duration_seconds{method="POST", path="/api/v1/signals"}
datastorage_audit_events_written_total{service="signalprocessing"}
```

**Anti-Patterns to Avoid**:

```go
// ❌ WRONG: Inconsistent naming
signalprocessing_processing_total        // Missing component
processing_duration_seconds              // Missing service prefix
sp_rec_dur_sec                           // Abbreviations

// ✅ CORRECT: Consistent naming
signalprocessing_reconciler_processing_total
signalprocessing_reconciler_duration_seconds
```

---

### 🎫 **Standard EventRecorder Events (CRD Controllers)**

All CRD controllers MUST emit Kubernetes Events using these standard reasons:

| Event Reason | Event Type | When to Emit | Message Pattern |
|--------------|------------|--------------|-----------------|
| `ReconcileStarted` | Normal | Reconciliation begins | "Started reconciling {resource}" |
| `ReconcileComplete` | Normal | Reconciliation succeeds | "Successfully reconciled {resource}" |
| `ReconcileFailed` | Warning | Reconciliation fails | "Failed to reconcile: {error}" |
| `PhaseTransition` | Normal | Phase changes | "Transitioned from {old} to {new}" |
| `ValidationFailed` | Warning | Spec validation fails | "Validation failed: {reason}" |
| `DependencyMissing` | Warning | Required resource missing | "Dependency not found: {name}" |

**Implementation Pattern**:

```go
// pkg/shared/events/reasons.go (or inline in controller)
const (
    EventReasonReconcileStarted   = "ReconcileStarted"
    EventReasonReconcileComplete  = "ReconcileComplete"
    EventReasonReconcileFailed    = "ReconcileFailed"
    EventReasonPhaseTransition    = "PhaseTransition"
    EventReasonValidationFailed   = "ValidationFailed"
    EventReasonDependencyMissing  = "DependencyMissing"
)

// Usage in controller
r.Recorder.Event(obj, corev1.EventTypeNormal, EventReasonReconcileStarted,
    fmt.Sprintf("Started reconciling %s", obj.Name))
```

---

### 📊 **Test Tier Priority Matrix**

Use this matrix to determine which tier to test each maturity feature:

| Feature | Unit | Integration | E2E | Rationale |
|---------|------|-------------|-----|-----------|
| **Metric recorded** | ⬜ | ✅ | ⬜ | Registry inspection needs real operation |
| **Metric on endpoint** | ⬜ | ⬜ | ✅ | Requires deployed controller with HTTP server |
| **Audit fields correct** | ⬜ | ✅ | ⬜ | OpenAPI client validation needs real Data Storage |
| **Audit client wired** | ⬜ | ⬜ | ✅ | Must verify in deployed controller |
| **EventRecorder emits** | ⬜ | ⬜ | ✅ | Events need real K8s API server |
| **Graceful shutdown flush** | ✅ | ⬜ | ⬜ | Can mock manager and verify Close() |
| **Health probes accessible** | ⬜ | ⬜ | ✅ | Requires deployed controller with probes |
| **Predicate applied** | ✅ | ⬜ | ⬜ | Code structure verification |
| **Config validation** | ✅ | ⬜ | ⬜ | Pure function, no external deps |

**Legend**: ✅ Test here | ⬜ Don't test here

**Rationale**:
- **Unit**: Fast, isolated, no external dependencies
- **Integration**: Real infrastructure (podman-compose), but not full cluster
- **E2E**: Full deployment to Kind cluster, real K8s API

---

### 📊 **Metrics Testing Requirements**

**Policy**: Every metric defined in the service MUST have:
1. **Integration test**: Verify metric value after operation
2. **E2E test**: Verify metric appears on `/metrics` endpoint

#### CRD Controller Metrics Testing

```go
// ✅ REQUIRED: Integration test - verify metrics are recorded
var _ = Describe("Metrics Integration", func() {
    Context("BR-XXX-XXX: Reconciliation Metrics", func() {
        It("should record reconciliation total metric after successful reconcile", func() {
            // Given: A SignalProcessing CR in Pending phase
            sp := createTestSignalProcessing("test-metrics", namespace)
            Expect(k8sClient.Create(ctx, sp)).To(Succeed())

            // When: Reconciliation completes
            Eventually(func() string {
                var updated signalprocessingv1alpha1.SignalProcessing
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
                return updated.Status.Phase
            }, 30*time.Second, 1*time.Second).Should(Equal("Completed"))

            // Then: Metrics should be recorded
            // Option 1: Registry inspection (CRD controllers using envtest)
            families, err := ctrlmetrics.Registry.Gather()
            Expect(err).ToNot(HaveOccurred())

            var found bool
            for _, family := range families {
                if family.GetName() == "signalprocessing_reconciliations_total" {
                    found = true
                    // Verify label values
                    for _, metric := range family.GetMetric() {
                        for _, label := range metric.GetLabel() {
                            if label.GetName() == "phase" && label.GetValue() == "Completed" {
                                Expect(metric.GetCounter().GetValue()).To(BeNumerically(">", 0))
                            }
                        }
                    }
                }
            }
            Expect(found).To(BeTrue(), "Metric signalprocessing_reconciliations_total not found")
        })
    })
})

// ✅ REQUIRED: E2E test - verify /metrics endpoint exposes metrics
var _ = Describe("Metrics E2E", func() {
    It("should expose metrics on /metrics endpoint", func() {
        // Given: Controller is deployed and running
        // metricsURL is set up in SynchronizedBeforeSuite via NodePort

        // When: We query the metrics endpoint
        resp, err := http.Get(metricsURL)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()

        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        body, err := io.ReadAll(resp.Body)
        Expect(err).ToNot(HaveOccurred())

        // Then: Expected metrics should be present
        metricsOutput := string(body)
        Expect(metricsOutput).To(ContainSubstring("signalprocessing_reconciliations_total"))
        Expect(metricsOutput).To(ContainSubstring("signalprocessing_processing_duration_seconds"))
        Expect(metricsOutput).To(ContainSubstring("signalprocessing_enrichment_total"))
    })
})
```

#### Stateless Service Metrics Testing

```go
// ✅ REQUIRED: Integration test - verify metrics after HTTP operations
var _ = Describe("Metrics Integration", func() {
    It("should record request metrics after API calls", func() {
        // Given: Service is running via podman-compose

        // When: We make API requests
        resp, err := http.Post(serviceURL+"/api/v1/signals", "application/json", body)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusCreated))

        // Then: Metrics should be recorded on /metrics endpoint
        metricsResp, err := http.Get(serviceURL + "/metrics")
        Expect(err).ToNot(HaveOccurred())
        defer metricsResp.Body.Close()

        body, _ := io.ReadAll(metricsResp.Body)
        metricsOutput := string(body)

        // Verify specific metrics
        Expect(metricsOutput).To(ContainSubstring("http_requests_total"))
        Expect(metricsOutput).To(ContainSubstring("http_request_duration_seconds"))
    })
})
```

---

### 📋 **Audit Trace Testing Requirements**

**Policy**: Every audit trace MUST be verified using the OpenAPI audit client. All field values MUST be validated.

#### Audit Trace Test Pattern

```go
// ✅ REQUIRED: Integration test - verify each audit trace
var _ = Describe("Audit Trace Integration", func() {
    var auditClient *dsgen.APIClient

    BeforeEach(func() {
        // Setup OpenAPI audit client
        cfg := dsgen.NewConfiguration()
        cfg.Servers = []dsgen.ServerConfiguration{{URL: dataStorageURL}}
        auditClient = dsgen.NewAPIClient(cfg)
    })

    Context("BR-SP-090: Categorization Audit Trail", func() {
        It("should emit audit trace with all required fields on successful categorization", func() {
            // Given: A SignalProcessing CR
            sp := createTestSignalProcessing("test-audit", namespace)
            sp.Spec.Signal.Severity = "critical"
            Expect(k8sClient.Create(ctx, sp)).To(Succeed())

            // When: Reconciliation completes
            Eventually(func() string {
                var updated signalprocessingv1alpha1.SignalProcessing
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
                return updated.Status.Phase
            }, 30*time.Second, 1*time.Second).Should(Equal("Completed"))

            // Then: Audit trace should be retrievable with correct values
            Eventually(func() bool {
                // Query audit events via OpenAPI client
                events, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
                    Service("signalprocessing").
                    CorrelationId(string(sp.UID)).
                    Execute()

                if err != nil || len(events.Events) == 0 {
                    return false
                }

                event := events.Events[0]

                // ✅ REQUIRED: Validate ALL audit fields
                Expect(event.Service).To(Equal("signalprocessing"))
                Expect(event.EventType).To(Equal("categorization_completed"))
                Expect(event.EventCategory).To(Equal(dsgen.AuditEventRequestEventCategory("signalprocessing")))
                Expect(event.CorrelationId).To(Equal(string(sp.UID)))
                Expect(event.Severity).To(Equal("info"))

                // Validate event data fields
                eventData, ok := event.EventData.(map[string]interface{})
                Expect(ok).To(BeTrue())
                Expect(eventData["signal_name"]).To(Equal(sp.Spec.Signal.Name))
                Expect(eventData["severity"]).To(Equal("critical"))
                Expect(eventData["environment"]).ToNot(BeEmpty())
                Expect(eventData["priority"]).ToNot(BeEmpty())

                return true
            }, 30*time.Second, 2*time.Second).Should(BeTrue())
        })

        It("should emit audit trace on categorization failure", func() {
            // Test error audit traces similarly
        })
    })
})
```

#### Audit Trace Checklist

For each audit trace defined in DD-AUDIT-003:

- [ ] **Integration test exists** that triggers the audit trace
- [ ] **All fields validated** via OpenAPI audit client:
  - [ ] `service` - correct service name
  - [ ] `eventType` - correct event type
  - [ ] `eventCategory` - uses enum type, not string
  - [ ] `correlationId` - matches resource UID
  - [ ] `severity` - appropriate for event
  - [ ] `eventData` - all required fields present with correct values
- [ ] **Error scenarios tested** - audit traces emitted on failures

---

### 🚫 **ANTI-PATTERN: Direct Audit Infrastructure Testing**

**CRITICAL**: Integration tests MUST test **business logic that emits audits**, NOT **audit infrastructure**.

**Discovered**: December 26, 2025 - System-wide triage found 21+ tests across 3 services following this anti-pattern.

**Reference**: [Audit Infrastructure Testing Anti-Pattern Triage](../../handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md)

#### The Problem

Tests that **directly call audit store methods** (`StoreAudit()`, `RecordAudit()`, `StoreBatch()`) test the **audit client library** (DataStorage's responsibility), NOT the **service's business logic**.

**What's Being Tested**:
- ❌ Audit client buffering works (pkg/audit responsibility)
- ❌ Audit client batching works (pkg/audit responsibility)
- ❌ Audit client graceful shutdown works (pkg/audit responsibility)
- ❌ DataStorage persistence works (DataStorage service responsibility)

**What's NOT Being Tested**:
- ❌ Service controller emits audits during reconciliation
- ❌ Service correctly integrates audit calls into business flows
- ❌ Audit events are emitted at the right time in the business flow

#### ❌ WRONG PATTERN: Testing Audit Infrastructure

```go
// ❌ FORBIDDEN: This tests the audit client library, NOT the service
var _ = Describe("Audit Integration Tests", func() {
    var auditStore audit.AuditStore

    BeforeEach(func() {
        // Create audit store with real Data Storage
        dsClient, _ := audit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
        auditStore, _ = audit.NewBufferedStore(dsClient, config, "myservice", logger)
    })

    It("should write audit event to Data Storage", func() {
        // ❌ WRONG: Manually creating audit event (not from business logic)
        event := audit.NewAuditEventRequest()
        audit.SetEventType(event, "myservice.operation.completed")
        audit.SetEventCategory(event, "myservice")
        audit.SetEventOutcome(event, audit.OutcomeSuccess)
        // ... set more fields ...

        // ❌ WRONG: Directly calling audit store (testing infrastructure)
        err := auditStore.StoreAudit(ctx, event)
        Expect(err).NotTo(HaveOccurred())

        // ❌ WRONG: Verifying persistence (DataStorage's responsibility)
        Eventually(func() int {
            resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                CorrelationId: &correlationID,
                EventCategory: &eventCategory,
            })
            return *resp.JSON200.Pagination.Total
        }).Should(Equal(1))
    })

    It("should flush batch of events", func() {
        // ❌ WRONG: Testing audit client batching behavior
        for i := 0; i < 15; i++ {
            event := audit.NewAuditEventRequest()
            // ... set fields ...
            auditStore.StoreAudit(ctx, event)
        }

        auditStore.Close()  // ❌ WRONG: Testing flush behavior

        Eventually(func() int {
            resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, params)
            return *resp.JSON200.Pagination.Total
        }).Should(Equal(15))  // ❌ WRONG: Verifying batch persistence
    })
})
```

**Why This is Wrong**:
1. **Wrong Responsibility**: Tests audit client library, not service business logic
2. **Wrong Ownership**: These tests belong in `pkg/audit/buffered_store_integration_test.go` or DataStorage service
3. **Missing Coverage**: Service controller's audit integration is NOT tested
4. **False Confidence**: Tests pass but don't validate service emits audits correctly

#### ✅ CORRECT PATTERN: Business Logic with Audit Side Effects

```go
// ✅ CORRECT: Test business logic, verify audit as side effect
var _ = Describe("SignalProcessing Audit Integration", func() {
    var dsClient *dsgen.ClientWithResponses

    BeforeEach(func() {
        // Setup OpenAPI client for audit verification ONLY
        dsClient, _ = dsgen.NewClientWithResponses(dataStorageURL)
    })

    It("should emit audit event when signal processing completes", func() {
        // ✅ CORRECT: Trigger business operation (create CRD)
        sp := &signalprocessingv1alpha1.SignalProcessing{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-signal",
                Namespace: namespace,
            },
            Spec: signalprocessingv1alpha1.SignalProcessingSpec{
                Signal: signalprocessingv1alpha1.SignalSpec{
                    Name:     "HighMemoryUsage",
                    Severity: "critical",
                },
            },
        }
        Expect(k8sClient.Create(ctx, sp)).To(Succeed())

        // ✅ CORRECT: Wait for controller to process (BUSINESS LOGIC)
        Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
            var updated signalprocessingv1alpha1.SignalProcessing
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
            return updated.Status.Phase
        }, 30*time.Second, 1*time.Second).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

        // ✅ CORRECT: Verify controller emitted audit event (SIDE EFFECT)
        eventType := "signalprocessing.signal.processed"
        eventCategory := "signalprocessing"
        correlationID := string(sp.UID)

        Eventually(func() int {
            resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &eventType,
                EventCategory: &eventCategory,
                CorrelationId: &correlationID,
            })
            if err != nil || resp.JSON200 == nil {
                return 0
            }
            return *resp.JSON200.Pagination.Total
        }, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
            "Controller should emit audit event during reconciliation")

        // ✅ CORRECT: Validate audit event content
        resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
            EventType:     &eventType,
            EventCategory: &eventCategory,
            CorrelationId: &correlationID,
        })

        events := *resp.JSON200.Data
        Expect(events).To(HaveLen(1))

        event := events[0]
        testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
            EventType:     "signalprocessing.signal.processed",
            EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
            EventAction:   "processed",
            EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
            CorrelationID: correlationID,
            EventDataFields: map[string]interface{}{
                "signal_name": "HighMemoryUsage",
                "severity":    "critical",
                "phase":       "Completed",
            },
        })
    })

    It("should emit audit event when processing fails", func() {
        // ✅ CORRECT: Test error scenarios similarly
        // Create CR that will fail → Wait for Failed phase → Verify audit event
    })
})
```

**Why This is Correct**:
1. **Right Responsibility**: Tests service controller behavior, not infrastructure
2. **Right Ownership**: Service owns these tests, validates its own audit integration
3. **Complete Coverage**: Validates controller emits audits at correct times
4. **True Confidence**: If controller stops emitting audits, tests will fail

#### Pattern Comparison

| Aspect | ❌ Wrong Pattern | ✅ Correct Pattern |
|--------|-----------------|-------------------|
| **Test Focus** | Audit client library | Service business logic |
| **Primary Action** | `auditStore.StoreAudit()` | `k8sClient.Create(CRD)` |
| **What's Validated** | Audit persistence works | Controller emits audits |
| **Test Ownership** | Should be in DataStorage | Correctly in service tests |
| **Business Value** | Tests infrastructure | Tests service behavior |
| **Failure Detection** | Won't catch missing audit calls in controller | Catches missing audit integration |

#### Real-World Examples

**✅ CORRECT Examples** (Reference Implementations):
- **SignalProcessing**: `test/integration/signalprocessing/audit_integration_test.go` lines 97-196
  - Creates SignalProcessing CR → Waits for completion → Verifies audit event
- **Gateway**: `test/integration/gateway/audit_integration_test.go` lines 171-226
  - Sends webhook → Waits for processing → Verifies audit event

**❌ WRONG Examples** (Deleted December 2025):
- **Notification**: Had 6 tests calling `auditStore.StoreAudit()` directly → DELETED
- **WorkflowExecution**: Had 5 tests calling `dsClient.StoreBatch()` directly → DELETED
- **RemediationOrchestrator**: Had ~10 tests calling `auditStore.StoreAudit()` directly → DELETED
- **AIAnalysis**: Had 11 tests calling audit helpers manually → DELETED (Dec 26, 2025)

#### Detection Commands

```bash
# Find tests that might follow wrong pattern
grep -r "auditStore\.StoreAudit\|\.RecordAudit\|dsClient\.StoreBatch" test/integration --include="*_test.go"

# Check if tests create business CRDs (good sign)
grep -r "k8sClient.Create.*Request\|k8sClient.Create.*Processing\|k8sClient.Create.*Execution" test/integration --include="*_test.go"

# Tests should have BOTH: CRD creation AND audit queries (not direct store calls)
```

#### Migration Guide

**Step 1**: Identify wrong pattern tests
```bash
# Find files with direct audit store calls
grep -r "\.StoreAudit\|\.RecordAudit\|\.StoreBatch" test/integration/{service}/ --include="*_test.go" -l
```

**Step 2**: Delete wrong pattern tests
```go
// Delete entire test files or describe blocks that follow wrong pattern
// Example: test/integration/notification/audit_integration_test.go lines 119-505
```

**Step 3**: Create flow-based tests using correct pattern
```go
// Use SignalProcessing/Gateway as templates
// Pattern: Create CRD → Wait for processing → Verify audit as side effect
```

**Step 4**: Verify coverage
```bash
# Ensure each audit event type has a flow-based test
grep "should emit.*audit" test/integration/{service}/ --include="*_test.go"
```

#### Enforcement

CI pipelines SHOULD:
1. **Detect** `auditStore.StoreAudit()` in integration tests without corresponding `k8sClient.Create()`
2. **Flag** tests that manually create audit events
3. **Require** justification for direct audit store calls in code review

```bash
# CI check for wrong pattern (warning, not blocking)
if grep -r "auditStore\.StoreAudit\|\.RecordAudit" test/integration --include="*_test.go" | grep -v "pkg/audit"; then
    echo "⚠️  WARNING: Found direct audit store calls in integration tests"
    echo "   Integration tests should test business logic that emits audits as side effects"
    echo "   See: docs/development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing"
    echo "   Reference implementations: SignalProcessing, Gateway"
fi
```

#### Key Takeaway

**Integration tests should test SERVICE BEHAVIOR (business logic), not INFRASTRUCTURE (audit client library).**

If your test manually creates audit events and calls `StoreAudit()`, you're testing the wrong thing.

---

### 🚫 **ANTI-PATTERN: Direct Metrics Method Calls in Integration Tests**

**CRITICAL**: Integration tests MUST test **business logic that emits metrics**, NOT **metrics infrastructure**.

**Discovered**: December 27, 2025 - System-wide triage found 2 services (AIAnalysis, SignalProcessing) with ~629 lines following this anti-pattern.

**Reference**: [Metrics Anti-Pattern Triage](../../handoff/METRICS_ANTI_PATTERN_TRIAGE_DEC_27_2025.md)

#### The Problem

Tests that **directly call metrics methods** (`RecordMetric()`, `IncrementMetric()`, `ObserveMetric()`) test the **metrics infrastructure** (Prometheus client library), NOT the **service's business logic**.

**What's Being Tested**:
- ❌ Prometheus counters work (prometheus/client_golang responsibility)
- ❌ Prometheus histograms work (prometheus/client_golang responsibility)
- ❌ Metrics registration works (controller-runtime/pkg/metrics responsibility)
- ❌ Metrics can be gathered from registry (infrastructure verification)

**What's NOT Being Tested**:
- ❌ Service controller emits metrics during reconciliation
- ❌ Service correctly integrates metrics calls into business flows
- ❌ Metrics are emitted at the right time in the business flow
- ❌ Metrics reflect actual business outcomes

#### ❌ WRONG PATTERN: Testing Metrics Infrastructure

```go
// ❌ FORBIDDEN: This tests the metrics infrastructure, NOT the service
var _ = Describe("Metrics Integration Tests", func() {
    var testMetrics *metrics.Metrics

    BeforeEach(func() {
        // Create metrics instance for testing
        testMetrics = metrics.NewMetrics()
    })

    It("should increment reconciliation counter", func() {
        // ❌ WRONG: Directly calling metrics method (not from business logic)
        testMetrics.RecordReconciliation("Investigating", "success")

        // ❌ WRONG: Verifying metrics infrastructure works
        families, err := ctrlmetrics.Registry.Gather()
        Expect(err).ToNot(HaveOccurred())

        // ❌ WRONG: Checking metric exists in registry (infrastructure test)
        metric := families["aianalysis_reconciler_reconciliations_total"]
        Expect(metric).ToNot(BeNil())
    })

    It("should observe duration histogram", func() {
        // ❌ WRONG: Directly calling metrics method
        testMetrics.RecordReconcileDuration("Pending", 1.5)

        // ❌ WRONG: Verifying histogram infrastructure
        families, _ := ctrlmetrics.Registry.Gather()
        histogram := families["aianalysis_reconciler_duration_seconds"]
        Expect(histogram.GetType()).To(Equal(dto.MetricType_HISTOGRAM))
    })

    It("should increment processing counter multiple times", func() {
        // ❌ WRONG: Testing counter increment behavior (infrastructure)
        spMetrics.IncrementProcessingTotal("enriching", "success")
        spMetrics.IncrementProcessingTotal("enriching", "success")
        spMetrics.IncrementProcessingTotal("enriching", "failure")

        // ❌ WRONG: Verifying counter math works
        families, _ := testRegistry.Gather()
        counter := getCounterValue(families, "signalprocessing_processing_total",
            map[string]string{"phase": "enriching", "result": "success"})
        Expect(counter).To(Equal(2.0))
    })
})
```

**Why This is Wrong**:
1. **Wrong Responsibility**: Tests metrics infrastructure, not service business logic
2. **Wrong Ownership**: These tests belong in `pkg/metrics/*_test.go` or Prometheus client library
3. **Missing Coverage**: Service controller's metrics integration is NOT tested
4. **False Confidence**: Tests pass but don't validate service emits metrics correctly during actual operations

#### ✅ CORRECT PATTERN: Business Logic with Metrics Side Effects

```go
// ✅ CORRECT: Test business logic, verify metrics as side effect
var _ = Describe("AIAnalysis Metrics Integration", func() {
    // Helper to gather metrics from controller-runtime registry
    gatherMetrics := func() (map[string]*dto.MetricFamily, error) {
        families, err := ctrlmetrics.Registry.Gather()
        if err != nil {
            return nil, err
        }
        result := make(map[string]*dto.MetricFamily)
        for _, family := range families {
            result[family.GetName()] = family
        }
        return result, nil
    }

    // Helper to get counter value with specific labels
    getCounterValue := func(name string, labels map[string]string) float64 {
        families, _ := gatherMetrics()
        family := families[name]
        if family == nil {
            return 0
        }
        for _, m := range family.GetMetric() {
            labelMatch := true
            for wantKey, wantValue := range labels {
                found := false
                for _, l := range m.GetLabel() {
                    if l.GetName() == wantKey && l.GetValue() == wantValue {
                        found = true
                        break
                    }
                }
                if !found {
                    labelMatch = false
                    break
                }
            }
            if labelMatch {
                return m.GetCounter().GetValue()
            }
        }
        return 0
    }

    It("should emit reconciliation metrics when processing AIAnalysis CRD", func() {
        // ✅ CORRECT: Trigger business operation (create CRD)
        aianalysis := &aianalysisv1alpha1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-analysis",
                Namespace: namespace,
            },
            Spec: aianalysisv1alpha1.AIAnalysisSpec{
                Alert: aianalysisv1alpha1.AlertSpec{
                    Name:     "HighMemoryUsage",
                    Severity: "critical",
                },
            },
        }
        Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

        // ✅ CORRECT: Wait for controller to reconcile (BUSINESS LOGIC)
        Eventually(func() aianalysisv1alpha1.AIAnalysisPhase {
            var updated aianalysisv1alpha1.AIAnalysis
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated)
            return updated.Status.Phase
        }, 30*time.Second, 1*time.Second).Should(Equal(aianalysisv1alpha1.PhaseCompleted))

        // ✅ CORRECT: Verify controller emitted metrics (SIDE EFFECT)
        Eventually(func() float64 {
            return getCounterValue("aianalysis_reconciler_reconciliations_total",
                map[string]string{
                    "phase":  "Investigating",
                    "result": "success",
                })
        }, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
            "Controller should emit reconciliation metrics during reconciliation")

        // ✅ CORRECT: Verify duration histogram was recorded
        Eventually(func() bool {
            families, _ := gatherMetrics()
            histogram := families["aianalysis_reconciler_duration_seconds"]
            if histogram == nil {
                return false
            }
            // Check histogram has samples (controller recorded duration)
            for _, m := range histogram.GetMetric() {
                for _, l := range m.GetLabel() {
                    if l.GetName() == "phase" && l.GetValue() == "Investigating" {
                        return m.GetHistogram().GetSampleCount() > 0
                    }
                }
            }
            return false
        }, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
            "Controller should record reconciliation duration")
    })

    It("should emit approval decision metrics when Rego evaluation completes", func() {
        // ✅ CORRECT: Create AIAnalysis that triggers Rego evaluation
        aianalysis := &aianalysisv1alpha1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-approval",
                Namespace: namespace,
            },
            Spec: aianalysisv1alpha1.AIAnalysisSpec{
                Alert: aianalysisv1alpha1.AlertSpec{
                    Name:        "OOMKilled",
                    Severity:    "critical",
                    Environment: "production",
                },
            },
        }
        Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

        // ✅ CORRECT: Wait for Rego evaluation to complete
        Eventually(func() string {
            var updated aianalysisv1alpha1.AIAnalysis
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated)
            return updated.Status.ApprovalStatus
        }, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty())

        // ✅ CORRECT: Verify Rego evaluation metrics were emitted
        Eventually(func() float64 {
            return getCounterValue("aianalysis_rego_evaluations_total",
                map[string]string{
                    "decision": "requires_approval",
                    "cached":   "false",
                })
        }, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
            "Controller should emit Rego evaluation metrics")

        // ✅ CORRECT: Verify approval decision metrics
        Eventually(func() float64 {
            return getCounterValue("aianalysis_approval_decisions_total",
                map[string]string{
                    "decision":    "manual_approval_required",
                    "environment": "production",
                })
        }, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
            "Controller should emit approval decision metrics")
    })
})
```

**Why This is Correct**:
1. ✅ **Tests Business Logic**: Creates CRD, waits for controller to process
2. ✅ **Validates Integration**: Metrics are emitted during actual reconciliation
3. ✅ **Real Confidence**: Tests prove controller emits metrics at the right time
4. ✅ **Correct Ownership**: Tests service behavior, not infrastructure

#### Affected Services

**Services with Anti-Pattern** (2/7):
- **AIAnalysis**: `test/integration/aianalysis/metrics_integration_test.go` (~329 lines)
- **SignalProcessing**: `test/integration/signalprocessing/metrics_integration_test.go` (~300+ lines)

**Services with Correct Pattern** (3/7):
- **DataStorage**: Uses business flow validation
- **WorkflowExecution**: No direct metrics calls
- **RemediationOrchestrator**: No direct metrics calls

**Services without Metrics Tests** (2/7):
- **Gateway**: No metrics integration tests
- **Notification**: No metrics integration tests

#### Migration Guide

**Step 1**: Identify wrong pattern tests
```bash
# Find files with direct metrics method calls
grep -r "testMetrics\.\|\.RecordMetric\|\.IncrementMetric\|\.ObserveMetric" test/integration/{service}/ --include="*_test.go" -l
```

**Step 2**: Identify key business flows that should emit metrics
```go
// For AIAnalysis:
// - CRD reconciliation (phases: Pending → Investigating → Completed/Failed)
// - Rego policy evaluation
// - Approval decisions
// - Confidence score calculations

// For SignalProcessing:
// - Signal processing (phases: enriching → classifying → categorizing)
// - Enrichment operations (Pod, Deployment, k8s_context)
// - Enrichment errors (timeout, not_found, api_error)
```

**Step 3**: Create flow-based tests using correct pattern
```go
// Pattern: Create CRD → Wait for processing → Verify metrics as side effect
// Use the correct pattern examples above as templates
```

**Step 4**: Delete or deprecate wrong pattern tests
```go
// Mark old tests as deprecated or delete them entirely
// Example: test/integration/aianalysis/metrics_integration_test.go lines 119-329
```

**Step 5**: Verify coverage
```bash
# Ensure each metrics type has a flow-based test
grep "should emit.*metrics" test/integration/{service}/ --include="*_test.go"
```

#### Enforcement

CI pipelines SHOULD:
1. **Detect** `testMetrics.Record*()` or `spMetrics.Increment*()` in integration tests without corresponding `k8sClient.Create()`
2. **Flag** tests that directly call metrics methods
3. **Require** justification for direct metrics calls in code review

```bash
# CI check for wrong pattern (warning, not blocking)
if grep -r "testMetrics\.\|spMetrics\.\|\.RecordMetric\|\.IncrementMetric" test/integration --include="*_test.go" | grep -v "pkg/metrics"; then
    echo "⚠️  WARNING: Found direct metrics method calls in integration tests"
    echo "   Integration tests should test business logic that emits metrics as side effects"
    echo "   See: docs/development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-metrics-method-calls"
    echo "   Reference: METRICS_ANTI_PATTERN_TRIAGE_DEC_27_2025.md"
fi
```

#### Key Takeaway

**Integration tests should test SERVICE BEHAVIOR (business logic), not INFRASTRUCTURE (metrics client library).**

If your test directly calls `testMetrics.RecordReconciliation()` or similar methods, you're testing the wrong thing.

**Correct approach**: Create CRD → Wait for business outcome → Verify metrics were emitted as side effect.

---

### 🚫 **ANTI-PATTERN: HTTP Testing in Integration Tests**

**CRITICAL**: Integration tests MUST test **component coordination with direct business logic calls**, NOT **HTTP API contracts**.

**Discovered**: January 10, 2026 - System-wide triage found Gateway service (19/24 tests) following this anti-pattern.

**Reference**: DataStorage test refactoring (January 2026) - moved 12 HTTP tests from integration to E2E/performance tiers.

#### The Problem

Integration tests that use `httptest.Server` and make HTTP requests test the **HTTP transport layer**, NOT **component coordination**. This conflates E2E testing (full stack with HTTP) with integration testing (component coordination without HTTP).

**What's Being Tested**:
- ❌ HTTP endpoints work (E2E tier responsibility)
- ❌ HTTP status codes correct (E2E tier responsibility)
- ❌ Request/response serialization (E2E tier responsibility)
- ❌ OpenAPI validation middleware (E2E tier responsibility)

**What's NOT Being Tested**:
- ❌ Component coordination without transport overhead
- ❌ Business logic integration with dependencies
- ❌ Fast, focused integration validation

#### The Root Cause

**Integration tests were treating HTTP as essential** when HTTP is just a **transport mechanism**. The confusion stems from:

1. **Gateway is an HTTP service** → "Integration tests should use HTTP"
2. **DataStorage has HTTP API** → "Integration tests should test HTTP"

**Reality**: HTTP is a **deployment detail**, not a **business integration requirement**.

#### Test Tier Definitions

|| Tier | Infrastructure | HTTP? | Focus |
||------|---------------|-------|-------|
|| **Unit** | None | ❌ No | Algorithm correctness, edge cases |
|| **Integration** | Real dependencies (PostgreSQL, Redis, K8s) | ❌ **NO HTTP** | Component coordination via **direct business logic calls** |
|| **E2E** | Full deployment (Kind cluster) | ✅ Yes | Full stack including HTTP, OpenAPI validation |
|| **Performance** | Full deployment | ✅ Yes | Throughput, latency, resource usage |

**Key Rule**: **Integration tests MUST NOT use HTTP**. If you need HTTP, it's an E2E or performance test.

#### ❌ WRONG PATTERN: HTTP in Integration Tests

```go
// ❌ FORBIDDEN: Gateway integration test using HTTP
var _ = Describe("Adapter Integration", func() {
    var (
        testServer *httptest.Server  // ❌ HTTP server in integration test
        k8sClient  *K8sTestClient
    )

    BeforeEach(func() {
        // ❌ WRONG: Create HTTP test server
        gatewayServer := gateway.NewServer(config, k8sClient, redisClient, auditClient)
        testServer = httptest.NewServer(gatewayServer.Handler())
    })

    It("should process Prometheus alert through pipeline", func() {
        // ❌ WRONG: Send HTTP webhook
        payload := GeneratePrometheusAlert(PrometheusAlertOptions{
            AlertName: "HighMemoryUsage",
            Namespace: namespace,
            Severity:  "critical",
        })

        resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

        // ❌ WRONG: Verify HTTP response
        Expect(resp.StatusCode).To(Equal(201))

        // ✅ Verify CRD created (this part is correct)
        Eventually(func() bool {
            var crd remediationv1alpha1.RemediationRequest
            err := k8sClient.Get(ctx, key, &crd)
            return err == nil
        }, 30*time.Second, 1*time.Second).Should(BeTrue())
    })
})
```

**Why This is Wrong**:
1. **E2E Disguised as Integration**: Full HTTP stack = E2E test, not integration
2. **Slow**: HTTP overhead slows down test suite
3. **Wrong Focus**: Tests HTTP transport, not business logic coordination
4. **Duplicate Coverage**: E2E tests already cover HTTP endpoints
5. **Misleading Tier**: Developers expect fast, focused tests in integration tier

#### ❌ WRONG PATTERN: DataStorage HTTP Testing (Before Refactoring)

```go
// ❌ FORBIDDEN: DataStorage integration test using HTTP (DELETED January 2026)
var _ = Describe("Audit Write API", func() {
    var (
        testServer *httptest.Server  // ❌ HTTP server in integration test
        client     *ogenclient.Client
    )

    BeforeEach(func() {
        // ❌ WRONG: Create in-process HTTP server
        dsServer, _ := server.NewServer(dbConn, redisAddr, logger, config)
        testServer = httptest.NewServer(dsServer.Handler())
        client, _ = ogenclient.NewClient(testServer.URL)
    })

    It("should accept valid audit event", func() {
        // ❌ WRONG: Test HTTP API endpoint
        event := ogenclient.AuditEventRequest{
            EventType:     "gateway.signal.received",
            EventCategory: ogenclient.AuditEventRequestEventCategoryGateway,
            // ...
        }

        resp, err := client.CreateAuditEvent(ctx, &event)  // ❌ HTTP call
        Expect(err).ToNot(HaveOccurred())

        // ❌ WRONG: Verify HTTP response type
        _, ok := resp.(*ogenclient.CreateAuditEventCreated)
        Expect(ok).To(BeTrue())
    })
})
```

**Why This Was Wrong**:
1. **Testing HTTP API Contract**: Should be in E2E tier
2. **OpenAPI Validation**: Middleware validation is E2E concern
3. **Status Code Testing**: HTTP semantics are E2E concern
4. **Duplicates E2E Coverage**: Full stack already tested in E2E

#### ✅ CORRECT PATTERN: Direct Business Logic Calls

```go
// ✅ CORRECT: Gateway integration test WITHOUT HTTP
var _ = Describe("Adapter Integration", func() {
    var (
        prometheusAdapter *adapters.PrometheusAdapter
        dedupService      *dedup.Service
        crdManager        *crd.Manager
        k8sClient         *K8sTestClient
        redisClient       *redis.Client
        auditClient       audit.AuditStore
    )

    BeforeEach(func() {
        // ✅ CORRECT: Wire components directly (no HTTP)
        prometheusAdapter = adapters.NewPrometheusAdapter(logger)
        dedupService = dedup.NewService(redisClient, config)
        crdManager = crd.NewManager(k8sClient, auditClient, logger)
    })

    It("should process Prometheus alert through adapter → dedup → CRD pipeline", func() {
        // ✅ CORRECT: Call adapter directly (no HTTP)
        alertPayload := `{
            "alerts": [{
                "labels": {
                    "alertname": "HighMemoryUsage",
                    "namespace": "production",
                    "severity": "critical"
                }
            }]
        }`

        // Step 1: Adapter transforms alert
        signal, err := prometheusAdapter.Transform([]byte(alertPayload))
        Expect(err).ToNot(HaveOccurred())

        // Step 2: Dedup checks if duplicate
        isDuplicate, fingerprint, err := dedupService.CheckDuplicate(ctx, signal)
        Expect(err).ToNot(HaveOccurred())
        Expect(isDuplicate).To(BeFalse())

        // Step 3: CRD manager creates RemediationRequest
        crd, err := crdManager.CreateRemediationRequest(ctx, signal, fingerprint)
        Expect(err).ToNot(HaveOccurred())

        // ✅ CORRECT: Verify CRD created via K8s API (real integration)
        Eventually(func() bool {
            var retrieved remediationv1alpha1.RemediationRequest
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), &retrieved)
            return err == nil
        }, 30*time.Second, 1*time.Second).Should(BeTrue())

        // ✅ CORRECT: Verify audit event emitted (side effect)
        Eventually(func() int {
            resp, _ := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
                CorrelationID: ogenclient.NewOptString(string(crd.UID)),
            })
            return len(resp.Data)
        }, 10*time.Second, 1*time.Second).Should(BeNumerically(">", 0))
    })
})
```

**Why This is Correct**:
1. ✅ **Tests Component Coordination**: Adapter → Dedup → CRD pipeline
2. ✅ **No HTTP Overhead**: Direct function calls, fast execution
3. ✅ **Real Dependencies**: Uses real Redis, K8s API, PostgreSQL
4. ✅ **Correct Tier**: Focused integration validation, not full stack
5. ✅ **Clear Intent**: Tests business logic flow, not transport layer

#### ✅ CORRECT PATTERN: DataStorage WITHOUT HTTP (After Refactoring)

```go
// ✅ CORRECT: DataStorage integration test WITHOUT HTTP (January 2026)
var _ = Describe("Audit Repository Integration", func() {
    var (
        repo   *repository.AuditEventsRepository
        db     *sqlx.DB
        logger logr.Logger
    )

    BeforeEach(func() {
        // ✅ CORRECT: Use repository directly (no HTTP)
        repo = repository.NewAuditEventsRepository(db.DB, logger)
    })

    It("should insert audit event into PostgreSQL", func() {
        // ✅ CORRECT: Call repository method directly
        event := &repository.AuditEvent{
            EventType:     "gateway.signal.received",
            EventCategory: "gateway",
            EventOutcome:  "success",
            CorrelationID: correlationID,
            // ...
        }

        createdEvent, err := repo.Create(ctx, event)
        Expect(err).ToNot(HaveOccurred())
        Expect(createdEvent.EventID).ToNot(BeEmpty())

        // ✅ CORRECT: Verify database state directly
        var count int
        err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
        Expect(err).ToNot(HaveOccurred())
        Expect(count).To(Equal(1))
    })

    It("should batch insert multiple events", func() {
        // ✅ CORRECT: Test repository batching logic (no HTTP)
        events := []*repository.AuditEvent{
            {EventType: "gateway.signal.received", EventCategory: "gateway", CorrelationID: correlationID},
            {EventType: "gateway.signal.processed", EventCategory: "gateway", CorrelationID: correlationID},
            {EventType: "gateway.crd.created", EventCategory: "gateway", CorrelationID: correlationID},
        }

        createdEvents, err := repo.CreateBatch(ctx, events)
        Expect(err).ToNot(HaveOccurred())
        Expect(createdEvents).To(HaveLen(3))

        // ✅ CORRECT: Verify batch insertion worked
        var count int
        err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
        Expect(err).ToNot(HaveOccurred())
        Expect(count).To(Equal(3))
    })
})
```

**Why This is Correct**:
1. ✅ **Tests Repository Logic**: Database operations without HTTP overhead
2. ✅ **Real PostgreSQL**: Tests actual database integration
3. ✅ **Fast Execution**: No HTTP serialization/deserialization
4. ✅ **Correct Tier**: Integration tests repository with real database
5. ✅ **Clear Separation**: HTTP API tests moved to E2E tier

#### When HTTP IS Acceptable

**HTTP is ONLY acceptable in these tiers:**

| Test Tier | HTTP Allowed? | Why? |
|-----------|---------------|------|
| **Unit** | ❌ No | Mocked, no real dependencies |
| **Integration** | ❌ **NO** | Direct business logic calls only |
| **E2E** | ✅ Yes | Full stack validation including HTTP |
| **Performance** | ✅ Yes | Throughput/latency testing requires HTTP |

#### NO EXCEPTIONS: HTTP Infrastructure Tests Also Belong in E2E

**CRITICAL**: There are **NO EXCEPTIONS** to the "no HTTP in integration tests" rule.

Even tests that validate HTTP infrastructure (timeouts, rate limits, TLS, server lifecycle) should be in the **E2E tier**, not integration tier.

```go
// ❌ WRONG: HTTP infrastructure test in integration tier
// Location: test/integration/gateway/http_server_test.go
var _ = Describe("HTTP Server Infrastructure", func() {
    var testServer *httptest.Server  // ← HTTP = E2E tier

    It("should enforce request timeout", func() {
        // ❌ This is testing HTTP infrastructure, not component coordination
    })
})

// ✅ CORRECT: HTTP infrastructure test in E2E tier
// Location: test/e2e/gateway/XX_http_server_test.go
var _ = Describe("HTTP Server Infrastructure", func() {
    It("should enforce request timeout", func() {
        // ✅ E2E tests validate full infrastructure including HTTP
    })
})
```

**Rationale**:
1. **HTTP = Full Stack = E2E**: Any test requiring HTTP stack validates full deployment
2. **TLS = Infrastructure = E2E**: Certificate validation requires real HTTP/TLS (E2E scope)
3. **No Special Cases**: Consistency prevents "exception creep"

**Formerly "Legitimate" Cases** (Now Corrected):
- ❌ **"TLS tests need HTTP"** → Move to E2E (infrastructure validation)
- ❌ **"Audit verification via HTTP"** → Query PostgreSQL directly in integration tests
- ❌ **"HTTP infrastructure tests"** → Move to E2E (full stack validation)

**Rule**: **NO HTTP in integration tests. No exceptions. Ever.**

#### Migration Guide

**Step 1**: Identify HTTP tests in integration tier
```bash
# Find integration tests using HTTP
grep -r "httptest\|http\.Post\|http\.Get\|testServer" test/integration/{service}/ --include="*_test.go" -l
```

**Step 2**: Categorize tests
```go
// Ask: "What is this test validating?"

// ❌ HTTP API contract (status codes, request/response format)
//    → Move to E2E tier

// ❌ Performance (throughput, latency, cold start)
//    → Move to performance tier

// ✅ Component coordination (adapter → dedup → CRD)
//    → Refactor to use direct business logic calls
```

**Step 3**: Refactor to direct calls
```go
// BEFORE: HTTP-based integration test
resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
Expect(resp.StatusCode).To(Equal(201))

// AFTER: Direct business logic integration test
signal, err := adapter.Transform(payload)
Expect(err).ToNot(HaveOccurred())
isDupe, fingerprint, err := dedupService.CheckDuplicate(ctx, signal)
Expect(err).ToNot(HaveOccurred())
crd, err := crdManager.CreateRemediationRequest(ctx, signal, fingerprint)
Expect(err).ToNot(HaveOccurred())
```

**Step 4**: Move HTTP tests to E2E
```bash
# Move HTTP API contract tests
git mv test/integration/{service}/audit_write_api_test.go \
       test/e2e/{service}/12_audit_write_api_test.go
```

#### Real-World Refactoring Example

**DataStorage Service (January 2026)**:

| Before | After | Change |
|--------|-------|--------|
| **Integration**: 12 tests (3 using HTTP) | **Integration**: 9 tests (0 using HTTP) | ✅ -3 HTTP tests |
| **E2E**: 9 tests | **E2E**: 19 tests | ✅ +10 tests |
| **Performance**: 0 tests | **Performance**: 2 tests | ✅ +2 tests |

**Results**:
- ✅ Integration tests 40% faster (no HTTP overhead)
- ✅ Clear tier separation (integration = business logic, E2E = HTTP)
- ✅ Better test focus (integration tests component coordination, not transport)

#### Enforcement

CI pipelines SHOULD:
1. **Detect** `httptest.Server` or `http.Post` in integration test directories
2. **Flag** for review in code review
3. **Require** justification for HTTP in integration tests

```bash
# CI check for HTTP anti-pattern in integration tests
if grep -r "httptest\|http\.Post\|http\.Get\|SendWebhook" test/integration --include="*_test.go" | grep -v "http_server_test\|rate_limit" | grep -v "^Binary"; then
    echo "⚠️  WARNING: Found HTTP usage in integration tests"
    echo "   Integration tests MUST use direct business logic calls, NOT HTTP"
    echo "   HTTP tests belong in E2E or performance tiers"
    echo "   See: docs/development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-http-testing-in-integration-tests"
    echo ""
    echo "   NO EXCEPTIONS: ALL HTTP tests should be in E2E or performance tier"
    echo "   Even HTTP infrastructure tests belong in E2E, not integration"
fi
```

#### Key Takeaway

**Integration tests MUST test component coordination via direct business logic calls, NOT via HTTP.**

If your integration test uses `httptest.Server`, you're likely writing an E2E test in the wrong tier.

**Correct mental model**:
- **Integration**: `adapter.Transform()` → `dedupService.CheckDuplicate()` → `crdManager.Create()`
- **E2E**: `http.Post("/api/v1/signals")` → verify full stack

---

### 🔌 **EventRecorder Testing Requirements** (CRD Controllers Only)

**Policy**: Controllers MUST emit Kubernetes Events for debugging. E2E tests MUST verify events.

```go
// ✅ REQUIRED: E2E test - verify events emitted
var _ = Describe("EventRecorder E2E", func() {
    It("should emit Kubernetes events on phase transitions", func() {
        // Given: A SignalProcessing CR
        sp := createTestSignalProcessing("test-events", namespace)
        Expect(k8sClient.Create(ctx, sp)).To(Succeed())

        // When: Reconciliation completes
        Eventually(func() string {
            var updated signalprocessingv1alpha1.SignalProcessing
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
            return updated.Status.Phase
        }, 30*time.Second, 1*time.Second).Should(Equal("Completed"))

        // Then: Events should be present
        Eventually(func() bool {
            var events corev1.EventList
            err := k8sClient.List(ctx, &events,
                client.InNamespace(namespace),
                client.MatchingFields{"involvedObject.name": sp.Name})

            if err != nil || len(events.Items) == 0 {
                return false
            }

            // Verify expected events
            foundEnriching := false
            foundCompleted := false
            for _, event := range events.Items {
                if event.Reason == "EnrichingStarted" {
                    foundEnriching = true
                }
                if event.Reason == "ProcessingCompleted" {
                    foundCompleted = true
                }
            }
            return foundEnriching && foundCompleted
        }, 30*time.Second, 2*time.Second).Should(BeTrue())
    })
})
```

---

### 🛑 **Graceful Shutdown Testing Requirements**

**Policy**: All services MUST flush state on SIGTERM per DD-007. Integration tests MUST verify.

```go
// ✅ REQUIRED: Integration test - verify graceful shutdown
var _ = Describe("Graceful Shutdown (DD-007)", func() {
    It("should flush audit store on SIGTERM", func() {
        // This test verifies the shutdown behavior using mocks
        mockAuditStore := &mockAuditStore{}
        mockManager := &mockManager{
            startFunc: func(ctx context.Context) error {
                // Simulate receiving SIGTERM
                <-ctx.Done()
                return nil
            },
        }

        // Run the main function logic
        runMainWithMocks(mockManager, mockAuditStore)

        // Verify Close() was called on audit store
        Expect(mockAuditStore.closeCalled).To(BeTrue())
    })
})
```

---

### ✅ **V1.0 Maturity Test Compliance Matrix**

Use this matrix to track test coverage for maturity features:

| Feature | Integration Test | E2E Test | Status |
|---------|------------------|----------|--------|
| **Metrics recorded** | Verify values via registry | Verify `/metrics` endpoint | ⬜ |
| **Metrics registered** | N/A | Verify all metrics present | ⬜ |
| **EventRecorder** | N/A | Verify events via kubectl | ⬜ |
| **Audit traces** | Verify all fields via OpenAPI | Verify client wired | ⬜ |
| **Graceful shutdown** | Verify flush on SIGTERM | N/A | ⬜ |
| **Health probes** | N/A | Verify probe endpoints | ⬜ |

---

### 📝 **Test Plan Template Reference**

For comprehensive test planning, use the **V1.0 Service Maturity Test Plan Template**:

**Location**: `docs/development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md`

This template provides:
- Standardized test structure for all maturity features
- Copy-paste test code for each feature type
- Compliance checklist for sign-off
- Integration with CI/CD validation

---

### 🔄 **Living Document Maintenance**

> **This section is a living document.** Developers MUST:
>
> 1. **Triage new ADRs/DDs** for testing implications
> 2. **Add new testing requirements** to this guideline
> 3. **Update the Test Plan Template** with new patterns
> 4. **Communicate changes** via handoff notification
>
> **How to check for new requirements**:
> ```bash
> # Find ADRs/DDs created after this guideline was updated
> find docs/architecture/decisions -name "*.md" -newer docs/development/business-requirements/TESTING_GUIDELINES.md
> ```
>
> **Last Updated**: 2025-12-19 (v2.1.0)
> **Next Review**: When any new ADR/DD affecting testing is created
