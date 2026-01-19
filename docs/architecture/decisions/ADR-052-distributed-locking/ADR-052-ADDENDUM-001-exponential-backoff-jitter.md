# ADR-052 Addendum 001: Exponential Backoff with Jitter for Lock Retry

**Status**: ✅ **APPROVED**
**Date**: January 18, 2026
**Parent ADR**: [ADR-052: Kubernetes Lease-Based Distributed Locking Pattern](ADR-052-distributed-locking-pattern.md)
**Affects**: Gateway service (`pkg/gateway/server.go`)
**Shared Component**: [`pkg/shared/backoff`](../../../pkg/shared/backoff/backoff.go)

---

## Context

### Original ADR-052 Retry Strategy

ADR-052 (December 30, 2025) specified the following retry strategy for lock contention:

```
**Retry Strategy** (lines 480-484):
- **Backoff**: 100ms initial (controller-native `RequeueAfter`)
- **Max retries**: Let controller retry indefinitely
- **Exponential backoff**: Not needed (lock will be released within 30s)
```

### Implementation Issue Discovered (January 18, 2026)

**Root Cause Analysis**: The Gateway service's implementation (`pkg/gateway/server.go`) revealed **four critical design flaws**:

#### 1. ❌ **Unbounded Recursion (Stack Overflow Risk)**

**Original Implementation** (lines 1039-1043):
```go
// Still no RR - recursively retry lock acquisition
logger.V(1).Info("No RR found after backoff, retrying lock acquisition",
    "fingerprint", signal.Fingerprint)
return s.ProcessSignal(ctx, signal) // ← RECURSIVE CALL
```

**Problem**: Under prolonged lock contention (e.g., 100 concurrent requests for same fingerprint), the recursion depth could reach **hundreds of levels**, exhausting the stack and causing a **panic**.

**Evidence**:
- Each `ProcessSignal` call adds ~200 bytes to stack
- 500 recursive calls = ~100KB stack usage
- Go default stack = 2-8KB initial, can grow but not unlimited
- Real-world scenario: Alert storm with 30s lease = 300 recursive calls possible

#### 2. ❌ **No Retry Limit (Potential Infinite Loop)**

**Problem**: The original design stated "let controller retry indefinitely", but Gateway is **not a controller** - it's an HTTP server. Unbounded retries on an HTTP request path can:
- Exhaust goroutines (each request holds a goroutine)
- Block HTTP workers indefinitely
- Cause cascading timeouts across clients
- No circuit breaker or timeout protection

**Impact**:
- HTTP client timeout (typically 30s) would occur **before** internal retry logic terminates
- Wasted resources during that 30s window
- Poor user experience (long response times without clarity)

#### 3. ❌ **Fixed Backoff (Thundering Herd Risk)**

**Original Implementation**:
```go
time.Sleep(100 * time.Millisecond) // ← FIXED BACKOFF
```

**Problem**: When 100 Gateway pods experience lock contention simultaneously:
- **Without jitter**: All 100 retry at EXACTLY 100ms → thundering herd on K8s API
- **K8s API impact**: 100 simultaneous `Lease.Get()` calls → API rate limiting risk
- **No exponential growth**: Lock holder processing slow operation (e.g., K8s CRD create + validation) may take >1s, causing repeated retries

**Evidence from Production Patterns**:
- Notification service (BR-NOT-055) uses ±10% jitter to prevent thundering herd
- Industry best practice (AWS, Google Cloud) always recommends jitter
- ADR-052 cited 30s lease but didn't account for multi-replica retry storm

#### 4. ❌ **Recursive Retry Instead of Iterative**

**Problem**: Recursive implementation makes it **impossible to track progress**:
- Cannot count retry attempts (each recursion creates new scope)
- Cannot implement exponential backoff (no attempt counter)
- Cannot add circuit breaker or max retry logic
- Debugging stack traces become unreadable after 10+ levels

---

## Decision

**APPROVED: Replace fixed-backoff recursion with iterative exponential backoff + jitter**

### Solution: Reuse `pkg/shared/backoff` (Battle-Tested)

**Why Shared Backoff**:
- ✅ Already proven in production (Notification v3.1)
- ✅ Supports exponential backoff with configurable multiplier
- ✅ Includes jitter (anti-thundering herd)
- ✅ Flexible configuration (conservative/standard/aggressive strategies)
- ✅ Single source of truth across all services
- ✅ Enables BR-NOT-052, BR-NOT-055, BR-WE-012

### Implementation Pattern

**New Retry Strategy**:
```go
// pkg/gateway/server.go (lines 992-1083)
const maxRetries = 10 // 10 retries = ~2.5s total wait

// Configure shared backoff with jitter
backoffConfig := backoff.Config{
    BasePeriod:    100 * time.Millisecond,  // Start at 100ms (proven)
    MaxPeriod:     1 * time.Second,         // Cap at 1s (< 30s lease)
    Multiplier:    2.0,                     // Standard exponential
    JitterPercent: 10,                      // ±10% jitter (anti-thundering herd)
}

// Iterative retry loop (replaces recursive call)
for attempt := int32(1); attempt <= maxRetries; attempt++ {
    acquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
    if err != nil {
        return nil, fmt.Errorf("distributed lock acquisition failed: %w", err)
    }

    if acquired {
        break // Exit retry loop
    }

    // Lock contention - exponential backoff with jitter
    if attempt < maxRetries {
        backoffDuration := backoffConfig.Calculate(attempt)
        time.Sleep(backoffDuration)

        // Check if other pod created RR during backoff
        shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(...)
        if shouldDeduplicate && existingRR != nil {
            // Success - RR created by other pod
            return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
        }
        // Continue to next retry
    } else {
        // Max retries exceeded
        return nil, fmt.Errorf("lock acquisition timeout after %d attempts", maxRetries)
    }
}
```

### Backoff Schedule with Jitter

| Attempt | Base Backoff | With ±10% Jitter | Cumulative Wait |
|---------|-------------|------------------|-----------------|
| 1       | 100ms       | 90-110ms         | ~100ms          |
| 2       | 200ms       | 180-220ms        | ~300ms          |
| 3       | 400ms       | 360-440ms        | ~700ms          |
| 4       | 800ms       | 720-880ms        | ~1.5s           |
| 5-10    | 1000ms (capped) | 900-1100ms   | ~2.5s-7.5s      |

**Total max wait**: ~7.5s (10 retries) - well within typical HTTP timeout (30s)

---

## Comparison: Before vs. After

| Aspect | Original (Fixed + Recursive) | Fixed (Exponential + Iterative) |
|--------|------------------------------|----------------------------------|
| **Retry Method** | Recursive call | Iterative loop |
| **Backoff** | Fixed 100ms | Exponential 100ms → 1s |
| **Jitter** | ❌ None (thundering herd risk) | ✅ ±10% (distributed load) |
| **Max Retries** | ❌ Unbounded (infinite) | ✅ 10 retries (~7.5s max) |
| **Max Wait Time** | ∞ (until HTTP timeout) | ~7.5s (predictable) |
| **Stack Safety** | ❌ Stack overflow risk | ✅ Constant stack usage |
| **Debuggability** | ❌ Deep stack traces | ✅ Clear iteration counter |
| **API Load** | ⚠️ Synchronized retry storm | ✅ Distributed (jitter) |
| **Code Reuse** | ❌ Custom logic | ✅ Shared, tested implementation |
| **Circuit Breaker** | ❌ Not possible (recursion) | ✅ Can add timeout/breaker |

---

## Why Jitter Matters: Real-World Scenario

### Scenario: Alert Storm with 100 Concurrent Signals (Same Fingerprint)

**System State**:
- 3 Gateway pods (replicas)
- 100 concurrent signals arrive (same fingerprint)
- Load balancer distributes: 33 to pod-1, 33 to pod-2, 34 to pod-3
- 1 pod acquires lock, 99 pods wait and retry

**Without Jitter (Original)**:
```
t=0ms:    100 goroutines → all attempt lock → 1 succeeds, 99 fail
t=100ms:  99 goroutines retry SIMULTANEOUSLY
          ├─ K8s API: 99 x Lease.Get() calls AT SAME INSTANT
          ├─ K8s API rate limiting may trigger
          └─ API latency spike: 99 → ~200-300ms (congestion)
t=200ms:  98 goroutines retry SIMULTANEOUSLY (repeat)
t=300ms:  97 goroutines retry SIMULTANEOUSLY (repeat)
```

**With ±10% Jitter (Fixed)**:
```
t=0ms:    100 goroutines → all attempt lock → 1 succeeds, 99 fail
t=90-110ms:   99 goroutines retry DISTRIBUTED over 20ms window
              ├─ K8s API: ~5 calls/ms (manageable load)
              └─ API latency: stable at ~50ms
t=180-220ms:  Remaining retry distributed over 40ms window
              ├─ Exponential backoff → fewer concurrent retries
              └─ Some requests already succeeded (RR created)
```

**Result**:
- **Without jitter**: K8s API experiences 99 simultaneous requests → rate limiting risk
- **With jitter**: K8s API load spread over 20-40ms windows → smooth processing

---

## Updated Implementation Guidance

### ADR-052 Section "Retry Strategy" (Line 480-484) - AMENDED

**Original Text** (DEPRECATED):
```
**Retry Strategy**:
- **Backoff**: 100ms initial (controller-native `RequeueAfter`)
- **Max retries**: Let controller retry indefinitely
- **Exponential backoff**: Not needed (lock will be released within 30s)
```

**Amended Text** (January 2026):
```
**Retry Strategy**:
- **Backoff**: Exponential with jitter (100ms → 1s, ±10%)
- **Max retries**: 10 retries (~7.5s total, iterative loop)
- **Implementation**: Use `pkg/shared/backoff` (proven in Notification v3.1)
- **Rationale**: Prevents stack overflow, thundering herd, and unbounded retries
```

### When to Use Exponential Backoff in Distributed Locking

**Use exponential backoff + jitter when**:
1. ✅ **HTTP request path** (not controller reconciliation)
2. ✅ **Multiple replicas** (>1 pod) can contend for same lock
3. ✅ **Lock hold time unpredictable** (e.g., depends on K8s API latency)
4. ✅ **High concurrency expected** (alert storms, burst traffic)

**Use fixed backoff when**:
1. ✅ **Controller reconciliation** (built-in `RequeueAfter`)
2. ✅ **Single replica** (no thundering herd risk)
3. ✅ **Lock hold time predictable** (<100ms, simple operations)
4. ✅ **Low concurrency** (few simultaneous requests)

### Shared Backoff Configuration Examples

**Standard Configuration (Gateway)**:
```go
backoffConfig := backoff.Config{
    BasePeriod:    100 * time.Millisecond,
    MaxPeriod:     1 * time.Second,
    Multiplier:    2.0,
    JitterPercent: 10,
}
```

**Aggressive Configuration (High-Volume Services)**:
```go
backoffConfig := backoff.Config{
    BasePeriod:    50 * time.Millisecond,  // Start faster
    MaxPeriod:     500 * time.Millisecond, // Lower cap
    Multiplier:    3.0,                    // Grow faster
    JitterPercent: 20,                     // More jitter
}
```

**Conservative Configuration (Low-Priority Operations)**:
```go
backoffConfig := backoff.Config{
    BasePeriod:    200 * time.Millisecond,
    MaxPeriod:     5 * time.Second,
    Multiplier:    1.5,  // Slower growth
    JitterPercent: 10,
}
```

**Use Defaults (Production-Ready)**:
```go
// Equivalent to: BasePeriod=30s, MaxPeriod=5m, Multiplier=2.0, Jitter=10%
backoffDuration := backoff.CalculateWithDefaults(attempts)
```

---

## Consequences

### Positive

- ✅ **Prevents Stack Overflow**: Iterative loop uses constant stack space
- ✅ **Bounded Retries**: Max 10 attempts = predictable behavior
- ✅ **Anti-Thundering Herd**: Jitter distributes retry load across time
- ✅ **Better API Citizenship**: Reduced K8s API load spikes
- ✅ **Debuggability**: Clear attempt counter in logs
- ✅ **Code Reuse**: Shared `pkg/shared/backoff` used across services
- ✅ **Production-Proven**: Backoff logic extracted from Notification v3.1

### Negative

- ⚠️ **Slight Latency Increase**: First retry now 90-110ms (was 100ms)
  - **Mitigation**: Jitter variance is minimal (±10ms)
  - **Benefit**: Prevents thundering herd (worth the trade-off)
- ⚠️ **Max Retry Timeout**: 10 retries = ~7.5s max wait
  - **Mitigation**: Well within HTTP timeout (30s typical)
  - **Benefit**: Prevents indefinite blocking

### Neutral

- 🔄 **Dependency on `pkg/shared/backoff`**: Gateway now imports shared package
  - **Note**: This is the intended design - shared backoff is stable and versioned

---

## Observability Updates

### New Metrics (Recommended)

```go
// Lock retry metrics (add to Gateway metrics)
gateway_lock_retry_attempts_total{fingerprint}       // Counter (per retry)
gateway_lock_retry_success_total{fingerprint}        // Counter (eventually acquired)
gateway_lock_retry_timeout_total{fingerprint}        // Counter (max retries exceeded)
gateway_lock_backoff_duration_seconds{attempt}       // Histogram (backoff per attempt)
```

### Monitoring Queries

**Lock Retry Success Rate**:
```promql
# Target: >99% success rate within 10 retries
sum(rate(gateway_lock_retry_success_total[5m])) /
sum(rate(gateway_lock_retry_attempts_total[5m]))
```

**Lock Retry Timeout Rate**:
```promql
# Target: <0.1% timeout rate
sum(rate(gateway_lock_retry_timeout_total[5m])) /
sum(rate(gateway_lock_retry_attempts_total[5m]))
```

**Backoff Duration Distribution**:
```promql
# Verify exponential growth: P50 < P95 < P99
histogram_quantile(0.50, rate(gateway_lock_backoff_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(gateway_lock_backoff_duration_seconds_bucket[5m]))
```

---

## Testing Guidance

### Unit Tests (Required)

**Test Scenarios**:
1. ✅ Lock acquired on first attempt (no retry)
2. ✅ Lock acquired after 3 retries (exponential backoff)
3. ✅ Max retries exceeded (return timeout error)
4. ✅ Backoff duration increases exponentially
5. ✅ Jitter variance within ±10% (statistical test over 100 runs)
6. ✅ Deduplication check during retry (RR created by other pod)

**Example Test**:
```go
func TestLockRetryExponentialBackoff(t *testing.T) {
    // Mock lock manager: fail 3 times, then succeed
    lockManager := &MockLockManager{
        acquireResults: []bool{false, false, false, true},
    }

    start := time.Now()
    result, err := server.ProcessSignal(ctx, signal)
    elapsed := time.Since(start)

    // Verify: 100ms + 200ms + 400ms ≈ 700ms (±jitter)
    assert.InDelta(t, 700*time.Millisecond, elapsed, 100*time.Millisecond)
    assert.NoError(t, err)
}
```

### Integration Tests (Recommended)

**Test Scenarios**:
1. ✅ Multiple Gateway pods contending for same lock (3 replicas)
2. ✅ Verify only 1 RemediationRequest created (no duplicates)
3. ✅ Verify all requests eventually succeed (<30s)
4. ✅ Verify jitter prevents simultaneous K8s API calls

### E2E Tests (Existing - No Changes Needed)

- ✅ **Test Plan**: `docs/services/stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md`
- ✅ **Status**: Existing E2E tests already validate lock behavior at system level
- ✅ **Note**: Exponential backoff is internal implementation detail (E2E tests remain unchanged)

---

## Migration Guide

### For Gateway Service (COMPLETED)

**Changes**:
- ✅ **Added**: `import "github.com/jordigilh/kubernaut/pkg/shared/backoff"`
- ✅ **Replaced**: Recursive `ProcessSignal` call with iterative loop
- ✅ **Added**: Exponential backoff configuration with jitter
- ✅ **Added**: Max retry limit (10 attempts)

**Files Modified**:
- `pkg/gateway/server.go` (lines 992-1083)

### For RemediationOrchestrator (FUTURE - If Needed)

**Note**: RO uses controller-native `RequeueAfter` (not HTTP request path), so the original ADR-052 guidance still applies:
- ✅ **Keep**: Fixed 100ms backoff via `ctrl.Result{RequeueAfter: 100 * time.Millisecond}`
- ✅ **Rationale**: Controller retry is unbounded by design (reconciliation loop)
- ❌ **No Change Needed**: RO's retry is not on HTTP request path (no stack overflow risk)

**When to Apply This Addendum to RO**:
- If RO adds synchronous lock acquisition on HTTP endpoint (e.g., admission webhook)
- If RO implements custom retry logic outside controller reconciliation

---

## Related Decisions

### Parent Decision
- **ADR-052**: Kubernetes Lease-Based Distributed Locking Pattern (December 30, 2025)

### Shared Component References
- **Shared Backoff**: [`pkg/shared/backoff/backoff.go`](../../../pkg/shared/backoff/backoff.go)
- **Backoff Tests**: [`test/unit/shared/backoff/backoff_test.go`](../../../test/unit/shared/backoff/backoff_test.go)

### Related Business Requirements
- **BR-GATEWAY-190**: Multi-Replica Deduplication Safety (enabled by this fix)
- **BR-NOT-052**: Automatic Retry with Custom Retry Policies (shared backoff origin)
- **BR-NOT-055**: Graceful Degradation (jitter anti-thundering herd)

### Related Design Decisions
- **DD-SHARED-001**: Shared Backoff Utility (to be created - rationale for `pkg/shared/backoff`)

---

## References

### Implementation References
- **Gateway Server**: [`pkg/gateway/server.go`](../../../pkg/gateway/server.go) (lines 992-1083)
- **Shared Backoff**: [`pkg/shared/backoff/backoff.go`](../../../pkg/shared/backoff/backoff.go)
- **Original ADR**: [`ADR-052-distributed-locking-pattern.md`](ADR-052-distributed-locking-pattern.md)

### Industry Best Practices
- **AWS SDK**: [Exponential Backoff and Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
- **Google Cloud**: [Retry Pattern Best Practices](https://cloud.google.com/architecture/scalable-and-resilient-apps#retry_pattern)
- **Kubernetes**: [Client-Go Rate Limiting](https://github.com/kubernetes/client-go/blob/master/util/flowcontrol/backoff.go)

### Code Review Context
- **Discovery**: E2E test triage (January 18, 2026)
- **Triage Report**: `docs/services/stateless/gateway-service/GW_DISTRIBUTED_LOCK_TRIAGE_JAN18_2026.md`
- **Fix PR**: (to be linked)

---

**Status**: ✅ **APPROVED** - Implementation complete, awaiting unit test validation

**Date Applied**: January 18, 2026

**Next Review**: After Gateway E2E tests validate fix (estimated: January 19, 2026)
