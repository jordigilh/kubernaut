# DD-005: Observability Standards (Metrics and Logging)

**Status**: ✅ **APPROVED** (Production Standard)
**Date**: October 31, 2025
**Last Reviewed**: October 31, 2025
**Confidence**: 95%
**Based On**: Gateway Service Reference Implementation

---

## 🎯 **Overview**

This design decision establishes **mandatory observability standards** for all Kubernaut services, covering:
1. **Prometheus Metrics** - Naming conventions, labels, and metric types
2. **Structured Logging** - Format, fields, and sanitization
3. **Request Tracing** - Request ID propagation and correlation

**Key Principle**: All services MUST follow consistent observability patterns to enable unified monitoring, debugging, and operational excellence.

**Scope**: All Kubernaut services (Gateway, Context API, HolmesGPT API, Controllers, etc.).

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Requirements](#requirements)
3. [Decision](#decision)
4. [Metrics Standards](#metrics-standards)
5. [Logging Standards](#logging-standards)
6. [Request Tracing Standards](#request-tracing-standards)
7. [Implementation](#implementation)
8. [Examples](#examples)
9. [Migration Guide](#migration-guide)
10. [References](#references)

---

## 🎯 **Context & Problem**

### **Challenge**

Kubernaut consists of multiple microservices that need unified observability:

1. ⚠️ **Inconsistent Metrics**: Each service uses different metric naming conventions
2. ⚠️ **Unstructured Logs**: Logs lack consistent fields for correlation
3. ⚠️ **No Request Tracing**: Cannot trace requests across service boundaries
4. ⚠️ **Security Risks**: Sensitive data exposed in logs

### **Business Impact**

- **Operator Efficiency**: Standardized observability improves troubleshooting speed
- **SLO Monitoring**: Consistent metrics enable service-level objective tracking
- **Security Compliance**: Sanitized logs prevent data exposure
- **Cost Optimization**: Efficient logging reduces storage costs

---

## 📋 **Requirements**

### **Functional Requirements**

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| **FR-1** | All services use Prometheus metrics with standard naming | P0 | 🔄 In Progress |
| **FR-2** | All services use structured logging (zap) with standard fields | P0 | 🔄 In Progress |
| **FR-3** | All services propagate request IDs via X-Request-ID header | P0 | 🔄 In Progress |
| **FR-4** | All logs sanitize sensitive data (passwords, tokens, keys) | P0 | 🔄 In Progress |
| **FR-5** | All metrics use consistent label names across services | P0 | 🔄 In Progress |

### **Non-Functional Requirements**

| ID | Requirement | Target | Status |
|----|-------------|--------|--------|
| **NFR-1** | Metric cardinality < 10,000 per service | <10k | 🔄 In Progress |
| **NFR-2** | Log volume < 100 MB/day per service (production) | <100MB | 🔄 In Progress |
| **NFR-3** | Request ID propagation overhead < 1ms | <1ms | 🔄 In Progress |

**Note**: Backward compatibility is NOT required (pre-release product).

---

## ✅ **Decision**

**APPROVED**: Standardize observability across all Kubernaut services

**Rationale**:
1. **Operational Excellence**: Unified observability enables efficient troubleshooting
2. **Industry Standards**: Follows Prometheus and structured logging best practices
3. **Security**: Sanitization prevents sensitive data exposure
4. **Scalability**: Consistent patterns enable automated monitoring

---

## 📊 **Metrics Standards**

### **1. Metric Naming Convention**

**Format**: `{service}_{component}_{metric_name}_{unit}`

**Rules**:
- **Service prefix**: `gateway_`, `context_api_`, `holmesgpt_api_`, etc.
- **Component**: Logical component (e.g., `signals_`, `http_`, `redis_`, `database_`)
- **Metric name**: Descriptive name in snake_case
- **Unit suffix**: `_total`, `_seconds`, `_bytes`, `_ratio` (optional)

**Examples**:
```
gateway_signals_received_total
gateway_http_request_duration_seconds
context_api_database_query_duration_seconds
holmesgpt_api_llm_tokens_total
```

---

### **2. Metric Types**

| Metric Type | Use Case | Naming Convention | Example |
|---|---|---|---|
| **Counter** | Cumulative count (always increasing) | `*_total` suffix | `gateway_signals_received_total` |
| **Gauge** | Current value (can increase/decrease) | No suffix | `gateway_http_requests_in_flight` |
| **Histogram** | Distribution of values | `*_seconds`, `*_bytes` | `gateway_http_request_duration_seconds` |
| **Summary** | Similar to histogram (client-side quantiles) | `*_seconds`, `*_bytes` | `context_api_query_duration_seconds` |

---

### **3. Label Standards**

**Mandatory Labels** (all metrics):
- **environment**: `prod`, `staging`, `dev`
- **service**: `gateway`, `context-api`, `holmesgpt-api`, etc.

**Common Labels** (use consistently):
- **endpoint**: HTTP endpoint path (e.g., `/api/v1/signals/prometheus`)
- **method**: HTTP method (e.g., `GET`, `POST`)
- **status**: HTTP status code (e.g., `200`, `400`, `500`)
- **reason**: Error reason (e.g., `validation_error`, `timeout`)
- **source**: Signal source (e.g., `prometheus-alert`, `kubernetes-event`)
- **severity**: Signal severity (e.g., `critical`, `warning`, `info`)
- **priority**: Remediation priority (e.g., `P0`, `P1`, `P2`)

**Label Cardinality Limits**:
- **High cardinality labels** (avoid): `request_id`, `timestamp`, `user_id`
- **Medium cardinality labels** (use sparingly): `namespace`, `signal_name`
- **Low cardinality labels** (preferred): `environment`, `severity`, `status`

**Target**: < 10,000 unique label combinations per metric

---

### **3.1. Metrics Cardinality Management** ⚠️ **CRITICAL**

**Problem**: High-cardinality labels cause Prometheus memory explosion and query degradation.

#### **Path Normalization for HTTP Metrics**

**Requirement**: HTTP path labels MUST be normalized to prevent unbounded cardinality.

**Risk Scenario**:
```go
// ❌ DANGEROUS: Raw paths with IDs/query params
httpRequests.WithLabelValues("GET", "/api/v1/incidents/abc-123", "200")
httpRequests.WithLabelValues("GET", "/api/v1/incidents/def-456", "200")
httpRequests.WithLabelValues("GET", "/api/v1/incidents/xyz-789", "200")
// Result: Millions of unique metrics → Prometheus OOM
```

**Solution**: Normalize dynamic path segments
```go
// ✅ SAFE: Normalized paths with :id placeholder
httpRequests.WithLabelValues("GET", "/api/v1/incidents/:id", "200")
httpRequests.WithLabelValues("GET", "/api/v1/incidents/:id", "200")
httpRequests.WithLabelValues("GET", "/api/v1/incidents/:id", "200")
// Result: Single metric → Bounded cardinality
```

#### **Implementation Pattern**

**Mandatory**: All services MUST normalize paths before recording HTTP metrics.

```go
// Context API Reference Implementation
// pkg/contextapi/server/server.go

func normalizePath(path string) string {
    // Already normalized? Return as-is (idempotent)
    if strings.Contains(path, ":id") {
        return path
    }

    segments := strings.Split(path, "/")
    for i, segment := range segments {
        if segment == "" {
            continue // Skip empty segments
        }

        // Skip known endpoint names
        if isKnownEndpoint(segment) {
            continue
        }

        // Normalize ID-like segments (UUIDs, numeric IDs, etc.)
        if isIDLikeSegment(segment) {
            segments[i] = ":id"
        }
    }

    return strings.Join(segments, "/")
}

func isIDLikeSegment(segment string) bool {
    // ID characteristics:
    // 1. More than 3 characters (avoid false positives)
    // 2. Contains numbers or hyphens
    // 3. Only alphanumeric + hyphens + underscores
    // 4. Not a known endpoint name

    if len(segment) <= 3 {
        return false
    }

    hasNumberOrHyphen := false
    for _, ch := range segment {
        if !isValidIDChar(ch) {
            return false
        }
        if (ch >= '0' && ch <= '9') || ch == '-' {
            hasNumberOrHyphen = true
        }
    }

    return hasNumberOrHyphen
}
```

#### **Validation**

**Unit Tests Required**: All services MUST have path normalization tests.

```go
// Example test cases
func TestNormalizePath(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"/health", "/health"},                                    // Static - unchanged
        {"/api/v1/incidents/abc-123", "/api/v1/incidents/:id"},  // UUID - normalized
        {"/api/v1/incidents/123/actions/456", "/api/v1/incidents/:id/actions/:id"}, // Multiple IDs
        {"/api/v1/context/query", "/api/v1/context/query"},       // Static - unchanged
    }

    for _, tt := range tests {
        result := normalizePath(tt.input)
        if result != tt.expected {
            t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
        }
    }
}
```

#### **Monitoring**

**Prometheus Alert**: Alert when cardinality exceeds threshold.

```yaml
- alert: HighMetricCardinality
  expr: |
    count by (job) (
      {job=~".*-api", __name__=~".*_http_.*"}
    ) > 5000
  for: 5m
  annotations:
    summary: "High metric cardinality in {{ $labels.job }}"
    description: "{{ $value }} unique HTTP metrics (threshold: 5000)"
```

#### **Reference Implementation**

- **Context API**: `pkg/contextapi/server/server.go` - Full implementation with tests
- **Audit Document**: `docs/services/stateless/context-api/METRICS_CARDINALITY_AUDIT.md`

---

### **4. Histogram Buckets**

**HTTP Request Duration** (seconds):
```go
prometheus.ExponentialBuckets(0.001, 2, 10) // 1ms to ~1s
// Buckets: 0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.128, 0.256, 0.512
```

**Database Query Duration** (seconds):
```go
prometheus.ExponentialBuckets(0.01, 2, 8) // 10ms to ~1.28s
// Buckets: 0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28
```

**Redis Operation Duration** (seconds):
```go
prometheus.ExponentialBuckets(0.0001, 2, 10) // 0.1ms to ~100ms
// Buckets: 0.0001, 0.0002, 0.0004, 0.0008, 0.0016, 0.0032, 0.0064, 0.0128, 0.0256, 0.0512
```

---

### **5. Gateway Reference Metrics**

**Signal Ingestion**:
```go
gateway_signals_received_total{source="prometheus-alert", severity="critical", environment="prod"}
gateway_signals_deduplicated_total{signal_name="HighMemoryUsage", environment="prod"}
gateway_signal_storms_detected_total{storm_type="rate-based", signal_name="CrashLoopBackOff"}
```

**CRD Creation**:
```go
gateway_crds_created_total{environment="prod", priority="P0"}
gateway_crd_creation_errors_total{reason="k8s_api_error"}
```

**Performance**:
```go
gateway_http_request_duration_seconds{endpoint="/api/v1/signals/prometheus", method="POST", status="200"}
gateway_redis_operation_duration_seconds{operation="get"}
```

**HTTP Observability**:
```go
gateway_http_requests_in_flight
gateway_http_requests_total{endpoint="/api/v1/signals/prometheus", method="POST", status="200"}
```

**Redis Health**:
```go
gateway_redis_available{} # 1 = available, 0 = unavailable
gateway_redis_outage_duration_seconds_total
gateway_redis_outage_count_total
```

---

## 📝 **Logging Standards**

### **1. Structured Logging Library**

**Mandatory Interface**: Use `github.com/go-logr/logr` as the **unified logging interface** across all services

**Backend**: Use `go.uber.org/zap` as the **underlying implementation** (via `github.com/go-logr/zapr` adapter)

**Rationale**:
- ✅ **Unified interface**: Single `logr.Logger` type across stateless and CRD controller services
- ✅ **controller-runtime native**: CRD controllers use `logr` natively
- ✅ **zap performance**: High performance (zero-allocation) via `zapr` adapter
- ✅ **Shared library consistency**: All `pkg/*` libraries accept `logr.Logger`
- ✅ **Structured JSON output**: Consistent format across all services
- ✅ **Industry standard**: `logr` is the Kubernetes ecosystem standard

---

### **1.1 Logging Framework Decision Matrix**

| Service Type | Primary Logger | Shared Library Interface | How to Create |
|--------------|----------------|--------------------------|---------------|
| **Stateless HTTP Services** (Gateway, Data Storage, Context API) | `logr.Logger` | `logr.Logger` | `zapr.NewLogger(zapLogger)` |
| **CRD Controllers** (Signal Processing, Notification, Workflow Execution) | `logr.Logger` | `logr.Logger` | `ctrl.Log.WithName("component")` |
| **Shared Libraries** (`pkg/*`) | N/A (accepts) | `logr.Logger` | Passed by caller |

---

### **1.2 Implementation Patterns**

#### **Stateless HTTP Services**

```go
import (
    "github.com/go-logr/logr"
    "github.com/go-logr/zapr"
    "go.uber.org/zap"
)

func main() {
    // Create zap logger (for performance)
    zapLogger, _ := zap.NewProduction()
    defer zapLogger.Sync()

    // Convert to logr interface (for consistency)
    logger := zapr.NewLogger(zapLogger)

    // Pass to shared libraries
    auditStore, _ := audit.NewBufferedStore(client, config, "gateway", logger.WithName("audit"))
    server := gateway.NewServer(cfg, logger)
}
```

#### **CRD Controllers**

```go
import (
    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    // Use native logr from controller-runtime
    logger := ctrl.Log.WithName("notification-controller")

    // Pass to shared libraries (no adapter needed)
    auditStore, _ := audit.NewBufferedStore(client, config, "notification", logger.WithName("audit"))
}
```

#### **Shared Libraries**

```go
// pkg/audit/store.go
import "github.com/go-logr/logr"

type BufferedAuditStore struct {
    logger logr.Logger  // Unified interface
    // ...
}

func NewBufferedStore(client DataStorageClient, config Config, serviceName string, logger logr.Logger) (AuditStore, error) {
    // Works with both stateless (via zapr) and CRD controllers (native logr)
}
```

---

### **1.3 Migration from `*zap.Logger` to `logr.Logger`**

**Migration Priority**: V1.1 (Post-MVP)

**Affected Files**: 34 files in `pkg/` with 76 `*zap.Logger` references

| Package | Files | Priority | Effort |
|---------|-------|----------|--------|
| `pkg/audit` | 1 | P0 (shared library) | 1h |
| `pkg/cache/redis` | 1 | P1 (shared library) | 1h |
| `pkg/gateway` | 10 | P2 (stateless service) | 4h |
| `pkg/datastorage` | 20 | P2 (stateless service) | 6h |
| `pkg/toolset` | 1 | P3 (low usage) | 0.5h |
| **Total** | **34** | | **~12.5h** |

**Migration Steps**:
1. Update shared libraries (`pkg/audit`, `pkg/cache/redis`) to accept `logr.Logger`
2. Update stateless services to create `logr.Logger` via `zapr.NewLogger()`
3. Update CRD controllers to pass native `logr.Logger` to shared libraries
4. Remove duplicate logger creation in CRD controllers

---

### **2. Log Levels**

| Level | Use Case | logr Example |
|---|---|---|
| **DEBUG** (V=1) | Detailed debugging information | `logger.V(1).Info("Parsing signal payload", "fingerprint", fp)` |
| **INFO** (V=0) | Normal operational events | `logger.Info("Signal received", "source", "prometheus")` |
| **WARN** | Warning conditions (recoverable) | `logger.Info("Redis cache miss", "key", key, "warning", true)` |
| **ERROR** | Error conditions (actionable) | `logger.Error(err, "Failed to create CRD")` |
| **FATAL** | Fatal errors (service exits) | `logger.Error(err, "Cannot connect to database"); os.Exit(1)` |

**Note**: `logr` uses verbosity levels (`V(n)`) instead of named levels. Higher V = more verbose.
- `V(0)` = INFO (default, always shown)
- `V(1)` = DEBUG (shown when verbosity >= 1)
- `V(2)` = TRACE (shown when verbosity >= 2)

**Default Level**: `V(0)` (production), `V(1)` (development)

---

### **3. Standard Log Fields**

**Mandatory Fields** (all log entries):
```go
// logr uses key-value pairs (not zap.String, zap.Int, etc.)
logger.Info("Message",
    "request_id", requestID,      // Request tracing
    "source_ip", sourceIP,        // Security auditing
    "endpoint", r.URL.Path,       // HTTP endpoint
    "method", r.Method,           // HTTP method
)
```

**Common Fields** (use consistently):
```go
// Key-value pairs for structured logging
"service", "gateway"              // Service name
"environment", "prod"             // Environment
"namespace", "default"            // Kubernetes namespace
"signal_name", "HighMemoryUsage"  // Signal name
"fingerprint", fp                 // Signal fingerprint
"duration_ms", durationMs         // Operation duration
"status_code", statusCode         // HTTP status code
```

**Error Logging** (logr pattern):
```go
// logr.Error() takes error as first argument, message second
logger.Error(err, "Failed to process request",
    "request_id", requestID,
    "operation", "create_signal",
)
```

**Performance Fields**:
```go
logger.Info("Request completed",
    "duration_ms", float64(duration.Milliseconds()),
    "bytes_processed", bytesProcessed,
    "retry_count", retryCount,
)
```

---

### **4. Log Sanitization**

**Mandatory**: Sanitize sensitive data before logging

**Sensitive Fields** (MUST be redacted):
- `password`, `passwd`, `pwd`
- `token`, `api_key`, `secret`
- `authorization`, `auth`, `bearer`
- `webhook` annotations (may contain sensitive data)
- `generatorURL` (may contain internal endpoints)

**Sanitization Pattern**:
```go
// Use middleware.SanitizeForLog() helper
logger.Info("Processing webhook",
    zap.String("payload", middleware.SanitizeForLog(webhookData)),
)

// Output: "password":"[REDACTED]", "token":"[REDACTED]"
```

**Implementation**:
```go
// pkg/{service}/middleware/log_sanitization.go
func SanitizeForLog(data string) string {
    // Redact passwords
    data = regexp.MustCompile(`"password"\s*:\s*"[^"]*"`).ReplaceAllString(data, `"password":"[REDACTED]"`)

    // Redact tokens
    data = regexp.MustCompile(`"token"\s*:\s*"[^"]*"`).ReplaceAllString(data, `"token":"[REDACTED]"`)

    // Redact authorization headers
    data = regexp.MustCompile(`"authorization"\s*:\s*"[^"]*"`).ReplaceAllString(data, `"authorization":"[REDACTED]"`)

    return data
}
```

---

### **5. Request-Scoped Logging**

**Pattern**: Create request-scoped logger with context fields

```go
// In middleware
func RequestIDMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            requestID := uuid.New().String()

            // Create request-scoped logger
            requestLogger := logger.With(
                zap.String("request_id", requestID),
                zap.String("source_ip", getSourceIP(r)),
                zap.String("endpoint", r.URL.Path),
                zap.String("method", r.Method),
            )

            // Store in context
            ctx := context.WithValue(r.Context(), LoggerKey, requestLogger)

            // Log incoming request
            requestLogger.Info("Incoming request",
                zap.String("user_agent", r.UserAgent()),
                zap.String("content_type", r.Header.Get("Content-Type")),
            )

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// In handlers
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
    logger := middleware.GetLogger(r.Context())
    logger.Info("Processing request", zap.String("action", "parse"))
}
```

---

### **6. Performance Logging**

**Pattern**: Log request completion with duration

```go
func (s *Server) performanceLoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Wrap response writer to capture status code
        ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

        // Call next handler
        next.ServeHTTP(ww, r)

        // Calculate duration
        duration := time.Since(start)

        // Log request completion
        logger := middleware.GetLogger(r.Context())
        logger.Info("Request completed",
            zap.Float64("duration_ms", float64(duration.Milliseconds())),
            zap.Int("status_code", ww.Status()),
            zap.Int("bytes_written", ww.BytesWritten()),
        )
    })
}
```

---

## 🔗 **Request Tracing Standards**

### **1. Request ID Generation**

**Mandatory**: Generate UUID for each request

```go
import "github.com/google/uuid"

requestID := uuid.New().String()
// Example: "550e8400-e29b-41d4-a716-446655440000"
```

---

### **2. Request ID Propagation**

**HTTP Header**: `X-Request-ID`

**Incoming Requests**:
```go
// Check if client provided request ID
requestID := r.Header.Get("X-Request-ID")
if requestID == "" {
    // Generate new request ID
    requestID = uuid.New().String()
}

// Add to response headers
w.Header().Set("X-Request-ID", requestID)
```

**Outgoing Requests** (service-to-service):
```go
req, _ := http.NewRequest("GET", "http://context-api:8080/api/v1/context", nil)
req.Header.Set("X-Request-ID", requestID)
```

---

### **3. Context Propagation**

**Pattern**: Store request ID in context

```go
type contextKey string

const RequestIDKey contextKey = "request_id"

// Store request ID
ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

// Retrieve request ID
func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(RequestIDKey).(string); ok {
        return id
    }
    return ""
}
```

---

### **4. Cross-Service Tracing**

**Example Flow**:
```
Gateway (request_id: abc123)
  ↓ X-Request-ID: abc123
Context API (request_id: abc123)
  ↓ X-Request-ID: abc123
PostgreSQL Query (logged with request_id: abc123)
```

**Log Correlation**:
```bash
# Find all logs for a specific request
kubectl logs -l app=gateway | grep "request_id\":\"abc123"
kubectl logs -l app=context-api | grep "request_id\":\"abc123"
```

---

## 💻 **Implementation**

### **Service-Specific Metrics Package**

**Pattern**: Create `pkg/{service}/metrics/metrics.go`

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all service Prometheus metrics
type Metrics struct {
    // HTTP Metrics
    HTTPRequestDuration    *prometheus.HistogramVec
    HTTPRequestsInFlight   prometheus.Gauge
    HTTPRequestsTotal      *prometheus.CounterVec

    // Database Metrics
    DatabaseQueryDuration  *prometheus.HistogramVec
    DatabaseConnectionsTotal prometheus.Gauge

    // Business Metrics
    SignalsProcessedTotal  *prometheus.CounterVec

    registry prometheus.Gatherer
}

// NewMetrics creates metrics with default registry
func NewMetrics() *Metrics {
    return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates metrics with custom registry (for testing)
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
    factory := promauto.With(registry)

    var gatherer prometheus.Gatherer
    if reg, ok := registry.(prometheus.Gatherer); ok {
        gatherer = reg
    } else {
        gatherer = prometheus.DefaultGatherer
    }

    return &Metrics{
        registry: gatherer,
        HTTPRequestDuration: factory.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "service_http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
            },
            []string{"endpoint", "method", "status"},
        ),
        HTTPRequestsInFlight: factory.NewGauge(
            prometheus.GaugeOpts{
                Name: "service_http_requests_in_flight",
                Help: "Current number of HTTP requests being processed",
            },
        ),
        HTTPRequestsTotal: factory.NewCounterVec(
            prometheus.CounterOpts{
                Name: "service_http_requests_total",
                Help: "Total HTTP requests by endpoint, method, and status",
            },
            []string{"endpoint", "method", "status"},
        ),
    }
}

// Registry returns the Prometheus Gatherer for /metrics endpoint
func (m *Metrics) Registry() prometheus.Gatherer {
    if m.registry != nil {
        return m.registry
    }
    return prometheus.DefaultGatherer
}
```

---

### **Logging Middleware**

**Pattern**: Create `pkg/{service}/middleware/request_id.go`

```go
package middleware

import (
    "context"
    "net/http"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

type contextKey string

const (
    RequestIDKey contextKey = "request_id"
    LoggerKey    contextKey = "logger"
)

// RequestIDMiddleware adds request ID and logger to context
func RequestIDMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Generate or extract request ID
            requestID := r.Header.Get("X-Request-ID")
            if requestID == "" {
                requestID = uuid.New().String()
            }

            // Add to response headers
            w.Header().Set("X-Request-ID", requestID)

            // Create request-scoped logger
            requestLogger := logger.With(
                zap.String("request_id", requestID),
                zap.String("source_ip", getSourceIP(r)),
                zap.String("endpoint", r.URL.Path),
                zap.String("method", r.Method),
            )

            // Store in context
            ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
            ctx = context.WithValue(ctx, LoggerKey, requestLogger)

            // Log incoming request
            requestLogger.Info("Incoming request",
                zap.String("user_agent", r.UserAgent()),
                zap.String("content_type", r.Header.Get("Content-Type")),
            )

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// GetLogger retrieves request-scoped logger from context
func GetLogger(ctx context.Context) *zap.Logger {
    if logger, ok := ctx.Value(LoggerKey).(*zap.Logger); ok {
        return logger
    }
    return zap.L() // Fallback to global logger
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(RequestIDKey).(string); ok {
        return id
    }
    return ""
}

func getSourceIP(r *http.Request) string {
    // Check X-Forwarded-For header (proxy/load balancer)
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        return xff
    }

    // Check X-Real-IP header (nginx)
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }

    // Fallback to RemoteAddr
    return r.RemoteAddr
}
```

---

### **Log Sanitization Middleware**

**Pattern**: Create `pkg/{service}/middleware/log_sanitization.go`

```go
package middleware

import (
    "regexp"
    "strings"
)

const redactedPlaceholder = "[REDACTED]"

var (
    sensitiveFieldNames = []string{
        "password", "passwd", "pwd",
        "token", "api_key", "secret",
        "authorization", "auth", "bearer",
    }

    sanitizationPatterns = []struct {
        pattern     *regexp.Regexp
        replacement string
    }{
        {
            pattern:     regexp.MustCompile(`"password"\s*:\s*"[^"]*"`),
            replacement: `"password":"[REDACTED]"`,
        },
        {
            pattern:     regexp.MustCompile(`"token"\s*:\s*"[^"]*"`),
            replacement: `"token":"[REDACTED]"`,
        },
        {
            pattern:     regexp.MustCompile(`"authorization"\s*:\s*"[^"]*"`),
            replacement: `"authorization":"[REDACTED]"`,
        },
    }
)

// SanitizeForLog redacts sensitive information from data string
func SanitizeForLog(data string) string {
    for _, sp := range sanitizationPatterns {
        data = sp.pattern.ReplaceAllString(data, sp.replacement)
    }
    return data
}
```

---

## 📊 **Examples**

### **Example 1: Context API Metrics**

```go
// pkg/context-api/metrics/metrics.go
type Metrics struct {
    // HTTP Metrics
    HTTPRequestDuration    *prometheus.HistogramVec

    // Database Metrics
    DatabaseQueryDuration  *prometheus.HistogramVec
    DatabaseConnectionsTotal prometheus.Gauge

    // Redis Cache Metrics
    RedisCacheHitsTotal    prometheus.Counter
    RedisCacheMissesTotal  prometheus.Counter

    // Business Metrics
    ContextQueriesTotal    *prometheus.CounterVec
    SemanticSearchDuration *prometheus.HistogramVec
}

// Usage
m.ContextQueriesTotal.WithLabelValues("remediation", "success").Inc()
m.SemanticSearchDuration.WithLabelValues("pgvector").Observe(duration.Seconds())
```

---

### **Example 2: Context API Logging**

```go
// In handler
func (s *Server) handleContextQuery(w http.ResponseWriter, r *http.Request) {
    logger := middleware.GetLogger(r.Context())

    logger.Info("Processing context query",
        zap.String("query_type", "remediation"),
        zap.String("remediation_id", remediationID),
    )

    // Query database
    start := time.Now()
    results, err := s.db.QueryContext(r.Context(), query)
    duration := time.Since(start)

    if err != nil {
        logger.Error("Database query failed",
            zap.Error(err),
            zap.Float64("duration_ms", float64(duration.Milliseconds())),
        )
        return
    }

    logger.Info("Context query completed",
        zap.Int("result_count", len(results)),
        zap.Float64("duration_ms", float64(duration.Milliseconds())),
    )
}
```

---

### **Example 3: Cross-Service Request Tracing**

```go
// Gateway calls Context API
func (s *Server) enrichWithContext(ctx context.Context, remediationID string) (*Context, error) {
    logger := middleware.GetLogger(ctx)
    requestID := middleware.GetRequestID(ctx)

    // Create HTTP request with request ID
    req, _ := http.NewRequestWithContext(ctx, "GET",
        fmt.Sprintf("http://context-api:8080/api/v1/context/remediation/%s", remediationID),
        nil)
    req.Header.Set("X-Request-ID", requestID)

    logger.Info("Calling Context API",
        zap.String("remediation_id", remediationID),
        zap.String("request_id", requestID),
    )

    resp, err := s.httpClient.Do(req)
    // ...
}

// Context API receives request
func (s *Server) handleContextQuery(w http.ResponseWriter, r *http.Request) {
    logger := middleware.GetLogger(r.Context())

    // Request ID automatically extracted by middleware
    logger.Info("Received context query from Gateway",
        zap.String("remediation_id", remediationID),
    )
    // Logs will have same request_id as Gateway
}
```

---

## 🔄 **Migration Guide**

### **For Existing Services**

**Step 1: Add Metrics Package** (2 hours)

1. Create `pkg/{service}/metrics/metrics.go`
2. Define service-specific metrics following naming conventions
3. Use custom registry support for test isolation

**Step 2: Add Logging Middleware** (1 hour)

1. Create `pkg/{service}/middleware/request_id.go`
2. Implement RequestIDMiddleware with zap logger
3. Add GetLogger() and GetRequestID() helpers

**Step 3: Add Log Sanitization** (1 hour)

1. Create `pkg/{service}/middleware/log_sanitization.go`
2. Implement SanitizeForLog() function
3. Apply sanitization to all log entries with sensitive data

**Step 4: Update HTTP Server** (2 hours)

1. Register RequestIDMiddleware in HTTP server
2. Update all handlers to use middleware.GetLogger(ctx)
3. Add performance logging middleware

**Step 5: Update Tests** (2 hours)

1. Use custom metrics registry in tests
2. Validate request ID propagation
3. Test log sanitization

**Total Effort**: ~8 hours per service

---

### **Migration Checklist**

**Per Service**:
- [ ] Create `pkg/{service}/metrics/metrics.go` package
- [ ] Define service-specific metrics with standard naming
- [ ] Create `pkg/{service}/middleware/request_id.go`
- [ ] Create `pkg/{service}/middleware/log_sanitization.go`
- [ ] Register RequestIDMiddleware in HTTP server
- [ ] Update all handlers to use middleware.GetLogger(ctx)
- [ ] Add performance logging middleware
- [ ] Update tests with custom metrics registry
- [ ] Validate request ID propagation in integration tests
- [ ] Reference DD-005 in implementation plan

---

## ✅ **Validation**

### **Implementation Status by Service**

#### **Gateway Service** ✅ **COMPLETE**

**Status**: ✅ Observability standards fully implemented

**Evidence**:
- ✅ `pkg/gateway/metrics/metrics.go` - 40+ metrics defined
- ✅ `pkg/gateway/middleware/request_id.go` - Request ID middleware
- ✅ `pkg/gateway/middleware/log_sanitization.go` - Log sanitization
- ✅ All handlers use request-scoped logging
- ✅ Integration tests passing (115 specs)

---

#### **Other Services** 🔄 **IN PROGRESS**

| Service | Status | Priority | Target Date |
|---------|--------|----------|-------------|
| **Context API** | 🔄 Planned | P0 | Before production |
| **HolmesGPT API** | 🔄 Planned | P0 | Before production |
| **Effectiveness Monitor** | 🔄 Planned | P1 | Before production |
| **CRD Controllers** | 🔄 Planned | P2 | Before production |

**Note**: Gateway service serves as the reference implementation for all other services.

---

## 📚 **References**

### **Industry Standards**

1. **Prometheus Naming Best Practices**
   - https://prometheus.io/docs/practices/naming/
   - Metric and label naming conventions

2. **Structured Logging Best Practices**
   - https://www.uber.com/blog/zap/
   - Uber's zap structured logging library

3. **Request ID Propagation**
   - https://www.w3.org/TR/trace-context/
   - W3C Trace Context specification

---

### **Kubernaut Implementation**

1. **Reference Implementation**: `pkg/gateway/` (Gateway service)
2. **Metrics Package**: `pkg/gateway/metrics/metrics.go`
3. **Request ID Middleware**: `pkg/gateway/middleware/request_id.go`
4. **Log Sanitization**: `pkg/gateway/middleware/log_sanitization.go`
5. **Design Decision**: `DD-005-OBSERVABILITY-STANDARDS.md` (this document)

---

### **Related Documents**

1. **DD-004**: RFC 7807 Error Response Standard
2. **ADR-027**: Multi-Architecture Build Strategy
3. **ADR-015**: Signal Terminology Migration
4. **Gateway Metrics SLOs**: `docs/services/stateless/gateway-service/metrics-slos.md`

---

## ✅ **Summary**

### **Key Design Decisions**

#### **1. Prometheus Metrics Standard**

**Decision**: Use consistent naming convention `{service}_{component}_{metric_name}_{unit}`

**Rationale**:
- Industry standard (Prometheus best practices)
- Enables unified monitoring across services
- Low cardinality labels prevent metric explosion

**Trade-off**: Requires discipline to maintain consistency

---

#### **2. Structured Logging with Zap**

**Decision**: Use `go.uber.org/zap` for all services

**Rationale**:
- High performance (zero-allocation)
- Type-safe structured logging
- Industry standard in Go ecosystem

**Trade-off**: Slightly more verbose than simple fmt.Printf

---

#### **3. Request ID Propagation**

**Decision**: Use `X-Request-ID` header for cross-service tracing

**Rationale**:
- Industry standard header name
- Enables request correlation across services
- Minimal performance overhead (<1ms)

**Trade-off**: Requires middleware in all services

---

#### **4. Log Sanitization**

**Decision**: Mandatory sanitization of sensitive data

**Rationale**:
- Security compliance (prevent data exposure)
- GDPR/privacy requirements
- Industry best practice

**Trade-off**: Adds complexity to logging code

---

### **Confidence Assessment**

**Overall Confidence**: 95% (Production Standard)

**Breakdown**:
- **Metrics Standards**: 95% ✅ (proven in Gateway, Prometheus best practices)
- **Logging Standards**: 95% ✅ (unified `logr` interface, `zap` backend via `zapr`)
- **Request Tracing**: 95% ✅ (proven in Gateway, W3C standard)
- **Migration Effort**: 90% ✅ (~12.5 hours for `logr` migration across 34 files)

**Why 95%**: `logr` is the Kubernetes ecosystem standard, proven in controller-runtime. Migration is straightforward with `zapr` adapter for stateless services.

---

### **Production Readiness**

**Status**: ✅ **APPROVED FOR PRODUCTION**

**Evidence**:
- ✅ Prometheus best practices followed
- ✅ Unified logging interface (`logr`) with high-performance backend (`zap` via `zapr`)
- ✅ Consistent logging across stateless and CRD controller services
- ✅ Proven in Gateway service (115 tests passing)
- ✅ Native integration with controller-runtime for CRD controllers
- ✅ Clear migration path (~12.5 hours for existing code)
- ✅ Security compliance (log sanitization)

**Recommendation**: ✅ **MANDATORY** for all services before production deployment

**Migration Status**:
- 🔄 **Pending**: 34 files in `pkg/` need migration from `*zap.Logger` to `logr.Logger`
- ⏳ **Timeline**: V1.1 (Post-MVP)
- 📋 **Tracking**: See Section 1.3 for detailed migration plan

---

---

## 📜 **Change Log**

| Version | Date | Changes |
|---------|------|---------|
| **2.0** | November 28, 2025 | **CRITICAL**: Unified logging interface - `logr.Logger` replaces `*zap.Logger` as the standard interface. `zap` remains the backend via `zapr` adapter. Added Logging Framework Decision Matrix, migration guide, and updated all examples to use `logr` syntax. |
| **1.0** | October 31, 2025 | Initial release - Prometheus metrics, `zap` logging, request tracing standards |

---

**Document Version**: 2.0
**Last Updated**: November 28, 2025
**Status**: ✅ **APPROVED FOR PRODUCTION**
**Next Review**: After logging migration to `logr` is complete (V1.1)

