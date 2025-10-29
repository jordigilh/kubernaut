# Redis HA Integration Test - Final Triage

**Date**: 2025-10-24
**Test Run**: Second attempt (port-forward to master pod)
**Status**: ❌ **STILL FAILING - 100% 503 errors**

---

## 🚨 **Critical Finding: Port-Forward Works, But Tests Still Fail**

### **Test Results**

**Duration**: 10 minutes (600 seconds, full timeout)
**Total Tests**: ~33 tests
**Passed**: 0
**Failed**: ~33
**Failure Rate**: 100%

**All requests returning**: `503 Service Unavailable` (307 bytes response)

---

## 🔍 **Root Cause Analysis**

### **Problem: Gateway Cannot Connect to Redis via Port-Forward**

**Evidence**:
1. ✅ Port-forward is running: `kubectl port-forward -n kubernaut-system redis-gateway-0 6379:6379` (PID: 96526)
2. ✅ Redis master is healthy: `kubectl exec redis-gateway-0 -c redis -- redis-cli ping` → `PONG`
3. ❌ **Gateway returns 503 for ALL requests** → Redis unavailable from Gateway's perspective

**Why 503?**:
Per `REDIS_FAILURE_HANDLING.md` and `DD-GATEWAY-003`, the Gateway returns `503 Service Unavailable` when:
- Redis is unreachable
- Redis connection fails
- Deduplication service cannot connect to Redis
- Storm detection service cannot connect to Redis
- Rate limiting service cannot connect to Redis

---

## 🎯 **Hypothesis: Integration Tests Use Wrong Redis Address**

### **The Problem**

**Integration tests** create a Gateway server that connects to Redis at `localhost:6379` (via port-forward).

**However**, the Gateway's Redis client configuration might be:
1. **Hardcoded** to `redis-gateway-ha.kubernaut-system:6379` (cluster DNS)
2. **Not overridden** in integration test setup
3. **Using cluster service** instead of `localhost:6379`

**Result**: Gateway tries to connect to `redis-gateway-ha.kubernaut-system:6379` (which doesn't exist in the test environment), fails, and returns 503.

---

## 📋 **Verification Steps**

### **Step 1: Check Integration Test Redis Configuration**

**File**: `test/integration/gateway/helpers.go`

**Look for**:
```go
func StartTestGateway(...) {
    // How is Redis client created?
    redisClient := redis.NewClient(&redis.Options{
        Addr: "???",  // What address is used here?
    })
}
```

**Expected**:
- Integration tests should use `localhost:6379` (port-forward)
- NOT `redis-gateway-ha.kubernaut-system:6379` (cluster DNS)

---

### **Step 2: Check Gateway Server Redis Initialization**

**File**: `pkg/gateway/server/server.go`

**Look for**:
```go
func NewServer(..., redisClient *redis.Client, ...) {
    // Is redisClient passed from tests?
    // Or is it created internally with hardcoded address?
}
```

**Expected**:
- `NewServer` should accept `redisClient` as parameter (✅ it does)
- Integration tests should pass `localhost:6379` client

---

### **Step 3: Verify Test Helper Redis Client Creation**

**File**: `test/integration/gateway/helpers.go`

**Function**: `SetupRedisTestClient()`

**Check**:
```go
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
    client := goredis.NewClient(&goredis.Options{
        Addr:     "localhost:6379",  // ← Should be localhost
        Password: "",
        DB:       0,
    })

    // Verify connectivity
    _, err := client.Ping(ctx).Result()
    if err != nil {
        // Redis not available
        return &RedisTestClient{Client: nil}
    }

    return &RedisTestClient{Client: client}
}
```

**Expected**: `Addr: "localhost:6379"` (port-forward address)

---

## 🛠️ **Resolution Options**

### **Option A: Fix Integration Test Redis Configuration** ⭐ **RECOMMENDED**

**Approach**: Ensure integration tests use `localhost:6379` for Redis

**Changes**:
1. **Verify** `SetupRedisTestClient()` uses `localhost:6379`
2. **Verify** `StartTestGateway()` passes this client to `server.NewServer()`
3. **Add debug logging** to confirm Redis address

**Implementation**:
```go
// test/integration/gateway/helpers.go
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
    client := goredis.NewClient(&goredis.Options{
        Addr:     "localhost:6379",  // Port-forward address
        Password: "",
        DB:       0,
    })

    // Add debug logging
    fmt.Printf("🔍 Redis client connecting to: %s\n", "localhost:6379")

    _, err := client.Ping(ctx).Result()
    if err != nil {
        fmt.Printf("❌ Redis ping failed: %v\n", err)
        return &RedisTestClient{Client: nil}
    }

    fmt.Printf("✅ Redis ping successful\n")
    return &RedisTestClient{Client: client}
}
```

**Confidence**: **90%** - This is the most likely issue

---

### **Option B: Add Redis Connection Logging to Gateway**

**Approach**: Add logging to Gateway to see what Redis address it's trying to connect to

**Changes**:
```go
// pkg/gateway/processing/deduplication.go
func NewDeduplicationService(redisClient *redis.Client, logger *zap.Logger) *DeduplicationService {
    logger.Info("Deduplication service initialized",
        zap.String("redis_addr", redisClient.Options().Addr))  // ← Add this

    return &DeduplicationService{
        redisClient: redisClient,
        logger:      logger,
    }
}
```

**Confidence**: **80%** - Helps diagnose the issue

---

### **Option C: Verify Port-Forward is Accessible from Test Process**

**Approach**: Test if `localhost:6379` is reachable from Go test process

**Test**:
```bash
# From test directory
go run -c '
package main
import (
    "context"
    "fmt"
    redis "github.com/go-redis/redis/v8"
)
func main() {
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    pong, err := client.Ping(context.Background()).Result()
    fmt.Printf("Ping: %s, Error: %v\n", pong, err)
}
'
```

**Expected**: `Ping: PONG, Error: <nil>`

**Confidence**: **70%** - Rules out network issues

---

## 💡 **Recommended Action Plan**

### **Immediate (Next 15 minutes)**

1. **Read `test/integration/gateway/helpers.go`**:
   - Check `SetupRedisTestClient()` implementation
   - Verify Redis address is `localhost:6379`
   - Check if client is passed to `StartTestGateway()`

2. **Add debug logging**:
   - Log Redis address in `SetupRedisTestClient()`
   - Log Redis ping result
   - Log Gateway Redis client initialization

3. **Re-run ONE test** (not full suite):
   ```bash
   go test -v ./test/integration/gateway -run "TestGatewayIntegration/should_create_RemediationRequest_CRD_successfully" -timeout 5m
   ```

4. **Analyze logs**:
   - Check if Redis client connects to `localhost:6379`
   - Check if Redis ping succeeds
   - Check if Gateway receives Redis client

---

### **If Redis Address is Wrong**

**Fix**:
```go
// test/integration/gateway/helpers.go
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
    // CHANGE FROM:
    // Addr: "redis-gateway-ha.kubernaut-system:6379",

    // TO:
    Addr: "localhost:6379",  // Port-forward address
}
```

**Re-run tests**: Should see 201/202 responses instead of 503

---

### **If Redis Address is Correct**

**Next Steps**:
1. Check if port-forward is accessible from test process (Option C)
2. Check if Redis client is nil in Gateway (Option B)
3. Check if there's a firewall/network issue blocking `localhost:6379`

---

## 📊 **Test Failure Breakdown**

| Test Category | Expected | Actual | Status |
|---------------|----------|--------|--------|
| **Storm Aggregation** | 201/202 | 503 | ❌ Redis unavailable |
| **Redis Integration** | 201/202 | 503 | ❌ Redis unavailable |
| **K8s API Integration** | 201 | 503 | ❌ Redis unavailable |
| **Security Integration** | 201/202 | 503 | ❌ Redis unavailable |
| **Error Handling** | Various | 503 | ❌ Redis unavailable |

**Pattern**: **ALL tests fail with 503** → Gateway cannot connect to Redis

---

## 🔗 **Related Files**

- **Test Helper**: `test/integration/gateway/helpers.go` - `SetupRedisTestClient()`, `StartTestGateway()`
- **Gateway Server**: `pkg/gateway/server/server.go` - `NewServer()`
- **Deduplication**: `pkg/gateway/processing/deduplication.go` - Redis client usage
- **Storm Detection**: `pkg/gateway/processing/storm_detection.go` - Redis client usage
- **Rate Limiting**: `pkg/gateway/middleware/ratelimit.go` - Redis client usage

---

## ✅ **Next Steps**

1. ✅ **Immediate**: Read `helpers.go` to verify Redis address
2. ⏳ **Short-term**: Add debug logging to confirm Redis connectivity
3. ⏳ **Short-term**: Run single test with verbose logging
4. ⏳ **Medium-term**: Fix Redis address if wrong
5. ⏳ **Medium-term**: Re-run full test suite

**Confidence**: **90%** - Redis address misconfiguration is the most likely root cause


