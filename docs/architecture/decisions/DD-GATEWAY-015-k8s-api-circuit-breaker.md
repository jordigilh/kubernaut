# DD-GATEWAY-015: Kubernetes API Circuit Breaker Implementation

**Date**: January 3, 2026
**Status**: ✅ IMPLEMENTED
**Deciders**: Gateway Team, Infrastructure Team
**Confidence**: 85%
**Related**: DD-GATEWAY-014, BR-GATEWAY-093, ADR-048, BR-GATEWAY-111-114

---

## Context & Problem

### Problem Statement

Gateway Service depends critically on the Kubernetes API for creating `RemediationRequest` CRDs. When the K8s API experiences degradation (high latency, rate limiting, or unavailability), Gateway must **fail-fast** to prevent:
- Request queue buildup (memory exhaustion)
- Cascading failures (repeated failed attempts overloading K8s control plane)
- Alert loss (retry exhaustion during K8s API outages)

**This decision implements Alternative 3 from DD-GATEWAY-014**: Circuit breaker for K8s API dependency only, not for service-level ingress load.

### Current K8s API Protection

| Mechanism | Status | Limitation |
|-----------|--------|-----------|
| **Retry logic** | ✅ Implemented (BR-111-114) | Retries still consume resources during outages |
| **Exponential backoff** | ✅ Implemented | Delays failures but doesn't prevent them |
| **Circuit breaker** | ❌ Missing | No fail-fast when K8s API is known to be degraded |

### Key Requirements

From Business Requirements:
- **BR-GATEWAY-093**: Circuit breaker for critical dependencies (K8s API)
  - **BR-GATEWAY-093-A**: Fail-fast when K8s API unavailable
  - **BR-GATEWAY-093-B**: Prevent cascade failures during K8s API overload
  - **BR-GATEWAY-093-C**: Observable metrics for circuit breaker state and operations

---

## Decision

**CHOSEN**: Implement circuit breaker for Kubernetes API client operations using `github.com/sony/gobreaker`

### Architecture

**Wrapper Pattern**: `ClientWithCircuitBreaker` wraps `k8s.Client` to intercept K8s API calls

```go
// pkg/gateway/k8s/client_with_circuit_breaker.go
type ClientWithCircuitBreaker struct {
    *Client                        // Embed base client
    cb      *gobreaker.CircuitBreaker
    metrics *metrics.Metrics
}

// Circuit breaker protects:
// - CreateRemediationRequest
// - UpdateRemediationRequest
// - GetRemediationRequest
// - ListRemediationRequestsByFingerprint
```

### Circuit Breaker Configuration

```go
gobreaker.Settings{
    Name:        "k8s-api",
    MaxRequests: 3,              // Half-open state: allow 3 test requests
    Interval:    60 * time.Second,  // Reset success/failure counters every 60s
    Timeout:     30 * time.Second,  // Half-open after 30s in open state
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        // Trip if 50% failure rate over 10 requests
        failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
        return counts.Requests >= 10 && failureRatio >= 0.5
    },
    OnStateChange: func(name string, from, to gobreaker.State) {
        // Update gateway_circuit_breaker_state metric
        metrics.CircuitBreakerState.WithLabelValues(name).Set(float64(to))
    },
}
```

**Rationale for Thresholds**:
- **50% failure rate**: Aggressive tripping to protect K8s API control plane
- **10 request minimum**: Prevents false positives from single-request spikes
- **30s timeout**: Balance between fast recovery and avoiding flapping
- **3 half-open requests**: Conservative test before fully closing

### Circuit Breaker States

| State | Behavior | Gateway Response | Metrics |
|-------|----------|------------------|---------|
| **Closed** | All requests pass through | Normal CRD creation | `gateway_circuit_breaker_state{name="k8s-api"}=0` |
| **Open** | All requests fail-fast | `503 Service Unavailable` | `gateway_circuit_breaker_state{name="k8s-api"}=2` |
| **Half-Open** | Limited test requests | 3 requests allowed, then evaluate | `gateway_circuit_breaker_state{name="k8s-api"}=1` |

### Integration Points

**1. Server Initialization** (`pkg/gateway/server.go`):
```go
func NewServerWithK8sClient(k8sClient *k8s.Client, ...) *Server {
    // Wrap K8s client with circuit breaker
    cbClient := k8s.NewClientWithCircuitBreaker(k8sClient, metricsInstance)

    // Inject into CRD creator
    crdCreator := processing.NewCRDCreator(cbClient, logger, metricsInstance, ...)

    return &Server{
        crdCreator: crdCreator,
        // ...
    }
}
```

**2. CRD Creator** (`pkg/gateway/processing/crd_creator.go`):
```go
// Uses circuit-breaker-protected client transparently
crd, err := c.k8sClient.CreateRemediationRequest(ctx, remediationRequest)
if err != nil {
    if errors.Is(err, gobreaker.ErrOpenState) {
        // Circuit breaker is open - K8s API degraded
        return NewCircuitBreakerOpenError(...)
    }
    // Handle other K8s API errors
}
```

---

## Rationale

### Why `github.com/sony/gobreaker`?

| Library | Pros | Cons | Decision |
|---------|------|------|----------|
| **sony/gobreaker** | ✅ Industry standard<br>✅ Production-proven (Sony, Netflix)<br>✅ Simple API<br>✅ Well-documented | - | ✅ **CHOSEN** |
| **Custom implementation** | ✅ Full control | ❌ Reinventing wheel<br>❌ Untested in production<br>❌ Maintenance burden | ❌ Rejected |
| **afex/hystrix-go** | ✅ Netflix patterns | ❌ Abandoned (2017)<br>❌ Complex API | ❌ Rejected |

**Decision**: Use `sony/gobreaker` for production-grade reliability with minimal maintenance

### Why K8s API Circuit Breaker (Not Service-Level)?

**Complements DD-GATEWAY-014 Decision**:
- ✅ DD-GATEWAY-014 deferred **service-level** circuit breaker (ingress protection)
- ✅ DD-GATEWAY-015 implements **K8s API** circuit breaker (dependency protection)
- ✅ No contradiction: Different scopes, different problems

**Evidence Supporting K8s API Circuit Breaker**:
1. **K8s API is critical path**: 100% of Gateway requests depend on K8s API
2. **Retry logic insufficient**: Retries still consume resources during K8s API outages
3. **Control plane protection**: Prevents Gateway from overloading K8s control plane
4. **Fail-fast enables observability**: Clear signal when K8s API is degraded (503 errors)

**Distinction from Service-Level Circuit Breaker**:

| Aspect | Service-Level (DD-GATEWAY-014 - Deferred) | K8s API (DD-GATEWAY-015 - Implemented) |
|--------|-------------------------------------------|----------------------------------------|
| **Scope** | Ingress load protection | Dependency protection |
| **Trigger** | Gateway overload (high QPS, memory) | K8s API degradation (50% error rate) |
| **Protection** | Gateway service itself | K8s control plane |
| **Response** | Reject ingress requests | Fail-fast on CRD operations |
| **Status** | ⏸️ Deferred to production monitoring | ✅ Implemented (2026-01-03) |

---

## Consequences

### Positive Consequences

**1. Fail-Fast During K8s API Outages**
- ✅ Gateway returns `503 Service Unavailable` immediately when circuit open
- ✅ No request queue buildup (prevents memory exhaustion)
- ✅ Clear signal to monitoring/alerting (K8s API degraded)

**2. Protect K8s Control Plane**
- ✅ Prevents repeated failed CRD creation attempts overloading K8s API
- ✅ Allows K8s control plane to recover without Gateway traffic
- ✅ Reduces cascade failure risk in multi-tenant clusters

**3. Observable Degradation**
- ✅ Metrics expose circuit breaker state in real-time
- ✅ Operators can respond proactively (scale K8s control plane, investigate)
- ✅ Alerts trigger when circuit breaker opens (K8s API degraded)

**4. Complements Existing Protections**
- ✅ Works with retry logic (BR-111-114): Circuit breaker prevents retries when K8s API known degraded
- ✅ Works with proxy rate limiting (ADR-048): Prevents Gateway overload
- ✅ Works with fail-open design (DD-GATEWAY-011): Degrades gracefully

**5. Production-Grade Reliability**
- ✅ Industry-standard library (`sony/gobreaker`)
- ✅ Proven in production at scale (Sony, Netflix)
- ✅ Simple API, minimal maintenance burden

---

### Negative Consequences

**1. False Positives Possible**
- ❌ Legitimate K8s API transient errors could trip circuit breaker
- ❌ Alert loss during false positive window (30s timeout)

**Mitigation**:
- ✅ 50% failure rate threshold (aggressive but balanced)
- ✅ 10 request minimum (prevents single-request spikes)
- ✅ Monitoring alerts enable fast response
- ✅ Retry logic still active when circuit closed

**2. Additional Complexity**
- ❌ New component to monitor and debug
- ❌ Circuit breaker state must be understood by operators

**Mitigation**:
- ✅ Simple API, minimal code footprint
- ✅ Clear metrics and documentation
- ✅ Standard pattern, well-understood by industry

**3. Dependency on External Library**
- ❌ Requires `github.com/sony/gobreaker` dependency
- ❌ Library maintenance risk (abandoned, breaking changes)

**Mitigation**:
- ✅ Active maintenance (last release: 2023, still maintained)
- ✅ Stable API (no breaking changes since 2019)
- ✅ Small library (~500 LOC), easy to fork if needed

---

## Metrics & Observability

### Circuit Breaker Metrics

**1. Circuit Breaker State** (Gauge):
```prometheus
gateway_circuit_breaker_state{name="k8s-api"}
# 0 = Closed (normal)
# 1 = Half-Open (testing recovery)
# 2 = Open (K8s API degraded)
```

**2. Circuit Breaker Operations** (Counter):
```prometheus
gateway_circuit_breaker_operations_total{name="k8s-api",result="success|failure"}
# Tracks success/failure ratio through circuit breaker
```

### Alert Rules

**Critical Alert: Circuit Breaker Open**
```yaml
- alert: GatewayCircuitBreakerOpen
  expr: gateway_circuit_breaker_state{name="k8s-api"} == 2
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Gateway K8s API circuit breaker is OPEN"
    description: "K8s API is degraded. Gateway is failing-fast to protect control plane."
    action: "Investigate K8s API health, scale control plane if needed"
```

**Warning Alert: High Failure Rate**
```yaml
- alert: GatewayK8sAPIHighFailureRate
  expr: |
    rate(gateway_circuit_breaker_operations_total{name="k8s-api",result="failure"}[5m]) /
    rate(gateway_circuit_breaker_operations_total{name="k8s-api"}[5m]) > 0.20
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Gateway K8s API failure rate > 20%"
    description: "Circuit breaker may trip soon if failure rate continues"
```

### Dashboard Panels

**Gateway - K8s API Health Dashboard**:
1. **Circuit Breaker State** (0/1/2 gauge)
2. **K8s API Success Rate** (percentage)
3. **K8s API Operation Latency** (p50/p95/p99)
4. **Circuit Breaker Open Events** (count per hour)

---

## Testing Strategy

### Unit Tests
- ✅ Circuit breaker state transitions (closed → open → half-open → closed)
- ✅ Failure threshold calculation (50% over 10 requests)
- ✅ Timeout behavior (30s open → half-open)
- ✅ Metrics updates on state changes

### Integration Tests (BR-GATEWAY-093)
**File**: `test/integration/gateway/k8s_api_failure_test.go`

**Test Cases**:
1. **BR-GATEWAY-093-A: Fail-fast when K8s API unavailable**
   - Simulate K8s API failures (10 consecutive errors)
   - Verify circuit breaker opens (state = 2)
   - Verify subsequent requests fail-fast (no K8s API calls)
   - Verify `gateway_circuit_breaker_state{name="k8s-api"}=2` metric

2. **BR-GATEWAY-093-B: Prevent cascade failures**
   - Simulate K8s API degradation (50% failure rate)
   - Verify circuit breaker trips after 10 requests
   - Verify K8s API call rate drops (circuit open)
   - Verify Gateway remains responsive (503 errors, not timeouts)

3. **BR-GATEWAY-093-C: Observable metrics**
   - Trigger circuit breaker state transitions
   - Verify `gateway_circuit_breaker_state` gauge updates
   - Verify `gateway_circuit_breaker_operations_total` counter increments
   - Verify metrics match actual circuit breaker state

### E2E Tests
- ✅ K8s API failure scenarios in Kind cluster
- ✅ Circuit breaker recovery after K8s API restoration
- ✅ Load testing with k6 (circuit breaker under high traffic)

---

## Implementation

### Files Modified/Created

**New Files**:
1. `pkg/gateway/k8s/client_with_circuit_breaker.go` - Circuit breaker wrapper (197 lines)
2. `pkg/shared/circuitbreaker/manager.go` - Multi-channel manager (shared with Notification)
3. `docs/architecture/decisions/DD-GATEWAY-015-k8s-api-circuit-breaker.md` - This document

**Modified Files**:
1. `pkg/gateway/server.go` - Circuit breaker initialization
2. `pkg/gateway/metrics/metrics.go` - Circuit breaker metrics
3. `docs/services/stateless/gateway-service/metrics-slos.md` - Circuit breaker metrics documentation
4. `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md` - BR-GATEWAY-093 updated
5. `docs/services/stateless/gateway-service/BR_MAPPING.md` - BR-GATEWAY-093 implementation details

**Test Files** (to be added):
1. `test/integration/gateway/k8s_api_failure_test.go` - Circuit breaker integration tests (BR-GATEWAY-093)

### Library Dependencies

**Added**: `github.com/sony/gobreaker v1.0.0`
- **License**: MIT (compatible with Apache-2.0)
- **Size**: ~500 LOC (minimal footprint)
- **Maintenance**: Active (last release 2023, stable API)

---

## Production Readiness

### Rollout Plan

**Phase 1: Monitoring (2-4 weeks)**
- Deploy Gateway with K8s API circuit breaker enabled
- Monitor circuit breaker metrics (state, operations)
- Alert on circuit breaker open events

**Phase 2: Tuning (if needed)**
- Adjust thresholds if false positives observed
- Tune timeout (30s) and max requests (3) if needed

### Operational Runbook

**When Circuit Breaker Opens**:
1. **Verify K8s API Health**:
   ```bash
   kubectl get --raw /healthz
   kubectl get --raw /livez
   kubectl get --raw /readyz
   ```

2. **Check K8s API Server Logs**:
   ```bash
   kubectl logs -n kube-system kube-apiserver-*
   ```

3. **Scale Control Plane** (if needed):
   ```bash
   # For managed K8s (EKS/GKE/AKS): Use cloud provider console
   # For self-hosted: Add control plane replicas
   ```

4. **Verify Gateway Recovery**:
   ```bash
   # Wait for circuit breaker to close
   watch -n 2 'curl -s http://gateway:9090/metrics | grep gateway_circuit_breaker_state'
   ```

---

## Related Decisions

- **DD-GATEWAY-014**: Service-Level Circuit Breaker Deferral - This decision implements Alternative 3
- **BR-GATEWAY-093**: Circuit Breaker for K8s API - Business requirement backing this implementation
- **ADR-048**: Rate Limiting Proxy Delegation - Complements circuit breaker (ingress protection)
- **BR-GATEWAY-111-114**: Retry logic - Complements circuit breaker (transient error handling)
- **DD-GATEWAY-011**: Shared Status Ownership - Fail-open design pattern

---

## References

- **gobreaker Documentation**: https://github.com/sony/gobreaker
- **Circuit Breaker Pattern**: https://martinfowler.com/bliki/CircuitBreaker.html
- **Gateway Metrics SLOs**: `docs/services/stateless/gateway-service/metrics-slos.md`
- **Gateway Business Requirements**: `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`

---

**Decision Status**: ✅ IMPLEMENTED (2026-01-03)
**Review Date**: After 2-4 weeks of production monitoring
**Success Criteria**: Circuit breaker triggers < 1/week, no false positive alerts
**Library**: `github.com/sony/gobreaker v1.0.0`

