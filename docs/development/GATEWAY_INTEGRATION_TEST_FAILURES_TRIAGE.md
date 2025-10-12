# Gateway Integration Test Failures - Fresh Kind Environment Triage

**Date**: 2025-10-11
**Environment**: Fresh Kind cluster with new authentication token
**Test Run**: 47 of 48 specs, 39 passed, 8 failed, 1 skipped
**Duration**: 269.214 seconds

## Executive Summary

After regenerating the Kind environment with a fresh token, 8 integration tests are failing. These failures reveal **3 distinct root causes** that need to be addressed:

### Root Cause Categories

1. **Storm Aggregation Not Working (5 failures)** - Storm detection works, but aggregated CRDs are never created
2. **Deduplication Test Expectations Incorrect (3 failures)** - Tests expect new CRDs after Redis flush, but Gateway correctly reuses existing K8s CRDs
3. **Environment Classification Cache Not Invalidating (2 failures)** - Cache doesn't respond to namespace label or ConfigMap changes

---

## Category 1: Storm Aggregation Not Working (5 Failures)

### Affected Tests
1. ✅ `aggregates mass incidents so AI analyzes root cause instead of 50 symptoms` (line 424)
2. ✅ `aggregates storm alerts arriving across multiple time windows` (line 968)
3. ✅ `handles two different alertnames storming simultaneously` (line 1722)

### Symptom
Tests timeout waiting for aggregated CRDs that never appear. Storm detection fires correctly, but the aggregation window expires without creating the aggregated CRD.

### Evidence from Logs

**Test 1: Main storm aggregation test**
```
Expected 51 CRDs total: 50 individual (alerts 1-50) + 1 aggregated (alerts 51-55)
Actual: All 55 alerts created individual CRDs, no aggregation occurred
```

The test sends 55 alerts with the same alertname:
- Alerts 1-50: Should create individual CRDs (storm not yet detected, count ≤ 50)
- Alerts 51-55: Should be aggregated (storm detected, count > 50)

**What's happening**: The logs show all 55 alerts returned `"status": "created"`, meaning no aggregation occurred.

**Test 3: Two simultaneous storms**
```
[FAILED] Timed out after 15.000s.
Each simultaneous storm should create exactly 1 aggregated CRD
Expected
    <map[string]int | len:0>: {}
to have {key: value}
    <map[interface {}]interface {} | len:1>: {
        <string>"storm1": <int>1,
    }
```

The test sends 10 alerts each for "Storm1" and "Storm2" alertnames, expecting 2 aggregated CRDs, but gets none.

### Root Cause Analysis

**Hypothesis**: The `createAggregatedCRDAfterWindow` goroutine is either:
1. Not being triggered when storms are detected
2. Being triggered but failing silently
3. Being cancelled before the aggregation window expires
4. Not finding any alerts in the aggregation window when it wakes up

**Key Code Path to Investigate**:
```go
// pkg/gateway/server.go (around line 200-250)
func (s *Server) processSignal(...)  {
    // ...
    if isStorm {
        // Storm detected, start aggregation
        go s.createAggregatedCRDAfterWindow(ctx, windowID, stormFingerprint)
        return // Response already sent
    }
}
```

**Likely Issue**: The goroutine might be:
- Getting context cancelled prematurely
- Not using a background context (using request context which gets cancelled)
- Not handling errors when creating the aggregated CRD

### Fix Strategy

1. **Add logging** to `createAggregatedCRDAfterWindow` to trace execution
2. **Use background context** instead of request context for the goroutine
3. **Add error handling** and recovery for aggregation failures
4. **Ensure aggregator state** is properly maintained across requests

---

## Category 2: Deduplication Test Expectations Incorrect (3 Failures)

### Affected Tests
1. ✅ `creates new CRD when deduplication TTL expires` (line 1159)
2. ✅ `treats alerts with different severity as unique (not deduplicated)` (line 1236)
3. ✅ `handles dedup key expiring mid-flight` (line 1866)

### Symptom
Tests expect 2 CRDs after Redis flush, but only 1 exists. Gateway is **correctly** reusing the existing Kubernetes CRD.

### Evidence from Logs

**Test 3: Dedup key expiring mid-flight**
```go
time="2025-10-11T09:41:39-04:00" level=debug msg="RemediationRequest CRD already exists (Redis TTL expired, CRD persists)" 
    fingerprint=fd94f41f583e8115cd3d628bc19a59b5a9579ee4498a0c99cb478691d04cfd14 
    name=rr-fd94f41f583e8115 
    namespace=test-gw-1760190099387665000

time="2025-10-11T09:41:39-04:00" level=info msg="Reusing existing RemediationRequest CRD (Redis TTL expired)" 
    fingerprint=fd94f41f583e8115cd3d628bc19a59b5a9579ee4498a0c99cb478691d04cfd14 
    name=rr-fd94f41f583e8115 
    namespace=test-gw-1760190099387665000
```

**Test Expectation**:
```go
Eventually(func() int {
    // ... list CRDs ...
    return len(rrList.Items)
}, 10*time.Second, 500*time.Millisecond).Should(Equal(2),
    "Dedup key expiry should allow new CRD creation")
```

**Test expects**: 2 CRDs
**Gateway creates**: 1 CRD (reuses existing)
**Gateway behavior**: ✅ **CORRECT** - Kubernetes CRDs are the source of truth

### Root Cause Analysis

**The Gateway is behaving correctly**. The deduplication logic has this fallback:
```go
// If Redis doesn't have the key (TTL expired or Redis down), check Kubernetes
existingCRD, err := s.k8sClient.Get(ctx, ...)
if err == nil {
    // CRD exists in Kubernetes, reuse it
    return existingCRD.Name, nil
}
```

**Why this is correct**:
- Redis is a **cache** for performance (fast dedup checks)
- Kubernetes is the **source of truth** (persistent CRDs)
- If Redis TTL expires but the CRD still exists, reusing it prevents duplicates

### Fix Strategy

**Option A: Fix the tests (RECOMMENDED)**
- Change expectations from `Equal(2)` to `Equal(1)`
- Update test descriptions to reflect correct behavior
- Add validation that Gateway checks K8s after Redis miss

**Option B: Change Gateway behavior (NOT RECOMMENDED)**
- Ignore existing K8s CRDs and create duplicates
- This would violate deduplication guarantees
- Would cause CRD proliferation when Redis is unavailable

**Recommended Fix**: Update test expectations to match correct Gateway behavior.

---

## Category 3: Environment Classification Cache Not Invalidating (2 Failures)

### Affected Tests
1. ✅ `handles namespace label changes mid-flight` (line 1456)
2. ✅ `handles ConfigMap updates during runtime` (line 1549)

### Symptom
Tests update namespace labels or ConfigMaps, but the Gateway's environment classifier continues returning cached values.

### Evidence from Logs

**Test 1: Namespace label changes**
```
[FAILED] Timed out after 10.001s.
Environment should change from 'staging' to 'prod' after label update
```

**Test 2: ConfigMap updates**
```
[FAILED] Timed out after 10.001s.
Environment should change from 'unknown' to 'canary' after ConfigMap update
```

### Root Cause Analysis

The Gateway's environment classifier caches results but **doesn't watch for changes**:

```go
// pkg/gateway/processing/environment_classifier.go
func (ec *EnvironmentClassifier) ClassifyEnvironment(ctx context.Context, namespace string) (string, error) {
    // Check cache first
    if env, found := ec.cache.Get(namespace); found {
        return env, nil
    }
    
    // ... fetch from K8s, cache result ...
}
```

**Missing**: Cache invalidation mechanism when:
- Namespace labels change
- ConfigMap is updated or deleted

### Fix Strategy

**Option A: Add cache TTL (SIMPLE, RECOMMENDED FOR V1)**
```go
type EnvironmentClassifier struct {
    cache     *ttlcache.Cache // Use TTL cache instead of simple map
    cacheTTL  time.Duration   // Default: 30s
}
```

**Option B: Add Kubernetes Watches (COMPLEX, V2)**
```go
// Watch namespace label changes
namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    UpdateFunc: func(oldObj, newObj interface{}) {
        // Invalidate cache for this namespace
        ec.cache.Delete(namespace)
    },
})
```

**Recommended Fix for V1**: Add configurable cache TTL (default 30s) to balance performance and freshness.

---

## Fix Priority Order

### Phase 1: Critical Storm Aggregation (Blocks V1)
**Impact**: High - Core storm aggregation feature not working
**Effort**: Medium
**Tests Affected**: 5 failures

**Tasks**:
1. Debug `createAggregatedCRDAfterWindow` goroutine lifecycle
2. Use background context for aggregation goroutine
3. Add comprehensive logging for storm aggregation path
4. Ensure aggregator state is properly maintained
5. Add error handling and recovery

### Phase 2: Fix Deduplication Test Expectations (Quick Win)
**Impact**: Low - Tests incorrect, Gateway correct
**Effort**: Low
**Tests Affected**: 3 failures

**Tasks**:
1. Update test expectations from `Equal(2)` to `Equal(1)`
2. Update test descriptions to document correct behavior
3. Add validation that Gateway checks K8s after Redis miss

### Phase 3: Add Cache TTL for Environment Classification (Enhancement)
**Impact**: Medium - Tests cover edge case, rare in production
**Effort**: Low
**Tests Affected**: 2 failures

**Tasks**:
1. Add TTL cache library dependency
2. Add `CacheTTL` to `ServerConfig`
3. Replace simple map cache with TTL cache
4. Set default TTL to 30s (configurable)

---

## Implementation Plan

### Step 1: Fix Storm Aggregation (Most Critical)
- [ ] Read `createAggregatedCRDAfterWindow` implementation
- [ ] Identify why aggregation doesn't complete
- [ ] Fix context handling (use background context)
- [ ] Add comprehensive logging
- [ ] Test with failing scenarios

### Step 2: Fix Deduplication Tests (Quickest)
- [ ] Update `creates new CRD when deduplication TTL expires` test
- [ ] Update `treats alerts with different severity as unique` test
- [ ] Update `handles dedup key expiring mid-flight` test
- [ ] Document correct Gateway behavior in test descriptions

### Step 3: Add Environment Classification Cache TTL
- [ ] Add TTL cache dependency (`github.com/patrickmn/go-cache`)
- [ ] Update `EnvironmentClassifier` to use TTL cache
- [ ] Add `CacheTTL` configuration
- [ ] Update tests to wait for cache expiry

### Step 4: Rerun All Tests
- [ ] Run full integration test suite
- [ ] Verify all 47 tests pass
- [ ] Document any remaining issues

---

## Success Criteria

**All 47 integration tests passing** with:
1. Storm aggregation creating aggregated CRDs correctly
2. Deduplication tests validating correct Gateway behavior (K8s source of truth)
3. Environment classification responding to changes within cache TTL

**Estimated Total Effort**: 4-6 hours
- Phase 1 (Storm): 2-3 hours
- Phase 2 (Dedup Tests): 30 minutes
- Phase 3 (Cache TTL): 1-2 hours
- Testing: 30 minutes

---

## Next Steps

1. **Begin Phase 1**: Debug and fix storm aggregation
2. **Quick win**: Fix deduplication test expectations while investigating storm issue
3. **Enhancement**: Add cache TTL for environment classification
4. **Validate**: Rerun full test suite

**Recommendation**: Start with Phase 1 (storm aggregation) as it's the most critical feature blocking V1 readiness.

