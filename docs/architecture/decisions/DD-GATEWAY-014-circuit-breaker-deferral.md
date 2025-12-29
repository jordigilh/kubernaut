# DD-GATEWAY-014: Service-Level Circuit Breaker Deferral

**Date**: December 13, 2025
**Status**: ⏸️ DEFERRED to Production Monitoring Phase
**Deciders**: Gateway Team, Architecture Team
**Confidence**: 75%
**Related**: ADR-048, BR-GATEWAY-105, BR-GATEWAY-110, DD-INFRASTRUCTURE-001, DD-GATEWAY-011

---

## Context & Problem

### Problem Statement

Gateway Service is a P0-Critical entry point for Kubernaut, designed to handle 1000 req/s sustained load with p95 latency <50ms. The question is: **Should Gateway implement a service-level circuit breaker to protect against overload and cascading failures?**

### Key Requirements

From Gateway business requirements:
- **BR-GATEWAY-105**: Backpressure Handling - Deferred to v2.0 with rationale: "Gateway is stateless with minimal processing. K8s API backpressure handled by retry logic."
- **BR-GATEWAY-110**: Load Shedding - Deferred to v2.0 with rationale: "Rate limiting (BR-038 via proxy - ADR-048) provides sufficient protection. No need for additional load shedding."

### Current Protection Mechanisms

| Mechanism | Status | Scope | Purpose |
|-----------|--------|-------|---------|
| **Proxy rate limiting** | ✅ Implemented (ADR-048) | Cluster-wide, per-IP | Prevent abuse, limit ingress |
| **Retry logic** | ✅ Implemented | Per-request | Handle K8s API transient errors |
| **Fail-open design** | ✅ Implemented (DD-GATEWAY-011) | Dependencies | Degrade gracefully (Redis/K8s API) |
| **Stateless architecture** | ✅ Implemented | Service-level | Horizontal scaling |
| **Backpressure handling** | ❌ Deferred (BR-105) | Service-level | Handle slow downstream |
| **Load shedding** | ❌ Deferred (BR-110) | Service-level | Reject when overloaded |
| **Circuit breaker** | ❓ Under evaluation | Service-level | Prevent cascading failures |

### Gateway Performance Characteristics

From `docs/services/stateless/gateway-service/api-specification.md`:

| Metric | Target | Current Implementation |
|--------|--------|------------------------|
| **Throughput** | 1000 req/s sustained | ✅ Designed for this |
| **Latency p50** | < 20ms | ✅ Achievable |
| **Latency p95** | < 50ms | ✅ With Redis + K8s API |
| **Latency p99** | < 100ms | ✅ Worst case |

**Total Processing Time**: 15-30ms per request (p95)

```
Processing Pipeline:
1. Authentication (TokenReviewer): ~2ms
2. Adapter Parsing: ~1ms
3. Normalization: ~1ms
4. Deduplication Check (K8s/Redis): ~3-5ms
5. Storm Detection: ~2-3ms
6. CRD Creation (K8s API): ~10-15ms
7. Response: ~1ms

Total: ~20-28ms
```

---

## Alternatives Considered

### Alternative 1: Implement Service-Level Circuit Breaker (Not Chosen)

**Approach**: Add circuit breaker to detect and prevent Gateway overload.

**Architecture**:
```go
type CircuitBreaker struct {
    state atomic.Value // Closed, Open, HalfOpen

    // Thresholds
    errorThreshold float64       // e.g., 50% error rate
    qpsThreshold int              // e.g., 1200 req/s (120% of capacity)
    requestVolume int             // Min requests before evaluation
    timeout time.Duration         // Time before half-open

    // Metrics
    successCount atomic.Int64
    errorCount atomic.Int64
}

func (cb *CircuitBreaker) Allow() bool {
    state := cb.getState()
    switch state {
    case Closed: return true
    case Open: return time.Since(cb.lastStateChange) > cb.timeout
    case HalfOpen: return cb.allowHalfOpen()
    }
}
```

**Integration**:
```go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if !s.circuitBreaker.Allow() {
        http.Error(w, "Service Temporarily Unavailable", http.StatusServiceUnavailable)
        return
    }
    // Process request...
}
```

**Pros**:
- ✅ Defense-in-depth protection against overload
- ✅ Protects Gateway from K8s API cascading failures
- ✅ Fast failure (no goroutine accumulation)
- ✅ Standard pattern, well-understood (8-12h implementation)
- ✅ Prevents memory exhaustion during extreme load

**Cons**:
- ❌ Adds complexity without proven need (no production evidence)
- ❌ False positives → critical alert loss during legitimate spikes
- ❌ Redundant with proxy rate limiting (primary defense)
- ❌ Requires production tuning (error/QPS thresholds)
- ❌ New component to maintain, monitor, debug
- ❌ Contradicts BR-GATEWAY-110 rationale ("rate limiting provides sufficient protection")

**Risk Assessment**: Would provide ~40% improvement in cascading failure scenarios, but likelihood of such scenarios is LOW (15%) given current protections.

---

### Alternative 2: Defer to Production Monitoring Phase (CHOSEN)

**Approach**: Deploy Gateway v1.0 WITHOUT circuit breaker, monitor production metrics for 2-4 weeks, implement if triggers met.

**Monitoring Strategy**:
```promql
# Gateway overload indicators
gateway_http_requests_in_flight > 100  # High concurrent requests
gateway_crd_retry_exhausted_total      # Retry exhaustion (alert loss)
rate(gateway_http_request_duration_seconds_bucket{le="1.0"}[5m]) < 0.95  # p95 > 1s

# K8s API backpressure
rate(gateway_crd_creation_errors_total{reason="TooManyRequests"}[5m]) > 0.01

# Memory/CPU exhaustion
container_memory_usage_bytes{pod=~"gateway-.*"} / container_spec_memory_limit_bytes > 0.85
rate(container_cpu_usage_seconds_total{pod=~"gateway-.*"}[5m]) > 1.8  # >90% of 2 CPU
```

**Implementation Triggers**:
Implement circuit breaker IF ANY of:
- ✅ Gateway OOM events (>1 per week)
- ✅ Latency p99 > 1s sustained (>5 minutes)
- ✅ Retry exhaustion rate > 1%
- ✅ Proxy rate limiting bypassed or insufficient

**Pros**:
- ✅ Aligns with BR-GATEWAY-110 rationale ("no need for load shedding")
- ✅ Evidence-based decision making (production data drives implementation)
- ✅ Avoids premature optimization (YAGNI principle)
- ✅ Quick implementation if needed (8-12h effort)
- ✅ Lower maintenance burden (fewer moving parts)
- ✅ Stateless design already enables horizontal scaling

**Cons**:
- ❌ No protection during initial production rollout (2-4 week monitoring window)
- ❌ Reactive approach (must experience failure first)
- ❌ Risk of cascading failure during K8s API outages

**Risk Mitigation**:
- ✅ Proxy rate limiting prevents excess ingress load
- ✅ Retry logic with exponential backoff reduces K8s API load
- ✅ Fail-open design (Redis/K8s API failures don't crash Gateway)
- ✅ Pod resource limits (OOM → restart, not hang)
- ✅ Horizontal scaling ready (add replicas if needed)

---

### Alternative 3: Hybrid Approach - Circuit Breaker for K8s API Only (Considered)

**Approach**: Implement circuit breaker ONLY for K8s API client dependency, not for ingress load.

**Architecture**:
```go
type K8sAPICircuitBreaker struct {
    state atomic.Value
    errorThreshold float64 // 30% error rate
    timeout time.Duration   // 30s recovery window
}

func (c *CRDCreator) CreateRemediationRequest(...) error {
    if !c.k8sCircuitBreaker.Allow() {
        return fmt.Errorf("K8s API circuit breaker open")
    }
    // Create CRD...
}
```

**Pros**:
- ✅ Protects Gateway from K8s API cascading failures specifically
- ✅ Lower scope than full service-level circuit breaker
- ✅ Allows Gateway to remain responsive (reject fast) during K8s API outages

**Cons**:
- ❌ Retry logic already provides exponential backoff (overlapping protection)
- ❌ Still requires production tuning
- ❌ Partial solution (doesn't address memory exhaustion from high ingress)
- ❌ K8s API has its own rate limiting (protects itself)

**Evaluation**: Provides marginal benefit over existing retry logic. Deferred with Alternative 2.

---

## Decision

**CHOSEN**: **Alternative 2 - Defer to Production Monitoring Phase**

### Rationale

**1. Current Protections Are Sufficient (75% confidence)**

Evidence supporting adequacy of existing protections:
- ✅ **Proxy rate limiting (ADR-048)**: Cluster-wide ingress protection prevents Gateway from seeing >1000 req/s
- ✅ **Retry logic**: Exponential backoff (1s → 2s → 4s) handles K8s API transient errors
- ✅ **Fail-open design (DD-GATEWAY-011)**: Redis failures don't crash Gateway (dedup disabled)
- ✅ **Stateless architecture**: Horizontal scaling is simple (add pods)
- ✅ **BR-GATEWAY-110 rationale**: Architecture team already assessed load protection not needed for v1.0

**2. No Production Evidence of Overload (0% evidence)**

Pre-release product → no data to validate need:
- ❌ No metrics on actual QPS under production load
- ❌ No evidence of K8s API cascading failures
- ❌ No data on memory/CPU usage patterns
- ❌ Cannot tune circuit breaker thresholds without real load data

**3. Low Risk with Quick Implementation Path (85% feasibility)**

Risk analysis shows LOW likelihood of overload scenarios:
- **Alert Storm**: Proxy rate limiting prevents excess load → LOW risk (25%)
- **K8s API Cascading Failure**: Retry logic reduces load, K8s API self-protects → LOW risk (15%)
- **Memory Exhaustion**: Stateless design, no buffering, pod resource limits → VERY LOW risk (5%)

**If production shows need**: 8-12h implementation effort (standard pattern, straightforward integration)

**4. Alignment with YAGNI Principle**

Adding circuit breaker now would be premature optimization:
- ✅ No proven need
- ✅ Adds complexity and maintenance burden
- ✅ False positives → critical alert loss
- ✅ Monitor first, implement if evidence supports

**5. Contradicts Existing Architecture Decisions**

Implementing circuit breaker would contradict:
- **BR-GATEWAY-110**: "Rate limiting provides sufficient protection. No need for additional load shedding."
- **BR-GATEWAY-105**: "Gateway is stateless with minimal processing. K8s API backpressure handled by retry logic."

**These rationales remain valid** → circuit breaker not needed unless production disproves them.

---

## Implementation Guidance

### Phase 1: Production Monitoring (2-4 weeks)

**Deploy Gateway v1.0 WITHOUT circuit breaker**

**Monitor these metrics**:
```yaml
# Gateway overload indicators
- alert: GatewayHighConcurrentRequests
  expr: gateway_http_requests_in_flight > 100
  for: 5m
  severity: warning

- alert: GatewayRetryExhaustion
  expr: rate(gateway_crd_retry_exhausted_total[5m]) > 0.01
  for: 5m
  severity: critical

- alert: GatewayHighLatency
  expr: histogram_quantile(0.95, rate(gateway_http_request_duration_seconds_bucket[5m])) > 1.0
  for: 5m
  severity: warning

# K8s API backpressure
- alert: GatewayK8sAPIRateLimiting
  expr: rate(gateway_crd_creation_errors_total{reason="TooManyRequests"}[5m]) > 0.01
  for: 5m
  severity: warning

# Resource exhaustion
- alert: GatewayHighMemoryUsage
  expr: container_memory_usage_bytes{pod=~"gateway-.*"} / container_spec_memory_limit_bytes > 0.85
  for: 10m
  severity: warning

- alert: GatewayHighCPUUsage
  expr: rate(container_cpu_usage_seconds_total{pod=~"gateway-.*"}[5m]) > 1.8
  for: 10m
  severity: warning
```

**Evaluation Criteria** (after 2-4 weeks):
- ✅ ALL alerts quiet → Circuit breaker NOT needed
- ❌ ANY alert firing frequently (>1/week) → Proceed to Phase 2

---

### Phase 2: Circuit Breaker Implementation (IF triggered)

**Triggers for Implementation**:
```
IF ANY of:
  - Gateway OOM events > 1/week
  - Latency p99 > 1s sustained (>5 minutes)
  - Retry exhaustion rate > 1%
  - Proxy rate limiting bypassed or insufficient

THEN: Implement circuit breaker (8-12h effort)
```

**Implementation Steps**:

**1. Create Circuit Breaker Component** (2-3h)
```go
// pkg/gateway/circuitbreaker/breaker.go
type CircuitBreaker struct {
    state atomic.Value // Closed, Open, HalfOpen
    errorThreshold float64       // e.g., 50%
    qpsThreshold int              // e.g., 1200 req/s
    requestVolume int             // e.g., 100
    timeout time.Duration         // e.g., 30s
    successCount atomic.Int64
    errorCount atomic.Int64
}
```

**2. Integrate with Server** (1-2h)
```go
// pkg/gateway/server.go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if !s.circuitBreaker.Allow() {
        http.Error(w, "Service Temporarily Unavailable", http.StatusServiceUnavailable)
        s.metrics.CircuitBreakerRejectionsTotal.Inc()
        return
    }
    // Process request...
}
```

**3. Add Configuration** (1h)
```yaml
# ConfigMap: kubernaut-gateway-config
circuitBreaker:
  enabled: true
  errorThreshold: 0.50      # 50% error rate
  qpsThreshold: 1200        # 120% of capacity
  requestVolume: 100        # Min requests before evaluation
  timeout: 30s              # Recovery window
```

**4. Add Metrics** (1h)
```go
circuitBreakerState      // Gauge: 0=Closed, 1=Open, 2=HalfOpen
circuitBreakerRejectionsTotal  // Counter
circuitBreakerTransitionsTotal // Counter (by state)
```

**5. Testing** (4-6h)
- Unit tests: Circuit breaker logic
- Integration tests: Overload scenarios
- E2E tests: Load testing with k6

**Total Effort**: 8-12 hours

---

## Consequences

### Positive Consequences

**1. Evidence-Based Decision Making**
- ✅ Production metrics will validate or invalidate need
- ✅ Avoids premature optimization
- ✅ Focuses engineering effort on proven needs

**2. Alignment with Existing Architecture**
- ✅ Consistent with BR-GATEWAY-110 rationale
- ✅ Leverages existing protections (proxy rate limiting, retry logic)
- ✅ Maintains stateless design benefits

**3. Quick Implementation Path**
- ✅ 8-12h effort if production shows need
- ✅ Standard pattern, well-understood
- ✅ Clear triggers for implementation decision

**4. Lower Initial Complexity**
- ✅ Fewer moving parts in v1.0
- ✅ Reduced maintenance burden
- ✅ Simpler debugging and troubleshooting

---

### Negative Consequences

**1. No Protection During Initial Rollout**
- ❌ 2-4 week monitoring window without circuit breaker
- ❌ Risk of cascading failure during K8s API outages (LOW likelihood: 15%)
- ❌ Potential alert loss if overload occurs

**Mitigation**:
- ✅ Proxy rate limiting provides primary defense
- ✅ Retry logic reduces K8s API load
- ✅ Horizontal scaling ready (add replicas quickly)
- ✅ Monitoring alerts enable fast response

**2. Reactive Approach**
- ❌ Must experience failure first to implement
- ❌ Cannot preemptively tune circuit breaker thresholds

**Mitigation**:
- ✅ Quick implementation path (8-12h)
- ✅ Standard pattern requires minimal design work
- ✅ Production data enables better tuning

**3. False Sense of Security**
- ❌ Team may assume existing protections are infallible
- ❌ Risk of complacency during monitoring phase

**Mitigation**:
- ✅ Clear monitoring alerts and escalation
- ✅ Documented triggers for circuit breaker implementation
- ✅ Regular review of production metrics

---

### Neutral Consequences

**1. Monitoring Overhead**
- Additional alerts and dashboards required
- Regular review of Gateway metrics (weekly)

**2. Future Implementation Burden**
- IF triggers met, must allocate 8-12h for implementation
- Requires production load testing to tune thresholds

---

## Production Readiness

### Monitoring Requirements

**Dashboard**: Gateway Service Overload Monitoring

**Panels**:
1. **QPS and Concurrency**
   - `rate(gateway_http_requests_total[5m])`
   - `gateway_http_requests_in_flight`

2. **Latency Percentiles**
   - `histogram_quantile(0.50, rate(gateway_http_request_duration_seconds_bucket[5m]))`
   - `histogram_quantile(0.95, ...)`
   - `histogram_quantile(0.99, ...)`

3. **Error Rates**
   - `rate(gateway_crd_creation_errors_total[5m])`
   - `rate(gateway_crd_retry_exhausted_total[5m])`

4. **Resource Usage**
   - `container_memory_usage_bytes{pod=~"gateway-.*"}`
   - `rate(container_cpu_usage_seconds_total{pod=~"gateway-.*"}[5m])`

5. **K8s API Health**
   - `rate(gateway_crd_creation_errors_total{reason="TooManyRequests"}[5m])`

**Review Cadence**: Weekly for first 4 weeks, then monthly

---

### Decision Review Criteria

**After 2-4 weeks of production monitoring, evaluate**:

**Criteria A: Circuit Breaker NOT Needed**
```
IF ALL of:
  - Gateway OOM events = 0
  - Latency p99 < 500ms (5× margin below trigger)
  - Retry exhaustion rate < 0.1% (10× margin below trigger)
  - No K8s API rate limiting observed

THEN: Mark DD-GATEWAY-014 as ✅ VALIDATED (circuit breaker not needed)
```

**Criteria B: Circuit Breaker NEEDED**
```
IF ANY of:
  - Gateway OOM events > 1/week
  - Latency p99 > 1s sustained
  - Retry exhaustion rate > 1%
  - Proxy rate limiting bypassed

THEN: Implement circuit breaker per Phase 2 guidance (8-12h)
      Create DD-GATEWAY-015 documenting circuit breaker implementation
```

---

## Related Decisions

- **ADR-048**: Rate Limiting Proxy Delegation - Primary ingress protection
- **BR-GATEWAY-105**: Backpressure Handling - Deferred with similar rationale
- **BR-GATEWAY-110**: Load Shedding - Deferred with similar rationale
- **DD-GATEWAY-011**: Shared Status Ownership - Fail-open design for Redis
- **DD-INFRASTRUCTURE-001**: Redis HA - Prevents Redis cascading failures
- **ADR-019**: HolmesGPT Circuit Breaker - Example of circuit breaker in Kubernaut

---

## References

- **Confidence Assessment**: `docs/handoff/CONFIDENCE_ASSESSMENT_GATEWAY_CIRCUIT_BREAKER.md` - Detailed analysis
- **Gateway API Spec**: `docs/services/stateless/gateway-service/api-specification.md` - Performance targets
- **Gateway Business Requirements**: `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md` - BR-105, BR-110
- **Storm Detection Purpose**: `docs/handoff/BRAINSTORM_STORM_DETECTION_PURPOSE.md` - Circuit breaker vs storm detection

---

**Decision Status**: ⏸️ DEFERRED to Production Monitoring Phase
**Review Date**: After 2-4 weeks of production monitoring
**Implementation Trigger**: Any monitoring alert firing >1/week
**Quick Win**: 8-12h implementation if needed

