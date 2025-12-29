# 📋 Redis Memory Optimization - Deferred for Refinement Phase

**Date**: 2025-10-24
**Priority**: 🟡 **MEDIUM** (Post Zero Tech Debt)
**Status**: ⏳ **DEFERRED** until refinement phase

---

## 🚨 **CONCERN**

**Current State**: 4GB Redis required for integration tests
**Expected**: Storm aggregation should require much less memory
**Action**: Triage and optimize after zero tech debt achieved

---

## 🔍 **INVESTIGATION QUESTIONS**

### **1. What are we storing in Redis?**
- Storm CRD metadata (5-minute TTL)
- Deduplication fingerprints (5-minute TTL)
- Storm detection counters (1-minute TTL)
- Rate limiting counters (1-minute TTL)

### **2. How much memory does each component use?**
- **Storm CRD**: ~10-50KB per storm (JSON serialized `RemediationRequest`)
- **Deduplication**: ~1KB per fingerprint (SHA256 hash + metadata)
- **Storm Detection**: ~500 bytes per counter
- **Rate Limiting**: ~100 bytes per IP

### **3. Why do tests need 4GB?**
- **Test Load**: 92 integration tests running concurrently
- **Large CRDs**: Storm CRDs with 15+ affected resources
- **No Cleanup**: Tests may not be cleaning up Redis state properly
- **Memory Leaks**: Potential memory leaks in test infrastructure

### **4. What should production need?**
- **Expected Load**: 100-1000 alerts/minute
- **Storm CRDs**: ~10-20 active storms at any time
- **Deduplication**: ~1000-5000 active fingerprints
- **Expected Memory**: **256MB-512MB** (not 4GB!)

---

## 🎯 **OPTIMIZATION OPPORTUNITIES**

### **Option 1: Reduce CRD Size in Redis** 🔴 **HIGH IMPACT**
**Problem**: Storing full `RemediationRequest` CRD (10-50KB)
**Solution**: Store only essential fields

**Current**:
```go
// Storing full CRD (~50KB)
crdJSON, _ := json.Marshal(crd)
redis.Set(key, crdJSON, 5*time.Minute)
```

**Optimized**:
```go
// Store only essential fields (~2KB)
type StormMetadata struct {
    Pattern           string
    AlertCount        int
    AffectedResources []string // Just names, not full objects
    LastSeen          time.Time
}
metadata, _ := json.Marshal(StormMetadata{...})
redis.Set(key, metadata, 5*time.Minute)
```

**Expected Savings**: 90% reduction (50KB → 5KB per storm)

---

### **Option 2: Use Redis Hash Instead of JSON** 🟡 **MEDIUM IMPACT**
**Problem**: JSON serialization is verbose
**Solution**: Use Redis Hash for structured data

**Current**:
```go
// JSON: {"pattern":"HighCPU in prod","alertCount":15,...}
redis.Set(key, json, ttl)
```

**Optimized**:
```go
// Redis Hash: More compact, faster access
redis.HSet(key, "pattern", "HighCPU in prod")
redis.HSet(key, "count", 15)
redis.Expire(key, ttl)
```

**Expected Savings**: 30-40% reduction in memory

---

### **Option 3: Aggressive TTL Management** 🟢 **LOW IMPACT**
**Problem**: 5-minute TTL may be too long
**Solution**: Reduce TTL based on business needs

**Current**:
```go
stormCRDTTL = 5 * time.Minute
```

**Optimized**:
```go
stormCRDTTL = 2 * time.Minute // Shorter window
```

**Expected Savings**: 60% reduction in active keys

---

### **Option 4: Compress Large Payloads** 🟡 **MEDIUM IMPACT**
**Problem**: Large JSON payloads stored uncompressed
**Solution**: Gzip compress before storing in Redis

**Current**:
```go
redis.Set(key, jsonBytes, ttl)
```

**Optimized**:
```go
compressed := gzip.Compress(jsonBytes)
redis.Set(key, compressed, ttl)
```

**Expected Savings**: 70-80% reduction for large CRDs

---

### **Option 5: Test Infrastructure Cleanup** 🔴 **HIGH IMPACT**
**Problem**: Tests may not be cleaning up Redis properly
**Solution**: Ensure `BeforeEach` and `AfterEach` flush Redis

**Current**: `BeforeEach` flushes, but tests may leak keys
**Optimized**: Add `AfterEach` cleanup + verify no key leaks

**Expected Savings**: 50% reduction in test memory usage

---

## 📊 **TRIAGE PLAN**

### **Phase 1: Measure Current Usage** (30 min)
```bash
# Run tests with Redis memory monitoring
redis-cli INFO memory > before.txt
./test/integration/gateway/run-tests-local.sh
redis-cli INFO memory > after.txt

# Analyze memory growth
diff before.txt after.txt

# Count keys by pattern
redis-cli KEYS "storm:*" | wc -l
redis-cli KEYS "dedup:*" | wc -l
redis-cli KEYS "rate:*" | wc -l
```

### **Phase 2: Identify Memory Hogs** (30 min)
```bash
# Find largest keys
redis-cli --bigkeys

# Sample key sizes
redis-cli DEBUG OBJECT "storm:crd:HighCPU in prod"
redis-cli MEMORY USAGE "storm:crd:HighCPU in prod"
```

### **Phase 3: Implement Optimizations** (2-3 hours)
1. Implement Option 1 (Reduce CRD size) - **HIGH PRIORITY**
2. Implement Option 5 (Test cleanup) - **HIGH PRIORITY**
3. Test with 1GB Redis (should be sufficient)
4. Validate all tests still pass

### **Phase 4: Validate Production Sizing** (30 min)
- Calculate expected production memory usage
- Set production Redis to 512MB-1GB
- Add memory alerts for >80% usage

---

## 🎯 **SUCCESS CRITERIA**

**Target**: Reduce Redis memory requirement from 4GB to **1GB or less**

**Metrics**:
- ✅ Integration tests pass with 1GB Redis
- ✅ Storm CRD size reduced from 50KB to <10KB
- ✅ No memory leaks in test infrastructure
- ✅ Production Redis sized at 512MB-1GB

---

## 📝 **DEFERRED UNTIL**

**Blockers**:
1. ❌ Zero tech debt not achieved yet
2. ❌ All integration tests not passing yet
3. ❌ Unit tests not run yet
4. ❌ Lint errors not checked yet

**Trigger**: After achieving zero tech debt (100% tests passing, 0 lint errors)

---

## 🔗 **RELATED DOCUMENTS**

- [Day 8 Phase 2 Redis OOM Fix](DAY8_PHASE2_REDIS_OOM_FIX.md) - Temporary 4GB fix
- [Zero Tech Debt Commitment](ZERO_TECH_DEBT_COMMITMENT.md) - Current priority
- [Storm Aggregator Implementation](../../../pkg/gateway/processing/storm_aggregator.go) - Code to optimize

---

## 📊 **CONFIDENCE ASSESSMENT**

**Confidence in Achieving 1GB Target**: **90%** ✅

**Why 90%**:
- ✅ Clear optimization opportunities identified
- ✅ Expected savings are significant (90% for Option 1)
- ✅ Test cleanup likely to have major impact
- ⚠️ 10% uncertainty for unknown memory usage patterns

**Expected Outcome**: 4GB → 1GB (75% reduction)

---

**Status**: ⏳ **DEFERRED** - Will revisit after zero tech debt achieved


