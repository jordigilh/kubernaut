# Gateway Metrics Cleanup Plan - Specification-Only Approach

**Date**: December 21, 2025
**Author**: AI Assistant
**Directive**: Keep ONLY metrics from specification, remove Redis (Gateway doesn't use Redis anymore)
**Authority**: [metrics-slos.md](../services/stateless/gateway-service/metrics-slos.md)

---

## 🎯 **Executive Summary**

**Current State**: 43 metrics implemented
**Specification**: ~10 metrics defined (but includes Redis)
**Redis Status**: ❌ Gateway no longer uses Redis (removed per DD-GATEWAY-012)
**Target State**: **7 metrics** (specification minus Redis)

**Cleanup**: **Remove 36 metrics (84% reduction)**

---

## ✅ **KEEP: 7 Metrics from Specification (Non-Redis)**

| Metric Name | Specification Line | Business Requirement | Justification |
|-------------|-------------------|---------------------|---------------|
| `gateway_signals_received_total` | Line 15-21 | BR-GATEWAY-066 | Core ingestion tracking |
| `gateway_signals_deduplicated_total` | Line 23-29 | BR-GATEWAY-069 | Deduplication effectiveness |
| `gateway_crds_created_total` | Line 39-43 | BR-GATEWAY-068 | CRD creation success tracking |
| `gateway_crd_creation_errors_total` | Line 45-52 | BR-GATEWAY-068 | CRD creation failure tracking |
| `gateway_http_request_duration_seconds` | Line 61-66 | BR-GATEWAY-067, BR-GATEWAY-079 | Latency SLO (P50/P95/P99) |
| `gateway_deduplication_cache_hits_total` | Line 83-88 | BR-GATEWAY-069 | Cache effectiveness |
| `gateway_deduplication_rate` | Line 90-96 | BR-GATEWAY-069 | Deduplication percentage |

**Total**: **7 metrics** aligned with business requirements and current architecture

---

## ❌ **REMOVE: 36 Metrics (84%)**

### **Category A: Redis Metrics (12 metrics) - Gateway doesn't use Redis**

| Metric Name | Reason |
|-------------|--------|
| `gateway_redis_operation_duration_seconds` | No Redis (DD-GATEWAY-012) |
| `gateway_redis_operation_errors_total` | No Redis |
| `gateway_redis_available` | No Redis |
| `gateway_redis_outage_count_total` | No Redis |
| `gateway_redis_outage_duration_seconds_total` | No Redis |
| `gateway_redis_pool_connections_total` | No Redis |
| `gateway_redis_pool_connections_idle` | No Redis |
| `gateway_redis_pool_connections_active` | No Redis |
| `gateway_redis_pool_hits_total` | No Redis |
| `gateway_redis_pool_misses_total` | No Redis |
| `gateway_redis_pool_timeouts_total` | No Redis |
| `gateway_redis_pool_*` (duplicates) | No Redis |

### **Category B: Not in Specification (24 metrics)**

| Metric Name | Reason |
|-------------|--------|
| `gateway_signals_rejected_total` | Not in specification |
| `gateway_retry_attempts_total` | Not in specification |
| `gateway_retry_duration_seconds` | Not in specification |
| `gateway_retry_exhausted_total` | Not in specification |
| `gateway_retry_success_total` | Not in specification |
| `gateway_http_requests_total` | Not in specification |
| `gateway_http_requests_in_flight` | Not in specification |
| `gateway_requests_rejected_total` | Not in specification |
| `gateway_consecutive_503_responses` | Not in specification |
| `gateway_deduplication_cache_misses_total` | Not in specification (redundant) |
| `gateway_deduplication_pool_size` | Not in specification |
| `gateway_deduplication_pool_max_size` | Not in specification |
| `gateway_duplicate_crds_prevented_total` | Not in specification |
| `gateway_duplicate_prevention_active` | Not in specification |
| `gateway_duplicate_signals_total` | Not in specification |
| `gateway_signals_received_by_adapter_total` | Not in specification |
| `gateway_signals_processed_total` | Not in specification |
| `gateway_signals_failed_total` | Not in specification |
| `gateway_crds_created_by_type_total` | Not in specification |
| Plus 5 more duplicate redis_pool variants | Not in specification |

---

## 📋 **Implementation Plan**

### **Step 1: Update metrics.go - Remove 36 Metrics**

**Files to Modify**:
- `pkg/gateway/metrics/metrics.go` (463 lines → ~150 lines estimated)

**Actions**:
1. Remove all Redis-related metrics (12 metrics + fields)
2. Remove all non-specification metrics (24 metrics + fields)
3. Keep only 7 specification-aligned metrics
4. Clean up struct fields and NewMetricsWithRegistry()

**Estimated Reduction**: 463 lines → ~150 lines (-68%)

---

### **Step 2: Update Tests - Remove References**

**Files to Modify**:
- `test/unit/gateway/metrics/metrics_test.go`
- `test/unit/gateway/metrics/failure_metrics_test.go`
- `test/integration/gateway/*_test.go` (if any hardcoded metric names)
- `test/e2e/gateway/04_metrics_endpoint_test.go` (minimal - only checks prefix)

**Actions**:
1. Remove test cases for deleted metrics
2. Remove assertions checking deleted metrics
3. Keep tests for 7 remaining metrics

---

### **Step 3: Update Specification - Remove Redis**

**Files to Modify**:
- `docs/services/stateless/gateway-service/metrics-slos.md`

**Actions**:
1. Remove gateway_redis_operation_duration_seconds section (lines 68-76)
2. Remove Redis performance Grafana panel (lines 136-138)
3. Remove Redis latency alert rule (lines 200-208)
4. Update SLO queries if they reference Redis

---

### **Step 4: Apply DD-005 V3.0 Constants (7 Metrics Only)**

**After cleanup, define constants for FINAL 7 metrics**:

```go
// pkg/gateway/metrics/metrics.go

const (
    // Metric name constants (DD-005 V3.0 Section 1.1 - MANDATORY)

    // MetricNameSignalsReceivedTotal tracks total signals received
    MetricNameSignalsReceivedTotal = "signals_received_total"

    // MetricNameSignalsDeduplicatedTotal tracks deduplicated signals
    MetricNameSignalsDeduplicatedTotal = "signals_deduplicated_total"

    // MetricNameCRDsCreatedTotal tracks successful CRD creations
    MetricNameCRDsCreatedTotal = "crds_created_total"

    // MetricNameCRDCreationErrorsTotal tracks CRD creation failures
    MetricNameCRDCreationErrorsTotal = "crd_creation_errors_total"

    // MetricNameHTTPRequestDuration tracks HTTP request latency
    MetricNameHTTPRequestDuration = "http_request_duration_seconds"

    // MetricNameDeduplicationCacheHitsTotal tracks cache hits
    MetricNameDeduplicationCacheHitsTotal = "deduplication_cache_hits_total"

    // MetricNameDeduplicationRate tracks deduplication percentage
    MetricNameDeduplicationRate = "deduplication_rate"
)
```

---

### **Step 5: Verification**

```bash
# 1. Verify metrics compile
go build ./pkg/gateway/metrics/...

# 2. Verify no Redis references remain
grep -r "redis\|Redis" pkg/gateway/metrics/metrics.go

# 3. Run unit tests
make test-unit-gateway-metrics

# 4. Run integration tests
make test-integration-gateway

# 5. Run E2E tests
make test-e2e-gateway

# 6. Verify only 7 metrics registered
grep "Name:.*\"gateway_" pkg/gateway/metrics/metrics.go | wc -l
# Expected: 7
```

---

## 📊 **Before vs. After**

| Aspect | Before | After | Change |
|--------|--------|-------|--------|
| **Total Metrics** | 43 | 7 | **-84%** |
| **Redis Metrics** | 12 | 0 | **-100%** |
| **Specification Aligned** | 23% (10/43) | 100% (7/7) | **+77%** |
| **metrics.go Size** | 463 lines | ~150 lines | **-68%** |
| **DD-005 V3.0 Constants** | 0 | 7 | **100% compliant** |
| **Maintenance Burden** | High (43 metrics) | Low (7 metrics) | **-84%** |

---

## ✅ **Benefits**

1. **Specification-Driven**: 100% alignment with metrics-slos.md
2. **Architecture-Aligned**: No Redis metrics (Gateway doesn't use Redis)
3. **Business Value**: Every metric maps to BR-GATEWAY-XXX requirement
4. **Maintainability**: 84% reduction in metric count
5. **DD-005 V3.0 Compliant**: Constants for all 7 metrics
6. **Clear Purpose**: No "buffet metrics" without justification

---

## 🚨 **Risks & Mitigations**

| Risk | Mitigation |
|------|-----------|
| **Breaking Grafana dashboards** | Audit Grafana dashboards, remove references to deleted metrics |
| **Breaking alert rules** | Audit Prometheus alert rules, remove references to deleted metrics |
| **CI/CD failures** | Run full test suite before merge |
| **Production metrics missing** | Only removing unspecified/Redis metrics - core business metrics preserved |

---

## 📝 **Approval Checklist**

- [ ] User confirms: Remove ALL Redis metrics (Gateway doesn't use Redis)
- [ ] User confirms: Keep ONLY 7 metrics from specification
- [ ] User confirms: Remove 36 unspecified metrics
- [ ] Proceed with implementation

---

**Status**: ⏸️ **AWAITING USER APPROVAL**

**Next Action**: Proceed with Step 1 (Update metrics.go) upon approval

---

**End of Plan**










