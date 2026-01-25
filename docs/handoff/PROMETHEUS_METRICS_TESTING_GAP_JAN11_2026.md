# Prometheus Metrics Testing Gap - Handoff to RO Team

**Date**: January 11, 2026
**Audience**: RemediationOrchestrator Team, All Service Teams
**Priority**: HIGH - Testing Gap Identified
**Status**: ⚠️ ACTION REQUIRED

---

## 🚨 Executive Summary

**GAP DISCOVERED**: RemediationOrchestrator (and potentially other services) do not test Prometheus metrics in the integration tier, creating a conformance validation gap.

**IMPACT**:
- Metrics registry collisions undetected until E2E
- Business metric correctness untested
- Parallel test execution issues hidden
- No validation that metrics follow specifications

**DISCOVERED DURING**: Gateway integration test debugging (January 11, 2026)

**REQUIRED ACTION**: RO team must add Prometheus metrics testing to integration tier using isolated registry pattern.

---

## 📊 Current State Analysis

### **RemediationOrchestrator Integration Tests**

**Current Coverage**:
- ✅ Audit event emission (100% tested in integration tier)
- ✅ CRD lifecycle (tested)
- ✅ Business logic (tested)
- ❌ **Prometheus metrics** (NOT tested in integration tier)

**Location**: `test/integration/remediationorchestrator/`

**Evidence**:
```bash
# Search for Prometheus metrics testing in RO integration tier
$ grep -r "prometheus\|metrics\|NewMetrics" test/integration/remediationorchestrator/*.go
# Result: No metrics testing found (only config YAML references)
```

**Current E2E Coverage**:
- E2E tests likely validate metrics exist, but don't validate:
  - Metric value correctness
  - Label correctness
  - Counter increments
  - Histogram bucket distribution
  - Registry isolation in parallel tests

---

## 🎯 Testing Tier Responsibilities

### **INTEGRATION TIER** (Where Metrics Testing Belongs)

**Purpose**: Validate business logic with real infrastructure (K8s API, metrics registry)

**Metrics Testing Responsibilities**:
- ✅ Metric values are correct (counters increment, gauges update)
- ✅ Metric labels are populated correctly
- ✅ Histogram buckets are appropriate
- ✅ Registry isolation in parallel tests (NO collisions)
- ✅ Metric naming follows specifications
- ✅ Business requirement mapping (BR-XXX-XXX)

**Example**: "When RR transitions to `Analyzing`, `ro_phase_transitions_total{phase="Analyzing"}` increments by 1"

**Why Integration Tier?**:
- Fast feedback (~90s for full suite)
- Isolated test environment (envtest or Kind)
- No external service dependencies
- Can test edge cases (error paths, retries, timeouts)
- Validates business logic correctness

---

### **E2E TIER** (Limited Metrics Validation)

**Purpose**: Validate full system integration across multiple services

**Metrics Testing Responsibilities**:
- ✅ Metrics endpoint is accessible
- ✅ Metrics are exposed via `/metrics` HTTP endpoint
- ✅ Prometheus can scrape metrics
- ✅ Controller and metrics registry are wired correctly

**Example**: "RO `/metrics` endpoint returns 200 OK and includes `ro_phase_transitions_total`"

**Why NOT Full Testing in E2E?**:
- Slow feedback (~15-20 minutes for full E2E suite)
- Complex multi-service orchestration
- Infrastructure flakiness
- Difficult to test edge cases
- Limited to happy path validation

---

## 🐛 Problem Discovered in Gateway

### **Symptom**: Parallel Test Failures

```
[PANICKED!] BR-001, BR-002: Adapter Interaction Patterns
/vendor/github.com/prometheus/client_golang/prometheus/registry.go:406

panic: duplicate metrics collector registration attempted
```

**Root Cause**: All tests used global Prometheus registry (`prometheus.DefaultRegisterer`)

**Why This Matters**:
- In parallel test execution, multiple tests tried to register the same metric
- Prometheus panics on duplicate registration (by design)
- Tests that worked sequentially failed in parallel

**Why RO Didn't Hit This**:
- RO integration tests don't create metrics instances
- RO E2E tests run sequentially (no parallel execution stress test)
- Problem hidden until parallel test execution

---

## ✅ Solution Pattern: Isolated Registry Per Test

### **Gateway Fix** (January 11, 2026)

**Before** (BROKEN - uses global registry):
```go
var _ = Describe("Adapter Interaction Tests", func() {
    var (
        metricsInstance *metrics.Metrics
        crdCreator      *processing.CRDCreator
    )

    BeforeEach(func() {
        // ❌ WRONG: Uses global registry
        metricsInstance = metrics.NewMetrics()
        crdCreator = processing.NewCRDCreator(
            k8sClient,
            logger,
            metricsInstance, // ← Registry collision!
            namespace,
            retryConfig,
        )
    })
})
```

**After** (FIXED - isolated registry):
```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

var _ = Describe("Adapter Interaction Tests", func() {
    var (
        metricsInstance *metrics.Metrics
        crdCreator      *processing.CRDCreator
    )

    BeforeEach(func() {
        // ✅ CORRECT: Create isolated registry per test
        testRegistry := prometheus.NewRegistry()
        metricsInstance = metrics.NewMetricsWithRegistry(testRegistry)

        crdCreator = processing.NewCRDCreator(
            k8sClient,
            logger,
            metricsInstance, // ← No collision!
            namespace,
            retryConfig,
        )
    })
})
```

**Key Changes**:
1. Import `github.com/prometheus/client_golang/prometheus`
2. Create `prometheus.NewRegistry()` in `BeforeEach`
3. Use `metrics.NewMetricsWithRegistry(testRegistry)` instead of `metrics.NewMetrics()`
4. Each test gets isolated registry → no collisions

**Files Modified**:
- `test/integration/gateway/adapter_interaction_test.go`
- `test/integration/gateway/k8s_api_integration_test.go`
- `test/integration/gateway/k8s_api_interaction_test.go`

**Result**: 50/50 tests passing (100% pass rate) in parallel execution

---

## 📋 Action Items for RO Team

### **PRIORITY 1: Verify RO Metrics Infrastructure**

**Task 1.1**: Check if RO metrics package supports isolated registries

```bash
# Check RO metrics implementation
grep -n "NewMetrics\|NewMetricsWithRegistry" pkg/remediationorchestrator/metrics/*.go
```

**Expected**: RO should have a `NewMetricsWithRegistry()` function similar to Gateway

**If Missing**: Add `NewMetricsWithRegistry()` function following Gateway pattern:
```go
// pkg/remediationorchestrator/metrics/metrics.go

func NewMetrics() *Metrics {
    return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
    factory := promauto.With(registry)

    return &Metrics{
        PhaseTransitions: factory.NewCounterVec(
            prometheus.CounterOpts{
                Name: "ro_phase_transitions_total",
                Help: "Total phase transitions",
            },
            []string{"phase", "status"},
        ),
        // ... other metrics
    }
}
```

---

### **PRIORITY 2: Add Metrics Testing to Integration Tier**

**Task 2.1**: Create `test/integration/remediationorchestrator/metrics_integration_test.go`

**Test Categories** (aligned with business requirements):

#### **A. Phase Transition Metrics** (BR-RO-001)
```go
It("should increment ro_phase_transitions_total when RR transitions phases", func() {
    // Setup: Create isolated registry
    testRegistry := prometheus.NewRegistry()
    metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)

    // Setup: Create RR controller with isolated metrics
    controller := setupControllerWithMetrics(metricsInstance)

    // Action: Create RR that triggers phase transition
    rr := createTestRR("test-rr", namespace)
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Wait for phase transition (Pending → Analyzing)
    Eventually(func() string {
        k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
        return rr.Status.Phase
    }, "10s").Should(Equal("Analyzing"))

    // Verify: Metric incremented
    families := gatherMetrics(testRegistry)
    counter := findMetric(families, "ro_phase_transitions_total",
        map[string]string{"phase": "Analyzing", "status": "success"})
    Expect(counter.GetCounter().GetValue()).To(Equal(1.0))
})
```

#### **B. Reconciliation Duration Metrics** (BR-RO-002)
```go
It("should record ro_reconciliation_duration_seconds histogram", func() {
    testRegistry := prometheus.NewRegistry()
    metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)

    // Setup controller
    controller := setupControllerWithMetrics(metricsInstance)

    // Action: Reconcile RR
    rr := createTestRR("test-rr", namespace)
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Wait for reconciliation
    Eventually(func() bool {
        k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
        return rr.Status.Phase != ""
    }, "10s").Should(BeTrue())

    // Verify: Histogram recorded
    families := gatherMetrics(testRegistry)
    histogram := findMetric(families, "ro_reconciliation_duration_seconds", nil)
    Expect(histogram.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
})
```

#### **C. Error Metrics** (BR-RO-003)
```go
It("should increment ro_errors_total on reconciliation failure", func() {
    testRegistry := prometheus.NewRegistry()
    metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)

    // Setup: Controller with isolated metrics
    controller := setupControllerWithMetrics(metricsInstance)

    // Action: Create invalid RR (missing required field)
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "invalid-rr",
            Namespace: namespace,
        },
        Spec: remediationv1.RemediationRequestSpec{
            // Missing SignalFingerprint (required field)
        },
    }
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Wait for error metric
    Eventually(func() float64 {
        families := gatherMetrics(testRegistry)
        counter := findMetric(families, "ro_errors_total",
            map[string]string{"type": "validation_error"})
        return counter.GetCounter().GetValue()
    }, "10s").Should(BeNumerically(">", 0))
})
```

#### **D. Parallel Test Isolation** (Infrastructure Test)
```go
It("should support parallel test execution without registry collisions", func() {
    // This test validates registry isolation works correctly
    // Run 10 parallel goroutines creating metrics instances

    var wg sync.WaitGroup
    errors := make(chan error, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            defer GinkgoRecover()

            // Each goroutine creates isolated registry
            testRegistry := prometheus.NewRegistry()
            metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)

            // Simulate metric operations
            metricsInstance.PhaseTransitions.WithLabelValues("Analyzing", "success").Inc()
            metricsInstance.ReconciliationDuration.WithLabelValues("success").Observe(0.5)

            // Verify metrics work
            families := gatherMetrics(testRegistry)
            if len(families) == 0 {
                errors <- fmt.Errorf("goroutine %d: no metrics gathered", id)
            }
        }(i)
    }

    wg.Wait()
    close(errors)

    // Verify: No errors
    errorList := []error{}
    for err := range errors {
        errorList = append(errorList, err)
    }
    Expect(errorList).To(BeEmpty(), "All parallel metrics instances should work")
})
```

**Helper Functions** (add to `test/integration/remediationorchestrator/helpers.go`):
```go
// gatherMetrics collects all metrics from a registry
func gatherMetrics(registry prometheus.Gatherer) []*dto.MetricFamily {
    families, err := registry.Gather()
    Expect(err).ToNot(HaveOccurred())
    return families
}

// findMetric finds a specific metric by name and labels
func findMetric(families []*dto.MetricFamily, name string, labels map[string]string) *dto.Metric {
    for _, family := range families {
        if family.GetName() != name {
            continue
        }
        for _, metric := range family.GetMetric() {
            if labelsMatch(metric, labels) {
                return metric
            }
        }
    }
    return nil
}

// labelsMatch checks if metric labels match the given map
func labelsMatch(metric *dto.Metric, labels map[string]string) bool {
    if labels == nil {
        return true // No filter
    }
    for key, expectedValue := range labels {
        found := false
        for _, label := range metric.GetLabel() {
            if label.GetName() == key && label.GetValue() == expectedValue {
                found = true
                break
            }
        }
        if !found {
            return false
        }
    }
    return true
}
```

---

### **PRIORITY 3: Update Existing Integration Tests**

**Task 3.1**: Modify `test/integration/remediationorchestrator/suite_test.go`

**Add to BeforeEach**:
```go
BeforeEach(func() {
    // ... existing setup ...

    // Create isolated metrics registry for test
    testRegistry := prometheus.NewRegistry()
    metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)

    // Use isolated metrics in controller setup
    controller := setupROController(k8sClient, metricsInstance, auditStore)
})
```

**Task 3.2**: Update all integration tests that create controllers
- `lifecycle_test.go`
- `blocking_integration_test.go`
- `notification_lifecycle_integration_test.go`
- `operational_metrics_integration_test.go` (already metrics-focused, add registry isolation)

---

## 📚 Reference Implementation

### **Gateway Metrics Package** (Complete Example)

**Location**: `pkg/gateway/metrics/metrics.go`

**Key Features**:
- ✅ `NewMetrics()` for production (global registry)
- ✅ `NewMetricsWithRegistry()` for testing (isolated registry)
- ✅ All metrics defined as struct fields
- ✅ Metric name constants for test validation

**Pattern to Follow**:
```go
// Production constructor (uses global registry)
func NewMetrics() *Metrics {
    return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

// Test constructor (uses isolated registry)
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
    factory := promauto.With(registry)

    return &Metrics{
        // Use factory for all metrics
        PhaseTransitions: factory.NewCounterVec(...),
        ReconciliationDuration: factory.NewHistogramVec(...),
        ErrorsTotal: factory.NewCounterVec(...),
    }
}
```

---

## 🔍 Verification Steps

### **After Implementing Metrics Testing**

**Step 1**: Run integration tests in parallel
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test ./test/integration/remediationorchestrator/... -v -p 4
```

**Expected**: All tests pass without `AlreadyRegisteredError` panics

**Step 2**: Verify metrics are tested
```bash
# Check test coverage
grep -r "gatherMetrics\|findMetric\|NewMetricsWithRegistry" test/integration/remediationorchestrator/
```

**Expected**: Metrics helper functions used in tests

**Step 3**: Run full integration suite
```bash
make test-integration-ro
```

**Expected**: 100% pass rate with metrics validation

---

## 🎯 Success Criteria

**MANDATORY Before PR Approval**:
- [ ] RO metrics package has `NewMetricsWithRegistry()` function
- [ ] RO integration tests use isolated registries in all tests
- [ ] New `metrics_integration_test.go` created with 4+ test categories
- [ ] All existing integration tests updated to use isolated registries
- [ ] Integration tests pass in parallel execution (`-p 4`)
- [ ] No `AlreadyRegisteredError` panics in test output
- [ ] Metrics values validated (not just existence checked)
- [ ] Business requirements mapped to metric tests (BR-RO-XXX)

**RECOMMENDED**:
- [ ] Add metrics testing to other services (SignalProcessing, AIAnalysis, WorkflowExecution)
- [ ] Document metrics testing pattern in `.cursor/rules/03-testing-strategy.mdc`
- [ ] Add pre-commit hook to prevent `metrics.NewMetrics()` in test files

---

## 📖 Related Documentation

### **Testing Strategy**
- [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth testing approach
- [15-testing-coverage-standards.mdc](mdc:.cursor/rules/15-testing-coverage-standards.mdc) - Coverage requirements

### **Gateway Implementation** (Reference)
- `pkg/gateway/metrics/metrics.go` - Isolated registry pattern
- `test/integration/gateway/adapter_interaction_test.go` - Metrics testing example
- `test/integration/gateway/k8s_api_integration_test.go` - Multiple registry instances

### **Prometheus Documentation**
- [Prometheus Go Client](https://github.com/prometheus/client_golang) - Registry documentation
- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/) - Metric naming

---

## 🚀 Timeline & Ownership

**Owner**: RemediationOrchestrator Team
**Estimated Effort**: 4-6 hours
**Deadline**: Before next sprint planning

**Breakdown**:
- 1 hour: Add `NewMetricsWithRegistry()` to RO metrics package
- 2 hours: Create `metrics_integration_test.go` with 4 test categories
- 1 hour: Update existing integration tests with isolated registries
- 1 hour: Run tests, fix issues, verify parallel execution
- 30 min: Documentation and PR review

---

## 📞 Questions & Support

**Primary Contact**: Gateway Team (discovered and fixed the issue)
**Location**: `test/integration/gateway/` - Reference implementation

**Questions to Ask**:
1. "Does our metrics package support isolated registries?"
2. "Do we test metric values in integration tier?"
3. "Can our integration tests run in parallel without panics?"
4. "Are metrics validated for correctness or just existence?"

**Expected Answers**:
1. YES (via `NewMetricsWithRegistry()`)
2. YES (in integration tier, not just E2E)
3. YES (using isolated registries)
4. VALUES VALIDATED (counters increment, histograms record)

---

## ✅ Checklist for RO Team

Copy this checklist to your sprint planning:

```markdown
## Prometheus Metrics Testing Implementation

### Phase 1: Metrics Infrastructure
- [ ] Verify `NewMetricsWithRegistry()` exists in `pkg/remediationorchestrator/metrics/`
- [ ] If missing, implement following Gateway pattern
- [ ] Add metric name constants for test validation
- [ ] Update main application to use `NewMetrics()` (global registry)

### Phase 2: Integration Test Creation
- [ ] Create `test/integration/remediationorchestrator/metrics_integration_test.go`
- [ ] Add test: Phase transition metrics (BR-RO-001)
- [ ] Add test: Reconciliation duration metrics (BR-RO-002)
- [ ] Add test: Error metrics (BR-RO-003)
- [ ] Add test: Parallel execution isolation
- [ ] Add helper functions: `gatherMetrics()`, `findMetric()`, `labelsMatch()`

### Phase 3: Existing Test Updates
- [ ] Update `suite_test.go` to create isolated registries
- [ ] Update `lifecycle_test.go` to use isolated metrics
- [ ] Update `blocking_integration_test.go` to use isolated metrics
- [ ] Update `notification_lifecycle_integration_test.go` to use isolated metrics
- [ ] Update `operational_metrics_integration_test.go` to use isolated metrics

### Phase 4: Validation
- [ ] Run integration tests sequentially: `go test ./test/integration/remediationorchestrator/... -v`
- [ ] Run integration tests in parallel: `go test ./test/integration/remediationorchestrator/... -v -p 4`
- [ ] Verify no `AlreadyRegisteredError` panics
- [ ] Verify 100% pass rate
- [ ] Verify metrics values are validated (not just existence)

### Phase 5: Documentation
- [ ] Update testing strategy docs with metrics testing requirements
- [ ] Add code comments explaining isolated registry pattern
- [ ] Create PR with clear description of changes
- [ ] Request review from Gateway team for pattern validation
```

---

## 🎓 Lessons Learned

### **Why This Gap Existed**

1. **Historical**: RO integration tests focused on audit events (primary business requirement)
2. **Hidden Problem**: Sequential test execution masked registry collision issues
3. **E2E Validation**: E2E tests validated metrics exist, but not business correctness

### **Why This Matters**

1. **Conformance**: Integration tier validates metrics follow specifications
2. **Fast Feedback**: Metric bugs caught in ~90s (integration) vs ~15min (E2E)
3. **Business Validation**: Metrics must reflect business events (phase transitions, errors)
4. **Parallel Execution**: Required for fast CI/CD pipelines

### **Pattern for Future Services**

**ALL services must**:
- Implement `NewMetricsWithRegistry()` in metrics package
- Test metrics values in integration tier
- Use isolated registries in all tests
- Validate business requirement mapping

---

**Document Status**: ✅ READY FOR ACTION
**Next Review**: After RO team completes implementation
**Success Metric**: RO integration tests achieve 100% pass rate with metrics validation
