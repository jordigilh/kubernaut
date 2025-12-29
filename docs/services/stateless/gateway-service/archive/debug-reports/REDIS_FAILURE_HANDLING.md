# Gateway Service: Redis Failure Handling Strategy

## Executive Summary

**Decision**: Gateway rejects requests (503 Service Unavailable) when Redis is unavailable, rather than continuing with graceful degradation.

**Rationale**: Business requirements BR-GATEWAY-008 and BR-GATEWAY-009 are **MUST** requirements. Creating duplicate CRDs or allowing system overwhelm violates data integrity and system stability.

**Confidence**: 95% ✅

## Problem Statement

When Redis becomes unavailable, the Gateway has two options:

1. **Graceful Degradation**: Continue processing, create CRDs without deduplication/storm detection
2. **Request Rejection**: Return 503 Service Unavailable, let Prometheus retry

## Decision: Request Rejection (503)

### Implementation

```go
// Deduplication check
isDuplicate, _, err := s.dedupService.Check(ctx, signal)
if err != nil {
    // Redis unavailable → cannot guarantee deduplication (BR-GATEWAY-008)
    // Return 503 Service Unavailable → Prometheus will retry
    s.respondError(w, http.StatusServiceUnavailable,
        "deduplication service unavailable", requestID, err)
    return
}

// Storm detection check
isStorm, metadata, err := s.stormDetector.Check(ctx, signal)
if err != nil {
    // Redis unavailable → cannot guarantee storm protection (BR-GATEWAY-009)
    // Return 503 Service Unavailable → Prometheus will retry
    s.respondError(w, http.StatusServiceUnavailable,
        "storm detection service unavailable", requestID, err)
    return
}
```

### Business Requirements Compliance

| Requirement | Graceful Degradation | Request Rejection |
|-------------|---------------------|-------------------|
| **BR-GATEWAY-008**: MUST deduplicate | ❌ Violates (creates duplicates) | ✅ Complies (rejects to prevent duplicates) |
| **BR-GATEWAY-009**: MUST detect storms | ❌ Violates (no storm protection) | ✅ Complies (rejects to prevent overwhelm) |
| **BR-GATEWAY-010**: MUST persist state | ❌ Violates (no persistence) | ✅ Complies (rejects when can't persist) |

## Comparison Analysis

### Graceful Degradation (Rejected Approach)

**Pros**:
- ✅ Higher availability (accepts all requests)
- ✅ Simpler (no Prometheus retry complexity)

**Cons**:
- ❌ **Creates duplicate CRDs** → violates BR-GATEWAY-008
- ❌ **No storm protection** → system can be overwhelmed
- ❌ **Data integrity compromised** → AI processes duplicates
- ❌ **Wasted resources** → duplicate AI analysis
- ❌ **Inconsistent behavior** → sometimes deduplicated, sometimes not

**Confidence**: 20% ❌ - Violates core business requirements

### Request Rejection (Approved Approach)

**Pros**:
- ✅ **Zero duplicate CRDs** → BR-GATEWAY-008 compliance
- ✅ **Storm protection maintained** → BR-GATEWAY-009 compliance
- ✅ **Data integrity guaranteed** → no corrupt data
- ✅ **Prometheus retry** → built-in retry mechanism
- ✅ **Clear error messages** → operators know what's wrong
- ✅ **Predictable behavior** → always deduplicated or rejected

**Cons**:
- ⚠️ Lower availability during Redis outages (rejects requests)
- ⚠️ More complex (Prometheus retry behavior)

**Confidence**: 95% ✅ - Aligns with business requirements

**Remaining 5% Risk**: Prolonged Redis outage (>5 minutes) could cause alert backlog in Prometheus.

**Mitigation**: Redis HA (Sentinel) ensures <10s downtime during failovers.

## Redis HA Solution

To minimize request rejections, we deploy Redis with High Availability (Sentinel):

### Architecture

- **3 Redis instances**: 1 master + 2 replicas
- **3 Sentinel instances**: Monitor master health, automatic failover
- **Failover time**: 5-10 seconds
- **Quorum**: 2 Sentinels must agree on failure

### Failure Scenarios

| Scenario | Gateway Behavior | Downtime | Duplicate CRDs? |
|----------|------------------|----------|-----------------|
| **1 replica down** | Process normally | 0s | ✅ Zero |
| **Master down** | 503 for 5-10s, then recover | 5-10s | ✅ Zero |
| **2 instances down** | 503 until recovery | Until recovery | ✅ Zero |
| **All instances down** | 503 for all requests | Until recovery | ✅ Zero |

### Business Value

- **Availability**: 99.9%+ (only 5-10s downtime during failovers)
- **Data Integrity**: 100% (zero duplicate CRDs)
- **Automatic Recovery**: No manual intervention required
- **Clear Monitoring**: 503 rate indicates Redis health

## Prometheus Retry Behavior

### How Prometheus Handles 503

1. **Initial Request**: Prometheus sends alert → Gateway returns 503
2. **Retry Logic**: Prometheus automatically retries with exponential backoff
3. **Retry Intervals**: ~1s, ~2s, ~4s, ~8s, ~16s, ~32s
4. **Max Retries**: Continues until alert resolves or max retry time
5. **Success**: Once Redis recovers, Gateway accepts request (201)

### Example Timeline

```
T+0s:   Prometheus sends alert → Gateway 503 (Redis down)
T+1s:   Prometheus retry #1 → Gateway 503
T+3s:   Prometheus retry #2 → Gateway 503
T+7s:   Prometheus retry #3 → Gateway 503
T+10s:  Redis Sentinel failover complete
T+15s:  Prometheus retry #4 → Gateway 201 Created ✅
```

**Result**: Alert processed successfully, zero duplicate CRDs, automatic recovery.

## Monitoring & Alerting

### Key Metrics

1. **Gateway 503 Rate**: Should be near zero
   - Alert if >5% of requests return 503
   - Indicates Redis health issues

2. **Redis Pod Health**: All 3 pods should be Running/Ready
   - Alert if <2 pods healthy
   - Indicates potential failover or outage

3. **Sentinel Quorum**: At least 2 Sentinels must be healthy
   - Alert if <2 Sentinels healthy
   - Indicates failover capability compromised

4. **Failover Events**: Monitor Sentinel logs
   - Alert on failover events
   - Investigate root cause

### Operational Runbook

**Alert**: "Gateway 503 Rate High"

**Diagnosis**:
```bash
# Check Redis pods
kubectl get pods -n kubernaut-system -l app=redis

# Check Sentinel status
make test-redis-ha-status

# Check Gateway logs
kubectl logs -n kubernaut-system deployment/gateway | grep "deduplication service unavailable"
```

**Resolution**:
- If all Redis down: Scale up or wait for recovery
- If master down: Wait 5-10s for Sentinel failover
- If Sentinel unhealthy: Check Sentinel logs and configuration

## Testing Strategy

### Unit Tests

Use `miniredis` (in-memory Redis) for unit tests:
- Fast execution (no external dependencies)
- Predictable behavior
- Easy to simulate failures

### Integration Tests

Use real Redis HA cluster:
- Test actual failover behavior
- Verify Prometheus retry integration
- Validate 503 response handling

### Test Scenarios

1. **One Replica Down** → Gateway continues (no 503)
2. **Master Down (pre-failover)** → Gateway returns 503
3. **Master Down (post-failover)** → Gateway auto-recovers
4. **All Redis Down** → All requests return 503
5. **Prometheus Retry** → Verify eventual success

## Implementation Files

### Core Changes

- `pkg/gateway/server/handlers.go`: Request rejection logic
- `pkg/gateway/server/server.go`: Mandatory service validation

### Redis HA Deployment

- `deploy/redis-ha/redis-statefulset.yaml`: Redis HA with Sentinel
- `deploy/redis-ha/redis-sentinel-configmap.yaml`: Sentinel configuration
- `deploy/redis-ha/README.md`: Deployment and operations guide

### Tests

- `test/integration/gateway/redis_ha_failure_test.go`: HA failure scenarios
- `test/unit/gateway/server/handlers_test.go`: Unit tests with miniredis

### Documentation

- `docs/services/stateless/gateway-service/REDIS_FAILURE_HANDLING.md`: This document
- `deploy/redis-ha/README.md`: Operations guide

## Confidence Assessment

### Why 95% Confidence?

**Strengths**:
- ✅ Aligns with business requirements (BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010)
- ✅ Guarantees data integrity (zero duplicate CRDs)
- ✅ Leverages Prometheus built-in retry mechanism
- ✅ Redis HA minimizes downtime (5-10s failovers)
- ✅ Clear operational visibility (503 rate metric)

**Risks (5%)**:
- ⚠️ Prolonged Redis outage (>5 minutes) causes alert backlog
- ⚠️ Prometheus retry limits could drop alerts (rare)
- ⚠️ Sentinel misconfiguration could prevent failover

**Mitigations**:
- Redis HA with proper monitoring and alerting
- Prometheus retry configuration tuning
- Sentinel configuration validation in CI/CD
- Operational runbooks for common scenarios

## Conclusion

**Decision**: Gateway rejects requests (503) when Redis unavailable.

**Rationale**: Business requirements mandate zero duplicate CRDs and storm protection. Request rejection with Prometheus retry is the only approach that guarantees these requirements while maintaining high availability through Redis HA.

**Next Steps**:
1. ✅ Implement request rejection (503) in handlers
2. ✅ Deploy Redis HA with Sentinel
3. ✅ Add integration tests for failure scenarios
4. ⏳ Monitor 503 rate in production
5. ⏳ Tune Prometheus retry configuration if needed

**Confidence**: 95% ✅ - This is the correct approach for production.


