# Gateway Concurrent Deduplication Race Condition - Analysis & Solutions

## 📋 **Problem Statement**

**Test**: `GW-DEDUP-002: Concurrent Deduplication Races` (test 35)
**Failure**: Expected 1 `RemediationRequest`, got 5 (one per concurrent request)
**Root Cause**: Kubernetes cached client race condition in high-concurrency scenarios

---

## 🔍 **Technical Root Cause Analysis**

### **Current Deduplication Architecture** (DD-GATEWAY-011)

```
Gateway uses K8s CRD-based deduplication (not Redis):
1. Calculate fingerprint from signal (alertname:namespace:kind:name)
2. Query K8s API for existing RemediationRequest with that fingerprint
3. If exists → return "duplicate"
4. If not exists → create new RemediationRequest
```

### **The Race Condition**

```
Timeline (5 concurrent requests, same fingerprint):

T=0ms:  Request 1 → Check cache for RR (fingerprint=ABC) → NOT FOUND
T=1ms:  Request 2 → Check cache for RR (fingerprint=ABC) → NOT FOUND
T=2ms:  Request 3 → Check cache for RR (fingerprint=ABC) → NOT FOUND
T=3ms:  Request 4 → Check cache for RR (fingerprint=ABC) → NOT FOUND
T=4ms:  Request 5 → Check cache for RR (fingerprint=ABC) → NOT FOUND

T=10ms: Request 1 → Creates RR-1 in K8s API
T=11ms: Request 2 → Creates RR-2 in K8s API
T=12ms: Request 3 → Creates RR-3 in K8s API
T=13ms: Request 4 → Creates RR-4 in K8s API
T=14ms: Request 5 → Creates RR-5 in K8s API

T=100ms: Cache syncs → All 5 RRs now visible

PROBLEM: All 5 requests checked cache BEFORE any RR was created
```

### **Why Kubernetes Cached Client?**

The Gateway uses `controller-runtime` client with caching:
- **Performance**: Reads from in-memory cache (fast)
- **API Efficiency**: Reduces load on K8s API server
- **Standard Pattern**: Recommended for controllers

**BUT**: Cache has a sync delay (typically 100-500ms)

---

## 🎯 **Long-Term Solution Options**

### **Option A: Use apiReader (Non-Cached Client) for Deduplication Checks**

**Approach**: Use `apiReader` for `PhaseBasedDeduplicationChecker.ShouldDeduplicate()` reads

#### **Implementation**
```go
// pkg/gateway/processing/phase_checker.go
type PhaseBasedDeduplicationChecker struct {
    client    client.Client  // For writes (cached)
    apiReader client.Reader  // For reads (non-cached, direct API)
}

func (c *PhaseBasedDeduplicationChecker) ShouldDeduplicate(ctx context.Context, fingerprint string) (bool, error) {
    // Use apiReader for immediate consistency
    var rrList remediationv1alpha1.RemediationRequestList
    err := c.apiReader.List(ctx, &rrList,
        client.MatchingFields{"spec.fingerprint": fingerprint},
        client.Limit(1))
    // ...
}
```

#### **Pros** ✅
- **Immediate Consistency**: Reads directly from K8s API (no cache delay)
- **Minimal Code Changes**: Only change `PhaseBasedDeduplicationChecker`
- **Proven Pattern**: Used by many K8s operators for critical consistency

#### **Cons** ⚠️
- **API Load**: More requests to K8s API (but only for deduplication checks)
- **Performance**: ~10-50ms slower per request (API call vs cache read)
- **Previous Attempt Failed**: Earlier attempt caused HTTP 500 errors

#### **Why Previous Attempt Failed**
We need to investigate the HTTP 500 errors from the earlier `apiReader` attempt. Possible causes:
1. Field indexer not configured for `apiReader`
2. Incorrect usage pattern
3. Context cancellation issue

#### **Risk**: Medium (need to diagnose HTTP 500 root cause)
#### **Effort**: Low (if we fix HTTP 500 issue)
#### **Impact**: High (completely fixes race condition)

---

### **Option B: Application-Level Locking (In-Memory Mutex)**

**Approach**: Add in-memory mutex keyed by fingerprint

#### **Implementation**
```go
// pkg/gateway/processing/deduplication_lock.go
type DeduplicationLockManager struct {
    locks sync.Map // fingerprint -> *sync.Mutex
}

func (m *DeduplicationLockManager) WithLock(fingerprint string, fn func() error) error {
    mu, _ := m.locks.LoadOrStore(fingerprint, &sync.Mutex{})
    mutex := mu.(*sync.Mutex)

    mutex.Lock()
    defer mutex.Unlock()

    return fn()
}

// In Gateway.ProcessSignal():
func (s *Server) ProcessSignal(ctx context.Context, signal types.Signal) error {
    fingerprint := types.CalculateFingerprint(signal.AlertName, signal.Resource)

    // Critical section: check + create if needed
    return s.dedupLockManager.WithLock(fingerprint, func() error {
        shouldDedup, err := s.deduplicationChecker.ShouldDeduplicate(ctx, fingerprint)
        if err != nil {
            return err
        }

        if shouldDedup {
            // Emit deduplicated event
            return nil
        }

        // Create RR (no one else can be here for this fingerprint)
        return s.createRemediationRequestCRD(ctx, signal, fingerprint)
    })
}
```

#### **Pros** ✅
- **Eliminates Race**: Serializes deduplication checks per fingerprint
- **Low Latency**: In-memory lock (no network calls)
- **Works with Cached Client**: No need to change K8s client usage

#### **Cons** ⚠️
- **Single-Pod Only**: Won't work across multiple Gateway pods (Deployment with replicas)
- **Memory Overhead**: Needs lock cleanup for old fingerprints
- **Deployment Limitation**: Requires `replicas: 1` in production

#### **Risk**: High (breaks horizontal scaling)
#### **Effort**: Medium (need lock lifecycle management)
#### **Impact**: Medium (fixes single-pod deployments only)

---

### **Option C: Optimistic Locking with K8s CRD Names**

**Approach**: Use CRD name as natural lock (K8s ensures uniqueness)

#### **Implementation**
```go
// Change CRD naming from UUID-based to fingerprint-based
// pkg/gateway/processing/crd_creator.go
func (c *CRDCreator) CreateRemediationRequest(...) error {
    // Use fingerprint as CRD name (K8s ensures no duplicates)
    crdName := fmt.Sprintf("rr-%s", fingerprint[:63]) // K8s name limit

    rr := &remediationv1alpha1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      crdName, // Fingerprint-based (not UUID)
            Namespace: namespace,
        },
        // ...
    }

    err := c.client.Create(ctx, rr)
    if apierrors.IsAlreadyExists(err) {
        // Another concurrent request won the race - this is a duplicate
        return ErrDuplicate
    }
    return err
}
```

#### **Pros** ✅
- **K8s Native**: Uses K8s uniqueness guarantee (no external lock needed)
- **Multi-Pod Safe**: Works across multiple Gateway replicas
- **Low Latency**: Single API call (create with conflict detection)
- **Simple Code**: No mutex, no apiReader complexity

#### **Cons** ⚠️
- **Breaking Change**: Changes CRD naming convention
- **Audit Trail Impact**: Correlation ID format changes (DD-AUDIT-CORRELATION-002)
- **Recovery Scenarios**: Multiple attempts for same signal create multiple CRDs (by design, but confusing)
- **Fingerprint Truncation**: K8s names limited to 63 chars (fingerprint is 64 chars SHA256)

#### **Risk**: Medium (requires careful migration)
#### **Effort**: Medium (change CRD naming, update audit correlation)
#### **Impact**: High (completely fixes race, works multi-pod)

---

### **Option D: Distributed Locking (Redis/etcd)**

**Approach**: Use external distributed lock (Redis SETNX)

#### **Implementation**
```go
// pkg/gateway/locking/redis_lock.go
type RedisLockManager struct {
    redisClient *redis.Client
}

func (m *RedisLockManager) WithDistributedLock(fingerprint string, ttl time.Duration, fn func() error) error {
    lockKey := fmt.Sprintf("dedup:lock:%s", fingerprint)

    // Try to acquire lock (SETNX)
    acquired, err := m.redisClient.SetNX(ctx, lockKey, "locked", ttl).Result()
    if err != nil {
        return err
    }
    if !acquired {
        // Someone else has the lock - wait and retry
        time.Sleep(10ms)
        return ErrRetryLater
    }

    defer m.redisClient.Del(ctx, lockKey) // Release lock

    return fn()
}
```

#### **Pros** ✅
- **Multi-Pod Safe**: Works across multiple Gateway replicas
- **Proven Pattern**: Standard distributed locking
- **Flexible**: Can add timeout, retry logic

#### **Cons** ⚠️
- **New Dependency**: Adds Redis (Gateway was Redis-free per DD-GATEWAY-012)
- **Complexity**: Need Redis deployment, connection management
- **Performance**: Network round-trip for every lock acquire/release
- **Failure Mode**: Redis failure blocks deduplication

#### **Risk**: High (contradicts DD-GATEWAY-012 architecture decision)
#### **Effort**: High (Redis deployment, client integration)
**Impact**: High (completely fixes race, but adds dependency)

---

### **Option E: Accept Race, Detect and Clean Up**

**Approach**: Allow duplicates to be created, clean up asynchronously

#### **Implementation**
```go
// Add background controller that detects duplicate RRs
// pkg/gateway/cleanup/duplicate_detector.go
type DuplicateCleanupController struct {
    client client.Client
}

func (c *DuplicateCleanupController) Reconcile(ctx context.Context) error {
    // Find RRs with same fingerprint
    var rrList remediationv1alpha1.RemediationRequestList
    c.client.List(ctx, &rrList)

    // Group by fingerprint
    grouped := groupByFingerprint(rrList.Items)

    // For each duplicate set, keep oldest, delete others
    for fingerprint, rrs := range grouped {
        if len(rrs) <= 1 {
            continue
        }

        sort.Slice(rrs, func(i, j int) bool {
            return rrs[i].CreationTimestamp.Before(&rrs[j].CreationTimestamp)
        })

        // Delete duplicates (keep first)
        for _, rr := range rrs[1:] {
            c.client.Delete(ctx, &rr)
        }
    }
}
```

#### **Pros** ✅
- **No Code Changes to Gateway**: Separate controller
- **Eventually Consistent**: Duplicates cleaned up after creation
- **Multi-Pod Safe**: Works with multiple replicas

#### **Cons** ⚠️
- **Temporary Duplicates**: Window where duplicates exist
- **Workflow Impact**: Multiple RRs might trigger multiple workflows
- **Complexity**: Need to decide which RR to keep (oldest? status?)
- **Audit Trail**: Cleanup creates audit noise

#### **Risk**: Medium (temporary duplicates may trigger workflows)
#### **Effort**: Medium (need cleanup controller)
#### **Impact**: Medium (fixes race eventually, not immediately)

---

### **Option F: Tune K8s Cache Sync Interval**

**Approach**: Reduce cache sync delay

#### **Implementation**
```go
// cmd/gateway/main.go
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    // Reduce cache sync interval
    SyncPeriod: &metav1.Duration{Duration: 100 * time.Millisecond}, // Default: 10s
})
```

#### **Pros** ✅
- **Minimal Change**: One-line config change
- **No Architecture Changes**: Still uses cached client

#### **Cons** ⚠️
- **Doesn't Eliminate Race**: Only reduces window (100ms → 10ms)
- **Increased API Load**: More frequent cache syncs
- **Still Fails Under High Load**: 10ms is enough for race in test

#### **Risk**: Low (safe change)
#### **Effort**: Low (one config line)
#### **Impact**: Low (reduces race probability, doesn't eliminate)

---

## 📊 **Solution Comparison Matrix**

| Solution | Multi-Pod Safe | Eliminates Race | Effort | Risk | Performance | Arch Impact |
|----------|----------------|-----------------|--------|------|-------------|-------------|
| **A: apiReader** | ✅ Yes | ✅ Yes | Low | Medium | -20ms/req | Low |
| **B: In-Memory Lock** | ❌ No (single-pod) | ✅ Yes | Medium | High | +1ms/req | Low |
| **C: CRD Name Lock** | ✅ Yes | ✅ Yes | Medium | Medium | +5ms/req | Medium |
| **D: Redis Lock** | ✅ Yes | ✅ Yes | High | High | -30ms/req | High |
| **E: Async Cleanup** | ✅ Yes | ⚠️ Eventually | Medium | Medium | No impact | Medium |
| **F: Cache Tuning** | ✅ Yes | ❌ No (reduces) | Low | Low | -5ms/req | None |

---

## 🎯 **Recommended Approach**

### **Primary Recommendation: Option A (apiReader) + Diagnostic Work**

**Rationale**:
1. **Best Balance**: Eliminates race, minimal code change, proven pattern
2. **Investigation Needed**: Diagnose why previous attempt caused HTTP 500
3. **Multi-Pod Safe**: Works in production with multiple replicas
4. **Standard Practice**: Many K8s operators use this pattern

**Implementation Plan**:
1. **Phase 1**: Diagnose HTTP 500 root cause from earlier attempt
   - Check field indexer configuration for `apiReader`
   - Verify context propagation
   - Test with must-gather logs

2. **Phase 2**: Implement with proper error handling
   - Add retry logic for transient API errors
   - Fallback to cached client if apiReader fails
   - Comprehensive logging

3. **Phase 3**: Performance testing
   - Measure latency impact (expected: 10-20ms)
   - Load test with 100+ concurrent requests
   - Verify K8s API load is acceptable

### **Fallback Recommendation: Option C (CRD Name Lock)**

If `apiReader` investigation reveals fundamental blocker:
- Use fingerprint as CRD name (K8s uniqueness guarantee)
- Handle `AlreadyExists` as legitimate duplicate
- Update correlation ID format in audit system

---

## 🔬 **Next Steps for Diagnosis**

### **Step 1: Reproduce HTTP 500 with apiReader**
```go
// Test in development
checker := processing.NewPhaseBasedDeduplicationChecker(apiReader)
// Send 5 concurrent requests
// Capture full error with stack trace
```

### **Step 2: Check Field Indexer Configuration**
```go
// Verify field indexer is configured for both clients
err := mgr.GetFieldIndexer().IndexField(
    context.Background(),
    &remediationv1alpha1.RemediationRequest{},
    "spec.fingerprint",
    func(obj client.Object) []string { ... },
)
```

### **Step 3: Test with Must-Gather**
```bash
# Run test with apiReader
make test-e2e-gateway TEST_FLAGS="-focus 'GW-DEDUP-002'"

# Collect must-gather logs
# Analyze HTTP 500 error details
```

---

## 📝 **Decision Framework**

### **Choose Option A (apiReader) if:**
- ✅ HTTP 500 is fixable (likely)
- ✅ 10-20ms latency acceptable
- ✅ Want proven K8s pattern

### **Choose Option C (CRD Name Lock) if:**
- ✅ apiReader HTTP 500 is fundamental blocker
- ✅ Can accept CRD naming change
- ✅ Want K8s-native solution

### **Choose Option D (Redis Lock) if:**
- ✅ Need absolute consistency
- ✅ Can accept new dependency
- ✅ Already have Redis infrastructure

### **Avoid Options:**
- ❌ **Option B**: Single-pod limitation unacceptable for production
- ❌ **Option F**: Doesn't eliminate race, only reduces

---

## 🎯 **Acceptance Criteria for Solution**

Any chosen solution must:
1. ✅ **Eliminate Race**: Test passes with 100 concurrent requests
2. ✅ **Multi-Pod Safe**: Works with `replicas: 3` in production
3. ✅ **Performance**: <50ms latency increase per request
4. ✅ **No Data Loss**: All signals processed (no dropped requests)
5. ✅ **Audit Compliant**: Maintains correlation ID traceability
6. ✅ **Operationally Simple**: No complex failure modes

---

## 📚 **References**

- **DD-GATEWAY-011**: Gateway Deduplication Architecture
- **DD-AUDIT-CORRELATION-002**: Universal Correlation ID Standard
- **Kubernetes Controller Best Practices**: https://kubernetes.io/docs/concepts/architecture/controller/
- **Controller-Runtime Documentation**: https://pkg.go.dev/sigs.k8s.io/controller-runtime

---

**Status**: Awaiting Decision
**Priority**: P0 (blocks test suite green)
**Created**: 2026-01-18
**Author**: AI Assistant
