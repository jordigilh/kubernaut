# Dynamic Toolset Service - Observability & Logging

**Version**: v1.0
**Last Updated**: October 10, 2025
**Status**: ✅ Design Complete

---

## Structured Logging

### Log Format

Using `zap` with JSON format for structured logging:

```go
// pkg/toolset/logging.go
package toolset

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func init() {
    config := zap.NewProductionConfig()
    config.EncoderConfig.TimeKey = "timestamp"
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

    logger, _ = config.Build()
}

// logServiceDiscovery logs service discovery with structured fields
func logServiceDiscovery(
    serviceType, serviceName, namespace, endpoint string,
    healthy bool,
    duration time.Duration,
) {
    logger.Info("Service discovered",
        zap.String("service_type", serviceType),
        zap.String("service_name", serviceName),
        zap.String("namespace", namespace),
        zap.String("endpoint", endpoint),
        zap.Bool("healthy", healthy),
        zap.Duration("duration", duration),
        zap.String("component", "service_discovery"),
    )
}

// logConfigMapReconciliation logs reconciliation with structured fields
func logConfigMapReconciliation(
    operation, status string,
    driftKeys []string,
    duration time.Duration,
) {
    logger.Info("ConfigMap reconciliation",
        zap.String("operation", operation), // "create", "update", "drift_detected"
        zap.String("status", status), // "success", "failure"
        zap.Strings("drift_keys", driftKeys),
        zap.Duration("duration", duration),
        zap.String("component", "reconciliation"),
    )
}
```

### Log Levels

| Level | Usage | Examples |
|-------|-------|----------|
| **ERROR** | Service failures, critical errors | Discovery failed, ConfigMap update failed, K8s API error |
| **WARN** | Degraded mode, transient failures | Service health check failed, drift detected, slow discovery |
| **INFO** | Normal operations | Service discovered, ConfigMap reconciled, health check passed |
| **DEBUG** | Detailed debugging | Detector evaluation, cache hit, endpoint construction |

### Example Log Output

**Service Discovery Log**:
```json
{
  "timestamp": "2025-10-10T10:00:05.123Z",
  "level": "info",
  "message": "Service discovered",
  "service_type": "prometheus",
  "service_name": "prometheus-server",
  "namespace": "monitoring",
  "endpoint": "http://prometheus-server.monitoring.svc.cluster.local:9090",
  "healthy": true,
  "duration": "142ms",
  "component": "service_discovery",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**ConfigMap Reconciliation Log**:
```json
{
  "timestamp": "2025-10-10T10:05:30.456Z",
  "level": "info",
  "message": "ConfigMap reconciliation",
  "operation": "update",
  "status": "success",
  "drift_keys": ["prometheus-toolset.yaml"],
  "duration": "89ms",
  "component": "reconciliation",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Health Check Log**:
```json
{
  "timestamp": "2025-10-10T10:10:15.789Z",
  "level": "warn",
  "message": "Service health check failed",
  "service_type": "grafana",
  "service_name": "grafana",
  "namespace": "monitoring",
  "endpoint": "http://grafana.monitoring.svc.cluster.local:3000/api/health",
  "error": "context deadline exceeded",
  "retry_attempt": 1,
  "component": "health_check"
}
```

---

## Logging Patterns

### Service Discovery Logging

**Start of Discovery**:
```go
logger.Info("Starting service discovery",
    zap.Duration("interval", 5*time.Minute),
    zap.Int("registered_detectors", len(d.detectors)))
```

**Service Detected**:
```go
logger.Info("Service detected",
    zap.String("service_type", svc.Type),
    zap.String("service_name", svc.Name),
    zap.String("namespace", svc.Namespace),
    zap.String("endpoint", svc.Endpoint))
```

**Health Check Failed**:
```go
logger.Warn("Service health check failed, skipping",
    zap.String("service_type", svc.Type),
    zap.String("service_name", svc.Name),
    zap.String("endpoint", svc.Endpoint),
    zap.Error(err))
```

**Discovery Complete**:
```go
logger.Info("Service discovery complete",
    zap.Int("discovered_count", len(discovered)),
    zap.Duration("duration", time.Since(startTime)))
```

---

### ConfigMap Reconciliation Logging

**Start of Reconciliation**:
```go
logger.Debug("Reconciling ConfigMap",
    zap.String("configmap", r.configMapName),
    zap.String("namespace", r.namespace))
```

**Drift Detected**:
```go
logger.Info("ConfigMap drift detected, reconciling",
    zap.Strings("drift_keys", driftDetails),
    zap.Int("drift_count", len(driftDetails)))
```

**Admin Overrides Preserved**:
```go
logger.Debug("Preserved admin overrides",
    zap.String("override_key", "overrides.yaml"))
```

**ConfigMap Updated**:
```go
logger.Info("ConfigMap updated successfully",
    zap.String("configmap", cm.Name),
    zap.Int("keys_updated", len(cm.Data)))
```

---

### HTTP API Logging

**Request Received**:
```go
logger.Info("HTTP request received",
    zap.String("method", r.Method),
    zap.String("path", r.URL.Path),
    zap.String("remote_addr", r.RemoteAddr),
    zap.String("user_agent", r.UserAgent()),
    zap.String("request_id", requestID))
```

**Manual Discovery Triggered**:
```go
logger.Info("Manual discovery triggered",
    zap.String("triggered_by", "api_request"),
    zap.String("request_id", requestID))
```

**API Response**:
```go
logger.Info("HTTP request completed",
    zap.String("method", r.Method),
    zap.String("path", r.URL.Path),
    zap.Int("status_code", statusCode),
    zap.Duration("duration", time.Since(startTime)),
    zap.String("request_id", requestID))
```

---

## Log Correlation

### Request ID Generation

```go
// pkg/toolset/correlation.go
package toolset

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

        // Add to logger
        logger = logger.With(zap.String("request_id", requestID))

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

Discovery flow with correlated logs:

```json
{
  "timestamp": "2025-10-10T10:00:05.000Z",
  "level": "info",
  "message": "Starting service discovery",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "interval": "5m0s",
  "registered_detectors": 4
}
{
  "timestamp": "2025-10-10T10:00:05.142Z",
  "level": "info",
  "message": "Service detected",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "service_type": "prometheus",
  "service_name": "prometheus-server",
  "namespace": "monitoring"
}
{
  "timestamp": "2025-10-10T10:00:05.285Z",
  "level": "info",
  "message": "Service discovery complete",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "discovered_count": 3,
  "duration": "285ms"
}
```

---

## Sensitive Data Sanitization

### API Keys in Toolset Configs

**Problem**: Toolset configurations may contain API keys (e.g., Grafana API key).

**Solution**: Sanitize logs to never log actual API keys.

```go
// pkg/toolset/sanitize.go
package toolset

import "regexp"

var (
    // Regex patterns for sensitive data
    apiKeyPattern = regexp.MustCompile(`apiKey:\s*"([^"]+)"`)
    tokenPattern  = regexp.MustCompile(`token:\s*"([^"]+)"`)
)

// sanitizeConfigMap sanitizes ConfigMap data before logging
func sanitizeConfigMap(data map[string]string) map[string]string {
    sanitized := make(map[string]string)
    for key, value := range data {
        sanitized[key] = sanitizeString(value)
    }
    return sanitized
}

// sanitizeString replaces sensitive values with placeholders
func sanitizeString(s string) string {
    s = apiKeyPattern.ReplaceAllString(s, `apiKey: "***REDACTED***"`)
    s = tokenPattern.ReplaceAllString(s, `token: "***REDACTED***"`)
    return s
}
```

**Example**:
```go
// WRONG: Logs actual API key
logger.Info("Generated toolset config", zap.String("config", configData))

// CORRECT: Sanitizes before logging
logger.Info("Generated toolset config",
    zap.String("config", sanitizeString(configData)))
```

---

## Error Logging with Stack Traces

### Error Wrapping

```go
// pkg/toolset/errors.go
package toolset

import (
    "fmt"
    "go.uber.org/zap"
)

// logErrorWithStack logs error with full stack trace
func logErrorWithStack(err error, message string, fields ...zap.Field) {
    allFields := append(fields, zap.Error(err), zap.Stack("stacktrace"))
    logger.Error(message, allFields...)
}

// Example usage
if err := d.DiscoverServices(ctx); err != nil {
    logErrorWithStack(err, "Service discovery failed",
        zap.String("component", "discovery"),
        zap.Int("detectors_count", len(d.detectors)))
    return err
}
```

**Example Error Log with Stack Trace**:
```json
{
  "timestamp": "2025-10-10T10:00:05.500Z",
  "level": "error",
  "message": "Service discovery failed",
  "component": "discovery",
  "detectors_count": 4,
  "error": "failed to list services: connection refused",
  "stacktrace": "goroutine 1 [running]:\ngithub.com/jordigilh/kubernaut/pkg/toolset/discovery.(*ServiceDiscovererImpl).DiscoverServices(...)\n\t/app/pkg/toolset/discovery/discoverer.go:45 +0x2a5\n..."
}
```

---

## Log Aggregation

### Elasticsearch Integration

**Filebeat Configuration** (for log shipping):

```yaml
filebeat.inputs:
  - type: container
    paths:
      - '/var/log/containers/dynamic-toolset-*.log'
    json.keys_under_root: true
    json.add_error_key: true

output.elasticsearch:
  hosts: ["elasticsearch.logging:9200"]
  index: "dynamic-toolset-%{+yyyy.MM.dd}"

processors:
  - add_kubernetes_metadata:
      in_cluster: true
      default_indexers.enabled: true
      default_matchers.enabled: true
```

### Log Retention Policies

| Log Type | Retention | Rationale |
|----------|-----------|-----------|
| **ERROR logs** | 90 days | Long retention for post-mortem analysis |
| **WARN logs** | 30 days | Medium retention for pattern analysis |
| **INFO logs** | 7 days | Short retention, high volume |
| **DEBUG logs** | 1 day | Very short retention, debugging only |

---

## Health Checks

### Liveness Probe

```go
// GET /health (no auth)
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

### Readiness Probe

```go
// GET /ready (no auth)
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
    // Check Kubernetes API connection
    _, err := s.k8sClient.CoreV1().Services("").List(r.Context(), metav1.ListOptions{Limit: 1})
    if err != nil {
        logger.Error("Kubernetes API health check failed", zap.Error(err))
        http.Error(w, "Kubernetes API unavailable", http.StatusServiceUnavailable)
        return
    }

    // Check ConfigMap exists
    _, err = s.k8sClient.CoreV1().ConfigMaps("kubernaut-system").
        Get(r.Context(), "kubernaut-toolset-config", metav1.GetOptions{})
    if err != nil {
        logger.Warn("ConfigMap health check failed", zap.Error(err))
        // Still ready, ConfigMap will be created by reconciler
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
  timeoutSeconds: 3
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 2
```

---

## Observability Best Practices

### 1. Always Include Request ID
```go
logger.Info("Processing request", zap.String("request_id", getRequestID(ctx)))
```

### 2. Log Start and End of Operations
```go
logger.Info("Starting service discovery")
// ... discovery logic ...
logger.Info("Service discovery complete", zap.Duration("duration", elapsed))
```

### 3. Log Errors with Context
```go
logger.Error("Failed to detect service",
    zap.String("service_type", "prometheus"),
    zap.Error(err),
    zap.String("namespace", namespace))
```

### 4. Use Structured Fields
```go
// WRONG: Unstructured message
logger.Info(fmt.Sprintf("Discovered %d services in %s", count, duration))

// CORRECT: Structured fields
logger.Info("Service discovery complete",
    zap.Int("discovered_count", count),
    zap.Duration("duration", duration))
```

### 5. Sanitize Sensitive Data
```go
// ALWAYS sanitize before logging ConfigMap data
logger.Debug("ConfigMap content",
    zap.Any("data", sanitizeConfigMap(cm.Data)))
```

---

**Document Status**: ✅ Complete Observability & Logging Guide
**Last Updated**: October 10, 2025
**Confidence**: 95% (Very High)

