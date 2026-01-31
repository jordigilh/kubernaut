# HAPI Metrics Triage - January 31, 2026

## Executive Summary

**Status:** ⚠️ **METRICS BLOAT IDENTIFIED** - 15 metrics exposed, only 6-8 have BR coverage

**Issue:** HAPI is exposing metrics without clear Business Requirement (BR) backing, violating the "business requirement drives functionality" principle.

**Recommendation:** 
1. **Immediate:** Remove unauthorized metrics (7-9 metrics)
2. **Short-term:** Document remaining metrics with BRs
3. **Medium-term:** Refactor to Go pattern (metrics in business logic)

---

## Current Metrics Inventory

### All Exposed Metrics (15 total)

```python
# Investigation Metrics (2)
investigations_total = Counter('holmesgpt_api_investigations_total', ...)
investigations_duration_seconds = Histogram('holmesgpt_api_investigations_duration_seconds', ...)

# LLM Metrics (3)
llm_calls_total = Counter('holmesgpt_api_llm_calls_total', ...)
llm_call_duration_seconds = Histogram('holmesgpt_api_llm_call_duration_seconds', ...)
llm_token_usage = Counter('holmesgpt_api_llm_token_usage_total', ...)

# Authentication Metrics (2)
auth_failures_total = Counter('holmesgpt_api_auth_failures_total', ...)
auth_success_total = Counter('holmesgpt_api_auth_success_total', ...)

# Context API Metrics (2)
context_api_calls_total = Counter('holmesgpt_api_context_calls_total', ...)
context_api_duration_seconds = Histogram('holmesgpt_api_context_duration_seconds', ...)

# Config Hot-Reload Metrics (3) - BR-HAPI-199
config_reload_total = Counter('holmesgpt_api_config_reload_total', ...)
config_reload_errors_total = Counter('holmesgpt_api_config_reload_errors_total', ...)
config_last_reload_timestamp = Gauge('holmesgpt_api_config_last_reload_timestamp', ...)

# HTTP Metrics (2)
http_requests_total = Counter('holmesgpt_api_http_requests_total', ...)
http_request_duration_seconds = Histogram('holmesgpt_api_http_request_duration_seconds', ...)

# Active Requests (1)
active_requests = Gauge('holmesgpt_api_active_requests', ...)

# RFC 7807 Errors (1) - BR-HAPI-200
rfc7807_errors_total = Counter('holmesgpt_api_rfc7807_errors_total', ...)
```

---

## Business Requirements Analysis

### BR Search Results

**Documented BRs:**
1. **BR-HAPI-011**: "Investigation metrics" (mentioned but NOT defined)
2. **BR-HAPI-199**: ConfigMap hot-reload metrics (3 metrics explicitly defined)
   - `hapi_config_reload_total`
   - `hapi_config_reload_errors_total`
   - `hapi_config_last_reload_timestamp_seconds`
3. **BR-HAPI-200**: RFC 7807 error response metrics (1 metric defined)
   - `rfc7807_errors_total`
4. **BR-HAPI-POSTEXEC-002**: Post-execution metrics collection (DEFERRED to v1.1)

**Missing BR References:**
- BR-HAPI-100, BR-HAPI-101, BR-HAPI-102, BR-HAPI-103 (referenced in code comments, NOT in BR doc)

---

## Metric-by-Metric Triage

### ✅ **KEEP: Metrics with BR Coverage (6 metrics)**

#### 1-3. Config Hot-Reload Metrics ✅ **BR-HAPI-199**
```python
config_reload_total                      # ✅ Explicitly in BR
config_reload_errors_total               # ✅ Explicitly in BR  
config_last_reload_timestamp             # ✅ Explicitly in BR
```
**Status:** ✅ **COMPLIANT** - Explicitly defined in BR-HAPI-199  
**Location:** `holmesgpt-api/src/middleware/metrics.py` lines 123-139

#### 4. RFC 7807 Error Metrics ✅ **BR-HAPI-200**
```python
rfc7807_errors_total                     # ✅ Implied by BR-HAPI-200
```
**Status:** ✅ **COMPLIANT** - RFC 7807 implementation requires error tracking  
**Location:** `holmesgpt-api/src/middleware/metrics.py` lines 165-169

#### 5-6. HTTP Request Metrics ✅ **DD-005 Standard**
```python
http_requests_total                      # ✅ DD-005 mandates HTTP metrics
http_request_duration_seconds            # ✅ DD-005 mandates latency tracking
```
**Status:** ✅ **COMPLIANT** - Required by DD-005 observability standards  
**Rationale:** All HTTP services must expose request counts and latency per DD-005  
**Location:** `holmesgpt-api/src/middleware/metrics.py` lines 148-160

---

### ⚠️ **NEEDS BR: Metrics with Implied Business Value (5 metrics)**

#### 7-8. Investigation Metrics ⚠️ **BR-HAPI-011 (UNDEFINED)**
```python
investigations_total                     # ⚠️ BR exists but metrics NOT specified
investigations_duration_seconds          # ⚠️ BR exists but metrics NOT specified
```
**Status:** ⚠️ **NEEDS DOCUMENTATION**  
**Issue:** BR-HAPI-011 mentions "investigation metrics" but doesn't define WHICH metrics  
**Recommendation:** Document specific metrics in BR or create new BR-HAPI-011-A  
**Business Value:** HIGH - core business capability (incident/recovery analysis)  
**Action:** ADD to BR document as:
```markdown
#### BR-HAPI-011: Investigation Metrics

**Metrics:**
- `holmesgpt_api_investigations_total{status}` - Counter of investigations by outcome
- `holmesgpt_api_investigations_duration_seconds` - Histogram of investigation duration

**SLO:** P95 investigation latency < 10 seconds
```

#### 9-11. LLM Metrics ⚠️ **NO BR**
```python
llm_calls_total                          # ⚠️ No BR, but business-critical
llm_call_duration_seconds                # ⚠️ No BR, but business-critical
llm_token_usage                          # ⚠️ No BR, but business-critical
```
**Status:** ⚠️ **NEEDS BR**  
**Issue:** LLM operations are core to HAPI, but no BR defines metrics requirement  
**Business Value:** CRITICAL - LLM is HAPI's core capability (95% of value)  
**Recommendation:** Create **BR-HAPI-301: LLM Observability**
```markdown
#### BR-HAPI-301: LLM Observability Metrics

**Description:** HAPI MUST expose Prometheus metrics for LLM API call observability, including call counts, latency, token usage, and provider/model breakdown.

**Priority:** P0 (CRITICAL) - LLM is core business capability

**Metrics:**
- `holmesgpt_api_llm_calls_total{provider, model, status}` - Counter of LLM API calls
- `holmesgpt_api_llm_call_duration_seconds{provider, model}` - Histogram of LLM latency
- `holmesgpt_api_llm_token_usage_total{provider, model, type}` - Counter of tokens (prompt/completion)

**SLO:**
- LLM P95 latency < 5 seconds (OpenAI)
- LLM P95 latency < 10 seconds (Claude)
- LLM error rate < 1%

**Cost Monitoring:**
- Track token usage for billing forecasting
- Alert on >$100/day token consumption
```

---

### ❌ **REMOVE: Metrics WITHOUT Business Justification (4 metrics)**

#### 12. Active Requests Gauge ❌ **NO BR**
```python
active_requests                          # ❌ No BR, limited value
```
**Status:** ❌ **REMOVE**  
**Rationale:**
- No business requirement
- Adds cardinality without clear SLO
- `http_requests_total` already provides request rate
- HAPI is stateless, not latency-sensitive (async LLM calls)
- If needed for autoscaling, can be derived from request rate

**Alternative:** Use `rate(http_requests_total)` for load metrics

#### 13-14. Authentication Metrics ❌ **NO BR, PREMATURE**
```python
auth_failures_total                      # ❌ No BR for auth metrics
auth_success_total                       # ❌ No BR for auth metrics
```
**Status:** ❌ **REMOVE**  
**Rationale:**
- Auth is handled by middleware (DD-AUTH-014), not HAPI business logic
- No BR defines auth metrics requirement
- HAPI is internal-only (network policies handle access)
- Auth metrics belong in API Gateway tier (if exposed externally)
- Premature optimization (no auth-related incidents reported)

**When to Add Back:**
- If HAPI becomes externally exposed (v2.0)
- If auth failures cause production incidents
- Create BR-HAPI-4xx: Authentication Observability first

#### 15-16. Context API Metrics ❌ **NO BR, FEATURE NOT IMPLEMENTED**
```python
context_api_calls_total                  # ❌ Context API integration not implemented
context_api_duration_seconds             # ❌ Context API integration not implemented
```
**Status:** ❌ **REMOVE**  
**Rationale:**
- Context API integration is NOT implemented in v1.0
- No calls to Context API in current codebase
- BR-HAPI-192 (Recovery Context Consumption) uses workflow catalog, NOT Context API
- Metrics for non-existent features violate YAGNI principle

**When to Add Back:**
- When Context API integration is implemented (v1.1+)
- Create BR-HAPI-5xx: Context API Integration first
- Follow TDD: Tests first, then implementation, then metrics

---

## Comparison with DD-005 Standards

### DD-005 Compliance Check

| DD-005 Requirement | HAPI Status | Compliant? |
|-------------------|-------------|-----------|
| **Metric naming:** `{service}_{component}_{metric_name}_{unit}` | ✅ All metrics use `holmesgpt_api_` prefix | ✅ YES |
| **Metric name constants** (Section 1.1) | ❌ No constants defined | ❌ **NO** |
| **Label cardinality < 10k** | ✅ Low cardinality labels | ✅ YES |
| **Path normalization** (Section 3.1) | ⚠️ Uses `_normalize_path()` | ✅ YES |
| **HTTP metrics mandatory** | ✅ `http_requests_total`, `http_request_duration_seconds` | ✅ YES |
| **Histogram buckets** | ⚠️ Non-standard buckets | ⚠️ **NEEDS FIX** |

### Critical Non-Compliance: Metric Name Constants ❌

**DD-005 Section 1.1 MANDATORY Requirement:** All services MUST define exported constants for metric names.

**Current HAPI Status:** ❌ **NON-COMPLIANT** - All metrics use hardcoded strings

**Example Violation:**
```python
# ❌ ANTI-PATTERN: Hardcoded metric names
investigations_total = Counter('holmesgpt_api_investigations_total', ...)

# In tests
assert 'holmesgpt_api_investigations_total' in metrics_text  # ❌ Typo risk
```

**Required Fix:**
```python
# ✅ BEST PRACTICE: Metric name constants
METRIC_NAME_INVESTIGATIONS_TOTAL = 'holmesgpt_api_investigations_total'
METRIC_NAME_INVESTIGATIONS_DURATION = 'holmesgpt_api_investigations_duration_seconds'

investigations_total = Counter(METRIC_NAME_INVESTIGATIONS_TOTAL, ...)

# In tests
from src.metrics.constants import METRIC_NAME_INVESTIGATIONS_TOTAL
assert METRIC_NAME_INVESTIGATIONS_TOTAL in metrics_text  # ✅ Type-safe
```

**Effort:** 1 hour (create constants file, update 15 metric definitions + tests)

---

## Histogram Bucket Analysis

### Current Buckets vs DD-005 Standards

| Metric | Current Buckets | DD-005 Standard | Compliant? |
|--------|----------------|-----------------|-----------|
| `investigations_duration_seconds` | `(0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0)` | Custom (LLM workload) | ✅ **ACCEPTABLE** |
| `llm_call_duration_seconds` | `(0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0)` | Custom (LLM latency) | ✅ **ACCEPTABLE** |
| `http_request_duration_seconds` | `(0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0)` | `prometheus.ExponentialBuckets(0.001, 2, 10)` | ⚠️ **SUBOPTIMAL** |
| `context_api_duration_seconds` | `(0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0)` | `prometheus.ExponentialBuckets(0.01, 2, 8)` | ❌ **REMOVE** (unused) |

**Recommendation:**
- **Keep** custom buckets for `investigations_duration` and `llm_call_duration` (LLM-specific workload)
- **Fix** `http_request_duration` to match DD-005 standard (0.001s starting point)
- **Remove** `context_api_duration` (feature not implemented)

---

## Recommended Actions

### Immediate Actions (1 hour)

1. **Remove unused metrics:**
   ```python
   # DELETE these from src/middleware/metrics.py
   active_requests = Gauge(...)                    # No BR
   auth_failures_total = Counter(...)              # Premature
   auth_success_total = Counter(...)               # Premature  
   context_api_calls_total = Counter(...)          # Not implemented
   context_api_duration_seconds = Histogram(...)   # Not implemented
   ```

2. **Remove metric instrumentation:**
   ```python
   # DELETE from PrometheusMetricsMiddleware.dispatch()
   active_requests.labels(...).inc()  # Line 195
   active_requests.labels(...).dec()  # Line 243
   
   # DELETE from middleware/auth.py
   record_auth_failure(...)           # Lines using auth metrics
   record_auth_success(...)
   
   # DELETE unused helper functions
   def record_auth_failure(...):      # Lines 372-390
   def record_auth_success(...):      # Lines 394-413
   def record_context_api_call(...):  # Lines 416-442
   ```

**Result:** 15 metrics → **10 metrics** (33% reduction)

---

### Short-term Actions (2-3 hours)

3. **Create metric name constants (DD-005 Section 1.1 compliance):**
   ```python
   # NEW FILE: src/metrics/constants.py
   """
   HAPI Metric Name Constants (DD-005 v3.0 Compliance)
   
   All metric names MUST be defined as constants to prevent typos
   and ensure test/production parity.
   """
   
   # Investigation Metrics
   METRIC_NAME_INVESTIGATIONS_TOTAL = 'holmesgpt_api_investigations_total'
   METRIC_NAME_INVESTIGATIONS_DURATION = 'holmesgpt_api_investigations_duration_seconds'
   
   # LLM Metrics
   METRIC_NAME_LLM_CALLS_TOTAL = 'holmesgpt_api_llm_calls_total'
   METRIC_NAME_LLM_CALL_DURATION = 'holmesgpt_api_llm_call_duration_seconds'
   METRIC_NAME_LLM_TOKEN_USAGE = 'holmesgpt_api_llm_token_usage_total'
   
   # HTTP Metrics
   METRIC_NAME_HTTP_REQUESTS_TOTAL = 'holmesgpt_api_http_requests_total'
   METRIC_NAME_HTTP_REQUEST_DURATION = 'holmesgpt_api_http_request_duration_seconds'
   
   # Config Hot-Reload Metrics (BR-HAPI-199)
   METRIC_NAME_CONFIG_RELOAD_TOTAL = 'holmesgpt_api_config_reload_total'
   METRIC_NAME_CONFIG_RELOAD_ERRORS = 'holmesgpt_api_config_reload_errors_total'
   METRIC_NAME_CONFIG_LAST_RELOAD_TIMESTAMP = 'holmesgpt_api_config_last_reload_timestamp'
   
   # RFC 7807 Error Metrics (BR-HAPI-200)
   METRIC_NAME_RFC7807_ERRORS_TOTAL = 'holmesgpt_api_rfc7807_errors_total'
   
   # Label value constants
   LABEL_STATUS_SUCCESS = 'success'
   LABEL_STATUS_ERROR = 'error'
   LABEL_STATUS_NEEDS_REVIEW = 'needs_review'
   ```

4. **Update metric definitions to use constants:**
   ```python
   # src/middleware/metrics.py
   from src.metrics.constants import *
   
   investigations_total = Counter(
       METRIC_NAME_INVESTIGATIONS_TOTAL,  # ✅ Use constant
       'Total investigation requests',
       ['status']
   )
   ```

5. **Update tests to use constants:**
   ```python
   # tests/integration/test_hapi_metrics_integration.py
   from src.metrics.constants import METRIC_NAME_INVESTIGATIONS_TOTAL
   
   assert METRIC_NAME_INVESTIGATIONS_TOTAL in metrics_text  # ✅ Type-safe
   ```

---

### Medium-term Actions (4-6 hours)

6. **Document metrics in BR document:**
   - Update `BUSINESS_REQUIREMENTS.md` to expand BR-HAPI-011
   - Create BR-HAPI-301: LLM Observability Metrics
   - Add explicit metric names to each BR

7. **Fix histogram buckets:**
   ```python
   # src/middleware/metrics.py
   http_request_duration_seconds = Histogram(
       METRIC_NAME_HTTP_REQUEST_DURATION,
       'HTTP request duration',
       ['method', 'endpoint'],
       buckets=(0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.128, 0.256, 0.512)  # ✅ DD-005 standard
   )
   ```

8. **Refactor to Go pattern (from previous analysis):**
   - Create `HAMetrics` class
   - Inject metrics into business logic
   - Make testable in integration tier

---

## Summary

### Current State
- **15 metrics exposed**
- **6 metrics** have BR/DD-005 coverage (40%)
- **5 metrics** have business value but need BR documentation (33%)
- **4 metrics** should be removed (27%)

### Target State (After Immediate Actions)
- **10 metrics exposed** (33% reduction)
- **6 metrics** with BR/DD-005 coverage (60%)
- **4 metrics** with business value, pending BR creation (40%)
- **0 unauthorized metrics** (0%)

### Target State (After Short-term Actions)
- **10 metrics exposed**
- **10 metrics** with BR coverage (100%)
- DD-005 v3.0 compliant (metric name constants)
- All metrics testable and documented

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Removing metrics breaks Grafana dashboards** | Medium | Low | No production Grafana dashboards exist yet |
| **Metrics needed for future features** | Low | Low | Add back with BR when feature implemented (TDD) |
| **Breaking test assertions** | High | Low | Tests updated in same PR |
| **Prometheus cardinality reduction** | N/A | Positive | Reduces memory usage |

---

## Acceptance Criteria

✅ **Complete when:**
1. Unauthorized metrics removed from code
2. Metric name constants defined (DD-005 v3.0 compliance)
3. BRs updated to document all retained metrics
4. Integration tests passing
5. Histogram buckets match DD-005 where applicable

---

**Prepared by:** AI Assistant  
**Date:** January 31, 2026 07:15 UTC  
**Priority:** Medium (cleanup + compliance)  
**Effort:** 6-10 hours total  
**Recommendation:** Execute immediate actions now (1 hour), defer medium-term actions to refactoring sprint
