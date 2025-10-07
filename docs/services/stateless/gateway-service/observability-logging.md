# Gateway Service - Observability & Logging

**Version**: v1.0
**Last Updated**: October 4, 2025
**Status**: ✅ Design Complete

---

## Structured Logging

### Log Format

Using `logrus` with JSON format for structured logging:

```go
// pkg/gateway/logging.go
package gateway

import (
    "github.com/sirupsen/logrus"
    "net/http"
)

var log = logrus.New()

func init() {
    log.SetFormatter(&logrus.JSONFormatter{
        TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
        FieldMap: logrus.FieldMap{
            logrus.FieldKeyTime:  "timestamp",
            logrus.FieldKeyLevel: "level",
            logrus.FieldKeyMsg:   "message",
        },
    })
    log.SetLevel(logrus.InfoLevel)
}

// logAlertProcessing logs alert processing with structured fields
func logAlertProcessing(
    fingerprint, alertName, environment, priority string,
    duration time.Duration,
    isStorm bool,
) {
    log.WithFields(logrus.Fields{
        "fingerprint": fingerprint,
        "alertName":   alertName,
        "environment": environment,
        "priority":    priority,
        "duration_ms": duration.Milliseconds(),
        "isStorm":     isStorm,
        "service":     "gateway",
        "component":   "alert_processing",
    }).Info("Alert processed successfully")
}
```

### Log Levels

| Level | Usage | Examples |
|-------|-------|----------|
| **ERROR** | Request failures, system errors | Redis connection failed, CRD creation failed |
| **WARN** | Degraded mode, fallback | Rego evaluation failed (using fallback), Redis slow |
| **INFO** | Normal operations | Alert received, deduplicated, CRD created |
| **DEBUG** | Detailed debugging | Fingerprint generated, environment lookup cache hit |

### Example Log Output

```json
{
  "timestamp": "2025-10-04T10:00:05.123Z",
  "level": "info",
  "message": "Alert processed successfully",
  "fingerprint": "a1b2c3d4e5...",
  "alertName": "HighMemoryUsage",
  "environment": "prod",
  "priority": "P0",
  "duration_ms": 42,
  "isStorm": false,
  "service": "gateway",
  "component": "alert_processing",
  "trace_id": "1234567890abcdef"
}
```

---

## Distributed Tracing

### OpenTelemetry Integration

```go
// pkg/gateway/tracing.go
package gateway

import (
    "context"
    "net/http"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("kubernaut.gateway")

// traceAlertProcessing creates tracing span for alert processing
func (s *Server) traceAlertProcessing(
    ctx context.Context,
    alert *NormalizedSignal,
    handler func(context.Context, *NormalizedSignal) (*AlertResponse, error),
) (*AlertResponse, error) {
    ctx, span := tracer.Start(ctx, "gateway.process_alert",
        trace.WithAttributes(
            attribute.String("alert.name", alert.AlertName),
            attribute.String("alert.fingerprint", alert.Fingerprint),
            attribute.String("alert.source", alert.SourceType),
            attribute.String("alert.severity", alert.Severity),
            attribute.String("alert.namespace", alert.Namespace),
        ),
    )
    defer span.End()

    response, err := handler(ctx, alert)
    if err != nil {
        span.RecordError(err)
        span.SetAttributes(attribute.Bool("error", true))
    } else {
        span.SetAttributes(
            attribute.String("remediation.ref", response.RemediationRequestRef),
            attribute.String("environment", response.Environment),
            attribute.String("priority", response.Priority),
            attribute.Bool("storm", response.IsStorm),
        )
    }

    return response, err
}
```

### Trace Propagation

HTTP headers for distributed tracing:
- `traceparent`: W3C Trace Context (e.g., `00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`)
- `X-Request-ID`: Request correlation ID

---

## Log Correlation

### Request ID Generation

```go
// pkg/gateway/correlation.go
package gateway

import (
    "context"
    "github.com/google/uuid"
    "net/http"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// requestIDMiddleware generates or extracts request ID
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check if request already has ID (from upstream)
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = uuid.New().String()
        }

        // Add to response headers
        w.Header().Set("X-Request-ID", requestID)

        // Add to context
        ctx := context.WithValue(r.Context(), requestIDKey, requestID)

        // Continue with request
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// getRequestID retrieves request ID from context
func getRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return "unknown"
}
```

### Correlated Log Example

```json
{
  "timestamp": "2025-10-04T10:00:05.100Z",
  "level": "info",
  "message": "Alert received",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "fingerprint": "a1b2c3d4e5...",
  "source_ip": "192.168.1.100",
  "service": "gateway"
}
{
  "timestamp": "2025-10-04T10:00:05.105Z",
  "level": "debug",
  "message": "Deduplication check",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "fingerprint": "a1b2c3d4e5...",
  "redis_latency_ms": 2
}
{
  "timestamp": "2025-10-04T10:00:05.142Z",
  "level": "info",
  "message": "CRD created",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "remediation_ref": "remediation-abc123",
  "duration_ms": 42
}
```

---

## Health Checks

### Liveness Probe

```go
// GET /health (no auth)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

### Readiness Probe

```go
// GET /ready (no auth)
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
    // Check Redis connection
    if err := s.deduplication.HealthCheck(r.Context()); err != nil {
        log.WithError(err).Error("Redis health check failed")
        http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
        return
    }

    // Check K8s API connection
    if err := s.k8sClient.List(r.Context(), &corev1.NamespaceList{}); err != nil {
        log.WithError(err).Error("Kubernetes API health check failed")
        http.Error(w, "Kubernetes API unavailable", http.StatusServiceUnavailable)
        return
    }

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("READY"))
}
```

### Kubernetes Probe Configuration

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

**Confidence**: 95%
