# Redis Outage Risk Tracking - Metrics Analysis

## 📊 **Current Metrics Gap Analysis**

### **Existing Metrics** (server.go:124-151)

```go
// ✅ HAVE: Basic request metrics
gateway_webhook_requests_total       // Total requests
gateway_webhook_errors_total         // Total errors (all types)
gateway_crd_creation_total          // Successful CRD creations
gateway_webhook_processing_seconds  // Processing latency
```

### **❌ MISSING: Redis Outage Risk Metrics**

We need these metrics to track the risks identified in `REDIS_FAILURE_HANDLING.md`:

| Risk | Metric Needed | Current Status |
|------|---------------|----------------|
| **Prolonged Redis outage (>5 min)** | Redis availability duration | ❌ Missing |
| **Alert backlog in Prometheus** | 503 response rate | ❌ Missing |
| **Prometheus retry exhaustion** | Consecutive 503 count | ❌ Missing |
| **Redis failover impact** | Redis connection failures | ❌ Missing |
| **Sentinel misconfiguration** | Failover duration | ❌ Missing |

---

## 🎯 **Required Metrics for Risk Tracking**

### **1. Redis Service Availability Metrics**

```go
// Track Redis service health over time
gateway_redis_availability_seconds{service="deduplication|storm_detection"}
  Type: Gauge
  Labels: service (deduplication, storm_detection)
  Purpose: Track continuous Redis availability
  Alert: If gauge shows >300s (5 min) unavailability

gateway_redis_connection_failures_total{service="deduplication|storm_detection"}
  Type: Counter
  Labels: service, error_type
  Purpose: Count Redis connection failures
  Alert: If rate > 10/min for >1 min

gateway_redis_operation_errors_total{operation="check|record|increment",service="deduplication|storm_detection"}
  Type: Counter
  Labels: operation, service, error_type
  Purpose: Track specific Redis operation failures
  Alert: If rate > 5/min for specific operation
```

### **2. Request Rejection Metrics (503)**

```go
// Track 503 responses and their causes
gateway_requests_rejected_total{reason="redis_unavailable",service="deduplication|storm_detection"}
  Type: Counter
  Labels: reason, service
  Purpose: Count 503 responses by cause
  Alert: If rate > 1/sec for >30s

gateway_consecutive_503_responses{namespace=""}
  Type: Gauge
  Labels: namespace (optional)
  Purpose: Track consecutive 503s (indicates prolonged outage)
  Alert: If gauge > 10 (indicates Prometheus retry exhaustion risk)

gateway_503_duration_seconds
  Type: Histogram
  Purpose: Track how long 503 periods last
  Alert: If p95 > 60s (indicates failover issues)
```

### **3. Prometheus Retry Impact Metrics**

```go
// Track potential alert backlog
gateway_alerts_queued_estimate
  Type: Gauge
  Purpose: Estimate alerts waiting in Prometheus (based on 503 rate)
  Calculation: consecutive_503s * prometheus_retry_interval
  Alert: If estimate > 100 alerts

gateway_duplicate_prevention_active
  Type: Gauge (0 or 1)
  Purpose: Boolean: Can we guarantee zero duplicates?
  Calculation: redis_available && dedup_service_healthy
  Alert: If gauge == 0 for >5 min
```

### **4. Redis Failover Detection Metrics**

```go
// Track Redis master changes (Sentinel failover)
gateway_redis_master_changes_total
  Type: Counter
  Purpose: Count Redis master failovers
  Alert: If rate > 1/hour (indicates instability)

gateway_redis_failover_duration_seconds
  Type: Histogram
  Purpose: Track how long failovers take
  Alert: If p95 > 15s (exceeds expected 5-10s)

gateway_redis_sentinel_health{instance="redis-0|redis-1|redis-2"}
  Type: Gauge (0 or 1)
  Labels: instance
  Purpose: Track individual Sentinel health
  Alert: If sum < 2 (quorum at risk)
```

### **5. Business Impact Metrics**

```go
// Track business outcomes during Redis issues
gateway_duplicate_crds_prevented_total
  Type: Counter
  Purpose: Count how many duplicates we prevented by rejecting (503)
  Calculation: Increment on every 503 due to Redis unavailable
  Business Value: Shows cost of NOT having Redis HA

gateway_storm_protection_active
  Type: Gauge (0 or 1)
  Purpose: Boolean: Is storm protection working?
  Calculation: redis_available && storm_detector_healthy
  Alert: If gauge == 0 for >1 min
```

---

## 🚨 **Alerting Rules (Prometheus)**

### **Critical Alerts** (Page immediately)

```yaml
# Alert: Redis unavailable for >5 minutes
- alert: GatewayRedisOutageProlonged
  expr: gateway_redis_availability_seconds > 300
  for: 1m
  severity: critical
  annotations:
    summary: "Gateway Redis unavailable for >5 minutes"
    description: "Redis has been unavailable for {{ $value }}s. Alert backlog risk."
    runbook: "Check Redis HA status, verify Sentinel failover"

# Alert: High 503 rate (>5% of requests)
- alert: Gateway503RateHigh
  expr: rate(gateway_requests_rejected_total[1m]) / rate(gateway_webhook_requests_total[1m]) > 0.05
  for: 30s
  severity: critical
  annotations:
    summary: "Gateway rejecting >5% of requests (503)"
    description: "{{ $value | humanizePercentage }} of requests rejected due to Redis issues"
    runbook: "Check Redis HA status, verify Sentinel quorum"

# Alert: Consecutive 503s indicate Prometheus retry exhaustion risk
- alert: GatewayPrometheusRetryExhaustion
  expr: gateway_consecutive_503_responses > 10
  for: 1m
  severity: critical
  annotations:
    summary: "Gateway consecutive 503s > 10 (Prometheus retry risk)"
    description: "{{ $value }} consecutive 503s. Alerts may be dropped."
    runbook: "Immediate Redis recovery required"
```

### **Warning Alerts** (Investigate soon)

```yaml
# Alert: Redis failover detected
- alert: GatewayRedisFailover
  expr: increase(gateway_redis_master_changes_total[5m]) > 0
  for: 1m
  severity: warning
  annotations:
    summary: "Redis master failover detected"
    description: "Redis master changed {{ $value }} times in last 5 minutes"
    runbook: "Verify failover successful, check Sentinel logs"

# Alert: Redis connection failures
- alert: GatewayRedisConnectionFailures
  expr: rate(gateway_redis_connection_failures_total[1m]) > 1
  for: 2m
  severity: warning
  annotations:
    summary: "Gateway experiencing Redis connection failures"
    description: "{{ $value }} connection failures/sec"
    runbook: "Check Redis pod health, network connectivity"

# Alert: Sentinel quorum at risk
- alert: GatewaySentinelQuorumRisk
  expr: sum(gateway_redis_sentinel_health) < 2
  for: 1m
  severity: warning
  annotations:
    summary: "Redis Sentinel quorum at risk (<2 healthy)"
    description: "Only {{ $value }} Sentinels healthy. Failover capability compromised."
    runbook: "Check Sentinel pod status, restart unhealthy instances"
```

---

## 📈 **Grafana Dashboard Panels**

### **Panel 1: Redis Availability Overview**

```
Title: Redis Service Availability
Type: Stat
Query: gateway_redis_availability_seconds
Thresholds:
  - Green: 0-10s (healthy)
  - Yellow: 10-60s (degraded)
  - Red: >60s (critical)
```

### **Panel 2: 503 Response Rate**

```
Title: Request Rejection Rate (503)
Type: Graph
Query: rate(gateway_requests_rejected_total[1m]) / rate(gateway_webhook_requests_total[1m]) * 100
Y-Axis: Percentage (0-100%)
Alert Line: 5% (critical threshold)
```

### **Panel 3: Consecutive 503 Tracker**

```
Title: Consecutive 503 Responses (Prometheus Retry Risk)
Type: Gauge
Query: gateway_consecutive_503_responses
Thresholds:
  - Green: 0-5 (normal)
  - Yellow: 5-10 (monitor)
  - Red: >10 (critical - retry exhaustion risk)
```

### **Panel 4: Redis Failover History**

```
Title: Redis Master Failovers (Last 24h)
Type: Stat
Query: increase(gateway_redis_master_changes_total[24h])
Description: "Expected: 0 (stable), >3 indicates instability"
```

### **Panel 5: Duplicate Prevention Status**

```
Title: Duplicate Prevention Active
Type: Stat (Boolean)
Query: gateway_duplicate_prevention_active
Thresholds:
  - Green: 1 (active)
  - Red: 0 (inactive - data integrity at risk)
```

### **Panel 6: Estimated Alert Backlog**

```
Title: Estimated Prometheus Alert Backlog
Type: Graph
Query: gateway_alerts_queued_estimate
Description: "Estimated alerts waiting in Prometheus retry queue"
Alert Line: 100 (high backlog risk)
```

---

## 🔧 **Implementation Plan**

### **Phase 1: Core Redis Metrics** (2 hours)

**File**: `pkg/gateway/server/server.go`

Add to `Server` struct:
```go
// Redis health metrics (v2.9+)
redisAvailabilitySeconds      *prometheus.GaugeVec
redisConnectionFailuresTotal  *prometheus.CounterVec
redisOperationErrorsTotal     *prometheus.CounterVec
requestsRejectedTotal         *prometheus.CounterVec
consecutive503Responses       *prometheus.GaugeVec
```

Add to `initMetrics()`:
```go
s.redisAvailabilitySeconds = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "gateway_redis_availability_seconds",
        Help: "Seconds Redis has been unavailable (0 = healthy)",
    },
    []string{"service"}, // deduplication, storm_detection
)
s.registry.MustRegister(s.redisAvailabilitySeconds)

// ... (similar for other metrics)
```

### **Phase 2: Metric Recording in Handlers** (1 hour)

**File**: `pkg/gateway/server/handlers.go`

Update deduplication check:
```go
isDuplicate, _, err := s.dedupService.Check(ctx, signal)
if err != nil {
    // Record Redis failure metrics
    s.redisConnectionFailuresTotal.WithLabelValues("deduplication", "check_failed").Inc()
    s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
    s.increment503Counter(signal.Namespace) // Track consecutive 503s

    s.respondError(w, http.StatusServiceUnavailable,
        "deduplication service unavailable", requestID, err)
    return
}
// Reset consecutive 503 counter on success
s.reset503Counter(signal.Namespace)
```

### **Phase 3: Redis Health Monitoring** (2 hours)

**File**: `pkg/gateway/processing/redis_health.go` (new)

Create background health checker:
```go
// StartRedisHealthMonitor monitors Redis availability
func (s *Server) StartRedisHealthMonitor(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    unavailableSince := make(map[string]time.Time)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Check deduplication service
            if err := s.dedupService.HealthCheck(ctx); err != nil {
                if _, exists := unavailableSince["deduplication"]; !exists {
                    unavailableSince["deduplication"] = time.Now()
                }
                duration := time.Since(unavailableSince["deduplication"]).Seconds()
                s.redisAvailabilitySeconds.WithLabelValues("deduplication").Set(duration)
            } else {
                delete(unavailableSince, "deduplication")
                s.redisAvailabilitySeconds.WithLabelValues("deduplication").Set(0)
            }

            // Similar for storm_detection
        }
    }
}
```

### **Phase 4: Prometheus Alerts** (30 min)

**File**: `deploy/monitoring/gateway-alerts.yaml` (new)

Create Prometheus alert rules (see Alerting Rules section above).

### **Phase 5: Grafana Dashboard** (1 hour)

**File**: `deploy/monitoring/gateway-dashboard.json` (new)

Create Grafana dashboard with panels (see Grafana Dashboard Panels section above).

---

## 🎯 **Success Criteria**

After implementation, we can track:

1. ✅ **Prolonged Redis outage risk**: `gateway_redis_availability_seconds` > 300s
2. ✅ **Alert backlog risk**: `gateway_alerts_queued_estimate` > 100
3. ✅ **Prometheus retry exhaustion**: `gateway_consecutive_503_responses` > 10
4. ✅ **Redis failover impact**: `gateway_redis_failover_duration_seconds` p95
5. ✅ **Sentinel misconfiguration**: `sum(gateway_redis_sentinel_health)` < 2

---

## 📊 **Confidence Assessment**

**Confidence**: **90% ✅**

**Why 90%?**
- ✅ Metrics directly address identified risks
- ✅ Alerting rules provide actionable thresholds
- ✅ Grafana dashboard enables real-time monitoring
- ✅ Implementation plan is straightforward (6.5 hours total)

**Remaining 10% Risk**:
- ⚠️ Metric overhead impact on Gateway performance (need to measure)
- ⚠️ Prometheus alert rule tuning may be needed in production

**Mitigation**:
- Use efficient Prometheus client (already using official Go client)
- Start with conservative alert thresholds, tune based on production data
- Monitor Gateway CPU/memory impact after metric addition

---

## 🔗 **Related Documents**

- [REDIS_FAILURE_HANDLING.md](./REDIS_FAILURE_HANDLING.md) - Strategy document (identified risks)
- [deploy/redis-ha/README.md](../../../deploy/redis-ha/README.md) - Redis HA operations
- [IMPLEMENTATION_PLAN_V2.9.md](./IMPLEMENTATION_PLAN_V2.9.md) - Current implementation plan

---

## 📝 **Next Steps**

1. **Approve metrics design** (this document)
2. **Implement Phase 1-3** (core metrics + health monitoring)
3. **Deploy to staging** (validate metric collection)
4. **Implement Phase 4-5** (alerts + dashboard)
5. **Monitor in production** (tune alert thresholds)
6. **Document runbooks** (how to respond to alerts)

**Estimated Total Time**: 6.5 hours
**Priority**: High (enables risk tracking for Redis HA)
**Blocking**: No (can be implemented after v2.9 core features)


