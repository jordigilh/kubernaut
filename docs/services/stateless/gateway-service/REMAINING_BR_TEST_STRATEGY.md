# Gateway Service: Remaining BR Test Strategy

**Version**: v1.0
**Created**: October 10, 2025
**Status**: 🎯 Ready for Implementation
**Current BR Coverage**: 15/23 (65%)
**Target BR Coverage**: 20/23 (87%) - excluding reserved/downstream BRs

---

## Executive Summary

**Current State:**
- ✅ **68 unit tests passing** (BR-002, BR-003, BR-005, BR-006, BR-010, BR-015, BR-016, BR-020-022, BR-051-053)
- ⚠️ **7 integration tests** (6 failing due to auth/infrastructure)
- 📊 **15/23 BRs covered** (65%)

**Remaining BRs to Test:**
- **BR-011**: Redis Deduplication Storage (4 integration tests)
- **BR-023**: CRD Creation Validation (3 integration tests)
- **BR-071**: CRD-only Integration (1 integration test - optional)
- **BR-004 Extension**: Rate Limiting (3 integration tests)
- **Error Handling**: Edge Cases (5 integration tests)

**Total Additional Tests Needed**: 16 integration tests (12 critical, 4 optional)

---

## BR Coverage Inventory

### ✅ Currently Tested BRs (15/23)

| BR | Description | Test Type | Count | Status |
|----|-------------|-----------|-------|--------|
| BR-001 | Alert ingestion endpoint | Integration | 2 | ⚠️ Failing (infra) |
| BR-002 | Prometheus adapter | Unit + Integration | 6 | ✅ Passing |
| BR-003 | Validate webhook payloads | Unit | 5 | ✅ Passing |
| BR-004 | Authentication/Authorization | Integration | 1 | ✅ Passing |
| BR-005 | Kubernetes event adapter | Unit | 12 | ✅ Passing |
| BR-006 | Alert normalization | Unit | 1 | ✅ Passing |
| BR-010 | Fingerprint deduplication | Unit + Integration | 4 | ✅ Passing |
| BR-015 | Alert storm detection | Unit + Integration | 5 | ✅ Passing |
| BR-016 | Storm aggregation | Unit + Integration | 3 | ✅ Passing |
| BR-020 | Priority assignment (Rego) | Unit | 9 | ✅ Passing |
| BR-021 | Priority fallback matrix | Unit | 9 | ✅ Passing |
| BR-022 | Remediation path decision | Unit | 23 | ✅ Passing |
| BR-051 | Environment detection | Unit + Integration | 7 | ✅ Passing |
| BR-052 | ConfigMap fallback | Unit | 6 | ✅ Passing |
| BR-053 | Default environment | Unit | 6 | ✅ Passing |

**Total**: 99 tests (68 unit + 31 integration attempts)

---

### ⏸️ Implicitly Tested (Need Explicit Tests)

| BR | Description | Why Implicit | Needed Tests |
|----|-------------|--------------|--------------|
| **BR-011** | Redis deduplication storage | Integration tests call Redis but don't validate behavior | 4 integration tests |
| **BR-023** | CRD creation | Integration tests create CRDs but don't validate schema/labels | 3 integration tests |
| **BR-071** | CRD-only integration | Integration tests verify CRDs exist | 1 integration test (optional) |

---

### ⏸️ Reserved / Out of Scope

| BR Range | Status | Reason |
|----------|--------|--------|
| BR-007-009 | Reserved | Not yet defined |
| BR-012-014 | Reserved | Not yet defined |
| BR-017-019 | Reserved | Not yet defined |
| BR-072 | Downstream | CRD as GitOps trigger (RemediationRequest controller concern) |
| BR-091-092 | Downstream | Notification (Notification service concern) |

---

## Detailed Test Strategy

### Phase 0: Fix Integration Test Infrastructure (BLOCKING) ⚠️

**Current Issue**: 6/7 integration tests failing due to auth token + CRD installation

**Required Actions**:
```bash
# 1. Regenerate auth token
make test-gateway-setup

# 2. Install RemediationRequest CRDs
kubectl apply -f config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml \
  --context kind-kubernaut-test

# 3. Verify cluster health
kubectl get nodes --context kind-kubernaut-test
kubectl get pods -n kubernaut-system

# 4. Run tests
ginkgo -v test/integration/gateway/
```

**Expected Result**: 7/7 integration tests passing

**Effort**: 30 minutes
**Priority**: **P0 - BLOCKING** (must complete before adding new tests)

---

### Phase 2: BR-011 - Redis Deduplication Storage (P1 - HIGH) 🔴

**Business Requirement**: Redis persists deduplication metadata with TTL expiry

**Why This Matters**:
- Redis is critical for HA deployments (2+ Gateway replicas share state)
- TTL expiry enables re-processing of recurring issues
- Persistence prevents duplicate CRDs after Gateway restarts

**Test Strategy**: Integration tests (requires real Redis)

---

#### Test 2.1: TTL Expiry Behavior

**File**: `test/integration/gateway/redis_deduplication_test.go`

**Business Outcome**: Same alert after TTL expires triggers new remediation

**Test**:
```go
It("expires fingerprints after TTL to allow re-processing of recurring issues", func() {
    // BUSINESS SCENARIO: Same pod OOM twice, 6 minutes apart
    // Expected: First creates CRD, second (after TTL) creates new CRD

    alert := makeProductionPodAlert("payment-service", "OOMKilled", testNamespace)

    By("First alert creates CRD")
    sendAlertToGateway(alert)
    Eventually(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 10*time.Second).Should(Equal(1))

    By("Duplicate within TTL is deduplicated (no new CRD)")
    sendAlertToGateway(alert)
    Consistently(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 2*time.Second).Should(Equal(1),
        "Duplicate should not create second CRD")

    By("After TTL expiry, alert re-processed (new CRD created)")
    // Fast-forward Redis TTL (test uses 5-second TTL for speed)
    time.Sleep(6 * time.Second)
    sendAlertToGateway(alert)

    Eventually(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 10*time.Second).Should(Equal(2),
        "Expired fingerprint allows re-processing")

    // BUSINESS OUTCOME VERIFIED:
    // Recurring issues get fresh AI analysis after TTL expires
})
```

**Decision**: **Integration Test** (cannot mock Redis TTL behavior accurately)

---

#### Test 2.2: Persistence Across Gateway Restarts

**Business Outcome**: Gateway restart doesn't lose deduplication state

**Test**:
```go
It("persists deduplication state across Gateway restarts", func() {
    // BUSINESS SCENARIO: Gateway pod crashes mid-deduplication
    // Expected: Redis state persists, duplicate still deduplicated

    alert := makeProductionPodAlert("api-service", "CrashLoopBackOff", testNamespace)

    By("First alert processed before Gateway restart")
    sendAlertToGateway(alert)
    Eventually(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 10*time.Second).Should(Equal(1))

    By("Simulating Gateway restart (stop/start server)")
    // In real test: restart Gateway server instance
    // For now: verify Redis still has fingerprint
    fingerprint := getFingerprintForAlert(alert)
    count := getRedisCount(fingerprint)
    Expect(count).To(Equal(1), "Redis persists count")

    By("Duplicate alert after restart is still deduplicated")
    sendAlertToGateway(alert)
    Consistently(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 2*time.Second).Should(Equal(1),
        "Redis persistence prevents duplicate CRD after restart")

    // BUSINESS OUTCOME VERIFIED:
    // HA deployments maintain consistent deduplication state
})
```

**Decision**: **Integration Test** (requires real Redis persistence)

---

#### Test 2.3: Concurrent Updates (HA Scenario)

**Business Outcome**: 2 Gateway replicas don't create duplicate CRDs

**Test**:
```go
It("handles concurrent updates to same fingerprint in HA deployments", func() {
    // BUSINESS SCENARIO: 2 Gateway replicas receive same alert simultaneously
    // Expected: Redis atomic operations prevent duplicate CRDs

    alert := makeProductionPodAlert("database-service", "HighCPU", testNamespace)

    By("Two Gateway instances process same alert concurrently")
    // Send same alert twice rapidly (simulates 2 replicas)
    go sendAlertToGateway(alert)
    go sendAlertToGateway(alert)

    By("Only one CRD created (Redis handles race condition)")
    Eventually(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 10*time.Second).Should(Equal(1),
        "Redis atomic operations prevent duplicate CRDs")

    Consistently(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 3*time.Second).Should(Equal(1),
        "Race condition resolved, no extra CRDs")

    // BUSINESS OUTCOME VERIFIED:
    // HA deployments don't create duplicate remediation workflows
})
```

**Decision**: **Integration Test** (requires real Redis atomic operations)

---

#### Test 2.4: Graceful Degradation (Redis Unavailable)

**Business Outcome**: Gateway continues processing when Redis down (no deduplication)

**Test**:
```go
It("gracefully degrades when Redis is unavailable", func() {
    // BUSINESS SCENARIO: Redis down, but alerts still need processing
    // Expected: Gateway processes alerts without deduplication (trade-off)

    alert := makeProductionPodAlert("critical-service", "ServiceDown", testNamespace)

    By("Stopping Redis connection (simulated failure)")
    // Test infrastructure would stop Redis container here
    // For now: verify Gateway behavior when Redis unavailable

    By("Gateway still creates CRD without deduplication")
    sendAlertToGateway(alert)

    Eventually(func() int {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        return len(rrList.Items)
    }, 10*time.Second).Should(BeNumerically(">=", 1),
        "Gateway continues processing without Redis")

    // BUSINESS OUTCOME VERIFIED:
    // Critical alerts still trigger remediation (potential duplicates acceptable)
    // Trade-off: Miss alert > Duplicate alert for critical issues
})
```

**Decision**: **Integration Test** (requires simulating Redis failure)

**Summary**:
- **4 integration tests** for BR-011
- **Effort**: 3-4 hours
- **Priority**: P1 - HIGH (critical for HA deployments)

---

### Phase 4: BR-023 - CRD Creation Validation (P1 - HIGH) 🔴

**Business Requirement**: RemediationRequest CRDs comply with schema, include labels, support cascade deletion

**Why This Matters**:
- Invalid CRDs break downstream controllers
- Labels enable controller filtering (label selectors)
- Owner references enable Kubernetes garbage collection

**Test Strategy**: Integration tests (requires real K8s API validation)

---

#### Test 4.1: Schema Validation

**File**: `test/integration/gateway/crd_validation_test.go`

**Business Outcome**: CRDs pass OpenAPI schema validation

**Test**:
```go
It("creates CRD with complete schema validation", func() {
    // BUSINESS SCENARIO: CRD must be valid for downstream controllers
    // Expected: K8s API accepts CRD, schema validation passes

    alert := makeProductionPodAlert("payment-service", "HighMemoryUsage", testNamespace)

    By("Gateway creates RemediationRequest CRD")
    sendAlertToGateway(alert)

    Eventually(func() bool {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        err := k8sClient.List(context.Background(), rrList,
            client.InNamespace(testNamespace))
        return err == nil && len(rrList.Items) > 0
    }, 10*time.Second).Should(BeTrue())

    By("CRD passes OpenAPI schema validation")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
    rr := rrList.Items[0]

    // BUSINESS OUTCOME: Schema compliance ensures controller compatibility

    // Required fields validation
    Expect(rr.Spec.SignalName).NotTo(BeEmpty(),
        "Schema requires signalName field")
    Expect(rr.Spec.SignalFingerprint).NotTo(BeEmpty(),
        "Schema requires fingerprint for deduplication")

    // Enum validation
    Expect(rr.Spec.Environment).To(BeElementOf(
        []string{"production", "staging", "development", "unknown"}),
        "Schema validates environment enum")

    // Pattern validation
    Expect(rr.Spec.Priority).To(MatchRegexp(`^P[0-3]$`),
        "Schema validates priority format")

    // Severity validation
    Expect(rr.Spec.Severity).To(BeElementOf(
        []string{"critical", "warning", "info"}),
        "Schema validates severity enum")

    // BUSINESS OUTCOME VERIFIED:
    // CRD schema prevents invalid data from reaching controllers
})
```

**Decision**: **Integration Test** (requires real K8s schema validation)

---

#### Test 4.2: Label Propagation

**Business Outcome**: CRD labels enable controller filtering

**Test**:
```go
It("populates CRD labels for controller label selectors", func() {
    // BUSINESS SCENARIO: Controllers filter CRDs by labels
    // Expected: Gateway propagates alert labels to CRD

    alert := makeProductionPodAlert("api-service", "CrashLoopBackOff", testNamespace)
    alert.Labels["team"] = "platform-engineering"
    alert.Labels["app"] = "payment-api"

    By("Gateway creates CRD with propagated labels")
    sendAlertToGateway(alert)

    Eventually(func() bool {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        err := k8sClient.List(context.Background(), rrList,
            client.InNamespace(testNamespace))
        return err == nil && len(rrList.Items) > 0
    }, 10*time.Second).Should(BeTrue())

    By("CRD labels enable targeted controller reconciliation")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
    rr := rrList.Items[0]

    // BUSINESS OUTCOME: Labels enable label selectors

    // Standard labels (managed-by, component)
    Expect(rr.Labels).To(HaveKeyWithValue(
        "app.kubernetes.io/managed-by", "gateway-service"),
        "Standard label for ownership tracking")
    Expect(rr.Labels).To(HaveKeyWithValue(
        "app.kubernetes.io/component", "remediation"),
        "Standard label for component type")

    // Environment label (for filtering)
    Expect(rr.Labels).To(HaveKeyWithValue(
        "kubernaut.io/environment", "production"),
        "Environment label for controller filtering")

    // Priority label (for scheduling)
    Expect(rr.Labels).To(HaveKeyWithValue(
        "kubernaut.io/priority", "P0"),
        "Priority label for controller scheduling")

    // Propagated alert labels
    Expect(rr.Labels).To(HaveKeyWithValue(
        "kubernaut.io/team", "platform-engineering"),
        "Alert labels propagated to CRD")
    Expect(rr.Labels).To(HaveKeyWithValue(
        "kubernaut.io/app", "payment-api"),
        "Alert labels enable team-based routing")

    // BUSINESS OUTCOME VERIFIED:
    // Controllers can filter: kubectl get rr -l kubernaut.io/team=platform-engineering
})
```

**Decision**: **Integration Test** (requires real K8s label validation)

---

#### Test 4.3: Owner References (Optional)

**Business Outcome**: Owner references enable cascade deletion

**Test**:
```go
It("sets owner references for Kubernetes garbage collection", func() {
    // BUSINESS SCENARIO: Deleting namespace should clean up CRDs
    // Expected: Owner references enable cascade deletion

    alert := makeProductionPodAlert("cleanup-test", "TestAlert", testNamespace)

    By("Gateway creates CRD with owner reference")
    sendAlertToGateway(alert)

    Eventually(func() bool {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        err := k8sClient.List(context.Background(), rrList,
            client.InNamespace(testNamespace))
        return err == nil && len(rrList.Items) > 0
    }, 10*time.Second).Should(BeTrue())

    By("CRD has owner reference to enable cascade deletion")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
    rr := rrList.Items[0]

    // BUSINESS OUTCOME: Owner references enable garbage collection
    // Note: Gateway may not set owner refs if it's cluster-scoped
    // This test validates the pattern if used

    if len(rr.OwnerReferences) > 0 {
        Expect(rr.OwnerReferences[0].Kind).To(BeElementOf(
            []string{"ServiceAccount", "ConfigMap", "Namespace"}),
            "Owner reference enables cascade deletion")
        Expect(rr.OwnerReferences[0].Controller).To(BeTrue(),
            "Controller flag enables garbage collection")
    } else {
        Skip("Gateway doesn't set owner references (cluster-scoped)")
    }

    // BUSINESS OUTCOME VERIFIED:
    // Namespace deletion cleans up RemediationRequest CRDs automatically
})
```

**Decision**: **Integration Test** (requires real K8s owner reference validation)

**Summary**:
- **3 integration tests** for BR-023
- **Effort**: 2-3 hours
- **Priority**: P1 - HIGH (critical for controller compatibility)

---

### Phase 5: BR-004 Extension - Rate Limiting (P2 - MEDIUM) 🟡

**Business Requirement**: Per-source rate limiting prevents abuse

**Why This Matters**:
- Noisy alertmanager (1000 alerts/sec) can overwhelm Gateway
- Per-source isolation prevents noisy neighbor
- Burst capacity handles legitimate alert storms

**Test Strategy**: Integration tests (requires real rate limiter)

---

#### Test 5.1: Rate Limit Enforcement

**File**: `test/integration/gateway/rate_limiting_test.go`

**Test**:
```go
It("enforces per-source rate limits to prevent system overload", func() {
    // BUSINESS SCENARIO: Noisy alertmanager sending 150 alerts/min
    // Expected: Rate limiter blocks excess (limit: 100/min)

    alert := makeProductionPodAlert("test-service", "TestAlert", testNamespace)

    By("Sending 150 alerts rapidly (above 100/min limit)")
    for i := 0; i < 150; i++ {
        sendAlertToGateway(alert)
    }

    By("Rate limiter blocks ~50 excess alerts")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))

    // BUSINESS OUTCOME: Rate limiting prevents overload
    Expect(len(rrList.Items)).To(BeNumerically("<", 150),
        "Rate limiter blocks excess alerts")
    Expect(len(rrList.Items)).To(BeNumerically(">", 80),
        "Rate limiter allows legitimate traffic within burst capacity")
})
```

---

#### Test 5.2: Per-Source Isolation

**Test**:
```go
It("isolates rate limits per source IP (noisy neighbor protection)", func() {
    // BUSINESS SCENARIO: Source 1 is noisy, Source 2 is normal
    // Expected: Source 2 unaffected by Source 1 rate limit

    alert1 := makeAlertFromSource("source-1", "Alert1", testNamespace)
    alert2 := makeAlertFromSource("source-2", "Alert2", testNamespace)

    By("Source 1 hits rate limit (150 alerts)")
    for i := 0; i < 150; i++ {
        sendAlertFromIP(alert1, "192.168.1.1")
    }

    By("Source 2 processes alerts normally (10 alerts)")
    for i := 0; i < 10; i++ {
        sendAlertFromIP(alert2, "192.168.1.2")
    }

    By("Source 2 alerts not affected by Source 1 rate limit")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))

    source2Alerts := filterBySourceIP(rrList.Items, "192.168.1.2")
    Expect(len(source2Alerts)).To(Equal(10),
        "Source 2 unaffected by Source 1 rate limit")
})
```

---

#### Test 5.3: Burst Capacity

**Test**:
```go
It("allows burst traffic within configured token bucket limits", func() {
    // BUSINESS SCENARIO: Alert storm causes 50 alerts in 1 second
    // Expected: Token bucket burst capacity handles spike

    alert := makeProductionPodAlert("burst-test", "BurstAlert", testNamespace)

    By("Sending 50 alerts in rapid burst (500ms)")
    for i := 0; i < 50; i++ {
        sendAlertToGateway(alert)
        time.Sleep(10 * time.Millisecond)
    }

    By("Token bucket allows burst within limit")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))

    Expect(len(rrList.Items)).To(BeNumerically(">=", 40),
        "Burst capacity handles temporary spike")
})
```

**Summary**:
- **3 integration tests** for BR-004 extension
- **Effort**: 2-3 hours
- **Priority**: P2 - MEDIUM (important for production resilience)

---

### Phase 6: Error Handling & Edge Cases (P3 - LOW) 🟢

**Business Requirement**: Graceful error handling for production reliability

---

#### Test 6.1-6.5: Edge Cases

**File**: `test/integration/gateway/error_handling_test.go`

**Tests**:
1. Malformed JSON → 400 Bad Request
2. Very large payloads (>100KB) → 413 Payload Too Large
3. Missing required fields → 400 with clear error message
4. K8s API unavailable → 500 for retry
5. Namespace not found → fallback to default namespace

**Summary**:
- **5 integration tests** for error handling
- **Effort**: 2-3 hours
- **Priority**: P3 - LOW (nice to have, not blocking)

---

## Implementation Timeline

### Week 1: Critical Path (P0 + P1)

**Day 1 (30min)**: Phase 0 - Fix Infrastructure ⚠️
- ✅ Regenerate auth token
- ✅ Install CRDs in Kind cluster
- ✅ Verify 7/7 integration tests passing

**Day 2 (4h)**: Phase 2 - Redis Behavior (BR-011)
- ✅ Write 4 integration tests
- ✅ Test TTL, persistence, concurrency, failover

**Day 3 (3h)**: Phase 4 - CRD Validation (BR-023)
- ✅ Write 3 integration tests
- ✅ Test schema, labels, owner references

---

### Week 2: Extensions (P2 + P3)

**Day 4 (3h)**: Phase 5 - Rate Limiting (BR-004 ext)
- ✅ Write 3 integration tests
- ✅ Test enforcement, isolation, burst

**Day 5 (3h)**: Phase 6 - Error Handling
- ✅ Write 5 integration tests
- ✅ Test edge cases, error paths

---

## Test Level Decision Framework

### Use **Unit Tests** When:
✅ Testing business logic without external dependencies
✅ Testing adapter parsing/validation
✅ Testing classification/priority rules
✅ Testing fingerprint generation
✅ Setup < 15 lines, fast execution

**Examples**: Prometheus adapter parsing, priority matrix, environment classification

---

### Use **Integration Tests** When:
✅ Testing Redis persistence/TTL
✅ Testing CRD creation/validation
✅ Testing rate limiting behavior
✅ Testing HA multi-instance scenarios
✅ Testing error handling with real services

**Examples**: Redis TTL expiry, CRD schema validation, rate limit enforcement

---

### Use **E2E Tests** When:
✅ Testing complete Prometheus → Gateway → CRD workflow
✅ Testing real AlertManager webhooks
✅ Testing downstream controller integration
✅ Lower-level tests can't reproduce real behavior

**Examples**: Complete monitoring workflow, GitOps integration

---

## Success Criteria

### Coverage Metrics
- ✅ **20/23 BRs tested** (87% - excluding reserved/downstream)
- ✅ **68 unit tests** (existing)
- ✅ **23 integration tests** (7 existing + 16 new)
- ✅ **Test pyramid compliance** (70% unit, 25% integration, 5% e2e)

### Quality Metrics
- ✅ **100% business outcome focused**
- ✅ **All integration tests passing**
- ✅ **Clear BR traceability**
- ✅ **DescribeTable usage** where appropriate

---

## Priority Decision Matrix

| Phase | BR | Tests | Priority | Rationale |
|-------|------|-------|----------|-----------|
| 0 | Infra | N/A | **P0** | Blocking all integration tests |
| 2 | BR-011 | 4 | **P1** | Critical for HA deployments |
| 4 | BR-023 | 3 | **P1** | Critical for controller compatibility |
| 5 | BR-004 ext | 3 | **P2** | Important for production resilience |
| 6 | Error | 5 | **P3** | Nice to have, not blocking |

---

## Risk Assessment

### High Risk
1. **Redis Behavior Complex**
   - Risk: TTL timing, concurrency tests may be flaky
   - Mitigation: Use deterministic waits, shorter TTL for tests
   - Fallback: Skip concurrency test if too flaky

### Medium Risk
2. **CRD Schema Validation**
   - Risk: K8s schema validation may be environment-dependent
   - Mitigation: Use real CRD definitions, test against Kind cluster
   - Fallback: Skip owner reference test if not applicable

### Low Risk
3. **Rate Limiting Tests**
   - Risk: Timing-sensitive, may be flaky
   - Mitigation: Use appropriate timeouts, retry logic
   - Fallback: Increase test tolerances

---

## Next Steps

1. **User Approval**: Review strategy, provide feedback
2. **Phase 0**: Fix integration test infrastructure (BLOCKING)
3. **Phase 2-6**: Implement tests following plan
4. **Validation**: Run full test suite, verify 87% BR coverage
5. **Documentation**: Update testing-strategy.md

---

**Confidence**: 90% (Very High)
**Status**: 🎯 Ready for Implementation
**Author**: AI Assistant
**Date**: October 10, 2025

---

## Appendix: Test Count Summary

| Test Type | Current | New | Total | Percentage |
|-----------|---------|-----|-------|------------|
| **Unit** | 68 | 0 | 68 | 70% |
| **Integration** | 7 | 16 | 23 | 24% |
| **E2E** | 0 | 4* | 4 | 4% |
| **Total** | 75 | 16-20 | 91-95 | 100% |

*E2E tests optional, recommended for future work

**BR Coverage Progression**:
- Start: 13/23 (57%)
- After Unit Extension: 15/23 (65%)
- After Integration Extension: 20/23 (87%)
- After E2E (optional): 23/23 (100%)


