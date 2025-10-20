# Day 8 Suite 2: Cache Fallback Testing - APDC Analysis & Plan

**Date**: October 20, 2025
**Status**: 📋 **PLAN PHASE** - Ready for User Approval
**Suite Focus**: Cache Fallback & Graceful Degradation
**Business Requirement**: BR-CONTEXT-005 (Multi-tier caching with graceful degradation)
**Estimated Duration**: 8-12 hours (pure TDD approach - 8 tests)

---

## 📋 **TABLE OF CONTENTS**

1. [APDC Analysis: Context & Existing Implementation](#apdc-analysis)
2. [Test Scenarios & Strategy](#test-scenarios)
3. [APDC Plan: Implementation Strategy](#apdc-plan)
4. [Pure TDD Workflow](#tdd-workflow)
5. [Confidence Assessment](#confidence-assessment)

---

<a name="apdc-analysis"></a>
## 🎯 **APDC PHASE 1: ANALYSIS**

### **1.1 Business Context**

**BR-CONTEXT-005**: Multi-tier caching with graceful degradation

**Requirements**:
- System MUST continue functioning when Redis is unavailable
- API responses MUST NOT fail due to cache infrastructure issues
- Performance degradation is acceptable, but service interruption is NOT
- Health checks MUST report degraded state

**User Impact**:
- ✅ API continues to serve requests (slower, but functional)
- ✅ No 500 errors due to Redis being down
- ✅ Monitoring alerts on degraded state
- ⚠️ Increased database load (cache misses hit DB)

---

### **1.2 Existing Implementation Review**

#### **Code Analysis: `pkg/contextapi/cache/manager.go`**

**Initialization Behavior** (lines 75-121):
```go
// Redis connection test during initialization
redisClient := redis.NewClient(&redis.Options{...})
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

if err := redisClient.Ping(ctx).Err(); err != nil {
    // Graceful degradation: Log warning and continue with LRU only
    logger.Warn("Redis unavailable, using LRU only (graceful degradation)", ...)
    redisClient = nil  // Disable Redis
    redisAvail = false
} else {
    redisAvail = true
}

return &multiTierCache{
    redis:      redisClient,  // May be nil
    memory:     make(map[string]*cacheEntry),
    redisAvail: redisAvail,
    ...
}
```

**Get Operation** (lines 134-179):
```go
func (c *multiTierCache) Get(ctx context.Context, key string) ([]byte, error) {
    // Try L1 (Redis) if available
    if c.redis != nil {
        val, err := c.redis.Get(ctx, key).Bytes()
        if err == nil {
            c.stats.RecordHitL1()
            // Populate L2 for faster access
            c.memory[key] = &cacheEntry{...}
            return val, nil
        }
        if err != redis.Nil {
            c.stats.RecordError()
        }
    }

    // Try L2 (LRU) - graceful fallback
    c.mu.RLock()
    entry, exists := c.memory[key]
    c.mu.RUnlock()

    if exists && time.Now().Before(entry.expiresAt) {
        c.stats.RecordHitL2()
        return entry.data, nil
    }

    // Cache miss
    c.stats.RecordMiss()
    return nil, nil
}
```

**Set Operation** (lines 181-233):
```go
func (c *multiTierCache) Set(ctx context.Context, key string, value interface{}) error {
    // Try L1 (Redis) - best effort
    if c.redis != nil {
        if err := c.redis.Set(ctx, key, jsonData, c.ttl).Err(); err != nil {
            c.stats.RecordError()
            // Continue to L2 (don't fail the operation)
        }
    }

    // Always set in L2 (guaranteed to work)
    c.mu.Lock()
    c.memory[key] = &cacheEntry{...}
    c.mu.Unlock()

    return nil  // Never fails
}
```

**Health Check** (lines 273-293):
```go
func (c *multiTierCache) HealthCheck(ctx context.Context) (*HealthStatus, error) {
    if c.redis == nil {
        return &HealthStatus{
            Degraded: true,
            Message:  "Redis unavailable, using LRU only",
        }, nil
    }

    if err := c.redis.Ping(ctx).Err(); err != nil {
        return &HealthStatus{
            Degraded: true,
            Message:  fmt.Sprintf("Redis unhealthy: %v", err),
        }, nil
    }

    return &HealthStatus{
        Degraded: false,
        Message:  "All cache tiers operational",
    }, nil
}
```

---

### **1.3 Key Findings**

#### **✅ Strengths of Current Implementation**

1. **Initialization Graceful Degradation** ✅
   - Tests Redis connectivity at startup
   - Continues with LRU-only if Redis fails
   - Logs degraded state

2. **Runtime Graceful Degradation** ✅
   - `Get()` falls through from L1 → L2
   - `Set()` continues if Redis fails (always sets L2)
   - No operations fail due to Redis issues

3. **Health Reporting** ✅
   - `HealthCheck()` reports degraded state
   - Distinguishes between Redis unavailable and Redis unhealthy

4. **Statistics Tracking** ✅
   - Records L1 hits, L2 hits, misses
   - Records errors for monitoring

#### **📊 What Needs Testing**

1. **Startup Scenarios** (2 tests)
   - Redis unavailable at startup → Falls back to LRU
   - Redis available at startup → Uses L1+L2

2. **Runtime Failure Scenarios** (3 tests)
   - Redis becomes unavailable mid-operation → Continues with L2
   - Redis timeout during Get → Falls back to L2
   - Redis timeout during Set → Sets L2 only

3. **Recovery Scenarios** (1 test)
   - Redis recovers → Starts using L1 again (if reconnected)

4. **Health Check Scenarios** (2 tests)
   - Health check when Redis down → Reports degraded
   - Health check when Redis up → Reports healthy

**Total**: 8 tests (matches plan)

---

<a name="test-scenarios"></a>
## 🧪 **TEST SCENARIOS & STRATEGY**

### **Test Suite Design**

**File**: `test/integration/contextapi/02_cache_fallback_test.go`
**Approach**: Pure TDD (RED → GREEN → REFACTOR for each test)
**Complexity**: Medium (requires Redis control for failure simulation)

---

### **Test Scenarios** (8 tests)

#### **Scenario 1: Redis Unavailable at Initialization**
```go
It("should initialize with LRU-only when Redis is unavailable", func() {
    // RED: Test will fail - need to simulate Redis down at init
    // GREEN: Create cache manager with invalid Redis address
    // REFACTOR: Extract Redis simulation helper

    cfg := &cache.Config{
        RedisAddr:  "localhost:9999",  // Invalid port (Redis not running)
        RedisDB:    0,
        LRUSize:    100,
        DefaultTTL: 5 * time.Minute,
    }

    manager, err := cache.NewCacheManager(cfg, logger)

    Expect(err).ToNot(HaveOccurred(), "Manager should initialize successfully")

    // Verify degraded state
    health, err := manager.HealthCheck(context.Background())
    Expect(err).ToNot(HaveOccurred())
    Expect(health.Degraded).To(BeTrue(), "Should be in degraded state")
    Expect(health.Message).To(ContainSubstring("Redis unavailable"))
})
```

#### **Scenario 2: Cache Operations Work with LRU-Only**
```go
It("should handle Set and Get with LRU-only (no Redis)", func() {
    // RED: Test will fail - need LRU-only cache
    // GREEN: Use cache from Scenario 1, do Set+Get
    // REFACTOR: Verify L2 stats increment

    // Set value
    err := manager.Set(ctx, "test-key", map[string]string{"value": "test"})
    Expect(err).ToNot(HaveOccurred(), "Set should succeed with LRU only")

    // Get value
    data, err := manager.Get(ctx, "test-key")
    Expect(err).ToNot(HaveOccurred(), "Get should succeed from LRU")
    Expect(data).ToNot(BeNil(), "Should retrieve from L2")

    // Verify L2 hit recorded
    stats := manager.Stats()
    Expect(stats.HitsL2).To(BeNumerically(">", 0), "L2 hit should be recorded")
})
```

#### **Scenario 3: Redis Timeout During Get**
```go
It("should fallback to L2 when Redis times out during Get", func() {
    // RED: Test will fail - need Redis timeout simulation
    // GREEN: Use short context timeout to simulate Redis slow response
    // REFACTOR: Extract timeout helper

    // Setup: Normal cache with Redis, set value in L2
    manager := createCacheWithRedis()
    manager.Set(ctx, "timeout-key", "test-value")

    // Simulate Redis timeout with very short context
    shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
    defer cancel()

    // Should fallback to L2
    data, err := manager.Get(shortCtx, "timeout-key")

    Expect(err).ToNot(HaveOccurred(), "Should succeed despite Redis timeout")
    Expect(data).ToNot(BeNil(), "Should retrieve from L2")

    // Verify L2 hit (not L1)
    stats := manager.Stats()
    Expect(stats.HitsL2).To(BeNumerically(">", 0))
})
```

#### **Scenario 4: Redis Timeout During Set**
```go
It("should complete Set to L2 when Redis times out", func() {
    // RED: Test will fail - need to verify Set continues
    // GREEN: Use short timeout, verify L2 has value
    // REFACTOR: Verify no panic or error

    shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
    defer cancel()

    // Set with timeout context
    err := manager.Set(shortCtx, "set-key", "set-value")

    Expect(err).ToNot(HaveOccurred(), "Set should succeed (L2 only)")

    // Verify value is in L2 (use normal context for Get)
    data, err := manager.Get(context.Background(), "set-key")
    Expect(err).ToNot(HaveOccurred())
    Expect(data).ToNot(BeNil(), "Value should be in L2")
})
```

#### **Scenario 5: LRU Eviction Works Correctly**
```go
It("should evict oldest entries when LRU is full", func() {
    // RED: Test will fail - need to fill LRU and verify eviction
    // GREEN: Create small LRU (size=3), add 5 entries
    // REFACTOR: Verify oldest entries evicted first

    smallCache := createCacheWithSmallLRU(3)  // LRU size = 3

    // Fill LRU beyond capacity
    smallCache.Set(ctx, "key1", "value1")
    smallCache.Set(ctx, "key2", "value2")
    smallCache.Set(ctx, "key3", "value3")
    smallCache.Set(ctx, "key4", "value4")  // Should evict key1
    smallCache.Set(ctx, "key5", "value5")  // Should evict key2

    // Verify oldest entries evicted
    data1, _ := smallCache.Get(ctx, "key1")
    data2, _ := smallCache.Get(ctx, "key2")
    data3, _ := smallCache.Get(ctx, "key3")
    data4, _ := smallCache.Get(ctx, "key4")
    data5, _ := smallCache.Get(ctx, "key5")

    Expect(data1).To(BeNil(), "key1 should be evicted")
    Expect(data2).To(BeNil(), "key2 should be evicted")
    Expect(data3).ToNot(BeNil(), "key3 should still exist")
    Expect(data4).ToNot(BeNil(), "key4 should still exist")
    Expect(data5).ToNot(BeNil(), "key5 should still exist")
})
```

#### **Scenario 6: Health Check Reports Degraded State**
```go
It("should report degraded state when Redis is down", func() {
    // RED: Test will fail - need degraded health check
    // GREEN: Create LRU-only cache, check health
    // REFACTOR: Verify specific message content

    lruOnlyCache := createCacheWithoutRedis()

    health, err := lruOnlyCache.HealthCheck(ctx)

    Expect(err).ToNot(HaveOccurred())
    Expect(health.Degraded).To(BeTrue(), "Should be degraded without Redis")
    Expect(health.Message).To(ContainSubstring("Redis unavailable"))
    Expect(health.Message).To(ContainSubstring("LRU only"))
})
```

#### **Scenario 7: Health Check Reports Healthy State**
```go
It("should report healthy state when Redis is up", func() {
    // RED: Test will fail - need healthy health check
    // GREEN: Create normal cache with Redis, check health
    // REFACTOR: Verify no degraded flag

    normalCache := createCacheWithRedis()

    health, err := normalCache.HealthCheck(ctx)

    Expect(err).ToNot(HaveOccurred())
    Expect(health.Degraded).To(BeFalse(), "Should not be degraded with Redis")
    Expect(health.Message).To(ContainSubstring("operational"))
})
```

#### **Scenario 8: Statistics Tracking During Fallback**
```go
It("should track error statistics when Redis operations fail", func() {
    // RED: Test will fail - need error stat tracking
    // GREEN: Simulate Redis errors, check stats
    // REFACTOR: Verify error count increments

    cache := createCacheWithRedis()
    initialStats := cache.Stats()

    // Simulate Redis errors with timeout context
    shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
    defer cancel()

    // Trigger multiple Redis timeouts
    for i := 0; i < 5; i++ {
        cache.Get(shortCtx, fmt.Sprintf("error-key-%d", i))
    }

    // Verify error stats increased
    finalStats := cache.Stats()
    errorIncrease := finalStats.Errors - initialStats.Errors

    Expect(errorIncrease).To(BeNumerically(">", 0),
        "Error count should increase when Redis times out")
})
```

---

<a name="apdc-plan"></a>
## 📋 **APDC PHASE 2: PLAN**

### **2.1 Implementation Strategy**

#### **Test File Structure**
```go
package contextapi

import (
    "context"
    "fmt"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/contextapi/cache"
)

var _ = Describe("Cache Fallback Integration Tests", func() {
    var (
        ctx    context.Context
        cancel context.CancelFunc
        logger *zap.Logger
    )

    BeforeEach(func() {
        ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
        logger = zaptest.NewLogger(GinkgoT())
    })

    AfterEach(func() {
        cancel()
    })

    Context("Initialization Scenarios", func() {
        It("should initialize with LRU-only when Redis is unavailable", func() {
            // Test #1
        })
    })

    Context("Runtime Fallback Scenarios", func() {
        It("should handle Set and Get with LRU-only", func() {
            // Test #2
        })

        It("should fallback to L2 when Redis times out during Get", func() {
            // Test #3
        })

        It("should complete Set to L2 when Redis times out", func() {
            // Test #4
        })
    })

    Context("LRU Behavior", func() {
        It("should evict oldest entries when LRU is full", func() {
            // Test #5
        })
    })

    Context("Health Check Scenarios", func() {
        It("should report degraded state when Redis is down", func() {
            // Test #6
        })

        It("should report healthy state when Redis is up", func() {
            // Test #7
        })
    })

    Context("Statistics Tracking", func() {
        It("should track error statistics when Redis operations fail", func() {
            // Test #8
        })
    })
})
```

---

### **2.2 Helper Functions Needed**

```go
// createCacheWithoutRedis creates cache manager with invalid Redis address
func createCacheWithoutRedis() cache.CacheManager {
    cfg := &cache.Config{
        RedisAddr:  "localhost:9999",  // Invalid port
        RedisDB:    0,
        LRUSize:    1000,
        DefaultTTL: 5 * time.Minute,
    }
    manager, err := cache.NewCacheManager(cfg, logger)
    Expect(err).ToNot(HaveOccurred())
    return manager
}

// createCacheWithRedis creates cache manager with working Redis
func createCacheWithRedis() cache.CacheManager {
    cfg := &cache.Config{
        RedisAddr:  "localhost:6379",
        RedisDB:    4,  // Use DB 4 for Suite 2 (parallel isolation)
        LRUSize:    1000,
        DefaultTTL: 5 * time.Minute,
    }
    manager, err := cache.NewCacheManager(cfg, logger)
    Expect(err).ToNot(HaveOccurred())
    return manager
}

// createCacheWithSmallLRU creates cache with small LRU for eviction testing
func createCacheWithSmallLRU(size int) cache.CacheManager {
    cfg := &cache.Config{
        RedisAddr:  "localhost:9999",  // No Redis (LRU only)
        RedisDB:    0,
        LRUSize:    size,
        DefaultTTL: 5 * time.Minute,
    }
    manager, err := cache.NewCacheManager(cfg, logger)
    Expect(err).ToNot(HaveOccurred())
    return manager
}
```

---

### **2.3 Redis Database Assignment**

**Suite 2 will use Redis DB 4** for parallel test isolation:
```
01_query_lifecycle_test.go  → Redis DB 0
03_vector_search_test.go    → Redis DB 1
04_aggregation_test.go      → Redis DB 2
05_http_api_test.go         → Redis DB 3
02_cache_fallback_test.go   → Redis DB 4 (NEW)
```

---

<a name="tdd-workflow"></a>
## 🔄 **PURE TDD WORKFLOW**

### **Test-by-Test Implementation**

#### **Test #1: Redis Unavailable at Initialization**
1. **RED** (5-10 min): Write test, verify it fails
2. **GREEN** (5-10 min): Implementation already exists, verify test passes
3. **REFACTOR** (5 min): Extract helper `createCacheWithoutRedis()`
4. **VERIFY** (2 min): Run full suite (42 tests + 1 new = 43 tests passing)

#### **Test #2-8: Repeat for each test**
- Each test follows same RED → GREEN → REFACTOR → VERIFY cycle
- Estimated 30-45 minutes per test
- Total: 8 tests × 40 min avg = 5.3 hours

### **Expected Timeline**

| Phase | Duration | Cumulative |
|-------|----------|------------|
| Test #1-2 | 1.5 hours | 1.5 hours |
| Test #3-4 | 1.5 hours | 3 hours |
| Test #5-6 | 1.5 hours | 4.5 hours |
| Test #7-8 | 1.5 hours | 6 hours |
| **Buffer** | 2 hours | **8 hours** |

**Estimated Completion**: 8 hours (within 8-12 hour estimate)

---

<a name="confidence-assessment"></a>
## 🎯 **CONFIDENCE ASSESSMENT**

### **Plan Quality**: 92% ✅

**High Confidence Because**:
- ✅ Implementation already exists and works (proven by logs)
- ✅ Tests validate existing behavior, not new code
- ✅ Clear test scenarios (8 well-defined tests)
- ✅ Pure TDD approach (1 test at a time)
- ✅ Helper functions simplify test creation
- ✅ Redis DB isolation prevents conflicts (DB 4)

**Risks (8%)**:
- ⚠️ Redis timeout simulation may be flaky (need reliable approach)
- ⚠️ LRU eviction order might not be FIFO (need to verify implementation)

---

## 📊 **Next Steps**

**IMMEDIATE ACTION**: Request User Approval

**Questions for User**:
1. ✅ Approve proceeding with Day 8 Suite 2 (Cache Fallback Testing)?
2. ✅ Approve 8-test strategy as outlined above?
3. ✅ Any specific scenarios to add or modify?

**On Approval**:
1. Create `test/integration/contextapi/02_cache_fallback_test.go`
2. Start with Test #1 (RED → GREEN → REFACTOR)
3. Progress through Tests #2-8 using pure TDD
4. Target: 50/50 tests passing (42 existing + 8 new)

---

**Status**: ⏸️ **AWAITING USER APPROVAL**
**Recommended Action**: **Approve and proceed with Test #1**
