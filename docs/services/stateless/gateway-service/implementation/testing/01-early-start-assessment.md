# Gateway Testing: Early Start Assessment

**Date**: 2025-10-09
**Goal**: Determine if we can start E2E/integration testing immediately with current implementation

---

## Assessment Summary

✅ **YES - We can start integration testing NOW**

We have all core components implemented and can validate the complete pipeline with real dependencies (Redis + Kubernetes API).

---

## What We Have (Ready for Testing)

### Complete Pipeline ✅
1. **HTTP Server** - Can receive webhook requests
2. **Prometheus Adapter** - Can parse AlertManager payloads
3. **Deduplication Service** - Needs real Redis
4. **Storm Detection** - Needs real Redis
5. **Environment Classification** - Needs real K8s API
6. **Priority Assignment** - Pure logic (no external deps)
7. **CRD Creator** - Needs real K8s API
8. **Middleware** (auth, rate limiting) - Can test independently

### Dependencies Available ✅
1. **Redis**: Can use testcontainers or local Redis in CI
2. **Kubernetes API**: Can use envtest (controller-runtime's test environment)
3. **CRDs**: RemediationRequest already defined in `api/remediation/v1alpha1`

### Test Data ✅
We can create sample Prometheus AlertManager webhook payloads

---

## Testing Strategy: Integration-First Approach

### Why Integration Tests First?
1. **Higher Value**: Validates end-to-end flow immediately
2. **Faster Feedback**: Catches integration issues early (API mismatches, timing issues)
3. **Real Dependencies**: Tests with actual Redis/K8s behavior (not mocks)
4. **Confidence**: Proves the pipeline works before detailed unit testing
5. **TDD Spirit**: Write tests that prove the system works, then refine with unit tests

### Traditional vs Integration-First

**Traditional (Plan)**:
```
Day 7-8: Unit tests (40+ tests)
Day 9-10: Integration tests (12+ tests)
```

**Integration-First (Recommended)**:
```
Day 7 (Morning): Core integration tests (5 tests) ← START HERE
Day 7 (Afternoon): Unit tests for adapters (10 tests)
Day 8 (Morning): Unit tests for processing (15 tests)
Day 8 (Afternoon): Unit tests for middleware (10 tests)
Day 9 (Morning): Advanced integration tests (7 tests)
Day 9 (Afternoon): E2E workflow tests (5 tests)
Day 10: Performance tests + cleanup
```

**Benefits**:
- Immediate feedback on architectural issues
- Validates assumptions about Redis/K8s integration
- Finds timing/concurrency issues early
- Can iterate on unit tests with confidence

---

## Integration Test Priorities (Day 7 Morning)

### 🎯 Critical Path Tests (Must Have)

#### Test 1: Basic Signal Ingestion → CRD Creation
**Goal**: Prove the complete pipeline works end-to-end

```
Setup: Redis + envtest + RemediationRequest CRD registered
Action: POST Prometheus webhook to /api/v1/signals/prometheus
Verify:
  - HTTP 201 Created
  - RemediationRequest CRD exists in K8s
  - CRD has correct spec fields (fingerprint, severity, priority, environment)
  - Redis has deduplication metadata
```

**Estimated Time**: 2 hours (includes test setup)

#### Test 2: Deduplication (Duplicate Signal)
**Goal**: Verify Redis deduplication works

```
Setup: Same as Test 1
Action:
  1. POST signal (first time) → expect 201
  2. POST same signal (duplicate) → expect 202
Verify:
  - First request creates CRD
  - Second request returns 202 Accepted
  - Redis metadata updated (count incremented, lastSeen updated)
  - Only one RemediationRequest CRD exists
```

**Estimated Time**: 1 hour

#### Test 3: Environment Classification
**Goal**: Verify namespace label → environment mapping

```
Setup: envtest with namespace "prod-api" labeled "environment=prod"
Action: POST signal for namespace "prod-api"
Verify:
  - Environment classified as "prod"
  - Priority assigned correctly (critical + prod → P0)
  - CRD has environment="prod" in spec
```

**Estimated Time**: 1 hour

#### Test 4: Storm Detection (Rate-Based)
**Goal**: Verify Redis storm detection works

```
Setup: Redis + envtest
Action: POST 15 alerts with same alertname in 1 minute
Verify:
  - Storm detected after 10th alert
  - Storm metadata logged (type="rate", count=15)
  - All CRDs created (storm doesn't block creation)
```

**Estimated Time**: 1.5 hours

#### Test 5: Authentication Failure
**Goal**: Verify TokenReview middleware rejects invalid tokens

```
Setup: envtest with TokenReview API
Action: POST with invalid/missing Bearer token
Verify:
  - HTTP 401 Unauthorized
  - Metrics: gateway_authentication_failures_total incremented
  - No CRD created
```

**Estimated Time**: 0.5 hours

**Total Day 7 Morning**: ~6 hours, 5 critical tests ✅

---

## Dependencies & Setup

### Test Environment Setup

#### 1. Redis (Testcontainers Recommended)
```go
// test/integration/gateway/redis_setup.go
func SetupRedis(t *testing.T) *redis.Client {
    ctx := context.Background()
    req := testcontainers.ContainerRequest{
        Image:        "redis:7-alpine",
        ExposedPorts: []string{"6379/tcp"},
        WaitStrategy: wait.ForLog("Ready to accept connections"),
    }
    redisC, err := testcontainers.GenericContainer(ctx, req)
    require.NoError(t, err)

    endpoint, _ := redisC.Endpoint(ctx, "")
    client := redis.NewClient(&redis.Options{Addr: endpoint})

    t.Cleanup(func() { redisC.Terminate(ctx) })
    return client
}
```

#### 2. Envtest (Controller-Runtime)
```go
// test/integration/gateway/k8s_setup.go
func SetupEnvtest(t *testing.T) client.Client {
    testEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd"),
        },
    }

    cfg, err := testEnv.Start()
    require.NoError(t, err)

    k8sClient, err := client.New(cfg, client.Options{
        Scheme: scheme.Scheme,
    })
    require.NoError(t, err)

    t.Cleanup(func() { testEnv.Stop() })
    return k8sClient
}
```

#### 3. Sample Test Data
```go
// test/integration/gateway/testdata.go
var PrometheusAlertSample = `{
  "version": "4",
  "groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
  "status": "firing",
  "receiver": "kubernaut",
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "HighMemoryUsage",
      "namespace": "prod-api",
      "pod": "payment-service-789",
      "severity": "critical"
    },
    "annotations": {
      "summary": "Pod memory usage > 90%",
      "description": "Payment service pod is using 95% memory"
    },
    "startsAt": "2025-10-09T10:00:00Z",
    "endsAt": "0001-01-01T00:00:00Z"
  }]
}`
```

---

## Risks & Mitigations

### Risk 1: RemediationRequest CRD Schema Mismatch
**Impact**: CRD creation fails in envtest
**Mitigation**:
- Verify CRD schema in `api/remediation/v1alpha1` matches `crd_creator.go`
- Run `make manifests` to regenerate CRD YAML from code
- Check for missing kubebuilder markers

**Action**: Check schema before starting tests

### Risk 2: Redis Connection in CI/CD
**Impact**: Tests fail in CI without Redis
**Mitigation**:
- Use testcontainers (auto-starts Redis container)
- Fallback: Use Redis service in CI config
- Document setup in test README

**Action**: Test with testcontainers first

### Risk 3: Envtest Binary Missing
**Impact**: Tests fail with "etcd binary not found"
**Mitigation**:
- Already resolved in RemediationRequest controller testing
- Use `setup-envtest` to download binaries
- Document in test setup

**Action**: Reuse existing envtest setup from RR controller tests

### Risk 4: Test Timing/Flakiness
**Impact**: Storm detection or deduplication tests intermittently fail
**Mitigation**:
- Use `Eventually()` blocks with retries
- Increase timeouts for CI environments
- Use fixed time.Now() for deterministic tests

**Action**: Use Ginkgo/Gomega `Eventually()` pattern

---

## Success Criteria (Day 7)

By end of Day 7, we should have:

✅ **5 passing integration tests** (critical path covered)
✅ **Redis integration working** (deduplication + storm detection)
✅ **K8s integration working** (CRD creation + environment classification)
✅ **Test infrastructure reusable** (for Days 8-10)
✅ **Confidence in architecture** (no major refactoring needed)

**If successful**: Continue with unit tests (Days 8-9) and advanced integration tests (Day 10)
**If issues found**: Fix architectural problems early (cheaper than after 40 unit tests)

---

## Recommendation

**START WITH INTEGRATION TESTS (Day 7 Morning)**

Rationale:
1. Validates architecture immediately
2. Catches integration issues early
3. Provides confidence for unit testing
4. Follows TDD spirit (prove it works first)
5. Higher ROI (5 tests cover more than 20 isolated unit tests)

**Proposed Schedule**:
- **Now**: Check RemediationRequest CRD schema alignment
- **Day 7 Morning**: Setup test infrastructure + 5 critical integration tests
- **Day 7 Afternoon**: Unit tests for adapters
- **Days 8-9**: Complete unit + integration test coverage
- **Day 10**: Performance + edge cases

---

## Next Immediate Actions

1. ✅ **Verify CRD Schema** (5 minutes)
   - Check `api/remediation/v1alpha1/remediationrequest_types.go`
   - Compare with `pkg/gateway/processing/crd_creator.go` expectations
   - Run `make manifests` if needed

2. ✅ **Create Test Setup Files** (30 minutes)
   - `test/integration/gateway/suite_test.go` (Ginkgo suite)
   - `test/integration/gateway/redis_setup.go`
   - `test/integration/gateway/k8s_setup.go`
   - `test/integration/gateway/testdata.go`

3. ✅ **Write Test 1** (2 hours)
   - Basic signal → CRD creation
   - Validates end-to-end pipeline

4. **Iterate**: Tests 2-5 (4 hours)

**Total Day 7 Morning**: ~6.5 hours to critical path validation ✅

---

## Conclusion

**YES - Start integration testing NOW**. We have everything needed, and it's the fastest path to validating the Gateway implementation works as designed.

