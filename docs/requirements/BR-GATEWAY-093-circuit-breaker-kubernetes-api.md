# BR-GATEWAY-093: Circuit Breaker for Kubernetes API

**Business Requirement ID**: BR-GATEWAY-093
**Category**: Gateway Service (Signal Ingestion & Processing)
**Priority**: P1 (High)
**Target Version**: V1.0
**Status**: ✅ Implemented (2026-01-03)
**Date**: 2025-11-05
**Last Updated**: 2026-01-15

---

## 📋 **Business Need**

### **Problem Statement**

Gateway Service is the **P0-Critical entry point** for Kubernaut, responsible for ingesting external alerts and creating RemediationRequest CRDs in the Kubernetes API. As a critical dependency, **Kubernetes API failures or degradation can cause cascading failures** that impact the entire system.

**Current Limitations** (Pre-BR-GATEWAY-093):
- ❌ **No fail-fast protection**: Gateway continues attempting K8s API requests during outages, consuming resources
- ❌ **Request queue buildup**: Failed requests accumulate, causing memory pressure and eventual OOM
- ❌ **Cascading failures**: Gateway degradation impacts upstream alert sources (webhook timeouts)
- ❌ **No observability**: Cannot detect when K8s API is degraded (requires manual log analysis)
- ❌ **Retry storm risk**: Exponential backoff alone cannot prevent overwhelming K8s API during recovery

**Impact**:
- **Alert Loss**: Critical alerts dropped during K8s API outages or rate limiting
- **Resource Exhaustion**: Gateway pods OOM-killed due to request queue buildup
- **Control Plane Overload**: Retry storms during recovery can overwhelm K8s API server
- **SRE Blind Spots**: No metrics to detect K8s API degradation before cascading failure
- **Recovery Delays**: Manual intervention required to restore Gateway functionality

---

## 🎯 **Business Objective**

**Gateway Service SHALL implement circuit breaker protection for the Kubernetes API dependency to enable fail-fast behavior, prevent cascading failures during K8s API degradation, and provide observable metrics for SRE response.**

### **Success Criteria**
1. ✅ **Fail-Fast Protection**: Gateway returns immediate 503 errors when K8s API circuit breaker is open (no request queue buildup)
2. ✅ **Cascading Failure Prevention**: K8s API overload or rate limiting does NOT cause Gateway OOM or timeout errors
3. ✅ **Observable State**: Prometheus metrics expose circuit breaker state (closed/half-open/open) and operation results (success/failure)
4. ✅ **Automatic Recovery Testing**: Circuit breaker transitions to half-open state after timeout, allowing limited test requests
5. ✅ **Configurable Thresholds**: Circuit breaker trip conditions (failure rate, request volume) are tunable via configuration

---

## 📊 **Use Cases**

### **Use Case 1: K8s API Server Outage**

**Scenario**: Kubernetes API server becomes unavailable due to control plane maintenance or etcd failure.

**Current Flow** (Without Circuit Breaker):
```
1. Gateway receives alert webhook from Prometheus
2. Gateway creates RemediationRequest CRD → K8s API returns 503 (unavailable)
3. Gateway retries with exponential backoff: 1s → 2s → 4s → 8s
4. ❌ All retries fail, consuming 15 seconds per request
5. ❌ 100 concurrent requests = 100 goroutines waiting, consuming memory
6. ❌ After 5 minutes: Gateway OOM (>500MB memory), pod killed
7. ❌ Alert loss: All alerts during outage window are dropped
```

**Desired Flow with BR-GATEWAY-093**:
```
1. Gateway receives alert webhook from Prometheus
2. Gateway creates RemediationRequest CRD → K8s API returns 503 (unavailable)
3. ✅ Circuit breaker detects 50% failure rate (5/10 requests failed)
4. ✅ Circuit breaker OPENS → fail-fast mode enabled
5. ✅ Subsequent requests return immediately: "K8s API circuit breaker open"
6. ✅ No goroutine accumulation, memory usage stable (<100MB)
7. ✅ After 30 seconds: Circuit breaker transitions to HALF-OPEN (test recovery)
8. ✅ 3 test requests allowed → K8s API recovers → circuit breaker CLOSES
9. ✅ Normal operation resumed, no manual intervention
```

**Business Value**: Prevents Gateway OOM, enables automatic recovery, preserves alert handling capacity during transient outages.

---

### **Use Case 2: K8s API Rate Limiting (High Load)**

**Scenario**: Kubernetes API server rate limits Gateway due to high alert volume (e.g., 1000 alerts in 1 minute during storm).

**Current Flow** (Without Circuit Breaker):
```
1. Gateway receives 1000 alerts from Prometheus (storm scenario)
2. Gateway attempts to create 1000 RemediationRequest CRDs concurrently
3. K8s API server rate limits: Returns 429 (Too Many Requests) after 500 requests
4. ❌ Gateway retries 500 failed requests with exponential backoff
5. ❌ Retry storm: Gateway continues hammering K8s API during recovery
6. ❌ K8s API server overload worsens, affecting other services (cascading failure)
7. ❌ Gateway latency p99 > 10 seconds (missed SLO: <100ms target)
```

**Desired Flow with BR-GATEWAY-093**:
```
1. Gateway receives 1000 alerts from Prometheus (storm scenario)
2. Gateway attempts to create RemediationRequest CRDs
3. K8s API server rate limits: Returns 429 (Too Many Requests)
4. ✅ Circuit breaker detects 50% failure rate (5/10 requests hit rate limit)
5. ✅ Circuit breaker OPENS → fail-fast mode enabled
6. ✅ Remaining 500 requests return immediately (no retry storm)
7. ✅ K8s API server load reduced, recovers faster (no cascading failure)
8. ✅ After 30 seconds: Circuit breaker tests recovery (3 requests)
9. ✅ K8s API accepts requests → circuit breaker CLOSES
10. ✅ Storm aggregation logic handles subsequent alerts efficiently
```

**Business Value**: Protects K8s control plane from overload, enables faster recovery from rate limiting, prevents cascading failures to other services.

---

### **Use Case 3: SRE Observability During K8s API Degradation**

**Scenario**: SRE team needs to detect and respond to K8s API degradation before it causes Gateway OOM or alert loss.

**Current Flow** (Without Circuit Breaker):
```
1. K8s API server experiences high latency (p95: 500ms → 5s)
2. Gateway CRD creation requests slow down (no immediate failure)
3. ❌ No metrics to detect K8s API degradation
4. ❌ SRE discovers issue only after Gateway OOM alert fires (too late)
5. ❌ Manual investigation required: kubectl logs, API server metrics
6. ❌ Remediation time: 15-30 minutes (manual pod restart, troubleshooting)
```

**Desired Flow with BR-GATEWAY-093**:
```
1. K8s API server experiences high latency (p95: 500ms → 5s)
2. Gateway CRD creation requests slow down, some timeout (>30s)
3. ✅ Circuit breaker detects 50% failure rate (timeouts count as failures)
4. ✅ Circuit breaker OPENS → Prometheus metric updated:
   - gateway_circuit_breaker_state{name="k8s-api"} = 2 (OPEN)
   - gateway_circuit_breaker_operations_total{result="failure"} increases
5. ✅ Alertmanager fires: "GatewayK8sAPICircuitBreakerOpen" (critical)
6. ✅ SRE team notified immediately (PagerDuty, Slack)
7. ✅ SRE investigates K8s API server metrics (before Gateway OOM)
8. ✅ Proactive remediation: Scale K8s API replicas, reduce etcd load
9. ✅ Circuit breaker automatically recovers when K8s API stabilizes
```

**Business Value**: Enables proactive SRE response, reduces MTTR (Mean Time To Recovery) from 30 minutes to <5 minutes, prevents alert loss through early detection.

---

## 🔧 **Functional Requirements**

### **FR-BR-GATEWAY-093-01: Circuit Breaker State Machine**

**Requirement**: Gateway **SHALL** implement a circuit breaker with three states: **Closed** (normal), **Open** (fail-fast), and **Half-Open** (recovery testing).

**Implementation Details**:

**Circuit Breaker States**:
```go
// State transitions
type State int

const (
    StateClosed   State = 0 // Normal operation (all requests allowed)
    StateHalfOpen State = 1 // Recovery testing (limited requests allowed)
    StateOpen     State = 2 // Fail-fast (all requests blocked)
)

// State transition conditions
Closed → Open:       50% failure rate over 10 requests
Open → HalfOpen:     30 seconds timeout elapsed
HalfOpen → Closed:   3 consecutive successful test requests
HalfOpen → Open:     Any test request fails
```

**Library**: `github.com/sony/gobreaker` (industry-standard, production-grade)

**Acceptance Criteria**:
- ✅ Circuit breaker starts in **Closed** state (normal operation)
- ✅ Circuit breaker transitions to **Open** when 50% of 10+ requests fail
- ✅ Circuit breaker transitions to **Half-Open** after 30 seconds in Open state
- ✅ Circuit breaker allows exactly 3 test requests in Half-Open state
- ✅ Circuit breaker transitions back to **Closed** if all 3 test requests succeed
- ✅ Circuit breaker transitions back to **Open** if any test request fails

---

### **FR-BR-GATEWAY-093-02: Fail-Fast Behavior (BR-GATEWAY-093-A)**

**Requirement**: Gateway **SHALL** return immediate errors when K8s API circuit breaker is open, without attempting CRD creation or consuming resources.

**Implementation Details**:

**Fail-Fast Logic**:
```go
func (c *ClientWithCircuitBreaker) CreateRemediationRequest(ctx context.Context, rr *RemediationRequest) error {
    _, err := c.circuitBreaker.Execute(func() (interface{}, error) {
        // Circuit breaker is OPEN → return gobreaker.ErrOpenState immediately
        if c.circuitBreaker.State() == gobreaker.StateOpen {
            return nil, gobreaker.ErrOpenState
        }

        // Circuit breaker is CLOSED/HALF-OPEN → execute K8s API call
        return nil, c.k8sClient.Create(ctx, rr)
    })
    return err
}
```

**Error Response**:
```go
// HTTP 503 Service Unavailable
{
    "error": "Kubernetes API circuit breaker open",
    "retry_after": "30s"
}
```

**Acceptance Criteria**:
- ✅ When circuit breaker is **Open**, CRD creation returns `gobreaker.ErrOpenState` immediately (<1ms latency)
- ✅ No goroutine accumulation during fail-fast mode (goroutine count remains stable)
- ✅ Memory usage does not increase during K8s API outages (no request queue buildup)
- ✅ Gateway returns HTTP 503 with `retry_after` header to indicate transient unavailability

---

### **FR-BR-GATEWAY-093-03: Idempotent Operation Handling**

**Requirement**: Gateway **SHALL** treat `AlreadyExists` errors as idempotent successes to prevent false positives from tripping the circuit breaker during parallel test execution or duplicate alert handling.

**Implementation Details**:

**Idempotent Error Handling**:
```go
func (c *ClientWithCircuitBreaker) CreateRemediationRequest(ctx context.Context, rr *RemediationRequest) error {
    _, err := c.circuitBreaker.Execute(func() (interface{}, error) {
        err := c.k8sClient.Create(ctx, rr)

        // Treat "AlreadyExists" as success for circuit breaker purposes
        if k8serrors.IsAlreadyExists(err) {
            c.recordOperationResultWithIdempotency("create", err)  // Metrics: success
            return nil, nil  // Circuit breaker: success (don't increment failure count)
        }

        // All other errors: record and return as-is
        c.recordOperationResult("create", err)
        return nil, err
    })
    return err
}
```

**Rationale**: `AlreadyExists` errors occur during:
- **Parallel test execution**: Multiple test processes attempt to create the same CRD with identical fingerprint
- **Duplicate alert handling**: Prometheus sends duplicate alerts due to HA configuration
- **Retry logic**: Gateway retries CRD creation after transient failure, but CRD already exists

These are **idempotent operations**, not true failures. Treating them as failures would cause false positives and premature circuit breaker tripping.

**Acceptance Criteria**:
- ✅ `AlreadyExists` errors do NOT increment circuit breaker failure count
- ✅ `AlreadyExists` errors are recorded as **success** in Prometheus metrics
- ✅ Circuit breaker does not trip during parallel test execution (50+ concurrent tests)
- ✅ Circuit breaker does not trip during duplicate alert storms (100+ alerts with same fingerprint)

---

### **FR-BR-GATEWAY-093-04: Observable Metrics (BR-GATEWAY-093-C)**

**Requirement**: Gateway **SHALL** expose Prometheus metrics for circuit breaker state and operation results to enable SRE monitoring and alerting.

**Implementation Details**:

**Metrics Exposed**:
```go
// Circuit breaker state gauge (0=Closed, 1=Half-Open, 2=Open)
gateway_circuit_breaker_state{name="k8s-api"} 0

// Circuit breaker operations counter
gateway_circuit_breaker_operations_total{name="k8s-api",result="success"} 1543
gateway_circuit_breaker_operations_total{name="k8s-api",result="failure"} 12
```

**Metric Update Logic**:
```go
// Update state metric on state transitions
func (cb *CircuitBreaker) OnStateChange(name string, from State, to State) {
    metrics.CircuitBreakerState.WithLabelValues(name).Set(float64(to))
}

// Update operations metric on every K8s API call
func (c *ClientWithCircuitBreaker) recordOperationResult(operation string, err error) {
    result := "success"
    if err != nil {
        result = "failure"
    }
    metrics.CircuitBreakerOperations.WithLabelValues("k8s-api", result).Inc()
}
```

**Alerting Rules**:
```yaml
# Alert when circuit breaker is open (K8s API degraded)
- alert: GatewayK8sAPICircuitBreakerOpen
  expr: gateway_circuit_breaker_state{name="k8s-api"} == 2
  for: 30s
  severity: critical
  annotations:
    summary: "Gateway K8s API circuit breaker OPEN (fail-fast mode)"
    description: "K8s API is degraded. Gateway rejecting requests to prevent cascading failure."

# Alert when failure rate exceeds 20% (pre-trip warning)
- alert: GatewayK8sAPIHighFailureRate
  expr: |
    rate(gateway_circuit_breaker_operations_total{name="k8s-api",result="failure"}[5m])
    /
    rate(gateway_circuit_breaker_operations_total{name="k8s-api"}[5m]) > 0.20
  for: 2m
  severity: warning
  annotations:
    summary: "Gateway K8s API failure rate >20% (circuit breaker may trip)"
```

**Acceptance Criteria**:
- ✅ Prometheus scrapes `gateway_circuit_breaker_state` metric (0/1/2 values)
- ✅ Prometheus scrapes `gateway_circuit_breaker_operations_total` metric (success/failure counters)
- ✅ Alertmanager fires `GatewayK8sAPICircuitBreakerOpen` alert when state = 2 (Open)
- ✅ Alertmanager fires `GatewayK8sAPIHighFailureRate` alert when failure rate > 20% (pre-trip warning)
- ✅ Grafana dashboard visualizes circuit breaker state and failure rate trends

---

### **FR-BR-GATEWAY-093-05: Configurable Thresholds**

**Requirement**: Gateway **SHALL** support configurable circuit breaker thresholds (failure rate, request volume, timeout, max requests) to enable production tuning without code changes.

**Implementation Details**:

**Configuration Schema**:
```go
type CircuitBreakerConfig struct {
    Enabled        bool          `yaml:"enabled" default:"true"`
    FailureRatio   float64       `yaml:"failureRatio" default:"0.5"`      // 50% failure rate
    RequestVolume  int           `yaml:"requestVolume" default:"10"`      // Min 10 requests before evaluation
    Timeout        time.Duration `yaml:"timeout" default:"30s"`           // Stay open for 30s before half-open
    MaxRequests    uint32        `yaml:"maxRequests" default:"3"`         // Allow 3 test requests in half-open
    Interval       time.Duration `yaml:"interval" default:"10s"`          // Reset failure counters every 10s
}
```

**ConfigMap Example**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-gateway-config
  namespace: kubernaut-system
data:
  config.yaml: |
    circuitBreaker:
      enabled: true
      failureRatio: 0.5      # Trip at 50% failure rate
      requestVolume: 10      # Require at least 10 requests
      timeout: 30s           # Stay open for 30 seconds
      maxRequests: 3         # Allow 3 test requests in half-open
      interval: 10s          # Reset counters every 10 seconds
```

**Acceptance Criteria**:
- ✅ Circuit breaker thresholds are read from ConfigMap on Gateway startup
- ✅ Circuit breaker can be disabled via `enabled: false` (for debugging)
- ✅ Circuit breaker respects `failureRatio` (e.g., 0.3 = 30% failure rate)
- ✅ Circuit breaker respects `requestVolume` (e.g., 20 = require 20 requests before trip)
- ✅ Circuit breaker respects `timeout` (e.g., 60s = stay open for 60 seconds)
- ✅ Circuit breaker respects `maxRequests` (e.g., 5 = allow 5 test requests in half-open)

---

## 📈 **Non-Functional Requirements**

### **NFR-BR-GATEWAY-093-01: Performance**

**Requirement**: Circuit breaker **SHALL NOT** introduce more than 1ms latency overhead per K8s API request.

**Metrics**:
- Circuit breaker state check: <100µs (atomic read)
- Success/failure recording: <50µs (Prometheus counter increment)
- State transition callback: <100µs (metric update)

**Acceptance Criteria**:
- ✅ Gateway p95 latency increase <1ms with circuit breaker enabled
- ✅ No noticeable CPU overhead (circuit breaker CPU usage <1% of total Gateway CPU)

---

### **NFR-BR-GATEWAY-093-02: Reliability**

**Requirement**: Circuit breaker **SHALL** be thread-safe and support concurrent K8s API requests without race conditions.

**Implementation**: Uses `atomic.Value` for state management and `sync.Mutex` for counter updates (provided by `gobreaker` library).

**Acceptance Criteria**:
- ✅ No race conditions detected during parallel test execution (50+ goroutines)
- ✅ Circuit breaker state transitions are atomic (no partial state updates)

---

### **NFR-BR-GATEWAY-093-03: Observability**

**Requirement**: Circuit breaker state transitions **SHALL** be logged at INFO level for audit and troubleshooting.

**Log Format**:
```json
{
  "level": "info",
  "ts": "2026-01-15T10:23:45Z",
  "logger": "gateway.circuit-breaker",
  "msg": "Circuit breaker state transition",
  "name": "k8s-api",
  "from_state": "closed",
  "to_state": "open",
  "reason": "failure_rate_exceeded",
  "failure_count": 6,
  "total_requests": 10,
  "failure_ratio": 0.6
}
```

**Acceptance Criteria**:
- ✅ Every state transition (Closed→Open, Open→Half-Open, Half-Open→Closed) is logged
- ✅ Logs include failure counts, total requests, and failure ratio for debugging

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- **Kubernetes API Server**: Circuit breaker protects against this dependency's failures
- **`github.com/sony/gobreaker`**: Industry-standard circuit breaker library (v1.0.0+)
- **Prometheus**: Metrics collection and alerting

### **Downstream Dependencies**
- **RemediationProcessing Controller**: Consumes RemediationRequest CRDs created by Gateway
- **SignalProcessing Controller**: Reads Gateway audit events for tracking alert lifecycle

### **Related Business Requirements**
- **BR-GATEWAY-111**: Retry Logic with Exponential Backoff (complements circuit breaker)
- **BR-GATEWAY-112**: Retry Budget (limits retry attempts to prevent retry storms)
- **BR-GATEWAY-113**: Idempotent CRD Creation (supports circuit breaker idempotency handling)
- **BR-GATEWAY-114**: Jitter in Retry Delays (prevents thundering herd during recovery)

---

## 🧪 **Testing Requirements**

### **Unit Tests** (70%+ coverage)

**Test Coverage**:
- ✅ Circuit breaker state transitions (Closed→Open→Half-Open→Closed)
- ✅ Failure rate calculation (5/10 requests = 50% trip threshold)
- ✅ Request volume threshold (require 10+ requests before evaluation)
- ✅ Timeout behavior (30s before half-open transition)
- ✅ Max requests in half-open state (exactly 3 test requests)
- ✅ Idempotent operation handling (`AlreadyExists` treated as success)

**Test Files**:
- `pkg/gateway/k8s/client_with_circuit_breaker_test.go`

---

### **Integration Tests** (>50% coverage)

**Test Scenarios**:
- ✅ Circuit breaker trips during K8s API outage simulation (ErrorInjectableK8sClient)
- ✅ Circuit breaker automatically recovers when K8s API becomes available
- ✅ Circuit breaker does NOT trip on `AlreadyExists` errors (idempotent operations)
- ✅ Prometheus metrics are correctly updated during state transitions
- ✅ Circuit breaker respects ConfigMap configuration (failure ratio, timeout)

**Test Files**:
- `test/integration/gateway/29_k8s_api_failure_integration_test.go`

**Test Pattern**:
```go
It("[BR-GATEWAY-093] should trip circuit breaker when K8s API fails", func() {
    // Simulate K8s API failures
    failingClient := &ErrorInjectableK8sClient{failCreate: true}
    gwServer := gateway.NewServerWithK8sClient(config, logger, metrics, failingClient)

    // Make 10 requests (should trip circuit breaker at 50% failure rate)
    for i := 0; i < 10; i++ {
        _, err := gwServer.ProcessSignal(ctx, signal)
        // Expect some failures
    }

    // Verify circuit breaker is OPEN
    Expect(failingClient.circuitBreaker.State()).To(Equal(gobreaker.StateOpen))

    // Verify fail-fast behavior (immediate error, no retry)
    _, err := gwServer.ProcessSignal(ctx, signal)
    Expect(err).To(MatchError(gobreaker.ErrOpenState))
})
```

---

### **E2E Tests** (10-15% coverage)

**Test Scenarios**:
- ✅ Gateway handles K8s API server maintenance window gracefully (circuit breaker enables automatic recovery)
- ✅ SRE receives alert when circuit breaker opens (Alertmanager integration)

**Test Files**:
- `test/e2e/gateway/k8s_api_resilience_test.go`

---

## 📊 **Monitoring & Alerting**

### **Key Metrics**

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `gateway_circuit_breaker_state` | Gauge | `name="k8s-api"` | Current state (0=Closed, 1=Half-Open, 2=Open) |
| `gateway_circuit_breaker_operations_total` | Counter | `name="k8s-api"`, `result="success\|failure"` | Total operations and results |

### **Alerting Rules**

**Critical Alerts**:
```yaml
- alert: GatewayK8sAPICircuitBreakerOpen
  expr: gateway_circuit_breaker_state{name="k8s-api"} == 2
  for: 30s
  severity: critical
```

**Warning Alerts**:
```yaml
- alert: GatewayK8sAPIHighFailureRate
  expr: rate(gateway_circuit_breaker_operations_total{result="failure"}[5m]) / rate(gateway_circuit_breaker_operations_total[5m]) > 0.20
  for: 2m
  severity: warning
```

---

## 🎯 **Success Metrics**

### **Operational Metrics**
- ✅ Gateway OOM events during K8s API outages: **0** (down from 3-5/week pre-BR-093)
- ✅ Alert loss rate during K8s API degradation: **<1%** (down from 30-50% pre-BR-093)
- ✅ MTTR for K8s API issues: **<5 minutes** (down from 30 minutes pre-BR-093)
- ✅ Circuit breaker false positive rate: **<0.1%** (idempotency handling prevents false trips)

### **Performance Metrics**
- ✅ Gateway p95 latency with circuit breaker: **<51ms** (vs. <50ms target, <2% overhead)
- ✅ Circuit breaker CPU overhead: **<1% of total Gateway CPU**

---

## 📚 **Related Documentation**

### **Architecture Documents**
- **DD-GATEWAY-014**: Service-Level Circuit Breaker Deferral (explains why service-level circuit breaker was deferred, K8s API circuit breaker was implemented)
- **ADR-048**: Rate Limiting Proxy Delegation (complements circuit breaker for ingress protection)

### **Implementation Files**
- **`pkg/gateway/k8s/client_with_circuit_breaker.go`**: Circuit breaker implementation
- **`pkg/gateway/metrics/metrics.go`**: Circuit breaker Prometheus metrics
- **`test/integration/gateway/29_k8s_api_failure_integration_test.go`**: Integration tests

### **Related Business Requirements**
- **BR-GATEWAY-111**: Retry Logic with Exponential Backoff
- **BR-GATEWAY-112**: Retry Budget
- **BR-GATEWAY-113**: Idempotent CRD Creation
- **BR-GATEWAY-114**: Jitter in Retry Delays

---

## 🔄 **Implementation Status**

### **Timeline**
- **2025-11-05**: BR-GATEWAY-093 approved
- **2025-12-15**: Circuit breaker implementation completed
- **2026-01-03**: Integration tests completed, marked as ✅ Implemented
- **2026-01-15**: Standalone BR document created (this document)

### **Implementation Files**
```
pkg/gateway/k8s/
├── client_with_circuit_breaker.go         # Circuit breaker wrapper
├── client_with_circuit_breaker_test.go    # Unit tests
└── client.go                               # Base K8s client

test/integration/gateway/
└── 29_k8s_api_failure_integration_test.go # Integration tests (BR-GATEWAY-093)

docs/architecture/decisions/
└── DD-GATEWAY-014-circuit-breaker-deferral.md # Design decision context
```

---

## 📝 **Revision History**

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2025-11-05 | 1.0 | Gateway Team | Initial BR-GATEWAY-093 specification in consolidated catalog |
| 2026-01-03 | 1.1 | Gateway Team | Implementation completed, tests passing |
| 2026-01-15 | 2.0 | Gateway Team | Created standalone BR document per documentation standards |

---

## ✅ **Approval**

**Approved By**: Gateway Team, Architecture Team
**Approval Date**: 2025-11-05
**Implementation Confidence**: 95%
**Production Status**: ✅ Implemented and validated (2026-01-03)

---

**Document Maintained By**: Gateway Service Team
**Last Review**: 2026-01-15
**Next Review**: 2026-04-15 (quarterly review)
