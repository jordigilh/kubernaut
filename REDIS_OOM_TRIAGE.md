# Redis OOM Triage Report

**Date**: October 29, 2025
**Issue**: Redis `maxmemory` intermittently shows 1MB instead of 2GB during test runs

---

## 🔍 **Investigation Summary**

### **Current State**
- **Container**: `redis-gateway` (running 12+ hours)
- **Start Command**: Correctly configured with `--maxmemory 2147483648`
- **Current Config**: `maxmemory 1048576` (1MB) ❌ **WRONG!**
- **Port**: 6379 (localhost)

### **Root Cause Analysis**

**The issue is NOT**:
- ❌ Container start command (verified correct: `--maxmemory 2147483648`)
- ❌ Podman restart behavior (verified: `podman restart` DOES preserve args)
- ❌ Multiple Redis instances (only one active: `redis-gateway`)
- ❌ Wrong Redis connection (tests correctly use `localhost:6379`)

**The ACTUAL issue**:
✅ **Test helper function `SimulateMemoryPressure()` sets Redis to 1MB and never resets it**

### **Root Cause: Missing Test Cleanup**

**Location**: `test/integration/gateway/helpers.go:505`

```go
func (r *RedisTestClient) SimulateMemoryPressure(ctx context.Context) {
    r.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
    r.Client.ConfigSet(ctx, "maxmemory", "1mb") // Force memory pressure ⚠️
}
```

**The Problem**:
1. At some point (past or present), a test called `SimulateMemoryPressure()`
2. This set Redis `maxmemory` to 1MB via `CONFIG SET`
3. The test finished, but **never reset Redis back to 2GB**
4. Redis has been running with 1MB ever since (persists across test runs)
5. All subsequent test runs hit OOM errors

**Evidence**:
```bash
# Container start command (correct):
$ podman inspect redis-gateway | grep maxmemory
"--maxmemory", "2147483648"

# Runtime config (WRONG - overridden by test):
$ podman exec redis-gateway redis-cli CONFIG GET maxmemory
maxmemory
1048576

# Verification: podman restart DOES preserve args:
$ podman restart redis-gateway && sleep 2
$ podman exec redis-gateway redis-cli CONFIG GET maxmemory
maxmemory
2147483648  # ✅ Correct after restart!
```

**Conclusion**: The issue is NOT Podman behavior. It's **missing test cleanup** in `AfterEach` blocks.

---

## 🔧 **Root Cause: Missing Test Cleanup**

### **The Problem**

1. **Test helper exists**: `SimulateMemoryPressure()` in `helpers.go:505`
2. **No cleanup code**: No `AfterEach` resets Redis maxmemory to 2GB
3. **Persistent state**: `CONFIG SET` changes persist across test runs
4. **Cascade failures**: One test's memory pressure simulation breaks all subsequent tests

---

## ✅ **Solutions**

### **Option A: Add Test Cleanup** (RECOMMENDED - IMMEDIATE FIX)

**Pros**:
- ✅ Fixes root cause (missing cleanup)
- ✅ No infrastructure changes needed
- ✅ Follows TDD best practices

**Implementation**:

1. **Add cleanup helper** to `helpers.go`:
```go
// ResetRedisConfig resets Redis to test-safe configuration
// Call this in AfterEach to prevent state pollution
func (r *RedisTestClient) ResetRedisConfig(ctx context.Context) {
    if r.Client == nil {
        return
    }
    // Reset to 2GB (matches container start command)
    r.Client.ConfigSet(ctx, "maxmemory", "2147483648")
    r.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
}
```

2. **Add to AfterEach** in all test files:
```go
AfterEach(func() {
    if redisClient != nil {
        redisClient.ResetRedisConfig(ctx)
        redisClient.Client.FlushDB(ctx)
    }
    // ... other cleanup ...
})
```

3. **Update `SimulateMemoryPressure()`** to document cleanup requirement:
```go
// SimulateMemoryPressure simulates Redis memory pressure
// ⚠️ IMPORTANT: Call ResetRedisConfig() in AfterEach to restore 2GB limit
func (r *RedisTestClient) SimulateMemoryPressure(ctx context.Context) {
    r.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
    r.Client.ConfigSet(ctx, "maxmemory", "1mb")
}
```

---

### **Option B: Use Redis Config File** (DEFENSE-IN-DEPTH)

**Pros**:
- ✅ Persistent across restarts
- ✅ Standard Redis practice
- ✅ Easy to version control

**Implementation**:

1. Create `redis-test.conf`:
```conf
maxmemory 2147483648
maxmemory-policy allkeys-lru
```

2. Update container start:
```bash
podman run -d \
  --name redis-gateway \
  -p 6379:6379 \
  -v $(pwd)/redis-test.conf:/usr/local/etc/redis/redis.conf:ro \
  redis:7-alpine \
  redis-server /usr/local/etc/redis/redis.conf
```

3. Update `scripts/start-redis-for-tests.sh` to use config file

---

### **Option B: Never Use `podman restart`** (WORKAROUND)

**Pros**:
- ✅ No code changes needed
- ✅ Works with current setup

**Cons**:
- ❌ Easy to forget
- ❌ Doesn't fix system restarts

**Implementation**:
- Always use `podman stop` + `podman rm` + `podman run` (full recreate)
- Update scripts to use `start-redis-for-tests.sh` instead of restart

---

### **Option C: Use `CONFIG REWRITE`** (PARTIAL FIX)

**Pros**:
- ✅ Quick fix for current session

**Cons**:
- ❌ Requires config file to exist
- ❌ Doesn't work with Alpine image (no default config)

**Current Status**: Not viable (Redis running without config file)

---

## 📋 **Recommended Action Plan**

### **Immediate (Today)** ✅ COMPLETED
1. ✅ **Document the issue** (this file)
2. ✅ **Identify root cause** (`SimulateMemoryPressure()` + missing cleanup)
3. ✅ **Reset Redis to 2GB** (manual `CONFIG SET`)
4. ⚠️ **Implement Option A** (add test cleanup)

### **Short-Term (This Session)**
1. **Add `ResetRedisConfig()` helper** to `helpers.go`
2. **Update all test files** to call `ResetRedisConfig()` in `AfterEach`
3. **Document cleanup requirement** in `SimulateMemoryPressure()`
4. **Run full test suite** to verify fix

### **Long-Term (Defense-in-Depth)**
1. **Implement Option B** (Redis config file)
   - Create `scripts/redis-test.conf`
   - Update `scripts/start-redis-for-tests.sh`
   - Test with container restart
   - Document in README

2. **Add Redis health check** to test setup
   - Verify maxmemory before running tests
   - Auto-fix if wrong
   - Fail fast with clear error message

---

## 🎯 **Impact Assessment**

### **Test Failures Caused by Redis OOM**
- **Estimated**: 9-15 tests (based on 37 passed with correct Redis, 26-28 with OOM)
- **Symptoms**:
  - `OOM command not allowed when used memory > 'maxmemory'`
  - Fingerprint not stored in Redis
  - Deduplication failures

### **Workaround Effectiveness**
- **Manual `CONFIG SET`**: ✅ Works until next restart
- **Current Success Rate**: ~67% (37/55 tests) with correct config
- **With OOM**: ~47% (26/55 tests)

---

## 📊 **Verification Steps**

### **Check Current State**
```bash
# Check maxmemory
podman exec redis-gateway redis-cli CONFIG GET maxmemory

# Should show: 2147483648 (2GB)
# If shows: 1048576 (1MB) → Redis was restarted without args
```

### **Fix Immediately**
```bash
# Set memory
podman exec redis-gateway redis-cli CONFIG SET maxmemory 2147483648

# Flush DB (clean state)
podman exec redis-gateway redis-cli FLUSHDB

# Verify
podman exec redis-gateway redis-cli CONFIG GET maxmemory
```

### **Permanent Fix** (Option A)
```bash
# Stop current container
podman stop redis-gateway
podman rm redis-gateway

# Create config file
cat > /tmp/redis-test.conf <<EOF
maxmemory 2147483648
maxmemory-policy allkeys-lru
EOF

# Start with config file
podman run -d \
  --name redis-gateway \
  -p 6379:6379 \
  -v /tmp/redis-test.conf:/usr/local/etc/redis/redis.conf:ro \
  redis:7-alpine \
  redis-server /usr/local/etc/redis/redis.conf

# Test restart persistence
podman restart redis-gateway
podman exec redis-gateway redis-cli CONFIG GET maxmemory
# Should still show: 2147483648
```

---

## 🚨 **Critical Finding**

**The Redis OOM issue is NOT an infrastructure problem.**

**It's a test cleanup issue**: `SimulateMemoryPressure()` sets Redis to 1MB and tests never reset it back to 2GB.

**Root Cause**: Missing `ResetRedisConfig()` calls in `AfterEach` blocks.

**Solution**: Add test cleanup helper and call it in all test `AfterEach` blocks (Option A).

---

## ✅ **Decision for User**

**Question**: Should we implement Option A (test cleanup) now, or defer to later?

**Recommendation**: **Implement Option A NOW**
- ✅ Fixes root cause immediately
- ✅ No infrastructure changes needed
- ✅ Prevents future OOM cascade failures
- ✅ Takes ~10 minutes to implement

**Additional**: Consider Option B (Redis config file) as defense-in-depth later

---

## 📊 **Summary**

**What we learned**:
1. ✅ Podman `restart` DOES preserve `--maxmemory` args (not the issue)
2. ✅ Container start command is correct (not the issue)
3. ✅ `SimulateMemoryPressure()` test helper sets Redis to 1MB
4. ✅ Tests never reset Redis back to 2GB (root cause)
5. ✅ `CONFIG SET` changes persist across test runs (cascade failures)

**What we fixed**:
1. ✅ Manually reset Redis to 2GB (`CONFIG SET maxmemory 2147483648`)
2. ✅ Flushed Redis data (`FLUSHDB`)
3. ✅ Documented root cause in this file

**What we need to do**:
1. ⚠️ Add `ResetRedisConfig()` helper to `helpers.go`
2. ⚠️ Update all test files to call it in `AfterEach`
3. ⚠️ Run full test suite to verify fix

---

**End of Triage Report**

