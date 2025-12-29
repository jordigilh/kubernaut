# Brainstorm: What IS Storm Detection For?

**Date**: December 13, 2025
**Status**: 🤔 **OPEN QUESTION** - Business value unclear
**Priority**: FOUNDATIONAL - May reveal architectural gaps or unnecessary complexity

---

## 🎯 The Core Question

After determining that storm context has **minimal value for LLM RCA** (3-6%), we must ask:

**"What is the actual business purpose of storm detection in Gateway?"**

---

## 📊 Current State Analysis

### What Storm Detection Currently Does

```go
// pkg/gateway/server.go
isThresholdReached := occurrenceCount >= s.stormThreshold  // Default: 5

if isThresholdReached {
    s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues("rate", signal.AlertName).Inc()
}

// Async status update
go func() {
    if err := s.statusUpdater.UpdateStormAggregationStatus(ctx, rrCopy, isThresholdReached); err != nil {
        s.logger.Info("Failed to update storm aggregation status (async, DD-GATEWAY-013)",
            "error", err,
            "fingerprint", signal.Fingerprint)
    }
}()
```

**Actions Taken**:
1. ✅ Increments Prometheus metric `AlertStormsDetectedTotal`
2. ✅ Updates `RemediationRequest.status.stormAggregation.isStorm = true`
3. ❌ **NOTHING ELSE** - No routing changes, no backpressure, no circuit breaking

---

### Business Requirements for Storm Detection

From `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`:

#### **BR-GATEWAY-008: Storm Detection**
> Gateway must detect alert storms (>10 alerts/minute) and aggregate them

**Implementation**: `pkg/gateway/processing/storm_detector.go`
**Status**: ✅ Implemented (but aggregation unclear)

#### **BR-GATEWAY-009: Concurrent Storm Detection**
> Gateway must handle concurrent alert bursts without race conditions

**Implementation**: Concurrent-safe storm detection
**Status**: ✅ Implemented

#### **BR-GATEWAY-010: Storm State Recovery**
> Gateway must recover storm state from Redis after restart

**Implementation**: Storm state in K8s CRD status (not Redis)
**Status**: ✅ Implemented (via DD-GATEWAY-011)

---

### Key Findings

1. **Business requirements say "aggregate storms"** but current implementation only TRACKS storms (no aggregation behavior)
2. **No downstream consumer** of `status.stormAggregation.isStorm` (RO doesn't use it, AIAnalysis doesn't see it)
3. **Only observable output** is a Prometheus metric

---

## 🤔 Potential Business Values (Brainstorm)

### Option 1: Circuit Breaker for Gateway Overload Protection

**Hypothesis**: Storm detection prevents Gateway from being overwhelmed by alert floods.

#### **CRITICAL ARCHITECTURAL ISSUE**:

**Current storm detection = per-fingerprint** (too granular for circuit breaking):
```
❌ WRONG: Per-fingerprint storm detection
Storm detected for SHA256("PodNotReady:prod:Pod:app-pod-1")
→ Reject only alerts for this ONE specific pod?
→ Doesn't protect Gateway from overload (other pods still creating load)
```

**Proper circuit breaker = service-level** (protects Gateway itself):
```
✅ CORRECT: Service-level circuit breaker
Total QPS > threshold (e.g., 1000 req/s)
→ Reject ALL new requests with HTTP 503
→ Protects Gateway pod from OOM/CPU exhaustion
→ Load balancer routes traffic to other pods
```

#### **How a REAL circuit breaker would work**:
```go
// Service-level protection (NOT per-fingerprint)
type CircuitBreaker struct {
    qpsThreshold int       // e.g., 1000 req/s
    errorRate    float64   // e.g., 50% errors triggers open
    state        State     // Closed, Open, HalfOpen
}

func (cb *CircuitBreaker) Allow() bool {
    if cb.state == Open {
        return false  // Reject ALL requests
    }

    currentQPS := cb.metrics.GetCurrentQPS()
    if currentQPS > cb.qpsThreshold {
        cb.state = Open
        return false
    }

    return true
}
```

#### **Current state**:
```
❌ NO CIRCUIT BREAKER EXISTS

Gateway has:
- ✅ Rate limiting delegated to proxy (ADR-048)
- ✅ Retry logic for K8s API errors (exponential backoff)
- ❌ NO service-level circuit breaker
- ❌ NO per-namespace circuit breaker
- ❌ NO overload protection

Storm detection does NOT provide circuit breaking:
- Storm = per-fingerprint tracking (single resource flapping)
- Circuit breaker = service-level protection (Gateway overload)
```

#### **Key distinctions**:

| Mechanism | Scope | Purpose | Status |
|-----------|-------|---------|--------|
| **Rate limiting** | Per-source IP | Prevent abuse | ✅ Delegated to proxy (ADR-048) |
| **Storm detection** | Per-fingerprint | Track resource flapping | ✅ Implemented (but no action taken) |
| **Circuit breaker** | Service-level | Protect Gateway from overload | ❌ NOT IMPLEMENTED |

#### **Investigation needed**:
- **Q1**: Is Gateway at risk of overload? (QPS capacity analysis)
- **Q2**: Should we add service-level circuit breaker?
- **Q3**: If yes, storm detection is NOT the right mechanism (too granular)

---

### Option 2: Observability Signal for SRE Teams

**Hypothesis**: Storm detection is purely for SRE observability, not automation.

#### **How it works (current)**:
```
Storm detected → Prometheus metric incremented
                → Grafana dashboard shows spike
                → SRE team investigates manually
```

#### **Business value**:
- ✅ SREs can see when a resource is flapping
- ✅ Identifies problematic deployments/nodes
- ✅ Tracks storm frequency over time

#### **Current state**:
```
✅ IMPLEMENTED
Metric: alert_storms_detected_total{storm_type="rate",alert_name="..."}
```

#### **Problems**:
- ❌ Unclear if SREs actually use this metric
- ❌ Duplicate signal: `occurrence_count` already shows flapping
- ❌ If observability is the only value, why the complexity?

---

### Option 3: Aggregation for Reduced Downstream Load

**Hypothesis**: Storm detection aggregates multiple alerts into a single CRD to reduce downstream processing load.

#### **How it would work**:
```
WITHOUT storm aggregation:
  Alert 1-20 → 20 separate CRDs → 20 SignalProcessing → 20 AIAnalysis → 20 WorkflowExecutions

WITH storm aggregation:
  Alert 1-5  → 5 separate CRDs → 5 SignalProcessing → 5 AIAnalysis → 5 WorkflowExecutions
  Alert 6-20 → UPDATE existing CRD (no new CRDs) → Reduced load
```

#### **Business value**:
- ✅ Reduces K8s API server load (fewer CRD creates)
- ✅ Reduces downstream service load (SP, AA, WE)
- ✅ Faster processing (no redundant enrichment/analysis)

#### **Current state**:
```
❌ NOT FULLY IMPLEMENTED

Current behavior:
  Alert 1 → Create CRD (occurrenceCount=1)
  Alert 2 → Update CRD (occurrenceCount=2) ✅ Deduplication works!
  Alert 5 → Update CRD (occurrenceCount=5, isStorm=true) ✅ Storm flag set!
  Alert 6-20 → Update CRD (occurrenceCount=6...20) ✅ Still deduplicated!

Result: Aggregation ALREADY HAPPENS via deduplication!
        Storm flag adds NO additional aggregation behavior!
```

#### **Key insight**:
**Deduplication already aggregates storms!**
- Same fingerprint → Same CRD → Occurrence count increases
- Storm flag is just a threshold indicator, not an aggregation mechanism

---

### Option 4: Workflow Routing Signal (Already Disproven)

**Hypothesis**: RO uses storm flag to skip AIAnalysis for storms.

**Status**: ❌ **IMPOSSIBLE** (see Decision Document)
- RO makes routing decision before storm threshold is reached
- RO cannot make remediation decisions (no workflow selection capability)

---

### Option 5: Operator Alert for Manual Intervention

**Hypothesis**: Storm detection triggers operator notifications for manual triage.

#### **How it would work**:
```
IF isStorm == true:
   → Send Slack/PagerDuty notification
   → Alert: "Storm detected for PodNotReady in prod-payments (20 occurrences)"
   → Operator manually investigates root cause
   → Operator decides to:
      a) Let remediation proceed
      b) Manually fix infrastructure
      c) Silence alerts
```

#### **Business value**:
- ✅ Human escalation for severe issues
- ✅ Prevents automated remediation for dangerous scenarios
- ✅ Enables manual root cause analysis

#### **Current state**:
```
❌ NOT IMPLEMENTED

No notification logic exists for storm detection.
Only Prometheus metric exists.
```

#### **Problems**:
- ❌ This overlaps with existing alerting (Prometheus Alertmanager)
- ❌ Why not just alert on `occurrence_count >= 5` in Prometheus?
- ❌ Gateway shouldn't be in the notification business

---

## 🔍 Investigation: What Do Other Systems Do?

### Prometheus Alertmanager: Alert Grouping

```yaml
# Alertmanager groups similar alerts to reduce notification noise
route:
  group_by: ['alertname', 'namespace', 'pod']
  group_wait: 30s
  group_interval: 5m

# Result: Multiple PodNotReady alerts → Single grouped notification
```

**Insight**: Alertmanager already handles storm aggregation for notifications.

---

### Kubernetes Event Rate Limiting

Kubernetes API server has built-in event rate limiting:
- Per-source rate limits
- Event deduplication (similar events aggregated)
- Prevents event storms from overloading API server

**Insight**: K8s already has infrastructure-level storm protection.

---

### AWS CloudWatch: Alarm Throttling

CloudWatch suppresses repeat alarm notifications:
- Alarm enters ALARM state → Send notification
- Alarm stays in ALARM state → Suppress repeat notifications
- Alarm clears → Send OK notification

**Insight**: Storm suppression is for NOTIFICATIONS, not processing.

---

## 🎯 Hypothesis: Storm Detection is Redundant

### Evidence

1. **Deduplication already aggregates**:
   - Same fingerprint → Same CRD → Occurrence count increases
   - No additional CRD creation for storm alerts
   - Storm flag adds nothing beyond `occurrenceCount >= threshold`

2. **No downstream consumer**:
   - RO doesn't use storm flag (routing happens before storm)
   - AIAnalysis doesn't see storm flag (created before storm threshold)
   - WorkflowExecution doesn't know about storms

3. **Observability duplication**:
   - Storm metric = `occurrence_count >= 5`
   - Could be replaced with Prometheus query: `count(occurrence_count >= 5)`

4. **Rate limiting removed**:
   - ADR-048: Rate limiting delegated to proxy
   - Storm detection might have been intended for this, but it's now redundant

---

## 💡 Recommendation Options

### Option A: Remove Storm Detection Entirely ✅ RECOMMENDED

**Status**: ✅ **APPROVED** - See [DD-GATEWAY-015: Storm Detection Logic Removal](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)

**Rationale**:
- Deduplication already provides aggregation
- No downstream consumer uses storm flag (DD-AIANALYSIS-004)
- Observability can be achieved via Prometheus query on `occurrence_count`
- Simpler codebase, less maintenance

**Impact**:
```diff
- status.stormAggregation (entire field)
- pkg/gateway/processing/status_updater.go UpdateStormAggregationStatus()
- test/integration/gateway/webhook_integration_test.go storm test
- docs references to storm detection
```

**Risk**: Low - no known consumer of storm flag

---

### Option B: Repurpose Storm Detection as Circuit Breaker

**Status**: ❌ **ARCHITECTURALLY INCORRECT**

**Problem**: Storm detection is per-fingerprint, circuit breakers should be service-level.

**Decision**: See [DD-GATEWAY-014: Service-Level Circuit Breaker Deferral](../architecture/decisions/DD-GATEWAY-014-circuit-breaker-deferral.md) for authoritative decision on Gateway circuit breaker.

**Why this doesn't work**:
```
Storm detection = per-fingerprint
  → Tracks: SHA256("PodNotReady:prod:Pod:app-pod-1")
  → Granularity: Single specific resource
  → Protection: None (other fingerprints still create load)

Circuit breaker = service-level
  → Tracks: Total Gateway QPS, memory, error rate
  → Granularity: Entire Gateway service
  → Protection: Prevents Gateway pod from OOM/crash
```

**If we wanted circuit breaking**, we'd need NEW functionality:
```go
// NEW: Service-level circuit breaker (NOT storm detection)
type GatewayCircuitBreaker struct {
    state State
    qpsThreshold int
    memoryThreshold float64
}

func (gcb *GatewayCircuitBreaker) CheckOverload() bool {
    currentQPS := metrics.GetQPS()
    memoryUsage := metrics.GetMemoryPercent()

    if currentQPS > gcb.qpsThreshold || memoryUsage > gcb.memoryThreshold {
        gcb.state = Open
        return true  // Gateway is overloaded
    }
    return false
}
```

**Recommendation**: **DO NOT repurpose storm detection for circuit breaking**
- Storm detection operates at wrong granularity (per-fingerprint vs service-level)
- If circuit breaking is needed, implement it separately
- Circuit breaker monitors Gateway health, not individual fingerprints

**Risk**: High - architectural mismatch between storm detection and circuit breaking

---

### Option C: Keep as Observability Metric Only

**Rationale**:
- Storm metric might be useful for SRE teams
- Low cost to maintain (already implemented)
- No harm in keeping if observability value exists

**Implementation**: Keep current code, document as observability-only

**Risk**: Low - but adds complexity for minimal value

---

## 🔎 Questions to Answer

Before deciding on Option A/B/C:

1. **Capacity Analysis**:
   - Q: What is Gateway's actual QPS capacity?
   - Q: Has Gateway ever been overloaded in production?
   - Q: Is per-namespace circuit breaking needed?

2. **Observability Analysis**:
   - Q: Do SRE teams use `alert_storms_detected_total` metric?
   - Q: Could this be replaced with Prometheus query on `occurrence_count`?
   - Q: Is there value in a separate storm metric?

3. **Historical Context**:
   - Q: Why was storm detection originally implemented?
   - Q: Was it intended for rate limiting (now removed via ADR-048)?
   - Q: Was it intended for aggregation (already handled by deduplication)?

4. **Downstream Dependencies**:
   - Q: Does ANY service read `status.stormAggregation`?
   - Q: Are there any future plans to use storm flag?
   - Q: Is this being kept for a future feature?

---

## 📋 Action Items

1. **Investigate Gateway capacity**: Determine if circuit breaking is needed
2. **Check metric usage**: Query Grafana/Prometheus to see if storm metric is used
3. **Review ADR-048**: Confirm rate limiting delegation covers storm scenarios
4. **Consult SRE team**: Ask if storm detection has observability value
5. **Make decision**: Keep, repurpose, or remove storm detection

---

## 🔗 Related Documents

- **[DECISION_STORM_CONTEXT_NOT_EXPOSED.md](../../crd-controllers/02-aianalysis/DECISION_STORM_CONTEXT_NOT_EXPOSED.md)** - Storm context not exposed to LLM
- **[ADR-048](../../../architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md)** - Rate limiting delegated to proxy
- **[DD-GATEWAY-011](../../../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)** - Storm state in CRD status
- **[BR-GATEWAY-008](./BUSINESS_REQUIREMENTS.md)** - Storm detection business requirement

---

**Document Status**: 🤔 Open Question - Needs Investigation
**Priority**: Medium - Not blocking, but architectural clarity needed
**Next Step**: Answer investigation questions, make decision on Option A/B/C

