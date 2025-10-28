# DD-GATEWAY-003: Redis Outage Risk Tracking Metrics

## Status
**✅ Approved Design** (2025-10-23)
**Last Reviewed**: 2025-10-23
**Confidence**: 90%

## Context & Problem

After implementing Redis HA with Sentinel and request rejection (503) for Redis failures (v2.9), we identified 5 critical risks in `REDIS_FAILURE_HANDLING.md`:

1. **Prolonged Redis outage (>5 min)** → Alert backlog in Prometheus
2. **Prometheus retry exhaustion** → Alerts may be dropped
3. **Redis failover impact** → Service degradation during failover
4. **Sentinel misconfiguration** → Failover capability compromised
5. **Alert backlog accumulation** → System overwhelm after recovery

**Problem**: Current metrics (`gateway_webhook_requests_total`, `gateway_webhook_errors_total`, etc.) don't provide **Redis-specific visibility** needed to track these risks.

**Key Requirements**:
- **BR-GATEWAY-008**: MUST deduplicate (requires Redis availability tracking)
- **BR-GATEWAY-009**: MUST detect storms (requires Redis health monitoring)
- **BR-GATEWAY-010**: MUST persist state (requires Redis connection monitoring)
- **Operational Need**: Track prolonged outages, failover impact, Prometheus retry exhaustion

## Alternatives Considered

### Alternative A: Comprehensive Redis-Specific Metrics (Approved)

**Approach**: Add 15 Redis-specific metrics across 5 categories:
1. **Redis Service Availability**: `gateway_redis_availability_seconds`, `gateway_redis_connection_failures_total`
2. **Request Rejection (503)**: `gateway_requests_rejected_total`, `gateway_consecutive_503_responses`
3. **Prometheus Retry Impact**: `gateway_alerts_queued_estimate`, `gateway_duplicate_prevention_active`
4. **Redis Failover Detection**: `gateway_redis_master_changes_total`, `gateway_redis_failover_duration_seconds`
5. **Business Impact**: `gateway_duplicate_crds_prevented_total`, `gateway_storm_protection_active`

**Pros**:
- ✅ **Complete risk coverage**: All 5 identified risks can be tracked
- ✅ **Actionable alerts**: Clear thresholds for critical/warning alerts
- ✅ **Business value visibility**: Shows cost of Redis failures (duplicates prevented)
- ✅ **Proactive monitoring**: Detects issues before user impact
- ✅ **Grafana dashboard ready**: Metrics designed for visualization
- ✅ **Standard Prometheus patterns**: Uses official Go client, efficient

**Cons**:
- ⚠️ **Metric overhead**: 15 new metrics may impact Gateway performance
  - **Mitigation**: Use efficient Prometheus client, measure impact in staging
- ⚠️ **Alert tuning needed**: Thresholds may need adjustment in production
  - **Mitigation**: Start conservative, tune based on production data
- ⚠️ **Implementation time**: 6.5 hours total effort
  - **Mitigation**: Non-blocking, can be done after v2.9 core features

**Confidence**: 90% (approved)

---

### Alternative B: Minimal Metrics (HTTP Status Codes Only)

**Approach**: Add only `gateway_http_responses_total{status_code}` to track 503 rate

**Pros**:
- ✅ **Minimal overhead**: Single metric, low performance impact
- ✅ **Quick implementation**: 30 minutes
- ✅ **Standard pattern**: HTTP status code metrics are common

**Cons**:
- ❌ **Insufficient risk coverage**: Only tracks 503 rate, not Redis-specific issues
- ❌ **No root cause visibility**: Can't distinguish Redis failures from other 503 causes
- ❌ **No proactive detection**: Only shows problems after they occur
- ❌ **No business impact**: Can't track duplicates prevented, storm protection status
- ❌ **No failover detection**: Can't track Sentinel failover events or duration

**Confidence**: 40% (rejected - insufficient for production operations)

---

### Alternative C: External Monitoring Only (Prometheus Blackbox Exporter)

**Approach**: Use Prometheus Blackbox Exporter to probe Gateway health endpoint, no internal metrics

**Pros**:
- ✅ **No code changes**: External monitoring only
- ✅ **Simple setup**: Standard Prometheus exporter

**Cons**:
- ❌ **No internal visibility**: Can't see Redis-specific failures
- ❌ **No business metrics**: Can't track duplicates prevented, storm protection
- ❌ **Coarse-grained**: Only knows "Gateway up/down", not why
- ❌ **No actionable alerts**: Can't distinguish Redis issues from other failures
- ❌ **No failover detection**: Can't track Sentinel events

**Confidence**: 30% (rejected - insufficient for root cause analysis)

---

## Decision

**APPROVED: Alternative A** - Comprehensive Redis-Specific Metrics

**Rationale**:
1. **Complete Risk Coverage**: All 5 identified risks can be tracked and alerted on
2. **Operational Excellence**: Provides actionable alerts with clear runbooks
3. **Business Value Visibility**: Demonstrates ROI of Redis HA investment
4. **Proactive Monitoring**: Detects issues before user impact (e.g., Sentinel quorum risk)
5. **Standard Patterns**: Uses official Prometheus Go client, follows industry best practices

**Key Insight**: The 6.5-hour implementation cost is justified by the operational visibility gained. Without these metrics, we're "flying blind" during Redis outages and can't validate the 95% confidence we claimed in `REDIS_FAILURE_HANDLING.md`.

## Implementation

**Primary Implementation Files**:
- `pkg/gateway/server/server.go`: Add 15 new Prometheus metrics to `Server` struct and `initMetrics()`
- `pkg/gateway/server/handlers.go`: Record metrics in deduplication/storm detection paths
- `pkg/gateway/processing/redis_health.go` (new): Background Redis health monitoring goroutine
- `deploy/monitoring/gateway-alerts.yaml` (new): Prometheus alert rules (3 critical + 3 warning)
- `deploy/monitoring/gateway-dashboard.json` (new): Grafana dashboard with 6 panels

**Metric Categories**:

### 1. Redis Service Availability (3 metrics)
```go
gateway_redis_availability_seconds{service="deduplication|storm_detection"}
gateway_redis_connection_failures_total{service, error_type}
gateway_redis_operation_errors_total{operation, service, error_type}
```

### 2. Request Rejection (503) (3 metrics)
```go
gateway_requests_rejected_total{reason, service}
gateway_consecutive_503_responses{namespace}
gateway_503_duration_seconds
```

### 3. Prometheus Retry Impact (2 metrics)
```go
gateway_alerts_queued_estimate
gateway_duplicate_prevention_active
```

### 4. Redis Failover Detection (3 metrics)
```go
gateway_redis_master_changes_total
gateway_redis_failover_duration_seconds
gateway_redis_sentinel_health{instance}
```

### 5. Business Impact (2 metrics)
```go
gateway_duplicate_crds_prevented_total
gateway_storm_protection_active
```

**Data Flow**:
1. Background goroutine checks Redis health every 5s → updates `gateway_redis_availability_seconds`
2. Handler detects Redis failure → increments `gateway_requests_rejected_total`, updates `gateway_consecutive_503_responses`
3. Handler succeeds → resets `gateway_consecutive_503_responses`, increments `gateway_duplicate_crds_prevented_total`
4. Prometheus scrapes `/metrics` endpoint every 15s
5. Alert rules evaluate metrics every 30s → fire alerts if thresholds exceeded
6. Grafana dashboard visualizes metrics in real-time

**Graceful Degradation**:
- If Prometheus scraping fails → metrics still recorded in memory (no Gateway impact)
- If metric recording fails → Gateway continues processing (metrics are non-blocking)
- If Redis health check fails → metric shows unavailable, but doesn't block requests

## Consequences

**Positive**:
- ✅ **Complete risk visibility**: All 5 risks tracked with actionable alerts
- ✅ **Proactive operations**: Detect Sentinel quorum issues before failover needed
- ✅ **Business value proof**: Quantify duplicates prevented by Redis HA
- ✅ **Faster incident response**: Clear metrics show root cause (Redis vs. other failures)
- ✅ **Production confidence**: Validate 95% confidence claim from `REDIS_FAILURE_HANDLING.md`
- ✅ **Grafana dashboard**: Real-time visibility for operators

**Negative**:
- ⚠️ **Metric overhead**: 15 new metrics may increase Gateway CPU/memory by ~2-5%
  - **Mitigation**: Measure impact in staging, optimize if needed
- ⚠️ **Alert tuning required**: Initial thresholds may need adjustment
  - **Mitigation**: Start conservative (e.g., 503 rate >5% vs. >1%), tune based on production data
- ⚠️ **Implementation time**: 6.5 hours before production-ready
  - **Mitigation**: Non-blocking, implement after v2.9 core features

**Neutral**:
- 🔄 **Prometheus scrape interval**: May need tuning (default 15s, may need 5s for faster detection)
- 🔄 **Metric retention**: Standard Prometheus retention (15 days default)
- 🔄 **Grafana dashboard maintenance**: Will need updates as Gateway evolves

## Validation Results

**Confidence Assessment Progression**:
- Initial assessment: 85% confidence (concern about metric overhead)
- After analysis: 90% confidence (standard Prometheus patterns, low overhead expected)
- After staging validation: TBD (will measure actual overhead)

**Key Validation Points**:
- ✅ **Metrics cover all 5 risks**: Confirmed in `REDIS_OUTAGE_METRICS.md`
- ✅ **Alert thresholds are actionable**: Based on Redis HA failover times (5-10s)
- ✅ **Grafana panels designed**: 6 panels for complete visibility
- ⏳ **Performance impact measured**: TBD in staging

## Related Decisions
- **Builds On**: DD-GATEWAY-001 (HTTP payload size limit - similar metrics pattern)
- **Supports**: BR-GATEWAY-008 (deduplication), BR-GATEWAY-009 (storm detection), BR-GATEWAY-010 (state persistence)
- **Related To**: Redis HA implementation (v2.9), Request rejection strategy (503)
- **Documented In**:
  - [REDIS_OUTAGE_METRICS.md](../../services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md)
  - [REDIS_FAILURE_HANDLING.md](../../services/stateless/gateway-service/REDIS_FAILURE_HANDLING.md)
  - [IMPLEMENTATION_PLAN_V2.10.md](../../services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.10.md)

## Review & Evolution

**When to Revisit**:
- If metric overhead >5% CPU/memory (may need to reduce metric count)
- If alert thresholds cause too many false positives (>10/day)
- If Grafana dashboard doesn't provide actionable insights
- After 1 month of production metrics (validate thresholds)

**Success Metrics**:
- **Metric Overhead**: <5% CPU/memory increase (Target: <2%)
- **Alert Accuracy**: <5 false positives/day (Target: <2/day)
- **MTTR Improvement**: 50% faster incident resolution with metrics (Target: 30min → 15min)
- **Risk Detection**: 100% of prolonged Redis outages (>5 min) detected (Target: 100%)


