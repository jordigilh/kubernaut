# Redis OOM (Out of Memory) - Root Cause & Fix

**Date**: 2025-10-24
**Status**: ✅ **ROOT CAUSE IDENTIFIED**

---

## 🚨 **Root Cause: Redis Out of Memory**

### **Error Message**

```
OOM command not allowed when used memory > 'maxmemory'.
```

**Meaning**: Redis has reached its `maxmemory` limit and is rejecting all write operations (SET, INCR, Lua scripts).

**Impact**: Gateway returns `503 Service Unavailable` for all requests because:
- Deduplication service cannot write fingerprints → 503
- Storm detection service cannot write storm state → 503
- Rate limiting service cannot increment counters → 503

---

## 📊 **Why This Happened**

### **Integration Test Behavior**

Integration tests run **33 tests** sequentially, each test:
1. Writes fingerprints to Redis (deduplication)
2. Writes storm state to Redis (storm detection)
3. Increments rate limit counters (rate limiting)
4. **Never cleans up Redis between tests**

**Result**: Redis DB 2 fills up with test data until `maxmemory` is reached.

---

## 🛠️ **Immediate Fix: Flush Redis Before Tests**

### **Option A: Flush Redis in BeforeSuite** ⭐ **RECOMMENDED**

**File**: `test/integration/gateway/suite_test.go`

**Add**:
```go
var _ = BeforeSuite(func() {
    // ... existing setup ...

    // CRITICAL: Flush Redis DB 2 before tests to prevent OOM
    ctx := context.Background()
    redisClient := SetupRedisTestClient(ctx)
    if redisClient != nil && redisClient.Client != nil {
        err := redisClient.Client.FlushDB(ctx).Err()
        if err != nil {
            GinkgoWriter.Printf("⚠️  Warning: Failed to flush Redis DB 2: %v\n", err)
        } else {
            GinkgoWriter.Println("✅ Redis DB 2 flushed successfully")
        }
    }
})
```

**Pros**:
- ✅ Simple one-time cleanup before all tests
- ✅ Prevents OOM from previous test runs
- ✅ No per-test overhead

**Cons**:
- ⚠️ Doesn't clean up between individual tests (tests may still interfere)

**Confidence**: **95%** - This will fix the OOM issue

---

### **Option B: Flush Redis in BeforeEach** (More Thorough)

**File**: `test/integration/gateway/suite_test.go`

**Add**:
```go
var _ = BeforeEach(func() {
    // Flush Redis before EACH test to ensure clean state
    ctx := context.Background()
    redisClient := SetupRedisTestClient(ctx)
    if redisClient != nil && redisClient.Client != nil {
        err := redisClient.Client.FlushDB(ctx).Err()
        if err != nil {
            GinkgoWriter.Printf("⚠️  Warning: Failed to flush Redis: %v\n", err)
        }
    }
})
```

**Pros**:
- ✅ Complete isolation between tests
- ✅ Prevents test interference
- ✅ Guarantees clean Redis state for each test

**Cons**:
- ⚠️ Adds ~100ms overhead per test (33 tests × 100ms = 3.3 seconds)

**Confidence**: **98%** - This will fix OOM and prevent test interference

---

### **Option C: Increase Redis maxmemory** (Not Recommended)

**Approach**: Increase Redis `maxmemory` limit

**Why Not Recommended**:
- ❌ Doesn't solve the root problem (tests not cleaning up)
- ❌ Just delays OOM until more tests are added
- ❌ Wastes memory on test data

---

## 💡 **Recommended Implementation**

### **Step 1: Add BeforeSuite Cleanup (Immediate)**

```go
// test/integration/gateway/suite_test.go
var _ = BeforeSuite(func() {
    GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    GinkgoWriter.Println("🚀 Gateway Integration Test Suite - Setup")
    GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

    ctx := context.Background()

    // Step 1: Flush Redis DB 2 to prevent OOM from previous runs
    GinkgoWriter.Println("✓ Step 1: Flushing Redis DB 2...")
    redisClient := SetupRedisTestClient(ctx)
    if redisClient != nil && redisClient.Client != nil {
        err := redisClient.Client.FlushDB(ctx).Err()
        if err != nil {
            GinkgoWriter.Printf("  ⚠️  Warning: Failed to flush Redis DB 2: %v\n", err)
        } else {
            GinkgoWriter.Println("  ✅ Redis DB 2 flushed successfully")
        }
    } else {
        GinkgoWriter.Println("  ⚠️  Warning: Redis client not available (tests may fail)")
    }

    // ... rest of existing setup ...
})
```

---

### **Step 2: Add BeforeEach Cleanup (Optional, for test isolation)**

```go
// test/integration/gateway/suite_test.go
var _ = BeforeEach(func() {
    // Clean Redis state before each test for isolation
    ctx := context.Background()
    redisClient := SetupRedisTestClient(ctx)
    if redisClient != nil && redisClient.Client != nil {
        _ = redisClient.Client.FlushDB(ctx).Err()
    }
})
```

---

## 📋 **Verification**

### **After Implementing Fix**

1. **Flush Redis manually**:
   ```bash
   kubectl exec -n kubernaut-system redis-gateway-0 -c redis -- redis-cli -n 2 FLUSHDB
   ```

2. **Re-run tests**:
   ```bash
   go test -v ./test/integration/gateway -run "TestGatewayIntegration" -timeout 30m
   ```

3. **Expected outcome**:
   - ✅ Tests return `201` (Created) or `202` (Duplicate) instead of `503`
   - ✅ No more OOM errors in logs
   - ✅ Tests complete in 5-10 minutes (not 10 minutes timeout)

---

## 🔍 **Additional Investigation**

### **Check Redis Memory Usage**

```bash
# From Redis pod
kubectl exec -n kubernaut-system redis-gateway-0 -c redis -- redis-cli INFO memory

# Look for:
# used_memory_human: 256M
# maxmemory_human: 256M  ← If these are equal, Redis is full
```

### **Check Redis maxmemory Policy**

```bash
kubectl exec -n kubernaut-system redis-gateway-0 -c redis -- redis-cli CONFIG GET maxmemory-policy

# Should be: allkeys-lru (evict least recently used keys)
# NOT: noeviction (reject writes when full)
```

---

## ✅ **Success Criteria**

After implementing the fix:

1. ✅ **No OOM errors** in test logs
2. ✅ **Tests return 201/202** instead of 503
3. ✅ **Test duration < 10 minutes** (currently timing out at 10 minutes)
4. ✅ **Redis memory usage stable** (not growing indefinitely)

---

## 🔗 **Related Files**

- **Test Suite**: `test/integration/gateway/suite_test.go` - Add Redis flush in `BeforeSuite`
- **Test Helpers**: `test/integration/gateway/helpers.go` - `SetupRedisTestClient()`
- **Redis Config**: `deploy/redis-ha/redis-gateway-statefulset.yaml` - Check `maxmemory` setting

---

## 📊 **Confidence Assessment**

**Confidence**: **98%** - Redis OOM is the confirmed root cause

**Evidence**:
1. ✅ Error logs show `OOM command not allowed when used memory > 'maxmemory'`
2. ✅ All tests return 503 (consistent with Redis unavailable)
3. ✅ Port-forward is working (ruled out connectivity issues)
4. ✅ Redis address is correct (`localhost:6379`)

**Next Steps**:
1. ✅ Implement `BeforeSuite` Redis flush
2. ⏳ Re-run tests
3. ⏳ Verify tests pass
4. ⏳ (Optional) Add `BeforeEach` flush for better test isolation


