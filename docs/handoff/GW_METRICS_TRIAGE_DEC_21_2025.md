# Gateway Metrics Triage - DD-005 V3.0 Compliance

**Date**: December 21, 2025
**Author**: AI Assistant
**Purpose**: Triage Gateway metrics against business requirements and specifications
**Authority**: [metrics-slos.md](../services/stateless/gateway-service/metrics-slos.md), [BUSINESS_REQUIREMENTS.md](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)

---

## 🎯 **Executive Summary**

**Problem**: Gateway has **43 implemented metrics**, but specification only defines **~10 core metrics**

**Finding**: **33 metrics (77%) lack specification and unclear business value**

**Risk**: "Buffet metrics" without business alignment increase maintenance burden without delivering observability value

---

## 📊 **Metrics Audit Results**

### **Category 1: ✅ SPECIFIED & BUSINESS-ALIGNED** (10 metrics)

These metrics are explicitly defined in `metrics-slos.md` and mapped to business requirements:

| Metric Name | Specification | Business Requirement | Value |
|-------------|--------------|---------------------|-------|
| `gateway_signals_received_total` | ✅ Yes (line 15) | BR-GATEWAY-066 | ✅ Core ingestion tracking |
| `gateway_signals_deduplicated_total` | ✅ Yes (line 23) | BR-GATEWAY-069 | ✅ Deduplication effectiveness |
| `gateway_crds_created_total` | ✅ Yes (line 39) | BR-GATEWAY-068 | ✅ CRD creation tracking |
| `gateway_crd_creation_errors_total` | ✅ Yes (line 45) | BR-GATEWAY-068 | ✅ Error tracking |
| `gateway_http_request_duration_seconds` | ✅ Yes (line 61) | BR-GATEWAY-067, BR-GATEWAY-079 | ✅ Latency SLO (P50/P95/P99) |
| `gateway_redis_operation_duration_seconds` | ✅ Yes (line 69) | BR-GATEWAY-079 | ✅ Redis performance tracking |
| `gateway_deduplication_cache_hits_total` | ✅ Yes (line 83) | BR-GATEWAY-069 | ✅ Cache effectiveness |
| `gateway_deduplication_rate` | ✅ Yes (line 90) | BR-GATEWAY-069 | ✅ Deduplication % tracking |
| `gateway_signals_rejected_total` | ✅ Implied (API spec) | BR-GATEWAY-TARGET-RESOURCE-VALIDATION | ✅ Validation tracking |
| `gateway_retry_attempts_total` | ✅ Implied | BR-GATEWAY-114 | ✅ K8s API retry observability |

**Status**: ✅ **KEEP ALL** - Core business metrics with clear value

---

### **Category 2: ⚠️ IMPLEMENTED BUT NOT SPECIFIED** (33 metrics)

These metrics were implemented but lack specification or clear business alignment:

#### **Subcategory 2A: Likely Useful (Need Specification)** (8 metrics)

| Metric Name | Business Case | Recommendation |
|-------------|--------------|----------------|
| `gateway_retry_duration_seconds` | Retry latency tracking (extends BR-GATEWAY-114) | ⚠️ **KEEP** - Add to spec |
| `gateway_retry_exhausted_total` | Retry failure tracking (extends BR-GATEWAY-114) | ⚠️ **KEEP** - Add to spec |
| `gateway_retry_success_total` | Retry success tracking (extends BR-GATEWAY-114) | ⚠️ **KEEP** - Add to spec |
| `gateway_http_requests_total` | Request count (complements duration) | ⚠️ **KEEP** - Add to spec |
| `gateway_http_requests_in_flight` | Concurrent request tracking | ⚠️ **KEEP** - Add to spec |
| `gateway_redis_operation_errors_total` | Redis error tracking | ⚠️ **KEEP** - Add to spec |
| `gateway_requests_rejected_total` | HTTP rejection tracking | ⚠️ **KEEP** - Add to spec |
| `gateway_consecutive_503_responses` | Circuit breaker indicator | ⚠️ **KEEP** - Add to spec (if circuit breaker is V1.0) |

**Action Required**: Add these 8 metrics to `metrics-slos.md` with business justification

---

#### **Subcategory 2B: Redundant/Duplicate** (12 metrics)

| Metric Name | Issue | Recommendation |
|-------------|-------|----------------|
| `gateway_deduplication_cache_misses_total` | Redundant (can derive from hits + total) | ❌ **REMOVE** |
| `gateway_deduplication_pool_size` | No deduplication pool in DD-GATEWAY-011 | ❌ **REMOVE** (outdated design) |
| `gateway_deduplication_pool_max_size` | No deduplication pool in DD-GATEWAY-011 | ❌ **REMOVE** (outdated design) |
| `gateway_duplicate_crds_prevented_total` | Duplicates `signals_deduplicated_total` | ❌ **REMOVE** (redundant) |
| `gateway_duplicate_prevention_active` | Unclear purpose (what does "active" mean?) | ❌ **REMOVE** (vague) |
| `gateway_duplicate_signals_total` | Duplicates `signals_deduplicated_total` | ❌ **REMOVE** (redundant) |
| `gateway_redis_pool_hits` | Duplicate of `redis_pool_hits_total` | ❌ **REMOVE** (duplicate) |
| `gateway_redis_pool_misses` | Duplicate of `redis_pool_misses_total` | ❌ **REMOVE** (duplicate) |
| `gateway_redis_pool_timeouts` | Duplicate of `redis_pool_timeouts_total` | ❌ **REMOVE** (duplicate) |
| `gateway_redis_pool_idle_connections` | Duplicate of `redis_pool_connections_idle` | ❌ **REMOVE** (duplicate) |
| `gateway_redis_pool_stale_connections` | No stale connection handling in spec | ❌ **REMOVE** (unused) |
| `gateway_redis_pool_total_connections` | Duplicate of `redis_pool_connections_total` | ❌ **REMOVE** (duplicate) |

**Impact**: Remove 12 metrics, reduce maintenance burden by ~28%

---

#### **Subcategory 2C: Redis Health/Pool (Low Priority V1.0)** (8 metrics)

| Metric Name | Issue | Recommendation |
|-------------|-------|----------------|
| `gateway_redis_available` | Redis health (useful for alerting) | ⚠️ **DEFER V1.1** - Not V1.0 critical |
| `gateway_redis_outage_count_total` | Redis outage tracking | ⚠️ **DEFER V1.1** - Nice to have |
| `gateway_redis_outage_duration_seconds_total` | Redis outage duration | ⚠️ **DEFER V1.1** - Nice to have |
| `gateway_redis_pool_connections_total` | Redis pool size tracking | ⚠️ **DEFER V1.1** - Operational, not business |
| `gateway_redis_pool_connections_idle` | Redis pool idle tracking | ⚠️ **DEFER V1.1** - Operational, not business |
| `gateway_redis_pool_connections_active` | Redis pool active tracking | ⚠️ **DEFER V1.1** - Operational, not business |
| `gateway_redis_pool_hits_total` | Redis pool hit rate | ⚠️ **DEFER V1.1** - Operational, not business |
| `gateway_redis_pool_misses_total` | Redis pool miss rate | ⚠️ **DEFER V1.1** - Operational, not business |

**Rationale**: Redis pool metrics are **operational**, not business-critical. Defer to V1.1 observability enhancements.

**Alternative**: If Redis observability is needed, use **Redis Exporter** (industry standard) instead of custom metrics.

---

#### **Subcategory 2D: Processing Pipeline (Missing Specification)** (5 metrics)

| Metric Name | Issue | Recommendation |
|-------------|-------|----------------|
| `gateway_signals_received_by_adapter_total` | Duplicates `signals_received_total` with adapter dimension? | ⚠️ **CLARIFY** - Is this different from signals_received_total? |
| `gateway_signals_processed_total` | What does "processed" mean vs "received"? | ⚠️ **CLARIFY** - Vague definition |
| `gateway_signals_failed_total` | Failed at what stage? (validation? CRD creation?) | ⚠️ **CLARIFY** - Overlaps with crd_creation_errors_total? |
| `gateway_crds_created_by_type_total` | What "type" dimension? (TargetType?) | ⚠️ **CLARIFY** - Useful if tracking K8s vs future non-K8s |

**Action Required**: Define clear semantics for these metrics or remove if redundant

---

## 📋 **Recommended Action Plan**

### **Phase 1: Immediate Cleanup (Remove Redundant/Duplicate)** - 🔴 **HIGH PRIORITY**

**Remove 12 metrics** (Subcategory 2B - Redundant/Duplicate):
- `gateway_deduplication_cache_misses_total`
- `gateway_deduplication_pool_size`
- `gateway_deduplication_pool_max_size`
- `gateway_duplicate_crds_prevented_total`
- `gateway_duplicate_prevention_active`
- `gateway_duplicate_signals_total`
- `gateway_redis_pool_hits` (keep `_total` version)
- `gateway_redis_pool_misses` (keep `_total` version)
- `gateway_redis_pool_timeouts` (keep `_total` version)
- `gateway_redis_pool_idle_connections` (keep `connections_idle` version)
- `gateway_redis_pool_stale_connections`
- `gateway_redis_pool_total_connections` (keep `connections_total` version)

**Effort**: 1 hour (delete metric definitions + tests)

**Result**: 43 metrics → 31 metrics (-28%)

---

### **Phase 2: Specification Update (Add Useful Metrics)** - 🟡 **MEDIUM PRIORITY**

**Add 8 metrics to `metrics-slos.md`** (Subcategory 2A):
- Define business justification for each
- Map to existing or new BR-GATEWAY-XXX requirements
- Document in metrics specification

**Effort**: 1-2 hours (documentation + BR mapping)

---

### **Phase 3: Defer Redis Pool Metrics to V1.1** - 🟢 **LOW PRIORITY**

**Defer 8 Redis pool metrics** (Subcategory 2C):
- Keep code but document as V1.1 enhancement
- Consider using Redis Exporter instead of custom metrics

**Effort**: 30 minutes (documentation update)

---

### **Phase 4: Clarify Processing Pipeline Metrics** - 🟡 **MEDIUM PRIORITY**

**Clarify or remove 4 metrics** (Subcategory 2D):
- Define clear semantics for each
- Remove if redundant with existing metrics

**Effort**: 1 hour (investigation + decision)

---

### **Phase 5: DD-005 V3.0 Compliance (Metric Constants)** - 🟡 **MEDIUM PRIORITY**

**Apply DD-005 V3.0 constants to FINAL metric set**:
- After cleanup (Phases 1-4), final metric count: ~26-31 metrics
- Define constants for remaining metrics only
- Avoid constants for deprecated/removed metrics

**Effort**: 2-3 hours (for ~26-31 metrics, not 43)

---

## 🎯 **Final Metric Count Projection**

| Category | Current Count | After Cleanup | Change |
|----------|--------------|---------------|--------|
| **Specified & Aligned** | 10 | 10 | No change |
| **Useful (Add to Spec)** | 8 | 8 | No change |
| **Redundant/Duplicate** | 12 | 0 | ❌ **Remove 12** |
| **Redis Pool (Defer V1.1)** | 8 | 8 | Defer (keep code) |
| **Processing Pipeline (Clarify)** | 4 | 2-4 | Clarify or remove |
| **TOTAL** | **43** | **26-31** | **-28% to -40%** |

---

## 💰 **Business Value Assessment**

### **High Business Value (Keep)** - 18 metrics
- Core ingestion tracking (signals received/deduplicated/rejected)
- CRD creation success/failure
- Latency SLOs (HTTP request duration)
- Retry observability (attempts/success/exhausted)
- Error tracking (CRD creation, Redis operations, HTTP rejections)

### **Operational Value (Defer V1.1)** - 8 metrics
- Redis pool health/performance metrics
- Better served by Redis Exporter (industry standard)

### **No Clear Value (Remove)** - 12 metrics
- Redundant duplicates
- Outdated design artifacts (deduplication pool)
- Vague purpose (duplicate_prevention_active)

### **Unclear Value (Clarify or Remove)** - 4 metrics
- Processing pipeline metrics with unclear semantics

---

## 📝 **Questions for User**

1. **Redis Pool Metrics**: Should we defer all 8 Redis pool metrics to V1.1 and use Redis Exporter instead?
2. **Processing Pipeline**: What do `signals_processed_total` and `signals_failed_total` track that isn't covered by existing metrics?
3. **Circuit Breaker**: Is `consecutive_503_responses` part of V1.0 circuit breaker (BR-GATEWAY-093)?
4. **Approval**: Proceed with Phase 1 cleanup (remove 12 redundant metrics)?

---

## 🔗 **References**

- **Metrics Specification**: [metrics-slos.md](../services/stateless/gateway-service/metrics-slos.md)
- **Business Requirements**: [BUSINESS_REQUIREMENTS.md](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)
- **DD-005 V3.0 Mandate**: [DD005_V3_METRIC_CONSTANTS_MANDATE_DEC_21_2025.md](DD005_V3_METRIC_CONSTANTS_MANDATE_DEC_21_2025.md)
- **Implementation**: `pkg/gateway/metrics/metrics.go` (463 lines)

---

**End of Triage**

