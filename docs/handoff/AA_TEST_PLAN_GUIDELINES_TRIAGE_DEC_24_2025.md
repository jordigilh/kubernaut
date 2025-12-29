# AIAnalysis Test Plan vs Testing Guidelines Triage

**Date**: December 24, 2025
**Test Plan**: `docs/testing/test-plans/AA_INTEGRATION_TEST_PLAN_V1.0.md`
**Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
**Status**: 🟡 GAPS IDENTIFIED - Requires Plan Updates

---

## 📋 EXECUTIVE SUMMARY

**Total Gaps Found**: 8 critical + 5 recommendations
**Compliance Score**: 65% (Critical gaps must be addressed before Phase 1)

### Critical Gaps (Must Fix)
1. ❌ **Missing V1.0 Maturity Tests** - No metrics/audit/EventRecorder tests
2. ❌ **Skip() Usage Risk** - Test plan doesn't explicitly forbid Skip()
3. ❌ **time.Sleep() Guidance Missing** - No Eventually() requirements documented
4. ❌ **Coverage Target Mismatch** - 85-90% vs guidelines 70%/50%/50%
5. ❌ **Infrastructure Pattern** - Doesn't reference DD-TEST-002 sequential startup
6. ❌ **Test Tier Misalignment** - Integration tests should validate metrics via registry, not HTTP
7. ❌ **Audit Trace Testing Gap** - No OpenAPI client validation requirements
8. ❌ **Mock LLM Policy Missing** - Doesn't reference HAPI mock mode requirements

### Recommendations (Should Fix)
1. ⚠️ Add test structure templates with Eventually() examples
2. ⚠️ Reference podman-compose race condition warning
3. ⚠️ Add explicit Fail() pattern for missing services
4. ⚠️ Include kubeconfig isolation for E2E (when added)
5. ⚠️ Add living document maintenance section

---

## 🔴 CRITICAL GAP #1: V1.0 Maturity Testing Requirements

**Guideline Reference**: Lines 1368-1806
**Impact**: HIGH - Service cannot be production-ready without these tests
**Priority**: 🔴 CRITICAL

### What's Missing

The test plan does **NOT** include ANY V1.0 maturity tests:

| V1.0 Maturity Feature | Test Plan Coverage | Guidelines Requirement |
|----------------------|-------------------|----------------------|
| **Metrics Testing** | ❌ Missing | Integration: Registry inspection |
| **Metrics on /metrics** | ❌ Missing | E2E: HTTP endpoint validation |
| **Audit Trace Fields** | ❌ Missing | Integration: OpenAPI client validation of ALL fields |
| **Audit Client Wired** | ❌ Missing | E2E: Verify audit client integration |
| **EventRecorder** | ❌ Missing | E2E: Verify K8s events emitted |
| **Graceful Shutdown** | ❌ Missing | Integration: Verify audit flush on SIGTERM |
| **Health Probes** | ❌ Missing | E2E: Verify probe endpoints accessible |

### Guideline Requirements

**Metrics Testing** (Lines 1490-1590):
```go
// ✅ REQUIRED: Integration test - verify via registry inspection (NO HTTP server)
It("should register all business metrics", func() {
    // Create test-specific registry (DD-METRICS-001)
    testRegistry := prometheus.NewRegistry()
    testMetrics := metrics.NewMetricsWithRegistry(testRegistry)

    testMetrics.RecordReconciliation("Pending", "success")

    // Verify via registry inspection (NOT HTTP endpoint)
    families, err := testRegistry.Gather()
    // ... verify metric exists
})

// ✅ REQUIRED: E2E test - verify /metrics endpoint
It("should expose metrics on /metrics endpoint", func() {
    resp, err := http.Get(metricsURL)
    body, _ := io.ReadAll(resp.Body)
    Expect(string(body)).To(ContainSubstring("aianalysis_reconciler_reconciliations_total"))
})
```

**Audit Trace Testing** (Lines 1593-1678):
```go
// ✅ REQUIRED: Integration test - OpenAPI client validation
It("should emit audit trace with all required fields", func() {
    // Setup OpenAPI audit client
    cfg := dsgen.NewConfiguration()
    cfg.Servers = []dsgen.ServerConfiguration{{URL: dataStorageURL}}
    auditClient := dsgen.NewAPIClient(cfg)

    // Query audit events
    events, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
        Service("aianalysis").
        CorrelationId(string(analysis.UID)).
        Execute()

    // ✅ REQUIRED: Validate ALL audit fields
    Expect(event.Service).To(Equal("aianalysis"))
    Expect(event.EventType).To(Equal("analysis_completed"))
    Expect(event.CorrelationId).To(Equal(string(analysis.UID)))
    // ... validate all event_data fields
})
```

**EventRecorder** (Lines 1681-1726):
```go
// ✅ REQUIRED: E2E test - verify events emitted
It("should emit Kubernetes events on phase transitions", func() {
    var events corev1.EventList
    err := k8sClient.List(ctx, &events,
        client.InNamespace(namespace),
        client.MatchingFields{"involvedObject.name": analysis.Name})

    // Verify expected events
    foundInvestigating := false
    foundAnalyzing := false
    for _, event := range events.Items {
        if event.Reason == "PhaseTransition" {
            // Verify message format
        }
    }
})
```

### Recommended Fix

**ADD Phase 4 to Test Plan**: V1.0 Maturity Compliance

**New Tests to Add**:

| Test ID | Scenario | Test Tier | Guidelines Ref |
|---------|----------|-----------|----------------|
| **AA-INT-METRICS-001** | Record reconciliation metrics | Integration | Lines 1490-1537 |
| **AA-E2E-METRICS-001** | Expose metrics on /metrics endpoint | E2E | Lines 1539-1560 |
| **AA-INT-AUDIT-001** | Validate audit trace fields via OpenAPI | Integration | Lines 1600-1656 |
| **AA-INT-AUDIT-002** | Verify audit on phase transitions | Integration | Lines 1600-1656 |
| **AA-INT-AUDIT-003** | Verify audit on error scenarios | Integration | Lines 1600-1656 |
| **AA-E2E-EVENTS-001** | Verify EventRecorder emits K8s events | E2E | Lines 1686-1726 |
| **AA-INT-SHUTDOWN-001** | Verify graceful shutdown flushes audit | Integration | Lines 1733-1755 |
| **AA-E2E-HEALTH-001** | Verify health probe endpoints accessible | E2E | Lines 1479 |

**Estimated Additional Tests**: 8-12 tests
**Effort**: 2-3 days
**Priority**: Must complete before V1.0 release

---

## 🔴 CRITICAL GAP #2: Skip() Forbidden - Not Documented

**Guideline Reference**: Lines 855-985
**Impact**: HIGH - Risk of introducing Skip() calls during implementation
**Priority**: 🔴 CRITICAL

### What's Missing

The test plan does **NOT** mention that `Skip()` is **ABSOLUTELY FORBIDDEN**.

**Current Test Plan** (AA-INT-ERR-001 example):
```go
// Test Steps:
1. Configure mock HAPI to return 401
2. Create AIAnalysis CRD with valid spec
3. Wait for reconciliation

// Expected Results:
- Status.Phase = "Failed"
```

**Missing**: Explicit guidance that tests MUST fail (not skip) when HAPI unavailable.

### Guideline Requirements (Lines 855-985)

```go
// ❌ FORBIDDEN: Skipping when service unavailable
BeforeEach(func() {
    resp, err := http.Get(hapiURL + "/health")
    if err != nil {
        Skip("HAPI not available")  // ← ABSOLUTELY FORBIDDEN
    }
})

// ✅ REQUIRED: Fail with clear error message
BeforeEach(func() {
    resp, err := http.Get(hapiURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf(
            "REQUIRED: HAPI not available at %s\n"+
            "  Per DD-TEST-002: Infrastructure is auto-started in SynchronizedBeforeSuite\n"+
            "  Start with: podman-compose -f podman-compose.test.yml up -d",
            hapiURL))
    }
})
```

### Recommended Fix

**ADD to Test Plan** (Test Writing Guidelines section):

```markdown
### Anti-Patterns - ABSOLUTELY FORBIDDEN

#### Skip() is FORBIDDEN (Lines 855-985)

❌ **NEVER use Skip()** when services are unavailable:

```go
// ❌ FORBIDDEN
if !hapiAvailable {
    Skip("HAPI not running")
}

// ✅ REQUIRED: Fail with clear error
Expect(hapiAvailable).To(BeTrue(),
    "REQUIRED: HAPI must be running at %s\n"+
    "Per DD-TEST-002: Start with SynchronizedBeforeSuite", hapiURL)
```

**Rationale**: If a service can run without HAPI, then HAPI is optional. Integration tests MUST fail when required services are unavailable.
```

---

## 🔴 CRITICAL GAP #3: time.Sleep() / Eventually() Requirements

**Guideline Reference**: Lines 573-852
**Impact**: HIGH - Risk of flaky tests with time.Sleep()
**Priority**: 🔴 CRITICAL

### What's Missing

The test plan shows **NO guidance** on using `Eventually()` vs `time.Sleep()`.

**Current Test Plan** (AA-INT-RETRY-002 example):
```
Test Steps:
1. Configure mock HAPI to return 503 on first call, 200 on second
2. Create AIAnalysis CRD
3. Record timestamp of first failure
4. Record timestamp of first retry
5. Calculate actual backoff duration
```

**Missing**: How to "wait for first retry" - should use `Eventually()`, never `time.Sleep()`.

### Guideline Requirements (Lines 573-852)

```go
// ❌ FORBIDDEN: time.Sleep() before assertions
time.Sleep(5 * time.Second)
err := k8sClient.Get(ctx, key, &crd)
Expect(err).ToNot(HaveOccurred())

// ✅ REQUIRED: Eventually() for async operations
Eventually(func() error {
    return k8sClient.Get(ctx, key, &crd)
}, 30*time.Second, 1*time.Second).Should(Succeed())

// ✅ REQUIRED: Eventually() for status checks
Eventually(func() string {
    _ = k8sClient.Get(ctx, key, &crd)
    return crd.Status.Phase
}, 30*time.Second, 1*time.Second).Should(Equal("Failed"))
```

**Timeout Guidelines** (Lines 696-719):

| Test Tier | Typical Timeout | Interval | Rationale |
|-----------|----------------|----------|-----------|
| Unit | 1-5 seconds | 10-100ms | Fast, no I/O |
| Integration | 30-60 seconds | 1-2 seconds | Real K8s API, slower |
| E2E | 2-5 minutes | 5-10 seconds | Full infrastructure |

### Recommended Fix

**UPDATE Test Plan** (Assertion Patterns section):

```markdown
### Assertion Patterns - MANDATORY

#### Eventually() for All Async Operations (Lines 573-852)

❌ **time.Sleep() is ABSOLUTELY FORBIDDEN** for waiting on operations:

```go
// ❌ FORBIDDEN
time.Sleep(5 * time.Second)
Expect(analysis.Status.Phase).To(Equal("Failed"))

// ✅ REQUIRED: Eventually() with timeout
Eventually(func() string {
    var updated aianalysisv1.AIAnalysis
    _ = k8sClient.Get(ctx, key, &updated)
    return updated.Status.Phase
}, 30*time.Second, 1*time.Second).Should(Equal("Failed"))
```

**Integration Test Timeouts**: 30-60s timeout, 1-2s interval
**Rationale**: Real K8s API is slower than unit tests, but faster than E2E.

**Acceptable time.Sleep() Usage** (ONLY for timing behavior tests):
```go
// ✅ Acceptable: Testing backoff duration
start := time.Now()
// trigger retry logic
duration := time.Since(start)
Expect(duration).To(BeNumerically("~", 5*time.Second, 500*time.Millisecond))
```
```

---

## 🔴 CRITICAL GAP #4: Coverage Target Mismatch

**Guideline Reference**: Lines 47-81
**Impact**: MEDIUM - Expectations misalignment
**Priority**: 🔴 CRITICAL (Documentation)

### What's Missing

**Test Plan Says**: Target 85-90% integration coverage
**Guidelines Say**: Target 50% integration coverage (cumulative defense-in-depth)

### Guideline Requirements (Lines 47-81)

**Code Coverage - CUMULATIVE (~100% combined)**:

| Tier | Code Coverage Target | What It Validates |
|------|---------------------|-------------------|
| **Unit** | **70%+** | Algorithm correctness, edge cases, error handling |
| **Integration** | **50%** | Cross-component flows, CRD operations, real K8s API |
| **E2E** | **50%** | Full stack: main.go, reconciliation, business logic, metrics, audit |

**Key Insight**: With 70%/50%/50% code coverage targets, **50%+ of codebase is tested in ALL 3 tiers**.

**BR Coverage - OVERLAPPING**:

| Tier | BR Coverage Target | Purpose |
|------|-------------------|---------|
| **Unit** | **70%+ of ALL BRs** | Ensure all unit-testable business requirements implemented |
| **Integration** | **>50% of ALL BRs** | Validate cross-service coordination and CRD operations |
| **E2E** | **<10% BR coverage** | Critical user journeys only |

### Current Misalignment

**Test Plan Coverage Tracking Table**:
```
| Phase | Coverage Target | Tests |
|-------|----------------|-------|
| Phase 1 | 70-75% | 68-73 |
| Phase 2 | 80-85% | 78-88 |
| Phase 3 | 85-90% | 83-98 |
```

**Problem**: This suggests integration tests should reach 85-90% **code coverage**, which exceeds guidelines target of 50%.

### Recommended Fix

**UPDATE Test Plan** - Coverage Tracking section:

```markdown
## 📊 COVERAGE TRACKING

### Coverage Targets (Per TESTING_GUIDELINES.md)

**Code Coverage** (Cumulative Defense-in-Depth):
- **Unit Tests**: 70%+ (current: 70.0% ✅)
- **Integration Tests**: 50% target (current: 54.6% ✅ **EXCEEDS TARGET**)
- **E2E Tests**: 50% target (not yet measured)

**Interpretation**: AIAnalysis integration tests achieve 54.6% coverage, **EXCEEDING** the 50% guideline target. The new tests focus on **uncovered critical paths** (error handling, retry logic) rather than increasing overall coverage percentage.

**BR Coverage** (Overlapping):
- **Unit Tests**: 70%+ of BRs (validated via test-to-BR mapping)
- **Integration Tests**: >50% of BRs (same BRs tested at multiple tiers)
- **E2E Tests**: <10% of BRs (critical user journeys)

### Phase Goals (REVISED)

| Phase | Code Coverage | BR Coverage | Tests | Focus |
|-------|--------------|-------------|-------|-------|
| **Phase 1** | 54.6% → 60-65% | 60-70% | 68-73 | Error handling critical paths |
| **Phase 2** | 60-65% → 65-70% | 70-80% | 78-88 | Controller edge cases |
| **Phase 3** | 65-70% → 70%+ | 80-90% | 83-98 | Comprehensive coverage |

**Key Change**: Phase targets adjusted to reflect that **50% integration coverage is sufficient** per guidelines. Additional tests focus on **critical paths** and **BR coverage**, not just code coverage percentage.
```

---

## 🔴 CRITICAL GAP #5: Infrastructure Pattern Not Referenced

**Guideline Reference**: Lines 1030-1199 (DD-TEST-002)
**Impact**: MEDIUM - Risk of race conditions
**Priority**: 🔴 CRITICAL (Infrastructure)

### What's Missing

The test plan does **NOT** reference:
1. DD-TEST-002 sequential startup pattern
2. podman-compose race condition warning
3. How AIAnalysis infrastructure is started (SynchronizedBeforeSuite)

### Guideline Requirements (Lines 1030-1199)

**⚠️ CRITICAL: `podman-compose` Race Condition Warning**:
- `podman-compose up -d` starts all services **simultaneously**
- Causes race conditions when services have startup dependencies
- **Solution**: Sequential startup with explicit health checks (DD-TEST-002)

**Working Pattern** (DataStorage team proven, Dec 20, 2025):
```bash
# Sequential startup (DD-TEST-002)
1. Start PostgreSQL → wait for pg_isready
2. Start Redis → wait for redis-cli ping
3. Start DataStorage → wait for /health endpoint
4. Start HAPI → wait for /health endpoint
```

### Current State

**AIAnalysis Integration Tests** already use DD-TEST-002 sequential startup (from previous fixes):
- `test/integration/aianalysis/suite_test.go` uses `infrastructure.StartAIAnalysisIntegrationInfrastructure()`
- Infrastructure started in `SynchronizedBeforeSuite`
- Sequential: PostgreSQL → Redis → DataStorage → HAPI with health checks

### Recommended Fix

**ADD to Test Plan** (Infrastructure section):

```markdown
## 🏗️ TEST INFRASTRUCTURE

### Sequential Startup Pattern (DD-TEST-002)

**Authority**: [DD-TEST-002: Integration Test Container Orchestration](../../architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md)

AIAnalysis integration tests use **DD-TEST-002 compliant sequential startup** to avoid race conditions:

```go
// test/integration/aianalysis/suite_test.go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Sequential startup: PostgreSQL → Redis → DataStorage → HAPI
    // Each service waits for previous service's health check
    err := infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())

    // All services healthy before tests start
    return []byte{}
}, func(data []byte) {
    // Parallel workers reuse infrastructure
})
```

**Why NOT `podman-compose`?**

❌ `podman-compose up -d` starts services **simultaneously** → race conditions
✅ Sequential startup with health checks → **NO race conditions**

**Services Started**:
1. PostgreSQL (port 15434) - Wait for `pg_isready`
2. Redis (port 16380) - Wait for `redis-cli ping`
3. DataStorage (port 18095) - Wait for `/health` endpoint
4. HAPI (port 18120) - Wait for `/health` endpoint

**Reference**: `test/infrastructure/aianalysis.go` - `StartAIAnalysisIntegrationInfrastructure()`
```

---

## 🔴 CRITICAL GAP #6: Metrics Testing Tier Mismatch

**Guideline Reference**: Lines 473-528, 1490-1590
**Impact**: HIGH - Wrong test patterns documented
**Priority**: 🔴 CRITICAL

### What's Missing

**Test Plan Says**: Integration tests verify metrics via HTTP endpoint
**Guidelines Say**: CRD Controllers use **registry inspection** (NO HTTP in integration)

### Guideline Requirements (Lines 473-528)

**CRD Controller Metrics Testing**:

| Test Tier | Approach | Why |
|-----------|----------|-----|
| **Unit** | Registry inspection | Fresh test registry, fast |
| **Integration** | **Registry inspection** (NOT HTTP) | envtest has NO HTTP server |
| **E2E** | HTTP `/metrics` endpoint | Deployed controller with Service |

```go
// ✅ CORRECT: Integration test - registry inspection (NO HTTP)
It("should register all business metrics", func() {
    testRegistry := prometheus.NewRegistry()
    testMetrics := metrics.NewMetricsWithRegistry(testRegistry)

    testMetrics.RecordReconciliation("Pending", "success")

    families, err := testRegistry.Gather()
    // Verify metric via registry
})

// ❌ WRONG: Starting HTTP server in integration test
BeforeAll(func() {
    metricsServer = &http.Server{Addr: ":19184"}  // ❌ NO HTTP in integration
})
```

### Test Plan Current State

The test plan's mock HAPI configuration section shows:
```go
// Simulate timeout
mockHAPI.SetDelay(10 * time.Second)
```

But doesn't show **metrics validation** patterns.

### Recommended Fix

**ADD to Test Plan** - Validation Checklist:

```markdown
### Metrics Validation (Per DD-METRICS-001)

**Integration Tests** (CRD Controllers):
```go
// ✅ REQUIRED: Registry inspection (NO HTTP server)
Context("AA-INT-METRICS-001: Reconciliation metrics", func() {
    It("should record metrics via registry inspection", func() {
        // Use test-specific registry
        testRegistry := prometheus.NewRegistry()
        testMetrics := metrics.NewMetricsWithRegistry(testRegistry)

        // Trigger reconciliation
        // ... create AIAnalysis CRD

        // Verify via registry (NOT HTTP)
        families, err := testRegistry.Gather()
        Expect(err).ToNot(HaveOccurred())

        found := false
        for _, family := range families {
            if family.GetName() == "aianalysis_reconciler_reconciliations_total" {
                found = true
            }
        }
        Expect(found).To(BeTrue())
    })
})
```

**E2E Tests** (Deployed Controller):
```go
// ✅ REQUIRED: HTTP /metrics endpoint
Context("AA-E2E-METRICS-001: Metrics endpoint", func() {
    It("should expose metrics on /metrics", func() {
        resp, err := http.Get(metricsURL) // Via NodePort
        body, _ := io.ReadAll(resp.Body)
        Expect(string(body)).To(ContainSubstring("aianalysis_reconciler_reconciliations_total"))
    })
})
```

**Why This Matters**: Integration tests use envtest (NO HTTP server). Only E2E tests have deployed controllers with HTTP endpoints.
```

---

## 🔴 CRITICAL GAP #7: Audit Trace Validation Missing

**Guideline Reference**: Lines 1593-1678
**Impact**: HIGH - Audit compliance not validated
**Priority**: 🔴 CRITICAL

### What's Missing

The test plan mentions audit events but does **NOT** specify:
1. OpenAPI client usage (MANDATORY per guidelines)
2. ALL fields validation requirement
3. Field-by-field validation pattern

### Guideline Requirements (Lines 1593-1678)

**Policy**: Every audit trace MUST be verified using the OpenAPI audit client. **All field values MUST be validated.**

```go
// ✅ REQUIRED: Integration test with OpenAPI client
It("should emit audit trace with all required fields", func() {
    // Setup OpenAPI audit client (MANDATORY)
    cfg := dsgen.NewConfiguration()
    cfg.Servers = []dsgen.ServerConfiguration{{URL: dataStorageURL}}
    auditClient := dsgen.NewAPIClient(cfg)

    // Query audit events
    events, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
        Service("aianalysis").
        CorrelationId(string(analysis.UID)).
        Execute()

    event := events.Events[0]

    // ✅ REQUIRED: Validate ALL audit fields (NOT OPTIONAL)
    Expect(event.Service).To(Equal("aianalysis"))
    Expect(event.EventType).To(Equal("analysis_completed"))
    Expect(event.EventCategory).To(Equal(dsgen.AuditEventRequestEventCategory("aianalysis")))
    Expect(event.CorrelationId).To(Equal(string(analysis.UID)))
    Expect(event.Severity).To(Equal("info"))

    // Validate event data fields
    eventData, ok := event.EventData.(map[string]interface{})
    Expect(ok).To(BeTrue())
    Expect(eventData["signal_name"]).To(Equal(analysis.Spec.Signal.Name))
    // ... ALL fields validated
})
```

### Audit Trace Checklist (Lines 1665-1678)

For each audit trace:
- [ ] **Integration test exists** that triggers the audit trace
- [ ] **All fields validated** via OpenAPI audit client:
  - [ ] `service` - correct service name
  - [ ] `eventType` - correct event type
  - [ ] `eventCategory` - uses enum type, not string
  - [ ] `correlationId` - matches resource UID
  - [ ] `severity` - appropriate for event
  - [ ] `eventData` - all required fields present with correct values
- [ ] **Error scenarios tested** - audit traces emitted on failures

### Recommended Fix

**ADD to Test Plan** - Phase 4 (V1.0 Maturity):

```markdown
## 🟢 PHASE 4: V1.0 MATURITY COMPLIANCE (Priority: MANDATORY)

**Target**: V1.0 Production Readiness
**Estimated Tests**: 8-12
**Effort**: 2-3 days
**File**: `test/integration/aianalysis/v1_maturity_test.go`

### Audit Trace Validation (DD-AUDIT-003)

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-AUDIT-001** | Validate analysis_completed audit trace fields | DD-AUDIT-003 | 🟢 Mandatory | ⏸️ Pending |
| **AA-INT-AUDIT-002** | Validate phase_transition audit trace fields | DD-AUDIT-003 | 🟢 Mandatory | ⏸️ Pending |
| **AA-INT-AUDIT-003** | Validate analysis_failed audit trace fields | DD-AUDIT-003 | 🟢 Mandatory | ⏸️ Pending |

### Test Details

#### AA-INT-AUDIT-001: Validate analysis_completed audit trace fields
**Description**: When AIAnalysis completes successfully, audit trace MUST have ALL required fields validated via OpenAPI client.

**Test Steps**:
1. Create AIAnalysis CRD with valid spec
2. Wait for Phase = "Complete"
3. Query DataStorage audit API via OpenAPI client
4. Validate ALL fields (no fields skipped)

**Expected Results** (ALL FIELDS REQUIRED):
- `event.Service` = `"aianalysis"`
- `event.EventType` = `"analysis_completed"`
- `event.EventCategory` = `dsgen.AuditEventRequestEventCategory("aianalysis")`
- `event.CorrelationId` = `string(analysis.UID)`
- `event.Severity` = `"info"`
- `eventData["analysis_id"]` = `analysis.Name`
- `eventData["phase"]` = `"Complete"`
- `eventData["selected_workflow"]` = workflow ID (if applicable)
- `eventData["duration_seconds"]` > 0
- ALL other fields per DD-AUDIT-003

**Related Code**:
- `pkg/aianalysis/audit/audit.go:120` - RecordAnalysisComplete()
- OpenAPI client: `pkg/datastorage/client/generated/` (dsgen)

**Validation Checklist**:
- [ ] OpenAPI client used (MANDATORY)
- [ ] ALL fields validated (no fields skipped)
- [ ] EventCategory uses enum type (not string)
- [ ] eventData structure matches schema
```

---

## 🔴 CRITICAL GAP #8: Mock LLM Policy Not Referenced

**Guideline Reference**: Lines 437-469, 1224-1234
**Impact**: LOW - Already implemented correctly but not documented
**Priority**: 🟡 MEDIUM (Documentation)

### What's Missing

The test plan does **NOT** mention:
1. LLM mocking policy (mock in ALL tiers due to cost)
2. HAPI uses `MOCK_LLM_MODE=true`
3. Why LLM is mocked (cost constraint)

### Guideline Requirements (Lines 437-469)

**LLM Mocking Policy (Cost Constraint)**:

| Test Type | Infrastructure (DB, APIs) | LLM |
|-----------|---------------------------|-----|
| **Unit Tests** | Mock ✅ | Mock ✅ |
| **Integration Tests** | **REAL** ❌ No mocking | Mock ✅ (cost) |
| **E2E Tests** | **REAL** ❌ No mocking | Mock ✅ (cost) |

**Rationale**: LLM API calls incur significant costs per request. Mocking the LLM:
- Prevents runaway costs during test runs
- Allows deterministic, repeatable tests
- Still validates the complete integration with real infrastructure

### Current Implementation

AIAnalysis tests already use HAPI mock LLM (from previous fixes):
```go
// test/integration/aianalysis/suite_test.go
Env: map[string]string{
    "MOCK_LLM_MODE": "true", // ✅ Correct
    "LOG_LEVEL":     "INFO",
},
```

### Recommended Fix

**ADD to Test Plan** (Test Infrastructure section):

```markdown
### Mock LLM Policy (Cost Constraint)

**Authority**: TESTING_GUIDELINES.md Lines 437-469

**All test tiers use mock LLM** to prevent runaway API costs:

| Test Tier | HAPI | LLM | Rationale |
|-----------|------|-----|-----------|
| Unit | Mock | Mock | No real services |
| Integration | **Real** | **Mock** | Cost constraint |
| E2E | **Real** | **Mock** | Cost constraint |

**HAPI Configuration**:
```go
// test/integration/aianalysis/suite_test.go
hapiConfig := infrastructure.GenericContainerConfig{
    Env: map[string]string{
        "MOCK_LLM_MODE": "true", // Enables deterministic mock responses
        "LOG_LEVEL":     "INFO",
    },
}
```

**Mock Response Behavior**:
- HAPI returns deterministic responses based on signal type
- Example: `SignalType="OOMKilled"` → returns memory-related workflow
- Allows repeatable, predictable test scenarios
- No actual LLM API costs

**Reference**: `holmesgpt-api/src/mock_responses.py` - Mock response generator
```

---

## ⚠️ RECOMMENDATION #1: Add Eventually() Examples to Templates

**Guideline Reference**: Lines 659-694
**Impact**: MEDIUM - Improves test quality
**Priority**: ⚠️ RECOMMENDED

### Recommended Addition

**UPDATE Test Plan** - Test Structure Template:

```go
var _ = Describe("Error Classification - AA-INT-ERR", Label("integration", "error_handling"), func() {
    Context("HTTP Status Code Classification", func() {
        It("AA-INT-ERR-001: should classify 401 as authentication error", func() {
            By("Configuring mock HAPI to return 401")
            // Setup code

            By("Creating AIAnalysis CRD")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Waiting for reconciliation with Eventually()")
            // ✅ REQUIRED: Use Eventually(), NEVER time.Sleep()
            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                _ = k8sClient.Get(ctx, key, &updated)
                return updated.Status.Phase
            }, 30*time.Second, 1*time.Second).Should(Equal("Failed"))

            By("Verifying error classification")
            var final aianalysisv1.AIAnalysis
            Expect(k8sClient.Get(ctx, key, &final)).To(Succeed())
            Expect(final.Status.Reason).To(Equal("APIError"))
            Expect(final.Status.SubReason).To(Equal("AuthenticationError"))

            By("Verifying metrics recorded")
            // Metric validation with Eventually()
            Eventually(func() float64 {
                return testutil.GetMetricValue(metrics, "aianalysis_failures_total",
                    prometheus.Labels{"sub_reason": "AuthenticationError"})
            }, 10*time.Second, 1*time.Second).Should(Equal(1.0))
        })
    })
})
```

---

## ⚠️ RECOMMENDATION #2: Reference podman-compose Warning

**Guideline Reference**: Lines 1030-1199
**Impact**: LOW - Already implemented correctly
**Priority**: ⚠️ RECOMMENDED (Documentation)

### Recommended Addition

**ADD to Test Plan** (Background section):

```markdown
## ⚠️ Why NOT podman-compose?

**Historical Context**: AIAnalysis originally attempted to use `podman-compose` for integration test infrastructure but encountered race conditions (Dec 20, 2025).

**Problem**: `podman-compose up -d` starts services **simultaneously**:
```
PostgreSQL starts (takes 10-15s) ⏱️
Redis starts (takes 2-3s) ⏱️
DataStorage starts (connects IMMEDIATELY) ⚡ → ❌ Connection fails
HAPI starts (connects IMMEDIATELY) ⚡ → ❌ Connection fails
```

**Solution**: DD-TEST-002 Sequential Startup Pattern (Dec 21, 2025)
- Start services one at a time
- Wait for health check before starting next service
- Prevents race conditions and container crashes

**Reference**:
- [DD-TEST-002](../../architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md)
- [RO Team Debug Session](../../handoff/SHARED_RO_DS_INTEGRATION_DEBUG_DEC_20_2025.md)
```

---

## ⚠️ RECOMMENDATION #3: Add Kubeconfig Isolation (Future E2E)

**Guideline Reference**: Lines 1237-1332
**Impact**: LOW - E2E tests not in scope yet
**Priority**: ⚠️ FUTURE

### When E2E Tests Added

**ADD to Test Plan** (E2E section when created):

```markdown
## 🔐 Kubeconfig Isolation (E2E Tests)

**Authority**: TESTING_GUIDELINES.md Lines 1237-1332

AIAnalysis E2E tests MUST use service-specific kubeconfig:

**Location**: `~/.kube/aianalysis-e2e-config`
**Cluster Name**: `aianalysis-e2e`

```go
// test/e2e/aianalysis/aianalysis_e2e_suite_test.go
var _ = SynchronizedBeforeSuite(func() []byte {
    homeDir, _ := os.UserHomeDir()
    kubeconfigPath := fmt.Sprintf("%s/.kube/aianalysis-e2e-config", homeDir)

    err := infrastructure.CreateCluster("aianalysis-e2e", kubeconfigPath, GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())

    os.Setenv("KUBECONFIG", kubeconfigPath)
    return []byte(kubeconfigPath)
}, func(data []byte) {
    os.Setenv("KUBECONFIG", string(data))
})
```

**Why**: Prevents kubeconfig collisions when multiple service E2E tests run in parallel.
```

---

## 📊 COMPLIANCE SUMMARY

### Critical Gaps (Must Fix Before Phase 1)

| Gap # | Issue | Impact | Effort | Priority |
|-------|-------|--------|--------|----------|
| 1 | Missing V1.0 maturity tests | HIGH | 2-3 days | 🔴 CRITICAL |
| 2 | Skip() not documented as forbidden | HIGH | 30 min | 🔴 CRITICAL |
| 3 | Eventually() requirements missing | HIGH | 1 hour | 🔴 CRITICAL |
| 4 | Coverage target mismatch | MEDIUM | 1 hour | 🔴 CRITICAL |
| 5 | DD-TEST-002 not referenced | MEDIUM | 30 min | 🔴 CRITICAL |
| 6 | Metrics testing tier mismatch | HIGH | 1 hour | 🔴 CRITICAL |
| 7 | Audit validation pattern missing | HIGH | 1 hour | 🔴 CRITICAL |
| 8 | Mock LLM policy not documented | LOW | 30 min | 🟡 MEDIUM |

### Recommendations (Should Fix)

| Rec # | Issue | Impact | Effort | Priority |
|-------|-------|--------|--------|----------|
| 1 | Add Eventually() examples | MEDIUM | 1 hour | ⚠️ RECOMMENDED |
| 2 | Reference podman-compose warning | LOW | 30 min | ⚠️ RECOMMENDED |
| 3 | Add kubeconfig isolation (future) | LOW | 30 min | ⚠️ FUTURE |

---

## 🎯 RECOMMENDED ACTION PLAN

### Immediate (Before Phase 1 Implementation)

1. **Day 1 Morning**: Fix documentation gaps (#2, #3, #4, #5, #8)
   - Add Skip() forbidden section
   - Add Eventually() requirements
   - Update coverage targets
   - Add DD-TEST-002 reference
   - Add mock LLM policy
   - **Effort**: 3-4 hours

2. **Day 1 Afternoon**: Update test patterns (#6, #7, Rec #1)
   - Fix metrics testing pattern (registry vs HTTP)
   - Add audit validation pattern
   - Add Eventually() examples
   - **Effort**: 2-3 hours

3. **Day 2-4**: Add Phase 4 tests (Gap #1)
   - Create V1.0 maturity test file
   - Implement 8-12 maturity tests
   - **Effort**: 2-3 days

### Before Phase 1 Sign-Off

- [ ] All 8 critical gaps addressed
- [ ] Test plan updated with guidelines compliance
- [ ] Phase 4 (V1.0 maturity) tests added
- [ ] All recommendations implemented

---

## ✅ COMPLIANCE CHECKLIST

Use this checklist to verify test plan compliance:

### Documentation Compliance

- [ ] **Skip() Forbidden** - Explicitly documented with examples
- [ ] **Eventually() Required** - Pattern and timeouts documented
- [ ] **Coverage Targets** - Aligned with 70%/50%/50% guidelines
- [ ] **DD-TEST-002** - Sequential startup referenced
- [ ] **Mock LLM Policy** - Cost rationale documented

### Test Pattern Compliance

- [ ] **Metrics Testing** - Registry inspection for integration, HTTP for E2E
- [ ] **Audit Validation** - OpenAPI client usage documented
- [ ] **Eventually() Examples** - Included in test templates
- [ ] **Fail() Pattern** - Used instead of Skip() for missing services

### V1.0 Maturity Compliance

- [ ] **Phase 4 Added** - V1.0 maturity tests planned
- [ ] **Metrics Tests** - Integration (registry) + E2E (HTTP) tests
- [ ] **Audit Tests** - OpenAPI client validation of ALL fields
- [ ] **EventRecorder** - E2E tests for K8s events (future)
- [ ] **Graceful Shutdown** - Integration tests for audit flush (future)

---

## 📚 REFERENCES

- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **Test Plan**: `docs/testing/test-plans/AA_INTEGRATION_TEST_PLAN_V1.0.md`
- **DD-TEST-002**: `docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md`
- **DD-METRICS-001**: `docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md`
- **DD-AUDIT-003**: (Reference AIAnalysis audit requirements)

---

**Triage Complete**: December 24, 2025
**Next Action**: Update test plan per recommendations before Phase 1 implementation
**Owner**: AIAnalysis Service Team









