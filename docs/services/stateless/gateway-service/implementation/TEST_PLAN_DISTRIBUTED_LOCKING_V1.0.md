# Gateway Service - Distributed Locking Test Plan V1.0

**Version**: 1.0.0
**Created**: December 30, 2025
**Status**: Active
**Purpose**: Comprehensive test plan for DD-GATEWAY-013 distributed locking implementation
**Service Type**: Stateless HTTP API
**Team**: Gateway Team
**Implementation Plan**: [IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md](./IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md)

---

## Overview

This test plan validates the K8s Lease-based distributed locking mechanism for multi-replica Gateway deployments. The goal is to eliminate duplicate RemediationRequest creation when multiple Gateway pods process concurrent signals with the same fingerprint.

**Reference Documents**:
- [DD-GATEWAY-013](../../../../architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md) - Distributed locking design decision
- [TESTING_GUIDELINES.md](../../../../docs/development/TESTING_GUIDELINES.md) - Testing standards
- [IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md](./IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md) - Implementation details

---

## Test Strategy

### Test Pyramid Distribution

| Test Tier | Coverage | Focus | Duration |
|-----------|----------|-------|----------|
| **Unit Tests** | 90%+ | Distributed lock manager logic, error handling | ~2 hours |
| **Integration Tests** | Multi-replica simulation | Concurrent signal processing, K8s Lease operations | ~3 hours |
| **E2E Tests** | Production deployment | 3-replica Gateway with 100 concurrent signals | ~2 hours |

### Success Criteria

- ✅ Zero duplicate RRs created with multi-replica deployment
- ✅ Lock acquisition failure rate <0.1% (K8s API errors only)
- ✅ P95 latency increase <20ms
- ✅ All tests passing (unit, integration, E2E)

---

## 1. Unit Tests

### 1.1 Test File

**Location**: `pkg/gateway/processing/distributed_lock_test.go`

**Coverage Target**: 90%+ for `distributed_lock.go`

### 1.2 Test Scenarios

#### Scenario 1: Lock Acquisition Success

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **New Lock** | Acquire lock for fingerprint with no existing lease | Lease created, `acquired=true` |
| **Reentrant Lock** | Acquire lock we already hold | Idempotent, `acquired=true` |
| **Expired Lease Takeover** | Acquire expired lease held by another pod | Lease updated with our holder ID, `acquired=true` |

**Test Pattern**:
```go
It("should acquire lock when lease doesn't exist", func() {
    // When: Acquire lock for new fingerprint
    acquired, err := lockManager.AcquireLock(ctx, "test-fingerprint-1")

    // Then: Lock acquired successfully
    Expect(err).ToNot(HaveOccurred())
    Expect(acquired).To(BeTrue())

    // And: Lease created in K8s (same namespace as Gateway pod)
    lease := &coordinationv1.Lease{}
    err = k8sClient.Get(ctx, client.ObjectKey{
        Namespace: namespace,  // Same namespace as Gateway pod
        Name:      "gw-lock-test-fingerpri",
    }, lease)
    Expect(err).ToNot(HaveOccurred())
    Expect(*lease.Spec.HolderIdentity).To(Equal("gateway-pod-1"))
})
```

#### Scenario 2: Lock Acquisition Failure

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Lock Held by Another Pod** | Try to acquire lock held by another pod | `acquired=false`, no error |
| **K8s API Down** | K8s API returns communication error | `acquired=false`, error returned |
| **Permission Denied** | RBAC insufficient for Lease operations | `acquired=false`, error returned |

**Test Pattern**:
```go
It("should NOT acquire lock when held by another pod", func() {
    // Given: Lease exists and held by another pod
    createLease(ctx, k8sClient, "test-fingerprint-2", "gateway-pod-2")

    // When: Try to acquire lock
    acquired, err := lockManager.AcquireLock(ctx, "test-fingerprint-2")

    // Then: Lock NOT acquired (no error - expected behavior)
    Expect(err).ToNot(HaveOccurred())
    Expect(acquired).To(BeFalse())
})
```

#### Scenario 3: Lock Release

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Release Held Lock** | Release lock we hold | Lease deleted |
| **Release Non-Existent Lock** | Release lock that doesn't exist | No error (idempotent) |

**Test Pattern**:
```go
It("should release lock successfully", func() {
    // Given: Lease exists and held by us
    createLease(ctx, k8sClient, "test-fingerprint-3", "gateway-pod-1")

    // When: Release lock
    err := lockManager.ReleaseLock(ctx, "test-fingerprint-3")

    // Then: Lease deleted
    Expect(err).ToNot(HaveOccurred())

    lease := &coordinationv1.Lease{}
    err = k8sClient.Get(ctx, client.ObjectKey{
        Namespace: "kubernaut-system",
        Name:      "gw-lock-test-fingerpri",
    }, lease)
    Expect(apierrors.IsNotFound(err)).To(BeTrue())
})
```

#### Scenario 4: Error Handling

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Get Error - NotFound** | Lease doesn't exist (expected) | Proceed to create lease |
| **Get Error - API Down** | K8s API communication error | Return error, `acquired=false` |
| **Create Error - AlreadyExists** | Another pod created lease first | Return `acquired=false`, no error |
| **Create Error - API Down** | K8s API communication error | Return error, `acquired=false` |
| **Update Error - Conflict** | Another pod updated lease first | Return `acquired=false`, no error |
| **Update Error - API Down** | K8s API communication error | Return error, `acquired=false` |

**Test Pattern**:
```go
It("should handle race condition when creating lease", func() {
    // Given: Two lock managers trying to acquire same lock
    lockManager2 := processing.NewDistributedLockManager(k8sClient, "kubernaut-system", "gateway-pod-2")
    fingerprint := "test-fingerprint-race"

    // When: Both try to acquire simultaneously
    results := make(chan bool, 2)
    go func() {
        acquired, _ := lockManager.AcquireLock(ctx, fingerprint)
        results <- acquired
    }()
    go func() {
        acquired, _ := lockManager2.AcquireLock(ctx, fingerprint)
        results <- acquired
    }()

    // Then: Only one acquires the lock
    acquired1 := <-results
    acquired2 := <-results
    Expect(acquired1 != acquired2).To(BeTrue())
})
```

### 1.3 Unit Test Checklist

- [ ] Lock acquisition when lease doesn't exist
- [ ] Lock acquisition when we already hold it (reentrant)
- [ ] Lock acquisition blocked by another pod
- [ ] Lock acquisition of expired lease
- [ ] Lock release success
- [ ] Lock release idempotency (non-existent lock)
- [ ] Error handling: K8s API NotFound (expected)
- [ ] Error handling: K8s API communication errors
- [ ] Error handling: Create race condition (AlreadyExists)
- [ ] Error handling: Update race condition (Conflict)
- [ ] Concurrent lock acquisition (race condition simulation)

---

## 2. Integration Tests

### 2.1 Test File

**Location**: `test/integration/gateway/distributed_locking_test.go`

**Infrastructure**: Real K8s client (not fake), simulated multiple Gateway servers

### 2.2 Test Scenarios

#### Scenario 1: Multi-Replica Deduplication Protection

**Objective**: Verify only 1 RR created when multiple Gateway pods process same fingerprint

**Setup**:
- 3 simulated Gateway servers (separate HTTP servers)
- Each with unique POD_NAME
- Each with distributed locking enabled

**Test**:
```go
It("should prevent duplicate RRs with simulated multiple Gateway replicas", func() {
    // Given: 3 Gateway pods running
    gatewayPods := []string{"gateway-pod-1", "gateway-pod-2", "gateway-pod-3"}
    gatewayServers := setupGatewayServers(ctx, gatewayPods)

    // When: Send 15 concurrent requests with SAME fingerprint to DIFFERENT pods
    fingerprint := "multi-replica-test"
    sendConcurrentSignals(15, gatewayServers, fingerprint)

    // Then: Only 1 RemediationRequest created
    Eventually(func() int {
        return countRRsByFingerprint(ctx, testNamespace, fingerprint)
    }, 30*time.Second, 1*time.Second).Should(Equal(1))

    // And: OccurrenceCount reflects all duplicates
    Eventually(func() int32 {
        rr := getRRByFingerprint(ctx, testNamespace, fingerprint)
        return rr.Status.Deduplication.OccurrenceCount
    }, 30*time.Second, 1*time.Second).Should(BeNumerically(">=", 15))
})
```

**Expected Result**:
- ✅ 1 RemediationRequest created
- ✅ OccurrenceCount = 15

#### Scenario 2: Lock Contention Handling

**Objective**: Verify retry with backoff when lock held by another pod

**Test**:
```go
It("should handle lock contention gracefully", func() {
    // Given: 2 Gateway pods
    pod1, pod2 := setupTwoGatewayPods(ctx)

    // When: Send signals to both pods simultaneously
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        sendSignal(pod1, "contention-test")
    }()

    go func() {
        defer wg.Done()
        sendSignal(pod2, "contention-test")
    }()

    wg.Wait()

    // Then: Only 1 RR created
    Eventually(func() int {
        return countRRs(ctx, testNamespace, "contention-test")
    }).Should(Equal(1))
})
```

**Expected Result**:
- ✅ 1 RemediationRequest created
- ✅ Second pod retried after backoff

#### Scenario 3: Lease Expiration Simulation

**Objective**: Verify new pod can take over expired lease

**Test**:
```go
It("should handle lease expiration on pod crash simulation", func() {
    // Given: Gateway pod acquires lock
    pod1 := setupGatewayPod(ctx, "gateway-pod-1")
    sendSignal(pod1, "expiry-test")

    // Get lease before "crash"
    lease := getLease(ctx, "expiry-test")
    Expect(*lease.Spec.HolderIdentity).To(Equal("gateway-pod-1"))

    // When: Simulate pod crash (don't release lock)
    // Wait for lease expiration (30s + buffer)
    time.Sleep(35 * time.Second)

    // And: New pod tries to acquire lock
    pod2 := setupGatewayPod(ctx, "gateway-pod-2")
    sendSignal(pod2, "expiry-test")

    // Then: New pod took over expired lease
    Eventually(func() string {
        lease := getLease(ctx, "expiry-test")
        if lease.Spec.HolderIdentity != nil {
            return *lease.Spec.HolderIdentity
        }
        return ""
    }).Should(Equal("gateway-pod-2"))
})
```

**Expected Result**:
- ✅ Expired lease taken over by new pod
- ✅ Signal processing continues

### 2.3 Integration Test Checklist

- [ ] Multi-replica deduplication (3 pods, 15 concurrent signals)
- [ ] Lock contention handling (2 pods, simultaneous signals)
- [ ] Lease expiration and takeover
- [ ] OccurrenceCount accuracy across replicas
- [ ] K8s Lease resource lifecycle (create, update, delete)

---

## 3. E2E Tests

### 3.1 Test File

**Location**: `test/e2e/gateway/distributed_locking_test.go`

**Infrastructure**: Real K8s cluster (Kind), real Gateway deployment with 3 replicas

### 3.2 Test Scenarios

#### Scenario 1: Production Multi-Replica Deployment

**Objective**: Validate distributed locking in production-like environment

**Prerequisites**:
- Gateway deployed with 3 replicas
- Distributed locking always enabled (hardcoded)
- RBAC permissions for Lease resources

**Test**:
```go
It("should handle 100 concurrent signals across 3 Gateway replicas", func() {
    // Given: Gateway deployed with 3 replicas
    verifyGatewayDeployment(3)

    // When: Send 100 concurrent signals with SAME fingerprint
    fingerprint := "e2e-production-test"
    sendConcurrentPrometheusAlerts(100, fingerprint)

    // Then: Only 1 RemediationRequest created
    Eventually(func() int {
        return countRRs(ctx, testNamespace, "e2e-production-test")
    }, 60*time.Second, 2*time.Second).Should(Equal(1))

    // And: OccurrenceCount = 100
    Eventually(func() int32 {
        rr := getRR(ctx, testNamespace, "e2e-production-test")
        return rr.Status.Deduplication.OccurrenceCount
    }).Should(Equal(int32(100)))
})
```

**Expected Result**:
- ✅ 1 RemediationRequest created
- ✅ OccurrenceCount = 100
- ✅ All 100 requests succeeded (HTTP 201/202)

#### Scenario 2: Load Distribution Across Replicas

**Objective**: Verify all Gateway pods are processing requests

**Test**:
```go
It("should distribute load across all Gateway replicas", func() {
    // Given: 3 Gateway pods running
    pods := getGatewayPods(ctx)
    Expect(len(pods)).To(Equal(3))

    // When: Send 30 signals (10 per pod expected)
    for i := 0; i < 30; i++ {
        sendPrometheusAlert(fmt.Sprintf("load-test-%d", i))
    }

    // Then: All pods show activity in metrics
    for _, pod := range pods {
        metrics := fetchPodMetrics(pod)
        Expect(metrics).To(ContainSubstring("gateway_signals_received_total"))
    }
})
```

**Expected Result**:
- ✅ All 3 pods processed requests
- ✅ Load distributed by K8s Service

#### Scenario 3: RBAC Validation

**Objective**: Verify Gateway has correct Lease permissions

**Test**:
```go
It("should have RBAC permissions for Lease resources", func() {
    // Given: Gateway ServiceAccount
    sa := getServiceAccount("gateway", "kubernaut-system")

    // When: Check ClusterRole bindings
    role := getClusterRole("gateway-role")

    // Then: Lease permissions present
    leasePerms := findResourcePermissions(role, "coordination.k8s.io", "leases")
    Expect(leasePerms.Verbs).To(ContainElements("get", "create", "update", "delete"))
})
```

**Expected Result**:
- ✅ Gateway has Lease resource permissions
- ✅ No permission denied errors in logs

### 3.3 E2E Test Checklist

- [ ] Gateway deployment with 3 replicas verified
- [ ] 100 concurrent signals create only 1 RR
- [ ] OccurrenceCount accuracy in production
- [ ] Load distribution across all replicas
- [ ] RBAC permissions validated
- [ ] No errors in Gateway pod logs
- [ ] Metrics endpoint accessible on all pods

---

## 4. Performance Testing

### 4.1 Latency Baseline Comparison

**Objective**: Measure latency impact of distributed locking

**Tool**: `vegeta` load testing tool

**Test Configuration**:
- Duration: 60 seconds
- Rate: 50 requests/second
- Scenario 1: Unique fingerprints (no lock contention)
- Scenario 2: Same fingerprint (high lock contention)

**Metrics to Capture**:
| Metric | Target | Notes |
|--------|--------|-------|
| P50 latency increase | <10ms | Median impact |
| P95 latency increase | <20ms | Tail latency impact |
| P99 latency increase | <30ms | Worst-case impact |

**Test Script**:
```bash
#!/bin/bash
# performance_test.sh

GATEWAY_URL="http://localhost:8080"
DURATION="60s"
RATE="50/s"

# Test 1: Unique fingerprints (no contention)
echo "=== Unique Fingerprints (No Contention) ==="
vegeta attack -duration=${DURATION} -rate=${RATE} \
  -targets=unique_fingerprints.txt | \
  vegeta report -type=text > unique_results.txt

# Test 2: Same fingerprint (high contention)
echo "=== Same Fingerprint (High Contention) ==="
vegeta attack -duration=${DURATION} -rate=${RATE} \
  -targets=same_fingerprint.txt | \
  vegeta report -type=text > contention_results.txt

# Compare results
diff unique_results.txt contention_results.txt
```

**Success Criteria**:
- ✅ P95 latency increase <20ms under no contention
- ✅ P95 latency increase <50ms under high contention
- ✅ No HTTP 500 errors (lock acquisition failures)

### 4.2 Lock Acquisition Failure Rate

**Objective**: Measure K8s API error rate

**Metrics**:
- `gateway_lock_acquisition_failures_total` (Prometheus)

**Test**:
```go
It("should have lock acquisition failure rate <0.1%", func() {
    // Given: Gateway processing 1000 signals
    for i := 0; i < 1000; i++ {
        sendSignal(fmt.Sprintf("perf-test-%d", i))
    }

    // When: Query failure metric
    failures := queryPrometheus("gateway_lock_acquisition_failures_total")
    totalRequests := 1000

    // Then: Failure rate <0.1%
    failureRate := float64(failures) / float64(totalRequests)
    Expect(failureRate).To(BeNumerically("<", 0.001))
})
```

**Success Criteria**:
- ✅ Lock acquisition failure rate <0.1%

---

## 5. Metrics Validation

### 5.1 Failure Metric Exposure (Integration Test)

**Metric**: `gateway_lock_acquisition_failures_total`

**Test Location**: `test/integration/gateway/distributed_locking_test.go`

**Test**:
```go
It("should expose lock acquisition failure metric on /metrics endpoint", func() {
    // Given: Gateway with metrics endpoint
    gatewayURL := setupGatewayWithMetrics(ctx, testClient, dataStorageURL)
    metricsURL := fmt.Sprintf("%s/metrics", gatewayURL)

    // When: Query metrics endpoint
    resp, err := http.Get(metricsURL)
    Expect(err).ToNot(HaveOccurred())
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    metricsOutput := string(body)

    // Then: Failure metric is exposed
    Expect(metricsOutput).To(ContainSubstring("gateway_lock_acquisition_failures_total"),
        "Distributed locking failure metric should be exposed")

    // And: Metric has help text
    Expect(metricsOutput).To(ContainSubstring("Total number of failed distributed lock acquisitions"),
        "Metric should have descriptive help text")
})
```

**Expected Result**:
- ✅ Metric exposed on `/metrics` endpoint
- ✅ Metric has proper help text
- ✅ Metric format is valid Prometheus format

---

### 5.2 Failure Metric Increment (Integration Test)

**Objective**: Verify metric increments when K8s API errors occur

**Test**:
```go
It("should increment failure metric on K8s API errors", func() {
    // Given: Gateway configured with mock K8s client that returns errors
    // (This would require dependency injection or test-specific configuration)
    // For integration tests, we simulate K8s API unavailability

    // When: Query initial metric value
    initialFailures := queryPrometheusMetric(metricsURL, "gateway_lock_acquisition_failures_total")

    // And: Simulate K8s API error scenario
    // (In integration tests, this might be done by temporarily disrupting K8s API access
    //  or using a test-specific error injection mechanism)

    // Then: Metric should increment
    Eventually(func() float64 {
        return queryPrometheusMetric(metricsURL, "gateway_lock_acquisition_failures_total")
    }, 30*time.Second, 1*time.Second).Should(BeNumerically(">", initialFailures),
        "Failure metric should increment when K8s API errors occur")
})
```

**Note**: Full validation of metric increment may require:
- Mock K8s client with configurable error responses
- Or chaos engineering (temporarily disrupt K8s API in test environment)
- Minimal validation: Verify metric is zero initially (no failures)

---

### 5.3 Failure Metric in E2E Tests

**Objective**: Verify metric accessible across all Gateway pods

**Test Location**: `test/e2e/gateway/distributed_locking_test.go`

**Test**:
```go
It("should expose failure metric on all Gateway pod replicas", func() {
    // Given: Gateway deployed with 3 replicas
    pods := getGatewayPods(ctx, "kubernaut-system")
    Expect(len(pods)).To(Equal(3))

    // When: Query metrics endpoint on each pod
    for _, pod := range pods {
        podMetricsURL := getPodMetricsURL(pod)

        resp, err := http.Get(podMetricsURL)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()

        body, _ := io.ReadAll(resp.Body)
        metricsOutput := string(body)

        // Then: Metric present on this pod
        Expect(metricsOutput).To(ContainSubstring("gateway_lock_acquisition_failures_total"),
            "Metric should be exposed on pod %s", pod.Name)
    }
})
```

**Expected Result**:
- ✅ Metric accessible on all 3 Gateway pods
- ✅ Each pod has independent metric value
- ✅ No pod returns 404 or metric error

---

### 5.4 Metrics Checklist

**Integration Tests**:
- [ ] `gateway_lock_acquisition_failures_total` exposed on `/metrics`
- [ ] Metric has correct help text
- [ ] Metric starts at 0 (no failures initially)
- [ ] Metric increments on K8s API errors (if testable)

**E2E Tests**:
- [ ] Metric accessible on all Gateway pods
- [ ] Each pod has independent counter
- [ ] Metric format is valid Prometheus format
- [ ] No errors in Prometheus scrape logs

**Performance Tests**:
- [ ] Metric value <0.1% of total requests under normal operation
- [ ] Metric correlates with K8s API errors in logs

---

## 6. Test Execution Summary

### 6.1 Test Execution Order

1. **Unit Tests** (2 hours)
   - Run: `ginkgo -v --race ./pkg/gateway/processing`
   - Expected: 90%+ coverage, all passing

2. **Integration Tests** (3 hours)
   - Run: `ginkgo -v --race --procs=4 ./test/integration/gateway`
   - Expected: Multi-replica scenarios validated

3. **E2E Tests** (2 hours)
   - Run: `ginkgo -v --race --procs=4 ./test/e2e/gateway`
   - Expected: Production deployment validated

4. **Performance Tests** (1 hour)
   - Run: `./performance_test.sh`
   - Expected: P95 latency <20ms increase

### 6.2 Test Environment Requirements

**Unit Tests**:
- Fake K8s client
- No external dependencies

**Integration Tests**:
- Real K8s client (envtest or Kind)
- DataStorage service running (for audit tests)
- Isolated test namespaces

**E2E Tests**:
- Kind cluster with Gateway deployed
- 3 Gateway replicas
- Real Kubernetes API
- DataStorage service deployed

### 6.3 Acceptance Criteria

- [ ] All unit tests passing (90%+ coverage)
- [ ] All integration tests passing
- [ ] All E2E tests passing
- [ ] Performance targets met (P95 <20ms increase)
- [ ] Zero duplicate RRs in production deployment
- [ ] Lock acquisition failure rate <0.1%
- [ ] No errors in Gateway logs

---

## 7. Risk Mitigation Tests

### 7.1 K8s API Unavailability

**Test**: Simulate K8s API down during lock acquisition

**Expected**: Gateway returns HTTP 500 (fail-fast)

### 7.2 RBAC Insufficient

**Test**: Deploy Gateway without Lease permissions

**Expected**: Gateway logs permission denied errors, signals fail

### 7.3 Lease Resource Garbage Collection

**Test**: Verify old leases are cleaned up

**Expected**: Leases deleted after release, no resource leak

---

## 8. Documentation Validation

### 8.1 Checklist

- [ ] DD-GATEWAY-013 updated with test results
- [ ] Implementation plan marked complete
- [ ] Runbook created for troubleshooting distributed locking
- [ ] Metrics documentation updated with failure metric

---

## Status Tracking

| Test Tier | Status | Coverage | Notes |
|-----------|--------|----------|-------|
| Unit Tests | ⬜ Pending | Target: 90%+ | distributed_lock_test.go |
| Integration Tests | ⬜ Pending | Multi-replica scenarios | distributed_locking_test.go |
| E2E Tests | ⬜ Pending | Production deployment | distributed_locking_test.go |
| Performance Tests | ⬜ Pending | Latency <20ms | vegeta load test |
| Metrics Validation | ⬜ Pending | Failure metric | Prometheus query |

**Last Updated**: December 30, 2025
**Next Review**: After implementation completion

