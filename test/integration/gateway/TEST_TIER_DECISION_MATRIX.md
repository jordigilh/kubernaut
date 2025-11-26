# Test Tier Decision Matrix

**Purpose**: Guide developers in choosing the correct test tier (Unit, Integration, E2E) for Gateway tests
**Authority**: Based on [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
**Last Updated**: 2025-11-23

---

## 🎯 **Quick Decision Tree**

```
Does the test require external infrastructure (Redis, K8s, HTTP)?
├─ NO → UNIT TEST (70%+ coverage)
└─ YES → Does it test complete user workflow?
    ├─ NO → INTEGRATION TEST (>50% coverage)
    └─ YES → E2E TEST (10-15% coverage)
```

---

## 📊 **Test Tier Characteristics**

| Characteristic | Unit | Integration | E2E |
|---|---|---|---|
| **Speed** | <100ms | <5s | >5s (often 30s+) |
| **Infrastructure** | None (mocks only) | Real (Redis, K8s, DB) | Complete (Kind, Redis, Gateway) |
| **Scope** | Single function/component | Component + infrastructure | Complete workflow |
| **Parallelization** | ✅ Always | ✅ Usually | ⚠️ Limited |
| **Flakiness** | ✅ Stable | ⚠️ Moderate | ❌ Higher risk |
| **Coverage Target** | 70%+ | >50% | 10-15% |

---

## 🔍 **UNIT TEST** - Business Logic in Isolation

### **When to Use**
- Testing pure business logic without external dependencies
- Testing algorithms, calculations, data transformations
- Testing error handling and validation logic
- Testing struct methods that don't call external services

### **Characteristics**
- ✅ **Fast**: <100ms per test
- ✅ **No Infrastructure**: Uses mocks/fakes only
- ✅ **Deterministic**: Same input → same output
- ✅ **Parallel-Safe**: No shared state

### **Examples from Gateway**

#### ✅ **Good Unit Test**
```go
// Test: Storm detection threshold calculation
It("should detect storm when rate exceeds threshold", func() {
    detector := processing.NewStormDetector(10) // threshold = 10

    // Business logic: 15 alerts/minute > 10 threshold
    isStorm := detector.IsStorm(15, time.Minute)

    Expect(isStorm).To(BeTrue())
})
```

**Why Unit**: Pure calculation, no external dependencies

---

#### ✅ **Good Unit Test with Miniredis**
```go
// Test: Deduplication fingerprint generation
It("should generate consistent fingerprint for same alert", func() {
    mr := miniredis.NewMiniRedis() // In-memory Redis
    defer mr.Close()

    signal := &types.NormalizedSignal{
        AlertName: "HighCPU",
        Namespace: "prod",
    }

    fp1 := processing.GenerateFingerprint(signal)
    fp2 := processing.GenerateFingerprint(signal)

    Expect(fp1).To(Equal(fp2))
})
```

**Why Unit**: Tests fingerprint algorithm, miniredis is fast enough for unit tests

---

### **Decision Criteria**

| Criterion | Threshold | Example |
|---|---|---|
| **Duration** | <100ms | Fingerprint calculation |
| **Dependencies** | None or miniredis | Hash generation, validation |
| **Infrastructure** | No real services | Pure functions, algorithms |
| **Scope** | Single function/method | `IsStorm()`, `GenerateFingerprint()` |

---

## 🔗 **INTEGRATION TEST** - Infrastructure Interaction

### **When to Use**
- Testing component interaction with real infrastructure (Redis, K8s, PostgreSQL)
- Testing data persistence and retrieval
- Testing concurrent access patterns
- Testing infrastructure failure handling

### **Characteristics**
- ⚠️ **Moderate Speed**: <5s per test (target)
- ✅ **Real Infrastructure**: Redis, K8s (envtest), PostgreSQL
- ⚠️ **Some Flakiness**: Timing-sensitive operations
- ✅ **Parallel-Safe**: With proper isolation (unique namespaces/keys)

### **Examples from Gateway**

#### ✅ **Good Integration Test**
```go
// Test: Redis deduplication with real Redis
It("should prevent duplicate CRD creation", func() {
    // Uses REAL Redis from test suite
    dedupService := processing.NewDeduplicationService(
        redisClient, // Real Redis
        k8sClient,   // Real K8s (envtest)
        logger,
        metrics,
    )

    signal := &types.NormalizedSignal{
        Fingerprint: "test-fp-123",
        AlertName:   "HighCPU",
    }

    // First call: Should create CRD
    isDupe, _, err := dedupService.CheckDuplication(ctx, signal)
    Expect(err).ToNot(HaveOccurred())
    Expect(isDupe).To(BeFalse())

    // Second call: Should detect duplication
    isDupe, _, err = dedupService.CheckDuplication(ctx, signal)
    Expect(err).ToNot(HaveOccurred())
    Expect(isDupe).To(BeTrue())
})
```

**Why Integration**: Tests Redis + K8s interaction, validates real infrastructure behavior

---

#### ✅ **Good Integration Test - Concurrent Access**
```go
// Test: Multi-pod deduplication consistency
It("should handle concurrent deduplication from multiple pods", func() {
    // Simulate 2 Gateway pods with separate K8s clients
    dedupService1 := processing.NewDeduplicationService(redisClient, k8sClient1, ...)
    dedupService2 := processing.NewDeduplicationService(redisClient, k8sClient2, ...)

    signal := &types.NormalizedSignal{Fingerprint: "concurrent-test"}

    // Both pods check simultaneously
    var isDupe1, isDupe2 bool
    done := make(chan bool, 2)

    go func() {
        isDupe1, _, _ = dedupService1.CheckDuplication(ctx, signal)
        done <- true
    }()

    go func() {
        isDupe2, _, _ = dedupService2.CheckDuplication(ctx, signal)
        done <- true
    }()

    <-done
    <-done

    // Exactly ONE should create CRD, ONE should detect duplication
    Expect(isDupe1 != isDupe2).To(BeTrue())
})
```

**Why Integration**: Tests real Redis coordination between multiple clients

---

### **Decision Criteria**

| Criterion | Threshold | Example |
|---|---|---|
| **Duration** | <5s | Redis operations, K8s CRD creation |
| **Dependencies** | Real infrastructure | Redis, K8s (envtest), PostgreSQL |
| **Infrastructure** | Required | Cannot mock Redis/K8s behavior |
| **Scope** | Component + infrastructure | Deduplication service + Redis + K8s |
| **Timing** | Not timing-sensitive | No long waits (>5s) |

---

### **❌ When NOT to Use Integration**

| Anti-Pattern | Why Wrong | Correct Tier |
|---|---|---|
| **10s+ wait times** | Too slow, flaky | E2E |
| **HTTP webhook testing** | Tests external interface | E2E |
| **Complete workflows** | Too broad | E2E |
| **Pure calculations** | No infrastructure needed | Unit |

---

## 🌐 **E2E TEST** - Complete User Workflows

### **When to Use**
- Testing critical end-to-end user journeys
- Testing HTTP webhook endpoints (external interfaces)
- Testing complete alert ingestion → CRD creation → notification flow
- Testing timing-sensitive workflows (TTL expiration, window closure)

### **Characteristics**
- ❌ **Slow**: >5s per test (often 30s-2min)
- ✅ **Complete Infrastructure**: Kind cluster, Redis, Gateway pods, AlertManager
- ❌ **Higher Flakiness**: Network, timing, infrastructure issues
- ⚠️ **Limited Parallelization**: Shared infrastructure

### **Examples from Gateway**

#### ✅ **Good E2E Test**
```go
// Test: Storm window TTL expiration (90s wait)
It("should create new storm window after TTL expiration", func() {
    // Send 5 alerts to trigger storm
    for i := 1; i <= 5; i++ {
        payload := createAlertPayload(fmt.Sprintf("pod-%d", i))
        resp, err := httpClient.Post(gatewayURL+"/api/v1/signals/prometheus",
            "application/json", bytes.NewBuffer(payload))
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusAccepted)) // 202 during storm
    }

    // Wait for window to expire (90 seconds)
    time.Sleep(90 * time.Second)

    // Send new alert - should create NEW window
    payload := createAlertPayload("pod-6")
    resp, err := httpClient.Post(gatewayURL+"/api/v1/signals/prometheus",
        "application/json", bytes.NewBuffer(payload))
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(http.StatusCreated)) // 201 for new window
})
```

**Why E2E**:
- ✅ Tests complete HTTP workflow
- ✅ Validates HTTP status codes (external interface behavior)
- ✅ Requires 90s wait (too slow for integration)
- ✅ Tests end-to-end timing behavior

---

#### ✅ **Good E2E Test - Concurrent HTTP Requests**
```go
// Test: 15 concurrent Prometheus alerts → 1 aggregated CRD
It("should aggregate 15 concurrent alerts into single storm CRD", func() {
    // Send 15 concurrent HTTP requests
    var wg sync.WaitGroup
    responses := make([]int, 15)

    for i := 0; i < 15; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            payload := createAlertPayload(fmt.Sprintf("pod-%d", idx))
            resp, _ := httpClient.Post(gatewayURL+"/api/v1/signals/prometheus",
                "application/json", bytes.NewBuffer(payload))
            responses[idx] = resp.StatusCode
        }(i)
    }

    wg.Wait()

    // Validate HTTP status codes
    // First alert: 201 Created
    // Subsequent alerts: 202 Accepted (during storm)
    createdCount := 0
    acceptedCount := 0
    for _, status := range responses {
        if status == http.StatusCreated {
            createdCount++
        } else if status == http.StatusAccepted {
            acceptedCount++
        }
    }

    Expect(createdCount).To(Equal(1))
    Expect(acceptedCount).To(Equal(14))

    // Verify single CRD created
    crds := listRemediationRequests(ctx, k8sClient, "production")
    Expect(len(crds)).To(Equal(1))
})
```

**Why E2E**:
- ✅ Tests complete HTTP webhook flow
- ✅ Validates HTTP status code transitions (201 → 202)
- ✅ Tests concurrent request handling
- ✅ Validates end-to-end CRD creation

---

### **Decision Criteria**

| Criterion | Threshold | Example |
|---|---|---|
| **Duration** | >5s (often 30s+) | TTL expiration (90s), workflow completion |
| **Dependencies** | Complete stack | Kind, Redis, Gateway pods, AlertManager |
| **Infrastructure** | Production-like | Real HTTP endpoints, real services |
| **Scope** | Complete workflow | Alert ingestion → storm detection → CRD creation |
| **HTTP Testing** | External interface | Webhook endpoints, status codes |
| **Timing** | Timing-sensitive | TTL expiration, window closure |

---

### **❌ When NOT to Use E2E**

| Anti-Pattern | Why Wrong | Correct Tier |
|---|---|---|
| **Fast operations (<5s)** | Overkill, too slow | Integration |
| **Unit logic testing** | No workflow validation | Unit |
| **Infrastructure-only** | No user journey | Integration |
| **Every test** | Too slow, expensive | Unit/Integration |

---

## 📋 **Decision Matrix - Quick Reference**

### **Timing Thresholds**

| Duration | Tier | Rationale |
|---|---|---|
| **<100ms** | Unit | Fast, deterministic |
| **100ms-5s** | Integration | Infrastructure interaction |
| **>5s** | E2E | Complete workflow, timing-sensitive |
| **>30s** | E2E (mandatory) | Too slow for integration |

---

### **Infrastructure Requirements**

| Infrastructure | Tier | Example |
|---|---|---|
| **None** | Unit | Pure functions, algorithms |
| **Miniredis** | Unit | Fast in-memory Redis |
| **Real Redis** | Integration | Persistence, TTL, concurrency |
| **Real K8s (envtest)** | Integration | CRD operations, watches |
| **Real PostgreSQL** | Integration | Data storage, queries |
| **HTTP Endpoints** | E2E | Webhook testing, status codes |
| **Complete Stack** | E2E | Kind + Redis + Gateway + AlertManager |

---

### **Scope Guidelines**

| Scope | Tier | Example |
|---|---|---|
| **Single function** | Unit | `GenerateFingerprint()` |
| **Component + infrastructure** | Integration | Deduplication service + Redis + K8s |
| **Complete workflow** | E2E | Alert → storm detection → CRD → notification |
| **External interface** | E2E | HTTP webhook endpoints |

---

## 🎯 **Common Scenarios**

### **Scenario 1: Testing Storm Detection Logic**

**Question**: Should I test storm detection as unit, integration, or E2E?

**Answer**: **All three tiers** (defense-in-depth)

| Test | Tier | What to Test |
|---|---|---|
| **Threshold calculation** | Unit | `IsStorm(15, time.Minute)` → true |
| **Redis rate tracking** | Integration | Real Redis counter increments |
| **HTTP webhook flow** | E2E | 15 alerts → HTTP 202 → 1 CRD |

---

### **Scenario 2: Testing Deduplication**

**Question**: Should I test deduplication as unit, integration, or E2E?

**Answer**: **Unit + Integration** (no E2E needed)

| Test | Tier | What to Test |
|---|---|---|
| **Fingerprint generation** | Unit | Consistent hash for same alert |
| **Redis deduplication** | Integration | Real Redis + K8s coordination |
| **Multi-pod consistency** | Integration | Concurrent access from 2 clients |

**Why no E2E**: Deduplication is infrastructure interaction, not a complete user workflow

---

### **Scenario 3: Testing TTL Expiration**

**Question**: Should I test TTL expiration as unit, integration, or E2E?

**Answer**: **Integration for short TTL (<5s), E2E for long TTL (>5s)**

| Test | Tier | What to Test |
|---|---|---|
| **TTL calculation** | Unit | `CalculateTTL(5*time.Second)` |
| **Short TTL (5s)** | Integration | Real Redis TTL expiration |
| **Long TTL (90s)** | E2E | Window closure after 90s |

**Rationale**: 5s wait is acceptable for integration, 90s is too slow

---

### **Scenario 4: Testing HTTP Webhooks**

**Question**: Should I test HTTP webhook endpoints as unit, integration, or E2E?

**Answer**: **E2E** (external interface)

| Test | Tier | What to Test |
|---|---|---|
| **Request parsing** | Unit | `ParsePrometheusPayload(json)` |
| **Handler logic** | Unit | `HandleAlert(signal)` |
| **Complete HTTP flow** | E2E | POST → 201/202 → CRD created |

**Rationale**: HTTP endpoints are external interfaces, require E2E validation

---

## 🚫 **Anti-Patterns to Avoid**

### **❌ Anti-Pattern 1: Unit Tests with Real Infrastructure**

```go
// ❌ BAD: Unit test using real Redis
It("should store alert in Redis", func() {
    redisClient := redis.NewClient(...) // Real Redis!
    service := processing.NewService(redisClient)

    err := service.StoreAlert(ctx, alert)
    Expect(err).ToNot(HaveOccurred())
})
```

**Why Wrong**: Unit tests should not require real infrastructure
**Fix**: Use miniredis or move to integration tier

---

### **❌ Anti-Pattern 2: Integration Tests with Long Waits**

```go
// ❌ BAD: Integration test with 90s wait
It("should expire window after 90 seconds", func() {
    aggregator.StartWindow(ctx, signal)

    time.Sleep(90 * time.Second) // Too slow!

    isExpired := aggregator.IsWindowExpired(ctx, windowID)
    Expect(isExpired).To(BeTrue())
})
```

**Why Wrong**: Integration tests should be <5s
**Fix**: Move to E2E tier or use shorter TTL for testing

---

### **❌ Anti-Pattern 3: E2E Tests for Simple Logic**

```go
// ❌ BAD: E2E test for fingerprint calculation
It("should generate fingerprint", func() {
    // Start complete E2E environment
    gatewayURL := startGatewayE2E()

    // Send HTTP request just to test fingerprint
    resp, _ := httpClient.Post(gatewayURL+"/api/v1/signals/prometheus", ...)

    // Check fingerprint in response
    Expect(resp.Fingerprint).To(MatchRegexp("[a-f0-9]{64}"))
})
```

**Why Wrong**: E2E is overkill for simple logic
**Fix**: Move to unit test with direct function call

---

## ✅ **Best Practices**

### **1. Start with Unit Tests**
- Test business logic first
- Use mocks/fakes for external dependencies
- Aim for 70%+ coverage

### **2. Add Integration Tests for Infrastructure**
- Test real Redis, K8s, PostgreSQL behavior
- Validate concurrent access patterns
- Test infrastructure failure handling

### **3. Add E2E Tests for Critical Journeys**
- Test complete user workflows
- Validate HTTP endpoints and status codes
- Test timing-sensitive operations (>5s)

### **4. Use Timing as a Guide**
- <100ms → Unit
- 100ms-5s → Integration
- >5s → E2E

### **5. Follow Defense-in-Depth**
- Unit: Business logic
- Integration: Infrastructure interaction
- E2E: Complete workflow

---

## 📚 **References**

- **Testing Strategy**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
- **Coverage Standards**: [15-testing-coverage-standards.mdc](mdc:.cursor/rules/15-testing-coverage-standards.mdc)
- **E2E Migration Analysis**: `test/integration/gateway/TRIAGE_E2E_MIGRATION.md`
- **Tier Triage**: `test/integration/gateway/INTEGRATION_TEST_TIER_TRIAGE.md`

---

## 🎓 **Summary**

| Question | Answer |
|---|---|
| **When Unit?** | Pure logic, no infrastructure, <100ms |
| **When Integration?** | Real infrastructure, <5s, component interaction |
| **When E2E?** | Complete workflow, >5s, HTTP endpoints, timing-sensitive |
| **Timing Threshold?** | <100ms (Unit), 100ms-5s (Integration), >5s (E2E) |
| **Infrastructure?** | None (Unit), Real (Integration), Complete (E2E) |
| **Coverage Target?** | 70%+ (Unit), >50% (Integration), 10-15% (E2E) |

**Golden Rule**: Use the **fastest tier** that can reliably test the behavior.

