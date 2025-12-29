# PROPOSAL: Gateway Redis Deprecation

**Date**: December 7, 2025
**From**: Architecture Team
**To**: Gateway Service Team
**Priority**: 🔴 HIGH (Major Architectural Change)
**Status**: ✅ **APPROVED FOR V1.0**
**Approval Date**: 2025-12-07
**Decision**: Gateway team proposed Async Storm Aggregation pattern, Architecture Team approved

---

## 📋 Executive Summary

This proposal recommends **deprecating Redis as a Gateway dependency** by migrating all Redis-backed functionality to Kubernetes-native alternatives:

| Feature | Current (Redis) | Proposed (K8s/In-Memory) |
|---------|-----------------|--------------------------|
| **Deduplication** | Redis key-value | RR Status + Informer |
| **Storm Aggregation** | Redis state | RR Status + Informer |
| **Rate Limiting** | Redis counters | In-Memory (go-cache) |

**Result**: Gateway becomes a **Redis-free, K8s-native service**.

---

## 🎯 Motivation

### Why Deprecate Redis?

| Issue | Current State | Impact |
|-------|---------------|--------|
| **Infrastructure complexity** | Redis is additional dependency | More ops burden |
| **Split state** | Dedup in Redis, RR in K8s | Two sources of truth |
| **Audit gap** | Storm/dedup data in Redis | Not in RR audit trail |
| **Spec immutability violation** | Gateway updates RR.Spec.Deduplication | K8s anti-pattern |

### What Enables This Change?

**ADR-001: Shared Status Ownership** introduces a pattern where Gateway owns sections of `RemediationRequest.Status`:

```yaml
status:
  # Gateway-owned
  deduplication:
    occurrenceCount: 5
    lastOccurrence: "2025-12-07T10:05:00Z"
  stormAggregation:
    isStorm: true
    aggregatedCount: 15

  # RO-owned
  overallPhase: "Processing"
  # ...
```

This pattern eliminates the need for Redis as the deduplication/storm state store.

---

## 🔄 Proposed Changes

### 1. Deduplication: Redis → RR Status

**Current Flow:**
```
Signal → Check Redis (fingerprint exists?) → Create RR or increment Redis counter
```

**Proposed Flow:**
```
Signal → Check Informer Cache (RR exists?) → Create RR or update RR.Status.Deduplication
```

**Code Change:**
```go
// Before (Redis)
exists, _ := redis.Get("dedup:" + fingerprint)
if exists {
    redis.Incr("dedup:" + fingerprint)
    return DuplicateResponse()
}
redis.Set("dedup:"+fingerprint, rrName, TTL)
return createRR()

// After (K8s Informer)
rrList := &RemediationRequestList{}
client.List(ctx, rrList, client.MatchingLabels{"kubernaut.ai/fingerprint": fingerprint})

for _, rr := range rrList.Items {
    if !isTerminalPhase(rr.Status.OverallPhase) {
        // Update existing RR's deduplication status
        rr.Status.Deduplication.OccurrenceCount++
        rr.Status.Deduplication.LastOccurrence = metav1.Now()
        client.Status().Update(ctx, rr)
        return DuplicateResponse()
    }
}
return createRR()
```

---

### 2. Storm Aggregation: Redis → RR Status

**Current Flow:**
```
Signals → Aggregate in Redis → Storm window closes → Create single RR
```

**Proposed Flow:**
```
First Signal → Create RR immediately
Subsequent Signals → Update RR.Status.StormAggregation
```

**Benefits:**
- RR created immediately (lower latency)
- Storm data visible in RR (auditable)
- No storm window delay

**Code Change:**
```go
// Before (Redis)
stormKey := "storm:" + fingerprint
count, _ := redis.Incr(stormKey)
if count == 1 {
    redis.Expire(stormKey, stormWindow)
}
if count >= stormThreshold {
    // Wait for window to close, then create RR
}

// After (K8s Status)
// RR already exists (created on first signal)
if rr.Status.Deduplication.OccurrenceCount >= stormThreshold {
    if rr.Status.StormAggregation == nil {
        rr.Status.StormAggregation = &StormAggregationStatus{
            IsStorm:     true,
            WindowStart: rr.Status.Deduplication.FirstOccurrence,
        }
    }
    rr.Status.StormAggregation.AggregatedCount++
    rr.Status.StormAggregation.WindowEnd = metav1.Now()
}
client.Status().Update(ctx, rr)
```

---

### 3. Rate Limiting: Redis → In-Memory

**Current Flow:**
```
Request → Check Redis counter → Allow/Deny
```

**Proposed Flow:**
```
Request → Check in-memory cache → Allow/Deny
```

**Rationale:**
- Failure mode is equivalent (Redis crash = lost state, Pod crash = lost state)
- In-memory is faster (~100ns vs ~1ms)
- No external dependency

**Implementation:**
```go
import "github.com/patrickmn/go-cache"

type RateLimiter struct {
    cache *cache.Cache
}

func NewRateLimiter() *RateLimiter {
    // 1 minute TTL, cleanup every 2 minutes
    return &RateLimiter{
        cache: cache.New(1*time.Minute, 2*time.Minute),
    }
}

func (r *RateLimiter) Allow(sourceIP string, limit int) bool {
    key := "ratelimit:" + sourceIP

    count, found := r.cache.Get(key)
    if !found {
        r.cache.Set(key, 1, cache.DefaultExpiration)
        return true
    }

    current := count.(int)
    if current >= limit {
        return false
    }

    r.cache.Set(key, current+1, cache.DefaultExpiration)
    return true
}
```

**Trade-off:**

| Aspect | Redis | In-Memory |
|--------|-------|-----------|
| **Scope** | Global (all pods) | Per-pod |
| **Effective limit** | Exact | limit × pods |
| **Latency** | ~1ms | ~100ns |
| **Crash recovery** | Redis survives pod crash | State lost on pod crash |

**Mitigation**: Add Nginx Ingress rate limiting as global safety net:
```yaml
annotations:
  nginx.ingress.kubernetes.io/limit-rps: "1000"
```

---

## 📊 Impact Analysis

### Infrastructure Changes

| Component | Before | After |
|-----------|--------|-------|
| **Redis** | Required | ❌ Removed |
| **Redis Helm chart** | Deployed | ❌ Removed |
| **Redis monitoring** | Required | ❌ Removed |
| **Redis secrets** | Required | ❌ Removed |

### Code Changes

| Area | Effort | Files Affected |
|------|--------|----------------|
| **Deduplication** | Medium | `pkg/gateway/processing/deduplication.go` |
| **Storm Aggregation** | Medium | `pkg/gateway/processing/storm_detector.go` |
| **Rate Limiting** | Low | `pkg/gateway/middleware/ratelimit.go` |
| **Redis client removal** | Low | `pkg/gateway/redis/client.go` (delete) |
| **Tests** | High | All Redis-dependent tests |
| **Informer setup** | Medium | `cmd/gateway/main.go` |

### Performance Comparison

| Operation | Redis | K8s Informer | In-Memory |
|-----------|-------|--------------|-----------|
| **Dedup check** | ~1ms | ~100ns (cache hit) | N/A |
| **Storm check** | ~1ms | ~100ns (cache hit) | N/A |
| **Rate limit** | ~1ms | N/A | ~100ns |
| **Cold start** | Instant | Cache sync (~5s) | Instant |

---

## ⚠️ Risk Assessment

### Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **Informer cache sync delay** | Medium | Low | Wait for cache sync before accepting requests |
| **Per-pod rate limits** | Low | Medium | Nginx Ingress as global safety net |
| **Conflict retries** | Low | Low | Built-in retry with exponential backoff |
| **etcd pressure** | Low | Medium | No additional CRDs, only status updates |

### Failure Mode Comparison

| Scenario | With Redis | Without Redis |
|----------|------------|---------------|
| **Gateway crash** | Redis state survives, new pod reconnects | State lost, starts fresh |
| **Redis crash** | State lost, Gateway can't function | N/A - no Redis |
| **K8s API unavailable** | Dedup works (Redis), RR creation fails | Dedup fails, RR creation fails |

**Key Insight**: Redis crash is equivalent or worse than pod crash. Removing Redis reduces failure modes.

---

## 📅 Implementation Timeline

| Phase | Duration | Tasks |
|-------|----------|-------|
| **Phase 1: Preparation** | 2 days | Create BRs, update DDs, design review |
| **Phase 2: Informer Setup** | 2 days | Add controller-runtime, field indexes |
| **Phase 3: Deduplication Migration** | 3 days | Migrate dedup to RR Status |
| **Phase 4: Storm Migration** | 2 days | Migrate storm to RR Status |
| **Phase 5: Rate Limit Migration** | 1 day | Implement in-memory rate limiter |
| **Phase 6: Redis Removal** | 1 day | Remove Redis client, delete Redis code |
| **Phase 7: Testing** | 3 days | Integration tests, load tests |
| **Total** | ~14 days | |

---

## ✅ Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|--------------|
| AC-1 | Gateway starts without Redis connection | Unit test |
| AC-2 | Deduplication works via RR Status | Integration test |
| AC-3 | Storm aggregation works via RR Status | Integration test |
| AC-4 | Rate limiting works via in-memory cache | Unit test |
| AC-5 | No Redis code remains in Gateway | Code review |
| AC-6 | Helm chart has no Redis dependency | Helm lint |
| AC-7 | Performance: <10ms P99 for dedup check | Load test |
| AC-8 | Dedup data visible in `kubectl get rr -o yaml` | Manual test |

---

## 📚 Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-001](../architecture/decisions/ADR-001-gateway-ro-deduplication-communication.md) | Shared status ownership decision |
| [DD-GATEWAY-011](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) | Implementation details |
| [BR-GATEWAY-181-185](../requirements/BR-GATEWAY-181-185-shared-status-deduplication.md) | Business requirements |
| [DD-GATEWAY-009](../architecture/decisions/DD-GATEWAY-009-state-based-deduplication.md) | Previous dedup design (superseded) |
| [NOTICE](NOTICE_SHARED_STATUS_OWNERSHIP_DD_GATEWAY_011.md) | Shared status notice to teams |

---

## ❓ Questions for Gateway Team

1. **Rate Limiting**: Is per-pod rate limiting acceptable, or do you need strict global limits?

2. **Cold Start**: Is ~5 second cache sync delay on Gateway startup acceptable?

3. **Storm Behavior**: Is immediate RR creation (vs. waiting for storm window) a breaking change for any downstream consumers?

4. **Testing**: Are there Redis-specific integration tests that need special migration attention?

5. **Rollback**: Should we maintain Redis compatibility behind a feature flag for initial rollout?

---

## 🗳️ Decision Required

**Please review and provide feedback:**

| Option | Description |
|--------|-------------|
| **A: Approve** | Proceed with Redis deprecation as proposed |
| **B: Approve with changes** | Approve with modifications (specify) |
| **C: Reject** | Keep Redis (provide rationale) |
| **D: Need more information** | Request additional analysis |

---

## ✅ Gateway Team Response

**Status**: ✅ **APPROVED**

```
Date: 2025-12-07
Reviewed by: Gateway Service Team
Decision: A (Approve with enhancement)
Comments:
  Gateway team proposed "Async Storm Aggregation" pattern that:
  - Creates RR immediately on first alert (no buffering delay)
  - Updates storm status asynchronously as more alerts arrive
  - Aligns with DD-ORCHESTRATOR-001 point-in-time snapshot pattern
  - Enables COMPLETE Redis removal (not just dedup/storm)

Rate Limiting Preference:
- [x] Per-pod acceptable (with Nginx Ingress safety net)
- [ ] Need global limits (keep Redis for rate limiting only)
- [ ] Other: _______________

Cold Start Delay:
- [x] 5 seconds acceptable
- [ ] Need faster startup (specify: ___ seconds)

Storm Behavior Change:
- [x] Immediate RR creation acceptable (PREFERRED over sync buffering)
- [ ] Need storm window delay preserved

Additional Concerns:
  None. Async Storm Aggregation is superior to sync buffering:
  - Lower latency (immediate vs 5min wait)
  - No Redis dependency
  - Aligns with existing DD-ORCHESTRATOR-001 pattern
```

### 🔑 Key Innovation: Async Storm Aggregation

The Gateway team identified that DD-GATEWAY-008's sync buffering contradicted the Redis deprecation goal. They proposed:

| Old (DD-GATEWAY-008 Sync) | New (Async Storm Aggregation) |
|---------------------------|-------------------------------|
| Buffer in Redis until threshold | Create RR immediately |
| Wait up to 5min before RR | RO processes first alert instantly |
| Storm context guaranteed for first RCA | Storm context for retry (acceptable) |
| Redis required | **Redis-free** |

**Architecture Team approved this enhancement on 2025-12-07.**

---

**Issued By**: Architecture Team
**Date**: December 7, 2025

