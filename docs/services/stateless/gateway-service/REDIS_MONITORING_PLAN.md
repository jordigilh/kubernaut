# 🔍 Redis Memory Monitoring Plan

**Date**: 2025-10-26
**Status**: ✅ **Redis Working - Monitoring for OOM**
**Decision**: Continue with current setup, monitor for issues

---

## 📊 **Current Redis Status**

| Metric | Value | Status |
|--------|-------|--------|
| **Max Memory** | 512MB | ✅ Good |
| **Used Memory** | 1.38MB | ✅ Excellent (0.27% used) |
| **Peak Memory** | 4.06MB | ✅ Good (0.79% of max) |
| **Fragmentation Ratio** | 7.14 | 🟡 High (but not critical) |
| **RSS Memory** | 9.60MB | ✅ Good |
| **Keys in DB** | 0 | ✅ Clean |

---

## ✅ **Current Cleanup Strategy**

### **BeforeSuite Cleanup**
**File**: `test/integration/gateway/suite_test.go`
```go
redisClient.Cleanup(ctx)  // Calls FlushDB()
```

### **BeforeEach Cleanup** (Most Tests)
**Pattern**: Added to 9+ test files
```go
err := redisClient.Client.FlushDB(ctx).Err()
Expect(err).ToNot(HaveOccurred(), "Should clean Redis before test")
```

### **Individual Test Cleanup**
**Pattern**: Each test cleans up its own keys
```go
err = client.Del(ctx, "test:key").Err()
```

---

## 🎯 **Monitoring Strategy**

### **What to Watch For**

1. **OOM Errors in Test Output**
   - Pattern: `OOM command not allowed when used memory > 'maxmemory'`
   - Action: Investigate immediately

2. **Slow Test Performance**
   - Pattern: Tests taking longer than usual
   - Possible Cause: Redis memory pressure

3. **Test Flakiness**
   - Pattern: Intermittent failures
   - Possible Cause: Memory fragmentation

---

## 🚨 **If OOM Occurs Again**

### **Immediate Actions**

1. **Check Redis Memory**
   ```bash
   podman exec redis-gateway-test redis-cli INFO memory | grep -E "used_memory_human|maxmemory_human"
   ```

2. **Check Key Count**
   ```bash
   podman exec redis-gateway-test redis-cli DBSIZE
   podman exec redis-gateway-test redis-cli INFO keyspace
   ```

3. **Flush All Databases**
   ```bash
   podman exec redis-gateway-test redis-cli FLUSHALL
   ```

4. **Restart Redis**
   ```bash
   ./test/integration/gateway/stop-redis.sh
   ./test/integration/gateway/start-redis.sh
   ```

---

### **Root Cause Analysis**

If OOM occurs, investigate:

1. **Test Not Cleaning Up**
   - Check which test was running
   - Verify it has `FlushDB()` in BeforeEach
   - Add cleanup if missing

2. **Memory Leak**
   - Check if keys are accumulating
   - Use `redis-cli KEYS *` to see all keys
   - Identify pattern of leaked keys

3. **Large Values**
   - Check if tests are storing large objects
   - Use `redis-cli MEMORY USAGE <key>` to check key size
   - Consider reducing test data size

4. **Wrong Database**
   - Verify tests are using DB 2
   - Check if other databases have keys
   - Use `INFO keyspace` to see all databases

---

## 🔧 **Quick Fixes**

### **Fix 1: Add FLUSHALL to BeforeSuite** (5 min)

**File**: `test/integration/gateway/suite_test.go`

**Change**:
```go
// OLD:
redisClient.Cleanup(ctx)  // Only flushes DB 2

// NEW:
err := redisClient.Client.FlushAll(ctx).Err()
Expect(err).ToNot(HaveOccurred(), "Should clean all Redis databases")
```

**Impact**: Cleans ALL databases, not just DB 2

---

### **Fix 2: Add Memory Logging** (10 min)

**File**: `test/integration/gateway/suite_test.go`

**Add to BeforeSuite**:
```go
// Log Redis memory status
info, err := redisClient.Client.Info(ctx, "memory").Result()
if err == nil {
    GinkgoWriter.Println("📊 Redis Memory Status:")
    // Parse and log key metrics
    for _, line := range strings.Split(info, "\n") {
        if strings.Contains(line, "used_memory_human") ||
           strings.Contains(line, "maxmemory_human") {
            GinkgoWriter.Printf("  %s\n", line)
        }
    }
}
```

**Impact**: Visibility into memory usage before tests

---

### **Fix 3: Increase Redis Memory** (2 min)

**File**: `test/integration/gateway/start-redis.sh`

**Change**:
```bash
# OLD:
--maxmemory 512mb

# NEW:
--maxmemory 1gb
```

**Impact**: More headroom for tests

---

## 📋 **Decision Log**

### **2025-10-26: Continue with Current Setup**

**Decision**: Monitor for OOM, don't implement fixes preemptively

**Rationale**:
- ✅ Current setup is working (all tests passing)
- ✅ 512MB is plenty (only 1.38MB used, peak 4.06MB)
- ✅ Cleanup is in place (BeforeSuite + BeforeEach)
- ✅ No current OOM issues

**Risk**: LOW - Memory usage is minimal

**Mitigation**: If OOM occurs, implement Fix 1 (FLUSHALL)

---

## 🎯 **Success Criteria**

**Redis is healthy if**:
- ✅ No OOM errors in test output
- ✅ Tests complete in reasonable time (<5 min)
- ✅ No test flakiness
- ✅ Memory usage stays below 50MB

**Trigger for Action**:
- ❌ Any OOM error
- ❌ Memory usage >100MB
- ❌ Test flakiness increases
- ❌ Fragmentation ratio >10

---

## 📊 **Monitoring Commands**

### **Quick Memory Check**
```bash
podman exec redis-gateway-test redis-cli INFO memory | grep -E "used_memory_human|maxmemory_human|mem_fragmentation_ratio"
```

### **Key Count Check**
```bash
podman exec redis-gateway-test redis-cli DBSIZE
```

### **All Databases Check**
```bash
podman exec redis-gateway-test redis-cli INFO keyspace
```

### **Memory by Key**
```bash
podman exec redis-gateway-test redis-cli --scan --pattern '*' | while read key; do
    echo "$key: $(redis-cli MEMORY USAGE $key)"
done
```

---

## 🎯 **Next Steps**

1. ✅ **Continue with Day 9 Phase 2** (Metrics + Observability)
2. 🔍 **Monitor for OOM** during test runs
3. 📊 **Check memory** periodically
4. 🚨 **Implement Fix 1** if OOM occurs

---

**Status**: ✅ **MONITORING ACTIVE**
**Confidence**: 95% that current setup is sufficient
**Action**: Continue development, watch for OOM


