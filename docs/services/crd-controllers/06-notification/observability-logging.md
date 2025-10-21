# Notification Service - Observability & Logging

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service
**Status**: ⚠️ NEEDS IMPLEMENTATION

---

## 📋 Overview

Comprehensive observability and logging configuration for the Notification Service, covering metrics, logging, tracing, and alerting.

---

## 📊 **Prometheus Metrics**

### **HTTP Metrics**

```go
package notification

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    httpRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )

    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "notification_http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets, // 0.005 to 10 seconds
        },
        []string{"method", "endpoint"},
    )
)
```

---

### **Notification Delivery Metrics**

```go
var (
    notificationDeliverTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_delivery_total",
            Help: "Total notifications delivered",
        },
        []string{"channel", "status"}, // status: "success", "failure"
    )

    notificationDeliveryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "notification_delivery_duration_seconds",
            Help:    "Notification delivery duration",
            Buckets: prometheus.LinearBuckets(0.1, 0.1, 20), // 0.1s to 2s
        },
        []string{"channel"},
    )

    notificationPayloadSize = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "notification_payload_size_bytes",
            Help:    "Notification payload size",
            Buckets: prometheus.ExponentialBuckets(1024, 2, 15), // 1KB to 16MB
        },
        []string{"channel"},
    )
)
```

---

### **Sanitization Metrics** (BR-NOT-034)

```go
var (
    sanitizationActionsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_sanitization_actions_total",
            Help: "Total sanitization actions applied",
        },
        []string{"type"}, // "api_key", "password", "email", etc.
    )

    sanitizationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "notification_sanitization_duration_seconds",
            Help:    "Sanitization duration",
            Buckets: prometheus.LinearBuckets(0.001, 0.001, 10), // 1ms to 10ms
        },
    )
)
```

---

### **Channel Health Metrics**

```go
var (
    channelHealthStatus = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "notification_channel_health_status",
            Help: "Channel health status (1=healthy, 0=unhealthy)",
        },
        []string{"channel"},
    )

    circuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "notification_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"channel"},
    )

    channelRateLimitExceeded = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_rate_limit_exceeded_total",
            Help: "Rate limit violations",
        },
        []string{"channel", "recipient"},
    )
)
```

---

## 📝 **Structured Logging** (Zap)

### **Logger Configuration**

```go
package notification

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func NewLogger() (*zap.Logger, error) {
    config := zap.NewProductionConfig()

    // Custom encoder config
    config.EncoderConfig.TimeKey = "timestamp"
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

    // Log level from environment
    config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

    // Output paths
    config.OutputPaths = []string{"stdout"}
    config.ErrorOutputPaths = []string{"stderr"}

    return config.Build()
}
```

---

### **Log Levels**

| Level | Use Case | Example |
|-------|----------|---------|
| **DEBUG** | Detailed debugging | Payload content (sanitized), adapter logic |
| **INFO** | Normal operations | Notification delivered, channel selected |
| **WARN** | Non-critical issues | Rate limit approaching, channel degraded |
| **ERROR** | Critical failures | Delivery failed, authentication failed |
| **FATAL** | Unrecoverable errors | Startup failure, missing secrets |

---

### **Logging Examples**

```go
// INFO: Successful delivery
logger.Info("Notification delivered",
    zap.String("notificationId", "notif-abc123"),
    zap.String("recipient", "sre@company.com"),
    zap.Strings("channels", []string{"email", "slack"}),
    zap.Duration("duration", 450*time.Millisecond),
    zap.String("sanitizationActions", "Redacted 2 API keys"),
)

// WARN: Rate limit approaching
logger.Warn("Rate limit approaching",
    zap.String("recipient", "user@company.com"),
    zap.Int("currentRate", 85),
    zap.Int("limit", 100),
)

// ERROR: Delivery failure
logger.Error("Notification delivery failed",
    zap.String("notificationId", "notif-xyz789"),
    zap.String("channel", "slack"),
    zap.Error(err),
    zap.String("webhookURL", "[REDACTED]"), // Never log secrets!
)
```

---

### **Correlation IDs**

```go
// Add correlation ID to all logs
func (s *NotificationService) HandleEscalation(w http.ResponseWriter, r *http.Request) {
    correlationID := r.Header.Get("X-Correlation-ID")
    if correlationID == "" {
        correlationID = uuid.New().String()
    }

    logger := s.logger.With(
        zap.String("correlationId", correlationID),
        zap.String("endpoint", "/api/v1/notify/escalation"),
    )

    logger.Info("Processing escalation request")
    // ... handle request ...
}
```

---

## 🔍 **Health Checks**

### **Liveness Probe** (`/health`)

**Port**: 8080
**Authentication**: None

```go
func (s *NotificationService) HealthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status":    "OK",
        "timestamp": time.Now().Format(time.RFC3339),
    })
}
```

**Kubernetes Configuration**:
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

---

### **Readiness Probe** (`/ready`)

**Port**: 8080
**Authentication**: None

```go
func (s *NotificationService) ReadyHandler(w http.ResponseWriter, r *http.Request) {
    // Check channel health
    channels := map[string]string{
        "email":     s.checkEmailHealth(),
        "slack":     s.checkSlackHealth(),
        "teams":     s.checkTeamsHealth(),
        "sms":       s.checkSMSHealth(),
        "pagerduty": s.checkPagerDutyHealth(),
    }

    allHealthy := true
    for _, status := range channels {
        if status != "healthy" {
            allHealthy = false
            break
        }
    }

    w.Header().Set("Content-Type", "application/json")
    if allHealthy {
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status":    "READY",
            "channels":  channels,
            "timestamp": time.Now().Format(time.RFC3339),
        })
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status":    "NOT_READY",
            "channels":  channels,
            "timestamp": time.Now().Format(time.RFC3339),
        })
    }
}
```

**Kubernetes Configuration**:
```yaml
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

## 📈 **Grafana Dashboard**

### **Key Metrics to Display**

1. **Delivery Rate** (req/s) - `rate(notification_delivery_total[5m])`
2. **Success Rate** (%) - `rate(notification_delivery_total{status="success"}[5m]) / rate(notification_delivery_total[5m])`
3. **Latency** (p50, p95, p99) - `histogram_quantile(0.95, notification_delivery_duration_seconds)`
4. **Channel Health** - `notification_channel_health_status`
5. **Sanitization Actions** - `rate(notification_sanitization_actions_total[5m])`
6. **Rate Limit Hits** - `rate(notification_rate_limit_exceeded_total[5m])`

### **Example Dashboard JSON** (excerpt)

```json
{
  "dashboard": {
    "title": "Notification Service",
    "panels": [
      {
        "title": "Delivery Rate",
        "targets": [{
          "expr": "rate(notification_delivery_total[5m])"
        }]
      },
      {
        "title": "Success Rate",
        "targets": [{
          "expr": "rate(notification_delivery_total{status=\"success\"}[5m]) / rate(notification_delivery_total[5m]) * 100"
        }]
      }
    ]
  }
}
```

---

## 🚨 **Prometheus Alert Rules**

### **Critical Alerts**

```yaml
groups:
- name: notification-service-critical
  interval: 30s
  rules:
  # High failure rate
  - alert: NotificationHighFailureRate
    expr: |
      rate(notification_delivery_total{status="failure"}[5m]) /
      rate(notification_delivery_total[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
      service: notification
    annotations:
      summary: "Notification failure rate > 10%"
      description: "{{ $value | humanizePercentage }} of notifications failing"

  # Channel unavailable
  - alert: NotificationChannelDown
    expr: notification_channel_health_status == 0
    for: 2m
    labels:
      severity: critical
      service: notification
    annotations:
      summary: "Notification channel {{ $labels.channel }} is down"
      description: "Channel health check failing"

  # Circuit breaker open
  - alert: NotificationCircuitBreakerOpen
    expr: notification_circuit_breaker_state == 1
    for: 5m
    labels:
      severity: warning
      service: notification
    annotations:
      summary: "Circuit breaker open for {{ $labels.channel }}"
      description: "Too many failures, channel disabled"
```

### **Warning Alerts**

```yaml
  # High latency
  - alert: NotificationHighLatency
    expr: |
      histogram_quantile(0.95,
        rate(notification_delivery_duration_seconds_bucket[5m])
      ) > 0.5
    for: 10m
    labels:
      severity: warning
      service: notification
    annotations:
      summary: "Notification p95 latency > 500ms"
      description: "Slow notification delivery detected"

  # Rate limit approaching
  - alert: NotificationRateLimitApproaching
    expr: rate(notification_rate_limit_exceeded_total[5m]) > 0
    for: 5m
    labels:
      severity: warning
      service: notification
    annotations:
      summary: "Rate limits being exceeded"
      description: "{{ $labels.channel }} rate limit hit for {{ $labels.recipient }}"
```

---

## 🎯 **SLO Targets**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Availability** | 99.5% | Uptime (health check) |
| **Success Rate** | 99%+ | Delivery success / total |
| **Latency (p95)** | < 500ms | End-to-end delivery |
| **Latency (p99)** | < 1s | Worst case |
| **Throughput** | 100 req/s | Sustained load |

---

## 📊 **Metrics Endpoint**

**URL**: `http://notification-service.kubernaut-system.svc.cluster.local:9090/metrics`
**Port**: 9090
**Authentication**: Kubernetes TokenReviewer (required)

**Kubernetes ServiceMonitor**:
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: notification-service
  namespace: kubernaut-system
spec:
  selector:
    matchLabels:
      app: notification-service
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    scheme: http
```

---

## 🔐 **Log Sanitization** (CRITICAL)

### **NEVER Log Secrets**

```go
// ❌ WRONG - Logs webhook URL (contains secret token)
logger.Info("Delivering to Slack", zap.String("webhookURL", webhookURL))

// ✅ CORRECT - Redacts sensitive parts
logger.Info("Delivering to Slack", zap.String("webhookURL", "[REDACTED]"))
```

### **Sanitize Before Logging**

```go
func sanitizeForLog(payload string) string {
    // Remove sensitive patterns before logging
    sanitized := regexp.MustCompile(`(?i)(api[_-]?key|token)["\s:=]+([a-zA-Z0-9_\-]{20,})`).
        ReplaceAllString(payload, "$1=[REDACTED]")
    return sanitized
}

logger.Debug("Notification payload", zap.String("payload", sanitizeForLog(payloadJSON)))
```

---

## ✅ **Observability Checklist**

### **Metrics**
- [ ] HTTP request metrics exposed
- [ ] Delivery success/failure counters
- [ ] Latency histograms per channel
- [ ] Sanitization action counters
- [ ] Channel health gauges
- [ ] Circuit breaker state gauges
- [ ] Rate limit violation counters

### **Logging**
- [ ] Structured logging (Zap) configured
- [ ] Correlation IDs added to all logs
- [ ] Log levels appropriately set
- [ ] Secrets never logged
- [ ] Sanitization applied before logging

### **Health Checks**
- [ ] Liveness probe implemented (`/health`)
- [ ] Readiness probe implemented (`/ready`)
- [ ] Channel health checks working

### **Dashboards**
- [ ] Grafana dashboard created
- [ ] Key metrics visualized
- [ ] SLO indicators displayed

### **Alerts**
- [ ] Critical alerts configured
- [ ] Warning alerts configured
- [ ] Alert routing to on-call

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

