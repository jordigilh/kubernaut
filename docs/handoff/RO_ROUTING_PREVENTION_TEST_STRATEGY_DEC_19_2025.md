# RO Routing Prevention Test Strategy - Defense-in-Depth Analysis

**Date**: December 19, 2025
**Question**: Should RO routing prevention have integration tests, or is E2E sufficient?
**Answer**: ✅ **BOTH REQUIRED** - Integration tests are MANDATORY per defense-in-depth strategy
**Confidence**: 98%

---

## Executive Summary

**Recommendation**: RO routing prevention logic **MUST have integration tests** + strategic E2E validation.

**Rationale**: Per 03-testing-strategy.mdc (authoritative):
- Integration tests (>50%): "CRD-based coordination between services" (line 98)
- E2E tests (10-15%): "Complete business scenarios" for critical journeys (line 119)

**RO routing prevention is CRD-based coordination** → Integration tests MANDATORY

---

## Defense-in-Depth Analysis

### What is RO Routing Prevention Logic?

**Functionality**:
```go
// BEFORE creating WorkflowExecution CRD:
func (c *WorkflowExecutionCreator) Create(...) {
    // 1. Query K8s API for previous WFEs
    previousWFE := c.findMostRecentWFE(ctx, targetResource)

    // 2. Check WasExecutionFailure
    if previousWFE.Status.FailureDetails.WasExecutionFailure {
        return nil, &PreviousExecutionFailedError{...}
    }

    // 3. Check MaxConsecutiveFailures
    if previousWFE.Status.ConsecutiveFailures >= 5 {
        return nil, &ExhaustedRetriesError{...}
    }

    // 4. Check NextAllowedExecution
    if time.Now().Before(previousWFE.Status.NextAllowedExecution.Time) {
        return nil, &BackoffActiveError{...}
    }

    // 5. Create WFE or update RR.Status with skip reason
}
```

**Key Characteristics**:
- ✅ Queries K8s API (WorkflowExecution CRDs)
- ✅ Cross-service coordination (RO → WE CRDs)
- ✅ CRD status propagation (RR.Status updates)
- ✅ Multiple decision paths (skip vs create)

---

## Authoritative Testing Strategy (03-testing-strategy.mdc)

### Integration Tests (>50%) - MANDATORY for This Logic

**From lines 89-103**:
> **Purpose**: Cross-service behavior, data flow validation, and **microservices coordination**
>
> **MICROSERVICES INTEGRATION FOCUS**: In a microservices architecture, integration tests must cover:
> - **CRD-based coordination between services** ✅ THIS!
> - Watch-based status propagation ✅ THIS!
> - Owner reference lifecycle management
> - Cross-service error handling ✅ THIS!
> - Service discovery and communication patterns

**Confidence**: 80-85%

**Strategy**: "Focus on cross-service flows, CRD coordination, and service-to-service integration with **real business logic**"

---

### E2E Tests (10-15%) - Strategic Validation Only

**From lines 119-131**:
> **Purpose**: Complete end-to-end business workflow validation across all services
>
> **Coverage Mandate**: **10-15% of total business requirements** for critical user journeys
>
> **E2E FOCUS**: Test complete alert-to-resolution journeys:
> - Alert ingestion → Processing → AI Analysis → **Workflow Execution** → Kubernetes Execution → Resolution

**Confidence**: 90-95%

**Strategy**: "Complete business scenarios with minimal mocking, focusing on **critical remediation workflows**"

---

## Classification: RO Routing Prevention

### Is This Integration Test Territory? ✅ YES

| Characteristic | Integration Test Criteria | RO Routing Matches? |
|---|---|---|
| **CRD-based coordination** | ✅ Required | ✅ YES (queries WFE CRDs) |
| **Cross-service interaction** | ✅ Required | ✅ YES (RO → WE) |
| **Status propagation** | ✅ Required | ✅ YES (RR.Status updates) |
| **K8s API queries** | ✅ Required | ✅ YES (findMostRecentWFE) |
| **Service-to-service error handling** | ✅ Required | ✅ YES (skip reasons) |

**Verdict**: **MANDATORY integration test coverage** per defense-in-depth strategy.

---

### Is This E2E Test Territory? ✅ YES (But Strategic)

| Characteristic | E2E Test Criteria | RO Routing Matches? |
|---|---|---|
| **Complete business workflow** | ✅ Required | ✅ YES (alert → routing → execution) |
| **Critical user journey** | ✅ Required | ✅ YES (prevents dangerous retries) |
| **Multi-service coordination** | ✅ Required | ✅ YES (Gateway → RO → WE) |
| **Minimal mocking** | ✅ Required | ✅ YES (real Tekton, real failures) |

**Verdict**: **E2E validation for critical scenarios** (1-2 tests, not comprehensive).

---

## Recommended Test Distribution

### Unit Tests (70%+) ✅ REQUIRED

**What to Test**:
- Routing decision logic (mock K8s API)
- Field comparisons (WasExecutionFailure, ConsecutiveFailures)
- Time calculations (NextAllowedExecution)
- Error construction (PreviousExecutionFailedError)

**Example**:
```go
// test/unit/remediationorchestrator/workflowexecution_creator_test.go

Describe("Routing Prevention Logic", func() {
    It("should detect PreviousExecutionFailed", func() {
        // Mock K8s client returns WFE with WasExecutionFailure=true
        mockWFE := &workflowexecutionv1.WorkflowExecution{
            Status: workflowexecutionv1.WorkflowExecutionStatus{
                FailureDetails: &workflowexecutionv1.FailureDetails{
                    WasExecutionFailure: true,
                    Reason: "TaskFailed",
                },
            },
        }

        // Mock findMostRecentWFE
        creator.k8sClient = fakeclient.NewClientBuilder().
            WithObjects(mockWFE).Build()

        // Call Create
        wfe, err := creator.Create(ctx, rr, aiAnalysis)

        // Verify skip decision
        Expect(err).To(HaveOccurred())
        Expect(err).To(BeAssignableToTypeOf(&PreviousExecutionFailedError{}))
        Expect(wfe).To(BeNil())
    })

    It("should detect ExhaustedRetries", func() {
        mockWFE := &workflowexecutionv1.WorkflowExecution{
            Status: workflowexecutionv1.WorkflowExecutionStatus{
                ConsecutiveFailures: 5,
            },
        }
        // ... similar test
    })

    It("should detect active backoff window", func() {
        nextAllowed := metav1.NewTime(time.Now().Add(5 * time.Minute))
        mockWFE := &workflowexecutionv1.WorkflowExecution{
            Status: workflowexecutionv1.WorkflowExecutionStatus{
                NextAllowedExecution: &nextAllowed,
            },
        }
        // ... similar test
    })
})
```

**Estimated**: 8-10 unit tests
**Execution Time**: <1s
**Coverage**: Decision logic, edge cases, error handling

---

### Integration Tests (>50%) ✅ MANDATORY

**What to Test**:
- **REAL K8s API queries** (EnvTest with real CRDs)
- CRD creation/skipping based on routing decisions
- RemediationRequest status updates
- Multiple WFE query scenarios (no previous, one previous, multiple)
- Cross-service coordination (RO writes RR.Status, WE reads it)

**Example**:
```go
// test/integration/remediationorchestrator/routing_integration_test.go

Describe("BR-ORCH-032: PreviousExecutionFailed Routing", func() {
    It("should NOT create new WFE when previous execution failed", func() {
        By("Creating first WorkflowExecution for target resource")
        targetResource := "default/deployment/test-app"
        wfe1 := createWFE("wfe-1", targetResource)
        Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

        By("Marking first WFE as execution failure")
        Eventually(func() error {
            updated, _ := getWFE(wfe1.Name)
            updated.Status.Phase = workflowexecutionv1.PhaseFailed
            updated.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
                WasExecutionFailure: true,
                Reason: "TaskFailed",
            }
            return k8sClient.Status().Update(ctx, updated)
        }).Should(Succeed())

        By("Attempting to create second WFE for same target resource")
        rr2 := createRemediationRequest("rr-2", targetResource)
        Expect(k8sClient.Create(ctx, rr2)).To(Succeed())

        By("Verifying RO does NOT create second WFE")
        Consistently(func() int {
            wfeList := &workflowexecutionv1.WorkflowExecutionList{}
            k8sClient.List(ctx, wfeList, client.MatchingFields{
                "spec.targetResource": targetResource,
            })
            return len(wfeList.Items)
        }, 10*time.Second).Should(Equal(1),
            "Only one WFE should exist (second was skipped)")

        By("Verifying RR2 has PreviousExecutionFailed skip reason")
        Eventually(func() string {
            updated, _ := getRemediationRequest(rr2.Name)
            return updated.Status.SkipReason
        }).Should(Equal("PreviousExecutionFailed"))

        By("Verifying RR2 is in Failed phase")
        updated, _ := getRemediationRequest(rr2.Name)
        Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
        Expect(updated.Status.RequiresManualReview).To(BeTrue())
    })

    It("should NOT create new WFE when consecutive failures exhausted", func() {
        targetResource := "default/deployment/exhausted-test"

        By("Creating WFE with 5 consecutive failures")
        wfe := createWFE("wfe-exhausted", targetResource)
        wfe.Status.ConsecutiveFailures = 5
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        By("Attempting to create new WFE")
        rr := createRemediationRequest("rr-exhausted", targetResource)
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RO skips with ExhaustedRetries")
        Eventually(func() string {
            updated, _ := getRemediationRequest(rr.Name)
            return updated.Status.SkipReason
        }).Should(Equal("ExhaustedRetries"))
    })

    It("should NOT create new WFE when backoff window active", func() {
        targetResource := "default/deployment/backoff-test"

        By("Creating WFE with active backoff")
        nextAllowed := metav1.NewTime(time.Now().Add(5 * time.Minute))
        wfe := createWFE("wfe-backoff", targetResource)
        wfe.Status.NextAllowedExecution = &nextAllowed
        wfe.Status.ConsecutiveFailures = 2
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        By("Attempting to create new WFE")
        rr := createRemediationRequest("rr-backoff", targetResource)
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RO skips with ExponentialBackoff")
        Eventually(func() string {
            updated, _ := getRemediationRequest(rr.Name)
            return updated.Status.SkipReason
        }).Should(Equal("ExponentialBackoff"))

        By("Verifying BlockedUntil is set")
        updated, _ := getRemediationRequest(rr.Name)
        Expect(updated.Status.BlockedUntil).ToNot(BeNil())
        Expect(updated.Status.BlockedUntil.Time).To(BeTemporally("~", nextAllowed.Time, 1*time.Second))
    })

    It("should CREATE new WFE when backoff window expired", func() {
        targetResource := "default/deployment/backoff-expired"

        By("Creating WFE with EXPIRED backoff")
        pastTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
        wfe := createWFE("wfe-expired", targetResource)
        wfe.Status.NextAllowedExecution = &pastTime
        wfe.Status.ConsecutiveFailures = 2
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        By("Attempting to create new WFE")
        rr := createRemediationRequest("rr-expired", targetResource)
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RO DOES create second WFE (backoff expired)")
        Eventually(func() int {
            wfeList := &workflowexecutionv1.WorkflowExecutionList{}
            k8sClient.List(ctx, wfeList, client.MatchingFields{
                "spec.targetResource": targetResource,
            })
            return len(wfeList.Items)
        }, 15*time.Second).Should(Equal(2),
            "Two WFEs should exist (backoff window expired)")
    })

    It("should CREATE new WFE when no previous execution exists", func() {
        targetResource := "default/deployment/first-execution"

        By("Creating RR with no previous WFE")
        rr := createRemediationRequest("rr-first", targetResource)
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RO creates WFE (no routing blocks)")
        Eventually(func() int {
            wfeList := &workflowexecutionv1.WorkflowExecutionList{}
            k8sClient.List(ctx, wfeList, client.MatchingFields{
                "spec.targetResource": targetResource,
            })
            return len(wfeList.Items)
        }, 15*time.Second).Should(Equal(1),
            "One WFE should exist (no previous execution to block)")
    })
})
```

**Estimated**: 5-7 integration tests
**Execution Time**: ~30-60s (EnvTest + reconciliation)
**Coverage**: CRD coordination, K8s API queries, status propagation

**Why Integration Tests Are MANDATORY**:
1. ✅ Validates **REAL K8s API queries** (not mocked)
2. ✅ Tests **CRD coordination** (RO → WFE CRD interaction)
3. ✅ Verifies **status propagation** (RR.Status updates)
4. ✅ Catches **field index issues** (queries by targetResource)
5. ✅ Tests **timing edge cases** (backoff expiration)

---

### E2E Tests (10-15%) ✅ STRATEGIC (1-2 tests)

**What to Test**:
- **Complete alert-to-resolution flow** with routing block
- **Real Tekton failures** triggering routing prevention
- **Multi-service coordination** (Gateway → RO → WE)

**Example**:
```go
// test/e2e/remediationorchestrator/routing_e2e_test.go

Describe("E2E: PreviousExecutionFailed Blocks Retry", func() {
    It("should prevent retry when previous workflow failed during execution", func() {
        By("Creating first RemediationRequest")
        alert := createAlert("critical-pod-crash")
        rr1 := createRemediationRequestFromAlert(alert)
        Expect(k8sClient.Create(ctx, rr1)).To(Succeed())

        By("Waiting for RO to create WorkflowExecution")
        Eventually(func() bool {
            wfe := getWorkflowExecutionForRR(rr1.Name)
            return wfe != nil
        }, 30*time.Second).Should(BeTrue())

        wfe1 := getWorkflowExecutionForRR(rr1.Name)

        By("Waiting for real Tekton PipelineRun to start")
        Eventually(func() string {
            updated := getWorkflowExecution(wfe1.Name)
            return updated.Status.Phase
        }, 60*time.Second).Should(Equal(workflowexecutionv1.PhaseRunning))

        By("Waiting for PipelineRun to fail with TaskFailed (execution failure)")
        // Real Tekton controller will fail the PipelineRun
        // WE controller detects failure and marks wasExecutionFailure=true
        Eventually(func() bool {
            updated := getWorkflowExecution(wfe1.Name)
            return updated.Status.Phase == workflowexecutionv1.PhaseFailed &&
                   updated.Status.FailureDetails != nil &&
                   updated.Status.FailureDetails.WasExecutionFailure
        }, 180*time.Second).Should(BeTrue(),
            "WFE should fail with execution failure")

        By("Creating second RemediationRequest for SAME alert/target")
        rr2 := createRemediationRequestFromAlert(alert)
        Expect(k8sClient.Create(ctx, rr2)).To(Succeed())

        By("Verifying RO does NOT create second WorkflowExecution")
        Consistently(func() int {
            wfeList := &workflowexecutionv1.WorkflowExecutionList{}
            k8sClient.List(ctx, wfeList)
            count := 0
            for _, wfe := range wfeList.Items {
                if wfe.Spec.TargetResource == wfe1.Spec.TargetResource {
                    count++
                }
            }
            return count
        }, 30*time.Second, 2*time.Second).Should(Equal(1),
            "Only first WFE should exist (second blocked by PreviousExecutionFailed)")

        By("Verifying second RR is marked with PreviousExecutionFailed")
        Eventually(func() string {
            updated := getRemediationRequest(rr2.Name)
            return updated.Status.SkipReason
        }, 30*time.Second).Should(Equal("PreviousExecutionFailed"))

        By("Verifying second RR requires manual review")
        updated := getRemediationRequest(rr2.Name)
        Expect(updated.Status.RequiresManualReview).To(BeTrue())
        Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))

        GinkgoWriter.Println("✅ E2E: Routing prevention protected cluster from dangerous retry")
    })
})
```

**Estimated**: 1-2 E2E tests
**Execution Time**: ~5-10min (Kind cluster + real Tekton)
**Coverage**: Critical user journey, multi-service coordination

**Why E2E Tests Are STRATEGIC (Not Comprehensive)**:
1. ✅ Validates **complete business workflow** (alert → routing → blocking)
2. ✅ Tests **real Tekton failures** (not simulated)
3. ✅ Verifies **multi-service coordination** (Gateway → RO → WE)
4. ❌ Too slow for comprehensive coverage (10min per test)
5. ❌ Should focus on critical paths only (10-15% target)

---

## Answer: Both Required!

### Defense-in-Depth Distribution

```
╔═══════════════════════════════════════════════════════════╗
║     RO ROUTING PREVENTION TEST COVERAGE STRATEGY         ║
╠═══════════════════════════════════════════════════════════╣
║ TIER            │ TESTS   │ WHAT TO TEST         │ WHY   ║
╠─────────────────┼─────────┼──────────────────────┼───────╣
║ Unit Tests      │ 8-10    │ Decision logic       │ Fast  ║
║                 │         │ Field comparisons    │ Edge  ║
║                 │         │ Time calculations    │ Cases ║
║                 │         │ Mock K8s client      │       ║
╠─────────────────┼─────────┼──────────────────────┼───────╣
║ Integration     │ 5-7     │ REAL K8s API queries │ CRD   ║
║ ✅ MANDATORY    │         │ CRD coordination     │ Coord ║
║                 │         │ Status propagation   │       ║
║                 │         │ EnvTest with CRDs    │       ║
╠─────────────────┼─────────┼──────────────────────┼───────╣
║ E2E Tests       │ 1-2     │ Complete workflows   │ Crit  ║
║ ✅ STRATEGIC    │         │ Real Tekton failures │ Path  ║
║                 │         │ Multi-service coord  │ Only  ║
╠─────────────────┼─────────┼──────────────────────┼───────╣
║ TOTAL           │ 14-19   │ Comprehensive        │ ✅    ║
╚═══════════════════════════════════════════════════════════╝
```

---

## Rationale: Why Integration Tests Are MANDATORY

### 1. Authoritative Requirement (03-testing-strategy.mdc)

**Line 98**: "CRD-based coordination between services"
**Line 99**: "Watch-based status propagation"
**Line 101**: "Cross-service error handling"

**RO routing prevention is ALL THREE** → Integration tests MANDATORY.

---

### 2. What Integration Tests Validate That Unit/E2E Can't

| Validation | Unit Tests | Integration Tests | E2E Tests |
|---|---|---|---|
| **Decision logic** | ✅ With mocks | ✅ With real CRDs | ✅ Complete flow |
| **K8s API queries** | ❌ Mocked | ✅ **REAL** | ✅ Real |
| **Field index performance** | ❌ Can't test | ✅ **VALIDATES** | ❌ Too slow to test |
| **CRD coordination** | ❌ Mocked | ✅ **REAL** | ✅ Real |
| **Edge cases (timing)** | ✅ Fast | ✅ **FAST** | ❌ Too slow |
| **Status propagation** | ❌ Mocked | ✅ **REAL** | ✅ Real |
| **Query by targetResource** | ❌ Can't validate | ✅ **VALIDATES** | ✅ Real but slow |

**Integration tests are the ONLY tier that can validate CRD coordination quickly and comprehensively.**

---

### 3. Cost-Benefit Analysis

**Unit Tests**:
- Cost: ~1 hour to write 8-10 tests
- Benefit: Fast edge case coverage (<1s execution)
- Coverage: Decision logic only (no real K8s)

**Integration Tests**:
- Cost: ~2-3 hours to write 5-7 tests
- Benefit: **CRD coordination validated** (~30-60s execution)
- Coverage: **K8s API queries, status propagation, field indexes**

**E2E Tests**:
- Cost: ~2 hours to write 1-2 tests
- Benefit: Complete workflow validation (~10min execution)
- Coverage: Critical user journey only

**Conclusion**: Integration tests provide **MAXIMUM validation per time invested** for CRD coordination logic.

---

## Recommendation

### Phase 1: Unit Tests (FIRST)
- 8-10 tests for decision logic
- Mock K8s client
- Fast feedback (<1s)
- Estimated: 1-2 hours

### Phase 2: Integration Tests (REQUIRED)
- 5-7 tests for CRD coordination
- Real K8s API (EnvTest)
- Validate queries, status propagation
- Estimated: 2-3 hours

### Phase 3: E2E Tests (STRATEGIC)
- 1-2 tests for critical paths
- Complete business workflow
- Real Tekton failures
- Estimated: 2 hours

**Total Effort**: 5-7 hours for comprehensive coverage

---

## Confidence Assessment

**Answer Confidence**: 98%

**Rationale**:
1. ✅ Authoritative strategy clearly mandates integration tests for CRD coordination
2. ✅ RO routing prevention is textbook CRD coordination logic
3. ✅ Integration tests provide unique validation (field indexes, queries)
4. ✅ E2E tests alone would miss edge cases and be too slow

**Remaining 2% Risk**: Interpretation of "critical user journeys" could argue for E2E-only, but the >50% integration requirement for microservices coordination is explicit.

---

## Summary

**Question**: Should RO routing prevention have integration tests or is E2E enough?

**Answer**: ✅ **BOTH REQUIRED**

**Distribution**:
- Unit Tests: 8-10 tests (decision logic, mocks)
- **Integration Tests**: 5-7 tests (CRD coordination - MANDATORY)
- E2E Tests: 1-2 tests (critical paths - strategic)

**Rationale**: Per 03-testing-strategy.mdc, CRD-based coordination requires >50% integration coverage. E2E tests alone would be:
- Too slow for comprehensive coverage (10min per test)
- Miss edge cases (timing, field indexes)
- Violate defense-in-depth pyramid (10-15% E2E target)

**Confidence**: 98% - Integration tests are MANDATORY for microservices CRD coordination logic.

---

**Document Version**: 1.0
**Date**: December 19, 2025
**Status**: ✅ COMPLETE
**Authoritative Source**: 03-testing-strategy.mdc (lines 89-131)

