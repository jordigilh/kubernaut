# Business Requirements: HTTP Server & Observability

**Document Version**: 1.0
**Date**: October 29, 2025
**Status**: ✅ APPROVED
**Related**: IMPLEMENTATION_PLAN_V2.19.md, GATEWAY_SERVICE_SPEC.md

---

## 📋 **Purpose**

This document formally defines Business Requirements for:
1. **HTTP Server** (BR-GATEWAY-036 to BR-GATEWAY-045): 10 BRs
2. **Observability** (BR-GATEWAY-101 to BR-GATEWAY-110): 10 BRs

**Total**: 20 new BRs for Gateway V1.0

---

## 🚨 **BR Range Conflict Resolution**

### **Problem Identified**

IMPLEMENTATION_PLAN_V2.19.md had conflicting BR assignments:
- **Health & Observability** claimed BR-016 to BR-025 (10 BRs)
- **BUT** BR-016 to BR-023 were already assigned to Signal Ingestion features:
  - BR-016: Storm aggregation (1-minute window)
  - BR-017: Return HTTP 201 for new CRD creation
  - BR-018: Return HTTP 202 for duplicate signals
  - BR-019: Return HTTP 400 for invalid signal payloads
  - BR-020: Return HTTP 500 for processing errors
  - BR-021: Record signal metadata in CRD
  - BR-022: Support adapter-specific routes
  - BR-023: Dynamic adapter registration
- **AND** BR-024 to BR-040 were deferred to v1.1 (OpenTelemetry)

### **Resolution**

**Observability BRs** use **NEW RANGE**: **BR-GATEWAY-101 to BR-GATEWAY-110**

This avoids conflicts and provides clear separation:
- **BR-001 to BR-099**: Core Gateway functionality (v1.0)
- **BR-100+**: Cross-cutting concerns (observability, monitoring, diagnostics)

---

## 🌐 **HTTP Server Business Requirements**

### **Category Overview**

**BR Range**: BR-GATEWAY-036 to BR-GATEWAY-045
**Count**: 10 BRs
**Implementation**: `pkg/gateway/server.go`
**Test Tier**: Integration (primary), Unit (secondary)

---

### **BR-GATEWAY-036: HTTP Server Startup**

**Business Capability**: Gateway accepts HTTP connections on configured port

**Business Value**: Enables signal ingestion from external sources (Prometheus, Kubernetes)

**Acceptance Criteria**:
- Server starts within 5 seconds of `Start()` call
- Server responds to HTTP requests within 100ms of startup
- Server binds to configured port (default: 8080)
- Server logs startup success with port number

**Implementation Reference**: `pkg/gateway/server.go:Start()`

**Test Strategy**:
- **Integration**: Start server, send HTTP request, verify 200 OK response
- **Unit**: Verify ServerConfig validation

**Business Outcome**: Operators can deploy Gateway and receive signals immediately

---

### **BR-GATEWAY-037: HTTP ReadTimeout Protection**

**Business Capability**: Gateway protects against slow-read attacks

**Business Value**: Prevents resource exhaustion from malicious/slow clients

**Acceptance Criteria**:
- Requests taking >ReadTimeout to send body are terminated
- Server returns 408 Request Timeout
- Server logs timeout event with client IP
- Default ReadTimeout: 30 seconds

**Implementation Reference**: `pkg/gateway/server.go:ReadTimeout`

**Test Strategy**:
- **Integration**: Send request with slow body transmission (>ReadTimeout), verify 408 response
- **Unit**: Verify ReadTimeout configuration parsing

**Business Outcome**: Gateway remains available during slow-client attacks

---

### **BR-GATEWAY-038: HTTP WriteTimeout Protection**

**Business Capability**: Gateway protects against slow-write attacks

**Business Value**: Prevents resource exhaustion from slow response consumers

**Acceptance Criteria**:
- Responses taking >WriteTimeout to send are terminated
- Connection is closed after timeout
- Server logs timeout event
- Default WriteTimeout: 30 seconds

**Implementation Reference**: `pkg/gateway/server.go:WriteTimeout`

**Test Strategy**:
- **Integration**: Send request with slow response consumer (>WriteTimeout), verify connection closed
- **Unit**: Verify WriteTimeout configuration parsing

**Business Outcome**: Gateway remains responsive during slow-consumer scenarios

---

### **BR-GATEWAY-039: HTTP IdleTimeout Connection Management**

**Business Capability**: Gateway closes idle keep-alive connections

**Business Value**: Prevents connection pool exhaustion from idle clients

**Acceptance Criteria**:
- Connections idle >IdleTimeout are closed
- Server logs connection closure
- Default IdleTimeout: 120 seconds
- Active connections are NOT affected

**Implementation Reference**: `pkg/gateway/server.go:IdleTimeout`

**Test Strategy**:
- **Integration**: Establish connection, wait >IdleTimeout, verify connection closed
- **Unit**: Verify IdleTimeout configuration parsing

**Business Outcome**: Gateway maintains healthy connection pool under varying load

---

### **BR-GATEWAY-040: Graceful Shutdown**

**Business Capability**: Gateway shuts down without dropping in-flight requests

**Business Value**: Zero data loss during deployments/restarts

**Acceptance Criteria**:
- Server stops accepting new requests immediately on shutdown signal
- In-flight requests complete successfully (up to shutdown timeout)
- Server waits up to shutdown timeout (default: 30s) for completion
- Server logs shutdown start and completion
- Kubernetes readiness probe fails immediately on shutdown

**Implementation Reference**: `pkg/gateway/server.go:Shutdown()`

**Test Strategy**:
- **Integration**: Send background requests, trigger shutdown, verify in-flight requests complete
- **Unit**: Verify shutdown timeout configuration

**Business Outcome**: Zero signal loss during Gateway deployments

---

### **BR-GATEWAY-041: RFC 7807 Error Responses**

**Business Capability**: Gateway returns structured error responses

**Business Value**: Enables automated error handling by clients

**Acceptance Criteria**:
- 4xx/5xx responses follow RFC 7807 Problem Details format
- Response includes: `type`, `title`, `detail`, `status`, `instance`
- Content-Type: `application/problem+json`
- Error details are sanitized (no sensitive data)

**Implementation Reference**: `pkg/gateway/server.go:writeErrorResponse()`

**Test Strategy**:
- **Unit**: Verify error response structure for various error types
- **Integration**: Trigger errors, verify RFC 7807 compliance

**Business Outcome**: Clients can programmatically handle Gateway errors

---

### **BR-GATEWAY-042: Content-Type Validation**

**Business Capability**: Gateway validates webhook Content-Type headers

**Business Value**: Prevents processing of invalid payloads

**Acceptance Criteria**:
- Non-JSON requests to webhook endpoints return 415 Unsupported Media Type
- Accepted Content-Types: `application/json`, `application/json; charset=utf-8`
- Response includes `Accept` header with supported types
- Server logs invalid Content-Type attempts

**Implementation Reference**: `pkg/gateway/server.go:createAdapterHandler()`

**Test Strategy**:
- **Unit**: Verify Content-Type validation logic
- **Integration**: Send requests with various Content-Types, verify 415 for invalid

**Business Outcome**: Gateway rejects invalid webhook payloads early

---

### **BR-GATEWAY-043: HTTP Method Validation**

**Business Capability**: Gateway enforces POST-only for webhook endpoints

**Business Value**: Prevents accidental GET requests exposing webhook URLs

**Acceptance Criteria**:
- Non-POST requests to webhook endpoints return 405 Method Not Allowed
- Response includes `Allow: POST` header
- Health endpoints support GET only
- Server logs invalid method attempts

**Implementation Reference**: `pkg/gateway/server.go:createAdapterHandler()`

**Test Strategy**:
- **Unit**: Verify method validation logic
- **Integration**: Send GET/PUT/DELETE to webhooks, verify 405 response

**Business Outcome**: Gateway enforces correct webhook usage patterns

---

### **BR-GATEWAY-044: Request Body Size Limits**

**Business Capability**: Gateway enforces maximum request body size

**Business Value**: Prevents memory exhaustion from large payloads

**Acceptance Criteria**:
- Requests >MaxBodySize return 413 Request Entity Too Large
- Default MaxBodySize: 1MB (configurable)
- Response includes `Retry-After` header if applicable
- Server logs oversized request attempts with size

**Implementation Reference**: `pkg/gateway/server.go:ServerConfig.MaxBodySize`

**Test Strategy**:
- **Integration**: Send requests with various body sizes, verify 413 for oversized
- **Unit**: Verify MaxBodySize configuration parsing

**Business Outcome**: Gateway remains stable under large payload attacks

---

### **BR-GATEWAY-045: Concurrent Request Handling**

**Business Capability**: Gateway handles concurrent requests without degradation

**Business Value**: Maintains performance under high load

**Acceptance Criteria**:
- Server handles 100 concurrent requests without errors
- p95 latency remains <500ms under 100 concurrent requests
- No connection refused errors under concurrent load
- Server logs concurrent request metrics

**Implementation Reference**: `pkg/gateway/server.go:http.Server`

**Test Strategy**:
- **Integration**: Send 100 concurrent requests, verify all succeed with acceptable latency
- **Load**: Stress test with 1000+ concurrent requests (test/load tier)

**Business Outcome**: Gateway scales to production alert volumes

---

## 📊 **Observability Business Requirements**

### **Category Overview**

**BR Range**: BR-GATEWAY-101 to BR-GATEWAY-110
**Count**: 10 BRs
**Implementation**: `pkg/gateway/metrics/metrics.go`, `pkg/gateway/server.go`
**Test Tier**: Integration (primary)

---

### **BR-GATEWAY-101: Prometheus Metrics Endpoint**

**Business Capability**: Gateway exposes operational metrics via `/metrics`

**Business Value**: Enables Prometheus scraping for monitoring and alerting

**Acceptance Criteria**:
- `/metrics` endpoint returns Prometheus text format
- Endpoint responds within 100ms
- Metrics include all Gateway operational data
- Endpoint supports GET method only
- No authentication required (network-level security)

**Implementation Reference**: `pkg/gateway/server.go:Handler()`, Prometheus `/metrics` handler

**Test Strategy**:
- **Integration**: Query `/metrics`, verify Prometheus text format and key metrics present

**Business Outcome**: Operators can scrape Gateway metrics into Prometheus

---

### **BR-GATEWAY-102: Alert Ingestion Metrics**

**Business Capability**: Gateway tracks alert ingestion statistics

**Business Value**: Enables monitoring of signal processing pipeline

**Acceptance Criteria**:
- `gateway_alerts_received_total{source, namespace}` increments on each alert
- `gateway_alerts_deduplicated_total{source, namespace}` increments on duplicates
- `gateway_alert_storms_detected_total{source, namespace}` increments on storm detection
- Metrics include labels: `source` (prometheus/kubernetes), `namespace`
- Metrics persist across Gateway restarts (via Prometheus scraping)

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:AlertsReceivedTotal`

**Test Strategy**:
- **Integration**: Send alerts, query metrics, verify counters increment correctly

**Business Outcome**: Operators can detect alert storms via Prometheus query: `rate(gateway_alerts_received_total[1m]) > 10`

---

### **BR-GATEWAY-103: CRD Creation Metrics**

**Business Capability**: Gateway tracks CRD creation success/failure rates

**Business Value**: Enables SLO tracking for CRD creation (target: 99.9% success)

**Acceptance Criteria**:
- `gateway_crds_created_total{namespace, priority}` increments on successful CRD creation
- `gateway_crd_creation_errors{namespace, error_type}` increments on failures
- Metrics include labels: `namespace`, `priority` (P0/P1/P2/P3), `error_type`
- Error types: `k8s_api_error`, `validation_error`, `conflict_error`

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:CRDsCreatedTotal`

**Test Strategy**:
- **Integration**: Create CRDs (success/failure scenarios), verify metrics increment

**Business Outcome**: Operators can track CRD creation SLO: `sum(rate(gateway_crds_created_total[5m])) / sum(rate(gateway_alerts_received_total[5m])) > 0.999`

---

### **BR-GATEWAY-104: HTTP Request Duration Metrics**

**Business Capability**: Gateway tracks HTTP request latency distribution

**Business Value**: Enables p95 latency SLO tracking (target: <500ms)

**Acceptance Criteria**:
- `gateway_http_request_duration_seconds{endpoint, method, status}` histogram
- Buckets: 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0 seconds
- Metrics include labels: `endpoint`, `method`, `status` (2xx/4xx/5xx)
- Supports Prometheus `histogram_quantile()` for p50/p95/p99 calculation

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:HTTPRequestDuration`

**Test Strategy**:
- **Integration**: Send requests, query histogram, verify p95 calculation

**Business Outcome**: Operators can track latency SLO: `histogram_quantile(0.95, gateway_http_request_duration_seconds) < 0.5`

---

### **BR-GATEWAY-105: Redis Operation Duration Metrics**

**Business Capability**: Gateway tracks Redis operation latency

**Business Value**: Enables Redis performance monitoring and bottleneck detection

**Acceptance Criteria**:
- `gateway_redis_operation_duration_seconds{operation}` histogram
- Operations: `get`, `set`, `expire`, `del`, `hgetall`, `hset`
- Buckets: 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0 seconds
- Supports p95 latency tracking per operation

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:RedisOperationDuration`

**Test Strategy**:
- **Integration**: Perform Redis operations, verify histogram updates

**Business Outcome**: Operators can detect Redis performance degradation: `histogram_quantile(0.95, gateway_redis_operation_duration_seconds{operation="set"}) > 0.05`

---

### **BR-GATEWAY-106: Redis Health Metrics**

**Business Capability**: Gateway tracks Redis availability and outages

**Business Value**: Enables Redis availability SLO tracking (target: 99.9%)

**Acceptance Criteria**:
- `gateway_redis_available` gauge (1=available, 0=unavailable)
- `gateway_redis_outage_count` counter (increments on each outage)
- `gateway_redis_outage_duration_seconds` counter (cumulative outage time)
- Metrics update within 5 seconds of Redis state change

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:RedisAvailable`

**Test Strategy**:
- **Integration**: Simulate Redis failure/recovery, verify metrics update

**Business Outcome**: Operators can track Redis availability SLO: `avg_over_time(gateway_redis_available[30d]) > 0.999`

---

### **BR-GATEWAY-107: Redis Pool Metrics**

**Business Capability**: Gateway tracks Redis connection pool health

**Business Value**: Enables connection pool tuning and leak detection

**Acceptance Criteria**:
- `gateway_redis_pool_connections_total` gauge (total pool size)
- `gateway_redis_pool_connections_idle` gauge (idle connections)
- `gateway_redis_pool_connections_active` gauge (active connections)
- `gateway_redis_pool_hits_total` counter (cache hits)
- `gateway_redis_pool_misses_total` counter (cache misses)
- `gateway_redis_pool_timeouts_total` counter (connection timeouts)

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:RedisPool*`

**Test Strategy**:
- **Integration**: Perform Redis operations, verify pool metrics update

**Business Outcome**: Operators can detect connection leaks: `gateway_redis_pool_connections_active` continuously increasing

---

### **BR-GATEWAY-108: HTTP In-Flight Requests Metric**

**Business Capability**: Gateway tracks concurrent request count

**Business Value**: Enables load monitoring and capacity planning

**Acceptance Criteria**:
- `gateway_http_requests_in_flight` gauge (current concurrent requests)
- Metric increments on request start, decrements on request completion
- Metric updates in real-time (<100ms latency)
- Supports max concurrency alerting

**Implementation Reference**: `pkg/gateway/metrics/metrics.go:HTTPRequestsInFlight`

**Test Strategy**:
- **Integration**: Send concurrent requests, verify gauge tracks concurrency accurately

**Business Outcome**: Operators can detect overload: `gateway_http_requests_in_flight > 100` triggers alert

---

### **BR-GATEWAY-109: Structured Logging with Request Context**

**Business Capability**: Gateway logs include request tracing context

**Business Value**: Enables distributed tracing and request debugging

**Acceptance Criteria**:
- All logs include: `request_id`, `source_ip`, `endpoint`, `duration_ms`
- Request ID propagates through entire request lifecycle
- Logs use structured format (JSON in production, console in dev)
- Log levels: DEBUG, INFO, WARN, ERROR
- Sensitive data is sanitized (no secrets, tokens, passwords)

**Implementation Reference**: `pkg/gateway/server.go` logging, `pkg/gateway/middleware/`

**Test Strategy**:
- **Unit**: Verify log structure and sanitization
- **Integration**: Send request, verify logs include context

**Business Outcome**: Operators can trace requests across Gateway components using `request_id`

---

### **BR-GATEWAY-110: Health Endpoints**

**Business Capability**: Gateway exposes health status for Kubernetes probes

**Business Value**: Enables Kubernetes liveness/readiness detection

**Acceptance Criteria**:
- `/health` (liveness): Returns `{"status": "healthy", "timestamp": "..."}` if Gateway is running
- `/healthz` (liveness alias): Kubernetes-style alias for `/health`
- `/ready` (readiness): Returns `{"status": "ready"}` if Gateway can process requests
- Readiness checks: Redis connectivity, Kubernetes API connectivity
- Health endpoints respond within 100ms
- Health endpoints support GET method only

**Implementation Reference**: `pkg/gateway/server.go:healthHandler()`, `readinessHandler()`

**Test Strategy**:
- **Integration**: Query health endpoints, verify responses and status codes
- **Integration**: Simulate Redis failure, verify `/ready` returns 503

**Business Outcome**: Kubernetes can detect unhealthy Gateway pods and restart them automatically

---

## 📈 **BR Coverage Summary**

### **HTTP Server BRs**

| BR | Description | Test Tier | Estimated Tests |
|----|-------------|-----------|-----------------|
| BR-036 | HTTP Server Startup | Integration | 2 tests |
| BR-037 | ReadTimeout Protection | Integration | 2 tests |
| BR-038 | WriteTimeout Protection | Integration | 2 tests |
| BR-039 | IdleTimeout Management | Integration | 2 tests |
| BR-040 | Graceful Shutdown | Integration | 3 tests |
| BR-041 | RFC 7807 Error Responses | Unit | 4 tests |
| BR-042 | Content-Type Validation | Unit + Integration | 3 tests |
| BR-043 | HTTP Method Validation | Unit + Integration | 3 tests |
| BR-044 | Body Size Limits | Integration | 2 tests |
| BR-045 | Concurrent Handling | Integration | 2 tests |
| **Total** | **10 BRs** | **Mixed** | **25 tests** |

### **Observability BRs**

| BR | Description | Test Tier | Estimated Tests |
|----|-------------|-----------|-----------------|
| BR-101 | Prometheus Metrics Endpoint | Integration | 2 tests |
| BR-102 | Alert Ingestion Metrics | Integration | 3 tests |
| BR-103 | CRD Creation Metrics | Integration | 3 tests |
| BR-104 | HTTP Duration Metrics | Integration | 2 tests |
| BR-105 | Redis Duration Metrics | Integration | 2 tests |
| BR-106 | Redis Health Metrics | Integration | 3 tests |
| BR-107 | Redis Pool Metrics | Integration | 3 tests |
| BR-108 | In-Flight Requests | Integration | 2 tests |
| BR-109 | Structured Logging | Unit + Integration | 6 tests ✅ (3 failing, 3 passing) |
| BR-110 | Health Endpoints | Integration | 3 tests |
| **Total** | **10 BRs** | **Mixed** | **26 tests** |

### **Combined Totals**

- **Total BRs**: 20
- **Total Tests**: 51 (25 HTTP + 26 Observability)
- **Test Distribution**: ~35 integration, ~16 unit
- **Estimated Effort**: 11-15 hours (including infrastructure)

---

## 🔗 **Integration with Existing Documentation**

### **Updates Required**

1. **IMPLEMENTATION_PLAN_V2.19.md**:
   - Update BR range table to remove BR-016 to BR-025 conflict
   - Add BR-036 to BR-045 (HTTP Server) definitions
   - Add BR-101 to BR-110 (Observability) definitions
   - Update test count estimates

2. **GATEWAY_SERVICE_SPEC.md**:
   - Add HTTP Server section with BR references
   - Add Observability section with BR references
   - Update API documentation with error response formats (RFC 7807)

3. **README.md**:
   - Update BR coverage table
   - Add HTTP Server and Observability to feature list

---

## ✅ **Approval Status**

**Approved By**: User
**Date**: October 29, 2025
**Next Steps**:
1. Update IMPLEMENTATION_PLAN_V2.19.md with new BR definitions
2. Build test infrastructure (Phase 1)
3. Implement HTTP Server tests (Phase 2)
4. Implement Observability tests (Phase 3)

---

## 📝 **Change History**

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | Oct 29, 2025 | Initial BR definitions for HTTP Server and Observability | AI Assistant |
| 1.1 | Oct 29, 2025 | BR-109 test infrastructure complete, 6 tests active (3 failing, 3 passing) | AI Assistant |

---

## 🎯 **BR-109 Implementation Status** (October 29, 2025)

### **Infrastructure Complete** ✅
- **Log Capture**: `test/integration/gateway/log_capture.go` (328 lines)
- **Test Helpers**: Updated `StartTestGateway()` with logger injection
- **Tests Active**: `test/unit/gateway/observability_test.go` (6 tests, 406 lines)

### **Test Results**
| Test | Status | Business Outcome | Implementation Gap |
|------|--------|------------------|-------------------|
| request_id in logs | ❌ FAILING | Operators cannot trace requests | Add request_id middleware |
| source_ip in logs | ❌ FAILING | Operators cannot audit sources | Add source_ip to log context |
| endpoint + duration_ms | ❌ FAILING | Operators cannot analyze performance | Add performance metrics to logs |
| JSON format | ✅ PASSING | Logs are machine-readable | ✅ Implemented |
| Sensitive data sanitization | ✅ PASSING | Logs don't leak secrets | ✅ Implemented |
| Log level control | ✅ PASSING | Operators control verbosity | ✅ Implemented |

### **Next Action**
Implement Gateway structured logging (request_id, source_ip, endpoint, duration_ms) to make all BR-109 tests pass.

**Full Details**: See `BR109_LOG_CAPTURE_IMPLEMENTATION_COMPLETE.md`

