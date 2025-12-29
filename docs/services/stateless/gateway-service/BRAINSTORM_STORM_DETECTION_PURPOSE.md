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

#### **How it would work**:
```
IF storm detected for namespace "prod-payments":
   → Start rejecting NEW signals from that namespace
   → Return HTTP 503 Service Unavailable
   → Emit log: "Circuit breaker tripped for namespace prod-payments"
   → Continue processing existing signals
ELSE:
   → Process normally
```

#### **Business value**:
- ✅ Protects Gateway from DoS via alert floods
- ✅ Ensures other namespaces continue to work (multi-tenant isolation)
- ✅ Prevents K8s API server overload (no CRD creation for storm alerts)

#### **Current state**:
```
❌ NOT IMPLEMENTED

Note: Rate limiting was REMOVED from Gateway (ADR-048)
Reason: "Rate limiting delegated to Ingress/Route proxy"

But this is different:
- Rate limiting: Per-source IP (prevents abuse)
- Circuit breaker: Per-namespace or per-fingerprint (prevents overload)
```

#### **Investigation needed**:
- **Q1**: Does ADR-048 cover per-namespace circuit breaking?
- **Q2**: Is Gateway actually at risk of overload from alert storms?
- **Q3**: What's the actual QPS capacity of Gateway?

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

### Option A: Remove Storm Detection Entirely

**Rationale**:
- Deduplication already provides aggregation
- No downstream consumer uses storm flag
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

**Rationale**:
- Storm detection could protect Gateway from overload
- Per-namespace circuit breaking = multi-tenant isolation
- Would add actual business value

**Implementation**:
```go
if isThresholdReached {
    // Activate circuit breaker for this namespace
    s.circuitBreaker.Trip(signal.Namespace, 5*time.Minute)
    return &ProcessingResponse{
        Status: "circuit_breaker_tripped",
        Message: fmt.Sprintf("Storm detected for %s, circuit breaker active", signal.Namespace),
    }, nil
}
```

**Risk**: Medium - needs capacity planning to determine if Gateway actually needs circuit breaking

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


