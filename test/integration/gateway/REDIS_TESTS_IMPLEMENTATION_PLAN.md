# Redis Integration Tests - Implementation Plan

**Date**: 2025-10-27
**Status**: 🔍 **ASSESSMENT COMPLETE**
**Current**: 61/61 tests passing (100%)

---

## 📊 **Disabled Redis Tests Analysis**

### **Test 1: TTL Expiration** (line 101)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Waits 6 minutes for TTL expiration (impractical for integration tests)

**Implementation Options**:
1. **Configurable TTL** (5 seconds for tests, 5 minutes for production)
2. **Mock TTL expiration** (simulate time passage)
3. **Move to E2E tier** (accept long test duration)

**Confidence to Implement**: **85%** ✅
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (configurable TTL)

---

### **Test 2: Redis Connection Failure** (line 137)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Closes test client, not server; needs chaos testing infrastructure

**Implementation Options**:
1. **Stop Redis container** (podman stop redis-gateway-test)
2. **Network partition simulation** (iptables rules)
3. **Move to E2E tier** (dedicated chaos testing)

**Confidence to Implement**: **60%** ⚠️
**Effort**: 2-3 hours
**Recommendation**: **Option 1** (stop Redis container, expect 503)

---

### **Test 3: CRD Deletion Cleanup** (line 238)
**Status**: ✅ **DELETED** (DD-GATEWAY-005)
**Reason**: Current TTL-based cleanup is intentional design, not a missing feature

**Decision**: **Option A - Current behavior is correct** ✅
- Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
- This protects against false positives and alert storms after CRD deletion
- If admin deletes CRD, same alert shouldn't immediately recreate it
- **Design Decision**: [DD-GATEWAY-005](../../docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md)

**Action Taken**: Test deleted from `redis_integration_test.go` with explanatory comment

---

### **Test 4: Pipeline Command Failures** (line 335)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Requires Redis failure injection not available in integration tests

**Implementation Options**:
1. **Mock Redis client** (inject failures)
2. **Redis proxy** (intercept and fail commands)
3. **Move to E2E tier** (chaos testing)

**Confidence to Implement**: **40%** ❌
**Effort**: 3-4 hours
**Recommendation**: **Option 3** (move to E2E, too complex for integration)

---

### **Test 5: Connection Pool Exhaustion** (line 370)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: This is a LOAD TEST, not an integration test

**Implementation Options**:
1. **Move to load test tier** (test/load/gateway/)
2. **Reduce concurrency** (20 requests, not 200)
3. **Skip test** (not appropriate for integration tier)

**Confidence to Implement**: **70%** ⚠️
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (move to load test tier, out of scope)

---

## 🎯 **Recommendation Summary**

### **Implement Now** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - Configurable TTL approach

### **Implement with Caution** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Stop Redis container approach

### **Deleted** (DD-GATEWAY-005)
✅ **Test 3: CRD Deletion Cleanup** - Current TTL-based behavior is intentional design

### **Defer to E2E Tier** (40% confidence, 3-4 hours)
❌ **Test 4: Pipeline Command Failures** - Too complex for integration tier

### **Out of Scope** (70% confidence, 1-2 hours)
❌ **Test 5: Connection Pool Exhaustion** - Belongs in load test tier

---

## 📋 **Revised Action Plan**

### **Phase 1: Quick Win (1-2 hours, 85% confidence)**
**Goal**: Implement TTL expiration test with configurable TTL

**Steps**:
1. Add `TTL` field to `DeduplicationService` config (default: 5 minutes, test: 5 seconds)
2. Update `NewDeduplicationService` to accept TTL parameter
3. Update test helpers to use 5-second TTL for tests
4. Remove `XIt` prefix from TTL expiration test
5. Update test to wait 6 seconds (not 6 minutes)
6. Run tests to verify

**Expected Result**: 62/62 tests passing (100%)

---

### **Phase 2: Medium Risk (2-3 hours, 60% confidence)**
**Goal**: Implement Redis connection failure test

**Steps**:
1. Add helper function `StopRedis()` to test infrastructure
2. Update test to stop Redis container before sending webhook
3. Expect 503 response (storm detection service unavailable)
4. Add helper function `StartRedis()` to restart Redis
5. Clean up Redis state in AfterEach
6. Remove `XIt` prefix from connection failure test
7. Run tests to verify

**Expected Result**: 63/63 tests passing (100%)

---

### **Deferred** (v2.0)
- **Test 3**: CRD Deletion Cleanup (requires controller implementation)
- **Test 4**: Pipeline Command Failures (move to E2E tier)
- **Test 5**: Connection Pool Exhaustion (move to load test tier)

---

## 🔍 **Detailed Implementation: Test 1 (TTL Expiration)**

### **Current Code** (line 101-135)
```go
XIt("should expire deduplication entries after TTL", func() {
    // TODO: This test waits 6 minutes for TTL expiration
    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    time.Sleep(6 * time.Minute) // ❌ TOO LONG

    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))
})
```

### **Proposed Implementation**
```go
It("should expire deduplication entries after TTL", func() {
    // BR-GATEWAY-008: TTL-based expiration
    // BUSINESS OUTCOME: Old fingerprints cleaned up automatically
    // TEST-SPECIFIC: Using 5-second TTL for fast testing

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "TTLTest",
        Namespace: "production",
    })

    // Send alert
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: Fingerprint stored with TTL
    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(1))

    // Wait for TTL to expire (5 seconds + 1 second buffer)
    time.Sleep(6 * time.Second)

    // BUSINESS OUTCOME: Expired fingerprints removed
    fingerprintCount = redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))

    // Send same alert again - should create new CRD
    resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp2.StatusCode).To(Equal(201)) // New CRD created
})
```

### **Required Code Changes**

#### **1. Update DeduplicationService Config**
**File**: `pkg/gateway/processing/deduplication.go`

```go
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // NEW: Configurable TTL
    logger      *zap.Logger
}

func NewDeduplicationService(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger) *DeduplicationService {
    if ttl == 0 {
        ttl = 5 * time.Minute // Default: 5 minutes
    }
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         ttl,
        logger:      logger,
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, fingerprint string, namespace string) (bool, error) {
    key := fmt.Sprintf("dedup:%s:%s", namespace, fingerprint)

    // Check if fingerprint exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    if exists > 0 {
        return true, nil // Duplicate found
    }

    // Store fingerprint with TTL
    err = d.redisClient.Set(ctx, key, time.Now().Unix(), d.ttl).Err()
    if err != nil {
        return false, fmt.Errorf("failed to store fingerprint: %w", err)
    }

    return false, nil // Not a duplicate
}
```

#### **2. Update Test Helper**
**File**: `test/integration/gateway/helpers.go`

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // ... existing code ...

    // Create deduplication service with TEST TTL (5 seconds)
    dedupService := processing.NewDeduplicationService(
        redisClient.Client,
        5*time.Second, // TEST ONLY: 5 seconds (production: 5 minutes)
        logger,
    )

    // ... rest of code ...
}
```

---

## 🔍 **Detailed Implementation: Test 2 (Redis Connection Failure)**

### **Current Code** (line 137-158)
```go
XIt("should handle Redis connection failure gracefully", func() {
    // TODO: This test closes the test Redis client, not the server
    _ = redisClient.Client.Close() // ❌ WRONG APPROACH

    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
})
```

### **Proposed Implementation**
```go
It("should handle Redis connection failure gracefully", func() {
    // BR-GATEWAY-008: Redis failure handling
    // BUSINESS OUTCOME: Gateway rejects requests when Redis unavailable (503)
    // DD-GATEWAY-002: Fail-fast strategy for Redis outages

    // Stop Redis container to simulate failure
    err := StopRedis()
    Expect(err).ToNot(HaveOccurred())

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "RedisFailureTest",
        Namespace: "production",
    })

    // Send alert (should return 503 - service unavailable)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    // BUSINESS OUTCOME: Request rejected with 503 (Redis unavailable)
    Expect(resp.StatusCode).To(Equal(503))
    Expect(resp.Body).To(ContainSubstring("storm detection service unavailable"))

    // Restart Redis for subsequent tests
    err = StartRedis()
    Expect(err).ToNot(HaveOccurred())

    // Verify Redis is back online
    err = redisClient.Client.Ping(ctx).Err()
    Expect(err).ToNot(HaveOccurred())
})
```

### **Required Helper Functions**
**File**: `test/integration/gateway/helpers.go`

```go
// StopRedis stops the Redis container for chaos testing
func StopRedis() error {
    cmd := exec.Command("podman", "stop", "redis-gateway-test")
    return cmd.Run()
}

// StartRedis starts the Redis container after chaos testing
func StartRedis() error {
    // Start Redis container
    cmd := exec.Command("podman", "start", "redis-gateway-test")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    for i := 0; i < 10; i++ {
        cmd := exec.Command("podman", "exec", "redis-gateway-test", "redis-cli", "PING")
        if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "PONG" {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("Redis failed to start after 10 seconds")
}
```

---

## 🎯 **Final Recommendation**

### **Implement Phase 1 Only** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - High confidence, low risk, clear business value

**Rationale**:
- Simple implementation (configurable TTL)
- No infrastructure changes needed
- Clear test (wait 6 seconds, verify expiration)
- High confidence (85%)

### **Defer Phase 2** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Medium confidence, requires chaos testing

**Rationale**:
- Requires stopping/starting Redis container
- May cause flakiness in test suite
- Moderate confidence (60%)
- Can be implemented later if needed

### **Defer to v2.0**
❌ **Tests 3-5** - Low confidence or out of scope

---

## 📊 **Expected Outcomes**

### **Phase 1 Complete**
```
Before:  61/61 tests passing (100%)
After:   62/62 tests passing (100%) ⬆️ +1 test
Time:    ~38 seconds (stable)
```

### **Phase 1 + Phase 2 Complete**
```
Before:  61/61 tests passing (100%)
After:   63/63 tests passing (100%) ⬆️ +2 tests
Time:    ~40 seconds (+2 seconds for Redis stop/start)
```

---

**Status**: ✅ **READY FOR PHASE 1 IMPLEMENTATION**
**Recommendation**: Implement Test 1 (TTL Expiration) now, defer Test 2 and others


**Date**: 2025-10-27
**Status**: 🔍 **ASSESSMENT COMPLETE**
**Current**: 61/61 tests passing (100%)

---

## 📊 **Disabled Redis Tests Analysis**

### **Test 1: TTL Expiration** (line 101)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Waits 6 minutes for TTL expiration (impractical for integration tests)

**Implementation Options**:
1. **Configurable TTL** (5 seconds for tests, 5 minutes for production)
2. **Mock TTL expiration** (simulate time passage)
3. **Move to E2E tier** (accept long test duration)

**Confidence to Implement**: **85%** ✅
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (configurable TTL)

---

### **Test 2: Redis Connection Failure** (line 137)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Closes test client, not server; needs chaos testing infrastructure

**Implementation Options**:
1. **Stop Redis container** (podman stop redis-gateway-test)
2. **Network partition simulation** (iptables rules)
3. **Move to E2E tier** (dedicated chaos testing)

**Confidence to Implement**: **60%** ⚠️
**Effort**: 2-3 hours
**Recommendation**: **Option 1** (stop Redis container, expect 503)

---

### **Test 3: CRD Deletion Cleanup** (line 238)
**Status**: ✅ **DELETED** (DD-GATEWAY-005)
**Reason**: Current TTL-based cleanup is intentional design, not a missing feature

**Decision**: **Option A - Current behavior is correct** ✅
- Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
- This protects against false positives and alert storms after CRD deletion
- If admin deletes CRD, same alert shouldn't immediately recreate it
- **Design Decision**: [DD-GATEWAY-005](../../docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md)

**Action Taken**: Test deleted from `redis_integration_test.go` with explanatory comment

---

### **Test 4: Pipeline Command Failures** (line 335)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Requires Redis failure injection not available in integration tests

**Implementation Options**:
1. **Mock Redis client** (inject failures)
2. **Redis proxy** (intercept and fail commands)
3. **Move to E2E tier** (chaos testing)

**Confidence to Implement**: **40%** ❌
**Effort**: 3-4 hours
**Recommendation**: **Option 3** (move to E2E, too complex for integration)

---

### **Test 5: Connection Pool Exhaustion** (line 370)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: This is a LOAD TEST, not an integration test

**Implementation Options**:
1. **Move to load test tier** (test/load/gateway/)
2. **Reduce concurrency** (20 requests, not 200)
3. **Skip test** (not appropriate for integration tier)

**Confidence to Implement**: **70%** ⚠️
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (move to load test tier, out of scope)

---

## 🎯 **Recommendation Summary**

### **Implement Now** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - Configurable TTL approach

### **Implement with Caution** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Stop Redis container approach

### **Deleted** (DD-GATEWAY-005)
✅ **Test 3: CRD Deletion Cleanup** - Current TTL-based behavior is intentional design

### **Defer to E2E Tier** (40% confidence, 3-4 hours)
❌ **Test 4: Pipeline Command Failures** - Too complex for integration tier

### **Out of Scope** (70% confidence, 1-2 hours)
❌ **Test 5: Connection Pool Exhaustion** - Belongs in load test tier

---

## 📋 **Revised Action Plan**

### **Phase 1: Quick Win (1-2 hours, 85% confidence)**
**Goal**: Implement TTL expiration test with configurable TTL

**Steps**:
1. Add `TTL` field to `DeduplicationService` config (default: 5 minutes, test: 5 seconds)
2. Update `NewDeduplicationService` to accept TTL parameter
3. Update test helpers to use 5-second TTL for tests
4. Remove `XIt` prefix from TTL expiration test
5. Update test to wait 6 seconds (not 6 minutes)
6. Run tests to verify

**Expected Result**: 62/62 tests passing (100%)

---

### **Phase 2: Medium Risk (2-3 hours, 60% confidence)**
**Goal**: Implement Redis connection failure test

**Steps**:
1. Add helper function `StopRedis()` to test infrastructure
2. Update test to stop Redis container before sending webhook
3. Expect 503 response (storm detection service unavailable)
4. Add helper function `StartRedis()` to restart Redis
5. Clean up Redis state in AfterEach
6. Remove `XIt` prefix from connection failure test
7. Run tests to verify

**Expected Result**: 63/63 tests passing (100%)

---

### **Deferred** (v2.0)
- **Test 3**: CRD Deletion Cleanup (requires controller implementation)
- **Test 4**: Pipeline Command Failures (move to E2E tier)
- **Test 5**: Connection Pool Exhaustion (move to load test tier)

---

## 🔍 **Detailed Implementation: Test 1 (TTL Expiration)**

### **Current Code** (line 101-135)
```go
XIt("should expire deduplication entries after TTL", func() {
    // TODO: This test waits 6 minutes for TTL expiration
    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    time.Sleep(6 * time.Minute) // ❌ TOO LONG

    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))
})
```

### **Proposed Implementation**
```go
It("should expire deduplication entries after TTL", func() {
    // BR-GATEWAY-008: TTL-based expiration
    // BUSINESS OUTCOME: Old fingerprints cleaned up automatically
    // TEST-SPECIFIC: Using 5-second TTL for fast testing

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "TTLTest",
        Namespace: "production",
    })

    // Send alert
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: Fingerprint stored with TTL
    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(1))

    // Wait for TTL to expire (5 seconds + 1 second buffer)
    time.Sleep(6 * time.Second)

    // BUSINESS OUTCOME: Expired fingerprints removed
    fingerprintCount = redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))

    // Send same alert again - should create new CRD
    resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp2.StatusCode).To(Equal(201)) // New CRD created
})
```

### **Required Code Changes**

#### **1. Update DeduplicationService Config**
**File**: `pkg/gateway/processing/deduplication.go`

```go
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // NEW: Configurable TTL
    logger      *zap.Logger
}

func NewDeduplicationService(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger) *DeduplicationService {
    if ttl == 0 {
        ttl = 5 * time.Minute // Default: 5 minutes
    }
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         ttl,
        logger:      logger,
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, fingerprint string, namespace string) (bool, error) {
    key := fmt.Sprintf("dedup:%s:%s", namespace, fingerprint)

    // Check if fingerprint exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    if exists > 0 {
        return true, nil // Duplicate found
    }

    // Store fingerprint with TTL
    err = d.redisClient.Set(ctx, key, time.Now().Unix(), d.ttl).Err()
    if err != nil {
        return false, fmt.Errorf("failed to store fingerprint: %w", err)
    }

    return false, nil // Not a duplicate
}
```

#### **2. Update Test Helper**
**File**: `test/integration/gateway/helpers.go`

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // ... existing code ...

    // Create deduplication service with TEST TTL (5 seconds)
    dedupService := processing.NewDeduplicationService(
        redisClient.Client,
        5*time.Second, // TEST ONLY: 5 seconds (production: 5 minutes)
        logger,
    )

    // ... rest of code ...
}
```

---

## 🔍 **Detailed Implementation: Test 2 (Redis Connection Failure)**

### **Current Code** (line 137-158)
```go
XIt("should handle Redis connection failure gracefully", func() {
    // TODO: This test closes the test Redis client, not the server
    _ = redisClient.Client.Close() // ❌ WRONG APPROACH

    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
})
```

### **Proposed Implementation**
```go
It("should handle Redis connection failure gracefully", func() {
    // BR-GATEWAY-008: Redis failure handling
    // BUSINESS OUTCOME: Gateway rejects requests when Redis unavailable (503)
    // DD-GATEWAY-002: Fail-fast strategy for Redis outages

    // Stop Redis container to simulate failure
    err := StopRedis()
    Expect(err).ToNot(HaveOccurred())

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "RedisFailureTest",
        Namespace: "production",
    })

    // Send alert (should return 503 - service unavailable)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    // BUSINESS OUTCOME: Request rejected with 503 (Redis unavailable)
    Expect(resp.StatusCode).To(Equal(503))
    Expect(resp.Body).To(ContainSubstring("storm detection service unavailable"))

    // Restart Redis for subsequent tests
    err = StartRedis()
    Expect(err).ToNot(HaveOccurred())

    // Verify Redis is back online
    err = redisClient.Client.Ping(ctx).Err()
    Expect(err).ToNot(HaveOccurred())
})
```

### **Required Helper Functions**
**File**: `test/integration/gateway/helpers.go`

```go
// StopRedis stops the Redis container for chaos testing
func StopRedis() error {
    cmd := exec.Command("podman", "stop", "redis-gateway-test")
    return cmd.Run()
}

// StartRedis starts the Redis container after chaos testing
func StartRedis() error {
    // Start Redis container
    cmd := exec.Command("podman", "start", "redis-gateway-test")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    for i := 0; i < 10; i++ {
        cmd := exec.Command("podman", "exec", "redis-gateway-test", "redis-cli", "PING")
        if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "PONG" {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("Redis failed to start after 10 seconds")
}
```

---

## 🎯 **Final Recommendation**

### **Implement Phase 1 Only** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - High confidence, low risk, clear business value

**Rationale**:
- Simple implementation (configurable TTL)
- No infrastructure changes needed
- Clear test (wait 6 seconds, verify expiration)
- High confidence (85%)

### **Defer Phase 2** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Medium confidence, requires chaos testing

**Rationale**:
- Requires stopping/starting Redis container
- May cause flakiness in test suite
- Moderate confidence (60%)
- Can be implemented later if needed

### **Defer to v2.0**
❌ **Tests 3-5** - Low confidence or out of scope

---

## 📊 **Expected Outcomes**

### **Phase 1 Complete**
```
Before:  61/61 tests passing (100%)
After:   62/62 tests passing (100%) ⬆️ +1 test
Time:    ~38 seconds (stable)
```

### **Phase 1 + Phase 2 Complete**
```
Before:  61/61 tests passing (100%)
After:   63/63 tests passing (100%) ⬆️ +2 tests
Time:    ~40 seconds (+2 seconds for Redis stop/start)
```

---

**Status**: ✅ **READY FOR PHASE 1 IMPLEMENTATION**
**Recommendation**: Implement Test 1 (TTL Expiration) now, defer Test 2 and others


**Date**: 2025-10-27
**Status**: 🔍 **ASSESSMENT COMPLETE**
**Current**: 61/61 tests passing (100%)

---

## 📊 **Disabled Redis Tests Analysis**

### **Test 1: TTL Expiration** (line 101)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Waits 6 minutes for TTL expiration (impractical for integration tests)

**Implementation Options**:
1. **Configurable TTL** (5 seconds for tests, 5 minutes for production)
2. **Mock TTL expiration** (simulate time passage)
3. **Move to E2E tier** (accept long test duration)

**Confidence to Implement**: **85%** ✅
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (configurable TTL)

---

### **Test 2: Redis Connection Failure** (line 137)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Closes test client, not server; needs chaos testing infrastructure

**Implementation Options**:
1. **Stop Redis container** (podman stop redis-gateway-test)
2. **Network partition simulation** (iptables rules)
3. **Move to E2E tier** (dedicated chaos testing)

**Confidence to Implement**: **60%** ⚠️
**Effort**: 2-3 hours
**Recommendation**: **Option 1** (stop Redis container, expect 503)

---

### **Test 3: CRD Deletion Cleanup** (line 238)
**Status**: ✅ **DELETED** (DD-GATEWAY-005)
**Reason**: Current TTL-based cleanup is intentional design, not a missing feature

**Decision**: **Option A - Current behavior is correct** ✅
- Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
- This protects against false positives and alert storms after CRD deletion
- If admin deletes CRD, same alert shouldn't immediately recreate it
- **Design Decision**: [DD-GATEWAY-005](../../docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md)

**Action Taken**: Test deleted from `redis_integration_test.go` with explanatory comment

---

### **Test 4: Pipeline Command Failures** (line 335)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Requires Redis failure injection not available in integration tests

**Implementation Options**:
1. **Mock Redis client** (inject failures)
2. **Redis proxy** (intercept and fail commands)
3. **Move to E2E tier** (chaos testing)

**Confidence to Implement**: **40%** ❌
**Effort**: 3-4 hours
**Recommendation**: **Option 3** (move to E2E, too complex for integration)

---

### **Test 5: Connection Pool Exhaustion** (line 370)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: This is a LOAD TEST, not an integration test

**Implementation Options**:
1. **Move to load test tier** (test/load/gateway/)
2. **Reduce concurrency** (20 requests, not 200)
3. **Skip test** (not appropriate for integration tier)

**Confidence to Implement**: **70%** ⚠️
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (move to load test tier, out of scope)

---

## 🎯 **Recommendation Summary**

### **Implement Now** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - Configurable TTL approach

### **Implement with Caution** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Stop Redis container approach

### **Deleted** (DD-GATEWAY-005)
✅ **Test 3: CRD Deletion Cleanup** - Current TTL-based behavior is intentional design

### **Defer to E2E Tier** (40% confidence, 3-4 hours)
❌ **Test 4: Pipeline Command Failures** - Too complex for integration tier

### **Out of Scope** (70% confidence, 1-2 hours)
❌ **Test 5: Connection Pool Exhaustion** - Belongs in load test tier

---

## 📋 **Revised Action Plan**

### **Phase 1: Quick Win (1-2 hours, 85% confidence)**
**Goal**: Implement TTL expiration test with configurable TTL

**Steps**:
1. Add `TTL` field to `DeduplicationService` config (default: 5 minutes, test: 5 seconds)
2. Update `NewDeduplicationService` to accept TTL parameter
3. Update test helpers to use 5-second TTL for tests
4. Remove `XIt` prefix from TTL expiration test
5. Update test to wait 6 seconds (not 6 minutes)
6. Run tests to verify

**Expected Result**: 62/62 tests passing (100%)

---

### **Phase 2: Medium Risk (2-3 hours, 60% confidence)**
**Goal**: Implement Redis connection failure test

**Steps**:
1. Add helper function `StopRedis()` to test infrastructure
2. Update test to stop Redis container before sending webhook
3. Expect 503 response (storm detection service unavailable)
4. Add helper function `StartRedis()` to restart Redis
5. Clean up Redis state in AfterEach
6. Remove `XIt` prefix from connection failure test
7. Run tests to verify

**Expected Result**: 63/63 tests passing (100%)

---

### **Deferred** (v2.0)
- **Test 3**: CRD Deletion Cleanup (requires controller implementation)
- **Test 4**: Pipeline Command Failures (move to E2E tier)
- **Test 5**: Connection Pool Exhaustion (move to load test tier)

---

## 🔍 **Detailed Implementation: Test 1 (TTL Expiration)**

### **Current Code** (line 101-135)
```go
XIt("should expire deduplication entries after TTL", func() {
    // TODO: This test waits 6 minutes for TTL expiration
    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    time.Sleep(6 * time.Minute) // ❌ TOO LONG

    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))
})
```

### **Proposed Implementation**
```go
It("should expire deduplication entries after TTL", func() {
    // BR-GATEWAY-008: TTL-based expiration
    // BUSINESS OUTCOME: Old fingerprints cleaned up automatically
    // TEST-SPECIFIC: Using 5-second TTL for fast testing

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "TTLTest",
        Namespace: "production",
    })

    // Send alert
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: Fingerprint stored with TTL
    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(1))

    // Wait for TTL to expire (5 seconds + 1 second buffer)
    time.Sleep(6 * time.Second)

    // BUSINESS OUTCOME: Expired fingerprints removed
    fingerprintCount = redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))

    // Send same alert again - should create new CRD
    resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp2.StatusCode).To(Equal(201)) // New CRD created
})
```

### **Required Code Changes**

#### **1. Update DeduplicationService Config**
**File**: `pkg/gateway/processing/deduplication.go`

```go
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // NEW: Configurable TTL
    logger      *zap.Logger
}

func NewDeduplicationService(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger) *DeduplicationService {
    if ttl == 0 {
        ttl = 5 * time.Minute // Default: 5 minutes
    }
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         ttl,
        logger:      logger,
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, fingerprint string, namespace string) (bool, error) {
    key := fmt.Sprintf("dedup:%s:%s", namespace, fingerprint)

    // Check if fingerprint exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    if exists > 0 {
        return true, nil // Duplicate found
    }

    // Store fingerprint with TTL
    err = d.redisClient.Set(ctx, key, time.Now().Unix(), d.ttl).Err()
    if err != nil {
        return false, fmt.Errorf("failed to store fingerprint: %w", err)
    }

    return false, nil // Not a duplicate
}
```

#### **2. Update Test Helper**
**File**: `test/integration/gateway/helpers.go`

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // ... existing code ...

    // Create deduplication service with TEST TTL (5 seconds)
    dedupService := processing.NewDeduplicationService(
        redisClient.Client,
        5*time.Second, // TEST ONLY: 5 seconds (production: 5 minutes)
        logger,
    )

    // ... rest of code ...
}
```

---

## 🔍 **Detailed Implementation: Test 2 (Redis Connection Failure)**

### **Current Code** (line 137-158)
```go
XIt("should handle Redis connection failure gracefully", func() {
    // TODO: This test closes the test Redis client, not the server
    _ = redisClient.Client.Close() // ❌ WRONG APPROACH

    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
})
```

### **Proposed Implementation**
```go
It("should handle Redis connection failure gracefully", func() {
    // BR-GATEWAY-008: Redis failure handling
    // BUSINESS OUTCOME: Gateway rejects requests when Redis unavailable (503)
    // DD-GATEWAY-002: Fail-fast strategy for Redis outages

    // Stop Redis container to simulate failure
    err := StopRedis()
    Expect(err).ToNot(HaveOccurred())

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "RedisFailureTest",
        Namespace: "production",
    })

    // Send alert (should return 503 - service unavailable)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    // BUSINESS OUTCOME: Request rejected with 503 (Redis unavailable)
    Expect(resp.StatusCode).To(Equal(503))
    Expect(resp.Body).To(ContainSubstring("storm detection service unavailable"))

    // Restart Redis for subsequent tests
    err = StartRedis()
    Expect(err).ToNot(HaveOccurred())

    // Verify Redis is back online
    err = redisClient.Client.Ping(ctx).Err()
    Expect(err).ToNot(HaveOccurred())
})
```

### **Required Helper Functions**
**File**: `test/integration/gateway/helpers.go`

```go
// StopRedis stops the Redis container for chaos testing
func StopRedis() error {
    cmd := exec.Command("podman", "stop", "redis-gateway-test")
    return cmd.Run()
}

// StartRedis starts the Redis container after chaos testing
func StartRedis() error {
    // Start Redis container
    cmd := exec.Command("podman", "start", "redis-gateway-test")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    for i := 0; i < 10; i++ {
        cmd := exec.Command("podman", "exec", "redis-gateway-test", "redis-cli", "PING")
        if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "PONG" {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("Redis failed to start after 10 seconds")
}
```

---

## 🎯 **Final Recommendation**

### **Implement Phase 1 Only** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - High confidence, low risk, clear business value

**Rationale**:
- Simple implementation (configurable TTL)
- No infrastructure changes needed
- Clear test (wait 6 seconds, verify expiration)
- High confidence (85%)

### **Defer Phase 2** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Medium confidence, requires chaos testing

**Rationale**:
- Requires stopping/starting Redis container
- May cause flakiness in test suite
- Moderate confidence (60%)
- Can be implemented later if needed

### **Defer to v2.0**
❌ **Tests 3-5** - Low confidence or out of scope

---

## 📊 **Expected Outcomes**

### **Phase 1 Complete**
```
Before:  61/61 tests passing (100%)
After:   62/62 tests passing (100%) ⬆️ +1 test
Time:    ~38 seconds (stable)
```

### **Phase 1 + Phase 2 Complete**
```
Before:  61/61 tests passing (100%)
After:   63/63 tests passing (100%) ⬆️ +2 tests
Time:    ~40 seconds (+2 seconds for Redis stop/start)
```

---

**Status**: ✅ **READY FOR PHASE 1 IMPLEMENTATION**
**Recommendation**: Implement Test 1 (TTL Expiration) now, defer Test 2 and others




**Date**: 2025-10-27
**Status**: 🔍 **ASSESSMENT COMPLETE**
**Current**: 61/61 tests passing (100%)

---

## 📊 **Disabled Redis Tests Analysis**

### **Test 1: TTL Expiration** (line 101)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Waits 6 minutes for TTL expiration (impractical for integration tests)

**Implementation Options**:
1. **Configurable TTL** (5 seconds for tests, 5 minutes for production)
2. **Mock TTL expiration** (simulate time passage)
3. **Move to E2E tier** (accept long test duration)

**Confidence to Implement**: **85%** ✅
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (configurable TTL)

---

### **Test 2: Redis Connection Failure** (line 137)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Closes test client, not server; needs chaos testing infrastructure

**Implementation Options**:
1. **Stop Redis container** (podman stop redis-gateway-test)
2. **Network partition simulation** (iptables rules)
3. **Move to E2E tier** (dedicated chaos testing)

**Confidence to Implement**: **60%** ⚠️
**Effort**: 2-3 hours
**Recommendation**: **Option 1** (stop Redis container, expect 503)

---

### **Test 3: CRD Deletion Cleanup** (line 238)
**Status**: ✅ **DELETED** (DD-GATEWAY-005)
**Reason**: Current TTL-based cleanup is intentional design, not a missing feature

**Decision**: **Option A - Current behavior is correct** ✅
- Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
- This protects against false positives and alert storms after CRD deletion
- If admin deletes CRD, same alert shouldn't immediately recreate it
- **Design Decision**: [DD-GATEWAY-005](../../docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md)

**Action Taken**: Test deleted from `redis_integration_test.go` with explanatory comment

---

### **Test 4: Pipeline Command Failures** (line 335)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Requires Redis failure injection not available in integration tests

**Implementation Options**:
1. **Mock Redis client** (inject failures)
2. **Redis proxy** (intercept and fail commands)
3. **Move to E2E tier** (chaos testing)

**Confidence to Implement**: **40%** ❌
**Effort**: 3-4 hours
**Recommendation**: **Option 3** (move to E2E, too complex for integration)

---

### **Test 5: Connection Pool Exhaustion** (line 370)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: This is a LOAD TEST, not an integration test

**Implementation Options**:
1. **Move to load test tier** (test/load/gateway/)
2. **Reduce concurrency** (20 requests, not 200)
3. **Skip test** (not appropriate for integration tier)

**Confidence to Implement**: **70%** ⚠️
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (move to load test tier, out of scope)

---

## 🎯 **Recommendation Summary**

### **Implement Now** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - Configurable TTL approach

### **Implement with Caution** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Stop Redis container approach

### **Deleted** (DD-GATEWAY-005)
✅ **Test 3: CRD Deletion Cleanup** - Current TTL-based behavior is intentional design

### **Defer to E2E Tier** (40% confidence, 3-4 hours)
❌ **Test 4: Pipeline Command Failures** - Too complex for integration tier

### **Out of Scope** (70% confidence, 1-2 hours)
❌ **Test 5: Connection Pool Exhaustion** - Belongs in load test tier

---

## 📋 **Revised Action Plan**

### **Phase 1: Quick Win (1-2 hours, 85% confidence)**
**Goal**: Implement TTL expiration test with configurable TTL

**Steps**:
1. Add `TTL` field to `DeduplicationService` config (default: 5 minutes, test: 5 seconds)
2. Update `NewDeduplicationService` to accept TTL parameter
3. Update test helpers to use 5-second TTL for tests
4. Remove `XIt` prefix from TTL expiration test
5. Update test to wait 6 seconds (not 6 minutes)
6. Run tests to verify

**Expected Result**: 62/62 tests passing (100%)

---

### **Phase 2: Medium Risk (2-3 hours, 60% confidence)**
**Goal**: Implement Redis connection failure test

**Steps**:
1. Add helper function `StopRedis()` to test infrastructure
2. Update test to stop Redis container before sending webhook
3. Expect 503 response (storm detection service unavailable)
4. Add helper function `StartRedis()` to restart Redis
5. Clean up Redis state in AfterEach
6. Remove `XIt` prefix from connection failure test
7. Run tests to verify

**Expected Result**: 63/63 tests passing (100%)

---

### **Deferred** (v2.0)
- **Test 3**: CRD Deletion Cleanup (requires controller implementation)
- **Test 4**: Pipeline Command Failures (move to E2E tier)
- **Test 5**: Connection Pool Exhaustion (move to load test tier)

---

## 🔍 **Detailed Implementation: Test 1 (TTL Expiration)**

### **Current Code** (line 101-135)
```go
XIt("should expire deduplication entries after TTL", func() {
    // TODO: This test waits 6 minutes for TTL expiration
    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    time.Sleep(6 * time.Minute) // ❌ TOO LONG

    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))
})
```

### **Proposed Implementation**
```go
It("should expire deduplication entries after TTL", func() {
    // BR-GATEWAY-008: TTL-based expiration
    // BUSINESS OUTCOME: Old fingerprints cleaned up automatically
    // TEST-SPECIFIC: Using 5-second TTL for fast testing

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "TTLTest",
        Namespace: "production",
    })

    // Send alert
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: Fingerprint stored with TTL
    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(1))

    // Wait for TTL to expire (5 seconds + 1 second buffer)
    time.Sleep(6 * time.Second)

    // BUSINESS OUTCOME: Expired fingerprints removed
    fingerprintCount = redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))

    // Send same alert again - should create new CRD
    resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp2.StatusCode).To(Equal(201)) // New CRD created
})
```

### **Required Code Changes**

#### **1. Update DeduplicationService Config**
**File**: `pkg/gateway/processing/deduplication.go`

```go
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // NEW: Configurable TTL
    logger      *zap.Logger
}

func NewDeduplicationService(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger) *DeduplicationService {
    if ttl == 0 {
        ttl = 5 * time.Minute // Default: 5 minutes
    }
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         ttl,
        logger:      logger,
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, fingerprint string, namespace string) (bool, error) {
    key := fmt.Sprintf("dedup:%s:%s", namespace, fingerprint)

    // Check if fingerprint exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    if exists > 0 {
        return true, nil // Duplicate found
    }

    // Store fingerprint with TTL
    err = d.redisClient.Set(ctx, key, time.Now().Unix(), d.ttl).Err()
    if err != nil {
        return false, fmt.Errorf("failed to store fingerprint: %w", err)
    }

    return false, nil // Not a duplicate
}
```

#### **2. Update Test Helper**
**File**: `test/integration/gateway/helpers.go`

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // ... existing code ...

    // Create deduplication service with TEST TTL (5 seconds)
    dedupService := processing.NewDeduplicationService(
        redisClient.Client,
        5*time.Second, // TEST ONLY: 5 seconds (production: 5 minutes)
        logger,
    )

    // ... rest of code ...
}
```

---

## 🔍 **Detailed Implementation: Test 2 (Redis Connection Failure)**

### **Current Code** (line 137-158)
```go
XIt("should handle Redis connection failure gracefully", func() {
    // TODO: This test closes the test Redis client, not the server
    _ = redisClient.Client.Close() // ❌ WRONG APPROACH

    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
})
```

### **Proposed Implementation**
```go
It("should handle Redis connection failure gracefully", func() {
    // BR-GATEWAY-008: Redis failure handling
    // BUSINESS OUTCOME: Gateway rejects requests when Redis unavailable (503)
    // DD-GATEWAY-002: Fail-fast strategy for Redis outages

    // Stop Redis container to simulate failure
    err := StopRedis()
    Expect(err).ToNot(HaveOccurred())

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "RedisFailureTest",
        Namespace: "production",
    })

    // Send alert (should return 503 - service unavailable)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    // BUSINESS OUTCOME: Request rejected with 503 (Redis unavailable)
    Expect(resp.StatusCode).To(Equal(503))
    Expect(resp.Body).To(ContainSubstring("storm detection service unavailable"))

    // Restart Redis for subsequent tests
    err = StartRedis()
    Expect(err).ToNot(HaveOccurred())

    // Verify Redis is back online
    err = redisClient.Client.Ping(ctx).Err()
    Expect(err).ToNot(HaveOccurred())
})
```

### **Required Helper Functions**
**File**: `test/integration/gateway/helpers.go`

```go
// StopRedis stops the Redis container for chaos testing
func StopRedis() error {
    cmd := exec.Command("podman", "stop", "redis-gateway-test")
    return cmd.Run()
}

// StartRedis starts the Redis container after chaos testing
func StartRedis() error {
    // Start Redis container
    cmd := exec.Command("podman", "start", "redis-gateway-test")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    for i := 0; i < 10; i++ {
        cmd := exec.Command("podman", "exec", "redis-gateway-test", "redis-cli", "PING")
        if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "PONG" {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("Redis failed to start after 10 seconds")
}
```

---

## 🎯 **Final Recommendation**

### **Implement Phase 1 Only** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - High confidence, low risk, clear business value

**Rationale**:
- Simple implementation (configurable TTL)
- No infrastructure changes needed
- Clear test (wait 6 seconds, verify expiration)
- High confidence (85%)

### **Defer Phase 2** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Medium confidence, requires chaos testing

**Rationale**:
- Requires stopping/starting Redis container
- May cause flakiness in test suite
- Moderate confidence (60%)
- Can be implemented later if needed

### **Defer to v2.0**
❌ **Tests 3-5** - Low confidence or out of scope

---

## 📊 **Expected Outcomes**

### **Phase 1 Complete**
```
Before:  61/61 tests passing (100%)
After:   62/62 tests passing (100%) ⬆️ +1 test
Time:    ~38 seconds (stable)
```

### **Phase 1 + Phase 2 Complete**
```
Before:  61/61 tests passing (100%)
After:   63/63 tests passing (100%) ⬆️ +2 tests
Time:    ~40 seconds (+2 seconds for Redis stop/start)
```

---

**Status**: ✅ **READY FOR PHASE 1 IMPLEMENTATION**
**Recommendation**: Implement Test 1 (TTL Expiration) now, defer Test 2 and others


**Date**: 2025-10-27
**Status**: 🔍 **ASSESSMENT COMPLETE**
**Current**: 61/61 tests passing (100%)

---

## 📊 **Disabled Redis Tests Analysis**

### **Test 1: TTL Expiration** (line 101)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Waits 6 minutes for TTL expiration (impractical for integration tests)

**Implementation Options**:
1. **Configurable TTL** (5 seconds for tests, 5 minutes for production)
2. **Mock TTL expiration** (simulate time passage)
3. **Move to E2E tier** (accept long test duration)

**Confidence to Implement**: **85%** ✅
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (configurable TTL)

---

### **Test 2: Redis Connection Failure** (line 137)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Closes test client, not server; needs chaos testing infrastructure

**Implementation Options**:
1. **Stop Redis container** (podman stop redis-gateway-test)
2. **Network partition simulation** (iptables rules)
3. **Move to E2E tier** (dedicated chaos testing)

**Confidence to Implement**: **60%** ⚠️
**Effort**: 2-3 hours
**Recommendation**: **Option 1** (stop Redis container, expect 503)

---

### **Test 3: CRD Deletion Cleanup** (line 238)
**Status**: ✅ **DELETED** (DD-GATEWAY-005)
**Reason**: Current TTL-based cleanup is intentional design, not a missing feature

**Decision**: **Option A - Current behavior is correct** ✅
- Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
- This protects against false positives and alert storms after CRD deletion
- If admin deletes CRD, same alert shouldn't immediately recreate it
- **Design Decision**: [DD-GATEWAY-005](../../docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md)

**Action Taken**: Test deleted from `redis_integration_test.go` with explanatory comment

---

### **Test 4: Pipeline Command Failures** (line 335)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Requires Redis failure injection not available in integration tests

**Implementation Options**:
1. **Mock Redis client** (inject failures)
2. **Redis proxy** (intercept and fail commands)
3. **Move to E2E tier** (chaos testing)

**Confidence to Implement**: **40%** ❌
**Effort**: 3-4 hours
**Recommendation**: **Option 3** (move to E2E, too complex for integration)

---

### **Test 5: Connection Pool Exhaustion** (line 370)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: This is a LOAD TEST, not an integration test

**Implementation Options**:
1. **Move to load test tier** (test/load/gateway/)
2. **Reduce concurrency** (20 requests, not 200)
3. **Skip test** (not appropriate for integration tier)

**Confidence to Implement**: **70%** ⚠️
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (move to load test tier, out of scope)

---

## 🎯 **Recommendation Summary**

### **Implement Now** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - Configurable TTL approach

### **Implement with Caution** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Stop Redis container approach

### **Deleted** (DD-GATEWAY-005)
✅ **Test 3: CRD Deletion Cleanup** - Current TTL-based behavior is intentional design

### **Defer to E2E Tier** (40% confidence, 3-4 hours)
❌ **Test 4: Pipeline Command Failures** - Too complex for integration tier

### **Out of Scope** (70% confidence, 1-2 hours)
❌ **Test 5: Connection Pool Exhaustion** - Belongs in load test tier

---

## 📋 **Revised Action Plan**

### **Phase 1: Quick Win (1-2 hours, 85% confidence)**
**Goal**: Implement TTL expiration test with configurable TTL

**Steps**:
1. Add `TTL` field to `DeduplicationService` config (default: 5 minutes, test: 5 seconds)
2. Update `NewDeduplicationService` to accept TTL parameter
3. Update test helpers to use 5-second TTL for tests
4. Remove `XIt` prefix from TTL expiration test
5. Update test to wait 6 seconds (not 6 minutes)
6. Run tests to verify

**Expected Result**: 62/62 tests passing (100%)

---

### **Phase 2: Medium Risk (2-3 hours, 60% confidence)**
**Goal**: Implement Redis connection failure test

**Steps**:
1. Add helper function `StopRedis()` to test infrastructure
2. Update test to stop Redis container before sending webhook
3. Expect 503 response (storm detection service unavailable)
4. Add helper function `StartRedis()` to restart Redis
5. Clean up Redis state in AfterEach
6. Remove `XIt` prefix from connection failure test
7. Run tests to verify

**Expected Result**: 63/63 tests passing (100%)

---

### **Deferred** (v2.0)
- **Test 3**: CRD Deletion Cleanup (requires controller implementation)
- **Test 4**: Pipeline Command Failures (move to E2E tier)
- **Test 5**: Connection Pool Exhaustion (move to load test tier)

---

## 🔍 **Detailed Implementation: Test 1 (TTL Expiration)**

### **Current Code** (line 101-135)
```go
XIt("should expire deduplication entries after TTL", func() {
    // TODO: This test waits 6 minutes for TTL expiration
    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    time.Sleep(6 * time.Minute) // ❌ TOO LONG

    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))
})
```

### **Proposed Implementation**
```go
It("should expire deduplication entries after TTL", func() {
    // BR-GATEWAY-008: TTL-based expiration
    // BUSINESS OUTCOME: Old fingerprints cleaned up automatically
    // TEST-SPECIFIC: Using 5-second TTL for fast testing

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "TTLTest",
        Namespace: "production",
    })

    // Send alert
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: Fingerprint stored with TTL
    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(1))

    // Wait for TTL to expire (5 seconds + 1 second buffer)
    time.Sleep(6 * time.Second)

    // BUSINESS OUTCOME: Expired fingerprints removed
    fingerprintCount = redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))

    // Send same alert again - should create new CRD
    resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp2.StatusCode).To(Equal(201)) // New CRD created
})
```

### **Required Code Changes**

#### **1. Update DeduplicationService Config**
**File**: `pkg/gateway/processing/deduplication.go`

```go
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // NEW: Configurable TTL
    logger      *zap.Logger
}

func NewDeduplicationService(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger) *DeduplicationService {
    if ttl == 0 {
        ttl = 5 * time.Minute // Default: 5 minutes
    }
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         ttl,
        logger:      logger,
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, fingerprint string, namespace string) (bool, error) {
    key := fmt.Sprintf("dedup:%s:%s", namespace, fingerprint)

    // Check if fingerprint exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    if exists > 0 {
        return true, nil // Duplicate found
    }

    // Store fingerprint with TTL
    err = d.redisClient.Set(ctx, key, time.Now().Unix(), d.ttl).Err()
    if err != nil {
        return false, fmt.Errorf("failed to store fingerprint: %w", err)
    }

    return false, nil // Not a duplicate
}
```

#### **2. Update Test Helper**
**File**: `test/integration/gateway/helpers.go`

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // ... existing code ...

    // Create deduplication service with TEST TTL (5 seconds)
    dedupService := processing.NewDeduplicationService(
        redisClient.Client,
        5*time.Second, // TEST ONLY: 5 seconds (production: 5 minutes)
        logger,
    )

    // ... rest of code ...
}
```

---

## 🔍 **Detailed Implementation: Test 2 (Redis Connection Failure)**

### **Current Code** (line 137-158)
```go
XIt("should handle Redis connection failure gracefully", func() {
    // TODO: This test closes the test Redis client, not the server
    _ = redisClient.Client.Close() // ❌ WRONG APPROACH

    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
})
```

### **Proposed Implementation**
```go
It("should handle Redis connection failure gracefully", func() {
    // BR-GATEWAY-008: Redis failure handling
    // BUSINESS OUTCOME: Gateway rejects requests when Redis unavailable (503)
    // DD-GATEWAY-002: Fail-fast strategy for Redis outages

    // Stop Redis container to simulate failure
    err := StopRedis()
    Expect(err).ToNot(HaveOccurred())

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "RedisFailureTest",
        Namespace: "production",
    })

    // Send alert (should return 503 - service unavailable)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    // BUSINESS OUTCOME: Request rejected with 503 (Redis unavailable)
    Expect(resp.StatusCode).To(Equal(503))
    Expect(resp.Body).To(ContainSubstring("storm detection service unavailable"))

    // Restart Redis for subsequent tests
    err = StartRedis()
    Expect(err).ToNot(HaveOccurred())

    // Verify Redis is back online
    err = redisClient.Client.Ping(ctx).Err()
    Expect(err).ToNot(HaveOccurred())
})
```

### **Required Helper Functions**
**File**: `test/integration/gateway/helpers.go`

```go
// StopRedis stops the Redis container for chaos testing
func StopRedis() error {
    cmd := exec.Command("podman", "stop", "redis-gateway-test")
    return cmd.Run()
}

// StartRedis starts the Redis container after chaos testing
func StartRedis() error {
    // Start Redis container
    cmd := exec.Command("podman", "start", "redis-gateway-test")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    for i := 0; i < 10; i++ {
        cmd := exec.Command("podman", "exec", "redis-gateway-test", "redis-cli", "PING")
        if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "PONG" {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("Redis failed to start after 10 seconds")
}
```

---

## 🎯 **Final Recommendation**

### **Implement Phase 1 Only** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - High confidence, low risk, clear business value

**Rationale**:
- Simple implementation (configurable TTL)
- No infrastructure changes needed
- Clear test (wait 6 seconds, verify expiration)
- High confidence (85%)

### **Defer Phase 2** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Medium confidence, requires chaos testing

**Rationale**:
- Requires stopping/starting Redis container
- May cause flakiness in test suite
- Moderate confidence (60%)
- Can be implemented later if needed

### **Defer to v2.0**
❌ **Tests 3-5** - Low confidence or out of scope

---

## 📊 **Expected Outcomes**

### **Phase 1 Complete**
```
Before:  61/61 tests passing (100%)
After:   62/62 tests passing (100%) ⬆️ +1 test
Time:    ~38 seconds (stable)
```

### **Phase 1 + Phase 2 Complete**
```
Before:  61/61 tests passing (100%)
After:   63/63 tests passing (100%) ⬆️ +2 tests
Time:    ~40 seconds (+2 seconds for Redis stop/start)
```

---

**Status**: ✅ **READY FOR PHASE 1 IMPLEMENTATION**
**Recommendation**: Implement Test 1 (TTL Expiration) now, defer Test 2 and others


**Date**: 2025-10-27
**Status**: 🔍 **ASSESSMENT COMPLETE**
**Current**: 61/61 tests passing (100%)

---

## 📊 **Disabled Redis Tests Analysis**

### **Test 1: TTL Expiration** (line 101)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Waits 6 minutes for TTL expiration (impractical for integration tests)

**Implementation Options**:
1. **Configurable TTL** (5 seconds for tests, 5 minutes for production)
2. **Mock TTL expiration** (simulate time passage)
3. **Move to E2E tier** (accept long test duration)

**Confidence to Implement**: **85%** ✅
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (configurable TTL)

---

### **Test 2: Redis Connection Failure** (line 137)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Closes test client, not server; needs chaos testing infrastructure

**Implementation Options**:
1. **Stop Redis container** (podman stop redis-gateway-test)
2. **Network partition simulation** (iptables rules)
3. **Move to E2E tier** (dedicated chaos testing)

**Confidence to Implement**: **60%** ⚠️
**Effort**: 2-3 hours
**Recommendation**: **Option 1** (stop Redis container, expect 503)

---

### **Test 3: CRD Deletion Cleanup** (line 238)
**Status**: ✅ **DELETED** (DD-GATEWAY-005)
**Reason**: Current TTL-based cleanup is intentional design, not a missing feature

**Decision**: **Option A - Current behavior is correct** ✅
- Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
- This protects against false positives and alert storms after CRD deletion
- If admin deletes CRD, same alert shouldn't immediately recreate it
- **Design Decision**: [DD-GATEWAY-005](../../docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md)

**Action Taken**: Test deleted from `redis_integration_test.go` with explanatory comment

---

### **Test 4: Pipeline Command Failures** (line 335)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: Requires Redis failure injection not available in integration tests

**Implementation Options**:
1. **Mock Redis client** (inject failures)
2. **Redis proxy** (intercept and fail commands)
3. **Move to E2E tier** (chaos testing)

**Confidence to Implement**: **40%** ❌
**Effort**: 3-4 hours
**Recommendation**: **Option 3** (move to E2E, too complex for integration)

---

### **Test 5: Connection Pool Exhaustion** (line 370)
**Status**: ⏸️ **PENDING** (XIt)
**Reason**: This is a LOAD TEST, not an integration test

**Implementation Options**:
1. **Move to load test tier** (test/load/gateway/)
2. **Reduce concurrency** (20 requests, not 200)
3. **Skip test** (not appropriate for integration tier)

**Confidence to Implement**: **70%** ⚠️
**Effort**: 1-2 hours
**Recommendation**: **Option 1** (move to load test tier, out of scope)

---

## 🎯 **Recommendation Summary**

### **Implement Now** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - Configurable TTL approach

### **Implement with Caution** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Stop Redis container approach

### **Deleted** (DD-GATEWAY-005)
✅ **Test 3: CRD Deletion Cleanup** - Current TTL-based behavior is intentional design

### **Defer to E2E Tier** (40% confidence, 3-4 hours)
❌ **Test 4: Pipeline Command Failures** - Too complex for integration tier

### **Out of Scope** (70% confidence, 1-2 hours)
❌ **Test 5: Connection Pool Exhaustion** - Belongs in load test tier

---

## 📋 **Revised Action Plan**

### **Phase 1: Quick Win (1-2 hours, 85% confidence)**
**Goal**: Implement TTL expiration test with configurable TTL

**Steps**:
1. Add `TTL` field to `DeduplicationService` config (default: 5 minutes, test: 5 seconds)
2. Update `NewDeduplicationService` to accept TTL parameter
3. Update test helpers to use 5-second TTL for tests
4. Remove `XIt` prefix from TTL expiration test
5. Update test to wait 6 seconds (not 6 minutes)
6. Run tests to verify

**Expected Result**: 62/62 tests passing (100%)

---

### **Phase 2: Medium Risk (2-3 hours, 60% confidence)**
**Goal**: Implement Redis connection failure test

**Steps**:
1. Add helper function `StopRedis()` to test infrastructure
2. Update test to stop Redis container before sending webhook
3. Expect 503 response (storm detection service unavailable)
4. Add helper function `StartRedis()` to restart Redis
5. Clean up Redis state in AfterEach
6. Remove `XIt` prefix from connection failure test
7. Run tests to verify

**Expected Result**: 63/63 tests passing (100%)

---

### **Deferred** (v2.0)
- **Test 3**: CRD Deletion Cleanup (requires controller implementation)
- **Test 4**: Pipeline Command Failures (move to E2E tier)
- **Test 5**: Connection Pool Exhaustion (move to load test tier)

---

## 🔍 **Detailed Implementation: Test 1 (TTL Expiration)**

### **Current Code** (line 101-135)
```go
XIt("should expire deduplication entries after TTL", func() {
    // TODO: This test waits 6 minutes for TTL expiration
    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    time.Sleep(6 * time.Minute) // ❌ TOO LONG

    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))
})
```

### **Proposed Implementation**
```go
It("should expire deduplication entries after TTL", func() {
    // BR-GATEWAY-008: TTL-based expiration
    // BUSINESS OUTCOME: Old fingerprints cleaned up automatically
    // TEST-SPECIFIC: Using 5-second TTL for fast testing

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "TTLTest",
        Namespace: "production",
    })

    // Send alert
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: Fingerprint stored with TTL
    fingerprintCount := redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(1))

    // Wait for TTL to expire (5 seconds + 1 second buffer)
    time.Sleep(6 * time.Second)

    // BUSINESS OUTCOME: Expired fingerprints removed
    fingerprintCount = redisClient.CountFingerprints(ctx, "production")
    Expect(fingerprintCount).To(Equal(0))

    // Send same alert again - should create new CRD
    resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
    Expect(resp2.StatusCode).To(Equal(201)) // New CRD created
})
```

### **Required Code Changes**

#### **1. Update DeduplicationService Config**
**File**: `pkg/gateway/processing/deduplication.go`

```go
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // NEW: Configurable TTL
    logger      *zap.Logger
}

func NewDeduplicationService(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger) *DeduplicationService {
    if ttl == 0 {
        ttl = 5 * time.Minute // Default: 5 minutes
    }
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         ttl,
        logger:      logger,
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, fingerprint string, namespace string) (bool, error) {
    key := fmt.Sprintf("dedup:%s:%s", namespace, fingerprint)

    // Check if fingerprint exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    if exists > 0 {
        return true, nil // Duplicate found
    }

    // Store fingerprint with TTL
    err = d.redisClient.Set(ctx, key, time.Now().Unix(), d.ttl).Err()
    if err != nil {
        return false, fmt.Errorf("failed to store fingerprint: %w", err)
    }

    return false, nil // Not a duplicate
}
```

#### **2. Update Test Helper**
**File**: `test/integration/gateway/helpers.go`

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // ... existing code ...

    // Create deduplication service with TEST TTL (5 seconds)
    dedupService := processing.NewDeduplicationService(
        redisClient.Client,
        5*time.Second, // TEST ONLY: 5 seconds (production: 5 minutes)
        logger,
    )

    // ... rest of code ...
}
```

---

## 🔍 **Detailed Implementation: Test 2 (Redis Connection Failure)**

### **Current Code** (line 137-158)
```go
XIt("should handle Redis connection failure gracefully", func() {
    // TODO: This test closes the test Redis client, not the server
    _ = redisClient.Client.Close() // ❌ WRONG APPROACH

    payload := GeneratePrometheusAlert(...)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
})
```

### **Proposed Implementation**
```go
It("should handle Redis connection failure gracefully", func() {
    // BR-GATEWAY-008: Redis failure handling
    // BUSINESS OUTCOME: Gateway rejects requests when Redis unavailable (503)
    // DD-GATEWAY-002: Fail-fast strategy for Redis outages

    // Stop Redis container to simulate failure
    err := StopRedis()
    Expect(err).ToNot(HaveOccurred())

    payload := GeneratePrometheusAlert(PrometheusAlertOptions{
        AlertName: "RedisFailureTest",
        Namespace: "production",
    })

    // Send alert (should return 503 - service unavailable)
    resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)

    // BUSINESS OUTCOME: Request rejected with 503 (Redis unavailable)
    Expect(resp.StatusCode).To(Equal(503))
    Expect(resp.Body).To(ContainSubstring("storm detection service unavailable"))

    // Restart Redis for subsequent tests
    err = StartRedis()
    Expect(err).ToNot(HaveOccurred())

    // Verify Redis is back online
    err = redisClient.Client.Ping(ctx).Err()
    Expect(err).ToNot(HaveOccurred())
})
```

### **Required Helper Functions**
**File**: `test/integration/gateway/helpers.go`

```go
// StopRedis stops the Redis container for chaos testing
func StopRedis() error {
    cmd := exec.Command("podman", "stop", "redis-gateway-test")
    return cmd.Run()
}

// StartRedis starts the Redis container after chaos testing
func StartRedis() error {
    // Start Redis container
    cmd := exec.Command("podman", "start", "redis-gateway-test")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    for i := 0; i < 10; i++ {
        cmd := exec.Command("podman", "exec", "redis-gateway-test", "redis-cli", "PING")
        if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "PONG" {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("Redis failed to start after 10 seconds")
}
```

---

## 🎯 **Final Recommendation**

### **Implement Phase 1 Only** (85% confidence, 1-2 hours)
✅ **Test 1: TTL Expiration** - High confidence, low risk, clear business value

**Rationale**:
- Simple implementation (configurable TTL)
- No infrastructure changes needed
- Clear test (wait 6 seconds, verify expiration)
- High confidence (85%)

### **Defer Phase 2** (60% confidence, 2-3 hours)
⚠️ **Test 2: Redis Connection Failure** - Medium confidence, requires chaos testing

**Rationale**:
- Requires stopping/starting Redis container
- May cause flakiness in test suite
- Moderate confidence (60%)
- Can be implemented later if needed

### **Defer to v2.0**
❌ **Tests 3-5** - Low confidence or out of scope

---

## 📊 **Expected Outcomes**

### **Phase 1 Complete**
```
Before:  61/61 tests passing (100%)
After:   62/62 tests passing (100%) ⬆️ +1 test
Time:    ~38 seconds (stable)
```

### **Phase 1 + Phase 2 Complete**
```
Before:  61/61 tests passing (100%)
After:   63/63 tests passing (100%) ⬆️ +2 tests
Time:    ~40 seconds (+2 seconds for Redis stop/start)
```

---

**Status**: ✅ **READY FOR PHASE 1 IMPLEMENTATION**
**Recommendation**: Implement Test 1 (TTL Expiration) now, defer Test 2 and others

