# Data Storage Service - Observability & Logging

**Version**: 1.0
**Last Updated**: December 4, 2025
**Status**: ✅ CURRENT
**BR Coverage**: BR-STORAGE-019 (Logging and metrics)

---

## Table of Contents

1. [Structured Logging Patterns](#structured-logging-patterns)
2. [Correlation ID Propagation](#correlation-id-propagation)
3. [Secret Sanitization](#secret-sanitization)
4. [OpenTelemetry Integration](#opentelemetry-integration)
5. [Debugging Guidelines](#debugging-guidelines)
6. [Log Aggregation](#log-aggregation)

---

## Structured Logging Patterns

### Log Format

The Data Storage Service uses **structured JSON logging** via the `zap` library for easy parsing by log aggregation systems.

**Environment Variables**:
```yaml
env:
  - name: LOG_LEVEL
    value: "info"  # Options: debug, info, warn, error
  - name: LOG_FORMAT
    value: "json"  # Options: json, console
  - name: LOG_OUTPUT
    value: "stdout"  # Options: stdout, file
```

### Standard Log Fields

All log entries include these standard fields:

```go
import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// Standard fields for all Data Storage operations
logger.Info("audit record created",
    zap.String("service", "data-storage"),
    zap.String("operation", "write"),
    zap.String("table", "notification_audit"),
    zap.String("remediation_id", audit.RemediationID),
    zap.String("correlation_id", ctx.Value("correlation_id").(string)),
    zap.Duration("duration", duration),
    zap.Int("record_count", 1),
)
```

### Log Levels

| Level | Use Case | Example |
|-------|----------|---------|
| `debug` | Detailed debugging, request/response bodies | SQL query with parameters |
| `info` | Normal operations, successful writes | "audit record created" |
| `warn` | Recoverable issues, validation failures | "validation failed for field X" |
| `error` | Non-recoverable errors, database failures | "PostgreSQL connection failed" |

### Operation-Specific Logging

#### Write Operations (BR-STORAGE-001, BR-STORAGE-002)

```go
// Success
logger.Info("write operation completed",
    zap.String("operation", "write"),
    zap.String("table", tableName),
    zap.String("remediation_id", remediationID),
    zap.Duration("duration", duration),
    zap.Bool("success", true),
)

// Failure
logger.Error("write operation failed",
    zap.String("operation", "write"),
    zap.String("table", tableName),
    zap.String("remediation_id", remediationID),
    zap.Error(err),
    zap.String("reason", classifyError(err)),
)
```

#### Dual-Write Coordination (BR-STORAGE-014)

```go
logger.Info("dual-write completed",
    zap.String("operation", "dualwrite"),
    zap.Bool("postgresql_success", pgSuccess),
    zap.Bool("vectordb_success", vdbSuccess),
    zap.Bool("fallback_mode", fallbackMode),
    zap.Duration("pg_duration", pgDuration),
    zap.Duration("vdb_duration", vdbDuration),
)
```

#### Semantic Search (BR-STORAGE-012)

```go
logger.Info("semantic search executed",
    zap.String("operation", "semantic_search"),
    zap.Int("results_count", len(results)),
    zap.Float64("top_similarity", results[0].Similarity),
    zap.Duration("embedding_time", embeddingDuration),
    zap.Duration("search_time", searchDuration),
)
```

---

## Correlation ID Propagation

### Overview

Correlation IDs enable request tracing across multiple services in the Kubernaut system. Every request should carry a unique correlation ID from entry to exit.

### HTTP Header Extraction

```go
const CorrelationIDHeader = "X-Correlation-ID"

func extractCorrelationID(r *http.Request) string {
    correlationID := r.Header.Get(CorrelationIDHeader)
    if correlationID == "" {
        correlationID = uuid.New().String()
    }
    return correlationID
}

// Middleware example
func correlationIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        correlationID := extractCorrelationID(r)
        ctx := context.WithValue(r.Context(), "correlation_id", correlationID)
        
        // Set response header for downstream tracking
        w.Header().Set(CorrelationIDHeader, correlationID)
        
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Context-Aware Logging

```go
func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
    correlationID := r.Context().Value("correlation_id").(string)
    
    logger := s.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("method", r.Method),
        zap.String("path", r.URL.Path),
    )
    
    logger.Info("processing write request")
    // ... handle request
    logger.Info("write request completed")
}
```

### Downstream Propagation

When calling other services, pass the correlation ID:

```go
func (c *EmbeddingClient) GenerateEmbedding(ctx context.Context, text string) (*pgvector.Vector, error) {
    correlationID := ctx.Value("correlation_id").(string)
    
    req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embed", body)
    req.Header.Set("X-Correlation-ID", correlationID)
    
    // ... make request
}
```

---

## Secret Sanitization

### Sensitive Data Rules

**NEVER log**:
- Database passwords or connection strings with passwords
- API keys or tokens
- User PII (personally identifiable information)
- Full audit record content (may contain sensitive data)

**ALWAYS sanitize**:
- Connection strings → log host:port only
- Request bodies → redact sensitive fields
- Error messages → sanitize stack traces

### Implementation

```go
// Safe connection string logging
func sanitizeConnStr(connStr string) string {
    // Only log host and database name, never password
    re := regexp.MustCompile(`password=\S+`)
    return re.ReplaceAllString(connStr, "password=***")
}

logger.Info("connecting to database",
    zap.String("connection", sanitizeConnStr(connStr)),
)

// Audit record logging - only metadata, not content
logger.Info("audit created",
    zap.String("id", audit.ID),
    zap.String("remediation_id", audit.RemediationID),
    zap.String("namespace", audit.Namespace),
    // ❌ NEVER: zap.String("content", audit.Content)
    // ❌ NEVER: zap.String("api_key", apiKey)
)
```

---

## OpenTelemetry Integration

### Tracing Configuration

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initTracer() (*trace.TracerProvider, error) {
    exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint(os.Getenv("JAEGER_ENDPOINT")),
    ))
    if err != nil {
        return nil, err
    }
    
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
        trace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String("data-storage-service"),
        )),
    )
    
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

### Span Creation

```go
func (r *Repository) Write(ctx context.Context, audit *models.Audit) error {
    ctx, span := otel.Tracer("data-storage").Start(ctx, "repository.write")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("table", "notification_audit"),
        attribute.String("remediation_id", audit.RemediationID),
    )
    
    // ... perform write
    
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }
    
    span.SetStatus(codes.Ok, "write successful")
    return nil
}
```

---

## Debugging Guidelines

### Common Debug Scenarios

#### 1. Write Failures

```bash
# View write failure logs
kubectl logs -n kubernaut deployment/data-storage-service | \
  grep -E '"operation":"write".*"success":false'

# Check failure reasons
kubectl logs -n kubernaut deployment/data-storage-service | \
  jq 'select(.operation == "write" and .success == false) | {reason, error}'
```

#### 2. Slow Queries

```bash
# Find queries taking > 100ms
kubectl logs -n kubernaut deployment/data-storage-service | \
  jq 'select(.operation == "query" and .duration_ms > 100) | {path, duration_ms, query_type}'
```

#### 3. Database Connection Issues

```bash
# Check PostgreSQL connectivity
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U db_user -d action_history -c "SELECT 1;"

# View connection pool status
kubectl logs -n kubernaut deployment/data-storage-service | \
  grep -E 'connection pool|pg_stat_activity'
```

#### 4. Correlation ID Tracing

```bash
# Trace a specific request across logs
CORRELATION_ID="abc-123-def"
kubectl logs -n kubernaut deployment/data-storage-service | \
  jq --arg cid "$CORRELATION_ID" 'select(.correlation_id == $cid)'
```

### Log Analysis Commands

```bash
# Error rate over last 5 minutes
kubectl logs -n kubernaut deployment/data-storage-service --since=5m | \
  jq 'select(.level == "error")' | wc -l

# Top error messages
kubectl logs -n kubernaut deployment/data-storage-service --since=1h | \
  jq 'select(.level == "error") | .msg' | sort | uniq -c | sort -rn | head -10

# Write operations by table
kubectl logs -n kubernaut deployment/data-storage-service --since=1h | \
  jq 'select(.operation == "write") | .table' | sort | uniq -c
```

---

## Log Aggregation

### Fluentd/Fluent Bit Configuration

**Kubernetes ConfigMap**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluentd-config
  namespace: logging
data:
  fluent.conf: |
    <source>
      @type tail
      path /var/log/containers/data-storage-service-*.log
      pos_file /var/log/fluentd-data-storage.pos
      tag kubernetes.data-storage
      <parse>
        @type json
        time_key time
        time_format %Y-%m-%dT%H:%M:%S.%NZ
      </parse>
    </source>

    <filter kubernetes.data-storage>
      @type parser
      key_name log
      <parse>
        @type json
      </parse>
    </filter>

    <match kubernetes.data-storage>
      @type elasticsearch
      host elasticsearch.logging.svc.cluster.local
      port 9200
      index_name data-storage-logs
      type_name _doc
      logstash_format true
      logstash_prefix data-storage
    </match>
```

### Elasticsearch Query Examples

```bash
# High-level errors
level:error AND service:data-storage

# Write failures in last hour
level:error AND operation:write AND @timestamp:[now-1h TO now]

# PostgreSQL connection errors
level:error AND (postgres OR postgresql) AND service:data-storage

# Validation failures
level:warn AND validation AND service:data-storage

# Slow operations (>500ms)
duration_ms:>500 AND service:data-storage
```

### Kibana Dashboard Suggestions

1. **Error Rate Panel**: `sum(rate(level:error[5m]))`
2. **Write Latency Histogram**: `duration_ms` field distribution
3. **Top Error Messages**: Terms aggregation on `msg` field
4. **Request Volume**: Count by `operation` field

---

## Related Documentation

- [metrics-slos.md](./metrics-slos.md) - SLIs, SLOs, and Prometheus metrics
- [observability/ALERTING_RUNBOOK.md](./observability/ALERTING_RUNBOOK.md) - Alert troubleshooting
- [observability/PROMETHEUS_QUERIES.md](./observability/PROMETHEUS_QUERIES.md) - Query reference
- [observability/DEPLOYMENT_CONFIGURATION.md](./observability/DEPLOYMENT_CONFIGURATION.md) - Setup guide

---

**Document Version**: 1.0
**Changelog**:
- v1.0 (Dec 4, 2025): Initial document - consolidated from observability/ folder per SERVICE_DOCUMENTATION_GUIDE.md

