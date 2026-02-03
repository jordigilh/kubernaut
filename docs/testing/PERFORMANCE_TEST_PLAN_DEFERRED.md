# Performance Test Plan (Deferred)

**Status**: 🟡 Deferred to Future Sprint  
**Created**: 2026-02-03  
**Reason**: Performance testing not prioritized for v1.0

---

## Overview

This document captures performance and operational E2E tests that were identified but deferred to a future sprint when performance benchmarking becomes a priority.

**Source**: Tests originally in `test/e2e/remediationorchestrator/operational_e2e_test.go` (removed 2026-02-03)

---

## Test Scenarios

### 1. Reconcile Performance - Timing SLO

**Business Value**: Validates RemediationOrchestrator reconcile loop performance SLOs

**Test ID**: `E2E-RO-PERF-001`  
**Priority**: P2 (Performance optimization)  
**Confidence**: 90%

**Scenario**:
- **Given**: RemediationRequest created in test namespace
- **When**: RO reconcile loop processes the RR
- **Then**: SignalProcessing CRD should be created within **<1 second**

**Success Criteria**:
- RR → SP creation time < 1s (baseline)
- Relaxed timeout: 5s (to account for test environment overhead)

**Implementation Notes**:
- Uses `time.Since(startTime)` to measure elapsed time
- Already works functionally (RO creates SP CRDs successfully)
- Performance validation deferred until performance benchmarking sprint

**Test Code Reference** (removed from codebase):
```go
It("should complete initial reconcile loop quickly (<1s baseline)", func() {
    startTime := time.Now()
    
    // Create RemediationRequest
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())
    
    // Wait for SignalProcessing creation
    Eventually(func() error {
        spList := &signalprocessingv1.SignalProcessingList{}
        if err := k8sClient.List(ctx, spList, client.InNamespace(namespace)); err != nil {
            return err
        }
        if len(spList.Items) == 0 {
            return fmt.Errorf("no SP created yet")
        }
        return nil
    }, "5s", "100ms").Should(Succeed())
    
    elapsed := time.Since(startTime)
    Expect(elapsed).To(BeNumerically("<", 5*time.Second))
})
```

---

### 2. Cross-Namespace Isolation (Multi-Tenancy)

**Business Value**: Multi-tenant isolation guarantee - ensures RRs in different namespaces don't interfere

**Test ID**: `E2E-RO-ISO-001`  
**Priority**: P3 (Already implicitly validated)  
**Confidence**: 95%

**Scenario**:
- **Given**: RR in namespace A (that will fail) + RR in namespace B (that will succeed)
- **When**: Both RRs are processed concurrently
- **Then**: 
  - RR in namespace B completes successfully
  - RR in namespace A fails independently
  - No cross-namespace interference

**Success Criteria**:
- RR-B reaches `Completed` phase
- RR-A failure does not affect RR-B processing
- Namespace isolation maintained

**Current Status**: 
- ✅ **Already Validated Implicitly**: All existing E2E tests run in parallel in separate namespaces without interference
- ✅ **Kubernetes Native**: Namespace isolation is a Kubernetes guarantee, not a controller concern
- ✅ **Functional Validation Exists**: Tests in `needs_human_review_e2e_test.go`, `approval_e2e_test.go`, `lifecycle_e2e_test.go` all run concurrently in different namespaces

**Implementation Notes**:
- This test is **redundant** - namespace isolation is already proven by parallel E2E execution
- If re-implemented, focus on **business logic isolation** (e.g., RBAC, resource quotas) rather than basic namespace separation

**Test Code Reference** (removed from codebase):
```go
It("should process RRs in different namespaces independently", func() {
    // Create two namespaces
    nsA := createNamespace("ns-isolation-a-e2e")
    nsB := createNamespace("ns-isolation-b-e2e")
    
    // Create RR in namespace A (will fail)
    rrA := createRemediationRequest("rr-fail", nsA)
    
    // Create RR in namespace B (will succeed)
    rrB := createRemediationRequest("rr-success", nsB)
    
    // Validate RR-B completes successfully
    Eventually(func() bool {
        var rr remediationv1.RemediationRequest
        if err := k8sClient.Get(ctx, client.ObjectKey{Name: "rr-success", Namespace: nsB}, &rr); err != nil {
            return false
        }
        return rr.Status.Phase == "Completed"
    }, timeout, interval).Should(BeTrue())
    
    // Validate RR-A fails independently (no cross-namespace impact)
    Eventually(func() bool {
        var rr remediationv1.RemediationRequest
        if err := k8sClient.Get(ctx, client.ObjectKey{Name: "rr-fail", Namespace: nsA}, &rr); err != nil {
            return false
        }
        return rr.Status.Phase == "Failed"
    }, timeout, interval).Should(BeTrue())
})
```

---

## When to Implement

### Triggers for Performance Test Implementation:
1. **Performance Regression Investigation**: When users report slow RR processing
2. **Scalability Testing**: Before scaling to high-volume production environments
3. **Performance Optimization Sprint**: When performance becomes a priority
4. **SLO Definition**: When formal performance SLOs are established for v1.1+

### Triggers for Namespace Isolation Test Implementation:
1. **Never** (redundant with existing parallel test execution)
2. **Alternative**: If business logic isolation (RBAC, quotas) is added, create focused tests for those features

---

## Current E2E Coverage (v1.0)

**Functional Validation** ✅:
- RO creates SP CRDs (validated in all E2E tests)
- Parallel test execution in separate namespaces (implicit isolation validation)
- Complete RR lifecycle (validated in `lifecycle_e2e_test.go`)
- Human review workflows (validated in `needs_human_review_e2e_test.go`)
- RAR approval audit trail (validated in `approval_e2e_test.go`)

**Performance Validation** 🟡:
- Deferred to future sprint

---

## Test Environment Requirements

When implementing performance tests:
- **Infrastructure**: Dedicated performance test cluster (isolated from functional tests)
- **Metrics**: Prometheus + Grafana for real-time performance monitoring
- **Load Generation**: Tools for simulating high RR volume (e.g., k6, Locust)
- **Baseline Establishment**: Run tests in controlled environment to establish SLO baselines

---

## References

- **Original File**: `test/e2e/remediationorchestrator/operational_e2e_test.go` (removed 2026-02-03)
- **Related ADRs**: 
  - ADR-001: Flat CRD hierarchy (namespace isolation by design)
  - DD-TEST-006: Test plan policy
- **E2E Test Suite**: `test/e2e/remediationorchestrator/`
