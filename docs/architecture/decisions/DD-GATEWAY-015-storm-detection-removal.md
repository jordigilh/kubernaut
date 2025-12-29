# DD-GATEWAY-015: Storm Detection Logic Removal

**Date**: December 13, 2025
**Status**: 📋 PLANNED for Implementation
**Deciders**: Gateway Team, Architecture Team
**Confidence**: 90%
**Related**: DD-GATEWAY-011, DD-GATEWAY-012, DD-GATEWAY-008, DD-AIANALYSIS-004, DD-GATEWAY-014, BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010

---

## Context & Problem

### Problem Statement

Storm detection in Gateway tracks resource-specific persistence (`occurrenceCount >= threshold`) and sets a boolean flag (`status.stormAggregation.isPartOfStorm`). After comprehensive analysis, **storm detection provides NO business value**:

1. **Redundant with Deduplication**: `status.deduplication.occurrenceCount` already tracks persistence
2. **No Downstream Consumers**: AI Analysis does NOT expose storm context to LLM (DD-AIANALYSIS-004)
3. **No Workflow Routing**: Remediation Orchestrator does NOT route differently for storms
4. **Boolean Flag = Zero Added Value**: Storm is just `occurrenceCount >= 5` (threshold)
5. **Observability via Existing Metrics**: Can query `occurrence_count >= 5` in Prometheus

### Current Storm Detection Implementation

**Architecture** (as of DD-GATEWAY-012):
```
Signal arrives → Deduplication check
↓
IF existing RR:
  1. Update status.deduplication.occurrenceCount
  2. Check: occurrenceCount >= stormThreshold (default: 5)
  3. IF threshold reached:
     - Set status.stormAggregation.isPartOfStorm = true
     - Increment status.stormAggregation.aggregatedCount
     - Emit gateway_alert_storms_detected_total metric
     - Emit audit event "gateway.storm.detected"
```

**Code Footprint**:
- 5 source files: `types.go`, `config.go`, `server.go`, `status_updater.go`, `metrics.go`
- 3 test files: unit + integration + E2E
- 4 business requirements: BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010, BR-GATEWAY-070
- 2 design decisions: DD-GATEWAY-008, DD-GATEWAY-012
- CRD schema: `status.stormAggregation` field
- Metrics: `gateway_alert_storms_detected_total`
- Audit events: `gateway.storm.detected`

### Key Requirements

**Why Storm Detection Was Originally Implemented**:
- **BR-GATEWAY-008**: Detect alert storms (>10 alerts/minute) and aggregate them
- **BR-GATEWAY-009**: Concurrent storm detection across multiple signals
- **BR-GATEWAY-010**: Storm state recovery from Redis after restart

**Original Intent**: Aggregate multiple alerts into single storm CRD to reduce downstream load.

**Reality per DD-GATEWAY-012**: Storm detection became **status tracking only** (not CRD aggregation).
- Gateway creates **1 CRD per fingerprint** (deduplication)
- Storm is just a **boolean flag** based on `occurrenceCount >= 5`
- No CRD count reduction (deduplication already does that)

---

## Alternatives Considered

### Alternative 1: Remove Storm Detection Entirely (RECOMMENDED)

**Approach**: Delete all storm detection code, tests, configuration, and documentation.

**Rationale**:
1. ✅ **Deduplication already provides aggregation** (1 CRD per fingerprint with `occurrenceCount`)
2. ✅ **No downstream consumers** (AI Analysis doesn't use storm flag per DD-AIANALYSIS-004)
3. ✅ **Observability via existing metrics** (query `occurrence_count >= 5` in Prometheus)
4. ✅ **Simpler codebase** (less maintenance, fewer bugs, clearer intent)
5. ✅ **No business value** (storm = boolean based on existing `occurrenceCount`)

**Components to Remove**:

**1. Source Code** (5 files):
```
pkg/gateway/types/types.go
  - Remove: IsStorm, StormType, StormWindow, AlertCount, AffectedResources fields

pkg/gateway/config/config.go
  - Remove: StormSettings struct (entire struct)
  - Remove: cfg.Processing.Storm references

pkg/gateway/server.go
  - Remove: stormThreshold field
  - Remove: Async storm status update logic (lines 852-867)
  - Remove: isThresholdReached calculation (lines 845-850)
  - Remove: AlertStormsDetectedTotal metric call (lines 871-872)
  - Remove: emitStormDetectedAudit() method (lines 1245-1284)

pkg/gateway/processing/status_updater.go
  - Remove: UpdateStormAggregationStatus() method (lines 108-153)

pkg/gateway/metrics/metrics.go
  - Remove: AlertStormsDetectedTotal metric
```

**2. Tests** (3 files):
```
test/unit/gateway/storm_aggregation_status_test.go - DELETE FILE
test/unit/gateway/storm_detection_test.go - DELETE FILE
test/e2e/gateway/01_storm_buffering_test.go - DELETE FILE

test/integration/gateway/webhook_integration_test.go
  - Remove: "BR-GATEWAY-013: Storm Detection" test case
```

**3. CRD Schema** (2 files):
```
api/remediation/v1alpha1/remediationrequest_types.go
  - Remove: StormAggregationStatus struct
  - Remove: status.StormAggregation field

config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml
  - Remove: status.stormAggregation schema definition
```

**4. Configuration**:
```
pkg/gateway/config/testdata/valid-config.yaml
  - Remove: storm: {} section
```

**5. Documentation** (update, not remove):
```
docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md
  - Mark BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010, BR-GATEWAY-070 as REMOVED

docs/architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md
  - Add SUPERSEDED notice pointing to DD-GATEWAY-015

docs/architecture/decisions/DD-GATEWAY-012-redis-free-storm-detection.md
  - Add SUPERSEDED notice pointing to DD-GATEWAY-015
```

**Pros**:
- ✅ Simpler codebase (removes ~500 lines of code)
- ✅ Fewer tests to maintain (removes 3 test files)
- ✅ Clearer intent (`occurrenceCount` is self-explanatory)
- ✅ No breaking changes for downstream (storm flag was never used)
- ✅ Observability preserved (Prometheus can query `occurrence_count >= 5`)
- ✅ Lower maintenance burden
- ✅ Fewer CRD schema changes going forward

**Cons**:
- ❌ Loss of explicit "storm detected" event (but can derive from `occurrenceCount`)
- ❌ No storm-specific metrics (but can query `occurrence_count >= 5`)
- ❌ Loss of historical design context (mitigated by DD-GATEWAY-015 documentation)

**Risk Assessment**: **LOW** - No downstream consumers, no breaking changes

---

### Alternative 2: Keep Storm Detection for Observability (Not Recommended)

**Approach**: Retain storm detection as observability-only feature (metrics + logs).

**Rationale**:
- Storm-specific metrics (`gateway_alert_storms_detected_total`) provide explicit signal
- Audit events (`gateway.storm.detected`) enable storm tracking in dashboards
- SRE teams may find storm flag useful for alerting

**Pros**:
- ✅ Preserves explicit storm observability
- ✅ No code deletion risk
- ✅ SRE teams have dedicated storm metrics

**Cons**:
- ❌ Maintains unnecessary complexity (storm = `occurrenceCount >= 5`)
- ❌ CRD schema remains bloated
- ❌ Tests require maintenance
- ❌ Same observability achievable via Prometheus query: `occurrence_count >= 5`

**Evaluation**: Rejected - observability can be achieved without dedicated storm code.

---

### Alternative 3: Repurpose Storm Detection as Circuit Breaker (Rejected)

**Approach**: Convert storm detection to service-level circuit breaker.

**Status**: ❌ **ARCHITECTURALLY INCORRECT**

**Problem**: Storm detection is **per-fingerprint** (single resource tracking), circuit breakers should be **service-level** (entire Gateway service protection).

**Evaluation**: Rejected - see DD-GATEWAY-014 for circuit breaker decision (deferred to production monitoring).

---

## Decision

**CHOSEN**: **Alternative 1 - Remove Storm Detection Entirely**

### Rationale

**1. Storm Detection Provides Zero Business Value (90% confidence)**

**Evidence**:
- ✅ **DD-AIANALYSIS-004**: AI Analysis does NOT expose storm to LLM (3-6% business value)
- ✅ **DD-GATEWAY-014**: Storm detection ≠ circuit breaker (architecturally different)
- ✅ **Deduplication already aggregates**: `occurrenceCount` provides same information
- ✅ **No workflow routing**: Remediation Orchestrator treats all signals equally
- ✅ **Boolean flag adds nothing**: Storm = `occurrenceCount >= 5` (derivable)

**2. Simplicity > Feature Creep**

Storm detection violates YAGNI principle:
- Originally designed for CRD aggregation (never implemented per DD-GATEWAY-012)
- Became status tracking only (redundant with `occurrenceCount`)
- No downstream consumers emerged

**3. Observability Preserved**

**Prometheus Query Replacement**:
```promql
# OLD: Storm-specific metric
gateway_alert_storms_detected_total

# NEW: Query occurrence_count directly (same information)
count(kube_customresource_remediation_kubernaut_ai_remediationrequest_status_deduplication_occurrence_count >= 5)
```

**4. No Breaking Changes**

Storm flag was never consumed:
- ✅ AI Analysis ignores storm (DD-AIANALYSIS-004)
- ✅ Remediation Orchestrator ignores storm
- ✅ WorkflowExecution ignores storm
- ✅ No external APIs expose storm

**5. Alignment with Architecture Decisions**

Removing storm detection aligns with:
- **DD-GATEWAY-011**: Shared Status Ownership - `occurrenceCount` is authoritative
- **DD-AIANALYSIS-004**: Storm Context NOT Exposed to LLM
- **DD-GATEWAY-014**: Circuit Breaker Deferral - storm ≠ overload protection

---

## Implementation Guidance

### Phase 1: Code Removal (4-6 hours)

**Estimated Effort**: 4-6 hours
**Risk**: LOW - well-scoped changes

#### Step 1: Remove Source Code (2-3h)

**1.1. Remove Storm Fields from NormalizedSignal**

```diff
// pkg/gateway/types/types.go
type NormalizedSignal struct {
    // ... existing fields ...
    RawPayload json.RawMessage

-   // Storm Detection Fields
-   IsStorm bool
-   StormType string
-   StormWindow string
-   AlertCount int
-   AffectedResources []string
}
```

**1.2. Remove StormSettings Configuration**

```diff
// pkg/gateway/config/config.go
type ProcessingSettings struct {
    Adapters   AdapterSettings
-   Storm      StormSettings
    Retry      RetrySettings
    CRD        CRDSettings
}

-// StormSettings contains storm detection configuration.
-type StormSettings struct {
-   RateThreshold int `yaml:"rate_threshold"`
-   PatternThreshold int `yaml:"pattern_threshold"`
-   // ... all storm fields ...
-}
```

**1.3. Remove Storm Logic from Server**

```diff
// pkg/gateway/server.go
type Server struct {
    // ... existing fields ...
-   stormThreshold int32
}

func createServerWithClients(...) (*Server, error) {
    // ... existing code ...

-   // DD-GATEWAY-012: Storm detection via Redis REMOVED
-   stormThreshold := int32(cfg.Processing.Storm.BufferThreshold)
-   if stormThreshold <= 0 {
-       stormThreshold = 5
-   }

    return &Server{
        // ... existing fields ...
-       stormThreshold: stormThreshold,
    }, nil
}

func (s *Server) ProcessSignal(...) {
    // ... deduplication logic ...

    if shouldDeduplicate && existingRR != nil {
        if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
            // ... error handling ...
        }

-       // Calculate storm threshold
-       occurrenceCount := int32(1)
-       if existingRR.Status.Deduplication != nil {
-           occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
-       }
-       isThresholdReached := occurrenceCount >= s.stormThreshold
-
-       // ASYNC: Update status.stormAggregation
-       rrCopy := existingRR.DeepCopy()
-       go func() {
-           asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
-           defer cancel()
-           if err := s.statusUpdater.UpdateStormAggregationStatus(asyncCtx, rrCopy, isThresholdReached); err != nil {
-               s.logger.Info("Failed to update storm aggregation status", ...)
-           }
-       }()

        s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName).Inc()
-       if isThresholdReached {
-           s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues("rate", signal.AlertName).Inc()
-       }
    }
}

-func (s *Server) emitStormDetectedAudit(...) {
-   // ... entire method removed ...
-}
```

**1.4. Remove UpdateStormAggregationStatus Method**

```diff
// pkg/gateway/processing/status_updater.go
-func (u *StatusUpdater) UpdateStormAggregationStatus(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, isThresholdReached bool) error {
-   // ... entire method removed ...
-}
```

**1.5. Remove Storm Metrics**

```diff
// pkg/gateway/metrics/metrics.go
type Metrics struct {
    // ... existing metrics ...
-   AlertStormsDetectedTotal *prometheus.CounterVec
}

func NewMetrics() *Metrics {
    // ... existing metrics ...

-   alertStormsDetectedTotal := prometheus.NewCounterVec(
-       prometheus.CounterOpts{
-           Namespace: "gateway",
-           Subsystem: "alerts",
-           Name:      "storms_detected_total",
-           Help:      "Total number of alert storms detected",
-       },
-       []string{"storm_type", "alert_name"},
-   )
-   prometheus.MustRegister(alertStormsDetectedTotal)

    return &Metrics{
        // ... existing metrics ...
-       AlertStormsDetectedTotal: alertStormsDetectedTotal,
    }
}
```

---

#### Step 2: Remove CRD Schema (1h)

**2.1. Remove StormAggregationStatus from CRD Types**

```diff
// api/remediation/v1alpha1/remediationrequest_types.go
type RemediationRequestStatus struct {
    // ... existing fields ...
    Deduplication *DeduplicationStatus `json:"deduplication,omitempty"`
-   StormAggregation *StormAggregationStatus `json:"stormAggregation,omitempty"`
}

-// StormAggregationStatus tracks storm-related aggregation state
-type StormAggregationStatus struct {
-   IsPartOfStorm    bool        `json:"isPartOfStorm"`
-   AggregatedCount  int32       `json:"aggregatedCount"`
-   StormDetectedAt  *metav1.Time `json:"stormDetectedAt,omitempty"`
-}
```

**2.2. Regenerate CRD YAML**

```bash
make manifests
```

**2.3. Verify CRD Changes**

```bash
# Ensure status.stormAggregation removed
grep -q "stormAggregation" config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml && echo "ERROR: Storm still in CRD" || echo "✅ Storm removed"
```

---

#### Step 3: Remove Tests (1-2h)

**3.1. Delete Storm-Specific Test Files**

```bash
rm test/unit/gateway/storm_aggregation_status_test.go
rm test/unit/gateway/storm_detection_test.go
rm test/e2e/gateway/01_storm_buffering_test.go
```

**3.2. Remove Storm Test from Integration Tests**

```diff
// test/integration/gateway/webhook_integration_test.go
-Describe("BR-GATEWAY-013: Storm Detection", func() {
-   // ... entire test case removed ...
-})
```

**3.3. Remove Storm Configuration from Test Data**

```diff
# pkg/gateway/config/testdata/valid-config.yaml
processing:
  adapters:
    # ... adapter config ...
- storm:
-   buffer_threshold: 5
-   inactivity_timeout: 60s
  retry:
    # ... retry config ...
```

---

### Phase 2: Update Documentation (1-2 hours)

**Estimated Effort**: 1-2 hours

#### Step 1: Mark Business Requirements as REMOVED

```diff
# docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md
-### **BR-GATEWAY-008: Storm Detection**
-**Description**: Gateway must detect alert storms (>10 alerts/minute) and aggregate them
-**Priority**: P1 (High)
-**Test Coverage**: ✅ Unit + Integration + E2E
+### **BR-GATEWAY-008: Storm Detection** ❌ REMOVED (DD-GATEWAY-015)
+**Status**: REMOVED in favor of deduplication-based occurrence tracking
+**Reason**: Storm detection provided no business value (redundant with `occurrenceCount`)
+**See**: [DD-GATEWAY-015](../../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)

-### **BR-GATEWAY-009: Concurrent Storm Detection**
+### **BR-GATEWAY-009: Concurrent Storm Detection** ❌ REMOVED (DD-GATEWAY-015)

-### **BR-GATEWAY-010: Storm State Recovery**
+### **BR-GATEWAY-010: Storm State Recovery** ❌ REMOVED (DD-GATEWAY-015)

-### **BR-GATEWAY-070: Storm Detection Metrics**
+### **BR-GATEWAY-070: Storm Detection Metrics** ❌ REMOVED (DD-GATEWAY-015)
```

#### Step 2: Add SUPERSEDED Notice to Design Decisions

```diff
# docs/architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md
+# DD-GATEWAY-008: Storm Aggregation First-Alert Handling
+
+**Status**: ❌ **SUPERSEDED** by DD-GATEWAY-015
+**Superseded Date**: December 13, 2025
+**Reason**: Storm detection removed entirely (no business value)
+**See**: [DD-GATEWAY-015: Storm Detection Logic Removal](DD-GATEWAY-015-storm-detection-removal.md)
+
+---
+
+## SUPERSEDED NOTICE
+
+This design decision is **no longer applicable**. Storm detection has been removed from Gateway per DD-GATEWAY-015.
+
+**Why Storm Detection Was Removed**:
+1. Redundant with deduplication (`occurrenceCount` provides same information)
+2. No downstream consumers (AI Analysis doesn't use storm flag)
+3. Boolean flag adds no value (storm = `occurrenceCount >= 5`)
+
+**Original content preserved below for historical context.**
+
+---
```

**Same pattern for DD-GATEWAY-012**.

#### Step 3: Update Gateway Service Documentation

```diff
# docs/services/stateless/gateway-service/README.md
## Core Capabilities

1. **Signal Ingestion**: Multi-adapter architecture (Prometheus, K8s Events)
2. **Deduplication**: State-based deduplication with occurrence count tracking
-3. **Storm Detection**: Identifies alert storms via occurrence count threshold
-4. **CRD Management**: Creates RemediationRequest CRDs
-5. **Observability**: Comprehensive metrics, structured logging, audit events
+3. **CRD Management**: Creates RemediationRequest CRDs
+4. **Observability**: Comprehensive metrics, structured logging, audit events

## Architecture
-Storm detection removed per DD-GATEWAY-015. Use `status.deduplication.occurrenceCount` for persistence tracking.
```

---

### Phase 3: Migration & Validation (1-2 hours)

**Estimated Effort**: 1-2 hours

#### Step 1: CRD Migration Plan

**No migration needed** - `status.stormAggregation` field removal is backward-compatible:
- Old CRDs with `status.stormAggregation` → field ignored
- New CRDs without `status.stormAggregation` → no change

**Kubernetes OpenAPI v3 Schema Validation**: Handles backward compatibility automatically.

#### Step 2: Observability Migration

**Replace Storm Metrics with Occurrence Count Queries**:

```promql
# OLD: Storm-specific metric
gateway_alert_storms_detected_total

# NEW: Query occurrence count (same information)
count(
  kube_customresource_remediation_kubernaut_ai_remediationrequest_status_deduplication_occurrence_count >= 5
)

# NEW: Storm alerts by namespace
sum by (namespace) (
  kube_customresource_remediation_kubernaut_ai_remediationrequest_status_deduplication_occurrence_count >= 5
)

# NEW: Top 10 highest occurrence counts (storm-like behavior)
topk(10,
  kube_customresource_remediation_kubernaut_ai_remediationrequest_status_deduplication_occurrence_count
)
```

**Dashboard Updates**:
```yaml
# OLD: Storm Detection Panel
- title: "Alert Storms Detected"
  targets:
    - expr: rate(gateway_alert_storms_detected_total[5m])

# NEW: High Occurrence Count Panel
- title: "Signals with High Occurrence Count (≥5)"
  targets:
    - expr: count(kube_customresource_remediation_kubernaut_ai_remediationrequest_status_deduplication_occurrence_count >= 5)
```

#### Step 3: Validation Tests

**Run All Gateway Tests**:
```bash
# Unit tests
go test ./pkg/gateway/... -v

# Integration tests
cd test/integration/gateway && go test -v

# E2E tests
cd test/e2e/gateway && go test -v
```

**Expected Results**:
- ✅ All tests pass (storm tests removed)
- ✅ No compilation errors
- ✅ No lint errors

**Smoke Test CRD Creation**:
```bash
# Deploy updated Gateway
kubectl apply -f deploy/gateway/

# Send test alert
curl -X POST http://gateway:8080/api/v1/signals/prometheus \
  -d '{"alerts":[{"labels":{"alertname":"TestAlert"}}]}'

# Verify CRD created WITHOUT status.stormAggregation
kubectl get remediationrequest -o yaml | grep -q stormAggregation && echo "❌ Storm field still present" || echo "✅ Storm field removed"
```

---

### Phase 4: Documentation & Communication (1h)

**Estimated Effort**: 1 hour

#### Step 1: Create Migration Notice

```markdown
# NOTICE: Storm Detection Removal (DD-GATEWAY-015)

**Date**: December 13, 2025
**Affected Service**: Gateway
**Breaking Change**: NO

## Summary

Storm detection logic has been removed from Gateway per DD-GATEWAY-015.

## Rationale

1. **Redundant**: `status.deduplication.occurrenceCount` already tracks persistence
2. **No Consumers**: AI Analysis does NOT expose storm to LLM (DD-AIANALYSIS-004)
3. **Zero Value**: Storm = boolean flag based on `occurrenceCount >= 5`

## Impact

### What Changed

- ✅ `status.stormAggregation` field removed from RemediationRequest CRD
- ✅ `gateway_alert_storms_detected_total` metric removed
- ✅ `gateway.storm.detected` audit event removed
- ✅ Storm configuration removed from ConfigMap

### What Remains

- ✅ `status.deduplication.occurrenceCount` (authoritative persistence tracking)
- ✅ All deduplication logic unchanged
- ✅ CRD creation unchanged

### Migration Required?

**NO** - Backward-compatible change. Existing CRDs with `status.stormAggregation` continue to work (field ignored).

## Observability

**Replace storm metrics with occurrence count queries**:

```promql
# Storm-like behavior: Signals with high occurrence count
count(kube_customresource_remediation_kubernaut_ai_remediationrequest_status_deduplication_occurrence_count >= 5)
```

## References

- [DD-GATEWAY-015: Storm Detection Logic Removal](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
- [DD-AIANALYSIS-004: Storm Context NOT Exposed to LLM](../architecture/decisions/DD-AIANALYSIS-004-storm-context-not-exposed.md)
```

#### Step 2: Update DESIGN_DECISIONS.md Index

```diff
# docs/architecture/DESIGN_DECISIONS.md
| DD-GATEWAY-014 | Service-Level Circuit Breaker Deferral | ⏸️ Deferred | 2025-12-13 | [DD-GATEWAY-014-circuit-breaker-deferral.md](decisions/DD-GATEWAY-014-circuit-breaker-deferral.md) |
+| DD-GATEWAY-015 | Storm Detection Logic Removal | 📋 Planned | 2025-12-13 | [DD-GATEWAY-015-storm-detection-removal.md](decisions/DD-GATEWAY-015-storm-detection-removal.md) |
```

---

## Consequences

### Positive Consequences

**1. Simplified Codebase**
- ✅ Removes ~500 lines of code
- ✅ Removes 3 test files
- ✅ Removes StormAggregationStatus from CRD schema
- ✅ Clearer intent (deduplication via `occurrenceCount`)

**2. Lower Maintenance Burden**
- ✅ Fewer tests to maintain
- ✅ Fewer configuration options to support
- ✅ Fewer CRD schema changes going forward

**3. No Breaking Changes**
- ✅ No downstream consumers of storm flag
- ✅ Backward-compatible CRD schema change
- ✅ Observability preserved via Prometheus queries

**4. Clearer Architecture**
- ✅ Single source of truth: `occurrenceCount`
- ✅ No confusion between storm and deduplication
- ✅ Simpler mental model for developers

---

### Negative Consequences

**1. Loss of Explicit Storm Signal**
- ❌ No dedicated `gateway_alert_storms_detected_total` metric
- ❌ No `gateway.storm.detected` audit event
- ❌ SRE teams must use Prometheus queries instead

**Mitigation**: Provide documented Prometheus queries for storm-like behavior.

**2. Historical Context Loss**
- ❌ Future developers won't see storm detection code
- ❌ Design decisions (DD-GATEWAY-008, DD-GATEWAY-012) become historical artifacts

**Mitigation**: Preserve design decisions with SUPERSEDED notices, maintain DD-GATEWAY-015 as authoritative removal documentation.

**3. Dashboard Updates Required**
- ❌ Existing Grafana dashboards using `gateway_alert_storms_detected_total` will break
- ❌ SRE teams must update queries

**Mitigation**: Provide migration guide with replacement queries.

---

### Neutral Consequences

**1. CRD Schema Change**
- Removal of `status.stormAggregation` field
- Backward-compatible (old CRDs continue to work)

**2. Configuration Simplification**
- Storm settings removed from ConfigMap
- Fewer tuning parameters for operators

---

## Validation Checklist

### Pre-Implementation Validation

- [ ] All storm detection code identified and documented
- [ ] Downstream consumer analysis confirms no usage (DD-AIANALYSIS-004 validated)
- [ ] Prometheus query replacements tested and documented
- [ ] Team alignment on removal decision

### Implementation Validation

- [ ] All source code removed (5 files modified)
- [ ] All tests removed (3 files deleted, 1 file modified)
- [ ] CRD schema updated (`make manifests` executed)
- [ ] Configuration files updated (storm settings removed)
- [ ] All tests pass (`go test ./pkg/gateway/...`)
- [ ] No compilation errors
- [ ] No lint errors
- [ ] CRD smoke test confirms storm field removed

### Post-Implementation Validation

- [ ] Grafana dashboards updated with new queries
- [ ] SRE teams notified of metric changes
- [ ] Migration notice published
- [ ] Design decision index updated
- [ ] Business requirements marked as REMOVED
- [ ] DD-GATEWAY-008 and DD-GATEWAY-012 marked as SUPERSEDED

---

## Implementation Timeline

| Phase | Duration | Tasks | Validation |
|-------|----------|-------|------------|
| **Phase 1**: Code Removal | 4-6h | Remove source code, CRD schema, tests | All tests pass |
| **Phase 2**: Documentation | 1-2h | Update BRs, DDs, service docs | Review approval |
| **Phase 3**: Validation | 1-2h | CRD migration, observability migration | Smoke tests |
| **Phase 4**: Communication | 1h | Migration notice, index update | Team notification |

**Total Effort**: 7-11 hours

---

## Rollback Plan

**If removal causes unexpected issues**:

### Rollback Option A: Revert Git Commit (Recommended)

```bash
git revert <dd-gateway-015-commit-sha>
git push
```

**Impact**: Restores all storm detection code in 5 minutes.

### Rollback Option B: Re-implement from DD-GATEWAY-012

**Effort**: 8-12 hours (same as original implementation)

**Reason to Avoid**: If rollback is needed, use Option A instead.

---

## Related Decisions

- **DD-GATEWAY-011**: Shared Status Ownership - `occurrenceCount` is authoritative
- **DD-GATEWAY-012**: Redis-free Storm Detection - Superseded by DD-GATEWAY-015
- **DD-GATEWAY-008**: Storm Aggregation First-Alert Handling - Superseded by DD-GATEWAY-015
- **DD-AIANALYSIS-004**: Storm Context NOT Exposed to LLM - Confirms no downstream consumers
- **DD-GATEWAY-014**: Service-Level Circuit Breaker Deferral - Storm ≠ circuit breaker
- **BR-GATEWAY-008**: Storm Detection - Removed
- **BR-GATEWAY-009**: Concurrent Storm Detection - Removed
- **BR-GATEWAY-010**: Storm State Recovery - Removed
- **BR-GATEWAY-070**: Storm Detection Metrics - Removed

---

## References

- **Analysis Document**: `docs/handoff/BRAINSTORM_STORM_DETECTION_PURPOSE.md` - Storm purpose analysis
- **AI Analysis**: `docs/handoff/CONFIDENCE_ASSESSMENT_GATEWAY_CIRCUIT_BREAKER.md` - Circuit breaker vs storm
- **Implementation Plan**: This document (DD-GATEWAY-015)
- **CRD Schema**: `api/remediation/v1alpha1/remediationrequest_types.go`
- **Gateway Server**: `pkg/gateway/server.go`

---

**Decision Status**: 📋 PLANNED for Implementation
**Implementation Effort**: 7-11 hours
**Risk**: LOW - No downstream consumers, backward-compatible
**Rollback**: Simple (git revert)


